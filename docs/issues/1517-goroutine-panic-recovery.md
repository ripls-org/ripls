# Issue #1517: [error_handling] Add panic recovery to chat notification and AI streaming goroutines

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1517
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:error_handling | **Assignees:** unassigned
**Context docs:** none (issue references `docs/reviews/reports/error_handling_review.md`)

## Problem Statement

Two hot paths still launch bare `go func()` without any `recover()`:

1. `server/services/chat/messages.go:148` — fired on every `SendMessage` RPC to deliver push notifications to non-streaming participants.
2. ~27 goroutines in `server/ai/provider_anthropic_streaming.go`, `server/ai/provider_openai_streaming.go`, and `server/ai/provider_gemini_streaming.go` — fired on every `Gen*Streaming` Provider call (one "producer" per provider that drives the SDK stream + one "consumer" per public `Gen*Streaming` method that unmarshals the terminal value).

Both sites outlive the originating HTTP request, so the `middleware.PanicRecovery` middleware that protects synchronous request handlers cannot help. A panic in any of these — third-party SDK regression, unexpected `nil` from a notification provider, malformed stream terminal — crashes the entire server process.

The reviewer's recommendation is: migrate the chat goroutine to `logging.GoSafe`, and apply the inline-recover shape already used in `server/services/streaming/sender.go:42-54` to every streaming goroutine (because they must signal their `final` / `fields` channels on panic so the consumer doesn't block forever on a closed-but-empty channel that yields a confusing zero-value error). The reviewer additionally asks for a CI gate that flags new raw `go func` outside an explicit allowlist so this gap doesn't reopen.

## Context from Comments

- 2026-04-28 (`maintainer`, triage): set Issue Type = Task, Priority = P1. Rationale: "HTTP middleware doesn't cover detached chat/AI streaming goroutines."
- 2026-04-29 (`maintainer`): `@claude` ping — drives this run.

No technical disagreement in comments; the issue body is the authoritative scope.

## Codebase Findings

**Existing patterns.** The pieces this plan needs already exist:

- `server/logging/logger.go:266-283` — `logging.GoSafe(ctx, name, fn)` wraps a goroutine in `defer recover()` + structured-error logging. Used wherever a fire-and-forget goroutine has no terminal channel to feed.
- `server/services/streaming/sender.go:42-54` — inline `defer recover()` that *both* logs and writes a synthetic error to the terminal `done` channel before close, so a panic doesn't leak as a deadlock. This is the shape the reviewer points at.

**Goroutine inventory** (verified `grep -n 'go func()' …`):

| File | Goroutines | Shape |
|---|---|---|
| `server/services/chat/messages.go` | 1 (line 148) | fire-and-forget, no terminal channel |
| `server/ai/provider_anthropic_streaming.go` | 9 (lines 101, 216, 269, 322, 385, 441, 492, 544, 597) | 1 producer (streamToolCall) + 8 consumers |
| `server/ai/provider_openai_streaming.go` | 9 (lines 103, 205, 262, 323, 404, 470, 524, 592, 649) | 1 producer (streamStructuredOutput) + 8 consumers |
| `server/ai/provider_gemini_streaming.go` | 9 (lines 58, 138, 177, 241, 294, 342, 378, 416, 455) | 1 producer (streamStructuredOutput) + 8 consumers |

The issue body lists 24 line numbers across the AI files — line counts have shifted since the review (some methods were consolidated or moved), but the count is now exactly **3 producers + 24 consumers + 1 chat = 28**, all uniformly shaped within their category. Every consumer follows the identical template (`defer close(final); streamFinal := <-raw; … final <- T{Result: …}`), so the inline-recover insertion is mechanical.

**Lint enforcement constraint.** `forbidigo` operates on the AST at *call-expression* granularity (it matches identifiers like `time.Sleep`); it cannot match the `*ast.GoStmt` keyword. The existing config (`server/.golangci.yml:8-12`) confirms — only call patterns. So the issue's literal "forbidigo rule that flags raw `go func\b`" is not implementable in forbidigo as-is. We need a small purpose-built check; options below.

## Potential Solutions

### Option A: Inline `defer recover()` in every streaming goroutine + `GoSafe` for chat + grep-based CI gate
- **Approach:**
  1. Replace the chat `go func()` with `logging.GoSafe(ctx, "chat-notify-participants", func() { … })`.
  2. Insert a `defer func() { if r := recover(); r != nil { … } }()` as the **last-registered** defer in every streaming goroutine. LIFO means it runs first on panic, *before* the existing `close(fields)` / `close(final)` defers — so it can write a synthetic terminal-error value into the channel before the deferred close fires. Pattern is identical to `sender.go:42-54` but parameterized over the terminal value type via a tiny construction helper (e.g. `panicErr[T any](r any, mk func(error) T) T`).
  3. Add a CI check: a small Go program under `server/cmd/check-goroutines/main.go` that uses `go/parser` + `go/ast` to walk the `server/...` tree, find every `*ast.GoStmt`, and fail if the file is not in an explicit allowlist (`server/logging/logger.go`, `server/services/streaming/sender.go`, plus the touched-up streaming + chat files — once they're verified clean, they don't need the allowlist anymore because the `GoStmt` visitor can also confirm a recover defer is the first statement of the FuncLit body, and skip those). Wire into `npm run lint:go`.
- **Pros:** Faithful to the reviewer's shape recommendation. Keeps each streaming goroutine's existing close-defer ordering. CI gate prevents regression on every future PR.
- **Cons:** Touches 28 sites — repetitive but mechanical. Custom CI check is ~80 lines of Go we now own.
- **Effort:** medium.

### Option B: Extract a generic `goWithChannelRecover[T]` helper and rewrite each streaming goroutine to call it
- **Approach:** Add `logging.GoWithChannelRecover[T any](ctx, name, finalCh chan<- T, mkErr func(error) T, fn func())`. Each streaming goroutine becomes `logging.GoWithChannelRecover(ctx, "anthropic-streamToolCall", final, func(err error) toolCallStreamFinal { return toolCallStreamFinal{Err: err} }, func() { /* existing body, minus the spawn */ })`. The helper handles spawn + recover + synthetic-error push.
- **Pros:** Single-source-of-truth recover logic. Forbids drift. Easier to enforce — the CI gate becomes "no `go` statement in `server/ai/` or `server/services/chat/` ever, period".
- **Cons:** The streaming bodies have *several* legitimate deferred closes (`cancel`, `close(fields)`, `close(final)`) and need access to the bound channel and cancel function. Threading those into a helper either bloats the helper signature (multiple channel parameters, varied error-shape types per provider) or requires capturing them in the closure — at which point we're not really centralizing much. Anthropic's producer also handles a separate `fields` channel; OpenAI/Gemini's producers do too. The helper would either need to know about both channels (provider-specific) or we'd inline the close defers anyway. Net: marginal reduction in boilerplate at the cost of a more rigid abstraction.
- **Effort:** medium-large.

### Option C: Wrap each provider with a top-level recover at the `Gen*Streaming` entry point
- **Approach:** Add a `safeProvider` wrapper that intercepts each `Gen*Streaming` call, spawns *its own* terminal goroutine that pulls from the wrapped provider's `final` channel under recover, and re-emits.
- **Pros:** Touches one new file, leaves provider implementations alone.
- **Cons:** Doesn't solve the actual problem. The bare goroutines that crash the process are *inside* the provider, spawned synchronously by the unwrapped `Gen*Streaming` method. A wrapper around the public surface can't recover panics that happen in goroutines launched downstream of the wrapper. Misses the chat path entirely.
- **Effort:** small to write, zero benefit.

## Recommendation

**Option A.** It matches the reviewer's stated shape (inline recover at the goroutine boundary, identical to `sender.go`), is the smallest surface-area diff that actually fixes every site, and the lint gate gives us the regression protection the reviewer asked for. Option B's apparent abstraction win evaporates once you account for the per-provider channel topology, and Option C doesn't solve the problem.

The shape for each streaming goroutine is exactly:

```go
go func() {
    defer cancel()             // [if applicable — producers only]
    defer close(fields)        // [if applicable — producers only]
    defer close(final)
    defer func() {
        if r := recover(); r != nil {
            logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
                "goroutine", "<provider>-<method>",
                "panic", r,
                "stack", string(debug.Stack()),
            )
            // Push a synthetic terminal-error value so the consumer
            // doesn't read the zero value from a closed-empty channel.
            final <- <FinalType>{Err: fmt.Errorf("panic in <name>: %v", r)}
        }
    }()
    // existing body
}()
```

LIFO defer ordering puts the recover *first* on panic, so the synthetic `final <- T{Err: …}` runs while `final` is still open, then `close(final)` (and `close(fields)`, `cancel()` for producers) runs second. Buffer size on `final` is 1 (verified across all three files) — the only path that has already written to `final` returns immediately afterward, so the recover-time write never blocks.

For the chat notification, swap to `logging.GoSafe(ctx, "chat-notify-participants", func() { … })` — there is no terminal channel; the `notifDone <- struct{}{}` block stays inside the wrapped function (and is itself benign on panic because the channel is also buffered/sized for one signal in tests).

## Open Questions

- **Should the producer goroutine in `streamToolCall` / `streamStructuredOutput` push a synthetic value on the `fields` channel before closing it on panic?** Recommendation: **no**. The consumer iterates `fields` with a normal `range`, and any unclosed `FieldEvent` keys will be discovered when the consumer reads the synthetic terminal error from `final` (every consumer does `if streamFinal.Err != nil { return }` *before* using fields). Pushing to `fields` would require knowing which keys to fabricate and would add noise.
- **Is the `notifDone <- struct{}{}` signal in the chat goroutine still needed under `GoSafe`?** Recommendation: **yes, keep it**. `GoSafe` only wraps the goroutine body in recover; the body still executes its normal completion path on the success branch, including the `notifDone` signal that drives test synchronization. On panic, `notifDone` won't fire — but that matches today's behavior (panic-then-crash also doesn't fire it), and tests that wait on it should already use a timeout (verify in Phase 2).
- **Lint gate scope: every Go file under `server/`, or just `server/ai/` + `server/services/chat/`?** Recommendation: **every file under `server/`**, with an allowlist of (a) `server/logging/logger.go` (defines `GoSafe`), (b) `server/services/streaming/sender.go` (already has inline recover), and (c) `_test.go` files. The whole point is to prevent the next reviewer from filing this exact issue again about a different file.
- **Test coverage shape: panic-injection test per goroutine, or one canonical test per provider?** Recommendation: **one panic-injection test per provider + one for chat**. Each provider's 8 consumer goroutines are byte-for-byte identical in shape; testing one validates the pattern. The producers are also one-per-provider. Test the panic by injecting a fault at a known seam (a new `panicAfterFirstDelta` test-only field on the provider, gated behind a build tag, or by using a provider mock that panics).

## Implementation Plan

### Phase 1: Chat notification + scaffolding

**Goal:** Easiest site shipped first; verifies pattern + sets up test scaffolding for the AI sites.
**Tasks:**
- [ ] `server/services/chat/messages.go:148` — replace `go func() { s.notifyParticipants(…); if notifDone != nil { notifDone <- struct{}{} } }()` with `logging.GoSafe(ctx, "chat-notify-participants", func() { … })`. Note: `GoSafe` uses `LoggerWithContext(ctx)` which preserves request_id/user_id.
- [ ] Add `server/services/chat/messages_panic_test.go`. Existing `server/notifications/mock.go` (`MockService.NotifyUser`) does not panic — add a tiny test-only `panicSender` adapter (in the test file) that wraps `MockService` and panics on the first `NotifyUser` call, or supply a `notifications.Service` implementation literal in the test that panics. Call `SendMessage`, assert (a) the RPC returns success, (b) no goroutine leak (deadline + select; `goleak` is not currently a server dependency), (c) the panic is logged at error level (capture via a `bytes.Buffer`-backed `*logging.Logger` injected through `logging.WithLogger(ctx, …)`).
- [ ] Run `npm run build` and `go test ./server/services/chat/...`.

**Validates:** `GoSafe` logging+recovery actually fires; test seam for panic injection works.

### Phase 2: Streaming consumers (Anthropic, OpenAI, Gemini)

**Goal:** Cover the 24 consumer goroutines. They share an identical shape, so this is the bulk of the diff but the lowest per-site risk.
**Tasks:**
- [ ] In each of the three `provider_*_streaming.go` files, add the inline recover defer to every consumer goroutine. The defer is registered last (so it runs first on panic) and pushes `<FinalType>{Err: fmt.Errorf("panic in <provider> <method> consumer: %v", r)}` to `final` before the existing `close(final)` defer fires. Use `logging.LoggerWithContext(ctx)` and `runtime/debug.Stack()` (already imported in `sender.go` — add the import where missing).
- [ ] Replicate the `sender.go:42-54` shape verbatim, parameterized only on the final-channel type and operation name.
- [ ] Add an `aitest`-package helper `PanicProvider` (or extend the existing `provider_mock.go`) that exposes a streaming method which panics after a configurable number of `FieldEvent`s — used by tests in Phase 4.

**Files:**
- `server/ai/provider_anthropic_streaming.go` — lines 216, 269, 322, 385, 441, 492, 544, 597
- `server/ai/provider_openai_streaming.go` — lines 205, 262, 323, 404, 470, 524, 592, 649
- `server/ai/provider_gemini_streaming.go` — lines 138, 177, 241, 294, 342, 378, 416, 455

**Validates:** Pattern compiles and passes existing tests (no behavior change on success paths).

### Phase 3: Streaming producers

**Goal:** Cover the 3 producer goroutines (`streamToolCall`, `streamStructuredOutput`, Gemini's `streamStructuredOutput`). These also own `close(fields)` and `cancel()` defers — verify ordering.
**Tasks:**
- [ ] `server/ai/provider_anthropic_streaming.go:101` — add inline recover; on panic, push `toolCallStreamFinal{Err: fmt.Errorf("panic in anthropic streamToolCall: %v", r)}` to `final` before the deferred `close(final)`. The `cancel()` defer (registered first → runs last) still fires correctly.
- [ ] `server/ai/provider_openai_streaming.go:103` — same, with `openaiStreamFinal`.
- [ ] `server/ai/provider_gemini_streaming.go:58` — same, with `geminiStreamFinal`.
- [ ] Confirm by reading: in each producer, the panic-time `final <- err` doesn't deadlock. `final` is `make(chan …Final, 1)`. The success path does exactly one send to `final` before returning. On panic mid-body, no send has occurred → buffer has space → recover defer's send is non-blocking.

**Validates:** Producer-side panics surface as a clean error to the consumer rather than a hang (consumer reads the synthetic err, returns to caller cleanly).

### Phase 4: Tests

**Goal:** Pin the recovery behavior so a future refactor that drops the defer breaks CI.
**Tasks:**
- [ ] `server/services/chat/messages_panic_test.go` (from Phase 1) — already covers chat.
- [ ] `server/ai/provider_streaming_panic_test.go` — for each provider, exercise both producer-panic and consumer-panic paths via fault injection. Note `goleak` is NOT a current server dependency — use a `t.Cleanup` deadline + select to detect channel-close hangs, e.g. `select { case <-final: case <-time.After(2*time.Second): t.Fatal("final never closed after panic") }`. Assert: `FieldEvent` channel closes (range loop terminates), `final` channel yields exactly one value with `Err != nil` whose message contains "panic". Do not pull in a new test dep just for this.
- [ ] All tests use `--reporter expanded` per CLAUDE.md (Flutter convention; for Go just confirm `go test -timeout 30s` passes).

**Validates:** Recovery actually fires for every shape; pattern is enforced by tests, not just code review.

### Phase 5: CI gate against new raw goroutines

**Goal:** Prevent regression. Reviewer asked for `forbidigo`, but `forbidigo` can't match the `go` keyword (it's AST-level call-expression matching). A purpose-built check is required.
**Tasks:**
- [ ] Add `server/cmd/check-goroutines/main.go` (~80 lines): walks `server/...` with `go/parser`, finds every `*ast.GoStmt`, and for each one inspects the spawned `*ast.FuncLit` body to confirm the *first* statement is a `defer func() { if r := recover() …` block (or, if the file path is in a hardcoded allowlist, skips). Allowlist: `server/logging/logger.go`, `server/services/streaming/sender.go`. Test files (`_test.go`) are also exempt — tests legitimately spawn deliberately-crashing goroutines.
- [ ] Wire into `npm run lint:go` (next to the existing `golangci-lint` invocation in `package.json`).
- [ ] Document the rule in `server/README.md` (or wherever goroutine conventions live — check if `docs/server/` has a relevant page).
- [ ] Confirm `npm run build` passes after wiring.

**Validates:** Adding a new `go func()` without recover anywhere under `server/` fails CI immediately.

### Phase 6: Build verification

**Goal:** Ship-ready state per CLAUDE.md.
**Tasks:**
- [ ] `npm run build` (golangci-lint + `go build ./...`).
- [ ] `go build -tags=integration ./...` (CLAUDE.md requires this when changing exported API; this PR doesn't, but we touched provider files).
- [ ] `go test -timeout 5m ./server/services/chat/... ./server/ai/...` — focus runs to keep the loop fast.
- [ ] Full `go test ./...` at the end.

## Related Issues

- The reviewer cites a recent "GoSafe rollout" — that's the broader effort; this issue is the residual gap. Search for the rollout PR(s) when implementing to confirm any conventions established there (e.g. naming convention for the `name` parameter passed to `GoSafe`).
- `server/services/streaming/sender.go` already implements the pattern this issue asks for; if there's an open issue tracking "extract sender-style recover into a helper," cross-link it — Phase 2/3 here intentionally inlines (see Option B for why) but a future consolidation could revisit.
