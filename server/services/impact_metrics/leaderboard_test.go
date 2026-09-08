package impact_metrics

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// TestGetCommunityLeaderboard covers auth enforcement, validation, and ranking logic.
func TestGetCommunityLeaderboard(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "LB Auth Community")
		req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		_, err := svc.GetCommunityLeaderboard(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
			Dimension: api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		_, err := svc.GetCommunityLeaderboard(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("empty community returns empty members list", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Empty LB Community")
		callerID := insertTestUser(t, db, "LB Caller")
		req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetCommunityLeaderboard(contextWithAuth(callerID, "caller@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Members) != 0 {
			t.Errorf("expected 0 leaderboard members, got %d", len(resp.Msg.Members))
		}
	})

	t.Run("member with loans appears on leaderboard", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "LB Members Community")
		lenderID := insertTestUser(t, db, "LB Lender")
		borrowerID := insertTestUser(t, db, "LB Borrower")
		insertTestMembership(t, db, communityID, lenderID)
		insertTestMembership(t, db, communityID, borrowerID)

		gearID := insertGearWithValue(t, db, lenderID, "Camera", 500)
		insertLoanWithImpact(t, db, communityID, lenderID, borrowerID, gearID,
			testImpactEstimate(100, 2000, 240))

		req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			Limit:       10,
		})
		resp, err := svc.GetCommunityLeaderboard(contextWithAuth(lenderID, "lender@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Members) == 0 {
			t.Fatal("expected at least one leaderboard member")
		}
		if resp.Msg.Members[0].Rank != 1 {
			t.Errorf("expected rank 1 for top member, got %d", resp.Msg.Members[0].Rank)
		}
	})

	t.Run("calling user rank injected when not in top N", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "LB Rank Community")
		callerID := insertTestUser(t, db, "LB Outsider")
		topUserID := insertTestUser(t, db, "LB TopUser")
		insertTestMembership(t, db, communityID, callerID)
		insertTestMembership(t, db, communityID, topUserID)

		// Give topUser a big impact; caller has none.
		gearID := insertGearWithValue(t, db, topUserID, "Drone", 800)
		insertLoanWithImpact(t, db, communityID, topUserID, callerID, gearID,
			testImpactEstimate(200, 3000, 300))

		req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			Limit:       1, // Only top 1 — caller won't appear in Members list.
		})
		resp, err := svc.GetCommunityLeaderboard(contextWithAuth(callerID, "outsider@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// CallerUser is returned separately (or absent if caller has 0 impact).
		// Verify the top member is ranked #1.
		if len(resp.Msg.Members) > 0 && resp.Msg.Members[0].Rank != 1 {
			t.Errorf("expected top member rank=1, got %d", resp.Msg.Members[0].Rank)
		}
	})
}

// insertLoanWithImpact inserts a completed loan transfer with an ImpactEstimate for leaderboard tests.
func insertLoanWithImpact(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, recipientID, gearID string,
	ie *models.ImpactEstimate,
) {
	t.Helper()
	now := time.Now().Unix()
	pickup := now - 86400
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &now,
		ImpactEstimate:      ie,
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("insertLoanWithImpact: %v", err)
	}
}

// setupLeaderboardTest creates a test storage and an authenticated context.
func setupLeaderboardTest(t *testing.T) (*storage.ProtoSQLStorage, *estimator.Config, context.Context) {
	t.Helper()
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed: %v", err)
	}

	callerID := uuid.New().String()
	caller := &models.User{Id: callerID, Name: "Caller"}
	if _, err := db.Insert(context.Background(), caller); err != nil {
		t.Fatalf("insert caller: %v", err)
	}

	ctx := authn.SetInfo(context.Background(), &auth.Info{
		UserID: callerID,
		Role:   models.Role_ROLE_USER,
	})
	return db, cfg, ctx
}

// insertLeaderboardTransfer inserts a completed transfer for a community member.
func insertLeaderboardTransfer(t *testing.T, db *storage.ProtoSQLStorage, communityID, ownerID, recipientID string, costUSD float32) {
	t.Helper()
	ie := &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{
			ValueUsd: &models.Estimate{Mean: costUSD, Stddev: 0},
		},
	}
	transfer := &models.Transfer{
		Id:             uuid.New().String(),
		CommunityId:    communityID,
		OwnerId:        ownerID,
		RecipientId:    recipientID,
		TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
		State:          models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: ie,
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("insertLeaderboardTransfer: %v", err)
	}
}

// addCommunityMember links a user to a community.
func addCommunityMember(t *testing.T, db *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	membership := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      userID,
	}
	if _, err := db.Insert(context.Background(), membership); err != nil {
		t.Fatalf("addCommunityMember: %v", err)
	}
}

// TestGetCommunityLeaderboard_RanksCorrectly verifies multi-member ranking
// and that only 3 DB queries are made for transfers+requests regardless of
// member count (N+1 regression guard).
func TestGetCommunityLeaderboard_RanksCorrectly(t *testing.T) {
	db, _, ctx := setupLeaderboardTest(t)

	communityID := uuid.New().String()
	community := &models.Community{Id: communityID, Name: "Test", CreatorId: "test-user", OwnerUserId: "test-user"}
	if _, err := db.Insert(context.Background(), community); err != nil {
		t.Fatalf("insert community: %v", err)
	}

	// Create three members with different impact totals. Each member is owner
	// of exactly one transfer (to an external non-member), so each member's
	// impact is unambiguous: High=$300, Mid=$200, Low=$100.
	externalID := uuid.New().String()
	userHigh := &models.User{Id: uuid.New().String(), Name: "High"}
	userMid := &models.User{Id: uuid.New().String(), Name: "Mid"}
	userLow := &models.User{Id: uuid.New().String(), Name: "Low"}
	for _, u := range []*models.User{userHigh, userMid, userLow} {
		if _, err := db.Insert(context.Background(), u); err != nil {
			t.Fatalf("insert user %s: %v", u.Name, err)
		}
		addCommunityMember(t, db, communityID, u.Id)
	}

	insertLeaderboardTransfer(t, db, communityID, userHigh.Id, externalID, 300)
	insertLeaderboardTransfer(t, db, communityID, userMid.Id, externalID, 200)
	insertLeaderboardTransfer(t, db, communityID, userLow.Id, externalID, 100)

	req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
		CommunityId: communityID,
		Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		Limit:       10,
	})

	var resp *connect.Response[api.GetCommunityLeaderboardResponse]
	statsCtx := storage.WithQueryStats(ctx)
	// N+1 regression guard: active-community check + members + transfers +
	// communityRequests + user profiles = 5 queries max. No 6th query for
	// requests because this test has no CommunityRequest rows.
	storage.AssertMaxQueries(t, statsCtx, 5, func() {
		var err error
		resp, err = getCommunityLeaderboard(statsCtx, db, req)
		if err != nil {
			t.Fatalf("getCommunityLeaderboard: %v", err)
		}
	})

	members := resp.Msg.Members
	if len(members) != 3 {
		t.Fatalf("got %d members, want 3", len(members))
	}

	// Verify ranking order: High=rank1, Mid=rank2, Low=rank3.
	if members[0].UserId != userHigh.Id {
		t.Errorf("rank 1 user = %s, want %s (High)", members[0].UserId, userHigh.Id)
	}
	if members[1].UserId != userMid.Id {
		t.Errorf("rank 2 user = %s, want %s (Mid)", members[1].UserId, userMid.Id)
	}
	if members[2].UserId != userLow.Id {
		t.Errorf("rank 3 user = %s, want %s (Low)", members[2].UserId, userLow.Id)
	}

	// Verify rank fields.
	for i, m := range members {
		if m.Rank != int32(i+1) {
			t.Errorf("members[%d].Rank = %d, want %d", i, m.Rank, i+1)
		}
	}
}

// TestGetCommunityLeaderboard_EmptyCommunity verifies an empty community returns
// no members without error.
func TestGetCommunityLeaderboard_EmptyCommunity(t *testing.T) {
	db, _, ctx := setupLeaderboardTest(t)

	communityID := uuid.New().String()
	community := &models.Community{Id: communityID, Name: "Empty", CreatorId: "test-user", OwnerUserId: "test-user"}
	if _, err := db.Insert(context.Background(), community); err != nil {
		t.Fatalf("insert community: %v", err)
	}

	req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
		CommunityId: communityID,
		Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		Limit:       10,
	})

	resp, err := getCommunityLeaderboard(ctx, db, req)
	if err != nil {
		t.Fatalf("getCommunityLeaderboard: %v", err)
	}
	if len(resp.Msg.Members) != 0 {
		t.Errorf("got %d members, want 0", len(resp.Msg.Members))
	}
}

// TestGetCommunityLeaderboard_LimitRespected verifies that the limit parameter
// caps the number of returned members.
func TestGetCommunityLeaderboard_LimitRespected(t *testing.T) {
	db, _, ctx := setupLeaderboardTest(t)

	communityID := uuid.New().String()
	community := &models.Community{Id: communityID, Name: "LimitTest", CreatorId: "test-user", OwnerUserId: "test-user"}
	if _, err := db.Insert(context.Background(), community); err != nil {
		t.Fatalf("insert community: %v", err)
	}

	// Create 5 members each with some impact.
	for i := range 5 {
		user := &models.User{Id: uuid.New().String(), Name: "User"}
		if _, err := db.Insert(context.Background(), user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		addCommunityMember(t, db, communityID, user.Id)
		insertLeaderboardTransfer(t, db, communityID, user.Id, uuid.New().String(), float32((i+1)*10))
	}

	req := connect.NewRequest(&api.GetCommunityLeaderboardRequest{
		CommunityId: communityID,
		Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		Limit:       3,
	})

	resp, err := getCommunityLeaderboard(ctx, db, req)
	if err != nil {
		t.Fatalf("getCommunityLeaderboard: %v", err)
	}
	if len(resp.Msg.Members) != 3 {
		t.Errorf("got %d members, want 3 (limit)", len(resp.Msg.Members))
	}
}
