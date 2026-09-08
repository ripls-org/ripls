// Unit tests for StreamGenExperience: event ordering (title → final),
// AI-error terminal event, USER_PRIMARY_LOCATION skips Mapbox fan-out.

package experience

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
	"go.ripls.org/ripls/server/webfetch"
)

// captureExperienceTarget collects StreamGenExperienceResponse events for
// assertions. The sender serializes writes via a single goroutine, but the
// mutex still matters because test assertions read concurrently with
// in-flight sender goroutines.
type captureExperienceTarget struct {
	mu     sync.Mutex
	events []*api.StreamGenExperienceResponse
}

func (c *captureExperienceTarget) Send(ev *api.StreamGenExperienceResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *captureExperienceTarget) snapshot() []*api.StreamGenExperienceResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*api.StreamGenExperienceResponse, len(c.events))
	copy(out, c.events)
	return out
}

// scriptedExperienceStream builds a mock-provider Func that emits a caller-
// specified sequence of FieldEvents followed by a terminal ExperienceStreamFinal.
func scriptedExperienceStream(events []ai.FieldEvent, final ai.ExperienceStreamFinal) func(context.Context, string, string, string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
	return func(_ context.Context, _, _, _ string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.ExperienceStreamFinal, 1)
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

// scriptedExperienceImageStream is the image-mode analogue of
// scriptedExperienceStream — same shape, different Func signature
// (DetectionImage + notes/currentTime instead of prompt).
func scriptedExperienceImageStream(events []ai.FieldEvent, final ai.ExperienceStreamFinal) func(context.Context, *ai.DetectionImage, string, string, string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
	return func(_ context.Context, _ *ai.DetectionImage, _, _, _ string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.ExperienceStreamFinal, 1)
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

// TestStreamGenExperience_EventOrdering asserts description arrives first
// (immediate, equals the prompt), title arrives next, final arrives last, no
// error event, and USER_PRIMARY_LOCATION skips Mapbox.
func TestStreamGenExperience_EventOrdering(t *testing.T) {
	service, _, _ := setupTestService(t)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromTextStreamingFunc = scriptedExperienceStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Evening Hike")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.ExperienceStreamFinal{Result: &ai.ExperienceGeneration{
			Title:         "Evening Hike",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.85,
		}},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenExperienceFromText(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, "evening hike", sender); err != nil {
		t.Fatalf("streamGenExperienceFromText: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (description, title, final); got %d", len(events))
	}

	if desc, ok := events[0].Event.(*api.StreamGenExperienceResponse_Description); !ok {
		t.Fatalf("first event = %T, want Description", events[0].Event)
	} else if desc.Description != "evening hike" {
		t.Errorf("first.Description = %q, want %q", desc.Description, "evening hike")
	}
	if title, ok := events[1].Event.(*api.StreamGenExperienceResponse_Title); !ok {
		t.Fatalf("second event = %T, want Title", events[1].Event)
	} else if title.Title != "Evening Hike" {
		t.Errorf("second.Title = %q, want %q", title.Title, "Evening Hike")
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenExperienceResponse_Final)
	if !ok {
		t.Fatalf("last event = %T, want Final", last.Event)
	}
	if finalEv.Final.Name != "Evening Hike" {
		t.Errorf("final.Name = %q, want %q", finalEv.Final.Name, "Evening Hike")
	}

	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenExperienceResponse_Error); isErr {
			t.Errorf("unexpected error event: %+v", e)
		}
		if _, isGeo := e.Event.(*api.StreamGenExperienceResponse_Geocoded); isGeo {
			t.Errorf("unexpected geocoded event (USER_PRIMARY_LOCATION should skip Mapbox): %+v", e)
		}
	}
}

// TestStreamGenExperience_AIError surfaces a terminal error event when the
// AI provider fails mid-stream.
func TestStreamGenExperience_AIError(t *testing.T) {
	service, _, _ := setupTestService(t)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromTextStreamingFunc = scriptedExperienceStream(
		nil,
		ai.ExperienceStreamFinal{Err: errors.New("rate limited")},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenExperienceFromText(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, "anything", sender); err != nil {
		t.Fatalf("handler returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("expected at least one event; got 0")
	}

	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenExperienceResponse_Error)
	if !ok {
		t.Fatalf("last event = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v, want AI_PROVIDER_FAILED", errEv.Error.Code)
	}

	for _, e := range events[:len(events)-1] {
		if _, isFin := e.Event.(*api.StreamGenExperienceResponse_Final); isFin {
			t.Errorf("unexpected final event preceding terminal error: %+v", e)
		}
	}
}

// seedMediaForUser creates a Media row and a corresponding bucket entry so
// that streamGenExperienceFromImage / streamGenRequestFromImage can resolve
// a presigned URL against the test bucket.
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

// TestStreamGenExperienceFromImage_EventOrdering asserts title arrives before
// final, USER_PRIMARY_LOCATION skips Mapbox, and the input media_id is
// included in the final response media list.
func TestStreamGenExperienceFromImage_EventOrdering(t *testing.T) {
	service, sqlStorage, bucket := setupTestService(t)
	mediaID := seedMediaForUser(t, sqlStorage, bucket, "user123")

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromImageStreamingFunc = scriptedExperienceImageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Mountain Vista")},
			{Key: ai.StreamFieldDescription, Value: rawJSON(t, "A breathtaking alpine view")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.ExperienceStreamFinal{Result: &ai.ExperienceGeneration{
			Title:         "Mountain Vista",
			Description:   "A breathtaking alpine view",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.85,
		}},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenExperienceFromImage(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("streamGenExperienceFromImage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (title, final); got %d", len(events))
	}

	if title, ok := events[0].Event.(*api.StreamGenExperienceResponse_Title); !ok {
		t.Fatalf("first event = %T, want Title", events[0].Event)
	} else if title.Title != "Mountain Vista" {
		t.Errorf("first.Title = %q, want %q", title.Title, "Mountain Vista")
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenExperienceResponse_Final)
	if !ok {
		t.Fatalf("last event = %T, want Final", last.Event)
	}
	if finalEv.Final.Name != "Mountain Vista" {
		t.Errorf("final.Name = %q, want %q", finalEv.Final.Name, "Mountain Vista")
	}
	// Image mode preserves the AI-generated description (text mode replaces
	// it with the user's prompt).
	if finalEv.Final.Description != "A breathtaking alpine view" {
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
		if _, isErr := e.Event.(*api.StreamGenExperienceResponse_Error); isErr {
			t.Errorf("unexpected error event: %+v", e)
		}
		if _, isGeo := e.Event.(*api.StreamGenExperienceResponse_Geocoded); isGeo {
			t.Errorf("unexpected geocoded event (USER_PRIMARY_LOCATION should skip Mapbox): %+v", e)
		}
		if desc, ok := e.Event.(*api.StreamGenExperienceResponse_Description); ok {
			descriptionEmitted = true
			if desc.Description != "A breathtaking alpine view" {
				t.Errorf("description event = %q, want AI-generated text", desc.Description)
			}
		}
	}
	if !descriptionEmitted {
		t.Error("expected mid-stream description event in image mode; got none")
	}
}

// TestStreamGenExperienceFromImage_AIError surfaces a terminal error event
// when the AI provider fails mid-stream on an image-mode call.
func TestStreamGenExperienceFromImage_AIError(t *testing.T) {
	service, sqlStorage, bucket := setupTestService(t)
	mediaID := seedMediaForUser(t, sqlStorage, bucket, "user123")

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromImageStreamingFunc = scriptedExperienceImageStream(
		nil,
		ai.ExperienceStreamFinal{Err: errors.New("rate limited")},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenExperienceFromImage(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("handler returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("expected at least one event; got 0")
	}

	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenExperienceResponse_Error)
	if !ok {
		t.Fatalf("last event = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v, want AI_PROVIDER_FAILED", errEv.Error.Code)
	}

	for _, e := range events[:len(events)-1] {
		if _, isFin := e.Event.(*api.StreamGenExperienceResponse_Final); isFin {
			t.Errorf("unexpected final event preceding terminal error: %+v", e)
		}
	}
}

// scriptedExperienceWebpageStream is the webpage-mode analogue of
// scriptedExperienceStream / scriptedExperienceImageStream.
func scriptedExperienceWebpageStream(events []ai.FieldEvent, final ai.ExperienceStreamFinal) func(context.Context, string, string, string, string, string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
	return func(_ context.Context, _, _, _, _, _ string) (<-chan ai.FieldEvent, <-chan ai.ExperienceStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.ExperienceStreamFinal, 1)
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

// TestStreamGenExperienceFromWebpage_EventOrdering asserts title arrives
// before final, USER_PRIMARY_LOCATION skips Mapbox, and the final response
// includes the source URL.
func TestStreamGenExperienceFromWebpage_EventOrdering(t *testing.T) {
	service, _, _ := setupTestService(t)
	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(_ context.Context, url string) (*webfetch.PageContent, error) {
			return &webfetch.PageContent{
				URL:         url,
				Title:       "Sample Event",
				Description: "Description from the page",
				BodyText:    "Body text",
			}, nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromWebpageStreamingFunc = scriptedExperienceWebpageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Mountain Yoga Retreat")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.ExperienceStreamFinal{Result: &ai.ExperienceGeneration{
			Title:         "Mountain Yoga Retreat",
			Description:   "A weekend retreat in the Rockies",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.85,
		}},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	websiteURL := "https://example.com/event"

	if err := service.StreamGenExperienceFromWebpage(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, websiteURL, sender); err != nil {
		t.Fatalf("streamGenExperienceFromWebpage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (title, final); got %d", len(events))
	}

	if title, ok := events[0].Event.(*api.StreamGenExperienceResponse_Title); !ok {
		t.Fatalf("first event = %T, want Title", events[0].Event)
	} else if title.Title != "Mountain Yoga Retreat" {
		t.Errorf("first.Title = %q", title.Title)
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenExperienceResponse_Final)
	if !ok {
		t.Fatalf("last event = %T, want Final", last.Event)
	}
	if finalEv.Final.Name != "Mountain Yoga Retreat" {
		t.Errorf("final.Name = %q", finalEv.Final.Name)
	}
	if finalEv.Final.SourceUrl != websiteURL {
		t.Errorf("final.SourceUrl = %q, want %q", finalEv.Final.SourceUrl, websiteURL)
	}

	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenExperienceResponse_Error); isErr {
			t.Errorf("unexpected error event: %+v", e)
		}
		if _, isGeo := e.Event.(*api.StreamGenExperienceResponse_Geocoded); isGeo {
			t.Errorf("unexpected geocoded event (USER_PRIMARY_LOCATION should skip Mapbox): %+v", e)
		}
	}
}

// TestStreamGenExperienceFromWebpage_AIError surfaces a terminal error
// event when the AI provider fails mid-stream after the webpage was
// fetched successfully.
func TestStreamGenExperienceFromWebpage_AIError(t *testing.T) {
	service, _, _ := setupTestService(t)
	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(_ context.Context, _ string) (*webfetch.PageContent, error) {
			return &webfetch.PageContent{Title: "x", Description: "y", BodyText: "z"}, nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceFromWebpageStreamingFunc = scriptedExperienceWebpageStream(
		nil,
		ai.ExperienceStreamFinal{Err: errors.New("rate limited")},
	)
	service.aiProvider = mock

	target := &captureExperienceTarget{}
	sender := newExperienceEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenExperienceFromWebpage(ctx, logger, time.Now(), GenExperienceContext{UserID: "user123"}, "https://example.com/event", sender); err != nil {
		t.Fatalf("handler returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("expected at least one event; got 0")
	}

	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenExperienceResponse_Error)
	if !ok {
		t.Fatalf("last event = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v, want AI_PROVIDER_FAILED", errEv.Error.Code)
	}
}
