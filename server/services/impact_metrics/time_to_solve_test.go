package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestGetTimeToSolveDetail covers auth enforcement, validation, and median computation.
func TestGetTimeToSolveDetail(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "TTS Community")
		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetTimeToSolveDetail(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{})
		_, err := svc.GetTimeToSolveDetail(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("community with no requests returns empty response", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "No Requests Community")
		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetTimeToSolveDetail(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.RecentRequests) != 0 {
			t.Errorf("expected no recent requests, got %d", len(resp.Msg.RecentRequests))
		}
	})

	t.Run("single fulfilled request produces correct median", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "TTS Single Community")
		requesterID := insertTestUser(t, db, "TTS Requester")
		helperID := insertTestUser(t, db, "TTS Helper")

		// Solve time = 120 minutes exactly.
		insertFulfilledRequestWithTimes(t, db, communityID, requesterID, helperID, 120)

		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetTimeToSolveDetail(contextWithAuth(requesterID, "req@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if math.Abs(float64(resp.Msg.MedianMinutes)-120) > 1 {
			t.Errorf("expected median ~120 min, got %v", resp.Msg.MedianMinutes)
		}
		if len(resp.Msg.RecentRequests) != 1 {
			t.Errorf("expected 1 recent request, got %d", len(resp.Msg.RecentRequests))
		}
	})

	t.Run("multiple fulfilled requests: median is computed correctly", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "TTS Multi Community")
		requesterID := insertTestUser(t, db, "TTS Req2")
		helperID := insertTestUser(t, db, "TTS Hlp2")

		// Solve times: 60, 120, 180 min → sorted → median = 120.
		insertFulfilledRequestWithTimes(t, db, communityID, requesterID, helperID, 60)
		insertFulfilledRequestWithTimes(t, db, communityID, requesterID, helperID, 120)
		insertFulfilledRequestWithTimes(t, db, communityID, requesterID, helperID, 180)

		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetTimeToSolveDetail(contextWithAuth(requesterID, "req2@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if math.Abs(float64(resp.Msg.MedianMinutes)-120) > 1 {
			t.Errorf("expected median ~120 min, got %v", resp.Msg.MedianMinutes)
		}
	})

	t.Run("formatted_median is human-readable", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "TTS Formatted Community")
		requesterID := insertTestUser(t, db, "TTS FmtReq")
		helperID := insertTestUser(t, db, "TTS FmtHlp")

		// 30 minutes → should format as "30 min".
		insertFulfilledRequestWithTimes(t, db, communityID, requesterID, helperID, 30)

		req := connect.NewRequest(&api.GetTimeToSolveDetailRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetTimeToSolveDetail(contextWithAuth(requesterID, "fmtreq@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.FormattedMedian == "" {
			t.Error("expected non-empty formatted_median")
		}
	})
}

// insertFulfilledRequestWithTimes inserts a fulfilled request whose solve time is solveMinutes.
func insertFulfilledRequestWithTimes(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, requesterID, helperID string,
	solveMinutes int64,
) {
	t.Helper()
	ctx := context.Background()
	sharedAt := time.Now().Unix() - solveMinutes*60 - 3600
	fulfilledAt := sharedAt + solveMinutes*60
	request := &models.Request{
		Id:                 uuid.New().String(),
		Title:              "TTS Request",
		RequesterId:        requesterID,
		State:              models.RequestState_REQUEST_STATE_FULFILLED,
		FulfilledAtUnixSec: &fulfilledAt,
		ConfirmedHelperIds: []string{helperID},
	}
	requestID, err := db.Insert(ctx, request)
	if err != nil {
		t.Fatalf("insertFulfilledRequestWithTimes (request): %v", err)
	}
	cr := &models.CommunityRequest{
		Id:              uuid.New().String(),
		CommunityId:     communityID,
		RequestId:       requestID,
		SharedAtUnixSec: sharedAt,
	}
	if _, err := db.Insert(ctx, cr); err != nil {
		t.Fatalf("insertFulfilledRequestWithTimes (community_request): %v", err)
	}
}
