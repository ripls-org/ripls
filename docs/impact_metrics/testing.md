---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Test strategy for the completion-time impact draft and per-attribute override flow — collapses the experience/request × metric × override × state permutation space into load-bearing invariants, assigning each to exactly one test layer.
  globs: [server/impact_metrics/**, server/services/experience/**, server/services/request/**, app/lib/presentation/screens/impact_metrics/**, app/lib/presentation/widgets/impact/**]
  triggers: [impact-testing, draft-override, provenance-stamping, invariant, equivalence-class, completion-impact]
  lens: [domain, testing]
  domain: impact
freshness:
  verified_commit: "c071b8768"
  verified_on: "2026-07-31"
---
# Testing the Completion-Time Impact Draft & Override Flow

**Scope:** The draft-at-completion + per-attribute override UX for experiences and requests
introduced by [#1249](../issues/1249-completion-impact-draft.md). Gear loans / giveaways
are out of scope here — their single-LLM-call-at-completion path is unchanged and tested
by the existing loan / giveaway suites.

**Goal of this doc:** make the permutation space tractable. The feature crosses four axes
(transaction type × metric × override scenario × transaction state), and a naïve
"write a test for each combination" produces hundreds of tests that nobody will maintain.
This doc pins down the equivalence classes, assigns each class to exactly one test layer,
and explains what the manual QA pass has to check *on top* of the automated suite.

## 1. Why the permutation space is large

The draft + override flow intersects several independent axes:

| Axis | Values | Count |
|---|---|---|
| Target kind | experience, request | 2 |
| Metric | quality_time, money_saved, prevented_emissions | 3 |
| QT attribute | duration, modality, group_size, tie_strength, reciprocity, novelty, vulnerability | 7 |
| Money inputs | hire_equivalent + N per-contribution | N+1 |
| CO2 inputs | per-contribution (embodied + waste) + travel_avoided + repair_credit | ≥3 |
| Provenance source | LLM, USER | 2 |
| Transaction state | draft, scheduled, in-progress, completed/fulfilled, cancelled | 5 |
| Caller role | host/creator, confirmed attendee/helper, stranger, unauthenticated | 4 |
| Override scope | none, single input, all inputs in one metric, inputs across all three metrics | 4 |

Multiplied out this is thousands of cases. We collapse it as follows.

## 2. Collapsing the space: invariants and equivalence classes

Nearly every test case reduces to checking one of a **small set of invariants**. Writing
tests per-invariant (property-style) rather than per-permutation is what keeps the suite
maintainable.

### 2.1 Load-bearing invariants

1. **INV-COMPOSITE-DERIVED.** The composite fields (`money_saved.value_usd`,
   `manufacture_avoided_carbon` + `waste_reduced_carbon`,
   `quality_time_estimate.quality_time_minutes`) are **always** server-computed from the
   inputs. No RPC accepts a caller-supplied composite; any attempt is rejected.
2. **INV-PROVENANCE-PER-INPUT.** Every input field has its own `Provenance`. An
   override to one input stamps `source=USER` on that input only and leaves sibling
   inputs' provenance untouched.
3. **INV-REDRAFT-SKIPS-LLM.** `DraftImpactEstimateWithOverrides` never calls the LLM.
   Latency target: sub-100ms server-side. Overrides survive a round-trip.
4. **INV-AUTH-GATED.** Only host/creator or confirmed attendees/helpers can draft or
   redraft. Strangers get `CodePermissionDenied`. Unauthenticated get `CodeUnauthenticated`.
5. **INV-STATE-GATED-DISPLAY.** Client renders impact tiles only when
   `state ∈ {COMPLETED, FULFILLED}` **and** `ImpactEstimate` is non-null. Any other
   state hides the tiles entirely.
6. **INV-GEAR-UNAFFECTED.** Loan and giveaway flows are unchanged. Neither the draft
   RPC nor the completion-modal override UX is reachable from gear screens.
7. **INV-REFRESH-RESPECTS-USER.** `ShouldRefresh()` on stored `ImpactEstimate` skips
   inputs with `source=USER`. Bumping the algorithm version re-drafts LLM-sourced inputs
   only; user overrides are sticky.
8. **INV-CACHE-INVALIDATED.** After `CompleteExperience` / `MarkRequestFulfilled`,
   community-level and user-level impact aggregations are invalidated via
   `dailyCacheInvalidationProvider.notify()` and `ImpactMetricsRepository.invalidate*()`.
9. **INV-DRAFT-NOT-CACHED.** `draftImpact()` / `redraftImpact()` bypass
   `CacheService.get()`. Opening the modal after a chat edit sees fresh data.
10. **INV-LLM-PRIVACY.** The prompt to `aiProvider.InferSocialAttributes()` carries
    user IDs and aggregated counts — never names, emails, or other PII.

Each invariant maps to **one canonical test layer** (§3). That's the discipline: don't
re-test an invariant everywhere it happens to be visible.

### 2.2 Equivalence classes on each axis

- **Target kind.** Experience and request take the same draft shape but differ in
  input wiring (`PlanningNeed` visibility, RSVP vs Helper records, creator vs host).
  Run the full matrix for one target and a **smoke subset** for the other.
  Recommendation: full matrix on `experience`; smoke on `request`.
- **Metric.** QT, money, CO2 share the provenance-stamping mechanism but have distinct
  estimators. Test estimator math per-metric in
  [server/impact_metrics/estimator/](../../server/impact_metrics/estimator/); test the
  *stamping* once in the builder.
- **QT attributes.** The 7 attributes share identical provenance plumbing. Unit-test
  stamping against **one** attribute (e.g. `group_size`) and use a property-style
  table-driven test across all 7 to guard against drift.
- **State gate.** 5 states but 2 equivalence classes: `∈ {COMPLETED, FULFILLED}` (show)
  vs everything else (hide). One parameterised widget test with both classes.
- **Auth.** 3 classes: authorised (host or confirmed), authenticated but unauthorised
  (stranger), unauthenticated. One test per class per RPC.

## 3. Automated test layers

The full suite is organised into layers, each responsible for a disjoint set of
invariants. An invariant should have **exactly one** canonical test location.

### 3.1 Proto wire-format checks (`proto/`)

**Responsibility:** INV-COMPOSITE-DERIVED structural half (the shape).

- Existing `go test ./proto/...` and `buf lint` runs catch proto hygiene;
  `npm run lint:proto:breaking` catches wire breaks — a field whose type or
  cardinality changed in place, or one deleted without reserving its number.
  Renames are *not* caught: the gate runs at `WIRE`, where field numbers are the
  wire identity and names are not. See `docs/proto_conventions.md`
  → [Deprecating and removing fields](../proto_conventions.md#deprecating-and-removing-fields).
- Add a static assertion test that every `*Input` proto message has a `Provenance`
  field and that composite fields (`value_usd`, `*_avoided_carbon`,
  `quality_time_minutes`) do **not** appear in the draft override request messages
  — if they did, a client could try to set them.
- Location: `server/impact_metrics/impact_estimate_schema_test.go`.
- Mechanism: reflect over `api.DraftImpactEstimateWithOverridesRequest` descriptors
  and fail the test if any composite field name is present.

### 3.2 Go estimator math (`server/impact_metrics/estimator/`)

**Responsibility:** money / CO2 / QT formula correctness per-input.

- Extend [savings_test.go](../../server/impact_metrics/estimator/savings_test.go)
  with table-driven cases for the contribution-aware path:
  - gear contribution with co-use discount at 0.30 and 0.40
  - mixed contribution types (gear + travel-avoided + hire-equivalent)
  - single contribution vs many (linearity check)
  - zero contributions → zero money, zero CO2 (no defaults leak in)
- Extend [carbon_test.go](../../server/impact_metrics/estimator/carbon_test.go) for
  per-contribution embodied + waste terms and travel-avoided.
- Property-style test: scaling all contribution counts by K should scale the
  composite by K (up to caps). Cheap regression guard.

### 3.3 Go builder provenance stamping (`server/impact_metrics/builder_overrides_test.go`)

**Responsibility:** INV-PROVENANCE-PER-INPUT, INV-COMPOSITE-DERIVED (the stamping
half), INV-REFRESH-RESPECTS-USER.

Tests against `ApplyImpactOverrides` (`server/impact_metrics/builder.go`), one per
concern:

- `TestApplyImpactOverrides_QT_OverrideDuration_StampsUserOnDurationOnly` — override
  duration; assert only that attribute's provenance flips to `USER`.
- `TestApplyImpactOverrides_QT_OverrideVulnerability_StampsUserOnVulnerabilityOnly` —
  same pattern for the vulnerability attribute.
- `TestApplyImpactOverrides_MoneySavings_StampsUserProvenance` — override money
  inputs; assert the top-level provenance flips to `USER`.
- `TestApplyImpactOverrides_Emissions_StampsUserProvenance` — same pattern for
  emissions inputs.
- `TestApplyImpactOverrides_QT_CompositeRecomputedNotCopied` — the composite
  `quality_time_minutes` is recomputed from the override's inputs, not copied from
  a caller-supplied composite value. (`applyMoneySavingsOverride` /
  `applyEmissionsOverride` in `builder.go` likewise only ever read `.GetInputs()` —
  a caller-supplied composite field is silently ignored/overwritten rather than
  rejected with an error, so there is no `CodeInvalidArgument` case to test here.)
- `TestShouldRefresh` (`server/impact_metrics/provenance_test.go`) — covers
  INV-REFRESH-RESPECTS-USER: a `USER`-sourced field is skipped on refresh, an
  `LLM`-sourced field past the staleness window is refreshed.

A parameterised helper iterates the 7 QT attributes so we catch any new attribute
added without a provenance field.

### 3.4 Go drafting service (`server/services/impact_metrics/drafting_test.go`) — **new file**

**Responsibility:** authorization, the RPC wiring, the LLM/estimator seam, and the
`DraftImpactEstimateWithOverrides` redraft path.

This test file does not exist yet — [drafting.go](../../server/services/impact_metrics/drafting.go)
currently has no `_test.go` sibling. Highest-priority gap.

Use a **fake AI provider** (not the real Gemini / Vertex client) that returns a
canned `InferSocialAttributes` response. Use `storage.SetupTestStorage(t)` for the
backing store (pgvector testcontainer per [CLAUDE.md](../../CLAUDE.md)).

Canonical cases:

- **Auth:**
  - `TestDraftImpactEstimate_HostCanDraft` (experience host)
  - `TestDraftImpactEstimate_ConfirmedAttendeeCanDraft`
  - `TestDraftImpactEstimate_StrangerDenied` → `CodePermissionDenied`
  - `TestDraftImpactEstimate_UnauthenticatedDenied` → `CodeUnauthenticated`
  - Requests: `TestDraftImpactEstimate_Request_CreatorCanDraft`,
    `TestDraftImpactEstimate_Request_StrangerDenied`,
    `TestDraftImpactEstimate_Request_UnauthenticatedDenied` — a confirmed-helper
    case (the request equivalent of `ConfirmedAttendeeCanDraft`) is not yet
    covered separately.
- **Input assembly:**
  - `TestDraftImpactEstimate_ScopesChatTranscriptToLimits` — seed a thread with
    60 messages; assert the LLM sees only the most recent 50 (or 4000-token budget).
  - INV-LLM-PRIVACY is covered as a secondary signal by
    `TestImpactDraftPrompt_GoldenRoundTrip` (§3.12), which asserts sender names
    and emails never appear in the rendered prompt; there is no dedicated
    `drafting_test.go` case for this.
- **Redraft semantics:**
  - `TestDraftImpactEstimateWithOverrides_DoesNotCallLLM` — fake AI provider
    tracks call count; override RPC invoked 10 times; call count stays 0.
  - `TestDraftImpactEstimateWithOverrides_RecomputesComposite` — override one
    money input; composite `value_usd` reflects the new input and nothing else
    shifts.
  - `TestDraftImpactEstimateWithOverrides_Unauthorized` — a stranger cannot use
    the override redraft path. (A caller-supplied composite field in the request
    is silently recomputed rather than rejected — see §3.3 — so there is no
    `CodeInvalidArgument` case to test here.)
- **Service independence:** construct the service with only `ProtoSQLStorage` and
  the fake AI provider (no chat/experience/request service injected). The test
  *cannot* compile if those dependencies are added back — the file acts as a
  regression guard for the architecture rule.

### 3.5 Go lifecycle tests (`server/services/experience/lifecycle_test.go`, `server/services/request/lifecycle_test.go`)

**Responsibility:** override pass-through on the real completion RPCs.

- `TestCompleteExperience_WithOverrides_PersistsPerInputProvenance`
- `TestMarkRequestFulfilled_WithOverrides_PersistsPerInputProvenance`
- `TestCompleteExperience_NoOverrides_UsesLLMDraft` — ensures the "all LLM, no
  user overrides" path still works and stamps `source=LLM` across the board.

### 3.6 Go integration tests (`server/integration_tests/`)

**Responsibility:** end-to-end round trip through Connect. Kept deliberately thin
— one happy-path per target, not a matrix.

- [experience_impact_test.go](../../server/integration_tests/experience_impact_test.go):
  `TestCompleteExperience_DraftThenOverrideThenCommit` — seed experience + chat +
  contributions + attendees; call `DraftImpactEstimate` → override one QT
  attribute and one money input → call `DraftImpactEstimateWithOverrides` →
  commit via `CompleteExperience` with the overrides → read the persisted
  `ImpactEstimate` and assert per-input provenance is correct.
- A parallel round trip for requests in
  [request_test.go](../../server/integration_tests/request_test.go) is not yet
  added — see the gap noted in §8.2.
- Gate under `//go:build integration` per [CLAUDE.md](../../CLAUDE.md) if any
  external AI call leaks through — the preferred approach is a fake AI provider
  so the test stays in the default suite.

### 3.7 Dart repository (`app/test/data/repositories/impact_repository_test.dart`) — **new file**

**Responsibility:** repository calls the service correctly and **does not cache**
draft results. INV-DRAFT-NOT-CACHED.

Pattern matches the existing `gear_repository_test.dart` / `experience_repository_test.dart`:
`@GenerateMocks` on `ImpactMetricsService` and `CacheService`, then:

- `draftExperienceImpact` delegates to the service and does not touch `CacheService.get`.
- `redraftImpact` likewise bypasses the cache.
- After a simulated `CompleteExperience`, `invalidateUserImpact()` and
  `invalidateCommunityImpact()` are called.

### 3.8 Dart ViewModel (`app/test/presentation/viewmodels/impact_draft_notifier_test.dart`) — **new file**

**Responsibility:** state machine of
[impact_draft_notifier.dart](../../app/lib/presentation/viewmodels/impact_draft_notifier.dart),
plus disposal safety.

This test file does not exist today — highest-priority client gap mirroring §3.4.

Core cases:

- `draftExperience` sets `isLoading=true`, then populates `draft` on success.
- `draftExperience` on service failure sets `errorMessage` and `isLoading=false`.
- `applyQualityTimeOverrides` sets `isRedrafting=true`, triggers `_redraft`, and
  merges the new draft on success.
- Concurrent override calls don't clobber each other's fields — applying money
  overrides then CO2 overrides results in state that carries both.
- **Disposal test** (per [client/architecture.md](../client/architecture.md)):
  start `draftExperience`, dispose the container mid-flight, assert no throw.
  Mirror the existing
  [disposal_safety_test.dart](../../app/test/presentation/viewmodels/disposal_safety_test.dart)
  pattern.

### 3.9 Dart widget tests (`app/test/presentation/widgets/impact/completion/`)

**Responsibility:** each detail modal renders, edits a single input, and triggers a
redraft with the correct override payload.

One widget test per modal, all following the same shape:

- `value_detail_modal_test.dart` — edit hire-equivalent rate; assert the repo
  mock was called with a `MoneySavings` override whose `hire_equivalent_value`
  carries the new number and whose sibling inputs are absent (or carry the
  original LLM value with `source=LLM`).
- `co2_detail_modal_test.dart` — edit travel-avoided distance; same shape.
- `quality_time_detail_modal_test.dart` — edit `group_size`; same shape.
- **Composite is read-only:** each modal test must assert there is no editable
  widget for the composite field (no `TextField` / `Slider` bound to `value_usd`,
  `quality_time_minutes`, etc.). This is the client-side guard on
  INV-COMPOSITE-DERIVED.

Each test wraps `MaterialApp` with the `localizationsDelegates` and `supportedLocales`
helper from `app/test/helpers/l10n_helpers.dart` per
[client/i18n.md](../client/i18n.md).

### 3.10 Dart screen gating (`app/test/presentation/screens/`)

**Responsibility:** INV-STATE-GATED-DISPLAY.

- One parameterised screen test that renders the experience detail / feed card /
  sharing card once per `ExperienceState` and asserts tiles are absent except in
  `COMPLETED`.
- Same for `RequestState` → only `FULFILLED` shows tiles.
- Loans / giveaways unchanged — a single "still shows tiles pre-return" test per
  target stands in for INV-GEAR-UNAFFECTED.

### 3.11 i18n lint

[i18n lint](../client/i18n.md) (`npm run lint:dart:i18n`) already fails CI on
missing strings. No new test work; just verify both
[app_en.arb](../../app/lib/l10n/app_en.arb) and
[app_es.arb](../../app/lib/l10n/app_es.arb) have every new `impact*` /
`metricDetail*` key before merge.

### 3.12 Prompt golden test

**Responsibility:** detect accidental prompt regressions (PII leakage, missing
transcript truncation, wrong attribute list).

- Location: `server/services/impact_metrics/prompt_golden_test.go`
  (`TestImpactDraftPrompt_GoldenRoundTrip`).
- Pattern: render the prompt against a seeded fixture (5 messages, 2 contributions,
  3 attendees, one location), diff against a checked-in golden file.
- Updates require `go test ... -update-golden` and a human review in the diff.
- This is the primary guard against "we changed the prompt and accidentally
  inlined `user.email`" — it catches the kind of regression no other layer sees.

## 4. Coverage matrix: invariants × layers

| Invariant | Canonical layer | Secondary signal |
|---|---|---|
| INV-COMPOSITE-DERIVED | §3.3 builder (composite silently recomputed, not caller-set), §3.9 widget (no composite edit UI) | §3.1 proto |
| INV-PROVENANCE-PER-INPUT | §3.3 builder | §3.5 lifecycle (persisted) |
| INV-REDRAFT-SKIPS-LLM | §3.4 drafting service | — |
| INV-AUTH-GATED | §3.4 drafting service | — |
| INV-STATE-GATED-DISPLAY | §3.10 screen gating | Manual §5 |
| INV-GEAR-UNAFFECTED | §3.10 (one smoke case) | Manual §5 |
| INV-REFRESH-RESPECTS-USER | §3.3 builder (`TestShouldRefresh`) | — |
| INV-CACHE-INVALIDATED | §3.7 repository | — |
| INV-DRAFT-NOT-CACHED | §3.7 repository | — |
| INV-LLM-PRIVACY | §3.12 prompt golden (`TestImpactDraftPrompt_GoldenRoundTrip`) | — (no dedicated §3.4 case; see §3.4) |

Every invariant has a canonical automated home. If a layer is red, we know which
invariant is at risk.

## 5. Manual testing script

This section is a step-by-step script that can be followed against a running device
or simulator. It covers three tiers:

- **§5.1 Smoke script** — run for every PR that touches this flow (~15 min).
- **§5.2 Edge-case script** — run pre-release (~30 min).
- **§5.3 Device matrix** — run before a release cut.

Each step names the exact UI element to interact with, the expected result, and the
invariant being guarded. Failure modes are noted where the risk is non-obvious.

---

### 5.1 Smoke script (every PR)

**Prerequisites:**
- A test account that is a host on at least one experience (ideally one with ≥3 chat
  messages, ≥2 confirmed attendees, and ≥1 gear contribution).
- The app running in debug or release mode on a device or simulator.

---

#### S1 — Pre-completion: tiles must not appear

1. Open the experience detail screen for an experience in `SCHEDULED` or
   `IN_PROGRESS` state.
2. Scroll through the entire screen.
3. **Expect:** No "SAVED", "CO₂", or "QUALITY TIME" tiles appear anywhere on the
   screen. *(INV-STATE-GATED-DISPLAY)*

---

#### S2 — Open the completion modal

1. From the experience detail, tap the menu (⋯) → **"Mark Completed"**.
2. The `MarkCompletedModal` bottom sheet opens (dark glass surface, ~92% height).
3. **Expect:** At the top of the sheet, the experience name appears under a "COMPLETED"
   eyebrow label.
4. **Expect:** An impact bar with three tiles is visible below the header — "SAVED",
   "CO₂", and "QUALITY TIME". All three show a short loading state ("Estimating your
   impact…") then resolve to numbers within a few seconds.
5. **Expect:** While loading, the **Complete** button (coral, bottom of sheet) is
   disabled and shows a spinner with "Crafting your story…". After loading it becomes
   active.
6. **Failure mode to watch:** If the bar never resolves (spinner runs >10s), the
   `draftExperience` call failed — check network and server logs.

---

#### S3 — Quality Time detail modal

1. Tap the **"QUALITY TIME"** tile in the impact bar.
2. The `QualityTimeDetailModal` sheet opens with title **"Quality time breakdown"**.
3. **Expect (composite row):** A row labelled with the formatted time (e.g., "1h 30m")
   in orange (`#F4A97D`) with sub-text **"Computed from inputs below"**. This row has
   no text field or slider — it is read-only. *(INV-COMPOSITE-DERIVED)*
4. **Expect (editable rows):** Two `TextField` widgets are visible:
   - **"Duration (minutes)"** — pre-populated with a number (e.g., `90`).
   - **"Group size"** — pre-populated with a number (e.g., `4`).
5. **Expect (vulnerability picker):** A `SegmentedButton` with three segments:
   **Low**, **Medium**, **High**. One segment is selected.
6. **Override badge test:** Clear the **"Duration (minutes)"** field and type `75`.
   **Expect:** An orange **"Override"** badge appears next to the "Duration (minutes)"
   label, indicating the value has diverged from the LLM suggestion.
7. Tap **Save**.
8. **Expect:** The sheet shows a spinner briefly (redraft in progress), then closes.
9. **Expect:** The "QUALITY TIME" tile in the impact bar updates to reflect the new
   duration. *(INV-REDRAFT-SKIPS-LLM)*
10. Tap **"QUALITY TIME"** again to re-open.
11. **Expect:** The "Duration (minutes)" field shows `75` — the override persisted.
    The "Override" badge is still visible. *(INV-PROVENANCE-PER-INPUT)*

---

#### S4 — Savings detail modal

1. Tap the **"SAVED"** tile.
2. The `ValueDetailModal` sheet opens with title **"Savings breakdown"**.
3. **Expect (composite row):** A row showing total savings (e.g., `$120`) in green
   (`#7FB87E`) with sub-text **"Computed from inputs below"**. No text field on this
   row. *(INV-COMPOSITE-DERIVED)*
4. **Expect (editable rows):** At minimum one `TextField` labelled **"Service value"**
   pre-populated with a dollar amount (e.g., `120.00`).
5. Clear **"Service value"** and enter `200.00`. Tap **Save**.
6. **Expect:** Sheet closes, "SAVED" tile updates (green number increases).
7. Re-open the sheet: **Expect** "Service value" shows `200.00`.

---

#### S5 — CO₂ detail modal

1. Tap the **"CO₂"** tile.
2. The `CO2DetailModal` sheet opens with title **"CO₂ breakdown"**.
3. **Expect (composite row):** A row showing total CO₂ (e.g., `5.00 kg`) in blue
   (`#7DB8D4`) with sub-text **"Computed from inputs below"**. No text field. *(INV-COMPOSITE-DERIVED)*
4. **Expect (editable rows):** Two `TextField` widgets:
   - **"Travel avoided"** — value in kg CO₂e (e.g., `5.00`).
   - **"Repair credit"** — value in g CO₂e (e.g., `250`).
5. Change **"Travel avoided"** to `8.00`. Tap **Save**.
6. **Expect:** Sheet closes, "CO₂" tile updates.
7. Re-open: **Expect** "Travel avoided" shows `8.00`.

---

#### S6 — Commit completion

1. With overrides applied from S3–S5, tap **Complete** (coral button, bottom of sheet).
2. **Expect:** Modal dismisses. Experience transitions to `COMPLETED` state.
3. **Expect:** Impact tiles ("SAVED", "CO₂", "QUALITY TIME") now appear on the
   experience detail screen — visible without opening any modal. *(INV-STATE-GATED-DISPLAY)*
4. **Expect:** Community-level impact numbers (if visible on a community screen) have
   updated within 1–2 refreshes. *(INV-CACHE-INVALIDATED)*

---

#### S7 — Request happy path (smoke only)

Repeat S2–S6 for a request that the test account created, using
**"Mark Fulfilled"** instead of "Mark Completed". Verify the same three tiles appear
and overrides persist.

---

#### S8 — Gear loan unaffected (INV-GEAR-UNAFFECTED)

1. Open a gear loan's completion flow (if reachable from the test account).
2. **Expect:** The completion screen does **not** show the three-tile impact bar or
   any draft/override UX. The flow is unchanged from the pre-#1249 behaviour.

---

### 5.2 Edge-case script (pre-release)

Run these after S1–S8 pass. Each scenario probes a specific risk the smoke script
doesn't cover.

---

#### E1 — Zero-contribution experience

**Setup:** Use an experience with no gear contributions and ≤1 attendee.

1. Open the completion modal (S2).
2. **Expect:** All three tiles load (no crash/blank).
3. **Expect (CO₂ breakdown):** "Travel avoided" and "Repair credit" fields are either
   `0.00` or empty — no phantom default (e.g., `5000.00`) should appear.
4. **Expect (Savings breakdown):** "Service value" shows a sensible baseline, not `0`.
5. **Expect (Quality time breakdown):** "Duration (minutes)" and "Group size" both
   have non-zero defaults.

---

#### E2 — Reset to suggestion

**Setup:** Open any detail modal and make an override (as in S3 step 6).

1. After saving, re-open the same modal.
2. Tap **"Reset to suggestion"** (appears near the overridden field).
3. **Expect:** The field reverts to the original LLM value. The "Override" badge
   disappears. *(INV-PROVENANCE-PER-INPUT — USER stamp should be removed)*
4. Tap **Save**.
5. **Expect:** The tile updates to the LLM value.

---

#### E3 — Multi-metric overrides accumulate

1. Override a field in the Quality Time modal (S3) — tap Save.
2. Override a field in the Savings modal (S4) — tap Save.
3. Override a field in the CO₂ modal (S5) — tap Save.
4. Re-open each modal in turn.
5. **Expect:** All three overrides are still in place. No override was lost when a
   second or third one was applied. *(INV-PROVENANCE-PER-INPUT)*

---

#### E4 — Abandon mid-draft (discard behaviour)

1. Open the completion modal (S2) — wait for tiles to load.
2. Tap the Quality Time tile, change "Duration (minutes)" to `999`. Do **not** tap
   Save — tap the sheet's drag handle or swipe down to close the detail modal.
3. **Expect:** The override is discarded. Tile shows original value.
4. Now open the modal again via S2, make a change, tap Save, then close the
   *completion* modal (not the detail modal) by swiping it away entirely.
5. Re-open the completion modal.
6. **Expect:** A fresh draft is requested; prior unsaved session overrides are gone.

---

#### E5 — Rapid Save taps (debounce guard)

1. Open a detail modal (e.g., CO₂).
2. Tap **Save** 5 times in rapid succession.
3. **Expect:** Only one redraft RPC fires (the Save button should disable while
   `isRedrafting` is true). No duplicate mutations reach the server.

---

#### E6 — No-network draft

1. Put the device in Airplane mode.
2. Open an experience that has not yet been drafted (no cached estimate).
3. Open the completion modal.
4. **Expect:** The impact bar shows a loading state, then a friendly error message
   ("Could not estimate impact") — no crash, no blank tiles with no label.
5. Restore network. Tap retry (if available) or close and reopen the modal.
6. **Expect:** Draft loads successfully.

---

#### E7 — Bilingual sanity check (Spanish)

1. Switch device language to Spanish.
2. Open the completion modal.
3. **Expect:** All three tile labels, all detail modal titles, all field labels,
   and all button labels render in Spanish with no `MISSING_TRANSLATION` fallbacks.
4. Pay attention to: tile labels (short, space-constrained), the vulnerability
   segment labels (Low/Medium/High), and the "Computed from inputs below" sub-text
   under each composite row.

---

#### E8 — Narrow width (iPhone SE 3rd gen or equivalent)

1. Run on the smallest supported width (~375pt logical).
2. Open the completion modal.
3. **Expect:** The three-tile impact bar fits without overflow (no RenderFlex errors
   in the console, no clipped labels).
4. Open each detail modal.
5. **Expect:** The composite row, editable fields, and Save button all lay out
   correctly — no bottom overflow, keyboard does not cover the Save button.

---

#### E9 — Dark mode

1. Switch device to Dark mode.
2. Open the completion modal and all three detail modals.
3. **Expect:** No hardcoded `Colors.white` or `Colors.black` surfaces are jarring.
   The glass-border / container-fill palette should render consistently in dark
   theme. The orange, green, and blue metric colours are still readable.

---

### 5.3 Device / platform coverage matrix

Run S1–S8 (smoke) on each combination before a release cut:

| Device | OS | Light | Dark |
|---|---|---|---|
| iPhone 15 (or simulator) | Latest iOS | [ ] | [ ] |
| iPhone SE 3rd gen (or simulator) | Latest iOS | [ ] | [ ] |
| Pixel 8 (or emulator, API 34) | Android 14 | [ ] | [ ] |

Check E7 (bilingual) on at least one device per platform.

## 6. LLM quality vs LLM correctness

The automated suite **does not** assess whether the LLM's drafts are *good*. It only
checks that prompts are well-formed and responses plumb through correctly. Drift in
LLM quality is a different problem:

- **Calibration check** (Phase 4 of the plan): track override rate per attribute
  with a periodic query run as the read-only database role
  (`scripts/readonly-db-user.sql`). There is no dashboard tool — Metabase was
  removed (#3075). High override rate on one attribute → prompt refinement task.
- **Golden-set eval** (recommended follow-up): curate ~20 sample experiences + ideal
  impact values; run the draft RPC against them in an off-CI nightly job; track
  drift. Not a CI gate — a signal.
- **Do not** write unit tests that pin specific LLM outputs. The model changes; the
  test breaks; people start disabling it. Test the shape, not the content.

## 7. Running the suites locally

```bash
# Server — fast unit + service layer, no external AI
npm run generate && go test ./server/...

# Server — including integration tests (testcontainers; needs Docker)
go test -tags=integration ./server/...

# Server — build + lint (non-negotiable per CLAUDE.md)
npm run build

# Client — all tests, expanded reporter per CLAUDE.md
npm run test:app

# Client — just the draft/override surface
cd app && flutter test --reporter expanded \
  test/data/repositories/impact_repository_test.dart \
  test/presentation/viewmodels/impact_draft_notifier_test.dart \
  test/presentation/widgets/impact/completion/

# i18n lint
npm run lint:dart:i18n

# Full pre-merge check
npm run build && npm run test:app && npm run lint:dart:i18n
```

## 8. Gap summary vs current repo

### 8.1 Implemented

All automated test coverage described in §3 has been implemented. Files created or
extended as part of the #1249 test pass:

| Layer | File | Status |
|---|---|---|
| §3.2 Estimator | `server/impact_metrics/estimator/savings_test.go` extended | Done |
| §3.3 Builder | `server/impact_metrics/builder_overrides_test.go` (new), `provenance_test.go` | Done |
| §3.4 Drafting service | `server/services/impact_metrics/drafting_test.go` | Done |
| §3.5 Lifecycle | `server/services/experience/lifecycle_test.go` extended | Done |
| §3.5 Lifecycle | `server/services/request/lifecycle_test.go` extended | Done |
| §3.6 Integration | `server/integration_tests/experience_impact_test.go` (new) | Done |
| §3.7 Dart repository | `app/test/data/repositories/impact_repository_test.dart` | Done |
| §3.8 Dart ViewModel | `app/test/presentation/viewmodels/impact_draft_notifier_test.dart` | Done |
| §3.9 Widget — QT | `app/test/presentation/widgets/impact/completion/quality_time_detail_modal_test.dart` | Done |
| §3.9 Widget — Value | `app/test/presentation/widgets/impact/completion/value_detail_modal_test.dart` | Done |
| §3.9 Widget — CO₂ | `app/test/presentation/widgets/impact/completion/co2_detail_modal_test.dart` | Done |
| §3.10 Screen gating | `app/test/presentation/widgets/content/content_metric_sheet_gating_test.dart` | Done |
| §3.12 Prompt | `server/services/impact_metrics/prompt_golden_test.go` | Done |

### 8.2 Remaining gaps

No automated gaps remain against the §3 plan. The only items deferred from the
original design are lower-priority expansions:

- **Estimator linearity / scaling property test** (§3.2) — scaling all contribution
  counts by K should scale the composite by K (up to caps). Not yet added; the
  existing contribution tests cover correctness but not linearity.
- **Integration round-trip for requests** (§3.6) — `TestCompleteExperience_DraftThenOverrideThenCommit`
  covers the experience path. A parallel test for `MarkRequestFulfilled` would
  add coverage but the experience test exercises the same server-side code path.

## 9. Keeping this doc honest

This doc assumes the invariants in §2.1 and the file layout in §3. When either
changes — e.g. a new metric is added, or the override UX grows to cover gear loans
per the follow-up issue — update the invariants list first, then re-map each
invariant to its canonical layer. The coverage matrix in §4 is the artifact that
shows whether the suite is still in sync with the design.
