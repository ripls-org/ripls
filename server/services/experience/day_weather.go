package experience

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/weather"
	"go.ripls.org/ripls/server/weatherapi"
)

const (
	// calendarWeatherWindowDays is the default forward span of the "When"
	// calendar weather, mirroring the Home calendar's window.
	calendarWeatherWindowDays = 46
	// calendarWeatherMaxDays caps the span so an event far in the future can't
	// trigger an unbounded daily fetch.
	calendarWeatherMaxDays = 120
)

// GetExperienceDayWeather returns the hour-by-hour forecast at the experience's
// location for one local day — the "When" screen weather strip, requested for
// the event day and for any other day the viewer taps to explore. Best-effort:
// the hours slice is empty (never an error) when there is no weather provider,
// no coordinates, or the day is beyond the forecast horizon, so the client can
// always render the strip with a graceful "closer to the day" fallback.
func (s *Service) GetExperienceDayWeather(
	ctx context.Context,
	req *connect.Request[api.GetExperienceDayWeatherRequest],
) (*connect.Response[api.GetExperienceDayWeatherResponse], error) {
	expStored, err := s.authorizeExperienceForWeather(ctx, req.Msg.ExperienceId, "GetExperienceDayWeather")
	if err != nil {
		return nil, err
	}

	resp := &api.GetExperienceDayWeatherResponse{}
	if s.weatherProvider == nil {
		return connect.NewResponse(resp), nil
	}

	place, tz, ok, err := s.resolveExperienceWeatherPlace(ctx, expStored)
	if err != nil {
		return nil, err
	}
	if !ok {
		return connect.NewResponse(resp), nil // no location → no forecast
	}

	// The requested day, interpreted at the event's location/timezone.
	day := time.Unix(req.Msg.DateUnixSec, 0).In(tz)
	resp.Hours = weatherapi.HourlyForecastFor(ctx, s.weatherProvider, place, day, tz)
	// Daily fallback (with the climate-normal fill beyond the hourly horizon) so
	// the client can still show the day's weather when no hourly is available.
	resp.DayForecast = weatherapi.DayForecastFor(ctx, s.weatherProvider, place, day, tz)

	logging.LoggerWithContext(ctx).DebugContext(ctx, "assembled day weather",
		"experience_id", req.Msg.ExperienceId,
		"date_unix_sec", req.Msg.DateUnixSec, "hour_count", len(resp.Hours),
		"has_daily", resp.DayForecast != nil)
	return connect.NewResponse(resp), nil
}

// GetExperienceCalendarWeather returns one DayForecast per local day across the
// "When" calendar window, all at the experience's own location — so the month
// grid paints weather for where the event is, not the viewer's home. The window
// runs from today forward, extended to cover the event day, and capped.
// Best-effort: an empty list (never an error) when there is no provider or no
// coordinates.
func (s *Service) GetExperienceCalendarWeather(
	ctx context.Context,
	req *connect.Request[api.GetExperienceCalendarWeatherRequest],
) (*connect.Response[api.GetExperienceCalendarWeatherResponse], error) {
	expStored, err := s.authorizeExperienceForWeather(ctx, req.Msg.ExperienceId, "GetExperienceCalendarWeather")
	if err != nil {
		return nil, err
	}

	resp := &api.GetExperienceCalendarWeatherResponse{}
	if s.weatherProvider == nil {
		return connect.NewResponse(resp), nil
	}

	place, tz, ok, err := s.resolveExperienceWeatherPlace(ctx, expStored)
	if err != nil {
		return nil, err
	}
	if !ok {
		return connect.NewResponse(resp), nil
	}

	now := time.Unix(clock.UnixSec(ctx), 0).In(tz)
	from := truncDayTZ(now, tz)
	to := from.AddDate(0, 0, calendarWeatherWindowDays)
	// Stretch the window so the event's month is covered when it's further out.
	if sec := expStored.GetTime().GetSpecific().GetUnixTimestampSec(); sec > 0 {
		evEnd := truncDayTZ(time.Unix(sec, 0).In(tz), tz).AddDate(0, 0, 8)
		if evEnd.After(to) {
			to = evEnd
		}
	}
	if maxTo := from.AddDate(0, 0, calendarWeatherMaxDays); to.After(maxTo) {
		to = maxTo
	}

	days, err := s.weatherProvider.DailyWeather(ctx, place, from, to, tz)
	if err != nil {
		// Best-effort — a provider error yields no calendar weather, not a failure.
		logging.LoggerWithContext(ctx).WarnContext(ctx, "calendar weather provider error",
			"experience_id", req.Msg.ExperienceId, "error", err)
		return connect.NewResponse(resp), nil
	}
	resp.Days = make([]*api.DayForecast, 0, len(days))
	for _, d := range days {
		resp.Days = append(resp.Days, weatherapi.ToAPI(d, tz))
	}

	logging.LoggerWithContext(ctx).DebugContext(ctx, "assembled calendar weather",
		"experience_id", req.Msg.ExperienceId, "day_count", len(resp.Days),
		"lat", place.Lat, "lng", place.Lng)
	return connect.NewResponse(resp), nil
}

// authorizeExperienceForWeather runs the shared auth + visibility gate (active
// member of a shared community, or the owner) and returns the stored experience.
func (s *Service) authorizeExperienceForWeather(ctx context.Context, experienceID, op string) (*models.Experience, error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	logger := logging.LoggerWithContext(ctx).With("user_id", authInfo.UserID, "experience_id", experienceID)
	exp, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, op)
	if err != nil {
		return nil, err
	}
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityExperience, exp.Id, exp.OwnerId,
	); err != nil {
		return nil, err
	}
	return exp, nil
}

// resolveExperienceWeatherPlace returns the experience's coordinates and the
// location's timezone (from the event's SpecificTime, else UTC). ok is false
// when the experience has no coordinates.
func (s *Service) resolveExperienceWeatherPlace(ctx context.Context, exp *models.Experience) (weather.LatLng, *time.Location, bool, error) {
	apiExp, err := s.buildAPIExperience(ctx, exp, "")
	if err != nil {
		return weather.LatLng{}, nil, false, err
	}
	lat, lng := apiExp.GetLatitudeDeg(), apiExp.GetLongitudeDeg()
	if lat == 0 && lng == 0 {
		return weather.LatLng{}, time.UTC, false, nil
	}
	tz := time.UTC
	if name := apiExp.GetTime().GetSpecific().GetTimezone(); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			tz = loc
		}
	}
	return weather.LatLng{Lat: lat, Lng: lng}, tz, true, nil
}

// truncDayTZ returns local midnight of t in tz.
func truncDayTZ(t time.Time, tz *time.Location) time.Time {
	t = t.In(tz)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}
