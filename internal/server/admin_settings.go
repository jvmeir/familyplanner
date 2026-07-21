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
		Enabled:    r.FormValue("voice_enabled") != "",
		QuietStart: r.FormValue("quiet_start"),
		QuietEnd:   r.FormValue("quiet_end"),
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
	s.render(w, r, web.SettingsPage(s.settingsVM(r.Context(), true)))
}

func (s *Server) settingsVM(ctx context.Context, saved bool) web.SettingsVM {
	cfg := s.voiceClockConfig(ctx)
	return web.SettingsVM{
		VoiceEnabled: cfg.Enabled,
		QuietStart:   cfg.QuietStart,
		QuietEnd:     cfg.QuietEnd,
		KioskScale:   strconv.FormatFloat(s.kioskScale(ctx), 'f', 2, 64),
		Saved:        saved,
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
