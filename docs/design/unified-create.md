---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Design for unified-create (omni-create) — collapsing the separate gear/event/request create-from-prompt flows into one entry point with server-inferred type, a single streaming result shape, and reversible type inference.
  globs: [app/lib/presentation/viewmodels/gen_experience_view_model.dart, app/lib/presentation/viewmodels/gen_request_view_model.dart, app/lib/presentation/viewmodels/unified_create_view_model.dart, app/lib/presentation/widgets/create/**, app/lib/presentation/widgets/modal/glass/**]
  triggers: [unified-create, omni-create, gen, create-modal, streaming, type-inference, glass-modal]
  lens: [client, domain]
freshness:
  verified_commit: "d811df3cb"
  verified_on: "2026-06-17"
---
# Unified Create (omni-create)

**Status:** Draft — design review
**Codename:** `unified-create`
**Reference prototype:** `docs/concepts/create/bottom-drawer-standalone.html` (PR #1785)
**Glass design foundation:** PR #1798 + PR #1854 (modals migrated to frosted glass; see
`app/lib/presentation/widgets/modal/glass/`)
**Out of scope:** Community creation (keeps existing `GenCommunity` flow), Loan creation
(implicit downstream of Gear)

## Summary

Today Ripls has three independent "create from a prompt" flows — Gear, Event, Request
— each with its own modal, view-model, prompt, and streaming RPC. The unified-create
work collapses them into one entry point. The user types text, snaps a photo, or
pastes a URL; the server infers the type (gear / event / request, with gear split
into lending vs. giveaway by transfer intent) and streams back a single result
shape that the client renders into the same glass preview card. The user can
flip the inferred type without re-calling the server.

This is the flagship "share something" surface and is the most performance-sensitive
LLM path in the app. The design optimizes for: low time-to-first-token, coherent
streaming UX, and the ability to evolve the server-side LLM strategy
(two-call classifier-then-detail vs. one-call unified) **without** breaking
the client contract.

This work ships behind a single client feature flag; the server is purely
additive (new RPC, new prompt path) and the existing per-type flows stay
intact for an unknown rollback period.

## Context: what exists today

### Client gen surfaces

| Type    | View model                          | Lines | Streaming actions                          |
|---------|-------------------------------------|-------|--------------------------------------------|
| Event   | `gen_experience_view_model.dart`    | 997   | inline in viewmodel                        |
| Request | `gen_request_view_model.dart`       | 543   | inline in viewmodel                        |

Each owns: input collection (text / image / URL), RPC dispatch, partial-result
reconciliation, validation, save-button gating, and post-save navigation.

The legacy gear-only entry modal (`create_gear_modal.dart`) has since been
removed — gear creation now flows through unified-create. Event and request
still have their own per-type modals
(`app/lib/presentation/screens/experience/experience_creation_modal.dart`,
`app/lib/presentation/screens/request/request_creation_modal.dart`).

### Server gen surfaces

Unary + streaming pairs per type:

| Proto                                  | Unary RPC       | Streaming RPC          | Prompt builder (`server/ai/prompts.go`) |
|----------------------------------------|-----------------|------------------------|------------------------------------------|
| `proto/ripls/api/gear_service.proto`   | `GenGear`       | `StreamGenGear`        | `buildGearGenerationPrompt`, `buildGearFromWebpagePrompt`, `buildGearDetectionPrompt` |
| `proto/ripls/api/experience_service.proto` | `GenExperience` | `StreamGenExperience` | `buildExperienceFromTextPrompt`, `buildExperienceFromImagePrompt`, `buildExperienceFromWebpagePrompt` |
| `proto/ripls/api/request_service.proto`| `GenRequest`, `GenRequestFromMedia` | `StreamGenRequest` | `buildRequestGenerationPrompt`, `buildRequestImageAnalysisPrompt` |

Each prompt is hand-tuned for one output shape. There is no shared classifier
prompt; each RPC implicitly assumes the type. `proto/ripls/api/gen_stream.proto`
already defines `GenStreamErrorCode` and `MediaReady` as the shared streaming
primitives — those carry over unchanged.

### Streaming contract today

All three `StreamGen*Response` messages share the same shape: a `oneof event`
with non-terminal variants (`title`, `description`, `geocoded`, `media_ready`,
plus type-specific extras like `ExperienceTimeExtraction`) and exactly one
terminal variant (`final` containing the full unary response, or `error`).
Non-terminal events arrive in any order; the client reconciles against `final`.

This contract is the model for `StreamGenUnifiedCreate`.

## Goals

1. **One entry point.** A single "Create" affordance opens a single modal. Type is
   inferred, not chosen up front.
2. **Type-correct preview.** Once the type lands, the preview card shows the
   right per-type fields (gear → item details modal; event → time; request → no
   extras), in the cascade shown in the prototype.
3. **Reversible inference.** User can flip type via the type selector at any
   point during or after streaming. The flip **re-issues the stream** with a
   `force_type` hint so type-specific fields (gear item details, event time,
   etc.) are populated for the new type rather than leaving the user to fill
   them by hand. Cross-type fields the user might have already edited — title,
   description, location, media, communities — are preserved client-side and
   are not overwritten by the re-stream unless the user hasn't touched them.
   When `force_type` is set, the server skips the classifier and goes directly
   to the per-type generator (see [LLM design](#llm-design) and
   [RPC contract](#rpc-contract-streamgenunifiedcreate)).
4. **Save gated identically to today.** Save is disabled until streaming completes
   AND all required fields for the (possibly-flipped) type are populated. On save,
   the client calls the existing per-type `Save*` RPC for the final selected type
   — no new save RPC.
5. **LLM strategy hidden from client.** Client speaks one new RPC
   (`StreamGenUnifiedCreate`). Server implementation can start as classify-then-delegate
   (calls existing prompts) and migrate to one unified prompt later without
   shipping a new client.
6. **Latency targets.**
   - Time to first non-terminal event (typically `title`): **≤ p50 1.5s, p95 3s**
     for text input. Match or beat current single-type flows.
   - Time to terminal `final`: match current per-type p95 (text ~6s, image ~8s,
     url ~10s).
7. **Per-prompt token efficiency.** Unified prompt token count (system + user)
   should not exceed the **maximum** of the three current per-type prompts by
   more than 20%. See [LLM design](#llm-design).
8. **Behind a single client flag.** No server flag — the server exposes the new
   RPC unconditionally. Rollout is an app release.

## Non-goals

- Replacing the existing per-type Gen RPCs. They stay wired and tested as the
  fallback path; they remain the production path for community creation.
- Server-side feature flag or canary. The server is additive; rollout is via the
  client app version.
- Touching the Save\* RPCs. The unified flow calls the existing per-type save.
- A unified "stuff" object on the server. Per-type storage is unchanged — gear
  rows go in `gear`, experiences in `experiences`, requests in `requests`.
- Adding new fields to gear / experience / request schemas.
- Voice input. Future work.

## UX overview

The prototype (`bottom-drawer-standalone.html`) is the source of truth for visual
behaviour. Summary of what we'll implement in Flutter:

### Stages

1. **Input.** Bottom drawer with three tabs: **Text**, **Image**, **URL**. The
   drawer is collapsed (~80px) when Image (camera) is active, expanded
   (~360px) when Text or URL is active. Camera tab shows live viewfinder
   underneath the drawer; Text tab shows a 5-row textarea; URL tab shows a
   single-line URL input. A `Generate` button submits.
2. **Preview.** A single glass card cascades in over a darkened background.
   The image (stock or uploaded) fades in last. Cascade order:
   `type selector` → `title` → `description` → per-type fields → `image` → `Share with communities`.

### Suggested prompts (Text tab)

When the Text tab is active and the input is empty, surface a two-tier
chatbot-style nudge below the textarea. The interaction mirrors the
empty-state suggestion UX in ChatGPT, Claude, and Gemini: a small set of
top-level category chips that, when tapped, expand into concrete examples
the user can tap to populate the input.

**Tier 1** is a row of three category chips that match the preview-card
type selector exactly so the user learns one vocabulary:

- **Request** → "I'm asking for something"
- **Event** → "I'm gathering people"
- **Item** → "I have a thing to share" (covers both lend and give; the
  user picks lend vs. give via a toggle on the preview card — see
  [Lend / Give toggle](#lend--give-toggle-item-only))

Tapping a tier-1 chip expands **tier 2**: a short list (3–5 items) of
example prompts relevant to that category. Tapping a tier-2 item populates
the textarea with that example. The user can then edit the text or hit
Generate as-is. Tapping the tier-1 chip again (or selecting a different
tier-1) replaces the tier-2 list.

Tier 2 is **community-aware**:

- For users with a primary community, tier-2 examples are drawn from recent
  activity in that community (e.g., the titles of recent gear posts as
  "Stuff" examples) plus a short list of generic prompts as ballast.
- For users with no primary community (or a brand-new community with no
  posts), tier 2 falls back to a static seed list per category.

**Data source for Phase 1.** Reuse the existing list/feed RPCs against the
user's primary community to pull the most recent N gear / event / request
titles. No new server RPC needed. If latency on the list call is a concern,
populate tier-2 lazily on first tier-1 tap rather than upfront.

The Image and URL tabs do not need suggested prompts — the affordance is
self-evident.

**Note on tier-1 labels.** The tier-1 chips read **Request · Event · Item**
to match the preview-card type selector exactly — same words, same order, so
the user learns the vocabulary in one place. "Item" is the user-facing label
for `gear`; we are not renaming `Request`, `Experience`, or `Gear` in code,
ARB keys for other surfaces, RPCs, or anywhere else in the app.

### Type-specific fields shown in preview card

- **Event:** time, location, communities
- **Item (gear):** Lend/Give toggle (default Lend), item details
  (push-modal: brand, model, est. value, material, weight), location,
  communities
- **Request:** location, communities

### Type selector

A horizontal segmented selector at the top of the preview card shows **three
types** as labelled chips: **Request · Event · Item** (icon + text + filled
background on the selected one), with the inferred type pre-selected.

Lend vs. give is **not** a server-classified type — it is an attribute the
user sets within the Item flow (see [Lend / Give toggle](#lend--give-toggle-item-only)).
This keeps the classifier prompt small (three categories) and preserves
today's behaviour where the user decides whether to lend or give.

Tapping a non-selected chip triggers `viewModel.flipType(newType)`, subject to
the timing rule below, which:

1. Preserves cross-type fields the user might have edited (title, description,
   location, media, communities).
2. Cancels any in-flight `StreamGenUnifiedCreate`.
3. Re-issues `StreamGenUnifiedCreate` with `force_type = newType` so the new
   per-type fields stream in.
4. Replays the cascade animation for the new field set.

The server skips the classifier when `force_type` is set, so the re-stream
costs roughly the same as a fresh first-type generation (no double classify
penalty).

**Flip timing rule.** The selector enables flips as soon as the server emits
its `type` event (the first non-terminal event in practice). Once the user
triggers a flip, the selector is **disabled** until the resulting re-stream
reaches its terminal `final` event. This prevents a user from flipping
mid-flip and avoids racing partial streams. The selector chips are still
visible during the disabled window; they just don't respond to tap (with
the building pill indicating activity).

### Lend / Give toggle (Item only)

When the selected type is Item, the preview card shows a small two-segment
toggle (Lend ⇄ Give) near the item-details row. It defaults to **Lend** and
the user can flip it to Give at any time, including after streaming
completes. This is purely a client-side state — the server does not stream a
value for it — and it flows into the Save call as `transfer_intent`. Cost
to the user is one tap before sharing; cost to the system is zero since the
LLM is not involved.

### Building indicator

A subtle "Building..." pill (spinner + label) appears in the top-right of the
preview card while streaming is in flight. It disappears when the terminal
event arrives.

### Lend vs. give

Same model as today: the AI does **not** classify lend vs. give. The
classifier returns `gear` (rendered as "Item" to the user); the user picks
Lend or Give via the toggle described above. This keeps the classifier
prompt tight and avoids relitigating an inference we already know how to
let the user make.

`transfer_intent` is sent on the per-type `SaveGear` call with the toggle's
final value. There is no `transfer_intent` field in any
`StreamGenUnifiedCreate*` message — it is purely a save-time concern.

## Architecture

### Client

```
app/lib/presentation/
  screens/create/
    unified_create_modal.dart          # Bottom drawer + preview card host
    unified_create_input_drawer.dart   # Tabs + text/image/url panels
    unified_create_preview_card.dart   # Glass preview with cascade
    unified_create_type_selector.dart  # Inline 4-segment selector
    unified_create_item_details_sheet.dart  # Glass push-modal for gear fields
  viewmodels/
    unified_create_view_model.dart     # Single VM, replaces three
    unified_create_state.dart          # Freezed state
    unified_create_streaming_actions.dart
```

**State shape** (`UnifiedCreateState`, freezed):

```dart
@freezed
class UnifiedCreateState with _$UnifiedCreateState {
  const factory UnifiedCreateState({
    @Default(CreateStage.input) CreateStage stage,
    @Default(InputMode.text) InputMode inputMode,
    @Default('') String prompt,
    @Default('') String urlInput,
    String? mediaId,                       // image mode after upload
    DetectedContentType? type,             // most recent type from server
    String? title,
    String? description,
    GeocodedLocation? location,
    List<String> mediaIds,
    ExperienceTimeExtraction? eventTime,   // event-only
    GearItemDetails? itemDetails,          // gear-only (loan + give)
    @Default(TransferIntent.loan) TransferIntent transferIntent, // user-controlled; only meaningful when type==gear
    @Default(<String>[]) List<String> selectedCommunityIds,
    // Track which fields the user has manually edited so that a re-stream
    // (triggered by flipType) does not overwrite user edits.
    @Default(<UserEditedField>{}) Set<UserEditedField> userEditedFields,
    @Default(false) bool streaming,
    @Default(false) bool streamComplete,
    GenStreamError? error,
  }) = _UnifiedCreateState;
}
```

**Type flip handler** (`flipType(newType)`):

1. Cancel any in-flight `StreamGenUnifiedCreate` subscription.
2. Reset type-specific fields that are not in `userEditedFields`
   (`eventTime`, `itemDetails`, possibly `description` if untouched).
3. Issue a new `StreamGenUnifiedCreate` with `force_type = newType`. The new stream
   emits a `type` event matching `newType`, then proceeds with title /
   description / per-type fields just like the initial stream.
4. As each incoming event arrives, the reducer only writes to a field if
   that field is **not** in `userEditedFields`. (e.g., if user edited the
   title, the new stream's `title` event is dropped.)

**Save dispatch.** When `streamComplete && _requiredFieldsValid(state.type)`,
the Save button becomes enabled. Tap calls one of:

- `SaveGear` (Item) — passes `transfer_intent` from the Lend/Give toggle
- `SaveExperience` (event)
- `SaveRequest` (request) — RPC TBD by reading current request save path

then `Share*` per community in the existing pattern.

**Required-fields contract** (mirrors today's per-type flows):

| Type     | Required fields                                                |
|----------|----------------------------------------------------------------|
| Gear     | title, description, location, ≥1 community, media (>=1)        |
| Event    | title, description, location, time (or explicit TBD), ≥1 community |
| Request  | title, description, location, ≥1 community                     |

**Feature flag.** Follows the same `ENABLE_NOTIFICATIONS` pattern already in
the codebase:

- New key `ENABLE_UNIFIED_CREATE` in `app/env.dev.json`,
  `app/env.local.json`, `app/env.prod.json`. Stored
  as a string `"true"` / `"false"`.
- Read via `String.fromEnvironment('ENABLE_UNIFIED_CREATE', defaultValue: 'false')`
  in `app/lib/core/config/environment.dart`, exposed as a typed bool
  `Environment.enableUnifiedCreate`.
- Build-time only — flipping the flag requires an app rebuild. No
  RemoteConfig, no runtime toggle.

When true, the create button opens `UnifiedCreateModal`. When false, it
opens the existing legacy chooser that fans out to per-type modals.

### Server

Net-new packages:

```
server/services/unified_create/      # New RPC implementation
  service.go
  stream_gen.go               # StreamGenUnifiedCreate handler
  classifier.go               # If we pick two-call topology, lives here
  doc.go
  README.md

server/ai/
  prompts_unified.go          # New prompt(s); existing prompts.go untouched
```

**Classifier lives on `ai.Provider`.** As of #1907 (P2.16) the classifier
is a method on the shared `ai.Provider` interface
(`ClassifyUnifiedCreate(ctx, in)`), implemented by every provider
(Anthropic, Gemini, OpenAI) and routed through the same
`FallbackProvider` chain as every other AI surface. The
`server/services/unified_create` package consumes it through a narrow
service-local adapter (`unified_create.NewProviderClassifier(p)`) so the
service stays decoupled from the full `Provider` surface. No
classifier-specific fallback infrastructure exists.

**Image-mode classifier receives image bytes.** As of #1939, image-mode
unified-create requests resolve `media_id` → presigned URL → `*DetectionImage`
in the `unified_create` service before calling the classifier; the
classifier provider attaches the image as a vision content block
alongside the text prompt. Before #1939 the classifier image prompt
was text-only ("imagine an image is attached"), so the model
classified on its prior rather than the actual photo — defaulting to
gear via prompt instruction. Now the model reads the photo and
classifies appropriately (event flyer → event, lost-pet poster →
request, product photo → gear). The same `*DetectionImage` is not yet
threaded forward to the per-type image-streaming call, so the per-type
generator still re-resolves the signed URL — a small optimization
tracked as a follow-up.

**Approach (Phase 1 = classifier topology).** The first server implementation
calls a tiny "classify" prompt that returns `{type, confidence, brief_notes}`,
then delegates to the existing per-type generator. Streaming-wise:

1. Stream begins.
2. If `force_type` is set on the request: skip step 3, emit `type = force_type`,
   and go straight to step 4. This is the user-flipped-type path.
3. Otherwise: classifier returns type. Server emits `type` event.
4. Server invokes the existing per-type prompt (with brief notes carried over to
   avoid re-tokenizing the input). Existing per-type events (`title`,
   `description`, `geocoded`, `media_ready`, `time`) are forwarded as-is.
5. Server emits the appropriate per-type terminal payload boxed inside a
   `StreamGenUnifiedCreateResponse.final` (see RPC contract below).

This path is **strictly additive** on the server. Touching `prompts.go` only
to add the classifier; existing per-type prompts compile unchanged.

**Migration path to unified prompt (Phase 2+).** Once we have eval data on the
classifier and the per-type prompts, we collapse to a single prompt that emits
type + all fields in one shot. The client contract does not change.

## LLM design

This is the hard part. The two candidate topologies:

### Option A — Two-call (classify-then-delegate)

```
[user input] ─► [Classify prompt, ~200 tok in / ~50 tok out, small model]
                        │
                        ▼
                {gear|event|request, confidence, brief_notes}
                        │
                        ▼
                [Per-type prompt — existing, unchanged]
                        │
                        ▼
                Streaming events to client
```

**Pros:**
- Existing per-type prompts stay golden. We don't risk regressing any of them
  while shipping the UX.
- Each prompt evolves independently.
- Classifier failure is contained: if classify times out, fall back to a
  "default to gear" heuristic and emit `type=gear` with a low confidence.
- Easy to A/B server-side later by swapping the classifier without touching
  the per-type prompts.

**Cons:**
- Two sequential LLM calls = worst-case 2× latency to first token. If we use
  the same model and pay full TTFT each time, this kills the UX.
- Tokens duplicated: classifier prompt sees the input, then the per-type prompt
  sees a near-duplicate version of the input plus notes.

**Latency mitigation if we ship this:**
1. **Classifier on the server's current default model** (per
   `server/ai/provider_factory.go`). We deliberately do **not** introduce
   a separate "fast classifier" model in Phase 1 — it adds operational
   surface area and the per-type prompts already use this model. Target
   classifier round trip ≤ **400 ms p50, 800 ms p95** with the short
   200-token system prompt. If we miss this target after tuning, we revisit
   in Phase 2.
2. **Streaming the classifier.** As soon as the classifier emits `type:gear`
   (which we can extract from a 1-token decision), kick off the per-type call.
3. **Reuse the input.** The per-type prompt receives both the original user
   input and the classifier's "brief_notes" so it can skip re-extracting.
4. **Speculative pre-fetch.** When confidence is high, start the per-type call
   *before* the classifier finishes streaming its final token. (Phase 1.5
   optimization, not Phase 1.)

### Option B — One-call (unified prompt)

```
[user input] ─► [Unified prompt, ~800 tok system / variable tok user, capable model]
                        │
                        ▼
                Streaming events: type → title → description → fields → final
```

**Pros:**
- Single LLM round trip. Best possible TTFT.
- No duplicated input tokens.
- The model can refine its type decision as it generates (e.g., if title
  generation reveals the input is actually a request, the type field updated
  before the title is emitted is recoverable).

**Cons:**
- One prompt that has to do everything = longer system prompt, more brittle.
- Hard to evolve: tweaking the gear extraction rules risks regressing event
  extraction in unrelated ways.
- Token budget tighter for small models. The current per-type prompts in
  `server/ai/prompts.go` are already 100–300 lines of careful instructions each;
  combining them naively triples that.

### Recommendation

**Ship Phase 1 with Option A (two-call classifier).** Migrate to Option B
opportunistically based on eval data. The `StreamGenUnifiedCreate` contract supports
both, so the migration is invisible to the client.

### Classifier prompt sketch

The classifier is intentionally tiny. Output is structured (JSON) and the model
chooses from a closed set.

```
You are a content type classifier. Given the user's text (or an image / URL),
return JSON of the form:

  {"type": "gear" | "event" | "request", "confidence": 0.0–1.0, "notes": "<one sentence>"}

Rules:
- "gear" = user has (or is offering) a physical item. Whether to lend or
  give it is decided by the user later — do not try to guess.
- "event" = user is announcing or planning an activity at a time/place.
- "request" = user is asking for help, an item, or someone's time.

Examples:
- "Going for a hike tomorrow morning at Mt Sanitas" → event
- "Looking for someone to hit Mt Sanitas with me tomorrow" → request
- "Climbing rope, used a few times, happy to lend" → gear
- "Hoka Speedgoats, free to a good home" → gear
- [image of a power drill] → gear

Return only the JSON.
```

Target: ≤ 200 tokens system + variable user input. Server's current default
model; **must stream**.

### Eval coverage

We need an eval set for the classifier covering:

- Each of the 3 types × 3 input modes (text, image, URL) = 9 buckets.
- ≥ 20 golden inputs per bucket.
- An "ambiguous" bucket where 2 types are reasonable — score on consistency
  with the user-intent label, not strict accuracy.
- Mode-specific edge cases: image of a flyer (event), image of a product
  page screenshot (URL-like → gear), image of a tool (gear), URL to an
  Eventbrite page (event), URL to an Amazon listing (gear).

Builds on `server/ai/eval/` infrastructure. New corpus lives in
`server/ai/eval/testdata/unified_create_classifier_goldens.json`. Threshold
gate via existing `threshold_test.go` pattern.

### Token budget table (Phase 1)

| Path                   | System tok | User tok (typ.)  | Output tok | TTFT target  |
|------------------------|-----------:|-----------------:|-----------:|-------------:|
| Classifier             | ~200       | ~50              | ~30        | ≤ 400ms p50  |
| Existing gear prompt   | ~1200      | ~80              | ~250       | ≤ 1.2s p50   |
| Existing event prompt  | ~1500      | ~80              | ~300       | ≤ 1.2s p50   |
| Existing request prompt| ~900       | ~80              | ~200       | ≤ 1.0s p50   |
| **Two-call total p50** | —          | —                | —          | **≤ 1.5s**   |

If Phase 1 misses the ≤ 1.5s TTFT target, Phase 1.5 = speculative pre-fetch.
If still missing, that forces Phase 2 = Option B.

## RPC contract: `StreamGenUnifiedCreate`

Single new streaming RPC, lives in a new `UnifiedCreateService`. Field numbers reflect
introduction order; we reserve gaps for future per-type events.

```proto
// proto/ripls/api/unified_create_service.proto
syntax = "proto3";

package ripls.api;

import "ripls/api/gen_stream.proto";
import "ripls/api/gear_service.proto";       // for DetectedGearItem
import "ripls/api/experience_service.proto"; // for ExperienceTimeExtraction, ExperienceMetadata
import "ripls/api/request_service.proto";    // for RequestMetadata
import "ripls/api/location.proto";           // for GeocodedLocation

option go_package = "go.ripls.org/ripls/server/gen/ripls/api;api";

enum DetectedContentType {
  DETECTED_CONTENT_TYPE_UNSPECIFIED = 0;
  DETECTED_CONTENT_TYPE_GEAR        = 1; // user-facing label: "Item"
  DETECTED_CONTENT_TYPE_EVENT       = 2;
  DETECTED_CONTENT_TYPE_REQUEST     = 3;
}

message StreamGenUnifiedCreateRequest {
  oneof prompt {
    string text        = 1;
    string media_id    = 2;
    string website_url = 3;
  }
  string location_id           = 4;
  double latitude_deg          = 5;
  double longitude_deg         = 6;
  int64  current_time_unix_sec = 7;
  string timezone              = 8;

  // Optional: caller forces the type and the server skips the classifier.
  // Used when the user has flipped the type selector after an initial
  // generation; the client re-issues the stream with the new type so
  // type-specific fields (gear item details, event time, etc.) are
  // populated by the appropriate per-type generator.
  // When unset (the default), the server runs the classifier.
  optional DetectedContentType force_type = 9;
}

// Terminal payload carries one of the per-type "final" shapes. Reuses
// existing response messages so existing Save* paths can be reused without
// translation.
message StreamGenUnifiedCreateFinal {
  DetectedContentType type = 1;
  oneof final_payload {
    GenGearResponse        gear        = 2;
    GenExperienceResponse  experience  = 3;
    GenRequestResponse     request     = 4;
  }
  // Classifier confidence (0.0–1.0). Surfaced for analytics, not UX.
  // Reserved tag 5 — was transfer_intent in an earlier draft; not needed
  // because lend-vs-give is a user toggle, never a server inference.
  reserved 5;
  float classifier_confidence = 6;
}

// Variants listed in semantic order (type → identity → location → media → time
// → terminal). Non-terminal events may arrive in any order and any subset
// before the terminal event. Streams are terminated by exactly one of `final`
// or `error`.
message StreamGenUnifiedCreateResponse {
  oneof event {
    DetectedContentType         type        = 1;
    string                      title       = 2;
    string                      description = 3;
    GeocodedLocation            geocoded    = 4;
    MediaReady                  media_ready = 5;
    ExperienceTimeExtraction    time        = 6; // event-only
    StreamGenUnifiedCreateFinal       final       = 7;
    GenStreamError              error       = 8;
    // Reserved for future:
    //   detected_gear_item gear_details = 9; // brand/model/weight/value
  }
}

service UnifiedCreateService {
  rpc StreamGenUnifiedCreate(StreamGenUnifiedCreateRequest) returns (stream StreamGenUnifiedCreateResponse);
}
```

**Notes:**

- `type` event arrives first (or near-first) in nearly all real flows. Client
  must tolerate other events arriving before `type` and queue them.
- `time` is emitted only when `type ∈ {EVENT}`. Other events ignore it client-side
  if the user has flipped to non-event.
- We deliberately do **not** unify the per-type response messages into a single
  flat message — reusing the existing shapes means we can reuse existing save
  RPCs and existing client conversion utilities. The cost is a `oneof` in the
  terminal payload, which is cheap.

## Phasing / issue breakdown

Each phase is a shippable increment. Phase 1 ships to TestFlight behind the
flag; Phase 2 is the LLM efficiency pass; Phase 3 is rollout.

### Phase 1 — End-to-end skeleton (flag off in prod)

| # | Issue                                                          | Owner    | Notes |
|---|----------------------------------------------------------------|----------|-------|
| 1 | Define `unified_create_service.proto` + generate                      |          | Adds `StreamGenUnifiedCreate` RPC + `DetectedContentType` enum |
| 2 | Server: classifier prompt + eval seeds                         |          | `prompts_unified.go`, `server/ai/eval/testdata/unified_create_classifier_goldens.json` |
| 3 | Server: `UnifiedCreateService.StreamGenUnifiedCreate` (classify-then-delegate) |      | New `server/services/unified_create/`; delegates to existing per-type fanouts |
| 4 | Server: structured logging + Cloud Monitoring policy           |          | Per `docs/server/observability.md`; alert on `error` events |
| 5 | Client: `UnifiedCreateState` + view model + streaming actions  |          | Mirrors gen_*_streaming_actions.dart, single VM |
| 6 | Client: input drawer (text / image / URL) — glass styled       |          | Reuses `widgets/modal/glass/glass_sheet.dart`; new tabs widget |
| 7 | Client: preview card with cascade + type selector              |          | New widgets under `widgets/create/glass_preview_card.dart`. Selector is a 4-segment chip row; tapping a non-selected chip calls `flipType` which re-issues `StreamGenUnifiedCreate` with `force_type` and preserves user-edited fields. |
| 8 | Client: item-details push-sheet (gear loan/give)               |          | Reuses `glass_inset_card.dart` + standard glass primitives |
| 9 | Client: save dispatch (per-type Save\* + Share\*)              |          | No new RPCs; reuses existing |
| 10| Feature flag wired (`ENABLE_UNIFIED_CREATE` in `app/env.*.json`) |        | Same pattern as `ENABLE_NOTIFICATIONS`; default false in prod, true in dev. |
| 11| Tests: viewmodel + integration                                 |          | ViewModel-first per `docs/client/testing.md` |

**Validates:** A single user can flip the flag in dev and complete a full
gear, event, and request creation flow with the new UX.

### Phase 2 — LLM efficiency + observability

| # | Issue                                                          | Notes |
|---|----------------------------------------------------------------|-------|
| 12| Classifier latency dashboard (Cloud Monitoring)                | TTFT and round-trip for classifier specifically |
| 13| Speculative pre-fetch in `StreamGenUnifiedCreate`                    | Begin per-type prompt before classifier completes when confidence threshold hit |
| 14| Eval threshold gate for classifier accuracy                    | Build on `server/ai/eval/threshold_test.go` |
| 15| Eval threshold gate for round-trip latency                     | Test that p95 stays ≤ target |
| 16| Multi-provider classifier via the existing provider abstraction (#1907) | **Done.** Chose Option A from the plan (`docs/issues/1907-multi-provider-classifier.md`): added `ClassifyUnifiedCreate` to `ai.Provider`, implemented on Anthropic / Gemini / OpenAI, fallback + load-balancing inherited for free. Eval runner runs per-(provider, model) pair under `//go:build benchmark` via the shared `Test*Prompt` harness. Targeted `Provider` refactor (collapse short structured-output methods onto a shared primitive) deferred to a follow-up. |

**Validates:** Latency meets targets in [Goals](#goals) on prod-like load, and the classifier rides the same fallback path as every other AI surface.

### Phase 3 — Rollout

| # | Issue                                                          | Notes |
|---|----------------------------------------------------------------|-------|
| 16| Enable flag in dev build                                       | Internal dogfood |
| 17| Enable flag in TestFlight                                      | External beta |
| 18| Enable flag in app store release                               | Public |
| 19| Deprecate per-type modals (post-rollout)                       | Delete `create_gear_modal.dart` and friends, fold their tests into unified VM. Keep server per-type Gen RPCs for community + as fallback. |

**Validates:** No regressions in create funnel. Same or better conversion vs.
per-type baseline.

### Phase 4 — Option B migration (deferred)

Track separately. Becomes a candidate only if Phase 2 telemetry shows
classifier+delegate cannot hit latency targets, **or** if the per-type prompts
start diverging in ways that benefit from a single canonical extraction.

## Decisions (resolved in design review)

1. **Feature flag mechanism.** Same pattern as `ENABLE_NOTIFICATIONS` — new
   `ENABLE_UNIFIED_CREATE` key in `app/env.*.json`, read in
   `app/lib/core/config/environment.dart`. Build-time only.
2. **Camera tab pipeline.** Follow the same model as the existing
   `StreamGen*` modals: upload to `media_id` first via `MediaRepository`,
   then dispatch `StreamGenUnifiedCreate`. Drawer shows "Uploading..." in
   between.
3. **URL fetch boundary.** Fetch once at the start; classify on title +
   meta description; hand body to the per-type generator. Avoids the
   double-fetch.
4. **Classifier model choice.** Server's current default model — same one
   the per-type prompts use today. No new model added in Phase 1.
5. **Server prompt-version flag.** Yes — add a `*_PROMPT_VERSION` constant
   in `prompts_unified.go` for safe iteration.
6. **Per-type save error.** Surface the error in the unified modal and let
   the user retry. No fallback to the legacy modal.
7. **RPC and service naming.** `StreamGenUnifiedCreate` on
   `UnifiedCreateService` in `proto/ripls/api/unified_create_service.proto`.
   Screen-attribution name: `unified_create`. The enum stays
   `DetectedContentType` (a generic classification name).
8. **Type selector.** Three chips — **Request · Event · Item** — with
   icon + text + filled background on the selected one. **Lend vs. give is
   a separate Lend/Give toggle within the Item flow**, defaulting to Lend,
   user-controlled, not server-classified. The classifier returns 3 types
   (not 4); `transfer_intent` is sent only at save time.
9. **Suggested prompts.** Static lists per category for Phase 1 (no live
   community fetch). Localised via ARB. Live-fetched and personalised
   variants are future work.
10. **Flip timing.** A flip is enabled as soon as the server emits its
    `type` event. Once a flip is triggered, the selector is disabled until
    that flip's re-stream reaches its terminal `final` event. Prevents
    nested in-flight flips and the races they would create.

## Related

- `docs/concepts/create/bottom-drawer-standalone.html` — visual + interaction
  prototype
- PR #1785 — concept introduction
- PR #1798, PR #1854 — frosted-glass modal foundation
- `app/lib/presentation/widgets/modal/glass/README.md` — glass primitives
- `docs/client/modals.md` — modal conventions
- `docs/proto_conventions.md` — request/response shape conventions
- `docs/server/architecture.md` — service organization
- `docs/server/observability.md` — structured logging + alerting
- `docs/client/testing.md` — testing patterns
- `server/ai/eval/README.md` — eval harness
