package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TestGetCommunityUtilization covers auth enforcement, validation, and utilization computation.
func TestGetCommunityUtilization(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Util Auth Community")
		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetCommunityUtilization(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{})
		_, err := svc.GetCommunityUtilization(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("community with no gear returns empty utilization", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Empty Util Community")
		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityUtilization(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.ItemCount != 0 {
			t.Errorf("expected 0 items, got %d", resp.Msg.ItemCount)
		}
	})

	t.Run("community with gear and completed loan has positive utilization", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Util Active Community")
		ownerID := insertTestUser(t, db, "Util Owner")
		borrowerID := insertTestUser(t, db, "Util Borrower")
		gearID := insertGearWithValue(t, db, ownerID, "Power Drill", 150)
		insertCommunityGearLink(t, db, communityID, gearID)
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID,
			testImpactEstimate(20, 400, 45))

		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityUtilization(contextWithAuth(ownerID, "owner@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.ItemCount != 1 {
			t.Errorf("expected 1 item, got %d", resp.Msg.ItemCount)
		}
		if len(resp.Msg.MostUtilized) == 0 {
			t.Error("expected at least one entry in most_utilized")
		}
		if resp.Msg.MostUtilized[0].Name != "Power Drill" {
			t.Errorf("expected most_utilized item name=Power Drill, got %q", resp.Msg.MostUtilized[0].Name)
		}
	})

	t.Run("utilization trend always returns 6 monthly points", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Util Trend Community")
		ownerID := insertTestUser(t, db, "Trend Owner")
		borrowerID := insertTestUser(t, db, "Trend Borrower")
		gearID := insertGearWithValue(t, db, ownerID, "Kayak", 600)
		insertCommunityGearLink(t, db, communityID, gearID)
		insertTestCompletedTransfer(t, db, communityID, ownerID, borrowerID, gearID,
			testImpactEstimate(60, 1000, 120))

		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityUtilization(contextWithAuth(ownerID, "trend@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.UtilizationTrend) != 6 {
			t.Errorf("expected 6 trend points, got %d", len(resp.Msg.UtilizationTrend))
		}
	})

	t.Run("redundancy group created for same-category gear", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Util Redundancy Community")
		ownerID := insertTestUser(t, db, "Redundancy Owner")
		gearID1 := insertGearWithCategory(t, db, ownerID, "Drill A", "power tools", 120)
		gearID2 := insertGearWithCategory(t, db, ownerID, "Drill B", "power tools", 100)
		insertCommunityGearLink(t, db, communityID, gearID1)
		insertCommunityGearLink(t, db, communityID, gearID2)

		req := connect.NewRequest(&api.GetCommunityUtilizationRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityUtilization(contextWithAuth(ownerID, "redundancy@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, rg := range resp.Msg.RedundancyGroups {
			if rg.Label == "power tools" && rg.Count >= 2 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected a redundancy group for 'power tools' category with count >= 2")
		}
	})
}
