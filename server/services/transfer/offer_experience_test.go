package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// expOfferFixture is the world an OfferExperienceTransfer test starts from: a
// participant (lender) who owns available gear, a host with a live event, and
// one community both belong to with the event shared into it. The lender claimed
// a gear-backed need, producing an experience-scoped contribution.
type expOfferFixture struct {
	service        *Service
	storage        *storage.ProtoSQLStorage
	mockNotif      *notifications.MockService
	done           chan struct{}
	lenderID       string
	hostID         string
	gearID         string
	communityID    string
	experienceID   string
	contributionID string
	sharedWith     []models.Availability
}

func setupExpOfferFixture(t *testing.T) *expOfferFixture {
	t.Helper()
	service, testStorage, mockNotif, done := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	f := &expOfferFixture{service: service, storage: testStorage, mockNotif: mockNotif, done: done}
	f.lenderID = setupTestUser(t, testStorage, "Theo", "theo@example.com")
	f.hostID = setupTestUser(t, testStorage, "June", "june@example.com")
	f.gearID = setupTestGear(t, testStorage, f.lenderID, models.GearState_GEAR_STATE_AVAILABLE)
	f.communityID = setupCommunityAndShareGear(t, testStorage, f.lenderID, f.hostID, f.gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	experience := &models.Experience{
		OwnerId:     f.hostID,
		Name:        "Community garden yard day",
		Description: "Bring tools",
		State:       models.ExperienceState_EXPERIENCE_STATE_JOINED,
	}
	experienceID, err := testStorage.Insert(ctx, experience)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	f.experienceID = experienceID
	if _, err := testStorage.Insert(ctx, &models.CommunityExperience{
		ExperienceId: experienceID,
		CommunityId:  f.communityID,
	}); err != nil {
		t.Fatalf("insert community experience: %v", err)
	}

	gearID := f.gearID
	contribution := &models.PlanningContribution{
		ContributorId: f.lenderID,
		Title:         "Wheelbarrow",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: experienceID},
		GearId:        &gearID,
	}
	contributionID, err := testStorage.Insert(ctx, contribution)
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}
	f.contributionID = contributionID

	service.SetGearSharer(func(ctx context.Context, gearID, communityID, actorUserID string, availability models.Availability) error {
		f.sharedWith = append(f.sharedWith, availability)
		existing, err := storage.QueryByFields[*models.CommunityGear](testStorage, ctx, map[string]any{
			"gear_id":      gearID,
			"community_id": communityID,
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			existing[0].Availability = availability
			return testStorage.Update(ctx, existing[0])
		}
		_, err = testStorage.Insert(ctx, &models.CommunityGear{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: availability,
		})
		return err
	})
	return f
}

func (f *expOfferFixture) offerRequest(transferType api.TransferType) *api.OfferExperienceTransferRequest {
	return &api.OfferExperienceTransferRequest{
		GearId:             f.gearID,
		TransferType:       transferType,
		CommunityId:        f.communityID,
		OriginExperienceId: f.experienceID,
		ContributionId:     f.contributionID,
	}
}

func (f *expOfferFixture) lenderCtx() context.Context {
	return createAuthenticatedContext(f.lenderID, "theo@example.com", models.Role_ROLE_USER)
}

func TestOfferExperienceTransfer_Loan_HappyPath(t *testing.T) {
	f := setupExpOfferFixture(t)

	resp, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)))
	if err != nil {
		t.Fatalf("OfferExperienceTransfer failed: %v", err)
	}

	tr := resp.Msg.Transfer
	if tr.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("expected RECIPIENT_SELECTED, got %s", tr.State)
	}
	if tr.GetOriginExperienceId() != f.experienceID {
		t.Errorf("expected origin_experience_id %s, got %q", f.experienceID, tr.GetOriginExperienceId())
	}
	// Recipient is the event host, derived server-side (never supplied by the caller).
	if tr.Recipient == nil || tr.Recipient.Id != f.hostID {
		t.Errorf("expected recipient host %s, got %+v", f.hostID, tr.Recipient)
	}

	// The contribution now carries the transfer link.
	contribution := &models.PlanningContribution{}
	if err := f.storage.GetByID(context.Background(), f.contributionID, contribution); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	if contribution.GetTransferId() != tr.Id {
		t.Errorf("expected contribution.transfer_id %s, got %q", tr.Id, contribution.GetTransferId())
	}

	// The gear was shared with the loan availability.
	if len(f.sharedWith) != 1 || f.sharedWith[0] != models.Availability_AVAILABILITY_FOR_LOAN {
		t.Errorf("expected one FOR_LOAN share, got %v", f.sharedWith)
	}
}

func TestOfferExperienceTransfer_Giveaway_SharesForGiveaway(t *testing.T) {
	f := setupExpOfferFixture(t)

	resp, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_GIVEAWAY)))
	if err != nil {
		t.Fatalf("OfferExperienceTransfer failed: %v", err)
	}
	if resp.Msg.Transfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
		t.Errorf("expected GIVEAWAY, got %s", resp.Msg.Transfer.TransferType)
	}
	if len(f.sharedWith) != 1 || f.sharedWith[0] != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
		t.Errorf("expected one FOR_GIVEAWAY share, got %v", f.sharedWith)
	}
}

func TestOfferExperienceTransfer_DerivesLoanDurationFromEventEnd(t *testing.T) {
	f := setupExpOfferFixture(t)

	// Give the event a scheduled time far in the future so the derived duration
	// is deterministically positive regardless of the wall clock.
	exp := &models.Experience{}
	if err := f.storage.GetByID(context.Background(), f.experienceID, exp); err != nil {
		t.Fatalf("reload experience: %v", err)
	}
	exp.Time = &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: 4102444800, // 2100-01-01
				DurationMinutes:  180,
			},
		},
	}
	if err := f.storage.Update(context.Background(), exp); err != nil {
		t.Fatalf("update experience time: %v", err)
	}

	resp, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)))
	if err != nil {
		t.Fatalf("OfferExperienceTransfer failed: %v", err)
	}
	if resp.Msg.Transfer.LoanDurationDays == nil || resp.Msg.Transfer.GetLoanDurationDays() <= 0 {
		t.Errorf("expected a positive derived loan_duration_days, got %v", resp.Msg.Transfer.LoanDurationDays)
	}
}

func TestOfferExperienceTransfer_RecipientSelectedPushSuppressed(t *testing.T) {
	f := setupExpOfferFixture(t)

	if _, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("OfferExperienceTransfer failed: %v", err)
	}
	<-f.done

	for _, call := range f.mockNotif.GetCalls() {
		if call.UserID == f.hostID {
			t.Errorf("expected no push to the host for an origin-linked RECIPIENT_SELECTED, got %+v", call)
		}
	}
}

func TestOfferExperienceTransfer_Validation(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context
		wantCode connect.Code
	}{
		{
			name: "host offering on own event",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				// The host owns gear and tries to bring it to their own event.
				hostGearID := setupTestGear(t, f.storage, f.hostID, models.GearState_GEAR_STATE_AVAILABLE)
				req.GearId = hostGearID
				hostContribID, err := f.storage.Insert(context.Background(), &models.PlanningContribution{
					ContributorId: f.hostID,
					Title:         "Host's rake",
					Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: f.experienceID},
					GearId:        &hostGearID,
				})
				if err != nil {
					t.Fatalf("insert host contribution: %v", err)
				}
				req.ContributionId = hostContribID
				return createAuthenticatedContext(f.hostID, "june@example.com", models.Role_ROLE_USER)
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "terminal event",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				exp := &models.Experience{}
				if err := f.storage.GetByID(context.Background(), f.experienceID, exp); err != nil {
					t.Fatalf("reload experience: %v", err)
				}
				exp.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED
				if err := f.storage.Update(context.Background(), exp); err != nil {
					t.Fatalf("update experience: %v", err)
				}
				return f.lenderCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "gear unavailable",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				gear := &models.Gear{}
				if err := f.storage.GetByID(context.Background(), f.gearID, gear); err != nil {
					t.Fatalf("reload gear: %v", err)
				}
				gear.State = models.GearState_GEAR_STATE_UNAVAILABLE
				if err := f.storage.Update(context.Background(), gear); err != nil {
					t.Fatalf("update gear: %v", err)
				}
				return f.lenderCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "event not shared into community",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				rows, err := storage.QueryByFields[*models.CommunityExperience](f.storage, context.Background(), map[string]any{
					"experience_id": f.experienceID,
				})
				if err != nil || len(rows) == 0 {
					t.Fatalf("load community experience: %v", err)
				}
				for _, r := range rows {
					if err := f.storage.Delete(context.Background(), r); err != nil {
						t.Fatalf("delete community experience: %v", err)
					}
				}
				return f.lenderCtx()
			},
			wantCode: connect.CodeFailedPrecondition,
		},
		{
			name: "contribution owned by someone else",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				contribution := &models.PlanningContribution{}
				if err := f.storage.GetByID(context.Background(), f.contributionID, contribution); err != nil {
					t.Fatalf("reload contribution: %v", err)
				}
				contribution.ContributorId = f.hostID
				if err := f.storage.Update(context.Background(), contribution); err != nil {
					t.Fatalf("update contribution: %v", err)
				}
				return f.lenderCtx()
			},
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "contribution scoped to a different experience",
			mutate: func(t *testing.T, f *expOfferFixture, req *api.OfferExperienceTransferRequest) context.Context {
				contribution := &models.PlanningContribution{}
				if err := f.storage.GetByID(context.Background(), f.contributionID, contribution); err != nil {
					t.Fatalf("reload contribution: %v", err)
				}
				contribution.Scope = &models.PlanningContribution_ExperienceId{ExperienceId: "some-other-event"}
				if err := f.storage.Update(context.Background(), contribution); err != nil {
					t.Fatalf("update contribution: %v", err)
				}
				return f.lenderCtx()
			},
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := setupExpOfferFixture(t)
			req := f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)
			ctx := tc.mutate(t, f, req)

			_, err := f.service.OfferExperienceTransfer(ctx, connect.NewRequest(req))
			if err == nil {
				t.Fatal("expected error, got success")
			}
			if connect.CodeOf(err) != tc.wantCode {
				t.Errorf("expected code %s, got %s (%v)", tc.wantCode, connect.CodeOf(err), err)
			}
		})
	}
}

func TestOfferExperienceTransfer_DuplicateOfferRejected(t *testing.T) {
	f := setupExpOfferFixture(t)

	if _, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN))); err != nil {
		t.Fatalf("first OfferExperienceTransfer failed: %v", err)
	}

	// A second contribution escalating the same gear on the same event is
	// rejected while the first offer is live.
	ctx := context.Background()
	gearID := f.gearID
	secondContributionID, err := f.storage.Insert(ctx, &models.PlanningContribution{
		ContributorId: f.lenderID,
		Title:         "Wheelbarrow again",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: f.experienceID},
		GearId:        &gearID,
	})
	if err != nil {
		t.Fatalf("insert second contribution: %v", err)
	}
	req := f.offerRequest(api.TransferType_TRANSFER_TYPE_LOAN)
	req.ContributionId = secondContributionID

	_, err = f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(req))
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", err)
	}
}
