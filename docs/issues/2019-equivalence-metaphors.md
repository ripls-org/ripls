# Issue #2019: Replace Time Together and Money Saved metaphors with cited tier ladders

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2019
**Filed:** 2026-05-18 | **Last activity:** 2026-05-18
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/cowork/Data Scientist/Ripls_Equivalence_Spec.md](../cowork/Data%20Scientist/Ripls_Equivalence_Spec.md)

## Problem Statement

The workshop screen (communities) and user profile screen surface three
community-impact metrics: Time Together (hours), Money Saved (dollars), and
CO₂ Avoided (kg). Raw numbers are abstract, so each one is paired with a
metaphor. CO₂ → trees works — the Holt-Lunstad analogy of "15 kg ≈ one
front-yard tree" is concrete and motivating. The other two metaphors do not:

- **Walk-distance for Time Together** stretches from "crosswalk" to "beyond
  the moon" but never communicates *why hours together matter*. It's a unit
  conversion dressed up as a metaphor.
- **Meals/coffee for Money Saved** anchors only on small-dollar food — by
  the time a community has saved $5,000 the comparison ("feeding a city")
  collapses into hyperbole and loses defensibility.

The new spec (v1.0) replaces these with cited, tiered ladders that escalate
through *meaningful life landmarks* rather than scale-only conversions:

- **Time Together → "What it's doing for your body"** — 16 health tiers,
  cited to peer-reviewed research, the Surgeon General advisory, and the
  Harvard Study of Adult Development. Anchors on the Holt-Lunstad
  meta-analysis (148 studies, 308,000+ people).
- **Money Saved → "What your community gave one of you"** — 17 financial
  tiers cited to BLS, Census, and IRS, anchored on US median household
  income ($87,730 → every $877 saved ≈ a 1% raise).

The metaphor surfaces are unchanged (workshop detail screens + profile impact
rows); the copy and selection logic are not.

## Context from Comments

No comments on this issue — it was filed and immediately picked up for
planning. All requirements live in the issue body and the linked spec doc.
The three open questions in the spec ("Open questions" §) are forwarded to
this plan's [Open Questions](#open-questions) section.

## Potential Solutions

### Option A: Hardcoded ladders in code + ARB (parity with existing cards)

Replicate the exact pattern used today (`walk_comparison_card.dart`,
`meal_comparison_card.dart`): a Dart `enum` per metric, a threshold-based
selection function, and one ARB key per tier-field per language. Add a
"Source: [link]" footer to each card. Animate tier label transitions via
`AnimatedSwitcher`.

- **Approach:** New files `time_health_comparison_card.dart` and
  `money_paycheck_comparison_card.dart` (or rename existing files via
  `git mv` per CLAUDE.md). 16 + 17 tier enums. ~64 new ARB keys × 2
  languages. Tier copy and source URLs live in ARB; threshold tables in
  Dart. CO₂/forest card is left alone.
- **Pros:** Matches every existing pattern in the codebase. Zero
  server-side work. Ships in one PR. Source URLs are trivially keyboard
  focusable via existing link widgets. Easy to A/B test by branching the
  enum.
- **Cons:** Copy changes require a client release. Spanish translations
  must be authored before shipping. New ARB surface is large
  (~64 keys/language).
- **Effort:** Medium.

### Option B: Server-served versioned JSON config (per spec)

Implement the spec's suggested `equivalences.v1.json` config served via a
new RPC. Client caches it, the comparison cards consume tier records
dynamically, and copy can be updated server-side without a client release.

- **Approach:** New proto messages (`EquivalenceTier`, `GetEquivalencesResponse`),
  new RPC, new server config loader, new client cache + repository.
  Comparison cards become generic ("render this tier ladder") instead of
  metric-specific. ARB keys become *fallback only* — server copy wins.
- **Pros:** A/B testing copy without a release. Faster iteration cycle on
  marketing language. Future-proofs the slot (a fourth metric ladder
  could be added without code).
- **Cons:** Substantial new infrastructure (server config loader, RPC, client
  cache, repository plumbing) for a single use case. No remote-config
  precedent in the codebase. i18n becomes harder — server payloads either
  ship copy per locale or rely on client-side localization keys, defeating
  the "edit copy without release" benefit. Localized formatting (dollar
  amounts, "hours" vs "hr") still needs client code.
- **Effort:** Large.

### Option C: Hybrid — hardcoded ladders now, config plumbing later

Ship the cited ladders in code + ARB exactly as Option A, but factor the
client-side tier logic behind a small `EquivalenceLadder` data class that
both metrics consume. When config-as-data is needed for a separate reason
(third metric, marketing copy A/B), swap the source from hardcoded list
to RPC-fed list without touching the rendering layer.

- **Approach:** Phase 1 = Option A but rendered through a shared
  `EquivalenceLadderCard` widget that takes a `List<EquivalenceTier>` and
  a value. Tier lists are hardcoded constants for now. Phase 2 (deferred,
  out of scope for this issue) replaces the constants with a repository
  call once a real driver appears.
- **Pros:** Ships fast like Option A. Two metrics use one rendering widget
  instead of two near-identical files. Future config swap touches one file
  (the data source), not the widget tree. Mirrors how the forest card
  could later be unified.
- **Cons:** Slight up-front abstraction cost (one extra data class, one
  shared widget). The shared widget must support both "hours" and "USD"
  value formatting cleanly, which means the tier record carries a unit
  enum.
- **Effort:** Medium.

## Recommendation

**Option C (Hybrid).** It ships the new ladders at the same speed as the
straight-copy Option A but pays a small abstraction cost up front
(`EquivalenceLadder` + one shared card widget) that pays back the moment a
third metric or remote config arrives. The current per-metric duplication
(walk + meal + forest are three near-identical cards, plus copy of unit
conversion in three places per the research) is already a small mess; this
issue is a natural moment to consolidate.

Option B is the spec's preferred end-state but premature today — there is no
remote-config precedent in the codebase, and authoring 17 tiers in Spanish
remains a release-gated activity regardless of whether copy ships from
config or ARB. The plumbing wins nothing on the first ship.

## Open Questions

- **Per-person toggle on large communities (spec open question 1)?** —
  **Decision: defer indefinitely; no follow-up filed.** The headline shows
  the community total; per-person split is not part of the product
  direction and won't be reconsidered unless user signal demands it.
- **"Days of life added" top-of-time ladder (spec open question 2)?** —
  Recommendation: skip. It's morbid for a community app and the 16-tier
  health ladder already lands the same point at tier 13 ("Survival odds up
  50%") with cited evidence.
- **State-median wage for money tiers (spec open question 3)?** —
  Recommendation: skip for v1, US-national only. The spec already calls
  this out as a localization step; doing it well requires per-state median
  income data + per-state regeneration of every tier label, which is its
  own project. National anchors are still defensible.
- **Should we rename `walk_comparison_card.dart` / `meal_comparison_card.dart`
  on disk?** — Recommendation: yes, via `git mv` (CLAUDE.md preserves
  history). The metaphor shift is conceptual, not just copy — calling the
  new file `walk_comparison_card.dart` would mislead future readers. Move
  to `time_health_comparison_card.dart` and `money_paycheck_comparison_card.dart`,
  or — if we adopt Option C's shared widget — collapse both into
  `equivalence_ladder_card.dart` and delete the old files.
- **Source link UI affordance?** — Recommendation: small "Source: [name]"
  footer below the tier copy, rendered as a tappable / keyboard-focusable
  link using the existing link styling. Don't show the URL inline; the
  citation name is enough for trust. URLs open in the system browser.
- **Tier-unlocked animation?** — Recommendation: `AnimatedSwitcher` on the
  headline text node with a 300ms cross-fade when tier index changes. Only
  fires on real change (not on first render). Respect
  `MediaQuery.disableAnimationsOf(context)` for reduced-motion users.
- **`tier_unlocked` analytics event timing — fire from client or server?**
  — Recommendation: client-side only, fire from the comparison card the
  first time it renders a higher tier than the prior render for the same
  community / user surface. The spec wants milestone telemetry; client-side
  is sufficient (matches the existing Firebase Analytics pattern in
  `ObservabilityService.logEvent`). No server work needed.
- **CO₂/forest card — leave alone or unify behind the new shared widget?**
  — Recommendation: leave alone in Phase 1 (issue scope says "CO₂ → trees
  unchanged"). If we ship Option C's shared widget, file a tiny follow-up
  to migrate the forest card onto it for consistency.

## Implementation Plan

### Phase 1: Shared ladder widget and Time Together ladder

**Goal:** New 16-tier health ladder ships behind a reusable
`EquivalenceLadderCard` widget, fully cited and animated, on both workshop
and profile surfaces. Money Saved continues to render the old meal card
unchanged until Phase 2.

**Tasks:**

- [ ] **Define `EquivalenceTier` data class** in a new file
  `app/lib/presentation/widgets/impact/equivalence/equivalence_tier.dart`.
  Fields: `id`, `threshold` (double), `unit` (enum: hours / usd / kgCO2),
  `labelKey`, `copyKey`, `sourceName`, `sourceUrl`. Pure data, no
  rendering logic. Selection helper:
  `EquivalenceTier? selectTier(List<EquivalenceTier> ladder, double value)`
  returns the highest tier where `threshold ≤ value`, or null if value is
  below the lowest threshold.
- [ ] **Create `EquivalenceLadderCard` widget** in
  `app/lib/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart`.
  Takes a `List<EquivalenceTier>`, a value, a group size (for solo-vs-group
  copy switching), and the headline subject. Renders: emoji or icon
  (passed in), bold tier label, body copy, and a "Source: [name]" footer
  rendered with the `Tappable` accessibility primitive (not raw
  `GestureDetector` / `InkWell` — caught by `avoid_raw_gesture_detector`
  lint). The tap target carries a `semanticsLabel` resolved from an
  `a11y*` ARB key (e.g., `a11yEquivalenceSourceLink`) — raw string
  literals on `semanticsLabel:` are rejected by the
  `require_l10n_for_semantic_labels` lint. Wraps the headline in
  `LiveRegion` so screen-reader users hear the new tier when it changes,
  and in `AnimatedSwitcher` with
  `duration: accessibleDuration(context, const Duration(milliseconds: 300))`
  (the `prefer_accessible_duration` lint rejects raw `Duration` here;
  `accessibleDuration` collapses to `Duration.zero` when reduce-motion
  is on). Use `AppColors.cardBackground(context)` / `AppColors.border(context)`
  for surfaces — no hardcoded `Colors.white` / `Colors.grey[*]`. Headline
  font reuses existing theme tokens (e.g., `AppTheme.headingFont`) per
  the project's theme-reuse memory. Below-lowest-threshold returns a
  "Just getting started" fallback per spec §"Selection logic". All colors
  must come from `AppColors.*(context)` so dark mode renders correctly.
- [ ] **Author the 16 Time Together tiers** as a `const` list in
  `app/lib/presentation/widgets/impact/equivalence/time_health_ladder.dart`.
  Each tier carries an ARB-key reference; copy text itself lives in ARB.
  Thresholds in *hours* per the spec table.
- [ ] **Add ARB keys for all 16 tiers** in `app/lib/l10n/app_en.arb`:
  `impactTimeHealthLabel<TierId>` (e.g., `impactTimeHealthLabelT40hr`) and
  `impactTimeHealthCopy<TierId>` for each. Plus the headline subject keys
  ("of time together") with ICU plural form for solo-vs-group framing
  (one key, not two). Plus the "Just getting started" fallback. Plus
  source-name keys (e.g., `impactTimeHealthSourceUnhExtension`) — keep
  source URLs as `const String` in the ladder data file (URLs do not need
  translation). Plus `a11y`-prefixed keys for any accessibility labels
  (e.g., `a11yEquivalenceSourceLink`) — `docs/client/i18n.md` and
  `docs/client/accessibility.md` both require this prefix and the
  `require_l10n_for_semantic_labels` lint enforces it.
- [ ] **Translate all new keys to Spanish** in `app/lib/l10n/app_es.arb`
  **before running `npm run generate`** (per i18n.md: missing keys are
  caught at compile time because `nullable-getter: false`). Use a fluent
  translator for the health-framing nuance — auto-translation will lose
  the "Surgeon General inverse" framing on tier 8. Verify CO₂ baseline
  keys (`impactForest*`) remain untouched. Verify the ICU plural form
  translates correctly into Spanish's plural rules.
- [ ] **Wire `EquivalenceLadderCard` into the workshop time detail
  screen** ([workshop_time_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart)).
  Replace the existing `WalkComparisonCard` slot. Existing `_buildEquivalent()`
  composition in `workshopTimeDetailEquivalentBody` l10n should be removed
  — the new card owns its own narrative composition. The screen does
  **not** touch the observability service or `TierTrackingRepository`
  directly; it only renders the card, which in turn calls the
  `EquivalenceTierTrackingNotifier` via `ref.read`. The detail screen's
  existing ViewModel already supplies the raw `hours` (or `usd`, `kg`)
  value and `groupSize`; pass those through.
- [ ] **Wire `EquivalenceLadderCard` into the profile impact row**
  via [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/profile_section/impact_row_narrative_tiers.dart).
  The existing `timeGivenNarrative()` helper either delegates to the new
  ladder selector or is replaced by direct widget consumption — depends on
  how the profile renders narrative today (per research, narrative is a
  single composed sentence; the card replaces that sentence with full card
  rendering or with a one-line label fallback when card height isn't
  available).
- [ ] **Handle solo-user edge case** per spec: when `groupSize == 1`, the
  copy uses "gave you" instead of "gave one of you". Implement as a
  single ARB key using **ICU plural form**
  (`{groupSize, plural, =1{gave you} other{gave one of you}}`) — not
  two separate keys with a Dart switch. ICU plurals are the documented
  i18n pattern (see `docs/client/i18n.md` §"Best Practices") and keep
  the variant logic in the locale data where translators can adjust it.
  Declare the `groupSize` placeholder in the `@key` metadata block. This
  applies to Money in Phase 2 too — implement the key now so Phase 2
  inherits it.
- [ ] **Delete the legacy `walk_comparison_card.dart`** via `git rm` after
  all call sites are migrated. Also delete the now-unused
  `workshopTimeDetailEquivalentBody`-style ARB keys and the
  `impactWalk*` keys (37 EN + 37 ES). Confirm no other widget references
  them via grep before deletion.
- [ ] **Define `TierUnlockedEvent` analytics event** as a subclass of
  `AnalyticsEvent` (mirroring `GearCreatedEvent` in
  `app/lib/core/observability/events.dart`) with parameters
  `{ metric, tier_id, value, community_size }` where `metric` is the
  string `'time_together'` (Phase 1) or `'money_saved'` (Phase 2).
- [ ] **Add `TierTrackingRepository`** in
  `app/lib/data/repositories/tier_tracking_repository.dart`. Persists the
  last-seen tier id per `(surface, metric)` tuple using
  `SharedPreferences` (key pattern: `tier_tracking:{surface}:{metric}`).
  Exposes `Future<String?> getLastSeenTier(surface, metric)` and
  `Future<void> recordTier(surface, metric, tierId)`. This is the
  documented architecture: widgets never touch SharedPreferences or the
  observability service directly; everything routes through a
  repository per `docs/client/architecture.md` §"Repository Pattern".
- [ ] **Add `EquivalenceTierTrackingNotifier`** as an `autoDispose`
  `Notifier` in
  `app/lib/presentation/viewmodels/equivalence_tier_tracking_view_model.dart`.
  Exposes `Future<void> recordCurrentTier({surface, metric, tierId, value,
  communitySize})`. The notifier reads `TierTrackingRepository`,
  compares the supplied `tierId` against the persisted last-seen tier,
  and on a real change calls `ObservabilityService.logAnalyticsEvent`
  with a `TierUnlockedEvent`, then persists the new tier id. Uses
  `SafeNotifierMixin` and guards every state assignment after an
  `await` with `if (!ref.mounted) return;` per
  `docs/client/architecture.md` §"Async Safety in Notifiers".
- [ ] **Widget fires `tier_unlocked` via the ViewModel** —
  `EquivalenceLadderCard` calls
  `ref.read(equivalenceTierTrackingProvider.notifier).recordCurrentTier(...)`
  inside a `WidgetsBinding.instance.addPostFrameCallback` after build
  (so the call lands once per resolved tier, not per re-render). The
  widget does **not** call the observability service or SharedPreferences
  directly — that would violate the
  Screen → ViewModel → Repository → Service rule from
  `docs/client/architecture.md`.
- [ ] **Unit test** for `selectTier`: empty ladder returns null, value
  below lowest returns null, value at threshold returns that tier, value
  above highest returns the highest tier, value between two thresholds
  returns the lower one.
- [ ] **ViewModel unit test** for `EquivalenceTierTrackingNotifier` using
  `ProviderContainer` + mock `TierTrackingRepository` + mock
  `ObservabilityService` (Mockito generated mocks per
  `docs/client/testing.md`). Cases: first-render fires event once;
  same-tier re-render does not fire; tier change fires; verify
  repository was called with the right `(surface, metric, tierId)` tuple.
- [ ] **Disposal test** for `EquivalenceTierTrackingNotifier` per
  architecture.md "All async Notifiers must have disposal tests" — start
  a `recordCurrentTier()` call, dispose the container mid-flight, expect
  no crash and the future to complete.
- [ ] **Widget test** for `EquivalenceLadderCard` with an overridden
  `equivalenceTierTrackingProvider` that records calls — verify the
  widget calls the ViewModel once per resolved tier, falls back to
  "Just getting started" below the lowest threshold, and that the
  source link uses `Tappable` (presence of a `Tappable` ancestor on
  the link node). Animation behaviour is verified indirectly by the
  `accessibleDuration` call site — no need to drive reduce-motion in
  the widget test itself.
- [ ] **Golden test** for the time-health card at three representative
  values (5min, 100hr, 5000hr) in both English and Spanish, in light
  and dark mode (verifies `AppColors.*(context)` resolution).

**Validates:** New Time Together ladder is visible on both workshop and
profile surfaces, cited, animated, accessible, and analytics-instrumented.
The shared widget proves itself before Money Saved migrates onto it.

### Phase 2: Money Saved ladder on the same widget

**Goal:** 17-tier money ladder ships through the same
`EquivalenceLadderCard`, deleting the meal card. Confirms the abstraction
holds across two units (hours vs USD) and two anchor styles (health vs
paycheck).

**Tasks:**

- [ ] **Author the 17 Money Saved tiers** in
  `app/lib/presentation/widgets/impact/equivalence/money_paycheck_ladder.dart`.
  Thresholds in USD per spec table.
- [ ] **Add ARB keys for all 17 tiers** in `app_en.arb`
  (`impactMoneyPaycheckLabel<TierId>`, `impactMoneyPaycheckCopy<TierId>`,
  source-name keys). Note copy formula uses "giving a member / one of you"
  vs "giving you" (solo) — the same `groupSize` switch implemented in
  Phase 1's shared widget covers both ladders.
- [ ] **Translate to Spanish** in `app_es.arb`. Dollar amounts are USD;
  Spanish copy retains "$" — do not localize currency in v1 (spec defers
  i18n to a later phase).
- [ ] **Wire into workshop money detail screen**
  ([workshop_money_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_money_detail_screen.dart))
  and the profile impact row money path. Remove
  `workshopMoneyDetailEquivalentBody`-style l10n composition; the new
  card owns it.
- [ ] **Delete `meal_comparison_card.dart`** via `git rm`. Delete
  `impactMeal*` ARB keys (30 EN + 30 ES). Confirm no other widget
  references them via grep.
- [ ] **Widget test** for the money card at three representative values
  ($15, $1000, $50000) in both languages.
- [ ] **Golden test** for the money card matching Phase 1 coverage.
- [ ] **Verify `tier_unlocked` event fires** for `metric: 'money_saved'`
  end-to-end via the analytics dev tool. No new event class needed —
  Phase 1's `TierUnlockedEvent` parameterizes on metric name.

**Validates:** Both new ladders ship through one widget, prove the shared
abstraction, and the cited copy is live for all users on both surfaces.

### Phase 3: Cleanup and follow-ups

**Goal:** Decide whether to migrate the forest (CO₂) card onto the shared
widget for consistency, and capture the deferred open questions.

**Tasks:**

- [ ] **Decide on forest card migration.** Read the now-shipped
  `EquivalenceLadderCard` against `forest_comparison_card.dart` and
  determine whether a third migration is worth the churn or whether the
  forest card's distinct emoji-driven aesthetic justifies keeping it
  separate. If migrate: do it; if not: add a one-line note to the
  ladder widget's README explaining the exception.
- [ ] **File a follow-up issue for state-median money localization**
  (spec open question 3) once we have a real signal that the per-state
  framing matters (e.g., analytics show wide variance in tier-cross
  rates by US state). The per-person toggle (spec open question 1) is
  deferred indefinitely and is **not** filed as a follow-up.
- [ ] **Verify analytics dashboard receives `tier_unlocked`** events in
  Metabase. If a chart is needed, that's a follow-up.

**Validates:** No loose ends; the codebase has a single consistent pattern
for equivalence metaphors and a clean trail of follow-up issues.

## Related Issues

- **#1047 — Quality time metrics** ([1047-quality-time-metrics.md](1047-quality-time-metrics.md)).
  That plan is about the *correctness* of the hours-together number
  (dynamic recalculation, attribute display, server-side QT formula). This
  plan is about the *presentation* of the resulting number. The two
  intersect at the workshop time detail screen — coordinate ARB key naming
  so the two efforts don't conflict on the same screen.
- **Spec file:** [docs/cowork/Data Scientist/Ripls_Equivalence_Spec.md](../cowork/Data%20Scientist/Ripls_Equivalence_Spec.md)
  — authoritative source for tier values, copy, and citations. Do not
  re-derive thresholds; copy them verbatim. The `id` strings (`t_40hr`,
  `m_1025`, etc.) are the canonical analytics tier IDs.

## Audit Notes

The following changes were made during architectural compliance review
against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`.

1. **Data-access layering (client/architecture.md):** Original plan had
   `EquivalenceLadderCard` calling `ObservabilityService.logEvent` and
   `SharedPreferences` directly from the widget. This violates the
   Screen → ViewModel → Repository → Service rule. Restructured into:
   (a) new `TierTrackingRepository` that owns SharedPreferences
   persistence; (b) new `EquivalenceTierTrackingNotifier` (autoDispose
   Notifier) that reads the repository, compares against the supplied
   tier, and fires `TierUnlockedEvent` via the observability service;
   (c) the widget only calls
   `ref.read(equivalenceTierTrackingProvider.notifier).recordCurrentTier(...)`.

2. **Async safety in notifiers (client/architecture.md §"Async Safety"):**
   Added explicit `SafeNotifierMixin` and `if (!ref.mounted) return;`
   guard requirements after every `await` in the new notifier, and
   added a disposal test task to satisfy the rule that "all async
   Notifiers must have disposal tests."

3. **i18n: ICU pluralization (client/i18n.md §"Best Practices"):** The
   solo-vs-group copy ("gave you" vs "gave one of you") was originally
   specified as two separate ARB keys selected by a Dart switch.
   Replaced with a single ARB key using ICU plural form
   (`{groupSize, plural, =1{...} other{...}}`), keeping the variant in
   the locale data where translators can adjust it. Added a note that
   Spanish translation must verify ICU plural rules render correctly.

4. **i18n: translation timing (client/i18n.md):** Strengthened the
   Spanish translation tasks to specify that all keys must land in
   `app_es.arb` **before** `npm run generate` runs — the
   `nullable-getter: false` config in `app/l10n.yaml` turns missing keys
   into compile errors. Reaffirmed that human translation is required;
   auto-translation loses the health-framing nuance.

5. **Accessibility primitives (client/accessibility.md):** Original plan
   said the source link was "keyboard-focusable" and used `AnimatedSwitcher`
   with `MediaQuery.disableAnimationsOf`. Replaced with the documented
   primitives: `Tappable` for the source link (catches
   `avoid_raw_gesture_detector`); `semanticsLabel:` resolved from an
   `a11y*` ARB key (catches `require_l10n_for_semantic_labels`);
   `accessibleDuration(context, ...)` for the animation duration
   (catches `prefer_accessible_duration`); `LiveRegion` around the
   headline so screen-reader users hear tier changes. Added `a11y`-prefixed
   ARB keys to the key inventory.

6. **Theme tokens (memory: feedback_theme_reuse):** Added explicit
   requirement to use `AppColors.cardBackground(context)`,
   `AppColors.border(context)`, and existing `AppTheme.headingFont`
   tokens rather than hardcoded colors / per-section fonts. Added a
   light + dark mode golden test to verify dark-mode rendering.

7. **Testing layering (client/testing.md):** Split the original
   "widget test fires analytics" task into a dedicated ViewModel unit
   test (using `ProviderContainer` + Mockito mocks for the repository
   and observability service) and a separate widget test (verifies the
   widget calls the ViewModel and renders correctly). This matches the
   "test ViewModels with fake implementations" / "don't test UI and
   logic together" guidance.

8. **Server architecture / observability (server/*):** No server-side
   changes are proposed for Phase 1 or Phase 2 — `BriefPayload` already
   carries the raw `hoursTogether`, `replacedCostUsd`, and
   `co2AvoidedPounds` values the cards need. Phase 3's "config-as-data
   RPC" is explicitly deferred as a follow-up; when it's filed it will
   need its own plan covering `server/architecture.md` constructor
   injection, `logging.LoggerWithContext(ctx)` with `operation` field,
   and `connecterr.Internal` error wrapping. None of those apply to
   this issue's client-only scope.

9. **Caching (client/caching.md):** Confirmed no Stash cache namespace
   is needed — equivalence ladders are hardcoded `const` data, not
   network-fetched. SharedPreferences persistence for tier-tracking is
   a separate persistence layer (per-key, durable across restarts) and
   correctly sits behind a repository, not in the `StashCacheManager`.
