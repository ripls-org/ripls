package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestRequireParticipant_NonParticipant(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Lender starts conversation
	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	resp, _ := svc.StartConversation(ctx, req)

	// Test that a non-participant is rejected
	err := svc.requireParticipant(context.Background(), resp.Msg.ConversationId, "stranger")
	if err == nil {
		t.Fatal("Expected error when checking non-participant")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", err)
	}
}

func TestRequireParticipant_Participant(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Lender starts conversation
	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	resp, _ := svc.StartConversation(ctx, req)

	// Test that participants are allowed
	err := svc.requireParticipant(context.Background(), resp.Msg.ConversationId, "lender1")
	if err != nil {
		t.Errorf("Expected no error for lender participant, got: %v", err)
	}

	err = svc.requireParticipant(context.Background(), resp.Msg.ConversationId, "borrower1")
	if err != nil {
		t.Errorf("Expected no error for borrower participant, got: %v", err)
	}
}

func TestRequireConversationAccess(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Set up shared community and participants.
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestCommunityMembership(t, sqlStorage, communityID, "community_member")

	// Start a transfer conversation with lender and borrower as participants.
	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	ctx := contextWithAuth("lender1", "lender@example.com")
	resp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	transferConversationID := resp.Msg.ConversationId

	// Insert a conversation with no community_id directly.
	conversationWithoutCommunity, err := sqlStorage.Insert(context.Background(), &models.ChatConversation{
		ParticipantIds: []string{"lender1", "borrower1"},
	})
	if err != nil {
		t.Fatalf("Insert conversation without community failed: %v", err)
	}

	tests := []struct {
		name           string
		conversationID string
		userID         string
		wantCode       connect.Code
		wantErr        bool
	}{
		{
			name:           "participant is allowed in transfer conversation",
			conversationID: transferConversationID,
			userID:         "lender1",
			wantErr:        false,
		},
		{
			name:           "other participant is allowed in transfer conversation",
			conversationID: transferConversationID,
			userID:         "borrower1",
			wantErr:        false,
		},
		// Transfer conversations are participant-only: community membership alone
		// is not sufficient. The preview-access semantic for GetConversation is
		// handled by that RPC's own auth, not by requireConversationAccess.
		{
			name:           "community member non-participant is denied for transfer conversation",
			conversationID: transferConversationID,
			userID:         "community_member",
			wantErr:        true,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "non-member is denied",
			conversationID: transferConversationID,
			userID:         "outsider",
			wantErr:        true,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "non-participant on conversation without community_id is denied",
			conversationID: conversationWithoutCommunity,
			userID:         "outsider",
			wantErr:        true,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "participant on conversation without community_id is allowed",
			conversationID: conversationWithoutCommunity,
			userID:         "lender1",
			wantErr:        false,
		},
		{
			name:           "nonexistent conversation returns not found",
			conversationID: "does-not-exist",
			userID:         "lender1",
			wantErr:        true,
			wantCode:       connect.CodeNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.requireConversationAccess(context.Background(), tc.conversationID, tc.userID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if connect.CodeOf(err) != tc.wantCode {
					t.Errorf("expected code %v, got %v (err: %v)", tc.wantCode, connect.CodeOf(err), err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	// Ex-member scenarios — these subtests share fresh storage to avoid
	// interference from the table-driven setup above.

	t.Run("ex-member with stale participant entry is denied for gear topic", func(t *testing.T) {
		freshSvc, freshStore := setupTestChatService(t)

		cid := createTestCommunity(t, freshStore, "owner")
		createTestCommunityMembership(t, freshStore, cid, "owner")
		createTestCommunityMembership(t, freshStore, cid, "exmember")

		gear := &models.Gear{OwnerId: "owner", Name: "tent"}
		gearID, _ := freshStore.Insert(context.Background(), gear)
		_, _ = freshStore.Insert(context.Background(), &models.CommunityGear{CommunityId: cid, GearId: gearID})

		// Insert conversation with exmember already in ParticipantIds.
		convID, _ := freshStore.Insert(context.Background(), &models.ChatConversation{
			CommunityId:    cid,
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
			ParticipantIds: []string{"owner", "exmember"},
		})

		// exmember leaves: soft-delete their membership.
		softDeleteMembership(t, freshStore, cid, "exmember")

		_, err := freshSvc.requireConversationAccess(context.Background(), convID, "exmember")
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("ex-member with stale participant entry is denied for experience topic", func(t *testing.T) {
		freshSvc, freshStore := setupTestChatService(t)

		cid := createTestCommunity(t, freshStore, "owner2")
		createTestCommunityMembership(t, freshStore, cid, "owner2")
		createTestCommunityMembership(t, freshStore, cid, "exmember2")

		expID, _ := freshStore.Insert(context.Background(), &models.Experience{Name: "hike"})
		_, _ = freshStore.Insert(context.Background(), &models.CommunityExperience{CommunityId: cid, ExperienceId: expID})

		convID, _ := freshStore.Insert(context.Background(), &models.ChatConversation{
			CommunityId:    cid,
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: expID}},
			ParticipantIds: []string{"owner2", "exmember2"},
		})

		softDeleteMembership(t, freshStore, cid, "exmember2")

		_, err := freshSvc.requireConversationAccess(context.Background(), convID, "exmember2")
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("ex-member with stale participant entry is denied for request topic", func(t *testing.T) {
		freshSvc, freshStore := setupTestChatService(t)

		cid := createTestCommunity(t, freshStore, "owner3")
		createTestCommunityMembership(t, freshStore, cid, "owner3")
		createTestCommunityMembership(t, freshStore, cid, "exmember3")

		reqID, _ := freshStore.Insert(context.Background(), &models.Request{RequesterId: "owner3", Title: "drill"})
		_, _ = freshStore.Insert(context.Background(), &models.CommunityRequest{CommunityId: cid, RequestId: reqID})

		convID, _ := freshStore.Insert(context.Background(), &models.ChatConversation{
			CommunityId:    cid,
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: reqID}},
			ParticipantIds: []string{"owner3", "exmember3"},
		})

		softDeleteMembership(t, freshStore, cid, "exmember3")

		_, err := freshSvc.requireConversationAccess(context.Background(), convID, "exmember3")
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("participant in transfer conversation is still allowed after leaving community", func(t *testing.T) {
		freshSvc, freshStore := setupTestChatService(t)

		cid := createTestCommunity(t, freshStore, "lender2")
		createTestCommunityMembership(t, freshStore, cid, "lender2")
		createTestCommunityMembership(t, freshStore, cid, "borrower2")

		transferID := createTestLoan(t, freshStore, "lender2", "borrower2")
		ctx2 := contextWithAuth("lender2", "lender2@example.com")
		resp2, err := freshSvc.StartConversation(ctx2, connect.NewRequest(&api.StartConversationRequest{
			CommunityId: cid,
			Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
		}))
		if err != nil {
			t.Fatalf("StartConversation: %v", err)
		}
		convID := resp2.Msg.ConversationId

		// lender2 leaves the community.
		softDeleteMembership(t, freshStore, cid, "lender2")

		// Transfer-conversation participant must still be allowed (escape hatch).
		_, err = freshSvc.requireConversationAccess(context.Background(), convID, "lender2")
		if err != nil {
			t.Errorf("transfer participant should be allowed after leaving; got: %v", err)
		}
	})

	t.Run("ex-member of one community remains allowed when still member of another shared community", func(t *testing.T) {
		freshSvc, freshStore := setupTestChatService(t)

		cid1 := createTestCommunity(t, freshStore, "owner4")
		cid2 := createTestCommunity(t, freshStore, "owner4")
		createTestCommunityMembership(t, freshStore, cid1, "owner4")
		createTestCommunityMembership(t, freshStore, cid2, "owner4")
		createTestCommunityMembership(t, freshStore, cid1, "multimember")
		createTestCommunityMembership(t, freshStore, cid2, "multimember")

		// Gear shared in both communities.
		gearID, _ := freshStore.Insert(context.Background(), &models.Gear{OwnerId: "owner4", Name: "kayak"})
		_, _ = freshStore.Insert(context.Background(), &models.CommunityGear{CommunityId: cid1, GearId: gearID})
		_, _ = freshStore.Insert(context.Background(), &models.CommunityGear{CommunityId: cid2, GearId: gearID})

		convID, _ := freshStore.Insert(context.Background(), &models.ChatConversation{
			CommunityId:    cid1,
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
			ParticipantIds: []string{"owner4", "multimember"},
		})

		// multimember leaves cid1 but is still in cid2.
		softDeleteMembership(t, freshStore, cid1, "multimember")

		_, err := freshSvc.requireConversationAccess(context.Background(), convID, "multimember")
		if err != nil {
			t.Errorf("member of remaining shared community should be allowed; got: %v", err)
		}
	})
}

// TestRequireConversationAccess_QueryCount pins the number of DB round-trips
// for a gear-topic conversation with two shared communities:
// 1 (GetByID conversation) + 1 (collectSharedCommunityIDs via CommunityGear)
// + 1 (GetCommunitiesWithMembership batched) = 3 total.
func TestRequireConversationAccess_QueryCount(t *testing.T) {
	freshSvc, freshStore := setupTestChatService(t)

	cid1 := createTestCommunity(t, freshStore, "owner5")
	cid2 := createTestCommunity(t, freshStore, "owner5b")
	createTestCommunityMembership(t, freshStore, cid1, "owner5")
	createTestCommunityMembership(t, freshStore, cid1, "member5")

	gearID, _ := freshStore.Insert(context.Background(), &models.Gear{OwnerId: "owner5", Name: "tent5"})
	_, _ = freshStore.Insert(context.Background(), &models.CommunityGear{CommunityId: cid1, GearId: gearID})
	_, _ = freshStore.Insert(context.Background(), &models.CommunityGear{CommunityId: cid2, GearId: gearID})

	convID, _ := freshStore.Insert(context.Background(), &models.ChatConversation{
		CommunityId:    cid1,
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{"owner5", "member5"},
	})

	statsCtx := storage.WithQueryStats(context.Background())
	storage.AssertMaxQueries(t, statsCtx, 3, func() {
		if _, err := freshSvc.requireConversationAccess(statsCtx, convID, "member5"); err != nil {
			t.Fatalf("requireConversationAccess: %v", err)
		}
	})
}

// softDeleteMembership simulates a user leaving a community by soft-deleting
// their CommunityUser row, mirroring what LeaveCommunity does.
func softDeleteMembership(t *testing.T, store *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	rows, err := store.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"user_id":      userID,
	}, &models.CommunityUser{})
	if err != nil || len(rows) == 0 {
		t.Fatalf("membership not found for %s in %s: %v", userID, communityID, err)
	}
	cu := rows[0].(*models.CommunityUser)
	cu.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  userID,
		DeletedAtUnixSec: 1, // any positive value satisfies the > 0 check
	}
	if err := store.Update(context.Background(), cu); err != nil {
		t.Fatalf("soft-delete membership: %v", err)
	}
}
