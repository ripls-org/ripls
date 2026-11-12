# Issue #1648: Image-mode community creation: drop the placeholder and add a real StreamGenCommunityFromMedia RPC

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1648
**Filed:** 2026-04-30 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit Notes (2026-05-07)

This plan was audited against `docs/server/architecture.md`,
`docs/server/observability.md`, `docs/client/architecture.md`,
`docs/client/testing.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, and `docs/client/caching.md`. Changes
made:

- **Logging field set tightened** — added explicit `LoggerWithContext(ctx)`,
  `logging.MaskEmail(authInfo.Email)`, and ERROR-level expectation for
  AI open-failure per `observability.md` ("ERROR is reserved for system
  failures"; AI provider failures qualify).
- **Error wrapping** — clarified that streaming errors go through
  `sender.emitError` (existing pattern) and that any *unary* error path
  must use `connecterr.Internal(ctx, op, err, kv...)` to avoid leaking
  raw storage / SDK text to the wire (per
  `architecture.md` §"Error wrapping").
- **Goroutine safety** — flagged that the new image-mode handler does not
  spawn fan-out goroutines (no Pexels branch), so `logging.GoSafe` is not
  needed. CI's `check-goroutines` would fail if a bare `go func()` slips
  in; the plan now states this invariant explicitly.
- **Test layering** — the client viewmodel test was already
  `ProviderContainer + Mockito + fake Stream` per `testing.md`, but this
  is now called out explicitly. Added note that no widget tests are
  needed (logic-only change).
- **i18n / accessibility** — no new user-visible strings or UI changes
  introduced; the AI generates text server-side. ARB files are not
  touched. Added a one-line note so future readers don't search for it.
- **README hygiene** — added a task to update
  `server/services/community/README.md` per CLAUDE.md's multi-file
  directory rule.
- **Proto comments** — re-confirmed the comment update in Phase 1
  does not mention vendor names ("Pexels", "Anthropic", etc.) per
  proto-comments rules.
- **Caching** — confirmed no cache invalidation work is needed: the
  user's uploaded media flows through the existing
  `MediaRepository.addMedia` path which already emits the correct
  cache key, and no new lists or cached entities are created on the
  hot path.

## Problem Statement

The community creation modal exposes an image input mode, but the streaming
path silently degrades it. Today, when the user picks an image:

1. `community_repository.dart` uploads the image via `MediaRepository`.
2. It then calls `streamGenCommunity` with the **literal string**
   `'Generate a community based on the provided image'` and **drops the
   `mediaId`** entirely (`// ignore: unused_local_variable`).
3. The server, which has no image-mode pathway for community at all, runs a
   text-mode generation against that placeholder string.
4. The user gets a generic "Community Hub" community with a generic Pexels
   stock image — neither the user's image nor any visual content from it is
   ever associated with the resulting community.

This was a known shortcut from #1540 that was never followed up on. Since
PR #1651 (merged 2026-05-04) the streaming path is the only path users hit,
so this is now the user-visible default behavior for the New Image flow.

The issue proposes a new `StreamGenCommunityFromMedia` RPC that mirrors a
hypothetical `StreamGenGearFromMedia` / `StreamGenRequestFromMedia`. The
**actual codebase pattern is different**: `StreamGenGear` and
`StreamGenRequest` are single RPCs that take *either* `prompt` *or*
`media_id` (proto3 string fields, "exactly one of" validated server-side).
There is no separate streaming-from-media RPC for any other resource. This
plan recommends following the existing pattern, not the one in the issue
description.

## Context from Comments

Brian reproduced manually on a debug APK (Pixel, dev server, plant photo
input):

- Community name returned: `Community Hub` — a content-free default.
- Community image: a generic Pexels stock photo, not the uploaded image.
- `flutter` logs confirm the only `mediaId` reaching the created community
  is the Pexels one from the AI's `mediaReady` event (`mediaIds=1`); the
  user's plant image is uploaded but orphaned.

The repro confirms both halves of the bug — image content ignored and
user `mediaId` dropped — and adds urgency: with #1651 merged, the streaming
path is the only path.

No maintainer pushback. No alternate proposals raised in the comments.

## Potential Solutions

### Option A: Add `media_id` to `StreamGenCommunityRequest` (mirrors gear / request)
- **Approach:** Add a `media_id` field to the existing
  `StreamGenCommunityRequest` proto. Server validates "exactly one of
  `prompt`, `media_id`" (returns `GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT`
  on violation, mirroring `gen_ai_streaming.go:78-86`). When `media_id` is
  set, route to a new `streamGenCommunityFromImage` handler that resolves
  the media, generates a presigned URL, calls a new
  `GenerateCommunityFromImageStreaming` AI method, and emits the same
  `title` / `description` / `media_ready` / `final` event shape.
- **Pros:** Matches the existing pattern in `StreamGenGearRequest` and
  `StreamGenRequestRequest` exactly — no new shape for clients to learn,
  no proto-convention exception. Single repository method on the client
  (`streamGenCommunity` already takes `imagePath`); only the inner
  service-layer call changes. Wire-compatible — adding a proto3 string
  field is backward-safe.
- **Cons:** The request message gains a "exactly one of" semantic. This
  pattern is already in `StreamGenGearRequest` (3-way) and
  `StreamGenRequestRequest` (2-way), so it's not new — but it's slightly
  more error-prone than two separate RPCs with disjoint inputs.
- **Effort:** Medium. Proto + server handler + AI provider methods +
  client repository routing + tests. The AI provider work
  (`GenerateCommunityFromImageStreaming` + Anthropic + Gemini + Fallback
  implementations) is the bulk of it.

### Option B: New `StreamGenCommunityFromMedia` RPC (issue's original proposal)
- **Approach:** Define a separate
  `StreamGenCommunityFromMediaRequest` message and a separate RPC. Reuse
  `StreamGenCommunityResponse` since the event shape is identical.
- **Pros:** Strict separation between text and image inputs at the
  proto layer; simpler validation per RPC; smaller blast radius for a
  text-only change.
- **Cons:** Diverges from the codebase pattern. Gear and request both use
  a single streaming RPC with `media_id`; this would be the only resource
  with a separate from-media stream. Two RPCs means two service-method
  registrations, two Connect-Go handlers, and two client service methods
  for what the existing precedent does in one. CLAUDE.md proto-conventions
  forbid reusing request types but explicitly *allow* reusing response
  types when the event shape is genuinely identical, so the duplication
  cost is uneven.
- **Effort:** Medium-large. Same AI provider work as Option A, plus
  duplication of the streaming wire path.

### Option C: Use the existing unary `GenCommunityFromMedia` from streaming client
- **Approach:** Don't add streaming for image mode. Have the client's
  image branch call a new unary `GenCommunityFromMedia` and shim it into
  the streaming-shaped event flow (single `final` event after the unary
  RPC returns).
- **Pros:** No streaming server work; no changes to AI streaming
  interfaces; can ship in days.
- **Cons:** Image mode loses the visible benefit of streaming (early
  title / description rendering). Inconsistent with text mode. Defers
  the actual fix and likely re-creates the gap when the next streaming
  performance push lands.
- **Effort:** Small.

## Recommendation

**Option A.** It is the only option that matches the existing pattern in
`StreamGenGearRequest` and `StreamGenRequestRequest`. Adding a `media_id`
field to `StreamGenCommunityRequest` is wire-compatible and reduces the
cognitive overhead of the API surface for downstream clients (one
streaming RPC per resource, with input mode determined by which field is
set). Option B trades that consistency for a marginally cleaner per-RPC
contract — not worth diverging from the precedent. Option C is a hack
that intentionally regresses the user experience.

The bulk of the work is the AI-provider side: there is currently no
`GenerateCommunityFromImage` method anywhere (provider interface,
Anthropic, Gemini, Fallback). That has to be added before the server
handler can route to it.

## Open Questions

- **Should image-mode community generation produce a stock image too, or
  use the user's uploaded image as the community media?** — Recommendation:
  **use the user's image**, suppress the Pexels fan-out for image mode.
  The user's image is the more meaningful visual signal for the community
  they're creating, and the manual repro showed Pexels overriding it as a
  bug, not a feature. Mirrors how `StreamGenGearFromImage` skips the
  Pexels branch (image-mode gear has no Pexels fan-out per
  `gen_ai_streaming.go:121-122`).
- **What should the AI prompt extract from the image?** — Recommendation:
  community name + description + (optional) suggested region. Same
  schema as text-mode `CommunityGeneration`, so the streaming `final`
  event lands a `GenCommunityResponse` of the same shape. No new client
  rendering work.
- **Should the AI also emit `search_keywords` for image mode?** —
  Recommendation: **no**. Without Pexels fan-out the keywords have no
  consumer; emitting them just adds tokens. Match gear's image-mode
  behavior (no keywords emitted, no media_ready Pexels event).
- **Default behavior when the user provides both an image AND text in the
  modal?** — Recommendation: prefer image mode (richer signal), drop the
  text. The current modal already enforces single-mode at the UI layer
  (`CreationInputMode.image` vs `.text`); server-side reject as
  `INVALID_ARGUMENT` to match gear / request validation.
- **Where should the image-mode AI prompt live?** — Recommendation: same
  place as the text-mode community prompt
  (`server/ai/prompts.go`), gated by a `community_image` key. Per the
  token-efficient prompts feedback memory, keep additions minimal — image
  prompts are short by nature.

## Implementation Plan

### Phase 1: Server scaffolding (proto + AI provider)
**Goal:** Add the wire surface and the AI capability without wiring it
to the streaming handler yet.
**Tasks:**
- [ ] Add `string media_id = 3;` to `StreamGenCommunityRequest` in
  `proto/ripls/api/community_service.proto`. Update the message comment
  to clarify "exactly one of `prompt`, `media_id`".
- [ ] Add `GenerateCommunityFromImage` and
  `GenerateCommunityFromImageStreaming` to the `Provider` interface in
  `server/ai/provider.go`, mirroring `GenerateRequestFromImage{,Streaming}`.
- [ ] Implement both methods on `AnthropicProvider`, `GeminiProvider`,
  and `FallbackProvider` (the fallback wrapper just delegates).
- [ ] Add a new `community_image` prompt entry to `server/ai/prompts.go`,
  reusing the text-mode response schema (`CommunityGeneration`) so the
  parsing path is unchanged. Keep it short.
- [ ] `npm run build` clean (lint + compile). `npm run generate` clean.
- [ ] Unit tests for the new AI methods (mock HTTP, assert the prompt
  contains the image URL in the appropriate role).
**Validates:** AI provider can produce community content from an image
end-to-end, callable from a Go test without touching the streaming server.

### Phase 2: Server streaming handler + tests
**Goal:** Make the streaming RPC actually serve image-mode requests.
**Tasks:**
- [ ] In `server/services/community/gen_streaming.go`, add a
  `streamGenCommunityFromImage` method modeled on
  `streamGenGearFromImage` (`server/services/gear/gen_ai_streaming.go:123`).
  Resolves media via `storage.GetByID`, validates `media.UserId == authInfo.UserID`,
  generates a presigned URL via the bucket, calls
  `GenerateCommunityFromImageStreaming`, emits `title` / `description`
  events as JSON fields close, terminates with `final` carrying a
  `GenCommunityResponse` containing the user's `media_id`.
- [ ] Update `streamGenCommunityCore` to validate "exactly one of" and
  route to either the existing text-mode path or the new image-mode
  handler. Use the same `GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT` shape
  as gear / request.
- [ ] Suppress the Pexels fan-out branch for image mode — the
  `media_ready` event should carry the **user's** `media_id`, not a
  Pexels result. The `final` event's `media_ids` is the user's `media_id`.
- [ ] Unit tests in `gen_streaming_test.go` covering: (a) image-mode
  happy path emits title/description/media_ready/final with user's
  media_id; (b) media not found → INVALID_ARGUMENT; (c) media owned by
  different user → INVALID_ARGUMENT; (d) both `prompt` and `media_id`
  set → INVALID_ARGUMENT; (e) AI streaming open-failure → AI_PROVIDER_FAILED.
- [ ] Integration test (build-tag-gated, `//go:build integration`) that
  drives the real AI provider with a fixture image and asserts the
  generated `title` / `description` references the image content (not a
  generic "Community Hub").
- [ ] **Logging** (per `docs/server/observability.md`):
  use `logging.LoggerWithContext(ctx).With("operation",
  "StreamGenCommunity", "user_id", authInfo.UserID, "user_email",
  logging.MaskEmail(authInfo.Email), "media_id", mediaID)`. Stream-open
  failures from the AI provider log at **ERROR** ("system failure").
  Stream-end success logs at INFO with `stream_duration_ms` and
  `input_mode="image"`. INVALID_ARGUMENT user-input issues (media not
  found, cross-user access, both fields set) log at WARN.
- [ ] **Error wrapping**: streaming errors continue to use
  `sender.emitError(...)` per the existing pattern. Any non-stream
  unary error introduced (none expected) must go through
  `connecterr.Internal(ctx, op, err, kv...)` per `architecture.md`
  §"Error wrapping" — bare `connect.NewError(connect.CodeInternal, ...)`
  is gated by `scripts/check_codeinternal.js`.
- [ ] **No new goroutines.** The image-mode handler reads from the AI
  provider's existing channels (no Pexels fan-out). If any new
  fire-and-forget goroutine is added during implementation, it must use
  `logging.GoSafe(ctx, name, fn)`; CI's `check-goroutines` enforces this.
- [ ] Update `server/services/community/README.md` to mention the new
  image-mode pathway.
**Validates:** End-to-end server flow accepts a `media_id`, reads the
image, drives the AI, and emits the correct event sequence — without
client changes.

### Phase 3: Client routing + cleanup
**Goal:** Eliminate the placeholder string and route image-mode calls to
the new server pathway.
**Tasks:**
- [ ] Update `CommunityService.streamGenCommunity` (Dart) to accept
  `mediaId` alongside `prompt`.
- [ ] Update `community_repository.dart` image branch:
  upload media → call `streamGenCommunity(mediaId: id, region: region)`
  with no `prompt`. Remove the placeholder string and the
  `// ignore: unused_local_variable` comment.
- [ ] Update `community_creation_view_model.dart`:
  remove the "follow-up issue" comment; add a code comment only if it
  documents a non-obvious invariant (otherwise drop entirely per
  CLAUDE.md "default to no comments").
- [ ] Add a viewmodel test (per `docs/client/testing.md` — ViewModel-first,
  `ProviderContainer` + Mockito) that drives `generateCommunityStreaming`
  with an `imagePath` and a fake `Stream<StreamGenCommunityResponse>`,
  asserting the user's `mediaId` ends up in `genCommunityProvider.mediaIds`
  (the manual-repro bug fixed at the test level). No widget tests
  required — this is a logic-only path; widget behavior is unchanged.
- [ ] Manual repro on emulator: select a personal photo, confirm the
  resulting community name / description references the image and the
  community media is the uploaded image (not a generic Pexels stock photo).
- [ ] `flutter analyze` clean. `npm run test:app` clean.
- [ ] **No ARB / l10n changes.** The AI generates community text
  server-side; no new user-facing strings are introduced. `app_en.arb`
  and `app_es.arb` are not touched.
**Validates:** The user-visible bug from Brian's manual repro is gone:
the image content shapes the community, and the user's media is
preserved as the community's image.

## Related Issues

- #1540 — Wired streaming for community creation (this issue is the
  missing image-mode half).
- #1539 — Wired streaming for gear-image and request-image flows (the
  precedent this plan follows).
- #1634 — Removed the unary fallback, making this gap visible to all
  users.
- #1647 — Other community-streaming bugs in the preview modal (closed
  2026-05-06 via #1651).
