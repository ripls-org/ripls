package weather

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// gatedProvider blocks each fetch until a token arrives on release, and signals
// started when a fetch begins executing. It lets tests hold a fetch in flight
// deterministically.
type gatedProvider struct {
	inner   Provider
	release chan struct{}
	started chan struct{}

	mu    sync.Mutex
	calls int
}

func newGatedProvider() *gatedProvider {
	return &gatedProvider{
		inner:   NewFakeProvider(),
		release: make(chan struct{}, 16),
		started: make(chan struct{}, 16),
	}
}

func (g *gatedProvider) DailyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	g.started <- struct{}{}
	<-g.release
	return g.inner.DailyWeather(ctx, place, from, to, tz)
}

func (g *gatedProvider) HourlyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	g.started <- struct{}{}
	<-g.release
	return g.inner.HourlyWeather(ctx, place, from, to, tz)
}

func (g *gatedProvider) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestCachedProviderServesStaleWhileRefreshing(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	gated := newGatedProvider()
	cached := NewCachedProvider(gated)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	// First fetch populates the cache (released immediately).
	gated.release <- struct{}{}
	first, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err != nil {
		t.Fatal(err)
	}
	<-gated.started

	// Past TTL with the refresh gate closed: the stale entry must come back
	// immediately — if this call waited on the provider it would deadlock, and
	// the test would time out.
	now = now.Add(defaultCacheTTL + time.Minute)
	stale, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != len(first) || !stale[0].Date.Equal(first[0].Date) {
		t.Errorf("stale serve returned different data: %d days vs %d", len(stale), len(first))
	}

	// Release the background refresh and join it.
	<-gated.started // the refresh has begun executing
	gated.release <- struct{}{}
	cached.refreshes.Wait()
	if got := gated.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2", got)
	}

	// The refresh reset freshness: the next call is a cache hit.
	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}
	if got := gated.callCount(); got != 2 {
		t.Errorf("inner called %d times after refresh, want still 2", got)
	}
}

func TestCachedProviderMidnightSpanRolloverServesStale(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	counter := &countingProvider{inner: NewFakeProvider()}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 1, 23, 30, 0, 0, tz)
	cached.now = func() time.Time { return now }

	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}

	// One hour later — well inside the TTL — the day rolled over, shifting the
	// requested span. The old span's data must be served (stale fallback keyed
	// by place), not fetched inline.
	now = now.Add(time.Hour)
	days, err := cached.DailyWeather(context.Background(), place, from.AddDate(0, 0, 1), to.AddDate(0, 0, 1), tz)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) == 0 || !days[0].Date.Equal(from) {
		t.Errorf("expected stale pre-rollover data starting %v, got %d days starting %v",
			from, len(days), days[0].Date)
	}

	cached.refreshes.Wait()
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2 (background refresh for the new span)", got)
	}

	// The refreshed span is now fresh.
	fresh, err := cached.DailyWeather(context.Background(), place, from.AddDate(0, 0, 1), to.AddDate(0, 0, 1), tz)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh[0].Date.Equal(from.AddDate(0, 0, 1)) {
		t.Errorf("expected refreshed span starting %v, got %v", from.AddDate(0, 0, 1), fresh[0].Date)
	}
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want still 2", got)
	}
}

func TestCachedProviderStaleCeiling(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	fake := NewFakeProvider()
	counter := &countingProvider{inner: fake}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}

	// Past the stale ceiling the old entry is unservable: the fetch happens
	// inline, and a provider failure surfaces as an error, not ancient data.
	now = now.Add(defaultStaleCeiling + time.Minute)
	fake.Err = errors.New("boom")
	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err == nil {
		t.Error("expected error past the stale ceiling, got served data")
	}
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2 (inline refetch past ceiling)", got)
	}
}

func TestCachedProviderFailureBackoff(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	fake := &FakeProvider{Err: errors.New("boom")}
	counter := &countingProvider{inner: fake}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err == nil {
		t.Fatal("expected provider error")
	}
	if got := counter.callCount(); got != 1 {
		t.Fatalf("inner called %d times, want 1", got)
	}

	// Within the backoff window, no refetch: the failure is served from memory.
	now = now.Add(time.Minute)
	_, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err == nil || !strings.Contains(err.Error(), "suppressed") {
		t.Fatalf("expected suppressed error, got %v", err)
	}
	if got := counter.callCount(); got != 1 {
		t.Errorf("inner called %d times during backoff, want still 1", got)
	}

	// Past the backoff the provider is retried; a recovered provider serves.
	now = now.Add(defaultFailureBackoff)
	fake.Err = nil
	days, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err != nil || len(days) == 0 {
		t.Fatalf("expected recovery after backoff, got %d days, err %v", len(days), err)
	}
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2", got)
	}
}

func TestCachedProviderStaleServedDuringBrownout(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	fake := NewFakeProvider()
	counter := &countingProvider{inner: fake}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}

	// TTL expires during a provider brownout: stale is served, the background
	// refresh fails and arms the backoff.
	now = now.Add(defaultCacheTTL + time.Minute)
	fake.Err = errors.New("brownout")
	days, err := cached.DailyWeather(context.Background(), place, from, to, tz)
	if err != nil || len(days) == 0 {
		t.Fatalf("expected stale serve during brownout, got %d days, err %v", len(days), err)
	}
	cached.refreshes.Wait()
	if got := counter.callCount(); got != 2 {
		t.Fatalf("inner called %d times, want 2 (failed refresh)", got)
	}

	// While suppressed, stale keeps being served with no further provider hits.
	now = now.Add(time.Minute)
	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}
	cached.refreshes.Wait()
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times during backoff, want still 2", got)
	}

	// Backoff expiry + recovered provider: one refresh brings the cache current.
	now = now.Add(defaultFailureBackoff)
	fake.Err = nil
	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}
	cached.refreshes.Wait()
	if got := counter.callCount(); got != 3 {
		t.Errorf("inner called %d times after recovery, want 3", got)
	}
}

func TestCachedProviderCollapsesConcurrentFetches(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	gated := newGatedProvider()
	cached := NewCachedProvider(gated)

	results := make(chan int, 2)
	go func() {
		days, err := cached.DailyWeather(context.Background(), place, from, to, tz)
		if err != nil {
			t.Errorf("first caller: %v", err)
		}
		results <- len(days)
	}()
	<-gated.started // the first caller's fetch is executing (and blocked)

	go func() {
		days, err := cached.DailyWeather(context.Background(), place, from, to, tz)
		if err != nil {
			t.Errorf("second caller: %v", err)
		}
		results <- len(days)
	}()

	// Release generously so no interleaving can deadlock; singleflight (or the
	// populated cache, for a late-arriving second caller) keeps the actual
	// provider hits at one.
	gated.release <- struct{}{}
	gated.release <- struct{}{}
	for i := 0; i < 2; i++ {
		if n := <-results; n == 0 {
			t.Error("caller received no days")
		}
	}
	if got := gated.callCount(); got != 1 {
		t.Errorf("inner called %d times, want 1 (singleflight collapse)", got)
	}
}

func TestCachedProviderHourlyStaleWhileRefreshing(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 24, 12, 0, 0, 0, tz)
	to := from.Add(6 * time.Hour)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	counter := &countingProvider{inner: NewFakeProvider()}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	first, err := cached.HourlyWeather(context.Background(), place, from, to, tz)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(defaultCacheTTL + time.Minute)
	stale, err := cached.HourlyWeather(context.Background(), place, from, to, tz)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != len(first) {
		t.Errorf("stale hourly serve returned %d hours, want %d", len(stale), len(first))
	}
	cached.refreshes.Wait()
	if got := counter.callCount(); got != 2 {
		t.Errorf("inner called %d times, want 2 (background hourly refresh)", got)
	}
}

func TestCachedProviderPrunesExpiredEntries(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	place := LatLng{Lat: 39.7, Lng: -105.0}
	counter := &countingProvider{inner: NewFakeProvider()}
	cached := NewCachedProvider(counter)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, tz)
	cached.now = func() time.Time { return now }

	if _, err := cached.DailyWeather(context.Background(), place, from, to, tz); err != nil {
		t.Fatal(err)
	}

	// A fresh store past the ceiling evicts the dead entries (span keys accrue
	// one per day per place; without pruning the maps grow forever).
	now = now.Add(defaultStaleCeiling + time.Hour)
	newFrom := from.AddDate(0, 0, 3)
	if _, err := cached.DailyWeather(context.Background(), place, newFrom, newFrom.AddDate(0, 0, 7), tz); err != nil {
		t.Fatal(err)
	}
	cached.mu.Lock()
	entryCount := len(cached.entries)
	cached.mu.Unlock()
	if entryCount != 1 {
		t.Errorf("expected 1 daily entry after pruning, got %d", entryCount)
	}
}
