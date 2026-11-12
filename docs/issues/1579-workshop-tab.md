# Issue #1579: Workshop tab: replace Impact screen with proactive host workshop (v1 of impact-proactive-host)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1579
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:**
- [docs/cowork/App UXR/impact-screen-proactive-host-brief.md](../cowork/App%20UXR/impact-screen-proactive-host-brief.md)
- [docs/cowork/App Design/impact-proactive-host.jsx](../cowork/App%20Design/impact-proactive-host.jsx)
- [docs/users/host-problems.md](../users/host-problems.md)
- [docs/users/priorities.md](../users/priorities.md)

## Problem Statement

The 5th tab in the app currently renders [PortfolioMetricsScreen](../../app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart) — a portfolio-metrics dashboard that serves reflection but exposes no levers. Per the brief, the latent host's binding constraint is bandwidth, and the tab should be a *proactive workshop* with one-tap actions for the six host-side jobs they actually go there to do (latent opportunities, pulse check, continuity decision, sole-organizer fatigue, pattern blindness, decay detection).

This issue ships **Direction B** from the brief: engine wrapped around jobs, with surfacing discipline keeping at most 2–3 sections active at a time. v1 covers the host side end-to-end. The Mike-said-yes recipient handoff (Phase 1b in the brief), the once-per-season hero, side-door / first-touch / brand-new / in-care state variants (Phase 2), and full all-circles aggregation (Phase 3) are explicitly out of scope.

**Note on the tab's current label.** The issue body refers to renaming the tab from "Impact" to "Workshop." The codebase does not currently expose a visible "Impact" string for this tab; the a11y semantic label is `a11yMiscNavRipls` and the icon is `RiplsIcons.ripls` ([home_screen.dart:174](../../app/lib/presentation/screens/home/home_screen.dart#L174)). The plan treats this as adding a `Workshop` label and routing the 5th tab to a new `WorkshopScreen`, leaving the existing `PortfolioMetricsScreen` reachable through a new non-tab entry point.

**Note on missing existing entry points.** The Explore survey found no non-tab entry points to `PortfolioMetricsScreen` today — it is reachable only through the 5th tab. The issue's requirement that "the metrics screens still reachable from non-tab entry points" therefore means *adding* one as part of this work. The plan does so via a settings/profile drawer link.

## Context from Comments

The issue has no comments. Surrounding context comes from the brief (rev 4), the JSX mock, and the host-problems map. All three are dense; the brief's Working Spec section is the reference of record where prose conflicts arise.

A companion issue, **#1580 (lightweight ESM cards)**, supplies signal for cascade priority (2) *strong ESM repeat signal ≥60%* via the new `esm_service.GetESMSignal` RPC. Cascade priority (3) *calendar gap near a recurring rhythm* is **not** ESM-driven — it's derived from existing experience-completion history. v1 of the Workshop reads from a hand-curated fixture for all signal until #1580's pipeline and the cascade-(3) calendar-gap detector are wired; the fixture's JSON shape mirrors the production read API for each seam (the `ESMSignal` shape from #1580, the calendar-gap aggregation derived from existing experience storage) so the swap to real data is one repository implementation change, not a UI change.

## Potential Solutions

### Option A: Rename tab and rebuild from scratch in app/lib/presentation/screens/workshop/

- **Approach:** Add `WorkshopScreen` under [app/lib/presentation/screens/workshop/](../../app/lib/presentation/screens/workshop/), point the 5th tab to it, and add a settings entry to keep `PortfolioMetricsScreen` reachable. Build new components (Quest hero card, Crew strip with load overlay, Bring Something Back card, surfacing discipline, circles pill carousel) as widgets under `app/lib/presentation/widgets/workshop/`. Lever taps **do not open a new review modal**; they generate a draft item server-side and route into the existing preview flows ([experience_preview_modal.dart](../../app/lib/presentation/screens/experience/experience_preview_modal.dart) and the request/offer equivalents) for edit-and-publish. No parallel create/review chrome. Use `AsyncNotifier<WorkshopState>` + freezed state per the existing pattern in [portfolio_metrics_view_model.dart](../../app/lib/presentation/viewmodels/portfolio_metrics_view_model.dart). Stub the server data layer with a `WorkshopRepository` that reads from a local fixture file in v1 so the UI can ship without the backend. The repository is the only caller of any service it ends up needing, per the strict MVVM layering in [docs/client/architecture.md](../client/architecture.md) (ViewModels → Repositories → Services).
- **Pros:** Clean separation from the existing portfolio screens; easy to gate behind a feature flag; the UI is fully testable without server changes; contract with #1580 stays narrow because the repository is the seam.
- **Cons:** No real signal in v1 — the screen is dressed-up stub data until ESM lands. Mitigated by sequencing #1580's read API ahead of the Workshop's signal-driven cascade priorities.
- **Effort:** Medium-Large.

### Option B: Layer the Workshop on top of the existing portfolio infrastructure

- **Approach:** Reuse `PortfolioRepository` + extend `PortfolioMetricsState`/`PortfolioMetricsNotifier` to include workshop surfaces. Keep one viewmodel; one screen with mode switching.
- **Pros:** Less new code; fewer providers to wire.
- **Cons:** Conflates two unrelated jobs (portfolio reflection vs. proactive workshop) into one viewmodel that already does several things; risks the 1000-line cap on either the screen or the state class; makes the Phase 2/3 work harder. The brief is explicit that the workshop is *not* the same job as the portfolio metrics — bundling them violates Principle 1 (*Workshop, not dashboard*).
- **Effort:** Medium, but with the larger Phase 2/3 cleanup cost amortized in.

### Option C: Build the screen as a single large `WorkshopScreen` widget

- **Approach:** Same as A but without splitting into per-component widgets up front.
- **Pros:** Faster initial walkthrough.
- **Cons:** Will hit the 1000-line cap from CLAUDE.md the moment the four lever variants and three Crew strip states are wired. Splitting later costs more than splitting up front.
- **Effort:** Medium initially, large after the inevitable refactor.

## Recommendation

**Option A.** The brief's whole point is that the workshop and the portfolio dashboard are different jobs; the implementation should reflect that. Building under a new `workshop/` namespace gives clean ownership of the surface and keeps the seam to #1580 explicit (one repository method, mockable). Splitting widgets up front avoids the 1000-line cap. The settings entry that keeps `PortfolioMetricsScreen` reachable is a small concession — a single drawer/profile menu link — but preserves the existing investment.

The biggest tradeoff is shipping a Workshop UI on stub data before #1580's pipeline lands. This is acceptable because (a) the brief's Phase 1a validation explicitly uses mocked states, (b) shipping the host-facing surface unlocks the user research the brief calls for, and (c) #1580 can swap its real data in behind the same `WorkshopRepository` interface without UI changes.

**Reconciliation with nudges and stories (2026-04-29 — load-bearing).** Workshop suggestions are structurally `StoredNudge`s on a different surface, and the once-per-season hero is a `Story` on a particular schedule. Rather than inventing parallel data models, the Workshop reuses both. See [docs/nudges.md](../nudges.md) for the existing nudge data model. Specifically:

- **Quest hero variants, Bring Something Back items, and the seed-the-catalyst prompt** are persisted as `StoredNudge` rows with a new `surface` enum value (`WORKSHOP_QUEST_HERO`, `WORKSHOP_BRING_BACK`, `WORKSHOP_SEED_CATALYST`) and may use new `cta_action` values (`schedule_repeat`, `revive_experience`, `propose_share`, `seed_subhost`). Generation, consumption tracking, soft-delete, and async stock imagery all reuse the existing nudge pipeline at [server/services/feed/nudges.go](../../server/services/feed/nudges.go) and friends.
- **The once-per-season hero** is a `Story` with a new category (e.g., `season_summary`); the Workshop reads stories matching that category for the active circle scope.
- **Tapping any Workshop tile slides the existing nudge or story detail view in from the right** via `NavigationHelpers.pushScreen()`, reusing the existing `NudgeContentView` / `StoryContentView` widgets wrapped in a pushable screen with `SwipeToCloseMixin`. The Workshop becomes a curation layer; the actual interaction (CTA tap → existing creation/preview modal) is unchanged from the feed surface.
- **Catalyst-pull stays its own object.** Recipient eligibility, rate limits, and host-private visibility don't fit the nudge model and would pollute it. The catalyst-pull tile in the Crew strip is the only Workshop surface that does *not* route to an existing nudge/story detail view; it routes to a Workshop-specific catalyst-pull review screen (specified in Phase 3).

## Open Questions

- **Does the renamed tab break analytics dashboards / event-name conventions?** — Recommendation: keep any existing analytics event names that include "impact" or "ripls" stable to avoid breaking dashboards; rename only the user-visible string and a11y label. → **Decided (2026-04-29):** Reuse `impact` naming for client + server analytics/metrics infrastructure (don't expose to the user); use `workshop` naming for everything new — including this screen and any new client/server code paths it introduces. The user-visible label is *Workshop*; the underlying analytics seam keeps its existing names so dashboards don't churn.
- **Where does `PortfolioMetricsScreen` live after losing the tab?** — Recommendation: add a "Your impact" entry to the profile/settings drawer. → **Decided (2026-04-29):** Add the link from user settings now; revisit placement later. Navigation uses `NavigationHelpers.pushWithSlide()` per [docs/client/architecture.md](../client/architecture.md) — never raw `Navigator.push(MaterialPageRoute(...))` or `context.push()`.
- **What server-side aggregation is needed to drive the load-distribution overlay?** — Recommendation: a single read-only RPC (`GetCatalystSuggestions`) returning load signal + eligible recipients + no-eligible flag; eligibility logic in a new `server/catalyst/` shared library, RPC handler is a thin wrapper. → **Decided (2026-04-29):** Proceed with the recommendation.
- **Where do the Quest hero cascade priorities (2) and (3) get their signal in v1?** — Recommendation: hand-curated fixture for both, with shapes that mirror their respective production read APIs. Cascade (2) consumes `esm_service.GetESMSignal` from #1580 — the fixture entry per-experience matches `ESMSignal{prompt_id, respondent_count, response_distribution: map<option_key, count>, free_text_count, completed_at_unix_sec}`; the Workshop applies the ≥60% threshold by computing `response_distribution[<repeat_option_key>] / respondent_count`. Cascade (3) is **not** ESM-driven — it's a calendar-gap signal derived from existing experience completion history (no new RPC needed; the Workshop server can compute it from `Experience` storage). → **Decided (2026-04-29):** Proceed with the recommendation.
- **Which `option_key` represents "repeat" / "do this again" in #1580's `response_distribution` map?** — Recommendation: by convention, operators authoring repeat-signal prompts use the option key `do_again`. The Workshop's threshold check looks up that exact key. Document the convention in the WorkshopRepository fixture README and in #1580's runbook (`docs/runbooks/esm-manual-authoring.md`) so prompt authors and signal consumers agree. v2's automated trigger picks the same key when generating prompts from templates.
- **Is the `selectedCommunityProvider` the right scope, or does the Workshop need its own circle picker state?** — → **Decided (2026-04-29):** The top pill carousel renders the same circles already selected/visible in the sidebar (no separate Workshop selection state, no per-screen-stash). The pills show *All*, then one pill per sidebar-selected community in the order they appear. Selecting a pill filters the Workshop content to just that community; *All* is the default and shows unioned content across the sidebar-selected set. Implementation: a derived provider (e.g., `workshopCirclePillsProvider`) reads the sidebar's selection state and produces the pill list; a separate `workshopActivePillProvider` tracks the active filter (defaults to *All*). No `selectedCommunityProvider` mutation from the Workshop.
- **Do haptics on lever taps require platform-specific plumbing?** — → **Decided (2026-04-29):** Do not add haptics. (Removed from Phase 2.)
- **What is the "voice signal" capture format for the learning loop in v1?** — → **Decided (2026-04-29):** Punt. The brief's learning-loop affordance is deferred to a follow-up issue; v1 ships without capturing host edits. (Removed from Phase 2.)
- **How are localized Workshop strings boundaried between widgets and viewmodels?** — Recommendation: viewmodels expose typed enums (`LeverKind`, `LoadOverlayCase`); widgets resolve via `context.l10n.*`. → **Decided (2026-04-29):** Proceed with the recommendation.
- **Are color choices theme-aware?** — → **Decided (2026-04-29):** Reuse existing `AppColors` and font tokens wherever possible; the screen must render correctly in both light and dark themes. Add new `AppColors` entries only if a Workshop-specific semantic color is genuinely missing — never inline hex or `Colors.grey[*]`. The JSX mock's parchment palette is a reference for layout density, not a literal palette to copy.

## Implementation Plan

### Phase 1: Tab plumbing and shell

**Goal:** The 5th tab shows a Workshop placeholder backed by a real screen widget and viewmodel; the existing portfolio metrics remain reachable from a non-tab entry point.

**Tasks:**
- [ ] Add `a11yMiscNavWorkshop` (and any visible "Workshop" label string) to [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb) and [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb).
- [ ] Create [app/lib/presentation/screens/workshop/workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart) as a `ConsumerWidget` placeholder that renders an empty Scaffold with the circle pill carousel.
- [ ] Circle pill carousel reads from a derived `workshopCirclePillsProvider` that mirrors the sidebar's currently-selected communities (in sidebar order) and prepends an *All* pill. A `workshopActivePillProvider` (autoDispose) tracks the active filter; default is *All*. The Workshop never mutates `selectedCommunityProvider` or the sidebar's selection — pill changes only narrow the Workshop's view.
- [ ] Create [app/lib/presentation/viewmodels/workshop_view_model.dart](../../app/lib/presentation/viewmodels/workshop_view_model.dart) with `@freezed WorkshopState` and `WorkshopNotifier extends AsyncNotifier<WorkshopState>`. Mirror the structure of [portfolio_metrics_view_model.dart](../../app/lib/presentation/viewmodels/portfolio_metrics_view_model.dart).
- [ ] Wire the 5th tab in [home_screen.dart](../../app/lib/presentation/screens/home/home_screen.dart) (visual index 4 → stack index 3) to render `WorkshopScreen` instead of `PortfolioMetricsScreen`. Update the icon and a11y label.
- [ ] Add a "Your impact" entry in the profile/settings drawer (or equivalent) that pushes `PortfolioMetricsScreen` via `NavigationHelpers.pushWithSlide()`. Do not use `Navigator.push(MaterialPageRoute(...))` or `context.push()` — both violate the navigation conventions in [docs/client/architecture.md](../client/architecture.md) and will be flagged by `npm run lint:nav`. Confirm the existing portfolio routes still work.
- [ ] `WorkshopRepository` skeleton at [app/lib/data/repositories/workshop_repository.dart](../../app/lib/data/repositories/workshop_repository.dart) with all read methods (latent opportunities, catalyst suggestions, bring-something-back items) returning fixture data from a single JSON file under [app/lib/data/datasources/](../../app/lib/data/datasources/). The repository wraps `CacheService(cacheManager, 'workshop')` per [docs/client/caching.md](../client/caching.md); cache keys follow `'workshop:cascade:<circle_id>'`, `'workshop:catalyst:<circle_id>'`, `'workshop:revival:<circle_id>'`. All methods are async and return Futures (no `getSync()`). Constructor takes `CacheManager` and any service dependencies, exactly like [GearRepository](../../app/lib/data/repositories/gear_repository.dart). Repository depth stays shallow (max 1 level) — if cross-data composition is needed, do it at the viewmodel layer, not by chaining repositories.
- [ ] Provider wiring: `workshopRepositoryProvider` (Provider), `workshopProvider` (`AsyncNotifierProvider.autoDispose<WorkshopNotifier, WorkshopState>`). The viewmodel uses `ref.read(workshopRepositoryProvider)` — never `ref.read(<service>Provider)` directly, per the strict ViewModel → Repository layering.

**Validates:** The shell switches cleanly without breaking portfolio metrics; the viewmodel/repository scaffold is in place for the next phase to fill.

### Phase 2: Workshop tiles + tap-to-detail into existing nudge/story screens

**Goal:** Quest hero variants render as compact tiles that tap into the existing `NudgeContentView`. The Workshop curates which nudges to show in cascade priority order; rendering and interaction are unchanged from the feed surface.

**Critical design decision (2026-04-29):** Workshop suggestions are `StoredNudge` rows with a workshop `surface` tag. The Workshop tile is a small summary; tapping it slides the existing nudge detail view in from the right via `NavigationHelpers.pushScreen()`. The CTA inside the nudge view does what nudge CTAs already do — open the existing creation/preview modal with a pre-populated draft. **No new nudge generation pipeline. No new draft-generation RPC. No new lever review chrome.** The Workshop's job is curation (which nudges to show, in what order, with what surfacing discipline), not rendering.

**Tasks (server side — extend existing nudge infrastructure):**
- [ ] Add a `surface` enum to [proto/ripls/models/nudge.proto](../../proto/ripls/models/nudge.proto) with values `FEED` (default, unset = FEED for backward compatibility), `WORKSHOP_QUEST_HERO`, `WORKSHOP_BRING_BACK`, `WORKSHOP_SEED_CATALYST`. Run `npm run generate`.
- [ ] Add new `cta_action` values to the existing nudge action enum: `schedule_repeat`, `revive_experience`, `propose_share`, `seed_subhost`. Each maps to a creation-modal entry the same way existing actions map (per [docs/nudges.md](../nudges.md) action table). For Workshop nudges, the AI generation prompt should pre-populate richer draft context (prior instance ID, attendee list) so the resulting creation/preview modal has more to work with than a feed-surfaced nudge.
- [ ] Extend [server/services/feed/nudges.go](../../server/services/feed/nudges.go) (or factor into a sibling `workshop_nudges.go` if it pushes the file over 1000 lines) so `generateAndPersistNudges` accepts a `surface` parameter. Workshop-surface generation reads cascade-priority signal (active quest detection, ESM repeat signal from #1580's `esm_service.GetESMSignal`, calendar-gap detection, seasonal triggers, idle-offer detection) and emits one `StoredNudge` per matched priority slot.
- [ ] **Cascade priority logic lives server-side** in the nudge generation pipeline, not on the client. The five priority slots map to detection rules; the highest-priority detected slot becomes the Quest hero nudge for that circle. Bring Something Back items become `WORKSHOP_BRING_BACK` nudges. Seed-the-catalyst becomes `WORKSHOP_SEED_CATALYST`.
- [ ] Storage helpers in [server/services/feed/nudge_storage.go](../../server/services/feed/nudge_storage.go) gain a `QueryWorkshopNudgesByCircle(circleID, surface)` method. Index on `(community_id, surface, consumed_action)` for the read.
- [ ] New RPC on the workshop service (introduced in Phase 4): `GetWorkshopFeed(circle_id) → {quest_hero: NudgePayload?, bring_something_back: list<NudgePayload>, crew_strip: CrewStripPayload, seed_catalyst: NudgePayload?, season_hero: StoryPayload?}`. The response is the curated set of existing payload types — no new payload shapes for the curation surface itself.
- [ ] Lever-copy guard test: load any persisted Workshop-surface nudge's `cta_label` and assert ≤9 words + concrete-object rule from the brief. The same guard runs on Workshop fixtures used by the client until the server pipeline ships.

**Tasks (client side — reuse existing nudge/story rendering):**
- [ ] Wrap the existing [nudge_content_view.dart](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart) in a pushable screen at [app/lib/presentation/screens/nudge/nudge_detail_screen.dart](../../app/lib/presentation/screens/nudge/nudge_detail_screen.dart) using `SwipeToCloseMixin` per [docs/client/architecture.md](../client/architecture.md). The screen renders nothing new — it just gives `NudgeContentView` a Scaffold + slide-in/swipe-to-close lifecycle. The feed continues to render `NudgeContentView` inline (no behavior change for feed).
- [ ] Same wrap for stories: [app/lib/presentation/screens/story/story_detail_screen.dart](../../app/lib/presentation/screens/story/story_detail_screen.dart) with `SwipeToCloseMixin` over the existing `StoryContentView`. (If a story detail screen already exists, reuse it.)
- [ ] Create [app/lib/presentation/widgets/workshop/workshop_tile.dart](../../app/lib/presentation/widgets/workshop/workshop_tile.dart) — a compact tile that takes a `NudgePayload` (or `StoryPayload` for the season hero) and renders the headline, kicker/atmosphere line, and a thumbnail. Tap action: `NavigationHelpers.pushScreen(NudgeDetailScreen(nudge: payload))` (or the story equivalent). One tile widget for all Workshop surfaces; visual variant driven by the surface enum on the payload.
- [ ] Workshop screen composition: render the curated set returned by `GetWorkshopFeed` — Quest hero tile (if present) at the top, Crew strip below (host-only, see Phase 3), Bring Something Back row, season hero (if present). All non-Crew tiles use `WorkshopTile`.
- [ ] `WorkshopRepository.getFeed(circleId)` calls `GetWorkshopFeed`. Cache namespace `'workshop'`, key `'workshop:feed:<circle_id>'`. v1 may stub from a fixture whose payloads match `NudgePayload` and `StoryPayload` shapes exactly; the swap to real data is one method body.
- [ ] Async safety: any non-`build()` notifier method that performs an `await` and then writes to `state` must guard with `if (!ref.mounted) return;` per the Async Safety section in [docs/client/architecture.md](../client/architecture.md).
- [ ] Theme: `WorkshopTile` reuses existing `AppColors` and font tokens — and importantly reuses the existing nudge text styles (`nudgeHeadlineMediumStyle`, `nudgeBodyStyle` from [app_theme.dart](../../app/lib/core/theme/app_theme.dart) per [docs/nudges.md](../nudges.md)) for consistency between the Workshop tile and the detail view it opens. Light + dark theme aware.
- [ ] Tests: golden tests for `WorkshopTile` rendering each surface variant in light and dark themes; widget test that tap → `NudgeDetailScreen` opens via `NavigationHelpers.pushScreen` (slide-from-right transition) and the existing CTA dispatcher fires correctly. Disposal test for `WorkshopNotifier`.

**Out of Phase 2 (deferred to follow-up issues):**
- Haptic feedback on tile/CTA tap.
- Voice-signal capture for the learning loop.

**Validates:** Workshop suggestions render and dispatch through the existing nudge/story infrastructure. Tap-to-detail uses the standard slide-from-right navigation. The Workshop's curation logic (cascade priorities) lives where signal also lives — the server-side nudge generation pipeline.

### Phase 3: Crew strip + catalyst-pull (the only Workshop-specific object) + surfacing discipline

**Goal:** Workshop renders the Crew strip with correct surfacing discipline. Catalyst-pull is the one Workshop surface that does *not* reuse nudges — it has its own data model, RPC, and review screen because of recipient eligibility, rate limits, and host-private visibility.

**Tasks (catalyst-pull — stays separate):**
- [ ] Create [app/lib/presentation/widgets/workshop/crew_strip.dart](../../app/lib/presentation/widgets/workshop/crew_strip.dart) with three states: standard (no overlay), Case A (relieving someone else), Case B (host-as-Sarah). Copy comes from l10n; cases driven by viewmodel-supplied state. The Case A/B overlay tile, when tapped, slides a `CatalystPullReviewScreen` in from the right via `NavigationHelpers.pushScreen()` — same navigation pattern as the nudge detail screen, different destination.
- [ ] Catalyst-pull eligibility filtering, recipient rate limit (3 catalyst-pulls in 14 days across all hosts/circles), and decline cooldown (30 days for the same recurring thing) are **server-side concerns** owned by the catalyst suggestions RPC defined in Phase 4. The client receives only filtered, rate-limit-respecting suggestions; it never sees the rejected candidates. No client-side cache maps for these rules.
- [ ] No-eligible-recipient branch: when the load is genuinely lopsided but no eligible recipient exists, the catalyst RPC returns a `seed_catalyst` `StoredNudge` instead of a load overlay. The Crew strip silently does not show the overlay; the Quest hero instead surfaces the seed-the-catalyst nudge through the nudge tile pattern from Phase 2. **Reconciliation note:** the seed-the-catalyst surface is a `WORKSHOP_SEED_CATALYST` nudge, not a Workshop-specific object — only the catalyst-pull message itself stays separate.
- [ ] `CatalystPullReviewScreen`: shows the AI-drafted host-voiced direct message to the proposed recipient, supports inline edit, and a single-tap send. The screen is Workshop-specific (no existing surface routes to it) because the message is a host-to-Mike DM, not a feed-rendered card. Send routes through the existing chat infrastructure with a pre-populated message body.

**Tasks (Bring Something Back — collapses into nudges):**
- [ ] No new "bring something back" widget. Bring Something Back items are `WORKSHOP_BRING_BACK` nudges rendered with `WorkshopTile` (defined in Phase 2). Tap routes to `NudgeDetailScreen`. The brief's *flat* vs. *tiered* layout test happens at the tile-collection level on the Workshop screen — not by introducing a new widget.

**Tasks (surfacing discipline + language guards):**
- [ ] Implement surfacing discipline at the top-level `WorkshopScreen`: choose 1–3 sections to show in full content based on signal; the rest collapse to one-line affordances. Three layout snapshots (hot day, average day, quiet day) tested against fixtures.
- [ ] Add a banned-words guard: a unit test that fails if any l10n string under the workshop namespace **OR any persisted `StoredNudge.headline/description/cta_label` for a workshop surface** contains *quiet*, *decay*, *dropped*, *inactive*, *fading*, *we need someone*, *take a turn*, *rotation*, or *community*. The guard test runs server-side against the AI generation prompt validation step, and client-side against l10n strings.
- [ ] Tests: golden tests for the three Crew strip states; widget tests for the three Workshop layout snapshots; lint test for banned words; widget test that catalyst-pull tile tap → `CatalystPullReviewScreen` push works; widget test that Bring Something Back tile tap → `NudgeDetailScreen` push works.

**Validates:** The full Workshop surface is operable on stub data with surfacing discipline holding under all three signal volumes. The single Workshop-specific object (catalyst-pull) is cleanly isolated; everything else flows through the existing nudge/story infrastructure.

### Phase 4: Production read seams and stub-to-real data swap

**Goal:** Each production read seam is concrete enough that real data can swap in without UI changes. The Workshop has three seams in v1:

1. **ESM repeat signal** for cascade priority (2) — owned by #1580 via `esm_service.GetESMSignal(experience_id)`.
2. **Calendar-gap signal** for cascade priority (3) — derived from existing `Experience` storage; not owned by #1580.
3. **Catalyst suggestions** (load distribution, eligibility, rate limit) — owned by this issue via `workshop_service.GetCatalystSuggestions(circle_id)`.

**Tasks:**
- [ ] Document the JSON shape of the WorkshopRepository fixture in a README at [app/lib/data/repositories/](../../app/lib/data/repositories/). For each cascade priority and each card type, document which production read API it maps to and the field-by-field shape correspondence. Specifically: cascade (2) entries match the `ESMSignal` shape from #1580; cascade (3) entries match the calendar-gap shape this issue defines; catalyst-suggestions entries match the `GetCatalystSuggestions` response. Document the `do_again` `option_key` convention used to identify the "repeat" response in `ESMSignal.response_distribution` (also documented in #1580's [docs/runbooks/esm-manual-authoring.md](../runbooks/esm-manual-authoring.md)).
- [ ] Wire `WorkshopRepository.getCascade(circleId)` to call `esm_service.GetESMSignal` for each recently-completed experience in the circle (within 7 days), then apply the ≥60% threshold using the `do_again` key. v1 may stub this call with the fixture; the swap is one method body, not a contract change. **Privacy note:** `GetESMSignal` is host-only per #1580's authorization — `WorkshopRepository` only calls it when the viewing user is the experience host; for non-host views, cascade priority (2) is silently skipped.
- [ ] Server-side scaffolding for `GetCatalystSuggestions`: define the RPC request/response messages in [proto/ripls/api/](../../proto/ripls/api/) under a new `workshop_service.proto` (new RPC, new dedicated request/response types — never reuse messages across RPCs per [CLAUDE.md](../../CLAUDE.md) and [docs/proto_conventions.md](../proto_conventions.md)). The eligibility logic lives in a new `server/catalyst/` shared library (Pattern 1 from [docs/server/architecture.md](../server/architecture.md)); the RPC handler in `server/services/workshop/` is a thin wrapper. Workshop service does **not** depend on the ESM service — both depend on storage and on their respective shared libraries (`server/catalyst/`, `server/esm/`). Unit tests cover both library and handler. File size <1000 lines per server file.
- [ ] Server-side scaffolding for the calendar-gap signal: a helper in `server/catalyst/` (or a new `server/momentum/` shared library if the catalyst grouping doesn't fit) that, given a circle ID, returns recurring rhythms with detectable gaps from existing `Experience` rows. Pure function over storage; unit-testable independently. Wired into `GetCatalystSuggestions` or surfaced as its own RPC — pick the simpler shape during implementation.
- [ ] Behind a feature flag, swap the WorkshopRepository's source from the fixture to the real RPCs. Default off in v1; flipping it on requires #1580's `esm_service` to be deployed and prompt rows to exist.
- [ ] Phase 1a validation per the brief: 5 host interviews (with 2 hosts recruited specifically for the host-as-Sarah case). Capture the bandwidth check (*did this give the host more work or take work off their plate?*) and the language check (*did the host-as-Sarah variant land as relief or guilt?*).

**Validates:** The Workshop can ship to a small beta on real signal once #1580 closes its loop and the calendar-gap detector lands. Phase 1a validation gates the move from internal-only to broader rollout.

**Reconciliation update (2026-04-29):** Phase 4's read seams collapse with the nudge/story reconciliation. The client reads only `GetWorkshopFeed(circle_id)` returning `{quest_hero: NudgePayload?, bring_something_back: list<NudgePayload>, crew_strip: CrewStripPayload, seed_catalyst: NudgePayload?, season_hero: StoryPayload?}`. The cascade-priority logic that consumes ESM signal (priority 2), calendar-gap (priority 3), active-quest detection (priority 1), seasonal triggers (priority 4), and idle-offer detection (priority 5) all live **server-side in the nudge generation pipeline** at [server/services/feed/nudges.go](../../server/services/feed/nudges.go) (extended to accept a `surface` parameter). The "8 of 11 want this again" copy is baked into the resulting nudge's `description` at generation time — the client never reads `ESMSignal` directly. `esm_service.GetESMSignal` becomes a server-internal call from the nudge generation pipeline; #1580's host-only auth gate is satisfied by the server-internal call path, and the resulting `WORKSHOP_QUEST_HERO` nudge is host-scoped via the existing `community_id` + `user_id` filter on `StoredNudge`. The client-side `WorkshopRepository` is correspondingly thinner: one fetch (`GetWorkshopFeed`), one cache key (`'workshop:feed:<circle_id>'`), no per-experience ESM round-trips.

## Related Issues

- **#1580** — Lightweight ESM (post-event signal cards in the feed). Supplies the signal that drives Quest hero cascade priority (2). The read contract (`esm_service.GetESMSignal`) is consumed server-internally by this issue's nudge generation pipeline. **Phase 1 of #1580 is a hard prerequisite for this issue** — see Sequencing below.
- **Future issue** — Mike-said-yes recipient handoff into a fully-drafted Create event (Phase 1b in the brief). Out of scope here; should be filed as its own issue once this one lands and the catalyst-pull payload format is final.
- **Future issue** — Once-per-season hero card + Phase 2 state variants (questless, in-care, first-touch, brand-new). Filed after Phase 1a validation.
- **Future issue** — All-circles aggregation (Phase 3). Filed after a multi-circle host has used the per-circle Workshop for at least one full instrumentation cycle.

## Sequencing with #1580 (2026-04-29)

**Decision:** #1580 ships first, but only its Phase 1 (proto contract + `esm_service` + `server/esm/` shared library + storage models) is a hard prerequisite. Everything after #1580 Phase 1 can run in parallel with this issue.

**Reasoning:** Cascade priority (2) — the canonical "Schedule round 6" Quest hero, the most concrete lever in the brief — lights up only once ESM signal exists in the system. Sequencing #1580 Phase 1 first means the Workshop launches with that surface working from day one rather than dark.

**Staged plan across both issues:**

1. **Step 1 (~1 week):** #1580 Phase 1 — proto contract, ESM service, shared library, storage models. Unblocks all parallel work below.
2. **Step 2 (~2 weeks, parallel):** #1580 Phase 2 (manual authoring runbook) **and** this issue's Phases 1–2 (tab plumbing + `StoredNudge.surface` enum + `WorkshopTile` + nudge/story detail screen wraps). The Workshop's Phase 2 reuses the existing nudge pipeline; it depends only on #1580's contract being defined, not on its RPC being live.
3. **Step 3 (~1.5 weeks, parallel):** #1580 Phase 3 (ESM client card in feed) **and** this issue's Phase 3 (Crew strip + catalyst-pull review screen + surfacing discipline).
4. **Step 4 (~1.5 weeks, integration):** #1580 Phase 4 (forward-compat README) **and** this issue's Phase 4 (cascade detectors + `GetWorkshopFeed` RPC; the cascade-(2) detector calls `GetESMSignal` server-internally).

**Fallback if you'd rather demo the Workshop sooner:** flip Steps 1 and 2 — start the Workshop's Phase 1–2 in parallel with #1580 Phase 1, accept that cascade priority (2) is initially dark, and light it up later via a server-side detector + config flip. The reconciliation makes that fallback cheap because the cascade logic lives server-side in the nudge generation pipeline; no client-side re-work is needed.

---

## Audit Notes (2026-04-29)

The plan was reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), and [docs/server/architecture.md](../server/architecture.md). Changes made in place:

1. **Navigation pattern fixed.** The original plan said "push `PortfolioMetricsScreen` via GoRouter or `Navigator.push`." Both violate the navigation conventions — `MaterialPageRoute` flashes the previous screen and `context.push()` is reserved for auth flows. Updated Phase 1 and the related Open Question to specify `NavigationHelpers.pushWithSlide()` (utility-screen pattern). The repo's `npm run lint:nav` would have caught this in CI.
2. **Cache namespace and keys spelled out.** The original plan named `WorkshopRepository` but didn't specify the namespace or key shape. Per the cache key naming convention, added `CacheService(cacheManager, 'workshop')` and concrete key patterns (`'workshop:cascade:<circle_id>'`, etc.).
3. **Strict MVVM layering reinforced.** Added explicit notes that the viewmodel reads only from `workshopRepositoryProvider` (never directly from a service provider) and that repository depth stays shallow (max 1 level), with cross-data composition done at the viewmodel layer if needed.
4. **AsyncNotifier safety pattern called out.** Added a Phase 2 task requiring `if (!ref.mounted) return;` guards in non-`build()` async methods that mutate state, per the Async Safety section in the architecture doc.
5. **Local cache maps in viewmodels explicitly disallowed.** The original plan said the rate limit and decline cooldown "use local-only state in v1." Updated to clarify the state lives in the repository under a cache key, not as ad-hoc maps in the viewmodel — the cache rules forbid manual cache maps in viewmodels.
6. **i18n boundary clarified.** Added an Open Question recommending viewmodels expose typed enums (`LeverKind`, `LoadOverlayCase`) and widgets resolve all strings via `context.l10n.*`. The lever-copy guard test loads resolved English strings via `AppLocalizations` rather than asserting on hardcoded fixtures.
7. **Theme-aware colors required.** Added an Open Question and Phase 2 task requiring `AppColors.xxx(context)` everywhere; no hardcoded `Color(0xFF...)` or `Colors.grey[*]`. The JSX mock's parchment palette translates to semantic `AppColors` entries, with new entries added to `AppColors` if needed rather than inlined.
8. **Server-side library boundary added.** The original plan named a single `GetCatalystSuggestions` RPC but didn't specify where eligibility logic lives. Per the *Service Independence* and *Standalone Libraries for Shared Logic* principles, eligibility logic now lives in a new shared library at `server/catalyst/`; the RPC handler in `server/services/workshop/` is a thin wrapper. This avoids service-to-service coupling and keeps the logic unit-testable in isolation.
9. **Proto conventions reinforced.** Added explicit reminder in Phase 4 that the new RPC defines its own dedicated request/response types — never reuse messages across RPCs per CLAUDE.md and `docs/proto_conventions.md`.
10. **Disposal test required.** Added to Phase 2 tests — every async Notifier in the codebase must have a disposal test per the architecture doc.

No structural reorganization of phases was needed; the original sequence (shell → Quest hero + lever flow → Crew strip + revival + surfacing discipline → server contract & validation) holds up against the architecture rules.

## Decisions Log (2026-04-29, second pass)

User-supplied direction collapsed all open questions and added one structural change. Plan updated in place:

1. **Analytics naming** — keep `impact` for existing client/server analytics infrastructure (don't expose to user); use `workshop` for everything new. User-facing label is *Workshop*.
2. **PortfolioMetricsScreen entry point** — add a link from user settings now via `NavigationHelpers.pushWithSlide()`; revisit placement later.
3. **Server-side aggregation** — proceed with the recommended `GetCatalystSuggestions` RPC + `server/catalyst/` shared library shape.
4. **Quest hero cascade signal in v1** — proceed with the hand-curated fixture; #1580 fills the same JSON shape from real data.
5. **Circle picker scope** — pills mirror the sidebar's currently-selected communities (in sidebar order) with an *All* prepended; selecting a pill narrows the Workshop view only. The Workshop never mutates `selectedCommunityProvider`. Implemented as a derived `workshopCirclePillsProvider` + an autoDispose `workshopActivePillProvider`.
6. **Haptics** — do not implement. Removed from Phase 2.
7. **Voice-signal capture** — punted. Removed from Phase 2.
8. **i18n boundary** — viewmodels expose typed enums; widgets resolve via `context.l10n.*`.
9. **Theme** — reuse existing `AppColors` and font tokens; light + dark theme aware; the JSX mock's parchment palette is a layout-density reference, not a literal palette.
10. **Lever flow architecturally inverted (load-bearing change).** The brief's "shared bottom-sheet review pattern" is **not** built as a new modal. Lever taps generate a draft item server-side and route the host into the existing preview screens (e.g., [experience_preview_modal.dart](../../app/lib/presentation/screens/experience/experience_preview_modal.dart)) for edit-and-publish. No `workshop_review_sheet.dart` file is created. This reuses every affordance the existing create flows already provide (image picker, time picker, community selector, voice/edit handling) and prevents a parallel review surface. Phase 2 was rewritten to reflect this — including a server-side `GenerateWorkshopDraft` RPC that populates the same draft providers (`genExperienceProvider`, request/offer equivalents) the existing previews already consume.

## Consistency Pass with #1580 (2026-04-29)

Reviewed against [docs/issues/1580-esm-cards.md](1580-esm-cards.md) after that plan's second pass landed. Reconciled:

1. **Read API named explicitly.** Phase 4 and the Context section now reference `esm_service.GetESMSignal(experience_id)` by name (1580 created a dedicated `esm_service` rather than putting the RPC on `feed_service`). Workshop's previous "the read endpoint" phrasing was vague.
2. **Cascade priority (3) is not ESM-driven.** Earlier phrasing implied #1580 supplied signal for both priorities (2) and (3). It only supplies (2). Cascade (3) — *calendar gap near a recurring rhythm* — is derived from existing `Experience` completion history and gets a separate server-side helper (`server/catalyst/` or a new `server/momentum/` shared library, pick during implementation). Phase 4 now lists three production read seams instead of conflating two of them.
3. **Fixture shape mirrors `ESMSignal`.** The Open Question on cascade signal source now spells out the field-by-field correspondence: cascade (2) fixture entries match `ESMSignal{prompt_id, respondent_count, response_distribution: map<option_key, count>, free_text_count, completed_at_unix_sec}`. The swap from fixture to real RPC is one method body, not a contract change.
4. **`option_key` "repeat" convention pinned down.** Added an Open Question + decision: the `option_key` `do_again` is the agreed-upon key in `ESMSignal.response_distribution` that represents "yes, repeat this." Documented in both the WorkshopRepository fixture README (Phase 4 task) and in #1580's [docs/runbooks/esm-manual-authoring.md](../runbooks/esm-manual-authoring.md) so prompt authors and signal consumers agree.
5. **Authorization boundary for `GetESMSignal` enforced.** #1580 specifies `requireESMSignalReader` — only the experience host (or community owner) can read aggregate signal. Phase 4 now states the WorkshopRepository only calls `GetESMSignal` when the viewing user is the experience host; for non-host views, cascade priority (2) is silently skipped (which is the right behavior anyway, since that priority is about a host scheduling the next round of *their own* recurring thing).
6. **Service Independence between Workshop and ESM services confirmed.** Both `server/services/workshop/` and `server/services/esm/` exist as independent services; neither depends on the other. The Workshop's `GetCatalystSuggestions` consumes `server/catalyst/`; the ESM service's `GetESMSignal` consumes `server/esm/`. The Workshop client repository may call both RPCs to assemble its cascade — that's fine because *repositories can call multiple services*; the per-server prohibition is on service-to-service injection, not client-side composition.
7. **No structural conflicts found.** Cache namespace (`'workshop'`) does not collide with #1580's (`'esm'`). The l10n boundary, theme rules, and async-safety guidance are consistent across both plans. The "no admin tool / no operator CLI" decision in #1580 does not affect the Workshop, since the Workshop never authored ESM prompts in the first place.

## Reconciliation with Nudges and Stories (2026-04-29, third pass)

User direction: Workshop suggestions are another form of nudges; the once-per-season hero is a story. Don't duplicate data models or logic. Tapping a Workshop tile should slide the existing nudge/story detail view in from the right.

Plan updated end-to-end. Major changes:

1. **Workshop suggestions are `StoredNudge` rows with a new `surface` enum** — values `WORKSHOP_QUEST_HERO`, `WORKSHOP_BRING_BACK`, `WORKSHOP_SEED_CATALYST`. New `cta_action` values: `schedule_repeat`, `revive_experience`, `propose_share`, `seed_subhost`. Generation, persistence, consumption tracking, soft-delete, and async stock imagery all reuse the existing nudge pipeline at [server/services/feed/nudges.go](../../server/services/feed/nudges.go) — the cascade priority logic becomes `surface`-aware code in that pipeline, not a parallel system.
2. **Once-per-season hero is a `Story` with category `season_summary`.** No new data model.
3. **Tap-to-detail uses `NavigationHelpers.pushScreen()`** — Workshop tiles route to a new `NudgeDetailScreen` (a Scaffold + `SwipeToCloseMixin` wrapper around the existing `NudgeContentView`) for nudge surfaces, and `StoryDetailScreen` for the season hero. The slide-from-right transition ties the Workshop visually to the rest of the app's navigation grammar.
4. **Catalyst-pull stays separate** — recipient eligibility, rate limits, and host-private visibility don't fit the nudge model. The Crew strip's catalyst-pull tile routes to a Workshop-specific `CatalystPullReviewScreen` (the only Workshop-owned detail screen). The seed-the-catalyst surface, by contrast, *does* fit the nudge model and becomes a `WORKSHOP_SEED_CATALYST` nudge.
5. **Phase 2 collapses substantially.** No `quest_hero_card.dart` rendering all five cascade variants. No `GenerateWorkshopDraft` RPC. No new bottom sheet. The Workshop becomes a curation layer: `WorkshopTile` over `NudgePayload` (or `StoryPayload`), tap routes to existing detail view, CTA inside dispatches via the existing nudge action mechanism.
6. **Phase 3's Bring Something Back card disappears as a custom widget.** Items render as `WorkshopTile`s over `WORKSHOP_BRING_BACK` nudges; the *flat* vs. *tiered* layout test happens at the tile-collection level, not by introducing a new widget.
7. **Phase 4's three read seams collapse to one.** Client reads `GetWorkshopFeed` returning curated `NudgePayload` and `StoryPayload` objects. Cascade priority logic (including the ESM-driven priority 2 and calendar-gap priority 3) runs server-side in the nudge generation pipeline; the client never reads `ESMSignal` directly. `esm_service.GetESMSignal` is consumed server-internally by the nudge pipeline; the host-only auth gate is satisfied by the server-internal call path.
8. **Banned-words guard extended.** The guard now also checks persisted `StoredNudge.headline/description/cta_label` for any Workshop surface, not just l10n strings — because the prose now lives in AI-generated nudge fields, not exclusively in l10n.
9. **Reduced scope, increased coherence.** The reconciliation eliminates a parallel AI generation pipeline, parallel consumption tracking, parallel soft-delete, and parallel rate-limiting logic that would otherwise have drifted out of sync with the feed surface. The cost paid up front — generalizing `StoredNudge` with a `surface` field, teaching the nudge pipeline about Workshop surfaces, wrapping `NudgeContentView` in a pushable screen — is small relative to the cost avoided.
