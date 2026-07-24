package voiceclock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func at(h, m int) time.Time { return time.Date(2026, 7, 21, h, m, 0, 0, time.UTC) }

func TestDutchHour(t *testing.T) {
	require.Equal(t, "drie uur", DutchHour(3))
	require.Equal(t, "drie uur", DutchHour(15)) // 12h spoken form
	require.Equal(t, "twaalf uur", DutchHour(0))
	require.Equal(t, "twaalf uur", DutchHour(12))
	require.Equal(t, "één uur", DutchHour(13))
	require.Equal(t, "elf uur", DutchHour(23))
}

func TestDecideHourVsQuarter(t *testing.T) {
	cfg := Config{
		Enabled: true, QuietStart: "22:00", QuietEnd: "07:00",
		QuarterSound: SoundBingBong, HourSound: SoundTimeSignal, Announce: true,
	}

	ch, ok := cfg.Decide(at(15, 0)) // top of the hour
	require.True(t, ok)
	require.Equal(t, SoundTimeSignal, ch.Sound)
	require.True(t, ch.Announce)
	require.Equal(t, "drie uur", ch.Text)
	require.Equal(t, 0, ch.Quarter)
	require.Equal(t, 15, ch.Hour)

	ch, ok = cfg.Decide(at(15, 15)) // quarter past
	require.True(t, ok)
	require.Equal(t, SoundBingBong, ch.Sound)
	require.False(t, ch.Announce)
	require.Empty(t, ch.Text)
	require.Equal(t, 1, ch.Quarter)

	ch, _ = cfg.Decide(at(15, 45))
	require.Equal(t, 3, ch.Quarter)
}

func TestDecideNoneSounds(t *testing.T) {
	cfg := Config{Enabled: true, QuarterSound: SoundNone, HourSound: SoundNone, Announce: false}
	// Quarter sound "none" -> no quarter chime.
	_, ok := cfg.Decide(at(15, 15))
	require.False(t, ok)
	// Hour sound "none" + no announce -> nothing on the hour.
	_, ok = cfg.Decide(at(15, 0))
	require.False(t, ok)
	// Hour sound "none" but announce on -> voice-only event.
	cfg.Announce = true
	ch, ok := cfg.Decide(at(15, 0))
	require.True(t, ok)
	require.Equal(t, SoundNone, ch.Sound)
	require.True(t, ch.Announce)
}

func TestValidSoundsAndDefaults(t *testing.T) {
	require.Equal(t, SoundBingBong, ValidQuarterSound(""))
	require.Equal(t, SoundTimeSignal, ValidQuarterSound("timesignal")) // any sound allowed per beat now
	require.Equal(t, SoundWestminster, ValidQuarterSound(SoundWestminster))
	require.Equal(t, SoundBong, ValidHalfSound("")) // half hour defaults to a single tone
	require.Equal(t, SoundBing, ValidHalfSound(SoundBing))
	require.Equal(t, SoundTimeSignal, ValidHourSound(""))
	require.Equal(t, SoundTimeSignal, ValidHourSound("bogus"))
	require.Equal(t, SoundGong, ValidHourSound(SoundGong))
	require.Equal(t, SoundBingBong, Default().QuarterSound)
	require.Equal(t, SoundBong, Default().HalfSound)
	require.Equal(t, SoundTimeSignal, Default().HourSound)
}

func TestDecideHalfHourUsesHalfSound(t *testing.T) {
	cfg := Config{Enabled: true, QuarterSound: SoundBingBong, HalfSound: SoundGong, HourSound: SoundTimeSignal}
	ch, ok := cfg.Decide(at(15, 30))
	require.True(t, ok)
	require.Equal(t, SoundGong, ch.Sound) // :30 uses the dedicated half-hour sound
	require.Equal(t, 2, ch.Quarter)
	// A half-hour sound of "none" silences only :30.
	cfg.HalfSound = SoundNone
	_, ok = cfg.Decide(at(15, 30))
	require.False(t, ok)
	_, ok = cfg.Decide(at(15, 15)) // quarters still chime
	require.True(t, ok)
}

func TestNextBoundaryAndLead(t *testing.T) {
	require.Equal(t, at(15, 15), NextBoundary(at(15, 10)))
	require.Equal(t, at(15, 15), NextBoundary(at(15, 0))) // strictly after
	require.Equal(t, at(16, 0), NextBoundary(at(15, 45))) // rolls to next hour
	// Lead: pips-only leads ~2s; other sounds play on the beat; a spoken
	// announcement auto-estimates a larger lead (attention chime + phrase + pips).
	require.Equal(t, 2*time.Second, Chime{Sound: SoundTimeSignal}.Lead())
	require.Equal(t, time.Duration(0), Chime{Sound: SoundBingBong}.Lead())
	spoken := Chime{Sound: SoundTimeSignal, Announce: true, Attention: true, Text: "drie uur", Rate: 0.7}.Lead()
	require.Greater(t, spoken, 8*time.Second)
	require.Less(t, spoken, 25*time.Second)
}

func TestDecideDisabled(t *testing.T) {
	_, ok := Config{Enabled: false}.Decide(at(15, 0))
	require.False(t, ok)
}

func TestDecideQuietHours(t *testing.T) {
	cfg := Config{Enabled: true, QuietStart: "22:00", QuietEnd: "07:00"}
	_, ok := cfg.Decide(at(23, 0)) // overnight quiet
	require.False(t, ok)
	_, ok = cfg.Decide(at(3, 0))
	require.False(t, ok)
	_, ok = cfg.Decide(at(7, 0)) // end is exclusive -> active again
	require.True(t, ok)
	_, ok = cfg.Decide(at(21, 45))
	require.True(t, ok)
}

func TestInQuietNonWrapping(t *testing.T) {
	cfg := Config{Enabled: true, QuietStart: "13:00", QuietEnd: "14:00"}
	require.True(t, cfg.InQuiet(at(13, 30)))
	require.False(t, cfg.InQuiet(at(12, 59)))
	require.False(t, cfg.InQuiet(at(14, 0)))
}

func TestInQuietBadBounds(t *testing.T) {
	require.False(t, Config{QuietStart: "", QuietEnd: ""}.InQuiet(at(3, 0)))
	require.False(t, Config{QuietStart: "08:00", QuietEnd: "08:00"}.InQuiet(at(8, 0)))
}
