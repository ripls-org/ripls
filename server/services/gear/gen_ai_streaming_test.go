// Unit tests for StreamGenGear.

package gear

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/webfetch"
)

type captureGearTarget struct {
	mu     sync.Mutex
	events []*api.StreamGenGearResponse
}

func (c *captureGearTarget) Send(ev *api.StreamGenGearResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *captureGearTarget) snapshot() []*api.StreamGenGearResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*api.StreamGenGearResponse, len(c.events))
	copy(out, c.events)
	return out
}

func scriptedGearStream(events []ai.FieldEvent, final ai.GearStreamFinal) func(context.Context, string, string) (<-chan ai.FieldEvent, <-chan ai.GearStreamFinal, error) {
	return func(_ context.Context, _, _ string) (<-chan ai.FieldEvent, <-chan ai.GearStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.GearStreamFinal, 1)
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

func gearTestService(t *testing.T, mock *ai.MockProvider) *Service {
	t.Helper()
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(mock)
	return service
}

// TestStreamGenGear_EventOrdering verifies title → final ordering. Gear
// fires Pexels off title, but with no stockImageryProvider configured the
// media branch is skipped — so this test asserts the plain title/final
// sequence with no geocoded event (no mapboxClient).
func TestStreamGenGear_EventOrdering(t *testing.T) {
	mock := ai.NewMockProvider()
	mock.GenerateGearFromTextStreamingFunc = scriptedGearStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "Coleman Cooler")},
			{Key: ai.StreamFieldLocationQuery, Value: rawJSON(t, "USER_PRIMARY_LOCATION")},
		},
		ai.GearStreamFinal{Result: &ai.GearGeneration{
			Title:         "Coleman Cooler",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.9,
		}},
	)
	service := gearTestService(t, mock)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenGearRequest{Prompt: "coleman cooler"}

	if err := service.StreamGenGearFromText(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, msg.Prompt, sender); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 3 {
		t.Fatalf("expected ≥3 events; got %d", len(events))
	}

	// Text mode: server emits the user's prompt as description before
	// opening the AI stream because gearFromTextOutput has no description
	// field (see schemas.go). Without this, no Description event ever
	// reaches the client and downstream gates that require description
	// (e.g. unified-create Share) stay closed.
	if first, ok := events[0].Event.(*api.StreamGenGearResponse_Description); !ok {
		t.Fatalf("first = %T, want Description (text-mode prompt-as-description)", events[0].Event)
	} else if first.Description != "coleman cooler" {
		t.Errorf("first.Description = %q, want prompt verbatim", first.Description)
	}
	if second, ok := events[1].Event.(*api.StreamGenGearResponse_Title); !ok {
		t.Fatalf("second = %T, want Title", events[1].Event)
	} else if second.Title != "Coleman Cooler" {
		t.Errorf("second.Title = %q", second.Title)
	}
	if _, ok := events[len(events)-1].Event.(*api.StreamGenGearResponse_Final); !ok {
		t.Fatalf("last = %T, want Final", events[len(events)-1].Event)
	}
	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenGearResponse_Error); isErr {
			t.Errorf("unexpected error event")
		}
	}
}

// TestStreamGenGear_AIError surfaces a terminal error event on AI failure.
func TestStreamGenGear_AIError(t *testing.T) {
	mock := ai.NewMockProvider()
	mock.GenerateGearFromTextStreamingFunc = scriptedGearStream(
		nil,
		ai.GearStreamFinal{Err: errors.New("kaput")},
	)
	service := gearTestService(t, mock)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	msg := &api.StreamGenGearRequest{Prompt: "broken"}

	if err := service.StreamGenGearFromText(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, msg.Prompt, sender); err != nil {
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
	errEv, ok := last.Event.(*api.StreamGenGearResponse_Error)
	if !ok {
		t.Fatalf("last = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v", errEv.Error.Code)
	}
}

// scriptedGearDetectionImageStream builds a mock-provider Func that emits a
// caller-specified sequence of FieldEvents followed by a terminal
// GearDetectionStreamFinal for image-mode gear detection.
func scriptedGearDetectionImageStream(events []ai.FieldEvent, final ai.GearDetectionStreamFinal) func(context.Context, *ai.DetectionImage) (<-chan ai.FieldEvent, <-chan ai.GearDetectionStreamFinal, error) {
	return func(_ context.Context, _ *ai.DetectionImage) (<-chan ai.FieldEvent, <-chan ai.GearDetectionStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.GearDetectionStreamFinal, 1)
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

// gearTestServiceWithBucket is like gearTestService but exposes the test
// bucket so image-mode tests can seed media bytes that GetSignedURL can
// resolve against.
func gearTestServiceWithBucket(t *testing.T, mock *ai.MockProvider) (*Service, *storage.ProtoSQLStorage, *services.MockBucketStorage) {
	t.Helper()
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(mock)
	return service, testStorage, mockBucket
}

// seedMediaForUser inserts a Media row owned by the given user so the
// image-mode dispatcher can resolve a presigned URL.
func seedMediaForUser(t *testing.T, sqlStorage *storage.ProtoSQLStorage, userID string) string {
	t.Helper()
	media := &models.Media{UserId: userID, ContentType: "image/jpeg"}
	mediaID, err := sqlStorage.Insert(context.Background(), media)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	return mediaID
}

// TestStreamGenGearFromImage_EventOrdering asserts title arrives before
// final and that the final response includes the input media_id.
func TestStreamGenGearFromImage_EventOrdering(t *testing.T) {
	mock := ai.NewMockProvider()
	service, sqlStorage, _ := gearTestServiceWithBucket(t, mock)
	mediaID := seedMediaForUser(t, sqlStorage, "user123")

	mock.DetectGearInImageStreamingFunc = scriptedGearDetectionImageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "DeWalt Drill")},
			{Key: ai.StreamFieldDescription, Value: rawJSON(t, "Cordless 18V drill driver")},
		},
		ai.GearDetectionStreamFinal{Result: &ai.GearDetection{
			Title:       "DeWalt Drill",
			Description: "Cordless 18V drill driver",
			Brand:       "DeWalt",
			Category:    "tools",
			Confidence:  0.92,
		}},
	)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	if err := service.StreamGenGearFromImage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("StreamGenGearFromImage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 2 {
		t.Fatalf("expected ≥2 events (title, final); got %d", len(events))
	}

	if title, ok := events[0].Event.(*api.StreamGenGearResponse_Title); !ok {
		t.Fatalf("first = %T, want Title", events[0].Event)
	} else if title.Title != "DeWalt Drill" {
		t.Errorf("first.Title = %q", title.Title)
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenGearResponse_Final)
	if !ok {
		t.Fatalf("last = %T, want Final", last.Event)
	}
	if finalEv.Final.DetectedGear == nil {
		t.Fatal("final.DetectedGear is nil")
	}
	if finalEv.Final.DetectedGear.Title != "DeWalt Drill" {
		t.Errorf("final.DetectedGear.Title = %q", finalEv.Final.DetectedGear.Title)
	}
	if finalEv.Final.DetectedGear.Brand != "DeWalt" {
		t.Errorf("final.DetectedGear.Brand = %q", finalEv.Final.DetectedGear.Brand)
	}
	foundInputMedia := false
	for _, mid := range finalEv.Final.DetectedGear.MediaIds {
		if mid == mediaID {
			foundInputMedia = true
			break
		}
	}
	if !foundInputMedia {
		t.Errorf("final.DetectedGear.MediaIds = %v, missing input media_id %q",
			finalEv.Final.DetectedGear.MediaIds, mediaID)
	}

	// Image mode now emits the AI-generated description mid-stream so the
	// preview modal's description skeleton can fill before final.
	descriptionEmitted := false
	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenGearResponse_Error); isErr {
			t.Errorf("unexpected error event: %+v", e)
		}
		if desc, ok := e.Event.(*api.StreamGenGearResponse_Description); ok {
			descriptionEmitted = true
			if desc.Description != "Cordless 18V drill driver" {
				t.Errorf("description event = %q, want AI-generated text", desc.Description)
			}
		}
	}
	if !descriptionEmitted {
		t.Error("expected mid-stream description event in gear-image mode; got none")
	}
}

// TestStreamGenGearFromImage_UnaryProviderEmitsDescription covers the
// unary-wrapper path (no DetectGearInImageStreamingFunc override →
// runUnaryDetectGearInImageStream → emitGearDetectionFromUnary): providers
// without native streaming must still deliver a mid-stream description
// event, or the unified-create preview can never satisfy its Save gate in
// image mode (#2687 — the e2e/dev mock provider is exactly this path).
func TestStreamGenGearFromImage_UnaryProviderEmitsDescription(t *testing.T) {
	mock := ai.NewMockProvider() // default unary DetectGearInImage
	service, sqlStorage, _ := gearTestServiceWithBucket(t, mock)
	mediaID := seedMediaForUser(t, sqlStorage, "user123")

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	if err := service.StreamGenGearFromImage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("StreamGenGearFromImage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	var sawTitle, sawDescription, sawFinal bool
	for _, e := range target.snapshot() {
		switch ev := e.Event.(type) {
		case *api.StreamGenGearResponse_Title:
			sawTitle = ev.Title != ""
		case *api.StreamGenGearResponse_Description:
			sawDescription = ev.Description != ""
		case *api.StreamGenGearResponse_Final:
			sawFinal = true
		case *api.StreamGenGearResponse_Error:
			t.Errorf("unexpected error event: %+v", e)
		}
	}
	if !sawTitle || !sawDescription || !sawFinal {
		t.Errorf("unary image stream: title=%v description=%v final=%v — all must be true",
			sawTitle, sawDescription, sawFinal)
	}
}

// TestStreamGenGearFromImage_AIError surfaces a terminal error event when
// the AI provider fails mid-stream on an image-mode call.
func TestStreamGenGearFromImage_AIError(t *testing.T) {
	mock := ai.NewMockProvider()
	service, sqlStorage, _ := gearTestServiceWithBucket(t, mock)
	mediaID := seedMediaForUser(t, sqlStorage, "user123")

	mock.DetectGearInImageStreamingFunc = scriptedGearDetectionImageStream(
		nil,
		ai.GearDetectionStreamFinal{Err: errors.New("rate limited")},
	)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	if err := service.StreamGenGearFromImage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, mediaID, sender); err != nil {
		t.Fatalf("handler returned non-nil error (should surface as event): %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenGearResponse_Error)
	if !ok {
		t.Fatalf("last = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v", errEv.Error.Code)
	}
}

// scriptedGearWebpageStream is the webpage-mode analogue of scriptedGearStream.
func scriptedGearWebpageStream(events []ai.FieldEvent, final ai.GearStreamFinal) func(context.Context, string, string, string, string) (<-chan ai.FieldEvent, <-chan ai.GearStreamFinal, error) {
	return func(_ context.Context, _, _, _, _ string) (<-chan ai.FieldEvent, <-chan ai.GearStreamFinal, error) {
		fields := make(chan ai.FieldEvent, len(events))
		finalCh := make(chan ai.GearStreamFinal, 1)
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

// TestStreamGenGearFromWebpage_EventOrdering asserts title arrives before
// final, the final response includes the source URL, and the AI-generated
// brand/value-estimate flow through.
func TestStreamGenGearFromWebpage_EventOrdering(t *testing.T) {
	mock := ai.NewMockProvider()
	service := gearTestService(t, mock)
	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(_ context.Context, url string) (*webfetch.PageContent, error) {
			return &webfetch.PageContent{
				URL:         url,
				Title:       "DeWalt 20V Drill",
				Description: "Cordless drill driver",
				BodyText:    "Detailed specs and features",
			}, nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	mock.GenerateGearFromWebpageStreamingFunc = scriptedGearWebpageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "DeWalt 20V Drill Driver")},
		},
		ai.GearStreamFinal{Result: &ai.GearGeneration{
			Title:       "DeWalt 20V Drill Driver",
			Description: "Cordless 20V drill, brushless motor",
			Brand:       "DeWalt",
			Category:    "tools",
			Confidence:  0.95,
			ValueEstimate: &ai.ValueEstimate{
				EstimatedValueUSD: 149.0,
				Confidence:        0.9,
				Reasoning:         "MSRP listed on page",
			},
		}},
	)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	websiteURL := "https://example.com/dewalt-drill"

	if err := service.StreamGenGearFromWebpage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, websiteURL, sender); err != nil {
		t.Fatalf("StreamGenGearFromWebpage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) < 2 {
		t.Fatalf("expected ≥2 events; got %d", len(events))
	}

	if title, ok := events[0].Event.(*api.StreamGenGearResponse_Title); !ok {
		t.Fatalf("first = %T, want Title", events[0].Event)
	} else if title.Title != "DeWalt 20V Drill Driver" {
		t.Errorf("first.Title = %q", title.Title)
	}

	last := events[len(events)-1]
	finalEv, ok := last.Event.(*api.StreamGenGearResponse_Final)
	if !ok {
		t.Fatalf("last = %T, want Final", last.Event)
	}
	if finalEv.Final.DetectedGear == nil {
		t.Fatal("final.DetectedGear is nil")
	}
	if finalEv.Final.DetectedGear.SourceUrl != websiteURL {
		t.Errorf("final.SourceUrl = %q, want %q", finalEv.Final.DetectedGear.SourceUrl, websiteURL)
	}
	if finalEv.Final.DetectedGear.Brand != "DeWalt" {
		t.Errorf("final.Brand = %q", finalEv.Final.DetectedGear.Brand)
	}
	if finalEv.Final.DetectedGear.ValueEstimate == nil {
		t.Error("final.ValueEstimate is nil; expected from AI generation")
	}

	for _, e := range events {
		if _, isErr := e.Event.(*api.StreamGenGearResponse_Error); isErr {
			t.Errorf("unexpected error event: %+v", e)
		}
	}
}

// TestStreamGenGearFromWebpage_EmitsCandidates asserts that when the
// webpage fetcher returns multiple image URLs (the picked one plus
// alternates), the MediaReady event carries the alternates as
// MediaCandidate entries the client can render in Replace Media.
func TestStreamGenGearFromWebpage_EmitsCandidates(t *testing.T) {
	mock := ai.NewMockProvider()
	service := gearTestService(t, mock)

	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff fake jpeg"))
	}))
	defer imgServer.Close()

	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(_ context.Context, url string) (*webfetch.PageContent, error) {
			return &webfetch.PageContent{
				URL:      url,
				Title:    "DeWalt Drill",
				ImageURL: imgServer.URL + "/picked.jpg",
				ImageURLs: []string{
					imgServer.URL + "/picked.jpg",
					imgServer.URL + "/alt1.jpg",
					imgServer.URL + "/alt2.jpg",
					imgServer.URL + "/alt3.jpg",
				},
			}, nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	mock.GenerateGearFromWebpageStreamingFunc = scriptedGearWebpageStream(
		[]ai.FieldEvent{
			{Key: ai.StreamFieldTitle, Value: rawJSON(t, "DeWalt Drill")},
		},
		ai.GearStreamFinal{Result: &ai.GearGeneration{
			Title: "DeWalt Drill", Confidence: 0.9,
		}},
	)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)
	if err := service.StreamGenGearFromWebpage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, "https://example.com/x", sender); err != nil {
		t.Fatalf("StreamGenGearFromWebpage: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	var mediaReady *api.MediaReady
	for _, e := range target.snapshot() {
		if m, ok := e.Event.(*api.StreamGenGearResponse_MediaReady); ok {
			mediaReady = m.MediaReady
		}
	}
	if mediaReady == nil {
		t.Fatal("no MediaReady event emitted")
	}
	if len(mediaReady.Candidates) != 3 {
		t.Errorf("candidates len = %d, want 3", len(mediaReady.Candidates))
	}
	for i, c := range mediaReady.Candidates {
		if c.GetContentType() != "image/jpeg" {
			t.Errorf("candidates[%d].ContentType = %q", i, c.GetContentType())
		}
		if c.GetUrl() == "" {
			t.Errorf("candidates[%d].Url empty", i)
		}
	}
}

// TestStreamGenGearFromWebpage_AIError surfaces a terminal error event
// when the AI provider fails after the webpage was fetched successfully.
func TestStreamGenGearFromWebpage_AIError(t *testing.T) {
	mock := ai.NewMockProvider()
	service := gearTestService(t, mock)
	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(_ context.Context, _ string) (*webfetch.PageContent, error) {
			return &webfetch.PageContent{Title: "x", Description: "y", BodyText: "z"}, nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	mock.GenerateGearFromWebpageStreamingFunc = scriptedGearWebpageStream(
		nil,
		ai.GearStreamFinal{Err: errors.New("boom")},
	)

	target := &captureGearTarget{}
	sender := newGearEventSender(target)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	logger := logging.LoggerWithContext(ctx)

	if err := service.StreamGenGearFromWebpage(ctx, logger, time.Now(), GenGearContext{UserID: "user123"}, "https://example.com/x", sender); err != nil {
		t.Fatalf("stream returned non-nil error: %v", err)
	}
	if cerr := sender.close(); cerr != nil {
		t.Fatalf("sender close: %v", cerr)
	}

	events := target.snapshot()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	last := events[len(events)-1]
	errEv, ok := last.Event.(*api.StreamGenGearResponse_Error)
	if !ok {
		t.Fatalf("last = %T, want Error", last.Event)
	}
	if errEv.Error.Code != api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED {
		t.Errorf("error code = %v", errEv.Error.Code)
	}
}
