package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.ripls.org/ripls/server/logging"
)

const (
	// forecastHorizonDays is how far out Open-Meteo serves a real daily
	// forecast. Beyond it we fall back to climate normals. Open-Meteo's free
	// forecast reaches ~16 days.
	forecastHorizonDays = 16

	defaultForecastBaseURL = "https://api.open-meteo.com/v1/forecast"
	defaultArchiveBaseURL  = "https://archive-api.open-meteo.com/v1/archive"

	// weatherHTTPTimeout bounds each upstream call. Kept short: weather is
	// best-effort everywhere, callers serve stale or no weather past their own
	// budgets, and a hung provider must not hold request goroutines (#2646).
	weatherHTTPTimeout = 4 * time.Second
)

// OpenMeteoProvider fetches daily weather from Open-Meteo. It needs no API key.
// Real forecasts come from the forecast endpoint; days beyond the forecast
// horizon are filled from the archive endpoint (the same calendar dates a year
// earlier) as a climate-normal proxy and marked IsTypical.
//
// The endpoint hosts are constant (never user-controlled), so the outbound
// requests carry no SSRF surface.
type OpenMeteoProvider struct {
	httpClient      *http.Client
	forecastBaseURL string
	archiveBaseURL  string
	// now is injectable so tests can pin the horizon boundary.
	now func() time.Time
}

// NewOpenMeteoProvider builds an Open-Meteo provider with production defaults.
func NewOpenMeteoProvider() *OpenMeteoProvider {
	return &OpenMeteoProvider{
		httpClient:      &http.Client{Timeout: weatherHTTPTimeout},
		forecastBaseURL: defaultForecastBaseURL,
		archiveBaseURL:  defaultArchiveBaseURL,
		now:             time.Now,
	}
}

// DailyWeather implements Provider.
func (p *OpenMeteoProvider) DailyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	if tz == nil {
		tz = time.UTC
	}
	from = truncToDay(from, tz)
	to = truncToDay(to, tz)
	if !from.Before(to) {
		return nil, nil
	}
	today := truncToDay(p.now().In(tz), tz)
	horizon := today.AddDate(0, 0, forecastHorizonDays)

	// The two portions hit independent endpoints, so they fetch concurrently —
	// a cache-miss window spanning the horizon costs one round trip, not two
	// (#2646). GoSafe keeps a panic in either fetch from killing the process;
	// the WaitGroup still joins because fn's defer runs before the recover.
	var (
		wg           sync.WaitGroup
		fDays, aDays []DayWeather
		fErr, aErr   error
	)

	// Real-forecast portion: [max(from, today), min(to, horizon)).
	fStart := maxTime(from, today)
	fEnd := minTime(to, horizon)
	if fStart.Before(fEnd) {
		wg.Add(1)
		logging.GoSafe(ctx, "weather-forecast-range", func() {
			defer wg.Done()
			fDays, fErr = p.fetchRange(ctx, p.forecastBaseURL, place, fStart, fEnd, tz)
		})
	}

	// Climate-normal portion: [max(from, horizon), to). Approximated by the same
	// calendar dates one year earlier from the archive, shifted forward and
	// marked IsTypical. A true multi-year normal is the production upgrade
	// (docs/weather.md).
	tStart := maxTime(from, horizon)
	if tStart.Before(to) {
		wg.Add(1)
		logging.GoSafe(ctx, "weather-archive-range", func() {
			defer wg.Done()
			aDays, aErr = p.fetchRange(ctx, p.archiveBaseURL, place, tStart.AddDate(-1, 0, 0), to.AddDate(-1, 0, 0), tz)
		})
	}
	wg.Wait()

	// Same partial-result semantics as the sequential version: a forecast
	// error yields (nil, err); an archive error yields the forecast days + err.
	if fErr != nil {
		return nil, fErr
	}
	out := append([]DayWeather{}, fDays...)
	if aErr != nil {
		return out, aErr
	}
	for _, d := range aDays {
		d.Date = d.Date.AddDate(1, 0, 0)
		d.IsTypical = true
		out = append(out, d)
	}
	return out, nil
}

// HourlyWeather implements Provider. Only the real-forecast window is served;
// hours past the forecast horizon (or before today) are omitted — there is no
// hourly climate-normal proxy.
func (p *OpenMeteoProvider) HourlyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	if tz == nil {
		tz = time.UTC
	}
	from = from.In(tz)
	to = to.In(tz)
	if !from.Before(to) {
		return nil, nil
	}
	today := truncToDay(p.now().In(tz), tz)
	horizon := today.AddDate(0, 0, forecastHorizonDays)
	// Clamp the request to the real-forecast window [today, horizon).
	start := maxTime(from, today)
	end := minTime(to, horizon)
	if !start.Before(end) {
		return nil, nil
	}
	return p.fetchHourly(ctx, place, start, end, tz)
}

// openMeteoResponse is the subset of the Open-Meteo daily payload we read. Both
// the forecast and archive endpoints share this shape.
type openMeteoResponse struct {
	Daily struct {
		Time        []string  `json:"time"`
		WeatherCode []int     `json:"weather_code"`
		Temperature []float64 `json:"temperature_2m_max"`
	} `json:"daily"`
}

// openMeteoHourlyResponse is the subset of the Open-Meteo hourly payload we read.
type openMeteoHourlyResponse struct {
	Hourly struct {
		Time        []string  `json:"time"`
		WeatherCode []int     `json:"weather_code"`
		Temperature []float64 `json:"temperature_2m"`
	} `json:"hourly"`
}

// fetchHourly fetches hourly weather for [from, to) (the end hour is exclusive).
func (p *OpenMeteoProvider) fetchHourly(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", place.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", place.Lng))
	q.Set("hourly", "weather_code,temperature_2m")
	q.Set("timezone", tz.String())
	// The hourly endpoint range is by date (inclusive); request the days the
	// window spans and filter to [from, to) after parsing.
	q.Set("start_date", truncToDay(from, tz).Format("2006-01-02"))
	q.Set("end_date", truncToDay(to.Add(-time.Second), tz).Format("2006-01-02"))
	reqURL := p.forecastBaseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build hourly weather request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hourly weather request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather endpoint returned %d", resp.StatusCode)
	}

	var payload openMeteoHourlyResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode hourly weather response: %w", err)
	}
	return parseHourly(payload, from, to, tz), nil
}

// parseHourly zips the parallel hourly arrays into HourWeather rows within
// [from, to), skipping any hour whose arrays disagree in length or whose
// timestamp is outside the window.
func parseHourly(payload openMeteoHourlyResponse, from, to time.Time, tz *time.Location) []HourWeather {
	n := len(payload.Hourly.Time)
	if len(payload.Hourly.WeatherCode) < n || len(payload.Hourly.Temperature) < n {
		return nil
	}
	out := make([]HourWeather, 0, n)
	for i := 0; i < n; i++ {
		// Open-Meteo hourly timestamps are local-to-tz wall times without an
		// offset, e.g. "2026-06-24T14:00".
		t, err := time.ParseInLocation("2006-01-02T15:04", payload.Hourly.Time[i], tz)
		if err != nil {
			continue
		}
		if t.Before(from) || !t.Before(to) {
			continue
		}
		out = append(out, HourWeather{
			Time:      t,
			Condition: conditionFromWMO(payload.Hourly.WeatherCode[i]),
			TempC:     payload.Hourly.Temperature[i],
		})
	}
	return out
}

func (p *OpenMeteoProvider) fetchRange(ctx context.Context, base string, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	// The range is inclusive on the wire; subtract a day from the exclusive end.
	endInclusive := to.AddDate(0, 0, -1)
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", place.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", place.Lng))
	q.Set("daily", "weather_code,temperature_2m_max")
	q.Set("timezone", tz.String())
	q.Set("start_date", from.Format("2006-01-02"))
	q.Set("end_date", endInclusive.Format("2006-01-02"))
	reqURL := base + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build weather request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather endpoint returned %d", resp.StatusCode)
	}

	var payload openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode weather response: %w", err)
	}
	return parseDaily(payload, tz), nil
}

// parseDaily zips the parallel daily arrays into DayWeather rows, skipping any
// day whose arrays disagree in length.
func parseDaily(payload openMeteoResponse, tz *time.Location) []DayWeather {
	n := len(payload.Daily.Time)
	if len(payload.Daily.WeatherCode) < n || len(payload.Daily.Temperature) < n {
		return nil
	}
	out := make([]DayWeather, 0, n)
	for i := 0; i < n; i++ {
		day, err := time.ParseInLocation("2006-01-02", payload.Daily.Time[i], tz)
		if err != nil {
			continue
		}
		out = append(out, DayWeather{
			Date:      day,
			Condition: conditionFromWMO(payload.Daily.WeatherCode[i]),
			HighTempC: payload.Daily.Temperature[i],
		})
	}
	return out
}

func truncToDay(t time.Time, tz *time.Location) time.Time {
	t = t.In(tz)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
