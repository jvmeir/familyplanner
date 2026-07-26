package widget

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// PdfConfig is the per-instance configuration. The PDF file itself is uploaded
// separately (stored per widget) — this only holds display options. With
// interval > 0 the widget behaves as a slideshow/movie (one page per `interval`
// seconds, playing to the end); with interval 0 it's a scrollable document.
type PdfConfig struct {
	File     string `json:"file"`     // original filename (display only)
	Interval string `json:"interval"` // seconds per page (>0 = slideshow; 0 = scroll)
	Fit      string `json:"fit"`      // "width" (default) | "page"
}

// PdfData is the normalized render data.
type PdfData struct {
	File         string `json:"file"`
	IntervalSecs int    `json:"interval_secs"`
	Fit          string `json:"fit"`
}

type pdfProvider struct{ cfg PdfConfig }

func newPdf(raw json.RawMessage, _ []SourceInput, _ NowFunc) (Provider, error) {
	var cfg PdfConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return pdfProvider{cfg: cfg}, nil
}

func decodePdf(raw json.RawMessage) (Data, error) {
	var d PdfData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p pdfProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	secs := 0
	if n, err := strconv.Atoi(p.cfg.Interval); err == nil && n > 0 {
		secs = n
	}
	fit := p.cfg.Fit
	if fit == "" {
		fit = "width"
	}
	// Static content; refresh rarely (the file is served directly, not cached here).
	return PdfData{File: p.cfg.File, IntervalSecs: secs, Fit: fit}, 24 * time.Hour, nil
}

// PdfSlideshowInterval returns a pdf widget's configured per-page interval (0 =
// scroll mode). Used server-side to know whether a pdf view plays to an end.
func PdfSlideshowInterval(configJSON string) int {
	var cfg PdfConfig
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return 0
	}
	n, _ := strconv.Atoi(cfg.Interval)
	if n < 0 {
		n = 0
	}
	return n
}
