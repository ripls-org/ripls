//go:build integration && ai_providers

package ai

import (
	"context"
	"os"
	"testing"
)

// providerConfig holds configuration for creating a test provider.
type providerConfig struct {
	name       string
	envKey     string
	createFunc func(envValue string) (Provider, error)
}

// getProviderConfigs returns configurations for all available providers.
func getProviderConfigs(ctx context.Context) []providerConfig {
	return []providerConfig{
		{
			name:   "Gemini",
			envKey: "VERTEX_AI_PROJECT",
			createFunc: func(project string) (Provider, error) {
				return NewGeminiProvider(ctx, "vertexai/gemini-2.5-flash", project, "us-central1", DefaultTemperature)
			},
		},
		{
			name:   "OpenAI",
			envKey: "OPENAI_API_KEY",
			createFunc: func(apiKey string) (Provider, error) {
				return NewOpenAIProvider(apiKey, "gpt-5-mini", DefaultTemperature)
			},
		},
		{
			name:   "Anthropic",
			envKey: "ANTHROPIC_API_KEY",
			createFunc: func(apiKey string) (Provider, error) {
				return NewAnthropicProvider(apiKey, "claude-haiku-4-5", DefaultTemperature)
			},
		},
	}
}

// TestProviderIntegration runs integration tests for all configured providers.
// Each provider is tested with all four AI methods.
func TestProviderIntegration(t *testing.T) {
	ctx := context.Background()

	// Load test image
	testImagePath := "../test_data/rocky_talkie.jpg"
	imageData, err := os.ReadFile(testImagePath)
	if err != nil {
		t.Fatalf("Test image not found at %s: %v", testImagePath, err)
	}

	// Run tests for each provider. Providers are independent (separate API
	// keys, independent rate limits) so subtests run concurrently.
	for _, cfg := range getProviderConfigs(ctx) {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()

			envValue := os.Getenv(cfg.envKey)
			if envValue == "" {
				t.Skipf("Skipping %s integration test: %s not set", cfg.name, cfg.envKey)
			}

			provider, err := cfg.createFunc(envValue)
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", cfg.name, err)
			}

			runAllIntegrationTests(t, provider, imageData)
		})
	}
}

// runAllIntegrationTests runs all four AI method tests against a provider.
func runAllIntegrationTests(t *testing.T, provider Provider, imageData []byte) {
	ctx := context.Background()

	// Each method is independent within a provider; run them concurrently
	// so one slow provider doesn't serialize the whole matrix. Per-method
	// subtests inside still run sequentially within each method (e.g. the
	// GenerateGearFromText test cases), which keeps per-provider rate
	// pressure bounded.

	// Test 1: DetectGearInImage (with metadata fields)
	t.Run("DetectGearInImage", func(t *testing.T) {
		t.Parallel()
		detections, err := retryOnRateLimit(t, "DetectGearInImage", func() (*GearDetection, error) {
			return provider.DetectGearInImage(ctx, &DetectionImage{
				ImageData: imageData,
				MimeType:  "image/jpeg",
			})
		})
		if err != nil {
			t.Fatalf("DetectGearInImage failed: %v", err)
		}

		// Validate results
		if detections == nil {
			t.Fatal("Expected gear item to be detected, got nil")
		}

		// Log results
		t.Logf("Detected gear: %s (confidence: %.2f)", detections.Title, detections.Confidence)
		t.Logf("  Category: %s", detections.Category)
		t.Logf("  Brand: %s", detections.Brand)
		t.Logf("  Model: %s", detections.Model)
		t.Logf("  Description: %s", detections.Description)
		t.Logf("  MaterialCategory: %s", detections.MaterialCategory)
		t.Logf("  WeightGrams: %.0f", detections.WeightGrams)

		if detections.Confidence < 0.0 || detections.Confidence > 1.0 {
			t.Errorf("Invalid confidence score: %f", detections.Confidence)
		}
		if detections.Title == "" {
			t.Error("Expected non-empty title")
		}
		if detections.Category == "" {
			t.Error("Expected non-empty category")
		}

		// Verify new metadata fields are populated
		if detections.MaterialCategory == "" {
			t.Error("Expected non-empty material_category from image detection")
		}
		if detections.WeightGrams <= 0 {
			t.Error("Expected positive weight_grams from image detection")
		}
	})

	// Test 1b: GenerateGearFromText (with metadata fields)
	t.Run("GenerateGearFromText", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name   string
			prompt string
			region string
		}{
			{
				name:   "cordless_drill",
				prompt: "DeWalt 20V MAX cordless drill with lithium battery",
				region: "Denver, CO",
			},
			{
				name:   "camping_tent",
				prompt: "4-person backpacking tent",
				region: "Boulder, CO",
			},
			{
				name:   "hand_saw",
				prompt: "Japanese pull saw for woodworking",
				region: "",
			},
			{
				name:   "stand_mixer",
				prompt: "KitchenAid Artisan stand mixer",
				region: "Austin, TX",
			},
			{
				name:   "lawn_mower",
				prompt: "gas powered push lawn mower",
				region: "",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				generation, err := retryOnRateLimit(t, "GenerateGearFromText", func() (*GearGeneration, error) {
					return provider.GenerateGearFromText(ctx, tc.prompt, tc.region)
				})
				if err != nil {
					t.Fatalf("GenerateGearFromText failed: %v", err)
				}

				if generation == nil {
					t.Fatal("Expected non-nil generation")
				}

				// Log all fields
				t.Logf("Generated gear from %q:", tc.prompt)
				t.Logf("  Title: %s", generation.Title)
				t.Logf("  Category: %s", generation.Category)
				t.Logf("  Brand: %s", generation.Brand)
				t.Logf("  MaterialCategory: %s", generation.MaterialCategory)
				t.Logf("  WeightGrams: %.0f", generation.WeightGrams)
				t.Logf("  Confidence: %.2f", generation.Confidence)
				if generation.ValueEstimate != nil {
					t.Logf("  ValueEstimate: $%.2f (confidence: %.2f)",
						generation.ValueEstimate.EstimatedValueUSD, generation.ValueEstimate.Confidence)
				}

				// Validate basic fields
				if generation.Title == "" {
					t.Error("Expected non-empty title")
				}
				if generation.Category == "" {
					t.Error("Expected non-empty category")
				}
				if generation.Confidence < 0.0 || generation.Confidence > 1.0 {
					t.Errorf("Invalid confidence: %f", generation.Confidence)
				}

				// Validate new metadata fields
				if generation.MaterialCategory == "" {
					t.Error("Expected non-empty material_category")
				}
				if generation.WeightGrams <= 0 {
					t.Error("Expected positive weight_grams")
				}

				// Validate material_category is from the allowed enum set
				validCategories := map[string]bool{
					"solid_metal": true, "solid_plastic": true,
					"mixed_plastic_metal": true, "mixed_wood_metal": true,
					"mixed_wood_plastic": true, "wood": true,
					"aluminum": true, "fabric": true,
					"cordless_power_tool": true, "corded_power_tool": true,
					"petrol_tool": true, "electronics_small": true,
				}
				if !validCategories[generation.MaterialCategory] {
					t.Errorf("Invalid material_category %q - not in allowed set", generation.MaterialCategory)
				}
			})
		}
	})

	// Test 2: GenerateCommunityContent
	t.Run("GenerateCommunityContent", func(t *testing.T) {
		t.Parallel()
		generation, err := retryOnRateLimit(t, "GenerateCommunityContent", func() (*CommunityGeneration, error) {
			return provider.GenerateCommunityContent(ctx,
				"A group for sharing outdoor camping and hiking gear in the neighborhood",
				"Boulder, CO",
			)
		})
		if err != nil {
			t.Fatalf("GenerateCommunityContent failed: %v", err)
		}

		// Log results
		t.Logf("Generated community content:")
		t.Logf("  Keywords: %v", generation.SearchKeywords)

		// Validate results — community generation produces only search keywords.
		if len(generation.SearchKeywords) == 0 {
			t.Error("Expected at least one search keyword")
		}
	})

	// Test 3: GenerateRequestContent
	t.Run("GenerateRequestContent", func(t *testing.T) {
		t.Parallel()
		generation, err := retryOnRateLimit(t, "GenerateRequestContent", func() (*RequestGeneration, error) {
			return provider.GenerateRequestContent(ctx,
				"I need to borrow a power drill for a weekend project building shelves",
				"Denver, CO",
			)
		})
		if err != nil {
			t.Fatalf("GenerateRequestContent failed: %v", err)
		}

		// Log results
		t.Logf("Generated request content:")
		t.Logf("  Title: %s", generation.Title)
		t.Logf("  Description: %s", generation.Description)
		t.Logf("  Keywords: %v", generation.SearchKeywords)
		t.Logf("  Confidence: %.2f", generation.Confidence)

		// Validate results
		if generation.Title == "" {
			t.Error("Expected non-empty title")
		}
		if len(generation.Title) > 50 {
			t.Errorf("Title too long: %d chars (max 50)", len(generation.Title))
		}
		// Description is no longer generated by providers in text mode;
		// the service layer sets it to the user's original text.
		if len(generation.SearchKeywords) == 0 {
			t.Error("Expected at least one search keyword")
		}
		if generation.Confidence < 0.0 || generation.Confidence > 1.0 {
			t.Errorf("Invalid confidence score: %f", generation.Confidence)
		}
	})

	// Test 4: GenerateExperienceFromText
	t.Run("GenerateExperienceFromText", func(t *testing.T) {
		t.Parallel()
		generation, err := retryOnRateLimit(t, "GenerateExperienceFromText", func() (*ExperienceGeneration, error) {
			return provider.GenerateExperienceFromText(ctx,
				"hiking tomorrow morning at Chautauqua",
				"Boulder, CO",
				"2025-11-30T15:00:00-07:00",
			)
		})
		if err != nil {
			t.Fatalf("GenerateExperienceFromText failed: %v", err)
		}

		// Log results
		t.Logf("Generated experience content:")
		t.Logf("  Title: %s", generation.Title)
		t.Logf("  Description: %s", generation.Description)
		t.Logf("  Keywords: %v", generation.SearchKeywords)
		t.Logf("  Confidence: %.2f", generation.Confidence)
		t.Logf("  Date: %s", generation.Date)
		t.Logf("  Time: %s", generation.Time)
		t.Logf("  TimeConfidence: %s", generation.TimeConfidence)
		t.Logf("  LocationQuery: %s", generation.LocationQuery)

		// Validate results
		if generation.Title == "" {
			t.Error("Expected non-empty title")
		}
		if len(generation.Title) > 50 {
			t.Errorf("Title too long: %d chars (max 50)", len(generation.Title))
		}
		// Description is no longer generated by providers in text mode;
		// the service layer sets it to the user's original text.
		if len(generation.SearchKeywords) == 0 {
			t.Error("Expected at least one search keyword")
		}
		if generation.Confidence < 0.0 || generation.Confidence > 1.0 {
			t.Errorf("Invalid confidence score: %f", generation.Confidence)
		}

		// Validate time extraction fields
		if generation.Date == "" {
			t.Error("Expected date to be extracted (should be '2025-12-01' for 'tomorrow')")
		}
		if generation.Time == "" {
			t.Error("Expected time to be extracted (should be '08:00' for 'morning')")
		}
		if generation.TimeConfidence == "" {
			t.Error("Expected time confidence to be set")
		}
		if generation.LocationQuery == "" {
			t.Error("Expected location query to be extracted (should be 'Chautauqua')")
		}
	})

	// Test 5: GenerateConversationSummary
	t.Run("GenerateConversationSummary", func(t *testing.T) {
		t.Parallel()
		messages := []ConversationMessage{
			{SenderName: "Alice", Text: "Hi! I saw you have a camping tent available. Is it still free this weekend?", SentAtUnixSec: 1700000000},
			{SenderName: "Bob", Text: "Yes it is! It's a 4-person tent, great for families. When would you need it?", SentAtUnixSec: 1700000100},
			{SenderName: "Alice", Text: "Perfect! We're planning a trip to the mountains Saturday through Monday. Could I pick it up Friday evening?", SentAtUnixSec: 1700000200},
			{SenderName: "Bob", Text: "Friday evening works great. I'll have it packed and ready. Just bring it back clean and dry!", SentAtUnixSec: 1700000300},
			{SenderName: "Alice", Text: "Will do! Thank you so much. See you Friday around 6pm?", SentAtUnixSec: 1700000400},
		}

		summary, err := retryOnRateLimit(t, "GenerateConversationSummary", func() (*ConversationSummary, error) {
			return provider.GenerateConversationSummary(
				ctx,
				messages,
				"Need camping tent for weekend",
				"Looking to borrow a tent for a family camping trip this weekend",
				SummaryStyleProgress,
			)
		})
		if err != nil {
			t.Fatalf("GenerateConversationSummary failed: %v", err)
		}

		// Log results
		t.Logf("Generated conversation summary:")
		t.Logf("  Summary: %s", summary.Summary)

		// Validate results
		if summary.Summary == "" {
			t.Error("Expected non-empty summary")
		}
	})
}
