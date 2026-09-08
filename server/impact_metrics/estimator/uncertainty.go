package estimator

import (
	"math"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// RelativeUncertainty creates an Estimate from a mean and relative standard deviation.
// relStddev is a fraction of mean (e.g., 0.40 means stddev = 40% of mean).
func RelativeUncertainty(mean, relStddev float32) *api.Estimate {
	return &api.Estimate{
		Mean:   mean,
		Stddev: mean * relStddev,
	}
}

// FromRange creates an Estimate from a mean and a range, treating the range as
// approximately a 95% confidence interval: stddev = (high - low) / 4.
func FromRange(mean, rangeLow, rangeHigh float32) *api.Estimate {
	return &api.Estimate{
		Mean:   mean,
		Stddev: (rangeHigh - rangeLow) / 4.0,
	}
}

// CombineEstimates produces a precision-weighted average of two estimates.
// This is used when two independent methods agree (within ~1 stddev of each other).
// The combined uncertainty is lower than either input — the correct statistical
// behavior when independent estimates corroborate each other.
func CombineEstimates(a, b *api.Estimate) *api.Estimate {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// Avoid division by zero: if both have zero stddev, just average.
	if a.Stddev == 0 && b.Stddev == 0 {
		return &api.Estimate{
			Mean:   (a.Mean + b.Mean) / 2,
			Stddev: 0,
		}
	}
	// If one has zero stddev (exact value), return it.
	if a.Stddev == 0 {
		return &api.Estimate{Mean: a.Mean, Stddev: 0}
	}
	if b.Stddev == 0 {
		return &api.Estimate{Mean: b.Mean, Stddev: 0}
	}

	// Precision-weighted (inverse-variance) combination.
	va := a.Stddev * a.Stddev
	vb := b.Stddev * b.Stddev
	combinedVariance := 1.0 / (1.0/va + 1.0/vb)
	wa := combinedVariance / va
	wb := combinedVariance / vb

	return &api.Estimate{
		Mean:   wa*a.Mean + wb*b.Mean,
		Stddev: float32(math.Sqrt(float64(combinedVariance))),
	}
}

// SumEstimates sums a slice of estimates with quadrature uncertainty propagation.
// total_mean = sum(mean_i), total_stddev = sqrt(sum(stddev_i^2)).
func SumEstimates(estimates []*api.Estimate) *api.Estimate {
	if len(estimates) == 0 {
		return &api.Estimate{}
	}

	var totalMean float32
	var sumVariance float32
	for _, e := range estimates {
		if e == nil {
			continue
		}
		totalMean += e.Mean
		sumVariance += e.Stddev * e.Stddev
	}

	return &api.Estimate{
		Mean:   totalMean,
		Stddev: float32(math.Sqrt(float64(sumVariance))),
	}
}

// ScaleEstimate multiplies an estimate by a scalar factor.
// Both mean and stddev are scaled linearly (stddev by absolute value of factor).
func ScaleEstimate(e *api.Estimate, factor float32) *api.Estimate {
	if e == nil {
		return &api.Estimate{}
	}
	absFactor := float32(math.Abs(float64(factor)))
	return &api.Estimate{
		Mean:   e.Mean * factor,
		Stddev: e.Stddev * absFactor,
	}
}
