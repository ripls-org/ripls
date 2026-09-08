package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) (*storage.ProtoSQLStorage, *storage.ChatConversationStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage, storage.NewChatConversationStorage(sqlStorage)
}

func TestCreateOrGetConversation_Transfer_NewConversation(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Create a test transfer
	transfer := &models.Transfer{
		OwnerId:      "owner123",
		RecipientId:  "recipient456",
		GearId:       "gear789",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  "community123",
	}

	transferID, err := sqlStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	// Create conversation for transfer
	topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: transferID}}
	convID, participants, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)
	if err != nil {
		t.Fatalf("CreateOrGetConversation failed: %v", err)
	}

	// Verify conversation was created
	if convID == "" {
		t.Fatal("Expected conversation ID, got empty string")
	}

	// Verify participants (should be owner and recipient)
	if len(participants) != 2 {
		t.Fatalf("Expected 2 participants, got %d", len(participants))
	}

	expectedParticipants := map[string]bool{"owner123": true, "recipient456": true}
	for _, p := range participants {
		if !expectedParticipants[p] {
			t.Errorf("Unexpected participant: %s", p)
		}
	}

	// Verify conversation exists in storage
	conversation := &models.ChatConversation{}
	err = sqlStorage.GetByID(ctx, convID, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if conversation.CommunityId != "community123" {
		t.Errorf("Expected community_id=community123, got %s", conversation.CommunityId)
	}

	// Verify topic is set correctly
	if conversation.GetTopic().GetTransferId() != transferID {
		t.Errorf("Expected transfer_id=%s, got %s", transferID, conversation.GetTopic().GetTransferId())
	}
}

func TestCreateOrGetConversation_Transfer_ExistingConversation(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Create a test transfer
	transfer := &models.Transfer{
		OwnerId:      "owner123",
		RecipientId:  "recipient456",
		GearId:       "gear789",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  "community123",
	}

	transferID, err := sqlStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	// Create conversation first time
	topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: transferID}}
	convID1, _, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)
	if err != nil {
		t.Fatalf("CreateOrGetConversation failed: %v", err)
	}

	// Try to create conversation again with same transfer
	convID2, participants2, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)
	if err != nil {
		t.Fatalf("CreateOrGetConversation failed on second call: %v", err)
	}

	// Should return the same conversation ID
	if convID1 != convID2 {
		t.Errorf("Expected same conversation ID, got %s and %s", convID1, convID2)
	}

	// Participants should still be correct
	if len(participants2) != 2 {
		t.Errorf("Expected 2 participants on second call, got %d", len(participants2))
	}
}

func TestCreateOrGetConversation_Request_NewConversation(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Create a test request
	request := &models.Request{
		RequesterId: "requester123",
		Description: "Test request",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}

	requestID, err := sqlStorage.Insert(ctx, request)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	// Link request to community
	_, err = sqlStorage.Insert(ctx, &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: "community456",
	})
	if err != nil {
		t.Fatalf("Failed to link request to community: %v", err)
	}

	// Create conversation for request
	topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: requestID}}
	convID, participants, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community456", topic)
	if err != nil {
		t.Fatalf("CreateOrGetConversation failed: %v", err)
	}

	// Verify conversation was created
	if convID == "" {
		t.Fatal("Expected conversation ID, got empty string")
	}

	// Verify participants (should be empty for requests - added separately)
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants for request conversation, got %d", len(participants))
	}

	// Verify conversation exists in storage
	conversation := &models.ChatConversation{}
	err = sqlStorage.GetByID(ctx, convID, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if conversation.CommunityId != "community456" {
		t.Errorf("Expected community_id=community456, got %s", conversation.CommunityId)
	}

	// Verify topic is set correctly
	if conversation.GetTopic().GetRequestId() != requestID {
		t.Errorf("Expected request_id=%s, got %s", requestID, conversation.GetTopic().GetRequestId())
	}
}

func TestCreateOrGetConversation_NoTopic(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Try to create conversation with no topic
	topic := &models.ConversationTopic{}
	_, _, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)

	// Should fail with invalid argument error
	if err == nil {
		t.Fatal("Expected error when no topic provided, got nil")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}

func TestCreateOrGetConversation_BothTopics(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Try to create conversation with both topics — now that topic is a oneof,
	// we just test with a nil topic (no topic set).
	_, _, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", &models.ConversationTopic{})

	// Should fail with invalid argument error
	if err == nil {
		t.Fatal("Expected error when both topics provided, got nil")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}

func TestCreateOrGetConversation_TransferNotFound(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Try to create conversation for non-existent transfer
	topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: "nonexistent"}}
	_, _, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)

	// Should fail with not found error
	if err == nil {
		t.Fatal("Expected error when transfer not found, got nil")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestCreateOrGetConversation_RequestNotFound(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Try to create conversation for non-existent request
	topic := &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "nonexistent"}}
	_, _, err := CreateOrGetConversation(ctx, sqlStorage, chatConvStorage, "community123", topic)

	// Should fail with not found error
	if err == nil {
		t.Fatal("Expected error when request not found, got nil")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestAddParticipantToConversation_Success(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Create a conversation via the wrapper so participant_ids is indexed.
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}

	convID, err := chatConvStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create test conversation: %v", err)
	}

	// Add a new participant
	err = AddParticipantToConversation(ctx, chatConvStorage, convID, "user3")
	if err != nil {
		t.Fatalf("AddParticipantToConversation failed: %v", err)
	}

	// Verify participant was added
	updatedConv := &models.ChatConversation{}
	err = sqlStorage.GetByID(ctx, convID, updatedConv)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(updatedConv.ParticipantIds) != 3 {
		t.Fatalf("Expected 3 participants, got %d", len(updatedConv.ParticipantIds))
	}

	found := false
	for _, p := range updatedConv.ParticipantIds {
		if p == "user3" {
			found = true
			break
		}
	}
	if !found {
		t.Error("New participant user3 not found in conversation")
	}
}

func TestAddParticipantToConversation_AlreadyPresent(t *testing.T) {
	sqlStorage, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Create a conversation
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}

	convID, err := chatConvStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create test conversation: %v", err)
	}

	// Try to add existing participant
	err = AddParticipantToConversation(ctx, chatConvStorage, convID, "user1")
	if err != nil {
		t.Fatalf("AddParticipantToConversation failed: %v", err)
	}

	// Verify participant count unchanged
	updatedConv := &models.ChatConversation{}
	err = sqlStorage.GetByID(ctx, convID, updatedConv)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(updatedConv.ParticipantIds) != 2 {
		t.Fatalf("Expected 2 participants (unchanged), got %d", len(updatedConv.ParticipantIds))
	}
}

func TestAddParticipantToConversation_ConversationNotFound(t *testing.T) {
	_, chatConvStorage := setupTestStorage(t)
	ctx := context.Background()

	// Try to add participant to non-existent conversation
	err := AddParticipantToConversation(ctx, chatConvStorage, "nonexistent", "user1")

	// Should fail with not found error
	if err == nil {
		t.Fatal("Expected error when conversation not found, got nil")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
	}
}
