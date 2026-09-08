package chat

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"

	"go.ripls.org/ripls/server/chat_event_bus"
)

// StreamSubscriberName is the stable identifier used by pubsub log lines for
// the chat stream broadcast subscriber.
const StreamSubscriberName = "chat_stream"

// StreamSubscriber returns a chat_event_bus.Subscriber that delivers every
// published event to all active StreamMessages handlers for the conversation.
// It wraps the service's in-memory stream registry (streams / broadcastMessage)
// so the registry stays the single source of truth for active streams.
//
// Co-located with the service rather than in a separate package because it
// shares per-stream state with StreamMessages — moving it across a package
// boundary would require exposing the registry.
//
// No soft-delete gate: streams self-clean when communities are soft-deleted
// via membership cascade; broadcasting to zero subscribers is a no-op.
func (s *Service) StreamSubscriber() chat_event_bus.Subscriber {
	return &chatStreamSubscriber{svc: s}
}

// chatStreamSubscriber implements pubsub.Subscriber[*chat_event_bus.PublishedEvent].
type chatStreamSubscriber struct {
	svc *Service
}

func (ss *chatStreamSubscriber) Name() string { return StreamSubscriberName }

// Handle converts the PublishedEvent to the appropriate StreamMessagesResponse
// variant and fans it out to all active streams for the conversation.
func (ss *chatStreamSubscriber) Handle(ctx context.Context, evt *chat_event_bus.PublishedEvent) error {
	if evt == nil || evt.Conversation == nil {
		return nil
	}

	response := ss.toStreamResponse(ctx, evt)
	if response == nil {
		return nil
	}

	ss.svc.broadcastMessage(ctx, evt.Conversation.Id, response)
	return nil
}

// toStreamResponse converts a PublishedEvent into the StreamMessagesResponse
// shape that live stream clients expect, based on the event Kind.
func (ss *chatStreamSubscriber) toStreamResponse(ctx context.Context, evt *chat_event_bus.PublishedEvent) *api.StreamMessagesResponse {
	switch evt.Kind {
	case chat_event_bus.KindUserMessage:
		if evt.Message == nil {
			return nil
		}
		userMsg := evt.Message.GetUserMessage()
		if userMsg == nil {
			return nil
		}
		apiUser := apiUserOrNil(evt.Sender)
		resp := &api.StreamMessagesResponse{
			MessageId:      evt.Message.Id,
			ConversationId: evt.Conversation.Id,
			SentAtUnixSec:  evt.Message.SentAtUnixSec,
			Message: &api.StreamMessagesResponse_UserMessage{
				UserMessage: &api.UserMessage{
					Sender:            apiUser,
					Text:              userMsg.GetText(),
					MediaIds:          userMsg.GetMediaIds(),
					NeedId:            userMsg.NeedId,
					NeedName:          userMsg.NeedName,
					ContributionId:    userMsg.ContributionId,
					ContributionTitle: userMsg.ContributionTitle,
					EditedAtUnixSec:   userMsg.EditedAtUnixSec,
					ReplyTo:           ss.svc.convertReplyToAPI(ctx, userMsg.ReplyTo),
				},
			},
		}
		return resp

	case chat_event_bus.KindSystemMessage:
		if evt.Message == nil {
			return nil
		}
		sys := evt.Message.GetSystemMessage()
		if sys == nil {
			return nil
		}
		apiActor := apiUserOrNil(evt.Sender)
		apiSysMsg := &api.SystemMessage{
			Actor:          apiActor,
			Action:         api.ChatSystemAction(sys.GetAction()),
			Description:    sys.GetDescription(),
			TemplateKey:    sys.TemplateKey,
			TemplateParams: sys.TemplateParams,
		}
		if sys.PollId != nil {
			apiSysMsg.PollId = sys.PollId
		}
		return &api.StreamMessagesResponse{
			MessageId:      evt.Message.Id,
			ConversationId: evt.Conversation.Id,
			SentAtUnixSec:  evt.Message.SentAtUnixSec,
			Message: &api.StreamMessagesResponse_SystemMessage{
				SystemMessage: apiSysMsg,
			},
		}

	case chat_event_bus.KindSystemMessageUpdate:
		if evt.Message == nil {
			return nil
		}
		sys := evt.Message.GetSystemMessage()
		if sys == nil {
			return nil
		}
		apiActor := apiUserOrNil(evt.Sender)
		return &api.StreamMessagesResponse{
			MessageId:      evt.Message.Id,
			ConversationId: evt.Conversation.Id,
			SentAtUnixSec:  evt.Message.SentAtUnixSec,
			Message: &api.StreamMessagesResponse_SystemMessageUpdate{
				SystemMessageUpdate: &api.SystemMessageUpdate{
					MessageId:      evt.Message.Id,
					SentAtUnixSec:  evt.Message.SentAtUnixSec,
					Actor:          apiActor,
					Action:         api.ChatSystemAction(sys.GetAction()),
					Description:    sys.GetDescription(),
					TemplateKey:    sys.TemplateKey,
					TemplateParams: sys.TemplateParams,
				},
			},
		}

	case chat_event_bus.KindReactionUpdate:
		if evt.ReactionUpdate == nil {
			return nil
		}
		return &api.StreamMessagesResponse{
			MessageId:      evt.ReactionUpdate.MessageId,
			ConversationId: evt.Conversation.Id,
			Message: &api.StreamMessagesResponse_ReactionUpdate{
				ReactionUpdate: evt.ReactionUpdate,
			},
		}

	case chat_event_bus.KindUserMessageUpdate:
		if evt.Message == nil {
			return nil
		}
		userMsg := evt.Message.GetUserMessage()
		if userMsg == nil {
			return nil
		}
		return &api.StreamMessagesResponse{
			MessageId:      evt.Message.Id,
			ConversationId: evt.Conversation.Id,
			SentAtUnixSec:  evt.Message.SentAtUnixSec,
			Message: &api.StreamMessagesResponse_UserMessageUpdate{
				UserMessageUpdate: &api.UserMessageUpdate{
					MessageId:       evt.Message.Id,
					Text:            userMsg.GetText(),
					EditedAtUnixSec: userMsg.GetEditedAtUnixSec(),
				},
			},
		}

	case chat_event_bus.KindMessageDelete:
		if evt.Message == nil {
			return nil
		}
		return &api.StreamMessagesResponse{
			MessageId:      evt.Message.Id,
			ConversationId: evt.Conversation.Id,
			SentAtUnixSec:  evt.Message.SentAtUnixSec,
			Message: &api.StreamMessagesResponse_MessageDelete{
				MessageDelete: &api.MessageDelete{
					MessageId: evt.Message.Id,
				},
			},
		}

	default:
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"chat_stream: unhandled event kind",
			"chat_event_kind", evt.Kind,
			"conversation_id", evt.Conversation.Id,
		)
		return nil
	}
}

// apiUserOrNil is a nil-safe passthrough. Declared to make nil handling
// explicit at the call site — a nil Sender in a published event is valid (e.g.
// prefetch failure) and the API accepts a nil actor for system messages.
func apiUserOrNil(u *api.User) *api.User {
	return u
}
