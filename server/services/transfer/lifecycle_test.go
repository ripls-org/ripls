package transfer

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_CompleteTransfer_Loan_MarksGearAvailable(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Start loan
	startReq := connect.NewRequest(&api.StartLoanRequest{
		TransferId: transfer.Id,
	})
	_, err = service.StartLoan(ownerCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Complete transfer
	completeReq := connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transfer.Id,
	})
	_, err = service.CompleteTransfer(recipientCtx, completeReq)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %v", transfer.State)
	}

	// Verify ImpactEstimate was persisted at CompleteTransfer.
	// Note: MoneySaved requires gear with a ValueEstimate; test gear has none,
	// so we only verify TimeSaved is always set.
	if transfer.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set after CompleteTransfer")
	} else if transfer.ImpactEstimate.TimeSaved == nil || transfer.ImpactEstimate.TimeSaved.Minutes == nil {
		t.Error("Expected TimeSaved.Minutes to be set after CompleteTransfer")
	}

	// Verify provenance on TimeSaved
	if ts := transfer.ImpactEstimate.GetTimeSaved(); ts != nil {
		if ts.Provenance == nil {
			t.Error("Expected TimeSaved.Provenance to be set after CompleteTransfer")
		} else {
			if ts.Provenance.Source != models.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
				t.Errorf("Expected TimeSaved source CONFIG_DEFAULT, got %v", ts.Provenance.Source)
			}
			if ts.Provenance.Name == "" {
				t.Error("Expected TimeSaved provenance name to be non-empty")
			}
			if ts.Provenance.Version <= 0 {
				t.Errorf("Expected TimeSaved provenance version > 0, got %d", ts.Provenance.Version)
			}
		}
	}

	// Verify gear is available again
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear: %v", err)
	}

	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE, got %v", gear.State)
	}

	// Verify ownership unchanged
	if gear.OwnerId != ownerID {
		t.Errorf("Expected owner_id %s, got %s", ownerID, gear.OwnerId)
	}
}

func TestService_CompleteTransfer_OwnerCanMarkReturned(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Start loan (recipient marks picked up)
	startReq := connect.NewRequest(&api.StartLoanRequest{
		TransferId: transfer.Id,
	})
	_, err = service.StartLoan(recipientCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Owner marks as returned (not recipient)
	completeReq := connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transfer.Id,
	})
	_, err = service.CompleteTransfer(ownerCtx, completeReq)
	if err != nil {
		t.Fatalf("CompleteTransfer by owner failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %v", transfer.State)
	}
}

func TestService_CompleteTransfer_Giveaway_KeepsOwnership(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Owner selects recipient (giveaway goes directly from INTEREST_EXPRESSED to RECIPIENT_SELECTED)
	selectReq := connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transfer.Id,
		RecipientId: recipientID,
	})
	_, err = service.SelectRecipient(ownerCtx, selectReq)
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Complete transfer (giveaway goes from RECIPIENT_SELECTED to COMPLETED, keeping original ownership)
	completeReq := connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transfer.Id,
	})
	_, err = service.CompleteTransfer(ownerCtx, completeReq)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %v", transfer.State)
	}

	// Verify gear ownership REMAINS with original owner (does NOT transfer)
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear: %v", err)
	}

	if gear.OwnerId != ownerID {
		t.Errorf("Expected owner_id to remain with original owner %s, got %s", ownerID, gear.OwnerId)
	}

	if gear.State != models.GearState_GEAR_STATE_GIVEN_AWAY {
		t.Errorf("Expected gear state GIVEN_AWAY, got %v", gear.State)
	}

	// Verify CommunityGear is archived (not deleted)
	communityGears, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear relationships: %v", err)
	}

	if len(communityGears) != 1 {
		t.Fatalf("Expected 1 community gear relationship, found %d", len(communityGears))
	}

	communityGear := communityGears[0].(*models.CommunityGear)
	if !communityGear.Archived {
		t.Error("Expected CommunityGear to be archived")
	}
}

func TestService_CompleteTransfer_Loan_RecordsActualReturnTime(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Start loan
	startReq := connect.NewRequest(&api.StartLoanRequest{TransferId: transfer.Id})
	_, err = service.StartLoan(ownerCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Verify no actual return time before CompleteTransfer
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.ActualReturnUnixSec != nil {
		t.Errorf("Expected no actual return time before CompleteTransfer, got %d", *transfer.ActualReturnUnixSec)
	}

	// Complete transfer
	completeReq := connect.NewRequest(&api.CompleteTransferRequest{TransferId: transfer.Id})
	_, err = service.CompleteTransfer(recipientCtx, completeReq)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify actual return time was recorded
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.ActualReturnUnixSec == nil {
		t.Error("Expected actual_return_unix_sec to be set after CompleteTransfer for loan")
	}
}

func TestService_CompleteTransfer_Giveaway_RecordsActualPickupTime(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Select recipient
	selectReq := connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transfer.Id,
		RecipientId: recipientID,
	})
	_, err = service.SelectRecipient(ownerCtx, selectReq)
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Verify no actual pickup time before CompleteTransfer
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.ActualPickupUnixSec != nil {
		t.Errorf("Expected no actual pickup time before CompleteTransfer, got %d", *transfer.ActualPickupUnixSec)
	}

	// Complete transfer (giveaway goes directly from RECIPIENT_SELECTED to COMPLETED)
	completeReq := connect.NewRequest(&api.CompleteTransferRequest{TransferId: transfer.Id})
	_, err = service.CompleteTransfer(ownerCtx, completeReq)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify actual pickup time was recorded (giveaway handoff)
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.ActualPickupUnixSec == nil {
		t.Error("Expected actual_pickup_unix_sec to be set after CompleteTransfer for giveaway")
	}
}

// TestService_CompleteTransfer_SkipsAIDuringSimulation verifies that InferSocialAttributes
// is not called when completing a transfer in a simulated context.
func TestService_CompleteTransfer_SkipsAIDuringSimulation(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Wire mock AI provider that fails if called.
	mockAI := ai.NewMockProvider()
	inferCalled := false
	mockAI.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		inferCalled = true
		t.Error("InferSocialAttributes should not be called during simulation")
		return nil, nil
	}
	service.SetAIProvider(mockAI)

	// Express interest → auto-approved for loans.
	_, err := service.ExpressInterest(recipientCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer.
	transfers, err := testStorage.QueryByField(context.Background(), "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Start loan.
	_, err = service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{TransferId: transfer.Id}))
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Complete transfer with simulated context — AI should be skipped.
	simCtx := clock.WithSimulationTime(recipientCtx, time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	_, err = service.CompleteTransfer(simCtx, connect.NewRequest(&api.CompleteTransferRequest{TransferId: transfer.Id}))
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	if inferCalled {
		t.Fatal("InferSocialAttributes was called during simulation")
	}

	// Verify transfer still completed successfully with impact estimate.
	err = testStorage.GetByID(context.Background(), transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %v", transfer.State)
	}
	if transfer.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set even without AI inference")
	}
}
