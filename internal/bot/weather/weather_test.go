package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetWeather_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify required query params.
		q := r.URL.Query()
		if q.Get("latitude") == "" || q.Get("longitude") == "" {
			t.Error("missing lat/lon params")
		}
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck
		w.Write([]byte(`{
			"current_weather": {"temperature": 28.5, "weathercode": 2, "windspeed": 10},
			"hourly": {
				"time": ["2026-03-05T07:00"],
				"temperature_2m": [28.5],
				"apparent_temperature": [31.0],
				"precipitation_probability": [20]
			}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	f, err := c.Get(context.Background(), -6.4, 106.82)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if f.Temp != 28.5 {
		t.Errorf("Temp = %.1f, want 28.5", f.Temp)
	}
	if f.Condition == "" {
		t.Error("Condition should not be empty")
	}
}

func TestGetWeather_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Get(context.Background(), -6.4, 106.82)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestWeatherCodeDesc(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{0, "Clear sky"},
		{1, "Partly cloudy"},
		{3, "Partly cloudy"},
		{45, "Fog"},
		{61, "Rain"},
		{95, "Thunderstorm"},
		{80, "Showers"},
	}
	for _, tt := range tests {
		got := weatherCodeDesc(tt.code)
		if got != tt.want {
			t.Errorf("weatherCodeDesc(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestForecastEmoji(t *testing.T) {
	tests := []struct {
		precip float64
		want   string
	}{
		{80, "🌧️"},
		{50, "🌦️"},
		{30, "⛅"},
		{10, "🌤️"},
	}
	for _, tt := range tests {
		f := &Forecast{Precip: tt.precip}
		got := f.Emoji()
		if got != tt.want {
			t.Errorf("Emoji(precip=%.0f) = %q, want %q", tt.precip, got, tt.want)
		}
	}
}
