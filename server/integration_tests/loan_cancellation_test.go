package integration_tests

// Integration tests for loan withdrawal, decline, and cancellation workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/loan.md.

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestLoan_Example2_BorrowerWithdrawsInterest tests the borrower withdrawal workflow.
// Covers: docs/workflows/loan.md - Workflow Example 2.
func TestLoan_Example2_BorrowerWithdrawsInterest(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Two users exist: an owner and a borrower
	// - The owner has gear shared for loan with the community
	// - The borrower has expressed interest in the gear

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Loan Test Community", "Testing loan workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Seattle")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Kayak", "Two-person kayak", locationID)

	// Share gear for loan
	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	borrowerToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower@example.com", "Borrower")
	borrowerTransferClient := createAuthTransferClient(borrowerToken, serverURL)

	// Borrower expresses interest
	expressResp, err := borrowerTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id
	conversationID := expressResp.Msg.Transfer.GetConversationId()

	// ========== STEPS ==========

	// Step 1: Borrower withdraws interest via WithdrawInterest
	t.Run("Step1_BorrowerWithdrawsInterest", func(t *testing.T) {
		_, err := borrowerTransferClient.WithdrawInterest(ctx, connect.NewRequest(&api.WithdrawInterestRequest{
			TransferId: transferID,
		}))
		if err != nil {
			t.Fatalf("WithdrawInterest failed: %v", err)
		}
	})

	// Step 2: System marks transfer as cancelled and posts withdrawal system message
	t.Run("Step2_TransferCancelled", func(t *testing.T) {
		listResp, err := borrowerTransferClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers failed: %v", err)
		}
		var foundCancelled bool
		for _, transfer := range listResp.Msg.Transfers {
			if transfer.Id == transferID && transfer.State == api.TransferState_TRANSFER_STATE_CANCELLED {
				foundCancelled = true
				break
			}
		}
		if !foundCancelled {
			t.Error("Expected transfer to be in CANCELLED state")
		}
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_TransferCancelled", func(t *testing.T) {
		listResp, err := borrowerTransferClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers failed: %v", err)
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

	t.Run("Postcondition_BorrowerStillInConversation", func(t *testing.T) {
		borrowerChatClient := createAuthChatClient(borrowerToken, serverURL)
		_, err := borrowerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		// Borrower should still be able to access the conversation
		if err != nil {
			t.Errorf("Borrower should still have access to conversation: %v", err)
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
			t.Errorf("Expected gear to remain AVAILABLE, got %v", gearResp.Msg.State)
		}
	})

	// Per docs/workflows/loan.md Example 2: withdrawal sends an "Interest withdrawn"
	// notification to the gear owner (TRANSFER_INTEREST_WITHDRAWN).
	t.Run("Postcondition_OwnerNotifiedOfWithdrawal", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 1, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		if len(notifLogs) != 1 {
			t.Errorf("Expected 1 notification (interest withdrawn to owner), got %d", len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
		if len(notifLogs) > 0 && !strings.HasPrefix(notifLogs[0].Title, "Interest withdrawn") {
			t.Errorf("Expected 'Interest withdrawn' notification, got %q", notifLogs[0].Title)
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		// No completed loans, so gear stats and community metrics should show zero impact
		statsResp, err := ownerGearClient.GetGearStats(ctx, connect.NewRequest(&api.GetGearStatsRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}
		assertImpactZero(t, statsResp.Msg.Impact, "gear stats after withdrawal")

		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}

// TestLoan_Example3_OwnerDeclinesRequest tests the owner decline workflow.
// Covers: docs/workflows/loan.md - Workflow Example 3.
func TestLoan_Example3_OwnerDeclinesRequest(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Two users exist: an owner and a borrower
	// - The owner has gear shared for loan with the community
	// - The borrower has expressed interest in the gear

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Loan Test Community", "Testing loan workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Denver")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Camping Tent", "4-person tent", locationID)

	// Share gear for giveaway (use giveaway since loans are auto-approved and skip INTEREST_EXPRESSED state)
	// This test needs INTEREST_EXPRESSED state to test owner declining before approval
	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

	borrowerToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower@example.com", "Borrower")
	borrowerTransferClient := createAuthTransferClient(borrowerToken, serverURL)

	// Borrower expresses interest
	expressResp, err := borrowerTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id

	// ========== STEPS ==========

	// Step 1: Owner reviews request, decides not to lend
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
		listResp, err := borrowerTransferClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_GIVEAWAY,
		}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers failed: %v", err)
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

	t.Run("Postcondition_GearStillAvailable", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected gear to remain AVAILABLE, got %v", gearResp.Msg.State)
		}
	})

	// Per docs/workflows/loan.md Example 3 (giveaway availability, gear is
	// shared *before* the borrower joins):
	// - GEAR_SHARED has no recipients at share time (no other members yet).
	// - INTEREST_EXPRESSED notifies the owner when borrower expresses interest.
	// - TRANSFER_CANCELLED notifies the borrower when the owner cancels.
	t.Run("Postcondition_CancellationNotifiesBorrower", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 2, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 2 // interest + cancellation
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (interest + cancellation), got %d", expectedCount, len(notifLogs))
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

// TestLoan_Example4_LoanCancelledAfterApproval tests the post-approval cancellation workflow.
// Covers: docs/workflows/loan.md - Workflow Example 4.
func TestLoan_Example4_LoanCancelledAfterApproval(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Two users exist: an owner and a borrower
	// - The owner has gear shared for loan
	// - A loan has been approved (transfer in RECIPIENT_SELECTED state)

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Loan Test Community", "Testing loan workflows")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Austin")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Stand-up Paddleboard", "Inflatable SUP", locationID)

	// Share gear for loan
	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	borrowerToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower@example.com", "Borrower")
	borrowerTransferClient := createAuthTransferClient(borrowerToken, serverURL)

	// Borrower expresses interest
	expressResp, err := borrowerTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id

	// Loan is already auto-approved, no need to call SelectRecipient

	// ========== STEPS ==========

	// Step 1: Either party cancels the transfer
	t.Run("Step1_CancelTransfer", func(t *testing.T) {
		_, err := borrowerTransferClient.CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
			TransferId: transferID,
		}))
		if err != nil {
			t.Fatalf("CancelTransfer failed: %v", err)
		}
	})

	// Step 2: System marks transfer as CANCELLED

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_TransferCancelled", func(t *testing.T) {
		listResp, err := borrowerTransferClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		}))
		if err != nil {
			t.Fatalf("ListReceivedTransfers failed: %v", err)
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

	t.Run("Postcondition_GearStillAvailable", func(t *testing.T) {
		gearResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}
		if gearResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected gear to remain AVAILABLE, got %v", gearResp.Msg.State)
		}
	})

	// Per docs/workflows/loan.md: Cancellation triggers a notification to the other party.
	// For auto-approved loans, the borrower cancels and the owner is notified.
	t.Run("Postcondition_CancellationNotifiesOwner", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 1, 5*time.Second)
		// The cancellation notification is sent to the owner (since borrower cancelled)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 1 // Cancellation notification only
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notification (cancellation to owner), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		statsResp, err := ownerGearClient.GetGearStats(ctx, connect.NewRequest(&api.GetGearStatsRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}
		assertImpactZero(t, statsResp.Msg.Impact, "gear stats after cancellation")

		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}
