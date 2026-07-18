// Package kioskapi defines the JSON contract between the server and the kiosk
// SPA client. It is intentionally dependency-free (pure data structs) so the
// WASM client can import it without pulling in server/templ/db code.
//
// This is the "API" edge of the API+SPA split: the server renders each view's
// data into these structs (reusing the exact same view-model path as the
// server-rendered HTML kiosk, so the two stay in lock-step), and the browser
// client renders them into DOM.
package kioskapi

// ViewRef is a minimal view reference (jump dropdown, playlist listing).
type ViewRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// State is the kiosk's control/playback state: the active playlist, its views,
// all views (for jumping to parked ones), the current view, and paused flag.
type State struct {
	PlaylistName string    `json:"playlistName"`
	CurrentID    int64     `json:"currentId"`
	Paused       bool      `json:"paused"`
	Playlist     []ViewRef `json:"playlist"`
	All          []ViewRef `json:"all"`
}

// ViewRender is a fully-resolved view ready to paint: its theme's CSS custom
// properties plus the recursive layout tree with each leaf's widget data.
type ViewRender struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ThemeVars string `json:"themeVars"` // ":root{--bg:...;--fg:...}" style CSS
	Layout    Layout `json:"layout"`
}

// Layout mirrors the server's recursive split layout. A leaf has a non-nil
// Cell; a split has a Dir ("row"|"col") and weighted Children.
type Layout struct {
	Dir      string        `json:"dir,omitempty"`
	Children []LayoutChild `json:"children,omitempty"`
	Cell     *Cell         `json:"cell,omitempty"`
}

// LayoutChild is a weighted sub-pane.
type LayoutChild struct {
	Weight float64 `json:"weight"`
	Node   Layout  `json:"node"`
}

// Cell is the presentation-ready data for one widget pane. Only the fields
// relevant to its Kind are populated.
type Cell struct {
	Kind          string     `json:"kind"`
	Title         string     `json:"title,omitempty"`
	Big           string     `json:"big,omitempty"`
	Sub           string     `json:"sub,omitempty"`
	Body          string     `json:"body,omitempty"`
	Lines         []string   `json:"lines,omitempty"`
	Month         *Month     `json:"month,omitempty"`
	Schedule      []Schedule `json:"schedule,omitempty"`
	ScheduleTable bool       `json:"scheduleTable,omitempty"`
	IframeURL     string     `json:"iframeUrl,omitempty"`
	ImageURL      string     `json:"imageUrl,omitempty"`
	Stale         bool       `json:"stale,omitempty"`
}

// Month is a traditional month grid (calendar month/week mode).
type Month struct {
	Title    string   `json:"title"`
	Weekdays []string `json:"weekdays"`
	Weeks    [][]Day  `json:"weeks"`
}

// Day is one cell in the month grid.
type Day struct {
	Day     int      `json:"day"`
	InMonth bool     `json:"inMonth"`
	Today   bool     `json:"today"`
	Events  []string `json:"events,omitempty"`
}

// Schedule is one day row in the relative day-by-day calendar (days modes).
type Schedule struct {
	Label  string   `json:"label"`
	Today  bool     `json:"today"`
	Events []string `json:"events,omitempty"`
}
