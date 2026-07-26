package widget

import (
	"testing"
	"time"
)

func TestBuildAbsWeeks(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, loc) // a Sunday
	cfg := CalendarConfig{Mode: "weeks_abs", StartDate: "2026-07-06", WeeksAhead: "3"}

	// An all-day event inside the pinned range should land on its day.
	ev := calEvent{t: dateOf(time.Date(2026, 7, 8, 0, 0, 0, 0, loc)), title: "Camping", allDay: true}
	grid := buildAbsWeeks(now, []calEvent{ev}, cfg)

	if len(grid.Weeks) != 3 {
		t.Fatalf("expected 3 weeks, got %d", len(grid.Weeks))
	}
	// 2026-07-06 is a Monday, so week 0 day 0 = the 6th.
	if grid.Weeks[0][0].Day != 6 {
		t.Errorf("week0 should start Mon 6 Jul, got day %d", grid.Weeks[0][0].Day)
	}
	// Wed 8 Jul is week0 index 2; the event should appear there.
	if len(grid.Weeks[0][2].Events) != 1 || grid.Weeks[0][2].Events[0].Text != "• Camping" {
		t.Errorf("event not placed on 8 Jul: %+v", grid.Weeks[0][2].Events)
	}
	// "now" (26 Jul, a Sunday) is in week 2 index 6 → highlighted.
	if !grid.Weeks[2][6].Today {
		t.Errorf("26 Jul should be marked Today")
	}
	if grid.Title == "" {
		t.Errorf("expected a range title")
	}
}
