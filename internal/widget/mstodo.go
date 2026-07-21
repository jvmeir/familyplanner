package widget

import (
	"context"
	"time"
)

// TodoAllLists is the sentinel resource id meaning "every To Do list" (rather
// than one specific list), selectable on ms_todo sources.
const TodoAllLists = "__all__"

// TodoTask is one open Microsoft To Do task with its due date (zero if none).
type TodoTask struct {
	Title string
	Due   time.Time
}

// GraphTodoTasksAllLists returns open tasks across every To Do list.
func GraphTodoTasksAllLists(ctx context.Context, token string) ([]TodoTask, error) {
	lists, err := GraphListTodoLists(ctx, token)
	if err != nil {
		return nil, err
	}
	var out []TodoTask
	for _, l := range lists {
		ts, err := GraphTodoTasks(ctx, token, l.ID)
		if err != nil {
			continue
		}
		out = append(out, ts...)
	}
	return out, nil
}

// todoTasksFor resolves a list id (specific, "" = default list, or TodoAllLists)
// to its open tasks.
func todoTasksFor(ctx context.Context, token, listID string) ([]TodoTask, error) {
	switch listID {
	case TodoAllLists:
		return GraphTodoTasksAllLists(ctx, token)
	case "":
		lists, err := GraphListTodoLists(ctx, token)
		if err != nil || len(lists) == 0 {
			return nil, err
		}
		return GraphTodoTasks(ctx, token, lists[0].ID)
	default:
		return GraphTodoTasks(ctx, token, listID)
	}
}

// GraphListTodoLists lists the user's Microsoft To Do lists (for the picker).
func GraphListTodoLists(ctx context.Context, token string) ([]ResourceOption, error) {
	var body struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+"/me/todo/lists?$top=100", &body); err != nil {
		return nil, err
	}
	out := make([]ResourceOption, 0, len(body.Value))
	for _, l := range body.Value {
		out = append(out, ResourceOption{ID: l.ID, Name: l.DisplayName})
	}
	return out, nil
}

// GraphTodoTasks returns the open (not completed) tasks in a To Do list, with
// their due dates so callers can filter by "today / overdue". We fetch plainly
// (no $select/$filter — that combination can 400 on the todoTask endpoint) and
// drop completed tasks client-side.
func GraphTodoTasks(ctx context.Context, token, listID string) ([]TodoTask, error) {
	u := graphBase + "/me/todo/lists/" + listID + "/tasks?$top=100"
	var body struct {
		Value []struct {
			Title       string `json:"title"`
			Status      string `json:"status"`
			DueDateTime *struct {
				DateTime string `json:"dateTime"`
				TimeZone string `json:"timeZone"`
			} `json:"dueDateTime"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, u, &body); err != nil {
		return nil, err
	}
	out := make([]TodoTask, 0, len(body.Value))
	for _, t := range body.Value {
		if t.Status == "completed" {
			continue
		}
		task := TodoTask{Title: t.Title}
		if t.DueDateTime != nil && t.DueDateTime.DateTime != "" {
			// Graph returns e.g. "2026-07-21T00:00:00.0000000"; parse leniently.
			for _, layout := range []string{"2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", time.RFC3339} {
				if due, err := time.Parse(layout, t.DueDateTime.DateTime); err == nil {
					task.Due = due
					break
				}
			}
		}
		out = append(out, task)
	}
	return out, nil
}
