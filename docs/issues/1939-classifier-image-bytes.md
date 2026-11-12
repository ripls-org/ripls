# Issue #1939: Wire image bytes through the unified-create classifier prompt

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1939
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none (issue body is self-contained; this plan pulls from existing per-type image methods on `ai.Provider` and the unified-create stream_gen flow)

## Audit findings (applied 2026-05-15)

Cross-checked against `docs/server/architecture.md` and `docs/server/observability.md`.

1. **Phase 1 must specify how `unified_create.Service` gets bucket access.** The plan as drafted said "resolve media ID to a presigned URL (mirror the pattern at `server/services/gear/gen_ai.go:240-253`)" but didn't explain that the `unified_create` service today has no bucket dependency. Per `docs/server/architecture.md` § Pattern 3 (Constructor Injection), the bucket must be injected via the service constructor — not pulled from a global. Adds a constructor-signature change to `unified_create.New(...)` and a wiring change in `server/main.go`. Surfaced explicitly so an implementer doesn't reach for a package-global.
2. **Phase 1 needs a unit test for the new resolution path.** Originally the plan only said `go test ./...` clean. Per `docs/server/architecture.md` § Comprehensive Test Coverage ("No new functionality should be added without test coverage"), we need a unit test in `server/services/unified_create/stream_gen_test.go` covering the image-mode resolution branch. The existing `mockClassifier` test fixture can capture what it received; the test asserts `Image != nil && Image.ImageURL != ""`.
3. **Observability fields on the new resolution step.** Per `docs/server/observability.md` § Standard Field Names, the resolution log line should carry `operation`, `media_id`, `duration_ms`. The unified-create stream table in `observability.md` (lines ~772–797) should gain a `media_id` row noting it's emitted on image-mode streams.
4. **Defensive sensitive-data note.** Presigned bucket URLs carry time-limited credentials in their query string. The plan was silent on logging them. Per `docs/server/observability.md` § Sensitive Data Protection: **do not** log the full presigned URL — only `media_id` (which is the entity ID, not the URL). Explicit task added to Phase 1.
5. **Error-code mapping for fetch failures.** `server/services/gear/gen_ai.go:243` uses `connecterr.Internal(ctx, op, err, ...)` because it's a direct Connect handler. The unified-create stream_gen is a stream handler that uses `emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_*, ...)` instead — `connecterr.Internal` doesn't apply here. The right code is `GEN_STREAM_ERROR_CODE_INTERNAL` (or `GEN_STREAM_ERROR_CODE_MEDIA_FETCH_FAILED` if it exists; check `proto/ripls/api/gen_stream.proto` before adding a new enum value). Added to Phase 1.
6. **`Image *DetectionImage` REPLACES `MediaID string` on the input struct.** Originally the plan added `Image` alongside `MediaID`, leaving two parallel signals for image mode. After review: once `Image` carries the actual carrier (URL + bytes + mime type), `MediaID` provides no information the classifier consumes — the entity ID is only useful for logging, which happens at the caller (`stream_gen.go` knows `msg.GetMediaId()` and logs it on the resolution span). Keeping both fields means callers can disagree with themselves (which one wins?), and the audit's original "defensive check `MediaID != "" && Image == nil`" task becomes unnecessary when there's only one field. Every consumer of `MediaID` (`server/services/unified_create/classifier.go:50` stub validator, the 3 provider `case in.MediaID != "":` switches, the eval-runner `classifierInputForCase`) needs to migrate to `in.Image != nil`.
7. **Mock strategy.** `MockProvider.ClassifyUnifiedCreate` (`server/ai/provider_mock.go`) currently captures `ClassifyUnifiedCreateCall.Input` — after replacing `MediaID` with `Image`, the captured call still records the image presence on the new field. No mock plumbing changes needed. Per `docs/server/architecture.md` § Mock Strategy ("Define mocks in the library package") this is already in the right place.
8. **No goroutine-safety implications.** Sequential resolution, no fan-out. The `logging.GoSafe` requirement from § Goroutine Safety doesn't apply.

Items 1–6 added substantive plan tasks; items 7, 8 are clarifications added so an implementer doesn't have to re-derive them.

## Problem Statement

Image-mode classification on the unified-create flow is currently a coin flip biased toward "gear". The classifier's `Provider.ClassifyUnifiedCreate` method takes a `UnifiedCreateClassifierInput.MediaID`, but every provider's image-mode branch (Anthropic at `provider_anthropic.go:391`, Gemini at `provider_gemini.go:321`, OpenAI at `provider_openai.go:382`) only uses that signal to *select* a text-only prompt template (`buildUnifiedCreateClassifierImagePrompt`) — the actual image bytes are never resolved or attached to the API call. The model is asked to classify "the attached image" but no image is attached.

Empirically, during the #1932 baseline eval:

- **Gemini `3.1-flash-lite`** returns `gear` confidently because the prompt says "default to gear for product photography of an owned item."
- **Anthropic `claude-haiku-4-5`** does the same.
- **OpenAI `gpt-5-mini`** is honest: returns `request` with confidence near 0, notes like *"No image was provided; please upload the image so I can classify it."*

OpenAI's behavior is the correct read of the actual input. The other two providers are pattern-matching the prompt.

The production impact: every image-mode unified-create request silently misclassifies non-gear photos. An event flyer, a "lost dog" sign, or a help-wanted poster all come back as `gear`. The user has to flip the type-selector chip every time, which works (the `force_type` re-stream path is fast) but the auto-classification is providing no value.

The fix path is straightforward and well-precedented: every other multimodal AI surface on this server (`DetectGearInImage`, `GenerateExperienceFromImage`, `GenerateRequestFromImage`) already resolves `media_id` → bytes/URL → `*DetectionImage` → provider-native image block in the API call. The unified-create classifier just needs to do the same.

## Context from Comments

No comments on the issue yet. The issue body is detailed and three-option ranked. The user (during the planning request that produced this doc) reaffirmed: "we should actually be passing the image to the classifier for image classification queries." So the direction is settled — this plan is about the *how*.

## Potential Solutions

The issue body proposed three options; this plan re-examines them now that I've read the relevant call sites in detail.

### Option A — Reuse `*DetectionImage` as the carrier (recommended by issue body)

- **Approach:** Replace `MediaID string` with `Image *DetectionImage` on `UnifiedCreateClassifierInput` — single field, no parallel signals. Each provider's `ClassifyUnifiedCreate` image branch switches on `in.Image != nil`, reuses the existing image-block builder (`buildAnthropicImageBlock` / `buildGeminiImagePart` / `buildOpenAIImageContent`) and adds the image alongside the prompt text in the API call. The unified-create caller (`server/services/unified_create/stream_gen.go`) resolves `media_id` → presigned URL → `*DetectionImage{ImageURL: presigned, MimeType: media.ContentType}` (same shape as `server/services/gear/gen_ai.go:240-253`) before calling the classifier. The entity ID is logged at the caller (`media_id` field on the resolution log line) rather than threaded through to the classifier.
- **Pros:**
  - Reuses three battle-tested helpers (`buildAnthropicImageBlock`, `buildGeminiImagePart`, `buildOpenAIImageContent`) that already handle the bytes-vs-URL, localhost-fetch, and mime-type cases.
  - `*DetectionImage` is the canonical multimodal-input type across the AI surface; no new representation introduced.
  - Backward-compatible: existing callers and tests that don't set `Image` keep working (existing image-mode behavior is the no-image-attached classifier we're fixing, so "doesn't break" matters only for tests).
- **Cons:**
  - `UnifiedCreateClassifierInput` grows one field. Trivial.
- **Effort:** small-to-medium. Surface area is ~5 files (input struct + 3 providers + caller) + eval-runner update.

### Option B — Pass bytes directly on `UnifiedCreateClassifierInput`

- **Approach:** Add `ImageBytes []byte` + `ImageMimeType string` to the input. Each provider builds the image block from bytes only (skipping the URL path).
- **Pros:** No nil-pointer ambiguity ("is `Image` nil or zero-valued?"). Bytes-only is simpler to reason about.
- **Cons:** Loses the URL path that the existing helpers support. Forces the caller to resolve URL → bytes when the provider's SDK could have fetched a URL directly. Wastes bandwidth on the common case (Anthropic + OpenAI both accept URL image blocks natively for non-localhost URLs).
- **Effort:** small.

### Option C — Change `Provider.ClassifyUnifiedCreate` signature

- **Approach:** `ClassifyUnifiedCreate(ctx, in, image *DetectionImage) (*Classification, error)` — image as a method-level parameter alongside the input struct.
- **Pros:** Method signature reflects the conceptual shape (image is a structurally different input than text).
- **Cons:** Breaks the `Provider` interface contract. All four leaf implementations + `FallbackProvider` + `LoadBalancedProvider` + `MockProvider` + the `unified_create.providerClassifier` adapter need to change. Cascading mock + test edits. Bigger blast radius than necessary for a single-field addition to one method's input.
- **Effort:** medium.

## Recommendation

**Option A.** Confirms the issue's ranking. The `buildAnthropicImageBlock` / `buildGeminiImagePart` / `buildOpenAIImageContent` helpers do exactly what we need and handle localhost vs. signed-URL vs. raw-bytes already. Option B loses the URL path for no gain; Option C bloats the interface for no semantic clarity gain.

The implementation also gets a side benefit: the unified-create caller fetches the presigned URL **once** for both the classifier call and the downstream per-type generator (which also takes a `*DetectionImage`). Today the classifier doesn't fetch and the per-type call fetches independently. After this change, both share one presign — one network round-trip saved per image-mode request.

## Open Questions

- **Should the image-mode classifier prompt drop "default to gear"?** That instruction was a workaround for the no-image situation; with real bytes flowing the model should read the photo and answer honestly. — **Recommendation:** yes, drop it. Replace `buildUnifiedCreateClassifierImagePrompt` with a short instruction that just says "classify what the image depicts" without leaning the model toward gear. Bump `UnifiedCreateClassifierPromptVersion` to `v3` so the eval trail is clean.
- **What about the goldens corpus?** Currently 6 image cases, all `expected_type: gear` (since that's what the model emits without seeing the image). Once we wire the bytes through, the model's answers will reflect actual image content. — **Recommendation:** add 4–6 image cases for non-gear types (flyers, posters, "lost dog" signs) sourced from existing fixtures or fresh photos. Keep the gear-photo cases too — they validate that product photography still classifies correctly. Update the gear-photo `expected_type` cases unchanged; add the new non-gear ones.
- **Should the per-type call reuse the fetched `*DetectionImage`?** Today `stream_gen.go` doesn't fetch the image at all (classifier ignores it) and the per-type image method fetches its own presigned URL. — **Recommendation:** thread the `*DetectionImage` through the unified_create dispatcher so the per-type image call reuses it. Saves one fetch per request. Marginal latency win, real cost-of-API-call (Cloud Storage signed-URL fetch is cheap but not free). Folded into Phase 1 since it's the same `stream_gen.go` edit.
- **Eval cost.** Each image-mode benchmark run now sends the full image bytes (potentially 1–5MB) instead of an empty payload. At 6 image cases × 3 providers, that's 18 image uploads per eval run. — **Recommendation:** acceptable. The full eval is run on-demand and on nightly schedules, not per-PR. The fixtures under `server/test_data/` are tens-to-hundreds of KB each.
- **Backward compatibility for callers that pass `MediaID`.** *(Resolved during planning: this question disappears because `MediaID` is removed in Phase 1, not coexisting with `Image`. Every consumer of `MediaID` is migrated in the same commit; there is no period during which a caller can pass the old field.)*
- **Image-mode accuracy gate.** With bytes flowing, OpenAI's accuracy should jump from 1/6 (its honest-failure rate) to something matching Anthropic and Gemini. — **Recommendation:** track per-provider image-mode accuracy in the EVAL_SUMMARY output (add a `image_accuracy` field?) so a regression here is visible. Or accept that the overall accuracy gate is sufficient.

## Implementation Plan

### Progress

| Phase | Status | Commit |
|---|---|---|
| Phase 1 — Replace MediaID + resolve in caller | Done | `5d5f1fde4` |
| Phase 2 — Per-provider image-block construction | Done | `e2ede93a2` |
| Phase 3 — Goldens corpus expansion + measurement | Done | `7a75319a0` + `dba67304a` (lost-pet + flyers) + `ade70f3e3` (babysitter) + `36814f0f2` (charity) + `016fb3002` (results) |
| Phase 4 — Design doc + follow-up filing | Done | `9991bee70` |
| Phase 5 — Thread image forward + (attempted) Anthropic cache_control | Partially shipped | `c59b18e8f` + `90749b95a`; cache_control work reverted in `6ac62b5e3` |

### Phase 5 notes

**What shipped (image threading, commit `c59b18e8f`).** The unified-create dispatcher's resolved `*DetectionImage` is now passed forward to the per-type image call via a new `Image *ai.DetectionImage` field on each per-type Context (`experiencegen.Context`, `geargen.Context`, `requestgen.Context`). The per-type service uses it when set, skipping its own `storage.GetByID` + `bucket.GetSignedURL`. Single-create callers (direct per-type RPCs) pass `nil` and the existing resolve-from-mediaID path runs unchanged. Net effect: one DB lookup + one signed-URL generation saved per image-mode unified-create request, ~tens of ms of server time per request.

**Test coverage:** new `TestStreamGenUnifiedCreate_ImageMode_ThreadsImageToPerTypeCall` uses a `capturingGearGen` fake to assert the per-type call sees the exact same `ImageURL` as the classifier.

**What shipped (paired-latency benchmark scaffold, commit `90749b95a`).** A `//go:build benchmark` test in `server/ai/eval/paired_image_call_test.go` that exercises the classifier+per-type sequence on every image-mode goldens case and emits `EVAL_PAIRED_LATENCY` summary lines per (provider, model). Permanent infra for future paired-call measurement work.

**What was attempted and reverted (Anthropic cache_control, commits `0fcfe453a` + `6ac62b5e3`).** The original Phase 5 framing was that threading the same `*DetectionImage` forward would let the per-type Anthropic call hit a prompt cache primed by the classifier — billing image tokens at ~10% of the normal rate. Implemented as a `WithCacheControl` field on `ai.DetectionImage` + `cache_control: { type: "ephemeral" }` markers on the Anthropic image block. Live measurement showed every Anthropic call logging `cache_creation_input_tokens=0` and `cache_read_input_tokens=0` — the cache never engaged.

**Root cause:** Anthropic prompt caching is prefix-based (order: `tools → system → messages`). The classifier and per-type calls use entirely different tools (`unified_create_classification` vs. `gear_detection` / `experience_generation` / `request_generation`), so the cached prefix is already different before the message content. No amount of message-level reordering rescues a prefix that's mismatched at the tools section. cache_control markers were correctly serialized on the wire — they just had nothing to cache against.

Documented in detail on issue #1952 with a follow-up framing decision noted (either restructure to share tools — a separate architectural issue — or accept that paired-call cache reuse isn't viable on Anthropic).

**What stays preserved post-revert:**

- Image-threading optimization on the unified-create dispatcher (`c59b18e8f`).
- The `Image *DetectionImage` field on each per-type generator's `Context`.
- The per-type services' fast-path branch that uses pre-resolved images.
- The paired-image-latency benchmark scaffold (`90749b95a`).
- Block-order change on the Anthropic classifier (`[image, text]` to match the per-type methods — minor codebase consistency, neutral functionality).

**What's gone post-revert:**

- `WithCacheControl` field on `ai.DetectionImage` (commit `6ac62b5e3`).
- Manual `ImageBlockParam` construction in `buildAnthropicImageBlock` (back to using SDK convenience constructors).
- `WithCacheControl: true` assignment in `resolveMediaForClassifier`.
- The `-paired-image-cache-control` benchmark flag.

### Phase 3 notes

Landed: 2 event-type image cases (`experience.jpg`, `community.jpg`) reusing existing per-type-eval fixtures. The image-mode caveat docstring in `server/ai/eval/unified_classifier.go` was rewritten to reflect that bytes now flow through.

**Not landed (deferred to a follow-up):** request-type image fixtures (lost-pet signs, "looking for X" notices, free-curbside-stuff) and explicit event-flyer/poster shots. The `server/test_data/` collection today is product photos + two activity scenes; non-gear / request-photo fixtures don't exist there and can't be synthesized. Capturing those is a human-with-camera task; tracked as a soft follow-up — file a separate issue once we have the photos. Without them, Phase 4's live eval will measure event-scene vs. gear-photo discrimination but won't measure request-photo accuracy.

### Phase 1: Replace `MediaID` with `Image *DetectionImage` + resolve bytes in the caller

**Goal:** The unified-create caller fetches the presigned URL once, constructs `*DetectionImage`, and passes it on the classifier input. Migrate every consumer of `MediaID` to the new field shape in the same commit so there's no period when both fields exist on the wire. No provider behavioral changes yet — the providers switch on `in.Image != nil` instead of `in.MediaID != ""` and pick the same image prompt template; they don't yet attach the image bytes to the API call.

**Tasks:**
- [ ] **Replace** `MediaID string` with `Image *DetectionImage` on `UnifiedCreateClassifierInput` (`server/ai/classifier_unified.go`). Single field; no parallel signals.
- [ ] **Migrate the three provider switches.** Each of `provider_anthropic.go` / `provider_gemini.go` / `provider_openai.go` `ClassifyUnifiedCreate` has a `case in.MediaID != "":` selector for the image-mode prompt. Change to `case in.Image != nil:`. Same prompt selection; the providers don't yet attach the image bytes (Phase 2 does that).
- [ ] **Migrate the stub classifier.** `server/services/unified_create/classifier.go:50` empty-input check is currently `in.Text == "" && in.MediaID == "" && in.WebsiteURL == ""`. Change `in.MediaID == ""` to `in.Image == nil`.
- [ ] **Bucket dependency.** `unified_create.Service` today has no bucket. Inject `storage.BucketStorage` (the interface from `server/storage/bucket.go`) into `unified_create.New(...)` per `docs/server/architecture.md` § Pattern 3 (Constructor Injection). Update the `main.go` wiring (`server/main.go:1159`) to pass it.
- [ ] In `server/services/unified_create/stream_gen.go:152-156`: today the classifier input is built with `MediaID: msg.GetMediaId()`. Change to: when `msg.GetMediaId() != ""`, resolve the media ID to a presigned URL (mirror the pattern at `server/services/gear/gen_ai.go:240-253`), build `*DetectionImage{ImageURL: presigned, MimeType: media.ContentType}`, set on `classifierInput.Image`. Carry the same `*DetectionImage` forward to the per-type image-stream call so it doesn't re-resolve.
- [ ] **Error-code mapping** for resolution failures: emit `GEN_STREAM_ERROR_CODE_INTERNAL` (or add a new `GEN_STREAM_ERROR_CODE_MEDIA_FETCH_FAILED` enum value — check `proto/ripls/api/gen_stream.proto` first to see what's already defined). Don't use `connecterr.Internal`; this is a stream handler, not a direct Connect handler.
- [ ] **Logging.** Add a `media_id` field to the resolution log line per `docs/server/observability.md` § Standard Field Names. Log `operation=ResolveClassifierMedia`, `media_id` (from `msg.GetMediaId()`), `duration_ms`. **Do not** log the full presigned URL — it carries time-limited credentials in the query string. Note: `media_id` is logged at the caller, not on the classifier input — the classifier doesn't need to know the entity ID.
- [ ] **Unit test.** Add a case to `server/services/unified_create/stream_gen_test.go` that exercises image mode and verifies the `Image` field is populated on the classifier call: use the existing `mockClassifier` fixture, call `StreamGenUnifiedCreate` with `media_id`, assert the captured `ClassifierInput.Image != nil && Image.ImageURL != "" && Image.MimeType != ""`.
- [ ] Update the eval runner: `classifierInputForCase` in `server/ai/eval/unified_classifier_test.go` image branch currently sets `MediaID: c.ImageFile`. Change to read fixture bytes from `server/test_data/`, populate `Image: &ai.DetectionImage{ImageData: bytes, MimeType: c.MimeType}`. Eval doesn't go through bucket resolution — fixtures are bytes-only.
- [ ] Update `docs/server/observability.md` § Unified-create stream field table: add a `media_id` row noting it's emitted on image-mode streams.
- [ ] `npm run build` + `go test ./...` clean.

**Validates:** The classifier still passes the existing accuracy bar (it's not seeing the image yet, so no change in behavior). The eval-runner change verifies fixture loading works end-to-end. The new unit test confirms the resolution branch builds a non-nil `*DetectionImage` with a populated URL + MimeType. Plumbing exists for Phase 2.

### Phase 2: Per-provider image-block construction in `ClassifyUnifiedCreate`

**Goal:** Each provider's image branch actually sends the image to the API. This is the behavioral change.

**Tasks:**
- [ ] **Anthropic** (`server/ai/provider_anthropic.go` `ClassifyUnifiedCreate`): when `in.Image != nil`, build the image block via `buildAnthropicImageBlock(in.Image)` and include it as a content block alongside the text prompt in the user message. Anthropic supports multimodal user messages with both text and image content blocks in the same `MessageParam`.
- [ ] **Gemini** (`server/ai/provider_gemini.go` `ClassifyUnifiedCreate`): when `in.Image != nil`, build the part via `buildGeminiImagePart(in.Image)` and append to the message's parts list. Genkit's `ai.NewUserMessage` accepts multiple parts; the existing pattern in `provider_gemini.go` `DetectGearInImageStreaming` is the model.
- [ ] **OpenAI** (`server/ai/provider_openai.go` `ClassifyUnifiedCreate`): when `in.Image != nil`, the user message becomes a `ChatCompletionUserMessageParam` with `Content` as a slice carrying both the text part and the image part built via `buildOpenAIImageContent(in.Image)`.
- [ ] Bump `UnifiedCreateClassifierPromptVersion` to `"v3"` in `server/ai/prompts_unified.go`.
- [ ] Tighten `buildUnifiedCreateClassifierImagePrompt`: drop the "Default to 'gear' for product photography of an owned item" line. With real bytes flowing the model doesn't need a prior; the prior is now exactly the kind of accuracy-regressing prompt bias we want to remove.

**Validates:** Live eval (re-run `TestUnifiedClassifierPrompt`) shows the 6 existing gear image cases still classifying as gear, but with the image actually attached (visible in API request logs). Latency-per-call goes up modestly (bytes uploaded) but not catastrophically.

### Phase 3: Goldens corpus expansion for non-gear image cases

**Goal:** Prove the classifier now reads the image by classifying non-gear photos correctly. Without this phase, the only signal that the wiring works is "API request had image content."

**Tasks:**
- [ ] Curate / capture 4–6 non-gear photos under `server/test_data/`. Concrete ideas:
  - Event flyer / poster (expected_type: `event`)
  - "Lost dog" or "looking for X" hand-lettered sign (expected_type: `request`)
  - Garage-sale yard sign (expected_type: `event`, alt_types: `[gear]`)
  - Concert ticket / Eventbrite confirmation screenshot (expected_type: `event`)
  - "Free couch outside" with curb (expected_type: `gear`, alt_types: `[event]` for ambiguity)
- [ ] Add the cases to `server/ai/eval/testdata/unified_create_classifier_goldens.json`.
- [ ] Re-run the live eval. Compare per-provider image-mode accuracy to the Phase 2 numbers. Expect OpenAI to move from 1/6 → high pass rate, matching Anthropic and Gemini.
- [ ] Update the `image-mode caveat` paragraph in `server/ai/eval/unified_classifier.go` — it currently documents the limitation we're fixing; rewrite as "image-mode tests real vision now."

**Validates:** OpenAI image-mode accuracy parity with the other providers. Non-gear photos classify correctly across all three providers. The corpus expansion makes future regressions visible.

#### Phase 3 results (eval run 2026-05-15 against the 38-case corpus)

Transcript: `/tmp/1939-phase4-2026-05-15.txt`. Same harness as #1932 baseline + post-change runs.

| Provider / Model | n | p50_ms | p95_ms | mean_ms | accuracy |
|---|---|---|---|---|---|
| anthropic/claude-haiku-4-5 | 38 | 609 | 1031 | 640 | 38/38 (100%) |
| openai/gpt-5-mini | 38 | 1754 | 3190 | 2003 | 38/38 (100%) |
| gemini/vertexai/gemini-3.1-flash-lite | 38 | 905 | 3355 | 1252 | 38/38 (100%) |

**Image-case results (the actual point of #1939):** Every non-gear image case classifies correctly on every provider. Pre-#1939, all six non-gear photos would have come back as `gear` because the model wasn't receiving the bytes.

- Lost dog poster (Flaco) → request ✅
- Lost dog yard signs → request ✅
- Lost dog "take a photo & call" → request ✅
- White Stripes concert poster → event ✅
- Fall Into Art Show flyer → event ✅
- Babysitters Needed bulletin board → request ✅
- Charity donation flyer (ambiguous w/ event) → request ✅

**Latency comparison vs. post-#1932 baseline (29 cases, all text/url/no-image):**

| Provider | Pre-#1939 p50 | Post-#1939 p50 | Δ |
|---|---|---|---|
| anthropic | 604 | 609 | +1ms (noise) |
| openai | 1811 | 1754 | −3% |
| gemini | 698 | 905 | +30% |

Anthropic and OpenAI absorb the image-upload cost without measurable impact. Gemini's p50 moved +207ms because 6 of the 38 cases now upload image bytes; the p95 jump (997 → 3355) reflects that image cases sit in the tail. Acceptable: image classification on Gemini comes in around ~1s end-to-end, which is still under the design-doc target.

**OpenAI accuracy is the headline win.** Pre-#1939 OpenAI hit 24/29 (82.8%) because it honestly refused to classify when no image was attached. Post-#1939 it's 38/38 (100%) — same model, same prompt, just actually given the image to look at.

### Phase 4: Documentation + follow-ups

**Goal:** Reflect the fix in the design doc so the next reader understands the current shape; file the deferred optimization as its own issue rather than expand this one.

**Tasks:**
- [x] Update `docs/design/unified-create.md` § Architecture (Server) — added a "Image-mode classifier receives image bytes" paragraph documenting the post-#1939 shape and the deferred per-type reuse.
- [x] **Per-type image-call reuse deferred to a separate issue.** During Phase 1 I asserted the resolved `*DetectionImage` would be threaded forward to the per-type call to skip a redundant fetch. That wiring didn't actually land — the per-type dispatch (`streamPerTypeImage` in `stream_gen.go`) still passes `mediaID string` and the per-type generator re-resolves. Real savings are small (~one signed-URL generation per image-mode request) but real. Filed as **#1952**; not in scope here.
- [x] **`modelThresholds` tightening NOT done here.** The OpenAI accuracy fix means it's now feasible to set image-mode per-(provider, model) floors against the new baseline, but threshold calibration is #1880's scope. Leaving the existing defaults in place; #1880 picks them up.

**Validates:** Design doc accurately describes the post-#1939 shape. Follow-up filed for the deferred optimization so it doesn't get forgotten.

## Related Issues

- **#1932** — Cut classifier latency. Already shipped; this issue's per-provider edits touch the same `ClassifyUnifiedCreate` methods. Phase 2 of this plan should rebase onto whatever branch has #1932 merged so we're working on the slim+enum-constrained shape.
- **#1880** (P2.14) — Eval threshold gate for classifier accuracy. Image-mode pass rates change meaningfully when this issue lands; thresholds should be set against the post-#1939 baseline.
- **#1907** — Multi-provider classifier on `ai.Provider`. The interface foundation this issue builds on.
- **Per-type image methods** (`DetectGearInImage`, `GenerateExperienceFromImage`, `GenerateRequestFromImage` in `server/services/{gear,experience,request}/gen_ai.go`): pattern this issue copies. No code change required to those, but the unified-create caller's resolve-image-once design means the per-type call gets a reused `*DetectionImage` rather than re-resolving.
