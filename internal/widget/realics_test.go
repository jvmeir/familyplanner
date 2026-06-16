package widget

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRealICS is a diagnostic that runs only when FP_ICS_URL is set, so it
// never runs in CI. It reports how many agenda events the real feed yields
// (counts only — no event content is logged).
func TestRealICS(t *testing.T) {
	url := os.Getenv("FP_ICS_URL")
	if url == "" {
		t.Skip("set FP_ICS_URL to run the live-feed diagnostic")
	}
	cfg, _ := json.Marshal(CalendarConfig{URL: url, WeeksAhead: "4"})
	p, err := newCalendar(cfg, nil, time.Now)
	require.NoError(t, err)

	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)
	t.Logf("agenda events in next 4 weeks: %d", len(cd.Events))
}
