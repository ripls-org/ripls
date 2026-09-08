package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestCancelTransfer_FromInterestExpressed_OnlyActorPenalized verifies that when
// a transfer is cancelled from INTEREST_EXPRESSED state, only the actor is penalized
// (objectUserId should be empty in the community event).
// This tests Example 3 from docs/workflows/loan.md.
// Note: Uses giveaway since loans are auto-approved and skip INTEREST_EXPRESSED state.
func TestCancelTransfer_FromInterestExpressed_OnlyActorPenalized(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create available gear - use giveaway since loans are auto-approved
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Borrower expresses interest
	_, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is in INTEREST_EXPRESSED state
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Fatalf("Expected INTEREST_EXPRESSED state, got %v", transfer.State)
	}

	// Owner cancels the transfer (declining the request)
	_, err = service.CancelTransfer(ownerCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Query for the CANCELLED community event
	events, err := testStorage.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query community events: %v", err)
	}

	// Find the CANCELLED event
	var cancelledEvent *models.CommunityEvent
	for _, e := range events {
		event := e.(*models.CommunityEvent)
		if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED {
			cancelledEvent = event
			break
		}
	}

	if cancelledEvent == nil {
		t.Fatal("Expected to find TRANSFER_CANCELLED community event")
	}

	// Verify the actor is the owner (who cancelled)
	if cancelledEvent.ActorId != ownerID {
		t.Errorf("Expected actor_id %s, got %s", ownerID, cancelledEvent.ActorId)
	}

	// CRITICAL: objectUserId should be empty when cancelling from INTEREST_EXPRESSED
	// because only the actor should be penalized (borrower made good-faith request)
	if cancelledEvent.ObjectUserId != "" {
		t.Errorf("Expected empty object_user_id when cancelling from INTEREST_EXPRESSED, got %s", cancelledEvent.ObjectUserId)
	}
}

// TestCancelTransfer_FromRecipientSelected_BothPartyPenalized verifies that when
// a transfer is cancelled from RECIPIENT_SELECTED state, both parties are penalized
// (objectUserId should be set to the other party in the community event).
// This tests Example 4 from docs/workflows/loan.md.
func TestCancelTransfer_FromRecipientSelected_BothPartyPenalized(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Borrower expresses interest
	_, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved and in RECIPIENT_SELECTED state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Owner cancels the transfer
	_, err = service.CancelTransfer(ownerCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Query for the CANCELLED community event
	events, err := testStorage.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query community events: %v", err)
	}

	// Find the CANCELLED event
	var cancelledEvent *models.CommunityEvent
	for _, e := range events {
		event := e.(*models.CommunityEvent)
		if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED {
			cancelledEvent = event
			break
		}
	}

	if cancelledEvent == nil {
		t.Fatal("Expected to find TRANSFER_CANCELLED community event")
	}

	// Verify the actor is the owner (who cancelled)
	if cancelledEvent.ActorId != ownerID {
		t.Errorf("Expected actor_id %s, got %s", ownerID, cancelledEvent.ActorId)
	}

	// CRITICAL: objectUserId should be set to the borrower when cancelling from RECIPIENT_SELECTED
	// because both parties had committed and should share the penalty
	if cancelledEvent.ObjectUserId != borrowerID {
		t.Errorf("Expected object_user_id %s when cancelling from RECIPIENT_SELECTED, got %s", borrowerID, cancelledEvent.ObjectUserId)
	}
}

// TestCancelTransfer_FromActive_BothPartyPenalized verifies that when
// a loan is cancelled from ACTIVE state, both parties are penalized.
func TestCancelTransfer_FromActive_BothPartyPenalized(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	borrowerID := setupTestUser(t, testStorage, "Borrower", "borrower@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, borrowerID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Borrower expresses interest
	_, err := service.ExpressInterest(borrowerCtx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Owner starts the loan
	_, err = service.StartLoan(ownerCtx, connect.NewRequest(&api.StartLoanRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Verify transfer is in ACTIVE state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		t.Fatalf("Expected ACTIVE state, got %v", transfer.State)
	}

	// Borrower cancels the loan (returning early)
	_, err = service.CancelTransfer(borrowerCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Query for the CANCELLED community event
	events, err := testStorage.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query community events: %v", err)
	}

	// Find the CANCELLED event
	var cancelledEvent *models.CommunityEvent
	for _, e := range events {
		event := e.(*models.CommunityEvent)
		if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED {
			cancelledEvent = event
			break
		}
	}

	if cancelledEvent == nil {
		t.Fatal("Expected to find TRANSFER_CANCELLED community event")
	}

	// Verify the actor is the borrower (who cancelled)
	if cancelledEvent.ActorId != borrowerID {
		t.Errorf("Expected actor_id %s, got %s", borrowerID, cancelledEvent.ActorId)
	}

	// objectUserId should be set to the owner when cancelling from ACTIVE
	// because both parties had committed
	if cancelledEvent.ObjectUserId != ownerID {
		t.Errorf("Expected object_user_id %s when cancelling from ACTIVE, got %s", ownerID, cancelledEvent.ObjectUserId)
	}
}

// TestCompleteGiveaway_MarksGearAsGivenAway verifies that completing a giveaway
// sets the gear state to GIVEN_AWAY and archives CommunityGear records.
func TestCompleteGiveaway_MarksGearAsGivenAway(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()

	// Create a transfer to complete
	transferID := "test-transfer-123"

	// Complete the giveaway
	_, err := service.completeGiveaway(ctx, gearID, transferID)
	if err != nil {
		t.Fatalf("completeGiveaway failed: %v", err)
	}

	// Verify gear state is GIVEN_AWAY
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_GIVEN_AWAY {
		t.Errorf("Expected gear state GIVEN_AWAY, got %v", gear.State)
	}

	// Verify ownership stays with original owner
	if gear.OwnerId != ownerID {
		t.Errorf("Expected gear owner to remain %s, got %s", ownerID, gear.OwnerId)
	}

	// Verify CommunityGear is archived (not deleted)
	communityGears, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear: %v", err)
	}
	if len(communityGears) != 1 {
		t.Fatalf("Expected 1 CommunityGear record, got %d", len(communityGears))
	}
	communityGear := communityGears[0].(*models.CommunityGear)
	if !communityGear.Archived {
		t.Error("Expected CommunityGear to be archived")
	}
}

// TestCompleteGiveaway_ArchivesAllCommunityGear verifies that when gear is shared
// in multiple communities, all CommunityGear records are archived.
func TestCompleteGiveaway_ArchivesAllCommunityGear(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	user1ID := setupTestUser(t, testStorage, "User1", "user1@example.com")
	user2ID := setupTestUser(t, testStorage, "User2", "user2@example.com")

	ctx := context.Background()

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	// Create two communities and share gear with both
	community1ID := setupCommunityAndShareGear(t, testStorage, ownerID, user1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	community2ID := setupCommunityAndShareGear(t, testStorage, ownerID, user2ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Create a transfer to complete
	transferID := "test-transfer-456"

	// Complete the giveaway
	_, err := service.completeGiveaway(ctx, gearID, transferID)
	if err != nil {
		t.Fatalf("completeGiveaway failed: %v", err)
	}

	// Verify both CommunityGear records are archived
	communityGears1, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": community1ID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear 1: %v", err)
	}
	if len(communityGears1) != 1 {
		t.Fatalf("Expected 1 CommunityGear record for community 1, got %d", len(communityGears1))
	}
	if !communityGears1[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear for community 1 to be archived")
	}

	communityGears2, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": community2ID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear 2: %v", err)
	}
	if len(communityGears2) != 1 {
		t.Fatalf("Expected 1 CommunityGear record for community 2, got %d", len(communityGears2))
	}
	if !communityGears2[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear for community 2 to be archived")
	}
}

// TestCancelGiveaway_ArchivesCommunityGear verifies that when a giveaway is cancelled,
// the CommunityGear record is archived so it no longer appears in the feed.
func TestCancelGiveaway_ArchivesCommunityGear(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear for giveaway
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Recipient expresses interest in giveaway
	_, err := service.ExpressInterest(recipientCtx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is in INTEREST_EXPRESSED state (giveaways don't auto-approve)
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Fatalf("Expected INTEREST_EXPRESSED state, got %v", transfer.State)
	}

	// Verify CommunityGear is NOT archived before cancellation
	communityGearsBefore, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear: %v", err)
	}
	if len(communityGearsBefore) != 1 {
		t.Fatalf("Expected 1 CommunityGear record, got %d", len(communityGearsBefore))
	}
	if communityGearsBefore[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear to NOT be archived before cancellation")
	}

	// Owner cancels the giveaway
	_, err = service.CancelTransfer(ownerCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Verify CommunityGear IS archived after cancellation
	communityGearsAfter, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear: %v", err)
	}
	if len(communityGearsAfter) != 1 {
		t.Fatalf("Expected 1 CommunityGear record, got %d", len(communityGearsAfter))
	}
	if !communityGearsAfter[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear to be archived after giveaway cancellation")
	}

	// Verify gear state is still AVAILABLE (not GIVEN_AWAY since giveaway was cancelled)
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE after cancellation, got %v", gear.State)
	}
}

// TestCancelGiveaway_FromRecipientSelected_RecipientActor_DoesNotArchiveCommunityGear
// verifies that when the selected recipient cancels a giveaway, the CommunityGear
// row is NOT archived and sibling transfers remain INTEREST_EXPRESSED so the owner
// can select a different recipient.
func TestCancelGiveaway_FromRecipientSelected_RecipientActor_DoesNotArchiveCommunityGear(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientBID := setupTestUser(t, testStorage, "RecipientB", "recipientB@example.com")
	recipientCID := setupTestUser(t, testStorage, "RecipientC", "recipientC@example.com")

	// Create available gear for giveaway; community includes B and C.
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientBID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	_, err := testStorage.Insert(ctx, &models.CommunityUser{
		CommunityId: communityID,
		UserId:      recipientCID,
		InviterId:   ownerID,
	})
	if err != nil {
		t.Fatalf("Failed to add recipientC membership: %v", err)
	}

	recipientBCtx := createAuthenticatedContext(recipientBID, "recipientB@example.com", models.Role_ROLE_USER)
	recipientCCtx := createAuthenticatedContext(recipientCID, "recipientC@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// B and C both express interest.
	_, err = service.ExpressInterest(recipientBCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest for B failed: %v", err)
	}
	services.WaitForNotification(t, done)

	_, err = service.ExpressInterest(recipientCCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest for C failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Locate B's transfer.
	transfers, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"recipient_id": recipientBID,
	}, &models.Transfer{})
	if err != nil || len(transfers) == 0 {
		t.Fatalf("Failed to find B's transfer: %v", err)
	}
	transferB := transfers[0].(*models.Transfer)

	// Locate C's transfer.
	transfersC, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"recipient_id": recipientCID,
	}, &models.Transfer{})
	if err != nil || len(transfersC) == 0 {
		t.Fatalf("Failed to find C's transfer: %v", err)
	}
	transferC := transfersC[0].(*models.Transfer)

	// Owner selects B.
	_, err = service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferB.Id,
		RecipientId: recipientBID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// B cancels (recipient withdraws after being selected).
	_, err = service.CancelTransfer(recipientBCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transferB.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer by recipient B failed: %v", err)
	}

	// (a) B's transfer is CANCELLED.
	if err := testStorage.GetByID(ctx, transferB.Id, transferB); err != nil {
		t.Fatalf("Failed to reload B's transfer: %v", err)
	}
	if transferB.State != models.TransferState_TRANSFER_STATE_CANCELLED {
		t.Errorf("Expected B's transfer to be CANCELLED, got %v", transferB.State)
	}

	// (b) C's transfer remains INTEREST_EXPRESSED.
	if err := testStorage.GetByID(ctx, transferC.Id, transferC); err != nil {
		t.Fatalf("Failed to reload C's transfer: %v", err)
	}
	if transferC.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected C's transfer to remain INTEREST_EXPRESSED, got %v", transferC.State)
	}

	// (c) CommunityGear is NOT archived.
	communityGears, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear: %v", err)
	}
	if len(communityGears) != 1 {
		t.Fatalf("Expected 1 CommunityGear record, got %d", len(communityGears))
	}
	if communityGears[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear to NOT be archived when recipient cancels")
	}

	// (d) Gear state is still AVAILABLE.
	gear := &models.Gear{}
	if err := testStorage.GetByID(ctx, gearID, gear); err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE, got %v", gear.State)
	}
}

// TestCancelGiveaway_FromRecipientSelected_RecipientActor_PhaseReverts verifies that
// after the selected recipient cancels, the overall giveaway phase reverts to OPEN
// and User C still has available actions.
func TestCancelGiveaway_FromRecipientSelected_RecipientActor_PhaseReverts(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientBID := setupTestUser(t, testStorage, "RecipientB", "recipientB@example.com")
	recipientCID := setupTestUser(t, testStorage, "RecipientC", "recipientC@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientBID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	_, err := testStorage.Insert(ctx, &models.CommunityUser{
		CommunityId: communityID,
		UserId:      recipientCID,
		InviterId:   ownerID,
	})
	if err != nil {
		t.Fatalf("Failed to add recipientC membership: %v", err)
	}

	recipientBCtx := createAuthenticatedContext(recipientBID, "recipientB@example.com", models.Role_ROLE_USER)
	recipientCCtx := createAuthenticatedContext(recipientCID, "recipientC@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	_, err = service.ExpressInterest(recipientBCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest for B failed: %v", err)
	}
	services.WaitForNotification(t, done)

	_, err = service.ExpressInterest(recipientCCtx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("ExpressInterest for C failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"recipient_id": recipientBID,
	}, &models.Transfer{})
	if err != nil || len(transfers) == 0 {
		t.Fatalf("Failed to find B's transfer: %v", err)
	}
	transferB := transfers[0].(*models.Transfer)

	_, err = service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferB.Id,
		RecipientId: recipientBID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	_, err = service.CancelTransfer(recipientBCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transferB.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer by recipient B failed: %v", err)
	}

	// Fetch gear transfer context for C — overall_phase must be OPEN and available_actions non-empty.
	ctxResp, err := service.GetGearTransferContext(recipientCCtx, connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetGearTransferContext for C failed: %v", err)
	}
	tc := ctxResp.Msg.Context
	if tc.OverallPhase != api.GiveawayPhase_GIVEAWAY_PHASE_OPEN {
		t.Errorf("Expected overall_phase OPEN for C after recipient B cancels, got %v", tc.OverallPhase)
	}
	if len(tc.AvailableActions) == 0 {
		t.Error("Expected C to have available actions after recipient B cancels")
	}

	// Fetch gear transfer context for owner — owner should be able to select a new recipient.
	ownerCtxResp, err := service.GetGearTransferContext(ownerCtx, connect.NewRequest(&api.GetGearTransferContextRequest{
		GearId:      gearID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetGearTransferContext for owner failed: %v", err)
	}
	ownerTC := ownerCtxResp.Msg.Context
	if ownerTC.OverallPhase != api.GiveawayPhase_GIVEAWAY_PHASE_OPEN {
		t.Errorf("Expected overall_phase OPEN for owner after recipient B cancels, got %v", ownerTC.OverallPhase)
	}
	hasSelectAction := false
	for _, action := range ownerTC.AvailableActions {
		if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
			hasSelectAction = true
			break
		}
	}
	if !hasSelectAction {
		t.Errorf("Expected owner to have SELECT_RECIPIENT action after recipient B cancels, got %v", ownerTC.AvailableActions)
	}
}

// TestCancelGiveaway_FromRecipientSelected_ArchivesCommunityGear verifies that
// cancelling a giveaway from RECIPIENT_SELECTED state also archives the CommunityGear.
func TestCancelGiveaway_FromRecipientSelected_ArchivesCommunityGear(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear for giveaway
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Recipient expresses interest in giveaway
	_, err := service.ExpressInterest(recipientCtx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Owner selects recipient
	_, err = service.SelectRecipient(ownerCtx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transfer.Id,
		RecipientId: recipientID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Verify transfer is in RECIPIENT_SELECTED state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Owner cancels the giveaway
	_, err = service.CancelTransfer(ownerCtx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transfer.Id,
	}))
	if err != nil {
		t.Fatalf("CancelTransfer failed: %v", err)
	}

	// Verify CommunityGear IS archived after cancellation
	communityGearsAfter, err := testStorage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		t.Fatalf("Failed to query community gear: %v", err)
	}
	if len(communityGearsAfter) != 1 {
		t.Fatalf("Expected 1 CommunityGear record, got %d", len(communityGearsAfter))
	}
	if !communityGearsAfter[0].(*models.CommunityGear).Archived {
		t.Error("Expected CommunityGear to be archived after giveaway cancellation from RECIPIENT_SELECTED")
	}
}
