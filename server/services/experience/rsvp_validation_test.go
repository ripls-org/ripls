package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestService_RSVPToExperience_InProcessAllowed verifies that RSVP is accepted
// for an IN_PROCESS experience and rejected for COMPLETED and CANCELLED states.
func TestService_RSVPToExperience_InProcessAllowed(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner-ip", "owner-ip@example.com", "Owner")
	createTestUser(t, testStorage, "joiner-ip", "joiner-ip@example.com", "Joiner")

	ownerCtx := createAuthenticatedContext("owner-ip", "owner-ip@example.com", models.Role_ROLE_USER)
	joinerCtx := createAuthenticatedContext("joiner-ip", "joiner-ip@example.com", models.Role_ROLE_USER)

	communityID := createTestCommunity(t, testStorage, "IP Test Community", "owner-ip")
	createTestCommunityMembership(t, testStorage, communityID, "owner-ip")
	createTestCommunityMembership(t, testStorage, communityID, "joiner-ip")

	newExperience := func(t *testing.T) string {
		t.Helper()
		resp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Name: "In-Process RSVP Test",
		}))
		if err != nil {
			t.Fatalf("SaveExperience: %v", err)
		}
		expID := resp.Msg.Experience.Id
		shareExperienceForTest(t, service, ownerCtx, expID, communityID)
		return expID
	}

	t.Run("RSVP succeeds for IN_PROCESS experience", func(t *testing.T) {
		expID := newExperience(t)

		_, err := service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("MarkExperienceInProcess: %v", err)
		}

		_, err = service.RSVPToExperience(joinerCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience for IN_PROCESS experience failed: %v", err)
		}

		// Verify RSVP was recorded.
		rsvps, err := testStorage.QueryByFields(joinerCtx, map[string]any{
			"experience_id": expID,
			"user_id":       "joiner-ip",
		}, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("QueryByFields: %v", err)
		}
		if len(rsvps) != 1 {
			t.Fatalf("Expected 1 RSVP, got %d", len(rsvps))
		}
		if rsvps[0].(*models.ExperienceRSVP).GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
			t.Errorf("Expected intention YES, got %v", rsvps[0].(*models.ExperienceRSVP).GetIntention())
		}
	})

	t.Run("RSVP rejected for COMPLETED experience", func(t *testing.T) {
		expID := newExperience(t)

		_, err := service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("MarkExperienceInProcess: %v", err)
		}
		_, err = service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("CompleteExperience: %v", err)
		}

		_, err = service.RSVPToExperience(joinerCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err == nil {
			t.Fatal("Expected RSVP to COMPLETED experience to fail, but it succeeded")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition, got %v", connectErr.Code())
		}
	})

	t.Run("RSVP rejected for CANCELLED experience", func(t *testing.T) {
		expID := newExperience(t)

		_, err := service.CancelExperience(ownerCtx, connect.NewRequest(&api.CancelExperienceRequest{
			ExperienceId: expID,
		}))
		if err != nil {
			t.Fatalf("CancelExperience: %v", err)
		}

		_, err = service.RSVPToExperience(joinerCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err == nil {
			t.Fatal("Expected RSVP to CANCELLED experience to fail, but it succeeded")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition, got %v", connectErr.Code())
		}
	})
}

// TestService_RecordAttendance_SkillsWithCommunityID tests that skills are
// properly awarded when RSVPs have valid community_id, and that the fallback
// works when community_id is empty (legacy data).
// TestService_RecordAttendance_SkillsWithCommunityID removed - skill system has been removed.

// TestService_RSVPToExperience_RebuildsSFGroupSize verifies that after enough YES RSVPs to
// cross a group-size tier boundary the stored ImpactEstimate's SF score increases.
// Group size tiers: dyadic (≤2), small (3–5). The host is not double-counted.
func TestService_RSVPToExperience_RebuildsSFGroupSize(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner-sf", "owner-sf@test.com", "Owner SF")
	createTestUser(t, testStorage, "user-sf-1", "u1-sf@test.com", "User1")
	createTestUser(t, testStorage, "user-sf-2", "u2-sf@test.com", "User2")
	createTestUser(t, testStorage, "user-sf-3", "u3-sf@test.com", "User3")

	ownerCtx := createAuthenticatedContext("owner-sf", "owner-sf@test.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user-sf-1", "u1-sf@test.com", models.Role_ROLE_USER)
	user2Ctx := createAuthenticatedContext("user-sf-2", "u2-sf@test.com", models.Role_ROLE_USER)
	user3Ctx := createAuthenticatedContext("user-sf-3", "u3-sf@test.com", models.Role_ROLE_USER)

	communityID := createTestCommunity(t, testStorage, "SF Test Community", "owner-sf")
	createTestCommunityMembership(t, testStorage, communityID, "owner-sf")
	createTestCommunityMembership(t, testStorage, communityID, "user-sf-1")
	createTestCommunityMembership(t, testStorage, communityID, "user-sf-2")
	createTestCommunityMembership(t, testStorage, communityID, "user-sf-3")

	// Create and share the experience.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Group SF Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Read baseline QT score (set at creation, group size = 0/default → dyadic tier).
	baseline := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, baseline); err != nil {
		t.Fatalf("GetByID baseline: %v", err)
	}
	baselineQT := float32(0)
	if baseline.ImpactEstimate != nil && baseline.ImpactEstimate.QualityTime != nil {
		baselineQT = baseline.ImpactEstimate.QualityTime.QualityTimeMinutes.GetMean()
	}

	rsvp := func(ctx context.Context, label string) {
		t.Helper()
		_, err = service.RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience %s: %v", label, err)
		}
	}

	qtScore := func(label string) float32 {
		t.Helper()
		exp := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
			t.Fatalf("GetByID %s: %v", label, err)
		}
		if exp.ImpactEstimate != nil && exp.ImpactEstimate.QualityTime != nil {
			return exp.ImpactEstimate.QualityTime.QualityTimeMinutes.GetMean()
		}
		return 0
	}

	// RSVP 1 and 2 → group sizes 1 and 2 (both dyadic tier, ≤2); score may stay flat.
	rsvp(user1Ctx, "user1")
	sf1 := qtScore("after RSVP 1")
	if sf1 < baselineQT {
		t.Errorf("SF after 1 RSVP (%v) should not decrease from baseline (%v)", sf1, baselineQT)
	}

	rsvp(user2Ctx, "user2")
	sf2 := qtScore("after RSVP 2")
	if sf2 < sf1 {
		t.Errorf("SF after 2 RSVPs (%v) should not decrease from SF after 1 (%v)", sf2, sf1)
	}

	// RSVP 3 → group size 3 crosses into the small tier (factor 1.0 → 1.2); score must rise.
	rsvp(user3Ctx, "user3")
	sf3 := qtScore("after RSVP 3")
	if sf3 <= baselineQT {
		t.Errorf("SF after 3 RSVPs (%v) should exceed baseline (%v) after crossing dyadic→small tier", sf3, baselineQT)
	}

	// Other dimensions must not be touched.
	finalExp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, finalExp); err != nil {
		t.Fatalf("GetByID final: %v", err)
	}
	if finalExp.ImpactEstimate.TimeSaved == nil {
		t.Error("TimeSaved should be preserved after RSVP SF update")
	}
}

// TestService_RSVPOncePerExperience verifies that a user can only hold one RSVP
// per experience regardless of how many communities the experience is shared with.
// RSVPing via a second community must update the existing record, not create a new one.
func TestService_RSVPOncePerExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "user1", "user1@example.com", "User One")

	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user1", "user1@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")
	for _, uid := range []string{"owner1", "user1"} {
		createTestCommunityMembership(t, testStorage, communityA, uid)
		createTestCommunityMembership(t, testStorage, communityB, uid)
	}

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Multi-Community Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	for _, cid := range []string{communityA, communityB} {
		shareExperienceForTest(t, service, ownerCtx, expID, cid)
	}

	// User RSVPs via community A.
	_, err = service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityA,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience (community A) failed: %v", err)
	}

	// User RSVPs again via community B — must update, not create a second record.
	_, err = service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityB,
		Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience (community B) failed: %v", err)
	}

	// Exactly one RSVP record must exist across all communities.
	allRSVPs, err := testStorage.QueryByFields(ownerCtx, map[string]any{
		"experience_id": expID,
		"user_id":       "user1",
	}, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("QueryByFields failed: %v", err)
	}
	if len(allRSVPs) != 1 {
		t.Errorf("expected exactly 1 RSVP for user1 across all communities, got %d", len(allRSVPs))
	}
	if rsvp := allRSVPs[0].(*models.ExperienceRSVP); rsvp.GetIntention() != models.RSVPIntention_RSVP_INTENTION_MAYBE {
		t.Errorf("expected RSVP intention MAYBE (latest), got %v", rsvp.GetIntention())
	}
}

// TestService_RSVPToExperience_CommunityNotShared verifies that RSVPToExperience
// returns CodeNotFound when the supplied community_id does not match any community
// the experience is shared with. Without this check a stale communityId from the
// client would silently corrupt per-community attendance data.
func TestService_RSVPToExperience_CommunityNotShared(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "user1", "user1@example.com", "User One")

	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user1", "user1@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, testStorage, "Community A", "owner1")
	communityB := createTestCommunity(t, testStorage, "Community B", "owner1")

	// Only add users to community A.
	for _, uid := range []string{"owner1", "user1"} {
		createTestCommunityMembership(t, testStorage, communityA, uid)
		createTestCommunityMembership(t, testStorage, communityB, uid)
	}

	// Create an experience and share it only with community A.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Community Guard Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityA)

	t.Run("RSVP with wrong community returns CodeNotFound", func(t *testing.T) {
		_, err := service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityB, // experience is NOT shared with community B
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err == nil {
			t.Fatal("expected error when RSVPing with a non-shared community, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
		}

		// Verify no RSVP row was inserted.
		rsvps, qErr := testStorage.QueryByFields(ownerCtx, map[string]any{
			"experience_id": expID,
			"user_id":       "user1",
		}, &models.ExperienceRSVP{})
		if qErr != nil {
			t.Fatalf("QueryByFields failed: %v", qErr)
		}
		if len(rsvps) != 0 {
			t.Errorf("expected 0 RSVP rows after rejected RSVP, got %d", len(rsvps))
		}
	})

	t.Run("RSVP with correct community succeeds", func(t *testing.T) {
		_, err := service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityA,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience with correct community failed: %v", err)
		}

		rsvps, qErr := testStorage.QueryByFields(ownerCtx, map[string]any{
			"experience_id": expID,
			"user_id":       "user1",
		}, &models.ExperienceRSVP{})
		if qErr != nil {
			t.Fatalf("QueryByFields failed: %v", qErr)
		}
		if len(rsvps) != 1 {
			t.Fatalf("expected 1 RSVP row after successful RSVP, got %d", len(rsvps))
		}
		if rsvps[0].(*models.ExperienceRSVP).GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
			t.Errorf("expected intention YES, got %v", rsvps[0].(*models.ExperienceRSVP).GetIntention())
		}
	})

	t.Run("RSVP with empty community_id skips validation", func(t *testing.T) {
		// Empty community_id is treated as "no community context" — validation is skipped.
		createTestUser(t, testStorage, "user2", "user2@example.com", "User Two")
		user2Ctx := createAuthenticatedContext("user2", "user2@example.com", models.Role_ROLE_USER)

		_, err := service.RSVPToExperience(user2Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  "",
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience with empty community_id failed: %v", err)
		}
	})
}
