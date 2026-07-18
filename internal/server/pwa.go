package server

import (
	"io/fs"
	"net/http"

	"github.com/jvmeir/familyplanner/internal/web"
)

// ---------- PWA: service worker + web app manifests ----------
//
// These are served at the site root (public, no auth) so the service worker can
// claim scope "/" and control every page. Service workers only register in a
// secure context (HTTPS or http://localhost), so on a plain-LAN http host the
// app simply behaves as a normal web app; on the Tailscale .ts.net HTTPS name
// (the real kiosk) offline caching activates.

// handleServiceWorker serves the embedded sw.js with root scope.
func (s *Server) handleServiceWorker(w http.ResponseWriter, _ *http.Request) {
	b, err := fs.ReadFile(web.Assets(), "sw.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// handleManifest serves the general (phone/admin) manifest: installs to /,
// standalone. Used by every server-rendered page via the shared Layout.
func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	writeManifest(w, generalManifest)
}

// handleKioskManifest serves the kiosk manifest: installs to /spa, fullscreen —
// intended for the wall display. Linked from the WASM SPA bootstrap.
func (s *Server) handleKioskManifest(w http.ResponseWriter, _ *http.Request) {
	writeManifest(w, kioskManifest)
}

func writeManifest(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(body))
}

const generalManifest = `{
  "name": "Family Planner",
  "short_name": "Planner",
  "description": "Familie-planner: kiosk en beheer.",
  "lang": "nl",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f4f6fb",
  "theme_color": "#2f6df6",
  "icons": [
    {"src": "/static/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any maskable"},
    {"src": "/static/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable"}
  ]
}`

const kioskManifest = `{
  "name": "Family Planner Kiosk",
  "short_name": "Kiosk",
  "description": "Familie-planner kioskweergave.",
  "lang": "nl",
  "start_url": "/spa",
  "scope": "/",
  "display": "fullscreen",
  "orientation": "landscape",
  "background_color": "#0f1220",
  "theme_color": "#0f1220",
  "icons": [
    {"src": "/static/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any maskable"},
    {"src": "/static/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable"}
  ]
}`
