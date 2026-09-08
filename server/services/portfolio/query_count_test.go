package portfolio

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// Query-count guards for the portfolio assembly path.
//
// These exist to make the #2816 cyclop/funlen ratchet safe to run on this
// package. fetchAll (complexity 127, 570 lines) and computeWeeklyMetrics (97)
// are the two worst functions in the codebase and both sit behind GetHomeView /
// GetDirectoryPeople, which carry P95 SLOs and burn-rate alerts (#1613) and
// have already regressed once on latency (#2646).
//
// Splitting a function that fans out over storage is exactly how an N+1 gets
// introduced without any test noticing: the response is identical, only the
// query count changes. docs/server/profiling.md prescribes AssertMaxQueries for
// this; the portfolio package had no coverage at all before these.
//
// The bounds are empirical. To regenerate after an intentional change: drop the
// AssertMaxQueries wrapper, run the test, and read the count off GetQueryStats.
// Raising a bound to make a refactor pass is the regression this guards against
// — raise it only once you have confirmed the extra queries are intended.

type portfolioQueryFixture struct {
	svc         *Service
	store       *storage.ProtoSQLStorage
	ctx         context.Context
	userID      string
	communityID string
}

// setupPortfolioQueryFixture builds a small but non-trivial world: two users in
// one community, gear shared both ways, a request, and an experience with an
// RSVP. The point is breadth — every branch of the assembly path should have at
// least one row to walk, so a per-item query shows up in the count.
func setupPortfolioQueryFixture(t *testing.T) portfolioQueryFixture {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("load estimator config: %v", err)
	}
	svc := New(store, impact_metrics.NewMetricDetailCalculator(store, cfg))

	bg := context.Background()
	selfID, otherID := "pf-self", "pf-other"
	for _, u := range []string{selfID, otherID} {
		if _, err := store.Insert(bg, &models.User{Id: u, Name: "User " + u, Email: u + "@example.com"}); err != nil {
			t.Fatalf("insert user %s: %v", u, err)
		}
	}

	communityID, err := store.Insert(bg, &models.Community{
		Name: "Query Count Community", CreatorId: selfID, OwnerUserId: selfID,
	})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, u := range []string{selfID, otherID} {
		if _, err := store.Insert(bg, &models.CommunityUser{
			CommunityId: communityID, UserId: u, InviterId: selfID,
		}); err != nil {
			t.Fatalf("insert membership %s: %v", u, err)
		}
	}

	// Gear owned by each side, both shared into the community, so the assembly
	// walks the owned and the borrowable branches.
	for _, owner := range []string{selfID, otherID} {
		gearID, err := store.Insert(bg, &models.Gear{OwnerId: owner, Name: "Gear of " + owner})
		if err != nil {
			t.Fatalf("insert gear for %s: %v", owner, err)
		}
		if _, err := store.Insert(bg, &models.CommunityGear{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		}); err != nil {
			t.Fatalf("insert community gear for %s: %v", owner, err)
		}
	}

	requestID, err := store.Insert(bg, &models.Request{
		RequesterId: selfID, Title: "Need a ladder", State: models.RequestState_REQUEST_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := store.Insert(bg, &models.CommunityRequest{
		RequestId: requestID, CommunityId: communityID,
	}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}

	experienceID, err := store.Insert(bg, &models.Experience{
		OwnerId: selfID, Name: "Block Party", State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if _, err := store.Insert(bg, &models.CommunityExperience{
		ExperienceId: experienceID, CommunityId: communityID,
	}); err != nil {
		t.Fatalf("insert community experience: %v", err)
	}
	if _, err := store.Insert(bg, &models.ExperienceRSVP{
		ExperienceId: experienceID, UserId: otherID, CommunityId: communityID,
		Intention: models.RSVPIntention_RSVP_INTENTION_YES,
	}); err != nil {
		t.Fatalf("insert rsvp: %v", err)
	}

	ctx := authn.SetInfo(bg, &auth.Info{
		UserID: selfID, Email: selfID + "@example.com", Role: models.Role_ROLE_USER,
	})

	return portfolioQueryFixture{
		svc: svc, store: store, ctx: ctx, userID: selfID, communityID: communityID,
	}
}

// TestGetHomeView_QueryCount pins the query count for the Home view assembly,
// whose fetchAll is the highest-complexity function in the codebase and the
// first target of the #2816 cyclop ratchet.
func TestGetHomeView_QueryCount(t *testing.T) {
	fx := setupPortfolioQueryFixture(t)

	// Measured at 23 for this fixture; bounded at 30 to absorb minor drift.
	// Deliberately tight — a loose bound (the first draft used 120) would let a
	// 5x N+1 regression through and the guard would be decorative.
	const maxQueries = 30

	statsCtx := storage.WithQueryStats(fx.ctx)
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		if _, err := fx.svc.GetHomeView(statsCtx,
			connect.NewRequest(&api.GetHomeViewRequest{})); err != nil {
			t.Fatalf("GetHomeView: %v", err)
		}
	})
}

// TestGetDirectoryPeople_QueryCount pins the query count for the Directory's
// people list. It inherited this guard from GetPortfolioInboxView (#2830),
// which called the same fetchAll and has since been removed (#2834); the N+1
// regression it protects against is unchanged.
func TestGetDirectoryPeople_QueryCount(t *testing.T) {
	fx := setupPortfolioQueryFixture(t)

	// Bounded at 35 to absorb minor drift, matching the guard this replaced.
	const maxQueries = 35

	statsCtx := storage.WithQueryStats(fx.ctx)
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		if _, err := fx.svc.GetDirectoryPeople(statsCtx,
			connect.NewRequest(&api.GetDirectoryPeopleRequest{})); err != nil {
			t.Fatalf("GetDirectoryPeople: %v", err)
		}
	})
}
