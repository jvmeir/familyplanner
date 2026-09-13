package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
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

// newTestHandlerStoreB mirrors newTestHandlerStore but takes a testing.TB so
// benchmarks can use it.
func newTestHandlerStoreB(tb testing.TB) (http.Handler, *db.Store) {
	tb.Helper()
	dir := tb.TempDir()
	cfg := &config.Config{
		Env: "dev", Addr: ":0", BaseURL: "http://localhost",
		DataDir: dir, DBPath: filepath.Join(dir, "t.db"),
		EncryptionKey: make([]byte, 32), AdminPassphrase: "secret",
		DefaultLocale: "nl", TimeZone: time.UTC, SessionTTL: time.Hour,
	}
	store, err := db.Open(context.Background(), cfg.DBPath)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = store.DB.Close() })

	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)
	i18nSvc, err := i18n.New("nl")
	require.NoError(tb, err)

	srv, err := server.New(cfg, store, reg, i18nSvc)
	require.NoError(tb, err)
	return srv.Handler(), store
}

func pairedClientB(tb testing.TB, ts *httptest.Server) *http.Client {
	tb.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(tb, err)
	c := &http.Client{Jar: jar}
	resp, err := c.PostForm(ts.URL+"/pair", url.Values{"passphrase": {"secret"}})
	require.NoError(tb, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return c
}
