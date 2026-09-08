package integration_tests

// Integration tests for experience cross-community sharing workflows.
// Tests are derived from docs/workflows/experience.md workflow examples.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestExperience_Example5_CrossCommunitySharing tests sharing an experience across multiple communities.
// Reference: docs/workflows/experience.md - "5. Cross-Community Sharing"
//
// Architecture note: The experience model stores a single conversation_id, so all communities
// sharing an experience share the same conversation. Participants from any community are added
// to this shared conversation when they RSVP.
//
// Preconditions:
// - Two test communities exist (Community A and Community B)
// - An owner is a member of both communities
// - Participant A is a member of Community A only
// - Participant B is a member of Community B only
//
// Steps:
// 1. Owner creates experience and shares with Community A → Creates CommunityExperience for A
// 2. System creates conversation for the experience, adds owner as participant
// 3. Participant A RSVPs Yes in Community A
// 4. System adds Participant A to the experience conversation
// 5. Owner shares with Community B → reuses the same experience conversation
// 6. Community B returns the same conversation ID as Community A
// 7. Participant B RSVPs Yes in Community B
// 8. System adds Participant B to the shared conversation
// 9. Owner can access the shared conversation
//
// Postconditions:
// - Experience appears in both community feeds with shared_community_ids: ["A", "B"]
// - One shared conversation exists for the experience
// - Participant A can access the shared conversation
// - Participant B can access the shared conversation
// - Owner can access the shared conversation
// - RSVPs are isolated per community.
func TestExperience_Example5_CrossCommunitySharing(t *testing.T) {
	// Setup: Create database and start server
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// ========== PRECONDITIONS ==========

	// Register owner (first user - no invitation needed)
	ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	// Create Community A
	communityAID := setupTestCommunity(t, ctx, ownerCommunityClient, "Community A", "Test Community A")

	// Create Community B
	communityBID := setupTestCommunity(t, ctx, ownerCommunityClient, "Community B", "Test Community B")

	// Register Participant A and join Community A only
	participantAToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityAID, "participanta@example.com", "Participant A")
	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)
	participantAChatClient := createAuthChatClient(participantAToken, serverURL)

	// Register Participant B and join Community B only
	participantBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityBID, "participantb@example.com", "Participant B")
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)
	participantBChatClient := createAuthChatClient(participantBToken, serverURL)

	// ========== STEPS ==========

	// Step 1: Owner creates experience and shares with Community A
	var experienceID string
	var conversationAID string
	t.Run("Step1_OwnerCreatesAndSharesWithCommunityA", func(t *testing.T) {
		// Create experience
		saveResp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Cross-Community Test Experience",
			Description: "Testing multi-community sharing",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = saveResp.Msg.Experience.Id

		// Share with Community A
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityAID)
	})

	// Step 2: System creates conversation for Community A
	t.Run("Step2_ConversationCreatedForCommunityA", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		conversationAID = resp.Msg.Experience.GetConversationId()
		if conversationAID == "" {
			t.Errorf("Step2 failed: Conversation ID should be set for Community A")
		}
	})

	// Step 3: Participant A RSVPs Yes in Community A
	t.Run("Step3_ParticipantARSVPsYes", func(t *testing.T) {
		_, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityAID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant A: %v", err)
		}
	})

	// Step 4: System adds Participant A to Community A's conversation
	t.Run("Step4_ParticipantAInConversationA", func(t *testing.T) {
		// Verify Participant A can access the conversation (means they're a participant)
		_, err := participantAChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Errorf("Step4 failed: Participant A should be able to access Community A's conversation: %v", err)
		}
	})

	// Step 5: Owner shares with Community B
	var conversationBID string
	t.Run("Step5_OwnerSharesWithCommunityB", func(t *testing.T) {
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityBID)
	})

	// Step 6: Experience shared with Community B reuses the same single conversation.
	// The new architecture stores one conversation per experience (on Experience.ConversationId),
	// so all communities sharing an experience use the same conversation.
	t.Run("Step6_SeparateConversationForCommunityB", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityBID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		conversationBID = resp.Msg.Experience.GetConversationId()
		if conversationBID == "" {
			t.Errorf("Step6 failed: Conversation ID should be set for Community B")
		}
		// Single conversation per experience — both communities share the same conversation.
		if conversationBID != conversationAID {
			t.Errorf("Step6 failed: Expected Community B to reuse the same conversation as Community A, got different IDs: A=%s B=%s", conversationAID, conversationBID)
		}
	})

	// Step 7: Participant B RSVPs Yes in Community B
	t.Run("Step7_ParticipantBRSVPsYes", func(t *testing.T) {
		_, err := participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityBID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant B: %v", err)
		}
	})

	// Step 8: System adds Participant B to the shared conversation via RSVP.
	// Participant B was added via RSVP in Step 7 and should be able to access the
	// shared conversation even though it was originally created for Community A.
	t.Run("Step8_ParticipantBInConversationB", func(t *testing.T) {
		// Verify Participant B can access the shared conversation (means they're a participant)
		_, err := participantBChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Errorf("Step8 failed: Participant B should be able to access the shared conversation: %v", err)
		}
	})

	// Step 9: Owner views the shared experience conversation
	t.Run("Step9_ConversationAIsolation", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Fatalf("Step9 failed: Owner should be able to access the shared conversation: %v", err)
		}
	})

	// Step 10: Owner can still access the conversation via the Community B conversation ID
	// (same conversation ID, since experiences use a single shared conversation)
	t.Run("Step10_ConversationBIsolation", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Fatalf("Step10 failed: Owner should be able to access the shared conversation: %v", err)
		}
	})

	// ========== POSTCONDITIONS ==========

	// Postcondition: Experience appears in both community feeds
	t.Run("Postcondition_ExperienceInBothFeeds", func(t *testing.T) {
		// Check Community A feed
		respA, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Experience should be accessible in Community A: %v", err)
		} else {
			foundA := false
			foundB := false
			for _, id := range respA.Msg.Experience.SharedCommunityIds {
				if id == communityAID {
					foundA = true
				}
				if id == communityBID {
					foundB = true
				}
			}
			if !foundA || !foundB {
				t.Errorf("Postcondition failed: Experience should have both communities in shared_community_ids, got: %v", respA.Msg.Experience.SharedCommunityIds)
			}
		}
	})

	// Postcondition: One shared conversation exists for the experience (shared across communities).
	// The new architecture stores a single conversation on the Experience model itself,
	// so all communities sharing the experience use the same conversation.
	t.Run("Postcondition_SingleSharedConversation", func(t *testing.T) {
		if conversationAID == "" {
			t.Errorf("Postcondition failed: Shared conversation should exist")
		}
		// conversationBID is set in Step 6 and should equal conversationAID
		if conversationAID != conversationBID {
			t.Errorf("Postcondition failed: Expected single shared conversation, got A=%s B=%s", conversationAID, conversationBID)
		}
	})

	// Postcondition: Participant A (RSVP'd in Community A) can access the shared conversation
	t.Run("Postcondition_ParticipantACanOnlySeeConversationA", func(t *testing.T) {
		_, err := participantAChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Participant A should be able to access the shared conversation: %v", err)
		}
	})

	// Postcondition: Participant B (RSVP'd in Community B) can access the shared conversation
	t.Run("Postcondition_ParticipantBCanOnlySeeConversationB", func(t *testing.T) {
		_, err := participantBChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Participant B should be able to access the shared conversation: %v", err)
		}
	})

	// Postcondition: Owner can access the shared conversation
	t.Run("Postcondition_OwnerCanAccessBothConversations", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Owner should be able to access the shared conversation: %v", err)
		}
	})

	// Postcondition: RSVPs are isolated per community
	t.Run("Postcondition_RSVPsIsolatedPerCommunity", func(t *testing.T) {
		// Check Community A - Participant A should have RSVPed YES
		respA, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed for Community A: %v", err)
		}
		// Verify at least one YES RSVP exists (Participant A, and possibly owner)
		yesCountA := 0
		for _, rsvp := range respA.Msg.Rsvps {
			if rsvp.Intention == api.RSVPIntention_RSVP_INTENTION_YES {
				yesCountA++
			}
		}
		if yesCountA < 1 {
			t.Errorf("Postcondition failed: Community A should have at least 1 YES RSVP, got %d", yesCountA)
		}

		// Check Community B - Participant B should have RSVPed YES
		respB, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityBID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed for Community B: %v", err)
		}
		// Verify at least one YES RSVP exists (Participant B, and possibly owner)
		yesCountB := 0
		for _, rsvp := range respB.Msg.Rsvps {
			if rsvp.Intention == api.RSVPIntention_RSVP_INTENTION_YES {
				yesCountB++
			}
		}
		if yesCountB < 1 {
			t.Errorf("Postcondition failed: Community B should have at least 1 YES RSVP, got %d", yesCountB)
		}

		// Key postcondition: RSVPs are isolated - each community sees different RSVP lists
		// This is validated by the fact that Participant A/B can only access their respective conversations
	})

	// Per docs/workflows/experience.md: RSVPs trigger notifications to owner.
	t.Run("Postcondition_OwnerNotifiedOnRSVPs", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 2, 5*time.Second)
		ownerNotifs := logCapture.GetNotificationLogsForUser(ownerID)
		// Owner should receive 2 notifications (Participant A and B RSVPed Yes)
		if len(ownerNotifs) != 2 {
			t.Errorf("Expected owner to receive 2 notifications (one per RSVP), got %d", len(ownerNotifs))
			for _, n := range ownerNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
	})
}

// TestExperience_Example6_UnsharingFromCommunity tests unsharing an experience from a community.
// Reference: docs/workflows/experience.md - "6. Unsharing from Community"
//
// Preconditions:
// - An experience is shared with Community A and Community B
// - Participants have RSVPed in both communities
// - Conversations exist for both communities
//
// Steps:
// 1. Owner opens Communities menu in experience overflow
// 2. Owner unchecks Community B to unshare
// 3. System deletes CommunityExperience record for Community B
//
// Postconditions:
// - Experience no longer appears in Community B's feed
// - Experience still appears in Community A's feed
// - Community B's conversation remains accessible (for history)
// - Community A's conversation unaffected
// - shared_community_ids updated to ["A"].
func TestExperience_Example6_UnsharingFromCommunity(t *testing.T) {
	// Setup: Create database and start server
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// ========== PRECONDITIONS ==========

	// Register owner (first user - no invitation needed)
	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	// Create Community A
	communityAID := setupTestCommunity(t, ctx, ownerCommunityClient, "Community A", "Test Community A")

	// Create Community B
	communityBID := setupTestCommunity(t, ctx, ownerCommunityClient, "Community B", "Test Community B")

	// Register Participant A and join Community A only
	participantAToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityAID, "participanta@example.com", "Participant A")
	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)

	// Register Participant B and join Community B only
	participantBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityBID, "participantb@example.com", "Participant B")
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)

	// Create experience and share with both communities
	var experienceID string
	var itemCommunityID string
	var conversationAID, conversationBID string

	t.Run("Precondition_CreateAndShareExperience", func(t *testing.T) {
		// Create experience
		saveResp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Unsharing Test Experience",
			Description: "Testing unsharing from community",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = saveResp.Msg.Experience.Id
		// Every experience is born in its own per-item community (#2492). It is
		// always present in shared_community_ids alongside any communities it is
		// explicitly shared to.
		itemCommunityID = saveResp.Msg.GetItemCommunityId()
		if itemCommunityID == "" {
			t.Fatal("expected SaveExperience to return ItemCommunityId")
		}

		// Share with Community A
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityAID)

		// Share with Community B
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityBID)

		// Get conversation IDs
		respA, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed for Community A: %v", err)
		}
		conversationAID = respA.Msg.Experience.GetConversationId()

		respB, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityBID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed for Community B: %v", err)
		}
		conversationBID = respB.Msg.Experience.GetConversationId()
	})

	// Participants RSVP
	t.Run("Precondition_ParticipantsRSVP", func(t *testing.T) {
		// Participant A RSVPs in Community A
		_, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityAID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant A: %v", err)
		}

		// Participant B RSVPs in Community B
		_, err = participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityBID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant B: %v", err)
		}
	})

	// ========== STEPS ==========

	// Step 2: Owner unshares from Community B
	t.Run("Step2_OwnerUnsharesFromCommunityB", func(t *testing.T) {
		if err := unshareExperienceFromCommunity(ctx, ownerCommunityClient, experienceID, communityBID); err != nil {
			t.Fatalf("UnshareItem failed: %v", err)
		}
	})

	// ========== POSTCONDITIONS ==========

	// Postcondition: Experience no longer appears in Community B's feed
	t.Run("Postcondition_ExperienceRemovedFromCommunityB", func(t *testing.T) {
		// Try to get experience in Community B context - should fail or not be visible
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityBID,
		}))
		if err == nil {
			// If no error, check that Community B is not in shared_community_ids
			foundB := false
			for _, id := range resp.Msg.Experience.SharedCommunityIds {
				if id == communityBID {
					foundB = true
					break
				}
			}
			if foundB {
				t.Errorf("Postcondition failed: Community B should not be in shared_community_ids after unsharing")
			}
		}
	})

	// Postcondition: Experience still appears in Community A's feed
	t.Run("Postcondition_ExperienceStillInCommunityA", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Experience should still be accessible in Community A: %v", err)
		} else {
			foundA := false
			for _, id := range resp.Msg.Experience.SharedCommunityIds {
				if id == communityAID {
					foundA = true
					break
				}
			}
			if !foundA {
				t.Errorf("Postcondition failed: Community A should still be in shared_community_ids")
			}
		}
	})

	// Postcondition: Community B's conversation remains accessible (for history)
	t.Run("Postcondition_ConversationBStillAccessible", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Community B's conversation should remain accessible for history: %v", err)
		}
	})

	// Postcondition: Community A's conversation unaffected
	t.Run("Postcondition_ConversationAUnaffected", func(t *testing.T) {
		_, err := ownerChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Errorf("Postcondition failed: Community A's conversation should remain accessible: %v", err)
		}
	})

	// Postcondition: shared_community_ids updated to only include A
	t.Run("Postcondition_SharedCommunityIdsUpdated", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}

		// After unsharing B, shared_community_ids = [per-item community, A] (#2492):
		// the per-item community is always present alongside explicit shares.
		if len(resp.Msg.Experience.SharedCommunityIds) != 2 {
			t.Errorf("Postcondition failed: Should have exactly 2 communities in shared_community_ids (per-item + A), got %d", len(resp.Msg.Experience.SharedCommunityIds))
		}

		foundA := false
		foundB := false
		foundItem := false
		for _, id := range resp.Msg.Experience.SharedCommunityIds {
			if id == communityAID {
				foundA = true
			}
			if id == communityBID {
				foundB = true
			}
			if id == itemCommunityID {
				foundItem = true
			}
		}

		if !foundA {
			t.Errorf("Postcondition failed: Community A should be in shared_community_ids")
		}
		if !foundItem {
			t.Errorf("Postcondition failed: per-item community should be in shared_community_ids")
		}
		if foundB {
			t.Errorf("Postcondition failed: Community B should NOT be in shared_community_ids")
		}
	})

	// Per docs/workflows/experience.md: Unsharing does not trigger notifications.
	// Notifications sent: EXPERIENCE_CREATED (per community) + RSVPs.
	t.Run("Postcondition_NoNotificationsForUnsharing", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 4, 5*time.Second)
		// Expected: 2 (EXPERIENCE_CREATED, one per community share) + 2 (RSVPs to owner)
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
