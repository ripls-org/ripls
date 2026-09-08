package impact_metrics

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestGetCommunityActions covers auth enforcement, validation, pagination, and action types.
func TestGetCommunityActions(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Actions Community")
		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetCommunityActions(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityActionsRequest{})
		_, err := svc.GetCommunityActions(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("non-member is denied", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Member-Gated Community")
		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetCommunityActions(contextWithAuth("outsider", "outsider@example.com"), req)
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("empty community returns empty items without error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Empty Actions Community")
		insertTestMembership(t, db, communityID, "u")
		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityActions(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.Msg.Items))
		}
		if resp.Msg.NextPageToken != nil {
			t.Error("expected nil next_page_token for empty result")
		}
	})

	t.Run("completed loan appears as loan action", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Loan Actions Community")
		ownerID := insertTestUser(t, db, "Lender")
		borrowerID := insertTestUser(t, db, "Borrower")
		insertTestMembership(t, db, communityID, ownerID)
		gearID := insertGearWithValue(t, db, ownerID, "Tent", 200)
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID,
			testImpactEstimate(40, 800, 120))

		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityActions(contextWithAuth(ownerID, "lender@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Items) != 1 {
			t.Fatalf("expected 1 action, got %d", len(resp.Msg.Items))
		}
		if resp.Msg.Items[0].TransactionType != "loan" {
			t.Errorf("expected transaction_type=loan, got %q", resp.Msg.Items[0].TransactionType)
		}
		if resp.Msg.Items[0].ItemName != "Tent" {
			t.Errorf("expected item_name=Tent, got %q", resp.Msg.Items[0].ItemName)
		}
	})

	t.Run("fulfilled request appears as request action", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Request Actions Community")
		requesterID := insertTestUser(t, db, "Requester")
		helperID := insertTestUser(t, db, "Helper")
		insertTestMembership(t, db, communityID, requesterID)
		insertTestFulfilledRequest(t, db, communityID, requesterID, helperID,
			testImpactEstimate(0, 200, 30))

		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityActions(contextWithAuth(requesterID, "req@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Items) != 1 {
			t.Fatalf("expected 1 action, got %d", len(resp.Msg.Items))
		}
		if resp.Msg.Items[0].TransactionType != "request" {
			t.Errorf("expected transaction_type=request, got %q", resp.Msg.Items[0].TransactionType)
		}
	})

	t.Run("completed experience appears as event action", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Experience Actions Community")
		hostID := insertTestUser(t, db, "Host")
		insertTestMembership(t, db, communityID, hostID)
		insertTestCompletedExperience(t, db, communityID, hostID, testImpactEstimate(0, 0, 90))

		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityActions(contextWithAuth(hostID, "host@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Items) != 1 {
			t.Fatalf("expected 1 action, got %d", len(resp.Msg.Items))
		}
		if resp.Msg.Items[0].TransactionType != "event" {
			t.Errorf("expected transaction_type=event, got %q", resp.Msg.Items[0].TransactionType)
		}
	})

	t.Run("pagination: next_page_token present when items exceed page_size", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pagination Community")
		ownerID := insertTestUser(t, db, "PagOwner")
		recipientID := insertTestUser(t, db, "PagRecipient")
		insertTestMembership(t, db, communityID, ownerID)

		// Insert 3 completed transfers.
		for i := range 3 {
			gearID := insertGearWithValue(t, db, ownerID, fmt.Sprintf("Gear%d", i), 50)
			insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID, nil)
		}

		req := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
			PageSize:    2,
		})
		resp, err := svc.GetCommunityActions(contextWithAuth(ownerID, "pag@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Items) != 2 {
			t.Errorf("expected 2 items on first page, got %d", len(resp.Msg.Items))
		}
		if resp.Msg.NextPageToken == nil {
			t.Error("expected next_page_token for additional items")
		}
	})

	t.Run("pagination: second page uses next_page_token", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pagination2 Community")
		ownerID := insertTestUser(t, db, "Pag2Owner")
		recipientID := insertTestUser(t, db, "Pag2Recipient")
		insertTestMembership(t, db, communityID, ownerID)

		for i := range 3 {
			gearID := insertGearWithValue(t, db, ownerID, fmt.Sprintf("PGear%d", i), 50)
			insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID, nil)
		}

		firstReq := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
			PageSize:    2,
		})
		firstResp, err := svc.GetCommunityActions(contextWithAuth(ownerID, "pag2@example.com"), firstReq)
		if err != nil {
			t.Fatalf("first page error: %v", err)
		}

		secondReq := connect.NewRequest(&api.GetCommunityActionsRequest{
			CommunityId: communityID,
			PageSize:    2,
			PageToken:   firstResp.Msg.NextPageToken,
		})
		secondResp, err := svc.GetCommunityActions(contextWithAuth(ownerID, "pag2@example.com"), secondReq)
		if err != nil {
			t.Fatalf("second page error: %v", err)
		}
		if len(secondResp.Msg.Items) != 1 {
			t.Errorf("expected 1 item on second page, got %d", len(secondResp.Msg.Items))
		}
		if secondResp.Msg.NextPageToken != nil {
			t.Error("expected nil next_page_token after last page")
		}
	})
}

// insertTestCompletedExperience inserts a completed experience linked to a community.
func insertTestCompletedExperience(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID string,
	ie *models.ImpactEstimate,
) string {
	t.Helper()
	ctx := context.Background()
	now := int64(1_700_000_000)
	experience := &models.Experience{
		Id:                 uuid.New().String(),
		Name:               "Test Workshop",
		OwnerId:            ownerID,
		State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		CompletedAtUnixSec: &now,
		ImpactEstimate:     ie,
	}
	experienceID, err := db.Insert(ctx, experience)
	if err != nil {
		t.Fatalf("insertTestCompletedExperience (experience): %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: experienceID,
	}
	if _, err := db.Insert(ctx, ce); err != nil {
		t.Fatalf("insertTestCompletedExperience (community_experience): %v", err)
	}
	return experienceID
}
