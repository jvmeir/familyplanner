package widget

import (
	"context"
	"encoding/json"
	"time"
)

// QuoteData is the normalized render data.
type QuoteData struct {
	Text   string `json:"text"`
	Author string `json:"author"`
}

// A small built-in set of Dutch quotes; one is picked per day (no network).
var quotes = []QuoteData{
	{"De beste tijd om een boom te planten was twintig jaar geleden. De op een na beste tijd is nu.", "Chinees spreekwoord"},
	{"Geluk is niet kant-en-klaar. Het komt voort uit je eigen daden.", "Dalai Lama"},
	{"Doe wat je kunt, met wat je hebt, waar je bent.", "Theodore Roosevelt"},
	{"Een reis van duizend mijl begint met een enkele stap.", "Lao Tzu"},
	{"Het leven is wat je overkomt terwijl je andere plannen maakt.", "John Lennon"},
	{"Wie goed doet, goed ontmoet.", "Nederlands spreekwoord"},
	{"Verbeelding is belangrijker dan kennis.", "Albert Einstein"},
}

type quoteProvider struct{ now NowFunc }

func newQuote(_ json.RawMessage, _ []SourceInput, now NowFunc) (Provider, error) {
	if now == nil {
		now = time.Now
	}
	return quoteProvider{now: now}, nil
}

func decodeQuote(raw json.RawMessage) (Data, error) {
	var d QuoteData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p quoteProvider) Fetch(_ context.Context) (Data, time.Duration, error) {
	return quotes[p.now().YearDay()%len(quotes)], time.Hour, nil
}
