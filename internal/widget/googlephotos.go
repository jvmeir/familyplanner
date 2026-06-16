package widget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// googlePhotosBase is overridable in tests.
var googlePhotosBase = "https://photoslibrary.googleapis.com/v1"

// GoogleListAlbums lists the user's photo albums (for the configure picker).
func GoogleListAlbums(ctx context.Context, token string) ([]ResourceOption, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googlePhotosBase+"/albums?pageSize=50", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google photos: albums status %d", resp.StatusCode)
	}
	var body struct {
		Albums []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]ResourceOption, 0, len(body.Albums))
	for _, a := range body.Albums {
		out = append(out, ResourceOption{ID: a.ID, Name: a.Title})
	}
	return out, nil
}

// GooglePhotoURLs returns display URLs for the media items in an album.
func GooglePhotoURLs(ctx context.Context, token, albumID string) ([]string, error) {
	reqBody, _ := json.Marshal(map[string]any{"albumId": albumID, "pageSize": 100})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googlePhotosBase+"/mediaItems:search", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google photos: search status %d", resp.StatusCode)
	}
	var body struct {
		MediaItems []struct {
			BaseURL       string `json:"baseUrl"`
			MediaMetadata struct {
				CreationTime string `json:"creationTime"`
			} `json:"mediaMetadata"`
		} `json:"mediaItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	// Order chronologically (ascending) so the "by date" mode is meaningful.
	sort.Slice(body.MediaItems, func(i, j int) bool {
		return body.MediaItems[i].MediaMetadata.CreationTime < body.MediaItems[j].MediaMetadata.CreationTime
	})
	out := make([]string, 0, len(body.MediaItems))
	for _, m := range body.MediaItems {
		// Google base URLs need a size suffix; expire ~60 min (broker TTL handles refresh).
		out = append(out, m.BaseURL+"=w1600-h900")
	}
	return out, nil
}
