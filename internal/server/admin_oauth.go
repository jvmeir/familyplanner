package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"

	"github.com/jvmeir/familyplanner/internal/auth"
	"github.com/jvmeir/familyplanner/internal/crypto"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/oauth"
	"github.com/jvmeir/familyplanner/internal/widget"
)

const oauthCallbackPath = "/admin/datasources/oauth/callback"

// tokenFromSecret decrypts a data source's stored OAuth token.
func (s *Server) tokenFromSecret(ds dbgen.DataSource) *oauth2.Token {
	if ds.SecretCiphertext == "" {
		return nil
	}
	pt, err := crypto.Open(ds.SecretCiphertext, s.cfg.EncryptionKey)
	if err != nil {
		return nil
	}
	var sec struct {
		Token *oauth2.Token `json:"token"`
	}
	_ = json.Unmarshal(pt, &sec)
	return sec.Token
}

func (s *Server) storeToken(ctx context.Context, id int64, tok *oauth2.Token) error {
	ns, _ := json.Marshal(map[string]any{"token": tok})
	cipher, err := crypto.Seal(ns, s.cfg.EncryptionKey)
	if err != nil {
		return err
	}
	return s.store.UpdateDataSourceSecret(ctx, dbgen.UpdateDataSourceSecretParams{
		SecretCiphertext: cipher, OauthStatus: "connected", ID: id,
	})
}

func (s *Server) oauthConfig(dsType, redirect string) (*oauth2.Config, bool) {
	clientID, clientSecret := s.cfg.OAuthApp(dsType)
	if clientID == "" {
		return nil, false
	}
	return oauth.Config(dsType, clientID, clientSecret, redirect)
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ds, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cfg, ok := s.oauthConfig(ds.Type, s.cfg.BaseURL+oauthCallbackPath)
	if !ok {
		http.Error(w, "OAuth app credentials not configured for this provider", http.StatusBadRequest)
		return
	}
	state, _ := auth.NewToken()
	s.sessions.Put(r.Context(), "oauth_state", state)
	s.sessions.Put(r.Context(), "oauth_ds", strconv.FormatInt(id, 10))
	http.Redirect(w, r, cfg.AuthCodeURL(state, oauth.AuthOptions(ds.Type)...), http.StatusSeeOther)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" || state != s.sessions.GetString(r.Context(), "oauth_state") {
		http.Error(w, "bad oauth state", http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(s.sessions.GetString(r.Context(), "oauth_ds"), 10, 64)
	ds, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cfg, ok := s.oauthConfig(ds.Type, s.cfg.BaseURL+oauthCallbackPath)
	if !ok {
		http.Error(w, "OAuth app credentials not configured", http.StatusBadRequest)
		return
	}
	tok, err := cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	if err := s.storeToken(r.Context(), id, tok); err != nil {
		http.Error(w, "seal failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/datasources", http.StatusSeeOther)
}

// freshAccessToken returns a valid access token, refreshing + persisting rotations.
func (s *Server) freshAccessToken(ctx context.Context, ds dbgen.DataSource) (string, error) {
	tok := s.tokenFromSecret(ds)
	if tok == nil {
		return "", errors.New("not connected")
	}
	clientID, clientSecret := s.cfg.OAuthApp(ds.Type)
	fresh, err := oauth.FreshToken(ctx, ds.Type, clientID, clientSecret, tok)
	if err != nil {
		return "", err
	}
	if fresh.AccessToken != tok.AccessToken {
		_ = s.storeToken(ctx, ds.ID, fresh)
	}
	return fresh.AccessToken, nil
}

// listResources fetches selectable resources for a data source from its API.
func (s *Server) listResources(ctx context.Context, ds dbgen.DataSource) ([]widget.ResourceOption, error) {
	typ, _ := s.dsRegistry.Get(ds.Type)
	switch typ.ResourceKind {
	case "ms_calendars":
		tok, err := s.freshAccessToken(ctx, ds)
		if err != nil {
			return nil, err
		}
		return widget.GraphListCalendars(ctx, tok)
	case "google_albums":
		tok, err := s.freshAccessToken(ctx, ds)
		if err != nil {
			return nil, err
		}
		return widget.GoogleListAlbums(ctx, tok)
	case "onedrive_folders":
		tok, err := s.freshAccessToken(ctx, ds)
		if err != nil {
			return nil, err
		}
		// Offer both folders and photo albums; both resolve to a driveItem id
		// whose children are the photos. Albums are labelled distinctly.
		opts, err := widget.GraphListFolders(ctx, tok)
		if err != nil {
			return nil, err
		}
		if albums, aerr := widget.GraphListAlbums(ctx, tok); aerr != nil {
			// Albums (bundles) are a personal-OneDrive feature; OneDrive for
			// Business returns an error here. Log it so "no albums" is diagnosable.
			slog.Warn("onedrive: list albums failed (likely OneDrive for Business, which has no albums)", "source", ds.ID, "err", aerr)
		} else {
			for _, a := range albums {
				opts = append(opts, widget.ResourceOption{ID: a.ID, Name: "📷 " + a.Name})
			}
		}
		return opts, nil
	case "ms_todo_lists":
		tok, err := s.freshAccessToken(ctx, ds)
		if err != nil {
			return nil, err
		}
		lists, err := widget.GraphListTodoLists(ctx, tok)
		if err != nil {
			return nil, err
		}
		// Offer aggregating across every list (stored per widget↔source link).
		return append([]widget.ResourceOption{{ID: widget.TodoAllLists, Name: "★ Alle lijsten"}}, lists...), nil
	case "bring_lists":
		if ds.SecretCiphertext == "" {
			return nil, errors.New("no credentials")
		}
		pt, err := crypto.Open(ds.SecretCiphertext, s.cfg.EncryptionKey)
		if err != nil {
			return nil, err
		}
		var sec struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		_ = json.Unmarshal(pt, &sec)
		return widget.BringLists(ctx, sec.Email, sec.Password)
	}
	return nil, nil
}
