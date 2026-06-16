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

func TestRegistryUnknownType(t *testing.T) {
	reg := widget.NewRegistry()
	_, ok := reg.Get("nope")
	require.False(t, ok)
}
