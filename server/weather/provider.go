package weather

import (
	"context"
	"math"
	"time"
)

// Condition is a coarse sky bucket. The set is intentionally small — five
// buckets are all a daily forecast meaningfully supports, and they map 1:1 onto
// the calendar's five glyphs. It mirrors api.DayForecastCondition but stays
// transport-agnostic so the provider layer has no proto dependency.
type Condition int

const (
	// ConditionUnknown is the zero value, used when no condition is available.
	ConditionUnknown Condition = iota
	// ConditionClear is clear / sunny.
	ConditionClear
	// ConditionMostlySunny is mostly sunny with some cloud.
	ConditionMostlySunny
	// ConditionPartlyCloudy is partly cloudy / mild.
	ConditionPartlyCloudy
	// ConditionOvercast is overcast but dry (also fog).
	ConditionOvercast
	// ConditionRain is any precipitation — rain, showers, snow, thunderstorm.
	// The five-bucket scheme has no separate snow glyph; precipitation collapses
	// to one bucket.
	ConditionRain
)

// DayWeather is the weather for a single local calendar day at a place.
type DayWeather struct {
	// Date is local midnight of the day this applies to.
	Date time.Time
	// Condition is the coarse sky bucket for the glyph.
	Condition Condition
	// HighTempC is the forecast (or climate-normal) daily high in Celsius. The
	// provider works in one canonical unit; the caller formats the user-facing
	// unit per the recipient locale so the per-place cache stays locale-neutral.
	HighTempC float64
	// IsTypical is true when this came from climate normals rather than a real
	// forecast — beyond the forecast horizon or a future month.
	IsTypical bool
}

// HourWeather is the weather for a single hour at a place — the granularity the
// event "When" screen paints around the scheduled window. Like DayWeather, the
// temperature is canonical Celsius and the caller formats the user-facing unit.
type HourWeather struct {
	// Time is the start of the hour this applies to, in the requested timezone.
	Time time.Time
	// Condition is the coarse sky bucket for the glyph.
	Condition Condition
	// TempC is the forecast temperature for the hour in Celsius.
	TempC float64
}

// LatLng is a geographic point.
type LatLng struct {
	Lat float64
	Lng float64
}

// gridPrecision rounds a place to a ~0.1° (~11 km) grid. Weather is identical
// across a cell that size for a calendar, and rounding collapses every viewer
// near a place onto one cache key — the lever that keeps API call volume to a
// handful per place per day regardless of user count.
const gridPrecision = 0.1

// Snap rounds a point to the shared ~0.1° cache grid.
func (p LatLng) Snap() LatLng {
	return LatLng{
		Lat: math.Round(p.Lat/gridPrecision) * gridPrecision,
		Lng: math.Round(p.Lng/gridPrecision) * gridPrecision,
	}
}

// Provider fetches per-day weather for a place over a date range. Implementations
// must be safe for concurrent use.
type Provider interface {
	// DailyWeather returns one DayWeather per local day in [from, to] (inclusive
	// of from, exclusive of to) for the given place, in the given timezone. Days
	// beyond the provider's forecast horizon are filled from climate normals and
	// marked IsTypical. Returns the days it could resolve; a partial result is
	// valid. Best-effort — callers treat an error as "no weather".
	DailyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error)

	// HourlyWeather returns one HourWeather per hour in [from, to) for the given
	// place and timezone. Unlike DailyWeather there is no climate-normal
	// fallback: hours beyond the provider's forecast horizon are simply omitted
	// (an hourly climate normal is not meaningful), so a far-future day yields an
	// empty slice and the caller shows "forecast available closer to the day".
	// Best-effort — callers treat an error as "no weather".
	HourlyWeather(ctx context.Context, place LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error)
}
