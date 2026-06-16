// Package theme defines the design-token system. A theme is just a named set of
// CSS custom properties; every widget renders against these variables, so
// reskinning the whole wall is a matter of swapping the token values.
package theme

import (
	"sort"
	"strings"
)

// Theme is a named bag of CSS custom-property values.
type Theme struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"` // user-facing (Dutch) label
	Tokens map[string]string `json:"tokens"`
}

// DefaultID is the global default theme used when a view has no override.
const DefaultID = "licht"

// Presets are the built-in themes. Custom themes (added later) live in the DB.
var Presets = map[string]Theme{
	"licht": {
		ID:   "licht",
		Name: "Licht",
		Tokens: map[string]string{
			"--bg":         "#f4f6fb",
			"--surface":    "#ffffff",
			"--text":       "#1b1f27",
			"--text-muted": "#5b6472",
			"--accent":     "#2f6df6",
			"--accent-2":   "#13b981",
			"--radius":     "18px",
			"--gap":        "14px",
			"--shadow":     "0 6px 20px rgba(20,30,60,.08)",
			"--font-body":  "system-ui, sans-serif",
		},
	},
	"donker": {
		ID:   "donker",
		Name: "Donker",
		Tokens: map[string]string{
			"--bg":         "#0f1115",
			"--surface":    "#1b1f27",
			"--text":       "#e8eaed",
			"--text-muted": "#9aa3b2",
			"--accent":     "#4f9cff",
			"--accent-2":   "#2ee6a6",
			"--radius":     "18px",
			"--gap":        "14px",
			"--shadow":     "0 6px 20px rgba(0,0,0,.35)",
			"--font-body":  "system-ui, sans-serif",
		},
	},
	"pastel": {
		ID:   "pastel",
		Name: "Pastel",
		Tokens: map[string]string{
			"--bg":         "#fdf6f0",
			"--surface":    "#fffdfb",
			"--text":       "#3a2f2a",
			"--text-muted": "#8a7a70",
			"--accent":     "#e8896b",
			"--accent-2":   "#7bb0a8",
			"--radius":     "22px",
			"--gap":        "16px",
			"--shadow":     "0 6px 20px rgba(120,90,70,.12)",
			"--font-body":  "system-ui, sans-serif",
		},
	},
	"hoogcontrast": {
		ID:   "hoogcontrast",
		Name: "Hoog contrast",
		Tokens: map[string]string{
			"--bg":         "#000000",
			"--surface":    "#0a0a0a",
			"--text":       "#ffffff",
			"--text-muted": "#d0d0d0",
			"--accent":     "#ffd400",
			"--accent-2":   "#00e5ff",
			"--radius":     "8px",
			"--gap":        "12px",
			"--shadow":     "none",
			"--font-body":  "system-ui, sans-serif",
		},
	},
}

// Resolve implements the cascade: an explicit view theme wins, otherwise the
// global default, otherwise the hard default ("licht").
func Resolve(viewThemeID, globalDefaultID string) Theme {
	if t, ok := Presets[viewThemeID]; ok {
		return t
	}
	if t, ok := Presets[globalDefaultID]; ok {
		return t
	}
	return Presets[DefaultID]
}

// VarsCSS renders the theme's tokens as an inline style declaration, e.g.
// "--bg:#fff;--accent:#2f6df6". Keys are sorted for stable output.
func (t Theme) VarsCSS() string {
	keys := make([]string, 0, len(t.Tokens))
	for k := range t.Tokens {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(':')
		b.WriteString(t.Tokens[k])
		b.WriteByte(';')
	}
	return b.String()
}
