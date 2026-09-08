package ai

import (
	"context"
	"math"
	"os"
	"testing"
)

// TestProviderInitialization tests that each provider initializes correctly.
func TestProviderInitialization(t *testing.T) {
	ctx := context.Background()

	t.Run("GeminiProvider", func(t *testing.T) {
		// Skip if no GCP credentials available (requires ADC for Vertex AI)
		if os.Getenv("VERTEX_AI_PROJECT") == "" {
			t.Skip("Skipping Gemini tests: VERTEX_AI_PROJECT not set (requires GCP credentials)")
		}
		project := os.Getenv("VERTEX_AI_PROJECT")

		t.Run("creates provider with default model", func(t *testing.T) {
			provider, err := NewGeminiProvider(ctx, "", project, "us-central1", DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider == nil {
				t.Fatal("Expected non-nil provider")
			}
			if provider.model != "vertexai/gemini-2.5-flash-lite" {
				t.Errorf("Expected default model 'vertexai/gemini-2.5-flash-lite', got %s", provider.model)
			}
			if provider.g == nil {
				t.Error("Expected non-nil genkit instance")
			}
		})

		t.Run("creates provider with custom model", func(t *testing.T) {
			customModel := "vertexai/gemini-1.5-pro"
			provider, err := NewGeminiProvider(ctx, customModel, project, "us-central1", DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider.model != customModel {
				t.Errorf("Expected model %s, got %s", customModel, provider.model)
			}
		})
	})

	t.Run("OpenAIProvider", func(t *testing.T) {
		t.Run("creates provider with default model", func(t *testing.T) {
			provider, err := NewOpenAIProvider("test-api-key", "", DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider == nil {
				t.Fatal("Expected non-nil provider")
			}
			if provider.model != "gpt-5-mini" {
				t.Errorf("Expected default model 'gpt-5-mini', got %s", provider.model)
			}
			if provider.client == nil {
				t.Error("Expected non-nil client")
			}
		})

		t.Run("creates provider with custom model", func(t *testing.T) {
			customModel := "gpt-4o-mini"
			provider, err := NewOpenAIProvider("test-api-key", customModel, DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider.model != customModel {
				t.Errorf("Expected model %s, got %s", customModel, provider.model)
			}
		})

		t.Run("rejects missing API key", func(t *testing.T) {
			_, err := NewOpenAIProvider("", "", DefaultTemperature)
			if err == nil {
				t.Error("Expected error for missing API key")
			}
		})
	})

	t.Run("AnthropicProvider", func(t *testing.T) {
		t.Run("creates provider with default model", func(t *testing.T) {
			provider, err := NewAnthropicProvider("test-api-key", "", DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider == nil {
				t.Fatal("Expected non-nil provider")
			}
			if provider.model != "claude-haiku-4-5" {
				t.Errorf("Expected default model 'claude-haiku-4-5', got %s", provider.model)
			}
			if provider.client == nil {
				t.Error("Expected non-nil client")
			}
		})

		t.Run("creates provider with custom model", func(t *testing.T) {
			customModel := "claude-haiku-4-5-20251001"
			provider, err := NewAnthropicProvider("test-api-key", customModel, DefaultTemperature)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}
			if provider.model != customModel {
				t.Errorf("Expected model %s, got %s", customModel, provider.model)
			}
		})

		t.Run("rejects missing API key", func(t *testing.T) {
			_, err := NewAnthropicProvider("", "", DefaultTemperature)
			if err == nil {
				t.Error("Expected error for missing API key")
			}
		})
	})
}

// TestNormalizeConfidence tests the confidence normalization function.
func TestNormalizeConfidence(t *testing.T) {
	tests := []struct {
		name     string
		input    float32
		expected float32
	}{
		{"zero stays zero", 0.0, 0.0},
		{"decimal stays decimal", 0.92, 0.92},
		{"one stays one", 1.0, 1.0},
		{"percentage 50 becomes 0.5", 50.0, 0.5},
		{"percentage 92 becomes 0.92", 92.0, 0.92},
		{"percentage 100 becomes 1.0", 100.0, 1.0},
		{"value just over 1 normalizes", 1.5, 0.015},
		{"value over 100 clamps to 1.0", 150.0, 1.0},
		{"very large value clamps to 1.0", 900000000.0, 1.0},
		{"negative clamps to 0.0", -0.5, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeConfidence(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeConfidence(%f) = %f, want %f", tt.input, result, tt.expected)
			}
		})
	}
}

// TestRobustFloat32UnmarshalJSON verifies that robustFloat32 tolerates
// out-of-range JSON numbers instead of returning an unmarshal error.
func TestRobustFloat32UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want float32
	}{
		{"normal decimal", `0.9`, 0.9},
		{"zero", `0`, 0},
		{"one", `1`, 1},
		{"negative", `-0.5`, -0.5},
		// Huge positive exponent — the value that caused the CI failure.
		{"gigantic positive exponent", `0.9e9999999999999999999999`, 1.0},
		{"gigantic negative exponent", `-0.9e9999999999999999999999`, 0.0},
		// Values in float64 range but outside float32 range overflow to ±Inf;
		// NormalizeConfidence clamps those to [0, 1].
		{"large but float64-range", `1e39`, float32(math.Inf(1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r robustFloat32
			if err := r.UnmarshalJSON([]byte(tt.json)); err != nil {
				t.Fatalf("UnmarshalJSON(%s) returned error: %v", tt.json, err)
			}
			got := NormalizeConfidence(float32(r))
			wantN := NormalizeConfidence(tt.want)
			if got != wantN {
				t.Errorf("NormalizeConfidence(robustFloat32(%s)) = %v, want %v", tt.json, got, wantN)
			}
		})
	}
}

// TestProviderValidation tests input validation for all providers.
func TestProviderValidation(t *testing.T) {
	ctx := context.Background()

	// Create test providers
	providers := map[string]Provider{}

	// Only add Gemini if GCP credentials are available
	if project := os.Getenv("VERTEX_AI_PROJECT"); project != "" {
		geminiProvider, err := NewGeminiProvider(ctx, "", project, "us-central1", DefaultTemperature)
		if err != nil {
			t.Fatalf("Failed to create Gemini provider: %v", err)
		}
		providers["Gemini"] = geminiProvider
	}

	openaiProvider, err := NewOpenAIProvider("test-api-key", "", DefaultTemperature)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}
	providers["OpenAI"] = openaiProvider

	anthropicProvider, err := NewAnthropicProvider("test-api-key", "", DefaultTemperature)
	if err != nil {
		t.Fatalf("Failed to create Anthropic provider: %v", err)
	}
	providers["Anthropic"] = anthropicProvider

	// Run validation tests for each provider
	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			runProviderValidationTests(t, provider)
		})
	}
}

// runProviderValidationTests runs common validation tests against any provider.
func runProviderValidationTests(t *testing.T, provider Provider) {
	ctx := context.Background()

	t.Run("DetectGearInImage validation", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *DetectionImage
			wantErr string
		}{
			{
				name:    "no image data or URL",
				req:     &DetectionImage{MimeType: "image/jpeg"},
				wantErr: "either ImageData or ImageURL must be set",
			},
			{
				name: "both image data and URL",
				req: &DetectionImage{
					ImageData: []byte("test"),
					ImageURL:  "https://example.com/image.jpg",
					MimeType:  "image/jpeg",
				},
				wantErr: "only one of ImageData or ImageURL can be set",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := provider.DetectGearInImage(ctx, tt.req)
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				if err.Error() != tt.wantErr {
					t.Errorf("Error = %v, want %v", err.Error(), tt.wantErr)
				}
			})
		}
	})

	t.Run("GenerateCommunityContent validation", func(t *testing.T) {
		_, err := provider.GenerateCommunityContent(ctx, "", "")
		if err == nil {
			t.Error("Expected error for empty prompt but got nil")
			return
		}
		if err.Error() != "prompt cannot be empty" {
			t.Errorf("Error = %v, want 'prompt cannot be empty'", err.Error())
		}
	})

	t.Run("GenerateRequestContent validation", func(t *testing.T) {
		_, err := provider.GenerateRequestContent(ctx, "", "")
		if err == nil {
			t.Error("Expected error for empty prompt but got nil")
			return
		}
		if err.Error() != "prompt cannot be empty" {
			t.Errorf("Error = %v, want 'prompt cannot be empty'", err.Error())
		}
	})

	t.Run("GenerateExperienceFromText validation", func(t *testing.T) {
		_, err := provider.GenerateExperienceFromText(ctx, "", "", "")
		if err == nil {
			t.Error("Expected error for empty prompt but got nil")
			return
		}
		if err.Error() != "prompt cannot be empty" {
			t.Errorf("Error = %v, want 'prompt cannot be empty'", err.Error())
		}
	})

	t.Run("GenerateConversationSummary validation", func(t *testing.T) {
		_, err := provider.GenerateConversationSummary(ctx, []ConversationMessage{}, "", "", SummaryStyleProgress)
		if err == nil {
			t.Error("Expected error for empty messages but got nil")
			return
		}
		if err.Error() != "messages cannot be empty" {
			t.Errorf("Error = %v, want 'messages cannot be empty'", err.Error())
		}
	})
}

// TestPromptBuilders tests the prompt building functions.
func TestPromptBuilders(t *testing.T) {
	t.Run("buildGearDetectionPrompt", func(t *testing.T) {
		prompt := buildGearDetectionPrompt()
		if prompt == "" {
			t.Error("Expected non-empty prompt")
		}

		// Verify the one-sentence description framing (#2225) and that
		// the structured output fields are still requested.
		expectedTerms := []string{
			"One crisp sentence",
			"primary use",
			"model",
			"value_estimate",
			"estimated_value_usd",
		}
		for _, term := range expectedTerms {
			if !containsString(prompt, term) {
				t.Errorf("Expected prompt to contain %q for one-sentence framing", term)
			}
		}

		// Verify marketing language and photo-narration framing are
		// explicitly disallowed — these are the patterns #2225 was
		// filed to remove.
		bannedTerms := []string{
			"appealing to potential members",
			"Potential uses",
			"Factual specs you can observe",
		}
		for _, term := range bannedTerms {
			if containsString(prompt, term) {
				t.Errorf("Prompt should NOT contain banned framing %q", term)
			}
		}
	})

	t.Run("buildGearGenerationPrompt", func(t *testing.T) {
		prompt := buildGearGenerationPrompt("DeWalt drill", "Boulder, CO")
		if prompt == "" {
			t.Error("Expected non-empty prompt")
		}

		// Verify user input and region are included
		if !containsString(prompt, "DeWalt drill") {
			t.Error("Expected prompt to contain user input")
		}
		if !containsString(prompt, "Boulder, CO") {
			t.Error("Expected prompt to contain region")
		}

		// Verify factual focus - should contain specification-focused
		// instructions. MODEL is intentionally absent: the text-mode wire
		// schema dropped it because nothing consumes it (#1265).
		expectedTerms := []string{
			"MATERIAL_CATEGORY",
			"VALUE_ESTIMATE",
			"estimated_value_usd",
		}
		for _, term := range expectedTerms {
			if !containsString(prompt, term) {
				t.Errorf("Expected prompt to contain %q for factual focus", term)
			}
		}

		// Verify marketing language removed
		marketingTerms := []string{
			"appealing gear listing",
			"Potential benefits to borrowers",
		}
		for _, term := range marketingTerms {
			if containsString(prompt, term) {
				t.Errorf("Prompt should NOT contain marketing language %q", term)
			}
		}
	})

	t.Run("buildCommunityGenerationPrompt", func(t *testing.T) {
		tests := []struct {
			name           string
			userPrompt     string
			region         string
			expectContains []string
		}{
			{
				name:       "basic prompt without region",
				userPrompt: "A community for sharing tools",
				region:     "",
				expectContains: []string{
					"A community for sharing tools",
					"SEARCH_KEYWORDS (3-5 terms)",
					"background image",
				},
			},
			{
				name:       "prompt with region",
				userPrompt: "A community for sharing baby gear",
				region:     "Boulder, CO",
				expectContains: []string{
					"A community for sharing baby gear",
					"Boulder, CO area",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				prompt := buildCommunityGenerationPrompt(tt.userPrompt, tt.region)
				if prompt == "" {
					t.Fatal("Expected non-empty prompt")
				}
				for _, expected := range tt.expectContains {
					if !containsString(prompt, expected) {
						t.Errorf("Expected prompt to contain %q, but it did not", expected)
					}
				}
			})
		}
	})

	t.Run("buildRequestGenerationPrompt", func(t *testing.T) {
		prompt := buildRequestGenerationPrompt("Need a drill", "Denver")
		if prompt == "" {
			t.Error("Expected non-empty prompt")
		}
		if !containsString(prompt, "Need a drill") {
			t.Error("Expected prompt to contain user input")
		}
		if !containsString(prompt, "Denver") {
			t.Error("Expected prompt to contain region")
		}
	})

	t.Run("buildConversationSummaryPrompt", func(t *testing.T) {
		messages := []ConversationMessage{
			{SenderName: "Alice", Text: "Hello", SentAtUnixSec: 1000},
			{SenderName: "Bob", Text: "Hi there", SentAtUnixSec: 1001},
		}
		// Test progress style without request context
		progressPrompt := buildConversationSummaryPrompt(messages, "", "", SummaryStyleProgress)
		if progressPrompt == "" {
			t.Error("Expected non-empty prompt")
		}
		if !containsString(progressPrompt, "Alice") {
			t.Error("Expected prompt to contain sender name")
		}
		if !containsString(progressPrompt, "Hello") {
			t.Error("Expected prompt to contain message text")
		}
		if !containsString(progressPrompt, "third person") {
			t.Error("Expected progress prompt to mention third person")
		}

		// Test completion style
		completionPrompt := buildConversationSummaryPrompt(messages, "", "", SummaryStyleCompletion)
		if !containsString(completionPrompt, "first person") {
			t.Error("Expected completion prompt to mention first person")
		}
		if !containsString(completionPrompt, "gratitude") {
			t.Error("Expected completion prompt to mention gratitude")
		}

		// Test with request context
		promptWithContext := buildConversationSummaryPrompt(messages, "Need camping tent", "Looking to borrow a tent for the weekend", SummaryStyleProgress)
		if !containsString(promptWithContext, "REQUEST CONTEXT") {
			t.Error("Expected prompt with context to include REQUEST CONTEXT section")
		}
		if !containsString(promptWithContext, "Need camping tent") {
			t.Error("Expected prompt to contain request title")
		}
		if !containsString(promptWithContext, "Looking to borrow a tent for the weekend") {
			t.Error("Expected prompt to contain request description")
		}
	})
}

// TestGenerateSchema tests schema generation for structured outputs.
func TestGenerateSchema(t *testing.T) {
	type testStruct struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	schema := generateSchema[testStruct]()
	if schema == nil {
		t.Error("generateSchema returned nil")
	}
}

// containsString checks if haystack contains needle (case-sensitive).
func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || indexString(haystack, needle) >= 0)
}

// indexString returns the index of needle in haystack, or -1 if not found.
func indexString(haystack, needle string) int {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
