// Unit tests for StreamGenRequest.

package request

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

type captureRequestTarget struct {
	mu     sync.Mutex
	events []*api.StreamGenRequestResponse
}

func (c *captureRequestTarget) Send(ev *api.StreamGenRequestResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *captureRequestTarget) snapshot() []*api.StreamGenRequestResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*api.StreamGenRequestResponse, len(c.events))
	copy(out, c.events)
	return out
}

func scriptedRequestStream(events []ai.FieldEvent, final ai.RequestStreamFinal) func(context.Context, string, string) (<-chan ai.FieldEvent, <-chan ai.RequestStreamFinal, error) {
	return func(_ context.Context, _, _ string) (<-chan ai.FieldEvent, <-chan ai.RequestStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.RequestStreamFinal, 1)
		go func() {
			for _, ev := range events {
				fields <- ev
			}
			close(fields)
			finalCh <- final
			close(finalCh)
		}()
		return fields, finalCh, nil
	}
}

// scriptedRequestImageStream is the image-mode analogue of
// scriptedRequestStream — same shape, different Func signature
// (DetectionImage instead of prompt).
func scriptedRequestImageStream(events []ai.FieldEvent, final ai.RequestStreamFinal) func(context.Context, *ai.DetectionImage, string) (<-chan ai.FieldEvent, <-chan ai.RequestStreamFinal, error) {
	return func(_ context.Context, _ *ai.DetectionImage, _ string) (<-chan ai.FieldEvent, <-chan ai.RequestStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.RequestStreamFinal, 1)
		go func() {
			for _, ev := range events {
				fields <- ev
			}
			close(fields)
			finalCh <- final
			close(finalCh)
		}()
		return fields, finalCh, nil
	}
}

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestStreamGenRequest_EventOrdering verifies description (immediate) →
// title → final ordering, no geocoded event for USER_PRIMARY_LOCATION, no
// error event.
func TestStreamGenRequest_EventOrdering(t *testing.T) {
	service, _, _, _, _ := setupTestServiceWithNotifications(t)

	mock := ai.NewMockProvider()
	mock.GenerateRequestContentStreamingFunc = scriptedRequestStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Borrow Drill")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.RequestStreamFinal{Result: &ai.RequestGeneration{
			Title:         "Borrow Drill",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.9,
		}},
	)
	service.aiProvider = mock

	target := &captureRequestTarget{}
	sender := newRequestEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenRequestFromText(ctx, logger, time.Now(), GenRequestContext{UserID: "user123"}, "borrow drill", sender); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected ≥3 events; got %d", len(events))
	}

	if desc, ok := events[0].Event.(*api.StreamGenRequestResponse_Description); !ok {
		t.Fatalf("first = %T, want Description", events[0].Event)
	} else if desc.Description != "borrow drill" {
		t.Errorf("first.Description = %q", desc.Description)
	}
	if title, ok := events[1].Event.(*api.StreamGenRequestResponse_Title); !ok {
		t.Fatalf("second = %T, want Title", events[1].Event)
	} else if title.Title != "Borrow Drill" {
		t.Errorf("second.Title = %q", title.Title)
	}
	if _, ok := events[len(events)-1].Event.(*api.StreamGenRequestResponse_Final); !ok {
		t.Fatalf("last = %T, want Final", events[len(events)-1].Event)
	}
	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenRequestResponse_Error); isErr {
			t.Errorf("unexpected error event")
		}
		if _, isGeo := e.Event.(*api.StreamGenRequestResponse_Geocoded); isGeo {
			t.Errorf("unexpected geocoded for USER_PRIMARY_LOCATION")
		}
	}
}

// TestStreamGenRequest_AIError surfaces a terminal error event on AI failure.
func TestStreamGenRequest_AIError(t *testing.T) {
	service, _, _, _, _ := setupTestServiceWithNotifications(t)

	mock := ai.NewMockProvider()
	mock.GenerateRequestContentStreamingFunc = scriptedRequestStream(
		nil,
		ai.RequestStreamFinal{Err: errors.New("boom")},
	)
	service.aiProvider = mock

	target := &captureRequestTarget{}
	sender := newRequestEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenRequestFromText(ctx, logger, time.Now(), GenRequestContext{UserID: "user123"}, "broken", sender); err != nil {
		t.Fatalf("stream returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenRequestResponse_Error)
	if !ok {
		t.Fatalf("last = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v", errEv.Error.Code)
	}
}

// seedMediaForUser creates a Media row and a corresponding bucket entry so
// streamGenRequestFromImage can resolve a presigned URL against the test bucket.
func seedMediaForUser(t *testing.T, sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage, userID string) string {
	t.Helper()
	media := &models.Media{UserId: userID, ContentType: "image/jpeg"}
	mediaID, err := sqlStorage.Insert(context.Background(), media)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	bucketKey := storage.MediaBucketKey(userID, mediaID)
	if _, err := bucket.Put(context.Background(), bucketKey, []byte("fake jpeg"), "image/jpeg", map[string]string{"userId": userID}); err != nil {
		t.Fatalf("put bucket: %v", err)
	}
	return mediaID
}

// TestStreamGenRequestFromImage_EventOrdering asserts title arrives before
// final, USER_PRIMARY_LOCATION skips Mapbox, and the input media_id is
// included in the final response media list.
func TestStreamGenRequestFromImage_EventOrdering(t *testing.T) {
	service, sqlStorage, _, _, _ := setupTestServiceWithNotifications(t)
	mediaID := seedMediaForUser(t, sqlStorage, service.bucket, "user123")

	mock := ai.NewMockProvider()
	mock.GenerateRequestFromImageStreamingFunc = scriptedRequestImageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Borrow Drill")},
			{Key: ai.StreamFieldDescription, Value: rawJSON(t, "Cordless 18V drill, weekend project")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.RequestStreamFinal{Result: &ai.RequestGeneration{
			Title:         "Borrow Drill",
			Description:   "Cordless 18V drill, weekend project",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.9,
		}},
	)
	service.aiProvider = mock

	target := &captureRequestTarget{}
	sender := newRequestEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenRequestFromImage(ctx, logger, time.Now(), GenRequestContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 2 {
		t.Fatalf("expected ≥2 events (title, final); got %d", len(events))
	}

	if title, ok := events[0].Event.(*api.StreamGenRequestResponse_Title); !ok {
		t.Fatalf("first = %T, want Title", events[0].Event)
	} else if title.Title != "Borrow Drill" {
		t.Errorf("first.Title = %q", title.Title)
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenRequestResponse_Final)
	if !ok {
		t.Fatalf("last = %T, want Final", last.Event)
	}
	// Image mode preserves the AI-generated description (text mode replaces
	// it with the user's prompt).
	if finalEv.Final.Description != "Cordless 18V drill, weekend project" {
		t.Errorf("final.Description = %q, want AI-generated description preserved", finalEv.Final.Description)
	}
	// Image mode includes the input media_id in the final response.
	foundInputMedia := false
	for _, mid := range finalEv.Final.MediaIds {
		if mid == mediaID {
			foundInputMedia = true
			break
		}
	}
	if !foundInputMedia {
		t.Errorf("final.MediaIds = %v, missing input media_id %q", finalEv.Final.MediaIds, mediaID)
	}

	// Image mode emits the AI-generated description mid-stream so the
	// preview modal's description skeleton can fill before final.
	descriptionEmitted := false
	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenRequestResponse_Error); isErr {
			t.Errorf("unexpected error event")
		}
		if _, isGeo := e.Event.(*api.StreamGenRequestResponse_Geocoded); isGeo {
			t.Errorf("unexpected geocoded for USER_PRIMARY_LOCATION")
		}
		if desc, ok := e.Event.(*api.StreamGenRequestResponse_Description); ok {
			descriptionEmitted = true
			if desc.Description != "Cordless 18V drill, weekend project" {
				t.Errorf("description event = %q, want AI-generated text", desc.Description)
			}
		}
	}
	if !descriptionEmitted {
		t.Error("expected mid-stream description event in image mode; got none")
	}
}

// TestStreamGenRequestFromImage_AIError surfaces a terminal error event on
// AI failure during an image-mode call.
func TestStreamGenRequestFromImage_AIError(t *testing.T) {
	service, sqlStorage, _, _, _ := setupTestServiceWithNotifications(t)
	mediaID := seedMediaForUser(t, sqlStorage, service.bucket, "user123")

	mock := ai.NewMockProvider()
	mock.GenerateRequestFromImageStreamingFunc = scriptedRequestImageStream(
		nil,
		ai.RequestStreamFinal{Err: errors.New("boom")},
	)
	service.aiProvider = mock

	target := &captureRequestTarget{}
	sender := newRequestEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenRequestFromImage(ctx, logger, time.Now(), GenRequestContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("stream returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenRequestResponse_Error)
	if !ok {
		t.Fatalf("last = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v", errEv.Error.Code)
	}
}
