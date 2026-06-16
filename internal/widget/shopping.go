package widget

import (
	"context"
	"encoding/json"
	"time"
)

// ShoppingData is the normalized render data (a flat item list).
type ShoppingData struct {
	Items []string `json:"items"`
}

type shoppingProvider struct{ sources []SourceInput }

func newShopping(_ json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	return shoppingProvider{sources: sources}, nil
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
		var cfg struct {
			List string `json:"list"`
		}
		_ = json.Unmarshal(s.Config, &cfg)
		if cred.Email == "" || cred.Password == "" {
			continue
		}
		list := cfg.List
		if s.Resource != "" {
			list = s.Resource // per-widget list choice wins
		}
		its, err := bringFetch(ctx, cred.Email, cred.Password, list)
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
