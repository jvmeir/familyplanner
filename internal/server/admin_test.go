package server_test

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func loggedInClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.PostForm(ts.URL+"/login", url.Values{"passphrase": {"secret"}})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return c
}

var csrfRe = regexp.MustCompile(`name="csrf-token" content="([^"]+)"`)
var widgetIDRe = regexp.MustCompile(`/admin/widgets/(\d+)`)

func TestAdminWidgetCRUD(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	// Load the widgets page and grab the CSRF token.
	resp, err := c.Get(ts.URL + "/admin/widgets")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	m := csrfRe.FindStringSubmatch(string(body))
	require.Len(t, m, 2, "csrf token must be present")
	token := m[1]

	// Create without a token is rejected.
	resp, err = c.PostForm(ts.URL+"/admin/widgets", url.Values{
		"name": {"Verjaardag"}, "type": {"countdown"}, "title": {"X"}, "date": {"2026-01-01"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Create with the token succeeds and the widget appears in the returned list.
	resp, err = c.PostForm(ts.URL+"/admin/widgets", url.Values{
		"_csrf": {token}, "name": {"Verjaardag"}, "type": {"countdown"}, "title": {"X"}, "date": {"2026-01-01"},
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Verjaardag")

	// Delete it (sorted last by name) via HTMX-style header CSRF.
	ids := widgetIDRe.FindAllStringSubmatch(string(body), -1)
	require.NotEmpty(t, ids)
	lastID := ids[len(ids)-1][1]
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/admin/widgets/"+lastID, nil)
	req.Header.Set("X-CSRF-Token", token)
	resp, err = c.Do(req)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, string(body), "Verjaardag")
}

func TestAdminViewCreate(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	resp, err := c.Get(ts.URL + "/admin/views")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	token := csrfRe.FindStringSubmatch(string(body))[1]

	resp, err = c.PostForm(ts.URL+"/admin/views", url.Values{
		"_csrf": {token}, "name": {"Keuken"}, "cols": {"2"}, "rows": {"2"}, "theme": {"donker"},
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Keuken")
}
