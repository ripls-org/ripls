package portfolio

import (
	"context"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/weather"
	"go.ripls.org/ripls/server/weatherapi"
)

// homeForecastWindowDays is how far forward the calendar paints weather. It
// spans the rest of the current month plus the next, so paging a month either
// way still shows glyphs; days past the provider's forecast horizon come back as
// climate normals (IsTypical).
const homeForecastWindowDays = 46

// homeWeatherFetchBudget bounds how long GetHomeView waits for a weather fetch.
// The cache serves stale data instantly, so only a place with no cached data at
// all (first view on a fresh instance) reaches the network here — and the app's
// front door must not stall on it (#2646). Past the budget the view renders
// without weather while the fetch finishes in the background and warms the
// cache for the next request.
const homeWeatherFetchBudget = 1200 * time.Millisecond

// fetchHomeForecast resolves the viewer's place and returns per-day forecasts
// for the calendar window, converted to the wire type and formatted for the
// recipient. Best-effort: any failure (no provider, no location, provider error)
// yields nil so the calendar simply renders without weather — a weather failure
// never blocks the Home view. Mirrors the inbox-nudge pattern.
func (s *Service) fetchHomeForecast(ctx context.Context, d *fetchedData, userID string, tz *time.Location, now time.Time) []*api.DayForecast {
	logger := logging.LoggerWithContext(ctx).With("operation", "fetchHomeForecast", "user_id", userID)
	if s.weatherProvider == nil {
		logger.WarnContext(ctx, "weather skipped: no provider configured")
		return nil
	}
	place, ok := s.resolveViewerPlace(ctx, d, userID)
	if !ok {
		logger.InfoContext(ctx, "weather skipped: no usable viewer location (no home, saved, or event location with coordinates)")
		return nil
	}

	from := truncDayLocal(now, tz)
	to := from.AddDate(0, 0, homeForecastWindowDays)
	days, err, timedOut := fetchForecastWithBudget(ctx, s.weatherProvider, place, from, to, tz, homeWeatherFetchBudget)
	if timedOut {
		logger.WarnContext(ctx, "weather skipped: fetch exceeded budget; cache warms in background",
			"budget_ms", homeWeatherFetchBudget.Milliseconds(),
			"lat", place.Lat, "lng", place.Lng)
		return nil
	}
	if err != nil {
		logger.WarnContext(ctx, "weather skipped: provider error", "error", err,
			"lat", place.Lat, "lng", place.Lng)
		return nil
	}
	if len(days) == 0 {
		logger.WarnContext(ctx, "weather skipped: provider returned no days",
			"lat", place.Lat, "lng", place.Lng,
			"from", from.Format("2006-01-02"), "to", to.Format("2006-01-02"))
		return nil
	}

	out := make([]*api.DayForecast, 0, len(days))
	for _, d := range days {
		out = append(out, weatherapi.ToAPI(d, tz))
	}
	logger.InfoContext(ctx, "weather assembled", "day_count", len(out),
		"lat", place.Lat, "lng", place.Lng,
		"first_day", out[0].DateUnixSec, "last_day", out[len(out)-1].DateUnixSec)
	return out
}

// fetchForecastWithBudget runs the provider call detached from the request
// context and waits at most budget for it. On budget expiry it returns
// timedOut=true and the fetch keeps running — the Connect handler context is
// cancelled when the RPC returns, so without context.WithoutCancel the
// abandoned fetch would die and the provider cache would never warm (#2646).
// A panic in the fetch is recovered by GoSafe; nothing arrives on the channel,
// so the budget expires and the view renders without weather.
func fetchForecastWithBudget(
	ctx context.Context,
	provider weather.Provider,
	place weather.LatLng,
	from, to time.Time,
	tz *time.Location,
	budget time.Duration,
) (days []weather.DayWeather, err error, timedOut bool) {
	type result struct {
		days []weather.DayWeather
		err  error
	}
	ch := make(chan result, 1)
	fetchCtx := context.WithoutCancel(ctx)
	logging.GoSafe(fetchCtx, "home-forecast-fetch", func() {
		d, e := provider.DailyWeather(fetchCtx, place, from, to, tz)
		ch <- result{days: d, err: e}
	})
	select {
	case r := <-ch:
		return r.days, r.err, false
	case <-time.After(budget):
		return nil, nil, true
	}
}

// resolveViewerPlace resolves the viewer's place for weather, trying in order:
// their primary-residence location, any other saved location, then the location
// of their events / gear / requests (a get-outside community almost always has
// one). Returns ok=false only when none of those has coordinates. Best-effort
// I/O — locations are batch-fetched in one query.
func (s *Service) resolveViewerPlace(ctx context.Context, d *fetchedData, userID string) (weather.LatLng, bool) {
	// Ordered, deduped candidate location IDs — most relevant first.
	ordered := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ordered = append(ordered, id)
	}

	user := &models.User{}
	if err := s.storage.GetByID(ctx, userID, user); err == nil {
		add(user.GetPrimaryResidenceLocationId())
		for _, id := range user.GetOtherLocationIds() {
			add(id)
		}
	}
	// Content locations: the viewer's events, then gear, then requests.
	for _, e := range d.ownedExperiences {
		add(e.GetLocationId())
	}
	for _, msg := range d.expMap {
		if e, ok := msg.(*models.Experience); ok {
			add(e.GetLocationId())
		}
	}
	for _, g := range d.ownedGear {
		add(g.GetLocationId())
	}
	for _, r := range d.ownedRequests {
		add(r.GetLocationId())
	}

	if len(ordered) == 0 {
		return weather.LatLng{}, false
	}

	locs, err := s.storage.GetByIDs(ctx, ordered, &models.Location{})
	if err != nil {
		return weather.LatLng{}, false
	}
	for _, id := range ordered {
		msg, ok := locs[id]
		if !ok {
			continue
		}
		loc, ok := msg.(*models.Location)
		if !ok {
			continue
		}
		geo := loc.GetGeolocation()
		if geo == nil {
			continue
		}
		lat, lng := geo.GetLatitudeDeg(), geo.GetLongitudeDeg()
		if lat == 0 && lng == 0 {
			continue
		}
		return weather.LatLng{Lat: lat, Lng: lng}, true
	}
	return weather.LatLng{}, false
}

// truncDayLocal returns local midnight of t in tz.
func truncDayLocal(t time.Time, tz *time.Location) time.Time {
	if tz == nil {
		tz = time.UTC
	}
	t = t.In(tz)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}
