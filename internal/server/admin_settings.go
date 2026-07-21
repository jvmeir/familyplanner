package server

import (
	"context"
	"encoding/json"
	"net/http"

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
	s.render(w, r, web.SettingsPage(s.settingsVM(r.Context(), true)))
}

func (s *Server) settingsVM(ctx context.Context, saved bool) web.SettingsVM {
	cfg := s.voiceClockConfig(ctx)
	return web.SettingsVM{
		VoiceEnabled: cfg.Enabled,
		QuietStart:   cfg.QuietStart,
		QuietEnd:     cfg.QuietEnd,
		Saved:        saved,
	}
}
