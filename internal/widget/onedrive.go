package widget

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// DriveChild is one item in a OneDrive folder listing (the pdf/pptx file picker).
type DriveChild struct {
	ID       string
	Name     string
	IsFolder bool
	IsDoc    bool // a PDF or an Office doc we can render (Graph converts to PDF)
}

// isPDFName / IsOfficeDoc / IsRenderableDoc classify a filename for the picker.
func isPDFName(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".pdf") }

// IsOfficeDoc reports whether a file is an Office document Graph can render to
// PDF via ?format=pdf (PowerPoint / Word / Excel).
func IsOfficeDoc(name string) bool {
	n := strings.ToLower(name)
	for _, e := range []string{".pptx", ".ppt", ".docx", ".doc", ".xlsx"} {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// IsRenderableDoc reports whether a file can be shown by the pdf widget.
func IsRenderableDoc(name string) bool { return isPDFName(name) || IsOfficeDoc(name) }

// GraphListChildren lists the folders + renderable documents in a OneDrive folder
// (empty id = drive root), for the pdf widget's file picker.
func GraphListChildren(ctx context.Context, token, folderID string) ([]DriveChild, error) {
	path := "/me/drive/root/children"
	if folderID != "" {
		path = "/me/drive/items/" + folderID + "/children"
	}
	var body struct {
		Value []struct {
			ID     string    `json:"id"`
			Name   string    `json:"name"`
			Folder *struct{} `json:"folder"`
			File   *struct {
				MimeType string `json:"mimeType"`
			} `json:"file"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+path+"?$select=id,name,folder,file&$top=400", &body); err != nil {
		return nil, err
	}
	out := make([]DriveChild, 0, len(body.Value))
	for _, it := range body.Value {
		c := DriveChild{ID: it.ID, Name: it.Name, IsFolder: it.Folder != nil}
		c.IsDoc = it.File != nil && IsRenderableDoc(it.Name)
		if c.IsFolder || c.IsDoc {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsFolder != out[j].IsFolder {
			return out[i].IsFolder // folders first
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GraphFetchContent streams a drive item's bytes to dst. asPDF requests Graph's
// on-the-fly PDF rendering (?format=pdf) for Office files; the /content redirect
// to a pre-authenticated download URL is followed by the http client.
func GraphFetchContent(ctx context.Context, token, itemID string, asPDF bool, dst io.Writer) error {
	u := graphBase + "/me/drive/items/" + itemID + "/content"
	if asPDF {
		u += "?format=pdf"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graph content: status %d", resp.StatusCode)
	}
	_, err = io.Copy(dst, io.LimitReader(resp.Body, 60<<20)) // cap at 60MB
	return err
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
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
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
		if it.Location != nil && (it.Location.Latitude != 0 || it.Location.Longitude != 0) {
			p.Lat, p.Lon, p.HasGeo = it.Location.Latitude, it.Location.Longitude, true
		}
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].When < items[j].When })
	withGeo := 0
	for _, it := range items {
		if it.HasGeo {
			withGeo++
		}
	}
	slog.Info("onedrive: photos listed", "total", len(items), "withGeo", withGeo)
	return items, nil
}
