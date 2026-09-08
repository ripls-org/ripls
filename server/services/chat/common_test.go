package chat

import (
	"context"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

func setupTestChatService(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	topic := pubsub.NewMemTopic[*chat_event_bus.PublishedEvent](chat_event_bus.TopicName)
	bus := chat_event_bus.NewInProcessBus(sqlStorage, topic)
	svc := New(sqlStorage, bus)
	if _, err := bus.Subscribe(svc.StreamSubscriber()); err != nil {
		t.Fatalf("subscribe stream subscriber: %v", err)
	}
	return svc, sqlStorage
}

// drainChatBus waits for all in-flight chat bus events to be processed.
func drainChatBus(t *testing.T, svc *Service) {
	t.Helper()
	if b, ok := svc.bus.(*chat_event_bus.InProcessBus); ok {
		if err := b.Drain(context.Background()); err != nil {
			t.Fatalf("drain chat bus: %v", err)
		}
	}
}

func contextWithAuth(userID, email string) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   models.Role_ROLE_USER,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

// createTestLoan creates a loan for testing.
// Note: This function uses hardcoded gear_id and community_id for backward compatibility.
// For tests that need CommunityGear records, use createTestLoanWithCommunity instead.
func createTestLoan(t *testing.T, storage *storage.ProtoSQLStorage, lenderID, borrowerID string) string {
	transfer := &models.Transfer{
		OwnerId:      lenderID,
		RecipientId:  borrowerID,
		GearId:       "gear123",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  "community123",
	}

	transferID, err := storage.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	return transferID
}

// createTestLoanWithCommunity creates a loan for testing with a specific community ID
// and creates the necessary CommunityGear record.
func createTestLoanWithCommunity(t *testing.T, storage *storage.ProtoSQLStorage, lenderID, borrowerID, communityID string) string {
	// Create gear
	gear := &models.Gear{
		OwnerId: lenderID,
		Name:    "Test Gear",
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}

	// Create transfer
	transfer := &models.Transfer{
		OwnerId:      lenderID,
		RecipientId:  borrowerID,
		GearId:       gearID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  communityID,
	}

	transferID, err := storage.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	// Create CommunityGear record (needed for GetConversationForTransfer)
	communityGear := &models.CommunityGear{
		GearId:      gearID,
		CommunityId: communityID,
	}
	_, err = storage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create test community gear: %v", err)
	}

	return transferID
}

// createTestTransferWithState creates a transfer with a specific state for testing.
func createTestTransferWithState(t *testing.T, storage *storage.ProtoSQLStorage, lenderID, borrowerID string, state models.TransferState) string {
	transfer := &models.Transfer{
		OwnerId:      lenderID,
		RecipientId:  borrowerID,
		GearId:       "gear123",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        state,
		CommunityId:  "community123",
	}

	transferID, err := storage.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	return transferID
}

// createTestRequest creates a request for testing.
func createTestRequest(t *testing.T, storage *storage.ProtoSQLStorage, requesterID, communityID string) string {
	request := &models.Request{
		RequesterId: requesterID,
		Description: "Test request",
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

	return requestID
}

// createTestUsers creates users for testing.
func createTestUsers(t *testing.T, storage *storage.ProtoSQLStorage, userID, name string) {
	user := &models.User{
		Id:   userID,
		Name: name,
	}

	_, err := storage.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
}

// createTestCommunity creates a community for testing and returns the community ID.
func createTestCommunity(t *testing.T, storage *storage.ProtoSQLStorage, creatorID string) string {
	community := &models.Community{
		Name:        "Test Community",
		Description: "A test community",
		CreatorId:   creatorID,
		OwnerUserId: creatorID,
	}

	communityID, err := storage.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	return communityID
}

// createTestCommunityMembership adds a user to a community.
func createTestCommunityMembership(t *testing.T, storage *storage.ProtoSQLStorage, communityID, userID string) {
	membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
		InviterId:   userID, // For simplicity, user invites themselves
	}

	_, err := storage.Insert(context.Background(), membership)
	if err != nil {
		t.Fatalf("Failed to create test community membership: %v", err)
	}
}
