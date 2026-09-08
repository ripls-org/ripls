package product

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/webfetch"
)

const (
	// DefaultTimeout is the default timeout for product lookups.
	DefaultTimeout = 3 * time.Second

	// MinConfidenceForLookup is the minimum AI confidence required to attempt a product lookup.
	MinConfidenceForLookup = 0.8
)

// Info contains identified product details from AI detection.
type Info struct {
	Brand       string  // Product brand name (e.g., "DeWalt")
	Model       string  // Product model number (e.g., "DCD771C2")
	Confidence  float64 // AI confidence in the identification (0.0-1.0)
	SearchQuery string  // Constructed search query
}

// Lookup provides product specification retrieval from manufacturer and retailer websites.
type Lookup struct {
	webFetcher webfetch.Fetcher
	aiProvider ai.Provider
	timeout    time.Duration
}

// NewLookup creates a new ProductLookup with the given dependencies.
func NewLookup(webFetcher webfetch.Fetcher, aiProvider ai.Provider) *Lookup {
	return &Lookup{
		webFetcher: webFetcher,
		aiProvider: aiProvider,
		timeout:    DefaultTimeout,
	}
}

// SetTimeout sets the maximum time allowed for product lookups.
func (l *Lookup) SetTimeout(timeout time.Duration) {
	l.timeout = timeout
}

// LookupSpecs fetches specifications for an identified product.
// Returns nil if lookup fails, times out, or the product cannot be found.
// This provides graceful degradation - callers should fall back to AI-only results.
func (l *Lookup) LookupSpecs(ctx context.Context, info *Info) (*ai.GearGeneration, error) {
	if l.webFetcher == nil || l.aiProvider == nil {
		return nil, nil
	}

	if info == nil || (info.Brand == "" && info.Model == "") {
		return nil, nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"brand", info.Brand,
		"model", info.Model,
		"confidence", info.Confidence,
	)

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	// Try manufacturer website first for recognized brands
	if info.Brand != "" {
		specs, err := l.lookupFromManufacturer(ctx, info)
		if err == nil && specs != nil {
			logger.Info("product specs found from manufacturer site")
			return specs, nil
		}
		if err != nil {
			logger.Debug("manufacturer lookup failed, trying retailers", "error", err)
		}
	}

	// Fall back to retailer search
	for _, provider := range DefaultProviders {
		specs, err := l.lookupFromProvider(ctx, info, provider)
		if err == nil && specs != nil {
			logger.Info("product specs found from retailer",
				"provider", string(provider))
			return specs, nil
		}
		if ctx.Err() != nil {
			// Context timeout or cancelled
			logger.Debug("product lookup timed out")
			return nil, nil
		}
	}

	logger.Debug("no product specs found from any source")
	return nil, nil
}

// lookupFromManufacturer attempts to fetch specs from the manufacturer's website.
func (l *Lookup) lookupFromManufacturer(ctx context.Context, info *Info) (*ai.GearGeneration, error) {
	searchURL := BuildManufacturerSearchURL(info.Brand, info.Model)
	if searchURL == "" {
		return nil, nil
	}

	return l.fetchAndExtract(ctx, searchURL)
}

// lookupFromProvider attempts to fetch specs from a specific search provider.
func (l *Lookup) lookupFromProvider(ctx context.Context, info *Info, provider SearchProvider) (*ai.GearGeneration, error) {
	searchURL := BuildSearchURL(info.Brand, info.Model, provider)
	if searchURL == "" {
		return nil, nil
	}

	return l.fetchAndExtract(ctx, searchURL)
}

// fetchAndExtract fetches a webpage and extracts product information using AI.
func (l *Lookup) fetchAndExtract(ctx context.Context, url string) (*ai.GearGeneration, error) {
	logger := logging.LoggerWithContext(ctx).With("url", url)

	// Fetch the webpage
	pageContent, err := l.webFetcher.FetchPageContent(ctx, url)
	if err != nil {
		logger.Debug("failed to fetch product page", "error", err)
		return nil, err
	}

	// Check if we got meaningful content
	if pageContent.Title == "" && pageContent.BodyText == "" {
		logger.Debug("product page has no content")
		return nil, nil
	}

	// Use AI to extract specifications from the page
	generation, err := l.aiProvider.GenerateGearFromWebpage(
		ctx,
		pageContent.Title,
		pageContent.Description,
		pageContent.BodyText,
		"", // region
	)
	if err != nil {
		logger.Debug("failed to extract specs from page", "error", err)
		return nil, err
	}

	return generation, nil
}

// ShouldAttemptLookup determines if a product lookup should be attempted
// based on the AI detection results.
func ShouldAttemptLookup(brand string, confidence float64) bool {
	return brand != "" && confidence >= MinConfidenceForLookup
}

// MergeGearGeneration merges fetched specs with AI detection results.
// The fetched specs take precedence for description and value, but
// the original title is preserved if the fetched one seems generic.
func MergeGearGeneration(aiResult, fetchedSpecs *ai.GearGeneration) *ai.GearGeneration {
	if fetchedSpecs == nil {
		return aiResult
	}
	if aiResult == nil {
		return fetchedSpecs
	}

	merged := &ai.GearGeneration{
		Title:         aiResult.Title, // Prefer original title
		Description:   aiResult.Description,
		Category:      aiResult.Category,
		Confidence:    aiResult.Confidence,
		LocationQuery: aiResult.LocationQuery,
	}

	// Use fetched description if it's more detailed
	if len(fetchedSpecs.Description) > len(aiResult.Description) {
		merged.Description = fetchedSpecs.Description
	}

	// Use fetched value estimate if available and has higher confidence
	if fetchedSpecs.ValueEstimate != nil {
		if aiResult.ValueEstimate == nil ||
			fetchedSpecs.ValueEstimate.Confidence > aiResult.ValueEstimate.Confidence {
			merged.ValueEstimate = fetchedSpecs.ValueEstimate
		}
	} else if aiResult.ValueEstimate != nil {
		merged.ValueEstimate = aiResult.ValueEstimate
	}

	// Update confidence to reflect merged data
	if fetchedSpecs.Confidence > 0 {
		// Average the confidences, weighted toward higher confidence
		merged.Confidence = (aiResult.Confidence + fetchedSpecs.Confidence) / 2
		if merged.Confidence < aiResult.Confidence {
			merged.Confidence = aiResult.Confidence
		}
	}

	return merged
}
