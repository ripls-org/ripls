package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestVerifyExperienceViewer covers the view-access check gating link-only
// ShareItem calls (#2630): the owner and any member of a community the
// experience is shared with pass (and get the owner ID back); everyone else is
// denied.
func TestVerifyExperienceViewer(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	createTestUser(t, testStorage, "owner1", "owner1@example.com", "Owner")
	createTestUser(t, testStorage, "member1", "member1@example.com", "Member")
	createTestUser(t, testStorage, "stranger1", "stranger1@example.com", "Stranger")

	expID, err := testStorage.Insert(ctx, &models.Experience{OwnerId: "owner1", Name: "Picnic"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	communityID, err := testStorage.Insert(ctx, &models.Community{CreatorId: "owner1", OwnerUserId: "owner1"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, uid := range []string{"owner1", "member1"} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: uid}); err != nil {
			t.Fatalf("insert membership for %s: %v", uid, err)
		}
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityExperience{CommunityId: communityID, ExperienceId: expID, SharedAtUnixSec: 1}); err != nil {
		t.Fatalf("insert community experience: %v", err)
	}

	t.Run("owner passes and gets owner id", func(t *testing.T) {
		ownerID, err := service.VerifyExperienceViewer(ctx, expID, "owner1")
		if err != nil || ownerID != "owner1" {
			t.Errorf("got (%q, %v), want (owner1, nil)", ownerID, err)
		}
	})

	t.Run("shared-community member passes and gets owner id", func(t *testing.T) {
		ownerID, err := service.VerifyExperienceViewer(ctx, expID, "member1")
		if err != nil || ownerID != "owner1" {
			t.Errorf("got (%q, %v), want (owner1, nil)", ownerID, err)
		}
	})

	t.Run("non-member is denied", func(t *testing.T) {
		_, err := service.VerifyExperienceViewer(ctx, expID, "stranger1")
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("missing experience is NotFound", func(t *testing.T) {
		_, err := service.VerifyExperienceViewer(ctx, "does-not-exist", "owner1")
		if err == nil || connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})
}

// TestService_ShareExperienceToCommunity covers the sharing core that
// CommunityService.ShareItem drives: the CommunityExperience junction, the
// per-item conversation being created once and reused, and the owner check the
// ItemSharer hook applies before the core runs.
func TestService_ShareExperienceToCommunity(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   "user123",
		OwnerUserId: "user123",
	}
	communityID, err := testStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Add user as member
	membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "user123",
	}
	_, err = testStorage.Insert(ctx, membership)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Test Experience",
		Description: "A test experience",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	// The experience is born in its own per-item community (#2492): its
	// conversation is created there and reused on subsequent explicit shares.
	itemCommunityID := createResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("expected SaveExperience to return ItemCommunityId")
	}

	t.Run("successful share creates CommunityExperience with conversation", func(t *testing.T) {
		shareExperienceForTest(t, service, ctx, expID, communityID)

		// Verify CommunityExperience record was created
		queryFields := map[string]any{
			"community_id":  communityID,
			"experience_id": expID,
		}
		shares, err := testStorage.QueryByFields(ctx, queryFields, &models.CommunityExperience{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Errorf("Expected 1 share, got %d", len(shares))
		}

		// Verify conversation was created and stored in Experience
		expStored := &models.Experience{}
		if err := testStorage.GetByID(ctx, expID, expStored); err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}
		if expStored.ConversationId == "" {
			t.Error("Expected conversation_id to be set in Experience")
		}

		// Verify conversation exists in database
		conversation := &models.ChatConversation{}
		err = testStorage.GetByID(ctx, expStored.ConversationId, conversation)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		// The conversation was created when the experience was born in its
		// per-item community (#2492) and is reused on this explicit share, so it
		// remains scoped to the per-item community, not communityID.
		if conversation.CommunityId != itemCommunityID {
			t.Errorf("Expected conversation community_id=%s (per-item), got %s", itemCommunityID, conversation.CommunityId)
		}

		// Verify conversation topic is experience_id
		if conversation.GetTopic().GetExperienceId() != expID {
			t.Errorf("Expected conversation experience_id=%s, got %s", expID, conversation.GetTopic().GetExperienceId())
		}

		// Verify owner was added as participant
		if len(conversation.ParticipantIds) != 1 {
			t.Errorf("Expected 1 participant (owner), got %d", len(conversation.ParticipantIds))
		}

		if conversation.ParticipantIds[0] != "user123" {
			t.Errorf("Expected owner user123 as participant, got %s", conversation.ParticipantIds[0])
		}
	})

	t.Run("share to multiple communities reuses single conversation", func(t *testing.T) {
		// Create second community
		community2 := &models.Community{
			Name:        "Second Community",
			CreatorId:   "user123",
			OwnerUserId: "user123",
		}
		communityID2, err := testStorage.Insert(ctx, community2)
		if err != nil {
			t.Fatalf("Failed to create second community: %v", err)
		}

		// Add user as member of second community
		membership2 := &models.CommunityUser{
			CommunityId: communityID2,
			UserId:      "user123",
		}
		_, err = testStorage.Insert(ctx, membership2)
		if err != nil {
			t.Fatalf("Failed to create second membership: %v", err)
		}

		// Get the conversation ID set by the first share.
		expBefore := &models.Experience{}
		if err := testStorage.GetByID(ctx, expID, expBefore); err != nil {
			t.Fatalf("Failed to get experience before second share: %v", err)
		}
		firstConvID := expBefore.ConversationId
		if firstConvID == "" {
			t.Fatal("Expected conversation_id to be set after first share")
		}

		// Share to second community — should reuse the existing conversation.
		shareExperienceForTest(t, service, ctx, expID, communityID2)

		// Verify that Experience.conversation_id is unchanged (same conversation reused).
		expAfter := &models.Experience{}
		if err := testStorage.GetByID(ctx, expID, expAfter); err != nil {
			t.Fatalf("Failed to get experience after second share: %v", err)
		}
		if expAfter.ConversationId != firstConvID {
			t.Errorf("Expected conversation_id to remain %s after sharing to second community, got %s",
				firstConvID, expAfter.ConversationId)
		}

		// Three CommunityExperience records exist: the per-item community the
		// experience was born into (#2492) plus communityID and communityID2.
		shares, err := testStorage.QueryByField(ctx, "experience_id", expID, &models.CommunityExperience{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}
		if len(shares) != 3 {
			t.Fatalf("Expected 3 shares, got %d", len(shares))
		}
	})

	// Audience mutation is owner-only, enforced by ShareItem through the
	// VerifyExperienceOwner hook before the sharing core runs — so that hook is
	// where the check is asserted.
	t.Run("share by non-owner fails", func(t *testing.T) {
		createTestUser(t, testStorage, "user456", "user456@example.com", "User 456")
		ctx456 := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

		err := service.VerifyExperienceOwner(ctx456, expID, "user456")
		if err == nil {
			t.Fatal("Expected error when non-owner tries to share")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})
}

// TestService_ShareExperience_OwnerAutoRSVP verifies that sharing an experience
// auto-RSVPs the owner as YES exactly once, even when shared to multiple communities.
func TestService_ShareExperience_OwnerAutoRSVP(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")

	ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")
	createTestCommunityMembership(t, testStorage, communityA, "owner1")
	createTestCommunityMembership(t, testStorage, communityB, "owner1")

	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Auto-RSVP Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to both communities.
	for _, cid := range []string{communityA, communityB} {
		shareExperienceForTest(t, service, ctx, expID, cid)
	}

	// Owner must have exactly one YES RSVP across all communities.
	allRSVPs, err := testStorage.QueryByFields(ctx, map[string]any{
		"experience_id": expID,
		"user_id":       "owner1",
	}, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("QueryByFields failed: %v", err)
	}
	if len(allRSVPs) != 1 {
		t.Errorf("expected exactly 1 owner RSVP across all communities, got %d", len(allRSVPs))
	}
	if rsvp := allRSVPs[0].(*models.ExperienceRSVP); rsvp.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
		t.Errorf("expected owner RSVP intention YES, got %v", rsvp.GetIntention())
	}
}
