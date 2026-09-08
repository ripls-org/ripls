package request

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// fulfillParams carries the caller-computed inputs to fulfillRequestCore.
// Both fulfillment paths — the requester's MarkRequestFulfilled RPC and the
// gear-backed handoff auto-fulfillment (#2702) — share the core so their side
// effects (events, messages, archives, undo snapshot, watches) cannot drift.
type fulfillParams struct {
	// actorID is the community-event actor. Always the requester: undo
	// authorization requires the undoing caller to be the event actor, and
	// the requester owns undoing a fulfillment.
	actorID            string
	fulfilledAtUnixSec int64
	resolutionSummary  *string
	confirmedHelperIDs []string
	// impact, when non-nil, is persisted on the request. The caller computes
	// it (value-based default, or transfer-adopted for gear-backed offers).
	impact *api.ImpactEstimate
	// fulfilledByTransferID records on the undo snapshot which origin-linked
	// transfer's handoff drove this fulfillment; undoing then also cancels
	// that transfer.
	fulfilledByTransferID string
	// adoptedFromTransferID marks the request's persisted impact as adopted
	// from this transfer (money/emissions/time are display copies; aggregation
	// counts them on the transfer — see impact_metrics masking).
	adoptedFromTransferID string
	// fulfillmentMessage is posted to the request's conversation per community.
	fulfillmentMessage *chat.LocalizedMessage
}

// fulfillRequestCore transitions the request to FULFILLED and performs every
// fulfillment side effect: undo snapshot, per-community system message +
// REQUEST_FULFILLED event + junction archive, impact persistence, and watch
// dismissal. Validation (requester-only, live state) is the caller's job.
// Returns the first emitted community event id (the undo anchor).
func (s *Service) fulfillRequestCore(ctx context.Context, requestStored *models.Request, p fulfillParams) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "fulfillRequestCore",
		"target_request_id", requestStored.Id,
	)

	// Snapshot prior values for UndoMarkRequestFulfilled BEFORE mutating.
	priorRequestState := requestStored.State
	priorImpactEstimate := requestStored.ImpactEstimate

	requestStored.State = models.RequestState_REQUEST_STATE_FULFILLED
	fulfilledAt := p.fulfilledAtUnixSec
	requestStored.FulfilledAtUnixSec = &fulfilledAt
	if p.resolutionSummary != nil {
		requestStored.ResolutionSummary = p.resolutionSummary
	}
	if len(p.confirmedHelperIDs) > 0 {
		requestStored.ConfirmedHelperIds = p.confirmedHelperIDs
	}
	if p.impact != nil {
		requestStored.ImpactEstimate = impact_metrics.APIImpactToModels(p.impact)
	}
	if p.adoptedFromTransferID != "" {
		adopted := p.adoptedFromTransferID
		requestStored.ImpactAdoptedFromTransferId = &adopted
	}
	if err := s.storage.Update(ctx, requestStored); err != nil {
		return "", fmt.Errorf("update request to fulfilled: %w", err)
	}

	fulfillUndo := &models.RequestFulfillmentUndo{
		PriorState:          priorRequestState,
		PriorImpactEstimate: priorImpactEstimate,
	}
	if p.fulfilledByTransferID != "" {
		fulfillUndo.FulfilledByTransferId = &p.fulfilledByTransferID
	}
	fulfillmentUndoData := &models.UndoData{
		Variant: &models.UndoData_RequestFulfillment{RequestFulfillment: fulfillUndo},
	}

	// Record events and system messages for all communities this request is
	// shared with, archiving each junction so the request leaves the feeds.
	communityRequests, err := storage.QueryByFields[*models.CommunityRequest](s.storage, ctx, map[string]any{
		"request_id": requestStored.Id,
		"archived":   false,
	})
	var primaryFulfillmentEventID string
	if err != nil {
		logger.Warn("failed to query community requests", "error", err)
	} else {
		for _, communityRequest := range communityRequests {
			convID := requestStored.ConversationId
			var fulfillmentMessageID string
			if convID != "" && p.fulfillmentMessage != nil && s.systemMessageWriter != nil {
				msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(ctx, convID, p.actorID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_FULFILLED, *p.fulfillmentMessage)
				if err != nil {
					logger.Warn("failed to write FULFILLED system message", "community_id", communityRequest.CommunityId, "error", err)
				} else {
					fulfillmentMessageID = msgID
				}
			}

			eventID, err := s.recordCommunityEventForRequest(ctx, requestStored.Id, models.RequestState_REQUEST_STATE_FULFILLED, p.actorID, communityRequest.CommunityId, fulfillmentMessageID, fulfillmentUndoData)
			if err != nil {
				logger.Warn("failed to record community event", "community_id", communityRequest.CommunityId, "error", err)
			} else if primaryFulfillmentEventID == "" {
				primaryFulfillmentEventID = eventID
			}

			communityRequest.Archived = true
			if err := s.storage.Update(ctx, communityRequest); err != nil {
				logger.Warn("failed to archive community request", "community_request_id", communityRequest.Id, "error", err)
			}
		}
	}

	// Dismiss all watches — request reached terminal state.
	wsf := storage.NewWatchStorage(s.storage)
	if err := wsf.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestStored.Id); err != nil {
		logger.Warn("failed to dismiss watches on request fulfillment", "error", err)
	}

	return primaryFulfillmentEventID, nil
}

// deliverableOriginTransfers returns the gear-backed offers on this request
// whose item-based impact the fulfillment delivers (#2724): transfers that
// already changed hands (loans ACTIVE, giveaways/loans COMPLETED) plus the
// still-open RECIPIENT_SELECTED offers from confirmed helpers — the exact set
// finalizeOriginOffersOnFulfillment (transfer service) finalizes when the
// REQUEST_FULFILLED event lands. With no confirmed helpers this reduces to
// the handed-off set.
func (s *Service) deliverableOriginTransfers(ctx context.Context, requestID string, confirmedHelperIDs []string) ([]*models.Transfer, error) {
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", requestID)
	if err != nil {
		return nil, fmt.Errorf("query origin transfers: %w", err)
	}
	confirmed := make(map[string]bool, len(confirmedHelperIDs))
	for _, id := range confirmedHelperIDs {
		confirmed[id] = true
	}
	deliverable := transfers[:0]
	for _, t := range transfers {
		switch t.State {
		case models.TransferState_TRANSFER_STATE_ACTIVE,
			models.TransferState_TRANSFER_STATE_COMPLETED:
			deliverable = append(deliverable, t)
		case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
			if confirmed[t.OwnerId] {
				deliverable = append(deliverable, t)
			}
		default:
		}
	}
	return deliverable, nil
}

// requestFulfillmentImpact builds a request's fulfillment impact from ONE set
// of inputs shared by every path that produces the number (#2724): the
// modal's live PreviewRequestImpact, the requester's MarkRequestFulfilled,
// and the bus-driven auto-fulfill/re-adopt flows. Routing them through the
// same computation is what guarantees the numbers a requester previews are
// the numbers that persist at commit.
//
// Money, emissions, and time adopt the item-based estimates of the request's
// deliverable gear-backed offers (see deliverableOriginTransfers); with none,
// the value-based default applies. Quality Time is always the request's own
// computation, rebuilt for the confirmed group size (helpers + requester)
// with the request's STORED social hints.
//
// Stored hints only, deliberately: a live preview cannot afford an LLM call
// per helper toggle, so inferring social attributes here would hand the
// commit an input the preview never had — and the headline number would
// change under the requester at the instant they commit. Every input must be
// one both paths can see (#2724). Detection-time inference still reaches
// this through the request's stored SocialContext.
//
// seedTransfers are unioned with the queried deliverable set (the bus-driven
// auto-fulfill passes its triggering handoff, which a read replica may not
// surface yet). The second return value is the transfer whose impact was
// adopted ("" when the value-based path ran).
func (s *Service) requestFulfillmentImpact(
	ctx context.Context,
	requestStored *models.Request,
	confirmedHelperIDs []string,
	confirmedHelperCount int32,
	communityID string,
	seedTransfers []*models.Transfer,
	logger *logging.Logger,
) (*api.ImpactEstimate, string) {
	if s.estimatorCfg == nil {
		return &api.ImpactEstimate{}, ""
	}

	helperCount := int32(len(confirmedHelperIDs))
	if confirmedHelperCount > 0 {
		helperCount = confirmedHelperCount
	}
	groupSize := helperCount + 1

	deliverable, err := s.deliverableOriginTransfers(ctx, requestStored.Id, confirmedHelperIDs)
	if err != nil {
		logger.Warn("failed to query origin transfers for impact; using value-based estimate", "error", err)
		deliverable = nil
	}
	seen := make(map[string]bool, len(deliverable))
	for _, t := range deliverable {
		seen[t.Id] = true
	}
	for _, t := range seedTransfers {
		if t != nil && !seen[t.Id] {
			deliverable = append(deliverable, t)
			seen[t.Id] = true
		}
	}

	// Connection context: requester ↔ first confirmed non-requester helper,
	// falling back to the first deliverable offer's owner.
	var counterpartID string
	for _, uid := range confirmedHelperIDs {
		if uid != requestStored.RequesterId {
			counterpartID = uid
			break
		}
	}
	if counterpartID == "" && len(deliverable) > 0 {
		counterpartID = deliverable[0].OwnerId
	}
	var connCtx *api.ConnectionContext
	if counterpartID != "" {
		connCtx = s.resolveConnectionContext(ctx, requestStored.RequesterId, counterpartID, communityID, logger)
	}

	// Stored attributes only — the sole source the live preview can see.
	hint := buildHintFromStoredRequest(requestStored)

	var ie *api.ImpactEstimate
	adoptedTransferID := ""
	if len(deliverable) > 0 {
		ie = s.transferAdoptedImpact(ctx, requestStored, deliverable, connCtx, logger)
		adoptedTransferID = deliverable[0].Id
	} else {
		var valueUSD float32
		if requestStored.ValueEstimate != nil {
			valueUSD = requestStored.ValueEstimate.EstimatedValueUsd
		}
		ie = impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, connCtx, hint)
	}
	if ie != nil {
		ie.QualityTime = impact_metrics.RebuildQualityTimeForGroupSizeWithContext(
			estimator.TransactionRequestFulfilled,
			groupSize,
			estimator.RoleMutual,
			connCtx,
			hint,
			fmt.Sprintf("Group size from %d confirmed helpers", helperCount),
			s.estimatorCfg,
		)
	}
	return ie, adoptedTransferID
}

// readoptImpactFromHandoffs rebuilds an already-fulfilled request's impact from
// its handed-off gear children and stamps the adopted-from marker (#2703). It
// runs when a confirmed gear offer is delivered *after* a multi-need request
// was marked fulfilled manually: the manual fulfillment recorded a value-based
// estimate with no adopted marker, so without this the request's money/CO2/time
// would double-count against the transfers now carrying their own item-based
// estimates. Idempotent and convergent — it recomputes over every handed-off
// child each time one lands, so the last delivery leaves the full roll-up. A
// no-op when nothing has handed off.
func (s *Service) readoptImpactFromHandoffs(ctx context.Context, requestStored *models.Request, communityID string, logger *logging.Logger) {
	impact, adoptedID := s.requestFulfillmentImpact(ctx, requestStored, requestStored.ConfirmedHelperIds, 0, communityID, nil, logger)
	if adoptedID == "" {
		// Nothing has handed off — keep the persisted estimate.
		return
	}
	// Re-adoption exists to stop the request double-counting its children's
	// money/emissions/time. Quality Time is not its business: the fulfillment
	// already computed it from the requester's confirmed set, in the request's
	// own community, and this runs per delivered child on the bus — with a
	// child's community for context. Recomputing it here overwrote the number
	// the requester was shown at commit with a slightly different one, moments
	// later and with no user action (#2724).
	if prior := impact_metrics.ModelsImpactToAPI(requestStored.ImpactEstimate); prior.GetQualityTime() != nil {
		impact.QualityTime = prior.QualityTime
	}
	requestStored.ImpactEstimate = impact_metrics.APIImpactToModels(impact)
	requestStored.ImpactAdoptedFromTransferId = &adoptedID
	if err := s.storage.Update(ctx, requestStored); err != nil {
		logger.ErrorContext(ctx, "failed to persist re-adopted impact", "error", err)
		return
	}
	logger.InfoContext(ctx, "re-adopted request impact from delivered gear")
}

// transferAdoptedImpact builds the fulfillment impact for a request satisfied
// by real gear (#2702): money, emissions, and time roll up from EVERY
// deliverable child transfer's item-based estimate (falling back to a fresh
// item-based build when a transfer has none persisted yet), summed with
// quadrature uncertainty; Quality Time keeps the request-side computation
// (the caller rebuilds it for the confirmed group size — see
// requestFulfillmentImpact). The adopted dimensions are display provenance
// only — community and user aggregation counts each child once, on the
// transfer (see impact_metrics.RequestDimensionValue). [transfers] must be
// non-empty.
func (s *Service) transferAdoptedImpact(ctx context.Context, requestStored *models.Request, transfers []*models.Transfer, connCtx *api.ConnectionContext, logger *logging.Logger) *api.ImpactEstimate {
	if s.estimatorCfg == nil {
		return &api.ImpactEstimate{}
	}

	var valueUSD float32
	if requestStored.ValueEstimate != nil {
		valueUSD = requestStored.ValueEstimate.EstimatedValueUsd
	}
	merged := impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, connCtx, nil)

	itemEstimates := make([]*api.ImpactEstimate, 0, len(transfers))
	for _, transfer := range transfers {
		itemBased := impact_metrics.ModelsImpactToAPI(transfer.ImpactEstimate)
		if itemBased == nil {
			gear := &models.Gear{}
			if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
				logger.Warn("gear missing for transfer-adopted impact; skipping child",
					"gear_id", transfer.GearId, "transfer_id", transfer.Id, "error", err)
				continue
			}
			itemBased = impact_metrics.BuildTransferImpactMetrics(gear, transfer.TransferType, s.estimatorCfg, connCtx, nil)
		}
		itemEstimates = append(itemEstimates, itemBased)
	}
	if len(itemEstimates) == 0 {
		// No child could produce an item-based estimate — keep the request's
		// own value-based defaults rather than zeroing the dimensions.
		return merged
	}

	summed := impact_metrics.SumTransferImpactDimensions(itemEstimates)
	merged.MoneySaved = summed.MoneySaved
	merged.EmissionsPrevented = summed.EmissionsPrevented
	merged.TimeSaved = summed.TimeSaved
	return merged
}

// autoFulfillFromTransfer fulfills a request whose gear-backed offer just
// changed hands (#2702): loan ACTIVE or giveaway COMPLETED on an origin-linked
// transfer. Applies only to requests with at most one need — multi-need
// requests stay open (each handoff covers its need; the requester closes via
// Mark Fulfilled). Idempotent: terminal requests and replayed events are
// no-ops. Failures are logged at Error and swallowed by documented design —
// the loan is already physically underway and the requester's manual Mark
// Fulfilled is the fallback.
func (s *Service) autoFulfillFromTransfer(ctx context.Context, transfer *models.Transfer) {
	requestID := transfer.GetOriginRequestId()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "autoFulfillFromTransfer",
		"transfer_id", transfer.Id,
		"target_request_id", requestID,
	)

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, requestStored); err != nil {
		logger.ErrorContext(ctx, "origin request not found; skipping auto-fulfill", "error", err)
		return
	}
	if isTerminalRequestState(requestStored.State) {
		// The request is already fulfilled — either by a prior single-need
		// handoff, or manually by the requester on a multi-need request whose
		// confirmed gear offers are now being delivered (#2703). In the latter
		// case the manual fulfillment left a value-based estimate; re-adopt the
		// handed-off children's item-based impact so the request stops
		// double-counting against them (RequestDimensionValue zeroes an adopted
		// request's money/CO2/time, keeping only its Quality Time).
		s.readoptImpactFromHandoffs(ctx, requestStored, transfer.CommunityId, logger)
		return
	}

	needs, err := storage.QueryByField[*models.PlanningNeed](s.storage, ctx, "request_id", requestID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to count needs; skipping auto-fulfill", "error", err)
		return
	}
	if len(needs) > 1 {
		logger.InfoContext(ctx, "multi-need request; handoff covers its need only", "need_count", len(needs))
		return
	}

	// Roll up every handed-off child, not just the triggering transfer — a
	// single need with multiple slots can be covered by several handoffs. The
	// trigger is seeded explicitly in case the query doesn't surface its
	// just-committed state yet.
	impact, _ := s.requestFulfillmentImpact(ctx, requestStored, []string{transfer.OwnerId}, 0, transfer.CommunityId, []*models.Transfer{transfer}, logger)

	gearName := ""
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transfer.GearId, gear); err == nil {
		gearName = gear.Name
	}
	msg := chat.RequestFulfilledByHandoffMessage(s.getUserDisplayName(ctx, transfer.OwnerId), gearName)

	if _, err := s.fulfillRequestCore(ctx, requestStored, fulfillParams{
		actorID:               requestStored.RequesterId,
		fulfilledAtUnixSec:    clock.UnixSec(ctx),
		confirmedHelperIDs:    []string{transfer.OwnerId},
		impact:                impact,
		fulfilledByTransferID: transfer.Id,
		adoptedFromTransferID: transfer.Id,
		fulfillmentMessage:    &msg,
	}); err != nil {
		logger.ErrorContext(ctx, "auto-fulfill failed; requester can fulfill manually", "error", err)
		return
	}
	logger.InfoContext(ctx, "request auto-fulfilled by gear handoff")
}

// computeManualFulfillImpact builds the impact for the requester-driven
// MarkRequestFulfilled path via requestFulfillmentImpact — the same
// computation the modal previews (#2724) — so the committed numbers match
// the previewed ones. When gear-backed offers are deliverable, their
// item-based estimate supersedes the value-based default for money,
// emissions, and time (#2702, fixes #2275's zero-value "backdoor loan").
// User-supplied overrides from the fulfillment modal win either way. The
// second return value is the transfer whose impact was adopted ("" when the
// value-based path ran).
func (s *Service) computeManualFulfillImpact(ctx context.Context, requestStored *models.Request, req *api.MarkRequestFulfilledRequest, communityID string, logger *logging.Logger) (*api.ImpactEstimate, string) {
	if s.estimatorCfg == nil {
		return &api.ImpactEstimate{}, ""
	}

	// The incoming confirmation wins over any previously stored helper set.
	confirmedIDs := req.ConfirmedHelperIds
	if len(confirmedIDs) == 0 {
		confirmedIDs = requestStored.ConfirmedHelperIds
	}
	ie, adoptedTransferID := s.requestFulfillmentImpact(ctx, requestStored, confirmedIDs, req.ConfirmedHelperCount, communityID, nil, logger)

	// Apply user-supplied overrides from the fulfillment modal when present.
	if req.QualityTimeOverrides != nil || req.MoneySavingsOverrides != nil || req.EmissionsOverrides != nil {
		overrides := &impact_metrics.ImpactOverrides{
			QualityTime:  req.QualityTimeOverrides,
			MoneySavings: req.MoneySavingsOverrides,
			Emissions:    req.EmissionsOverrides,
		}
		if applied, applyErr := impact_metrics.ApplyImpactOverrides(ie, overrides, s.estimatorCfg); applyErr == nil {
			ie = applied
		} else {
			logger.WarnContext(ctx, "failed to apply fulfillment overrides, using base estimate", "error", applyErr)
		}
	}
	return ie, adoptedTransferID
}
