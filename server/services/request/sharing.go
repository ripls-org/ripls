package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// VerifyRequestOwner returns an error unless actorUserID created the request.
// Implements the ItemSharer owner check used by CommunityService.ShareItem.
func (s *Service) VerifyRequestOwner(ctx context.Context, requestID, actorUserID string) error {
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}
	if request.RequesterId != actorUserID {
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_share", "only the request creator can share this request", nil)
	}
	return nil
}

// VerifyRequestViewer returns the request creator's user ID after checking
// that actorUserID may view the request — the creator, or an active member of
// at least one community it is shared with. Implements the ItemSharer
// view-access check gating link-only ShareItem calls, so any member can
// reshare the request's open link (#2630).
func (s *Service) VerifyRequestViewer(ctx context.Context, requestID, actorUserID string) (string, error) {
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		return "", connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, actorUserID,
		auth.EntityRequest, request.Id, request.RequesterId,
	); err != nil {
		return "", err
	}
	return request.RequesterId, nil
}

// ShareRequestToCommunity shares a request into a community idempotently. Auth
// (ownership + membership) is the caller's responsibility. Used by SubmitRequest
// (the per-item community at creation) and by the request ItemSharer
// (CommunityService.ShareItem).
func (s *Service) ShareRequestToCommunity(ctx context.Context, requestID, communityID, actorUserID string) error {
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}
	return s.shareRequestToCommunity(ctx, request, communityID, actorUserID)
}

// shareRequestToCommunity performs the core of sharing a request into a
// community: ensures the request conversation, writes the CommunityRequest
// junction, adds the requester to the conversation, emits the REQUEST_CREATED
// anchor + description seed on first share, and records the community event. It
// is idempotent — already shared → no changes. Auth is the caller's
// responsibility. The passed request's ConversationId may be updated in place.
func (s *Service) shareRequestToCommunity(ctx context.Context, request *models.Request, communityID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "shareRequestToCommunity",
		"target_request_id", request.Id,
		"community_id", communityID,
	)

	// Already shared to this community? Idempotent skip.
	existing, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   request.Id,
		"community_id": communityID,
	}, &models.CommunityRequest{})
	if err != nil {
		return connecterr.Internal(ctx, "shareRequestToCommunity", err)
	}
	if len(existing) > 0 {
		return nil
	}

	// Create or reuse the request conversation (scoped to the request itself).
	chatConvStorage := storage.NewChatConversationStorage(s.storage)
	conversationID := request.ConversationId
	isNewConversation := conversationID == ""
	if isNewConversation {
		conversationID, _, err = chat.CreateOrGetConversation(ctx, s.storage, chatConvStorage, communityID, &models.ConversationTopic{
			TopicId: &models.ConversationTopic_RequestId{RequestId: request.Id},
		})
		if err != nil {
			return connecterr.Internal(ctx, "shareRequestToCommunity", err, "detail", "failed to create conversation")
		}
		request.ConversationId = conversationID
		if err := s.storage.Update(ctx, request); err != nil {
			return connecterr.Internal(ctx, "shareRequestToCommunity", err, "detail", "failed to update request")
		}
	}

	// Create CommunityRequest junction record. conversation_id on this record is
	// deprecated; the canonical conversation lives on Request.conversation_id.
	communityRequest := &models.CommunityRequest{
		RequestId:       request.Id,
		CommunityId:     communityID,
		SharedAtUnixSec: clock.UnixSec(ctx),
		Archived:        false,
	}
	if _, err := s.storage.Insert(ctx, communityRequest); err != nil {
		return connecterr.Internal(ctx, "shareRequestToCommunity", err, "detail", "failed to link request to community")
	}

	// Add requester to conversation (best-effort).
	if err := chat.AddParticipantToConversation(ctx, chatConvStorage, conversationID, actorUserID); err != nil {
		logger.WarnContext(ctx, "failed to add requester to conversation", "error", err)
	}

	// On first share, anchor the request info card and seed its description as
	// the first comment (best-effort; mirrors the experience flow).
	if isNewConversation && s.systemMessageWriter != nil {
		displayName := s.getUserDisplayName(ctx, actorUserID)
		if err := s.systemMessageWriter.InsertLocalized(ctx, conversationID, actorUserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_REQUEST_CREATED, chat.RequestCreatedMessage(displayName, request.Title)); err != nil {
			logger.WarnContext(ctx, "failed to write REQUEST_CREATED system message", "conversation_id", conversationID, "error", err)
		}
		if err := chat.PostCreationDescription(ctx, s.systemMessageWriter, conversationID, actorUserID, request.Description); err != nil {
			logger.WarnContext(ctx, "failed to seed request description as first comment", "conversation_id", conversationID, "error", err)
		}
	}

	// Record the community event for this community (best-effort).
	if _, err := s.recordCommunityEventForRequest(ctx, request.Id, request.State, actorUserID, communityID, "", nil); err != nil {
		logger.WarnContext(ctx, "failed to record community event for request share", "error", err)
	}

	logger.InfoContext(ctx, "shared request with community")
	return nil
}

// UnshareRequest removes a request from a specific community.
// The request creator can unshare from any community except the last one.
// UnshareRequestFromCommunity removes a request from a community by soft-archiving
// its CommunityRequest record. Owner-only; refuses to remove the request from its
// last remaining community, and returns NotFound when it isn't shared there.
// Implements the ItemSharer UnshareFromCommunity hook used by
// CommunityService.UnshareItem.
func (s *Service) UnshareRequestFromCommunity(ctx context.Context, requestID, communityID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", actorUserID,
		"target_request_id", requestID,
		"community_id", communityID,
	)

	logger.InfoContext(ctx, "unsharing request from community")

	// Get the request to verify ownership
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		logger.ErrorContext(ctx, "failed to get request", "error", err)
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Verify user is the request creator
	if request.RequesterId != actorUserID {
		logger.WarnContext(ctx, "user is not request creator", "requester_id", request.RequesterId)
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_unshare", "only the request creator can unshare this request", nil)
	}

	// Get all CommunityRequest records for this request
	allRecords, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id": requestID,
		"archived":   false,
	}, &models.CommunityRequest{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community requests", "error", err)
		return connecterr.Internal(ctx, "UnshareRequest", err)
	}

	// Prevent unsharing from the last community
	if len(allRecords) <= 1 {
		logger.WarnContext(ctx, "cannot unshare from last community")
		return connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_last_community_cannot_unshare", "you can't unshare from the last remaining community", nil)
	}

	// Find the CommunityRequest record for this community
	var communityRequestToArchive *models.CommunityRequest
	for _, record := range allRecords {
		cr := record.(*models.CommunityRequest)
		if cr.CommunityId == communityID {
			communityRequestToArchive = cr
			break
		}
	}

	if communityRequestToArchive == nil {
		logger.WarnContext(ctx, "request not shared with this community")
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("request is not shared with this community"))
	}

	// Archive the CommunityRequest record (soft delete)
	communityRequestToArchive.Archived = true
	if err := s.storage.Update(ctx, communityRequestToArchive); err != nil {
		logger.ErrorContext(ctx, "failed to archive community request", "error", err)
		return connecterr.Internal(ctx, "UnshareRequest", err)
	}

	logger.InfoContext(ctx, "unshared request from community")
	return nil
}
