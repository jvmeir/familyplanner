package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	st, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.DB.Close() })
	return st
}

func TestMigrationsAndCRUD(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// settings upsert
	require.NoError(t, st.SetSetting(ctx, dbgen.SetSettingParams{Key: "default_theme", Value: "licht"}))
	require.NoError(t, st.SetSetting(ctx, dbgen.SetSettingParams{Key: "default_theme", Value: "donker"}))
	v, err := st.GetSetting(ctx, "default_theme")
	require.NoError(t, err)
	require.Equal(t, "donker", v)

	// empty to start
	n, err := st.CountViews(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)

	// three-layer model: view <- placement -> widget
	view, err := st.CreateView(ctx, dbgen.CreateViewParams{
		Name: "Demo", Cols: 3, Rows: 2, ThemeID: "licht",
		InRotation: 1, RotationOrder: 0, DwellSeconds: 30,
	})
	require.NoError(t, err)
	require.NotZero(t, view.ID)

	w, err := st.CreateWidget(ctx, dbgen.CreateWidgetParams{
		Name: "Kerst", Type: "countdown", ConfigJson: `{"title":"Kerst","date":"2026-12-25"}`,
	})
	require.NoError(t, err)

	_, err = st.CreatePlacement(ctx, dbgen.CreatePlacementParams{
		ViewID: view.ID, WidgetID: w.ID, Col: 1, Row: 1, ColSpan: 2, RowSpan: 2,
		PlacementConfigJson: "{}",
	})
	require.NoError(t, err)

	placements, err := st.ListPlacementsByView(ctx, view.ID)
	require.NoError(t, err)
	require.Len(t, placements, 1)
	require.Equal(t, w.ID, placements[0].WidgetID)
}

func TestForeignKeyCascade(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	view, err := st.CreateView(ctx, dbgen.CreateViewParams{Name: "V", Cols: 1, Rows: 1, ThemeID: "", InRotation: 1, RotationOrder: 0, DwellSeconds: 30})
	require.NoError(t, err)
	w, err := st.CreateWidget(ctx, dbgen.CreateWidgetParams{Name: "W", Type: "clock", ConfigJson: "{}"})
	require.NoError(t, err)
	_, err = st.CreatePlacement(ctx, dbgen.CreatePlacementParams{ViewID: view.ID, WidgetID: w.ID, Col: 1, Row: 1, ColSpan: 1, RowSpan: 1, PlacementConfigJson: "{}"})
	require.NoError(t, err)

	// deleting the view cascades to its placements (foreign_keys pragma on)
	_, err = st.DB.ExecContext(ctx, "DELETE FROM views WHERE id = ?", view.ID)
	require.NoError(t, err)
	placements, err := st.ListPlacementsByView(ctx, view.ID)
	require.NoError(t, err)
	require.Empty(t, placements)
}
