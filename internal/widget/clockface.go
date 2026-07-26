package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// nlWikiAPIBase is overridable in tests. The Dutch Wikipedia REST "on this day"
// feed does not exist (nl is unsupported), so we parse the date page's
// "Gebeurtenissen" section from the MediaWiki action API instead.
var nlWikiAPIBase = "https://nl.wikipedia.org/w/api.php"

// nlMonths maps 1..12 to the Dutch month name used in the date page title.
var nlMonths = []string{"", "januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"}

var (
	reWikiComment  = regexp.MustCompile(`(?s)<!--.*?-->`)
	reWikiRef      = regexp.MustCompile(`(?s)<ref[^>]*/>|<ref[^>]*>.*?</ref>`)
	reWikiTemplate = regexp.MustCompile(`\{\{[^{}]*\}\}`)
	reWikiTag      = regexp.MustCompile(`<[^>]+>`)
	reWikiLinkAlt  = regexp.MustCompile(`\[\[[^\]|]*\|([^\]]*)\]\]`)
	reWikiLink     = regexp.MustCompile(`\[\[([^\]]*)\]\]`)
	reWikiBold     = regexp.MustCompile(`'{2,}`)
	reWikiWS       = regexp.MustCompile(`\s+`)
	reEventLine    = regexp.MustCompile(`^(\d{1,4})\s*[-–—]\s*(.+)$`)
)

// ClockFaceConfig is the per-instance configuration for the analog clock face. Sun
// times come from Open-Meteo for the given place (reuses the weather geocoder).
type ClockFaceConfig struct {
	Location string `json:"location"` // place/address; geocoded (wins over lat/lon)
	Lat      string `json:"lat"`
	Lon      string `json:"lon"`
	Facts    string `json:"facts"`        // "yes" = show a rotating "op deze dag" fact
	Announce string `json:"announce_nav"` // "yes" = jump to this screen on a voice-clock announcement
}

// ClockFaceData is the normalized render data: sun marks, moon phase, and (optional)
// on-this-day facts. The hands/date/week/countdown are computed live client-side.
type ClockFaceData struct {
	Place      string   `json:"place,omitempty"`
	SunriseMin int      `json:"sunrise_min"` // minutes from local midnight (-1 = unknown)
	SunsetMin  int      `json:"sunset_min"`
	MoonIcon   string   `json:"moon_icon"`
	MoonName   string   `json:"moon_name"`
	Facts      []string `json:"facts,omitempty"`
}

type clockfaceProvider struct {
	cfg ClockFaceConfig
	now NowFunc
}

func newClockface(raw json.RawMessage, _ []SourceInput, now NowFunc) (Provider, error) {
	var cfg ClockFaceConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	if now == nil {
		now = time.Now
	}
	return clockfaceProvider{cfg: cfg, now: now}, nil
}

func decodeClockface(raw json.RawMessage) (Data, error) {
	var d ClockFaceData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p clockfaceProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	now := p.now()
	icon, name := moonPhase(now)
	out := ClockFaceData{SunriseMin: -1, SunsetMin: -1, MoonIcon: icon, MoonName: name}

	// Sun times (best-effort; the clock still works without them).
	lat, lon, place := p.cfg.Lat, p.cfg.Lon, ""
	if p.cfg.Location != "" {
		if gLat, gLon, gPlace, err := geocode(ctx, p.cfg.Location); err == nil {
			lat, lon, place = gLat, gLon, gPlace
		}
	}
	if lat == "" {
		lat = "50.85" // Brussels
	}
	if lon == "" {
		lon = "4.35"
	}
	out.Place = place
	if sr, ss, err := sunTimes(ctx, lat, lon); err == nil {
		out.SunriseMin, out.SunsetMin = sr, ss
	}

	if p.cfg.Facts == "yes" {
		out.Facts = onThisDay(ctx, now)
	}
	return out, time.Hour, nil
}

// sunTimes fetches today's sunrise/sunset from Open-Meteo, as minutes from local
// midnight.
func sunTimes(ctx context.Context, lat, lon string) (sunrise, sunset int, err error) {
	u := fmt.Sprintf("%s?latitude=%s&longitude=%s&daily=sunrise,sunset&timezone=auto&forecast_days=1", openMeteoBase, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return -1, -1, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return -1, -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1, -1, fmt.Errorf("suntimes: status %d", resp.StatusCode)
	}
	var body struct {
		Daily struct {
			Sunrise []string `json:"sunrise"`
			Sunset  []string `json:"sunset"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return -1, -1, err
	}
	return hhmmMinutes(body.Daily.Sunrise), hhmmMinutes(body.Daily.Sunset), nil
}

// hhmmMinutes parses the first "2006-01-02T15:04" value → minutes from midnight.
func hhmmMinutes(v []string) int {
	if len(v) == 0 {
		return -1
	}
	t, err := time.Parse("2006-01-02T15:04", v[0])
	if err != nil {
		return -1
	}
	return t.Hour()*60 + t.Minute()
}

// moonPhase returns an emoji + Dutch name for the moon phase on t (simple synodic
// approximation from a known new moon).
func moonPhase(t time.Time) (icon, name string) {
	known := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	const synodic = 29.530588853
	days := t.UTC().Sub(known).Hours() / 24
	age := math.Mod(days, synodic)
	if age < 0 {
		age += synodic
	}
	idx := int(age/synodic*8+0.5) % 8
	icons := []string{"🌑", "🌒", "🌓", "🌔", "🌕", "🌖", "🌗", "🌘"}
	names := []string{"Nieuwe maan", "Wassende sikkel", "Eerste kwartier", "Wassende maan", "Volle maan", "Afnemende maan", "Laatste kwartier", "Afnemende sikkel"}
	return icons[idx], names[idx]
}

// onThisDay returns Dutch "op deze dag" events for t (best-effort). The Dutch
// Wikipedia has no on-this-day REST feed, so we fetch the date page (e.g.
// "26 juli") via the MediaWiki action API and parse its "Gebeurtenissen" section.
func onThisDay(ctx context.Context, t time.Time) []string {
	page := fmt.Sprintf("%d %s", t.Day(), nlMonths[int(t.Month())])
	q := url.Values{
		"action":        {"parse"},
		"page":          {page},
		"prop":          {"wikitext"},
		"format":        {"json"},
		"formatversion": {"2"},
		"redirects":     {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nlWikiAPIBase+"?"+q.Encode(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "FamilyPlanner/1.0 (self-hosted family kiosk)")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var body struct {
		Parse struct {
			Wikitext string `json:"wikitext"`
		} `json:"parse"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	return parseEvents(body.Parse.Wikitext)
}

// parseEvents extracts "YEAR – text" strings from the "Gebeurtenissen" section of
// a Dutch Wikipedia date page's wikitext.
func parseEvents(wikitext string) []string {
	start := strings.Index(wikitext, "== Gebeurtenissen ==")
	if start < 0 {
		return nil
	}
	section := wikitext[start:]
	// Cut at the next level-2 heading ("\n== "); "=== " subheadings don't match.
	if end := strings.Index(section[len("== Gebeurtenissen =="):], "\n== "); end >= 0 {
		section = section[:len("== Gebeurtenissen ==")+end]
	}
	out := make([]string, 0, 25)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "*"))
		if line == "" || strings.HasPrefix(line, "{{") {
			continue
		}
		clean := cleanWiki(line)
		m := reEventLine.FindStringSubmatch(clean)
		if m == nil {
			continue
		}
		out = append(out, m[1]+" – "+strings.TrimSpace(m[2]))
		if len(out) >= 25 {
			break
		}
	}
	return out
}

// cleanWiki strips MediaWiki markup (templates, refs, links, bold) to plain text.
func cleanWiki(s string) string {
	s = reWikiComment.ReplaceAllString(s, "")
	s = reWikiRef.ReplaceAllString(s, "")
	s = reWikiTemplate.ReplaceAllString(s, "")
	s = reWikiLinkAlt.ReplaceAllString(s, "$1") // [[a|b]] -> b
	s = reWikiLink.ReplaceAllString(s, "$1")    // [[a]]   -> a
	s = reWikiTag.ReplaceAllString(s, "")
	s = reWikiBold.ReplaceAllString(s, "")
	return strings.TrimSpace(reWikiWS.ReplaceAllString(s, " "))
}
