package chat

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/auth"
	libchat "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// MentionedUser contains info about a user who was mentioned and added to a conversation.
type MentionedUser struct {
	UserID   string
	UserName string
	WasAdded bool // true if user was newly added to the conversation
}

// processMentions parses @-mentions from message text and adds mentioned users to the conversation.
// Returns the list of mentioned users (for notification purposes).
func (s *Service) processMentions(
	ctx context.Context,
	conversation *models.ChatConversation,
	messageText string,
	senderID string,
	senderName string,
) ([]MentionedUser, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "processMentions",
		"conversation_id", conversation.Id,
		"sender_id", senderID,
	)

	// Parse user mentions from the message text
	mentionedUserIDs := libchat.ParseUserMentions(messageText)
	if len(mentionedUserIDs) == 0 {
		return nil, nil
	}

	logger.Debug("found user mentions", "count", len(mentionedUserIDs))

	// Build set of current participants for quick lookup
	participantSet := make(map[string]bool)
	for _, pid := range conversation.ParticipantIds {
		participantSet[pid] = true
	}

	var mentionedUsers []MentionedUser
	var usersToAdd []string

	for _, userID := range mentionedUserIDs {
		// Skip self-mentions
		if userID == senderID {
			continue
		}

		// Check if user is a community member
		membership, err := auth.GetCommunityUser(ctx, s.storage, conversation.CommunityId, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check community membership for mentioned user %s: %w", userID, err)
		}
		if membership == nil {
			// User is not a community member - skip this mention (not an error)
			logger.Debug("mentioned user is not a community member, ignoring",
				"mentioned_user_id", userID,
			)
			continue
		}

		// Get user info for the display name
		user := &models.User{}
		if err := s.storage.GetByID(ctx, userID, user); err != nil {
			return nil, fmt.Errorf("failed to fetch mentioned user %s: %w", userID, err)
		}

		wasAdded := false
		if !participantSet[userID] {
			// User is not a participant, add them
			usersToAdd = append(usersToAdd, userID)
			participantSet[userID] = true
			wasAdded = true
			logger.Info("adding user to conversation via mention",
				"mentioned_user_id", userID,
				"mentioned_user_name", user.Name,
			)
		}

		mentionedUsers = append(mentionedUsers, MentionedUser{
			UserID:   userID,
			UserName: user.Name,
			WasAdded: wasAdded,
		})
	}

	// Update conversation with new participants
	if len(usersToAdd) > 0 {
		conversation.ParticipantIds = append(conversation.ParticipantIds, usersToAdd...)
		if err := s.chatConvStorage.Update(ctx, conversation); err != nil {
			return nil, fmt.Errorf("failed to update conversation participants: %w", err)
		}

		// Create system messages for each added user
		for _, user := range mentionedUsers {
			if !user.WasAdded {
				continue
			}

			if err := s.insertAddedViaMentionMessage(ctx, conversation, user.UserID, user.UserName, senderID, senderName); err != nil {
				return nil, fmt.Errorf("failed to insert system message for mention add: %w", err)
			}
		}
	}

	return mentionedUsers, nil
}

// insertAddedViaMentionMessage creates a system message indicating a user was added via @-mention.
func (s *Service) insertAddedViaMentionMessage(
	ctx context.Context,
	conversation *models.ChatConversation,
	addedUserID string,
	addedUserName string,
	mentionerID string,
	mentionerName string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "insertAddedViaMentionMessage",
		"conversation_id", conversation.Id,
		"added_user_id", addedUserID,
		"mentioner_id", mentionerID,
	)

	now := clock.UnixSec(ctx)

	// Initialize read status for all participants (including the newly added user)
	participantReadStatus := make(map[string]bool)
	for _, participantID := range conversation.ParticipantIds {
		participantReadStatus[participantID] = false
	}

	message := &models.ChatMessage{
		ConversationId:        conversation.Id,
		SentAtUnixSec:         now,
		ParticipantIdToIsRead: participantReadStatus,
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				ActorId:     &mentionerID,
				Action:      models.ChatSystemAction_CHAT_SYSTEM_ACTION_ADDED_VIA_MENTION,
				Description: libchat.AddedViaMentionText(addedUserName, mentionerName),
			},
		},
	}

	messageID, err := s.storage.Insert(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to insert system message: %w", err)
	}
	message.Id = messageID

	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindSystemMessage, message, conversation); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for mention system message", "error", err)
		}
	}

	logger.Info("inserted added-via-mention system message", "message_id", messageID)

	return nil
}
