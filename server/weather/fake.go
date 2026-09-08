package weather

import (
	"context"
	"time"
)

// FakeProvider returns deterministic weather derived from the day-of-year, so
// tests exercise the full assembly path without external calls or flakiness.
// Days beyond a configurable horizon are marked IsTypical, mirroring the real
// provider's forecast/climate-normal split.
type FakeProvider struct {
	// TypicalAfter, when non-nil, marks days on or after it as IsTypical.
	TypicalAfter *time.Time
	// Err, when set, is returned from DailyWeather/HourlyWeather to exercise the
	// error path.
	Err error
	// HourlyHorizon, when non-nil, omits hours on or after it from HourlyWeather,
	// mirroring the real provider's "no hourly past the horizon" behavior.
	HourlyHorizon *time.Time
}

// NewFakeProvider builds a fake provider with no error and no typical cutoff.
func NewFakeProvider() *FakeProvider { return &FakeProvider{} }

// DailyWeather implements Provider deterministically.
func (f *FakeProvider) DailyWeather(_ context.Context, _ LatLng, from, to time.Time, tz *time.Location) ([]DayWeather, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if tz == nil {
		tz = time.UTC
	}
	from = truncToDay(from, tz)
	to = truncToDay(to, tz)
	var out []DayWeather
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		yday := d.YearDay()
		out = append(out, DayWeather{
			Date:      d,
			Condition: Condition((yday % 5) + 1), // cycles Clear..Rain
			HighTempC: 10 + float64(yday%18),     // 10–27°C
			IsTypical: f.TypicalAfter != nil && !d.Before(*f.TypicalAfter),
		})
	}
	return out, nil
}

// HourlyWeather implements Provider deterministically, one row per hour.
func (f *FakeProvider) HourlyWeather(_ context.Context, _ LatLng, from, to time.Time, tz *time.Location) ([]HourWeather, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if tz == nil {
		tz = time.UTC
	}
	from = from.In(tz).Truncate(time.Hour)
	to = to.In(tz)
	var out []HourWeather
	for t := from; t.Before(to); t = t.Add(time.Hour) {
		if f.HourlyHorizon != nil && !t.Before(*f.HourlyHorizon) {
			continue
		}
		out = append(out, HourWeather{
			Time:      t,
			Condition: Condition((t.Hour() % 5) + 1), // cycles Clear..Rain by hour
			TempC:     10 + float64(t.Hour()%18),     // 10–27°C
		})
	}
	return out, nil
}
