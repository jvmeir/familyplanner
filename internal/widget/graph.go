package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// graphBase is overridable in tests.
var graphBase = "https://graph.microsoft.com/v1.0"

// ResourceOption is a selectable remote resource (a calendar, list, …) used by
// the data-source configure picker.
type ResourceOption struct {
	ID   string
	Name string
}

func graphGet(ctx context.Context, token, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graph: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseGraphTime(s string, loc *time.Location) time.Time {
	if len(s) < 19 {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s[:19], time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t.In(loc)
}

// GraphCalendar fetches events from a calendar (empty id = the default calendar).
// Graph expands recurrences server-side, so no RRULE handling is needed here.
func GraphCalendar(ctx context.Context, token, calendarID string, from, to time.Time, loc *time.Location) ([]calEvent, error) {
	path := "/me/calendarView"
	if calendarID != "" {
		path = "/me/calendars/" + calendarID + "/calendarView"
	}
	url := fmt.Sprintf("%s%s?startDateTime=%s&endDateTime=%s&$select=subject,start,end,isAllDay&$top=100&$orderby=start/dateTime",
		graphBase, path,
		from.UTC().Format("2006-01-02T15:04:05"), to.UTC().Format("2006-01-02T15:04:05"))

	var body struct {
		Value []struct {
			Subject  string `json:"subject"`
			IsAllDay bool   `json:"isAllDay"`
			Start    struct {
				DateTime string `json:"dateTime"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
			} `json:"end"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, url, &body); err != nil {
		return nil, err
	}
	out := make([]calEvent, 0, len(body.Value))
	for _, e := range body.Value {
		t := parseGraphTime(e.Start.DateTime, loc)
		if t.IsZero() {
			continue
		}
		out = append(out, calEvent{t: t, end: parseGraphTime(e.End.DateTime, loc), title: e.Subject, allDay: e.IsAllDay})
	}
	return out, nil
}

// GraphListCalendars lists the user's calendars (for the configure picker).
func GraphListCalendars(ctx context.Context, token string) ([]ResourceOption, error) {
	var body struct {
		Value []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"value"`
	}
	if err := graphGet(ctx, token, graphBase+"/me/calendars?$select=id,name&$top=100", &body); err != nil {
		return nil, err
	}
	out := make([]ResourceOption, 0, len(body.Value))
	for _, c := range body.Value {
		out = append(out, ResourceOption{ID: c.ID, Name: c.Name})
	}
	return out, nil
}
