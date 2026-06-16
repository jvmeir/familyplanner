package rotation_test

import (
	"testing"
	"time"

	"github.com/jvmeir/familyplanner/internal/rotation"
	"github.com/stretchr/testify/require"
)

func items() []rotation.Item {
	return []rotation.Item{
		{ViewID: 10, Dwell: 15 * time.Second},
		{ViewID: 20, Dwell: 10 * time.Second},
		{ViewID: 30, Dwell: 30 * time.Second},
	}
}

func curID(t *testing.T, s *rotation.State) int64 {
	t.Helper()
	it, ok := s.Current()
	require.True(t, ok)
	return it.ViewID
}

func TestNextPrevWrap(t *testing.T) {
	s := rotation.NewState(items())
	require.Equal(t, int64(10), curID(t, s))
	s.Next()
	require.Equal(t, int64(20), curID(t, s))
	s.Next()
	s.Next() // wrap
	require.Equal(t, int64(10), curID(t, s))
	s.Prev() // wrap backwards
	require.Equal(t, int64(30), curID(t, s))
}

func TestGoto(t *testing.T) {
	s := rotation.NewState(items())
	require.True(t, s.Goto(30))
	require.Equal(t, int64(30), curID(t, s))
	require.False(t, s.Goto(999))
}

func TestPause(t *testing.T) {
	s := rotation.NewState(items())
	require.False(t, s.Paused())
	s.SetPaused(true)
	require.True(t, s.Paused())
}

func TestEmptyPlaylist(t *testing.T) {
	s := rotation.NewState(nil)
	_, ok := s.Current()
	require.False(t, ok)
	s.Next() // must not panic
}

func TestManagerCommandsReachDevice(t *testing.T) {
	m := rotation.NewManager()

	// command to an unconnected device is a no-op
	require.False(t, m.Command(1, rotation.CmdNext))

	state, notify, release := m.Connect(1, items())
	defer release()

	require.True(t, m.Command(1, rotation.CmdNext))
	require.Equal(t, int64(20), curID(t, state))
	require.True(t, state.Paused(), "manual next pauses rotation")

	// the SSE loop would be woken via notify
	select {
	case <-notify:
	default:
		t.Fatal("expected a notify signal after a command")
	}

	require.True(t, m.Goto(1, 30))
	require.Equal(t, int64(30), curID(t, state))

	require.True(t, m.Command(1, rotation.CmdResume))
	require.False(t, state.Paused())
}

func TestDisconnectRemovesDevice(t *testing.T) {
	m := rotation.NewManager()
	_, _, release := m.Connect(2, items())
	require.True(t, m.Command(2, rotation.CmdNext))
	release()
	require.False(t, m.Command(2, rotation.CmdNext), "after release the device is gone")
}
