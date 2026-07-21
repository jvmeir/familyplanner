package widget

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildAgendaMarksToday(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	// Agenda shows events from now onward, so both are later than `now`.
	all := []calEvent{
		{t: now.Add(2 * time.Hour), title: "Tandarts"}, // today, later
		{t: now.AddDate(0, 0, 1), title: "Voetbal"},    // tomorrow
	}
	out := buildAgenda(now, all, CalendarConfig{WeeksAhead: "2"})
	require.Len(t, out, 2)

	byTitle := map[string]CalendarEvent{}
	for _, e := range out {
		byTitle[e.Title] = e
	}
	require.True(t, byTitle["Tandarts"].Today, "same-day event flagged today")
	require.False(t, byTitle["Voetbal"].Today, "tomorrow's event not flagged today")
}
