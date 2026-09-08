package experience

import (
	"context"

	"go.ripls.org/ripls/server/clock"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/storage"
)

// TransferSubscriberName is the stable identifier used in pubsub log lines.
const TransferSubscriberName = "experience_transfer_coupling"

// transferEventSubscriber couples origin-experience transfer cancellations back
// to this service's planning layer (#2708): when a participant cancels the gear
// they brought to an event (a pre-handoff transfer cancel), the escalated
// contribution is removed and its need slot reopens — the mirror of #2702's
// request-side unwind. Bus-driven so the transfer and experience services stay
// independent.
type transferEventSubscriber struct {
	s *Service
}

// TransferEventSubscriber returns the bus subscriber that couples origin-linked
// transfer cancellations to experience planning. Wire it onto the
// community-event bus alongside the other production subscribers.
func (s *Service) TransferEventSubscriber() cebus.Subscriber {
	return &transferEventSubscriber{s: s}
}

// Name implements pubsub.Subscriber.
func (t *transferEventSubscriber) Name() string { return TransferSubscriberName }

// Handle implements pubsub.Subscriber. Returns nil for events this subscriber
// doesn't care about; per-event failures are logged and swallowed — the
// participant can always re-claim.
func (t *transferEventSubscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event
	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED {
		return nil
	}
	if event.GetTransferId() == "" {
		return nil
	}
	transfer := &models.Transfer{}
	if err := t.s.storage.GetByID(ctx, event.GetTransferId(), transfer); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "transfer missing for experience coupling",
			"transfer_id", event.GetTransferId(), "error", err)
		return nil
	}
	if transfer.GetOriginExperienceId() == "" {
		return nil
	}
	t.s.unwindCancelledExperienceOffer(ctx, transfer)
	return nil
}

// unwindCancelledExperienceOffer removes the escalated contribution a cancelled
// gear-backed event offer rode on and reopens its need slot (#2708). Skipped
// once the experience is terminal — the transfer cancellations that follow an
// event cancellation must not mutate the closed event's planning list, and the
// contributions stay as history. Idempotent on replayed events (the
// contribution is already gone).
func (s *Service) unwindCancelledExperienceOffer(ctx context.Context, transfer *models.Transfer) {
	experienceID := transfer.GetOriginExperienceId()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "unwindCancelledExperienceOffer",
		"transfer_id", transfer.Id,
		"target_experience_id", experienceID,
	)

	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, exp); err != nil {
		logger.WarnContext(ctx, "origin experience missing; nothing to unwind", "error", err)
		return
	}
	if isTerminalExperienceState(exp.State) {
		logger.DebugContext(ctx, "experience terminal; cancellation unwind is a no-op")
		return
	}

	contributions, err := storage.QueryByField[*models.PlanningContribution](s.storage, ctx, "experience_id", experienceID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query contributions for unwind", "error", err)
		return
	}
	var linked *models.PlanningContribution
	for _, c := range contributions {
		if c.GetTransferId() == transfer.Id && c.Deleted == nil {
			linked = c
			break
		}
	}
	if linked == nil {
		logger.DebugContext(ctx, "no live contribution carries this transfer; unwind is a no-op")
		return
	}

	linked.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  transfer.OwnerId,
		DeletedAtUnixSec: clock.UnixSec(ctx),
	}
	if err := s.storage.Update(ctx, linked); err != nil {
		logger.ErrorContext(ctx, "failed to remove escalated contribution", "contribution_id", linked.Id, "error", err)
		return
	}
	if linked.FromNeedId != nil {
		planning.ReleaseSlot(ctx, s.storage, *linked.FromNeedId)
	}
	logger.InfoContext(ctx, "unwound cancelled experience gear offer", "contribution_id", linked.Id)
}
