package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/web"
)

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.PlaylistsPage(s.playlistVMs(r.Context())))
}

func (s *Server) handlePlaylistCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	var isDefault int64
	if r.FormValue("is_default") == "1" {
		_ = s.store.ClearDefaultPlaylists(r.Context())
		isDefault = 1
	}
	if _, err := s.store.CreatePlaylist(r.Context(), dbgen.CreatePlaylistParams{
		Name:                name,
		IsDefault:           isDefault,
		DefaultDwellSeconds: parseIntDefault(r.FormValue("interval"), 30),
	}); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.PlaylistList(s.playlistVMs(r.Context())))
}

func (s *Server) handlePlaylistDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeletePlaylist(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.PlaylistList(s.playlistVMs(r.Context())))
}

func (s *Server) handlePlaylistSetDefault(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	_ = s.store.ClearDefaultPlaylists(r.Context())
	if err := s.store.SetDefaultPlaylist(r.Context(), id); err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.PlaylistList(s.playlistVMs(r.Context())))
}

func (s *Server) handlePlaylistDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	vm, ok := s.playlistDetailVM(r.Context(), id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, r, web.PlaylistDetailPage(vm))
}

func (s *Server) handlePlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdatePlaylist(r.Context(), dbgen.UpdatePlaylistParams{
		Name:                name,
		DefaultDwellSeconds: parseIntDefault(r.FormValue("interval"), 30),
		ID:                  id,
	}); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/playlists/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) handlePlaylistAddItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	viewID, err := strconv.ParseInt(r.FormValue("view_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad view", http.StatusBadRequest)
		return
	}
	var dwell int64
	if d, perr := strconv.ParseInt(r.FormValue("dwell"), 10, 64); perr == nil && d > 0 {
		dwell = d
	}
	maxPos, _ := s.store.MaxPlaylistPosition(r.Context(), id)
	if _, err := s.store.AddPlaylistItem(r.Context(), dbgen.AddPlaylistItemParams{
		PlaylistID: id, ViewID: viewID, Position: maxPos + 1, DwellSeconds: dwell,
	}); err != nil {
		http.Error(w, "add failed", http.StatusInternalServerError)
		return
	}
	s.renderPlaylistItems(w, r, id)
}

func (s *Server) handlePlaylistItemDelete(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	item, err := s.store.GetPlaylistItem(r.Context(), itemID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = s.store.DeletePlaylistItem(r.Context(), itemID)
	s.renderPlaylistItems(w, r, item.PlaylistID)
}

func (s *Server) handlePlaylistItemMove(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	item, err := s.store.GetPlaylistItem(r.Context(), itemID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	items, _ := s.store.ListPlaylistItems(r.Context(), item.PlaylistID)
	idx := -1
	for i, it := range items {
		if it.ID == itemID {
			idx = i
			break
		}
	}
	swap := -1
	switch r.URL.Query().Get("dir") {
	case "up":
		if idx > 0 {
			swap = idx - 1
		}
	case "down":
		if idx >= 0 && idx < len(items)-1 {
			swap = idx + 1
		}
	}
	if swap >= 0 {
		a, b := items[idx], items[swap]
		_ = s.store.UpdatePlaylistItemPosition(r.Context(), dbgen.UpdatePlaylistItemPositionParams{Position: b.Position, ID: a.ID})
		_ = s.store.UpdatePlaylistItemPosition(r.Context(), dbgen.UpdatePlaylistItemPositionParams{Position: a.Position, ID: b.ID})
	}
	s.renderPlaylistItems(w, r, item.PlaylistID)
}

func (s *Server) renderPlaylistItems(w http.ResponseWriter, r *http.Request, playlistID int64) {
	vm, ok := s.playlistDetailVM(r.Context(), playlistID)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, r, web.PlaylistItems(vm))
}

// ---- view-model builders ----

func (s *Server) playlistVMs(ctx context.Context) []web.PlaylistVM {
	rows, err := s.store.ListPlaylists(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.PlaylistVM, 0, len(rows))
	for _, p := range rows {
		out = append(out, web.PlaylistVM{
			ID: p.ID, Name: p.Name, DefaultDwell: p.DefaultDwellSeconds, IsDefault: p.IsDefault == 1,
		})
	}
	return out
}

func (s *Server) playlistDetailVM(ctx context.Context, id int64) (web.PlaylistDetailVM, bool) {
	pl, err := s.store.GetPlaylist(ctx, id)
	if err != nil {
		return web.PlaylistDetailVM{}, false
	}
	vm := web.PlaylistDetailVM{ID: pl.ID, Name: pl.Name, DefaultDwell: pl.DefaultDwellSeconds}
	items, _ := s.store.ListPlaylistItems(ctx, id)
	for _, it := range items {
		name := "?"
		if v, err := s.store.GetView(ctx, it.ViewID); err == nil {
			name = v.Name
		}
		vm.Items = append(vm.Items, web.PlaylistItemVM{ID: it.ID, ViewName: name, Dwell: it.DwellSeconds})
	}
	vm.AvailableViews = s.viewRefs(ctx)
	return vm, true
}

func (s *Server) viewRefs(ctx context.Context) []web.ViewRef {
	rows, err := s.store.ListViews(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.ViewRef, 0, len(rows))
	for _, v := range rows {
		out = append(out, web.ViewRef{ID: v.ID, Name: v.Name})
	}
	return out
}
