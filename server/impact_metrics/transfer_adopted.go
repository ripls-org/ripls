package impact_metrics

import (
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// MaskTransferAdoptedRequestDimensions zeroes the money, emissions, and time
// components of a request's estimate when they were adopted from a
// gear-backed offer transfer (#2702). Those dimensions are display copies on
// the request — aggregation counts them exactly once, on the transfer — while
// Quality Time remains the request's own contribution. Requests without the
// adoption marker pass through unchanged.
func MaskTransferAdoptedRequestDimensions(request *models.Request, ie *api.ImpactEstimate) *api.ImpactEstimate {
	if request.GetImpactAdoptedFromTransferId() == "" || ie == nil {
		return ie
	}
	masked, ok := proto.Clone(ie).(*api.ImpactEstimate)
	if !ok {
		return ie
	}
	masked.MoneySaved = nil
	masked.EmissionsPrevented = nil
	masked.TimeSaved = nil
	return masked
}

// SumTransferImpactDimensions rolls the money, emissions, and time
// dimensions of several transfers' impact estimates into one (#2702): a
// request fulfilled through more than one gear-backed handoff displays the
// sum of all its children. Means add; uncertainties combine in quadrature.
// Quality Time is intentionally absent from the result — it stays the
// request's own computation. A single input passes its dimensions through
// unchanged (cloned), preserving structured inputs and provenance; with
// several inputs each dimension's provenance is taken from the first
// estimate that carries it. Nil entries are skipped; an all-nil input
// yields an estimate with nil dimensions.
func SumTransferImpactDimensions(items []*api.ImpactEstimate) *api.ImpactEstimate {
	nonNil := make([]*api.ImpactEstimate, 0, len(items))
	for _, ie := range items {
		if ie != nil {
			nonNil = append(nonNil, ie)
		}
	}
	out := &api.ImpactEstimate{}
	if len(nonNil) == 0 {
		return out
	}
	if len(nonNil) == 1 {
		cloned, ok := proto.Clone(nonNil[0]).(*api.ImpactEstimate)
		if ok {
			out.MoneySaved = cloned.MoneySaved
			out.EmissionsPrevented = cloned.EmissionsPrevented
			out.TimeSaved = cloned.TimeSaved
			return out
		}
	}

	var money, manufacture, waste, minutes []*api.Estimate
	var moneyProv, emissionsProv, timeProv *api.Provenance
	for _, ie := range nonNil {
		if ms := ie.GetMoneySaved(); ms != nil {
			money = append(money, ms.GetValueUsd())
			if moneyProv == nil {
				moneyProv = ms.GetProvenance()
			}
		}
		if ep := ie.GetEmissionsPrevented(); ep != nil {
			if c := ep.GetManufactureAvoidedCarbon(); c != nil {
				manufacture = append(manufacture, c.GetCo2EGrams())
			}
			if c := ep.GetWasteReducedCarbon(); c != nil {
				waste = append(waste, c.GetCo2EGrams())
			}
			if emissionsProv == nil {
				emissionsProv = ep.GetProvenance()
			}
		}
		if ts := ie.GetTimeSaved(); ts != nil {
			minutes = append(minutes, ts.GetMinutes())
			if timeProv == nil {
				timeProv = ts.GetProvenance()
			}
		}
	}
	if len(money) > 0 {
		out.MoneySaved = &api.MoneySavings{
			ValueUsd:   estimator.SumEstimates(money),
			Provenance: moneyProv,
		}
	}
	if len(manufacture) > 0 || len(waste) > 0 {
		emissions := &api.PreventedEmissions{Provenance: emissionsProv}
		if len(manufacture) > 0 {
			emissions.ManufactureAvoidedCarbon = &api.CarbonEstimate{
				Co2EGrams: estimator.SumEstimates(manufacture),
			}
		}
		if len(waste) > 0 {
			emissions.WasteReducedCarbon = &api.CarbonEstimate{
				Co2EGrams: estimator.SumEstimates(waste),
			}
		}
		out.EmissionsPrevented = emissions
	}
	if len(minutes) > 0 {
		out.TimeSaved = &api.TimeSavings{
			Minutes:    estimator.SumEstimates(minutes),
			Provenance: timeProv,
		}
	}
	return out
}

// RequestDimensionValue returns a request's contribution to one aggregate
// dimension, applying the transfer-adoption masking (#2702): zero for money,
// emissions, and time on transfer-backed requests, the stored value otherwise.
// Use this instead of ExtractDimensionValue when summing request impact.
func RequestDimensionValue(request *models.Request, dimension api.ImpactMetricDimension) float64 {
	if request.GetImpactAdoptedFromTransferId() != "" &&
		dimension != api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME {
		return 0
	}
	return ExtractDimensionValue(request.ImpactEstimate, dimension)
}

// MaskTransferAdoptedExperienceDimensions zeroes the money, emissions, and
// time components of an experience's estimate when they were adopted from its
// gear-backed child transfers at completion (#2724, mirroring the request
// masking above). Those dimensions are display copies on the experience —
// aggregation counts them exactly once, on the transfers — while Quality Time
// remains the experience's own contribution. Experiences without the adoption
// marker pass through unchanged.
func MaskTransferAdoptedExperienceDimensions(experience *models.Experience, ie *api.ImpactEstimate) *api.ImpactEstimate {
	if experience.GetImpactAdoptedFromTransferId() == "" || ie == nil {
		return ie
	}
	masked, ok := proto.Clone(ie).(*api.ImpactEstimate)
	if !ok {
		return ie
	}
	masked.MoneySaved = nil
	masked.EmissionsPrevented = nil
	masked.TimeSaved = nil
	return masked
}

// ExperienceDimensionValue returns an experience's contribution to one
// aggregate dimension, applying the transfer-adoption masking (#2724): zero
// for money, emissions, and time on transfer-backed experiences, the stored
// value otherwise. Use this instead of ExtractDimensionValue when summing
// experience impact.
func ExperienceDimensionValue(experience *models.Experience, dimension api.ImpactMetricDimension) float64 {
	if experience.GetImpactAdoptedFromTransferId() != "" &&
		dimension != api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME {
		return 0
	}
	return ExtractDimensionValue(experience.ImpactEstimate, dimension)
}
