---
issue: 1229
status: planning
---

# Issue #1229: Extend server-side streaming to Gen* image + webpage modes

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1229
**Filed:** 2026-04-22 | **Last activity:** 2026-04-25
**Labels:** none | **Assignees:** briangbrown
**State:** OPEN
**Context docs:** [docs/issues/1157-genexperience-latency.md](1157-genexperience-latency.md)
**Triage:** Type=Feature, Priority=P4 (icebox until text-mode streaming proves stable — per maintainer 2026-04-25)

> **Audit (2026-04-26):** Reviewed against `docs/server/architecture.md`,
> `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`.
> See `### Audit findings (2026-04-26)` below for the diff.
>
> **Scope extension (2026-04-26, post-audit):** After verifying that
> Phase 4b/4c of #1157 shipped for text mode, the plan was extended to
> include the 4b/4c equivalents for image and webpage modes (Phases 3
> and 4 below). Adds two proto fields, server handler work to replace
> `streamGen*FromUnary` short-circuits, and small Flutter viewmodel
> updates. Client-side architecture invariants (Repository ⇄ Service
> ⇄ ViewModel layering per `client/architecture.md`, Stash caching per
> `client/caching.md`) still hold — the new client work goes through
> the existing `*Repository.streamGen*` methods + viewmodel state
> updates; no new layers, no new caches.

## Problem Statement

Phase 4a, 4b, and 4c of #1157 all shipped for **text mode**:

- **4a (PR #1218, merged 2026-04-22)** — server-internal streaming + early
  fan-out, unary client response.
- **4b** — streaming RPCs (`StreamGenExperience`, `StreamGenRequest`,
  `StreamGenGear`) and server-side handlers in
  `server/services/*/gen_*streaming.go`.
- **4c** — Flutter consumers in
  `app/lib/data/repositories/*_repository.dart` +
  `app/lib/presentation/viewmodels/*_creation_view_model.dart`, with
  skeleton/incremental-fill UI.

The image and webpage variants of those same flows are **not** fully on
the streaming path. Verified state at HEAD:

| Service | Streaming RPC proto today | Client today | Server today |
|---|---|---|---|
| Experience | accepts `text \| media_id \| website_url` (oneof — proto-complete) | calls `streamGenExperience` for all three modes — client-complete | text streams; **image and webpage punt to `streamGenExperienceFromUnary`** which calls the unary handler and emits only the terminal `final` event |
| Gear | accepts `prompt + media_id` (no `website_url`) | calls `streamGenGear` for text+image; webpage flow uses unary `DetectGearFromURL` | text streams; **image punts to `streamGenGearFromUnary`** (terminal-only); no webpage streaming surface at all |
| Request | accepts `prompt` only (no `media_id`) | text uses streaming; **image is explicitly guarded out** (`request_creation_view_model.dart:197` — "Image mode is not currently supported by the streaming RPC") and falls back to unary `genRequestFromMedia` | text only — no image or webpage streaming surface |

Net: image/webpage flows pay the cost of a streaming RPC channel
(extra connection, extra error surface) but get **none** of the
incremental-event UX or fan-out latency win. The fix is two-layered:

1. **Server-internal streaming for the providers** (the original #1229
   scope, Phases 1–2 below) — needed regardless of whether the client
   sees streaming events, because it's what makes the post-AI fan-out
   parallel.
2. **Wire image/webpage through the existing streaming RPCs as
   first-class branches** that emit `title` / `description` / `geocoded`
   / `media_ready` / `final` events the same way text mode does
   (Phases 3–4 below). This is the 4b/4c-for-image+webpage extension.

Five provider methods are in scope, all sharing the post-AI Mapbox+stock-media
fan-out shape:

- `GenerateExperienceFromImage` — Mapbox + Pexels on `location_query` / `search_keywords`
- `GenerateExperienceFromWebpage` — Mapbox + Pexels (same triggers)
- `GenerateGearFromWebpage` — stock-image + Mapbox (webpage-extracted fields)
- `GenerateRequestFromImage` — Mapbox + Pexels
- `DetectGearInImage` — stock-image keyed on `title` (matches text-mode Gear trigger)

Out of scope per the issue (no fan-out to parallelize, so no streaming win):
`ParseInformalTime`, `GenerateCommunityContent`, `GenerateConversationSummary`,
`GenerateExperienceCompletionSummary`, `GenerateStoryContent`,
`GenerateNudgeContent`, `GenerateExperienceSuggestions`,
`GenerateRequestSuggestions`, `InferSocialAttributes`.

## Context from Comments

- **2026-04-25 (maintainer, /ripls-triage)** — Classified as Type=Feature,
  Priority=P4. Rationale: "nice-to-have streaming expansion. No product
  urgency currently; icebox until text-mode streaming proves stable." This is
  the only comment on the issue. The plan below treats P4 as a deliberate
  pacing signal: don't start until Phase 4a's prod metrics confirm the
  text-mode streaming is delivering the expected ~1 s win without
  regressions.

The issue body itself already does most of the planning work — it lists
in-scope methods, provider impls, service rewrites, dependencies, and split
options with a recommendation. This document validates those claims against
the current codebase, surfaces things the issue body got slightly wrong, and
sequences the work.

## Findings from codebase research

Verifying the issue body's claims against the repo at HEAD:

- **PR #1218 is merged** (2026-04-22). Streaming primitives exist:
  `server/ai/provider_anthropic_streaming.go`, `provider_openai_streaming.go`,
  `provider_gemini_streaming.go`, `provider_streaming_unary.go`. The
  `runStreamingFanout` pattern is in place at `server/services/experience/gen_ai_fanout.go:98`
  and is wired into `gen_ai.go:122` for text-mode `GenExperience`. Same shape
  expected in `server/services/request/gen.go` and `server/services/gear/gen_ai.go`.
- **Provider methods exist as named** in `server/ai/provider_anthropic.go`
  (lines 149, 254, 349, 493, 594) and have matching `FallbackProvider`
  wrappers at `server/ai/provider_fallback.go:153–229`.
- **Eval coverage is mostly there but has a gap** the issue body misses.
  `server/ai/eval/testdata/` contains `experience_image_goldens.json`,
  `experience_webpage_goldens.json`, `gear_image_goldens.json` (covers
  `DetectGearInImage`), `gear_webpage_goldens.json` — four of the five.
  **There is no `request_image_goldens.json`.** `request_goldens.json` covers
  text-mode only. So `GenerateRequestFromImage` would ship without an eval
  baseline unless we author one as part of this work, or accept that the
  unit-tested fake-provider scaffolding is sufficient pre-merge regression
  cover for that one method.
- **Unary fallback wrapper pattern (PR #1218 line-reduction claim).** PR #1218
  added `provider_streaming_unary.go` so each unary method becomes a thin
  drain-wrapper over its streaming counterpart. The same pattern applies to
  the five image/webpage methods; net diff is removal of duplicated
  schema/parsing code in three providers per method.

- **4b/4c verification (added after initial plan).** Searched
  `proto/ripls/api/`, `server/services/*/gen_*streaming.go`, and
  `app/lib/{data,presentation}/`. All three streaming RPCs and Flutter
  consumers exist for text mode. The image/webpage paths through 4b/4c
  are **partial**:
  - `StreamGenExperienceRequest` is mode-complete (oneof has
    `text | media_id | website_url`); the client already calls it for all
    three modes; only the server handler short-circuits image/webpage to
    `streamGenExperienceFromUnary` (`server/services/experience/gen_ai_streaming.go:76`,
    comment line 65: "Only text mode exercises the streaming fan-out in
    this phase; media/webpage go through the unary handler and emit only
    the terminal event").
  - `StreamGenGearRequest` has `prompt + media_id` but no `website_url`.
    The client calls `streamGenGear` for text+image; image short-circuits
    to `streamGenGearFromUnary` (`server/services/gear/gen_ai_streaming.go:78`).
    Webpage gear has its own unary RPC `DetectGearFromURL` and doesn't
    flow through `StreamGenGear` at all today.
  - `StreamGenRequestRequest` has `prompt` only — no `media_id`,
    no `website_url`. The Flutter viewmodel guards image streaming out
    explicitly (`request_creation_view_model.dart:197`) and falls back
    to unary.

  This means the 4b/4c extension to image/webpage requires **two proto
  field additions** (`media_id` on `StreamGenRequestRequest`,
  `website_url` on `StreamGenGearRequest`) plus server handler work
  plus small client viewmodel updates. Experience needs no proto or
  client work — only server.

Assumptions in the issue that hold up:

- All five methods share the post-AI fan-out shape — confirmed by reading
  each provider impl and each service call site.
- `StreamingJSONFields` tokenizer is provider-agnostic and reusable as-is —
  it operates on `[]byte → []FieldEvent` per the Phase 4a design.
- Image and webpage methods take different upstream params (image parts vs.
  webpage body string) but produce the same downstream JSON-shape responses
  as their text-mode siblings, so the streaming machinery downstream of
  "AI returns JSON" is identical.

## Potential Solutions

### Option A: Per-mode-group PRs (image-first, webpage-second)

- **Approach:** Two PRs sequenced. PR-1 ships streaming for the three image
  methods (`GenerateExperienceFromImage`, `GenerateRequestFromImage`,
  `DetectGearInImage`) across all three providers + service rewrites + unary
  drain-wrappers. PR-2 ships the two webpage methods
  (`GenerateExperienceFromWebpage`, `GenerateGearFromWebpage`) the same way.
  Mirrors the Phase 4a → 4b/4c sequencing #1157 already used.
- **Pros:** Independently bisectable and revertable. Eval harness runs
  separately per PR — easier to attribute any regression to its mode group.
  Reviewer-friendly diff size (each PR is ~9 provider methods + 2-3 service
  rewrites + tests). Image-first delivers the biggest user-perceived win
  first, which is the right ordering if P4 is upgraded mid-flight.
- **Cons:** Two review cycles, two eval runs, two deploys. Slightly more
  total wall-clock to "done" than (B).
- **Effort:** Medium each. ~9 provider methods + 3 service rewrites + tests
  per PR. Likely ~600–800 net lines per PR (add streaming, remove duplicated
  unary parsing). Each PR is similar in size to PR #1218.

### Option B: Single PR for all five methods × three providers

- **Approach:** One branch. 15 provider impls + 5 service rewrites + 5 unary
  drain-wrappers + eval-harness runs for both mode groups, all merged
  together.
- **Pros:** Single review cycle, single eval comparison, one deploy.
- **Cons:** Diff size is double PR #1218 — likely 1200–1600 net lines
  changed. Bisection is coarser (a regression bisects to the whole streaming
  rollout, not a specific mode group). Eval failures in one mode block the
  other. PR-review fatigue is real on diffs this size.
- **Effort:** Medium-large. Same total effort as (A), worse review ergonomics.

### Option C: Defer until Phase 4a prod metrics are in

- **Approach:** Don't start until #1157 / PR #1218 has run in prod long
  enough to confirm the expected ~1 s wall-clock win materialized for text
  mode and didn't introduce regressions. If the prod data shows
  image/webpage wall-clock is dominated by image upload or HTTP fetch (not
  the post-AI fan-out), the streaming work returns less than projected and
  P4 stays icebox.
- **Pros:** Avoids speculative work. Aligns with the P4 triage rationale
  ("icebox until text-mode streaming proves stable"). The Phase 4a streaming
  log fields (`streaming_first_delta_ms`, `streaming_*_closed_ms`) make this
  measurement cheap.
- **Cons:** No code progress; the issue stays open. Doesn't itself produce a
  decision — needs someone to pull the prod numbers and apply judgment.
- **Effort:** Trivial (one query / dashboard read), but the *output* is a
  go/no-go for (A) or (B), not a finished feature.

## Recommendation

**(C) first, then (A) extended.** The triage explicitly icebox'd this until
text-mode streaming proves stable, and Phase 4a only landed four days
before triage — that prod-soak hasn't happened yet at the time of this
plan. The lowest-cost next step is to pull the post-PR-#1218 wall-clock
numbers from prod logs once we have ~2 weeks of data, confirm the win
matched the ~1 s projection, and then start (A).

Picking (A) over (B) for the implementation phase because the diff sizes
favor reviewer ergonomics, and image-first ordering means the biggest
user-visible win (image-mode `GenExperience` already pays the upload tax
that makes the post-AI wait feel longer) ships first if priorities shift.

The order of PRs in (A) — image, then webpage — also matches available
eval coverage. Image mode has goldens for two of three methods today
(`GenerateExperienceFromImage`, `DetectGearInImage`); we author
`request_image_goldens.json` as part of PR-1. Webpage mode has full eval
coverage already, so PR-2 is a clean apply-the-pattern exercise.

**Extended scope (4b/4c parity for image/webpage).** Phases 1–2 of (A)
deliver the 4a equivalent (server-internal streaming) for image/webpage.
Phases 3–4 then extend 4b/4c to those modes so users see the win, not
just prod logs. Sequencing matters: Phase 1 image-mode provider
streaming has **no user-visible effect on its own** because the server
handler still calls `streamGenExperienceFromUnary` and emits a
terminal-only event. The user-visible win for image needs Phase 1 + the
image half of Phase 3 together. Same for webpage (Phase 2 + webpage half
of Phase 3). The phases are still useful to land separately for review
size, but the eval/prod-impact gate evaluates the *vertical pair*, not
each phase alone.

## Open Questions

- **Is the Phase 4a prod wall-clock win actually materializing?** —
  Recommendation: query prod logs for the new `streaming_first_delta_ms` /
  `streaming_*_closed_ms` fields plus `fanout_duration_ms` over a 2-week
  window post-PR-#1218 (i.e., starting ~2026-05-06). Compare against the
  pre-streaming Phase 3a baseline in `docs/evals/1157-genexperience-latency.md`.
  If the per-handler streaming-vs-unary wall-clock delta is <500 ms in prod,
  re-evaluate (C) before committing to (A) — the assumption that image/webpage
  modes will see a similar ~1 s win is downstream of text-mode actually
  delivering it.

- **Do we ship `GenerateRequestFromImage` without a golden eval?** —
  Recommendation: author a minimal `request_image_goldens.json` (3–5 cases
  reusing existing `server/test_data/` images) as the first commit on the
  image-modes branch. Each case asserts on a small subset of fields
  (`title`, `description`, `category`) at wide tolerances. Cost is ~1 hour
  of authoring + ~$0.50 of API calls to seed the goldens; benefit is a
  regression canary on par with the other four methods. Without it, the
  image-modes PR has weaker before/after evidence than its peers.

- **Does Gemini's coarser structured-output streaming cost us the win on
  image modes specifically?** — Recommendation: include a one-off Gemini
  benchmark in PR-1 alongside the Anthropic/OpenAI numbers, even though we
  expect Gemini to emit field events in a single end-of-call burst (per
  Phase 4a Gemini notes at `docs/issues/1157-genexperience-latency.md:225`).
  If Gemini gives <100 ms early-fire on image modes, the documented limitation
  is consistent with text mode and we ship as-is. If it's worse than text
  mode (e.g., image generation also serializes vision processing in a way
  that delays the first delta further), surface it in the eval doc and
  consider whether Gemini's image-mode role becomes "fallback only" rather
  than primary in the provider chain.

- **Do we extend `unary-wraps-streaming` to the fallback provider too, or
  keep `FallbackProvider` calling unary directly?** — Recommendation: keep
  `FallbackProvider` calling each provider's unary method (its current
  shape). The fallback path runs only when the primary already failed —
  there is no streaming win to chase on a fallback because the user-visible
  latency budget is already blown. The drain-wrapper pattern means the
  unary entrypoint still works; nothing in `provider_fallback.go` needs to
  change.

- **Do we update `docs/issues/1157-genexperience-latency.md` to mark image
  and webpage as in-progress / done, or treat #1229 as the canonical doc
  going forward?** — Recommendation: treat #1229 as canonical for this
  scope. Add a one-line "Follow-up: image+webpage streaming tracked in
  #1229" pointer at the top of the #1157 doc when PR-1 lands, but don't
  retroactively expand #1157's phase numbering. The Phase 4a scope was
  deliberately drawn to text mode; this work is a sequel, not an unfinished
  phase.

- **Gear webpage flow: extend `StreamGenGear` or stream `DetectGearFromURL`
  separately?** — Recommendation: add `website_url` to
  `StreamGenGearRequest` (oneof or alongside `prompt`/`media_id`) and
  route gear webpage through `StreamGenGear` like Experience does. The
  alternative — adding a new `StreamDetectGearFromURL` RPC — duplicates
  surface area and the unary `DetectGearFromURL` handler is already a
  thin wrapper over `GenerateGearFromWebpage`. Folding into
  `StreamGenGear` keeps gear-creation funnels uniform from the client's
  perspective. Wire-compatible add per `CLAUDE.md` proto conventions.

- **Request streaming proto: oneof or coexisting fields?** —
  Recommendation: introduce a `prompt` oneof (`text` / `media_id`) on
  `StreamGenRequestRequest`, mirroring `StreamGenExperienceRequest`'s
  shape. The current flat `string prompt = 1` is the legacy singleton;
  promoting to oneof is wire-compatible because the existing field
  number stays put under the oneof. Same case applies eventually for
  `StreamGenGearRequest` (today: flat `prompt` + `media_id`, no
  webpage). Worth doing the oneof refactor in the same PR to keep
  request shapes consistent across the three streaming RPCs.

- **Should `DetectGearFromURL` (unary) be retired in favor of
  `StreamGenGear` with `website_url`?** — Recommendation: don't retire
  in this issue's scope. `DetectGearFromURL` is a separate user flow
  (paste a URL on an existing gear edit screen, get autofilled
  metadata) versus the create-gear webpage flow that this would streamline.
  File a follow-up issue to evaluate consolidating once the streaming
  webpage path is proven. Avoid scope creep here.

## Implementation Plan

### Phase 0: Confirm the prod win on text-mode streaming — **DONE 2026-04-26**

**Goal:** Decide whether to proceed with image+webpage streaming based on
prod evidence that text-mode streaming actually delivered.

**Verdict:** **Gate passes.** Investigation appended to
`docs/evals/1157-genexperience-latency.md` under the 2026-04-26 entry
"#1229 Phase 0: post-deploy prod evidence pull for text-mode streaming
gate". Three independent signals confirm the win:

1. 2026-04-22 Anthropic Phase 4a benchmark: **830 ms wall-clock win**
   server-side (1873 ms streaming vs 2707 ms parallel-fan-out
   baseline) — clears the ≥500 ms gate.
2. 2026-04-23 on-device Phase 4c benchmark: **−78% time-to-first-content**
   client-side; all three flows (Experience 82 ms / Request 50 ms /
   Gear 741 ms) clear the <800 ms `client_first_event_ms` gate.
3. Prod-deploy AI-call durations from `anthropic API call completed`
   logs (n=2 each in dev): `GenerateExperienceFromTextStreaming`
   median 1807 ms, `GenerateRequestContentStreaming` median 1777 ms —
   inside the benchmark range; no regression. Plus one rich-fields
   `post-AI fan-out completed` entry showing Mapbox firing at offset
   900 ms and Pexels at 1077 ms (early-fire architecture working
   in prod).

**Why we didn't wait the full 2 weeks:** prod streaming traffic for the
targeted RPCs is essentially zero (1 streaming gear text call, 0
streaming Experience or Request calls in 3 days post-deploy). The
bottleneck is organic creation-flow traffic, not the streaming
infrastructure. Continuing to wait would not produce statistically
meaningful prod evidence by the original 2026-05-06 target.

**Side finding — log field gap:** five of the six `streaming_*_ms`
fields the plan named are **not actually emitted** by the deployed
code (only `fanout_duration_ms` is). What ships instead:
`stream_duration_ms`, `mapbox_started_offset_ms`,
`stock_media_started_offset_ms`, plus boolean `mapbox_hit` /
`stock_media_hit`. Functionally equivalent for the Phase 0 decision;
flagged here so any future per-key telemetry need (e.g., diagnosing
mode-specific regressions in Phase 3) knows it has to add the
plumbing.

**Tasks:**
- [x] Pull post-deploy prod + dev logs (3-day window, 2026-04-23 to
      2026-04-26).
- [x] Compare AI-call durations against the 2026-04-22 Phase 4a
      benchmark.
- [x] Append a 2026-04-26 entry to
      `docs/evals/1157-genexperience-latency.md`.
- [x] Decision: proceed to Phase 1.
- [ ] (Carried into Phase 3 follow-ups) If image/webpage modes need
      per-key timing for regression diagnosis, plumb
      `streaming_*_closed_ms` fields through `LogStreamingCompleted`.
      Not blocking Phase 1.

### Phase 1: Image-mode streaming (PR-1)

**Goal:** Ship streaming + early fan-out for the three image-mode methods
(`GenerateExperienceFromImage`, `GenerateRequestFromImage`, `DetectGearInImage`)
across all three providers, with end-to-end eval coverage.

**Scope correction (2026-04-26, after vertical-slice verification on
`GenerateRequestFromImage`):** "Early fan-out" is **not** part of Phase 1
for image mode. The image-mode service handlers don't use fan-out today
(verified at HEAD against `server/services/request/gen.go:327`,
`server/services/experience/gen_ai.go:404`, and the gear DetectGearInImage
call sites). Phase 1 is therefore **provider-layer streaming only**: the
streaming methods exist on the `ai.Provider` interface, are implemented on
all three leaf providers, and are drained by the unary methods for
backwards compatibility. No service-layer code changes. The user-visible
win comes in Phase 3 when image mode is wired through the `StreamGen*`
RPCs to emit incremental events to the client. See the per-task
strikethrough below.

**Branch model (per user direction):** each image method ships in its own
commit on a single feature branch (`1229-image-streaming-*`):
- Commit 1: `GenerateRequestFromImage` (landed `f371d9aa`)
- Commit 2: `GenerateExperienceFromImage`
- Commit 3: `DetectGearInImage`
After all three land, the branch becomes a single PR.

**Tasks:**
- [ ] Author `server/ai/eval/testdata/request_image_goldens.json` (3–5 cases,
      reuse images from `server/test_data/`). Run it once against the
      current unary path to seed the baseline.
- [ ] Add streaming variants to the `ai.Provider` interface:
      `GenerateExperienceFromImageStreaming`, `GenerateRequestFromImageStreaming`,
      `DetectGearInImageStreaming`. Uniform
      `(<-chan FieldEvent, <-chan FinalResult, error)` shape, matching
      Phase 4a.
- [ ] Implement streaming for each method in `provider_anthropic.go`,
      `provider_openai.go`, `provider_gemini.go` — reuse
      `streamToolCall` / `streamStructuredOutput` helpers. New code lives
      in `provider_<name>_streaming.go`. Image inputs build the same
      `image_part`/`media_url` shape today's unary methods build; only the
      response handler swaps to streaming.
- [ ] Refactor each unary method to drain its streaming counterpart via
      `provider_streaming_unary.go`'s helpers. Net line reduction comes
      from removing the duplicated structured-output parsing in each
      provider.
- [ ] ~~Service-layer rewrite: switch from `fanout.Run` to
      `fanout.StreamingRun`.~~ **Verified 2026-04-26 against HEAD: this
      task is moot for image-mode.** Image-mode service handlers
      (`generateFromMedia` in `server/services/request/gen.go:327`,
      `generateFromMedia` in `server/services/experience/gen_ai.go:404`,
      and the `DetectGearInImage` call sites) **do not use any
      fan-out today** — they call the unary AI method directly with no
      Mapbox or Pexels parallelism. Two reasons it's intentional:
      (a) the user-uploaded image *is* the preview media, so Pexels is
      unused; (b) image mode rarely yields a `location_query`, so the
      code falls back to the user's primary location instead of
      geocoding (`gen.go:252-259`). After the unary→streaming drain
      refactor in the provider layer, calling sites are unchanged
      because the unary method now drains streaming under the hood.
      **Phase 1 image-mode work therefore stops at the provider layer.**
      The user-visible image-mode wins materialize in Phase 3 (wiring
      image mode through the `StreamGenExperience`/`StreamGenRequest`
      RPCs so clients see incremental events). A separate question —
      whether to *add* Mapbox geocoding when `location_query` is
      non-empty in image mode — is net-new behavior, not in scope here.
      Considered for a follow-up issue if image-mode location-extract
      quality improves with newer vision models.
- [ ] Structured logging additions: same field set as Phase 4a
      (`streaming_first_delta_ms` etc.) on the `"post-AI fan-out completed"`
      span for each handler. Field naming follows
      `docs/server/observability.md` (the canonical logging doc — note
      that CLAUDE.md and #1157's plan refer to a `docs/server/logging.md`
      that was renamed/merged into observability.md; keep that in mind
      when chasing references).
- [ ] Unit tests: per-method scripted-event tests using the existing
      `ai.MockProvider` (`server/ai/provider_mock.go`) — extend it with a
      streaming-event helper if one doesn't exist yet rather than
      authoring per-test fakes — plus controlled-delay Mapbox/Pexels stubs.
      Verify fan-outs fire on the right key-close events and
      partial-failure handling matches text mode. Per
      `docs/server/architecture.md` §Mock Strategy: one shared mock per
      shared library, not service-specific fakes.
- [ ] Eval-harness run against Anthropic + OpenAI + Gemini for all three
      image goldens. Append a Phase-1 entry to
      `docs/evals/1157-genexperience-latency.md` with per-handler
      before/after wall-clock.
- [ ] **Integration test** in `server/main_integration_test.go` (or the
      existing per-service integration suite) covering at least one
      image-mode flow end-to-end with a scripted streaming
      `ai.MockProvider` and a real testcontainers PostgreSQL —
      `docs/server/architecture.md` §Testing Strategy classifies this as
      a "major feature" requiring integration coverage. One flow per
      mode-group is sufficient (e.g., `GenerateExperienceFromImage`)
      since the streaming machinery is shared; no need for full
      Cartesian coverage.
- [ ] **File-size check after the diff lands**: existing
      `provider_anthropic_streaming.go` (352 lines), `_openai_streaming.go`
      (364), `_gemini_streaming.go` (282) will roughly double once five
      new methods land. Per `docs/server/architecture.md` §File Size
      Guidelines (target <500, max <1000), if any of the three crosses
      ~800 lines, split per-method (e.g., `provider_anthropic_image_streaming.go`,
      `provider_anthropic_webpage_streaming.go`) before merge rather
      than after. Cleanest split is by mode-group, matching the PR
      structure.
- [ ] `npm run build` passes (lint + go build, including
      `go build -tags=integration ./...` since `ai.Provider` signatures
      change).

**Validates:** Wall-clock parity-or-better with the Phase 4a text-mode
gains for image modes; eval pass rates within normal LLM variance against
the seeded goldens.

### Phase 2: Webpage-mode streaming (PR-2)

**Goal:** Ship streaming + early fan-out for `GenerateExperienceFromWebpage`
and `GenerateGearFromWebpage` across all three providers.

**Tasks:**
- [ ] Add streaming variants
      `GenerateExperienceFromWebpageStreaming`,
      `GenerateGearFromWebpageStreaming` to `ai.Provider`. Same
      uniform-channel signature.
- [ ] Implement streaming in each provider. Webpage inputs are
      `pageTitle`, `pageDescription`, `pageBody`, `region` (+ `currentTime`
      for Experience) — the input plumbing differs from image mode but
      the streaming output handler is identical.
- [ ] Drain-wrapper refactor of the two unary methods.
- [ ] Service-layer rewrite: **verify the webpage-mode handlers' fan-out
      shape against HEAD before assuming `runStreamingFanout` is the
      right adoption.** The Phase 1 verification (2026-04-26) found that
      image-mode services don't use `fanout.Run` today; the same may
      apply to webpage mode. If webpage handlers also bypass fan-out,
      this task collapses to "no service-layer change needed" the same
      way Phase 1 did. If they do use fan-out, switch to
      `fanout.StreamingRun` per the original plan.
- [ ] Structured logging additions matching Phase 1 / Phase 4a.
- [ ] Unit tests per the Phase 1 pattern (reuse `ai.MockProvider`
      streaming helper authored in Phase 1).
- [ ] Eval-harness run against
      `experience_webpage_goldens.json`, `gear_webpage_goldens.json`.
      Append a Phase-2 entry to the eval doc.
- [ ] One integration test covering a webpage-mode flow end-to-end
      (analogous to Phase 1 image-mode integration test).
- [ ] File-size check on the streaming files (see Phase 1).
- [ ] `npm run build` + `go build -tags=integration ./...`.

**Validates:** Same shape of win as Phase 1, on the second mode group.
After this phase, every `Provider.Generate*` and `Provider.Detect*` method
that has a post-AI fan-out is on the streaming path.

### Phase 3: Image/webpage streaming events on the server handlers (extends 4b)

**Goal:** Replace the `streamGen*FromUnary` short-circuits with first-class
image and webpage streaming branches that emit incremental
`title`/`description`/`geocoded`/`media_ready` events the same way text
mode does. Adds the two missing proto fields needed to make the request
and gear streaming RPCs mode-complete.

**Sequencing:** Phase 3 splits cleanly into 3a (image) and 3b (webpage),
matching the Phase 1 / Phase 2 split. Each can be its own PR or
co-shipped with its Phase 1 / Phase 2 counterpart depending on diff size.
The vertical-pair gate is what produces the user-visible win:

- **3a (image)** depends on Phase 1 having landed.
- **3b (webpage)** depends on Phase 2 having landed.

**Tasks — proto additions:**
- [ ] Add `string media_id = N;` to `StreamGenRequestRequest` (or, per
      Open Questions, refactor `prompt` into a `oneof prompt { string
      text; string media_id; }` matching `StreamGenExperienceRequest`'s
      shape — wire-compatible since field 1 stays put).
- [ ] Add `string website_url = N;` to `StreamGenGearRequest` (or
      similarly refactor into a oneof). Wire-compatible.
- [ ] `npm run generate` to regenerate Go + Dart stubs.
- [ ] No proto change needed for `StreamGenExperienceRequest` —
      already mode-complete.

**Tasks — Phase 3a server handler (image):**
- [ ] `server/services/experience/gen_ai_streaming.go`: replace the
      `*api.StreamGenExperienceRequest_MediaId` branch's call to
      `streamGenExperienceFromUnary` with a new
      `streamGenExperienceFromImage` modeled on
      `streamGenExperienceFromText` (line 88). Emits
      `description`-from-prompt-equivalent (likely empty since image
      mode has no text prompt — emit nothing here, or an empty string,
      matching unary behavior), opens the streaming AI call via the
      Phase-1 `GenerateExperienceFromImageStreaming`, fans Mapbox out
      on `location_query` close and Pexels on `search_keywords` close.
- [ ] `server/services/request/gen_streaming.go`: add
      `streamGenRequestFromImage`. Same pattern. Drives the new
      `media_id` proto field through to the streaming provider.
- [ ] `server/services/gear/gen_ai_streaming.go`: replace the unary
      fallback for `MediaId != ""` with `streamGenGearFromImage`. Gear
      uses `title` as the stock-image trigger (not `search_keywords`),
      consistent with text-mode gear streaming.
- [ ] Per-handler structured logging additions: same `streaming_*_ms`
      field set as text mode, plus an `input_mode=image` field on the
      `"post-AI fan-out completed"` span so prod queries can split by
      mode.
- [ ] Unit tests using the streaming `ai.MockProvider` per Phase 1 +
      capture-sender pattern from existing
      `gen_ai_streaming_test.go`. Verify image-mode streams emit the
      expected event sequence and the terminal `final` matches the
      unary handler's response shape.
- [ ] Integration test: one image-mode end-to-end streaming flow per
      service, asserting the stream produces multiple non-terminal
      events before the terminal `final` (today's behavior emits
      exactly one event — the terminal — so this is a strong
      regression canary).

**Tasks — Phase 3b server handler (webpage):**
- [ ] `server/services/experience/gen_ai_streaming.go`: same replacement
      for `*api.StreamGenExperienceRequest_WebsiteUrl` —
      `streamGenExperienceFromWebpage`.
- [ ] `server/services/gear/gen_ai_streaming.go`: add
      `streamGenGearFromWebpage` driven by the new `website_url` proto
      field. (Decide here whether the existing `DetectGearFromURL`
      unary RPC stays as-is — recommendation per Open Questions: yes,
      it serves a different flow.)
- [ ] Request webpage: out of scope. Today's
      `request_creation_view_model.dart` only routes URL-text into
      `genExperience` (Experience flow). There is no Request webpage
      flow today. Don't add one.
- [ ] Logging additions with `input_mode=webpage`.
- [ ] Unit + integration tests per Phase 3a pattern using webpage
      goldens.

**Tasks — observability and eval:**
- [ ] Append a Phase-3a entry to
      `docs/evals/1157-genexperience-latency.md` with per-handler
      `streaming_first_delta_ms` and `final_close_ms` for image mode
      across all three providers. Compare against the Phase-1 unary
      baseline (which is also what
      `streamGenExperienceFromUnary` currently produces, since it
      drains the unary handler).
- [ ] Phase-3b entry for webpage mode, same shape.
- [ ] `npm run build` + `go build -tags=integration ./...`.

**Validates:** Image-mode and webpage-mode streaming RPCs now emit
multiple incremental events instead of terminal-only. The eval
harness's existing image+webpage goldens still pass (final-event
shape unchanged). Per-mode `streaming_first_delta_ms` shows
sub-second first-paint for image and webpage flows in prod logs after
deploy.

### Phase 4: Client wiring for image/webpage streaming (extends 4c)

**Goal:** Make the Flutter client actually surface the new streaming
events for image and webpage modes. Most of this is small —
Experience already calls `streamGenExperience` for all three modes, so
the win flows automatically once Phase 3 server changes deploy. Request
needs the most client work because today's viewmodel guards image
streaming out entirely.

**Tasks — Experience:**
- [ ] No code change. Verify in a manual smoke test (and in the
      `gen_probe` log entries already emitted by the viewmodel) that
      image and webpage flows now log `first_event` /
      `geocoded_event` / `media_event` before `final`, instead of
      jumping straight to `final` as they do today.

**Tasks — Request:**
- [ ] Update `app/lib/data/repositories/request_repository.dart`'s
      `streamGenRequest()` signature to accept `mediaId` (mirroring
      `streamGenExperience`). Pass it through to
      `_service.streamGenRequest(...)` populating the new proto
      field.
- [ ] `app/lib/presentation/viewmodels/request_creation_view_model.dart`:
      drop the `if (state.inputMode != CreationInputMode.text)` guard
      at line 200. Branch on `inputMode` to pass either `prompt` or
      `mediaId` to `streamGenRequest`. For image mode, upload media
      first via `MediaRepository.addMedia` (same as Experience does at
      lines 246–266).
- [ ] Remove the now-redundant unary `genRequestFromMedia` path in
      the viewmodel's image branch — replace with the streaming path.
      Keep the unary RPC and repository method on the server/client
      surface for now (don't break callers); just stop using it from
      the creation viewmodel.
- [ ] Update existing widget tests to cover the new image-streaming
      path, mirroring the existing Experience test scaffolding.

**Tasks — Gear:**
- [ ] Update `app/lib/data/repositories/gear_repository.dart`'s
      `streamGenGear()` to accept `websiteUrl`.
- [ ] `app/lib/presentation/viewmodels/gen_gear_view_model.dart`:
      check the current image+webpage branching. Image mode at
      `gen_gear_view_model.dart:382` already calls
      `streamGenGear(prompt: prompt)` — verify it now passes
      `mediaId` for image input; the existing flow likely already
      handles it, but the unary `streamGenGearFromUnary` short-circuit
      meant the difference wasn't visible. Add `mediaId` parameter
      passing if missing.
- [ ] Webpage flow for gear: today goes through `DetectGearFromURL`
      (unary). Per Open Questions, keep that flow unchanged in this
      issue's scope. The new `StreamGenGear` `website_url` field is
      available for a future consolidation but doesn't replace
      `DetectGearFromURL` here.
- [ ] Widget tests for the new gear image-streaming path.

**Tasks — UI verification:**
- [ ] Smoke-test all three creation flows in image mode and webpage
      mode (where applicable) on a dev build. Verify skeleton UIs
      progressively fill in (title → description → geocoded →
      media → final) instead of staying blank until the terminal
      event. Per `CLAUDE.md` "For UI or frontend changes, start the
      dev server and use the feature in a browser before reporting
      the task as complete."
- [ ] Capture and attach a screen recording or before/after timing
      table to the PR description showing perceived-latency
      improvement.

**Validates:** End-to-end perceived latency win: image and webpage
flows show partial fields within ~500–800 ms (matching text-mode
4c behavior) instead of staying blank for ~3–5 s until the terminal
event. The `gen_probe` log entries in
`*_creation_view_model.dart` will surface this in prod.

### Phase 5: Documentation cleanup

**Goal:** Close out the issue with up-to-date docs.

**Tasks:**
- [ ] Add a "Follow-up complete: see #1229" pointer to the top of
      `docs/issues/1157-genexperience-latency.md`. Note that 4b/4c
      were extended to image+webpage modes here, not in #1157.
- [ ] Confirm `docs/evals/1157-genexperience-latency.md` has entries
      for Phase 1 (image internal), Phase 2 (webpage internal),
      Phase 3a (image client-visible), Phase 3b (webpage
      client-visible), and Phase 4 (end-to-end perceived latency
      smoke).
- [ ] If `#1226` (schema-consolidation) hasn't landed by this point, leave
      a note in #1226 listing the new image/webpage streaming impls'
      schema copies as additional surface area to consolidate.
- [ ] If `#1260` (remove unary gen path + `ENABLE_GEN_STREAMING` flag)
      hasn't landed, expand its scope note to cover image+webpage
      modes too — they're now safe to remove from the unary path
      after Phase 4 deploys cleanly.
- [ ] Close #1229.

**Validates:** Future readers landing on either #1157 or #1229 can find
each other and the eval evidence.

## Related Issues

- **#1157** — Parent latency-optimization issue. Phase 4a (PR #1218) shipped
  text-mode streaming; this issue extends the same pattern to image+webpage.
- **PR #1218** — `1157-phase4a-server-streaming` (merged 2026-04-22).
  Established the `StreamingJSONFields` tokenizer, `fanout.StreamingRun`,
  unary-wraps-streaming refactor pattern, and per-handler streaming log
  fields. Direct prerequisite — already satisfied.
- **#1226** — Consolidate Gen* JSON output schemas into a single source of
  truth per method. Will see additional duplication after #1229 lands
  (each new streaming impl currently copies its method's schema into the
  provider file). Worth flagging in #1226 once Phase 2 here ships.
- **#1260** — Remove unary gen path and `ENABLE_GEN_STREAMING` flag (#1157
  cleanup). Once #1229 ships, the unary code paths for image+webpage
  modes also become removable; #1260's scope expands from text-mode-only
  to all-modes if it hasn't run yet. Also: the
  `streamGen*FromUnary` short-circuits this plan replaces in Phase 3
  go away entirely after #1260 — the unary handlers they wrap will be
  gone.
- **#1216** — Re-evaluate AI provider model defaults using Phase 4a
  benchmark findings. Streaming behavior is per-provider; image+webpage
  numbers from Phase 1 / Phase 2 here feed back into that re-evaluation.

---

### Audit findings (2026-04-26)

Reviewed against `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`. Five fixes applied; one
boundary confirmation; nothing blocking.

1. **Mock strategy: use the canonical `ai.MockProvider`, not ad-hoc fakes.**
   `docs/server/architecture.md` §Mock Strategy says "use a single shared
   mock implementation where possible" and `ai.MockProvider` already exists
   at `server/ai/provider_mock.go`. Phase 1 / Phase 2 task lists updated to
   say "extend `ai.MockProvider` with a streaming-event helper" instead of
   spawning per-test fakes.

2. **Logging doc reference: `docs/server/observability.md`, not `logging.md`.**
   CLAUDE.md and the #1157 plan both reference `docs/server/logging.md`,
   which doesn't exist in the repo at HEAD — content lives in
   `docs/server/observability.md`. Plan now points at the real file and
   notes the upstream reference rot so the next reader doesn't chase a
   404.

3. **Integration test gap closed.** `docs/server/architecture.md` §Testing
   Strategy classifies "complete RPC flows" as integration-test territory
   for major features. The original plan only listed unit tests + eval
   harness. Phase 1 and Phase 2 now require one integration test per
   mode-group (image-mode in Phase 1, webpage-mode in Phase 2) with
   scripted-streaming `ai.MockProvider` + real testcontainers PostgreSQL.
   Not full-Cartesian coverage — one flow per group is enough since the
   streaming machinery is shared.

4. **File-size pre-merge check added.** Architecture doc §File Size
   Guidelines targets <500 lines, max 1000. Existing
   `provider_anthropic_streaming.go` (352), `_openai_streaming.go` (364),
   `_gemini_streaming.go` (282) will roughly double when five new methods
   land. Phase 1 and Phase 2 now include a check: if any file crosses
   ~800 lines, split per-mode-group before merge
   (`provider_<name>_image_streaming.go`,
   `provider_<name>_webpage_streaming.go`). Cleanest split mirrors the
   PR structure.

5. **Header note added** linking to this audit section so reviewers can
   see at the top that the plan was audited and what changed.

**Boundary confirmation (no fix needed):** Plan keeps the RPC contracts
unary on the wire. `app/lib/data/repositories/`, `app/lib/services/`,
ViewModels, Stash caching, and ARB i18n are all unaffected — the client
sees the same `GenExperience{...}Response` it sees today, just sooner.
This is consistent with the Phase 4a/4b/4c sequencing in
`docs/issues/1157-genexperience-latency.md`, which explicitly defers any
client-visible streaming surface to phases #1229 deliberately scopes out.

**Service Independence (no fix needed):** All work is inside
`server/ai/` (shared library) and three existing service packages
(`experience`, `request`, `gear`). No service-to-service injection is
introduced; the `fanout` shared library and `ai.Provider` interface
remain the only cross-package surfaces. Aligns with §Core Design
Principles #1.
