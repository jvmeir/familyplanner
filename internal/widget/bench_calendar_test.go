package widget

import (
	"fmt"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
)

// synthFeed builds an iCal feed with n events, every third one weekly-recurring,
// approximating a busy family calendar.
func synthFeed(n int) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//bench//EN\r\n")
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		t := day.AddDate(0, 0, i%60)
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:e%d@bench\r\nDTSTAMP:20260901T090000Z\r\n"+
			"SUMMARY:Event %d\r\nCATEGORIES:sport\r\nDTSTART:%s\r\nDTEND:%s\r\n",
			i, i, t.Format("20060102T150405Z"), t.Add(time.Hour).Format("20060102T150405Z"))
		if i%3 == 0 {
			b.WriteString("RRULE:FREQ=WEEKLY;COUNT=52\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func BenchmarkICSParse200(b *testing.B) {
	feed := synthFeed(200)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ics.ParseCalendar(strings.NewReader(feed)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpandRecurringWeeklyYear(b *testing.B) {
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	from := start.AddDate(0, 0, -7)
	to := start.AddDate(1, 0, 0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if occ := expandRecurring("FREQ=WEEKLY;COUNT=52", start, from, to); len(occ) == 0 {
			b.Fatal("no occurrences")
		}
	}
}

// BenchmarkFilterMatch measures the per-event filter cost on the hot loop.
func BenchmarkFilterMatch(b *testing.B) {
	f := parseFilter("class:private, categories:sport")
	get := func(prop string) string {
		switch prop {
		case "class":
			return "PRIVATE"
		case "categories":
			return "sport"
		}
		return "Voetbaltraining Jane"
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.match(get)
	}
}
