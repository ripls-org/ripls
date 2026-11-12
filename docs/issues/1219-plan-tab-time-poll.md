---
name: Issue 1219 plan — Plan-tab time polls
description: Implementation plan for surfacing time polls on the Plan tab, simplifying vote/propose modals, and letting any participant start a poll
---

# Issue #1219: Surface time polls on the Plan tab and let any participant propose/edit options

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1219
**Filed:** 2026-04-22 | **Last activity:** 2026-04-22
**Labels:** enhancement, P2 | **Assignees:** unassigned
**Context docs:** [docs/planning.md](../planning.md), [docs/client/time.md](../client/time.md), [docs/cowork/mocks/time_poll.jsx](../cowork/mocks/time_poll.jsx)

## Audit Findings (post-write)

This plan was audited against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), and [docs/server/architecture.md](../server/architecture.md). Compliance gaps found and fixed below; details folded into each phase's task list:

- **i18n.** The mock uses English strings inline. Each phase adds explicit tasks to put every user-visible string through `context.l10n.*` and add ARB entries to both `app_en.arb` and `app_es.arb` before generation. The `npm run lint:dart:i18n` check is added to Phase 8.
- **Theming.** The mock bakes hex (`#D4714E`, `#6B8F71`, etc.). All new widgets must consume `AppColors.*(context)` tokens so dark mode works. Each phase specifies which token maps to which mock color.
- **Async safety.** All new viewmodel methods and widget callbacks that cross async gaps must include `if (!ref.mounted) return;` checks before state assignment — required for autoDispose providers per [architecture.md §Async Safety](../client/architecture.md). Called out in Phases 3, 4, 5, 7.
- **Cache invalidation.** Mutations from the new modals reuse the existing `TimeModalNotifier` pattern (`ref.invalidateSelf()` → cache invalidation → experience provider refresh). No new repository methods are needed — explicitly noted in Phases 3, 4, 5, 7 so implementers don't duplicate the invalidation logic.
- **State placement.** The vote modal's "which options have I picked" selection, and the "Choose time" chip's visibility rule, are both *business* state — they should live in the notifier/state layer, not in widget `setState`. Phases 3 and 5 updated to keep selection state in a small scoped notifier / computed state rather than `StatefulWidget` fields (consistent with architecture.md's rule that `setState` is for local UI state only, like text-field focus).
- **Server error handling.** Phase 1 note: the existing `ProposeTime` code at [time_proposals.go:71-74](../../server/services/experience/time_proposals.go#L71-L74) warns-and-continues on a failed `time_poll_active` update. That violates CLAUDE.md's fail-fast rule. This is pre-existing and out of scope for #1219, but flagged here as a latent issue to file separately.

## Problem Statement

Time polling on Experiences is powerful but underused. It is:

1. **Hard to find.** A poll lives two taps deep inside the time modal's Alternatives tab. Non-owners see a coral button on the Experience content view, but it sits next to RSVP/Share and is easy to miss.
2. **Hard to use.** The vote view ([poll_vote_view.dart](../../app/lib/presentation/widgets/experience/poll_vote_view.dart)) packs three sub-flows (vote, "none of these work," suggest a different time) into one scrolling panel with a three-state bottom bar. The propose flow ([poll_options_builder.dart](../../app/lib/presentation/widgets/experience/poll_options_builder.dart), [propose_time_form.dart](../../app/lib/presentation/widgets/experience/propose_time_form.dart)) asks for date + time + duration via three chip triads, which is more picking than most users need.
3. **Too organizer-heavy.** Today **only the experience owner can create a poll or add options to an existing one** — `ProposeTime` server-side is gated `if expStored.OwnerId != authInfo.UserID` ([time_proposals.go:51](../../server/services/experience/time_proposals.go#L51)). This is the root cause of (1): polls don't get started because the organizer has to notice the need and take the action.

The issue proposes (a) a coral "Poll is live" banner at the top of the Plan tab, (b) a simpler standalone vote modal, (c) a simpler standalone propose modal with an "any date/time" escape hatch, (d) server-side permission loosening so any eligible participant can start a poll, (e) a built-in **"Choose time"** Need suggestion chip that creates a poll when tapped, and (f) removal of the Alternatives tab from the unified time modal.

**Key factual correction from codebase research:** The issue body states "Today `ProposeTime` is gated to 'any participant' on the server" and that "participants can propose additional times *into an existing* poll." Both are inaccurate — [time_proposals.go:50-54](../../server/services/experience/time_proposals.go#L50-L54) rejects every non-owner `ProposeTime` call with `PermissionDenied`, whether or not a poll is active. The client's non-owner propose button in [poll_vote_view.dart](../../app/lib/presentation/widgets/experience/poll_vote_view.dart) therefore errors on submit today. This corrects the plan's scope: item 4 is not a one-line server change, it's the load-bearing change that makes the whole feature work.

## Context from Comments

The issue has **no comments** at time of planning. The description is the sole source of intent. This is a signal to flag scope and permission-model decisions as open questions rather than assume silent consensus.

## Potential Solutions

### Option A: Ship Experience-scope only, descope Request

- **Approach:** Build the Plan-tab banner, new vote modal, new propose modal, any-participant server permission, "Choose time" suggestion chip, and Alternatives-tab removal — all for Experiences only. File a follow-up issue for Request-scope time polls.
- **Pros:** Keeps the blast radius proportional to the UX value. The server infrastructure for Request-scope polls does not exist (no `time_poll_active` field on [request.proto](../../proto/ripls/models/request.proto), no RPCs in [server/services/request/](../../server/services/request/), no storage). Building it is a sizeable parallel workstream that would delay the Experience win.
- **Cons:** The Plan tab then has asymmetric behavior — Experience Plan tabs show the poll banner and "Choose time" chip, Request Plan tabs don't. Discoverable, documentable, but asymmetric.
- **Effort:** medium (Experience-only, client-heavy, one server permission change).

### Option B: Ship full symmetry — build Request-scope time polls first

- **Approach:** Add time-poll RPCs, storage fields, and chat system messages to the Request service as a prerequisite phase, then do the shared client work for both scopes in one pass using the existing `NeedsScope` sealed-class pattern.
- **Pros:** Ends with symmetric Plan tabs and a single set of shared widgets across both scopes. Matches the rest of the Plan-tab feature surface, which is already symmetric.
- **Cons:** Doubles the server work and adds proto + storage migrations that are not what the issue is really about. The issue-stated blocker — findability and organizer-bottleneck — is an Experience pain; Request polls are speculative ("when can someone help me move?"). Risk of building Request-scope infra that doesn't get used.
- **Effort:** large.

### Option C: Client-only first — surface what exists, defer server permission change

- **Approach:** Ship the Plan-tab banner, new vote modal, new propose modal, and Alternatives-tab removal as a pure client refactor. Keep the server permission as owner-only for now. Drop the "any participant can start a poll" item and the "Choose time" suggestion chip.
- **Pros:** Smallest blast radius. Pure refactor; no server changes; validates the new UX with existing permissions.
- **Cons:** Ignores the issue's headline goal ("too organizer-heavy"). The Plan tab becomes a prettier wrapper around an owner-only feature. We ship (1) and (2) from the problem statement but not (3) — the actual root cause.
- **Effort:** small-to-medium.

## Recommendation

**Option A.** The issue's headline pain is organizer-bottleneck — solving it requires the server permission change and the "Choose time" suggestion chip (both Experience-only), so Option C undersells the work. Option B is disproportionate: Request-scope time polls are a speculative feature that would double the server work, and there's no signal in the issue or elsewhere that users are asking for Request-scope scheduling today.

Ship Experience-scope end-to-end in Option A. File a tracked follow-up for Request-scope polls that bundles the server infrastructure (proto, storage, RPCs, chat messages) with the client symmetry work. The `NeedsScope` sealed-class pattern ([needs_scope.dart](../../app/lib/presentation/viewmodels/needs_scope.dart)) already handles the two scopes cleanly, so the Experience-only widgets built here can be made scope-agnostic without rework when Request support lands.

## Resolved Decisions

All open questions resolved by the issue author on 2026-04-22. Phases updated to reflect these calls.

- **Non-owners proposing options** — ✅ **Allow both** (start the first poll *and* add to a live poll). Gating is "eligible participant" (owner or YES/MAYBE RSVP), not "first proposer only." Matches the Needs/Contributions model.
- **"Choose time" chip visibility** — ✅ **Show whenever there is no active poll** — even if a confirmed time already exists. Rationale: someone who can't make the confirmed time should be able to re-open coordination by proposing an alternative. The chip disappears only while a poll is active (the banner replaces it). **Change from initial recommendation** (which would have hidden the chip after any time was confirmed); Phase 5 conditions updated to single rule `!experience.timePollActive`.
- **Content-view poll button** — ✅ **Keep both** the content-view button and the new Plan-tab banner. Users may be on either tab. Revisit after launch telemetry.
- **"None of these work for me"** — ✅ **Drop the mutually-exclusive row permanently**; keep the server's `TIME_VOTE_STATUS_NONE_WORK` path untouched. If a user can't make any listed option, they now propose an alternative (one-tap from the vote modal's dashed "Add a time option" row) and vote for it — a more useful signal than "none work."
- **Chat message for non-owner poll start** — ✅ **Use the existing `TimePollOpenedText(displayName)` template** with the non-owner's name. No new chat template.
- **Activation RPC** — ✅ **Keep piggy-backing** activation onto the first `ProposeTime`. No new RPC; Phase 1 only loosens the permission check.
- **Duration stays owner-editable** — ✅ **Yes, kept out of the simple propose modal.** Simple flow is day + period only. Owners change duration through the time-modal Set Time tab; anyone who needs custom duration uses the "Add a specific date & time" escape hatch.
- **Poll-activated push notification / badge** — ✅ **Out of scope for #1219.** File separately if we want to nudge non-openers.

## Implementation Plan

### Phase 1: Server — loosen `ProposeTime` permission

**Goal:** Allow any eligible participant (owner or YES/MAYBE RSVP) to call `ProposeTime`. This is the single change that makes the whole client plan possible.

**Tasks:**
- [ ] Replace the `expStored.OwnerId != authInfo.UserID` check at [time_proposals.go:50-54](../../server/services/experience/time_proposals.go#L50-L54) with a check that the caller is (a) the owner, (b) an attendee with YES or MAYBE RSVP, or (c) an experience participant in a JOINED state. Reuse the same rule used by the Experience needs service (see [server/services/experience/needs.go](../../server/services/experience/needs.go) for the canonical check). Per [server/architecture.md §Service Independence](../server/architecture.md), factor the shared check into the existing `authorization.go` helper (or create one) within the experience package if it's duplicated; do not reach across service boundaries.
- [ ] Keep the chat system message on first activation (the existing `TimePollOpenedText(displayName)` call works for any actor).
- [ ] Update the `ProposeTime` handler's logger (via `logging.LoggerWithContext(ctx)`) to include an `actor_is_owner` field for observability — helps diagnose the new path in production. Use the standard field conventions in [CLAUDE.md §Structured Logging](../../CLAUDE.md).
- [ ] Add unit tests in [server/services/experience/time_proposals_test.go](../../server/services/experience/time_proposals_test.go): (a) non-owner YES RSVP can call `ProposeTime` and the first call sets `time_poll_active=true`, (b) non-owner with no RSVP is rejected, (c) community member with no RSVP is rejected, (d) owner still works (regression), (e) terminal-state experience still rejects all proposers. Follow the table-driven pattern in [server/architecture.md §Unit Tests](../server/architecture.md).
- [ ] Verify `VoteOnTime` and `CancelTimePoll` permissions are unchanged (they already have the right semantics: vote = any participant, cancel = owner only).
- [ ] Run `npm run build` (golangci-lint + go build) per CLAUDE.md §Go Server Build Verification before closing the phase.

**Validates:** Non-owners can start and add to a time poll end-to-end via the RPC. The existing `poll_vote_view.dart` non-owner propose path (which errors today) will start succeeding — this is in itself a useful regression fix.

### Phase 2: Client — Plan-tab poll banner (Experience scope)

**Goal:** Render the coral "Poll is live" banner at the top of the Plan tab, matching the `PlanTab` component in [docs/cowork/mocks/time_poll.jsx](../cowork/mocks/time_poll.jsx). Voted-state computation reuses the Experience content-view poll button's exact logic.

**Tasks:**
- [ ] Create `app/lib/presentation/widgets/planning/poll_banner.dart` with a `PollBanner` `ConsumerWidget` that reads the experience via `ref.watch` on the existing experience provider (per [architecture.md §Providers](../client/architecture.md)). Widgets never touch repositories directly.
- [ ] Visibility: hidden when `experience.timePollActive == false`; otherwise shown.
- [ ] Voted-state: reuse the `YES`-vote filter from [experience_content_view.dart:1049-1069](../../app/lib/presentation/screens/experience/experience_content_view.dart#L1049-L1069) — NONE_WORK and UNSPECIFIED do not trigger the voted state. Expose this as a computed getter on the existing state class (or a tiny helper) rather than duplicating the filter inline in the widget.
- [ ] **Theme**: map mock colors to `AppColors` tokens — coral gradient background uses `AppColors.coral*(context)` family; sage voted-state uses `AppColors.sage*(context)`; text over the gradient uses `AppColors.onPrimary(context)` or equivalent. No hardcoded `Color(0xFF...)` values; add a new token to `AppColors` if needed (per [architecture.md §Colors and Theming](../client/architecture.md)).
- [ ] **i18n**: add ARB keys to both `app_en.arb` and `app_es.arb` for: `planPollBannerEyebrow` ("Poll is live"), `planPollBannerHeadline` ("When should we go?"), `planPollBannerMeta` (with `{votes}` and `{options}` ICU plural placeholders), `planPollBannerCta` ("Vote →"), `planPollBannerVotedLabel` ("Voted ✓"). Access via `context.l10n.*`.
- [ ] Tap opens the new standalone vote modal (Phase 3).
- [ ] Wire the banner into [needs_section.dart](../../app/lib/presentation/widgets/shared/needs_section.dart) at the top of the scroll area. The banner is Experience-scope-only; in the scope switch, only `ExperienceNeedsScope` renders it (no-op for `RequestNeedsScope`).
- [ ] Widget tests (with `localizationsDelegates` wrapper per [helpers/l10n_helpers.dart](../../app/test/helpers/l10n_helpers.dart)): voted-state computation (YES only), visibility on `timePollActive` toggle, tap-to-open callback fires, and golden-ish render in both light and dark `AppColors` themes.

**Validates:** Discoverability — users who open the Plan tab can see a poll is live without opening the time card.

### Phase 3: Client — Standalone vote modal

**Goal:** Replace the Alternatives-tab voting UI with a standalone modal matching the `VoteModal` component in the mock. The modal is reachable from the banner (Phase 2), the content-view poll button, and anywhere else that currently deeplinks to the Alternatives tab for a non-owner.

**Tasks:**
- [ ] Create `app/lib/presentation/screens/experience/time_poll_vote_modal.dart`. Opens via `showModalBottomSheet`; 82% height per mock. Use `ConsumerStatefulWidget` only if strictly necessary for ephemeral UI state (close animations, scroll controller); prefer routing selection state through a small scoped notifier (next task).
- [ ] **State placement**: the "which options have I picked" selection is business state, not local UI state — put it in a new `AsyncNotifierProvider.autoDispose.family` (e.g., `timePollVoteSelectionProvider(experienceId)`) rather than `setState`. This keeps the modal aligned with [architecture.md §State Management](../client/architecture.md) ("Don't use setState() for app state") and makes the Save behavior testable without a widget tree.
- [ ] UI per mock: sage eyebrow "{X} of {Y} voted", close button, serif "Cast your / vote." title, sage "Tap all that work" sub-label, option tiles with checkbox + date/time + voter-avatar stack, dashed "Add a time option" pill row, single sage "Save (N)" bottom button.
- [ ] **Theme**: map mock tokens to `AppColors` — sage fill/border/eyebrow → `AppColors.sage*(context)`; card background → `AppColors.cardBackground(context)`; borders → `AppColors.border(context)`; muted text → `AppColors.textSecondary(context)`. No hardcoded hex.
- [ ] **i18n**: ARB keys in both locales for: `timePollVoteEyebrow` (with `{voted}` and `{total}` placeholders), `timePollVoteTitleLine1` ("Cast your"), `timePollVoteTitleLine2` ("vote."), `timePollVoteSubLabel` ("Tap all that work"), `timePollVoteAddOption` ("Add a time option"), `timePollVoteSave` (with `{count}` placeholder), `timePollVoteClose` (semantics label).
- [ ] Voting semantics: Save calls the existing `voteOnTime(id, YES)` on the `TimeModalNotifier` for each picked ID — prefer a batched `voteOnTimes(List<String>)` method on the notifier if one can be added cleanly; otherwise `Future.wait` across calls. Reuse the existing optimistic-update pattern from [time_modal_view_model.dart](../../app/lib/presentation/viewmodels/time_modal_view_model.dart). **Cache invalidation**: `TimeModalNotifier` already calls `ref.invalidateSelf()` after mutations per [docs/client/time.md](../client/time.md), which cascades to the experience-repository cache — no new invalidation logic needed.
- [ ] **Async safety**: every notifier method that crosses an `await` must guard with `if (!ref.mounted) return;` before state assignment. Add a disposal test per [architecture.md §Disposal Testing](../client/architecture.md).
- [ ] "Add a time option" opens the new propose modal (Phase 4). On save, the new option appears in the list immediately and the modal stays open for the user to vote on it.
- [ ] Drop the mutually-exclusive "None of these work for me" row; keep the server-side `TIME_VOTE_STATUS_NONE_WORK` path untouched (no RPC changes).
- [ ] Widget tests (with `localizationsDelegates` wrapper): (a) tapping a tile toggles selection in the provider, (b) Save button disabled with no picks, (c) Save fires votes for all picked IDs, (d) add-option row opens propose modal. Plus a disposal test for the selection provider.
- [ ] Do not yet delete [poll_vote_view.dart](../../app/lib/presentation/widgets/experience/poll_vote_view.dart) — wait for Phase 6 when the Alternatives tab is removed.

**Validates:** The voting UX matches the mock's single-screen checklist; Save replaces the tri-state bottom bar.

### Phase 4: Client — Standalone propose modal

**Goal:** Replace the inline propose form (and the owner's Send Poll builder) with a standalone modal matching the `AddModal` component in the mock. Any eligible participant can open it.

**Tasks:**
- [ ] Create `app/lib/presentation/screens/experience/time_poll_propose_modal.dart`. Opens via `showModalBottomSheet`; 82% height per mock.
- [ ] UI per mock: coral eyebrow ("Add an alternative" when poll is live, "Start a time poll" when not), serif "Suggest a / time." title, 4-cell day grid (smart defaults: today, tomorrow, +7d, +14d — reuse the existing smart-default logic from [poll_options_builder.dart](../../app/lib/presentation/widgets/experience/poll_options_builder.dart)), 3-cell period grid (Morning=8am / Afternoon=1pm / Evening=5pm), preview pill with sage numbered badge showing the current selection.
- [ ] **State placement**: put day/period selection in a small scoped notifier (`AsyncNotifierProvider.autoDispose.family` keyed by experienceId) so selection survives rebuilds and is testable without a widget tree. Do not use `StatefulWidget` for selection state.
- [ ] **Theme**: map mock tokens to `AppColors` — coral eyebrow/button/preview badge → `AppColors.coral*(context)`; sage selection highlight → `AppColors.sage*(context)`; surface/background → `AppColors.surface(context)` / `AppColors.cardBackground(context)`; borders → `AppColors.border(context)`. No hardcoded hex.
- [ ] **i18n**: ARB keys in both locales for: `timePollProposeEyebrowAdd` ("Add an alternative"), `timePollProposeEyebrowStart` ("Start a time poll"), `timePollProposeTitleLine1` ("Suggest a"), `timePollProposeTitleLine2` ("time."), `timePollProposeSubtitle` ("Pick a day and a time of day. Your option joins the poll."), `timePollProposePeriodMorning` / `...Afternoon` / `...Evening`, `timePollProposeEscapeHatch` ("Add a specific date & time"), `timePollProposeSaveAdd` ("Add to poll"), `timePollProposeSaveStart` ("Start poll"). Day labels use existing locale-aware `DateFormat` output, not ARB.
- [ ] Escape hatch: dashed coral "Add a specific date & time" button drops down into [propose_time_form.dart](../../app/lib/presentation/widgets/experience/propose_time_form.dart) for custom date/time/duration. Keep that widget — it's the reuse target. Confirm its existing strings are already i18n'd; if not, migrate in this phase.
- [ ] Duration: default to the experience's existing duration (or 2 hrs if unset). Not user-selectable in the simple flow; only via the escape hatch.
- [ ] Bottom bar: coral "Add to poll" (when poll is live) or "Start poll" (when not). On save, call `proposeTime()` on the notifier — server handles activation-on-first-proposal automatically (Phase 1). **Cache invalidation**: reuse `TimeModalNotifier`'s existing `ref.invalidateSelf()` pattern; no new repository methods.
- [ ] **Async safety**: `if (!ref.mounted) return;` guard before every `state = ...` after an `await`. Add a disposal test.
- [ ] Permission gate: only show the "Start a time poll" entry points to callers who pass the eligibility check (owner or YES/MAYBE RSVP). Mirror the rule used by Needs/Contributions so the client-side check matches the server; rule source lives in [needs_scope.dart](../../app/lib/presentation/viewmodels/needs_scope.dart) (`isRsvped || isRsvpedMaybe || isOwner`).
- [ ] Widget tests (with `localizationsDelegates` wrapper): (a) day grid selection updates preview, (b) period grid selection updates preview, (c) Save button calls `proposeTime()` with the right (date, time) tuple, (d) escape hatch opens ProposeTimeForm, (e) permission-gate hides entry points for ineligible callers.

**Validates:** Both poll-creation and add-to-poll flows share one modal; the simple path stays one screen; custom date/time still works via the escape hatch.

### Phase 5: Client — "Choose time" built-in Need suggestion chip

**Goal:** Add a first-class "Choose time" suggestion chip to the empty-state chips on the Experience Plan tab. Tapping it routes to the propose modal instead of the usual Need-creation sheet.

**Tasks:**
- [ ] Define a typed `BuiltInNeedSuggestion` sealed class (or enum) in the viewmodel layer rather than using a sentinel string. Variants: `chooseTime`, plus any other future built-ins. The dispatcher in [needs_section.dart](../../app/lib/presentation/widgets/shared/needs_section.dart) switches on the variant — one branch opens the propose modal (Phase 4), the default branch opens the existing Need-creation sheet. This avoids fragile string comparison and keeps business logic out of the widget layer.
- [ ] **State placement**: compute the "should the Choose time chip appear" visibility rule in `ExperienceNeedsState` (or a thin computed getter on it), not in the widget. The widget renders whatever the state exposes. Single rule: **show whenever `!experience.timePollActive`** — even if a confirmed time exists. Rationale (resolved decision): a user who can't make the confirmed time should be able to re-open coordination by tapping the chip and proposing an alternative, which starts a new poll via Phase 1's permission change.
- [ ] Inject the chip into the displayed suggestions list client-side via the experience-needs notifier's state, not by calling the server or by touching the server's suggestion generator. The server remains the source of AI-generated suggestions; the built-in is prepended in the client viewmodel.
- [ ] **Theme**: chip coloring uses `AppColors.sage*(context)` (to match "request" chips) or `AppColors.coral*(context)` (to match "offer"/poll coral) — decide during implementation; no hardcoded hex.
- [ ] **i18n**: ARB key `planChipChooseTime` ("Choose time") in both locales. Access via `context.l10n.planChipChooseTime`.
- [ ] Hide the chip while a poll is active (the banner replaces it). When the poll ends — whether by confirmation or cancellation — the chip returns because `timePollActive` flips back to false. If a time is subsequently confirmed, the chip stays visible (a user can still start a new poll to re-coordinate). No special tracking needed; all derived from `experience.timePollActive`.
- [ ] On tap: open the propose modal (Phase 4) with "Start a time poll" variant. On save, the server activates the poll (Phase 1), the chip disappears (derived state reacts to the flipped `timePollActive`), and the banner (Phase 2) appears.
- [ ] **Cache invalidation**: after `proposeTime()` succeeds, the existing `ref.invalidateSelf()` on `TimeModalNotifier` cascades to the experience cache. The experience-needs provider separately listens for experience updates (verify this during implementation; if it doesn't, add a cross-provider listener so the chip disappears without a manual refresh).
- [ ] **Async safety**: `if (!ref.mounted) return;` guard in the tap handler before any post-modal state reads.
- [ ] Verify the existing `TimePollOpenedText(displayName)` chat message fires correctly for a non-owner proposer (server-side; will come for free after Phase 1).
- [ ] Widget tests (with `localizationsDelegates` wrapper): (a) chip appears when `timePollActive` is false (both with and without a confirmed time), (b) chip is hidden when `timePollActive` is true, (c) chip reappears after a poll is cancelled or confirmed, (d) tap opens propose modal, (e) save activates poll and banner appears, (f) dispatcher routes `chooseTime` variant to the propose modal and all other variants to the existing Need sheet.

**Validates:** Non-owners can start a poll in one tap from the Plan tab — the headline outcome of the issue.

### Phase 6: Client — Remove the Alternatives tab

**Goal:** Collapse the time modal to a single view (Set Time only). All poll functionality lives on the Plan tab + standalone modals from Phases 2-5.

**Tasks:**
- [ ] In [unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart), delete the `TabController`, tab bar, and Alternatives-tab branch. The modal renders the Set Time view directly. Modal sizing logic simplifies: owner-editing = 90%, read-only = 55%.
- [ ] Remove the `initialTab` parameter from `UnifiedTimeModal.show()` and update every caller listed in the "Entry Points" table of [docs/client/time.md](../client/time.md): EventOrganizerModal, EventJoinerModal (both paths), [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart) (menu + poll button + time card for non-owners).
- [ ] The content-view poll button (line ~1794) currently opens the modal on Alternatives tab. Change it to open the new standalone vote modal (Phase 3) directly. This is the same change in behavior as the Plan-tab banner.
- [ ] Delete [poll_vote_view.dart](../../app/lib/presentation/widgets/experience/poll_vote_view.dart), [poll_live_view.dart](../../app/lib/presentation/widgets/experience/poll_live_view.dart), and [poll_options_builder.dart](../../app/lib/presentation/widgets/experience/poll_options_builder.dart). Keep [propose_time_form.dart](../../app/lib/presentation/widgets/experience/propose_time_form.dart) — the escape hatch in Phase 4 reuses it.
- [ ] [poll_results_view.dart](../../app/lib/presentation/widgets/experience/poll_results_view.dart) (confirm-winner) moves to its own modal — see Phase 7.
- [ ] Keep the owner-facing "Start a time poll" CTA on the Set Time tab — it opens the new propose modal (Phase 4) instead of switching tabs. This preserves Entry Point 5a from the issue.
- [ ] Update [docs/client/time.md](../client/time.md): replace the tab-based description with single-tab modal + two external entry points (Plan-tab banner, Plan-tab "Choose time" chip). Re-do the Entry Points table.
- [ ] Run `flutter analyze` and fix any dangling references to the deleted widgets.

**Validates:** The time modal is a single-purpose surface (view/edit the confirmed time); polling is a first-class Plan-tab activity.

### Phase 7: Client — Confirm-winner modal

**Goal:** Move the poll-ended / confirm-winner flow out of the (now-deleted) Alternatives tab into a standalone modal reachable from the banner (Phase 2) for owners only.

**Tasks:**
- [ ] Create `app/lib/presentation/screens/experience/time_poll_confirm_modal.dart`. Reuse the UX from [poll_results_view.dart](../../app/lib/presentation/widgets/experience/poll_results_view.dart) (sage radio tiles + Confirm Time bottom bar) — visual refresh is out of scope per the issue.
- [ ] **State placement**: the radio selection is business state — put it in a small scoped notifier (`AsyncNotifierProvider.autoDispose.family` keyed by experienceId), not in `setState`.
- [ ] **Theme**: sage highlight/checkmark/button → `AppColors.sage*(context)`; card surface → `AppColors.cardBackground(context)`; borders → `AppColors.border(context)`. No hardcoded hex.
- [ ] **i18n**: ARB keys in both locales for: `timePollConfirmBannerOwner` ("Poll ended · Pick the winning time"), `timePollConfirmTitle`, `timePollConfirmCta` ("Confirm Time"), `timePollConfirmDisabled` ("Pick a time"). Reuse any existing l10n keys from `poll_results_view.dart` where they already exist.
- [ ] The banner (Phase 2) shifts from coral "Poll is live" to sage "Poll ended · Pick the winning time" for the owner when `timePollCompleted=true && !hasSpecificTime`. For non-owners in the same state, the banner is hidden (they have no action to take).
- [ ] Tapping the owner's sage banner opens this modal.
- [ ] Confirm button calls the existing `ConfirmTime` RPC via `TimeModalNotifier.lockTime(proposalId)` — no server change needed. **Cache invalidation**: reuses the existing `ref.invalidateSelf()` path.
- [ ] **Async safety**: `if (!ref.mounted) return;` guard before state reads after confirm RPC.
- [ ] Delete [poll_results_view.dart](../../app/lib/presentation/widgets/experience/poll_results_view.dart) after migration; check for any inbound imports with `flutter analyze` before deletion.
- [ ] Widget tests (with `localizationsDelegates` wrapper): (a) modal shows only for owner, (b) radio selection updates the provider, (c) confirm fires ConfirmTime and closes, (d) disposal test for the selection provider.

**Validates:** Owners can still finalize a poll; non-owners don't see a confirm CTA.

### Phase 8: Docs, polish, and rollout

**Goal:** Update documentation, add analytics hooks, and verify the feature end-to-end.

**Tasks:**
- [ ] Rewrite [docs/client/time.md](../client/time.md) to reflect single-tab time modal + Plan-tab banner + standalone vote/propose/confirm modals + two creation entry points.
- [ ] Update [docs/planning.md](../planning.md) to list "Choose time" as a built-in Need suggestion and describe the poll banner's placement at the top of the Plan tab.
- [ ] Add analytics events for: banner tap, "Choose time" chip tap, propose modal save (owner vs non-owner), vote modal save. Match existing analytics conventions — check how [needs_section.dart](../../app/lib/presentation/widgets/shared/needs_section.dart) emits events.
- [ ] **Run generators and lint**: `npm run generate` (regenerates `AppLocalizations` from ARB), then `npm run lint:dart:i18n` to confirm no hardcoded user-visible strings remain. Also `flutter analyze` to catch dangling references from Phase 6 deletions.
- [ ] **Verify ARB completeness**: every new key exists in both `app_en.arb` and `app_es.arb` with a matching `@key` metadata block declaring placeholders. Missing keys are a compile error because of `nullable-getter: false` in `app/l10n.yaml`.
- [ ] **Theme audit**: grep the new files for `Color(0x` and `Colors.` — should be zero matches outside `AppColors` itself. Any hit is a bug, per [architecture.md §Colors and Theming](../client/architecture.md).
- [ ] Manual QA pass on both Android and iOS in **both light and dark themes, and both locales (en/es)**: (a) owner creates poll from Set Time CTA, (b) non-owner creates poll from "Choose time" chip, (c) non-owner adds option to live poll via vote modal's dashed row, (d) banner shows coral then sage after voting, (e) owner confirms winner from banner, (f) banner disappears after time confirmed.
- [ ] Run [app/test/](../../app/test/) with `flutter test --reporter expanded` per CLAUDE.md, and `go test ./...` from `server/`.
- [ ] Run `npm run build` (Go lint + build) to confirm the Phase 1 server changes pass `golangci-lint` including `gofumpt`, `godot`, and `revive`.

**Validates:** Ship-ready, docs match behavior, analytics let us see whether the organizer-bottleneck hypothesis panned out.

## Related Issues

- None explicitly referenced in #1219. Worth scanning for:
  - Prior issues about time polling discoverability (if any)
  - Prior issues about the Plan tab / Needs / Contributions (recent planning.md rename from "coordination" suggests a live thread)
  - Any server-side issue about `ProposeTime` permission — the current owner-only gate may have been intentional and worth a maintainer sign-off.
