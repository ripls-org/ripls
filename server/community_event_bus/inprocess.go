package community_event_bus

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// TopicName is the stable name of the underlying pubsub topic. Appears as
// the "topic" field on every emitted log line; alerts and dashboards key on
// it.
const TopicName = "community_events"

// InProcessBus is the production Publisher implementation. It writes the
// CommunityEvent row synchronously, pre-fetches denormalized context, and
// fans out asynchronously to subscribers via the underlying pubsub.Topic.
type InProcessBus struct {
	storage *storage.ProtoSQLStorage
	topic   Topic
}

// NewInProcessBus constructs a Publisher backed by sqlStorage and topic.
// In production main.go constructs topic as a fresh
// pubsub.NewMemTopic[*PublishedEvent](TopicName); tests can pass a
// pubsub.MockTopic[*PublishedEvent] to capture publishes without running
// fan-out.
func NewInProcessBus(sqlStorage *storage.ProtoSQLStorage, topic Topic) *InProcessBus {
	return &InProcessBus{
		storage: sqlStorage,
		topic:   topic,
	}
}

// Publish satisfies the Publisher interface.
func (b *InProcessBus) Publish(ctx context.Context, event *models.CommunityEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("community_event_bus: nil event")
	}

	if event.OccurredAtUnixSec == 0 {
		event.OccurredAtUnixSec = clock.UnixSec(ctx)
	}

	eventID, err := b.storage.Insert(ctx, event)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx,
			"failed to record community event",
			"operation", "community_event_bus.Publish",
			"community_id", event.CommunityId,
			"event_type", event.EventType.String(),
			"error", err,
		)
		return "", fmt.Errorf("community_event_bus: insert event: %w", err)
	}
	event.Id = eventID

	pe := &PublishedEvent{Event: event}
	prefetchEntities(ctx, b.storage, pe)
	snapshotMembers(ctx, b.storage, pe)

	// Publish into the underlying topic. The topic emits its own
	// "pubsub event published" INFO line and dispatches asynchronously.
	// Ignore the topic's nil-error return — the contract is that publish
	// always succeeds at enqueue, with subscriber outcomes surfaced via
	// dispatch log lines.
	_ = b.topic.Publish(ctx, pe)

	return eventID, nil
}

// Subscribe registers s on the bus's underlying topic. Convenience
// passthrough so callers don't need to hold a separate reference to the
// topic. Returns the same Unsubscribe / error semantics as
// pubsub.Topic.Subscribe.
func (b *InProcessBus) Subscribe(s Subscriber, opts ...pubsub.SubscribeOption) (pubsub.Unsubscribe, error) {
	return b.topic.Subscribe(s, opts...)
}

// Drain is a passthrough to the underlying topic for tests that need to
// wait for in-flight dispatches.
func (b *InProcessBus) Drain(ctx context.Context) error {
	return b.topic.Drain(ctx)
}
