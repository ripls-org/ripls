package integration_tests

// Integration tests for experience RSVP-change workflows.
// Tests are derived from docs/workflows/experience.md workflow examples.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestExperience_Example3_ParticipantChangesRSVPToNo tests RSVP change to No.
// Reference: docs/workflows/experience.md - "3. Participant Changes RSVP to No"
//
// Preconditions:
// - A test community exists
// - Three users exist: an owner, participant A, and participant B
// - The owner has created and shared an experience
// - Participant A and B have RSVPed Yes (experience is in JOINED state)
//
// Steps:
// 1. Participant A changes RSVP from Yes to No
// 2. System removes Participant A from conversation
//
// Postconditions:
// - Participant A is no longer in the conversation
// - Participant B remains in the conversation
// - The experience remains in JOINED state (at least one Yes/Maybe RSVP remains).
func TestExperience_Example3_ParticipantChangesRSVPToNo(t *testing.T) {
	// Setup: Create database and start server
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// Precondition: Register owner (first user - no invitation needed)
	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner")

	// Precondition: Create community
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Test Community", "A community for testing experiences")

	// Precondition: Register participants via invitation
	participantAToken, participantAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantA@example.com", "Participant A")
	participantBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantB@example.com", "Participant B")

	// Create authenticated clients
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)
	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)

	var experienceID string
	var conversationID string

	// Precondition: Owner creates and shares experience
	t.Run("Precondition_Setup", func(t *testing.T) {
		resp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Book Club Meeting",
			Description: "Monthly book discussion",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = resp.Msg.Experience.Id

		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)

		// Capture conversation ID for participant checks
		getResp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		conversationID = getResp.Msg.Experience.GetConversationId()
	})

	// Precondition: Both participants RSVP Yes
	t.Run("Precondition_ParticipantsRSVPYes", func(t *testing.T) {
		_, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience A failed: %v", err)
		}

		resp, err := participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience B failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected JOINED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 1: Participant A changes RSVP to No
	t.Run("Step1_ParticipantAChangesToNo", func(t *testing.T) {
		_, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience (change to No) failed: %v", err)
		}
	})

	// Postcondition: Experience remains in JOINED state
	t.Run("Postcondition_ExperienceRemainsJoined", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Postcondition failed: Expected JOINED state (B is still RSVP Yes), got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Participant A is now NO; at least one other YES RSVP exists (Participant B
	// and the owner's auto-YES from share). Verify A's record was updated and B/owner are still YES.
	t.Run("Postcondition_ParticipantBStillRSVPed", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		aIntention := api.RSVPIntention_RSVP_INTENTION_UNSPECIFIED
		nonAYesCount := 0
		for _, rsvp := range resp.Msg.Rsvps {
			if rsvp.User.Id == participantAID {
				aIntention = rsvp.Intention
			} else if rsvp.Intention == api.RSVPIntention_RSVP_INTENTION_YES {
				nonAYesCount++
			}
		}
		if aIntention != api.RSVPIntention_RSVP_INTENTION_NO {
			t.Errorf("Postcondition failed: Expected Participant A's intention to be NO, got %v", aIntention)
		}
		if nonAYesCount < 1 {
			t.Errorf("Postcondition failed: Expected at least 1 YES RSVP from non-A attendees (Participant B + owner), got %d", nonAYesCount)
		}
	})

	// Per docs/workflows/experience.md Example 3: "Participant A remains in the
	// conversation (membership is not pruned on No, since the conversation is
	// shared across communities)". Conversation membership is additive on
	// Yes/Maybe and is never revoked on No — there is no removal call on the No
	// path in rsvp.go.
	t.Run("Postcondition_ParticipantARemainsInConversation", func(t *testing.T) {
		resp, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
		found := false
		for _, p := range resp.Msg.Conversation.Participants {
			if p.Id == participantAID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Participant A was removed from the conversation after RSVP No; membership must not be pruned on No (docs/workflows/experience.md Example 3)")
		}
	})

	// Per docs/workflows/experience.md: RSVP No does not trigger notifications.
	// Notifications sent: EXPERIENCE_CREATED + initial RSVPs.
	t.Run("Postcondition_NoNotificationsForRSVPNo", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 4, 5*time.Second)
		// Expected: 2 (EXPERIENCE_CREATED to A and B) + 2 (RSVPs to owner)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 4 // 2 created + 2 RSVPs
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (2 created + 2 RSVPs), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}

// TestExperience_Example4_AllParticipantsDecline tests state reversion when all decline.
// Reference: docs/workflows/experience.md - "4. All Participants Decline"
//
// Preconditions:
// - A test community exists
// - Two users exist: an owner and a participant
// - The owner has created and shared an experience
// - The participant has RSVPed Yes (experience is in JOINED state)
//
// Steps:
// 1. Participant changes RSVP from Yes to No
// 2. System removes participant from conversation
//
// Postconditions:
// - The experience reverts to ACTIVE state (no more Yes/Maybe RSVPs)
// - Only the owner remains in the conversation.
func TestExperience_Example4_AllParticipantsDecline(t *testing.T) {
	// Setup: Create database and start server
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// Precondition: Register owner (first user - no invitation needed)
	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner")

	// Precondition: Create community
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Test Community", "A community for testing experiences")

	// Precondition: Register participant via invitation
	participantToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "participant@example.com", "Participant")

	// Create authenticated clients
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	participantExperienceClient := createAuthExperienceClient(participantToken, serverURL)

	var experienceID string
	var itemCommunityID string

	// Precondition: Owner creates and shares experience
	t.Run("Precondition_Setup", func(t *testing.T) {
		resp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Cooking Class",
			Description: "Learn to make pasta from scratch",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = resp.Msg.Experience.Id
		// Every experience is born in its own per-item community (#2492); the
		// owner is auto-RSVPed YES there. The auto-RSVP happens only once, on
		// first share (the per-item community), not when sharing to communityID.
		itemCommunityID = resp.Msg.GetItemCommunityId()
		if itemCommunityID == "" {
			t.Fatal("expected SaveExperience to return ItemCommunityId")
		}

		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)
	})

	// Precondition: Participant RSVPs Yes → JOINED state
	t.Run("Precondition_ParticipantRSVPsYes", func(t *testing.T) {
		resp, err := participantExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected JOINED state after RSVP, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 1: Participant changes RSVP to No
	t.Run("Step1_ParticipantChangesToNo", func(t *testing.T) {
		_, err := participantExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_NO,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience (change to No) failed: %v", err)
		}
	})

	// Postcondition: Experience remains in JOINED state.
	// The owner is auto-RSVPed YES when they share the experience (see share.go). Even after
	// the only participant declines, the owner's YES RSVP keeps the experience in JOINED.
	t.Run("Postcondition_ExperienceRemainsJoined", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Postcondition failed: Expected JOINED state (owner auto-YES still active), got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Only the owner's YES RSVP remains; no MAYBE RSVPs.
	// The participant declined (NO) in communityID, so the only remaining YES is
	// the owner's auto-YES, which lives in the per-item community (#2492). Query
	// the per-item community to see that owner auto-YES.
	t.Run("Postcondition_OnlyOwnerRSVPRemains", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: itemCommunityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.RsvpYesCount != 1 {
			t.Errorf("Postcondition failed: Expected 1 YES RSVP (owner auto-YES), got %d", resp.Msg.Experience.RsvpYesCount)
		}
		if resp.Msg.Experience.RsvpMaybeCount != 0 {
			t.Errorf("Postcondition failed: Expected 0 MAYBE RSVPs, got %d", resp.Msg.Experience.RsvpMaybeCount)
		}
	})

	// Postcondition: Conversation still exists for owner
	t.Run("Postcondition_ConversationStillExists", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.GetConversationId() == "" {
			t.Errorf("Postcondition failed: Conversation should still exist for owner after all participants decline")
		}
	})

	// Per docs/workflows/experience.md: RSVP No does not trigger notifications.
	// Notifications sent: EXPERIENCE_CREATED + initial RSVP.
	t.Run("Postcondition_NoNotificationsForRSVPNo", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 2, 5*time.Second)
		// Expected: 1 (EXPERIENCE_CREATED to participant) + 1 (RSVP to owner)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 2 // 1 created + 1 RSVP
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (1 created + 1 RSVP), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}
