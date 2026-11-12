# Issue #625: Scheduled notifications (e.g. timed before an event)

> **For the current design, see
> [`docs/server/scheduled_notifications.md`](../server/scheduled_notifications.md).**
> This document is implementation history — the options considered,
> the decisions made, the schema revisions along the way. It is
> useful for understanding *why* the system is shaped the way it is.
> If you need to know *what* it does, read the canonical design doc
> instead.

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/625
**Filed:** 2026-01-22 | **Last activity:** 2026-05-14
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none

> **Audit (2026-05-14):** Plan reviewed against `docs/server/architecture.md`,
> `docs/server/observability.md`, `docs/client/architecture.md`,
> `docs/client/caching.md`, `docs/client/i18n.md`,
> `docs/client/accessibility.md`, `docs/client/testing.md`. Changes:
> background goroutines must use `logging.GoSafe` (not raw `go func()`);
> all new RPCs return internal errors via `connecterr.Internal`;
> logger conventions and standard field names made explicit in a new
> *Observability conventions* section; the new `UserNotificationPreferences`
> RPCs land in `server/services/user/`; the client-side preferences screen
> follows the MVVM Service → Repository → ViewModel → Screen layering, uses
> `Toggle`/`Tappable` accessibility primitives, and adds ARB strings under
> the existing key conventions.

> **Schema revision (2026-05-15):** After Phase 1 landed, six
> refinements applied based on user review:
>
> 1. **Drop the `kind` enum; collapse `anchor` and `config` into a
>    single `oneof item`.** The oneof variant fully identifies both
>    the entity type and its per-item-type config — having a separate
>    `kind` enum was redundant. Variants are
>    `ExperienceNotification` and `LoanNotification`, each carrying
>    the anchored ID plus its own purpose+offset.
> 2. **Slot/class enum split into `purpose` + `offset_seconds_from_anchor`.**
>    The earlier `EventNotificationClass` conflated two dimensions
>    (timing relative to the anchor, and purpose of the notification).
>    Now each per-item-type config carries a `purpose` enum (with
>    semantically distinct values: e.g.
>    `EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START`,
>    `EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT`,
>    `EXPERIENCE_NOTIFICATION_PURPOSE_RECURRENCE_REMINDER`) and a
>    continuous signed `offset_seconds_from_anchor` (negative =
>    before, positive = after). Adding a new offset (e.g. 5-minute
>    reminder) is a policy-constant change, not a schema change.
> 3. **`EVENT` → `EXPERIENCE` in all code identifiers.** Per the
>    CLAUDE.md rule: domain code uses `Experience` to avoid colliding
>    with the programming concept of events. User-visible strings
>    still say "Event".
> 4. **`target_user_id` → `recipient_user_id`.** Clearer intent.
> 5. **`optional` for genuinely-optional fields.** `community_id` and
>    `quiet_hours_timezone` are now `optional` so callers see
>    presence-tracked accessors.
> 6. **No SENT status; hard-delete on dispatch.** The dispatcher
>    claims by atomic `DELETE ... WHERE id = $1` and the record of
>    send lives in structured logs. Drops the status enum + fields
>    and the need for a retention job. A rescheduled experience
>    naturally re-fires its reminders, because the reconciler sees no
>    existing row and inserts fresh for the new fire_at.
>
> Also: `expected_return_unix_sec` on `Transfer` is models-only (no
> API mirror).

## Problem Statement

Ripls today sends notifications only as a *reaction* to events that just
happened: someone RSVPed, someone offered to fulfill a request, someone
withdrew interest. There is no mechanism for sending notifications on a
*schedule* relative to a future or in-flight activity.

Two concrete needs surface in the issue (clarified by the author on
2026-05-14):

1. **Pre-event reminders.** Push nudges sent at planned lead times before an
   event (e.g. day-before, hour-before, "starting now"), enriched with
   context such as what each recipient has promised to bring
   (`PlanningContribution`).
2. **Return reminders.** Push nudges sent when a borrowed item is approaching
   — or past — its return date.

The author also named two foreseeable extensions that the design must not
paint into a corner:

3. **Post-event close prompts.** Nudge the host to close the event after it
   has occurred.
4. **Recurrence nudges.** Suggest scheduling another instance of an activity
   the user has already run.

The fundamental capability gap is producing notifications **proactively**,
keyed off durable properties of in-flight entities (event start time, loan
pickup time + duration, "time since last X"), rather than reactively in
response to an RPC.

Calendar export (the original issue body's framing) is **out of scope**;
the author's 2026-05-14 comment supersedes it.

## Context from Comments

- **tescobar (2026-03-04, triage):** Marked P2. Enables event reminders for
  Betty (Time×Giving) and Emma (Time×Exchange) — both community-oriented,
  time-rich personas for whom event participation is a core loop. Called
  out as a prerequisite for calendar-like notification features.
- **maintainer (2026-04-24, triage):** Bumped to **P1**.
- **maintainer (2026-05-14):** Rewrote scope. Not a Google-Calendar
  integration. The fundamental ask is *scheduled* (not event-driven) push
  notifications, with two named use cases (pre-event nudges including
  promised contributions, and "return what you borrowed" nudges). Email
  is a follow-up — push only for now.

The 2026-05-14 comment is the authoritative scope for this plan; the
original body is superseded.

## Codebase landscape

What already exists:

- **Push delivery.** `server/notifications/service.go` defines
  `Service.NotifyUser(ctx, userID, *models.Notification)` with FCM and
  noop providers. Strongly-typed payloads via a proto `oneof`. Invalid
  tokens are auto-unregistered. This is the right pipe for all
  scheduled-notification dispatches — no new delivery layer needed.
- **Background-job pattern.** `server/jobs/community_purge_reminder.go` is
  the closest existing precedent: a ticker-driven job (wired in
  `server/main.go:1029`) that queries candidates, atomically claims via a
  conditional-UPDATE primitive (`storage.ClaimCommunityPurgeReminder`),
  builds copy, and dispatches one notification per recipient. The
  *delivery* shape is right; the *scheduling model* (one-off flat column
  per slot) is what we're replacing.
- **Per-community preferences.** `CommunityNotificationPreferences` with
  toggles like `notify_transfer_updates`, `notify_experience_rsvps`,
  `notify_planning_updates`. Defaults are "on" when unset. The
  purge-reminder push deliberately bypasses this — precedent for
  excluding some scheduled notifications from per-community gating when
  they don't fit any existing category.
- **Scheduling-relevant data:**
  - `Experience.time` is `ExperienceTime { specific|range|tbd }`. The
    `SpecificTime.unix_timestamp_sec` and `TimeRange.start_unix_sec` are
    absolute Unix times — no timezone normalization needed for
    comparison against `time.Now()`. `is_all_day` and `timezone` matter
    only for copy formatting and quiet-hours adjustment.
  - `ExperienceRSVP { intention: YES|MAYBE|NO }` defines recipients.
  - `PlanningContribution` (claimed `PlanningNeed`s) carries
    "what people promised to bring" — the per-recipient context the
    issue explicitly calls for.
  - `Transfer` stores `actual_pickup_unix_sec` and
    `loan_duration_days` but **not an absolute `expected_return_unix_sec`**
    — that gap must be closed before return-due reminders can be queried
    efficiently.
- **CommunityEvent pattern** (the analogue the user wants to mirror):
  past-tense ledger entries with `id`, `community_id`, `actor_id`, a
  `oneof topic { invitation_short_code | transfer_id | request_id |
  experience_id }`, and `occurred_at_unix_sec`. The new
  `ScheduledNotification` type mirrors this shape but for *planned
  future events*.

What does **not** exist:

- Any storage table or queue for "send this push at time T".
- Any concept of reconciling desired-vs-actual reminder rows from current
  entity state.
- Any per-user (rather than per-community) notification preference
  surface.

## Potential Solutions

### Option A — Per-slot polling jobs (purge-reminder pattern, scaled out)

- **Approach:** One background `jobs.*` package per reminder family.
  Per-slot flat columns on each entity (`experience.day_before_reminder_sent_at_unix_sec`,
  etc.) plus a conditional-UPDATE claim per slot.
- **Pros:** Tiny step from the existing purge-reminder pattern. Cheap
  first delivery.
- **Cons:** Every new reminder kind requires schema work and a new claim
  primitive. The "post-event close" and "recurrence nudge" use cases the
  author already names would each add another column to the same row.
  No unified place to inspect what's scheduled for a given user. No clean
  story for re-firing after a missed window across deploys.
- **Effort:** Medium initially, but each subsequent reminder type costs
  the same again.

### Option B — Generic `ScheduledNotification` table with reconciler + dispatcher (RECOMMENDED)

- **Approach:** Introduce a `ScheduledNotification` proto and table.
  Periodically, a **reconciler** scans relevant entities and ensures the
  table reflects the *desired* set of pending notifications derived from
  current state. A **dispatcher** wakes more often, pulls rows whose
  `fire_at_unix_sec` is due, claims them, renders copy from current
  entity state, applies user preferences and quiet hours, and dispatches
  via `notifications.Service`. New reminder kinds are: (a) a new enum
  value, (b) a new reconciler function that returns desired-state tuples,
  and (c) a new copy builder. No new schema.
- **Pros:** One primitive serves every named use case (pre-event, return
  reminders, close-event, recurrence nudges) and every plausible future
  one. The reconciler model is *self-healing*: if the server is down for
  an hour, the next tick re-syncs the table correctly; if a reminder
  type rolls out, the next tick enqueues rows for already-existing
  entities automatically. **Policy lives in the reconciler, not in the
  rows.** Changing how many reminders to send, when to send them, or
  adding a new reminder kind is a code change plus one reconciler tick —
  the next pass produces the new desired-state set for all in-flight
  entities and upserts accordingly. No backfill script. B1 cannot match
  this: a row written at entity-creation time freezes the policy that was
  in force then, and a later policy change leaves old entities on the
  old schedule unless a one-off migration walks them. Inspectable: a
  single `SELECT * FROM scheduled_notification WHERE target_user_id = ?`
  shows every future notification this user will receive.
- **Cons:** Real upfront work: new proto, new table with indexes, two
  job loops instead of one, copy-rendering layer that hydrates from
  current entity state. Reconciler logic has to correctly cancel rows
  when an entity is deleted or its trigger time shifts.
- **Effort:** Large.

### Option B1 — Option B but enqueue at write-time instead of reconciling

- **Approach:** Same proto/table/dispatcher, but rows are written by
  service-layer hooks on entity create/update/delete instead of by a
  reconciler.
- **Pros:** Lower latency — a brand new event gets its reminders into
  the table within the same RPC, not on the next reconciler tick.
- **Cons:** Every reminder type must be enqueued at *every* write path
  that could affect it (create, edit, cancel, time-change, RSVP join,
  RSVP leave). Easy to miss one. Recovery from a missed enqueue (server
  crashed mid-RPC, a bug deployed, a backfill needed) requires extra
  one-off backfill scripts each time. **Policy changes don't propagate
  to in-flight entities** — adding a new lead time or extra slot
  requires walking all existing rows and rewriting them. Net: brittler
  than reconciliation, and the gap widens every time we tune policy.
- **Effort:** Comparable code volume to B, but spread across many files
  and harder to audit.

## Recommendation

**Option B.** The user has already named four reminder kinds; option A's
per-slot column model would force schema changes for each one and would
leave us with no inspection surface. The reconciler discipline pays back
the upfront effort within the second reminder type, and pays back again
every time we ship a new kind.

Within Option B, prefer **reconciliation** over enqueue-at-write
(Option B1) for the first delivery. The user's intuition is right:
"scan and idempotently upsert" is more durable, easier to reason about,
and far easier to extend. We can layer in enqueue-at-write later as a
*latency optimization* — write a row eagerly, let the reconciler correct
or cancel it on the next pass — without rewriting anything. Starting
with pure reconciliation keeps the surface area small and the failure
mode legible: at any moment, the table reflects what the reconciler
last computed, and the dispatcher delivers what's due.

The one piece of schema work that *cannot* be replaced by the new
primitive: `Transfer` still needs an absolute `expected_return_unix_sec`,
because the reconciler must query loans by their due date. Today only
`loan_duration_days` exists, and `actual_pickup + duration` isn't
indexable.

## Open Questions

- **Reconcile interval and dispatch interval?** — Recommendation:
  reconciler every **10 minutes**, dispatcher every **1 minute**. A 10-min
  reconciliation latency is fine: the earliest meaningful slot (event
  starting "now" at ±15 min) tolerates it. A 1-min dispatcher loop keeps
  fire-time precision under the noise of FCM delivery itself. Both run
  as `time.NewTicker` goroutines off `main.go` — no cron daemons.

- **Reconciler scan scope?** — **Resolved (full scan):** every tick,
  each reconciler walks all in-flight entities for its kind — no
  artificial horizon. "In-flight" is the reconciler's own predicate
  (upcoming non-deleted experiences for event reminders, ACTIVE
  transfers with `expected_return_unix_sec` for loan reminders). At
  Ripls scale this is a small set (low thousands), and the predicate is
  index-backed.

- **Row uniqueness and re-RSVP semantics?** — **Resolved (hard-delete
  everywhere):** the reconciler maintains at most one row per
  `(recipient_user_id, item.<variant_id>, purpose, offset_seconds_from_anchor)`.
  When a tuple leaves the desired set (Experience cancelled, RSVP
  withdrawn, loan returned, prefs gated, etc.) the row is
  hard-deleted; when the dispatcher fires the row, it hard-deletes
  too. A subsequent re-entry into the desired set (re-RSVP,
  Experience rescheduled to later) produces a fresh row. There are
  no historical rows in the table — record-of-send is in structured
  logs.

- **Policy/lead-time changes?** — **Resolved (update fire_at on
  existing row):** when a reconciler's lead-time constant changes,
  the next tick produces tuples with the new `fire_at` and the driver
  updates matching rows in place via
  `UpdateScheduledNotificationFireAt` (re-encodes binary_proto in
  lockstep). New reminder kinds enqueue rows for every in-flight
  entity on the first tick after rollout. No backfill script
  required.

- **Dispatcher claim semantics?** — **Resolved (atomic delete-as-claim):**
  the dispatcher selects due rows in a batch, then for each row calls
  `ClaimAndDeleteScheduledNotification`. The function runs
  `DELETE WHERE id = $1` and reports whether the caller's DELETE
  affected a row — exactly one concurrent runner wins. After the
  claim succeeds, the dispatcher renders copy and calls
  `notifications.Service.NotifyUser`, then emits an `INFO` structured
  log line recording the send. Provider failure post-claim is logged
  as ERROR and the push is lost — we accept this on the grounds that
  (a) FCM is reliable, (b) crash-mid-send is rare, and (c) invented
  retry machinery is a worse risk than the rare miss.
- **Initial pre-event reminder slots?** — Recommendation: **24h
  before**, **2h before**, **starting now** (within ±15 min). Skip
  "1 week before"; long-lead reminders annoy more than they help for
  hyperlocal events.
- **Initial loan return reminder slots?** — Recommendation: **1 day
  before expected return**, **on the return date** (morning-of),
  **3 days overdue**. Skip "7 days overdue" until we see real misses.
- **Initial post-event close-prompt slot?** — Recommendation: **1 day
  after** the event end time, to the host only. Idempotent: if the host
  marks the event completed before then, the reconciler cancels the
  pending row.
- **Initial recurrence-nudge logic?** — Recommendation: **defer
  beyond phase 5.** It needs heuristics (which past events to nudge
  recurrence for? cadence?) that deserve their own design. The proto and
  reconciler architecture is built to accommodate it later by adding one
  enum value plus one reconciler-function — no schema change required.
- **Per-community gate vs. per-user gate?** — Recommendation: pre-event
  reminders gated by a new `notify_event_reminders` toggle on
  `CommunityNotificationPreferences`. Loan return reminders and close
  prompts gated **per-user** via a new
  `UserNotificationPreferences` proto — they're about "your stuff" and
  "your event," not about a community feed. Recurrence nudges TBD when
  they ship.
- **Eager-render vs. lazy-render the copy?** — Recommendation: **lazy
  render at dispatch time.** Store `kind + entity_id + recipient` on the
  row; the per-kind copy builder reads current entity state when the
  dispatcher picks the row up. Avoids stale copy after edits ("Pasta
  night" was renamed to "Lasagna potluck" yesterday and we send the old
  name); avoids storing PII redundantly; reconciler stays cheap because
  it doesn't touch copy at all.
- **Quiet hours implementation?** — Recommendation: quiet hours
  **22:00–07:00** in the relevant timezone (event timezone for
  pre-event reminders; borrower's most-recent device timezone for loan
  reminders; host's last-seen timezone for close prompts). The
  *dispatcher* — not the reconciler — applies quiet hours at dispatch
  time: if the current local time is inside the quiet window, advance
  the row's `fire_at_unix_sec` to 07:00 local and leave it pending.
  This keeps reconciliation pure (no clock-time math) and concentrates
  the timezone logic in one place.
- **Backfill for entities that existed before this ships?** —
  Recommendation: **no migration needed.** The reconciler scans current
  entity state on every tick, so on first deploy it will enqueue rows
  for every existing future event and active loan automatically.
  Document this in the PR.
- **What happens if an event's time changes after a row is enqueued?** —
  The reconciler's next tick sees the desired-state tuple has a different
  `fire_at_unix_sec` and updates the existing row in place. If the row
  was already fired (and therefore deleted), the reconciler finds no
  row for the key, computes that the new `fire_at` is in the future,
  and inserts a fresh row — i.e. a rescheduled event correctly
  re-fires its reminders. Edge case: time moved *earlier* than the
  original schedule and the new fire time is already in the past →
  the dispatcher picks it up on its next tick.
- **Where do reconciler-function implementations live?** —
  Recommendation: each reminder kind owns a file under
  `server/jobs/scheduled_notifications/` (`event_reminders.go`,
  `loan_return_reminders.go`, `event_close_prompts.go`, ...). The
  reconciler driver iterates a registry of kind→function so we never
  forget to wire a new kind in.

## Proto Schema

New file `proto/ripls/models/scheduled_notification.proto`:

```proto
syntax = "proto3";

package ripls.models;

option go_package = "type2fun.org/ripls/server/gen/ripls/models;models";

enum ExperienceNotificationPurpose {
  EXPERIENCE_NOTIFICATION_PURPOSE_UNSPECIFIED = 0;
  EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START = 1;
  EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT = 2;
  EXPERIENCE_NOTIFICATION_PURPOSE_RECURRENCE_REMINDER = 3;
}

message ExperienceNotification {
  string experience_id = 1;
  ExperienceNotificationPurpose purpose = 2;
  // Negative = before start, positive = after start. Reconciler
  // computes fire_at = experience.start_unix_sec + offset.
  int64 offset_seconds_from_anchor = 3;
}

enum LoanNotificationPurpose {
  LOAN_NOTIFICATION_PURPOSE_UNSPECIFIED = 0;
  LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER = 1;
}

message LoanNotification {
  string transfer_id = 1;
  LoanNotificationPurpose purpose = 2;
  // Negative = before due, positive = after due. Reconciler computes
  // fire_at = transfer.expected_return_unix_sec + offset.
  int64 offset_seconds_from_anchor = 3;
}

message ScheduledNotification {
  string id = 1;
  string recipient_user_id = 2;
  optional string community_id = 3;

  int64 fire_at_unix_sec = 4;
  optional string quiet_hours_timezone = 5;

  int64 created_at_unix_sec = 6;
  int64 updated_at_unix_sec = 7;

  // Which entity this notification is anchored to and the per-item-type
  // config. Variant fully identifies both — no separate `kind` enum.
  oneof item {
    ExperienceNotification experience = 10;
    LoanNotification loan = 11;
  }
}
```

Uniqueness: the reconciler maintains **at most one** row per
`(recipient_user_id, item.<variant_id>, purpose, offset_seconds_from_anchor)`.
Two reminders to the same user about the same experience with the
same purpose but different offsets (e.g. 24h-before *and* 2h-before)
are distinct rows. The dispatcher **hard-deletes** the row on a
successful send via an atomic `DELETE ... WHERE id = $1` — exactly
one runner's DELETE wins under concurrent dispatch. There is no SENT
state; record-of-send lives in structured logs. A rescheduled
Experience therefore naturally re-fires its reminders, because the
reconciler sees no existing row and inserts fresh for the new
fire_at.

This is a `models/` type only — server-internal bookkeeping, not
part of the wire contract with clients. No API mirror. `Transfer`
adds `expected_return_unix_sec` for the same reason (reconciler
index); that field is also models-only.

## Observability conventions

All reconciler and dispatcher code paths follow the conventions in
`docs/server/observability.md`:

- Acquire a context-aware logger via `logging.LoggerWithContext(ctx)`.
  Never log via `log.Printf`, `fmt.Println`, or `slog.Default()`.
- Set a stable `operation` label on every log line for alerting rollup.
  Use `operation=ScheduledNotificationReconcile` for reconciler-driver
  lines, `operation=ScheduledNotificationDispatch` for dispatcher-driver
  lines, and pass these through to the per-item-type
  reconciler/dispatcher functions so per-row lines inherit them.
- Use standard entity field names: `recipient_user_id`, `community_id`,
  `experience_id` / `transfer_id` (anchor type, taken from the `item`
  oneof variant), `scheduled_notification_id`, `purpose` (proto enum
  name), `offset_seconds_from_anchor`, `fire_at_unix_sec`.
- Log levels:
  - `INFO` — per-tick summaries (counts inserted / updated / deleted /
    dispatched / claim-lost), and per-row dispatch successes.
  - `DEBUG` — per-row reconciler decisions when diffing desired-state
    against current rows.
  - `WARN` — claim contention (lost CAS race; expected with concurrent
    runners), preference-gated skips when surprising.
  - `ERROR` — provider dispatch failure after a successful CAS (push
    lost), storage errors, unrecognized `item` variant at dispatch
    time.
- Background tickers spawn via `logging.GoSafe(ctx, "scheduled-notifications-reconciler", fn)`
  and `logging.GoSafe(ctx, "scheduled-notifications-dispatcher", fn)`.
  Raw `go func()` is rejected by `server/cmd/check-goroutines` in CI
  outside the two allowlisted files.
- All new RPCs added by this work return `connect.CodeInternal` via
  `connecterr.Internal(ctx, op, err, kv...)` — never bare
  `connect.NewError(connect.CodeInternal, err)`, which would leak raw
  storage messages and is gated by `scripts/check_codeinternal.js`.

## Reconciler model in one paragraph

A reconciler function for a given item type takes the current world
(via `storage` reads) and returns a set of
`(recipient_user_id, anchor_id, purpose, offset_seconds_from_anchor,
fire_at_unix_sec, quiet_hours_timezone)` tuples — the *desired state*
for that item type. The reconciler driver diffs the desired set
against the rows currently in storage for that item type:

- **In desired set, no row** → INSERT a new row.
- **In desired set, row exists with matching `fire_at`** →
  leave alone.
- **In desired set, row exists with a different `fire_at`** →
  UPDATE the row in place (this is how policy changes propagate).
- **Row exists, not in desired set** → DELETE the row.

The driver runs each registered reconciler in sequence on every tick,
time-boxed. Each reconciler is responsible for its own "in-flight"
predicate (e.g. event reminders consider non-deleted experiences with
start time in the future; loan reminders consider ACTIVE transfers
with `expected_return_unix_sec` set). There is no shared time
horizon — the scope is dictated by entity state.

## Implementation Plan

### Phase 1 — Schema foundations  *(COMPLETE — commits 672a4f51c, refined 2026-05-15)*

**Goal:** Land the proto, table, indexes, and the
`expected_return_unix_sec` field on Transfer. Nothing fires yet.

**Tasks:**
- [x] Add `proto/ripls/models/scheduled_notification.proto` per the
      schema above (single `oneof item` with per-item-type variants
      `ExperienceNotification` and `LoanNotification`; each carrying
      its anchor id, a `purpose` enum, and a continuous
      `offset_seconds_from_anchor`; no status field; delete-on-fire
      lifecycle).
- [x] Add `optional int64 expected_return_unix_sec = 15` on the
      `Transfer` **models** proto (no API mirror; reconciler-only
      field). Set on the ACTIVE transition in
      `server/services/transfer/state_machine.go` whenever
      `loan_duration_days` is present. Indexed partially for loans
      that have it set.
- [x] `npm run generate` and confirm Go generated code compiles.
- [x] Add indexes on `scheduled_notification`:
      `(fire_at_unix_sec)` for the dispatcher's "what's due" pull and
      `(recipient_user_id)` for the per-user inspection query.
      Per-item-type reconciler queries filter on the oneof
      discriminator column (`experience_experience_id IS NOT NULL` or
      `loan_transfer_id IS NOT NULL`) — no dedicated index yet; add
      one if profiling shows it's needed. Delete-on-fire keeps the
      table small (rows ≈ in-flight entities × slots × recipients);
      no retention job needed.
- [x] Storage helpers (in `server/storage/scheduled_notification.go`):
  - `Insert` / `Delete` / `GetByID` — covered by the generic
    `ProtoSQLStorage` CRUD.
  - `UpdateScheduledNotificationFireAt(ctx, row, fireAt, updatedAt)` —
    targeted update used by the reconciler. Re-encodes `binary_proto`
    in lockstep with the flat column so callers reading via
    binary_proto don't see a stale fire_at.
  - `FindExperienceScheduledNotifications(ctx)` /
    `FindLoanScheduledNotifications(ctx)` — reconciler input per
    item type; returns every row whose `item` oneof matches that
    variant so the diff can run.
  - `FindDueScheduledNotifications(ctx, now, limit)` — dispatcher
    input. `WHERE fire_at_unix_sec <= now LIMIT n`, ordered FIFO.
  - `ClaimAndDeleteScheduledNotification(ctx, id)` — atomic
    `DELETE WHERE id = $1`, returns whether the caller won the row,
    mirroring the `RowsAffected()` shape of
    `ClaimCommunityPurgeReminder`.
- [x] Unit tests on every storage helper plus the new Transfer
      derivation paths.

**Validated:** Schema compiles, indexes exist, claim primitive is
atomic. No user-visible behavior change.

### Phase 2 — Reconciler/dispatcher framework + pre-event reminders

**Goal:** Ship the first user-visible scheduled notifications, end to
end, using the new framework. Pre-event reminders are the highest-value
case and exercise every layer (recipients, contributions context, quiet
hours, deep linking).

**Tasks:**
- [ ] `server/jobs/scheduled_notifications/` package with:
  - `framework.go` — types `Reconciler` (interface returning desired
    tuples) and `Dispatcher` (interface taking a `*ScheduledNotification`
    plus current entity state and returning `*models.Notification`).
    `Registry` mapping kind → (reconciler, dispatcher).
  - `reconcile.go` — driver that runs all registered reconcilers,
    diffs and upserts.
  - `dispatch.go` — driver that pulls due rows, applies quiet hours,
    looks up the per-kind dispatcher, applies user preferences,
    claims the row via `ClaimAndDeleteScheduledNotification`,
    delivers via `notifications.Service`, and logs the send. No
    SENT-state writeback — the claim *is* the deletion.
  - `quiet_hours.go` — shared timezone-aware "is now inside quiet
    hours?" + "next 07:00 local in Unix seconds" helpers.
- [ ] `experience_reminders.go` in that package:
  - Reconciler for the experience-anchored `REMINDER_BEFORE_START`
    purpose at three policy-defined offsets (-86400, -7200, 0).
    Inputs: non-deleted, non-cancelled experiences with a
    `SpecificTime` or `TimeRange.start_unix_sec` in the future. For
    each, expand RSVPs (YES + MAYBE) and produce three tuples per
    attendee `(attendee, ExperienceNotification{experience_id,
    REMINDER_BEFORE_START, offset}, fire_at, experience.timezone)`.
    Offsets live as policy constants in the reconciler — adding a
    new offset never changes proto.
  - Dispatcher: looks up the experience, the RSVP, and the
    recipient's claimed `PlanningContribution`s; builds title + body
    via a small server-rendered template, choosing copy from
    `offset_seconds_from_anchor` (e.g. `offset <= -7200` →
    contributions list "You're bringing: salad, breadsticks";
    `offset == 0` → location-emphasizing "starting now" copy). Use
    the user-facing word "Event" (CLAUDE.md). After a successful
    send the dispatcher calls `ClaimAndDeleteScheduledNotification`
    (which is what consumed the row to begin with) — no separate
    "mark SENT" step.
- [ ] Add `notify_event_reminders` to
      `CommunityNotificationPreferences` (models + API), default-on.
- [ ] Wire reconciler and dispatcher tickers in `main.go` (10-min and
      1-min cadences respectively). Spawn via
      `logging.GoSafe(ctx, "scheduled-notifications-reconciler", fn)`
      and `logging.GoSafe(ctx, "scheduled-notifications-dispatcher", fn)`
      so panics are caught and logged instead of crashing the process —
      raw `go func()` is rejected by the `check-goroutines` linter.
- [ ] Tests:
  - Reconciler upsert/delete correctness across event lifecycle:
    create → add RSVP → change time (PENDING `fire_at` updated in
    place) → remove RSVP (row hard-deleted) → re-RSVP (fresh PENDING
    row, prior SENT rows untouched) → cancel event (all PENDING rows
    for the anchor hard-deleted on next tick).
  - Dispatcher: quiet hours push `fire_at` forward to 07:00 local;
    preference toggle off at dispatch time → row hard-deleted;
    contributions render correctly; deleted experience between
    reconciler ticks → dispatcher safety-checks anchor existence and
    deletes if missing.
  - End-to-end integration test against a real `Experience` produces a
    push to the mock provider with the expected payload.

**Validates:** The framework works end-to-end for one realistic kind;
the reconciler self-heals across edits; quiet hours and prefs gate
correctly.

### Phase 3 — Loan return reminders + user-level preferences

**Goal:** Second reminder family. Exercise the framework against a
different anchor type and a different preference surface.

**Server:**
- [ ] New proto `UserNotificationPreferences` keyed by `user_id`,
      starting with `notify_loan_return_reminders` (boolean, default-on)
      and `notify_event_close_prompts` (used in Phase 4). Mirror in API
      proto.
- [ ] Host the new RPCs in `server/services/user/` (the
      `UserNotificationPreferences` row belongs to the user, not to a
      community): `GetUserNotificationPreferences` /
      `UpdateUserNotificationPreferences`. Use
      `connecterr.Internal(ctx, op, err, kv...)` for any storage error
      paths; never bare `connect.NewError(connect.CodeInternal, err)`.
- [ ] `loan_return_reminders.go`:
  - Reconciler for the three loan-return kinds. Input: active loans
    (state = ACTIVE) with `expected_return_unix_sec` set. Produce one
    tuple per slot per borrower. Borrower's quiet-hours timezone:
    most-recent `UserDevice.timezone` if we store it; otherwise leave
    empty (no quiet-hours adjustment).
  - Dispatcher: render title/body with gear title, owner name, and the
    appropriate framing (upcoming / due today / overdue).
- [ ] Wire kind into the registry.
- [ ] Tests covering each slot, opt-out, and the transition out of
      ACTIVE state cancelling pending rows on the next reconciler tick.

**Client:**
- [ ] **Service layer:** add `getUserNotificationPreferences` /
      `updateUserNotificationPreferences` methods to
      `app/lib/services/user_service.dart` (or the equivalent existing
      user-RPC client).
- [ ] **Repository layer:** extend `UserRepository` with cached
      `getNotificationPreferences()` and `updateNotificationPreferences()`
      methods using the existing `CacheService`. Use namespaced cache
      key `user:notification_prefs:<user_id>`. The update method
      invalidates the cached entry on success. Do not introduce a
      manual cache map in the viewmodel.
- [ ] **ViewModel:** new `UserNotificationPreferencesNotifier`
      (Riverpod `Notifier` with Freezed state). Loads via the
      repository; exposes per-toggle setters that call the repository's
      update method. No string resolution in the viewmodel —
      expose typed state only.
- [ ] **Screen:** simple settings screen under
      `app/lib/presentation/screens/`. Use `Toggle` (from
      `widgets/accessibility/`) for each preference switch — not raw
      `Switch` — so the toggle exposes correct `selected:` semantics.
      Navigate to the screen via `NavigationHelpers.pushWithSlide()`
      (utility screen, no swipe-to-close).
- [ ] **ARB strings:** add keys to `app/lib/l10n/app_en.arb` and
      `app_es.arb`. Follow `profile*` and `settings*` prefix
      conventions for visible copy, and `a11y*` prefix for any
      `semanticsLabel` strings. Required: title, section headers, and
      a short description per toggle. Use `context.l10n` at the widget
      layer — never in the viewmodel.
- [ ] **Tests:**
  - ViewModel tests with `ProviderContainer` + Mockito on the
    repository, covering load / toggle / error paths and
    `ref.mounted` checks if the notifier is `autoDispose`.
  - Repository tests confirming cache invalidation on update.

**Validates:** Adding a new reminder kind is `(enum value) +
(reconciler function) + (dispatcher function) + (preference)` and
nothing else — i.e. the framework's promise holds.

### Phase 4 — Event close prompts

**Goal:** Third kind. Should be almost trivial to add now.

**Tasks:**
- [ ] `event_close_prompts.go`:
  - Reconciler input: experiences in IN_PROCESS state (started but not
    completed) with `started_at_unix_sec` + event duration ≥ 1 day
    ago. One tuple per host.
  - Dispatcher: simple "Close [event name]?" copy with deep link to
    the event detail screen's complete-action.
- [ ] Tests: pending row cancelled when host marks completed; sent when
      threshold reached.

**Validates:** Adding a third reminder kind is a one-file change plus
tests. Confirms the abstraction holds.

### Phase 5 — Client deep-linking polish

**Goal:** Tapping any of these reminders lands on the right screen
with the relevant action prominent.

**Tasks:**
- [ ] Event reminders → event detail screen with RSVP banner and
      contributions list visible.
- [ ] Loan return reminders → borrowed-item detail screen with
      "Mark returned" prominent.
- [ ] Close prompts → event detail with "Mark completed" action
      surfaced.
- [ ] Route into target screens via `NavigationHelpers.pushScreen()`
      so the target uses `SwipeToCloseMixin` per the navigation
      conventions in `docs/client/architecture.md`.
- [ ] Notification deep-link tests: ViewModel-layer tests verifying
      that an inbound notification payload routes to the correct
      target screen identifier and seeds the right viewmodel
      initialization parameters. Use `ProviderContainer` + Mockito
      on the FCM service; do not test through a widget tree.

**Validates:** A real reminder produces a one-tap path to the action
the user wants to take.

### Phase 6 — Canonical design doc at `docs/server/scheduled_notifications.md`

**Goal:** Move the permanent design reference into the place a future
maintainer (human or LLM) will actually look for it. This issue doc
records *how the design evolved*; once the implementation has shipped
it stops being the entry point for anyone trying to understand how
the system works.

**Tasks:**
- [ ] Write `docs/server/scheduled_notifications.md` as a
      forward-looking design reference (not a plan with
      options + decisions): proto shape, reconciler / dispatcher
      contract, delete-on-fire lifecycle, uniqueness key,
      quiet-hours behavior, the per-item-type pattern, and
      observability conventions. Describe the system *as it is*, not
      *how it was chosen* — that history belongs in this issue doc
      and in commits.
- [ ] Cite the actual code so the reader can jump in:
      `server/jobs/scheduled_notifications/`,
      `server/storage/scheduled_notification.go`,
      `proto/ripls/models/scheduled_notification.proto`, plus the
      per-item-type files (`experience_reminders.go`,
      `loan_return_reminders.go`, etc.).
- [ ] Add a "How to add a new scheduled-notification kind" section
      walking through: extend the `item` oneof + per-kind purpose
      enum, add the storage finder, implement `Reconciler` +
      `Dispatcher`, register in `main.go`. One paragraph each.
- [ ] Cross-link from `server/jobs/scheduled_notifications/README.md`
      so the package readme points to the design doc rather than
      duplicating it.
- [ ] Compress this issue doc to a one-paragraph pointer at the top
      ("Implementation history — see `docs/server/scheduled_notifications.md`
      for current design"). Leave the rest as a frozen record of
      decision history so the *why* behind the shape stays
      retrievable.

**Validates:** A contributor unfamiliar with the work can read
`docs/server/scheduled_notifications.md` end-to-end and successfully
add a new reminder kind without consulting this issue doc, the
commit log, or the original GitHub issue.

### Phase 7 (deferred) — Recurrence nudges + email channel

Out of scope for the first ship. Recurrence-nudge heuristics deserve
their own design pass; email needs templates and unsubscribe handling.
Both fit cleanly on top of the framework when their time comes.

## Related Issues

- #1659 — Day-before community purge reminder. Source of the claim and
  dispatch patterns we're generalizing.
- #1622 — Notification dispatcher deleted-guard. Confirms scheduled
  reminders must respect entity soft-deletion (reconciler must cancel
  rows when the anchor is deleted).
- #1533 — Experience lifecycle notifications. Reactive cousins of the
  scheduled reminders here; overlap mainly in copy style.
- #1893 — Event update notifications. The reconciler must update pending
  rows when an event's time changes — this overlaps with the update
  notification flow.
- #1892 — Chat notification batching. Distantly related — if a reminder
  fires and the user already has unread chat in the same event's
  conversation, batching considerations apply.
- #977 — Notification deep links. Phase 5 of this plan depends on the
  routing primitives established there.
