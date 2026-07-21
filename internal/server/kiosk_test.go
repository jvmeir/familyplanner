package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// pairedClient returns an http.Client whose jar holds a valid kiosk device cookie.
func pairedClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.PostForm(ts.URL+"/pair", url.Values{"passphrase": {"secret"}})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return c
}

func TestKioskViewFragment(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	// View 1 is the seeded "Demo" view; it renders the reused countdown widget.
	resp, err := c.Get(ts.URL + "/kiosk/view/1")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Kerst") // title is standardized to the widget name
}

func TestKioskControlEndpoints(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	resp, err := c.Post(ts.URL+"/kiosk/control/pause", "", nil)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, err = c.Post(ts.URL+"/kiosk/control/bogus", "", nil)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestKioskStreamEmitsNavigate(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := pairedClient(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/kiosk/stream", nil)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// The first SSE message points the kiosk at the current view.
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	require.Contains(t, string(buf[:n]), "navigate")
}
