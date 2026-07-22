package widget

import (
	"context"
	"log/slog"
	"sort"
	"strings"
)

// GraphListAlbums lists OneDrive photo albums (personal OneDrive "bundles" with
// an album facet). An album's photos are fetched with GraphFolderPhotos, since a
// bundle is a driveItem whose children are its photos.
func GraphListAlbums(ctx context.Context, token string) ([]ResourceOption, error) {
	var body struct {
		Value []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Bundle *struct {
				Album *struct{} `json:"album"`
			} `json:"bundle"`
		} `json:"value"`
	}
	// Fetch ALL bundles (no $filter — that filter can return empty on some drives)
	// and keep those that are albums (or, if none carry an explicit album facet,
	// all bundles as a fallback).
	u := graphBase + "/me/drive/bundles?$select=id,name,bundle&$top=200"
	if err := graphGet(ctx, token, u, &body); err != nil {
		return nil, err
	}
	var albums, all []ResourceOption
	for _, it := range body.Value {
		opt := ResourceOption{ID: it.ID, Name: it.Name}
		all = append(all, opt)
		if it.Bundle != nil && it.Bundle.Album != nil {
			albums = append(albums, opt)
		}
	}
	slog.Info("onedrive: bundles listed", "total", len(all), "albums", len(albums))
	if len(albums) > 0 {
		return albums, nil
	}
	return all, nil // fallback: expose all bundles if none are tagged as albums
}

// PhotoItem is one OneDrive photo with the metadata used for an optional caption.
type PhotoItem struct {
	URL    string
	When   string  // takenDateTime (or createdDateTime), RFC3339
	Lat    float64 // GPS, valid only when HasGeo
	Lon    float64
	HasGeo bool
}

// GraphFolderPhotos returns the photos in a OneDrive folder or album (empty id =
// drive root), ordered by capture/creation date, each with its capture time and
// GPS (when present). It prefers a large thumbnail URL (a proper image/* CDN link
// that renders reliably in an <img>) and only falls back to the raw downloadUrl
// (which OneDrive serves as application/octet-stream, so browsers often won't
// render it).
func GraphFolderPhotos(ctx context.Context, token, folderID string) ([]PhotoItem, error) {
	path := "/me/drive/root/children"
	if folderID != "" {
		path = "/me/drive/items/" + folderID + "/children"
	}
	var body struct {
		Value []struct {
			File *struct {
				MimeType string `json:"mimeType"`
			} `json:"file"`
			DownloadURL     string `json:"@microsoft.graph.downloadUrl"`
			CreatedDateTime string `json:"createdDateTime"`
			Photo           *struct {
				TakenDateTime string `json:"takenDateTime"`
			} `json:"photo"`
			Location *struct {
				Coordinates *struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"coordinates"`
			} `json:"location"`
			Thumbnails []struct {
				Large *struct {
					URL string `json:"url"`
				} `json:"large"`
				Medium *struct {
					URL string `json:"url"`
				} `json:"medium"`
			} `json:"thumbnails"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+path+"?$top=200&$expand=thumbnails", &body); err != nil {
		return nil, err
	}

	var items []PhotoItem
	for _, it := range body.Value {
		isImage := it.File != nil && strings.HasPrefix(it.File.MimeType, "image/")
		url := ""
		if len(it.Thumbnails) > 0 {
			if it.Thumbnails[0].Large != nil {
				url = it.Thumbnails[0].Large.URL
			} else if it.Thumbnails[0].Medium != nil {
				url = it.Thumbnails[0].Medium.URL
			}
		}
		if url == "" && isImage {
			url = it.DownloadURL // last resort (may not render if octet-stream)
		}
		if url == "" || (!isImage && len(it.Thumbnails) == 0) {
			continue
		}
		p := PhotoItem{URL: url, When: it.CreatedDateTime}
		if it.Photo != nil && it.Photo.TakenDateTime != "" {
			p.When = it.Photo.TakenDateTime
		}
		if it.Location != nil && it.Location.Coordinates != nil &&
			(it.Location.Coordinates.Latitude != 0 || it.Location.Coordinates.Longitude != 0) {
			p.Lat, p.Lon, p.HasGeo = it.Location.Coordinates.Latitude, it.Location.Coordinates.Longitude, true
		}
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].When < items[j].When })
	return items, nil
}
