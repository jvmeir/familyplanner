package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/kioskapi"
)

func TestAPIKioskState(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	resp, err := c.Get(ts.URL + "/api/kiosk/state")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var st kioskapi.State
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&st))
	require.Equal(t, "Standaard", st.PlaylistName)      // seeded default playlist
	require.NotZero(t, st.CurrentID)                    // first playlist view
	require.GreaterOrEqual(t, len(st.Playlist), 2)      // Demo + Aftellen
	require.GreaterOrEqual(t, len(st.All), 2)
}

func TestAPIKioskView(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	// View 1 = seeded "Demo": a row split of countdown (2) beside clock (1).
	resp, err := c.Get(ts.URL + "/api/kiosk/view/1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var vr kioskapi.ViewRender
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&vr))
	require.Equal(t, int64(1), vr.ID)
	require.NotEmpty(t, vr.ThemeVars)
	require.Equal(t, "row", vr.Layout.Dir)
	require.Len(t, vr.Layout.Children, 2)

	// The first leaf is the countdown widget with title "Kerstmis".
	countdown := vr.Layout.Children[0].Node.Cell
	require.NotNil(t, countdown)
	require.Equal(t, "countdown", countdown.Kind)
	require.Equal(t, "Kerstmis", countdown.Title)
}

func TestSPABootstrapServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	resp, err := c.Get(ts.URL + "/spa")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}
