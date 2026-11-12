# Issue #2274: Post for help notification

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2274
**Filed:** 2026-06-01 | **Last activity:** 2026-06-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:**
- `docs/workflows/request.md` (request state machine + RPCs)
- `docs/server/scheduled_notifications.md` (reconciler/dispatcher framework)
- `docs/push_notifications.md` (push payload / FCM routing)
- `docs/server/architecture.md` ⚓
- `docs/server/conventions.md` ⚓
- `docs/server/observability.md` ⚓
- `docs/client/architecture.md` ⚓
- `docs/client/conventions.md` ⚓
- `docs/client/i18n.md` (ARB parity)
- `docs/proto_conventions.md` ⚓
- `docs/users/priorities.md` ⚓ (latent host, no personal ledger, AI does the chasing)
- `docs/issues/625-scheduled-notifications.md` (history for adding a new scheduled-notif kind)
- `docs/reviews/api_design_review_prompt.md` ⚓, `security_review_prompt.md` ⚓, `observability_review_prompt.md` ⚓, `error_handling_review_prompt.md` ⚓, `performance_review_prompt.md` ⚓, `testing_review_prompt.md` ⚓, `accessibility_review_prompt.md` ⚓

> **Refinement (2026-06-10, maintainer):** Drop in-notification Yes/No
> action buttons and the `SnoozeRequestFollowupPrompt` RPC. The push
> should just deep-link to the request detail screen where the
> existing Mark Fulfilled / Cancel action bar already lets the
> requester resolve it. Phase 4 removed; related open questions and
> audit notes pruned. Cadence and recurrence unchanged.

> **Audit (2026-06-10):** Plan reviewed against the docs above. Fixes
> applied inline: (1) ARB strings now go to both `app_en.arb` and
> `app_es.arb` per parity gate; (2) reconciler logger uses
> `target_request_id` (not `request_id`) per server logging
> conventions; (3) preference gate in `Render` is a direct
> `prefs.NotifyRequestFollowupPrompts` check, not via
> `CategoryFor`/`CategoryEnabled` (clarified Phase 1); (4) reconciler
> batches the per-request `CommunityRequest` lookup with
> `QueryByFieldIn` to avoid N+1, with an
> `storage.AssertMaxQueries` test; (5) cycling-reminder backlog cutoff
> spelled out explicitly; (6) quiet-hours timezone source: empty
> `User.preferred_timezone` falls back to "quiet hours off" per
> framework convention.

## Problem Statement

A requester (the "Brian" in the issue) posts asking the community for items
or help. The community responds quickly and showers them in offers. The
requester gets what they need but, in the rush of life, forgets to come
back and mark the request fulfilled. Other community members see the
post still open, think Brian is still in need, and reach out — wasted
effort on both sides, and a slow leak of trust ("did anyone actually
help?"). The fix is for the system to proactively check in with the
requester after some time and surface a one-tap path back into the
request so they can mark it fulfilled (or cancel) without hunting
through the feed.

The maintainer comment (maintainer, 2026-06-09) settled the cadence:
**ping every three days** until the request leaves `ACTIVE` /
`OFFERS_RECEIVED`. The maintainer's follow-up (maintainer,
2026-06-10) settled the UX: **a push that deep-links to the request
detail screen**, where the existing Mark Fulfilled / Cancel action bar
already lets the requester resolve it — no in-notification action
buttons, no snoozing.

This is a momentum / proactive-system feature in the spirit of the
priorities doc — *"AI does the writing; the system handles most of the
chasing"* — and the system already has the right plumbing: the
scheduled-notification reconciler/dispatcher pair built for #625.

## Context from Comments

There are four relevant comments:

1. **2026-06-09 — maintainer (collaborator):** "We should implement
   this by pinging the requester every three days to check whether
   their request is still needed and hasn't been otherwise fulfilled."
   → cadence settled at **3 days**, recurring until the request is no
   longer open.
2. **2026-06-09 / 2026-06-10 — maintainer:** `@claude` (kicked off
   planning; re-triggered after a workflow issue).
3. **2026-06-10 — maintainer (on PR #2430):** "we don't need
   notification actions or snoozing capability — just deep link to
   the request when it remains open." → scope narrowed to push +
   deep-link only.

The reporter's original body asked for Yes/No buttons on the push, but
the maintainer's PR comment overrides that — v1 is push + deep-link.
The deep-link target already contains the Mark Fulfilled / Cancel
action bar (`docs/workflows/request.md` §UI Elements), so the
requester still gets a one-tap resolution path; they just take it
inside the app instead of from the system shade.

No remaining disagreement. The decisions to settle are the
implementation choices below, not the product intent.

## Potential Solutions

### Option A (recommended): Extend the scheduled-notification framework with a new `RequestNotification` item-variant; ship copy + deep-link to existing detail screen

- **Approach:**
  - **Schema** — add `RequestNotification { request_id, purpose,
    offset_seconds_from_anchor }` and an
    `EXPERIENCE_NOTIFICATION_PURPOSE_*`-style enum
    `RequestNotificationPurpose` with one value:
    `REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT`. Add it as a third
    variant of the `oneof item` in
    `proto/ripls/models/scheduled_notification.proto`. Extend
    `UniquenessKey` in
    `server/jobs/scheduled_notifications/framework.go` to handle the
    new variant.
  - **Storage finder** — add
    `FindRequestScheduledNotifications(ctx)` to
    `server/storage/scheduled_notification.go`, discriminator column
    `request_request_id`.
  - **Reconciler + dispatcher** — new file
    `server/jobs/scheduled_notifications/request_followup_prompts.go`
    implementing both. **Anchor = `Request.created_at_unix_sec`** (so
    a clean public anchor; see Open Questions for why not "last activity"). The reconciler emits one row per
    open request to the **requester** at offsets `+3d, +6d, +9d, +12d,
    +15d`. Use the existing `loanBacklogGrace`-style `maxOffset +
    grace` cutoff so we never cycle on ancient requests. Reconciler
    skips rows once the request leaves `ACTIVE` / `OFFERS_RECEIVED`
    (re-checked at dispatch render too so a fulfillment between
    reconcile and dispatch suppresses the push). `QuietHoursPolicy =
    Defer` (the prompt is day-grained; nothing breaks if it slides to
    07:00 local).
  - **Preference gate** — new toggle on
    `CommunityNotificationPreferences`:
    `notify_request_followup_prompts` (unset = on). Lives on the
    per-community surface because the requester engages with a request
    in the context of a community feed; mirrors `notify_new_requests`.
  - **Push payload** — render via `Notification_CommunityEvent` with
    `EventType = "REQUEST_FOLLOWUP_PROMPT"`,
    `request_id = request.Id`, `community_id` set from the
    first-shared community (the row carries `community_id` for the
    pref gate and deep-link; cross-community requests get one prompt,
    not one per community).
  - **Client deep-link** — extend `routeNotificationPayload` in
    `app/lib/services/fcm_service.dart` so the new `EventType` routes
    to the request detail screen, scrolled to the
    "Mark Fulfilled / Cancel" action bar (already present per
    `docs/workflows/request.md` §UI Elements). The requester resolves
    the request from inside the app via the existing fulfillment UI.
- **Pros:**
  - Reuses the entire scheduled-notification machinery (delete-on-fire,
    re-fire protection, quiet hours, atomic claim). Adding a third
    item variant is the system's documented extension point —
    `docs/server/scheduled_notifications.md` literally has a
    *"Adding a new scheduled-notification kind"* section.
  - Per-community pref toggle slots into the existing notification-prefs
    screen with one line.
  - Honors the priorities doc: "the system handles the chasing."
  - No platform-specific FCM action-button work, no new RPC — the
    push uses the normal `notification` payload path and the deep-link
    routing the codebase already supports.
- **Cons:**
  - Recurring/cycling reminders are a new pattern for this framework
    — existing kinds are one-shot per offset. Have to be careful that
    the reconciler doesn't keep extending offsets forever (the
    backlog-grace cutoff handles this; document explicitly).
- **Effort:** Small-medium.

### Option B: One-shot 3-day prompt only (no recurrence)

- **Approach:** Same as Option A, but emit only the single
  `+3d` offset. If the requester ignores the prompt, no further pings.
- **Pros:**
  - Simplest possible. No "cycling reminder" concern. Smallest blast
    radius if something is wrong.
- **Cons:**
  - Doesn't match the maintainer's stated cadence ("every three days").
  - Doesn't solve the underlying problem — the same distracted host who
    didn't return to the app on day 3 won't return on day 4 either.
- **Effort:** Small.

### Option C: In-feed nudge card instead of push

- **Approach:** No push. Instead, the requester sees a feed-card prompt
  next time they open the app, asking "Still need '[title]'?" with
  Yes/No buttons. Implemented as a new
  `StoredNudge` shape (the workshop system already emits postcard-style
  nudges).
- **Pros:**
  - Avoids the scheduled-notification infra entirely.
  - Stays inside the existing nudge / postcard surface that the latent
    host is already trained to engage with.
- **Cons:**
  - Doesn't reach the requester if they don't open the app — which is
    exactly the failure mode in the original story.
- **Effort:** Small-medium.

## Recommendation

**Option A: recurring 3-day push that deep-links to the request detail
screen.** No in-notification action buttons, no snooze RPC — the
requester resolves the request from inside the app via the existing
Mark Fulfilled / Cancel action bar.

Why:

- It satisfies the maintainer's stated cadence (every 3 days while the
  request remains open) and the maintainer's UX direction (deep-link
  only, no notification actions).
- It re-uses the existing reconciler/dispatcher framework — that
  framework was designed for exactly this kind of "anchored, recurring,
  state-aware" reminder, and the design doc has a step-by-step "add a
  new kind" guide.
- Dropping the action-button and snooze surfaces removes the
  platform-specific FCM payload work, the new RPC + ownership check,
  and the iOS `UNNotificationCategory` registration — none of which
  the maintainer wants in v1.

We pay a small complexity cost on the framework (cycling reminders are
a new pattern for it), bounded by the `requestBacklogGrace` cutoff.

## Open Questions

- **What's the right anchor — `created_at_unix_sec`, or "last
  activity" (last message in the conversation, last offer made)?**
  Recommendation: **`created_at_unix_sec`**. "Last activity"
  sounds smarter but is much harder to implement deterministically
  inside the reconciler (it has to inspect conversation rows and offer
  rows per request per tick) and produces surprising user behavior
  (one new offer resets the prompt timer, leaving the requester
  perpetually un-prompted). Pinned to creation, the cadence is exactly
  what the maintainer asked for: every 3 days from the post.
- **How many recurring offsets do we emit before we stop?**
  Recommendation: **5 (i.e. up to +15 days from creation)**. After
  ~15 days an unsatisfied request has almost certainly stalled out;
  continuing to ping the requester past that becomes nagging, which
  the priorities doc explicitly warns against (no personal ledger /
  no shaming). The limit is one line in the offset slice and trivially
  tunable.
- **Per-community prefs vs per-user prefs for the toggle?**
  Recommendation: **per-community** (`CommunityNotificationPreferences.
  notify_request_followup_prompts`). The matching event
  (`notify_request_updates`) is per-community; the toggle is about
  "things this community pesters me about." This keeps the
  notification-prefs surface coherent. Disable by adding the toggle to
  the existing community prefs row and re-using `community.CategoryEnabled`.
- **Cross-community requests — one prompt per request, or one per
  community-shared?** Recommendation: **one per request, total**.
  The `Request` is one entity even when shared; pinging the requester
  N times for N communities would be annoying and is inconsistent with
  `REQUEST_FULFILLED` deduplication (#2088). Achieve this by keying
  the reconciler's uniqueness off `(recipient_user_id, request_id,
  purpose, offset)` — community context lives only on the row's
  `CommunityId` (used for pref-gate and deep-link), set to the request's
  first non-archived `CommunityRequest.CommunityId` at reconcile time.
- **Story / community-event emission?** Recommendation: **No**. This
  is a private, requester-only prompt — it should not surface in the
  community feed or stories. The push goes out without
  `recordCommunityEventForRequest` ever being called. Mirrors the
  loan-return-reminder / experience-close-prompt pattern.
- **Localization?** Recommendation: defer past v1. The existing
  experience close-prompt copy is hard-coded English in
  `experience_reminders.go`; matching that posture keeps the diff
  small. Localization can come along with the broader scheduled-
  notification i18n work tracked on #1659.

## Implementation Plan

Each phase is a shippable, independently landable PR.

### Phase 1: Schema + storage finder

**Goal:** New `RequestNotification` item variant exists end-to-end in
the proto/storage surface, with no behavior change yet.

**Tasks:**
- [ ] Add `RequestNotificationPurpose` enum (one value:
      `REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT`) and
      `RequestNotification { request_id, purpose,
      offset_seconds_from_anchor }` to
      `proto/ripls/models/scheduled_notification.proto`. Add as a
      third `oneof item` variant.
- [ ] Add `notify_request_followup_prompts` (`optional bool`, unset = on)
      to `CommunityNotificationPreferences` in
      `proto/ripls/models/community.proto`. Pick the next free field
      number after `notify_event_reminders = 17`.
- [ ] Run `npm run generate:proto` (verify Go + Dart generated code).
- [ ] In `server/jobs/scheduled_notifications/framework.go` extend
      `UniquenessKey` with a `ScheduledNotification_Request` switch
      arm — `fmt.Sprintf("u=%s|t=req|id=%s|p=%d|o=%d", …)`.
- [ ] In `server/storage/scheduled_notification.go` add
      `FindRequestScheduledNotifications(ctx)`, discriminator column
      `request_request_id`. Add a unit test that round-trips a
      request-variant row.
- [ ] **Preference-gate wiring.** Two options here, and the existing
      framework uses both for different surfaces:
  - **Direct check in `Render`** (recommended). The
    `ExperienceClosePromptDispatcher` reads `prefs.NotifyEventClosePrompts`
    directly without going through `CategoryFor`/`CategoryEnabled`
    (`server/jobs/scheduled_notifications/experience_reminders.go:524`).
    Follow that pattern: in Phase 2 the dispatcher's `Render` calls a
    new `world.GetCommunityNotificationPreferences(ctx, userID,
    communityID)` and gates on `prefs.NotifyRequestFollowupPrompts`
    being unset or true. No changes to
    `server/community/notifications.go` are needed.
  - Adding a `NotificationCategory` enum value is **only**
    required if we want the unified Manage Notifications
    "categories" surface to gate this prompt for whole categories of
    events at once — which we don't (the per-community toggle is
    standalone). Skip the enum addition.
- [ ] Tests: `framework_test.go` — uniqueness-key for the new variant;
      `scheduled_notification_test.go` — round-trip insert/find/delete
      for a request-variant row.

**Validates:** Code generation works; the new item variant participates
in storage/diff plumbing with no behavior change.

### Phase 2: Reconciler + dispatcher (push + deep-link)

**Goal:** Open requests trigger a real push every 3 days to the
requester, deep-linking to the request detail screen where the
existing Mark Fulfilled / Cancel action bar lives.

**Tasks:**
- [ ] Add `server/jobs/scheduled_notifications/request_followup_prompts.go`
      with:
  - Offsets slice: `[3d, 6d, 9d, 12d, 15d]` (in seconds).
  - `RequestWorld` interface (`ListOpenRequests`, `GetRequest`,
    `GetPrimaryCommunityIDsForRequests`, `GetUserTimezone`,
    `GetCommunityNotificationPreferences`) wired against
    `ProtoSQLStorage`, mirroring `LoanWorld` and the
    `storageExperienceReader` patterns. **N+1 guard:**
    `GetPrimaryCommunityIDsForRequests` takes a batch of request IDs
    and returns a `map[requestID]communityID` in one
    `QueryByFieldIn`+aggregate over `CommunityRequest` (archived=false,
    earliest `shared_at`) — *not* a per-row `GetByID` loop. SQL
    efficiency rule per `docs/server/conventions.md` §SQL Efficiency;
    test guarded by `storage.AssertMaxQueries` so the bound stays
    constant as the open-request count grows.
  - `RequestFollowupReconciler` — emits one row per open request
    per offset, recipient is `requester_id`. Skips if state is
    `FULFILLED` / `CANCELLED` / soft-deleted. Drops tuples past
    `DesiredFireAtGrace` and past
    `created_at + maxOffset + requestBacklogGrace`. **The
    `requestBacklogGrace = 24h` cutoff is what prevents the
    "recurring reminder" pattern from cycling forever** — once
    `now > created_at + (15d) + 24h`, no more tuples for that
    request, and existing rows time out via the dispatcher's
    normal grace handling. Mirrors `loanBacklogGrace` in
    `loan_return_reminders.go`. Sets `CommunityId` from the
    batched lookup (one push per request, regardless of
    cross-community sharing — see Open Questions). Sets
    `QuietHoursTimezone` from the recipient's
    `User.preferred_timezone`; **empty string is fine** — the
    framework's `quiet_hours.go` treats unset tz as "quiet hours
    disabled for this row" (`docs/server/scheduled_notifications.md`
    §Quiet hours).
  - `RequestFollowupDispatcher` — `CanHandle` matches the
    request variant + `FOLLOWUP_PROMPT` purpose. `QuietHoursPolicy
    = Defer`. `Render` re-loads the request, no-ops if state is
    no longer open / soft-deleted / community soft-deleted, gates
    on `notify_request_followup_prompts` (direct read, no
    `CategoryFor`; preference-fetch errors **fail open** per
    framework convention), builds a `Notification_CommunityEvent`
    with `EventType = "REQUEST_FOLLOWUP_PROMPT"`, `request_id`,
    `community_id`, `request_title`. Title/body: `"Still need
    this?"` / `"You asked: '{request_title}' 3 days ago — did your
    community come through?"`. **No
    `recordCommunityEventForRequest` call** — the prompt is
    private to the requester (mirrors close prompt + loan-return
    reminder).
  - **Logger field names** — per `docs/server/conventions.md`
    §Structured Logging, `models.Request` IDs log as
    `target_request_id` (not `request_id`, which is reserved for
    the HTTP correlation ID). `operation =
    "ScheduledNotificationReconcile"` / `"ScheduledNotificationDispatch"`,
    matching framework convention.
- [ ] Wire into `server/main.go`'s `scheduledReconcilers` and
      `scheduledDispatchers` slices.
- [ ] Client: extend `routeNotificationPayload` in
      `app/lib/services/fcm_service.dart` to handle
      `EventType == "REQUEST_FOLLOWUP_PROMPT"` — route to the existing
      request-detail route (`requests/<request_id>`), opening the
      action bar / conversation bottom tab so the Mark Fulfilled /
      Cancel controls are immediately visible.
- [ ] Tests:
  - `request_followup_prompts_test.go` covering: three-state emission
    (future / recent-past / past-grace), state-change suppression
    (`FULFILLED` between reconcile and dispatch), preference gating
    (unset / explicit on / explicit off / fetch-error fails open),
    soft-delete suppression, multi-community → one row,
    past-`requestBacklogGrace` → no emission (anti-cycle invariant).
  - `storage.AssertMaxQueries`-guarded test in the same file proving
    the reconciler's per-tick query count does not scale with the
    number of open requests (catches N+1 regressions per
    `docs/server/conventions.md` §SQL Efficiency / `AssertMaxQueries`
    pattern).
  - Update `docs/workflows/request.md` §Notifications and the
    "workflow examples" Postconditions to mention the new push.
  - Update `docs/server/scheduled_notifications.md` policy table
    (new dispatcher row) and "per-item-type pattern" list. Add a
    "Cycling reminders" note so the recurring-offset pattern is
    documented for the next contributor.

**Validates:** A locally-run request that's left open for ≥3 days
produces a push that lands on the requester's device and opens the
right screen on tap.

### Phase 3: Per-community settings UI + e2e

**Goal:** Users can toggle the prompt per community; full client e2e.

**Tasks:**
- [ ] Add a tile to the per-community Manage Notifications screen
      (`app/lib/presentation/screens/communities/manage_notifications_screen.dart`
      or equivalent — confirm exact path during implementation).
      Reuse the existing `Toggle` accessibility primitive (with a
      meaningful `semanticsLabel` per
      `docs/reviews/accessibility_review_prompt.md` §Semantic Labels)
      and the `notify_request_updates` tile as a model. ARB keys
      `notifyRequestFollowupPromptsTitle` and
      `notifyRequestFollowupPromptsDescription` must be added to
      **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb`
      — `scripts/check_l10n_parity.js` (run as
      `npm run lint:dart:l10n-parity`) fails on missing keys. English
      placeholder in `app_es.arb` is acceptable v1 per
      `docs/client/i18n.md` (the gate is about parity, not translation
      quality — see #1972). Push-copy strings rendered server-side
      stay English in v1 per the close-prompt precedent; full
      localization tracked separately on #1659.
- [ ] Client viewmodel/repo plumbing: extend the existing
      community-notification-prefs Service → Repository → ViewModel
      chain to round-trip the new field.
- [ ] Playwright e2e test in
      `e2e/tests/workflows/request_followup_prompt.spec.ts` — toggle
      off, simulate a clock-advance via the dev clock, assert no push;
      toggle on, assert push lands and tapping it opens the request
      detail screen with the action bar visible.
- [ ] Update `docs/workflows/request.md` and
      `docs/push_notifications.md` ("Event Types that Trigger
      Notifications" table) with the new event.

**Validates:** End-to-end pref toggle works on a real device; localized
ARBs parity-pass; docs match code.

## Related Issues

- **#625** — Scheduled notifications framework. This plan piggybacks on
  that framework; we are following its documented "add a new kind"
  recipe.
- **#1659** — Push-copy localization. Phases 1–3 hard-code English copy;
  the i18n work for this dispatcher should land alongside the broader
  scheduled-notification i18n there.
- **#2088** — Cross-community fulfillment notification dedup. Same
  pattern (one push per request, not per community-shared) applies to
  the followup prompt; reuse the keying approach.
- **`workflows/request.md` Future Considerations #3 ("Request
  Expiration")** — this prompt is the soft, opt-out form of that idea.

## Audit notes (2026-06-10)

**Docs consulted:** `server/architecture.md` ⚓, `server/conventions.md` ⚓,
`server/observability.md` ⚓, `client/architecture.md` ⚓,
`client/conventions.md` ⚓, `client/i18n.md`, `proto_conventions.md` ⚓,
`users/priorities.md` ⚓; review prompts: `api_design`, `security`,
`error_handling`, `observability`, `performance`, `testing`,
`accessibility`, `code_quality`, `architecture`, `tech_debt`,
`dependency`; leaf docs: `server/scheduled_notifications.md`,
`push_notifications.md`, `workflows/request.md`,
`issues/625-scheduled-notifications.md`.

**Compliance checks that passed without change:**
- Reuses the `#625` scheduled-notification framework's documented
  "Adding a new scheduled-notification kind" recipe end-to-end.
- Push payload uses the existing `Notification_CommunityEvent`
  envelope with a string `EventType` — does **not** add a
  `CommunityEventType` enum value, so no `server/undo/registry.go`
  entry is required.
- Reconciler / dispatcher run inside the framework's existing
  `logging.GoSafe`-launched goroutines from `server/main.go`; no new
  raw `go func()` introduced.
- Plan does **not** emit a community event or story for the prompt
  itself, matching `LoanReturnReminderDispatcher` /
  `ExperienceClosePromptDispatcher` precedent — the prompt stays
  private to the requester.
- No new external dependencies and no new API surface — the push uses
  the existing FCM `notification` payload path; the deep-link is
  handled by the existing `routeNotificationPayload` switch.

**Caveats / explicit non-goals:**
- Push copy stays English in v1; localization rides on #1659.
- Cycling 5-offset reminder pattern is new to the framework but is
  bounded by `requestBacklogGrace` — the doc update in Phase 2
  should add a "Cycling reminders" note to
  `docs/server/scheduled_notifications.md` so the pattern is
  documented for the next contributor.
- **Out of scope per maintainer (2026-06-10):** in-notification Yes/No
  action buttons, `SnoozeRequestFollowupPrompt` RPC, `data`-only FCM
  payload work, and iOS `UNNotificationCategory` registration. The
  push deep-links to the existing in-app Mark Fulfilled / Cancel
  action bar instead.
