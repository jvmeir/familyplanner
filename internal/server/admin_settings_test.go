package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/voiceclock"
)

func TestSettingsPageRenders(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	c := loggedInClient(t, ts)

	resp, err := c.Get(ts.URL + "/admin/settings")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Spraakklok")           // voice clock section
	require.Contains(t, string(body), `name="voice_enabled"`) // toggle present
}

func TestSettingsSavePersists(t *testing.T) {
	handler, store := newTestHandlerStore(t)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	c := loggedInClient(t, ts)

	// Need the CSRF token for the POST (admin group is CSRF-protected).
	page, err := c.Get(ts.URL + "/admin/settings")
	require.NoError(t, err)
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	m := csrfRe.FindStringSubmatch(string(html))
	require.Len(t, m, 2, "csrf token must be present")
	token := m[1]

	form := url.Values{
		"_csrf":        {token},
		"voice_enabled": {""}, // unchecked -> disabled
		"quiet_start":  {"23:00"},
		"quiet_end":    {"06:30"},
	}
	resp, err := c.PostForm(ts.URL+"/admin/settings", form)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	raw, err := store.GetSetting(context.Background(), "voiceclock")
	require.NoError(t, err)
	var cfg voiceclock.Config
	require.NoError(t, json.Unmarshal([]byte(raw), &cfg))
	require.False(t, cfg.Enabled)
	require.Equal(t, "23:00", cfg.QuietStart)
	require.Equal(t, "06:30", cfg.QuietEnd)
}
