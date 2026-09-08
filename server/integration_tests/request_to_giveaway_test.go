package integration_tests

// Integration test for gear-backed request GIVEAWAYS on a multi-need request
// (#2703). Where request_to_loan_test covers the single loan that hands off and
// auto-fulfills, a supply drive is many parents giving items to the requester.
// Marking such a request fulfilled must DELIVER the confirmed helpers' gear —
// giveaways complete (the item is marked given away), loans go active — while
// an unconfirmed helper's offer stands down. Before the fix, Mark Fulfilled
// cancelled every open offer, so the donations evaporated.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

func TestRequestToGiveaway_FulfillDeliversConfirmedOffers(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Teacher Maya posts a supply request; three parents join her class group.
	mayaToken, _ := registerFirstUser(t, serverURL, "maya@example.com", "Maya")
	mayaCommunityClient := createAuthCommunityClient(mayaToken, serverURL)
	mayaRequestClient := createAuthRequestClient(mayaToken, serverURL)
	communityID := setupTestCommunity(t, ctx, mayaCommunityClient, "Room 7 Families", "Supply drive test")

	// Parents: Theo gives books, Bea lends a whiteboard, Cara offers a giveaway
	// that Maya will NOT confirm (it must stand down).
	theoToken, theoID := registerUserByInvite(t, serverURL, mayaToken, communityID, "theo-g@example.com", "Theo")
	theoRequestClient := createAuthRequestClient(theoToken, serverURL)
	theoTransferClient := createAuthTransferClient(theoToken, serverURL)
	theoGearClient := createAuthGearClient(theoToken, serverURL)
	theoLocationClient := createAuthLocationClient(theoToken, serverURL)

	beaToken, beaID := registerUserByInvite(t, serverURL, mayaToken, communityID, "bea-g@example.com", "Bea")
	beaRequestClient := createAuthRequestClient(beaToken, serverURL)
	beaTransferClient := createAuthTransferClient(beaToken, serverURL)
	beaGearClient := createAuthGearClient(beaToken, serverURL)
	beaLocationClient := createAuthLocationClient(beaToken, serverURL)

	caraToken, _ := registerUserByInvite(t, serverURL, mayaToken, communityID, "cara-g@example.com", "Cara")
	caraRequestClient := createAuthRequestClient(caraToken, serverURL)
	caraTransferClient := createAuthTransferClient(caraToken, serverURL)
	caraGearClient := createAuthGearClient(caraToken, serverURL)
	caraLocationClient := createAuthLocationClient(caraToken, serverURL)

	// A multi-need request: books (born), whiteboard, storage bins.
	seedName := "Picture books"
	submitResp, err := mayaRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Back-to-school supplies for Room 7",
		Description:   "Building out the classroom for the new year",
		SeedNeedNames: []string{seedName},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	shareRequestIntoCommunity(t, ctx, mayaCommunityClient, requestID, communityID)
	booksNeedID := seededNeedID(t, ctx, mayaRequestClient, requestID, "Picture books")
	whiteboardResp, err := mayaRequestClient.AddRequestNeed(ctx, connect.NewRequest(&api.AddRequestNeedRequest{
		RequestId: requestID, Name: "Whiteboard", Slots: 1,
	}))
	if err != nil {
		t.Fatalf("AddRequestNeed whiteboard: %v", err)
	}
	whiteboardNeedID := whiteboardResp.Msg.Need.Id
	binsResp, err := mayaRequestClient.AddRequestNeed(ctx, connect.NewRequest(&api.AddRequestNeedRequest{
		RequestId: requestID, Name: "Storage bins", Slots: 1,
	}))
	if err != nil {
		t.Fatalf("AddRequestNeed bins: %v", err)
	}
	binsNeedID := binsResp.Msg.Need.Id

	requesterID := submitRequesterID(t, ctx, mayaRequestClient, requestID)

	// Theo GIVES a set of picture books.
	theoLocation := setupTestLocation(t, ctx, theoLocationClient, "Bexley")
	theoGearID := setupTestGear(t, ctx, theoGearClient, "Picture book set", "A box of gently-used favorites", theoLocation)
	theoContrib := claimWithGear(t, ctx, theoRequestClient, requestID, booksNeedID, communityID, theoGearID)
	theoTransferID := offerGear(t, ctx, theoTransferClient, theoGearID, api.TransferType_TRANSFER_TYPE_GIVEAWAY, requesterID, communityID, requestID, theoContrib)

	// Bea LENDS a whiteboard for the year.
	beaLocation := setupTestLocation(t, ctx, beaLocationClient, "Bexley")
	beaGearID := setupTestGear(t, ctx, beaGearClient, "Whiteboard", "Large dry-erase board", beaLocation)
	beaContrib := claimWithGear(t, ctx, beaRequestClient, requestID, whiteboardNeedID, communityID, beaGearID)
	beaTransferID := offerGear(t, ctx, beaTransferClient, beaGearID, api.TransferType_TRANSFER_TYPE_LOAN, requesterID, communityID, requestID, beaContrib)

	// Cara offers bins as a giveaway — but Maya won't confirm her.
	caraLocation := setupTestLocation(t, ctx, caraLocationClient, "Bexley")
	caraGearID := setupTestGear(t, ctx, caraGearClient, "Storage bins", "Stackable cubbies", caraLocation)
	caraContrib := claimWithGear(t, ctx, caraRequestClient, requestID, binsNeedID, communityID, caraGearID)
	caraTransferID := offerGear(t, ctx, caraTransferClient, caraGearID, api.TransferType_TRANSFER_TYPE_GIVEAWAY, requesterID, communityID, requestID, caraContrib)

	// Maya marks the drive fulfilled, confirming Theo and Bea only.
	fulfillResp, err := mayaRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{theoID, beaID},
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled: %v", err)
	}

	// The confirmed giveaway completes and the books are handed over — the gear
	// is marked GIVEN_AWAY (Ripls records the giveaway on the gear + transfer;
	// it does not reassign the gear record).
	pollUntil(t, 15*time.Second, "confirmed giveaway completed", func() bool {
		resp, err := theoTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: theoTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_COMPLETED
	})
	gearResp, err := theoGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: theoGearID}))
	if err != nil {
		t.Fatalf("GetGear after giveaway: %v", err)
	}
	if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_GIVEN_AWAY {
		t.Errorf("expected the given books marked GIVEN_AWAY, got %s", gearResp.Msg.State)
	}

	// The confirmed loan goes active: Maya holds the whiteboard.
	pollUntil(t, 15*time.Second, "confirmed loan active", func() bool {
		resp, err := beaTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: beaTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_ACTIVE
	})

	// The unconfirmed offer stands down.
	pollUntil(t, 15*time.Second, "unconfirmed giveaway cancelled", func() bool {
		resp, err := caraTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: caraTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_CANCELLED
	})

	final, err := mayaRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest final: %v", err)
	}
	if final.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED {
		t.Fatalf("expected FULFILLED, got %s", final.Msg.Request.State)
	}

	// Undo the fulfillment: the delivered whiteboard LOAN reverts (it goes back
	// to Bea), but the completed GIVEAWAYS are permanent — the books stay given
	// (#2703 undo generalization, decision 10).
	if _, err := mayaRequestClient.UndoMarkRequestFulfilled(ctx, connect.NewRequest(&api.UndoMarkRequestFulfilledRequest{
		CommunityEventId: fulfillResp.Msg.CommunityEventId,
	})); err != nil {
		t.Fatalf("UndoMarkRequestFulfilled: %v", err)
	}
	pollUntil(t, 15*time.Second, "delivered loan cancelled by fulfillment undo", func() bool {
		resp, err := beaTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: beaTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_CANCELLED
	})
	giftAfterUndo, err := theoTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: theoTransferID}))
	if err != nil {
		t.Fatalf("GetTransfer after undo: %v", err)
	}
	if giftAfterUndo.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("a completed giveaway must survive fulfillment undo, got %s", giftAfterUndo.Msg.Transfer.State)
	}
	// The undo restores state synchronously, but the delivered-loan
	// cancellation it cascades feeds unwindCancelledOffer, which can re-write
	// state as a bus step; poll so the read lands after the pipeline settles.
	pollUntil(t, 15*time.Second, "request reopened after undo (not FULFILLED)", func() bool {
		resp, err := mayaRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		return err == nil && resp.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED
	})
}

// claimWithGear claims a need with a linked gear item and returns the
// contribution id the OfferTransfer will escalate.
func claimWithGear(t *testing.T, ctx context.Context, client apiconnect.RequestServiceClient, requestID, needID, communityID, gearID string) string {
	t.Helper()
	resp, err := client.ClaimRequestNeed(ctx, connect.NewRequest(&api.ClaimRequestNeedRequest{
		NeedId:      needID,
		RequestId:   requestID,
		CommunityId: communityID,
		GearId:      &gearID,
	}))
	if err != nil {
		t.Fatalf("ClaimRequestNeed(%s): %v", needID, err)
	}
	return resp.Msg.Contribution.Id
}

// offerGear escalates a gear-linked claim into a give/lend offer targeting the
// requester and returns the created transfer id, asserting it was born selected.
func offerGear(t *testing.T, ctx context.Context, client apiconnect.TransferServiceClient, gearID string, transferType api.TransferType, requesterID, communityID, requestID, contributionID string) string {
	t.Helper()
	resp, err := client.OfferTransfer(ctx, connect.NewRequest(&api.OfferTransferRequest{
		GearId:          gearID,
		TransferType:    transferType,
		RecipientUserId: requesterID,
		CommunityId:     communityID,
		OriginRequestId: requestID,
		ContributionId:  contributionID,
	}))
	if err != nil {
		t.Fatalf("OfferTransfer(%s): %v", transferType, err)
	}
	if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("expected RECIPIENT_SELECTED, got %s", resp.Msg.Transfer.State)
	}
	return resp.Msg.Transfer.Id
}
