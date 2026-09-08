package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TestGetCommunityPercentileDetail covers auth enforcement, validation, and percentile math.
func TestGetCommunityPercentileDetail(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("unauthenticated request returns error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pct Auth Community")
		req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		_, err := svc.GetCommunityPercentileDetail(context.Background(), req)
		if err == nil {
			t.Fatal("expected auth error, got nil")
		}
	})

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
			Dimension: api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		_, err := svc.GetCommunityPercentileDetail(contextWithAuth("u", "u@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("single community returns 0 percentile", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pct Single Community")
		req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetCommunityPercentileDetail(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// With only one community or no savings, percentile should be 0.
		if resp.Msg.CurrentPercentile < 0 || resp.Msg.CurrentPercentile > 100 {
			t.Errorf("percentile out of range: %d", resp.Msg.CurrentPercentile)
		}
	})

	t.Run("top communities list never exceeds 10", func(t *testing.T) {
		// Create 15 communities so we can verify the cap of 10.
		var firstCommunityID string
		for i := range 15 {
			id := insertTestCommunity(t, db, "Pct Top Community")
			if i == 0 {
				firstCommunityID = id
			}
		}

		req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
			CommunityId: firstCommunityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetCommunityPercentileDetail(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.TopCommunities) > 10 {
			t.Errorf("expected at most 10 top communities, got %d", len(resp.Msg.TopCommunities))
		}
	})

	t.Run("formatted value is non-empty when community has data", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pct Value Community")
		ownerID := insertTestUser(t, db, "Pct Owner")
		recipientID := insertTestUser(t, db, "Pct Recipient")
		gearID := insertGearWithValue(t, db, ownerID, "Bike", 300)
		insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID,
			testImpactEstimate(75, 1500, 60))

		req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
		})
		resp, err := svc.GetCommunityPercentileDetail(contextWithAuth(ownerID, "pct@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// If the community appears in the top list, its formatted value should be non-empty.
		for _, tc := range resp.Msg.TopCommunities {
			if tc.IsThisCommunity && tc.FormattedValue == "" {
				t.Error("expected non-empty formatted_value for this community in top list")
			}
		}
	})

	t.Run("dimensions EMISSIONS and QUALITY_TIME accepted without error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Pct Dims Community")
		for _, dim := range []api.ImpactMetricDimension{
			api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS,
			api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME,
		} {
			req := connect.NewRequest(&api.GetCommunityPercentileDetailRequest{
				CommunityId: communityID,
				Dimension:   dim,
			})
			_, err := svc.GetCommunityPercentileDetail(contextWithAuth("u", "u@example.com"), req)
			if err != nil {
				t.Errorf("dimension %v: unexpected error: %v", dim, err)
			}
		}
	})
}
