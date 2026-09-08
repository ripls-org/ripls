package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/weather"
)

// TestService_GetExperienceDayWeather covers the "When" screen weather strip:
// with coordinates + a weather provider it returns hourly forecasts, and it
// degrades to an empty slice (never an error) when either is missing.
func TestService_GetExperienceDayWeather(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	communityID := createTestCommunity(t, testStorage, "Weather Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Weather Strip Event",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	const eventTS = int64(1750795200) // a fixed specific time
	locID := "loc-" + expID
	if _, err := testStorage.Insert(context.Background(), &models.Location{Id: locID}); err != nil {
		t.Fatalf("insert location: %v", err)
	}
	// Point the experience at the location and give it a scheduled, timezoned time.
	exp := &models.Experience{}
	if err := testStorage.GetByID(context.Background(), expID, exp); err != nil {
		t.Fatalf("get experience: %v", err)
	}
	exp.LocationId = locID
	exp.Time = &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: eventTS,
				Timezone:         "America/Denver",
				DurationMinutes:  60,
			},
		},
	}
	if err := testStorage.Update(context.Background(), exp); err != nil {
		t.Fatalf("update experience: %v", err)
	}

	// setCoords toggles the location's geolocation (nil → no coordinates).
	setCoords := func(t *testing.T, lat, lng float64) {
		t.Helper()
		loc := &models.Location{}
		if err := testStorage.GetByID(context.Background(), locID, loc); err != nil {
			t.Fatalf("get location: %v", err)
		}
		if lat == 0 && lng == 0 {
			loc.Geolocation = nil
		} else {
			loc.Geolocation = &models.Geolocation{LatitudeDeg: lat, LongitudeDeg: lng}
		}
		if err := testStorage.Update(context.Background(), loc); err != nil {
			t.Fatalf("update location: %v", err)
		}
	}

	req := connect.NewRequest(&api.GetExperienceDayWeatherRequest{
		ExperienceId: expID,
		DateUnixSec:  eventTS,
	})

	t.Run("no provider yields empty hours", func(t *testing.T) {
		setCoords(t, 39.74, -104.99)
		resp, err := service.GetExperienceDayWeather(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetExperienceDayWeather failed: %v", err)
		}
		if len(resp.Msg.Hours) != 0 {
			t.Errorf("expected no hours without a provider, got %d", len(resp.Msg.Hours))
		}
	})

	service.SetWeatherProvider(weather.NewFakeProvider())

	t.Run("with provider + coordinates returns a full day of hours", func(t *testing.T) {
		setCoords(t, 39.74, -104.99)
		resp, err := service.GetExperienceDayWeather(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetExperienceDayWeather failed: %v", err)
		}
		if len(resp.Msg.Hours) != 24 {
			t.Fatalf("expected 24 hours, got %d", len(resp.Msg.Hours))
		}
		// Earliest-first, with a formatted temperature label.
		if resp.Msg.Hours[0].TimeUnixSec >= resp.Msg.Hours[1].TimeUnixSec {
			t.Error("hours should be earliest-first")
		}
		if resp.Msg.Hours[0].TemperatureDisplay == "" {
			t.Error("hour should carry a formatted temperature")
		}
		// The coarse daily fallback is also populated.
		if resp.Msg.DayForecast == nil {
			t.Error("expected a daily fallback forecast alongside the hours")
		}
	})

	t.Run("no coordinates yields empty hours", func(t *testing.T) {
		setCoords(t, 0, 0)
		resp, err := service.GetExperienceDayWeather(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetExperienceDayWeather failed: %v", err)
		}
		if len(resp.Msg.Hours) != 0 {
			t.Errorf("expected no hours without coordinates, got %d", len(resp.Msg.Hours))
		}
	})

	calReq := connect.NewRequest(&api.GetExperienceCalendarWeatherRequest{ExperienceId: expID})

	t.Run("calendar weather spans the window at the event location", func(t *testing.T) {
		setCoords(t, 39.74, -104.99)
		resp, err := service.GetExperienceCalendarWeather(ownerCtx, calReq)
		if err != nil {
			t.Fatalf("GetExperienceCalendarWeather failed: %v", err)
		}
		if len(resp.Msg.Days) < calendarWeatherWindowDays {
			t.Errorf("expected at least %d days, got %d", calendarWeatherWindowDays, len(resp.Msg.Days))
		}
		if len(resp.Msg.Days) > 0 && resp.Msg.Days[0].TemperatureDisplay == "" {
			t.Error("day should carry a formatted temperature")
		}
	})

	t.Run("calendar weather is empty without coordinates", func(t *testing.T) {
		setCoords(t, 0, 0)
		resp, err := service.GetExperienceCalendarWeather(ownerCtx, calReq)
		if err != nil {
			t.Fatalf("GetExperienceCalendarWeather failed: %v", err)
		}
		if len(resp.Msg.Days) != 0 {
			t.Errorf("expected no days without coordinates, got %d", len(resp.Msg.Days))
		}
	})
}
