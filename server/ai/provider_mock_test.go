package ai

import (
	"context"
	"sync"
	"testing"
)

func TestMockProvider_CallTracking(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	t.Run("records DetectGearInImage calls", func(t *testing.T) {
		req := &DetectionImage{ImageData: []byte("test"), MimeType: "image/jpeg"}
		_, _ = mock.DetectGearInImage(ctx, req)

		if len(mock.Calls.DetectGearInImage) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls.DetectGearInImage))
		}
		if mock.Calls.DetectGearInImage[0].Req != req {
			t.Error("expected captured req to match")
		}
	})

	t.Run("records GenerateGearFromText calls", func(t *testing.T) {
		mock.Reset()
		_, _ = mock.GenerateGearFromText(ctx, "my drill", "US-West")

		if len(mock.Calls.GenerateGearFromText) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls.GenerateGearFromText))
		}
		call := mock.Calls.GenerateGearFromText[0]
		if call.Prompt != "my drill" {
			t.Errorf("expected prompt 'my drill', got %q", call.Prompt)
		}
		if call.Region != "US-West" {
			t.Errorf("expected region 'US-West', got %q", call.Region)
		}
	})

	t.Run("records GenerateGearFromWebpage calls", func(t *testing.T) {
		mock.Reset()
		_, _ = mock.GenerateGearFromWebpage(ctx, "Title", "Desc", "Body", "US")

		if len(mock.Calls.GenerateGearFromWebpage) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls.GenerateGearFromWebpage))
		}
		call := mock.Calls.GenerateGearFromWebpage[0]
		if call.PageTitle != "Title" || call.PageDescription != "Desc" || call.PageBody != "Body" || call.Region != "US" {
			t.Errorf("unexpected captured args: %+v", call)
		}
	})

	t.Run("records GenerateConversationSummary calls with style", func(t *testing.T) {
		mock.Reset()
		msgs := []ConversationMessage{{SenderName: "Alice", Text: "Hi"}}
		_, _ = mock.GenerateConversationSummary(ctx, msgs, "req-title", "req-desc", SummaryStyleCompletion)

		if len(mock.Calls.GenerateConversationSummary) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls.GenerateConversationSummary))
		}
		call := mock.Calls.GenerateConversationSummary[0]
		if call.Style != SummaryStyleCompletion {
			t.Errorf("expected SummaryStyleCompletion, got %v", call.Style)
		}
		if call.RequestTitle != "req-title" {
			t.Errorf("expected request title 'req-title', got %q", call.RequestTitle)
		}
		if len(call.Messages) != 1 || call.Messages[0].SenderName != "Alice" {
			t.Errorf("unexpected messages: %+v", call.Messages)
		}
	})

	t.Run("records InferSocialAttributes calls", func(t *testing.T) {
		mock.Reset()
		_, _ = mock.InferSocialAttributes(ctx, "Drill", "A power drill", "gear_loan", 50.0)

		if len(mock.Calls.InferSocialAttributes) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls.InferSocialAttributes))
		}
		call := mock.Calls.InferSocialAttributes[0]
		if call.Title != "Drill" || call.TxType != "gear_loan" || call.ItemValueUSD != 50.0 {
			t.Errorf("unexpected captured args: %+v", call)
		}
	})
}

func TestMockProvider_TotalCalls(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	_, _ = mock.DetectGearInImage(ctx, &DetectionImage{ImageData: []byte("test"), MimeType: "image/jpeg"})
	_, _ = mock.GenerateCommunityContent(ctx, "test", "")
	_, _ = mock.GenerateRequestContent(ctx, "test", "")
	_, _ = mock.CheckHealth(ctx)

	if mock.Calls.TotalCalls() != 4 {
		t.Errorf("expected 4 total calls, got %d", mock.Calls.TotalCalls())
	}
}

func TestMockProvider_Reset(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	_, _ = mock.GenerateCommunityContent(ctx, "test", "")
	_, _ = mock.GenerateRequestContent(ctx, "test", "")
	_, _ = mock.CheckHealth(ctx)

	if mock.Calls.TotalCalls() != 3 {
		t.Fatalf("expected 3 calls before reset, got %d", mock.Calls.TotalCalls())
	}

	mock.Reset()

	if mock.Calls.TotalCalls() != 0 {
		t.Errorf("expected 0 calls after reset, got %d", mock.Calls.TotalCalls())
	}
	if len(mock.Calls.GenerateCommunityContent) != 0 {
		t.Errorf("expected empty GenerateCommunityContent after reset")
	}
}

func TestMockProvider_NameValue(t *testing.T) {
	t.Run("defaults to mock", func(t *testing.T) {
		mock := NewMockProvider()
		if mock.Name() != "mock" {
			t.Errorf("expected 'mock', got %q", mock.Name())
		}
	})

	t.Run("uses custom name", func(t *testing.T) {
		mock := NewMockProvider()
		mock.NameValue = "provider-1"
		if mock.Name() != "provider-1" {
			t.Errorf("expected 'provider-1', got %q", mock.Name())
		}
	})
}

func TestMockProvider_FuncOverrideWithCallTracking(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	// Override a function and verify both the override runs AND the call is tracked.
	customCalled := false
	mock.GenerateGearFromTextFunc = func(_ context.Context, prompt, region string) (*GearGeneration, error) {
		customCalled = true
		return &GearGeneration{Title: "Custom: " + prompt}, nil
	}

	result, err := mock.GenerateGearFromText(ctx, "tent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !customCalled {
		t.Error("expected custom func to be called")
	}
	if result.Title != "Custom: tent" {
		t.Errorf("expected 'Custom: tent', got %q", result.Title)
	}
	if len(mock.Calls.GenerateGearFromText) != 1 {
		t.Fatalf("expected 1 tracked call, got %d", len(mock.Calls.GenerateGearFromText))
	}
	if mock.Calls.GenerateGearFromText[0].Prompt != "tent" {
		t.Errorf("expected captured prompt 'tent', got %q", mock.Calls.GenerateGearFromText[0].Prompt)
	}
}

func TestMockProvider_ClassifyUnifiedCreate(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	// Default returns a gear classification with high confidence.
	got, err := mock.ClassifyUnifiedCreate(ctx, UnifiedCreateClassifierInput{Text: "drill"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != UnifiedCreateContentTypeGear {
		t.Errorf("default classification: expected GEAR, got %q", got.Type)
	}
	if len(mock.Calls.ClassifyUnifiedCreate) != 1 {
		t.Fatalf("expected 1 tracked call, got %d", len(mock.Calls.ClassifyUnifiedCreate))
	}
	if mock.Calls.ClassifyUnifiedCreate[0].Input.Text != "drill" {
		t.Errorf("expected captured input text 'drill', got %q", mock.Calls.ClassifyUnifiedCreate[0].Input.Text)
	}

	// Override returns a custom classification.
	mock.ClassifyUnifiedCreateFunc = func(_ context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
		return &UnifiedCreateClassification{Type: UnifiedCreateContentTypeEvent}, nil
	}
	got, err = mock.ClassifyUnifiedCreate(ctx, UnifiedCreateClassifierInput{Text: "hike sunday"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != UnifiedCreateContentTypeEvent {
		t.Errorf("override classification: expected EVENT, got %q", got.Type)
	}
	if len(mock.Calls.ClassifyUnifiedCreate) != 2 {
		t.Errorf("expected 2 tracked calls after override, got %d", len(mock.Calls.ClassifyUnifiedCreate))
	}
}

// TestMockProvider_ConcurrentCallRecording is the regression guard for #2658:
// several fan-out sites (e.g. experience.ExtractTimeCandidates) invoke the
// same MockProvider method from parallel goroutines, so the Calls-slice
// appends must be serialized. Without the mock's internal mutex, this test
// races on the Calls slice header and fails under `go test -race`.
func TestMockProvider_ConcurrentCallRecording(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()
	mock.ParseInformalTimeFunc = func(_ context.Context, _, _, _ string) (string, error) {
		return "{}", nil
	}

	const goroutines = 32
	const callsPerGoroutine = 25
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				_, _ = mock.ParseInformalTime(ctx, "tonight", "2026-01-01T00:00:00Z", "UTC")
				_, _ = mock.GenerateGearFromText(ctx, "tent", "US")
				_, _ = mock.CheckHealth(ctx)
			}
		}()
	}
	wg.Wait()

	// Post-fanout reads are safe because Wait establishes happens-before.
	// Snapshot() also works while the mock is being hammered — verify both.
	want := goroutines * callsPerGoroutine
	if got := len(mock.Calls.ParseInformalTime); got != want {
		t.Errorf("ParseInformalTime: expected %d recorded calls, got %d", want, got)
	}
	if got := len(mock.Calls.GenerateGearFromText); got != want {
		t.Errorf("GenerateGearFromText: expected %d recorded calls, got %d", want, got)
	}
	if got := len(mock.Calls.CheckHealth); got != want {
		t.Errorf("CheckHealth: expected %d recorded calls, got %d", want, got)
	}
}

// TestMockProvider_SnapshotDuringConcurrentCalls exercises the Snapshot API
// under contention: readers pull consistent copies while writers append.
// The race detector catches any unsynchronized slice access.
func TestMockProvider_SnapshotDuringConcurrentCalls(t *testing.T) {
	ctx := context.Background()
	mock := NewMockProvider()

	writers := 8
	reads := 200
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = mock.CheckHealth(ctx)
				}
			}
		}()
	}

	for i := 0; i < reads; i++ {
		snap := mock.Snapshot()
		// Just touch the slices so the race detector observes the read.
		_ = len(snap.CheckHealth)
	}
	close(stop)
	wg.Wait()
}
