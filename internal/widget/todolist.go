package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TodoData is the normalized render data (open task titles).
type TodoData struct {
	Items []string `json:"items"`
}

type todoProvider struct {
	sources   []SourceInput
	scope     string // "" / "all" = all open tasks; "today_overdue" = due today or earlier
	hideNoDue bool   // drop tasks that have no due date
	allLists  bool   // query every To Do list instead of the one linked/selected
	now       NowFunc
}

func newTodo(raw json.RawMessage, sources []SourceInput, now NowFunc) (Provider, error) {
	var cfg struct {
		Scope     string `json:"scope"`
		HideNoDue string `json:"hide_no_due"`
		AllLists  string `json:"all_lists"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	if now == nil {
		now = time.Now
	}
	return todoProvider{sources: sources, scope: cfg.Scope, hideNoDue: cfg.HideNoDue == "yes", allLists: cfg.AllLists == "yes", now: now}, nil
}

// todoDueLabel renders a task's due date as a short Dutch status: "te laat"
// (overdue), "vandaag" (today), or a short date; "" when there is no due date.
func todoDueLabel(due, now time.Time) string {
	if due.IsZero() {
		return ""
	}
	today, dd := dateOf(now), dateOf(due)
	switch {
	case dd.Before(today):
		return "te laat"
	case dd.Equal(today):
		return "vandaag"
	default:
		return fmt.Sprintf("%s %d %s", nlWeekday[due.Weekday()], due.Day(), nlMonthShort[int(due.Month())-1])
	}
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
		if sec.AccessToken == "" {
			continue
		}
		// Decide which lists to query: every list, or the linked/selected one
		// (falling back to the account's default list when none is picked).
		var listIDs []string
		if p.allLists {
			if lists, err := GraphListTodoLists(ctx, sec.AccessToken); err == nil {
				for _, l := range lists {
					listIDs = append(listIDs, l.ID)
				}
			}
		} else {
			listID := cfg.ListID
			if s.Resource != "" {
				listID = s.Resource
			}
			if listID == "" {
				if lists, err := GraphListTodoLists(ctx, sec.AccessToken); err == nil && len(lists) > 0 {
					listID = lists[0].ID
				}
			}
			if listID != "" {
				listIDs = []string{listID}
			}
		}
		if len(listIDs) == 0 {
			continue
		}
		for _, lid := range listIDs {
			tasks, err := GraphTodoTasks(ctx, sec.AccessToken, lid)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, t := range tasks {
				if p.hideNoDue && t.Due.IsZero() {
					continue // caller only wants dated tasks
				}
				if p.scope == "today_overdue" {
					// Only tasks with a due date at/before end of today.
					if t.Due.IsZero() || t.Due.After(endOfToday) {
						continue
					}
				}
				// Suffix a short due label ("· vandaag" / "· te laat" / "· di 22 jul").
				if label := todoDueLabel(t.Due, now); label != "" {
					items = append(items, t.Title+" · "+label)
				} else {
					items = append(items, t.Title)
				}
			}
			ok++
		}
	}
	if ok == 0 && firstErr != nil {
		return nil, 0, firstErr
	}
	return TodoData{Items: items}, 10 * time.Minute, nil
}
