// Package server wires HTTP routing, auth/session middleware, the kiosk SSE
// stream, and first-run bootstrap (passphrase + demo seed).
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jvmeir/familyplanner/internal/auth"
	"github.com/jvmeir/familyplanner/internal/broker"
	"github.com/jvmeir/familyplanner/internal/config"
	"github.com/jvmeir/familyplanner/internal/datasource"
	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/layout"
	"github.com/jvmeir/familyplanner/internal/rotation"
	"github.com/jvmeir/familyplanner/internal/theme"
	"github.com/jvmeir/familyplanner/internal/voiceclock"
	"github.com/jvmeir/familyplanner/internal/web"
	"github.com/jvmeir/familyplanner/internal/widget"
)

// Server holds dependencies and the built HTTP handler.
type Server struct {
	cfg        *config.Config
	store      *db.Store
	registry   *widget.Registry
	i18n       *i18n.Service
	sessions   *scs.SessionManager
	rotation   *rotation.Manager
	brk        *broker.Broker
	dsRegistry *datasource.Registry
	now        func() time.Time
	bootID     string // changes each server start; kiosks auto-reload when it changes
	handler    http.Handler

	refMu       sync.Mutex          // guards refreshing + lastRefresh
	refreshing  map[int64]bool      // widget IDs with an in-flight background refresh (de-dupe)
	lastRefresh map[int64]time.Time // widget ID → last on-show refresh (throttle)

	photoMu    sync.Mutex      // guards photoOrder + photoPos
	photoOrder map[int64][]int // photos widget ID → shuffled index order (no-repeat slideshow)
	photoPos   map[int64]int   // photos widget ID → next position in photoOrder

	authLimiter *rateLimiter // throttles passphrase attempts on /login + /pair
}

// onShowRefreshThrottle is the minimum gap between background refreshes triggered
// by a widget being shown; rapid re-renders (30s in-place refresh, quick
// next/prev) within this window reuse the cache instead of stampeding a source.
const onShowRefreshThrottle = 10 * time.Second

// New builds a Server: configures sessions, runs first-run bootstrap, wires routes.
func New(cfg *config.Config, store *db.Store, reg *widget.Registry, i18nSvc *i18n.Service) (*Server, error) {
	sm := scs.New()
	sm.Lifetime = cfg.SessionTTL
	sm.Store = sqlite3store.New(store.DB)
	sm.Cookie.Name = "fp_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.Env == "prod"

	s := &Server{
		cfg:      cfg,
		store:    store,
		registry: reg,
		i18n:     i18nSvc,
		sessions: sm,
		rotation: rotation.NewManager(),
		now:      func() time.Time { return time.Now().In(cfg.TimeZone) },
		bootID:   strconv.FormatInt(time.Now().UnixNano(), 10),
		// 10 passphrase attempts per minute per IP, then 429 until the bucket refills.
		authLimiter: newRateLimiter(10, time.Minute, func() time.Time { return time.Now() }),
	}
	s.brk = broker.New(store, reg, s.now, cfg.EncryptionKey, cfg.OAuthApp)
	s.dsRegistry = datasource.NewRegistry()
	datasource.RegisterDefaults(s.dsRegistry)
	if err := s.bootstrap(context.Background()); err != nil {
		return nil, err
	}
	s.handler = s.routes()
	return s, nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// StartBackground launches the cache-refresh broker until ctx is cancelled.
func (s *Server) StartBackground(ctx context.Context) { go s.brk.Start(ctx) }

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(web.Assets()))))

	// Session-backed routes (admin + auth + pairing).
	r.Group(func(r chi.Router) {
		r.Use(s.sessions.LoadAndSave)
		r.Use(s.localeMiddleware)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
		})
		r.Get("/login", func(w http.ResponseWriter, r *http.Request) { s.render(w, r, web.Login("")) })
		r.With(s.authLimiter.limit).Post("/login", s.handleLoginPost)
		r.Get("/pair", func(w http.ResponseWriter, r *http.Request) {
			s.render(w, r, web.Pair("", s.playlistRefs(r.Context())))
		})
		r.With(s.authLimiter.limit).Post("/pair", s.handlePairPost)

		r.Group(func(r chi.Router) {
			r.Use(s.requireLogin)
			r.Use(s.csrfMiddleware)
			r.Post("/logout", s.handleLogout)
			r.Get("/admin", s.handleAdmin)
			r.Get("/admin/views", s.handleViews)
			r.Post("/admin/views", s.handleViewCreate)
			r.Post("/admin/views/{id}/name", s.handleViewRename)
			r.Post("/admin/views/{id}/mode", s.handleViewMode)
			r.Post("/admin/views/{id}/advance", s.handleViewAdvance)
			r.Delete("/admin/views/{id}", s.handleViewDelete)
			r.Get("/admin/views/{id}/layout", s.handleViewLayout)
			r.Get("/admin/views/{id}/preview", s.handleViewPreview)
			r.Post("/admin/views/{id}/layout/split", s.handleLayoutSplit)
			r.Post("/admin/views/{id}/layout/remove", s.handleLayoutRemove)
			r.Post("/admin/views/{id}/layout/widget", s.handleLayoutSetWidget)
			r.Post("/admin/views/{id}/layout/weights", s.handleLayoutWeights)
			r.Get("/admin/widgets", s.handleWidgets)
			r.Post("/admin/widgets", s.handleWidgetCreate)
			r.Get("/admin/widgets/fields", s.handleWidgetFields)
			r.Get("/admin/widgets/{id}/edit", s.handleWidgetEdit)
			r.Get("/admin/widgets/{id}/preview", s.handleWidgetPreview)
			r.Post("/admin/widgets/{id}", s.handleWidgetUpdate)
			r.Delete("/admin/widgets/{id}", s.handleWidgetDelete)

			r.Get("/admin/playlists", s.handlePlaylists)
			r.Post("/admin/playlists", s.handlePlaylistCreate)
			r.Delete("/admin/playlists/{id}", s.handlePlaylistDelete)
			r.Post("/admin/playlists/{id}/default", s.handlePlaylistSetDefault)
			r.Get("/admin/playlists/{id}", s.handlePlaylistDetail)
			r.Post("/admin/playlists/{id}", s.handlePlaylistUpdate)
			r.Post("/admin/playlists/{id}/pip", s.handlePlaylistPip)
			r.Post("/admin/playlists/{id}/items", s.handlePlaylistAddItem)
			r.Delete("/admin/playlists/items/{itemID}", s.handlePlaylistItemDelete)
			r.Post("/admin/playlists/items/{itemID}/move", s.handlePlaylistItemMove)

			r.Get("/admin/settings", s.handleSettings)
			r.Post("/admin/settings", s.handleSettingsSave)

			r.Get("/admin/devices", s.handleDevices)
			r.Post("/admin/devices/{id}/name", s.handleDeviceRename)
			r.Delete("/admin/devices/{id}", s.handleDeviceDelete)
			r.Post("/admin/devices/{id}/playlist", s.handleDeviceAssign)
			r.Post("/admin/devices/{id}/control/{cmd}", s.handleDeviceControl)

			r.Get("/admin/datasources", s.handleDataSources)
			r.Post("/admin/datasources/{id}/name", s.handleDataSourceRename)
			r.Get("/admin/datasources/fields", s.handleDataSourceFields)
			r.Get("/admin/datasources/oauth/callback", s.handleOAuthCallback)
			r.Post("/admin/datasources", s.handleDataSourceCreate)
			r.Get("/admin/datasources/{id}/edit", s.handleDataSourceEdit)
			r.Post("/admin/datasources/{id}", s.handleDataSourceUpdate)
			r.Get("/admin/datasources/{id}/oauth/start", s.handleOAuthStart)
			r.Delete("/admin/datasources/{id}", s.handleDataSourceDelete)
			r.Post("/admin/widgets/{id}/sources", s.handleWidgetSourceAdd)
			r.Post("/admin/widgets/{id}/sources/{linkID}/resource", s.handleWidgetSourceResource)
			r.Post("/admin/widgets/{id}/sources/{linkID}/color", s.handleWidgetSourceColor)
			r.Delete("/admin/widgets/{id}/sources/{linkID}", s.handleWidgetSourceDelete)
		})
	})

	// Kiosk routes: device-cookie auth, no session buffering so SSE can stream.
	r.Group(func(r chi.Router) {
		r.Use(s.localeMiddleware)
		r.Use(s.requireKiosk)
		r.Get("/kiosk", s.handleKiosk)
		r.Get("/kiosk/view/{id}", s.handleKioskView)
		r.Get("/kiosk/ticker", s.handleKioskTicker)
		r.Get("/kiosk/stream", s.handleKioskStream)
		r.Post("/kiosk/control/{cmd}", s.handleKioskControl)
	})

	return r
}

// ---------- middleware ----------

func (s *Server) localeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// M0: always the default locale. Later: cookie / Accept-Language / setting.
		loc := s.i18n.Loc(s.cfg.DefaultLocale)
		next.ServeHTTP(w, r.WithContext(i18n.WithLoc(r.Context(), loc)))
	})
}

func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.sessions.GetBool(r.Context(), "authed") {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireKiosk(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("fp_kiosk")
		if err != nil || c.Value == "" {
			http.Redirect(w, r, "/pair", http.StatusSeeOther)
			return
		}
		dev, err := s.store.GetDeviceByTokenHash(r.Context(), auth.HashToken(c.Value))
		if err != nil {
			http.Redirect(w, r, "/pair", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), deviceCtxKey{}, dev)
		_ = s.store.TouchDevice(ctx, dbgen.TouchDeviceParams{
			LastSeen: s.now().Format(time.RFC3339), ID: dev.ID,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type deviceCtxKey struct{}

func deviceFrom(ctx context.Context) (dbgen.KioskDevice, bool) {
	d, ok := ctx.Value(deviceCtxKey{}).(dbgen.KioskDevice)
	return d, ok
}

// ---------- auth handlers ----------

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if s.checkPassphrase(r.Context(), r.FormValue("passphrase")) {
		_ = s.sessions.RenewToken(r.Context())
		s.sessions.Put(r.Context(), "authed", true)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	s.render(w, r, web.Login(i18n.T(r.Context(), "login.error")))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.sessions.Destroy(r.Context())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handlePairPost(w http.ResponseWriter, r *http.Request) {
	if !s.checkPassphrase(r.Context(), r.FormValue("passphrase")) {
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, r, web.Pair(i18n.T(r.Context(), "login.error"), s.playlistRefs(r.Context())))
		return
	}
	token, err := auth.NewToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	dev, err := s.store.CreateDevice(r.Context(), dbgen.CreateDeviceParams{
		Name: "kiosk", TokenHash: auth.HashToken(token),
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Assign the playlist chosen at pairing (0 = default playlist).
	if plID, _ := strconv.ParseInt(r.FormValue("playlist_id"), 10, 64); plID > 0 {
		_ = s.store.SetDevicePlaylist(r.Context(), dbgen.SetDevicePlaylistParams{PlaylistID: plID, ID: dev.ID})
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "fp_kiosk",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().AddDate(10, 0, 0), // effectively permanent
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Env == "prod",
	})
	http.Redirect(w, r, "/kiosk", http.StatusSeeOther)
}

func (s *Server) checkPassphrase(ctx context.Context, passphrase string) bool {
	hash, err := s.store.GetSetting(ctx, "passphrase_hash")
	if err != nil || hash == "" {
		return false
	}
	ok, err := auth.VerifyPassphrase(passphrase, hash)
	return err == nil && ok
}

// ---------- kiosk handlers ----------

func (s *Server) handleKiosk(w http.ResponseWriter, r *http.Request) {
	dev, _ := deviceFrom(r.Context())
	health := s.healthVM(r.Context())
	ticker := s.tickerItems(r.Context())
	pip := s.pipVM(r.Context(), dev)
	view, err := s.currentPlaylistView(r.Context(), dev)
	if err != nil {
		s.render(w, r, web.Kiosk(web.Grid("", nil), web.ControlsVM{}, health, ticker, pip))
		return
	}
	body := s.renderViewComponent(r.Context(), view)
	s.render(w, r, web.Kiosk(body, s.buildControls(r.Context(), dev, view.ID), health, ticker, pip))
}

// pipVM resolves the device's playlist corner-PiP: the configured video widget's
// cached video ids plus its presentation. Returns nil when no PiP is set.
func (s *Server) pipVM(ctx context.Context, dev dbgen.KioskDevice) *web.PipVM {
	pl, ok := s.resolvePlaylist(ctx, dev)
	if !ok || pl.PipWidgetID == 0 {
		return nil
	}
	cache, err := s.store.GetWidgetCache(ctx, pl.PipWidgetID)
	if err != nil {
		if wgt, gerr := s.store.GetWidget(ctx, pl.PipWidgetID); gerr == nil {
			s.bgRefresh(wgt)
		}
		return nil
	}
	typ, ok := s.registry.Get("video")
	if !ok || typ.Decode == nil {
		return nil
	}
	d, derr := typ.Decode(json.RawMessage(cache.DataJson))
	if derr != nil {
		return nil
	}
	vd, ok := d.(widget.VideoData)
	if !ok || len(vd.IDs) == 0 {
		return nil
	}
	pc := parsePipConfig(pl.PipConfigJson)
	return &web.PipVM{IDs: vd.IDs, Corner: pc.Corner, Size: pc.Size, Muted: pc.Muted, Interval: pc.Interval}
}

// tickerItems resolves the configured global ticker widget's cached items (empty
// if none configured). Read-only from cache, like the rest of the kiosk.
func (s *Server) tickerItems(ctx context.Context) []string {
	raw, err := s.store.GetSetting(ctx, "ticker_widget_id")
	if err != nil || raw == "" {
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id == 0 {
		return nil
	}
	cache, err := s.store.GetWidgetCache(ctx, id)
	if err != nil {
		if wgt, gerr := s.store.GetWidget(ctx, id); gerr == nil {
			s.brk.RefreshWidget(ctx, wgt)
			cache, err = s.store.GetWidgetCache(ctx, id)
		}
		if err != nil {
			return nil
		}
	}
	typ, ok := s.registry.Get("ticker")
	if !ok || typ.Decode == nil {
		return nil
	}
	d, derr := typ.Decode(json.RawMessage(cache.DataJson))
	if derr != nil {
		return nil
	}
	if td, ok := d.(widget.TickerData); ok {
		return td.Items
	}
	return nil
}

// handleKioskTicker returns just the ticker track fragment, so the kiosk can
// refresh the ticker (which lives in the persistent bar) without a full reload.
func (s *Server) handleKioskTicker(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.TickerTrack(s.tickerItems(r.Context())))
}

// healthVM maps the health summary into the kiosk badge view-model (empty when
// everything is healthy, so the badge stays hidden).
func (s *Server) healthVM(ctx context.Context) web.HealthVM {
	sum := s.buildHealth(ctx)
	vm := web.HealthVM{Level: string(sum.Level), Count: sum.Count}
	if len(sum.Issues) > 0 {
		vm.Message = sum.Issues[0].Message
	}
	return vm
}

func (s *Server) handleKioskView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad view id", http.StatusBadRequest)
		return
	}
	view, err := s.store.GetView(r.Context(), id)
	if err != nil {
		http.Error(w, "view not found", http.StatusNotFound)
		return
	}
	// Wrap with the health badge so it persists across SSE view swaps and
	// refreshes with the latest health on each tick.
	s.render(w, r, web.KioskBody(s.renderViewComponent(r.Context(), view), s.healthVM(r.Context())))
}

func (s *Server) handleKioskControl(w http.ResponseWriter, r *http.Request) {
	dev, ok := deviceFrom(r.Context())
	if !ok {
		http.Error(w, "no device", http.StatusUnauthorized)
		return
	}
	switch cmd := chi.URLParam(r, "cmd"); cmd {
	case "goto":
		viewID, _ := strconv.ParseInt(r.URL.Query().Get("view"), 10, 64)
		s.rotation.Goto(dev.ID, viewID)
	case "next", "prev", "pause", "resume":
		s.rotation.Command(dev.ID, rotation.Command(cmd))
	default:
		http.Error(w, "bad command", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKioskStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	dev, ok := deviceFrom(r.Context())
	if !ok {
		http.Error(w, "no device", http.StatusUnauthorized)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	state, notify, release := s.rotation.Connect(dev.ID, s.deviceItems(r.Context(), dev))
	defer release()

	send := func(event, data string) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	// displayDwell is the configured dwell (never the on_end safety cap); it's what
	// the client sees, drives the progress bar, and uses as the on_end fallback.
	displayDwell := func() time.Duration {
		if it, ok := state.Current(); ok && it.Dwell > 0 {
			return it.Dwell
		}
		return 30 * time.Second
	}
	dwell := func() time.Duration {
		it, ok := state.Current()
		if !ok {
			return 30 * time.Second
		}
		// "Advance on end" screens with an end-capable widget (video) suspend the
		// server timer — the CLIENT advances (on video-end, or after the display
		// dwell when the rendered widget isn't a video, e.g. random-single). A
		// generous safety cap still advances if the client never reports back.
		if v, err := s.store.GetView(r.Context(), it.ViewID); err == nil &&
			v.AdvanceMode == "on_end" && s.viewHasEndWidget(r.Context(), v) {
			return 30 * time.Minute
		}
		return displayDwell()
	}
	sendCurrent := func() bool {
		it, ok := state.Current()
		if !ok {
			return send("refresh", "empty")
		}
		if !send("navigate", strconv.FormatInt(it.ViewID, 10)) {
			return false
		}
		secs := 0 // 0 = paused / no countdown
		if !state.Paused() {
			secs = int(displayDwell().Seconds())
		}
		return send("dwell", strconv.Itoa(secs))
	}
	sendConfig := func() bool {
		b, _ := json.Marshal(map[string]any{
			"scale":      s.kioskScale(r.Context()),
			"tickerSecs": s.tickerSpeed(r.Context()),
			"dateFmt":    s.bannerDate(r.Context()),
			"transition": s.kioskTransition(r.Context()),
		})
		return send("config", string(b))
	}
	// sendNames pushes the id->name map for ALL views so the banner can always
	// label the current screen, even ones created after the kiosk connected.
	sendNames := func() bool {
		names := map[string]string{}
		if all, err := s.store.ListViews(r.Context()); err == nil {
			for _, v := range all {
				names[strconv.FormatInt(v.ID, 10)] = v.Name
			}
		}
		b, _ := json.Marshal(names)
		return send("names", string(b))
	}
	reset := func(t *time.Timer, d time.Duration) {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(d)
	}

	if !sendCurrent() {
		return
	}
	sendConfig()              // push scale + ticker speed + date format on connect
	sendNames()               // push the view-name map (banner labels)
	send("version", s.bootID) // kiosks reload when this changes (i.e. after a redeploy)
	advance := time.NewTimer(dwell())
	defer advance.Stop()
	refresh := time.NewTicker(30 * time.Second)
	defer refresh.Stop()
	// Global voice clock: fire on each quarter-hour so every screen chimes in
	// sync. beat is the boundary the pending timer is aiming at; the timer fires
	// slightly early when that beat's sound has a marking tone (speaking-clock
	// pips) so the tone lands exactly on the boundary.
	beat := voiceclock.NextBoundary(s.now())
	chime := time.NewTimer(s.chimeDelay(r.Context(), s.now(), beat))
	defer chime.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-notify: // a control command mutated this device's state
			if !sendCurrent() {
				return
			}
			reset(advance, dwell())
		case <-refresh.C: // periodic in-view data refresh (e.g. the clock) + scale sync
			state.SetItems(s.deviceItems(r.Context(), dev)) // pick up playlist edits live
			if !send("refresh", "tick") {
				return
			}
			sendConfig()
			sendNames() // keep banner labels current as screens are added/renamed
		case <-advance.C: // dwell elapsed -> advance unless paused
			if !state.Paused() {
				state.Next()
			}
			if !sendCurrent() {
				return
			}
			reset(advance, dwell())
		case <-chime.C: // quarter-hour voice-clock chime (gated by config + quiet hours)
			if ch, ok := s.voiceClockConfig(r.Context()).Decide(beat); ok {
				ch.AtUnixMs = beat.UnixMilli() // let the client align the marking pip to the exact beat
				if payload, err := json.Marshal(ch); err == nil {
					if !send("chime", string(payload)) {
						return
					}
				}
			}
			beat = voiceclock.NextBoundary(beat) // the boundary strictly after this one
			chime.Reset(s.chimeDelay(r.Context(), s.now(), beat))
		}
	}
}

// chimeDelay is how long to wait before firing the chime timer for the given
// beat: the time until the beat, minus the chime's lead so a spoken readout has
// time to finish and its marking pip can land on the beat. Never negative.
func (s *Server) chimeDelay(ctx context.Context, now, beat time.Time) time.Duration {
	d := beat.Sub(now)
	if ch, ok := s.voiceClockConfig(ctx).Decide(beat); ok {
		d -= ch.Lead()
	}
	if d < 0 {
		d = 0
	}
	return d
}

// voiceClockConfig loads the global voice-clock setting (seeded default if unset).
func (s *Server) voiceClockConfig(ctx context.Context) voiceclock.Config {
	raw, err := s.store.GetSetting(ctx, "voiceclock")
	if err != nil || raw == "" {
		return voiceclock.Default()
	}
	var cfg voiceclock.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return voiceclock.Default()
	}
	return cfg
}

// ---------- view resolution ----------

// renderViewComponent renders a view's body: the recursive layout tree if the
// view has one, otherwise the legacy fixed grid.
func (s *Server) renderViewComponent(ctx context.Context, view dbgen.View) templ.Component {
	// The client owns advancement only when the server actually suspends its timer
	// (on_end AND the view has a video). Then the client advances on video-end, or
	// after the dwell when the rendered widget isn't a video (random-single).
	onEnd := view.AdvanceMode == "on_end" && s.viewHasEndWidget(ctx, view)
	// Random-single mode: show one of the screen's widgets, chosen at random each
	// time it's rendered; the grid/split layout is ignored.
	if view.RenderMode == "random_single" {
		if ids := s.viewWidgetIDs(ctx, view); len(ids) > 0 {
			wid := ids[rand.IntN(len(ids))]
			th := theme.Resolve(s.defaultTheme(ctx), theme.DefaultID)
			cell := s.cellForWidget(ctx, wid, "")
			return web.PreviewWidget(web.ThemeVars(th), cell, onEnd)
		}
	}
	if lm, th, ok := s.buildViewVM(ctx, view); ok {
		return web.View(web.ThemeVars(th), lm, onEnd)
	}
	gs, cells := s.renderLegacyGrid(ctx, view)
	return web.Grid(gs, cells)
}

// viewHasEndWidget reports whether a view contains a widget that emits an end
// event (currently: a video), so its rotation timer can be suspended in on_end.
func (s *Server) viewHasEndWidget(ctx context.Context, view dbgen.View) bool {
	for _, id := range s.viewWidgetIDs(ctx, view) {
		if w, err := s.store.GetWidget(ctx, id); err == nil && w.Type == "video" {
			return true
		}
	}
	return false
}

// viewWidgetIDs returns the widget ids assigned to a view (from its layout tree,
// or legacy placements), used to pick one at random in random-single mode.
func (s *Server) viewWidgetIDs(ctx context.Context, view dbgen.View) []int64 {
	var ids []int64
	if view.LayoutJson != "" {
		if root, err := layout.Parse(view.LayoutJson); err == nil {
			var walk func(layout.Node)
			walk = func(n layout.Node) {
				if n.Leaf != nil && n.Leaf.WidgetID != 0 {
					ids = append(ids, n.Leaf.WidgetID)
				}
				if n.Split != nil {
					for _, c := range n.Split.Children {
						walk(c.Node)
					}
				}
			}
			walk(root)
			return ids
		}
	}
	if pls, err := s.store.ListPlacementsByView(ctx, view.ID); err == nil {
		for _, p := range pls {
			if p.WidgetID != 0 {
				ids = append(ids, p.WidgetID)
			}
		}
	}
	return ids
}

// buildViewVM resolves a view's recursive layout into a render-ready LayoutVM
// plus its theme. ok is false when the view has no layout tree (legacy grid);
// callers then fall back to renderLegacyGrid. Shared by the HTML kiosk and the
// JSON kiosk API so both render identical data.
func (s *Server) buildViewVM(ctx context.Context, view dbgen.View) (web.LayoutVM, theme.Theme, bool) {
	if view.LayoutJson == "" {
		return web.LayoutVM{}, theme.Theme{}, false
	}
	root, err := layout.Parse(view.LayoutJson)
	if err != nil {
		slog.Error("parse layout", "view", view.ID, "err", err)
		return web.LayoutVM{}, theme.Theme{}, false
	}
	th := theme.Resolve(s.defaultTheme(ctx), theme.DefaultID) // theme is a global setting
	return s.buildLayoutVM(ctx, root), th, true
}

// buildLayoutVM walks the layout tree, fetching each leaf widget's data.
func (s *Server) buildLayoutVM(ctx context.Context, node layout.Node) web.LayoutVM {
	if node.Leaf != nil {
		cell := s.cellForWidget(ctx, node.Leaf.WidgetID, "")
		return web.LayoutVM{Cell: &cell}
	}
	if node.Split != nil {
		vm := web.LayoutVM{Dir: string(node.Split.Dir)}
		for _, c := range node.Split.Children {
			vm.Children = append(vm.Children, web.LayoutChildVM{
				Weight: c.Weight,
				Node:   s.buildLayoutVM(ctx, c.Node),
			})
		}
		return vm
	}
	return web.LayoutVM{}
}

// renderLegacyGrid builds the fixed cols×rows grid from placements.
func (s *Server) renderLegacyGrid(ctx context.Context, view dbgen.View) (templ.SafeCSS, []web.CellVM) {
	placements, err := s.store.ListPlacementsByView(ctx, view.ID)
	if err != nil {
		slog.Error("list placements", "err", err)
		return "", nil
	}
	cells := make([]web.CellVM, 0, len(placements))
	for _, p := range placements {
		cells = append(cells, s.cellForWidget(ctx, p.WidgetID, web.CellStyle(p)))
	}
	th := theme.Resolve(s.defaultTheme(ctx), theme.DefaultID) // theme is a global setting
	return web.GridStyle(view, th), cells
}

// cellForWidget fetches a widget's data and formats it into a cell view-model.
// WidgetID 0 is an empty (unassigned) pane. The kiosk read path never calls
// external services; widgets compute or read cache only.
func (s *Server) cellForWidget(ctx context.Context, widgetID int64, style templ.SafeCSS) web.CellVM {
	if widgetID == 0 {
		return web.CellVM{Kind: "empty", Style: style}
	}
	wgt, err := s.store.GetWidget(ctx, widgetID)
	if err != nil {
		return web.FormatCell(ctx, "missing", nil, true, style)
	}
	typ, ok := s.registry.Get(wgt.Type)
	if !ok {
		return web.FormatCell(ctx, wgt.Type, nil, true, style)
	}

	// Lazy on-show refresh, done in the BACKGROUND so rendering never blocks on a
	// slow/failing feed (a synchronous fetch here stalled the /kiosk/view fragment
	// during rotation — the calendar "failed to load"). We render whatever is
	// cached immediately; the background fetch makes the next tick fresh.
	cache, err := s.store.GetWidgetCache(ctx, widgetID)
	if err != nil {
		// Never fetched yet: do a bounded synchronous fetch so the first paint
		// isn't empty, but cap it (2s) so a slow feed can't stall rotation for
		// long; if it doesn't finish, show a placeholder and keep trying in bg.
		fctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		s.brk.RefreshWidget(fctx, wgt)
		cancel()
		if cache, err = s.store.GetWidgetCache(ctx, widgetID); err != nil {
			s.bgRefresh(wgt)
			return web.FormatCell(ctx, wgt.Type, nil, false, style)
		}
	} else {
		// Refresh the data source on EVERY show so the widget is as fresh as
		// possible: render the current cache immediately, kick a background fetch
		// for next tick. bgRefresh throttles per-widget so rapid re-renders (the
		// 30s in-place refresh, quick next/prev) can't stampede a slow source.
		s.bgRefresh(wgt)
	}

	var data any
	stale := cache.Status != "ok"
	if typ.Decode != nil {
		if d, derr := typ.Decode(json.RawMessage(cache.DataJson)); derr == nil {
			data = d
		} else {
			stale = true
		}
	}
	// Photo slideshow: each time a photos widget is shown, advance to the next
	// photo in a per-widget shuffled order so no picture repeats until the whole
	// album has been shown once.
	if pd, ok := data.(widget.PhotosData); ok && len(pd.URLs) > 0 {
		data = widget.PhotosData{URLs: []string{s.nextPhoto(widgetID, pd.URLs)}, Mode: "single"}
	}
	vm := web.FormatCell(ctx, wgt.Type, data, stale, style)
	// Standardized widget title: every widget shows its own name as the header,
	// uniformly, unless the generic per-widget "hide title" flag (config_json
	// hide_title) is set. Full-bleed media (web page / photo) has no title bar.
	var meta struct {
		HideTitle  string `json:"hide_title"`
		TitleSize  string `json:"title_size"`
		TitleAlign string `json:"title_align"`
	}
	_ = json.Unmarshal([]byte(wgt.ConfigJson), &meta)
	switch {
	case meta.HideTitle == "1":
		vm.Title = ""
	case vm.IframeURL != "" || vm.ImageURL != "" || len(vm.VideoIDs) > 0:
		// full-bleed media: no title bar
		vm.Title = ""
	default:
		vm.Title = wgt.Name
	}
	vm.TitleSize = pick(meta.TitleSize, "small", "medium", "large")  // default small
	vm.TitleAlign = pick(meta.TitleAlign, "left", "center", "right") // default left
	return vm
}

// pick returns v if it's one of the allowed values, else the first (default).
func pick(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return allowed[0]
}

// nextPhoto returns the next photo URL for a widget's slideshow, advancing a
// per-widget shuffled order so every picture in the album is shown once before
// any repeats. The order is reshuffled when the album changes or is exhausted.
func (s *Server) nextPhoto(widgetID int64, urls []string) string {
	n := len(urls)
	if n == 0 {
		return ""
	}
	s.photoMu.Lock()
	defer s.photoMu.Unlock()
	if s.photoOrder == nil {
		s.photoOrder = map[int64][]int{}
		s.photoPos = map[int64]int{}
	}
	order := s.photoOrder[widgetID]
	pos := s.photoPos[widgetID]
	if len(order) != n || pos >= len(order) {
		order = make([]int, n)
		for i := range order {
			order[i] = i
		}
		rand.Shuffle(n, func(i, j int) { order[i], order[j] = order[j], order[i] })
		pos = 0
	}
	idx := order[pos]
	s.photoOrder[widgetID] = order
	s.photoPos[widgetID] = pos + 1
	if idx < 0 || idx >= n {
		idx = 0
	}
	return urls[idx]
}

// bgRefresh fetches a widget's data in the background (detached context),
// de-duplicating concurrent refreshes of the same widget so a busy rotation
// can't stampede a slow source.
func (s *Server) bgRefresh(wgt dbgen.Widget) {
	s.refMu.Lock()
	if s.refreshing == nil {
		s.refreshing = map[int64]bool{}
	}
	if s.lastRefresh == nil {
		s.lastRefresh = map[int64]time.Time{}
	}
	if s.refreshing[wgt.ID] {
		s.refMu.Unlock()
		return
	}
	if t, ok := s.lastRefresh[wgt.ID]; ok && s.now().Sub(t) < onShowRefreshThrottle {
		s.refMu.Unlock()
		return // refreshed too recently; reuse cache
	}
	s.refreshing[wgt.ID] = true
	s.lastRefresh[wgt.ID] = s.now()
	s.refMu.Unlock()
	go func() {
		defer func() {
			s.refMu.Lock()
			delete(s.refreshing, wgt.ID)
			s.refMu.Unlock()
		}()
		s.brk.RefreshWidget(context.Background(), wgt)
	}()
}

// cacheExpired reports whether a widget_cache expiry (SQLite UTC timestamp
// "2006-01-02 15:04:05") is in the past. A blank/unparseable value is treated as
// expired so it gets refreshed.
func cacheExpired(expiresAt string, now time.Time) bool {
	t, err := time.Parse("2006-01-02 15:04:05", expiresAt)
	if err != nil {
		return true
	}
	return now.UTC().After(t)
}

// resolvePlaylist returns the device's assigned playlist, or the default.
func (s *Server) resolvePlaylist(ctx context.Context, dev dbgen.KioskDevice) (dbgen.Playlist, bool) {
	if dev.PlaylistID != 0 {
		if pl, err := s.store.GetPlaylist(ctx, dev.PlaylistID); err == nil {
			return pl, true
		}
	}
	if pl, err := s.store.GetDefaultPlaylist(ctx); err == nil {
		return pl, true
	}
	return dbgen.Playlist{}, false
}

// deviceItems resolves a device's playlist into rotation items, applying the
// per-item dwell override (0 = playlist default) with a small minimum.
func (s *Server) deviceItems(ctx context.Context, dev dbgen.KioskDevice) []rotation.Item {
	pl, ok := s.resolvePlaylist(ctx, dev)
	if !ok {
		return nil
	}
	rows, err := s.store.ListPlaylistItems(ctx, pl.ID)
	if err != nil {
		return nil
	}
	out := make([]rotation.Item, 0, len(rows))
	for _, it := range rows {
		d := time.Duration(it.DwellSeconds) * time.Second
		if d <= 0 {
			d = time.Duration(pl.DefaultDwellSeconds) * time.Second
		}
		if d < 3*time.Second {
			d = 3 * time.Second
		}
		out = append(out, rotation.Item{ViewID: it.ViewID, Dwell: d})
	}
	return out
}

func (s *Server) currentPlaylistView(ctx context.Context, dev dbgen.KioskDevice) (dbgen.View, error) {
	if items := s.deviceItems(ctx, dev); len(items) > 0 {
		if v, err := s.store.GetView(ctx, items[0].ViewID); err == nil {
			return v, nil
		}
	}
	return s.currentView(ctx)
}

func (s *Server) buildControls(ctx context.Context, dev dbgen.KioskDevice, currentID int64) web.ControlsVM {
	vm := web.ControlsVM{CurrentID: currentID}
	if pl, ok := s.resolvePlaylist(ctx, dev); ok {
		vm.PlaylistName = pl.Name
	}
	if items := s.deviceItems(ctx, dev); len(items) > 0 {
		for _, it := range items {
			if v, err := s.store.GetView(ctx, it.ViewID); err == nil {
				vm.Playlist = append(vm.Playlist, web.ViewRef{ID: v.ID, Name: v.Name})
			}
		}
	}
	if all, err := s.store.ListViews(ctx); err == nil {
		names := make(map[string]string, len(all))
		for _, v := range all {
			vm.All = append(vm.All, web.ViewRef{ID: v.ID, Name: v.Name})
			names[strconv.FormatInt(v.ID, 10)] = v.Name
		}
		if b, err := json.Marshal(names); err == nil {
			vm.NamesJSON = string(b)
		}
	}
	return vm
}

func (s *Server) currentView(ctx context.Context) (dbgen.View, error) {
	if rot, err := s.store.ListRotationViews(ctx); err == nil && len(rot) > 0 {
		return rot[0], nil
	}
	all, err := s.store.ListViews(ctx)
	if err != nil {
		return dbgen.View{}, err
	}
	if len(all) == 0 {
		return dbgen.View{}, errors.New("no views configured")
	}
	return all[0], nil
}

func (s *Server) defaultTheme(ctx context.Context) string {
	if v, err := s.store.GetSetting(ctx, "default_theme"); err == nil && v != "" {
		return v
	}
	return theme.DefaultID
}

// ---------- helpers ----------

func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("render", "err", err)
	}
}

// ---------- first-run bootstrap ----------

// cacheSchemaVersion identifies the shape of the JSON stored in widget_cache.
// Bump it whenever a widget's cached Data struct changes incompatibly so stale
// rows from an older build are dropped (and re-fetched) instead of mis-decoded.
const cacheSchemaVersion = "2" // v2: PhotosData album-only, CalendarEvent.Today

// invalidateStaleCache clears widget_cache when the running build's cache schema
// differs from what produced the stored rows.
func (s *Server) invalidateStaleCache(ctx context.Context) {
	cur, err := s.store.GetSetting(ctx, "cache_schema_version")
	if err == nil && cur == cacheSchemaVersion {
		return
	}
	if err := s.store.ClearWidgetCache(ctx); err != nil {
		slog.Error("clear widget cache on schema change", "err", err)
		return
	}
	_ = s.store.SetSetting(ctx, dbgen.SetSettingParams{Key: "cache_schema_version", Value: cacheSchemaVersion})
	if err != nil { // first run (no version stored): don't log a scary message
		return
	}
	slog.Info("widget cache cleared (schema version changed)", "from", cur, "to", cacheSchemaVersion)
}

func (s *Server) bootstrap(ctx context.Context) error {
	s.invalidateStaleCache(ctx)
	// Seed the admin passphrase from the bootstrap env var if none is stored yet.
	if _, err := s.store.GetSetting(ctx, "passphrase_hash"); errors.Is(err, sql.ErrNoRows) {
		if s.cfg.AdminPassphrase != "" {
			hash, herr := auth.HashPassphrase(s.cfg.AdminPassphrase)
			if herr != nil {
				return herr
			}
			if err := s.store.SetSetting(ctx, dbgen.SetSettingParams{Key: "passphrase_hash", Value: hash}); err != nil {
				return err
			}
			slog.Info("bootstrapped admin passphrase from FP_ADMIN_PASSPHRASE")
		} else {
			slog.Warn("no admin passphrase set; set FP_ADMIN_PASSPHRASE to enable login")
		}
	}

	if _, err := s.store.GetSetting(ctx, "default_theme"); errors.Is(err, sql.ErrNoRows) {
		if err := s.store.SetSetting(ctx, dbgen.SetSettingParams{Key: "default_theme", Value: theme.DefaultID}); err != nil {
			return err
		}
	}

	n, err := s.store.CountViews(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return s.seedDemo(ctx)
	}
	return nil
}

func (s *Server) seedDemo(ctx context.Context) error {
	view, err := s.store.CreateView(ctx, dbgen.CreateViewParams{
		Name: "Demo", Cols: 3, Rows: 2, ThemeID: theme.DefaultID,
		InRotation: 1, RotationOrder: 0, DwellSeconds: 15,
	})
	if err != nil {
		return err
	}

	countdown, err := s.store.CreateWidget(ctx, dbgen.CreateWidgetParams{
		Name: "Kerst", Type: "countdown", ConfigJson: `{"title":"Kerstmis","date":"2026-12-25"}`,
	})
	if err != nil {
		return err
	}
	clock, err := s.store.CreateWidget(ctx, dbgen.CreateWidgetParams{
		Name: "Klok", Type: "clock", ConfigJson: "{}",
	})
	if err != nil {
		return err
	}

	if _, err := s.store.CreatePlacement(ctx, dbgen.CreatePlacementParams{
		ViewID: view.ID, WidgetID: countdown.ID, Col: 1, Row: 1, ColSpan: 2, RowSpan: 2, PlacementConfigJson: "{}",
	}); err != nil {
		return err
	}
	if _, err := s.store.CreatePlacement(ctx, dbgen.CreatePlacementParams{
		ViewID: view.ID, WidgetID: clock.ID, Col: 3, Row: 1, ColSpan: 1, RowSpan: 2, PlacementConfigJson: "{}",
	}); err != nil {
		return err
	}

	// A second view that REUSES the same countdown widget full-screen — proves a
	// single configured widget can appear in multiple views.
	aftellen, err := s.store.CreateView(ctx, dbgen.CreateViewParams{
		Name: "Aftellen", Cols: 1, Rows: 1, ThemeID: "donker",
		InRotation: 1, RotationOrder: 1, DwellSeconds: 10,
	})
	if err != nil {
		return err
	}
	if _, err := s.store.CreatePlacement(ctx, dbgen.CreatePlacementParams{
		ViewID: aftellen.ID, WidgetID: countdown.ID, Col: 1, Row: 1, ColSpan: 1, RowSpan: 1, PlacementConfigJson: "{}",
	}); err != nil {
		return err
	}

	// Give the views recursive layout trees (exercises the M1c renderer):
	// Demo = countdown beside clock (2:1); Aftellen = a single full-screen countdown.
	demoLayout := layout.Node{Split: &layout.Split{Dir: layout.Row, Children: []layout.Child{
		{Weight: 2, Node: layout.SingleLeaf(countdown.ID)},
		{Weight: 1, Node: layout.SingleLeaf(clock.ID)},
	}}}
	if js, err := demoLayout.Marshal(); err == nil {
		_ = s.store.SetViewLayout(ctx, dbgen.SetViewLayoutParams{LayoutJson: js, ID: view.ID})
	}
	if js, err := layout.SingleLeaf(countdown.ID).Marshal(); err == nil {
		_ = s.store.SetViewLayout(ctx, dbgen.SetViewLayoutParams{LayoutJson: js, ID: aftellen.ID})
	}

	// Default playlist rotating both views (interval from playlist default, with
	// a per-view override on the second).
	playlist, err := s.store.CreatePlaylist(ctx, dbgen.CreatePlaylistParams{
		Name: "Standaard", IsDefault: 1, DefaultDwellSeconds: 15,
	})
	if err != nil {
		return err
	}
	if _, err := s.store.AddPlaylistItem(ctx, dbgen.AddPlaylistItemParams{
		PlaylistID: playlist.ID, ViewID: view.ID, Position: 0, DwellSeconds: 0, // use playlist default (15s)
	}); err != nil {
		return err
	}
	if _, err := s.store.AddPlaylistItem(ctx, dbgen.AddPlaylistItemParams{
		PlaylistID: playlist.ID, ViewID: aftellen.ID, Position: 1, DwellSeconds: 10, // override
	}); err != nil {
		return err
	}

	slog.Info("seeded demo views + default playlist")
	return nil
}
