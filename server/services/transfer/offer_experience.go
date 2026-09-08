package transfer

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/storage"
)

// OfferExperienceTransfer creates a live transfer of the caller's gear to an
// event's host for the event's duration (#2708): a participant claiming a
// gear-backed need on an event ("I'll bring my wheelbarrow") escalates that
// claim into a real loan/giveaway. The transfer is born in RECIPIENT_SELECTED
// with the event host as recipient — the host receives on the event's behalf —
// and carries the event as its origin so the event's lifecycle drives it
// (completes on EXPERIENCE_COMPLETED, cancels on EXPERIENCE_CANCELLED). The
// gear is shared into the community with the availability matching the transfer
// type, and the escalated planning contribution is linked to the transfer.
//
// All experience/community-share/contribution reads go through shared storage,
// never by calling the experience service (service independence).
func (s *Service) OfferExperienceTransfer(
	ctx context.Context,
	req *connect.Request[api.OfferExperienceTransferRequest],
) (*connect.Response[api.OfferExperienceTransferResponse], error) {
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
	if req.Msg.OriginExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("origin_experience_id is required"))
	}
	if req.Msg.ContributionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution_id is required"))
	}
	var availability models.Availability
	switch req.Msg.TransferType {
	case api.TransferType_TRANSFER_TYPE_LOAN:
		availability = models.Availability_AVAILABILITY_FOR_LOAN
	case api.TransferType_TRANSFER_TYPE_GIVEAWAY:
		availability = models.Availability_AVAILABILITY_FOR_GIVEAWAY
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transfer_type must be LOAN or GIVEAWAY"))
	}
	if s.gearSharer == nil {
		return nil, connecterr.Internal(ctx, "OfferExperienceTransfer", fmt.Errorf("gear sharer not configured"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "OfferExperienceTransfer",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
		"target_experience_id", req.Msg.OriginExperienceId,
	)

	// Load and validate gear: caller-owned and available.
	gear, err := s.validateOfferGear(ctx, req.Msg.GearId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Caller must be a member of an active community (they saw the event there).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Load and validate the experience: live, not the caller's own event, and
	// shared into this community. Read via shared storage, never the experience
	// service (service independence).
	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.OriginExperienceId, exp); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not found"))
	}
	if exp.OwnerId == authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "cannot_bring_gear_to_own_event", "you can't lend gear to your own event", nil)
	}
	switch exp.State {
	case models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		models.ExperienceState_EXPERIENCE_STATE_JOINED,
		models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
		// open for contributions
	default:
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "event_not_open_for_contributions", "this event is no longer open", nil)
	}
	// The presence of a CommunityExperience row is the authoritative "shared"
	// signal (archived only governs feed visibility on terminal events, which
	// the state check above already excludes).
	sharedRows, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": exp.Id,
		"community_id":  req.Msg.CommunityId,
	}, &models.CommunityExperience{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "OfferExperienceTransfer", err, "detail", "query community experience")
	}
	if len(sharedRows) == 0 {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "event_not_in_community", "this event is not shared with that community", nil)
	}

	// Validate the contribution: owned by the caller, scoped to this experience,
	// and not already escalated.
	contribution, err := planning.ValidateTransferLink(ctx, s.storage, authInfo.UserID, req.Msg.ContributionId, planning.Scope{ExperienceID: exp.Id}, gear.Id)
	if err != nil {
		return nil, err
	}
	if contribution.GetTransferId() != "" {
		return nil, connecterr.UserVisible(ctx, connect.CodeAlreadyExists, "contribution_already_escalated", "this contribution already has a transfer", nil)
	}

	// Dedup: one live offer per (gear, experience).
	existing, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_experience_id", exp.Id)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OfferExperienceTransfer", err, "detail", "query existing origin transfers")
	}
	for _, t := range existing {
		if t.GearId == gear.Id &&
			t.State != models.TransferState_TRANSFER_STATE_COMPLETED &&
			t.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			return nil, connecterr.UserVisible(ctx, connect.CodeAlreadyExists, "gear_already_offered_on_event", "you're already bringing this item to the event", nil)
		}
	}

	// Loan duration spans until the event ends. The client may pass an explicit
	// value; otherwise derive it from the event's scheduled end (loans only —
	// the field is ignored for giveaways).
	loanDuration := req.Msg.LoanDurationDays
	if loanDuration == nil && req.Msg.TransferType == api.TransferType_TRANSFER_TYPE_LOAN {
		if d, ok := deriveEventLoanDurationDays(ctx, exp); ok {
			loanDuration = &d
		}
	}

	// Share the gear, birth the transfer in RECIPIENT_SELECTED with the host as
	// recipient, link the contribution, and record the moment — shared with the
	// request-offer path so the two cannot drift.
	apiTransfer, err := s.createOriginOfferTransfer(ctx, originOffer{
		gear:                 gear,
		ownerID:              authInfo.UserID,
		recipientID:          exp.OwnerId,
		communityID:          req.Msg.CommunityId,
		transferType:         req.Msg.TransferType,
		availability:         availability,
		originExperienceID:   exp.Id,
		contribution:         contribution,
		loanDuration:         loanDuration,
		originConversationID: exp.ConversationId,
	}, logger)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.OfferExperienceTransferResponse{Transfer: apiTransfer}), nil
}

// deriveEventLoanDurationDays computes a loan duration (in whole days from now
// to the event's scheduled end) for a gear-backed event contribution. Returns
// false when the event has no scheduled time (TBD/undated) — the caller then
// leaves loan_duration_days unset, which is harmless because event child
// transfers complete on EXPERIENCE_COMPLETED rather than on a return date.
func deriveEventLoanDurationDays(ctx context.Context, exp *models.Experience) (int32, bool) {
	end, ok := eventEndUnixSec(exp)
	if !ok {
		return 0, false
	}
	now := clock.UnixSec(ctx)
	days := int32(math.Ceil(float64(end-now) / 86400.0))
	if days < 1 {
		days = 1
	}
	return days, true
}

// eventEndUnixSec returns the event's scheduled end (Unix seconds) and whether
// one is known, from either a specific time (+ optional duration) or a range.
func eventEndUnixSec(exp *models.Experience) (int64, bool) {
	t := exp.GetTime()
	if t == nil {
		return 0, false
	}
	if sp := t.GetSpecific(); sp != nil && sp.GetUnixTimestampSec() > 0 {
		return sp.GetUnixTimestampSec() + int64(sp.GetDurationMinutes())*60, true
	}
	if r := t.GetRange(); r != nil {
		if r.GetEndUnixSec() > 0 {
			return r.GetEndUnixSec(), true
		}
		if r.GetStartUnixSec() > 0 {
			return r.GetStartUnixSec() + int64(r.GetDurationMinutes())*60, true
		}
	}
	return 0, false
}
