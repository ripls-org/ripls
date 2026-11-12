# Issue #2731: Create only extracts one need from a request that plainly names several

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2731
**Filed:** 2026-07-16 | **Last activity:** 2026-07-16
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/server/llm.md`, `docs/client/needs.md`, `docs/server/architecture.md` (anchor), `docs/server/conventions.md` (anchor), `docs/client/architecture.md` (anchor), `docs/client/conventions.md` (anchor), `docs/proto_conventions.md` (anchor), `docs/client/create.md`, `docs/walkthroughs.md`; review dimensions: api_design, error_handling, testing

## Problem Statement

When a request's text plainly enumerates the things it needs, create keeps only
the first and drops the rest. `GenerateRequestSuggestions` returns a single
`SeedNeed` — "the ONE primary thing or help being asked for" (`server/ai/prompts.go:934`,
`server/ai/provider.go:197`) — and `SubmitRequest` seeds exactly that one
`PlanningNeed` at creation (`server/services/request/lifecycle.go:135`). So:

> *"…we need picture books, a whiteboard, storage bins, art supplies, and
> construction paper — new or gently used."*

…is born with **one** need (*Picture books*); the other four are named,
unambiguous, and thrown away. The requester then re-types (or re-pastes) each one
into the compose sheet, having already written them once.

**The decision is made.** The issue's "Worth deciding" fork — seed the extracted
list outright vs. offer it as chips to confirm — is resolved by the issue owner:
**when the text names a clear list of needs, seed them directly as needs.** This
plan implements the seed-directly path. It is a graceful ladder that *subsumes*
today's behavior:

- text names **nothing** concrete → born with an **empty** list (the compose
  sheet "What would help?" opens from that empty row — the #2724 behavior)
- text names **one** primary thing → born with **one** need (today's behavior)
- text plainly enumerates **several** → born with **all of them** (the fix)

### What #2724 already fixed (don't re-litigate)

Two things the issue's comment flagged are already on the current tree (via the
#2724 branch), and this issue builds on them — do not undo them:

1. **The title-fallback seed is gone.** `SubmitRequest` seeds a need *only* when
   the generation names something concrete (`lifecycle.go:135` — `seedNeedName != ""`).
   A request whose text names nothing is born empty, which is what the compose
   sheet exists to fill. (The proto comment on `seed_need_name`,
   `request_service.proto:174-178`, still describes the removed title fallback —
   **stale, fix it in this PR**.)
2. **The compose sheet is now reachable.** `_add()` opens the multi-piece "What
   would help?" sheet when the needs list is empty
   (`request_read_shell.dart:748-763`); an empty-born request now reaches it.

## Context from Comments

One comment (from the issue owner, `maintainer`), which revises the body:

- **Multi-item extraction already exists — from a paste, not from the request
  text.** The compose sheet's "Paste a list" (`composePasteTrigger`) splits a
  comma/semicolon/newline list into one piece each (`request_compose_view_model.dart:162`
  `addPasteList`). The gap is narrower than "we can't do this": we can, *once the
  requester pastes*. What we don't do is read the list out of the request's own
  text at create time.
- **`BreakdownPieces` is the right shape but the wrong surface.** It reaches only
  the compose sheet's suggestion chips, which open from the empty-needs row — so
  it's unreachable on exactly the requests born with a need. (This plan does **not**
  overload `BreakdownPieces`; see Recommendation.)
- **The reel currently films the paste, not extraction.** The classroom
  walkthrough (`e2e/tests/walkthroughs/classroom.spec.ts`) deliberately has Ms.
  Okafor name *nothing* in her request, then paste the five-item list on camera
  (scene 01, lines 317-335) — because create only extracts one. The issue owner
  wants the reel redone against the real extraction flow **as part of this PR**
  (see Phase 5): the top-level prompt names all five, create is born with all
  five, and the paste beat is dropped.
- **The deterministic e2e provider is one line from doing this.** `e2eSeedNeed`
  (`server/ai/provider_e2e.go:369`) already scans a vocabulary that includes all
  five classroom items and returns the first match; returning *all* matches makes
  the reel film the real flow.

## Potential Solutions

The layers are the same for every option; they differ only in **how the
"born-with needs" list is modeled** end to end.

### Option A: Generalize `seed_need` (single) → `seed_needs` (repeated) — RECOMMENDED
- **Approach:** Promote the existing single-need primitive to a list at every
  layer. `RequestSuggestionResult.SeedNeed string` → `SeedNeeds []string`;
  `GenRequestResponse.seed_need` / `GenRequestFromMediaResponse.seed_need` (proto
  field 12) → `repeated string seed_needs`; `SubmitRequestRequest.seed_need_name`
  (field 8) → `repeated string seed_need_names`. `SubmitRequest` loops and inserts
  one `PlanningNeed` per name. The prompt instructs the model to return *every
  claimable thing the text plainly names, primary first* — one element for a
  single request, several for an enumerated list, empty when nothing concrete.
- **Pros:** One source of truth for "the needs the request is born with." The
  0/1/N ladder falls out naturally and preserves both existing behaviors. Aligns
  with *prefer generic primitives* and *no speculative API fields*.
  **Wire-compatible migration:** proto3 singular↔repeated of the same scalar type
  share wire format (a `string` value is one LEN record on field N; a
  `repeated string` is zero-or-more LEN records on the same field N — a single
  value decodes as a one-element list and vice-versa). **Keep the same field
  numbers (8 and 12), no reserve.** Reusing the number is the deliberate choice:
  an old app build that still sends one `seed_need_name` on field 8 decodes as a
  one-element `seed_need_names` on the new server (born with one need — graceful).
  A *new* field number would instead make the old client's value invisible (born
  empty) — a regression until every app build updates, which for a mobile client
  is never instantaneous. Generated code isn't checked in, so client + server
  regenerate together. Note: `buf.yaml` declares a `breaking: FILE` ruleset, but
  **`buf breaking` is not invoked by any CI workflow or npm script** (only
  `buf lint` runs), so the cardinality + name change trips no mechanical gate;
  it is nonetheless a source-level rename, called out here so review isn't
  surprised.
- **Cons:** Touches the field name in the proto (source-level rename) and every
  client/server reference to it (~3 client wiring points, ~4 server files). The
  preview modal's single editable "Needs" field becomes a small editable list.
- **Effort:** Medium.

### Option B: Keep `seed_need`, add a parallel `seed_needs`
- **Approach:** Leave the single field alone; add a new repeated field carried
  alongside it, with `seed_need == seed_needs[0]` by convention.
- **Pros:** Smallest diff to existing single-need call sites.
- **Cons:** Two fields for one concept invites drift (which wins when they
  disagree?). Every consumer must learn to prefer the list. Violates *prefer
  generic primitives* — the single field becomes dead weight the day the list
  ships. Rejected.
- **Effort:** Small-medium (but worse to live with).

### Option C: Reuse `BreakdownPieces` as the seed list
- **Approach:** Seed the request from `BreakdownPieces` instead of adding a
  concept.
- **Pros:** No new field.
- **Cons:** `BreakdownPieces` means "*inferred* sub-tasks that make the request
  more claimable" (decomposing one request into steps), which is a **different semantic**
  from "*plainly named* distinct needs." Overloading it would seed speculative
  decompositions the requester never wrote, and it already feeds the compose
  sheet's chip surface for a different purpose. The issue owner calls it "the
  right shape" only loosely; conflating the two surfaces is the wrong move.
  Rejected.
- **Effort:** Small (but semantically wrong).

## Recommendation

**Option A.** Generalize the existing `seed_need` primitive to `seed_needs`
end to end. It is the cleanest expression of the issue owner's decision ("add
them directly as needs"), it subsumes both current behaviors as the N=0 and N=1
cases, it keeps `BreakdownPieces`/`AdditionalAsks` doing their distinct jobs, and
— because singular↔repeated is wire-compatible — it migrates without field
renumbering or a flag day.

The one genuine risk lives in the **real-LLM prompt**, not the plumbing: the model
must seed multiple needs *only when the text plainly enumerates them* (high
precision), or a vague description explodes into speculative needs. The
deterministic e2e path is exact (vocabulary scan). The cloud path needs a
precision-favoring prompt and, ideally, a small extraction eval (see Open
Questions). The plumbing is safe to build first; the prompt is where review
attention belongs.

**Why `ripls-ui-investigate` was skipped:** the misbehavior is already understood
and specified — the issue and its comment bisect it precisely to the
server-side extraction returning one need, and the fix is a known change to the
create/extraction path, not a mystery to reproduce. There is no ambiguous layer
to isolate. (The classroom reel already demonstrates the current one-need
behavior on the real bundle.)

## Open Questions

- **Cap on seeded needs?** — Recommendation: cap at **8** via
  `truncateSuggestions` (the existing cap for `offer_ideas`), applied in the
  provider parse, and dedup case-insensitively in `SubmitRequest` before insert.
  Eight is generous for a "plainly named" list and bounds the birth cost.
- **Precision guard for the cloud prompt?** — Recommendation: rewrite prompt item
  4 to demand items **the text explicitly names** ("do not invent needs the
  requester didn't write — inferred extras belong in `additional_asks` /
  `breakdown_pieces`"), primary first. Add a request-**suggestion** eval fixture
  (a few golden cases: single request → 1, enumerated list → N, vague → 0). Note:
  there is **no** suggestion-eval harness today (`server/ai/eval/request.go`
  evaluates `RequestGeneration`, not `RequestSuggestionResult`), so this is
  net-new scaffolding — acceptable to land as a fast-follow if it balloons scope,
  but the prompt rewrite itself is in-scope here.
- **Preview UX for the multi-need list?** — Recommendation: replace the single
  editable "Needs" field (`request_preview_modal.dart:564-578`) with a compact,
  removable **chip/row list** plus an inline add, so extraction is visible and the
  requester can drop a wrong one before saving. Keep it minimal — this is a
  preview affordance, not the full compose sheet. If the widget work is large,
  ship a read-only list first and make it editable in a fast-follow. **Any
  deferred fast-follow here (editable preview, suggestion eval) gets a GitHub
  issue filed before it's deferred** — per the TODO/deferral convention, "tracked
  separately" is not a substitute for a filed issue.
- **Migration for in-flight app builds?** — Recommendation: rely on
  singular→repeated wire compatibility; no reserve/renumber. Old app builds send
  one `seed_need_name` → server reads a one-element `seed_need_names` → born with
  one need (graceful). No server-side special-casing needed.

## Implementation Plan

### Phase 1: Server — extraction returns a list
**Goal:** `GenerateRequestSuggestions` yields every plainly-named need, primary
first, capped and deduped; deterministic providers return the whole list.
**Tasks:**
- [ ] `RequestSuggestionResult.SeedNeed string` → `SeedNeeds []string`
      (`server/ai/provider.go:197`); update the doc comment.
- [ ] Parse `seed_needs` (array) in `provider_openai.go`, `provider_anthropic.go`,
      `provider_gemini.go`; apply `truncateSuggestions(..., 8)`.
- [ ] Rewrite prompt item 4 in `server/ai/prompts.go:934` for multi-extraction
      with an explicit **precision** rule (only what the text names; inferred
      extras go to `additional_asks`/`breakdown_pieces`), primary first; update
      the JSON example.
- [ ] `provider_mock.go`: `SeedNeed` → `SeedNeeds`.
- [ ] `provider_e2e.go`: `e2eSeedNeed` → `e2eSeedNeeds` returning **all** matched
      vocabulary items in text order (`:369`, and the caller `:107`).
- [ ] Unit tests: `e2eSeedNeeds` multi-match/ordering; provider parse tests.
**Validates:** the model/deterministic layer can surface N needs from one text.

### Phase 2: Proto + wiring
**Goal:** the list flows from generation through create to storage.
**Tasks:**
- [ ] `GenRequestResponse.seed_need` (12) → `repeated string seed_needs`; same for
      `GenRequestFromMediaResponse` (`request_service.proto:87,138`). Place near
      the other chip lists per field-ordering convention.
- [ ] `SubmitRequestRequest.seed_need_name` (8, optional) → `repeated string
      seed_need_names`; **rewrite the stale comment** (drop the removed
      title-fallback line, `request_service.proto:174-178`).
- [ ] `server/services/request/gen.go`, `gen_streaming.go`, `contributions.go`:
      carry `SeedNeeds` → `seed_needs`.
- [ ] `SubmitRequest` (`lifecycle.go:135`): trim, case-insensitive dedup, and
      **cap server-side** (the provider cap is advisory — a client can send any
      count, so bound it here too; document the bound in the proto comment). Seed
      the needs with a **single `storage.InsertBatch`** (`server/storage/protosql_batch.go:172`),
      not N sequential `Insert` calls, to keep create off an N-insert path.
      Preserve the "empty list → born empty" guard, and keep the existing
      **best-effort** semantics — a seed-insert failure logs `Warn` and does not
      fail `SubmitRequest` (mirrors `lifecycle.go:146`). Treat index 0 as primary.
- [ ] `npm run generate`; fix compile.
- [ ] Tests: `seed_need_test.go` (→ multi), `gen_chips_test.go`, and a
      behavioral `SubmitRequest` test — create with N/1/0 names, then
      `ListRequestNeedsAndContributions` and assert the resulting need set. Note:
      `seed_need_names` is a **write-only create input** with no symmetric read
      field, so `AssertFieldRoundTrip` (`roundtrip_helpers.go`) does **not**
      apply; the create→list assertion is the contract test.
**Validates:** a create call with several names is born with several needs;
zero/one still behave correctly.

### Phase 3: Client wiring
**Goal:** the app reads the list, passes it to create, and shows it in preview.
**Tasks:**
- [ ] `GenRequestState.seedNeed` (String?) → `seedNeeds` (List<String>)
      (`gen_request_view_model.dart`); read `resp.seedNeeds`
      (`request_creation_view_model.dart:228`).
- [ ] `unified_create_state.dart` / `unified_create_view_model.dart` /
      `unified_create_save_actions.dart:196`: `requestSeedNeed` → `requestSeedNeeds`;
      pass `seedNeedNames`. Update the second submit path
      (`gen_request_view_model.dart:410`).
- [ ] `request_repository.dart:205` `submitRequest`: `seedNeedName` →
      `seedNeedNames`.
- [ ] `request_preview_modal.dart:564-578`: single field → removable chip/row list
      + inline add (per Open Questions; read-only fallback acceptable).
- [ ] l10n: pluralize `unifiedCreateSeedNeedLabel`/`Hint` as needed (en + es).
- [ ] Tests: gen / unified-create view-model tests; preview widget test.
**Validates:** the full text → preview → create → born-with-N-needs path in-app.

### Phase 4: Reconcile the compose sheet
**Goal:** born-with-N and the empty-born compose path stay coherent.
**Tasks:**
- [ ] Confirm a request born with N needs no longer routes to the empty-needs
      compose sheet (`_add()`, `request_read_shell.dart:754`) and that adding
      more needs still works.
- [ ] Confirm the paste-a-list disclosure still works for the empty-born case
      (no regression) — it remains the manual path when extraction found nothing.
**Validates:** extraction and the manual compose/paste paths coexist without
double-seeding.

### Phase 5: Redo the classroom reel (in this PR)
**Goal:** the walkthrough films the real extraction flow, not the paste.
**Tasks:**
- [ ] Rewrite `CREATE_PROMPT` (`classroom.spec.ts:88-91`) so the request text
      **plainly names all five** items (picture books, whiteboard, storage bins,
      art supplies, construction paper). Keep gear cues ("borrow"/"lend") OUT so
      the classifier still routes to Request.
- [ ] Delete the on-camera paste beat (scene 01, lines 317-335) and the off-camera
      "post the rest of the supply list" workaround (lines 278-280); the request is
      now born with all five.
- [ ] Update settle captions ("…first piece named" → "…five pieces, named for
      her") and `e2e/fixtures/walkthroughs/classroom/walkthrough.md` narration.
- [ ] Re-render the reel and verify all scenes green end to end
      (`e2e/scripts/run_walkthrough.sh`; build via `npm run build:e2e`).
**Validates:** one typed sentence → five claimable needs, on camera, truthfully.

### Phase 6: Docs
**Goal:** durable docs match the new behavior.
**Tasks:**
- [ ] `docs/server/llm.md`: `seed_needs` (list) on `RequestSuggestionResult`.
- [ ] `docs/client/needs.md` / `docs/client/create.md`: "born with the needs the
      text names (0/1/N)."
- [ ] `docs/walkthroughs.md`: classroom reel now films extraction, not paste.
- [ ] Advance `freshness:` stamps on touched durable docs.
**Validates:** doc-freshness gate stays green; prose matches code.

## Related Issues
- #2702 — introduced the single `SeedNeed` seed-at-creation this generalizes.
- #2724 — removed the title-fallback seed and made the compose sheet reachable;
  this plan builds directly on that tree and redoes its classroom reel.
- #2703 — the classroom-supply-drive reel whose scene 01 changes here.

## Audit Notes

Audited against the documentation context map on 2026-07-22. Adjustments folded
into the plan above:

- **Proto migration (api_design):** Confirmed `buf breaking` is *configured*
  (`buf.yaml` → `breaking: FILE`) but **not invoked** by any CI workflow or npm
  script — only `buf lint` runs. Hardened the migration guidance: reuse field
  numbers 8/12 (no reserve) so old app builds stay wire-compatible via
  singular↔repeated equivalence; a new number would strand old clients. Called out
  that the rename is a source-level break with no mechanical gate.
- **Repeated-field bound (api_design):** The review checklist flags unbounded
  repeated fields. Made the cap **server-enforced** in `SubmitRequest` (not only
  in the provider parse) and documented in the proto comment.
- **Performance (server conventions, N+1):** Switched the seed loop to a single
  `storage.InsertBatch` instead of N sequential `Insert` calls, keeping create
  off an N-insert path.
- **Error handling (server conventions):** Made the best-effort semantics explicit
  — a seed-insert failure logs `Warn` and does not fail `SubmitRequest`, matching
  the existing single-need path.
- **Testing (proto_conventions):** Corrected the test shape — `seed_need_names` is
  a write-only create input with no symmetric read field, so `AssertFieldRoundTrip`
  doesn't apply; the contract is a behavioral create→list assertion.
- **Deferral discipline (TODO convention):** Deferred fast-follows must be filed as
  issues, not left as "tracked separately."
- **Terminology (never "ask" as a noun):** Rewrote noun uses of "ask" to
  "request." Left the pre-existing `additional_asks` proto field name untouched
  (renaming it is out of scope).

Docs consulted: `docs/proto_conventions.md` (anchor), `docs/reviews/api_design_review_prompt.md`
(Areas to Review), `docs/server/architecture.md` + `docs/server/conventions.md`
(anchors), `docs/client/architecture.md` + `docs/client/conventions.md` (anchors),
`docs/server/llm.md`, `docs/client/needs.md`, `docs/client/create.md`,
`docs/walkthroughs.md`, `docs/server/observability.md` (anchor). Verified against
code: `buf.yaml`, `server/storage/protosql_batch.go`, `server/services/request/lifecycle.go`,
`server/ai/{provider.go,prompts.go,provider_e2e.go}`, `proto/ripls/api/request_service.proto`.
