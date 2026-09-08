package request

import (
	"context"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/storage"
)

// TransferSubscriberName is the stable identifier used in pubsub log lines.
const TransferSubscriberName = "request_transfer_coupling"

// transferEventSubscriber couples origin-linked transfer lifecycle events to
// this service's request lifecycle (#2702): a gear-backed offer's handoff
// auto-fulfills a single-need request, and its cancellation unwinds the claim and
// offer it rode on. Bus-driven so the transfer and request services stay
// independent.
type transferEventSubscriber struct {
	s *Service
}

// TransferEventSubscriber returns the bus subscriber that couples
// origin-linked transfer events to request lifecycle. Wire it onto the
// community-event bus alongside the other production subscribers.
func (s *Service) TransferEventSubscriber() cebus.Subscriber {
	return &transferEventSubscriber{s: s}
}

// Name implements pubsub.Subscriber.
func (t *transferEventSubscriber) Name() string { return TransferSubscriberName }

// Handle implements pubsub.Subscriber. Returns nil for events this subscriber
// doesn't care about; per-event failures are logged and swallowed by
// documented design — the requester's manual paths (Mark Fulfilled, unclaim,
// withdraw) remain the fallback for every coupling this automates.
func (t *transferEventSubscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event

	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:
		transfer, ok := t.loadOriginTransfer(ctx, event.GetTransferId())
		if !ok {
			return nil
		}
		t.s.autoFulfillFromTransfer(ctx, transfer)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED:
		transfer, ok := t.loadOriginTransfer(ctx, event.GetTransferId())
		if !ok {
			return nil
		}
		t.s.unwindCancelledOffer(ctx, transfer)
	default:
	}
	return nil
}

// loadOriginTransfer loads the event's transfer and reports whether it is an
// origin-linked gear-backed offer this subscriber acts on.
func (t *transferEventSubscriber) loadOriginTransfer(ctx context.Context, transferID string) (*models.Transfer, bool) {
	if transferID == "" {
		return nil, false
	}
	transfer := &models.Transfer{}
	if err := t.s.storage.GetByID(ctx, transferID, transfer); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "transfer missing for request coupling",
			"transfer_id", transferID, "error", err)
		return nil, false
	}
	if transfer.GetOriginRequestId() == "" {
		return nil, false
	}
	return transfer, true
}

// unwindCancelledOffer unwinds the claim and offer a cancelled gear-backed
// offer rode on (#2702, design decision 11): the escalated contribution is
// removed, its need slot reopens, and the helper's RequestOffer withdraws
// unless another live contribution of theirs remains on the request. Skipped
// entirely once the request is terminal — the sibling cancellations that
// follow a fulfillment must not mutate the closed request. Idempotent on
// replayed events (the contribution is already gone).
func (s *Service) unwindCancelledOffer(ctx context.Context, transfer *models.Transfer) {
	requestID := transfer.GetOriginRequestId()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "unwindCancelledOffer",
		"transfer_id", transfer.Id,
		"target_request_id", requestID,
	)

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, requestStored); err != nil {
		logger.WarnContext(ctx, "origin request missing; nothing to unwind", "error", err)
		return
	}
	if isTerminalRequestState(requestStored.State) {
		logger.DebugContext(ctx, "request terminal; cancellation unwind is a no-op")
		return
	}

	contributions, err := storage.QueryByField[*models.PlanningContribution](s.storage, ctx, "request_id", requestID)
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

	// The helper may still be helping another way; only a helper with no
	// remaining live contribution stands down.
	for _, c := range contributions {
		if c.Id != linked.Id && c.ContributorId == transfer.OwnerId && c.Deleted == nil {
			logger.InfoContext(ctx, "helper keeps another live contribution; offer stays")
			return
		}
	}

	offers, err := storage.QueryByFields[*models.RequestOffer](s.storage, ctx, map[string]any{
		"request_id": requestID,
		"user_id":    transfer.OwnerId,
		"withdrawn":  false,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query offers for unwind", "error", err)
		return
	}
	for _, offer := range offers {
		offer.Withdrawn = true
		if err := s.storage.Update(ctx, offer); err != nil {
			logger.ErrorContext(ctx, "failed to withdraw offer", "error", err)
			continue
		}
		if err := s.recordOfferWithdrawnEvent(ctx, requestID, transfer.OwnerId, offer.CommunityId); err != nil {
			logger.WarnContext(ctx, "failed to record offer withdrawn event", "error", err)
		}
	}
	if len(offers) == 0 {
		return
	}

	if requestStored.ConversationId != "" {
		if s.systemMessageWriter != nil {
			displayName := s.getUserDisplayName(ctx, transfer.OwnerId)
			if err := s.systemMessageWriter.InsertLocalized(ctx, requestStored.ConversationId, transfer.OwnerId, models.ChatSystemAction_CHAT_SYSTEM_ACTION_LEFT, chat.WithdrewOfferMessage(displayName)); err != nil {
				logger.WarnContext(ctx, "failed to write LEFT system message", "error", err)
			}
		}
		if err := chat.RemoveParticipantFromConversation(ctx, storage.NewChatConversationStorage(s.storage), requestStored.ConversationId, transfer.OwnerId); err != nil {
			logger.WarnContext(ctx, "failed to remove helper from conversation", "error", err)
		}
	}

	// Existing last-offer reversion: a request with no offers left reopens.
	remaining, err := storage.QueryByFields[*models.RequestOffer](s.storage, ctx, map[string]any{
		"request_id": requestID,
		"withdrawn":  false,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to count remaining offers", "error", err)
		return
	}
	if len(remaining) == 0 && requestStored.State == models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		requestStored.State = models.RequestState_REQUEST_STATE_ACTIVE
		if err := s.storage.Update(ctx, requestStored); err != nil {
			logger.ErrorContext(ctx, "failed to revert request state", "error", err)
			return
		}
		logger.InfoContext(ctx, "last offer unwound; request reverted to ACTIVE")
	}
}
