//go:build integration

// End-to-end happy-path tests for UnifiedCreateService.StreamGenUnifiedCreate.
// Drive the handler through a real connect-go server (httptest) so the
// wire-format encoding + streaming envelope are exercised the same way
// production clients hit them. Uses ai.MockProvider for the AI surface
// and a mockClassifier for the classifier — no real LLM credentials
// required. Tagged //go:build integration to mirror the wider eval /
// integration test convention.

package unified_create

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// authedClient returns a connect-go client + base URL backed by an
// httptest server hosting the unified-create service. authedCtx
// injects the test user into ctx so RequireAuth in the handler sees a
// valid user.
func newIntegrationServer(t *testing.T, classifier Classifier) apiconnect.UnifiedCreateServiceClient {
	t.Helper()
	svc := New(classifier, ai.NewMockProvider(), nil, nil, nil, fakeExperienceGen{}, fakeGearGen{}, fakeRequestGen{})
	mux := http.NewServeMux()
	path, h := apiconnect.NewUnifiedCreateServiceHandler(svc)
	// Inject auth via a tiny wrapper handler: each request gets a
	// pre-populated auth.Info in its context, matching what
	// authMiddleware does in production.
	mux.Handle(path, withAuth(h))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return apiconnect.NewUnifiedCreateServiceClient(http.DefaultClient, srv.URL)
}

func withAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authn.SetInfo(r.Context(), &auth.Info{
			UserID: "test-user",
			Email:  "test@example.com",
		})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// collect drains a streaming response, returning every received event
// (or the first transport-level error).
func collect(t *testing.T, stream *connect.ServerStreamForClient[api.StreamGenUnifiedCreateResponse]) []*api.StreamGenUnifiedCreateResponse {
	t.Helper()
	defer stream.Close()
	var out []*api.StreamGenUnifiedCreateResponse
	for stream.Receive() {
		out = append(out, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream receive: %v", err)
	}
	return out
}

func TestIntegration_HappyPath_Event(t *testing.T) {
	client := newIntegrationServer(t, &mockClassifier{
		result: &ai.UnifiedCreateClassification{
			Type: ai.UnifiedCreateContentTypeEvent,
		},
	})

	stream, err := client.StreamGenUnifiedCreate(
		context.Background(),
		connect.NewRequest(&api.StreamGenUnifiedCreateRequest{
			Prompt: &api.StreamGenUnifiedCreateRequest_Text{
				Text: "Hike at Mt Sanitas tomorrow morning",
			},
		}),
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	events := collect(t, stream)
	assertStreamShape(t, events, api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT)
}

func TestIntegration_HappyPath_Gear(t *testing.T) {
	client := newIntegrationServer(t, &mockClassifier{
		result: &ai.UnifiedCreateClassification{
			Type: ai.UnifiedCreateContentTypeGear,
		},
	})

	stream, err := client.StreamGenUnifiedCreate(
		context.Background(),
		connect.NewRequest(&api.StreamGenUnifiedCreateRequest{
			Prompt: &api.StreamGenUnifiedCreateRequest_Text{
				Text: "Climbing rope 9.6mm",
			},
		}),
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	events := collect(t, stream)
	assertStreamShape(t, events, api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR)
}

func TestIntegration_HappyPath_Request(t *testing.T) {
	client := newIntegrationServer(t, &mockClassifier{
		result: &ai.UnifiedCreateClassification{
			Type: ai.UnifiedCreateContentTypeRequest,
		},
	})

	stream, err := client.StreamGenUnifiedCreate(
		context.Background(),
		connect.NewRequest(&api.StreamGenUnifiedCreateRequest{
			Prompt: &api.StreamGenUnifiedCreateRequest_Text{
				Text: "Looking for someone with a hammer drill",
			},
		}),
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	events := collect(t, stream)
	assertStreamShape(t, events, api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST)
}

func TestIntegration_ForceType_BypassesClassifier(t *testing.T) {
	mock := &mockClassifier{
		result: &ai.UnifiedCreateClassification{
			Type: ai.UnifiedCreateContentTypeRequest,
		},
	}
	client := newIntegrationServer(t, mock)

	ft := api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT
	stream, err := client.StreamGenUnifiedCreate(
		context.Background(),
		connect.NewRequest(&api.StreamGenUnifiedCreateRequest{
			Prompt: &api.StreamGenUnifiedCreateRequest_Text{
				Text: "Doesn't matter — force_type wins",
			},
			ForceType: &ft,
		}),
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	events := collect(t, stream)
	if mock.called {
		t.Error("classifier should NOT be called when force_type is set")
	}
	assertStreamShape(t, events, api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT)
}

// assertStreamShape verifies the minimum contract every stream must
// satisfy:
//   - Exactly one `type` event matching `want`.
//   - Exactly one terminal event (final OR error).
//   - The terminal final's type field matches `want`.
//   - At least one description event (mirroring the user's prompt
//     verbatim per the text-mode convention).
func assertStreamShape(t *testing.T, events []*api.StreamGenUnifiedCreateResponse, want api.DetectedContentType) {
	t.Helper()
	var (
		typeCount, finalCount, errorCount int
		sawDescription                    bool
	)
	for _, ev := range events {
		switch e := ev.Event.(type) {
		case *api.StreamGenUnifiedCreateResponse_Type:
			typeCount++
			if e.Type != want {
				t.Errorf("type event mismatch: got %v want %v", e.Type, want)
			}
		case *api.StreamGenUnifiedCreateResponse_Description:
			sawDescription = true
		case *api.StreamGenUnifiedCreateResponse_Final:
			finalCount++
			if e.Final.Type != want {
				t.Errorf("final type mismatch: got %v want %v", e.Final.Type, want)
			}
		case *api.StreamGenUnifiedCreateResponse_Error:
			errorCount++
			t.Errorf("unexpected error event: %v", e.Error.Message)
		}
	}
	if typeCount != 1 {
		t.Errorf("expected exactly 1 type event, got %d", typeCount)
	}
	if finalCount != 1 {
		t.Errorf("expected exactly 1 final event, got %d", finalCount)
	}
	if errorCount != 0 {
		t.Errorf("expected 0 error events, got %d", errorCount)
	}
	if !sawDescription {
		t.Error("expected at least one description event (description = user prompt)")
	}
}
