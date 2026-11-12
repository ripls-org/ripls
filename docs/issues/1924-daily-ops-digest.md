# Issue #1924: Daily digest email to ops of all activity across all communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1924
**Filed:** 2026-05-14 | **Last activity:** 2026-05-14
**Labels:** none | **Assignees:** maintainer
**Context docs:** none (user did not pass any; the issue body links to Metabase
analytics conceptually but provides no doc reference)

## Audit findings (applied 2026-05-14)

`/ripls-audit` against `docs/server/architecture.md` and
`docs/server/observability.md` surfaced the items below. All have been
reconciled inline in the plan; this section is the changelog of what
moved and why.

1. **Name + scope collision with `observability.md` Part 3.** The
   observability roadmap already has a planned "Automated Daily Digest
   Reports" feature in `server/reports/` that ships server *health* +
   error + traffic data via Cloud Scheduler → `/internal/reports/daily-digest`
   to `HEALTH_REPORT_RECIPIENTS`. Neither the package nor the endpoint
   exist yet, but the name "daily digest" is taken. **Resolution:** this
   feature is renamed throughout to the **Daily Activity Digest** and
   lives at `server/activity_digest/`. The email-service method is
   `SendActivityDigest`. The flag namespace is `--activity-digest-*`. The
   two digests stay separate — one is engagement, one is ops health —
   and the team can decide later whether to fold them into a single
   email at the template layer. Cross-reference both ways: this plan
   names the observability digest; the observability roadmap should
   gain a back-pointer when its phase 4 work begins.
2. **Cloud Scheduler vs in-process ticker.** `observability.md` Part 3
   chose Cloud Scheduler for the same shape of work this plan describes.
   The codebase has precedent for both (Cloud Scheduler is not yet
   wired, in-process ticker via `time.NewTicker` is the standard for
   `CommunityPurgeReminderJob` and `CommunityPurgeJob`). **Resolution:**
   keep the in-process ticker for v1 because it matches the existing
   shipped jobs and avoids a Terraform module + service account on the
   path-to-prod. Drop a note that if/when the observability digest
   adopts Cloud Scheduler, the activity digest should follow — making a
   one-off architectural inconsistency unattractive to anyone reading
   `main.go`. Captured in Open Questions.
3. **Admin RPC gating.** The plan originally called for an
   `SendDigestNow` admin RPC "operator-gated, like the other admin
   RPCs." The admin service today gates only on `devMode`
   (`server/services/admin/cleanup.go`), which is off in prod.
   **Resolution:** keep the admin RPC dev-only — operators iterate on
   the template in dev, and prod re-sends are handled by flipping the
   schedule flag or (later) by the Cloud Scheduler "run now" button.
   Plan updated to match.
4. **Error-wrapping convention.** Any new code path that returns a
   `connect.CodeInternal` must go through
   `server/connecterr.Internal(ctx, op, err, kv...)` — the bare
   `connect.NewError(connect.CodeInternal, err)` form is CI-gated
   (`scripts/check_codeinternal.js`). **Resolution:** explicit task
   added to Phase 3 for the admin RPC.
5. **Structured logging conventions.** `observability.md` requires
   `operation`, `request_id` (HTTP correlation only), masked
   emails via `logging.MaskEmail`, and an explicit field list per
   operation. **Resolution:** Phase 3 now enumerates the field set for
   the digest job and the admin RPC.
6. **PII in the rendered email.** The body intentionally surfaces new
   user names + emails and waitlist signup details so ops can act on
   them. That's a deliberate trade-off (it's an internal email to a
   single ops mailbox), but the same data must remain *masked in
   logs*. The observability rules apply to log lines, not to the
   intentional content of the email body. **Resolution:** Phase 1's
   template task notes that user-identifying fields appear in the
   body verbatim; Phase 3's logging task notes that the same fields
   must be masked when logged.
7. **Service-to-service boundary.** `Builder` lives in a new shared
   library (`server/activity_digest/`), depends on
   `server/storage/`, `server/email/`, and the shared user-resolution
   helper `services.FetchAPIUsersBatch` (which is itself a shared
   helper at `server/services/` package level — *not* a service
   handle). **Resolution:** no change needed; the original design
   already complies with the "no service-to-service imports" rule.
   Noted here for the audit record.
8. **No client work.** This feature is server-only. The
   `docs/client/*` audit pass produced no findings — there is no
   ViewModel, Repository, ARB, or a11y surface touched by this plan.
   Recorded for the audit record.


## Problem Statement

We currently have no human-readable, push-style daily summary of what happened
across all communities. Firebase / GA4 covers client behavior; Metabase covers
ad-hoc SQL exploration; but no one gets a once-a-day "here is everything that
happened yesterday" digest in their inbox. The issue asks for a daily email to
`ops@ripls.org` (test target `user2@example.org`, on dev infra first) that
aggregates and summarizes activity from across every community.

A purely `CommunityEvent`-based digest captures the *transactional* surface
(gear flow, transfers, requests, experiences, RSVPs, invitations, membership
changes) but misses two equally important signals of platform health:

1. **Conversation activity** — `chat_message` rows whose oneof is
   `user_message` (system messages are already represented in
   `CommunityEvent` and shouldn't be counted twice). Communities can be
   active in conversation without any gear transaction firing; a digest
   that ignores chat will under-report engagement.
2. **Authentication activity** — new user signups (`user.created_at`),
   recurring logins (`refresh_token.created_at_unix_sec`), waitlist
   signups (`waitlist_entry`), and password-reset requests
   (`pending_password_reset.created_at_unix_sec`). These tell ops who
   is coming in the door, how often returning users are coming back,
   and whether anyone is bouncing off the auth flow.

So the digest is built from three data sources, in this order of
prominence in the rendered email: **Authentication** (new arrivals,
returning users) → **Community activity** (the `CommunityEvent`
narrative) → **Chat** (conversation volume per community). Each has the
same plumbing requirement: raw SQL over a time window plus batch
resolution of foreign keys (users, communities, conversations).

The plumbing required to convert this into an email already exists in
`server/email/` (Mailgun + embedded templates), `server/jobs/` (daily-tick
+ claim-based idempotency, modelled on `CommunityPurgeReminderJob`), and
`server/storage/` (raw `QueryContext` with proto unmarshal, modelled on
`FindCommunitiesNeedingPurgeReminder`).

There is no ambiguity in the issue itself, but several decisions about
scope, schedule, and idempotency need calling out before code lands —
those are in Open Questions.

> The branch this plan is being written on is already named `email_digest`,
> matching the assignee. Treat it as the working branch.

## Context from Comments

The GitHub issue has **no comments** — the body is the entire spec, and it is
short. The author flagged two architectural alternatives in the body itself:

- **Metabase-driven**: piggyback on the existing Metabase analytics deployment
  (Pulses / Subscriptions can email dashboard snapshots on a cron).
- **Server-driven**: the server is always running, owns the data, and already
  has Mailgun wired up — just write a job.

The author leans server-driven ("may be easier"). Recipient is
`ops@ripls.org` in production, `user2@example.org` for dev testing.

A subsequent in-conversation note from the user (after the plan was opened)
asked whether the **same periodic mechanism** could also solve #625
(scheduled notifications timed before an event). That's addressed in
[Relationship to #625](#relationship-to-625) below.

## Codebase audit

What already exists that this plan will lean on:

- **`server/email/`** — `email.Service` interface with `MailgunService` and
  `MockEmailService`. Each existing email type (password reset, waitlist
  welcome, waitlist notification) is a method on `Service`, a pair of `.html`
  + `.txt` embedded templates, and identical wiring in mock + mailgun.
  Adding `SendDailyDigest` follows the exact same shape — see
  `server/email/README.md` step-by-step.
- **`server/jobs/`** — the canonical daily-job pattern is
  `CommunityPurgeReminderJob`: constructor takes the storage handle (and any
  collaborators), `Run(ctx)` does one pass, and `main.go` wires
  `logging.GoSafe(ctx, "name", ...)` with a startup `Run` plus a 24h
  `time.NewTicker`. Idempotency is enforced by a conditional UPDATE primitive
  (`ClaimCommunityPurgeReminder`) in `server/storage/community.go` so that
  re-runs and concurrent instances are safe. Prod and dev are sized at
  `max_instances = 2`, so idempotency is **required**, not optional.
- **`server/storage/`** — raw-SQL query pattern is established
  (`FindCommunitiesNeedingPurgeReminder` in `community.go:454`). The
  `community_event` table already has the right composite index
  (`idx_community_event_community_occurred` on `(community_id,
  occurred_at_unix_sec DESC)`), but the digest queries on the *time leading
  column*; we'll need an index on `occurred_at_unix_sec` (descending) to
  avoid a sequential scan across the whole table once daily. That's a single
  `CREATE INDEX IF NOT EXISTS` add in `protosql_schema.go`.
- **`server/services/community/events.go`** — already does the user/gear
  resolution we'll need for human-readable rendering. The actor and object
  resolution via `services.FetchAPIUsersBatch` and `storage.GetByIDs` is
  reusable; we should not duplicate it inside the job.
- **`CommunityEvent` enum** — 40+ values, including UNDONE retractions and
  "lifecycle update" types like `EXPERIENCE_UPDATED`. The digest will
  intentionally exclude UNDONE events from headline counts (they net out the
  original action) and roll up the noisiest ones (RSVP_MAYBE/NO, planning
  add/remove churn) into a single line per community. See Open Questions.
- **`chat_message` / `chat_conversation` tables** —
  `ChatMessage` carries `sent_at_unix_sec`, a `conversation_id`, an
  optional `deleted` field, and a `oneof message` discriminator. The
  protosql layer stores the oneof as two nullable columns
  (`user_message`, `system_message`); the digest filters for
  `user_message IS NOT NULL` and joins through `chat_conversation` to
  recover `community_id`. There is no time-only index on
  `chat_message.sent_at_unix_sec` today — same `CREATE INDEX IF NOT
  EXISTS` add required as for `community_event`.
- **`user`, `refresh_token`, `waitlist_entry`,
  `pending_password_reset` tables** — `User.created_at` is the
  signup timestamp; `RefreshToken.created_at_unix_sec` is the
  best login proxy (a fresh refresh token is minted at every
  successful sign-in for every auth method);
  `WaitlistEntry.created_at_unix_sec` is the waitlist signup
  timestamp; `PendingPasswordReset.created_at_unix_sec` is the
  reset-request timestamp. All are already SQL-queryable. `User`
  is the only table that already has indexes on
  related columns; refresh-token / waitlist / password-reset
  scans across a 24h window are small enough that one daily query
  on a `created_at_unix_sec` filter is cheap, but a single
  index per table is still worth adding so the cost stays cheap
  as the tables grow.
- **`User.auth_method` enum** — `EMAIL_PASSWORD`, `GOOGLE`,
  `APPLE`, `PHONE`. The digest breaks new-signup counts down by
  this enum so ops can see which auth flow is actually being
  used.
- **Mailgun config** — `--mailgun-api-key`, `--mailgun-domain` (default
  `ripls.app`), `--mailgun-from` (default `Ripls <noreply@ripls.app>`)
  already plumbed through `cfg` in `main.go`. The waitlist notification flow
  already uses a flag for the recipient (`--waitlist-notify-email`, default
  `support@ripls.org`) — the digest should follow this exact pattern with
  `--digest-notify-email` (default `ops@ripls.org`).

What does **not** exist and would need to be built:

- An email-digest template (HTML + text) that covers all three
  sections (auth, community activity, chat).
- A storage-side aggregator that pulls community events, chat
  messages (joined to conversations), users, refresh tokens,
  waitlist entries, and password-reset requests for a date range
  and bulk-loads the referenced users/gear/communities without
  any N+1.
- A small idempotency primitive keyed by run-date (so two Cloud Run
  instances ticking the same day don't both send).
- Two new time-leading indexes:
  `idx_community_event_occurred` on `community_event
  (occurred_at_unix_sec DESC)`, and
  `idx_chat_message_sent` on `chat_message (sent_at_unix_sec
  DESC)`.

## Potential Solutions

### Option A: Server-side daily job in `server/jobs/`

- **Approach:** New `DailyDigestJob` in `server/jobs/daily_digest.go` that
  on each tick: (1) computes the digest window `[start, end)` for "yesterday"
  in a configured timezone; (2) atomically claims that date in a tiny
  `daily_digest_run` table (or, simpler, a one-row state singleton with
  `last_sent_date`); (3) queries `community_event` rows in the window via
  a new `storage.FindEventsInWindow(ctx, start, end)` raw-SQL helper;
  (4) groups by `community_id`, batch-fetches users + gear + community
  names; (5) renders one HTML + plain-text email via a new
  `email.Service.SendDailyDigest`. Schedule: `logging.GoSafe` in `main.go`
  with a 1-hour tick that no-ops unless the current local hour matches a
  configured "send hour" (default 08:00 in `America/Los_Angeles`). Catch-up:
  on startup, run once — if today's digest already shipped it claims-fails
  and does nothing.
- **Pros:**
  - Matches the author's stated preference and reuses every piece of infra
    the server already has (Mailgun, storage, jobs pattern, claim
    idempotency, batch resolution).
  - Templating is local; copy lives next to the rest of the email templates.
  - Easy to extend later — add `SendWeeklyDigest`, per-community digests,
    or replace `ops@` with a list — without touching infrastructure.
  - Trivially testable with `MockEmailService` and `SetupTestStorage`; no
    external system in the loop.
  - No new infrastructure (no Cloud Scheduler, no Metabase Pulse config).
- **Cons:**
  - Server owns yet another scheduled responsibility. Two instances means
    the claim primitive is required — but that pattern already exists.
  - Hand-rolled aggregation logic must stay aligned with the actual set
    of `CommunityEventType` values as the enum grows. Some discipline
    needed to update digest rollup rules when new event types land
    (similar to the `undo/registry.go` discipline, but lower stakes —
    a missed event type just shows up as a generic line item).
- **Effort:** Small — most of the work is template prose and the rollup
  table.

### Option B: Metabase Pulse / Subscription

- **Approach:** Build a Metabase question that aggregates yesterday's
  `community_event` rows, then attach a daily Subscription that emails the
  result to `ops@ripls.org`. Zero Go code; lives entirely in the Metabase
  admin UI.
- **Pros:**
  - No new code to maintain in the server.
  - Reuses existing Metabase deployment.
- **Cons:**
  - Output looks like a Metabase dashboard email (table + chart image), not
    a curated narrative. Hard to add prose like "Alice loaned a tent to
    Bob in Backyard Tinkerers."
  - Resolving foreign keys (user → email/name, gear → name) in a single
    Metabase question gets ugly fast — multiple JOINs per row, no batch
    fetch, no oneof topic field to switch on.
  - Subscription config lives in Metabase, not in source — review,
    testing, and rollback are weaker.
  - Not on the path to richer digests later (per-community digests,
    inline action buttons, etc.).
- **Effort:** Small to ship a crude version; medium if we want a polished
  result.

### Option C: Cloud Scheduler → admin RPC

- **Approach:** Add an admin RPC `SendDigestNow` that builds and sends the
  digest synchronously; wire a Google Cloud Scheduler job in Terraform that
  hits it once a day with OIDC auth.
- **Pros:**
  - Externalizes the schedule so the server is "pure" — no in-process
    ticker.
  - Schedule is visible in the GCP console; pause/run-now is one click.
- **Cons:**
  - All the same Go-side aggregation work as Option A, plus a new admin
    auth path, plus a new Terraform module, plus a second source of truth
    for "when does the digest fire."
  - Not justified at this scale — we have one daily cron, not dozens.
- **Effort:** Medium.

## Recommendation

**Option A** — server-side daily job. The issue itself favors this; the
codebase already has every primitive (email service, jobs pattern, claim
idempotency, batch user/gear fetch); and the alternatives either produce
weaker output (B) or add infrastructure with no offsetting benefit at our
scale (C).

The one nuance worth being explicit about: with `max_instances = 2`,
**idempotency is mandatory**, not optional. Use the same conditional
UPDATE pattern as `ClaimCommunityPurgeReminder` — either a one-row
singleton table with a `last_sent_date` column whose UPDATE has a
`WHERE last_sent_date < $newDate` guard, or a tiny `daily_digest_run`
table with a UNIQUE constraint on `run_date`. The singleton is simpler;
the table version is easier to audit if we ever want a history of when
the digest ran. Recommend the **singleton**.

## Relationship to #625

The user asked whether the periodic-email mechanism here could also solve
[#625 — Scheduled notifications (e.g. timed before an event)](https://github.com/Type2Fun-Foundation/ripls/issues/625).
Honest answer: **partially, but not as much as it sounds.**

What's genuinely shared:

- The **jobs pattern** itself — a `logging.GoSafe` ticker that wakes on a
  schedule, finds candidates, and dispatches with claim-based idempotency.
  This pattern is already in the codebase (`CommunityPurgeReminderJob`);
  this issue uses it, #625 will too. Shipping #1924 first does not change
  what #625 needs to do.
- The **dispatcher half** — Mailgun for email, FCM for push. Both already
  exist. #625 is push-only; it doesn't touch the email path at all.

What's **not** shared:

- **Trigger model.** The digest is a cron tick: "every day at 08:00
  local." #625 is *time-relative to an entity*: "N hours before
  `Experience.start_unix_sec`, for each Yes/Maybe RSVP, send a push." The
  candidate query is fundamentally different — the digest scans events in
  a fixed window; #625 scans experiences whose start time falls into a
  window and then fans out to that experience's RSVPs.
- **Per-user preferences.** The digest goes to one ops mailbox. #625
  goes to every RSVP'd user and must respect `CommunityNotificationPreferences`.
- **Catch-up semantics.** Missing yesterday's digest by a few hours is
  fine — the next day's digest is still useful. Missing an event reminder
  by a few hours is *not* fine; the event is over. #625 will likely need a
  finer tick (hourly or every-15-min) and a stricter SLO.

There is no scheduling abstraction worth building for both at once. The
right move is to **ship the digest as a focused job**, and when #625
follows it, lift the conditional-UPDATE claim primitive into a shared
helper if a third polling job has appeared by then. Premature abstraction
across two jobs would lock both into the simpler one's shape. Worth
listing #625 in *Related Issues* so the connection isn't lost, but it
should not block or expand this plan's scope.

## Open Questions

- **What timezone defines "a day"?** — Recommendation: `America/Denver`
  (Mountain Time, with DST), configurable via a `--digest-timezone`
  flag. Reasoning: explicitly requested by the user; the digest is for
  ops in Mountain Time. `America/Denver` is the correct IANA name for
  observing DST; `America/Phoenix` (Arizona, no DST) is a foot-gun if
  someone copy-pastes the wrong one. Persist the configured TZ in
  code, not in DB.
- **What hour of the day fires the send?** — Recommendation: 08:00 local,
  configurable via `--digest-send-hour` (0-23). Reasoning: ops should see
  it with their morning coffee, not at 03:00 UTC.
- **Which event types appear, and how?** — Recommendation: three buckets:
  (1) **Headlines** — community-level lifecycle (CREATED, DELETED, RESTORED,
  OWNERSHIP_TRANSFERRED, MEMBER_LEFT, MEMBER_REJOINED_WITHIN_WINDOW), gear
  flow (GEAR_SHARED, transfer state transitions, request lifecycle,
  experience CREATED/COMPLETED/CANCELLED). Rendered inline with actor/object
  names. (2) **Rolled-up counts** — RSVP churn (YES/MAYBE/NO),
  planning need/contribution add/remove/claim, EXPERIENCE_UPDATED. Shown as
  a single per-community line: "12 RSVPs, 4 planning updates." (3)
  **Excluded** — all `_UNDONE` events; they're noise in a daily-summary
  context (the digest is "what happened" not "what was reversed"). If an
  action and its undo both fall in the same window, the original still
  appears — that's accurate.
- **What chat signals appear, and how?** — Recommendation: per-community
  rollup line of the form *"42 chat messages from 11 members in 6
  conversations."* Skip the message content entirely — privacy and
  noise both argue against it. Distinct senders and distinct
  conversations are cheap to compute alongside the count and far more
  informative than the raw message total. System messages
  (`system_message` oneof) are skipped because every system message
  is already mirrored by a `CommunityEvent` and would double-count.
  Soft-deleted messages (`deleted IS NOT NULL`) are skipped.
- **What authentication signals appear, and how?** — Recommendation:
  the email opens with an "Engagement" section above the per-community
  sections, with four lines:
    - *"3 new users today: 1 Google, 1 Apple, 1 phone."* — count of
      `user` rows whose `created_at` falls in the window, broken
      down by `auth_method`. Names linked to their primary community
      (if any) so ops can see who they're talking about.
    - *"17 returning users signed in today."* — distinct `user_id`
      with at least one `refresh_token.created_at_unix_sec` in the
      window, minus the user_ids who appear in the new-users count
      (so first-login-of-a-new-signup is reported once, as a new
      signup).
    - *"2 new waitlist signups."* — count of `waitlist_entry` rows
      created in the window, with a single line of detail per
      signup (email + intended use, if present).
    - *"1 password reset requested."* — count of
      `pending_password_reset` rows created in the window. Omit
      this line entirely when the count is zero so the section
      doesn't clutter on quiet days.
  Exclude soft-deleted users from the new-signup count.
- **Order communities in the email how?** — Recommendation: by combined
  activity descending — sum of (CommunityEvent headline count × 2) +
  user-message count + rollup count. The `×2` weight on headline events
  keeps a community with three transfers ahead of one with thirty RSVP
  toggles. Then by community name. Top-of-email lists the 5 most active
  communities; the rest follow.
- **Zero-activity day handling?** — Recommendation: still send the email,
  with a single line "No activity in any community in the last 24 hours."
  Reasoning: silence indistinguishable from a job failure; a daily heartbeat
  is more valuable than silence. The Engagement section ships even on
  a zero-activity day — if literally nothing happened, the email is one
  Engagement line ("No new signups, no logins.") plus the per-community
  silence line. The next time a digest *does* contain activity, ops
  trusts that the previous day's silence meant zero, not bug.
- **Singleton idempotency table shape?** — Recommendation: new table
  `daily_digest_state` with one row, columns `id` (UUID, fixed to a
  sentinel constant), `last_sent_date` (DATE, indexed/PK-ish), updated via
  `UPDATE ... WHERE last_sent_date < $today`. Affected-rows == 1 means we
  own the send for today; == 0 means someone else got it. Same pattern as
  `purge_reminder_sent_at_unix_sec` but at the singleton level instead of
  per-community.
- **Manual on-demand send for testing?** — Recommendation: add a
  **dev-only** admin RPC `SendActivityDigestNow(date)`. Gating follows
  the existing pattern in `server/services/admin/cleanup.go` —
  early-return `connect.CodePermissionDenied` when `!s.devMode`,
  because the admin service has no role-based check today. Bypasses
  the claim guard. For prod re-sends: flip the schedule flag, or
  wait for the next scheduled fire. If/when the activity digest
  migrates to Cloud Scheduler (see scheduling-mechanism question
  below), prod also gets a "run now" button in the GCP console.
- **In-process ticker vs Cloud Scheduler?** — Recommendation: keep
  the in-process ticker for v1. `docs/server/observability.md`
  Part 3 architected its (separate, health-focused) daily digest
  around Cloud Scheduler, but neither that work nor its Terraform
  module have shipped yet. Shipping this one with the in-process
  ticker matches the existing job pattern in the codebase and
  avoids a Terraform module and service-account on the path to
  prod. If/when the observability digest does adopt Cloud Scheduler,
  migrate this one in the same PR so `main.go` does not end up with
  two competing patterns side-by-side.
- **Backfill window?** — Recommendation: don't backfill on first deploy.
  The job's first scheduled fire will send "yesterday." Older history is
  available in Metabase if anyone needs it. Avoids confusing first-run
  blasts.
- **Recipient lifecycle?** — Recommendation: single flag
  `--activity-digest-notify-email` (default `ops@ripls.org`, set to
  `user2@example.org` on dev). Multi-recipient is a Mailgun-side mailing
  list concern — `ops@ripls.org` itself can already alias to a group; no
  reason to build that into the server.

## Implementation Plan

### Phase 1: Email primitive + template

**Goal:** Be able to render and send the activity digest email in
isolation. No scheduling, no claim, no aggregation — just
`email.Service.SendActivityDigest` with a fully-populated input struct
covering all three sections (engagement, community activity, chat).

> Naming note: the method is `SendActivityDigest`, the input struct is
> `email.ActivityDigestInput`, and the templates are
> `activity_digest_template.html|txt`. The name "daily digest" is
> reserved by the planned health/error/traffic digest in
> `docs/server/observability.md` Part 3.

**Tasks:**
- [ ] Define `email.ActivityDigestInput` struct:
  - Window: `WindowStartUnixSec`, `WindowEndUnixSec`,
    `TimezoneName` (label), `GeneratedAtUnixSec`.
  - Engagement: `NewUsers []DigestNewUser` (with `Name`, `Email`,
    `AuthMethod`, `PrimaryCommunityName`), `NewUserBreakdown
    map[AuthMethodLabel]int`, `ReturningSignInCount int`,
    `WaitlistSignups []DigestWaitlistSignup` (email + intended use),
    `PasswordResetCount int`. These fields surface raw emails and
    names *in the email body* because that's the point — ops needs
    to know who just signed up. They must still be masked when
    written to logs (see Phase 3 logging tasks).
  - Communities: `Communities []DigestCommunity` with `Name`,
    `HeadlineLines []string` (rendered event narratives),
    `RolledUpSummary string` (event-type rollup),
    `ChatSummary string` (message + sender + conversation counts),
    `TotalActivityScore int` (used for ordering).
  - Totals: `TotalCommunityEventCount`, `TotalUserMessageCount`.
- [ ] Add `SendActivityDigest(ctx, toEmail string, in
  ActivityDigestInput) error` to `email.Service` interface.
- [ ] Implement on `MailgunService`: parse two new embedded templates,
  execute, send. Match the logging + masking pattern used by the existing
  three email types (`logging.MaskEmail` on `toEmail` in every log line;
  the email body itself keeps the unmasked values).
- [ ] Implement on `MockEmailService`: record into a new
  `ActivityDigests []RecordedActivityDigest` slice; signal `OnEmailSent`.
- [ ] Create `activity_digest_template.html` and
  `activity_digest_template.txt` with sections in this order: Header
  (window + timezone) → Engagement (signups, returning, waitlist,
  resets) → Top communities (top 5 with full detail) → Other communities
  (one-line summary each) → Footer (totals). Plain prose, no unicode
  tricks; designed for Apple Mail / Gmail web reading.
- [ ] Unit tests in `email/mailgun_test.go` exercising the templates with
  fixtures covering: zero-activity (all sections empty), engagement-only
  (signups but no community events), single-community-single-event,
  many-communities-with-chat-rollup, mixed auth-method breakdown.
  Use a recording mailgun stub or assert on the rendered template output.

**Validates:** The email looks good, renders both HTML and text correctly,
all three sections render even when their inputs are empty, and the
interface change compiles across mock + real implementations.

### Phase 2: Aggregator + storage helpers + idempotency

**Goal:** Be able to call a single function `Builder.Build(ctx, date,
tz)` from anywhere and get back an `email.ActivityDigestInput`
(engagement + communities + chat) ready to hand to
`SendActivityDigest`. Add the claim primitive so two server instances
can't both send.

**Tasks:**

Storage helpers (all in `server/storage/`, raw `QueryContext` pattern):
- [ ] `FindEventsInWindow(ctx, startUnixSec, endUnixSec int64) (
  []*models.CommunityEvent, error)` — ordered by `occurred_at_unix_sec
  ASC`.
- [ ] `CountUserMessagesByCommunity(ctx, startUnixSec, endUnixSec
  int64) ([]ChatActivityRow, error)` where `ChatActivityRow` is
  `{CommunityID, MessageCount, DistinctSenderCount,
  DistinctConversationCount}`. Single GROUP BY query joining
  `chat_message` to `chat_conversation` on `conversation_id`,
  filtered to `user_message IS NOT NULL AND deleted IS NULL AND
  sent_at_unix_sec >= $1 AND sent_at_unix_sec < $2`. Group by
  `chat_conversation.community_id`.
- [ ] `FindUsersCreatedInWindow(ctx, startUnixSec, endUnixSec
  int64) ([]*models.User, error)` — filter on `created_at`,
  exclude soft-deleted.
- [ ] `CountDistinctSignInsInWindow(ctx, startUnixSec, endUnixSec
  int64, excludeUserIDs []string) (int, error)` — `SELECT COUNT(DISTINCT
  user_id) FROM refresh_token WHERE created_at_unix_sec
  >= $1 AND created_at_unix_sec < $2 AND user_id != ALL($3)`.
  The exclusion list is the new-signup user IDs so a first login
  isn't double-counted.
- [ ] `FindWaitlistSignupsInWindow(ctx, startUnixSec, endUnixSec
  int64) ([]*models.WaitlistEntry, error)`.
- [ ] `CountPasswordResetsInWindow(ctx, startUnixSec, endUnixSec
  int64) (int, error)`.

Indexes (in `protosql_schema.go`):
- [ ] `CREATE INDEX IF NOT EXISTS idx_community_event_occurred ON
  "community_event" (occurred_at_unix_sec DESC)`.
- [ ] `CREATE INDEX IF NOT EXISTS idx_chat_message_sent ON
  "chat_message" (sent_at_unix_sec DESC)`.
- [ ] `CREATE INDEX IF NOT EXISTS idx_user_created_at ON "user"
  (created_at DESC)`.
- [ ] `CREATE INDEX IF NOT EXISTS idx_refresh_token_created ON
  "refresh_token" (created_at_unix_sec DESC, user_id)`.
- [ ] `CREATE INDEX IF NOT EXISTS idx_waitlist_entry_created ON
  "waitlist_entry" (created_at_unix_sec DESC)`.
- [ ] `CREATE INDEX IF NOT EXISTS idx_pending_password_reset_created
  ON "pending_password_reset" (created_at_unix_sec DESC)`.

Builder (new package `server/activity_digest/` with a `README.md`):
- [ ] `Builder.Build(ctx, dateInTZ time.Time)
  (email.ActivityDigestInput, error)` — converts the local date to a
  UTC `[start, end)` window, fans out to all six storage helpers in
  parallel (via `errgroup`), batch-resolves users via
  `services.FetchAPIUsersBatch` (the shared helper at
  `server/services/` package level — not a service handle, so the
  "no service-to-service imports" rule is preserved),
  batch-resolves gear via `storage.GetByIDs[*models.Gear]`,
  batch-resolves community names via a single
  `storage.GetByIDs[*models.Community]`, groups community events by
  community, merges per-community chat counts onto the same
  community rollup, formats per-event lines, computes the activity
  score, sorts top-5 vs. rest, builds the engagement section,
  returns `email.ActivityDigestInput`.

Idempotency:
- [ ] Add singleton state table `activity_digest_state` to
  `protosql_schema.go` (manual table, not a proto-backed one — it's
  ops state, not a domain entity) plus `ClaimActivityDigestSend(ctx,
  runDate time.Time) (bool, error)` in `server/storage/`. Pattern
  mirrors `ClaimCommunityPurgeReminder`: conditional `UPDATE ...
  WHERE last_sent_date < $today`, affected-rows == 1 means caller
  owns today's send.

Tests:
- [ ] Unit tests in `server/activity_digest/` covering: empty window,
  community-events-only, chat-only, engagement-only, all-three-combined,
  multi-community ordering by combined activity score, headline rollup
  computation, chat-system-message exclusion, soft-deleted-chat-message
  exclusion, soft-deleted-user exclusion from new signups,
  returning-signin de-duplication against new signups,
  former-member placeholder rendering, missing-gear graceful skip,
  UNDONE events excluded. Use `AssertMaxQueries` to prove the
  aggregator is bounded — target: ≤ 9 queries regardless of dataset
  size (6 source queries + community batch + user batch + gear batch).
- [ ] Unit tests for `ClaimActivityDigestSend`: first call wins,
  second same-day call loses, next-day call wins again.

**Validates:** End-to-end "feed it a window, get an email-ready struct"
works for all three input streams, is idempotent under concurrent runs,
and stays within a fixed query budget. No live emails sent yet.

### Phase 3: Schedule + wire-up + admin escape hatch

**Goal:** The job runs in production on a daily cadence to the right
recipient, with a dev-only manual re-send tool available for template
iteration.

**Tasks:**
- [ ] Create `server/jobs/activity_digest.go` with
  `ActivityDigestJob{storage, emailService, builder, recipient, tz,
  sendHour}`. `Run(ctx)` computes "yesterday in tz," attempts the
  claim, calls the builder, sends. Error handling: claim fail =
  debug log, build fail = error + return, send fail = error and
  **do not** reset the claim. Reasoning: a send failure is most
  likely transient at Mailgun's end, and resetting the claim creates
  a retry storm under instance flapping. Document the choice in the
  job's doc comment.
- [ ] Wire the job into `main.go` next to the existing
  `CommunityPurgeReminderJob` block via
  `logging.GoSafe(ctx, "activity-digest", ...)` to comply with the
  goroutine-safety rule. Hourly tick (not 24h tick): the job
  internally checks "is it currently the configured send-hour?" so a
  deploy at 09:00 doesn't push the next send 24h out. Hourly + claim
  guarantees one send per local day regardless of restarts.
- [ ] Add config flags: `--activity-digest-notify-email` (default
  `ops@ripls.org`), `--activity-digest-timezone` (default
  `America/Denver`), `--activity-digest-send-hour` (default 8),
  `--activity-digest-enabled` (default `false` initially — flip to
  true via a follow-up commit once we've validated dev output).
- [ ] Add **dev-only** admin RPC `SendActivityDigestNow(date string)`
  in `server/services/admin/`. Gate matches the existing pattern in
  `cleanup.go`: early-return `connect.CodePermissionDenied` when
  `!s.devMode`. Bypasses the claim and calls the builder + emailer
  directly. Any internal error path returns via
  `connecterr.Internal(ctx, "SendActivityDigestNow", err, kv...)` per
  the `server/connecterr/` convention — bare
  `connect.NewError(connect.CodeInternal, err)` is CI-gated
  (`scripts/check_codeinternal.js`). Add a one-line plain-text
  response with "sent to <masked recipient>".
- [ ] Structured-logging field set for both the job and the RPC,
  per `docs/server/observability.md` conventions:
  - Always: `operation` (`"ActivityDigest"` for the job,
    `"SendActivityDigestNow"` for the RPC),
    `window_start_unix_sec`, `window_end_unix_sec`, `timezone`,
    `recipient_email` (run through `logging.MaskEmail`),
    `duration_ms`.
  - On success: `new_user_count`, `returning_signin_count`,
    `waitlist_signup_count`, `password_reset_count`,
    `community_count`, `total_community_event_count`,
    `total_user_message_count`.
  - On failure: `error`, plus whichever counts were already known.
  - Use `logging.LoggerWithContext(ctx)` for the request-scoped
    logger. Do not redefine `request_id` (reserved for HTTP
    correlation).
- [ ] Integration test in `server/integration_tests/`: seed
  community events, chat messages (both user and system, plus a
  soft-deleted one), new users via three different auth methods,
  refresh tokens, and a waitlist signup across two communities and
  one day; run the job; assert the mock email service received
  exactly one email and the recorded `ActivityDigestInput` matches
  expected counts; assert a second `Run` is a no-op via the claim.
- [ ] Deploy to dev with
  `--activity-digest-notify-email=user2@example.org` and
  `--activity-digest-enabled=true`. Verify one full day of real dev
  activity renders correctly. Iterate on template prose without the
  production recipient set.
- [ ] Flip prod on once the dev output looks right: set
  `--activity-digest-notify-email=ops@ripls.org` and
  `--activity-digest-enabled=true` in
  `terraform/environments/prod/main.tf`.

**Validates:** The full system runs daily, hits the right inbox, and
recovers cleanly across restarts/deploys.

## Related Issues

- **#625** — "Scheduled notifications (e.g. timed before an event)."
  Shares the *background-job-with-claim-idempotency* pattern but has a
  fundamentally different trigger model (entity-relative, not calendar-
  relative) and recipient model (per-user push, not ops email). See
  [Relationship to #625](#relationship-to-625) above. Shipping the
  digest first does not constrain #625; both should use the same jobs
  pattern that already exists, and we can extract a shared claim helper
  only when a third caller arrives.
- **#1659** — "Day-before-purge reminder." Exact same job/claim pattern
  this plan reuses (`CommunityPurgeReminderJob`,
  `ClaimCommunityPurgeReminder`). Worth reading
  `docs/issues/1659-day-before-purge-reminder.md` before implementing
  for the precedent on log fields, naming, and test layout.
- **#1072** — "Add Metabase analytics dashboards on Postgres." Surfaced
  the same need for cross-community visibility but solves the
  ad-hoc-exploration half; the digest covers the push-to-inbox half.
  The two are complementary, not overlapping.
