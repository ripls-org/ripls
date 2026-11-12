# Issue #1107: Make notifications configurable per-community

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1107
**Filed:** 2026-04-13 | **Last activity:** 2026-04-28
**Labels:** (none, P2 in triage project) | **Assignees:** maintainer
**Context docs:** none provided (relevant existing docs read: `docs/push_notifications.md`, `server/community/events.go`, `server/notifications/service.go`)

## Audit Findings (2026-04-28)

Audited against `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`, `docs/proto_conventions.md`,
and CLAUDE.md. Issues found and addressed below; corrections folded into
the relevant phases.

- **Proto convention gap (now fixed in Phase 1).** The plan only mentioned
  adding the storage model in `proto/ripls/models/community.proto`. Per
  `docs/proto_conventions.md` and CLAUDE.md, RPC input/output cannot be
  storage models — each RPC needs dedicated API messages. Phase 1 now
  explicitly adds the API-side messages (`Get…Request/Response`,
  `Update…Request/Response`) plus the conversion helpers. A round-trip
  test in `community_test.go` covers the conversion.
- **Error-handling rule (CLAUDE.md "Fail Fast", now in Phase 2).** Plan
  was silent on what happens if the per-recipient preference fetch
  fails. Default behavior is now documented: log a warning and treat
  preferences as fully on (fail-open), so a transient DB issue can't
  silently mute pushes for a whole community. Errors propagated to the
  caller would force the entire `notifyTargetedUsers` goroutine to
  abort, which is worse than fail-open here.
- **File-size rule (CLAUDE.md, now in Phase 1).** `server/community/events.go`
  is ~490 lines today. Adding `categoryFor`, batch-prefs lookups,
  recipient cases for the expanded coverage, and notification copy will
  push it past the 1000-line gate. Phase 1 splits notification logic
  into `server/community/notifications.go` (filter + recipient + copy)
  while leaving the event-recording bits in `events.go`.
- **Client architecture gap (now in Phase 5).** Plan said "service →
  viewmodel" and skipped the repository layer. Per
  `docs/client/architecture.md` rule "ViewModels are the ONLY layer
  that calls Repositories" and "Repositories are the ONLY layer that
  calls Services", a `CommunityNotificationsRepository` is required.
  It owns the cache namespace `notification_prefs` keyed by
  `community_id` and invalidates on update.
- **Async safety (now in Phase 5).** Notifier was unspecified; per
  `client_architecture.md` Async Safety section, the screen-scoped
  provider should be `AsyncNotifierProvider.autoDispose` so disposal
  during in-flight save is handled. Added a disposal test requirement.
- **i18n gap (now in Phase 5).** Plan referenced UI strings inline.
  Per `docs/client/i18n.md`, every user-visible string must come from
  `context.l10n` and have entries in both `app_en.arb` and
  `app_es.arb`. Phase 5 lists the ARB keys to add (`settingsManageNotifications*`).
- **NavigationHelpers + SwipeToCloseMixin (now in Phase 5).** Plan
  was silent. Per `client_architecture.md` "Navigation & Gestures",
  the new screen uses `NavigationHelpers.pushScreen` and applies
  `SwipeToCloseMixin` since it's a primary settings screen.
- **AppColors compliance (already implicit, now explicit in Phase 5).**
  The screen reuses `buildSettingsCard` / `buildSettingsActionItem`,
  which already use `AppColors.*(context)`. Added a note to ensure no
  new hardcoded `Colors.*` values land.

## Problem Statement

Today notifications are governed entirely by a hard-coded server-side
`shouldNotify()` switch in `server/community/events.go:94`. There is no way for
a member to opt in or out of a category, and there are no per-community
controls. Two problems result:

1. **Under-notification.** Early-community evidence shows members *want* to
   know about activity we currently suppress (e.g. `GEAR_SHARED`,
   `REQUEST_CREATED`, `EXPERIENCE_COMPLETED`, `TRANSFER_ACTIVE`,
   `REQUEST_OFFER_SELECTED/WITHDRAWN/CANCELLED`, planning updates). The
   `shouldNotify()` allowlist treats most of these as broadcast spam and
   never fires push.
2. **No member control.** Once we expand the set, the same content will be
   noise to a different cohort of members, and we have no way for them to
   tune it.

The fix has two halves: **expand the set of events that can produce
notifications**, and **make the actual delivery gated by per-community,
per-category preferences** that a member edits in
Settings → Communities → [Community] → Membership → "Manage Notifications".
Defaults must be all-on so the change is additive for opted-in users and
zero-config for new users.

The maintainer's 2026-04-28 comment (the issue author) is the authoritative
spec and supersedes the original description, which did not specify event
groupings or settings location.

## Context from Comments

Two comments, both from the issue author / project maintainer:

- **2026-04-25 triage:** Set priority P2 — "meaningful improvement for
  multiple personas". Mid-tier; not urgent but planned.
- **2026-04-28 design spec:** Locks in scope and grouping. Key decisions:
  - **Location:** Settings → Communities → [Community Settings page] →
    Membership section → new "Manage Notifications" entry. (This is the
    `GovernanceScreen._buildMembershipSection` in
    `app/lib/presentation/screens/governance/governance_screen.dart:162`.)
  - **Default = enabled** for every category.
  - **Per-community, not global.**
  - **Categories** (each is one toggle):
    - All new requests *(REQUEST_CREATED)*
    - All new experiences *(EXPERIENCE_CREATED)*
    - `GEAR_SHARED`
    - `EXPERIENCE_COMPLETED` — notify even users not involved
    - "Updates on loans and giveaways" *(transfers the user is a party to:
      `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`,
      `TRANSFER_ACTIVE`, `TRANSFER_CANCELLED`, `TRANSFER_PICKUP_PROPOSED`)*
    - "Updates on requests" *(requests the user is a party to:
      `REQUEST_OFFER_MADE`, `REQUEST_OFFER_SELECTED`,
      `REQUEST_OFFER_WITHDRAWN`, `REQUEST_FULFILLED`, `REQUEST_CANCELLED`)*
    - "Experience RSVPs" — for events the user said "yes" or "maybe" to
      *(this is the existing `notifyRSVPedUsers` path, not a community
      event type)*
    - "Event planning updates" — for events the user said "yes" or
      "maybe" to *(`PLANNING_NEED_ADDED`, `PLANNING_NEED_CLAIMED`,
      `PLANNING_CONTRIBUTION_ADDED`)*
    - **Chats in this community** *(single toggle gating
      `notifyParticipants` for any chat conversation whose
      `community_id` matches — added in review of the original spec)*

There is no debate or counter-proposal in the thread — the design is the
plan-of-record.

## Codebase Findings

- **Single emission funnel.** `community.RecordCommunityEventAndNotify`
  (`server/community/events.go:46`) is the entry point used by 23
  service-side call sites. The filter (`shouldNotify`,
  `server/community/events.go:94`) and recipient resolver
  (`getNotificationRecipients`, `server/community/events.go:113`) sit
  side-by-side here. Adding preference-aware filtering belongs at this
  funnel, not at each call site.
- **RSVP path is separate.** `notifyRSVPedUsers`
  (`server/services/experience/lifecycle.go:722`) bypasses the community-
  event funnel and uses a custom `Notification` payload. It needs its own
  pref check.
- **Chat path is separate.** `notifyParticipants`
  (`server/services/chat/notifications.go:14`) is for chat-message pushes.
  Every `ChatConversation` carries a `community_id`
  (`ChatMessagePayload.CommunityId`, `notifications.go:99`), so a single
  per-community "chats" toggle is a clean gate at this funnel. We add it
  to scope per the user's review of the spec.
- **Planning events are chat-system-only today.** `PLANNING_NEED_ADDED`,
  `PLANNING_NEED_CLAIMED`, `PLANNING_CONTRIBUTION_ADDED` exist as both
  `CommunityEventType` enum values (`proto/ripls/models/community.proto:231`)
  and `ChatSystemAction` (`server/chat/system_messages.go:49`), but
  `grep` shows they are emitted only as system messages in
  `server/planning/`, never via `RecordCommunityEventAndNotify`. To honor
  "Event planning updates" we have to start emitting community events for
  them.
- **Several event types in the spec aren't notified today.** `shouldNotify`
  excludes `GEAR_SHARED`, `REQUEST_CREATED`, `EXPERIENCE_COMPLETED`,
  `TRANSFER_ACTIVE`, `REQUEST_OFFER_SELECTED`, `REQUEST_OFFER_WITHDRAWN`,
  `REQUEST_CANCELLED`, `TRANSFER_INTEREST_WITHDRAWN`. The spec keeps
  `*_WITHDRAWN` and `*_CANCELLED` in scope under their group toggles, so
  most of these need both a recipient resolver case *and* a notification
  template (`buildNotification`, `server/community/events.go:328`).
- **No preference table exists.** `grep "NotificationPref"` is empty.
  `CommunityUser` (`proto/ripls/models/community.proto:51`) is the
  membership row; we should *not* widen it — preferences are an
  evolving shape and belong in a dedicated table.
- **`CommunityEventType` undo registry gate.** `CLAUDE.md` requires that
  every new `CommunityEventType` be classified in
  `server/undo/registry.go`. The spec doesn't introduce new event types,
  but if we promote planning events to community events we may need to
  wire their undo classification (likely `ClassificationClientOnly`
  since planning add/remove are already inverse pairs).

## Potential Solutions

### Option A: New per-(user, community) preferences row, server-side filter at the funnel

- **Approach.** Add a `CommunityNotificationPreferences` storage proto keyed
  by `(user_id, community_id)`. Add `GetCommunityNotificationPreferences`
  and `UpdateCommunityNotificationPreferences` RPCs to
  `community_service.proto`. In `notifyTargetedUsers`
  (`server/community/events.go:203`), look up preferences once per
  recipient before calling `NotifyUser`; same check inside
  `notifyRSVPedUsers`. Use a `Category` enum to map event types →
  category. Absence of a row is treated as "all enabled" so new users and
  unmigrated rows behave like today.
- **Pros.** Single chokepoint. Clean separation from `CommunityUser` so
  preferences can evolve (digest cadence, quiet hours) without touching
  membership semantics. Default-on with absent rows means zero migration
  cost. Aligns with the existing pattern used elsewhere
  (`UserDevice` is a separate row keyed by user_id).
- **Cons.** One extra DB read per recipient per event. For
  `EXPERIENCE_CREATED` and the new `EXPERIENCE_COMPLETED` /
  `GEAR_SHARED` / `REQUEST_CREATED` paths the recipient list is the full
  community, so this is N reads per event. Mitigate with batched
  `QueryByFieldIn` per the `CLAUDE.md` SQL-efficiency rules.
- **Effort.** Medium.

### Option B: Embed preferences as a JSON map on `CommunityUser`

- **Approach.** Add `map<string, bool> notification_categories` (or a
  proto message of bools) to `CommunityUser`. Read it during membership
  fetch, which is already on the read path for many recipient-resolver
  cases.
- **Pros.** Cheap reads — preference comes "free" with membership.
  Zero new tables.
- **Cons.** Couples the membership shape to a feature whose categories
  will keep evolving. Schema migrations on `CommunityUser` are
  higher-risk than on a feature-scoped table. Most recipient-resolver
  cases load entity rows (gear, transfer, request) rather than
  `CommunityUser`, so the "free read" advantage isn't real for the
  per-party paths — we'd add a fetch anyway.
- **Effort.** Small-medium, but worse long-term.

### Option C: Client-side filter (server still notifies, client suppresses)

- **Approach.** Server keeps notifying for everything. Client stores
  preferences locally and suppresses the FCM/APNs payload at the
  notification handler.
- **Pros.** No server work, no schema change.
- **Cons.** Wastes FCM/APNs quota on suppressed sends. Doesn't work on
  iOS for category-level filtering — the system has already shown the
  banner by the time Dart code runs in some delivery modes. Cross-device
  inconsistency. **Rejected.**
- **Effort.** Small but the wrong shape.

## Recommendation

**Option A.** It matches the existing notification-pipeline structure
(filter at the funnel, single decision point), gives clean evolution
room, and the per-recipient lookup is a tractable batch-fetch problem.
Default-on via absent rows is the right semantics for the
"all-on by default" requirement and avoids backfill.

The performance concern is real but manageable: for broadcast-style
events (`EXPERIENCE_CREATED`, `EXPERIENCE_COMPLETED`, `GEAR_SHARED`,
`REQUEST_CREATED`) we already fan out to the full member list, so adding
a single batched `QueryByFieldIn(user_id IN (...) AND community_id = ?)`
before the loop is one extra query, not N.

## Open Questions

- **Q1: Should `EXPERIENCE_COMPLETED` notify the entire community by
  default?** The spec says "even if user isn't involved", which is a
  significant noise increase versus today (we don't notify on it at
  all). — *Recommendation:* yes, default-on per spec, but ship Phase 5
  (the UI) before Phase 3 (the new emit sites) so users have an off
  switch the moment they see the change. Sequencing matters.
- **Q2: Do "Updates on requests" / "Updates on loans and giveaways"
  apply globally or only when the user is a party?** Spec says "for any
  transfer the user is involved in" / "for requests the user is involved
  in". — *Recommendation:* keep the existing `getNotificationRecipients`
  involvement-resolution logic; the toggle is a *gate* for those who
  would already receive, not an opt-in for non-parties.
- **Q3: Should we promote planning events to full community events, or
  keep them as chat-system-messages and notify off the chat-system
  hook?** — *Recommendation:* promote to community events. It keeps the
  funnel single, and the planning-system-message path already has the
  metadata. The chat-system-message stays for in-conversation rendering;
  the new community event triggers push.
- **Q4: Recipient set for planning events?** Spec says "events the user
  is involved in". — *Decision:* RSVP=yes/maybe only. Simpler to reason
  about, matches the "Experience RSVPs" recipient set, and avoids
  surprise pushes to the host or to people who once claimed a need but
  aren't attending.
- **Q5: Do we need a separate iOS APNs category for each toggle, or
  one?** — *Decision:* one APNs category for now; native categories
  tracked separately in **#1551**. The Phase 1–5 gating is server-side
  and lands without OS-category work.
- **Q6: Should we ship a "mute all for this community" master switch?**
  Spec doesn't ask. — *Recommendation:* no, defer. Eight individual
  toggles cover it; adding a master adds proto/UI surface for an
  unproven need.
- **Q7: Does the API need a way to read defaults so the client can render
  toggles before the user has saved anything?** — *Recommendation:* yes
  — `GetCommunityNotificationPreferences` returns a fully-populated
  message with default-on values when no row exists. Client never sees
  "missing".

## Implementation Plan

### Phase 1: Schema, API surface, and file split (no behavior change)

**Goal:** Land the model, RPCs, and the events.go split so subsequent
phases have something to read from and somewhere to put new code. No
notification-path code changes yet — this phase ships dark.

**Tasks:**

*Storage model:*
- [ ] Add `CommunityNotificationPreferences` to
      `proto/ripls/models/community.proto`: fields are `id`,
      `community_id`, `user_id`, nine `optional bool` category fields
      (`notify_new_requests`, `notify_new_experiences`,
      `notify_gear_shared`, `notify_experience_completed`,
      `notify_transfer_updates`, `notify_request_updates`,
      `notify_experience_rsvps`, `notify_planning_updates`,
      `notify_chats`), `created_at_unix_sec`,
      `updated_at_unix_sec`. **Unset = on.** This is the
      forward-compat semantics we want: adding a future category
      means existing rows automatically opt in to it without a
      migration.
- [ ] Add a `NotificationCategory` enum in `proto/ripls/models/` for
      the 9 categories so the server filter is a switch on enums, not
      strings.

*API messages (separate per `docs/proto_conventions.md` — never reuse
request/response types across RPCs):*
- [ ] Add `CommunityNotificationPreferences` API message in
      `proto/ripls/api/community_service.proto` mirroring the storage
      shape with the same nine `optional bool` fields. The API
      message intentionally does not carry `id`,
      `created_at_unix_sec`, or `updated_at_unix_sec` — those are
      storage concerns the client doesn't need.
- [ ] Add `GetCommunityNotificationPreferencesRequest/Response` and
      `UpdateCommunityNotificationPreferencesRequest/Response` to
      `community_service.proto`. `Get` returns a fully-populated
      message with defaults; `Update` is upsert by (user_id,
      community_id).
- [ ] Add the two RPCs to the `CommunityService` definition.

*Server (with file split per CLAUDE.md ≤1000-line rule):*
- [ ] Split `server/community/events.go` into:
      - `server/community/events.go` — keep
        `RecordCommunityEventAndNotify`, `eventBroadcaster`,
        `streamChecker`, `SetEventBroadcaster`, `SetStreamChecker`,
        constants.
      - `server/community/notifications.go` — move `shouldNotify`,
        `getNotificationRecipients`, `notifyTargetedUsers`,
        `buildNotification`, all `get*` helpers
        (`getGearOwnerID`, `getTransferRecipientID`,
        `getRequestCreatorID`, `getRequestOfferers`,
        `getCommunityMemberIDs`, `getUserName`, `getGearName`,
        `getExperienceName`, `getRequestTitle`).
      - Use `git mv` to preserve blame on the moved bits where
        practical, or do plain moves and note the rationale in the
        commit.
- [ ] Implement the RPCs in
      `server/services/community/notification_prefs.go`. Helper:
      `loadPreferences(ctx, userID, communityID)` reads the row (or
      absence) and returns a resolved view where each category is
      `true` unless explicitly set to `false`. Server-side filter
      logic only ever asks "should I send?" against the resolved
      view, never inspects `Has*()` directly.
- [ ] Conversion helpers `apiToModelsPreferences` /
      `modelsToAPIPreferences` in the same file.

*Tests:*
- [ ] Round-trip test asserting API ↔ models conversion preserves
      every field, including the unset state of any optional bool
      (per `docs/proto_conventions.md` round-trip test expectation).
- [ ] Unit tests covering: missing row → all categories resolve to
      on; partially-set row → only set fields override the default;
      Update is idempotent upsert; cross-user/cross-community
      isolation.
- [ ] No new `CommunityEventType` values yet — undo-registry gate
      doesn't fire in this phase.

**Validates:** Schema and conversion compile cleanly, RPCs round-trip,
defaults work, `npm run build` passes (file-size lint included).

### Phase 2: Wire preferences into existing notification paths

**Goal:** Make the *currently-notified* events respect preferences.
Behavior change: a user who toggles a category off stops getting that
category's pushes today. Includes chats — the lowest-risk place to
expose the gate end-to-end first since chat notifications are the
loudest path.

**Tasks:**
- [ ] Add a `categoryFor(eventType) NotificationCategory` map in
      `server/community/notifications.go`.
- [ ] In `notifyTargetedUsers`, batch-fetch
      `CommunityNotificationPreferences` for all `recipients` in one
      `QueryByFieldIn` and skip recipients whose category resolves to
      off.
- [ ] In `notifyRSVPedUsers`, look up the
      `notify_experience_rsvps` toggle for each RSVP'd user before
      sending. Batch-fetch by community_id. (If #1552 lands first,
      this entry collapses into the standard funnel.)
- [ ] In `server/services/chat/notifications.go:notifyParticipants`,
      look up `notify_chats` for each participant against
      `conversation.CommunityId` and skip those who have it off.
      Batch-fetch — message volume is high so this matters most here.

*Error-handling semantics (per CLAUDE.md "Fail Fast" rule, with a
documented exception):*
- [ ] If the batch preference fetch fails, **fail open**: log a
      `warn` with `community_id`, `recipient_count`, and the error,
      then proceed as if every category were on. Rationale documented
      inline: a transient DB error must not silently mute pushes for
      a whole community; missed pushes are a worse failure mode than
      a duplicate during the brief degraded period. This is the
      "documented fallback strategy" the rule allows.
- [ ] Test the fail-open path: inject a storage error and assert
      `NotifyUser` is still called for all recipients, and a warn-
      level log is emitted.

*Tests:*
- [ ] Integration test that toggles off one category, fires a
      matching event, asserts `MockService.NotifyUser` was not called
      for the toggled-off user but was called for the rest. Repeat
      for the chat path.
- [ ] No new event types yet. No UI yet.

**Validates:** Filtering works end-to-end on the unchanged event set.

### Phase 3: Expand event coverage

**Goal:** Start sending pushes for the categories that were previously
silent — now safely gated by preferences (which default on).

**Tasks:**
- [ ] Update `shouldNotify` to allow: `GEAR_SHARED`, `REQUEST_CREATED`,
      `EXPERIENCE_COMPLETED`, `TRANSFER_ACTIVE`,
      `REQUEST_OFFER_SELECTED`, `REQUEST_OFFER_WITHDRAWN`,
      `REQUEST_CANCELLED`, `TRANSFER_INTEREST_WITHDRAWN`.
- [ ] Add cases to `getNotificationRecipients` for each new type.
      Broadcast types (`GEAR_SHARED`, `REQUEST_CREATED`,
      `EXPERIENCE_COMPLETED`) use the existing
      `getCommunityMemberIDs(communityID)` minus actor pattern from
      `EXPERIENCE_CREATED`. Per-party types use the relevant entity
      lookup helpers.
- [ ] Add cases to `buildNotification` with copy for each new type.
      Match existing tone ("Interest in X", "Request Approved").
- [ ] Tests for each new type: recipient resolution, preference gate,
      copy assertion.

**Validates:** New event types flow through the full pipeline,
respecting preferences.

### Phase 4: Promote planning events to community events

**Goal:** Hook the "Event planning updates" toggle into actual push
delivery. Today these only emit chat-system-messages.

**Tasks:**
- [ ] In `server/planning/`, after each chat-system-message emission
      for `PLANNING_NEED_ADDED`, `PLANNING_NEED_CLAIMED`,
      `PLANNING_CONTRIBUTION_ADDED`, also call
      `RecordCommunityEventAndNotify` with the matching
      `CommunityEventType`.
- [ ] Add recipient resolvers in `getNotificationRecipients`:
      `ExperienceRSVP` rows for the experience where
      `intention IN ("YES", "MAYBE")`, minus the event actor. Same
      shape as `notifyRSVPedUsers` (`lifecycle.go:749`). Host is
      included only if they have an RSVP — keeps the recipient set
      consistent with "Experience RSVPs".
- [ ] Add notification copy in `buildNotification`.
- [ ] Add the three event types to `categoryFor` →
      `NOTIFY_PLANNING_UPDATES`.
- [ ] Verify undo registry coverage. Planning events likely fit
      `ClassificationClientOnly` since add/remove pairs already exist
      — confirm in `server/undo/registry.go`.
- [ ] Tests: planning event creates both chat system message and
      community event; preference gate works; recipient resolution
      excludes uninvolved members.

**Validates:** All eight categories now wired to a real push path.

### Phase 5: Flutter UI

**Goal:** Ship the "Manage Notifications" screen so users can act on
the preferences. Architecture must follow `docs/client/architecture.md`
strictly: ViewModels call repositories, repositories call services,
screens never reach past the ViewModel.

**Tasks:**

*Service layer (API client, no caching):*
- [ ] Add `community_notifications_service.dart` (or a method on the
      existing `community_service.dart`) with
      `getPreferences(communityId)` and
      `updatePreferences(communityId, prefs)` methods. Pure pass-
      through to the new RPCs — no caching, no logic.

*Repository layer (caching + invalidation, per
`docs/client/caching.md`):*
- [ ] Add `community_notifications_repository.dart` under
      `app/lib/data/repositories/`. Namespace
      `notification_prefs`, key `community_id` (one prefs row per
      community per user). `get(communityId)` uses
      `_cache.get(key: communityId, fetch: () => …)`.
      `update(communityId, prefs)` calls the service then
      `await _cache.invalidate(communityId)` so the next read
      refetches.
- [ ] Add `communityNotificationsRepositoryProvider` Riverpod
      `Provider` wiring `cacheManagerProvider` and the service
      provider.

*ViewModel layer (Riverpod AsyncNotifier per
`client_architecture.md` Async Safety section):*
- [ ] Add `community_notifications_view_model.dart` with
      `AsyncNotifierProvider.autoDispose<…, CommunityNotificationsState>`.
      `build(communityId)` calls the repository's `get`. Per-category
      toggle setters call the repository's `update`, then refresh
      `state` via `AsyncValue.guard` with a `ref.mounted` check after
      the await.
- [ ] State class is Freezed with one `bool` per category plus
      isLoading/error fields. No manual cache map (rule from
      `client_architecture.md`).

*Screen:*
- [ ] Add `manage_notifications_screen.dart` under
      `app/lib/presentation/screens/communities/`. Use
      `buildSwipeableScaffold` + `SwipeToCloseMixin` +
      `AppBarBackButton(onPressed: handleClose)` per the
      Navigation & Gestures section. Build the toggle list with the
      existing `buildSettingsCard` / `buildSettingsSwitchItem`
      widgets (these already use `AppColors.*(context)` — do not
      introduce new hardcoded `Colors.*`).
- [ ] In `governance_screen.dart:162`, insert "Manage Notifications"
      as a new `_GovernanceAction` in the Membership section,
      between "Manage Members" and "Manage Privacy". Open it via
      `NavigationHelpers.pushScreen` (not raw `Navigator.push`,
      not `context.push`).

*i18n (per `docs/client/i18n.md` — every user-visible string in
both locales):*
- [ ] Add ARB keys to `app/lib/l10n/app_en.arb` and `app_es.arb`:
      `settingsManageNotificationsTitle`,
      `settingsManageNotificationsDescription`,
      `settingsNotifyNewRequests`,
      `settingsNotifyNewExperiences`,
      `settingsNotifyGearShared`,
      `settingsNotifyExperienceCompleted`,
      `settingsNotifyTransferUpdates`,
      `settingsNotifyRequestUpdates`,
      `settingsNotifyExperienceRsvps`,
      `settingsNotifyPlanningUpdates`,
      `settingsNotifyChats`, plus a short subtitle per key.
- [ ] Run `npm run generate` to regenerate `AppLocalizations`. Use
      `context.l10n.*` in the screen — not literals. (CI
      `npm run lint:dart:i18n` enforces this.)

*Tests:*
- [ ] Repository unit tests with mocked service: cache hit returns
      cached prefs without re-calling service; `update` invalidates
      so the next `get` fetches fresh.
- [ ] ViewModel unit tests with `ProviderContainer` and a mocked
      repository: toggling a category updates state and calls
      `update`; error path surfaces an `AsyncError`.
- [ ] **Disposal test** (required for autoDispose async Notifiers):
      start an `update` call, dispose the container before it
      completes, assert no crash and the future settles.
- [ ] Widget test: screen renders 9 toggles, defaults are loaded
      (every toggle starts on when the row is absent), toggling a
      switch calls the repository through the ViewModel. Test wrapper
      includes `localizationsDelegates` and `supportedLocales` per
      `client_testing.md` / `app/test/helpers/l10n_helpers.dart`.

*Docs:*
- [ ] Update `docs/push_notifications.md` to reflect the new
      preference-gated model and the expanded event coverage. Add a
      reference to this issue from
      `docs/client/architecture.md` if a notable new pattern lands
      (likely not — this follows existing repo/viewmodel patterns).

**Validates:** End-to-end: a user can open the screen, toggle a
category off, trigger a matching event from another user's session,
and confirm no push lands on the toggled-off user's device.

## Related Issues

- **#1027 (iOS notifications)** — Delivery infrastructure that this
  feature relies on. If iOS push is still flaky when this ships,
  preference-gated suppression won't be visible to iOS users either way.
- **#962 / #977 (chat notification routing / deep links)** — Same
  notification subsystem; chat is now in scope via the single
  per-community chat toggle.
- **#1133 (stale chat notification)** — Adjacent reliability work in
  the same pipeline; no direct overlap.
- **#1551 (native notification categories on iOS and Android)** —
  Filed as a follow-up to this issue. Tracks per-category OS
  channels/categories so users see them in system Notification
  Settings. Out of scope here; this issue ships the server-side
  gate.
- **#1552 (normalize experience-lifecycle RSVP notifications onto
  the community-event funnel)** — Filed as a follow-up. The
  custom `notifyRSVPedUsers` path bypasses the funnel; folding it
  back lets #1107's preference gate work without a bespoke check.
  If #1552 lands first, this plan's Phase 2 chore on
  `notifyRSVPedUsers` collapses into the standard
  `notifyTargetedUsers` change.
