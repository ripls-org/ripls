# PR #1886: Phase 1 scaffolding for unified-create — replan as three stacked PRs

**Source:** https://github.com/Type2Fun-Foundation/ripls/pull/1886
**Filed:** 2026-05-08 (approx, based on first P1.1 commit) | **Last activity:** 2026-05-12
**Labels:** none | **Assignees:** briangbrown
**Context docs:** `docs/design/unified-create.md`, the working branch `unified-create-phase-1` (39 commits, 58 files, +5892/−282)

## Problem Statement

PR #1886 is the entire Phase 1 implementation of unified-create — a single
"share something" entry point that classifies user input (text / image / URL)
into Request / Event / Item and runs the matching per-type generator behind a
build-time flag (`ENABLE_UNIFIED_CREATE`, off in every env). The work is
functionally complete and validated end-to-end on a Pixel emulator against a
local dev server.

The PR is **too big to review well**: 5,892 added lines across 58 files,
spanning proto schema, Go server services + AI prompts + classifier + tests,
and Flutter UI + view models + widgets + ARB strings + integration tests.
Even with the per-issue commits called out in the body (P1.1–P1.11), the
PR mixes three independently reviewable domains and a substantial mid-PR
refactor (delegation from `unified_create` into the per-type single-create
generators via a shared `server/genai/*` library).

The request: split #1886 into **three stacked PRs** — proto, server, client —
each landing in dependency order, each small enough that one reviewer in one
sitting can audit it.

## Context from Comments

PR #1886 has zero comments. All context for this replan comes from in-session
work and the design doc. Key facts to carry forward:

- **Functional code already exists** on `unified-create-phase-1`. This is a
  re-organization of finished work, not a re-implementation. The plan must
  preserve every existing commit's content; only the topology changes.
- **The PR description already maps each Phase 1 issue (#1867–#1877) to one
  or more commits.** That mapping is the basis for the three-PR split — the
  P1.x issues are roughly proto/server/client-shaped already.
- **A non-trivial refactor landed mid-PR** (`fc7c7275d` "Delegate
  unified-create to single-create generators via shared library") that
  pulled `experiencegen`, `geargen`, `requestgen` types into a new
  `server/genai/` package and exported the per-type streaming functions
  with new `*StreamSender` interfaces. The server PR must include this
  refactor *with* its callers updated; splitting the refactor across PRs
  would leave one side broken.
- **The branch is 39 commits ahead of main and 18 commits behind.** A
  fresh rebase onto main is part of the split work.
- **Three stacking constraints** (in conversation):
  - Reviewed and merged in order (protos → server → client).
  - Each PR is a single commit (not three commits in one PR).
  - The three-PR boundary is determined by domain, not by file count.
- **`package.json` change is out of scope.** The `--gemini-model` removal got
  blocked by gitleaks (pre-existing hardcoded API keys on the same line) and
  the user decided to drop it. The hardcoded-secrets blocker was independently
  resolved on `main` after the rebase (start:server now sources secrets via
  `scripts/run_server_with_sm_secrets.sh`); the `--gemini-model` flag is still
  there but unblocked. Still not part of this replan — tracked as a separate
  follow-up.
- **Branch rebased onto `origin/main` HEAD after the plan was written.** All
  39 commits replayed cleanly with zero conflicts; the 6-file overlap (4 env
  JSONs + 2 ARBs) auto-merged. File-level scope unchanged (still 58 files,
  +5892/−282). Plan tables, file lists, and PR boundaries below remain
  accurate post-rebase.

## Potential Solutions

### Option A: Soft-reset and split working tree by domain

- **Approach:** On the existing branch, `git reset --soft $(merge-base main)`
  to collapse all 39 commits into the working tree. Then create three new
  branches off `main` (each stacked on the previous), and on each branch
  selectively stage + commit the files belonging to that domain. Close #1886
  with a reference to the three replacements.
- **Pros:** Simplest mental model. Working tree already has the final state
  of every file; staging by domain is mechanical. Generated code stays
  unchecked-in (per CLAUDE.md). No risk of dropping a file because the
  working tree is the source of truth.
- **Cons:** Loses the per-issue commit history (P1.1–P1.11 + iteration
  commits). The three new commits each have a single squashed message
  covering 1–2k lines. Bisecting between the three is coarser.
- **Effort:** Small. One branch-creation + selective-`git add` pass per PR.

### Option B: Cherry-pick by commit into three branches

- **Approach:** Map each of the 39 commits to one of {proto, server, client}
  using the P1.x labels in the PR body. Create three branches stacked on
  `main` (rebased to current HEAD) and cherry-pick the relevant commits into
  each. Close #1886.
- **Pros:** Preserves commit-level history. Each PR still shows the
  incremental work (e.g. server PR shows the classifier coming in, then the
  real Anthropic backend, then the eval-runner test).
- **Cons:** Several commits cross domain boundaries:
  - `fc7c7275d` "Delegate unified-create to single-create generators via
    shared library" touches `server/genai/*`, `server/services/experience/`,
    `server/services/gear/`, `server/services/request/`, AND
    `server/services/unified_create/`. It belongs to "server" but it
    refactors single-create internals to support the unified caller — the
    delineation is real but the diff is one logical change.
  - `598218236` "Run post-creation after unified-create modal closes"
    touches both `app/lib/presentation/screens/create/unified_create_modal.dart`
    AND `app/lib/presentation/screens/home/home_screen.dart` AND
    `app/lib/presentation/viewmodels/unified_create_save_actions.dart` —
    all client, but it's also coupled to behaviour validated only against
    the running server.
  - `483c05c33` "Wire real per-type streaming + context + UX polish" spans
    server + client + protobufs all at once.
  Picking these cleanly across three branches means hand-editing the
  cherry-picks anyway, which converges back to Option A.
- **Effort:** Medium. Conflict resolution per cross-domain commit; 39
  cherry-picks to script and verify.

### Option C: Three full PRs of new code authored from main

- **Approach:** Throw away the branch. Re-author the work from `main` in
  three new branches, building only the proto/server/client surface in each.
  Reuse the existing code verbatim by copying files, but write fresh commits
  scoped to each PR.
- **Pros:** Cleanest history. Each PR reads like a fresh, intentional change
  (not "a slice of an in-flight refactor").
- **Cons:** Discards all the per-issue context (P1.1–P1.11 mapping) and the
  iteration commits that document why certain UX decisions were made. We've
  also already pushed #1886 — losing the AI-evals goldens, classifier prompt
  history, and other work that was validated in-branch means losing the
  audit trail. Effort-wise it's a re-do of the integration step (testing
  end-to-end against a Pixel + local server).
- **Effort:** Medium-large. Re-validating the stack end-to-end after each
  PR's contents shift.

## Recommendation

**Option A (soft-reset + split working tree).** Three reasons:

1. **The working tree is canonical.** Every file's *final* contents on
   `unified-create-phase-1` represent the validated state — we've verified
   the gear-share end-to-end flow, fixed the spinner bug, aligned the gates.
   Anything less than that final state needs re-validation. Option A
   preserves it bit-for-bit per file.
2. **Cross-domain commits ARE the noise we're removing.** Commit
   `483c05c33` ("Wire real per-type streaming + context + UX polish") spans
   proto + server + client because the work iterated against a running
   stack. The point of the three-PR split is that we *no longer* want
   reviewers to context-switch across those boundaries — so preserving the
   cross-domain commits (Option B) defeats the purpose.
3. **The P1.x history is already in the design doc and PR description.**
   Future readers wanting "why did this preview row fade in?" can grep the
   PR body, which lists the iteration commits. The replan commit messages
   should reference the design doc and the original PR #1886 for context;
   they don't need to re-encode the entire iteration log.

The single squashed commit per PR is a feature, not a bug: each PR's diff
is the smallest reviewable change that ships its layer. If we ever need
finer history we can resurrect `unified-create-phase-1` from refs.

## Open Questions

- **Should the three replacement PRs reference back to #1886, or to the
  original Phase 1 issues (#1867–#1877)?** — Recommendation: reference
  both. Each PR's body should say "Replaces part of #1886; closes
  #1867 [+ #1868 + ...]" with the subset of P1.x issues that PR covers.
  This keeps the GitHub-issue closeout automatic and the cross-link
  obvious.
- **Does the proto PR ship alone (134 lines, one file) or get bundled into
  the server PR?** — Recommendation: ship alone. The proto file is the wire
  contract; it's reviewed by a different lens (API stability, naming,
  field-number reservations) than Go service code. The downstream Go + Dart
  generators are gitignored (per CLAUDE.md) so the proto PR has truly
  zero runtime impact — it just publishes the contract.
- **Should the server PR include the `experiencegen` / `geargen` /
  `requestgen` library extraction, or land that as a separate PR first?**
  — Recommendation: include it. The extraction has no runtime semantics on
  its own — it only matters once `unified_create` consumes the library.
  Splitting it means one PR of "added unused library" sitting on its own,
  which adds review overhead without isolating risk.
- **Where do the test files go?** — Recommendation: with their target.
  `server/ai/prompts_unified_test.go` ships with the server PR (it tests
  the prompt builder shipping in the same PR). Flutter widget + view-model
  tests ship with the client PR. Integration tests under
  `server/services/unified_create/integration_test.go` ship with the server
  PR. No test file straddles two PRs.
- **Should the existing `unified-create-phase-1` branch be deleted after
  the split?** — Recommendation: keep it for one release cycle, then
  delete. Provides a fallback if any of the three replacement PRs uncovers
  a regression that needs the original integration context.

## In-hindsight improvements folded into the split

After re-reading the whole branch through the audit lens, three small
improvements are worth fixing as part of the split rather than punting.
None reshape the PR boundaries or grow the diff materially; each is a
correctness or staleness fix that's cheaper to do now than later.

1. **Move `_descriptionHint` strings into ARB keys (PR 3).** The
   description-editor placeholder text was introduced this session as
   hardcoded English strings inside `unified_preview_card.dart`:
   `'Describe the item…'`, `'Describe what you need…'`, `'Describe the
   event…'`, `'Add a description…'`. `npm run lint:dart:i18n` doesn't
   flag them today (the script's pattern set has gaps), but they violate
   the rule from `docs/client/i18n.md`: "Every user-visible string
   should use `context.l10n.*`." Adds four keys
   (`unifiedCreateDescriptionHintItem`, `*HintRequest`, `*HintEvent`,
   `*HintDefault`) in both `app_en.arb` and `app_es.arb` and switches
   `_descriptionHint` to look them up via `context.l10n`.

2. **Repoint the `// dart-error-tostring-allow #1886` markers to a new
   follow-up issue (PR 3).** Four sites in the unified-create viewmodel
   carry this lint-escape comment referencing the original PR
   (`unified_create_state.dart:111`,
   `unified_create_view_model.dart:132 / 200 / 294`). Once #1886 is
   closed, those references rot — the next reader cannot find what was
   deferred or why. Either:
     - **(a) File a follow-up issue** "Migrate unified-create
       `errorMessage: String` to typed `UserError`" and bump the
       markers from `#1886` to the new issue number, OR
     - **(b) Do the migration now** — wire
       `RpcErrorHandler.classify(e)` in each catch and change
       `errorMessage: String?` to `error: UserError?` in
       `unified_create_state.dart`. Widgets resolve with
       `RpcErrorHandler.localize(state.error!, context.l10n)`. Small,
       constrained refactor that matches the convention in
       `docs/client/i18n.md` § "Viewmodels never resolve strings".
   **Recommendation: (a)**. The migration is its own discussion (typed
   variants for stream-timeout vs. AI-provider vs. image-upload errors)
   and is not blocking; the marker repoint is one line per site.

3. **Add the `Gen*Response`-reuse rationale to PR 1's commit body and
   PR description (PR 1).** Audit finding A flagged that
   `StreamGenUnifiedCreateFinal.final_payload` is a `oneof` of
   `GenGearResponse` / `GenExperienceResponse` / `GenRequestResponse` —
   a deliberate exception to `CLAUDE.md`'s "Never reuse RPC request or
   response message types across multiple RPCs." Don't add the rationale
   as a proto comment (CLAUDE.md § Proto Comments forbids
   project-convention talk in `.proto` files — proto comments are the
   public API contract). Instead, surface the rationale in PR 1's body
   so the proto reviewer sees it without digging:

   > "StreamGenUnifiedCreateFinal reuses the per-type Gen*Response
   > messages in its final_payload oneof. This is a deliberate
   > exception to CLAUDE.md's no-reuse rule — see
   > docs/design/unified-create.md § RPC contract. The alternative is a
   > full translation layer that re-encodes every field on every
   > response, which would double the per-type churn whenever a field
   > is added."

**Items deliberately not in scope** for this split (still listed in the
Audit findings section below, repeated here for cross-reference):

- Save-dispatch path uses services directly instead of repositories
  (pre-existing pattern across all create surfaces; needs a coordinated
  refactor, not a one-off here).
- Hardcoded cycling-prompt list in
  `unified_create_input_drawer.dart` (12 example prompts, currently
  exempted by `check_i18n.js`). Marginal user-facing impact (the prompts
  are illustrative, not copy users dwell on); revisit when Spanish-
  speaking dogfooders surface examples that read awkwardly.
- Redundant `generation.Description = prompt` in
  `server/services/gear/gen_ai_streaming.go:330` now that the wire
  description is emitted earlier. Removing it has non-zero risk
  (the per-type Save path would need to be checked) and zero
  user-facing benefit; leave it.
- Goldens corpus growth to ≥20 per bucket and classifier accuracy
  threshold gate — Phase 2 work (P2.14).

## Implementation Plan

Each phase is one PR. The PRs stack: client merges last, depends on server
merging first, which depends on proto merging first. Treat the order as a
hard prerequisite — do not open the next PR until the prior one is
approved (or at least under active review with no blocking changes
expected to the contract).

### Phase 1: Proto contract (`unified-create-proto`)

**Goal:** Publish the wire contract for `StreamGenUnifiedCreate` so the
server + client PRs have a stable schema to depend on.

**Files (1):**

- `proto/ripls/api/unified_create_service.proto` (+134)

**Tasks:**

- [ ] Branch `unified-create-proto` from `main`.
- [ ] Copy `proto/ripls/api/unified_create_service.proto` from
      `unified-create-phase-1` verbatim.
- [ ] Run `npm run generate` locally to confirm both Go (`server/gen/`) and
      Dart (`app/lib/data/gen/`) code generates cleanly against current
      `main` (no schema collisions, no missing imports).
- [ ] Verify `buf lint` passes (standard rules — `unified-create-phase-1`
      already cleared this so it should be a no-op).
- [ ] Confirm no implementation/convention talk in the `.proto` comments
      per `CLAUDE.md` § Proto Comments. (The proto on the branch already
      complies — gate is here to catch a regression.)
- [ ] Commit message: see template under "Commit-message templates" below.
- [ ] Open PR `unified-create-proto` against `main`. Body references
      `docs/design/unified-create.md` § RPC contract, **explicitly
      surfaces the `Gen*Response`-reuse rationale per "In-hindsight
      improvements" #3 above** (so the proto reviewer sees it without
      digging), and closes #1867 (P1.1).

**Validates:** Generated Go + Dart code from the new proto compiles
standalone against `main` (no consumer code shipping yet). Reviewers can
audit the contract — field numbers, oneof shapes, comments — in isolation
from any implementation.

**Approx diff size:** +134 / −0, 1 file.

### Phase 2: Server (`unified-create-server`)

**Goal:** Land the server-side classifier, RPC handler, and per-type
generator delegation. After merge, the server speaks
`StreamGenUnifiedCreate` end-to-end but no client invokes it yet.

**Files (28):** all `server/` paths from the branch, plus
`docs/server/observability.md`. Listed in dependency order
(infrastructure → consumers → wiring → tests):

Shared library (introduced by the delegation refactor):

- `server/genai/experiencegen/types.go`
- `server/genai/geargen/types.go`
- `server/genai/requestgen/types.go`

Per-type single-create refactor (exports + sender abstraction):

- `server/services/experience/stream_sender.go` (NEW)
- `server/services/experience/gen_ai_streaming.go` (refactor: export
  `StreamGenExperienceFromText/Image/Webpage`, take `Context` + `StreamSender`)
- `server/services/experience/gen_ai_streaming_test.go`
- `server/services/experience/gen_ai_streaming_integration_test.go`
- `server/services/gear/stream_sender.go` (NEW)
- `server/services/gear/gen_ai_streaming.go` (refactor + emit prompt as
  description in text mode, per session fix)
- `server/services/gear/gen_ai_streaming_test.go`
- `server/services/request/stream_sender.go` (NEW)
- `server/services/request/gen_streaming.go`
- `server/services/request/gen_streaming_test.go`

AI: classifier + prompts + eval:

- `server/ai/prompts_unified.go`
- `server/ai/prompts_unified_test.go`
- `server/ai/classifier_unified.go`
- `server/ai/eval/testdata/unified_create_classifier_goldens.json`
- `server/ai/eval/unified_classifier_test.go`

`unified_create` service:

- `server/services/unified_create/doc.go`
- `server/services/unified_create/README.md`
- `server/services/unified_create/classifier.go`
- `server/services/unified_create/service.go`
- `server/services/unified_create/stream_adapters.go`
- `server/services/unified_create/stream_gen.go`
- `server/services/unified_create/stream_gen_test.go`
- `server/services/unified_create/integration_test.go`

Wiring + docs:

- `server/main.go` (UnifiedCreateService registration, +13)
- `docs/server/observability.md` (log-field documentation for the new
  `operation=StreamGenUnifiedCreate` tag)

**Tasks:**

- [ ] Branch `unified-create-server` from `unified-create-proto`. (The
      proto generated code is gitignored so this branch only inherits the
      `.proto` file; that's enough for `npm run generate` to wire Go
      consumers.)
- [ ] `git checkout unified-create-phase-1 -- <each file above>`.
- [ ] Verify with `npm run generate && npm run build` (runs golangci-lint
      including gofumpt/godot/revive, then `go build ./...`).
- [ ] Run unit tests: `go test ./server/services/unified_create/
      ./server/services/experience/ ./server/services/gear/
      ./server/services/request/ ./server/ai/`.
- [ ] Run integration tests: `go test -tags=integration
      ./server/services/unified_create/ ./server/services/experience/
      ./server/services/gear/ ./server/services/request/ ./server/ai/eval/`.
- [ ] Run `go build -tags=integration ./...` per CLAUDE.md (the per-type
      stream-sender signature changes are exported APIs that integration
      tests consume).
- [ ] Confirm `TestEveryEventTypeClassified` still passes — no new
      `CommunityEventType` values are added in this PR, so no
      `undo/registry.go` change should be needed; the gate is here to
      catch a surprise.
- [ ] Confirm `npm run lint:ephemeral-comments` is clean (no Phase 1 / P1.x
      markers should have leaked into source comments; they live in commit
      messages + this plan doc only).
- [ ] Confirm the `service.go` constructor uses **interface injection only**
      per `docs/server/architecture.md` § Service Independence — the
      delegation refactor must inject `experiencegen.Generator`,
      `geargen.Generator`, `requestgen.Generator` interfaces, not the
      concrete `experience.Service`, `gear.Service`, `request.Service`
      structs. (This is how `unified-create-phase-1` already wires it;
      the gate is here to catch a regression during the split.)
- [ ] Confirm structured-logging fields match
      `docs/server/observability.md` § Standard Field Names:
      `operation=StreamGenUnifiedCreate`, `request_id` (HTTP correlation,
      from `LoggerWithContext`), `user_id`, `user_email` masked via
      `logging.MaskEmail`, per-stage `duration_ms`. No raw user emails or
      AI prompt text in logs. The `target_request_id` field name is
      reserved for `models.Request` entity IDs and is **not** applicable
      to this RPC (the unified-create stream doesn't operate on a
      `models.Request` until the per-type Final lands, and even then the
      ID is logged under the request service's own `operation`).
- [ ] Confirm AI provider errors return through `connecterr.Internal` so
      raw upstream error text is logged server-side but never reaches the
      wire (`scripts/check_codeinternal.js` gate). Stream-level errors
      use the `GenStreamError` event, which is a public payload — verify
      it carries the typed error code (`GEN_STREAM_ERROR_CODE_*`) not raw
      upstream messages.
- [ ] Commit message: see template below.
- [ ] Open PR `unified-create-server` against `main` (NOT against the
      proto branch — by the time the server PR opens, the proto PR is
      either merged or close to merging; if the proto PR merges first,
      the server branch rebases trivially onto main).
- [ ] Body references `docs/design/unified-create.md` §§ RPC contract +
      LLM design + Architecture, and closes #1868 (P1.2), #1869 (P1.3),
      #1870 (P1.4).

**Validates:** Server speaks `StreamGenUnifiedCreate` end-to-end. Local
test via `grpcurl` or `buf curl` against the running dev server with a
canned text input returns a streamed sequence ending in a `final` event.

**Approx diff size:** ~+3000 / −150 across 28 files. (Per-type
`gen_ai_streaming.go` files account for ~−150 because of the delegation
refactor moving code into the new `genai/*` library.)

### Phase 3: Client (`unified-create-client`)

**Goal:** Wire the Flutter UI, view models, and feature flag so the
unified-create flow is reachable from the home-screen `+` button when
`ENABLE_UNIFIED_CREATE=true`. After merge, internal users can flip the
flag and use the full flow.

**Files (29):**

Feature flag:

- `app/lib/core/config/environment.dart` (+8)
- `app/env.dev.json` (+5)
- `app/env.local.json` (+5)
- `app/env.patrol.json` (+5)
- `app/env.prod.json` (+5)

ARB strings (en + es):

- `app/lib/l10n/app_en.arb`
- `app/lib/l10n/app_es.arb`

API client:

- `app/lib/services/unified_create_service.dart`
- `app/lib/services/providers/unified_create_providers.dart`
- `app/lib/services/providers/location_providers.dart` (adds
  `locationDisplayProvider` for the preview-card location row)

View models:

- `app/lib/presentation/viewmodels/unified_create_state.dart`
- `app/lib/presentation/viewmodels/unified_create_view_model.dart`
- `app/lib/presentation/viewmodels/unified_create_streaming_actions.dart`
- `app/lib/presentation/viewmodels/unified_create_save_actions.dart`

UI:

- `app/lib/presentation/screens/create/unified_create_modal.dart`
- `app/lib/presentation/screens/create/unified_create_input_drawer.dart`
- `app/lib/presentation/widgets/create/unified_preview_card.dart`
- `app/lib/presentation/widgets/create/unified_type_selector.dart`
- `app/lib/presentation/widgets/create/unified_lend_give_toggle.dart`
- `app/lib/presentation/widgets/create/unified_item_details_sheet.dart`
- `app/lib/presentation/widgets/create/unified_building_pill.dart`
- `app/lib/presentation/widgets/create/unified_primary_button.dart`

Home-screen hookup + post-create flow:

- `app/lib/presentation/screens/home/home_screen.dart` (await modal,
  call `handlePostCreation` after pop)
- `app/lib/presentation/viewmodels/feed_view_model.dart` (drop the
  per-item provider invalidation in `refreshInPlace` that left the new
  top-of-feed item stuck on a spinner)

Tests:

- `app/test/presentation/viewmodels/unified_create_view_model_test.dart`
- `app/test/presentation/screens/create/unified_create_input_drawer_test.dart`
- `app/test/presentation/widgets/create/unified_preview_card_test.dart`
- `app/test/presentation/widgets/create/unified_type_selector_test.dart`
- `app/test/presentation/widgets/create/unified_item_details_sheet_test.dart`

**Tasks:**

- [ ] Branch `unified-create-client` from `unified-create-server` (or
      from `main` once both prior PRs are merged — same outcome).
- [ ] `git checkout unified-create-phase-1 -- <each file above>`.
- [ ] `npm run generate` to confirm the Dart-side generated code wires
      up (uses the proto from PR 1).
- [ ] `cd app && flutter analyze && flutter test`.
- [ ] `npm run lint:dart:size` (the file-size gate — all unified-create
      files are under the 1000-line cap; this is a safety net).
- [ ] `npm run lint:ephemeral-comments` (no Phase 1 / P1.x markers).
- [ ] `npm run lint:dart:a11y` (allowlist must stay flat or shrink) per
      `docs/client/accessibility.md`. Confirm:
  - All taps go through `Tappable` (no raw `GestureDetector` /
    `InkWell`); icon-only buttons use `IconAction` not `IconButton`.
  - The unified modal is opened via `showAccessibleModal`, not raw
    `showModalBottomSheet`.
  - All animation durations use `accessibleDuration(context, ...)`.
  - All `semanticsLabel:` / `tooltip:` values come from
    `context.l10n.a11y*` keys, never raw strings.
- [ ] `npm run lint:dart:i18n` (no hardcoded user-facing strings).
      Confirm every user-visible string in unified-create widgets uses
      `context.l10n.unifiedCreate*` keys per `docs/client/i18n.md` §
      Best Practices.
- [ ] Confirm both `app_en.arb` AND `app_es.arb` have matching keys
      with `@key` metadata blocks (per `docs/client/i18n.md`: "Each
      new string needs both locales"). Plural keys
      (`unifiedCreateCommunitiesCount`) declare `placeholders` in the
      `@key` block.
- [ ] Confirm `UnifiedCreateViewModel` and `UnifiedCreateState` expose
      typed state (enums, `Detected*` types, error codes) — **no
      localized strings inside the viewmodel** per `docs/client/i18n.md`
      § "Viewmodels never resolve strings". User-visible copy is
      resolved by widgets via `context.l10n` at build time.
- [ ] Confirm ViewModel tests use `ProviderContainer` per
      `docs/client/testing.md` § ViewModel Testing — no `MaterialApp`
      wrapper, no `localizationsDelegates` needed (viewmodel tests don't
      need l10n setup). Widget tests, where present, use
      `app/test/helpers/l10n_helpers.dart`.
- [ ] **Apply in-hindsight improvement #1** (description-hint i18n
      fix): replace the hardcoded English strings in
      `_descriptionHint` (`unified_preview_card.dart`) with
      `context.l10n.unifiedCreateDescriptionHint{Item,Request,Event,
      Default}`. Add the four keys to **both** `app_en.arb` and
      `app_es.arb` with `@key` metadata blocks. Re-run
      `npm run lint:dart:i18n` to confirm clean.
- [ ] **Apply in-hindsight improvement #2** (stale issue marker):
      file a follow-up issue
      `"Migrate unified-create errorMessage: String to typed UserError"`
      (with the 4 site references), then update the 4
      `// dart-error-tostring-allow #1886` markers in
      `unified_create_state.dart:111` and
      `unified_create_view_model.dart:132 / 200 / 294` to reference the
      new issue number. Capture the file:line list in the follow-up
      issue body so the migration is easy to start.
- [ ] Manual smoke test on emulator: flip `ENABLE_UNIFIED_CREATE=true` in
      `env.local.json`, run a text prompt end-to-end through Event / Item
      / Request, confirm Share saves and the entity appears in the feed.
      Run against the merged server PR's code (i.e., local server running
      `main` after server PR landed).
- [ ] Manual smoke test: image tab, URL tab, type-selector flip mid-flow.
- [ ] Commit message: see template below.
- [ ] Open PR `unified-create-client` against `main`. Body references
      `docs/design/unified-create.md` §§ UX overview + Client + Feature
      flag, and closes #1871 (P1.5), #1872 (P1.6), #1873 (P1.7),
      #1874 (P1.8), #1875 (P1.9), #1876 (P1.10), #1877 (P1.11).

**Validates:** A user can flip the flag in `env.local.json`, build and
run the app, and complete a full gear / event / request creation flow
with the new UX. (i.e., the original Phase 1 acceptance criteria.)

**Approx diff size:** ~+2750 / −20 across 29 files.

### Cleanup tasks (post-merge of all three PRs)

- [ ] Close PR #1886 with a comment linking the three replacements.
- [ ] Delete `unified-create-phase-1` branch (after one release cycle).

## Commit-message templates

Each PR is a single commit. Use these as starting points; tighten before
opening.

**Proto:**

```
Add unified_create_service proto (StreamGenUnifiedCreate)

Phase 1 of the unified-create flow (docs/design/unified-create.md):
new RPC service that classifies text/image/URL input into Request/Event/
Item and runs the matching per-type generator. This PR ships only the
wire contract; the server handler lands in unified-create-server and the
client UI in unified-create-client.

Replaces the proto slice of #1886. Closes #1867.
```

**Server:**

```
Add UnifiedCreateService + per-type generator delegation library

Server side of unified-create (docs/design/unified-create.md). Adds:

- server/services/unified_create — RPC handler that classifies the
  input (Anthropic-backed UnifiedCreateClassifier), then delegates to
  the matching per-type FromText/FromImage/FromWebpage streaming
  generator and forwards events through the unified envelope.
- server/genai/{experiencegen,geargen,requestgen} — shared library
  package extracted from the per-type services so unified_create can
  consume them without crossing the service-isolation boundary.
- server/ai/prompts_unified.go + classifier_unified.go + eval seeds
  and goldens.
- Per-type streaming generators are now exported (StreamGen*FromText
  etc.) and take a Context + StreamSender so the same code path
  serves both the single-create RPCs and the unified caller.

Behaviour preserved across single-create RPCs: every existing per-type
test passes unchanged. Description-from-prompt convention extended to
gear text mode (gearFromTextOutput has no description field; the
unified-create client gate needs the wire event).

Requires the proto contract from unified-create-proto (now on main).

Replaces the server slice of #1886. Closes #1868, #1869, #1870.
```

**Client:**

```
Add unified-create UI behind ENABLE_UNIFIED_CREATE flag

Client side of unified-create (docs/design/unified-create.md). Adds a
glass-styled bottom-sheet modal with three input modes (text / image /
URL), a streaming preview card that fills in title / description /
location / media / time as the AI emits events, a 3-segment
Request/Event/Item type selector that re-streams with force_type when
flipped, and per-type Share dispatch to the existing SaveGear /
SaveExperience / SubmitRequest RPCs.

Gated by ENABLE_UNIFIED_CREATE (default false in every env). Home-
screen + button branches on the flag; when on, the modal runs;
when off, the existing per-type modals run unchanged.

Includes English + Spanish ARB strings for all user-visible copy and
viewmodel / widget tests covering the reducer, the preview card, the
input drawer, the type selector, and the item-details sheet.

Also drops the per-item provider invalidation in FeedViewModel.
refreshInPlace that left the new top-of-feed item stuck on a loading
spinner after a unified-create share.

Requires the server from unified-create-server (now on main).

Replaces the client slice of #1886. Closes #1871, #1872, #1873, #1874,
#1875, #1876, #1877.
```

## Related Issues

- **#1886** — the PR being replaced by this three-PR stack.
- **#1867 (P1.1)** — proto. Lands in PR 1.
- **#1868 (P1.2), #1869 (P1.3), #1870 (P1.4)** — server. Lands in PR 2.
- **#1871–#1877 (P1.5–P1.11)** — client. Lands in PR 3.
- **Phase 2 issues (#12–#15 in the design doc)** — out of scope for this
  replan; ship after all three PRs land.

## Audit findings (`/ripls-audit`)

The plan was audited against:
`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`,
`docs/server/observability.md`.

Changes made to the plan during audit:

1. **Server PR — explicit observability checklist added.** The previous
   draft delegated this to "log fields are documented in
   `docs/server/observability.md`". The audit replaces that with a
   concrete checklist (operation name, request_id, masked user_email,
   per-stage duration_ms, no raw prompt text, `target_request_id` not
   applicable). Same source-of-truth, but the reviewer now has an
   explicit pass/fail list.
2. **Server PR — interface-injection gate added.** Per
   `docs/server/architecture.md` § Service Independence, services
   inject shared libraries (interfaces), not other services. The
   delegation refactor must inject `experiencegen.Generator` etc.
   interfaces, never concrete `experience.Service` etc. The audit
   added a confirmation step so the reviewer can verify the
   constructor signature.
3. **Server PR — `connecterr.Internal` + `GenStreamError` distinction
   noted.** Per `CLAUDE.md` § Error Handling, raw upstream error text
   must never reach the wire. `scripts/check_codeinternal.js` gates
   `connect.NewError(connect.CodeInternal, err)`. For stream-level
   errors the unified-create flow returns a `GenStreamError` event
   instead — those must carry typed codes (`GEN_STREAM_ERROR_CODE_*`)
   not raw upstream messages. Added as an explicit task.
4. **Client PR — accessibility checklist added.** Per
   `docs/client/accessibility.md`, every interactive widget composes
   from `widgets/accessibility/` primitives (`Tappable`, `IconAction`,
   `showAccessibleModal`, `accessibleDuration`) and every label comes
   from `context.l10n.a11y*`. The `lint:dart:a11y` allowlist is
   currently **empty** and must stay flat or shrink. Added explicit
   confirmation steps.
5. **Client PR — i18n constraints made explicit.** Per
   `docs/client/i18n.md` § "Viewmodels never resolve strings":
   `UnifiedCreateViewModel` and `UnifiedCreateState` expose only typed
   state (enums, `DetectedContentType`, error codes); widgets resolve
   to localized text via `context.l10n` in `build()`. Both `app_en.arb`
   and `app_es.arb` must ship matching keys. Added as explicit tasks.
6. **Client PR — testing convention pinned to ViewModel-first.** Per
   `docs/client/testing.md`: ViewModel tests use `ProviderContainer`
   and don't need localization setup; widget tests, when present, use
   `app/test/helpers/l10n_helpers.dart`. Added confirmation that the
   imported tests follow this split.

Concerns surfaced by the audit that the replan **does not** address
(flagged here for follow-up, not blocking the split):

A. **Proto convention edge case.** `CLAUDE.md` says "Never reuse RPC
   request or response message types across multiple RPCs." However,
   `StreamGenUnifiedCreateFinal.final_payload` is a `oneof` containing
   `GenGearResponse`, `GenExperienceResponse`, `GenRequestResponse` —
   the existing per-type Gen responses. The design doc
   (`docs/design/unified-create.md` § RPC contract) explicitly chose
   this reuse so the client can route directly to the matching per-
   type Save RPC without a translation layer. **The replan preserves
   this design decision verbatim**, but proto reviewers should be
   aware: this is an intentional exception to the convention, not an
   oversight, and the rationale lives in the design doc rather than
   in the `.proto` file's comments. Recommendation: add a comment in
   `StreamGenUnifiedCreateFinal` pointing at the design-doc § so the
   reviewer doesn't need to dig.

B. **Save-dispatch calls services directly.** The client architecture
   doc (`docs/client/architecture.md` § "Repositories are the ONLY
   layer that calls Services") says ViewModels access data through
   repositories. `unified_create_save_actions.dart` reads
   `experienceServiceProvider`, `gearServiceProvider`,
   `communityServiceProvider`, `requestServiceProvider` directly.
   **This mirrors the pre-existing pattern** in the per-type single-
   create modals (`experience_preview_modal.dart`,
   `gear_preview_modal.dart`, `request_preview_modal.dart`), which
   also call services directly via providers. **The replan does not
   fix this** — fixing it would require either landing
   `ExperienceRepository.create*`, `GearRepository.create*`,
   `RequestRepository.submit*` repository methods (none exist today,
   only read methods) or accepting a one-off departure here. Should
   be tracked as a separate architectural cleanup across the entire
   create surface, not lumped into the unified-create replan.

C. **No client-side caching invariants apply.** Unified-create is a
   write-path flow (Save + Share) followed by feed refresh. The
   feed-refresh path uses `refreshInPlace` which still goes through
   the appropriate repositories (the `feed_view_model.dart` change in
   PR 3 only drops a per-item-provider invalidation loop, not any
   repository call). No caching-convention violations introduced.
