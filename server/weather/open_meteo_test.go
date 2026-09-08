package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// writeDailyPayload echoes a valid daily payload for the requested inclusive
// date range, one clear 20°C day per date.
func writeDailyPayload(w http.ResponseWriter, r *http.Request) {
	start, err := time.Parse("2006-01-02", r.URL.Query().Get("start_date"))
	if err != nil {
		http.Error(w, "bad start_date", http.StatusBadRequest)
		return
	}
	end, err := time.Parse("2006-01-02", r.URL.Query().Get("end_date"))
	if err != nil {
		http.Error(w, "bad end_date", http.StatusBadRequest)
		return
	}
	var payload openMeteoResponse
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		payload.Daily.Time = append(payload.Daily.Time, d.Format("2006-01-02"))
		payload.Daily.WeatherCode = append(payload.Daily.WeatherCode, 0)
		payload.Daily.Temperature = append(payload.Daily.Temperature, 20)
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TestOpenMeteoDailyWeatherFetchesConcurrently proves the forecast and archive
// calls overlap: each handler waits for the other request to arrive before
// responding. Sequential fetches would leave the forecast handler waiting for
// an archive request that never comes, trip the guard timeout, and fail the
// test with a 500.
func TestOpenMeteoDailyWeatherFetchesConcurrently(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, tz)
	forecastArrived := make(chan struct{})
	archiveArrived := make(chan struct{})

	barrier := func(signal chan<- struct{}, awaited <-chan struct{}, label string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			close(signal)
			select {
			case <-awaited:
			case <-time.After(5 * time.Second):
				http.Error(w, label+": peer fetch never arrived — fetches ran sequentially", http.StatusInternalServerError)
				return
			}
			writeDailyPayload(w, r)
		}
	}
	forecastSrv := httptest.NewServer(barrier(forecastArrived, archiveArrived, "forecast"))
	defer forecastSrv.Close()
	archiveSrv := httptest.NewServer(barrier(archiveArrived, forecastArrived, "archive"))
	defer archiveSrv.Close()

	p := &OpenMeteoProvider{
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		forecastBaseURL: forecastSrv.URL,
		archiveBaseURL:  archiveSrv.URL,
		now:             func() time.Time { return now },
	}

	from := truncToDay(now, tz)
	to := from.AddDate(0, 0, 30) // spans the 16-day horizon → both endpoints engaged
	days, err := p.DailyWeather(context.Background(), LatLng{Lat: 39.7, Lng: -105.0}, from, to, tz)
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if len(days) != 30 {
		t.Fatalf("got %d days, want 30", len(days))
	}
	// Order and the archive +1y/IsTypical shift must match the sequential
	// implementation: forecast days first, then shifted climate normals.
	if !days[0].Date.Equal(from) {
		t.Errorf("first day = %v, want %v", days[0].Date, from)
	}
	horizon := from.AddDate(0, 0, forecastHorizonDays)
	if !days[forecastHorizonDays].Date.Equal(horizon) {
		t.Errorf("first typical day = %v, want %v", days[forecastHorizonDays].Date, horizon)
	}
	if days[forecastHorizonDays-1].IsTypical {
		t.Error("last forecast day marked IsTypical")
	}
	if !days[forecastHorizonDays].IsTypical {
		t.Error("first beyond-horizon day not marked IsTypical")
	}
	if !days[len(days)-1].Date.Equal(to.AddDate(0, 0, -1)) {
		t.Errorf("last day = %v, want %v", days[len(days)-1].Date, to.AddDate(0, 0, -1))
	}
}

func TestOpenMeteoDailyWeatherForecastErrorReturnsNil(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, tz)
	forecastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer forecastSrv.Close()
	archiveSrv := httptest.NewServer(http.HandlerFunc(writeDailyPayload))
	defer archiveSrv.Close()

	p := &OpenMeteoProvider{
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		forecastBaseURL: forecastSrv.URL,
		archiveBaseURL:  archiveSrv.URL,
		now:             func() time.Time { return now },
	}
	from := truncToDay(now, tz)
	days, err := p.DailyWeather(context.Background(), LatLng{}, from, from.AddDate(0, 0, 30), tz)
	if err == nil {
		t.Fatal("expected forecast error")
	}
	if days != nil {
		t.Errorf("expected nil days on forecast error, got %d", len(days))
	}
}

func TestOpenMeteoDailyWeatherArchiveErrorReturnsForecastDays(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, tz)
	forecastSrv := httptest.NewServer(http.HandlerFunc(writeDailyPayload))
	defer forecastSrv.Close()
	archiveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer archiveSrv.Close()

	p := &OpenMeteoProvider{
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		forecastBaseURL: forecastSrv.URL,
		archiveBaseURL:  archiveSrv.URL,
		now:             func() time.Time { return now },
	}
	from := truncToDay(now, tz)
	days, err := p.DailyWeather(context.Background(), LatLng{}, from, from.AddDate(0, 0, 30), tz)
	if err == nil {
		t.Fatal("expected archive error")
	}
	if len(days) != forecastHorizonDays {
		t.Errorf("expected %d forecast days alongside the archive error, got %d", forecastHorizonDays, len(days))
	}
	if fmt.Sprintf("%v", err) == "" {
		t.Error("empty error text")
	}
}
