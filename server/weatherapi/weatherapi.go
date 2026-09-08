// Package weatherapi converts the transport-agnostic weather.DayWeather into
// the api.DayForecast wire type — the coarse condition, a locale-formatted
// temperature, and a short summary. It is the single home for that conversion
// so every surface that shows weather (the Home calendar, an event) formats it
// identically. See docs/weather.md.
package weatherapi

import (
	"context"
	"fmt"
	"math"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/weather"
)

// ConditionToAPI maps a coarse weather condition to its wire enum.
func ConditionToAPI(c weather.Condition) api.DayForecastCondition {
	switch c {
	case weather.ConditionClear:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_CLEAR
	case weather.ConditionMostlySunny:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_MOSTLY_SUNNY
	case weather.ConditionPartlyCloudy:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_PARTLY_CLOUDY
	case weather.ConditionOvercast:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_OVERCAST
	case weather.ConditionRain:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_RAIN
	default:
		return api.DayForecastCondition_DAY_FORECAST_CONDITION_UNSPECIFIED
	}
}

// UsesFahrenheit returns the temperature unit for the viewer. The app defaults
// to Fahrenheit everywhere for the US-first launch; the parameter is retained
// as the seam for a future locale-based unit choice (docs/weather.md).
func UsesFahrenheit(_ *time.Location) bool {
	return true
}

// FormatTemp renders a Celsius high as a whole-degree label, in Fahrenheit
// when requested, e.g. "72°".
func FormatTemp(tempC float64, fahrenheit bool) string {
	v := tempC
	if fahrenheit {
		v = tempC*9/5 + 32
	}
	return fmt.Sprintf("%d°", int(math.Round(v)))
}

// Summary is a short English label combining condition and temperature. The
// client builds its own localized accessibility label from the condition enum
// + temperature_display; this string is a server-side fallback only.
func Summary(c weather.Condition, tempC float64, fahrenheit bool) string {
	temp := FormatTemp(tempC, fahrenheit)
	switch c {
	case weather.ConditionClear:
		return "Clear and " + temp
	case weather.ConditionMostlySunny:
		return "Sun breaking through, " + temp
	case weather.ConditionPartlyCloudy:
		return "Mild and " + temp
	case weather.ConditionOvercast:
		return "Overcast but dry, " + temp
	case weather.ConditionRain:
		return "Showers on and off, " + temp
	default:
		return temp
	}
}

// ToAPI converts a single day's weather to the wire type, formatted for tz.
func ToAPI(d weather.DayWeather, tz *time.Location) *api.DayForecast {
	fahrenheit := UsesFahrenheit(tz)
	return &api.DayForecast{
		DateUnixSec:        truncDay(d.Date, tz).Unix(),
		Condition:          ConditionToAPI(d.Condition),
		TemperatureDisplay: FormatTemp(d.HighTempC, fahrenheit),
		Summary:            Summary(d.Condition, d.HighTempC, fahrenheit),
		IsTypical:          d.IsTypical,
	}
}

// HourToAPI converts a single hour's weather to the wire type, formatted for tz.
func HourToAPI(h weather.HourWeather, tz *time.Location) *api.HourForecast {
	return &api.HourForecast{
		TimeUnixSec:        h.Time.Unix(),
		Condition:          ConditionToAPI(h.Condition),
		TemperatureDisplay: FormatTemp(h.TempC, UsesFahrenheit(tz)),
	}
}

// HourlyForecastFor fetches the hour-by-hour forecast for the local day
// containing `day` at `place`, formatted for tz. Best-effort: returns nil when
// there is no provider, no hours come back (e.g. the day is beyond the forecast
// horizon), or the provider errors.
func HourlyForecastFor(ctx context.Context, provider weather.Provider, place weather.LatLng, day time.Time, tz *time.Location) []*api.HourForecast {
	if provider == nil {
		return nil
	}
	if tz == nil {
		tz = time.UTC
	}
	from := truncDay(day, tz)
	hours, err := provider.HourlyWeather(ctx, place, from, from.AddDate(0, 0, 1), tz)
	if err != nil || len(hours) == 0 {
		return nil
	}
	out := make([]*api.HourForecast, 0, len(hours))
	for _, h := range hours {
		out = append(out, HourToAPI(h, tz))
	}
	return out
}

// DayForecastFor fetches the forecast for the local day containing `day` at
// `place`, formatted for tz. Best-effort: returns nil when no provider, no
// usable place, a provider error, or no day comes back.
func DayForecastFor(ctx context.Context, provider weather.Provider, place weather.LatLng, day time.Time, tz *time.Location) *api.DayForecast {
	if provider == nil {
		return nil
	}
	from := truncDay(day, tz)
	days, err := provider.DailyWeather(ctx, place, from, from.AddDate(0, 0, 1), tz)
	if err != nil || len(days) == 0 {
		return nil
	}
	return ToAPI(days[0], tz)
}

func truncDay(t time.Time, tz *time.Location) time.Time {
	if tz == nil {
		tz = time.UTC
	}
	t = t.In(tz)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}
