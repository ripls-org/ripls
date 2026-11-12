# Issue #1930: Add known_for capability tags to user profile

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1930
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (depends on [docs/issues/1929-profile-v4-redesign.md](1929-profile-v4-redesign.md))

> **Audit notes (2026-05-15):** Reviewed against [client/architecture.md](../client/architecture.md), [client/caching.md](../client/caching.md), [client/i18n.md](../client/i18n.md), [client/accessibility.md](../client/accessibility.md), [client/testing.md](../client/testing.md), [server/architecture.md](../server/architecture.md), [server/observability.md](../server/observability.md). Gaps flagged inline with **[audit]** tags. The material adjustments: (1) Phase 4 must coordinate `ViewerProfileState` extension with the parallel #1929 / #1931 PRs to avoid merge collisions; (2) lowercase/title-case category normalization is English-only — call out as a v1 limitation; (3) widget tests stay scoped to behavior per [testing.md], not visual rendering.

## Problem Statement

The V4-A profile redesign (#1929) introduces a "Known For" chip row on the user profile (`user_profile_known_for_chips.dart`) — short capability tags like "Chainsaw lender", "Trailhead host", "Dinner cook" collapsed to 4 with a `+` expand. The V4-A PR ships the widget rendering empty (`SizedBox.shrink()` when the field is absent) because there is no `known_for` / `capabilities` / `tags` field on any current proto carrying a user profile (`UserImpactMetrics`, `GetUserProfileForViewerResponse`, etc.).

This issue covers landing real data behind the section.

The substantive design call is **where the tags come from.** Four candidate approaches, with what we found in the codebase:

1. **Activity-derived** — top gear-loan categories and top experience-host categories. Feasible (~70% of plumbing exists): `Gear.category` ([proto/ripls/models/gear.proto:59](../../proto/ripls/models/gear.proto#L59)) and `Experience.category` ([proto/ripls/models/experience.proto:61](../../proto/ripls/models/experience.proto#L61)) are AI-inferred at creation and preserved per item. The metric-detail calculator at [server/impact_metrics/metric_detail_rankings.go:307-492](../../server/impact_metrics/metric_detail_rankings.go) already does per-user ranking. Missing: no materialized "top categories by user" — needs a new aggregation query.
2. **Self-declared** — minimal-viable but requires a proto/schema change. `User.description` ([proto/ripls/models/user.proto:65](../../proto/ripls/models/user.proto#L65)) is a free-text bio; there is no `User.known_for` / `User.skills` field today.
3. **AI-summarized** — infrastructure exists but is not wired. The `Provider` interface at [server/ai/provider.go:14-150](../../server/ai/provider.go) has `GenerateHeroCardCopy`, `GenerateConversationSummary`, `GenerateExperienceCompletionSummary`, but no user-trait summarizer. The deprecated `BriefPayload.kicker_label` / `closing_line` ([proto/ripls/api/workshop_service.proto:147-158](../../proto/ripls/api/workshop_service.proto#L147)) prove the codebase has *removed* per-user prose generation, not maintained it. New prompt design + eval gold set needed.
4. **Hybrid** — activity-derived defaults with optional self-declared override.

## Context from Comments

No comments on this issue (filed today). The body and the parent #1929 plan provide enough direction; the open call is which of the four approaches to ship.

## Potential Solutions

### Option A: Activity-derived only

Add `repeated string known_for = N` on `GetUserProfileForViewerResponse`. Derive server-side as: top 3 by count from `Gear.category` over `Transfer` rows where `owner_id = target AND state = COMPLETED`, plus top 3 by count from `Experience.category` over `Experience` rows where `owner_id = target AND state = COMPLETED`. Format each as `"{category} lender"` / `"{category} host"`.

- **Approach:** Reuse the proven aggregation pattern from [server/impact_metrics/metric_detail_rankings.go:307](../../server/impact_metrics/metric_detail_rankings.go#L307). Single SQL per user with `GROUP BY category ORDER BY COUNT DESC LIMIT N`. Two queries total (gear + experience). No new AI infrastructure. Server-side English suffix; locale handled later if/when needed.
- **Pros:** Smallest diff. Reuses existing patterns. Zero AI cost. Always-fresh on profile view. No new schema migration.
- **Cons:** Suffix templating (`" lender"`, `" host"`) is rigid and English-only. Cold start: new users with no completed transfers / experiences get an empty section. The V4-A copy ("Chainsaw lender", "Trailhead host", "Climb partner", "Knot tier") includes flavors that won't fall out of category strings — "Climb partner" and "Knot tier" are vibes, not categories.
- **Effort:** Small.

### Option B: Self-declared only

Add `repeated string known_for` on `models.User` and a `SetKnownFor` RPC on `UserService` (or fold into the existing `UpdateUserProfile` if one exists). Render on profile. Edit UI in profile-settings.

- **Approach:** Proto + storage migration + mutation handler + settings UI.
- **Pros:** Captures aspiration and skill that doesn't fall out of activity. Users own their narrative. No AI cost.
- **Cons:** Empty by default. Maintenance burden — users have to add and prune. Spammy / misleading labels are possible if not moderated. The settings UI alone is non-trivial. Locale already user-owned (they type what they want).
- **Effort:** Medium.

### Option C: AI-summarized

New `GenerateUserKnownFor` method on the `Provider` interface. Prompt takes user activity context (top categories, recent transfers, experience history, bio text) and returns 4–6 evocative chip strings. Cache server-side on a new `models.UserKnownFor` row keyed by user, regenerated on milestone activity.

- **Approach:** Follows the `GenerateHeroCardCopy` pattern at [server/services/workshop/copy_provider.go](../../server/services/workshop/copy_provider.go) — provider call, persistent storage, graceful fallback. New prompt in [server/ai/prompts.go](../../server/ai/prompts.go) + eval gold set following [server/ai/eval/](../../server/ai/eval/) conventions.
- **Pros:** Captures flavor ("Climb partner", "Knot tier") that activity-only misses. Tone matches the V4-A redesign. Locale via the regional `region` parameter already threaded through the `Provider` interface.
- **Cons:** Cost — every active user needs periodic regeneration. Hallucination risk. Needs prompt design, eval coverage, and cadence-and-cache plumbing. Larger PR.
- **Effort:** Large.

### Option D: Hybrid (Option A defaults + Option B overrides)

Activity-derived tags compute as in Option A, but if `User.known_for` (self-declared, from Option B) is non-empty, those override the derived list.

- **Approach:** Both Option A's server aggregation and Option B's proto field + mutation handler. The settings UI is the long pole; the proto and override logic are small.
- **Pros:** Day-one tags for everyone with activity. User control where it matters. Empty-state risk only for brand-new users.
- **Cons:** Combined effort of A + B (without C). Two paths to maintain. The override-vs-merge question is real — if a user self-declares only 2 tags, do we still pad with derived ones?
- **Effort:** Medium.

## Recommendation

**Option A (activity-derived) for v1, with the proto field named generically so Option B / C can layer on later.**

Reasoning:
- Smallest diff that lands real content under the chip section. Unblocks the V4-A redesign visually for the majority of users.
- The infrastructure already exists; we're materializing one new aggregation, not building anything novel.
- The cold-start case (new users with no activity) is acceptable — the V4-A redesign already hides empty sections (#1929 decision).
- Option C's flavor advantage ("Climb partner", "Knot tier") is real but should not block landing *something*. File C as a follow-up if v1 reads flat.
- Option B is a real product decision (do we want users editing their tags?) that shouldn't be conflated with the V4-A redesign rollout.

Concretely:
- Field name `known_for` (not `known_for_categories`) so the same field carries Option C strings later without a wire change.
- Suffix templating is server-side and minimal: `" lender"` for gear, `" host"` for experiences. No locale handling for v1 — the templates are English-only, documented as a known limitation in the PR description with a follow-up.

## Open Questions (Resolved)

All resolved 2026-05-15 in favor of Option A. Decisions are load-bearing.

- **Approach:** Activity-derived (Option A). Defer self-declared (B) and AI-summarized (C) to separate follow-ups if the v1 output reads flat.
- **Proto field name:** `known_for` on `GetUserProfileForViewerResponse` — generic enough for future Option B / C to populate without a wire change.
- **Tag count:** Up to 6 total: top 3 by count from gear-loan categories + top 3 from experience-host categories. The chip widget collapses to 4 with `+` expand already.
- **Suffix templating:** `"{category} lender"` for gear, `"{category} host"` for experiences. English-only for v1. Documented as a limitation; locale follow-up filed only if Spanish surface area calls for it.
- **Threshold:** require at least 2 completed transactions in a category before it becomes a tag, to avoid one-off "lender" labels. Tunable constant in the derivation file.
- **Category normalization:** `Gear.category` is free-text AI-inferred. Case-normalize ("Power Tools" → "power tools" → display "Power tools") and de-duplicate near-matches in v1 with a simple lowercase compare. Do not attempt synonym collapsing — defer to Option C if it matters.
- **Cold start:** users with no qualifying activity surface an empty `known_for`. V4-A widget already returns `SizedBox.shrink()`.
- **Caching:** rely on the existing profile-response cache. No separate caching layer for the derivation — it runs as part of `GetUserProfileForViewer` and rides that TTL.

## Implementation Plan

### Phase 1: Proto + derivation skeleton

**Goal:** Wire the field through with a deterministic dummy derivation so the client can integrate before the real query lands.

**Tasks:**
- [ ] Add `repeated string known_for = N` on `GetUserProfileForViewerResponse` in [proto/ripls/api/profile_service.proto](../../proto/ripls/api/profile_service.proto). Comment describes the field per [CLAUDE.md §Proto Comments](../../CLAUDE.md#proto-comments) — no vendor names, no server-side mechanics, just "Short capability tags the target is known for in shared communities."
- [ ] Run `npm run generate` to produce Go and Dart bindings.
- [ ] In [server/services/profile/profile.go](../../server/services/profile/profile.go), populate `known_for` with an empty slice. Land this first so the wire shape is stable.

**Validates:** Proto compiles. Client can read the field even though it's empty.

### Phase 2: Activity-derived query

**Goal:** Replace the empty-slice stub with the real aggregation.

**Tasks:**
- [ ] Add a new derivation function in `server/services/profile/` (new file `known_for.go`):
  - Input: `targetUserID string`, `viewerCommunityIDs []string` (the viewer ∩ target community scope from the existing profile handler).
  - Output: `[]string` of up to 6 tags.
- [ ] Query 1 — top gear-loan categories:
  ```sql
  SELECT g.category, COUNT(*) AS n
    FROM transfers t
    JOIN gear g ON g.id = t.gear_id
   WHERE t.owner_id = $1
     AND t.state = 'COMPLETED'
     AND t.community_id = ANY($2)
     AND g.category <> ''
  GROUP BY g.category
  HAVING COUNT(*) >= 2
  ORDER BY n DESC
  LIMIT 3
  ```
- [ ] Query 2 — top experience-host categories:
  ```sql
  SELECT category, COUNT(*) AS n
    FROM experiences
   WHERE owner_id = $1
     AND state = 'COMPLETED'
     AND community_id = ANY($2)
     AND category <> ''
  GROUP BY category
  HAVING COUNT(*) >= 2
  ORDER BY n DESC
  LIMIT 3
  ```
- [ ] Compose: lowercase + title-case the category, append the suffix (`" lender"` / `" host"`), dedupe by lowercased value, return up to 6 strings. **[audit]** This normalization is ASCII-English-aware only — `strings.Title` (or its replacement, `cases.Title(language.English)`) does not handle Spanish properly. Documented as a v1 limitation; the locale follow-up below covers it.
- [ ] Call from [server/services/profile/profile.go](../../server/services/profile/profile.go) and populate the response field.
- [ ] **Structured logging** per [server/observability.md](../server/observability.md): `logger := logging.LoggerWithContext(ctx).With("operation", "GetUserProfileForViewer.known_for", "target_user_id", targetID)`; log derivation duration at Debug; on query error, log at Error and return `connecterr.Internal(ctx, "GetUserProfileForViewer", err, "target_user_id", targetID)` per [CLAUDE.md §Error Handling](../../CLAUDE.md#error-handling-and-robustness).
- [ ] **SQL efficiency** per [CLAUDE.md §SQL Efficiency](../../CLAUDE.md#sql-efficiency): two queries total (one JOIN for gear, one straight read for experience). No per-user loops. Indexes: `transfers(owner_id, state, community_id)` and `experiences(owner_id, state, community_id, category)` — check `terraform/modules/storage/` / migration files to confirm; if missing, add migrations in the same PR.

**Validates:** A target user with 3+ completed loans of a single category and 2+ hosted experiences surfaces both as chips. Empty target → empty array.

### Phase 3: Tests

**Goal:** Lock the contract.

**Tasks:**
- [ ] Server unit tests in `server/services/profile/known_for_test.go`:
  - Empty user (no transfers, no experiences) → `[]`.
  - Single category at threshold (exactly 2 loans) → 1 chip.
  - Single category below threshold (1 loan) → `[]`.
  - Mixed gear + experience activity → up to 6 chips, gear first.
  - Categories with empty strings or whitespace → filtered.
  - Categories outside the viewer-community scope → excluded.
  - Wrap each in `storage.AssertMaxQueries(t, ctx, 2, fn)` to lock the query budget at 2.
- [ ] Client widget test in `app/test/presentation/widgets/user_profile/v4/user_profile_known_for_chips_test.dart`: populated case (already covered for empty in V4-A PR). Confirms the `+` expand from a 6-chip list reveals the hidden 2.

**Validates:** Empty case, threshold edge, max-budget query count, client rendering of populated case.

### Phase 4: Drop the V4-A stub

**Goal:** Replace `SizedBox.shrink()` gating with the real field.

**Tasks:**
- [ ] In `user_profile_known_for_chips.dart` (created by #1929), read `state.knownFor` from `ViewerProfileState`. Show when non-empty; collapse when empty (existing behavior).
- [ ] Extend `ViewerProfileState` in [app/lib/presentation/viewmodels/viewer_profile_view_model.dart](../../app/lib/presentation/viewmodels/viewer_profile_view_model.dart): add `@Default(<String>[]) List<String> knownFor`. Populate in `_load()` from `response.knownFor`. **[audit] Merge-coordination:** #1929 already extends this same Freezed class to add `userImpactMetrics`, and #1931 extends it again with three narrative fields. If these PRs land out of order, expect Freezed regeneration conflicts. Call out the dependency in the PR description and rebase against `main` before merging.
- [ ] **ViewModel test** under `app/test/presentation/viewmodels/viewer_profile_view_model_test.dart` per [client/testing.md](../client/testing.md): mocked `ProfileRepository` returns a response with 3 `known_for` entries; assert state carries them through.
- [ ] Run `npm run lint:dart:i18n` — confirm the chip strings (server-derived) don't trip the hardcoded-string rule. If they do (the rule scans for string literals at render sites), wrap rendering with `Text(tag)` which is acceptable since `tag` is a variable, not a literal.

**Validates:** End-to-end: server query → proto → repository cache → viewmodel state → chip widget. `npm run build && npm run test:app` pass.

## Related Issues

- #1929 — V4-A profile redesign. This issue unblocks the Known For chip section that ships empty in #1929's PR.
- #1931 — Per-user AI narrative copy for impact rows. Sibling follow-up; same approach considerations (AI vs derived) but for prose, not chips.
- Possible follow-ups, file only if v1 reads flat:
  - "Self-declared known_for overrides" (Option B).
  - "AI-summarized known_for chips" (Option C).
  - "Localize known_for tag suffixes" (Spanish surface area).
