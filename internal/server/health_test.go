package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
)

// The server-rendered kiosk shows the health badge only when something is wrong.
func TestKioskHealthBadgeHiddenWhenHealthy(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	resp, err := c.Get(ts.URL + "/kiosk")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, string(body), "khealth", "no badge when all healthy")
}

func TestKioskHealthBadgeShownOnReconnect(t *testing.T) {
	handler, store := newTestHandlerStore(t)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	c := pairedClient(t, ts)

	// An OAuth (ms_graph) source that was never connected -> reconnect needed.
	_, err := store.CreateDataSource(context.Background(), dbgen.CreateDataSourceParams{
		Name: "Outlook", Type: "ms_graph", ConfigJson: "{}", SecretCiphertext: "",
	})
	require.NoError(t, err)

	resp, err := c.Get(ts.URL + "/kiosk")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "khealth-error", "red badge rendered")
	require.Contains(t, string(body), "Outlook: opnieuw verbinden")
}
