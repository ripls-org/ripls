package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// crossCommunityFixture holds the state for a completed experience shared to
// two communities with one attendee recorded in each.
type crossCommunityFixture struct {
	service      *Service
	experienceID string
	communityA   string
	communityB   string
	userA        string // attended in community A
	userB        string // attended in community B
}

// setupCrossCommunityScenario creates a fully completed experience shared to
// two communities, with user A attending in community A and user B attending in
// community B. This exercises the full lifecycle so RSVPs have Attended="YES".
func setupCrossCommunityScenario(t *testing.T) crossCommunityFixture {
	t.Helper()

	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner1@example.com", "Owner")
	createTestUser(t, testStorage, "userA", "userA@example.com", "User A")
	createTestUser(t, testStorage, "userB", "userB@example.com", "User B")

	ownerCtx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)
	userACtx := createAuthenticatedContext("userA", "userA@example.com", models.Role_ROLE_USER)
	userBCtx := createAuthenticatedContext("userB", "userB@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")

	for _, uid := range []string{"owner1", "userA", "userB"} {
		createTestCommunityMembership(t, testStorage, communityA, uid)
		createTestCommunityMembership(t, testStorage, communityB, uid)
	}

	// Create experience and share to both communities.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Cross-Community Experience",
	}))
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	for _, cid := range []string{communityA, communityB} {
		shareExperienceForTest(t, service, ownerCtx, expID, cid)
	}

	// User A RSVPs in community A, user B in community B.
	_, err = service.RSVPToExperience(userACtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityA,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("User A RSVP failed: %v", err)
	}
	_, err = service.RSVPToExperience(userBCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityB,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("User B RSVP failed: %v", err)
	}

	// Complete the full lifecycle: mark in-process → record attendance → complete.
	_, err = service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	_, err = service.RecordAttendance(ownerCtx, connect.NewRequest(&api.RecordAttendanceRequest{
		ExperienceId: expID,
		CommunityId:  communityA,
		Attendance: []*api.AttendanceRecord{
			{UserId: "userA", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
		},
	}))
	if err != nil {
		t.Fatalf("RecordAttendance (community A) failed: %v", err)
	}

	_, err = service.RecordAttendance(ownerCtx, connect.NewRequest(&api.RecordAttendanceRequest{
		ExperienceId: expID,
		CommunityId:  communityB,
		Attendance: []*api.AttendanceRecord{
			{UserId: "userB", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
		},
	}))
	if err != nil {
		t.Fatalf("RecordAttendance (community B) failed: %v", err)
	}

	_, err = service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	return crossCommunityFixture{
		service:      service,
		experienceID: expID,
		communityA:   communityA,
		communityB:   communityB,
		userA:        "userA",
		userB:        "userB",
	}
}

// TestGetExperiencePeople_CrossCommunityIsolation verifies that
// GetExperiencePeople only returns attendees from the requested community.
func TestGetExperiencePeople_CrossCommunityIsolation(t *testing.T) {
	f := setupCrossCommunityScenario(t)
	ctx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)

	// Request people for community A — should only include user A.
	resp, err := f.service.GetExperiencePeople(ctx, connect.NewRequest(&api.GetExperiencePeopleRequest{
		ExperienceId: f.experienceID,
		CommunityId:  f.communityA,
	}))
	if err != nil {
		t.Fatalf("GetExperiencePeople failed: %v", err)
	}

	attendeeIDs := make(map[string]bool)
	for _, u := range resp.Msg.PastAttendees {
		attendeeIDs[u.Id] = true
	}

	if !attendeeIDs[f.userA] {
		t.Errorf("Expected user A (%s) in community A past attendees", f.userA)
	}
	if attendeeIDs[f.userB] {
		t.Errorf("User B (%s) from community B leaked into community A results", f.userB)
	}

	// Request people for community B — should only include user B.
	respB, err := f.service.GetExperiencePeople(ctx, connect.NewRequest(&api.GetExperiencePeopleRequest{
		ExperienceId: f.experienceID,
		CommunityId:  f.communityB,
	}))
	if err != nil {
		t.Fatalf("GetExperiencePeople (community B) failed: %v", err)
	}

	attendeeIDsB := make(map[string]bool)
	for _, u := range respB.Msg.PastAttendees {
		attendeeIDsB[u.Id] = true
	}

	if !attendeeIDsB[f.userB] {
		t.Errorf("Expected user B (%s) in community B past attendees", f.userB)
	}
	if attendeeIDsB[f.userA] {
		t.Errorf("User A (%s) from community A leaked into community B results", f.userA)
	}
}

// TestGetExperience_RSVPs_AreCrossCommunity verifies that the RSVPs returned
// by GetExperience are NOT community-scoped: access to the event is gated by
// shared-community membership, but once a viewer can see the event they see
// every attendee regardless of which community each RSVP was registered in.
//
// "Who's going" is a property of the event, not of the community lens.
func TestGetExperience_RSVPs_AreCrossCommunity(t *testing.T) {
	f := setupCrossCommunityScenario(t)
	ctx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)

	for _, cid := range []string{f.communityA, f.communityB} {
		resp, err := f.service.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
			Id:          f.experienceID,
			CommunityId: cid,
		}))
		if err != nil {
			t.Fatalf("GetExperience (community %s) failed: %v", cid, err)
		}

		rsvpUserIDs := make(map[string]bool)
		for _, rsvp := range resp.Msg.Rsvps {
			rsvpUserIDs[rsvp.User.Id] = true
		}

		if !rsvpUserIDs[f.userA] {
			t.Errorf("community lens %s: expected user A (%s) in cross-community RSVP list", cid, f.userA)
		}
		if !rsvpUserIDs[f.userB] {
			t.Errorf("community lens %s: expected user B (%s) in cross-community RSVP list", cid, f.userB)
		}
	}
}

// TestGetExperienceStats_CrossCommunityIsolation verifies that stats are
// scoped to the requested community.
func TestGetExperienceStats_CrossCommunityIsolation(t *testing.T) {
	f := setupCrossCommunityScenario(t)
	ctx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)

	// Stats for community A — should see 1 attendee (user A only).
	statsA, err := f.service.GetExperienceStats(ctx, connect.NewRequest(&api.GetExperienceStatsRequest{
		ExperienceId: f.experienceID,
		CommunityId:  f.communityA,
	}))
	if err != nil {
		t.Fatalf("GetExperienceStats (community A) failed: %v", err)
	}
	if statsA.Msg.TotalAttendees != 1 {
		t.Errorf("Community A: expected 1 attendee, got %d (cross-community leakage?)", statsA.Msg.TotalAttendees)
	}

	// Stats for community B — should see 1 attendee (user B only).
	statsB, err := f.service.GetExperienceStats(ctx, connect.NewRequest(&api.GetExperienceStatsRequest{
		ExperienceId: f.experienceID,
		CommunityId:  f.communityB,
	}))
	if err != nil {
		t.Fatalf("GetExperienceStats (community B) failed: %v", err)
	}
	if statsB.Msg.TotalAttendees != 1 {
		t.Errorf("Community B: expected 1 attendee, got %d (cross-community leakage?)", statsB.Msg.TotalAttendees)
	}
}
