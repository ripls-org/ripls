package chat_event_bus

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
)

// Kind discriminates what kind of event is being published. The stream
// subscriber handles all four kinds; the push subscriber only handles
// KIND_USER_MESSAGE.
type Kind int

const (
	// KindUserMessage is a standard user-authored chat message.
	KindUserMessage Kind = iota + 1
	// KindSystemMessage is a server-generated system message inserted into the conversation.
	KindSystemMessage
	// KindReactionUpdate reflects a reaction being added or removed on an existing message.
	// For this kind, Message may be nil — the update is carried by ReactionUpdate.
	KindReactionUpdate
	// KindSystemMessageUpdate is a coalesced update to an existing system message card.
	KindSystemMessageUpdate
	// KindUserMessageUpdate reflects an edit to an existing user message's text.
	// Message carries the edited message; the stream subscriber emits a
	// UserMessageUpdate so clients replace the text in place.
	KindUserMessageUpdate
	// KindMessageDelete reflects a message being deleted. Message carries the
	// deleted message; the stream subscriber emits a MessageDelete so clients
	// remove it in place.
	KindMessageDelete
)

// Publisher is the surface emit sites depend on. Emit sites call Publish after
// the ChatMessage audit row has already been inserted; the bus handles only
// prefetch and fan-out.
//
// Errors are returned only for nil-input invariants. Actual dispatch failures
// are surfaced via log lines from the underlying pubsub.MemTopic.
type Publisher interface {
	// Publish prefetches denormalized context and dispatches the event to all
	// registered subscribers asynchronously.
	//
	// msg may be nil when kind is KindReactionUpdate; the reaction payload is
	// supplied via WithReactionUpdate. For all other kinds, msg must be non-nil.
	// conversation must always be non-nil.
	Publish(ctx context.Context, kind Kind, msg *models.ChatMessage, conversation *models.ChatConversation, opts ...PublishOption) error
}

// PublishedEvent is what subscribers receive. The raw ChatMessage is augmented
// with pre-fetched denormalized context so each subscriber avoids re-fetching
// the same rows.
//
// Pointer fields are populated only when available. Prefetch failures leave
// fields nil and are logged at WARN. Subscribers MUST handle nil fields
// gracefully.
type PublishedEvent struct {
	Kind         Kind
	Message      *models.ChatMessage
	Conversation *models.ChatConversation

	// Sender is the prefetched api.User for the message sender/actor. Nil when
	// the message has no sender (some system events) or when prefetch failed.
	Sender *api.User

	// Transfer is the prefetched transfer when the conversation topic is a
	// transfer — used by the push subscriber to resolve gear_id for deep links.
	Transfer *models.Transfer

	// MentionedUserIDs is the list of user IDs @-mentioned in the message,
	// populated from WithMentionedUserIDs.
	MentionedUserIDs []string

	// OnBehalfOf, when true, indicates this message was inserted by the server
	// on behalf of the actor (e.g. InsertUserMessageOnBehalfOf). The push
	// subscriber short-circuits on this flag to avoid duplicate notifications.
	OnBehalfOf bool

	// ReactionUpdate carries the updated reaction set for KindReactionUpdate
	// events. Nil for all other kinds.
	ReactionUpdate *api.ReactionUpdate
}

// Subscriber is the per-event handler shape. Type alias keeps the underlying
// pubsub interface in scope for callers that want to register, while letting
// subscribers be referred to by the domain-flavored name.
type Subscriber = pubsub.Subscriber[*PublishedEvent]

// Topic is a type alias to the underlying parametric topic. Useful for tests
// that want to construct a MemTopic[*PublishedEvent] directly.
type Topic = pubsub.Topic[*PublishedEvent]

// LogContext implements pubsub.LogContextProvider so dispatch log lines
// automatically include the canonical chat identifiers.
//
// Field names follow docs/server/observability.md § "Standard Field Names":
//   - message_id (the chat message id)
//   - conversation_id
//   - chat_event_kind
func (p *PublishedEvent) LogContext() []any {
	if p == nil {
		return nil
	}
	msgID := ""
	if p.Message != nil {
		msgID = p.Message.Id
	} else if p.ReactionUpdate != nil {
		msgID = p.ReactionUpdate.MessageId
	}
	convID := ""
	if p.Conversation != nil {
		convID = p.Conversation.Id
	}
	return []any{
		"message_id", msgID,
		"conversation_id", convID,
		"chat_event_kind", p.Kind,
	}
}

// PublishOption is a functional option for Publish.
type PublishOption func(*publishOptions)

type publishOptions struct {
	mentionedUserIDs []string
	onBehalfOf       bool
	reactionUpdate   *api.ReactionUpdate
}

// WithMentionedUserIDs sets the user IDs that were @-mentioned in the message.
// The push subscriber uses this to customize the notification title.
func WithMentionedUserIDs(ids []string) PublishOption {
	return func(o *publishOptions) {
		o.mentionedUserIDs = ids
	}
}

// WithOnBehalfOf marks the message as server-inserted on behalf of the actor.
// The push subscriber short-circuits when this flag is true, deferring push
// to the caller's own notification path.
func WithOnBehalfOf(v bool) PublishOption {
	return func(o *publishOptions) {
		o.onBehalfOf = v
	}
}

// WithReactionUpdate supplies the updated reaction set for KindReactionUpdate
// events. Required when kind is KindReactionUpdate and msg is nil.
func WithReactionUpdate(ru *api.ReactionUpdate) PublishOption {
	return func(o *publishOptions) {
		o.reactionUpdate = ru
	}
}
