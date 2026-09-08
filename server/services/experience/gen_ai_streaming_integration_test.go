//go:build integration

// Live-AI integration test for StreamGenExperience. Drives the streaming
// core against real Anthropic streaming, captures emitted events, and
// verifies the canonical event sequence (title → final, optional geocoded
// / media_ready in between). Gated by the `integration` build tag so it
// does not run in the default test suite.

package experience

import (
	"context"
	"os"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/ai/aitest"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// streamGenCallTimeout bounds the live-provider streaming call. Same reasoning
// as genExperienceCallTimeout: the go-test deadline is per test binary, so an
// unbounded stalled stream fails the whole package instead of one test. A
// stream is more exposed than a unary call — it can stall mid-response after a
// clean handshake — so bounding it matters more, not less.
const streamGenCallTimeout = 90 * time.Second

func TestStreamGenExperience_RealAnthropic(t *testing.T) {
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		t.Skip("Skipping: ANTHROPIC_API_KEY not set")
	}

	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	provider, err := ai.NewAnthropicProvider(anthropicKey, "claude-haiku-4-5", ai.DefaultTemperature)
	if err != nil {
		t.Fatalf("new anthropic provider: %v", err)
	}
	service.SetAIProvider(provider)

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	msg := &api.StreamGenExperienceRequest{
		Prompt:             &api.StreamGenExperienceRequest_Text{Text: "hiking tomorrow morning at Chautauqua"},
		CurrentTimeUnixSec: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC).Unix(),
		LatitudeDeg:        40.015,
		LongitudeDeg:       -105.27,
	}
	logger := logging.LoggerWithContext(ctx)
	args := GenExperienceContext{
		UserID:             "user123",
		CurrentTimeUnixSec: msg.GetCurrentTimeUnixSec(),
		LatitudeDeg:        msg.GetLatitudeDeg(),
		LongitudeDeg:       msg.GetLongitudeDeg(),
	}

	bounded, cancel := context.WithTimeout(ctx, streamGenCallTimeout)
	defer cancel()

	if err := service.StreamGenExperienceFromText(bounded, logger, time.Now(), args, msg.GetText(), sender); err != nil {
		// A stalled stream surfaces as "context deadline exceeded", which
		// errs.IsTransient matches — so it skips rather than failing.
		if aitest.IsTransientError(err) {
			t.Skipf("Anthropic transient failure, skipping: %v", err)
		}
		t.Fatalf("StreamGenExperienceFromText: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected ≥3 events (description, title, final); got %d", len(events))
	}

	// First emitted event should be description (fires immediately, no AI wait).
	if desc, ok := events[0].Event.(*api.StreamGenExperienceResponse_Description); !ok {
		t.Errorf("first event = %T, want Description", events[0].Event)
	} else {
		t.Logf("description: %q", desc.Description)
	}

	// Title should be the next non-terminal event in the sequence.
	titleSeen := false
	for _, e := range events[1:] {
		if t2, ok := e.Event.(*api.StreamGenExperienceResponse_Title); ok {
			titleSeen = true
			t.Logf("title: %q", t2.Title)
			break
		}
	}
	if !titleSeen {
		t.Errorf("no title event observed in stream")
	}

	// Last event should be the terminal Final (no error on this prompt).
	last, ok := events[len(events)-1].Event.(*api.StreamGenExperienceResponse_Final)
	if !ok {
		t.Fatalf("last event = %T, want Final; all events: %+v", events[len(events)-1].Event, events)
	}
	if last.Final.Name == "" {
		t.Errorf("final.Name is empty")
	}

	// Log the observed sequence for debuggability.
	for i, e := range events {
		switch v := e.Event.(type) {
		case *api.StreamGenExperienceResponse_Description:
			t.Logf("  %d: description = %q", i, v.Description)
		case *api.StreamGenExperienceResponse_Title:
			t.Logf("  %d: title = %q", i, v.Title)
		case *api.StreamGenExperienceResponse_Geocoded:
			t.Logf("  %d: geocoded = %s", i, v.Geocoded.Name)
		case *api.StreamGenExperienceResponse_MediaReady:
			t.Logf("  %d: media_ready = %v", i, v.MediaReady.MediaIds)
		case *api.StreamGenExperienceResponse_Time:
			t.Logf("  %d: time (confidence=%v)", i, v.Time.Confidence)
		case *api.StreamGenExperienceResponse_Final:
			t.Logf("  %d: final (name=%q)", i, v.Final.Name)
		case *api.StreamGenExperienceResponse_Error:
			t.Errorf("  %d: error = %s (code=%v)", i, v.Error.Message, v.Error.Code)
		}
	}
}
