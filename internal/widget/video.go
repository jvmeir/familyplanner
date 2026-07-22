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

// VideoConfig is the per-instance configuration: one YouTube URL plus playback
// options. To show several clips, add several video views to a playlist and let
// the standard rotation cycle them (with advance-on-end).
type VideoConfig struct {
	URL  string `json:"url"`
	Mute string `json:"mute"` // "yes" | "no" (default no)
	Loop string `json:"loop"` // "yes" | "no" (default yes)
}

// VideoData is the normalized render data: the YouTube video id(s) to play plus
// playback options. The kiosk embeds them with the YouTube IFrame Player API.
type VideoData struct {
	IDs  []string `json:"ids"`
	Mute bool     `json:"mute"`
	Loop bool     `json:"loop"`
}

type videoProvider struct{ cfg VideoConfig }

func newVideo(raw json.RawMessage, _ []SourceInput, _ NowFunc) (Provider, error) {
	var cfg VideoConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return videoProvider{cfg: cfg}, nil
}

func decodeVideo(raw json.RawMessage) (Data, error) {
	var d VideoData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p videoProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	var ids []string
	if id := youtubeID(p.cfg.URL); id != "" {
		ids = append(ids, id)
	}
	return VideoData{
		IDs:  ids,
		Mute: p.cfg.Mute == "yes",
		Loop: p.cfg.Loop != "no", // loop defaults on
	}, time.Hour, nil
}
