package widget

import (
	"context"
	"encoding/json"
	"time"
)

// TodoData is the normalized render data (open task titles).
type TodoData struct {
	Items []string `json:"items"`
}

type todoProvider struct{ sources []SourceInput }

func newTodo(_ json.RawMessage, sources []SourceInput, _ NowFunc) (Provider, error) {
	return todoProvider{sources: sources}, nil
}

func decodeTodo(raw json.RawMessage) (Data, error) {
	var d TodoData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p todoProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	var items []string
	var firstErr error
	ok := 0
	for _, s := range p.sources {
		if s.Type != "ms_todo" {
			continue
		}
		var sec struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.Unmarshal(s.Secret, &sec)
		var cfg struct {
			ListID string `json:"list_id"`
		}
		_ = json.Unmarshal(s.Config, &cfg)
		listID := cfg.ListID
		if s.Resource != "" {
			listID = s.Resource
		}
		if sec.AccessToken == "" || listID == "" {
			continue
		}
		its, err := GraphTodoTasks(ctx, sec.AccessToken, listID)
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
	return TodoData{Items: items}, 10 * time.Minute, nil
}
