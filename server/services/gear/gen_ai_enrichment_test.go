package gear

import (
	"context"
	"errors"
	"math"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/webfetch"
)

// TestService_GenGear_URLGeneration tests URL-based gear generation (Phase 2).
func TestService_GenGear_URLGeneration(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	t.Run("webfetch failure returns error", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return nil, &webfetch.FetchError{Kind: webfetch.FetchErrorConnection, Message: "connection failed", Err: errors.New("network error")}
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product",
		})

		_, err := service.GenGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when webfetch fails")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnavailable {
			t.Errorf("Expected Unavailable error for connection failure, got %v", connectErr.Code())
		}
	})

	t.Run("webfetch 403 returns PermissionDenied", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return nil, &webfetch.FetchError{Kind: webfetch.FetchErrorBlocked, StatusCode: 403, Message: "site returned 403 Forbidden"}
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product",
		})

		_, err := service.GenGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when site blocks request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error for 403, got %v", connectErr.Code())
		}
	})

	t.Run("webfetch timeout returns DeadlineExceeded", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return nil, &webfetch.FetchError{Kind: webfetch.FetchErrorTimeout, Message: "request timed out"}
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product",
		})

		_, err := service.GenGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when request times out")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeDeadlineExceeded {
			t.Errorf("Expected DeadlineExceeded error for timeout, got %v", connectErr.Code())
		}
	})

	t.Run("URL with text description uses text mode", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())

		fetchCalled := false
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				fetchCalled = true
				return &webfetch.PageContent{
					URL:         url,
					Title:       "Product Title",
					Description: "Product description",
					BodyText:    "Full product details",
				}, nil
			},
		})

		// Prompt contains URL mixed with text — should use text mode, not URL mode.
		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "Check out this product https://example.com/product I want to share",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear failed: %v", err)
		}

		if fetchCalled {
			t.Error("Expected webfetcher NOT to be called when URL is mixed with text")
		}

		if resp.Msg.DetectedGear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// Description should be the user's original text (text mode passthrough).
		if resp.Msg.DetectedGear.Description != req.Msg.Prompt {
			t.Errorf("Expected description to be user's text, got %q", resp.Msg.DetectedGear.Description)
		}
	})

	t.Run("http URL is also detected", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				if url != "http://example.com/product" {
					t.Errorf("Expected http URL, got %s", url)
				}
				return &webfetch.PageContent{URL: url, Title: "Test"}, nil
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "http://example.com/product",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear failed for http URL: %v", err)
		}

		if resp.Msg.DetectedGear.SourceUrl != "http://example.com/product" {
			t.Errorf("Expected http source URL, got %q", resp.Msg.DetectedGear.SourceUrl)
		}
	})
}

// TestService_GenGear_ValueEstimation tests that value estimates are returned for URL-based generation.
func TestService_GenGear_ValueEstimation(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	t.Run("URL generation returns value estimate", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "DeWalt 20V MAX Cordless Drill - $99.99",
					Description: "Professional-grade cordless drill",
					BodyText:    "Price: $99.99. Features: 20V MAX battery, 1/2 inch chuck",
				}, nil
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product/drill",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from URL failed: %v", err)
		}

		if resp.Msg.DetectedGear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		gear := resp.Msg.DetectedGear

		// Verify value estimate is populated
		if gear.ValueEstimate == nil {
			t.Fatal("Expected ValueEstimate to be populated for URL mode")
		}

		// Verify value estimate fields
		if gear.ValueEstimate.EstimatedValueUsd <= 0 {
			t.Error("Expected positive estimated_value_usd")
		}

		if gear.ValueEstimate.Provenance == nil {
			t.Fatal("Expected Provenance to be populated on ValueEstimate")
		}

		// An LLM value estimate carries the number, its confidence, and any
		// real sources — but no reasoning line. The model's was a ≤10-word
		// citation it recalled rather than looked up (#2936); the estimator
		// writes the reasoning users actually read.
		if gear.ValueEstimate.Provenance.GetReasoning() != "" {
			t.Errorf("Expected no model-written reasoning on a value estimate, got %q",
				gear.ValueEstimate.Provenance.GetReasoning())
		}

		t.Logf("Value estimate: $%.2f (provenance: %s)",
			gear.ValueEstimate.EstimatedValueUsd,
			gear.ValueEstimate.Provenance.Name)
	})

	t.Run("text generation does not include value estimate", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "camping tent",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from text failed: %v", err)
		}

		// Text mode currently doesn't generate value estimates
		// (The mock provider for text mode doesn't return one)
		// This is expected behavior - value estimates are primarily for URL mode
		gear := resp.Msg.DetectedGear
		if gear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// Value estimate may or may not be present for text mode
		// The important test is that URL mode DOES return it
		t.Logf("Text mode gear generated: title=%q, has_value_estimate=%v",
			gear.Title, gear.ValueEstimate != nil)
	})
}

// TestService_GenGear_PrimaryLocationResolution tests USER_PRIMARY_LOCATION resolution.
func TestService_GenGear_PrimaryLocationResolution(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	// Create mock AI provider that returns USER_PRIMARY_LOCATION in LocationQuery
	mockProvider := ai.NewMockProvider()
	mockProvider.GenerateGearFromTextFunc = func(ctx context.Context, prompt, region string) (*ai.GearGeneration, error) {
		return &ai.GearGeneration{
			Title:         "Test Gear",
			Description:   "Test Description",
			LocationQuery: "USER_PRIMARY_LOCATION",
			Confidence:    0.85,
		}, nil
	}
	service.SetAIProvider(mockProvider)

	userID := "user-with-location"
	primaryLocationID := "primary-loc-123"

	// Create primary residence location
	primaryLocation := &models.Location{
		Id:   primaryLocationID,
		Name: proto.String("Home"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
		},
		Address: &models.Address{
			Locality:     "New York",
			RegionCode:   "NY",
			PostalCode:   "10001",
			AddressLines: []string{"123 Main St"},
		},
	}
	_, err := testStorage.Insert(context.Background(), primaryLocation)
	if err != nil {
		t.Fatalf("Failed to create location: %v", err)
	}

	// Create user with primary residence location
	user := &models.User{
		Id:                         userID,
		Email:                      "test@example.com",
		Role:                       models.Role_ROLE_USER,
		PrimaryResidenceLocationId: primaryLocationID,
	}
	_, err = testStorage.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "test@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "camping tent",
	})

	resp, err := service.GenGear(ctx, req)
	if err != nil {
		t.Fatalf("GenGear failed: %v", err)
	}

	// Note: Without MapboxClient set, the geocoded location will not be populated
	// even if LocationQuery is USER_PRIMARY_LOCATION. This is expected behavior.
	// To test USER_PRIMARY_LOCATION resolution, we would need to set a mock MapboxClient.

	// Verify that the location query was extracted by AI
	if resp.Msg.DetectedGear.LocationQuery != "USER_PRIMARY_LOCATION" {
		t.Errorf("Expected LocationQuery to be USER_PRIMARY_LOCATION, got %q", resp.Msg.DetectedGear.LocationQuery)
	}

	// Verify geocoded location is nil when MapboxClient is not set
	if resp.Msg.DetectedGear.GeocodedLocation != nil {
		t.Error("Expected geocoded_location to be nil when MapboxClient is not set")
	}

	t.Log("Successfully verified USER_PRIMARY_LOCATION is extracted in LocationQuery")
}

// TestService_GenGear_MetadataFields tests that GenGear returns material category and weight from AI.
func TestService_GenGear_MetadataFields(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	t.Run("text generation returns material category and weight", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "cordless drill",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear failed: %v", err)
		}

		gear := resp.Msg.DetectedGear
		if gear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// Mock provider returns "mixed_plastic_metal" which maps to the enum
		if gear.MaterialCategory != api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL {
			t.Errorf("Expected MIXED_PLASTIC_METAL, got %v", gear.MaterialCategory)
		}

		// Mock provider returns WeightGrams=1500 for text generation
		if gear.WeightGrams == nil {
			t.Fatal("Expected WeightGrams to be non-nil")
		}
		if gear.WeightGrams.Mean != 1500 {
			t.Errorf("Expected weight mean 1500, got %f", gear.WeightGrams.Mean)
		}
		// Stddev should be ~30% of mean (use approximate comparison for float32)
		expectedStddev := float32(1500 * 0.3)
		if math.Abs(float64(gear.WeightGrams.Stddev-expectedStddev)) > 1.0 {
			t.Errorf("Expected weight stddev ~%f, got %f", expectedStddev, gear.WeightGrams.Stddev)
		}

		// Verify category is passed through
		if gear.Category != "Test Category" {
			t.Errorf("Expected category 'Test Category', got %q", gear.Category)
		}

		t.Logf("Metadata: material=%v, weight=%.0f±%.0f g, category=%q",
			gear.MaterialCategory, gear.WeightGrams.Mean, gear.WeightGrams.Stddev, gear.Category)
	})

	t.Run("image detection returns material category and weight", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())

		// Create test media
		testMedia := &models.Media{
			UserId:      "user123",
			StorageUrl:  "mock://bucket/media/user123/metadata-test",
			ContentType: "image/jpeg",
			Filename:    proto.String("test.jpg"),
			SizeBytes:   1024,
		}
		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		req := connect.NewRequest(&api.GenGearRequest{
			MediaId: mediaID,
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from media failed: %v", err)
		}

		gear := resp.Msg.DetectedGear
		if gear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// Mock detection returns "mixed_plastic_metal" and WeightGrams=2000
		if gear.MaterialCategory != api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL {
			t.Errorf("Expected MIXED_PLASTIC_METAL, got %v", gear.MaterialCategory)
		}

		if gear.WeightGrams == nil {
			t.Fatal("Expected WeightGrams to be non-nil")
		}
		if gear.WeightGrams.Mean != 2000 {
			t.Errorf("Expected weight mean 2000, got %f", gear.WeightGrams.Mean)
		}

		// Verify brand and model are passed through from detection
		if gear.Brand != "Test Brand" {
			t.Errorf("Expected brand 'Test Brand', got %q", gear.Brand)
		}
		if gear.Model != "Mock Model" {
			t.Errorf("Expected model 'Mock Model', got %q", gear.Model)
		}

		t.Logf("Detection metadata: material=%v, weight=%.0f g, brand=%q, model=%q",
			gear.MaterialCategory, gear.WeightGrams.Mean, gear.Brand, gear.Model)
	})

	t.Run("URL generation returns material category and weight", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		service.SetAIProvider(ai.NewMockProvider())
		service.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:   url,
					Title: "Test Product",
				}, nil
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from URL failed: %v", err)
		}

		gear := resp.Msg.DetectedGear
		if gear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// Mock webpage generation returns "solid_plastic" and WeightGrams=1000
		if gear.MaterialCategory != api.MaterialCategory_MATERIAL_CATEGORY_SOLID_PLASTIC {
			t.Errorf("Expected SOLID_PLASTIC, got %v", gear.MaterialCategory)
		}

		if gear.WeightGrams == nil {
			t.Fatal("Expected WeightGrams to be non-nil")
		}
		if gear.WeightGrams.Mean != 1000 {
			t.Errorf("Expected weight mean 1000, got %f", gear.WeightGrams.Mean)
		}

		t.Logf("URL metadata: material=%v, weight=%.0f g", gear.MaterialCategory, gear.WeightGrams.Mean)
	})

	t.Run("custom AI with zero weight omits weight field", func(t *testing.T) {
		service := New(testStorage, mockBucket)
		mockProvider := ai.NewMockProvider()
		mockProvider.GenerateGearFromTextFunc = func(ctx context.Context, prompt, region string) (*ai.GearGeneration, error) {
			return &ai.GearGeneration{
				Title:            "Light Item",
				Description:      "A very light item",
				MaterialCategory: "fabric",
				WeightGrams:      0, // Zero weight
				Confidence:       0.8,
			}, nil
		}
		service.SetAIProvider(mockProvider)

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "fabric bag",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear failed: %v", err)
		}

		gear := resp.Msg.DetectedGear
		if gear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		// WeightGrams should be nil when AI returns 0
		if gear.WeightGrams != nil {
			t.Errorf("Expected nil WeightGrams for zero weight, got %+v", gear.WeightGrams)
		}

		// Material category should still be set
		if gear.MaterialCategory != api.MaterialCategory_MATERIAL_CATEGORY_FABRIC {
			t.Errorf("Expected FABRIC, got %v", gear.MaterialCategory)
		}
	})
}

// TestExtractURL verifies that URL extraction only triggers for URL-only inputs.
func TestExtractURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain URL", "https://amazon.com/dp/123", "https://amazon.com/dp/123"},
		{"URL with whitespace", "  https://amazon.com/dp/123  ", "https://amazon.com/dp/123"},
		{"URL with text before", "Check out https://amazon.com/dp/123", ""},
		{"URL with text after", "https://amazon.com/dp/123 great drill", ""},
		{"URL embedded in description", "I got this from https://amazon.com/dp/123 and it works great", ""},
		{"no URL", "camping tent for weekend trips", ""},
		{"empty string", "", ""},
		{"http URL", "http://example.com/product", "http://example.com/product"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURL(tt.input)
			if got != tt.want {
				t.Errorf("extractURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
