# Issue #2245: Request composition: AI-break-down-the-ask flow with pre-claim toggle

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2245
**Filed:** 2026-05-29 | **Last activity:** 2026-05-29
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/App Design/compose-to-claim-prototype.html](../cowork/App%20Design/compose-to-claim-prototype.html), [docs/client/needs.md](../client/needs.md), [docs/issues/2240-claim-gear-linking.md](2240-claim-gear-linking.md)

> **Simplification (2026-05-29).** The "kind" classification per piece
> (LEND / DO / COVER) was scoped OUT after the initial pass — not worth the
> added AI cost or the cross-cutting complexity for a v1. All references to
> a per-Need `kind` enum, kind tags in the picker, kind-aware claim UI, and
> per-piece kind editing have been removed. The plan now ships the
> chip+paste+pieces+pre-claim UX without classification. The prototype's
> kind tags are visual chrome only — we don't store or render them. If kind
> classification becomes valuable later, the AI prompt can be extended in
> one place and a `NeedKind` enum added to `PlanningNeed` then; nothing in
> this plan precludes that follow-up.
>
> **Reuse over new RPC (2026-05-29).** During Phase 1a implementation we
> dropped the proposed `GenComposeSuggestions` RPC entirely. The chip-gen
> infrastructure already exists: `ai.Provider.GenerateRequestSuggestions`
> is called post-publish by `lazyFillRequestSuggestions` and the resulting
> chips are exposed on `ListRequestNeedsAndContributionsResponse`
> (`additional_asks` / `breakdown_pieces` / `offer_ideas`). Instead of a
> new wire surface, we extended the existing `GenRequestResponse` (and its
> image/streaming siblings) with the same three repeated-string fields,
> populated sequentially after the AI primary stream completes via the
> same `GenerateRequestSuggestions` call. Compose reads the chips off
> the `GenRequest` response it was already going to consume — zero new
> RPC, no new repository method for fetching chips. Trade-off: chips
> add ~1–2 s to the GenRequest tail latency. If that's user-visible we
> can add a streaming `chips` event variant later.
>
> **Implementation outcome (2026-05-29).** All four phases landed in a
> single sweep. Server: chip plumbing on `genRequestSuggestionsBestEffort`
> at all three callsites (unary text, unary image, streaming) plus the
> new `BatchClaimRequestNeeds` RPC handler in `needs.go`. Client:
> `RequestRepository.batchClaimNeeds`, `requestComposeProvider`
> (NotifierProvider.autoDispose) carrying pieces / pre-claims with
> `publish()` running the three-step
> SubmitRequest → BatchAddRequestNeeds → BatchClaimRequestNeeds, and
> `RequestComposeSection` embedded inline in `RequestPreviewModal`. ARB
> strings (en/es), a11y primitives (`Tappable` / `Toggle` / `IconAction`),
> and AppColors tokens used throughout. Test coverage: 5 new server
> tests (`batch_claim_test.go`), 4 chip-plumbing tests
> (`gen_chips_test.go`), 16 ViewModel tests
> (`request_compose_view_model_test.dart`), 4 widget tests
> (`request_compose_section_test.dart`). All passing. `npm run
> lint:dart:i18n`, `npm run lint:dart:l10n-parity`, `npm run
> lint:dart:a11y` clean. `go build -tags=integration ./server/...`
> clean. The compose stage is wired but the kind-aware claim picker
> branch and analytics events are deferred to a polish PR.
>
> **Audit pass (2026-05-29).** Reviewed against
> [docs/client/architecture.md](../client/architecture.md),
> [caching.md](../client/caching.md), [i18n.md](../client/i18n.md),
> [accessibility.md](../client/accessibility.md),
> [testing.md](../client/testing.md),
> [docs/server/architecture.md](../server/architecture.md), and
> [docs/server/observability.md](../server/observability.md). Compliance
> notes (repository-owned caching, AsyncNotifier `ref.mounted` checks,
> `runDebounced`-based debounce tests, `SemanticAnnouncer` + `LiveRegion`
> for status, `prompt_version` on AI logs, `target_request_id` log field,
> `connecterr.Internal` wrapping, server file-size split) all carry
> through to this simplified plan.

## Problem Statement

Today the Request creation flow produces a **monolithic Need**: the asker types
a title + optional description, an LLM may suggest "additional asks" /
"breakdown pieces" / "offer ideas" *after* the Request is created (these
already come back from `ListRequestNeedsAndContributions`, see
[request_needs_view_model.dart:108](../../app/lib/presentation/viewmodels/request_needs_view_model.dart#L108)),
and from there everything happens on the published Request screen. The asker
has no way during *composition* to:

1. **Decompose the ask into discrete pieces** — "yard cleanup" stays as one
   bucket every claimer fights over. The pieces version (saw, gloves,
   haul-debris, lunch) maps 1:1 to slots and reads like a checklist.
2. **Pre-claim pieces they're handling themselves** — the asker can't say
   "I'll bring the saw, I just need the gloves and the driver" at compose
   time; that has to happen post-publish in a separate step.

The [compose-to-claim prototype](../cowork/App%20Design/compose-to-claim-prototype.html)
shows a compose sheet that delivers both: AI-suggestion chips driven by the
title/sub text, a paste-a-list disclosure, a pieces list with inline rename,
a per-piece "I've got this" pre-claim toggle, and a Publish CTA that surfaces
both piece count and pre-claim count. On publish, each piece becomes a Need on
the new Request; pre-claimed pieces land with the asker's contribution already
applied.

## Context from Comments

**The issue has no comments yet** (filed 2026-05-29). Direction lives in the
issue body. No need to wait for discussion before planning — the audit pass
on this document is the next quality gate.

## Key codebase findings (from research)

- **`SubmitRequest` does not accept Needs in its payload.**
  [request_service.proto:104–124](../../proto/ripls/api/request_service.proto#L104-L124).
  Needs are always added post-creation via `AddRequestNeed` or
  `BatchAddRequestNeeds` ([request_service.proto:497–512](../../proto/ripls/api/request_service.proto#L497-L512)).

- **`ClaimRequestNeed` has no guard against the requester claiming their own
  Need.** Handler at
  [server/services/request/needs.go:334–433](../../server/services/request/needs.go#L334-L433).
  The auto-`ensureRequestOffer` only fires when the claimer is *not* the
  requester (line 418), so a requester self-claim is already a supported
  no-op of that side effect — the slot just gets filled by the asker. No
  proto change needed to support pre-claim.

- **The AI break-down generation already exists server-side.**
  [`buildRequestSuggestionsPrompt`](../../server/ai/prompts.go) emits
  `breakdown_pieces` labels (1–4 words). Today these are produced *after* the
  Request is created and returned by `ListRequestNeedsAndContributions`. The
  new compose flow needs them *before* publish — a new
  `GenComposeSuggestions` RPC reuses the same prompt at compose time, with
  **no schema extension** (we don't need a per-piece kind).

- **A latency-first classifier pattern already exists.** See
  [docs/issues/1932-classifier-latency-slim-response.md](1932-classifier-latency-slim-response.md):
  Gemini-2.5-Flash, slim prompt, JSON-schema-constrained output, ~300 ms
  target. The compose chip endpoint follows this pattern.

- **`UnifiedCreateModal` is the new entry path** behind
  `unifiedCreateEnabledProvider`; the legacy `RequestCreationModal` is
  flagged out for new users. Compose lives inside the Request branch of
  the unified flow — specifically, it extends
  [`RequestPreviewModal`](../../app/lib/presentation/screens/request/request_preview_modal.dart),
  which is where the user lands after generation/preview to finalize the
  Request before `SubmitRequest`.

- **No schema change to `PlanningNeed`.** Without a `kind` field there is
  nothing to add to the proto. `BatchAddRequestNeeds` already takes
  `{name, optional note, slots}` — that's the full piece payload. Wire-
  compatible by construction.

- **Parity is safe.** [needs.md:171–192](../client/needs.md#L171-L192)
  enumerates Intentional differences narrowly. This compose flow is
  Request-only; storage paths are unchanged so Experience parity is
  trivially preserved.

## Potential Solutions

The product shape is decided by the issue body + prototype. Two real choices
remain: AI source, and publish wire flow.

### AI suggestion source

#### Option S1: New `GenComposeSuggestions` RPC, reuse existing prompt

- **Approach:** New RPC takes `{title, sub, community_id}` and returns
  `{suggestions: [{label}], generation_id}`. Reuses
  [`buildRequestSuggestionsPrompt`](../../server/ai/prompts.go) as-is —
  same Gemini-2.5-Flash model, same fallback chain (Anthropic / OpenAI).
  The handler just plucks `breakdown_pieces` out of the existing output
  schema.
- **Pros:** Reuses a prompt that already works in production. Matches the
  slim-response, single-call pattern from #1932. Existing auth /
  rate-limit story. **Zero prompt or schema change** now that kind is
  dropped.
- **Cons:** One AI call per compose session (debounced + repo-cached
  within Stash TTL).
- **Effort:** Small on server (one new RPC handler), medium on client
  (debounce + repository method + ViewModel wiring).

#### Option S2: Client-side keyword extraction

- **Approach:** No server endpoint. Surface a fixed dictionary of noun
  chips when a hardcoded category matches the title (saw, gloves, lunch,
  etc.).
- **Pros:** Zero AI cost.
- **Cons:** Useless for unfamiliar asks. Reduces the feature to "paste-a-
  list + manual add" — the whole point is that an AI decomposes "yard
  cleanup" into ("Stump grinder", "Haul debris", "Provide lunch").
- **Effort:** Tiny but wrong.

### Publish wire flow

#### Option P1: Three-step (SubmitRequest → BatchAddRequestNeeds → batch claim)

- **Approach:** Client calls `SubmitRequest` to create the Request; on
  success, calls `BatchAddRequestNeeds` with all pieces; on success,
  calls a new `BatchClaimRequestNeeds` for the pre-claimed pieces. Single
  client-side spinner.
- **Pros:** No new transactional surface; reuses existing RPCs and the
  post-#2240 ClaimRequestNeed handler. Partial-success windows are
  recoverable.
- **Cons:** Three sequential RPCs adds a beat. Branched error story
  ("Posted, but couldn't add 2 of your 5 pieces — retry?").
- **Effort:** Small.

#### Option P2: Atomic `ComposeRequest` RPC

- **Approach:** New server RPC writes Request + all Needs + all
  pre-claims in one transaction.
- **Pros:** No partial-success window.
- **Cons:** New surface that duplicates `SubmitRequest` semantics
  (sharing, community-id validation, fan-out). Higher coupling. Higher
  effort.
- **Effort:** Large.

#### Option P3: Extend `SubmitRequest` with optional `pieces` field

- **Approach:** Add a repeated `pieces` field to `SubmitRequestRequest`;
  when non-empty, server runs the batch-add + pre-claim loop in the
  handler.
- **Pros:** No new RPC; backwards-compatible.
- **Cons:** `SubmitRequest` grows wider; complicates a heavily-used
  handler for a single caller.
- **Effort:** Medium.

## Recommendation

**AI source: Option S1 (`GenComposeSuggestions` RPC, reuse existing
prompt).** Reusing the prompt that already produces `breakdown_pieces`
post-publish is the obvious win — it just needs to fire at compose time.
Debounce on title-blur with a manual "re-extract" button; the repository
caches by `(title, sub, communityId)` within the global Stash TTL, so
repeats are free.

**Publish: Option P1 (three-step) with a new `BatchClaimRequestNeeds`** so
the pre-claim leg is one round trip instead of N. This keeps the
SubmitRequest contract clean (P3's expansion is tempting but pollutes a
heavily-used RPC for one caller), and reuses the existing
ClaimRequestNeed handler that already allows requester self-claim. P3 is a
straightforward optimization later if compose-publish latency feels off.

## Open Questions

- **AI chip endpoint cost / cadence.** — Recommendation: **debounce
  + single fire on first non-empty title (~600 ms after pause), with a
  manual "re-extract" affordance.** Matches the latency-first pattern from
  #1932. Repository-layer cache makes re-opens with the same prompt free
  within the global TTL.

- **Where does the Compose sheet live in the UI tree?** —
  Recommendation: **as a new stage inside `RequestPreviewModal`**, not a
  parallel modal. The current preview modal is where the user lands after
  Generate (text or photo) and edits before SubmitRequest; inserting a
  "Compose: break down" step between preview and submit keeps the
  existing back-stack and reuses the modal chrome.

- **Pre-claim slot accounting.** — Recommendation: **each pre-claimed
  piece starts with `slots=1` and the asker's contribution fills that one
  slot.** The prototype shows no UI for "I'm bringing 2 of these," and
  the existing `planning.ClaimSlot` bounds-check handles any abuse.

- **Pre-claim guard observability.** — Recommendation: **log pre-claim
  failures at WARN with `operation=BatchClaimRequestNeeds`,
  `target_request_id`, `need_id`, `failure_reason`** so we can spot the
  "Posted, but X pre-claims failed" UX path in production. Use
  `connecterr.Internal` for storage failures and
  `connect.CodeInvalidArgument` for slot-bounds failures.

- **Discard semantics.** — Recommendation: **discard prompts only when
  there's draft content** (title, sub, or any pieces). Empty drafts close
  silently. Match the prototype's `handleClose` flow exactly.

## Cross-cutting compliance notes

These rules apply across every phase below; calling them out once here so
the per-phase task lists stay short.

- **Architecture (MVVM + Riverpod 3).** New compose state lives on a
  dedicated `requestComposeProvider` `AsyncNotifier.autoDispose`. No
  `ChangeNotifier`, no `ref.read` in `build`. Per
  [architecture.md§"Async Safety in Notifiers"](../client/architecture.md#async-safety-in-notifiers),
  `AsyncNotifier.build()` disposal is automatic, but **methods other
  than `build()` (e.g. `requestSuggestions`, `publish`) MUST check
  `ref.mounted` after every `await` before assigning `state`**. Wrap the
  inner work in `AsyncValue.guard(...)` and add an explicit
  `if (!ref.mounted) return;` guard before each `state =` assignment.
  Disposal-safety test for in-flight AI suggestion calls and in-flight
  publish.
- **Repository ownership of caching.** Per
  [caching.md](../client/caching.md), ViewModels never own caches. AI
  compose suggestions are fetched via a new
  `RequestRepository.getComposeSuggestions(title, sub, communityId)`
  method that goes through `CacheService` with cache key
  `request:compose-suggestions:<sha-prefix-of-(title|sub|communityId)>`.
  The "re-extract" button calls
  `RequestRepository.refreshComposeSuggestions(...)` which invalidates
  first, then fetches. The compose ViewModel holds only the latest
  result in immutable Freezed state — no `Map<String, T>` cache. No
  gear-related cache invalidation triggered by this flow.
- **Accessibility primitives.** Every new tappable surface uses
  `Tappable` / `Toggle` / `IconAction` with `semanticsLabel` from
  `context.l10n.a11y*` per
  [accessibility.md](../client/accessibility.md). Modals open via
  `showAccessibleModal`. The per-piece "I've got this" toggle is a
  `Toggle` (announces selected state). **Status announcements:** the
  partial-success outcome ("Posted, couldn't add 1 piece — retry?") is
  announced via `SemanticAnnouncer.announce(context, msg)` (one-shot)
  the moment the toast appears. The AI-suggestion error banner — which
  persists on screen until dismissed — wraps the banner widget in
  `LiveRegion` so the screen reader re-announces if the error message
  changes.
- **AppColors only.** No raw color literals. Pieces, chip rows, and
  publish footer use existing `AppColors.xxx(context)` tokens.
- **i18n.** Every new string in both `app_en.arb` and `app_es.arb` with
  `@key` metadata per [i18n.md](../client/i18n.md);
  `npm run lint:dart:i18n` blocks raw literals,
  `npm run lint:dart:l10n-parity` blocks keys missing from either
  locale. ViewModels never resolve strings — they expose typed
  `UserError? error` on state and the widget calls
  `RpcErrorHandler.localize(state.error!, context.l10n)` in `build()`.
  **"Event" terminology rule does not fire here.** Compose is
  Request-only and reuses no Experience-scope copy.
- **Testing.** Per [testing.md](../client/testing.md): ViewModel
  coverage via `ProviderContainer` + Mockito mocks (`@GenerateMocks` on
  `RequestRepository` and the request service). `setUp` constructs the
  container with overrides; `tearDown` / `addTearDown` disposes it and
  `reset()`s mocks. Coverage: chip add/remove, paste-list split, inline
  rename commit/cancel, pre-claim toggle, publish three-step
  partial-success retry, disposal during AI call. **Debounce tests**
  use `runDebounced` from `app/test/helpers/fake_async_helpers.dart` to
  drive the 600 ms title-blur debounce deterministically — never
  `await Future.delayed`. **Cache-hit-vs-miss** for
  `getComposeSuggestions`: warm the repo, call again with the same
  prompt, assert `verifyNever(service.genComposeSuggestions(...))`.
  Coverage targets: ViewModels ≥ 90%, repositories ≥ 80%. Run with
  `flutter test --reporter expanded`.
- **Server observability.** Per
  [observability.md](../server/observability.md):
  `logging.LoggerWithContext` with `operation`, `user_id`,
  `user_email` (via `logging.MaskEmail`), `target_request_id` (NEVER
  `request_id` for entity IDs — that key is reserved for the HTTP
  correlation UUID), `need_id`, `duration_ms`, `error`. AI calls on
  `GenComposeSuggestions` also log `provider`, `model`, and
  `prompt_version`, mirroring the
  [`StreamGenUnifiedCreate` shape](../server/observability.md#unified-create-stream-streamgenunifiedcreate).
  Levels: ownership / slot-bounds rejections INFO/WARN (user error);
  storage / AI infra failures ERROR. Request-scoped RPC, NOT a
  background job, so `errs.TransientStreak` does not apply. Internal
  errors use `connecterr.Internal(ctx, op, err, kv...)`; user errors
  use `connect.NewError(connect.CodeInvalidArgument, err)`.
- **Server SQL efficiency.** `BatchAddRequestNeeds` (existing) and the
  new `BatchClaimRequestNeeds` use multi-row INSERT via `InsertBatch`,
  not a loop. Wrap in `storage.AssertMaxQueries` guard.
- **Parity contract.** No new "Intentional differences" entry in
  [needs.md](../client/needs.md). No proto change to shared models;
  Experience-scope compose UX is explicitly deferred but trivially
  parity-preserving.

## Implementation Plan

### Phase 1: Server foundation — AI endpoint + batch claim

**Goal:** Land the compose-time AI endpoint and the publish-time batch
claim RPC. No UX visible to users; no schema change to `PlanningNeed`.

**Tasks:**
- [ ] New RPC `GenComposeSuggestions(GenComposeSuggestionsRequest) →
      GenComposeSuggestionsResponse` in
      [`request_service.proto`](../../proto/ripls/api/request_service.proto).
      Request: `{title, sub, community_id}`. Response:
      `{suggestions: [{label}], generation_id}`.
- [ ] Server: handler in `server/services/request/gen.go` (mirror
      `GenRequest` shape). Reuse
      [`buildRequestSuggestionsPrompt`](../../server/ai/prompts.go) as-is
      — no prompt or schema change; just extract `breakdown_pieces` as
      the `suggestions` payload. Gemini-2.5-Flash, ~300 ms target.
      Fallback chain identical to existing AI handlers
      (Anthropic / OpenAI). Log `prompt_version`, `provider`, `model`
      per the
      [`StreamGenUnifiedCreate` field set](../server/observability.md#unified-create-stream-streamgenunifiedcreate).
- [ ] New RPC `BatchClaimRequestNeeds(BatchClaimRequestNeedsRequest) →
      BatchClaimRequestNeedsResponse`: `{request_id, claims:
      [{need_id, optional note, optional gear_id}]}` →
      `{results: [{need_id, contribution_id, error?}]}`. Per-claim
      success/failure granularity so the client can surface partial
      success. Server-side: single transaction over the claim loop;
      reuses [`ClaimRequestNeed`'s](../../server/services/request/needs.go#L334)
      slot-bounds and ownership-allowed checks (requester self-claim
      is already permitted; no new guard). **File placement:** add to
      [`server/services/request/needs.go`](../../server/services/request/needs.go)
      if it stays under 1000 lines per
      [server/architecture.md§"Short, Focused Files"](../server/architecture.md#3-short-focused-files);
      otherwise create `server/services/request/batch_claims.go` and
      move `ClaimRequestNeed` there with it.
- [ ] Storage unit tests (use `storage.SetupTestStorage(t)`):
      `BatchClaimRequestNeeds` partial-success path, requester
      self-claim succeeds, slot-bounds rejection per-claim, and
      `storage.AssertMaxQueries(t, ctx, 1, ...)` on
      `BatchClaimRequestNeeds` to prove the batch is one INSERT.
- [ ] Integration-tagged test for `GenComposeSuggestions` mirroring
      [`server/ai/provider_integration_test.go`](../../server/ai/provider_integration_test.go)
      (`//go:build integration && ai_providers`) — asserts the response
      contains at least one suggestion for a canonical input ("yard
      cleanup after the storm").
- [ ] Run `npm run build`, then `go build -tags=integration ./...`,
      then `npm run lint:go` for `check-goroutines` +
      `check-codeinternal`.

**Validates:** the compose AI endpoint produces useful chip suggestions
at low latency, and the pre-claim leg is one round-trip with per-piece
results.

### Phase 2: Compose ViewModel + state plumbing

**Goal:** All client-side state plumbing for the compose sheet, without
the UI layer. ViewModel-level tests prove the logic before any widgets
are wired.

**Tasks:**
- [ ] **Repository-side first.** Add
      `getComposeSuggestions({required String title, String? sub,
      String? communityId})` and `refreshComposeSuggestions(...)` to
      [`RequestRepository`](../../app/lib/data/repositories/request_repository.dart).
      Cache key `compose-suggestions:<sha256-prefix-of-(title|sub|
      communityId)>` under namespace `request`. Refresh method calls
      `_cache.invalidate(key)` first per
      [caching.md§"Cache Invalidation"](../client/caching.md#3-cache-invalidation).
      Service-layer wrapper hits the new `GenComposeSuggestions` RPC.
- [ ] New
      `app/lib/presentation/viewmodels/request_compose_view_model.dart`
      exposing `requestComposeProvider` (`AsyncNotifier.autoDispose`).
      State: `{title, sub, latestSuggestionResponse:
      ComposeSuggestionsResponse?, pieces: List<ComposePiece>,
      loading, error: UserError?}`. Each `ComposePiece` is
      `{id, label, preclaimed: bool, fromSuggestionId: String?}`.
- [ ] Methods: `setTitle`, `setSub`, `requestSuggestions()` (debounced
      ~600 ms on title-blur — calls
      `requestRepository.getComposeSuggestions(...)`),
      `forceRefreshSuggestions()` (calls `refreshComposeSuggestions`),
      `toggleSuggestion(id)`, `addPasteList(text)` (split on
      commas / semicolons / newlines), `addInlinePiece(text)`,
      `renamePiece(id, label)`, `togglePreclaim(id)`,
      `removePiece(id)`, `publish() → Future<PublishResult>`. Every
      method that sets `state` after an `await` first checks
      `if (!ref.mounted) return;` (see Cross-cutting compliance
      §Architecture).
- [ ] `publish()` runs the three-step:
      `submitRequest` → `batchAddRequestNeeds` →
      `batchClaimRequestNeeds` (only for pre-claimed pieces). On
      partial success of step 2 or 3, return a `PublishResult` with
      `{requestId, addedNeedIds, failedAdds, claimedNeedIds,
      failedClaims}` so the widget can render a "Posted, but…" toast.
- [ ] ViewModel tests (`ProviderContainer` + Mockito mocks for
      `RequestRepository` and the request service):
  - Suggestion chip add/remove round-trip; chip state stays in sync
    with the pieces list.
  - Paste-list split (commas / semicolons / newlines / mixed
    whitespace).
  - Inline rename commit and cancel.
  - Pre-claim toggle round-trip; pre-claim count reflected in derived
    state.
  - Publish happy path (3-step succeeds; state cleared).
  - Publish partial success: SubmitRequest succeeds, one need-add
    fails → `PublishResult.failedAdds.length == 1`, `requestId` set,
    ViewModel state preserved for retry.
  - Publish partial success: all needs added, one pre-claim fails →
    `PublishResult.failedClaims.length == 1`.
  - Disposal during in-flight `requestSuggestions` →
    `addTearDown(container.dispose)` + `expect(future, completes)`.
  - **Debounce test:** `runDebounced` from
    `app/test/helpers/fake_async_helpers.dart` drives the 600 ms
    debounce — two rapid `setTitle` calls within 600 ms result in
    exactly one `getComposeSuggestions(...)` after
    `async.elapse(Duration(milliseconds: 600))`.
  - **Cache hit:** warm the repo, second call with the same prompt →
    `verifyNever` on the service mock; only the repo's cache.get
    path runs.
- [ ] No new ARB strings yet (no UI in this phase).

**Validates:** every piece of compose logic is unit-testable in
isolation; partial-success paths have explicit shapes the UI can render
without ambiguity.

### Phase 3: Compose sheet UI on `RequestPreviewModal`

**Goal:** Ship the visible feature behind the existing
`unifiedCreateEnabledProvider` flag. Users on the unified create path
see the new compose sheet; legacy users continue with the old
`RequestCreationModal` → `RequestPreviewModal` path.

**Tasks:**
- [ ] Extend
      [`request_preview_modal.dart`](../../app/lib/presentation/screens/request/request_preview_modal.dart)
      with a new stage `compose` inserted between the existing preview
      stage and the submit action. Stage transition is internal to the
      modal — no new top-level navigation.
- [ ] New widget
      `app/lib/presentation/widgets/request/compose/request_compose_sheet.dart`
      composed of:
      `compose_title_inputs.dart` (title + sub fields),
      `compose_suggestion_chips.dart` (AI chip grid),
      `compose_paste_list_disclosure.dart`,
      `compose_pieces_list.dart` + `compose_piece_row.dart` (label +
      inline rename + pre-claim toggle + remove),
      `compose_inline_add.dart`,
      `compose_publish_footer.dart`. Each file < 200 lines so the
      sheet stays under the 1000-line cap.
- [ ] Suggestion chips fire `requestSuggestions()` on first title
      blur; render skeleton chips while loading; render error state
      with retry button on AI failure. The error banner is wrapped in
      `LiveRegion` so the screen reader re-announces if the error
      message changes. Banner copy resolves via
      `RpcErrorHandler.localize(state.error!, context.l10n)` —
      ViewModel never resolves strings.
- [ ] Inline rename via `editable_text`-style tap-to-edit;
      `Tappable` with
      `semanticsLabel: context.l10n.a11yComposePieceRename(label)`.
- [ ] Pre-claim toggle uses `Toggle` (announces selected state);
      flips piece to a "You" pill per prototype.
- [ ] Publish CTA shows `pieces.length` + (`claimedCount > 0 ? "· $n
      you've got" : "")` strings. CTA disabled when `pieces.isEmpty`.
- [ ] Discard confirmation modal when title/sub/pieces are non-empty;
      empty drafts close silently.
- [ ] RPC errors via `RpcErrorHandler.classify` → `UserError` on
      state; widget calls `RpcErrorHandler.localize` in `build`.
- [ ] Partial-success toast on publish: "Posted. Couldn't add 1 piece
      — retry?" with a retry action that re-runs the failed sub-leg.
      The moment the toast renders, call
      `SemanticAnnouncer.announce(context, context.l10n.composePartial<Add|Claim>Error(...))`
      so the screen reader speaks the partial-success state once —
      a `LiveRegion` is wrong here because the toast auto-dismisses.
- [ ] New ARB strings in both
      [`app_en.arb`](../../app/lib/l10n/app_en.arb) and
      [`app_es.arb`](../../app/lib/l10n/app_es.arb):
      `composeTitleHint`, `composeSubHint`,
      `composeSuggestionsHeader`, `composePasteTrigger`,
      `composePastePlaceholder`, `composePasteAdd`,
      `composePiecesHeader`, `composeInlineAddHint`,
      `composeIveGotThis`, `composeIveGotThisShort`,
      `composePublishCta`, `composePieceCount`, `composeYouCount`,
      `composeDiscardTitle`, `composeDiscardBody`,
      `composeDiscardConfirm`, `composeDiscardKeep`,
      `composePartialAddError`, `composePartialClaimError`, plus
      `a11y*` siblings for every interactive surface
      (`a11yComposeChip`, `a11yComposePieceRow`,
      `a11yComposePreclaimToggle`, `a11yComposeRemovePiece`,
      `a11yComposePieceRename`).
- [ ] Run `npm run lint:dart:i18n`, `npm run lint:dart:l10n-parity`,
      `npm run lint:dart:a11y` (no allowlist additions).
- [ ] Widget tests for: empty / loading / error suggestion states;
      paste preview count; disable-when-empty CTA; discard confirm.

**Validates:** the prototype's UX lives in the app end-to-end, the
unified create path produces decomposed Requests, and the partial-
success story works in front of a user.

### Phase 4: Polish + telemetry

**Goal:** Catch the long tail before broader rollout. Treat as a 1-week
cleanup once Phase 3 is dogfooded.

**Tasks:**
- [ ] Re-run `storage.AssertMaxQueries` guards from Phase 1 with
      production-sized batches; tighten bounds if possible.
- [ ] Analytics events via the existing analytics layer:
      `compose_chip_added` (which suggestion),
      `compose_paste_used` (count of pieces added in one paste),
      `compose_piece_renamed`,
      `compose_preclaim_toggled`,
      `compose_publish` (total pieces, pre-claim count, AI vs.
      manual ratio),
      `compose_publish_partial_failure` (which sub-leg failed,
      retry attempted bool). Confirm event names with the analytics
      owner before the PR lands.
- [ ] Accessibility audit pass: every interactive surface announces
      via the right primitive; `npm run lint:dart:a11y` clean with no
      allowlist additions.
- [ ] Update [`docs/client/needs.md`](../client/needs.md) with a
      compose-side section: how an asker breaks down a Request into
      pieces, how pre-claim contributions are written. No new
      "Intentional differences" entry — parity holds.

**Validates:** no N+1 regressions, accessibility doesn't degrade, the
documentation stays current.

## Related Issues

- **#2240** — claim-side gear linking. Adjacent; this compose flow
  produces the per-piece Needs that #2240's picker (and gear-pill
  rendering) shows to claimers. Pre-claimed pieces by the asker
  appear with the asker's contribution already on the row.
- **#1932** — classifier-latency-slim-response. The
  `GenComposeSuggestions` RPC follows the latency-first pattern
  established there (Gemini-Flash, slim schema-constrained response,
  ~300 ms target).
- **#2228** — earlier Request-only framing of gear linking,
  superseded by #2240. Mentioned only for context — this issue does
  not depend on it.
- **needs.md Deferred #5** (saved kits / Picker rail) — adjacent;
  a "kit" is a saved set of gear linked to a saved set of Needs.
  Compose decomposes a Request into per-piece Needs, which is the
  primitive a "saved kit" feature would build on.
- **(Future) Per-Need kind classification** — explicitly scoped OUT
  of this issue ("not worth the AI cost" for v1). If kind-aware
  claim UI becomes valuable later, a follow-up adds `NeedKind` to
  `PlanningNeed`, extends the AI prompt schema to emit a `kind` per
  chip, and branches the claim picker on `kind`. Nothing in this
  plan precludes that follow-up.
- **(Future) Experience-scope compose** — explicitly out of scope.
  Storage paths in this plan are unchanged, so an Experience-scope
  follow-up does not require re-touching this work.
