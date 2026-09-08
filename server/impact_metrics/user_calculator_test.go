package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestCalculateUserActivityCounts exercises per-user activity counting across
// gear, transfers, requests, and experiences — including soft-delete handling
// and the unique-community aggregation derived from CommunityGear.
func TestCalculateUserActivityCounts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, db *storage.ProtoSQLStorage, userID string)
		want  *UserActivityCounts
	}{
		{
			name:  "no activity",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {},
			want:  &UserActivityCounts{},
		},
		{
			name: "gear shared across two communities",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				// Two non-deleted gear items, one shared in two communities.
				gear1 := &models.Gear{Id: uuid.New().String(), OwnerId: userID, Name: "Drill"}
				if _, err := db.Insert(ctx, gear1); err != nil {
					t.Fatalf("insert gear1: %v", err)
				}
				gear2 := &models.Gear{Id: uuid.New().String(), OwnerId: userID, Name: "Saw"}
				if _, err := db.Insert(ctx, gear2); err != nil {
					t.Fatalf("insert gear2: %v", err)
				}

				communityA := uuid.New().String()
				communityB := uuid.New().String()
				for _, link := range []*models.CommunityGear{
					{Id: uuid.New().String(), CommunityId: communityA, GearId: gear1.Id},
					{Id: uuid.New().String(), CommunityId: communityB, GearId: gear1.Id},
					{Id: uuid.New().String(), CommunityId: communityA, GearId: gear2.Id},
				} {
					if _, err := db.Insert(ctx, link); err != nil {
						t.Fatalf("insert community gear: %v", err)
					}
				}

				// Soft-deleted gear must not be counted.
				deletedGear := &models.Gear{
					Id:      uuid.New().String(),
					OwnerId: userID,
					Name:    "Old Hammer",
					Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now},
				}
				if _, err := db.Insert(ctx, deletedGear); err != nil {
					t.Fatalf("insert deletedGear: %v", err)
				}
			},
			want: &UserActivityCounts{
				ItemsShared:    2,
				CommunityCount: 2,
			},
		},
		{
			name: "loans as owner counted by state",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE, false)
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// Soft-deleted active loan: excluded.
				deleted := &models.Transfer{
					Id:           uuid.New().String(),
					OwnerId:      userID,
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					State:        models.TransferState_TRANSFER_STATE_ACTIVE,
					Deleted:      &models.DeletedMetadata{DeletedAtUnixSec: now},
				}
				if _, err := db.Insert(ctx, deleted); err != nil {
					t.Fatalf("insert deleted loan: %v", err)
				}
			},
			want: &UserActivityCounts{
				ActiveLoans:    1,
				CompletedLoans: 2,
				TotalLoans:     3,
			},
		},
		{
			name: "completed giveaways as owner counted",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()

				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED, false)
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED, false)
				// Open giveaway should not bump CompletedGiveaways.
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, false)
			},
			want: &UserActivityCounts{
				CompletedGiveaways: 2,
			},
		},
		{
			name: "borrows as recipient counted by state",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()

				// Loan borrows count.
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE, false)
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// Giveaway borrows are NOT counted (only loan-type transfers count for borrows).
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// Soft-deleted: excluded.
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE, true)
			},
			want: &UserActivityCounts{
				ActiveBorrows:    1,
				CompletedBorrows: 2,
				TotalBorrows:     3,
			},
		},
		{
			name: "fulfilled requests counted; other states excluded",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				fulfilled := &models.Request{
					Id:          uuid.New().String(),
					RequesterId: userID,
					State:       models.RequestState_REQUEST_STATE_FULFILLED,
				}
				if _, err := db.Insert(ctx, fulfilled); err != nil {
					t.Fatalf("insert fulfilled request: %v", err)
				}

				active := &models.Request{
					Id:          uuid.New().String(),
					RequesterId: userID,
					State:       models.RequestState_REQUEST_STATE_ACTIVE,
				}
				if _, err := db.Insert(ctx, active); err != nil {
					t.Fatalf("insert active request: %v", err)
				}

				// Soft-deleted fulfilled: excluded.
				deleted := &models.Request{
					Id:          uuid.New().String(),
					RequesterId: userID,
					State:       models.RequestState_REQUEST_STATE_FULFILLED,
					Deleted:     &models.DeletedMetadata{DeletedAtUnixSec: now},
				}
				if _, err := db.Insert(ctx, deleted); err != nil {
					t.Fatalf("insert deleted request: %v", err)
				}
			},
			want: &UserActivityCounts{
				FulfilledRequests: 1,
			},
		},
		{
			name: "events created counted; soft-deleted excluded",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				for i := 0; i < 3; i++ {
					exp := &models.Experience{
						Id:      uuid.New().String(),
						OwnerId: userID,
						State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
					}
					if _, err := db.Insert(ctx, exp); err != nil {
						t.Fatalf("insert experience: %v", err)
					}
				}

				// Soft-deleted: excluded.
				deleted := &models.Experience{
					Id:      uuid.New().String(),
					OwnerId: userID,
					State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
					Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now},
				}
				if _, err := db.Insert(ctx, deleted); err != nil {
					t.Fatalf("insert deleted experience: %v", err)
				}
			},
			want: &UserActivityCounts{
				EventsCreated: 3,
			},
		},
		{
			name: "mixed activity scenario",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()

				// 1 gear in 1 community.
				gear := &models.Gear{Id: uuid.New().String(), OwnerId: userID, Name: "Drill"}
				if _, err := db.Insert(ctx, gear); err != nil {
					t.Fatalf("insert gear: %v", err)
				}
				cg := &models.CommunityGear{
					Id:          uuid.New().String(),
					CommunityId: uuid.New().String(),
					GearId:      gear.Id,
				}
				if _, err := db.Insert(ctx, cg); err != nil {
					t.Fatalf("insert community gear: %v", err)
				}

				// 1 active loan, 1 completed loan, 1 completed giveaway as owner.
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE, false)
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)
				insertOwnerTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// 1 completed borrow.
				insertRecipientTransfer(t, db, ctx, userID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// 1 fulfilled request.
				req := &models.Request{
					Id:          uuid.New().String(),
					RequesterId: userID,
					State:       models.RequestState_REQUEST_STATE_FULFILLED,
				}
				if _, err := db.Insert(ctx, req); err != nil {
					t.Fatalf("insert request: %v", err)
				}

				// 1 event created.
				exp := &models.Experience{
					Id:      uuid.New().String(),
					OwnerId: userID,
					State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
				}
				if _, err := db.Insert(ctx, exp); err != nil {
					t.Fatalf("insert experience: %v", err)
				}
			},
			want: &UserActivityCounts{
				ItemsShared:        1,
				CommunityCount:     1,
				ActiveLoans:        1,
				CompletedLoans:     1,
				TotalLoans:         2,
				CompletedBorrows:   1,
				TotalBorrows:       1,
				CompletedGiveaways: 1,
				FulfilledRequests:  1,
				EventsCreated:      1,
			},
		},
		{
			name: "other users' activity does not leak",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				otherUserID := uuid.New().String()

				// Other user's gear: must not be counted.
				otherGear := &models.Gear{Id: uuid.New().String(), OwnerId: otherUserID, Name: "Other"}
				if _, err := db.Insert(ctx, otherGear); err != nil {
					t.Fatalf("insert other gear: %v", err)
				}

				// Other user's loan as owner.
				insertOwnerTransfer(t, db, ctx, otherUserID,
					models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED, false)

				// Other user's request.
				otherReq := &models.Request{
					Id:          uuid.New().String(),
					RequesterId: otherUserID,
					State:       models.RequestState_REQUEST_STATE_FULFILLED,
				}
				if _, err := db.Insert(ctx, otherReq); err != nil {
					t.Fatalf("insert other request: %v", err)
				}
			},
			want: &UserActivityCounts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := setupTestDatabase(t)
			calc := NewCalculator(db, loadTestEstimatorConfig(t))

			user := createTestUser(t, db, "Test User")
			tt.setup(t, db, user.Id)

			got, err := calc.CalculateUserActivityCounts(ctx, user.Id)
			if err != nil {
				t.Fatalf("CalculateUserActivityCounts() error = %v", err)
			}

			if got.ItemsShared != tt.want.ItemsShared {
				t.Errorf("ItemsShared = %d, want %d", got.ItemsShared, tt.want.ItemsShared)
			}
			if got.CommunityCount != tt.want.CommunityCount {
				t.Errorf("CommunityCount = %d, want %d", got.CommunityCount, tt.want.CommunityCount)
			}
			if got.ActiveLoans != tt.want.ActiveLoans {
				t.Errorf("ActiveLoans = %d, want %d", got.ActiveLoans, tt.want.ActiveLoans)
			}
			if got.CompletedLoans != tt.want.CompletedLoans {
				t.Errorf("CompletedLoans = %d, want %d", got.CompletedLoans, tt.want.CompletedLoans)
			}
			if got.TotalLoans != tt.want.TotalLoans {
				t.Errorf("TotalLoans = %d, want %d", got.TotalLoans, tt.want.TotalLoans)
			}
			if got.ActiveBorrows != tt.want.ActiveBorrows {
				t.Errorf("ActiveBorrows = %d, want %d", got.ActiveBorrows, tt.want.ActiveBorrows)
			}
			if got.CompletedBorrows != tt.want.CompletedBorrows {
				t.Errorf("CompletedBorrows = %d, want %d", got.CompletedBorrows, tt.want.CompletedBorrows)
			}
			if got.TotalBorrows != tt.want.TotalBorrows {
				t.Errorf("TotalBorrows = %d, want %d", got.TotalBorrows, tt.want.TotalBorrows)
			}
			if got.CompletedGiveaways != tt.want.CompletedGiveaways {
				t.Errorf("CompletedGiveaways = %d, want %d", got.CompletedGiveaways, tt.want.CompletedGiveaways)
			}
			if got.FulfilledRequests != tt.want.FulfilledRequests {
				t.Errorf("FulfilledRequests = %d, want %d", got.FulfilledRequests, tt.want.FulfilledRequests)
			}
			if got.EventsCreated != tt.want.EventsCreated {
				t.Errorf("EventsCreated = %d, want %d", got.EventsCreated, tt.want.EventsCreated)
			}
		})
	}
}

// TestCalculateUserImpactSavings exercises the per-user savings rollup. Only
// completed loans contribute cost/carbon; time aggregates across loans,
// fulfilled requests, and non-deleted experiences.
func TestCalculateUserImpactSavings(t *testing.T) {
	t.Run("no activity returns zero estimates", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Empty User")

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateZero(t, "CostSavings", result.CostSavings)
		assertEstimateZero(t, "CarbonSavings", result.CarbonSavings)
		assertEstimateZero(t, "TimeBanked", result.TimeBanked)
		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0", result.CostCount)
		}
		if result.CarbonCount != 0 {
			t.Errorf("CarbonCount = %d, want 0", result.CarbonCount)
		}
	})

	t.Run("completed loan with ImpactEstimate aggregates cost, carbon, and loan time", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Lender")

		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(100, 5000, 60), false)

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "CostSavings", result.CostSavings, 100)
		if result.CostCount != 1 {
			t.Errorf("CostCount = %d, want 1", result.CostCount)
		}
		// Carbon comes from EmissionsPrevented.ManufactureAvoidedCarbon = 5000.
		assertEstimateMean(t, "CarbonSavings", result.CarbonSavings, 5000)
		if result.CarbonCount != 1 {
			t.Errorf("CarbonCount = %d, want 1", result.CarbonCount)
		}
		assertEstimateMean(t, "TimeFromLoans", result.TimeFromLoans, 60)
		assertEstimateMean(t, "TimeBanked", result.TimeBanked, 60)
	})

	t.Run("non-completed loans excluded from cost", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Active Lender")

		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE,
			testImpactEstimate(500, 9000, 120), false)

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0 (active loan not counted)", result.CostCount)
		}
		assertEstimateZero(t, "CostSavings", result.CostSavings)
	})

	t.Run("completed giveaway excluded from owner impact savings", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Giver")

		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(200, 8000, 30), false)

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		// User savings only counts completed LOANs from the owner side.
		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0 (giveaway not counted in user savings)", result.CostCount)
		}
	})

	t.Run("soft-deleted completed loan excluded", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Lender")

		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(100, 5000, 60), true)

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0 (deleted)", result.CostCount)
		}
	})

	t.Run("fulfilled request contributes time only", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Helper")

		req := &models.Request{
			Id:             uuid.New().String(),
			RequesterId:    user.Id,
			State:          models.RequestState_REQUEST_STATE_FULFILLED,
			ImpactEstimate: testImpactEstimate(0, 0, 45),
		}
		if _, err := db.Insert(ctx, req); err != nil {
			t.Fatalf("insert fulfilled request: %v", err)
		}

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0 (requests carry no cost)", result.CostCount)
		}
		assertEstimateMean(t, "TimeFromRequests", result.TimeFromRequests, 45)
	})

	t.Run("non-fulfilled request excluded", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Open Requester")

		req := &models.Request{
			Id:             uuid.New().String(),
			RequesterId:    user.Id,
			State:          models.RequestState_REQUEST_STATE_ACTIVE,
			ImpactEstimate: testImpactEstimate(0, 0, 100),
		}
		if _, err := db.Insert(ctx, req); err != nil {
			t.Fatalf("insert active request: %v", err)
		}

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateZero(t, "TimeFromRequests", result.TimeFromRequests)
	})

	t.Run("non-deleted experience contributes time-from-skills", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Host")

		exp := &models.Experience{
			Id:             uuid.New().String(),
			OwnerId:        user.Id,
			State:          models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
			ImpactEstimate: testImpactEstimate(0, 0, 90),
		}
		if _, err := db.Insert(ctx, exp); err != nil {
			t.Fatalf("insert experience: %v", err)
		}

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "TimeFromSkills", result.TimeFromSkills, 90)
	})

	t.Run("soft-deleted experience excluded", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Host")

		exp := &models.Experience{
			Id:             uuid.New().String(),
			OwnerId:        user.Id,
			State:          models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
			ImpactEstimate: testImpactEstimate(0, 0, 90),
			Deleted:        &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()},
		}
		if _, err := db.Insert(ctx, exp); err != nil {
			t.Fatalf("insert deleted experience: %v", err)
		}

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateZero(t, "TimeFromSkills", result.TimeFromSkills)
	})

	t.Run("mixed sources sum into TimeBanked", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Polymath")

		// Two completed loans: cost 100+200 = 300, time 60+90 = 150.
		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(100, 5000, 60), false)
		insertOwnerTransferWithImpact(t, db, ctx, user.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(200, 8000, 90), false)

		// Fulfilled request: time 45.
		req := &models.Request{
			Id:             uuid.New().String(),
			RequesterId:    user.Id,
			State:          models.RequestState_REQUEST_STATE_FULFILLED,
			ImpactEstimate: testImpactEstimate(0, 0, 45),
		}
		if _, err := db.Insert(ctx, req); err != nil {
			t.Fatalf("insert request: %v", err)
		}

		// Experience: time 120.
		exp := &models.Experience{
			Id:             uuid.New().String(),
			OwnerId:        user.Id,
			State:          models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
			ImpactEstimate: testImpactEstimate(0, 0, 120),
		}
		if _, err := db.Insert(ctx, exp); err != nil {
			t.Fatalf("insert experience: %v", err)
		}

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateMean(t, "CostSavings", result.CostSavings, 300)
		if result.CostCount != 2 {
			t.Errorf("CostCount = %d, want 2", result.CostCount)
		}
		assertEstimateMean(t, "TimeFromLoans", result.TimeFromLoans, 150)
		assertEstimateMean(t, "TimeFromRequests", result.TimeFromRequests, 45)
		assertEstimateMean(t, "TimeFromSkills", result.TimeFromSkills, 120)
		// Total banked time = 150 + 45 + 120 = 315.
		assertEstimateMean(t, "TimeBanked", result.TimeBanked, 315)

		// Quadrature stddev should be > 0 across multiple positive estimates.
		if result.CostSavings.Stddev <= 0 {
			t.Errorf("CostSavings.Stddev = %v, want > 0 (quadrature)", result.CostSavings.Stddev)
		}
	})

	t.Run("other users' transfers do not leak into user savings", func(t *testing.T) {
		ctx := context.Background()
		db := setupTestDatabase(t)
		calc := NewCalculator(db, loadTestEstimatorConfig(t))
		user := createTestUser(t, db, "Target")
		other := createTestUser(t, db, "Other")

		insertOwnerTransferWithImpact(t, db, ctx, other.Id,
			models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED,
			testImpactEstimate(999, 9999, 99), false)

		result, err := calc.CalculateUserImpactSavings(ctx, user.Id)
		if err != nil {
			t.Fatalf("CalculateUserImpactSavings() error = %v", err)
		}

		assertEstimateZero(t, "CostSavings", result.CostSavings)
		if result.CostCount != 0 {
			t.Errorf("CostCount = %d, want 0", result.CostCount)
		}
	})
}

// TestCalculateUserTotalValue exercises per-user gear value aggregation.
func TestCalculateUserTotalValue(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, db *storage.ProtoSQLStorage, userID string)
		wantTotalUsd  float32
		wantGearCount int32
	}{
		{
			name:          "no gear",
			setup:         func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {},
			wantTotalUsd:  0,
			wantGearCount: 0,
		},
		{
			name: "single gear with value",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				gear := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       userID,
					Name:          "Drill",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 150},
				}
				if _, err := db.Insert(ctx, gear); err != nil {
					t.Fatalf("insert gear: %v", err)
				}
			},
			wantTotalUsd:  150,
			wantGearCount: 1,
		},
		{
			name: "multiple gear summed",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				for _, v := range []float32{100, 250, 50} {
					g := &models.Gear{
						Id:            uuid.New().String(),
						OwnerId:       userID,
						Name:          "Item",
						ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: v},
					}
					if _, err := db.Insert(ctx, g); err != nil {
						t.Fatalf("insert gear: %v", err)
					}
				}
			},
			wantTotalUsd:  400,
			wantGearCount: 3,
		},
		{
			name: "gear without value estimate excluded from count and sum",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				valued := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       userID,
					Name:          "Valued",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
				}
				if _, err := db.Insert(ctx, valued); err != nil {
					t.Fatalf("insert valued: %v", err)
				}
				unvalued := &models.Gear{
					Id:      uuid.New().String(),
					OwnerId: userID,
					Name:    "Unvalued",
				}
				if _, err := db.Insert(ctx, unvalued); err != nil {
					t.Fatalf("insert unvalued: %v", err)
				}
				zeroValue := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       userID,
					Name:          "Zero",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 0},
				}
				if _, err := db.Insert(ctx, zeroValue); err != nil {
					t.Fatalf("insert zero: %v", err)
				}
			},
			wantTotalUsd:  100,
			wantGearCount: 1,
		},
		{
			name: "soft-deleted gear excluded",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				active := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       userID,
					Name:          "Active",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
				}
				if _, err := db.Insert(ctx, active); err != nil {
					t.Fatalf("insert active: %v", err)
				}
				deleted := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       userID,
					Name:          "Deleted",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 500},
					Deleted:       &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()},
				}
				if _, err := db.Insert(ctx, deleted); err != nil {
					t.Fatalf("insert deleted: %v", err)
				}
			},
			wantTotalUsd:  100,
			wantGearCount: 1,
		},
		{
			name: "other owner's gear not counted",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, userID string) {
				ctx := context.Background()
				other := &models.Gear{
					Id:            uuid.New().String(),
					OwnerId:       uuid.New().String(),
					Name:          "Other",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 999},
				}
				if _, err := db.Insert(ctx, other); err != nil {
					t.Fatalf("insert other gear: %v", err)
				}
			},
			wantTotalUsd:  0,
			wantGearCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := setupTestDatabase(t)
			calc := NewCalculator(db, loadTestEstimatorConfig(t))

			user := createTestUser(t, db, "Owner")
			tt.setup(t, db, user.Id)

			result, err := calc.CalculateUserTotalValue(ctx, user.Id)
			if err != nil {
				t.Fatalf("CalculateUserTotalValue() error = %v", err)
			}

			if math.Abs(float64(result.TotalValueUsd-tt.wantTotalUsd)) > 0.01 {
				t.Errorf("TotalValueUsd = %v, want %v", result.TotalValueUsd, tt.wantTotalUsd)
			}
			if result.GearCount != tt.wantGearCount {
				t.Errorf("GearCount = %d, want %d", result.GearCount, tt.wantGearCount)
			}
		})
	}
}

// TestAggregateWithQuadrature verifies the quadrature helper directly. The
// public aggregations route through this helper, so a unit test pins the
// numeric contract independent of database setup.
func TestAggregateWithQuadrature(t *testing.T) {
	t.Run("empty slice returns zero estimate", func(t *testing.T) {
		got := aggregateWithQuadrature(nil)
		if got == nil {
			t.Fatal("got nil, want zero estimate")
		}
		if got.Mean != 0 || got.Stddev != 0 {
			t.Errorf("got {%v, %v}, want {0, 0}", got.Mean, got.Stddev)
		}
	})

	t.Run("single value uses 30 percent proportional uncertainty", func(t *testing.T) {
		got := aggregateWithQuadrature([]float64{100})
		if math.Abs(float64(got.Mean)-100) > 0.01 {
			t.Errorf("Mean = %v, want 100", got.Mean)
		}
		// Stddev = sqrt((100*0.3)^2) = 30.
		if math.Abs(float64(got.Stddev)-30) > 0.01 {
			t.Errorf("Stddev = %v, want 30", got.Stddev)
		}
	})

	t.Run("multiple values sum mean and combine stddev in quadrature", func(t *testing.T) {
		got := aggregateWithQuadrature([]float64{100, 200})
		// Mean = 100 + 200 = 300.
		if math.Abs(float64(got.Mean)-300) > 0.01 {
			t.Errorf("Mean = %v, want 300", got.Mean)
		}
		// Stddev = sqrt((30)^2 + (60)^2) = sqrt(4500) ≈ 67.082.
		want := float32(math.Sqrt(4500))
		if math.Abs(float64(got.Stddev-want)) > 0.01 {
			t.Errorf("Stddev = %v, want %v", got.Stddev, want)
		}
	})
}

// TestGetUser verifies retrieval of a user by ID and surfacing of not-found errors.
func TestGetUser(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	calc := NewCalculator(db, loadTestEstimatorConfig(t))

	t.Run("found", func(t *testing.T) {
		user := createTestUser(t, db, "Alice")
		got, err := calc.GetUser(ctx, user.Id)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}
		if got.Id != user.Id {
			t.Errorf("Id = %q, want %q", got.Id, user.Id)
		}
		if got.Name != "Alice" {
			t.Errorf("Name = %q, want Alice", got.Name)
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := calc.GetUser(ctx, uuid.New().String())
		if err == nil {
			t.Fatal("GetUser() error = nil, want non-nil for missing ID")
		}
	})
}

// TestGetLocation verifies retrieval of a location by ID and surfacing of not-found errors.
func TestGetLocation(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	calc := NewCalculator(db, loadTestEstimatorConfig(t))

	t.Run("found", func(t *testing.T) {
		name := "Test Park"
		loc := &models.Location{
			Id:   uuid.New().String(),
			Name: &name,
		}
		if _, err := db.Insert(ctx, loc); err != nil {
			t.Fatalf("insert location: %v", err)
		}

		got, err := calc.GetLocation(ctx, loc.Id)
		if err != nil {
			t.Fatalf("GetLocation() error = %v", err)
		}
		if got.Id != loc.Id {
			t.Errorf("Id = %q, want %q", got.Id, loc.Id)
		}
		if got.GetName() != "Test Park" {
			t.Errorf("Name = %q, want Test Park", got.GetName())
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := calc.GetLocation(ctx, uuid.New().String())
		if err == nil {
			t.Fatal("GetLocation() error = nil, want non-nil for missing ID")
		}
	})
}

// insertOwnerTransfer inserts a transfer where userID is the owner.
func insertOwnerTransfer(t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, userID string,
	transferType models.TransferType, state models.TransferState, deleted bool,
) {
	t.Helper()
	tr := &models.Transfer{
		Id:           uuid.New().String(),
		OwnerId:      userID,
		TransferType: transferType,
		State:        state,
	}
	if deleted {
		tr.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	}
	if _, err := db.Insert(ctx, tr); err != nil {
		t.Fatalf("insert owner transfer: %v", err)
	}
}

// insertRecipientTransfer inserts a transfer where userID is the recipient.
func insertRecipientTransfer(t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, userID string,
	transferType models.TransferType, state models.TransferState, deleted bool,
) {
	t.Helper()
	tr := &models.Transfer{
		Id:           uuid.New().String(),
		RecipientId:  userID,
		TransferType: transferType,
		State:        state,
	}
	if deleted {
		tr.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	}
	if _, err := db.Insert(ctx, tr); err != nil {
		t.Fatalf("insert recipient transfer: %v", err)
	}
}

// insertOwnerTransferWithImpact inserts a transfer owned by userID with a
// preset ImpactEstimate, used by savings tests to bypass fallback computation.
func insertOwnerTransferWithImpact(t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, userID string,
	transferType models.TransferType, state models.TransferState,
	ie *models.ImpactEstimate, deleted bool,
) {
	t.Helper()
	tr := &models.Transfer{
		Id:             uuid.New().String(),
		OwnerId:        userID,
		TransferType:   transferType,
		State:          state,
		ImpactEstimate: ie,
	}
	if deleted {
		tr.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	}
	if _, err := db.Insert(ctx, tr); err != nil {
		t.Fatalf("insert owner transfer with impact: %v", err)
	}
}
