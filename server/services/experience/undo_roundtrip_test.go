package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
	undotesting "go.ripls.org/ripls/server/undo/testing"
)

// TestUndoCompleteExperience_RoundTrip asserts that undoing an
// experience completion restores the full composite snapshot
// (experience + CommunityExperience rows + pre-action chat) to
// byte-equal pre-action state. Load-bearing correctness invariant
// per docs/ai/undo_plan.md §10.5.
func TestUndoCompleteExperience_RoundTrip(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ownerCtx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Round-trip Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	communityID := createTestCommunity(t, testStorage, "Round-trip Test Community", "user123")
	createTestCommunityMembership(t, testStorage, communityID, "user123")
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	if _, err := service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})); err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	// Look up the conversation id so we can snapshot chat messages.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, stored); err != nil {
		t.Fatalf("load experience: %v", err)
	}
	conversationID := stored.ConversationId
	preActionChatIDs := experienceChatMessageIDs(t, testStorage, conversationID)

	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "complete-experience",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotExperienceComposite(t, testStorage, expID, conversationID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
				ExperienceId: expID,
			}))
			if err != nil {
				t.Fatalf("CompleteExperience failed: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotExperienceComposite(t, testStorage, expID, conversationID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoCompleteExperience(ownerCtx, connect.NewRequest(&api.UndoCompleteExperienceRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoCompleteExperience failed: %v", err)
			}
		},
	})
}

// snapshotExperienceComposite loads the experience + every
// CommunityExperience row + pre-action chat messages into a composite
// snapshot. CommunityExperience is captured because CompleteExperience
// flips its Archived flag — a working undo must un-archive.
func snapshotExperienceComposite(t *testing.T, s *storage.ProtoSQLStorage, experienceID, conversationID string, preActionChatIDs []string) undotesting.Snapshot {
	t.Helper()
	ctx := context.Background()

	exp := &models.Experience{}
	if err := s.GetByID(ctx, experienceID, exp); err != nil {
		t.Fatalf("snapshot: load experience: %v", err)
	}

	entries := []undotesting.SnapshotEntry{
		{Label: "experience", Message: exp},
	}

	communityExperiences, err := s.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		t.Fatalf("snapshot: query community_experiences: %v", err)
	}
	for _, ce := range communityExperiences {
		m := ce.(*models.CommunityExperience)
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "community_experience:" + m.Id,
			Message: m,
		})
	}

	for _, msgID := range preActionChatIDs {
		msg := &models.ChatMessage{}
		if err := s.GetByID(ctx, msgID, msg); err != nil {
			entries = append(entries, undotesting.SnapshotEntry{
				Label:   "chat:" + msgID + " (missing)",
				Message: &models.ChatMessage{Id: msgID},
			})
			continue
		}
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "chat:" + msg.Id,
			Message: msg,
		})
	}
	return undotesting.Snapshot{Entries: entries}
}

// experienceChatMessageIDs returns the ids of every non-deleted chat
// message in a conversation.
func experienceChatMessageIDs(t *testing.T, s *storage.ProtoSQLStorage, conversationID string) []string {
	t.Helper()
	if conversationID == "" {
		return nil
	}
	msgs, err := s.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("chat message ids: query: %v", err)
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.(*models.ChatMessage).Id)
	}
	return ids
}
