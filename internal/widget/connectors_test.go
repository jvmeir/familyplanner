package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWeatherFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":12.3,"weather_code":3}}`))
	}))
	defer srv.Close()

	old := openMeteoBase
	openMeteoBase = srv.URL
	defer func() { openMeteoBase = old }()

	p, err := newWeather(json.RawMessage(`{"lat":"50.85","lon":"4.35"}`), nil, nil)
	require.NoError(t, err)
	data, ttl, err := p.Fetch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 15*time.Minute, ttl)
	wd := data.(WeatherData)
	require.InDelta(t, 12.3, wd.TempC, 0.001)
	require.Equal(t, 3, wd.Code)
}

func TestCalendarFetch(t *testing.T) {
	const ical = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nDTSTART:20260601T090000Z\r\nSUMMARY:Tandarts\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ical))
	}))
	defer srv.Close()

	now := func() time.Time { return time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC) }
	cfg, _ := json.Marshal(CalendarConfig{URL: srv.URL})
	p, err := newCalendar(cfg, nil, now)
	require.NoError(t, err)

	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)
	require.Len(t, cd.Events, 1)
	require.Contains(t, cd.Events[0].Title, "Tandarts")
}

func TestCalendarFilter(t *testing.T) {
	const ical = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTART:20260601T090000Z\r\nSUMMARY:Tandarts\r\nCATEGORIES:gezondheid\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:2\r\nDTSTART:20260602T180000Z\r\nSUMMARY:Voetbal\r\nCATEGORIES:sport\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ical))
	}))
	defer srv.Close()

	now := func() time.Time { return time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC) }
	cfg, _ := json.Marshal(CalendarConfig{URL: srv.URL, WeeksAhead: "2", Filter: "sport"})
	p, err := newCalendar(cfg, nil, now)
	require.NoError(t, err)

	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)
	require.Len(t, cd.Events, 1, "only the sport-category event should pass the filter")
	require.Contains(t, cd.Events[0].Title, "Voetbal")
}

func TestCalendarMonthMode(t *testing.T) {
	const ical = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTART:20260520T100000Z\r\nSUMMARY:Verjaardag\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ical))
	}))
	defer srv.Close()

	now := func() time.Time { return time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC) }
	cfg, _ := json.Marshal(CalendarConfig{URL: srv.URL, Mode: "month"})
	p, err := newCalendar(cfg, nil, now)
	require.NoError(t, err)

	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)
	require.Equal(t, "month", cd.Mode)
	require.NotNil(t, cd.Month)

	found := false
	for _, week := range cd.Month.Weeks {
		for _, day := range week {
			if day.InMonth && day.Day == 20 {
				// Timed events show the start time in front ("10:00 Verjaardag").
				require.Len(t, day.Events, 1)
				require.Contains(t, day.Events[0].Text, "Verjaardag")
				found = true
			}
		}
	}
	require.True(t, found, "the 20th should carry the event")
}

func TestCalendarDaysMode(t *testing.T) {
	const ical = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:1\r\nDTSTART:20260531T090000Z\r\nSUMMARY:Tandarts\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ical))
	}))
	defer srv.Close()

	now := func() time.Time { return time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC) }
	cfg, _ := json.Marshal(CalendarConfig{URL: srv.URL, Mode: "days", WeeksAhead: "1"})
	p, _ := newCalendar(cfg, nil, now)
	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)
	require.Equal(t, "days", cd.Mode)
	require.NotEmpty(t, cd.Days)
	require.Equal(t, 8, len(cd.Days), "0 weeks back + 1 ahead = 8 days inclusive")
	// every day present (incl. empty), and the 31st carries the event
	var withEvent int
	for _, d := range cd.Days {
		if len(d.Events) > 0 {
			withEvent++
		}
	}
	require.Equal(t, 1, withEvent)
}

func TestComplexFilter(t *testing.T) {
	f := parseFilter("location:school, sport")
	// clause 1: location contains "school"; clause 2: any contains "sport"
	require.True(t, f.match(func(p string) string {
		if p == "location" {
			return "Basisschool X"
		}
		return ""
	}))
	require.True(t, f.match(func(p string) string {
		if p == "" {
			return "Voetbal sporttraining"
		}
		return ""
	}))
	require.False(t, f.match(func(p string) string { return "iets anders" }))

	// AND within a clause (space-separated)
	f2 := parseFilter("categories:sport voetbal")
	require.True(t, f2.match(func(p string) string {
		if p == "categories" {
			return "sport"
		}
		return "voetbal training"
	}))
	require.False(t, f2.match(func(p string) string {
		if p == "categories" {
			return "sport"
		}
		return "hockey"
	}))
}

func TestCalendarMultiSource(t *testing.T) {
	icsA := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:a1\r\nDTSTART:20260601T090000Z\r\nSUMMARY:Tandarts\r\nCATEGORIES:gezondheid\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:a2\r\nDTSTART:20260602T180000Z\r\nSUMMARY:Voetbal\r\nCATEGORIES:sport\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	icsB := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\nUID:b1\r\nDTSTART:20260603T080000Z\r\nSUMMARY:Werk\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(icsA)) }))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(icsB)) }))
	defer srvB.Close()

	cfgA, _ := json.Marshal(map[string]string{"url": srvA.URL})
	cfgB, _ := json.Marshal(map[string]string{"url": srvB.URL})
	sources := []SourceInput{
		{Type: "ical", Config: cfgA, Filter: "sport"}, // only the sport event from A
		{Type: "ical", Config: cfgB, Filter: ""},      // everything from B
	}
	now := func() time.Time { return time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC) }
	cfg, _ := json.Marshal(CalendarConfig{Mode: "agenda", WeeksAhead: "4"})
	p, err := newCalendar(cfg, sources, now)
	require.NoError(t, err)

	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	cd := data.(CalendarData)

	var titles string
	for _, e := range cd.Events {
		titles += e.Title + "|"
	}
	require.Contains(t, titles, "Voetbal", "sport event from source A")
	require.Contains(t, titles, "Werk", "event from source B")
	require.NotContains(t, titles, "Tandarts", "filtered out of source A by per-source filter")
}

func TestGraphCalendar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/calendarView", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"subject":"Werk","isAllDay":false,"start":{"dateTime":"2026-06-01T09:00:00.0000000","timeZone":"UTC"}}]}`))
	})
	mux.HandleFunc("/me/calendars", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"id":"c1","name":"Familie"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := graphBase
	graphBase = srv.URL
	defer func() { graphBase = old }()

	from := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	evs, err := GraphCalendar(context.Background(), "tok", "", from, to, time.UTC)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "Werk", evs[0].title)

	cals, err := GraphListCalendars(context.Background(), "tok")
	require.NoError(t, err)
	require.Len(t, cals, 1)
	require.Equal(t, "Familie", cals[0].Name)
}

func TestOneDrivePhotos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/drive/bundles", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"id":"a1","name":"Vakantie","bundle":{"album":{}}},{"id":"b1","name":"Deelmap","bundle":{}}]}`))
	})
	mux.HandleFunc("/me/drive/items/a1/children", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"file":{"mimeType":"image/jpeg"},"@microsoft.graph.downloadUrl":"https://dl/p1","createdDateTime":"2026-01-02T00:00:00Z"},{"file":{"mimeType":"text/plain"},"@microsoft.graph.downloadUrl":"https://dl/doc"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := graphBase
	graphBase = srv.URL
	defer func() { graphBase = old }()

	albums, err := GraphListAlbums(context.Background(), "tok")
	require.NoError(t, err)
	require.Len(t, albums, 1) // only the album-faceted bundle
	require.Equal(t, "Vakantie", albums[0].Name)

	photos, err := GraphFolderPhotos(context.Background(), "tok", "a1")
	require.NoError(t, err)
	require.Len(t, photos, 1) // only the image, not the .txt
	require.Equal(t, "https://dl/p1", photos[0].URL)
}

func TestMSTodo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/todo/lists", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"id":"l1","displayName":"Boodschappen"}]}`))
	})
	mux.HandleFunc("/me/todo/lists/l1/tasks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"title":"Melk kopen"},{"title":"Bank bellen"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := graphBase
	graphBase = srv.URL
	defer func() { graphBase = old }()

	lists, err := GraphListTodoLists(context.Background(), "tok")
	require.NoError(t, err)
	require.Len(t, lists, 1)
	require.Equal(t, "Boodschappen", lists[0].Name)

	tasks, err := GraphTodoTasks(context.Background(), "tok", "l1")
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	var titles []string
	for _, tk := range tasks {
		titles = append(titles, tk.Title)
	}
	require.Contains(t, titles, "Melk kopen")
}

func TestNormalizeWebcal(t *testing.T) {
	require.Equal(t, "https://x.com/c.ics", normalizeICSURL("webcal://x.com/c.ics"))
	require.Equal(t, "https://x.com/c.ics", normalizeICSURL("webcals://x.com/c.ics"))
	require.Equal(t, "https://x.com/c.ics", normalizeICSURL("https://x.com/c.ics"))
}

func TestBringShopping(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/bringauth", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok","uuid":"u1","bringListUUID":"list1"}`))
	})
	mux.HandleFunc("/bringlists/list1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"purchase":[{"name":"Melk","specification":"2L"},{"name":"Brood","specification":""}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := bringBase
	bringBase = srv.URL
	defer func() { bringBase = old }()

	secret, _ := json.Marshal(map[string]string{"email": "a@b.c", "password": "x"})
	cfg, _ := json.Marshal(map[string]string{})
	sources := []SourceInput{{Type: "bring", Config: cfg, Secret: secret}}

	p, err := newShopping(nil, sources, nil)
	require.NoError(t, err)
	data, _, err := p.Fetch(context.Background())
	require.NoError(t, err)
	sd := data.(ShoppingData)
	require.Len(t, sd.Items, 2)
	require.Contains(t, sd.Items[0], "Melk")
	require.Contains(t, sd.Items[0], "2L")
}
