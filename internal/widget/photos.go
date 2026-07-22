package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// bigDataCloudBase is the free (no-key) reverse-geocoder used to turn a photo's
// GPS into a place name. Overridable in tests.
var bigDataCloudBase = "https://api.bigdatacloud.net/data/reverse-geocode-client"

// PhotosConfig is the per-instance configuration.
type PhotosConfig struct {
	Interval string `json:"interval"` // seconds per photo in the client slideshow (default 8)
	Caption  string `json:"caption"`  // "" | "date" | "full" (date + place, from OneDrive metadata)
}

// PhotosData is the normalized render data: the full set of photo URLs (the
// client cycles them as a shuffled, no-repeat slideshow) + a parallel captions
// array (empty when captions are off) + seconds per photo.
type PhotosData struct {
	URLs         []string `json:"urls"`
	Captions     []string `json:"captions,omitempty"`
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
	var items []PhotoItem
	var firstErr error
	ok := 0
	for _, s := range p.sources {
		var sec struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.Unmarshal(s.Secret, &sec)
		if sec.AccessToken == "" || s.Type != "onedrive" {
			continue
		}
		// Folder/album chosen per widget↔source link (resource); "" = drive root.
		its, err := GraphFolderPhotos(ctx, sec.AccessToken, s.Resource)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		items = append(items, its...)
		ok++
	}
	if ok == 0 && firstErr != nil {
		return nil, 0, firstErr
	}

	captions := p.cfg.Caption == "date" || p.cfg.Caption == "full"
	geoCache := map[string]string{} // dedupe reverse-geocode by rounded coords
	urls := make([]string, 0, len(items))
	var caps []string
	if captions {
		caps = make([]string, 0, len(items))
	}
	for _, it := range items {
		urls = append(urls, it.URL)
		if !captions {
			continue
		}
		c := photoDate(it.When)
		if p.cfg.Caption == "full" && it.HasGeo {
			if place := reverseGeocode(ctx, it.Lat, it.Lon, geoCache); place != "" {
				if c != "" {
					c += " · " + place
				} else {
					c = place
				}
			}
		}
		caps = append(caps, c)
	}

	secs := 8
	if n, err := strconv.Atoi(p.cfg.Interval); err == nil && n > 0 {
		secs = n
	}
	// OneDrive thumbnail URLs expire ~60 min; refresh before then.
	return PhotosData{URLs: urls, Captions: caps, IntervalSecs: secs}, 45 * time.Minute, nil
}

// photoDate formats an RFC3339 capture time as a short Dutch date ("2 jul 2026").
func photoDate(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), nlMonthShort[int(t.Month())-1], t.Year())
}

// reverseGeocode resolves GPS to a place name (city/locality), memoized per fetch
// by rounded coordinates so an album shot in one place makes a single call.
func reverseGeocode(ctx context.Context, lat, lon float64, cache map[string]string) string {
	key := fmt.Sprintf("%.2f,%.2f", lat, lon)
	if v, ok := cache[key]; ok {
		return v
	}
	place := geocodeReverse(ctx, lat, lon)
	cache[key] = place
	return place
}

func geocodeReverse(ctx context.Context, lat, lon float64) string {
	u := fmt.Sprintf("%s?latitude=%.4f&longitude=%.4f&localityLanguage=nl", bigDataCloudBase, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var b struct {
		City                 string `json:"city"`
		Locality             string `json:"locality"`
		PrincipalSubdivision string `json:"principalSubdivision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return ""
	}
	switch {
	case b.City != "":
		return b.City
	case b.Locality != "":
		return b.Locality
	default:
		return b.PrincipalSubdivision
	}
}
