package impact_metrics

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestComputeMoneyDetail tests Money dimension-specific detail computation.
func TestComputeMoneyDetail(t *testing.T) {
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

	// Create gear items
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

	// Compute money detail (total library value = $300, total saved = $150)
	moneyDetail, err := calc.computeMoneyDetail(ctx, communityID, 150.0)
	if err != nil {
		t.Fatalf("computeMoneyDetail() error = %v", err)
	}

	if moneyDetail.LibraryValue == "" {
		t.Error("LibraryValue should not be empty")
	}
	if moneyDetail.UtilizationRate == "" {
		t.Error("UtilizationRate should not be empty")
	}
	if moneyDetail.SummaryText == "" {
		t.Error("SummaryText should not be empty")
	}
}

// TestComputeTimeDetail tests Time dimension-specific detail computation.
func TestComputeTimeDetail(t *testing.T) {
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

	// Create community members
	for i := 0; i < 3; i++ {
		user := &models.User{
			Id:   uuid.New().String(),
			Name: "Test User",
		}
		if _, err := db.Insert(ctx, user); err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}

		communityUser := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      user.Id,
		}
		if _, err := db.Insert(ctx, communityUser); err != nil {
			t.Fatalf("Failed to insert community user: %v", err)
		}
	}

	// Compute time detail (180 minutes = 3 hours, 3 members)
	timeDetail, err := calc.computeTimeDetail(ctx, communityID, 180.0)
	if err != nil {
		t.Fatalf("computeTimeDetail() error = %v", err)
	}

	if timeDetail.TotalHours != 3 {
		t.Errorf("TotalHours = %d, want 3", timeDetail.TotalHours)
	}
	if timeDetail.PerPersonHours != 1 {
		t.Errorf("PerPersonHours = %d, want 1 (3 hours / 3 members)", timeDetail.PerPersonHours)
	}
	if timeDetail.SpeedComparison == "" {
		t.Error("SpeedComparison should not be empty")
	}
}

// TestComputeCO2Detail tests CO2 dimension-specific detail computation.
func TestComputeCO2Detail(t *testing.T) {
	calc := NewMetricDetailCalculator(nil, loadTestEstimatorConfig(t))

	monthlyBars := []*api.TimeSeriesPoint{
		{Value: 5.0},
		{Value: 10.0},
		{Value: 15.0},
	}

	// Compute CO2 detail (50,000 grams = 50 kg)
	co2Detail := calc.computeCO2Detail(50000.0, monthlyBars, 10.0)

	if co2Detail.CurrentKg != 50.0 {
		t.Errorf("CurrentKg = %v, want 50.0", co2Detail.CurrentKg)
	}
	if co2Detail.GoalKg != 75.0 {
		t.Errorf("GoalKg = %v, want 75.0 (default)", co2Detail.GoalKg)
	}
	if co2Detail.Equivalence == "" {
		t.Error("Equivalence should not be empty")
	}
	if co2Detail.MonthlyAverage != 10.0 {
		t.Errorf("MonthlyAverage = %v, want 10.0", co2Detail.MonthlyAverage)
	}
	if len(co2Detail.MonthlyBars) != 3 {
		t.Errorf("MonthlyBars length = %d, want 3", len(co2Detail.MonthlyBars))
	}
}
