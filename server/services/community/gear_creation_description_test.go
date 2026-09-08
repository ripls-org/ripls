package community

import (
	"testing"

	"connectrpc.com/connect"

	chatlib "go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// shareGearWithDescription creates gear with the given description, shares it to
// communityID with the given availability, and returns the gear's conversation ID.
func shareGearWithDescription(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, ownerID, communityID, description string, availability api.Availability) (gearID, conversationID string) {
	t.Helper()
	ctxOwner := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	gear := &models.Gear{
		Name:        "Extension Ladder",
		Description: description,
		OwnerId:     ownerID,
		State:       models.GearState_GEAR_STATE_AVAILABLE,
	}
	id, err := testStorage.Insert(ctxOwner, gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	gearID = id

	shareGearForTestWithAvailability(
		t, service, ctxOwner, gearID, communityID, apiAvailabilityToModel(availability),
	)

	gearStored := &models.Gear{}
	if err := testStorage.GetByID(ctxOwner, gearID, gearStored); err != nil {
		t.Fatalf("failed to reload gear: %v", err)
	}
	return gearID, gearStored.ConversationId
}

func TestService_ShareGear_SeedsDescriptionAsFirstComment(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	service.SetSystemMessageWriter(chatlib.NewSystemMessageWriter(testStorage, nil))

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	resp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{Name: "Tool Library"}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := resp.Msg.Id

	const desc = "Sturdy 24ft ladder, fiberglass."
	_, conversationID := shareGearWithDescription(t, service, testStorage, ownerID, communityID, desc, api.Availability_AVAILABILITY_FOR_LOAN)

	msgs, err := storage.ListByConversation(ctxOwner, testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (anchor + description), got %d", len(msgs))
	}
	if !chatlib.IsCreationAnchorMessage(msgs[0]) {
		t.Errorf("first message should be the LOAN_SHARED anchor, got %+v", msgs[0])
	}
	user := msgs[1].GetUserMessage()
	if user == nil {
		t.Fatalf("second message should be the description user comment, got %+v", msgs[1])
	}
	if user.SenderId != ownerID {
		t.Errorf("comment sender = %q, want owner %q", user.SenderId, ownerID)
	}
	if user.Text != desc {
		t.Errorf("comment text = %q, want %q", user.Text, desc)
	}
}

func TestService_ShareGear_EmptyDescriptionSeedsNoComment(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	service.SetSystemMessageWriter(chatlib.NewSystemMessageWriter(testStorage, nil))

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	resp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{Name: "Tool Library"}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := resp.Msg.Id

	_, conversationID := shareGearWithDescription(t, service, testStorage, ownerID, communityID, "", api.Availability_AVAILABILITY_FOR_GIVEAWAY)

	msgs, err := storage.ListByConversation(ctxOwner, testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	for _, m := range msgs {
		if m.GetUserMessage() != nil {
			t.Errorf("expected no user comment for empty description, got %q", m.GetUserMessage().Text)
		}
	}
}

func TestService_ShareGear_ResharingDoesNotDuplicateComment(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	service.SetSystemMessageWriter(chatlib.NewSystemMessageWriter(testStorage, nil))

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	createCommunity := func(name string) string {
		t.Helper()
		resp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{Name: name}))
		if err != nil {
			t.Fatalf("CreateCommunity(%q) failed: %v", name, err)
		}
		return resp.Msg.Id
	}
	communityA := createCommunity("Community A")
	communityB := createCommunity("Community B")

	const desc = "Shared into two communities."
	gearID, conversationID := shareGearWithDescription(t, service, testStorage, ownerID, communityA, desc, api.Availability_AVAILABILITY_FOR_LOAN)

	// Re-share the same gear into a second community. The conversation is
	// per-gear, so the seeded comment must not be duplicated.
	shareGearForTestWithAvailability(
		t, service, ctxOwner, gearID, communityB, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	msgs, err := storage.ListByConversation(ctxOwner, testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	commentCount := 0
	for _, m := range msgs {
		if u := m.GetUserMessage(); u != nil && u.Text == desc {
			commentCount++
		}
	}
	if commentCount != 1 {
		t.Errorf("expected exactly 1 seeded comment after re-share, got %d", commentCount)
	}
}
