package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/rotation"
	"github.com/jvmeir/familyplanner/internal/web"
)

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.DevicesPage(s.deviceVMs(r.Context()), s.playlistRefs(r.Context()), s.viewRefs(r.Context())))
}

func (s *Server) handleDeviceAssign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	playlistID, _ := strconv.ParseInt(r.FormValue("playlist_id"), 10, 64) // 0 = default
	if err := s.store.SetDevicePlaylist(r.Context(), dbgen.SetDevicePlaylistParams{
		PlaylistID: playlistID, ID: id,
	}); err != nil {
		http.Error(w, "assign failed", http.StatusInternalServerError)
		return
	}
	// Corner PiP: an independent playlist rotating in a corner (0 = none).
	pipID, _ := strconv.ParseInt(r.FormValue("pip_playlist_id"), 10, 64)
	pc := devicePip{
		Corner: pick(r.FormValue("pip_corner"), "br", "bl", "tr", "tl", "right", "left"),
		Size:   pick(r.FormValue("pip_size"), "m", "s", "l"),
		Muted:  r.FormValue("pip_muted") != "",
	}
	cj, _ := json.Marshal(pc)
	if err := s.store.SetDevicePip(r.Context(), dbgen.SetDevicePipParams{
		PipPlaylistID: pipID, PipConfigJson: string(cj), ID: id,
	}); err != nil {
		http.Error(w, "assign failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.DeviceList(s.deviceVMs(r.Context()), s.playlistRefs(r.Context()), s.viewRefs(r.Context())))
}

// handleDeviceDelete unpairs a kiosk device (e.g. cleaning up test screens).
// The device's cookie becomes invalid immediately; its next request re-pairs.
func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteDevice(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.DeviceList(s.deviceVMs(r.Context()), s.playlistRefs(r.Context()), s.viewRefs(r.Context())))
}

func (s *Server) handleDeviceRename(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if name := r.FormValue("name"); name != "" {
		_ = s.store.UpdateDeviceName(r.Context(), dbgen.UpdateDeviceNameParams{Name: name, ID: id})
	}
	s.render(w, r, web.DeviceList(s.deviceVMs(r.Context()), s.playlistRefs(r.Context()), s.viewRefs(r.Context())))
}

// handleDeviceControl is the phone "remote": it drives a specific device's
// rotation (works only while that device has an open SSE stream).
func (s *Server) handleDeviceControl(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	switch cmd := chi.URLParam(r, "cmd"); cmd {
	case "goto":
		viewID, _ := strconv.ParseInt(r.FormValue("view"), 10, 64)
		s.rotation.Goto(id, viewID)
	case "next", "prev", "pause", "resume":
		s.rotation.Command(id, rotation.Command(cmd))
	case "mute", "unmute", "pip-toggle", "pip-next", "pip-prev":
		// UI-only actions handled by the kiosk itself; forward over its SSE stream.
		s.rotation.SendClientCmd(id, cmd)
	default:
		http.Error(w, "bad command", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deviceVMs(ctx context.Context) []web.DeviceVM {
	rows, err := s.store.ListDevices(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.DeviceVM, 0, len(rows))
	for _, d := range rows {
		name := d.Name
		if name == "" {
			name = "kiosk"
		}
		lastSeen := d.LastSeen
		if lastSeen == "" {
			lastSeen = "—"
		}
		pc := parseDevicePip(d.PipConfigJson)
		out = append(out, web.DeviceVM{
			ID: d.ID, Name: name, LastSeen: lastSeen, PlaylistID: d.PlaylistID,
			PipPlaylistID: d.PipPlaylistID, PipCorner: pc.Corner, PipSize: pc.Size, PipMuted: pc.Muted,
		})
	}
	return out
}

// devicePip is the JSON stored in kiosk_devices.pip_config_json (corner PiP
// presentation; the content is the assigned pip playlist).
type devicePip struct {
	Corner string `json:"corner"`
	Size   string `json:"size"`
	Muted  bool   `json:"muted"`
}

func parseDevicePip(raw string) devicePip {
	pc := devicePip{Corner: "br", Size: "m", Muted: true}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &pc)
	}
	if pc.Corner == "" {
		pc.Corner = "br"
	}
	if pc.Size == "" {
		pc.Size = "m"
	}
	return pc
}

func (s *Server) playlistRefs(ctx context.Context) []web.PlaylistRef {
	rows, err := s.store.ListPlaylists(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.PlaylistRef, 0, len(rows))
	for _, p := range rows {
		out = append(out, web.PlaylistRef{ID: p.ID, Name: p.Name})
	}
	return out
}
