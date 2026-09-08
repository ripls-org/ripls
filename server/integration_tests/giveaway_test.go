package integration_tests

// Integration tests for giveaway workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/giveaway.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestGiveaway_Example1_GivingAwayItem tests the happy path giveaway workflow.
// Covers: docs/workflows/giveaway.md - Workflow Example 1.
func TestGiveaway_Example1_GivingAwayItem(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Four users exist: an owner, user A, user B, and user C, all members of the community
	// - The owner has gear that is not yet shared

	ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Giveaway Test Community", "Testing giveaway workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Portland")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Old Tent", "4-person tent, lightly used", locationID)

	userAToken, userAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-a@example.com", "User A")
	userATransferClient := createAuthTransferClient(userAToken, serverURL)

	userBToken, userBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-b@example.com", "User B")
	userBTransferClient := createAuthTransferClient(userBToken, serverURL)

	userCToken, userCID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-c@example.com", "User C")
	userCTransferClient := createAuthTransferClient(userCToken, serverURL)

	var conversationID string
	var transferID string

	// ========== STEPS ==========

	// Step 1: Owner shares gear with community for giveaway
	t.Run("Step1_OwnerSharesGear", func(t *testing.T) {
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)
	})

	// Step 2: System creates perpetual conversation for the gear
	t.Run("Step2_ConversationCreated", func(t *testing.T) {
		listResp, err := ownerCommunityClient.ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}
		if len(listResp.Msg.GearItems) != 1 {
			t.Fatalf("Expected 1 gear item, got %d", len(listResp.Msg.GearItems))
		}
		conversationID = listResp.Msg.GearItems[0].ConversationId
		if conversationID == "" {
			t.Fatal("Expected conversation to be created when gear is shared")
		}
	})

	// Step 3: User A expresses interest
	t.Run("Step3_UserAExpressesInterest", func(t *testing.T) {
		resp, err := userATransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		transferID = resp.Msg.Transfer.Id
		if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected INTEREST_EXPRESSED state, got %v", resp.Msg.Transfer.State)
		}
	})

	// Step 5: User B expresses interest (creates their own transfer)
	t.Run("Step5_UserBExpressesInterest", func(t *testing.T) {
		resp, err := userBTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		// Save UserB's transferID - this is the one the owner will select
		transferID = resp.Msg.Transfer.Id
		if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected INTEREST_EXPRESSED state, got %v", resp.Msg.Transfer.State)
		}
	})

	// Step 7: User C expresses interest
	t.Run("Step7_UserCExpressesInterest", func(t *testing.T) {
		resp, err := userCTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected INTEREST_EXPRESSED state, got %v", resp.Msg.Transfer.State)
		}
	})

	// Step 9: Owner selects User B as recipient
	t.Run("Step9_OwnerSelectsUserB", func(t *testing.T) {
		_, err := ownerTransferClient.SelectRecipient(ctx, connect.NewRequest(&api.SelectRecipientRequest{
			TransferId:  transferID,
			RecipientId: userBID,
		}))
		if err != nil {
			t.Fatalf("SelectRecipient failed: %v", err)
		}

		// Verify state changed
		listResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		}))
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}
		var foundSelected bool
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Id == transferID && transfer.State == api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
				foundSelected = true
				break
			}
		}
		if !foundSelected {
			t.Error("Expected transfer to be in RECIPIENT_SELECTED state")
		}
	})

	// Step 10: Owner confirms pickup time (optional — sets estimated_pickup_unix_sec, no state change)
	// Per docs/workflows/giveaway.md operation 4: Either owner or selected recipient can set the pickup time.
	var confirmedPickupTime int64
	t.Run("Step10_OwnerConfirmsPickupTime", func(t *testing.T) {
		confirmedPickupTime = time.Now().Add(48 * time.Hour).Unix()
		_, err := ownerTransferClient.UpdateTransfer(ctx, connect.NewRequest(&api.UpdateTransferRequest{
			TransferId:             transferID,
			EstimatedPickupUnixSec: &confirmedPickupTime,
		}))
		if err != nil {
			t.Fatalf("UpdateTransfer (confirm pickup time) failed: %v", err)
		}

		// Verify pickup time was set and state is still RECIPIENT_SELECTED (no state change)
		getResp, err := ownerTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferID,
		}))
		if err != nil {
			t.Fatalf("GetTransfer failed: %v", err)
		}
		if getResp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			t.Errorf("Expected transfer to remain in RECIPIENT_SELECTED after setting pickup time, got %v", getResp.Msg.Transfer.State)
		}
		if getResp.Msg.Transfer.EstimatedPickupUnixSec == nil {
			t.Error("Expected estimated_pickup_unix_sec to be set after UpdateTransfer")
		} else if *getResp.Msg.Transfer.EstimatedPickupUnixSec != confirmedPickupTime {
			t.Errorf("Expected estimated_pickup_unix_sec %d, got %d", confirmedPickupTime, *getResp.Msg.Transfer.EstimatedPickupUnixSec)
		}
	})

	// Step 11: Owner completes giveaway
	var completeImpact *api.ImpactEstimate
	t.Run("Step11_OwnerCompletesGiveaway", func(t *testing.T) {
		resp, err := ownerTransferClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
			TransferId: transferID,
		}))
		if err != nil {
			t.Fatalf("CompleteTransfer failed: %v", err)
		}
		completeImpact = resp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_GearArchivedNotVisible", func(t *testing.T) {
		// After giveaway completes, gear should be archived (not visible in default list)
		listResp, err := ownerCommunityClient.ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}
		if len(listResp.Msg.GearItems) != 0 {
			t.Errorf("Expected gear to be archived (not in active list) after giveaway, got %d items", len(listResp.Msg.GearItems))
		}
	})

	t.Run("Postcondition_GearInPastGiveaways", func(t *testing.T) {
		// Gear should appear in past giveaways list
		pastResp, err := ownerCommunityClient.ListCompletedGiveaways(ctx, connect.NewRequest(&api.ListCompletedGiveawaysRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListCompletedGiveaways failed: %v", err)
		}
		if len(pastResp.Msg.GearItems) != 1 {
			t.Errorf("Expected 1 past giveaway, got %d", len(pastResp.Msg.GearItems))
		}
		if len(pastResp.Msg.GearItems) > 0 && pastResp.Msg.GearItems[0].Id != gearID {
			t.Errorf("Expected gear %s in past giveaways, got %s", gearID, pastResp.Msg.GearItems[0].Id)
		}
	})

	t.Run("Postcondition_GearStillOwnedByOriginalOwner", func(t *testing.T) {
		// Gear ownership stays with original owner (NOT transferred to User B)
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.Owner == nil {
			t.Fatal("Expected gear to have an owner")
		}
		// Owner should still be the original owner, not User B
		if gearResp.Msg.Owner.Id == userBID {
			t.Errorf("Gear ownership should NOT transfer to recipient (User B). Expected original owner, got User B (%s)", userBID)
		}
	})

	t.Run("Postcondition_GearStateGivenAway", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_GIVEN_AWAY {
			t.Errorf("Expected gear to be GIVEN_AWAY after giveaway, got %v", gearResp.Msg.State)
		}
	})

	t.Run("Postcondition_TransferCompleted", func(t *testing.T) {
		listResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		}))
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}
		var found bool
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Id == transferID {
				found = true
				if transfer.State != api.TransferState_TRANSFER_STATE_COMPLETED {
					t.Errorf("Expected COMPLETED state, got %v", transfer.State)
				}
				break
			}
		}
		if !found {
			t.Error("Transfer not found")
		}
	})

	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
		// Conversation remains accessible for historical reference
	})

	t.Run("Postcondition_ActiveLoanShowsRecipient", func(t *testing.T) {
		// GetGear should show who received the gear in active_loan field
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id:          gearID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if gearResp.Msg.ActiveLoan == nil {
			t.Fatal("Expected active_loan to be populated for completed giveaway")
		}
		if gearResp.Msg.ActiveLoan.Borrower.Id != userBID {
			t.Errorf("Expected active_loan borrower to be UserB (%s), got %s",
				userBID, gearResp.Msg.ActiveLoan.Borrower.Id)
		}
		if gearResp.Msg.ActiveLoan.Status != "Given away" {
			t.Errorf("Expected active_loan status 'Given away', got %s",
				gearResp.Msg.ActiveLoan.Status)
		}
	})

	t.Run("Postcondition_OtherTransfersCancelled", func(t *testing.T) {
		// UserA and UserC's transfers should be automatically cancelled when UserB's was completed

		// Query all transfers for this gear from the owner's perspective
		listResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		}))
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}

		var userATransfer, userCTransfer *api.Transfer
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Recipient != nil {
				switch transfer.Recipient.Id {
				case userAID:
					userATransfer = transfer
				case userCID:
					userCTransfer = transfer
				}
			}
		}

		if userATransfer == nil {
			t.Fatal("UserA's transfer not found")
		}
		if userATransfer.State != api.TransferState_TRANSFER_STATE_CANCELLED {
			t.Errorf("Expected UserA's transfer to be CANCELLED, got %v", userATransfer.State)
		}

		if userCTransfer == nil {
			t.Fatal("UserC's transfer not found")
		}
		if userCTransfer.State != api.TransferState_TRANSFER_STATE_CANCELLED {
			t.Errorf("Expected UserC's transfer to be CANCELLED, got %v", userCTransfer.State)
		}
	})

	// Per docs/workflows/giveaway.md: Notifications are sent for:
	// - Interest expressed → owner receives notification
	// - Recipient selected → selected recipient receives notification
	t.Run("Postcondition_OwnerNotifiedOnInterest", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		ownerNotifs := logCapture.GetNotificationLogsForUser(ownerID)
		// Owner should receive 3 notifications (one for each user expressing interest)
		if len(ownerNotifs) != 3 {
			t.Errorf("Expected owner to receive 3 notifications (one per interested user), got %d", len(ownerNotifs))
			for _, n := range ownerNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
		// Verify notifications are interest-related
		for _, n := range ownerNotifs {
			if n.Title != "Interest in Old Tent" {
				t.Errorf("Expected notification title 'Interest in Old Tent', got '%s'", n.Title)
			}
		}
	})

	t.Run("Postcondition_RecipientNotifiedOnSelection", func(t *testing.T) {
		// Per docs/workflows/giveaway.md Example 1, User B's notifications are:
		//   1. "New giveaway" when the owner shares with the community
		//   2. "Request approved" when selected as recipient
		//   3. "Pickup proposed" when the owner confirms a pickup time
		// (Total of 8 notifications across all recipients: 3 "New giveaway" +
		// 3 "Interest in" + 1 "Request approved" + 1 "Pickup proposed".)
		WaitForNotificationCount(logCapture, 8, 5*time.Second)
		userBNotifs := logCapture.GetNotificationLogsForUser(userBID)
		if len(userBNotifs) != 3 {
			t.Errorf("Expected User B to receive 3 notifications (gear shared + selection + pickup), got %d", len(userBNotifs))
			for _, n := range userBNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
		var sawApproved, sawPickup, sawShared bool
		for _, n := range userBNotifs {
			switch n.Title {
			case "Request approved":
				sawApproved = true
			case "Pickup proposed":
				sawPickup = true
			case "New giveaway":
				sawShared = true
			}
		}
		if !sawApproved {
			t.Error("User B did not receive 'Request Approved' notification")
		}
		if !sawPickup {
			t.Error("User B did not receive 'Pickup Proposed' notification")
		}
		if !sawShared {
			t.Error("User B did not receive 'New giveaway' notification")
		}
	})

	// Impact estimation postconditions (per docs/workflows/giveaway.md Example 1)
	t.Run("Postcondition_TransferImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, completeImpact, "giveaway CompleteTransfer response")
		if completeImpact.MoneySaved != nil {
			assertEstimatePositive(t, completeImpact.MoneySaved.ValueUsd, "giveaway money_saved")
		} else {
			t.Error("money_saved is nil")
		}
		if completeImpact.TimeSaved != nil {
			assertEstimatePositive(t, completeImpact.TimeSaved.Minutes, "giveaway time_saved")
		} else {
			t.Error("time_saved is nil")
		}
	})

	t.Run("Postcondition_CommunityImpactMetrics", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertEstimatePositive(t, metrics.CostSavingsUsd, "community cost_savings_usd")
		assertEstimatePositive(t, metrics.TimeBankedMinutes, "community time_banked_minutes")
	})
}
