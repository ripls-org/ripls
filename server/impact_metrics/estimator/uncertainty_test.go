package estimator

import (
	"math"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

const tolerance = 0.01

func assertEstimate(t *testing.T, label string, got *api.Estimate, wantMean, wantStddev float32) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil estimate", label)
	}
	if math.Abs(float64(got.Mean-wantMean)) > float64(tolerance) {
		t.Errorf("%s: mean = %f, want %f", label, got.Mean, wantMean)
	}
	if math.Abs(float64(got.Stddev-wantStddev)) > float64(tolerance) {
		t.Errorf("%s: stddev = %f, want %f", label, got.Stddev, wantStddev)
	}
}

func TestRelativeUncertainty(t *testing.T) {
	tests := []struct {
		name       string
		mean       float32
		relStddev  float32
		wantMean   float32
		wantStddev float32
	}{
		{"40% of 100", 100, 0.40, 100, 40},
		{"15% of 200", 200, 0.15, 200, 30},
		{"zero mean", 0, 0.50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeUncertainty(tt.mean, tt.relStddev)
			assertEstimate(t, tt.name, got, tt.wantMean, tt.wantStddev)
		})
	}
}

func TestFromRange(t *testing.T) {
	// stddev = (30 - 10) / 4 = 5
	got := FromRange(20, 10, 30)
	assertEstimate(t, "20 [10-30]", got, 20, 5)

	// stddev = (50 - 10) / 4 = 10
	got = FromRange(30, 10, 50)
	assertEstimate(t, "30 [10-50]", got, 30, 10)
}

func TestSumEstimates(t *testing.T) {
	// sum of [10±3, 10±4] = [20, sqrt(9+16)] = [20, 5]
	a := &api.Estimate{Mean: 10, Stddev: 3}
	b := &api.Estimate{Mean: 10, Stddev: 4}
	got := SumEstimates([]*api.Estimate{a, b})
	assertEstimate(t, "sum [10±3, 10±4]", got, 20, 5)
}

func TestSumEstimatesEmpty(t *testing.T) {
	got := SumEstimates(nil)
	assertEstimate(t, "empty sum", got, 0, 0)
}

func TestSumEstimatesWithNil(t *testing.T) {
	a := &api.Estimate{Mean: 10, Stddev: 3}
	got := SumEstimates([]*api.Estimate{a, nil})
	assertEstimate(t, "sum with nil", got, 10, 3)
}

func TestScaleEstimate(t *testing.T) {
	// [100±30] * 2 = [200±60]
	e := &api.Estimate{Mean: 100, Stddev: 30}
	got := ScaleEstimate(e, 2)
	assertEstimate(t, "scale by 2", got, 200, 60)
}

func TestScaleEstimateNegativeFactor(t *testing.T) {
	e := &api.Estimate{Mean: 100, Stddev: 30}
	got := ScaleEstimate(e, -2)
	assertEstimate(t, "scale by -2", got, -200, 60)
}

func TestScaleEstimateNil(t *testing.T) {
	got := ScaleEstimate(nil, 2)
	assertEstimate(t, "scale nil", got, 0, 0)
}

func TestCombineEstimatesReducesUncertainty(t *testing.T) {
	// Two estimates that agree: [100±30, 100±40]
	a := &api.Estimate{Mean: 100, Stddev: 30}
	b := &api.Estimate{Mean: 100, Stddev: 40}
	got := CombineEstimates(a, b)

	// Combined stddev should be less than either individual.
	if got.Stddev >= a.Stddev || got.Stddev >= b.Stddev {
		t.Errorf("combined stddev %f should be less than both %f and %f", got.Stddev, a.Stddev, b.Stddev)
	}
	// Mean should be close to 100 (precision-weighted toward the more precise one).
	assertEstimate(t, "combined", got, 100, 24)
}

func TestCombineEstimatesNilHandling(t *testing.T) {
	a := &api.Estimate{Mean: 100, Stddev: 30}

	got := CombineEstimates(nil, a)
	assertEstimate(t, "nil + a", got, 100, 30)

	got = CombineEstimates(a, nil)
	assertEstimate(t, "a + nil", got, 100, 30)

	// Both nil.
	if got := CombineEstimates(nil, nil); got != nil {
		t.Error("CombineEstimates(nil, nil) should return nil")
	}
}

func TestCombineEstimatesZeroStddev(t *testing.T) {
	exact := &api.Estimate{Mean: 42, Stddev: 0}
	uncertain := &api.Estimate{Mean: 50, Stddev: 10}

	got := CombineEstimates(exact, uncertain)
	assertEstimate(t, "exact + uncertain", got, 42, 0)

	got = CombineEstimates(uncertain, exact)
	assertEstimate(t, "uncertain + exact", got, 42, 0)

	got = CombineEstimates(exact, &api.Estimate{Mean: 45, Stddev: 0})
	assertEstimate(t, "exact + exact", got, 43.5, 0)
}

func TestCombineEstimatesWeightsTowardMorePrecise(t *testing.T) {
	// a is much more precise, so combined mean should be closer to a.
	a := &api.Estimate{Mean: 100, Stddev: 5}
	b := &api.Estimate{Mean: 200, Stddev: 50}
	got := CombineEstimates(a, b)

	if got.Mean > 110 {
		t.Errorf("combined mean %f should be close to 100 (the more precise estimate), not %f", got.Mean, got.Mean)
	}
}
