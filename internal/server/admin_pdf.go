package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
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
// preview). LAN/Tailscale-only deployment; the file is addressed by widget id.
func (s *Server) handlePdfMedia(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
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
