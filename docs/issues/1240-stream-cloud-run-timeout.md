# Issue #1240: StreamCommunityEvents: 504s, zombie streams, and close-race on Cloud Run

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1240
**Filed:** 2026-04-23 | **Last activity:** 2026-04-23
**Labels:** bug, P1 | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

`StreamCommunityEvents` (server/services/community/streaming.go) is a long-lived
server-streaming RPC used by the Flutter client to get sub-second community
event updates. It has three bugs that interact:

1. **Cloud Run 900s hard cap.** The handler has no self-imposed lifetime, so
   every healthy long-lived subscription ends in a 504 when Cloud Run's
   frontend terminates the request at the platform ceiling.
2. **No heartbeat.** Silent client disconnects (mobile NAT idle timeouts,
   network handoffs, app backgrounding without graceful dispose, device sleep)
   leave the server's select loop blocked with no way to detect the dead peer.
   `BroadcastCommunityEvent` keeps pushing into the abandoned 10-deep buffered
   channel, hits the 1s send timeout on every broadcast, and logs an ERROR for
   each one — until the Cloud Run 900s reaper finally fires.
3. **Close-race in unregister.** `unregisterEventStream` removes the subscriber
   under `eventStreamsMu` then closes the channel. `BroadcastCommunityEvent`
   copies the slice under RLock and then sends *outside* the lock, so a send
   can race with the close and panic rather than hit its 1s timeout. Low
   probability (unregister only fires when the handler returns) but not
   airtight.

The issue description already proposes a concrete fix: server-side 14-minute
cap + 30s heartbeat + relocate `close(ch)` to the handler. That proposal is
sound — this plan accepts the direction and focuses on the delivery strategy,
the proto shape, the client changes, and the monitoring gap the maintainer
flagged in the sole comment.

## Context from Comments

Single comment from the issue author (also the assignee):

> Thanks! I wonder why we don't see these as alerts on our Cloud Run monitoring...

Worth answering as part of this work. Looking at `terraform/modules/monitoring/main.tf`,
alerts fire on log-based metrics filtered by `severity>=ERROR`, not on Cloud Run
request status codes. The 504 is emitted by the Cloud Run frontend, not by the
Go server, so it never appears in the severity-indexed log stream and never
triggers an alert. The `logger.Error("timeout sending event to stream", ...)`
line at streaming.go:204 *does* emit ERROR logs for every broadcast into a
dead channel, which should have triggered the generic server-error alert — we
should verify empirically whether this has been firing quietly or whether
broadcast volume is just too low to cross the threshold.

## Potential Solutions

### Option A: Bundled fix as described in the issue (server cap + heartbeat + move close)
- **Approach:** One PR covering (a) `context.WithTimeout(ctx, 14*time.Minute)`
  on the handler's select loop, (b) `oneof payload { CommunityEventItem event;
  Heartbeat heartbeat; }` on `StreamCommunityEventsResponse` with a 30s ticker
  sending heartbeats, (c) move `close(ch)` from `unregisterEventStream` into a
  `defer` in the handler so the registry removal always precedes the close,
  (d) Flutter client ignores heartbeats (already uses `hasEvent()` check at
  `community_event_stream.dart:81`, so unrecognized oneof case is already
  safe), (e) confirm client reconnect-on-EOF works (already handled at
  `community_event_stream.dart:98-108`).
- **Pros:** All three root causes resolved together — they share the same
  select loop and the same channel lifecycle, so splitting them is artificial
  and would require rewriting the select twice. The client already handles
  unknown/missing events gracefully (`hasEvent()` guard) and already
  reconnects on clean EOF, so the client-side changes are minimal.
- **Cons:** Proto wire-format change (replacing a top-level field with a
  oneof). Any server code that constructs `StreamCommunityEventsResponse{Event: item}`
  as a struct literal would break — needs an audit and conversion to the
  oneof setter. Bigger review surface than a single-bug fix.
- **Effort:** Medium.

### Option B: Fix close-race first as a separate minimal PR, then do cap + heartbeat
- **Approach:** Ship a tiny PR that just moves `close(ch)` out of
  `unregisterEventStream` into the handler, plus adds a regression test.
  Follow up with cap + heartbeat in a second PR.
- **Pros:** De-risks the close-race fix — it's the highest-severity of the
  three (a panic, not just a 504 or log-spam) and can ship behind less
  review. Smaller proto changes to defer and reason about.
- **Cons:** Two PRs for a shared select-loop rewrite means touching the same
  ~30 lines twice, with the second PR essentially rewriting the first. The
  close-race has "low probability in practice" per the issue, so urgency is
  limited. Splitting creates an awkward state where one of the three bugs is
  fixed and the other two are not.
- **Effort:** Two PRs, small + medium.

### Option C: Move streaming off Cloud Run (e.g., long-lived Compute Engine / GKE sidecar)
- **Approach:** Route `/StreamCommunityEvents` to a separate deployment with
  no 900s cap. Keep Cloud Run for short RPCs.
- **Pros:** Eliminates the 14-min reconnect glitch (a user mid-session still
  sees a brief disconnect every 14 min with Option A). No artificial lifetime.
- **Cons:** Significant infrastructure work — new service, new deployment,
  new networking, DNS-level or Connect-level routing. The issue explicitly
  lists this as out of scope. The 14-min reconnect is UX-imperceptible given
  the client already has catch-up-on-reconnect working. Huge effort for a
  marginal UX win over Option A.
- **Effort:** Large.

## Recommendation

**Option A.** The three bugs share one select loop and one channel — they
want to be fixed together. The proto wire change is the only substantive
compatibility concern, and the Flutter client already accesses events via
`hasEvent()` / `response.event`, which means the generated Dart code will
continue to work when `event` becomes a oneof case (the generated accessors
are the same name). The only server-side hazard is struct-literal
construction of the response type; a quick grep audit covers it.

Ship one PR containing: proto change + regen, server handler rewrite (cap +
heartbeat + deferred close), client heartbeat branch (cosmetic — log + ignore),
regression test for the close-race, and a monitoring follow-up that alerts on
Cloud Run 504s directly (log-based metric keyed on `httpRequest.status=504`
for the Cloud Run request log stream).

## Open Questions

- **Should the heartbeat carry any payload (server time, stream ID, sequence number) or stay as an empty message?** —
  Recommendation: keep it empty (`message Heartbeat {}`). The only current
  consumer is connection-liveness; adding fields invites drift and tempts
  clients to rely on server time for things that should use the request
  path. If we later need debug info, add it then.

- **14 minutes or something shorter (e.g., 10 min)?** —
  Recommendation: 14 minutes. Cloud Run's cap is 900s = 15 min, so 14 min
  leaves a 60s buffer for the final `Send` + EOF flush to complete before
  the platform kills the stream. Shorter caps mean more reconnect churn for
  no benefit.

- **Should the 30s heartbeat be configurable?** —
  Recommendation: no, constant in code. 30s balances dead-peer detection
  latency against network chatter. If we later need per-environment tuning,
  promote to config then.

- **Alert on the `logger.Error("timeout sending event to stream")` line, or delete it entirely once heartbeats make it unreachable?** —
  Recommendation: after the heartbeat lands, the send-timeout path should
  only fire during the 30s window between a silent disconnect and the next
  heartbeat tick. Downgrade to `Warn` (it's a transient, self-healing
  condition) and drop the alert noise. If we see sustained volume post-fix
  it means the heartbeat isn't detecting what we think it is and we should
  investigate rather than suppress.

- **Does Cloud Run actually buffer `stream.Send` in a way that hides a dead peer for longer than 30s?** —
  Recommendation: verify with a manual test — open a stream, kill the
  client's TCP connection (iptables or airplane mode), confirm the handler
  exits within ~30-60s post-heartbeat-tick. If not, we need to tune
  TCP keepalive or the Connect-go transport settings.

- **Does the existing 504-triggered client reconnect re-query catch-up events correctly?** —
  Recommendation: the client code at `community_event_stream.dart:72-76`
  passes `_lastTimestamp[communityId]` as `sinceUnixSec` on reconnect and
  the `onDone` handler reconnects with backoff, so yes in principle.
  Add an integration test or manual smoke test that forces an EOF
  mid-stream and confirms no events are duplicated or missed.

## Implementation Plan

### Phase 1: Close-race fix with regression test
**Goal:** Eliminate the panic-on-closed-channel hazard first, as it's the only
one of the three that can corrupt process state rather than degrade UX.
**Tasks:**
- [ ] Remove `close(ch)` from `unregisterEventStream` in `server/services/community/streaming.go`.
- [ ] In `StreamCommunityEvents`, replace the existing
  `defer s.unregisterEventStream(...)` with a single deferred closure that
  orders the two operations explicitly:
  ```go
  defer func() {
      s.unregisterEventStream(req.Msg.CommunityId, eventChan)
      close(eventChan)
  }()
  ```
  **Audit fix (2026-04-23):** the earlier version of this plan proposed two
  separate `defer` statements and claimed Go's LIFO order would run
  `unregisterEventStream` first. That is backwards — two defers in registration
  order `[unregister, close]` would execute `close` first on return, which is
  exactly the race we're trying to eliminate. The single deferred closure above
  makes ordering explicit, doesn't rely on reader familiarity with LIFO defer
  semantics, and is robust to future code insertions between the defers.
- [ ] Add a unit test that spins up a broadcaster and a handler, forces the
  handler to return mid-broadcast, and asserts no panic. Use a fake
  `connect.ServerStream` + a goroutine calling `BroadcastCommunityEvent` in a
  tight loop while the handler's context is cancelled.
- [ ] Per `docs/server/architecture.md` §Testing, unit tests go alongside the
  implementation — add the regression test to `server/services/community/streaming_test.go`
  (create if absent).
**Validates:** The close-race fix works in isolation and doesn't require the
proto change to land.

### Phase 2: Proto change for heartbeat payload
**Goal:** Land the wire-format change with the rest of the generated code,
without yet exercising the heartbeat path.
**Tasks:**
- [ ] Edit `proto/ripls/api/community_service.proto`: replace
  `CommunityEventItem event = 1;` in `StreamCommunityEventsResponse` with
  `oneof payload { CommunityEventItem event = 1; Heartbeat heartbeat = 2; }`
  and add `message Heartbeat {}`.
- [ ] Write conventions-compliant proto comments per CLAUDE.md §"Proto Comments".
  The `payload` oneof and `Heartbeat` need a client-facing description
  ("A periodic liveness signal. Clients may ignore this message; its
  presence indicates the subscription is healthy.") — no server mechanics,
  no transport plumbing, no mention of Cloud Run or the 30s cadence.
- [ ] `npm run generate`.
- [ ] Audit all Go callers: `grep -rn "StreamCommunityEventsResponse{" server/`.
  Existing hits are at `streaming.go:84` and `streaming.go:194`. Convert to
  the oneof struct-literal pattern used elsewhere in the codebase (see
  `server/services/feed/service.go:849` for a representative example):
  ```go
  resp := &api.StreamCommunityEventsResponse{
      Payload: &api.StreamCommunityEventsResponse_Event{Event: item},
  }
  ```
  **Audit fix (2026-04-23):** the earlier version proposed `resp.SetEvent(item)`,
  which is not how `protoc-gen-go` generates oneof accessors for this codebase
  — use direct struct-literal assignment with the generated oneof wrapper type.
- [ ] Confirm the Dart generator still emits `hasEvent()` for the oneof case.
  `protoc-gen-dart` emits per-case `hasXxx()` accessors for oneof members,
  so the existing client code at `app/lib/services/community_event_stream.dart:81`
  (`if (response.hasEvent())`) continues to work without changes. Verify by
  reading the regenerated `community_service.pb.dart` after `npm run generate`.
- [ ] Run `npm run build` (which runs golangci-lint + `go build ./...` per
  CLAUDE.md) and `go test ./server/services/community/...`.
**Validates:** Proto change compiles server- and client-side, existing
event-delivery tests still pass, no caller was missed.

### Phase 3: Server-side lifetime cap + heartbeat
**Goal:** Prevent 504s on the live path and reap zombie streams within 30s.
**Tasks:**
- [ ] In `StreamCommunityEvents`, after the catch-up loop, wrap the select
  with `streamCtx, cancel := context.WithTimeout(ctx, 14*time.Minute); defer cancel()`.
- [ ] Add `ticker := time.NewTicker(30 * time.Second); defer ticker.Stop()`.
- [ ] Add `case <-streamCtx.Done():` → return nil (clean EOF).
- [ ] Add `case <-ticker.C:` → `stream.Send(&api.StreamCommunityEventsResponse{...Heartbeat: &api.Heartbeat{}})`.
  On error, log and return — this is the dead-peer signal.
- [ ] Keep the existing `case <-ctx.Done():` (genuine client disconnect) and
  `case event := <-eventChan:` branches.
- [ ] Downgrade `logger.Error("timeout sending event to stream", ...)` at
  line 204 to `logger.Warn(...)` — post-heartbeat, this path should be rare
  and self-healing.
- [ ] Unit tests: (a) stream exits cleanly after 14-min cap (use a shorter
  cap via parameterization, e.g., inject a `streamLifetime` field on
  `Service` defaulting to 14m and set to 100ms in tests); (b) handler
  returns promptly when `stream.Send` for a heartbeat errors; (c) assert
  new log lines use `logging.LoggerWithContext(ctx)` with the standard
  `operation`, `user_id`, `community_id`, `duration_ms` fields already
  present in this handler — don't introduce `log.Printf` or ad-hoc slog
  loggers per `docs/server/observability.md`.
- [ ] Per architecture doc §Testing, major features also need integration
  coverage: add a test to `server/main_integration_test.go` (or a sibling
  in the community package with a real `storage.SetupTestStorage(t)`) that
  opens a stream with a sub-second lifetime, confirms EOF is delivered to
  the client without error, and confirms a subsequent reconnect with
  `since_unix_sec` sees no duplicate events.
**Validates:** No more 504s on live subscriptions, zombie streams reaped
within ≤30s.

### Phase 4: Client heartbeat handling
**Goal:** Make sure the Flutter client silently ignores heartbeats and
reconnects on EOF.
**Tasks:**
- [ ] In `app/lib/services/community_event_stream.dart`, the existing
  `if (response.hasEvent())` check at line 81 will already skip
  heartbeats cleanly because `hasEvent()` returns `false` when the oneof
  case is `heartbeat`. Confirm by reading the regenerated
  `community_service.pb.dart`.
- [ ] Add explicit `else if (response.hasHeartbeat())` branch that calls
  `_resetBackoff()` (useful liveness signal for the reconnect logic) and
  logs at FINE level via the existing `Logger('CommunityEventStream')`.
  Do not expose heartbeats to `_eventRouter`. Per `docs/client/architecture.md`,
  this file is a service — not a viewmodel — so business logic still lives
  here rather than being promoted into a Notifier. No architectural shift.
- [ ] No cache invalidation on heartbeat: the client's cache invariants are
  driven by `_eventRouter.routeCommunityEvent`, and heartbeats carry no
  entity data. Nothing to plumb through repositories per
  `docs/client/caching.md`.
- [ ] Manual smoke test: start a stream, wait past 14 min, confirm the
  client reconnects via `onDone` and resubscribes with `since_unix_sec`
  equal to the last-seen event timestamp. Confirm no duplicated or dropped
  events using a synthetic event emitter on the server.
**Validates:** End-to-end the client correctly handles cap-triggered EOFs
and heartbeat messages without surfacing errors to the user.

### Phase 5: Monitoring — answer the maintainer's question
**Goal:** Ensure the next time a long-lived stream regresses, we hear about
it.
**Tasks:**
- [ ] Add a Cloud Run log-based metric in `terraform/modules/monitoring/main.tf`
  that counts requests where `httpRequest.requestUrl =~ "/StreamCommunityEvents"`
  AND `httpRequest.status = 504` (or `>= 500`). Cloud Run writes these to the
  request log stream, distinct from application severity logs.
- [ ] Wire that metric to an alert policy with threshold (e.g., >0 in 5m =
  page). Route to the existing `ops@ripls.org` notification channel.
- [ ] `terraform plan` + apply in dev to confirm the metric captures the
  existing 504s; apply to prod after the server fix has been live long
  enough to confirm 504s stop.
**Validates:** Future regressions of the 900s-cap class are detectable
without waiting for a user report.

## Audit Notes (2026-04-23)

This plan was audited against `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, and the proto/logging conventions in `CLAUDE.md`. Changes
applied during audit:

1. **Phase 1 defer ordering (correctness fix).** Original plan asserted two
   separate `defer` statements in `[unregister, close]` registration order
   would execute `unregister` first by Go's LIFO rules. That's backwards and
   would reintroduce the exact close-race the phase aims to fix. Replaced
   with a single deferred closure that calls both in explicit order.
2. **Phase 2 Go oneof construction (correctness fix).** Original plan proposed
   `resp.SetEvent(item)` which is not the accessor style `protoc-gen-go`
   emits in this codebase. Updated to the struct-literal pattern used in
   `server/services/feed/service.go` (`Payload: &api.StreamCommunityEventsResponse_Event{Event: item}`).
3. **Phase 2 proto comments.** Added a task to write client-facing, vendor-
   and transport-neutral comments for `Heartbeat` and the `payload` oneof
   per CLAUDE.md §"Proto Comments".
4. **Phase 2 Dart generator confirmation.** Clarified that `protoc-gen-dart`
   emits `hasEvent()`/`hasHeartbeat()` for oneof members, so the existing
   client check at `community_event_stream.dart:81` is safe without a
   rewrite. Noted the regenerate-and-verify step explicitly.
5. **Phase 3 integration test.** Per `docs/server/architecture.md` §Testing,
   major features need integration coverage, not only unit tests. Added a
   task to `server/main_integration_test.go`.
6. **Phase 3 observability alignment.** Added explicit requirement to keep
   using `logging.LoggerWithContext(ctx)` with standard field names per
   `docs/server/observability.md` — no `log.Printf`, no ad-hoc slog loggers.
7. **Phase 4 architectural clarification.** `CommunityEventStreamService` is
   a service, not a viewmodel — `docs/client/architecture.md` permits
   business logic in service classes for stream lifecycle concerns (mirrors
   the `DiscoverMapHelper` example). No change needed; added a note so a
   reviewer doesn't mistakenly flag this as a missing Notifier migration.
8. **Phase 4 caching.** Confirmed heartbeats do not touch the repository or
   cache layer (no entity identity, no mutation). Nothing required per
   `docs/client/caching.md`.

## Related Issues
- #1232 / #1218 — Server-side streaming for `GenExperience`/`GenRequest`/`GenGear`.
  Those streams are bounded (fixed-length generation pipeline), so they don't
  need the cap or heartbeat — but the close-race fix pattern may be worth
  reapplying to `server/services/streaming/sender.go` if the same
  close-outside-lock shape exists there. Worth a quick audit during Phase 1.
- #920 — RSVP realtime. If that shipped using the same event-stream
  infrastructure, it benefits from this fix for free.
