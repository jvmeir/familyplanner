package widget

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// TickerData is the merged list of scrolling-ticker items.
type TickerData struct {
	Items []string `json:"items"`
}

const tickerMaxItems = 50

type tickerProvider struct {
	order   string // "sequential" (source order) | "random" (shuffled)
	sources []SourceInput
}

func newTicker(raw json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	var cfg struct {
		Order string `json:"order"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return tickerProvider{order: cfg.Order, sources: sources}, nil
}

func decodeTicker(raw json.RawMessage) (Data, error) {
	var d TickerData
	err := json.Unmarshal(raw, &d)
	return d, err
}

// Fetch merges each linked source's items: RSS feed titles + text-list lines, in
// source order. Failing feeds are skipped (last-good handled by the broker).
func (p tickerProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	var items []string
	for _, s := range p.sources {
		switch s.Type {
		case "text":
			var c struct {
				Lines string `json:"lines"`
			}
			_ = json.Unmarshal(s.Config, &c)
			for _, ln := range strings.Split(c.Lines, "\n") {
				if ln = strings.TrimSpace(ln); ln != "" {
					items = append(items, ln)
				}
			}
		case "rss":
			var c struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(s.Config, &c)
			if c.URL != "" {
				if titles, err := fetchRSS(ctx, c.URL); err == nil {
					items = append(items, titles...)
				}
			}
		}
	}
	if p.order == "random" {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	}
	if len(items) > tickerMaxItems {
		items = items[:tickerMaxItems]
	}
	return TickerData{Items: items}, 5 * time.Minute, nil
}

// fetchRSS downloads a feed and returns item/entry titles. Handles both RSS 2.0
// (channel>item>title) and Atom (feed>entry>title) with one struct, since the
// unmatched path stays empty for whichever format isn't present.
func fetchRSS(ctx context.Context, rawURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FamilyPlanner/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss: status %d", resp.StatusCode)
	}
	var feed struct {
		Items []struct {
			Title string `xml:"title"`
		} `xml:"channel>item"`
		Entries []struct {
			Title string `xml:"title"`
		} `xml:"entry"`
	}
	dec := xml.NewDecoder(resp.Body)
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := dec.Decode(&feed); err != nil {
		return nil, err
	}
	var out []string
	for _, it := range feed.Items {
		if t := strings.TrimSpace(it.Title); t != "" {
			out = append(out, t)
		}
	}
	for _, e := range feed.Entries {
		if t := strings.TrimSpace(e.Title); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}
