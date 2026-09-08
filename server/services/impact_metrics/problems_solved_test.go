package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertScopedNeed inserts a planning need scoped to an experience, optionally
// claimed by a contribution (from_need_id set). A claimed need is "handled"; an
// unclaimed one counts only toward potential.
func insertScopedNeed(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	experienceID, claimerID string,
	claimed bool,
) {
	t.Helper()
	ctx := context.Background()
	need := &models.PlanningNeed{
		Id:             uuid.New().String(),
		Name:           "Bring chairs",
		Slots:          2,
		SlotsRemaining: 2,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: experienceID},
	}
	if _, err := db.Insert(ctx, need); err != nil {
		t.Fatalf("insertScopedNeed (need): %v", err)
	}
	if claimed {
		pc := &models.PlanningContribution{
			Id:            uuid.New().String(),
			ContributorId: claimerID,
			Title:         "Bring chairs",
			FromNeedId:    &need.Id,
			Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: experienceID},
		}
		if _, err := db.Insert(ctx, pc); err != nil {
			t.Fatalf("insertScopedNeed (contribution): %v", err)
		}
	}
}

func TestGetCommunityProblemsSolvedDetail(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityProblemsSolvedDetailRequest{})
		_, err := svc.GetCommunityProblemsSolvedDetail(contextWithAuth("u", "u@example.com"), req)
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("non-member is denied", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "PS Gated")
		req := connect.NewRequest(&api.GetCommunityProblemsSolvedDetailRequest{CommunityId: communityID})
		_, err := svc.GetCommunityProblemsSolvedDetail(contextWithAuth("outsider", "o@example.com"), req)
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("empty community returns zero", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "PS Empty")
		insertTestMembership(t, db, communityID, "u")
		req := connect.NewRequest(&api.GetCommunityProblemsSolvedDetailRequest{CommunityId: communityID})
		resp, err := svc.GetCommunityProblemsSolvedDetail(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.HandledCount != 0 || resp.Msg.PotentialCount != 0 || len(resp.Msg.Items) != 0 {
			t.Fatalf("expected empty, got handled=%d potential=%d items=%d",
				resp.Msg.HandledCount, resp.Msg.PotentialCount, len(resp.Msg.Items))
		}
	})

	t.Run("X-of-Y buckets: handled loan/request/need over potential; excludes giveaway", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "PS Buckets")
		ownerID := insertTestUser(t, db, "Owner")
		borrowerID := insertTestUser(t, db, "Borrower")
		helperID := insertTestUser(t, db, "Helper")
		insertTestMembership(t, db, communityID, ownerID)

		// Loan completed (handled + potential loan).
		gearID := insertGearWithValue(t, db, ownerID, "Tent", 200)
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID, nil)
		// Fulfilled request (handled + potential request).
		insertTestFulfilledRequest(t, db, communityID, ownerID, helperID, nil)
		// Needs: one claimed (handled + potential), one unclaimed (potential
		// only), both scoped to a community experience.
		expID := insertTestCompletedExperience(t, db, communityID, ownerID, nil)
		insertScopedNeed(t, db, expID, helperID, true)
		insertScopedNeed(t, db, expID, helperID, false)
		// Excluded: giveaway (never counts toward the loans bucket).
		giveGear := insertGearWithValue(t, db, ownerID, "Sleeping bag", 80)
		insertGiveawayTransfer(t, db, communityID, ownerID, borrowerID, giveGear, nil)

		req := connect.NewRequest(&api.GetCommunityProblemsSolvedDetailRequest{CommunityId: communityID})
		resp, err := svc.GetCommunityProblemsSolvedDetail(contextWithAuth(ownerID, "owner@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := resp.Msg
		if m.HandledLoans != 1 || m.HandledRequests != 1 || m.HandledNeeds != 1 {
			t.Fatalf("handled = loans:%d requests:%d needs:%d, want 1/1/1",
				m.HandledLoans, m.HandledRequests, m.HandledNeeds)
		}
		if m.PotentialLoans != 1 || m.PotentialRequests != 1 || m.PotentialNeeds != 2 {
			t.Fatalf("potential = loans:%d requests:%d needs:%d, want 1/1/2",
				m.PotentialLoans, m.PotentialRequests, m.PotentialNeeds)
		}
		if m.HandledCount != 3 || m.PotentialCount != 4 || len(m.Items) != 3 {
			t.Fatalf("handled=%d potential=%d items=%d, want 3/4/3",
				m.HandledCount, m.PotentialCount, len(m.Items))
		}
	})
}
