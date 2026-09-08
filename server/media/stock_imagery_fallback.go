package media

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

const (
	// DefaultPerProviderTimeout is the maximum time allowed for each provider in the fallback chain.
	// This ensures one slow provider doesn't consume the entire time budget.
	DefaultPerProviderTimeout = 30 * time.Second
)

// FallbackStockImageryProvider implements a fallback chain for stock imagery providers.
// It tries each provider in sequence until one succeeds, with a timeout for each provider.
type FallbackStockImageryProvider struct {
	providers          []StockImageryProvider
	names              []string
	perProviderTimeout time.Duration
}

// NewFallbackStockImageryProvider creates a fallback provider with an ordered list of providers.
// Providers are tried in order until one succeeds. Each provider has a 30-second timeout
// to prevent one slow provider from consuming the entire time budget.
func NewFallbackStockImageryProvider(providers []StockImageryProvider, names []string) (*FallbackStockImageryProvider, error) {
	return NewFallbackStockImageryProviderWithTimeout(providers, names, DefaultPerProviderTimeout)
}

// NewFallbackStockImageryProviderWithTimeout creates a fallback provider with a custom per-provider timeout.
func NewFallbackStockImageryProviderWithTimeout(providers []StockImageryProvider, names []string, perProviderTimeout time.Duration) (*FallbackStockImageryProvider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}
	if len(providers) != len(names) {
		return nil, fmt.Errorf("providers and names must have the same length")
	}

	return &FallbackStockImageryProvider{
		providers:          providers,
		names:              names,
		perProviderTimeout: perProviderTimeout,
	}, nil
}

// GetStockImage tries each provider in sequence until one succeeds.
// Each provider is given its own timeout (perProviderTimeout) to prevent one slow
// provider from consuming the entire time budget. Returns the first successful result,
// or the last error if all fail.
func (f *FallbackStockImageryProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockImageryProvider",
		"query", query,
		"provider_count", len(f.providers),
		"per_provider_timeout_sec", int(f.perProviderTimeout.Seconds()),
	)

	var lastErr error
	for i, provider := range f.providers {
		providerName := f.names[i]
		providerStartTime := time.Now()

		logger.InfoContext(ctx, "trying stock imagery provider",
			"provider", providerName,
			"attempt", i+1,
			"total_providers", len(f.providers))

		// Create a per-provider context with timeout to prevent one slow provider
		// from consuming the entire time budget. This ensures each provider gets
		// a fair chance even if an earlier one is slow.
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		result, err := provider.GetStockImage(providerCtx, query, opts)
		cancel() // Always cancel to release resources
		providerDurationMs := time.Since(providerStartTime).Milliseconds()

		if err == nil {
			totalDurationMs := time.Since(startTime).Milliseconds()
			logger.InfoContext(ctx, "stock imagery provider succeeded",
				"provider", providerName,
				"attempt", i+1,
				"stock_image_id", result.Id,
				"provider_duration_ms", providerDurationMs,
				"total_duration_ms", totalDurationMs)
			return result, nil
		}

		// Log failure and try next provider
		logger.WarnContext(ctx, "stock imagery provider failed, trying next",
			"provider", providerName,
			"attempt", i+1,
			"duration_ms", providerDurationMs,
			"error", err)
		lastErr = err

		// If the parent context is cancelled, don't try more providers
		if ctx.Err() != nil {
			break
		}
	}

	// All providers failed
	totalDurationMs := time.Since(startTime).Milliseconds()
	logger.ErrorContext(ctx, "all stock imagery providers failed",
		"providers_tried", len(f.providers),
		"total_duration_ms", totalDurationMs,
		"last_error", lastErr)
	return nil, fmt.Errorf("all %d stock imagery providers failed: %w", len(f.providers), lastErr)
}

// GetStockImageByID tries each provider until one returns a non-error
// result. Most providers return "unsupported" today; the fallback chain
// surfaces the first one that knows how to look up by id.
func (f *FallbackStockImageryProvider) GetStockImageByID(ctx context.Context, providerImageID string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockImageryProvider.GetStockImageByID",
		"provider_image_id", providerImageID,
		"provider_count", len(f.providers),
	)
	var lastErr error
	for i, provider := range f.providers {
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		img, err := provider.GetStockImageByID(providerCtx, providerImageID)
		cancel()
		if err == nil && img != nil {
			return img, nil
		}
		if err != nil {
			lastErr = err
			logger.DebugContext(ctx, "by-id lookup failed on provider, trying next",
				"provider", f.names[i], "error", err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no provider supports stock image by-id lookup")
}

// SearchStockImageCandidates tries each provider in sequence until one
// returns a non-empty candidate slice, applying the same per-provider
// timeout used by GetStockImage. Empty results from one provider fall
// through to the next so a flaky primary doesn't suppress alternates
// from the rest of the chain.
func (f *FallbackStockImageryProvider) SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockImageryProvider.SearchStockImageCandidates",
		"query", query,
		"limit", limit,
		"provider_count", len(f.providers),
	)
	var lastErr error
	for i, provider := range f.providers {
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		cands, err := provider.SearchStockImageCandidates(providerCtx, query, limit)
		cancel()
		if err == nil && len(cands) > 0 {
			logger.DebugContext(ctx, "candidate provider succeeded",
				"provider", f.names[i], "result_count", len(cands))
			return cands, nil
		}
		if err != nil {
			lastErr = err
			logger.WarnContext(ctx, "candidate provider failed, trying next",
				"provider", f.names[i], "error", err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all stock imagery candidate providers failed: %w", lastErr)
	}
	return nil, nil
}

// CheckHealth validates all underlying stock imagery providers and returns their individual statuses.
// Individual provider failures are reported in each status's Error field.
func (f *FallbackStockImageryProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	var allStatuses []*health.Status

	for _, provider := range f.providers {
		statuses, err := provider.CheckHealth(ctx)
		if err != nil {
			return nil, err
		}
		allStatuses = append(allStatuses, statuses...)
	}

	return allStatuses, nil
}
