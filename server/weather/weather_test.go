package weather

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestConditionFromWMO(t *testing.T) {
	cases := map[int]Condition{
		0:  ConditionClear,
		1:  ConditionMostlySunny,
		2:  ConditionPartlyCloudy,
		3:  ConditionOvercast,
		45: ConditionOvercast,
		51: ConditionRain,
		65: ConditionRain,
		71: ConditionRain, // snow collapses to the wet bucket
		95: ConditionRain,
		7:  ConditionUnknown,
	}
	for code, want := range cases {
		if got := conditionFromWMO(code); got != want {
			t.Errorf("conditionFromWMO(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestLatLngSnap(t *testing.T) {
	got := LatLng{Lat: 39.7392, Lng: -104.9903}.Snap()
	if got.Lat != 39.7 || got.Lng != -105.0 {
		t.Errorf("Snap() = %+v, want {39.7 -105}", got)
	}
}

func TestFakeProviderDeterministicAndTypical(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 5)
	cutoff := time.Date(2026, 6, 3, 0, 0, 0, 0, tz)
	f := &FakeProvider{TypicalAfter: &cutoff}

	days, err := f.DailyWeather(context.Background(), LatLng{}, from, to, tz)
	if err != nil {
		t.Fatalf("DailyWeather: %v", err)
	}
	if len(days) != 5 {
		t.Fatalf("got %d days, want 5", len(days))
	}
	if days[0].IsTypical || !days[2].IsTypical {
		t.Errorf("typical cutoff not applied: day0=%v day2=%v", days[0].IsTypical, days[2].IsTypical)
	}
	// Deterministic across calls.
	again, _ := f.DailyWeather(context.Background(), LatLng{}, from, to, tz)
	if again[1].Condition != days[1].Condition || again[1].HighTempC != days[1].HighTempC {
		t.Error("FakeProvider not deterministic")
	}
}

func TestFakeProviderError(t *testing.T) {
	f := &FakeProvider{Err: errors.New("boom")}
	if _, err := f.DailyWeather(context.Background(), LatLng{}, time.Now(), time.Now().AddDate(0, 0, 1), time.UTC); err == nil {
		t.Error("expected error")
	}
}

// countingProvider records how many times the inner provider is hit.
type countingProvider struct {
	mu    sync.Mutex
	calls int
	inner Provider
}

func (c *countingProvider) DailyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.DailyWeather(ctx, place, from, to, tz)
}

func (c *countingProvider) HourlyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.HourlyWeather(ctx, place, from, to, tz)
}

// callCount reads the counter race-safely — background refreshes hit the inner
// provider from goroutines.
func (c *countingProvider) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestFakeHourlyWeather(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 24, 12, 0, 0, 0, tz)
	to := from.Add(6 * time.Hour)
	f := NewFakeProvider()
	hours, err := f.HourlyWeather(context.Background(), LatLng{}, from, to, tz)
	if err != nil {
		t.Fatalf("HourlyWeather: %v", err)
	}
	if len(hours) != 6 {
		t.Fatalf("expected 6 hours, got %d", len(hours))
	}
	if !hours[0].Time.Equal(from) {
		t.Errorf("first hour = %v, want %v", hours[0].Time, from)
	}
	if hours[5].Time.Hour() != 17 {
		t.Errorf("last hour = %d, want 17", hours[5].Time.Hour())
	}
}

func TestFakeHourlyHorizonOmitsLateHours(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 24, 12, 0, 0, 0, tz)
	to := from.Add(6 * time.Hour)
	horizon := time.Date(2026, 6, 24, 15, 0, 0, 0, tz)
	f := &FakeProvider{HourlyHorizon: &horizon}
	hours, err := f.HourlyWeather(context.Background(), LatLng{}, from, to, tz)
	if err != nil {
		t.Fatalf("HourlyWeather: %v", err)
	}
	// 12,13,14 are before the horizon; 15,16,17 are omitted.
	if len(hours) != 3 {
		t.Fatalf("expected 3 hours before horizon, got %d", len(hours))
	}
}

func TestCachedProviderCollapsesCalls(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	counter := &countingProvider{inner: NewFakeProvider()}
	cached := NewCachedProvider(counter)

	// Two viewers near the same place (within one grid cell) → one upstream call.
	denver := LatLng{Lat: 39.74, Lng: -104.99}
	denverNearby := LatLng{Lat: 39.73, Lng: -105.01}
	if _, err := cached.DailyWeather(context.Background(), denver, from, to, tz); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := cached.DailyWeather(context.Background(), denverNearby, from, to, tz); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if counter.calls != 1 {
		t.Errorf("inner called %d times, want 1 (per-place cache)", counter.calls)
	}
}

func TestCachedProviderRefreshesAfterTTL(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	counter := &countingProvider{inner: NewFakeProvider()}
	cached := NewCachedProvider(counter)

	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }
	place := LatLng{Lat: 39.7, Lng: -105.0}
	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}
	now = now.Add(defaultCacheTTL + time.Minute) // past TTL
	// The expired entry is served stale while a background refresh runs.
	days, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) == 0 {
		t.Fatal("expected stale days to be served past TTL")
	}
	cached.refreshes.Wait()
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2 (background refresh after TTL)", got)
	}
}

func TestParseDailyMismatchedArrays(t *testing.T) {
	var payload openMeteoResponse
	payload.Daily.Time = []string{"2026-06-01", "2026-06-02"}
	payload.Daily.WeatherCode = []int{0} // short
	payload.Daily.Temperature = []float64{20, 21}
	if got := parseDaily(payload, time.UTC); got != nil {
		t.Errorf("expected nil on mismatched arrays, got %v", got)
	}
}

func TestParseDailyHappy(t *testing.T) {
	var payload openMeteoResponse
	payload.Daily.Time = []string{"2026-06-01", "2026-06-02"}
	payload.Daily.WeatherCode = []int{0, 61}
	payload.Daily.Temperature = []float64{22.4, 15.1}
	got := parseDaily(payload, time.UTC)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Condition != ConditionClear || got[1].Condition != ConditionRain {
		t.Errorf("conditions = %v, %v", got[0].Condition, got[1].Condition)
	}
	if got[0].HighTempC != 22.4 {
		t.Errorf("temp = %v, want 22.4", got[0].HighTempC)
	}
}
