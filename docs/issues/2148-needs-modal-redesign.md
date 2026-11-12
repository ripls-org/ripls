# Issue #2148: Redesign Needs & Contributions to mirror poll-modal pattern

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2148
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [`docs/planning.md`](../planning.md), [`docs/client/polls.md`](../client/polls.md), [`docs/cowork/App Design/needs-redesign.html`](../cowork/App%20Design/needs-redesign.html)

## Problem Statement

The current Needs & Contributions sheets — Experience-scoped sheets under [`app/lib/presentation/widgets/experience/needs/`](../../app/lib/presentation/widgets/experience/needs/) and Request-scoped sheets in [`app/lib/presentation/widgets/request/request_needs_sheets.dart`](../../app/lib/presentation/widgets/request/request_needs_sheets.dart) — share a custom "editorial chrome" (LibreBaskerville titles, NOTE rule sections, coral/sage primary buttons) that predates the unified glass-modal pattern shipped for the location and time polls.

We have a separate, more recent design system for collective-decision surfaces: the five-surface poll flow (Propose / Vote / Manage / Confirm / Finalized) built on `GlassSheet`, with parity enforced as a design contract. Needs & Contributions is conceptually adjacent to polls but **not the same shape** — it is *N people each pick K of M things to do/bring*, not *N people pick 1 of M options*. Multiple participants claim the same Need concurrently up to its slot capacity, freestanding Contributions exist alongside Need-derived ones, and there is no single "winner" to lock in.

We want to take the **chrome** and **lifecycle structure** of the poll modal pattern and apply it to Needs & Contributions, using [`needs-redesign.html`](../cowork/App%20Design/needs-redesign.html) as the visual target. The lifecycle maps as:

| Poll surface | Needs/Contributions equivalent | Mock |
|---|---|---|
| Propose | Planner (bulk-draft + one-at-a-time add) | NP1–NP3 |
| (new) Picker | Picker (search + suggestions + qty stepper + total/each) | NK1–NK4 |
| Vote | Volunteer (tap-to-claim, sage-fill, autosave) | NV1–NV2 |
| Manage | Organizer Manage menu | NO1 |
| Confirm / Finalized | Mark Ready / Finalized roster | NX1–NX2 |

The existing `NeedsScope` sealed class ([`app/lib/presentation/viewmodels/needs_scope.dart`](../../app/lib/presentation/viewmodels/needs_scope.dart)), `NeedsSection`, and `NeedsActions` ([`app/lib/presentation/widgets/shared/needs_actions.dart`](../../app/lib/presentation/widgets/shared/needs_actions.dart)) are the right abstraction layer and stay — they continue to dispatch by scope. The work is replacing the *sheets* they open.

## Context from Comments

The issue has no comments yet (filed in this session). The issue body itself is detailed and was authored after reading the same three context docs, so the brief is internally consistent. Two things worth flagging because they are *not* explicit in the issue:

1. The issue says "no server data-model changes" but the NX1 mock introduces a "Mark Ready" affordance that doesn't map to any existing RPC. Today, Needs/Contributions terminate when the parent entity terminates (COMPLETED/CANCELLED for Experiences, FULFILLED/CANCELLED for Requests) — there is no per-needs-list "locked / ready" state. Either (a) Mark Ready is a pure client visualization that surfaces *when* the parent is in its terminal state, or (b) we do need a new `needs_locked` flag on the parent. This needs a decision before phase 2.
2. The NK1–NK4 Picker introduces capabilities the existing Needs sheets do not have: typeahead search, event-aware suggestions, saved "kits," and a quantity stepper with total/each unit toggle. The current `Need` proto carries `slots` (1–20) but no per-claim quantity unit — the "each" semantics on the mock claim row may need a proto field.

## Potential Solutions

### Option A: Full lockstep replacement, both scopes at once

Build a new `needs/` widget directory under `app/lib/presentation/widgets/needs/` containing the five poll-parallel sheets (Propose / Picker / Volunteer / Manage / MarkReady / Finalized), parameterized over `NeedsScope` exactly the way `PollManageMenuSheet<T>` is parameterized over action enum. Replace the call sites in `NeedsActions` for both scopes in a single PR.

- **Approach:** New shared sheet files, scope-aware via `NeedsScope`. Delete the old per-scope sheet files (`view_need_sheet.dart`, `add_contribution_sheet.dart`, `batch_add_need_sheet.dart`, `batch_add_contribution_sheet.dart`, `edit_contribution_sheet.dart`, `view_contribution_sheet.dart` for Experience; the four request sheets in `request_needs_sheets.dart`) once the new sheets are wired up.
- **Pros:** Enforces parity from day one (matches the explicit acceptance criterion). One review surface. No dead code period. The `NeedsScope` sealed class already pattern-matches cleanly, so per-scope behavior fits the existing seam.
- **Cons:** Larger PR, harder to review. Higher risk of regression across both scopes simultaneously. Hard to break into shippable increments.
- **Effort:** Large (estimate 4–6 weeks of focused client work).

### Option B: Scope-by-scope migration with shared widgets factored out as we go

Ship Experience scope first behind a feature flag, observe, then ship Request scope. Build the shared widgets (sheet chrome, picker, claim row) as the Experience port lands so the Request port becomes mostly wiring.

- **Approach:** Phase 1 builds the shared widget primitives (analogous to `PollManageMenuSheet<T>`) using the Experience scope as the consumer. Phase 2 ports the Request scope onto the same primitives. Phase 3 deletes old sheets and removes the flag.
- **Pros:** Smaller, reviewable PRs. Real-user signal on Experience scope before committing Request. Forces the "what's shared vs. what's scope-specific" decomposition early.
- **Cons:** Violates the "ship in lockstep" acceptance criterion for an interim period (probably 1–2 weeks). Two visual languages live side-by-side temporarily. The "Polls deferred work" closed item #1 in [`docs/client/polls.md`](../client/polls.md) is a cautionary tale: per-flow drift creeps in when shipping one side before the other.
- **Effort:** Large total, but each phase is medium.

### Option C: Chrome-first port, picker/kits/mark-ready deferred

Restyle the existing six sheets onto `GlassSheet` chrome without changing their structure or adding new capabilities (no Picker, no Kits, no Mark Ready). Ship a separate follow-up issue for the Picker/Kits/MarkReady additions.

- **Approach:** Keep the existing sheet inventory but rewrite each one to use `GlassSheet`, `GlassModalHeader`, `GlassFooterButtons`, etc. Same flow, same RPCs, new skin.
- **Pros:** Smallest, lowest-risk increment. Removes the editorial-chrome divergence today. Leaves the open questions about quantity units, saved kits, and Mark Ready for a follow-up where the product decisions can be made deliberately.
- **Cons:** Doesn't deliver the [`needs-redesign.html`](../cowork/App%20Design/needs-redesign.html) vision — that mock shows new *interactions* (typeahead picker, kits, mark-ready roster), not just a re-skin. Splits the work in a way that may produce throwaway code if the picker reorganizes the flow.
- **Effort:** Medium.

## Recommendation

**Option B**, with the parity risk explicitly mitigated by treating "shared widgets factored out as we go" as a non-negotiable per-PR contract — any sheet that diverges from the shared shape needs an "Intentional differences" entry in the redesigned docs the way [`docs/client/polls.md`](../client/polls.md) §Intentional-differences does.

Reasoning:

- **Option A is too big to land safely.** Six sheets × two scopes × new picker capability + new MarkReady surface in one PR is the kind of change that gets stuck in review. The poll migration ([#2131](https://github.com/Type2Fun-Foundation/ripls/issues/2131), tracked in [`docs/issues/2131-time-poll-migration.md`](2131-time-poll-migration.md)) is the reference here: it shipped scope-by-scope and was better for it.
- **Option C leaves the Picker on the floor.** The mock's most distinctive contribution — NK1–NK4 with the suggestion grid, kits, and quantity stepper — is genuinely new product, not chrome. A pure re-skin would be throwaway work.
- **Option B's lockstep risk is real but bounded.** The poll-deferred-work list shows what drift looks like in practice and how to prevent it (call out intentional differences, treat anything else as a bug). We'd apply the same contract here.

**What I'd want to know before committing:** (a) the product call on Mark Ready — pure client visualization vs. new server flag (see Open Questions), and (b) whether the quantity-unit on contributions (the "total/each" toggle in NK3/NK4) is in scope for the first release or a follow-up. If both are "client only / defer the unit toggle," Option B becomes substantially smaller and the case for it strengthens further.

## Cross-cutting conventions (audit guardrails)

These apply to every phase. Listed here so the per-phase task lists can stay focused.

- **Accessibility primitives ([`docs/client/accessibility.md`](../client/accessibility.md)) — the a11y allowlist is empty, so any new violation fails CI.**
  - Use `Tappable` (not `GestureDetector` / `InkWell`) for tap-to-claim rows on `NeedsClaimRow`; required `semanticsLabel` from `context.l10n`.
  - Use `Toggle` for the claimed-vs-idle row state (it's a selectable state, exactly the case `Toggle` is for) — `selected: true` when claimed, not "claimed" baked into the label.
  - Open every new sheet via `showAccessibleModal`, never raw `showModalBottomSheet`.
  - Use `ModalHeader` (already wraps title in `Semantics(header: true)`) for the serif sheet titles.
  - Use `IconAction` (with `tooltip` + `semanticsLabel`) for close buttons and the Manage-menu trigger; no raw `IconButton`.
  - Wrap any sage-fill / settle animations with `accessibleDuration(context, dur)` so reduce-motion disables them.
  - On successful autosave-claim, announce the state change via `SemanticAnnouncer.announce(context, ...)` (parallels the poll vote modal's "YOU'RE IN" eyebrow being screen-reader–perceivable).
  - Any new `*Dot` / `*Indicator` / `*Badge` widget (e.g. the amber "uncovered" flag in NX1) needs a `Semantics(` or `Tooltip(` — the `avoid_color_only_status` rule will fail otherwise.
- **i18n parity ([`docs/client/i18n.md`](../client/i18n.md)) — `npm run lint:dart:l10n-parity` fails on any key present in one ARB but missing in the other.**
  - Every new user-visible string (sheet titles, button labels, empty-state copy, snackbar/toast text, suggestion-grid category labels) needs a key added to **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` with matching `@key` metadata. English placeholders are acceptable when no Spanish translation is ready — parity is the gate, not translation quality.
  - Use the `needs*` namespace prefix for non-a11y strings (mirrors `gear*`, `experience*`); use the `a11y*` prefix for every semantics label, tooltip, and `Semantics(...)` argument.
  - Per the [UI Terminology rule in CLAUDE.md](../../CLAUDE.md#ui-terminology-event-not-experience): user-facing strings for Experience scope say **"Event"**, not "Experience". Code identifiers stay `Experience`.
  - ViewModels never resolve strings — keep error states typed (`UserError?`) and let widgets call `RpcErrorHandler.localize(state.error!, context.l10n)` in `build()`.
- **Theming ([`docs/client/architecture.md` §Colors and Theming](../client/architecture.md)) and the [`feedback_theme_reuse`](../../../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md) memory.**
  - The [`needs-redesign.html`](../cowork/App%20Design/needs-redesign.html) mock declares raw tokens (`t.sage`, `t.coral`, `t.amber`, `t.violet`, `t.catGearBg`, etc.). **Map every one to `AppColors.xxx(context)` helpers** — add new semantic helpers (e.g. `AppColors.suggestionGearBg(context)`) when a token genuinely doesn't fit existing ones; never hardcode hex into widgets. The mock's category-tinted suggestion grid (gear / food / help / personal) is the most likely new-helper site.
  - Use `AppTheme.headingFont` for the LibreBaskerville-style serif title — no per-section font constants. Do not create version-suffixed directories (no `widgets/needs/v2/`).
  - Every color helper must be theme-aware (light + dark mode); the mock's dark glass tint translates to `AppColors.cardBackground(context)` / the existing `GlassSheet` tinting, not a hardcoded `Color(0xFF141C1C)`.
- **Riverpod async safety ([`docs/client/architecture.md` §Async Safety in Notifiers](../client/architecture.md)).**
  - `experienceNeedsProvider` and `requestNeedsProvider` are already `autoDispose.family`. The new autosave-on-tap path runs through their existing notifiers — no new manual cache maps in any sheet widget. Every `await` inside an existing notifier method must be followed by an `if (!ref.mounted) return;` guard before assigning to `state` (or migrate to `AsyncNotifier`/`SafeNotifierMixin` if the touch is large enough to justify).
  - Sheet `show(...)` static helpers that return a `Future<T?>` are an accepted "widget async exception" — the calling site adds a single `if (!context.mounted) return;` guard between the await and any subsequent `Navigator` / `ref.read` call, matching the pattern in `NeedsActions.onNeedTap` today.
- **Navigation.** The full-screen Picker should ship as a modal (`showAccessibleModal`), not a pushed route — keeps it consistent with `LocationPickerModal` / `DateTimePickerModal`. If product later wants it as a screen, it must be routed via `NavigationHelpers.pushScreen()` with `SwipeToCloseMixin`, not `Navigator.push(MaterialPageRoute(...))` — `npm run lint:nav` enforces this.
- **File size.** `npm run lint:dart:size` caps non-generated Dart files under `app/lib/` at 1000 lines. The current `_sheet_chrome.dart` (585 lines) and `needs_section.dart` (708 lines) are both within budget; the redesign should not push any new file over. If a sheet starts approaching 1000 lines, split before merging — do not add the `// dart-line-count-allow` hatch without an issue.
- **TODOs.** Any TODO left in the new code must be `TODO(#NNNN):` with an open tracked issue (CLAUDE.md §TODO Comments). The deferred items in this plan (kits, quantity unit, server "needs locked", Nudge RPC) need GitHub issues filed before the corresponding TODO is added in code.

## Open Questions

- **Mark Ready: client visualization or new server state?** — Recommendation: **Client visualization for now.** The simplest mapping is "Mark Ready" surfaces only when the parent entity is in a terminal state (Experience COMPLETED, Request FULFILLED) and the affordance to *enter* that state is the parent's existing complete/fulfill flow. Avoid adding a `needs_locked` flag until we see real demand. If demand emerges, add it as a follow-up issue.
- **Quantity unit on Contributions (NK3 total/each toggle).** — Recommendation: **Defer.** Current `Need.slots` (1–20) is per-claim, not per-unit. Adding "I'm bringing 4 of these (total)" vs. "I'm bringing 4 each" requires a new optional proto field on `Contribution` (`quantity_unit` enum: TOTAL/EACH) and UI logic to render the rollup correctly on the Finalized roster. Ship the redesign with the existing slot semantics first; track the quantity-unit feature as a follow-up so we don't conflate the chrome migration with a data-model change.
- **Saved kits (NK1 mention).** — Recommendation: **Defer to follow-up.** Kits are a meaningful new persistence concept (per-user, scoped how? per-community?). Show the suggestion grid in the picker without the kit row in v1.
- **Suggestion grid source.** — Recommendation: **Reuse the existing suggestion chip data path.** [`docs/planning.md`](../planning.md) already references AI-generated suggestion chips for empty states. The NK1 grid can ride the same provider; we're rendering them as a grid instead of a chip row.
- **Where does the new shared widget directory live?** — Recommendation: **Create `app/lib/presentation/widgets/needs/`** to mirror `app/lib/presentation/widgets/poll/`. Move `NeedsSection`/`NeedsActions` here from `widgets/shared/` as part of phase 1 cleanup. Each multi-file directory under `app/lib/` requires a README per [CLAUDE.md](../../CLAUDE.md) — write it as part of the move.
- **Feature flag yes or no?** — Recommendation: **No.** The poll migration didn't use one. We control the only callers (`NeedsActions`), can roll back via git, and the surfaces are not visible until the user opens the Plan tab — fast user-perceived rollback if needed.
- **Picker as a standalone modal vs. inline in the Planner?** — Recommendation: **Standalone modal**, opened from the Planner's "Add a thing" affordance and from the Volunteer flow's "Offer something" affordance. Matches the way `LocationPickerModal` and `DateTimePickerModal` work for the poll-propose surfaces, and keeps the Picker reusable across both scopes and both planner/volunteer entry points.

## Implementation Plan

### Phase 1: Foundation — shared widget primitives & directory move

**Goal:** Create the new `widgets/needs/` directory with the cross-cutting primitives the rest of the work will compose. No user-visible change yet; existing sheets continue to be used.

**Tasks:**
- [ ] Create `app/lib/presentation/widgets/needs/` and move `NeedsSection` and `NeedsActions` here from `widgets/shared/`. Use `git mv` to preserve history per CLAUDE.md. Update imports.
- [ ] Write `widgets/needs/README.md` per [CLAUDE.md](../../CLAUDE.md) conventions — capability-named, no vendor references. Describe the parity contract: surfaces consumed by both scopes, deviations require an entry in `docs/client/needs.md` §Intentional differences (file created in phase 5).
- [ ] Build `NeedsSheetChrome` — composes `GlassSheet` + `ModalHeader` + `GlassFooterButtons`. Opened via `showAccessibleModal`. All text via `context.l10n`; new ARB keys land in **both** `app_en.arb` and `app_es.arb` in the same PR (otherwise `lint:dart:l10n-parity` fails).
- [ ] Build `NeedsClaimRow` — tap-to-claim row with idle/claimed visual states. **Use `Toggle` (not `Tappable`)** because the row carries selectable state (`selected: true` when claimed); `Toggle`'s built-in `Semantics(selected:)` removes the need to encode state into the label. Inline `+`/`-` stepper for multi-slot, each as an `IconAction` with `tooltip` + `semanticsLabel` from `a11y*` ARB keys. Wrap the sage-fill settle animation in `accessibleDuration(context, ...)`. Pure widget; no provider coupling.
- [ ] Build `NeedsSuggestionGrid` — NK1 category-tinted suggestion grid. Add new theme-aware semantic helpers to `AppColors` for the four category tints (gear / food-drink / help / personal) rather than inlining hex; every helper accepts `context` so light + dark modes both work. Per the [`feedback_theme_reuse`](../../../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md) memory: extend `AppColors`, do not introduce per-section constants.
- [ ] Smoke-test the a11y allowlist baseline: run `npm run lint:dart:a11y` and `npm run lint:dart:i18n` after the move; both must still pass with zero new entries.

**Validates:** The primitives are scope-agnostic enough that both Experience and Request consumers can adopt them without per-scope branches.

### Phase 2: Experience scope port — Propose, Volunteer, Manage

**Goal:** Replace the Experience-scoped sheets that have a clean 1:1 with the new design. `NeedsActions` continues to work; just opens new sheets.

**Tasks:**
- [ ] Build `NeedsProposeSheet` (NP1–NP3) — bulk-draft + one-at-a-time entry, suggestion grid for empty state. Replaces `BatchAddNeedSheet`. User-visible copy uses "Event" not "Experience" per CLAUDE.md §UI Terminology.
- [ ] Build `NeedsPickerModal` (NK1–NK3) — search field, suggestion grid, simple slot stepper. NO total/each toggle in v1 (deferred). NO saved kits in v1 (deferred). Replaces the in-line entry fields of the current sheets. Opens via `showAccessibleModal`.
- [ ] Build `NeedsVolunteerSheet` (NV1–NV2) — wraps the existing experience-needs list view in `GlassSheet`. Reuses `NeedsClaimRow`. Tap-to-claim autosaves via the existing `experienceNeedsProvider.claimNeed` (no direct service calls from the sheet; all data access through the provider/repository per [`docs/client/caching.md`](../client/caching.md)). After a successful claim, announce the row's new state via `SemanticAnnouncer.announce(context, context.l10n.a11yNeedClaimed)`.
- [ ] Wire the organizer Manage menu to `PollManageMenuSheet<NeedsManageAction>` with rows for: Add a thing, Lock list, Mark ready (only when parent is terminal), Cancel needs. Reuse the existing pattern from [`location_poll_manage_menu.dart`](../../app/lib/presentation/screens/experience/location_poll_manage_menu.dart). ARB keys per row land in both locales.
- [ ] Update `NeedsActions.onNeedTap`, `claimNeed`, `addContribution`, etc. for the Experience branch only. Each `await showXxx(...)` followed by an `if (!context.mounted) return;` guard before any `ref.read` / `Navigator` call (widget async exception pattern, [`docs/client/architecture.md`](../client/architecture.md)).
- [ ] Audit `experienceNeedsProvider` for new `state = state.copyWith(...)` assignments introduced by the autosave path; every one preceded by `if (!ref.mounted) return;` since the notifier is `autoDispose.family`.
- [ ] **ViewModel-first tests** (per [`docs/client/testing.md`](../client/testing.md)): add `experience_needs_view_model_test.dart` coverage for the new state transitions — autosave-claim happy path, RSVP auto-enrollment when no RSVP exists, claim-on-zero-slots rejection, mutation error → `state.error` typed result. Use `ProviderContainer` + `@GenerateMocks([ExperienceNeedsService, ExperienceRepository, ...])` + `addTearDown(() => container.dispose())`. Skip widget tests for the sheets themselves — keep coverage at the viewmodel layer where the logic lives. Run with `flutter test --reporter expanded` per CLAUDE.md.

**Validates:** Multi-claim semantics survive the chrome change (tap a Need with remaining slots → atomic decrement → row reflects new count). RSVP auto-enrollment still fires for participants without an RSVP. The autosave path is disposal-safe under autoDispose.

### Phase 3: Request scope port

**Goal:** Bring the Request scope onto the same shared widgets. Both scopes now consume the same shape.

**Tasks:**
- [ ] Wire `NeedsProposeSheet`, `NeedsPickerModal`, `NeedsVolunteerSheet` for the `RequestNeedsScope` branch in `NeedsActions`. No new ARB keys expected here (the strings landed in phase 2); audit for any Request-specific copy and add to both locales if so.
- [ ] Adapt the Manage menu for Request semantics (only requester sees "Add a thing"; any member sees "Offer something"). Per-scope row eligibility lives in the calling site, not in the shared sheet.
- [ ] Replace `request_needs_sheets.dart` call sites; delete the file once unreferenced.
- [ ] **ViewModel tests** mirroring phase 2: `request_needs_view_model_test.dart` covers requester-only Need posting, any-member contribution, terminal-state mutation rejection. `ProviderContainer` + Mockito mocks. `flutter test --reporter expanded`.

**Validates:** `NeedsScope` pattern matching is the only place that needs scope-aware code; the sheets themselves know nothing about whether they're showing an Experience or a Request.

### Phase 4: Mark Ready & Finalized roster

**Goal:** Surface NX1 / NX2 — the roster view of who's bringing what, with the covered vs. still-needed callout.

**Tasks:**
- [ ] Build `NeedsFinalizedSheet` (NX1/NX2) — roster, covered/still-needed split, amber flag for uncovered Needs, "hasn't-claimed" nudge button. Read-only when the parent is terminal; editable otherwise. The amber uncovered indicator must include a `Semantics(...)` or `Tooltip(...)` describing the state ("uncovered need" → `a11yNeedUncovered` ARB key) — `avoid_color_only_status` will reject any `*Indicator`/`*Badge`/`*Dot` widget without one.
- [ ] Wire `NeedsActions` so that when the parent is terminal, tapping the Plan-tab card opens `NeedsFinalizedSheet` instead of `NeedsVolunteerSheet` (mirrors the poll state-aware dispatcher in [`experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart)).
- [ ] Implement the "Nudge" action by reusing or paralleling the `NudgeLocationPollVoters`/`NudgeTimePollVoters` server pattern. **Open question for this phase**: is there an existing nudge RPC for needs, or do we need a `NudgeUncoveredNeedClaimers` RPC? If the latter, this becomes a server change with these constraints:
  - Use `connecterr.Internal(ctx, op, err, kv...)` for any `connect.CodeInternal` return — bare `connect.NewError(connect.CodeInternal, err)` fails `scripts/check_codeinternal.js` in CI.
  - Structured logging per [`docs/server/observability.md`](../server/observability.md): `logging.LoggerWithContext(ctx).With("operation", "NudgeUncoveredNeedClaimers", "experience_id", expID, "need_id", needID, ...)`. Mask emails via `logging.MaskEmail()`. Use `target_request_id` (NOT `request_id`) for Request scope entity IDs to avoid collision with the HTTP correlation key. INFO level for the action; ERROR only for system failures like FCM delivery rejection.
  - Classify the new `CommunityEventType` (if one is emitted) in `server/undo/registry.go` before merge — `TestEveryEventTypeClassified` blocks otherwise.
- [ ] **ViewModel tests** for the state-aware dispatcher (terminal → Finalized; non-terminal → Volunteer; non-terminal with no Needs → Propose). Roster rendering is widget territory and skipped — the dispatcher decision is the testable logic.

**Validates:** The full lifecycle (propose → claim → manage → mark ready → finalized) is end-to-end navigable from the Plan-tab card for both scopes.

### Phase 5: Old-code cleanup & documentation

**Goal:** Remove the editorial-chrome sheets, document the parity contract, close the loop.

**Tasks:**
- [ ] Delete `app/lib/presentation/widgets/experience/needs/` sheet files (`view_need_sheet.dart`, `add_contribution_sheet.dart`, `batch_add_need_sheet.dart`, `batch_add_contribution_sheet.dart`, `edit_contribution_sheet.dart`, `view_contribution_sheet.dart`, `_sheet_chrome.dart`, `_shared_sheet_widgets.dart`, `needs_sheet_models.dart`). Move any still-used models into `widgets/needs/`.
- [ ] Delete `app/lib/presentation/widgets/request/request_needs_sheets.dart`.
- [ ] Write `docs/client/needs.md` modeled on [`docs/client/polls.md`](../client/polls.md) — lifecycle, surfaces table, scope dispatch, intentional differences list, deferred work (kits, quantity unit, server-side "needs locked" flag).
- [ ] Update [`docs/planning.md`](../planning.md) to point at `docs/client/needs.md` for the modal layer and trim the §Unified-modal-design section accordingly.
- [ ] Run the full Dart lint + test gate: `npm run lint:dart:size` (1000-line cap), `npm run lint:dart:a11y` (a11y allowlist must still be empty), `npm run lint:dart:i18n` (no raw user-visible strings), `npm run lint:dart:l10n-parity` (ARB key parity), `npm run lint:nav` (no raw `Navigator.push`), `npm run lint:ephemeral-comments` (no phase markers in source), then `flutter test --reporter expanded`. If the Nudge RPC landed in phase 4, also `npm run build` and `go build -tags=integration ./...` per CLAUDE.md.

**Validates:** No two visual languages for collaborative-planning surfaces in the app; new contributors have a single doc to read for the pattern.

## Related Issues

- #2131 — Time-poll migration; produced the parallel-structure pattern and the "Intentional differences" contract this work borrows. Plan doc at [`docs/issues/2131-time-poll-migration.md`](2131-time-poll-migration.md).
- #1802 — Migrate remaining bottom-sheet modals to glass surface. This issue is a child of that larger initiative; closing #2148 takes a meaningful bite out of #1802.
