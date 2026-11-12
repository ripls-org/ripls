# Issue #1647: Community preview modal: race seed, swallowed stream errors, dead params, missing tests (after #1634)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1647
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

> **Audit applied** (`/ripls-audit`, 2026-05-01) — confirmed alignment
> with `docs/client/testing.md` (ViewModel-first + `ProviderContainer`
> + Mockito) and `docs/client/i18n.md` ("Viewmodels never resolve
> strings"). Three refinements landed in the plan body:
> 1. Phase 2 explicitly cites `UserError.generic(fallback: …)` →
>    `RpcErrorHandler.localize(state.error!, context.l10n)` as the
>    boundary between ViewModel and widget; this is the canonical
>    pattern from `app/lib/core/errors/user_error.dart`.
> 2. Phase 1 keeps the existing hardcoded English strings in
>    `_errorMessage` (`"Community name is required"`,
>    `"Community description is required"`, `"Failed to create
>    community"`) as **out of scope**. They violate
>    `lint:dart:i18n` already; touching them here would mix
>    concerns. Track separately.
> 3. `ContentErrorBanner` already wraps its body in `LiveRegion`
>    (see `app/lib/presentation/widgets/content/content_error_banner.dart:34`),
>    so two stacked banners will be announced by screen readers
>    without additional `Semantics` plumbing.

> **Cross-modal audit applied** (reviewer ask, 2026-05-01) —
> `@claude Also look at GenExperience streaming which I think
> fixed this model race in the past as well. If there are other
> Gen* streaming modal races look for them and fix as well.`
> Findings are summarized in **Cross-Modal Audit** below; the
> recommendation now adopts Experience's pattern verbatim and
> the scope expands to also fix the swallowed-streaming-error
> bug in `RequestPreviewModal`.

## Problem Statement

After #1634 collapsed the Community creation flow onto the streaming path
(removing the unary `genCommunity` branch), four latent defects in the
preview-modal seam are now load-bearing:

1. **Race seed.** `community_creation_modal.dart:_handleGenerate` starts
   the stream (`generateCommunityStreaming()` calls `beginStreaming()`,
   subscribes via `GenStreamController.start(...)`, returns `true`)
   *before* `showDialog` mounts the preview modal. Any `title` /
   `description` / `final` event landing between subscription and
   `initState` writes to `genCommunityProvider` before the modal's
   `ref.listen` registers. `initState` then seeds `_editingName` /
   `_editingDescription` from `widget.title` / `widget.description`,
   which are always `''`. `StreamingFieldSlot` flips out of skeleton
   mode (because `state.name` is non-empty), revealing two empty
   `ContentEditableField`s. Improbable on slow AI calls today, certain
   on a future cache-hit fast path.

2. **Swallowed stream errors.** `failStreaming(message)` writes
   `genCommunityProvider.error` and `isStreaming=false`. The preview
   modal renders only `_errorMessage` (local state, only ever written
   by `_handleCreate`). A server-side AI failure flips the skeleton off,
   shows two empty fields with no error visible, and on tap surfaces
   `"Community name is required"` — misleading.

3. **Dead constructor parameters.** `CommunityPreviewModal` declares
   five required-or-nullable parameters (`title`, `description`,
   `mediaIds`, `confidence`, `reasoning`). Both call sites in
   `community_creation_modal.dart` pass constants (`'', '', null, 0.0,
   ''`). `widget.title` / `widget.description` only seed the (wrong)
   buffer; `widget.mediaIds` is read in three places as a fallback but
   is always `null` on the streaming entry path; `widget.confidence`
   and `widget.reasoning` are never read.

4. **No streaming-method test coverage.** `gen_community_view_model_test.dart`
   tests `setPreviewData` / `createCommunity` / `reset` only —
   `beginStreaming`, `applyStreamingTitle`, `applyStreamingDescription`,
   `applyStreamingMediaReady`, `finalizeStreaming`, `failStreaming`
   are untested. `community_creation_view_model_test.dart` has no
   integration test that drives `generateCommunityStreaming` against
   a fake stream — bugs 1 and 2 above would have been caught by even
   a minimal end-to-end exercise.

The reviewer expanded scope on 2026-05-01 to also audit the sibling
Gen* preview modals (Experience, Gear, Request) for the same bugs.
The audit found one additional bug to fix in this PR: Request's
preview modal swallows streaming-side errors the same way Community
does. See **Cross-Modal Audit** below.

## Cross-Modal Audit

`Experience`, `Gear`, and `Request` share the same architectural
shape as `Community`: a creation modal calls `generate*Streaming()`
on a notifier (which subscribes a `GenStreamController` and returns
immediately), then calls `showDialog(builder: (_) => const *PreviewModal())`.
Each preview modal owns an editing buffer that has to start in sync
with whatever events have already landed in the gen-provider before
mount.

The audit looked at four specific properties:

- **A: dead constructor parameters?** Does the modal accept
  initial `title` / `description` / etc. that the call sites
  always pass as constants?
- **B: race-safe seed?** Does `initState` read the gen-provider
  rather than `widget.*`? Either an immediate `ref.read` or a
  `WidgetsBinding.instance.addPostFrameCallback` read works.
- **C: defense-in-depth listener?** Does `build` `ref.listen` the
  gen-provider so events that arrive after mount also reach the
  buffer (and the buffer doesn't go stale once `isStreaming`
  flips false)?
- **D: streaming error visible?** Does the modal render the
  gen-provider's `error` (the streaming-side failure) above the
  action button — not just the create-RPC's local
  `_errorMessage`?

| Modal           | A: no dead params | B: race-safe seed                     | C: ref.listen sync           | D: streaming error visible          |
| --------------- | ----------------- | ------------------------------------- | ---------------------------- | ----------------------------------- |
| **Experience**  | ✅ `super.key`    | ✅ post-frame `ref.read`              | ✅ in `build`                | ✅ `state.error` → banner           |
| **Gear**        | ✅ `super.key`    | ⚠️ post-frame `ref.read`, but only seeds when `detectedGear != null` (terminal) | ✅ in `build` (covers partials) | ✅ `state.hasError` → banner        |
| **Request**     | ✅ `super.key`    | ✅ post-frame `ref.read`              | ✅ in `build`                | ❌ only local `_errorMessage`       |
| **Community**   | ❌ 5 params       | ❌ `widget.title` / `widget.description` | ⚠️ in `build`, but only fires while `isStreaming==true` and bails on `finalizeStreaming` | ❌ only local `_errorMessage` |

**Conclusion.** The reviewer's intuition is correct: the
`Experience` preview modal is the canonical fix and our north
star. `Gear` is in compliance via a slightly different shape
(its terminal payload carries everything in `detectedGear`, and
the `build`-time `ref.listen` is what actually catches partial
title/description events — not the post-frame seed). `Request`
has the race fix (B + C) but still has Bug 2 (D) — same shape
as Community's. `Community` is the only modal still failing on
A, and it fails on B and D.

**In-scope additions for this PR:**

- Fix Bug 2 in `RequestPreviewModal` (render
  `genRequestProvider.error` as a `ContentErrorBanner` above the
  action button, mirroring the Experience pattern).
- Add a regression test for Request's streaming-error path so
  the same bug doesn't recur.

**Explicitly out of scope** (deferred to follow-up issue):

- Gear's narrow seed condition (`detectedGear != null`). The
  `build`-time `ref.listen` covers the partial-event window today;
  the issue is theoretical and orthogonal to this PR.
- Generalizing the `addPostFrameCallback` + `ref.listen` pattern
  into a shared mixin or helper. Once Community lands on the same
  pattern, four modals all repeat the same six-line seed; that's
  the right time to extract.

## Context from Comments

Two relevant comments:

- **briangbrown (maintainer, 2026-05-01)**: `@claude create plan` —
  the original kickoff.
- **briangbrown (reviewer, 2026-05-01)**: `@claude Also look at
  GenExperience streaming which I think fixed this model race in the
  past as well. If there are other Gen* streaming modal races look
  for them and fix as well. Update the plan.` — drove the cross-modal
  audit and the Request fix added below.

The issue body is exhaustive — it describes the trace, points at the
exact lines, and proposes specific fixes for each bug. Treat the
description as the spec; this plan extends it with the cross-modal
audit, the Experience-derived recommendation, the Request fix, and
the test surface.

## Potential Solutions

### Option A: Apply the four community fixes + the one request fix, mirroring the Experience pattern (recommended)

- **Approach:** Use `Experience`'s preview-modal as the canonical
  template. Specifically:
  1. **Community modal — race seed (Bug 1 + Bug 3 prep).** In
     `_CommunityPreviewModalState.initState`, seed the editing
     buffers from `ref.read(genCommunityProvider)` inside a
     `WidgetsBinding.instance.addPostFrameCallback((_) { … })`
     (matches the Experience pattern at
     `app/lib/presentation/screens/experience/experience_preview_modal.dart:79`).
     Fall back to `''` only when the provider's name/description is
     null. The existing `build`-time `ref.listen` on
     `genCommunityProvider` stays — but adjust it to **also fire
     once after `isStreaming` flips false**, so that a `final` event
     that lands while the user hasn't typed still wins. The cleanest
     adjustment: drop the `if (!next.isStreaming) return;` guard and
     instead require `next.name != null` and `_editingName == priorName`
     (matches Experience's own listener at
     `experience_preview_modal.dart:334-353`).
  2. **Community modal — streaming error (Bug 2).** In `build`, watch
     `streamingError = ref.watch(genCommunityProvider).error;` and
     render a `ContentErrorBanner` for it above the action button via
     `RpcErrorHandler.localize(streamingError, context.l10n)`. Keep
     the existing `_errorMessage` banner for create-RPC failures —
     both may stack, distinct provenance.
  3. **Community modal — dead params (Bug 3).** Drop `title`,
     `description`, `mediaIds`, `confidence`, `reasoning` from
     `CommunityPreviewModal`. Source `mediaIds` from
     `genCommunityProvider` everywhere; drop the three `widget.mediaIds`
     fallbacks. Update both call sites in `community_creation_modal.dart`
     to `const CommunityPreviewModal()`.
  4. **Request modal — streaming error (Bug 2 in the audit).** In
     `_RequestPreviewModalState.build`, render a `ContentErrorBanner`
     for `ref.watch(genRequestProvider).error` above the existing
     `_errorMessage` banner. Keep `_errorMessage` for create-RPC
     errors, same pattern as Community Phase 2. Mirrors
     `experience_preview_modal.dart:418-422` exactly.
  5. **Tests (Bug 4).** Add unit tests for the six streaming methods
     on `GenCommunityNotifier` (mirroring the existing
     `setPreviewData` / `createCommunity` style with
     `ProviderContainer`). Add an integration test in
     `community_creation_view_model_test.dart` that drives
     `generateCommunityStreaming` with a fake
     `Stream<StreamGenCommunityResponse>` and asserts
     `genCommunityProvider` and `communityCreationProvider` state
     across title → description → media_ready → final, and across
     the error event. Add a parallel integration test in
     `request_creation_view_model_test.dart` for the
     streaming-error path (proves the gen-provider's `error` is
     populated; the widget test below proves the modal renders it).
- **Pros:** Each fix is local and matches the canonical Experience
  pattern, so the four modals converge in shape rather than diverging.
  No public-API change beyond a leaf-widget constructor signature.
  Catches a second instance of Bug 2 that the reviewer's audit
  surfaced. Test additions lock in regressions for both modals.
- **Cons:** Touches four production files (community_preview_modal,
  community_creation_modal, request_preview_modal, gen_community_view_model)
  and three test files. Faking
  `Stream<StreamGenCommunityResponse>` /
  `Stream<StreamGenRequestResponse>` requires constructing proto
  values, a few lines of helper boilerplate.
- **Effort:** Small-to-medium.

### Option B: Restructure to mount the modal *before* starting the stream

- **Approach:** Flip `_handleGenerate` so `showDialog` opens first with
  a "loading" state, and the modal itself kicks off
  `generateCommunityStreaming` in `initState`. That structurally rules
  out Bug 1 (the listener exists before the first event is emitted).
- **Pros:** Eliminates the race at the architectural level rather than
  papering over it with an `initState` read. Symmetric with sibling
  preview modals once they're audited.
- **Cons:** Larger refactor that ripples through `CommunityCreationNotifier`
  (state ownership of the stream moves). Doesn't fix Bug 2, Bug 3, or
  the Request bug on its own; tests still missing. Risk of introducing
  new races (e.g. modal disposed before stream completes). Bigger
  blast radius for a bug that today is theoretical on this device class.
  Moves Community **away** from the Experience pattern instead of
  toward it.
- **Effort:** Medium-large.

### Option C: Defer `showDialog` until first event arrives

- **Approach:** Have `generateCommunityStreaming` return a
  `Completer`/`Future` that resolves after the first non-trivial
  event, and only call `showDialog` after it resolves.
- **Pros:** Avoids the race without touching the modal.
- **Cons:** Breaks the streaming UX premise — the whole point of the
  skeleton is to mount the modal *before* events arrive. Does not
  address Bug 2, 3, 4, or the Request bug.
- **Effort:** Small, but wrong direction.

## Recommendation

**Option A.** The reviewer's audit confirms that Experience's
preview modal already solved this set of problems; the cleanest fix
is to make Community match Experience verbatim (post-frame
`ref.read` seed + relaxed `ref.listen` guard + streaming-error
banner + zero-param constructor) and to apply the same
streaming-error banner to Request. After this PR, Community,
Experience, and Request use the identical four-property pattern
(rows A–D in the audit table). Gear stays as-is — its `build`-time
`ref.listen` already covers the partial-event window, and its
`detectedGear`-only seed is theoretical-only.

The judgment call inside Option A is how to render two error
sources (streaming vs. create-RPC) on Community and Request:

- **Stacked (recommended).** Render two `ContentErrorBanner`
  widgets stacked when both are present. Distinct provenance (one
  before user action, one after). Matches the Experience pattern
  loosely (Experience only has the streaming banner because its
  create-path errors flow through the same `state.error`). Safer:
  no information is lost.
- **Coalesced.** Render a single banner that prefers the create-RPC
  error when it exists. Cleaner UI, but the streaming error becomes
  invisible once the user taps create.

This plan adopts the **stacked** approach. If review prefers the
merged form, swap during implementation — both are local changes.

## Open Questions

- **How should the create button behave when `state.error` is non-null
  from the streaming path?** Recommendation: keep it enabled. The user
  may still know what they want and have typed values into the now-empty
  fields. The validation guards in `_handleCreate` already block empty
  submissions. The streaming error banner is informational, not blocking.
  Same answer for Request.
- **Should the Gear preview modal's narrow `detectedGear != null` seed
  be tightened to also seed from `editedName`/`editedDescription`?**
  Recommendation: no, not in this PR. The `build`-time `ref.listen`
  catches the partial-event window today (lines 185–204 of
  `gear_preview_modal.dart`). File a follow-up if a regression surfaces.
- **Should we delete the `mediaIds` fallback in `_handleCreate` /
  `_buildCommunityDetails` when removing `widget.mediaIds`?**
  Recommendation: yes. The streaming path is the only entry path; the
  fallback was dead code per the issue. The provider is now the single
  source of truth for media on the preview surface.
- **Where does the integration-style streaming test live?**
  Recommendation: extend the existing `community_creation_view_model_test.dart`
  and `request_creation_view_model_test.dart` rather than creating new
  files. The notifiers are the System Under Test; the fake stream is
  a thin helper. Mirrors the existing ViewModel-first testing
  convention in `docs/client/testing.md`.
- **Should we extract the post-frame seed + `ref.listen` defense
  pattern into a shared mixin once four modals all share it?**
  Recommendation: yes, but in a follow-up. Land Community-mirrors-
  Experience first; then four call sites makes the abstraction
  cheap and obvious.

## Implementation Plan

### Phase 1: Fix race seed + dead params on the community modal

**Goal:** Eliminate Bug 1 and Bug 3. After this phase the modal sources
all displayed state from `genCommunityProvider`, the constructor has
zero parameters, and the seed pattern matches Experience verbatim.

**Tasks:**

- [ ] In `app/lib/presentation/screens/communities/community_preview_modal.dart`:
    - [ ] Remove `title`, `description`, `mediaIds`, `confidence`,
      `reasoning` from `CommunityPreviewModal` and its constructor.
    - [ ] In `initState`, seed buffers via
      `WidgetsBinding.instance.addPostFrameCallback((_) { … })`,
      reading
      `_editingName = ref.read(genCommunityProvider).name ?? ''` and
      `_editingDescription = ref.read(genCommunityProvider).description ?? ''`.
      Wrap state mutation in `setState` so the post-frame seed
      re-renders. (Mirrors `experience_preview_modal.dart:75-103`.)
    - [ ] Loosen the existing `build`-time `ref.listen` on
      `genCommunityProvider`: drop the
      `if (!next.isStreaming) return;` guard so that `final` events
      (which flip `isStreaming` to false) still mirror into the
      editing buffer when the user hasn't typed. Keep the
      "user-typed wins" guard (`_editingName == priorName`).
      Mirrors `experience_preview_modal.dart:334-353`.
    - [ ] In `_showMediaPickerDialog`, drop the `widget.mediaIds`
      branch from `hasMedia`.
    - [ ] In `_handleCreate`, drop the `widget.mediaIds` fallback;
      pass `state.mediaIds` directly to `setPreviewData`.
    - [ ] In `build`, drop the `?? widget.mediaIds?.firstOrNull`
      fallback from `effectiveMediaId`.
- [ ] In `app/lib/presentation/screens/communities/community_creation_modal.dart`:
    - [ ] Replace both
      `CommunityPreviewModal(title: '', description: '', mediaIds: null, confidence: 0.0, reasoning: '')`
      sites with `const CommunityPreviewModal()`.
    - [ ] Confirm `_handleManualCreate` still seeds correct empty
      state — since `genCommunityProvider` was reset on the prior
      generation attempt, but for a cold manual entry it may carry
      stale values from a previous flow. Add a
      `genCommunityProvider.notifier.reset()` call at the top of
      `_handleManualCreate` to mirror `_handleGenerate`.

**Validates:** Manual run on a slow device (or with a deliberate delay
inserted between `controller.start(stream)` and `showDialog`)
confirms the modal renders the streamed name as soon as it's available.
`flutter analyze` passes.

### Phase 2: Surface streaming errors in the community modal

**Goal:** Eliminate Bug 2 in Community. A server-side AI failure
during generation is visible above the action button. Pattern
mirrors `experience_preview_modal.dart:417-422`.

**Tasks:**

- [ ] In `_CommunityPreviewModalState.build`, read
  `final streamingError = ref.watch(genCommunityProvider).error;`
  alongside `communityState`.
- [ ] Above `_buildActionButtons`, render a banner for
  `streamingError` when set:
  ```dart
  if (streamingError != null) ...[
    ContentErrorBanner(
      errorMessage: RpcErrorHandler.localize(streamingError, context.l10n),
    ),
    const SizedBox(height: 16),
  ],
  ```
  Keep the existing `_errorMessage` banner immediately after; both
  may be visible simultaneously.
- [ ] Confirm `_handleCreate` still clears its own `_errorMessage`
  on retry; do **not** clear `genCommunityProvider.error` from the
  modal — that's owned by the streaming lifecycle.

**Validates:** Inject a fake stream that emits the `error` event;
the modal shows the localized error banner.

### Phase 3: Surface streaming errors in the request modal

**Goal:** Eliminate the audit-discovered Bug 2 in `RequestPreviewModal`.
A server-side AI failure during request generation is visible above
the action button. Same pattern as Phase 2; same call to
`RpcErrorHandler.localize`.

**Tasks:**

- [ ] In `app/lib/presentation/screens/request/request_preview_modal.dart`,
  inside `build`, read
  `final streamingError = ref.watch(genRequestProvider).error;`
  alongside the existing `state` watch.
- [ ] Above the existing `_errorMessage` banner block (currently
  around `request_preview_modal.dart:407-411`), render a
  streaming-error banner:
  ```dart
  if (streamingError != null) ...[
    ContentErrorBanner(
      errorMessage: RpcErrorHandler.localize(streamingError, context.l10n),
    ),
    const SizedBox(height: 16),
  ],
  ```
- [ ] Do **not** modify `_handleCreate` — its
  `finalState.hasError` branch is the create-RPC path and is
  orthogonal to the streaming-error display.

**Validates:** Manual injection of a fake `error` event into the
request stream surfaces the localized banner above the action
button. Existing `_errorMessage` banner still shows for create-RPC
failures.

### Phase 4: Tests

**Goal:** Eliminate Bug 4. Lock in regressions for Bugs 1 and 2 in
Community and the audit-found bug in Request.

**Tasks:**

- [ ] Extend `app/test/presentation/viewmodels/gen_community_view_model_test.dart`
  with a `streaming methods` group:
    - [ ] `beginStreaming` clears name/description/mediaIds/error and
      sets `isStreaming=true`.
    - [ ] `applyStreamingTitle('Foo')` writes `state.name = 'Foo'`.
    - [ ] `applyStreamingTitle('')` is a no-op.
    - [ ] `applyStreamingDescription('Bar')` and
      `applyStreamingDescription('')` mirror the title behavior.
    - [ ] `applyStreamingMediaReady(['m1'])` writes mediaIds; empty
      list is a no-op.
    - [ ] `finalizeStreaming` with non-empty title/description/mediaIds
      overwrites partial state, clears `isStreaming`, leaves `error`
      null.
    - [ ] `finalizeStreaming` with empty title falls back to existing
      `state.name` (round-trip after `applyStreamingTitle`).
    - [ ] `failStreaming('boom')` sets a non-null `state.error` and
      clears `isStreaming`.
- [ ] Extend `app/test/presentation/viewmodels/community_creation_view_model_test.dart`
  with an `integration: generateCommunityStreaming` group:
    - [ ] Helper:
      `_fakeCommunityStream(List<StreamGenCommunityResponse>)`
      yielding events from a
      `StreamController<StreamGenCommunityResponse>` that the test
      can drive. Override `communityRepositoryProvider` to return a
      `MockCommunityRepository` whose `streamGenCommunity` returns
      the helper's stream.
    - [ ] Happy path: drive title → description → media_ready → final.
      After the stream closes, assert
      `container.read(genCommunityProvider)` carries the final values
      and `isStreaming == false`. Assert
      `container.read(communityCreationProvider).isLoading == false`
      and `generatedCommunity` is set.
    - [ ] Race-seed regression: drive title → final, then snapshot
      `genCommunityProvider`; assert it holds the final values
      *before* a hypothetical late subscriber. The test demonstrates
      that the buffers a freshly-mounted modal would seed from
      `ref.read(genCommunityProvider)` carry the streamed data.
    - [ ] Error path: emit an `error` event. Assert
      `genCommunityProvider.error` is non-null and
      `communityCreationProvider.error` is non-null;
      `isLoading == false`; `isStreaming == false`.
    - [ ] Transport error path: invoke the `onError` branch (close
      the source stream with an error). Assert the same end-state.
- [ ] Extend `app/test/presentation/viewmodels/request_creation_view_model_test.dart`
  with a parallel `integration: generateRequestStreaming` error
  group (the happy-path coverage may already exist; only the error
  paths are new for this PR):
    - [ ] Helper:
      `_fakeRequestStream(List<StreamGenRequestResponse>)` driven
      by a `StreamController<StreamGenRequestResponse>`. Override
      `requestRepositoryProvider` similarly.
    - [ ] Error path: emit an `error` event. Assert
      `genRequestProvider.error` is non-null and
      `requestCreationProvider.error` is non-null;
      `isLoading == false`. (The Phase 3 widget code path then
      renders this `genRequestProvider.error` as a banner.)
    - [ ] Transport error path: close the source with an error;
      assert the same end-state.
- [ ] Run
  `cd app && flutter pub run build_runner build --delete-conflicting-outputs`
  if new `@GenerateMocks` annotations require it.
- [ ] Run `cd app && flutter test --reporter expanded` and confirm
  green.

**Validates:** All four acceptance items in the issue body are
covered by automated tests for Community; Request's
streaming-error path is covered as well; future regressions on
either streaming seam fail at test time.

## Related Issues

- #1634 — collapsed creation modals onto the streaming path; this
  issue is direct fallout.
- Sibling preview modals (Experience, Gear) — audited in this plan.
  Experience is the canonical fix; Gear is in compliance via a
  slightly different shape. No code changes needed for either.
  Request's swallowed-streaming-error fix is now folded into this
  PR per the reviewer's 2026-05-01 ask.
- Future follow-ups (file after this lands): (a) extract the
  post-frame seed + `ref.listen` defense pattern into a shared
  mixin once Community joins Experience and Request; (b)
  optionally tighten Gear's `detectedGear`-only initial seed.
