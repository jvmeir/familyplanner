package widget

import (
	"context"
	"log/slog"
	"sort"
	"strings"
)

// GraphListFolders lists top-level OneDrive folders (for the configure picker).
func GraphListFolders(ctx context.Context, token string) ([]ResourceOption, error) {
	var body struct {
		Value []struct {
			ID     string    `json:"id"`
			Name   string    `json:"name"`
			Folder *struct{} `json:"folder"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+"/me/drive/root/children?$select=id,name,folder&$top=200", &body); err != nil {
		return nil, err
	}
	out := make([]ResourceOption, 0, len(body.Value))
	for _, it := range body.Value {
		if it.Folder != nil {
			out = append(out, ResourceOption{ID: it.ID, Name: it.Name})
		}
	}
	return out, nil
}

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

// GraphFolderPhotos returns pre-authenticated download URLs for the images in a
// OneDrive folder (empty id = drive root), ordered by capture/creation date.
// The download URLs are temporary (~1h) and load directly in an <img> (no token).
func GraphFolderPhotos(ctx context.Context, token, folderID string) ([]string, error) {
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
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+path+"?$top=200", &body); err != nil {
		return nil, err
	}

	type item struct {
		url  string
		when string
	}
	var items []item
	for _, it := range body.Value {
		if it.File == nil || !strings.HasPrefix(it.File.MimeType, "image/") || it.DownloadURL == "" {
			continue
		}
		when := it.CreatedDateTime
		if it.Photo != nil && it.Photo.TakenDateTime != "" {
			when = it.Photo.TakenDateTime
		}
		items = append(items, item{url: it.DownloadURL, when: when})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].when < items[j].when })

	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.url)
	}
	return out, nil
}
