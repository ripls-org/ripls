package portfolio

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.ripls.org/ripls/server/weather"
)

// blockingWeatherProvider blocks DailyWeather until release is closed, then
// records the context error it observed and closes done. It lets tests prove
// the budget path returns early while the detached fetch keeps running.
type blockingWeatherProvider struct {
	release chan struct{}
	done    chan struct{}
	ctxErr  error
}

func newBlockingWeatherProvider() *blockingWeatherProvider {
	return &blockingWeatherProvider{
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (b *blockingWeatherProvider) DailyWeather(ctx context.Context, _ weather.LatLng, from, to time.Time, tz *time.Location) ([]weather.DayWeather, error) {
	<-b.release
	b.ctxErr = ctx.Err() // written before close(done); the test reads after <-done
	close(b.done)
	return weather.NewFakeProvider().DailyWeather(ctx, weather.LatLng{}, from, to, tz)
}

func (b *blockingWeatherProvider) HourlyWeather(context.Context, weather.LatLng, time.Time, time.Time, *time.Location) ([]weather.HourWeather, error) {
	return nil, nil
}

func TestFetchForecastWithBudgetFastPath(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	to := from.AddDate(0, 0, 7)
	days, err, timedOut := fetchForecastWithBudget(
		context.Background(), weather.NewFakeProvider(), weather.LatLng{}, from, to, tz, 5*time.Second,
	)
	if timedOut {
		t.Fatal("fast provider reported as timed out")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(days) != 7 {
		t.Errorf("got %d days, want 7", len(days))
	}
}

func TestFetchForecastWithBudgetProviderError(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	provider := &weather.FakeProvider{Err: errors.New("boom")}
	_, err, timedOut := fetchForecastWithBudget(
		context.Background(), provider, weather.LatLng{}, from, from.AddDate(0, 0, 7), tz, 5*time.Second,
	)
	if timedOut {
		t.Fatal("provider error reported as timeout")
	}
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestFetchForecastWithBudgetTimeoutKeepsFetchAlive(t *testing.T) {
	tz := time.UTC
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, tz)
	provider := newBlockingWeatherProvider()
	ctx, cancel := context.WithCancel(context.Background())

	days, err, timedOut := fetchForecastWithBudget(
		ctx, provider, weather.LatLng{}, from, from.AddDate(0, 0, 7), tz, 10*time.Millisecond,
	)
	if !timedOut {
		t.Fatal("blocking provider did not report timeout")
	}
	if days != nil || err != nil {
		t.Fatalf("timeout should yield (nil, nil, true), got days=%v err=%v", days, err)
	}

	// The request finished (context cancelled), but the detached fetch must
	// keep running with a live context so it can warm the provider cache.
	cancel()
	close(provider.release)
	select {
	case <-provider.done:
	case <-time.After(2 * time.Second):
		t.Fatal("detached fetch never completed")
	}
	if provider.ctxErr != nil {
		t.Errorf("detached fetch saw cancelled context: %v (want context.WithoutCancel)", provider.ctxErr)
	}
}
