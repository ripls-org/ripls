package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestCalculateImpactSavings tests the unified impact savings calculation.
func TestCalculateImpactSavings(t *testing.T) {
	t.Run("empty community", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)
		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		assertEstimateZero(t, "CostSavings", result.CostSavings)
		assertEstimateZero(t, "CarbonSavings", result.CarbonSavings)
		assertEstimateZero(t, "TimeBanked", result.TimeBanked)
		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0", result.CostCount)
		}
		if result.CarbonCount != 0 {
			t.Errorf("CarbonCount = %d, want 0", result.CarbonCount)
		}
	})

	t.Run("completed loan with ImpactEstimate", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(100, 5000, 60), false)

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "CostSavings", result.CostSavings, 100)
		if result.CostCount != 1 {
			t.Errorf("CostCount = %d, want 1", result.CostCount)
		}
		if result.CarbonSavings.Mean <= 0 {
			t.Errorf("CarbonSavings.Mean = %v, want > 0", result.CarbonSavings.Mean)
		}
		if result.CarbonCount != 1 {
			t.Errorf("CarbonCount = %d, want 1", result.CarbonCount)
		}
		assertEstimateMean(t, "TimeFromLoans", result.TimeFromLoans, 60)
	})

	t.Run("completed giveaway included in totals", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(200, 8000, 30), false)

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "CostSavings", result.CostSavings, 200)
		if result.CostCount != 1 {
			t.Errorf("CostCount = %d, want 1", result.CostCount)
		}
	})

	t.Run("fulfilled request with ImpactEstimate", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)
		insertFulfilledRequest(t, db, communityID, testImpactEstimate(0, 3000, 45))

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		// Requests have no cost savings.
		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0 (requests have no cost)", result.CostCount)
		}
		if result.CarbonSavings.Mean <= 0 {
			t.Errorf("CarbonSavings.Mean = %v, want > 0", result.CarbonSavings.Mean)
		}
		assertEstimateMean(t, "TimeFromRequests", result.TimeFromRequests, 45)
	})

	t.Run("completed experience with ImpactEstimate", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)
		insertCompletedExperience(t, db, communityID, testImpactEstimate(0, 0, 90))

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "TimeFromSkills", result.TimeFromSkills, 90)
	})

	t.Run("mixed transactions summed with quadrature", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)

		// 2 loans: cost 100 and 200, time 60 and 90
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(100, 5000, 60), false)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(200, 8000, 90), false)

		// 1 request: time 45
		insertFulfilledRequest(t, db, communityID, testImpactEstimate(0, 3000, 45))

		// 1 experience: time 120
		insertCompletedExperience(t, db, communityID, testImpactEstimate(0, 0, 120))

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "CostSavings", result.CostSavings, 300)
		if result.CostCount != 2 {
			t.Errorf("CostCount = %d, want 2", result.CostCount)
		}

		// Time from loans: 60 + 90 = 150
		assertEstimateMean(t, "TimeFromLoans", result.TimeFromLoans, 150)
		assertEstimateMean(t, "TimeFromRequests", result.TimeFromRequests, 45)
		assertEstimateMean(t, "TimeFromSkills", result.TimeFromSkills, 120)
		// Total: 150 + 45 + 120 = 315
		assertEstimateMean(t, "TimeBanked", result.TimeBanked, 315)

		// Stddev should be > 0 (quadrature sum)
		if result.CostSavings.Stddev <= 0 {
			t.Errorf("CostSavings.Stddev = %v, want > 0 (quadrature)", result.CostSavings.Stddev)
		}
	})

	t.Run("legacy transfer without ImpactEstimate uses fallback", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)

		// Insert transfer with no ImpactEstimate (legacy), along with gear.
		user := createTestUser(t, db, "Owner")
		gear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: user.Id,
			Name:    "Test Item",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 75.0,
			},
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		transfer := &models.Transfer{
			Id:           uuid.New().String(),
			CommunityId:  communityID,
			GearId:       gear.Id,
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		}
		if _, err := db.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		// Fallback should produce non-zero values via builder.
		if result.CostSavings.Mean <= 0 {
			t.Errorf("CostSavings.Mean = %v, want > 0 (fallback from gear value)", result.CostSavings.Mean)
		}
		if result.CostCount != 1 {
			t.Errorf("CostCount = %d, want 1", result.CostCount)
		}
		if result.TimeBanked.Mean <= 0 {
			t.Errorf("TimeBanked.Mean = %v, want > 0 (fallback time)", result.TimeBanked.Mean)
		}
	})

	t.Run("soft-deleted and active transfers excluded", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		defer db.Close()
		calc := NewCalculator(db, loadTestEstimatorConfig(t))

		communityID := createTestCommunity(t, db)

		// Active loan (excluded - not completed)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_ACTIVE, testImpactEstimate(100, 5000, 60), false)

		// Soft-deleted completed loan (excluded)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(200, 8000, 90), true)

		// Valid completed loan (included)
		insertTransfer(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			models.TransferState_TRANSFER_STATE_COMPLETED, testImpactEstimate(50, 2000, 30), false)

		result, err := calc.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			t.Fatalf("CalculateImpactSavings() error = %v", err)
		}

		// Only the valid completed loan should be counted.
		assertEstimateMean(t, "CostSavings", result.CostSavings, 50)
		if result.CostCount != 1 {
			t.Errorf("CostCount = %d, want 1", result.CostCount)
		}
	})
}

// TestCostEstimate tests the CostEstimate helper.
func TestCostEstimate(t *testing.T) {
	if CostEstimate(nil) != nil {
		t.Error("CostEstimate(nil) should be nil")
	}
	ie := &api.ImpactEstimate{
		MoneySaved: &api.MoneySavings{
			ValueUsd: &api.Estimate{Mean: 42, Stddev: 5},
		},
	}
	e := CostEstimate(ie)
	if e == nil || e.Mean != 42 {
		t.Errorf("CostEstimate() = %v, want mean=42", e)
	}
}

// TestCarbonEstimate tests the CarbonEstimate helper.
func TestCarbonEstimate(t *testing.T) {
	if CarbonEstimate(nil) != nil {
		t.Error("CarbonEstimate(nil) should be nil")
	}
	ie := &api.ImpactEstimate{
		EmissionsPrevented: &api.PreventedEmissions{
			ManufactureAvoidedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: 3000, Stddev: 300},
			},
			WasteReducedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: 1000, Stddev: 100},
			},
		},
	}
	e := CarbonEstimate(ie)
	if e == nil {
		t.Fatal("CarbonEstimate() should not be nil")
	}
	if e.Mean != 4000 {
		t.Errorf("CarbonEstimate().Mean = %v, want 4000", e.Mean)
	}
	// Stddev should be sqrt(300^2 + 100^2) ≈ 316.2
	expectedStddev := float32(math.Sqrt(300*300 + 100*100))
	if math.Abs(float64(e.Stddev-expectedStddev)) > 0.1 {
		t.Errorf("CarbonEstimate().Stddev = %v, want ≈%v", e.Stddev, expectedStddev)
	}
}

// TestTimeEstimate tests the TimeEstimate helper.
func TestTimeEstimate(t *testing.T) {
	if TimeEstimate(nil) != nil {
		t.Error("TimeEstimate(nil) should be nil")
	}
	ie := &api.ImpactEstimate{
		TimeSaved: &api.TimeSavings{
			Minutes: &api.Estimate{Mean: 120, Stddev: 20},
		},
	}
	e := TimeEstimate(ie)
	if e == nil || e.Mean != 120 {
		t.Errorf("TimeEstimate() = %v, want mean=120", e)
	}
}

// TestQualityTimeMinutes_ReturnsCompositeNotTimeSaved verifies the helper that
// experience/request stories use to populate Story.TimeSavedMinutes. The
// completion modal's hero "time" metric is QualityTime.QualityTimeMinutes,
// not TimeSaved.Minutes — these are different values for the same impact.
//
// Regression guard: an earlier implementation populated stories from
// TimeMinutes (TimeSaved), causing the story-card "time" number to disagree
// with the completion modal.
func TestQualityTimeMinutes_ReturnsCompositeNotTimeSaved(t *testing.T) {
	if QualityTimeMinutes(nil) != 0 {
		t.Error("QualityTimeMinutes(nil) should be 0")
	}
	// Build an estimate where QualityTime and TimeSaved are intentionally
	// different so any wrong-field reach picks up the wrong number.
	ie := &api.ImpactEstimate{
		QualityTime: &api.QualityTimeEstimate{
			QualityTimeMinutes: &api.Estimate{Mean: 183, Stddev: 22},
		},
		TimeSaved: &api.TimeSavings{
			Minutes: &api.Estimate{Mean: 30, Stddev: 5},
		},
	}
	if got := QualityTimeMinutes(ie); got != 183 {
		t.Errorf("QualityTimeMinutes() = %v, want 183 (must not return TimeSaved.Minutes=30)", got)
	}
	if got := TimeMinutes(ie); got != 30 {
		t.Errorf("TimeMinutes() = %v, want 30 (must remain decoupled from QualityTime)", got)
	}
}

// TestCalculateCo2Potential tests the CO2 potential calculation for community gear.
func TestCalculateCo2Potential(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	cfg := loadTestEstimatorConfig(t)

	communityID := createTestCommunity(t, db)
	ctx := context.Background()

	// Empty community: expect 0.
	calc := NewCalculator(db, cfg)
	potential, err := calc.CalculateCo2Potential(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateCo2Potential() error = %v", err)
	}
	if potential != 0 {
		t.Errorf("empty community: got %v, want 0", potential)
	}

	// Add gear with embodied carbon.
	gear := &models.Gear{
		Id:   uuid.New().String(),
		Name: "Drill",
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 5000, Stddev: 500},
		},
	}
	if _, err := db.Insert(ctx, gear); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	cg := &models.CommunityGear{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		GearId:      gear.Id,
	}
	if _, err := db.Insert(ctx, cg); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}

	potential, err = calc.CalculateCo2Potential(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateCo2Potential() error = %v", err)
	}
	if math.Abs(float64(potential-5000)) > 0.01 {
		t.Errorf("single gear: got %v, want 5000", potential)
	}
}

// TestCalculateTimeToSolveMedian tests the median time-to-solve computation.
func TestCalculateTimeToSolveMedian(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	cfg := loadTestEstimatorConfig(t)

	communityID := createTestCommunity(t, db)
	ctx := context.Background()
	calc := NewCalculator(db, cfg)

	// No requests: expect 0.
	median, err := calc.CalculateTimeToSolveMedian(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateTimeToSolveMedian() error = %v", err)
	}
	if median != 0 {
		t.Errorf("no requests: got %v, want 0", median)
	}

	// Add two fulfilled requests with known solve times.
	now := time.Now().Unix()
	for _, solveSec := range []int64{600, 1800} { // 10 min, 30 min
		fulfilledAt := now
		sharedAt := now - solveSec
		reqID := uuid.New().String()
		req := &models.Request{
			Id:                 reqID,
			State:              models.RequestState_REQUEST_STATE_FULFILLED,
			FulfilledAtUnixSec: &fulfilledAt,
		}
		if _, err := db.Insert(ctx, req); err != nil {
			t.Fatalf("insert request: %v", err)
		}
		cr := &models.CommunityRequest{
			Id:              uuid.New().String(),
			CommunityId:     communityID,
			RequestId:       reqID,
			SharedAtUnixSec: sharedAt,
		}
		if _, err := db.Insert(ctx, cr); err != nil {
			t.Fatalf("insert community request: %v", err)
		}
	}

	median, err = calc.CalculateTimeToSolveMedian(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateTimeToSolveMedian() error = %v", err)
	}
	// Median of [10, 30] = 20 minutes.
	if math.Abs(float64(median)-20) > 0.5 {
		t.Errorf("two requests: got %v, want ~20", median)
	}
}
