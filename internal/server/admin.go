package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/auth"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/theme"
	"github.com/jvmeir/familyplanner/internal/web"
	"github.com/jvmeir/familyplanner/internal/widget"
)

// csrfMiddleware ensures a per-session CSRF token exists, exposes it to templates
// via context, and verifies it on mutating requests (header X-CSRF-Token, set by
// HTMX, or the _csrf form field).
func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := s.sessions.GetString(r.Context(), "csrf")
		if tok == "" {
			tok, _ = auth.NewToken()
			s.sessions.Put(r.Context(), "csrf", tok)
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			sent := r.Header.Get("X-CSRF-Token")
			if sent == "" {
				sent = r.FormValue("_csrf")
			}
			if sent == "" || sent != tok {
				http.Error(w, "bad csrf token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(web.WithCSRF(r.Context(), tok)))
	})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.Admin())
}

// ---- widgets ----

func (s *Server) handleWidgets(w http.ResponseWriter, r *http.Request) {
	types := s.typeVMs(r.Context())
	var initial []web.FieldVM
	if len(types) > 0 {
		initial = s.fieldVMs(r.Context(), types[0].ID, nil)
	}
	s.render(w, r, web.WidgetsPage(types, initial, s.widgetVMs(r.Context())))
}

func (s *Server) handleWidgetFields(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.WidgetFields(s.fieldVMs(r.Context(), r.URL.Query().Get("type"), nil)))
}

func (s *Server) handleWidgetCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	typeID := r.FormValue("type")
	typ, ok := s.registry.Get(typeID)
	if name == "" || !ok {
		http.Error(w, "invalid widget", http.StatusBadRequest)
		return
	}
	cfg := map[string]string{}
	for _, f := range typ.Schema.Fields {
		cfg[f.Name] = r.FormValue(f.Name)
	}
	js, _ := json.Marshal(cfg)
	if _, err := s.store.CreateWidget(r.Context(), dbgen.CreateWidgetParams{
		Name: name, Type: typeID, ConfigJson: string(js),
	}); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.WidgetList(s.widgetVMs(r.Context())))
}

func (s *Server) handleWidgetEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	wgt, err := s.store.GetWidget(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	typ, _ := s.registry.Get(wgt.Type)
	typeName := wgt.Type
	if typ.NameKey != "" {
		typeName = i18n.T(r.Context(), typ.NameKey)
	}
	cfg := map[string]string{}
	_ = json.Unmarshal([]byte(wgt.ConfigJson), &cfg)
	s.render(w, r, web.WidgetEditPage(
		wgt.ID, wgt.Name, typeName,
		s.fieldVMs(r.Context(), wgt.Type, cfg),
		len(typ.AcceptsSources) > 0,
		s.widgetSourceVMs(r.Context(), wgt.ID),
		s.availableSourcesFor(r.Context(), typ),
	))
}

func (s *Server) handleWidgetUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	wgt, err := s.store.GetWidget(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	typ, ok := s.registry.Get(wgt.Type)
	if !ok {
		http.Error(w, "unknown type", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	cfg := map[string]string{}
	for _, f := range typ.Schema.Fields {
		cfg[f.Name] = r.FormValue(f.Name)
	}
	js, _ := json.Marshal(cfg)
	if err := s.store.UpdateWidget(r.Context(), dbgen.UpdateWidgetParams{
		Name: name, ConfigJson: string(js), ID: id,
	}); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	// Apply the new config immediately by refreshing the cache.
	if updated, err := s.store.GetWidget(r.Context(), id); err == nil {
		s.brk.RefreshWidget(r.Context(), updated)
	}
	http.Redirect(w, r, "/admin/widgets", http.StatusSeeOther)
}

func (s *Server) handleWidgetDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteWidget(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.WidgetList(s.widgetVMs(r.Context())))
}

// ---- views ----

func (s *Server) handleViews(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.ViewsPage(s.viewVMs(r.Context())))
}

func (s *Server) handleViewCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if _, err := s.store.CreateView(r.Context(), dbgen.CreateViewParams{
		Name:          name,
		Cols:          parseIntDefault(r.FormValue("cols"), 3),
		Rows:          parseIntDefault(r.FormValue("rows"), 2),
		ThemeID:       r.FormValue("theme"),
		InRotation:    1,
		RotationOrder: 0,
		DwellSeconds:  30,
	}); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.ViewList(s.viewVMs(r.Context())))
}

func (s *Server) handleViewDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteView(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.ViewList(s.viewVMs(r.Context())))
}

// ---- view-model builders ----

func (s *Server) typeVMs(ctx context.Context) []web.WidgetTypeVM {
	out := []web.WidgetTypeVM{}
	for _, t := range s.registry.Types() {
		out = append(out, web.WidgetTypeVM{ID: t.ID, Name: i18n.T(ctx, t.NameKey)})
	}
	return out
}

func (s *Server) fieldVMs(ctx context.Context, typeID string, cfg map[string]string) []web.FieldVM {
	typ, ok := s.registry.Get(typeID)
	if !ok {
		return nil
	}
	return schemaFieldVMs(ctx, typ.Schema, cfg)
}

// schemaFieldVMs renders any widget.Schema into form field view-models (reused
// for widget config and data-source config/credential schemas).
func schemaFieldVMs(ctx context.Context, schema widget.Schema, values map[string]string) []web.FieldVM {
	out := make([]web.FieldVM, 0, len(schema.Fields))
	for _, f := range schema.Fields {
		fvm := web.FieldVM{
			Name:     f.Name,
			Label:    i18n.T(ctx, f.LabelKey),
			Type:     string(f.Type),
			Required: f.Required,
			Value:    values[f.Name],
		}
		if f.Type == widget.FieldSelect {
			current := values[f.Name]
			for i, o := range f.Options {
				fvm.Options = append(fvm.Options, web.OptionVM{
					Value:    o.Value,
					Label:    i18n.T(ctx, o.LabelKey),
					Selected: current == o.Value || (current == "" && i == 0),
				})
			}
		}
		out = append(out, fvm)
	}
	return out
}

// availableSourcesFor lists data sources whose type the widget type accepts.
func (s *Server) availableSourcesFor(ctx context.Context, typ widget.Type) []web.DataSourceVM {
	var out []web.DataSourceVM
	for _, d := range s.dataSourceVMs(ctx) {
		if typ.Accepts(d.Type) {
			out = append(out, d)
		}
	}
	return out
}

func (s *Server) widgetVMs(ctx context.Context) []web.WidgetVM {
	rows, err := s.store.ListWidgets(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.WidgetVM, 0, len(rows))
	for _, wgt := range rows {
		typeName := wgt.Type
		if typ, ok := s.registry.Get(wgt.Type); ok {
			typeName = i18n.T(ctx, typ.NameKey)
		}
		out = append(out, web.WidgetVM{ID: wgt.ID, Name: wgt.Name, Type: wgt.Type, TypeName: typeName})
	}
	return out
}

func (s *Server) viewVMs(ctx context.Context) []web.ViewVM {
	rows, err := s.store.ListViews(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.ViewVM, 0, len(rows))
	for _, v := range rows {
		out = append(out, web.ViewVM{ID: v.ID, Name: v.Name, Cols: v.Cols, Rows: v.Rows})
	}
	return out
}

func (s *Server) themeOpts() []web.ThemeOptVM {
	out := make([]web.ThemeOptVM, 0, len(theme.Presets))
	for id, t := range theme.Presets {
		out = append(out, web.ThemeOptVM{ID: id, Name: t.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parseIntDefault(s string, def int64) int64 {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}
