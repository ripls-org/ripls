# Issue #1260: Remove unary gen path and ENABLE_GEN_STREAMING flag (#1157 cleanup)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1260
**Filed:** 2026-04-24 | **Last activity:** 2026-05-01
**Labels:** due:2026-04-30 | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

Streaming gen shipped to dev and prod (PR #1259, flag flip in `app/env.dev.json`
and `app/env.prod.json` on 2026-04-24). The `ENABLE_GEN_STREAMING` dart-define
was scaffolded as a kill switch for "one release cycle of dual-path before
unary is deprecated" — see the docstring at
`app/lib/core/config/environment.dart:58-60`. The one-week production soak
ended **2026-04-30** (a day before this plan); time to delete the flag, both
branches of every conditional, and the now-dead unary client code so the
streaming path is the only path.

The issue body lists the three creation-modal call sites the reporter
remembered (Experience, Gear, Request). A grep across `app/lib/` shows a
**fourth** call site — Community creation
(`community_creation_modal.dart:78`) — that the body missed. The exit
criterion ("grep returns empty for `ENABLE_GEN_STREAMING` /
`genStreamingEnabled`") still applies, so this plan covers all four modals.

Scope is **client-only**. Server-side `GenExperience` / `GenGear` /
`GenRequest` / `GenCommunity` unary RPCs and their proto definitions stay
where they are — older app builds in the field still reach for them, and
deleting wire-level RPCs is a different conversation than collapsing a
client kill-switch.

## Context from Comments

- **2026-04-24 — maintainer (`/ripls-triage`):** Triaged as `Task` /
  `P2`. Confirmed as scheduled tech-debt cleanup with a hard
  `due:2026-04-30` label. Engineering-lens, not user-facing.
- **2026-05-01 — briangbrown (assignee):** Posted `@claude create plan`
  the day after the soak window closed. The scope as written in the issue
  body is the contract — write the plan against it, do not implement.

No disagreements, no clarifying questions outstanding. The reporter's
ranked options ("single PR" preferred over "two PRs" or "leave as kill
switch") are sound and adopted below.

## Potential Solutions

### Option A: Single cleanup PR (recommended)
- **Approach:** One PR collapses every `if (Environment.genStreamingEnabled)`
  branch to its streaming arm, drops the flag from `Environment` and the
  three env JSONs, and deletes every viewmodel / repository / service
  method whose only callers were the deleted unary branches. Server-side
  protos and RPC handlers stay untouched.
- **Pros:** One reviewable diff. Clear "does the streaming path still work
  end-to-end after this?" gate. Easy to revert by reverting one commit if
  streaming regresses (the flag was already off in code paths nobody is
  exercising — there's no production rollback dimension).
- **Cons:** Diff is wide (~5 modal files, ~5 viewmodels, ~4 repos, ~4
  service files, ~6 test files). Reviewer has to trust the dead-code
  trace.
- **Effort:** Medium.

### Option B: Two PRs — flag removal first, dead-code sweep second
- **Approach:** PR1 removes the flag and collapses every conditional to
  its streaming branch (the `else` branches stay momentarily as dead
  code). PR2 deletes the now-unreachable methods and tests.
- **Pros:** Each diff is narrower and easier to review. Bisect on PR1
  cleanly identifies any "streaming wasn't actually equivalent" bug.
- **Cons:** Doubles the review and merge overhead for a low-risk
  cleanup. PR2 is mechanically derivable from PR1, so splitting them
  manufactures churn rather than insight. Issue body explicitly calls
  out that splitting is "only worth it if the dead-code surface turns
  out to be large (multiple service methods, shared helpers)" — and it
  does, but mostly across files that mirror each other 1:1, so the
  reviewer's burden is "spot-check four parallel deletions" not
  "reason about novel changes."
- **Effort:** Medium + tax.

### Option C: Leave the flag in as a long-lived kill switch
- **Approach:** Do nothing. Flip the default to `true` and keep both code
  paths around indefinitely.
- **Pros:** Zero engineering work today.
- **Cons:** Explicitly rejected in the issue body — the docstring at
  `environment.dart:58-60` calls out one release cycle, the soak window
  closed 2026-04-30, and a stale flag is a maintenance tax with no one
  operating it. Every future contributor has to re-derive whether the
  unary path is still valid; tests have to keep covering it; the proto
  conventions doc keeps drifting from reality.
- **Effort:** None upfront, ongoing tax forever.

## Recommendation

**Option A — single cleanup PR.** Matches the issue body's preferred
option, matches the human reviewer's ranking, and is the right call
when both PRs in Option B would be small mechanical mirror images of
each other. If the reviewer wants to bisect, the diff is partitioned
cleanly by file, so a hand-rolled split is still possible at review
time without committing to it upfront.

## Open Questions

- **Are server-side unary `GenExperience` / `GenGear` / `GenRequest` /
  `GenCommunity` RPCs in scope?** — Recommendation: **no, leave them.**
  The issue body specifically scopes "client unary call sites" and
  "client service-client methods that no caller hits." Older app builds
  in the field still call these RPCs; collapsing the wire is a separate
  decision with App Store soak implications, not a code-cleanup
  decision. If the reviewer wants a follow-up issue to track
  server-side removal once the oldest in-the-wild client drops below
  the streaming-required version, that's worth filing — but it is not
  this PR.

- **Should `GenGearNotifier.retry()` (currently dead, no callers) be
  deleted as part of this sweep?** — Recommendation: **yes, delete it
  along with `generateGear()` and `uploadImage`'s `autoGenerate`
  parameter.** `retry()` reaches the dead `generateGear()` /
  `uploadImage()` paths; once those are gone, `retry()` references
  nothing real. Deleting it is one extra mechanical step in the same
  diff and avoids leaving an orphan that compiles only because dead
  code calls dead code. (If the reviewer prefers minimal scope,
  preserving `retry()` would force us to keep `generateGear()` and the
  `autoGenerate` parameter — which contradicts the exit-criteria
  language about removing dead unary code.)

- **What about `GenRequestNotifier.generate()`
  (`gen_request_view_model.dart:170`)?** — This method was already
  dead before this issue (no callers — its only consumers were
  removed during the streaming rollout). It still calls
  `_requestRepository.genRequest()`, so it transitively holds that
  repository method alive. Recommendation: **delete it.** Same
  rationale as `retry()` — leaving dead code that calls dead code
  defeats the cleanup.

- **Docs treatment for `docs/evals/1157-genexperience-latency.md`?** —
  Recommendation: **leave the body as-is, append a one-line "Status"
  note at the top marking the flag retired in #1260.** It's an eval
  retrospective; preserving the historical mention of
  `ENABLE_GEN_STREAMING` is the point. Keep references intact and
  stamp the doc as historical. Do **not** rewrite the body to
  past-tense the comparison — that erases the snapshot.

## Implementation Plan

### Phase 1: Collapse modal conditionals and drop the flag
**Goal:** Streaming path is the only path; flag and dart-define are
gone; app still builds and runs.
**Tasks:**
- [ ] `app/lib/core/config/environment.dart` — delete the
  `genStreamingEnabled` getter and its docstring (lines 56-67).
- [ ] `app/lib/presentation/screens/experience/experience_creation_modal.dart`
  — replace the `if (Environment.genStreamingEnabled) { ... } else
  { ... }` at line 111-115 with the inlined body of
  `_handleGenerateStreaming()`. Delete the `_handleGenerateUnary()`
  method (line 161-228) and the unused
  `package:ripls/core/config/environment.dart` import if it has no
  other uses in the file.
- [ ] `app/lib/presentation/screens/gear/create_gear_modal.dart` —
  collapse both flag conditionals (lines 83 and 124). The text-mode
  branch keeps the streaming arm; the image-mode branch keeps the
  upload-then-stream arm. Delete the legacy `await
  viewModel.generateGearFromText()` and `await viewModel.uploadImage()`
  fall-through calls and the post-fall-through preview block (lines
  152-178). Drop the now-unused environment import.
- [ ] `app/lib/presentation/screens/request/request_creation_modal.dart`
  — replace the `if (Environment.genStreamingEnabled) { ... }` at
  line 117 (and the trailing legacy unary block at 146-205) with the
  streaming arm only. Drop the environment import; the `_log` field
  is only used by the legacy block, delete it too.
- [ ] `app/lib/presentation/screens/communities/community_creation_modal.dart`
  — collapse the conditional at line 78. Streaming arm stays; legacy
  unary block (line 103-123) goes. Drop the environment import.
- [ ] `app/env.dev.json` — remove the `"ENABLE_GEN_STREAMING": "true"`
  entry.
- [ ] `app/env.prod.json` — same.
- [ ] `app/env.local.json` — same. (`app/env.patrol.json` already
  doesn't contain the key.)
- [ ] `flutter analyze` clean. App boots in dev/local; one creation
  modal opens and shows streaming preview to confirm wiring before
  proceeding to phase 2.
**Validates:** App still works with the streaming path as the only
path, and no `ENABLE_GEN_STREAMING` / `genStreamingEnabled` references
remain in production code.

### Phase 2: Delete now-unreachable viewmodel, repository, and service methods
**Goal:** Drop the dead client-side surface so future contributors
don't see two paths and wonder which one is canonical.
**Tasks:**
- [ ] `app/lib/presentation/viewmodels/experience_creation_view_model.dart`
  — delete `Future<GenExperienceResponse?> generateExperience()`
  (line 150-215).
- [ ] `app/lib/presentation/viewmodels/request_creation_view_model.dart`
  — delete `Future<GenRequestResponse?> generateRequest()` (line
  118-193). Drop the `_log` import if unused after phase 1's modal
  cleanup left it dangling.
- [ ] `app/lib/presentation/viewmodels/community_creation_view_model.dart`
  — delete `Future<GenCommunityResponse?> generateCommunity()` (line
  119-157). Update the comment block at line 159-167 ("Image mode
  preserves the unary-path sham" — the unary path no longer exists,
  so just describe the streaming behavior).
- [ ] `app/lib/presentation/viewmodels/gen_gear_view_model.dart` —
  delete `Future<void> generateGearFromText()` (line 310-364), delete
  `Future<void> generateGear()` (line 367-403), delete the
  `autoGenerate` parameter on `uploadImage()` and the
  `if (autoGenerate) await generateGear();` branch inside it (line
  300-302). Delete `Future<void> retry()` (line 666-687) — it has no
  UI callers and its `uploading` / `generating` cases referenced the
  methods we just deleted. Update the `uploadImage()` doc comment to
  drop the `autoGenerate` paragraph.
- [ ] `app/lib/presentation/viewmodels/gen_request_view_model.dart` —
  delete `Future<void> generate(String prompt, String communityId)`
  (line 170 onward through the catch block) — already dead pre-#1260,
  see the second open question above.
- [ ] `app/lib/data/repositories/experience_repository.dart` — delete
  `Future<GenExperienceResponse> genExperience({...})` (line 445-472).
- [ ] `app/lib/data/repositories/request_repository.dart` — delete
  `Future<GenRequestResponse> genRequest({...})` (line 165-) and
  `Future<GenRequestResponse> genRequestFromMedia({...})` (line
  215-).
- [ ] `app/lib/data/repositories/community_repository.dart` — delete
  `Future<GenCommunityResponse> genCommunity({...})` (line 79-101).
  Keep `streamGenCommunity()`. Update the streaming doc comment to
  drop the "mirrors the unary genCommunity sham" reference, since
  there is no longer a unary genCommunity to mirror — describe the
  image-mode generic-prompt behavior in its own terms.
- [ ] `app/lib/data/repositories/gear_repository.dart` — delete
  `Future<GenGearResponse> genGear({...})` (line 198-210) and
  `Future<DetectedGearItem?> genGearFromMedia(String mediaId)` (line
  241-243).
- [ ] `app/lib/services/experience/gen_methods.dart` — delete
  `Future<GenExperienceResponse> genExperience({...})` (line 16-62).
  Keep `streamGenExperience`, `convertInformalTime`,
  `generateCompletionSummary`. Update the part-of file's leading
  comment to drop `genExperience` from the listed RPCs.
- [ ] `app/lib/services/request_service.dart` — delete `genRequest`
  (line 61-) and `genRequestFromMedia` (line 129-). Keep streaming
  variants.
- [ ] `app/lib/services/community_service.dart` — delete
  `genCommunity` (line 110-).
- [ ] `app/lib/services/gear_service.dart` — delete `genGear` (line
  157-181) and `genGearFromMedia` (line 218-234). Keep
  `streamGenGear` and `detectGearFromURL`.

**Note on observability:** The `gen_probe flow=... mode=unary
metric=complete` log lines inside the deleted unary methods
disappear along with their methods. The corresponding `mode=streaming`
probes in the streaming methods are untouched, so the eval doc's
streaming-path measurements stay valid going forward. No probe
schema or analytics-event changes are required.

**Validates:** `npm run build` (Go is unaffected, but the script
wraps everything) and `cd app && flutter analyze` are clean. No
public client surface still references the unary path.

### Phase 3: Update tests and regenerate mocks
**Goal:** Test suite reflects the collapsed surface.
**Tasks:**
- [ ] `app/test/presentation/viewmodels/experience_creation_view_model_test.dart`
  — delete the `group('generateExperience', ...)` block (line
  312-onward through the streaming group). Confirm the
  `generateExperienceStreaming` and disposal-safety groups still
  pass.
- [ ] `app/test/presentation/viewmodels/request_creation_view_model_test.dart`
  — delete `group('generateRequest', ...)` (line 117-178) and the
  disposal-safety test that calls `notifier.generateRequest()` (line
  237). Keep the `generateRequestStreaming` group.
- [ ] `app/test/presentation/viewmodels/community_creation_view_model_test.dart`
  — delete `group('generateCommunity', ...)` (line 200-365). Update
  the `setInputMode` / `setTextInput` / `setImagePath` / `clearImage`
  tests that currently call `notifier.generateCommunity()` to set
  state into an error-bearing shape (lines 80, 105, 128, 151) — port
  them to `generateCommunityStreaming()` (returns `bool`, sets the
  same error fields on `canGenerate() == false`) so they continue to
  exercise the same error-clearing semantics.
- [ ] `app/test/presentation/viewmodels/gen_gear_view_model_test.dart`
  — delete `group('uploadImage', ...)` (line 184-310) and the
  `generateGearFromText` tests in the
  `'sourceUrl and valueEstimate handling'` group (lines 520-672 —
  those that hit `generateGearFromText`/`genGearFromMedia`). Keep
  the streaming-image / state / error-reset / reset / manual-creation
  / getMediaUrl groups. The `autoGenerate=false` test at line 291
  loses its `autoGenerate` argument; the call collapses to plain
  `uploadImage()`.
- [ ] `app/test/presentation/viewmodels/gen_request_view_model_test.dart`
  — sanity check; `GenRequestNotifier.generate()` had no test
  coverage so deleting it should be a no-op here.
- [ ] `app/test/data/repositories/experience_repository_test.dart` —
  delete `genExperience` test group if present.
- [ ] `app/test/data/repositories/request_repository_test.dart` —
  delete `genRequest` and `genRequestFromMedia` test groups.
- [ ] `app/test/data/repositories/gear_repository_test.dart` — delete
  `group('genGear', ...)` (line 445) and
  `group('genGearFromMedia', ...)` (line 658).
- [ ] Regenerate mocks: `cd app && flutter pub run build_runner
  build --delete-conflicting-outputs` (the form documented in
  `CLAUDE.md` and `docs/client/testing.md`). Mockito will rebuild
  `*.mocks.dart` files without the deleted methods. Diff should
  show the corresponding `genExperience` / `genGear` / `genRequest`
  / `genCommunity` mock methods disappearing across every
  `.mocks.dart` that previously referenced them
  (`experience_creation_view_model_test.mocks.dart`,
  `gear_repository_test.mocks.dart`, etc.).
- [ ] `cd app && flutter test --reporter expanded` is green.
**Validates:** Test suite covers only paths that exist; mock surface
matches real surface; no orphan test references to deleted methods.

### Phase 4: Doc updates
**Goal:** Internal docs reflect the new reality without erasing the
historical eval comparison.
**Tasks:**
- [ ] `docs/evals/1157-genexperience-latency.md` — append a one-line
  "**Status (2026-05+):** `ENABLE_GEN_STREAMING` retired in #1260;
  streaming is now the only path." note near the top of the doc
  (immediately under the heading or under the Environment section).
  Leave the rest of the body alone — the unary baseline numbers are
  the historical reference point and should stay.
- [ ] `docs/issues/1229-streaming-image-webpage-genai.md` — its
  follow-up checklist references this issue (line 714) and #1260's
  flag-removal sentence (line 735); update the checklist item to
  note completion (or leave for the #1229 owner to check off as part
  of their own PR — the simpler call). Recommendation: **leave the
  checklist item alone** — it's #1229's todo, not ours, and ticking
  someone else's box is rude. Owner of #1229 will see #1260
  closed and tick it themselves.
- [ ] `docs/issues/1539-image-streaming.md` and
  `docs/issues/1540-community-streaming.md` reference
  `Environment.genStreamingEnabled` as a precondition assumption.
  Recommendation: **leave them alone** — those plans were written
  while the flag was live, and editing them would rewrite history.
  When their implementation lands the flag will already be gone, so
  the implementer can drop the conditional then. Mark this in the
  PR description so reviewers don't expect those files to change.
**Validates:** Final exit criterion satisfied — `grep -r
"ENABLE_GEN_STREAMING\|genStreamingEnabled"
app/lib app/test docs/evals` returns empty. Hits in `docs/issues/`
plan files are intentionally preserved as historical record.

## Audit notes (`/ripls-audit`, 2026-05-01)

Reviewed against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/testing.md`,
`docs/client/accessibility.md`, `docs/client/observability.md`.
Two small fixes applied in-place:

- **Phase 3 build_runner invocation** rewritten from
  `dart run build_runner build` to `flutter pub run build_runner
  build` to match the canonical form in `CLAUDE.md` and
  `docs/client/testing.md`.
- **Phase 2 observability note** added to make the disappearance of
  the `gen_probe ... mode=unary` log lines explicit (they go away
  with the deleted methods; streaming probes are untouched), so a
  reviewer scanning for "did you forget to migrate the
  observability hooks?" gets a direct answer instead of having to
  trace the deletion.

No other architectural drift found. The plan preserves the
viewmodel → repository → service direction, keeps gen RPCs
correctly cache-bypass (the streaming path is also cache-bypass —
unchanged), touches no accessibility primitive, and follows the
ViewModel-first / `ProviderContainer` + `@GenerateMocks` test
convention from `docs/client/testing.md`.

## Related Issues

- **#1157** — Original streaming gen design issue (parent). #1260 is
  the cleanup checkpoint for #1157's "one release cycle of dual-path"
  promise.
- **#1259** — PR that flipped `ENABLE_GEN_STREAMING=true` in dev/prod
  on 2026-04-24 and started the soak window.
- **#1229** — Streaming image / webpage / genAI tracking issue. Its
  internal checklist references #1260; close-out propagates.
- **#1539** / **#1540** — In-flight follow-ups for image-mode
  streaming (gear/request) and community streaming. Both reference
  `Environment.genStreamingEnabled` in their plans; whichever lands
  after #1260 will need a small rebase to drop the flag check from
  whatever conditional logic they were planning.
