// provider_factory.go provides factory functions for creating composed AI providers.
// Use these functions to initialize the AI provider system from command-line flags.
//
// The recommended function for most users is NewAllProvidersWithFallback,
// which creates a production-ready provider with load balancing and automatic fallback.
//
// Example usage:
//
//	// Recommended: All providers with weighted load balancing and fallback
//	provider, _ := ai.NewAllProvidersWithFallback(ctx, ai.AllProvidersConfig{
//	    OpenAIAPIKey:    openaiKey,
//	    AnthropicAPIKey: anthropicKey,
//	    VertexAIProject: gcpProject,
//	})
//
//	// Single provider
//	provider, _ := ai.NewSingleProvider(ctx, ai.ProviderConfig{
//	    Type: ai.ProviderTypeOpenAI,
//	    APIKey: openaiKey,
//	    Model: "gpt-4o",
//	})
package ai

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// ProviderType identifies the AI provider implementation.
type ProviderType string

const (
	ProviderTypeGemini    ProviderType = "gemini"
	ProviderTypeOpenAI    ProviderType = "openai"
	ProviderTypeAnthropic ProviderType = "anthropic"
)

// DefaultTemperature is the sampling temperature used for Gen* extraction calls
// when no per-provider override is supplied. 0.2 is low enough to collapse
// most run-to-run output variance (see #1216) while still leaving enough
// headroom for the model to escape a wrong deterministic answer at temperature=0.
const DefaultTemperature = 0.2

// ProviderConfig holds configuration for a single AI provider.
type ProviderConfig struct {
	Type   ProviderType
	APIKey string // Used by OpenAI and Anthropic
	Model  string

	// Temperature is the sampling temperature for Gen* calls. Zero falls
	// back to DefaultTemperature.
	Temperature float64

	// Vertex AI fields (for Gemini)
	VertexAIProject  string
	VertexAILocation string
}

// WeightedProviderConfig pairs a provider configuration with its load balancing weight.
type WeightedProviderConfig struct {
	Config ProviderConfig
	Weight float64
}

// AllProvidersConfig holds API keys and optional model overrides for all providers.
type AllProvidersConfig struct {
	// API keys (required for OpenAI and Anthropic)
	OpenAIAPIKey    string
	AnthropicAPIKey string

	// Model overrides (empty string uses default)
	GeminiModelOverride    string
	OpenAIModelOverride    string
	AnthropicModelOverride string

	// Per-provider sampling temperature for Gen* calls. Zero falls back to
	// DefaultTemperature. Each provider has its own value so they can be
	// tuned independently — the per-call variance characteristics differ.
	GeminiTemperature    float64
	OpenAITemperature    float64
	AnthropicTemperature float64

	// Vertex AI (for Gemini) - required for Gemini provider
	VertexAIProject  string
	VertexAILocation string
}

// resolveTemperature returns t if non-zero, otherwise DefaultTemperature.
func resolveTemperature(t float64) float64 {
	if t == 0 {
		return DefaultTemperature
	}
	return t
}

// NewAllProvidersWithFallback creates a provider with Gemini 3.1 Flash-Lite
// as primary and Anthropic Claude Haiku 4.5 as fallback. Selected based on the 2026-05-09
// gear-image eval (#1777, see docs/issues/1777-baseline-2026-05-09.md):
// Gemini 3.1 Flash-Lite scored 93.6% to Haiku's 84.9% on the small-model
// tier with comparable median latency (~3.1s vs ~3.2s) and lower per-call
// cost. The eval covers gear-from-image specifically, but Flash-Lite also
// performed well on the text and webpage suites in earlier runs, so it's
// a defensible global default.
//
// Default configuration:
//   - Primary: Gemini 3.1 Flash-Lite (vertexai/gemini-3.1-flash-lite)
//   - Fallback: Anthropic Claude Haiku 4.5 (on any error from primary)
//   - Tertiary fallback: OpenAI GPT-5-mini (if configured)
//
// Only providers with API keys set will be included. If only one provider is configured,
// it will be returned directly without fallback.
func NewAllProvidersWithFallback(ctx context.Context, cfg AllProvidersConfig) (Provider, error) {
	logger := logging.Default()

	// Default models
	const (
		defaultOpenAIModel    = "gpt-5-mini"
		defaultAnthropicModel = "claude-haiku-4-5"
		defaultVertexModel    = "vertexai/gemini-3.1-flash-lite"
	)

	// Create individual provider instances.
	var anthropicProvider, geminiProvider, openaiProvider Provider
	var err error

	// Create Gemini provider (primary).
	if cfg.VertexAIProject != "" {
		model := cfg.GeminiModelOverride
		if model == "" {
			model = defaultVertexModel
		}

		// Default to the global endpoint: it works for every Gemini
		// generation (2.5, 3.x, preview), spreads load across regions
		// (regional pools intermittently throw 429 RESOURCE_EXHAUSTED on
		// 2.5-flash), and is the only endpoint exposing newer models
		// like gemini-3.1-flash-lite as of 2026-05.
		location := cfg.VertexAILocation
		if location == "" {
			location = vertexLocationGlobal
		}

		geminiProvider, err = NewGeminiProvider(ctx, model, cfg.VertexAIProject, location, resolveTemperature(cfg.GeminiTemperature))
		if err != nil {
			return nil, err
		}
		logger.Info("Gemini provider created", "model", model, "location", location, "role", "primary")
	}

	// Create Anthropic provider (fallback).
	if cfg.AnthropicAPIKey != "" {
		model := cfg.AnthropicModelOverride
		if model == "" {
			model = defaultAnthropicModel
		}
		anthropicProvider, err = NewAnthropicProvider(cfg.AnthropicAPIKey, model, resolveTemperature(cfg.AnthropicTemperature))
		if err != nil {
			return nil, err
		}
		logger.Info("Anthropic provider created", "model", model, "role", "fallback")
	}

	// Create OpenAI provider (tertiary fallback).
	if cfg.OpenAIAPIKey != "" {
		model := cfg.OpenAIModelOverride
		if model == "" {
			model = defaultOpenAIModel
		}
		openaiProvider, err = NewOpenAIProvider(cfg.OpenAIAPIKey, model, resolveTemperature(cfg.OpenAITemperature))
		if err != nil {
			return nil, err
		}
		logger.Info("OpenAI provider created", "model", model, "role", "tertiary fallback")
	}

	// Determine primary provider: prefer Gemini, fall back to Anthropic.
	primary := geminiProvider
	if primary == nil {
		primary = anthropicProvider
		anthropicProvider = nil // Don't also use as fallback.
	}

	if primary == nil {
		logger.Info("no AI providers configured, AI-powered features disabled")
		return nil, nil
	}

	// Build fallback chain.
	var fallbacks []Provider
	if anthropicProvider != nil {
		fallbacks = append(fallbacks, anthropicProvider)
	}
	if openaiProvider != nil {
		fallbacks = append(fallbacks, openaiProvider)
	}

	if len(fallbacks) == 0 {
		return primary, nil
	}

	fb, err := NewFallbackProvider(primary, fallbacks)
	if err != nil {
		return nil, err
	}

	logger.Info("fallback provider configured",
		"primary", primary.Name(),
		"fallback_count", len(fallbacks))

	return fb, nil
}

// NewSingleProvider creates a single AI provider from configuration.
func NewSingleProvider(ctx context.Context, cfg ProviderConfig) (Provider, error) {
	return createProvider(ctx, cfg)
}

// NewFallbackProviderFromConfigs creates a provider with fallback behavior.
// The primary provider is tried first, then fallbacks in order on server errors.
func NewFallbackProviderFromConfigs(ctx context.Context, primary ProviderConfig, fallbacks []ProviderConfig) (Provider, error) {
	primaryProvider, err := createProvider(ctx, primary)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary provider: %w", err)
	}

	var fallbackProviders []Provider
	for i, cfg := range fallbacks {
		p, err := createProvider(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create fallback provider %d: %w", i, err)
		}
		fallbackProviders = append(fallbackProviders, p)
	}

	fb, err := NewFallbackProvider(primaryProvider, fallbackProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback provider: %w", err)
	}

	logging.Default().Info("fallback provider created",
		"primary_count", 1,
		"fallback_count", len(fallbackProviders))
	return fb, nil
}

// NewWeightedProvider creates a load-balanced provider from weighted configurations.
// Requests are distributed across providers based on their weights.
func NewWeightedProvider(ctx context.Context, configs []WeightedProviderConfig) (Provider, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one provider configuration is required")
	}

	var weightedProviders []WeightedProvider
	for i, wc := range configs {
		p, err := createProvider(ctx, wc.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider %d: %w", i, err)
		}
		weightedProviders = append(weightedProviders, WeightedProvider{
			Provider: p,
			Name:     string(wc.Config.Type),
			Weight:   wc.Weight,
		})
	}

	lb, err := NewLoadBalancedProvider(weightedProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to create load-balanced provider: %w", err)
	}

	logging.Default().Info("weighted provider created",
		"provider_count", len(weightedProviders))
	return lb, nil
}

// NewWeightedProviderWithFallback creates a load-balanced provider wrapped with fallback behavior.
// Requests are distributed across the weighted providers, with fallbacks tried on server errors.
func NewWeightedProviderWithFallback(ctx context.Context, weighted []WeightedProviderConfig, fallbacks []ProviderConfig) (Provider, error) {
	if len(weighted) == 0 {
		return nil, fmt.Errorf("at least one weighted provider configuration is required")
	}

	// Create load-balanced provider
	lb, err := NewWeightedProvider(ctx, weighted)
	if err != nil {
		return nil, err
	}

	// Create fallback providers
	var fallbackProviders []Provider
	for i, cfg := range fallbacks {
		p, err := createProvider(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create fallback provider %d: %w", i, err)
		}
		fallbackProviders = append(fallbackProviders, p)
	}

	// Wrap with fallback
	fb, err := NewFallbackProvider(lb, fallbackProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback provider: %w", err)
	}

	logging.Default().Info("weighted provider with fallback created",
		"weighted_count", len(weighted),
		"fallback_count", len(fallbackProviders))
	return fb, nil
}

// createProvider creates a single provider from configuration.
func createProvider(ctx context.Context, cfg ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case ProviderTypeGemini:
		if cfg.VertexAIProject == "" {
			return nil, fmt.Errorf("missing Vertex AI project for the Gemini provider")
		}
		model := cfg.Model
		if model == "" {
			model = "vertexai/gemini-3.1-flash-lite"
		}
		// Preview models require "global" location instead of regional endpoints
		location := cfg.VertexAILocation
		if strings.Contains(model, "preview") {
			location = vertexLocationGlobal
		}
		logging.Default().Info("initializing Vertex AI provider",
			"provider", "gemini",
			"project", cfg.VertexAIProject,
			"location", location,
			"model", model)
		provider, err := NewGeminiProvider(ctx, model, cfg.VertexAIProject, location, resolveTemperature(cfg.Temperature))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Gemini provider: %w", err)
		}
		logging.Default().Info("Vertex AI provider initialized", "provider", "gemini")
		return provider, nil

	case ProviderTypeOpenAI:
		model := cfg.Model
		if model == "" {
			model = "gpt-5-mini"
		}
		logging.Default().Info("initializing OpenAI provider",
			"provider", "openai",
			"model", model)
		provider, err := NewOpenAIProvider(cfg.APIKey, model, resolveTemperature(cfg.Temperature))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenAI provider: %w", err)
		}
		logging.Default().Info("OpenAI provider initialized", "provider", "openai")
		return provider, nil

	case ProviderTypeAnthropic:
		model := cfg.Model
		if model == "" {
			model = "claude-haiku-4-5"
		}
		logging.Default().Info("initializing Anthropic provider",
			"provider", "anthropic",
			"model", model)
		provider, err := NewAnthropicProvider(cfg.APIKey, model, resolveTemperature(cfg.Temperature))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Anthropic provider: %w", err)
		}
		logging.Default().Info("Anthropic provider initialized", "provider", "anthropic")
		return provider, nil

	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}
