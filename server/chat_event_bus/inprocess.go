package chat_event_bus

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// TopicName is the stable name of the underlying pubsub topic. Appears as the
// "topic" field on every emitted log line; alerts and dashboards key on it.
const TopicName = "chat_messages"

// InProcessBus is the production Publisher implementation. It prefetches
// denormalized context (sender User, transfer for transfer-topic deep links)
// and fans out asynchronously to subscribers via the underlying pubsub.Topic.
// Unlike community_event_bus, it does NOT insert any storage row.
type InProcessBus struct {
	storage *storage.ProtoSQLStorage
	topic   Topic
}

// NewInProcessBus constructs a Publisher backed by sqlStorage and topic.
// In production main.go constructs topic as a fresh
// pubsub.NewMemTopic[*PublishedEvent](TopicName); tests can pass a
// pubsub.MockTopic[*PublishedEvent] to capture publishes without running fan-out.
func NewInProcessBus(sqlStorage *storage.ProtoSQLStorage, topic Topic) *InProcessBus {
	return &InProcessBus{
		storage: sqlStorage,
		topic:   topic,
	}
}

// Publish satisfies the Publisher interface.
func (b *InProcessBus) Publish(ctx context.Context, kind Kind, msg *models.ChatMessage, conversation *models.ChatConversation, opts ...PublishOption) error {
	if conversation == nil {
		return fmt.Errorf("chat_event_bus: nil conversation")
	}
	if kind != KindReactionUpdate && msg == nil {
		return fmt.Errorf("chat_event_bus: nil message for kind %d", kind)
	}

	o := &publishOptions{}
	for _, fn := range opts {
		fn(o)
	}

	pe := &PublishedEvent{
		Kind:             kind,
		Message:          msg,
		Conversation:     conversation,
		MentionedUserIDs: o.mentionedUserIDs,
		OnBehalfOf:       o.onBehalfOf,
		ReactionUpdate:   o.reactionUpdate,
	}

	prefetchEntities(ctx, b.storage, pe)

	// Publish into the underlying topic. The topic emits its own
	// "pubsub event published" INFO line and dispatches asynchronously.
	_ = b.topic.Publish(ctx, pe)

	return nil
}

// Subscribe registers s on the bus's underlying topic. Convenience
// passthrough so callers don't need to hold a separate reference to the topic.
func (b *InProcessBus) Subscribe(s Subscriber, opts ...pubsub.SubscribeOption) (pubsub.Unsubscribe, error) {
	return b.topic.Subscribe(s, opts...)
}

// Drain is a passthrough to the underlying topic for tests that need to wait
// for in-flight dispatches.
func (b *InProcessBus) Drain(ctx context.Context) error {
	return b.topic.Drain(ctx)
}
