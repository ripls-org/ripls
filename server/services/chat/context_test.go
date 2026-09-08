package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	storagepkg "go.ripls.org/ripls/server/storage"
)

// TestGetConversationContext_CommunityMemberAccess verifies that any community member
// can access conversation context for gear conversations in their community.
func TestGetConversationContext_CommunityMemberAccess(t *testing.T) {
	svc, storage := setupTestChatService(t)

	// Create users
	ownerID := "owner-user"
	participantID := "participant-user"
	communityMemberID := "community-member-user"
	createTestUsers(t, storage, ownerID, "Owner User")
	createTestUsers(t, storage, participantID, "Participant User")
	createTestUsers(t, storage, communityMemberID, "Community Member")

	// Create community
	communityID := createTestCommunity(t, storage, ownerID)

	// Add all users to the community
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, participantID, communityID)
	addUserToCommunity(t, storage, communityMemberID, communityID)

	// Create gear
	gear := &models.Gear{
		OwnerId:  ownerID,
		Name:     "Test Gear",
		MediaIds: []string{"media123"},
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}

	// Create CommunityGear (gear shared in community)
	communityGear := &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	_, err = storage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	// Create a gear conversation with owner and participant
	conversation := &models.ChatConversation{
		CommunityId: communityID,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{
			GearId: gearID,
		}},
		ParticipantIds: []string{ownerID, participantID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test 1: Participant can access
	ctx := contextWithAuth(participantID, "participant@example.com")
	req := connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	resp, err := svc.GetConversationContext(ctx, req)
	if err != nil {
		t.Fatalf("Participant should be able to access conversation context: %v", err)
	}
	if resp.Msg.Context.TopicTitle != "Test Gear" {
		t.Errorf("Expected topic title 'Test Gear', got '%s'", resp.Msg.Context.TopicTitle)
	}
	if resp.Msg.Context.Availability != api.Availability_AVAILABILITY_FOR_LOAN {
		t.Errorf("Expected availability FOR_LOAN, got %v", resp.Msg.Context.Availability)
	}

	// Test 2: Community member (not a participant) can also access
	ctx = contextWithAuth(communityMemberID, "member@example.com")
	req = connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	resp, err = svc.GetConversationContext(ctx, req)
	if err != nil {
		t.Fatalf("Community member should be able to access conversation context: %v", err)
	}
	if resp.Msg.Context.TopicTitle != "Test Gear" {
		t.Errorf("Expected topic title 'Test Gear', got '%s'", resp.Msg.Context.TopicTitle)
	}
}

// TestGetConversationContext_NonCommunityMemberDenied verifies that users who are not
// in the community cannot access conversation context.
func TestGetConversationContext_NonCommunityMemberDenied(t *testing.T) {
	svc, storage := setupTestChatService(t)

	// Create users
	ownerID := "owner-user"
	participantID := "participant-user"
	outsiderID := "outsider-user"
	createTestUsers(t, storage, ownerID, "Owner User")
	createTestUsers(t, storage, participantID, "Participant User")
	createTestUsers(t, storage, outsiderID, "Outsider User")

	// Create community
	communityID := createTestCommunity(t, storage, ownerID)

	// Add only owner and participant to the community (NOT outsider)
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, participantID, communityID)

	// Create gear
	gear := &models.Gear{
		OwnerId: ownerID,
		Name:    "Test Gear",
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}

	// Create a gear conversation
	conversation := &models.ChatConversation{
		CommunityId: communityID,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{
			GearId: gearID,
		}},
		ParticipantIds: []string{ownerID, participantID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test: Outsider (not in community) should be denied
	ctx := contextWithAuth(outsiderID, "outsider@example.com")
	req := connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	_, err = svc.GetConversationContext(ctx, req)
	if err == nil {
		t.Fatal("Expected error when non-community-member tries to access conversation context")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", connectErr.Code())
	}
}

// TestGetConversationContext_DeletedRequestReturnsNotFound verifies that conversation
// context returns NOT_FOUND when the associated request has been soft-deleted.
func TestGetConversationContext_DeletedRequestReturnsNotFound(t *testing.T) {
	svc, storage := setupTestChatService(t)

	// Create users
	requesterID := "requester-user"
	offererID := "offerer-user"
	createTestUsers(t, storage, requesterID, "Requester User")
	createTestUsers(t, storage, offererID, "Offerer User")

	// Create community
	communityID := createTestCommunity(t, storage, requesterID)
	addUserToCommunity(t, storage, requesterID, communityID)
	addUserToCommunity(t, storage, offererID, communityID)

	// Create request
	request := &models.Request{
		RequesterId: requesterID,
		Title:       "Need a drill",
		Description: "Looking for a power drill",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	requestID, err := storage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	// Link request to community
	_, err = storage.Insert(context.Background(), &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	if err != nil {
		t.Fatalf("Failed to link request to community: %v", err)
	}

	// Create a request conversation
	conversation := &models.ChatConversation{
		CommunityId: communityID,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{
			RequestId: requestID,
		}},
		ParticipantIds: []string{requesterID, offererID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test 1: Can access context before deletion
	ctx := contextWithAuth(offererID, "offerer@example.com")
	req := connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	resp, err := svc.GetConversationContext(ctx, req)
	if err != nil {
		t.Fatalf("Should be able to access conversation context before deletion: %v", err)
	}
	if resp.Msg.Context.TopicTitle != "Need a drill" {
		t.Errorf("Expected topic title 'Need a drill', got '%s'", resp.Msg.Context.TopicTitle)
	}

	// Soft-delete the request (need to get fresh copy with ID set)
	deleteRequest := &models.Request{}
	if err := storage.GetByID(context.Background(), requestID, deleteRequest, storagepkg.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("Failed to get request for deletion: %v", err)
	}
	deleteRequest.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  requesterID,
		DeletedAtUnixSec: 12345,
	}
	if err := storage.Update(context.Background(), deleteRequest); err != nil {
		t.Fatalf("Failed to soft-delete request: %v", err)
	}

	// Test 2: Cannot access context after deletion - should return NOT_FOUND
	req = connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	_, err = svc.GetConversationContext(ctx, req)
	if err == nil {
		t.Fatal("Expected error when accessing context for deleted request")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected NotFound error, got: %v", connectErr.Code())
	}
}

// TestGetConversationContext_OwnerSeesActiveLoan verifies that the gear owner
// gets userTransfer populated with the most advanced non-terminal transfer.
// This is critical for loan conversations where the owner needs to see the
// current phase and act on it (e.g., "Mark as Returned" in ACTIVE state).
func TestGetConversationContext_OwnerSeesActiveLoan(t *testing.T) {
	svc, storage := setupTestChatService(t)

	ownerID := "owner-user"
	borrowerID := "borrower-user"
	createTestUsers(t, storage, ownerID, "Owner")
	createTestUsers(t, storage, borrowerID, "Borrower")

	communityID := createTestCommunity(t, storage, ownerID)
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, borrowerID, communityID)

	gear := &models.Gear{
		OwnerId: ownerID,
		Name:    "Drill",
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	communityGear := &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := storage.Insert(context.Background(), communityGear); err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	conversation := &models.ChatConversation{
		CommunityId:    communityID,
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{ownerID, borrowerID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	tests := []struct {
		name          string
		transferState models.TransferState
		wantState     api.TransferState
	}{
		{
			name:          "owner sees RECIPIENT_SELECTED transfer",
			transferState: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			wantState:     api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		},
		{
			name:          "owner sees ACTIVE transfer",
			transferState: models.TransferState_TRANSFER_STATE_ACTIVE,
			wantState:     api.TransferState_TRANSFER_STATE_ACTIVE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create transfer in the specified state.
			transfer := &models.Transfer{
				GearId:       gearID,
				CommunityId:  communityID,
				OwnerId:      ownerID,
				RecipientId:  borrowerID,
				TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
				State:        tt.transferState,
			}
			transferID, err := storage.Insert(context.Background(), transfer)
			if err != nil {
				t.Fatalf("Failed to create transfer: %v", err)
			}

			// Owner requests context.
			ctx := contextWithAuth(ownerID, "owner@example.com")
			req := connect.NewRequest(&api.GetConversationContextRequest{
				ConversationId: conversationID,
			})
			resp, err := svc.GetConversationContext(ctx, req)
			if err != nil {
				t.Fatalf("GetConversationContext failed: %v", err)
			}

			gearCtx := resp.Msg.Context.GearTransferContext
			if gearCtx == nil {
				t.Fatal("Expected gear transfer context to be set")
			}
			if !gearCtx.IsOwner {
				t.Error("Expected IsOwner to be true")
			}
			if gearCtx.UserTransfer == nil {
				t.Fatal("Expected UserTransfer to be populated for owner")
			}
			if gearCtx.UserTransfer.State != tt.wantState {
				t.Errorf("Expected UserTransfer.State=%v, got %v",
					tt.wantState, gearCtx.UserTransfer.State)
			}

			// Clean up for next iteration.
			if err := storage.Delete(context.Background(), &models.Transfer{Id: transferID}); err != nil {
				t.Fatalf("Failed to delete transfer: %v", err)
			}
		})
	}
}

// TestGetConversationContext_TransferIncludesPickupDetails verifies that the
// transfer context includes pickup times and loan duration when populated,
// not just the minimal id/state/type fields.
func TestGetConversationContext_TransferIncludesPickupDetails(t *testing.T) {
	svc, storage := setupTestChatService(t)

	ownerID := "owner-user"
	borrowerID := "borrower-user"
	createTestUsers(t, storage, ownerID, "Owner")
	createTestUsers(t, storage, borrowerID, "Borrower")

	communityID := createTestCommunity(t, storage, ownerID)
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, borrowerID, communityID)

	gear := &models.Gear{OwnerId: ownerID, Name: "Drill"}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	communityGear := &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := storage.Insert(context.Background(), communityGear); err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	conversation := &models.ChatConversation{
		CommunityId:    communityID,
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{ownerID, borrowerID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Create a transfer with pickup details.
	pickupSec := int64(1710000000)
	durationDays := int32(7)
	transfer := &models.Transfer{
		GearId:                 gearID,
		CommunityId:            communityID,
		OwnerId:                ownerID,
		RecipientId:            borrowerID,
		TransferType:           models.TransferType_TRANSFER_TYPE_LOAN,
		State:                  models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		EstimatedPickupUnixSec: &pickupSec,
		LoanDurationDays:       &durationDays,
	}
	if _, err := storage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	// Borrower fetches context — should see pickup details on userTransfer.
	ctx := contextWithAuth(borrowerID, "borrower@example.com")
	req := connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	})
	resp, err := svc.GetConversationContext(ctx, req)
	if err != nil {
		t.Fatalf("GetConversationContext failed: %v", err)
	}

	gearCtx := resp.Msg.Context.GearTransferContext
	if gearCtx == nil || gearCtx.UserTransfer == nil {
		t.Fatal("Expected UserTransfer to be populated")
	}
	if gearCtx.UserTransfer.EstimatedPickupUnixSec == nil {
		t.Error("Expected EstimatedPickupUnixSec to be set on transfer")
	} else if *gearCtx.UserTransfer.EstimatedPickupUnixSec != pickupSec {
		t.Errorf("Expected EstimatedPickupUnixSec=%d, got %d",
			pickupSec, *gearCtx.UserTransfer.EstimatedPickupUnixSec)
	}
	if gearCtx.UserTransfer.LoanDurationDays == nil {
		t.Error("Expected LoanDurationDays to be set on transfer")
	} else if *gearCtx.UserTransfer.LoanDurationDays != durationDays {
		t.Errorf("Expected LoanDurationDays=%d, got %d",
			durationDays, *gearCtx.UserTransfer.LoanDurationDays)
	}
}

// addUserToCommunity is a helper to add a user to a community.
func addUserToCommunity(t *testing.T, storage *storagepkg.ProtoSQLStorage, userID, communityID string) {
	membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}
	_, err := storage.Insert(context.Background(), membership)
	if err != nil {
		t.Fatalf("Failed to add user to community: %v", err)
	}
}
