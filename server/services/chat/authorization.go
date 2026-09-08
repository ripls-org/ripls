package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// requireParticipant verifies that a user is a participant in a conversation.
func (s *Service) requireParticipant(ctx context.Context, conversationID, userID string) error {
	conversation := &models.ChatConversation{}
	if err := s.storage.GetByID(ctx, conversationID, conversation); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	for _, participantID := range conversation.ParticipantIds {
		if participantID == userID {
			return nil
		}
	}

	return connect.NewError(connect.CodePermissionDenied,
		fmt.Errorf("user is not a participant in this conversation"))
}

// collectSharedCommunityIDs returns every community a conversation's topic is
// shared with. Gear, experience, and request topics fan out across their
// CommunityGear / CommunityExperience / CommunityRequest rows because a single
// item can be shared with many communities. Community-wide topics return the
// topic's community. Other topics (transfer, etc.) fall back to the
// conversation's primary CommunityId.
func collectSharedCommunityIDs(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	conversation *models.ChatConversation,
) ([]string, error) {
	topic := conversation.GetTopic()
	if topic != nil {
		if gearID := topic.GetGearId(); gearID != "" {
			cgs, err := store.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
			if err != nil {
				return nil, fmt.Errorf("failed to check gear communities: %w", err)
			}
			ids := make([]string, 0, len(cgs))
			for _, msg := range cgs {
				ids = append(ids, msg.(*models.CommunityGear).CommunityId)
			}
			return ids, nil
		}
		if experienceID := topic.GetExperienceId(); experienceID != "" {
			ces, err := store.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
			if err != nil {
				return nil, fmt.Errorf("failed to check experience communities: %w", err)
			}
			ids := make([]string, 0, len(ces))
			for _, msg := range ces {
				ids = append(ids, msg.(*models.CommunityExperience).CommunityId)
			}
			return ids, nil
		}
		if requestID := topic.GetRequestId(); requestID != "" {
			crs, err := store.QueryByField(ctx, "request_id", requestID, &models.CommunityRequest{})
			if err != nil {
				return nil, fmt.Errorf("failed to check request communities: %w", err)
			}
			ids := make([]string, 0, len(crs))
			for _, msg := range crs {
				ids = append(ids, msg.(*models.CommunityRequest).CommunityId)
			}
			return ids, nil
		}
		if communityID := topic.GetCommunityId(); communityID != "" {
			return []string{communityID}, nil
		}
	}

	if conversation.CommunityId != "" {
		return []string{conversation.CommunityId}, nil
	}
	return nil, nil
}
