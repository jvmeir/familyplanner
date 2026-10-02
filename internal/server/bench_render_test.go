package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/layout"
	"github.com/stretchr/testify/require"
)

// seedCountdownView builds a view whose layout is a single row of n countdown
// leaves, mirroring the "Aftellen" view on the deployed kiosk.
func seedCountdownView(b *testing.B, store interface {
	CreateWidget(context.Context, dbgen.CreateWidgetParams) (dbgen.Widget, error)
	CreateView(context.Context, dbgen.CreateViewParams) (dbgen.View, error)
	SetViewLayout(context.Context, dbgen.SetViewLayoutParams) error
}, n int) int64 {
	ctx := context.Background()
	view, err := store.CreateView(ctx, dbgen.CreateViewParams{
		Name: fmt.Sprintf("bench-%d", n), Cols: 1, Rows: 1, DwellSeconds: 15,
	})
	require.NoError(b, err)

	children := make([]layout.Child, 0, n)
	for i := 0; i < n; i++ {
		w, err := store.CreateWidget(ctx, dbgen.CreateWidgetParams{
			Name: fmt.Sprintf("cd-%d", i), Type: "countdown",
			ConfigJson: `{"date":"2026-12-25","precision":"dhms","time":""}`,
		})
		require.NoError(b, err)
		children = append(children, layout.Child{Weight: 1, Node: layout.SingleLeaf(w.ID)})
	}
	// A split needs >=2 children to validate; a single tile is a bare leaf.
	root := layout.Node{Split: &layout.Split{Dir: layout.Row, Children: children}}
	if n == 1 {
		root = children[0].Node
	}
	js, err := json.Marshal(root)
	require.NoError(b, err)
	require.NoError(b, store.SetViewLayout(ctx, dbgen.SetViewLayoutParams{
		ID: view.ID, LayoutJson: string(js),
	}))
	return view.ID
}

func benchRender(b *testing.B, tiles int) {
	h, store := newTestHandlerStoreB(b)
	viewID := seedCountdownView(b, store, tiles)
	ts := httptest.NewServer(h)
	defer ts.Close()
	c := pairedClientB(b, ts)
	url := fmt.Sprintf("%s/kiosk/view/%d", ts.URL, viewID)

	// warm the widget cache so we measure the steady-state render, not first fetch
	for i := 0; i < 2; i++ {
		resp, err := c.Get(url)
		require.NoError(b, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(url)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("status %d", resp.StatusCode)
		}
	}
}

func BenchmarkKioskRender1Tile(b *testing.B)  { benchRender(b, 1) }
func BenchmarkKioskRender5Tiles(b *testing.B) { benchRender(b, 5) }
func BenchmarkKioskRender9Tiles(b *testing.B) { benchRender(b, 9) }
