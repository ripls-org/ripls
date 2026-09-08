package ai

import (
	"context"
	"testing"
)

func TestNewSingleProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty API key for OpenAI", func(t *testing.T) {
		_, err := NewSingleProvider(ctx, ProviderConfig{
			Type:   ProviderTypeOpenAI,
			APIKey: "",
		})
		if err == nil {
			t.Error("Expected error for empty API key")
		}
	})

	t.Run("rejects empty API key for Anthropic", func(t *testing.T) {
		_, err := NewSingleProvider(ctx, ProviderConfig{
			Type:   ProviderTypeAnthropic,
			APIKey: "",
		})
		if err == nil {
			t.Error("Expected error for empty API key")
		}
	})

	t.Run("rejects unknown provider type", func(t *testing.T) {
		_, err := NewSingleProvider(ctx, ProviderConfig{
			Type:   "unknown",
			APIKey: "test-key",
		})
		if err == nil {
			t.Error("Expected error for unknown provider type")
		}
	})
}

func TestNewWeightedProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty configs", func(t *testing.T) {
		_, err := NewWeightedProvider(ctx, []WeightedProviderConfig{})
		if err == nil {
			t.Error("Expected error for empty configs")
		}
	})
}

func TestNewFallbackProviderFromConfigs(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects invalid primary config", func(t *testing.T) {
		_, err := NewFallbackProviderFromConfigs(ctx, ProviderConfig{
			Type:   ProviderTypeOpenAI,
			APIKey: "",
		}, nil)
		if err == nil {
			t.Error("Expected error for invalid primary config")
		}
	})
}

func TestNewAllProvidersWithFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil with no providers configured", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if provider != nil {
			t.Error("Expected nil provider when no API keys configured")
		}
	})

	t.Run("returns nil when only OpenAI configured", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{
			OpenAIAPIKey: "test-key",
		})
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		// OpenAI alone is not enough - need Anthropic or Gemini as primary
		if provider != nil {
			t.Errorf("Expected nil provider (OpenAI is tertiary fallback only), got %T", provider)
		}
	})

	t.Run("creates single Anthropic provider", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{
			AnthropicAPIKey: "test-key",
		})
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		if provider == nil {
			t.Error("Expected non-nil provider")
		}
		if _, ok := provider.(*AnthropicProvider); !ok {
			t.Errorf("Expected *AnthropicProvider, got %T", provider)
		}
	})

	t.Run("creates single Anthropic provider without load balancing", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{
			AnthropicAPIKey: "test-key-2",
		})
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		if provider == nil {
			t.Error("Expected non-nil provider")
		}
		// Single Anthropic provider returns directly without load balancing
		if _, ok := provider.(*AnthropicProvider); !ok {
			t.Errorf("Expected *AnthropicProvider, got %T", provider)
		}
	})

	t.Run("applies model overrides", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{
			AnthropicAPIKey:        "test-key",
			AnthropicModelOverride: "claude-opus-4-5",
		})
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		anthropic, ok := provider.(*AnthropicProvider)
		if !ok {
			t.Fatalf("Expected *AnthropicProvider, got %T", provider)
		}
		if anthropic.model != "claude-opus-4-5" {
			t.Errorf("Expected model 'claude-opus-4-5', got '%s'", anthropic.model)
		}
	})

	t.Run("uses default model when no override", func(t *testing.T) {
		provider, err := NewAllProvidersWithFallback(ctx, AllProvidersConfig{
			AnthropicAPIKey: "test-key",
		})
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		anthropic, ok := provider.(*AnthropicProvider)
		if !ok {
			t.Fatalf("Expected *AnthropicProvider, got %T", provider)
		}
		// Check default model is used
		if anthropic.model != "claude-haiku-4-5" {
			t.Errorf("Expected default model 'claude-haiku-4-5', got '%s'", anthropic.model)
		}
	})
}

func TestProviderTypeConstants(t *testing.T) {
	// Verify provider type constants have expected values
	if ProviderTypeGemini != "gemini" {
		t.Errorf("Expected ProviderTypeGemini='gemini', got '%s'", ProviderTypeGemini)
	}
	if ProviderTypeOpenAI != "openai" {
		t.Errorf("Expected ProviderTypeOpenAI='openai', got '%s'", ProviderTypeOpenAI)
	}
	if ProviderTypeAnthropic != "anthropic" {
		t.Errorf("Expected ProviderTypeAnthropic='anthropic', got '%s'", ProviderTypeAnthropic)
	}
}
