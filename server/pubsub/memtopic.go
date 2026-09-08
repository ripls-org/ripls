package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// MemTopic is the in-memory implementation of Topic[T]. Each registered
// subscriber gets its own goroutine and bounded ingress channel; Publish
// fans out non-blockingly. Subscribers consume serially.
type MemTopic[T any] struct {
	name string

	mu          sync.RWMutex
	subscribers map[string]*subscriberState[T]

	// inflight tracks dispatches currently executing across all subscribers
	// so Drain can wait for them. Incremented on enqueue acceptance,
	// decremented after Handle returns (or panics).
	inflight sync.WaitGroup
}

type subscriberState[T any] struct {
	sub     Subscriber[T]
	opts    subscribeOptions
	ingress chan dispatchEnvelope[T]
	stop    chan struct{}
}

// dispatchEnvelope carries the publisher's context alongside the event so the
// dispatcher goroutine can honour its deadline and cancellation. An async
// envelope has nowhere else to put it.
//
//nolint:containedctx // see above
type dispatchEnvelope[T any] struct {
	ctx   context.Context
	event T
}

// NewMemTopic constructs a topic with the given name. The name appears as the
// "topic" log field on every emitted line and should be stable across
// deployments (e.g. "community_events").
func NewMemTopic[T any](name string) *MemTopic[T] {
	return &MemTopic[T]{
		name:        name,
		subscribers: make(map[string]*subscriberState[T]),
	}
}

// Name returns the topic name.
func (t *MemTopic[T]) Name() string {
	return t.name
}

// Subscribe registers s on this topic. Returns an error if a subscriber with
// the same Name() is already registered.
func (t *MemTopic[T]) Subscribe(s Subscriber[T], opts ...SubscribeOption) (Unsubscribe, error) {
	if s == nil {
		return nil, errors.New("pubsub: nil subscriber")
	}
	name := s.Name()
	if name == "" {
		return nil, errors.New("pubsub: subscriber Name() must be non-empty")
	}

	resolved := resolveOptions(opts)
	state := &subscriberState[T]{
		sub:     s,
		opts:    resolved,
		ingress: make(chan dispatchEnvelope[T], resolved.ingressBufferSize),
		stop:    make(chan struct{}),
	}

	t.mu.Lock()
	if _, exists := t.subscribers[name]; exists {
		t.mu.Unlock()
		return nil, fmt.Errorf("pubsub: subscriber %q already registered on topic %q", name, t.name)
	}
	t.subscribers[name] = state
	t.mu.Unlock()

	// Start the per-subscriber worker goroutine. logging.GoSafe wraps it
	// in panic recovery; the worker has its own per-dispatch defer/recover
	// for outcome logging.
	logging.GoSafe(context.Background(), "pubsub-worker-"+name, func() {
		t.runWorker(state)
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			delete(t.subscribers, name)
			t.mu.Unlock()
			close(state.stop)
		})
	}, nil
}

// Publish enqueues event into every subscribed ingress. A subscriber whose
// channel is full has its event dropped with a WARN log. Publish returns
// nil; backpressure drops are observable only via the dispatched-line log
// stream.
func (t *MemTopic[T]) Publish(ctx context.Context, event T) error {
	publishLogger := logging.LoggerWithContext(ctx)

	publishFields := []any{
		"operation", "pubsub.publish",
		"topic", t.name,
	}
	if lcp, ok := any(event).(LogContextProvider); ok {
		publishFields = append(publishFields, lcp.LogContext()...)
	}
	publishLogger.InfoContext(ctx, "pubsub event published", publishFields...)

	t.mu.RLock()
	subs := make([]*subscriberState[T], 0, len(t.subscribers))
	for _, s := range t.subscribers {
		subs = append(subs, s)
	}
	t.mu.RUnlock()

	for _, s := range subs {
		t.enqueue(ctx, s, event)
	}
	return nil
}

// enqueue tries to push the event onto subscriber s's ingress channel.
// On full, drops with a WARN log line. Increments inflight on success so
// Drain can track completion.
func (t *MemTopic[T]) enqueue(ctx context.Context, s *subscriberState[T], event T) {
	select {
	case <-s.stop:
		// Subscriber unsubscribed concurrently; skip silently.
		return
	default:
	}

	t.inflight.Add(1)
	select {
	case s.ingress <- dispatchEnvelope[T]{ctx: ctx, event: event}:
		// Enqueued. Worker decrements inflight after Handle returns.
	default:
		// Ingress full — drop and log.
		t.inflight.Done()
		t.logDispatchDrop(ctx, s, event)
	}
}

// runWorker is the per-subscriber dispatch loop. Reads envelopes from
// ingress, derives the dispatch ctx, calls Handle, logs the outcome.
func (t *MemTopic[T]) runWorker(s *subscriberState[T]) {
	for {
		select {
		case <-s.stop:
			return
		case env, ok := <-s.ingress:
			if !ok {
				return
			}
			t.dispatchOne(s, env)
		}
	}
}

// Dispatch outcomes, emitted as the `outcome` field on the dispatch log line
// and used to pick its level. Log-based metrics key off these values
// (docs/server/observability.md), so they are a wire contract, not free text.
const (
	outcomeSuccess = "success"
	outcomeFailure = "failure"
)

// dispatchOne handles a single envelope: derives the dispatch context,
// invokes Handle with panic recovery, and emits the outcome log line.
func (t *MemTopic[T]) dispatchOne(s *subscriberState[T], env dispatchEnvelope[T]) {
	defer t.inflight.Done()

	dispatchCtx, cancel := deriveDispatchCtx(env.ctx, s.opts.subscriberTimeout)
	defer cancel()

	start := time.Now()
	var (
		outcome  = outcomeSuccess
		reason   string
		errMsg   string
		panicVal any
		panicked bool
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
				outcome = outcomeFailure
				reason = "panic"
				errMsg = fmt.Sprintf("%v", r)
			}
		}()
		if err := s.sub.Handle(dispatchCtx, env.event); err != nil {
			outcome = outcomeFailure
			errMsg = err.Error()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				reason = "ctx_canceled"
			}
		}
	}()

	t.logDispatchOutcome(env.ctx, s, env.event, outcome, reason, errMsg, time.Since(start))

	if panicked {
		// Panics are surfaced via the dispatch line at ERROR level above;
		// we deliberately do NOT re-panic so the GoSafe outer recover stays
		// the safety net rather than the primary log surface. _ is here to
		// keep panicVal referenced (avoids "unused" warning if the variable
		// is ever consumed by future tracing wiring).
		_ = panicVal
	}
}

// deriveDispatchCtx returns a context detached from the publish ctx's
// lifetime (so a cancelled HTTP request doesn't kill in-flight dispatch)
// but preserves request_id and user_id so fan-out log lines stay
// joinable to the originating RPC. Same pattern as
// services/chat/streaming.go#broadcastMessage.
func deriveDispatchCtx(publishCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if rid := logging.RequestIDFromContext(publishCtx); rid != "" {
		base = logging.WithRequestID(base, rid)
	}
	if uid := logging.UserIDFromContext(publishCtx); uid != "" {
		base = logging.WithUserID(base, uid)
	}
	return context.WithTimeout(base, timeout)
}

// logDispatchOutcome emits the per-dispatch log line at the level appropriate
// for the outcome.
func (t *MemTopic[T]) logDispatchOutcome(
	publishCtx context.Context,
	s *subscriberState[T],
	event T,
	outcome, reason, errMsg string,
	dur time.Duration,
) {
	level := slog.LevelInfo
	switch outcome {
	case outcomeFailure:
		switch reason {
		case "panic":
			level = slog.LevelError
		case "ctx_canceled":
			level = slog.LevelInfo
		default:
			level = slog.LevelWarn
		}
	case "dropped":
		level = slog.LevelWarn
	}

	fields := []any{
		"operation", "pubsub.dispatch",
		"topic", t.name,
		"subscriber", s.sub.Name(),
		"duration_ms", dur.Milliseconds(),
		"outcome", outcome,
	}
	if reason != "" {
		fields = append(fields, "reason", reason)
	}
	if errMsg != "" {
		fields = append(fields, "error", errMsg)
	}
	if lcp, ok := any(event).(LogContextProvider); ok {
		fields = append(fields, lcp.LogContext()...)
	}

	// Log under the publish ctx so request_id / user_id propagate.
	logging.LoggerWithContext(publishCtx).LogAttrs(publishCtx, level,
		"pubsub event dispatched", toAttrs(fields)...)
}

// logDispatchDrop is the backpressure-drop variant of logDispatchOutcome.
// Separate function to keep the hot enqueue path free of the timing
// machinery that the success path uses.
func (t *MemTopic[T]) logDispatchDrop(publishCtx context.Context, s *subscriberState[T], event T) {
	t.logDispatchOutcome(publishCtx, s, event, "dropped", "ingress_full", "", 0)
}

// Drain blocks until every in-flight dispatch finishes or ctx is done.
// Tests call this; production never does.
func (t *MemTopic[T]) Drain(ctx context.Context) error {
	done := make(chan struct{})
	logging.GoSafe(ctx, "pubsub-drain-wait-"+t.name, func() {
		t.inflight.Wait()
		close(done)
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// toAttrs converts an alternating key/value []any into []slog.Attr for
// LogAttrs. We use LogAttrs (not Log) so the attributes flow through slog
// without an intermediate map allocation.
func toAttrs(kv []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		out = append(out, slog.Any(key, kv[i+1]))
	}
	return out
}
