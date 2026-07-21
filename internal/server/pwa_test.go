package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceWorkerServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	// Public (no pairing) — the SW must load before any auth.
	resp, err := http.Get(ts.URL + "/sw.js")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "javascript")
	// Root scope is required so the worker controls the whole origin.
	require.Equal(t, "/", resp.Header.Get("Service-Worker-Allowed"))
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "addEventListener")
	require.Contains(t, string(body), "fp-v1")
}

func TestManifestsServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	for _, tc := range []struct {
		path        string
		wantStart   string
		wantDisplay string
	}{
		{"/manifest.webmanifest", "/", "standalone"},
	} {
		resp, err := http.Get(ts.URL + tc.path)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, tc.path)
		require.Contains(t, resp.Header.Get("Content-Type"), "manifest+json", tc.path)

		var m struct {
			Name      string `json:"name"`
			StartURL  string `json:"start_url"`
			Display   string `json:"display"`
			Scope     string `json:"scope"`
			Icons     []struct {
				Src   string `json:"src"`
				Sizes string `json:"sizes"`
			} `json:"icons"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m), tc.path)
		resp.Body.Close()
		require.Equal(t, tc.wantStart, m.StartURL, tc.path)
		require.Equal(t, tc.wantDisplay, m.Display, tc.path)
		require.Equal(t, "/", m.Scope, tc.path)
		require.GreaterOrEqual(t, len(m.Icons), 2, tc.path)
	}
}

func TestIconsServed(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	for _, p := range []string{"/static/icon-192.png", "/static/icon-512.png"} {
		resp, err := http.Get(ts.URL + p)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, p)
		require.Contains(t, resp.Header.Get("Content-Type"), "image/png", p)
		resp.Body.Close()
	}
}
