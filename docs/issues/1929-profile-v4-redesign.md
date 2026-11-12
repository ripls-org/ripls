# Issue #1929: Update user profile screen to match V4-A concept design

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1929
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

> **Audit notes (2026-05-15):** Reviewed against [client/architecture.md](../client/architecture.md), [client/caching.md](../client/caching.md), [client/i18n.md](../client/i18n.md), [client/accessibility.md](../client/accessibility.md), [client/testing.md](../client/testing.md), [server/architecture.md](../server/architecture.md), [server/observability.md](../server/observability.md). Changes from the audit are flagged inline with **[audit]** tags. The most material adjustments: (1) accessibility primitives are mandatory and CI-enforced — `Tappable`, `IconAction`, `CachedMediaImage(semanticsLabel:)`, `accessibleDuration` — and the plan's "widget test per new widget" target was reframed as ViewModel-first per [testing.md]; (2) color tokens must go through `AppColors.xxx(context)` (theme-aware, dark-mode variants) rather than raw hex; (3) the parallel impact + profile fetch in Phase 3 must follow the `Future.wait` + `ref.mounted` pattern; (4) the available-now server query must use the structured-logging helpers (`logging.LoggerWithContext`), `connecterr.Internal`, and an `AssertMaxQueries` test to prevent N+1.

## Problem Statement

The user profile screen at [app/lib/presentation/screens/users/user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart) currently renders the viewer's lens of a target user as: an identity block, the Workshop hero number (hours together), the Workshop three-column ticker (people, $ saved, CO₂), and a list of `BriefCTARow` postcards. The issue asks for a visual redesign to match the V4-A concept at [docs/cowork/App Design/profile-v4-a-standalone.html](../cowork/App%20Design/profile-v4-a-standalone.html) (screenshot: [user-profile-v4.png](../cowork/App%20Design/user-profile-v4.png)).

The new layout adds two sections the current screen doesn't have data for:

1. **Known For chips** — short capability tags like "Chainsaw lender", "Trailhead host". No `knownFor` / `capabilities` / `tags` field exists on `UserImpactMetrics`, `GetUserProfileForViewerResponse`, or any current proto carrying a user profile.
2. **Available Now rail** — a horizontal scroll of the user's currently-available gear and upcoming experiences. No "available inventory" query for a user exists today.

It also restructures existing data:

3. **Impact rows** — three stacked rows (Time given, $ saved, CO₂ avoided) each pairing a metric value with a **one-line serif narrative** ("Lent enough hours for a season of trail work."). The narrative copy doesn't exist anywhere — it's parallel to the deprecated `BriefPayload.kicker_label` / `closing_line` AI prose, which is exactly the pattern the codebase has been moving away from.
4. **Keep it going lean cards** — a re-render of the existing `BriefCTARow` postcards in a thinner card style with a contextual subtitle.

The issue body says "leave a TODO with a follow-up issue link" wherever data doesn't exist, so the redesign can land before the backend catches up. Two judgement calls follow:
- Which existing aggregate to use for the impact rows: the **scope-aware** `BriefPayload` ticker (hours together with the viewer, $/CO₂ across the brief scope) or the target's **full-life** `UserImpactMetrics` (`cost_savings_usd`, `carbon_savings_grams`, `time_banked_minutes`). The V4-A copy ("300 hrs", "$955", "57 kg") and the issue language ("aggregate hero and ticker numbers reflect the target's full activity," from the existing `UserScreen` doc comment) both read as full-life — not pairwise.
- CO₂ unit: V4-A shows **kg**, the brief carries `co2_avoided_pounds`, the impact metrics carry `carbon_savings_grams`. Switching units is fine but the unit string must match the source.

## Context from Comments

No comments on this issue (it was filed today). The body is detailed and unambiguous about the visual target; the open work is in the data-mapping and how aggressively to ship before the missing backend pieces land.

There is one closely-related historical plan at [docs/issues/1009-user-screen-redesign.md](1009-user-screen-redesign.md), which proposed mirroring the portfolio metrics screen on the user profile. **That plan is superseded by this one** — V4-A goes in a different direction (editorial / narrative, not analytics-heavy), and the issue explicitly references the V4-A concept as the new target.

## Potential Solutions

### Option A: Visual-only refactor against existing data, stub the new sections

Rewrite `UserScreen`'s build method around the new five-section layout. Reuse the existing `viewerProfileProvider` and its `ViewerProfileState` / `BriefPayload` exactly as-is. The two genuinely-new sections (Known For chips, Available Now rail) render with placeholder TODOs gated behind follow-up issues. The Impact Rows section gets stub narrative strings (a static lookup table keyed by metric kind, e.g. "Lent enough hours for a season of trail work.") rather than calling out to AI.

- **Approach:** Build five new widgets under `app/lib/presentation/widgets/user_profile/` (`UserProfileIdentityV4`, `UserProfileKnownForChips`, `UserProfileImpactRows`, `UserProfileAvailableNowRail`, `UserProfileLeanCards`). Add Lora + Inter fonts to `app/pubspec.yaml`. Add coral / cream / chip / hair tokens to [app_colors.dart](../../app/lib/core/theme/app_colors.dart). The existing `UserProfileIdentity`, `WorkshopHero`, `WorkshopTicker`, and `WorkshopPostcard` widgets stay in place — they're shared with other surfaces — but `UserScreen` stops calling them.
- **Pros:** Ships fast. Decoupled from backend work. The visual redesign is independently reviewable. No risk of breaking other surfaces that use the Workshop widgets.
- **Cons:** Two of the five sections are visually present but functionally empty until backend follow-ups land. The static-narrative-strings approach for Impact Rows means every user reads the same sentence — that's fine as a stopgap but it'll look weird in beta and we'll need to remember to revisit it. The chip / rail TODOs need clean copy ("Known For — coming soon" or hide-when-empty) so the screen isn't broken at first launch.
- **Effort:** Medium.

### Option B: Full-stack delivery (proto + server + client) in one PR

Add `repeated string known_for` to `GetUserProfileForViewerResponse`, add a new `repeated AvailableNowItem available_now` field (or a new RPC), and add `string time_given_narrative` / `string money_saved_narrative` / `string co2_avoided_narrative` fields populated server-side from the AI brief generator. Wire all of it through.

- **Approach:** Proto changes → server implementations (known-for derivation, available-now query against gear + experience tables, AI prose generation for narratives) → repository / view model updates → client widgets.
- **Pros:** Ships the redesign in its final form. No stubs.
- **Cons:** Multiplies the surface area of one PR by 3-4×. The "known for" derivation is a significant design question on its own (what makes someone a "Chainsaw lender"? Top-borrowed item categories? Self-declared? AI-summarized?). AI narrative copy needs prompt design, eval, and the storage/caching infrastructure to avoid regenerating on every profile view. Available-now needs a new SQL query path. This is more like 3-4 issues, not 1.
- **Effort:** Large.

### Option C: Hybrid — ship visuals + the available-now rail in parallel

Same as Option A, but file the Available Now rail as a sibling issue and do it concurrently because it's the most contained backend piece (single query: `user.gear WHERE state = AVAILABLE AND community_id IN viewer_shared_community_ids LIMIT N`, possibly UNION with `experiences WHERE start_unix_sec > now() AND visibility = ...`). Known For chips and the AI narratives ship as a separate follow-up.

- **Approach:** Two PRs, roughly the same diff size as Option A — one client (V4-A redesign) and one server (`available_now_gear` + `available_now_experiences` fields on the existing profile response, or a new `GetAvailableNowForUser` RPC if we want it cacheable separately). They merge at roughly the same time.
- **Pros:** Three of five sections are real on launch instead of two of five. The available-now rail is the section most likely to look broken with a placeholder, so it's worth doing first. Splits backend work into the piece that is well-scoped (available-now) and the piece that isn't (known-for + AI narratives).
- **Cons:** Slightly more coordination overhead. The client PR has to be careful to render gracefully whether or not the available-now field is populated, since the two PRs may not land atomically.
- **Effort:** Medium (client) + Small-Medium (server).

## Recommendation

**Option C: Hybrid.** Available-now is well-scoped enough that holding it back gains nothing, and it's the section that most obviously looks broken if it's empty. Known For chips and the AI narratives have real design questions still open and shouldn't block a visual refactor.

Concretely:
- Land the V4-A client redesign with: identity, impact rows (existing aggregate from `UserImpactMetrics`, static narrative strings), available-now rail (new field, populated by sibling PR), keep-it-going lean cards (existing `BriefCTARow`), and Known For chips hidden when empty.
- File [#TBD] for known-for derivation (separate design discussion).
- File [#TBD] for per-user AI narratives on the impact rows.
- The static narrative strings are a stopgap that should be visibly labeled in the PR description so they don't get forgotten.

## Open Questions (Resolved)

All open questions resolved 2026-05-15 by accepting the original recommendations. Decisions are now load-bearing for the implementation plan below.

- **Impact-row data source: `UserImpactMetrics` (full-life), not `BriefPayload`.** Matches the V4-A copy ("Lent enough hours for a season of trail work") and the existing `UserScreen` doc comment ("Aggregate hero and ticker numbers reflect the target's full activity"). Opens up `cost_savings_usd` / `carbon_savings_grams` / `time_banked_minutes`. `BriefPayload` continues to feed the lean-cards section.
- **CO₂ unit: kg.** Convert from `UserImpactMetrics.carbon_savings_grams` (`grams / 1000`, integer round) at the widget edge in `user_profile_impact_rows.dart`. One-line comment naming the source proto field. No proto change.
- **Empty-state behavior: hide when empty.** Known For chips and Available Now rail return `SizedBox.shrink()` when their data is absent. No "coming soon" placeholders.
- **Shared-community gating:**
  - Identity, Known For, Impact Rows render unconditionally (they describe the target, not the relationship).
  - Available Now rail and Keep It Going lean cards both gate on `state.hasSharedCommunities` (their entities are only actionable inside shared communities).
  - **Self-view addendum (2026-05-17, #1996):** Self-view (`viewer == target`) is a strict mirror of other-view with one principled exception. The Available Now rail and Known For chips source from the target's full active community membership (not the empty viewer-target intersection); per-user AI narratives run unchanged (they have no community scope). Postcards stay implicitly empty because their gate `len(sharedRefs) > 0` collapses out on self-view by construction, and the "you two…" CTA framing is incoherent for a single user. See [docs/issues/1996-self-view-items.md](1996-self-view-items.md).
- **Narrative + a11y strings: ARB only.** Keys land in both `app_en.arb` and `app_es.arb` per [client/i18n.md](../client/i18n.md). Impact narratives use `userProfileImpact*Narrative`; a11y labels use the `a11y` prefix per [client/accessibility.md](../client/accessibility.md#writing-good-accessibility-labels) (e.g. `a11yProfileAvatarOf`, `a11yProfileShowMoreChips`, `a11yProfileRailTile`). No constants file. The "Event" vs "Experience" rule from [CLAUDE.md](../../CLAUDE.md) applies.
- **Workshop widgets on `UserScreen`: removed.** `UserScreen` drops `WorkshopHero` / `WorkshopTicker` / `WorkshopPostcard` / `WorkshopSectionHeader` / `WorkshopShowMoreLink` imports entirely. The widgets stay in the codebase; other surfaces (Workshop tab) continue to use them.
- **Keep It Going subtitle copy:** derive from `payload.ctaRows.first.kicker` when present, else fall back to a static ARB string (`userProfileKeepItGoingFallback`, value `Three ways to keep it going`). The V4-A example sentence ("You two — 2 events together…") is *not* hardcoded.

## Implementation Plan

### Phase 1: Theme, fonts, and shared tokens

**Goal:** Get the visual primitives in place so the new widgets can be built against real tokens instead of hardcoded hex.

**Tasks:**
- [ ] Add Lora (400/500/600/700, italic 400) and Inter (400/500/600) to [app/pubspec.yaml](../../app/pubspec.yaml) under `fonts:`. Drop the TTFs into `app/assets/fonts/`.
- [ ] **[audit]** Add theme-aware `AppColors` helpers per [client/architecture.md §Colors and Theming](../client/architecture.md#colors-and-theming) — colors must be accessed via `AppColors.xxx(context)`, not raw `Color(0xFF...)`. Required new helpers on [app_colors.dart](../../app/lib/core/theme/app_colors.dart): `profileCoral(context)`, `profileCoralDark(context)`, `profileCream(context)`, `profileInk(context)`, `profileInkSubtle(context)`, `profileMuted(context)`, `profileChipBg(context)`, `profileHair(context)`, `profileHairLight(context)`. Each helper must return a sensible dark-mode variant (don't reuse the existing `lightPrimary` — V4-A coral `#C25E3F` is distinct from `lightPrimary` `#E07A5F`).
- [ ] Add `displaySerif` (Lora) and `bodySans` (Inter) font-family constants to [app_theme.dart](../../app/lib/core/theme/app_theme.dart). Do **not** flip the existing `headingFont` (Libre Baskerville) — other screens depend on it.
- [ ] Run `flutter pub get` and confirm fonts load.

**Validates:** Lora and Inter render on a hello-world widget. Color helpers compile and resolve in both light and dark mode.

### Phase 2: New widgets under `widgets/user_profile/v4/`

**Goal:** Build the five sections as isolated widgets, wired to the existing `ViewerProfileState` + `UserImpactMetrics` only.

**[audit] Accessibility primitives — non-negotiable.** Every interactive element in these widgets is CI-gated by `scripts/check_a11y.js` (see [client/accessibility.md](../client/accessibility.md)). Concretely:
- Taps go through `Tappable` (or `IconAction` for icon-only) from [`widgets/accessibility/`](../../app/lib/presentation/widgets/accessibility/), never `GestureDetector` / `InkWell` / `IconButton` directly.
- Avatar / rail thumbnails / lean-card thumbnails use `CachedMediaImage(semanticsLabel: ...)`. Per [CLAUDE.md §Media Display](../../CLAUDE.md#media-display): never `Image.network`; use the stable `MediaUrl.cacheKey` from the repository for cache stability across presigned URL rotation.
- Any animation (chip expand, rail scroll cue) wraps its `Duration` with `accessibleDuration(context, ...)`.
- Toggleable chips (the `+` expand button maintains UI state) — semantics handled by `Tappable` with a localized label; expanded vs collapsed conveyed via `selected:` semantics if appropriate, not via the visible label string.

**Tasks:**
- [ ] `user_profile_identity_v4.dart` — circular avatar (64×64) + Lora display name + community subtitle ("With *Coral Community* and N other communities."). Reuse the existing community-link tap behavior from [user_profile_identity.dart](../../app/lib/presentation/widgets/user_profile/user_profile_identity.dart). Avatar uses `CachedMediaImage(semanticsLabel: l10n.a11yProfileAvatarOf(name))`.
- [ ] `user_profile_known_for_chips.dart` — wrap-layout chip row, collapse to 4 with `+` expand. Takes `List<String>` (empty list ⇒ widget returns `SizedBox.shrink()`). The `+` chip is a `Tappable` with `semanticsLabel: l10n.a11yProfileShowMoreChips(hiddenCount)`.
- [ ] `user_profile_impact_rows.dart` — three hairline-separated rows. Takes `UserImpactMetrics`. Renders `Estimate.mean` values:
  - Time given: `time_banked_minutes` → hours (`(minutes / 60).round()`).
  - $ saved: `cost_savings_usd` → whole dollars.
  - CO₂ avoided: `carbon_savings_grams` → kg (`(grams / 1000).round()`).
  Each row's narrative line comes from ARB (`userProfileImpactTimeNarrative`, `userProfileImpactMoneyNarrative`, `userProfileImpactCo2Narrative`). Each unit conversion has a single comment naming the source proto field.
- [ ] `user_profile_available_now_rail.dart` — horizontal `ListView` of tiles. **[audit]** Takes the generated proto `AvailableNowItem` directly once Phase 4 lands; in the interim, build against a Freezed view-model adapter under [`app/lib/presentation/models/`](../../app/lib/presentation/models/) (not under `viewmodels/` — per [architecture.md], view models hold state, not data shapes). Empty list ⇒ widget returns `SizedBox.shrink()`. Tile tap is a `Tappable` routing through the lever dispatcher.
- [ ] `user_profile_lean_cards.dart` — vertical list of `BriefCTARow` items rendered as thin 64px-thumbnail cards. Section subtitle comes from `payload.ctaRows.first.kicker` when present, else the ARB string `userProfileKeepItGoingFallback`. Reuse [user_profile_lever_dispatcher.dart](../../app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart) for tap routing. Each card is a `Tappable`.
- [ ] **[audit]** Widget tests under `app/test/presentation/widgets/user_profile/v4/` are scoped to *behavior*, not visual rendering: empty-state collapse, `+` expand toggle, lean-card tap dispatch. Visual / golden tests are explicitly out of scope per [client/testing.md §Testing Philosophy](../client/testing.md#testing-philosophy) ("Test business logic, not Flutter widgets"). The heavy lifting goes into Phase 3's ViewModel tests.
- [ ] README in the new `widgets/user_profile/v4/` directory per the project convention (multi-file dirs under `app/lib/` need one).

**Validates:** Each widget renders standalone in a widget test. Empty-state hiding works. `npm run lint:dart:a11y` and `npm run lint:dart:i18n` pass with no new allowlist entries.

### Phase 3: Wire widgets into `UserScreen`

**Goal:** Replace `UserScreen._buildContent` with the V4-A composition. Update the view model to surface `UserImpactMetrics`.

**Tasks:**
- [ ] Extend [viewer_profile_view_model.dart](../../app/lib/presentation/viewmodels/viewer_profile_view_model.dart) to also fetch `GetUserImpactMetricsResponse` via [impact_repository.dart](../../app/lib/data/repositories/impact_repository.dart) and expose it on `ViewerProfileState`. **[audit]** Per [architecture.md §Parallel Data Loading](../client/architecture.md#parallel-data-loading) and [§Async Safety](../client/architecture.md#async-safety-in-notifiers), load with `Future.wait` and treat impact-fetch failure as non-fatal (catch around the impact call only, log to `_log.warning`, leave the field null). Because `ViewerProfileNotifier` is an `AsyncNotifier.autoDispose`, the `build()` path doesn't need `ref.mounted` checks; the existing `refresh()` method does and already follows the pattern — preserve it.
- [ ] **[audit]** The impact-rows widget consumes the new state field via `ref.watch(viewerProfileProvider(userId))` and hides itself when the field is null — viewmodels expose typed state, widgets render. No string resolution inside the viewmodel (per [i18n.md §Best Practices](../client/i18n.md#best-practices)).
- [ ] Rewrite [user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart) `_buildContent` to compose the five new widgets, applying shared-community gating: Identity / Known For / Impact Rows render unconditionally; Available Now rail and Keep It Going lean cards render only when `state.hasSharedCommunities`. Drop the `WorkshopHero` / `WorkshopTicker` / `WorkshopPostcard` / `WorkshopSectionHeader` / `WorkshopShowMoreLink` imports entirely (those widgets stay in the codebase for other surfaces). Page background switches to `AppColors.profileCream(context)` (theme-aware — do not hardcode the cream hex).
- [ ] Update the doc comment at the top of `user_screen.dart` to describe the V4-A layout.
- [ ] **[audit] ViewModel tests** under `app/test/presentation/viewmodels/viewer_profile_view_model_test.dart` per [client/testing.md](../client/testing.md) (this is where the primary coverage lives). Use `ProviderContainer` with mock `ProfileRepository` and mock `ImpactRepository`, generated via `@GenerateMocks`. Required cases:
  - Profile + impact both succeed: state carries both.
  - Profile succeeds, impact fails: state carries profile, impact field is null, no exception propagated.
  - Profile fails: AsyncValue.error surfaces; impact field state is irrelevant.
  - Disposal during async: per [architecture.md §Disposal Testing](../client/architecture.md#disposal-testing), assert `container.dispose()` during in-flight load doesn't throw.
  - `refresh()` invalidates both repositories and re-loads.
- [ ] Update existing widget tests under `app/test/presentation/screens/users/` to reflect the new composition.
- [ ] Manual smoke on iOS simulator and Android emulator: viewer who shares a community with target, viewer who shares no community with target, target with no impact data, target with no postcards.

**Validates:** The screen renders against real backend data with no regressions to the navigation flow (back button, swipe-to-close). ViewModel tests cover all parallel-load failure modes and the disposal-safety path.

### Phase 4: Available Now field (sibling server PR)

**Goal:** Land the available-now data so the rail in Phase 3 has something to show.

**Tasks:**
- [ ] Add `repeated AvailableNowItem available_now_items = N;` (and a new `AvailableNowItem` message: kind enum {`GEAR`, `EXPERIENCE`, `GIVEAWAY`}, title, subtitle, media_id, context_id, mini_label) to `GetUserProfileForViewerResponse` in [profile_service.proto](../../proto/ripls/api/profile_service.proto). Reread the proto conventions in [CLAUDE.md](../../CLAUDE.md#proto-comments) before commenting these fields — no vendor names, no server-side mechanics. Use `optional` for any field that may be absent (e.g. `subtitle`, `media_id`) per [CLAUDE.md §Proto Field Conventions](../../CLAUDE.md#proto-field-conventions).
- [ ] Server implementation: query gear and experiences scoped to the union of (viewer's communities) ∩ (target's communities), filter to `state = AVAILABLE` (gear) and `start_unix_sec > now()` (experiences), order by recency, `LIMIT 12`. Follow [SQL Efficiency rules in CLAUDE.md](../../CLAUDE.md#sql-efficiency) — use JOINs, not loops; use `storage.AssertMaxQueries` in tests.
- [ ] **[audit] Structured logging.** Add a `logger := logging.LoggerWithContext(ctx).With("operation", "GetUserProfileForViewer", "user_id", viewerID, "target_user_id", targetID)` at the top of the handler per [server/observability.md §Logging Usage Pattern](../server/observability.md#logging-usage-pattern). Log the available-now query duration at Debug; log errors at Error with the `error` field. Never `log.Printf` or `fmt.Printf`.
- [ ] **[audit] Error handling.** On query failure, return `connecterr.Internal(ctx, "GetUserProfileForViewer", err, "target_user_id", targetID)` per [CLAUDE.md §Error Handling](../../CLAUDE.md#error-handling-and-robustness). Bare `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js`.
- [ ] Server unit tests for the empty case, the gear-only case, the experiences-only case, and the mixed case. Each test wraps the call in `storage.AssertMaxQueries(t, ctx, N, fn)` with an explicit N that documents the expected query budget.
- [ ] **[audit] Cache invalidation.** Profile cache entries on the client are keyed by target user ID under namespace `'profile'`. When a target's gear or experiences change, the profile cache for that user is stale. Decide in this PR: either (a) accept staleness for the duration of the global TTL (simplest — matches current Workshop postcard behavior) or (b) wire `ProfileRepository.invalidate(targetUserId)` calls from `GearRepository` / `ExperienceRepository` mutation paths per [caching.md §Pattern 7](../client/caching.md#pattern-7-event-driven-cache-invalidation-with-optimistic-patches-daily-screen). Recommendation: (a) for v1 — the existing pull-to-refresh handler at [user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart#L73) already invalidates on demand.
- [ ] Client: drop the freezed adapter from Phase 2 in favor of the generated proto type. Update `user_profile_available_now_rail.dart` and the view model.
- [ ] Run `npm run generate && npm run build && go test ./...` and `npm run test:app` before opening the PR.

**Validates:** A target user with available gear and an upcoming experience surfaces them in the rail. SQL query count stays bounded (`AssertMaxQueries` passes). Structured log lines correlate by `request_id` end-to-end.

### Phase 5: Follow-up issues (file, don't implement)

**Goal:** Capture the work that's deferred.

**Tasks:**
- [ ] File issue: "Add `known_for` capability tags to user profile" — covers data source (top-loaned categories? self-declared? AI summary?), proto field, server derivation, and chip wiring.
- [ ] File issue: "Per-user AI narrative copy for impact rows" — covers prompt design, generation cadence (likely cached on profile fetch), proto fields, and l10n fallback for offline / generation-failed cases.
- [ ] Link both follow-ups in the V4-A PR description and in code TODOs at the widget-empty-state sites.

**Validates:** No dangling TODOs without a tracking issue.

## Related Issues

- #1009 — Prior "User screen is wonky" plan ([docs/issues/1009-user-screen-redesign.md](1009-user-screen-redesign.md)) proposed mirroring the portfolio metrics screen on the user profile. **Superseded by #1929.** Close #1009 with a pointer to this plan when the V4-A redesign ships.
- #1012 — "Manage button social proof" — touches adjacent profile surface area; check for any conflicting in-flight work before opening the PR.
