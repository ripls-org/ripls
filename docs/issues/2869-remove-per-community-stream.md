# Issue #2869: Remove the deprecated per-community StreamCommunityEvents RPC

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2869
**Filed:** 2026-08-08 | **Last activity:** 2026-08-08
**Labels:** P3, review:tech_debt | **Assignees:** unassigned
**Context docs:** anchors `proto_conventions.md`, `server/architecture.md`,
`server/conventions.md`, `testing/architecture.md`, `users/priorities.md`;
leaf docs `realtime_updates.md`, `ad_hoc_communities.md`,
`server/observability.md`; review prompts `tech_debt`, `api_design`,
`testing`, `observability`, `code_quality`. Prior plan:
`docs/issues/2867-realtime-fanout.md`.

> **DECISION (2026-08-09): Option A, and the soak wait is waived.** The
> maintainer accepts the stranded-client risk in exchange for shipping now:
> one PR, no prep split, and no waiting for a release to carry the per-user
> client first. The **Implementation Plan** below is what that means in
> practice.
>
> **Superseded in part (2026-08-10):** `v0.29.1` shipped carrying the per-user
> client, so the policy wait is now satisfied on its own terms and the waiver
> narrows to adoption only. See the posture update under *Problem Statement*. Everything between here and the Recommendation heading is the
> analysis as it stood before that call — kept because the *reason* the risk is
> acceptable (see *What a stranded client actually experiences*) is now the
> load-bearing part of the decision, not background.
>
> The one thing the decision does **not** waive is the test ports in the
> inventory below. Those cover `runUserStreamLoop` and `broadcastToUserStreams`
> — live code, including a send-on-closed-channel panic hazard — and the risk
> that was accepted is about old clients, not about deleting coverage of the
> path that replaces them.

> **On `ripls-ui-investigate`:** not run. This is a scheduled code removal with
> no UI-observable behavior to reproduce — the client stopped calling the RPC on
> `main` as of #2867 and no screen renders anything sourced from it. The
> investigation this issue actually needs is a *deployment-state* one (has the
> replacement shipped, is anyone still calling it), and that is answered below
> from prod logs and the Cloud Run revision.

## Problem Statement

#2867 replaced the per-community realtime layer with per-user RPCs
(`StreamUserEvents` / `ListUserEvents`). `StreamCommunityEvents` and its two
messages were marked `deprecated` but left working, because removing an RPC is
wire-breaking and already-installed mobile clients still open it. This issue is
the scheduled cleanup.

The *code* removal is mechanical. Two things make it worth planning rather than
just doing:

**1. The formal gate is met; adoption is not.** *(Revised 2026-08-10 — see the
posture note below. The original text is preserved after it, because the first
draft of this plan was written against a materially different world.)*

> **POSTURE UPDATE (2026-08-12 19:25 CDT) — the gate is now fully met.**
> A *second* release, `v0.29.2`, shipped 2026-08-11 08:36 CDT, and it carries
> #2870 too; prod's server is `server:ed93fc3` = that release merge. So **two
> shipped releases** now carry the per-user client, both soaked well past 24h.
> That satisfies not just the one-release minimum but the policy's stated
> preference to "let an RPC removal ride a release further than the minimum when
> the surface it breaks is one a stranded build would hit on launch" — which is
> this surface.
>
> Adoption has turned over, which is the part that was missing two days ago:
>
> | window | `StreamCommunityEvents` (deprecated) | `StreamUserEvents` |
> |---|---|---|
> | since `v0.29.1` (3.1d) | 140 opens / 5 users | 24 opens / 7 users |
> | since `v0.29.2` (1.5d) | 21 opens / 2 users | 14 opens / 5 users |
> | last 24h | **15 opens / 1 user** | **12 opens / 5 users** |
>
> Five distinct users are on the new stream in the last 24h against one on the
> old. Two days ago that ratio was inverted (2 users deprecated, 1 per-user).
> Both stragglers named in the 2026-08-10 note have since moved off — the
> remaining opens come from *different* accounts, which is the tell that this is
> churn rather than a fixed holdout set: as installs update, other people open
> older builds. One straggler (`a4ac610f…`) was last seen 2026-08-12 20:53 UTC,
> ~3.5h before this check; the other (`e4abf268…`) went quiet ~33h ago.
>
> **That churn is the argument against waiting for zero.** There is no
> force-upgrade and no version telemetry, so zero is a state nothing guarantees
> will arrive — waiting further trades a real, met policy gate for an
> open-ended one. Recommendation: merge.
>
> ---
>
> **POSTURE UPDATE (2026-08-10 15:55 CDT).** `v0.29.1` shipped on 2026-08-09 at
> **17:34 CDT**, and it **carries #2870** (`git merge-base --is-ancestor
> 6a1793cc7 v0.29.1` → yes). Prod's server is now `server:20a7b56` = that release
> merge, so `StreamUserEvents` is deployed and serving. The policy wait — one
> shipped release plus ~24h — therefore **started at the v0.29.1 tag and
> completes around 17:35 on 2026-08-10.**
>
> What has *not* happened is adoption. Measured over the ~22h since the tag:
>
> | RPC | opens | distinct users | most recent |
> |---|---|---|---|
> | `StreamCommunityEvents` (deprecated) | **83** | **2** | 2026-08-10 **20:48 UTC** (~6 min before the check) |
> | `StreamUserEvents` (its replacement) | 3 | 1 | 2026-08-10 20:52 UTC |
>
> The deprecated RPC is still carrying the great majority of stream traffic. One
> of the two users appears in *both* lists — an updated client on one surface
> (web is served fresh) and a not-yet-updated mobile build on another. The other
> was last seen at 2026-08-10 00:49 UTC.
>
> This is exactly what `proto_conventions.md` says the interval does and does not
> buy: "The 24 hours is a soak for the release itself, not for adoption," and
> "neither interval ever bounded a build that simply never updates." The release
> build succeeded on 2026-08-09T23:07Z, so store rollout and user updates are the
> remaining variable, and nothing in the repo gates on them.
>
> **The waiver therefore shrinks from "skip the whole wait" to "skip adoption."**
> Yesterday there was no release carrying the client and prod did not even have
> the replacement deployed — the removal would have shipped into a population
> that was 100% on the deprecated RPC with no alternative running. Today the
> replacement is live in prod, the policy clock has run, and the exposure is two
> known installs. That is a substantially better footing for the same decision.

The original framing, written 2026-08-09, before `v0.29.1`:

> The issue says "the clock starts at that release," meaning the release carrying
> #2867. There is no such release. `v0.29.0` was tagged 2026-08-08 at 15:49 CDT
> and #2870 merged at 17:15 the same day; prod's server was `server:23d2e25` =
> the v0.29.0 release merge, which predates it. Prod logs for 2026-07-29 →
> 2026-08-09 showed 3,000 `StreamCommunityEvents` opens from 12 distinct users
> and zero per-user opens. 100% of the live population was on the RPC this issue
> removes, and the RPC that replaces it was not deployed anywhere. The waiver at
> that point was "the clock has not started and we are shipping anyway."

**2. The issue's own removal list is wrong in three places.** Following it
literally breaks the build or silently drops coverage. See *Removal inventory*
below. In particular `streaming.go` cannot simply be deleted, `streaming_test.go`
holds the only tests for invariants the *live* per-user loop also depends on, and
the instruction to "reserve the tags" contradicts the repo's own convention.

## Context from Comments

**No comments.** The issue was filed by `/ripls-reviews` from the tech_debt
review immediately after #2867 merged, so its body is a hand-off note from that
work rather than a discussion. That is fine for a mechanical cleanup, but it does
mean nothing in the issue has been checked against reality since — which is
exactly how the "clock starts at that release" premise got in. The plan treats
the issue body as a starting inventory, not as a specification.

## Removal inventory

What the issue lists, corrected and completed. The **bold** rows are changes to
the issue's own list.

### Must move before anything is deleted

- **`communityEventToItem` is in `streaming.go`, and it is the only converter.**
  `streaming.go:428` defines it; `stream_subscriber.go:48` and
  `user_streaming.go:126` — both live per-user code — call it. Deleting
  `streaming.go` as the issue instructs does not compile. It has to move first.
  Its own doc comment already points at its destination ("Mirrors the
  denormalization logic in `ListCommunityEvents` (`events.go`)"), and `events.go`
  is 90 lines, so the 1,000-line gate is not a factor.

### Deleted from `server/services/community/streaming.go` (then the file goes)

`StreamCommunityEvents` handler, `runStreamLoop`, `streamSender`,
`checkAndTerminateIfDeleted`, `registerEventStream`, `unregisterEventStream`,
`BroadcastCommunityEvent`, `HasActiveEventStream`, `eventStreamInfo`.

### Deleted elsewhere

- `service.go`: `eventStreamsMu` / `eventStreams` fields (lines 44-45) and both
  constructor initializations (lines 95, 113).
- `stream_subscriber.go`: the `BroadcastCommunityEvent` call and its comment
  (lines 61-64). Keep the `hasAnyUserStream()` gate — it is what stops an idle
  server paying for a membership query per event.
- `proto/ripls/api/community_service.proto`: the RPC (line 1204-1206) and both
  messages (`StreamCommunityEventsRequest` :819, `StreamCommunityEventsResponse`
  :835). **Reserve nothing.** `proto_conventions.md` § *Removing an RPC* is
  explicit: "Reserve nothing: RPC names and numbers are not reused the way field
  numbers are," and both messages go entirely so there is no retained field
  number to protect. The issue's "(reserve the tags)" is contrary to convention.
- `server/.golangci.yml` lines 383-392: the SA1019 carve-out, which names #2869
  as its removal trigger.

### Signature collapse — wider than the issue implies

The issue says `HasActiveEventStream` "collapses to `HasActiveUserStream(userID)`,
and the `StreamChecker` signature can drop its `communityID` parameter." That
touches five files, not one:

| file | change |
|---|---|
| `notifications/community_subscriber/subscriber.go:38` | `type StreamChecker func(userID string) bool` |
| `notifications/community_subscriber/subscriber.go:163` | `streamCheck(userID)` — drop `event.CommunityId` |
| `wiring.go:274` | `SetStreamChecker(communityService.HasActiveUserStream)` |
| `integration_tests/test_helpers.go:961` | same rewire |
| `services/community/common_test.go:59,106` | same rewire |
| `notifications/community_subscriber/subscriber_test.go:40,53,226,392` | one-arg closures |

Note the *chat* subscriber has its own `StreamChecker` type
(`chat_subscriber/subscriber.go:24`) that stays `(conversationID, userID)`. They
are separate types in separate packages; do not "harmonize" them.

### Tests — the issue says delete, but two of them carry live invariants

`streaming_test.go` (889 lines, 24 tests) is not all redundant.
`user_streaming_test.go` (289 lines, 7 tests) does **not** cover these, and
`runUserStreamLoop` / `broadcastToUserStreams` implement every one of them:

| invariant | only test today | live equivalent |
|---|---|---|
| dead peer reaped on the next heartbeat, not at the lifetime cap | `TestRunStreamLoop_DeadPeerReapedOnHeartbeat` | `runUserStreamLoop` ticker branch |
| loop exits at the lifetime cap | `TestRunStreamLoop_ExitsAtLifetimeCap` | same |
| healthy stream keeps heartbeating | `TestRunStreamLoop_HealthyStreamKeepsSendingHeartbeats` | same |
| broadcast clones per recipient | `TestBroadcastCommunityEvent_ClonesMessage` | `broadcastToUserStreams` `proto.Clone` |
| broadcast does not race unregister | `TestBroadcast_DoesNotRaceWithUnregister` | RLock-snapshot pattern |
| unregister does not close the channel | `TestUnregisterEventStream_DoesNotCloseChannel` | same pattern, same panic risk |
| dead peer ends up unregistered | `TestStreamCommunityEvents_UnregistersSubscriberAfterDeadPeer` | `defer unregisterUserStream` |
| push is **sent** to a non-streaming user | `TestPushSuppression_SendsToNonStreamingUsers` | only the *suppress* side is covered per-user |

Deleting the file without porting these leaves the live path with no test for the
zombie-stream guarantee or the send-on-closed-channel hazard. That last one is a
panic, not a regression.

### Integration test — port, do not delete

`integration_tests/community_broadcast_request_id_test.go` is the only test that
proves `request_id` survives the real bus → broadcast → Connect-stream path; the
unit tests use fake senders. Port it (`git mv` to
`user_broadcast_request_id_test.go`) with these substitutions:

- `StreamCommunityEvents(…{CommunityId, SinceUnixSec})` → `StreamUserEvents(…{SinceUnixSec})`
- log assertion `"registered event stream"` → `"registered user event stream"`
  (and drop the `community_id` predicate — the register line has none)
- log assertion `"broadcasting community event"` → `"broadcasting event to user streams"`
- entry field `event_id` → `community_event_id`

### Docs

- `docs/realtime_updates.md`: topology diagram line 29, the "per-community RPCs
  remain, deprecated" paragraph (112-114), § *Stream Registries* opening
  (152-153), § *Push Suppression* second paragraph (165), key-files row (260).
  Re-stamp `freshness.verified_commit` / `verified_on`.
- **`docs/community_delete_and_leave.md:640-643`** — states that "any open
  `WatchCommunity*` streams should terminate cleanly with a `community_deleted`
  signal so the client can route the user away. See `server/streaming/`." After
  this change *no* stream terminates on delete. (The line is doubly stale: neither
  `WatchCommunity*` nor `server/streaming/` exists — that part predates #1658.
  Fix both while here.)
- **`server/services/community/README.md`** — wider than the issue's one line.
  Line 15 names `streaming.go` / `StreamCommunityEvents`, *and* `user_streaming.go`,
  `user_events.go` and `stream_subscriber.go` are absent from the key-files list
  entirely (a #2867 gap). The intro paragraph's "streaming community events"
  needs the same pass.
- `server/notifications/community_subscriber/README.md:27, 105, 115`.
- `docs/server/observability.md:215` — names `StreamCommunityEvents` as an
  example; swap for `StreamUserEvents`.
- `npm run lint:context-map` must pass.

## The one behavior this deletes with no replacement

Distinct from the stranded-client risk, and not covered by it — this affects
**current** clients, not old ones.

`checkAndTerminateIfDeleted` + the `COMMUNITY_DELETED` terminal branch in
`runStreamLoop` are #1658's work: a viewer of a community that gets deleted
mid-session receives a synthetic `COMMUNITY_DELETED` event and a clean EOF, which
is what routes them off the screen. #2867 deliberately did **not** carry that to
the per-user stream — terminating would take every other community's realtime
with it — and both `docs/realtime_updates.md:127` and the #2867 plan justify it
with "the client navigates away on its own."

**It does not.** `app/lib/services/event_router.dart`'s switch (lines 95-190) has
no `COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED` case; the event falls through to the
default `_log.warning('unhandled event type: …')` at line 190. So today the
per-community stream's termination is the *only* mechanism that gets a viewer off
a deleted community, and this change removes it.

The consequence is bounded — the community is soft-deleted, so subsequent reads
fail or return nothing and the screen degrades rather than lying — but it is a
real regression against documented behavior, and it would have been introduced by
this PR rather than inherited.

> **DECISION (2026-08-09): fixed in this PR, not deferred to a follow-up.** The
> first draft proposed filing an issue and shipping the removal anyway. The
> maintainer chose to fix it here. Investigating it turned up that the whole
> **community-lifecycle family** was unrouted — `COMMUNITY_DELETED`,
> `COMMUNITY_RESTORED`, `COMMUNITY_NAMED`, `OWNERSHIP_TRANSFERRED` and
> `MEMBER_REJOINED_WITHIN_WINDOW` all hit the same default — so all five are
> handled, since fixing delete while leaving restore broken would be worse than
> either. See *As built*, step 3.

## What a stranded client actually experiences

Worth establishing before choosing how long to wait, because it is milder than
"the screen breaks" and the issue does not say.

The shipped `v0.29.0` client (`app/lib/services/community_event_stream.dart` at
that tag) opens one stream per community at startup and on resume. After removal
each open returns `unimplemented`. Consequences, in order of how much they
matter:

- **Realtime degrades, correctness does not.** `ListCommunityEvents` is *not*
  deprecated and stays. The old client's 30s per-community poll keeps running and
  keeps the event router fed.
- **FCM push is restored for those users.** Push suppression keys on stream
  presence (`HasActiveEventStream`), so a user with no stream starts receiving
  the pushes that were previously suppressed. Delivery improves.
- **A permanent reconnect loop.** `_connectCommunity`'s `onError` handler
  reconnects **unconditionally** with a global exponential backoff capped at 60s,
  and — this is the teardown defect catalogued as constraint 5 in the #2867 plan
  and fixed only on `main` — it ignores the `_streams` map, so even
  `disconnectAll()` / logout does not stop it. A stranded build with N
  communities issues ~N failing opens per 60s forever. At the ad-hoc community
  counts the phone-first pivot produces, that is real, if small, garbage traffic
  and log volume.

So the removal is low-risk for users and self-limiting in blast radius, but the
stranded population can never stop retrying on its own. That asymmetry is the
argument for letting this ride an extra release rather than removing at the first
legal moment.

## Potential Solutions

### Option A: One removal PR ← **chosen, with the gate waived**

- **Approach:** Land everything in the inventory above as a single PR. As
  written here it still waited on the release gate; the decision at the top
  drops that second half, so the PR lands now.
- **Pros:** One reviewable unit; the deprecated surface and its scaffolding leave
  together, which is how #2526 and #2835 did it.
- **Cons:** Buries the test ports and the `communityEventToItem` move — the only
  parts with any thinking in them — inside a large deletion diff, where they read
  as noise. And it leaves `runUserStreamLoop` untested for dead-peer reaping and
  lifetime-cap exit for however many weeks the gate takes, on the path that is
  actually serving traffic.
- **Effort:** Medium.

### Option B: Prep PR now, removal PR when the gate opens

- **Approach:** Split on wire exposure. Everything that is *not* wire-breaking —
  moving `communityEventToItem`, porting the eight loop/registry invariants to the
  per-user path, porting the integration test — lands immediately. The removal PR
  is then proto + handler + registry + carve-out + docs, and deletes only tests
  that are provably redundant by then.
- **Pros:** The coverage ports are worth having on their own merits: they cover
  the live code path, today, independent of whether the removal ever happens. The
  removal PR shrinks to something a reviewer can check against the gate rather
  than against a helper relocation. Matches the repo's "prove the replacement
  first" pattern.
- **Cons:** Two PRs; a short window where the ported tests duplicate the
  originals.
- **Effort:** Small + small.

### Option C: Keep the RPC, collapse the implementation onto the per-user registry

- **Approach:** Decide not to remove. Reimplement `StreamCommunityEvents` as a
  thin filter over `userStreams`, so there is one registry and one broadcast path
  while the wire surface stays forever.
- **Pros:** No client-aging risk at all. Kills the duplicate-registry maintenance
  cost, which is the real cost of the deprecation.
- **Cons:** Nets *more* code, not less — a translation layer plus a permanently
  deprecated wire surface. The SA1019 carve-out has to stay, since the types stay
  deprecated. And it re-legitimizes an RPC whose shape is the thing #2867
  concluded was wrong.
- **Effort:** Medium.

## Recommendation

**Option A, soak waived — decided 2026-08-09.** One PR, landing now.

What that trades away, stated plainly so the trade is on the record rather than
implied: the removal ships in the same release that first delivers the per-user
client, so there is no interval during which a client can migrate before the
server drops the old surface. Every build at `v0.29.0` or earlier becomes a
stranded build the moment that release goes out.

What makes that acceptable here is the failure mode, not the population size.
`ListCommunityEvents` is not deprecated and stays, so a stranded build keeps its
poll backstop and keeps converging; push suppression stops firing for it, so FCM
delivery *improves*. Nothing renders blank and no screen breaks — the visible
change is realtime latency going from sub-second to ≤30s on builds that never
update. The residual cost is the permanent reconnect loop described above, which
is traffic and log noise rather than a user-facing defect. Prod being 12 users
makes it cheap; the graceful degradation is what makes it *safe*.

The one thing not traded away is the test-port work. It moves inside the single
PR rather than disappearing: `runUserStreamLoop` has no test today for dead-peer
reaping or the lifetime cap, and `broadcastToUserStreams` none for the clone or
the unregister race — those tests exist but point at the code being deleted.
Landing the deletion without them would leave the live path with no coverage of
a send-on-closed-channel panic hazard, which is a different kind of risk from
the one that was accepted.

Option C loses on arithmetic — it trades two registries for one registry plus a
translation layer plus a permanent deprecated RPC. Recorded only so nobody
re-proposes it as "the safe option."

## Open Questions

- **~~How many releases do we ride?~~** — **Resolved: one, `v0.29.1`.** The soak
  was waived on 2026-08-09 when no release carried the client; `v0.29.1` shipped
  that evening and satisfies the policy on its own terms as of ~17:35 on
  2026-08-10. What is still waived is *adoption*: two installs were still opening
  the deprecated RPC ~22h after the tag. See the posture update.
- **~~Merge now, or wait for the deprecated opens to reach zero?~~** —
  **Resolved (2026-08-12): merge.** Two releases now carry the client, both
  soaked, and adoption has turned over — 5 distinct users on the new stream in
  the last 24h against 1 on the old. Waiting for zero has no defined end: the
  stragglers are *different accounts* week to week, so this is update churn, not
  a fixed holdout set that will drain. There is no force-upgrade and no version
  telemetry to make zero reachable.
- **Does `HasActiveEventStream` collapse, or does the two-arg shape stay?** —
  Recommendation: collapse, and delete `HasActiveEventStream` entirely rather
  than keeping it as a one-line forwarder — wire `SetStreamChecker` straight to
  `HasActiveUserStream`. A retained-but-unused `communityID` parameter is exactly
  what `unparam`/`revive` were turned on for in #1611, and the carve-out that
  would hide it is being deleted in the same PR.
- **Where does `communityEventToItem` land?** — Recommendation: `events.go`. Its
  comment already names that file as the thing it must not drift from, and
  co-locating the two denormalizers is the cheapest way to keep that true.
  `events.go` goes from 90 to ~135 lines, nowhere near the gate.
- **Port the integration test or delete it?** — Recommendation: port. It is the
  only end-to-end proof that `request_id` reaches the broadcast log through a
  real Connect stream; every other test in this area uses a fake sender. Deleting
  it would remove observability coverage that #1613's correlation work depends
  on, to save nothing.
- **Is there any non-Flutter caller?** — Recommendation: no, and this is already
  checked. A repo-wide grep across `.ts`, `.dart`, `.go`, `.json` finds
  `StreamCommunityEvents` only in the proto, the Go server, the one integration
  test, and docs — nothing in `e2e/` (protobuf-es), nothing in `website/`. Web
  clients are served fresh and cannot strand. Re-run the grep in Phase 3 anyway;
  it costs nothing and #2834 is the precedent for a proto reader surface being
  missed.
- **~~Does the missing client `COMMUNITY_DELETED` handler block this PR?~~** —
  **Resolved: fixed in this PR** (see the decision block above). The plan
  originally recommended filing it and shipping anyway.
- **Should we build version telemetry so a future removal has certainty rather
  than a convention?** — Recommendation: not here, and do not let it block this.
  `proto_conventions.md` § *When the wait is not enough* already names the gap,
  and this removal's failure mode (degrade to poll, push restored) is squarely
  inside what the convention is designed to tolerate. File it separately if a
  removal ever comes along whose absence would corrupt rather than degrade.

## Implementation Plan

> **As built (2026-08-09), branch `remove_community_stream`.** Four commits,
> matching the stages below plus the client fix folded in as stage 3:
>
> 1. **Coverage first.** `communityEventToItem` moved to `events.go`. Eight
>    invariants ported to the per-user path — dead-peer reaping, lifetime cap,
>    heartbeat liveness, clone-per-recipient, the broadcast/unregister race,
>    channel-not-closed-on-unregister, dead-peer deregistration, and the negative
>    push-suppression case. Each verified by mutation: removing `proto.Clone`
>    reddens the clone test, adding `close(stream.ch)` on unregister reproduces
>    the send-on-closed-channel panic, and ignoring heartbeat `Send` errors makes
>    the dead-peer test hang to the cap. The `request_id` integration test moved
>    to `StreamUserEvents` and passes against the real transport.
> 2. **The deletion.** RPC + both messages, the handler, `runStreamLoop`,
>    `checkAndTerminateIfDeleted`, the registry, `BroadcastCommunityEvent`, the
>    subscriber branch, `streaming.go`, `streaming_test.go`, and the SA1019
>    carve-out. `StreamChecker` collapsed to `func(userID string) bool` across
>    six call sites. Nothing reserved.
> 3. **The client fix** (finding 2 of the audit, folded in rather than deferred):
>    all five community-lifecycle event types routed in `event_router.dart`, with
>    the two list targets clearing the repository cache rather than only bumping
>    a notifier — the view models re-read through that cache on rebuild, so a
>    bump alone would have rebuilt them onto the same stale list. Seven tests,
>    mutation-verified.
> 4. **Docs.** Five sites in `realtime_updates.md` (freshness re-stamped),
>    `community_delete_and_leave.md`'s stream-termination claim, both READMEs,
>    `observability.md`, `update_system_tests.md`, and the e2e spec prose.
>
> **Verified green:** full `go test ./server/...`, `go test -tags=integration`,
> `flutter test` (4,335 tests), and `npm run lint` (exit 0, including
> `lint:go` with no carve-out, `lint:context-map`, and `lint:e2e:types`).
> `flutter analyze` clean after `npm run generate`, which is what proves no Dart
> reader survived the proto deletion.

One PR on a branch off `main`. The three stages below are commit boundaries
inside it, not separate PRs — ordered so the tree compiles and the suite is
green at each one, which is what makes the deletion commit reviewable on its
own.

### Stage 1: Move the helper and port the coverage

**Goal:** Nothing the live code needs is left in `streaming.go`, and every
invariant the deletion would drop is already held by a test pointing at
per-user code. This is the part the accepted risk does *not* cover, so it goes
first rather than last.

**Tasks:**
- [ ] Move `communityEventToItem` (with its doc comment) from `streaming.go` to
      `events.go`; reword the comment so it reads as the shared converter rather
      than the stream's private helper.
- [ ] Port to `user_streaming_test.go`: dead-peer-reaped-on-heartbeat,
      exits-at-lifetime-cap, healthy-stream-keeps-heartbeating,
      broadcast-clones-message, broadcast-does-not-race-with-unregister,
      unregister-does-not-close-channel, dead-peer-ends-up-unregistered.
- [ ] Add the push-suppression **negative** case for user streams
      (`TestPushSuppression_SendsToUsersWithoutUserStream`) — only the suppress
      side exists today.
- [ ] For each ported test, verify it fails with the behavior removed. Per the
      #2867 precedent, watch it go red; do not assume.
- [ ] `git mv server/integration_tests/community_broadcast_request_id_test.go
      server/integration_tests/user_broadcast_request_id_test.go` and apply the
      four substitutions listed in *Removal inventory*. Keep `since = 0` — the
      original passes it to force an immediate catch-up `Send` so the Connect
      client returns instead of blocking to the first 30s heartbeat, and
      `sendUserCatchUp` clamps it to 7 days ago, so the JOINED events still
      flush the headers. The observer must stay a real member: the per-user path
      resolves membership live per event, so a non-member observer receives
      nothing and the test hangs rather than failing usefully.
- [ ] `go test ./server/...` and `go test -tags=integration ./server/...` green
      locally — not "CI will tell us."

**Validates:** `runUserStreamLoop` and `broadcastToUserStreams` are covered on
their own, independent of the file about to be deleted. This closes a real gap
that exists whether or not #2869 ever lands.

### Stage 2: Delete the RPC and its scaffolding

**Goal:** One registry, one stream path, no carve-out.

**Tasks:**
- [ ] Proto: delete the `StreamCommunityEvents` RPC and both messages. Reserve
      nothing. Run `npm run lint:proto:breaking` knowing a green result is not
      evidence (`RPC_NO_DELETE` is `FILE`-scoped).
- [ ] `npm run generate`, then `flutter analyze` — the Dart-compile liveness gate
      from #2820. A surviving Dart reader shows up as a compile error.
- [ ] `npm run lint:e2e:types` — the protobuf-es reader surface (#2834's four
      surfaces; expected clean, but check rather than assume).
- [ ] Delete the handler and its scaffolding; delete `streaming.go` once empty.
- [ ] Delete `eventStreams` / `eventStreamsMu` and both constructor inits in
      `service.go`.
- [ ] Delete the `BroadcastCommunityEvent` branch in `stream_subscriber.go`;
      keep the `hasAnyUserStream()` gate and update its comment (it is no longer
      "the per-user half," it is the only half).
- [ ] Collapse `community_subscriber.StreamChecker` to `func(userID string) bool`
      and rewire all six call sites in the table above.
- [ ] Delete `streaming_test.go`, having confirmed against Stage 1 that nothing
      in it is now the sole holder of an invariant.
- [ ] Delete the SA1019 carve-out (`server/.golangci.yml:383-392`) and run
      `npm run lint:go` — no new `unparam` / `unused` findings.
- [ ] `npm run lint:go:size` (files shrink, but `events.go` grew).

**Validates:** The duplicate registry is gone and the lint gate stays green
without a carve-out — which is what proves the deprecation scaffolding left
with the RPC rather than outliving it.

### Stage 3: Docs, and the note the waiver requires

**Goal:** No doc describes a stream that no longer exists, and the waived soak is
recorded where the next person looking at an RPC removal will find it.

**Tasks:**
- [ ] File the client `COMMUNITY_DELETED` route-away issue (see *The one behavior
      this deletes with no replacement*) and get its number — the doc edit below
      references it.
- [ ] Update the five sites in `docs/realtime_updates.md`; rewrite line 127 so it
      states the gap and links the new issue instead of asserting a client
      behavior that does not exist. Re-stamp freshness.
- [ ] Update `docs/community_delete_and_leave.md:640-643` — no stream terminates
      on delete now; drop the dead `WatchCommunity*` / `server/streaming/`
      references while there.
- [ ] Update `server/services/community/README.md` — fix line 15 *and* add the
      three files #2867 never listed (`user_streaming.go`, `user_events.go`,
      `stream_subscriber.go`), plus the intro paragraph.
- [ ] Update `server/notifications/community_subscriber/README.md:27,105,115`
      and `docs/server/observability.md:215`.
- [ ] `npm run lint:context-map`.
- [ ] In the PR description, state that the `proto_conventions.md` § *Removing an
      RPC* wait was **waived deliberately**, with the reason (graceful degrade to
      the `ListCommunityEvents` poll; push suppression lifts). Do not amend the
      convention doc — this is one accepted exception, not a policy change, and
      writing it into the policy would turn a judgment call into a precedent.

**Validates:** The docs match the code, and the exception is legible as an
exception.

### After the release

Not blocking the PR, but the removal is not "done" until this is checked:

- [ ] For a week after the release carrying this, count `unimplemented` responses
      on `CommunityService/StreamCommunityEvents` in prod. That number *is* the
      stranded population — the measurement the soak would have taken in advance,
      taken after the fact instead.
- [ ] If it is non-trivial, record in the issue that those builds cannot stop
      retrying on their own (the `v0.29.0` `onError` defect reconnects
      unconditionally, ignoring `disconnectAll()`), so the only remedy is a
      client update and the traffic is otherwise harmless.
- [ ] Close #2869.

## Related Issues

- **#2867** — the per-user collapse that deprecated this RPC; its plan
  (`docs/issues/2867-realtime-fanout.md`) is the source for the teardown defects
  and the as-built server design.
- **#2492** — the phone-first pivot whose ad-hoc per-item communities invalidated
  the "users belong to 1-5 communities" premise, and thus the whole per-community
  shape.
- **#2835** — rewrote the soak policy this gate uses (one shipped release of any
  magnitude, plus 24h).
- **#2526** — worked example for an RPC removal, and the source of the
  "wrappers are not callers" rule applied in the Open Questions grep.
- **#2820** — established `flutter analyze` after `npm run generate` as the
  liveness gate for proto deletions.
- **#1658** (closed) — built the synthetic `COMMUNITY_DELETED` + clean-EOF
  termination that `checkAndTerminateIfDeleted` implements. This PR retires that
  guarantee outright; #2867 already declined to carry it to the per-user stream.
  See *The one behavior this deletes with no replacement*.
- **#1611** — the lint gate the SA1019 carve-out lives in; deleting the carve-out
  is what closes the loop opened there.
- **#1658's successor is `event_router.dart`**, not a follow-up issue — the
  client-side community-lifecycle handling landed in this PR rather than being
  deferred.
- **#2869 blocks nothing** — no other issue depends on this landing.

---

## Audit notes

Audited against the documentation context map. Changes made to the plan as a
result:

1. **Found a behavior this deletes with no replacement, and it is not the
   stranded-client risk.** `checkAndTerminateIfDeleted` is the only thing that
   routes a viewer off a community deleted mid-session. `realtime_updates.md:127`
   and the #2867 plan both justify dropping it per-user with "the client
   navigates away on its own" — but `event_router.dart`'s switch (95-190) has no
   `COMMUNITY_DELETED` case and the event hits the `unhandled event type`
   default at line 190. Verified by reading the switch, not inferred. Now its own
   section, an Open Question, and a Stage 3 task to file the follow-up before
   the doc sentence is rewritten.
2. **Added `docs/community_delete_and_leave.md:640-643` to the docs list.** It
   asserts open streams terminate with a `community_deleted` signal — false after
   this change. A durable doc whose globs cover `services/community/lifecycle.go`,
   so it is in scope and the first draft missed it. It is also stale
   independently (`WatchCommunity*`, `server/streaming/` — neither exists).
3. **Widened the `services/community/README.md` task.** The issue says line 15;
   the file is also missing `user_streaming.go`, `user_events.go` and
   `stream_subscriber.go` entirely — a #2867 gap that this PR is the natural
   place to close, and that the multi-file-directory README rule in CLAUDE.md
   requires.
4. **Specified the integration-test port's two failure modes** rather than
   leaving "apply the substitutions": keep `since = 0` (it is what flushes
   headers instead of waiting 30s for a heartbeat), and keep the observer a real
   member (per-user delivery filters on live membership, so a non-member observer
   makes the test *hang*, not fail).
5. **Verified the load-bearing claims** instead of asserting them: `buf.yaml`
   uses `WIRE`, so deleting both messages alongside the RPC is permitted;
   `BroadcastCommunityEvent`'s only non-test caller is `stream_subscriber.go:64`;
   no Terraform log-metric, SLO, dashboard or alert references
   `StreamCommunityEvents` or the `stream_count` field (the SLO's streaming
   exclusion is the generic `.*/Stream.*` regex, which keeps matching
   `StreamUserEvents`); `docs/server/architecture.md`'s `SetStreamChecker`
   example is the *chat* subscriber and needs no edit.

**Checked and not a finding — security.** The per-community handler gated on
`auth.RequireMemberOfActiveCommunity` at open; the per-user stream has no
open-time gate by design (#2867), filtering every event through live membership
at delivery instead. Deleting the handler therefore removes no authorization that
is not otherwise enforced. Called out because the diff *looks* like it deletes an
authz check.

**Dimensions engaged:** api_design (RPC + message deletion, WIRE compatibility,
the reserve-nothing convention), tech_debt (primary — deprecated-code removal,
carve-out retirement, doc and README debt), testing (the eight ported invariants
and the integration-test port), code_quality (dead code, the `StreamChecker`
unused-parameter collapse), observability (log-field renames in the ported test,
confirming no metric keys on the removed lines), error_handling (the
`COMMUNITY_DELETED` termination path being retired).
**Not engaged:** security (see above — verified, not assumed), accessibility (no
rendered surface), performance (removal strictly reduces work), dependency (no
package changes), client work of any kind beyond regenerating and analyzing.

**Docs consulted:** `docs/llms.txt`; anchors `proto_conventions.md`,
`server/architecture.md`, `server/conventions.md`, `testing/architecture.md`,
`server/observability.md`; leaf docs `realtime_updates.md`,
`community_delete_and_leave.md`, `ad_hoc_communities.md`; review prompts
`api_design`, `tech_debt`, `testing`, `observability`, `code_quality`,
`error_handling`. Code read for verification:
`server/services/community/{streaming,user_streaming,stream_subscriber,service,events,user_events}.go`,
`server/services/community/{streaming,user_streaming,user_events}_test.go`,
`server/notifications/community_subscriber/subscriber.go`, `server/wiring.go`,
`server/.golangci.yml`, `buf.yaml`,
`proto/ripls/api/community_service.proto`,
`server/integration_tests/community_broadcast_request_id_test.go`,
`app/lib/services/event_router.dart`,
`app/lib/services/community_event_stream.dart` @ `v0.29.0`,
`terraform/modules/monitoring/{log_metrics,slos}.tf`. Live state read:
`ripls-prod` Cloud Logging and the `server` Cloud Run revision.
