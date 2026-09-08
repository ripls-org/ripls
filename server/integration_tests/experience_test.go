package integration_tests

// Integration tests for experience workflows.
// Tests are derived from docs/workflows/experience.md workflow examples.
// Each test follows the Preconditions/Steps/Postconditions structure from the workflow docs.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestExperience_Example1_HappyPath tests the complete experience workflow.
// Reference: docs/workflows/experience.md - "1. Experience Completed (Happy Path)"
//
// Preconditions:
// - A test community exists
// - Four users exist: an owner, participant A, participant B, and participant C, all members of the community
//
// Steps:
// 1. Owner creates experience → ACTIVE state
// 2. Owner shares experience with community → Creates CommunityExperience record
// 3. System creates conversation, adds owner as initial participant
// 4. Participant A RSVPs Yes → Experience transitions to JOINED state
// 5. System adds Participant A to conversation
// 6. Participant B also RSVPs Yes
// 7. System adds Participant B to conversation
// 8. Participant C RSVPs Maybe
// 9. System adds Participant C to conversation
// 10. Owner (Optional) marks experience as in-process → State changes to IN_PROCESS
// 11. Owner records attendance (YES for A & B, NO for C) — called before CompleteExperience
// 12. Owner marks experience complete → State changes to COMPLETED
// 13. System generates story card for community feed (inside CompleteExperience)
//
// Postconditions:
// - The experience state is COMPLETED
// - A story card appears in community feed
// - The conversation remains accessible for history
// - All RSVPed participants (A, B, C) remain in conversation.
func TestExperience_Example1_HappyPath(t *testing.T) {
	// Setup: Create database and start server
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// Precondition: Register owner (first user - no invitation needed)
	ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Owner")

	// Precondition: Create community
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Test Community", "A community for testing experiences")

	// Precondition: Register participants via invitation
	participantAToken, participantAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantA@example.com", "Participant A")
	participantBToken, participantBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantB@example.com", "Participant B")
	participantCToken, participantCID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantC@example.com", "Participant C")

	// Create authenticated clients
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)
	participantCExperienceClient := createAuthExperienceClient(participantCToken, serverURL)

	var experienceID string

	// Step 1: Owner creates experience → ACTIVE state
	t.Run("Step1_OwnerCreatesExperience", func(t *testing.T) {
		resp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Community Game Night",
			Description: "Let's play board games together!",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = resp.Msg.Experience.Id
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			t.Errorf("Expected ACTIVE state, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 2: Owner shares experience with community
	t.Run("Step2_OwnerSharesExperience", func(t *testing.T) {
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)
	})

	// Step 3: Verify conversation was created
	t.Run("Step3_VerifyConversationCreated", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.GetConversationId() == "" {
			t.Errorf("Expected conversation to be created after sharing")
		}
	})

	// Step 4: Participant A RSVPs Yes → Experience transitions to JOINED state
	t.Run("Step4_ParticipantARSVPsYes", func(t *testing.T) {
		resp, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected JOINED state after first RSVP, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 6: Participant B also RSVPs Yes
	t.Run("Step6_ParticipantBRSVPsYes", func(t *testing.T) {
		_, err := participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
		// Verify via GetExperience RSVPs array (counts on Experience may not be populated)
		getResp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		yesCount := 0
		for _, rsvp := range getResp.Msg.Rsvps {
			if rsvp.Intention == api.RSVPIntention_RSVP_INTENTION_YES {
				yesCount++
			}
		}
		if yesCount < 2 {
			t.Errorf("Expected at least 2 YES RSVPs, got %d", yesCount)
		}
	})

	// Step 8: Participant C RSVPs Maybe
	t.Run("Step8_ParticipantCRSVPsMaybe", func(t *testing.T) {
		_, err := participantCExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
		// Verify via GetExperience RSVPs array (counts on Experience may not be populated)
		getResp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		maybeCount := 0
		for _, rsvp := range getResp.Msg.Rsvps {
			if rsvp.Intention == api.RSVPIntention_RSVP_INTENTION_MAYBE {
				maybeCount++
			}
		}
		if maybeCount < 1 {
			t.Errorf("Expected at least 1 MAYBE RSVP, got %d", maybeCount)
		}
	})

	// Step 10: Owner (optional) marks experience as in-process
	t.Run("Step10_OwnerMarksInProcess", func(t *testing.T) {
		resp, err := ownerExperienceClient.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("MarkExperienceInProcess failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			t.Errorf("Expected IN_PROCESS state, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 11: Owner records attendance BEFORE completing the experience.
	// Per docs/workflows/experience.md: "Client calls RecordAttendance first, then
	// CompleteExperience. Story generation happens inside CompleteExperience after
	// attendance is already recorded, so all participants are included."
	t.Run("Step11_OwnerRecordsAttendanceBeforeComplete", func(t *testing.T) {
		_, err := ownerExperienceClient.RecordAttendance(ctx, connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Attendance: []*api.AttendanceRecord{
				{
					UserId:   participantAID,
					Attended: api.AttendedStatus_ATTENDED_STATUS_YES,
				},
				{
					UserId:   participantBID,
					Attended: api.AttendedStatus_ATTENDED_STATUS_YES,
				},
				{
					UserId:   participantCID,
					Attended: api.AttendedStatus_ATTENDED_STATUS_NO,
				},
			},
		}))
		if err != nil {
			t.Fatalf("RecordAttendance (before complete) failed: %v", err)
		}
	})

	// Step 12: Owner marks experience complete.
	// Story generation happens inside CompleteExperience after attendance is already set.
	var completeImpact *api.ImpactEstimate
	t.Run("Step12_OwnerCompletesExperience", func(t *testing.T) {
		resp, err := ownerExperienceClient.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected COMPLETED state, got %v", resp.Msg.Experience.State)
		}
		completeImpact = resp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	// Postcondition: Verify experience state is COMPLETED
	t.Run("Postcondition_ExperienceCompleted", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Postcondition failed: Expected COMPLETED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Verify conversation remains accessible
	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.GetConversationId() == "" {
			t.Errorf("Postcondition failed: Conversation should remain accessible after completion")
		}
	})

	// Postcondition: Verify all RSVPed participants are recorded with correct attendance
	t.Run("Postcondition_AllParticipantsRecorded", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		// Build map of user ID to RSVP data
		attendanceMap := make(map[string]*api.RSVP)
		for _, rsvp := range resp.Msg.Rsvps {
			attendanceMap[rsvp.User.Id] = rsvp
		}
		// Participant A should have ATTENDED_STATUS_YES
		if rsvp, ok := attendanceMap[participantAID]; !ok {
			t.Errorf("Postcondition failed: Participant A not found in RSVPs")
		} else if rsvp.Attended != api.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Postcondition failed: Participant A should have ATTENDED_STATUS_YES, got %v", rsvp.Attended)
		}
		// Participant B should have ATTENDED_STATUS_YES
		if rsvp, ok := attendanceMap[participantBID]; !ok {
			t.Errorf("Postcondition failed: Participant B not found in RSVPs")
		} else if rsvp.Attended != api.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Postcondition failed: Participant B should have ATTENDED_STATUS_YES, got %v", rsvp.Attended)
		}
		// Participant C should have ATTENDED_STATUS_NO
		if rsvp, ok := attendanceMap[participantCID]; !ok {
			t.Errorf("Postcondition failed: Participant C not found in RSVPs")
		} else if rsvp.Attended != api.AttendedStatus_ATTENDED_STATUS_NO {
			t.Errorf("Postcondition failed: Participant C should have ATTENDED_STATUS_NO, got %v", rsvp.Attended)
		}
	})

	// Per docs/workflows/experience.md Notifications table, the full set of notifications for
	// this scenario:
	//   3  share pings (A, B, C via EXPERIENCE_CREATED broadcast)
	//   3  RSVP pings to owner (A Yes, B Yes, C Maybe)
	//   3  MarkInProcess pings via EXPERIENCE_STARTED (A, B, C; actor excluded by funnel)
	//   3  CompleteExperience EXPERIENCE_COMPLETED to Yes/Maybe RSVPs (A, B, C; actor excluded)
	//  = 12 total
	// Owner receives 3 RSVP pings only; actor exclusion now applies uniformly via funnel.
	t.Run("Postcondition_OwnerNotifiedOnRSVPs", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 12, 5*time.Second)
		ownerNotifs := logCapture.GetNotificationLogsForUser(ownerID)
		// Owner receives 3 RSVP pings; lifecycle self-pings are eliminated by actor exclusion in the funnel.
		if len(ownerNotifs) != 3 {
			t.Errorf("Expected 3 notifications to owner (3 RSVPs, no lifecycle self-pings), got %d", len(ownerNotifs))
			for _, n := range ownerNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
	})

	// Impact estimation postconditions (per docs/workflows/experience.md Example 1)
	t.Run("Postcondition_ExperienceImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, completeImpact, "CompleteExperience response")
		// Experience uses flat defaults: time_saved scaled by attendee count.
		// 2 attendees (A=YES, B=YES auto-marked) × 120 min default = 240 min.
		if completeImpact.TimeSaved != nil {
			assertEstimatePositive(t, completeImpact.TimeSaved.Minutes, "experience time_saved")
			expectedTimeMean := float32(240.0) // 2 attendees × 120 min
			if diff := completeImpact.TimeSaved.Minutes.Mean - expectedTimeMean; diff > 1.0 || diff < -1.0 {
				t.Errorf("Expected time_saved.mean ≈ %.0f (2 attendees × 120 min), got %.2f",
					expectedTimeMean, completeImpact.TimeSaved.Minutes.Mean)
			}
		} else {
			t.Error("time_saved is nil")
		}
	})

	t.Run("Postcondition_CommunityImpactMetrics", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertEstimatePositive(t, metrics.TimeBankedMinutes, "community time_banked_minutes after completed experience")
	})
}

// TestExperience_Example2_Cancelled tests experience cancellation.
// Reference: docs/workflows/experience.md - "2. Experience Cancelled"
//
// Preconditions:
// - A test community exists
// - Three users exist: an owner, participant A, and participant B
// - The owner has created and shared an experience
// - Participant A and B have RSVPed Yes (experience is in JOINED state)
//
// Steps:
// 1. Owner realizes event can't happen
// 2. Owner cancels experience → State changes to CANCELLED
// 3. System sends notifications to all RSVPed users
//
// Postconditions:
// - The experience state is CANCELLED
// - The conversation remains accessible for history
// - Participants received cancellation notifications.
func TestExperience_Example2_Cancelled(t *testing.T) {
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
	participantAToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantA@example.com", "Participant A")
	participantBToken, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantB@example.com", "Participant B")

	// Create authenticated clients
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)
	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)

	var experienceID string

	// Precondition: Owner creates experience
	t.Run("Precondition_OwnerCreatesExperience", func(t *testing.T) {
		resp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Weekend Hike",
			Description: "A group hike in the mountains",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = resp.Msg.Experience.Id
	})

	// Precondition: Owner shares experience
	t.Run("Precondition_OwnerSharesExperience", func(t *testing.T) {
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)
	})

	// Precondition: Participant A RSVPs Yes
	t.Run("Precondition_ParticipantARSVPsYes", func(t *testing.T) {
		_, err := participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
	})

	// Precondition: Participant B RSVPs Yes
	t.Run("Precondition_ParticipantBRSVPsYes", func(t *testing.T) {
		resp, err := participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_JOINED {
			t.Errorf("Expected JOINED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 2: Owner cancels experience
	t.Run("Step2_OwnerCancelsExperience", func(t *testing.T) {
		resp, err := ownerExperienceClient.CancelExperience(ctx, connect.NewRequest(&api.CancelExperienceRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("CancelExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			t.Errorf("Expected CANCELLED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Verify experience state is CANCELLED
	t.Run("Postcondition_ExperienceCancelled", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			t.Errorf("Postcondition failed: Expected CANCELLED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Verify conversation remains accessible
	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.GetConversationId() == "" {
			t.Errorf("Postcondition failed: Conversation should remain accessible after cancellation")
		}
	})

	// Per docs/workflows/experience.md Notifications table, the full set of notifications for
	// this scenario:
	//   2  share pings (A, B via EXPERIENCE_CREATED broadcast)
	//   2  RSVP pings to owner (A Yes, B Yes)
	//   2  CancelExperience pings via EXPERIENCE_CANCELLED (A, B; actor excluded by funnel)
	//  = 6 total
	t.Run("Postcondition_NotificationsForCancellation", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 6, 5*time.Second)
		// Actor exclusion via funnel eliminates the owner self-ping.
		notifLogs := logCapture.GetNotificationLogs()
		if len(notifLogs) != 6 {
			t.Errorf("Expected 6 notifications (2 share + 2 RSVPs + 2 cancel), got %d", len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		impactClient := createAuthImpactClient(ownerToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}
