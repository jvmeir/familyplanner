package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminPlaylistCreate(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	resp, err := c.Get(ts.URL + "/admin/playlists")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Standaard") // the seeded default playlist
	token := csrfRe.FindStringSubmatch(string(body))[1]

	resp, err = c.PostForm(ts.URL+"/admin/playlists", url.Values{
		"_csrf": {token}, "name": {"Avond"}, "interval": {"20"},
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Avond")
}

func TestAdminDevicesAndRemote(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	resp, err := c.Get(ts.URL + "/admin/devices")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	token := csrfRe.FindStringSubmatch(string(body))[1]

	// Remote control of a device with no open stream is a no-op but still 204.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/devices/1/control/pause", nil)
	req.Header.Set("X-CSRF-Token", token)
	resp, err = c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}
