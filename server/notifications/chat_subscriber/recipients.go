package chat_subscriber

import (
	"context"

	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// resolveRecipients returns the list of user IDs that should receive a push
// notification for the given conversation and message sender. The sender is
// excluded from the list.
//
// For community-wide conversations (topic = CommunityId, empty
// ParticipantIds), we fan out to all community members. For topic-scoped
// conversations (transfer/request/gear/experience) the stored ParticipantIds
// is the authoritative list.
func resolveRecipients(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	conversation *models.ChatConversation,
	senderID string,
	logger *logging.Logger,
) []string {
	var candidates []string

	if conversation.GetTopic().GetCommunityId() != "" {
		// Community-wide conversation: fan out to all members.
		candidates = community.GetCommunityMemberIDs(ctx, s, conversation.CommunityId)
		logger.DebugContext(ctx, "fanning out community-wide chat notification",
			"community_id", conversation.CommunityId,
			"candidate_count", len(candidates),
		)
	} else {
		candidates = conversation.ParticipantIds
	}

	// Exclude sender.
	recipients := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if id != senderID {
			recipients = append(recipients, id)
		}
	}
	return recipients
}
