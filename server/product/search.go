// Package product provides product recognition and specification lookup capabilities.
package product

import (
	"fmt"
	"net/url"
	"strings"
)

// SearchProvider represents a provider for product searches.
type SearchProvider string

const (
	// ProviderGoogle uses Google Shopping for product searches.
	ProviderGoogle SearchProvider = "google"
	// ProviderAmazon uses Amazon for product searches.
	ProviderAmazon SearchProvider = "amazon"
	// ProviderHomeDepot uses Home Depot for product searches.
	ProviderHomeDepot SearchProvider = "homedepot"
	// ProviderREI uses REI for product searches.
	ProviderREI SearchProvider = "rei"
)

// DefaultProviders is the default order of search providers to try.
var DefaultProviders = []SearchProvider{
	ProviderGoogle, // Google Shopping as primary fallback
}

// manufacturerDomains maps brand names to their manufacturer website domains.
var manufacturerDomains = map[string]string{
	"dewalt":       "dewalt.com",
	"milwaukee":    "milwaukeetool.com",
	"makita":       "makitatools.com",
	"bosch":        "boschtools.com",
	"ryobi":        "ryobitools.com",
	"craftsman":    "craftsman.com",
	"black+decker": "blackanddecker.com",
	"stanley":      "stanleytools.com",
	"coleman":      "coleman.com",
	"rei":          "rei.com",
	"patagonia":    "patagonia.com",
	"north face":   "thenorthface.com",
	"osprey":       "osprey.com",
	"thule":        "thule.com",
	"yakima":       "yakima.com",
	"weber":        "weber.com",
	"kitchenaid":   "kitchenaid.com",
	"cuisinart":    "cuisinart.com",
	"dyson":        "dyson.com",
	"bissell":      "bissell.com",
	"shark":        "sharkclean.com",
}

// BuildSearchURL constructs a search URL for the given product information.
// It returns a URL that can be fetched to find product specifications.
func BuildSearchURL(brand, model string, provider SearchProvider) string {
	query := buildSearchQuery(brand, model)
	if query == "" {
		return ""
	}

	switch provider {
	case ProviderGoogle:
		return buildGoogleShoppingURL(query)
	case ProviderAmazon:
		return buildAmazonURL(query)
	case ProviderHomeDepot:
		return buildHomeDepotURL(query)
	case ProviderREI:
		return buildREIURL(query)
	default:
		return buildGoogleShoppingURL(query)
	}
}

// BuildManufacturerSearchURL constructs a search URL for the manufacturer's website.
// Returns empty string if the brand is not recognized.
func BuildManufacturerSearchURL(brand, model string) string {
	brandLower := strings.ToLower(strings.TrimSpace(brand))
	domain, ok := manufacturerDomains[brandLower]
	if !ok {
		return ""
	}

	query := buildSearchQuery(brand, model)
	if query == "" {
		return ""
	}

	// Use site-specific Google search for manufacturer sites
	return fmt.Sprintf("https://www.google.com/search?q=%s+site:%s",
		url.QueryEscape(query),
		domain,
	)
}

// buildSearchQuery constructs a search query string from brand and model.
func buildSearchQuery(brand, model string) string {
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)

	if brand == "" && model == "" {
		return ""
	}

	if model == "" {
		return brand
	}

	if brand == "" {
		return model
	}

	return fmt.Sprintf("%s %s", brand, model)
}

// buildGoogleShoppingURL constructs a Google Shopping search URL.
func buildGoogleShoppingURL(query string) string {
	return fmt.Sprintf("https://www.google.com/search?q=%s&tbm=shop",
		url.QueryEscape(query),
	)
}

// buildAmazonURL constructs an Amazon search URL.
func buildAmazonURL(query string) string {
	return fmt.Sprintf("https://www.amazon.com/s?k=%s",
		url.QueryEscape(query),
	)
}

// buildHomeDepotURL constructs a Home Depot search URL.
func buildHomeDepotURL(query string) string {
	return fmt.Sprintf("https://www.homedepot.com/s/%s",
		url.QueryEscape(query),
	)
}

// buildREIURL constructs an REI search URL.
func buildREIURL(query string) string {
	return fmt.Sprintf("https://www.rei.com/search?q=%s",
		url.QueryEscape(query),
	)
}

// GetBrandDomain returns the manufacturer domain for a known brand.
// Returns empty string if the brand is not recognized.
func GetBrandDomain(brand string) string {
	brandLower := strings.ToLower(strings.TrimSpace(brand))
	return manufacturerDomains[brandLower]
}
