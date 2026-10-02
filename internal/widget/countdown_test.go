package widget_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jvmeir/familyplanner/internal/widget"
	"github.com/stretchr/testify/require"
)

func fixedNow(s string) widget.NowFunc {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return func() time.Time { return t }
}

func TestCountdownMath(t *testing.T) {
	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	typ, ok := reg.Get("countdown")
	require.True(t, ok)

	cases := []struct {
		name      string
		date      string
		now       string
		wantDays  int
		wantToday bool
	}{
		{"five days ahead", "2026-06-04", "2026-05-30", 5, false},
		{"today", "2026-05-30", "2026-05-30", 0, true},
		{"tomorrow", "2026-05-31", "2026-05-30", 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := json.Marshal(widget.CountdownConfig{Title: "X", Date: tc.date})
			p, err := typ.NewProvider(cfg, nil, fixedNow(tc.now))
			require.NoError(t, err)

			data, ttl, err := p.Fetch(context.Background())
			require.NoError(t, err)
			require.Equal(t, time.Hour, ttl)

			cd := data.(widget.CountdownData)
			require.Equal(t, tc.wantDays, cd.DaysLeft)
			require.Equal(t, tc.wantToday, cd.Today)
		})
	}
}

func TestCountdownRejectsBadDate(t *testing.T) {
	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	typ, _ := reg.Get("countdown")

	cfg, _ := json.Marshal(widget.CountdownConfig{Title: "X", Date: "not-a-date"})
	p, err := typ.NewProvider(cfg, nil, fixedNow("2026-05-30"))
	require.NoError(t, err)

	_, _, err = p.Fetch(context.Background())
	require.Error(t, err)
}

func TestCountdownLocalCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Brussels")
	require.NoError(t, err)
	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	typ, _ := reg.Get("countdown")
	for _, tc := range []struct {
		name, now, date string
		days            int
		ttl             time.Duration
	}{
		{"spring forward tomorrow", "2026-03-29 12:00", "2026-03-30", 1, time.Hour},
		{"spring forward yesterday", "2026-03-30 12:00", "2026-03-29", -1, time.Hour},
		{"fall back tomorrow", "2026-10-25 12:00", "2026-10-26", 1, time.Hour},
		{"across summer time", "2026-03-28 12:00", "2026-04-02", 5, time.Hour},
		{"midnight refresh", "2026-10-02 23:55", "2026-10-03", 1, 5 * time.Minute},
		{"today with later target time", "2026-10-02 12:00", "2026-10-02", 0, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.ParseInLocation("2006-01-02 15:04", tc.now, loc)
			require.NoError(t, err)
			cfg, err := json.Marshal(widget.CountdownConfig{Date: tc.date, Time: "18:30", Precision: "dhms"})
			require.NoError(t, err)
			p, err := typ.NewProvider(cfg, nil, func() time.Time { return now })
			require.NoError(t, err)
			data, ttl, err := p.Fetch(context.Background())
			require.NoError(t, err)
			cd := data.(widget.CountdownData)
			require.Equal(t, tc.days, cd.DaysLeft)
			require.Equal(t, tc.days == 0, cd.Today)
			require.Equal(t, tc.ttl, ttl)
			target, err := time.ParseInLocation("2006-01-02 15:04", tc.date+" 18:30", loc)
			require.NoError(t, err)
			require.Equal(t, target.Unix(), cd.TargetUnix)
			require.Equal(t, "dhms", cd.Precision)
		})
	}
}

func TestRegistryUnknownType(t *testing.T) {
	reg := widget.NewRegistry()
	_, ok := reg.Get("nope")
	require.False(t, ok)
}
