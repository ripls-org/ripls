# Issue #1658: Phase 2 of #1619: emit community_deleted signal on active watch streams

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1658
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 6.5, 12)

## Problem Statement

`StreamCommunityEvents` keeps a long-lived server stream open for as
long as the client is connected. Today, when a community is
soft-deleted via #1654, an open stream for that community has
nothing telling it the community is gone — it stays subscribed
forever (until lifetime cap or client disconnect). The client UI
keeps showing the community as alive while every other surface
(list, search, push) treats it as deleted.

The fix is small in shape but worth doing: when a
`COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED` event arrives over the
stream's event channel, send it to the client (so the client knows
*why* the stream is closing) and then return cleanly from the
stream handler. The client gets a clean EOF with the deletion
event as its last message; the client's existing stream-end
handler routes the user away.

The streaming surface to fix is narrow: `StreamCommunityEvents`
(`server/services/community/streaming.go`). The other server
streams in the codebase are either short-lived AI-generation
streams (`gen_streaming.go`, `gen_ai_streaming.go`) that complete
on their own, or chat streams scoped by `conversation_id`. The
chat-stream-on-community-delete question is real but bigger
(closing every community-scoped conversation's stream is a wider
audit) and the issue body explicitly leaves the per-stream shape
to the PR — recommending we keep this PR narrow to
`StreamCommunityEvents` and file a follow-up for chat streams if
the UX gap matters in practice.

## Context from Comments

The issue has **no comments**. Two notable cues from the body:

- The hint at line 47 of `streaming.go` already exists:
  `// Reject the stream open if the community is deleted or the caller is not
  // a member. Mid-stream termination on COMMUNITY_DELETED ships with #1619.`
  This issue is exactly the second sentence.
- The issue body says: *"the simplest path is for #1654's emission
  of COMMUNITY_DELETED to flow through the same channel and the
  stream sender to recognize that event and close."* — that's the
  recommended shape.

## Codebase audit

What already exists this RPC will lean on:

- **Broadcast plumbing.** `RecordCommunityEventAndNotify` →
  `eventBroadcaster` → `Service.BroadcastCommunityEvent` →
  per-stream channel send. Already implemented and tested.
  Wired in `service.go:registerBroadcaster`.
- **Stream loop.** `runStreamLoop`
  (`server/services/community/streaming.go:122`) handles
  `ctx.Done`, lifetime cap, heartbeat, and event delivery in a
  single `select`. The COMMUNITY_DELETED termination logic slots
  in as an extra branch on the `eventChan` case — *after* the
  send succeeds, recognise the event type and return nil.
- **Open-stream rejection.** `auth.RequireMemberOfActiveCommunity`
  already runs at stream open and returns NotFound for soft-deleted
  communities. New stream attempts can't subscribe to a deleted
  community. The bug is only for streams that were already open
  when the delete happened.
- **Actor skip.** `BroadcastCommunityEvent` skips `event.ActorId`
  ("they get their update via local cache invalidation"). For
  COMMUNITY_DELETED the actor is the deleter — they call
  DeleteCommunity, get a synchronous response, and their UI
  navigates away. Their own stream not receiving COMMUNITY_DELETED
  is fine.
- **Sole-member-leave conversion (Phase 4 of #1656).** Calls
  `deleteCommunityInternal` which emits COMMUNITY_DELETED with the
  leaver as actor. Same path; same termination semantics.

Race window of note (small, documented):

- `StreamCommunityEvents` does
  `RequireMemberOfActiveCommunity` (active check) → register
  channel → catch-up → enter `runStreamLoop`.
- Between the active check and registration, COMMUNITY_DELETED
  could fire and miss our channel because we weren't subscribed
  yet.
- After this PR ships, in that case the stream stays open against a
  now-deleted community until lifetime cap. New events for that
  community don't fire (LeaveCommunity returns FailedPrecondition
  on deleted community per #1656; other write paths are gated
  similarly), so the stream is silent for the duration but doesn't
  emit stale data either. Acceptable for a small race; mitigate
  with a single re-check after registration.

What's missing and must be built:

- **Stream loop event-type recognition.** A check after `Send`
  succeeds: if the event type is COMMUNITY_DELETED, return nil to
  end the handler.
- **Optional but cheap defensive re-check.** After registering
  the channel and before entering `runStreamLoop`, re-read the
  community with `IncludeDeleted: true`. If now deleted, send a
  synthetic terminal event (constructed from the community's
  `Deleted` metadata) and return. One extra storage read per
  stream open; closes the registration race window cleanly.
- **Tests.** Two integration cases are required:
  1. Open stream → delete community in another goroutine → assert
     the stream emits the COMMUNITY_DELETED event and the handler
     returns within bounded duration (use a fake sender like the
     existing `streaming_test.go` pattern).
  2. Race-window-mitigation: simulate the race by inserting a
     soft-deleted community and opening a stream that bypasses the
     pre-register check (or use the post-register defensive
     re-check directly). Assert termination.

## Potential Solutions

### Option A: Recognize event type in stream loop (recommended, per issue body)

- **Approach:** After the existing `sender.Send(resp)` for the
  event channel case, check `item.EventType ==
  COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED`; return nil if so.
  Add the optional defensive re-check after registration.
- **Pros:**
  - Smallest possible diff. ~10 lines in the loop, ~15 lines for
    the re-check.
  - Reuses the existing broadcast plumbing; no new infrastructure.
  - Per-RPC: only `StreamCommunityEvents` is touched, scope stays
    narrow.
- **Cons:**
  - The pre-register race window remains unless we add the
    defensive re-check (recommend including it).
- **Effort:** small. ~50 lines including tests.

### Option B: Out-of-band terminal-signal channel

- **Approach:** Add a per-community "kill switch" channel
  `s.streamKills[communityID]` that the deleter writes to; each
  stream's `select` includes a case for it. `BroadcastCommunityEvent`
  becomes responsible for emitting both the event AND the kill.
- **Pros:**
  - Termination signal is decoupled from the event payload.
- **Cons:**
  - Doubles the moving parts in the stream registry.
  - Non-trivial cleanup semantics for the kill channel
    (when does it get drained? per-stream? per-community?).
  - Solves a problem we don't actually have — the event payload
    is already a perfect carrier.
- **Effort:** medium. Rejected — over-engineered for the need.

### Option C: Stream re-checks community state on every heartbeat

- **Approach:** Every 30s on the heartbeat tick, re-read the
  community; if deleted, send synthetic event and exit.
- **Pros:**
  - Eventually catches every deletion regardless of race or event
    plumbing failure.
- **Cons:**
  - Up to 30s of stale events while waiting for the next heartbeat.
  - O(active-streams × heartbeat-frequency) storage reads — this
    is the most expensive path.
  - Storage layer is already the hot-path bottleneck per the
    SQL Efficiency rules in `CLAUDE.md`.
- **Effort:** small but wrong shape. Rejected.

## Recommendation

**Option A**, including the defensive post-register re-check. Per
issue body's explicit recommendation. The defensive re-check is
cheap and closes the only race window I can identify without
adding new infrastructure.

## Open Questions

- **Should the defensive re-check ship in this PR or as a
  follow-up?** — *Recommendation:* this PR. It's ~15 lines
  including the test for it, and it makes the race-window
  documentation a footnote rather than a known limitation.
- **What about `StreamMessages` (chat) streams scoped to
  community-tagged conversations?** —
  *Recommendation:* explicitly out of scope for #1658. The chat
  stream is keyed on `conversation_id`, not `community_id`, so
  the question of whether to terminate it on community-delete is
  a wider design call (do all community-scoped chats become
  inaccessible immediately on delete? Per design doc §6.5,
  community-scoped conversations should 403 for callers, but
  that's a read-path issue, not a stream-termination one.) File
  a follow-up issue if the UX gap is real in practice; don't
  conflate it with #1658's narrow scope.
- **Should we send a synthetic COMMUNITY_DELETED in the
  defensive re-check, or close without one?** —
  *Recommendation:* send a synthetic event so the client's
  stream-end handler has the same shape regardless of whether
  the delete fired during the pre-register race or during the
  loop. The synthetic event uses the community's stored
  `Deleted.DeletedByUserId` as actor and `DeletedAtUnixSec` as
  occurrence time so the client gets correct provenance.
- **Should `COMMUNITY_RESTORED` be similarly treated?** —
  *Recommendation:* no. Per issue body: "any pre-existing client
  subscription is gone (the stream was closed); the client must
  open a fresh stream." Restore doesn't reach an active stream
  because the delete already closed it. The client UX is to open
  a new stream after seeing the community return to active in
  Settings → Communities.

## Implementation Plan

### Phase 1: Stream-loop termination on COMMUNITY_DELETED

**Goal:** the simplest path the issue body recommends.

**Tasks:**
- [ ] In `runStreamLoop`'s `eventChan` branch, after the
  successful `sender.Send(resp)` call, check
  `item.GetEventType() == api.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED`.
  If so, log
  `"event stream ended on community-deleted signal"` at INFO
  with `duration_ms`, then return nil so the handler exits
  cleanly.
- [ ] Update the existing log line in
  `streaming.go:46-47` to remove the `"ships with #1619"` aside
  (it's about to ship — replace with a stable comment).
- [ ] Unit test: extend `streaming_test.go`'s fake-sender
  pattern to drive a COMMUNITY_DELETED event into the channel
  and assert `runStreamLoop` returns nil after the Send.
- [ ] Catch-up test: with `since_unix_sec` covering a previously
  recorded COMMUNITY_DELETED event, verify the stream sends it
  during catch-up and terminates without entering `runStreamLoop`
  for further events. (Catch-up runs in `StreamCommunityEvents`
  before `runStreamLoop`, so this case verifies the termination
  hook fires uniformly across both paths.)

**Validates:** the simplest path works against a unit-level fake
sender, decoupled from Connect's HTTP wiring and the testcontainer
DB.

### Phase 2: Defensive post-register re-check

**Goal:** close the registration race window cleanly.

**Tasks:**
- [ ] After
  `s.registerEventStream(...)` and `defer s.unregisterEventStream(...)`
  but *before* the catch-up block, re-read the community with
  `IncludeDeleted: true`. If `Deleted != nil &&
  DeletedAtUnixSec > 0`, construct a synthetic
  `*api.StreamCommunityEventsResponse` carrying a
  COMMUNITY_DELETED `CommunityEventItem` (use
  `community.Deleted.DeletedByUserId` for actor and
  `Deleted.DeletedAtUnixSec` for occurrence time, leave Actor
  denormalisation to `communityEventToItem` if practical, or
  build inline if simpler), send it, and return nil.
- [ ] Unit test: seed a soft-deleted community, drive
  `StreamCommunityEvents` against it, verify the stream sends a
  single COMMUNITY_DELETED event and returns. (Bypass the
  `RequireMemberOfActiveCommunity` rejection by seeding the
  community as active, registering, then flipping it
  soft-deleted in the test setup before runStreamLoop runs —
  mirrors the real race.)

**Validates:** the race window is closed end-to-end. Stream
clients see the same shape regardless of whether the delete fired
just before or just after their registration.

### Phase 3: Integration test for the full delete-during-stream flow

**Goal:** prove the full path works against the testcontainer DB,
not just the fake sender.

**Tasks:**
- [ ] Add `server/integration_tests/community_stream_delete_test.go`:
  1. Open `StreamCommunityEvents` against an active community in
     a goroutine, capturing every received event.
  2. Concurrently call `DeleteCommunity` from another goroutine
     (with the owner's auth context).
  3. Assert: the stream goroutine returns within ~5s; the last
     event captured was COMMUNITY_DELETED with the expected
     actor.
  4. Assert: the stream registry no longer contains the closed
     stream's channel (calls
     `s.HasActiveEventStream(communityID, userID)` returns
     false).
- [ ] Mirror the existing `streaming_test.go` patterns for
  channel cleanup. Use a `sync.WaitGroup` / `select` with
  timeout.

**Validates:** the full broadcast path → stream-loop termination
→ registry cleanup works under realistic concurrency.

## Audit Notes

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings folded into the plan above:

- **Logging conventions (observability §Standard Field Names).**
  The existing `streaming.go` log calls already carry the right
  field names (`operation=StreamCommunityEvents`, `user_id`,
  `community_id`, `duration_ms`, `reason`). The new INFO log on
  COMMUNITY_DELETED termination should use `reason=community_deleted`
  to match the existing taxonomy of `reason` values
  (`context_done`, `lifetime_cap`, etc.). The Phase 2 defensive
  re-check should log at INFO with
  `reason=community_deleted_pre_loop` (or similar) so the two
  paths are distinguishable in Cloud Logging.
- **`connecterr.Internal` (architecture §Error wrapping).** The
  defensive re-check's storage read is the only new internal-error
  surface in this PR. Use
  `connecterr.Internal(ctx, "StreamCommunityEvents", err, "community_id", ...)`
  on storage failure — never bare `connect.NewError(connect.CodeInternal, err)`
  (gated by `scripts/check_codeinternal.js`). All other error
  returns in the existing handler already comply.
- **Goroutine safety (architecture §Goroutine Safety).** No new
  goroutines. The existing broadcaster fan-out runs synchronously
  inside `BroadcastCommunityEvent`'s caller; no `go func()`
  introduced. ✓
- **Service-to-service boundary (architecture §1).** No changes —
  the stream loop already lives inside the community service and
  consumes the package-level `eventBroadcaster` registered in
  `service.go`. ✓
- **Test patterns.** The existing `streaming_test.go` already
  defines a `fakeSender` matching the `streamSender` interface,
  exactly the surface Phase 1 needs. Pinned that pattern in the
  Phase 1 task list rather than introducing a new fake.
- **Integration test location (architecture §7 + per-feature
  pattern).** Pinned to
  `server/integration_tests/community_stream_delete_test.go`
  matching the per-feature pattern from #1654 / #1655 / #1656 /
  #1657.
- **No PII or sensitive data in synthetic event.** The synthetic
  COMMUNITY_DELETED event built by the Phase 2 re-check carries
  the deleter's user_id (already a public field on
  `CommunityEvent`) and the deletion timestamp. No email or token
  surfaces — masking helpers are not required.
- **Catch-up emits COMMUNITY_DELETED naturally.** If a client
  reconnects after a delete with `since_unix_sec` covering the
  deletion event, the existing catch-up loop will surface it
  because `CommunityEvent` rows are not soft-deleted on community
  delete (see #1654 Phase 1 — events are kept for audit and the
  daily-purge reminder; #1620 hard-deletes them). Phase 1's
  termination check applies uniformly to catch-up and live events;
  document a single test case covering the catch-up path so the
  same termination semantic is verified there too. **Added to
  Phase 1 task list.**
- **No client work in scope.** This issue is server-only. Client
  handling of the stream-end signal is out of scope; the existing
  client stream-end handler (which routes the user away on EOF)
  already does the right thing for free.

The plan is now consistent with `docs/server/architecture.md` and
`docs/server/observability.md`. Ready for implementation.

## Related Issues

- **#1619** — parent epic.
- **#1621** — Phase 1 read-path filter audit (the *check* for
  deleted state at call sites; this issue adds the *signal* for
  long-lived streams).
- **#1654** — DeleteCommunity rewrite (the emission point).
- **#1656** — LeaveCommunity rewrite (sole-member-leave converts
  to delete and emits the same COMMUNITY_DELETED that this issue
  terminates the stream on).
- **#1622** — Phase 1 notification dispatcher guard (parallel
  treatment for push: skip community-deleted; the stream signal
  is the equivalent for live subscriptions).
