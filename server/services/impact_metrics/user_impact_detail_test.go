package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TestGetUserCommunityImpactDetail covers auth enforcement, validation, and basic happy path.
func TestGetUserCommunityImpactDetail(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("non-member caller is denied", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "UCD Non-Member Community")
		userID := insertTestUser(t, db, "UCD Target User")
		callerID := insertTestUser(t, db, "UCD Non-Member Caller")
		insertTestMembership(t, db, communityID, userID)
		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			CommunityId: communityID,
			UserId:      userID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		_, err := svc.GetUserCommunityImpactDetail(contextWithAuth(callerID, "caller@example.com"), req)
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "UCD Auth Community")
		userID := insertTestUser(t, db, "UCD Auth User")
		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			CommunityId: communityID,
			UserId:      userID,
		})
		_, err := svc.GetUserCommunityImpactDetail(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		userID := insertTestUser(t, db, "UCD Missing Comm User")
		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			UserId: userID,
		})
		_, err := svc.GetUserCommunityImpactDetail(contextWithAuth(userID, "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("missing user_id returns InvalidArgument", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "UCD Missing User Community")
		callerID := insertTestUser(t, db, "UCD Caller")
		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetUserCommunityImpactDetail(contextWithAuth(callerID, "caller@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing user_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("user with no activity returns empty transactions list", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "UCD Empty Community")
		userID := insertTestUser(t, db, "UCD Empty User")
		insertTestMembership(t, db, communityID, userID)
		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			CommunityId: communityID,
			UserId:      userID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetUserCommunityImpactDetail(contextWithAuth(userID, "empty@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Transactions) != 0 {
			t.Errorf("expected 0 transactions, got %d", len(resp.Msg.Transactions))
		}
	})

	t.Run("completed loan appears in transactions for lender", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "UCD Loan Community")
		lenderID := insertTestUser(t, db, "UCD Lender")
		borrowerID := insertTestUser(t, db, "UCD Borrower")
		insertTestMembership(t, db, communityID, lenderID)
		gearID := insertGearWithValue(t, db, lenderID, "Projector", 300)
		insertTestCompletedTransfer(t, db, communityID, lenderID, borrowerID, gearID,
			testImpactEstimate(50, 1000, 90))

		req := connect.NewRequest(&api.GetUserCommunityImpactDetailRequest{
			CommunityId: communityID,
			UserId:      lenderID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetUserCommunityImpactDetail(contextWithAuth(lenderID, "lender@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Transactions) != 1 {
			t.Errorf("expected 1 transaction, got %d", len(resp.Msg.Transactions))
		}
	})
}
