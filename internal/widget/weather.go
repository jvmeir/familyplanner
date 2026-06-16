package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// openMeteoBase is overridable in tests. Open-Meteo is free and needs no key.
var openMeteoBase = "https://api.open-meteo.com/v1/forecast"

// WeatherConfig is the per-instance configuration.
type WeatherConfig struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// WeatherData is the normalized render data.
type WeatherData struct {
	TempC float64 `json:"temp_c"`
	Code  int     `json:"code"` // WMO weather code
}

type weatherProvider struct{ cfg WeatherConfig }

func newWeather(raw json.RawMessage, _ []SourceInput, _ NowFunc) (Provider, error) {
	var cfg WeatherConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Lat == "" {
		cfg.Lat = "50.85" // Brussels
	}
	if cfg.Lon == "" {
		cfg.Lon = "4.35"
	}
	return weatherProvider{cfg: cfg}, nil
}

func decodeWeather(raw json.RawMessage) (Data, error) {
	var d WeatherData
	err := json.Unmarshal(raw, &d)
	return d, err
}

func (p weatherProvider) Fetch(ctx context.Context) (Data, time.Duration, error) {
	url := fmt.Sprintf("%s?latitude=%s&longitude=%s&current=temperature_2m,weather_code", openMeteoBase, p.cfg.Lat, p.cfg.Lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, err
	}
	return WeatherData{TempC: body.Current.Temp, Code: body.Current.Code}, 15 * time.Minute, nil
}
