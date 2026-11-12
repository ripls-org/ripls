# Issue #1540: End-to-end streaming for Community creation

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1540
**Filed:** 2026-04-28 | **Last activity:** 2026-04-28
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Community creation is the only remaining genAI creation flow on the legacy unary path. When a user describes the community they want to create, the four-stage `LoadingOverlay` ("Parsing your prose / Divining location / Temporal translation / Assembling your adventure") blocks the entire screen for the full ~3-8 second AI round trip. Experience, Gear, and Request creation have all migrated to streaming — preview modal opens immediately with `StreamingFieldSlot` skeleton placeholders, fields populate as `StreamGen*Response` events arrive, save button enables on the terminal `final` event. Community needs the same treatment.

The work spans server (provider interface + 3 implementations + canonical schema + new RPC + service handler), client (service + repository + viewmodel + modal), and eval (new prompt suite). Each piece has a recently-shipped template — Request streaming is the closest analog since both Community and Request use a text-only prompt with no Date/Time/LocationQuery/ValueEstimate, and both fan out to Pexels stock images via SearchKeywords.

A subtle finding from codebase research that shapes the plan: **community image-mode is currently a sham on the client**. `community_repository.dart:47-60` uploads the image, **throws away the mediaId**, and falls back to generating from a generic prompt: `"Generate a community based on the provided image"`. There's a TODO at line 55-56 to plumb a real `genCommunityFromMedia` RPC. So the modal has an "image mode" toggle but only text mode actually exercises the AI's understanding. This issue should preserve the sham (text-stream the generic prompt when image mode is selected) and file a separate follow-up to make image mode functional, rather than silently expanding scope.

A second subtlety: **`community_creation_view_model.dart` uses a plain `Notifier` without `SafeNotifierMixin`** (unlike `gen_gear_view_model.dart` and `request_creation_view_model.dart`, which both mix it in). The streaming method needs disposal-safe state updates. Either mix in `SafeNotifierMixin` for this PR or use manual `if (!ref.mounted) return;` checks throughout. The mix-in is the cleaner choice and is documented as the "transitional tool" in `docs/client/architecture.md` §Async Safety.

## Context from Comments

No comments — fresh issue I filed today after investigating #961. The body of the issue itself is detailed and reflects the full scope discovered during investigation; nothing was added by other maintainers.

## Potential Solutions

### Option A: Mirror Request streaming end-to-end (recommended)

- **Approach:** Build the streaming machinery for Community by faithfully copying the Request-streaming surface — same proto event shape (`title`, `description`, `media_ready`, `final`, `error`), same canonical schema pattern, same provider streaming methods, same service handler structure (Pexels fan-out fires the moment SearchKeywords closes), same client viewmodel + modal wiring. Preserve the existing image-mode sham (image upload + generic prompt) on the streaming path; file a follow-up to make image mode functional.
- **Pros:** Reviewer context warm — Request streaming shipped recently and the community shape is structurally identical (text-only, SearchKeywords drives Pexels). Low surprise for client devs (the modal pattern already exists three times). Preserves all existing tests.
- **Cons:** Continues the image-mode sham; users who pick image mode get text-streaming a generic prompt, not actual image analysis. Follow-up is needed to fix the sham — but that work is out of scope here.
- **Effort:** medium. Server (schema + 3 providers + RPC + handler + drain-wrapper) is the bulk; client + eval is small per-piece.

### Option B: Same as A, plus fix the image-mode sham in the same PR

- **Approach:** Add `media_id` to `StreamGenCommunityRequest`, implement real image-mode streaming for community (parallel to gear-image and request-image), drop the generic-prompt fallback in the repository.
- **Pros:** No follow-up issue; #961 cleanup unblocked completely.
- **Cons:** Doubles the scope. Image-mode community generation needs new prompt engineering (the existing `buildCommunityGenerationPrompt` is text-only) and provider streaming methods that handle the image input, plus eval coverage for image cases. PR review burden grows. Not blocked by anything here — can ship as a separate PR after this one and #1539 land.
- **Effort:** large.

### Option C: Skip Community streaming entirely; just delete the LoadingOverlay branches in #961

- **Approach:** Argue that community creation is a low-frequency action (most users never create a community; existing communities are joined) so the loading overlay's UX cost is low. Just delete the four-stage indicator and replace with a plain `CircularProgressIndicator` for community. Skip the streaming migration.
- **Pros:** Smallest diff. No server changes. Unblocks #961 cleanup.
- **Cons:** Inconsistent UX — users would see streaming skeletons for Experience / Gear / Request creation but a plain spinner for Community. Three of four genAI surfaces stream; the fourth being unary is a wart. The pattern is well-rehearsed at this point so there's no efficiency argument for skipping it.
- **Effort:** small.

## Recommendation

**Option A.** The work is mechanical and the pattern is rehearsed three times over. Option B doubles scope chasing a separate problem (image mode) that's been broken since the modal was first built — that's its own ticket. Option C ships an inconsistency that the team will have to clean up later anyway.

Tradeoff being made: Community image mode stays a sham post-PR (text-streams a generic prompt). File a follow-up issue tagged `genai-image-mode` to make image mode actually use the uploaded image.

## Open Questions

- **Should the streaming branch handle image mode, or should the modal fall back to the unary path for image mode?** — Recommendation: streaming branch handles both, but routes image mode through the same generic-prompt sham. The viewmodel already does this transparently in the unary path via `_communityRepository.genCommunity({prompt, imagePath})`; mirror that in the streaming path. Image mode just sends the generic prompt as if it were text. The user still sees the streaming UX (preview opens fast, skeletons, etc.).
- **Should `community_creation_view_model.dart` be migrated to `SafeNotifierMixin` as part of this PR?** — Recommendation: yes. It's a small mix-in addition (4 chars) plus replacing manual `state = state.copyWith(...)` with `safeUpdateState((s) => s.copyWith(...))`. The streaming method needs the disposal protection anyway; might as well do the legacy unary method at the same time so the viewmodel stays consistent. Per `docs/client/architecture.md` §Async Safety this is the documented transitional pattern.
- **Should `GenCommunityResponse` gain a `reasoning` field on the streaming `final` event?** — Recommendation: yes, mirror the unary response (`gen_community_response.proto:150-151`). The streaming RPC's `final` event should carry the same `GenCommunityResponse` proto so the client can drain the stream into the existing response handler unchanged.
- **Pexels fan-out — fire on first SearchKeywords or on every emission?** — Recommendation: first emission only, matching Request streaming behavior at `gen_streaming.go:151-169`. The streaming parser closes `search_keywords` once when the JSON array closes; if the AI emits a partial then a complete list, the parser surfaces the complete one via the StreamFieldEvent. One Pexels call per stream.
- **What region should be passed to the AI when image mode is the input?** — Recommendation: same as text mode — the user's primary location resolved via `s.resolveRegion()`. Image-mode community creation has no region context of its own.
- **Should we add an Anthropic tool description for `community_generation` distinct from the existing `community_generation` unary tool?** — Recommendation: reuse the same tool name + same schema. The streaming and unary paths emit identical tool inputs; the only difference is whether the SDK is asked for a streaming response.

## Architecture Compliance Notes

(Audit pass — items called out so reviewers see them up-front. Same dimensions as the #1539 audit, plus Community-specific gaps.)

- **MVVM layering preserved.** Phase 1 (server) is invisible to client. Phase 2 modifies the data layer (service + repository) before the viewmodels. Phase 3 modifies viewmodels and screens. Phase 4 is eval/test. Per `docs/client/architecture.md` §Architecture Layers: ViewModels are the only layer that calls Repositories; Repositories are the only layer that calls Services.
- **Async-first.** All proposed methods are `Future<bool>` (viewmodel) or `Stream<StreamGenCommunityResponse>` (repository). No sync APIs.
- **Caching.** AI generation is intentionally cache-bypassed — `community_repository.dart:43` already uses `_service` directly for `genCommunity`. The new streaming method does the same. After the user saves via `CreateCommunity`, the existing cache invalidation flow runs unchanged. The community **GET** caches (community list, community-by-id, member lists) are unaffected — they're populated post-save, not during generation.
- **i18n surface — likely small but non-zero.** Skeleton placeholders themselves use the existing `StreamingFieldSlot` widget (no new strings). However, if Phase 3 adds any user-visible labels (e.g. "Generating community…", "Save when ready", retry buttons in the streaming-error path), each new key needs entries in **both `app_en.arb` AND `app_es.arb`** with `@key` metadata, per `docs/client/i18n.md`. Run `npm run lint:dart:i18n` before commit.
- **Async-safety pattern.** New viewmodel streaming method must use `safeUpdateState` for every post-async state mutation. Recommend migrating the existing unary `generateCommunity()` method to `safeUpdateState` at the same time (see Open Question above) so the viewmodel is consistent. Per `docs/client/architecture.md` §Async Safety, `SafeNotifierMixin` is the documented transitional tool for legacy `Notifier` classes not ready for full `AsyncNotifier` migration.
- **Disposal testing.** Architecture doc §Disposal Testing requires disposal tests for all async Notifiers. The new `generateCommunityStreaming()` method must have a test that disposes the `ProviderContainer` mid-flight and asserts no exception. Already in Phase 3 task list.
- **Server/client boundary.** Proto change is additive (new RPC). Existing `GenCommunity` unary RPC is preserved as a drain wrapper around the streaming handler — same pattern Experience / Gear / Request use. Wire compat fully preserved.
- **Per CLAUDE.md "Never reuse RPC request or response message types across multiple RPCs":** `StreamGenCommunityRequest` and `StreamGenCommunityResponse` must be NEW message types, not reused from `GenCommunityRequest` / `GenCommunityResponse`, even though they carry the same fields.
- **Service Independence (server arch §Core Design Principles):** the community streaming handler uses `s.aiProvider`, `s.stockImageryProvider`, `s.storage` — shared libraries, not other services. ✓
- **File-size enforcement.** `community_creation_view_model.dart` is 152 lines today; adding ~80 lines for `generateCommunityStreaming()` lands around ~230 — comfortably under the 1000-line `lint:dart:size` cap from CLAUDE.md. No file split needed.
- **Test runner convention.** Per CLAUDE.md, run Flutter tests with `--reporter expanded`, not the default compact reporter. Apply to the disposal + happy-path tests added in Phase 3.

### Verified gap: `community_preview_modal.dart` does NOT use StreamingFieldSlot today

Confirmed by inspection: `community_preview_modal.dart` (395 lines) currently has zero `StreamingFieldSlot` / `StreamingSkeleton` references. By contrast, `request_preview_modal.dart` and `gear_preview_modal.dart` each use them three times. The current Community preview was built for the unary flow — it expects a fully-populated `GenCommunityResponse` and renders it.

This makes Phase 3 work somewhat heavier than just "add streaming to the modal":
- Each rendered field (Title, Description, image) needs to be wrapped in a `StreamingFieldSlot` so it shows a skeleton when the corresponding state field is null/empty.
- The Save button's `onPressed` needs to gate on `state.isLoading == false` (or equivalent terminal-event flag) so users can't save before the stream finalizes.
- Verify `community_preview_modal.dart` reads from `genCommunityProvider`/`communityCreationProvider` reactively (via `ref.watch`) so partial state lights up fields incrementally.

This is the riskiest unknown in the plan. Recommend reading `request_preview_modal.dart` line-by-line as the reference template before starting Phase 3.

## Implementation Plan

### Phase 1: Server canonical schema + Provider streaming methods

**Goal:** All three AI providers can stream community generation; the canonical schema is the single source of truth; the watch list is derived mechanically.

**Tasks:**
- [ ] Add `communityGenerationOutput` to `server/ai/schemas.go`. Field order (early-fire bias): `title`, `search_keywords`, `description`, `confidence`. No `omitempty` on any field (OpenAI strict-mode rule from #1226).
- [ ] Add `GenerateCommunityContentStreaming(ctx, prompt, region) (<-chan FieldEvent, <-chan CommunityStreamFinal, error)` to the `Provider` interface in `server/ai/provider.go`.
- [ ] Add `CommunityStreamFinal { Result *CommunityGeneration; Err error }` to `server/ai/streaming_provider.go`.
- [ ] Add `communityStreamingKeys = streamingKeysFromSchema[communityGenerationOutput]()` to `server/ai/streaming_provider.go`. Includes `title`, `description`, `search_keywords`.
- [ ] Implement on Anthropic (`provider_anthropic_streaming.go`) — copy `GenerateRequestContentStreaming` shape, swap to `communityGenerationOutput` and `buildCommunityGenerationPrompt(prompt, region)`. Tool name: `community_generation` (reuse the unary tool name).
- [ ] Implement on OpenAI (`provider_openai_streaming.go`) — copy the OpenAI Request-streaming shape, swap schema + prompt, response_format name `community_generation`.
- [ ] Implement on Gemini (`provider_gemini_streaming.go`) — copy Gemini Request-streaming shape, swap schema + prompt.
- [ ] Mock / Fallback / LoadBalanced wrappers gain the streaming variant. Use `runUnaryCommunityStream` adapter for Mock parity (mirrors `runUnaryRequestStream` in `streaming_provider.go`).
- [ ] Existing unary `GenerateCommunityContent` becomes a drain wrapper that opens the streaming variant, drains the field channel into `/dev/null`, reads the final, returns the result. Same drain pattern Experience / Gear / Request unary methods use today.
- [ ] `go build ./server/...` clean. `go build -tags=integration ./server/...` clean. `npm run lint:go` clean.

**Validates:** the streaming AI plumbing works for community at the provider layer — can be smoke-tested with an integration test that prints the streaming events to verify ordering.

### Phase 2: Server proto + service handler

**Goal:** Public-facing `StreamGenCommunity` RPC emits skeleton-friendly events; the existing `GenCommunity` unary RPC continues to work as a drain wrapper.

**Tasks:**
- [ ] Add to `proto/ripls/api/community_service.proto`:
  - New message `StreamGenCommunityRequest { string prompt; string region; }`. **Do not reuse `GenCommunityRequest`** per CLAUDE.md.
  - New message `StreamGenCommunityResponse { oneof event { string title; string description; MediaReady media_ready; GenCommunityResponse final; GenStreamError error; } }`. Match the `StreamGenRequestResponse` field-numbering convention.
  - New service RPC `rpc StreamGenCommunity(StreamGenCommunityRequest) returns (stream StreamGenCommunityResponse);`.
- [ ] `npm run generate` clean.
- [ ] Create `server/services/community/gen_streaming.go` mirroring `server/services/request/gen_streaming.go`:
  - `StreamGenCommunity(ctx, req, stream) error` — auth, sender, `streamGenCommunityCore` dispatch.
  - `streamGenCommunityCore(...)` — opens AI streaming call, fan-outs Pexels on SearchKeywords closing, drains, emits `final`. Description equals the user's prompt verbatim, emitted before the AI's first delta (mirrors Request — text mode uses prompt as description, see `gen_streaming.go:102`).
  - `communityEventSender` wrapping `streaming.Sender[api.StreamGenCommunityResponse]` with `emitTitle`, `emitDescription`, `emitMediaReady`, `emitFinal`, `emitError` helpers.
  - Use the existing `findAndStoreStockImage` helper for the Pexels fan-out — same per-stream copy semantics as the unary path.
- [ ] Convert existing `GenCommunity` unary handler in `server/services/community/gen.go` to a drain wrapper: open the streaming handler, drain field events to `/dev/null`, take the final from the terminal event, return the assembled `GenCommunityResponse`. (This is the same shape Experience / Gear / Request unary handlers ended up with after #1456.)
- [ ] Add `gen_streaming_test.go` mirroring `server/services/request/gen_streaming_test.go`: a `captureCommunityTarget` that records emitted events, plus tests for event ordering (title before final, description emitted immediately, media_ready fires on Pexels success, terminal `final` always last).
- [ ] `npm run build` clean.

**Validates:** the streaming RPC works end-to-end on the server; the unary RPC continues working unchanged from the client's perspective.

### Phase 3: Client service + repository + viewmodel + modal

**Goal:** Community creation modal opens preview immediately with skeleton placeholders; fields populate as stream events arrive; `LoadingOverlay` is bypassed when `Environment.genStreamingEnabled` is true.

**Tasks:**
- [ ] `app/lib/services/community_service.dart`: add `Stream<StreamGenCommunityResponse> streamGenCommunity({required String prompt, String? region})`. Mirror the existing `streamGenRequest` shape.
- [ ] `app/lib/data/repositories/community_repository.dart`: add `Stream<StreamGenCommunityResponse> streamGenCommunity(...)` as a cache-bypass pass-through (genAI is non-deterministic). Mirror `streamGenRequest` doc comment.
- [ ] `app/lib/presentation/viewmodels/community_creation_view_model.dart`:
  - Add `with SafeNotifierMixin<CommunityCreationState>` to the class.
  - Replace existing manual `state = state.copyWith(...)` calls in the unary `generateCommunity()` with `safeUpdateState((s) => s.copyWith(...))`.
  - Add `Future<bool> generateCommunityStreaming()` mirroring `generateRequestStreaming()` (request_creation_view_model.dart:200). Image mode: continue uploading the image but pass a generic prompt — preserve the existing sham so this PR doesn't expand scope.
  - Add `_streamController` field + `ref.onDispose` cleanup.
- [ ] `app/lib/presentation/screens/communities/community_creation_modal.dart`:
  - Mirror `request_creation_modal.dart#_handleGenerate()`: when `Environment.genStreamingEnabled` is true, call `generateCommunityStreaming()`, immediately open `community_preview_modal.dart`, return early. Legacy unary fallback preserved when flag is off.
  - The `if (state.isLoading) LoadingOverlay(...)` block at line 176-178 stays in place for the legacy path; the streaming branch returns before the build re-evaluates `state.isLoading` so the overlay never shows on the streaming path.
- [ ] `app/lib/presentation/screens/communities/community_preview_modal.dart`: **structural change, not a small tweak** — the modal currently has zero `StreamingFieldSlot` references and was built for the unary flow's fully-populated response. Wrap Title / Description / image renderers in `StreamingFieldSlot` so they show skeletons when the bound state field is null/empty. Gate the Save button's `onPressed` on the terminal-event flag (`state.isLoading == false` or equivalent) so the user can't save before stream finalizes. Read state reactively (`ref.watch`) so partial events light up fields incrementally. Reference template: `request_preview_modal.dart` (3 `StreamingFieldSlot` call sites at lines 529, 549, 581).
- [ ] `flutter analyze` clean.
- [ ] Unit tests: add `community_creation_view_model_test.dart` smoke tests for the streaming method (empty input, text mode happy path with mocked stream, disposal-during-flight per architecture-doc §Disposal Testing). Run with `--reporter expanded` per CLAUDE.md.
- [ ] If any new user-visible strings are introduced (skeleton labels, retry copy, error messages), add to **both `app_en.arb` AND `app_es.arb`** with `@key` metadata, then `npm run generate`. Run `npm run lint:dart:i18n` before commit.

**Validates:** all four genAI surfaces (Experience / Gear / Request / Community) consistently bypass the loading overlay when streaming is enabled.

### Phase 4: Eval suite + manual smoke test

**Goal:** Confirm the streaming path's AI quality matches the unary path across all three providers; document the pass-rates.

**Tasks:**
- [ ] Create `server/ai/eval/community_prompt_test.go` mirroring `request_prompt_test.go`. Test cases: 5-8 prompts spanning explicit / vague / named / regional / etc. Assertions on title, description-non-empty, search_keywords-non-empty, confidence threshold.
- [ ] Run the full eval suite (`-mode=all` style) against Anthropic / OpenAI / Gemini. 85% aggregate pass-rate gate.
- [ ] Append an entry to `docs/evals/1157-genexperience-latency.md` documenting the post-streaming numbers and any pre-existing flakes that surface.
- [ ] Manual device smoke test (Pixel 8 per `ripls-app-logs` skill): describe a community in text, confirm preview opens immediately, fields stream in, save button enables on terminal event. Repeat with image mode (preserved sham — should still feel responsive even though it text-streams a generic prompt).
- [ ] No regression on the existing genAI flows (Experience / Gear / Request) — quick smoke since shared schema infrastructure could surface unexpected interactions.

**Validates:** quality gate passes; community streaming UX meets the same bar as the other three flows.

## Related Issues

- **#961** — parent cleanup issue. Once this PR + #1539 land, `loading_stage_indicator.dart` and the genAI branches of `loading_overlay.dart` become dead code on every flow.
- **#1539** — gear-image and request-image streaming. Independent of this PR; can ship in either order. Together they close out all genAI streaming migrations.
- **#1456** — streaming UX gaps for Experience image/webpage. Established the per-token skeleton pattern that this PR applies to Community.
- **#1502** — text-mode streaming refactor. Defines the viewmodel + modal shape this PR mirrors.
- **#1241** — original streaming work. Background on the FieldEvent pipeline.
- **#1226** — Gen* schema consolidation. Pattern for the canonical `communityGenerationOutput` schema; same `valueEstimateOutput` infrastructure won't be needed here (Community has no value estimate).
- **(future) genai-image-mode** — separate follow-up to make Community image-mode functional. Out of scope here; the existing sham is preserved in the streaming path.
