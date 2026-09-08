package product

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/webfetch"
)

// mockWebFetcher implements webfetch.Fetcher for testing.
type mockWebFetcher struct {
	pageContent *webfetch.PageContent
	err         error
	delay       time.Duration
}

func (m *mockWebFetcher) FetchPageContent(ctx context.Context, url string) (*webfetch.PageContent, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.pageContent, nil
}

// newMockAIProvider creates a MockProvider that returns the given result from GenerateGearFromWebpage.
func newMockAIProvider(gearGeneration *ai.GearGeneration, err error) *ai.MockProvider {
	mock := ai.NewMockProvider()
	mock.GenerateGearFromWebpageFunc = func(_ context.Context, _, _, _, _ string) (*ai.GearGeneration, error) {
		if err != nil {
			return nil, err
		}
		return gearGeneration, nil
	}
	return mock
}

func TestNewLookup(t *testing.T) {
	fetcher := &mockWebFetcher{}
	provider := newMockAIProvider(nil, nil)

	lookup := NewLookup(fetcher, provider)

	if lookup == nil {
		t.Fatal("Expected non-nil lookup")
	}
	if lookup.timeout != DefaultTimeout {
		t.Errorf("Expected default timeout %v, got %v", DefaultTimeout, lookup.timeout)
	}
}

func TestLookup_SetTimeout(t *testing.T) {
	lookup := NewLookup(&mockWebFetcher{}, newMockAIProvider(nil, nil))

	newTimeout := 5 * time.Second
	lookup.SetTimeout(newTimeout)

	if lookup.timeout != newTimeout {
		t.Errorf("Expected timeout %v, got %v", newTimeout, lookup.timeout)
	}
}

func TestLookup_LookupSpecs_NilDependencies(t *testing.T) {
	tests := []struct {
		name       string
		webFetcher webfetch.Fetcher
		aiProvider ai.Provider
	}{
		{
			name:       "nil web fetcher",
			webFetcher: nil,
			aiProvider: newMockAIProvider(nil, nil),
		},
		{
			name:       "nil ai provider",
			webFetcher: &mockWebFetcher{},
			aiProvider: nil,
		},
		{
			name:       "both nil",
			webFetcher: nil,
			aiProvider: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := &Lookup{
				webFetcher: tt.webFetcher,
				aiProvider: tt.aiProvider,
				timeout:    DefaultTimeout,
			}

			result, err := lookup.LookupSpecs(context.Background(), &Info{
				Brand: "DeWalt",
				Model: "DCD771C2",
			})
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil result, got %v", result)
			}
		})
	}
}

func TestLookup_LookupSpecs_NilOrEmptyInfo(t *testing.T) {
	lookup := NewLookup(&mockWebFetcher{}, newMockAIProvider(nil, nil))

	tests := []struct {
		name string
		info *Info
	}{
		{
			name: "nil info",
			info: nil,
		},
		{
			name: "empty brand and model",
			info: &Info{Brand: "", Model: ""},
		},
		{
			name: "whitespace only",
			info: &Info{Brand: "  ", Model: "  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := lookup.LookupSpecs(context.Background(), tt.info)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil result, got %v", result)
			}
		})
	}
}

func TestLookup_LookupSpecs_Success(t *testing.T) {
	expectedGeneration := &ai.GearGeneration{
		Title:       "DeWalt DCD771C2 Drill",
		Description: "20V MAX Cordless Drill/Driver Kit",
		Confidence:  0.9,
		ValueEstimate: &ai.ValueEstimate{
			EstimatedValueUSD: 149.99,
			Confidence:        0.85,
			Reasoning:         "Based on manufacturer MSRP",
		},
	}

	fetcher := &mockWebFetcher{
		pageContent: &webfetch.PageContent{
			Title:       "DeWalt DCD771C2",
			Description: "Power drill from DeWalt",
			BodyText:    "Specifications: 20V, 1/2 inch chuck...",
		},
	}
	provider := newMockAIProvider(expectedGeneration, nil)

	lookup := NewLookup(fetcher, provider)

	result, err := lookup.LookupSpecs(context.Background(), &Info{
		Brand:      "DeWalt",
		Model:      "DCD771C2",
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Title != expectedGeneration.Title {
		t.Errorf("Expected title %q, got %q", expectedGeneration.Title, result.Title)
	}
	if result.ValueEstimate == nil {
		t.Fatal("Expected value estimate")
	}
	if result.ValueEstimate.EstimatedValueUSD != 149.99 {
		t.Errorf("Expected value 149.99, got %f", result.ValueEstimate.EstimatedValueUSD)
	}
}

func TestLookup_LookupSpecs_Timeout(t *testing.T) {
	fetcher := &mockWebFetcher{
		delay: 5 * time.Second, // Longer than timeout
		pageContent: &webfetch.PageContent{
			Title: "Test",
		},
	}
	provider := newMockAIProvider(&ai.GearGeneration{Title: "Test"}, nil)

	lookup := NewLookup(fetcher, provider)
	lookup.SetTimeout(100 * time.Millisecond)

	result, err := lookup.LookupSpecs(context.Background(), &Info{
		Brand: "DeWalt",
		Model: "DCD771C2",
	})
	if err != nil {
		t.Errorf("Expected no error (graceful degradation), got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result due to timeout, got %v", result)
	}
}

func TestLookup_LookupSpecs_FetchError(t *testing.T) {
	fetcher := &mockWebFetcher{
		err: errors.New("network error"),
	}
	provider := newMockAIProvider(&ai.GearGeneration{Title: "Test"}, nil)

	lookup := NewLookup(fetcher, provider)

	result, err := lookup.LookupSpecs(context.Background(), &Info{
		Brand: "UnknownBrand", // Won't try manufacturer lookup
		Model: "Model123",
	})
	// Should return nil gracefully, not error
	if err != nil {
		t.Errorf("Expected no error (graceful degradation), got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
}

func TestLookup_LookupSpecs_AIExtractionError(t *testing.T) {
	fetcher := &mockWebFetcher{
		pageContent: &webfetch.PageContent{
			Title:    "Test Product",
			BodyText: "Some content",
		},
	}
	provider := newMockAIProvider(nil, errors.New("AI extraction failed"))

	lookup := NewLookup(fetcher, provider)

	result, err := lookup.LookupSpecs(context.Background(), &Info{
		Brand: "UnknownBrand",
		Model: "Model123",
	})
	// Should return nil gracefully
	if err != nil {
		t.Errorf("Expected no error (graceful degradation), got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
}

func TestShouldAttemptLookup(t *testing.T) {
	tests := []struct {
		brand      string
		confidence float64
		want       bool
	}{
		{"DeWalt", 0.9, true},
		{"DeWalt", 0.8, true},
		{"DeWalt", 0.79, false},
		{"DeWalt", 0.5, false},
		{"", 0.9, false},
		{"", 0.5, false},
	}

	for _, tt := range tests {
		got := ShouldAttemptLookup(tt.brand, tt.confidence)
		if got != tt.want {
			t.Errorf("ShouldAttemptLookup(%q, %f) = %v, want %v",
				tt.brand, tt.confidence, got, tt.want)
		}
	}
}

func TestMergeGearGeneration(t *testing.T) {
	t.Run("nil fetched specs returns ai result", func(t *testing.T) {
		aiResult := &ai.GearGeneration{Title: "AI Title"}
		result := MergeGearGeneration(aiResult, nil)
		if result != aiResult {
			t.Error("Expected aiResult when fetchedSpecs is nil")
		}
	})

	t.Run("nil ai result returns fetched specs", func(t *testing.T) {
		fetchedSpecs := &ai.GearGeneration{Title: "Fetched Title"}
		result := MergeGearGeneration(nil, fetchedSpecs)
		if result != fetchedSpecs {
			t.Error("Expected fetchedSpecs when aiResult is nil")
		}
	})

	t.Run("prefers longer description", func(t *testing.T) {
		aiResult := &ai.GearGeneration{
			Title:       "Drill",
			Description: "Short",
		}
		fetchedSpecs := &ai.GearGeneration{
			Title:       "DeWalt Drill",
			Description: "A much longer and more detailed description from the manufacturer",
		}

		result := MergeGearGeneration(aiResult, fetchedSpecs)

		if result.Title != "Drill" {
			t.Errorf("Expected original title, got %q", result.Title)
		}
		if result.Description != fetchedSpecs.Description {
			t.Errorf("Expected longer description, got %q", result.Description)
		}
	})

	t.Run("prefers higher confidence value estimate", func(t *testing.T) {
		aiResult := &ai.GearGeneration{
			Title: "Drill",
			ValueEstimate: &ai.ValueEstimate{
				EstimatedValueUSD: 100.0,
				Confidence:        0.5,
			},
		}
		fetchedSpecs := &ai.GearGeneration{
			Title: "Drill",
			ValueEstimate: &ai.ValueEstimate{
				EstimatedValueUSD: 150.0,
				Confidence:        0.9,
			},
		}

		result := MergeGearGeneration(aiResult, fetchedSpecs)

		if result.ValueEstimate.EstimatedValueUSD != 150.0 {
			t.Errorf("Expected fetched value 150.0, got %f", result.ValueEstimate.EstimatedValueUSD)
		}
	})

	t.Run("uses ai value estimate when fetched has none", func(t *testing.T) {
		aiResult := &ai.GearGeneration{
			Title: "Drill",
			ValueEstimate: &ai.ValueEstimate{
				EstimatedValueUSD: 100.0,
				Confidence:        0.7,
			},
		}
		fetchedSpecs := &ai.GearGeneration{
			Title:       "Drill",
			Description: "Longer description",
		}

		result := MergeGearGeneration(aiResult, fetchedSpecs)

		if result.ValueEstimate == nil {
			t.Fatal("Expected value estimate to be preserved")
		}
		if result.ValueEstimate.EstimatedValueUSD != 100.0 {
			t.Errorf("Expected AI value 100.0, got %f", result.ValueEstimate.EstimatedValueUSD)
		}
	})
}
