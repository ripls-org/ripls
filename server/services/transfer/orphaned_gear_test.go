package transfer

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestListing_WithSoftDeletedGear is a regression test for #1641: prior to
// the fix, ListMyTransfers / ListReceivedTransfers / GetGearTransfers /
// ListTransfers / GetTransfer all returned connect.CodeInternal ("gear ...
// not found") whenever any of the user's transfers referenced a
// soft-deleted gear. This locked affected users out of their entire
// transfer list. The fix passes IncludeDeleted: true to the gear lookup in
// buildTransfers so transfer history remains viewable after the gear is
// deleted.
func TestListing_WithSoftDeletedGear(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	const (
		ownerID     = "orphan-owner"
		recipientID = "orphan-recipient"
	)

	ctx := context.Background()
	if _, err := testStorage.Insert(ctx, &models.User{Id: ownerID, Name: "Owner", Email: "owner@example.com"}); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.User{Id: recipientID, Name: "Recipient", Email: "recipient@example.com"}); err != nil {
		t.Fatalf("insert recipient: %v", err)
	}

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	// Recipient expresses interest → transfer is created.
	expressResp, err := service.ExpressInterest(recipientCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest: %v", err)
	}
	services.WaitForNotification(t, done)
	transferID := expressResp.Msg.Transfer.Id

	// Move the transfer to a terminal state (CANCELLED) so it survives the
	// realistic post-DeleteGear shape: DeleteGear cancels active transfers
	// and then soft-deletes the gear without soft-deleting the transfer rows.
	storedTransfer := &models.Transfer{}
	if err := testStorage.GetByID(ctx, transferID, storedTransfer); err != nil {
		t.Fatalf("get transfer: %v", err)
	}
	storedTransfer.State = models.TransferState_TRANSFER_STATE_CANCELLED
	if err := testStorage.Update(ctx, storedTransfer); err != nil {
		t.Fatalf("cancel transfer: %v", err)
	}

	// Soft-delete the gear, mirroring what DeleteGear does to the gear row.
	storedGear := &models.Gear{}
	if err := testStorage.GetByID(ctx, gearID, storedGear); err != nil {
		t.Fatalf("get gear: %v", err)
	}
	storedGear.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  ownerID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := testStorage.Update(ctx, storedGear); err != nil {
		t.Fatalf("soft-delete gear: %v", err)
	}

	assertTransferPresent := func(t *testing.T, transfers []*api.Transfer) {
		t.Helper()
		if len(transfers) != 1 {
			t.Fatalf("expected 1 transfer, got %d", len(transfers))
		}
		got := transfers[0]
		if got.Id != transferID {
			t.Errorf("transfer id: want %s, got %s", transferID, got.Id)
		}
		if got.GearName != "Test Drill" {
			t.Errorf("gear name not populated: got %q", got.GearName)
		}
		if got.GearId != gearID {
			t.Errorf("gear id: want %s, got %s", gearID, got.GearId)
		}
	}

	t.Run("ListMyTransfers (owner)", func(t *testing.T) {
		resp, err := service.ListMyTransfers(ownerCtx, connect.NewRequest(&api.ListMyTransfersRequest{}))
		if err != nil {
			t.Fatalf("ListMyTransfers: %v", err)
		}
		assertTransferPresent(t, resp.Msg.Transfers)
	})

	t.Run("ListReceivedTransfers (recipient)", func(t *testing.T) {
		resp, err := service.ListReceivedTransfers(recipientCtx, connect.NewRequest(&api.ListReceivedTransfersRequest{}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers: %v", err)
		}
		assertTransferPresent(t, resp.Msg.Transfers)
	})

	t.Run("GetGearTransfers", func(t *testing.T) {
		resp, err := service.GetGearTransfers(ownerCtx, connect.NewRequest(&api.GetGearTransfersRequest{GearId: gearID}))
		if err != nil {
			t.Fatalf("GetGearTransfers: %v", err)
		}
		assertTransferPresent(t, resp.Msg.Transfers)
	})

	t.Run("ListTransfers (community)", func(t *testing.T) {
		resp, err := service.ListTransfers(ownerCtx, connect.NewRequest(&api.ListTransfersRequest{CommunityId: communityID}))
		if err != nil {
			t.Fatalf("ListTransfers: %v", err)
		}
		assertTransferPresent(t, resp.Msg.Transfers)
	})

	t.Run("GetTransfer (single)", func(t *testing.T) {
		resp, err := service.GetTransfer(ownerCtx, connect.NewRequest(&api.GetTransferRequest{TransferId: transferID}))
		if err != nil {
			t.Fatalf("GetTransfer: %v", err)
		}
		got := resp.Msg.Transfer
		if got.Id != transferID || got.GearName != "Test Drill" || got.GearId != gearID {
			t.Errorf("unexpected transfer: %+v", got)
		}
	})
}
