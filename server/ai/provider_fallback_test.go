package ai

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

func TestNewFallbackProvider(t *testing.T) {
	primary := newDecoratorMock("primary", false, "")

	t.Run("creates with valid primary", func(t *testing.T) {
		fb, err := NewFallbackProvider(primary, nil)
		if err != nil {
			t.Fatalf("Failed to create: %v", err)
		}
		if fb == nil {
			t.Fatal("Expected non-nil")
		}
	})

	t.Run("rejects nil primary", func(t *testing.T) {
		_, err := NewFallbackProvider(nil, nil)
		if err == nil {
			t.Error("Expected error for nil primary")
		}
	})

	t.Run("accepts nil fallbacks", func(t *testing.T) {
		fb, err := NewFallbackProvider(primary, nil)
		if err != nil {
			t.Fatalf("Should accept nil fallbacks: %v", err)
		}
		if fb == nil {
			t.Fatal("Expected non-nil")
		}
	})
}

func TestFallbackProvider_UsePrimary(t *testing.T) {
	ctx := context.Background()
	primary := newDecoratorMock("primary", false, "")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	_, _ = fb.GenerateCommunityContent(ctx, "test", "")

	if primary.Calls.TotalCalls() != 1 {
		t.Errorf("Expected primary called once, got %d", primary.Calls.TotalCalls())
	}
	if fallback.Calls.TotalCalls() != 0 {
		t.Errorf("Expected fallback not called, got %d", fallback.Calls.TotalCalls())
	}
}

func TestFallbackProvider_FallbackOnError(t *testing.T) {
	ctx := context.Background()

	// Any error should trigger fallback
	errors := []string{
		"500 Internal Server Error",
		"400 Bad Request",
		"connection refused",
		"timeout",
		"invalid_image_url",
	}

	for _, errMsg := range errors {
		t.Run(errMsg, func(t *testing.T) {
			primary := newDecoratorMock("primary", true, errMsg)
			fallback := newDecoratorMock("fallback", false, "")

			fb, _ := NewFallbackProvider(primary, []Provider{fallback})
			_, _ = fb.GenerateCommunityContent(ctx, "test", "")

			if primary.Calls.TotalCalls() != 1 {
				t.Errorf("Expected primary called once, got %d", primary.Calls.TotalCalls())
			}
			if fallback.Calls.TotalCalls() != 1 {
				t.Errorf("Expected fallback called once for '%s', got %d", errMsg, fallback.Calls.TotalCalls())
			}
		})
	}
}

func TestFallbackProvider_ChainedFallbacks(t *testing.T) {
	ctx := context.Background()
	primary := newDecoratorMock("primary", true, "500 error")
	fallback1 := newDecoratorMock("fallback1", true, "502 error")
	fallback2 := newDecoratorMock("fallback2", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback1, fallback2})
	_, _ = fb.GenerateCommunityContent(ctx, "test", "")

	if primary.Calls.TotalCalls() != 1 {
		t.Errorf("Expected primary called once, got %d", primary.Calls.TotalCalls())
	}
	if fallback1.Calls.TotalCalls() != 1 {
		t.Errorf("Expected fallback1 called once, got %d", fallback1.Calls.TotalCalls())
	}
	if fallback2.Calls.TotalCalls() != 1 {
		t.Errorf("Expected fallback2 called once, got %d", fallback2.Calls.TotalCalls())
	}
}

func TestFallbackProvider_PerMethodFallbacks(t *testing.T) {
	ctx := context.Background()
	primary := newDecoratorMock("primary", true, "500 error")
	defaultFallback := newDecoratorMock("default", false, "")
	methodFallback := newDecoratorMock("method", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{defaultFallback})
	fb.SetMethodFallbacks(&MethodFallbacks{
		GenerateCommunityContent: []Provider{methodFallback},
	})

	_, _ = fb.GenerateCommunityContent(ctx, "test", "")

	if defaultFallback.Calls.TotalCalls() != 0 {
		t.Errorf("Expected default fallback not called, got %d", defaultFallback.Calls.TotalCalls())
	}
	if methodFallback.Calls.TotalCalls() != 1 {
		t.Errorf("Expected method fallback called once, got %d", methodFallback.Calls.TotalCalls())
	}
}

// TestMethodFallbacks_EveryFieldIsRoutable guards #2816 structurally.
//
// MethodFallbacks.GenerateNudgeContent once had a field but no case in the
// switch that read the fields back, so a chain configured for it silently fell
// through to the default. That method is gone (#2936), and pinning the bug to
// whichever method happened to be broken would let the same mistake return on
// the next one. Field names match Provider method names one-for-one, so the
// invariant is checkable directly: every field must appear in byMethod.
func TestMethodFallbacks_EveryFieldIsRoutable(t *testing.T) {
	m := &MethodFallbacks{}
	routes := m.byMethod()

	typ := reflect.TypeOf(*m)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := routes[name]; !ok {
			t.Errorf("MethodFallbacks.%s has no entry in byMethod() — a chain "+
				"configured for it would be silently ignored (#2816)", name)
		}
	}

	// And nothing routes to a field that does not exist.
	for name := range routes {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("byMethod() routes %q, which is not a MethodFallbacks field", name)
		}
	}
}

// TestMethodWeights_EveryFieldIsRoutable is the load-balancer twin of the
// above: MethodWeights carries the same field-name-per-method contract and
// flattens through its own byMethod.
func TestMethodWeights_EveryFieldIsRoutable(t *testing.T) {
	m := &MethodWeights{}
	routes := m.byMethod()

	typ := reflect.TypeOf(*m)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := routes[name]; !ok {
			t.Errorf("MethodWeights.%s has no entry in byMethod() — a weight "+
				"configured for it would be silently ignored (#2816)", name)
		}
	}
	for name := range routes {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("byMethod() routes %q, which is not a MethodWeights field", name)
		}
	}
}

func TestFallbackProvider_AllMethods(t *testing.T) {
	ctx := context.Background()
	primary := newDecoratorMock("primary", true, "500 error")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	// Test all four methods trigger fallback
	_, _ = fb.DetectGearInImage(ctx, &DetectionImage{ImageData: []byte("test"), MimeType: "image/jpeg"})
	_, _ = fb.GenerateCommunityContent(ctx, "test", "")
	_, _ = fb.GenerateRequestContent(ctx, "test", "")
	_, _ = fb.GenerateConversationSummary(ctx, []ConversationMessage{{SenderName: "A", Text: "Hi"}}, "", "", SummaryStyleProgress)

	if primary.Calls.TotalCalls() != 4 {
		t.Errorf("Expected primary called 4 times, got %d", primary.Calls.TotalCalls())
	}
	if fallback.Calls.TotalCalls() != 4 {
		t.Errorf("Expected fallback called 4 times, got %d", fallback.Calls.TotalCalls())
	}
}

func TestFallbackProvider_ClassifyUnifiedCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("primary succeeds", func(t *testing.T) {
		primary := NewMockProvider()
		primary.ClassifyUnifiedCreateFunc = func(_ context.Context, _ UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
			return &UnifiedCreateClassification{Type: UnifiedCreateContentTypeEvent}, nil
		}
		fallback := NewMockProvider()

		fb, _ := NewFallbackProvider(primary, []Provider{fallback})
		got, err := fb.ClassifyUnifiedCreate(ctx, UnifiedCreateClassifierInput{Text: "hike tomorrow"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != UnifiedCreateContentTypeEvent {
			t.Errorf("expected EVENT, got %q", got.Type)
		}
		if len(fallback.Calls.ClassifyUnifiedCreate) != 0 {
			t.Errorf("fallback should not be called when primary succeeds, got %d calls", len(fallback.Calls.ClassifyUnifiedCreate))
		}
	})

	t.Run("primary fails, fallback succeeds", func(t *testing.T) {
		primary := NewMockProvider()
		primary.ClassifyUnifiedCreateFunc = func(_ context.Context, _ UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
			return nil, fmt.Errorf("anthropic API call failed: 503")
		}
		fallback := NewMockProvider()
		fallback.ClassifyUnifiedCreateFunc = func(_ context.Context, _ UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
			return &UnifiedCreateClassification{Type: UnifiedCreateContentTypeRequest}, nil
		}

		fb, _ := NewFallbackProvider(primary, []Provider{fallback})
		got, err := fb.ClassifyUnifiedCreate(ctx, UnifiedCreateClassifierInput{Text: "need a ladder"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != UnifiedCreateContentTypeRequest {
			t.Errorf("expected REQUEST from fallback, got %q", got.Type)
		}
		if len(fallback.Calls.ClassifyUnifiedCreate) != 1 {
			t.Errorf("expected 1 fallback call, got %d", len(fallback.Calls.ClassifyUnifiedCreate))
		}
	})

	t.Run("all providers fail", func(t *testing.T) {
		primary := NewMockProvider()
		primary.ClassifyUnifiedCreateFunc = func(_ context.Context, _ UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
			return nil, fmt.Errorf("primary boom")
		}
		fallback := NewMockProvider()
		fallback.ClassifyUnifiedCreateFunc = func(_ context.Context, _ UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
			return nil, fmt.Errorf("fallback boom")
		}

		fb, _ := NewFallbackProvider(primary, []Provider{fallback})
		_, err := fb.ClassifyUnifiedCreate(ctx, UnifiedCreateClassifierInput{Text: "x"})
		if err == nil {
			t.Fatal("expected error when all providers fail")
		}
	})
}

func TestFallbackProvider_CheckHealth_AllHealthy(t *testing.T) {
	primary := newDecoratorMock("primary", false, "")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	statuses, err := fb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	// Composite provider returns a single synthesized status.
	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	if !statuses[0].IsHealthy() {
		t.Errorf("expected composite to be healthy, got error: %v", statuses[0].Error)
	}
}

func TestFallbackProvider_CheckHealth_PrimaryUnhealthy(t *testing.T) {
	primary := newDecoratorMock("primary", true, "connection failed")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	statuses, err := fb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}

	// Primary unhealthy but fallback healthy → composite is healthy.
	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	if !statuses[0].IsHealthy() {
		t.Errorf("expected composite healthy when fallback is up, got error: %v", statuses[0].Error)
	}
	// Metadata should reflect that the primary was unhealthy.
	if statuses[0].Metadata["primary_healthy"] != "false" {
		t.Errorf("expected primary_healthy=false in metadata, got %q", statuses[0].Metadata["primary_healthy"])
	}
	if statuses[0].Metadata["healthy_fallback_count"] != "1" {
		t.Errorf("expected healthy_fallback_count=1 in metadata, got %q", statuses[0].Metadata["healthy_fallback_count"])
	}
}

func TestFallbackProvider_CheckHealth_FallbackUnhealthy(t *testing.T) {
	primary := newDecoratorMock("primary", false, "")
	fallback := newDecoratorMock("fallback", true, "timeout")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	statuses, err := fb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}

	// Primary healthy → composite is healthy even with a failing fallback.
	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	if !statuses[0].IsHealthy() {
		t.Errorf("expected composite healthy when primary is up, got error: %v", statuses[0].Error)
	}
	// Metadata should reflect primary healthy and zero healthy fallbacks.
	if statuses[0].Metadata["primary_healthy"] != "true" {
		t.Errorf("expected primary_healthy=true in metadata, got %q", statuses[0].Metadata["primary_healthy"])
	}
	if statuses[0].Metadata["healthy_fallback_count"] != "0" {
		t.Errorf("expected healthy_fallback_count=0 in metadata, got %q", statuses[0].Metadata["healthy_fallback_count"])
	}
}

func TestFallbackProvider_CheckHealth_AllUnhealthy(t *testing.T) {
	primary := newDecoratorMock("primary", true, "primary down")
	fallback := newDecoratorMock("fallback", true, "fallback down")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	statuses, err := fb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}

	// All leaves unhealthy → composite is unhealthy.
	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	if statuses[0].IsHealthy() {
		t.Error("expected composite to be unhealthy when all leaves fail")
	}
	if statuses[0].Error == "" {
		t.Error("expected non-empty error in composite when all leaves fail")
	}
}

func TestFallbackProvider_CheckHealth_CompositeShape(t *testing.T) {
	primary := newDecoratorMock("primary", false, "")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})

	statuses, err := fb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	s := statuses[0]

	// Name must be "ai" (the dependency category, not a leaf backend name).
	if s.Name != "ai" {
		t.Errorf("expected Name=ai, got %q", s.Name)
	}
	// Backend must include "+fallback" to signal composite origin.
	if !strings.Contains(s.Backend, "+fallback") {
		t.Errorf("expected Backend to contain +fallback, got %q", s.Backend)
	}
	// Required metadata keys.
	for _, key := range []string{"primary_backend", "primary_healthy", "fallback_backends", "healthy_fallback_count"} {
		if _, ok := s.Metadata[key]; !ok {
			t.Errorf("expected metadata key %q to be present", key)
		}
	}
}

func TestFallbackProvider_CheckHealth_LeafWarnLog(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
	ctx := logging.WithLogger(context.Background(), logger)

	primary := newDecoratorMock("primary", true, "primary down")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})
	_, err := fb.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	out := buf.String()
	// Composite is healthy (fallback up), so no ERROR-level log must be emitted.
	if strings.Contains(out, `"severity":"ERROR"`) {
		t.Errorf("expected no ERROR log when composite is healthy; got:\n%s", out)
	}
	// An unhealthy leaf must produce a WARN "dependency_leaf_unhealthy" log.
	if !strings.Contains(out, `"severity":"WARNING"`) {
		t.Errorf("expected WARNING log for unhealthy leaf; got:\n%s", out)
	}
	if !strings.Contains(out, "dependency_leaf_unhealthy") {
		t.Errorf("expected dependency_leaf_unhealthy message; got:\n%s", out)
	}
}

// TestMockProvider_CheckHealth verifies the mock provider's CheckHealth implementation.
func TestMockProvider_CheckHealth(t *testing.T) {
	mp := &MockProvider{}

	statuses, err := mp.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Backend != "mock" {
		t.Errorf("expected backend=mock, got %s", statuses[0].Backend)
	}
}

// TestFallbackProvider_NoErrorLogWhenFallbackSucceeds verifies that when the primary provider
// fails and the fallback succeeds, the FallbackProvider emits no ERROR-level log entries.
// This is the regression guard for #2732: leaf-provider failures behind a fallback chain
// must stay at WARN so the `all_errors` Cloud Monitoring policy does not fire.
func TestFallbackProvider_NoErrorLogWhenFallbackSucceeds(t *testing.T) {
	// Capture structured log output via a buffer-backed JSON logger.
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
	ctx := logging.WithLogger(context.Background(), logger)

	primary := newDecoratorMock("primary", true, "gemini API call failed: 503 Service Unavailable")
	fallback := newDecoratorMock("fallback", false, "")

	fb, _ := NewFallbackProvider(primary, []Provider{fallback})
	_, err := fb.GenerateCommunityContent(ctx, "test", "")
	if err != nil {
		t.Fatalf("expected fallback to recover, got error: %v", err)
	}

	out := buf.String()
	// The fallback should warn about the primary failure but never escalate to ERROR.
	if strings.Contains(out, `"severity":"ERROR"`) {
		t.Errorf("expected no ERROR log when fallback succeeds; got log output:\n%s", out)
	}
	// At least one WARNING should have been emitted (primary failure).
	if !strings.Contains(out, `"severity":"WARNING"`) {
		t.Errorf("expected WARNING log for primary failure; got log output:\n%s", out)
	}
}

func TestMockProvider_CheckHealth_WithFunc(t *testing.T) {
	mp := &MockProvider{
		CheckHealthFunc: func(ctx context.Context) ([]*health.Status, error) {
			return []*health.Status{{Name: "custom", Backend: "custom", Metadata: map[string]string{"key": "value"}}}, nil
		},
	}

	statuses, err := mp.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Backend != "custom" {
		t.Errorf("expected backend=custom, got %s", statuses[0].Backend)
	}
	if statuses[0].Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value, got %s", statuses[0].Metadata["key"])
	}
}
