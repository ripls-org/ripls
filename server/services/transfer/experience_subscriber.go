package transfer

import (
	"context"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ExperienceSubscriberName is the stable identifier used in pubsub log lines.
const ExperienceSubscriberName = "transfer_experience_coupling"

// experienceEventSubscriber couples experience lifecycle events to the gear
// brought to an event as child transfers (#2708): when the event completes,
// its child transfers complete (the item was contributed for the event's
// duration); when the event is cancelled, they cancel. The coupling is
// one-directional — a gear handoff never changes the event's state, since the
// host completes the event by their own action. Bus-driven so the experience
// and transfer services stay independent.
type experienceEventSubscriber struct {
	s *Service
}

// ExperienceEventSubscriber returns the bus subscriber that couples experience
// lifecycle events to origin-experience transfers. Wire it onto the
// community-event bus alongside the other production subscribers.
func (s *Service) ExperienceEventSubscriber() cebus.Subscriber {
	return &experienceEventSubscriber{s: s}
}

// Name implements pubsub.Subscriber.
func (e *experienceEventSubscriber) Name() string { return ExperienceSubscriberName }

// Handle implements pubsub.Subscriber. Returns nil for events this subscriber
// doesn't care about. Per-transfer failures are logged and swallowed by
// documented design: the event has already concluded, so the only cost of a
// miss is a stale child-transfer record, and the loop over communities makes
// the handler naturally idempotent (terminal transfers are skipped).
func (e *experienceEventSubscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event
	experienceID := event.GetExperienceId()
	if experienceID == "" {
		return nil
	}

	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		// The event happened — every child transfer of it completes (the gear
		// was contributed for the event's duration). Fires once per shared
		// community; completion is idempotent.
		e.s.completeEventChildTransfers(ctx, experienceID)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED:
		// The event was called off — pre-handoff child transfers stand down.
		e.s.cancelEventChildTransfers(ctx, experienceID)
	default:
	}
	return nil
}

// completeEventChildTransfers completes every non-terminal transfer that sprang
// from the given experience (#2708). Loans complete directly from
// RECIPIENT_SELECTED (no ACTIVE hop, so no return reminder and the gear was
// never made unavailable); giveaways complete their handoff. Item-based impact
// is stamped on each completed transfer so aggregation counts it once, on the
// transfer.
func (s *Service) completeEventChildTransfers(ctx context.Context, experienceID string) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "completeEventChildTransfers",
		"target_experience_id", experienceID,
	)
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_experience_id", experienceID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query origin-experience transfers", "error", err)
		return
	}
	for _, t := range transfers {
		if IsTerminalState(t.State) {
			continue
		}
		updated, _, _, err := s.transitionTransferState(ctx, t, t.RecipientId, models.TransferState_TRANSFER_STATE_COMPLETED, "")
		if err != nil {
			logger.ErrorContext(ctx, "failed to complete event child transfer", "transfer_id", t.Id, "error", err)
			continue
		}
		s.persistTransferCompletionImpact(ctx, updated, logger)
		logger.InfoContext(ctx, "completed event child transfer", "transfer_id", t.Id)
	}
}

// cancelEventChildTransfers cancels every non-terminal transfer that sprang
// from the given experience when the event is called off (#2708). The gear was
// never made unavailable (loans skip ACTIVE), so there is nothing to restore.
func (s *Service) cancelEventChildTransfers(ctx context.Context, experienceID string) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "cancelEventChildTransfers",
		"target_experience_id", experienceID,
	)
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_experience_id", experienceID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query origin-experience transfers", "error", err)
		return
	}
	for _, t := range transfers {
		if IsTerminalState(t.State) {
			continue
		}
		s.cancelCoveredOffer(ctx, t, logger)
	}
}

// persistTransferCompletionImpact computes and stores the item-based impact
// estimate on a just-completed transfer (money/emissions from the gear, no LLM
// hint — matching the StartLoan initial-estimate and request roll-up paths).
// Best-effort: a failure is logged, not fatal, since the completion already
// succeeded. No-op when no estimator is configured.
func (s *Service) persistTransferCompletionImpact(ctx context.Context, transfer *models.Transfer, logger *logging.Logger) {
	if s.estimatorCfg == nil || transfer == nil {
		return
	}
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
		logger.WarnContext(ctx, "gear missing for completion impact", "gear_id", transfer.GearId, "transfer_id", transfer.Id, "error", err)
		return
	}
	connCtx := s.resolveConnectionContext(ctx, transfer.OwnerId, transfer.RecipientId, transfer.CommunityId, logger)
	ie := impact_metrics.BuildTransferImpactMetrics(gear, transfer.TransferType, s.estimatorCfg, connCtx, nil)
	transfer.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
	if err := s.storage.Update(ctx, transfer); err != nil {
		logger.WarnContext(ctx, "failed to persist completion impact", "transfer_id", transfer.Id, "error", err)
	}
}
