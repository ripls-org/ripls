package impact_metrics

import (
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// CostUSD extracts the cost mean in USD from an ImpactEstimate, returning 0 if nil.
func CostUSD(ie *api.ImpactEstimate) float32 {
	if ie != nil && ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
		return ie.MoneySaved.ValueUsd.Mean
	}
	return 0
}

// Co2Grams extracts the total CO2 in grams from an ImpactEstimate, returning 0 if nil.
// Total is manufacture_avoided_carbon + waste_reduced_carbon.
func Co2Grams(ie *api.ImpactEstimate) float32 {
	if ie == nil || ie.EmissionsPrevented == nil {
		return 0
	}
	var total float32
	if c := ie.EmissionsPrevented.ManufactureAvoidedCarbon; c != nil && c.Co2EGrams != nil {
		total += c.Co2EGrams.Mean
	}
	if c := ie.EmissionsPrevented.WasteReducedCarbon; c != nil && c.Co2EGrams != nil {
		total += c.Co2EGrams.Mean
	}
	return total
}

// TimeMinutes extracts the time mean in minutes from an ImpactEstimate, returning 0 if nil.
func TimeMinutes(ie *api.ImpactEstimate) float32 {
	if ie != nil && ie.TimeSaved != nil && ie.TimeSaved.Minutes != nil {
		return ie.TimeSaved.Minutes.Mean
	}
	return 0
}

// QualityTimeMinutes extracts the composite quality-time mean in minutes from
// an ImpactEstimate, returning 0 if absent. Use this for experience/request
// stories and surfaces — the completion modal's hero "time" metric is QT,
// not TimeSaved.
func QualityTimeMinutes(ie *api.ImpactEstimate) float32 {
	if ie != nil && ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
		return ie.QualityTime.QualityTimeMinutes.Mean
	}
	return 0
}

// CostEstimate extracts cost as an Estimate from an ImpactEstimate, returning nil if absent.
func CostEstimate(ie *api.ImpactEstimate) *api.Estimate {
	if ie != nil && ie.MoneySaved != nil {
		return ie.MoneySaved.ValueUsd
	}
	return nil
}

// CarbonEstimate extracts total carbon as an Estimate from an ImpactEstimate.
// Sums manufacture_avoided_carbon + waste_reduced_carbon with quadrature.
// Returns nil if no carbon data is present.
func CarbonEstimate(ie *api.ImpactEstimate) *api.Estimate {
	if ie == nil || ie.EmissionsPrevented == nil {
		return nil
	}
	var estimates []*api.Estimate
	if c := ie.EmissionsPrevented.ManufactureAvoidedCarbon; c != nil && c.Co2EGrams != nil {
		estimates = append(estimates, c.Co2EGrams)
	}
	if c := ie.EmissionsPrevented.WasteReducedCarbon; c != nil && c.Co2EGrams != nil {
		estimates = append(estimates, c.Co2EGrams)
	}
	if len(estimates) == 0 {
		return nil
	}
	return estimator.SumEstimates(estimates)
}

// TimeEstimate extracts time as an Estimate from an ImpactEstimate, returning nil if absent.
func TimeEstimate(ie *api.ImpactEstimate) *api.Estimate {
	if ie != nil && ie.TimeSaved != nil {
		return ie.TimeSaved.Minutes
	}
	return nil
}

// QualityTimeEstimate extracts Quality Time minutes as an Estimate from an ImpactEstimate, returning nil if absent.
func QualityTimeEstimate(ie *api.ImpactEstimate) *api.Estimate {
	if ie != nil && ie.QualityTime != nil {
		return ie.QualityTime.QualityTimeMinutes
	}
	return nil
}

// APIImpactToModels converts an api.ImpactEstimate to a models.ImpactEstimate.
func APIImpactToModels(a *api.ImpactEstimate) *models.ImpactEstimate {
	if a == nil {
		return nil
	}
	result := &models.ImpactEstimate{}
	if a.MoneySaved != nil {
		result.MoneySaved = &models.MoneySavings{
			ValueUsd:   apiEstimateToModels(a.MoneySaved.ValueUsd),
			Provenance: apiProvenanceToModels(a.MoneySaved.Provenance),
		}
	}
	if a.EmissionsPrevented != nil {
		result.EmissionsPrevented = &models.PreventedEmissions{
			Provenance: apiProvenanceToModels(a.EmissionsPrevented.Provenance),
		}
		if a.EmissionsPrevented.ManufactureAvoidedCarbon != nil {
			result.EmissionsPrevented.ManufactureAvoidedCarbon = &models.CarbonEstimate{
				Co2EGrams:  apiEstimateToModels(a.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams),
				Provenance: apiProvenanceToModels(a.EmissionsPrevented.ManufactureAvoidedCarbon.Provenance),
			}
		}
		if a.EmissionsPrevented.WasteReducedCarbon != nil {
			result.EmissionsPrevented.WasteReducedCarbon = &models.CarbonEstimate{
				Co2EGrams:  apiEstimateToModels(a.EmissionsPrevented.WasteReducedCarbon.Co2EGrams),
				Provenance: apiProvenanceToModels(a.EmissionsPrevented.WasteReducedCarbon.Provenance),
			}
		}
	}
	if a.TimeSaved != nil {
		result.TimeSaved = &models.TimeSavings{
			Minutes:    apiEstimateToModels(a.TimeSaved.Minutes),
			Provenance: apiProvenanceToModels(a.TimeSaved.Provenance),
		}
	}
	if a.QualityTime != nil {
		qt := &models.QualityTimeEstimate{
			QualityTimeMinutes: apiEstimateToModels(a.QualityTime.QualityTimeMinutes),
			BelongingMinutes:   apiEstimateToModels(a.QualityTime.BelongingMinutes),
			TrustCredits:       apiEstimateToModels(a.QualityTime.TrustCredits),
			Provenance:         apiProvenanceToModels(a.QualityTime.Provenance),
		}
		if a.QualityTime.Attributes != nil {
			attrs := a.QualityTime.Attributes
			qt.Attributes = &models.QualityTimeAttributes{
				EstimatedDurationMinutes: attrs.EstimatedDurationMinutes,
				Modality:                 int32(attrs.Modality),
				GroupSize:                attrs.GroupSize,
				TieStrength:              int32(attrs.TieStrength),
				Reciprocity:              int32(attrs.Reciprocity),
				Novelty:                  int32(attrs.Novelty),
				Vulnerability:            int32(attrs.Vulnerability),
			}
		}
		result.QualityTime = qt
	}
	return result
}

// apiEstimateToModels converts an api.Estimate to a models.Estimate.
func apiEstimateToModels(a *api.Estimate) *models.Estimate {
	if a == nil {
		return nil
	}
	return &models.Estimate{
		Mean:   a.Mean,
		Stddev: a.Stddev,
	}
}

// ModelsImpactToAPI converts a models.ImpactEstimate to an api.ImpactEstimate.
func ModelsImpactToAPI(m *models.ImpactEstimate) *api.ImpactEstimate {
	if m == nil {
		return nil
	}
	result := &api.ImpactEstimate{}
	if m.MoneySaved != nil {
		result.MoneySaved = &api.MoneySavings{
			ValueUsd:   modelsEstimateToAPI(m.MoneySaved.ValueUsd),
			Provenance: modelsProvenanceToAPI(m.MoneySaved.Provenance),
		}
	}
	if m.EmissionsPrevented != nil {
		result.EmissionsPrevented = &api.PreventedEmissions{
			Provenance: modelsProvenanceToAPI(m.EmissionsPrevented.Provenance),
		}
		if m.EmissionsPrevented.ManufactureAvoidedCarbon != nil {
			result.EmissionsPrevented.ManufactureAvoidedCarbon = &api.CarbonEstimate{
				Co2EGrams:  modelsEstimateToAPI(m.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams),
				Provenance: modelsProvenanceToAPI(m.EmissionsPrevented.ManufactureAvoidedCarbon.Provenance),
			}
		}
		if m.EmissionsPrevented.WasteReducedCarbon != nil {
			result.EmissionsPrevented.WasteReducedCarbon = &api.CarbonEstimate{
				Co2EGrams:  modelsEstimateToAPI(m.EmissionsPrevented.WasteReducedCarbon.Co2EGrams),
				Provenance: modelsProvenanceToAPI(m.EmissionsPrevented.WasteReducedCarbon.Provenance),
			}
		}
	}
	if m.TimeSaved != nil {
		result.TimeSaved = &api.TimeSavings{
			Minutes:    modelsEstimateToAPI(m.TimeSaved.Minutes),
			Provenance: modelsProvenanceToAPI(m.TimeSaved.Provenance),
		}
	}
	if m.QualityTime != nil {
		qt := &api.QualityTimeEstimate{
			QualityTimeMinutes: modelsEstimateToAPI(m.QualityTime.QualityTimeMinutes),
			BelongingMinutes:   modelsEstimateToAPI(m.QualityTime.BelongingMinutes),
			TrustCredits:       modelsEstimateToAPI(m.QualityTime.TrustCredits),
			Provenance:         modelsProvenanceToAPI(m.QualityTime.Provenance),
		}
		if m.QualityTime.Attributes != nil {
			attrs := m.QualityTime.Attributes
			qt.Attributes = &api.QualityTimeAttributes{
				EstimatedDurationMinutes: attrs.EstimatedDurationMinutes,
				Modality:                 api.SocialModality(attrs.Modality),
				GroupSize:                attrs.GroupSize,
				TieStrength:              api.SocialTieStrength(attrs.TieStrength),
				Reciprocity:              api.SocialReciprocity(attrs.Reciprocity),
				Novelty:                  api.SocialNovelty(attrs.Novelty),
				Vulnerability:            api.SocialVulnerabilityLevel(attrs.Vulnerability),
			}
		}
		result.QualityTime = qt
	}
	return result
}

// apiProvenanceToModels converts an api.Provenance to a models.Provenance.
func apiProvenanceToModels(a *api.Provenance) *models.Provenance {
	if a == nil {
		return nil
	}
	return &models.Provenance{
		Source:     models.ProvenanceSource(a.Source),
		Name:       a.Name,
		Version:    a.Version,
		Confidence: a.Confidence,
		Reasoning:  a.Reasoning,
		Sources:    a.Sources,
	}
}

// modelsProvenanceToAPI converts a models.Provenance to an api.Provenance.
func modelsProvenanceToAPI(m *models.Provenance) *api.Provenance {
	if m == nil {
		return nil
	}
	return &api.Provenance{
		Source:     api.ProvenanceSource(m.Source),
		Name:       m.Name,
		Version:    m.Version,
		Confidence: m.Confidence,
		Reasoning:  m.Reasoning,
		Sources:    m.Sources,
	}
}

// modelsEstimateToAPI converts a models.Estimate to an api.Estimate.
func modelsEstimateToAPI(m *models.Estimate) *api.Estimate {
	if m == nil {
		return nil
	}
	return &api.Estimate{
		Mean:   m.Mean,
		Stddev: m.Stddev,
	}
}

// ExtractDimensionValue extracts the value for a dimension from an ImpactEstimate.
func ExtractDimensionValue(ie *models.ImpactEstimate, dimension api.ImpactMetricDimension) float64 {
	if ie == nil {
		return 0
	}
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
			return float64(ie.MoneySaved.ValueUsd.Mean)
		}
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		var total float64
		if ie.EmissionsPrevented != nil {
			if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil &&
				ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				total += float64(ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
			if ie.EmissionsPrevented.WasteReducedCarbon != nil &&
				ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams != nil {
				total += float64(ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean)
			}
		}
		return total
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		if ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
			return float64(ie.QualityTime.QualityTimeMinutes.Mean)
		}
	}
	return 0
}

// ContainsString checks if a slice contains a given string.
func ContainsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
