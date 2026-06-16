package broker_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvmeir/familyplanner/internal/broker"
	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/widget"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *db.Store {
	t.Helper()
	st, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.DB.Close() })
	return st
}

func TestRefreshWritesCache(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	b := broker.New(st, reg, time.Now, make([]byte, 32), nil)

	wgt, err := st.CreateWidget(ctx, dbgen.CreateWidgetParams{
		Name: "Kerst", Type: "countdown", ConfigJson: `{"title":"Kerst","date":"2026-12-25"}`,
	})
	require.NoError(t, err)

	b.RefreshWidget(ctx, wgt)

	cache, err := st.GetWidgetCache(ctx, wgt.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", cache.Status)
	require.Contains(t, cache.DataJson, "Kerst")
}

func TestRefreshMarksErrorOnBadConfig(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	b := broker.New(st, reg, time.Now, make([]byte, 32), nil)

	wgt, err := st.CreateWidget(ctx, dbgen.CreateWidgetParams{
		Name: "Bad", Type: "countdown", ConfigJson: `{"title":"x","date":"not-a-date"}`,
	})
	require.NoError(t, err)

	b.RefreshWidget(ctx, wgt) // fetch fails (bad date) -> error row, no last-good

	cache, err := st.GetWidgetCache(ctx, wgt.ID)
	require.NoError(t, err)
	require.Equal(t, "error", cache.Status)
	require.NotEmpty(t, cache.ErrorMsg)
}
