package experience

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/webfetch"
)

// goldenScenarios mirrors the JSON structure of time_parsing_golden.json.
type goldenScenarios struct {
	TextScenarios     []goldenTextScenario     `json:"generate_experience_from_text"`
	WebpageScenarios  []goldenWebpageScenario  `json:"generate_experience_from_webpage"`
	InformalScenarios []goldenInformalScenario `json:"convert_informal_time"`
}

type goldenTextScenario struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Prompt      string                `json:"prompt"`
	Region      string                `json:"region"`
	CurrentTime string                `json:"current_time"`
	Timezone    string                `json:"timezone"`
	Expected    *goldenExpExpectation `json:"expected,omitempty"`
	ExpectError bool                  `json:"expected_error,omitempty"`
}

type goldenWebpageScenario struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	PageTitle   string                `json:"page_title"`
	PageDesc    string                `json:"page_description"`
	PageBody    string                `json:"page_body"`
	Region      string                `json:"region"`
	CurrentTime string                `json:"current_time"`
	Timezone    string                `json:"timezone"`
	Expected    *goldenExpExpectation `json:"expected,omitempty"`
}

type goldenInformalScenario struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	InformalDescription string                     `json:"informal_description"`
	CurrentTime         string                     `json:"current_time"`
	Timezone            string                     `json:"timezone"`
	Expected            *goldenInformalExpectation `json:"expected,omitempty"`
}

type goldenExpExpectation struct {
	ExpectedDate     string   `json:"expected_date"`
	ExpectedTime     string   `json:"expected_time"`
	ExpectedISO      string   `json:"expected_datetime_iso"`
	TimeConfidence   []string `json:"time_confidence"`
	TitleContains    []string `json:"title_contains"`
	HasDescription   bool     `json:"has_description"`
	LocationContains []string `json:"location_contains"`
	MinConfidence    float32  `json:"min_confidence"`
}

type goldenInformalExpectation struct {
	TimeType     string   `json:"time_type"`
	ExpectedDate string   `json:"expected_date"`
	ExpectedTime string   `json:"expected_time"`
	ExpectedISO  string   `json:"expected_datetime_iso"`
	Duration     int32    `json:"duration_minutes"`
	Confidence   []string `json:"confidence"`
}

// loadGoldenScenarios loads the golden test scenarios.
func loadGoldenScenarios(t *testing.T) *goldenScenarios {
	t.Helper()

	data, err := os.ReadFile("../../test_data/time_parsing_golden.json")
	if err != nil {
		t.Fatalf("Failed to load time_parsing_golden.json: %v", err)
	}

	var scenarios goldenScenarios
	if err := json.Unmarshal(data, &scenarios); err != nil {
		t.Fatalf("Failed to parse golden scenarios: %v", err)
	}

	return &scenarios
}

// parseCurrentTimeUnix parses a current_time string to Unix seconds.
func parseCurrentTimeUnix(t *testing.T, currentTime string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, currentTime)
	if err != nil {
		t.Fatalf("Failed to parse current_time %q: %v", currentTime, err)
	}
	return parsed.Unix()
}

// TestGenExperience_TextTimeParsing tests the full GenExperience pipeline for
// text-based experience creation, verifying that the server correctly converts
// AI responses into timestamps and proto fields.
func TestGenExperience_TextTimeParsing(t *testing.T) {
	scenarios := loadGoldenScenarios(t)

	for _, sc := range scenarios.TextScenarios {
		t.Run(sc.ID, func(t *testing.T) {
			// Test error cases directly.
			if sc.ExpectError {
				service, _, _ := setupTestService(t)
				mock := ai.NewMockProvider()
				service.aiProvider = mock

				ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
				req := connect.NewRequest(&api.GenExperienceRequest{
					Prompt:             &api.GenExperienceRequest_Text{Text: sc.Prompt},
					CurrentTimeUnixSec: parseCurrentTimeUnix(t, sc.CurrentTime),
					Timezone:           sc.Timezone,
				})

				_, err := service.GenExperience(ctx, req)
				if err == nil {
					t.Error("Expected error for empty input, got nil")
				}
				return
			}

			if sc.Expected == nil {
				t.Skip("No expected values defined")
			}

			// Build mock response based on expected values.
			mockResp := &ai.ExperienceGeneration{
				Title:          "Test " + sc.Name,
				Description:    "Test description for " + sc.Name,
				Date:           sc.Expected.ExpectedDate,
				Time:           sc.Expected.ExpectedTime,
				TimeConfidence: sc.Expected.TimeConfidence[0],
				LocationQuery:  "",
				Confidence:     0.85,
				SearchKeywords: []string{"test"},
				ValueEstimate: &ai.ValueEstimate{
					EstimatedValueUSD: 30.0,
					Confidence:        0.7,
					Reasoning:         "Test value",
				},
			}

			if len(sc.Expected.LocationContains) > 0 {
				mockResp.LocationQuery = sc.Expected.LocationContains[0]
			}

			service, _, _ := setupTestService(t)
			mock := ai.NewMockProvider()
			mock.GenerateExperienceFromTextFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceGeneration, error) {
				return mockResp, nil
			}
			service.aiProvider = mock

			ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
			req := connect.NewRequest(&api.GenExperienceRequest{
				Prompt:             &api.GenExperienceRequest_Text{Text: sc.Prompt},
				CurrentTimeUnixSec: parseCurrentTimeUnix(t, sc.CurrentTime),
				Timezone:           sc.Timezone,
			})

			resp, err := service.GenExperience(ctx, req)
			if err != nil {
				t.Fatalf("GenExperience failed: %v", err)
			}

			// Validate time confidence.
			validateTimeConfidence(t, resp.Msg.TimeConfidence, sc.Expected.TimeConfidence)

			// Validate suggested time.
			if sc.Expected.ExpectedDate != "" {
				validateSuggestedTime(t, resp.Msg, sc.Expected, sc.Timezone)
			} else {
				// Expect TBD.
				if resp.Msg.SuggestedTime == nil {
					t.Fatal("Expected SuggestedTime to be set")
				}
				if _, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Tbd); !ok {
					// UNKNOWN confidence should produce TBD.
					if len(sc.Expected.TimeConfidence) > 0 && sc.Expected.TimeConfidence[0] == "UNKNOWN" {
						t.Errorf("Expected TBD time for UNKNOWN confidence, got %T", resp.Msg.SuggestedTime.TimeType)
					}
				}
			}

			// Validate description is populated.
			if sc.Expected.HasDescription && resp.Msg.Description == "" {
				t.Error("Expected non-empty description")
			}
		})
	}
}

// TestGenExperience_WebpageTimeParsing tests the full GenExperience pipeline for
// webpage-based experience creation.
func TestGenExperience_WebpageTimeParsing(t *testing.T) {
	scenarios := loadGoldenScenarios(t)

	for _, sc := range scenarios.WebpageScenarios {
		t.Run(sc.ID, func(t *testing.T) {
			if sc.Expected == nil {
				t.Skip("No expected values defined")
			}

			// Build mock response.
			mockResp := &ai.ExperienceGeneration{
				Title:          "Test " + sc.Name,
				Description:    "Test description for " + sc.Name,
				Date:           sc.Expected.ExpectedDate,
				Time:           sc.Expected.ExpectedTime,
				TimeConfidence: sc.Expected.TimeConfidence[0],
				LocationQuery:  "",
				Confidence:     0.85,
				SearchKeywords: []string{"test"},
				ValueEstimate: &ai.ValueEstimate{
					EstimatedValueUSD: 50.0,
					Confidence:        0.8,
					Reasoning:         "Test value",
				},
			}

			if len(sc.Expected.LocationContains) > 0 {
				mockResp.LocationQuery = sc.Expected.LocationContains[0]
			}

			service, _, _ := setupTestService(t)
			mock := ai.NewMockProvider()
			mock.GenerateExperienceFromWebpageFunc = func(_ context.Context, _, _, _, _, _ string) (*ai.ExperienceGeneration, error) {
				return mockResp, nil
			}
			service.aiProvider = mock

			// Setup mock web fetcher.
			mockFetcher := &webfetch.MockFetcher{
				FetchPageContentFunc: func(_ context.Context, _ string) (*webfetch.PageContent, error) {
					return &webfetch.PageContent{
						URL:         "https://example.com/event",
						Title:       sc.PageTitle,
						Description: sc.PageDesc,
						BodyText:    sc.PageBody,
					}, nil
				},
			}
			service.SetWebFetcher(mockFetcher)

			ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
			req := connect.NewRequest(&api.GenExperienceRequest{
				Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://example.com/event"},
				CurrentTimeUnixSec: parseCurrentTimeUnix(t, sc.CurrentTime),
				Timezone:           sc.Timezone,
			})

			resp, err := service.GenExperience(ctx, req)
			if err != nil {
				t.Fatalf("GenExperience failed: %v", err)
			}

			// Validate time confidence.
			validateTimeConfidence(t, resp.Msg.TimeConfidence, sc.Expected.TimeConfidence)

			// Validate suggested time.
			if sc.Expected.ExpectedDate != "" && sc.Expected.ExpectedTime != "" {
				validateSuggestedTime(t, resp.Msg, sc.Expected, sc.Timezone)
			}

			// Validate description.
			if sc.Expected.HasDescription && resp.Msg.Description == "" {
				t.Error("Expected non-empty description")
			}

			// Validate source URL.
			if resp.Msg.SourceUrl != "https://example.com/event" {
				t.Errorf("Expected source_url to be echoed, got %q", resp.Msg.SourceUrl)
			}
		})
	}
}

// TestConvertInformalTime_Parsing tests the ConvertInformalTime RPC pipeline
// with golden scenarios.
func TestConvertInformalTime_Parsing(t *testing.T) {
	scenarios := loadGoldenScenarios(t)

	for _, sc := range scenarios.InformalScenarios {
		t.Run(sc.ID, func(t *testing.T) {
			if sc.Expected == nil {
				t.Skip("No expected values defined")
			}

			service, _, _ := setupTestService(t)

			// Build a mock ParseInformalTime response based on expected values.
			mockJSON := buildMockInformalResponse(t, sc)

			mock := ai.NewMockProvider()
			mock.ParseInformalTimeFunc = func(_ context.Context, _, _, _ string) (string, error) {
				return mockJSON, nil
			}
			service.aiProvider = mock

			ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
			req := connect.NewRequest(&api.ConvertInformalTimeRequest{
				InformalDescription: sc.InformalDescription,
				CurrentTimeUnixSec:  parseCurrentTimeUnix(t, sc.CurrentTime),
				Timezone:            sc.Timezone,
			})

			resp, err := service.ConvertInformalTime(ctx, req)
			if err != nil {
				t.Fatalf("ConvertInformalTime failed: %v", err)
			}

			// Validate time type.
			switch sc.Expected.TimeType {
			case "specific":
				specific, ok := resp.Msg.Time.TimeType.(*api.ExperienceTime_Specific)
				if !ok {
					t.Fatalf("Expected SpecificTime, got %T", resp.Msg.Time.TimeType)
				}

				// Validate timestamp.
				if sc.Expected.ExpectedISO != "" {
					expectedTime, err := time.Parse(time.RFC3339, sc.Expected.ExpectedISO)
					if err != nil {
						t.Fatalf("Failed to parse expected ISO %q: %v", sc.Expected.ExpectedISO, err)
					}
					actualTime := time.Unix(specific.Specific.UnixTimestampSec, 0)
					diff := actualTime.Sub(expectedTime).Abs()
					if diff > time.Hour {
						t.Errorf("Timestamp off by %v: got %s, want %s",
							diff, actualTime.Format(time.RFC3339), expectedTime.Format(time.RFC3339))
					}
				}

				// Validate duration.
				if sc.Expected.Duration > 0 && specific.Specific.DurationMinutes != sc.Expected.Duration {
					t.Errorf("Duration mismatch: got %d, want %d",
						specific.Specific.DurationMinutes, sc.Expected.Duration)
				}

			case "range":
				if _, ok := resp.Msg.Time.TimeType.(*api.ExperienceTime_Range); !ok {
					t.Errorf("Expected Range time, got %T", resp.Msg.Time.TimeType)
				}

			case "tbd":
				if _, ok := resp.Msg.Time.TimeType.(*api.ExperienceTime_Tbd); !ok {
					t.Errorf("Expected TBD time, got %T", resp.Msg.Time.TimeType)
				}
			}

			// Validate confidence.
			validateInformalConfidence(t, resp.Msg.Confidence, sc.Expected.Confidence)
		})
	}
}

// buildMockInformalResponse creates a mock JSON response matching the expected
// values for an informal time scenario.
func buildMockInformalResponse(t *testing.T, sc goldenInformalScenario) string {
	t.Helper()

	switch sc.Expected.TimeType {
	case "specific":
		var unixSec int64
		if sc.Expected.ExpectedISO != "" {
			parsed, err := time.Parse(time.RFC3339, sc.Expected.ExpectedISO)
			if err != nil {
				t.Fatalf("Failed to parse expected ISO: %v", err)
			}
			unixSec = parsed.Unix()
		}

		return jsonMarshal(t, map[string]any{
			"time_type":          "specific",
			"unix_timestamp_sec": unixSec,
			"timezone":           sc.Timezone,
			"duration_minutes":   sc.Expected.Duration,
			"confidence":         sc.Expected.Confidence[0],
		})

	case "range":
		var startSec int64
		if sc.Expected.ExpectedISO != "" {
			parsed, err := time.Parse(time.RFC3339, sc.Expected.ExpectedISO)
			if err != nil {
				t.Fatalf("Failed to parse expected ISO: %v", err)
			}
			startSec = parsed.Unix()
		} else if sc.Expected.ExpectedDate != "" {
			// Parse date with timezone for range start.
			dateStr := sc.Expected.ExpectedDate + "T00:00:00"
			loc, err := time.LoadLocation(sc.Timezone)
			if err != nil {
				t.Fatalf("Failed to load timezone %q: %v", sc.Timezone, err)
			}
			parsed, err := time.ParseInLocation("2006-01-02T15:04:05", dateStr, loc)
			if err != nil {
				t.Fatalf("Failed to parse date: %v", err)
			}
			startSec = parsed.Unix()
		}

		// End is 2 days after start for weekend ranges.
		endSec := startSec + 2*24*60*60

		return jsonMarshal(t, map[string]any{
			"time_type":        "range",
			"start_unix_sec":   startSec,
			"end_unix_sec":     endSec,
			"duration_minutes": 0,
			"description":      sc.InformalDescription,
			"confidence":       sc.Expected.Confidence[0],
		})

	case "tbd":
		return jsonMarshal(t, map[string]any{
			"time_type":  "tbd",
			"confidence": "UNKNOWN",
		})

	default:
		t.Fatalf("Unknown time type %q", sc.Expected.TimeType)
		return ""
	}
}

// jsonMarshal is a test helper that marshals to JSON.
func jsonMarshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	return string(data)
}

// validateTimeConfidence checks that the response time confidence matches one
// of the expected values.
func validateTimeConfidence(t *testing.T, got api.TimeConfidence, expected []string) {
	t.Helper()
	if len(expected) == 0 {
		return
	}

	gotStr := strings.TrimPrefix(got.String(), "TIME_CONFIDENCE_")
	matched := false
	for _, e := range expected {
		if gotStr == e {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("TimeConfidence mismatch: got %s, want one of %v", gotStr, expected)
	}
}

// validateInformalConfidence checks the informal time response confidence.
func validateInformalConfidence(t *testing.T, got api.TimeConfidence, expected []string) {
	t.Helper()
	if len(expected) == 0 {
		return
	}

	gotStr := strings.TrimPrefix(got.String(), "TIME_CONFIDENCE_")
	matched := false
	for _, e := range expected {
		if gotStr == e {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("Confidence mismatch: got %s, want one of %v", gotStr, expected)
	}
}

// validateSuggestedTime validates the suggested time fields on a GenExperience response.
// The timezone parameter is the user's IANA timezone from the scenario.
func validateSuggestedTime(t *testing.T, resp *api.GenExperienceResponse, expected *goldenExpExpectation, timezone string) {
	t.Helper()

	if resp.SuggestedTime == nil {
		t.Fatal("Expected SuggestedTime to be set")
	}

	specific, ok := resp.SuggestedTime.TimeType.(*api.ExperienceTime_Specific)
	if !ok {
		t.Fatalf("Expected SpecificTime, got %T", resp.SuggestedTime.TimeType)
	}

	// Validate the extracted Unix timestamp.
	if resp.ExtractedTimeUnixSec == 0 {
		t.Error("Expected non-zero ExtractedTimeUnixSec")
	}

	// Parse expected date+time in the user's timezone (the server now parses in user tz).
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("Failed to load timezone %q: %v", timezone, err)
	}

	expectedDateTimeStr := expected.ExpectedDate
	if expected.ExpectedTime != "" {
		expectedDateTimeStr += "T" + expected.ExpectedTime + ":00"
	} else {
		expectedDateTimeStr += "T00:00:00"
	}

	expectedParsed, err := time.ParseInLocation("2006-01-02T15:04:05", expectedDateTimeStr, loc)
	if err != nil {
		t.Fatalf("Failed to parse expected datetime %q: %v", expectedDateTimeStr, err)
	}

	if specific.Specific.UnixTimestampSec != expectedParsed.Unix() {
		t.Errorf("UnixTimestampSec mismatch: got %d (%s), want %d (%s)",
			specific.Specific.UnixTimestampSec,
			time.Unix(specific.Specific.UnixTimestampSec, 0).In(loc).Format(time.RFC3339),
			expectedParsed.Unix(),
			expectedParsed.Format(time.RFC3339))
	}

	// Verify timezone is set to the user's timezone.
	if specific.Specific.Timezone != timezone {
		t.Errorf("Timezone mismatch: got %q, want %q", specific.Specific.Timezone, timezone)
	}
}
