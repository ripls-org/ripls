package weatherapi

import (
	"context"
	"testing"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/weather"
)

func TestFormatTemp(t *testing.T) {
	if got := FormatTemp(22.2, true); got != "72°" { // 22.2C ≈ 72F
		t.Errorf("FormatTemp F = %q, want 72°", got)
	}
	if got := FormatTemp(22.2, false); got != "22°" {
		t.Errorf("FormatTemp C = %q, want 22°", got)
	}
}

func TestUsesFahrenheitDefaultsTrue(t *testing.T) {
	denver, _ := time.LoadLocation("America/Denver")
	paris, _ := time.LoadLocation("Europe/Paris")
	if !UsesFahrenheit(denver) || !UsesFahrenheit(paris) || !UsesFahrenheit(nil) {
		t.Error("temperature should default to Fahrenheit for all locales")
	}
}

func TestConditionToAPI(t *testing.T) {
	cases := map[weather.Condition]api.DayForecastCondition{
		weather.ConditionClear:        api.DayForecastCondition_DAY_FORECAST_CONDITION_CLEAR,
		weather.ConditionRain:         api.DayForecastCondition_DAY_FORECAST_CONDITION_RAIN,
		weather.ConditionUnknown:      api.DayForecastCondition_DAY_FORECAST_CONDITION_UNSPECIFIED,
		weather.ConditionPartlyCloudy: api.DayForecastCondition_DAY_FORECAST_CONDITION_PARTLY_CLOUDY,
	}
	for in, want := range cases {
		if got := ConditionToAPI(in); got != want {
			t.Errorf("ConditionToAPI(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestSummaryIncludesTemp(t *testing.T) {
	if got := Summary(weather.ConditionClear, 22.2, true); got != "Clear and 72°" {
		t.Errorf("Summary = %q, want %q", got, "Clear and 72°")
	}
}

func TestDayForecastFor(t *testing.T) {
	tz := time.UTC
	day := time.Date(2026, 6, 20, 14, 0, 0, 0, tz)
	fc := DayForecastFor(context.Background(), weather.NewFakeProvider(), weather.LatLng{Lat: 39.7, Lng: -105}, day, tz)
	if fc == nil {
		t.Fatal("expected a forecast")
	}
	if fc.GetDateUnixSec() != time.Date(2026, 6, 20, 0, 0, 0, 0, tz).Unix() {
		t.Errorf("date = %d, want local midnight of the 20th", fc.GetDateUnixSec())
	}
	if fc.GetTemperatureDisplay() == "" {
		t.Error("expected a formatted temperature")
	}
}

func TestDayForecastForNilProvider(t *testing.T) {
	if DayForecastFor(context.Background(), nil, weather.LatLng{}, time.Now(), time.UTC) != nil {
		t.Error("nil provider should yield nil forecast")
	}
}

func TestHourlyForecastFor(t *testing.T) {
	tz := time.UTC
	day := time.Date(2026, 6, 20, 14, 0, 0, 0, tz)
	hours := HourlyForecastFor(context.Background(), weather.NewFakeProvider(), weather.LatLng{Lat: 39.7, Lng: -105}, day, tz)
	if len(hours) != 24 {
		t.Fatalf("expected 24 hours for the local day, got %d", len(hours))
	}
	if hours[0].GetTimeUnixSec() != time.Date(2026, 6, 20, 0, 0, 0, 0, tz).Unix() {
		t.Errorf("first hour = %d, want local midnight of the 20th", hours[0].GetTimeUnixSec())
	}
	if hours[0].GetTemperatureDisplay() == "" {
		t.Error("expected a formatted temperature")
	}
}

func TestHourlyForecastForNilProvider(t *testing.T) {
	if HourlyForecastFor(context.Background(), nil, weather.LatLng{}, time.Now(), time.UTC) != nil {
		t.Error("nil provider should yield nil hours")
	}
}
