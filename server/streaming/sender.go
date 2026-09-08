// Generic serializing sender for Connect server-streaming RPCs. Emitters
// from arbitrary goroutines push events through a buffered channel; a
// single writer goroutine drains the channel and calls target.Send, so
// concurrent emits do not violate Connect's one-Send-at-a-time contract.
//
// The sender takes a minimal SendTarget interface — Connect's
// *ServerStream[T] satisfies it natively, and tests supply a capture
// target implementing the same Send(ev *T) error signature.
package streaming

import (
	"fmt"
	"runtime/debug"
	"sync"

	"go.ripls.org/ripls/server/logging"
)

// SendTarget is the minimal surface the sender needs from a Connect
// ServerStream. Tests inject a capture target implementing the same shape.
type SendTarget[T any] interface {
	Send(ev *T) error
}

// Sender serializes outbound events from arbitrary goroutines onto a
// single Send-capable target. Emit from any goroutine via Emit; close
// exactly once at the end of the handler to flush pending events and
// collect the first Send error observed (nil if all sends succeeded).
type Sender[T any] struct {
	ch   chan *T
	done chan error
	once sync.Once
}

// NewSender constructs a Sender bound to target. Starts one writer
// goroutine that drains the queue until Close is called.
func NewSender[T any](target SendTarget[T]) *Sender[T] {
	s := &Sender[T]{
		ch:   make(chan *T, 4),
		done: make(chan error, 1),
	}
	go func() {
		// GoSafe is not used here because on panic we must still send to s.done
		// to prevent Close() from blocking forever. Inline recovery handles both
		// the log and the unblock.
		defer func() {
			if r := recover(); r != nil {
				logging.Default().Error("panic in streaming sender goroutine",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				s.done <- fmt.Errorf("panic in streaming sender: %v", r)
			}
		}()
		var firstErr error
		for ev := range s.ch {
			if firstErr != nil {
				continue // drain remaining events after a send failure
			}
			if err := target.Send(ev); err != nil {
				firstErr = err
			}
		}
		s.done <- firstErr
	}()
	return s
}

// Emit queues an event for the writer goroutine. Safe to call from any
// goroutine. Blocks only if the buffered channel is full (unlikely in
// practice — event rate is measured in tens per streaming call).
func (s *Sender[T]) Emit(ev *T) {
	s.ch <- ev
}

// Close flushes pending events and returns the first Send error observed.
// Idempotent — subsequent calls return the same error without re-closing
// the channel. Call exactly once at the end of the streaming handler
// (typically via defer).
func (s *Sender[T]) Close() error {
	s.once.Do(func() { close(s.ch) })
	return <-s.done
}
