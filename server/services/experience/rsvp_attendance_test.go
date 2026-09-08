package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_RecordAttendance(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create community
	ctx := ownerCtx
	community := &models.Community{
		Id:          "community123",
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	_, err := testStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := community.Id

	// Add owner as community member
	ownerMembership := &models.CommunityUser{
		Id:          "owner123_community123",
		CommunityId: communityID,
		UserId:      "owner123",
	}
	_, err = testStorage.Insert(ctx, ownerMembership)
	if err != nil {
		t.Fatalf("Failed to add owner to community: %v", err)
	}

	// Add user as community member
	userMembership := &models.CommunityUser{
		Id:          "user456_community123",
		CommunityId: communityID,
		UserId:      "user456",
	}
	_, err = testStorage.Insert(ctx, userMembership)
	if err != nil {
		t.Fatalf("Failed to add user to community: %v", err)
	}

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Attendance Test Experience",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Create RSVP before marking in-process
	rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(userCtx, rsvpReq)
	if err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	// Mark in process
	markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})
	_, err = service.MarkExperienceInProcess(ownerCtx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark in process: %v", err)
	}

	t.Run("owner can record attendance", func(t *testing.T) {
		attendReq := connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Attendance: []*api.AttendanceRecord{
				{
					UserId:   "user456",
					Attended: api.AttendedStatus_ATTENDED_STATUS_YES,
				},
			},
		})

		_, err := service.RecordAttendance(ownerCtx, attendReq)
		if err != nil {
			t.Fatalf("RecordAttendance failed: %v", err)
		}

		// Verify attendance was recorded
		queryFields := map[string]any{
			"experience_id": expID,
			"user_id":       "user456",
		}
		rsvps, err := testStorage.QueryByFields(ownerCtx, queryFields, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("Failed to query RSVPs: %v", err)
		}

		if len(rsvps) != 1 {
			t.Fatalf("Expected 1 RSVP, got %d", len(rsvps))
		}

		rsvp := rsvps[0].(*models.ExperienceRSVP)
		if rsvp.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Expected attended status YES, got %v", rsvp.GetAttended())
		}
	})

	t.Run("non-owner cannot record attendance", func(t *testing.T) {
		attendReq := connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Attendance: []*api.AttendanceRecord{
				{
					UserId:   "user456",
					Attended: api.AttendedStatus_ATTENDED_STATUS_NO,
				},
			},
		})

		_, err := service.RecordAttendance(userCtx, attendReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to record attendance")
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

// TestService_RecordAttendance_CrossCommunity tests recording attendance when
// RSVP was created in a different community than the one specified in the request.
func TestService_RecordAttendance_CrossCommunity(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create two communities
	ctx := ownerCtx
	communityA := &models.Community{
		Id:          "communityA",
		Name:        "Community A",
		CreatorId:   "test-user-a",
		OwnerUserId: "test-user-a",
	}
	_, err := testStorage.Insert(ctx, communityA)
	if err != nil {
		t.Fatalf("Failed to create community A: %v", err)
	}

	communityB := &models.Community{
		Id:          "communityB",
		Name:        "Community B",
		CreatorId:   "test-user-b",
		OwnerUserId: "test-user-b",
	}
	_, err = testStorage.Insert(ctx, communityB)
	if err != nil {
		t.Fatalf("Failed to create community B: %v", err)
	}

	// Add users to both communities
	for _, communityID := range []string{communityA.Id, communityB.Id} {
		ownerMembership := &models.CommunityUser{
			Id:          "owner123_" + communityID,
			CommunityId: communityID,
			UserId:      "owner123",
		}
		_, err = testStorage.Insert(ctx, ownerMembership)
		if err != nil {
			t.Fatalf("Failed to add owner to community: %v", err)
		}

		userMembership := &models.CommunityUser{
			Id:          "user456_" + communityID,
			CommunityId: communityID,
			UserId:      "user456",
		}
		_, err = testStorage.Insert(ctx, userMembership)
		if err != nil {
			t.Fatalf("Failed to add user to community: %v", err)
		}
	}

	// Create experience and share to both communities
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Cross-Community Experience",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to both communities
	for _, communityID := range []string{communityA.Id, communityB.Id} {
		shareExperienceForTest(t, service, ownerCtx, expID, communityID)
	}

	// User RSVPs in Community A
	rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityA.Id,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(userCtx, rsvpReq)
	if err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	// Mark in process
	markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})
	_, err = service.MarkExperienceInProcess(ownerCtx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark in process: %v", err)
	}

	// Owner records attendance using Community B context even though user456's
	// RSVP lives in Community A. Attendance is event-scoped — the completion
	// roster is cross-community (buildRSVPs) — so this must succeed and mark the
	// existing Community A RSVP attended rather than 404 the whole batch (#2690).
	attendReq := connect.NewRequest(&api.RecordAttendanceRequest{
		ExperienceId: expID,
		CommunityId:  communityB.Id, // Mismatched: RSVP is in Community A
		Attendance: []*api.AttendanceRecord{
			{
				UserId:   "user456",
				Attended: api.AttendedStatus_ATTENDED_STATUS_YES,
			},
		},
	})

	if _, err = service.RecordAttendance(ownerCtx, attendReq); err != nil {
		t.Fatalf("Expected cross-community RecordAttendance to succeed, got: %v", err)
	}

	// The existing Community A RSVP row must now be marked attended.
	aRSVPs, err := testStorage.QueryByFields(ownerCtx, map[string]any{
		"experience_id": expID,
		"user_id":       "user456",
		"community_id":  communityA.Id,
	}, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("Failed to query Community A RSVPs: %v", err)
	}
	if len(aRSVPs) != 1 {
		t.Fatalf("Expected exactly 1 RSVP in Community A, got %d", len(aRSVPs))
	}
	if got := aRSVPs[0].(*models.ExperienceRSVP).GetAttended(); got != models.AttendedStatus_ATTENDED_STATUS_YES {
		t.Errorf("Expected Community A RSVP attended=YES, got %v", got)
	}

	// The fix must update the existing row, not fabricate a duplicate RSVP in
	// the request's community (Community B).
	bRSVPs, err := testStorage.QueryByFields(ownerCtx, map[string]any{
		"experience_id": expID,
		"user_id":       "user456",
		"community_id":  communityB.Id,
	}, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("Failed to query Community B RSVPs: %v", err)
	}
	if len(bRSVPs) != 0 {
		t.Errorf("Expected no RSVP fabricated in Community B, got %d", len(bRSVPs))
	}
}

// TestService_RecordAttendance_MultiCommunityBatch mirrors the real completion
// modal shape: a single RecordAttendance call scoped to one community carries a
// batch of attendees whose RSVPs are spread across several communities (the
// roster the modal shows is cross-community). It must mark each attendee on
// their own RSVP row wherever it lives, and it must do so without an N+1 read
// over the attendance list (#2690).
func TestService_RecordAttendance_MultiCommunityBatch(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner1@example.com", "Owner")
	createTestUser(t, testStorage, "userA", "userA@example.com", "User A")
	createTestUser(t, testStorage, "userB", "userB@example.com", "User B")
	createTestUser(t, testStorage, "userC", "userC@example.com", "User C")

	ownerCtx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")
	for _, uid := range []string{"owner1", "userA", "userB", "userC"} {
		createTestCommunityMembership(t, testStorage, communityA, uid)
		createTestCommunityMembership(t, testStorage, communityB, uid)
	}

	// Create the experience and share it to both communities.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Multi-Community Batch",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	for _, cid := range []string{communityA, communityB} {
		shareExperienceForTest(t, service, ownerCtx, expID, cid)
	}

	// userA and userC RSVP via Community A; userB RSVPs via Community B.
	rsvp := func(userID, email, communityID string) {
		t.Helper()
		userCtx := createAuthenticatedContext(userID, email, models.Role_ROLE_USER)
		if _, err := service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
			t.Fatalf("RSVP for %s in %s: %v", userID, communityID, err)
		}
	}
	rsvp("userA", "userA@example.com", communityA)
	rsvp("userB", "userB@example.com", communityB)
	rsvp("userC", "userC@example.com", communityA)

	// One completion request, scoped to Community A, confirming attendees whose
	// RSVPs live in both A (userA yes, userC no) and B (userB yes).
	attendance := []*api.AttendanceRecord{
		{UserId: "userA", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
		{UserId: "userB", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
		{UserId: "userC", Attended: api.AttendedStatus_ATTENDED_STATUS_NO},
	}
	// The read cost must not scale with the attendance list: one experience
	// fetch, one active-community check, and one RSVP fetch, plus one write per
	// attendee. A per-attendee read (the old behaviour) would blow this budget.
	statsCtx := storage.WithQueryStats(ownerCtx)
	storage.AssertMaxQueries(t, statsCtx, len(attendance)+4, func() {
		if _, err := service.RecordAttendance(statsCtx, connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: expID,
			CommunityId:  communityA,
			Attendance:   attendance,
		})); err != nil {
			t.Fatalf("RecordAttendance (multi-community batch) failed: %v", err)
		}
	})

	// Each attendee's own RSVP row — wherever it lives — carries the new status.
	assertAttended := func(userID, communityID string, want models.AttendedStatus) {
		t.Helper()
		rows, err := testStorage.QueryByFields(ownerCtx, map[string]any{
			"experience_id": expID,
			"user_id":       userID,
			"community_id":  communityID,
		}, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("query RSVP for %s: %v", userID, err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected exactly 1 RSVP for %s in %s, got %d", userID, communityID, len(rows))
		}
		if got := rows[0].(*models.ExperienceRSVP).GetAttended(); got != want {
			t.Errorf("%s: expected attended=%v, got %v", userID, want, got)
		}
	}
	assertAttended("userA", communityA, models.AttendedStatus_ATTENDED_STATUS_YES)
	assertAttended("userB", communityB, models.AttendedStatus_ATTENDED_STATUS_YES) // cross-community: request was scoped to A
	assertAttended("userC", communityA, models.AttendedStatus_ATTENDED_STATUS_NO)

	// No duplicate rows fabricated in the request's community for the
	// cross-community attendee.
	bDup, err := testStorage.QueryByFields(ownerCtx, map[string]any{
		"experience_id": expID,
		"user_id":       "userB",
		"community_id":  communityA,
	}, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("query userB Community A rows: %v", err)
	}
	if len(bDup) != 0 {
		t.Errorf("expected no fabricated userB RSVP in Community A, got %d", len(bDup))
	}
}

// TestService_RecordAttendance_JoinedState verifies that attendance can be
// recorded on a JOINED experience and that the full wrapUp flow (record
// attendance → complete) succeeds from JOINED state. This was the root cause
// of #1104: RecordAttendance rejected JOINED even though CompleteExperience
// accepted it, so the client's wrapUp sequence failed at the first call.
func TestService_RecordAttendance_JoinedState(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner-j", "owner-j@example.com", "Owner")
	createTestUser(t, testStorage, "user-j", "user-j@example.com", "Joiner")

	ownerCtx := createAuthenticatedContext("owner-j", "owner-j@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user-j", "user-j@example.com", models.Role_ROLE_USER)

	communityID := createTestCommunity(t, testStorage, "Joined Attendance Test", "owner-j")
	createTestCommunityMembership(t, testStorage, communityID, "owner-j")
	createTestCommunityMembership(t, testStorage, communityID, "user-j")

	// Create experience and share to community.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Joined State Attendance",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// RSVP transitions the experience to JOINED state.
	_, err = service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	// Verify experience is in JOINED state.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if exp.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		t.Fatalf("Expected JOINED state, got %v", exp.State)
	}

	t.Run("record attendance succeeds in JOINED state", func(t *testing.T) {
		attendReq := connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Attendance: []*api.AttendanceRecord{
				{
					UserId:   "user-j",
					Attended: api.AttendedStatus_ATTENDED_STATUS_YES,
				},
			},
		})

		_, err := service.RecordAttendance(ownerCtx, attendReq)
		if err != nil {
			t.Fatalf("RecordAttendance on JOINED experience failed: %v", err)
		}

		// Verify attendance was recorded.
		rsvps, err := testStorage.QueryByFields(ownerCtx, map[string]any{
			"experience_id": expID,
			"user_id":       "user-j",
		}, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("QueryByFields: %v", err)
		}
		if len(rsvps) != 1 {
			t.Fatalf("Expected 1 RSVP, got %d", len(rsvps))
		}
		if rsvps[0].(*models.ExperienceRSVP).GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
			t.Errorf("Expected attended YES, got %v", rsvps[0].(*models.ExperienceRSVP).GetAttended())
		}
	})

	t.Run("complete experience succeeds after attendance in JOINED state", func(t *testing.T) {
		// This mirrors the client's wrapUp() sequence: recordAttendance → completeExperience.
		_, err := service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience after attendance on JOINED experience failed: %v", err)
		}

		// Verify state is COMPLETED.
		completed := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID, completed); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if completed.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected COMPLETED state, got %v", completed.State)
		}
	})
}
