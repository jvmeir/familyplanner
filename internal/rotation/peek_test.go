package rotation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeekUnknownDevice(t *testing.T) {
	m := NewManager()
	_, _, ok := m.Peek(42)
	require.False(t, ok, "no live stream -> not ok")
}

func TestPeekReflectsLiveState(t *testing.T) {
	m := NewManager()
	items := []Item{
		{ViewID: 10, Dwell: time.Second},
		{ViewID: 20, Dwell: time.Second},
	}
	_, _, release := m.Connect(7, items)
	defer release()

	// Fresh connection: first item, not paused.
	vid, paused, ok := m.Peek(7)
	require.True(t, ok)
	require.Equal(t, int64(10), vid)
	require.False(t, paused)

	// A pause command is reflected.
	require.True(t, m.Command(7, CmdPause))
	_, paused, _ = m.Peek(7)
	require.True(t, paused)

	// next advances the cursor (and pauses).
	require.True(t, m.Command(7, CmdNext))
	vid, paused, ok = m.Peek(7)
	require.True(t, ok)
	require.Equal(t, int64(20), vid)
	require.True(t, paused)
}

func TestPeekAfterRelease(t *testing.T) {
	m := NewManager()
	_, _, release := m.Connect(1, []Item{{ViewID: 5}})
	release()
	_, _, ok := m.Peek(1)
	require.False(t, ok, "released stream -> not ok")
}
