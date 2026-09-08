package integration_tests

import (
	"testing"
	"time"
)

// TestStreamReceiveTimeout_SelectDefault demonstrates the broken pattern that
// caused TestServer_Integration_ChatMessaging to hang for 20 minutes in CI.
//
// The pattern: a select with a timeout case and a default case that calls a
// blocking function. Go's select evaluates once — it picks default immediately
// (since the timeout channel isn't ready), then the blocking call in default
// prevents the select from ever re-evaluating. The timeout case never fires.
func TestStreamReceiveTimeout_SelectDefault(t *testing.T) {
	// Simulate a stream that blocks after delivering buffered messages.
	stream := make(chan string, 2)
	stream <- "msg1"
	stream <- "msg2"
	// No more messages — the next read blocks forever.

	// The BROKEN pattern: select/default with a blocking read.
	// This must be detected by a wrapper timeout, not the internal one.
	detected := make(chan bool, 1)
	go func() {
		timeout := time.After(50 * time.Millisecond)
		drained := 0
		for {
			select {
			case <-timeout:
				detected <- true
				return
			default:
				// This blocks on the 3rd iteration, starving the timeout case.
				<-stream
				drained++
				if drained > 10 {
					detected <- false
					return
				}
			}
		}
	}()

	// The goroutine should time out in 50ms, but the broken pattern blocks it.
	// If it hasn't returned in 500ms, the pattern is confirmed broken.
	select {
	case ok := <-detected:
		if ok {
			t.Fatal("select/default unexpectedly worked — Go semantics may have changed")
		}
	case <-time.After(500 * time.Millisecond):
		// Expected: the goroutine is stuck in <-stream inside default.
		// This confirms the bug.
	}
}

// TestStreamReceiveTimeout_GoroutineChannel demonstrates the correct pattern:
// receive in a goroutine, forward to a channel, and select on that channel
// alongside a timeout. The timeout fires reliably because select only waits
// on channels, never on blocking function calls.
func TestStreamReceiveTimeout_GoroutineChannel(t *testing.T) {
	// Simulate a stream that blocks after delivering buffered messages.
	stream := make(chan string, 2)
	stream <- "msg1"
	stream <- "msg2"

	// The CORRECT pattern: goroutine forwards to a channel.
	msgCh := make(chan string, 10)
	go func() {
		for msg := range stream {
			msgCh <- msg
		}
	}()

	// Drain the 2 buffered messages.
	for range 2 {
		select {
		case <-msgCh:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout draining buffered messages")
		}
	}

	// Now wait for a message that will never come — timeout must fire.
	select {
	case msg := <-msgCh:
		t.Fatalf("Unexpected message: %s", msg)
	case <-time.After(100 * time.Millisecond):
		// Correct: timeout fires because select is only waiting on channels.
	}
}

// TestStreamReceiveTimeout_GoroutineChannelReceivesNewMessage verifies
// the correct pattern also delivers new messages that arrive after draining.
func TestStreamReceiveTimeout_GoroutineChannelReceivesNewMessage(t *testing.T) {
	stream := make(chan string, 2)
	stream <- "existing1"
	stream <- "existing2"

	msgCh := make(chan string, 10)
	go func() {
		for msg := range stream {
			msgCh <- msg
		}
	}()

	// Drain existing messages.
	for range 2 {
		select {
		case <-msgCh:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout draining buffered messages")
		}
	}

	// Send a new message after a short delay (simulating a broadcast).
	go func() {
		time.Sleep(20 * time.Millisecond) //nolint:forbidigo // simulates broadcast arriving after a real-world delay
		stream <- "new_message"
	}()

	// The new message should arrive within the timeout.
	select {
	case msg := <-msgCh:
		if msg != "new_message" {
			t.Errorf("Expected 'new_message', got %q", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for new message — goroutine+channel pattern is broken")
	}
}
