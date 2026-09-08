package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_SelectRecipient(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipient1ID := setupTestUser(t, testStorage, "Recipient1", "recipient1@example.com")
	recipient2ID := setupTestUser(t, testStorage, "Recipient2", "recipient2@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipient1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add recipient2 to community
	ctx := context.Background()
	recipient2Membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      recipient2ID,
		InviterId:   ownerID,
	}
	_, err := testStorage.Insert(ctx, recipient2Membership)
	if err != nil {
		t.Fatalf("Failed to add recipient2 membership: %v", err)
	}

	// Recipients express interest
	ctx1 := createAuthenticatedContext(recipient1ID, "recipient1@example.com", models.Role_ROLE_USER)
	req1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(ctx1, req1)
	if err != nil {
		t.Fatalf("First ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done) // Wait for notification

	// Get transfer ID
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Owner selects recipient
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	selectReq := connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transfer.Id,
		RecipientId: recipient1ID,
	})

	selectResp, err := service.SelectRecipient(ownerCtx, selectReq)
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}
	services.WaitForNotification(t, done) // Wait for notification

	if selectResp.Msg.ConversationId == "" {
		t.Error("Expected non-empty conversation ID")
	}

	// Verify transfer state updated
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get updated transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	if transfer.RecipientId != recipient1ID {
		t.Errorf("Expected recipient_id %s, got %s", recipient1ID, transfer.RecipientId)
	}

	// Verify new 1:1 conversation was created
	newConversation := &models.ChatConversation{}
	err = testStorage.GetByID(ctx, selectResp.Msg.ConversationId, newConversation)
	if err != nil {
		t.Errorf("Expected new 1:1 conversation to be created: %v", err)
	}

	// Verify RECIPIENT_SELECTED community event was created
	// Note: ObjectUserId is intentionally empty for RECIPIENT_SELECTED events because
	// the notification system looks up the recipient from the transfer record instead.
	events, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query community events: %v", err)
	}
	var recipientSelectedEvent *models.CommunityEvent
	for _, e := range events {
		event := e.(*models.CommunityEvent)
		if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED {
			recipientSelectedEvent = event
			break
		}
	}
	if recipientSelectedEvent == nil {
		t.Error("Expected RECIPIENT_SELECTED community event to be created")
	} else if recipientSelectedEvent.ObjectUserId != "" {
		t.Errorf("Expected ObjectUserId to be empty for RECIPIENT_SELECTED, got %s",
			recipientSelectedEvent.ObjectUserId)
	}
}
