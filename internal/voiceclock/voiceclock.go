// Package voiceclock holds the pure logic for the kiosk's global voice clock: a
// quarter-hour chime plus a spoken Dutch time announcement on the hour. It is a
// kiosk-wide behaviour (not a placed widget) — the server decides when to emit a
// chime (respecting enabled + quiet hours) and the browser plays it, so all
// screens chime in sync and it is configured in one place.
package voiceclock

import (
	"time"
)

// Chime sounds (open / public-domain, synthesised in the browser).
const (
	SoundNone        = "none"
	SoundBingBong    = "bingbong"    // two-tone PA "bing-bong"
	SoundGong        = "gong"        // airport-style gong
	SoundPips        = "pips"        // short time-signal beeps
	SoundTimeSignal  = "timesignal"  // 3 pips, third double-length (speaking-clock)
	SoundWestminster = "westminster" // Big Ben quarter chimes (public domain)
)

// allSounds is every selectable sound (any beat may use any of them).
var allSounds = []string{SoundNone, SoundBingBong, SoundGong, SoundPips, SoundTimeSignal, SoundWestminster}

func validSound(s, def string) string {
	for _, a := range allSounds {
		if s == a {
			return s
		}
	}
	return def
}

// ValidQuarterSound normalises the :15/:45 sound (default bing-bong).
func ValidQuarterSound(s string) string { return validSound(s, SoundBingBong) }

// ValidHalfSound normalises the :30 sound (default bing-bong).
func ValidHalfSound(s string) string { return validSound(s, SoundBingBong) }

// ValidHourSound normalises the :00 sound (default time-signal).
func ValidHourSound(s string) string { return validSound(s, SoundTimeSignal) }

// Config is the (JSON-persisted) voice-clock setting. Each beat type has its own
// sound (set to "none" to silence it): the hour (:00), the half hour (:30) and
// the quarters (:15/:45). The hour may additionally read the time aloud.
type Config struct {
	Enabled      bool   `json:"enabled"`
	QuietStart   string `json:"quietStart"`   // "HH:MM", inclusive
	QuietEnd     string `json:"quietEnd"`     // "HH:MM", exclusive (wraps past midnight)
	QuarterSound string `json:"quarterSound"` // sound at :15 and :45
	HalfSound    string `json:"halfSound"`    // sound at :30
	HourSound    string `json:"hourSound"`    // sound at :00
	Announce     bool   `json:"announce"`     // speak the Dutch time on the hour
}

// Default is the seeded configuration: on, silent overnight; bing-bong on the
// quarters + half hour, time-signal + spoken time on the hour.
func Default() Config {
	return Config{
		Enabled: true, QuietStart: "22:00", QuietEnd: "07:00",
		QuarterSound: SoundBingBong, HalfSound: SoundBingBong, HourSound: SoundTimeSignal, Announce: true,
	}
}

// soundForQuarter returns the configured sound for beat q (0=:00, 1=:15, 2=:30,
// 3=:45), normalised.
func (c Config) soundForQuarter(q int) string {
	switch q {
	case 0:
		return ValidHourSound(c.HourSound)
	case 2:
		return ValidHalfSound(c.HalfSound)
	default:
		return ValidQuarterSound(c.QuarterSound)
	}
}

// Chime is the payload sent to the browser. The browser synthesises the sound;
// the server only decides when + what.
type Chime struct {
	Sound    string `json:"sound"`          // none|bingbong|gong|pips|timesignal|westminster
	Quarter  int    `json:"quarter"`        // 0 = top of hour, 1 = :15, 2 = :30, 3 = :45
	Hour     int    `json:"hour"`           // 0–23 (Westminster hour strikes)
	Announce bool   `json:"announce"`       // speak the time (top of the hour)
	Text     string `json:"text,omitempty"` // Dutch words, e.g. "drie uur"
}

// Decide returns the chime to emit at local time t (a quarter-hour boundary), or
// ok=false when disabled, in quiet hours, or nothing is configured for that beat.
func (c Config) Decide(t time.Time) (Chime, bool) {
	if !c.Enabled || c.InQuiet(t) {
		return Chime{}, false
	}
	q := (t.Minute() / 15) % 4
	sound := c.soundForQuarter(q)
	if q != 0 { // quarter past / half / quarter to
		if sound == SoundNone {
			return Chime{}, false
		}
		return Chime{Sound: sound, Quarter: q, Hour: t.Hour()}, true
	}
	// top of the hour: sound and/or spoken readout
	if sound == SoundNone && !c.Announce {
		return Chime{}, false
	}
	ch := Chime{Sound: sound, Quarter: 0, Hour: t.Hour(), Announce: c.Announce}
	if c.Announce {
		ch.Text = DutchHour(t.Hour())
	}
	return ch, true
}

// NextBoundary returns the next quarter-hour wall-clock instant strictly after
// now (:00/:15/:30/:45, seconds zeroed).
func NextBoundary(now time.Time) time.Time {
	q := now.Minute() / 15
	b := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), q*15, 0, 0, now.Location())
	if !b.After(now) {
		b = b.Add(15 * time.Minute)
	}
	return b
}

// MarkLead is how far BEFORE a beat a chime should start so its time-marking
// tone lands exactly on the beat. Only the speaking-clock time signal has such a
// tone (its long third pip, ~2s in); every other sound plays on the beat.
func MarkLead(sound string) time.Duration {
	if sound == SoundTimeSignal {
		return 2 * time.Second
	}
	return 0
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
