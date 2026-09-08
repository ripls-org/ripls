package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// offerLiveExperienceTransfer runs OfferExperienceTransfer and returns the
// created (RECIPIENT_SELECTED) transfer id.
func (f *expOfferFixture) offerLiveExperienceTransfer(t *testing.T, transferType api.TransferType) string {
	t.Helper()
	resp, err := f.service.OfferExperienceTransfer(f.lenderCtx(), connect.NewRequest(f.offerRequest(transferType)))
	if err != nil {
		t.Fatalf("OfferExperienceTransfer failed: %v", err)
	}
	return resp.Msg.Transfer.Id
}

func (f *expOfferFixture) reloadTransfer(t *testing.T, id string) *models.Transfer {
	t.Helper()
	tr := &models.Transfer{}
	if err := f.storage.GetByID(context.Background(), id, tr); err != nil {
		t.Fatalf("reload transfer: %v", err)
	}
	return tr
}

func TestCompleteEventChildTransfers_Loan(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_LOAN)

	f.service.completeEventChildTransfers(context.Background(), f.experienceID)

	tr := f.reloadTransfer(t, transferID)
	if tr.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("state = %s, want COMPLETED", tr.State)
	}
	// The loan skipped ACTIVE, so no return reminder is ever anchored.
	if tr.ExpectedReturnUnixSec != nil {
		t.Errorf("expected_return_unix_sec = %v, want nil (loan never went ACTIVE)", tr.ExpectedReturnUnixSec)
	}
	// Item-based impact is stamped on completion so aggregation counts it once.
	if tr.ImpactEstimate == nil {
		t.Error("expected impact_estimate stamped on the completed transfer")
	}
	// The gear was never made unavailable (communal item the owner keeps).
	gear := &models.Gear{}
	if err := f.storage.GetByID(context.Background(), f.gearID, gear); err != nil {
		t.Fatalf("reload gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		t.Errorf("gear state = %s, want AVAILABLE", gear.State)
	}
}

func TestCompleteEventChildTransfers_Giveaway(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_GIVEAWAY)

	f.service.completeEventChildTransfers(context.Background(), f.experienceID)

	tr := f.reloadTransfer(t, transferID)
	if tr.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("state = %s, want COMPLETED", tr.State)
	}
	// A giveaway to the host transfers ownership.
	gear := &models.Gear{}
	if err := f.storage.GetByID(context.Background(), f.gearID, gear); err != nil {
		t.Fatalf("reload gear: %v", err)
	}
	if gear.State != models.GearState_GEAR_STATE_GIVEN_AWAY {
		t.Errorf("gear state = %s, want GIVEN_AWAY", gear.State)
	}
}

func TestCompleteEventChildTransfers_Idempotent(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_LOAN)

	f.service.completeEventChildTransfers(context.Background(), f.experienceID)
	// A replayed event (once per shared community) must be a no-op.
	f.service.completeEventChildTransfers(context.Background(), f.experienceID)

	if got := f.reloadTransfer(t, transferID).State; got != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("state = %s, want COMPLETED after replay", got)
	}
}

func TestCancelEventChildTransfers(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_LOAN)

	f.service.cancelEventChildTransfers(context.Background(), f.experienceID)

	if got := f.reloadTransfer(t, transferID).State; got != models.TransferState_TRANSFER_STATE_CANCELLED {
		t.Errorf("state = %s, want CANCELLED", got)
	}
}

func TestExperienceSubscriber_HandleRoutesCompletion(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_LOAN)

	sub := f.service.ExperienceEventSubscriber()
	err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
			Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
		},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := f.reloadTransfer(t, transferID).State; got != models.TransferState_TRANSFER_STATE_COMPLETED {
		t.Errorf("state = %s, want COMPLETED after EXPERIENCE_COMPLETED", got)
	}
}

func TestExperienceSubscriber_HandleRoutesCancellation(t *testing.T) {
	f := setupExpOfferFixture(t)
	transferID := f.offerLiveExperienceTransfer(t, api.TransferType_TRANSFER_TYPE_LOAN)

	sub := f.service.ExperienceEventSubscriber()
	err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
			Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
		},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := f.reloadTransfer(t, transferID).State; got != models.TransferState_TRANSFER_STATE_CANCELLED {
		t.Errorf("state = %s, want CANCELLED after EXPERIENCE_CANCELLED", got)
	}
}
