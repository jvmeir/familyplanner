// Package datasource defines the extensible registry of data-source types.
// Like widgets, each type declares a config schema (drives the admin form) and a
// credential kind. Credential field values are stored encrypted at rest.
package datasource

import (
	"sort"

	"github.com/jvmeir/familyplanner/internal/widget"
)

// CredentialKind describes how a data source authenticates.
type CredentialKind string

const (
	CredNone   CredentialKind = "none"
	CredBasic  CredentialKind = "basic"  // username/email + password (encrypted)
	CredOAuth2 CredentialKind = "oauth2" // authorization-code flow (M3)
)

// Type describes a registered data-source type.
type Type struct {
	ID         string        // stable id, e.g. "ical", "bring"
	NameKey    string        // i18n key for the display name
	Config     widget.Schema // non-secret config fields (stored in config_json)
	Credential widget.Schema // secret fields (stored encrypted in secret_ciphertext)
	CredKind   CredentialKind

	// Dynamic resource picker, shown on the per-widget↔source link (the chosen
	// resource is stored on that relation, never on the data source itself).
	// Empty ResourceKind = no picker.
	ResourceKind     string // "ms_calendars" | "bring_lists" | "ms_todo_lists" | …
	ResourceLabelKey string // i18n label for the picker
}

// Registry holds the known data-source types.
type Registry struct {
	types map[string]Type
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{types: make(map[string]Type)} }

// Register adds (or replaces) a type.
func (r *Registry) Register(t Type) { r.types[t.ID] = t }

// Get looks up a type by id.
func (r *Registry) Get(id string) (Type, bool) {
	t, ok := r.types[id]
	return t, ok
}

// Types returns the registered types sorted by id.
func (r *Registry) Types() []Type {
	out := make([]Type, 0, len(r.types))
	for _, t := range r.types {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RegisterDefaults registers the built-in data-source types.
func RegisterDefaults(r *Registry) {
	r.Register(Type{
		ID:       "ical",
		NameKey:  "datasource.type.ical",
		CredKind: CredNone,
		Config: widget.Schema{Fields: []widget.Field{
			{Name: "url", LabelKey: "datasource.field.url", Type: widget.FieldText, Required: true},
		}},
	})
	r.Register(Type{
		ID:       "rss",
		NameKey:  "datasource.type.rss",
		CredKind: CredNone,
		Config: widget.Schema{Fields: []widget.Field{
			{Name: "url", LabelKey: "datasource.field.rss_url", Type: widget.FieldText, Required: true},
		}},
	})
	r.Register(Type{
		ID:       "text",
		NameKey:  "datasource.type.text",
		CredKind: CredNone,
		Config: widget.Schema{Fields: []widget.Field{
			{Name: "lines", LabelKey: "datasource.field.text_lines", Type: widget.FieldTextarea, Required: true},
		}},
	})
	r.Register(Type{
		ID:       "bring",
		NameKey:  "datasource.type.bring",
		CredKind: CredBasic,
		Credential: widget.Schema{Fields: []widget.Field{
			{Name: "email", LabelKey: "datasource.field.email", Type: widget.FieldText, Required: true},
			{Name: "password", LabelKey: "datasource.field.password", Type: widget.FieldPassword, Required: true},
		}},
		ResourceKind:     "bring_lists",
		ResourceLabelKey: "datasource.field.bring_list",
	})
	// OAuth2 types: the app client id/secret come from app config; creating one
	// is just an interactive sign-in that stores the user's token.
	r.Register(Type{
		ID:               "ms_graph",
		NameKey:          "datasource.type.ms_graph",
		CredKind:         CredOAuth2,
		ResourceKind:     "ms_calendars",
		ResourceLabelKey: "datasource.field.calendar",
	})
	r.Register(Type{
		ID:               "google_photos",
		NameKey:          "datasource.type.google_photos",
		CredKind:         CredOAuth2,
		ResourceKind:     "google_albums",
		ResourceLabelKey: "datasource.field.album",
	})
	r.Register(Type{
		ID:               "onedrive",
		NameKey:          "datasource.type.onedrive",
		CredKind:         CredOAuth2,
		ResourceKind:     "onedrive_folders",
		ResourceLabelKey: "datasource.field.folder",
	})
	r.Register(Type{
		ID:               "ms_todo",
		NameKey:          "datasource.type.ms_todo",
		CredKind:         CredOAuth2,
		ResourceKind:     "ms_todo_lists",
		ResourceLabelKey: "datasource.field.todolist",
	})
}
