package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// requireConversationAccess verifies that a user can access a conversation.
//
// Transfer-topic conversations use a strict participant-only check — the
// transfer must be wrappable up after either party leaves their shared
// community, so community membership is intentionally not re-verified for
// that topic kind.
//
// For all other topic kinds (gear, experience, request, community, or a
// conversation with a community_id but no explicit topic), the caller must
// be an active member of at least one community the topic is shared with.
// ParticipantIds is a denormalization and must not grant access on its own
// because it is not updated when a member leaves a shared community.
//
// Conversations with no topic and no community_id fall back to a participant
// check (legacy conversations that predate the topic/community_id schema).
func (s *Service) requireConversationAccess(ctx context.Context, conversationID, userID string) (*models.ChatConversation, error) {
	conversation := &models.ChatConversation{}
	if err := s.storage.GetByID(ctx, conversationID, conversation); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	topic := conversation.GetTopic()

	// Transfer conversations are participant-only: active community membership
	// is not required because in-flight transfers must be completable even after
	// a party leaves. This mirrors GetConversationForTransfer's check.
	if topic != nil && topic.GetTransferId() != "" {
		for _, participantID := range conversation.ParticipantIds {
			if participantID == userID {
				return conversation, nil
			}
		}
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a participant in this transfer conversation"))
	}

	// Determine topic kind for the deny-path log entry.
	topicKind := "none"
	if topic != nil {
		switch {
		case topic.GetGearId() != "":
			topicKind = "gear"
		case topic.GetExperienceId() != "":
			topicKind = "experience"
		case topic.GetRequestId() != "":
			topicKind = "request"
		case topic.GetCommunityId() != "":
			topicKind = "community"
		}
	} else if conversation.CommunityId != "" {
		topicKind = "community"
	}

	// Re-verify active membership in any community the topic is shared with.
	// For gear, experience, and request topics this fans out across the join
	// tables; for community topics it is a single community; for conversations
	// with a CommunityId but no topic it falls back to that community.
	sharedCommunityIDs, err := collectSharedCommunityIDs(ctx, s.storage, conversation)
	if err != nil {
		return nil, connecterr.Internal(ctx, "requireConversationAccess", err,
			"conversation_id", conversationID, "user_id", userID)
	}

	// No community context — fall back to participant check. This covers legacy
	// conversations that predate the topic/community_id schema.
	if len(sharedCommunityIDs) == 0 {
		for _, participantID := range conversation.ParticipantIds {
			if participantID == userID {
				return conversation, nil
			}
		}
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to view this conversation"))
	}

	// Single batched JOIN: one round-trip for all candidate communities.
	pairs, err := s.storage.GetCommunitiesWithMembership(ctx, sharedCommunityIDs, userID)
	if err != nil {
		return nil, connecterr.Internal(ctx, "requireConversationAccess", err,
			"conversation_id", conversationID, "user_id", userID)
	}

	for _, communityID := range sharedCommunityIDs {
		pair, ok := pairs[communityID]
		if !ok {
			continue // community hard-deleted or never existed
		}
		if pair.Community.Deleted != nil && pair.Community.Deleted.DeletedAtUnixSec > 0 {
			continue // community soft-deleted
		}
		if pair.Membership == nil {
			continue // user never joined this community
		}
		if pair.Membership.Deleted != nil && pair.Membership.Deleted.DeletedAtUnixSec > 0 {
			continue // user left this community
		}
		return conversation, nil
	}

	logging.LoggerWithContext(ctx).With(
		"operation", "requireConversationAccess",
		"user_id", userID,
		"conversation_id", conversationID,
		"topic_kind", topicKind,
		"shared_community_count", len(sharedCommunityIDs),
	).InfoContext(ctx, "access denied: user is not an active member of any shared community")

	return nil, connect.NewError(connect.CodePermissionDenied,
		fmt.Errorf("user is not a member of any community this conversation is shared with"))
}
