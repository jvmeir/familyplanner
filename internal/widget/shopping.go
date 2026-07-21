package widget

import (
	"context"
	"encoding/json"
	"time"
)

// ShoppingHeaderPrefix marks a category-header line in a grouped shopping list
// (an otherwise plain string item); the renderer shows it as a header and
// indents the items beneath it.
const ShoppingHeaderPrefix = ""

// ShoppingData is the normalized render data (a flat item list; grouped lists
// carry ShoppingHeaderPrefix-marked header lines between the groups).
type ShoppingData struct {
	Items []string `json:"items"`
}

type shoppingProvider struct {
	sources []SourceInput
	group   bool // group items by Bring aisle/section, with header lines
}

func newShopping(raw json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	var cfg struct {
		Group string `json:"group"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return shoppingProvider{sources: sources, group: cfg.Group == "yes"}, nil
}

func decodeShopping(raw json.RawMessage) (Data, error) {
	var d ShoppingData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p shoppingProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	var items []string
	var firstErr error
	ok := 0
	for _, s := range p.sources {
		if s.Type != "bring" {
			continue
		}
		var cred struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		_ = json.Unmarshal(s.Secret, &cred)
		if cred.Email == "" || cred.Password == "" {
			continue
		}
		// The list is chosen per widget↔source link (empty = default list).
		its, err := bringFetch(ctx, cred.Email, cred.Password, s.Resource, p.group)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		items = append(items, its...)
		ok++
	}
	if ok == 0 && firstErr != nil {
		return nil, 0, firstErr
	}
	return ShoppingData{Items: items}, 5 * time.Minute, nil
}
