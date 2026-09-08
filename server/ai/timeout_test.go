package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallWithTimeout_FastSuccess(t *testing.T) {
	got, err := CallWithTimeout(context.Background(), time.Second, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestCallWithTimeout_ExceedsDeadline(t *testing.T) {
	_, err := CallWithTimeout(context.Background(), 10*time.Millisecond, func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return 99, nil
		}
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestCallWithTimeout_ParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond) //nolint:forbidigo // simulates async parent cancellation
		cancel()
	}()
	_, err := CallWithTimeout(parent, time.Second, func(ctx context.Context) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return 99, nil
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected Canceled, got %v", err)
	}
}

func TestCallWithTimeout_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	_, err := CallWithTimeout(context.Background(), time.Second, func(ctx context.Context) (int, error) {
		return 0, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
