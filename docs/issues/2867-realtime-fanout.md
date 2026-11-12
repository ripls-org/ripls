# Issue #2867: Unbounded per-community event streams starve every other RPC (infinite Home spinner on HTTP/1.1)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2867
**Filed:** 2026-08-08 | **Last activity:** 2026-08-08
**Labels:** bug, P1 | **Assignees:** unassigned
**Context docs:** `docs/realtime_updates.md`, `docs/ad_hoc_communities.md`,
`docs/client/architecture.md`, `docs/client/conventions.md`,
`docs/client/startup.md`, `docs/community_health.md` (skim),
`docs/reviews/performance_review_prompt.md`,
`docs/reviews/error_handling_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`, `docs/reviews/observability_review_prompt.md`

> **On `ripls-ui-investigate`:** not run separately. The issue is UI-observable
> (an infinite Home spinner), but it was already reproduced live against a local
> server with a driven browser during the debugging session that filed it, and
> bisected past the UI layer to browser connection-pool exhaustion — with
> request-level and `lsof` evidence, reproduced in both Firefox and Chromium.
> The verdict that skill produces is already in the Problem Statement below.

## Problem Statement

On sign-in the client opens **one long-lived `StreamCommunityEvents` RPC and one
30-second poll timer per community the user belongs to**, with no cap
(`main.dart:576-577` → `CommunityEventStreamService.connectAll` /
`CommunityEventPoller.startAll`). The realtime design that introduced this
(`docs/realtime_updates.md`) states its own premise explicitly:

> Users belong to 1-5 communities, so the connection count is trivial.

**That premise is no longer true.** The phone-first pivot (#2492) made every
item — event, request, giveaway, loan — provision an ad-hoc per-item community
at creation time. Community count now grows with *item* count, not with how many
standing groups a person has joined. In the local simulation database:

| scope | ad-hoc (unnamed) | named |
|---|---|---|
| `marcus@example.com`'s memberships | 71 | 1 |
| all communities | 425 | 1 |

`ListCommunities` returned **48** of those 71 (host-only ad-hoc communities are
suppressed server-side — `membership.go`, documented in `ad_hoc_communities.md`
§Surfacing), so the client opened 48 streams and 48 poll timers.

This surfaces two distinct defects, with different blast radii:

**1. Connection starvation (the reported symptom, local/web only).** A browser
on HTTP/1.1 allows ~6 connections per origin. Streams never complete by design,
so the first 6 consume every slot permanently and **every subsequent request is
queued in the browser and never sent** — including `GetHomeView`. The Home tab
spins forever. Verified by driving a real browser against a local server:

```text
browser side:   PortfolioService/GetHomeView          NEVER COMPLETED
                FeedService/GetFeed                   NEVER COMPLETED
                SearchService/Search                  NEVER COMPLETED
                CommunityService/ListCommunityEvents  x19  NEVER COMPLETED
                MediaService/GetMedia                 x17  NEVER COMPLETED

server side:    grep -c GetHomeView server.log  ->  0
```

The browser issued `GetHomeView`; the server never received it. `lsof` confirmed
exactly six ESTABLISHED Firefox→:8080 connections, idle, all held by
`StreamCommunityEvents`.

**2. Fan-out load (all platforms, production, today).** This one is not
hypothetical and is arguably the more important finding:

- **Poll QPS scales with membership.** 48 communities × 1 request / 30s = 1.6
  req/s **per user**, versus the ~0.1 req/s the design assumed. At the doc's own
  1,000-concurrent-user scenario that is ~1,600 req/s instead of ~100 req/s.
- **Server goroutines scale with total memberships.** ~48 streams/user instead
  of ~3 — 48,000 blocked goroutines at 1,000 users instead of 3,000
  (`docs/realtime_updates.md` §Server Load Estimates is off by ~16×).
- **Poll coverage degrades as membership grows.** `startAll` staggers timers by
  3s × index, so with 48 communities the last one is not polled until **141
  seconds** (index 47 × 3s) after startup.
- Mobile pays this in battery and cellular data.

Native mobile does not hit defect 1 — `auth_providers.dart:26` selects
`package:connectrpc/http2.dart` on `dart.library.io`, which multiplexes over one
HTTP/2 connection. Web in production reaches Cloud Run over HTTPS and likewise
gets HTTP/2. Only `http://localhost:8080` is forced to HTTP/1.1, because
browsers refuse to negotiate cleartext HTTP/2 even though the server offers h2c
(`server/main.go:232-238`). So defect 1 is a **local-dev and e2e blocker today
with a latent production cliff** (Go's default `MaxConcurrentStreams` is 250 and
is not overridden); defect 2 is **live in production on every platform**.

### A third-order effect worth designing around

Push suppression is keyed on stream presence: the push subscriber skips FCM for
any recipient with an active stream for that community
(`HasActiveEventStream`, wired at `wiring.go:274`). So **removing a stream
restores push for that community** — correctness improves, FCM volume rises.

And the mirror image, which constrains the solution: **web has no FCM at all**
(`main.dart` gates FCM init on `!kIsWeb`). On web, a community that is neither
streamed nor polled has *no* realtime path whatsoever. Any design that simply
drops ad-hoc communities from the realtime layer silently removes live updates
from the event page a web user is actively looking at.

## Context from Comments

No comments — the issue was filed today from a live debugging session, and the
reproduction, `lsof` output, and named/ad-hoc breakdown in the body come from
that session rather than from discussion.

One clarification arrived out-of-band from the maintainer and is the reason this
plan exists in its current shape:

> it doesn't likely make sense to have streams/polls for unnamed / ad hoc
> per-item communities

That is the right instinct and the measurements back it (99% of this user's
communities are ad-hoc). The plan below adopts it, with one qualification
discovered while reading the code: ad-hoc communities *do* need realtime while
the user is looking at the item they belong to, and on web there is no push
fallback to cover the gap. So the rule is "don't subscribe to ad-hoc communities
*in the background*", not "never subscribe to ad-hoc communities".

## Constraints discovered during research

These shaped the options and rule several obvious designs out:

1. **There is no "currently selected community."** The per-community filter was
   removed (#1895/#2040 — see the `TODO(#2040)` in
   `gen_experience_view_model.dart:821`). Home, Feed, Directory and Workshop are
   all portfolio-wide. A "stream only the active community" design has nothing
   to hang off.
2. **Both realtime RPCs are single-community.** `ListCommunityEventsRequest` and
   `StreamCommunityEventsRequest` each take one `community_id`
   (`community_service.proto:821`, `:840`). Batching requires a proto change.
3. **The client can already tell ad-hoc from named, and rank by recency.**
   `CommunityItem` carries `name` (empty ⇒ ad-hoc — the canonical discriminator
   per `ad_hoc_communities.md`) and `last_activity_unix_sec`, which already
   drives the directory's recency ordering. No new server field is needed for
   prioritization.
4. **Streams and polls are admitted through two entry points**, not one:
   `connectAll`/`startAll` at startup and resume (`main.dart:576-577`,
   `:631-632`), and `connectCommunity`/`startCommunity` on community join
   (`community_providers.dart:63`). Any budget has to be enforced in the service,
   not at the call site, or the join path bypasses it.
5. **Two latent teardown bugs in the same two files — both violate a documented
   logout invariant.** `docs/client/logout.md` §Invariants requires that
   "pollers/streams stop before caches clear so in-flight events don't
   repopulate just-cleared caches". Both services can currently route events
   *after* `reset()`:
   - `_connectCommunity`'s `onError` handler reconnects unconditionally, while
     `onDone` correctly checks `_streams.containsKey(communityId)` first
     (`community_event_stream.dart:93-111`). A stream error arriving after
     `disconnectAll()` resurrects a stream that nothing will ever close.
   - `CommunityEventPoller._pollCommunity` awaits `listCommunityEvents` and then
     calls `routeCommunityEvents` with no post-await guard
     (`community_event_poller.dart:86-106`). A poll in flight when `reset()`
     fires still routes its results.

   Either one lets the *previous* user's events invalidate caches during or
   after the *next* user's login. That makes these correctness bugs on the
   logout path, not merely cap violations, and they must be fixed as part of
   this work — a budget that a resurrected stream can silently exceed is not a
   budget.
6. **The reconnect backoff counter is global, not per-community.**
   `_reconnectAttempts` is a single field on the service
   (`community_event_stream.dart:24`), so one flapping community inflates the
   delay for all of them and any successful event anywhere resets it. Tolerable
   at 3 communities; with admission control, eviction and re-admission churn
   flow through the same counter, so the implementer must decide deliberately
   whether to make it per-community rather than inherit the behavior by
   accident. See Open Questions.

## Potential Solutions

### Option A: Bounded budget in the client services

- **Approach:** Enforce admission control inside `CommunityEventStreamService`
  and `CommunityEventPoller`. Cap concurrent streams at a small constant, ranked
  named-first then ad-hoc by `last_activity_unix_sec`. Replace the poller's
  N-timers-at-30s with **one** timer that round-robins a bounded slice of
  communities per tick, so poll QPS per user is constant regardless of
  membership count and every community is still covered within a bounded window.
- **Pros:** Client-only, no proto or server change, ships fast. Fixes the P1
  symptom and cuts production poll QPS and server goroutines by ~16× at once.
  Keeps the poll's documented "correctness backstop" role for *every* community
  rather than dropping some entirely. Degrades gracefully — more communities
  means a longer coverage cycle, not a broken app.
- **Cons:** Sub-second stream latency becomes partial and priority-ordered. The
  constants (stream budget, slice size) are judgment calls with no principled
  derivation. Does not change the per-community RPC shape, so the underlying
  N-proportional design survives.
- **Effort:** Small–medium.

### Option B: Subscribe to ad-hoc communities on demand only

- **Approach:** Exclude ad-hoc communities from the startup/resume fan-out
  entirely. Open a stream for one only while its item is on screen (event,
  request, gear, transfer detail), and close it on navigate-away.
- **Pros:** Directly encodes the maintainer's instinct and the domain
  semantics — a single-item audience matters exactly while you look at that item.
  Takes the seeded user from 72 subscriptions to 1. No proto change.
- **Cons:** Named communities stay unbounded (small in practice, but nothing
  enforces it). Needs subscribe/unsubscribe wiring at four-plus detail screens,
  and a screen that is missed loses realtime *silently* — the failure mode is
  invisible until someone notices staleness. Doesn't reduce poll load for named
  communities.
- **Effort:** Medium.

### Option C: Collapse the realtime layer to per-user RPCs

- **Approach:** Add `StreamUserEvents` and `ListUserEvents` — one stream and one
  poll covering every community the user belongs to. The server resolves
  membership and fans out per user, which is what the push subscriber already
  does for recipients. Deprecate the per-community pair.
- **Pros:** One connection and one poll per user *regardless of membership*.
  Eliminates both cliffs permanently and matches the portfolio-wide app the
  client has actually become. Server goroutines drop from Σ(memberships) to
  N(users). Restores true sub-second coverage everywhere, which A and B both
  give up.
- **Cons:** Largest change — proto, server stream registry, push suppression
  (`HasActiveEventStream(communityID, userID)` becomes "does this user have any
  stream"), catch-up replay across communities, mid-stream membership changes,
  and a deprecation window for the old RPCs. Too large to be the P1 fix.
- **Effort:** Large.

> **DECISION (2026-08-08): Option C, directly.** The maintainer chose to skip the
> A+B stopgap and do the per-user collapse now. Everything below the
> Recommendation heading is the reasoning as it stood before that call; the
> **Implementation Plan** section has been replaced with what was actually
> built. Options A and B were not implemented and no stopgap constants exist.
>
> The recommendation below argued for A+B *first* mainly on effort grounds. Two
> things it treats as costs of C turned out cheaper than expected once the code
> was read: resolving membership per event needs no new index-maintenance
> machinery (the push subscriber already resolves recipients the same way, and
> the community member cap bounds it), and mid-stream membership changes need no
> handling at all — live resolution means a join is covered with no client action
> and no reconnect. That last property also deleted work rather than adding it:
> the `onCommunityJoined` callback threaded through `CommunityRepository` was
> removed outright.

## Recommendation

**Ship A + B together as one client-only change, then file C as the durable
follow-up.**

A and B are complements, not alternatives, and the reason is the web/FCM gap in
the Problem Statement. B alone would strip realtime from ad-hoc communities in
the background — correct — but on web there is no push to cover the item page
the user is *currently* looking at, so B needs A's "pinned" slot to put that one
community back. A alone would leave a priority-ranked budget doing the work that
a domain rule (named vs ad-hoc) states more directly and more cheaply. Together:

- **Streams:** hard budget of `maxConcurrentStreams = 5`. The community whose
  item is on screen is *pinned* and always admitted. Remaining slots go to named
  communities first, then ad-hoc by `last_activity_unix_sec` descending.
- **Polls:** one timer, one bounded slice per tick, round-robin over every
  community. Constant QPS per user; worst-case coverage
  `ceil(N / slice) × tick` (with 48 communities, 4 per 10s tick ⇒ 120s), versus
  today's 144s just to *start* the last timer.

That is a small, testable, client-only change that closes the P1, cuts real
production load ~16×, and honours the "no background subscriptions for per-item
communities" rule — without silently removing live updates from a web user's
open event page.

C is the right end state and should be filed now so the constants introduced
here are understood as a stopgap rather than an architecture. It should not
block the P1 fix.

**What would change this recommendation:** if a measurement showed real
production users sit at 3–5 communities (i.e. ad-hoc accumulation is a
simulation artefact), then defect 2 evaporates, this drops from P1 to a
local-dev annoyance, and the right fix is the much smaller "cap streams at 6"
alone. See the first Open Question — it is worth answering before writing code,
because it is the difference between a stopgap and a rewrite.

## Open Questions

- **What is the real production distribution of communities per user?** —
  Recommendation: measure before implementing. A Metabase query over
  `community_user` joined to `community`, bucketed by name-presence, gives the
  named-vs-ad-hoc split per user at p50/p95/max in minutes. The plan's phasing
  holds either way, but the numbers decide whether C is urgent or icebox, and
  they belong in the issue regardless.
- **Stream budget of 5 — right number?** — Recommendation: 5. It sits under the
  ~6 HTTP/1.1 per-origin limit with one slot spare for unary traffic, which is
  what makes local dev work at all, and it is comfortably under any HTTP/2
  ceiling. Make it a named constant with the reasoning in a comment, not a
  literal.
- **Should the poll slice adapt to membership count?** — Recommendation: no,
  keep it constant. A slice that grows with N reintroduces the exact
  N-proportional load this change removes. Constant QPS with a lengthening
  coverage cycle is the correct trade for a backstop whose job is convergence,
  not latency.
- **Does dropping background streams for ad-hoc communities push FCM volume up
  enough to matter?** — Recommendation: expect a rise (push suppression keys on
  stream presence) and check it against the notification metrics after release
  rather than pre-emptively throttling. `ShouldNotify()` already filters to
  high-value targeted events, so the increment is bounded by event *type*, not
  by community count.
- **Should `origin_item`-based scoping replace name-presence as the "is this a
  per-item community" test?** — Recommendation: no. `ad_hoc_communities.md` is
  explicit that name-presence is the discriminator and `origin_item` is
  provenance retained across promotion. Using `origin_item` would wrongly
  classify a promoted, named community as ad-hoc forever.
- **Make the reconnect backoff per-community (constraint 6)?** —
  Recommendation: yes, and in this change rather than a follow-up. Admission
  control turns eviction into a routine event; a shared counter would let
  ordinary churn stretch every community's reconnect delay toward the 60s cap.
  The change is a `Map<String, int>` in place of one field, and the existing
  stream tests already cover the backoff path.

## Implementation Plan

**As built (Option C).** Five pieces, all landed on `realtime_fanout`:

### 1. Proto — per-user RPCs

`StreamUserEvents` and `ListUserEvents` in `community_service.proto`, each with
dedicated request/response types (no cross-RPC reuse, per
`proto_conventions.md`). Neither names a community. `StreamCommunityEvents` and
its messages are marked `deprecated` but kept working — already-installed mobile
clients still open them, and removing an RPC is a wire-breaking change.
`ListCommunityEvents` is deliberately **not** deprecated: reading one
community's activity is still a legitimate query, it is just no longer the
backstop.

### 2. Storage — one query instead of N

`FindEventsForUserSince` (`server/storage/community_events.go`) joins
`community_event` to `community_user` so membership is resolved *inside* the
query, with a time predicate, ordering, and a bound. The pattern it replaces —
`QueryByField` per community — has no time predicate and no limit, so it loaded
every community's complete event history into memory to discard almost all of
it. Ordered oldest-first so a truncated page stays resumable rather than
leaving a gap.

### 3. Server — per-user stream registry

`server/services/community/user_streaming.go`: a `userID → channels` registry
alongside the existing per-community one, plus the handler, catch-up, and
delivery loop. Two deliberate behavior changes, both because the subscription no
longer names a community:

- `COMMUNITY_DELETED` is no longer terminal. Ending the stream would take every
  other community's realtime with it.
- No membership gate at open time. There is no community to check; every event
  is filtered through live membership before delivery instead.

### 4. Server — fan-out and push suppression

`stream_subscriber.go` resolves the event's community members
(`communitylib.GetCommunityMemberIDs`) and delivers to their user streams. Live
resolution per event is what makes join and leave work with no client action —
it is the reason there is no per-community subscribe call any more. Gated on at
least one user stream being open so an idle server pays nothing, and bounded by
the 32-member community cap.

`HasActiveEventStream(communityID, userID)` keeps its signature but checks the
per-user registry first: one user stream satisfies the question for *any*
community, which is what preserves FCM suppression.

### 5. Client — one stream, one poll

`community_event_stream.dart` and `community_event_poller.dart` are single
subscriptions against the new RPCs. `main.dart` calls one `_startRealtime()`
with no community list. The poll also fires once on start so a resume does not
wait a full interval — affordable at one request.

The `onCommunityJoined` callback threaded through `CommunityRepository` is
**deleted**, not stubbed: with server-side membership resolution there is
nothing for it to do.

Both teardown defects from the Constraints section are fixed, each with a test
that fails without its guard (verified by removing the guard and watching the
test go red, not by assuming).

### Also done

- `docs/realtime_updates.md` rewritten where it was now false — the "1-5
  communities" premise, the topology diagram, the load estimates (which were off
  by ~16x), the lifecycle section, and the key-files table. Freshness re-stamped.
- `docs/ad_hoc_communities.md` § Lifecycle gained the generalized warning:
  per-item communities make community count grow with item count, so anything
  sized per community needs checking against that.

### Not done

- **Open Question 1 — the production distribution of communities per user — is
  still unanswered.** Option C is correct at any distribution, so it no longer
  gates the work, but the number is worth having: it sizes how much production
  load this actually removed.
- No *automated* e2e regression test for the many-communities case. It was
  verified by hand against the real transport instead — a driven browser signed
  in as `marcus@example.com` (48 communities) against a local HTTP/1.1 server:

  | | before | after |
  |---|---|---|
  | `GetHomeView` | never sent; server logged 0 | **119ms 200** |
  | `GetFeed` / `Search` | never sent | 320ms / 318ms, 200 |
  | poll requests | 48 timers | 1 `ListUserEvents`, 4ms |
  | pending requests after 60s | 40+ queued forever | 1 — the stream itself |
  | connections held | 6 (the browser's whole pool) | 1 |

  The unit tests pin the invariant structurally, which is stronger than a count
  assertion: there is no community argument left to fan out over. But a
  committed e2e spec would keep the *transport-level* property from regressing,
  and that is still worth adding.

## Related Issues

- #2492 — the phone-first pivot that introduced ad-hoc per-item communities and
  invalidated the "1-5 communities" premise this design rests on.
- #2040 — removal of the per-community filter, why there is no "selected
  community" to scope realtime to (constraint 1).
- #1895 — removal of the sidebar and persisted community selection; the source
  of the stale comment at `main.dart:567`.

---

## Audit notes

Audited against the documentation context map. Changes made to the plan as a
result:

1. **Reframed the reconnect bug as a logout-correctness defect, and found a
   second one.** `docs/client/logout.md` §Invariants requires pollers and
   streams to stop before caches clear "so in-flight events don't repopulate
   just-cleared caches". The `onError` resurrection path violates that — and so
   does `CommunityEventPoller._pollCommunity`, which routes results after an
   `await` with no stopped-check. The first draft treated the former as a
   cap-hygiene nit and missed the latter entirely. Both now sit in constraint 5
   with the cross-user consequence stated, and both have named tests in Phase 1.
2. **Constrained how the Phase 2 pinned-community provider is torn down.**
   `logout.md` splits new state into two rules with opposite answers: background
   *services* get teardown inside `logout()`, per-user *notifiers* get a
   `clear()` called from `_loadCommunities()` on the next login and must **not**
   touch `logout()` — a second state write in the logout frame caused a
   Duplicate-GlobalKey crash historically. The pinned provider is the second
   kind; the plan now says so, because the first draft left an implementer one
   coin-flip away from reintroducing that crash.
3. **Added the global reconnect-backoff counter as constraint 6.** A single
   `_reconnectAttempts` field is shared across all communities. Admission
   control makes eviction routine, so churn would stretch every community's
   delay toward the 60s cap. Raised to an explicit decision with a
   recommendation rather than left to be inherited by accident.
4. **Made the e2e task actionable and the doc task honest.** Named
   `e2e/lib/seed/communities.ts:27` as the seeding lever and specified *named*
   communities (the highest-priority admission class, so the cap is what's under
   test). Added the stale-comment cleanup at `main.dart:567`, which contradicts
   `startup.md` §Community Selection Persistence (removed).
5. **Verified the load-bearing factual claims** rather than leaving them as
   plausible assertions: Go's `http2defaultMaxStreams = 250`
   (`net/http/h2_bundle.go:4042`, Go 1.26.5) and not overridden in `main.go`;
   FCM is skipped wholesale on web (`main.dart:719`), which is what forces
   Phase 2 to exist; the transport split at `auth_providers.dart:26`
   (`connectrpc/http2` native, `connectrpc/web` on web). Corrected the poll
   stagger figure from 144s to 141s (index 47 × 3s).

**Dimensions engaged:** performance (primary — request batching, connection
reuse, stream disposal, goroutine cost), error_handling (reconnect and teardown
paths), testing, observability (the stream-count signal in Phase 3),
code_quality (named constants over literals), tech_debt (the plan's own
constants are a stopgap, with Option C filed as the successor).
**Not engaged:** security (no auth, PII, or input-validation surface — the
change moves existing authenticated calls around), accessibility (Phase 2 adds
lifecycle wiring to detail screens, no rendered surface), api_design (deferred
to Phase 4; nothing in Phases 1–3 touches proto), dependency (no new packages).

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`; leaf docs `realtime_updates.md`,
`ad_hoc_communities.md`, `client/startup.md`, `client/logout.md`,
`community_health.md` (skim); review prompts `performance`, `error_handling`,
`testing`, `observability`, `code_quality`, `tech_debt`. Code read for
verification: `app/lib/main.dart`, `app/lib/services/community_event_stream.dart`,
`app/lib/services/community_event_poller.dart`,
`app/lib/services/providers/community_providers.dart`,
`app/lib/services/providers/auth_providers.dart`, `app/lib/services/auth_state.dart`,
`server/main.go`, `server/wiring.go`, `proto/ripls/api/community_service.proto`,
`e2e/lib/seed/communities.ts`.
