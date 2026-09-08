package integration_tests

// Integration tests for cumulative loan impact workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/loan.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestLoan_Example5_MultipleLoansCumulativeImpact tests that cumulative impact scales correctly.
// Covers: docs/workflows/loan.md - Workflow Example 5.
func TestLoan_Example5_MultipleLoansCumulativeImpact(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Three users exist: an owner, borrower A, and borrower B
	// - The owner has gear shared for loan with metadata (value, material, weight)

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Gear Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerLocationClient := createAuthLocationClient(ownerToken, serverURL)
	ownerTransferClient := createAuthTransferClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Cumulative Impact Community", "Testing cumulative impact")
	locationID := setupTestLocation(t, ctx, ownerLocationClient, "Portland")
	gearID := setupTestGear(t, ctx, ownerGearClient, "Circular Saw", "DeWalt 7.25 inch", locationID)

	shareGearIntoCommunity(t, ctx, ownerCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	borrowerAToken, borrowerAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower-a@example.com", "Borrower A")
	borrowerATransferClient := createAuthTransferClient(borrowerAToken, serverURL)

	borrowerBToken, borrowerBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "borrower-b@example.com", "Borrower B")
	borrowerBTransferClient := createAuthTransferClient(borrowerBToken, serverURL)

	// ========== STEPS ==========

	// Steps 1-3: Borrower A borrows and returns gear
	var impactA *api.ImpactEstimate
	t.Run("Steps1to3_LoanACycle", func(t *testing.T) {
		expressResp, err := borrowerATransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest A failed: %v", err)
		}
		transferAID := expressResp.Msg.Transfer.Id

		_, err = ownerTransferClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{
			TransferId: transferAID,
		}))
		if err != nil {
			t.Fatalf("StartLoan A failed: %v", err)
		}

		completeResp, err := borrowerATransferClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
			TransferId: transferAID,
		}))
		if err != nil {
			t.Fatalf("CompleteTransfer A failed: %v", err)
		}
		impactA = completeResp.Msg.Impact
	})

	// Steps 4-6: Borrower B borrows and returns the same gear
	var impactB *api.ImpactEstimate
	t.Run("Steps4to6_LoanBCycle", func(t *testing.T) {
		expressResp, err := borrowerBTransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest B failed: %v", err)
		}
		transferBID := expressResp.Msg.Transfer.Id

		_, err = ownerTransferClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{
			TransferId: transferBID,
		}))
		if err != nil {
			t.Fatalf("StartLoan B failed: %v", err)
		}

		completeResp, err := borrowerBTransferClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
			TransferId: transferBID,
		}))
		if err != nil {
			t.Fatalf("CompleteTransfer B failed: %v", err)
		}
		impactB = completeResp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_TransferImpactsIdentical", func(t *testing.T) {
		// Same gear, same config → identical impact estimates
		assertImpactPopulated(t, impactA, "transfer A impact")
		assertImpactPopulated(t, impactB, "transfer B impact")

		if impactA.MoneySaved != nil && impactB.MoneySaved != nil {
			assertEstimateApproxEqual(t, impactA.MoneySaved.ValueUsd, impactB.MoneySaved.ValueUsd, "transfer A vs B money_saved")
		}
		if impactA.TimeSaved != nil && impactB.TimeSaved != nil {
			assertEstimateApproxEqual(t, impactA.TimeSaved.Minutes, impactB.TimeSaved.Minutes, "transfer A vs B time_saved")
		}
	})

	t.Run("Postcondition_GearStatsCumulativeImpact", func(t *testing.T) {
		// Gear stats should show 2× single transfer impact (timesLoaned=2)
		statsResp, err := ownerGearClient.GetGearStats(ctx, connect.NewRequest(&api.GetGearStatsRequest{
			GearId: gearID,
		}))
		if err != nil {
			t.Fatalf("GetGearStats failed: %v", err)
		}

		if statsResp.Msg.TimesLoaned != 2 {
			t.Errorf("Expected times_loaned=2, got %d", statsResp.Msg.TimesLoaned)
		}

		gearImpact := statsResp.Msg.Impact
		assertImpactPopulated(t, gearImpact, "gear cumulative impact")

		// Gear cumulative money should be ≈ 2× single transfer
		if impactA.MoneySaved != nil && gearImpact.MoneySaved != nil {
			expectedMean := impactA.MoneySaved.ValueUsd.Mean * 2
			expected := &api.Estimate{Mean: expectedMean}
			actual := &api.Estimate{Mean: gearImpact.MoneySaved.ValueUsd.Mean}
			assertEstimateApproxEqual(t, expected, actual, "gear cumulative money = 2x single")
		}

		// Gear cumulative time should be ≈ 2× single transfer
		if impactA.TimeSaved != nil && gearImpact.TimeSaved != nil {
			expectedMean := impactA.TimeSaved.Minutes.Mean * 2
			expected := &api.Estimate{Mean: expectedMean}
			actual := &api.Estimate{Mean: gearImpact.TimeSaved.Minutes.Mean}
			assertEstimateApproxEqual(t, expected, actual, "gear cumulative time = 2x single")
		}
	})

	t.Run("Postcondition_CommunityMetricsSumOfBoth", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)

		// Community time should be ≈ sum of both transfers
		if impactA.TimeSaved != nil && impactB.TimeSaved != nil {
			expectedTime := impactA.TimeSaved.Minutes.Mean + impactB.TimeSaved.Minutes.Mean
			expected := &api.Estimate{Mean: expectedTime}
			actual := &api.Estimate{Mean: estimateMean(metrics.TimeBankedMinutes)}
			assertEstimateApproxEqual(t, expected, actual, "community time = sum of transfers")
		}

		// Community cost should be ≈ sum of both transfers
		if impactA.MoneySaved != nil && impactB.MoneySaved != nil {
			expectedCost := impactA.MoneySaved.ValueUsd.Mean + impactB.MoneySaved.ValueUsd.Mean
			expected := &api.Estimate{Mean: expectedCost}
			actual := &api.Estimate{Mean: estimateMean(metrics.CostSavingsUsd)}
			assertEstimateApproxEqual(t, expected, actual, "community cost = sum of transfers")
		}
	})

	t.Run("Postcondition_EachBorrowerHasOwnSavings", func(t *testing.T) {
		userClientA := createAuthUserClient(borrowerAToken, serverURL)
		statsA, err := userClientA.GetUserStats(ctx, connect.NewRequest(&api.GetUserStatsRequest{
			UserId: borrowerAID,
		}))
		if err != nil {
			t.Fatalf("GetUserStats A failed: %v", err)
		}
		if statsA.Msg.Savings == nil || statsA.Msg.Savings.CostSavedUsd <= 0 {
			t.Error("Expected borrower A to have positive cost savings")
		}

		userClientB := createAuthUserClient(borrowerBToken, serverURL)
		statsB, err := userClientB.GetUserStats(ctx, connect.NewRequest(&api.GetUserStatsRequest{
			UserId: borrowerBID,
		}))
		if err != nil {
			t.Fatalf("GetUserStats B failed: %v", err)
		}
		if statsB.Msg.Savings == nil || statsB.Msg.Savings.CostSavedUsd <= 0 {
			t.Error("Expected borrower B to have positive cost savings")
		}
	})

	// Per docs/workflows/loan.md Example 5:
	// - GEAR_SHARED fires before any borrowers exist, so it has no recipients.
	// - Each loan cycle (StartLoan) emits TRANSFER_ACTIVE → "Loan started" to the
	//   borrower for that cycle. Two cycles → two notifications.
	t.Run("Postcondition_LoanStartedNotificationsPerCycle", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 2, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		if len(notifLogs) != 2 {
			t.Errorf("Expected 2 notifications (one Loan Started per cycle), got %d", len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
		for _, n := range notifLogs {
			if n.Title != "Loan started" {
				t.Errorf("Expected 'Loan Started' notifications, got %q", n.Title)
			}
		}
	})
}
