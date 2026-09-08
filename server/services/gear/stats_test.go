package gear

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/services"
)

func TestService_GetGearStats(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("get stats for gear with no transfers", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create a gear item with value estimate
		gear := &models.Gear{
			Id:          "gear123",
			OwnerId:     "user123",
			Name:        "Test Drill",
			Description: "A test drill",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 149.0, // $149
			},
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearStatsRequest{
			GearId: "gear123",
		})

		resp, err := service.GetGearStats(ctx, req)
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}

		if resp.Msg.TimesLoaned != 0 {
			t.Errorf("Expected 0 times loaned, got %d", resp.Msg.TimesLoaned)
		}

		if resp.Msg.PeopleHelped != 0 {
			t.Errorf("Expected 0 people helped, got %d", resp.Msg.PeopleHelped)
		}

		if resp.Msg.ValueSharedUsd != 0 {
			t.Errorf("Expected 0 value shared, got %f", resp.Msg.ValueSharedUsd)
		}

		if resp.Msg.InterestCount != 0 {
			t.Errorf("Expected 0 interest count, got %d", resp.Msg.InterestCount)
		}
	})

	t.Run("get stats for gear with completed loans", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create a gear item with value estimate
		gear := &models.Gear{
			Id:          "gear456",
			OwnerId:     "user123",
			Name:        "Test Saw",
			Description: "A test saw",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 100.0, // $100
			},
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Create completed transfers
		transfer1 := &models.Transfer{
			Id:           "transfer1",
			GearId:       "gear456",
			OwnerId:      "user123",
			RecipientId:  "borrower1",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
			CommunityId:  "community123",
		}
		if _, err := testStorage.Insert(ctx, transfer1); err != nil {
			t.Fatalf("Failed to create transfer1: %v", err)
		}

		transfer2 := &models.Transfer{
			Id:           "transfer2",
			GearId:       "gear456",
			OwnerId:      "user123",
			RecipientId:  "borrower2",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
			CommunityId:  "community123",
		}
		if _, err := testStorage.Insert(ctx, transfer2); err != nil {
			t.Fatalf("Failed to create transfer2: %v", err)
		}

		// Create active transfer
		transfer3 := &models.Transfer{
			Id:           "transfer3",
			GearId:       "gear456",
			OwnerId:      "user123",
			RecipientId:  "borrower3",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
			CommunityId:  "community123",
		}
		if _, err := testStorage.Insert(ctx, transfer3); err != nil {
			t.Fatalf("Failed to create transfer3: %v", err)
		}

		req := connect.NewRequest(&api.GetGearStatsRequest{
			GearId: "gear456",
		})

		resp, err := service.GetGearStats(ctx, req)
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}

		if resp.Msg.TimesLoaned != 3 {
			t.Errorf("Expected 3 times loaned, got %d", resp.Msg.TimesLoaned)
		}

		if resp.Msg.PeopleHelped != 3 {
			t.Errorf("Expected 3 people helped, got %d", resp.Msg.PeopleHelped)
		}

		// Value shared = $100 * 3 = $300
		expectedValueShared := float32(300.0)
		if resp.Msg.ValueSharedUsd != expectedValueShared {
			t.Errorf("Expected %f value shared, got %f", expectedValueShared, resp.Msg.ValueSharedUsd)
		}
	})

	t.Run("gear not found", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearStatsRequest{
			GearId: "nonexistent",
		})

		_, err := service.GetGearStats(ctx, req)
		if err == nil {
			t.Error("Expected error for nonexistent gear")
		}

		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			if connectErr.Code() != connect.CodeNotFound {
				t.Errorf("Expected NotFound error, got %v", connectErr.Code())
			}
		}
	})

	t.Run("potential_impact populated when no transactions", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Load estimator config for impact calculation
		cfg, err := estimator.LoadConfigFromEmbed()
		if err != nil {
			t.Fatalf("Failed to load estimator config: %v", err)
		}
		service.SetEstimatorConfig(cfg)

		// Create a gear item with value estimate
		gear := &models.Gear{
			Id:          "gear_potential",
			OwnerId:     "user123",
			Name:        "Test Gear with Potential",
			Description: "A gear with no transactions yet",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 200.0,
			},
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearStatsRequest{
			GearId: "gear_potential",
		})

		resp, err := service.GetGearStats(ctx, req)
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}

		// Verify times_loaned is 0
		if resp.Msg.TimesLoaned != 0 {
			t.Errorf("Expected 0 times loaned, got %d", resp.Msg.TimesLoaned)
		}

		// Verify impact is nil (no actual transactions)
		if resp.Msg.Impact != nil {
			t.Error("Expected impact to be nil when no transactions")
		}

		// Verify potential_impact is populated
		if resp.Msg.PotentialImpact == nil {
			t.Fatal("Expected potential_impact to be populated when times_loaned == 0")
		}

		// Verify potential_impact has money_saved (most important dimension)
		if resp.Msg.PotentialImpact.MoneySaved == nil {
			t.Error("Expected potential_impact to have money_saved")
		} else if resp.Msg.PotentialImpact.MoneySaved.ValueUsd == nil {
			t.Error("Expected potential_impact money_saved to have value_usd")
		} else if resp.Msg.PotentialImpact.MoneySaved.ValueUsd.Mean <= 0 {
			t.Errorf("Expected positive money savings, got %f", resp.Msg.PotentialImpact.MoneySaved.ValueUsd.Mean)
		}

		// Verify time_saved is populated (should always be present from config defaults)
		if resp.Msg.PotentialImpact.TimeSaved == nil {
			t.Error("Expected potential_impact to have time_saved")
		}
	})

	t.Run("potential_impact always populated when estimator configured", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Load estimator config for impact calculation
		cfg, err := estimator.LoadConfigFromEmbed()
		if err != nil {
			t.Fatalf("Failed to load estimator config: %v", err)
		}
		service.SetEstimatorConfig(cfg)

		// Create a gear item
		gear := &models.Gear{
			Id:          "gear_with_loans",
			OwnerId:     "user123",
			Name:        "Test Gear with Loans",
			Description: "A gear with transactions",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 150.0,
			},
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Create a completed loan
		transfer := &models.Transfer{
			Id:           "transfer_for_potential",
			GearId:       "gear_with_loans",
			OwnerId:      "user123",
			RecipientId:  "borrower1",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
			CommunityId:  "community123",
		}
		if _, err := testStorage.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		req := connect.NewRequest(&api.GetGearStatsRequest{
			GearId: "gear_with_loans",
		})

		resp, err := service.GetGearStats(ctx, req)
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}

		// Verify times_loaned is > 0
		if resp.Msg.TimesLoaned == 0 {
			t.Error("Expected times_loaned > 0")
		}

		// Verify impact is populated (cumulative across all loans)
		if resp.Msg.Impact == nil {
			t.Error("Expected impact to be populated when transactions exist")
		}

		// Verify potential_impact is always populated (per-loan baseline for SharingImpactCard)
		if resp.Msg.PotentialImpact == nil {
			t.Error("Expected potential_impact to always be populated when estimator is configured")
		}

		// Verify potential_impact represents a single-loan estimate (money > 0)
		if resp.Msg.PotentialImpact.MoneySaved == nil || resp.Msg.PotentialImpact.MoneySaved.ValueUsd == nil {
			t.Error("Expected potential_impact to have money_saved")
		} else if resp.Msg.PotentialImpact.MoneySaved.ValueUsd.Mean <= 0 {
			t.Errorf("Expected positive per-loan money savings, got %f", resp.Msg.PotentialImpact.MoneySaved.ValueUsd.Mean)
		}
	})
}
