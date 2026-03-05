// Package weather provides Open-Meteo weather data for KRL stations.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Forecast holds current weather conditions for a station.
type Forecast struct {
	Temp      float64 `json:"temp"`
	FeelsLike float64 `json:"feels_like"`
	Condition string  `json:"condition"`
	Precip    float64 `json:"precip_pct"`
}

// openMeteoResponse is the raw response shape from Open-Meteo.
type openMeteoResponse struct {
	CurrentWeather struct {
		Temperature   float64 `json:"temperature"`
		Weathercode   int     `json:"weathercode"`
		Windspeed     float64 `json:"windspeed"`
	} `json:"current_weather"`
	Hourly struct {
		Temperature2m            []float64 `json:"temperature_2m"`
		ApparentTemperature      []float64 `json:"apparent_temperature"`
		PrecipitationProbability []int     `json:"precipitation_probability"`
		Time                     []string  `json:"time"`
	} `json:"hourly"`
}

// Client fetches weather data from Open-Meteo.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a weather Client with the given base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Get fetches current weather for the given lat/lon.
func (c *Client) Get(ctx context.Context, lat, lon float64) (*Forecast, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("weather: invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("current_weather", "true")
	q.Set("hourly", "temperature_2m,apparent_temperature,precipitation_probability")
	q.Set("timezone", "Asia/Jakarta")
	q.Set("forecast_days", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("weather: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather: unexpected status %d", resp.StatusCode)
	}

	var raw openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("weather: decode response: %w", err)
	}

	f := &Forecast{
		Temp:      raw.CurrentWeather.Temperature,
		Condition: weatherCodeDesc(raw.CurrentWeather.Weathercode),
	}

	// Find the closest hourly slot to now for feels-like and precip.
	// Open-Meteo always returns Asia/Jakarta timezone when requested.
	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Now().In(loc)
	nowHour := now.Format("2006-01-02T15:00")

	for i, t := range raw.Hourly.Time {
		if t == nowHour {
			if i < len(raw.Hourly.ApparentTemperature) {
				f.FeelsLike = raw.Hourly.ApparentTemperature[i]
			}
			if i < len(raw.Hourly.PrecipitationProbability) {
				f.Precip = float64(raw.Hourly.PrecipitationProbability[i])
			}
			break
		}
	}

	return f, nil
}

// weatherCodeDesc maps WMO weather codes to human-readable descriptions.
func weatherCodeDesc(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1, 2, 3:
		return "Partly cloudy"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75, 77:
		return "Snow"
	case 80, 81, 82:
		return "Showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	default:
		return "Overcast"
	}
}

// Emoji returns a weather emoji for the forecast.
func (f *Forecast) Emoji() string {
	switch {
	case f.Precip > 70:
		return "🌧️"
	case f.Precip > 40:
		return "🌦️"
	case f.Precip > 20:
		return "⛅"
	default:
		return "🌤️"
	}
}
