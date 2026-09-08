package pubsub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// withTestRequestID returns a context carrying the given request_id. Used to
// verify that dispatch goroutines preserve the originating RPC's correlation
// ID.
func withTestRequestID(t *testing.T, id string) context.Context {
	t.Helper()
	return logging.WithRequestID(context.Background(), id)
}

func TestMemTopic_PublishFansOutToAllSubscribers(t *testing.T) {
	topic := NewMemTopic[int]("test")

	subA := NewFakeSubscriber[int]("a")
	subB := NewFakeSubscriber[int]("b")

	unsubA, err := topic.Subscribe(subA)
	if err != nil {
		t.Fatalf("Subscribe(a): %v", err)
	}
	defer unsubA()

	unsubB, err := topic.Subscribe(subB)
	if err != nil {
		t.Fatalf("Subscribe(b): %v", err)
	}
	defer unsubB()

	for _, n := range []int{1, 2, 3} {
		if err := topic.Publish(context.Background(), n); err != nil {
			t.Fatalf("Publish(%d): %v", n, err)
		}
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if got := subA.Received(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("subA received = %v, want [1 2 3]", got)
	}
	if got := subB.Received(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("subB received = %v, want [1 2 3]", got)
	}
}

func TestMemTopic_OneSubscriberFailureDoesNotPoisonOthers(t *testing.T) {
	topic := NewMemTopic[int]("test")

	bad := NewFakeSubscriber[int]("bad")
	bad.HandleErr = errors.New("boom")

	good := NewFakeSubscriber[int]("good")

	unsubBad, err := topic.Subscribe(bad)
	if err != nil {
		t.Fatalf("Subscribe(bad): %v", err)
	}
	defer unsubBad()

	unsubGood, err := topic.Subscribe(good)
	if err != nil {
		t.Fatalf("Subscribe(good): %v", err)
	}
	defer unsubGood()

	if err := topic.Publish(context.Background(), 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if got := good.Received(); len(got) != 1 || got[0] != 42 {
		t.Errorf("good subscriber received = %v, want [42]", got)
	}
	// The bad subscriber DID see the event — its handler was invoked. The
	// invariant under test is just that the bad subscriber's returned
	// error didn't prevent the good subscriber's dispatch.
	if got := bad.Received(); len(got) != 1 {
		t.Errorf("bad subscriber should still have seen the event (Handle was called), got %v", got)
	}
}

func TestMemTopic_PanicInHandlerIsContained(t *testing.T) {
	topic := NewMemTopic[int]("test")

	panicker := NewFakeSubscriber[int]("panicker")
	panicker.HandlePanic = "intentional"

	other := NewFakeSubscriber[int]("other")

	unsub1, _ := topic.Subscribe(panicker)
	defer unsub1()
	unsub2, _ := topic.Subscribe(other)
	defer unsub2()

	if err := topic.Publish(context.Background(), 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := topic.Publish(context.Background(), 2); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Panicker should not have crashed the worker — second event reaches
	// neither (Received() empty for panicker because it never returns
	// successfully) but the OTHER subscriber gets both events.
	if got := other.Received(); len(got) != 2 {
		t.Errorf("other subscriber received %d events, want 2 (panicker should not block sibling)", len(got))
	}
}

func TestMemTopic_SubscriberWithEmptyNameRejected(t *testing.T) {
	topic := NewMemTopic[int]("test")
	sub := NewFakeSubscriber[int]("")
	if _, err := topic.Subscribe(sub); err == nil {
		t.Error("Subscribe with empty Name() should error")
	}
}

func TestMemTopic_DuplicateSubscriberNameRejected(t *testing.T) {
	topic := NewMemTopic[int]("test")
	a1 := NewFakeSubscriber[int]("dup")
	a2 := NewFakeSubscriber[int]("dup")

	unsub1, err := topic.Subscribe(a1)
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	defer unsub1()

	if _, err := topic.Subscribe(a2); err == nil {
		t.Error("second Subscribe with duplicate name should error")
	}
}

func TestMemTopic_UnsubscribeStopsFutureDelivery(t *testing.T) {
	topic := NewMemTopic[int]("test")
	sub := NewFakeSubscriber[int]("s")

	unsub, _ := topic.Subscribe(sub)

	if err := topic.Publish(context.Background(), 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	unsub()
	// Idempotent — second call must not panic.
	unsub()

	if err := topic.Publish(context.Background(), 2); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	drainCtx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if err := topic.Drain(drainCtx2); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if got := sub.Received(); len(got) != 1 || got[0] != 1 {
		t.Errorf("after unsubscribe, received = %v, want [1]", got)
	}
}

// blockingSubscriber holds the dispatch worker on the first event until
// released, then proceeds normally. Used to drive deterministic
// backpressure / detachment tests without time-based synchronization.
type blockingSubscriber struct {
	NameVal string

	mu       sync.Mutex
	received []int

	started chan struct{} // signaled once after the first Handle entry
	release chan struct{} // close to release the held handler
	once    sync.Once
}

func newBlockingSubscriber(name string) *blockingSubscriber {
	return &blockingSubscriber{
		NameVal: name,
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (b *blockingSubscriber) Name() string { return b.NameVal }

func (b *blockingSubscriber) Handle(ctx context.Context, event int) error {
	// First call signals "started" and blocks on release. Subsequent calls
	// pass through immediately so a second event picked up after release
	// runs to completion.
	b.once.Do(func() {
		b.started <- struct{}{}
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	})
	b.mu.Lock()
	b.received = append(b.received, event)
	b.mu.Unlock()
	return nil
}

func (b *blockingSubscriber) Received() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int, len(b.received))
	copy(out, b.received)
	return out
}

func TestMemTopic_BackpressureDropsWhenIngressFull(t *testing.T) {
	topic := NewMemTopic[int]("test")

	// Capacity 1 + handler that holds the worker on the first event lets
	// us deterministically saturate the channel: first event held in
	// Handle, second sits in the buffer, third drops.
	sub := newBlockingSubscriber("blocking")
	unsub, err := topic.Subscribe(sub, WithIngressBufferSize(1))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	// First publish — worker dequeues, Handle blocks on release.
	if err := topic.Publish(context.Background(), 1); err != nil {
		t.Fatalf("Publish 1: %v", err)
	}
	// Wait until the worker actually entered Handle (so the buffer is
	// drained back to empty before we fill it).
	<-sub.started

	// Second publish — fits in the now-empty buffer (cap=1).
	if err := topic.Publish(context.Background(), 2); err != nil {
		t.Fatalf("Publish 2: %v", err)
	}
	// Third publish — buffer is full, this one should drop.
	if err := topic.Publish(context.Background(), 3); err != nil {
		t.Fatalf("Publish 3: %v", err)
	}

	close(sub.release)

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 2 {
		t.Errorf("after backpressure drop, received %d events, want 2 (3 was dropped)", len(got))
	}
	if len(got) >= 1 && got[0] != 1 {
		t.Errorf("first event = %d, want 1", got[0])
	}
	if len(got) >= 2 && got[1] != 2 {
		t.Errorf("second event = %d, want 2", got[1])
	}
}

func TestMemTopic_DispatchCtxPreservesRequestID(t *testing.T) {
	topic := NewMemTopic[int]("test")

	const wantRID = "req-abc123"
	gotRID := make(chan string, 1)

	sub := NewFakeSubscriber[int]("rid-checker")
	sub.HandleHook = func(ctx context.Context, _ int) {
		gotRID <- logging.RequestIDFromContext(ctx)
	}

	unsub, _ := topic.Subscribe(sub)
	defer unsub()

	publishCtx := withTestRequestID(t, wantRID)
	if err := topic.Publish(publishCtx, 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-gotRID:
		if got != wantRID {
			t.Errorf("dispatch ctx request_id = %q, want %q", got, wantRID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch")
	}
}

func TestMemTopic_DispatchCtxDetachedFromPublishCtxLifetime(t *testing.T) {
	topic := NewMemTopic[int]("test")

	// Sequence:
	//   1. Publish under a cancellable parent ctx.
	//   2. Worker picks up the event, signals started, waits on release.
	//   3. Cancel publish ctx.
	//   4. Release the held handler; it observes its own ctx.Err() and
	//      reports back. The dispatch ctx must NOT be cancelled — it was
	//      derived with a fresh deadline, not inherited from publish.
	type observation struct {
		errAfterPublishCancel error
	}
	obs := make(chan observation, 1)
	started := make(chan struct{})
	release := make(chan struct{})

	sub := NewFakeSubscriber[int]("detached")
	sub.HandleHook = func(ctx context.Context, _ int) {
		started <- struct{}{}
		<-release
		obs <- observation{errAfterPublishCancel: ctx.Err()}
	}

	unsub, _ := topic.Subscribe(sub)
	defer unsub()

	publishCtx, cancel := context.WithCancel(context.Background())
	if err := topic.Publish(publishCtx, 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	<-started
	// Cancel the publish ctx now that we know the worker is inside Handle.
	cancel()

	// The dispatch ctx is a separate context; cancelling publish must not
	// affect the worker's view of its own ctx.
	close(release)

	select {
	case got := <-obs:
		if got.errAfterPublishCancel != nil {
			t.Errorf("dispatch ctx had err=%v after publish ctx cancel; expected nil (detached)", got.errAfterPublishCancel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to report back")
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer drainCancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestMemTopic_DrainReturnsCtxErrOnTimeout(t *testing.T) {
	topic := NewMemTopic[int]("test")

	// Hold the worker longer than the drain timeout via HandleDelay.
	sub := NewFakeSubscriber[int]("slow")
	sub.HandleDelay = 500 * time.Millisecond

	unsub, _ := topic.Subscribe(sub)
	defer unsub()

	if err := topic.Publish(context.Background(), 1); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := topic.Drain(drainCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Drain after timeout returned %v, want DeadlineExceeded", err)
	}

	// Wait for the slow handler to finish so the test's goroutine count
	// settles before the next test runs.
	finalDrainCtx, finalCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer finalCancel()
	_ = topic.Drain(finalDrainCtx)
}

// eventWithLogContext exercises the LogContextProvider hook. Used to verify
// the event's structured fields appear on dispatch log lines.
type eventWithLogContext struct {
	id       string
	category string
}

func (e eventWithLogContext) LogContext() []any {
	return []any{"event_id_test", e.id, "category", e.category}
}

func TestMemTopic_LogContextProviderFieldsFlowThrough(t *testing.T) {
	// We can't easily intercept slog handler output from this test without
	// pulling in a log capture harness; this test mostly proves the type
	// assertion path compiles + runs without panicking. Functional
	// assertions on log output happen in the integration tests for
	// community_event_bus where the expected fields are concrete.
	topic := NewMemTopic[eventWithLogContext]("test")

	sub := NewFakeSubscriber[eventWithLogContext]("s")
	unsub, _ := topic.Subscribe(sub)
	defer unsub()

	if err := topic.Publish(context.Background(), eventWithLogContext{id: "e1", category: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if got := sub.Received(); len(got) != 1 || got[0].id != "e1" {
		t.Errorf("received = %v, want one event with id=e1", got)
	}
}

func TestMemTopic_OrderingWithinSubscriber(t *testing.T) {
	topic := NewMemTopic[int]("test")

	const N = 50
	var seen []int
	var seenMu sync.Mutex

	sub := NewFakeSubscriber[int]("ordered")
	sub.HandleHook = func(_ context.Context, n int) {
		seenMu.Lock()
		seen = append(seen, n)
		seenMu.Unlock()
	}

	unsub, _ := topic.Subscribe(sub)
	defer unsub()

	for i := range N {
		if err := topic.Publish(context.Background(), i); err != nil {
			t.Fatalf("Publish(%d): %v", i, err)
		}
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seen) != N {
		t.Fatalf("received %d events, want %d", len(seen), N)
	}
	for i := range N {
		if seen[i] != i {
			t.Errorf("event %d arrived out of order: got %d, want %d", i, seen[i], i)
			break
		}
	}
}

func TestMemTopic_ConcurrentPublishersAreSafe(t *testing.T) {
	topic := NewMemTopic[int]("test")

	const Publishers = 4
	const PerPublisher = 25

	var totalReceived atomic.Int64
	sub := NewFakeSubscriber[int]("counter")
	sub.HandleHook = func(_ context.Context, _ int) {
		totalReceived.Add(1)
	}

	unsub, _ := topic.Subscribe(sub, WithIngressBufferSize(Publishers*PerPublisher))
	defer unsub()

	var wg sync.WaitGroup
	for p := range Publishers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range PerPublisher {
				_ = topic.Publish(context.Background(), p*PerPublisher+i)
			}
		}()
	}
	wg.Wait()

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := topic.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if got := totalReceived.Load(); got != int64(Publishers*PerPublisher) {
		t.Errorf("received %d events, want %d", got, Publishers*PerPublisher)
	}
}

func TestMockTopic_CapturesPublishesAndRespectsError(t *testing.T) {
	mock := NewMockTopic[string]()
	if err := mock.Publish(context.Background(), "hello"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	mock.SetPublishError(fmt.Errorf("forced"))
	if err := mock.Publish(context.Background(), "world"); err == nil || !strings.Contains(err.Error(), "forced") {
		t.Errorf("Publish after SetPublishError = %v, want forced error", err)
	}

	got := mock.Captured()
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("captured = %v, want [hello world]", got)
	}

	mock.Reset()
	if got := mock.Captured(); len(got) != 0 {
		t.Errorf("after Reset, captured = %v, want []", got)
	}
}
