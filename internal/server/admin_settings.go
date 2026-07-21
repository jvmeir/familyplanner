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
		HourSound:    voiceclock.ValidHourSound(r.FormValue("hour_sound")),
		Announce:     r.FormValue("announce") != "",
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
		HourSound:      voiceclock.ValidHourSound(cfg.HourSound),
		Announce:       cfg.Announce,
		KioskScale:     strconv.FormatFloat(s.kioskScale(ctx), 'f', 2, 64),
		TickerWidgets:  tickers,
		TickerWidgetID: tickerID,
		Saved:          saved,
	}
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
