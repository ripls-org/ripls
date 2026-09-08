package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/provisional"
)

// createPastTransferRequest builds a CreateTransferRequest with completed_at set 1 hour ago.
func pastGiveawayRequest(gearID, communityID, recipientUserID string) *api.CreateTransferRequest {
	completedAt := int64(1_000_000) // fixed past timestamp for deterministic tests
	return &api.CreateTransferRequest{
		GearId:             gearID,
		CommunityId:        communityID,
		TransferType:       api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		RecipientUserId:    &recipientUserID,
		CompletedAtUnixSec: &completedAt,
	}
}

func pastLoanRequest(gearID, communityID, recipientUserID string, returnedAt *int64) *api.CreateTransferRequest {
	completedAt := int64(1_000_000)
	return &api.CreateTransferRequest{
		GearId:             gearID,
		CommunityId:        communityID,
		TransferType:       api.TransferType_TRANSFER_TYPE_LOAN,
		RecipientUserId:    &recipientUserID,
		CompletedAtUnixSec: &completedAt,
		ReturnedAtUnixSec:  returnedAt,
	}
}

func TestCreateTransfer_PastGiveaway_RealRecipient(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(pastGiveawayRequest(gearID, communityID, recipientID))

	resp, err := service.CreateTransfer(ctx, req)
	if err != nil {
		t.Fatalf("CreateTransfer failed: %v", err)
	}

	if resp.Msg.Transfer == nil {
		t.Fatal("Expected transfer in response")
	}
	if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %s", resp.Msg.Transfer.State)
	}
	if resp.Msg.Impact == nil {
		t.Error("Expected impact estimate in response")
	}
}

func TestCreateTransfer_PastLoan_NotReturned(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner2@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := createAuthenticatedContext(ownerID, "owner2@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(pastLoanRequest(gearID, communityID, borrowerID, nil))

	resp, err := service.CreateTransfer(ctx, req)
	if err != nil {
		t.Fatalf("CreateTransfer failed: %v", err)
	}

	// Loan without return date → ACTIVE state (item is still out)
	if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_ACTIVE {
		t.Errorf("Expected ACTIVE state for unreturned loan, got %s", resp.Msg.Transfer.State)
	}
}

func TestCreateTransfer_PastLoan_AlreadyReturned(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner3@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower2@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := createAuthenticatedContext(ownerID, "owner3@example.com", models.Role_ROLE_USER)
	returnedAt := int64(2_000_000) // after completedAt=1_000_000
	req := connect.NewRequest(pastLoanRequest(gearID, communityID, borrowerID, &returnedAt))

	resp, err := service.CreateTransfer(ctx, req)
	if err != nil {
		t.Fatalf("CreateTransfer failed: %v", err)
	}

	// Loan with return date → COMPLETED state
	if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state for returned loan, got %s", resp.Msg.Transfer.State)
	}
	if resp.Msg.Impact == nil {
		t.Error("Expected impact estimate for completed loan")
	}
}

func TestCreateTransfer_PastGiveaway_ProvisionalRecipient(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner4@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create a community with just the owner (provisional users are non-registered)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, ownerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Insert provisional user directly
	provID, err := testStorage.Insert(context.Background(), &models.ProvisionalUser{
		Name:            "Provisional Person",
		CommunityId:     communityID,
		CreatedByUserId: ownerID,
	})
	if err != nil {
		t.Fatalf("Insert provisional user failed: %v", err)
	}

	ctx := createAuthenticatedContext(ownerID, "owner4@example.com", models.Role_ROLE_USER)
	completedAt := int64(1_000_000)
	req := connect.NewRequest(&api.CreateTransferRequest{
		GearId:             gearID,
		CommunityId:        communityID,
		TransferType:       api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		ProvisionalUserId:  &provID,
		CompletedAtUnixSec: &completedAt,
	})

	resp, err := service.CreateTransfer(ctx, req)
	if err != nil {
		t.Fatalf("CreateTransfer with provisional recipient failed: %v", err)
	}
	if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %s", resp.Msg.Transfer.State)
	}
}

func TestCreateTransfer_NonOwnerRejected(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner5@example.com")
	otherID := setupTestUser(t, testStorage, "Other", "other@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, otherID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Other user (not the gear owner) attempts to create a transfer.
	ctx := createAuthenticatedContext(otherID, "other@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(pastGiveawayRequest(gearID, communityID, ownerID))

	_, err := service.CreateTransfer(ctx, req)
	if err == nil {
		t.Fatal("Expected permission denied for non-owner")
	}
}

func TestCreateTransfer_FutureTimestampRejected(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner6@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient3@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := createAuthenticatedContext(ownerID, "owner6@example.com", models.Role_ROLE_USER)
	futureTime := int64(9_999_999_999) // far future
	req := connect.NewRequest(&api.CreateTransferRequest{
		GearId:             gearID,
		CommunityId:        communityID,
		TransferType:       api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		RecipientUserId:    &recipientID,
		CompletedAtUnixSec: &futureTime,
	})

	_, err := service.CreateTransfer(ctx, req)
	if err == nil {
		t.Fatal("Expected invalid argument for future timestamp")
	}
}

func TestMergeIntoUser_MigratesTransfers(t *testing.T) {
	_, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner7@example.com")
	realUserID := setupTestUser(t, testStorage, "Real", "real@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, ownerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Insert provisional user
	provID, err := testStorage.Insert(context.Background(), &models.ProvisionalUser{
		Name:            "Pre-registered Person",
		CommunityId:     communityID,
		CreatedByUserId: ownerID,
	})
	if err != nil {
		t.Fatalf("Insert provisional user failed: %v", err)
	}

	// Insert a transfer with provisional_recipient_id.
	completedAt := int64(1_000_000)
	transferID, err := testStorage.Insert(context.Background(), &models.Transfer{
		GearId:                 gearID,
		OwnerId:                ownerID,
		ProvisionalRecipientId: &provID,
		TransferType:           models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:                  models.TransferState_TRANSFER_STATE_COMPLETED,
		CommunityId:            communityID,
		ActualPickupUnixSec:    &completedAt,
	})
	if err != nil {
		t.Fatalf("Insert transfer failed: %v", err)
	}

	// Merge provisional user into real user.
	if err := provisional.MergeIntoUser(context.Background(), testStorage, provID, realUserID); err != nil {
		t.Fatalf("MergeIntoUser failed: %v", err)
	}

	// Verify the transfer now has recipient_id = realUserID and provisional_recipient_id = nil.
	transfer := &models.Transfer{}
	if err := testStorage.GetByID(context.Background(), transferID, transfer); err != nil {
		t.Fatalf("GetByID transfer failed: %v", err)
	}
	if transfer.RecipientId != realUserID {
		t.Errorf("Expected recipient_id=%s, got %s", realUserID, transfer.RecipientId)
	}
	if transfer.ProvisionalRecipientId != nil {
		t.Errorf("Expected provisional_recipient_id to be nil after merge, got %s", *transfer.ProvisionalRecipientId)
	}
}
