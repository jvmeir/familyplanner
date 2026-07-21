package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
)

func TestDeviceDelete(t *testing.T) {
	handler, store := newTestHandlerStore(t)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	c := loggedInClient(t, ts)

	// Seed a throwaway "test kiosk" device.
	dev, err := store.CreateDevice(context.Background(), dbgen.CreateDeviceParams{
		Name: "test-kiosk", TokenHash: "deadbeef",
	})
	require.NoError(t, err)

	// Grab a CSRF token from the devices page.
	page, err := c.Get(ts.URL + "/admin/devices")
	require.NoError(t, err)
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	require.Contains(t, string(html), "test-kiosk")
	m := csrfRe.FindStringSubmatch(string(html))
	require.Len(t, m, 2)

	// Delete without a token is rejected (CSRF-protected).
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/admin/devices/"+strconv.FormatInt(dev.ID, 10), nil)
	resp, err := c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Delete with the token succeeds and the device is gone.
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/admin/devices/"+strconv.FormatInt(dev.ID, 10), nil)
	req.Header.Set("X-CSRF-Token", m[1])
	resp, err = c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	list, err := store.ListDevices(context.Background())
	require.NoError(t, err)
	require.Empty(t, list, "device removed")
}
