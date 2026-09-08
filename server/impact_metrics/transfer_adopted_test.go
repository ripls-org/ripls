package impact_metrics

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func adoptedRequest(transferID string) *models.Request {
	r := &models.Request{
		Id:    "req-1",
		State: models.RequestState_REQUEST_STATE_FULFILLED,
		ImpactEstimate: &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{
				ValueUsd: &models.Estimate{Mean: 40, Stddev: 12},
			},
			TimeSaved: &models.TimeSavings{
				Minutes: &models.Estimate{Mean: 90, Stddev: 30},
			},
			QualityTime: &models.QualityTimeEstimate{
				QualityTimeMinutes: &models.Estimate{Mean: 25, Stddev: 5},
			},
		},
	}
	if transferID != "" {
		r.ImpactAdoptedFromTransferId = &transferID
	}
	return r
}

func TestMaskTransferAdoptedRequestDimensions(t *testing.T) {
	r := adoptedRequest("transfer-1")
	ie := ModelsImpactToAPI(r.ImpactEstimate)

	masked := MaskTransferAdoptedRequestDimensions(r, ie)
	if masked.MoneySaved != nil || masked.TimeSaved != nil || masked.EmissionsPrevented != nil {
		t.Errorf("expected money/time/emissions masked, got %+v", masked)
	}
	if masked.QualityTime == nil || masked.QualityTime.QualityTimeMinutes.Mean != 25 {
		t.Errorf("expected quality time preserved, got %+v", masked.QualityTime)
	}
	// The input estimate is untouched (clone semantics).
	if ie.MoneySaved == nil {
		t.Error("masking must not mutate the input estimate")
	}

	// Without the marker the estimate passes through unchanged.
	plain := adoptedRequest("")
	plainIE := ModelsImpactToAPI(plain.ImpactEstimate)
	if got := MaskTransferAdoptedRequestDimensions(plain, plainIE); got.MoneySaved == nil {
		t.Error("expected unmarked request estimate to pass through")
	}
}

func TestRequestDimensionValue_MasksAdoptedDimensions(t *testing.T) {
	r := adoptedRequest("transfer-1")

	if got := RequestDimensionValue(r, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY); got != 0 {
		t.Errorf("expected money masked to 0, got %v", got)
	}
	if got := RequestDimensionValue(r, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME); got != 0 {
		t.Errorf("expected time masked to 0, got %v", got)
	}
	if got := RequestDimensionValue(r, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME); got != 25 {
		t.Errorf("expected quality time 25, got %v", got)
	}

	plain := adoptedRequest("")
	if got := RequestDimensionValue(plain, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY); got != 40 {
		t.Errorf("expected unmarked money 40, got %v", got)
	}
}

func transferEstimate(money, manufacture, minutes float32) *api.ImpactEstimate {
	return &api.ImpactEstimate{
		MoneySaved: &api.MoneySavings{
			ValueUsd:   &api.Estimate{Mean: money, Stddev: 3},
			Provenance: &api.Provenance{Name: "estimator"},
		},
		EmissionsPrevented: &api.PreventedEmissions{
			ManufactureAvoidedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: manufacture, Stddev: 4},
			},
		},
		TimeSaved: &api.TimeSavings{
			Minutes: &api.Estimate{Mean: minutes, Stddev: 0},
		},
		QualityTime: &api.QualityTimeEstimate{
			QualityTimeMinutes: &api.Estimate{Mean: 60},
		},
	}
}

func TestSumTransferImpactDimensions_SingleChildPassesThrough(t *testing.T) {
	in := transferEstimate(420, 30000, 45)
	got := SumTransferImpactDimensions([]*api.ImpactEstimate{in})

	if got.MoneySaved.ValueUsd.Mean != 420 {
		t.Errorf("expected money 420, got %v", got.MoneySaved.ValueUsd.Mean)
	}
	if got.MoneySaved.Provenance.GetName() != "estimator" {
		t.Errorf("single child must keep its provenance, got %+v", got.MoneySaved.Provenance)
	}
	if got.QualityTime != nil {
		t.Error("quality time must stay the request's own — never adopted")
	}
	// The pass-through must be a clone: mutating the result cannot reach in.
	got.MoneySaved.ValueUsd.Mean = 1
	if in.MoneySaved.ValueUsd.Mean != 420 {
		t.Error("pass-through aliased the input instead of cloning")
	}
}

func TestSumTransferImpactDimensions_SumsChildren(t *testing.T) {
	got := SumTransferImpactDimensions([]*api.ImpactEstimate{
		transferEstimate(420, 30000, 45),
		nil, // skipped
		transferEstimate(80, 5000, 15),
	})

	if got.MoneySaved.ValueUsd.Mean != 500 {
		t.Errorf("expected summed money 500, got %v", got.MoneySaved.ValueUsd.Mean)
	}
	if got.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean != 35000 {
		t.Errorf("expected summed manufacture carbon 35000, got %v",
			got.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
	}
	if got.TimeSaved.Minutes.Mean != 60 {
		t.Errorf("expected summed minutes 60, got %v", got.TimeSaved.Minutes.Mean)
	}
	// Quadrature: sqrt(3^2 + 3^2) ≈ 4.2426
	if s := got.MoneySaved.ValueUsd.Stddev; s < 4.24 || s > 4.25 {
		t.Errorf("expected quadrature stddev ~4.243, got %v", s)
	}
	if got.QualityTime != nil {
		t.Error("quality time must stay the request's own — never adopted")
	}
}

func TestSumTransferImpactDimensions_EmptyAndAllNil(t *testing.T) {
	for name, items := range map[string][]*api.ImpactEstimate{
		"empty":   {},
		"all-nil": {nil, nil},
	} {
		got := SumTransferImpactDimensions(items)
		if got == nil {
			t.Fatalf("%s: expected non-nil estimate", name)
		}
		if got.MoneySaved != nil || got.EmissionsPrevented != nil || got.TimeSaved != nil {
			t.Errorf("%s: expected nil dimensions, got %+v", name, got)
		}
	}
}
