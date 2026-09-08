---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Time-anchored push notifications (event reminders, loan-return prompts) driven by a reconciler/dispatcher goroutine pair — desired-state diffing, the delete-on-fire lifecycle, atomic claim-and-delete, quiet hours, and re-fire protection; distinct from the reactive RPC-triggered path.
  globs: [server/jobs/scheduled_notifications/**, server/notifications/**]
  triggers: [scheduled-notifications, reminder, reconciler, dispatcher, fire-at, quiet-hours, delete-on-fire, push]
  lens: [server, domain]
  domain: server
freshness:
  verified_commit: "9a2e20c14"
  verified_on: "2026-08-12"
---
# Scheduled Notifications

Push notifications that fire at scheduled times — keyed off durable
properties of in-flight entities (an Experience's start time, a loan's
expected return date) — rather than in response to a user action.

The reactive-notification path described in
[`docs/push_notifications.md`](../push_notifications.md) handles
everything that happens *because* of an RPC (someone RSVPed, someone
offered to help). Scheduled notifications handle everything that
should happen *at a planned future moment*. Both share the same
delivery surface (`server/notifications/Service`).

## Architecture at a glance

Two long-running goroutines, spawned from `server/jobs.go` via
`jobs.RunPeriodic` (panic-safe per the goroutine-safety contract in
[`docs/server/architecture.md`](architecture.md)):

- **Reconciler** (every 10 min,
  [`reconcile.go`](../../server/jobs/scheduled_notifications/reconcile.go))
  — for each registered kind, walks live entities to build a
  desired-state tuple set, loads the rows currently in storage for
  that kind, and applies inserts / fire-at updates / hard-deletes
  through the diff.
- **Dispatcher** (every 1 min,
  [`dispatch.go`](../../server/jobs/scheduled_notifications/dispatch.go))
  — pulls rows whose `fire_at_unix_sec` is in the past, routes each
  to the matching per-kind `Dispatcher` via `CanHandle`, applies
  the per-dispatcher quiet-hours policy, atomically claims (via
  `ClaimAndDeleteScheduledNotification`), renders copy from current
  entity state, and sends through `notifications.Service`.

Cadences chosen so the reconciler's grace window (5 minutes — see
*Re-fire protection* below) is smaller than its interval.

## The proto

The row shape and per-item-type variants are defined in
[`proto/ripls/models/scheduled_notification.proto`](../../proto/ripls/models/scheduled_notification.proto).
A single `ScheduledNotification` carries the recipient, optional
community context, `fire_at_unix_sec`, optional quiet-hours
timezone, and an `oneof item` that fully identifies both the entity
type and its per-item-type configuration — there is no top-level
`kind` enum.

Per-item-type variants — see the proto for the full definitions:

- `ExperienceNotification` — anchored to an `Experience.id`.
  Purposes include pre-event reminders (negative offset), the
  close-prompt (positive offset), and a reserved recurrence
  reminder.
- `LoanNotification` — anchored to a `Transfer.id`. The return
  reminder is the only purpose today; sign of the offset
  distinguishes upcoming-due from overdue.
- `RequestNotification` — anchored to a `Request.id`. The
  followup-prompt purpose fires repeatedly (3/6/9/12/15 days from
  `created_at_unix_sec`) while the request stays ACTIVE or
  OFFERS_RECEIVED, one row per offset.

**No `status` field, no `sent_at` field.** Every row in the table is
something the dispatcher should still fire. Record of send lives in
structured logs, not in the table (see *Observability* below).

## Lifecycle

The lifecycle is **delete-on-fire**: rows are hard-deleted at every
exit point.

1. Reconciler INSERTs a row when its desired set contains a tuple
   that has no matching row in storage.
2. Reconciler UPDATEs `fire_at_unix_sec` when a policy or anchor
   change moves the desired fire time (e.g. an Experience host
   reschedules).
3. Reconciler DELETEs a row whose key has left the desired set
   (Experience cancelled, RSVP withdrawn, loan returned, etc.) —
   **provided `fire_at` is still in the future**. Past-fire-at rows
   belong to the dispatcher.
4. Dispatcher atomically DELETEs a row via
   `ClaimAndDeleteScheduledNotification` and immediately sends. If the
   send fails post-claim, the push is lost (logged as ERROR). FCM is
   reliable and crash-mid-send is rare; we accepted this tradeoff to
   avoid a retry queue with its own correctness surface.

A re-scheduled Experience therefore re-fires its reminders
naturally: the old SENT-style record is gone, so the reconciler sees
no existing row for the (recipient, kind, anchor, purpose, offset)
key, and inserts fresh at the new `fire_at`.

## Uniqueness key

`(recipient_user_id, item-variant-id, purpose, offset_seconds_from_anchor)`.

Two reminders to the same user about the same Experience with the
same purpose but different offsets (24h-before AND 2h-before) are
distinct rows. The same purpose+offset to the same recipient about
the same anchor is one row that gets fire-time-updated when policy
changes, never duplicated.

The reconciler driver builds this key via
[`UniquenessKey`](../../server/jobs/scheduled_notifications/framework.go).

## Re-fire protection

The reconciler runs every 10 minutes. The dispatcher runs every 1
minute. After the dispatcher fires and deletes a row, the next
reconciler tick must NOT re-insert a tuple for the same key — or the
dispatcher fires it again and the user gets a duplicate push.

Three coordinated rules prevent the loop:

- **Reconciler emits only future-or-recent tuples.**
  `DesiredFireAtGrace` (5 minutes) is the maximum staleness for a
  desired-state tuple. Tuples whose fire_at is older than
  `now - 5min` are dropped. The grace must be smaller than the
  reconciler tick interval — otherwise the next tick re-emits.
- **Reconciler doesn't delete past-fire-at rows.** Even when a row's
  key has left the desired set, if its `fire_at` is in the past, the
  reconciler leaves it alone. The dispatcher owns those rows and will
  either fire them (a brief dispatcher outage caught up) or, in a
  future enhancement, drop them as too stale.
- **DB-level partial unique indexes on the per-`item`-variant
  uniqueness tuple guarantee at most one row per key, regardless of
  reconciler concurrency.** Three partial unique indexes (one per
  `item` oneof variant — experience, loan, request) enforce the
  invariant at write time. A second insert with the same
  `(recipient_user_id, anchor-id, purpose, offset)` tuple is silently
  rejected via `ON CONFLICT DO NOTHING` (`InsertScheduledNotificationIfAbsent`
  in `server/storage/scheduled_notification.go`). This closes the
  multi-replica race that caused duplicate close-prompt notifications
  (#2458).

These rules also let the dispatcher handle short outages cleanly: a
row emitted at fire_at = T, missed by the dispatcher because it was
down for 8 minutes, stays as PENDING in the table. Reconciler ticks
during the outage skip over it (no re-emission, no deletion). When
the dispatcher recovers, it fires the row.

## Quiet hours

The dispatcher checks whether a row's recipient is currently in their
local quiet-hours window (**22:00–07:00** in the row's
`quiet_hours_timezone`). When they are, what happens next depends on
the **per-dispatcher** `QuietHoursPolicy`:

- **`QuietHoursPolicyDefer`** (default): leave the row PENDING and
  try again on the next dispatcher tick. The reminder eventually
  fires when local time crosses 07:00. Suitable for **day-grained**
  reminders ("event tomorrow", "due tomorrow", "wrap up your event")
  whose copy stays accurate after a multi-hour shift.
- **`QuietHoursPolicySkip`**: claim and delete the row without
  sending. Suitable for **short-lead** reminders ("starts in 2
  hours", "starting now") whose copy would be actively misleading if
  delayed until 07:00 the next morning. The recipient simply doesn't
  get this reminder; they still get any longer-lead reminders that
  fired earlier in the day.

The current policy assignments:

| Dispatcher | Slot | Policy |
|---|---|---|
| `ExperienceReminderDispatcher` | 24h-before-start | Defer |
| `ExperienceReminderDispatcher` | 2h-before-start | Skip |
| `ExperienceReminderDispatcher` | starting-now | Skip |
| `ExperienceClosePromptDispatcher` | 24h-after-start | Defer |
| `LoanReturnReminderDispatcher` | day-before due | Defer |
| `LoanReturnReminderDispatcher` | due-day | Defer |
| `LoanReturnReminderDispatcher` | 3-days-overdue | Defer |
| `RequestFollowupDispatcher` | 3/6/9/12/15-days-since-created | Defer |

A 9-hour quiet window with a 1-minute dispatcher tick means a
deferred row gets ~540 wasted pulls per recipient overnight —
acceptable at current scale; can be optimized later by writing the
deferred `fire_at` forward to 07:00 local.

Empty or unrecognized timezones disable quiet hours entirely for
that row (better to ping at a slightly inconvenient hour than to
silently swallow a reminder because we couldn't parse the
recipient's locale).

See
[`server/jobs/scheduled_notifications/quiet_hours.go`](../../server/jobs/scheduled_notifications/quiet_hours.go)
and
[`framework.go`](../../server/jobs/scheduled_notifications/framework.go)
(`QuietHoursPolicy`).

## Preference gating

Two preference surfaces, applied at dispatch time (post-claim):

- **Per-community** —
  `CommunityNotificationPreferences.notify_event_reminders` covers
  pre-event reminders. Read via `community.CategoryEnabled` with
  category `NOTIFICATION_CATEGORY_EVENT_REMINDERS`.
  `CommunityNotificationPreferences.notify_request_followup_prompts`
  gates request followup prompts the same way, but
  `RequestFollowupDispatcher.Render` reads the field directly rather
  than going through `CategoryFor`/`CategoryEnabled` (there is no
  matching `NOTIFICATION_CATEGORY_*` case for it).
- **Per-user** — `UserNotificationPreferences` covers loan-return
  reminders (`notify_loan_return_reminders`) and Experience close
  prompts (`notify_event_close_prompts`). These are about "your
  stuff" / "your event", not about a community feed, so they follow
  the user across all communities. Read via
  `user.LoadUserNotificationPreferences`.

Both surfaces use the **"unset = on"** convention: a missing row, or
a missing field, means the user has not opted out.

Preference-fetch errors **fail open** (deliver) rather than suppress
a real reminder on a transient DB hiccup. Losing a reminder to a
DB blip is a worse user experience than the rare case of a
just-disabled user getting one more reminder.

## Per-item-type pattern

Each item type has:

- A per-item-type config message inside `oneof item` in the proto.
- A `Reconciler` implementation that emits tuples for that item type.
  Its `Existing()` calls the type-specific storage finder
  (`FindExperienceScheduledNotifications` /
  `FindLoanScheduledNotifications`) so the diff scope matches what
  the reconciler emits.
- One or more `Dispatcher` implementations, each handling one
  `purpose` within the item type. `CanHandle` does the routing.
- (Optional) A `LoanWorld` / `ExperienceReader`-style storage
  abstraction so unit tests can pass a fake without testcontainers.

**Why one reconciler per item type, not per purpose?** Two reconcilers
loading the same `Existing()` set would each see the other's rows as
"in storage but not in my desired set" and try to delete them. The
diff scope is per-item-type because the storage finder is per-item-type.
Multiple purposes can share a reconciler; multiple item types
cannot.

## Adding a new scheduled-notification kind

Four independent steps, each landable as its own PR.

1. **Schema** —
   - Add a new `*Notification` message to
     `proto/ripls/models/scheduled_notification.proto` with the
     entity anchor id, a `purpose` enum, and an
     `offset_seconds_from_anchor` int64.
   - Add it as a new variant of the `oneof item` field.
   - In `server/storage/scheduled_notification.go`, add a
     `FindXScheduledNotifications(ctx)` helper that selects on the
     new variant's discriminator column (protosql flattens the oneof
     by variant name; see the existing two finders for the column
     name convention).
   - In `framework.go`, extend `UniquenessKey` to handle the new
     `item` variant.

2. **Reconciler + dispatcher** — add a new file
   `server/jobs/scheduled_notifications/<kind>_reminders.go`
   implementing both `Reconciler` and `Dispatcher` for the new kind.
   Follow the existing pairs (Experience, Loan, Request) as templates.
   Remember:

   - `Desired()` filters out tuples whose `fire_at` is past
     `DesiredFireAtGrace` ago.
   - `Existing()` calls the new storage finder so the diff sees only
     rows this reconciler owns.
   - `Render()` returns `(nil, nil)` when the anchor is gone, the
     entity is in a terminal state, or a preference check disables
     this kind for the recipient.
   - Preference-fetch errors fail open.
   - `Render()` returns `notification_content.SystemNotification(ctx,
     payload)` — never a hand-built `Notification` with `fmt.Sprintf`
     copy. See step 3.

3. **Copy** — a scheduled notification reaches a recipient with no
   device through SMS or email, and those senders ignore
   `Notification.Title`/`Body` entirely: they re-render from the
   payload. Copy written inline in the dispatcher therefore reaches
   push only, and the off-app surfaces fall through to the generic
   default line — which for an actorless reminder means texting
   someone `Ripls:  posted an update in ` (#2896). So:

   - Mint an event-type string per *wording*, not per kind: the offset
     is not on the payload, so a reminder with three slots needs three
     event types (`EXPERIENCE_REMINDER_{DAY_BEFORE,TWO_HOUR,STARTING}`).
   - Add each to `notification_content.SystemEventTypes` and map it in
     `offAppKinds`. `TestEveryNotificationEventTypeHasCopy` fails the
     build until you do.
   - Add `notif.community_event.{kind}.title` (the push category
     label) and `notif.offapp.{kind}.{message,cta}` to **both**
     `server/l10n/source/en.toml` and `es.toml`. `.message` is the one
     self-contained sentence every surface renders — write it so it
     stands alone with no title above it.
   - Put every name the sentence interpolates on the payload, from an
     entity the dispatcher has **already loaded**. Do not add a lookup
     to decorate copy: `Dispatch` renders inside a per-row loop, so one
     read there is one read per due notification per tick.
   - Never rename an existing event-type string.
     `app/lib/services/fcm_service.dart` matches
     `REQUEST_FOLLOWUP_PROMPT` verbatim for deep-link routing, and an
     older installed client would lose it.

4. **Wiring** — in `server/jobs.go`, append the new reconciler and
   dispatcher to `scheduledReconcilers` / `scheduledDispatchers`.
   Don't touch the `jobs.RunPeriodic` driver; the registry pattern
   means new kinds slot in without changing it.

Tests for the new kind should cover: the three-state emission
(future, recent-past inside grace, past-grace), preference gating
(unset, explicit-on, explicit-off, fetch-error fails open), and the
two anchor-vanished skip paths (anchor missing, anchor in a terminal
state).

## Storage

Table `scheduled_notification`, defined by the proto. Five indexes:

- `(fire_at_unix_sec)` — powers the dispatcher's "what's due" pull.
- `(recipient_user_id)` — powers the per-user inspection query
  ("show me what's scheduled for me").
- `idx_scheduled_notification_unique_experience` — partial unique
  index on `(recipient_user_id, experience_experience_id, experience_purpose,
  experience_offset_seconds_from_anchor) WHERE experience_experience_id IS NOT NULL
  AND experience_experience_id <> ''`. Enforces uniqueness key at DB level (#2458).
- `idx_scheduled_notification_unique_loan` — symmetric partial unique
  index for the loan variant (keyed on `loan_transfer_id`).
- `idx_scheduled_notification_unique_request` — symmetric partial unique
  index for the request variant (keyed on `request_request_id`).

Per-item-type queries filter on the oneof discriminator column
(`experience_experience_id IS NOT NULL`,
`loan_transfer_id IS NOT NULL`). The unique indexes double as lookup
indexes for these per-type scans.

## Idempotent insert

The reconciler inserts via
[`InsertScheduledNotificationIfAbsent`](../../server/storage/scheduled_notification.go)
(`INSERT … ON CONFLICT DO NOTHING`). A conflict — meaning another reconciler
replica already wrote the row — is counted as `Unchanged` (not an error) and
logged at DEBUG. This means a second replica racing to insert the same tuple
produces no duplicate row and no alert noise.

A duplicate-reaper pass runs at the start of every reconciler tick to clean up
rows that may have been inserted before the unique indexes existed. The reaper
groups `Existing()` rows by uniqueness key; for each group with more than one
future-fire-at row, it deletes all but the smallest-id row (deterministic choice
among functionally equivalent rows) and emits an INFO log line per discarded row
with `outcome = "reaped_duplicate"`, useful as a before/after signal after the
fix deploys. Past-fire-at rows are never touched by the reaper — the dispatcher
owns them.

Companion field on `Transfer`: `expected_return_unix_sec`, computed
on the loan's ACTIVE transition from `actual_pickup_unix_sec +
loan_duration_days * 86400`. Indexed via a partial index
(`WHERE expected_return_unix_sec IS NOT NULL`) so the loan reconciler
scan is cheap. **Models-only — no API mirror.** Clients reach
the reminder feature through push payloads, not through reading the
Transfer object's due date directly.

## Observability

All reconciler / dispatcher code uses
[`logging.LoggerWithContext`](../../server/logging/) with these
conventions:

- `operation` field is `ScheduledNotificationReconcile` or
  `ScheduledNotificationDispatch` on driver lines; the per-kind
  functions inherit it.
- Standard entity field names: `recipient_user_id`, `community_id`,
  `experience_id` / `transfer_id` (from the `item` oneof),
  `scheduled_notification_id`, `purpose`,
  `offset_seconds_from_anchor`, `fire_at_unix_sec`.
- Log levels: INFO for per-tick summaries (counts) and per-row
  dispatch successes; DEBUG for reconciler diff decisions and
  quiet-hours skips; WARN for preference-fetch failures (fail-open
  path); ERROR for storage errors, unrecognized `item` variants at
  dispatch time, and provider failures post-claim (push lost).

The `rpc_errors` alert policy in
[`terraform/modules/monitoring/main.tf`](../../terraform/modules/monitoring/main.tf)
groups by `operation`, so an elevated ERROR rate on either operation
fires automatically without further configuration.

## Files

| Concern | Path |
|---|---|
| Proto definitions | [`proto/ripls/models/scheduled_notification.proto`](../../proto/ripls/models/scheduled_notification.proto) |
| Storage helpers | [`server/storage/scheduled_notification.go`](../../server/storage/scheduled_notification.go) |
| Transfer.expected_return derivation | [`server/services/transfer/state_machine.go`](../../server/services/transfer/state_machine.go) |
| Reconciler / dispatcher framework | [`server/jobs/scheduled_notifications/framework.go`](../../server/jobs/scheduled_notifications/framework.go), [`reconcile.go`](../../server/jobs/scheduled_notifications/reconcile.go), [`dispatch.go`](../../server/jobs/scheduled_notifications/dispatch.go) |
| Quiet hours | [`server/jobs/scheduled_notifications/quiet_hours.go`](../../server/jobs/scheduled_notifications/quiet_hours.go) |
| Experience reminders + close prompts | [`server/jobs/scheduled_notifications/experience_reminders.go`](../../server/jobs/scheduled_notifications/experience_reminders.go) |
| Loan return reminders | [`server/jobs/scheduled_notifications/loan_return_reminders.go`](../../server/jobs/scheduled_notifications/loan_return_reminders.go), [`loan_world.go`](../../server/jobs/scheduled_notifications/loan_world.go) |
| Request followup prompts | [`server/jobs/scheduled_notifications/request_followup_prompts.go`](../../server/jobs/scheduled_notifications/request_followup_prompts.go) |
| User-scoped prefs RPCs | [`server/services/user/notification_prefs.go`](../../server/services/user/notification_prefs.go) |
| Per-community pref category | [`server/community/notifications.go`](../../server/community/notifications.go) — `CategoryEnabled` |
| Startup wiring | [`server/jobs.go`](../../server/jobs.go) — search `scheduledReconcilers` |
| Client deep-link routing | [`app/lib/services/fcm_service.dart`](../../app/lib/services/fcm_service.dart) — `routeNotificationPayload` |

## See also

- [`docs/push_notifications.md`](../push_notifications.md) — the
  reactive notification path that scheduled notifications complement.
- [`docs/issues/625-scheduled-notifications.md`](../issues/625-scheduled-notifications.md)
  — implementation history (decisions, options that were rejected,
  schema revisions). Read this only if you need to understand *why*
  the design is shaped the way it is; for *what* it does, this
  document is canonical.
