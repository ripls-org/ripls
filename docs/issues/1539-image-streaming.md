# Issue #1539: Wire streaming for Gear-image and Request-image creation flows

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1539
**Filed:** 2026-04-28 | **Last activity:** 2026-04-28
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Gear and Request creation in image mode are the last two genAI flows still on the legacy unary path. Both modals show the four-stage `LoadingOverlay` ("Parsing your prose / Divining location / Temporal translation / Assembling your adventure") for the entire ~3-8 second AI round trip, while the user can do nothing but stare at it. Text-mode versions of the same RPCs already stream — preview UI opens immediately with `StreamingFieldSlot` skeleton placeholders, fields populate as `StreamGenGearResponse` / `StreamGenRequestResponse` events arrive, the Save button enables on the terminal `final` event.

The original issue framing implied a server-side gap ("Streaming RPC already exists … wire the existing streaming RPC into the gear creation viewmodel"). Investigation confirms **the gap is entirely in the Flutter client**:

- The proto `StreamGenGearRequest` (gear_service.proto:406-413) accepts `media_id` (line 408).
- The proto `StreamGenRequestRequest` (request_service.proto:544-551) accepts `media_id` (line 550) with the comment: "Provide either prompt (text mode) or media_id (image mode), but not both."
- Server-side `streamGenGearFromImage` (gear/gen_ai_streaming.go:115-211) is implemented and dispatched on `hasMedia` (line 108).
- Server-side `streamGenRequestFromImage` (request/gen_streaming.go:215-) is implemented and dispatched on `hasMedia` (line 89-90).

What's missing on the client:

1. **`app/lib/data/repositories/gear_repository.dart#streamGenGear()`** (line 217) takes `prompt`, `websiteUrl`, `locationId`, `latitudeDeg`, `longitudeDeg` — **no `mediaId`**.
2. **`app/lib/data/repositories/request_repository.dart#streamGenRequest()`** (line 186) likewise lacks `mediaId`.
3. **`app/lib/presentation/viewmodels/gen_gear_view_model.dart`** has `generateGearFromTextStreaming()` (line 379) but no image-mode streaming variant. Image mode flows through `uploadImage()` → `generateGear()` (line 494) which calls `_gearRepository.genGearFromMedia(mediaId)` — **unary**.
4. **`app/lib/presentation/viewmodels/request_creation_view_model.dart`** has `generateRequestStreaming()` (line 200) for text mode only.
5. **`app/lib/presentation/screens/gear/create_gear_modal.dart#_handleGenerate()`** (line 99-125) image-mode branch always goes unary.
6. **`app/lib/presentation/screens/request/request_creation_modal.dart`** (line 114-115) has an explicit comment: *"Streaming path: only supported in text mode. Image mode falls through to the existing unary flow below until the streaming RPC is extended."* That comment is now stale — the RPC was extended; the client wiring was never finished.

## Context from Comments

No comments yet — this is a fresh issue I filed today. The Problem Statement above synthesizes everything that would normally come from comments (proto verification, server confirmation, stale-comment discovery). Flagging the absence of external discussion as low risk: the streaming pattern has shipped for three other genAI flows (Experience text/image/webpage, Gear text/webpage, Request text), so reviewers have rehearsed context.

## Potential Solutions

### Option A: Mirror the text-mode streaming functions verbatim (recommended)

- **Approach:** Add `mediaId` parameter to `streamGenGear()` / `streamGenRequest()` on the service + repository layers. Add `generateGearFromImageStreaming(mediaId)` to `gen_gear_view_model.dart` and `generateRequestFromImageStreaming(mediaId)` to `request_creation_view_model.dart`, each a near-copy of the existing text-mode streaming method with the same event-handler switch and the same skeleton state population. Modify each modal's `_handleGenerate()` image-mode branch to: upload image → kick off the new streaming method with the resulting `mediaId` → immediately open the preview modal (don't await full completion) when `Environment.genStreamingEnabled` is true.
- **Pros:** Zero new abstractions. The duplication between text-mode and image-mode streaming is real but already exists at the server layer (separate `streamGenGearFromText` and `streamGenGearFromImage` handlers); matching that shape on the client makes the layers symmetric and keeps each method's call site obvious. Reviewers diff three nearly-identical functions and recognize the pattern from the prior #1456 / #1502 PRs.
- **Cons:** ~300 lines of near-duplicate Dart between text and image variants per surface. Some of the duplication is genuinely necessary (different state-update fields between text and image — image mode also pre-populates `selectedMediaIds`).
- **Effort:** small. ~3-4 hours including manual device testing.

### Option B: Unify text + image streaming into a single viewmodel method

- **Approach:** Replace `generateGearFromTextStreaming()` with `generateGearStreaming({String? prompt, String? mediaId, String? websiteUrl})` that picks the input source from whichever parameter is set. Same on request side.
- **Pros:** Less duplication. One streaming function per surface instead of three.
- **Cons:** Breaks the recently-shipped text-mode signatures, expanding blast radius for what is meant to be a small additive change. Image mode genuinely needs different state-update logic on the terminal event (preserves `selectedMediaIds` from the upload step rather than letting `final` overwrite it; pre-fills the image preview before the AI call lands), so the function body would still branch internally — net code count is similar, just hidden behind a wider signature.
- **Effort:** small-to-medium. ~5 hours including refactoring the existing text-mode call sites and re-testing all three modes (text, image, webpage).

### Option C: Defer until #1540 lands and unify all three streaming surfaces (gear, request, community) at once

- **Approach:** Hold #1539 until #1540 (Community streaming) ships, then refactor all three creation viewmodels into a shared `StreamingCreationViewModel<T>` mixin or base class.
- **Pros:** One coherent abstraction across all genAI surfaces. Future surfaces would inherit the streaming machinery.
- **Cons:** Couples #1539 timing to #1540, which is a larger lift (server + proto + service). Premature abstraction risk — generalizing across three surfaces before having three concrete examples is a known footgun. Doesn't unblock #961 cleanup any sooner.
- **Effort:** large.

## Recommendation

**Option A.** The text-mode streaming code is recent (≤4 weeks old, multiple PRs) and the repetition is the pattern reviewers will recognize. The image-mode variants need separate state-update logic anyway; bundling them into a single method (Option B) hides that branching behind a parameter sniff. Option C delays a small, well-scoped fix in pursuit of an abstraction that may not justify itself even with three concrete examples — wait until the duplication actually hurts before generalizing.

Tradeoff being made: ~300 lines of near-duplicate Dart in exchange for code that reads obviously and ships fast.

## Architecture Compliance Notes

(Audit pass — items called out so reviewers see them up-front.)

- **MVVM layering preserved.** Phase 1 modifies the data layer (service + repository); Phase 2 + 3 modify viewmodels (Notifiers) and screens. No layer-skipping. Per `docs/client/architecture.md` §Architecture Layers: ViewModels are the only layer that calls Repositories; Repositories are the only layer that calls Services.
- **Async-first.** All proposed methods are `Future<bool>` (viewmodel) or `Stream<StreamGenXxxResponse>` (repository). No sync APIs introduced.
- **Caching.** AI generation is intentionally cache-bypassed — `gear_repository.dart:213-215` documents `streamGenGear()` as "Generation is one-shot and unique per prompt/URL; this is a cache-bypass pass-through." Image-mode inherits the same property: `media_id` is the variable input, but the AI output is non-deterministic so caching the response would be wrong. After the user saves via `SaveGear` / `SaveRequest` mutations, the standard repository invalidation flow runs unchanged.
- **No i18n surface.** No new user-visible strings. The skeleton placeholders use the existing `StreamingFieldSlot` / `StreamingSkeleton` widgets, which already render via `context.l10n`. The only text change is the removal of a Dart code comment in `request_creation_modal.dart:114-115`.
- **Async-safety pattern.** The new viewmodel methods must use `safeUpdateState((s) => s.copyWith(...))` for every post-async state update — same as the existing `generateGearFromTextStreaming()` (gen_gear_view_model.dart:430, 437, 442, 447, 452, 485) and `generateRequestStreaming()`. `gen_gear_view_model.dart` already mixes in `SafeNotifierMixin`; the new methods inherit that lifecycle protection. After every `await` in the modal handlers, retain the existing `if (!mounted) return;` guards per `docs/client/architecture.md` §Widget Async Exceptions (navigation results are an explicit exception that warrants the manual check).
- **Streaming controller cleanup.** The existing pattern uses `_streamController?.dispose()` at method entry (gen_gear_view_model.dart:386) and `ref.onDispose` for terminal cleanup. The new image-mode methods reuse the same `_streamController` field — only one stream in flight per viewmodel — so no new disposal hooks are needed.

## Open Questions

- **Should the gear-image streaming kick off immediately after the upload completes (auto-progress, mirrors current unary flow) or wait for an explicit user tap?** — Recommendation: auto-progress, same as today. The user has already tapped the camera/upload button; making them tap again to "generate" would regress UX. The existing unary flow (`uploadImage()` → `generateGear()`) auto-progresses; preserve that.
- **Where should the streaming-vs-unary fallback live for image mode? Inline at the modal site like text mode, or push it down into the viewmodel?** — Recommendation: inline at the modal, matching text mode. Keeps the flag-checking logic in one place per surface and lets the unary path continue to exist as a fallback that doesn't require viewmodel awareness of the flag.
- **The unary `generateGear()` viewmodel method does extra steps (sets `currentStep`, fills `selectedMediaIds` from `detectedGear.mediaIds`, etc.). Should the streaming variant exactly reproduce these state updates, or only the subset that drives the preview UI?** — Recommendation: reproduce them. Anything the preview modal reads from state during the post-streaming preview must be set or the preview will glitch. Diff the unary `generateGear()` (gen_gear_view_model.dart:494) and `generateGearFromTextStreaming()` (line 379) and copy the union into the new image-streaming method.
- **Should the stale modal comment at request_creation_modal.dart:114-115 be removed in this PR or left for the cleanup PR (#961)?** — Recommendation: remove in this PR. It's literally the comment that says the work this PR does isn't done yet.
- **Eval suite changes needed?** — Recommendation: none for the server changes (none happen). Add a single integration test on the Dart side that asserts the streaming variant fires `media_ready` and `final` for an image-mode call — mirrors `gen_ai_streaming_test.go` patterns server-side.

## Implementation Plan

### Phase 1: Repository + service Dart layer

**Goal:** Plumb `mediaId` through to the gRPC streaming call site so the rest of the work has the wire to write to.

**Tasks:**
- [ ] `app/lib/services/gear_service.dart#streamGenGear()`: add `String? mediaId` parameter, set `request.mediaId = mediaId` when non-null.
- [ ] `app/lib/data/repositories/gear_repository.dart#streamGenGear()` (line 217): add `String? mediaId` parameter, forward to service.
- [ ] `app/lib/services/request_service.dart#streamGenRequest()`: same change.
- [ ] `app/lib/data/repositories/request_repository.dart#streamGenRequest()` (line 186): same change.
- [ ] `flutter analyze` clean. No callers should need updates yet (callers continue passing only text-mode params).

**Validates:** the proto field is reachable from Dart and correctly serialized.

### Phase 2: Gear-image streaming viewmodel + modal

**Goal:** Image-mode gear creation no longer shows `LoadingOverlay` when the flag is on; preview opens immediately, fields populate as events arrive.

**Tasks:**
- [ ] `gen_gear_view_model.dart`: add `Future<bool> generateGearFromImageStreaming(String mediaId)`. Body: copy `generateGearFromTextStreaming()` (line 379), swap the repository call to `streamGenGear(mediaId: mediaId)`, preserve the existing `selectedMediaIds: [mediaId]` pre-population from `onMediaUploaded()` (line 170) so the image preview renders before AI lands. **Use `safeUpdateState` for every post-async state mutation** (matches the text-mode method's pattern via `SafeNotifierMixin`).
- [ ] `create_gear_modal.dart#_handleGenerate()` (line 99-125): when `Environment.genStreamingEnabled` is true and `inputMode == CreationInputMode.image`, after `uploadImage()` completes successfully, call `generateGearFromImageStreaming(state.uploadedMediaId!)` instead of letting `uploadImage()` auto-call the unary `generateGear()`. Open the preview modal immediately on success (mirrors text-mode flow at line 88). Retain the existing `if (!mounted) return;` guards after each await — this is the documented widget-async exception per `docs/client/architecture.md`.
- [ ] **Disposal test** (per architecture doc §Disposal Testing): add a unit test in `app/test/presentation/viewmodels/gen_gear_view_model_test.dart` that starts `generateGearFromImageStreaming(...)` and disposes the `ProviderContainer` mid-flight; assert no exception is thrown and the stream subscription is cleaned up.
- [ ] **Event coverage test**: assert `media_ready`, `title`, `description`, and `final` events each update state correctly (mirrors existing text-mode tests).
- [ ] Verify that on Anthropic per-token streaming, title + description partial events render the gear name + blurb in the preview before `final` lands. Verify on Gemini lite. Verify on OpenAI gpt-5-mini (which buffers — preview will appear with skeletons that flip to populated near end-of-call; same as text-mode today).
- [ ] Manual device test: Pixel 8 (per the existing `ripls-app-logs` skill) — capture photo, confirm preview opens with skeleton UI immediately, fields populate mid-stream, save button enables on terminal event.
- [ ] No regression on text-mode gear creation (existing tests + manual smoke).

**Validates:** the streaming pattern transfers cleanly from text to image mode for gear; `LoadingOverlay` is bypassed in the image-mode happy path; disposal under flight is graceful.

### Phase 3: Request-image streaming viewmodel + modal

**Goal:** Same as Phase 2, but for request creation. Smaller blast radius since request-image's modal already has the explicit "until the streaming RPC is extended" comment marking the exact change site.

**Tasks:**
- [ ] `request_creation_view_model.dart`: add `Future<bool> generateRequestFromImageStreaming(String mediaId)` mirroring `generateRequestStreaming()` (line 200). Use `safeUpdateState` for post-async state mutations (same pattern as Phase 2; check the existing text-mode method for the `SafeNotifierMixin` mix-in).
- [ ] `request_creation_modal.dart#_handleGenerate()` (line 114-140): drop the `state.inputMode == CreationInputMode.text` restriction on the streaming branch; route image mode through the new viewmodel method when the flag is on. Remove the now-stale comment at line 114-115.
- [ ] **Disposal test + event coverage test** in `app/test/presentation/viewmodels/request_creation_view_model_test.dart` (mirrors Phase 2).
- [ ] Manual device test (Pixel 8): same checklist as gear-image.
- [ ] No regression on text-mode request creation.

**Validates:** all four genAI surfaces (Experience text/image/webpage, Gear text/image/webpage, Request text/image, Community pending #1540) consistently bypass the loading overlay when streaming is enabled.

### Phase 4: Cleanup hooks

**Goal:** Set up the precondition for #961's overlay deletion.

**Tasks:**
- [ ] Cross-reference #961 in the PR description: note that gear-image and request-image image-mode paths now stream, so the only remaining `LoadingOverlay` consumers on genAI paths are Community creation (pending #1540) and any unary fallback path for users with `Environment.genStreamingEnabled = false`.
- [ ] Confirm `Environment.genStreamingEnabled` is `true` in both `env.dev.json` and `env.prod.json` (per #1229 — should already be the case as of `56122cd33`).
- [ ] Update CLAUDE.md or `docs/client/architecture.md` only if the streaming pattern is documented elsewhere; otherwise no doc update needed (the pattern is self-evident in code).

**Validates:** the chain to #961 closure is unobstructed.

## Related Issues

- **#961** — parent cleanup issue. This PR is a precondition for deleting `loading_stage_indicator.dart` and the genAI branches of `loading_overlay.dart`.
- **#1540** — Community streaming end-to-end. Independent of this PR; together they close out all genAI streaming migrations.
- **#1456** — streaming UX gaps for Experience image/webpage. Established the per-token skeleton pattern this issue applies to gear and request.
- **#1502** — text-mode streaming refactor. Defines the viewmodel + modal shape this issue mirrors for image mode.
- **#1241** — original streaming work. Background on the field-event pipeline.
- **#1226** — schema consolidation. Recently merged; no overlap with this issue but worth being aware of since image-mode shares the same canonical schema infrastructure.
