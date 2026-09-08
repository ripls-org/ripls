package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestBriefAndMetricDetailAgree locks the invariant that the Workshop
// home tile (driven by Calculator.CalculateImpactSavings) and the
// Workshop metric detail screen (driven by MetricDetailCalculator.
// ComputeMetricDetail) report the same total for every dimension on
// the same community. Without this guarantee the headline number on
// the detail screen drifts away from the value the home tile shows
// (#1898). MONEY, EMISSIONS, and QUALITY_TIME are all covered.
func TestBriefAndMetricDetailAgree(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	cfg := loadTestEstimatorConfig(t)
	briefCalc := NewCalculator(db, cfg)
	detailCalc := NewMetricDetailCalculator(db, cfg)

	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Agreement Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now().Unix()

	// Build estimates that exercise every dimension across every source
	// type. Each estimate carries explicit cost + carbon + time + quality
	// time so neither the brief nor the breakdown can silently drop a
	// dimension via a nil-estimate fallback.
	loanA := testImpactEstimateWithQT(120.0, 6000.0, 30.0, 45.0)
	loanB := testImpactEstimateWithQT(80.0, 4000.0, 45.0, 60.0)
	giveaway := testImpactEstimateWithQT(50.0, 2500.0, 0.0, 30.0)
	request := testImpactEstimateWithQT(40.0, 3500.0, 90.0, 25.0)
	experience := testImpactEstimateWithQT(75.0, 2000.0, 120.0, 90.0)

	insertTransferWithTime(t, db, communityID,
		models.TransferType_TRANSFER_TYPE_LOAN, loanA, now)
	insertTransferWithTime(t, db, communityID,
		models.TransferType_TRANSFER_TYPE_LOAN, loanB, now)
	insertTransferWithTime(t, db, communityID,
		models.TransferType_TRANSFER_TYPE_GIVEAWAY, giveaway, now)
	insertFulfilledRequestWithTime(t, db, communityID, request, now)
	insertCompletedExperienceWithTime(t, db, communityID, experience, now)

	brief, err := briefCalc.CalculateImpactSavings(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateImpactSavings() error = %v", err)
	}

	const eps = 0.5

	type dimensionCase struct {
		name       string
		dimension  api.ImpactMetricDimension
		briefValue float32
		// breakdownSum derived from rendered SourceBreakdown rows — what
		// the detail screen actually displays after #1898.
	}
	cases := []dimensionCase{
		{"money", api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, brief.CostSavings.Mean},
		{"emissions", api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS, brief.CarbonSavings.Mean},
		{"quality_time", api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME, brief.QualityTime.Mean},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := detailCalc.ComputeMetricDetail(
				ctx, communityID, tc.dimension,
				api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
			)
			if err != nil {
				t.Fatalf("ComputeMetricDetail(%s) error = %v", tc.name, err)
			}

			// The detail screen's headline number is the sum of the
			// rendered breakdown rows — assert the brief matches that
			// sum, not just TotalValue, since the client renders rows.
			var breakdownSum float64
			for _, row := range result.SourceBreakdown {
				breakdownSum += float64(row.Value)
			}

			if math.Abs(float64(result.TotalValue.Mean)-breakdownSum) > eps {
				t.Errorf("detail TotalValue (%v) and breakdown sum (%v) disagree",
					result.TotalValue.Mean, breakdownSum)
			}
			if math.Abs(float64(tc.briefValue)-breakdownSum) > eps {
				t.Errorf("brief %s = %v, breakdown sum = %v — Workshop home would render a different number from the detail screen",
					tc.name, tc.briefValue, breakdownSum)
			}
		})
	}
}

// testImpactEstimateWithQT extends [testImpactEstimate] with an
// explicit QualityTime field. Used by tests that exercise the
// QUALITY_TIME dimension, since [testImpactEstimate] only populates
// TimeSaved (logistical) and leaves QualityTime nil.
func testImpactEstimateWithQT(
	costUSD, carbonGrams, timeMinutes, qtMinutes float32,
) *models.ImpactEstimate {
	ie := testImpactEstimate(costUSD, carbonGrams, timeMinutes)
	if qtMinutes > 0 {
		ie.QualityTime = &models.QualityTimeEstimate{
			QualityTimeMinutes: &models.Estimate{
				Mean:   qtMinutes,
				Stddev: qtMinutes * 0.2,
			},
		}
	}
	return ie
}
