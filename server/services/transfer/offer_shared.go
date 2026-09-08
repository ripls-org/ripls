package transfer

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// originOffer holds the resolved, validated inputs for creating an
// origin-linked child transfer — a gear-backed request offer (#2702) or a
// gear brought to an event (#2708). Each caller validates its own scope
// (recipient, entity state, community share) and hands the shared bits here;
// createOriginOfferTransfer owns everything downstream of validation so the
// two paths cannot drift.
type originOffer struct {
	gear         *models.Gear
	ownerID      string // the caller offering their gear
	recipientID  string // requester (request scope) or host (experience scope)
	communityID  string
	transferType api.TransferType
	availability models.Availability
	// Exactly one origin id is set: the request the offer fulfills (#2702) or
	// the event the gear was brought to (#2708).
	originRequestID    string
	originExperienceID string
	contribution       *models.PlanningContribution
	loanDuration       *int32
	// originConversationID is the request/experience thread that receives the
	// "gear offered" system message. May be empty (the offer still succeeds).
	originConversationID string
}

// createOriginOfferTransfer performs the shared work of birthing an
// origin-linked transfer in RECIPIENT_SELECTED: share the gear into the
// community with the offer's availability, insert the transfer, link the
// contribution back-ref, add the recipient to the gear's perpetual
// conversation, announce the moment in the origin thread, record the
// RECIPIENT_SELECTED community event (push suppressed for origin-linked
// transfers), and watch the gear for the owner. Returns the built API transfer.
func (s *Service) createOriginOfferTransfer(ctx context.Context, o originOffer, logger *logging.Logger) (*api.Transfer, error) {
	// Share the gear into the community with the offer's Lend/Give choice.
	// Idempotent; an existing share is updated to this availability (the owner
	// just expressed current intent).
	if err := s.gearSharer(ctx, o.gear.Id, o.communityID, o.ownerID, o.availability); err != nil {
		return nil, connecterr.Internal(ctx, "createOriginOfferTransfer", err, "detail", "share gear into community")
	}

	// Create the transfer, born in RECIPIENT_SELECTED with the resolved
	// recipient (requester for a request offer, host for an event).
	transfer := &models.Transfer{
		GearId:               o.gear.Id,
		OwnerId:              o.ownerID,
		RecipientId:          o.recipientID,
		TransferType:         apiTransferTypeToModel(o.transferType),
		State:                models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:          o.communityID,
		LatestRequestUnixSec: clock.UnixSec(ctx),
		LoanDurationDays:     o.loanDuration,
	}
	if o.originExperienceID != "" {
		transfer.Origin = &models.Transfer_OriginExperienceId{OriginExperienceId: o.originExperienceID}
	} else if o.originRequestID != "" {
		transfer.Origin = &models.Transfer_OriginRequestId{OriginRequestId: o.originRequestID}
	}
	transferID, err := s.storage.Insert(ctx, transfer)
	if err != nil {
		return nil, connecterr.Internal(ctx, "createOriginOfferTransfer", err, "detail", "insert transfer")
	}
	transfer.Id = transferID

	// Link the contribution to its transfer.
	o.contribution.TransferId = &transferID
	if err := s.storage.Update(ctx, o.contribution); err != nil {
		return nil, connecterr.Internal(ctx, "createOriginOfferTransfer", err, "detail", "link contribution", "transfer_id", transferID)
	}

	// The recipient joins the gear's perpetual conversation, where the loan
	// lifecycle posts its system messages. The conversation exists after the
	// share above; a missing one is unexpected but must not fail the offer.
	refreshedGear := &models.Gear{}
	if err := s.storage.GetByID(ctx, o.gear.Id, refreshedGear); err == nil && refreshedGear.ConversationId != "" {
		if err := chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(s.storage), refreshedGear.ConversationId, o.recipientID); err != nil {
			logger.WarnContext(ctx, "failed to add recipient to gear conversation",
				"conversation_id", refreshedGear.ConversationId, "error", err)
		}
	} else {
		logger.WarnContext(ctx, "gear conversation missing after share; recipient not added", "error", err)
	}

	// Announce the structural moment in the origin (request/experience) thread
	// as ONE line carrying the richest info (#2724). When the offer
	// escalates a claim on a named need, the line combines both halves
	// ("X is bringing <need> — lending <item>"); passing the contribution id
	// as the coalesce key makes it supersede the earlier claim/offer lines
	// from the same actor in place instead of stacking a third line.
	if o.originConversationID != "" {
		giveaway := o.transferType == api.TransferType_TRANSFER_TYPE_GIVEAWAY
		displayName := s.getUserDisplayName(ctx, o.ownerID)
		var msg chat.LocalizedMessage
		if needName := o.contribution.GetOriginalNeedName(); o.contribution.FromNeedId != nil && needName != "" {
			msg = chat.GearOfferedForNeedMessage(displayName, needName, o.gear.Name, giveaway)
		} else {
			msg = chat.GearOfferedMessage(displayName, o.gear.Name, giveaway)
		}
		contribID := o.contribution.Id
		if err := s.systemMessageWriter.InsertLocalized(ctx, o.originConversationID, o.ownerID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED, msg,
			chat.SystemMessageInsertOptions{CoalesceKey: &contribID}); err != nil {
			logger.WarnContext(ctx, "failed to write gear-offered system message",
				"conversation_id", o.originConversationID, "error", err)
		}
	}

	// Record the RECIPIENT_SELECTED community event. The notification layer
	// suppresses the push for origin-linked transfers — the claim that
	// preceded this offer already surfaced the contribution.
	if _, err := s.recordCommunityEventForTransfer(ctx, transferID, o.gear.Id, transfer.State, transfer.TransferType, o.ownerID, o.recipientID, o.communityID, "", nil); err != nil {
		logger.WarnContext(ctx, "failed to record community event for offer", "transfer_id", transferID, "error", err)
	}

	// Owner watches the gear for inbox tracking.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, o.ownerID, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, o.gear.Id); err != nil {
		logger.WarnContext(ctx, "failed to create watch for offer owner", "error", err)
	}

	logger.InfoContext(ctx, "created origin-linked offer transfer", "transfer_id", transferID)

	return s.buildTransfer(ctx, transfer)
}

// validateOfferGear loads the caller's gear and confirms it is owned by the
// caller and available — the common precondition for both offer paths.
func (s *Service) validateOfferGear(ctx context.Context, gearID, callerID string) (*models.Gear, error) {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}
	if gear.OwnerId != callerID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_offer", "only the gear owner can offer it", nil)
	}
	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_not_available_for_offer", "this item is not available right now", nil)
	}
	return gear, nil
}
