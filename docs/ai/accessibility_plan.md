# Accessibility Plan — Ripls Flutter App

**Status**: Draft — 2026-04-26
**Source review**: `docs/reviews/reports/accessibility_review.md` (2026-04-24)
**Branch**: `accessibility_plan`

## Goals

Improve accessibility *proactively* and *durably* — the bar is not a one-time audit cleanup but a codebase where new features ship accessible by default. This plan optimizes for:

1. **Automatic enforcement over manual passes.** We add features rapidly and will not stop to do separate accessibility passes. Static analysis, lint rules, and base-widget primitives must make the accessible path the default path.
2. **Localization as a first-class accessibility concern.** Every user-visible semantic label, tooltip, and announcement is localized via `context.l10n`. We will not accept English-only labels — l10n is foundational to our geographic expansion strategy and inseparable from accessibility quality.
3. **Leverage-first sequencing.** Single-widget changes that propagate to dozens of call sites (e.g., `CachedMediaImage` adding a `semanticsLabel` param, `ModalHeader` adding `header: true`) ship before per-feature sweeps. We pay down the highest-multiplier debt first.
4. **No new violations.** Once enforcement is in place (Phase 2), CI rejects PRs that introduce raw `GestureDetector` / `InkWell` without `Semantics`, hardcoded English strings on user-visible labels, or icon-only buttons without semantic labels.

We are explicitly *deferring* manual VoiceOver/TalkBack testing and external audits. Static analysis + Flutter's semantics tree is sufficient signal for the leverage we want now; we revisit live-tester validation at the end of Phase 6 once the codebase is in shape to benefit from it.

## WCAG 2.1: Level A vs. Level AA — what each gets us

WCAG 2.1 has three conformance levels (A, AA, AAA), each cumulative. Most regulatory frameworks and app-store accessibility standards anchor on AA.

**Level A** — the *minimum* for a usable app. Failures here mean the app is unusable for some users, not just suboptimal.
- 1.1.1 Non-text Content: every image, icon, chart needs a text alternative
- 1.3.1 Info and Relationships: structure (headings, lists, form labels) is conveyed semantically, not just visually
- 1.4.1 Use of Color: color is never the *only* way information is conveyed (relevant: `ConfidenceDot`, status indicators)
- 2.1.1 Keyboard: all functionality available via keyboard
- 2.4.3 Focus Order: focus moves in a logical order (relevant: modal dismissal)
- 3.3.1 Error Identification: errors are programmatically identified and announced
- 4.1.2 Name, Role, Value: every interactive element has a name, role, and current value (relevant: 174 raw `GestureDetector` files)

**Level AA** — the *standard* bar for production apps. Adds:
- 1.4.3 Contrast (Minimum): 4.5:1 for normal text, 3:1 for large text (relevant: `SectionHeader` 10px low-contrast)
- 1.4.5 Images of Text: avoid baking text into images
- 2.4.6 Headings and Labels: descriptive headings/labels (relevant: `ModalHeader` lacking `header: true`)
- 2.5.5 Target Size (Enhanced) — at AAA, but informally treated as a baseline by mobile platforms (relevant: 24dp dismiss buttons)
- 3.3.3/3.3.4: Error suggestion + error prevention for legal/financial submissions
- 4.1.3 Status Messages: dynamic content announces to assistive tech (relevant: `liveRegion` gap, `ConnectivityBanner`, form errors)

**Level AAA** — aspirational, often impossible for some content. We do not target AAA explicitly but pick up specific AAA criteria where cheap (e.g., 2.3.3 reduced motion).

**Our practical target**: full Level A within ~6 months of plan execution; full Level AA within ~12 months. We will not formally certify against either — the goal is the underlying experience improvement, not a compliance badge.

## Strategy

The plan is structured into **six phases**, each sized at 1–2 weeks of focused work for one engineer. Phases 1–2 are foundational and must land before later phases for maximum effect. Phases 3–6 are largely parallelizable once the foundation is in place.

```
Phase 1: Foundation primitives (1-2 wks) ────┐
                                              ├─→ Phase 3: Auth & status (1 wk)
Phase 2: Enforcement infrastructure (2 wks) ──┤   Phase 4: Transfer feature (1-2 wks)
                                              ├─→ Phase 5: Impact Metrics & charts (2 wks)
                                              └─→ Phase 6: Long tail & polish (1-2 wks)
```

### Phase 1 — Foundation primitives (1–2 weeks)

Build the small set of accessible base widgets and helpers that subsequent phases will adopt. Until these exist, every feature engineer reinvents the wheel inconsistently.

**Deliverables**:

1. **`CachedMediaImage` `semanticsLabel` parameter.** Add `final String? semanticsLabel` and wrap output in `Semantics(label:, image: true, ...)`. This is the single highest-leverage change in the review — it propagates to 20+ caller files.
2. **`Tappable` widget.** A drop-in replacement for `GestureDetector` + `InkWell` patterns that *requires* `semanticsLabel` (non-nullable, must come from `l10n`) and accepts `selected`, `enabled`, `button`/`link` role hints. Subsequent phases migrate raw GD/InkWell sites to this. Accessibility is the default in our codebase, so the name is unprefixed.
3. **`Toggle` widget.** Variant of `Tappable` for segmented-button / chip patterns; requires `selected`. Targets RSVP buttons, InputModeToggle, period togglers, filter chips.
4. **`IconAction` widget.** Wraps Flutter's `IconButton` with required `semanticsLabel` and `tooltip` (both from `l10n`). Named to avoid colliding with `IconButton` in the Flutter namespace. Targets the password-visibility toggle, modal close buttons, app-bar back buttons, chat button.
5. **`ModalHeader` semantic header.** Add `Semantics(header: true)` to the existing widget. Propagates to all modals.
6. **`SemanticAnnouncer` service.** A thin wrapper over `SemanticsService.announce` and a `LiveRegion` widget that wraps content. Used by Phase 3 for connectivity banner, error banners, loading completions.
7. **`accessibleDuration(BuildContext, Duration)` utility.** Returns `Duration.zero` when `MediaQuery.disableAnimations` is true. Adopted by Phase 6 across the 45 animation files.
8. **`showAccessibleModal<T>` utility.** Wraps `showModalBottomSheet` to restore focus on dismissal. Adopted by Phase 6 across 86 modal sites.
9. **Localization keys for common patterns.** Seed `app_localizations.arb` with: `closeButton`, `backButton`, `showPassword`, `hidePassword`, `loading`, `errorOccurred`, `connectivityOffline`, `selected`, `notSelected`, plus per-status labels we already need (`avatarBadgeGoing`, etc.).

**Acceptance**: Each primitive ships with widget tests asserting Semantics tree shape, and a short README in `app/lib/presentation/widgets/accessibility/` documenting when to use which primitive. No callers migrated yet — that's Phases 3–6.

### Phase 2 — Enforcement infrastructure (2 weeks)

Make accessibility violations a CI failure, not a code-review request. This is the load-bearing phase: without it, Phases 3–6 fix the existing problems but new ones accumulate.

**Deliverables**:

1. **Lint script** at `scripts/check_a11y.js` (Node, matching the existing `check_i18n.js` and `check_dart_file_size.sh` pattern) with rules:
   - `avoid_raw_gesture_detector` — flags `GestureDetector` and `InkWell` with `onTap` outside `widgets/accessibility/`.
   - `require_semantic_label_on_icon_button` — flags `IconButton` without `tooltip:`.
   - `require_l10n_for_semantic_labels` — flags string literals passed to `tooltip:`, `semanticLabel:`, or arguments inside a `Semantics(...)` constructor. This is the primary l10n enforcement mechanism.
   - `require_semantic_label_on_cached_media_image` — flags `CachedMediaImage(...)` without `semanticsLabel`.
   - `avoid_color_only_status` — heuristic: flags widgets named `*Dot`, `*Indicator`, `*Badge` whose file has no `Semantics(` or `Tooltip(`.
   - `prefer_accessible_modal` — flags raw `showModalBottomSheet` calls outside of the `showAccessibleModal` utility.
   - `prefer_accessible_duration` — flags `Duration` literals passed as `duration:` arguments not wrapped in `accessibleDuration(context, ...)`.
2. **CI wiring.** `npm run lint:dart:a11y` runs the script; added to the existing `lint` aggregate and to the Flutter Tests workflow.
3. **Baseline allowlist** at `scripts/a11y_allowlist.txt`. Each entry is `<rule_name> <file_path>`, so a file is exempted from the named rule (not all rules). The allowlist may shrink as Phases 3–6 fix files but never grow on `main`; CI enforces this by comparing the entry count on the PR head to the count on the base ref.
4. **PR template addition** at `.github/PULL_REQUEST_TEMPLATE.md` — short `## Accessibility` section with a checkbox.
5. **Developer docs** at `docs/client/accessibility.md` covering: when to use which primitive, how to write good `semanticsLabel` text, how to add new lint rules, how to remove a file from the allowlist. The `a11y` l10n key prefix is established here.

**Acceptance**: a PR adding a raw `GestureDetector` with `onTap` fails CI. A PR using `tooltip: 'Close'` (English literal) fails CI and recommends the equivalent l10n key.

**Implementation note**: the original plan called for a `custom_lint` Dart package. The repo already has a precedent of small Node lint scripts wired into npm and CI; matching that pattern keeps Phase 2 lightweight without losing any of the enforcement it requires.

### Phase 3 — Auth flow & global status (1 week)

Highest-impact, lowest-effort user-flow remediation. Auth is the gate to the entire app; a screen-reader user blocked here cannot use anything else.

**Deliverables**:

1. **Form errors as live regions** in `login_screen.dart`, `register_screen.dart`, `content_error_banner.dart`, `connectivity_banner.dart` — wrap with `SemanticAnnouncer.LiveRegion` (Phase 1). Removes the 142-day-old critical issue.
2. **Password visibility toggles** migrated to `AccessibleIconButton` with l10n'd `showPassword`/`hidePassword`. Removes the 45-day-old high issue.
3. **Loading-completion announcements.** `SemanticAnnouncer.announce(l10n.loaded)` after auth-related `CircularProgressIndicator` resolves. Pattern documented for reuse by other features.
4. **Touch-target audit on auth screens.** Any sub-48dp interactive element in auth flow brought to 48dp minimum (form submit, social-auth buttons, the "show password" toggle).
5. **Allowlist shrink.** Auth screens removed from Phase-2 allowlist; CI now enforces accessibility on auth permanently.

**Acceptance**: Auth flow passes the lint suite without allowlist entries. Manual smoke test: error states announce when triggered (verifiable via `showSemanticsDebugger: true`).

### Phase 4 — Transfer feature (1–2 weeks)

The 27-widget Transfer library is a primary app workflow with zero current coverage. After Phase 1's primitives exist, this is mostly migration work.

**Deliverables**:

1. **`ChatButton`** → `AccessibleIconButton` with l10n `openConversation` label.
2. **`TransferPhaseBottomBar` buttons** — already `ElevatedButton`/`OutlinedButton` so role is correct, but ensure all states (disabled, loading) have l10n labels.
3. **`TransferModalShell`** loading state — `SemanticAnnouncer.announce(l10n.loadingTransfer)` when shown; `Semantics(label: ...)` on the progress indicator.
4. **`TransferModalHeader` / `TransferModalTitle`** — adopt `ModalHeader` (now header-tagged from Phase 1).
5. **`TransferProgressBar`** — `Semantics(value: '${current} of ${total}')` with progress role.
6. **All transfer-feature `GestureDetector`/`InkWell` sites** migrated to `AccessibleTappable`.
7. **Allowlist shrink** for `app/lib/presentation/widgets/transfer/` and related screens.

**Acceptance**: `app/lib/presentation/widgets/transfer/` contains zero allowlist entries. Lint enforces on all transfer code.

### Phase 5 — Impact Metrics, charts & color-only indicators (2 weeks)

The largest single remediation surface: 45 widgets and 18 screens with zero coverage, plus the chart and color-only-information patterns introduced here.

**Deliverables**:

1. **`ImpactMetricCard`, `ImpactTabBar._Tab`, `PeriodToggleBar._PeriodButton`, `SharingImpactCard._MetricTile`** migrated to `Tappable` / `Toggle` with l10n labels composed from value + unit (e.g., `l10n.impactMetricCardLabel(amount, unit)`).
2. **`LoanHistoryCard`** expandable rows expose `Semantics(expanded: ...)`.
3. **`ConfidenceDot`** wrapped in `Semantics(label: l10n.confidenceHigh/Moderate/Low)`. Screen-reader label only — no visual change.
4. **Chart widgets** (`AreaChartWidget`, `BarChartWidget`, `DualAxisChartWidget`, `RingProgressWidget`, `SparklineWidget`, `StackedBarWidget`) — accept required `String semanticsLabel` parameter; callers compose data summaries (e.g., `l10n.savingsTrendSummary(co2Kg, months)`). Lint rule added in Phase 2 enforces non-null param at call sites.
5. **`MilestoneCard`** dismiss button touch target 24dp → 48dp.
6. **`UserAvatar` status badge** composite semantic label: `${user.name}, ${badgeStatus}`.
7. **Allowlist shrink** for impact and chart directories.

`SectionHeader` font size and muted color-token contrast remediation are deferred to a later design-token review pass — they require visual-hierarchy decisions outside the scope of this plan.

**Acceptance**: Impact Metrics directory has zero allowlist entries. Charts cannot be instantiated without a semantic label (compile-time enforcement via required parameter, runtime enforcement via lint).

### Phase 6 — Long tail & polish (1–2 weeks)

Sweep through the remaining flagged areas. Most of these are mechanical migrations now that primitives and lint are in place.

**Phase 6a — Mechanical sweeps** (shipped):

1. **Modal focus restoration.** All 66 `showModalBottomSheet` sites migrate to `showAccessibleModal`. Lint enforces from this point.
2. **Reduced motion adoption.** All 53 `Duration` literals on animation surfaces migrate to `accessibleDuration(context, ...)`. Lint enforces from this point.
3. **CachedMediaImage decorative labels.** All 20+ caller sites pass an explicit `semanticsLabel:` (mostly `null` with a comment marking the image as decorative because the surrounding card already carries the semantic label).

**Phase 6b — Per-call-site label authoring** (shipped via 9 parallel agents):

The remaining ~220 allowlist entries spread across feature directories were migrated by 9 parallel agents, one per directory bucket. Each agent:

- Replaced raw `GestureDetector` / `InkWell` with `Tappable` or `Toggle` (Toggle for selectable state with `selected:`).
- Replaced `IconButton` (no tooltip) with `IconAction`.
- Replaced `Semantics(label: 'literal')` with `context.l10n.<key>`.
- Wrapped color-only `*Dot` / `*Badge` / `*Indicator` widgets in `Semantics(label: ...)` with reader-only labels.
- Added a directory-namespaced l10n key prefix (`a11yExp`, `a11yContent`, `a11yChat`, `a11yPortfolio`, `a11yGear`, `a11yImpactScreen`, `a11yDiscover`, `a11yMedia`, `a11yDaily` / `a11yReq`, `a11yMisc`) to avoid collisions while editing the shared ARB files in parallel.

Result: allowlist shrinks **220 → 3 entries**. The 3 remaining are legitimate exemptions:
- `community_metrics_screen.dart` and `portfolio_metrics_screen.dart` — outer `GestureDetector` for `onHorizontalDragEnd` swipe nav with a nested `Tappable` for `onTap`. The lint regex's 30-line look-ahead window matches the inner `onTap:` and false-positives.
- `home_view_model.dart` — viewmodel with no `BuildContext` in scope; `accessibleDuration` requires a context.

**Phase 6c — Final acceptance** (follow-up):

- VoiceOver / TalkBack smoke test on iOS and Android. File issues for any gaps the static analysis missed.

**Phase 6 acceptance** (shipped): allowlist 412 → 3 entries (>99% shrink); CI ratchet enforces no regressions; remaining 3 entries are documented legitimate exemptions, not unfinished work.

## Resolved decisions

Captured here as durable record so phase issues don't relitigate them:

- **`ConfidenceDot`**: screen-reader label only, no visual change. (Phase 5)
- **Color-token contrast remediation** (`SectionHeader` 10px font, `lightTextTertiary`, `transferTextMuted`): deferred to a later design-token review pass, not part of this plan.
- **Base widget naming**: `Tappable`, `Toggle`, `IconAction` — unprefixed because accessibility is the default. `IconAction` chosen specifically to avoid colliding with Flutter's `IconButton`.
- **L10n key prefix** for accessibility-only strings: `a11y` (e.g., `a11yClose`, `a11yShowPassword`).
- **Allowlist bypass policy**: trust-based — engineers add to the allowlist when needed without process gating. The ratchet (allowlist can shrink but not grow on `main`) prevents accumulation; the trust model accommodates legitimate exceptions.
- **VoiceOver/TalkBack smoke-test follow-ups**: any gaps the smoke test finds become follow-up issues rather than extending Phase 6, since they imply the static-analysis enforcement needs sharpening rather than the migration being incomplete.

## Issue plan

Each phase below maps to one GitHub issue. Phase 1, 2, 5, and 6 are large enough that the issue body should pre-chunk them into sub-tasks for parallel execution within the phase.

| Phase | Issue | Estimated size | Depends on |
|-------|-------|----------------|------------|
| 1 | [#1440](https://github.com/Type2Fun-Foundation/ripls/issues/1440) — Foundation primitives | 1–2 weeks | — |
| 2 | [#1441](https://github.com/Type2Fun-Foundation/ripls/issues/1441) — Enforcement infrastructure | 2 weeks | Phase 1 |
| 3 | [#1442](https://github.com/Type2Fun-Foundation/ripls/issues/1442) — Auth flow & global status | 1 week | Phases 1, 2 |
| 4 | [#1443](https://github.com/Type2Fun-Foundation/ripls/issues/1443) — Transfer feature migration | 1–2 weeks | Phases 1, 2 |
| 5 | [#1444](https://github.com/Type2Fun-Foundation/ripls/issues/1444) — Impact Metrics & charts | 2 weeks | Phases 1, 2 |
| 6 | [#1445](https://github.com/Type2Fun-Foundation/ripls/issues/1445) — Long-tail sweep & smoke test | 1–2 weeks | Phases 1–5 |

## Out of scope

- Server-side accessibility concerns (e.g., alt-text generation for AI-described images). May be a future plan.
- Web app or marketing-site accessibility. Plan covers the Flutter app only.
- AAA-level conformance work beyond the specific items mentioned (reduced motion).
- Formal compliance certification. We are not pursuing a VPAT or accessibility statement.
- External accessibility audit. Explicitly deferred; revisit after Phase 6.
