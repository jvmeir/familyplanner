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

type todoProvider struct {
	sources []SourceInput
	scope   string // "" / "all" = all open tasks; "today_overdue" = due today or earlier
	now     NowFunc
}

func newTodo(raw json.RawMessage, sources []SourceInput, now NowFunc) (Provider, error) {
	var cfg struct {
		Scope string `json:"scope"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	if now == nil {
		now = time.Now
	}
	return todoProvider{sources: sources, scope: cfg.Scope, now: now}, nil
}

func decodeTodo(raw json.RawMessage) (Data, error) {
	var d TodoData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p todoProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	// End of today (local): a task is "today or overdue" if it's due at/before this.
	now := p.now()
	endOfToday := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())

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
		if sec.AccessToken == "" {
			continue
		}
		// No list picked: fall back to the account's default To Do list so a
		// freshly-linked widget still shows tasks without extra configuration.
		if listID == "" {
			if lists, err := GraphListTodoLists(ctx, sec.AccessToken); err == nil && len(lists) > 0 {
				listID = lists[0].ID
			}
		}
		if listID == "" {
			continue
		}
		tasks, err := GraphTodoTasks(ctx, sec.AccessToken, listID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, t := range tasks {
			if p.scope == "today_overdue" {
				// Only tasks with a due date at/before end of today.
				if t.Due.IsZero() || t.Due.After(endOfToday) {
					continue
				}
			}
			items = append(items, t.Title)
		}
		ok++
	}
	if ok == 0 && firstErr != nil {
		return nil, 0, firstErr
	}
	return TodoData{Items: items}, 10 * time.Minute, nil
}
