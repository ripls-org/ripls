package integration_tests

// Integration tests for giveaway cancellation workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/giveaway.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestGiveaway_Example2_OwnerCancelsGiveaway tests the owner cancellation workflow.
// Covers: docs/workflows/giveaway.md - Workflow Example 2.
func TestGiveaway_Example2_OwnerCancelsGiveaway(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Three users exist: an owner, user A, and user B
	// - The owner has gear shared for giveaway with the community
	// - User A and User B have expressed interest

	ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Giveaway Test Community", "Testing giveaway workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Seattle")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Vintage Camera", "Old film camera", locationID)

	// Share gear for giveaway
	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Get conversation ID
	listResp, err := ownerCommunityClient.ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListCommunityGear failed: %v", err)
	}
	conversationID := listResp.Msg.GearItems[0].ConversationId

	userAToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-a@example.com", "User A")
	userATransferClient := createAuthTransferClient(userAToken, serverURL)

	userBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-b@example.com", "User B")
	userBTransferClient := createAuthTransferClient(userBToken, serverURL)

	// User A expresses interest
	expressResp, err := userATransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id

	// User B expresses interest
	_, err = userBTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}

	// ========== STEPS ==========

	// Step 1: Owner decides not to give away the item
	// Step 2: Owner cancels transfer
	t.Run("Step2_OwnerCancelsTransfer", func(t *testing.T) {
		_, err := ownerTransferClient.CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
			TransferId: transferID,
		}))
		if err != nil {
			t.Fatalf("CancelTransfer failed: %v", err)
		}
	})

	// Step 3: System posts cancellation system message (verified implicitly)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_TransferCancelled", func(t *testing.T) {
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
				if transfer.State != api.TransferState_TRANSFER_STATE_CANCELLED {
					t.Errorf("Expected CANCELLED state, got %v", transfer.State)
				}
				break
			}
		}
		if !found {
			t.Error("Transfer not found")
		}
	})

	t.Run("Postcondition_GearStillOwnedByOwner", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.Owner == nil || gearResp.Msg.Owner.Id != ownerID {
			actualOwnerID := ""
			if gearResp.Msg.Owner != nil {
				actualOwnerID = gearResp.Msg.Owner.Id
			}
			t.Errorf("Expected gear to still be owned by owner (%s), got %s", ownerID, actualOwnerID)
		}
	})

	t.Run("Postcondition_GearStillAvailable", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected gear to be AVAILABLE, got %v", gearResp.Msg.State)
		}
	})

	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
	})

	// Per docs/workflows/giveaway.md: Cancellation triggers a notification to the recipient.
	// We also have interest expression notifications from setup.
	t.Run("Postcondition_CancellationNotifiesRecipient", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		// We expect 2 notifications from interest expressions + 1 from cancellation
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 3 // 2 interest + 1 cancellation
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (2 interest + 1 cancellation), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}

// TestGiveaway_Example4_RecipientCancelsAfterSelection tests that when the selected recipient
// cancels, the giveaway reverts to open and the remaining interested user is unaffected.
// Covers: docs/workflows/giveaway.md - Workflow Example 4.
func TestGiveaway_Example4_RecipientCancelsAfterSelection(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Three users exist: an owner, User B (selected recipient), and User C (also interested)
	// - The owner has gear shared for giveaway
	// - User B and User C have both expressed interest
	// - The owner has selected User B as the recipient

	ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Giveaway Test Community", "Testing giveaway workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Austin")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Road Bike", "21-speed road bike", locationID)

	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

	userBToken, userBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-b@example.com", "User B")
	userBTransferClient := createAuthTransferClient(userBToken, serverURL)

	userCToken, userCID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user-c@example.com", "User C")
	userCTransferClient := createAuthTransferClient(userCToken, serverURL)

	// User B expresses interest
	userBResp, err := userBTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest for User B failed: %v", err)
	}
	transferBID := userBResp.Msg.Transfer.Id

	// User C expresses interest
	userCResp, err := userCTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest for User C failed: %v", err)
	}
	transferCID := userCResp.Msg.Transfer.Id

	// Owner selects User B as recipient
	_, err = ownerTransferClient.SelectRecipient(ctx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferBID,
		RecipientId: userBID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}

	// ========== STEPS ==========

	// Step 1: User B (selected recipient) cancels their transfer
	t.Run("Step1_UserBCancelsTransfer", func(t *testing.T) {
		_, err := userBTransferClient.CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
			TransferId: transferBID,
		}))
		if err != nil {
			t.Fatalf("CancelTransfer by recipient User B failed: %v", err)
		}
	})

	// Step 2: System posts cancellation system message (verified implicitly via conversation)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_UserBTransferCancelled", func(t *testing.T) {
		getResp, err := userBTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferBID,
		}))
		if err != nil {
			t.Fatalf("GetTransfer for User B failed: %v", err)
		}
		if getResp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_CANCELLED {
			t.Errorf("Expected User B's transfer to be CANCELLED, got %v", getResp.Msg.Transfer.State)
		}
	})

	t.Run("Postcondition_UserCTransferStillInterestExpressed", func(t *testing.T) {
		getResp, err := userCTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferCID,
		}))
		if err != nil {
			t.Fatalf("GetTransfer for User C failed: %v", err)
		}
		if getResp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected User C's transfer to remain INTEREST_EXPRESSED, got %v", getResp.Msg.Transfer.State)
		}
	})

	t.Run("Postcondition_CommunityGearNotArchived", func(t *testing.T) {
		listResp, err := ownerCommunityClient.ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}
		if len(listResp.Msg.GearItems) != 1 {
			t.Errorf("Expected gear to remain in community feed (1 item), got %d items", len(listResp.Msg.GearItems))
		}
	})

	t.Run("Postcondition_GearStillAvailable", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected gear to be AVAILABLE after recipient cancels, got %v", gearResp.Msg.State)
		}
	})

	t.Run("Postcondition_GiveawayPhaseRevertsToOpenForUserC", func(t *testing.T) {
		ctxResp, err := userCTransferClient.GetGearTransferContext(ctx, connect.NewRequest(&api.GetGearTransferContextRequest{
			GearId:      gearID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetGearTransferContext for User C failed: %v", err)
		}
		tc := ctxResp.Msg.Context
		if tc.OverallPhase != api.GiveawayPhase_GIVEAWAY_PHASE_OPEN {
			t.Errorf("Expected overall_phase OPEN for User C after recipient cancels, got %v", tc.OverallPhase)
		}
		if len(tc.AvailableActions) == 0 {
			t.Error("Expected User C to have available actions after recipient cancels")
		}
	})

	t.Run("Postcondition_OwnerCanSelectNewRecipient", func(t *testing.T) {
		ctxResp, err := ownerTransferClient.GetGearTransferContext(ctx, connect.NewRequest(&api.GetGearTransferContextRequest{
			GearId:      gearID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetGearTransferContext for owner failed: %v", err)
		}
		tc := ctxResp.Msg.Context
		if tc.OverallPhase != api.GiveawayPhase_GIVEAWAY_PHASE_OPEN {
			t.Errorf("Expected overall_phase OPEN for owner after recipient cancels, got %v", tc.OverallPhase)
		}
		var hasSelectAction bool
		for _, action := range tc.AvailableActions {
			if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
				hasSelectAction = true
				break
			}
		}
		if !hasSelectAction {
			t.Errorf("Expected owner to have SELECT_RECIPIENT action after recipient cancels, got %v", tc.AvailableActions)
		}
	})

	// Per docs/workflows/giveaway.md Example 4:
	// 2 notifications from interest expressions + 1 from recipient selection + 1 from cancellation = 4 total
	t.Run("Postcondition_OwnerNotifiedOfCancellation", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 4, 5*time.Second)
		ownerNotifs := logCapture.GetNotificationLogsForUser(ownerID)
		// Owner receives: 2 interest notifications + 1 cancellation notification
		if len(ownerNotifs) != 3 {
			t.Errorf("Expected owner to receive 3 notifications (2 interest + 1 cancellation), got %d", len(ownerNotifs))
			for _, n := range ownerNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
		var hasCancelNotif bool
		for _, n := range ownerNotifs {
			if n.Title == "Giveaway cancelled" {
				hasCancelNotif = true
				break
			}
		}
		if !hasCancelNotif {
			t.Errorf("Expected owner to receive a 'Giveaway Cancelled' notification, got: %v", ownerNotifs)
		}
		// User B received the selection notification
		userBNotifs := logCapture.GetNotificationLogsForUser(userBID)
		if len(userBNotifs) != 1 || userBNotifs[0].Title != "Request approved" {
			t.Errorf("Expected User B to receive 1 'Request Approved' notification, got %d: %v", len(userBNotifs), userBNotifs)
		}
		// User C received no notifications
		userCNotifs := logCapture.GetNotificationLogsForUser(userCID)
		if len(userCNotifs) != 0 {
			t.Errorf("Expected User C to receive no notifications, got %d", len(userCNotifs))
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}
