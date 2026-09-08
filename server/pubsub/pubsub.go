package pubsub

import (
	"context"
	"time"
)

// Topic is the publish/subscribe surface for events of type T. In-memory
// implementations live in this package (see MemTopic); durable implementations
// may live in server/storage or a sibling package and satisfy the same
// interface.
type Topic[T any] interface {
	// Subscribe registers s to receive events from this topic. The returned
	// Unsubscribe is idempotent. Calling Subscribe with a subscriber whose
	// Name() collides with an already-registered one returns an error.
	Subscribe(s Subscriber[T], opts ...SubscribeOption) (Unsubscribe, error)

	// Publish enqueues event into every registered subscriber's ingress
	// channel. Returns nil on successful enqueue; subscribers whose ingress
	// channel is full have their event dropped with a WARN log, but that
	// drop is not surfaced to the caller.
	Publish(ctx context.Context, event T) error

	// Drain blocks until every in-flight dispatch on every subscriber
	// completes or ctx is done. Used by tests; production never calls
	// Drain.
	Drain(ctx context.Context) error
}

// Subscriber consumes events of type T. Implementations MUST be idempotent:
// the same event may be delivered more than once if the topic is later
// upgraded to a durable transport.
type Subscriber[T any] interface {
	// Name is a stable identifier used in logs. Must be unique within a
	// single Topic.
	Name() string

	// Handle processes a single event. Errors are logged and counted by
	// the log-based metric stream but do NOT trigger retries in MemTopic.
	// Subscribers that need retries opt into a durable Topic implementation.
	Handle(ctx context.Context, event T) error
}

// Unsubscribe removes a previously-registered subscriber. Idempotent.
type Unsubscribe func()

// SubscribeOption configures per-subscriber behavior on a topic.
type SubscribeOption func(*subscribeOptions)

type subscribeOptions struct {
	ingressBufferSize int
	subscriberTimeout time.Duration
}

// DefaultIngressBufferSize is the per-subscriber bounded-channel size used
// when WithIngressBufferSize is not supplied.
const DefaultIngressBufferSize = 256

// DefaultSubscriberTimeout is the per-dispatch deadline used when
// WithSubscriberTimeout is not supplied. Mirrors the historical 30-second
// notification timeout from community.RecordCommunityEventAndNotify so the
// notification subscriber's behavior is preserved across the migration.
const DefaultSubscriberTimeout = 30 * time.Second

// WithIngressBufferSize overrides the per-subscriber ingress channel size.
// Smaller values produce more aggressive backpressure (drops); larger values
// trade memory for resilience to short bursts.
func WithIngressBufferSize(n int) SubscribeOption {
	return func(o *subscribeOptions) { o.ingressBufferSize = n }
}

// WithSubscriberTimeout overrides the per-dispatch deadline applied to the
// subscriber's Handle call.
func WithSubscriberTimeout(d time.Duration) SubscribeOption {
	return func(o *subscribeOptions) { o.subscriberTimeout = d }
}

func resolveOptions(opts []SubscribeOption) subscribeOptions {
	o := subscribeOptions{
		ingressBufferSize: DefaultIngressBufferSize,
		subscriberTimeout: DefaultSubscriberTimeout,
	}
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

// LogContextProvider is implemented by event types that want to contribute
// extra structured fields to pubsub log lines. Implementing this is optional;
// events that don't implement it get only the topic-level log fields.
//
// The returned slice is a slog-style alternating key/value list (e.g.
// []any{"event_type", "...", "community_event_id", "..."}). Keep field names
// aligned with docs/server/observability.md § "Standard Field Names".
type LogContextProvider interface {
	LogContext() []any
}
