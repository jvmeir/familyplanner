package widget

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jvmeir/familyplanner/internal/i18n"
)

// ClockData is the normalized render data for the clock widget.
type ClockData struct {
	TimeText string // "14:05"
	DateText string // "30 mei 2026"
}

type clockProvider struct {
	now NowFunc
}

func decodeClock(raw json.RawMessage) (Data, error) {
	var d ClockData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func newClock(_ json.RawMessage, _ []SourceInput, now NowFunc) (Provider, error) {
	if now == nil {
		now = time.Now
	}
	return clockProvider{now: now}, nil
}

func (p clockProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	n := p.now()
	return ClockData{
		TimeText: n.Format("15:04"),
		DateText: i18n.Date(n),
	}, 30 * time.Second, nil
}
