package ai

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

func TestNewLoadBalancedProvider(t *testing.T) {
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	t.Run("creates with valid providers", func(t *testing.T) {
		lb, err := NewLoadBalancedProvider([]WeightedProvider{
			{Provider: provider1, Name: "p1", Weight: 0.5},
			{Provider: provider2, Name: "p2", Weight: 0.5},
		})
		if err != nil {
			t.Fatalf("Failed to create: %v", err)
		}
		if lb == nil {
			t.Fatal("Expected non-nil")
		}
	})

	t.Run("rejects empty providers", func(t *testing.T) {
		_, err := NewLoadBalancedProvider([]WeightedProvider{})
		if err == nil {
			t.Error("Expected error for empty providers")
		}
	})

	t.Run("normalizes weights to sum to 1.0", func(t *testing.T) {
		lb, _ := NewLoadBalancedProvider([]WeightedProvider{
			{Provider: provider1, Name: "p1", Weight: 1.0},
			{Provider: provider2, Name: "p2", Weight: 3.0},
		})
		total := 0.0
		for _, wp := range lb.providers {
			total += wp.Weight
		}
		if total < 0.99 || total > 1.01 {
			t.Errorf("Weights should sum to 1.0, got %f", total)
		}
	})

	t.Run("handles zero weights with equal distribution", func(t *testing.T) {
		lb, _ := NewLoadBalancedProvider([]WeightedProvider{
			{Provider: provider1, Name: "p1", Weight: 0},
			{Provider: provider2, Name: "p2", Weight: 0},
		})
		for _, wp := range lb.providers {
			if wp.Weight != 0.5 {
				t.Errorf("Expected equal weight 0.5, got %f", wp.Weight)
			}
		}
	})
}

func TestLoadBalancedProvider_Distribution(t *testing.T) {
	ctx := context.Background()
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	// Make many calls to test distribution
	for i := 0; i < 100; i++ {
		_, _ = lb.GenerateCommunityContent(ctx, "test", "")
	}

	// Both should have been called
	if provider1.Calls.TotalCalls() == 0 {
		t.Error("Expected provider1 to receive calls")
	}
	if provider2.Calls.TotalCalls() == 0 {
		t.Error("Expected provider2 to receive calls")
	}
}

func TestLoadBalancedProvider_WeightedDistribution(t *testing.T) {
	ctx := context.Background()
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	// 90% to provider1, 10% to provider2
	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.9},
		{Provider: provider2, Name: "p2", Weight: 0.1},
	})

	for i := 0; i < 1000; i++ {
		_, _ = lb.GenerateCommunityContent(ctx, "test", "")
	}

	// Provider1 should have significantly more calls
	p1Calls := provider1.Calls.TotalCalls()
	p2Calls := provider2.Calls.TotalCalls()
	ratio := float64(p1Calls) / float64(p1Calls+p2Calls)
	if ratio < 0.8 || ratio > 0.95 {
		t.Errorf("Expected ~90%% calls to provider1, got %.2f%%", ratio*100)
	}
}

func TestLoadBalancedProvider_PerMethodWeights(t *testing.T) {
	ctx := context.Background()
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	// Set method-specific weights: 100% to provider1 for community content
	lb.SetMethodWeights(&MethodWeights{
		GenerateCommunityContent: []WeightedProvider{
			{Provider: provider1, Name: "p1", Weight: 1.0},
		},
	})

	for i := 0; i < 50; i++ {
		_, _ = lb.GenerateCommunityContent(ctx, "test", "")
	}

	if provider1.Calls.TotalCalls() != 50 {
		t.Errorf("Expected all calls to provider1, got %d", provider1.Calls.TotalCalls())
	}
	if provider2.Calls.TotalCalls() != 0 {
		t.Errorf("Expected no calls to provider2, got %d", provider2.Calls.TotalCalls())
	}
}

func TestLoadBalancedProvider_AllMethods(t *testing.T) {
	ctx := context.Background()
	provider := newDecoratorMock("p1", false, "")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider, Name: "p1", Weight: 1.0},
	})

	// Test all four methods
	_, _ = lb.DetectGearInImage(ctx, &DetectionImage{ImageData: []byte("test"), MimeType: "image/jpeg"})
	_, _ = lb.GenerateCommunityContent(ctx, "test", "")
	_, _ = lb.GenerateRequestContent(ctx, "test", "")
	_, _ = lb.GenerateConversationSummary(ctx, []ConversationMessage{{SenderName: "A", Text: "Hi"}}, "", "", SummaryStyleProgress)

	if provider.Calls.TotalCalls() != 4 {
		t.Errorf("Expected 4 calls, got %d", provider.Calls.TotalCalls())
	}
}

// newDecoratorMock creates a MockProvider configured for decorator (load-balancer/fallback) tests.
// It uses NameValue for identification and optionally returns errors from all methods.
func newDecoratorMock(name string, shouldError bool, errorMsg string) *MockProvider {
	mock := NewMockProvider()
	mock.NameValue = name

	if shouldError {
		mock.DetectGearFunc = func(_ context.Context, _ *DetectionImage) (*GearDetection, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateGearFromTextFunc = func(_ context.Context, _, _ string) (*GearGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateGearFromWebpageFunc = func(_ context.Context, _, _, _, _ string) (*GearGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateCommunityFunc = func(_ context.Context, _, _ string) (*CommunityGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateRequestFunc = func(_ context.Context, _, _ string) (*RequestGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateExperienceFromTextFunc = func(_ context.Context, _, _, _ string) (*ExperienceGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateExperienceFromImageFunc = func(_ context.Context, _ *DetectionImage, _, _, _ string) (*ExperienceGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateExperienceFromWebpageFunc = func(_ context.Context, _, _, _, _, _ string) (*ExperienceGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateRequestFromImageFunc = func(_ context.Context, _ *DetectionImage, _ string) (*RequestGeneration, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateConversationSummaryFunc = func(_ context.Context, _ []ConversationMessage, _, _ string, _ SummaryStyle) (*ConversationSummary, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.GenerateExperienceCompletionSummaryFunc = func(_ context.Context, _ []ConversationMessage, _, _ string, _ []string) (*ConversationSummary, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.ParseInformalTimeFunc = func(_ context.Context, _, _, _ string) (string, error) {
			return "", fmt.Errorf("%s", errorMsg)
		}
		mock.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*SocialAttributeInference, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		}
		mock.CheckHealthFunc = func(_ context.Context) ([]*health.Status, error) {
			return []*health.Status{{Name: name, Backend: name, Error: errorMsg}}, nil
		}
	} else {
		mock.CheckHealthFunc = func(_ context.Context) ([]*health.Status, error) {
			return []*health.Status{{Name: name, Backend: name}}, nil
		}
	}

	return mock
}

func TestLoadBalancedProvider_CheckHealth_AllHealthy(t *testing.T) {
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	statuses, err := lb.CheckHealth(context.Background())
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

func TestLoadBalancedProvider_CheckHealth_OneUnhealthy(t *testing.T) {
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", true, "connection failed")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	statuses, err := lb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}

	// One healthy provider → composite is healthy.
	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	if !statuses[0].IsHealthy() {
		t.Errorf("expected composite healthy with one working provider, got error: %v", statuses[0].Error)
	}
	if statuses[0].Metadata["healthy_provider_count"] != "1" {
		t.Errorf("expected healthy_provider_count=1, got %q", statuses[0].Metadata["healthy_provider_count"])
	}
}

func TestLoadBalancedProvider_CheckHealth_AllUnhealthy(t *testing.T) {
	provider1 := newDecoratorMock("p1", true, "p1 down")
	provider2 := newDecoratorMock("p2", true, "p2 down")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	statuses, err := lb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}

	// All providers unhealthy → composite is unhealthy.
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

func TestLoadBalancedProvider_CheckHealth_CompositeShape(t *testing.T) {
	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", false, "")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	statuses, err := lb.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}
	s := statuses[0]

	if s.Name != "ai" {
		t.Errorf("expected Name=ai, got %q", s.Name)
	}
	if s.Backend != "load-balanced" {
		t.Errorf("expected Backend=load-balanced, got %q", s.Backend)
	}
	for _, key := range []string{"weighted_backends", "healthy_provider_count", "total_provider_count"} {
		if _, ok := s.Metadata[key]; !ok {
			t.Errorf("expected metadata key %q to be present", key)
		}
	}
}

func TestLoadBalancedProvider_CheckHealth_LeafWarnLog(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
	ctx := logging.WithLogger(context.Background(), logger)

	provider1 := newDecoratorMock("p1", false, "")
	provider2 := newDecoratorMock("p2", true, "p2 down")

	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
		{Provider: provider1, Name: "p1", Weight: 0.5},
		{Provider: provider2, Name: "p2", Weight: 0.5},
	})

	statuses, err := lb.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	// Composite is healthy (p1 up), so no ERROR-level log.
	if !statuses[0].IsHealthy() {
		t.Errorf("expected composite healthy, got error: %v", statuses[0].Error)
	}
	out := buf.String()
	if strings.Contains(out, `"severity":"ERROR"`) {
		t.Errorf("expected no ERROR log when composite is healthy; got:\n%s", out)
	}
	if !strings.Contains(out, "dependency_leaf_unhealthy") {
		t.Errorf("expected dependency_leaf_unhealthy message; got:\n%s", out)
	}
}
