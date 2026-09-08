package transfer

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// GetGearTransferContext retrieves transfer context for a gear item.
// This includes the user's transfer (if any), available actions, and pending requests for all users.
func (s *Service) GetGearTransferContext(
	ctx context.Context,
	req *connect.Request[api.GetGearTransferContextRequest],
) (*connect.Response[api.GetGearTransferContextResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetGearTransferContext",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
	)

	logger.DebugContext(ctx, "getting gear transfer context")

	// Validate inputs
	if req.Msg.GearId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("gear_id is required"))
	}
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Get gear to find owner. Include soft-deleted gear so a non-owner with a
	// dangling Transfer row can still view their (now CANCELLED) transfer
	// state after the owner deletes the gear (#1701). Transfer rows are
	// preserved as audit trail and are the entire point of this RPC; if the
	// gear itself is soft-deleted, the context still describes the
	// participant's prior involvement.
	gear := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.GearId, gear, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.ErrorContext(ctx, "failed to get gear", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connecterr.Internal(ctx, "GetGearTransferContext", err)
	}
	if gear.Deleted != nil {
		logger.WarnContext(
			ctx, "gear is soft-deleted; serving transfer context from surviving rows",
			"reason", "soft_deleted",
			"deleted_at_unix_sec", gear.Deleted.DeletedAtUnixSec,
		)
	}
	ownerID := gear.OwnerId

	// Build transfer context
	transferContext, err := s.buildGearTransferContext(ctx, req.Msg.GearId, authInfo.UserID, ownerID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build gear transfer context", "error", err)
		return nil, connecterr.Internal(ctx, "GetGearTransferContext", err)
	}

	logger.InfoContext(
		ctx, "retrieved gear transfer context",
		"has_user_transfer", transferContext.UserTransfer != nil,
		"pending_requests_count", len(transferContext.PendingRequests),
		"available_actions_count", len(transferContext.AvailableActions),
	)

	return connect.NewResponse(&api.GetGearTransferContextResponse{
		Context: transferContext,
	}), nil
}

// buildGearTransferContext builds the transfer context for a gear item.
func (s *Service) buildGearTransferContext(ctx context.Context, gearID, userID, ownerID string) (*api.GearTransferContext, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "buildGearTransferContext",
		"gear_id", gearID,
		"user_id", userID,
		"owner_id", ownerID,
	)

	transferContext := &api.GearTransferContext{}
	isOwner := userID == ownerID

	// Find user's active transfer (if any)
	// For non-owners: look for transfers where user is recipient
	// For owners: look for transfers in actionable states (RECIPIENT_SELECTED or ACTIVE)
	if !isOwner {
		transfers, err := s.storage.QueryByFields(ctx, map[string]any{
			"gear_id":      gearID,
			"recipient_id": userID,
		}, &models.Transfer{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query user transfers", "error", err)
			return nil, fmt.Errorf("failed to query user transfers: %w", err)
		}
		if len(transfers) > 0 {
			logger.DebugContext(ctx, "found user transfers for gear", "transfer_count", len(transfers))
			// Find the most recent non-terminal transfer
			for _, t := range transfers {
				transfer := t.(*models.Transfer)
				logger.DebugContext(
					ctx, "checking transfer",
					"transfer_id", transfer.Id,
					"state", transfer.State.String(),
				)
				// Only include non-terminal transfers
				if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED &&
					transfer.State != models.TransferState_TRANSFER_STATE_CANCELLED {
					// Convert to API transfer
					apiTransfer := s.convertTransferToAPI(transfer)
					transferContext.UserTransfer = apiTransfer
					logger.DebugContext(ctx, "selected non-terminal transfer", "transfer_id", transfer.Id)
					break
				}
			}
		}
	} else {
		// For owners, find the most relevant transfer by priority:
		// RECIPIENT_SELECTED/ACTIVE for in-progress workflows, COMPLETED for
		// post-completion display (e.g. giveaway recipient in View Impact menu).
		// Use full API conversion to include recipient user info and timestamps.
		for _, state := range []models.TransferState{
			models.TransferState_TRANSFER_STATE_ACTIVE,
			models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			models.TransferState_TRANSFER_STATE_COMPLETED,
		} {
			transfers, err := s.storage.QueryByFields(ctx, map[string]any{
				"gear_id": gearID,
				"state":   int32(state),
			}, &models.Transfer{})
			if err != nil {
				logger.ErrorContext(ctx, "failed to query owner transfers", "error", err)
				return nil, fmt.Errorf("failed to query owner transfers: %w", err)
			}
			if len(transfers) > 0 {
				transfer := transfers[0].(*models.Transfer)
				apiTransfer, err := s.convertTransferToFullAPI(ctx, transfer)
				if err != nil {
					logger.ErrorContext(ctx, "failed to convert owner transfer", "error", err)
					return nil, err
				}
				transferContext.UserTransfer = apiTransfer
				logger.DebugContext(
					ctx, "found owner actionable transfer",
					"transfer_id", transfer.Id,
					"state", transfer.State.String(),
					"type", transfer.TransferType.String(),
				)
				break
			}
		}
	}

	// Get pending requests (visible to all users per Decision 1)
	// Include both INTEREST_EXPRESSED and RECIPIENT_SELECTED states so all users
	// can see selected transfers (important for giveaway "SELECTED" badges)
	pendingTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query pending transfers", "error", err)
		return nil, fmt.Errorf("failed to query pending transfers: %w", err)
	}

	for _, t := range pendingTransfers {
		transfer := t.(*models.Transfer)

		// Fetch borrower user info
		borrower, err := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch borrower", "recipient_id", transfer.RecipientId, "error", err)
			return nil, fmt.Errorf("failed to fetch borrower %s: %w", transfer.RecipientId, err)
		}

		transferContext.PendingRequests = append(transferContext.PendingRequests, transferRequestFromModel(transfer, borrower))
	}

	// Also include RECIPIENT_SELECTED and ACTIVE transfers so all users can see them
	// (important for showing "APPROVED" badge and "Currently Borrowing" status)
	for _, state := range []models.TransferState{
		models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		models.TransferState_TRANSFER_STATE_ACTIVE,
	} {
		stateTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
			"gear_id": gearID,
			"state":   int32(state),
		}, &models.Transfer{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query transfers by state", "state", state.String(), "error", err)
			return nil, fmt.Errorf("failed to query transfers by state %s: %w", state.String(), err)
		}

		for _, t := range stateTransfers {
			transfer := t.(*models.Transfer)

			// An ACTIVE loan is the current holder — already surfaced to
			// clients as gear.active_loan — not a pending request. Listing it
			// here too rendered the borrower twice in "Who's using it" (#2638).
			// ACTIVE giveaways stay: their roster is driven by these entries.
			if state == models.TransferState_TRANSFER_STATE_ACTIVE &&
				transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
				continue
			}

			// Fetch borrower user info
			borrower, err := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
			if err != nil {
				logger.ErrorContext(ctx, "failed to fetch borrower", "recipient_id", transfer.RecipientId, "error", err)
				return nil, fmt.Errorf("failed to fetch borrower %s: %w", transfer.RecipientId, err)
			}

			transferContext.PendingRequests = append(transferContext.PendingRequests, transferRequestFromModel(transfer, borrower))
		}
	}

	// Set SelectedRecipient for giveaways. If the userTransfer has a recipient
	// (RECIPIENT_SELECTED, ACTIVE, or COMPLETED), expose it so the client can
	// display the recipient's name and avatar in the manage menu.
	selectedCount := 0
	if transferContext.UserTransfer != nil && transferContext.UserTransfer.Recipient != nil {
		transferContext.SelectedRecipient = &api.TransferRequest{
			TransferId: transferContext.UserTransfer.Id,
			Borrower:   transferContext.UserTransfer.Recipient,
		}
		if transferContext.UserTransfer.State == api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED ||
			transferContext.UserTransfer.State == api.TransferState_TRANSFER_STATE_ACTIVE {
			selectedCount = 1
		}
	}

	// Count completed and cancelled transfers for giveaway phase computation.
	completedTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query completed transfers", "error", err)
		return nil, fmt.Errorf("failed to query completed transfers: %w", err)
	}
	cancelledTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_CANCELLED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query cancelled transfers", "error", err)
		return nil, fmt.Errorf("failed to query cancelled transfers: %w", err)
	}

	transferContext.OverallPhase = computeGiveawayPhase(
		len(pendingTransfers), selectedCount, len(completedTransfers), len(cancelledTransfers),
	)

	// For a completed giveaway, expose the recipient to *every* viewer (not just
	// the recipient/owner via their own user_transfer) so any reader of the
	// "who wants it" card can see who it went to.
	if transferContext.SelectedRecipient == nil {
		for _, row := range completedTransfers {
			t := row.(*models.Transfer)
			if t.TransferType != models.TransferType_TRANSFER_TYPE_GIVEAWAY || t.RecipientId == "" {
				continue
			}
			recipient, err := services.FetchAPIUser(ctx, s.storage, t.RecipientId)
			if err == nil && recipient != nil {
				transferContext.SelectedRecipient = &api.TransferRequest{
					TransferId: t.Id,
					Borrower:   recipient,
				}
			}
			break
		}
	}

	// Determine available actions based on state and role.
	transferContext.AvailableActions = s.determineTransferActions(ctx, isOwner, transferContext.UserTransfer, len(transferContext.PendingRequests) > 0)

	// Set ownership flag for client UI.
	transferContext.IsOwner = isOwner

	logger.DebugContext(
		ctx, "built gear transfer context",
		"is_owner", isOwner,
		"has_user_transfer", transferContext.UserTransfer != nil,
		"pending_requests_count", len(transferContext.PendingRequests),
		"available_actions", transferContext.AvailableActions,
		"overall_phase", transferContext.OverallPhase.String(),
	)

	return transferContext, nil
}

// computeGiveawayPhase determines the overall giveaway phase from transfer counts.
func computeGiveawayPhase(pendingCount, selectedCount, completedCount, cancelledCount int) api.GiveawayPhase {
	if completedCount > 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_COMPLETED
	}
	if selectedCount > 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_RECIPIENT_SELECTED
	}
	if cancelledCount > 0 && pendingCount == 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_CANCELLED
	}
	return api.GiveawayPhase_GIVEAWAY_PHASE_OPEN
}

// determineTransferActions determines which workflow actions are available to the user.
func (s *Service) determineTransferActions(_ context.Context, isOwner bool, userTransfer *api.Transfer, hasPendingRequests bool) []api.TransferAction {
	var actions []api.TransferAction

	if isOwner {
		// Owner actions based on transfer state
		if hasPendingRequests {
			// Has pending requests - can approve/decline
			actions = append(actions, api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT)
		}

		if userTransfer != nil {
			switch userTransfer.State {
			case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
				// Giveaways skip ACTIVE state and go directly to COMPLETED
				// Loans need to be started first (transition to ACTIVE)
				if userTransfer.TransferType == api.TransferType_TRANSFER_TYPE_GIVEAWAY {
					actions = append(actions, api.TransferAction_TRANSFER_ACTION_COMPLETE)
				} else {
					actions = append(actions, api.TransferAction_TRANSFER_ACTION_START_LOAN)
				}
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_ACTIVE:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_COMPLETE)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			}
		}
	} else {
		// Borrower actions based on transfer state
		if userTransfer == nil {
			// No transfer - can express interest
			actions = append(actions, api.TransferAction_TRANSFER_ACTION_EXPRESS_INTEREST)
		} else {
			switch userTransfer.State {
			case api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_WITHDRAW_INTEREST)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_ACTIVE:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_COMPLETE)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			}
		}
	}

	return actions
}

// convertTransferToAPI converts a models.Transfer to api.Transfer.
// This creates a minimal transfer without Recipient populated. Use
// convertTransferToFullAPI when the recipient User is needed.
func (s *Service) convertTransferToAPI(transfer *models.Transfer) *api.Transfer {
	return s.buildAPITransfer(transfer, nil)
}

// convertTransferToFullAPI converts a models.Transfer to api.Transfer with
// the Recipient field populated by fetching the user.
func (s *Service) convertTransferToFullAPI(ctx context.Context, transfer *models.Transfer) (*api.Transfer, error) {
	var recipient *api.User
	if transfer.RecipientId != "" {
		var err error
		recipient, err = services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch recipient %s: %w", transfer.RecipientId, err)
		}
	}
	return s.buildAPITransfer(transfer, recipient), nil
}

// buildAPITransfer builds an api.Transfer from a models.Transfer.
func (s *Service) buildAPITransfer(transfer *models.Transfer, recipient *api.User) *api.Transfer {
	transferType := api.TransferType_TRANSFER_TYPE_LOAN
	if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		transferType = api.TransferType_TRANSFER_TYPE_GIVEAWAY
	}

	state := api.TransferState_TRANSFER_STATE_UNSPECIFIED
	switch transfer.State {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		state = api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		state = api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		state = api.TransferState_TRANSFER_STATE_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		state = api.TransferState_TRANSFER_STATE_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		state = api.TransferState_TRANSFER_STATE_CANCELLED
	}

	apiTransfer := &api.Transfer{
		Id:                   transfer.Id,
		GearId:               transfer.GearId,
		TransferType:         transferType,
		State:                state,
		LatestRequestUnixSec: transfer.LatestRequestUnixSec,
		Recipient:            recipient,
	}

	if transfer.EstimatedPickupUnixSec != nil {
		apiTransfer.EstimatedPickupUnixSec = transfer.EstimatedPickupUnixSec
	}
	if transfer.ActualPickupUnixSec != nil {
		apiTransfer.ActualPickupUnixSec = transfer.ActualPickupUnixSec
	}
	if transfer.ActualReturnUnixSec != nil {
		apiTransfer.ActualReturnUnixSec = transfer.ActualReturnUnixSec
	}

	return apiTransfer
}

// transferRequestFromModel builds the pending-request view of a transfer,
// including the booking window when the transfer is a dated calendar booking
// so clients can render which days each request covers (#2638).
func transferRequestFromModel(transfer *models.Transfer, borrower *api.User) *api.TransferRequest {
	req := &api.TransferRequest{
		TransferId:         transfer.Id,
		Borrower:           borrower,
		RequestedAtUnixSec: transfer.LatestRequestUnixSec,
	}
	if transfer.EstimatedPickupUnixSec != nil {
		req.EstimatedPickupUnixSec = transfer.EstimatedPickupUnixSec
	}
	if transfer.ExpectedReturnUnixSec != nil {
		req.ExpectedReturnUnixSec = transfer.ExpectedReturnUnixSec
	}
	return req
}
