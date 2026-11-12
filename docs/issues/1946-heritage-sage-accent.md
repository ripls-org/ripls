# Issue #1946: theme: migrate accent color from coral/orange to Heritage Sage (#5B8268)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1946
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** _none_ | **Assignees:** unassigned
**Context docs:** none

> **Audit pass — 2026-05-15.** Plan reviewed against [docs/client/architecture.md](../client/architecture.md) § "Colors and Theming", [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and [docs/client/i18n.md](../client/i18n.md). Server-side reference docs (architecture, observability, caching) intentionally not consulted — this migration has zero server, repository, viewmodel, cache, or RPC surface. Audit-driven changes are summarized in the "Audit notes" callouts below each affected section.

## Problem Statement

The app's accent color is currently coral/orange — `lightPrimary = #E07A5F` ("coral sunset") and `lightAccent = #E89B7E` ("light coral") in [app/lib/core/theme/app_colors.dart](app/lib/core/theme/app_colors.dart) — with a constellation of coral-derived semantic tokens (`transferCoral`, `transferCoralSoft`, `experienceCoral`, `ImpactModalColors.coral`, `warning`, `requestColor`) and a handful of hardcoded `Colors.orange*` / coral-hex literals scattered through widgets, utilities, and the router.

The product direction is to reskin to **Heritage Sage `#5B8268`** as the single accent. The intent is twofold:
1. Swap the brand color from warm coral to cool sage.
2. Fix the underlying maintenance problem — *every* accent-tinted pixel should resolve through `AppColors`, so the next reskin is a one-file change rather than a 25-file find-and-replace.

The audit in the issue body lists ~17 widget-side bypasses (`Colors.orange.shade400`, `Color(0xFFFF8A80)`, `Colors.orange`) plus several semantic-token clusters (`transferCoral*`, `experienceCoral`, `ImpactModalColors.coral`, `warning`, `requestColor`, `community_color.dart` rust-orange) whose treatment needs an explicit decision: do they shift to sage variants, do they keep coral as a domain-meaningful color, or are they merged into a new token?

There is also a non-obvious complication: `AppColors.modalPrimaryButtonBackground = lightPrimary` was deliberately set to the darker coral (not the lighter accent) to clear WCAG large-text 3:1 contrast on the frosted glass modal — see the comment at [app/lib/core/theme/app_colors.dart:676-680](app/lib/core/theme/app_colors.dart#L676-L680) and issue #1934. The new sage must be verified against the same contrast threshold; #5B8268 has lower luminance than #E07A5F, so white-on-sage should pass more easily, but it must be measured, not assumed.

> **Audit note (architectural fit).** This work is *exactly* the discipline described in [docs/client/architecture.md § "Colors and Theming"](../client/architecture.md): "All colors must be theme-aware … use `AppColors.xxx(context)` helpers … DON'T use hardcoded hex `Color(0xFF...)` values for UI elements — add a named helper to `AppColors` if a new semantic color is needed." The issue is closing existing gaps relative to that doc, not introducing a new pattern. Reference that section when reviewing PRs from this plan.

> **Audit note (collision risk — elevated).** The single biggest visual risk in this reskin is that Heritage Sage `#5B8268` lands in the same hue family as five *other* existing colors: `success = #7A9B76`, `experienceSageGreen = #7A9B8C`, `transferSage = #6B8F71`, `lightChatBubble = #7A9B76`, and the `success`-derived `availabilityGiveawayBackground`. After the swap, a user can see four-to-five distinct sages on a single screen (e.g. accent button + sage chat bubble + "Going" RSVP badge + giveaway pill + transfer success state). Phase 1 task 3 already lists this, but it warrants top-billing — if the palette can't be made visually distinct, the reskin needs a design pass before any code lands.

## Context from Comments

No comments on the issue yet — it was filed in the same session as this plan. The acceptance criteria, the scope inventory, and the optional CI guardrail were all defined up front in the issue body, so there's no maintainer-vs-author back-and-forth to reconcile. **All ambiguity therefore lives in the issue body itself** — primarily around which tokens stay coral, which shift to sage, and how the light/dark variants are picked. See Open Questions below.

## Potential Solutions

### Option A: Pure token swap — replace `lightPrimary`/`lightAccent`/`darkPrimary`/`darkAccent` and the coral semantic tokens with sage equivalents, keep token *names*
- **Approach:** Update the four core accent constants and the `transferCoral*` / `experienceCoral` / `ImpactModalColors.coral` values to sage variants in [app/lib/core/theme/app_colors.dart](app/lib/core/theme/app_colors.dart). Token *names* stay the same (still `transferCoral` etc.) — only the underlying `Color(0xFF...)` shifts. Then fix the ~17 widget-side bypasses to read from `AppColors` instead. The `chatBubble`, `success`, `experienceSageGreen`, `transferSage` greens — already sage — get re-tuned if they collide.
- **Pros:** Minimal blast radius. Existing call sites need no changes (a `transferCoral` reference at 128 call sites stays valid). Lands in one PR. Aligns with the issue's "ideally just app_colors" framing. Easy to revert.
- **Cons:** Token names become misleading (`transferCoral` is now sage). Future engineers reading `transferCoral` and seeing sage will be confused. Carries semantic debt forward.
- **Effort:** Small (1-2 days including visual QA).

### Option B: Rename tokens to neutral semantic names, then swap values
- **Approach:** Rename `transferCoral` → `transferAccent`, `experienceCoral` → `experienceAccent`, `ImpactModalColors.coral` → `ImpactModalColors.accent`, etc., across all 128+ call sites. Then update the underlying values to sage. Fix the widget bypasses in the same PR.
- **Pros:** Token names stay accurate after future reskins. No more "coral" vocabulary in the codebase. Better long-term hygiene.
- **Cons:** Touches 100+ files in a single PR — high review burden, high merge-conflict risk against active branches (`user-profile`, the impact_metrics work, etc.). The rename adds noise that obscures the actual visual change. Doesn't ship faster value than Option A.
- **Effort:** Medium (3-4 days including the rename diff churn and review).

### Option C: Option A now, Option B as a follow-up issue
- **Approach:** Ship Option A as the visible reskin. File a separate follow-up issue tracking the rename of `*Coral*` tokens to `*Accent*`. The CI guardrail (`Colors.orange` / coral-hex literal grep) lands with Option A and prevents new violations regardless of token names.
- **Pros:** Decouples the visible reskin (high product value, time-sensitive) from the codebase hygiene (low urgency, can land any time). Reviewers can focus on visual diff in PR 1, mechanical rename in PR 2. CI guardrail catches regressions in the gap.
- **Cons:** Two PRs instead of one. Risk that the follow-up rename never gets prioritized and the misleading names persist indefinitely.
- **Effort:** Small for PR 1, medium for PR 2 — but split across two ship cycles.

## Recommendation

**Option C.** The product asked for a reskin; deliver the reskin in one focused PR with a clean visual diff (Option A's scope) plus the CI guardrail to lock in the discipline. File a follow-up issue for the `*Coral*` → `*Accent*` rename so it doesn't get lost.

Two tradeoffs I'm explicit about:
1. **I'm prioritizing time-to-ship over name accuracy.** A `transferCoral` constant that resolves to sage is awkward but harmless. A 100-file rename PR landing against active branches is a real merge-conflict hazard right now (the user is on `user-profile`; impact work is live).
2. **The CI guardrail is the load-bearing piece.** Without it, the next engineer to hardcode `Colors.orange.shade400` undoes half this work. With it, the misleading token names are the only remaining hygiene issue — and that's a low-stakes rename anyone can do at leisure.

If the user pushes back on the misleading names, fall back to Option B and accept the review cost.

## Open Questions

Each question below is something the implementer needs answered (by the user or by a design decision) before starting. I've included a recommendation as a starting point.

- **Which sage variants for light vs. dark mode?** Heritage Sage `#5B8268` is one value but the theme needs four (`lightPrimary`, `lightAccent`, `darkPrimary`, `darkAccent`) plus a gradient pair. — **Recommendation:** Use `#5B8268` as `lightPrimary` and a lighter shade (e.g. `#7BA088` ≈ +20% L*) as `lightAccent`; in dark mode invert (`darkPrimary = #7BA088`, `darkAccent = #5B8268`) to mirror the current coral pattern in [app/lib/core/theme/app_colors.dart:29-33](app/lib/core/theme/app_colors.dart#L29-L33). The exact lighter shade should come from the `accent-color-variations-greens.html` concept the user is iterating on — that doc isn't in the repo yet, so confirm the values from there before writing them into `app_colors.dart`.

- **Does `warning` / `requestColor` (currently `#E8A661` warm orange) shift to sage?** — **Recommendation:** No. `warning` is a semantic status color meaning "attention needed"; in interaction-design conventions warning ≠ brand accent. Keep `warning` as warm orange. This aligns with [docs/client/architecture.md § Colors and Theming](../client/architecture.md): "Use `AppColors` semantic colors (`success`, `warning`, `error`, `info`) for status indicators" — they're an explicit, distinct axis from brand color. For `requestColor`, the comment "warm orange" suggests it was deliberately distinguished from giveaway (sage) and loan (stone blue). Keep it warm orange too — otherwise requests and giveaways collide. If product wants requests to use the brand accent, that's a separate decision and should be raised explicitly.

- **Do `transferCoral` / `transferCoralSoft` / `experienceCoral` / `ImpactModalColors.coral` shift to sage?** — **Recommendation:** Yes, all of them. These tokens are *named* coral but they're functionally "the accent color in the transfer/experience/impact surfaces." After the reskin the accent is sage, so they shift. The token names get renamed in the follow-up issue (Option B).

- **Does `transferSage` (#6B8F71) need to change?** It's currently used alongside `transferCoral` to mean "completion / success" in the transfer modal. If `transferCoral` shifts to sage too, the two greens will collide. — **Recommendation:** Move `transferSage` toward a deeper or more saturated green (or keep the existing `success = #7A9B76`) to maintain separation from the new Heritage Sage accent. Verify visually in the transfer flow.

- **Does the rust-orange community palette slot (`#A36D3A` in [app/lib/core/utils/community_color.dart:12](app/lib/core/utils/community_color.dart#L12)) need to change?** It's one of four deterministic per-community colors. — **Recommendation:** Keep it. Per-community colors are a separate palette from the brand accent; they exist precisely to *not* be the brand color. Rust-orange isn't on a collision course with sage.

- **Does the CI guardrail ship in the same PR or separately?** — **Recommendation:** Same PR. Write `scripts/check_no_hardcoded_accent.js` modeled on [scripts/check_ephemeral_comments.js](scripts/check_ephemeral_comments.js), wire it into `npm run lint:dart` (or a new `lint:colors` script invoked by `npm run build`), and commit it alongside the migration. Otherwise the next sloppy widget reintroduces hardcoded coral before the guardrail lands.

- **What does the guardrail actually forbid?** Naive `Colors.orange` ban is easy. Coral hex literals are harder — `Color(0xFFE07A5F)` is the obvious one, but the issue lists 6+ shades. — **Recommendation:** Forbid (a) `Colors.orange`, `Colors.deepOrange`, `Colors.amber` outside `app_colors.dart`, and (b) a small allowlist of coral hex constants (`0xFFE07A5F`, `0xFFE89B7E`, `0xFFD4714E`, `0xFFD97052`, `0xFFD35E3A`, `0xFFE8A661`, `0xFFFF8A80`) outside `app/lib/core/theme/`. Don't try to be exhaustive about all "orange-ish" colors — that's a rabbit hole. Catch the known offenders.

- **Does the WCAG contrast on `modalPrimaryButtonBackground` actually pass?** The current comment in [app/lib/core/theme/app_colors.dart:676-680](app/lib/core/theme/app_colors.dart#L676-L680) is specific that coral was chosen for the 3:1 large-text threshold. — **Recommendation:** Heritage Sage `#5B8268` is L*≈51, white is L*=100 → contrast ratio ≈ 4.4:1, which clears both 3:1 (large) and the stricter 4.5:1 normal-text threshold by a hair. Verify with a contrast checker before merging and update the comment to cite the new ratio and sage hex value. The lighter `#7BA088` would *not* pass 4.5:1, which is why `modalPrimaryButtonBackground` must stay bound to `lightPrimary` (the darker shade).

- **Does `AppTheme.heroMetricLabelStyle` need re-thinking?** At [app/lib/core/theme/app_theme.dart:75](app/lib/core/theme/app_theme.dart#L75) the const text style hard-codes `color: AppColors.lightPrimary` — it is NOT theme-aware. With coral it was visible against both light and dark hero headers because coral is mid-luminance. Heritage Sage `#5B8268` is darker (L*≈51) and may have insufficient contrast on the dark hero background. — **Recommendation:** Convert `heroMetricLabelStyle` from a `static const` to a theme-aware getter `heroMetricLabelStyle(BuildContext context)` that picks `lightPrimary` for light hero contexts and `lightAccent` (or a near-white) for dark hero contexts. Touch every call site in [app/lib/presentation/screens/impact_metrics/](app/lib/presentation/screens/impact_metrics/) in the same PR. This is a small but real refactor the issue body didn't anticipate.

- **Do the accessibility primitives (`Tappable`, `Toggle`, `IconAction`, `ModalHeader`) bake in any accent colors?** Per [docs/client/accessibility.md](../client/accessibility.md), interactive widgets are funneled through these primitives. — **Recommendation:** Grep `app/lib/presentation/widgets/accessibility/` for `AppColors.` and any hex literals before Phase 3 to confirm they read through theme tokens. If a primitive hard-codes coral (e.g. a focus ring or selected-state indicator), it needs the same treatment as the widget bypasses — otherwise the swap will visually miss the tap targets that matter most.

- **Should this reskin introduce golden tests for theme-sensitive screens?** [docs/client/testing.md § Visual Testing](../client/testing.md) lists "Test dark mode and theme variations" / "Add golden tests for key UI components" as a *Next Steps* recommendation; none exist today. — **Recommendation:** Out of scope for #1946. A reskin where the *only* change is `AppColors` values is the cheapest possible moment to add a golden test (the diff is exactly the intended change), but adding the golden infrastructure is a meaningfully separate workstream and would balloon this PR. File a follow-up issue tagged with this one.

- **Does the existing concept document `accent-color-variations-greens.html` exist?** The issue body links to it but the actual file in the repo is `accent-color-variations.html` (singular). — **Recommendation:** This is the user's working document, presumably being created/forked. Confirm with the user whether the implementer should reference the existing `.html` or wait for the `-greens` variant. Either way, update the issue body link before merging the reskin.

- **Update concept doc to reflect chosen sage variants?** Acceptance criterion 5 in the issue body says yes. — **Recommendation:** After picking the four sage shades, write them into the concept doc as the "shipped" variant so future designers can see what was actually chosen vs. what was explored.

## Implementation Plan

### Phase 1: Choose the palette and prove the contrast
**Goal:** Lock in the four sage values + the gradient pair, and verify they pass accessibility and don't collide with existing greens.
**Tasks:**
- [ ] Pick `lightPrimary`, `lightAccent`, `darkPrimary`, `darkAccent`, `lightGradient`, `darkGradient` values anchored on Heritage Sage `#5B8268`.
- [ ] Run a contrast check: white-on-`lightPrimary` ≥ 4.5:1 for normal text; verify large-text threshold for modal button.
- [ ] Lay the new sage values next to `success = #7A9B76`, `experienceSageGreen = #7A9B8C`, `transferSage = #6B8F71`, `lightChatBubble = #7A9B76`, `transferSageBackground` in a swatch test — confirm each is visually distinct or merge them.
- [ ] Either update `accent-color-variations.html` (or the `-greens` variant if it lands) to reflect the chosen values, or note the gap for follow-up.
**Validates:** That the reskin is visually and accessibly sound before any code changes ship.

### Phase 2: Swap tokens in `AppColors` + `AppTheme` + verify ripple
**Goal:** Land the one-file change in [app/lib/core/theme/app_colors.dart](app/lib/core/theme/app_colors.dart) and confirm Material's `ColorScheme.primary`, FAB, ElevatedButton, OutlinedButton, BottomNav, DatePicker, TimePicker — every consumer wired through `lightPrimary`/`darkPrimary` in [app/lib/core/theme/app_theme.dart](app/lib/core/theme/app_theme.dart) — picks up sage automatically.
**Tasks:**
- [ ] Update `lightPrimary`, `lightAccent`, `darkPrimary`, `darkAccent`, `lightGradient`, `darkGradient` to Phase 1 values.
- [ ] Update `transferCoral`, `transferCoralSoft`, `experienceCoral`, `ImpactModalColors.coral`, `ImpactModalColors.coralBg` (or decide explicitly per Open Questions to keep some as coral).
- [ ] Update the `modalPrimaryButtonBackground` comment block ([app_colors.dart:676-680](app/lib/core/theme/app_colors.dart#L676-L680)) to cite the new sage hex and contrast ratio.
- [ ] Convert `AppTheme.heroMetricLabelStyle` from `static const` to a theme-aware getter (see Open Questions) and update all call sites in [app/lib/presentation/screens/impact_metrics/](app/lib/presentation/screens/impact_metrics/).
- [ ] Audit [app/lib/presentation/widgets/accessibility/](app/lib/presentation/widgets/accessibility/) primitives (`Tappable`, `Toggle`, `IconAction`, `ModalHeader`, etc.) for any hardcoded accent — fix in this PR if found, since these primitives carry the most-touched tap surfaces.
- [ ] Boot the app on both iOS simulator and Android emulator (per [.claude/skills/ripls-app-logs](.claude/skills/ripls-app-logs)) and visually walk: gear list, FAB, primary buttons, RSVP buttons (light + dark), transfer modal, experience modal, impact metrics hero header (light + dark), leaderboard, chat input bar + chat bubble (verify sage accent ≠ sage chat bubble), login/register, plus button modal.
**Validates:** That the token swap alone delivers ~80% of the visible reskin — and surfaces any widget that bypassed `AppColors`.

> **Audit note (testing scope).** No viewmodel, repository, or service code changes in this phase, so the testing patterns described in [docs/client/testing.md](../client/testing.md) (ProviderContainer + Mockito + ViewModel-first) do not directly apply. The validation here is *visual* — light and dark mode, both platforms. If a screen needs to react differently to theme brightness and currently doesn't, that's a real ViewModel-shape bug that *would* warrant a unit test. Don't preemptively add one.

### Phase 3: Fix widget bypasses
**Goal:** Route every site in the issue audit through `AppColors`.
**Tasks:**
- [ ] Replace `Colors.orange*` in `community_event_helper.dart`, `value_helpers.dart`, `app_router.dart`, `register_screen.dart`, `preview_time_selector.dart`, `gear_preview_modal.dart`, `leaderboard_list_item.dart`.
- [ ] Replace hardcoded `Color(0xFFFF8A80)` (and any other coral hex) in `mark_completed_modal.dart`, `past_transfer_modal.dart`, `mark_fulfilled_modal.dart`.
- [ ] Update misleading "orange means …" comments in `experience_content_view.dart`, `gear_content_view.dart`, `request_content_view.dart`, `unified_item_data.dart`, `impact_metrics/metric_detail_*.dart`. Either remove or update the wording so the comment matches the new color.
- [ ] Decide on `community_color.dart` rust-orange (per Open Questions — recommendation: keep).
- [ ] Run `grep -rni "orange\\|coral" app/lib --include="*.dart"` and audit the remaining hits — every one should be either in `app_colors.dart`, the `success/warning/requestColor` semantic tokens we explicitly kept, or `community_color.dart`.
**Validates:** That `AppColors` is now the single source of truth for accent-tinted pixels.

### Phase 4: Lock in with CI guardrail
**Goal:** Make sure the next sloppy hardcode is caught in PR review automatically.
**Tasks:**
- [ ] Add `scripts/check_no_hardcoded_accent.js` modeled on [scripts/check_ephemeral_comments.js](../../scripts/check_ephemeral_comments.js) (small Node script, no deps, walks `app/lib/`, regex-based). For parity with existing color/a11y checks see also [scripts/check_a11y.js](../../scripts/check_a11y.js).
- [ ] Forbid `Colors.orange`, `Colors.deepOrange`, `Colors.amber` and the allowlist of coral/sage hex literals outside the theme directory.
- [ ] Follow the [docs/client/accessibility.md § Lint rules](../client/accessibility.md) pattern: include a small allowlist file (`scripts/color_allowlist.txt`) seeded empty, with a "ratchet" rule — entries may shrink but never grow on `main`. This gives hotfix room without weakening the guardrail.
- [ ] Add a smoke test for the script per CLAUDE.md ("always add unit tests for each new library or significant library function"): a tiny fixture Dart file in `scripts/test_fixtures/` that should trip the rule, plus a make-it-pass fixture. Wire as `npm run test:scripts` if not already present.
- [ ] Wire into `package.json` as `lint:colors` and add to the `lint` and `build` scripts.
- [ ] Add a CI job line in [.github/workflows/test_flutter.yaml](../../.github/workflows/test_flutter.yaml) (same pattern as `lint:dart:size`).
- [ ] Add a brief section to [app/lib/core/theme/README.md](../../app/lib/core/theme/README.md) explaining the guardrail and pointing to it (CLAUDE.md requires every multi-file dir under `app/lib/` to have a README — this is an update, not a new file).
**Validates:** That regressions are caught mechanically, not by eyeball.

### Phase 5 (follow-up issue): Rename `*Coral*` tokens to `*Accent*`
**Goal:** Resolve the token-name vs. value mismatch left over from Option C.
**Tasks:** _(Tracked in a separate issue; this phase is not part of #1946.)_
- [ ] File new issue: "theme: rename `transferCoral*`, `experienceCoral`, `ImpactModalColors.coral` to neutral `*Accent` names".
- [ ] Mention in the new issue that it's the follow-up to #1946.

## Related Issues
- #1934 — original WCAG contrast work on `modalPrimaryButtonBackground`; the new sage value must preserve that contrast guarantee.
- #1929 — "Update user profile screen to match V4-A concept design" — currently in flight on the `user-profile` branch; the reskin should land *after* the user-profile work merges, or both PRs need to coordinate on `AppColors` changes.

## Audit Summary

The plan is consistent with the project's established architectural patterns. Specific compliance points:

- **Architecture ([docs/client/architecture.md](../client/architecture.md)).** This work is the canonical example of the doc's "Colors and Theming" guidance — moving hardcoded `Color(0xFF...)` literals and `Colors.orange*` references behind `AppColors` helpers. No viewmodel/repository/service layers are touched, which is correct for a theme-only change.
- **Accessibility ([docs/client/accessibility.md](../client/accessibility.md)).** The plan now explicitly audits `app/lib/presentation/widgets/accessibility/` primitives (Phase 2) and follows the "ratchet allowlist" pattern from `check_a11y.js` for the new color guardrail (Phase 4). WCAG contrast verification on `modalPrimaryButtonBackground` and `heroMetricLabelStyle` is called out.
- **Testing ([docs/client/testing.md](../client/testing.md)).** No ViewModel-side tests apply (no business logic changes). A smoke test for the new lint script is added per CLAUDE.md. A follow-up note about golden tests is recorded in Open Questions, scoped *out* of this PR.
- **i18n ([docs/client/i18n.md](../client/i18n.md)).** No user-visible strings change; the "orange means …" updates are to *code comments*, not ARB strings. No ARB additions required.
- **Server boundary.** No server, RPC, proto, or observability changes — this is a pure-client reskin. Server reference docs intentionally not consulted.

The audit elevated the sage-on-sage collision risk to the Problem Statement and added one previously-missed item — the non-theme-aware `AppTheme.heroMetricLabelStyle` const — to Open Questions and Phase 2.
