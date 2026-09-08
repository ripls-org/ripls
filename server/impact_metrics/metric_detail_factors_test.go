package impact_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestComputeFactors tests contributing factor computation.
func TestComputeFactors(t *testing.T) {
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

	// Insert mixed transactions
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(100.0, 5000.0, 30.0), now)
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		testImpactEstimate(50.0, 2500.0, 0.0), now)
	insertFulfilledRequestWithTime(t, db, communityID,
		testImpactEstimate(0.0, 1000.0, 15.0), now)
	insertCompletedExperienceWithTime(t, db, communityID,
		testImpactEstimate(0.0, 3000.0, 60.0), now)

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	if result.Factors == nil {
		t.Fatal("Factors is nil")
	}

	// Should have factors for Loans, Giveaways, and possibly Items
	if len(result.Factors) < 2 {
		t.Errorf("Factors length = %d, want at least 2", len(result.Factors))
	}

	// Verify each factor has required fields
	for i, factor := range result.Factors {
		if factor.Count == 0 {
			t.Errorf("Factors[%d].Count = 0, want > 0", i)
		}
	}
}

// TestComputeItemsFactor tests the Items factor for Money dimension.
func TestComputeItemsFactor(t *testing.T) {
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

	// Create user
	user := &models.User{
		Id:   uuid.New().String(),
		Name: "Test User",
	}
	if _, err := db.Insert(ctx, user); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Create gear items with values
	for i := 0; i < 3; i++ {
		gear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: user.Id,
			Name:    "Test Gear",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 100.0,
			},
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Link to community
		communityGear := &models.CommunityGear{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			GearId:      gear.Id,
		}
		if _, err := db.Insert(ctx, communityGear); err != nil {
			t.Fatalf("Failed to insert community gear: %v", err)
		}
	}

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	if result.Factors == nil {
		t.Fatal("Factors is nil")
	}

	// The Items factor is emitted first for the Money dimension. Factors no
	// longer carry a label to match on (#2835), so position is the contract.
	if len(result.Factors) == 0 {
		t.Fatal("Items factor not found")
	}
	itemsFactor := result.Factors[0]

	// Verify items factor data
	if itemsFactor.Count != 3 {
		t.Errorf("Items factor Count = %d, want 3", itemsFactor.Count)
	}
}

// TestFactorsEmpty tests that empty communities handle factors gracefully.
func TestFactorsEmpty(t *testing.T) {
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

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	// Factors can be nil or empty for empty communities
	if len(result.Factors) > 0 {
		t.Errorf("Expected empty or nil Factors for empty community, got %d factors", len(result.Factors))
	}
}
