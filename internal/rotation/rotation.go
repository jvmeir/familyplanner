// Package rotation drives per-device playlist playback: which view is showing,
// auto-advance on the configured interval, and manual prev/next/pause/jump.
//
// State is the pure, testable playback state for one device. Manager tracks the
// live (connected) devices so commands from a phone "remote" can reach the TV's
// open SSE stream.
package rotation

import (
	"sync"
	"time"
)

// Item is one entry in a resolved playlist: a view and its effective dwell.
type Item struct {
	ViewID int64
	Dwell  time.Duration
}

// State is the playback cursor over a playlist. All methods are safe for
// concurrent use (the SSE loop reads while a control request mutates).
type State struct {
	mu     sync.Mutex
	items  []Item
	index  int
	paused bool
}

// NewState creates playback state for the given (ordered) items.
func NewState(items []Item) *State { return &State{items: items} }

// Current returns the item at the cursor (false if the playlist is empty).
func (s *State) Current() (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return Item{}, false
	}
	if s.index < 0 || s.index >= len(s.items) {
		s.index = 0
	}
	return s.items[s.index], true
}

// Next advances the cursor (wrapping).
func (s *State) Next() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) > 0 {
		s.index = (s.index + 1) % len(s.items)
	}
}

// Prev moves the cursor back (wrapping).
func (s *State) Prev() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) > 0 {
		s.index = (s.index - 1 + len(s.items)) % len(s.items)
	}
}

// Goto jumps to the item with viewID. Returns false if it isn't in the playlist.
func (s *State) Goto(viewID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.items {
		if it.ViewID == viewID {
			s.index = i
			return true
		}
	}
	return false
}

// SetPaused sets the paused flag.
func (s *State) SetPaused(p bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = p
}

// Paused reports whether auto-advance is paused.
func (s *State) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Command is a control action issued to a device.
type Command string

const (
	CmdNext   Command = "next"
	CmdPrev   Command = "prev"
	CmdPause  Command = "pause"
	CmdResume Command = "resume"
)

// Manager tracks connected devices so external commands reach their SSE loops.
type Manager struct {
	mu      sync.Mutex
	devices map[int64]*conn
}

type conn struct {
	state  *State
	notify chan struct{}
}

// NewManager creates an empty manager.
func NewManager() *Manager { return &Manager{devices: make(map[int64]*conn)} }

// Connect registers a device's live stream with its resolved playlist items.
// It returns the playback State, a notify channel that fires when a command
// mutates the state, and a release func to call when the stream ends.
func (m *Manager) Connect(deviceID int64, items []Item) (*State, <-chan struct{}, func()) {
	c := &conn{state: NewState(items), notify: make(chan struct{}, 1)}
	m.mu.Lock()
	m.devices[deviceID] = c
	m.mu.Unlock()

	release := func() {
		m.mu.Lock()
		if m.devices[deviceID] == c {
			delete(m.devices, deviceID)
		}
		m.mu.Unlock()
	}
	return c.state, c.notify, release
}

// Command applies a prev/next/pause/resume to a connected device. Manual
// next/prev/pause pause auto-advance (resume re-enables it). Returns false if
// the device has no live stream.
func (m *Manager) Command(deviceID int64, cmd Command) bool {
	c := m.lookup(deviceID)
	if c == nil {
		return false
	}
	switch cmd {
	case CmdNext:
		c.state.Next()
		c.state.SetPaused(true)
	case CmdPrev:
		c.state.Prev()
		c.state.SetPaused(true)
	case CmdPause:
		c.state.SetPaused(true)
	case CmdResume:
		c.state.SetPaused(false)
	default:
		return false
	}
	m.signal(c)
	return true
}

// Goto jumps a connected device to a specific view and pauses rotation.
func (m *Manager) Goto(deviceID, viewID int64) bool {
	c := m.lookup(deviceID)
	if c == nil {
		return false
	}
	if !c.state.Goto(viewID) {
		return false
	}
	c.state.SetPaused(true)
	m.signal(c)
	return true
}

func (m *Manager) lookup(deviceID int64) *conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.devices[deviceID]
}

func (m *Manager) signal(c *conn) {
	select {
	case c.notify <- struct{}{}:
	default: // a signal is already pending; coalesce
	}
}
