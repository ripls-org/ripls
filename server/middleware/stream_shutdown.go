package middleware

import (
	"context"

	"connectrpc.com/connect"
)

// StreamShutdownInterceptor ends every streaming RPC once shutdown is done, by
// cancelling the stream's context. The server's graceful drain waits for open
// requests, and a long-lived stream never finishes by itself: behind a reverse
// proxy that holds it open, the drain runs out its timeout and the process
// outlives its stop grace period. Stream handlers end cleanly on a cancelled
// context, the same retryable end-of-stream as their lifetime cap, so clients
// reconnect. Unary RPCs pass through untouched and still drain.
//
// Pass the context main cancels when shutdown begins, before the drain.
func StreamShutdownInterceptor(shutdown context.Context) connect.Interceptor {
	return streamShutdownInterceptor{
		onShutdown: func(f func()) func() bool { return context.AfterFunc(shutdown, f) },
	}
}

// streamShutdownInterceptor holds a registrar rather than the shutdown context
// itself: it only ever needs to run a callback when shutdown happens.
type streamShutdownInterceptor struct {
	onShutdown func(f func()) (stop func() bool)
}

func (i streamShutdownInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i streamShutdownInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i streamShutdownInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := i.onShutdown(cancel)
		defer stop()
		return next(ctx, conn)
	}
}
