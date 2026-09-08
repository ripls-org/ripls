package media

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// FallbackStockVideoProvider implements a fallback chain for stock video providers.
// It tries each provider in sequence until one succeeds, with a per-provider timeout
// to ensure one slow provider does not exhaust the overall time budget.
type FallbackStockVideoProvider struct {
	providers          []StockVideoProvider
	names              []string
	perProviderTimeout time.Duration
}

// NewFallbackStockVideoProvider creates a fallback provider with an ordered list of
// video providers. Providers are tried in order until one succeeds.
func NewFallbackStockVideoProvider(providers []StockVideoProvider, names []string) (*FallbackStockVideoProvider, error) {
	return NewFallbackStockVideoProviderWithTimeout(providers, names, DefaultPerProviderTimeout)
}

// NewFallbackStockVideoProviderWithTimeout creates a fallback video provider with a
// custom per-provider timeout.
func NewFallbackStockVideoProviderWithTimeout(providers []StockVideoProvider, names []string, perProviderTimeout time.Duration) (*FallbackStockVideoProvider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}
	if len(providers) != len(names) {
		return nil, fmt.Errorf("providers and names must have the same length")
	}

	return &FallbackStockVideoProvider{
		providers:          providers,
		names:              names,
		perProviderTimeout: perProviderTimeout,
	}, nil
}

// GetStockVideo tries each provider in sequence until one succeeds.
// Each provider is given its own timeout (perProviderTimeout) to prevent one slow
// provider from consuming the entire time budget. Returns the first successful result,
// or the last error if all providers fail.
func (f *FallbackStockVideoProvider) GetStockVideo(ctx context.Context, query string) (*models.StockImage, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockVideoProvider",
		"query", query,
		"provider_count", len(f.providers),
		"per_provider_timeout_sec", int(f.perProviderTimeout.Seconds()),
	)

	var lastErr error
	for i, provider := range f.providers {
		providerName := f.names[i]
		providerStartTime := time.Now()

		logger.InfoContext(ctx, "trying stock video provider",
			"provider", providerName,
			"attempt", i+1,
			"total_providers", len(f.providers))

		// Each provider gets an isolated timeout so one slow provider cannot
		// consume the entire time budget allocated to the fallback chain.
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		result, err := provider.GetStockVideo(providerCtx, query)
		cancel()
		providerDurationMs := time.Since(providerStartTime).Milliseconds()

		if err == nil {
			totalDurationMs := time.Since(startTime).Milliseconds()
			logger.InfoContext(ctx, "stock video provider succeeded",
				"provider", providerName,
				"attempt", i+1,
				"stock_image_id", result.Id,
				"provider_duration_ms", providerDurationMs,
				"total_duration_ms", totalDurationMs)
			return result, nil
		}

		logger.WarnContext(ctx, "stock video provider failed, trying next",
			"provider", providerName,
			"attempt", i+1,
			"duration_ms", providerDurationMs,
			"error", err)
		lastErr = err

		// If the parent context is cancelled, stop trying additional providers.
		if ctx.Err() != nil {
			break
		}
	}

	totalDurationMs := time.Since(startTime).Milliseconds()
	logger.ErrorContext(ctx, "all stock video providers failed",
		"providers_tried", len(f.providers),
		"total_duration_ms", totalDurationMs,
		"last_error", lastErr)
	return nil, fmt.Errorf("all %d stock video providers failed: %w", len(f.providers), lastErr)
}

// GetStockVideoByID tries each provider until one returns a non-error
// result.
func (f *FallbackStockVideoProvider) GetStockVideoByID(ctx context.Context, providerVideoID string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockVideoProvider.GetStockVideoByID",
		"provider_video_id", providerVideoID,
		"provider_count", len(f.providers),
	)
	var lastErr error
	for i, provider := range f.providers {
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		v, err := provider.GetStockVideoByID(providerCtx, providerVideoID)
		cancel()
		if err == nil && v != nil {
			return v, nil
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
	return nil, fmt.Errorf("no provider supports stock video by-id lookup")
}

// SearchStockVideoCandidates tries each provider in sequence until one
// returns a non-empty candidate slice, applying the same per-provider
// timeout used by GetStockVideo.
func (f *FallbackStockVideoProvider) SearchStockVideoCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FallbackStockVideoProvider.SearchStockVideoCandidates",
		"query", query,
		"limit", limit,
		"provider_count", len(f.providers),
	)
	var lastErr error
	for i, provider := range f.providers {
		providerCtx, cancel := context.WithTimeout(ctx, f.perProviderTimeout)
		cands, err := provider.SearchStockVideoCandidates(providerCtx, query, limit)
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
		return nil, fmt.Errorf("all stock video candidate providers failed: %w", lastErr)
	}
	return nil, nil
}

// CheckHealth validates all underlying stock video providers and returns their
// individual statuses. Individual provider failures are reported in each status's
// Error field rather than propagating as a Go error.
func (f *FallbackStockVideoProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
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
