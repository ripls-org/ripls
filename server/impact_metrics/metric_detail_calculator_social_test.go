package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestComputeMetricDetail_SocialDimension tests the social dimension end-to-end via ComputeMetricDetail.
func TestComputeMetricDetail_SocialDimension(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community.
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Social Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// Create 3 community members for per-capita calculation.
	userIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		user := &models.User{
			Id:   uuid.New().String(),
			Name: "User " + uuid.New().String()[:8],
		}
		if _, err := db.Insert(ctx, user); err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}
		userIDs[i] = user.Id

		cu := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      user.Id,
		}
		if _, err := db.Insert(ctx, cu); err != nil {
			t.Fatalf("Failed to insert community user: %v", err)
		}
	}

	now := time.Now()

	// Insert transfers with social footprint data.
	// Transfer 1: 20 SF, 15 belonging min, 2.0 trust credits.
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testQualityTimeImpactEstimate(20.0, 15.0, 2.0), now.Add(-7*24*time.Hour).Unix())

	// Transfer 2: 30 SF, 25 belonging min, 1.0 trust credits.
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testQualityTimeImpactEstimate(30.0, 25.0, 1.0), now.Add(-14*24*time.Hour).Unix())

	// Transfer 3 (giveaway): 10 SF, 8 belonging min, 0.5 trust credits.
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		testQualityTimeImpactEstimate(10.0, 8.0, 0.5), now.Add(-21*24*time.Hour).Unix())

	result, err := calc.ComputeMetricDetail(ctx, communityID,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail(SOCIAL) error = %v", err)
	}

	// Total SF: 20 + 30 + 10 = 60.
	wantTotal := float32(60.0)
	if math.Abs(float64(result.TotalValue.Mean-wantTotal)) > 0.01 {
		t.Errorf("TotalValue.Mean = %v, want %v", result.TotalValue.Mean, wantTotal)
	}

	t.Run("QualityTimeDetail populated", func(t *testing.T) {
		sd := result.QualityTimeDetail
		if sd == nil {
			t.Fatal("QualityTimeDetail is nil")
		}

		// Weekly average: 60 SF / 52 weeks ≈ 1.15.
		if sd.WeeklyAverageMinutes == nil || sd.WeeklyAverageMinutes.Mean <= 0 {
			t.Error("WeeklyAverageMinutes should be positive")
		}

		if sd.SufficiencyTarget != 360 {
			t.Errorf("SufficiencyTarget = %d, want 360", sd.SufficiencyTarget)
		}

		if sd.Equivalence == "" {
			t.Error("Equivalence should not be empty")
		}
	})

	t.Run("PerCapitaSF", func(t *testing.T) {
		sd := result.QualityTimeDetail
		if sd == nil {
			t.Fatal("QualityTimeDetail is nil")
		}
		// 60 SF / 3 members = 20.
		wantPerCapita := 20.0
		if math.Abs(sd.PerCapitaMinutes-wantPerCapita) > 0.01 {
			t.Errorf("PerCapitaMinutes = %v, want %v", sd.PerCapitaMinutes, wantPerCapita)
		}
	})

	t.Run("MonthlyBars", func(t *testing.T) {
		sd := result.QualityTimeDetail
		if sd == nil {
			t.Fatal("QualityTimeDetail is nil")
		}
		// Monthly bars should have 6 entries (last 6 months).
		if len(sd.MonthlyBars) != 6 {
			t.Errorf("MonthlyBars length = %d, want 6", len(sd.MonthlyBars))
		}
	})

	t.Run("Categories", func(t *testing.T) {
		sd := result.QualityTimeDetail
		if sd == nil {
			t.Fatal("QualityTimeDetail is nil")
		}
		if len(sd.Categories) == 0 {
			t.Error("Categories should not be empty")
		}

		// Verify percentages sum to ~100%.
		totalPct := 0.0
		for _, cat := range sd.Categories {
			totalPct += cat.Percentage
		}
		if totalPct < 99 || totalPct > 101 {
			t.Errorf("Category percentages sum = %v, want ~100", totalPct)
		}
	})

	t.Run("SubsidiaryMetrics", func(t *testing.T) {
		sd := result.QualityTimeDetail
		if sd == nil {
			t.Fatal("QualityTimeDetail is nil")
		}
		if len(sd.SubsidiaryMetrics) != 3 {
			t.Fatalf("SubsidiaryMetrics length = %d, want 3", len(sd.SubsidiaryMetrics))
		}

		// Belonging Minutes.
		bm := sd.SubsidiaryMetrics[0]
		if bm.Key != "belonging_minutes" {
			t.Errorf("SubsidiaryMetrics[0].Key = %q, want \"belonging_minutes\"", bm.Key)
		}
		// Total belonging: 15 + 25 + 8 = 48 minutes = 0.8 hours.
		if bm.Value < 0.7 || bm.Value > 0.9 {
			t.Errorf("Belonging Minutes Value = %v, want ~0.8 hours", bm.Value)
		}
		if len(bm.Sparkline) != 6 {
			t.Errorf("Belonging Minutes sparkline length = %d, want 6", len(bm.Sparkline))
		}
		// Trust Credits.
		tc := sd.SubsidiaryMetrics[1]
		if tc.Key != "trust_credits" {
			t.Errorf("SubsidiaryMetrics[1].Key = %q, want \"trust_credits\"", tc.Key)
		}
		// Total trust: 2.0 + 1.0 + 0.5 = 3.5.
		if math.Abs(tc.Value-3.5) > 0.5 {
			t.Errorf("Trust Credits Value = %v, want ~3.5", tc.Value)
		}
		if len(tc.Sparkline) != 6 {
			t.Errorf("Trust Credits sparkline length = %d, want 6", len(tc.Sparkline))
		}

		// Network Diversity.
		nd := sd.SubsidiaryMetrics[2]
		if nd.Key != "network_diversity" {
			t.Errorf("SubsidiaryMetrics[2].Key = %q, want \"network_diversity\"", nd.Key)
		}
	})

	t.Run("SourceBreakdown", func(t *testing.T) {
		if len(result.SourceBreakdown) == 0 {
			t.Error("SourceBreakdown should not be empty")
		}
		// Should have Loans and Giveaways.
		loansBreakdown := findBreakdown(result.SourceBreakdown, api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS)
		if loansBreakdown == nil {
			t.Fatalf("Loans breakdown not found; got labels: %v", breakdownLabels(result.SourceBreakdown))
		}
		// Loans total: 20 + 30 = 50 SF.
		if math.Abs(loansBreakdown.Value-50.0) > 0.01 {
			t.Errorf("Loans Value = %v, want 50.0", loansBreakdown.Value)
		}
	})
}

// TestComputeMetricDetail_SocialEmptyCommunity tests that an empty community returns graceful zeros for social dimension.
func TestComputeMetricDetail_SocialEmptyCommunity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Empty Social Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	result, err := calc.ComputeMetricDetail(ctx, communityID,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail(SOCIAL) error = %v, want nil for empty community", err)
	}

	if result.TotalValue.Mean != 0 {
		t.Errorf("TotalValue.Mean = %v, want 0 for empty community", result.TotalValue.Mean)
	}

	sd := result.QualityTimeDetail
	if sd == nil {
		t.Fatal("QualityTimeDetail should not be nil even for empty community")
	}

	if sd.WeeklyAverageMinutes == nil || sd.WeeklyAverageMinutes.Mean != 0 {
		t.Errorf("WeeklyAverageMinutes.Mean = %v, want 0", sd.WeeklyAverageMinutes.GetMean())
	}
	if sd.PerCapitaMinutes != 0 {
		t.Errorf("PerCapitaMinutes = %v, want 0", sd.PerCapitaMinutes)
	}
	if sd.SufficiencyPct != 0 {
		t.Errorf("SufficiencyPct = %v, want 0", sd.SufficiencyPct)
	}

	// Subsidiary metrics should still be present (3 items) with zero values.
	if len(sd.SubsidiaryMetrics) != 3 {
		t.Errorf("SubsidiaryMetrics length = %d, want 3", len(sd.SubsidiaryMetrics))
	}

	// Recent activity should be empty.
	if len(sd.RecentActivity) != 0 {
		t.Errorf("RecentActivity length = %d, want 0", len(sd.RecentActivity))
	}
}

// TestComputeMetricDetail_SocialRecentActivity tests that recent activity returns top 4 most recent items.
func TestComputeMetricDetail_SocialRecentActivity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Activity Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now()

	// Insert 6 transfers at different times. Only top 4 most recent should appear.
	for i := 0; i < 6; i++ {
		completedAt := now.Add(-time.Duration(i) * 24 * time.Hour).Unix()
		insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
			testQualityTimeImpactEstimate(float32(10+i), 5.0, 1.0), completedAt)
	}

	result, err := calc.ComputeMetricDetail(ctx, communityID,
		api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail(SOCIAL) error = %v", err)
	}

	sd := result.QualityTimeDetail
	if sd == nil {
		t.Fatal("QualityTimeDetail is nil")
	}

	// Should cap at 4 recent items.
	if len(sd.RecentActivity) != 4 {
		t.Errorf("RecentActivity length = %d, want 4 (capped)", len(sd.RecentActivity))
	}

	// Each should carry a QT value the client can format.
	for i, ra := range sd.RecentActivity {
		if ra.RawValue == nil {
			t.Errorf("RecentActivity[%d].RawValue is absent", i)
		}
	}
}

// testQualityTimeImpactEstimate creates an ImpactEstimate with quality time data.
func testQualityTimeImpactEstimate(qtMinutes, belongingMinutes, trustCredits float32) *models.ImpactEstimate {
	ie := &models.ImpactEstimate{}
	ie.QualityTime = &models.QualityTimeEstimate{
		QualityTimeMinutes: &models.Estimate{Mean: qtMinutes, Stddev: qtMinutes * 0.2},
		BelongingMinutes:   &models.Estimate{Mean: belongingMinutes, Stddev: belongingMinutes * 0.2},
		TrustCredits:       &models.Estimate{Mean: trustCredits, Stddev: trustCredits * 0.2},
	}
	return ie
}

// Helper functions.
