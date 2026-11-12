# Issue #2598: Event chat shows "poll ended" but there was no poll

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2598
**Filed:** 2026-06-30 | **Last activity:** 2026-06-30
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/time.md, docs/client/polls.md, docs/client/chat.md,
docs/issues/2152-tbd-single-direct-set.md, docs/issues/1255-time-poll-second-poll-bugs.md,
docs/client/architecture.md (autoDispose + `ref.mounted`), docs/client/conventions.md
(viewmodel ↔ repository layering, ARB-key reuse), docs/server/architecture.md,
docs/server/conventions.md (fail-fast errors, `connecterr.Internal`, `npm run build`),
docs/server/observability.md (structured-log field names), docs/proto_conventions.md
(no proto change required), docs/testing/architecture.md (unit-first viewmodel testing),
docs/client/testing.md (`--reporter expanded`, ProviderContainer + Mockito),
docs/workflows/experience.md (`SaveExperience` is the manual time-edit path),
docs/context_map.md (freshness ratchet)

## Problem Statement

The reporter manually changed an event's time without starting a poll and the
event's chat thread immediately showed a sage "Poll ended · See results →"
banner. No poll was ever opened in the user's mental model, so the banner is
both confusing and incorrect — there is no poll to "end."

Root cause: the propose-modal's "Set the time" CTA, when the user has staged
exactly one time, calls
[`TimeModalNotifier.setSingleTime()`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart#L699)
which executes `proposeTime` followed by `confirmTime` back-to-back. The server
treats `proposeTime`-on-no-active-poll as **opening a fresh poll**:
[`time_proposals.go:96-118`](../../server/services/experience/time_proposals.go#L96-L118)
flips `TimePollActive = true`, mints a `current_poll_id`, and writes a
`CHAT_SYSTEM_ACTION_TIME_PROPOSED` system message stamped with that `poll_id`.
The immediately-following `ConfirmTime` then clears `TimePollActive`. The chat
message survives.

When the chat renders that message via
[`PollBannerOrLabel`](../../app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart#L57-L75)
the message has a non-empty `messagePollId` but `exp.timePollActive == false`,
so the widget falls into the "ended" branch and renders the spurious
"Poll ended · See results →" card.

The same defect was anticipated as a residual risk in
[`docs/issues/2152-tbd-single-direct-set.md`](2152-tbd-single-direct-set.md)
("Phase 3 — server-side suppression (only if Phase 1 found a leak)") but the
leak was not eliminated when #2152 shipped, so the bug now surfaces in PROD on
v0.25.0 / build 8574.

The same architectural shape exists on the location side (`setSingleLocation`
→ `ProposeLocation` + `ConfirmLocation`), so the same banner can fire from
`LocationPollEndedBanner` for a direct-set spot. The reporter only mentioned
time; the fix should treat both consistently to prevent a paired user-feedback
issue landing next week.

## Context from Comments

The only comment on the issue is a bare `@claude` from the maintainer
(`maintainer`) inviting analysis. No additional reproduction info, no
disagreement, no workaround discussed. The screenshot in the body confirms the
banner reads "Poll ended" in the chat thread.

The relevant prior decision sits in
[`docs/issues/2152-tbd-single-direct-set.md` → Recommendation](2152-tbd-single-direct-set.md):
the team picked Option A (client-side branch, `proposeTime` + `confirmTime`)
contingent on a runtime check that the propose-then-confirm chain did **not**
leak a "poll opened" chat message. This issue is empirical evidence that the
check either was not run or did not surface the leak — Phase 3 ("server-side
suppression") was deferred and now needs to happen.

## Potential Solutions

### Option A: Client-side — route `setSingleTime` through `SaveExperience`

**Approach:** Replace the `proposeTime` + `confirmTime` body of
[`setSingleTime`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart#L699)
with the same pattern
[`updateEventTime`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart#L539)
already uses: fetch the experience to preserve all fields, then call
`saveExperience(id: experienceId, ..., time: …)`. Server-side `SaveExperience`
emits `CHAT_SYSTEM_ACTION_DETAIL_CHANGED` ("Bryan updated time → …"), which
the chat already renders as a non-interactive system row — no poll banner of
any kind appears.

- **Pros:** Smallest patch (one viewmodel method). No proto change. Already
  the pattern used by `updateEventTime` (manual edit of the event time).
  Removes the bug at its source — the TIME_PROPOSED message is never written.
  No transient `TimePollActive=true` server-side either, so the bus event
  `EXPERIENCE_UPDATED` fires cleanly with `time_changed=true` instead of two
  conflicting events.
- **Cons:** Loses the byproduct that the propose+confirm path creates a
  `models.ExperienceTimeProposal{IsConfirmed: true}`. Today the
  experience-time UI does not strictly require a confirmed proposal — the
  same direct-edit `updateEventTime` path also leaves no proposal and the
  panel renders correctly off `experience.time`. Some "time history" code
  may use proposals as a record of past times; needs a quick audit.
- **Effort:** Small. Mirrors an existing well-tested helper.

### Option B: Server-side — `ConfirmTime` retroactively deletes the TIME_PROPOSED message

**Approach:** In
[`server/services/experience/time_proposals.go` ConfirmTime](../../server/services/experience/time_proposals.go#L217-L330),
after the confirm settles, if the proposal being confirmed is the **only**
proposal stamped with the active `current_poll_id` and has zero votes, delete
the `CHAT_SYSTEM_ACTION_TIME_PROPOSED` system message that carries the same
`poll_id`. (Symmetric change in `ConfirmLocation`.) Requires a
`DeleteSystemMessageByPollID` helper on `chat.SystemMessageWriter`.

- **Pros:** Future-proofs any callsite that uses the propose-then-confirm
  shape (including `finalizeEventTime`, which has the same hazard whenever
  the leader was the only proposal). Self-healing for any propose-confirm
  pairs that may be added later. Catches both time and location.
- **Cons:** Adds a new storage/chat write inside a hot RPC path. Heuristic
  on "no votes + single proposal" — a real one-option poll that the owner
  confirms without any voting would also wipe its system message (probably
  fine, but a behavior change). Requires a new chat helper and an
  integration test per side.
- **Effort:** Medium.

### Option C: Proto contract — `direct_set` flag on `ProposeTimeRequest`

**Approach:** Add `optional bool direct_set = N;` to
[`ProposeTimeRequest`](../../proto/ripls/api/experience_service.proto) (and
the location twin). When set, `ProposeTime` skips the
`CHAT_SYSTEM_ACTION_TIME_PROPOSED` emit and skips the `default_deadline`
seeding. The client passes `direct_set: true` from `setSingleTime`.

- **Pros:** Explicit contract. No heuristic. Server still records the
  proposal so any UI that depends on a confirmed proposal keeps working.
- **Cons:** Largest surface area — proto + Go server + Dart client. The
  flag's intent leaks the modal's UX shape into a low-level RPC, which
  bites later if another caller (e.g. a future "shortcut" UI) wants the
  message but lands on the same request type.
- **Effort:** Medium-large.

## Recommendation

**Option A.** It removes the bug at its source with the smallest, most local
change, mirrors a pattern (`updateEventTime`) that already ships the same
contract for the equivalent manual-edit flow, and avoids both a proto change
and a heuristic on the server hot path. The "no confirmed proposal is
created" downside is what `updateEventTime` already does today, so any latent
dependency on that record would already be broken — an audit during
implementation will confirm or surface follow-up work.

For the location side, the same shape applies but with a wrinkle: a staged
**geocoded** drop-pin still needs `ProposeLocation` to materialize a
canonical `location_id`. The recommended split:

- `setSingleLocation(locationId: …)` (saved location case) → switch to
  `SaveExperience` with `locationId` (mirror Option A).
- `setSingleLocation(geocoded: …)` → keep `ProposeLocation`, but follow up by
  layering Option B's retroactive-delete on `ConfirmLocation` so the
  geocoded materialization path stops leaking the banner.

If Option B's retroactive-delete proves clean during implementation we should
take it as the durable belt-and-braces fix for both sides; otherwise Option A
+ a scoped Option B on the location-geocoded branch is the minimum that
closes #2598 and prevents the symmetric location issue from showing up next.

## Open Questions

- **Does the experience-time UI rely on a confirmed `ExperienceTimeProposal`
  being present after a direct set, beyond what `updateEventTime` already
  produces?** Recommendation: audit `TimeModalNotifier.build()` (lockedProposal
  resolution at lines 71-94) and the `mode` transitions, then `experience_time_panel.dart`
  rendering paths driven by `lockedProposalId`. Likely answer: no, because
  `updateEventTime` is the existing direct-edit flow and already runs without
  creating a proposal. Confirm with a manual run of the edit flow before
  shipping.
- **Is the `EXPERIENCE_UPDATED` community-event fan-out (lines 476-508 of
  `save.go`) the right signal for downstream consumers when the time was set
  directly via the modal?** Recommendation: yes — `time_changed=true` already
  fires for any time edit through `SaveExperience` and is what the daily-feed
  and watcher pushes consume. The current propose+confirm path emits a
  `EXPERIENCE_TIME_NUDGE`-style notification chain that nobody asked for.
- **Should we also delete the spurious TIME_PROPOSED messages already in
  PROD?** Recommendation: no — a one-off cleanup migration is more risk than
  the cosmetic stale banner is worth. The bug stops appearing for new edits
  after this ships; the small number of historical messages can be cleaned
  via a follow-up if user complaints continue.
- **Location side: do we close it in the same PR or split?** Recommendation:
  same PR for the `locationId` branch (one-line mirror of the time fix); file
  a follow-up for the geocoded branch since it needs the Option B server
  helper. Cross-link both PRs to #2598 and #2152.
- **Does the switch to `SaveExperience` double-fire push notifications?**
  Recommendation: review the `EXPERIENCE_UPDATED` fan-out in
  [`save.go:476-508`](../../server/services/experience/save.go#L476-L508)
  during implementation. Today the propose+confirm path runs
  `ProposeTime` (no fan-out) + `ConfirmTime` (no fan-out → only emits the
  `DETAIL_CHANGED` chat message), so this fix introduces one
  `EXPERIENCE_UPDATED` community-bus event per community the experience is
  shared with — the same volume an explicit "edit time" already produces.
  This is the correct signal volume; flag it only if a downstream consumer
  duplicates pushes for `time_changed=true`.

## Implementation Plan

### Phase 1: Fix the time-side direct set

**Goal:** Stop the spurious "Poll ended" banner in the chat for any future
event whose owner sets the time via the propose modal with one staged
candidate.

**Tasks:**
- [ ] Rewrite
      [`TimeModalNotifier.setSingleTime`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart#L699)
      to mirror `updateEventTime` (same shape, driven off the explicit
      `DateTime` argument instead of form state). The viewmodel keeps
      calling through the repository (`_repository.saveExperience(…)`),
      never the underlying service — preserves the
      ViewModel→Repository→Service layering required by
      `docs/client/architecture.md`:
  - Resolve the user's IANA timezone via `_resolveUserTimezone()` (already
    called).
  - Fetch the current experience details via
    `_repository.getExperienceDetails(experienceId)`.
  - Build the new `ExperienceTime(specific: …)` from the passed `DateTime` +
    a 60-minute default duration (matches today's behaviour at lines 707-717).
  - Call `_repository.saveExperience(id: …, name: …, description: …,
    mediaIds: …, locationId: …, time: …, maxParticipants: …, sourceUrl: …)`
    preserving all existing fields.
  - Keep the `assert(!previous.timePollActive)` precondition and the
    `if (!ref.mounted) return;` checks after each `await` — required by
    `docs/client/architecture.md` ("Disposal-testing").
  - End with `ref.invalidateSelf()` so the modal rebuilds from the new
    server state.
- [ ] Update
      [`time_modal_view_model_test.dart`](../../app/test/presentation/viewmodels/time_modal_view_model_test.dart)
      `group('setSingleTime', …)`:
  - Replace `verify(mockRepo.proposeTime(…))` / `verify(mockRepo.confirmTime(…))`
    happy-path assertions with `verify(mockRepo.saveExperience(id: …, time: …))`.
  - Keep the disposal-during-await test; have the mock save block on a
    pending Completer so we can dispose mid-flight.
  - Add a regression assertion: after `setSingleTime`, no `proposeTime` /
    `confirmTime` call is observed on the mock repository.
- [ ] Add a Go unit test in
      [`server/services/experience/time_proposals_test.go`](../../server/services/experience/time_proposals_test.go)
      asserting that **`SaveExperience` with a new `time` field never emits a
      `CHAT_SYSTEM_ACTION_TIME_PROPOSED` system message** (and that it does
      emit `CHAT_SYSTEM_ACTION_DETAIL_CHANGED`). This guards against any
      future regression that re-wires the modal into the propose+confirm
      path. Use the existing `systemMessageWriter` test helper; no
      `//go:build integration` tag (no external service calls — see
      `docs/server/conventions.md` → "Server Testing"). Run `npm run build`
      after the change to satisfy golangci-lint, gofumpt, godot, and revive
      gates.
- [ ] **Run client tests with `--reporter expanded`** —
      `cd app && flutter test --reporter expanded` (or
      `npm run test:app`). The compact reporter mangles output piped from
      CI; see `docs/client/conventions.md` → "Client Testing".
- [ ] Update [`docs/client/time.md`](../client/time.md) — the
      "TimeModalNotifier mutations" table currently records
      `setSingleTime(dt)` as backed by `ProposeTime + ConfirmTime`. After
      Phase 1 it is backed by `SaveExperience` only. The doc-freshness
      ratchet
      (`docs/context_map.md` → "Prose freshness") should pick this up
      automatically because the change touches the doc's `globs`; verify
      by running the `ripls-doc-freshness` skill against the file
      pre-merge so the verified-commit stamp advances cleanly.

**Validates:** New events created after the fix never show the spurious
"Poll ended" banner when the owner sets a single time directly.

### Phase 2: Mirror the fix on the location-saved-id branch

**Goal:** Close the symmetric defect for the `setSingleLocation(locationId)`
case so we don't ship a "fixed time, still buggy location" half-fix.

**Tasks:**
- [ ] In
      [`LocationModalNotifier.setSingleLocation`](../../app/lib/presentation/viewmodels/location_modal_view_model.dart#L344),
      branch on `locationId != null`: in that case, mirror the time fix —
      fetch the experience and call `saveExperience(locationId: …)` instead
      of `proposeLocation` + `confirmLocation`. Keep the geocoded branch on
      the existing propose+confirm path so the materialization side effect
      (resolving a drop-pin into a saved `location_id`) still happens.
- [ ] Extend the matching test
      [`location_modal_view_model_test.dart`](../../app/test/presentation/viewmodels/location_modal_view_model_test.dart)
      with happy-path + disposal-safety coverage for the new locationId
      branch. Match the existing test style — ProviderContainer +
      Mockito-generated `MockExperienceRepository` overrides, per
      `docs/client/testing.md`.
- [ ] Run `cd app && flutter test --reporter expanded
      test/presentation/viewmodels/location_modal_view_model_test.dart
      test/presentation/viewmodels/time_modal_view_model_test.dart`
      locally before pushing; both belong to the same suite and exercise
      the same surface.

**Validates:** Owners selecting an already-saved location for the first time
no longer see a spurious "Poll ended" location banner. The geocoded branch
continues to function (covered by existing tests); its residual leak is
addressed in Phase 3.

### Phase 3: File a follow-up for the geocoded-location residual

**Goal:** Track the remaining propose+confirm leak for the
location-geocoded drop-pin case without blocking this PR.

**Tasks:**
- [ ] Open a new GitHub issue titled "Geocoded direct-set location still
      leaks 'poll ended' chat banner (residual from #2598)" referencing
      both #2598 and #2152 → Phase 3.
- [ ] Suggested resolution in the new issue's body: Option B (server-side
      retroactive delete in `ConfirmLocation` when the confirmed proposal
      was the only one in the poll and had zero votes) — the same shape we
      considered here, scoped to the single remaining path. Any new server
      log lines on that path must use
      `logging.LoggerWithContext(ctx).With("operation", "ConfirmLocation",
      "experience_id", …)` per `docs/server/observability.md`. Any new
      errors returned must use `connecterr.Internal(ctx, op, err, kv…)`,
      never bare `connect.NewError(connect.CodeInternal, err)` (gated by
      `scripts/check_codeinternal.js`).
- [ ] In the new issue, link back to
      [`docs/issues/2152-tbd-single-direct-set.md`](2152-tbd-single-direct-set.md)
      → Phase 3 so future readers see the full lineage.

**Validates:** No surprise the next time a user-feedback issue lands on
geocoded direct-sets; reviewers can ack-and-close in seconds.

## Audit notes

Audited against the project's documentation context map (`docs/llms.txt`)
with the following docs loaded — anchors for both sides plus the leaf docs
whose `globs` match the touched paths:

- ⚓ `docs/client/architecture.md` — confirms ViewModel-only repository
  access, autoDispose semantics, `ref.mounted` guard discipline (already
  reflected in the plan).
- ⚓ `docs/client/conventions.md` — `--reporter expanded` for flutter
  test, ARB key reuse rule (the fix reuses `chat.detail_changed_time`,
  no new ARB keys needed).
- ⚓ `docs/server/architecture.md` + ⚓ `docs/server/conventions.md` —
  fail-fast, `connecterr.Internal`, no bare `connect.NewError(CodeInternal,
  …)`, `npm run build` after Go changes.
- ⚓ `docs/server/observability.md` — required field names if any new
  server log lines are added (none in Phase 1-2; Phase 3 note added).
- ⚓ `docs/proto_conventions.md` — verified the plan introduces **no
  proto change**, so the separation rule and field-naming parity rules
  do not engage.
- ⚓ `docs/testing/architecture.md` — unit-test-first, ViewModel +
  Mockito mock-service pattern (already reflected).
- `docs/client/time.md` — confirmed the doc currently records the
  `propose + confirm` shape for `setSingleTime`; added a freshness-stamp
  refresh task.
- `docs/client/polls.md` — confirmed parity contract (time + location
  flows must match); the plan addresses both sides explicitly.
- `docs/client/chat.md` (read transitively from the
  `PollBannerOrLabel` reference) — confirmed `_PollEndedChatBanner` is
  the spurious surface and that no chat-side change is needed once
  TIME_PROPOSED stops being written.
- `docs/workflows/experience.md` — confirmed `SaveExperience` is the
  conventional manual time-edit path (already used by `updateEventTime`).
- `docs/issues/2152-tbd-single-direct-set.md` — confirmed this issue is
  the residual Phase 3 that was conditionally deferred; cross-linked.

**Engaged engineering dimensions:** `testing`, `code_quality`,
`error_handling` (Phase 3 callout only — no new errors in Phase 1-2),
`observability` (Phase 3 callout only), `i18n` (no new ARB keys; reuses
the existing `chat.experience.detail_changed_time` template — verified
present in `server/chat/system_message_templates.go` and the
`app/lib/l10n/app_*.arb` catalogs). No engagement with `security`,
`performance`, `api_design` (no RPC surface change), `dependency`, or
`accessibility` (no new widget; the existing
`_PollEndedChatBanner` already has `semanticsLabel:
a11yChatViewPollResults`).

## Related Issues

- #2152 — Foundational propose-then-confirm contract for the modal; this
  issue is the residual Phase-3 work that was deferred.
- #1255 — Per-poll history rendering; the per-message `poll_id` stamping
  introduced there is what makes today's spurious banner specifically say
  "Poll ended · See results →" rather than fall back to the live banner.
- #2131 — Time-poll migration; the chat-message shape and `PollBannerOrLabel`
  selector were rebuilt here.
- #1219 — Plan-tab time-poll consolidation; defines the
  "Poll ended · Pick the winning time" copy that today's bug surfaces.
