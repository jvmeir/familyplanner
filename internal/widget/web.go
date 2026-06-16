package widget

import (
	"context"
	"encoding/json"
	"time"
)

// WebConfig configures the embedded-page widget.
type WebConfig struct {
	URL string `json:"url"`
}

// WebData is the normalized render data (just the URL to embed).
type WebData struct {
	URL string `json:"url"`
}

type webProvider struct{ url string }

func newWeb(raw json.RawMessage, _ []SourceInput, _ NowFunc) (Provider, error) {
	var cfg WebConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return webProvider{url: cfg.URL}, nil
}

func decodeWeb(raw json.RawMessage) (Data, error) {
	var d WebData
	err := json.Unmarshal(raw, &d)
	return d, err
}

// Fetch does no network call; the kiosk's browser loads the page in an iframe.
func (p webProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	return WebData{URL: p.url}, time.Hour, nil
}
