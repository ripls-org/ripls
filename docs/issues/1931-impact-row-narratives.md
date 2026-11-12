# Issue #1931: Per-user AI narrative copy for profile impact rows

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1931
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (depends on [docs/issues/1929-profile-v4-redesign.md](1929-profile-v4-redesign.md))

> **Audit notes (2026-05-15):** Reviewed against [client/architecture.md](../client/architecture.md), [client/caching.md](../client/caching.md), [client/i18n.md](../client/i18n.md), [client/accessibility.md](../client/accessibility.md), [client/testing.md](../client/testing.md), [server/architecture.md](../server/architecture.md), [server/observability.md](../server/observability.md). Gaps flagged inline with **[audit]** tags. The material adjustments: (1) the background `EnsureNarratives` goroutine must use a detached context, not the request `ctx` (which cancels on response); (2) Phase 5 extends `ViewerProfileState` — coordinate with the parallel #1929 / #1930 PRs to avoid Freezed-regen merge collisions; (3) the i18n fallback path needs explicit locale-detection (non-English locales always render the ARB string regardless of whether the per-user field is populated, since v1 generates English only); (4) `connecterr.Internal` for any error surfaced to the API layer.

## Problem Statement

The V4-A profile redesign (#1929) places three impact rows on the user profile (Time given, $ saved, CO₂ avoided), each pairing a metric value with a **one-line serif narrative**:

> **TIME GIVEN · 300 HRS**
> Lent enough hours for a season of trail work.
>
> **$ SAVED · $955**
> Spared the crew a summer of camp meals.
>
> **CO₂ AVOIDED · 57 KG**
> Kept a flight to Mexico City out of the air.

The V4-A PR ships static, ARB-based fallback strings — the same sentence for every user, per metric (`userProfileImpactTimeNarrative`, `userProfileImpactMoneyNarrative`, `userProfileImpactCo2Narrative`). Every user reads identical copy. This issue covers replacing them with per-user, AI-generated narrative.

The natural extension point is the existing `GenerateHeroCardCopy` pattern in [server/services/workshop/copy_provider.go](../../server/services/workshop/copy_provider.go) and the `Provider` interface at [server/ai/provider.go:14-150](../../server/ai/provider.go) — this is the production-shape for AI-tuned prose that has fallback-to-template, persistent storage, and prompt-version logging. The deprecated `BriefPayload.kicker_label` / `closing_line` ([proto/ripls/api/workshop_service.proto:147-158](../../proto/ripls/api/workshop_service.proto#L147)) prove the codebase has *moved away* from inline AI prose paragraphs toward structured cards, but `GenerateHeroCardCopy` shows the codebase still generates short evocative copy when it's payload-shaped.

The substantive design calls are: **generation cadence**, **storage shape**, and **failure path**.

## Context from Comments

No comments on this issue (filed today). The body and #1929 give the visual + fallback requirements. The open work is in the generation pattern.

## Potential Solutions

### Option A: Cached-per-user, milestone-regenerated (matches Hero Card pattern)

Add `GenerateImpactRowNarratives` to the `Provider` interface. Persist three narrative strings keyed by user (new `models.UserImpactNarratives` storage row or three new columns on an existing user-impact row). Regenerate when the underlying metric crosses a milestone threshold (e.g. +10% from last generation, or a new round number).

- **Approach:** Mirror [server/services/workshop/generation.go:186-212](../../server/services/workshop/generation.go#L186) — on-demand or scheduled regeneration, idempotent generation so re-runs are safe, soft-delete + insert pattern. Stored copy lives in DB; cheap reads on profile fetch.
- **Pros:** Matches existing production pattern. Provider abstraction (Anthropic / Gemini / OpenAI / Fallback) already plumbed. Cheap per-read. `prompt_version` logging for eval correlation. Background generation via the existing `WorkshopGenerationJob` pattern means the user-facing path never blocks on AI.
- **Cons:** New storage shape. Milestone-detection logic — when do we regenerate? Too-eager regeneration is expensive; too-lazy reads stale.
- **Effort:** Medium-Large.

### Option B: Generated inline, response-cached with TTL

Generate at profile-fetch time when no cached narrative exists; cache the result in-memory or in Redis/Postgres with a fixed TTL (e.g. 7 days). No milestone tracking.

- **Approach:** Simpler than A — TTL drives regeneration, no milestone logic. But profile fetch blocks on AI generation when the cache is cold.
- **Pros:** No milestone heuristic to tune. No background job. Less storage code.
- **Cons:** Cold-fetch latency is bad — Hero Card-style generation takes hundreds of ms. Either we block the profile response (unacceptable) or we serve fallback ARB on cold cache and fill in next time (acceptable but unsatisfying). Cache invalidation on user activity is harder — TTL doesn't reflect "this user just had a big transfer."
- **Effort:** Medium.

### Option C: On-demand generation, dual-key cache (per user + per "metric bucket")

Same as A but the cache key includes a bucketed version of the metric value (e.g. `floor(hours / 50)`), so the same narrative reuses across users with similar totals. New users with low totals hit the same cached copy as other low-total users.

- **Approach:** Shared cache space. The prompt is per-user but the *output* is shareable across users in the same bucket. Trades personalization for cost.
- **Pros:** Fewer total AI calls. New users get instant narratives (cache hit on first low-bucket fetch).
- **Cons:** The output is no longer truly per-user — it's per-bucket. Defeats the purpose of the issue. Bucket sizing is its own design problem.
- **Effort:** Medium.

### Option D: Skip AI entirely, hand-curate a tiered template library

Write ~50 tier-banded narratives per metric ("Lent enough hours for a season of trail work" at 200–400 hrs, "Lent enough hours to retile a community center" at 400–600 hrs, etc.). Pick from the bands at render time.

- **Approach:** No AI. Pure data-driven prose with bucketed selection.
- **Pros:** Zero AI cost. Zero hallucination. Deterministic. Easy to translate.
- **Cons:** Static — every user in the same band reads the same sentence. The whole point of the issue is per-user evocative copy. Curation cost is real and never stops growing.
- **Effort:** Small (engineering) + Large (copywriting).

## Recommendation

**Option A (cached-per-user, milestone-regenerated).**

Reasoning:
- The Hero Card copy generator at [server/services/workshop/copy_provider.go](../../server/services/workshop/copy_provider.go) is the closest production analog: persistent storage, fallback-to-template (the V4-A ARB strings are the template), idempotent regeneration, prompt-version logging. The plumbing is exactly the same shape — we'd be following an established path, not inventing one.
- Per-user narrative is the whole point. Bucketed sharing (C) and tier templates (D) regress that to "static" or "shared per band." Option B serves stale fallback on cold cache, which is acceptable but loses persistence.
- Generation runs in a background job and writes to the DB. The user-facing `GetUserProfileForViewer` reads the cached row; it never blocks on AI.
- The milestone detection is a real cost but a one-time cost. The Workshop generation infrastructure already handles "regenerate when signal materially changes" — we follow that shape.

Caveats:
- This is a Large effort. It should not block #1929's V4-A redesign. The V4-A PR's static ARB fallback is the safety net.
- The "Mexico City flight" / "season of trail work" specificity in the example copy is high. Initial eval coverage needs to confirm the prompt produces that quality at Haiku speed, or we accept that v1 is less vivid than the mockups.

## Open Questions (Resolved)

All resolved 2026-05-15 in favor of Option A. Decisions are load-bearing.

- **Approach:** Option A — provider-driven generation, persisted per user, milestone-regenerated.
- **Storage:** new `models.UserImpactNarratives` storage row keyed by `user_id`, columns: `time_narrative`, `money_narrative`, `co2_narrative`, `generated_at`, `prompt_version`, `inputs_hash` (sha256 of the activity-context inputs, used for milestone detection). Soft-delete + re-insert on regeneration, matching `StoredNudge`.
- **Generation cadence:** generate on first profile fetch when no row exists (background-kicked off, fallback ARB serves the first view). Regenerate when `inputs_hash` changes meaningfully — `floor(time_banked_minutes / 60 / 50)` (50-hour buckets), `floor(cost_savings_usd / 100)` ($100 buckets), `floor(carbon_savings_grams / 1000 / 10)` (10kg buckets) crosses a boundary. The bucket sizes are tunable constants in the milestone-detection file.
- **Wire shape:** three `optional string` fields on `GetUserProfileForViewerResponse`: `time_narrative`, `money_narrative`, `co2_narrative`. Optional per [CLAUDE.md §Proto Field Conventions](../../CLAUDE.md#proto-field-conventions) — absent when generation hasn't run yet.
- **Locale:** v1 is English-only. The `region` parameter already threaded through `Provider.Generate*` methods anchors regional flavor (city names, regional activities) but not language. Multi-locale generation is a separate follow-up; until then non-English locales fall back to the V4-A ARB strings (which *are* localized).
- **Failure path:** any generation error logs at Error, returns no row. Client widget sees `null` and renders the V4-A ARB fallback. The path is graceful by design — the V4-A redesign was built to work without these fields.
- **Eval:** new gold set under `server/ai/eval/testdata/user_impact_narratives_*.json` covering: low-activity user (sub-threshold values), mid-tier (200 hrs / $500 / 30 kg), high-tier (500 hrs / $2000 / 100 kg), edge cases (zero in one dimension). Assertion type: tone match (Jaccard similarity on evocative keywords) + length constraint (one sentence, ≤ 90 chars) + no hallucinated specific numbers (the prose should *evoke*, not *re-quote*).
- **Caching client-side:** the new fields ride on the existing profile-response cache. No new client-side cache key. Pull-to-refresh clears it as today.
- **Cost gating:** the milestone-detection means the same user does not get regenerated on every fetch. Worst case: a user transfers a high-value item, crosses a bucket boundary, regenerates once. Eyeball expected regeneration rate at ~1–2 per active user per month.

## Implementation Plan

### Phase 1: Storage + proto skeleton

**Goal:** Wire the field through with empty values so the client can read the new fields without blocking on the AI work.

**Tasks:**
- [ ] Add `models.UserImpactNarratives` storage message (new file [proto/ripls/models/user_impact_narratives.proto](../../proto/ripls/models/user_impact_narratives.proto)):
  ```protobuf
  message UserImpactNarratives {
    string user_id = 1;
    optional string time_narrative = 2;
    optional string money_narrative = 3;
    optional string co2_narrative = 4;
    optional int64 generated_at_unix_sec = 5;
    optional string prompt_version = 6;
    optional string inputs_hash = 7;  // sha256 hex of bucketed metric inputs
  }
  ```
  Per [CLAUDE.md §Proto Comments](../../CLAUDE.md#proto-comments) and §Proto Field Conventions — `optional` for absent-able fields; no vendor names in storage comments (storage protos are server-internal but the rule still applies for consistency).
- [ ] Add the three new `optional string` fields on `GetUserProfileForViewerResponse` in [proto/ripls/api/profile_service.proto](../../proto/ripls/api/profile_service.proto). API-comment them as user-facing narrative; never name the provider or model.
- [ ] Add a storage migration to create the `user_impact_narratives` table.
- [ ] Run `npm run generate`.
- [ ] In [server/services/profile/profile.go](../../server/services/profile/profile.go), read the narratives row when it exists, populate the new response fields. When the row doesn't exist, the fields stay absent. Land this first — wire shape is stable before AI lands.

**Validates:** Proto compiles. A row inserted by hand surfaces in the response. Client reads the new fields.

### Phase 2: AI provider method

**Goal:** Add the new generator to the `Provider` interface and the three concrete providers (Anthropic, Gemini, OpenAI).

**Tasks:**
- [ ] Add `GenerateImpactRowNarratives(ctx, *ImpactRowSignal) (*ImpactRowNarratives, error)` to the `Provider` interface in [server/ai/provider.go](../../server/ai/provider.go). Match the shape of `GenerateHeroCardCopy`. **CLAUDE.md note:** since this is an exported symbol, the doc comment must start with the name and avoid stuttering.
- [ ] Define input struct `ImpactRowSignal` (new file `server/ai/impact_row.go`):
  ```go
  type ImpactRowSignal struct {
      Region            string
      TimeBankedMinutes float64
      CostSavingsUSD    float64
      CarbonSavingsKg   float64
      TopCategories     []string // From #1930 known_for if available
      ActiveCommunity   string
      PromptVersion     string
  }
  type ImpactRowNarratives struct {
      Time, Money, CO2 string
      PromptVersion    string
  }
  ```
- [ ] Implement on Anthropic ([server/ai/provider_anthropic.go](../../server/ai/provider_anthropic.go)) following `GenerateHeroCardCopy`'s shape. Build the prompt in [server/ai/prompts.go](../../server/ai/prompts.go) — `buildImpactRowNarrativePrompt(s *ImpactRowSignal) string`. Use existing tone guidance and length constraint (one sentence, ≤ 90 chars per narrative).
- [ ] Implement on Gemini and OpenAI providers.
- [ ] Mock provider returns deterministic strings for tests.
- [ ] LoadBalanced and Fallback providers route to the appropriate underlying call automatically (they implement the interface, so the new method joins via the same dispatch).
- [ ] **Structured logging** per [server/observability.md](../server/observability.md): every call site adds `operation`, `provider`, `model`, `prompt_version`, `user_id`, plus per-provider duration. `FallbackProvider.withFallback` already rewrites `provider` to the implementation that served — preserved.
- [ ] **Error handling** per [CLAUDE.md §Error Handling](../../CLAUDE.md#error-handling-and-robustness): any provider error returns to the caller. Never swallow.

**Validates:** Unit test on `provider_mock.go` confirms interface conformance. `npm run build` + `go build -tags=integration ./...` pass (per [CLAUDE.md §Go Server Build Verification](../../CLAUDE.md#go-server-build-verification), the integration tag matters because integration tests under `server/ai/eval/` will eventually call this method).

### Phase 3: Generation service + milestone detection

**Goal:** Wire the provider call into a background-generation path that populates the storage row.

**Tasks:**
- [ ] New service file [server/services/profile/narratives.go](../../server/services/profile/narratives.go):
  - `EnsureNarratives(ctx, userID) error` — checks `user_impact_narratives` for the user; if absent or `inputs_hash` mismatch, regenerates.
  - `computeInputsHash(metrics UserImpactMetrics) string` — sha256 of bucketed values: time bucket (50-hour granularity), money bucket ($100 granularity), CO₂ bucket (10kg granularity). When any bucket changes, hash changes, regeneration fires.
  - Calls `Provider.GenerateImpactRowNarratives` with the user's current metrics + region + top categories from #1930.
  - Soft-deletes the old row and inserts the new one (mirroring [server/services/workshop/generation.go:117](../../server/services/workshop/generation.go#L117)).
- [ ] On `GetUserProfileForViewer`, kick off `EnsureNarratives` in a goroutine (background, non-blocking) before reading the current row. The handler returns whatever is currently stored. First fetch: returns empty fields, client falls back to ARB. Next fetch a few seconds later: returns generated narratives. **[audit] Context lifecycle:** the goroutine must NOT use the request `ctx` — that ctx cancels when the response is sent, killing the generation mid-flight. Mirror the precedent in [server/services/workshop/generation.go:129 `kickoffNudgeMaterialization`](../../server/services/workshop/generation.go#L129) — use `context.Background()` with a fresh `logging.WithRequestID(bgCtx, requestID)` so the background log lines stay correlated to the originating call per [server/observability.md §Streams and per-event correlation](../server/observability.md).
- [ ] **Structured logging** at the generation site: `logger := logging.LoggerWithContext(bgCtx).With("operation", "EnsureImpactNarratives", "user_id", userID, "prompt_version", promptVersion, "regenerated", bool)`. Log inputs hash change at Info, fresh-generation success at Info, provider failure at Error.
- [ ] **[audit] Error handling on the synchronous read path:** when reading the existing narratives row, any storage error (other than not-found) returns `connecterr.Internal(ctx, "GetUserProfileForViewer", err, "user_id", userID)` per [CLAUDE.md §Error Handling](../../CLAUDE.md#error-handling-and-robustness). Not-found is the expected first-fetch case — return absent fields silently, the client falls back to ARB.
- [ ] **Cache invalidation client-side:** the profile response cache holds narratives ride-along. When mutations land that change the user's metrics (transfers, experiences), the existing profile-cache invalidation handles them by way of the global TTL. No new client cache wiring per [client/caching.md](../client/caching.md) (the V4-A PR landed pull-to-refresh; that's the manual invalidation path).

**Validates:** A user with metrics + no row gets a row asynchronously. Subsequent fetches return populated narratives. Hash collision (same bucket on next call) is a no-op.

### Phase 4: Prompt + evals

**Goal:** Lock the prompt and gate regressions.

**Tasks:**
- [ ] Write the prompt in [server/ai/prompts.go](../../server/ai/prompts.go) following the project's existing tone guidance (Hero Card is the model). Constraints in the system prompt: each narrative is one sentence, ≤ 90 chars, evokes magnitude without re-quoting the number, prefers concrete regional anchors over abstract ones.
- [ ] New eval gold set under [server/ai/eval/testdata/](../../server/ai/eval/testdata/) — `user_impact_narratives_text.json`. Covers: low-tier (50 hrs / $100 / 5 kg), mid-tier (200 hrs / $500 / 30 kg), high-tier (500 hrs / $2000 / 100 kg), one-dimension-zero edge cases. Eval assertions per [docs/evals/](../evals/) conventions:
  - Length constraint: each narrative ≤ 90 chars (deterministic).
  - No re-quoted numerals: the original metric value must not appear verbatim in the narrative.
  - Tone-match: Jaccard similarity on a curated keyword list per metric ("trail", "season", "lent", "shared", "kept", etc.) — tunable in the test file.
- [ ] Eval runs in CI as `go test -tags=integration ./server/ai/eval/...` per [CLAUDE.md §Testing](../../CLAUDE.md#testing).
- [ ] Server unit tests for the milestone-detection (`computeInputsHash`) — same inputs → same hash; bucket boundary → new hash. Wrap in `storage.AssertMaxQueries(t, ctx, 2, fn)` to lock the read+write query budget.

**Validates:** Eval gold set passes on Anthropic + Gemini + OpenAI. Milestone hash is stable across calls with same inputs.

### Phase 5: Client wiring

**Goal:** Replace the ARB fallback with the new fields when present.

**Tasks:**
- [ ] Extend `ViewerProfileState` in [app/lib/presentation/viewmodels/viewer_profile_view_model.dart](../../app/lib/presentation/viewmodels/viewer_profile_view_model.dart): three new optional fields `timeNarrative`, `moneyNarrative`, `co2Narrative`. Populate from the new proto fields with `response.hasTimeNarrative() ? response.timeNarrative : null` (etc.). **[audit] Merge-coordination:** #1929 already extends this same Freezed class to add `userImpactMetrics`, and #1930 extends it again with `knownFor`. If these PRs land out of order, expect Freezed regeneration conflicts. Call out the dependency in the PR description and rebase against `main` before merging.
- [ ] In `user_profile_impact_rows.dart` (from #1929): prefer the per-user narrative when non-null **and the active locale is English**, fall back to the ARB string otherwise. Check the locale via `Localizations.localeOf(context).languageCode == 'en'`. **[audit] i18n** per [client/i18n.md](../client/i18n.md): viewmodels never resolve strings, so the locale check lives in the widget. The ARB fallback is always required — the field is hidden when generation hasn't run *or* when the locale is non-English (v1 generates English only).
- [ ] **ViewModel test** under `app/test/presentation/viewmodels/viewer_profile_view_model_test.dart` per [client/testing.md](../client/testing.md):
  - Response with all three narratives → state carries all three.
  - Response with one narrative, two absent → state carries one, the other two are null.
  - Response with none → all three null, widget falls back to ARB (covered by widget test).
- [ ] Widget test for the impact-rows widget: narrative present → renders narrative; narrative null → renders ARB fallback.

**Validates:** End-to-end: prompt → provider → storage → proto → repository cache → viewmodel state → impact-rows widget → fallback path on null. `npm run build && npm run test:app && go test -tags=integration ./...` all pass.

## Related Issues

- #1929 — V4-A profile redesign. This issue replaces the ARB fallback strings landed there.
- #1930 — Known For capability tags. Sibling follow-up; same "AI vs derived" design space but for chips, not prose. The `TopCategories` field in `ImpactRowSignal` reuses #1930's output when available, so #1930 should land first or be in flight by Phase 3.
- Possible follow-ups, file later:
  - "Multi-locale impact-row narratives" (per-locale generation or post-generation translation).
  - "Impact-row narrative regeneration triggered by community membership changes" — joining a new community might shift the regional anchor in the prose.
