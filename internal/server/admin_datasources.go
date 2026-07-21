package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/crypto"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/web"
)

func (s *Server) handleDataSources(w http.ResponseWriter, r *http.Request) {
	types := s.dsTypeVMs(r.Context())
	var initial []web.FieldVM
	if len(types) > 0 {
		initial = s.dsFieldVMs(r.Context(), types[0].ID)
	}
	s.render(w, r, web.DataSourcesPage(types, initial, s.dataSourceVMs(r.Context())))
}

func (s *Server) handleDataSourceFields(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, web.WidgetFields(s.dsFieldVMs(r.Context(), r.URL.Query().Get("type"))))
}

func (s *Server) handleDataSourceCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	typeID := r.FormValue("type")
	typ, ok := s.dsRegistry.Get(typeID)
	if name == "" || !ok {
		http.Error(w, "invalid data source", http.StatusBadRequest)
		return
	}

	config := map[string]string{}
	for _, f := range typ.Config.Fields {
		config[f.Name] = r.FormValue(f.Name)
	}
	cfgJSON, _ := json.Marshal(config)

	secretCipher := ""
	if len(typ.Credential.Fields) > 0 {
		secret := map[string]string{}
		for _, f := range typ.Credential.Fields {
			secret[f.Name] = r.FormValue(f.Name)
		}
		sj, _ := json.Marshal(secret)
		if c, err := crypto.Seal(sj, s.cfg.EncryptionKey); err == nil {
			secretCipher = c
		}
	}

	if _, err := s.store.CreateDataSource(r.Context(), dbgen.CreateDataSourceParams{
		Name: name, Type: typeID, ConfigJson: string(cfgJSON), SecretCiphertext: secretCipher,
	}); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.DataSourceList(s.dataSourceVMs(r.Context())))
}

// handleDataSourceEdit renders a form to edit a data source's config (e.g. the
// iCal/RSS URL or text lines) after creation.
func (s *Server) handleDataSourceEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ds, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	typ, ok := s.dsRegistry.Get(ds.Type)
	if !ok {
		http.Error(w, "unknown type", http.StatusBadRequest)
		return
	}
	values := map[string]string{}
	_ = json.Unmarshal([]byte(ds.ConfigJson), &values)
	typeName := i18n.T(r.Context(), typ.NameKey)
	rm := ""
	if ds.RefreshIntervalSecs > 0 {
		rm = strconv.FormatInt(ds.RefreshIntervalSecs/60, 10)
	}
	s.render(w, r, web.DataSourceEditPage(ds.ID, ds.Name, typeName, schemaFieldVMs(r.Context(), typ.Config, values), rm))
}

// handleDataSourceUpdate saves edited config, keeping keys not present in the
// config schema (e.g. an OAuth resource id) intact, then refreshes affected widgets.
func (s *Server) handleDataSourceUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ds, err := s.store.GetDataSource(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	typ, ok := s.dsRegistry.Get(ds.Type)
	if !ok {
		http.Error(w, "unknown type", http.StatusBadRequest)
		return
	}
	config := map[string]string{}
	_ = json.Unmarshal([]byte(ds.ConfigJson), &config) // preserve non-schema keys
	for _, f := range typ.Config.Fields {
		config[f.Name] = r.FormValue(f.Name)
	}
	cj, _ := json.Marshal(config)
	if err := s.store.UpdateDataSourceConfig(r.Context(), dbgen.UpdateDataSourceConfigParams{ConfigJson: string(cj), ID: id}); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	if name := r.FormValue("name"); name != "" && name != ds.Name {
		_ = s.store.UpdateDataSourceName(r.Context(), dbgen.UpdateDataSourceNameParams{Name: name, ID: id})
	}
	// Per-source refresh override in minutes (0/blank = use the global default).
	if m, perr := strconv.Atoi(r.FormValue("refresh_minutes")); perr == nil && m >= 0 {
		if m > 1440 {
			m = 1440
		}
		_ = s.store.UpdateDataSourceRefreshInterval(r.Context(), dbgen.UpdateDataSourceRefreshIntervalParams{
			RefreshIntervalSecs: int64(m) * 60, ID: id,
		})
	}
	s.refreshWidgetsUsingSource(r.Context(), id)
	http.Redirect(w, r, "/admin/datasources", http.StatusSeeOther)
}

// refreshWidgetsUsingSource re-fetches every widget linked to the given source,
// so a config edit takes effect immediately on the kiosk.
func (s *Server) refreshWidgetsUsingSource(ctx context.Context, dsID int64) {
	widgets, err := s.store.ListWidgets(ctx)
	if err != nil {
		return
	}
	for _, wgt := range widgets {
		for _, ws := range s.wsRows(ctx, wgt.ID) {
			if ws.DataSourceID == dsID {
				s.brk.RefreshWidget(ctx, wgt)
				break
			}
		}
	}
}

// wsRows lists a widget's source links (empty on error).
func (s *Server) wsRows(ctx context.Context, widgetID int64) []dbgen.ListWidgetSourcesRow {
	rows, _ := s.store.ListWidgetSources(ctx, widgetID)
	return rows
}

func (s *Server) handleDataSourceDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteDataSource(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.DataSourceList(s.dataSourceVMs(r.Context())))
}

func (s *Server) handleWidgetSourceAdd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	dsID, err := strconv.ParseInt(r.FormValue("data_source_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad data source", http.StatusBadRequest)
		return
	}
	pos, _ := s.store.MaxWidgetSourcePosition(r.Context(), id)
	if _, err := s.store.AddWidgetSource(r.Context(), dbgen.AddWidgetSourceParams{
		WidgetID: id, DataSourceID: dsID, Filter: r.FormValue("filter"), Position: pos + 1,
	}); err != nil {
		http.Error(w, "add failed", http.StatusInternalServerError)
		return
	}
	s.refreshWidgetCache(r.Context(), id)
	s.renderWidgetSources(w, r, id)
}

func (s *Server) handleWidgetSourceDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	linkID, err := strconv.ParseInt(chi.URLParam(r, "linkID"), 10, 64)
	if err != nil {
		http.Error(w, "bad link id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteWidgetSource(r.Context(), linkID); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.refreshWidgetCache(r.Context(), id)
	s.renderWidgetSources(w, r, id)
}

func (s *Server) handleWidgetSourceResource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	linkID, err := strconv.ParseInt(chi.URLParam(r, "linkID"), 10, 64)
	if err != nil {
		http.Error(w, "bad link id", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateWidgetSourceResource(r.Context(), dbgen.UpdateWidgetSourceResourceParams{
		Resource: r.FormValue("resource"), ID: linkID,
	}); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	s.refreshWidgetCache(r.Context(), id)
	s.renderWidgetSources(w, r, id)
}

func (s *Server) handleWidgetSourceColor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	linkID, err := strconv.ParseInt(chi.URLParam(r, "linkID"), 10, 64)
	if err != nil {
		http.Error(w, "bad link id", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateWidgetSourceColor(r.Context(), dbgen.UpdateWidgetSourceColorParams{
		Color: r.FormValue("color"), ID: linkID,
	}); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	s.refreshWidgetCache(r.Context(), id)
	s.renderWidgetSources(w, r, id)
}

func (s *Server) renderWidgetSources(w http.ResponseWriter, r *http.Request, widgetID int64) {
	wgt, err := s.store.GetWidget(r.Context(), widgetID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	typ, _ := s.registry.Get(wgt.Type)
	s.render(w, r, web.WidgetSources(widgetID, s.widgetSourceVMs(r.Context(), widgetID), s.availableSourcesFor(r.Context(), typ)))
}

func (s *Server) refreshWidgetCache(ctx context.Context, widgetID int64) {
	if wgt, err := s.store.GetWidget(ctx, widgetID); err == nil {
		s.brk.RefreshWidget(ctx, wgt)
	}
}

func (s *Server) dsTypeVMs(ctx context.Context) []web.WidgetTypeVM {
	out := []web.WidgetTypeVM{}
	for _, t := range s.dsRegistry.Types() {
		out = append(out, web.WidgetTypeVM{ID: t.ID, Name: i18n.T(ctx, t.NameKey)})
	}
	return out
}

func (s *Server) dsFieldVMs(ctx context.Context, typeID string) []web.FieldVM {
	typ, ok := s.dsRegistry.Get(typeID)
	if !ok {
		return nil
	}
	out := schemaFieldVMs(ctx, typ.Config, nil)
	out = append(out, schemaFieldVMs(ctx, typ.Credential, nil)...)
	return out
}

func (s *Server) dataSourceVMs(ctx context.Context) []web.DataSourceVM {
	rows, err := s.store.ListDataSources(ctx)
	if err != nil {
		return nil
	}
	out := make([]web.DataSourceVM, 0, len(rows))
	for _, d := range rows {
		var c struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal([]byte(d.ConfigJson), &c)
		typ, _ := s.dsRegistry.Get(d.Type)
		isOAuth := typ.CredKind == "oauth2"
		level, text := dsHealthDisplay(d, isOAuth, s.now())
		out = append(out, web.DataSourceVM{
			ID: d.ID, Name: d.Name, Type: d.Type, URL: c.URL,
			IsOAuth:     isOAuth,
			Connected:   d.OauthStatus == "connected",
			HasConfig:   len(typ.Config.Fields) > 0,
			HealthLevel: level,
			HealthText:  text,
			LastError:   d.LastError,
			LastSync:    d.LastSync,
		})
	}
	return out
}

func (s *Server) handleDataSourceRename(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if name := r.FormValue("name"); name != "" {
		_ = s.store.UpdateDataSourceName(r.Context(), dbgen.UpdateDataSourceNameParams{Name: name, ID: id})
	}
	s.render(w, r, web.DataSourceList(s.dataSourceVMs(r.Context())))
}

// dsHealthDisplay maps a data source's stored health into a status pill
// (level, Dutch label).
func dsHealthDisplay(d dbgen.DataSource, isOAuth bool, now time.Time) (level, text string) {
	if !isOAuth {
		// Non-OAuth sources (ical/rss/text/video/bring): report last fetch outcome.
		switch d.Health {
		case "ok":
			return "ok", "Werkt"
		case "error":
			return "warn", "Ophalen mislukt"
		default:
			return "", "" // never fetched yet
		}
	}
	switch {
	case d.Health == "reconnect" || d.OauthStatus != "connected":
		return "error", "Opnieuw verbinden"
	case d.Health == "error":
		return "warn", "Sync mislukt"
	}
	// A past access-token expiry is normal (refreshed automatically from the
	// refresh token); only a dead refresh token or a failed sync is a problem.
	return "ok", "Verbonden"
}

func (s *Server) widgetSourceVMs(ctx context.Context, widgetID int64) []web.WidgetSourceVM {
	rows, err := s.store.ListWidgetSources(ctx, widgetID)
	if err != nil {
		return nil
	}
	// Colour coding is meaningful for the calendar widget (events tinted by source).
	showColor := false
	if wgt, err := s.store.GetWidget(ctx, widgetID); err == nil {
		showColor = wgt.Type == "calendar"
	}
	out := make([]web.WidgetSourceVM, 0, len(rows))
	for _, r := range rows {
		vm := web.WidgetSourceVM{
			LinkID: r.ID, SourceName: r.SourceName, SourceType: r.SourceType, Filter: r.Filter,
			ShowColor: showColor, Color: r.Color,
		}
		// If the data-source type has a resource picker, fetch its options live
		// and mark the link's current choice.
		if typ, ok := s.dsRegistry.Get(r.SourceType); ok && typ.ResourceKind != "" {
			vm.HasPicker = true
			vm.ResourceLabel = i18n.T(ctx, typ.ResourceLabelKey)
			if ds, err := s.store.GetDataSource(ctx, r.DataSourceID); err == nil {
				if opts, err := s.listResources(ctx, ds); err == nil {
					for _, o := range opts {
						vm.ResourceOptions = append(vm.ResourceOptions, web.OptionVM{
							Value: o.ID, Label: o.Name, Selected: o.ID == r.Resource,
						})
					}
				}
			}
		}
		out = append(out, vm)
	}
	return out
}
