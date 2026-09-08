package impact_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestCalculateActivityCounts tests the activity counts calculation.
func TestCalculateActivityCounts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, db *storage.ProtoSQLStorage, communityID string)
		want  *ActivityCounts
	}{
		{
			name:  "empty community",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {},
			want: &ActivityCounts{
				GearCount:          0,
				ActiveLoans:        0,
				CompletedLoans:     0,
				OpenGiveaways:      0,
				CompletedGiveaways: 0,
				OpenRequests:       0,
				FulfilledRequests:  0,
				UpcomingEvents:     0,
				PastEvents:         0,
				MemberCount:        0,
			},
		},
		{
			name: "community with active and completed loans",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				// Create active loan
				activeLoan := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					State:        models.TransferState_TRANSFER_STATE_ACTIVE,
				}
				if _, err := db.Insert(ctx, activeLoan); err != nil {
					t.Fatalf("Failed to insert active loan: %v", err)
				}

				// Create completed loan
				completedLoan := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					State:        models.TransferState_TRANSFER_STATE_COMPLETED,
				}
				if _, err := db.Insert(ctx, completedLoan); err != nil {
					t.Fatalf("Failed to insert completed loan: %v", err)
				}

				// Create soft-deleted loan (should be excluded)
				deletedLoan := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					State:        models.TransferState_TRANSFER_STATE_ACTIVE,
					Deleted:      &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()},
				}
				if _, err := db.Insert(ctx, deletedLoan); err != nil {
					t.Fatalf("Failed to insert deleted loan: %v", err)
				}
			},
			want: &ActivityCounts{
				ActiveLoans:    1,
				CompletedLoans: 1,
			},
		},
		{
			name: "community with giveaways",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				// Create open giveaway (interest expressed)
				openGiveaway1 := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
					State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
				}
				if _, err := db.Insert(ctx, openGiveaway1); err != nil {
					t.Fatalf("Failed to insert open giveaway: %v", err)
				}

				// Create open giveaway (recipient selected)
				openGiveaway2 := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
					State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
				}
				if _, err := db.Insert(ctx, openGiveaway2); err != nil {
					t.Fatalf("Failed to insert open giveaway: %v", err)
				}

				// Create completed giveaway
				completedGiveaway := &models.Transfer{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
					State:        models.TransferState_TRANSFER_STATE_COMPLETED,
				}
				if _, err := db.Insert(ctx, completedGiveaway); err != nil {
					t.Fatalf("Failed to insert completed giveaway: %v", err)
				}
			},
			want: &ActivityCounts{
				OpenGiveaways:      2,
				CompletedGiveaways: 1,
			},
		},
		{
			name: "community with requests",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()

				// Create active request
				activeRequest := &models.Request{
					Id:    uuid.New().String(),
					State: models.RequestState_REQUEST_STATE_ACTIVE,
				}
				if _, err := db.Insert(ctx, activeRequest); err != nil {
					t.Fatalf("Failed to insert active request: %v", err)
				}
				communityRequest1 := &models.CommunityRequest{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					RequestId:   activeRequest.Id,
					Archived:    false,
				}
				if _, err := db.Insert(ctx, communityRequest1); err != nil {
					t.Fatalf("Failed to insert community request: %v", err)
				}

				// Create request with offers
				offersRequest := &models.Request{
					Id:    uuid.New().String(),
					State: models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
				}
				if _, err := db.Insert(ctx, offersRequest); err != nil {
					t.Fatalf("Failed to insert offers request: %v", err)
				}
				communityRequest2 := &models.CommunityRequest{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					RequestId:   offersRequest.Id,
					Archived:    false,
				}
				if _, err := db.Insert(ctx, communityRequest2); err != nil {
					t.Fatalf("Failed to insert community request: %v", err)
				}

				// Create fulfilled request
				fulfilledRequest := &models.Request{
					Id:    uuid.New().String(),
					State: models.RequestState_REQUEST_STATE_FULFILLED,
				}
				if _, err := db.Insert(ctx, fulfilledRequest); err != nil {
					t.Fatalf("Failed to insert fulfilled request: %v", err)
				}
				communityRequest3 := &models.CommunityRequest{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					RequestId:   fulfilledRequest.Id,
					Archived:    false,
				}
				if _, err := db.Insert(ctx, communityRequest3); err != nil {
					t.Fatalf("Failed to insert community request: %v", err)
				}

				// Create archived request (should be excluded)
				archivedRequest := &models.Request{
					Id:    uuid.New().String(),
					State: models.RequestState_REQUEST_STATE_ACTIVE,
				}
				if _, err := db.Insert(ctx, archivedRequest); err != nil {
					t.Fatalf("Failed to insert archived request: %v", err)
				}
				archivedCommunityRequest := &models.CommunityRequest{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					RequestId:   archivedRequest.Id,
					Archived:    true,
				}
				if _, err := db.Insert(ctx, archivedCommunityRequest); err != nil {
					t.Fatalf("Failed to insert archived community request: %v", err)
				}
			},
			want: &ActivityCounts{
				OpenRequests:      2,
				FulfilledRequests: 1,
			},
		},
		{
			name: "community with experiences",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				// Create upcoming experience (future time)
				upcomingExp := &models.Experience{
					Id:    uuid.New().String(),
					State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
					Time: &models.ExperienceTime{
						TimeType: &models.ExperienceTime_Specific{
							Specific: &models.SpecificTime{
								UnixTimestampSec: now + 86400, // Tomorrow
							},
						},
					},
				}
				if _, err := db.Insert(ctx, upcomingExp); err != nil {
					t.Fatalf("Failed to insert upcoming experience: %v", err)
				}
				communityExp1 := &models.CommunityExperience{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					ExperienceId: upcomingExp.Id,
				}
				if _, err := db.Insert(ctx, communityExp1); err != nil {
					t.Fatalf("Failed to insert community experience: %v", err)
				}

				// Create completed experience
				completedExp := &models.Experience{
					Id:    uuid.New().String(),
					State: models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
				}
				if _, err := db.Insert(ctx, completedExp); err != nil {
					t.Fatalf("Failed to insert completed experience: %v", err)
				}
				communityExp2 := &models.CommunityExperience{
					Id:           uuid.New().String(),
					CommunityId:  communityID,
					ExperienceId: completedExp.Id,
				}
				if _, err := db.Insert(ctx, communityExp2); err != nil {
					t.Fatalf("Failed to insert community experience: %v", err)
				}
			},
			want: &ActivityCounts{
				UpcomingEvents: 1,
				PastEvents:     1,
			},
		},
		{
			name: "community with members",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				// Create 3 members
				for i := 0; i < 3; i++ {
					user := createTestUser(t, db, "Member "+string(rune('A'+i)))
					member := &models.CommunityUser{
						Id:          uuid.New().String(),
						CommunityId: communityID,
						UserId:      user.Id,
					}
					if _, err := db.Insert(ctx, member); err != nil {
						t.Fatalf("Failed to insert community member: %v", err)
					}
				}
			},
			want: &ActivityCounts{
				MemberCount: 3,
			},
		},
		{
			name: "community with gear items",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				user := createTestUser(t, db, "Gear Owner")

				// Create 3 gear items
				for i := 0; i < 3; i++ {
					gear := &models.Gear{
						Id:          uuid.New().String(),
						OwnerId:     user.Id,
						Name:        "Test Gear " + string(rune('A'+i)),
						Description: "Test description",
					}
					if _, err := db.Insert(ctx, gear); err != nil {
						t.Fatalf("Failed to insert gear: %v", err)
					}

					// Link gear to community
					communityGear := &models.CommunityGear{
						Id:          uuid.New().String(),
						CommunityId: communityID,
						GearId:      gear.Id,
					}
					if _, err := db.Insert(ctx, communityGear); err != nil {
						t.Fatalf("Failed to insert community gear: %v", err)
					}
				}

				// Create soft-deleted gear (should be excluded)
				deletedGear := &models.Gear{
					Id:          uuid.New().String(),
					OwnerId:     user.Id,
					Name:        "Deleted Gear",
					Description: "Should not be counted",
					Deleted:     &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()},
				}
				if _, err := db.Insert(ctx, deletedGear); err != nil {
					t.Fatalf("Failed to insert deleted gear: %v", err)
				}
				communityGear := &models.CommunityGear{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					GearId:      deletedGear.Id,
				}
				if _, err := db.Insert(ctx, communityGear); err != nil {
					t.Fatalf("Failed to insert community gear for deleted item: %v", err)
				}
			},
			want: &ActivityCounts{
				GearCount: 3, // Only non-deleted gear
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := setupTestDatabase(t)
			defer db.Close()

			calc := NewCalculator(db, loadTestEstimatorConfig(t))

			// Create a community
			communityID := uuid.New().String()
			community := &models.Community{
				Id:          communityID,
				Name:        "Test Community",
				CreatorId:   "test-user",
				OwnerUserId: "test-user",
			}
			if _, err := db.Insert(ctx, community); err != nil {
				t.Fatalf("Failed to write community: %v", err)
			}

			// Run the test setup
			tt.setup(t, db, communityID)

			// Calculate activity counts
			got, err := calc.CalculateActivityCounts(ctx, communityID)
			if err != nil {
				t.Fatalf("CalculateActivityCounts() error = %v", err)
			}

			// Verify results
			if got.GearCount != tt.want.GearCount {
				t.Errorf("GearCount = %v, want %v", got.GearCount, tt.want.GearCount)
			}
			if got.ActiveLoans != tt.want.ActiveLoans {
				t.Errorf("ActiveLoans = %v, want %v", got.ActiveLoans, tt.want.ActiveLoans)
			}
			if got.CompletedLoans != tt.want.CompletedLoans {
				t.Errorf("CompletedLoans = %v, want %v", got.CompletedLoans, tt.want.CompletedLoans)
			}
			if got.OpenGiveaways != tt.want.OpenGiveaways {
				t.Errorf("OpenGiveaways = %v, want %v", got.OpenGiveaways, tt.want.OpenGiveaways)
			}
			if got.CompletedGiveaways != tt.want.CompletedGiveaways {
				t.Errorf("CompletedGiveaways = %v, want %v", got.CompletedGiveaways, tt.want.CompletedGiveaways)
			}
			if got.OpenRequests != tt.want.OpenRequests {
				t.Errorf("OpenRequests = %v, want %v", got.OpenRequests, tt.want.OpenRequests)
			}
			if got.FulfilledRequests != tt.want.FulfilledRequests {
				t.Errorf("FulfilledRequests = %v, want %v", got.FulfilledRequests, tt.want.FulfilledRequests)
			}
			if got.UpcomingEvents != tt.want.UpcomingEvents {
				t.Errorf("UpcomingEvents = %v, want %v", got.UpcomingEvents, tt.want.UpcomingEvents)
			}
			if got.PastEvents != tt.want.PastEvents {
				t.Errorf("PastEvents = %v, want %v", got.PastEvents, tt.want.PastEvents)
			}
			if got.MemberCount != tt.want.MemberCount {
				t.Errorf("MemberCount = %v, want %v", got.MemberCount, tt.want.MemberCount)
			}
		})
	}
}

// TestCalculateProblemsCounts verifies the "problems handled" X-of-Y buckets:
// handled (completed loans + fulfilled requests + claimed needs) over potential
// (all non-cancelled loans / requests / posted needs). Cancelled entries and
// giveaways are excluded.
func TestCalculateProblemsCounts(t *testing.T) {
	db := setupTestDatabase(t)
	ctx := context.Background()
	calc := NewCalculator(db, loadTestEstimatorConfig(t))
	communityID := uuid.New().String()

	insertTransfer := func(tt models.TransferType, state models.TransferState) {
		tr := &models.Transfer{
			Id:           uuid.New().String(),
			CommunityId:  communityID,
			TransferType: tt,
			State:        state,
		}
		if _, err := db.Insert(ctx, tr); err != nil {
			t.Fatalf("insert transfer: %v", err)
		}
	}
	// Loans: 1 completed (handled), 1 active (potential only), 1 cancelled
	// (excluded). A completed giveaway never counts toward the loans bucket.
	insertTransfer(models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_COMPLETED)
	insertTransfer(models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_ACTIVE)
	insertTransfer(models.TransferType_TRANSFER_TYPE_LOAN, models.TransferState_TRANSFER_STATE_CANCELLED)
	insertTransfer(models.TransferType_TRANSFER_TYPE_GIVEAWAY, models.TransferState_TRANSFER_STATE_COMPLETED)

	insertRequest := func(state models.RequestState) {
		r := &models.Request{Id: uuid.New().String(), State: state}
		if _, err := db.Insert(ctx, r); err != nil {
			t.Fatalf("insert request: %v", err)
		}
		cr := &models.CommunityRequest{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			RequestId:   r.Id,
		}
		if _, err := db.Insert(ctx, cr); err != nil {
			t.Fatalf("insert community request: %v", err)
		}
	}
	// Requests: 1 fulfilled (handled), 1 active (potential only), 1 cancelled.
	insertRequest(models.RequestState_REQUEST_STATE_FULFILLED)
	insertRequest(models.RequestState_REQUEST_STATE_ACTIVE)
	insertRequest(models.RequestState_REQUEST_STATE_CANCELLED)

	// One experience to scope the needs to.
	exp := &models.Experience{Id: uuid.New().String(), Name: "Exp", State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE}
	if _, err := db.Insert(ctx, exp); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	ce := &models.CommunityExperience{Id: uuid.New().String(), CommunityId: communityID, ExperienceId: exp.Id}
	if _, err := db.Insert(ctx, ce); err != nil {
		t.Fatalf("insert community experience: %v", err)
	}
	insertNeed := func(claimed bool) {
		need := &models.PlanningNeed{
			Id:             uuid.New().String(),
			Name:           "Chairs",
			Slots:          2,
			SlotsRemaining: 2,
			Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: exp.Id},
		}
		if _, err := db.Insert(ctx, need); err != nil {
			t.Fatalf("insert need: %v", err)
		}
		if claimed {
			pc := &models.PlanningContribution{
				Id:            uuid.New().String(),
				ContributorId: "u1",
				Title:         "Bring chairs",
				FromNeedId:    &need.Id,
				Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: exp.Id},
			}
			if _, err := db.Insert(ctx, pc); err != nil {
				t.Fatalf("insert contribution: %v", err)
			}
		}
	}
	// Needs: 1 claimed (handled), 1 unclaimed (potential only).
	insertNeed(true)
	insertNeed(false)

	got, err := calc.CalculateProblemsCounts(ctx, communityID)
	if err != nil {
		t.Fatalf("CalculateProblemsCounts: %v", err)
	}

	checks := []struct {
		name      string
		got, want int
	}{
		{"HandledLoans", got.HandledLoans, 1},
		{"PotentialLoans", got.PotentialLoans, 2},
		{"HandledRequests", got.HandledRequests, 1},
		{"PotentialRequests", got.PotentialRequests, 2},
		{"HandledNeeds", got.HandledNeeds, 1},
		{"PotentialNeeds", got.PotentialNeeds, 2},
		{"Handled", got.Handled(), 3},
		{"Potential", got.Potential(), 6},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}
