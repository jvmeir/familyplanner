package web

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/theme"
	"github.com/jvmeir/familyplanner/internal/widget"
)

// colorRe validates a colour token (hex or a plain CSS colour name) before it is
// interpolated into inline CSS, so a source colour can't inject styles.
var colorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,24})$`)

// evStyle returns a coloured left-accent for a calendar event line, or "" when
// the source has no (valid) colour.
func evStyle(color string) templ.SafeCSS {
	if !colorRe.MatchString(color) {
		return ""
	}
	return templ.SafeCSS("border-left:0.4em solid " + color + ";padding-left:0.3em")
}

// colorOrDefault gives an <input type=color> a valid hex value (it can't be
// empty); unset links default to a neutral grey.
func colorOrDefault(c string) string {
	if colorRe.MatchString(c) {
		return c
	}
	return "#888888"
}

// CellVM is the presentation-ready view-model for one grid cell. Widget-type
// specifics are flattened here (in Go) so templates stay dumb.
type CellVM struct {
	Kind          string
	Title         string
	TitleSize     string // small | medium | large (title font scale)
	TitleAlign    string // left | center | right
	Big           string
	Sub           string
	Body          string          // paragraph text (e.g. a quote)
	Lines         []string        // plain list rows (e.g. shopping, to-do)
	Agenda        []EvVM          // calendar agenda rows (colour-coded)
	Month         *MonthVM        // month-grid table (calendar month mode)
	Schedule      []ScheduleDayVM // relative day-by-day (calendar days modes)
	ScheduleTable bool            // render Schedule as a table (days_table) vs list (days)
	IframeURL     string          // embedded web page
	ImageURL      string          // single photo
	CountdownTo   int64           // >0: render a live dhms countdown to this Unix time
	Stale         bool
	Style         templ.SafeCSS
}

// EvVM is one rendered calendar line; Color is the source's colour ("" = none).
type EvVM struct {
	Text  string
	Color string
}

// evItems maps widget calendar items to the presentation view-model.
func evItems(in []widget.CalItem) []EvVM {
	out := make([]EvVM, 0, len(in))
	for _, e := range in {
		out = append(out, EvVM{Text: e.Text, Color: e.Color})
	}
	return out
}

// ScheduleDayVM is one day row in the relative day-by-day calendar table.
type ScheduleDayVM struct {
	Label  string
	Today  bool
	Events []EvVM
}

// MonthVM is a traditional month grid for the calendar widget.
type MonthVM struct {
	Title    string
	Weekdays []string
	Weeks    [][]DayVM
}

// DayVM is one day cell in the month grid.
type DayVM struct {
	Day     int
	InMonth bool
	Today   bool
	Events  []EvVM
}

// ViewRef is a minimal view reference for the kiosk's jump dropdown.
type ViewRef struct {
	ID   int64
	Name string
}

// HealthVM drives the kiosk's corner health badge. Level is "" or "ok" when
// everything is healthy (badge hidden), "warn" (amber) or "error" (red)
// otherwise. Message is the most urgent issue; Count is the total.
type HealthVM struct {
	Level   string
	Count   int
	Message string
}

// Bad reports whether the badge should show (something needs attention).
func (h HealthVM) Bad() bool { return h.Level == "warn" || h.Level == "error" }

// ControlsVM drives the kiosk control bar: the current playlist's views, all
// views (for jumping to parked ones), and which view is currently showing.
type ControlsVM struct {
	PlaylistName string
	Playlist     []ViewRef
	All          []ViewRef
	CurrentID    int64
	NamesJSON    string // JSON {viewID: name} so the footer can label the current view
}

// LayoutVM is the render tree for a view's recursive split layout. A leaf has a
// non-nil Cell; a split has Children. Rendered with nested flexbox.
type LayoutVM struct {
	Dir      string // "row" | "col" (split only)
	Children []LayoutChildVM
	Cell     *CellVM // leaf only
}

// LayoutChildVM is a weighted sub-pane.
type LayoutChildVM struct {
	Weight float64
	Node   LayoutVM
}

// ThemeVars renders just the theme's CSS custom properties (for the layout container).
func ThemeVars(th theme.Theme) templ.SafeCSS {
	return templ.SafeCSS(th.VarsCSS())
}

// PaneStyle renders a flex declaration for a weighted pane.
func PaneStyle(weight float64) templ.SafeCSS {
	if weight <= 0 {
		weight = 1
	}
	return templ.SafeCSS("flex:" + strconv.FormatFloat(weight, 'f', -1, 64) + " 1 0;min-width:0;min-height:0;")
}

// ---- admin view-models ----

// WidgetVM is a row in the admin widgets list.
type WidgetVM struct {
	ID       int64
	Name     string
	Type     string
	TypeName string
}

// WidgetTypeVM is an option in the widget-type dropdown.
type WidgetTypeVM struct {
	ID   string
	Name string
}

// FieldVM is one schema-driven form field.
type FieldVM struct {
	Name     string
	Label    string
	Type     string // input type: text | date | number | select
	Required bool
	Value    string
	Options  []OptionVM // for select
}

// OptionVM is a choice in a select field.
type OptionVM struct {
	Value    string
	Label    string
	Selected bool
}

// ViewVM is a row in the admin views list.
type ViewVM struct {
	ID   int64
	Name string
	Cols int64
	Rows int64
}

// ThemeOptVM is an option in the theme dropdown.
type ThemeOptVM struct {
	ID   string
	Name string
}

// listVars exposes the row count to CSS (for count-based font scaling).
func listVars(n int) templ.SafeCSS {
	if n < 1 {
		n = 1
	}
	return templ.SafeCSS("--n:" + strconv.Itoa(n))
}

func navClass(active, name string) string {
	if active == name {
		return "navlink active"
	}
	return "navlink"
}

// PlaylistVM is a row in the admin playlists list.
type PlaylistVM struct {
	ID           int64
	Name         string
	DefaultDwell int64
	IsDefault    bool
}

// PlaylistRef is a minimal playlist reference (device assignment dropdown).
type PlaylistRef struct {
	ID   int64
	Name string
}

// PlaylistItemVM is one view within a playlist.
type PlaylistItemVM struct {
	ID       int64
	ViewName string
	Dwell    int64 // 0 = use the playlist default
}

// PlaylistDetailVM drives the playlist detail/edit page.
type PlaylistDetailVM struct {
	ID             int64
	Name           string
	DefaultDwell   int64
	Items          []PlaylistItemVM
	AvailableViews []ViewRef
}

// DeviceVM is a row on the devices page.
type DeviceVM struct {
	ID         int64
	Name       string
	LastSeen   string
	PlaylistID int64
}

// DataSourceVM is a row on the data sources page / source dropdown.
type DataSourceVM struct {
	ID        int64
	Name      string
	Type      string
	URL       string
	IsOAuth   bool // shows a Connect action + status
	Connected bool // oauth connected
	HasPicker bool // shows a Configure action
	HasConfig bool // shows an Edit action (type has editable config fields)
	// Health telemetry (OAuth sources): shown as a status pill.
	HealthLevel string // "" | ok | warn | error
	HealthText  string // short Dutch status
	LastError   string // most recent error (tooltip)
	LastSync    string // last successful sync time
}

// WidgetSourceVM is one data source linked to a widget (with its filter and an
// optional live resource picker — e.g. which Bring list / calendar / folder).
type WidgetSourceVM struct {
	LinkID          int64
	SourceName      string
	SourceType      string
	Filter          string
	HasPicker       bool
	ResourceLabel   string
	ResourceOptions []OptionVM
	ShowColor       bool   // widget colour-codes its sources (calendar)
	Color           string // per-link colour (hex, e.g. "#3b82f6")
}

// EditorNodeVM mirrors a layout node for the visual editor, carrying each
// node's path (dot-separated child indices) for structural-edit requests.
type EditorNodeVM struct {
	Path       string // "" = root, "0.1" = root.child0.child1
	IsLeaf     bool
	WidgetID   int64
	WidgetName string
	Dir        string // "row" | "col" (split only)
	Children   []EditorChildVM
}

// EditorChildVM is a weighted sub-pane in the editor.
type EditorChildVM struct {
	Weight float64
	Node   EditorNodeVM
}

// CellFormatter turns a widget's normalized data into a CellVM. It receives ctx
// so it can localize text via the request's active locale.
type CellFormatter func(ctx context.Context, data any) CellVM

var formatters = map[string]CellFormatter{}

// RegisterFormatter wires a render formatter to a widget type. This is the
// render side of the widget registry (kept here to avoid templ <-> widget cycles).
func RegisterFormatter(kind string, f CellFormatter) { formatters[kind] = f }

// FormatCell builds the view-model for a cell, applying its grid placement style
// and stale flag. Unknown/failed widgets degrade gracefully.
func FormatCell(ctx context.Context, kind string, data any, stale bool, style templ.SafeCSS) CellVM {
	var vm CellVM
	if f, ok := formatters[kind]; ok && data != nil {
		vm = f(ctx, data)
	} else {
		vm = CellVM{Title: kind}
	}
	vm.Kind = kind
	vm.Stale = stale
	vm.Style = style
	return vm
}

// GridStyle renders the resolved theme tokens plus the CSS-grid track definition.
func GridStyle(v dbgen.View, th theme.Theme) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf(
		"%sdisplay:grid;grid-template-columns:repeat(%d,1fr);grid-template-rows:repeat(%d,1fr);gap:var(--gap);",
		th.VarsCSS(), v.Cols, v.Rows))
}

// CellStyle renders a placement's grid-area (column/row spans).
func CellStyle(p dbgen.Placement) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf(
		"grid-column:%d / span %d;grid-row:%d / span %d;",
		p.Col, p.ColSpan, p.Row, p.RowSpan))
}

func init() {
	RegisterFormatter("countdown", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.CountdownData)
		vm := CellVM{Title: d.Title}
		// Live days/hours/minutes/seconds ticker (client-side): the template emits
		// a data-target and kiosk.js updates it every second.
		if d.Precision == "dhms" {
			vm.CountdownTo = d.TargetUnix
			vm.Big = strconv.Itoa(d.DaysLeft) // server-side fallback if JS is off
			return vm
		}
		if d.Today {
			vm.Big = i18n.T(ctx, "countdown.today")
		} else {
			vm.Big = strconv.Itoa(d.DaysLeft)
			vm.Sub = i18n.T(ctx, "countdown.days", map[string]any{"Count": d.DaysLeft})
		}
		return vm
	})

	RegisterFormatter("clock", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.ClockData)
		return CellVM{Title: i18n.T(ctx, "widget.clock.label"), Big: d.TimeText, Sub: d.DateText}
	})

	RegisterFormatter("calendar", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.CalendarData)
		if (d.Mode == "month" || d.Mode == "week") && d.Month != nil {
			m := &MonthVM{Title: d.Month.Title, Weekdays: []string{"ma", "di", "wo", "do", "vr", "za", "zo"}}
			for _, wk := range d.Month.Weeks {
				row := make([]DayVM, 0, len(wk))
				for _, c := range wk {
					row = append(row, DayVM{Day: c.Day, InMonth: c.InMonth, Today: c.Today, Events: evItems(c.Events)})
				}
				m.Weeks = append(m.Weeks, row)
			}
			return CellVM{Month: m}
		}
		if d.Mode == "days" || d.Mode == "days_table" {
			sd := make([]ScheduleDayVM, 0, len(d.Days))
			for _, day := range d.Days {
				sd = append(sd, ScheduleDayVM{Label: day.Label, Today: day.Today, Events: evItems(day.Events)})
			}
			return CellVM{Schedule: sd, ScheduleTable: d.Mode == "days_table"}
		}
		if len(d.Events) == 0 {
			return CellVM{Sub: i18n.T(ctx, "widget.calendar.empty")}
		}
		vm := CellVM{}
		for _, e := range d.Events {
			vm.Agenda = append(vm.Agenda, EvVM{Text: e.When + "  " + e.Title, Color: e.Color})
		}
		return vm
	})

	RegisterFormatter("weather", func(_ context.Context, data any) CellVM {
		d, _ := data.(widget.WeatherData)
		return CellVM{Big: strconv.FormatFloat(d.TempC, 'f', 0, 64) + "°", Sub: wmoNL(d.Code)}
	})

	RegisterFormatter("quote", func(_ context.Context, data any) CellVM {
		d, _ := data.(widget.QuoteData)
		return CellVM{Body: d.Text, Sub: "— " + d.Author}
	})

	RegisterFormatter("web", func(_ context.Context, data any) CellVM {
		d, _ := data.(widget.WebData)
		return CellVM{IframeURL: d.URL}
	})

	RegisterFormatter("shopping", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.ShoppingData)
		if len(d.Items) == 0 {
			return CellVM{Sub: i18n.T(ctx, "widget.shopping.empty")}
		}
		return CellVM{Lines: d.Items}
	})

	RegisterFormatter("todolist", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.TodoData)
		if len(d.Items) == 0 {
			return CellVM{Sub: i18n.T(ctx, "widget.todolist.empty")}
		}
		return CellVM{Lines: d.Items}
	})

	RegisterFormatter("photos", func(ctx context.Context, data any) CellVM {
		d, _ := data.(widget.PhotosData)
		if len(d.URLs) == 0 {
			return CellVM{Sub: i18n.T(ctx, "widget.photos.empty")}
		}
		idx := 0
		switch d.Mode {
		case "random":
			idx = rand.IntN(len(d.URLs)) // re-rolled on every render (per display)
		case "by_date":
			// advance chronologically over time (~every 15s) without server state
			idx = int(time.Now().Unix()/15) % len(d.URLs)
		}
		return CellVM{ImageURL: d.URLs[idx]}
	})
}

// wmoNL maps WMO weather codes to short Dutch descriptions.
func wmoNL(code int) string {
	switch {
	case code == 0:
		return "Helder"
	case code <= 2:
		return "Licht bewolkt"
	case code == 3:
		return "Bewolkt"
	case code >= 45 && code <= 48:
		return "Mist"
	case code >= 51 && code <= 67:
		return "Regen"
	case code >= 71 && code <= 77:
		return "Sneeuw"
	case code >= 80 && code <= 82:
		return "Buien"
	case code >= 85 && code <= 86:
		return "Sneeuwbuien"
	case code >= 95:
		return "Onweer"
	default:
		return ""
	}
}
