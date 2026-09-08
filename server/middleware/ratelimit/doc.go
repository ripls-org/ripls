// Package ratelimit provides a Connect interceptor that enforces
// per-(procedure, key) token-bucket rate limits using
// golang.org/x/time/rate. The interceptor wraps both unary and streaming
// server RPCs, supporting three key dimensions:
//
//   - IP — client IP address, for unauthenticated traffic.
//   - email — extracted from request bodies via GetEmail(), for
//     unauthenticated procedures that carry an email (login, register,
//     password reset). Unary-only: streams have no request body at
//     stream-open time so this dimension is skipped for streaming
//     handlers.
//   - user-id — read from auth.GetAuthInfo(ctx), for authenticated
//     procedures where user-id is more stable than IP and more
//     privacy-respectful than email.
//
// Each Budget may set MetricsOnly = true for a soak-before-enforce mode:
// the limiter still tracks state and the "rate_limited" warn log still
// fires (with mode="metrics_only"), but the request is not actually
// rejected. Use this to characterize burst patterns in production before
// flipping a rule to enforce.
//
// Rejections in enforce mode return connect.CodeResourceExhausted (HTTP
// 429) with a standard HTTP Retry-After header set via the Connect
// error's Meta(). Every rejection (real or would-be) emits a
// "rate_limited" warn log carrying the procedure FQN, key dimension,
// and mode for Cloud Logging queries.
package ratelimit
