package transfer

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// transitionTransferState transitions a pre-loaded transfer to the target state with validation.
// Returns the updated transfer, the gear (if loaded during the transition), and the id of the
// CommunityEvent emitted for the transition (empty when no event was emitted — idempotent no-op).
// Callers pass the community_event_id back to clients so snackbar/story-screen undo can target
// the specific action.
//
// systemChatMessageID is optional; when non-empty it is persisted on the emitted
// CommunityEvent so Undo* RPCs can soft-delete the corresponding system chat message
// by direct read. Callers that emit a system message alongside the transition pass
// the inserted message id here (from InsertSystemMessageReturnID).
func (s *Service) transitionTransferState(
	ctx context.Context,
	transfer *models.Transfer,
	actorID string,
	targetState models.TransferState,
	systemChatMessageID string,
) (*models.Transfer, *models.Gear, string, error) {
	logger := logging.LoggerWithContext(ctx)

	// Verify actor is either sharer or recipient
	if transfer.OwnerId != actorID && transfer.RecipientId != actorID {
		return nil, nil, "", connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_owner_or_recipient_required_for_modify", "only the gear owner or recipient can modify this transfer", nil)
	}

	// Validate state transition
	if !isValidTransferTransition(transfer.State, targetState, transfer.TransferType, transfer.OwnerId, actorID, transfer.GetOriginExperienceId() != "") {
		return nil, nil, "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("invalid state transition from %s to %s", transfer.State, targetState))
	}

	// Check if already in target state (idempotent)
	if transfer.State == targetState {
		return transfer, nil, "", nil
	}

	// Save original state before modifying. Captured here so UndoData can
	// faithfully record what to revert to — all subsequent mutations
	// below overwrite these fields on the live transfer.
	originalState := transfer.State
	priorActualPickupUnixSec := transfer.ActualPickupUnixSec
	priorActualReturnUnixSec := transfer.ActualReturnUnixSec
	priorImpactEstimate := transfer.ImpactEstimate

	// Populated by completeGiveaway during the cascade and consumed
	// when building UndoData below.
	var giveawayCascadeSnapshot *models.GiveawayCascadeUndo

	// Fetch gear once for transitions that need it
	var gear *models.Gear
	needsGear := false
	switch targetState {
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		needsGear = true
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		if transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE && transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			needsGear = true
		}
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		if transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE && transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			needsGear = true
		}
	}
	if needsGear {
		gear = &models.Gear{}
		if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
			logger.ErrorContext(ctx, "failed to get gear",
				"gear_id", transfer.GearId,
				"error", err,
			)
			return nil, nil, "", connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
	}

	// Capture prior gear state for UndoData construction. Done after the
	// gear load above so the snapshot reflects the true pre-transition
	// value; markGearState calls below will overwrite gear.State in place.
	var priorGearState models.GearState
	if gear != nil {
		priorGearState = gear.State
	}

	// Handle gear state changes and record actual times
	switch targetState {
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		// Record actual pickup time
		now := clock.UnixSec(ctx)
		transfer.ActualPickupUnixSec = &now

		// Derive expected_return_unix_sec for loans with a duration, so
		// scheduled-notification reconcilers can index due-date queries
		// without recomputing actual_pickup + duration at scan time.
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN && transfer.LoanDurationDays != nil {
			expected := now + int64(*transfer.LoanDurationDays)*86400
			transfer.ExpectedReturnUnixSec = &expected
		}

		// Mark gear unavailable when transfer becomes active
		if err := s.markGearState(ctx, gear, models.GearState_GEAR_STATE_UNAVAILABLE); err != nil {
			return nil, nil, "", err
		}
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		// Record actual return time for loans completing from active state
		if transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE && transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			returnTime := clock.UnixSec(ctx)
			transfer.ActualReturnUnixSec = &returnTime
			if err := s.markGearState(ctx, gear, models.GearState_GEAR_STATE_AVAILABLE); err != nil {
				return nil, nil, "", err
			}
		}
		// For giveaways completing from RECIPIENT_SELECTED, record pickup time (handoff happened)
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY && transfer.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			pickupTime := clock.UnixSec(ctx)
			transfer.ActualPickupUnixSec = &pickupTime
		}
		// For giveaways completing from RECIPIENT_SELECTED, mark as given
		// away and archive community gear. The returned cascade snapshot
		// is stapled onto UndoData below so UndoCompleteGiveaway can
		// reverse every side effect.
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY && transfer.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			snapshot, err := s.completeGiveaway(ctx, transfer.GearId, transfer.Id)
			if err != nil {
				return nil, nil, "", err
			}
			giveawayCascadeSnapshot = snapshot
		}
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		// For loans cancelled from active state, mark gear available (returned to owner)
		if transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE && transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			if err := s.markGearState(ctx, gear, models.GearState_GEAR_STATE_AVAILABLE); err != nil {
				return nil, nil, "", err
			}
		}
		// For giveaways, archive community gear only when the owner cancels.
		// Recipient cancellation is one bid being withdrawn, not the end of the
		// giveaway; siblings stay open so the owner can re-select.
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY && actorID == transfer.OwnerId {
			if err := s.archiveCommunityGearForGiveaway(ctx, transfer.GearId, transfer.CommunityId); err != nil {
				return nil, nil, "", err
			}
		}
	}

	// Update transfer state
	transfer.State = targetState
	if err := s.storage.Update(ctx, transfer); err != nil {
		logger.ErrorContext(ctx, "failed to update transfer state",
			"transfer_id", transfer.Id,
			"target_state", targetState.String(),
			"error", err,
		)
		return nil, nil, "", connecterr.Internal(ctx, "transitionTransferState", err)
	}

	// Update watch state for the gear.
	ws := storage.NewWatchStorage(s.storage)
	if IsTerminalState(targetState) {
		// Terminal state: dismiss all watches for this gear.
		if err := ws.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, transfer.GearId); err != nil {
			logger.WarnContext(ctx, "failed to dismiss watches on terminal state", "error", err)
		}
	} else {
		// Non-terminal transition: mark all watchers unread except the actor.
		if err := ws.MarkUnreadForWatchers(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, transfer.GearId, actorID); err != nil {
			logger.WarnContext(ctx, "failed to mark watchers unread on state transition", "error", err)
		}
	}

	// Determine the object user ID (the other party) for events involving both parties.
	// For COMPLETED events, both parties are recorded.
	// For CANCELLED events:
	//   - From INTEREST_EXPRESSED: only actor (no commitment yet)
	//   - From RECIPIENT_SELECTED or later: both parties (both had committed)
	var objectUserID string
	switch targetState {
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		// Set objectUserID to the "other party"
		if actorID == transfer.OwnerId {
			objectUserID = transfer.RecipientId
		} else {
			objectUserID = transfer.OwnerId
		}
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		// Both parties share the penalty only if cancelling after approval
		// Use originalState (before we updated it) to check what state we're cancelling from
		if originalState != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			if actorID == transfer.OwnerId {
				objectUserID = transfer.RecipientId
			} else {
				objectUserID = transfer.OwnerId
			}
		}
	}

	// Build UndoData variant for server-authoritative undoable transitions.
	var undoData *models.UndoData
	switch {
	case targetState == models.TransferState_TRANSFER_STATE_ACTIVE &&
		originalState == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED &&
		transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN:
		undoData = &models.UndoData{
			Variant: &models.UndoData_LoanStart{
				LoanStart: &models.LoanStartUndo{
					PriorGearState:           priorGearState,
					PriorActualPickupUnixSec: priorActualPickupUnixSec,
				},
			},
		}
	case targetState == models.TransferState_TRANSFER_STATE_COMPLETED &&
		originalState == models.TransferState_TRANSFER_STATE_ACTIVE &&
		transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN:
		undoData = &models.UndoData{
			Variant: &models.UndoData_LoanCompletion{
				LoanCompletion: &models.LoanCompletionUndo{
					PriorGearState:           priorGearState,
					PriorActualReturnUnixSec: priorActualReturnUnixSec,
					PriorImpactEstimate:      priorImpactEstimate,
				},
			},
		}
	case targetState == models.TransferState_TRANSFER_STATE_COMPLETED &&
		originalState == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED &&
		transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY &&
		giveawayCascadeSnapshot != nil:
		giveawayCascadeSnapshot.PriorImpactEstimate = priorImpactEstimate
		undoData = &models.UndoData{
			Variant: &models.UndoData_GiveawayCascade{
				GiveawayCascade: giveawayCascadeSnapshot,
			},
		}
	case targetState == models.TransferState_TRANSFER_STATE_CANCELLED:
		cancelUndo := &models.TransferCancelUndo{
			PriorState: originalState,
		}
		// If the cancel came from ACTIVE on a loan, markGearState above
		// set gear AVAILABLE; capture the prior state (UNAVAILABLE) so
		// undo can restore it.
		if originalState == models.TransferState_TRANSFER_STATE_ACTIVE &&
			transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			cancelUndo.PriorGearState = &priorGearState
		}
		undoData = &models.UndoData{
			Variant: &models.UndoData_TransferCancel{
				TransferCancel: cancelUndo,
			},
		}
	}

	// Record community event
	eventID, err := s.recordCommunityEventForTransfer(ctx, transfer.Id, transfer.GearId, targetState, transfer.TransferType, actorID, objectUserID, transfer.CommunityId, systemChatMessageID, undoData)
	if err != nil {
		logger.ErrorContext(ctx, "failed to record community event for transfer",
			"transfer_id", transfer.Id,
			"error", err,
		)
		return nil, nil, "", fmt.Errorf("failed to record community event: %w", err)
	}

	return transfer, gear, eventID, nil
}

// isValidTransferTransition checks if a state transition is valid according to
// business rules. eventOrigin is true for a child transfer of an experience
// (#2708), which completes directly from RECIPIENT_SELECTED when the event ends
// — it never passes through ACTIVE because the lender keeps custody of a
// communally-used item, so there is no discrete handoff/return.
func isValidTransferTransition(from, to models.TransferState, transferType models.TransferType, ownerID, actorID string, eventOrigin bool) bool {
	switch from {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		if to == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			// Only owner can select recipient
			return actorID == ownerID
		}
		if to == models.TransferState_TRANSFER_STATE_CANCELLED {
			// Either party can cancel
			return true
		}
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		if to == models.TransferState_TRANSFER_STATE_ACTIVE {
			// Only for loans, either owner or recipient can mark picked up
			return transferType == models.TransferType_TRANSFER_TYPE_LOAN
		}
		if to == models.TransferState_TRANSFER_STATE_COMPLETED {
			// Giveaways complete directly (no ACTIVE state). Event-origin loans
			// also complete directly when the event ends (#2708).
			return transferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY ||
				(transferType == models.TransferType_TRANSFER_TYPE_LOAN && eventOrigin)
		}
		if to == models.TransferState_TRANSFER_STATE_CANCELLED {
			// Either party can cancel
			return true
		}
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		if to == models.TransferState_TRANSFER_STATE_COMPLETED {
			// Complete loan (either party can mark returned)
			return true
		}
		if to == models.TransferState_TRANSFER_STATE_CANCELLED {
			// Either party can cancel
			return true
		}
	case models.TransferState_TRANSFER_STATE_COMPLETED, models.TransferState_TRANSFER_STATE_CANCELLED:
		// No transitions allowed from terminal states
		return false
	}
	return false
}

// IsTerminalState returns true if the transfer state is a terminal state.
func IsTerminalState(state models.TransferState) bool {
	return state == models.TransferState_TRANSFER_STATE_COMPLETED ||
		state == models.TransferState_TRANSFER_STATE_CANCELLED
}

// markGearState updates a pre-loaded gear's state.
func (s *Service) markGearState(ctx context.Context, gear *models.Gear, targetState models.GearState) error {
	logger := logging.LoggerWithContext(ctx)

	// Check if already in target state
	if gear.State == targetState {
		return nil
	}

	// Update gear state
	gear.State = targetState
	if err := s.storage.Update(ctx, gear); err != nil {
		logger.ErrorContext(ctx, "failed to update gear state",
			"gear_id", gear.Id,
			"target_state", targetState.String(),
			"error", err,
		)
		return connecterr.Internal(ctx, "markGearState", err)
	}

	return nil
}

// completeGiveaway marks gear as given away and archives community gear relationships.
// Gear ownership stays with the original owner (not transferred to recipient).
// CommunityGear records are preserved with archived=true to maintain conversation history.
// All other pending transfers for this gear are automatically cancelled.
//
// Returns a GiveawayCascadeUndo snapshot capturing every side effect
// (gear state, cancelled siblings, archived community_gear ids) so the
// caller can persist it on the emitted CommunityEvent for later
// reversal via UndoCompleteGiveaway.
func (s *Service) completeGiveaway(ctx context.Context, gearID, completedTransferID string) (*models.GiveawayCascadeUndo, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "completeGiveaway",
		"gear_id", gearID,
		"completed_transfer_id", completedTransferID,
	)

	// Fetch the gear
	gearStored := &models.Gear{}
	err := s.storage.GetByID(ctx, gearID, gearStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	originalState := gearStored.State
	undoSnapshot := &models.GiveawayCascadeUndo{
		PriorGearState: originalState,
	}

	// Mark gear as given away (ownership stays with original owner)
	gearStored.State = models.GearState_GEAR_STATE_GIVEN_AWAY
	err = s.storage.Update(ctx, gearStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update gear state",
			"from_state", originalState.String(),
			"to_state", models.GearState_GEAR_STATE_GIVEN_AWAY.String(),
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "completeGiveaway", err)
	}

	logger.InfoContext(ctx, "gear marked as given away",
		"from_state", originalState.String(),
		"to_state", models.GearState_GEAR_STATE_GIVEN_AWAY.String(),
	)

	// Cancel all other pending transfers for this gear (except the completed one)
	allTransfers, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers for gear", "error", err)
		return nil, connecterr.Internal(ctx, "completeGiveaway", err, "detail", "failed to query transfers")
	}

	for _, msg := range allTransfers {
		transfer := msg.(*models.Transfer)
		// Skip the completed transfer and already-cancelled transfers
		if transfer.Id == completedTransferID ||
			transfer.State == models.TransferState_TRANSFER_STATE_CANCELLED ||
			transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		// Cancel pending transfers (INTEREST_EXPRESSED or RECIPIENT_SELECTED states)
		if transfer.State == models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED ||
			transfer.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			priorSiblingState := transfer.State
			transfer.State = models.TransferState_TRANSFER_STATE_CANCELLED
			err = s.storage.Update(ctx, transfer)
			if err != nil {
				logger.ErrorContext(ctx, "failed to cancel pending transfer",
					"transfer_id", transfer.Id,
					"error", err,
				)
				return nil, connecterr.Internal(ctx, "completeGiveaway", err, "detail", "failed to cancel transfer")
			}
			undoSnapshot.CancelledSiblings = append(undoSnapshot.CancelledSiblings, &models.GiveawayCascadeUndo_CancelledSiblingTransfer{
				TransferId: transfer.Id,
				PriorState: priorSiblingState,
			})
			logger.InfoContext(ctx, "cancelled pending transfer",
				"transfer_id", transfer.Id,
				"recipient_id", transfer.RecipientId,
			)
		}
	}

	// Archive all community gear relationships (preserve instead of deleting)
	communityGears, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear relationships", "error", err)
		return nil, connecterr.Internal(ctx, "completeGiveaway", err, "detail", "failed to query community gear")
	}

	for _, msg := range communityGears {
		communityGear := msg.(*models.CommunityGear)
		if communityGear.Archived {
			// Already archived (e.g., by an earlier operation) — do not
			// record in the undo snapshot, otherwise undo would flip
			// archived=false on a row we didn't touch here.
			continue
		}
		communityGear.Archived = true
		err = s.storage.Update(ctx, communityGear)
		if err != nil {
			logger.ErrorContext(ctx, "failed to archive community gear relationship",
				"community_gear_id", communityGear.Id,
				"community_id", communityGear.CommunityId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "completeGiveaway", err, "detail", "failed to archive community gear")
		}
		undoSnapshot.ArchivedCommunityGearIds = append(undoSnapshot.ArchivedCommunityGearIds, communityGear.Id)
		logger.InfoContext(ctx, "community gear archived",
			"community_gear_id", communityGear.Id,
			"community_id", communityGear.CommunityId,
		)
	}

	return undoSnapshot, nil
}

// archiveCommunityGearForGiveaway archives the community gear relationship for a cancelled giveaway.
// This ensures cancelled giveaways no longer appear in the feed for that community.
// Unlike completeGiveaway which archives ALL community relationships, this only archives
// the specific community where the giveaway was cancelled.
func (s *Service) archiveCommunityGearForGiveaway(ctx context.Context, gearID, communityID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "archiveCommunityGearForGiveaway",
		"gear_id", gearID,
		"community_id", communityID,
	)

	// Find the community gear relationship for this specific community
	communityGears, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear relationship", "error", err)
		return connecterr.Internal(ctx, "archiveCommunityGearForGiveaway", err, "detail", "failed to query community gear")
	}

	if len(communityGears) == 0 {
		// No relationship found - this is unexpected but not fatal
		logger.WarnContext(ctx, "no community gear relationship found to archive")
		return nil
	}

	communityGear := communityGears[0].(*models.CommunityGear)
	communityGear.Archived = true
	err = s.storage.Update(ctx, communityGear)
	if err != nil {
		logger.ErrorContext(ctx, "failed to archive community gear relationship", "error", err)
		return connecterr.Internal(ctx, "archiveCommunityGearForGiveaway", err, "detail", "failed to archive community gear")
	}

	logger.InfoContext(ctx, "community gear archived for cancelled giveaway")
	return nil
}
