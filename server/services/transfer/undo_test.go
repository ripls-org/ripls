package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// TestUndoSelectRecipient_Success verifies the happy path: an owner
// selects a recipient, then undoes the selection, and the transfer
// reverts to INTEREST_EXPRESSED, the approval chat message is
// soft-deleted, and a retraction CommunityEvent is emitted.
func TestUndoSelectRecipient_Success(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	// Recipient expresses interest.
	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("query transfers: %v", err)
	}
	transferID := transfers[0].(*models.Transfer).Id

	// Owner selects recipient.
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectResp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	communityEventID := selectResp.Msg.CommunityEventId
	if communityEventID == "" {
		t.Fatal("SelectRecipientResponse missing community_event_id — required for undo wiring")
	}

	// Sanity: transfer is in RECIPIENT_SELECTED.
	transfer := &models.Transfer{}
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("load transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("pre-undo: expected RECIPIENT_SELECTED, got %s", transfer.State)
	}

	// Locate the approval chat message before undo — it should be
	// soft-deleted after.
	approvalID := originalApprovalMessageID(t, testStorage, communityEventID)

	// Owner undoes the selection.
	_, err = service.UndoSelectRecipient(ownerCtx, connect.NewRequest(&api.UndoSelectRecipientRequest{
		CommunityEventId: communityEventID,
	}))
	if err != nil {
		t.Fatalf("UndoSelectRecipient failed: %v", err)
	}

	// Transfer should be back in INTEREST_EXPRESSED with no recipient.
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("post-undo load transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("post-undo state: want INTEREST_EXPRESSED, got %s", transfer.State)
	}
	// recipient_id is NOT cleared: ExpressInterest writes it at transfer
	// creation, so the pre-action value equals the post-selection value.
	// The round-trip test verifies this invariant more strictly.
	if transfer.RecipientId != recipientID {
		t.Errorf("post-undo recipient_id: want %q (unchanged from pre-action), got %q", recipientID, transfer.RecipientId)
	}

	// Approval chat message should be soft-deleted — the storage layer's
	// deleted filter makes GetByID refuse to return it.
	approvalAfter := &models.ChatMessage{}
	if err := testStorage.GetByID(ctx, approvalID, approvalAfter); err == nil {
		t.Errorf("expected approval chat message to be filtered as soft-deleted; GetByID returned it")
	}

	// Retraction CommunityEvent of type TRANSFER_RECIPIENT_SELECTED_UNDONE
	// should exist for this transfer.
	events, err := testStorage.QueryByField(ctx, "transfer_id", transferID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var foundRetraction bool
	for _, e := range events {
		if e.(*models.CommunityEvent).EventType ==
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE {
			foundRetraction = true
			break
		}
	}
	if !foundRetraction {
		t.Error("expected TRANSFER_RECIPIENT_SELECTED_UNDONE retraction event; none found")
	}
}

// TestUndoSelectRecipient_NotActor verifies that only the actor can
// undo their own action. A non-owner attempting to undo receives
// FailedPrecondition with UNDO_FAILURE_REASON_NOT_ACTOR.
func TestUndoSelectRecipient_NotActor(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectResp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Recipient (not the actor) tries to undo — should fail.
	_, err = service.UndoSelectRecipient(recipCtx, connect.NewRequest(&api.UndoSelectRecipientRequest{
		CommunityEventId: selectResp.Msg.CommunityEventId,
	}))
	if err == nil {
		t.Fatal("expected error from non-actor undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if cerr.Code() != connect.CodeFailedPrecondition {
		t.Errorf("code: want FailedPrecondition, got %s", cerr.Code())
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR {
		t.Errorf("reason: want NOT_ACTOR, got %s", reason)
	}
}

// TestUndoSelectRecipient_AlreadyUndone verifies the second undo call
// returns UNDO_FAILURE_REASON_ALREADY_UNDONE.
func TestUndoSelectRecipient_AlreadyUndone(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectResp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	undoReq := connect.NewRequest(&api.UndoSelectRecipientRequest{
		CommunityEventId: selectResp.Msg.CommunityEventId,
	})

	if _, err := service.UndoSelectRecipient(ownerCtx, undoReq); err != nil {
		t.Fatalf("first undo failed: %v", err)
	}

	_, err = service.UndoSelectRecipient(ownerCtx, undoReq)
	if err == nil {
		t.Fatal("expected error from second undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE {
		t.Errorf("reason: want ALREADY_UNDONE, got %s", reason)
	}
}

// TestUndoSelectRecipient_EntityChanged verifies the drift check: if
// the transfer is no longer in RECIPIENT_SELECTED (e.g., the giveaway
// was completed after selection), undo rejects with ENTITY_CHANGED.
func TestUndoSelectRecipient_EntityChanged(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	// Use a giveaway so SelectRecipient is required (loans auto-approve
	// interest when the owner is the expresser, which would skip the
	// explicit SelectRecipient step we want to drift out of).
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectResp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Complete the giveaway — transfer drifts to COMPLETED, so undo of
	// the earlier selection should reject with ENTITY_CHANGED.
	if _, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transferID,
	})); err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	_, err = service.UndoSelectRecipient(ownerCtx, connect.NewRequest(&api.UndoSelectRecipientRequest{
		CommunityEventId: selectResp.Msg.CommunityEventId,
	}))
	if err == nil {
		t.Fatal("expected error from undo after drift, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED {
		t.Errorf("reason: want ENTITY_CHANGED, got %s", reason)
	}
}

// TestUndoCompleteLoan_Success verifies the happy path: owner approves
// a loan, borrower starts it, owner marks it returned, then owner
// undoes the completion. Transfer reverts to ACTIVE, gear back to
// UNAVAILABLE, completion chat message soft-deleted, retraction event
// emitted.
func TestUndoCompleteLoan_Success(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()

	// Borrower expresses interest — auto-approved on loans, lands in RECIPIENT_SELECTED.
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id

	// Owner starts the loan.
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{TransferId: transferID})); err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Owner completes the loan.
	completeResp, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{TransferId: transferID}))
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	communityEventID := completeResp.Msg.CommunityEventId
	if communityEventID == "" {
		t.Fatal("CompleteTransferResponse missing community_event_id")
	}

	// Sanity: transfer is COMPLETED, gear is AVAILABLE again.
	transfer := &models.Transfer{}
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("load transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Fatalf("pre-undo state: want COMPLETED, got %s", transfer.State)
	}
	gear := &models.Gear{}
	if err := testStorage.GetByID(ctx, gearID, gear); err != nil {
		t.Fatalf("load gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Fatalf("pre-undo gear: want AVAILABLE, got %s", gear.State)
	}

	completionMessageID := originalApprovalMessageID(t, testStorage, communityEventID)

	// Owner undoes the completion.
	if _, err := service.UndoCompleteLoan(ownerCtx, connect.NewRequest(&api.UndoCompleteLoanRequest{
		CommunityEventId: communityEventID,
	})); err != nil {
		t.Fatalf("UndoCompleteLoan failed: %v", err)
	}

	// Transfer should be back to ACTIVE, gear back to UNAVAILABLE.
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("post-undo load transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		t.Errorf("post-undo state: want ACTIVE, got %s", transfer.State)
	}
	if transfer.ActualReturnUnixSec != nil {
		t.Errorf("post-undo ActualReturnUnixSec: want nil, got %v", *transfer.ActualReturnUnixSec)
	}
	if err := testStorage.GetByID(ctx, gearID, gear); err != nil {
		t.Fatalf("post-undo load gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_UNAVAILABLE {
		t.Errorf("post-undo gear: want UNAVAILABLE, got %s", gear.State)
	}

	// Completion chat message should be soft-deleted.
	completionAfter := &models.ChatMessage{}
	if err := testStorage.GetByID(ctx, completionMessageID, completionAfter); err == nil {
		t.Errorf("expected completion chat message to be filtered as soft-deleted")
	}

	// Retraction event exists.
	events, err := testStorage.QueryByField(ctx, "transfer_id", transferID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var foundRetraction bool
	for _, e := range events {
		if e.(*models.CommunityEvent).EventType ==
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE {
			foundRetraction = true
			break
		}
	}
	if !foundRetraction {
		t.Error("expected TRANSFER_COMPLETED_UNDONE retraction event; none found")
	}
}

// TestUndoCompleteLoan_WrongEventType verifies the RPC rejects events
// of the wrong type (e.g., a SelectRecipient event) with WRONG_EVENT_TYPE.
func TestUndoCompleteLoan_WrongEventType(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	transferID := transfers[0].(*models.Transfer).Id

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectResp, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Pass the SelectRecipient event id to UndoCompleteLoan — wrong type.
	_, err = service.UndoCompleteLoan(ownerCtx, connect.NewRequest(&api.UndoCompleteLoanRequest{
		CommunityEventId: selectResp.Msg.CommunityEventId,
	}))
	if err == nil {
		t.Fatal("expected error from wrong-event-type undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE {
		t.Errorf("reason: want WRONG_EVENT_TYPE, got %s", reason)
	}
}

// TestUndoCompleteGiveaway_Success verifies the happy path on the
// full giveaway cascade: gear restored to AVAILABLE, sibling transfer
// revived to INTEREST_EXPRESSED, community_gear un-archived.
func TestUndoCompleteGiveaway_Success(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	siblingID := setupTestUser(t, testStorage, "Sibling", "sibling@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	// Sibling also joins the community + expresses interest — cancels on completion.
	if _, err := testStorage.Insert(ctx, &models.CommunityUser{
		CommunityId: communityID, UserId: siblingID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("add sibling: %v", err)
	}
	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest recipient: %v", err)
	}
	services.WaitForNotification(t, done)
	siblingCtx := createAuthenticatedContext(siblingID, "sibling@example.com", models.Role_ROLE_USER)
	if _, err := service.ExpressInterest(siblingCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})); err != nil {
		t.Fatalf("ExpressInterest sibling: %v", err)
	}
	services.WaitForNotification(t, done)

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	recipientTransfers, _ := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID, "recipient_id": recipientID,
	}, &models.Transfer{})
	primaryID := recipientTransfers[0].(*models.Transfer).Id
	if _, err := service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId: primaryID, RecipientId: recipientID,
	})); err != nil {
		t.Fatalf("SelectRecipient: %v", err)
	}
	services.WaitForNotification(t, done)

	completeResp, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: primaryID,
	}))
	if err != nil {
		t.Fatalf("CompleteTransfer: %v", err)
	}
	communityEventID := completeResp.Msg.CommunityEventId
	if communityEventID == "" {
		t.Fatal("CompleteTransferResponse missing community_event_id")
	}

	// Sanity: gear is GIVEN_AWAY, sibling is CANCELLED.
	gear := &models.Gear{}
	_ = testStorage.GetByID(ctx, gearID, gear)
	if gear.State != models.GearState_GEAR_STATE_GIVEN_AWAY {
		t.Fatalf("pre-undo gear: want GIVEN_AWAY, got %s", gear.State)
	}

	siblingTransfers, _ := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID, "recipient_id": siblingID,
	}, &models.Transfer{})
	if len(siblingTransfers) != 1 {
		t.Fatalf("expected 1 sibling transfer, got %d", len(siblingTransfers))
	}
	siblingTransferID := siblingTransfers[0].(*models.Transfer).Id
	siblingPre := &models.Transfer{}
	_ = testStorage.GetByID(ctx, siblingTransferID, siblingPre)
	if siblingPre.State != models.TransferState_TRANSFER_STATE_CANCELLED {
		t.Fatalf("pre-undo sibling: want CANCELLED, got %s", siblingPre.State)
	}

	// Undo.
	if _, err := service.UndoCompleteGiveaway(ownerCtx, connect.NewRequest(&api.UndoCompleteGiveawayRequest{
		CommunityEventId: communityEventID,
	})); err != nil {
		t.Fatalf("UndoCompleteGiveaway: %v", err)
	}

	// Gear back to AVAILABLE.
	_ = testStorage.GetByID(ctx, gearID, gear)
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("post-undo gear: want AVAILABLE, got %s", gear.State)
	}

	// Primary transfer back to RECIPIENT_SELECTED.
	primary := &models.Transfer{}
	_ = testStorage.GetByID(ctx, primaryID, primary)
	if primary.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("post-undo primary: want RECIPIENT_SELECTED, got %s", primary.State)
	}

	// Sibling revived to INTEREST_EXPRESSED.
	siblingPost := &models.Transfer{}
	_ = testStorage.GetByID(ctx, siblingTransferID, siblingPost)
	if siblingPost.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("post-undo sibling: want INTEREST_EXPRESSED, got %s", siblingPost.State)
	}

	// Community_gear un-archived.
	cgs, _ := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	for _, cg := range cgs {
		m := cg.(*models.CommunityGear)
		if m.Archived {
			t.Errorf("community_gear %s still archived after undo", m.Id)
		}
	}
}

// TestUndoCompleteGiveaway_CascadeUnrecoverable verifies that if a
// sibling transfer has been mutated since completion (e.g., user
// explicitly cancelled it and the state diverged from CANCELLED),
// undo rejects rather than clobbering the later mutation.
func TestUndoCompleteGiveaway_CascadeUnrecoverable(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	siblingID := setupTestUser(t, testStorage, "Sibling", "sibling@example.com")
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	if _, err := testStorage.Insert(ctx, &models.CommunityUser{
		CommunityId: communityID, UserId: siblingID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("add sibling: %v", err)
	}
	recipCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	_, _ = service.ExpressInterest(recipCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	services.WaitForNotification(t, done)
	siblingCtx := createAuthenticatedContext(siblingID, "sibling@example.com", models.Role_ROLE_USER)
	_, _ = service.ExpressInterest(siblingCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	services.WaitForNotification(t, done)

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	rts, _ := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID, "recipient_id": recipientID,
	}, &models.Transfer{})
	primaryID := rts[0].(*models.Transfer).Id
	_, _ = service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId: primaryID, RecipientId: recipientID,
	}))
	services.WaitForNotification(t, done)

	completeResp, err := service.CompleteTransfer(ownerCtx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: primaryID,
	}))
	if err != nil {
		t.Fatalf("CompleteTransfer: %v", err)
	}

	// Manually mutate the cascaded sibling transfer AWAY from CANCELLED
	// — simulating a subsequent action. Undo should now reject.
	sts, _ := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID, "recipient_id": siblingID,
	}, &models.Transfer{})
	siblingTransfer := sts[0].(*models.Transfer)
	siblingTransfer.State = models.TransferState_TRANSFER_STATE_COMPLETED
	if err := testStorage.Update(ctx, siblingTransfer); err != nil {
		t.Fatalf("manual sibling mutation: %v", err)
	}

	_, err = service.UndoCompleteGiveaway(ownerCtx, connect.NewRequest(&api.UndoCompleteGiveawayRequest{
		CommunityEventId: completeResp.Msg.CommunityEventId,
	}))
	if err == nil {
		t.Fatal("expected CASCADE_UNRECOVERABLE, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_CASCADE_UNRECOVERABLE {
		t.Errorf("reason: want CASCADE_UNRECOVERABLE, got %s", reason)
	}
}

// extractUndoFailureReason unpacks UndoErrorDetail from a Connect
// error's details and returns its Reason. Returns UNSPECIFIED when
// no UndoErrorDetail is attached.
func extractUndoFailureReason(cerr *connect.Error) api.UndoFailureReason {
	for _, d := range cerr.Details() {
		val, err := d.Value()
		if err != nil {
			continue
		}
		if detail, ok := val.(*api.UndoErrorDetail); ok {
			return detail.Reason
		}
	}
	return api.UndoFailureReason_UNDO_FAILURE_REASON_UNSPECIFIED
}

// originalApprovalMessageID returns the id of the APPROVED chat
// message recorded when SelectRecipient was called. Looks it up via
// the SystemChatMessageId field on the CommunityEvent — exactly the
// path UndoSelectRecipient uses internally — so a regression in
// emission wiring shows up as a test failure here.
func originalApprovalMessageID(t *testing.T, testStorage *storage.ProtoSQLStorage, communityEventID string) string {
	t.Helper()
	ev := &models.CommunityEvent{}
	if err := testStorage.GetByID(context.Background(), communityEventID, ev); err != nil {
		t.Fatalf("load community event: %v", err)
	}
	id := ev.GetSystemChatMessageId()
	if id == "" {
		t.Fatalf("community event has no system_chat_message_id — SelectRecipient did not thread the message id")
	}
	return id
}
