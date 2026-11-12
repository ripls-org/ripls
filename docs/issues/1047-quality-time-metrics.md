# Issue #1047: Quality time metrics

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1047
**Filed:** 2026-04-06 | **Last activity:** 2026-04-06
**Labels:** none | **Assignees:** tescobar
**Context docs:** [docs/impact_metrics.md](../impact_metrics.md)

## Problem Statement

Quality Time (QT) metrics for experiences are static and disconnected from the
actual experience properties. Several issues exist:

1. **All experiences show the same ~2h / same-weight QT value** — the builder
   uses config defaults (120 min duration, 1.3 vulnerability, etc.) rather than
   inferring attributes from the experience description, duration, or RSVPs.
2. **No dynamic recalculation** — when a user edits the name, description,
   RSVPs, or duration of an experience, QT is not recomputed. Only RSVP YES
   changes trigger a group-size-only rebuild.
3. **No QT attributes displayed on experience detail** — unlike gear (which
   shows material, weight, est. value), experiences don't surface tie strength,
   reciprocity, novelty, or vulnerability as human-readable attributes in the
   detail section.
4. **Completion modal impact is a visual approximation** — the impact bar
   scales linearly by confirmed/total attendees, but doesn't run the real QT
   formula with actual group size, tie strength, etc.
5. **Post-completion metrics not refreshed** — community and portfolio metrics
   caches are not invalidated after experience completion, so the user doesn't
   see updated impact until manual refresh.
6. **No consistency guarantee** — no test ensures the QT value in the
   completion modal equals the value in the story, the experience detail tab,
   and the community/portfolio metrics drill-down.
7. **Requests not covered** — requests should have the same dynamic QT support,
   with helpers (offered → confirmed) playing the role that RSVPs play for
   experiences.

## Context from Comments

No comments on this issue. The requirements are entirely in the description.
This is a freshly filed issue — may benefit from clarification on priority
ordering and whether LLM inference at creation time is in scope.

## Potential Solutions

### Option A: Incremental lifecycle hooks

Add recalculation hooks at each mutation point (save, update, RSVP, complete)
without changing the estimator or builder APIs. LLM inference stays at
completion only; creation and updates use config defaults + user-set duration.

- **Approach:** Wire `BuildExperienceImpactMetrics` into `UpdateExperience`,
  extract duration from time fields, recalculate on every property change.
  Add cache invalidation for community/portfolio metrics after completion.
  Surface QT attributes as human-readable tiers in the experience detail UI.
- **Pros:** Minimal API changes, low risk, addresses most issues.
- **Cons:** QT attributes at creation/edit time are still config defaults
  (not LLM-inferred). Novelty/vulnerability won't be experience-specific
  until completion.
- **Effort:** Medium.

### Option B: LLM inference at creation and update

Call `InferSocialAttributes` when an experience is created or significantly
edited (name/description change), not just at completion. This makes QT
attributes experience-specific from the start.

- **Approach:** Add async LLM inference in `SaveExperience` and
  `UpdateExperience` (behind a feature flag or best-effort). Store inferred
  attributes on the experience. Rebuild QT whenever any input changes.
- **Pros:** QT values are meaningful and differentiated from creation onward.
  Users see realistic estimates while planning. Attributes in the detail
  section are experience-specific.
- **Cons:** Higher latency on save/update (LLM call). More complex error
  handling. Need to handle LLM unavailability gracefully.
- **Effort:** Large.

### Option C: Hybrid — user-set + config defaults now, LLM later

Use user-set values (duration from time fields, group size from RSVPs) and
config defaults for non-user-observable attributes (tie strength, novelty,
vulnerability). Add LLM inference as a follow-up phase. Focus on correctness,
display, cache invalidation, and testing first.

- **Approach:** Same as Option A but with a clearer phased plan that
  explicitly defers LLM inference. Phase 1 gets the plumbing right, Phase 2
  adds LLM enrichment, Phase 3 extends to requests.
- **Pros:** Fastest time to correctness. Each phase is independently
  shippable. Testing infrastructure built first.
- **Cons:** QT values remain partially generic until LLM phase ships.
- **Effort:** Medium (Phase 1), Medium (Phase 2), Medium (Phase 3).

## Recommendation

**Option C (Hybrid phased approach).** The core problems are (a) stale/static
values, (b) missing recalculation, (c) no display of attributes, (d) no cache
invalidation, and (e) no consistency tests. All of these can be fixed without
LLM inference. LLM enrichment at creation/update is valuable but orthogonal —
it improves accuracy, not correctness. Ship the plumbing first, then layer on
intelligence.

## Decisions

- **Completion modal calls server for live QT estimate:** Yes. Add a
  `PreviewExperienceImpact` RPC that returns a real estimate for the current
  confirmed attendee set. The client-side linear scale factor is not accurate
  enough — group-size tiers are non-linear (dyadic vs. small vs. medium vs.
  large have different multipliers).
- **QT attribute display in detail section:** Use human-readable tier labels
  from the proto enums (e.g., "Small group (3–5)", "Novel interaction", "High
  vulnerability"). Show duration and group size as actual numbers since they're
  user-set; show the remaining 5 attributes as tier labels with brief
  descriptions.
- **Which attributes to show:** Only show the 5 attributes that are not
  already displayed elsewhere (tie strength, reciprocity, novelty,
  vulnerability, modality). Duration and group size are already visible in the
  experience detail — do not repeat them in the QT attributes section.
- **LLM re-inference trigger:** Name or description change. Duration or RSVP
  changes use actual values directly (no LLM needed). Deferred to Phase 2.
- **Request group size:** During active phase, use offer count (current
  behavior). At fulfillment, recalculate with confirmed helper count. This
  matches the experience pattern (RSVPs during active, actual attendees at
  completion).

## Implementation Plan

### Phase 1: Experience QT correctness and display

**Goal:** QT values are dynamic, displayed as attributes, cached correctly,
and tested for consistency across all surfaces.

**Tasks:**

- [ ] **Server: Recalculate QT on experience update** — In `UpdateExperience`
  (server/services/experience/save.go), after persisting changes, call
  `BuildExperienceImpactMetrics` with current attendee count, user-set
  duration, and connection context. Persist the updated `ImpactEstimate`.
- [ ] **Server: Use user-set duration at creation** — In `SaveExperience`,
  extract duration from `Experience.Time` fields (using
  `extractExperienceDurationMinutes`) and pass it as a hint to the builder.
  Currently only done at completion time.
- [ ] **Server: Add `PreviewExperienceImpact` RPC** — New lightweight RPC
  that accepts experience ID + list of confirmed attendee IDs, computes QT
  with the real formula, and returns the estimate without persisting. Used by
  the completion modal for live preview.
- [ ] **Flutter: Add `PreviewExperienceImpact` to service and repository** —
  Add the new RPC to `ExperienceService` (service layer) and expose it via
  `ExperienceRepository` (repository layer). ViewModels must call the
  repository, never the service directly (per client architecture rules:
  ViewModels → Repositories → Services).
- [ ] **Flutter: Display QT attributes on experience detail** — Add a section
  to the experience detail screen showing the 5 attributes not already
  displayed elsewhere: tie strength, reciprocity, novelty, vulnerability, and
  modality. Do NOT repeat duration or group size (already visible in the
  experience detail). Use human-readable tier labels. All user-visible tier
  label strings must be i18n'd via ARB keys in `app_en.arb` / `app_es.arb`
  (e.g., `experienceQtTieStrengthNew`, `experienceQtNoveltyNovel`) — do not
  hardcode English strings in Dart code. The ViewModel exposes typed enum
  state; the widget resolves to localized text in `build()` via
  `context.l10n`.
- [ ] **Flutter: Live QT preview in completion modal** — Replace the linear
  scale factor in `_buildImpactBar` with calls to the repository's
  `previewExperienceImpact()` method (via the ViewModel) as attendees are
  toggled (debounced). The call chain must be: Screen → ViewModel →
  Repository → Service (never screen → service directly).
- [ ] **Flutter: Event-driven impact cache invalidation after completion** —
  In `ExperienceRepository.completeExperience()`, after persisting the
  completion, call `impactCacheInvalidationProvider.notify()` (following the
  event-driven cache invalidation pattern from caching.md Pattern 7 / the
  `dailyCacheInvalidationProvider` model). Do NOT invalidate impact caches
  from the ViewModel directly — repositories own cache invalidation.
- [ ] **Flutter: React to impact cache invalidation in metrics ViewModels** —
  Have `CommunityImpactMetricsNotifier` and `PortfolioMetricsNotifier` listen
  to the impact cache invalidation provider in their `build()` methods. On
  signal, refresh in place (no loading spinner). This replaces explicit
  "refresh on next viewing" logic with the established reactive pattern.
- [ ] **Server tests: QT consistency across surfaces** — Add integration tests
  that verify the QT value returned by `PreviewExperienceImpact` (with
  confirmed attendees) equals the value stored on the completed experience,
  equals the value in the generated story, and equals the value returned by
  `GetCommunityImpactMetrics` drill-down for that experience.
- [ ] **Server tests: QT recalculation on update** — Unit test that
  `UpdateExperience` with changed duration produces a different QT value.
- [ ] **Flutter tests: Completion modal shows real QT** — ViewModel unit test
  (using `ProviderContainer`, no widget tree) that verifies the completion
  ViewModel calls the repository's `previewExperienceImpact()` and exposes
  the returned value in state. Separate widget test verifies the screen
  renders the ViewModel's QT value correctly.

**Validates:** QT values are correct, dynamic, and consistent across the
completion modal, story, experience detail, and community/portfolio metrics.

### Phase 2: LLM-inferred attributes at creation and update

**Goal:** QT attributes are experience-specific from creation onward, inferred
by the LLM from the experience name and description.

**Tasks:**

- [ ] **Server: Call `InferSocialAttributes` in `SaveExperience`** — After
  persisting the experience, call the LLM to infer duration (if not user-set)
  and vulnerability. Update the stored `ImpactEstimate` with the inferred
  hint. Handle LLM unavailability gracefully (fall back to config defaults).
- [ ] **Server: Call `InferSocialAttributes` on name/description change** —
  In `UpdateExperience`, detect if name or description changed. If so,
  re-infer social attributes and rebuild QT. Skip if only time/RSVP changed
  (those use actual values directly).
- [ ] **Flutter: Show inferred attributes in real time** — After save/update,
  the repository invalidates the experience cache entry and fires the impact
  cache invalidation signal. The detail ViewModel reloads via the repository
  (not the service directly) to get updated QT attributes.
- [ ] **Server tests: LLM inference at creation** — Integration test (tagged
  `integration`) that creates an experience and verifies the stored QT
  attributes differ from config defaults when LLM is available.
- [ ] **Server tests: Fallback on LLM failure** — Unit test that LLM
  unavailability falls back to config defaults without error.

**Validates:** Experiences have differentiated QT values from creation.
Users see meaningful attribute estimates while planning.

### Phase 3: Extend to requests

**Goal:** Requests have the same dynamic QT support as experiences, with
helpers playing the role of RSVPs.

**Tasks:**

- [ ] **Server: Recalculate QT on request update** — In `UpdateRequest`,
  rebuild QT when title/description changes (for LLM re-inference) or when
  offers change.
- [ ] **Server: Use confirmed helper count at fulfillment** — In
  `MarkRequestFulfilled`, rebuild QT group size from confirmed helper IDs
  (not total offer count). Currently uses whatever group size was set by the
  last offer.
- [ ] **Server: Add `PreviewRequestImpact` RPC** — Same pattern as
  `PreviewExperienceImpact` but for requests with confirmed helper IDs.
- [ ] **Flutter: Add `PreviewRequestImpact` to service and repository** —
  Same layering as experience: Service → Repository → ViewModel. No direct
  service calls from ViewModels.
- [ ] **Flutter: Display QT attributes on request detail** — Same i18n'd
  pattern as experience detail (ARB keys, ViewModel exposes enums, widget
  resolves via `context.l10n`), showing human-readable tier labels for the 5
  attributes not already displayed elsewhere (tie strength, reciprocity,
  novelty, vulnerability, modality).
- [ ] **Flutter: Live QT preview in request fulfillment modal** — Show
  real-time QT via ViewModel → Repository → Service chain as helpers are
  confirmed/deconfirmed (debounced).
- [ ] **Flutter: Event-driven impact cache invalidation after fulfillment** —
  Same `impactCacheInvalidationProvider.notify()` pattern as experience
  completion, triggered from `RequestRepository.markRequestFulfilled()`.
- [ ] **Server tests: QT consistency for requests** — Same consistency tests
  as Phase 1 but for the request lifecycle.
- [ ] **Server tests: Helper count vs offer count** — Test that fulfilled
  request QT uses confirmed helper count, not total offer count.

**Validates:** Requests have dynamic, consistent QT metrics. Helper
confirmation drives group size at fulfillment.

## Related Issues

- None referenced in the issue. This is a standalone improvement to the
  quality time metric system for experiences and requests.

## Audit Notes

The following changes were made during architectural compliance review:

1. **Data access layering (client/architecture.md):** Original plan had the
   completion modal calling `PreviewExperienceImpact` directly and cache
   invalidation wired "into ExperienceRepository or the completion viewmodel."
   Fixed to enforce the strict Screen → ViewModel → Repository → Service
   chain. Added explicit task to create the service and repository methods
   for the new Preview RPCs. ViewModels never call services directly;
   repositories own all cache invalidation.

2. **Event-driven cache invalidation (client/caching.md Pattern 7):** Original
   plan used "invalidate caches" and "refresh on next viewing" — vague and
   inconsistent with the established `dailyCacheInvalidationProvider` reactive
   pattern. Replaced with an `impactCacheInvalidationProvider` signal fired
   from the repository after mutations, with metrics ViewModels listening and
   refreshing in place. This matches the proven debounce + cooldown model.

3. **i18n compliance (client/i18n.md):** Original plan hardcoded English tier
   labels ("Novel interaction", "High vulnerability") in task descriptions.
   Updated to require ARB keys for all user-visible QT attribute labels, with
   ViewModels exposing typed enum state and widgets resolving to localized
   text via `context.l10n`.

4. **Test separation (client/architecture.md):** Original plan had a single
   "Flutter tests: Completion modal shows real QT" task. Split into ViewModel
   unit test (using ProviderContainer, no widget tree) and separate widget
   test, following the "Test ViewModels with fake implementations" / "Don't
   test UI and logic together" guidelines.

5. **Server architecture (server/architecture.md):** No violations found. The
   `PreviewExperienceImpact` and `PreviewRequestImpact` RPCs are new service
   endpoints, not service-to-service calls. The impact builder is already a
   shared library, correctly used via constructor injection.
