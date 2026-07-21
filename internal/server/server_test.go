package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvmeir/familyplanner/internal/config"
	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/server"
	"github.com/jvmeir/familyplanner/internal/widget"
	"github.com/stretchr/testify/require"
)

func newTestHandler(t *testing.T) http.Handler {
	h, _ := newTestHandlerStore(t)
	return h
}

// newTestHandlerStore is like newTestHandler but also returns the store, so
// tests can seed rows (e.g. an unconnected OAuth source for health checks).
func newTestHandlerStore(t *testing.T) (http.Handler, *db.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Env: "dev", Addr: ":0", BaseURL: "http://localhost",
		DataDir: dir, DBPath: filepath.Join(dir, "t.db"),
		EncryptionKey: make([]byte, 32), AdminPassphrase: "secret",
		DefaultLocale: "nl", TimeZone: time.UTC, SessionTTL: time.Hour,
	}
	store, err := db.Open(context.Background(), cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.DB.Close() })

	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	i18nSvc, err := i18n.New("nl")
	require.NoError(t, err)

	srv, err := server.New(cfg, store, reg, i18nSvc)
	require.NoError(t, err)
	return srv.Handler(), store
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestHealth(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "ok", string(body))
}

func TestAdminRequiresLogin(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := noRedirect().Get(ts.URL + "/admin")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestKioskRedirectsToPair(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := noRedirect().Get(ts.URL + "/kiosk")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/pair", resp.Header.Get("Location"))
}

func TestWrongPassphrase(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := noRedirect().PostForm(ts.URL+"/login", map[string][]string{"passphrase": {"nope"}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLoginThenAdminThenKiosk(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar} // follows redirects, keeps cookies

	// Login -> follows 303 to /admin, which renders the Dutch welcome.
	resp, err := client.PostForm(ts.URL+"/login", map[string][]string{"passphrase": {"secret"}})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Welkom")

	// Pair the kiosk -> follows 303 to /kiosk, which shows the seeded countdown.
	resp, err = client.PostForm(ts.URL+"/pair", map[string][]string{"passphrase": {"secret"}})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Kerst") // title is standardized to the widget name
}
