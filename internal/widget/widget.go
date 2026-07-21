// Package widget defines the extensible widget-type registry and the built-in
// widget providers. Adding a widget type = register a Type with a provider that
// fetches+normalizes data. Rendering lives in the web package (keyed by type).
package widget

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

// Data is the normalized result a provider returns for rendering.
type Data any

// Provider fetches and normalizes a widget's data. For widgets backed by
// external sources, Fetch is also where last-good caching will hook in later.
type Provider interface {
	// Fetch returns the data and how long it stays valid (TTL).
	Fetch(ctx context.Context) (Data, time.Duration, error)
}

// NowFunc supplies the current time (injected so widgets are testable).
type NowFunc func() time.Time

// SourceInput is a resolved data source attached to a widget instance, with the
// per-link filter. Widgets that aggregate sources (e.g. calendar) consume these.
type SourceInput struct {
	Type     string          // e.g. "ical", "bring"
	Config   json.RawMessage // the data source's config (e.g. {"url": "..."})
	Secret   json.RawMessage // decrypted credentials (e.g. {"email","password"}); nil if none
	Filter   string          // per-(widget,source) filter expression
	Resource string          // per-(widget,source) chosen resource id (list/calendar/folder/album)
	Color    string          // per-(widget,source) colour (e.g. calendar event colour coding)
}

// FieldType is the input kind for a config field (drives the admin form widget).
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldDate     FieldType = "date"
	FieldNumber   FieldType = "number"
	FieldSelect   FieldType = "select"
	FieldPassword FieldType = "password"
	FieldTextarea FieldType = "textarea"
)

// Option is a choice for a FieldSelect field.
type Option struct {
	Value    string
	LabelKey string
}

// Field describes one configurable setting of a widget type.
type Field struct {
	Name     string    // config key
	LabelKey string    // i18n key for the field label
	Type     FieldType // input type
	Required bool
	Options  []Option // for FieldSelect
}

// Schema is a widget type's config schema. The admin UI generates a form from
// it, so adding a widget type needs no hand-built form.
type Schema struct {
	Fields []Field
}

// Type describes a registered widget type.
type Type struct {
	ID             string   // stable identifier, e.g. "countdown"
	NameKey        string   // i18n key for the display name
	Schema         Schema   // config schema (drives the admin form)
	AcceptsSources []string // data-source type ids this widget can use (nil = none)
	NewProvider    func(config json.RawMessage, sources []SourceInput, now NowFunc) (Provider, error)
	// Decode rehydrates cached JSON into this widget's concrete Data type so the
	// renderer can type-assert it (the broker caches Data as JSON).
	Decode func(json.RawMessage) (Data, error)
}

// Registry holds the known widget types.
type Registry struct {
	types map[string]Type
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{types: make(map[string]Type)}
}

// Register adds (or replaces) a widget type.
func (r *Registry) Register(t Type) { r.types[t.ID] = t }

// Get looks up a widget type by ID.
func (r *Registry) Get(id string) (Type, bool) {
	t, ok := r.types[id]
	return t, ok
}

// Accepts reports whether this widget type can use a data source of dsType.
func (t Type) Accepts(dsType string) bool {
	for _, s := range t.AcceptsSources {
		if s == dsType {
			return true
		}
	}
	return false
}

// Types returns the registered types sorted by ID (stable for the admin UI).
func (r *Registry) Types() []Type {
	out := make([]Type, 0, len(r.types))
	for _, t := range r.types {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RegisterDefaults registers the built-in widget types and their config schemas.
func RegisterDefaults(r *Registry) {
	r.Register(Type{
		ID:          "countdown",
		NameKey:     "widget.countdown.name",
		NewProvider: newCountdown,
		Decode:      decodeCountdown,
		Schema: Schema{Fields: []Field{
			{Name: "date", LabelKey: "widget.countdown.field.date", Type: FieldDate, Required: true},
			{Name: "time", LabelKey: "widget.countdown.field.time", Type: FieldText},
			{Name: "precision", LabelKey: "widget.countdown.field.precision", Type: FieldSelect, Options: []Option{
				{Value: "days", LabelKey: "widget.countdown.precision.days"},
				{Value: "dhms", LabelKey: "widget.countdown.precision.dhms"},
			}},
		}},
	})
	r.Register(Type{
		ID:          "clock",
		NameKey:     "widget.clock.name",
		NewProvider: newClock,
		Decode:      decodeClock,
		Schema:      Schema{}, // no configuration
	})
	r.Register(Type{
		ID:             "calendar",
		NameKey:        "widget.calendar.name",
		NewProvider:    newCalendar,
		Decode:         decodeCalendar,
		AcceptsSources: []string{"ical", "ms_graph", "ms_todo"},
		// Feeds + per-feed filters come from linked data sources (managed on the
		// widget's edit page). These fields control display only.
		Schema: Schema{Fields: []Field{
			{Name: "mode", LabelKey: "widget.calendar.field.mode", Type: FieldSelect, Options: []Option{
				{Value: "agenda", LabelKey: "widget.calendar.mode.agenda"},
				{Value: "days", LabelKey: "widget.calendar.mode.days"},
				{Value: "days_table", LabelKey: "widget.calendar.mode.days_table"},
				{Value: "week", LabelKey: "widget.calendar.mode.week"},
				{Value: "month", LabelKey: "widget.calendar.mode.month"},
			}},
			{Name: "weeks_before", LabelKey: "widget.calendar.field.weeks_before", Type: FieldNumber},
			{Name: "weeks_ahead", LabelKey: "widget.calendar.field.weeks_ahead", Type: FieldNumber},
		}},
	})
	r.Register(Type{
		ID:          "weather",
		NameKey:     "widget.weather.name",
		NewProvider: newWeather,
		Decode:      decodeWeather,
		Schema: Schema{Fields: []Field{
			{Name: "lat", LabelKey: "widget.weather.field.lat", Type: FieldText},
			{Name: "lon", LabelKey: "widget.weather.field.lon", Type: FieldText},
		}},
	})
	r.Register(Type{
		ID:          "quote",
		NameKey:     "widget.quote.name",
		NewProvider: newQuote,
		Decode:      decodeQuote,
		Schema:      Schema{}, // no configuration
	})
	r.Register(Type{
		ID:          "web",
		NameKey:     "widget.web.name",
		NewProvider: newWeb,
		Decode:      decodeWeb,
		Schema: Schema{Fields: []Field{
			{Name: "url", LabelKey: "widget.web.field.url", Type: FieldText, Required: true},
		}},
	})
	r.Register(Type{
		ID:             "shopping",
		NameKey:        "widget.shopping.name",
		NewProvider:    newShopping,
		Decode:         decodeShopping,
		AcceptsSources: []string{"bring"},
		Schema:         Schema{},
	})
	r.Register(Type{
		ID:             "todolist",
		NameKey:        "widget.todolist.name",
		NewProvider:    newTodo,
		Decode:         decodeTodo,
		AcceptsSources: []string{"ms_todo"},
		Schema: Schema{Fields: []Field{
			{Name: "scope", LabelKey: "widget.todolist.field.scope", Type: FieldSelect, Options: []Option{
				{Value: "all", LabelKey: "widget.todolist.scope.all"},
				{Value: "today_overdue", LabelKey: "widget.todolist.scope.today_overdue"},
			}},
			{Name: "hide_no_due", LabelKey: "widget.todolist.field.hide_no_due", Type: FieldSelect, Options: []Option{
				{Value: "no", LabelKey: "widget.todolist.hide_no_due.no"},
				{Value: "yes", LabelKey: "widget.todolist.hide_no_due.yes"},
			}},
			{Name: "all_lists", LabelKey: "widget.todolist.field.all_lists", Type: FieldSelect, Options: []Option{
				{Value: "no", LabelKey: "widget.todolist.all_lists.no"},
				{Value: "yes", LabelKey: "widget.todolist.all_lists.yes"},
			}},
		}},
	})
	r.Register(Type{
		ID:             "ticker",
		NameKey:        "widget.ticker.name",
		NewProvider:    newTicker,
		Decode:         decodeTicker,
		AcceptsSources: []string{"rss", "text"},
		Schema: Schema{Fields: []Field{
			{Name: "order", LabelKey: "widget.ticker.field.order", Type: FieldSelect, Options: []Option{
				{Value: "sequential", LabelKey: "widget.ticker.order.sequential"},
				{Value: "random", LabelKey: "widget.ticker.order.random"},
			}},
		}},
	})
	r.Register(Type{
		ID:             "photos",
		NameKey:        "widget.photos.name",
		NewProvider:    newPhotos,
		Decode:         decodePhotos,
		AcceptsSources: []string{"google_photos", "onedrive"},
		Schema: Schema{Fields: []Field{
			{Name: "mode", LabelKey: "widget.photos.field.mode", Type: FieldSelect, Options: []Option{
				{Value: "single", LabelKey: "widget.photos.mode.single"},
				{Value: "random", LabelKey: "widget.photos.mode.random"},
				{Value: "by_date", LabelKey: "widget.photos.mode.by_date"},
			}},
		}},
	})
}
