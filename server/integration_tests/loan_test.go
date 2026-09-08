package integration_tests

// Integration tests for loan workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/loan.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestLoan_Example1_SharingAndBorrowingGear tests the happy path loan workflow.
// Covers: docs/workflows/loan.md - Workflow Example 1.
func TestLoan_Example1_SharingAndBorrowingGear(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Three users exist: an owner, borrower A, and borrower B, all members of the community
	// - The owner has gear that is not yet shared

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Loan Test Community", "Testing loan workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Portland")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Mountain Bike", "Trek hardtail", locationID)

	borrowerAToken, borrowerAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower-a@example.com", "Borrower A")
	borrowerATransferClient := createAuthTransferClient(borrowerAToken, serverURL)
	borrowerAChatClient := createAuthChatClient(borrowerAToken, serverURL)

	borrowerBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower-b@example.com", "Borrower B")
	borrowerBTransferClient := createAuthTransferClient(borrowerBToken, serverURL)

	var conversationID string
	var transferAID string

	// ========== STEPS ==========

	// Step 1: Owner shares gear with community for loan
	t.Run("Step1_OwnerSharesGear", func(t *testing.T) {
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)
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

	// Step 3: Borrower A views the conversation (auto-adds them as participant)
	t.Run("Step3_BorrowerAViewsConversation", func(t *testing.T) {
		_, err := borrowerAChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
	})

	// Step 4: Borrower A sends message to gear conversation asking about the item
	t.Run("Step4_BorrowerAAsksQuestion", func(t *testing.T) {
		_, err := borrowerAChatClient.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           "Is this bike available this weekend?",
		}))
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
	})

	// Step 5: Owner responds with a message in the conversation
	t.Run("Step5_OwnerResponds", func(t *testing.T) {
		_, err := ownerChatClient.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           "Yes, it's available! Let me know if you want to borrow it.",
		}))
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
	})

	// Step 6: Borrower A expresses interest (loans are auto-approved)
	t.Run("Step6_BorrowerAExpressesInterest", func(t *testing.T) {
		resp, err := borrowerATransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		transferAID = resp.Msg.Transfer.Id
		// Loans are auto-approved, so expect RECIPIENT_SELECTED
		if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			t.Errorf("Expected RECIPIENT_SELECTED state (auto-approved loan), got %v", resp.Msg.Transfer.State)
		}
	})

	// Step 7: Borrower B also expresses interest (loans are auto-approved)
	var transferBID string
	t.Run("Step7_BorrowerBExpressesInterest", func(t *testing.T) {
		resp, err := borrowerBTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		transferBID = resp.Msg.Transfer.Id
		// Loans are auto-approved, so expect RECIPIENT_SELECTED
		if resp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			t.Errorf("Expected RECIPIENT_SELECTED state (auto-approved loan), got %v", resp.Msg.Transfer.State)
		}
	})

	// Step 8: Owner sees both borrowers (via transfer list)
	t.Run("Step8_OwnerSeesBothBorrowers", func(t *testing.T) {
		listResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}
		// Should have at least 2 transfers (one from each borrower)
		if len(listResp.Msg.Transfers) < 2 {
			t.Errorf("Expected at least 2 transfers, got %d", len(listResp.Msg.Transfers))
		}
	})

	// Step 9: Owner selects Borrower A (loans are auto-approved, so this step is automatic)
	t.Run("Step9_OwnerSelectsBorrowerA", func(t *testing.T) {
		// Loans are auto-approved, so SelectRecipient is not needed
		// Verify transfer A is already in RECIPIENT_SELECTED state
		listResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}
		var foundSelected bool
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Id == transferAID && transfer.State == api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
				foundSelected = true
				break
			}
		}
		if !foundSelected {
			t.Error("Expected transfer A to be in RECIPIENT_SELECTED state (auto-approved)")
		}
	})

	// Step 10: Owner starts loan
	t.Run("Step10_OwnerStartsLoan", func(t *testing.T) {
		_, err := ownerTransferClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{
			TransferId: transferAID,
		}))
		if err != nil {
			t.Fatalf("StartLoan failed: %v", err)
		}

		// Verify gear is unavailable
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_UNAVAILABLE {
			t.Errorf("Expected gear to be UNAVAILABLE, got %v", gearResp.Msg.State)
		}
	})

	// Step 11: Borrower A returns item and marks loan complete
	var completeImpact *api.ImpactEstimate
	t.Run("Step11_BorrowerACompletesLoan", func(t *testing.T) {
		resp, err := borrowerATransferClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
			TransferId: transferAID,
		}))
		if err != nil {
			t.Fatalf("CompleteTransfer failed: %v", err)
		}
		completeImpact = resp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_GearStillShared", func(t *testing.T) {
		listResp, err := ownerCommunityClient.ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}
		if len(listResp.Msg.GearItems) != 1 {
			t.Errorf("Expected gear to still be shared, got %d items", len(listResp.Msg.GearItems))
		}
	})

	t.Run("Postcondition_GearAvailable", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected gear to be AVAILABLE after loan completion, got %v", gearResp.Msg.State)
		}
	})

	t.Run("Postcondition_TransferBStillActive", func(t *testing.T) {
		// Transfer B should be in RECIPIENT_SELECTED state (loans are auto-approved)
		listResp, err := borrowerBTransferClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers failed: %v", err)
		}
		var foundTransferB bool
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Id == transferBID {
				foundTransferB = true
				if transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
					t.Errorf("Expected transfer B to be in RECIPIENT_SELECTED (auto-approved), got %v", transfer.State)
				}
				break
			}
		}
		if !foundTransferB {
			t.Error("Transfer B not found in borrower B's received transfers")
		}
	})

	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
		// Conversation remains accessible and contains system messages for state transitions
	})

	// Per docs/workflows/loan.md Example 1:
	// - GEAR_SHARED broadcasts "New gear to borrow" to each member except the actor (the owner).
	//   Example 1 has Borrower A and Borrower B → 2 notifications.
	// - TRANSFER_ACTIVE on StartLoan notifies Borrower A → "Loan started".
	// - INTEREST_EXPRESSED is skipped (auto-approval) and RECIPIENT_SELECTED self-notification
	//   is suppressed because actor == recipient.
	t.Run("Postcondition_NotificationsMatchWorkflow", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		if len(notifLogs) != 3 {
			t.Errorf("Expected 3 notifications (2 New gear to borrow + 1 Loan Started), got %d", len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
		var sharedCount, loanStartedCount int
		for _, n := range notifLogs {
			switch n.Title {
			case "New gear to borrow":
				sharedCount++
			case "Loan started":
				loanStartedCount++
			}
		}
		if sharedCount != 2 {
			t.Errorf("Expected 2 'New gear to borrow' notifications, got %d", sharedCount)
		}
		if loanStartedCount != 1 {
			t.Errorf("Expected 1 'Loan Started' notification, got %d", loanStartedCount)
		}
	})

	// Impact estimation postconditions (per docs/workflows/loan.md Example 1)
	t.Run("Postcondition_TransferImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, completeImpact, "CompleteTransfer response")
		// All 3 dimensions should be populated with positive means and stddevs
		if completeImpact.MoneySaved != nil {
			assertEstimatePositive(t, completeImpact.MoneySaved.ValueUsd, "money_saved.value_usd")
		} else {
			t.Error("money_saved is nil")
		}
		if completeImpact.EmissionsPrevented != nil {
			if completeImpact.EmissionsPrevented.ManufactureAvoidedCarbon != nil {
				assertEstimatePositive(t, completeImpact.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams, "manufacture_avoided_carbon")
			} else {
				t.Error("manufacture_avoided_carbon is nil")
			}
			if completeImpact.EmissionsPrevented.WasteReducedCarbon != nil {
				assertEstimatePositive(t, completeImpact.EmissionsPrevented.WasteReducedCarbon.Co2EGrams, "waste_reduced_carbon")
			} else {
				t.Error("waste_reduced_carbon is nil")
			}
		} else {
			t.Error("emissions_prevented is nil")
		}
		if completeImpact.TimeSaved != nil {
			assertEstimatePositive(t, completeImpact.TimeSaved.Minutes, "time_saved.minutes")
		} else {
			t.Error("time_saved is nil")
		}
	})

	t.Run("Postcondition_GearStatsImpact", func(t *testing.T) {
		// Gear stats cumulative impact should equal the single transfer's impact (timesLoaned=1)
		statsResp, err := ownerGearClient.GetGearStats(ctx, connect.NewRequest(&api.GetGearStatsRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}
		gearImpact := statsResp.Msg.Impact
		assertImpactPopulated(t, gearImpact, "gear stats")

		// Money saved should match transfer IE
		if completeImpact.MoneySaved != nil && gearImpact.MoneySaved != nil {
			assertEstimateApproxEqual(t, completeImpact.MoneySaved.ValueUsd, gearImpact.MoneySaved.ValueUsd, "gear stats money_saved vs transfer IE")
		}
		// Time saved should match transfer IE
		if completeImpact.TimeSaved != nil && gearImpact.TimeSaved != nil {
			assertEstimateApproxEqual(t, completeImpact.TimeSaved.Minutes, gearImpact.TimeSaved.Minutes, "gear stats time_saved vs transfer IE")
		}
	})

	t.Run("Postcondition_CommunityImpactMetrics", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)

		// Community metrics should have positive values matching the single completed transfer
		assertEstimatePositive(t, metrics.CostSavingsUsd, "community cost_savings_usd")
		assertEstimatePositive(t, metrics.CarbonSavingsGrams, "community carbon_savings_grams")
		assertEstimatePositive(t, metrics.TimeBankedMinutes, "community time_banked_minutes")

		// Community time should match the transfer's time saved
		if completeImpact.TimeSaved != nil {
			assertEstimateApproxEqual(t, completeImpact.TimeSaved.Minutes, metrics.TimeBankedMinutes, "community time vs transfer IE")
		}
	})

	t.Run("Postcondition_UserStatsSavings", func(t *testing.T) {
		userClient := createAuthUserClient(borrowerAToken, serverURL)
		statsResp, err := userClient.GetUserStats(ctx, connect.NewRequest(&api.GetUserStatsRequest{
			UserId: borrowerAID,
		}))
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}
		savings := statsResp.Msg.Savings
		if savings == nil {
			t.Fatal("UserSavings is nil")
		}
		if savings.CostSavedUsd <= 0 {
			t.Errorf("Expected cost_saved_usd > 0, got %f", savings.CostSavedUsd)
		}
		if savings.Co2SavedKg <= 0 {
			t.Errorf("Expected co2_saved_kg > 0, got %f", savings.Co2SavedKg)
		}
		if savings.TimeSavedHours <= 0 {
			t.Errorf("Expected time_saved_hours > 0, got %d", savings.TimeSavedHours)
		}
	})
}
