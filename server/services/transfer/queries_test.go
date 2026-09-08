package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

func TestService_ListMyTransfers(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	owner1ID := "owner1"
	owner2ID := "owner2"
	recipientID := "recipient123"

	// Create users
	ctx := context.Background()
	owner1 := &models.User{Id: owner1ID, Name: "Owner One", Email: "owner1@example.com"}
	_, err := testStorage.Insert(ctx, owner1)
	if err != nil {
		t.Fatalf("Failed to insert owner1: %v", err)
	}

	owner2 := &models.User{Id: owner2ID, Name: "Owner Two", Email: "owner2@example.com"}
	_, err = testStorage.Insert(ctx, owner2)
	if err != nil {
		t.Fatalf("Failed to insert owner2: %v", err)
	}

	recipient := &models.User{Id: recipientID, Name: "Recipient", Email: "recipient@example.com"}
	_, err = testStorage.Insert(ctx, recipient)
	if err != nil {
		t.Fatalf("Failed to insert recipient: %v", err)
	}

	// Create gear from different owners
	gear1ID := setupTestGear(t, testStorage, owner1ID, models.GearState_GEAR_STATE_AVAILABLE)
	gear2ID := setupTestGear(t, testStorage, owner1ID, models.GearState_GEAR_STATE_AVAILABLE)

	// Setup communities
	setupCommunityAndShareGear(t, testStorage, owner1ID, recipientID, gear1ID, models.Availability_AVAILABILITY_FOR_LOAN)
	setupCommunityAndShareGear(t, testStorage, owner1ID, recipientID, gear2ID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Create transfers
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	owner1Ctx := createAuthenticatedContext(owner1ID, "owner1@example.com", models.Role_ROLE_USER)

	// Recipient expresses interest in gear1 (loan)
	req1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gear1ID,
	})
	_, err = service.ExpressInterest(recipientCtx, req1)
	if err != nil {
		t.Fatalf("Failed to create transfer1: %v", err)
	}
	services.WaitForNotification(t, done)

	// Recipient expresses interest in gear2 (giveaway)
	req2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gear2ID,
	})
	_, err = service.ExpressInterest(recipientCtx, req2)
	if err != nil {
		t.Fatalf("Failed to create transfer2: %v", err)
	}
	services.WaitForNotification(t, done)

	t.Run("list all transfers for owner1", func(t *testing.T) {
		req := connect.NewRequest(&api.ListMyTransfersRequest{})
		resp, err := service.ListMyTransfers(owner1Ctx, req)
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}

		if len(resp.Msg.Transfers) != 2 {
			t.Errorf("Expected 2 transfers, got %d", len(resp.Msg.Transfers))
		}

		// Verify enriched data is present
		for _, transfer := range resp.Msg.Transfers {
			if transfer.GearName == "" {
				t.Error("Expected gear name to be populated")
			}
			if transfer.Recipient == nil || transfer.Recipient.Name == "" {
				t.Error("Expected recipient to be populated")
			}
			if transfer.Owner == nil || transfer.Owner.Name == "" {
				t.Error("Expected owner to be populated")
			}
		}
	})

	t.Run("filter by transfer type", func(t *testing.T) {
		req := connect.NewRequest(&api.ListMyTransfersRequest{
			TransferType: api.TransferType_TRANSFER_TYPE_LOAN,
		})
		resp, err := service.ListMyTransfers(owner1Ctx, req)
		if err != nil {
			t.Fatalf("ListMyTransfers failed: %v", err)
		}

		if len(resp.Msg.Transfers) != 1 {
			t.Errorf("Expected 1 loan transfer, got %d", len(resp.Msg.Transfers))
		}

		if resp.Msg.Transfers[0].TransferType != api.TransferType_TRANSFER_TYPE_LOAN {
			t.Errorf("Expected LOAN type, got %v", resp.Msg.Transfers[0].TransferType)
		}
	})
}

func TestService_ListReceivedTransfers(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := "owner123"
	recipientID := "recipient456"

	// Create users
	ctx := context.Background()
	owner := &models.User{Id: ownerID, Name: "Owner", Email: "owner@example.com"}
	_, err := testStorage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	recipient := &models.User{Id: recipientID, Name: "Recipient", Email: "recipient@example.com"}
	_, err = testStorage.Insert(ctx, recipient)
	if err != nil {
		t.Fatalf("Failed to insert recipient: %v", err)
	}

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create transfer
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// List received transfers
	listReq := connect.NewRequest(&api.ListReceivedTransfersRequest{})
	resp, err := service.ListReceivedTransfers(recipientCtx, listReq)
	if err != nil {
		t.Fatalf("ListReceivedTransfers failed: %v", err)
	}

	if len(resp.Msg.Transfers) != 1 {
		t.Errorf("Expected 1 transfer, got %d", len(resp.Msg.Transfers))
	}

	transfer := resp.Msg.Transfers[0]
	if transfer.Owner == nil || transfer.Owner.Name != "Owner" {
		t.Errorf("Expected owner name 'Owner', got %v", transfer.Owner)
	}

	if transfer.Recipient == nil || transfer.Recipient.Name != "Recipient" {
		t.Errorf("Expected recipient name 'Recipient', got %v", transfer.Recipient)
	}
}

func TestService_ListTransfers(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := "owner123"
	recipientID := "recipient456"

	// Create users
	ctx := context.Background()
	owner := &models.User{Id: ownerID, Name: "Owner", Email: "owner@example.com"}
	_, err := testStorage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	recipient := &models.User{Id: recipientID, Name: "Recipient", Email: "recipient@example.com"}
	_, err = testStorage.Insert(ctx, recipient)
	if err != nil {
		t.Fatalf("Failed to insert recipient: %v", err)
	}

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create transfer
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err = service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// List transfers in community
	listReq := connect.NewRequest(&api.ListTransfersRequest{
		CommunityId: communityID,
	})
	resp, err := service.ListTransfers(ownerCtx, listReq)
	if err != nil {
		t.Fatalf("ListTransfers failed: %v", err)
	}

	if len(resp.Msg.Transfers) != 1 {
		t.Errorf("Expected 1 transfer in community, got %d", len(resp.Msg.Transfers))
	}

	transfer := resp.Msg.Transfers[0]
	if transfer.GearName != "Test Drill" {
		t.Errorf("Expected gear name 'Test Drill', got %s", transfer.GearName)
	}
}

func TestService_GetTransfer(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := "owner123"
	recipientID := "recipient456"
	otherUserID := "other789"

	// Create users
	ctx := context.Background()
	owner := &models.User{Id: ownerID, Name: "Owner", Email: "owner@example.com"}
	_, err := testStorage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	recipient := &models.User{Id: recipientID, Name: "Recipient", Email: "recipient@example.com"}
	_, err = testStorage.Insert(ctx, recipient)
	if err != nil {
		t.Fatalf("Failed to insert recipient: %v", err)
	}

	otherUser := &models.User{Id: otherUserID, Name: "Other User", Email: "other@example.com"}
	_, err = testStorage.Insert(ctx, otherUser)
	if err != nil {
		t.Fatalf("Failed to insert other user: %v", err)
	}

	// Create gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create transfer
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	otherUserCtx := createAuthenticatedContext(otherUserID, "other@example.com", models.Role_ROLE_USER)

	expressReq := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	expressResp, err := service.ExpressInterest(recipientCtx, expressReq)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transferID := expressResp.Msg.Transfer.Id

	t.Run("owner can fetch their transfer", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferID,
		})
		resp, err := service.GetTransfer(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetTransfer failed for owner: %v", err)
		}

		transfer := resp.Msg.Transfer
		if transfer.Id != transferID {
			t.Errorf("Expected transfer ID %s, got %s", transferID, transfer.Id)
		}

		if transfer.GearName != "Test Drill" {
			t.Errorf("Expected gear name 'Test Drill', got %s", transfer.GearName)
		}

		if transfer.Owner == nil || transfer.Owner.Name != "Owner" {
			t.Errorf("Expected owner name 'Owner', got %v", transfer.Owner)
		}

		if transfer.Recipient == nil || transfer.Recipient.Name != "Recipient" {
			t.Errorf("Expected recipient name 'Recipient', got %v", transfer.Recipient)
		}

		if transfer.TransferType != api.TransferType_TRANSFER_TYPE_LOAN {
			t.Errorf("Expected LOAN type, got %v", transfer.TransferType)
		}

		// Loans are auto-approved, so expect RECIPIENT_SELECTED state
		if transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			t.Errorf("Expected RECIPIENT_SELECTED state, got %v", transfer.State)
		}
	})

	t.Run("recipient can fetch their transfer", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferID,
		})
		resp, err := service.GetTransfer(recipientCtx, req)
		if err != nil {
			t.Fatalf("GetTransfer failed for recipient: %v", err)
		}

		transfer := resp.Msg.Transfer
		if transfer.Id != transferID {
			t.Errorf("Expected transfer ID %s, got %s", transferID, transfer.Id)
		}

		if transfer.Owner.Id != ownerID {
			t.Errorf("Expected owner ID %s, got %s", ownerID, transfer.Owner.Id)
		}

		if transfer.Recipient.Id != recipientID {
			t.Errorf("Expected recipient ID %s, got %s", recipientID, transfer.Recipient.Id)
		}
	})

	t.Run("unauthorized user cannot fetch transfer", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferID,
		})
		_, err := service.GetTransfer(otherUserCtx, req)
		if err == nil {
			t.Fatal("Expected GetTransfer to fail for unauthorized user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected CodePermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("not found error for nonexistent transfer", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequest{
			TransferId: "nonexistent-id",
		})
		_, err := service.GetTransfer(ownerCtx, req)
		if err == nil {
			t.Fatal("Expected GetTransfer to fail for nonexistent transfer")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		unauthCtx := context.Background()
		req := connect.NewRequest(&api.GetTransferRequest{
			TransferId: transferID,
		})
		_, err := service.GetTransfer(unauthCtx, req)
		if err == nil {
			t.Fatal("Expected GetTransfer to fail for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected CodeUnauthenticated, got %v", connectErr.Code())
		}
	})
}

func TestService_GetUserTransferStatus(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ownerID := "owner123"
	participant1ID := "participant1"
	participant2ID := "participant2"
	nonParticipantID := "nonparticipant"

	// Create users
	ctx := context.Background()
	owner := &models.User{Id: ownerID, Name: "Owner", Email: "owner@example.com"}
	_, err := testStorage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Failed to insert owner: %v", err)
	}

	participant1 := &models.User{Id: participant1ID, Name: "Participant 1", Email: "participant1@example.com"}
	_, err = testStorage.Insert(ctx, participant1)
	if err != nil {
		t.Fatalf("Failed to insert participant1: %v", err)
	}

	participant2 := &models.User{Id: participant2ID, Name: "Participant 2", Email: "participant2@example.com"}
	_, err = testStorage.Insert(ctx, participant2)
	if err != nil {
		t.Fatalf("Failed to insert participant2: %v", err)
	}

	nonParticipant := &models.User{Id: nonParticipantID, Name: "Non Participant", Email: "nonparticipant@example.com"}
	_, err = testStorage.Insert(ctx, nonParticipant)
	if err != nil {
		t.Fatalf("Failed to insert nonparticipant: %v", err)
	}

	// Create gear for giveaway
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, participant1ID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Add participant2 to the community
	participant2Membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      participant2ID,
		InviterId:   ownerID,
	}
	_, err = testStorage.Insert(ctx, participant2Membership)
	if err != nil {
		t.Fatalf("Failed to add participant2 membership: %v", err)
	}

	// Add nonparticipant to the community (so they can query, just won't have active transfer)
	nonparticipantMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      nonParticipantID,
		InviterId:   ownerID,
	}
	_, err = testStorage.Insert(ctx, nonparticipantMembership)
	if err != nil {
		t.Fatalf("Failed to add nonparticipant membership: %v", err)
	}

	// First participant expresses interest (creates transfer + conversation)
	participant1Ctx := createAuthenticatedContext(participant1ID, "participant1@example.com", models.Role_ROLE_USER)
	participant2Ctx := createAuthenticatedContext(participant2ID, "participant2@example.com", models.Role_ROLE_USER)
	nonParticipantCtx := createAuthenticatedContext(nonParticipantID, "nonparticipant@example.com", models.Role_ROLE_USER)

	expressReq1 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	expressResp1, err := service.ExpressInterest(participant1Ctx, expressReq1)
	if err != nil {
		t.Fatalf("ExpressInterest failed for participant1: %v", err)
	}
	if expressResp1.Msg.Transfer == nil {
		t.Fatal("Expected transfer in response")
	}

	// Second participant expresses interest (joins existing group chat)
	expressReq2 := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	expressResp2, err := service.ExpressInterest(participant2Ctx, expressReq2)
	if err != nil {
		t.Fatalf("ExpressInterest failed for participant2: %v", err)
	}
	if expressResp2.Msg.Transfer == nil {
		t.Fatal("Expected transfer in response")
	}

	t.Run("participant1 has active transfer status", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserTransferStatusRequest{
			GearId: gearID,
		})
		resp, err := service.GetUserTransferStatus(participant1Ctx, req)
		if err != nil {
			t.Fatalf("GetUserTransferStatus failed for participant1: %v", err)
		}

		if !resp.Msg.HasActiveTransfer {
			t.Error("Expected participant1 to have active transfer")
		}

		if resp.Msg.ActiveTransfer == nil {
			t.Fatal("Expected ActiveTransfer to be populated")
		}

		if resp.Msg.ActiveTransfer.GearId != gearID {
			t.Errorf("Expected gear ID %s, got %s", gearID, resp.Msg.ActiveTransfer.GearId)
		}

		if resp.Msg.ActiveTransfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
			t.Errorf("Expected GIVEAWAY type, got %v", resp.Msg.ActiveTransfer.TransferType)
		}
	})

	t.Run("participant2 has active transfer status", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserTransferStatusRequest{
			GearId: gearID,
		})
		resp, err := service.GetUserTransferStatus(participant2Ctx, req)
		if err != nil {
			t.Fatalf("GetUserTransferStatus failed for participant2: %v", err)
		}

		if !resp.Msg.HasActiveTransfer {
			t.Error("Expected participant2 to have active transfer")
		}

		if resp.Msg.ActiveTransfer == nil {
			t.Fatal("Expected ActiveTransfer to be populated")
		}

		if resp.Msg.ActiveTransfer.GearId != gearID {
			t.Errorf("Expected gear ID %s, got %s", gearID, resp.Msg.ActiveTransfer.GearId)
		}
	})

	t.Run("non-participant has no active transfer status", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserTransferStatusRequest{
			GearId: gearID,
		})
		resp, err := service.GetUserTransferStatus(nonParticipantCtx, req)
		if err != nil {
			t.Fatalf("GetUserTransferStatus failed for non-participant: %v", err)
		}

		if resp.Msg.HasActiveTransfer {
			t.Error("Expected non-participant to have no active transfer")
		}

		if resp.Msg.ActiveTransfer != nil {
			t.Error("Expected ActiveTransfer to be nil for non-participant")
		}
	})
}

// TestGetUserTransferStatus_BatchConversationQuery verifies that GetUserTransferStatus
// fetches all transfer conversations in a single batch query regardless of transfer count,
// preventing N+1 regression on the topic_transfer_id lookup path.
func TestGetUserTransferStatus_BatchConversationQuery(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	ctx := context.Background()
	ownerID := setupTestUser(t, testStorage, "Batch Owner", "batch-owner@example.com")
	observer := &models.User{Name: "Batch Observer", Email: "batch-observer@example.com"}
	observerID, err := testStorage.Insert(ctx, observer)
	if err != nil {
		t.Fatalf("insert observer: %v", err)
	}

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	communityID := setupCommunityAndShareGear(t, testStorage, ownerID, observerID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	// Create three extra users who each have a transfer (to simulate multiple non-terminal transfers).
	const transferCount = 3
	for i := range transferCount {
		uid := setupTestUser(t, testStorage, "Requester", "requester@example.com")
		_, err := testStorage.Insert(ctx, &models.CommunityUser{
			CommunityId: communityID,
			UserId:      uid,
			InviterId:   ownerID,
		})
		if err != nil {
			t.Fatalf("insert requester %d membership: %v", i, err)
		}

		// Insert a transfer for this requester.
		transferID, err := testStorage.Insert(ctx, &models.Transfer{
			GearId:       gearID,
			OwnerId:      ownerID,
			RecipientId:  uid,
			TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			CommunityId:  communityID,
		})
		if err != nil {
			t.Fatalf("insert transfer %d: %v", i, err)
		}

		// Create a per-transfer conversation so the topic_transfer_id path is exercised.
		topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: transferID}}
		convID, _, err := chat.CreateOrGetConversation(ctx, testStorage, storage.NewChatConversationStorage(testStorage), communityID, topic)
		if err != nil {
			t.Fatalf("create conversation %d: %v", i, err)
		}

		// Add the observer to the first transfer's conversation only.
		if i == 0 {
			if err := chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(testStorage), convID, observerID); err != nil {
				t.Fatalf("add observer to conversation: %v", err)
			}
		}
	}

	observerCtx := createAuthenticatedContext(observerID, "batch-observer@example.com", models.Role_ROLE_USER)

	// Verify correctness: observer is found via the conversation participant path.
	resp, err := service.GetUserTransferStatus(observerCtx, connect.NewRequest(&api.GetUserTransferStatusRequest{GearId: gearID}))
	if err != nil {
		t.Fatalf("GetUserTransferStatus: %v", err)
	}
	if !resp.Msg.HasActiveTransfer {
		t.Error("expected observer to have active transfer via conversation participant path")
	}

	// Verify query count is bounded: should not grow with transferCount.
	// Old code: 2 + transferCount queries. New code: fixed set of batch queries.
	statsCtx := storage.WithQueryStats(observerCtx)
	storage.AssertMaxQueries(t, statsCtx, 15, func() {
		_, err := service.GetUserTransferStatus(statsCtx, connect.NewRequest(&api.GetUserTransferStatusRequest{GearId: gearID}))
		if err != nil {
			t.Errorf("GetUserTransferStatus (stats): %v", err)
		}
	})
}
