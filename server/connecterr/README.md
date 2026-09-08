# Connect Error Helpers

This package builds [Connect](https://connectrpc.com) RPC errors that log
internal detail server-side without leaking it to clients.

## Why this exists

Connect serializes a returned error's `Error()` string into the response
body. If the error is a raw value from storage, a third-party SDK, or a JSON
decoder, the client sees whatever the underlying library produced —
Postgres constraint violations (`pq: duplicate key value violates unique
constraint "users_email_key"`), JSON decoder messages with offsets,
third-party API bodies, request values reflected back in error text, and
so on. That's both an information-disclosure problem and a source of
high-cardinality, low-signal noise in client crash reporting.

This package replaces the bare pattern

```go
return connect.NewError(connect.CodeInternal, err)
```

with a single helper that logs the error server-side and returns a
`CodeInternal` whose public message is a fixed generic string.

## Usage

```go
import "go.ripls.org/ripls/server/connecterr"

func (s *Service) SaveGear(
    ctx context.Context,
    req *connect.Request[api.SaveGearRequest],
) (*connect.Response[api.SaveGearResponse], error) {
    gear, err := s.storage.GetByID(ctx, req.Msg.Id)
    if err != nil {
        return nil, connecterr.Internal(ctx, "SaveGear.GetByID", err,
            "gear_id", req.Msg.Id,
        )
    }
    // ...
}
```

The helper:

- Logs the wrapped error at `Error` level via
  `logging.LoggerWithContext(ctx)`, with `operation` set to the `op`
  argument and `error` set to the wrapped err. Trailing key/value pairs
  flow through to the structured log as additional fields.
- Returns a `connect.CodeInternal` error whose public `Error()` is the
  fixed string `"internal server error"`. The original err is never
  serialized to the wire.
- Treats a nil err as a programming bug: logs a `Warn`-level entry with
  a stack trace and still returns a valid `CodeInternal` so the caller
  contract holds. Does not panic.

## Conventions

- `op` should identify the RPC method (or method-step). Match the
  `operation` field convention in
  [`docs/server/observability.md`](../../docs/server/observability.md).
- Per-call detail (entity IDs, request IDs, retry counts) belongs in the
  trailing `kv` pairs, not in the public message.
- Don't pre-log the same error before calling `Internal` — the helper
  owns that log line. Delete redundant `logger.ErrorContext(...)` calls
  that immediately precede the wrap.
- Don't add `fmt.Errorf("…: %w", err)` boilerplate just to "annotate" the
  error before passing it in. Fold the annotation into the `op` argument
  or a `kv` pair so it lands as a structured field.

## When to add code here vs. `server/logging`

This package owns one cross-cutting concern: constructing Connect errors
that don't leak server internals. The `server/logging` package owns
structured logging primitives (handlers, masking, context propagation).

- New helper that wraps a different `connect.Code` (e.g. a future
  `Unavailable`, `FailedPrecondition`)? **Here.**
- New logging primitive (handler, masker, context key)? **`server/logging`.**
- A helper that logs without returning an error? **`server/logging`.**
- A helper that returns an error without logging? **Don't add it.** Every
  internal error worth returning is worth logging.

## Related

- Issue #1340 — the durable review finding this package addresses.
- Plan: [`docs/issues/1340-connecterr-internal-helper.md`](../../docs/issues/1340-connecterr-internal-helper.md).
