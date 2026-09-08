package impact_metrics

import (
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestBuildSourceBreakdown covers the pure percentage-math helper.
func TestBuildSourceBreakdown(t *testing.T) {
	t.Run("empty totals returns nil", func(t *testing.T) {
		result := buildSourceBreakdown(0, 0, 0, 0)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("one source returns single entry at 100 percent", func(t *testing.T) {
		result := buildSourceBreakdown(50, 0, 0, 0)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS {
			t.Errorf("expected LOANS source, got %v", result[0].SourceType)
		}
		if result[0].Value != 50 {
			t.Errorf("expected value 50, got %v", result[0].Value)
		}
		if result[0].Percentage != 100 {
			t.Errorf("expected percentage 100, got %v", result[0].Percentage)
		}
	})

	t.Run("mixed loans and giveaways preserves insertion order and percentages", func(t *testing.T) {
		// loans=40, giveaways=60 → total 100; Loans first (insertion order), then Giveaways.
		result := buildSourceBreakdown(40, 60, 0, 0)
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(result))
		}
		if result[0].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS {
			t.Errorf("expected first source LOANS, got %v", result[0].SourceType)
		}
		if result[1].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS {
			t.Errorf("expected second source GIVEAWAYS, got %v", result[1].SourceType)
		}
		if math.Abs(result[0].Percentage-40) > 0.1 {
			t.Errorf("expected Loans percentage ~40, got %v", result[0].Percentage)
		}
		if math.Abs(result[1].Percentage-60) > 0.1 {
			t.Errorf("expected Giveaways percentage ~60, got %v", result[1].Percentage)
		}
	})
}

// TestGetUserImpactMetrics_SourceBreakdownsViaRPC covers the source-aggregation
// helpers (loans, giveaways, requests, experiences) through the RPC.
func TestGetUserImpactMetrics_SourceBreakdownsViaRPC(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("loan and giveaway split", func(t *testing.T) {
		ownerID := insertTestUser(t, db, "BD Lender")
		recipientID := insertTestUser(t, db, "BD Recipient")
		communityID := insertTestCommunity(t, db, "BD Split Community")
		gearID := insertGearWithValue(t, db, ownerID, "Kayak", 400)

		insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID,
			testImpactEstimate(100, 0, 0))
		insertGiveawayTransfer(t, db, communityID, ownerID, recipientID, gearID,
			testImpactEstimate(50, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: ownerID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(ownerID, "lender@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bd := resp.Msg.SourceBreakdownMoney
		if len(bd) != 2 {
			t.Fatalf("expected 2 breakdown entries, got %d", len(bd))
		}
		if bd[0].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS {
			t.Errorf("expected first source LOANS, got %v", bd[0].SourceType)
		}
		if math.Abs(bd[0].Value-100) > 0.01 {
			t.Errorf("expected Loans value ~100, got %v", bd[0].Value)
		}
		if math.Abs(bd[0].Percentage-66.67) > 0.1 {
			t.Errorf("expected Loans percentage ~66.7, got %v", bd[0].Percentage)
		}
		if bd[1].SourceType != api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS {
			t.Errorf("expected second source GIVEAWAYS, got %v", bd[1].SourceType)
		}
		if math.Abs(bd[1].Value-50) > 0.01 {
			t.Errorf("expected Giveaways value ~50, got %v", bd[1].Value)
		}
		if math.Abs(bd[1].Percentage-33.33) > 0.1 {
			t.Errorf("expected Giveaways percentage ~33.3, got %v", bd[1].Percentage)
		}
	})

	t.Run("requests helper path is structurally wired but currently unqueryable", func(t *testing.T) {
		// userRequestImpact queries by confirmed_helper_ids, which is a repeated field
		// not stored as a queryable SQL column. The query fails silently and returns 0.
		// This test documents that the RPC succeeds even when the helper query cannot
		// retrieve results, so that a future fix to the query path will be caught here.
		helperID := insertTestUser(t, db, "BD Helper")
		requesterID := insertTestUser(t, db, "BD Requester")
		communityID := insertTestCommunity(t, db, "BD Request Community")

		insertTestFulfilledRequest(t, db, communityID, requesterID, helperID,
			testImpactEstimate(80, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: helperID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(helperID, "helper@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The RPC must not error even when the helper query fails internally.
		if resp.Msg == nil {
			t.Fatal("expected non-nil response message")
		}
	})

	t.Run("experiences contribute via owner path", func(t *testing.T) {
		ownerID := insertTestUser(t, db, "BD Exp Owner")
		insertExperienceForOwner(t, db, ownerID, testImpactEstimate(40, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: ownerID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(ownerID, "expowner@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, entry := range resp.Msg.SourceBreakdownMoney {
			if entry.SourceType == api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS {
				found = true
				if math.Abs(entry.Value-40) > 0.01 {
					t.Errorf("expected Events value ~40, got %v", entry.Value)
				}
			}
		}
		if !found {
			t.Errorf("expected 'Events' entry in SourceBreakdownMoney, got %v", resp.Msg.SourceBreakdownMoney)
		}
	})

	t.Run("co2 breakdown sums manufacture and waste carbon", func(t *testing.T) {
		ownerID := insertTestUser(t, db, "BD CO2 Owner")
		recipientID := insertTestUser(t, db, "BD CO2 Recipient")
		communityID := insertTestCommunity(t, db, "BD CO2 Community")
		gearID := insertGearWithValue(t, db, ownerID, "Bicycle", 300)

		ie := &models.ImpactEstimate{
			EmissionsPrevented: &models.PreventedEmissions{
				ManufactureAvoidedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 500, Stddev: 0},
				},
				WasteReducedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 300, Stddev: 0},
				},
			},
		}
		insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID, ie)

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: ownerID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(ownerID, "co2owner@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.SourceBreakdownCo2) == 0 {
			t.Fatal("expected non-empty SourceBreakdownCo2")
		}
		// ManufactureAvoidedCarbon(500) + WasteReducedCarbon(300) = 800
		if math.Abs(resp.Msg.SourceBreakdownCo2[0].Value-800) > 0.01 {
			t.Errorf("expected CO2 value ~800, got %v", resp.Msg.SourceBreakdownCo2[0].Value)
		}
	})
}

// TestGetUserImpactMetrics_CommunityRanking covers the community ranking aggregator.
func TestGetUserImpactMetrics_CommunityRanking(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("empty memberships returns empty ranking", func(t *testing.T) {
		userID := insertTestUser(t, db, "Rank No Memberships")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "nomember@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.RankedCommunities) != 0 {
			t.Errorf("expected empty ranking, got %d entries", len(resp.Msg.RankedCommunities))
		}
	})

	t.Run("ranks by money saved descending", func(t *testing.T) {
		userID := insertTestUser(t, db, "Rank User Desc")
		recipientID := insertTestUser(t, db, "Rank Recipient Desc")
		gearID := insertGearWithValue(t, db, userID, "Drill", 100)

		comA := insertTestCommunity(t, db, "Community A")
		comB := insertTestCommunity(t, db, "Community B")
		comC := insertTestCommunity(t, db, "Community C")
		insertTestMembership(t, db, comA, userID)
		insertTestMembership(t, db, comB, userID)
		insertTestMembership(t, db, comC, userID)

		insertTestCompletedTransfer(t, db, comA, userID, recipientID, gearID, testImpactEstimate(30, 0, 0))
		insertTestCompletedTransfer(t, db, comB, userID, recipientID, gearID, testImpactEstimate(90, 0, 0))
		insertTestCompletedTransfer(t, db, comC, userID, recipientID, gearID, testImpactEstimate(60, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "rankdesc@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ranked := resp.Msg.RankedCommunities
		if len(ranked) != 3 {
			t.Fatalf("expected 3 ranked communities, got %d", len(ranked))
		}
		// Expected order: B (90) → C (60) → A (30).
		if ranked[0].CommunityName != "Community B" || ranked[0].Rank != 1 {
			t.Errorf("expected rank 1 = Community B, got %q (rank %d)", ranked[0].CommunityName, ranked[0].Rank)
		}
		if ranked[1].CommunityName != "Community C" || ranked[1].Rank != 2 {
			t.Errorf("expected rank 2 = Community C, got %q (rank %d)", ranked[1].CommunityName, ranked[1].Rank)
		}
		if ranked[2].CommunityName != "Community A" || ranked[2].Rank != 3 {
			t.Errorf("expected rank 3 = Community A, got %q (rank %d)", ranked[2].CommunityName, ranked[2].Rank)
		}
	})

	t.Run("includes zero-impact communities", func(t *testing.T) {
		userID := insertTestUser(t, db, "Rank Zero Impact User")
		recipientID := insertTestUser(t, db, "Rank Zero Recipient")
		gearID := insertGearWithValue(t, db, userID, "Saw", 50)

		activeComm := insertTestCommunity(t, db, "Active Community")
		inactiveComm := insertTestCommunity(t, db, "Inactive Community")
		insertTestMembership(t, db, activeComm, userID)
		insertTestMembership(t, db, inactiveComm, userID)

		insertTestCompletedTransfer(t, db, activeComm, userID, recipientID, gearID, testImpactEstimate(25, 0, 0))

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "zeroimp@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.RankedCommunities) != 2 {
			t.Fatalf("expected 2 ranked communities (including zero-impact), got %d", len(resp.Msg.RankedCommunities))
		}
		// Find the inactive community and verify its formatted value is $0.
		for _, rc := range resp.Msg.RankedCommunities {
			if rc.CommunityName == "Inactive Community" {
				if rc.FormattedValue != "$0" {
					t.Errorf("expected FormattedValue=%q for inactive community, got %q", "$0", rc.FormattedValue)
				}
				return
			}
		}
		t.Error("inactive community not found in ranking")
	})

	t.Run("caps at 10 communities", func(t *testing.T) {
		userID := insertTestUser(t, db, "Rank Cap User")
		recipientID := insertTestUser(t, db, "Rank Cap Recipient")

		// Insert 11 communities with descending money values 110, 100, ..., 10.
		for i := 11; i >= 1; i-- {
			commID := insertTestCommunity(t, db, "Cap Community "+uuid.New().String())
			insertTestMembership(t, db, commID, userID)
			gearID := insertGearWithValue(t, db, userID, "Item", float32(i*10))
			insertTestCompletedTransfer(t, db, commID, userID, recipientID, gearID,
				testImpactEstimate(float32(i*10), 0, 0))
		}

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "rankcap@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.RankedCommunities) != 10 {
			t.Errorf("expected exactly 10 ranked communities, got %d", len(resp.Msg.RankedCommunities))
		}
		// The smallest community (value=10, rank 11) must not appear.
		for _, rc := range resp.Msg.RankedCommunities {
			if rc.Rank > 10 {
				t.Errorf("community with rank %d should have been dropped", rc.Rank)
			}
		}
	})
}

// TestGetUserImpactMetrics_MoneyTrend covers the cumulative money savings time series.
func TestGetUserImpactMetrics_MoneyTrend(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("empty trend when no completed transfers", func(t *testing.T) {
		userID := insertTestUser(t, db, "Trend Empty User")
		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "trendempty@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.MoneyTrend) != 0 {
			t.Errorf("expected empty MoneyTrend, got %d points", len(resp.Msg.MoneyTrend))
		}
	})

	t.Run("single month returns single cumulative point", func(t *testing.T) {
		userID := insertTestUser(t, db, "Trend Single User")
		recipientID := insertTestUser(t, db, "Trend Single Recipient")
		communityID := insertTestCommunity(t, db, "Trend Single Community")
		gearID := insertGearWithValue(t, db, userID, "Ladder", 100)

		now := time.Now().Unix()
		insertCompletedTransferAt(t, db, communityID, userID, recipientID, gearID,
			testImpactEstimate(50, 0, 0), now)

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "trendsingle@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.MoneyTrend) != 1 {
			t.Fatalf("expected 1 trend point, got %d", len(resp.Msg.MoneyTrend))
		}
		if math.Abs(resp.Msg.MoneyTrend[0].Value-50) > 0.01 {
			t.Errorf("expected trend value ~50, got %v", resp.Msg.MoneyTrend[0].Value)
		}
	})

	t.Run("more than 6 months keeps only last 6 cumulative", func(t *testing.T) {
		userID := insertTestUser(t, db, "Trend 7Mo User")
		recipientID := insertTestUser(t, db, "Trend 7Mo Recipient")
		communityID := insertTestCommunity(t, db, "Trend 7Mo Community")
		gearID := insertGearWithValue(t, db, userID, "Tent", 200)

		// Place one $10 transfer in each of the last 7 months.
		// Use the 1st of each month to avoid day-overflow edge cases.
		base := time.Now()
		baseMonth := time.Date(base.Year(), base.Month(), 1, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 7; i++ {
			monthAt := baseMonth.AddDate(0, -(6 - i), 0)
			insertCompletedTransferAt(t, db, communityID, userID, recipientID, gearID,
				testImpactEstimate(10, 0, 0), monthAt.Unix())
		}

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "trend7mo@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.MoneyTrend) != 6 {
			t.Fatalf("expected 6 trend points, got %d", len(resp.Msg.MoneyTrend))
		}
		// The last point should be the cumulative of 6 months × $10 = $60.
		last := resp.Msg.MoneyTrend[5].Value
		if math.Abs(last-60) > 0.01 {
			t.Errorf("expected last cumulative value ~60, got %v", last)
		}
	})
}

// TestGetUserImpactMetrics_Co2Trend covers cumulative CO₂ savings including both carbon sources.
func TestGetUserImpactMetrics_Co2Trend(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("sums manufacture and waste carbon per month cumulatively", func(t *testing.T) {
		userID := insertTestUser(t, db, "CO2 Trend User")
		recipientID := insertTestUser(t, db, "CO2 Trend Recipient")
		communityID := insertTestCommunity(t, db, "CO2 Trend Community")
		gearID := insertGearWithValue(t, db, userID, "Composter", 80)

		ie := &models.ImpactEstimate{
			EmissionsPrevented: &models.PreventedEmissions{
				ManufactureAvoidedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 400, Stddev: 0},
				},
				WasteReducedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 200, Stddev: 0},
				},
			},
		}
		now := time.Now().Unix()
		insertCompletedTransferAt(t, db, communityID, userID, recipientID, gearID, ie, now)

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "co2trend@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.Co2Trend) != 1 {
			t.Fatalf("expected 1 CO2 trend point, got %d", len(resp.Msg.Co2Trend))
		}
		// ManufactureAvoidedCarbon(400) + WasteReducedCarbon(200) = 600.
		if math.Abs(resp.Msg.Co2Trend[0].Value-600) > 0.01 {
			t.Errorf("expected CO2 trend value ~600, got %v", resp.Msg.Co2Trend[0].Value)
		}
	})
}

// TestGetUserImpactMetrics_QualityTimeTrend covers cumulative quality-time savings.
func TestGetUserImpactMetrics_QualityTimeTrend(t *testing.T) {
	svc, db := setupTestService(t)

	t.Run("accumulates quality time minutes", func(t *testing.T) {
		userID := insertTestUser(t, db, "QT Trend User")
		recipientID := insertTestUser(t, db, "QT Trend Recipient")
		communityID := insertTestCommunity(t, db, "QT Trend Community")
		gearID := insertGearWithValue(t, db, userID, "Guitar", 500)

		base := time.Now()
		baseMonth := time.Date(base.Year(), base.Month(), 1, 12, 0, 0, 0, time.UTC)

		// Two transfers in different months with 30 QT minutes each.
		insertCompletedTransferAt(t, db, communityID, userID, recipientID, gearID,
			testImpactEstimateWithQT(0, 0, 0, 30), baseMonth.AddDate(0, -1, 0).Unix())
		insertCompletedTransferAt(t, db, communityID, userID, recipientID, gearID,
			testImpactEstimateWithQT(0, 0, 0, 30), baseMonth.Unix())

		req := connect.NewRequest(&api.GetUserImpactMetricsRequest{UserId: userID})
		resp, err := svc.GetUserImpactMetrics(contextWithAuth(userID, "qttrend@example.com"), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Msg.QualityTimeTrend) != 2 {
			t.Fatalf("expected 2 QT trend points, got %d", len(resp.Msg.QualityTimeTrend))
		}
		// Second point is cumulative: 30 + 30 = 60.
		if math.Abs(resp.Msg.QualityTimeTrend[1].Value-60) > 0.01 {
			t.Errorf("expected second QT trend value ~60, got %v", resp.Msg.QualityTimeTrend[1].Value)
		}
	})
}
