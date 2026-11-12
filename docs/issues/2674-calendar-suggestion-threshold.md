# Issue #2674: Calendar suggestions are awkward

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2674
**Filed:** 2026-07-08 | **Last activity:** 2026-07-08
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/calendar.md (⚓ calendar/inbox domain), docs/weather.md (suggestion confidence gate), docs/proto_conventions.md (⚓), docs/server/architecture.md (⚓ per-section assemblers), server/ai/README.md + server/storage/embedding.go (the embedding subsystem this reuses). Manual directives from the requester: (1) *"suppress suggestions unless there are at least two similar events."* (2) *"we need some amount of fuzzy/semantic matching so the activities need not be identical to accrue multiple occurrences"* — reuse the existing search embedding for clustering.

## Problem Statement

The open-day calendar suggestion engine ([`home_suggestions.go`](../../server/services/portfolio/home_suggestions.go))
proposes "something to do" on empty calendar days, built from the viewer's past
activities × their usual crew × that day's weather. A prod user
(0.26.3, Android) reported the suggestions read as **awkward / premature**:

> "maybe wait to trigger these until you have a lot more data to draw from? I find it cute but others maybe react differently"

Two compounding root causes:

1. **A trigger threshold of one.** `viewerActivities` (`home_suggestions.go:111`)
   counts each activity the viewer was involved in and returns it as suggestable
   **even at `count == 1`**. A single occurrence then drives a full editorial
   suggestion, including the reason string `suggestionReason`
   (`home_suggestions.go:181`): `"You've done x 1× before, mostly on days like
   this."` — confidently pattern-matching off one data point, with a crew line
   ("…usually come along") implying a habit that doesn't exist yet.

2. **Exact-name keying is too brittle to accumulate occurrences.** Activities are
   keyed by the trimmed `Experience.Name` (`home_suggestions.go:118,134`). Even
   if we require ≥2, "Flatirons hike" and "Sunday hike" stay two separate
   count-1 activities and never clear the bar. Real repeated behavior is spread
   across near-but-not-identical titles, so a naive exact-match ≥2 rule would
   suppress almost everything — over-correcting into the opposite complaint.

**The directives** (from the requester, authoritative over the vaguer issue
text): suppress an activity unless the viewer has **at least two similar
events**, where "similar" is **semantic, not identical** — and reuse the
**existing search embedding** to decide similarity rather than building a new
matcher.

When no activity clears the bar, the engine emits nothing and the client falls
back to its existing, non-awkward empty-day nudge / static "nothing planned"
prompt — a path that already exists and needs no client change
(`calendar_day_detail.dart:229`, `docs/client/calendar.md` §"Weather and
open-day suggestions").

## Context from Comments

**The issue has no comments** — it is a single piece of raw in-app user feedback
plus three screenshots (the awkward suggestion as the user saw it). The
actionable direction comes from the two requester instructions above. The lack
of thread discussion is not a blocker; the one genuine ambiguity (how
aggressively to cluster) is captured in Open Questions and made a tunable
constant so it can be calibrated post-merge.

## UI Investigation — why skipped

The `ripls-issue` skill asks for a `ripls-ui-investigate` pass on UI-observable
reports. Skipped deliberately — not because the behavior can't render on web (it
can), but because the investigation's job is to **bisect an unknown failure to a
layer**, and here the layer is unambiguous from a direct read of the code:

- The suggestion is **server-assembled** (`assembleOpenDaySuggestions` →
  `GetHomeViewResponse.open_day_suggestions`) and rendered verbatim by
  `CalendarSuggestionPanel`. The client invents nothing.
- The panel is behaving exactly as designed; the "awkwardness" is a
  **product-tuning judgment** (threshold too low + grouping too brittle), not a
  render/state bug.

A hermetic repro would confirm what the code already shows. Phase 2 keeps an
optional e2e confirmation for the reviewer who wants end-to-end proof.

## How the reuse works (feasibility)

The existing semantic-search stack (`docs/server/semantic_search.md`) makes
clustering cheap **and** tells us how to build it in-convention:

- **`Experience` is already embedding-configured** — `protosql_schema.go:265`
  registers `Embedding: {Fields: ["name","description"]}`, so every row carries a
  stored **384-dim** name+description vector. *(Update: experiment below showed
  the name+description vector clusters activities poorly, so a second **name-only**
  embedding variant is now stored on the same table and read for clustering —
  the name+description column stays the search embedding. Both are precomputed;
  see the implementation-status note.)*
- **The embedder is a local in-process ONNX model** (`ripls-minilm-v1`,
  `server/ai/embedding/embedding.go`), not an external API, so even the fallback
  (embedding a fresh string) is a sub-millisecond local call with no network hop
  and no per-call cost.
- **Vectors are L2-normalized** (`embedding.go:normalize`), so cosine similarity
  is a plain **dot product** — a ~20-line helper, no pgvector round-trip needed
  for in-process clustering of a small set.
- **Clustering is tiny** — a viewer has tens of distinct experiences; greedy
  agglomerative clustering over their own history is negligible CPU inside
  `GetHomeView`.
- **There is prior art for the threshold and the fallback.** The doc's design
  principles govern this change:
  - *Storage-Layer Encapsulation* — "**all embedding logic lives in the storage
    layer; services call query methods without knowledge of whether semantic or
    text search is used.**" So the vector read + cosine + threshold must live in
    **storage**, exposed as a grouping method; the portfolio service must not
    handle raw `[]float32` (this is an audit correction — see Option A).
  - *Graceful Degradation* — semantic path falls back to text/exact matching.
    Our lexical fallback for un-embedded experiences is the same pattern.
  - *Similarity Threshold* — `SemanticSearchMinSimilarity = 0.5`
    (`protosql_types.go:21`) is the **search** cutoff, chosen for recall.
    Documented empirical bands for this fine-tuned model: matching pairs ~0.91,
    good matches ~0.55–0.65, moderately related ~0.4–0.5, noise floor ~0.3–0.4.
    **Clustering wants precision, not recall** (a wrong merge resurrects the
    "awkward" complaint), so the cluster threshold should sit at/above the
    good-match band — a separate constant, calibrated (see Open Questions), not
    a blind reuse of 0.5.

## Potential Solutions

### Option A: Storage-layer semantic grouping over stored vectors, ≥2-per-cluster threshold (recommended)

- **Approach (revised in audit to honor Storage-Layer Encapsulation):**
  1. **Storage owns the embedding math.** Add a storage method
     `GroupIDsBySimilarity(ctx, tableName, info, ids, minSimilarity) → [][]string`
     to `server/storage/embedding.go`: it reads the stored vectors for `ids`
     (one indexed by-id SELECT of the pgvector column), greedy-clusters them
     in-process by cosine (dot product of the L2-normalized vectors ≥
     `minSimilarity`) in a deterministic id-sorted order, and returns groups of
     ids. Ids whose vector is NULL/missing come back as **singleton groups**
     (caller applies its own lexical fallback). The cosine helper + threshold
     live here, beside `QueryByEmbeddingSimilarity` and `SemanticSearchMinSimilarity`
     — the portfolio service never touches a `[]float32`. Unit-test the greedy
     clusterer with synthetic vectors (no model needed).
  2. **fetchAll consumes groups, not vectors.** In `fetchAll` (`fetch.go`), call
     `GroupIDsBySimilarity` for the viewer's experience ids (best-effort: on
     error or no embedder, leave groups empty → step 3 degrades to lexical) and
     store the result as `fetchedData.expActivityGroups [][]string`. Keeps all
     DB I/O in fetch.go and `viewerActivities` a **pure function** over
     prefetched data (preserves the existing hand-authored unit-test style).
  3. **`viewerActivities` builds one `activityStat` per group.** For each group,
     merge its experiences: `count` = group size, `crew` = union of co-attendees,
     start-time avg across the group, `label` = the group's most-frequent
     (earliest as tiebreak) exact name as the human-facing representative. When
     `expActivityGroups` is empty (fallback), key by exact trimmed name as today
     — so identical titles still merge even before vectors land (the documented
     *Graceful Degradation* pattern).
  4. In `assembleOpenDaySuggestions`, keep only stats with
     `count >= minActivityOccurrences` (= 2); if none, `return nil` (existing
     no-history contract → client falls back to the nudge).
  5. **Constants (as implemented).** `minActivityOccurrences = 2` and
     `activityClusterMinSimilarity = 0.6` both live in the portfolio tuning
     block (`home_suggestions.go`); the storage primitive
     `GroupIDsBySimilarity(..., minSimilarity)` takes the threshold as a
     **parameter**, staying generic and reusable rather than hardcoding an
     activity-specific constant into storage. This satisfies the audit's real
     goal — no vectors in the service — while keeping storage free of a
     portfolio concept. Passing `minSimilarity = 1.0` recovers exact-name
     behavior, so the semantic rule **generalizes** the "two similar events"
     directive.
- **Pros:** Directly implements both directives; reuses the exact embedding space
  search matches on (no new model/infra); keeps embedding logic in storage per
  the documented principle; no proto or client change; adds one cheap by-id
  SELECT and zero embedding generation to the hot path; the greedy clusterer and
  the ≥2 policy are each pure and unit-testable; fixes the awkward `1×` reason
  string as a side effect.
- **Cons:** The stored vector embeds `name` **+ `description`**, so similarity is
  "whole experience," not purely "activity kind" — description noise can
  occasionally merge unlike activities or split like ones. Mitigation is the
  threshold knob or Option B's name-only vectors. Threshold needs empirical
  calibration (Phase 2).
- **Effort:** Medium.

### Option B: Semantic clustering over name-only vectors generated in the hot path

- **Approach:** Same storage-owned grouping, but instead of reading stored
  vectors the storage method generates a **name-only** embedding per distinct
  label via the local embedder (`embedder.Generate(ctx, name)`) and clusters
  those. (Still behind the storage method — the service stays vector-free.)
- **Pros:** Similarity keys on the activity name alone (no description
  pollution); no missing-vector gap; no new storage query.
- **Cons:** Does N local ONNX inferences per home view (N = distinct labels,
  ~tens) — still cheap but not free, and adds an embedder dependency to the
  portfolio service's hot path; less literally "reuse the stored search vectors."
  Best held in reserve as the mitigation if Option A shows description-driven
  mis-clustering in calibration.
- **Effort:** Medium.

### Option C: Exact-name ≥2 only (rejected — insufficient per directive 2)

- **Approach:** Require `count >= 2` on the current exact-name key, no semantics.
- **Cons:** The requester explicitly ruled this out ("need not be identical").
  Real repeated behavior spread across varied titles would be suppressed,
  swapping the "too eager" complaint for "never suggests." Kept only as the
  `threshold = 1.0` degenerate case of Option A, not a shipped design.
- **Effort:** Small.

### Option D: Write-time stored `activity_kind` field (rejected for now — scope)

- **Approach:** Derive a canonical activity kind once at experience-create time
  and store it on the model; group by that field.
- **Cons:** Biggest change (proto/model field + backfill + write-path hook) for
  a tuning fix. Option A already reuses precomputed vectors, giving most of the
  read-path cleanliness without the schema churn. Revisit only if clustering
  logic wants to move off the read path entirely.
- **Effort:** Large.

## Recommendation

**Option A.** It satisfies both directives with the least new surface area:
reuse the already-stored, already-search-tuned embeddings; keep the vector math
in storage (per *Storage-Layer Encapsulation*) behind a pure, testable grouping
method; gate on `count >= 2` per group; fall back to the existing empty-state
path with zero client work. The two behavioral knobs (`minActivityOccurrences`
in portfolio, `ActivityClusterMinSimilarity` in storage) are constants, so if
"two" or the similarity cutoff proves off in prod it's a one-line, regenerate-
free deploy. Hold **Option B** (name-only vectors) in reserve as the mitigation
if Phase 2 calibration shows description text driving bad merges.

## Open Questions

- **What cosine threshold counts as "similar"?** — Recommendation: a **new
  `ActivityClusterMinSimilarity` constant**, distinct from search's
  `SemanticSearchMinSimilarity = 0.5`. Ground the starting value in the
  documented bands for this fine-tuned model (matching pairs ~0.91, good matches
  ~0.55–0.65, noise floor ~0.3–0.4): start at **~0.6** and calibrate in Phase 2
  with the fixture below. Bias **high/stricter** if unsure — a missed merge
  (fewer suggestions) is less bad than a wrong merge that resurrects the
  "awkward" complaint. Do **not** just reuse 0.5: that cutoff was tuned for
  search recall, and clustering wants precision.
- **name+description vs. name-only similarity (Option A vs B)?** —
  Recommendation: **name+description (Option A)** for v1 since the vectors
  already exist; watch calibration for description-driven mis-clusters and
  switch to name-only (B) only if they appear. Flag as the primary quality risk.
- **Cluster representative label + reason wording.** — Recommendation:
  representative = the **most-frequent exact name** in the cluster (earliest as
  tiebreak). Keep `suggestionReason`'s "You've done X N× before" using the
  cluster count N (now always ≥2), so the phrasing that read as awkward at `1×`
  is structurally gone.
- **Missing/stale vectors (async embedding lag).** — Recommendation: **lexical
  exact-name fallback** for un-embedded experiences (identical titles still
  merge); near-duplicates simply won't merge until their vector lands, which is
  acceptable for a gentle feature. Log nothing loud — this is expected, not an
  error.
- **Config vs. constant for the knobs.** — Recommendation: **constants**, matching
  the sibling knobs (`maxOpenDaySuggestions`, `suggestionCrewSize`). Revisit only
  if we want to A/B the threshold.

## Implementation Plan

### Phase 1: Storage grouping primitive + ≥2 threshold (the fix)

**Goal:** Suggestions appear only for activity **groups** the viewer has done ≥2
times, where membership is decided by embedding similarity computed in storage;
thin/varied history yields no suggestion and the client falls back to its
existing nudge.

**Tasks:**
- [ ] In `server/storage/embedding.go`: add
      `GroupIDsBySimilarity(ctx, tableName, info, ids, minSimilarity) → [][]string`
      (batched by-id SELECT of the pgvector column; deterministic id-sorted
      greedy cosine clustering via a pure `dot`/`cosine` helper on the
      L2-normalized vectors; NULL-vector ids returned as singletons). Add
      `ActivityClusterMinSimilarity` beside `SemanticSearchMinSimilarity`,
      commented with the #2674 rationale and why it differs from the search
      cutoff (durable issue number, no ephemeral phase labels).
- [ ] **Pure unit tests (no model)** for the clusterer with **synthetic**
      vectors: identical vectors merge; orthogonal split; threshold boundary;
      NULL → singleton; `minSimilarity = 1.0` recovers exact grouping;
      `dot()` on identical/orthogonal/mismatched-length inputs.
- [ ] In `fetch.go`: populate `fetchedData.expActivityGroups [][]string` by
      calling `GroupIDsBySimilarity` for the viewer's experience ids —
      **best-effort**: guard on `s.storage.GetEmbedder() != nil`, and on error
      log at most `WarnContext` and leave it empty (never fail the view, matching
      the weather/nudge best-effort pattern in `home_view.go`). If it warrants a
      timing line, fold it into the existing per-phase duration logging.
- [ ] Add `minActivityOccurrences = 2` to the portfolio tuning block
      (`home_suggestions.go:14`) with the #2674 rationale.
- [ ] Rework `viewerActivities` to emit one `activityStat` per group from
      `d.expActivityGroups` (count = group size, unioned crew, avg start time,
      representative = most-frequent/earliest exact name); **exact-name lexical
      fallback** when groups are empty. Keep it a pure function over
      `fetchedData`.
- [ ] In `assembleOpenDaySuggestions`, filter stats to
      `count >= minActivityOccurrences`; `return nil` when empty (reuse existing
      early return). Confirm lead selection and `alternativesExcept` both read
      the filtered list.
- [ ] Tests in `home_suggestions_test.go` (portfolio, synthetic groups — no
      model): a 2-experience group → suggested; two singleton groups → `nil`
      (core suppression guard); single-occurrence + valid weather → `nil`
      (extend `TestAssembleOpenDaySuggestionsNoHistoryOrWeather`); missing-group
      lexical fallback merges identical names; **fix
      `TestAssembleOpenDaySuggestionsSkipsBusyAndPast`** (its "Trail run" ×1
      fixture now yields nothing — give it a 2-experience group so it still
      exercises busy/past/typical skipping and now the ≥2 path).
- [ ] `npm run generate`, then
      `go test ./server/services/portfolio/... ./server/storage/...` locally
      (server tests run locally per repo policy — not "CI-only").

**Validates:** One data point (or varied one-offs) no longer triggers a
suggestion; two similar events do — proven by pure tests over synthetic vectors
and synthetic groups, no client or proto change.

> **Implementation status (all phases landed in one PR).** Implemented and
> tested locally: `embedding.CosineSimilarity` + `storage.GroupTextsBySimilarity`
> / `clusterByCosine` (pure unit tests + validation + a real-model test),
> `fetchAll` grouping over the viewer's involved experience names, the
> group-aware `viewerActivities` with exact-name fallback, the `≥2` filter, a
> real-model calibration test, and a strategy-comparison harness.
>
> **We embed the name, not the stored name+description vector — decided by
> experiment.** A labeled-dataset harness (`clustering_strategy_test.go`) compared
> {name-only, name+desc} × {raw, filler-stripped} on discriminativeness:
>
> | strategy | AUC | recall @ P≥0.98 |
> |---|---|---|
> | **name-only, raw** | **0.938** | **46.7%** |
> | name-only, filler-stripped | 0.913 | 40.7% |
> | name+desc, raw | 0.935 | 14.7% |
> | name+desc, filler-stripped | 0.928 | 7.3% |
>
> Findings: (1) **name-only beats name+desc** decisively on precision —
> descriptions add cross-kind noise; (2) **filler-word stripping hurts** (the
> fine-tuned model already handles context; stripping just shortens the string);
> (3) a **picky threshold of 0.83** yields ~100% precision (zero false merges) at
> ~46% recall — we'd rather miss a merge than make a wrong one. So
> `activityClusterMinSimilarity = 0.83` and clustering uses **name-only**
> embeddings.
>
> **Precomputed (was #2694, now done in this PR).** The name-only vector is
> **precomputed and stored**, not generated on the fly: the embedding subsystem
> now supports **multiple embedding variants per table** (a `Variant` +
> `SkipIndex` on `EmbeddingFieldConfig`, config slices per table, variant-suffixed
> columns). The `experience` table carries both the default `name+description`
> embedding (for search, unchanged) and a `name`-only variant
> (`ExperienceNameEmbeddingVariant`, index-skipped since it's only read by id).
> Both are generated async on insert/update (one `embeddingDone` signal per row)
> and backfilled at startup, so `GetHomeView` just reads stored vectors via
> `storage.GroupIDsByStoredEmbedding` — **zero hot-path generation**.
>
> **Known limitation:** generic venue-flavored names (e.g. "Community garden
> workday") embed toward a diffuse "community event" direction and score
> ~0.66–0.70 against *unrelated* activities — no threshold clusters them cleanly.
> Inherent to a gear/search-tuned model on free-form titles; the `≥2` gate +
> gentle empty-day UX keep an occasional mis-merge low-stakes, and the
> calibration fixture excludes such names.

### Phase 2: Real-model calibration fixture (test data → calibrate → regression guard)

**Goal:** Choose `ActivityClusterMinSimilarity` from *real* embeddings, and lock
the choice in as a test that fails if the model or threshold drifts — per the
requester's direction to "create test data to calibrate, add unit tests, and
catch regressions."

**Tasks:**
- [ ] **Author a curated fixture** — a table-driven set of representative
      experience *names* grouped by intended activity kind (e.g. hikes:
      {"Flatirons hike","Sunday hike","Morning trail hike"}; runs: {"Trail
      run","5k jog"}; dinners: {"Taco night","Neighborhood potluck"}; plus
      deliberate near-miss distractors and unrelated one-offs). This fixture is
      the single source of truth for calibration **and** regression.
- [ ] **Embedder-backed calibration/regression test** (new
      `home_suggestions_calibration_test.go` or in the storage/embedding
      package): load the real model via the existing `getModelPaths()` +
      `t.Skip("Model files not found")` pattern (`embedding_test.go`), embed the
      fixture names, and assert that at the chosen `ActivityClusterMinSimilarity`
      **every intended same-kind pair scores ≥ threshold and every cross-kind
      pair scores < threshold**. Report the min same-kind / max cross-kind
      margin so a future model retrain that narrows the gap fails loudly.
      **Actually run it locally** (model ships via Git LFS at
      `model_tuning/ripls_embedding.onnx`; CI Docker bundles it) — do not treat
      the skip as "verified" (self-skip ≠ verified).
- [ ] Pick the threshold that maximizes that margin; if no single value cleanly
      separates the fixture, that is the signal to switch to **Option B**
      (name-only vectors) — decide here, record the rationale + chosen value in
      this doc.
- [ ] (Optional) `ripls-ui-investigate` / throwaway e2e probe: seed (a) one past
      experience → assert no `CalendarSuggestionPanel`; (b) two near-duplicate
      experiences → assert the panel appears; (c) two unrelated → assert no panel.

**Validates:** The threshold provably separates same-kind from different-kind on
real embeddings, and the fixture guards against silent regressions from model or
threshold changes.

## Related Issues
- #2514 — introduced the open-day suggestion engine + weather substrate this
  change tunes (calendar month × backdrop redesign).
- #2646 — GetHomeView latency work touching the same assembly path; the reason
  the one added by-id SELECT (and zero hot-path embedding generation) is called
  out explicitly as within budget.

## Audit Notes

Audited against the documentation context map. **Changes applied:**

1. **Storage-Layer Encapsulation (architecture — was a real violation).** The
   first draft had the portfolio service pull raw `[]float32` vectors
   (`GetEmbeddingVectorsByIDs`) and run cosine math itself. `semantic_search.md`
   mandates that *all embedding logic lives in the storage layer*. Reworked
   Option A + Phase 1 to expose a storage method `GroupIDsBySimilarity(...)
   → [][]string`; the service now consumes id-groups and never handles a vector.
2. **Threshold grounding (correctness/tuning).** Replaced the invented
   `activitySimilarityThreshold ≈ 0.6` with a dedicated
   `ActivityClusterMinSimilarity` constant sited beside the existing
   `SemanticSearchMinSimilarity = 0.5`, grounded in the model's documented
   similarity bands, with an explicit note that clustering needs precision (so
   it should not blindly reuse the search recall cutoff).
3. **Graceful Degradation (convention).** Framed the un-embedded lexical
   fallback as the documented semantic→text fallback pattern; the storage method
   returns NULL-vector ids as singletons so the caller degrades cleanly.
4. **Observability / hot-path (server conventions, #2646).** Made the fetch
   best-effort (`WarnContext`, never fail the view, guard on `GetEmbedder()`),
   consistent with the weather/nudge pattern in `home_view.go`, and folded any
   timing into the existing per-phase duration log rather than a new hot loop.
5. **Testing (testing-review + requester direction).** Split tests into (a) pure
   synthetic-vector clusterer unit tests, (b) pure synthetic-group assembler
   suppression tests, and (c) a real-model, fixture-driven calibration/regression
   test using the established `getModelPaths()`/`t.Skip` harness — and required
   it to actually run locally (model ships via Git LFS), since a self-skip is not
   verification.
6. **Layering of constants (code quality).** `minActivityOccurrences` stays a
   portfolio-policy constant; `ActivityClusterMinSimilarity` is an embedding
   constant in storage — each in its rightful package.

**No violations found in:** proto/API design (no proto or RPC change — the
existing `open_day_suggestions` contract is unchanged); client boundary (server
renders `title`/`reason` verbatim as today); dependencies (reuses the existing
ONNX embedder + pgvector, adds none); security/PII (operates on the viewer's own
experiences already loaded for the view).

**Note (prior art not chosen):** the codebase has category signals
(`server/category/`, `known_for.Derive`) that could group activities without
embeddings. Not pursued because the requester explicitly directed reuse of the
search embedding; recorded here so a reviewer can reconsider if embedding
clustering proves noisy.

**Docs consulted:** `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md`, `server/observability.md`, `testing/architecture.md`,
`proto_conventions.md`; leaf docs `server/semantic_search.md` (decisive),
`server/portfolio.md`, `client/calendar.md`, `weather.md`; review prompts
`reviews/performance_review_prompt.md`, `reviews/testing_review_prompt.md`,
`reviews/observability_review_prompt.md`, `reviews/architecture_review_prompt.md`.
