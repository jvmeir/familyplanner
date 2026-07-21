package widget

import (
	"context"
	"encoding/json"
	"regexp"
	"time"
)

// ytIDRe extracts an 11-char YouTube video id from a URL or a bare id.
var ytIDRe = regexp.MustCompile(`(?:v=|/embed/|youtu\.be/|/shorts/|^)([A-Za-z0-9_-]{11})`)

func youtubeID(s string) string {
	if m := ytIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// VideoConfig is the per-instance configuration. A video widget plays the videos
// from its linked "video" data sources (like the ticker aggregates feeds); URL is
// an optional single-video fallback when no sources are linked.
type VideoConfig struct {
	URL  string `json:"url"`
	Mute string `json:"mute"` // "yes" | "no" (default no)
	Loop string `json:"loop"` // "yes" | "no" (default yes)
}

// VideoData is the normalized render data: the YouTube video ids to play (in
// order) plus playback options. The kiosk embeds them with the YouTube IFrame
// Player API, cycling the list and (for the corner PiP) honouring a hide interval.
type VideoData struct {
	IDs  []string `json:"ids"`
	Mute bool     `json:"mute"`
	Loop bool     `json:"loop"`
}

type videoProvider struct {
	cfg     VideoConfig
	sources []SourceInput
}

func newVideo(raw json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	var cfg VideoConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return videoProvider{cfg: cfg, sources: sources}, nil
}

func decodeVideo(raw json.RawMessage) (Data, error) {
	var d VideoData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p videoProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	var ids []string
	for _, s := range p.sources {
		if s.Type != "video" {
			continue
		}
		var c struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(s.Config, &c)
		if id := youtubeID(c.URL); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 { // fallback: a single video configured directly on the widget
		if id := youtubeID(p.cfg.URL); id != "" {
			ids = append(ids, id)
		}
	}
	return VideoData{
		IDs:  ids,
		Mute: p.cfg.Mute == "yes",
		Loop: p.cfg.Loop != "no", // loop defaults on
	}, time.Hour, nil
}
