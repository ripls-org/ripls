package transfer

import (
	"context"

	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// RequestSubscriberName is the stable identifier used in pubsub log lines.
const RequestSubscriberName = "transfer_request_coupling"

// requestEventSubscriber couples request lifecycle events to the gear-backed
// offer transfers that target them (#2702): a fulfilled request politely
// cancels its still-open sibling offers, a handoff covering one need of a
// multi-need request cancels that need's siblings, and undoing a
// handoff-driven fulfillment cancels the transfer that drove it. Bus-driven
// so the request and transfer services stay independent.
type requestEventSubscriber struct {
	s *Service
}

// RequestEventSubscriber returns the bus subscriber that couples request
// lifecycle events to origin-linked transfers. Wire it onto the
// community-event bus alongside the other production subscribers.
func (s *Service) RequestEventSubscriber() cebus.Subscriber {
	return &requestEventSubscriber{s: s}
}

// Name implements pubsub.Subscriber.
func (r *requestEventSubscriber) Name() string { return RequestSubscriberName }

// Handle implements pubsub.Subscriber. Returns nil for events this subscriber
// doesn't care about; per-transfer failures are logged and swallowed —
// helpers can always withdraw a stale offer manually.
func (r *requestEventSubscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event

	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED:
		// The request is covered — settle its still-open gear offers (#2703):
		// a confirmed helper's offer is finalized (giveaway completes, loan goes
		// active) so the fulfillment actually delivers the gear; the rest stand
		// down. Fires once per shared community; every transition is idempotent.
		r.s.finalizeOriginOffersOnFulfillment(ctx, event.GetRequestId())
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE:
		// Undoing a fulfillment reverses the gear it delivered (design decision
		// 10, generalized in #2703). A single-need handoff-driven fulfillment
		// stands down its one driving transfer; a manual multi-need fulfillment
		// can have DELIVERED several offers (loans went active, giveaways
		// completed) — so every still-cancellable origin offer on the request
		// stands down. A completed giveaway is permanent and is left alone.
		r.s.cancelDeliveredOffersOnFulfillmentUndo(ctx, event.GetRequestId())
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:
		// A handoff on a multi-need request covers one need: that need's other
		// open offers stand down. (Single-need requests are covered by the
		// REQUEST_FULFILLED case once the auto-fulfillment lands.)
		transfer := &models.Transfer{}
		if event.GetTransferId() == "" {
			return nil
		}
		if err := r.s.storage.GetByID(ctx, event.GetTransferId(), transfer); err != nil {
			return nil
		}
		requestID := transfer.GetOriginRequestId()
		if requestID == "" {
			return nil
		}
		needID := r.s.originTransferNeedID(ctx, requestID, transfer.Id)
		if needID != "" && r.s.needWantsExactlyOne(ctx, needID) {
			// Only a single-slot need is fully covered by one handoff. A
			// multi-slot need ("3 rakes") still wants the other offers.
			r.s.cancelOpenOriginOffers(ctx, requestID, needID, transfer.Id)
		}
	default:
	}
	return nil
}

// originTransferNeedID resolves which need (if any) the contribution carrying
// this transfer claims. Empty when the offer was freestanding.
func (s *Service) originTransferNeedID(ctx context.Context, requestID, transferID string) string {
	contributions, err := storage.QueryByField[*models.PlanningContribution](s.storage, ctx, "request_id", requestID)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to query contributions for need scoping",
			"target_request_id", requestID, "error", err)
		return ""
	}
	for _, c := range contributions {
		if c.GetTransferId() == transferID {
			return c.GetFromNeedId()
		}
	}
	return ""
}

// needWantsExactlyOne reports whether the need asked for a single slot.
func (s *Service) needWantsExactlyOne(ctx context.Context, needID string) bool {
	need := &models.PlanningNeed{}
	if err := s.storage.GetByID(ctx, needID, need); err != nil {
		return false
	}
	return need.Slots == 1
}

// cancelOpenOriginOffers cancels the still-open (pre-handoff) gear-backed
// offers on a request — every origin-linked transfer in INTEREST_EXPRESSED or
// RECIPIENT_SELECTED — excluding excludeTransferID. When needID is non-empty,
// only offers whose contribution claims that need are cancelled (the
// multi-need per-need rule, design decision 9). Each cancellation notifies
// the helper with the "request was covered" copy via the standard
// TRANSFER_CANCELLED pipeline (actor = the requester). Idempotent: already
// terminal transfers are skipped.
func (s *Service) cancelOpenOriginOffers(ctx context.Context, requestID, needID, excludeTransferID string) {
	if requestID == "" {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "cancelOpenOriginOffers",
		"target_request_id", requestID,
	)

	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", requestID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query origin transfers", "error", err)
		return
	}

	var needByTransfer map[string]string
	if needID != "" {
		contributions, err := storage.QueryByField[*models.PlanningContribution](s.storage, ctx, "request_id", requestID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to query contributions for need scoping", "error", err)
			return
		}
		needByTransfer = make(map[string]string, len(contributions))
		for _, c := range contributions {
			if c.GetTransferId() != "" {
				needByTransfer[c.GetTransferId()] = c.GetFromNeedId()
			}
		}
	}

	for _, t := range transfers {
		if t.Id == excludeTransferID {
			continue
		}
		if t.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED &&
			t.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			continue
		}
		if needID != "" && needByTransfer[t.Id] != needID {
			continue
		}
		s.cancelCoveredOffer(ctx, t, logger)
	}
}

// cancelDeliveredOffersOnFulfillmentUndo stands down every still-cancellable
// origin-linked transfer on a request whose fulfillment was undone (#2703). A
// manual multi-need fulfillment can deliver several offers at once — loans go
// active, giveaways complete; undoing it reverses the loans (and any offer that
// never handed off) while a completed giveaway stays put, the item having
// permanently changed hands. The per-transfer cancel-vs-leave decision is the
// state guard in cancelOriginTransferByID; idempotent on replayed events.
func (s *Service) cancelDeliveredOffersOnFulfillmentUndo(ctx context.Context, requestID string) {
	if requestID == "" {
		return
	}
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", requestID)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx,
			"failed to query origin transfers for fulfillment-undo",
			"target_request_id", requestID, "error", err)
		return
	}
	for _, t := range transfers {
		s.cancelOriginTransferByID(ctx, t.Id)
	}
}

// cancelOriginTransferByID cancels a specific origin-linked transfer while it
// is still cancellable (pre-handoff states or an active loan). A completed
// giveaway is terminal — the item permanently changed hands — so it is left
// alone with an INFO log.
func (s *Service) cancelOriginTransferByID(ctx context.Context, transferID string) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "cancelOriginTransferByID",
		"transfer_id", transferID,
	)
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transfer); err != nil {
		logger.WarnContext(ctx, "transfer missing for fulfillment-undo unwind", "error", err)
		return
	}
	switch transfer.State {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		models.TransferState_TRANSFER_STATE_ACTIVE:
		s.cancelCoveredOffer(ctx, transfer, logger)
	default:
		logger.InfoContext(ctx, "transfer is terminal; fulfillment undo leaves it unchanged",
			"state", transfer.State.String())
	}
}

// cancelCoveredOffer cancels one origin-linked transfer with the requester as
// the acting party (either party may cancel), posting the standard cancellation
// message to the gear conversation first so undo plumbing keeps working.
func (s *Service) cancelCoveredOffer(ctx context.Context, t *models.Transfer, logger *logging.Logger) {
	var msgID string
	if s.systemMessageWriter != nil {
		conversationID, err := s.getGearConversationID(ctx, t.GearId, t.CommunityId)
		if err != nil {
			logger.WarnContext(ctx, "failed to get gear conversation for covered-offer cancel",
				"gear_id", t.GearId, "transfer_id", t.Id, "error", err)
		} else {
			id, err := s.systemMessageWriter.InsertLocalizedReturnID(ctx, conversationID, t.RecipientId,
				models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, chat.CancelledTransferMessage())
			if err != nil {
				logger.WarnContext(ctx, "failed to insert covered-offer cancel message",
					"transfer_id", t.Id, "error", err)
			} else {
				msgID = id
			}
		}
	}
	if _, _, _, err := s.transitionTransferState(ctx, t, t.RecipientId, models.TransferState_TRANSFER_STATE_CANCELLED, msgID); err != nil {
		logger.ErrorContext(ctx, "failed to cancel covered offer", "transfer_id", t.Id, "error", err)
		return
	}
	logger.InfoContext(ctx, "cancelled covered gear offer", "transfer_id", t.Id)
}

// finalizeOriginOffersOnFulfillment settles the still-open gear-backed offers on
// a just-fulfilled request (#2703). An offer from a helper the requester
// CONFIRMED in the Mark Fulfilled modal is finalized — the gear actually
// changes hands: a giveaway completes (ownership → the requester) and a loan
// goes active (the requester now holds it). An offer from an unconfirmed helper
// stands down, matching the prior blanket behavior. The confirmed-helper set is
// read from the freshly persisted request row. Fires once per shared community;
// every transition is idempotent — an offer already finalized or cancelled on a
// sibling event is no longer in an open state and is skipped.
func (s *Service) finalizeOriginOffersOnFulfillment(ctx context.Context, requestID string) {
	if requestID == "" {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "finalizeOriginOffersOnFulfillment",
		"target_request_id", requestID,
	)

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, requestStored); err != nil {
		logger.WarnContext(ctx, "request missing; cannot finalize origin offers", "error", err)
		return
	}
	confirmed := make(map[string]bool, len(requestStored.ConfirmedHelperIds))
	for _, id := range requestStored.ConfirmedHelperIds {
		confirmed[id] = true
	}

	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", requestID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query origin transfers", "error", err)
		return
	}
	for _, t := range transfers {
		// Only a still-open offer needs settling. An origin offer is born
		// RECIPIENT_SELECTED (the requester is its recipient); INTEREST_EXPRESSED
		// shouldn't occur but is stood down for safety.
		switch t.State {
		case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
			if confirmed[t.OwnerId] {
				s.finalizeCoveredOffer(ctx, t, logger)
			} else {
				s.cancelCoveredOffer(ctx, t, logger)
			}
		case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
			s.cancelCoveredOffer(ctx, t, logger)
		default:
			// Already terminal or active — nothing to settle.
		}
	}
}

// finalizeCoveredOffer delivers a confirmed helper's gear offer as part of a
// request's fulfillment: a giveaway completes (RECIPIENT_SELECTED → COMPLETED,
// ownership transfers to the requester), a loan goes active (RECIPIENT_SELECTED
// → ACTIVE, the requester holds it until it's returned). The requester is the
// acting party (either party may drive the transfer). Mirrors the item-based
// impact stamping StartLoan / CompleteTransfer do so the delivered gear carries
// its own estimate.
func (s *Service) finalizeCoveredOffer(ctx context.Context, t *models.Transfer, logger *logging.Logger) {
	target := models.TransferState_TRANSFER_STATE_COMPLETED
	action := models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED
	var msg chat.LocalizedMessage
	if t.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
		target = models.TransferState_TRANSFER_STATE_ACTIVE
		action = models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED
		msg = chat.StartedLoanMessage()
	} else {
		// Giveaway delivery names the recipient ("Given to X") — "transfer"
		// is internal jargon banned in user copy (#2724).
		msg = chat.GivenToMessage(s.getUserDisplayName(ctx, t.RecipientId))
	}

	var msgID string
	if s.systemMessageWriter != nil {
		conversationID, err := s.getGearConversationID(ctx, t.GearId, t.CommunityId)
		if err != nil {
			logger.WarnContext(ctx, "failed to get gear conversation for finalize",
				"gear_id", t.GearId, "transfer_id", t.Id, "error", err)
		} else if id, err := s.systemMessageWriter.InsertLocalizedReturnID(ctx, conversationID, t.RecipientId, action, msg); err != nil {
			logger.WarnContext(ctx, "failed to insert finalize system message", "transfer_id", t.Id, "error", err)
		} else {
			msgID = id
		}
	}

	updated, gear, _, err := s.transitionTransferState(ctx, t, t.RecipientId, target, msgID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to finalize covered offer", "transfer_id", t.Id, "error", err)
		return
	}

	// Stamp the item-based impact on the delivered transfer (mirrors
	// StartLoan / CompleteTransfer). The request adopts these estimates via the
	// request-service coupling so it never double-counts against them.
	if s.estimatorCfg != nil {
		if gear == nil {
			gear = &models.Gear{}
			if err := s.storage.GetByID(ctx, updated.GearId, gear); err != nil {
				logger.WarnContext(ctx, "failed to load gear for finalize impact", "gear_id", updated.GearId, "error", err)
			}
		}
		connCtx := s.resolveConnectionContext(ctx, updated.OwnerId, updated.RecipientId, updated.CommunityId, logger)
		ie := impact_metrics.BuildTransferImpactMetrics(gear, updated.TransferType, s.estimatorCfg, connCtx, nil)
		updated.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
		if err := s.storage.Update(ctx, updated); err != nil {
			logger.ErrorContext(ctx, "failed to persist finalize impact", "transfer_id", t.Id, "error", err)
		}
	}
	logger.InfoContext(ctx, "finalized covered gear offer on fulfillment",
		"transfer_id", t.Id, "final_state", target.String())
}
