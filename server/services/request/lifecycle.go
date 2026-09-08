package request

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/category"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// detachedContextWithTimeout creates a context that preserves values from parent but is independent of parent cancellation.
// This is useful for async operations that should continue after the parent request completes.
func detachedContextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	// Preserve request_id and other values from parent context but prevent cancellation propagation
	ctx := context.WithoutCancel(parent)
	// Add timeout for async operation
	return context.WithTimeout(ctx, timeout)
}

// SubmitRequest creates a new request. The request is created in its own
// per-item community (#2492). Sharing into additional existing communities goes
// through CommunityService.ShareItem (share_to_community_ids), not this RPC.
func (s *Service) SubmitRequest(
	ctx context.Context,
	req *connect.Request[api.SubmitRequestRequest],
) (*connect.Response[api.SubmitRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.Info("creating request")

	// Create the request loose — the community junction + conversation are created
	// when it's shared into its per-item community below.
	request := &models.Request{
		RequesterId:      authInfo.UserID,
		Title:            req.Msg.Title,
		Description:      req.Msg.Description,
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		MediaIds:         req.Msg.MediaIds, // May be empty; will be populated by stock imagery later
		LocationId:       req.Msg.LocationId,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}

	// Optional needed-by date — places the request on the Home calendar/Up-next.
	if req.Msg.NeededByUnixSec != nil && *req.Msg.NeededByUnixSec > 0 {
		request.NeededByUnixSec = req.Msg.NeededByUnixSec
	}

	// Extract and store metadata from Gen response (value estimate, category).
	var valueUSD float32
	if req.Msg.Metadata != nil {
		valueEstimate, cat := convertAPIRequestMetadataToStorage(req.Msg.Metadata)
		request.ValueEstimate = valueEstimate
		request.Category = cat

		if valueEstimate != nil {
			valueUSD = valueEstimate.EstimatedValueUsd
			logger.Info("storing request value estimate",
				"value_usd", valueEstimate.EstimatedValueUsd)
		}
	}

	// Fallback: the request-AI Gen path doesn't yet populate Category
	// (#2013). Derive a coarse label from title +
	// description so the `known_for` derivation can surface
	// "{Category} asker" chips. Remove once Gen sets Category itself.
	if request.Category == "" {
		request.Category = category.Categorize(request.Title, request.Description)
	}

	// Set initial ImpactEstimate before insert so it's persisted immediately.
	// Connection context is not available at submission time (no helper assigned yet), so pass nil.
	if s.estimatorCfg != nil {
		ie := impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, nil)
		request.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
	}

	requestID, err := s.storage.Insert(ctx, request)
	if err != nil {
		logger.Error("failed to insert request", "error", err)
		return nil, connecterr.Internal(ctx, "SubmitRequest", err)
	}
	request.Id = requestID

	logger.Info("created request", "target_request_id", requestID)

	// Provision the request's per-item community via the shared community library,
	// then share the request into it — creating the conversation, the
	// CommunityRequest junction, the REQUEST_CREATED anchor + description seed, and
	// the community event (#2492).
	itemCommunityID, err := community.ProvisionPerItemCommunity(ctx, s.storage, s.bus, authInfo.UserID, community.Origin{RequestID: requestID})
	if err != nil {
		logger.Error("failed to provision per-item community for request", "target_request_id", requestID, "error", err)
		return nil, connecterr.Internal(ctx, "SubmitRequest", err)
	}
	if err := s.shareRequestToCommunity(ctx, request, itemCommunityID, authInfo.UserID); err != nil {
		logger.Error("failed to share request into its per-item community", "target_request_id", requestID, "error", err)
		return nil, err
	}

	// A request is born with the claimable needs its text plainly names
	// (#2702, #2731) — the needs list is the offer surface, so handing helpers
	// something concrete to claim is worth doing for them. The generation may
	// name one thing ("Lawn mower"), several (a classroom supply list → picture
	// books, whiteboard, storage bins, …), or nothing; we seed one need per
	// named thing and leave the list empty when nothing concrete was named.
	//
	// Empty is deliberate, not a gap. The seed name used to fall back to the
	// request's own title, which is not a thing anyone brings: "Back-to-school
	// supplies for Room 7" restated the headline directly above it as a need
	// nobody would ever claim, so the request could never read as covered. A
	// request whose text names nothing concrete is better born with an empty
	// list, which is what the compose sheet ("What would help?") exists to fill.
	// Seeded directly (no chat message or planning event): the creation anchor
	// already announces the request.
	if seedNeeds := buildSeedNeeds(req.Msg.GetSeedNeedNames(), authInfo.UserID, requestID, clock.UnixSec(ctx)); len(seedNeeds) > 0 {
		// Best-effort in a single multi-row insert: a request with no seeded
		// needs is still valid (helpers can add them), so a failed seed logs
		// and continues rather than failing creation.
		if _, err := s.storage.InsertBatch(ctx, seedNeeds); err != nil {
			logger.Warn("failed to seed initial needs", "target_request_id", requestID, "seed_need_count", len(seedNeeds), "error", err)
		}
	}

	// Fetch stock image if media_ids not provided and provider is available.
	// Skip during simulation to avoid unnecessary external API calls. Async work
	// gates on the per-item community.
	if len(req.Msg.MediaIds) == 0 && s.stockImageryProvider != nil && !clock.IsSimulated(ctx) {
		// Fetch asynchronously to avoid blocking the request creation (with concurrency limit)
		go func(reqID, communityID, desc, userID string) {
			// Add panic recovery
			defer func() {
				if r := recover(); r != nil {
					asyncCtx, cancel := detachedContextWithTimeout(ctx, 5*time.Minute)
					defer cancel()
					logger := logging.LoggerWithContext(asyncCtx)
					logger.ErrorContext(asyncCtx, "panic in request image fetch goroutine",
						"panic", r,
						"target_request_id", reqID,
						"stack", string(debug.Stack()))
				}
			}()

			// Wait for shared media semaphore to limit concurrent image processing
			storage.AcquireMediaSemaphore()
			defer storage.ReleaseMediaSemaphore()

			// Create detached context with timeout preserving tracing values
			asyncCtx, cancel := detachedContextWithTimeout(ctx, 5*time.Minute)
			defer cancel()

			s.fetchAndAttachStockImage(asyncCtx, reqID, communityID, desc, userID)
		}(requestID, itemCommunityID, req.Msg.Description, authInfo.UserID)
	}

	// Generate suggestion chips asynchronously so they are ready when the
	// owner first opens the Plan tab. Best-effort: failures are logged and
	// the lazy-fill in ListRequestNeedsAndContributions acts as a safety net.
	if s.aiProvider != nil && !clock.IsSimulated(ctx) &&
		(req.Msg.Title != "" || req.Msg.Description != "") {
		go func(reqID, communityID, title, description string) {
			defer func() {
				if r := recover(); r != nil {
					asyncCtx, cancel := detachedContextWithTimeout(ctx, 5*time.Minute)
					defer cancel()
					logging.LoggerWithContext(asyncCtx).ErrorContext(asyncCtx,
						"panic in request suggestion generation goroutine",
						"panic", r,
						"target_request_id", reqID,
						"stack", string(debug.Stack()))
				}
			}()
			// Signal done for deterministic test waits (nil in production).
			defer func() {
				if s.suggestionChipsDone != nil {
					s.suggestionChipsDone <- struct{}{}
				}
			}()
			asyncCtx, cancel := detachedContextWithTimeout(ctx, 2*time.Minute)
			defer cancel()
			asyncLogger := logging.LoggerWithContext(asyncCtx).With("target_request_id", reqID)
			result, err := s.aiProvider.GenerateRequestSuggestions(asyncCtx, title, description, "")
			if err != nil {
				asyncLogger.WarnContext(asyncCtx, "async request suggestion generation failed", "error", err)
				return
			}
			if result == nil {
				return
			}
			// Cheap pre-tx early-out: if the request is already deleted, skip
			// opening a transaction at all. A concurrent soft-delete that lands
			// after this check is caught inside the tx by the locked reload.
			stored := &models.Request{}
			if err := s.storage.GetByID(asyncCtx, reqID, stored, storage.QueryOptions{IncludeDeleted: true}); err != nil {
				asyncLogger.WarnContext(asyncCtx, "failed to reload request for suggestion persist", "error", err)
				return
			}
			if stored.Deleted != nil {
				asyncLogger.DebugContext(asyncCtx, "skipping suggestion chips persist for deleted request")
				return
			}
			// Parallel soft-delete gate on the parent community. Keep before the
			// tx so we don't hold a row lock across a second storage call.
			// See #1623.
			if communityID != "" && !community.IsActive(asyncCtx, s.storage, communityID) {
				asyncLogger.InfoContext(asyncCtx, "skipping suggestion chips persist — community is soft-deleted",
					"community_id", communityID,
					"reason", "community_deleted_during_job")
				return
			}
			// Block here for deterministic race testing: test receives to release the goroutine
			// into WithTx, allowing it to call CancelRequest/UpdateRequest first. Nil in production.
			if s.suggestionChipsBeforeWrite != nil {
				s.suggestionChipsBeforeWrite <- struct{}{}
			}
			// Reload under a row lock and merge only the fields we own atomically.
			// The SELECT FOR UPDATE blocks any concurrent CancelRequest/UpdateRequest
			// from committing until this tx commits, so the merged write preserves
			// any concurrent state change. See WithTx godoc → "Locked reload" and #2900.
			if err := s.storage.WithTx(asyncCtx, nil, func(tx *storage.ProtoSQLStorage) error {
				fresh := &models.Request{}
				if err := tx.GetByID(asyncCtx, reqID, fresh, storage.QueryOptions{IncludeDeleted: true, ForUpdate: true}); err != nil {
					return err
				}
				if fresh.Deleted != nil {
					asyncLogger.InfoContext(asyncCtx, "skipping suggestion chips persist — request was soft-deleted",
						"reason", "request_deleted_during_job")
					return nil
				}
				fresh.AdditionalAsks = result.AdditionalAsks
				fresh.BreakdownPieces = result.BreakdownPieces
				fresh.OfferIdeas = result.OfferIdeas
				return tx.Update(asyncCtx, fresh)
			}); err != nil {
				asyncLogger.WarnContext(asyncCtx, "failed to persist async request suggestions", "error", err)
			} else {
				asyncLogger.DebugContext(asyncCtx, "persisted async request suggestions",
					"additional_asks", len(result.AdditionalAsks),
					"breakdown_pieces", len(result.BreakdownPieces),
					"offer_ideas", len(result.OfferIdeas))
			}
		}(requestID, itemCommunityID, req.Msg.Title, req.Msg.Description)
	}

	// Requester watches the request for inbox tracking.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestID); err != nil {
		logger.Warn("failed to create watch for request owner", "error", err)
	}

	return connect.NewResponse(&api.SubmitRequestResponse{
		RequestId:       requestID,
		ItemCommunityId: &itemCommunityID,
	}), nil
}

// MarkRequestFulfilled marks a request as fulfilled.
// The requester calls this when their need has been satisfied.
func (s *Service) MarkRequestFulfilled(
	ctx context.Context,
	req *connect.Request[api.MarkRequestFulfilledRequest],
) (*connect.Response[api.MarkRequestFulfilledResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.Info("marking request as fulfilled")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Verify caller is the requester
	if requestStored.RequesterId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_fulfill", "only the requester can mark this request as fulfilled", nil)
	}

	// Verify request is in ACTIVE or OFFERS_RECEIVED state
	if requestStored.State != models.RequestState_REQUEST_STATE_ACTIVE && requestStored.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_invalid_state_for_fulfill", "can only fulfill active or offered requests", nil)
	}

	// Resolve the community context for connection-context resolution.
	communityRequests, err := storage.QueryByFields[*models.CommunityRequest](s.storage, ctx, map[string]any{
		"request_id": req.Msg.RequestId,
		"archived":   false,
	})
	if err != nil {
		logger.Warn("failed to query community requests for impact context", "error", err)
	}
	var communityID string
	if len(communityRequests) > 0 {
		communityID = communityRequests[0].CommunityId
	}

	// Compute the fulfillment impact BEFORE the state flip so the core
	// persists it atomically with the FULFILLED transition.
	if len(req.Msg.ConfirmedHelperIds) > 0 {
		logger.Info("saving confirmed helpers", "confirmed_helper_count", len(req.Msg.ConfirmedHelperIds))
	}
	ie, adoptedTransferID := s.computeManualFulfillImpact(ctx, requestStored, req.Msg, communityID, logger)

	var fulfilledAt int64
	if req.Msg.FulfilledAtUnixSec != nil {
		fulfilledAt = *req.Msg.FulfilledAtUnixSec
	} else {
		fulfilledAt = clock.UnixSec(ctx)
	}

	var fulfillmentMessage *chat.LocalizedMessage
	if s.systemMessageWriter != nil {
		msg := chat.RequestFulfilledMessage(s.getUserDisplayName(ctx, authInfo.UserID))
		fulfillmentMessage = &msg
	}

	// Story generation is owned by the story_subscriber on the
	// community-event bus (#510 PR 4) — the REQUEST_FULFILLED events the
	// core emits flow through the bus and trigger one story per community.
	primaryFulfillmentEventID, err := s.fulfillRequestCore(ctx, requestStored, fulfillParams{
		actorID:               authInfo.UserID,
		fulfilledAtUnixSec:    fulfilledAt,
		resolutionSummary:     req.Msg.ResolutionSummary,
		confirmedHelperIDs:    req.Msg.ConfirmedHelperIds,
		impact:                ie,
		adoptedFromTransferID: adoptedTransferID,
		fulfillmentMessage:    fulfillmentMessage,
	})
	if err != nil {
		logger.Error("failed to fulfill request", "error", err)
		return nil, connecterr.Internal(ctx, "MarkRequestFulfilled", err)
	}

	logger.Info("marked request as fulfilled")

	// Build and return the updated request (use first community's conversation)
	requestItem, err := s.buildRequest(ctx, requestStored, "")
	if err != nil {
		logger.Error("failed to build request response", "error", err)
		return nil, connecterr.Internal(ctx, "MarkRequestFulfilled", err)
	}

	return connect.NewResponse(&api.MarkRequestFulfilledResponse{
		Request:          requestItem,
		Impact:           ie,
		CommunityEventId: primaryFulfillmentEventID,
	}), nil
}

// CancelRequest cancels an active request.
func (s *Service) CancelRequest(
	ctx context.Context,
	req *connect.Request[api.CancelRequestRequest],
) (*connect.Response[api.CancelRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.Info("cancelling request")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Verify caller is the requester
	if requestStored.RequesterId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_cancel", "only the requester can cancel this request", nil)
	}

	// Can't cancel if already fulfilled or cancelled
	if requestStored.State == models.RequestState_REQUEST_STATE_FULFILLED || requestStored.State == models.RequestState_REQUEST_STATE_CANCELLED {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("request is already %s", requestStored.State))
	}

	// Capture pre-cancel state for UndoCancelRequest.
	priorCancelState := requestStored.State

	// Update request state to CANCELLED
	requestStored.State = models.RequestState_REQUEST_STATE_CANCELLED
	err = s.storage.Update(ctx, requestStored)
	if err != nil {
		logger.Error("failed to update request", "error", err)
		return nil, connecterr.Internal(ctx, "CancelRequest", err)
	}

	cancelUndoData := &models.UndoData{
		Variant: &models.UndoData_RequestCancel{
			RequestCancel: &models.RequestCancelUndo{
				PriorState: priorCancelState,
			},
		},
	}

	// Get all CommunityRequest records to send events and messages
	communityRequests, err := storage.QueryByFields[*models.CommunityRequest](s.storage, ctx, map[string]any{
		"request_id": req.Msg.RequestId,
		"archived":   false,
	})
	var primaryCancelEventID string
	if err != nil {
		logger.Warn("failed to query community requests", "error", err)
	} else {
		for _, communityRequest := range communityRequests {
			// Record community event for each community
			eventID, err := s.recordCommunityEventForRequest(ctx, req.Msg.RequestId, models.RequestState_REQUEST_STATE_CANCELLED, authInfo.UserID, communityRequest.CommunityId, "", cancelUndoData)
			if err != nil {
				logger.Warn("failed to record community event", "community_id", communityRequest.CommunityId, "error", err)
			} else if primaryCancelEventID == "" {
				primaryCancelEventID = eventID
			}

			// Send system message to the request's conversation.
			if s.systemMessageWriter != nil {
				convID := requestStored.ConversationId
				if convID != "" {
					displayName := s.getUserDisplayName(ctx, authInfo.UserID)
					if err := s.systemMessageWriter.InsertLocalized(ctx, convID, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, chat.RequestCancelledMessage(displayName)); err != nil {
						logger.Warn("failed to write CANCELLED system message", "community_id", communityRequest.CommunityId, "error", err)
					}
				}
			}

			// Archive the CommunityRequest so it no longer appears in feed
			communityRequest.Archived = true
			if err := s.storage.Update(ctx, communityRequest); err != nil {
				logger.Warn("failed to archive community request", "community_request_id", communityRequest.Id, "error", err)
			}
		}
	}

	// Dismiss all watches — request reached terminal state.
	wsc := storage.NewWatchStorage(s.storage)
	if err := wsc.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestStored.Id); err != nil {
		logger.Warn("failed to dismiss watches on request cancellation", "error", err)
	}

	logger.Info("cancelled request")

	return connect.NewResponse(&api.CancelRequestResponse{
		CommunityEventId: primaryCancelEventID,
	}), nil
}

// UpdateRequest updates the description of an active request.
func (s *Service) UpdateRequest(
	ctx context.Context,
	req *connect.Request[api.UpdateRequestRequest],
) (*connect.Response[api.UpdateRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.Info("updating request")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Verify caller is the requester — with a carve-out for non-requesters
	// appending media. Any member of a community the request is shared
	// with may add media (e.g. supporter photos uploaded from the media
	// carousel), as long as they don't try to change any other field and
	// the new media list is a superset of the existing one. All other
	// update paths remain requester-only.
	if requestStored.RequesterId != authInfo.UserID {
		if !isAppendOnlyRequestMediaUpdate(req.Msg, requestStored) {
			return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_update", "only the requester can update this request", nil)
		}
		if mayAddErr := s.callerMayAddRequestMedia(ctx, req.Msg.RequestId, authInfo.UserID); mayAddErr != nil {
			return nil, mayAddErr
		}
	}

	// Can only update if in ACTIVE or OFFERS_RECEIVED state
	if requestStored.State != models.RequestState_REQUEST_STATE_ACTIVE && requestStored.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_invalid_state_for_update", "can only update active requests or requests with offers", nil)
	}

	// Update fields (only update if provided in request)
	if req.Msg.Title != "" {
		requestStored.Title = req.Msg.Title
	}
	if req.Msg.Description != "" {
		requestStored.Description = req.Msg.Description
	}
	if len(req.Msg.MediaIds) > 0 {
		requestStored.MediaIds = req.Msg.MediaIds
	}
	if req.Msg.LocationId != "" {
		requestStored.LocationId = req.Msg.LocationId
	}
	// Needed-by date: a positive value schedules the request; an explicit zero
	// clears it. A nil/unset field leaves the existing value untouched.
	if req.Msg.NeededByUnixSec != nil {
		if *req.Msg.NeededByUnixSec > 0 {
			requestStored.NeededByUnixSec = req.Msg.NeededByUnixSec
		} else {
			requestStored.NeededByUnixSec = nil
		}
	}

	// Store owner-provided social context if present, then recalculate impact metrics
	// so the QT estimate reflects the updated attributes immediately.
	if req.Msg.SocialContext != nil {
		requestStored.SocialContext = convertAPISocialContextToModels(req.Msg.SocialContext)

		if s.estimatorCfg != nil {
			var valueUSD float32
			if requestStored.ValueEstimate != nil {
				valueUSD = requestStored.ValueEstimate.EstimatedValueUsd
			}
			hint := &impact_metrics.QualityTimeHint{
				SocialContext: req.Msg.SocialContext,
			}
			// Preserve LLM-inferred vulnerability if no explicit override is set.
			if req.Msg.SocialContext.Vulnerability == api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED &&
				requestStored.ImpactEstimate != nil {
				storedAPI := impact_metrics.ModelsImpactToAPI(requestStored.ImpactEstimate)
				if storedAPI.QualityTime != nil && storedAPI.QualityTime.Attributes != nil {
					hint.VulnerabilityLevel = vulnerabilityLevelToString(storedAPI.QualityTime.Attributes.Vulnerability)
				}
			}
			requestStored.ImpactEstimate = impact_metrics.APIImpactToModels(
				impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, hint),
			)
		}
	}

	err = s.storage.Update(ctx, requestStored)
	if err != nil {
		logger.Error("failed to update request", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateRequest", err)
	}

	logger.Info("updated request")

	return connect.NewResponse(&api.UpdateRequestResponse{}), nil
}

// DeleteRequest permanently deletes a request.
// This is a soft delete - the request remains in the database but is marked as deleted.
// The associated conversation is also soft-deleted to remove it from inbox listings.
func (s *Service) DeleteRequest(
	ctx context.Context,
	req *connect.Request[api.DeleteRequestRequest],
) (*connect.Response[api.DeleteRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteRequest",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.InfoContext(ctx, "deleting request")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Verify caller is the requester
	if requestStored.RequesterId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_delete", "only the requester can delete this request", nil)
	}

	// Set the deleted metadata
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: clock.UnixSec(ctx),
	}
	requestStored.Deleted = deletedMetadata

	err = s.storage.Update(ctx, requestStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update request with deletion", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteRequest", err)
	}

	// Cascade deletion of the request's conversation.
	if requestStored.ConversationId != "" {
		if err := storage.CascadeDeleteConversationByID(ctx, s.storage, requestStored.ConversationId, deletedMetadata); err != nil {
			logger.WarnContext(ctx, "failed to delete conversation", "conversation_id", requestStored.ConversationId, "error", err)
		}
	}

	// Cascade deletion to community_request join rows so the deleted request
	// stops surfacing in community feeds and member inboxes.
	if err := storage.CascadeDeleteCommunityRequestByRequestID(ctx, s.storage, req.Msg.RequestId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteRequest", err)
	}

	// Cascade deletion to share links that point at this request.
	if err := storage.CascadeDeleteShareLinksByRequestID(ctx, s.storage, req.Msg.RequestId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteRequest", err)
	}

	// Cascade deletion to associated media
	if err := storage.CascadeDeleteMedia(ctx, s.storage, requestStored.MediaIds, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteRequest", err, "detail", "failed to cascade delete media")
	}

	logger.InfoContext(ctx, "deleted request successfully")

	return connect.NewResponse(&api.DeleteRequestResponse{}), nil
}

// generateStoryForFulfilledRequest moved to
// server/story/subscriber/request.go in #510 PR 4. The REQUEST_FULFILLED
// events emitted by MarkRequestFulfilled flow through the community-event
// bus and trigger one story per community via the subscriber.
