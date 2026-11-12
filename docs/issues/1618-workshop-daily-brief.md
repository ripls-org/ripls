# Issue #1618: Workshop UX: replace Hero card stack with LLM-driven Daily Brief + secondary CTA rows (workshop-brief.jsx)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1618
**Filed:** 2026-04-30 | **Last activity:** 2026-04-30
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** none provided

## Problem Statement

The current Workshop tab renders a vertical stack of Hero card tiles (one per recently-completed event from the per-event detector + drift cards from calendar-gap / seasonal / idle-offer detectors), plus a Crew avatar strip, weekly-need card, all-circles tension summary, recap-Story tile, and a three-panel "Together this season" synthesis. Each surface is a separate widget the host has to scan visually.

The reference mock at [docs/cowork/mocks/workshop-brief.jsx](../cowork/mocks/workshop-brief.jsx) collapses all of that into one **AI-generated narrative paragraph** with 2-3 underlined inline levers (e.g., *"want to bump it to 7pm? … or weekly through July? … or drop her an @ in the smoker chat"*), followed by compact secondary CTA rows for the lower-priority items. The brief ends with a single italic line summarizing season totals and a `See the season →` link to the metrics deep view.

The single load-bearing change is the brief itself. Templates can stitch one Hero card's copy at a time (the existing `momentum.HeroCardCopyProvider` already handles that), but they can't synthesize *heterogeneous priorities* — an ESM verdict + a load callout + a lapsed-regular re-invite — into a single coherent paragraph the way a host would write it themselves. That makes the brief the first surface where AI generation is **structurally required**, not just nice-to-have.

The detection chain (`server/momentum/`, `server/needs/`, `server/catalyst/`, `server/impact_metrics/`) does not need to change — the brief is a curation/render-layer rework that reads the same `StoredNudge` rows the existing per-event Hero card pipeline persists.

## Context from Comments

The issue has no comments yet — it was filed in the same session that produced the per-event Hero card model and the `HeroCardCopyProvider` AI seam. The body is detailed and self-consistent; the gaps below come from codebase research, not unanswered comment threads.

## Potential Solutions

### Option A: New `GetWorkshopBrief` RPC + new `BriefProvider` AI seam, render the brief above the existing per-event nudge data

- **Approach:** Keep the per-event Hero card pipeline, the catalyst load detector, the weekly-need detector, and the recap detector exactly as they are. They continue to persist `StoredNudge` rows + emit `CrewStripPayload` / `WeeklyNeedPayload` / `recap_story` on `GetWorkshopFeed`. Add a parallel `GetWorkshopBrief(circle_ids[]) → BriefPayload` RPC that:
  1. Loads the same signal set (recent Hero card rows, catalyst suggestion, weekly need, recap availability).
  2. Picks the top 2-3 priorities with a small ranking function in `server/services/workshop/brief_ranker.go`.
  3. Calls a new `momentum.BriefProvider` (bridged to `ai.Provider.GenerateWorkshopBrief`) for the prose and lever spans.
  4. Returns `BriefPayload{kicker_label, segments[], closing_line, metrics_link_label}` where `BriefSegment` is a oneof of `text_span` / `lever_span`.

  The client renders the brief at the top of the Workshop tab; the existing `WorkshopTile`, `CrewStrip`, `WeeklyNeedCard`, `AllCirclesSummary`, `TogetherThisSeason` widgets stop being mounted on the screen but stay in the codebase. The compact CTA rows below the brief reuse the same `StoredNudge` payload shape — `CTARow` is a new render of an old payload, not a new payload.

- **Pros:** Smallest blast radius. The detection chain is unchanged; the AI seam is symmetrical with the existing `HeroCardCopyProvider`; the existing `GetWorkshopFeed` RPC keeps working for any stale clients that aren't on the new build yet. The brief and the underlying nudges share data so the deep-view metrics stay consistent with the inline closing line.
- **Cons:** Two read RPCs the client calls in parallel from `WorkshopNotifier.build()` (already does this for `GetWorkshopFeed` + `GetWorkshopSynthesis`). The brief reads signal that has *already* been transformed once into Hero card copy by `CopyGenerator` — the AI is being asked to rewrite content it indirectly produced. Defensible (different output shape: synthesis vs. per-card) but worth noting.
- **Effort:** Large (proto + AI provider method on every implementation + brief assembler + ranker + client widget + bottom-sheet routing), but each step is contained.

### Option B: Replace `GetWorkshopFeed` with `GetWorkshopBrief`; the brief is the only Workshop response

- **Approach:** Delete the per-event Hero card iteration on the wire. `GetWorkshopFeed` becomes `GetWorkshopBrief` (or a new RPC retiring the old one). The brief carries everything: the prose with inline levers, the secondary CTA rows as a `repeated BriefCTARow` field on the response, the metrics deep view payload, the crew member avatars (if we keep them) — all in one fetch.

- **Pros:** One RPC, one render path, no parallel surfaces. The wire shape literally matches the screen. Simpler client viewmodel: no `hasHeroCards` / `hasBringBack` / `hasRecapStory` selectors — just `hasBrief`.
- **Cons:** Throws away wire compatibility for any client mid-rollout. The Hero card list shape, the catalyst-pull review screen wire contract, and `GetWorkshopSynthesis` either move into the brief response or get removed. The detection-chain side stays the same but every consumer gets touched. Not actually simpler from an *implementation* standpoint — just relocates the complexity.
- **Effort:** Large + risky. Locks Phase 1 client+server changes to coordinated rollout.

### Option C: Layer the brief on top of the existing surfaces — add it as a new tile that floats above a still-rendered Hero card stack

- **Approach:** Render the AI brief as one panel at the top, then keep all existing widgets (Hero cards, Crew strip, weekly need, synthesis) mounted below it. The brief becomes an additional surface, not a replacement.

- **Pros:** No demolition. Stale clients still see something familiar; new clients see the brief on top.
- **Cons:** The mock's whole point is *one* surface, not two. Stacking the brief above the Hero card list duplicates information ("bump round 6 to Saturday at 7pm" appearing in the brief prose AND as a Hero card tile); the screen reads as cluttered, not curated. The brief's premise — a host opens the tab and reads one paragraph — collapses.
- **Effort:** Smallest, but it ships the wrong product.

## Recommendation

**Option A.** The brief is a curation+render rework, not a data-model rework. Keeping the per-event Hero card pipeline and the existing detectors intact preserves the structural decision the user already made ("one Hero card per recently-completed event") — the brief just chooses to render that signal as a paragraph instead of a stack. The two-RPC parallel-fetch shape is already in place for `GetWorkshopFeed` + `GetWorkshopSynthesis`, so the brief slots in symmetrically.

The biggest tradeoff: Option A leaves four widgets in the codebase that the Workshop tab no longer mounts (`WorkshopTile`, `CrewStrip`, `WeeklyNeedCard`, `AllCirclesSummary`, `TogetherThisSeason`). The issue body explicitly punts deletion to a follow-up to keep this diff focused, which is the right call — those widgets have golden tests and disposal tests; ripping them out alongside the brief work risks ratchet failures on unrelated paths.

The other notable risk is AI cost: Workshop opens currently have **no analytics or rate-limiting** ([app/lib/presentation/screens/workshop/workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart) has no logger / analytics calls). The brief is per-host, per-tab-open; if a host bounces between circle pills the brief regenerates per pill. Phase 1 needs a server-side cache + the existing `WorkshopRepository` cache namespace `'workshop'` for the brief result — same pattern `getSynthesis` already uses with key `'synthesis:<sorted_ids>'`.

## Resolved Decisions (2026-04-30)

User direction: accept all recommendations from the original Open Questions. The
plan now codifies each decision so reviewers don't have to re-litigate.

1. **Unary brief in Phase 1; streaming follow-up.** The brief returns as a
   single `BriefPayload` per fetch. Streaming complicates `ValidateLeverCopy` /
   `AssertNoBannedWords` partial-segment validation and `Text.rich` mid-build
   repaint cost. A streaming method (`GenerateWorkshopBriefStreaming`) ships
   in a separate follow-up issue once the unary path is in production and
   first-paint latency data exists.

2. **Brief lives in its own cache namespace `'workshop_brief'`** with a 15-min
   TTL. Keeps the global `CACHE_TTL_MINUTES` default untouched. Does not require
   a `CacheService.get(... ttl: Duration?)` extension. Document the namespace
   choice at the top of `workshop_repository.dart`.

3. **On-demand brief generation in Phase 1; pre-warming pass added in Phase 3.**
   `GetWorkshopBrief` calls the AI provider on-demand for first-fetch latency.
   Phase 3 extends `workshop_generation` to call the same brief assembler and
   persist the result so subsequent opens return cached. Avoids spending AI
   tokens on hosts who don't open the tab.

4. **Inline lever rendering: `TextSpan` + `TapGestureRecognizer`** matching
   [linkified_text.dart](../../app/lib/presentation/widgets/content/linkified_text.dart).
   `WidgetSpan` reflows poorly inside `Text.rich` for italic / weight changes
   and would force chip-sized lever surfaces. Lever spans are styled prose, not
   payload widgets.

5. **`cta_action` vocabulary: reuse existing four + three additions.** The
   nudge dispatcher in
   [nudge_content_view.dart:113-148](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart#L113-L148)
   already routes `schedule_repeat`, `revive_experience`, `propose_share`,
   `seed_subhost`. Phase 1 adds `adjust_event`, `post_in_chat`, `ask_for_help`
   to cover the brief's gap. Mapping documented in [docs/workshop.md](../workshop.md).

6. **Time-of-day kicker is client-side.** Device clock is the source of truth
   for the host's local "morning"; server-side derivation forces a UTC
   round-trip the client already owns. The brief panel's own
   `BriefPayload.kicker_label` (e.g., "Good Thursday morning") is server-authored
   and is **distinct** from the floating-header time-of-day kicker.

7. **`WorkshopGenerationJob` stays.** Per-event Hero card persistence is the
   input to the brief; the brief reads those `StoredNudge` rows for its signal
   aggregation. Without the job, fresh-app hosts pay generation latency twice
   on first open.

8. **Eval suite at `server/ai/eval/workshop_brief.go` + `testdata/workshop_brief_goldens.json`**
   lands in Phase 3 with 6-10 hand-crafted cases. `//go:build benchmark` so it
   doesn't run in CI.

9. **`ExperienceStoriesJob` wiring is out of scope.** The brief's "publish the
   round-5 recap now?" lever degrades gracefully — when no
   `STORY_TYPE_EXPERIENCE_CONCLUDED` row exists, the prose simply omits the
   recap lever. File a sibling issue to wire the existing job into
   `server/main.go` startup; this issue's plan does not block on it.

10. **Provider-nil fallback is a templated bullet brief.** When `aiProvider` is
    `nil` (dev / OSS contributor without an API key), the handler returns a
    degraded-but-functional brief: a small ordered list of talking-point
    sentences (one per top priority) joined by separators, with the levers
    attached as separate `BriefCTARow` entries instead of inline spans. Reads
    as a bulleted briefing. Documented at the top of `server/services/workshop/brief.go`.

## Implementation Plan

### Phase 1: Proto contract, server brief assembler, and AI seam

**Goal:** `GetWorkshopBrief` returns a structured paragraph for any host with signal; AI fallback to a templated bullet brief works when no provider is wired.

**Tasks:**
- [ ] Define `BriefPayload`, `BriefSegment` (oneof `text_span` / `lever_span`), `BriefLever`, and `BriefCTARow` in [proto/ripls/api/workshop_service.proto](../../proto/ripls/api/workshop_service.proto). Mirror the `FeedItem.payload` oneof convention from [feed_service.proto](../../proto/ripls/api/feed_service.proto). Include `closing_line` and `metrics_link_label` on the response.
- [ ] Add `cta_action` enum values (or string conventions) covering `schedule_repeat`, `revive_experience`, `propose_share`, `seed_subhost`, `adjust_event`, `post_in_chat`, `ask_for_help`.
- [ ] Add `GetWorkshopBrief(GetWorkshopBriefRequest) returns (GetWorkshopBriefResponse)` RPC on `WorkshopService`.
- [ ] Add `momentum.BriefProvider` interface + `momentum.BriefSignal` (multi-priority shape) and `momentum.Brief` (segments + closing) types in [server/momentum/](../../server/momentum/) — sibling to the existing `HeroCardCopyProvider`.
- [ ] Add `ai.Provider.GenerateWorkshopBrief(ctx, signal) (*Brief, error)` to the Provider interface, with `ai.BriefSignal` / `ai.Brief` data types in [server/ai/brief.go](../../server/ai/brief.go) and `ErrWorkshopBriefNotImplemented` for non-Anthropic providers.
- [ ] Implement `GenerateWorkshopBrief` on `AnthropicProvider` via the existing `anthropicCallStructured[T]` helper. Prompt builder lives in [server/ai/prompts_workshop_brief.go](../../server/ai/prompts_workshop_brief.go) — separate file because [prompts.go](../../server/ai/prompts.go) is past the 1000-line cap.
- [ ] Stub `GenerateWorkshopBrief` on Gemini / OpenAI / Mock returning `ErrWorkshopBriefNotImplemented`; delegate on Fallback / LoadBalanced.
- [ ] Bridge `ai.Provider` → `momentum.BriefProvider` in [server/services/workshop/brief_provider.go](../../server/services/workshop/brief_provider.go) (sibling to the existing [copy_provider.go](../../server/services/workshop/copy_provider.go)).
- [ ] Workshop service handler at `server/services/workshop/brief.go`:
  - Call `auth.RequireAuth(ctx)` first — every Workshop RPC takes the standard auth path per [docs/server/architecture.md](../server/architecture.md).
  - Aggregate the signal set (recent Hero card rows + catalyst suggestion + weekly need + recap availability + impact-metrics totals) into a `momentum.BriefSignal`. Use **batched** storage reads — `storage.QueryByFields` / `QueryByField` over the (user_id, community_id) join, not per-experience round-trips. Per the SQL Efficiency rules in [CLAUDE.md](../../CLAUDE.md), every `for` loop containing a storage call is an N+1 bug. Cover the handler with `storage.AssertMaxQueries` in tests.
  - Rank top 2-3 priorities (`server/services/workshop/brief_ranker.go`).
  - Call the provider; on failure return the templated bullet-brief fallback (Open Question #10).
  - Run `ValidateLeverCopy` on every lever's `visible_text` and `AssertNoBannedWords` on every AI-generated text field — `kicker_label`, every `text_span`, every lever's `visible_text`, and `closing_line`. Drop AI levers that fail; keep a minimum of 1 templated lever to guarantee the brief is non-empty.
- [ ] **Proto field conventions.** Every conditionally-present field on `BriefPayload`, `BriefSegment`, `BriefLever`, `BriefCTARow`, and `StoredWorkshopBrief` uses the `optional` keyword per [CLAUDE.md](../../CLAUDE.md). Keeps `Has*()` presence checks explicit on the Dart side.
- [ ] Structured logging on the handler via `logging.LoggerWithContext(ctx)` with the standard fields from [docs/server/observability.md](../server/observability.md): `operation = "GetWorkshopBrief"`, `user_id`, `community_count`, `lever_count`, `duration_ms`, plus `external_service`, `provider`, `model` on the AI call. AI fallback to templates logs at `Warn`; system failures at `Error`; user-input failures at `Info`/`Warn`.
- [ ] Add `WorkshopBriefGoldens` shape + a small benchmark eval in [server/ai/eval/workshop_brief.go](../../server/ai/eval/workshop_brief.go) (build-tag gated; skeleton only — full case authoring in Phase 3).

**Server-side tests:**
- Unit tests for `BriefRanker` (deterministic top-3 across signal mixes — quiet, single-event, multi-priority, all-circles-scope).
- Unit tests for `brief.go` handler covering: auth required (`Unauthenticated` when no caller), provider success path, provider failure → templated fallback path, lever copy validation drops a non-conforming lever, banned-words guard drops a tainted segment.
- N+1 regression: wrap the handler in `storage.AssertMaxQueries(t, ctx, max, fn)` per [CLAUDE.md](../../CLAUDE.md) so a future detector signal-aggregation change doesn't reintroduce per-experience round-trips.

**Validates:** the AI seam ships end to end; the data layer is concrete enough that the client widget can plug in without further server changes; the templated fallback works on a dev box without an API key.

### Phase 2: Client brief widget, secondary CTA rows, time-of-day kicker

**Goal:** the Workshop tab matches the JSX mock — one `DailyBrief` panel above a list of `CTARow`s, with the floating header time-of-day kicker.

**Tasks:**
- [ ] Add `getWorkshopBrief(communityIds)` to [WorkshopService](../../app/lib/services/workshop_service.dart) and [WorkshopRepository](../../app/lib/data/repositories/workshop_repository.dart). Cache key `'brief:<sorted_ids>'` under namespace `'workshop'` per [docs/client/caching.md](../client/caching.md). The brief needs a **tighter TTL than the global 30-min `CACHE_TTL_MINUTES` default** (see Open Question #2). Two compatible options, in order of preference: (a) write the brief result to a separate cache namespace (`'workshop_brief'`) configured with a 15-min TTL — keeps the existing global default untouched; (b) extend `CacheService.get` with an optional per-call `ttl: Duration?` override. Pick (a) unless a wider audit reveals other surfaces want the same override (then (b)). Document the chosen pattern at the top of `workshop_repository.dart`.
- [ ] Extend `WorkshopState` with `brief: BriefPayload?` and `hasBrief` selector. Update `WorkshopNotifier.build()` to fetch feed + synthesis + brief in parallel via `Future.wait` with the existing `ref.mounted` guard.
- [ ] New widget [app/lib/presentation/widgets/workshop/daily_brief.dart](../../app/lib/presentation/widgets/workshop/daily_brief.dart):
  - **`StatefulWidget`** so it owns the lifecycle of its `TapGestureRecognizer` instances. The widget builds a fresh list of recognizers in `_rebuildSpans()` (called from `initState` + when the payload changes) and calls `recognizer.dispose()` for every entry in `dispose()`. Failing to dispose recognizers leaks closures over the WidgetRef — the disposal test (below) is the regression guard.
  - Parchment panel matching the JSX layout (warm-orange uppercase kicker, serif prose, dashed-rule separator, italic closing line).
  - Renders prose via `Text.rich` with one `TextSpan` per `BriefSegment.text_span` and one `TextSpan` + `TapGestureRecognizer` per `BriefSegment.lever_span` — pattern from [linkified_text.dart](../../app/lib/presentation/widgets/content/linkified_text.dart). Lever spans use the lever foreground color from `AppColors` (no hardcoded hex) per [docs/client/architecture.md](../client/architecture.md).
  - The brief's **own** kicker (`BriefPayload.kicker_label`, e.g., "Good Thursday morning") is rendered inside the panel and is server-authored. Distinct from the **floating header time-of-day kicker** (e.g., "Thursday morning" right-aligned next to the title) which is client-side l10n — see the time-of-day formatter task below. Both surfaces; do not conflate them.
  - Lever taps route through a new `WorkshopLeverDispatcher` helper that maps `cta_action` → existing flow. Any handler that performs an `await` followed by a `Navigator.push` / `showAccessibleModal` MUST guard `if (!context.mounted) return;` between the await and the navigation per the Async Safety section in [docs/client/architecture.md](../client/architecture.md). Mappings:
    - `schedule_repeat` / `revive_experience` → `_openWorkshopDraft` (already in [nudge_content_view.dart](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart) — already mounted-guards correctly)
    - `propose_share` → existing gear-context flow
    - `seed_subhost` → existing catalyst-pull review screen
    - `adjust_event` → `ExperiencePreviewModal` with the prior instance's time pre-edited
    - `post_in_chat` → opens the conversation by `context_id`, prefilled draft (existing chat compose). Navigation goes through `NavigationHelpers.pushScreen` / `pushToItemScreen` per [docs/client/architecture.md](../client/architecture.md) — never raw `Navigator.push(MaterialPageRoute(...))`. `npm run lint:nav` enforces this.
    - `esm_author` → [EsmAuthorSheet.show](../../app/lib/presentation/widgets/workshop/esm_author_sheet.dart) (already exists; uses `showAccessibleModal`)
    - `ask_for_help` → [RequestCreationModal](../../app/lib/presentation/screens/request/request_creation_modal.dart) (already uses `showAccessibleModal`)
  - Closing-line `See the season →` link pushes a new `MetricsSheet` (next bullet). Use `Tappable` + l10n-resolved `semanticsLabel` (key `a11yWorkshopOpenSeasonMetrics`) per [docs/client/accessibility.md](../client/accessibility.md).
- [ ] New widget [app/lib/presentation/widgets/workshop/cta_row.dart](../../app/lib/presentation/widgets/workshop/cta_row.dart) — compact icon-prefix row (kicker / circle chip, headline, atmosphere line, action-label tag). One row per `BriefCTARow`. Tap dispatches through the same `WorkshopLeverDispatcher`. `Tappable` with `semanticsLabel = headline + action_label`.
- [ ] New widget [app/lib/presentation/widgets/workshop/metrics_sheet.dart](../../app/lib/presentation/widgets/workshop/metrics_sheet.dart) — rendered via `showAccessibleModal`. Three sections (Quality Time, Replaced Cost, CO₂ Avoided) reading **server-authored labels from `GetWorkshopSynthesisResponse.panels`** (each panel already carries a `label`, `lede`, and `body`); per-circle breakdown rows render under each panel. Do not invent client-side l10n keys for the section labels — that would let server and client copy drift. Sheet chrome (close button, modal header) uses `ModalHeader` + `IconAction` per [docs/client/accessibility.md](../client/accessibility.md).
- [ ] Time-of-day formatter at `app/lib/core/utils/time_of_day.dart`. **i18n boundary** per [docs/client/i18n.md](../client/i18n.md): the helper returns a typed `TimeOfDayPeriod` enum (`morning` / `afternoon` / `evening`) from the device clock; the widget resolves the enum to a string via `context.l10n.workshopHeaderTimeOfDay{Morning,Afternoon,Evening}` in `build()`. ViewModels never carry strings. New l10n keys + a11y label `a11yWorkshopOpenSeasonMetrics` land in **both** [app_en.arb](../../app/lib/l10n/app_en.arb) and [app_es.arb](../../app/lib/l10n/app_es.arb) before running `npm run generate` — `npm run lint:dart:i18n` will fail otherwise.
- [ ] Update [WorkshopScreen](../../app/lib/presentation/screens/workshop/workshop_screen.dart):
  - Replace the body with: `DailyBrief` (when `state.hasBrief`), then `CTARow` list, then no other surfaces.
  - Floating header gains the trailing time-of-day kicker (right-aligned next to the title).
  - Stop mounting `CrewStrip`, `WorkshopTile`, `WeeklyNeedCard`, `AllCirclesSummary`, `TogetherThisSeason` on this screen. Leave the widget files in place per the issue body's deferral.
- [ ] Tests per [docs/client/testing.md](../client/testing.md):
  - ViewModel test: `WorkshopNotifier` exposes brief via `ProviderContainer` with overrides for `workshopRepositoryProvider`. Mockito mocks of the service client (`@GenerateMocks([WorkshopService])`); reset mocks in `tearDown`.
  - **Disposal test for `WorkshopNotifier`** — required for every async `Notifier` in the codebase per the architecture doc. Asserts `container.dispose()` mid-fetch does not throw and does not leak the in-flight Future.
  - Widget test: `DailyBrief` renders kicker + prose + closing line; tap on a lever span fires the dispatcher; the widget's `dispose()` is called and every `TapGestureRecognizer` is freed (use `WidgetsBinding.instance` to drive a rebuild, then verify recognizer count via test exposure or count `Diagnosticable` references).
  - Widget test: `CTARow` tap dispatches to the matching action handler — assert `Tappable` semantics include both the headline and the action label.
  - Widget test: `MetricsSheet` opens via `showAccessibleModal` and renders panels from a fixture `GetWorkshopSynthesisResponse`.
  - Existing workshop tests update their assertions to look for the brief instead of Hero card tiles.

**Validates:** end-to-end smoke — host opens the Workshop tab, sees one paragraph with 2-3 underlined levers, taps one, lands in the matching review modal, sends. The mock's promise.

### Phase 3: Eval suite, prompt iteration, scheduled brief pre-warming

**Goal:** the brief reads like the JSX mock copy on representative cases; cold-open hosts pay zero AI latency.

**Tasks:**
- [ ] Author 8-12 golden cases in [server/ai/eval/testdata/workshop_brief_goldens.json](../../server/ai/eval/testdata/workshop_brief_goldens.json):
  - Single recently-completed event (template-easy)
  - Multi-event with one ESM verdict and one calendar-gap (the brief's natural shape)
  - Lapsed-regular re-invite (`post_in_chat` + `@-mention`) — covers the brief's "Linda hasn't been around in five weeks" line
  - Load callout (host-as-Sarah variant)
  - Recap pending (assumes the recap-Story job is wired — see Open Question #9)
  - Quiet morning (no signal — should return an empty brief, not a paragraph that says "nothing to do")
  - Per-circle scope vs. all-circles scope
  - Banned-word adversarial case (the AI must avoid "quiet", "decay", "rotation", etc.)
- [ ] Iterate on the prompt in [prompts_workshop_brief.go](../../server/ai/prompts_workshop_brief.go) until the eval suite passes ≥80% per-field.
- [ ] Add a brief pre-warming pass to [server/jobs/workshop_generation.go](../../server/jobs/workshop_generation.go): for each (host, community) pair the job already walks, call the same shared-library helpers `server/services/workshop/brief.go` uses (Service Independence — the job does NOT inject the workshop service; it consumes shared libraries directly per Pattern 1 of [docs/server/architecture.md](../server/architecture.md)). Write the result to a small persisted cache (`StoredWorkshopBrief` — new proto model in [proto/ripls/models/](../../proto/ripls/models/) with soft-delete metadata, every conditionally-present field marked `optional`). The handler reads the cache when present, regenerates when stale (>15 min) or absent. Decision 8 from [docs/ai/workshop.md](../ai/workshop.md) holds: **no notifications**. Any goroutines fanned out per-pair use `logging.GoSafe(ctx, name, fn)` per the goroutine-safety rule — the CI gate at `server/cmd/check-goroutines` rejects bare `go ...` without recover-defers.
- [ ] `BriefRanker` regression test: given a fixture `BriefSignal` with all signal types populated, the ranker emits a deterministic top-3 — pinned in a unit test so prompt iteration doesn't drift the priority order.
- [ ] Documentation update — extend [docs/ai/workshop.md](../ai/workshop.md) with a Phase 4 section describing the brief, the cta_action vocabulary, and the lever-dispatcher mapping.

**Validates:** prompt quality is reproducible; cold-open performance matches Phase 2; the eval suite catches future prompt regressions; the brief is durably cached.

### Phase 4: Cleanup — remove the surfaces the brief supersedes

**Goal:** the Workshop tab is **only** the brief direction — floating header → `DailyBrief` → `CTARow` list → `MetricsSheet` (deep view). No orphan widgets, dead l10n keys, or unused proto fields. The user explicitly chose to skip the rollback-safety-net release window — Phase 4 ships in the same diff series as Phases 1-3.

**Deletion is wire-incompatible with stale clients.** Phase 4's proto removals retire the `GetWorkshopFeed` RPC and several payload messages. Any client running a pre-brief build will fail to deserialize. The ripls release process serves the matched (server, client) pair through the App Store rollout; coordinate the merge so the brief client ships at the same release as the brief server. Pin a release version in the issue checklist.

**Tasks:**
- [ ] **Delete client widgets** that the brief subsumes. The brief renders prose + CTA rows; nothing else should mount on the Workshop screen.
  - [app/lib/presentation/widgets/workshop/workshop_tile.dart](../../app/lib/presentation/widgets/workshop/workshop_tile.dart) + matching test under `app/test/presentation/widgets/workshop/`
  - [app/lib/presentation/widgets/workshop/crew_strip.dart](../../app/lib/presentation/widgets/workshop/crew_strip.dart) + test
  - [app/lib/presentation/widgets/workshop/weekly_need_card.dart](../../app/lib/presentation/widgets/workshop/weekly_need_card.dart)
  - [app/lib/presentation/widgets/workshop/all_circles_summary.dart](../../app/lib/presentation/widgets/workshop/all_circles_summary.dart)
  - [app/lib/presentation/widgets/workshop/together_this_season.dart](../../app/lib/presentation/widgets/workshop/together_this_season.dart)
  - **Keep:** [esm_author_sheet.dart](../../app/lib/presentation/widgets/workshop/esm_author_sheet.dart) (still used by the `esm_author` lever).
  - **Keep:** [catalyst_pull_review_screen.dart](../../app/lib/presentation/screens/workshop/catalyst_pull_review_screen.dart) (still used by the `seed_subhost` lever).

- [ ] **Simplify `WorkshopState` and `WorkshopNotifier`** ([workshop_view_model.dart](../../app/lib/presentation/viewmodels/workshop_view_model.dart)):
  - State carries only `brief: BriefPayload?` and `synthesis: GetWorkshopSynthesisResponse?`. The synthesis stays because the `MetricsSheet` deep view reads from it.
  - Drop `feed`, `hasHeroCards`, `hasBringBack`, `hasSeedCatalyst`, `hasSeasonHero`, `hasWeeklyNeed`, `hasCirclesSummary`, `hasCrewStripOverlay`, `hasCrewMembers`, `hasRecapStory` selectors.
  - `WorkshopNotifier.build()` fetches brief + synthesis only (drop the `getFeed` call).
  - Regenerate the freezed file (`flutter pub run build_runner build --delete-conflicting-outputs`).

- [ ] **Trim `WorkshopRepository`** ([workshop_repository.dart](../../app/lib/data/repositories/workshop_repository.dart)):
  - Delete `getFeed(circleId, communityIds)` and the `'feed:<circle_id>'` cache key.
  - Delete `invalidateFeed(...)`.
  - Keep `getSynthesis(...)`, `getBrief(...)` (added in Phase 2), and `generateDraft(...)` (still used by `_openWorkshopDraft`).

- [ ] **Drop the legacy `GetWorkshopFeed` RPC and its payloads from
      [proto/ripls/api/workshop_service.proto](../../proto/ripls/api/workshop_service.proto):**
  - Remove `GetWorkshopFeedRequest`, `GetWorkshopFeedResponse`, `CrewStripPayload`, `CrewMember`, `WeeklyNeedPayload`, `CircleTension`.
  - Remove the `GetWorkshopFeed` rpc entry from the `WorkshopService` service definition.
  - **Keep** `NudgePayload` (still used by the feed surface), `StoryPayload` (still used elsewhere), `GenerateWorkshopDraftRequest/Response`, `GetWorkshopBriefRequest/Response`, `GetWorkshopSynthesisRequest/Response`.
  - Run `npm run generate`.

- [ ] **Server-side handler + helper deletion** in [server/services/workshop/](../../server/services/workshop/):
  - Delete `service.go`'s `GetWorkshopFeed` handler.
  - Delete `loadWorkshopNudges`, `isWorkshopNudgeRenderable`, `sortByCreatedDesc`, `nudgeToPayload`, `lookupUserName`, `loadCrewMembers`, `buildCrewStrip` (no longer reachable — the brief signal aggregator reads `StoredNudge` and `CommunityUser` directly from storage, not via wire payloads).
  - Delete `weekly_need.go`, `circles_summary.go`, `recap.go` — the brief reads the underlying signal (`server/needs.DetectThisWeekNeed`, `server/catalyst.GetSuggestion`, `Story` storage) directly. Their server-to-payload converters were wire-only adapters that the brief doesn't need.
  - **Keep** `brief.go`, `brief_ranker.go`, `brief_provider.go`, `draft_generation.go`, `synthesis.go`, `generation.go`, `copy_provider.go`. These either implement the brief, feed it inputs, or serve adjacent endpoints that survive.
  - Delete tests covering the removed handlers (`service_test.go` cases for `GetWorkshopFeed`, etc.). Tests covering `EnsureHeroCard` / `EnsureBringBackItems` stay because the brief consumes their `StoredNudge` output.

- [ ] **`StoredNudge` schema and detector chain stay.** The cascade detectors (`server/momentum/`) keep emitting `WORKSHOP_HERO_CARD` and `WORKSHOP_BRING_BACK` nudges; the brief reads those rows. The `NUDGE_SURFACE_WORKSHOP_*` enum values are still used. No proto/storage changes here.

- [ ] **l10n cleanup** in [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb) and [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb). Delete keys that only the removed widgets reference:
  - `workshopTileSemantics`
  - `workshopTileAskCircle`
  - `workshopCrewStripCaseAHeadline`, `workshopCrewStripCaseBHeadline`, `workshopCrewStripCaseADescription`, `workshopCrewStripCaseBDescription`, `workshopCrewStripCta`
  - `workshopAllCirclesPill` (no pill carousel in the brief layout — the active scope follows `selectedCommunityProvider` directly)
  - **Keep** `workshopHeaderTitle`, `workshopHeaderTimeOfDay{Morning,Afternoon,Evening}` (added in Phase 2), `workshopEmptyHeadline`, `workshopEmptySubtitle`, `workshopCatalystPullReviewTitle` and downstream catalyst-pull keys (still used by `catalyst_pull_review_screen.dart`), all `esmCard*` and `esmAuthor*` keys (still used by feed + brief lever).
  - Run `flutter gen-l10n`.

- [ ] **Workshop screen body collapses** in [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart):
  - Remove `_CirclePillCarousel` if the active scope follows the sidebar (no pill picker in the brief layout per the JSX mock); otherwise keep the pill widget but stop importing it from the workshop providers' deleted exports.
  - Body is: `state.hasBrief ? DailyBrief(...) : _EmptyPlaceholder()`, then a list of `CTARow`s, then the `_EmptyPlaceholder` collapses to a single line ("Nothing pressing this morning") since the brief itself handles the empty case.
  - Drop `WorkshopTileVariant` import + all references.

- [ ] **Test cleanup:**
  - Delete `app/test/presentation/widgets/workshop/{workshop_tile,crew_strip}_test.dart`.
  - Delete `app/test/presentation/screens/workshop/per_event_test.go` — wait, this is a server test under `server/services/workshop/`. The per-event server tests stay (the per-event Hero card pipeline still runs as a brief input). Only the wire-payload tests go.
  - Update remaining client tests to assert against `DailyBrief` / `CTARow`, not Hero card tiles.

- [ ] **Documentation:**
  - Update [docs/workshop.md](../workshop.md) to describe the brief-driven layout (handled separately in this same change set).
  - Append a "Phase 4 cleanup" section to [docs/ai/workshop.md](../ai/workshop.md) noting the legacy widgets / RPC are gone and pointing to issue #1618.

**Validates:** the full brief direction ships as one cohesive diff series. No code path on the Workshop screen renders anything other than `DailyBrief`, `CTARow`s, the floating header, and (on demand via the inline link) `MetricsSheet`. `flutter analyze` and `npm run build` are both clean. Every touched l10n key change shows up in both ARB files. The `GetWorkshopFeed` RPC and its payloads are removed from the proto contract.

### Phase 5: Bridge the gap to the reference design

**Goal:** the rendered Workshop tab matches [docs/cowork/mocks/workshop-brief.jsx](../cowork/mocks/workshop-brief.jsx) — host-voiced multi-priority prose with underlined inline levers, a metrics-rich italic closing line, action-icon CTA rows under a section header, and a full-screen slide-from-right Season Report. Phase 1-4 wired the contract; Phase 5 closes the visible gaps the implementation revealed.

**Observed gaps (from `actual.png` vs the reference screenshots):**
1. **The brief's prose has no inline lever spans.** The AI is producing `text_span`-only output — every `BriefPayload.segments` is plain text. The reference shows three underlined tappable levers (*"bump it to 7pm"*, *"weekly through July"*, *"drop her an @ in the smoker chat"*) embedded in prose.
2. **The brief is short and generic.** Today's actual rendering is a one-sentence "Round 2 picks up where the last one left off, building on what you've already started" — formulaic, no names / dates / counts. The reference is multi-sentence host-voiced prose.
3. **CTA rows duplicate the same event.** Multiple recent instances of the same-named experience produce identical CTA rows ("ON THE WAY · Round 2 of Morning Hike at Chautauqua is the natural next move" repeated). The reference shows one row per *rhythm*, not per *instance*, with varied action types.
4. **CTA rows have no action icons or circle chips.** The reference shows a left-side icon column (Activity for ESM, CalendarPlus for create, MessageSquare for chat, Edit3 for adjust, HelpCircle for help, AtSign for catalyst chat) and a right-aligned circle chip with a colored dot.
5. **CTA rows lack a section header.** The reference puts an "Else stirring around the circles" serif heading with a right-aligned uppercase eyebrow ("QUIETLY HUMMING") above the rows.
6. **Closing line lacks the season metrics.** Today shows *"12 events · 80 h together this season"* — fine but thin. Reference includes hours-together + replaced-cost + CO₂ avoided in the prose itself, not just a count.
7. **`MetricsSheet` is a bottom sheet, not a slide-from-right screen.** Reference shows a full-screen route with a "← Workshop" back-arrow header, "THE SEASON REPORT" eyebrow, "Together this season" big serif title, and per-circle breakdown rows on each panel.

**Tasks:**

- [ ] **Sharpen the brief prompt for inline levers.** Update [server/ai/prompts_workshop_brief.go](../../server/ai/prompts_workshop_brief.go):
  - Make the JSON-output instruction explicit that `segments` MUST contain at least one `lever_span` per priority (target: 2-3 lever spans across the prose). Few-shot examples with the JSX mock's exact prose ("bump it to 7pm", "weekly through July") teach the model the lever-span shape inline.
  - Tell the model to drop names / counts / dates from the signal into the prose verbatim — no paraphrasing the structural facts.
  - Add a "BAD OUTPUT" section showing single-sentence lever-less prose so the model learns what to avoid.
  - Goldens-driven (Phase 3's eval suite) — assert each case produces ≥2 `lever_span` segments and that lever `visible_text` ≤ 9 words.

- [ ] **CTA row de-duplication by rhythm at the brief assembler.** [server/services/workshop/brief_ranker.go](../../server/services/workshop/brief_ranker.go):
  - When multiple `StoredNudge` rows in the candidate set share an experience name (post-`shortName` normalization), keep the highest-priority one and drop the rest.
  - When a context_id appears in both an inline brief lever AND a candidate CTA row, drop it from the CTA rows.
  - Cap CTA rows at a documented maximum (suggest 5).
  - Prefer rows that vary in `cta_action` so the screen surfaces a mix of host actions (ESM + create + adjust + chat + help), not five "schedule_repeat" rows.
  - Unit test the dedup + cap rules with fixture signal sets.

- [ ] **Top-level metrics in `closing_line`.** [server/services/workshop/brief.go](../../server/services/workshop/brief.go) (or wherever the closing line is assembled):
  - Compute the per-host total of {together-time hours, replaced-cost USD, CO₂ avoided lbs/kg} across the resolved circle scope using `server/impact_metrics.Calculator` (already plumbed for the synthesis surface).
  - Pass these as structured fields on `BriefSignal` to the AI prompt so the model can phrase them in host voice ("67 hours together, ~$1,420 in tools borrowed instead of bought, ~720 lbs of CO₂ that didn't leave the road"). The prompt requires all three to appear in the closing line when non-zero — eval-suite assertion.
  - When all three are zero (genuinely quiet host), fall back to a softer closing ("Quiet stretch — that's a feature, not a bug.") with the link still pointing at the metrics screen.

- [ ] **CTA row icons in the proto + client render.** Reference design has icons grouped by `cta_action`. Two compatible approaches; pick (a) unless cross-platform-icon coverage forces (b):
  - (a) Client maps `cta_action` → `IconData` locally. `BriefCTARow.icon_kind` is implicit from `cta_action`. Add a small mapping table in `cta_row.dart`: `schedule_repeat`/`adjust_event` → `Icons.calendar_today`, `revive_experience` → `Icons.refresh`, `propose_share` → `Icons.handyman`, `seed_subhost` → `Icons.alternate_email`, `esm_author` → `Icons.poll`, `post_in_chat` → `Icons.chat_bubble_outline`, `ask_for_help` → `Icons.help_outline`. Single icon-color theme — warm-orange per the JSX.
  - (b) Server returns an `icon_kind` enum on `BriefCTARow` for finer control. Adds a proto change but lets the LLM influence which icon shows. Defer to follow-up unless the prompt-iteration phase needs the override.
  - Either way, the row gains a left icon-tile (38×38, rounded) styled like the JSX `CTARow` mock. Update `cta_row_test.dart` to assert the icon renders for each `cta_action`.

- [ ] **Right-aligned circle chip on every CTA row.** `BriefCTARow.community_id` already on the wire (verify). Client renders a tiny colored dot + circle short-name (e.g., "BACKCOUNTRY") at the row's right edge, mirroring the JSX. Reuse the existing circle-color resolution (sidebar selection state) — no l10n drift since the names are user-authored.

- [ ] **Section header above CTA rows: "Else stirring around the circles".** New widget [app/lib/presentation/widgets/workshop/cta_section_header.dart](../../app/lib/presentation/widgets/workshop/cta_section_header.dart) — serif title left-aligned, uppercase tracking-spaced eyebrow right-aligned ("QUIETLY HUMMING" / "STILL STIRRING"). The eyebrow can be server-authored on the brief response (so the AI can tune it to the day's tone) or static-l10n; default to server-authored field `BriefPayload.cta_section_eyebrow` with a static fallback. Add the field to the proto.
  - New l10n keys (en + es): `workshopBriefCtaSectionTitle = "Else stirring around the circles"` and a default eyebrow `workshopBriefCtaSectionEyebrowDefault = "STILL STIRRING"`.

- [ ] **Inline lever rendering correctness verification.** [app/lib/presentation/widgets/workshop/daily_brief.dart](../../app/lib/presentation/widgets/workshop/daily_brief.dart):
  - Confirm `BriefSegment.whichContent()` is correctly switching between `text_span` and `lever_span` (the today rendering suggests it may be falling through to the text-only path). Add a widget test that constructs a `BriefPayload` with mixed segments and asserts each `lever_span` renders as an underlined tappable run with the primary color.
  - When a lever's `visible_text` would render *without* the underline due to fallback styling, add a regression test.
  - The `TapGestureRecognizer` for each lever fires the `WorkshopLeverDispatcher` from Phase 2 — assert the dispatcher receives the `cta_action` + `context_id` from the tapped lever.

- [ ] **`MetricsSheet` becomes `SeasonReportScreen` (slide-from-right).** Replace the `showAccessibleModal` call in `daily_brief.dart` with `NavigationHelpers.pushScreen(SeasonReportScreen(...))` per [docs/client/architecture.md](../client/architecture.md) navigation conventions. New file at [app/lib/presentation/screens/workshop/season_report_screen.dart](../../app/lib/presentation/screens/workshop/season_report_screen.dart). Layout matches `ref_metrics.png`:
  - Header: `BackButtonWidget` + "Workshop" title bar, plain background.
  - Body: scroll view with
    - "THE SEASON REPORT" uppercase tracking-spaced eyebrow (warm-orange).
    - "Together this season" serif H1.
    - "Across your N circles · aggregate, no scoreboard" subtitle.
    - One panel per `SynthesisPanel` (kind = `time` / `replaced` / `co2`):
      - Section eyebrow (e.g., "TOGETHER-TIME") with a small icon (Activity / Sparkles / Leaf).
      - Serif headline (`SynthesisPanel.lede`).
      - Serif body (`SynthesisPanel.body`).
      - **Per-circle breakdown** — list of rows: colored dot + circle name + value (e.g., "Backcountry · 23h"). Comes from a new `SynthesisPanel.breakdown` field (see next task).
  - Move the 1-line static disclosure ("Aggregate only — no person named, no scoreboard.") from `together_this_season.dart` (now removed) into the screen footer.
  - Delete the old `metrics_sheet.dart` and any `showAccessibleModal` call site for it. Update `daily_brief.dart` widget test for the new push behavior.

- [ ] **Per-circle breakdown on `SynthesisPanel`.** Extend the proto:
  ```proto
  message SynthesisPanel {
    // ... existing fields
    repeated SynthesisPanelBreakdown breakdown = 5;
  }

  message SynthesisPanelBreakdown {
    string community_id = 1;
    string display_name = 2;
    string formatted_value = 3;  // "23h", "~$340", "~190 lbs"
  }
  ```
  Server-side ([server/services/workshop/synthesis.go](../../server/services/workshop/synthesis.go)): when computing `aggregateTotals`, also build a per-community breakdown and emit on each panel. Format integers per kind (`time` → `"23h"`, `replaced` → `"~$340"`, `co2` → `"~190 lbs"`). Round / approximate per the JSX mock data.
  Update `synthesis_test.go` to assert `breakdown` is populated.

- [ ] **Eval suite cases for the new prompt shape.** Add to [server/ai/eval/testdata/workshop_brief_goldens.json](../../server/ai/eval/testdata/workshop_brief_goldens.json):
  - "Multi-priority morning" — three signals, expects 3 lever spans + closing line with all three metrics.
  - "Lapsed regular" — covers the "@ Linda" pattern; expects `post_in_chat` lever with the user's name in `visible_text`.
  - "Verdict after pulse" — closed ESM window with ≥60%; expects a `schedule_repeat` lever with the proposed cadence ("weekly through July") in the `visible_text`.
  - "Quiet host" — no top-3 signals; expects empty `segments` and a soft closing line.
  - Each case asserts: lever count ≥ minimum for the scenario, no banned words, lever copy ≤ 9 words, closing line contains all non-zero metrics.

- [ ] **Documentation refresh.** Update [docs/workshop.md](../workshop.md):
  - Replace the "What's on the screen" ASCII diagram with a tightened version reflecting Phase 5 layout (icon column on CTA rows, "Else stirring" section header, full-screen Season Report instead of bottom sheet).
  - Add the new `BriefCTARow` icon-mapping table to the cta_action vocabulary section.
  - Note the per-circle breakdown on `SynthesisPanel`.

**Validates:** the rendered Workshop tab matches the reference design — multi-priority host-voiced brief with 2-3 underlined inline levers, metrics-rich closing line, deduped icon-prefixed CTA rows under a section header, and a full-screen Season Report with per-circle breakdowns. The eval suite catches regressions in lever density and metric inclusion. Visual diff (host on a dev account with seeded signal) is indistinguishable from `ref_brief.png` + `ref_actions.png` + `ref_metrics.png`.

## Related Issues

- **#1579** — Workshop tab v1 (replaces Impact screen). The brief is a follow-on render layer over the per-event Hero card pipeline that issue established. The `cta_action` vocabulary, the `StoredNudge.surface` enum, and the `EnsureHeroCard` per-event pipeline all stay; only the wire payload + render layer change.
- **#1580** — Lightweight ESM (post-event signal cards). The brief consumes the same signal: ESM repeat-with-≥60%-do_again becomes one of the inline levers (verdict-after-pulse). No proto changes needed in `esm_service.proto`.
- **Future issue (file alongside this)** — Wire `ExperienceStoriesJob` into `server/main.go` startup so `STORY_TYPE_EXPERIENCE_CONCLUDED` rows persist; the brief's recap lever lights up once that ships (Resolved Decision #9).
- **Future issue** — Streaming brief generation (Resolved Decision #1). Add `GenerateWorkshopBriefStreaming` once Phase 1's unary path is in production and we have data on first-paint latency.

---

## Audit Notes (2026-04-30)

Reviewed against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/testing.md](../client/testing.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md). Changes
applied in place — none of the audit notes alter the plan's structure,
only its compliance:

1. **Auth gate explicit on Phase 1 handler.** Original plan implied the handler was authenticated; now states `auth.RequireAuth(ctx)` is the first call. Every Workshop RPC takes the standard auth path per the server architecture doc.

2. **SQL Efficiency / N+1 risk called out.** The brief's signal aggregation reads multiple data sources (recent Hero card rows + catalyst suggestion + weekly need + recap availability + impact-metrics totals). Plan now explicitly forbids per-experience round-trips and requires `storage.AssertMaxQueries` regression coverage per the SQL Efficiency rules in [CLAUDE.md](../../CLAUDE.md).

3. **Cache TTL override mechanism resolved.** Open Question #2 originally said "check whether `CacheService.get` accepts a per-key TTL". Phase 2 now picks a concrete approach: prefer a separate cache namespace (`'workshop_brief'`) configured with a 15-min TTL, leaving the global `CACHE_TTL_MINUTES` default untouched. Falls back to extending `CacheService.get` with a per-call `ttl: Duration?` only if other surfaces want the same override.

4. **`TapGestureRecognizer` disposal made explicit.** `DailyBrief` is required to be a `StatefulWidget` that builds recognizers in a single rebuild method and disposes every entry in `dispose()`. Failing to dispose recognizers leaks closures over the `WidgetRef`. The widget test asserts the disposal happens.

5. **`ref.mounted` / `context.mounted` guards on the lever dispatcher.** Any handler that performs an `await` followed by a `Navigator.push` / `showAccessibleModal` MUST guard `if (!context.mounted) return;` per the Async Safety section of the client architecture doc. Plan was implicit on this; now explicit.

6. **Navigation pattern enforced.** The lever dispatcher's `post_in_chat` path goes through `NavigationHelpers.pushScreen` / `pushToItemScreen` per the navigation conventions — never raw `Navigator.push(MaterialPageRoute(...))` and never `context.push()` (reserved for auth flows). `npm run lint:nav` is the gate.

7. **i18n boundary clarified for time-of-day kicker.** Original plan said the helper returned a string (`"morning"` / `"afternoon"` / `"evening"`); now returns a typed `TimeOfDayPeriod` enum and the widget resolves via `context.l10n.workshopHeaderTimeOfDay{Morning,Afternoon,Evening}` in `build()`. ViewModels never carry strings per [docs/client/i18n.md](../client/i18n.md). New a11y label key `a11yWorkshopOpenSeasonMetrics` lands in **both** `app_en.arb` and `app_es.arb` before `npm run generate`.

8. **Two-kicker disambiguation.** The brief has two distinct kicker surfaces — the brief panel's own `BriefPayload.kicker_label` (server-authored, e.g., "Good Thursday morning") and the floating-header time-of-day kicker (client-side l10n, e.g., "Thursday morning"). Plan was ambiguous; now explicit so readers don't conflate them.

9. **`MetricsSheet` reuses server-authored labels.** Plan originally implied the sheet would invent client-side l10n keys for "Quality Time / Replaced Cost / CO₂ Avoided"; now reads the existing `GetWorkshopSynthesisResponse.panels` `label` / `lede` / `body` fields directly. Avoids server/client copy drift. Sheet chrome still uses `ModalHeader` + `IconAction` per [docs/client/accessibility.md](../client/accessibility.md).

10. **`AssertNoBannedWords` coverage extended.** Plan now runs the banned-words guard over **every** AI-generated text field — `kicker_label`, every `text_span`, every lever `visible_text`, and `closing_line` — not only the levers. Drops bad output before persistence.

11. **Proto field convention reminder.** Every conditionally-present field on `BriefPayload`, `BriefSegment`, `BriefLever`, `BriefCTARow`, and `StoredWorkshopBrief` uses the `optional` keyword per [CLAUDE.md](../../CLAUDE.md). Keeps `Has*()` presence checks explicit on the Dart side.

12. **Observability fields aligned.** Phase 1 logging fields now match [docs/server/observability.md](../server/observability.md) exactly: `operation`, `user_id`, `community_count`, `lever_count`, `duration_ms`, plus `external_service`, `provider`, `model` on AI calls. Log levels follow the doc's discipline — AI fallback at `Warn`, system failures at `Error`, user-input failures at `Info`/`Warn`.

13. **Service Independence + goroutine safety on Phase 3 pre-warming.** The `workshop_generation` job consumes the same shared libraries the workshop service consumes — it does NOT inject the workshop service. Goroutines spawned per-pair wrap in `logging.GoSafe` per the recover-defer rule; the CI gate at `server/cmd/check-goroutines` rejects bare `go ...`.

14. **Test coverage explicit per layer.** Phase 1 adds server-side unit tests for `BriefRanker`, the handler (auth, provider success, fallback, guard rejection), and `storage.AssertMaxQueries` regression coverage. Phase 2 adds `ProviderContainer` + Mockito ViewModel tests, the required `WorkshopNotifier` disposal test, and widget tests including the recognizer-disposal regression guard per [docs/client/testing.md](../client/testing.md).

15. **Validation language reframed away from "golden tests".** Plan's Phase 1 testing called out widget tests, ViewModel tests, and disposal tests — not "golden tests". This is correct per [docs/client/testing.md](../client/testing.md), where golden tests are listed as a future improvement. Eval-suite golden cases (Phase 3) live in `server/ai/eval/` under `//go:build benchmark` — different mechanism, not Flutter golden tests.

No structural reorganization of phases was needed; the original sequence (proto + server + AI seam → client widget + dispatcher + sheet → eval suite + pre-warming) holds up against the architecture rules.
