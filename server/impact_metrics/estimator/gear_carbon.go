package estimator

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// EstimateGearCarbon estimates the embodied carbon of a gear item using table lookups.
// Primary path: material + weight → LookupMaterialCarbon.
// Fallback path: value + spend category → LookupSpendCarbon.
// Returns nil if neither path has sufficient data.
func EstimateGearCarbon(
	ctx context.Context,
	material models.MaterialCategory,
	weightGrams float32,
	valueUSD float32,
	spendCategory string,
	cfg *Config,
	matcher CategoryMatcher,
) *api.CarbonEstimate {
	// Primary: weight × material factor (most precise table lookup).
	if est := LookupMaterialCarbon(material, weightGrams, cfg); est != nil {
		provName := ProvenanceWeightMaterialCarbon
		return &api.CarbonEstimate{
			Co2EGrams: est,
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    provName,
				Version: cfg.ProvenanceVersion(provName),
			},
		}
	}

	// Fallback: spend-based EEIO factor.
	if est := LookupSpendCarbon(ctx, valueUSD, spendCategory, cfg, matcher); est != nil {
		provName := ProvenanceSpendBasedCarbon
		return &api.CarbonEstimate{
			Co2EGrams: est,
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    provName,
				Version: cfg.ProvenanceVersion(provName),
			},
		}
	}

	return nil
}
