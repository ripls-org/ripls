package experience

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestUnwindCancelledExperienceOffer(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	expID, err := testStorage.Insert(ctx, &models.Experience{
		OwnerId: "host",
		Name:    "Yard day",
		State:   models.ExperienceState_EXPERIENCE_STATE_JOINED,
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	needID, err := testStorage.Insert(ctx, &models.PlanningNeed{
		ProposerId:     "host",
		Name:           "Wheelbarrow",
		Slots:          2,
		SlotsRemaining: 1, // one already claimed
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: expID},
	})
	if err != nil {
		t.Fatalf("insert need: %v", err)
	}
	transferID, err := testStorage.Insert(ctx, &models.Transfer{
		GearId:       "gear-x",
		OwnerId:      "lender",
		RecipientId:  "host",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		Origin:       &models.Transfer_OriginExperienceId{OriginExperienceId: expID},
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	gearID := "gear-x"
	contribID, err := testStorage.Insert(ctx, &models.PlanningContribution{
		ContributorId: "lender",
		Title:         "Wheelbarrow",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: expID},
		FromNeedId:    &needID,
		GearId:        &gearID,
		TransferId:    &transferID,
	})
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}

	transfer := &models.Transfer{}
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("reload transfer: %v", err)
	}
	service.unwindCancelledExperienceOffer(ctx, transfer)

	// The escalated contribution is soft-deleted (GetByID filters deleted rows).
	if err := testStorage.GetByID(ctx, contribID, &models.PlanningContribution{}); err == nil {
		t.Error("expected contribution soft-deleted after transfer cancel")
	}
	// Its need slot reopens.
	need := &models.PlanningNeed{}
	if err := testStorage.GetByID(ctx, needID, need); err != nil {
		t.Fatalf("reload need: %v", err)
	}
	if need.SlotsRemaining != 2 {
		t.Errorf("slots_remaining = %d, want 2 (slot released)", need.SlotsRemaining)
	}
}

func TestUnwindCancelledExperienceOffer_TerminalNoOp(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	expID, err := testStorage.Insert(ctx, &models.Experience{
		OwnerId: "host",
		Name:    "Yard day",
		State:   models.ExperienceState_EXPERIENCE_STATE_CANCELLED, // terminal
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	transferID, err := testStorage.Insert(ctx, &models.Transfer{
		GearId:       "gear-x",
		OwnerId:      "lender",
		RecipientId:  "host",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_CANCELLED,
		Origin:       &models.Transfer_OriginExperienceId{OriginExperienceId: expID},
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	contribID, err := testStorage.Insert(ctx, &models.PlanningContribution{
		ContributorId: "lender",
		Title:         "Wheelbarrow",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: expID},
		TransferId:    &transferID,
	})
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}

	transfer := &models.Transfer{}
	if err := testStorage.GetByID(ctx, transferID, transfer); err != nil {
		t.Fatalf("reload transfer: %v", err)
	}
	service.unwindCancelledExperienceOffer(ctx, transfer)

	// On a terminal event the contribution stays as history.
	contrib := &models.PlanningContribution{}
	if err := testStorage.GetByID(ctx, contribID, contrib); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	if contrib.Deleted != nil {
		t.Error("expected contribution untouched on a terminal event")
	}
}

func TestExperienceTransferSubscriber_HandleRoutesCancel(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	expID, err := testStorage.Insert(ctx, &models.Experience{
		OwnerId: "host",
		Name:    "Yard day",
		State:   models.ExperienceState_EXPERIENCE_STATE_JOINED,
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	transferID, err := testStorage.Insert(ctx, &models.Transfer{
		GearId:       "gear-x",
		OwnerId:      "lender",
		RecipientId:  "host",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_CANCELLED,
		Origin:       &models.Transfer_OriginExperienceId{OriginExperienceId: expID},
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	contribID, err := testStorage.Insert(ctx, &models.PlanningContribution{
		ContributorId: "lender",
		Title:         "Wheelbarrow",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: expID},
		TransferId:    &transferID,
	})
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}

	sub := service.TransferEventSubscriber()
	if err := sub.Handle(ctx, &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
			Topic:     &models.CommunityEvent_TransferId{TransferId: transferID},
		},
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if err := testStorage.GetByID(ctx, contribID, &models.PlanningContribution{}); err == nil {
		t.Error("expected contribution soft-deleted after TRANSFER_CANCELLED routed through Handle")
	}
}
