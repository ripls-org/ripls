package experience

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// canonical mock JSON for a specific time. The handler converts this to
// an ExperienceTime with TimeType_Specific so the test can assert on the
// returned candidates.
func canonicalSpecificJSON(unixSec int64) string {
	return fmt.Sprintf(
		`{"time_type":"specific","unix_timestamp_sec":%d,"timezone":"UTC","duration_minutes":60,"confidence":"EXPLICIT"}`,
		unixSec,
	)
}

func TestService_ExtractTimeCandidates_MultipleSegments(t *testing.T) {
	service, _, _ := setupTestService(t)

	// Per-segment AI mock: each segment string maps to a distinct unix
	// second so the handler returns three distinct candidates.
	mock := ai.NewMockProvider()
	mock.ParseInformalTimeFunc = func(
		_ context.Context, description, _, _ string,
	) (string, error) {
		lower := strings.ToLower(strings.TrimSpace(description))
		switch lower {
		case "tonight":
			return canonicalSpecificJSON(1_735_780_000), nil
		case "tomorrow night":
			return canonicalSpecificJSON(1_735_866_400), nil
		case "monday afternoon":
			return canonicalSpecificJSON(1_736_280_000), nil
		default:
			// TBD shape — handler should drop these.
			return `{"time_type":"tbd","confidence":"UNKNOWN"}`, nil
		}
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user123", "user@example.com", models.Role_ROLE_USER)

	resp, err := service.ExtractTimeCandidates(ctx,
		connect.NewRequest(&api.ExtractTimeCandidatesRequest{
			Text:               "tonight, tomorrow night, or Monday afternoon",
			CurrentTimeUnixSec: 1_735_700_000,
			Timezone:           "UTC",
		}))
	if err != nil {
		t.Fatalf("ExtractTimeCandidates failed: %v", err)
	}

	if len(resp.Msg.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(resp.Msg.Candidates))
	}

	seen := make(map[int64]struct{})
	for _, c := range resp.Msg.Candidates {
		specific := c.GetSpecific()
		if specific == nil {
			t.Errorf("expected Specific candidate, got %v", c.TimeType)
			continue
		}
		seen[specific.UnixTimestampSec] = struct{}{}
	}
	for _, want := range []int64{1_735_780_000, 1_735_866_400, 1_736_280_000} {
		if _, ok := seen[want]; !ok {
			t.Errorf("missing expected candidate at %d", want)
		}
	}
}

func TestService_ExtractTimeCandidates_DedupesIdenticalInstants(t *testing.T) {
	service, _, _ := setupTestService(t)

	// Both segments parse to the same instant. Handler should collapse
	// them into one candidate.
	mock := ai.NewMockProvider()
	mock.ParseInformalTimeFunc = func(
		_ context.Context, _, _, _ string,
	) (string, error) {
		return canonicalSpecificJSON(1_735_780_000), nil
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user123", "user@example.com", models.Role_ROLE_USER)

	resp, err := service.ExtractTimeCandidates(ctx,
		connect.NewRequest(&api.ExtractTimeCandidatesRequest{
			Text:     "tonight, tonight again",
			Timezone: "UTC",
		}))
	if err != nil {
		t.Fatalf("ExtractTimeCandidates failed: %v", err)
	}
	if len(resp.Msg.Candidates) != 1 {
		t.Fatalf("expected dedupe to collapse to 1 candidate, got %d",
			len(resp.Msg.Candidates))
	}
}

func TestService_ExtractTimeCandidates_DropsTBDSegments(t *testing.T) {
	service, _, _ := setupTestService(t)

	// Mock returns TBD for every segment. Handler must return an empty
	// candidate list — TBD isn't a stageable concrete time.
	mock := ai.NewMockProvider()
	mock.ParseInformalTimeFunc = func(
		_ context.Context, _, _, _ string,
	) (string, error) {
		return `{"time_type":"tbd","confidence":"UNKNOWN"}`, nil
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user123", "user@example.com", models.Role_ROLE_USER)

	resp, err := service.ExtractTimeCandidates(ctx,
		connect.NewRequest(&api.ExtractTimeCandidatesRequest{
			Text:     "sometime, eventually, maybe",
			Timezone: "UTC",
		}))
	if err != nil {
		t.Fatalf("ExtractTimeCandidates failed: %v", err)
	}
	if len(resp.Msg.Candidates) != 0 {
		t.Errorf("expected 0 candidates from all-TBD response, got %d",
			len(resp.Msg.Candidates))
	}
}

func TestService_ExtractTimeCandidates_EmptyTextRejected(t *testing.T) {
	service, _, _ := setupTestService(t)
	ctx := createAuthenticatedContext("user123", "user@example.com", models.Role_ROLE_USER)

	_, err := service.ExtractTimeCandidates(ctx,
		connect.NewRequest(&api.ExtractTimeCandidatesRequest{Text: "   "}))
	if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("expected InvalidArgument for empty text, got %v", err)
	}
}

func TestService_ExtractTimeCandidates_NoAIProviderReturnsEmpty(t *testing.T) {
	service, _, _ := setupTestService(t)
	// Explicitly nil — degraded mode should return empty rather than error.
	service.aiProvider = nil
	ctx := createAuthenticatedContext("user123", "user@example.com", models.Role_ROLE_USER)

	resp, err := service.ExtractTimeCandidates(ctx,
		connect.NewRequest(&api.ExtractTimeCandidatesRequest{
			Text:     "tonight, tomorrow night",
			Timezone: "UTC",
		}))
	if err != nil {
		t.Fatalf("expected nil error when AI provider missing, got %v", err)
	}
	if len(resp.Msg.Candidates) != 0 {
		t.Errorf("expected empty candidates without AI provider, got %d",
			len(resp.Msg.Candidates))
	}
}
