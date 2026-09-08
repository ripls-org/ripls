package unified_create

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/genai/experiencegen"
	"go.ripls.org/ripls/server/genai/geargen"
	"go.ripls.org/ripls/server/genai/requestgen"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// fakeExperienceGen / fakeGearGen / fakeRequestGen are minimal fakes
// that mirror the contract of the real generators (a description event
// upfront followed by a Final) so the unified dispatcher's translation
// can be exercised in isolation — classifier output, type event,
// envelope translation, final stamping.
type fakeExperienceGen struct{}

func (fakeExperienceGen) StreamGenExperienceFromText(_ context.Context, _ *logging.Logger, _ time.Time,
	_ experiencegen.Context, prompt string, sender experiencegen.StreamSender,
) error {
	sender.EmitDescription(prompt)
	sender.EmitFinal(&api.GenExperienceResponse{Name: "fake event"})
	return nil
}

func (fakeExperienceGen) StreamGenExperienceFromImage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ experiencegen.Context, _ string, sender experiencegen.StreamSender,
) error {
	sender.EmitDescription("fake description")
	sender.EmitFinal(&api.GenExperienceResponse{Name: "fake event"})
	return nil
}

func (fakeExperienceGen) StreamGenExperienceFromWebpage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ experiencegen.Context, _ string, sender experiencegen.StreamSender,
) error {
	sender.EmitDescription("fake description")
	sender.EmitFinal(&api.GenExperienceResponse{Name: "fake event"})
	return nil
}

type fakeGearGen struct{}

func (fakeGearGen) StreamGenGearFromText(_ context.Context, _ *logging.Logger, _ time.Time,
	_ geargen.Context, prompt string, sender geargen.StreamSender,
) error {
	sender.EmitDescription(prompt)
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

func (fakeGearGen) StreamGenGearFromImage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ geargen.Context, _ string, sender geargen.StreamSender,
) error {
	sender.EmitDescription("fake description")
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

func (fakeGearGen) StreamGenGearFromWebpage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ geargen.Context, _ string, sender geargen.StreamSender,
) error {
	sender.EmitDescription("fake description")
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

type fakeRequestGen struct{}

func (fakeRequestGen) StreamGenRequestFromText(_ context.Context, _ *logging.Logger, _ time.Time,
	_ requestgen.Context, prompt string, sender requestgen.StreamSender,
) error {
	sender.EmitDescription(prompt)
	sender.EmitFinal(&api.GenRequestResponse{Title: "fake request"})
	return nil
}

func (fakeRequestGen) StreamGenRequestFromImage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ requestgen.Context, _ string, sender requestgen.StreamSender,
) error {
	sender.EmitDescription("fake description")
	sender.EmitFinal(&api.GenRequestResponse{Title: "fake request"})
	return nil
}

// newTestService constructs a Service with the supplied classifier and
// the fake per-type generators above. Use this for dispatcher-focused
// unit tests; integration tests that need real fan-out construct the
// real services.
func newTestService(c Classifier) *Service {
	return New(c, ai.NewMockProvider(), nil, nil, nil, fakeExperienceGen{}, fakeGearGen{}, fakeRequestGen{})
}

// mockClassifier returns a fixed result, tracks invocation, and
// captures the most recent input so tests can assert what the caller
// actually passed.
type mockClassifier struct {
	mu        sync.Mutex
	result    *ai.UnifiedCreateClassification
	err       error
	called    bool
	lastInput ClassifierInput
}

func (m *mockClassifier) Classify(_ context.Context, in ClassifierInput) (*ai.UnifiedCreateClassification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	m.lastInput = in
	return m.result, m.err
}

// captureTarget collects events for test assertions.
type captureTarget struct {
	mu     sync.Mutex
	events []*api.StreamGenUnifiedCreateResponse
}

func (c *captureTarget) Send(ev *api.StreamGenUnifiedCreateResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func authedCtx() context.Context {
	return authn.SetInfo(context.Background(), &auth.Info{
		UserID: "test-user",
		Email:  "test@example.com",
	})
}

func TestStreamGenUnifiedCreate_RejectsEmptyInput(t *testing.T) {
	svc := newTestService(&mockClassifier{})
	target := &captureTarget{}
	if err := svc.streamGenUnifiedCreate(authedCtx(), &api.StreamGenUnifiedCreateRequest{}, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}
	if len(target.events) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(target.events))
	}
	if _, ok := target.events[0].Event.(*api.StreamGenUnifiedCreateResponse_Error); !ok {
		t.Errorf("expected error event, got %T", target.events[0].Event)
	}
}

// TestStreamGenUnifiedCreate_AcceptsSingleInput is the happy path for a
// text input. Asserts no error event and exactly one terminal final.
func TestStreamGenUnifiedCreate_AcceptsSingleInput(t *testing.T) {
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeRequest}}
	svc := newTestService(mock)
	target := &captureTarget{}
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt: &api.StreamGenUnifiedCreateRequest_Text{Text: "hike"},
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}
	var sawFinal int
	for _, ev := range target.events {
		if _, ok := ev.Event.(*api.StreamGenUnifiedCreateResponse_Error); ok {
			t.Errorf("unexpected error event: %v", ev)
		}
		if _, ok := ev.Event.(*api.StreamGenUnifiedCreateResponse_Final); ok {
			sawFinal++
		}
	}
	if sawFinal != 1 {
		t.Errorf("expected exactly one final event, got %d", sawFinal)
	}
}

func TestStreamGenUnifiedCreate_ForceTypeSkipsClassifier(t *testing.T) {
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeGear}}
	svc := newTestService(mock)
	target := &captureTarget{}

	ft := api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt:    &api.StreamGenUnifiedCreateRequest_Text{Text: "hike tomorrow"},
		ForceType: &ft,
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}
	if mock.called {
		t.Error("classifier should NOT be called when force_type is set")
	}

	var sawType, sawFinal bool
	for _, ev := range target.events {
		switch e := ev.Event.(type) {
		case *api.StreamGenUnifiedCreateResponse_Type:
			sawType = true
			if e.Type != ft {
				t.Errorf("type event mismatch: got %v want %v", e.Type, ft)
			}
		case *api.StreamGenUnifiedCreateResponse_Final:
			sawFinal = true
			if e.Final.Type != ft {
				t.Errorf("final type mismatch: got %v want %v", e.Final.Type, ft)
			}
			if e.Final.GetExperience() == nil {
				t.Error("expected experience payload in final for EVENT type")
			}
		}
	}
	if !sawType {
		t.Error("did not see type event")
	}
	if !sawFinal {
		t.Error("did not see final event")
	}
}

func TestStreamGenUnifiedCreate_ClassifierIsCalledWhenNoForce(t *testing.T) {
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeRequest}}
	svc := newTestService(mock)
	target := &captureTarget{}
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt: &api.StreamGenUnifiedCreateRequest_Text{Text: "anyone have a drill?"},
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}
	if !mock.called {
		t.Error("classifier should be called when force_type is unset")
	}

	var lastType api.DetectedContentType
	for _, ev := range target.events {
		if e, ok := ev.Event.(*api.StreamGenUnifiedCreateResponse_Type); ok {
			lastType = e.Type
		}
	}
	if lastType != api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST {
		t.Errorf("expected REQUEST type, got %v", lastType)
	}
}

func TestStreamGenUnifiedCreate_RejectsUnspecifiedForceType(t *testing.T) {
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeGear}}
	svc := newTestService(mock)
	target := &captureTarget{}
	ft := api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt:    &api.StreamGenUnifiedCreateRequest_Text{Text: "anything"},
		ForceType: &ft,
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}
	var sawError, sawFinal bool
	for _, ev := range target.events {
		switch ev.Event.(type) {
		case *api.StreamGenUnifiedCreateResponse_Error:
			sawError = true
		case *api.StreamGenUnifiedCreateResponse_Final:
			sawFinal = true
		}
	}
	if !sawError {
		t.Error("expected an error event")
	}
	if sawFinal {
		t.Error("did not expect a final event")
	}
	if mock.called {
		t.Error("classifier should not be called when force_type is unspecified — short-circuit error")
	}
}

func TestDetectedTypeFromString(t *testing.T) {
	cases := map[string]api.DetectedContentType{
		"gear":    api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR,
		"event":   api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT,
		"request": api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST,
		"":        api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED,
		"junk":    api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := detectedTypeFromString(in); got != want {
			t.Errorf("detectedTypeFromString(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestStreamGenUnifiedCreate_ImageMode_ResolvesMediaToClassifierInput
// asserts that when the caller passes a media_id, the handler resolves
// it to a *DetectionImage (signed URL + content type) and passes it on
// the classifier input — the core wiring of #1939.
func TestStreamGenUnifiedCreate_ImageMode_ResolvesMediaToClassifierInput(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Seed a media row owned by test-user (matching authedCtx).
	media := &models.Media{
		Id:          "test-media-id",
		UserId:      "test-user",
		ContentType: "image/jpeg",
	}
	if _, err := sqlStorage.Insert(context.Background(), media); err != nil {
		t.Fatalf("insert media: %v", err)
	}

	mockBucket := &services.MockBucketStorage{SignedURL: "https://signed.example.invalid/test"}
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeGear}}
	svc := New(mock, ai.NewMockProvider(), sqlStorage, mockBucket, nil, fakeExperienceGen{}, fakeGearGen{}, fakeRequestGen{})

	target := &captureTarget{}
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt: &api.StreamGenUnifiedCreateRequest_MediaId{MediaId: "test-media-id"},
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}

	if !mock.called {
		t.Fatal("classifier not called")
	}
	if mock.lastInput.Image == nil {
		t.Fatal("ClassifierInput.Image is nil; expected a populated *DetectionImage")
	}
	if mock.lastInput.Image.ImageURL == "" {
		t.Error("ClassifierInput.Image.ImageURL is empty; bucket.GetSignedURL output should have landed here")
	}
	if mock.lastInput.Image.MimeType != "image/jpeg" {
		t.Errorf("ClassifierInput.Image.MimeType = %q, want %q", mock.lastInput.Image.MimeType, "image/jpeg")
	}
	// MediaID is intentionally NOT on ClassifierInput — replaced by Image
	// in #1939. Asserting the new shape is the whole point of this test.
}

// capturingGearGen is fakeGearGen with a hook that records the Context
// it received on the image-mode call. Used to assert the dispatcher
// threads the classifier's resolved *DetectionImage forward to the
// per-type call (#1952 / #1939 phase 5).
type capturingGearGen struct {
	lastImageCtx geargen.Context
}

func (c *capturingGearGen) StreamGenGearFromText(_ context.Context, _ *logging.Logger, _ time.Time,
	_ geargen.Context, _ string, sender geargen.StreamSender,
) error {
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

func (c *capturingGearGen) StreamGenGearFromImage(_ context.Context, _ *logging.Logger, _ time.Time,
	args geargen.Context, _ string, sender geargen.StreamSender,
) error {
	c.lastImageCtx = args
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

func (c *capturingGearGen) StreamGenGearFromWebpage(_ context.Context, _ *logging.Logger, _ time.Time,
	_ geargen.Context, _ string, sender geargen.StreamSender,
) error {
	sender.EmitFinal(&api.GenGearResponse{DetectedGear: &api.DetectedGearItem{Title: "fake gear"}})
	return nil
}

// TestStreamGenUnifiedCreate_ImageMode_ThreadsImageToPerTypeCall asserts
// the classifier's resolved *DetectionImage is passed forward to the
// per-type call's Context.Image so both AI calls see the same signed
// URL (enabling Anthropic prompt-cache hits on the per-type call).
// This is the core wiring of #1952 / #1939 phase 5.
func TestStreamGenUnifiedCreate_ImageMode_ThreadsImageToPerTypeCall(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	media := &models.Media{
		Id:          "test-media-id",
		UserId:      "test-user",
		ContentType: "image/jpeg",
	}
	if _, err := sqlStorage.Insert(context.Background(), media); err != nil {
		t.Fatalf("insert media: %v", err)
	}

	const signedURL = "https://signed.example.invalid/test-thread-forward"
	mockBucket := &services.MockBucketStorage{SignedURL: signedURL}
	mock := &mockClassifier{result: &ai.UnifiedCreateClassification{Type: ai.UnifiedCreateContentTypeGear}}
	gearGen := &capturingGearGen{}
	svc := New(mock, ai.NewMockProvider(), sqlStorage, mockBucket, nil, fakeExperienceGen{}, gearGen, fakeRequestGen{})

	target := &captureTarget{}
	msg := &api.StreamGenUnifiedCreateRequest{
		Prompt: &api.StreamGenUnifiedCreateRequest_MediaId{MediaId: "test-media-id"},
	}
	if err := svc.streamGenUnifiedCreate(authedCtx(), msg, target); err != nil {
		t.Fatalf("streamGenUnifiedCreate: %v", err)
	}

	if mock.lastInput.Image == nil {
		t.Fatal("classifier did not receive a *DetectionImage")
	}
	if gearGen.lastImageCtx.Image == nil {
		t.Fatal("per-type gear call did not receive Context.Image; thread-forward broken")
	}
	if got, want := gearGen.lastImageCtx.Image.ImageURL, mock.lastInput.Image.ImageURL; got != want {
		t.Errorf("per-type Context.Image.ImageURL = %q, want %q (same URL as classifier)", got, want)
	}
	if got, want := gearGen.lastImageCtx.Image.ImageURL, signedURL; got != want {
		t.Errorf("per-type Context.Image.ImageURL = %q, want signed URL %q (single resolve, reused)", got, want)
	}
}
