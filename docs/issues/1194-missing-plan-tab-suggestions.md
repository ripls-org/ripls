# Issue #1194: Auto-generate Plan tab suggestions for experiences that are missing them

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1194
**Filed:** 2026-04-20 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/experience_needs.md

## Problem Statement

The Plan tab's empty state shows suggestion chips drawn from `Experience.suggestions`
and `Experience.category_hint` (proto fields 24 and 25 on `models.Experience`). These are
populated by `aiProvider.GenerateExperienceSuggestions` at creation time
([save.go:152–174](server/services/experience/save.go#L152)). For any experience where
generation was skipped or failed — either because the experience pre-dates the feature or
because the LLM call errored — `suggestions` is an empty `repeated string`, and the Plan
tab renders with only the "I'll Bring / We Need" toggle and a blank chip area.

The flow today:
1. `GetExperience` → `buildAPIExperience` maps `exp.Suggestions` onto the API response.
2. `experience_content_view.dart:1401` reads `exp.suggestions` and passes it to
   `ExperienceNeedsSection`.
3. `ExperienceNeedsSection._EmptyStateSuggestions` renders chips from that list — or
   nothing if the list is empty.

There is no retry or backfill path. Once an experience misses suggestion generation it
stays chipless indefinitely.

## Context from Comments

No comments — issue was filed on 2026-04-20 and has had no discussion yet. The issue
body is well-specified with clear acceptance criteria, so the open questions below are
the main thing to resolve before implementation.

## Potential Solutions

### Option A: Lazy synchronous generation in `ListExperienceNeedsAndContributions`

Extend `ListExperienceNeedsAndContributionsResponse` with `suggestions` and
`category_hint` fields. In the handler, when `exp.Suggestions` is empty, call
`GenerateExperienceSuggestions` synchronously, persist the result via `storage.Update`,
and return the newly generated suggestions in the response. Client reads suggestions
from this response instead of from `exp.suggestions`.

- **Approach:** Proto change (two fields on the response message), synchronous LLM call
  gated on the same conditions as creation-time generation, client-side consumption of
  the new fields.
- **Pros:** Chips appear on the very first Plan tab open. No async coordination, no
  polling, no second load needed. Keeps suggestion data co-located with the Needs
  response so the Plan tab is self-sufficient.
- **Cons:** Adds ~500ms–2s LLM latency to the first `ListExperienceNeedsAndContributions`
  call for affected experiences. Requires a proto change and client update to consume new
  fields.
- **Effort:** Medium.

### Option B: Async fire-and-forget in `ListExperienceNeedsAndContributions`

Same proto extension as Option A, but generation fires in a background goroutine.
The first response returns the empty suggestions immediately; subsequent calls (after the
goroutine persists the result) return chips.

- **Approach:** Goroutine spawned from the RPC handler using a detached context (so the
  goroutine outlives the request). First render chipless; chips appear after the next
  Needs list refresh.
- **Pros:** Zero added latency to the first render. Matches the "or shortly after" phrasing
  in the issue acceptance criteria.
- **Cons:** First render is still chipless — the UX improvement only kicks in on the next
  force-refresh (after a mutation or explicit pull-to-refresh). Goroutines outliving RPC
  context require careful handling (detached context, panic recovery) and make the code
  harder to test.
- **Effort:** Medium (similar code, trickier lifecycle).

### Option C: Lazy backfill in `GetExperience` + client-side retry

Instead of touching the Needs handler, trigger async generation from `GetExperience`
when `exp.Suggestions` is empty. Client already reads suggestions from `exp.suggestions`.
The background goroutine persists the result; on the user's next navigation to the
experience (or after an explicit refresh) the chips appear.

- **Approach:** No proto change needed. Goroutine in `GetExperience`, same conditions as
  creation-time generation.
- **Pros:** Minimal diff — one short goroutine block in an existing function. No proto or
  client changes.
- **Cons:** Chips never appear on the first Plan tab open because `ExperienceNeedsSection`
  receives suggestions at construction time from the already-loaded Experience state.
  A subsequent manual refresh is required, which most users will not do.
- **Effort:** Small, but weaker UX than Options A or B.

## Recommendation

**Option A** (synchronous generation in `ListExperienceNeedsAndContributions`).

The current chipless state is the worst-case empty state. Spending ~1–2s on a one-time
LLM call for affected experiences is a better tradeoff than showing chips only after a
second interaction. Once suggestions are persisted (after the first successful load),
all subsequent calls are instant. The added latency is bounded by the LLM timeout and
falls within the normal acceptable range for the Plan tab load.

The proto extension (`suggestions` + `category_hint` on the Needs response) is also
architecturally sound: it makes the Plan tab self-sufficient and removes the implicit
dependency on the parent Experience being separately fetched. This avoids a subtle race
where `GetExperience` and `ListExperienceNeedsAndContributions` disagree on chip content.

## Open Questions

- **Should `ExperienceNeedsSection` stop reading suggestions from `exp.suggestions` and
  read exclusively from the Needs response?**  
  Recommendation: Yes — remove the `suggestions` prop from `ExperienceNeedsSection`
  and let it pull from the Needs viewmodel. This makes the Plan tab fully self-contained
  and eliminates the dual-source inconsistency. The Experience-level `suggestions` field
  is still useful for other consumers (e.g., preview cards that want to surface a
  category hint), so it stays on the model.

- **Should we add a time limit / deadline on the in-handler LLM call to bound worst-case
  latency?**  
  Recommendation: Yes — use `context.WithTimeout` of ~5s around the generation call.
  On timeout, log a warning and return empty suggestions. This prevents a slow LLM from
  blocking the Plan tab indefinitely.

- **Should generation also be triggered on update (`UpdateExperience`) when name or
  description changes and `suggestions` are non-empty?**  
  Recommendation: Out of scope for this issue. That is a "stale suggestions" problem,
  not a "missing suggestions" problem. Address separately if needed.

- **Should terminal experiences (COMPLETED or CANCELLED) get suggestions backfilled?**  
  Recommendation: No — the Plan tab is read-only for terminal experiences and the empty
  state with chips still prompts for actions (Request / Offer) that are disabled.
  Skip generation for terminal experiences to avoid unnecessary LLM calls.

## Implementation Plan

### Phase 1: Server — extend Needs response and add lazy generation

**Goal:** Ensure `ListExperienceNeedsAndContributions` always returns populated suggestions
for experiences that have them and generates+persists them for those that don't.

**Tasks:**
- [ ] Add `repeated string suggestions = 3` and `optional string category_hint = 4` to
  `ListExperienceNeedsAndContributionsResponse` in
  `proto/ripls/api/experience_service.proto`.
- [ ] Run `npm run generate` to regenerate Go and Dart code.
- [ ] In `needs_and_contributions.go`: after fetching the experience, populate the
  response from `exp.Suggestions` / `exp.CategoryHint`.
- [ ] Add the lazy-generation block: when `len(exp.Suggestions) == 0` and the experience
  is non-terminal and `s.aiProvider != nil` and `!clock.IsSimulated(ctx)` and name or
  description is non-empty — call `GenerateExperienceSuggestions` with a 5s timeout
  (`context.WithTimeout`), persist via `storage.Update` on success, log a `WarnContext`
  on failure and continue returning the response with empty suggestions. This is a
  documented deviation from the "Fail Fast" principle: LLM suggestion generation is
  best-effort (the same pattern is used at creation time in `save.go` lines 154–174);
  a chipless Plan tab is preferable to a failed RPC. Add a comment in the code explaining
  this justification.
- [ ] Run `npm run build` to verify the server compiles and passes lint.

**Validates:** Running `ListExperienceNeedsAndContributions` against an existing
chipless experience returns populated suggestions and persists them. Confirmed via a new
test (Phase 2).

### Phase 2: Server — test coverage

**Goal:** Cover the trigger path and the idempotency guarantee.

**Tasks:**
- [ ] In `needs_and_contributions_test.go`: add test verifying that calling
  `ListExperienceNeedsAndContributions` on an experience with empty suggestions triggers
  generation and returns chips in the response (mock AI provider returning fixed chips).
- [ ] Add test verifying that calling the same RPC on an experience that already has
  suggestions does NOT call `GenerateExperienceSuggestions` (verify mock call count is 0).
- [ ] Add test verifying that a terminal experience skips generation even when suggestions
  are empty.
- [ ] Add test verifying that an LLM failure results in an empty suggestions list but a
  successful response (no error propagation).
- [ ] Run `go test ./server/services/experience/...` to confirm all pass.

**Validates:** All acceptance-criteria scenarios are covered before client work begins.

### Phase 3: Client — consume suggestions from Needs response

**Goal:** Plan tab renders chips from the Needs list response, eliminating the
dual-source inconsistency.

**Tasks:**
- [ ] In `ExperienceRepository.listNeedsAndContributions` (and its `refreshNeedsAndContributions`
  counterpart), map the new `suggestions` and `category_hint` fields from the proto response
  onto the `ListExperienceNeedsAndContributionsResponse` Dart wrapper that the repository
  already returns. This keeps the data-access path in the repository layer, not the viewmodel.
- [ ] Add `suggestions` (`@Default([]) List<String>`) and `categoryHint` (`String?`) fields
  to `ExperienceNeedsState` (the Freezed state class in `experience_needs_view_model.dart`).
  Update `_load` and `_refreshAfterMutation` to populate these fields from the repository
  response via `state.copyWith(suggestions: response.suggestions, categoryHint: response.categoryHint)`.
- [ ] Update `ExperienceNeedsSection` to read `suggestions` and `categoryHint` from the
  Needs viewmodel state rather than via constructor props.
- [ ] Remove the `suggestions` and `categoryHint` props from `ExperienceNeedsSection`'s
  constructor; update `experience_content_view.dart` accordingly.
- [ ] Update `experience_needs_sheets.dart` if it also reads the props (it currently
  receives them — verify whether it should pull from the viewmodel too).
- [ ] Any new static user-visible strings added to the Plan tab UI (labels, button text,
  empty-state copy) must be added to `app/lib/l10n/app_en.arb` and `app_es.arb` before
  running `npm run generate`. Do not hardcode strings in Dart widget code.
- [ ] Run `npm run test:app` to confirm Flutter tests pass. Note: `npm run test:app` should
  invoke `flutter test --reporter expanded`; if running Flutter tests directly, always pass
  `--reporter expanded` to avoid garbled output.

**Validates:** Plan tab shows suggestion chips on first open for an experience that
previously had none, without any user-initiated refresh.

## Related Issues

- #1186 — Experience collaborative lists (original implementation where the Plan tab
  and suggestion chips were introduced).
- #1189 — Retiring one-shot on-startup backfills (argues against adding a new startup
  job; lazy-on-read approach taken here avoids that pattern).
- #1193 — Plan tab UX: request/offer descriptions and chat message noise (adjacent Plan
  tab improvement).

## Audit Notes

### Compliant items

1. **Proto conventions**: The plan extends `ListExperienceNeedsAndContributionsResponse`
   with its own `suggestions` and `category_hint` fields — it does not reuse another
   RPC's types. Each RPC has its own dedicated request/response. Compliant.

2. **Repository pattern (overall)**: Data flows through `ExperienceRepository` →
   `ExperienceNeedsNotifier` → `ExperienceNeedsSection`. The plan does not suggest
   viewmodels calling services directly. Compliant after the fix in item 3 below.

3. **No manual Map caches in viewmodels**: The plan does not recommend any manual
   `Map<String, T>` caches in viewmodels. Compliant.

4. **SQL efficiency**: The `storage.Update` call after successful generation is a single
   call on the success path only, not in a loop. Not an N+1 risk. Compliant.

5. **Test patterns**: Phase 2 references `setupTestService(t)`, which is the established
   test helper in `common_test.go`. That helper calls `storage.SetupTestStorage(t)`
   internally. The plan does not recommend mock databases. Compliant.

6. **Context timeout**: The existing `save.go` LLM calls do not apply a sub-timeout —
   they rely on the request context. The plan's recommendation to use `context.WithTimeout`
   of ~5s for the synchronous blocking RPC case is stricter and appropriate. Consistent
   with the `detachedContextWithTimeout` helper in `request/lifecycle.go`. Compliant and
   an improvement over the creation-time pattern.

7. **Viewmodel responsibility (loading/error state)**: The plan assigns loading and error
   state to the viewmodel, with data delegated to repositories. Compliant.

### Issues found and fixes made

**Issue 1 — Phase 3 ambiguous data routing (repository vs viewmodel)**
The original text read "Update `ExperienceNeedsViewModel` (or the Needs repository layer)
to expose `suggestions` and `categoryHint`". The "or" is ambiguous; per the architecture,
the repository layer is the correct home for data access, and the viewmodel should read
from repository response objects.

Fix: Replaced the single vague task with two explicit tasks: (a) map the new proto fields
in `ExperienceRepository.listNeedsAndContributions` / `refreshNeedsAndContributions`, and
(b) add `suggestions` and `categoryHint` fields to `ExperienceNeedsState` (the Freezed
state class) and populate them via `state.copyWith(...)` in `_load` and
`_refreshAfterMutation`. The viewmodel reads the data from the repository response object,
not from a service call.

**Issue 2 — Phase 3 missing Freezed state field update**
The original plan had no task to add `suggestions` and `categoryHint` to
`ExperienceNeedsState`. Without this, the viewmodel has no place to hold the data and
`ExperienceNeedsSection` cannot read it from the viewmodel.

Fix: Added an explicit task in Phase 3 to add `@Default([]) List<String> suggestions`
and `String? categoryHint` to `ExperienceNeedsState` and update `_load` /
`_refreshAfterMutation` accordingly.

**Issue 3 — i18n not mentioned for new UI strings**
The plan adds suggestion chips and potentially new empty-state copy to the Plan tab but
says nothing about string localization. Per `docs/client/i18n.md`, all user-visible
strings must be in ARB files; the CI lint check (`npm run lint:dart:i18n`) catches
hardcoded strings.

Fix: Added an explicit task in Phase 3 reminding implementers to add any new static
user-visible strings to `app_en.arb` and `app_es.arb` before running `npm run generate`.

**Issue 4 — "Fail Fast" deviation not justified**
The plan recommends logging a warning and returning empty suggestions on LLM failure,
which is a deviation from the "Fail Fast" principle in `CLAUDE.md`. The deviation is
legitimate (same pattern used in `save.go` lines 154–174 for creation-time generation),
but was not documented in the plan.

Fix: Expanded the lazy-generation task description to explicitly note this is a
documented best-effort deviation, reference the matching pattern in `save.go`, state the
UX justification, and require a code comment explaining the rationale.

**Issue 5 — Flutter test runner flag**
The plan mentions `npm run test:app` without noting the `--reporter expanded` requirement.
`CLAUDE.md` says to always use `--reporter expanded` when running Flutter tests directly.

Fix: Added a note in Phase 3 clarifying that `npm run test:app` should embed the flag,
and that direct `flutter test` invocations must pass `--reporter expanded`.
