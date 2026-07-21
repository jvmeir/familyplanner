package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/voiceclock"
	"github.com/jvmeir/familyplanner/internal/web"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.SettingsPage(s.settingsVM(r.Context(), false)))
}

func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	cfg := voiceclock.Config{
		Enabled:      r.FormValue("voice_enabled") != "",
		QuietStart:   r.FormValue("quiet_start"),
		QuietEnd:     r.FormValue("quiet_end"),
		QuarterSound: voiceclock.ValidQuarterSound(r.FormValue("quarter_sound")),
		HalfSound:    voiceclock.ValidHalfSound(r.FormValue("half_sound")),
		HourSound:    voiceclock.ValidHourSound(r.FormValue("hour_sound")),
		Announce:     r.FormValue("announce") != "",
		Attention:    r.FormValue("attention") != "",
		AnnounceRate: r.FormValue("announce_rate"),
	}
	if js, err := json.Marshal(cfg); err == nil {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "voiceclock", Value: string(js)})
	}
	// Kiosk UI scale multiplier (on top of the automatic viewport scaling).
	if v, err := strconv.ParseFloat(r.FormValue("kiosk_scale"), 64); err == nil {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{
			Key: "kiosk_scale", Value: strconv.FormatFloat(clampScale(v), 'f', 2, 64),
		})
	}
	// Global ticker widget selection (0 = none).
	if v := r.FormValue("ticker_widget_id"); v != "" {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "ticker_widget_id", Value: v})
	}
	// Global theme (applies to all screens).
	if v := r.FormValue("theme"); v != "" {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "default_theme", Value: v})
	}
	if v := r.FormValue("ticker_speed"); v != "" {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "ticker_speed", Value: v})
	}
	if v := r.FormValue("banner_date"); v != "" {
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "banner_date", Value: v})
	}
	// Global data-source background refresh cadence (minutes → seconds).
	if v, err := strconv.Atoi(r.FormValue("refresh_minutes")); err == nil && v > 0 {
		if v > 1440 {
			v = 1440
		}
		_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{
			Key: "refresh_interval_secs", Value: strconv.Itoa(v * 60),
		})
	}
	// Slide transition when rotating to a new screen (checkbox; default on).
	transition := "0"
	if r.FormValue("transition") != "" {
		transition = "1"
	}
	_ = s.store.SetSetting(r.Context(), dbgen.SetSettingParams{Key: "kiosk_transition", Value: transition})
	s.render(w, r, web.SettingsPage(s.settingsVM(r.Context(), true)))
}

func (s *Server) settingsVM(ctx context.Context, saved bool) web.SettingsVM {
	cfg := s.voiceClockConfig(ctx)
	var tickers []web.ViewRef
	if ws, err := s.store.ListWidgets(ctx); err == nil {
		for _, w := range ws {
			if w.Type == "ticker" {
				tickers = append(tickers, web.ViewRef{ID: w.ID, Name: w.Name})
			}
		}
	}
	var tickerID int64
	if v, err := s.store.GetSetting(ctx, "ticker_widget_id"); err == nil {
		tickerID, _ = strconv.ParseInt(v, 10, 64)
	}
	return web.SettingsVM{
		VoiceEnabled:   cfg.Enabled,
		QuietStart:     cfg.QuietStart,
		QuietEnd:       cfg.QuietEnd,
		QuarterSound:   voiceclock.ValidQuarterSound(cfg.QuarterSound),
		HalfSound:      voiceclock.ValidHalfSound(cfg.HalfSound),
		HourSound:      voiceclock.ValidHourSound(cfg.HourSound),
		Announce:       cfg.Announce,
		Attention:      cfg.Attention,
		AnnounceRate:   cfg.AnnounceRate,
		KioskScale:     strconv.FormatFloat(s.kioskScale(ctx), 'f', 2, 64),
		TickerWidgets:  tickers,
		TickerWidgetID: tickerID,
		TickerSpeed:    strconv.Itoa(s.tickerSpeed(ctx)),
		BannerDate:     s.bannerDate(ctx),
		Transition:     s.kioskTransition(ctx),
		Themes:         s.themeOpts(),
		Theme:          s.defaultTheme(ctx),
		RefreshMinutes: strconv.Itoa(s.refreshMinutes(ctx)),
		Saved:          saved,
	}
}

// refreshMinutes is the global data-source refresh cadence in minutes (default
// 15), read from the "refresh_interval_secs" setting.
func (s *Server) refreshMinutes(ctx context.Context) int {
	v, err := s.store.GetSetting(ctx, "refresh_interval_secs")
	if err != nil || v == "" {
		return 15
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 15
	}
	m := secs / 60
	if m < 1 {
		m = 1
	}
	return m
}

// tickerSpeed is the ticker scroll-loop duration in seconds (default 60).
func (s *Server) tickerSpeed(ctx context.Context) int {
	v, err := s.store.GetSetting(ctx, "ticker_speed")
	if err != nil || v == "" {
		return 60
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 60
	}
	if n < 20 {
		n = 20
	}
	if n > 240 {
		n = 240
	}
	return n
}

// bannerDate is the banner date display mode: none | short | long (default long).
func (s *Server) bannerDate(ctx context.Context) string {
	switch v, _ := s.store.GetSetting(ctx, "banner_date"); v {
	case "none", "short", "long":
		return v
	default:
		return "long"
	}
}

// kioskTransition reports whether the slide animation plays on screen changes
// (default on; only "0" disables it).
func (s *Server) kioskTransition(ctx context.Context) bool {
	v, err := s.store.GetSetting(ctx, "kiosk_transition")
	return err != nil || v != "0"
}

// kioskScale is the kiosk UI scale multiplier (default 1.0, clamped 0.5–2.0).
func (s *Server) kioskScale(ctx context.Context) float64 {
	raw, err := s.store.GetSetting(ctx, "kiosk_scale")
	if err != nil || raw == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 1.0
	}
	return clampScale(v)
}

func clampScale(v float64) float64 {
	if v < 0.5 {
		return 0.5
	}
	if v > 2.0 {
		return 2.0
	}
	return v
}
