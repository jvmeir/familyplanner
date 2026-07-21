package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTickerMergesTextAndRSS(t *testing.T) {
	const rss = `<?xml version="1.0"?><rss><channel>
		<item><title>Kop een</title></item>
		<item><title>Kop twee</title></item>
	</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rss))
	}))
	defer srv.Close()

	sources := []SourceInput{
		{Type: "text", Config: json.RawMessage(`{"lines":"Regel A\nRegel B\n\n"}`)},
		{Type: "rss", Config: json.RawMessage(`{"url":"` + srv.URL + `"}`)},
	}
	p, err := newTicker(nil, sources, nil)
	require.NoError(t, err)
	d, ttl, err := p.Fetch(context.Background())
	require.NoError(t, err)
	require.Positive(t, ttl)
	td, ok := d.(TickerData)
	require.True(t, ok)
	require.Equal(t, []string{"Regel A", "Regel B", "Kop een", "Kop twee"}, td.Items)
}

func TestFetchRSSAtom(t *testing.T) {
	const atom = `<?xml version="1.0"?><feed><entry><title>Atom een</title></entry></feed>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(atom))
	}))
	defer srv.Close()

	titles, err := fetchRSS(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, []string{"Atom een"}, titles)
}
