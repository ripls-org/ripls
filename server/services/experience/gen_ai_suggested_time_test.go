package experience

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGenExperience_SuggestedTime verifies that the server correctly wires the
// LLM-extracted date/time into the suggested_time response field.
func TestGenExperience_SuggestedTime(t *testing.T) {
	// Pick a reference "now" in UTC so location parsing is deterministic.
	refNow := time.Date(2025, 12, 1, 15, 0, 0, 0, time.UTC)
	refNowUnix := refNow.Unix()

	futureDate := "2025-12-02"
	futureTime := "09:00"
	futureUnix := time.Date(2025, 12, 2, 9, 0, 0, 0, time.UTC).Unix()

	pastDate := "2025-11-30"
	pastTimeStr := "08:00"
	pastUnix := time.Date(2025, 11, 30, 8, 0, 0, 0, time.UTC).Unix()

	tests := []struct {
		name         string
		aiResponse   *ai.ExperienceGeneration
		wantSpecific bool  // true → SpecificTime, false → TBD
		wantUnixSec  int64 // expected unix sec when wantSpecific=true
	}{
		{
			name: "EXPLICIT future time returns SpecificTime",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Morning Hike",
				Description:    "Let's go hiking tomorrow morning",
				Date:           futureDate,
				Time:           futureTime,
				TimeConfidence: "EXPLICIT",
			},
			wantSpecific: true,
			wantUnixSec:  futureUnix,
		},
		{
			name: "INFERRED future time returns SpecificTime",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Morning Hike",
				Description:    "Let's go hiking tomorrow morning",
				Date:           futureDate,
				Time:           futureTime,
				TimeConfidence: "INFERRED",
			},
			wantSpecific: true,
			wantUnixSec:  futureUnix,
		},
		{
			name: "EXPLICIT past time returns SpecificTime",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Morning Hike",
				Description:    "We went hiking yesterday morning",
				Date:           pastDate,
				Time:           pastTimeStr,
				TimeConfidence: "EXPLICIT",
			},
			wantSpecific: true,
			wantUnixSec:  pastUnix,
		},
		{
			name: "UNKNOWN confidence returns TBD even when date is set",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Morning Hike",
				Description:    "Let's go hiking sometime",
				Date:           futureDate,
				Time:           futureTime,
				TimeConfidence: "UNKNOWN",
			},
			wantSpecific: false,
		},
		{
			name: "Empty date returns TBD regardless of confidence",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Coffee hangout",
				Description:    "Grab coffee sometime",
				Date:           "",
				Time:           "",
				TimeConfidence: "UNKNOWN",
			},
			wantSpecific: false,
		},
		{
			name: "Date only no time returns SpecificTime at midnight",
			aiResponse: &ai.ExperienceGeneration{
				Title:          "Day trip",
				Description:    "Day trip tomorrow",
				Date:           futureDate,
				Time:           "", // no time component
				TimeConfidence: "INFERRED",
			},
			wantSpecific: true,
			wantUnixSec:  time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC).Unix(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupTestService(t)
			mock := ai.NewMockProvider()
			mock.GenerateExperienceFromTextFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceGeneration, error) {
				return tc.aiResponse, nil
			}
			service.aiProvider = mock

			ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
			req := connect.NewRequest(&api.GenExperienceRequest{
				Prompt:             &api.GenExperienceRequest_Text{Text: "test prompt"},
				CurrentTimeUnixSec: refNowUnix,
			})

			resp, err := service.GenExperience(ctx, req)
			if err != nil {
				t.Fatalf("GenExperience failed: %v", err)
			}

			if tc.wantSpecific {
				specific, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Specific)
				if !ok {
					t.Fatalf("Expected SpecificTime, got %T", resp.Msg.SuggestedTime.TimeType)
				}
				if specific.Specific.UnixTimestampSec != tc.wantUnixSec {
					t.Errorf("Expected unix_sec=%d, got %d", tc.wantUnixSec, specific.Specific.UnixTimestampSec)
				}
			} else {
				if _, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Tbd); !ok {
					t.Errorf("Expected TBD time, got %T", resp.Msg.SuggestedTime.TimeType)
				}
			}
		})
	}
}
