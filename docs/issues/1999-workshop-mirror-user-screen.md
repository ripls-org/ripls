# Issue #1999: Redesign Workshop screen to mirror User screen structure

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1999
**Filed:** 2026-05-17 | **Last activity:** 2026-05-17
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (design mock at [docs/cowork/App Design/workshop-redesign.jsx](../cowork/App%20Design/workshop-redesign.jsx))
**Audit:** Reviewed 2026-05-17 against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), [docs/server/architecture.md](../server/architecture.md), [docs/server/observability.md](../server/observability.md). Material changes are marked **[audit]** inline.

## Problem Statement

The Workshop tab today renders a centered serif hero ("87 hours together"), a three-cell ticker (people · saved · CO₂), and a list of action postcards. The design in [workshop-redesign.jsx](../cowork/App%20Design/workshop-redesign.jsx) keeps the same data but restructures the surface to read like the User profile screen — identity block, specialty pills, three impact rows (kicker + serif sentence), an Available Now rail, and a "Three ways to keep it going" lean-card stack — so a community is presented with the same grammar the app already uses for a person.

The issue asks for the rewrite to **reuse user-profile widgets where possible**, not to fork a parallel parchment-cream theme from the JSX palette.

The mock's structure maps cleanly onto today's user screen ([users/user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart)), but three of the seven sections need data that doesn't exist on the workshop side yet — specifically the identity subtitle ("With Mike, Diego and 8 others · since April 2026"), the specialty pills, and the community-scoped Available Now rail.

## Context from Comments

The issue has no comments yet — only the description I filed when creating it. The two implementation constraints are baked in:
- Reuse user-profile widgets where the visual shape already matches.
- Keep theme tokens (`AppColors`, `AppTheme.headingFont`); don't introduce the JSX's parchment-cream / forest-green hex constants. Forest green is already in the palette as `AppColors.primary` (`#5B8268` heritage sage).

No comments means we should err on the side of **conservative reuse** — match the user-screen widget contracts where they fit, file a follow-up issue for anything that needs new backend work, and avoid speculative refactors.

## Section-by-section parity

| # | Section | Today (Workshop) | Target (User-screen widget) | Gap |
|---|---|---|---|---|
| 1 | Header pill | [FloatingHeader](../../app/lib/presentation/widgets/floating_header.dart) | Same `FloatingHeader` shape | None |
| 2 | Community carousel | [WorkshopCommunityCarousel](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart) | Same widget, already has "Everything" pill when ≥ 2 communities | None — minor styling tweak only |
| 3 | Identity block | (none — title is in the header) | Analogue of [UserProfileIdentity](../../app/lib/presentation/widgets/user_profile/user_profile_identity.dart) | New widget. `UserProfileIdentity` couples to `ViewerProfileState` (target user) and can't be parameterized cleanly. Subtitle needs roster names + scope `since_unix_sec` |
| 4 | Specialty pills | (none) | Analogue of [UserProfileKnownForChips](../../app/lib/presentation/widgets/user_profile/user_profile_known_for_chips.dart) — pure `List<String>` | **[updated]** Chip widget is hoisted in Phase 0. The server derivation already exists for per-user scope at [server/services/profile/known_for.go](../../server/services/profile/known_for.go) and is community-aware; Phase 3 extracts it into a shared `server/known_for/` library and re-calls it from the workshop service with the workshop's enabled-community-id set |
| 5 | Three impact rows | [WorkshopHero](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart) + WorkshopTicker | Same shape as [UserProfileImpactRows](../../app/lib/presentation/widgets/user_profile/user_profile_impact_rows.dart) | The user widget reads `UserImpactMetrics`; workshop's `BriefPayload` carries the same three numbers (`hours_together`, `replaced_cost_usd`, `co2_avoided_pounds`) but in different fields. Narrative copy: user has per-user AI narratives; workshop has none (closest is `BriefPayload.kicker_label` which is deprecated). Tiered fallbacks in [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/user_profile/impact_row_narrative_tiers.dart) are vocabulary-neutral and reusable |
| 6 | Available Now rail | (none) | [UserProfileAvailableNowRail](../../app/lib/presentation/widgets/user_profile/user_profile_available_now_rail.dart) | **[updated]** Widget is hoisted in Phase 0. The server gather functions for the per-user feed already exist at [server/services/profile/available_now.go](../../server/services/profile/available_now.go) and scope by community set; Phase 4 extracts them into a shared `server/available_now/` library, re-calls from the workshop service, and adds `repeated AvailableNowItem available_now_items` to `GetWorkshopBriefResponse` |
| 7 | Three ways to keep it going | [WorkshopPostcard](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart) + WorkshopShowMoreLink + WorkshopLeverDispatcher | [UserProfileLeanCards](../../app/lib/presentation/widgets/user_profile/user_profile_lean_cards.dart) + [UserProfileLeverDispatcher](../../app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart) | **Open question, see below.** Both render `BriefCTARow`, but the dispatchers behave differently: the workshop dispatcher seeds the creation modal via `generateWorkshopDraft`; the user dispatcher routes to item / experience / request *detail*. Swapping in `UserProfileLeanCards` wholesale loses the creation flow |

## Reusable assets — already known to fit

- **`BriefCTARow` is the same proto** on both surfaces ([workshop_repository.dart](../../app/lib/data/repositories/workshop_repository.dart) re-exports it). The user-profile lean cards already consume it.
- **`AppColors` already maps the JSX palette.** `AppColors.primary` (`#5B8268`) is the forest green; `AppColors.background` (`#FDFAF7`) is the cream. No new tokens needed.
- **`AppTheme.headingFont`** (Libre Baskerville) is the serif used in both the JSX mock and existing user-profile widgets.
- **`workshopCommunityProvider`** already drives scope; the detail screens (time / money / CO₂ / roster) take `communityIds` so they continue to work no matter which surface element opens them.

## Potential Solutions

### Option A: Visual rewrite, no new backend

Replace hero/ticker/postcard widgets with new widgets that mirror the user-profile visual language, but feed them with **only the data that exists today**. Sections that require new data (identity subtitle, specialty pills, Available Now rail) get rendered conditionally — empty arrays / `SizedBox.shrink()` until follow-up issues add server support.

- **Approach:**
  - Build `WorkshopCommunityIdentity` as a sibling to `UserProfileIdentity` (different input contract — community, not viewer/target relationship). Subtitle uses `communityMembersProvider` for first-three names + `BriefPayload.since_unix_sec` for the date. For "Everything" scope, fall back to a short literal ("Across N communities · since {scope since}").
  - Specialty pills omitted in v1; file a follow-up issue.
  - Replace hero+ticker with a new `WorkshopImpactRows` widget that uses the same kicker + serif-sentence layout as `UserProfileImpactRows`. Data: hours / dollars / kg from `BriefPayload`. Narrative copy: reuse the tier functions in [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/user_profile/impact_row_narrative_tiers.dart) verbatim — they're vocabulary-neutral.
  - Available Now rail omitted in v1; file a follow-up issue.
  - "Three ways" postcards: rebind from `WorkshopPostcard` to the lean-card layout, but **keep `WorkshopLeverDispatcher`** (creation-modal flow) — the change is visual, not behavioural.
  - The row taps replacing hero/ticker route to the existing time / money / CO₂ detail screens.
- **Pros:**
  - Shippable as one Flutter-only PR. No proto changes, no server work, no migrations.
  - Captures the bulk of the redesign's visual intent in a small, reviewable change.
  - Sets up future server work behind clean widget seams.
- **Cons:**
  - Two of seven sections are missing in v1 (specialty pills, Available Now). Identity subtitle is degraded for "Everything" scope.
  - Risk of design ambiguity if reviewer expects the full mock on first cut.
- **Effort:** Medium.

### Option B: Hoist user-profile widgets to a shared package, parameterize for community/user, build everything

Refactor `user_profile/` widgets into a `widgets/profile_surface/` directory that takes a common contract (`ProfileSurfaceState` with `identity`, `chips`, `impactRows`, `availableNow`, `leanCards`) and add new RPCs to back the workshop-side data.

- **Approach:**
  - Move the user-profile widgets into a shared directory; rename to drop the `User` prefix.
  - Add a `CommunityProfileSurface` data adapter that fetches identity / specialty / available-now via new server endpoints and adapts everything into the shared contract.
  - Server: add `community_specialties` field (or RPC) and `GetCommunityAvailableNow` RPC (or extend `GetWorkshopBriefResponse`).
- **Pros:**
  - Full mock parity.
  - Single canonical widget set for "this is a profile-shaped surface."
- **Cons:**
  - Large blast radius. Touches proto, server, both surfaces, and CI tests.
  - Introduces shared abstraction with only two callers — premature per CLAUDE.md's "no half-finished implementations, three similar lines beats premature abstraction" guidance.
  - Higher coordination cost (proto review, server PR, client PR, ARB updates).
- **Effort:** Large.

### Option C: Cherry-pick reusable widgets in place, build new wrappers for the gaps

Use `UserProfileKnownForChips`, `UserProfileImpactRows`, `UserProfileAvailableNowRail`, and `UserProfileLeanCards` **directly** from the workshop screen — no parameterization, no hoisting. Build only the workshop-specific identity widget (`WorkshopCommunityIdentity`) as new code. Server gaps the same as Option A.

- **Approach:**
  - `UserProfileImpactRows` takes `UserImpactMetrics`. Either (a) construct a `UserImpactMetrics` from `BriefPayload` numbers at the screen edge, or (b) duck-type — add a constructor variant. (a) is cleaner.
  - `UserProfileAvailableNowRail` takes `List<AvailableNowItem>` — drop in unchanged once we have a source.
  - `UserProfileLeanCards` takes `List<BriefCTARow>` — drop in unchanged, but the routing question (dispatcher) is unresolved (see Open Questions).
- **Pros:**
  - Maximum literal reuse — the widgets *are* the user-profile widgets.
  - Smallest amount of new Dart code.
  - Easy to revert.
- **Cons:**
  - `UserProfile*` names rendering on a community surface read awkwardly in code review and future grep; future maintainers will assume those widgets only render users.
  - Coupling: any user-profile widget change is now also a workshop change. The user-profile widgets' README explicitly says *"Workshop tab and other surfaces continue to use the widgets under widgets/workshop/ — those are unrelated"* — this option breaks that boundary.
  - Mixing `UserProfileLeanCards` with `WorkshopLeverDispatcher` requires bypassing the lean cards' bundled tap handler.
- **Effort:** Small-to-medium.

## Recommendation

**[audit — revised]** A hybrid of A and B: **hoist the four surface-agnostic widgets** into a shared `widgets/profile_section/` directory and **build `WorkshopCommunityIdentity` as new code**. This satisfies the issue's explicit "reuse, don't fork" directive while respecting the dispatcher boundary that disqualifies wholesale Option C.

Concretely:

- **Hoist + rename** (no behavioural change, just relocation and parameterization):
  - `UserProfileKnownForChips` → `KnownForChips` (already takes `List<String>` — pure rename).
  - `UserProfileImpactRows` → `ImpactRows`, parameterized on `(hours, dollars, kg)` + three optional narrative strings instead of `UserImpactMetrics`. Callers compute the unit conversions at their own edge.
  - `UserProfileAvailableNowRail` → `AvailableNowRail` (already takes `onTapItem` callback — pure rename).
  - `UserProfileLeanCards` → `LeanCards`, gain an `onTapRow` callback parameter. The user surface passes `UserProfileLeverDispatcher.openPostcard`; the workshop surface passes `WorkshopLeverDispatcher.openPostcard`. Both dispatchers stay where they are.
- **Build new**:
  - `WorkshopCommunityIdentity` — sibling of `UserProfileIdentity`. Different input contract (community, not viewer/target relationship). Lives in `widgets/workshop/`.
- **Update READMEs**: the user-profile README's "Workshop tab and other surfaces continue to use the widgets under widgets/workshop/ — those are unrelated" line was correct before the redesign and is wrong after; update it to point to the new shared directory.

This avoids the "fork the look-and-feel" anti-pattern that the issue explicitly prohibits, avoids the over-engineering of a unified `ProfileSurfaceState` adapter (Option B), and keeps the dispatcher contract that makes Option C unworkable. The trade-off is one rename PR's worth of churn (the user-profile widgets' importers — only `users/user_screen.dart` today — get renamed imports).

The full Option B (single canonical surface contract) is overkill for two callers; file as a separate refactor issue if a third caller (place profile, tool profile) ever materializes.

## Open Questions

- **Should the "Three ways to keep it going" cards open the creation modal (today's `WorkshopLeverDispatcher.openPostcard`) or detail screens (`UserProfileLeverDispatcher.openPostcard`)?** — Recommendation: **keep the creation flow**. The workshop CTAs are action-oriented ("Share your pressure washer", "RSVP to Smoker round 6", "Bring back trail mornings") and the prior-instance draft seed is the workshop's signature behaviour. The user-screen lean cards route to item detail because the user is *somebody else's* — the CTA is "go see this." On the workshop, the CTA is "do this." Visual style copies from `UserProfileLeanCards`; tap target stays `WorkshopLeverDispatcher`.

- **"Everything" scope identity subtitle: what does "With X, Y, Z and N others · since {date}" mean across communities?** — Recommendation: the JSX mock spells it: *"With Mike, Diego, Sarah and 56 others across 5 communities · since September 2023"* — first-name-per-community for the named slot, total membership across all enabled communities for the count, and the earliest `since_unix_sec` across the scope. Implement the rollup at the view-model edge, not in the widget.

- **Where does "since {Month Year}" come from for a single-community scope?** — Recommendation: `BriefPayload.since_unix_sec` already carries this for the selected scope. No proto change needed for v1.

- **Specialty pills data source?** — **[updated — built in this issue, Phase 3.]** The user-side `known_for` already exists ([server/services/profile/known_for.go](../../server/services/profile/known_for.go)) and is implemented with a rule-based derivation: count completed loans per gear category within shared communities, threshold at 2, format as "{Category} lender", cap at 6. The shared-community filter is already the scoping mechanism. To produce the community-scoped equivalent, extract the derivation into a shared library at `server/known_for/` (per [docs/server/architecture.md](../server/architecture.md) §"Service Independence" — services don't import other services), then have the workshop service call it with the workshop's enabled community-id set. Surface as `repeated string specialties` on `BriefPayload`.

- **Available Now rail data source?** — **[updated — built in this issue, Phase 4.]** The user-side feed already exists ([server/services/profile/available_now.go](../../server/services/profile/available_now.go)) with three gather functions for gear / open requests / upcoming experiences. They already scope by `sharedSet[community_id]`, sort by recency, and cap at 12. To produce the workshop-scoped equivalent, extract the three gather functions into a shared library at `server/available_now/` (same service-independence rule), then have the workshop service call it with the workshop's enabled community-id set as the scope. Surface as `repeated AvailableNowItem available_now_items` on `GetWorkshopBriefResponse`, reusing the `AvailableNowItem` proto from [profile_service.proto](../../proto/ripls/api/profile_service.proto) — both protos live under `proto/ripls/api/` so the API-cross-import is allowed.

- **Do we deprecate `WorkshopHero` and `WorkshopTicker`, or keep them on `SeasonReportScreen`?** — Recommendation: check whether [season_report_screen.dart](../../app/lib/presentation/screens/workshop/season_report_screen.dart) still uses them; if it does, leave them in place and have the workshop home stop importing them. If nothing else imports them after the rewrite, delete them in the same PR.

- **Per-community AI narratives for the impact rows?** — Recommendation: v1 ships with the tier-based fallbacks from [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/user_profile/impact_row_narrative_tiers.dart). They're vocabulary-neutral; the user-side AI narrative landed in #1931 and can be mirrored for communities in a follow-up.

## Implementation Plan

### Phase 0 **[audit — added]**: Hoist surface-agnostic widgets

**Goal:** Move `UserProfileKnownForChips`, `UserProfileImpactRows`, `UserProfileAvailableNowRail`, and `UserProfileLeanCards` into a shared location, renamed and parameterized, with the user screen continuing to render byte-identical UI. This is a pure refactor — no visual change, no new feature.

**Tasks:**
- [ ] Create `app/lib/presentation/widgets/profile_section/` (or another agreed shared directory) with its own README.
- [ ] `git mv` the four files into it; rename the classes to drop the `UserProfile` prefix.
- [ ] Parameterize `ImpactRows`: input becomes `({int hours, int dollars, int kg, String? timeNarrative, String? moneyNarrative, String? co2Narrative})`. The user screen converts from `UserImpactMetrics` at its edge (where the unit math lives today). Move [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/user_profile/impact_row_narrative_tiers.dart) into the shared directory too.
- [ ] Parameterize `LeanCards`: add `required void Function(BriefCTARow row) onTapRow`. The user screen passes `UserProfileLeverDispatcher.openPostcard`. Section subtitle stays `BriefCTARow.kicker` with the same ARB fallback.
- [ ] Update imports in [users/user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart).
- [ ] Update [app/lib/presentation/widgets/user_profile/README.md](../../app/lib/presentation/widgets/user_profile/README.md) to reflect what stays (identity + lever dispatcher) and what moved.
- [ ] Update [app/test/presentation/viewmodels/](../../app/test/presentation/viewmodels/) imports.
- [ ] Run `npm run build` / `flutter analyze` / `npm run test:app -- --reporter expanded` (per [docs/client/testing.md](../client/testing.md)).

**Validates:** User screen renders unchanged on a real device, all existing viewmodel tests pass, no allowlist diff from `scripts/check_a11y.js`.

### Phase 1: Workshop screen rewrite using the shared widgets

**Goal:** The workshop home renders the new structure for the four data-complete sections — carousel, identity (basic single-community version), impact rows, lean-card postcards. "Everything" scope renders correctly. The two server-gated sections (specialty pills, Available Now) and the multi-community identity subtitle are stubbed.

**Tasks:**
- [ ] Add `WorkshopCommunityIdentity` widget under [app/lib/presentation/widgets/workshop/](../../app/lib/presentation/widgets/workshop/) **as its own file** (per CLAUDE.md: each new widget lands in its own file; the umbrella [workshop_home_layout.dart](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart) is already 635 lines). Avatar from the community's first `media_ids` entry via `mediaObjectProvider` (same pattern `UserProfileAvailableNowRail` already uses — repository access via the existing provider seam, not direct service calls — see [docs/client/caching.md](../client/caching.md) §"Pattern 3: Dependency Injection with Riverpod"). Serif name from `CommunityItem.name`; subtitle "since {Month Year}" from `BriefPayload.since_unix_sec`. Use `AppColors.primary(context)` + `AppTheme.headingFont`. Roster-derived subtitle deferred to Phase 2.
- [ ] Wire the workshop to the hoisted `ImpactRows`: at the screen edge, convert `BriefPayload.hours_together` / `replaced_cost_usd` / `co2_avoided_pounds` (pounds → kg) into the `ImpactRows` input contract. Each row tappable, routing to the existing time / money / CO₂ detail screens via the same handlers `WorkshopTicker` uses today.
- [ ] Wire the workshop to the hoisted `LeanCards`, passing `onTapRow: (row) => WorkshopLeverDispatcher.openPostcard(context, ref, contextId: row.hasContextId() ? row.contextId : '')` so the creation-modal flow is preserved.
- [ ] Edit [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart) `_WorkshopBody.build` to render: `WorkshopCommunityCarousel` → `WorkshopCommunityIdentity` → `ImpactRows` → `LeanCards`. Remove the `WorkshopHero` and `WorkshopTicker` calls.
- [ ] **[audit]** Confirm `WorkshopCommunityCarousel` already hides the "Everything" pill when the user is in a single community (research note from issue body). If the JSX mock shows "Everything" always present, decide before this PR whether to (a) keep current behaviour or (b) always show "Everything" as a top-of-list slot. Default: keep current behaviour to avoid scope creep; flag in PR description.
- [ ] Delete `WorkshopHero` / `WorkshopTicker` / `WorkshopSectionHeader` / `WorkshopPostcard` / `WorkshopShowMoreLink` from [workshop_home_layout.dart](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart) only if no other screen imports them; otherwise leave them. Grep for imports first — [season_report_screen.dart](../../app/lib/presentation/screens/workshop/season_report_screen.dart) is the suspect.

**[audit] Accessibility, i18n, and code-style requirements that apply to every widget added or modified in this phase** (per [docs/client/accessibility.md](../client/accessibility.md) and [docs/client/i18n.md](../client/i18n.md)):
- [ ] Every tappable surface uses `Tappable` (or `IconAction` for icon-only) — never bare `GestureDetector` / `InkWell` / `IconButton`. The new tappable impact rows and lean cards each need a `semanticsLabel` sourced from a new ARB key prefixed `a11yWorkshop*`.
- [ ] Every visible string and every `semanticsLabel:` argument comes from `context.l10n.*`. Add new keys to **both** [app_en.arb](../../app/lib/l10n/app_en.arb) and [app_es.arb](../../app/lib/l10n/app_es.arb), declare placeholders in the `@key` metadata block, run `npm run generate`. CI's `npm run lint:dart:i18n` rejects raw string literals.
- [ ] All colours via `AppColors.*(context)`; no hex literals, no `Colors.grey[*]`. Serif typography via `AppTheme.headingFont`.
- [ ] `CachedMediaImage` calls require `semanticsLabel:` (community avatar example: `context.l10n.a11yWorkshopCommunityAvatar(community.name)`).
- [ ] No raw `Navigator.push(MaterialPageRoute(...))` — all forward navigation routes through `NavigationHelpers` (the row taps already use this via the existing detail-screen entry points).

**[audit] Tests (per [docs/client/testing.md](../client/testing.md): ViewModel-first, no golden tests as a default):**
- [ ] **Primary coverage lives in [app/test/presentation/viewmodels/workshop_view_model_test.dart](../../app/test/presentation/viewmodels/workshop_view_model_test.dart)** — assert on the data shape the screen consumes (carousel ids, scope label, impact-row inputs derived from `BriefPayload`, lean-card row list). Use `ProviderContainer` with `overrideWithValue` on `workshopRepositoryProvider`; mock dependencies with `@GenerateMocks`. Cover the "Everything" / single-community / empty-state branches and a disposal-during-async case (`container.dispose()` while a `refresh()` future is in flight) per the architecture-doc's "Disposal Testing" pattern.
- [ ] Drop tests previously asserting on `WorkshopHero` / `WorkshopTicker` widget output. Do not replace them with golden tests — golden testing is explicitly out of current scope (testing doc §"Testing Weaknesses #7"). If a quick smoke widget test is needed for the new screen wiring, add a single `pumpWidget` test using `app/test/helpers/l10n_helpers.dart` for the localization wrapper.

- [ ] Update [app/lib/presentation/widgets/workshop/README.md](../../app/lib/presentation/widgets/workshop/README.md) to describe the new widgets and the rebinding; cross-link the shared `profile_section/` README from Phase 0.
- [ ] Run `npm run build`, `npm run test:app -- --reporter expanded`, `npm run lint:dart:a11y`, `npm run lint:dart:i18n`, `npm run lint:nav`.

**Validates:** The redesign reads as intended on a real device. The four data-complete sections render correctly across light/dark mode and for the "Everything" / single-community / empty-state cases. CI (a11y, i18n, nav, ephemeral-comments, file-size) is green.

### Phase 2: Identity subtitle with member rollup

**Goal:** The identity subtitle reads "With Mike, Diego, Sarah and N others · since April 2026" for a single community, and "With X, Y, Z and N others across 5 communities · since {earliest}" for "Everything".

**Tasks:**
- [ ] At the view-model edge in [workshop_view_model.dart](../../app/lib/presentation/viewmodels/workshop_view_model.dart), compose a `WorkshopIdentityRollup` (or extend `WorkshopState`) carrying `firstFewNames: List<String>`, `otherCount: int`, `scopeLabel: String` (singular community name or "across N communities"), `sinceUnixSec: int?`. Source the names from `communityMembersProvider` per enabled community via `Future.wait` (per [client_architecture.md](../client/architecture.md) §"Parallel Data Loading"). **[audit]** Because `workshopProvider` may be `autoDispose`, guard every `state = …` after the `await` with `if (!ref.mounted) return;` or migrate the notifier to `SafeNotifierMixin` (architecture doc §"Async Safety in Notifiers"). Translate any errors with `RpcErrorHandler.classify(e)` into a typed `UserError?` on state — viewmodels never resolve strings.
- [ ] Plumb the rollup into `WorkshopCommunityIdentity` and render the rich-text subtitle. **[audit]** All copy via `context.l10n.workshopIdentity*` and `context.l10n.workshopIdentity*OtherCount` with ICU `{count, plural, …}` placeholders for the "and N others" portion.
- [ ] Add ARB strings for the multi-community phrasing to both [app_en.arb](../../app/lib/l10n/app_en.arb) and [app_es.arb](../../app/lib/l10n/app_es.arb).
- [ ] **[audit]** ViewModel tests (not widget tests) for both single-community and "Everything" rollups, including the "no members" / "single member" / "one-community user" edge cases. Add a disposal-during-rollup test that calls `container.dispose()` while the names futures are pending. Use `@GenerateMocks` on the underlying providers' repositories.

**Validates:** The mock's exact subtitle copy is reproduced. The rollup is computed once at the view-model edge, not redone on every rebuild. Disposal is safe.

### Phase 3 **[updated — pulled out of follow-ups]**: Community specialty pills

**Goal:** The workshop screen renders a row of specialty chips above the impact rows. For a single-community scope, the chips describe that community's specialties ("Power tools lender", "Trail mornings"). For "Everything" scope, chips aggregate across the user's enabled communities, deduped.

**Server tasks** (per [docs/server/architecture.md](../server/architecture.md) §"Service Independence" — workshop service must not import the profile service):
- [ ] Extract the existing per-user derivation in [server/services/profile/known_for.go](../../server/services/profile/known_for.go) into a new shared library package `server/known_for/`. Function signature: `Derive(ctx, storage *storage.ProtoSQLStorage, communityIDs []string, opts Options) ([]string, error)`. Keep the threshold (2 completed loans/category), cap (6), normalization, and `"{Category} lender"` formatting. Add a `Reason` aggregation knob: per-user (existing) groups by `owner_user_id`; per-community (new) groups by the gear's community membership.
- [ ] Update [server/services/profile/profile.go](../../server/services/profile/profile.go) to call `known_for.Derive(...)` with the per-user `Reason` — pure refactor, behaviour unchanged. Add a snapshot test that the user response is byte-identical before and after.
- [ ] In the workshop service, call `known_for.Derive(...)` with the workshop's enabled community-id set and the per-community `Reason`. Populate a new `repeated string specialties` field on `BriefPayload` in [proto/ripls/api/workshop_service.proto](../../proto/ripls/api/workshop_service.proto). (`repeated` doesn't take `optional`; absence is `len == 0`.)
- [ ] Wire structured logging via `logging.LoggerWithContext(ctx)` with `operation=DeriveSpecialties` and `community_id_count=len(...)` fields. Use `connecterr.Internal(ctx, op, err, kv...)` for any returned internal error per [docs/server/architecture.md](../server/architecture.md) §"Error wrapping".
- [ ] Go unit tests for the `known_for` package (table-driven: threshold not met, threshold met, normalization collapses duplicates, cap honored, empty community set returns empty). Server build with `go test ./...` and `go test -tags=integration ./...` per [CLAUDE.md](../../CLAUDE.md) §"Go Server Build Verification".
- [ ] Run `npm run generate` and commit nothing under `server/gen/` (generated code is gitignored).

**Client tasks:**
- [ ] After regenerating the Dart protos, surface `BriefPayload.specialties` into the workshop view-model state.
- [ ] Render the chips between the identity block and the impact rows by passing `specialties` into the already-hoisted `KnownForChips` widget. Empty list → widget collapses to `SizedBox.shrink()` (existing behaviour).
- [ ] No new ARB strings unless the section gets an "X is known for" header (the mock shows pills without a header, so default to no header).
- [ ] ViewModel test that asserts on `state.specialties` for single-community / "Everything" / empty cases.

**Validates:** The chip row renders for at least one real community with completed loan history. The user-screen `known_for` response is unchanged. CI green.

### Phase 4 **[updated — pulled out of follow-ups]**: Workshop Available Now rail

**Goal:** The workshop screen renders the horizontal Available Now rail (UPCOMING / BORROW / REQUEST tiles) between the impact rows and the lean cards. Scope follows the carousel selection.

**Server tasks** (same service-independence rule):
- [ ] Extract the three gather functions in [server/services/profile/available_now.go](../../server/services/profile/available_now.go) (`gatherAvailableGear`, `gatherOpenRequests`, `gatherUpcomingExperiences`) into a new shared library at `server/available_now/`. Keep the existing ordering (recency / soonest-first for experiences) and cap (12). Function signature: `Gather(ctx, storage *storage.ProtoSQLStorage, communityIDs []string) ([]*apipb.AvailableNowItem, error)`. The library returns API protos directly — both callers will emit them on API responses.
- [ ] Refactor [server/services/profile/profile.go](../../server/services/profile/profile.go) to call `available_now.Gather(...)` with the shared-community set. Snapshot test the user response for byte-identical output.
- [ ] In the workshop service, call `available_now.Gather(...)` with the workshop's enabled community-id set. Populate a new `repeated AvailableNowItem available_now_items` field on `GetWorkshopBriefResponse`. Import the existing `AvailableNowItem` proto from [profile_service.proto](../../proto/ripls/api/profile_service.proto) — both protos live under `proto/ripls/api/` and the `api ↔ models` boundary in [CLAUDE.md](../../CLAUDE.md) doesn't apply between API protos.
- [ ] Structured logging: `operation=GatherAvailableNow`, `community_id_count`, `items_returned`. `connecterr.Internal` for storage errors.
- [ ] Go unit tests in the `available_now` package: each gather function in isolation (mock storage), plus an integration-tagged test that exercises the full path with a testcontainers Postgres. Tag the integration test with `//go:build integration` per [CLAUDE.md](../../CLAUDE.md) §"Testing".
- [ ] Re-run `go build -tags=integration ./...` per [CLAUDE.md](../../CLAUDE.md) §"Go Server Build Verification" since this change touches a function consumed by integration tests.

**Client tasks:**
- [ ] After regenerating the Dart protos, surface `availableNowItems` into the workshop view-model state.
- [ ] Render the rail between impact rows and lean cards by passing `availableNowItems` into the already-hoisted `AvailableNowRail` widget. Wire `onTapItem` to the existing detail-screen helpers (gear / experience / request) via `NavigationHelpers.pushToItemScreen` — the user screen already has this exact wiring at [user_screen.dart:120-150](../../app/lib/presentation/screens/users/user_screen.dart#L120-L150); copy that handler verbatim.
- [ ] Empty list → rail collapses to `SizedBox.shrink()` (existing behaviour).
- [ ] ViewModel test that asserts on `state.availableNowItems` for the scope branches plus a disposal-during-fetch test.
- [ ] No new ARB strings — the rail's section label `userProfileAvailableNowLabel` is already locale-neutral copy ("AVAILABLE NOW"); the audit recommends renaming the ARB key during Phase 0's hoist to drop the `userProfile` prefix, but the *value* stays the same.

**Validates:** Tapping a tile opens the correct detail screen. The user-screen rail is unchanged. CI green including the integration-tagged build.

### Phase 5 (follow-up issue): Per-community AI impact narratives

Filed as a separate issue; not blocking this redesign. Mirror the user-side narrative work from #1931. The tier fallbacks in [impact_row_narrative_tiers.dart](../../app/lib/presentation/widgets/user_profile/impact_row_narrative_tiers.dart) (hoisted into the shared `widgets/profile_section/` in Phase 0) are good enough until per-community narratives ship. New fields on `BriefPayload` use `optional string` so absence is distinguishable from empty per [CLAUDE.md](../../CLAUDE.md) §"Proto Field Conventions".

**Validates:** Independently shippable; the workshop home picks it up by reading the new optional narrative fields with no other change.

## Audit Findings (2026-05-17)

Material changes from the pre-audit draft, with the architectural rule each one comes from:

| # | Change | Source rule |
|---|--------|-------------|
| 1 | **Recommendation flipped from "visual mirror" (Option A) to "hoist + rename the four surface-agnostic widgets" (a hybrid of A and B).** The pre-audit draft proposed duplicating layout into `WorkshopImpactRows` / `WorkshopLeanCard` — that's the "fork the look-and-feel" pattern the issue body explicitly prohibits. Hoisting the four widgets (chips, impact rows, available-now rail, lean cards) into `widgets/profile_section/` with minimal parameterization keeps a single source of truth without forcing the over-engineered shared-state contract of full Option B. The lean-cards dispatcher problem is solved with an `onTapRow` callback — each surface passes its own dispatcher. | Issue body: *"Reuse, don't fork."* Combined with the architecture doc's "no half-finished implementations" / "three similar lines beats premature abstraction." |
| 2 | **New explicit Phase 0** to perform the hoist as a pure refactor before any workshop-facing change lands. | [docs/client/architecture.md](../client/architecture.md) §"UI Structure" — `git mv` for renames per CLAUDE.md, reviewable in isolation. |
| 3 | **Tests redirected from "golden tests, smoke tests" to ViewModel-first using `ProviderContainer` + `@GenerateMocks`.** Pre-audit recommended golden tests; testing doc explicitly lists golden tests as out-of-scope (§"Testing Weaknesses #7") and says "Test ViewModels, not widgets." Added disposal-during-async test per the architecture doc's "Disposal Testing" requirement. | [docs/client/testing.md](../client/testing.md) §"Testing Philosophy" + [docs/client/architecture.md](../client/architecture.md) §"Disposal Testing". |
| 4 | **Accessibility requirements made explicit** — every new tappable surface uses `Tappable` (not `GestureDetector`); every `semanticsLabel:` value is sourced from `context.l10n.a11y*`; `CachedMediaImage` calls require `semanticsLabel:`. Pre-audit was silent on a11y. | [docs/client/accessibility.md](../client/accessibility.md) §"When to reach for which primitive" + the `scripts/check_a11y.js` ratchet. |
| 5 | **i18n requirements made explicit** — both `app_en.arb` and `app_es.arb` updated, ICU plurals for "and N others", `npm run generate` after, viewmodels never resolve strings (`UserError?` on state with `RpcErrorHandler.classify` / `localize`). Pre-audit only said "add ARB strings" without locale parity. | [docs/client/i18n.md](../client/i18n.md) §"Best Practices". |
| 6 | **Async-safety call-out** in Phase 2 — `ref.mounted` guards after every `await` in the rollup composition, or migrate the workshop notifier to `SafeNotifierMixin` / `AsyncNotifier`. Pre-audit was silent on the autoDispose disposal hazard, which is the most common crash class per the architecture doc. | [docs/client/architecture.md](../client/architecture.md) §"Async Safety in Notifiers". |
| 7 | **File-organization call-out** — each new widget lands in its own file under `widgets/workshop/` rather than being added to the already-635-line `workshop_home_layout.dart`. | [CLAUDE.md](../../CLAUDE.md) §"File Organization" + Dart 1,000-line gate. |
| 8 | **Navigation explicit** — all forward navigation routes through `NavigationHelpers`; no raw `Navigator.push(MaterialPageRoute(...))`. | [docs/client/architecture.md](../client/architecture.md) §"Navigation & Gestures" + `npm run lint:nav`. |
| 9 | **Repository / provider seam preserved** — `WorkshopCommunityIdentity` reads avatar media through `mediaObjectProvider`, not a service call, matching what `UserProfileAvailableNowRail` already does. | [docs/client/caching.md](../client/caching.md) §"Pattern 3: Dependency Injection with Riverpod." |
| 10 | **Phase 3 follow-ups annotated with server-side compliance reminders** — structured logging via `logging.LoggerWithContext(ctx)`, `connecterr.Internal` for internal errors, `optional` qualifier on new presence-tracked fields. | [docs/server/observability.md](../server/observability.md) + [docs/server/architecture.md](../server/architecture.md) §"Error wrapping" + [CLAUDE.md](../../CLAUDE.md) §"Proto Field Conventions". |
| 11 | **Carousel "Everything" behaviour flagged for decision** — research found the current carousel hides "Everything" when the user has only one community; the JSX mock shows it always. Pre-audit treated this as no-op; audit pulls it into the plan as a question to resolve before the PR. | Avoid scope creep + match-the-mock fidelity, surfaced in the issue body. |
| 12 | **Lint sweep added to the validation step** — `npm run lint:dart:a11y`, `lint:dart:i18n`, `lint:nav`, plus existing `npm run build` / `flutter analyze` / `npm run test:app -- --reporter expanded`. | [docs/client/accessibility.md](../client/accessibility.md), [docs/client/i18n.md](../client/i18n.md), [CLAUDE.md](../../CLAUDE.md) §"Testing". |

No changes were made to the problem statement, the related-issues list, or the section-parity table's first five rows — those held up against the reference docs.

### Scope expansion (2026-05-17, second pass)

After the initial audit, the user asked to **pull the specialty pills and Available Now rail into this issue** rather than deferring them. Server research confirmed both features already exist for the user surface and are community-aware: [server/services/profile/known_for.go](../../server/services/profile/known_for.go) filters loan history by shared communities, and [server/services/profile/available_now.go](../../server/services/profile/available_now.go) already scopes its three gather functions by community set. The work is therefore a service-independence-respecting refactor (extract to `server/known_for/` and `server/available_now/`) plus two thin wire-ups (workshop service call + new proto field on `BriefPayload` / `GetWorkshopBriefResponse`), not a from-scratch design. Phases 3 and 4 were moved from "follow-up" to "in-scope" and the section-parity table's rows 4 and 6 were updated to point at the now-extracted library packages. Per-community AI narratives stay deferred as Phase 5 — that one *is* a meaningful new design (#1931 mirror).

## Related Issues

- **#1929 — Profile v4 redesign** — established the section structure (identity / known-for / impact rows / available-now / lean cards) this redesign mirrors. Plan: [docs/issues/1929-profile-v4-redesign.md](1929-profile-v4-redesign.md).
- **#1930 — Per-user "known for" signal** — the server signal behind `UserProfileKnownForChips`. The follow-up issue for community specialty pills should reference this as the prior art.
- **#1931 — Per-user AI narratives for impact rows** — same shape as the follow-up community-narrative work.
- **#1996 — Self-view items** — established how `UserProfileAvailableNowRail` handles the no-shared-community case; the community Available Now follow-up should mirror that gating.
- **#1898 — Workshop top strip / unique-people-count migration** — the precedent for ticker-cell taps routing to detail screens; the new impact-row taps inherit the same destinations.
