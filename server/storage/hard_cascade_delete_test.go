package storage

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// hardDeleteHelper is the (table, helper) pair shared by all the
// straightforward DELETE-WHERE-community_id helpers. The chat-
// message helper has its own dedicated test because it queries
// chat_conversation first.
type hardDeleteCase struct {
	name      string
	tableName string
	insert    func(t *testing.T, store *ProtoSQLStorage, communityID string)
	helper    func(ctx context.Context, store *ProtoSQLStorage, communityID string) (int64, error)
}

func hardDeleteCases() []hardDeleteCase {
	return []hardDeleteCase{
		{
			name:      "community_user",
			tableName: "community_user",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityUser{CommunityId: cid, UserId: "u-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityUserByCommunityID,
		},
		{
			name:      "community_gear",
			tableName: "community_gear",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityGear{CommunityId: cid, GearId: "g-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityGearByCommunityID,
		},
		{
			name:      "community_request",
			tableName: "community_request",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityRequest{CommunityId: cid, RequestId: "r-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityRequestByCommunityID,
		},
		{
			name:      "community_experience",
			tableName: "community_experience",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityExperience{CommunityId: cid, ExperienceId: "e-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityExperienceByCommunityID,
		},
		{
			name:      "community_notification_preferences",
			tableName: "community_notification_preferences",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityNotificationPreferences{CommunityId: cid, UserId: "u-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityNotificationPreferencesByCommunityID,
		},
		{
			name:      "community_invitation_link",
			tableName: "community_invitation_link",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityInvitationLink{CommunityId: cid, ShortCode: "code-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityInvitationLinkByCommunityID,
		},
		{
			name:      "community_event",
			tableName: "community_event",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityEvent{CommunityId: cid, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityEventByCommunityID,
		},
		{
			name:      "community_region",
			tableName: "community_region",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.CommunityRegion{CommunityId: cid, RegionId: "region-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteCommunityRegionByCommunityID,
		},
		{
			name:      "Story",
			tableName: "Story",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.Story{CommunityId: cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteStoriesByCommunityID,
		},
		{
			name:      "stored_nudge",
			tableName: "stored_nudge",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.StoredNudge{CommunityId: cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteStoredNudgesByCommunityID,
		},
		{
			name:      "FeedItemView",
			tableName: "FeedItemView",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.FeedItemView{CommunityId: cid, UserId: "u-" + cid, FeedItemId: "fi-" + cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteFeedItemViewsByCommunityID,
		},
		{
			name:      "chat_conversation",
			tableName: "chat_conversation",
			insert: func(t *testing.T, s *ProtoSQLStorage, cid string) {
				if _, err := s.Insert(context.Background(), &models.ChatConversation{CommunityId: cid}); err != nil {
					t.Fatalf("insert: %v", err)
				}
			},
			helper: HardDeleteChatConversationsByCommunityID,
		},
	}
}

// TestHardDeleteHelpers exercises every per-table helper through
// a single shape: insert one row scoped to communityA + one scoped
// to communityB; run the helper for communityA; assert
// rowsAffected == 1 and the communityB row remains.
func TestHardDeleteHelpers(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	for _, tc := range hardDeleteCases() {
		t.Run(tc.name, func(t *testing.T) {
			communityA := "comm-A-" + tc.name
			communityB := "comm-B-" + tc.name
			tc.insert(t, store, communityA)
			tc.insert(t, store, communityB)

			rows, err := tc.helper(ctx, store, communityA)
			if err != nil {
				t.Fatalf("helper failed: %v", err)
			}
			if rows != 1 {
				t.Errorf("rowsAffected = %d, want 1", rows)
			}

			// Helper is idempotent: a second call against the same
			// (now empty) community returns 0 rows, no error.
			rows, err = tc.helper(ctx, store, communityA)
			if err != nil {
				t.Fatalf("second call failed: %v", err)
			}
			if rows != 0 {
				t.Errorf("second call rowsAffected = %d, want 0", rows)
			}

			// Off-scope row for communityB must remain.
			remaining, err := store.QueryByField(ctx, "community_id", communityB, protoForTable(t, tc.tableName), QueryOptions{IncludeDeleted: true})
			if err != nil {
				t.Fatalf("query communityB: %v", err)
			}
			if len(remaining) != 1 {
				t.Errorf("communityB rows = %d, want 1 (off-scope row should survive)", len(remaining))
			}
		})
	}
}

// protoForTable returns a fresh empty proto matching the table
// under test, used to verify off-scope rows survived the cascade.
func protoForTable(t *testing.T, table string) proto.Message {
	t.Helper()
	switch table {
	case "community_user":
		return &models.CommunityUser{}
	case "community_gear":
		return &models.CommunityGear{}
	case "community_request":
		return &models.CommunityRequest{}
	case "community_experience":
		return &models.CommunityExperience{}
	case "community_notification_preferences":
		return &models.CommunityNotificationPreferences{}
	case "community_invitation_link":
		return &models.CommunityInvitationLink{}
	case "community_event":
		return &models.CommunityEvent{}
	case "community_region":
		return &models.CommunityRegion{}
	case "Story":
		return &models.Story{}
	case "stored_nudge":
		return &models.StoredNudge{}
	case "FeedItemView":
		return &models.FeedItemView{}
	case "chat_conversation":
		return &models.ChatConversation{}
	default:
		t.Fatalf("protoForTable: unknown table %q", table)
		return nil
	}
}

func TestHardDeleteChatMessagesByCommunityID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityA := "comm-A"
	communityB := "comm-B"

	convA, err := store.Insert(ctx, &models.ChatConversation{CommunityId: communityA})
	if err != nil {
		t.Fatalf("insert convA: %v", err)
	}
	convB, err := store.Insert(ctx, &models.ChatConversation{CommunityId: communityB})
	if err != nil {
		t.Fatalf("insert convB: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := store.Insert(ctx, &models.ChatMessage{ConversationId: convA}); err != nil {
			t.Fatalf("insert msg in convA: %v", err)
		}
	}
	if _, err := store.Insert(ctx, &models.ChatMessage{ConversationId: convB}); err != nil {
		t.Fatalf("insert msg in convB: %v", err)
	}

	rows, err := HardDeleteChatMessagesByCommunityID(ctx, store, communityA)
	if err != nil {
		t.Fatalf("helper failed: %v", err)
	}
	if rows != 3 {
		t.Errorf("rowsAffected = %d, want 3", rows)
	}

	// communityB's message must remain.
	remaining, err := store.QueryByField(ctx, "conversation_id", convB, &models.ChatMessage{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query communityB messages: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("communityB messages = %d, want 1", len(remaining))
	}

	// Helper is a no-op when the community has no conversations
	// (idempotent on second run after conversations have been
	// hard-deleted).
	if _, err := HardDeleteChatConversationsByCommunityID(ctx, store, communityA); err != nil {
		t.Fatalf("conv cleanup: %v", err)
	}
	rows, err = HardDeleteChatMessagesByCommunityID(ctx, store, communityA)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if rows != 0 {
		t.Errorf("second call rowsAffected = %d, want 0", rows)
	}
}

func TestHardDeleteCommunityByID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	idA, err := store.Insert(ctx, &models.Community{Name: "A", Description: "first", OwnerUserId: "owner-a"})
	if err != nil {
		t.Fatalf("insert A: %v", err)
	}
	idB, err := store.Insert(ctx, &models.Community{Name: "B", Description: "second", OwnerUserId: "owner-b"})
	if err != nil {
		t.Fatalf("insert B: %v", err)
	}

	rows, err := HardDeleteCommunityByID(ctx, store, idA)
	if err != nil {
		t.Fatalf("helper: %v", err)
	}
	if rows != 1 {
		t.Errorf("rowsAffected = %d, want 1", rows)
	}

	got := &models.Community{}
	err = store.GetByID(ctx, idA, got, QueryOptions{IncludeDeleted: true})
	if err == nil {
		t.Errorf("expected GetByID(idA) to fail after hard-delete, got %+v", got)
	}

	// Off-scope community survives.
	got = &models.Community{}
	if err := store.GetByID(ctx, idB, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("expected idB to survive: %v", err)
	}
}

// TestHardDeleteFullCascade exercises every helper in the order
// the purge job will use, against a single community fixture.
// Pins that no helper is missed and the final state is fully
// drained.
func TestHardDeleteFullCascade(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityID, err := store.Insert(ctx, &models.Community{Name: "purge-me", OwnerUserId: "owner-x"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}

	// One row in every cascade table.
	for _, tc := range hardDeleteCases() {
		tc.insert(t, store, communityID)
	}
	// Also one chat message scoped to the community via its
	// conversation.
	convs, err := store.QueryByField(ctx, "community_id", communityID, &models.ChatConversation{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query convs: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convs))
	}
	convID := convs[0].(*models.ChatConversation).Id
	if _, err := store.Insert(ctx, &models.ChatMessage{ConversationId: convID}); err != nil {
		t.Fatalf("insert chat message: %v", err)
	}

	// Run helpers in child-before-parent order.
	if _, err := HardDeleteChatMessagesByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("messages: %v", err)
	}
	if _, err := HardDeleteChatConversationsByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if _, err := HardDeleteStoriesByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("stories: %v", err)
	}
	if _, err := HardDeleteStoredNudgesByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("nudges: %v", err)
	}
	if _, err := HardDeleteFeedItemViewsByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("feed item views: %v", err)
	}
	if _, err := HardDeleteCommunityUserByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("users: %v", err)
	}
	if _, err := HardDeleteCommunityGearByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("gear: %v", err)
	}
	if _, err := HardDeleteCommunityRequestByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("requests: %v", err)
	}
	if _, err := HardDeleteCommunityExperienceByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("experiences: %v", err)
	}
	if _, err := HardDeleteCommunityNotificationPreferencesByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("notif prefs: %v", err)
	}
	if _, err := HardDeleteCommunityInvitationLinkByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("invite links: %v", err)
	}
	if _, err := HardDeleteCommunityEventByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("events: %v", err)
	}
	if _, err := HardDeleteCommunityRegionByCommunityID(ctx, store, communityID); err != nil {
		t.Fatalf("regions: %v", err)
	}
	if _, err := HardDeleteCommunityByID(ctx, store, communityID); err != nil {
		t.Fatalf("community: %v", err)
	}

	// Every cascade table should now be empty for this community.
	for _, tc := range hardDeleteCases() {
		remaining, err := store.QueryByField(ctx, "community_id", communityID, protoForTable(t, tc.tableName), QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("post-cascade query %s: %v", tc.tableName, err)
		}
		if len(remaining) != 0 {
			t.Errorf("table %s: %d rows remain after cascade, want 0", tc.tableName, len(remaining))
		}
	}

	// chat_message scoped via the (now-deleted) conversation
	// should also be empty.
	msgs, err := store.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("post-cascade chat_message query: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("chat_message: %d rows remain, want 0", len(msgs))
	}

	// Community itself is gone.
	got := &models.Community{}
	if err := store.GetByID(ctx, communityID, got, QueryOptions{IncludeDeleted: true}); err == nil {
		t.Errorf("community %s still exists after HardDeleteCommunityByID", communityID)
	}
}
