package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// shareViaNamedCommunityFixture sets up an experience the owner created (so it
// has an ad-hoc origin community) and ALSO shared to a named community that
// `member` belongs to but the owner-only origin community does not. Returns the
// experience id, the origin community id, and the named community id. This is
// the #2548 scenario: `member` can see the event only through the named
// community.
func shareViaNamedCommunityFixture(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, member string) (expID, originID, namedID string) {
	t.Helper()
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")
	createTestUser(t, testStorage, member, member+"@example.com", "Member")

	createResp, err := service.SaveExperience(ownerCtx,
		connect.NewRequest(&api.SaveExperienceRequest{Name: "Paddle", MaxParticipants: 10}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID = createResp.Msg.Experience.Id
	originID = originCommunityIDForTest(t, service, ownerCtx, expID)

	namedID = createTestCommunity(t, testStorage, "Book Club", "owner1")
	createTestCommunityMembership(t, testStorage, namedID, "owner1") // owner must be a member to share
	createTestCommunityMembership(t, testStorage, namedID, member)
	shareExperienceForTest(t, service, ownerCtx, expID, namedID)
	return expID, originID, namedID
}

// isActiveOriginMember reports whether userID has a live (not soft-deleted)
// CommunityUser row in communityID.
func isActiveOriginMember(t *testing.T, testStorage *storage.ProtoSQLStorage, ctx context.Context, communityID, userID string) bool {
	t.Helper()
	rows, err := testStorage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      userID,
	}, &models.CommunityUser{})
	if err != nil {
		t.Fatalf("query origin membership: %v", err)
	}
	return len(rows) > 0
}

// TestRSVPToExperience_PromotesYesRSVPerToOriginCommunity verifies #2548: when a
// user can see an event only through a named community and RSVPs yes, they are
// promoted into the event's ad-hoc origin community so the host sees them by name.
func TestRSVPToExperience_PromotesYesRSVPerToOriginCommunity(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	aliceCtx := createAuthenticatedContext("alice", "alice@example.com", models.Role_ROLE_USER)
	expID, originID, namedID := shareViaNamedCommunityFixture(t, service, testStorage, "alice")

	// Precondition: alice is not yet in the origin community.
	if isActiveOriginMember(t, testStorage, ownerCtx, originID, "alice") {
		t.Fatal("alice should not be an origin member before RSVP")
	}

	if _, err := service.RSVPToExperience(aliceCtx,
		connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID, CommunityId: namedID,
			Intention: api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	if !isActiveOriginMember(t, testStorage, ownerCtx, originID, "alice") {
		t.Error("alice was not promoted into the origin community after RSVP yes (#2548)")
	}
}

// TestRSVPToExperience_NoRSVPDoesNotPromote verifies a "not going" RSVP does NOT
// pull the user into the origin community.
func TestRSVPToExperience_NoRSVPDoesNotPromote(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)
	aliceCtx := createAuthenticatedContext("alice", "alice@example.com", models.Role_ROLE_USER)
	expID, originID, namedID := shareViaNamedCommunityFixture(t, service, testStorage, "alice")

	if _, err := service.RSVPToExperience(aliceCtx,
		connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID, CommunityId: namedID,
			Intention: api.RSVPIntention_RSVP_INTENTION_NO,
		})); err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	if isActiveOriginMember(t, testStorage, ownerCtx, originID, "alice") {
		t.Error("a 'no' RSVP should not promote the user into the origin community")
	}
}
