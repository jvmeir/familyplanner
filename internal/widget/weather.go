package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// openMeteoBase / openMeteoGeoBase are overridable in tests. Open-Meteo is free
// and needs no API key (forecast + geocoding).
var openMeteoBase = "https://api.open-meteo.com/v1/forecast"
var openMeteoGeoBase = "https://geocoding-api.open-meteo.com/v1/search"

// WeatherConfig is the per-instance configuration. Either give coordinates, or a
// place/address (geocoded to coordinates on fetch). Days is how many forecast
// days to show (including today).
type WeatherConfig struct {
	Location string `json:"location"` // place/address; geocoded when set (wins over lat/lon)
	Lat      string `json:"lat"`
	Lon      string `json:"lon"`
	Days     string `json:"days"` // forecast day count incl. today (default 5, 1..7)
}

// WeatherDay is one day in the forecast strip.
type WeatherDay struct {
	Label string  `json:"label"` // "Vandaag" for today, else short weekday
	Code  int     `json:"code"`  // WMO weather code
	Max   float64 `json:"max"`
	Min   float64 `json:"min"`
}

// WeatherData is the normalized render data: current conditions + a daily
// forecast (the first day is today, carrying today's hi/lo).
type WeatherData struct {
	TempC float64      `json:"temp_c"`
	Code  int          `json:"code"`
	Place string       `json:"place,omitempty"`
	Days  []WeatherDay `json:"days,omitempty"`
}

type weatherProvider struct{ cfg WeatherConfig }

func newWeather(raw json.RawMessage, _ []SourceInput, _ NowFunc) (Provider, error) {
	var cfg WeatherConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return weatherProvider{cfg: cfg}, nil
}

func decodeWeather(raw json.RawMessage) (Data, error) {
	var d WeatherData
	err := json.Unmarshal(raw, &d)
	return d, err
}

// geocode resolves a place/address to coordinates via Open-Meteo's free
// geocoding API, returning the coordinates and the canonical place name.
func geocode(ctx context.Context, name string) (lat, lon, place string, err error) {
	u := openMeteoGeoBase + "?count=1&language=nl&format=json&name=" + url.QueryEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("geocode: status %d", resp.StatusCode)
	}
	var body struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Name      string  `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", "", err
	}
	if len(body.Results) == 0 {
		return "", "", "", fmt.Errorf("geen locatie gevonden voor %q", name)
	}
	r := body.Results[0]
	return strconv.FormatFloat(r.Latitude, 'f', 4, 64), strconv.FormatFloat(r.Longitude, 'f', 4, 64), r.Name, nil
}

func (p weatherProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	lat, lon, place := p.cfg.Lat, p.cfg.Lon, ""
	if p.cfg.Location != "" {
		gLat, gLon, gPlace, err := geocode(ctx, p.cfg.Location)
		if err != nil {
			return nil, 0, err
		}
		lat, lon, place = gLat, gLon, gPlace
	}
	if lat == "" {
		lat = "50.85" // Brussels
	}
	if lon == "" {
		lon = "4.35"
	}
	days := 5
	if n, err := strconv.Atoi(p.cfg.Days); err == nil && n > 0 {
		days = n
	}
	if days > 7 {
		days = 7
	}

	u := fmt.Sprintf("%s?latitude=%s&longitude=%s&current=temperature_2m,weather_code"+
		"&daily=weather_code,temperature_2m_max,temperature_2m_min&timezone=auto&forecast_days=%d",
		openMeteoBase, lat, lon, days)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("weather: status %d", resp.StatusCode)
	}
	var body struct {
		Current struct {
			Temp float64 `json:"temperature_2m"`
			Code int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			Time []string  `json:"time"`
			Code []int     `json:"weather_code"`
			Max  []float64 `json:"temperature_2m_max"`
			Min  []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, err
	}

	out := WeatherData{TempC: body.Current.Temp, Code: body.Current.Code, Place: place}
	for i := range body.Daily.Time {
		// forecast_days starts at the current day, so index 0 is always today.
		label := "Vandaag"
		if i > 0 {
			if t, perr := time.Parse("2006-01-02", body.Daily.Time[i]); perr == nil {
				label = nlWeekday[t.Weekday()]
			}
		}
		out.Days = append(out.Days, WeatherDay{
			Label: label,
			Code:  atInt(body.Daily.Code, i),
			Max:   atFloat(body.Daily.Max, i),
			Min:   atFloat(body.Daily.Min, i),
		})
	}
	return out, 15 * time.Minute, nil
}

func atInt(s []int, i int) int {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return 0
}

func atFloat(s []float64, i int) float64 {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return 0
}
