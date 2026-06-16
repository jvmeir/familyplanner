package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// CalendarConfig is the per-instance configuration.
type CalendarConfig struct {
	URL         string `json:"url"`
	Mode        string `json:"mode"`         // "agenda" (default) | "month"
	WeeksBefore string `json:"weeks_before"` // agenda look-back (default 0)
	WeeksAhead  string `json:"weeks_ahead"`  // agenda look-ahead (default 2)
	Filter      string `json:"filter"`       // only events whose summary/CATEGORIES match (comma, any)
}

// CalendarEvent is one upcoming event, pre-formatted (agenda mode).
type CalendarEvent struct {
	When  string `json:"when"`
	Title string `json:"title"`
}

// DayCell is one day in the month grid.
type DayCell struct {
	Day     int      `json:"day"`
	InMonth bool     `json:"in_month"`
	Today   bool     `json:"today"`
	Events  []string `json:"events"`
}

// MonthGrid is a traditional month table (Monday-first weeks).
type MonthGrid struct {
	Title string      `json:"title"`
	Weeks [][]DayCell `json:"weeks"`
}

// ScheduleDay is one day in the relative day-by-day table (empty days included).
type ScheduleDay struct {
	Label  string   `json:"label"`
	Today  bool     `json:"today"`
	Events []string `json:"events"`
}

// CalendarData is the normalized render data; shape depends on Mode.
type CalendarData struct {
	Mode   string          `json:"mode"`
	Events []CalendarEvent `json:"events,omitempty"` // agenda
	Month  *MonthGrid      `json:"month,omitempty"`  // month
	Days   []ScheduleDay   `json:"days,omitempty"`   // days (relative table)
}

type calendarProvider struct {
	cfg     CalendarConfig
	sources []SourceInput
	now     NowFunc
}

func newCalendar(raw json.RawMessage, sources []SourceInput, now NowFunc) (Provider, error) {
	var cfg CalendarConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	if now == nil {
		now = time.Now
	}
	return calendarProvider{cfg: cfg, sources: sources, now: now}, nil
}

func decodeCalendar(raw json.RawMessage) (Data, error) {
	var d CalendarData
	err := json.Unmarshal(raw, &d)
	return d, err
}

var nlWeekday = map[time.Weekday]string{
	time.Monday: "ma", time.Tuesday: "di", time.Wednesday: "wo", time.Thursday: "do",
	time.Friday: "vr", time.Saturday: "za", time.Sunday: "zo",
}
var nlMonthShort = [...]string{"jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"}
var nlMonthFull = [...]string{"januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"}

type calEvent struct {
	t      time.Time
	title  string
	allDay bool
}

func (p calendarProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	now := p.now()
	loc := now.Location()
	// Expansion window for recurring events, generous enough for agenda + month.
	expandFrom := now.AddDate(0, 0, -70)
	expandTo := now.AddDate(0, 0, 70)

	// Gather iCal feeds from the linked data sources (each with its own filter),
	// falling back to a single URL in the widget config for backward compatibility.
	type feed struct{ url, filter string }
	type gsrc struct{ token, calID, filter string }
	var feeds []feed
	var graphs []gsrc
	for _, s := range p.sources {
		switch s.Type {
		case "ical":
			var c struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(s.Config, &c)
			if c.URL != "" {
				feeds = append(feeds, feed{c.URL, s.Filter})
			}
		case "ms_graph":
			var sec struct {
				AccessToken string `json:"access_token"`
			}
			_ = json.Unmarshal(s.Secret, &sec)
			var c struct {
				CalendarID string `json:"calendar_id"`
			}
			_ = json.Unmarshal(s.Config, &c)
			calID := c.CalendarID
			if s.Resource != "" {
				calID = s.Resource
			}
			if sec.AccessToken != "" {
				graphs = append(graphs, gsrc{sec.AccessToken, calID, s.Filter})
			}
		}
	}
	if len(feeds) == 0 && len(graphs) == 0 && p.cfg.URL != "" {
		feeds = append(feeds, feed{p.cfg.URL, p.cfg.Filter})
	}
	if len(feeds) == 0 && len(graphs) == 0 {
		return nil, 0, fmt.Errorf("calendar: no data sources configured")
	}

	var all []calEvent
	var firstErr error
	ok := 0
	for _, fd := range feeds {
		evs, err := fetchICS(ctx, fd.url, fd.filter, expandFrom, expandTo, loc)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, evs...)
		ok++
	}
	for _, g := range graphs {
		evs, err := GraphCalendar(ctx, g.token, g.calID, expandFrom, expandTo, loc)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		flt := parseFilter(g.filter)
		for _, e := range evs {
			if flt.match(func(prop string) string {
				if prop == "" {
					return e.title
				}
				return ""
			}) {
				all = append(all, e)
			}
		}
		ok++
	}
	if ok == 0 {
		return nil, 0, firstErr
	}

	switch p.cfg.Mode {
	case "month":
		return CalendarData{Mode: "month", Month: buildMonth(now, all)}, 15 * time.Minute, nil
	case "week":
		return CalendarData{Mode: "week", Month: buildWeekGrid(now, all, p.cfg)}, 15 * time.Minute, nil
	case "days", "days_table":
		return CalendarData{Mode: p.cfg.Mode, Days: buildSchedule(now, all, p.cfg)}, 15 * time.Minute, nil
	default:
		return CalendarData{Mode: "agenda", Events: buildAgenda(now, all, p.cfg)}, 15 * time.Minute, nil
	}
}

// buildWeekGrid renders weeks_before..weeks_ahead as full weeks (Monday-first),
// with each cell carrying the day's events (time-stamped or all-day "•").
func buildWeekGrid(now time.Time, all []calEvent, cfg CalendarConfig) *MonthGrid {
	before := atoiDefault(cfg.WeeksBefore, 0)
	ahead := atoiDefault(cfg.WeeksAhead, 1)
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	base := today.AddDate(0, 0, -before*7)
	offset := (int(base.Weekday()) + 6) % 7 // Monday = 0
	day := base.AddDate(0, 0, -offset)

	grid := &MonthGrid{Title: fmt.Sprintf("%s %d", nlMonthFull[int(now.Month())-1], now.Year())}
	for w := 0; w < before+ahead+1; w++ {
		var week []DayCell
		for d := 0; d < 7; d++ {
			cell := DayCell{Day: day.Day(), InMonth: true, Today: sameDate(day, now)}
			for _, e := range all {
				if sameDate(e.t, day) {
					if e.allDay {
						cell.Events = append(cell.Events, "• "+e.title)
					} else {
						cell.Events = append(cell.Events, fmt.Sprintf("%02d:%02d %s", e.t.Hour(), e.t.Minute(), e.title))
					}
				}
			}
			week = append(week, cell)
			day = day.AddDate(0, 0, 1)
		}
		grid.Weeks = append(grid.Weeks, week)
	}
	return grid
}

// fetchICS downloads and parses one iCal feed, expands recurrences, and applies
// the per-source filter.
func fetchICS(ctx context.Context, rawURL, filterStr string, expandFrom, expandTo time.Time, loc *time.Location) ([]calEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeICSURL(rawURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FamilyPlanner/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar: status %d", resp.StatusCode)
	}
	cal, err := ics.ParseCalendar(resp.Body)
	if err != nil {
		return nil, err
	}

	flt := parseFilter(filterStr)
	var out []calEvent
	for _, e := range cal.Events() {
		start, serr := e.GetStartAt()
		if serr != nil {
			continue
		}
		title := propVal(e, ics.ComponentPropertySummary)
		if !flt.match(func(prop string) string {
			if prop == "" {
				return title + " " + propVal(e, ics.ComponentProperty("CATEGORIES"))
			}
			return propVal(e, ics.ComponentProperty(strings.ToUpper(prop)))
		}) {
			continue
		}
		// All-day events use a date-only DTSTART (no time component).
		allDay := !strings.Contains(propVal(e, ics.ComponentPropertyDtStart), "T")
		if rule := propVal(e, ics.ComponentPropertyRrule); rule != "" {
			if occ := expandRecurring(rule, start, expandFrom, expandTo); len(occ) > 0 {
				for _, t := range occ {
					out = append(out, calEvent{t: t.In(loc), title: title, allDay: allDay})
				}
				continue
			}
		}
		out = append(out, calEvent{t: start.In(loc), title: title, allDay: allDay})
	}
	return out, nil
}

// buildSchedule lists every day in [now-weeksBefore, now+weeksAhead], including
// days with no events.
func buildSchedule(now time.Time, all []calEvent, cfg CalendarConfig) []ScheduleDay {
	before := atoiDefault(cfg.WeeksBefore, 0)
	ahead := atoiDefault(cfg.WeeksAhead, 1)
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -before*7)
	totalDays := (before+ahead)*7 + 1
	if totalDays > 60 {
		totalDays = 60
	}

	days := make([]ScheduleDay, 0, totalDays)
	for i := 0; i < totalDays; i++ {
		d := start.AddDate(0, 0, i)
		day := ScheduleDay{
			Label: fmt.Sprintf("%s %d %s", nlWeekday[d.Weekday()], d.Day(), nlMonthShort[int(d.Month())-1]),
			Today: sameDate(d, now),
		}
		for _, e := range all {
			if sameDate(e.t, d) {
				day.Events = append(day.Events, fmt.Sprintf("%02d:%02d %s", e.t.Hour(), e.t.Minute(), e.title))
			}
		}
		sort.Strings(day.Events)
		days = append(days, day)
	}
	return days
}

func buildAgenda(now time.Time, all []calEvent, cfg CalendarConfig) []CalendarEvent {
	before := atoiDefault(cfg.WeeksBefore, 0)
	ahead := atoiDefault(cfg.WeeksAhead, 4)
	from := now.AddDate(0, 0, -before*7)
	until := now.AddDate(0, 0, ahead*7)

	var evs []calEvent
	for _, e := range all {
		if e.t.Before(from) || e.t.After(until) {
			continue
		}
		evs = append(evs, e)
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].t.Before(evs[j].t) })
	if len(evs) > 10 {
		evs = evs[:10]
	}
	out := make([]CalendarEvent, 0, len(evs))
	for _, e := range evs {
		when := fmt.Sprintf("%s %d %s %02d:%02d",
			nlWeekday[e.t.Weekday()], e.t.Day(), nlMonthShort[int(e.t.Month())-1], e.t.Hour(), e.t.Minute())
		out = append(out, CalendarEvent{When: when, Title: e.title})
	}
	return out
}

func buildMonth(now time.Time, all []calEvent) *MonthGrid {
	loc := now.Location()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	offset := (int(first.Weekday()) + 6) % 7 // Monday = 0
	day := first.AddDate(0, 0, -offset)

	grid := &MonthGrid{Title: fmt.Sprintf("%s %d", nlMonthFull[int(now.Month())-1], now.Year())}
	for w := 0; w < 6; w++ {
		var week []DayCell
		anyInMonth := false
		for d := 0; d < 7; d++ {
			cell := DayCell{Day: day.Day(), InMonth: day.Month() == now.Month(), Today: sameDate(day, now)}
			if cell.InMonth {
				anyInMonth = true
			}
			for _, e := range all {
				if sameDate(e.t, day) {
					cell.Events = append(cell.Events, e.title)
				}
			}
			week = append(week, cell)
			day = day.AddDate(0, 0, 1)
		}
		if anyInMonth {
			grid.Weeks = append(grid.Weeks, week)
		}
	}
	return grid
}

// normalizeICSURL maps calendar-subscription schemes to https so feeds shared
// as webcal:// links work.
func normalizeICSURL(u string) string {
	switch {
	case strings.HasPrefix(u, "webcal://"):
		return "https://" + strings.TrimPrefix(u, "webcal://")
	case strings.HasPrefix(u, "webcals://"):
		return "https://" + strings.TrimPrefix(u, "webcals://")
	}
	return u
}

// expandRecurring computes occurrences of an RRULE within [from, to].
func expandRecurring(rule string, dtstart, from, to time.Time) []time.Time {
	opt, err := rrule.StrToROption(rule)
	if err != nil {
		return nil
	}
	opt.Dtstart = dtstart
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil
	}
	return r.Between(from, to, true)
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 {
		return n
	}
	return def
}

func propVal(e *ics.VEvent, name ics.ComponentProperty) string {
	if pr := e.GetProperty(name); pr != nil {
		return pr.Value
	}
	return ""
}

// filterTerm matches when the named iCal property (empty = summary+categories)
// contains val (case-insensitive).
type filterTerm struct {
	prop string
	val  string
}

// filterClause is an AND of terms; filter is an OR of clauses.
type filterClause []filterTerm
type filter []filterClause

// parseFilter parses a filter string. Comma = OR between clauses; whitespace =
// AND within a clause; "prop:value" targets a property, a bare token targets
// summary+categories. Examples:
//
//	"sport"                      -> summary/categories contains "sport"
//	"location:school, sport"     -> location contains "school"  OR  contains "sport"
//	"categories:sport voetbal"   -> categories~sport AND summary/cats~voetbal
func parseFilter(s string) filter {
	var f filter
	for _, clauseStr := range strings.Split(s, ",") {
		var clause filterClause
		for _, tok := range strings.Fields(clauseStr) {
			prop, val := "", strings.ToLower(tok)
			if i := strings.Index(tok, ":"); i > 0 {
				prop = strings.ToLower(tok[:i])
				val = strings.ToLower(tok[i+1:])
			}
			if val != "" {
				clause = append(clause, filterTerm{prop: prop, val: val})
			}
		}
		if len(clause) > 0 {
			f = append(f, clause)
		}
	}
	return f
}

// match reports whether an event (whose property values are read via get)
// satisfies the filter. No clauses = match everything.
func (f filter) match(get func(prop string) string) bool {
	if len(f) == 0 {
		return true
	}
	for _, clause := range f {
		ok := true
		for _, t := range clause {
			if !strings.Contains(strings.ToLower(get(t.prop)), t.val) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
