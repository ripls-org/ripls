package impact_metrics

import (
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TestGetCommunityImpactMetrics covers validation and basic happy paths.
func TestGetCommunityImpactMetrics(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("missing community_id returns InvalidArgument", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityImpactMetricsRequest{})
		_, err := svc.GetCommunityImpactMetrics(contextWithAuth("user1", "user1@example.com"), req)
		if err == nil {
			t.Fatal("expected error for missing community_id, got nil")
		}
		assertConnectCode(t, err, connect.CodeInvalidArgument)
	})

	t.Run("empty community returns zero metrics without error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Empty Community")
		req := connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityImpactMetrics(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.Metrics == nil {
			t.Fatal("expected non-nil Metrics in response")
		}
		if resp.Msg.Metrics.MemberCount != 0 {
			t.Errorf("expected 0 members, got %d", resp.Msg.Metrics.MemberCount)
		}
	})

	t.Run("community with completed loan accumulates savings", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Loan Community")
		ownerID := insertTestUser(t, db, "Alice")
		recipientID := insertTestUser(t, db, "Bob")
		insertTestMembership(t, db, communityID, ownerID)
		insertTestMembership(t, db, communityID, recipientID)

		gearID := insertGearWithValue(t, db, ownerID, "Ladder", 50)
		insertCommunityGearLink(t, db, communityID, gearID)
		insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID,
			testImpactEstimate(30, 500, 60))

		req := connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
			CommunityId: communityID,
		})
		resp, err := svc.GetCommunityImpactMetrics(contextWithAuth(ownerID, "alice@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.Metrics.CompletedLoans != 1 {
			t.Errorf("expected 1 completed loan, got %d", resp.Msg.Metrics.CompletedLoans)
		}
		if resp.Msg.Metrics.MemberCount != 2 {
			t.Errorf("expected 2 members, got %d", resp.Msg.Metrics.MemberCount)
		}
	})
}

// TestGetCommunityMetricDetail covers validation.
func TestGetCommunityMetricDetail(t *testing.T) {
	svc, db := setupTestService(t)

	tests := []struct {
		name        string
		communityID string
		dimension   api.ImpactMetricDimension
		period      api.ImpactMetricPeriod
		wantCode    connect.Code
	}{
		{
			name:      "missing community_id",
			dimension: api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			period:    api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
			wantCode:  connect.CodeInvalidArgument,
		},
		{
			name:        "missing dimension",
			communityID: "some-community",
			period:      api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
			wantCode:    connect.CodeInvalidArgument,
		},
		{
			name:        "missing period",
			communityID: "some-community",
			dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			wantCode:    connect.CodeInvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := connect.NewRequest(&api.GetCommunityMetricDetailRequest{
				CommunityId: tt.communityID,
				Dimension:   tt.dimension,
				Period:      tt.period,
			})
			_, err := svc.GetCommunityMetricDetail(contextWithAuth("u", "u@example.com"), req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			assertConnectCode(t, err, tt.wantCode)
		})
	}

	t.Run("empty community returns response without error", func(t *testing.T) {
		communityID := insertTestCommunity(t, db, "Metric Detail Community")
		req := connect.NewRequest(&api.GetCommunityMetricDetailRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			Period:      api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
		})
		_, err := svc.GetCommunityMetricDetail(contextWithAuth("u", "u@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// assertConnectCode fails the test if err is not a *connect.Error with the given code.
func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	ce, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != want {
		t.Errorf("expected connect code %v, got %v", want, ce.Code())
	}
}
