package chat

import (
	"context"
	"fmt"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// userChatMessageToAPI converts a stored user message to its API shape,
// resolving the sender and any quoted reply context. Shared by the history,
// stream, and broadcast paths so edited/reply fields stay consistent.
func (s *Service) userChatMessageToAPI(ctx context.Context, msg *models.UserChatMessage) (*api.UserMessage, error) {
	apiSender, err := services.FetchAPIUser(ctx, s.storage, msg.SenderId)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sender: %w", err)
	}

	return &api.UserMessage{
		Sender:            apiSender,
		Text:              msg.Text,
		MediaIds:          msg.MediaIds,
		NeedId:            msg.NeedId,
		NeedName:          msg.NeedName,
		ContributionId:    msg.ContributionId,
		ContributionTitle: msg.ContributionTitle,
		EditedAtUnixSec:   msg.EditedAtUnixSec,
		ReplyTo:           s.convertReplyToAPI(ctx, msg.ReplyTo),
	}, nil
}

// convertReplyToAPI converts a stored ReplyContext to its API shape, resolving
// the quoted sender. Returns nil when the message is not a reply.
func (s *Service) convertReplyToAPI(ctx context.Context, rc *models.ReplyContext) *api.ReplyContext {
	if rc == nil {
		return nil
	}

	var quotedSender *api.User
	if rc.QuotedSenderId != "" {
		if user, err := services.FetchAPIUser(ctx, s.storage, rc.QuotedSenderId); err == nil {
			quotedSender = user
		}
	}

	return &api.ReplyContext{
		ReplyToMessageId: rc.ReplyToMessageId,
		QuotedSender:     quotedSender,
		QuotedText:       rc.QuotedText,
		QuotedMediaId:    rc.QuotedMediaId,
	}
}

// chatMessageToHistoryItem converts a models.ChatMessage to api.MessageHistoryItem.
func (s *Service) chatMessageToHistoryItem(ctx context.Context, chatMsg *models.ChatMessage, currentUserID string) (*api.MessageHistoryItem, error) {
	isRead := chatMsg.ParticipantIdToIsRead[currentUserID]

	item := &api.MessageHistoryItem{
		MessageId:     chatMsg.Id,
		SentAtUnixSec: chatMsg.SentAtUnixSec,
		IsRead:        isRead,
		Reactions:     convertReactionsToAPI(ctx, s, chatMsg.Reactions),
	}

	// Convert based on message type
	switch msg := chatMsg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		apiUserMsg, err := s.userChatMessageToAPI(ctx, msg.UserMessage)
		if err != nil {
			return nil, err
		}
		item.Message = &api.MessageHistoryItem_UserMessage{
			UserMessage: apiUserMsg,
		}

	case *models.ChatMessage_SystemMessage:
		// Fetch actor info (may be nil)
		var apiActor *api.User
		if msg.SystemMessage.GetActorId() != "" {
			if user, err := services.FetchAPIUser(ctx, s.storage, msg.SystemMessage.GetActorId()); err == nil {
				apiActor = user
			}
		}

		apiSysMsg := &api.SystemMessage{
			Actor:          apiActor,
			Action:         api.ChatSystemAction(msg.SystemMessage.Action),
			Description:    msg.SystemMessage.Description,
			TemplateKey:    msg.SystemMessage.TemplateKey,
			TemplateParams: msg.SystemMessage.TemplateParams,
		}
		if msg.SystemMessage.PollId != nil {
			apiSysMsg.PollId = msg.SystemMessage.PollId
		}
		item.Message = &api.MessageHistoryItem_SystemMessage{
			SystemMessage: apiSysMsg,
		}

	default:
		return nil, fmt.Errorf("unknown message type: %T", msg)
	}

	return item, nil
}

// chatMessageToStreamResponse converts a models.ChatMessage to api.StreamMessagesResponse.
func (s *Service) chatMessageToStreamResponse(ctx context.Context, chatMsg *models.ChatMessage) (*api.StreamMessagesResponse, error) {
	response := &api.StreamMessagesResponse{
		MessageId:      chatMsg.Id,
		ConversationId: chatMsg.ConversationId,
		SentAtUnixSec:  chatMsg.SentAtUnixSec,
		Reactions:      convertReactionsToAPI(ctx, s, chatMsg.Reactions),
	}

	// Convert based on message type
	switch msg := chatMsg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		apiUserMsg, err := s.userChatMessageToAPI(ctx, msg.UserMessage)
		if err != nil {
			return nil, err
		}
		response.Message = &api.StreamMessagesResponse_UserMessage{
			UserMessage: apiUserMsg,
		}

	case *models.ChatMessage_SystemMessage:
		// Fetch actor info (may be nil)
		var apiActor *api.User
		if msg.SystemMessage.GetActorId() != "" {
			if user, err := services.FetchAPIUser(ctx, s.storage, msg.SystemMessage.GetActorId()); err == nil {
				apiActor = user
			}
		}

		streamSysMsg := &api.SystemMessage{
			Actor:          apiActor,
			Action:         api.ChatSystemAction(msg.SystemMessage.Action),
			Description:    msg.SystemMessage.Description,
			TemplateKey:    msg.SystemMessage.TemplateKey,
			TemplateParams: msg.SystemMessage.TemplateParams,
		}
		if msg.SystemMessage.PollId != nil {
			streamSysMsg.PollId = msg.SystemMessage.PollId
		}
		response.Message = &api.StreamMessagesResponse_SystemMessage{
			SystemMessage: streamSysMsg,
		}

	default:
		return nil, fmt.Errorf("unknown message type: %T", msg)
	}

	return response, nil
}

// getSenderID extracts the sender/actor ID from a ChatMessage for filtering purposes.
func getSenderID(chatMsg *models.ChatMessage) string {
	switch msg := chatMsg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		return msg.UserMessage.SenderId
	case *models.ChatMessage_SystemMessage:
		return msg.SystemMessage.GetActorId()
	default:
		return ""
	}
}

// getMessageText extracts the text content from a ChatMessage.
func getMessageText(chatMsg *models.ChatMessage) string {
	switch msg := chatMsg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		return msg.UserMessage.Text
	case *models.ChatMessage_SystemMessage:
		return msg.SystemMessage.Description
	default:
		return ""
	}
}
