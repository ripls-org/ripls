package integration_tests

// Integration tests for experience attendance-recording and completion workflows.
// Tests are derived from docs/workflows/experience.md workflow examples.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestExperience_Example7_RecordingAttendance tests attendance recording.
// Reference: docs/workflows/experience.md - "7. Recording Attendance"
//
// Preconditions:
// - A test community exists
// - Four users exist: an owner, participant A, participant B, and participant C
// - An experience exists in ACTIVE or IN_PROCESS state with RSVPs already recorded
// - Participant A RSVPed Yes, Participant B RSVPed Yes, Participant C RSVPed Maybe
//
// Steps:
// 1. Owner opens completion modal (experience may still be ACTIVE or IN_PROCESS)
// 2-5. Owner records attendance (YES for A & B, NO for C) via RecordAttendance
//  6. Owner confirms completion — RecordAttendance is called FIRST, then CompleteExperience
//     Story generation happens inside CompleteExperience after attendance is already set.
//
// Postconditions:
// - Participant A has ATTENDED_STATUS_YES
// - Participant B has ATTENDED_STATUS_YES
// - Participant C has ATTENDED_STATUS_NO
// - No notifications are sent for attendance recording.
func TestExperience_Example7_RecordingAttendance(t *testing.T) {
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

	// Create community
	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Test Community", "A community for testing experiences")

	// Register participants via invitation
	participantAToken, participantAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantA@example.com", "Participant A")
	participantBToken, participantBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantB@example.com", "Participant B")
	participantCToken, participantCID := registerUserByInvite(t, serverURL, ownerToken, communityID, "participantC@example.com", "Participant C")

	participantAExperienceClient := createAuthExperienceClient(participantAToken, serverURL)
	participantBExperienceClient := createAuthExperienceClient(participantBToken, serverURL)
	participantCExperienceClient := createAuthExperienceClient(participantCToken, serverURL)

	var experienceID string

	// Precondition: Create and share experience, gather RSVPs, mark in-process.
	// The experience is left in IN_PROCESS state (not yet completed).
	t.Run("Precondition_CreateShareAndGatherRSVPs", func(t *testing.T) {
		// Create experience
		saveResp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Attendance Test Experience",
			Description: "Testing attendance recording",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = saveResp.Msg.Experience.Id

		// Share with community
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)

		// Participant A RSVPs Yes
		_, err = participantAExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant A: %v", err)
		}

		// Participant B RSVPs Yes
		_, err = participantBExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant B: %v", err)
		}

		// Participant C RSVPs Maybe
		_, err = participantCExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed for Participant C: %v", err)
		}

		// Mark experience as in-process (experience is now IN_PROCESS, not yet completed)
		_, err = ownerExperienceClient.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("MarkExperienceInProcess failed: %v", err)
		}
	})

	// ========== STEPS ==========

	// Steps 2-5: Owner records attendance while experience is still IN_PROCESS.
	// Per docs/workflows/experience.md: RecordAttendance is allowed for all non-terminal
	// states. Client calls this BEFORE CompleteExperience so that story generation inside
	// CompleteExperience includes all participants.
	t.Run("Steps2to5_OwnerRecordsAttendanceBeforeComplete", func(t *testing.T) {
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

	// Step 6: Owner completes the experience AFTER recording attendance.
	// Story generation runs inside CompleteExperience with attendance already set.
	t.Run("Step6_OwnerCompletesExperienceAfterAttendance", func(t *testing.T) {
		resp, err := ownerExperienceClient.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected COMPLETED state, got %v", resp.Msg.Experience.State)
		}
	})

	// ========== POSTCONDITIONS ==========

	// Postcondition: Verify attendance statuses are recorded correctly
	t.Run("Postcondition_AttendanceStatusesRecorded", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}

		// Build a map of user ID to attendance status
		attendanceMap := make(map[string]api.AttendedStatus)
		for _, rsvp := range resp.Msg.Rsvps {
			attendanceMap[rsvp.User.Id] = rsvp.Attended
		}

		// Verify Participant A has ATTENDED_STATUS_YES
		if status, ok := attendanceMap[participantAID]; !ok {
			t.Errorf("Postcondition failed: Participant A not found in RSVPs")
		} else if status != api.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Postcondition failed: Participant A should have ATTENDED_STATUS_YES, got %v", status)
		}

		// Verify Participant B has ATTENDED_STATUS_YES
		if status, ok := attendanceMap[participantBID]; !ok {
			t.Errorf("Postcondition failed: Participant B not found in RSVPs")
		} else if status != api.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Postcondition failed: Participant B should have ATTENDED_STATUS_YES, got %v", status)
		}

		// Verify Participant C has ATTENDED_STATUS_NO
		if status, ok := attendanceMap[participantCID]; !ok {
			t.Errorf("Postcondition failed: Participant C not found in RSVPs")
		} else if status != api.AttendedStatus_ATTENDED_STATUS_NO {
			t.Errorf("Postcondition failed: Participant C should have ATTENDED_STATUS_NO, got %v", status)
		}
	})

	// Per docs/workflows/experience.md Notifications table, the full set of notifications for
	// this scenario (RecordAttendance itself sends none; the other operations do):
	//   3  share pings (A, B, C via EXPERIENCE_CREATED broadcast; precondition)
	//   3  RSVP pings to owner (A Yes, B Yes, C Maybe; precondition)
	//   3  MarkInProcess pings via EXPERIENCE_STARTED (A, B, C; actor excluded by funnel; precondition)
	//   3  CompleteExperience EXPERIENCE_COMPLETED to Yes/Maybe RSVPs (A, B, C; actor excluded; step 6)
	//  = 12 total
	t.Run("Postcondition_NotificationCountMatchesContract", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 12, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		// 3 share + 3 RSVPs + 3 EXPERIENCE_STARTED + 3 EXPERIENCE_COMPLETED (Yes/Maybe RSVPs) = 12 total.
		// Actor exclusion via funnel eliminates lifecycle self-pings from both MarkInProcess and CompleteExperience.
		if len(notifLogs) != 12 {
			t.Errorf("Expected 12 notifications per docs/workflows/experience.md contract, got %d", len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}

// TestExperience_Example8_DirectCompletionFromJoined tests the most common
// completion flow: the owner wraps up directly from JOINED state without ever
// marking the experience as IN_PROCESS. The client calls RecordAttendance then
// CompleteExperience in sequence — the flow that failed in issue #1104.
// Reference: docs/workflows/experience.md - "8. Direct Completion from JOINED State"
//
// Preconditions:
// - A test community exists
// - Three users exist: an owner, participant A, and participant B
//
// Steps:
// 1. Owner creates experience → ACTIVE state
// 2. Owner shares experience with community
// 3. Participant A RSVPs Yes → Experience transitions to JOINED state
// 4. Participant B RSVPs Yes
// 5. Owner records attendance (YES for A, NO for B) — experience is in JOINED state
// 6. Owner completes experience → COMPLETED
//
// Postconditions:
// - The experience state is COMPLETED
// - Participant A has ATTENDED_STATUS_YES
// - Participant B has ATTENDED_STATUS_NO
// - CompleteExperience response includes ImpactEstimate with populated metrics.
func TestExperience_Example8_DirectCompletionFromJoined(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, _ := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// ========== PRECONDITIONS ==========

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)

	communityID := setupTestCommunity(t, ctx, ownerCommunityClient, "Joined Completion Community", "Testing direct completion from JOINED")

	participantAToken, participantAID := registerUserByInvite(t, serverURL, ownerToken, communityID, "a@example.com", "Participant A")
	participantBToken, participantBID := registerUserByInvite(t, serverURL, ownerToken, communityID, "b@example.com", "Participant B")

	participantAClient := createAuthExperienceClient(participantAToken, serverURL)
	participantBClient := createAuthExperienceClient(participantBToken, serverURL)

	var experienceID string

	// ========== STEPS ==========

	// Step 1: Owner creates experience → ACTIVE state.
	t.Run("Step1_CreateExperience", func(t *testing.T) {
		resp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Direct Completion Test",
			Description: "Complete directly from JOINED state",
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		experienceID = resp.Msg.Experience.Id
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			t.Errorf("Expected ACTIVE state, got %v", resp.Msg.Experience.State)
		}
	})

	// Step 2: Owner shares experience with community.
	t.Run("Step2_ShareExperience", func(t *testing.T) {
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, communityID)
	})

	// Step 3: Participant A RSVPs Yes → JOINED state.
	t.Run("Step3_ParticipantARSVPsYes", func(t *testing.T) {
		resp, err := participantAClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
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

	// Step 4: Participant B RSVPs Yes.
	t.Run("Step4_ParticipantBRSVPsYes", func(t *testing.T) {
		_, err := participantBClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
	})

	// Step 5: Owner records attendance while experience is in JOINED state.
	// This is the exact call sequence that failed in issue #1104.
	t.Run("Step5_RecordAttendanceFromJoinedState", func(t *testing.T) {
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
					Attended: api.AttendedStatus_ATTENDED_STATUS_NO,
				},
			},
		}))
		if err != nil {
			t.Fatalf("RecordAttendance from JOINED state failed: %v", err)
		}
	})

	// Step 6: Owner completes experience directly from JOINED state.
	var completeImpact *api.ImpactEstimate
	t.Run("Step6_CompleteExperienceFromJoined", func(t *testing.T) {
		resp, err := ownerExperienceClient.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: experienceID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience from JOINED state failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected COMPLETED state, got %v", resp.Msg.Experience.State)
		}
		completeImpact = resp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	// Postcondition: Experience is COMPLETED.
	t.Run("Postcondition_ExperienceCompleted", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		if resp.Msg.Experience.State != api.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected COMPLETED state, got %v", resp.Msg.Experience.State)
		}
	})

	// Postcondition: Attendance statuses match what was recorded.
	t.Run("Postcondition_AttendanceRecorded", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}

		attendanceMap := make(map[string]api.AttendedStatus)
		for _, rsvp := range resp.Msg.Rsvps {
			attendanceMap[rsvp.User.Id] = rsvp.Attended
		}

		if status, ok := attendanceMap[participantAID]; !ok {
			t.Errorf("Participant A not found in RSVPs")
		} else if status != api.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Participant A should have ATTENDED_STATUS_YES, got %v", status)
		}

		if status, ok := attendanceMap[participantBID]; !ok {
			t.Errorf("Participant B not found in RSVPs")
		} else if status != api.AttendedStatus_ATTENDED_STATUS_NO {
			t.Errorf("Participant B should have ATTENDED_STATUS_NO, got %v", status)
		}
	})

	// Postcondition: Impact estimate is populated.
	t.Run("Postcondition_ImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, completeImpact, "CompleteExperience from JOINED")
	})
}

// TestExperience_CrossCommunityCompletion reproduces #2690 end-to-end: an event
// shared to more than one community, where an attendee RSVPs via one community
// but the host completes with a different community context. Before the fix,
// RecordAttendance 404'd the whole batch ("no RSVP found for user in specified
// community") and the event could never complete. Attendance is event-scoped —
// the completion roster is cross-community — so the mismatched community must be
// tolerated and completion must succeed.
//
// Reference: docs/workflows/experience.md - "7. Recording Attendance"
// (Cross-Community Completion).
func TestExperience_CrossCommunityCompletion(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()
	_, serverURL, _ := startTestServerWithLogCapture(t, dbURL)

	ctx := context.Background()

	// ========== PRECONDITIONS ==========

	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner")
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerExperienceClient := createAuthExperienceClient(ownerToken, serverURL)

	// Two communities the owner controls. The event is shared to both; the
	// attendee RSVPs via C, but the host records attendance scoped to A.
	communityA := setupTestCommunity(t, ctx, ownerCommunityClient, "Community A", "Host's completion context")
	communityC := setupTestCommunity(t, ctx, ownerCommunityClient, "Community C", "Where the attendee RSVPs")

	// The attendee is a member of C (the community they RSVP through).
	attendeeToken, attendeeID := registerUserByInvite(t, serverURL, ownerToken, communityC, "attendee@example.com", "Attendee")
	attendeeExperienceClient := createAuthExperienceClient(attendeeToken, serverURL)

	var experienceID string

	// Owner creates the experience and shares it to both communities.
	saveResp, err := ownerExperienceClient.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Cross-Community Event",
		Description: "Shared to two communities",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	experienceID = saveResp.Msg.Experience.Id

	for _, cid := range []string{communityA, communityC} {
		shareExperienceIntoCommunity(t, ctx, ownerCommunityClient, experienceID, cid)
	}

	// The attendee RSVPs YES via Community C.
	if _, err := attendeeExperienceClient.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: experienceID,
		CommunityId:  communityC,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})); err != nil {
		t.Fatalf("RSVPToExperience (via Community C) failed: %v", err)
	}

	// ========== STEPS ==========

	// Owner records attendance scoped to Community A, even though the attendee's
	// RSVP lives in Community C. This is the call that 404'd before the fix.
	t.Run("Step_RecordAttendanceCrossCommunity", func(t *testing.T) {
		if _, err := ownerExperienceClient.RecordAttendance(ctx, connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: experienceID,
			CommunityId:  communityA, // Mismatched: RSVP is in Community C
			Attendance: []*api.AttendanceRecord{
				{UserId: attendeeID, Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
			},
		})); err != nil {
			t.Fatalf("cross-community RecordAttendance should succeed, got: %v", err)
		}
	})

	// Owner completes the experience; story generation runs inside with the
	// recorded attendance already set.
	var completeImpact *api.ImpactEstimate
	t.Run("Step_CompleteExperience", func(t *testing.T) {
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

	// The attendee shows as attended in the cross-community roster.
	t.Run("Postcondition_AttendeeMarkedAttended", func(t *testing.T) {
		resp, err := ownerExperienceClient.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          experienceID,
			CommunityId: communityC,
		}))
		if err != nil {
			t.Fatalf("GetExperience failed: %v", err)
		}
		var found bool
		for _, rsvp := range resp.Msg.Rsvps {
			if rsvp.User.Id == attendeeID {
				found = true
				if rsvp.Attended != api.AttendedStatus_ATTENDED_STATUS_YES {
					t.Errorf("attendee should have ATTENDED_STATUS_YES, got %v", rsvp.Attended)
				}
			}
		}
		if !found {
			t.Errorf("attendee %s not found in cross-community RSVP roster", attendeeID)
		}
	})

	// Impact reflects the attendee (proof completion accounted for them).
	t.Run("Postcondition_ImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, completeImpact, "cross-community completion")
	})
}
