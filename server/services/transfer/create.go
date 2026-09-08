package transfer

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// CreateTransfer creates a transfer that has already happened (past-tense, single-step creation).
//
// When completed_at_unix_sec is set and in the past, the server skips the normal
// interest/selection state machine and creates the transfer directly in the target
// completed state. For giveaways this is COMPLETED; for loans it is ACTIVE (loan
// ongoing) or COMPLETED (loan returned) depending on whether returned_at_unix_sec is
// also provided.
func (s *Service) CreateTransfer(
	ctx context.Context,
	req *connect.Request[api.CreateTransferRequest],
) (*connect.Response[api.CreateTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.GearId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("gear_id is required"))
	}
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if req.Msg.TransferType == api.TransferType_TRANSFER_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transfer_type is required"))
	}
	hasRecipient := req.Msg.RecipientUserId != nil && *req.Msg.RecipientUserId != ""
	hasProvisional := req.Msg.ProvisionalUserId != nil && *req.Msg.ProvisionalUserId != ""
	if hasRecipient == hasProvisional {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("exactly one of recipient_user_id or provisional_user_id must be set"))
	}
	if req.Msg.CompletedAtUnixSec == nil || *req.Msg.CompletedAtUnixSec == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("completed_at_unix_sec is required for past-tense transfer creation"))
	}
	if *req.Msg.CompletedAtUnixSec > clock.UnixSec(ctx) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("completed_at_unix_sec must be in the past"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateTransfer",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
		"transfer_type", req.Msg.TransferType.String(),
	)

	// Load and validate gear.
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, req.Msg.GearId, gear); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}
	if gear.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_past_transfer", "only the gear owner can create a past transfer", nil)
	}

	// Validate caller is a member of an active (non-deleted) community.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Validate real user recipient is in the community (skip for provisional users — they're community-scoped by definition).
	// Community-active state was already validated by the caller check above; only membership is needed here.
	if hasRecipient {
		isMember, err := auth.IsMemberOfCommunity(ctx, s.storage, req.Msg.CommunityId, *req.Msg.RecipientUserId)
		if err != nil {
			return nil, connecterr.Internal(ctx, "CreatePastTransfer", err,
				"community_id", req.Msg.CommunityId, "recipient_user_id", *req.Msg.RecipientUserId)
		}
		if !isMember {
			return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_recipient_must_be_member", "the recipient must be a community member", nil)
		}
	}

	// Resolve transfer type from API to storage enum.
	var storageType models.TransferType
	switch req.Msg.TransferType {
	case api.TransferType_TRANSFER_TYPE_LOAN:
		storageType = models.TransferType_TRANSFER_TYPE_LOAN
	case api.TransferType_TRANSFER_TYPE_GIVEAWAY:
		storageType = models.TransferType_TRANSFER_TYPE_GIVEAWAY
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported transfer_type: %s", req.Msg.TransferType))
	}

	// Determine final state and timestamps.
	completedAt := *req.Msg.CompletedAtUnixSec
	var targetState models.TransferState
	var actualPickup, actualReturn *int64

	switch storageType {
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		// Giveaway: goes straight to COMPLETED.
		targetState = models.TransferState_TRANSFER_STATE_COMPLETED

	case models.TransferType_TRANSFER_TYPE_LOAN:
		// Loan: use completed_at as the pickup time.
		actualPickup = &completedAt
		if req.Msg.ReturnedAtUnixSec != nil && *req.Msg.ReturnedAtUnixSec > 0 {
			if *req.Msg.ReturnedAtUnixSec < completedAt {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("returned_at_unix_sec must be after completed_at_unix_sec"))
			}
			ret := *req.Msg.ReturnedAtUnixSec
			actualReturn = &ret
			targetState = models.TransferState_TRANSFER_STATE_COMPLETED
		} else {
			// Loan started but not yet returned.
			targetState = models.TransferState_TRANSFER_STATE_ACTIVE
		}
	}

	// Build the transfer record.
	recipientID := ""
	if hasRecipient {
		recipientID = *req.Msg.RecipientUserId
	}
	var provisionalRecipientID *string
	if hasProvisional {
		provisionalRecipientID = req.Msg.ProvisionalUserId
	}

	now := clock.UnixSec(ctx)
	transfer := &models.Transfer{
		GearId:                 req.Msg.GearId,
		OwnerId:                authInfo.UserID,
		RecipientId:            recipientID,
		ProvisionalRecipientId: provisionalRecipientID,
		TransferType:           storageType,
		State:                  targetState,
		CommunityId:            req.Msg.CommunityId,
		LatestRequestUnixSec:   now,
		ActualPickupUnixSec:    actualPickup,
		ActualReturnUnixSec:    actualReturn,
	}

	transferID, err := s.storage.Insert(ctx, transfer)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert transfer", "error", err)
		return nil, connecterr.Internal(ctx, "CreateTransfer", err)
	}
	transfer.Id = transferID

	// Owner watches the gear for inbox tracking.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, req.Msg.GearId); err != nil {
		logger.WarnContext(ctx, "failed to create watch for transfer owner", "error", err)
	}

	// Update gear state based on transfer outcome.
	switch storageType {
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		// Giveaway permanently transfers ownership — gear becomes unavailable.
		gear.State = models.GearState_GEAR_STATE_UNAVAILABLE
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.WarnContext(ctx, "failed to update gear state after past giveaway", "error", err)
		}
	case models.TransferType_TRANSFER_TYPE_LOAN:
		if targetState == models.TransferState_TRANSFER_STATE_ACTIVE {
			// Loan is currently ongoing — gear is unavailable.
			gear.State = models.GearState_GEAR_STATE_UNAVAILABLE
			if err := s.storage.Update(ctx, gear); err != nil {
				logger.WarnContext(ctx, "failed to update gear state for active past loan", "error", err)
			}
		}
		// Returned loan: gear remains available (no state change needed).
	}

	// Build impact estimate for completed transfers.
	var ie *api.ImpactEstimate
	if s.estimatorCfg != nil && targetState == models.TransferState_TRANSFER_STATE_COMPLETED {
		connCtx := s.resolveConnectionContext(ctx, transfer.OwnerId, recipientID, req.Msg.CommunityId, logger)
		ie = impact_metrics.BuildTransferImpactMetrics(gear, storageType, s.estimatorCfg, connCtx, nil)
		transfer.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
		if err := s.storage.Update(ctx, transfer); err != nil {
			logger.WarnContext(ctx, "failed to persist impact estimate on transfer", "error", err)
		}
	} else {
		ie = &api.ImpactEstimate{}
	}

	// Record community event for the completed state.
	if _, err := s.recordCommunityEventForTransfer(ctx, transferID, gear.Id, targetState, storageType, authInfo.UserID, recipientID, req.Msg.CommunityId, "", nil); err != nil {
		logger.WarnContext(ctx, "failed to record community event for past transfer", "error", err)
	}

	// Story generation for completed past transfers flows through the
	// community-event bus subscriber (#510 PR 4). The TRANSFER_COMPLETED
	// event emitted above by transitionTransferState triggers the
	// subscriber when targetState is COMPLETED.

	logger.InfoContext(ctx, "created past transfer",
		"transfer_id", transferID,
		"state", targetState.String(),
		"completed_at", time.Unix(completedAt, 0).UTC().Format(time.RFC3339),
	)

	// Build API response.
	apiTransfer, err := s.buildTransfer(ctx, transfer)
	if err != nil {
		logger.WarnContext(ctx, "failed to build transfer response, returning minimal response", "error", err)
		apiTransfer = &api.Transfer{Id: transferID}
	}

	var impactPtr *api.ImpactEstimate
	if targetState == models.TransferState_TRANSFER_STATE_COMPLETED {
		impactPtr = ie
	}

	return connect.NewResponse(&api.CreateTransferResponse{
		Transfer: apiTransfer,
		Impact:   impactPtr,
	}), nil
}
