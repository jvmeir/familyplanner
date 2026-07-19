package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/health"
)

func TestAPIKioskHealthAllOK(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	resp, err := c.Get(ts.URL + "/api/kiosk/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var sum health.Summary
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sum))
	// Fresh seed: countdown/clock widgets (no sources), nothing unhealthy.
	require.Equal(t, health.LevelOK, sum.Level)
	require.Equal(t, 0, sum.Count)
}

func TestAPIKioskHealthReconnect(t *testing.T) {
	handler, store := newTestHandlerStore(t)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	c := pairedClient(t, ts)

	// Create an OAuth (ms_graph) data source that was never connected -> the kiosk
	// must flag it as needing an interactive reconnect.
	_, err := store.CreateDataSource(context.Background(), dbgen.CreateDataSourceParams{
		Name: "Outlook", Type: "ms_graph", ConfigJson: "{}", SecretCiphertext: "",
	})
	require.NoError(t, err)

	resp, err := c.Get(ts.URL + "/api/kiosk/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	var sum health.Summary
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sum))
	require.Equal(t, health.LevelError, sum.Level)
	require.GreaterOrEqual(t, sum.Count, 1)
	require.Equal(t, "reconnect", sum.Issues[0].Kind)
	require.Contains(t, sum.Issues[0].Message, "Outlook")
}
