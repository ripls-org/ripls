package transfer

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGetGearTransferContext_OwnerSeesAllPendingRequests verifies that the gear owner
// can see all pending transfer requests.
func TestGetGearTransferContext_OwnerSeesAllPendingRequests(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and two borrowers
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrower1ID := setupTestUser(t, testStorage, "Borrower 1", "borrower1@example.com")
	borrower2ID := setupTestUser(t, testStorage, "Borrower 2", "borrower2@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear as giveaway (giveaways stay in INTEREST_EXPRESSED state)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrower1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add borrower2 to community
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	borrower2Membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      borrower2ID,
		InviterId:   ownerID,
	}
	_, err := testStorage.Insert(ctx, borrower2Membership)
	if err != nil {
		t.Fatalf("Failed to add borrower2 membership: %v", err)
	}

	// Borrower 1 expresses interest
	borrower1Ctx := createAuthenticatedContext(borrower1ID, "borrower1@example.com", models.Role_ROLE_USER)
	expressReq1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(borrower1Ctx, expressReq1)
	if err != nil {
		t.Fatalf("Failed to express interest (borrower 1): %v", err)
	}

	// Borrower 2 expresses interest
	borrower2Ctx := createAuthenticatedContext(borrower2ID, "borrower2@example.com", models.Role_ROLE_USER)
	expressReq2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(borrower2Ctx, expressReq2)
	if err != nil {
		t.Fatalf("Failed to express interest (borrower 2): %v", err)
	}

	// Owner calls GetGearTransferContext
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(ownerCtx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Owner sees both pending requests
	if len(resp.Msg.Context.PendingRequests) != 2 {
		t.Errorf("Expected 2 pending requests, got %d", len(resp.Msg.Context.PendingRequests))
	}

	// Verify: Owner has no user_transfer while transfers are in INTEREST_EXPRESSED state
	// (Owner only gets user_transfer populated after selecting a recipient)
	if resp.Msg.Context.UserTransfer != nil {
		t.Error("Expected owner to have no user_transfer before selecting recipient")
	}

	// Verify: Available actions include SELECT_RECIPIENT for owner
	hasSelectAction := false
	for _, action := range resp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
			hasSelectAction = true
			break
		}
	}
	if !hasSelectAction {
		t.Error("Expected owner to have SELECT_RECIPIENT action available")
	}
}

// TestGetGearTransferContext_NonOwnerSeesAllRequests verifies that non-owners
// can see all pending requests (but can only act on their own).
func TestGetGearTransferContext_NonOwnerSeesAllRequests(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and two borrowers
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrower1ID := setupTestUser(t, testStorage, "Borrower 1", "borrower1@example.com")
	borrower2ID := setupTestUser(t, testStorage, "Borrower 2", "borrower2@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear as giveaway (giveaways stay in INTEREST_EXPRESSED state)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrower1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add borrower2 to community
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	borrower2Membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      borrower2ID,
		InviterId:   ownerID,
	}
	_, err := testStorage.Insert(ctx, borrower2Membership)
	if err != nil {
		t.Fatalf("Failed to add borrower2 membership: %v", err)
	}

	// Borrower 1 expresses interest
	borrower1Ctx := createAuthenticatedContext(borrower1ID, "borrower1@example.com", models.Role_ROLE_USER)
	expressReq1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(borrower1Ctx, expressReq1)
	if err != nil {
		t.Fatalf("Failed to express interest (borrower 1): %v", err)
	}

	// Borrower 2 expresses interest
	borrower2Ctx := createAuthenticatedContext(borrower2ID, "borrower2@example.com", models.Role_ROLE_USER)
	expressReq2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(borrower2Ctx, expressReq2)
	if err != nil {
		t.Fatalf("Failed to express interest (borrower 2): %v", err)
	}

	// Borrower 1 calls GetGearTransferContext
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(borrower1Ctx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Borrower 1 sees all pending requests (including their own and borrower 2's)
	if len(resp.Msg.Context.PendingRequests) != 2 {
		t.Errorf("Expected 2 pending requests, got %d", len(resp.Msg.Context.PendingRequests))
	}

	// Verify: Borrower 1 has their own user_transfer
	if resp.Msg.Context.UserTransfer == nil {
		t.Fatal("Expected borrower to have user_transfer")
	}
	if resp.Msg.Context.UserTransfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected user_transfer state INTEREST_EXPRESSED, got %v", resp.Msg.Context.UserTransfer.State)
	}

	// Verify: Available actions include WITHDRAW_INTEREST for borrower
	hasWithdrawAction := false
	for _, action := range resp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_WITHDRAW_INTEREST {
			hasWithdrawAction = true
			break
		}
	}
	if !hasWithdrawAction {
		t.Error("Expected borrower to have WITHDRAW_INTEREST action available")
	}
}

// TestGetGearTransferContext_UserWithOwnTransfer verifies that a user's own transfer
// is included in the context.
func TestGetGearTransferContext_UserWithOwnTransfer(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and borrower
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear as giveaway (giveaways stay in INTEREST_EXPRESSED state)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Borrower expresses interest
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(borrowerCtx, expressReq)
	if err != nil {
		t.Fatalf("Failed to express interest: %v", err)
	}

	// Borrower calls GetGearTransferContext
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(borrowerCtx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Borrower has their own user_transfer
	if resp.Msg.Context.UserTransfer == nil {
		t.Fatal("Expected borrower to have user_transfer")
	}
	if resp.Msg.Context.UserTransfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected transfer state INTEREST_EXPRESSED, got %v", resp.Msg.Context.UserTransfer.State)
	}
}

// TestGetGearTransferContext_AvailableActionsCorrectPerRole verifies that available actions
// are correct based on user role (owner vs borrower).
func TestGetGearTransferContext_AvailableActionsCorrectPerRole(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and borrower
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear as giveaway (giveaways stay in INTEREST_EXPRESSED state)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Borrower expresses interest
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(borrowerCtx, expressReq)
	if err != nil {
		t.Fatalf("Failed to express interest: %v", err)
	}

	// Test 1: Owner's available actions
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ownerReq := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	ownerResp, err := service.GetGearTransferContext(ownerCtx, ownerReq)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context (owner): %v", err)
	}

	// Owner should have SELECT_RECIPIENT action
	hasSelectAction := false
	for _, action := range ownerResp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
			hasSelectAction = true
			break
		}
	}
	if !hasSelectAction {
		t.Error("Expected owner to have SELECT_RECIPIENT action")
	}

	// Test 2: Borrower's available actions
	borrowerReq := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	borrowerResp, err := service.GetGearTransferContext(borrowerCtx, borrowerReq)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context (borrower): %v", err)
	}

	// Borrower should have WITHDRAW_INTEREST action
	hasWithdrawAction := false
	for _, action := range borrowerResp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_WITHDRAW_INTEREST {
			hasWithdrawAction = true
			break
		}
	}
	if !hasWithdrawAction {
		t.Error("Expected borrower to have WITHDRAW_INTEREST action")
	}

	// Borrower should NOT have SELECT_RECIPIENT action
	for _, action := range borrowerResp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
			t.Error("Expected borrower to NOT have SELECT_RECIPIENT action")
		}
	}
}

// TestGetGearTransferContext_OwnerSeesCompleteActionForGiveaway verifies that after selecting
// a recipient for a giveaway, the owner sees the COMPLETE action (not START_LOAN).
// This is because giveaways skip the ACTIVE state and go directly to COMPLETED.
func TestGetGearTransferContext_OwnerSeesCompleteActionForGiveaway(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and recipient
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear for giveaway
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Recipient expresses interest
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	expressResp, err := service.ExpressInterest(recipientCtx, expressReq)
	if err != nil {
		t.Fatalf("Failed to express interest: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id

	// Owner selects recipient
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectReq := connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	})
	_, err = service.SelectRecipient(ownerCtx, selectReq)
	if err != nil {
		t.Fatalf("Failed to select recipient: %v", err)
	}

	// Owner calls GetGearTransferContext
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(ownerCtx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Owner has the transfer in RECIPIENT_SELECTED state
	if resp.Msg.Context.UserTransfer == nil {
		t.Fatal("Expected owner to have UserTransfer populated after selecting recipient")
	}
	if resp.Msg.Context.UserTransfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected transfer state RECIPIENT_SELECTED, got %v", resp.Msg.Context.UserTransfer.State)
	}
	if resp.Msg.Context.UserTransfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("Expected transfer type GIVEAWAY, got %v", resp.Msg.Context.UserTransfer.TransferType)
	}

	// Verify: Owner sees COMPLETE action (not START_LOAN) for giveaway
	hasCompleteAction := false
	hasStartLoanAction := false
	for _, action := range resp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_COMPLETE {
			hasCompleteAction = true
		}
		if action == api.TransferAction_TRANSFER_ACTION_START_LOAN {
			hasStartLoanAction = true
		}
	}
	if !hasCompleteAction {
		t.Error("Expected owner to have COMPLETE action for giveaway in RECIPIENT_SELECTED state")
	}
	if hasStartLoanAction {
		t.Error("Expected owner to NOT have START_LOAN action for giveaway (giveaways skip ACTIVE state)")
	}
}

// TestGetGearTransferContext_ActiveLoanIncludesBorrowerInPendingRequests verifies that
// when a loan is in ACTIVE state, the borrower info is still available in pendingRequests.
// This is important for the UI to show who currently has the item.
// TestGetGearTransferContext_ActiveLoanExcludedFromPendingRequests verifies
// that once a loan is ACTIVE its transfer is NOT repeated in pending_requests:
// the holder is already surfaced as gear.active_loan, and double-listing
// rendered the borrower twice in "Who's using it" (#2638).
func TestGetGearTransferContext_ActiveLoanExcludedFromPendingRequests(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and borrower
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear for loan
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Borrower expresses interest
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	expressResp, err := service.ExpressInterest(borrowerCtx, expressReq)
	if err != nil {
		t.Fatalf("Failed to express interest: %v", err)
	}
	transferID := expressResp.Msg.Transfer.Id

	// Loan is already auto-approved, no need to call SelectRecipient

	// Owner starts loan
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	startReq := connect.NewRequest(&api.StartLoanRequest{
		TransferId: transferID,
	})
	_, err = service.StartLoan(ownerCtx, startReq)
	if err != nil {
		t.Fatalf("Failed to start loan: %v", err)
	}

	// Owner calls GetGearTransferContext
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(ownerCtx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Owner has the transfer in ACTIVE state
	if resp.Msg.Context.UserTransfer == nil {
		t.Fatal("Expected owner to have UserTransfer populated for active loan")
	}
	if resp.Msg.Context.UserTransfer.State != api.TransferState_TRANSFER_STATE_ACTIVE {
		t.Errorf("Expected transfer state ACTIVE, got %v", resp.Msg.Context.UserTransfer.State)
	}

	// Verify: the ACTIVE loan is NOT in PendingRequests (it is the current
	// holder, conveyed via gear.active_loan — not a pending request).
	if len(resp.Msg.Context.PendingRequests) != 0 {
		t.Fatalf("Expected 0 pending requests (active loan is the holder, not pending), got %d",
			len(resp.Msg.Context.PendingRequests))
	}

	// Verify: Owner sees COMPLETE action for active loan
	hasCompleteAction := false
	for _, action := range resp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_COMPLETE {
			hasCompleteAction = true
			break
		}
	}
	if !hasCompleteAction {
		t.Error("Expected owner to have COMPLETE action for active loan")
	}
}

// TestGetGearTransferContext_OwnerSeesStartLoanActionForLoan verifies that after selecting
// a recipient for a loan, the owner sees the START_LOAN action (not COMPLETE).
func TestGetGearTransferContext_OwnerSeesStartLoanActionForLoan(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	// Setup: Create owner and borrower
	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create community and share gear for loan
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Borrower expresses interest
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(borrowerCtx, expressReq)
	if err != nil {
		t.Fatalf("Failed to express interest: %v", err)
	}

	// Loan is already auto-approved, no need to call SelectRecipient

	// Owner calls GetGearTransferContext
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	})

	resp, err := service.GetGearTransferContext(ownerCtx, req)
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	// Verify: Owner has the transfer in RECIPIENT_SELECTED state
	if resp.Msg.Context.UserTransfer == nil {
		t.Fatal("Expected owner to have UserTransfer populated after selecting recipient")
	}
	if resp.Msg.Context.UserTransfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected transfer state RECIPIENT_SELECTED, got %v", resp.Msg.Context.UserTransfer.State)
	}
	if resp.Msg.Context.UserTransfer.TransferType != api.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected transfer type LOAN, got %v", resp.Msg.Context.UserTransfer.TransferType)
	}

	// Verify: Owner sees START_LOAN action (not COMPLETE) for loan
	hasCompleteAction := false
	hasStartLoanAction := false
	for _, action := range resp.Msg.Context.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_COMPLETE {
			hasCompleteAction = true
		}
		if action == api.TransferAction_TRANSFER_ACTION_START_LOAN {
			hasStartLoanAction = true
		}
	}
	if !hasStartLoanAction {
		t.Error("Expected owner to have START_LOAN action for loan in RECIPIENT_SELECTED state")
	}
	if hasCompleteAction {
		t.Error("Expected owner to NOT have COMPLETE action for loan in RECIPIENT_SELECTED state (loans need to be started first)")
	}
}

// TestGetGearTransferContext_DeletedGearReturnsContext exercises the
// lenient-read path added for #1701. After the owner deletes the gear,
// a non-owner participant in a surviving Transfer must still be able to
// fetch their transfer context (driving their loan-detail / transfer
// screen) instead of receiving a 404 ERROR. Transfer rows are
// deliberately preserved as audit trail.
func TestGetGearTransferContext_DeletedGearReturnsContext(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "ctx-1701-owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "ctx-1701-borrower@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Borrower expresses interest — creates the Transfer row that the
	// post-delete GetGearTransferContext call has to honor.
	borrowerCtx := createAuthenticatedContext(borrowerID, "ctx-1701-borrower@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest: %v", err)
	}

	// Owner soft-deletes the gear directly via storage (replicates DeleteGear's
	// soft-delete cascade for the gear row without pulling in the gear service).
	ownerCtx := createAuthenticatedContext(ownerID, "ctx-1701-owner@example.com", models.Role_ROLE_USER)
	gearStored := &models.Gear{}
	if err := testStorage.GetByID(ownerCtx, gearID, gearStored); err != nil {
		t.Fatalf("load gear: %v", err)
	}
	gearStored.Deleted = &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: 1}
	if err := testStorage.Update(ownerCtx, gearStored); err != nil {
		t.Fatalf("soft-delete gear: %v", err)
	}

	// Borrower (non-owner) calls GetGearTransferContext after the delete.
	// Must return 200 with their surviving transfer visible, not 404.
	resp, err := service.GetGearTransferContext(borrowerCtx, connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetGearTransferContext on soft-deleted gear must succeed, got: %v", err)
	}
	if resp.Msg.Context == nil {
		t.Fatal("expected non-nil transfer context")
	}
	if resp.Msg.Context.UserTransfer == nil {
		t.Errorf("expected borrower's user_transfer to be surfaced even though gear is deleted")
	}
}

// TestGetGearTransferContext_BookingWindowsOnPendingRequests verifies that two
// disjoint calendar bookings by the same borrower both surface as pending
// requests carrying their booking windows, so clients can render which days
// each request covers instead of identical undated rows (#2638).
func TestGetGearTransferContext_BookingWindowsOnPendingRequests(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Two disjoint dated bookings by the same borrower (the ClaimGearDays
	// shape), inserted directly: this test is about the context view.
	windows := [][2]int64{
		{1770814800, 1770901200},
		{1771419600, 1771506000},
	}
	for _, w := range windows {
		start, end := w[0], w[1]
		transfer := &models.Transfer{
			GearId:                 gearID,
			OwnerId:                ownerID,
			RecipientId:            borrowerID,
			CommunityId:            communityID,
			TransferType:           models.TransferType_TRANSFER_TYPE_LOAN,
			State:                  models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			EstimatedPickupUnixSec: &start,
			ExpectedReturnUnixSec:  &end,
			LatestRequestUnixSec:   1770800000,
		}
		if _, err := testStorage.Insert(ownerCtx, transfer); err != nil {
			t.Fatalf("Failed to insert booking transfer: %v", err)
		}
	}

	resp, err := service.GetGearTransferContext(ownerCtx, connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("Failed to get gear transfer context: %v", err)
	}

	if len(resp.Msg.Context.PendingRequests) != 2 {
		t.Fatalf("Expected 2 pending requests (one per booking), got %d",
			len(resp.Msg.Context.PendingRequests))
	}
	seen := map[int64]bool{}
	for _, req := range resp.Msg.Context.PendingRequests {
		if req.EstimatedPickupUnixSec == nil || req.ExpectedReturnUnixSec == nil {
			t.Fatalf("Expected booking window on pending request %s, got pickup=%v return=%v",
				req.TransferId, req.EstimatedPickupUnixSec, req.ExpectedReturnUnixSec)
		}
		seen[*req.EstimatedPickupUnixSec] = true
	}
	for _, w := range windows {
		if !seen[w[0]] {
			t.Errorf("Expected a pending request with pickup %d; windows seen: %v", w[0], seen)
		}
	}
}
