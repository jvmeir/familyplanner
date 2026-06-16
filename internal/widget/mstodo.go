package widget

import (
	"context"
	"net/url"
)

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

// GraphTodoTasks returns the open (not completed) task titles in a To Do list.
func GraphTodoTasks(ctx context.Context, token, listID string) ([]string, error) {
	u := graphBase + "/me/todo/lists/" + listID + "/tasks?$top=100&$filter=" + url.QueryEscape("status ne 'completed'")
	var body struct {
		Value []struct {
			Title string `json:"title"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, u, &body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Value))
	for _, t := range body.Value {
		out = append(out, t.Title)
	}
	return out, nil
}
