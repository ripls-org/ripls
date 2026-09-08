package weather

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"go.ripls.org/ripls/server/logging"
)

const (
	// defaultCacheTTL is how long a place's weather is reused before a refresh.
	// Forecasts move slowly; a few hours collapses call volume to a handful per
	// place per day regardless of how many viewers look at it.
	defaultCacheTTL = 3 * time.Hour

	// defaultStaleCeiling bounds how old an entry may be and still be served
	// stale while a background refresh runs (#2646). A two-day-old daily
	// forecast is still glyph-accurate; beyond it, prefer no weather over
	// ancient weather.
	defaultStaleCeiling = 48 * time.Hour

	// defaultFailureBackoff suppresses refetches for a place after a provider
	// failure, so a provider brownout costs one bounded fetch per window
	// instead of hanging every request until its timeout (#2646).
	defaultFailureBackoff = 5 * time.Minute
)

// CachedProvider wraps a Provider with a per-place, in-process cache. The cache
// key is the snapped ~0.1° grid point plus the requested day span, so every
// viewer near a place shares one upstream fetch. This is the cost lever for
// weather: the external API is never called per user or per request.
//
// Freshness is served stale-while-revalidate (#2646): once an entry outlives
// the TTL (or the day span rolls over at midnight), the most recent good data
// for the place is returned immediately and a background refresh brings the
// cache current — a request only ever waits on the network when the place has
// never been fetched (or the last data is past the stale ceiling). Provider
// failures are negatively cached per place so a brownout can't stall every
// request, and concurrent fetches for one key collapse via singleflight.
//
// The cache is in-process; a multi-instance deployment that wants a single
// shared cache (Redis/Postgres) swaps this implementation without touching the
// Provider interface — see docs/weather.md.
type CachedProvider struct {
	inner          Provider
	ttl            time.Duration
	staleCeiling   time.Duration
	failureBackoff time.Duration
	now            func() time.Time

	// group collapses concurrent upstream fetches for the same span key.
	group singleflight.Group

	mu          sync.Mutex
	entries     map[string]cacheEntry // daily, keyed by exact place+tz+span
	latestDaily map[string]cacheEntry // most recent good daily fetch per place+tz
	hourEntries map[string]hourCacheEntry
	lastFailure map[string]time.Time // per place+tz, set on provider error

	// refreshes tracks in-flight background refreshes so tests can join them.
	refreshes sync.WaitGroup
}

type cacheEntry struct {
	days      []DayWeather
	fetchedAt time.Time
}

type hourCacheEntry struct {
	hours     []HourWeather
	fetchedAt time.Time
}

// NewCachedProvider wraps inner with the default TTL, stale ceiling, and
// failure backoff.
func NewCachedProvider(inner Provider) *CachedProvider {
	return &CachedProvider{
		inner:          inner,
		ttl:            defaultCacheTTL,
		staleCeiling:   defaultStaleCeiling,
		failureBackoff: defaultFailureBackoff,
		now:            time.Now,
		entries:        make(map[string]cacheEntry),
		latestDaily:    make(map[string]cacheEntry),
		hourEntries:    make(map[string]hourCacheEntry),
		lastFailure:    make(map[string]time.Time),
	}
}

// DailyWeather implements Provider, serving from cache when fresh. An expired
// (or span-rolled) entry under the stale ceiling is served immediately while a
// background refresh runs; only a place with no servable data waits on the
// upstream fetch.
func (c *CachedProvider) DailyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	if tz == nil {
		tz = time.UTC
	}
	snapped := place.Snap()
	from = truncToDay(from, tz)
	to = truncToDay(to, tz)
	placeKey := fmt.Sprintf("%.1f,%.1f|%s", snapped.Lat, snapped.Lng, tz.String())
	spanKey := fmt.Sprintf("%s|%s|%s", placeKey,
		from.Format("2006-01-02"), to.Format("2006-01-02"))

	c.mu.Lock()
	now := c.now()
	if e, ok := c.entries[spanKey]; ok && now.Sub(e.fetchedAt) < c.ttl {
		days := e.days
		c.mu.Unlock()
		return days, nil
	}
	// Stale fallback is keyed by place, not span: after the midnight span
	// rollover (or a different caller's window) the freshest data for the
	// place is still the right thing to show while the refresh runs. The
	// caller keys days by date, so span misalignment is harmless.
	stale, hasStale := c.latestDaily[placeKey]
	hasStale = hasStale && now.Sub(stale.fetchedAt) < c.staleCeiling
	suppressed := now.Sub(c.lastFailure[placeKey]) < c.failureBackoff
	c.mu.Unlock()

	logger := logging.LoggerWithContext(ctx)
	if hasStale {
		if suppressed {
			logger.DebugContext(ctx, "weather refetch suppressed after recent provider failure; serving stale",
				"weather_key", placeKey)
		} else {
			logger.DebugContext(ctx, "serving stale weather while refreshing", "weather_key", placeKey)
			c.refreshDaily(ctx, placeKey, spanKey, snapped, from, to, tz)
		}
		return stale.days, nil
	}
	if suppressed {
		return nil, fmt.Errorf("weather fetch for %s suppressed after recent provider failure", placeKey)
	}
	return c.fetchDaily(ctx, placeKey, spanKey, snapped, from, to, tz)
}

// refreshDaily updates the cache in the background, detached from the request
// context — the caller has already been served stale data and the RPC may
// complete (cancelling ctx) before the fetch finishes.
func (c *CachedProvider) refreshDaily(ctx context.Context, placeKey, spanKey string, place LatLng, from, to time.Time, tz *time.Location) {
	bCtx := context.WithoutCancel(ctx)
	c.refreshes.Add(1)
	logging.GoSafe(bCtx, "weather-daily-refresh", func() {
		defer c.refreshes.Done()
		if _, err := c.fetchDaily(bCtx, placeKey, spanKey, place, from, to, tz); err != nil {
			logging.LoggerWithContext(bCtx).WarnContext(bCtx, "background weather refresh failed",
				"weather_key", placeKey, "error", err)
		}
	})
}

// fetchDaily performs the upstream daily fetch (collapsed with any concurrent
// fetch for the same span) and records the outcome: success populates both the
// exact-span entry and the place's stale-fallback entry; failure arms the
// per-place backoff.
func (c *CachedProvider) fetchDaily(ctx context.Context, placeKey, spanKey string, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	v, err, _ := c.group.Do("daily|"+spanKey, func() (any, error) {
		days, err := c.inner.DailyWeather(ctx, place, from, to, tz)
		c.mu.Lock()
		defer c.mu.Unlock()
		if err != nil {
			c.lastFailure[placeKey] = c.now()
			return nil, err
		}
		delete(c.lastFailure, placeKey)
		e := cacheEntry{days: days, fetchedAt: c.now()}
		c.entries[spanKey] = e
		c.latestDaily[placeKey] = e
		c.pruneLocked()
		return days, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]DayWeather), nil
}

// HourlyWeather implements Provider, serving from a per-place hourly cache when
// fresh. The cache key snaps the place to the same ~0.1° grid and keys on the
// exact hour span, so every viewer exploring a day at a place shares one fetch.
// An expired entry under the stale ceiling is served while a background refresh
// runs; unlike the daily path there is no cross-span stale fallback — hours for
// a different span are the wrong data, not slightly-old data.
func (c *CachedProvider) HourlyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	if tz == nil {
		tz = time.UTC
	}
	snapped := place.Snap()
	from = from.In(tz)
	to = to.In(tz)
	placeKey := fmt.Sprintf("%.1f,%.1f|%s", snapped.Lat, snapped.Lng, tz.String())
	hourKey := fmt.Sprintf("%s|%s|%s", placeKey,
		from.Format("2006-01-02T15"), to.Format("2006-01-02T15"))

	c.mu.Lock()
	now := c.now()
	e, ok := c.hourEntries[hourKey]
	if ok && now.Sub(e.fetchedAt) < c.ttl {
		hours := e.hours
		c.mu.Unlock()
		return hours, nil
	}
	hasStale := ok && now.Sub(e.fetchedAt) < c.staleCeiling
	suppressed := now.Sub(c.lastFailure[placeKey]) < c.failureBackoff
	c.mu.Unlock()

	logger := logging.LoggerWithContext(ctx)
	if hasStale {
		if suppressed {
			logger.DebugContext(ctx, "hourly weather refetch suppressed after recent provider failure; serving stale",
				"weather_key", placeKey)
		} else {
			logger.DebugContext(ctx, "serving stale hourly weather while refreshing", "weather_key", placeKey)
			c.refreshHourly(ctx, placeKey, hourKey, snapped, from, to, tz)
		}
		return e.hours, nil
	}
	if suppressed {
		return nil, fmt.Errorf("hourly weather fetch for %s suppressed after recent provider failure", placeKey)
	}
	return c.fetchHourly(ctx, placeKey, hourKey, snapped, from, to, tz)
}

// refreshHourly is the hourly counterpart of refreshDaily.
func (c *CachedProvider) refreshHourly(ctx context.Context, placeKey, hourKey string, place LatLng, from, to time.Time, tz *time.Location) {
	bCtx := context.WithoutCancel(ctx)
	c.refreshes.Add(1)
	logging.GoSafe(bCtx, "weather-hourly-refresh", func() {
		defer c.refreshes.Done()
		if _, err := c.fetchHourly(bCtx, placeKey, hourKey, place, from, to, tz); err != nil {
			logging.LoggerWithContext(bCtx).WarnContext(bCtx, "background hourly weather refresh failed",
				"weather_key", placeKey, "error", err)
		}
	})
}

// fetchHourly performs the upstream hourly fetch (collapsed per hour-span key)
// and records the outcome, mirroring fetchDaily.
func (c *CachedProvider) fetchHourly(ctx context.Context, placeKey, hourKey string, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	v, err, _ := c.group.Do("hourly|"+hourKey, func() (any, error) {
		hours, err := c.inner.HourlyWeather(ctx, place, from, to, tz)
		c.mu.Lock()
		defer c.mu.Unlock()
		if err != nil {
			c.lastFailure[placeKey] = c.now()
			return nil, err
		}
		delete(c.lastFailure, placeKey)
		c.hourEntries[hourKey] = hourCacheEntry{hours: hours, fetchedAt: c.now()}
		c.pruneLocked()
		return hours, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]HourWeather), nil
}

// pruneLocked evicts entries past the stale ceiling and expired failure
// records. Span-keyed entries accrue one key per day per place (the daily span
// rolls at midnight) and one per explored hour window, so without pruning the
// maps grow forever. Called with c.mu held, on each successful store.
func (c *CachedProvider) pruneLocked() {
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.fetchedAt) >= c.staleCeiling {
			delete(c.entries, k)
		}
	}
	for k, e := range c.latestDaily {
		if now.Sub(e.fetchedAt) >= c.staleCeiling {
			delete(c.latestDaily, k)
		}
	}
	for k, e := range c.hourEntries {
		if now.Sub(e.fetchedAt) >= c.staleCeiling {
			delete(c.hourEntries, k)
		}
	}
	for k, t := range c.lastFailure {
		if now.Sub(t) >= c.failureBackoff {
			delete(c.lastFailure, k)
		}
	}
}
