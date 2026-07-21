package widget

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// VideoDir is where downloaded videos are stored (and served from at /media);
// YtdlpPath is the yt-dlp executable. Both are set at startup.
var (
	VideoDir  = ""
	YtdlpPath = "yt-dlp"
)

// ytIDRe extracts an 11-char YouTube video id from a URL or a bare id.
var ytIDRe = regexp.MustCompile(`(?:v=|/embed/|youtu\.be/|/shorts/|^)([A-Za-z0-9_-]{11})`)

func youtubeID(s string) string {
	if m := ytIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// VideoConfig is the per-instance configuration.
type VideoConfig struct {
	URL  string `json:"url"`
	Mute string `json:"mute"` // "yes" | "no" (default no)
	Loop string `json:"loop"` // "yes" | "no" (default yes)
}

// VideoData is the normalized render data. While the file is still downloading,
// Downloading is true and URL is empty.
type VideoData struct {
	URL         string `json:"url"`
	Mute        bool   `json:"mute"`
	Loop        bool   `json:"loop"`
	Downloading bool   `json:"downloading"`
}

type videoProvider struct {
	cfg VideoConfig
}

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

func (p videoProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	id := youtubeID(p.cfg.URL)
	if id == "" || VideoDir == "" {
		return VideoData{}, time.Minute, nil
	}
	data := VideoData{Mute: p.cfg.Mute == "yes", Loop: p.cfg.Loop != "no"} // loop defaults on
	path := filepath.Join(VideoDir, id+".mp4")
	if _, err := os.Stat(path); err == nil {
		data.URL = "/media/" + id + ".mp4"
		return data, time.Hour, nil // cached file; long TTL
	}
	// Not downloaded yet: kick off a background download and report "downloading";
	// a short TTL means the broker re-checks until the file is ready.
	ensureVideoDownload(id)
	data.Downloading = true
	return data, 15 * time.Second, nil
}

var (
	dlMu       sync.Mutex
	dlInflight = map[string]bool{}
)

// ensureVideoDownload downloads a YouTube video to VideoDir/<id>.mp4 (ad-free,
// since it plays a local file), de-duplicating concurrent downloads. yt-dlp
// writes to <id>.mp4.part first and renames on success, so the .mp4 only exists
// once complete.
func ensureVideoDownload(id string) {
	dlMu.Lock()
	if dlInflight[id] {
		dlMu.Unlock()
		return
	}
	dlInflight[id] = true
	dlMu.Unlock()
	go func() {
		defer func() {
			dlMu.Lock()
			delete(dlInflight, id)
			dlMu.Unlock()
		}()
		out := filepath.Join(VideoDir, id+".mp4")
		// Build the URL from the validated id so no arbitrary string reaches yt-dlp.
		url := "https://www.youtube.com/watch?v=" + id
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, YtdlpPath,
			"--no-playlist",
			"-f", "bv*[height<=1080]+ba/b[height<=1080]/b",
			"--merge-output-format", "mp4",
			"-o", out,
			url,
		)
		_ = cmd.Run()
	}()
}
