# Issue #2240: Needs & Contributions: link claims/contributions to items in the user's library

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2240
**Filed:** 2026-05-29 | **Last activity:** 2026-05-29
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/client/needs.md](../client/needs.md), [docs/cowork/App Design/claim-sheet-prototype.html](../cowork/App%20Design/claim-sheet-prototype.html)

> **Product decisions (2026-05-29).** Issue author resolved three of
> the open questions: (1) **exactly one gear link per contribution**
> — proto is `optional string gear_id`, picker is single-select,
> no count cap needed; (2) **search reuses the @-mention lookup**
> (`searchRepository.search` with `SEARCH_STRATEGY_EXACT` + gear
> item type, mirroring `ConversationViewModel.fetchMentionSuggestions`),
> intersected with the user's owned gear; (3) **capture path opens
> the standard create-gear modal** — camera / text / hyperlink input
> modes all stay available via the in-modal switcher, not a
> camera-only entry point. Plan revised end-to-end to reflect these.
>
> **Audit pass (2026-05-29).** Plan reviewed against
> [docs/client/architecture.md](../client/architecture.md),
> [caching.md](../client/caching.md), [i18n.md](../client/i18n.md),
> [accessibility.md](../client/accessibility.md),
> [testing.md](../client/testing.md),
> [docs/server/architecture.md](../server/architecture.md), and
> [docs/server/observability.md](../server/observability.md).
> Added a "Cross-cutting compliance notes" section
> and tightened per-phase tasks to require: `Tappable` / `Toggle` /
> `IconAction` + `a11y*` l10n labels for every new interactive
> surface, `showAccessibleModal` for sheet entry, `AppColors`-only
> color tokens, `CachedMediaImage` (with `semanticsLabel`) for the
> gear pill thumbnail, explicit `Semantics` annotation on the
> tombstone variant, `RpcErrorHandler` for error l10n,
> `ProviderContainer`+Mockito ViewModel tests (loading/success/error,
> cache hit vs. miss, disposal safety),
> `connecterr.Internal` vs `CodeInvalidArgument` on the server,
> `logging.LoggerWithContext` with the standard field set (entity
> IDs separate from `request_id`), `storage.ProtoSQLStorage` +
> `GetByIDs` batching + `storage.AssertMaxQueries` guard for the
> contribution-list builder, repository-owned caching with explicit
> `refreshUserGear()` after `SaveGear`, and preservation of the
> Experience-scope auto-RSVP-before-claim behavior so parity with
> needs.md "Intentional differences" is not perturbed.

## Problem Statement

Today a Needs claim or freestanding contribution is opaque to the rest of the
app: just a free-text title, optional note, and quantity. The `Gear` the user
actually owns (or is about to own, in the photo-capture path) is never linked
to the claim. That breaks three flows the rest of the product already
supports:

1. **Coordination context.** A claim that says "tarp" is a string. A claim
   linked to a `Gear` row shows photo, size, owner, condition — what the
   organizer/requester actually needs to know to plan.
2. **Library growth as a side effect.** New users have empty libraries.
   If "bringing a thing" is also the entry point to *adding* that thing to
   the library, the system gets richer at exactly the moment a user has a
   concrete reason to capture it.
3. **Downstream reuse.** A library item linked to a contribution is
   discoverable later (search, kit auto-complete, friend-graph hints — the
   already-deferred #3 in needs.md).

The issue proposes extending the Picker (NK1–NK4) and the claim/contribution
flow on both Experience and Request scopes so a single claim can link **one**
item from the contributor's library, plus support a capture path that creates
a new `Gear` and links it in the same gesture. Reference design is the v2
claim-sheet prototype.

**One claim → one gear link** is a deliberate constraint. If a participant
is bringing two distinct things, that's two contributions, not one with two
pills. This keeps slot accounting clean (each Need slot maps 1:1 to a
contribution and to at most one linked gear) and matches how the prototype
renders the pill on the row.

No contradictions surfaced between the issue body and comments — the issue
has no comments yet. The body is detailed and product-decided on the UX
shape; the open questions it lists are mostly server/proto plumbing
decisions, which this plan addresses.

## Context from Comments

The issue currently has **no comments**. The body itself was authored with
full context, and it back-links to #2228 ("Add ability to create gear /
share existing gear when satisfying a request"), which is the
Request-scope-only precursor of the same idea. #2228 has an empty body, so
this issue effectively supersedes it — the plan below treats #2240 as the
single source of truth.

Two things from the issue body that constrain the plan:

- **Parity is the contract.** needs.md's "Intentional differences" section
  is closed; this feature must not add a new row. The same widgets serve
  Experience and Request scopes.
- **Snapshot-on-claim semantics** in [docs/planning.md](../planning.md)
  must be respected. Today the claim freezes `original_need_name` and
  `original_need_note` at claim time so the contribution stays legible if
  the Need is later edited or removed. Linked gear gets the same
  treatment — see Option A in the proto discussion below.

## Potential Solutions

The product shape is decided (per the issue body and the prototype). The
real choices are on the data model and on the photo path. Three options
each, with a recommendation at the end.

### Data model: how a claim references gear

#### Option A: `optional string gear_id` on `PlanningContribution`

- **Approach:** Add `optional string gear_id = 13` to
  [PlanningContribution](../../proto/ripls/models/planning.proto#L36)
  (per `CLAUDE.md`§"Proto Field Conventions" — conditionally-present
  fields are `optional`). Add `optional string gear_id` to
  `ClaimExperienceNeedRequest`, `ClaimRequestNeedRequest`,
  `AddExperienceContributionRequest`, `AddRequestContributionRequest`,
  and `EditExperienceContributionRequest` (plus request equivalent).
  Snapshot-on-claim of gear-display data is *not* done — the linked
  `Gear` is fetched live on render. If a linked gear is later renamed,
  the claim shows the new name (intentional — the link is to the live
  thing, not a snapshot).
- **Pros:** Simplest possible model. Mirrors the existing single-field
  `gear_id` precedent across the codebase (transfer,
  conversation_topic, community.proto, notification.proto). No join
  table, no migrations beyond proto. Live-fetch matches how gear is
  shown on transfer and chat surfaces (no per-row snapshot there
  either). Single-link semantics line up with one-slot-one-thing
  accounting.
- **Cons:** Deleted gear becomes a dangling reference. The
  contribution row keeps the ID but the render-time fetch returns
  nothing — render as a tombstone pill. Bulk render still needs a
  single batched `GetByIDs` across the visible contribution set
  (standard per `CLAUDE.md`§"SQL Efficiency").
- **Effort:** Small (proto + storage column + server validation +
  client wiring).

#### Option B: New `PlanningContributionGear` join table

- **Approach:** Separate storage model linking `contribution_id` ↔
  `gear_id`, with optional snapshot fields (`gear_name_at_claim`,
  `gear_thumbnail_media_id_at_claim`).
- **Pros:** Easy to add metadata later (e.g., loan agreement
  reference). Tombstone for deleted gear is trivial.
- **Cons:** Heavier — new storage table, new conversion code,
  proto-API and proto-models both grow. With the one-link-per-claim
  constraint, the join table degenerates to a 1:1 — even more
  obviously YAGNI.
- **Effort:** Medium.

#### Option C: Use existing `description` field with structured JSON

- **Approach:** Stuff a `{"gear_id": "..."}` payload into
  `PlanningContribution.description`. No proto change.
- **Pros:** Ships in a day.
- **Cons:** Violates proto conventions; clients have to JSON-parse a
  free-text field; impossible to query server-side; no validation.
  Don't do this.
- **Effort:** Tiny but wrong.

### Capture path: new-gear plumbing

#### Option P1: Two-step (CreateGear → ClaimNeed with gear_id)

- **Approach:** Reuse the existing
  [`create_gear_modal.dart`](../../app/lib/presentation/screens/gear/create_gear_modal.dart)
  +
  [`gear_preview_modal.dart`](../../app/lib/presentation/screens/gear/gear_preview_modal.dart)
  +
  [`GenGearNotifier`](../../app/lib/presentation/viewmodels/gen_gear_view_model.dart)
  pipeline as-is. This is **not camera-only** — the existing modal
  already supports camera, text-entry, and hyperlink/URL input
  modes; the user can switch between them inside the modal. The
  claim sheet's "Snap" affordance is just one entry point into that
  same modal. On `SaveGear` success, the returned `gear_id` is
  added to the claim's pending selection; the claim sheet then
  submits `ClaimExperienceNeed` (or equivalent) with that ID. UX
  hides the seam with a single progress state; on error after gear
  creation, prompt the user to retry the claim (the gear is already
  saved to their library).
- **Pros:** Zero new server surface. Reuses a fully-tested pipeline
  *and* its three input modes for free.
- **Cons:** Partial-success window (gear saved, claim failed) —
  mitigated by an explicit toast.
- **Effort:** Small on server (none), medium on client (wiring the
  capture entry point into the claim sheet and threading the
  returned `gear_id` back).

#### Option P2: Atomic `ClaimNeedWithNewGear` RPC

- **Approach:** New server RPC that takes a draft gear + claim payload,
  saves the gear, then claims the slot, all in one transaction.
- **Pros:** No partial-success failure mode.
- **Cons:** Duplicates the gen/save pipeline; ill-fits the streaming
  `GenGearNotifier` flow which is already a multi-step state machine.
  Pays a heavy plumbing cost for a small UX win.
- **Effort:** Large.

## Recommendation

**Data model: Option A (`optional string gear_id` on
`PlanningContribution`).** It's the smallest change that solves the
stated problem, matches the existing `gear_id` precedent everywhere
else in the codebase, and the dangling-gear case is already a solved
problem elsewhere (transfer/chat surfaces show gear the same way and
deal with deletion via a tombstone). Take the live-fetch approach,
render a tombstone for deleted gear. If we later need per-link
metadata, migrating to a join table is a one-time backfill — cheap
insurance to defer.

**Capture path: Option P1 (two-step, reuse the existing create-gear
modal).** The existing modal already supports camera, text-entry, and
hyperlink input modes, and the `GenGearNotifier` pipeline behind it
has absorbed real bug fixes. Reusing it costs us almost nothing;
building a parallel atomic RPC costs a lot. The partial-success window
(gear saved, claim failed) is small and recoverable — the toast can
say "Gear saved. Tap to retry your claim."

## Decisions (product)

The following were resolved by the issue author and are now load-bearing
constraints, not open questions:

- **Maximum gear links per claim → exactly one.** One contribution
  links to at most one `Gear`. If a participant is bringing two
  things, they create two contributions. Proto field is
  `optional string gear_id`, not a repeated list. Client picker is
  single-select.
- **Search match → reuse the @-mention lookup from conversation.**
  Same name-match infrastructure as `ConversationViewModel.fetchMentionSuggestions`
  ([conversation_view_model.dart:647](../../app/lib/presentation/viewmodels/conversation_view_model.dart#L647))
  — `searchRepository.search` with `SearchStrategy.SEARCH_STRATEGY_EXACT`
  and `itemTypes: [SEARCH_ITEM_TYPE_GEAR]`. No new search code, no
  LLM. Filter to owner-only (see permissions decision below).
- **Capture path → standard create-gear modal, not camera-only.**
  Reuse [`create_gear_modal.dart`](../../app/lib/presentation/screens/gear/create_gear_modal.dart)
  as-is. That modal already supports camera, text-entry, and
  hyperlink/URL input modes with an in-modal switcher; the claim
  sheet's "Snap" affordance is just one entry point into that same
  modal. Users can switch input mode after entering.

## Open Questions

- **Permissions: can you link gear you don't own?** — Recommendation:
  **owner-only for v1**, matching what the issue body proposes. The
  prototype only shows owned gear in the picker rail. Borrowed gear is
  conceptually murky ("am I bringing your tarp?") and the library
  picker has no UX to surface non-owned gear today. Defer until there's
  a concrete request.

- **Deleted gear rendering on a historical claim** — Recommendation:
  render a **dim, non-tappable tombstone pill** ("removed item")
  rather than dropping the link silently. This matches the
  Archived-sheet's "dim uncovered rows" pattern and preserves the
  archaeological record on terminal events. Server returns the ID;
  client looks it up; null result → tombstone.

- **Snapshot on claim?** — Recommendation: **no snapshot for gear**.
  Name/note are already snapshotted for the *Need*, not the
  contribution. Gear is a first-class entity whose canonical state
  lives in `GearRepository`; mirroring it on the contribution would
  drift. The tombstone above handles deletion. If a user renames a
  gear, the claim correctly reflects the rename (this matches transfer
  + chat-pill behavior).

- **Does the capture path block on `SaveGear` before showing the
  claim as submitted?** — Recommendation: **yes, block.** The user
  explicitly tapped to add gear knowing the path runs gen+save first.
  Optimistic-claim-first would race against the gear ID not existing
  yet. Keep the spinner honest.

- **AddOptionGhost path** — the issue scopes "in" extending the Picker
  and the claim/contribution flow. The AddOptionGhost on Request scope
  is still gated behind needs.md Deferred work item 7 ("any member can
  add to a Request breakdown"). Recommendation: **don't unblock
  Deferred #7 in this work**. Gear-linking should land on the existing
  permission set; if Deferred #7 ships later, this feature comes along
  for free since the picker is the same.

- **`searchRepository.search` scope when no community context?** —
  The @-mention lookup is scoped by `communityIds` and requires a
  community to be present. The Needs flow always has a parent
  Experience or Request, both of which are community-scoped, so the
  community ID is available. Recommendation: **pass the parent's
  community ID** as the search scope, then **intersect with
  `listUserGear()`** to enforce owner-only. The intersection step is
  cheap (user libraries are small) and means the search index does
  not need to know "which gear does this user own."

## Cross-cutting compliance notes

These rules apply across every phase below; calling them out once
here so the per-phase task lists stay short.

- **Architecture (MVVM + Riverpod 3).** All gear-linking state lives
  on the existing `experienceNeedsProvider` / `requestNeedsProvider`
  Notifiers, extended via `copyWith`. No new `ChangeNotifier`, no
  `setState` for app state, no `ref.read` in `build`. If a transient
  picker-local state Notifier is added, use an `.autoDispose`
  `AsyncNotifier` (preferred per
  [docs/client/architecture.md](../client/architecture.md)) or apply
  `SafeNotifierMixin` if a regular `Notifier` is unavoidable, and
  ship a disposal-safety test.
- **Repository ownership of caching.** ViewModels must never touch
  `StashCacheManager` / `CacheService` directly. All library reads
  go through
  [`gearRepositoryProvider`](../../app/lib/data/repositories/gear_repository.dart).
  Library-state detection (Phase 3) uses the existing cached
  `listUserGear()` result; do not fork a new cache key. Cache key
  convention `namespace:identifier` is already honored
  (`gear:user:list`).
- **Cache invalidation after capture.** After `SaveGear` succeeds in
  the photo path, call `gearRepository.refreshUserGear()` (which
  invalidates `gear:user:list`) before the claim RPC submits so the
  next picker open reflects the new library. The new claim path
  itself does not need to invalidate gear caches.
- **Accessibility primitives.** Every new tappable surface uses
  [`Tappable`](../../app/lib/presentation/widgets/accessibility/) with
  a `semanticsLabel` resolved from `context.l10n.a11y*`. The picker's
  gear rows are
  [`Toggle`](../../app/lib/presentation/widgets/accessibility/) (not
  `Tappable`) so the screen reader announces selected state.
  Modals open via `showAccessibleModal`, never `showModalBottomSheet`.
  Any "remove" affordance on a `GearPill` uses `IconAction` with
  `semanticsLabel` + `tooltip`. The dim tombstone pill must include
  a `Semantics` annotation ("Removed item") to satisfy
  `avoid_color_only_status`. Lint
  (`npm run lint:dart:a11y` / `scripts/check_a11y.js`) blocks new
  violations — `scripts/a11y_allowlist.txt` may not grow.
- **AppColors only.** No `Colors.white`, `Colors.grey[*]`, or raw
  `Color(0xFF…)` in the picker, the pill, or the snap card. Use
  `AppColors.xxx(context)` and the theme tokens already wired into
  the v2 needs surfaces (see the `feedback_theme_reuse` memory).
- **i18n.** Every new string lands in
  [`app_en.arb`](../../app/lib/l10n/app_en.arb) and
  [`app_es.arb`](../../app/lib/l10n/app_es.arb) in the same change
  with `@key` metadata; `npm run lint:dart:i18n` (resolves via
  `context.l10n`) and `npm run lint:dart:l10n-parity` both run in CI.
  User-visible copy says **Event**, not "Experience" (code
  identifiers stay `Experience`). RPC errors surface through
  `RpcErrorHandler.classify` → `UserError` → `localize(...)` per
  [docs/client/i18n.md](../client/i18n.md); ViewModels never resolve
  strings.
- **Testing.** New ViewModel logic is tested via `ProviderContainer`
  with provider overrides and Mockito mocks (`@GenerateMocks`,
  `flutter pub run build_runner build`); `addTearDown(container.dispose)`;
  loading → success/error transitions; cache-hit-vs-miss
  (`verifyNever` on service for the hit path); optimistic-update
  rollback if any new mutation is optimistic; debounces use
  `runDebounced` from `app/test/helpers/fake_async_helpers.dart`.
  Coverage targets: ViewModels ≥ 90%, repositories ≥ 80%. Run with
  `flutter test --reporter expanded`.
- **Server observability.** Every new handler / extended handler
  uses `logger := logging.LoggerWithContext(ctx).With("operation",
  "ClaimExperienceNeed", "user_id", uid, ...)`. Standard fields per
  [docs/server/observability.md](../server/observability.md):
  `user_id`, `user_email` (via `logging.MaskEmail`), `operation`,
  `gear_id`, `experience_id` / `request_id`, `duration_ms`,
  `error`. Do **not** use `request_id` for entity IDs — it is
  reserved for the HTTP correlation header. Use `target_request_id`
  for a `models.Request` entity ID per
  `docs/server/observability.md`. Levels: ownership rejection
  (user error) is INFO/WARN, not ERROR; storage failure is ERROR.
  Use `connecterr.Internal(ctx, op, err, ...)` for internal errors
  so `scripts/check_codeinternal.js` stays green; use
  `connect.NewError(connect.CodeInvalidArgument, ...)` for
  ownership violations.
- **Server SQL efficiency.** Server-side response builders for
  contribution lists must batch-fetch linked gear in a single
  `GetByIDs` call across the visible contribution set — never per
  contribution. Wrap the contribution-response test with
  `storage.AssertMaxQueries(t, ctx, N, fn)` so N+1 regressions are
  caught at PR time.
- **Auto-RSVP on first claim (Experience scope only) is preserved.**
  Adding `gear_id` to `ClaimExperienceNeedRequest` does not change
  the Experience-scope-only "submit YES RSVP before the claim"
  behavior described in
  [docs/client/needs.md §"Intentional differences"](../client/needs.md#intentional-differences).
  The client wrapper around the new claim path keeps the existing
  pre-claim RSVP step. No new intentional-differences entry.
- **Parity contract.** Per
  [docs/client/needs.md](../client/needs.md), every behavior in this
  feature is identical across Experience and Request scopes. If an
  implementation step would diverge, that's a bug — surface it on
  the issue, don't ship the divergence.

## Implementation Plan

### Phase 1: Proto + server foundation

**Goal:** Land the data model so the client work can be unblocked. No
UX visible to users yet.

**Tasks:**
- [ ] Add `optional string gear_id = 13;` to
      [`PlanningContribution`](../../proto/ripls/models/planning.proto#L36).
      Use `optional` per `CLAUDE.md`§"Proto Field Conventions"
      (conditionally-present scalar).
- [ ] Add `optional string gear_id` to
      `ClaimExperienceNeedRequest`, `AddExperienceContributionRequest`,
      `EditExperienceContributionRequest`, and the three
      Request-scope equivalents in
      [`request_service.proto`](../../proto/ripls/api/request_service.proto).
- [ ] Add the same field to `ExperienceContributionResponse` /
      `RequestContributionResponse` so the client gets the link back
      on load.
- [ ] Server: when `gear_id` is set, validate that it exists and is
      owned by the calling user. Reject with
      `connect.NewError(connect.CodeInvalidArgument, ...)` on
      mismatch and log at INFO with `operation`, `user_id`, the
      offending `gear_id`, and a one-line reason. Use
      `connecterr.Internal(ctx, op, err, ...)` only for true
      internal failures (storage error, etc.). No count cap is
      needed — the proto admits at most one link.
- [ ] Server: persist `gear_id` on the contribution row through
      `storage.ProtoSQLStorage` (no raw SQL). `PlanningContribution`
      writes happen in
      [`server/services/experience/needs.go`](../../server/services/experience/needs.go#L259)
      and the request equivalent. Loading paths must batch via
      `GetByIDs` across the visible contribution set — never per
      contribution.
- [ ] Server: extend `EditExperienceContribution` /
      `EditRequestContribution` to accept a replacement `gear_id`
      (including explicit clear via unset).
- [ ] Unit tests using `storage.SetupTestStorage(t)`: ownership
      validation, edit-replace and edit-clear semantics, and a
      `storage.AssertMaxQueries` guard on the contribution-list
      builder to prove the batched-fetch path.
- [ ] Run `npm run build` (covers `golangci-lint` + `go build`),
      then `go build -tags=integration ./...` (per `CLAUDE.md`
      "Go Server Build Verification"). Also `npm run lint:go` to
      keep `check-goroutines` and `check-codeinternal` green.

**Validates:** the data model is sound, the proto convention rules
are honored, and the server enforces ownership before any UI ships.

### Phase 2: Library picker UI (full-library variant)

**Goal:** Owners of non-empty libraries can link an existing gear item
to a claim (single-select). Empty-library and capture paths come in
Phase 3 — this phase ships the picker for users who already have gear.

**Tasks:**
- [ ] New widget
      `app/lib/presentation/widgets/needs/needs_claim_gear_picker.dart`
      that takes the user's gear list plus an optional pre-selected
      `String? gearId` and returns the new selection. **Single-select**
      (radio-like), not multi-select. Composes
      [needs_sheet_chrome.dart](../../app/lib/presentation/widgets/needs/needs_sheet_chrome.dart);
      opened via `showAccessibleModal`. Rows use `Toggle` with
      `semanticsLabel` from `context.l10n.a11yNeedsClaimGearRow(name)`
      — selecting one row deselects the previous. Surface colors
      come from `AppColors.xxx(context)`.
- [ ] New reusable `GearPill` widget in
      `app/lib/presentation/widgets/gear/gear_pill.dart`. Built on
      `Tappable` with `semanticsLabel: context.l10n.a11yGearPill(name)`;
      tap → open gear detail via `NavigationHelpers.pushToItemScreen`
      (no raw `Navigator.push`). Gear thumbnail uses
      `CachedMediaImage` with a `semanticsLabel`. The dim
      "tombstone" variant adds an explicit
      `Semantics(label: context.l10n.a11yGearPillRemoved)` so
      `avoid_color_only_status` stays satisfied. Audit existing
      transfer/chat surfaces during implementation — if a `GearPill`
      already exists there under a different name, lift it rather
      than duplicate.
- [ ] Wire the picker into
      [`needs_picker_modal.dart`](../../app/lib/presentation/widgets/needs/needs_picker_modal.dart)
      NK3 confirm view: a new "Link gear" disclosure between the
      qty stepper and the note field. Default collapsed; expanded
      shows the picker rows.
- [ ] **Name-match search reusing the @-mention lookup.** Use
      `searchRepository.search` with
      `SearchStrategy.SEARCH_STRATEGY_EXACT`,
      `itemTypes: [SearchItemType.SEARCH_ITEM_TYPE_GEAR]`, and the
      parent entity's `communityIds`, mirroring
      [`ConversationViewModel.fetchMentionSuggestions`](../../app/lib/presentation/viewmodels/conversation_view_model.dart#L647).
      The query text defaults to the Need's title (auto-fill the
      "Suggested for this row" section); the user can override by
      typing. **Intersect results with
      [`gearRepositoryProvider.listUserGear()`](../../app/lib/data/repositories/gear_repository.dart#L59)
      to enforce owner-only** — search results that aren't in the
      user's library are dropped. Top section: search hits.
      Bottom section: the rest of the user's library, alphabetical.
      No new search code; no LLM (LLM extraction is already deferred
      per needs.md Deferred #6).
- [ ] Render a `GearPill` on
      [`needs_claim_row.dart`](../../app/lib/presentation/widgets/needs/needs_claim_row.dart)
      below the title line, between the title and the
      "added by {name}" line, when the contribution has a `gear_id`.
      Tombstone styling when the ID is set but the gear can't be
      resolved.
- [ ] Render the same pill on the read-only Archived sheet rows.
- [ ] Plumb `gear_id` through the claim/edit RPCs from
      [`experienceNeedsProvider`](../../app/lib/presentation/viewmodels/experience_needs_view_model.dart)
      and
      [`requestNeedsProvider`](../../app/lib/presentation/viewmodels/request_needs_view_model.dart).
      Preserve the Experience-scope auto-RSVP step that runs before
      first claim (it must still fire before the claim RPC; the new
      `gear_id` payload simply rides along).
- [ ] Surface RPC errors via `RpcErrorHandler.classify` →
      `UserError` on state; widget calls `RpcErrorHandler.localize`
      in `build`. ViewModels never resolve l10n strings.
- [ ] `ProviderContainer`-based ViewModel tests with Mockito mocks
      (`@GenerateMocks` on `GearRepository`, `SearchRepository`, and
      the needs service) covering: no selection, single-select
      switch between rows (selecting B clears A), edit-mode replace,
      edit-mode clear (unset `gear_id`), tombstone for unknown ID,
      search-result intersection with owned gear (results not in
      `listUserGear()` are dropped), cache-hit path
      (`verifyNever(service.listUserGear)` when repo cache is warm),
      and disposal safety (start claim future → dispose → expect
      `completes`).
- [ ] New ARB strings in both
      [`app_en.arb`](../../app/lib/l10n/app_en.arb) and
      [`app_es.arb`](../../app/lib/l10n/app_es.arb) with `@key`
      metadata. Keys: `needsClaimLinkGear`, `needsClaimGearPickerTitle`,
      `needsClaimGearPickerSuggestedHeader`,
      `needsClaimGearPickerOtherHeader`,
      `needsClaimGearRemovedTombstone`, plus `a11yNeedsClaimGearRow`,
      `a11yGearPill`, `a11yGearPillRemoved`. Run
      `npm run lint:dart:i18n` and `npm run lint:dart:l10n-parity`.

**Validates:** the picker fits the existing modal chrome, gear pills
render correctly across Volunteer/Archived sheets, and the
permission/ownership round-trip works end-to-end.

### Phase 3: Capture path (empty-library + opt-in)

**Goal:** Users with empty libraries can capture new gear inline; users
with full libraries get the secondary "Bringing something not in your
library yet" affordance from the prototype. The capture surface is the
**standard create-gear modal** — same camera / text / hyperlink input
modes as elsewhere in the app, with the in-modal switcher intact.

**Tasks:**
- [ ] Library-state detection in the ViewModel: pull the user's
      gear count via
      [`GearRepository.listUserGear()`](../../app/lib/data/repositories/gear_repository.dart#L59)
      (already cached under `gear:user:list`). If `0`, render the
      dominant "Snap your gear" card per the prototype empty-library
      variant; else render the picker with the secondary capture
      affordance below it. ViewModel computes the variant; the
      widget reads it from state (no Repository calls in widgets).
- [ ] On capture tap, launch the existing
      [`create_gear_modal.dart`](../../app/lib/presentation/screens/gear/create_gear_modal.dart)
      (which wraps the
      [`GenGearNotifier`](../../app/lib/presentation/viewmodels/gen_gear_view_model.dart)
      pipeline) inline, modal-stacked over the claim sheet via
      `showAccessibleModal`. **Do not fork to a camera-only entry
      point** — the existing modal already lets the user switch
      between camera, text-entry, and hyperlink/URL inputs, and that
      switcher must remain available from the claim flow. The
      empty-library card's "Snap your gear" copy is just the
      starting input mode, not a constraint.
- [ ] On successful `SaveGear`, call
      `gearRepository.refreshUserGear()` to invalidate the
      `gear:user:list` cache key, set the claim's pending `gear_id`
      to the returned ID, and return to the claim sheet.
- [ ] Toast on submit (per prototype): "Saved to library" appended to
      the standard contribution-saved toast when the capture path was
      used.
- [ ] Handle the partial-success window: if `SaveGear` succeeds but
      the claim RPC fails, dismiss the create-gear modal back to the
      claim sheet with the gear pre-selected so the user can retry
      without re-capturing.
- [ ] ViewModel tests (`ProviderContainer` + Mockito) for both
      library-state branches, all three input modes wired through
      the create-gear modal (camera, text, URL — at minimum verify
      that the modal is opened in its default state with mode
      switching enabled, not pre-locked to camera), the
      partial-success retry path, and cache invalidation
      (`verify(gearRepository.refreshUserGear)` after `SaveGear`
      succeeds). Disposal-safety test for the capture flow.
- [ ] ARB additions in both locales with `@key` metadata:
      `needsClaimSnapTitle`, `needsClaimSnapSubtitle`,
      `needsClaimAddOutsideLibrary`, `needsClaimSavedToLibraryToast`,
      plus `a11ySnapYourGear`. Run `npm run lint:dart:l10n-parity`.

**Validates:** the empty-library onboarding path turns the claim
moment into a library-growth moment, the capture seam is invisible
to the user, and the user retains the existing input-mode switcher
(camera / text / URL) without leaving the claim flow.

### Phase 4: Polish + telemetry

**Goal:** Catch the long tail before broader rollout. Optional —
treat as a 1-week cleanup once Phase 3 is dogfooded.

**Tasks:**
- [ ] Tombstone styling pass: confirm dim-pill is legible on both
      Volunteer and Archived sheets across light/dark using
      `AppColors` tokens only — no raw color literals.
- [ ] Analytics events via the existing analytics layer:
      `needs_claim_with_gear` (claim submitted with a `gear_id`
      set), `needs_claim_capture` (capture path used + which input
      mode landed: camera / text / URL),
      `needs_claim_tombstone_seen`. Confirm event names with the
      analytics owner before the PR lands.
- [ ] Re-run the `storage.AssertMaxQueries` guard added in Phase 1
      after the response builder has lived through real list
      sizes; tighten the bound if possible.
- [ ] Accessibility audit pass: every `GearPill` announces via
      `Tappable(semanticsLabel: …)`; tombstone variant carries an
      explicit `Semantics` annotation;
      `npm run lint:dart:a11y` clean with no allowlist additions.
- [ ] Update [docs/client/needs.md](../client/needs.md) to mention
      gear-linking under each surface; no new "Intentional
      differences" entry (parity holds).

**Validates:** no N+1 regressions, accessibility doesn't degrade, the
documentation stays current.

## Related Issues

- **#2228** — earlier framing of the Request-scope-only version of
  this feature. Empty body; this issue supersedes it. Close #2228 as
  "duplicate of #2240" when Phase 1 lands.
- **needs.md Deferred #3** (friend-graph hint) — gear-linking is the
  prerequisite. Once claims know which gear they reference, surfacing
  "Marcus has a 60m rope in his kit" becomes a server-side query, not
  a free-text guess.
- **needs.md Deferred #5** (saved kits / Picker rail) — same data
  shape. A "kit" is a saved set of `gear_ids`; this feature delivers
  the per-claim version of that primitive.
- **needs.md Deferred #7** (AddOptionGhost for non-requesters on
  Request scope) — orthogonal. Gear-linking should not gate on it; the
  picker is identical regardless of who opened it.
