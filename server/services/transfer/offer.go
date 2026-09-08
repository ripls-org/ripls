package transfer

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/storage"
)

// GearSharer shares gear into a community with an explicit Lend/Give
// availability. Injected from wiring (the implementation lives on the
// community service) so this service can auto-share offered gear without a
// service-to-service dependency.
type GearSharer func(ctx context.Context, gearID, communityID, actorUserID string, availability models.Availability) error

// SetGearSharer sets the gear-sharing hook used by OfferTransfer to share the
// offered gear into the request's community. Required for OfferTransfer.
func (s *Service) SetGearSharer(f GearSharer) {
	s.gearSharer = f
}

// OfferTransfer creates a live transfer targeted at a requester: the caller
// offers their gear toward a community request (#2702). The transfer is born
// in RECIPIENT_SELECTED — the owner commits by offering, and the requester
// already signaled by asking — and carries the request as its origin so the
// request's lifecycle and the transfer's lifecycle can drive each other. The gear
// is shared into the community with the availability matching the transfer
// type, and the escalated planning contribution is linked to the transfer.
func (s *Service) OfferTransfer(
	ctx context.Context,
	req *connect.Request[api.OfferTransferRequest],
) (*connect.Response[api.OfferTransferResponse], error) {
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
	if req.Msg.RecipientUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("recipient_user_id is required"))
	}
	if req.Msg.OriginRequestId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("origin_request_id is required"))
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
		return nil, connecterr.Internal(ctx, "OfferTransfer", fmt.Errorf("gear sharer not configured"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "OfferTransfer",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
		"target_request_id", req.Msg.OriginRequestId,
	)

	// Load and validate gear: caller-owned and available.
	gear, err := s.validateOfferGear(ctx, req.Msg.GearId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Caller must be a member of an active community; the recipient must be a
	// member too (they will coordinate the handoff there).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}
	isMember, err := auth.IsMemberOfCommunity(ctx, s.storage, req.Msg.CommunityId, req.Msg.RecipientUserId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OfferTransfer", err,
			"community_id", req.Msg.CommunityId, "recipient_user_id", req.Msg.RecipientUserId)
	}
	if !isMember {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_recipient_must_be_member", "the recipient must be a community member", nil)
	}

	// Load and validate the request: live, asked by the recipient, not by the
	// caller, and shared into this community.
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, req.Msg.OriginRequestId, request); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}
	if request.RequesterId != req.Msg.RecipientUserId {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("recipient_user_id must be the request's creator"))
	}
	if request.RequesterId == authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "cannot_offer_gear_on_own_request", "you cannot offer gear on your own request", nil)
	}
	if request.State != models.RequestState_REQUEST_STATE_ACTIVE &&
		request.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_not_open_for_offers", "this request is no longer open", nil)
	}
	sharedRows, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   request.Id,
		"community_id": req.Msg.CommunityId,
		"archived":     false,
	}, &models.CommunityRequest{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "OfferTransfer", err, "detail", "query community request")
	}
	if len(sharedRows) == 0 {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_not_in_community", "this request is not shared with that community", nil)
	}

	// Validate the contribution before creating anything, so a bad link fails
	// the whole offer rather than leaving an unlinked transfer behind.
	contribution, err := planning.ValidateTransferLink(ctx, s.storage, authInfo.UserID, req.Msg.ContributionId, planning.Scope{RequestID: request.Id}, gear.Id)
	if err != nil {
		return nil, err
	}
	if contribution.GetTransferId() != "" {
		return nil, connecterr.UserVisible(ctx, connect.CodeAlreadyExists, "contribution_already_escalated", "this offer already has a transfer", nil)
	}

	// Dedup: one live offer per (gear, request).
	existing, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", request.Id)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OfferTransfer", err, "detail", "query existing origin transfers")
	}
	for _, t := range existing {
		if t.GearId == gear.Id &&
			t.State != models.TransferState_TRANSFER_STATE_COMPLETED &&
			t.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			return nil, connecterr.UserVisible(ctx, connect.CodeAlreadyExists, "gear_already_offered_on_request", "this item is already offered on this request", nil)
		}
	}

	// Share the gear, birth the transfer in RECIPIENT_SELECTED, link the
	// contribution, and record the structural moment — shared with the
	// experience-offer path so the two cannot drift.
	apiTransfer, err := s.createOriginOfferTransfer(ctx, originOffer{
		gear:                 gear,
		ownerID:              authInfo.UserID,
		recipientID:          req.Msg.RecipientUserId,
		communityID:          req.Msg.CommunityId,
		transferType:         req.Msg.TransferType,
		availability:         availability,
		originRequestID:      request.Id,
		contribution:         contribution,
		loanDuration:         req.Msg.LoanDurationDays,
		originConversationID: request.ConversationId,
	}, logger)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.OfferTransferResponse{Transfer: apiTransfer}), nil
}
