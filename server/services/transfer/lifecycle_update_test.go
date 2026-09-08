package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_CancelTransfer(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

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

	// Cancel transfer
	cancelReq := connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	})
	_, err = service.CancelTransfer(recipientCtx, cancelReq)
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_CANCELLED {
		t.Errorf("Expected CANCELLED state, got %v", transfer.State)
	}

	// Verify gear remains available (cancelled before starting)
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear: %v", err)
	}

	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE, got %v", gear.State)
	}
}

func TestService_UpdateTransfer(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear - use GIVEAWAY since loans are auto-approved
	// and skip INTEREST_EXPRESSED state where updates are allowed
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

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

	// Verify initial transfer type
	if transfer.TransferType != models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("Expected initial GIVEAWAY type, got %v", transfer.TransferType)
	}

	// Verify transfer is in INTEREST_EXPRESSED state (required for updates)
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected INTEREST_EXPRESSED state, got %v", transfer.State)
	}

	// Owner updates transfer type to loan
	updateReq := connect.NewRequest(&api.UpdateTransferRequest{
		TransferId:   transfer.Id,
		TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
	})
	_, err = service.UpdateTransfer(ownerCtx, updateReq)
	if err != nil {
		t.Fatalf("UpdateTransfer failed: %v", err)
	}

	// Verify transfer type updated
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get updated transfer: %v", err)
	}

	if transfer.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected LOAN type after update, got %v", transfer.TransferType)
	}
}

func TestService_UpdateTransfer_EstimatedPickupTime(t *testing.T) {
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

	// Update with estimates - should succeed in RECIPIENT_SELECTED state
	estimatedPickup := int64(1704067200) // Some future timestamp
	loanDuration := int32(7)
	updateReq := connect.NewRequest(&api.UpdateTransferRequest{
		TransferId:             transfer.Id,
		EstimatedPickupUnixSec: &estimatedPickup,
		LoanDurationDays:       &loanDuration,
	})
	_, err = service.UpdateTransfer(ownerCtx, updateReq)
	if err != nil {
		t.Fatalf("UpdateTransfer failed: %v", err)
	}

	// Verify estimates were saved
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.EstimatedPickupUnixSec == nil || *transfer.EstimatedPickupUnixSec != estimatedPickup {
		t.Errorf("Expected estimated_pickup_unix_sec %d, got %v", estimatedPickup, transfer.EstimatedPickupUnixSec)
	}

	if transfer.LoanDurationDays == nil || *transfer.LoanDurationDays != loanDuration {
		t.Errorf("Expected loan_duration_days %d, got %v", loanDuration, transfer.LoanDurationDays)
	}
}

func TestService_UpdateTransfer_RecipientCanSetEstimates(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

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

	// Recipient sets estimates
	estimatedPickup := int64(1704153600) // Some future timestamp
	updateReq := connect.NewRequest(&api.UpdateTransferRequest{
		TransferId:             transfer.Id,
		EstimatedPickupUnixSec: &estimatedPickup,
	})
	_, err = service.UpdateTransfer(recipientCtx, updateReq)
	if err != nil {
		t.Fatalf("UpdateTransfer by recipient failed: %v", err)
	}

	// Verify estimate was saved
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.EstimatedPickupUnixSec == nil || *transfer.EstimatedPickupUnixSec != estimatedPickup {
		t.Errorf("Expected estimated_pickup_unix_sec %d, got %v", estimatedPickup, transfer.EstimatedPickupUnixSec)
	}
}

// TestService_UpdateTransfer_GiveawayPickupProposedNotification verifies that setting
// estimated_pickup_unix_sec on a giveaway records a TRANSFER_PICKUP_PROPOSED event
// and notifies the other party.
func TestService_UpdateTransfer_GiveawayPickupProposedNotification(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	_, err := service.ExpressInterest(recipientCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
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

	// Select recipient (giveaways require explicit selection)
	_, err = service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transfer.Id,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Owner proposes pickup time
	estimatedPickup := int64(1704067200)
	_, err = service.UpdateTransfer(ownerCtx, connect.NewRequest(&api.UpdateTransferRequest{
		TransferId:             transfer.Id,
		EstimatedPickupUnixSec: &estimatedPickup,
	}))
	if err != nil {
		t.Fatalf("UpdateTransfer failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Verify a TRANSFER_PICKUP_PROPOSED event was recorded
	events, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.(*models.CommunityEvent).EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected TRANSFER_PICKUP_PROPOSED event to be recorded")
	}
}
