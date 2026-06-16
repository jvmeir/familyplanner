package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLayoutEditor(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	// View 1 is the seeded "Demo" view; its layout is a row split (countdown|clock).
	resp, err := c.Get(ts.URL + "/admin/views/1/layout")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "editor-canvas")
	require.Contains(t, string(body), "egutter") // a divider exists between the two panes
	token := csrfRe.FindStringSubmatch(string(body))[1]

	// Split the first pane vertically -> the returned fragment has a col split.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/views/1/layout/split?dir=col&path=0", nil)
	req.Header.Set("X-CSRF-Token", token)
	resp, err = c.Do(req)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "dir-col")

	// Resize the root split's children (drag-end persists weights).
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/admin/views/1/layout/weights",
		strings.NewReader("path=&weights=2,1"))
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}
