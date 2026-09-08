package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// AcceptRequestOffer marks a gear-backed offer as the one the requester wants
// ("this one works", #2702). Requester-only, toggle semantics: accepting an
// already-accepted offer clears the acceptance without re-emitting the
// selection event. Acceptance is a signal to the chosen helper — it changes
// neither request nor transfer state; handoff still drives fulfillment.
func (s *Service) AcceptRequestOffer(
	ctx context.Context,
	req *connect.Request[api.AcceptRequestOfferRequest],
) (*connect.Response[api.AcceptRequestOfferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.RequestId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request_id is required"))
	}
	if req.Msg.ContributionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution_id is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AcceptRequestOffer",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
		"contribution_id", req.Msg.ContributionId,
	)

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}
	if requestStored.RequesterId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_accept", "only the requester can accept an offer", nil)
	}

	contribution := &models.PlanningContribution{}
	if err := s.storage.GetByID(ctx, req.Msg.ContributionId, contribution); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("offer not found"))
	}
	if contribution.GetRequestId() != requestStored.Id || contribution.Deleted != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("offer not found on this request"))
	}
	if contribution.GetTransferId() == "" {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "contribution_has_no_gear_offer", "this contribution has no item offer to accept", nil)
	}

	if contribution.AcceptedAtUnixSec != nil {
		// Toggle off: the requester changed their mind. No selection event.
		contribution.AcceptedAtUnixSec = nil
		if err := s.storage.Update(ctx, contribution); err != nil {
			return nil, connecterr.Internal(ctx, "AcceptRequestOffer", err, "detail", "clear acceptance")
		}
		logger.InfoContext(ctx, "cleared offer acceptance")
	} else {
		now := clock.UnixSec(ctx)
		contribution.AcceptedAtUnixSec = &now
		if err := s.storage.Update(ctx, contribution); err != nil {
			return nil, connecterr.Internal(ctx, "AcceptRequestOffer", err, "detail", "set acceptance")
		}

		// Emit the selection event in each community the request is shared
		// with the helper through — recipients resolve from ObjectUserId.
		// One event on the transfer's community suffices for the push; use
		// the community the offer's transfer lives in.
		transfer := &models.Transfer{}
		communityID := ""
		if err := s.storage.GetByID(ctx, contribution.GetTransferId(), transfer); err != nil {
			logger.WarnContext(ctx, "linked transfer missing for acceptance event", "transfer_id", contribution.GetTransferId(), "error", err)
		} else {
			communityID = transfer.CommunityId
		}
		if communityID != "" {
			if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
				CommunityId:  communityID,
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED,
				ActorId:      authInfo.UserID,
				ObjectUserId: contribution.ContributorId,
				Topic:        &models.CommunityEvent_RequestId{RequestId: requestStored.Id},
			}); err != nil {
				logger.WarnContext(ctx, "failed to publish offer-selected event", "error", err)
			}
		}

		// Announce the acceptance in the request thread — the emotional beat
		// of the flow ("X accepted Y's offer", #2724). Best-effort: the
		// acceptance itself already persisted above.
		if s.systemMessageWriter != nil && requestStored.ConversationId != "" {
			msg := chat.OfferAcceptedMessage(
				s.getUserDisplayName(ctx, authInfo.UserID),
				s.getUserDisplayName(ctx, contribution.ContributorId),
			)
			if err := s.systemMessageWriter.InsertLocalized(ctx, requestStored.ConversationId, authInfo.UserID,
				models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED, msg); err != nil {
				logger.WarnContext(ctx, "failed to write offer-accepted system message",
					"conversation_id", requestStored.ConversationId, "error", err)
			}
		}
		logger.InfoContext(ctx, "accepted gear-backed offer", "helper_id", contribution.ContributorId)
	}

	requestItem, err := s.buildRequest(ctx, requestStored, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.AcceptRequestOfferResponse{Request: requestItem}), nil
}
