package experience

import (
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// shareExperienceWithDescription creates an experience with the given
// description, shares it to communityID, and returns the experience ID and its
// conversation ID.
func shareExperienceWithDescription(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, ownerID, communityID, description string) (expID, conversationID string) {
	t.Helper()
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Trail Cleanup",
		Description: description,
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID = saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	adminCtx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	expStored := &models.Experience{}
	if err := testStorage.GetByID(adminCtx, expID, expStored); err != nil {
		t.Fatalf("failed to read experience: %v", err)
	}
	return expID, expStored.ConversationId
}

func TestShareExperience_SeedsDescriptionAsFirstComment(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-seed-desc"
	createTestUser(t, testStorage, ownerID, ownerID+"@example.com", "Owner")
	communityID := createTestCommunity(t, testStorage, "Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)

	const desc = "Meet at the north gate. Bring gloves."
	_, conversationID := shareExperienceWithDescription(t, service, testStorage, ownerID, communityID, desc)

	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	msgs, err := storage.ListByConversation(ctx, testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (anchor + description), got %d", len(msgs))
	}
	if !chat.IsCreationAnchorMessage(msgs[0]) {
		t.Errorf("first message should be the EXPERIENCE_CREATED anchor, got %+v", msgs[0])
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

func TestShareExperience_EmptyDescriptionSeedsNoComment(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-seed-empty"
	createTestUser(t, testStorage, ownerID, ownerID+"@example.com", "Owner")
	communityID := createTestCommunity(t, testStorage, "Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)

	_, conversationID := shareExperienceWithDescription(t, service, testStorage, ownerID, communityID, "")

	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	msgs, err := storage.ListByConversation(ctx, testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	for _, m := range msgs {
		if m.GetUserMessage() != nil {
			t.Errorf("expected no user comment for empty description, got %q", m.GetUserMessage().Text)
		}
	}
}

func TestShareExperience_ResharingDoesNotDuplicateComment(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-seed-reshare"
	createTestUser(t, testStorage, ownerID, ownerID+"@example.com", "Owner")
	communityA := createTestCommunity(t, testStorage, "Community A", ownerID)
	createTestCommunityMembership(t, testStorage, communityA, ownerID)
	communityB := createTestCommunity(t, testStorage, "Community B", ownerID)
	createTestCommunityMembership(t, testStorage, communityB, ownerID)

	const desc = "Shared into two communities."
	expID, conversationID := shareExperienceWithDescription(t, service, testStorage, ownerID, communityA, desc)

	// Re-share the same experience into a second community. The conversation is
	// per-experience, so the seeded comment must not be duplicated.
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)
	shareExperienceForTest(t, service, ownerCtx, expID, communityB)

	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	msgs, err := storage.ListByConversation(ctx, testStorage, conversationID, true, 0)
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
