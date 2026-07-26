package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/web"
	"github.com/jvmeir/familyplanner/internal/widget"
)

// maxPdfBytes caps an uploaded PDF (kiosk documents are small; keeps disk sane).
const maxPdfBytes = 25 << 20 // 25 MB

// handleWidgetPdfUpload stores the PDF uploaded on a pdf widget's edit page.
// Validates it's really a PDF (magic bytes) and within the size cap, writes it
// to the data volume keyed by widget id, and records the filename in config.
func (s *Server) handleWidgetPdfUpload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	wgt, err := s.store.GetWidget(r.Context(), id)
	if err != nil || wgt.Type != "pdf" {
		http.Error(w, "not a pdf widget", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(maxPdfBytes + (1 << 20)); err != nil {
		http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
		return
	}
	file, hdr, err := r.FormFile("pdf")
	if err != nil {
		http.Error(w, "no file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if hdr.Size > maxPdfBytes {
		http.Error(w, "file too large (max 25 MB)", http.StatusRequestEntityTooLarge)
		return
	}
	head := make([]byte, 5)
	if n, _ := io.ReadFull(file, head); n < 4 || string(head[:4]) != "%PDF" {
		http.Error(w, "not a PDF", http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(filepath.Join(s.cfg.DataDir, "pdf"), 0o755); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	dst, err := os.Create(s.pdfPath(id))
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(dst, io.LimitReader(file, maxPdfBytes)); err != nil {
		dst.Close()
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}
	dst.Close()

	// Record the original filename (display only) in the widget config.
	var cfg map[string]any
	if json.Unmarshal([]byte(wgt.ConfigJson), &cfg) != nil || cfg == nil {
		cfg = map[string]any{}
	}
	cfg["file"] = filepath.Base(hdr.Filename)
	if b, err := json.Marshal(cfg); err == nil {
		_ = s.store.UpdateWidget(r.Context(), dbgen.UpdateWidgetParams{Name: wgt.Name, ConfigJson: string(b), ID: id})
	}
	s.refreshWidgetCache(r.Context(), id)
	http.Redirect(w, r, "/admin/widgets/"+strconv.FormatInt(id, 10)+"/edit", http.StatusSeeOther)
}

// handlePdfMedia serves a pdf widget's uploaded file to the kiosk (and admin
// preview). Serves the local upload, or — for a OneDrive-sourced widget — a
// cached copy proxied from OneDrive (converted to PDF for Office files).
// LAN/Tailscale-only deployment; the file is addressed by widget id.
func (s *Server) handlePdfMedia(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// OneDrive-sourced: (re)fetch into the on-disk cache when stale.
	if wgt, err := s.store.GetWidget(r.Context(), id); err == nil {
		var pc struct {
			ODItem string `json:"od_item"`
			ODName string `json:"od_name"`
		}
		_ = json.Unmarshal([]byte(wgt.ConfigJson), &pc)
		if pc.ODItem != "" && s.pdfCacheStale(s.pdfPath(id)) {
			if ferr := s.fetchOneDrivePdf(r.Context(), id, pc.ODItem, pc.ODName); ferr != nil {
				if _, e := os.Stat(s.pdfPath(id)); e != nil { // no stale copy to fall back on
					http.Error(w, "onedrive fetch failed", http.StatusBadGateway)
					return
				}
			}
		}
	}
	f, err := os.Open(s.pdfPath(id))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeContent(w, r, "document.pdf", st.ModTime(), f)
}

// pdfCacheStale reports whether the on-disk cache is missing or older than 1h
// (OneDrive download URLs / conversions are re-fetched periodically).
func (s *Server) pdfCacheStale(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return true
	}
	return s.now().Sub(st.ModTime()) > time.Hour
}

// widgetOneDriveToken returns a fresh access token from the widget's linked
// OneDrive data source (the account used to browse + fetch the file).
func (s *Server) widgetOneDriveToken(ctx context.Context, widgetID int64) (string, error) {
	rows, err := s.store.ListWidgetSources(ctx, widgetID)
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if r.SourceType == "onedrive" {
			ds, derr := s.store.GetDataSource(ctx, r.DataSourceID)
			if derr != nil {
				continue
			}
			return s.freshAccessToken(ctx, ds)
		}
	}
	return "", errors.New("no OneDrive source linked")
}

// fetchOneDrivePdf downloads the chosen OneDrive item into the widget's cache,
// asking Graph to render Office files to PDF. Atomic via a temp file + rename.
func (s *Server) fetchOneDrivePdf(ctx context.Context, id int64, item, name string) error {
	tok, err := s.widgetOneDriveToken(ctx, id)
	if err != nil {
		return err
	}
	cache := s.pdfPath(id)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cache), "pdf-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := widget.GraphFetchContent(ctx, tok, item, widget.IsOfficeDoc(name), tmp); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), cache)
}

// handleWidgetOneDriveBrowse renders the OneDrive folder listing (HTMX fragment)
// for the pdf widget's file picker. folder="" = drive root.
func (s *Server) handleWidgetOneDriveBrowse(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	folder := r.URL.Query().Get("folder")
	tok, err := s.widgetOneDriveToken(r.Context(), id)
	if err != nil {
		s.render(w, r, web.OneDriveBrowser(id, nil, "", true))
		return
	}
	children, err := widget.GraphListChildren(r.Context(), tok, folder)
	if err != nil {
		s.render(w, r, web.OneDriveBrowser(id, nil, "", true))
		return
	}
	vms := make([]web.DriveChildVM, 0, len(children))
	for _, c := range children {
		vms = append(vms, web.DriveChildVM{ID: c.ID, Name: c.Name, IsFolder: c.IsFolder})
	}
	s.render(w, r, web.OneDriveBrowser(id, vms, folder, false))
}

// handleWidgetOneDriveSelect records the chosen OneDrive file on the pdf widget
// and drops the cache so the next request re-fetches it.
func (s *Server) handleWidgetOneDriveSelect(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	wgt, err := s.store.GetWidget(r.Context(), id)
	if err != nil || wgt.Type != "pdf" {
		http.Error(w, "not a pdf widget", http.StatusBadRequest)
		return
	}
	var cfg map[string]any
	if json.Unmarshal([]byte(wgt.ConfigJson), &cfg) != nil || cfg == nil {
		cfg = map[string]any{}
	}
	cfg["od_item"] = r.FormValue("item")
	cfg["od_name"] = r.FormValue("name")
	cfg["file"] = r.FormValue("name") // shown as the current file
	if b, err := json.Marshal(cfg); err == nil {
		_ = s.store.UpdateWidget(r.Context(), dbgen.UpdateWidgetParams{Name: wgt.Name, ConfigJson: string(b), ID: id})
	}
	_ = os.Remove(s.pdfPath(id)) // force a re-fetch from OneDrive
	http.Redirect(w, r, "/admin/widgets/"+strconv.FormatInt(id, 10)+"/edit", http.StatusSeeOther)
}
