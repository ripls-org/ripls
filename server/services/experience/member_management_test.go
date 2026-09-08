package experience

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestExperienceMemberManagement covers the Who's In host controls (#2492):
// the host sets a directly-invited member's RSVP and uninvites them, and
// non-owners are rejected.
func TestExperienceMemberManagement(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	for _, u := range []struct{ id, email, name string }{
		{"owner1", "owner@example.com", "Owner"},
		{"alice", "alice@example.com", "Alice"},
		{"mallory", "mallory@example.com", "Mallory"},
	} {
		createTestUser(t, testStorage, u.id, u.email, u.name)
	}
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	malloryCtx := createAuthenticatedContext("mallory", "mallory@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Resolve the origin community and directly invite alice into it.
	resp1, err := service.GetExperience(ownerCtx,
		connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	var originID string
	for _, sc := range resp1.Msg.SharedCommunities {
		if sc.IsOriginCommunity {
			originID = sc.CommunityId
		}
	}
	if originID == "" {
		t.Fatal("no origin community")
	}
	createTestCommunityMembership(t, testStorage, originID, "alice")

	invitedIDs := func() []string {
		r, err := service.GetExperience(ownerCtx,
			connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("GetExperience: %v", err)
		}
		ids := make([]string, 0, len(r.Msg.InvitedIndividuals))
		for _, u := range r.Msg.InvitedIndividuals {
			ids = append(ids, u.Id)
		}
		return ids
	}
	goingIDs := func() []string {
		r, err := service.GetExperience(ownerCtx,
			connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
		if err != nil {
			t.Fatalf("GetExperience: %v", err)
		}
		var ids []string
		for _, rs := range r.Msg.Rsvps {
			if rs.Intention == api.RSVPIntention_RSVP_INTENTION_YES && rs.User != nil {
				ids = append(ids, rs.User.Id)
			}
		}
		return ids
	}

	t.Run("alice starts as a directly-invited individual", func(t *testing.T) {
		if got := invitedIDs(); !reflect.DeepEqual(got, []string{"alice"}) {
			t.Errorf("InvitedIndividuals = %v, want [alice]", got)
		}
	})

	t.Run("non-owner cannot set a member's RSVP", func(t *testing.T) {
		_, err := service.SetExperienceMemberRSVP(malloryCtx,
			connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
				ExperienceId: expID,
				MemberUserId: "alice",
				Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
			}))
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("host sets alice to going", func(t *testing.T) {
		if _, err := service.SetExperienceMemberRSVP(ownerCtx,
			connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
				ExperienceId: expID,
				MemberUserId: "alice",
				Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
			})); err != nil {
			t.Fatalf("SetExperienceMemberRSVP: %v", err)
		}
		if got := goingIDs(); !slices.Contains(got, "alice") {
			t.Errorf("going = %v, want alice", got)
		}
		if got := invitedIDs(); len(got) != 0 {
			t.Errorf("InvitedIndividuals = %v, want empty after RSVP", got)
		}
	})

	t.Run("host resets alice to invited", func(t *testing.T) {
		if _, err := service.SetExperienceMemberRSVP(ownerCtx,
			connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
				ExperienceId: expID,
				MemberUserId: "alice",
				Intention:    api.RSVPIntention_RSVP_INTENTION_UNSPECIFIED,
			})); err != nil {
			t.Fatalf("SetExperienceMemberRSVP reset: %v", err)
		}
		if got := goingIDs(); slices.Contains(got, "alice") {
			t.Errorf("going still has alice after reset: %v", got)
		}
		if got := invitedIDs(); !slices.Contains(got, "alice") {
			t.Errorf("alice not back in Invited after reset: %v", got)
		}
	})

	t.Run("host removes alice", func(t *testing.T) {
		if _, err := service.RemoveExperienceMember(ownerCtx,
			connect.NewRequest(&api.RemoveExperienceMemberRequest{
				ExperienceId: expID,
				MemberUserId: "alice",
			})); err != nil {
			t.Fatalf("RemoveExperienceMember: %v", err)
		}
		if got := goingIDs(); slices.Contains(got, "alice") {
			t.Errorf("going still has alice: %v", got)
		}
		if got := invitedIDs(); slices.Contains(got, "alice") {
			t.Errorf("invited still has alice: %v", got)
		}
		// Alice's origin membership is soft-deleted (no active row).
		members, err := testStorage.QueryByFields(context.Background(), map[string]any{
			"community_id": originID,
			"user_id":      "alice",
		}, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("query members: %v", err)
		}
		if len(members) != 0 {
			t.Errorf("alice's origin membership not soft-deleted (%d active rows)", len(members))
		}
	})

	t.Run("cannot remove the host", func(t *testing.T) {
		_, err := service.RemoveExperienceMember(ownerCtx,
			connect.NewRequest(&api.RemoveExperienceMemberRequest{
				ExperienceId: expID,
				MemberUserId: "owner1",
			}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})
}

// TestSetExperienceMemberRSVP_PublishesStreamEvent verifies that a host-set RSVP
// emits an EXPERIENCE_RSVP_* community event to the event's origin community so
// the affected member's (and other viewers') open screens refresh live. The
// actor is the host (skipped on broadcast — they refresh locally) and there is
// no notification target (ObjectUserId empty) so no push fires (#2492).
func TestSetExperienceMemberRSVP_PublishesStreamEvent(t *testing.T) {
	service, testStorage, mockBus := setupTestServiceWithMockBus(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "alice", "alice@example.com", "Alice")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	resp1, err := service.GetExperience(ownerCtx,
		connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	var originID string
	for _, sc := range resp1.Msg.SharedCommunities {
		if sc.IsOriginCommunity {
			originID = sc.CommunityId
		}
	}
	if originID == "" {
		t.Fatal("no origin community")
	}
	createTestCommunityMembership(t, testStorage, originID, "alice")

	mockBus.Reset() // drop events from setup

	if _, err := service.SetExperienceMemberRSVP(ownerCtx,
		connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
			ExperienceId: expID,
			MemberUserId: "alice",
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
		t.Fatalf("SetExperienceMemberRSVP: %v", err)
	}

	var found *models.CommunityEvent
	for _, e := range mockBus.Captured() {
		if e.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES &&
			e.GetExperienceId() == expID {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("expected an EXPERIENCE_RSVP_YES event for the experience, got %d events",
			len(mockBus.Captured()))
	}
	if found.CommunityId != originID {
		t.Errorf("event community = %q, want origin %q", found.CommunityId, originID)
	}
	if found.ActorId != "owner1" {
		t.Errorf("event actor = %q, want host owner1", found.ActorId)
	}
	if found.ObjectUserId != "" {
		t.Errorf("event ObjectUserId = %q, want empty (no push)", found.ObjectUserId)
	}
}

// rosterChangedFor returns the EXPERIENCE_ROSTER_CHANGED events captured for the
// given experience.
func rosterChangedFor(events []*models.CommunityEvent, expID string) []*models.CommunityEvent {
	var out []*models.CommunityEvent
	for _, e := range events {
		if e.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED &&
			e.GetExperienceId() == expID {
			out = append(out, e)
		}
	}
	return out
}

// TestSetExperienceMemberRSVP_ResetToInvited_PublishesRosterChanged verifies the
// reset-to-invited path (Intention UNSPECIFIED) streams a silent
// EXPERIENCE_ROSTER_CHANGED so the member moves back to Invited on open screens
// live. No RSVP_* event maps to "reset", so this is the only signal (#2492).
func TestSetExperienceMemberRSVP_ResetToInvited_PublishesRosterChanged(t *testing.T) {
	service, testStorage, mockBus := setupTestServiceWithMockBus(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "alice", "alice@example.com", "Alice")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	originID := originCommunityIDForTest(t, service, ownerCtx, expID)
	createTestCommunityMembership(t, testStorage, originID, "alice")

	// Give alice a going RSVP so reset has something to clear.
	if _, err := service.SetExperienceMemberRSVP(ownerCtx,
		connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
			ExperienceId: expID, MemberUserId: "alice",
			Intention: api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
		t.Fatalf("seed RSVP: %v", err)
	}

	mockBus.Reset()
	if _, err := service.SetExperienceMemberRSVP(ownerCtx,
		connect.NewRequest(&api.SetExperienceMemberRSVPRequest{
			ExperienceId: expID, MemberUserId: "alice",
			Intention: api.RSVPIntention_RSVP_INTENTION_UNSPECIFIED,
		})); err != nil {
		t.Fatalf("SetExperienceMemberRSVP reset: %v", err)
	}

	got := rosterChangedFor(mockBus.Captured(), expID)
	if len(got) == 0 {
		t.Fatalf("expected a ROSTER_CHANGED event on reset, got none of %d events", len(mockBus.Captured()))
	}
	if got[0].CommunityId != originID {
		t.Errorf("roster-changed community = %q, want origin %q", got[0].CommunityId, originID)
	}
	if got[0].ObjectUserId != "" {
		t.Errorf("roster-changed ObjectUserId = %q, want empty (no push)", got[0].ObjectUserId)
	}
}

// TestRemoveExperienceMember_PublishesRosterChanged verifies removing a member
// streams a silent EXPERIENCE_ROSTER_CHANGED so open rosters drop them live.
func TestRemoveExperienceMember_PublishesRosterChanged(t *testing.T) {
	service, testStorage, mockBus := setupTestServiceWithMockBus(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "alice", "alice@example.com", "Alice")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	originID := originCommunityIDForTest(t, service, ownerCtx, expID)
	createTestCommunityMembership(t, testStorage, originID, "alice")

	mockBus.Reset()
	if _, err := service.RemoveExperienceMember(ownerCtx,
		connect.NewRequest(&api.RemoveExperienceMemberRequest{
			ExperienceId: expID, MemberUserId: "alice",
		})); err != nil {
		t.Fatalf("RemoveExperienceMember: %v", err)
	}

	got := rosterChangedFor(mockBus.Captured(), expID)
	if len(got) == 0 {
		t.Fatalf("expected a ROSTER_CHANGED event on remove, got none of %d events", len(mockBus.Captured()))
	}
	if got[0].ActorId != "owner1" {
		t.Errorf("roster-changed actor = %q, want host owner1", got[0].ActorId)
	}
	if got[0].ObjectUserId != "" {
		t.Errorf("roster-changed ObjectUserId = %q, want empty (no push)", got[0].ObjectUserId)
	}
}

// TestRosterChange_FansOutToSiblingCommunities verifies a roster change reaches
// EVERY community the experience is shared to, not just its origin — so a
// co-invitee streaming a sibling (named) community sees the update live (#2492).
func TestRosterChange_FansOutToSiblingCommunities(t *testing.T) {
	service, testStorage, mockBus := setupTestServiceWithMockBus(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "alice", "alice@example.com", "Alice")
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle"}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	originID := originCommunityIDForTest(t, service, ownerCtx, expID)
	createTestCommunityMembership(t, testStorage, originID, "alice")

	// The experience is also shared to a second (named) community.
	siblingID := createTestCommunity(t, testStorage, "Book Club", "owner1")
	if _, err := testStorage.Insert(ownerCtx, &models.CommunityExperience{
		ExperienceId: expID, CommunityId: siblingID,
	}); err != nil {
		t.Fatalf("insert sibling CommunityExperience: %v", err)
	}

	mockBus.Reset()
	if _, err := service.RemoveExperienceMember(ownerCtx,
		connect.NewRequest(&api.RemoveExperienceMemberRequest{
			ExperienceId: expID, MemberUserId: "alice",
		})); err != nil {
		t.Fatalf("RemoveExperienceMember: %v", err)
	}

	reached := map[string]bool{}
	for _, e := range rosterChangedFor(mockBus.Captured(), expID) {
		reached[e.CommunityId] = true
	}
	if !reached[originID] {
		t.Errorf("roster-changed did not reach origin community %q", originID)
	}
	if !reached[siblingID] {
		t.Errorf("roster-changed did not reach sibling community %q (fan-out missing)", siblingID)
	}
}

// originCommunityIDForTest resolves an experience's origin (per-item) community
// id via GetExperience, failing the test if there isn't one.
func originCommunityIDForTest(t *testing.T, service *Service, ctx context.Context, expID string) string {
	t.Helper()
	resp, err := service.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{Id: expID}))
	if err != nil {
		t.Fatalf("GetExperience: %v", err)
	}
	for _, sc := range resp.Msg.SharedCommunities {
		if sc.IsOriginCommunity {
			return sc.CommunityId
		}
	}
	t.Fatal("no origin community")
	return ""
}
