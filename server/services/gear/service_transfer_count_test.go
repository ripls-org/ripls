package gear

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
)

func TestService_GetTransferRequestCount(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ownerID := "owner123"
	borrowerID := "borrower456"
	gearID := "gear789"

	// Create test users
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	borrowerCtx := createAuthenticatedContext(borrowerID, "borrower@example.com", models.Role_ROLE_USER)

	ownerUser := &models.User{
		Id:    ownerID,
		Email: "owner@example.com",
		Name:  "Owner User",
	}
	_, err := testStorage.Insert(ownerCtx, ownerUser)
	if err != nil {
		t.Fatalf("Failed to insert owner user: %v", err)
	}

	borrowerUser := &models.User{
		Id:    borrowerID,
		Email: "borrower@example.com",
		Name:  "Borrower User",
	}
	_, err = testStorage.Insert(borrowerCtx, borrowerUser)
	if err != nil {
		t.Fatalf("Failed to insert borrower user: %v", err)
	}

	// Create test gear owned by owner
	testGear := &models.Gear{
		Id:      gearID,
		Name:    "Test Gear",
		OwnerId: ownerID,
	}
	_, err = testStorage.Insert(ownerCtx, testGear)
	if err != nil {
		t.Fatalf("Failed to insert test gear: %v", err)
	}

	// Share gear with community1 and add both users as members, so borrower
	// passes the RequireAccessToCommunityScopedEntity check.
	community1 := &models.Community{
		Id:          "community1",
		Name:        "Transfer Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	if _, err = testStorage.Insert(ownerCtx, community1); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}
	if _, err = testStorage.Insert(ownerCtx, &models.CommunityUser{CommunityId: "community1", UserId: ownerID, InviterId: ownerID}); err != nil {
		t.Fatalf("Failed to add owner membership: %v", err)
	}
	if _, err = testStorage.Insert(ownerCtx, &models.CommunityUser{CommunityId: "community1", UserId: borrowerID, InviterId: ownerID}); err != nil {
		t.Fatalf("Failed to add borrower membership: %v", err)
	}
	if _, err = testStorage.Insert(ownerCtx, &models.CommunityGear{CommunityId: "community1", GearId: gearID}); err != nil {
		t.Fatalf("Failed to add community gear: %v", err)
	}

	t.Run("no requests for owner", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		resp, err := service.GetTransferRequestCount(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		if resp.Msg.ReceivedCount != 0 {
			t.Errorf("Expected ReceivedCount 0, got %d", resp.Msg.ReceivedCount)
		}

		if resp.Msg.HasActiveRequest {
			t.Error("Expected HasActiveRequest false")
		}
	})

	t.Run("owner with active requests", func(t *testing.T) {
		// Create transfers
		transfer1 := &models.Transfer{
			GearId:      gearID,
			OwnerId:     ownerID,
			RecipientId: borrowerID,
			State:       models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			CommunityId: "community1",
		}
		_, err := testStorage.Insert(ownerCtx, transfer1)
		if err != nil {
			t.Fatalf("Failed to insert transfer1: %v", err)
		}

		transfer2 := &models.Transfer{
			GearId:      gearID,
			OwnerId:     ownerID,
			RecipientId: "borrower999",
			State:       models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			CommunityId: "community1",
		}
		_, err = testStorage.Insert(ownerCtx, transfer2)
		if err != nil {
			t.Fatalf("Failed to insert transfer2: %v", err)
		}

		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		resp, err := service.GetTransferRequestCount(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		if resp.Msg.ReceivedCount != 2 {
			t.Errorf("Expected ReceivedCount 2, got %d", resp.Msg.ReceivedCount)
		}
	})

	t.Run("owner with completed requests", func(t *testing.T) {
		// Create completed transfer
		transfer3 := &models.Transfer{
			GearId:      gearID,
			OwnerId:     ownerID,
			RecipientId: "borrower888",
			State:       models.TransferState_TRANSFER_STATE_COMPLETED,
			CommunityId: "community1",
		}
		_, err := testStorage.Insert(ownerCtx, transfer3)
		if err != nil {
			t.Fatalf("Failed to insert transfer3: %v", err)
		}

		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		resp, err := service.GetTransferRequestCount(ownerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		// Should still be 2 (archived not counted)
		if resp.Msg.ReceivedCount != 2 {
			t.Errorf("Expected ReceivedCount 2 (archived excluded), got %d", resp.Msg.ReceivedCount)
		}
	})

	t.Run("borrower without active request", func(t *testing.T) {
		// Use a different gear for this test
		gear2ID := "gear-no-request"
		gear2 := &models.Gear{
			Id:      gear2ID,
			Name:    "Gear Without Request",
			OwnerId: ownerID,
		}
		_, err := testStorage.Insert(ownerCtx, gear2)
		if err != nil {
			t.Fatalf("Failed to insert gear2: %v", err)
		}
		if _, err := testStorage.Insert(ownerCtx, &models.CommunityGear{CommunityId: "community1", GearId: gear2ID}); err != nil {
			t.Fatalf("Failed to add community gear for gear2: %v", err)
		}

		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gear2ID,
		})

		resp, err := service.GetTransferRequestCount(borrowerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		if resp.Msg.HasActiveRequest {
			t.Error("Expected HasActiveRequest false")
		}

		if resp.Msg.ActiveConversationId != "" {
			t.Error("Expected empty ActiveConversationId")
		}
	})

	t.Run("borrower with active request", func(t *testing.T) {
		// Find existing transfer for borrower
		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		resp, err := service.GetTransferRequestCount(borrowerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		// Borrower has an active transfer (created in earlier test)
		if !resp.Msg.HasActiveRequest {
			t.Error("Expected HasActiveRequest true")
		}
	})

	t.Run("borrower with active request and conversation", func(t *testing.T) {
		// Create a conversation for the transfer
		transfers, err := testStorage.QueryByFields(ownerCtx, map[string]any{
			"gear_id":      gearID,
			"recipient_id": borrowerID,
		}, &models.Transfer{})
		if err != nil {
			t.Fatalf("Failed to query transfers: %v", err)
		}

		if len(transfers) == 0 {
			t.Fatal("Expected to find transfer for borrower")
		}

		transfer := transfers[0].(*models.Transfer)

		conversation := &models.ChatConversation{}
		conversation.Topic = &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{
			TransferId: transfer.Id,
		}}
		conversationID, err := testStorage.Insert(ownerCtx, conversation)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		resp, err := service.GetTransferRequestCount(borrowerCtx, req)
		if err != nil {
			t.Fatalf("GetTransferRequestCount failed: %v", err)
		}

		if !resp.Msg.HasActiveRequest {
			t.Error("Expected HasActiveRequest true")
		}

		if resp.Msg.ActiveConversationId != conversationID {
			t.Errorf("Expected ActiveConversationId %s, got %s", conversationID, resp.Msg.ActiveConversationId)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: gearID,
		})

		_, err := service.GetTransferRequestCount(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("gear not found", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTransferRequestCountRequest{
			GearId: "nonexistent-gear",
		})

		_, err := service.GetTransferRequestCount(ownerCtx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})
}

// TestService_GetActiveLoanForGear tests the getActiveLoanForGear method behavior.
func TestService_GetActiveLoanForGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ownerCtx := createAuthenticatedContext("owner-user", "owner@example.com", models.Role_ROLE_USER)
	recipientCtx := createAuthenticatedContext("recipient-user", "recipient@example.com", models.Role_ROLE_USER)

	// Create owner and recipient users
	ownerUser := &models.User{
		Id:    "owner-user",
		Email: "owner@example.com",
		Name:  "Owner User",
		Role:  models.Role_ROLE_USER,
	}
	recipientUser := &models.User{
		Id:    "recipient-user",
		Email: "recipient@example.com",
		Name:  "Recipient User",
		Role:  models.Role_ROLE_USER,
	}
	if _, err := testStorage.Insert(ownerCtx, ownerUser); err != nil {
		t.Fatalf("Failed to create owner user: %v", err)
	}
	if _, err := testStorage.Insert(recipientCtx, recipientUser); err != nil {
		t.Fatalf("Failed to create recipient user: %v", err)
	}

	// Create community
	community := &models.Community{
		Id:          "test-community",
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := testStorage.Insert(ownerCtx, community); err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Create gear
	gear := &models.Gear{
		Id:      "test-gear",
		OwnerId: "owner-user",
		Name:    "Test Gear",
	}
	if _, err := testStorage.Insert(ownerCtx, gear); err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	// Share gear in community
	communityGear := &models.CommunityGear{
		GearId:       "test-gear",
		CommunityId:  "test-community",
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := testStorage.Insert(ownerCtx, communityGear); err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	t.Run("returns activeLoan for ACTIVE loan transfer", func(t *testing.T) {
		// Create an ACTIVE loan transfer
		transfer := &models.Transfer{
			Id:           "active-loan-transfer",
			GearId:       "test-gear",
			CommunityId:  "test-community",
			OwnerId:      "owner-user",
			RecipientId:  "recipient-user",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		if _, err := testStorage.Insert(ownerCtx, transfer); err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}
		defer func() {
			if err := testStorage.Delete(ownerCtx, transfer); err != nil {
				t.Errorf("Failed to delete transfer in cleanup: %v", err)
			}
		}()

		// Call getActiveLoanForGear
		activeLoan, err := service.getActiveLoanForGear(ownerCtx, "test-gear", "test-community")
		if err != nil {
			t.Fatalf("getActiveLoanForGear failed: %v", err)
		}

		// Should return the activeLoan
		if activeLoan == nil {
			t.Fatal("Expected activeLoan to be returned for ACTIVE loan transfer")
		}
		if activeLoan.TransferId != "active-loan-transfer" {
			t.Errorf("Expected transfer ID %s, got %s", "active-loan-transfer", activeLoan.TransferId)
		}
		if activeLoan.Status != "On loan" {
			t.Errorf("Expected status 'On loan', got %s", activeLoan.Status)
		}
	})

	t.Run("returns nil for COMPLETED loan transfer", func(t *testing.T) {
		// Create a COMPLETED loan transfer
		transfer := &models.Transfer{
			Id:           "completed-loan-transfer",
			GearId:       "test-gear",
			CommunityId:  "test-community",
			OwnerId:      "owner-user",
			RecipientId:  "recipient-user",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		}
		if _, err := testStorage.Insert(ownerCtx, transfer); err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}
		defer func() {
			if err := testStorage.Delete(ownerCtx, transfer); err != nil {
				t.Errorf("Failed to delete transfer in cleanup: %v", err)
			}
		}()

		// Call getActiveLoanForGear
		activeLoan, err := service.getActiveLoanForGear(ownerCtx, "test-gear", "test-community")
		if err != nil {
			t.Fatalf("getActiveLoanForGear failed: %v", err)
		}

		// Should return nil (no activeLoan for completed loans)
		if activeLoan != nil {
			t.Errorf("Expected nil for COMPLETED loan transfer, got activeLoan with status %s", activeLoan.Status)
		}
	})

	t.Run("returns activeLoan for COMPLETED giveaway transfer", func(t *testing.T) {
		// Query for existing community gear and update availability to giveaway
		results, err := testStorage.QueryByFields(ownerCtx, map[string]any{
			"gear_id":      "test-gear",
			"community_id": "test-community",
		}, &models.CommunityGear{})
		if err != nil || len(results) == 0 {
			t.Fatalf("Failed to query community gear: %v", err)
		}
		communityGear := results[0].(*models.CommunityGear)
		communityGear.Availability = models.Availability_AVAILABILITY_FOR_GIVEAWAY
		if err := testStorage.Update(ownerCtx, communityGear); err != nil {
			t.Fatalf("Failed to update community gear: %v", err)
		}

		// Create a COMPLETED giveaway transfer
		transfer := &models.Transfer{
			Id:           "completed-giveaway-transfer",
			GearId:       "test-gear",
			CommunityId:  "test-community",
			OwnerId:      "owner-user",
			RecipientId:  "recipient-user",
			TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		}
		if _, err := testStorage.Insert(ownerCtx, transfer); err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}
		defer func() {
			if err := testStorage.Delete(ownerCtx, transfer); err != nil {
				t.Errorf("Failed to delete transfer in cleanup: %v", err)
			}
		}()

		// Call getActiveLoanForGear
		activeLoan, err := service.getActiveLoanForGear(ownerCtx, "test-gear", "test-community")
		if err != nil {
			t.Fatalf("getActiveLoanForGear failed: %v", err)
		}

		// Should return the activeLoan for completed giveaways (to show who received it)
		if activeLoan == nil {
			t.Fatal("Expected activeLoan to be returned for COMPLETED giveaway transfer")
		}
		if activeLoan.TransferId != "completed-giveaway-transfer" {
			t.Errorf("Expected transfer ID %s, got %s", "completed-giveaway-transfer", activeLoan.TransferId)
		}
		if activeLoan.Status != "Given away" {
			t.Errorf("Expected status 'Given away', got %s", activeLoan.Status)
		}
	})

	t.Run("returns nil when no transfers exist", func(t *testing.T) {
		// Create a new gear with no transfers
		emptyGear := &models.Gear{
			Id:      "empty-gear",
			OwnerId: "owner-user",
			Name:    "Empty Gear",
		}
		if _, err := testStorage.Insert(ownerCtx, emptyGear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}
		defer func() {
			if err := testStorage.Delete(ownerCtx, emptyGear); err != nil {
				t.Errorf("Failed to delete gear in cleanup: %v", err)
			}
		}()

		emptyCommunityGear := &models.CommunityGear{
			GearId:       "empty-gear",
			CommunityId:  "test-community",
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		}
		if _, err := testStorage.Insert(ownerCtx, emptyCommunityGear); err != nil {
			t.Fatalf("Failed to create community gear: %v", err)
		}
		defer func() {
			if err := testStorage.Delete(ownerCtx, emptyCommunityGear); err != nil {
				t.Errorf("Failed to delete community gear in cleanup: %v", err)
			}
		}()

		// Call getActiveLoanForGear
		activeLoan, err := service.getActiveLoanForGear(ownerCtx, "empty-gear", "test-community")
		if err != nil {
			t.Fatalf("getActiveLoanForGear failed: %v", err)
		}

		// Should return nil
		if activeLoan != nil {
			t.Errorf("Expected nil when no transfers exist, got activeLoan with status %s", activeLoan.Status)
		}
	})
}
