package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGetUserImpactMetrics covers validation, auth-derived fields, and user profile assembly.
func TestGetUserImpactMetrics(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("missing user_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{})
		_, err := svc.GetUserImpactMetrics(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing user_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("user with no activity returns zero metrics without error", func(t *testing.T) {
		userID := insertTestUser(t, db, "Carol")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{
			UserId: userID,
		})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "carol@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.Metrics == nil {
			t.Fatal("expected non-nil Metrics")
		}
		if resp.Msg.Metrics.TotalLoans != 0 {
			t.Errorf("expected 0 total loans, got %d", resp.Msg.Metrics.TotalLoans)
		}
	})

	t.Run("is_own_profile true when caller matches requested user_id", func(t *testing.T) {
		userID := insertTestUser(t, db, "Dave")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "dave@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Msg.IsOwnProfile {
			t.Error("expected IsOwnProfile=true for caller viewing own profile")
		}
	})

	t.Run("is_own_profile false when caller views another user in shared community", func(t *testing.T) {
		targetID := insertTestUser(t, db, "Eve")
		callerID := insertTestUser(t, db, "Frank")
		sharedCommunityID := insertTestCommunity(t, db, "Shared Community Frank Eve")
		insertTestMembership(t, db, sharedCommunityID, callerID)
		insertTestMembership(t, db, sharedCommunityID, targetID)
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: targetID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(callerID, "frank@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.IsOwnProfile {
			t.Error("expected IsOwnProfile=false when caller views another user")
		}
	})

	t.Run("caller with no shared community is denied", func(t *testing.T) {
		targetID := insertTestUser(t, db, "Grace")
		callerID := insertTestUser(t, db, "Henry")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: targetID})
		_, err := svc.GetUserImpactMetrics(contextWithAuth(callerID, "henry@example.com"), req)
		if err == nil {
			t.Fatal("expected PermissionDenied for caller with no shared community")
		}
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		userID := insertTestUser(t, db, "Iris")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		_, err := svc.GetUserImpactMetrics(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for unauthenticated call")
		}
	})

	t.Run("response carries user profile fields", func(t *testing.T) {
		mediaID := uuid.New().String()
		user := &models.User{
			Id:          uuid.New().String(),
			Name:        "Harriet",
			Description: "A community sharer",
			MediaIds:    []string{mediaID},
		}
		userID, err := db.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "harriet@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.UserName != "Harriet" {
			t.Errorf("expected UserName=%q, got %q", "Harriet", resp.Msg.UserName)
		}
		if resp.Msg.UserDescription != "A community sharer" {
			t.Errorf("expected UserDescription=%q, got %q", "A community sharer", resp.Msg.UserDescription)
		}
		if resp.Msg.UserMediaId != mediaID {
			t.Errorf("expected UserMediaId=%q, got %q", mediaID, resp.Msg.UserMediaId)
		}
	})

	t.Run("response includes location name when user has primary residence", func(t *testing.T) {
		locName := "SF"
		location := &models.Location{
			Id:   uuid.New().String(),
			Name: &locName,
		}
		locationID, err := db.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("insert location: %v", err)
		}
		user := &models.User{
			Id:                         uuid.New().String(),
			Name:                       "Ivan",
			PrimaryResidenceLocationId: locationID,
		}
		userID, err := db.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "ivan@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.UserLocationName != "SF" {
			t.Errorf("expected UserLocationName=%q, got %q", "SF", resp.Msg.UserLocationName)
		}
	})

	t.Run("activity counts reflect inserted transfers", func(t *testing.T) {
		ownerID := insertTestUser(t, db, "Janet")
		borrowerID := insertTestUser(t, db, "Karl")
		communityID := insertTestCommunity(t, db, "Activity Community")
		gearID := insertGearWithValue(t, db, ownerID, "Tent", 200)

		// Two loans owned by ownerID.
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID, testImpactEstimate(20, 0, 0))
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID, testImpactEstimate(20, 0, 0))
		// One loan received by ownerID (ownerID borrows from borrowerID).
		insertTestCompletedTransfer(t, db, communityID, borrowerID, ownerID, gearID, testImpactEstimate(15, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: ownerID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(ownerID, "janet@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.Metrics.TotalLoans != 2 {
			t.Errorf("expected TotalLoans=2, got %d", resp.Msg.Metrics.TotalLoans)
		}
		if resp.Msg.Metrics.TotalBorrows != 1 {
			t.Errorf("expected TotalBorrows=1, got %d", resp.Msg.Metrics.TotalBorrows)
		}
	})
}
