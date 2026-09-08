package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_RSVPToExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create owner user
	ownerUser := &models.User{
		Id:    "owner123",
		Email: "owner@example.com",
		Name:  "Owner User",
	}
	_, err := testStorage.Insert(ownerCtx, ownerUser)
	if err != nil {
		t.Fatalf("Failed to insert owner user: %v", err)
	}

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	_, err = testStorage.Insert(ownerCtx, community)
	if err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}
	communityID := community.Id

	// Add owner and user to community
	ownerMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "owner123",
	}
	_, err = testStorage.Insert(ownerCtx, ownerMembership)
	if err != nil {
		t.Fatalf("Failed to add owner to community: %v", err)
	}

	userMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "user456",
	}
	_, err = testStorage.Insert(ownerCtx, userMembership)
	if err != nil {
		t.Fatalf("Failed to add user to community: %v", err)
	}

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:            "RSVP Test Experience",
		Description:     "Test RSVP functionality",
		MaxParticipants: 5,
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	t.Run("successful RSVP creates record", func(t *testing.T) {
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})

		_, err := service.RSVPToExperience(userCtx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}

		// Verify RSVP record was created
		queryFields := map[string]any{
			"experience_id": expID,
			"user_id":       "user456",
		}
		rsvps, err := testStorage.QueryByFields(userCtx, queryFields, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("Failed to query RSVPs: %v", err)
		}

		if len(rsvps) != 1 {
			t.Fatalf("Expected 1 RSVP, got %d", len(rsvps))
		}

		rsvp := rsvps[0].(*models.ExperienceRSVP)
		if rsvp.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
			t.Errorf("Expected intention YES, got %v", rsvp.GetIntention())
		}
	})

	t.Run("update existing RSVP", func(t *testing.T) {
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		})

		_, err := service.RSVPToExperience(userCtx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}

		// Verify RSVP was updated
		queryFields := map[string]any{
			"experience_id": expID,
			"user_id":       "user456",
		}
		rsvps, err := testStorage.QueryByFields(userCtx, queryFields, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("Failed to query RSVPs: %v", err)
		}

		if len(rsvps) != 1 {
			t.Fatalf("Expected 1 RSVP, got %d (should not duplicate)", len(rsvps))
		}

		rsvp := rsvps[0].(*models.ExperienceRSVP)
		if rsvp.GetIntention() != models.RSVPIntention_RSVP_INTENTION_NO {
			t.Errorf("Expected intention NO, got %v", rsvp.GetIntention())
		}
	})
}

func TestService_RSVPToExperience_ConversationParticipants(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "User 456")
	createTestUser(t, testStorage, "user789", "user789@example.com", "User 789")

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	user456Ctx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)
	user789Ctx := createAuthenticatedContext("user789", "user789@example.com", models.Role_ROLE_USER)

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   "owner123",
		OwnerUserId: "owner123",
	}
	communityID, err := testStorage.Insert(ownerCtx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Add all users as community members
	for _, userID := range []string{"owner123", "user456", "user789"} {
		membership := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      userID,
		}
		_, err = testStorage.Insert(ownerCtx, membership)
		if err != nil {
			t.Fatalf("Failed to create membership for %s: %v", userID, err)
		}
	}

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Test Experience",
		Description: "Test conversation participants",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share experience to community (creates conversation with owner as participant)
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Get the conversation ID from the Experience record.
	expStored := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, expStored); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	conversationID := expStored.ConversationId

	t.Run("RSVP Yes adds user to conversation participants", func(t *testing.T) {
		// User456 RSVPs Yes
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})
		_, err := service.RSVPToExperience(user456Ctx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVP failed: %v", err)
		}

		// Verify user456 was added to conversation
		conversation := &models.ChatConversation{}
		err = testStorage.GetByID(ownerCtx, conversationID, conversation)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		// Should have owner123 and user456
		if len(conversation.ParticipantIds) != 2 {
			t.Errorf("Expected 2 participants, got %d", len(conversation.ParticipantIds))
		}

		hasUser456 := false
		for _, pid := range conversation.ParticipantIds {
			if pid == "user456" {
				hasUser456 = true
			}
		}
		if !hasUser456 {
			t.Error("user456 should be in conversation participants after RSVP Yes")
		}
	})

	t.Run("RSVP Maybe adds user to conversation participants", func(t *testing.T) {
		// User789 RSVPs Maybe
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
		})
		_, err := service.RSVPToExperience(user789Ctx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVP failed: %v", err)
		}

		// Verify user789 was added to conversation
		conversation := &models.ChatConversation{}
		err = testStorage.GetByID(ownerCtx, conversationID, conversation)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		// Should have owner123, user456, and user789
		if len(conversation.ParticipantIds) != 3 {
			t.Errorf("Expected 3 participants, got %d", len(conversation.ParticipantIds))
		}

		hasUser789 := false
		for _, pid := range conversation.ParticipantIds {
			if pid == "user789" {
				hasUser789 = true
			}
		}
		if !hasUser789 {
			t.Error("user789 should be in conversation participants after RSVP Maybe")
		}
	})

	t.Run("RSVP to multi-community experience adds to all conversations", func(t *testing.T) {
		// Create second community
		community2 := &models.Community{
			Name:        "Second Community",
			CreatorId:   "owner123",
			OwnerUserId: "owner123",
		}
		communityID2, err := testStorage.Insert(ownerCtx, community2)
		if err != nil {
			t.Fatalf("Failed to create second community: %v", err)
		}

		// Add users as members
		for _, userID := range []string{"owner123", "user456"} {
			membership := &models.CommunityUser{
				CommunityId: communityID2,
				UserId:      userID,
			}
			_, err = testStorage.Insert(ownerCtx, membership)
			if err != nil {
				t.Fatalf("Failed to create membership for %s: %v", userID, err)
			}
		}

		// Create new experience
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name: "Multi-Community Experience",
		})
		createResp, err := service.SaveExperience(ownerCtx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}
		expID2 := createResp.Msg.Experience.Id

		// Share to both communities
		shareExperienceForTest(t, service, ownerCtx, expID2, communityID)
		shareExperienceForTest(t, service, ownerCtx, expID2, communityID2)

		// User456 RSVPs Yes to community 1
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID2,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})
		_, err = service.RSVPToExperience(user456Ctx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVP failed: %v", err)
		}

		// Get the single experience conversation.
		expStored2 := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID2, expStored2); err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}
		if expStored2.ConversationId == "" {
			t.Fatal("Expected experience to have a conversation_id")
		}

		// Verify user456 is participant in the experience conversation.
		conversation := &models.ChatConversation{}
		if err := testStorage.GetByID(ownerCtx, expStored2.ConversationId, conversation); err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		hasUser456 := false
		for _, pid := range conversation.ParticipantIds {
			if pid == "user456" {
				hasUser456 = true
			}
		}
		if !hasUser456 {
			t.Error("user456 should be participant in experience conversation")
		}
	})
}

func TestService_RSVPToExperience_StateReversion(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	_, err := testStorage.Insert(ownerCtx, community)
	if err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}
	communityID := community.Id

	// Add owner and users to community
	ownerMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "owner123",
	}
	_, err = testStorage.Insert(ownerCtx, ownerMembership)
	if err != nil {
		t.Fatalf("Failed to add owner to community: %v", err)
	}

	userMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      "user456",
	}
	_, err = testStorage.Insert(ownerCtx, userMembership)
	if err != nil {
		t.Fatalf("Failed to add user456 to community: %v", err)
	}

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "State Reversion Test Experience",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Verify experience starts in ACTIVE state
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if exp.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE {
		t.Errorf("Expected initial state ACTIVE, got %s", exp.State)
	}

	t.Run("RSVP Yes transitions experience to JOINED state", func(t *testing.T) {
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})
		resp, err := service.RSVPToExperience(userCtx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}

		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected experience state JOINED, got %s", resp.Msg.Experience.State)
		}
	})

	t.Run("RSVP No stays JOINED because owner always has YES RSVP", func(t *testing.T) {
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		})
		resp, err := service.RSVPToExperience(userCtx, rsvpReq)
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}

		// Owner's auto-YES RSVP keeps the experience in JOINED state.
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected experience to stay JOINED (owner has YES), got %s", resp.Msg.Experience.State)
		}
	})

	t.Run("RSVP No does not revert when other Yes/Maybe RSVPs exist", func(t *testing.T) {
		// Create another user
		createTestUser(t, testStorage, "user789", "user789@example.com", "User 789")
		user789Ctx := createAuthenticatedContext("user789", "user789@example.com", models.Role_ROLE_USER)

		// Add user789 to community
		user789Membership := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      "user789",
		}
		_, err = testStorage.Insert(ownerCtx, user789Membership)
		if err != nil {
			t.Fatalf("Failed to add user789 to community: %v", err)
		}

		// User456 RSVPs Yes
		rsvpReq1 := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})
		_, err = service.RSVPToExperience(userCtx, rsvpReq1)
		if err != nil {
			t.Fatalf("RSVPToExperience for user456 failed: %v", err)
		}

		// User789 RSVPs Maybe
		rsvpReq2 := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
		})
		_, err = service.RSVPToExperience(user789Ctx, rsvpReq2)
		if err != nil {
			t.Fatalf("RSVPToExperience for user789 failed: %v", err)
		}

		// User456 changes to No (but user789 still has Maybe)
		rsvpReq3 := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		})
		resp, err := service.RSVPToExperience(userCtx, rsvpReq3)
		if err != nil {
			t.Fatalf("RSVPToExperience for user456 failed: %v", err)
		}

		// Experience should still be JOINED (user789 still has Maybe)
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected experience to remain JOINED (another user has Maybe), got %s", resp.Msg.Experience.State)
		}

		// User789 also changes to No
		rsvpReq4 := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		})
		resp, err = service.RSVPToExperience(user789Ctx, rsvpReq4)
		if err != nil {
			t.Fatalf("RSVPToExperience for user789 failed: %v", err)
		}

		// Owner's auto-YES RSVP keeps the experience in JOINED state even
		// after all other attendees RSVP NO.
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected experience to stay JOINED (owner has YES), got %s", resp.Msg.Experience.State)
		}
	})
}
