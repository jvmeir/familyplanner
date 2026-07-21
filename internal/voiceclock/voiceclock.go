// Package voiceclock holds the pure logic for the kiosk's global voice clock: a
// quarter-hour chime plus a spoken Dutch time announcement on the hour. It is a
// kiosk-wide behaviour (not a placed widget) — the server decides when to emit a
// chime (respecting enabled + quiet hours) and the browser plays it, so all
// screens chime in sync and it is configured in one place.
package voiceclock

import (
	"time"
)

// Chime styles (open / public-domain sounds synthesised in the browser).
const (
	StyleSpeakingClock = "sprekende_klok" // bing-bong on quarters; "bij de derde toon…" + 3 pips on the hour
	StyleWestminster   = "westminster"    // Big Ben quarter chimes (public domain)
	StyleGong          = "gong"           // generic airport/PA "bing-bong"
	StylePips          = "pips"           // time-signal beeps
)

// ValidStyle normalises an unknown/empty style to the default.
func ValidStyle(s string) string {
	switch s {
	case StyleSpeakingClock, StyleWestminster, StyleGong, StylePips:
		return s
	default:
		return StyleSpeakingClock
	}
}

// Config is the (JSON-persisted) voice-clock setting.
type Config struct {
	Enabled    bool   `json:"enabled"`
	QuietStart string `json:"quietStart"` // "HH:MM", inclusive
	QuietEnd   string `json:"quietEnd"`   // "HH:MM", exclusive (wraps past midnight)
	ChimeStyle string `json:"chimeStyle"` // westminster | gong | pips
}

// Default is the seeded configuration: on, silent overnight, speaking-clock chime.
func Default() Config {
	return Config{Enabled: true, QuietStart: "22:00", QuietEnd: "07:00", ChimeStyle: StyleSpeakingClock}
}

// Chime is the payload sent to the browser on a quarter-hour. The browser
// synthesises the sound; the server only decides when + what.
type Chime struct {
	Style    string `json:"style"`          // westminster | gong | pips
	Quarter  int    `json:"quarter"`        // 0 = top of hour, 1 = :15, 2 = :30, 3 = :45
	Hour     int    `json:"hour"`           // 0–23 (Westminster hour strikes)
	Announce bool   `json:"announce"`       // speak the time (top of the hour)
	Text     string `json:"text,omitempty"` // Dutch words, e.g. "drie uur"
}

// Decide reports whether to emit a chime at local time t (called at a
// quarter-hour boundary) and its payload. Returns ok=false when disabled or
// within quiet hours.
func (c Config) Decide(t time.Time) (Chime, bool) {
	if !c.Enabled || c.InQuiet(t) {
		return Chime{}, false
	}
	ch := Chime{
		Style:   ValidStyle(c.ChimeStyle),
		Quarter: (t.Minute() / 15) % 4,
		Hour:    t.Hour(),
	}
	if t.Minute() == 0 {
		ch.Announce = true
		ch.Text = DutchHour(t.Hour())
	}
	return ch, true
}

// InQuiet reports whether t's wall-clock time falls in the quiet window. A start
// later than end (e.g. 22:00→07:00) wraps past midnight. Unparseable bounds or
// start==end mean "never quiet".
func (c Config) InQuiet(t time.Time) bool {
	start, ok1 := parseHM(c.QuietStart)
	end, ok2 := parseHM(c.QuietEnd)
	if !ok1 || !ok2 || start == end {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	if start < end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end // wraps midnight
}

func parseHM(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// UntilNextQuarter returns the duration from now to the next :00/:15/:30/:45
// wall-clock boundary.
func UntilNextQuarter(now time.Time) time.Duration {
	elapsed := time.Duration(now.Minute()%15)*time.Minute +
		time.Duration(now.Second())*time.Second +
		time.Duration(now.Nanosecond())
	d := 15*time.Minute - elapsed
	if d <= 0 {
		d = 15 * time.Minute
	}
	return d
}

var dutchHours = [...]string{
	"twaalf", "één", "twee", "drie", "vier", "vijf",
	"zes", "zeven", "acht", "negen", "tien", "elf",
}

// DutchHour renders a 24h hour as spoken 12h Dutch + " uur" (e.g. 15 -> "drie uur").
func DutchHour(h int) string {
	return dutchHours[((h%12)+12)%12] + " uur"
}
