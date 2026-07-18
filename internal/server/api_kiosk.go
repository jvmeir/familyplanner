package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/kioskapi"
	"github.com/jvmeir/familyplanner/internal/theme"
	"github.com/jvmeir/familyplanner/internal/web"
)

// ---------- JSON kiosk API (feeds the SPA client) ----------
//
// These endpoints expose the exact same data the server-rendered kiosk uses,
// as JSON. The SPA fetches state + the current view, subscribes to the existing
// /kiosk/stream SSE for navigate/refresh, and drives playback via the existing
// /kiosk/control endpoints. The HTML kiosk is untouched and keeps working.

func (s *Server) handleAPIKioskState(w http.ResponseWriter, r *http.Request) {
	dev, ok := deviceFrom(r.Context())
	if !ok {
		http.Error(w, "no device", http.StatusUnauthorized)
		return
	}

	// Prefer live rotation state (a stream is open); fall back to the playlist's
	// first item when the device hasn't connected its SSE stream yet.
	var currentID int64
	var paused bool
	if vid, p, live := s.rotation.Peek(dev.ID); live {
		currentID, paused = vid, p
	} else if v, err := s.currentPlaylistView(r.Context(), dev); err == nil {
		currentID = v.ID
	}

	c := s.buildControls(r.Context(), dev, currentID)
	writeJSON(w, kioskapi.State{
		PlaylistName: c.PlaylistName,
		CurrentID:    currentID,
		Paused:       paused,
		Playlist:     toRefs(c.Playlist),
		All:          toRefs(c.All),
	})
}

func (s *Server) handleAPIKioskView(w http.ResponseWriter, r *http.Request) {
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

	out := kioskapi.ViewRender{ID: view.ID, Name: view.Name}
	if lm, th, ok := s.buildViewVM(r.Context(), view); ok {
		out.ThemeVars = th.VarsCSS()
		out.Layout = toLayoutDTO(lm)
	} else {
		// Legacy fixed-grid view: degrade to an even row split of its cells so
		// the SPA still renders something sensible (demo views use layout trees).
		th := theme.Resolve(view.ThemeID, s.defaultTheme(r.Context()))
		out.ThemeVars = th.VarsCSS()
		_, cells := s.renderLegacyGrid(r.Context(), view)
		split := kioskapi.Layout{Dir: "row"}
		for i := range cells {
			cell := toCellDTO(cells[i])
			split.Children = append(split.Children, kioskapi.LayoutChild{
				Weight: 1, Node: kioskapi.Layout{Cell: &cell},
			})
		}
		out.Layout = split
	}
	writeJSON(w, out)
}

// ---------- view-model -> DTO mappers ----------

func toRefs(in []web.ViewRef) []kioskapi.ViewRef {
	out := make([]kioskapi.ViewRef, 0, len(in))
	for _, v := range in {
		out = append(out, kioskapi.ViewRef{ID: v.ID, Name: v.Name})
	}
	return out
}

func toLayoutDTO(vm web.LayoutVM) kioskapi.Layout {
	d := kioskapi.Layout{Dir: vm.Dir}
	if vm.Cell != nil {
		c := toCellDTO(*vm.Cell)
		d.Cell = &c
	}
	for _, ch := range vm.Children {
		d.Children = append(d.Children, kioskapi.LayoutChild{
			Weight: ch.Weight,
			Node:   toLayoutDTO(ch.Node),
		})
	}
	return d
}

func toCellDTO(vm web.CellVM) kioskapi.Cell {
	c := kioskapi.Cell{
		Kind:          vm.Kind,
		Title:         vm.Title,
		Big:           vm.Big,
		Sub:           vm.Sub,
		Body:          vm.Body,
		Lines:         vm.Lines,
		ScheduleTable: vm.ScheduleTable,
		IframeURL:     vm.IframeURL,
		ImageURL:      vm.ImageURL,
		Stale:         vm.Stale,
	}
	if vm.Month != nil {
		m := &kioskapi.Month{Title: vm.Month.Title, Weekdays: vm.Month.Weekdays}
		for _, wk := range vm.Month.Weeks {
			row := make([]kioskapi.Day, 0, len(wk))
			for _, d := range wk {
				row = append(row, kioskapi.Day{Day: d.Day, InMonth: d.InMonth, Today: d.Today, Events: d.Events})
			}
			m.Weeks = append(m.Weeks, row)
		}
		c.Month = m
	}
	for _, sd := range vm.Schedule {
		c.Schedule = append(c.Schedule, kioskapi.Schedule{Label: sd.Label, Today: sd.Today, Events: sd.Events})
	}
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}
