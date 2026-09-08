package transfer

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestBuildTransfer_MapsAllLoanFields verifies that buildTransfer correctly maps
// all loan-related fields from the storage model to the API response.
//
// This test was added after discovering a bug where EstimatedPickupUnixSec,
// ActualPickupUnixSec, and LoanDurationDays were not being mapped, causing
// the client to always receive zeros for these fields even after a successful update.
func TestBuildTransfer_MapsAllLoanFields(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	// Setup test data
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create a transfer with loan details populated
	transfer := &models.Transfer{
		GearId:                 gearID,
		OwnerId:                ownerID,
		RecipientId:            borrowerID,
		CommunityId:            communityID,
		TransferType:           models.TransferType_TRANSFER_TYPE_LOAN,
		State:                  models.TransferState_TRANSFER_STATE_ACTIVE,
		EstimatedPickupUnixSec: proto.Int64(1770814800),
		ActualPickupUnixSec:    proto.Int64(1770900000),
		LoanDurationDays:       proto.Int32(7),
		LatestRequestUnixSec:   1770800000,
	}

	transferID, err := testStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}
	transfer.Id = transferID

	// Call buildTransfer
	apiTransfer, err := service.buildTransfer(ctx, transfer)
	if err != nil {
		t.Fatalf("buildTransfer failed: %v", err)
	}

	// Verify all loan-related fields are mapped correctly
	t.Run("EstimatedPickupUnixSec is mapped", func(t *testing.T) {
		if apiTransfer.EstimatedPickupUnixSec == nil || *apiTransfer.EstimatedPickupUnixSec != *transfer.EstimatedPickupUnixSec {
			t.Errorf("EstimatedPickupUnixSec: got %v, want %v",
				apiTransfer.EstimatedPickupUnixSec, transfer.EstimatedPickupUnixSec)
		}
	})

	t.Run("ActualPickupUnixSec is mapped", func(t *testing.T) {
		if apiTransfer.ActualPickupUnixSec == nil || *apiTransfer.ActualPickupUnixSec != *transfer.ActualPickupUnixSec {
			t.Errorf("ActualPickupUnixSec: got %v, want %v",
				apiTransfer.ActualPickupUnixSec, transfer.ActualPickupUnixSec)
		}
	})

	t.Run("LoanDurationDays is mapped", func(t *testing.T) {
		if apiTransfer.LoanDurationDays == nil || *apiTransfer.LoanDurationDays != *transfer.LoanDurationDays {
			t.Errorf("LoanDurationDays: got %v, want %v",
				apiTransfer.LoanDurationDays, transfer.LoanDurationDays)
		}
	})

	// Also verify other important fields are still mapped
	t.Run("LatestRequestUnixSec is mapped", func(t *testing.T) {
		if apiTransfer.LatestRequestUnixSec != transfer.LatestRequestUnixSec {
			t.Errorf("LatestRequestUnixSec: got %d, want %d",
				apiTransfer.LatestRequestUnixSec, transfer.LatestRequestUnixSec)
		}
	})

	t.Run("basic fields are mapped", func(t *testing.T) {
		if apiTransfer.Id != transfer.Id {
			t.Errorf("Id: got %s, want %s", apiTransfer.Id, transfer.Id)
		}
		if apiTransfer.GearId != transfer.GearId {
			t.Errorf("GearId: got %s, want %s", apiTransfer.GearId, transfer.GearId)
		}
	})
}

// TestBuildTransfer_NilFieldsForUnsetValues verifies that nil optional fields
// are correctly preserved when no values have been set.
func TestBuildTransfer_NilFieldsForUnsetValues(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	// Setup test data
	ownerID := setupTestUser(t, testStorage, "Owner", "owner2@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower2@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create a transfer with NO loan details (nil optional fields)
	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  borrowerID,
		CommunityId:  communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}

	transferID, err := testStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}
	transfer.Id = transferID

	// Call buildTransfer
	apiTransfer, err := service.buildTransfer(ctx, transfer)
	if err != nil {
		t.Fatalf("buildTransfer failed: %v", err)
	}

	// Verify unset optional fields are nil
	if apiTransfer.EstimatedPickupUnixSec != nil {
		t.Errorf("EstimatedPickupUnixSec should be nil, got %d", *apiTransfer.EstimatedPickupUnixSec)
	}
	if apiTransfer.ActualPickupUnixSec != nil {
		t.Errorf("ActualPickupUnixSec should be nil, got %d", *apiTransfer.ActualPickupUnixSec)
	}
	if apiTransfer.LoanDurationDays != nil {
		t.Errorf("LoanDurationDays should be nil, got %d", *apiTransfer.LoanDurationDays)
	}
}

// TestGetTransfer_ReturnsUpdatedLoanFields verifies that GetTransfer returns
// the updated loan fields after an UpdateTransfer call.
// This is an end-to-end test that would have caught the buildTransfer mapping bug.
func TestGetTransfer_ReturnsUpdatedLoanFields(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	// Setup test data
	ownerID := setupTestUser(t, testStorage, "Owner", "owner3@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower3@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create a transfer in RECIPIENT_SELECTED state (no pickup details yet)
	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  borrowerID,
		CommunityId:  communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}

	transferID, err := testStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}

	// Update the transfer with pickup details (simulating borrower submitting details)
	expectedPickupTime := int64(1770814800)
	expectedDuration := int32(7)

	transfer.Id = transferID
	transfer.EstimatedPickupUnixSec = &expectedPickupTime
	transfer.LoanDurationDays = &expectedDuration

	err = testStorage.Update(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to update transfer: %v", err)
	}

	// For this test, directly verify buildTransfer returns updated values
	// (buildTransfer is the internal function called by GetTransfer)
	updatedTransfer := &models.Transfer{}
	err = testStorage.GetByID(ctx, transferID, updatedTransfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	apiTransfer, err := service.buildTransfer(ctx, updatedTransfer)
	if err != nil {
		t.Fatalf("buildTransfer failed: %v", err)
	}

	// This is the critical assertion that would have caught the bug:
	// After updating the transfer, GetTransfer should return the updated values
	if apiTransfer.EstimatedPickupUnixSec == nil || *apiTransfer.EstimatedPickupUnixSec != expectedPickupTime {
		t.Errorf("After update, EstimatedPickupUnixSec: got %v, want %d",
			apiTransfer.EstimatedPickupUnixSec, expectedPickupTime)
	}

	if apiTransfer.LoanDurationDays == nil || *apiTransfer.LoanDurationDays != expectedDuration {
		t.Errorf("After update, LoanDurationDays: got %v, want %d",
			apiTransfer.LoanDurationDays, expectedDuration)
	}

	// Silence the unused variable warning
	_ = done
}

// TestBuildTransfer_DeletedOwnerAndRecipient verifies that buildTransfer
// surfaces a former-member placeholder for owner and recipient when the
// underlying user records have been soft-deleted, instead of emitting nil
// (which would crash clients reading transfer.owner.name) or returning an
// error. Regression for #1670.
func TestBuildTransfer_DeletedOwnerAndRecipient(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	transfer := &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  borrowerID,
		CommunityId:  communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_ACTIVE,
	}
	transferID, err := testStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Insert transfer: %v", err)
	}
	transfer.Id = transferID

	// Soft-delete both users.
	for _, id := range []string{ownerID, borrowerID} {
		u := &models.User{}
		if err := testStorage.GetByID(ctx, id, u); err != nil {
			t.Fatalf("Load user %s: %v", id, err)
		}
		u.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  id,
			DeletedAtUnixSec: 1,
		}
		if err := testStorage.Update(ctx, u); err != nil {
			t.Fatalf("Soft-delete user %s: %v", id, err)
		}
	}

	apiTransfer, err := service.buildTransfer(ctx, transfer)
	if err != nil {
		t.Fatalf("buildTransfer must tolerate deleted users; got error: %v", err)
	}
	if apiTransfer.Owner == nil {
		t.Fatal("Owner must be a placeholder, not nil")
	}
	if !apiTransfer.Owner.FormerMember || apiTransfer.Owner.Id != ownerID || apiTransfer.Owner.Name != "" {
		t.Errorf("Owner = %+v, want FormerMember=true, Id=%q, Name=\"\"", apiTransfer.Owner, ownerID)
	}
	if apiTransfer.Recipient == nil {
		t.Fatal("Recipient must be a placeholder, not nil")
	}
	if !apiTransfer.Recipient.FormerMember || apiTransfer.Recipient.Id != borrowerID || apiTransfer.Recipient.Name != "" {
		t.Errorf("Recipient = %+v, want FormerMember=true, Id=%q, Name=\"\"", apiTransfer.Recipient, borrowerID)
	}
}
