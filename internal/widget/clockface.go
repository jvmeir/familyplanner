package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// wikiOnThisDayBase is overridable in tests. Wikipedia's free on-this-day feed.
var wikiOnThisDayBase = "https://nl.wikipedia.org/api/rest_v1/feed/onthisday/events"

// ClockFaceConfig is the per-instance configuration for the analog clock face. Sun
// times come from Open-Meteo for the given place (reuses the weather geocoder).
type ClockFaceConfig struct {
	Location string `json:"location"` // place/address; geocoded (wins over lat/lon)
	Lat      string `json:"lat"`
	Lon      string `json:"lon"`
	Facts    string `json:"facts"` // "yes" = show a rotating "op deze dag" fact
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

// onThisDay fetches Wikipedia's Dutch on-this-day events for t (best-effort).
func onThisDay(ctx context.Context, t time.Time) []string {
	u := fmt.Sprintf("%s/%02d/%02d", wikiOnThisDayBase, int(t.Month()), t.Day())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		Events []struct {
			Year int    `json:"year"`
			Text string `json:"text"`
		} `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	out := make([]string, 0, len(body.Events))
	for _, e := range body.Events {
		if e.Text == "" {
			continue
		}
		out = append(out, strconv.Itoa(e.Year)+" – "+e.Text)
	}
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}
