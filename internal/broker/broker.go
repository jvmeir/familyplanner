// Package broker keeps each widget's data fresh in widget_cache so the kiosk
// read path never blocks on (or hammers) external services. On a fetch error it
// keeps the last-good value and marks the cache stale.
package broker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"golang.org/x/oauth2"

	"github.com/jvmeir/familyplanner/internal/crypto"
	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/oauth"
	"github.com/jvmeir/familyplanner/internal/widget"
)

const tsLayout = "2006-01-02 15:04:05" // matches SQLite datetime('now') (UTC)

// Broker periodically refreshes expired widget caches.
type Broker struct {
	store      *db.Store
	reg        *widget.Registry
	now        func() time.Time
	key        []byte                            // app encryption key, to decrypt data-source secrets
	oauthCreds func(dsType string) (id, secret string) // app-level OAuth client credentials
	interval   time.Duration
}

// New creates a broker scanning for expired caches every 10s.
func New(store *db.Store, reg *widget.Registry, now func() time.Time, key []byte, oauthCreds func(string) (string, string)) *Broker {
	if oauthCreds == nil {
		oauthCreds = func(string) (string, string) { return "", "" }
	}
	return &Broker{store: store, reg: reg, now: now, key: key, oauthCreds: oauthCreds, interval: 10 * time.Second}
}

// Start runs the refresh loop until ctx is cancelled.
func (b *Broker) Start(ctx context.Context) {
	t := time.NewTicker(b.interval)
	defer t.Stop()
	b.refreshExpired(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.refreshExpired(ctx)
		}
	}
}

func (b *Broker) refreshExpired(ctx context.Context) {
	ids, err := b.store.ListExpiredWidgetIDs(ctx)
	if err != nil {
		slog.Error("broker: list expired", "err", err)
		return
	}
	for _, id := range ids {
		wgt, err := b.store.GetWidget(ctx, id)
		if err != nil {
			continue
		}
		b.RefreshWidget(ctx, wgt)
	}
}

// RefreshWidget fetches one widget's data and writes the cache. On error it
// keeps any existing value (marking it stale) or records an error row.
func (b *Broker) RefreshWidget(ctx context.Context, wgt dbgen.Widget) {
	typ, ok := b.reg.Get(wgt.Type)
	if !ok {
		b.markErr(ctx, wgt.ID, "unknown widget type")
		return
	}
	prov, err := typ.NewProvider(json.RawMessage(wgt.ConfigJson), b.sourcesFor(ctx, wgt.ID), b.now)
	if err != nil {
		b.markErr(ctx, wgt.ID, err.Error())
		return
	}
	data, ttl, err := prov.Fetch(ctx)
	if err != nil {
		b.markErr(ctx, wgt.ID, err.Error())
		return
	}
	js, err := json.Marshal(data)
	if err != nil {
		b.markErr(ctx, wgt.ID, err.Error())
		return
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if err := b.store.UpsertWidgetCache(ctx, dbgen.UpsertWidgetCacheParams{
		WidgetID:  wgt.ID,
		DataJson:  string(js),
		ExpiresAt: b.now().UTC().Add(ttl).Format(tsLayout),
		Status:    "ok",
		ErrorMsg:  "",
	}); err != nil {
		slog.Error("broker: upsert cache", "widget", wgt.ID, "err", err)
	}
}

// sourcesFor resolves a widget's linked data sources into provider inputs.
func (b *Broker) sourcesFor(ctx context.Context, widgetID int64) []widget.SourceInput {
	rows, err := b.store.ListWidgetSources(ctx, widgetID)
	if err != nil {
		return nil
	}
	out := make([]widget.SourceInput, 0, len(rows))
	for _, r := range rows {
		var secret json.RawMessage
		if r.SourceSecret != "" {
			if pt, err := crypto.Open(r.SourceSecret, b.key); err == nil {
				secret = pt
			}
		}
		// For OAuth2 sources, refresh the token (persisting rotations) and hand
		// the widget just a fresh access token.
		if oauth.Known(r.SourceType) && len(secret) > 0 {
			if tok := b.refreshOAuth(ctx, r.SourceType, r.DataSourceID, secret); tok != nil {
				secret = tok
			}
		}
		out = append(out, widget.SourceInput{
			Type:     r.SourceType,
			Config:   json.RawMessage(r.SourceConfig),
			Secret:   secret,
			Filter:   r.Filter,
			Resource: r.Resource,
		})
	}
	return out
}

// refreshOAuth refreshes a stored token, persists it if rotated, records the
// source's auth health, and returns a {"access_token": ...} secret for the
// widget (nil on failure).
func (b *Broker) refreshOAuth(ctx context.Context, dsType string, dsID int64, secret json.RawMessage) json.RawMessage {
	var sec struct {
		Token *oauth2.Token `json:"token"`
	}
	if err := json.Unmarshal(secret, &sec); err != nil || sec.Token == nil {
		b.recordSourceHealth(ctx, dsID, "", string(oauth.ErrReconnect), "geen token")
		return nil
	}
	clientID, clientSecret := b.oauthCreds(dsType)
	fresh, err := oauth.FreshToken(ctx, dsType, clientID, clientSecret, sec.Token)
	if err != nil {
		// Keep the last-known access-token expiry so the admin can see it.
		b.recordSourceHealth(ctx, dsID, expiryStr(sec.Token), string(oauth.ClassifyError(err)), err.Error())
		return nil
	}
	if fresh.AccessToken != sec.Token.AccessToken {
		if ns, err := json.Marshal(map[string]any{"token": fresh}); err == nil {
			if cipher, err := crypto.Seal(ns, b.key); err == nil {
				_ = b.store.UpdateDataSourceSecret(ctx, dbgen.UpdateDataSourceSecretParams{
					SecretCiphertext: cipher, OauthStatus: "connected", ID: dsID,
				})
			}
		}
	}
	b.recordSourceHealth(ctx, dsID, expiryStr(fresh), "ok", "")
	out, _ := json.Marshal(map[string]string{"access_token": fresh.AccessToken})
	return out
}

// recordSourceHealth persists a data source's auth health; on "ok" it also
// stamps last_sync.
func (b *Broker) recordSourceHealth(ctx context.Context, dsID int64, expiry, health, errMsg string) {
	_ = b.store.UpdateDataSourceHealth(ctx, dbgen.UpdateDataSourceHealthParams{
		AccessExpiry: expiry, LastError: errMsg, Health: health, ID: dsID,
	})
	if health == "ok" {
		_ = b.store.MarkDataSourceSynced(ctx, dsID)
	}
}

// expiryStr formats an access token's expiry as RFC3339 (empty if unset).
func expiryStr(tok *oauth2.Token) string {
	if tok == nil || tok.Expiry.IsZero() {
		return ""
	}
	return tok.Expiry.UTC().Format(time.RFC3339)
}

func (b *Broker) markErr(ctx context.Context, id int64, msg string) {
	if _, err := b.store.GetWidgetCache(ctx, id); err == nil {
		// keep last-good, just flag it stale
		_ = b.store.MarkWidgetCacheStale(ctx, dbgen.MarkWidgetCacheStaleParams{ErrorMsg: msg, WidgetID: id})
	} else {
		_ = b.store.UpsertWidgetCache(ctx, dbgen.UpsertWidgetCacheParams{
			WidgetID:  id,
			DataJson:  "null",
			ExpiresAt: b.now().UTC().Add(b.interval).Format(tsLayout),
			Status:    "error",
			ErrorMsg:  msg,
		})
	}
	slog.Warn("broker: widget fetch failed", "widget", id, "err", msg)
}
