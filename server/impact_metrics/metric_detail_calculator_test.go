package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestComputeMetricDetail_EmptyCommunity tests that an empty community returns zeros, not errors.
func TestComputeMetricDetail_EmptyCommunity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create empty community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Empty Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// Test each dimension
	dimensions := []api.ImpactMetricDimension{
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS,
	}

	for _, dimension := range dimensions {
		t.Run(dimension.String(), func(t *testing.T) {
			result, err := calc.ComputeMetricDetail(ctx, communityID, dimension, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
			if err != nil {
				t.Fatalf("ComputeMetricDetail() error = %v, want nil (empty community should not error)", err)
			}

			if result.TotalValue.Mean != 0 {
				t.Errorf("TotalValue.Mean = %v, want 0 for empty community", result.TotalValue.Mean)
			}

			if len(result.SourceBreakdown) != 0 {
				t.Errorf("SourceBreakdown length = %d, want 0 for empty community", len(result.SourceBreakdown))
			}
		})
	}
}

// TestComputeMetricDetail_SingleLoan tests that a single loan is correctly aggregated.
func TestComputeMetricDetail_SingleLoan(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// Insert a single completed loan transfer with impact estimate
	ie := testImpactEstimate(100.0, 5000.0, 30.0) // $100, 5kg CO2, 30 minutes
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		GearId:              uuid.New().String(),
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualReturnUnixSec: proto.Int64(time.Now().Unix()),
		ImpactEstimate:      ie,
	}
	if _, err := db.Insert(ctx, transfer); err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}

	// Test money dimension
	t.Run("Money", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		// Total should equal the loan value
		if math.Abs(float64(result.TotalValue.Mean-100.0)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want 100.0", result.TotalValue.Mean)
		}

		// Should have one breakdown entry: Loans at 100%
		if len(result.SourceBreakdown) != 1 {
			t.Fatalf("SourceBreakdown length = %d, want 1", len(result.SourceBreakdown))
		}

		if result.SourceBreakdown[0].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS {
			t.Errorf("SourceBreakdown[0].SourceType = %v, want LOANS", result.SourceBreakdown[0].SourceType)
		}
		if result.SourceBreakdown[0].Percentage != 100 {
			t.Errorf("SourceBreakdown[0].Percentage = %v, want 100", result.SourceBreakdown[0].Percentage)
		}
	})

	// Test time dimension
	t.Run("Time", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		if math.Abs(float64(result.TotalValue.Mean-30.0)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want 30.0", result.TotalValue.Mean)
		}
	})

	// Test CO2 dimension
	t.Run("CO2", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		// CO2 is stored in grams
		if math.Abs(float64(result.TotalValue.Mean-5000.0)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want 5000.0 grams", result.TotalValue.Mean)
		}
	})
}

// TestComputeMetricDetail_MixedTransactions tests mixed transaction types with correct breakdown.
func TestComputeMetricDetail_MixedTransactions(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now().Unix()

	// Insert 2 loans
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(100.0, 5000.0, 30.0), now)
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(200.0, 10000.0, 60.0), now)

	// Insert 1 giveaway
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		testImpactEstimate(150.0, 7500.0, 0.0), now) // Giveaways don't have time savings

	// Insert 1 fulfilled request
	insertFulfilledRequestWithTime(t, db, communityID,
		testImpactEstimate(0.0, 2500.0, 45.0), now) // Requests have carbon and time, no money

	// Insert 1 completed experience
	insertCompletedExperienceWithTime(t, db, communityID,
		testImpactEstimate(0.0, 3000.0, 120.0), now) // Experiences have carbon and time, no money

	// Test money dimension
	t.Run("Money", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		// Total: 100 + 200 + 150 = 450
		wantTotal := float32(450.0)
		if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want %v", result.TotalValue.Mean, wantTotal)
		}

		// Should have 2 sources: Loans (66%) and Giveaways (33%)
		if len(result.SourceBreakdown) != 2 {
			t.Fatalf("SourceBreakdown length = %d, want 2", len(result.SourceBreakdown))
		}

		// Check Loans
		loansBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS)
		if loansBreakdown == nil {
			t.Fatalf("Loans breakdown not found")
		}
		if math.Abs(float64(loansBreakdown.Value-300.0)) > 0.01 {
			t.Errorf("Loans Value = %v, want 300.0", loansBreakdown.Value)
		}
		// Percentage should be ~67% (300/450)
		if loansBreakdown.Percentage < 66 || loansBreakdown.Percentage > 67 {
			t.Errorf("Loans Percentage = %v, want 66-67", loansBreakdown.Percentage)
		}

		// Check Giveaways
		giveawaysBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS)
		if giveawaysBreakdown == nil {
			t.Fatalf("Giveaways breakdown not found")
		}
		if math.Abs(float64(giveawaysBreakdown.Value-150.0)) > 0.01 {
			t.Errorf("Giveaways Value = %v, want 150.0", giveawaysBreakdown.Value)
		}

		// Recent items carry the typed value and omit item_name when the
		// underlying gear rows don't exist — clients render a localized
		// fallback from kind (#2827).
		if len(result.RecentItems) == 0 {
			t.Fatal("RecentItems empty, want at least the returned loans")
		}
		for i, item := range result.RecentItems {
			if item.RawValue == nil {
				t.Errorf("RecentItems[%d].RawValue absent, want the dimension value", i)
			}
			if item.ItemName != nil {
				t.Errorf("RecentItems[%d].ItemName = %q, want absent (no gear row)", i, *item.ItemName)
			}
		}
	})

	// Test time dimension
	t.Run("Time", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		// Total: 30 + 60 + 45 + 120 = 255 minutes
		wantTotal := float32(255.0)
		if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want %v", result.TotalValue.Mean, wantTotal)
		}

		// Time dimension uses 3 sources: "Loans", "Requests", "Events".
		// Giveaways are folded into Requests for the Time dimension.
		if len(result.SourceBreakdown) != 3 {
			t.Fatalf("SourceBreakdown length = %d, want 3", len(result.SourceBreakdown))
		}

		// Verify Time-specific segments carry the canonical typed sources.
		loanTimeBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS)
		if loanTimeBreakdown == nil {
			t.Fatalf("LOANS breakdown not found; got sources: %v", breakdownLabels(result.SourceBreakdown))
		}
		// Loans total: 30 + 60 = 90 minutes.
		if math.Abs(loanTimeBreakdown.Value-90.0) > 0.01 {
			t.Errorf("Loans Value = %v, want 90.0", loanTimeBreakdown.Value)
		}

		helpBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS)
		if helpBreakdown == nil {
			t.Fatalf("REQUESTS breakdown not found; got sources: %v", breakdownLabels(result.SourceBreakdown))
		}
		// Requests: 0 (giveaway time) + 45 (request time) = 45 minutes.
		if math.Abs(helpBreakdown.Value-45.0) > 0.01 {
			t.Errorf("Requests Value = %v, want 45.0", helpBreakdown.Value)
		}

		eventBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS)
		if eventBreakdown == nil {
			t.Fatalf("EVENTS breakdown not found; got sources: %v", breakdownLabels(result.SourceBreakdown))
		}

		// Verify no Giveaways segment exists for Time dimension
		if findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS) != nil {
			t.Error("Time dimension should not have a GIVEAWAYS segment")
		}
	})

	// Test CO2 dimension
	t.Run("CO2", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		// Total: 5000 + 10000 + 7500 + 2500 + 3000 = 28000 grams
		wantTotal := float32(28000.0)
		if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want %v", result.TotalValue.Mean, wantTotal)
		}

		// Should have 4 sources: Loans, Giveaways, Requests, Events
		if len(result.SourceBreakdown) != 4 {
			t.Fatalf("SourceBreakdown length = %d, want 4", len(result.SourceBreakdown))
		}

		// Verify percentages sum to ~100%
		totalPercentage := float64(0)
		for _, breakdown := range result.SourceBreakdown {
			totalPercentage += breakdown.Percentage
		}
		// Allow for rounding (should be 99-100)
		if totalPercentage < 99 || totalPercentage > 100 {
			t.Errorf("Total percentage = %v, want 99-100", totalPercentage)
		}
	})
}

// TestComputeMetricDetail_PeriodFiltering tests that period filtering excludes old transactions.
func TestComputeMetricDetail_PeriodFiltering(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now()

	// Insert transfers at different times
	recentTime := now.Add(-7 * 24 * time.Hour).Unix() // 1 week ago
	oldTime := now.Add(-60 * 24 * time.Hour).Unix()   // 2 months ago

	// Recent transfer
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(100.0, 0.0, 0.0), recentTime)

	// Old transfer
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(200.0, 0.0, 0.0), oldTime)

	// Test ALL period - should include both
	t.Run("ALL period includes all", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		wantTotal := float32(300.0) // Both transfers
		if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want %v", result.TotalValue.Mean, wantTotal)
		}
	})

	// Test FOUR_WEEKS period - should only include recent
	t.Run("FOUR_WEEKS excludes old", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_FOUR_WEEKS)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		wantTotal := float32(100.0) // Only recent transfer
		if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
			t.Errorf("TotalValue.Mean = %v, want %v (should exclude 2-month-old transfer)", result.TotalValue.Mean, wantTotal)
		}
	})
}

// TestComputeMetricDetail_SkipsDeleted tests that soft-deleted transfers are excluded.
func TestComputeMetricDetail_SkipsDeleted(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now().Unix()

	// Insert active transfer
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(100.0, 0.0, 0.0), now)

	// Insert soft-deleted transfer
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		GearId:              uuid.New().String(),
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualReturnUnixSec: proto.Int64(now),
		ImpactEstimate:      testImpactEstimate(200.0, 0.0, 0.0),
		Deleted:             &models.DeletedMetadata{DeletedAtUnixSec: now},
	}
	if _, err := db.Insert(ctx, transfer); err != nil {
		t.Fatalf("Failed to insert deleted transfer: %v", err)
	}

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	// Should only include the active transfer
	wantTotal := float32(100.0)
	if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
		t.Errorf("TotalValue.Mean = %v, want %v (should exclude deleted transfer)", result.TotalValue.Mean, wantTotal)
	}
}

// insertTransferWithTime inserts a completed transfer with a specific completion time.
func insertTransferWithTime(t *testing.T, db *storage.ProtoSQLStorage, communityID string,
	transferType models.TransferType, ie *models.ImpactEstimate, completedAt int64,
) {
	t.Helper()
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		GearId:              uuid.New().String(),
		TransferType:        transferType,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualReturnUnixSec: proto.Int64(completedAt),
		ImpactEstimate:      ie,
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}
}

// insertFulfilledRequestWithTime inserts a fulfilled request.
// Note: Request model does not have a completion timestamp, so the completedAt parameter
// is ignored. This is left in the signature for test consistency.
func insertFulfilledRequestWithTime(t *testing.T, db *storage.ProtoSQLStorage, communityID string,
	ie *models.ImpactEstimate, _ int64,
) {
	t.Helper()
	ctx := context.Background()
	request := &models.Request{
		Id:             uuid.New().String(),
		State:          models.RequestState_REQUEST_STATE_FULFILLED,
		ImpactEstimate: ie,
	}
	if _, err := db.Insert(ctx, request); err != nil {
		t.Fatalf("Failed to insert request: %v", err)
	}
	cr := &models.CommunityRequest{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		RequestId:   request.Id,
		Archived:    true,
	}
	if _, err := db.Insert(ctx, cr); err != nil {
		t.Fatalf("Failed to insert community request: %v", err)
	}
}

// insertCompletedExperienceWithTime inserts a completed experience with a specific completion time.
func insertCompletedExperienceWithTime(t *testing.T, db *storage.ProtoSQLStorage, communityID string,
	ie *models.ImpactEstimate, completedAt int64,
) {
	t.Helper()
	ctx := context.Background()
	experience := &models.Experience{
		Id:                 uuid.New().String(),
		State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		CompletedAtUnixSec: &completedAt,
		ImpactEstimate:     ie,
	}
	if _, err := db.Insert(ctx, experience); err != nil {
		t.Fatalf("Failed to insert experience: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: experience.Id,
	}
	if _, err := db.Insert(ctx, ce); err != nil {
		t.Fatalf("Failed to insert community experience: %v", err)
	}
}

// findBreakdown finds a SourceBreakdown entry by its typed source — the
// contract clients key on (#2827).
func findBreakdown(breakdowns []*api.SourceBreakdown, sourceType api.ImpactSourceType) *api.SourceBreakdown {
	for _, b := range breakdowns {
		if b.SourceType == sourceType {
			return b
		}
	}
	return nil
}

// breakdownLabels returns all typed sources from breakdown entries for
// diagnostic output.
func breakdownLabels(breakdowns []*api.SourceBreakdown) []api.ImpactSourceType {
	types := make([]api.ImpactSourceType, len(breakdowns))
	for i, b := range breakdowns {
		types[i] = b.SourceType
	}
	return types
}
