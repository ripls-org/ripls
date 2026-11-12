# Issue #2547: Community creation — drop AI-generated name/description; two-field (name + optional description) input; keep LLM only for image-search keywords

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2547
**Filed:** 2026-06-25 | **Last activity:** 2026-06-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/server/llm.md`, `docs/client/create.md`, `docs/ad_hoc_communities.md`, `docs/client/conventions.md` (anchor), `docs/client/architecture.md` (anchor), `docs/server/architecture.md` (anchor), `docs/server/conventions.md` (anchor), `docs/proto_conventions.md` (anchor), `docs/stock_imagery.md`, `docs/reviews/api_design_review_prompt.md`, `docs/reviews/testing_review_prompt.md`, `docs/reviews/code_quality_review_prompt.md`

## Problem Statement

Today the "Create a community" flow is an AI-first, two-stage funnel: the user types a free-text **prompt** (≥20 chars), the server LLM invents a **title** + **description** + **search_keywords**, and the keywords drive a Pexels stock-image fetch. The client then shows a **preview** modal pre-filled with the AI's title/description for the user to edit before tapping Create.

We want the same philosophy we already applied to **experiences and requests** — *GenAI never overwrites the user's own text* — but taken one step further. For communities there is no structured field to extract (no place/time), so the LLM should stop producing user-visible prose entirely. Concretely:

- **(a)** Remove the 20-char minimum on the create input.
- **(b)** Replace the single-prompt input with **two plain fields — name and description** (description optional / may be empty). The user's typed text becomes the community's name/description verbatim; there is no AI-prefilled preview-and-edit round-trip.
- **(c)** Keep the background-image search exactly as today, **including its LLM-based keyword extraction** (so image relevance doesn't degrade). The LLM still runs, but only to turn the user's text into `search_keywords` for Pexels — it no longer emits a title or description. Image-upload ("bring your own background") stays.

Net: **no AI-generated name/description on community creation**; the LLM is demoted to a behind-the-scenes keyword extractor. The image-upload mode no longer needs an LLM call at all (the user's image *is* the media, and the name/description are now user-typed).

There is one design tension the issue itself flags and leaves open: once the LLM stops generating prose, the *purpose* of the second ("preview/edit the AI's work") modal largely evaporates — its only remaining job is to show and let the user replace the background image. So this change is partly a UX-collapse question, not just a "stop calling the LLM for text" question. The recommendation below resolves it.

## Context from Comments

The GitHub issue has **no comments** — but it is not under-specified, because it was authored and then refined in a planning conversation immediately before this plan. The body already records the resolved decisions (scope = everywhere; keep the LLM for keywords; image mode unchanged) and four open questions. One refinement from that conversation is load-bearing and worth restating: an earlier draft considered dropping the LLM entirely; the author reversed that — **keep the LLM for keyword extraction so image relevance doesn't regress.** That is the current intent and is assumed throughout this plan.

## Current behavior (with citations)

**Server**
- `server/services/community/gen.go:27` — `GenCommunity` (unary): `GenerateCommunityContent` → `findAndStoreStockImage(keywords)` (`gen.go:97`) → returns `{title, description, media_ids, confidence, reasoning}`.
- `server/services/community/gen_streaming.go:61` — `StreamGenCommunity`; text branch `streamGenCommunityFromText:131` emits `title`/`description` events (`:154-164`) and fires the Pexels fan-out when `search_keywords` closes (`:166-196`); image branch `streamGenCommunityFromImage:238` runs a vision call purely for `title`/`description` and emits the user's own `media_id` (`:281-305`). `fetchStockCandidates:45` pulls alternates.
- `server/ai/provider.go:377-393` — `CommunityGeneration{ Title, Description, SearchKeywords, Confidence, Reasoning }`. Interface methods: `GenerateCommunityContent:42`, `GenerateCommunityContentStreaming:47`, `GenerateCommunityFromImage:50`, `GenerateCommunityFromImageStreaming:55` — implemented across `provider_gemini.go`, `provider_openai.go`, `provider_anthropic.go`, their `_streaming.go` siblings, `provider_streaming_unary.go`, `provider_mock.go`, `provider_e2e.go`, `provider_fallback.go`, `provider_loadbalanced.go`.
- `server/ai/prompts.go:252-303` — `buildCommunityGenerationPrompt` instructs the model to emit TITLE, DESCRIPTION, SEARCH_KEYWORDS, CONFIDENCE, REASONING. The image mode reuses the same prompt.
- `server/ai/schemas.go:253-281` — `communityGenerationOutput{ Title, SearchKeywords, Description, Confidence, Reasoning }` (text) and `communityFromImageOutput{ Title, Description, Confidence, Reasoning }` (image; no keywords). Community stream keys are *derived from these schemas* via `streamingKeysFromSchema[...]()` (`streaming_provider.go:173-175`) — so narrowing the schema auto-narrows the community stream keys.
- `server/ai/streaming_provider.go:63-66` — `StreamFieldTitle/Description/SearchKeywords` are **shared across gear/experience/request/community**. They must NOT be deleted; we only stop *community* from emitting title/description.
- `server/services/community/lifecycle.go:20-54` → `createCommunity` (`adhoc.go`) → `communitylib.CreateCommunity`. **No server-side validation** of empty name or description today.

**Proto** (`proto/ripls/api/community_service.proto`)
- `CreateCommunityRequest:19` = `{ name=1, description=2, media_ids=3, simulation_id=4 }` — name/description already optional on the wire.
- `GenCommunityResponse:236` = `{ title=1, description=2, media_ids=3, confidence=4, reasoning=5 }`.
- `StreamGenCommunityResponse:278` oneof = `{ title=1, description=2, media_ready=3, final=4, error=5 }`.
- RPCs at `:990` (`CreateCommunity`), `:993` (`GenCommunity`), `:997` (`StreamGenCommunity`).

**Client**
- `app/lib/presentation/screens/communities/community_creation_modal.dart` — text/image toggle, a single prompt `TextInputArea` (`:268`), and a **Generate** button gated by `canGenerate()`; Generate → `generateCommunityStreaming()` → opens `CommunityPreviewModal`.
- `app/lib/presentation/viewmodels/community_creation_view_model.dart:93-99` — `canGenerate()` enforces the **20-char minimum** (text mode); `generateCommunityStreaming():118` pipes `title`→`applyStreamingTitle`, `description`→`applyStreamingDescription`, `mediaReady`, `final` (reads `resp.title`/`resp.description`), `error`.
- `app/lib/presentation/viewmodels/gen_community_view_model.dart` — holds `name`/`description`/`mediaIds`; `applyStreamingTitle→name` (`:163`), `applyStreamingDescription→description` (`:169`), `finalizeStreaming` (`:250`); `createCommunity()` **requires both name and description non-empty** (`:297-310`).
- `app/lib/presentation/screens/communities/community_preview_modal.dart` — editable name + description (`_SyncedTextField`), **Replace Background** → `MediaPickerDialog` (Photos/Camera/Video + streamed candidates), Create; `_handleCreate` **requires both name and description non-empty** (`:106-117`).
- `app/lib/data/repositories/community_repository.dart:50` — `streamGenCommunity({prompt,imagePath,region})`; `createCommunity({name,description,mediaIds}):294`.
- l10n (`app/lib/l10n/app_en.arb`): `communityCreationTextHint`, `communityCreationGenerateButton`, `communityPreviewNameHint`, `communityPreviewDescriptionHint`, `communityPreviewNameRequired`, `communityPreviewDescriptionRequired`, `communityPreviewCreate`, `communityPreviewHeader`, `communityPreviewNameLabel`, `communityPreviewDescriptionLabel`.

**Tests touching the fields we're changing**
- `server/services/community/gen_test.go` (asserts `resp.Title/Description/Confidence`, generate→create flow).
- `server/services/community/gen_streaming_test.go` (mock `CommunityGeneration` with Title/Description/Confidence; asserts final Title/Description).
- `server/ai/provider_community_from_image_test.go` (asserts Title/Description/SearchKeywords on image mode).
- `server/ai/eval/community.go` + `eval/community_test.go` (quality eval over Title/Description/Confidence).
- Client viewmodel tests under `app/test/` for community creation/gen (verify after Phase 2/3).

## Potential Solutions

The change has two semi-independent axes: **(I) the server/proto contract** and **(II) the client UX**. Options below are per-axis; the recommendation picks one of each.

### Axis I — server/proto contract

#### Option I-A: Narrow the existing Gen RPCs to image-only (recommended)
- **Approach:** Keep `GenCommunity` / `StreamGenCommunity`. Narrow `communityGenerationOutput` and `buildCommunityGenerationPrompt` to **`search_keywords` only**. Stop emitting `title`/`description` stream events; stop populating `title`/`description`/`confidence`/`reasoning` on `GenCommunityResponse`. Make the image branch skip the LLM entirely (just emit the user's `media_id`). Defer the proto field/oneof *removal* to a final cleanup phase (emit-empty first, delete once nothing reads them).
- **Pros:** Preserves the streamed-candidates → Replace-Background UX. Smallest blast radius; keeps `MediaReady`/`final`/`error` and the whole streaming-sender scaffold. The "emit empty, then delete" sequencing keeps `main` green at every step.
- **Cons:** Leaves the `Gen`/`StreamGen` *names* attached to an RPC that no longer "generates" prose — a mild API-design smell (see Open Questions). Carries the now-vestigial `final` envelope (`GenCommunityResponse`) whose only live field is `media_ids`.
- **Effort:** Medium.

#### Option I-B: Replace with a purpose-named background-image RPC
- **Approach:** Introduce `StreamCommunityBackground` (text → streamed image candidates) and retire `GenCommunity`/`StreamGenCommunity`.
- **Pros:** Honest naming; sheds the title/description/confidence/reasoning envelope cleanly.
- **Cons:** Larger wire break and more churn (service wiring, client service/repository, e2e provider) for little behavioral gain. Re-implements scaffolding I-A already has.
- **Effort:** Large.

#### Option I-C: Fold the image search into CreateCommunity (synchronous)
- **Approach:** Delete the Gen RPCs; when `CreateCommunity` receives no `media_ids`, derive keywords + fetch one stock image inline.
- **Pros:** Fewest moving parts server-side; no streaming.
- **Cons:** Loses the *preview the background before committing* and *choose among candidates* UX entirely; makes Create latency depend on Pexels; regresses the existing replace-from-candidates feature. Contradicts (c) "as it is currently."
- **Effort:** Small server / negative UX.

### Axis II — client UX

#### Option II-A: Keep two modals, minimal churn
- **Approach:** Add name+description fields to the *input* modal; carry them (user-entered, not AI) into the preview modal, which still shows the image + Replace.
- **Pros:** Least refactor.
- **Cons:** User types name/description in modal 1, then sees the same two fields again in modal 2 — redundant and confusing now that there's no AI text to "review."
- **Effort:** Small–Medium.

#### Option II-B: Collapse into a single modal (recommended)
- **Approach:** Promote `community_preview_modal.dart` to the **sole** create-community modal: name field + optional description field + auto/triggered replaceable background + Create. Delete the prompt-input modal (`community_creation_modal.dart`) and its text/image toggle; image-upload survives via the preview modal's existing **Replace Background → Camera/Gallery**. Trigger the keyword→image fetch from within the modal (see Open Q2). Update the entry point (`workshop_screen.dart`) to open it directly.
- **Pros:** Cleanest UX and matches the issue's "the generate→preview round-trip collapses." The preview modal already has every widget needed (name field, description field, Replace Background, candidates row, Create) — so this is mostly *deletion + a trigger wire-up*, not new UI.
- **Cons:** New behavior for the image-fetch trigger (must be debounced/guarded to avoid spamming Pexels/LLM); removes the dedicated camera-first capture surface (acceptable — Replace Background covers it).
- **Effort:** Medium.

## Recommendation

**Axis I → Option I-A** (narrow the existing RPCs, emit-empty-then-delete) and **Axis II → Option II-B** (collapse to a single modal built from the existing preview modal).

Rationale: I-A keeps the streamed-candidate/Replace-Background experience the issue says to preserve ("image search as it is currently") while minimizing wire churn, and the emit-empty-then-delete sequencing lets every phase ship green. II-B is the honest end state once AI text is gone, and because the preview modal already contains all the needed widgets, the cost is dominated by *removing* the prompt modal and wiring one image-fetch trigger — not building new UI. I-C is rejected because it regresses the background-preview/candidate UX; I-B/II-A add churn or keep a confusing double-entry without commensurate benefit.

The one genuinely new piece of behavior is the image-fetch trigger (Open Q2). Recommended: fire on the **name field's editing-complete / blur** (plus a manual re-fetch implicitly available via Replace Background), keying the Pexels/LLM call off `name` (+ `description` if non-empty); cancel any in-flight fetch on a new trigger; and **do not auto-refetch once the user has manually replaced the background.** This avoids per-keystroke debounce complexity and Pexels/LLM spam while still showing a relevant backdrop before Create.

## Open Questions

- **Q1 — Empty name on create vs. promote.** *Recommendation:* Require a non-empty **name** in the create-from-scratch flow (client-enforced; server stays permissive). The **promote** path (naming a nameless community) is `UpdateCommunity`, not `CreateCommunity` (`docs/ad_hoc_communities.md` → "Promote-by-naming"), so it is unaffected by this change. **Verify** whether the "#2543 promote-via-modal" path actually reuses `community_creation_modal`/`community_preview_modal`; if it does, ensure the collapse (II-B) doesn't regress it. If promote does reuse the modal, allow it to open with the nameless community's group-text identity (nothing AI-prefilled), consistent with the doc.
- **Q2 — When does the image fetch fire?** *Recommendation:* on name-field editing-complete/blur (see Recommendation), guarded against refetch-after-manual-replace and with in-flight cancellation. Alternatives if that feels too eager: an explicit "Find a background" affordance, or fetch lazily at Create with a brief spinner. Whichever is chosen, keep it best-effort: a Pexels/LLM failure must never block Create (mirror today's `WarnContext` + continue).
- **Q3 — Keyword source for the LLM.** *Recommendation:* pass **name + description** (when description is non-empty) as the prompt to the keyword extractor for better relevance; fall back to **name** alone when description is empty.
- **Q4 — Proto cleanup timing (wire break).** *Resolved:* **emit-empty first, delete later — and the delete is its own tracked follow-up, #2551.** Phase 1 stops *populating* `title`/`description`/`confidence`/`reasoning` (and stops emitting the title/description stream events), leaving `TODO(#2551):` markers; the field/oneof removal happens in #2551 once #2547 is stable. This keeps generated Dart/Go compiling through Phases 1–2. Per `docs/proto_conventions.md`, an intentional break is acceptable here (pre-launch, single first-party client).
- **Q5 — RPC naming.** `Gen`/`StreamGen` will name an RPC that only fetches an image. *Resolved:* **keep the names this round** (Option I-A) to avoid a second wire break; the rename to a background-image RPC is folded into the **#2551** cleanup and flagged in-code with `TODO(#2551):`. Note the smell in the PR description so api_design review isn't surprised.
- **Q6 — Is the unary `GenCommunity` still used by the client?** The Dart repository only calls `streamGenCommunity`. *Recommendation:* confirm `GenCommunity` (unary) has no live client caller; if unused, narrow it in Phase 1 for parity and mark it for removal in Phase 3 (or leave it as the documented unary drain over the stream — `gen.go:17-26`).
- **Q7 — Server-side name validation on `CreateCommunity`?** Today `CreateCommunity` does **zero** validation (`lifecycle.go:20-54`); name-required is purely client-side. *Recommendation:* keep client-side enforcement as the primary gate (matches today and avoids scope creep), and treat adding a server-side `CodeInvalidArgument` for an empty name as an **optional** fail-fast hardening — but only after confirming it can't break legitimate nameless paths. Note: nameless communities are created via `ProvisionAdHocCommunity`/`ProvisionPerItemCommunity`, **not** the `CreateCommunity` RPC (`docs/ad_hoc_communities.md`), so an RPC-level name-required check is defensible; still verify the simulation/dev create path before adding it.

## Implementation Plan

> Sequencing rule (Q4): never remove a proto field while generated code still reads it, and don't break the wire in the same change that ships the behavior. Phases 1–2 *stop populating/reading* the AI prose fields and keep the `Gen*` names so generated Go/Dart keeps compiling; they leave **`TODO(#2551):` markers** on every vestigial site. **Phase 3 is deferred to follow-up issue #2551** and runs only after #2547 has shipped and is stable — it deletes the fields/methods and renames the RPCs. Each phase keeps `npm run generate` + Go build + `flutter analyze` green.
>
> **TODO convention (per CLAUDE.md):** every marker is `TODO(#2551):` and self-contained — state what the code does today, what would let it be removed, and that it's blocked on the migration baking. Example to drop on the emit-empty site in `gen_streaming.go`:
> ```go
> // TODO(#2551): These fields are intentionally left empty — community
> // creation no longer AI-generates a title/description (only image
> // keywords). Remove title/description/confidence/reasoning from
> // GenCommunityResponse and rename Gen/StreamGenCommunity to a
> // background-image RPC once #2547 has shipped and is stable.
> ```

### Phase 1: Server — LLM produces keywords only; image mode drops the LLM
**Goal:** The server stops inventing community title/description; the LLM is narrowed to image keywords; image-upload mode no longer calls the LLM. Existing clients keep working (they receive empty title/description and ignore them).
**Tasks:**
- [ ] `server/ai/schemas.go`: narrow `communityGenerationOutput` to `{ SearchKeywords }` (drop Title/Description/Confidence/Reasoning). Decide the fate of `communityFromImageOutput` — it becomes unused once image mode skips the LLM (delete in Phase 3).
- [ ] `server/ai/prompts.go`: rewrite `buildCommunityGenerationPrompt` to ask **only** for 3–5 image search keywords from the user's text; remove the TITLE/DESCRIPTION/CONFIDENCE/REASONING sections.
- [ ] `server/ai/provider.go`: keep `CommunityGeneration` for now but treat Title/Description/Confidence/Reasoning as dead (populated empty). Keyword-only path flows through `GenerateCommunityContent[Streaming]`.
- [ ] `server/services/community/gen_streaming.go`: in `streamGenCommunityFromText`, stop emitting `title`/`description` events; keep the `search_keywords`→Pexels fan-out and `media_ready`. In `streamGenCommunityFromImage`, **skip the AI vision call** — resolve/verify the media and emit `media_ready` with the user's `media_id` directly. Set `title`/`description`/`confidence`/`reasoning` empty on the `final` `GenCommunityResponse`.
- [ ] `server/services/community/gen.go`: `GenCommunity` returns `media_ids` only (empty title/description/confidence/reasoning).
- [ ] **Observability:** the `logger.InfoContext` calls in `gen.go:60,79` and `gen_streaming.go:209,307` log `generation.Title`/`Confidence` — now always empty. Drop those fields and log `media_hit` / keyword-count / `media_fired` instead, so logs stay meaningful (per `docs/server/observability.md` log-field discipline).
- [ ] Use **name (+ description)** as the keyword-extraction input (Q3) — i.e. the `prompt`/`StreamGenCommunityRequest.prompt` now carries the user's name/description text.
- [ ] **Leave `TODO(#2551):` markers** (text per the convention note above) on every vestigial site: the empty `title`/`description`/`confidence`/`reasoning` assignments in `gen.go`/`gen_streaming.go`, the retained `Gen`/`StreamGenCommunity` RPC names, the soon-dead image-mode provider methods, and `communityFromImageOutput`.
- [ ] Update server tests: `gen_test.go`, `gen_streaming_test.go`, `provider_community_from_image_test.go` — drop title/description/confidence assertions; assert keywords drive the image and that the image branch needs no provider call.
- [ ] **Round-trip coverage** (`proto_conventions.md` → "Round-trip tests"): description is becoming legitimately empty, so add/confirm a `CreateCommunity(name, description="")` → `GetCommunity` round-trip asserting the empty description (and name + media_ids) come back intact — the mechanical guard that "optional/empty description" is wired on both the write and read paths.
- [ ] Refresh `docs/server/llm.md` (community is now keyword-only; the "Generate-Then-Preview" pattern no longer applies to community) and re-stamp freshness.
**Validates:** AI no longer generates prose; image search still works from user text; build + server tests green with no client change required.

### Phase 2: Client — two-field input, no min length, optional description (single-modal collapse)
**Goal:** Deliver requirements (a), (b), (c) end-to-end: a single create-community modal with a name field and an optional description field, no minimum length, user-entered text, and a relevant background fetched/selectable.
**Tasks:**
- [ ] Promote `community_preview_modal.dart` to the sole create modal (Option II-B): name field + **optional** description field + Replace Background + Create. Fire the keyword→image fetch per Q2 (name blur/editing-complete; cancel in-flight; no refetch after manual replace).
- [ ] Delete `community_creation_modal.dart` (prompt textarea + text/image toggle + camera layer wiring); image-upload survives via Replace Background → Camera/Gallery. **Note:** with the collapse, image-upload goes through `MediaPickerDialog` → `addMedia` (a direct upload that sets `mediaIds`), so the `StreamGenCommunity` **image branch** (`media_id` input) loses its client caller — flag it dead and fold its removal into #2551 (don't delete the branch in this phase; just stop calling it).
- [ ] `community_creation_view_model.dart`: remove `canGenerate()`'s 20-char gate and the "at least 20 characters" error; repurpose/rename `generateCommunityStreaming()` to a "fetch background for this name/description" trigger (still streams `media_ready`); stop seeding name/description from `title`/`description` events.
- [ ] `gen_community_view_model.dart`: make **description optional** — drop the description-required check in `createCommunity()` (`:304-310`); keep name-required. Stop using `applyStreamingTitle`/`applyStreamingDescription` to overwrite user text.
- [ ] `community_preview_modal.dart` `_handleCreate`: drop the description-required check (`:112-117`); keep name-required.
- [ ] Update the entry point (`workshop_screen.dart`) to open the collapsed modal directly.
- [ ] l10n: remove `communityCreationTextHint`/`communityCreationGenerateButton` and `communityPreviewDescriptionRequired`; ensure name/description hints read as a direct form (not "AI will…"). Run `npm run` l10n parity checks.
- [ ] Carry the `TODO(#2551):` markers through any client genai code that's retained for now (e.g. an `applyStreamingTitle`/`applyStreamingDescription` left in place but unused).
- [ ] **Accessibility** (`docs/client/accessibility.md` + accessibility review §6 Forms / §2 Live Regions): the new/relocated name + description fields need associated semantic labels (not just `hintText` on the bare `_SyncedTextField`); the name-required validation error must be announced to screen readers (wire it through a `LiveRegion`, not a silent inline-only banner); the background-fetch loading/changed state should announce via `LiveRegion` so a screen-reader user knows an image is resolving/has changed; confirm Replace Background and Create remain ≥48dp touch targets (they use existing primitives, so largely covered). Run `npm run` a11y lint (`scripts/check_a11y.js`).
- [ ] Refresh `docs/client/create.md` ("Community Creation Flow" §, steps 2–4: now a single two-field modal, no AI prose) and re-stamp freshness.
- [ ] Update/add client viewmodel + widget tests: no-min-length, empty-description-creates-successfully, name-required, background-fetch trigger + best-effort failure, Replace Background still works.
**Validates:** A user can create a community with a short name and no description; the background still appears and is replaceable; no AI text anywhere.

### Phase 3 (deferred — tracked by #2551): Cleanup — wire break + dead-code + RPC rename
**Goal:** After #2547 has shipped and baked, remove everything Phases 1–2 made vestigial and rename the image-only RPCs. **This is a separate follow-up (#2551), not part of the #2547 PR** — it is the thing the `TODO(#2551):` markers point at. Full scope lives in #2551; summarized here for continuity:
- [ ] Proto wire break (`community_service.proto`): remove `title`/`description`/`confidence`/`reasoning` from `GenCommunityResponse` and the `title`/`description` oneof variants from `StreamGenCommunityResponse`, and **mark the removed field numbers *and* names `reserved`** (per `proto_conventions.md` / api_design §4 "removed fields should be reserved") — e.g. `reserved 1, 2, 4, 5; reserved "title", "description", "confidence", "reasoning";` on `GenCommunityResponse`, and reserve `1, 2` + names at the `StreamGenCommunityResponse` message level (oneof field numbers share the message space). Keep `media_ready`/`final`/`error`.
- [ ] **Rename** `Gen`/`StreamGenCommunity` to a background-image RPC, and **collapse the vestigial `final` envelope**: today `StreamGenCommunityResponse.final` reuses the unary `GenCommunityResponse` (cross-RPC type reuse the conventions discourage). Give the stream its own terminal media-only type (or drop `final` if `media_ready` already carries everything).
- [ ] **Remove the dead `media_id` image branch:** `streamGenCommunityFromImage`, `StreamGenCommunityRequest.media_id` (reserve the field number), and the from-image provider methods — confirmed caller-less after the Phase-2 collapse (image-upload now goes through Replace Background).
- [ ] **Fix the proto-comment vendor leak:** `StreamGenCommunityResponse.media_ready` currently reads "stock image found via Pexels" (`:284`) — replace with the capability ("stock image"), per `proto_conventions.md` → "Proto comments" (no vendor names on the wire contract).
- [ ] `server/ai/provider.go` + all providers: delete `GenerateCommunityFromImage` / `GenerateCommunityFromImageStreaming` and `communityFromImageOutput`; trim `CommunityGeneration` to keywords-only. Resolve Q6 for the unary `GenCommunity`.
- [ ] `server/ai/eval/community.go` + `eval/community_test.go`: narrow to keyword relevance or retire (obsolete title/description/confidence checks; relates to closed #1800).
- [ ] Remove any remaining unused l10n keys (`communityPreviewHeader`, `communityPreviewNameLabel`, `communityPreviewDescriptionLabel`).
- [ ] Verify all `TODO(#2551):` markers are removed.
**Validates:** No dead AI code paths remain; proto surface and RPC names match behavior; full build + all suites green.

## Related Issues
- **#2551** — Follow-up cleanup (filed): delete the vestigial GenAI fields/methods and rename the image-only RPCs after this ships. This is what every `TODO(#2551):` marker points at.
- **#2492** — Epic (account-optional, phone-first pivot); this is DISPLAY-1-adjacent and the promote-by-naming context lives here.
- **#2543** — PR that added the promote-via-modal flow; verify Phase-2 collapse doesn't regress it (Q1).
- **#2548** — Per-item community join/promote semantics; sibling of the promote path.
- **#1800** (closed) — Community-naming eval goldens; the eval narrowing in Phase 3 effectively retires that surface.
- **#1648 / #1662** (closed) — Image-mode community creation; Phase 1 removes the AI vision call those added, leaving only the user-image-as-media behavior.
- **Precedent:** the experiences/requests "GenAI no longer overwrites user text" change (`server/services/experience/gen_ai_streaming.go`, `server/services/request/gen.go`) — same philosophy, one step further (no LLM prose at all for community).

## Audit Notes

Audited via `/ripls-audit` against the documentation context map. Engaged dimensions: **api_design**, **error_handling**, **testing**, **accessibility**, **observability**, **code_quality/tech_debt**.

**Fixes applied to this plan during the audit:**
- **api_design / proto_conventions:** Phase 3/#2551 now requires `reserved` declarations for removed field numbers **and** names (was "do not reuse field numbers" only); calls out fixing the **"Pexels"** vendor leak in the `media_ready` proto comment (`:284`); and names the **cross-RPC reuse** of `GenCommunityResponse` inside `StreamGenCommunityResponse.final` as something the `final`-envelope collapse should resolve.
- **accessibility (was entirely missing):** added a Phase-2 a11y task — semantic labels on the name/description fields, screen-reader announcement of the name-required error and the background-fetch state via `LiveRegion`, touch-target confirmation, and the `check_a11y.js` gate.
- **dead-code / api_design:** noted that the single-modal collapse orphans the `StreamGenCommunity` **`media_id` image branch** (Replace Background uploads directly), and folded its removal (branch + request field + from-image provider methods) into #2551.
- **error_handling:** added **Q7** on optional server-side name validation on `CreateCommunity` (currently unvalidated), with the ad-hoc-path caveat; reaffirmed the image fetch stays best-effort (`WarnContext` + continue).
- **testing:** added an explicit **round-trip** task for `CreateCommunity(description="")` → `GetCommunity` now that empty description is legitimate.
- **observability:** added a Phase-1 task to drop the now-empty `Title`/`Confidence` log fields and log `media_hit`/keyword-count instead.

**No contradictions found** with documented decisions: the promote path correctly stays on `UpdateCommunity` (`ad_hoc_communities.md`), the API/models separation is untouched, and the proto wire break is acceptable pre-launch per `proto_conventions.md`.

**Docs consulted:** `proto_conventions.md` (anchor), `server/architecture.md` + `server/conventions.md` (anchors), `client/architecture.md` + `client/conventions.md` (anchors), `server/llm.md`, `client/create.md`, `ad_hoc_communities.md`, `stock_imagery.md`, `client/accessibility.md`, and the review rubrics `reviews/api_design_review_prompt.md`, `reviews/accessibility_review_prompt.md`, `reviews/error_handling_review_prompt.md`, `reviews/testing_review_prompt.md`.
