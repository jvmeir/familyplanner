package widget

import (
	"context"
	"encoding/json"
	"time"
)

// CountdownConfig is the per-instance configuration for a countdown widget.
type CountdownConfig struct {
	Title     string `json:"title"`
	Date      string `json:"date"`      // target date, YYYY-MM-DD
	Time      string `json:"time"`      // optional target time-of-day, HH:MM (default 00:00)
	Precision string `json:"precision"` // "days" (default) | "dhms" (days/hours/min/sec)
}

// CountdownData is the normalized render data.
type CountdownData struct {
	Title      string
	DaysLeft   int
	Today      bool
	Precision  string // "days" | "dhms"
	TargetUnix int64  // target instant (Unix seconds); drives the live dhms ticker
}

type countdownProvider struct {
	cfg CountdownConfig
	now NowFunc
}

func decodeCountdown(raw json.RawMessage) (Data, error) {
	var d CountdownData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func newCountdown(raw json.RawMessage, _ []SourceInput, now NowFunc) (Provider, error) {
	var cfg CountdownConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	if now == nil {
		now = time.Now
	}
	return countdownProvider{cfg: cfg, now: now}, nil
}

func (p countdownProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	target, err := time.ParseInLocation("2006-01-02", p.cfg.Date, p.now().Location())
	if err != nil {
		return nil, 0, err
	}
	n := p.now()
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	tgt := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, n.Location())
	days := int(tgt.Sub(today).Hours() / 24)

	// Optional target time-of-day (default midnight) for the live dhms ticker.
	hh, mm := 0, 0
	if t, terr := time.Parse("15:04", p.cfg.Time); terr == nil {
		hh, mm = t.Hour(), t.Minute()
	}
	targetAt := time.Date(target.Year(), target.Month(), target.Day(), hh, mm, 0, 0, n.Location())

	precision := "days"
	if p.cfg.Precision == "dhms" {
		precision = "dhms"
	}

	return CountdownData{
		Title:      p.cfg.Title,
		DaysLeft:   days,
		Today:      days == 0,
		Precision:  precision,
		TargetUnix: targetAt.Unix(),
	}, time.Hour, nil
}
