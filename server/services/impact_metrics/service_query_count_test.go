package impact_metrics

import (
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestGetCommunityImpactMetrics_QueryBudget locks in the per-request
// query count for GetCommunityImpactMetrics so we catch regressions of
// the #2055 fan-out. The original handler issued 6×N queries (where N
// is total communities on the platform) because computePercentiles
// called CalculateImpactSavings per community. The post-#2055 shape
// preloads each table once and partitions in memory.
//
// Budget: ≤ 25 queries total, regardless of community count. This
// covers:
//   - auth.RequireActiveCommunity membership check
//   - 8 queries for LoadCommunityDataset (one per source table + batches)
//   - 6 queries for LoadAllCommunitiesDatasets in computePercentiles
//     (one ListAll per source table + 3 GetByIDs batches)
//   - small slack for any new aggregation the handler picks up
//
// If this test fails because of a genuine new feature, raise the budget
// — but verify the increase is constant in community count, not N×k.
func TestGetCommunityImpactMetrics_QueryBudget(t *testing.T) {
	svc, db := setupTestService(t)

	// Seed enough communities that an N×6 regression would obviously
	// exceed the budget. With 10 communities, the old shape issued 60+
	// queries; the new shape issues a fixed ~14 regardless.
	const otherCommunities = 10
	for i := 0; i < otherCommunities; i++ {
		insertTestCommunity(t, db, "Other Community")
	}

	communityID := insertTestCommunity(t, db, "Target Community")
	ownerID := insertTestUser(t, db, "Alice")
	recipientID := insertTestUser(t, db, "Bob")
	insertTestMembership(t, db, communityID, ownerID)
	insertTestMembership(t, db, communityID, recipientID)

	gearID := insertGearWithValue(t, db, ownerID, "Ladder", 50)
	insertCommunityGearLink(t, db, communityID, gearID)
	insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID,
		testImpactEstimate(30, 500, 60))

	ctx := contextWithAuth(ownerID, "alice@example.com")
	ctx = storage.WithQueryStats(ctx)

	storage.AssertMaxQueries(t, ctx, 25, func() {
		req := connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
			CommunityId: communityID,
		})
		_, err := svc.GetCommunityImpactMetrics(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Re-run with a much larger community pool to confirm the budget
	// is scale-invariant in community count. Pre-#2055, this would be
	// 6×N additional queries; post-#2055 it stays constant.
	for i := 0; i < 50; i++ {
		insertTestCommunity(t, db, "Extra Community")
	}
	t.Run("scale-invariant with 60+ communities", func(t *testing.T) {
		ctx := contextWithAuth(ownerID, "alice@example.com")
		ctx = storage.WithQueryStats(ctx)
		storage.AssertMaxQueries(t, ctx, 25, func() {
			req := connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
				CommunityId: communityID,
			})
			_, err := svc.GetCommunityImpactMetrics(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
}

// TestGetCommunityMetricDetail_QueryBudget locks in the per-request
// query count for GetCommunityMetricDetail after the #2055 Phase 1.5
// fix. Pre-fix this handler issued ~1081 queries on a populated
// fixture because loadFulfilledRequests / loadCompletedExperiences /
// transferImpact / computeMoneyDetail each issued one GetByID per row.
// Post-fix the path threads a preloaded CommunityDataset (+ a
// dataset-prepopulated EntityCache + a gear-map-in-context) so the
// helpers stop fanning out.
//
// Budget: ≤ 40 queries. Phase 1.6 folded the residual
// QueryByField(CommunityGear/CommunityUser) calls in
// computeFactors / computeLibraryItems / computeLibraryValueTrend /
// computeComparison / computeMoneyDetail / computeTimeDetail /
// computeQualityTimeDetail into the dataset preload via context.
// The remaining ~30+ queries are the dataset preload itself (8) plus
// auth + per-user lookups (computeQTMembers's GetByIDs) and a couple
// of cross-community comparison queries that fall outside this phase.
// At ~1ms per query on local PG that's ~40ms; at Cloud SQL ~40ms/q
// = ~1.5s — well under the original ~40s projection.
func TestGetCommunityMetricDetail_QueryBudget(t *testing.T) {
	svc, db := setupTestService(t)

	// Seed several other communities + a target community with enough
	// rows in each junction table that an N-per-row regression would
	// blow the budget by hundreds.
	for i := 0; i < 10; i++ {
		insertTestCommunity(t, db, "Other Community")
	}
	communityID := insertTestCommunity(t, db, "Target Community")
	ownerID := insertTestUser(t, db, "Alice")
	recipientID := insertTestUser(t, db, "Bob")
	insertTestMembership(t, db, communityID, ownerID)
	insertTestMembership(t, db, communityID, recipientID)

	// 20 gear / 20 completed transfers / 10 fulfilled requests / 10
	// completed experiences. Pre-fix this would be ~80+ queries from
	// the per-row GetByIDs alone.
	for i := 0; i < 20; i++ {
		gearID := insertGearWithValue(t, db, ownerID, "Item", 50)
		insertCommunityGearLink(t, db, communityID, gearID)
		insertTestCompletedTransfer(t, db, communityID, ownerID, recipientID, gearID,
			testImpactEstimate(30, 500, 60))
	}

	ctx := contextWithAuth(ownerID, "alice@example.com")
	ctx = storage.WithQueryStats(ctx)

	storage.AssertMaxQueries(t, ctx, 40, func() {
		req := connect.NewRequest(&api.GetCommunityMetricDetailRequest{
			CommunityId: communityID,
			Dimension:   api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			Period:      api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
		})
		_, err := svc.GetCommunityMetricDetail(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
