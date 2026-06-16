package widget

import (
	"context"
	"encoding/json"
	"time"
)

// PhotosConfig is the per-instance configuration.
type PhotosConfig struct {
	Mode string `json:"mode"` // "single" (default) | "random"
}

// PhotosData is the normalized render data: candidate photo URLs + display mode.
type PhotosData struct {
	URLs []string `json:"urls"`
	Mode string   `json:"mode"`
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
		var us []string
		var err error
		switch s.Type {
		case "google_photos":
			var cfg struct {
				AlbumID string `json:"album_id"`
			}
			_ = json.Unmarshal(s.Config, &cfg)
			albumID := cfg.AlbumID
			if s.Resource != "" {
				albumID = s.Resource
			}
			if albumID == "" {
				continue
			}
			us, err = GooglePhotoURLs(ctx, sec.AccessToken, albumID)
		case "onedrive":
			var cfg struct {
				FolderID string `json:"folder_id"`
			}
			_ = json.Unmarshal(s.Config, &cfg)
			folderID := cfg.FolderID
			if s.Resource != "" {
				folderID = s.Resource
			}
			us, err = GraphFolderPhotos(ctx, sec.AccessToken, folderID)
		default:
			continue
		}
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
	mode := p.cfg.Mode
	if mode == "" {
		mode = "single"
	}
	// Google base URLs expire ~60 min; refresh before then.
	return PhotosData{URLs: urls, Mode: mode}, 45 * time.Minute, nil
}
