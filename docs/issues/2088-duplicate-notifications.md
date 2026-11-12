# Issue #2088: Duplicate event notifications; don't lead to the right spot

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2088
**Filed:** 2026-05-21 | **Last activity:** 2026-05-30
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

> **Audit note (post-`ripls-audit`):** Reviewed against
> `docs/server/architecture.md` and `docs/server/observability.md`. The audit
> changed the **implementation surface of the chosen Option B**: the event bus
> already propagates the originating RPC's HTTP `request_id` to subscribers
> (`server/pubsub/memtopic.go:216–230` `deriveDispatchCtx`, "so fan-out log
> lines stay joinable to the originating RPC"). That existing, propagated
> `request_id` is a per-fan-out correlation id that reaches the notification
> subscriber — so the exact-dedup goal of Option B is achievable **without a new
> proto field, protosql column, or edits to the ~8 publish sites**. The plan
> below now keys dedup on the propagated `request_id`; the explicit
> `notification_group_id` field is retained only as a documented fallback. See
> the revised Recommendation, Open Questions, and Implementation Plan. Other
> audit additions: structured-logging compliance for the new suppression
> outcome, goroutine-safety note for the shared dedup set, and Go test-layer
> conventions.

## Problem Statement

A user (user7@example.org, Android, app 0.22.0) reported receiving **two
push notifications simultaneously for the same event**, and that tapping
through dropped them at "an unrelated discussion (maybe the last place I'd
been in the app)."

There are really **two distinct bugs** bundled here:

1. **Duplicate notifications (the priority).** When an item (gear/giveaway,
   request, experience/event) is shared into **multiple communities** and a
   user belongs to **more than one** of those communities, the server sends
   that user **one push per community** instead of a single push. The
   maintainer's diagnosis in the comments is correct and is **confirmed in
   production logs** (see below).

2. **Notification tap lands on the wrong screen.** Per the reporter and a
   maintainer follow-up, this **may only affect experiences**. There is a
   plausible code path for it on the client, but **no smoking-gun reproduction
   in the notification payload or deep-link content**. Per maintainer
   direction, this is **deferred** as possibly non-reproducible and is not in
   scope for the primary fix.

This plan therefore targets bug #1 (duplicate notifications) and records bug
#2 as a deferred investigation.

### Production confirmation of the duplicate

Querying prod Cloud Run logs (`server`, `jsonPayload.operation =
"community_notifications.handle"`, `outcome = "sent"`, last 14 days) shows the
exact pattern. User `d2aae17b-…` is a member of two communities
(`f7c08749-…` and `a92a11d3-…`). When gear is shared to both, they receive two
`GEAR_SHARED` pushes ~1 second apart, each carrying a different
`community_event_id` and `community_id`:

```
18:34:48.603  community a92a11d3  event e0fbec08   GEAR_SHARED
18:34:49.506  community f7c08749  event 55027d49   GEAR_SHARED   ← same user, +0.9s
18:35:19.414  community a92a11d3  event aec61288   GEAR_SHARED
18:35:20.424  community f7c08749  event 67d7e629   GEAR_SHARED   ← same user, +1.0s
18:35:48.601  community a92a11d3  event 79bf7fdc   GEAR_SHARED
18:35:49.453  community f7c08749  event a71ac7f1   GEAR_SHARED   ← same user, +0.85s
```

The duplicates are real, reproducible, and live in prod today.

## Context from Comments

- Two `@claude` pings from the maintainer (2026-05-21, 2026-05-22) — no textual
  content.
- **Maintainer diagnosis (2026-05-30, authoritative):** *"I think this is due
  to a user in multiple communities in which an item (request, event, transfer,
  giveaway) is shared with, they may receive a notification per-community. They
  should just get one."* — This matches the code and the prod logs exactly.
- **Mid-planning clarifications from the maintainer:**
  - The tap-routing bug (bug #2) "may only apply to experiences."
  - "The important thing is to fix the duplicate notifications. If there's no
    smoking gun in the notification content / deep linking, we can defer that
    as perhaps non-reproducible."
  - **Chosen approach: Option B (per-fan-out notification group id) — "it's
    the right long-term fix."**

The branch `duplicate_notifications_really` is already checked out, signaling
prior intent to fix this.

## Root-Cause Analysis (code)

### How items map to multiple communities

- **Request** → `CommunityRequest` junction rows (`request_id`, `community_id`).
  One request → N rows.
- **Experience** → `CommunityExperience` junction rows (`experience_id`,
  `community_id`). One experience → N rows.
- **Gear** → community-gear junction (shared into N communities).
- **Transfer** → single `community_id` column, **no junction**. Transfers are
  scoped to one community and therefore **do not exhibit this bug** (despite the
  issue mentioning "transfer/giveaway" — the giveaway *notification* is
  `GEAR_SHARED`, which does fan out).

### The event bus is publish-per-community, dispatch-async

`server/community_event_bus/inprocess.go` writes the `CommunityEvent` row
synchronously, then **dispatches to subscribers asynchronously** on goroutines
(line 21, 66). Three subscribers consume each event
(`server/main.go:1012–1072`):

1. **Notification subscriber** (`server/notifications/community_subscriber/`)
2. **Stream broadcaster** (community live stream)
3. **Story creation** (per-community feed/timeline)

Subscribers (2) and (3) **legitimately need one event per community** — each
community's feed and live stream should reflect the item. So the per-community
fan-out **cannot simply be collapsed** at the publish site; the fix has to make
the **notification** subscriber dedup across the fan-out without disturbing
stories or streams.

### Where the duplication actually happens

Every item-mutation handler loops over the junction rows and publishes **one
`CommunityEvent` per community**:

| Item | Event | Publish loop |
|---|---|---|
| Gear | `GEAR_SHARED` | `server/services/gear/service.go` (share path) |
| Request | `REQUEST_CREATED` | `request/sharing.go:66–127` (`for _, communityID := range req.Msg.CommunityIds`) |
| Request | `REQUEST_FULFILLED` | `request/lifecycle.go:343–394` (`for _, communityRequest := range communityRequests`) |
| Request | `REQUEST_CANCELLED` | `request/lifecycle.go` (same pattern) |
| Experience | `EXPERIENCE_CREATED` | `experience/share.go:171–187` (one per `ShareExperience` call) |
| Experience | `EXPERIENCE_STARTED` | `experience/lifecycle.go:81–104` |
| Experience | `EXPERIENCE_COMPLETED` | `experience/lifecycle.go:329–355` |
| Experience | `EXPERIENCE_UPDATED` | `experience/save.go:446–475` |
| Experience | `EXPERIENCE_CANCELLED` | `experience/lifecycle.go` |

The notification subscriber (`subscriber.go:84–219`) handles **one event at a
time, statelessly**. It resolves recipients via
`getNotificationRecipients` (`recipients.go:79`) and calls `NotifyUser` for
each — with **no awareness that other per-community events describe the same
item action**. There is per-event dedup (skip actor, skip active stream) but
**zero cross-event dedup**.

This produces **two different duplicate mechanisms**:

- **Broadcast events** (`GEAR_SHARED`, `REQUEST_CREATED`, `EXPERIENCE_CREATED`,
  `EXPERIENCE_COMPLETED`): recipients = members of *that event's* community
  (`recipients.go:242–255`). A user in N shared communities appears in N
  recipient lists → **N pushes**. (This is the prod-confirmed case.)
- **Targeted events fired per-community-record** (`REQUEST_FULFILLED`,
  `REQUEST_CANCELLED`, `EXPERIENCE_STARTED/UPDATED/CANCELLED`): recipients are
  **community-independent** (request offerers, experience RSVPs — resolved by
  `request_id`/`experience_id`, not `community_id`). Firing the event once per
  community notifies the **same full recipient set N times** → **every recipient
  gets N pushes**, regardless of which communities they belong to. This is the
  more severe variant and almost certainly what the reporter (an RSVP'd
  experience attendee) saw.

The fix must therefore live at the **notification dispatch boundary**: notify
each user **at most once per logical item-action**, while leaving the
per-community story/stream events untouched.

## Potential Solutions

### Option A: Short-TTL in-memory dedup keyed `(event_type, item_id, user_id)`
- **Approach:** Thread-safe TTL "seen" set on the subscriber; suppress repeat
  keys within ~60s.
- **Pros:** Smallest change; no proto/bus change; covers both mechanisms.
- **Cons:** Heuristic — two *genuinely distinct* same-type actions on the same
  item to the same user within the window collapse to one push. Approximates
  the grouping rather than encoding it.
- **Effort:** Small.

### Option B (CHOSEN): Exact, per-action dedup keyed on the propagated `request_id`
- **Approach:** Use the per-action correlation id the platform **already**
  threads through fan-out — the HTTP `request_id`. The community event bus's
  dispatch path (`server/pubsub/memtopic.go:216–230`, `deriveDispatchCtx`)
  copies `request_id` (and `user_id`) from the publishing RPC's context onto the
  subscriber's dispatch context, expressly "so fan-out log lines stay joinable to
  the originating RPC." Every per-community `CommunityEvent` emitted by one
  unary mutation RPC therefore reaches the notification subscriber with the
  **same `request_id`**. The subscriber dedups recipients by
  `(request_id, event_type, item_id, user_id)` using a thread-safe, TTL-bounded
  set. Because a `request_id` is 1:1 with a single user action
  (`docs/server/observability.md` request-ID glossary), dedup is **exact and
  timing-independent**; the TTL only caps memory.
- **Pros:** Encodes the true grouping ("one item action") via the codebase's
  existing correlation mechanism — the idiomatic Ripls pattern (same shape as
  `services/chat/streaming.go#broadcastMessage`). **No proto change, no protosql
  column, no edits to the ~8 publish sites.** Distinct actions get distinct
  `request_id`s and never collapse; per-community fan-out of one action shares
  the id and dedups cleanly. `event_type`+`item_id` in the key keep genuinely
  different notifications emitted by the *same* RPC separate.
- **Cons:** Depends on two invariants (both documented/verified): the client
  sends an `X-Request-ID` per call (it does — observability glossary), and
  fan-out publishes happen from **unary** RPCs, not from inside a long-lived
  stream handler (where `request_id` scopes the whole stream and would be too
  coarse). When `request_id` is empty (e.g. a background-job publisher), the
  subscriber simply **does not dedup** and behaves exactly as today.
- **Effort:** Small.

### Option B′ (fallback): explicit `notification_group_id` proto field
- **Approach:** Only if Option B's invariants don't hold on some fan-out path:
  add `optional string notification_group_id` to `CommunityEvent`, generate one
  id per fan-out before the publish loop, stamp every event in the loop, and
  dedup by `(group_id, user_id)`.
- **Pros:** Independent of `request_id` semantics; self-contained per event.
- **Cons:** New proto field + protosql column + threading through ~8 publish
  sites. Strictly more work than B for the same exactness.
- **Effort:** Medium. **Carry only if a verification in Phase 1 shows B is
  unsafe.**

### Option C: Decouple notification from per-community story/stream events
- **Approach:** Drive notifications off a single item-action event with a
  deduped union of recipients; keep per-community events only for story/stream.
- **Pros:** Cleanest separation of concerns.
- **Cons:** Largest change; alters the bus subscriber contract and every publish
  site; high regression risk across stories, streams, and undo classification.
- **Effort:** Large.

## Recommendation

**Go with Option B (maintainer-chosen), realized through the propagated
`request_id`.** It is the correct long-term fix: the dedup key encodes the
actual invariant — "one item-action notifies a user at most once" — instead of
approximating it with a time window (Option A) or restructuring the bus contract
(Option C). The audit refinement is that this exactness needs **no new schema**:
the `request_id` that already flows to subscribers *is* the per-action group id.
Keep Option B′ (the explicit proto field) in reserve only if Phase 1 verification
finds a fan-out path where `request_id` is missing or too coarse.

Key design points:
- **Dedup key = `(request_id, event_type, item_id, user_id)`**, where `item_id`
  is the experience→request→gear id on the event (same priority the client uses
  for routing). `event_type`+`item_id` keep genuinely different notifications
  from the same RPC separate; `request_id` is what collapses the per-community
  fan-out of one action.
- **No-dedup fallback when `request_id` is empty.** Background-job publishers and
  any non-RPC path send exactly as today — never worse than current behavior.
- **Record the dedup key only on the actual-send path** (after the
  actor/stream/category gates), so a stream-suppressed first event can't mask a
  legitimate later push.
- **In-memory dedup is sufficient.** The whole fan-out for one mutation is
  published by one RPC on one instance over an in-process MemTopic, so all
  duplicates originate on the same instance; a DB-backed ledger would be
  over-engineering.
- **Goroutine safety (audit).** The dedup set is read/written from the pubsub
  worker goroutines (`memtopic.go` dispatch), so it **must** be mutex-guarded
  with atomic check-and-set. No new goroutine is introduced (the subscriber runs
  inside the existing `GoSafe`-wrapped worker), so no new `check-goroutines`
  surface — but if any helper does spawn one, it must use `logging.GoSafe`.

## Open Questions

- **`request_id` (Option B) vs. explicit proto field (Option B′)?** —
  Recommendation: **B**, contingent on the Phase 1 verification below. B is
  exact, smaller, and uses the existing correlation mechanism; only fall back to
  B′ if a real fan-out path violates B's invariants. (This is the one design
  fork worth explicit sign-off — flag for reviewer.)
- **Are all duplicate-prone fan-out publishes emitted from unary RPCs (not from
  inside a long-lived stream)?** — *Must verify in Phase 1.* If a fan-out
  publish ever runs inside a `StreamCommunityEvents`-style handler, `request_id`
  scopes the whole stream and would over-suppress; that path would need B′.
  Recommendation: grep the ~8 publish sites; they are `ShareGear`,
  `ShareRequest`, `MarkRequestFulfilled`, etc. — all unary — so B is expected to
  hold.
- **Which `item_id` keys the dedup per event type?** — Recommendation: prefer
  `experience_id`, else `request_id`, else `gear_id` (mirrors the client's
  routing priority). If an event carries none, **skip dedup** (those event types
  are single-community and can't duplicate this way).
- **TTL / memory bound for the dedup set?** — Recommendation: a few minutes is
  ample (the burst is ~1–2s); the TTL only bounds memory, not correctness, since
  the key is unique per action. Prune opportunistically.
- **Logging of the new suppression (audit / observability).** — Recommendation:
  emit the existing per-recipient INFO line with a new
  `outcome = "suppressed_duplicate"` value, reusing the established structured
  shape (`user_id`, `community_event_id`, `event_type`) so Cloud Logging can
  group by `jsonPayload.outcome` (matches the README filter). Do **not** add a
  raw `request_id` field — `LoggerWithContext` already injects it on every line;
  never overload it for the entity id (`target_request_id` rule). Suppression is
  normal operation → **INFO**, consistent with the other `suppressed_*` lines,
  not WARN/ERROR.
- **Undo / `CommunityEventType` classification.** — Recommendation: Option B
  adds **no** enum value and no proto field, so `TestEveryEventTypeClassified`
  is unaffected. (Only relevant if B′ is chosen, and even then a field — not an
  enum value — leaves the gate untouched; verify during implementation.)

## Implementation Plan

> **Status (implemented):** Phases 1–2 are **done** and landed on branch
> `duplicate_notifications_really`. The invariant verification passed (all ~8
> fan-out publishes are unary RPC handlers — `GEAR_SHARED` ships from
> `community/gear_sharing.go` `ShareGear`/`UnshareGear`, etc.), so Option B was
> implemented as-is and Phase 3 (B′) is **not needed**. Phase 4 remains
> deferred. Code: `dedup.go` (TTL set + key derivation), `subscriber.go` (gate),
> `dedup_test.go` + `dedup_subscriber_test.go` (unit + regression incl. both
> duplicate mechanisms). `npm run build` and `go test ./server/notifications/...`
> green.

### Phase 1 (DONE): Lock the bug down with a failing test + verify the `request_id` invariant
**Goal:** A regression test that reproduces the duplicate, and confirmation that
Option B's `request_id` grouping is safe.
**Tasks:**
- [ ] Add a unit test in `server/notifications/community_subscriber/` (test file
  alongside `subscriber.go`, table-driven per the Go test conventions) that
  dispatches N per-community `CommunityEvent`s for the **same**
  gear/request/experience **through one originating ctx (one `request_id`)** to a
  `notifications.Mock`, with a recipient who is a member of all N communities (or,
  for targeted events, a single offerer/RSVP), and asserts `NotifyUser` is called
  **once** for that user. Drive it via the `community_event_bus` mock / a direct
  `Subscriber.Handle` loop so the propagated `request_id` is realistic.
- [ ] Cover both mechanisms: one broadcast case (`GEAR_SHARED`) and one targeted
  case (`REQUEST_FULFILLED` or `EXPERIENCE_UPDATED`).
- [ ] Confirm the test **fails** today (currently sends N).
- [ ] **Verify the invariant:** grep the ~8 fan-out publish sites and confirm all
  are unary RPC handlers (not inside a `Stream*` handler). If any fan-out emits
  from within a long-lived stream, switch that path (or the whole fix) to
  Option B′.
**Validates:** The duplicate is reproducible in a unit test, the fix has a
red→green target, and the `request_id`-as-group-id assumption holds. (Aligns
with the "use tests to find/fix bugs at the root" preference.)

### Phase 2 (DONE): Dedup in the notification subscriber (Option B)
**Goal:** Notify each user at most once per `request_id`-scoped item-action.
**Tasks:**
- [ ] Add a thread-safe, TTL-bounded "seen" set type (mutex-guarded
  `map[string]int64` expiry, opportunistic pruning) — minimal, no new dependency,
  with its own unit test (success + expiry + concurrent-access paths).
- [ ] Add it to `Subscriber`. In `subscriber.go`: derive `rid :=
  logging.RequestIDFromContext(ctx)` and the `item_id` (experience→request→gear
  priority); if `rid == ""` or no `item_id`, **skip dedup**. Otherwise compute key
  `rid + ":" + event_type + ":" + item_id + ":" + user_id` and, after the
  actor/stream/category gates and **only on the successful-send path**,
  check-and-set; skip + log when already seen.
- [ ] Add `outcomeSuppressedDuplicate = "suppressed_duplicate"` to the outcome
  consts and emit the standard per-recipient INFO line with it.
- [ ] Turn the Phase 1 test green. Add tests for: a *different* user still gets
  their single push; empty `request_id` bypasses dedup; events with no item id
  (e.g. `INVITATION_LINK_USED`) are unaffected; a stream-suppressed first event
  does not block a later legitimate push.
- [ ] `npm run build` and `go test ./server/notifications/...`.
**Validates:** Prod duplicate pattern eliminated; exact, schema-free dedup with
no collateral suppression of legitimate single notifications.

### Phase 3 (only if Phase 1 verification fails): explicit group id (Option B′)
**Goal:** Carry an explicit per-fan-out id when `request_id` is unsafe on some
path.
**Tasks:**
- [ ] Add `optional string notification_group_id` to `CommunityEvent` in
  `proto/ripls/models/` (placed near semantically related fields; client-agnostic
  proto comment, no server mechanics leaked). `npm run generate`.
- [ ] Generate one id per fan-out before each affected publish loop and stamp
  every event in the loop. `npm run build`.
- [ ] Switch the subscriber key to prefer `notification_group_id` when present,
  else fall back to the `request_id` key from Phase 2.
**Validates:** Exactness preserved on paths where `request_id` doesn't suffice.

### Phase 4 (deferred): Experience notification tap routing (bug #2)
**Goal:** Investigate "tap lands on wrong screen," scoped to experiences.
**Status:** **Deferred** per maintainer — possibly non-reproducible; no
smoking gun in payload/deep-link content. Recorded so it isn't lost.
**If revisited, start from:**
- `app/lib/services/fcm_service.dart` `routeNotificationPayload()` (≈542–600):
  the silent no-op branch when no entity id is present, and the
  experience→`/experience/:id` route.
- `app/lib/main.dart` (≈654–663): `onNavigate` receives `communityId` but never
  uses it; post-#1895 there is no "select community" mechanism
  (`community_providers.dart:117–118`), so an experience opened from a
  notification while browsing a different community may resolve against the
  wrong context.
- Confirm experience push payloads actually carry `experience_id`; if any
  experience event omits it, `routeNotificationPayload` silently leaves the user
  on the current screen — the most likely cause of "unrelated discussion."

## Related Issues
- None found directly. Note: #1895 removed the "selected community" concept,
  which is relevant background for the deferred bug #2 (Phase 4).
