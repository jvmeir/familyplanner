package widget

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// PhotosConfig is the per-instance configuration.
type PhotosConfig struct {
	Interval string `json:"interval"` // seconds per photo in the client slideshow (default 8)
}

// PhotosData is the normalized render data: the full set of photo URLs (the
// client cycles them as a shuffled, no-repeat slideshow) + seconds per photo.
type PhotosData struct {
	URLs         []string `json:"urls"`
	IntervalSecs int      `json:"interval_secs"`
}

type photosProvider struct {
	cfg     PhotosConfig
	sources []SourceInput
}

func newPhotos(raw json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	var cfg PhotosConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return photosProvider{cfg: cfg, sources: sources}, nil
}

func decodePhotos(raw json.RawMessage) (Data, error) {
	var d PhotosData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p photosProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	var urls []string
	var firstErr error
	ok := 0
	for _, s := range p.sources {
		var sec struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.Unmarshal(s.Secret, &sec)
		if sec.AccessToken == "" {
			continue
		}
		if s.Type != "onedrive" {
			continue
		}
		// Folder/album chosen per widget↔source link (resource); "" = drive root.
		us, err := GraphFolderPhotos(ctx, sec.AccessToken, s.Resource)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		urls = append(urls, us...)
		ok++
	}
	if ok == 0 && firstErr != nil {
		return nil, 0, firstErr
	}
	secs := 8
	if n, err := strconv.Atoi(p.cfg.Interval); err == nil && n > 0 {
		secs = n
	}
	// OneDrive thumbnail URLs expire ~60 min; refresh before then.
	return PhotosData{URLs: urls, IntervalSecs: secs}, 45 * time.Minute, nil
}
