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

func TestDecideAnnounceOnHour(t *testing.T) {
	cfg := Config{Enabled: true, QuietStart: "22:00", QuietEnd: "07:00"}

	ch, ok := cfg.Decide(at(15, 0))
	require.True(t, ok)
	require.True(t, ch.Announce)
	require.Equal(t, "drie uur", ch.Text)

	ch, ok = cfg.Decide(at(15, 15))
	require.True(t, ok)
	require.False(t, ch.Announce, "quarter past = chime only")
	require.Empty(t, ch.Text)
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

func TestUntilNextQuarter(t *testing.T) {
	require.Equal(t, 5*time.Minute, UntilNextQuarter(at(15, 10)))
	require.Equal(t, 15*time.Minute, UntilNextQuarter(at(15, 0)))
	require.Equal(t, 1*time.Minute, UntilNextQuarter(at(15, 44)))
}
