package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_ExpressInterest_Loan(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}

	// Verify enriched response contains full transfer details
	if resp.Msg.Transfer == nil {
		t.Fatal("Expected transfer in response")
	}

	transfer := resp.Msg.Transfer
	if transfer.GetConversationId() == "" {
		t.Error("Expected non-empty conversation ID")
	}
	if transfer.GearId != gearID {
		t.Errorf("Expected gear ID %s, got %s", gearID, transfer.GearId)
	}
	if transfer.GearName != "Test Drill" {
		t.Errorf("Expected gear name 'Test Drill', got %s", transfer.GearName)
	}
	if transfer.Owner == nil {
		t.Error("Expected owner to be populated")
	} else if transfer.Owner.Id != ownerID {
		t.Errorf("Expected owner ID %s, got %s", ownerID, transfer.Owner.Id)
	}
	if transfer.Recipient == nil {
		t.Error("Expected recipient to be populated")
	} else if transfer.Recipient.Id != recipientID {
		t.Errorf("Expected recipient ID %s, got %s", recipientID, transfer.Recipient.Id)
	}
	if transfer.TransferType != api.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected transfer type LOAN, got %v", transfer.TransferType)
	}
	// Loans auto-transition to RECIPIENT_SELECTED
	if transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected state RECIPIENT_SELECTED (auto-approved), got %v", transfer.State)
	}

	// Verify transfer was created with correct type and state in database
	transfersStored, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}

	if len(transfersStored) != 1 {
		t.Fatalf("Expected 1 transfer, got %d", len(transfersStored))
	}

	transferStored := transfersStored[0].(*models.Transfer)
	if transferStored.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected LOAN type, got %v", transferStored.TransferType)
	}

	// Verify loans are auto-approved (start in RECIPIENT_SELECTED state)
	if transferStored.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected RECIPIENT_SELECTED state (auto-approved for loans), got %v", transferStored.State)
	}

	// Verify chat was created
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(ctx, resp.Msg.Transfer.GetConversationId(), conversation)
	if err != nil {
		t.Errorf("Expected conversation to be created: %v", err)
	}

	// Verify gear state is still available
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear: %v", err)
	}

	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("Expected gear to remain AVAILABLE, got %v", gear.State)
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

func TestService_ExpressInterest_Giveaway(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}

	// Verify enriched response contains full transfer details
	if resp.Msg.Transfer == nil {
		t.Fatal("Expected transfer in response")
	}

	transfer := resp.Msg.Transfer
	if transfer.GetConversationId() == "" {
		t.Error("Expected non-empty conversation ID")
	}
	if transfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("Expected transfer type GIVEAWAY, got %v", transfer.TransferType)
	}
	if transfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected state INTEREST_EXPRESSED, got %v", transfer.State)
	}
	if transfer.Owner == nil || transfer.Owner.Id != ownerID {
		t.Error("Expected owner to be populated correctly")
	}
	if transfer.Recipient == nil || transfer.Recipient.Id != recipientID {
		t.Error("Expected recipient to be populated correctly")
	}

	// Verify transfer was created with correct type in database
	transfersStored, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}

	transferStored := transfersStored[0].(*models.Transfer)
	if transferStored.TransferType != models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("Expected GIVEAWAY type, got %v", transferStored.TransferType)
	}

	// Verify group chat was created with owner
	chatConversation := &models.ChatConversation{}
	err = testStorage.GetByID(ctx, resp.Msg.Transfer.GetConversationId(), chatConversation)
	if err != nil {
		t.Fatalf("Expected conversation to be created: %v", err)
	}

	// Verify participants include both owner and first interested user
	if len(chatConversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(chatConversation.ParticipantIds))
	}

	expectedParticipants := map[string]bool{ownerID: true, recipientID: true}
	for _, participantID := range chatConversation.ParticipantIds {
		if !expectedParticipants[participantID] {
			t.Errorf("Unexpected participant %s in conversation", participantID)
		}
	}
}

func TestService_ExpressInterest_ExistingGiveaway_CreatesMultipleTransfers(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

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

	// First user expresses interest
	ctx1 := createAuthenticatedContext(recipient1ID, "recipient1@example.com", models.Role_ROLE_USER)
	req1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp1, err := service.ExpressInterest(ctx1, req1)
	if err != nil {
		t.Fatalf("First ExpressInterest failed: %v", err)
	}

	// Verify first response
	if resp1.Msg.Transfer == nil {
		t.Fatal("Expected transfer in first response")
	}
	if resp1.Msg.Transfer.GetConversationId() == "" {
		t.Error("Expected non-empty conversation ID in first response")
	}

	// Get the conversation using the ID returned from ExpressInterest
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(ctx, resp1.Msg.Transfer.GetConversationId(), conversation)
	if err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}

	initialParticipantCount := len(conversation.ParticipantIds)

	// Second user expresses interest
	ctx2 := createAuthenticatedContext(recipient2ID, "recipient2@example.com", models.Role_ROLE_USER)
	req2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp2, err := service.ExpressInterest(ctx2, req2)
	if err != nil {
		t.Fatalf("Second ExpressInterest failed: %v", err)
	}

	// Verify same conversation ID returned
	if resp2.Msg.Transfer.GetConversationId() != resp1.Msg.Transfer.GetConversationId() {
		t.Error("Expected same conversation ID for second user")
	}

	// Verify second user was added to conversation
	err = testStorage.GetByID(ctx, conversation.Id, conversation)
	if err != nil {
		t.Fatalf("Failed to get updated conversation: %v", err)
	}

	if len(conversation.ParticipantIds) != initialParticipantCount+1 {
		t.Errorf("Expected %d participants, got %d", initialParticipantCount+1, len(conversation.ParticipantIds))
	}

	// Verify second user is in participants
	foundRecipient2 := false
	for _, participantID := range conversation.ParticipantIds {
		if participantID == recipient2ID {
			foundRecipient2 = true
			break
		}
	}
	if !foundRecipient2 {
		t.Error("Expected recipient2 to be added to conversation participants")
	}

	// Verify two transfers were created (one per user)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}

	if len(transfers) != 2 {
		t.Errorf("Expected 2 transfers, got %d", len(transfers))
	}
}

func TestService_ExpressInterest_ExistingLoan_CreatesMultipleTransfers(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipient1ID := setupTestUser(t, testStorage, "Recipient1", "recipient1@example.com")
	recipient2ID := setupTestUser(t, testStorage, "Recipient2", "recipient2@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipient1ID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

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

	// First user expresses interest
	ctx1 := createAuthenticatedContext(recipient1ID, "recipient1@example.com", models.Role_ROLE_USER)
	req1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp1, err := service.ExpressInterest(ctx1, req1)
	if err != nil {
		t.Fatalf("First ExpressInterest failed: %v", err)
	}

	// Second user expresses interest - should succeed and create a separate transfer
	ctx2 := createAuthenticatedContext(recipient2ID, "recipient2@example.com", models.Role_ROLE_USER)
	req2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})

	resp2, err := service.ExpressInterest(ctx2, req2)
	if err != nil {
		t.Fatalf("Second ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done) // Wait for second notification

	// Verify two separate transfers were created
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}

	if len(transfers) != 2 {
		t.Fatalf("Expected 2 transfers, got %d", len(transfers))
	}

	// Verify both transfers are for loans in RECIPIENT_SELECTED state (auto-approved)
	transfer1 := transfers[0].(*models.Transfer)
	transfer2 := transfers[1].(*models.Transfer)

	if transfer1.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected transfer 1 to be LOAN type, got %v", transfer1.TransferType)
	}
	if transfer2.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("Expected transfer 2 to be LOAN type, got %v", transfer2.TransferType)
	}

	// Loans auto-transition to RECIPIENT_SELECTED
	if transfer1.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected transfer 1 state RECIPIENT_SELECTED (auto-approved), got %v", transfer1.State)
	}
	if transfer2.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("Expected transfer 2 state RECIPIENT_SELECTED (auto-approved), got %v", transfer2.State)
	}

	// Verify different recipients
	recipients := map[string]bool{transfer1.RecipientId: true, transfer2.RecipientId: true}
	if !recipients[recipient1ID] {
		t.Error("Expected recipient1 to have a transfer")
	}
	if !recipients[recipient2ID] {
		t.Error("Expected recipient2 to have a transfer")
	}

	// Verify SAME conversation for both loans (new gear conversation model)
	if resp1.Msg.Transfer.GetConversationId() != resp2.Msg.Transfer.GetConversationId() {
		t.Error("Expected same conversation ID for loan requests (gear conversation is shared)")
	}

	// Verify conversation is gear-based and has all participants (owner + both borrowers)
	conv := &models.ChatConversation{}
	err = testStorage.GetByID(ctx, resp1.Msg.Transfer.GetConversationId(), conv)
	if err != nil {
		t.Fatalf("Failed to get gear conversation: %v", err)
	}

	// Check it's a gear conversation (not transfer-based)
	if conv.GetTopic().GetGearId() == "" {
		t.Error("Expected gear-based conversation")
	}
	if conv.GetTopic().GetGearId() != gearID {
		t.Errorf("Expected conversation for gear %s, got %s", gearID, conv.GetTopic().GetGearId())
	}

	// Verify all participants are present: owner + 2 borrowers = 3 total
	if len(conv.ParticipantIds) != 3 {
		t.Errorf("Expected 3 participants in gear conversation (owner + 2 borrowers), got %d", len(conv.ParticipantIds))
	}

	// Verify owner and both borrowers are in the conversation
	participantMap := make(map[string]bool)
	for _, pid := range conv.ParticipantIds {
		participantMap[pid] = true
	}
	if !participantMap[ownerID] {
		t.Error("Expected owner to be in conversation")
	}
	if !participantMap[recipient1ID] {
		t.Error("Expected recipient1 to be in conversation")
	}
	if !participantMap[recipient2ID] {
		t.Error("Expected recipient2 to be in conversation")
	}
}

func TestWithdrawInterest(t *testing.T) {
	tests := []struct {
		name              string
		transferType      models.TransferType
		state             models.TransferState
		callerIsOwner     bool
		callerIsRecipient bool
		wantErr           bool
		wantErrCode       connect.Code
	}{
		{
			name:              "loan_success",
			transferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			state:             models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			callerIsRecipient: true,
			wantErr:           false,
		},
		{
			name:              "loan_not_recipient",
			transferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			state:             models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			callerIsRecipient: false,
			wantErr:           true,
			wantErrCode:       connect.CodePermissionDenied,
		},
		{
			name:              "loan_withdraw_from_recipient_selected_success",
			transferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			state:             models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			callerIsRecipient: true,
			wantErr:           false, // Changed: loans now allow withdrawal from RECIPIENT_SELECTED
		},
		{
			name:              "cannot_withdraw_from_completed",
			transferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			state:             models.TransferState_TRANSFER_STATE_COMPLETED,
			callerIsRecipient: true,
			wantErr:           true,
			wantErrCode:       connect.CodeFailedPrecondition,
		},
		{
			name:              "cannot_withdraw_from_cancelled",
			transferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			state:             models.TransferState_TRANSFER_STATE_CANCELLED,
			callerIsRecipient: true,
			wantErr:           true,
			wantErrCode:       connect.CodeFailedPrecondition,
		},
		{
			name:          "owner_cannot_withdraw",
			transferType:  models.TransferType_TRANSFER_TYPE_LOAN,
			state:         models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			callerIsOwner: true,
			wantErr:       true,
			wantErrCode:   connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, testStorage, _, _ := setupTestServiceWithNotifications(t)

			ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
			recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
			otherUserID := setupTestUser(t, testStorage, "Other", "other@example.com")

			// Create a proper gear record
			gear := &models.Gear{
				Name:    "Test Gear",
				OwnerId: ownerID,
				State:   models.GearState_GEAR_STATE_AVAILABLE,
			}
			gearID, err := testStorage.Insert(context.Background(), gear)
			if err != nil {
				t.Fatalf("Failed to create gear: %v", err)
			}

			// Create a community
			community := &models.Community{
				Name:        "Test Community",
				CreatorId:   ownerID,
				OwnerUserId: ownerID,
			}
			communityID, err := testStorage.Insert(context.Background(), community)
			if err != nil {
				t.Fatalf("Failed to create community: %v", err)
			}

			// Create a conversation for the gear
			conversation := &models.ChatConversation{
				CommunityId:    communityID,
				Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
				ParticipantIds: []string{ownerID, recipientID},
			}
			conversationID, err := testStorage.Insert(context.Background(), conversation)
			if err != nil {
				t.Fatalf("Failed to create conversation: %v", err)
			}

			// Set conversation_id on the gear (canonical location since #1332 migration).
			gear.ConversationId = conversationID
			if err := testStorage.Update(context.Background(), gear); err != nil {
				t.Fatalf("Failed to update gear with conversation_id: %v", err)
			}

			// Create CommunityGear linked to community
			communityGear := &models.CommunityGear{
				GearId:      gearID,
				CommunityId: communityID,
			}
			_, err = testStorage.Insert(context.Background(), communityGear)
			if err != nil {
				t.Fatalf("Failed to create community gear: %v", err)
			}

			// Create transfer directly in storage
			transfer := &models.Transfer{
				GearId:       gearID,
				OwnerId:      ownerID,
				RecipientId:  recipientID,
				TransferType: tt.transferType,
				State:        tt.state,
				CommunityId:  communityID,
			}
			transferID, err := testStorage.Insert(context.Background(), transfer)
			if err != nil {
				t.Fatalf("Failed to create transfer: %v", err)
			}

			// Determine caller
			var callerID, callerEmail string
			if tt.callerIsOwner {
				callerID = ownerID
				callerEmail = "owner@example.com"
			} else if tt.callerIsRecipient {
				callerID = recipientID
				callerEmail = "recipient@example.com"
			} else {
				callerID = otherUserID
				callerEmail = "other@example.com"
			}

			ctx := createAuthenticatedContext(callerID, callerEmail, models.Role_ROLE_USER)
			req := connect.NewRequest(&api.WithdrawInterestRequest{
				TransferId: transferID,
			})

			_, err = service.WithdrawInterest(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				connectErr, ok := err.(*connect.Error)
				if !ok {
					t.Fatalf("Expected connect.Error, got %T", err)
				}
				if connectErr.Code() != tt.wantErrCode {
					t.Errorf("Expected error code %v, got %v", tt.wantErrCode, connectErr.Code())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				// Verify transfer was cancelled
				updatedTransfer := &models.Transfer{}
				err = testStorage.GetByID(context.Background(), transferID, updatedTransfer)
				if err != nil {
					t.Fatalf("Failed to get updated transfer: %v", err)
				}
				if updatedTransfer.State != models.TransferState_TRANSFER_STATE_CANCELLED {
					t.Errorf("Expected transfer to be CANCELLED, got %v", updatedTransfer.State)
				}
			}
		})
	}
}

func TestWithdrawInterest_TransferNotFound(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "User", "user@example.com")

	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.WithdrawInterestRequest{
		TransferId: "non-existent-transfer-id",
	})

	_, err := service.WithdrawInterest(ctx, req)
	if err == nil {
		t.Fatal("Expected error but got none")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected error code NotFound, got %v", connectErr.Code())
	}
}

func TestWithdrawInterest_Giveaway_Success(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Express interest to create the giveaway and conversation
	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	expressResp, err := service.ExpressInterest(ctx, expressReq)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transferID := expressResp.Msg.Transfer.Id
	conversationID := expressResp.Msg.Transfer.GetConversationId()

	// Withdraw interest
	withdrawReq := connect.NewRequest(&api.WithdrawInterestRequest{TransferId: transferID})
	_, err = service.WithdrawInterest(ctx, withdrawReq)
	if err != nil {
		t.Fatalf("WithdrawInterest failed: %v", err)
	}

	// Verify user was removed from conversation
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(context.Background(), conversationID, conversation)
	if err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}

	for _, pid := range conversation.ParticipantIds {
		if pid == recipientID {
			t.Error("Expected recipient to be removed from conversation")
		}
	}

	// Verify transfer's recipient_id was cleared (no other participants)
	transfer := &models.Transfer{}
	err = testStorage.GetByID(context.Background(), transferID, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.RecipientId != "" {
		t.Errorf("Expected recipient_id to be cleared, got %s", transfer.RecipientId)
	}

	// Verify transfer is still in INTEREST_EXPRESSED state (not archived)
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected transfer to remain in INTEREST_EXPRESSED state, got %v", transfer.State)
	}
}

func TestWithdrawInterest_Giveaway_RecipientReassignment(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipient1ID := setupTestUser(t, testStorage, "Recipient1", "recipient1@example.com")
	recipient2ID := setupTestUser(t, testStorage, "Recipient2", "recipient2@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipient1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add recipient2 to community
	recipient2Membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      recipient2ID,
		InviterId:   ownerID,
	}
	_, err := testStorage.Insert(context.Background(), recipient2Membership)
	if err != nil {
		t.Fatalf("Failed to add recipient2 membership: %v", err)
	}

	// First user expresses interest
	ctx1 := createAuthenticatedContext(recipient1ID, "recipient1@example.com", models.Role_ROLE_USER)
	expressReq1 := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	expressResp1, err := service.ExpressInterest(ctx1, expressReq1)
	if err != nil {
		t.Fatalf("First ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transferID := expressResp1.Msg.Transfer.Id

	// Second user expresses interest (joins group chat)
	ctx2 := createAuthenticatedContext(recipient2ID, "recipient2@example.com", models.Role_ROLE_USER)
	expressReq2 := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	_, err = service.ExpressInterest(ctx2, expressReq2)
	if err != nil {
		t.Fatalf("Second ExpressInterest failed: %v", err)
	}

	// Verify recipient1 is the current recipient_id
	transfer := &models.Transfer{}
	err = testStorage.GetByID(context.Background(), transferID, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.RecipientId != recipient1ID {
		t.Fatalf("Expected recipient1 to be recipient_id, got %s", transfer.RecipientId)
	}

	// First user (current recipient) withdraws
	withdrawReq := connect.NewRequest(&api.WithdrawInterestRequest{TransferId: transferID})
	_, err = service.WithdrawInterest(ctx1, withdrawReq)
	if err != nil {
		t.Fatalf("WithdrawInterest failed: %v", err)
	}

	// Verify recipient was reassigned to recipient2
	err = testStorage.GetByID(context.Background(), transferID, transfer)
	if err != nil {
		t.Fatalf("Failed to get updated transfer: %v", err)
	}

	if transfer.RecipientId != recipient2ID {
		t.Errorf("Expected recipient_id to be reassigned to recipient2, got %s", transfer.RecipientId)
	}

	// Verify transfer is still in INTEREST_EXPRESSED state
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		t.Errorf("Expected transfer to remain in INTEREST_EXPRESSED state, got %v", transfer.State)
	}
}

func TestWithdrawInterest_Giveaway_NonParticipantDenied(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")
	nonParticipantID := setupTestUser(t, testStorage, "NonParticipant", "nonparticipant@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add non-participant to community but they won't express interest
	nonParticipantMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      nonParticipantID,
		InviterId:   ownerID,
	}
	_, err := testStorage.Insert(context.Background(), nonParticipantMembership)
	if err != nil {
		t.Fatalf("Failed to add nonParticipant membership: %v", err)
	}

	// Recipient expresses interest
	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	expressReq := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	expressResp, err := service.ExpressInterest(ctx, expressReq)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transferID := expressResp.Msg.Transfer.Id

	// Non-participant tries to withdraw
	ctxNonParticipant := createAuthenticatedContext(nonParticipantID, "nonparticipant@example.com", models.Role_ROLE_USER)
	withdrawReq := connect.NewRequest(&api.WithdrawInterestRequest{TransferId: transferID})
	_, err = service.WithdrawInterest(ctxNonParticipant, withdrawReq)

	if err == nil {
		t.Fatal("Expected error but got none")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected error code PermissionDenied, got %v", connectErr.Code())
	}
}

func TestService_ExpressInterest_DuplicateCall_LoanReturnsExistingTransfer(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})

	resp1, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("first ExpressInterest failed: %v", err)
	}

	resp2, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("second ExpressInterest failed: %v", err)
	}

	if resp2.Msg.Transfer.Id != resp1.Msg.Transfer.Id {
		t.Errorf("expected idempotent response: got transfer %s, want %s",
			resp2.Msg.Transfer.Id, resp1.Msg.Transfer.Id)
	}

	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("failed to query transfers: %v", err)
	}
	if len(transfers) != 1 {
		t.Errorf("expected exactly 1 transfer row after duplicate call, got %d", len(transfers))
	}
}

func TestService_ExpressInterest_DuplicateCall_GiveawayReturnsExistingTransfer(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})

	resp1, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("first ExpressInterest failed: %v", err)
	}

	resp2, err := service.ExpressInterest(ctx, req)
	if err != nil {
		t.Fatalf("second ExpressInterest failed: %v", err)
	}

	if resp2.Msg.Transfer.Id != resp1.Msg.Transfer.Id {
		t.Errorf("expected idempotent response: got transfer %s, want %s",
			resp2.Msg.Transfer.Id, resp1.Msg.Transfer.Id)
	}

	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("failed to query transfers: %v", err)
	}
	if len(transfers) != 1 {
		t.Errorf("expected exactly 1 transfer row after duplicate call, got %d", len(transfers))
	}
}

func TestService_ExpressInterest_AfterWithdraw_AllowsNewTransfer(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	// First borrow request.
	resp1, err := service.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("first ExpressInterest failed: %v", err)
	}
	transferID1 := resp1.Msg.Transfer.Id

	// Withdraw the first request (transitions to CANCELLED).
	_, err = service.WithdrawInterest(ctx, connect.NewRequest(&api.WithdrawInterestRequest{TransferId: transferID1}))
	if err != nil {
		t.Fatalf("WithdrawInterest failed: %v", err)
	}

	// Second borrow request after cancellation — should succeed and create a new transfer.
	resp2, err := service.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("second ExpressInterest after withdraw failed: %v", err)
	}
	transferID2 := resp2.Msg.Transfer.Id

	if transferID2 == transferID1 {
		t.Error("expected a new transfer after withdrawal, but got the same transfer ID")
	}

	// Exactly two rows: one CANCELLED and one new non-terminal transfer.
	allTransfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("failed to query transfers: %v", err)
	}
	if len(allTransfers) != 2 {
		t.Fatalf("expected 2 transfer rows (one CANCELLED + one new), got %d", len(allTransfers))
	}

	cancelledCount := 0
	nonTerminalCount := 0
	for _, row := range allTransfers {
		t2 := row.(*models.Transfer)
		switch t2.State {
		case models.TransferState_TRANSFER_STATE_CANCELLED:
			cancelledCount++
		case models.TransferState_TRANSFER_STATE_COMPLETED:
			// Unexpected — loans never complete in this test flow.
		default:
			nonTerminalCount++
		}
	}
	if cancelledCount != 1 {
		t.Errorf("expected 1 CANCELLED transfer, got %d", cancelledCount)
	}
	if nonTerminalCount != 1 {
		t.Errorf("expected 1 non-terminal transfer, got %d", nonTerminalCount)
	}
}
