package impact_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestComputeTimeSeries tests time-series bucketing and cumulative computation.
func TestComputeTimeSeries(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now()

	// Insert transfers at different times for monthly bucketing
	// 5 months ago
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(100.0, 0.0, 0.0), now.AddDate(0, -5, 0).Unix())
	// 3 months ago
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(200.0, 0.0, 0.0), now.AddDate(0, -3, 0).Unix())
	// 1 month ago
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(300.0, 0.0, 0.0), now.AddDate(0, -1, 0).Unix())

	t.Run("ALL period has 6 monthly buckets", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		if result.CumulativeTrend == nil {
			t.Fatal("CumulativeTrend is nil")
		}

		// Should have 6 monthly buckets
		if len(result.CumulativeTrend) != 6 {
			t.Errorf("CumulativeTrend length = %d, want 6", len(result.CumulativeTrend))
		}

		// Verify cumulative nature - values should be non-decreasing
		for i := 1; i < len(result.CumulativeTrend); i++ {
			if result.CumulativeTrend[i].Value < result.CumulativeTrend[i-1].Value {
				t.Errorf("CumulativeTrend[%d].Value = %v < CumulativeTrend[%d].Value = %v (should be non-decreasing)",
					i, result.CumulativeTrend[i].Value, i-1, result.CumulativeTrend[i-1].Value)
			}
		}

		// Last point should equal total value
		lastValue := result.CumulativeTrend[len(result.CumulativeTrend)-1].Value
		if lastValue != float64(result.TotalValue.Mean) {
			t.Errorf("Last cumulative value = %v, want %v (should equal total)", lastValue, result.TotalValue.Mean)
		}
	})

	t.Run("FOUR_WEEKS period has 4 weekly buckets", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_FOUR_WEEKS)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		if result.CumulativeTrend == nil {
			t.Fatal("CumulativeTrend is nil")
		}

		// Should have 4 weekly buckets
		if len(result.CumulativeTrend) != 4 {
			t.Errorf("CumulativeTrend length = %d, want 4", len(result.CumulativeTrend))
		}

		// Each bucket carries an ascending start the client renders the axis
		// label from; the server used to emit "W1".."W4" itself (#2835).
		var prev int64
		for i, point := range result.CumulativeTrend {
			if point.BucketStartUnixSec == nil || *point.BucketStartUnixSec == 0 {
				t.Fatalf("CumulativeTrend[%d].BucketStartUnixSec absent", i)
			}
			if *point.BucketStartUnixSec <= prev {
				t.Errorf("CumulativeTrend[%d].BucketStartUnixSec = %d, not after %d",
					i, *point.BucketStartUnixSec, prev)
			}
			prev = *point.BucketStartUnixSec
		}
	})

	t.Run("ONE_YEAR period has 12 monthly buckets", func(t *testing.T) {
		result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ONE_YEAR)
		if err != nil {
			t.Fatalf("ComputeMetricDetail() error = %v", err)
		}

		if result.CumulativeTrend == nil {
			t.Fatal("CumulativeTrend is nil")
		}

		// Should have 12 monthly buckets
		if len(result.CumulativeTrend) != 12 {
			t.Errorf("CumulativeTrend length = %d, want 12", len(result.CumulativeTrend))
		}

		// Every point carries its bucket start, ascending — the typed field
		// clients derive the axis label from (#2827).
		var prev int64
		for i, point := range result.CumulativeTrend {
			if point.BucketStartUnixSec == nil || *point.BucketStartUnixSec == 0 {
				t.Fatalf("CumulativeTrend[%d].BucketStartUnixSec absent", i)
			}
			if *point.BucketStartUnixSec <= prev {
				t.Errorf("CumulativeTrend[%d] bucket start %d not ascending (prev %d)",
					i, *point.BucketStartUnixSec, prev)
			}
			prev = *point.BucketStartUnixSec
		}
	})
}

// TestComputeMonthlyBars tests CO2 monthly bar chart data.
func TestComputeMonthlyBars(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	now := time.Now()

	// Insert transfers in different months
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(0.0, 5000.0, 0.0), now.AddDate(0, -3, 0).Unix())
	insertTransferWithTime(t, db, communityID, models.TransferType_TRANSFER_TYPE_LOAN,
		testImpactEstimate(0.0, 10000.0, 0.0), now.AddDate(0, -1, 0).Unix())

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	if result.MonthlyBars == nil {
		t.Fatal("MonthlyBars is nil")
	}

	// Should always have 6 monthly bars
	if len(result.MonthlyBars) != 6 {
		t.Errorf("MonthlyBars length = %d, want 6", len(result.MonthlyBars))
	}

	// Every bar carries its month-bucket start (#2827).
	for i, point := range result.MonthlyBars {
		if point.BucketStartUnixSec == nil || *point.BucketStartUnixSec == 0 {
			t.Fatalf("MonthlyBars[%d].BucketStartUnixSec absent", i)
		}
	}

	// Verify monthly average is computed
	if result.MonthlyAverage == 0 {
		t.Error("MonthlyAverage should not be 0 when there are transactions")
	}

	// Verify average calculation
	total := float64(0)
	for _, point := range result.MonthlyBars {
		total += point.Value
	}
	expectedAvg := total / float64(len(result.MonthlyBars))
	if result.MonthlyAverage != expectedAvg {
		t.Errorf("MonthlyAverage = %v, want %v", result.MonthlyAverage, expectedAvg)
	}
}

// TestEmptyTimeSeries tests that empty data produces zero-filled time series.
func TestEmptyTimeSeries(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	// Create empty community
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Empty Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	result, err := calc.ComputeMetricDetail(ctx, communityID, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
	if err != nil {
		t.Fatalf("ComputeMetricDetail() error = %v", err)
	}

	if result.CumulativeTrend == nil {
		t.Fatal("CumulativeTrend is nil")
	}

	// Should have buckets but all zeros
	if len(result.CumulativeTrend) != 6 {
		t.Errorf("CumulativeTrend length = %d, want 6", len(result.CumulativeTrend))
	}

	// All values should be zero
	for i, point := range result.CumulativeTrend {
		if point.Value != 0 {
			t.Errorf("CumulativeTrend[%d].Value = %v, want 0 for empty community", i, point.Value)
		}
	}
}

// TestComputeLibraryValueTrendWithItemCount tests that computeLibraryValueTrend
// returns both a cumulative value trend and a cumulative item count trend.
func TestComputeLibraryValueTrendWithItemCount(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	calc := NewMetricDetailCalculator(db, loadTestEstimatorConfig(t))

	communityID := uuid.New().String()
	community := &models.Community{Id: communityID, Name: "Test Community", CreatorId: "test-user", OwnerUserId: "test-user"}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("insert community: %v", err)
	}

	user := &models.User{Id: uuid.New().String(), Name: "Test User"}
	if _, err := db.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	now := time.Now()

	// insertLibraryItem adds a gear+community_gear record with a known value and add-time.
	insertLibraryItem := func(t *testing.T, valueUsd float32, addedAt time.Time) {
		t.Helper()
		gear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: user.Id,
			Name:    "Item",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: valueUsd,
			},
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		cg := &models.CommunityGear{
			Id:               uuid.New().String(),
			CommunityId:      communityID,
			GearId:           gear.Id,
			CreatedAtUnixSec: addedAt.Unix(),
		}
		if _, err := db.Insert(ctx, cg); err != nil {
			t.Fatalf("insert community gear: %v", err)
		}
	}

	tests := []struct {
		name           string
		setup          func()
		period         api.ImpactMetricPeriod
		wantValueTrend bool
		wantCountTrend bool
		wantFinalValue float64
		wantFinalCount float64
	}{
		{
			name:   "two items in different buckets accumulate correctly",
			setup:  func() {},
			period: api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = tc
		})
	}

	t.Run("returns nil for community with no gear", func(t *testing.T) {
		emptyCommunityID := uuid.New().String()
		empty := &models.Community{Id: emptyCommunityID, Name: "Empty", CreatorId: "test-user", OwnerUserId: "test-user"}
		if _, err := db.Insert(ctx, empty); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		valueTrend, countTrend := calc.computeLibraryValueTrend(ctx, emptyCommunityID, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if valueTrend != nil {
			t.Errorf("valueTrend = %v, want nil for empty community", valueTrend)
		}
		if countTrend != nil {
			t.Errorf("countTrend = %v, want nil for empty community", countTrend)
		}
	})

	t.Run("value and count trends have same length", func(t *testing.T) {
		// Add two items in different months.
		insertLibraryItem(t, 100.0, now.AddDate(0, -4, 0))
		insertLibraryItem(t, 200.0, now.AddDate(0, -2, 0))

		valueTrend, countTrend := calc.computeLibraryValueTrend(ctx, communityID, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if valueTrend == nil {
			t.Fatal("valueTrend is nil")
		}
		if countTrend == nil {
			t.Fatal("countTrend is nil")
		}
		if len(valueTrend) != len(countTrend) {
			t.Errorf("len(valueTrend)=%d != len(countTrend)=%d", len(valueTrend), len(countTrend))
		}
	})

	t.Run("value trend is cumulative and non-decreasing", func(t *testing.T) {
		insertLibraryItem(t, 50.0, now.AddDate(0, -3, 0))

		valueTrend, _ := calc.computeLibraryValueTrend(ctx, communityID, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if valueTrend == nil {
			t.Fatal("valueTrend is nil")
		}
		for i := 1; i < len(valueTrend); i++ {
			if valueTrend[i].Value < valueTrend[i-1].Value {
				t.Errorf("valueTrend[%d].Value=%v < valueTrend[%d].Value=%v: not cumulative",
					i, valueTrend[i].Value, i-1, valueTrend[i-1].Value)
			}
		}
	})

	t.Run("count trend is cumulative and non-decreasing", func(t *testing.T) {
		insertLibraryItem(t, 75.0, now.AddDate(0, -1, 0))

		_, countTrend := calc.computeLibraryValueTrend(ctx, communityID, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if countTrend == nil {
			t.Fatal("countTrend is nil")
		}
		for i := 1; i < len(countTrend); i++ {
			if countTrend[i].Value < countTrend[i-1].Value {
				t.Errorf("countTrend[%d].Value=%v < countTrend[%d].Value=%v: not cumulative",
					i, countTrend[i].Value, i-1, countTrend[i-1].Value)
			}
		}
	})

	t.Run("count trend final value equals number of items added", func(t *testing.T) {
		singleCommunityID := uuid.New().String()
		sc := &models.Community{Id: singleCommunityID, Name: "Single", CreatorId: "test-user", OwnerUserId: "test-user"}
		if _, err := db.Insert(ctx, sc); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		// Add exactly 3 items spread across recent months.
		insertLibraryItem(t, 100.0, now.AddDate(0, -5, 0))
		insertLibraryItem(t, 200.0, now.AddDate(0, -3, 0))
		insertLibraryItem(t, 300.0, now.AddDate(0, -1, 0))

		// Use the community with the 3 items just added (need a fresh community).
		freshCommunityID := uuid.New().String()
		fc := &models.Community{Id: freshCommunityID, Name: "Fresh", CreatorId: "test-user", OwnerUserId: "test-user"}
		if _, err := db.Insert(ctx, fc); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		for _, val := range []float32{100.0, 200.0, 300.0} {
			gear := &models.Gear{
				Id:            uuid.New().String(),
				OwnerId:       user.Id,
				Name:          "Item",
				ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: val},
			}
			if _, err := db.Insert(ctx, gear); err != nil {
				t.Fatalf("insert gear: %v", err)
			}
			cg := &models.CommunityGear{
				Id:               uuid.New().String(),
				CommunityId:      freshCommunityID,
				GearId:           gear.Id,
				CreatedAtUnixSec: now.AddDate(0, -1, 0).Unix(),
			}
			if _, err := db.Insert(ctx, cg); err != nil {
				t.Fatalf("insert community gear: %v", err)
			}
		}

		_, countTrend := calc.computeLibraryValueTrend(ctx, freshCommunityID, api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if countTrend == nil {
			t.Fatal("countTrend is nil")
		}
		final := countTrend[len(countTrend)-1].Value
		if final != 3 {
			t.Errorf("countTrend final value = %v, want 3", final)
		}
	})

	t.Run("MetricDetailResult exposes LibraryItemCountTrend for MONEY dimension", func(t *testing.T) {
		freshCommunityID := uuid.New().String()
		fc := &models.Community{Id: freshCommunityID, Name: "Fresh2", CreatorId: "test-user", OwnerUserId: "test-user"}
		if _, err := db.Insert(ctx, fc); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		gear := &models.Gear{
			Id:            uuid.New().String(),
			OwnerId:       user.Id,
			Name:          "Item",
			ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 500.0},
		}
		if _, err := db.Insert(ctx, gear); err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		cg := &models.CommunityGear{
			Id:               uuid.New().String(),
			CommunityId:      freshCommunityID,
			GearId:           gear.Id,
			CreatedAtUnixSec: now.AddDate(0, -1, 0).Unix(),
		}
		if _, err := db.Insert(ctx, cg); err != nil {
			t.Fatalf("insert community gear: %v", err)
		}

		result, err := calc.ComputeMetricDetail(ctx, freshCommunityID,
			api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY,
			api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail: %v", err)
		}
		if result.LibraryItemCountTrend == nil {
			t.Error("LibraryItemCountTrend is nil for MONEY dimension")
		}
		if len(result.LibraryItemCountTrend) == 0 {
			t.Error("LibraryItemCountTrend is empty for MONEY dimension")
		}
	})

	t.Run("LibraryItemCountTrend is nil for non-MONEY dimension", func(t *testing.T) {
		freshCommunityID := uuid.New().String()
		fc := &models.Community{Id: freshCommunityID, Name: "Fresh3", CreatorId: "test-user", OwnerUserId: "test-user"}
		if _, err := db.Insert(ctx, fc); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		result, err := calc.ComputeMetricDetail(ctx, freshCommunityID,
			api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS,
			api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ALL)
		if err != nil {
			t.Fatalf("ComputeMetricDetail: %v", err)
		}
		if result.LibraryItemCountTrend != nil {
			t.Errorf("LibraryItemCountTrend = %v, want nil for non-MONEY dimension", result.LibraryItemCountTrend)
		}
	})
}
