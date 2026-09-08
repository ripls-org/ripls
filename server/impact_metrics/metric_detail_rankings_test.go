package impact_metrics

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestComputeTopItems tests gear ranking by cumulative impact.
func TestComputeTopItems(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))
	cache := NewEntityCache(db)

	// Create community and users
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

	user1 := &models.User{
		Id:   uuid.New().String(),
		Name: "Alice",
	}
	user2 := &models.User{
		Id:   uuid.New().String(),
		Name: "Bob",
	}
	if _, err := db.Insert(ctx, user1); err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}
	if _, err := db.Insert(ctx, user2); err != nil {
		t.Fatalf("Failed to insert user2: %v", err)
	}

	// Create gear items
	gear1 := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: user1.Id,
		Name:    "High Impact Item",
		ValueEstimate: &models.ValueEstimate{
			EstimatedValueUsd: 1000.0,
		},
	}
	gear2 := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: user2.Id,
		Name:    "Low Impact Item",
		ValueEstimate: &models.ValueEstimate{
			EstimatedValueUsd: 100.0,
		},
	}
	if _, err := db.Insert(ctx, gear1); err != nil {
		t.Fatalf("Failed to insert gear1: %v", err)
	}
	if _, err := db.Insert(ctx, gear2); err != nil {
		t.Fatalf("Failed to insert gear2: %v", err)
	}

	// Create transfers - gear1 has 3 loans, gear2 has 1 loan
	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Charlie",
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	var transfers []*models.Transfer

	// 3 loans for gear1
	for i := 0; i < 3; i++ {
		transfer := &models.Transfer{
			Id:                  uuid.New().String(),
			GearId:              gear1.Id,
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

	// 1 loan for gear2
	transfer2 := &models.Transfer{
		Id:                  uuid.New().String(),
		GearId:              gear2.Id,
		RecipientId:         borrower.Id,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualReturnUnixSec: proto.Int64(1000),
		ImpactEstimate: &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{
				ValueUsd: &models.Estimate{
					Mean: 50.0,
				},
			},
		},
	}
	if _, err := db.Insert(ctx, transfer2); err != nil {
		t.Fatalf("Failed to insert transfer2: %v", err)
	}
	transfers = append(transfers, transfer2)

	// Compute top items
	topItems := calc.computeTopItems(ctx, transfers, nil, nil, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, cache)

	// Verify ranking — highest cumulative value first.
	if len(topItems) != 2 {
		t.Fatalf("Expected 2 top items, got %d", len(topItems))
	}

	// First item should be gear1 (higher cumulative value)
	if topItems[0].GearId != gear1.Id {
		t.Errorf("Top item should be gear1, got %s", topItems[0].GearId)
	}
	if topItems[0].Name != "High Impact Item" {
		t.Errorf("Top item name = %q, want 'High Impact Item'", topItems[0].Name)
	}
	if topItems[0].SharerName != "Alice" {
		t.Errorf("Top item sharer = %q, want 'Alice'", topItems[0].SharerName)
	}
	if topItems[0].LoanCount != 3 {
		t.Errorf("Top item loan count = %d, want 3", topItems[0].LoanCount)
	}

	// Second item should be gear2
	if topItems[1].GearId != gear2.Id {
		t.Errorf("Second item should be gear2, got %s", topItems[1].GearId)
	}
	if topItems[1].LoanCount != 1 {
		t.Errorf("Second item loan count = %d, want 1", topItems[1].LoanCount)
	}
}

// TestComputeTopContributors tests user ranking by contribution.
func TestComputeTopContributors(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))
	cache := NewEntityCache(db)

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

	// Create users
	topContributor := &models.User{
		Id:   uuid.New().String(),
		Name: "Top Contributor",
	}
	lowContributor := &models.User{
		Id:   uuid.New().String(),
		Name: "Low Contributor",
	}
	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Borrower",
	}

	if _, err := db.Insert(ctx, topContributor); err != nil {
		t.Fatalf("Failed to insert top contributor: %v", err)
	}
	if _, err := db.Insert(ctx, lowContributor); err != nil {
		t.Fatalf("Failed to insert low contributor: %v", err)
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	// Create gear for top contributor
	gear1 := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: topContributor.Id,
		Name:    "High Value Gear",
	}
	if _, err := db.Insert(ctx, gear1); err != nil {
		t.Fatalf("Failed to insert gear1: %v", err)
	}

	// Create gear for low contributor
	gear2 := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: lowContributor.Id,
		Name:    "Low Value Gear",
	}
	if _, err := db.Insert(ctx, gear2); err != nil {
		t.Fatalf("Failed to insert gear2: %v", err)
	}

	// Create multiple high-value transfers for top contributor's gear
	var transfers []*models.Transfer
	for i := 0; i < 5; i++ {
		transfer := &models.Transfer{
			Id:                  uuid.New().String(),
			GearId:              gear1.Id,
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

	// Create one low-value transfer for low contributor's gear
	transfer2 := &models.Transfer{
		Id:                  uuid.New().String(),
		GearId:              gear2.Id,
		RecipientId:         borrower.Id,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualReturnUnixSec: proto.Int64(1000),
		ImpactEstimate: &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{
				ValueUsd: &models.Estimate{
					Mean: 20.0,
				},
			},
		},
	}
	if _, err := db.Insert(ctx, transfer2); err != nil {
		t.Fatalf("Failed to insert transfer2: %v", err)
	}
	transfers = append(transfers, transfer2)

	// Compute top contributors
	topContributors := calc.computeTopContributors(
		ctx,
		transfers,
		[]*models.Request{},
		[]*models.Experience{},
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		10,
		cache,
	)

	// Verify ranking
	if len(topContributors) != 2 {
		t.Fatalf("Expected 2 top contributors, got %d", len(topContributors))
	}

	// First should be top contributor
	if topContributors[0].UserId != topContributor.Id {
		t.Errorf("Top contributor should be top contributor user, got %s", topContributors[0].UserId)
	}
	if topContributors[0].Name != "Top Contributor" {
		t.Errorf("Top contributor name = %q, want 'Top Contributor'", topContributors[0].Name)
	}
	// Second should be low contributor
	if topContributors[1].UserId != lowContributor.Id {
		t.Errorf("Second contributor should be low contributor user, got %s", topContributors[1].UserId)
	}
}

// TestComputeRecentActivity tests recent activity loading.
func TestComputeRecentActivity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))
	cache := NewEntityCache(db)

	// Create users
	owner := &models.User{
		Id:   uuid.New().String(),
		Name: "Owner",
	}
	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Borrower",
	}
	if _, err := db.Insert(ctx, owner); err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	// Create gear
	gear := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: owner.Id,
		Name:    "Test Gear",
	}
	if _, err := db.Insert(ctx, gear); err != nil {
		t.Fatalf("Failed to insert gear: %v", err)
	}

	// Create loans with different timestamps
	var transfers []*models.Transfer
	for i := 0; i < 5; i++ {
		transfer := &models.Transfer{
			Id:                  uuid.New().String(),
			GearId:              gear.Id,
			RecipientId:         borrower.Id,
			TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
			State:               models.TransferState_TRANSFER_STATE_COMPLETED,
			ActualReturnUnixSec: proto.Int64(int64(1000 + i)), // Increasing timestamps
			ImpactEstimate: &models.ImpactEstimate{
				MoneySaved: &models.MoneySavings{
					ValueUsd: &models.Estimate{
						Mean: 50.0,
					},
				},
			},
		}
		if _, err := db.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}
		transfers = append(transfers, transfer)
	}

	// Get recent activity (limit 3)
	recentActivity := calc.computeRecentActivity(
		ctx,
		transfers,
		[]*models.Request{},
		[]*models.Experience{},
		api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		cache,
	)

	// Should return 3 items
	if len(recentActivity) != 3 {
		t.Fatalf("Expected 3 recent activities, got %d", len(recentActivity))
	}

	// All should carry the gear name, the borrower, and a formattable value.
	for i, activity := range recentActivity {
		if activity.GetItemName() != "Test Gear" {
			t.Errorf("Activity[%d] item_name = %q, want 'Test Gear'", i, activity.GetItemName())
		}
		if activity.GetPersonDisplayName() != "Borrower" {
			t.Errorf("Activity[%d] person = %q, want 'Borrower'", i, activity.GetPersonDisplayName())
		}
		if activity.RawValue == nil {
			t.Errorf("Activity[%d] raw_value should be present", i)
		}
		if activity.Kind != api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR {
			t.Errorf("Activity[%d] kind = %v, want GEAR", i, activity.Kind)
		}
	}
}

// TestEntityCache tests that entity cache avoids duplicate queries.
func TestEntityCache(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	cache := NewEntityCache(db)

	// Create a user
	user := &models.User{
		Id:   uuid.New().String(),
		Name: "Test User",
	}
	if _, err := db.Insert(ctx, user); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// First load should hit database
	user1 := cache.GetUser(ctx, user.Id, nil)
	if user1 == nil {
		t.Fatal("First load returned nil")
	}
	if user1.Name != "Test User" {
		t.Errorf("User name = %q, want 'Test User'", user1.Name)
	}

	// Second load should use cache
	user2 := cache.GetUser(ctx, user.Id, nil)
	if user2 == nil {
		t.Fatal("Second load returned nil")
	}
	if user2.Name != "Test User" {
		t.Errorf("User name = %q, want 'Test User'", user2.Name)
	}

	// Should be the same instance
	if user1 != user2 {
		t.Error("Cache should return same instance")
	}
}

// TestTopItemsLimit tests that limit is respected.
func TestTopItemsLimit(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))
	cache := NewEntityCache(db)

	// Create owner and borrower
	owner := &models.User{
		Id:   uuid.New().String(),
		Name: "Owner",
	}
	borrower := &models.User{
		Id:   uuid.New().String(),
		Name: "Borrower",
	}
	if _, err := db.Insert(ctx, owner); err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}
	if _, err := db.Insert(ctx, borrower); err != nil {
		t.Fatalf("Failed to insert borrower: %v", err)
	}

	// Create 5 gear items with transfers
	var transfers []*models.Transfer
	for i := 0; i < 5; i++ {
		gear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: owner.Id,
			Name:    "Gear " + string(rune('A'+i)),
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

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

	// All items returned (no limit), sorted by value descending.
	topItems := calc.computeTopItems(ctx, transfers, nil, nil, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, cache)

	if len(topItems) != 5 {
		t.Errorf("Expected 5 top items (all gear), got %d", len(topItems))
	}
}
