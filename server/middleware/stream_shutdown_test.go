package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
)

func TestStreamShutdownInterceptor_EndsStreamsOnShutdown(t *testing.T) {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	defer beginShutdown()

	started := make(chan struct{})
	handler := StreamShutdownInterceptor(shutdown).WrapStreamingHandler(
		func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			close(started)
			<-ctx.Done() // a long-lived stream: ends only when its context does
			return nil
		},
	)

	done := make(chan error, 1)
	go func() { done <- handler(context.Background(), nil) }()
	<-started

	beginShutdown()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stream ended with %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream still open 5s after shutdown began")
	}
}

func TestStreamShutdownInterceptor_LeavesStreamsAloneUntilShutdown(t *testing.T) {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	defer beginShutdown()

	handler := StreamShutdownInterceptor(shutdown).WrapStreamingHandler(
		func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			select {
			case <-ctx.Done():
				return errors.New("stream cancelled without a shutdown")
			case <-time.After(50 * time.Millisecond):
				return nil
			}
		},
	)

	if err := handler(context.Background(), nil); err != nil {
		t.Error(err)
	}
}

func TestStreamShutdownInterceptor_StreamStartedAfterShutdownEndsAtOnce(t *testing.T) {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	beginShutdown()

	handler := StreamShutdownInterceptor(shutdown).WrapStreamingHandler(
		func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				return errors.New("stream opened during shutdown was not ended")
			}
		},
	)

	if err := handler(context.Background(), nil); err != nil {
		t.Error(err)
	}
}

func TestStreamShutdownInterceptor_UnaryUntouched(t *testing.T) {
	shutdown, beginShutdown := context.WithCancel(context.Background())
	beginShutdown()

	unary := StreamShutdownInterceptor(shutdown).WrapUnary(
		func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			if ctx.Err() != nil {
				return nil, errors.New("unary context cancelled by shutdown; it should drain")
			}
			return nil, nil
		},
	)

	if _, err := unary(context.Background(), nil); err != nil {
		t.Error(err)
	}
}
