package estimator

import (
	"context"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// CategoryMatcher finds the best-matching config key for a free-text category string.
// Implementations may use embedding similarity, exact matching, or other strategies.
type CategoryMatcher interface {
	// Match returns the best-matching key from candidates for the given query.
	// Returns ("", 0) if no match is above the implementation's threshold.
	Match(ctx context.Context, query string, candidates []string) (key string, score float32, err error)
}

// LookupMaterialCarbon calculates carbon from weight and material emission factor.
// Returns nil if weightGrams is zero or the material category has no configured factor.
// The returned Estimate is in grams CO2e with method-derived stddev.
func LookupMaterialCarbon(material models.MaterialCategory, weightGrams float32, cfg *Config) *api.Estimate {
	if weightGrams <= 0 || material == models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
		return nil
	}

	entry := cfg.MaterialFactor(material)
	if entry == nil {
		return nil
	}

	// co2e_grams = (weight_grams / 1000) * kg_co2e_per_kg * 1000
	// Simplifies to: weight_grams * kg_co2e_per_kg.
	meanGrams := weightGrams * entry.GetKgCo2EPerKg()

	return RelativeUncertainty(meanGrams, cfg.MethodStddev("weight_material_v1"))
}

// LookupSpendCarbon calculates carbon from item value using EEIO spend-based factors.
// Returns nil if valueUSD is zero. Uses the CategoryMatcher to find the best spend
// category, falling back to "general_consumer_goods" if no match is found.
func LookupSpendCarbon(ctx context.Context, valueUSD float32, spendCategory string, cfg *Config, matcher CategoryMatcher) *api.Estimate {
	if valueUSD <= 0 {
		return nil
	}

	configKey := resolveSpendCategory(ctx, spendCategory, cfg, matcher)

	factors := cfg.GetSpendBasedEmissionFactors()
	factor, ok := factors[configKey]
	if !ok {
		return nil
	}

	// co2e_grams = value_usd * kg_co2e_per_usd * 1000
	meanGrams := valueUSD * factor * 1000

	return RelativeUncertainty(meanGrams, cfg.MethodStddev("spend_based_v1"))
}

const defaultSpendCategory = "general_consumer_goods"

// resolveSpendCategory finds the best spend category config key for the given string.
func resolveSpendCategory(ctx context.Context, category string, cfg *Config, matcher CategoryMatcher) string {
	if category == "" {
		return defaultSpendCategory
	}

	factors := cfg.GetSpendBasedEmissionFactors()

	// Try exact match (normalized to snake_case).
	normalized := strings.ToLower(strings.TrimSpace(category))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if _, ok := factors[normalized]; ok {
		return normalized
	}

	// Use embedding matcher if available.
	if matcher != nil {
		candidates := make([]string, 0, len(factors))
		for k := range factors {
			candidates = append(candidates, k)
		}
		key, _, err := matcher.Match(ctx, category, candidates)
		if err == nil && key != "" {
			return key
		}
	}

	return defaultSpendCategory
}
