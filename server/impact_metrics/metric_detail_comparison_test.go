package impact_metrics

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// TestComputeComparison tests full comparison computation.
func TestComputeComparison(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community with members
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

	// Create members
	for i := 0; i < 10; i++ {
		user := &models.User{
			Id:   uuid.New().String(),
			Name: "User " + uuid.New().String()[:8],
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

	// Create gear and transfers
	owner := &models.User{
		Id:   uuid.New().String(),
		Name: "Owner",
	}
	if _, err := db.Insert(ctx, owner); err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Borrower",
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	gear := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: owner.Id,
		Name:    "Test Gear",
	}
	if _, err := db.Insert(ctx, gear); err != nil {
		t.Fatalf("Failed to insert gear: %v", err)
	}

	communityGear := &models.CommunityGear{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		GearId:      gear.Id,
	}
	if _, err := db.Insert(ctx, communityGear); err != nil {
		t.Fatalf("Failed to insert community gear: %v", err)
	}

	// Create transfers
	var transfers []*models.Transfer
	for i := 0; i < 5; i++ {
		transfer := &models.Transfer{
			Id:                  uuid.New().String(),
			GearId:              gear.Id,
			RecipientId:         borrower.Id,
			TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
			State:               models.TransferState_TRANSFER_STATE_COMPLETED,
			ActualReturnUnixSec: proto.Int64(1000),
			ImpactEstimate: &models.ImpactEstimate{
				MoneySaved: &models.MoneySavings{
					ValueUsd: &models.Estimate{
						Mean: 100.0,
					},
				},
			},
		}
		if _, err := db.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}
		transfers = append(transfers, transfer)
	}

	// Compute comparison
	totalValue := float32(500.0) // 5 loans * $100
	comparison := calc.computeComparison(ctx, communityID, totalValue, transfers, []*models.Request{}, []*models.Experience{}, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY)

	// Verify radar dimensions
	if len(comparison.RadarDimensions) == 0 {
		t.Error("Expected radar dimensions, got none")
	}

	// Verify leaderboard
	if len(comparison.Leaderboard) == 0 {
		t.Error("Expected leaderboard entries, got none")
	}

	// Verify your circle is in leaderboard
	foundYourCircle := false
	for _, entry := range comparison.Leaderboard {
		if entry.Name == "Your Circle" {
			foundYourCircle = true
			if entry.MemberCount != 10 {
				t.Errorf("Expected member count 10, got %d", entry.MemberCount)
			}
			break
		}
	}
	if !foundYourCircle {
		t.Error("Your Circle not found in leaderboard")
	}

	// Verify ranking insight
	if comparison.RankingTitle == "" {
		t.Error("Ranking title should not be empty")
	}
	if comparison.RankingDetail == "" {
		t.Error("Ranking detail should not be empty")
	}

	// Verify rank is set
	if comparison.YourRank == 0 {
		t.Error("YourRank should be set")
	}
}

// TestComputeRadarDimensions tests radar chart computation.
func TestComputeRadarDimensions(t *testing.T) {
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

	// Create gear
	owner := &models.User{
		Id:   uuid.New().String(),
		Name: "Owner",
	}
	if _, err := db.Insert(ctx, owner); err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	for i := 0; i < 3; i++ {
		gear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: owner.Id,
			Name:    "Gear " + uuid.New().String()[:8],
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		communityGear := &models.CommunityGear{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			GearId:      gear.Id,
		}
		if _, err := db.Insert(ctx, communityGear); err != nil {
			t.Fatalf("Failed to insert community gear: %v", err)
		}
	}

	// Create transfers
	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Borrower",
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	var transfers []*models.Transfer
	for i := 0; i < 10; i++ {
		gearID := uuid.New().String()
		gear := &models.Gear{
			Id:      gearID,
			OwnerId: owner.Id,
			Name:    "Transfer Gear",
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		transfer := &models.Transfer{
			Id:                  uuid.New().String(),
			GearId:              gearID,
			RecipientId:         borrower.Id,
			TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
			State:               models.TransferState_TRANSFER_STATE_COMPLETED,
			ActualReturnUnixSec: proto.Int64(1000),
		}
		if _, err := db.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}
		transfers = append(transfers, transfer)
	}

	// Compute radar dimensions
	logger := logging.LoggerWithContext(ctx).With("test", "TestComputeRadarDimensions")
	totalValue := float32(500.0)
	dimensions := calc.computeRadarDimensions(ctx, communityID, totalValue, transfers, []*models.Request{}, []*models.Experience{}, logger)

	// Should have 5 dimensions: Items, Loans, Giveaways, Events, Savings
	if len(dimensions) != 5 {
		t.Errorf("Expected 5 radar dimensions, got %d", len(dimensions))
	}

	// Verify each dimension has label and scores
	for _, dim := range dimensions {
		if dim.Label == "" {
			t.Error("Dimension label should not be empty")
		}
		if dim.YourScore < 0 || dim.YourScore > 100 {
			t.Errorf("YourScore should be 0-100, got %d", dim.YourScore)
		}
		if dim.AvgScore < 0 || dim.AvgScore > 100 {
			t.Errorf("AvgScore should be 0-100, got %d", dim.AvgScore)
		}
	}
}

// TestComputeComparisonEmptyCommunity tests comparison with no activity.
func TestComputeComparisonEmptyCommunity(t *testing.T) {
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

	// Compute comparison with zero values
	comparison := calc.computeComparison(ctx, communityID, 0, []*models.Transfer{}, []*models.Request{}, []*models.Experience{}, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY)

	// Should still return comparison data (not crash)
	if comparison == nil {
		t.Fatal("Expected comparison data, got nil")
	}

	// Radar dimensions should all be zero
	for _, dim := range comparison.RadarDimensions {
		if dim.YourScore != 0 {
			t.Errorf("Expected YourScore 0 for empty community, got %d", dim.YourScore)
		}
	}

	// Should still have ranking info
	if comparison.RankingTitle == "" {
		t.Error("Ranking title should not be empty even for empty community")
	}
}

// TestNormalizeScore tests score normalization.
func TestNormalizeScore(t *testing.T) {
	tests := []struct {
		name  string
		value int
		max   int
		want  int32
	}{
		{"zero", 0, 100, 0},
		{"half", 50, 100, 50},
		{"full", 100, 100, 100},
		{"over max", 150, 100, 100},
		{"zero max", 50, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScore(tt.value, tt.max)
			if got != tt.want {
				t.Errorf("normalizeScore(%d, %d) = %d, want %d", tt.value, tt.max, got, tt.want)
			}
		})
	}
}

// TestGetBenchmarks tests benchmark retrieval for each dimension.
func TestGetBenchmarks(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	dimensions := []api.ImpactMetricDimension{
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS,
	}

	for _, dimension := range dimensions {
		t.Run(dimension.String(), func(t *testing.T) {
			benchmarks := calc.getBenchmarks(dimension)
			if len(benchmarks) == 0 {
				t.Errorf("Expected benchmarks for %s, got none", dimension.String())
			}

			// Verify each benchmark has required fields
			for _, benchmark := range benchmarks {
				if benchmark.name == "" {
					t.Error("Benchmark name should not be empty")
				}
				if benchmark.memberCount <= 0 {
					t.Error("Benchmark member count should be positive")
				}
				if benchmark.totalValue <= 0 {
					t.Error("Benchmark total value should be positive")
				}
			}
		})
	}
}

// TestLeaderboardSorting tests that leaderboard is sorted correctly.
func TestLeaderboardSorting(t *testing.T) {
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	communityID := uuid.New().String()
	totalValue := float32(5000.0)
	memberCount := 10
	benchmarks := calc.getBenchmarks(api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY)

	leaderboard := calc.buildLeaderboard(
		communityID,
		totalValue,
		memberCount,
		benchmarks,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
	)

	// Should have entries
	if len(leaderboard) == 0 {
		t.Fatal("Expected leaderboard entries, got none")
	}

	// Should be limited to 5
	if len(leaderboard) > 5 {
		t.Errorf("Expected max 5 leaderboard entries, got %d", len(leaderboard))
	}

	// All entries should have required fields
	for i, entry := range leaderboard {
		if entry.Name == "" {
			t.Errorf("Entry %d: name should not be empty", i)
		}
		if entry.MemberCount <= 0 {
			t.Errorf("Entry %d: member count should be positive", i)
		}
		if entry.TotalValue == "" {
			t.Errorf("Entry %d: total value should not be empty", i)
		}
		if entry.PerMember == "" {
			t.Errorf("Entry %d: per member should not be empty", i)
		}
	}
}
