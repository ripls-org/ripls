# Issue #1513: [code_quality] Phase-number references in production source comments regressing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1513
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** none (the review report at `docs/reviews/reports/code_quality_review.md` is the originating source; CLAUDE.md "Go Code Comments" is the durable policy)

> **Audit notes (2026-04-30):**
> 1. The `app/patrol_test/` tree is now explicitly excluded from
>    the lint and from the Phase 2 cleanup checklist — it's E2E
>    test code (the issue says "outside test files").
> 2. The original draft proposed a temporary `--baseline` /
>    `--report-only` mode so Phase 1 and Phase 2 could land in
>    separate PRs. That conflicted with the issue's "one cleanup
>    PR" mandate. The plan now lands the lint in its final
>    strict form on commit 1 and clears violations across
>    subsequent commits on the same branch.
> 3. The lint script itself is exempt from its own rule (its
>    header docs may legitimately describe the term "Phase").

## Problem Statement

CLAUDE.md ("Go Code Comments") forbids ephemeral context in source comments —
phase markers ("Phase 3a"), issue/PR numbers ("#1157"), and similar
"new in this PR" / "pre-refactor" hooks rot the moment the surrounding
work ships and leave readers chasing dead links.

A grep of the tree (excluding `_test.*`, `gen/`, `*.g.dart`, and
`*.freezed.dart`) finds **125** comment lines containing `Phase\s*\d+`
across **52** Go and Dart files, plus **~16** comment lines containing
standalone `#NNN` / `#NNNN` issue references. The previous review
(2026-04-24) counted ~25 phase markers; the count has grown ~5x since
then. Recent multi-phase efforts — accessibility, impact metrics,
provenance, the webpage-streaming rollout, the time-modal redesign,
the loan/giveaway state-machine modals — all leaked phase markers
into durable code.

The violations split into two genuinely different categories:

1. **Ephemeral migration markers** (the pure-rot case the policy
   targets). Examples: `// Phase 2: infer social attributes via
   LLM after insert` (`server/services/experience/save.go:118`);
   the seven `TODO(phase1):` entries in
   `server/services/leaderboard/leaderboard.go`; `// Phase 4+
   registry populator must…` (`server/undo/registry.go:114`); `//
   In Phase 5 a Connect-aware variant…` (`server/metrics/metrics.go:37`).
   These reference rollout phases of a specific work plan and have
   no meaning to a future reader.

2. **Domain "phase" terminology that happens to be numbered.** The
   loan, giveaway, request, and event modals each model their UX
   as a `*Phase` enum — `LoanBorrowerPhase`, `LoanOwnerPhase`,
   `GiveawayGiverPhase`, `GiveawayReceiverPhase`,
   `RequestOwnerPhase`, `RequestHelperPhase`. Their dartdoc
   comments use the form `/// Phase 0: Set date, time, duration`
   to document each enum value's position in the state machine.
   This is not migration ephemera — it describes a durable state
   machine — but it still trips the simple `Phase\s*\d+` regex.
   The issue's own recommendation anticipates this: "Allowlist legit
   algorithmic phases ('Step 1/Step 2') via rename."

A simple all-or-nothing lint would either (a) miss the migration
ephemera that spawned this issue or (b) force every domain `Phase N`
docstring to be reworded. The plan therefore needs to make a
deliberate choice about scope, regex shape, and rename strategy
*before* one cleanup PR is feasible.

## Context from Comments

Two comments. The triage pass on 2026-04-28 (maintainer) set
**Type: Task / Priority: P2** with the rationale "code_quality High
durable: phase-number references in production source comments
regressing (CLAUDE.md violation)." A subsequent `@claude` comment
on 2026-04-30 (also maintainer) is the trigger that produced
this run; no scope-narrowing or counter-proposal.

The recommendation in the issue body itself is the load-bearing
guidance:

> add a CI lint that fails on `Phase\s*[0-9]+` and standalone
> `#[0-9]{3,4}` in `*.go`/`*.dart` comments outside test files.
> Allowlist legit algorithmic phases ("Step 1/Step 2") via rename.
> Land one cleanup PR clearing the 106 existing sites.

That's the contract this plan fills out.

## Potential Solutions

### Option A: Strict lint + rename "Phase N" → "Step N" in domain enums, no allowlist mechanism

- **Approach:** Add `scripts/check_ephemeral_comments.js` that
  walks `*.go` and `*.dart` files (excluding tests, generated,
  l10n) and fails on `\bPhase\s*\d+\b` or standalone
  `\B#\d{3,4}\b` *in comment lines*. No exemption flag — every
  hit must be reworded. Wire into `npm run lint` via a new
  `lint:ephemeral-comments` sub-script. In the same PR, rewrite
  all 125 phase hits and ~16 hash hits:
    - Migration ephemera → delete the phase reference entirely
      (e.g. `// Phase 2: infer social attributes via LLM after
      insert so the stored estimate is experience-specific from
      the start.` → `// Infer social attributes via LLM after
      insert so the stored estimate is experience-specific from
      the start.`)
    - Domain-state phase comments → reword from "Phase N: …" to
      "Step N: …" (matches issue recommendation). The enum
      *type* names (`LoanBorrowerPhase`, etc.) stay — only the
      docstrings change. A `// Phase 0: Set date, time, duration`
      becomes `// Step 0: Set date, time, duration`. The literal
      numeral stays so cross-references in `event_modal_view_model.dart`
      ("Organizer: phase 2 (complete), Joiner: phase 1 (complete)")
      can be reworded as "Organizer: step 2 (complete), Joiner:
      step 1 (complete)" without renumbering anything.
    - `TODO(phase1):` → `TODO:` (drop the rollout marker; the
      durable signal is "this is unfinished").
    - Hash refs in comments → either delete (when the
      cross-reference adds nothing) or rephrase to describe the
      durable invariant (e.g. `// (issue #1185)` → `// (the
      cold-start race that motivated splitting it.)`). Each gets
      a per-site judgment in the PR.
- **Pros:** Bright-line rule. No allowlist mechanism to curate
  over time. Reviewer reads the lint output and immediately
  knows it's a real violation. The "Step" rename is a one-time
  global find-and-replace within domain files; it doesn't
  surface in the UI (these are dartdoc comments inside enum
  declarations).
- **Cons:** Forces ~50 domain-phase comments to be rewritten
  even though they document a durable state machine. "Step" is
  marginally less natural than "Phase" for UX flow modeling
  (other Flutter apps in the wild use both freely). The lint's
  comment-aware regex needs to handle `//`, `///`, `/* */`, and
  Go block comments — plus exclude string literals that contain
  `#1234` (rare but possible in test-data fixtures, log
  templates, etc.).
- **Effort:** Large. ~141 site rewrites across 52 files + lint
  script + tests. Most of the work is mechanical.

### Option B: Strict lint + inline escape-hatch comment for legit domain phases

- **Approach:** Same lint as Option A, but with an inline
  escape-hatch comment marker following the precedent of
  `scripts/check_dart_file_size.sh`'s `// dart-line-count-allow`.
  Files containing `// phase-ref-allow` (placed near the top
  with a short justification) skip the phase regex entirely.
  Migration ephemera still get cleaned up; the seven `*Phase`
  enum files each get a one-line `// phase-ref-allow: domain
  state-machine phase, see <type-name>` and keep their existing
  docstrings.
- **Pros:** Smaller cleanup scope (skip the ~50 domain-phase
  rewrites). Domain files keep the more-natural "Phase" wording.
  Future contributors can opt-in for genuinely-phase-named
  algorithms without a rename.
- **Cons:** Allowlist-by-marker is leaky — once a file has the
  marker, *every* phase reference in it is exempt, so future
  drift can sneak in. It also blesses the term "Phase" as the
  domain word, which then leaks into every new modal (gear,
  contributions, future flows) just to fit the convention.
  Marker drift was the failure mode of `dart-line-count-allow`
  in the first six months; the issue explicitly cites the
  growth-since-last-review as the reason for the lint.
- **Effort:** Medium. ~91 ephemeral-marker rewrites + ~7 marker
  insertions + lint script + tests. Less disruptive but defers
  the "domain term" choice indefinitely.

### Option C: Ratchet allowlist (mirror the `check_a11y.js` pattern), defer rename decision

- **Approach:** Add `scripts/check_ephemeral_comments.js`
  modelled on `scripts/check_a11y.js`'s ratchet: an
  `ephemeral_comments_allowlist.txt` lists every currently-violating
  file with its baseline count; CI fails on (a) any new
  non-allowlisted file with the pattern or (b) any allowlisted
  file's count exceeding its baseline. No bulk rewrite in this
  PR. The rename decision is deferred to whoever shrinks the
  allowlist next.
- **Pros:** Stops the bleeding immediately with one PR that
  ships a script + a frozen allowlist. Zero
  comment-rewriting risk in the gating PR. Each subsequent PR
  shrinks the allowlist as files are touched anyway.
- **Cons:** Doesn't resolve the issue ("Land one cleanup PR
  clearing the 106 existing sites" — explicit in the
  recommendation). Allowlist baking captures the current 125
  violations as accepted; in practice this means they linger
  for months. Defers the domain-rename judgment, leaving
  inconsistency in the tree.
- **Effort:** Small. Lint script + baseline file + CI wiring.
  No comment rewrites. Future PRs do the rewrites
  incrementally.

## Recommendation

**Option A.** The issue's recommendation is explicit: ratchet
allowlists are not the request. The ask is *one* cleanup PR plus a
strict gate. The "Step N" rename is a small wording change confined
to enum-value docstrings — none of these strings reach the UI (the
UI uses `progressSteps: ['Arrange', 'Confirmed', 'In Use', 'Return',
'Complete']` already), so the rename is safe and mechanical. A
strict gate without an allowlist also follows the precedent of
`scripts/check_codeinternal.js` which ships exemption-free with a
"the migration is complete, any new violation is a regression"
header.

The bright-line rule is the right tradeoff for this codebase: the
domain-phase enums are a finite, enumerable set (six modal
viewmodels) and the rename touches dartdoc only. The cost of
keeping `Phase` as a blessed domain term is that every future
modal has to either (a) inherit the term or (b) add an
escape-hatch — both of which were the trajectory the original
review flagged.

## Open Questions

- **Should the lint cover `*.go` and `*.dart` only, or also `*.js`,
  `*.sh`, `*.md`, `*.yaml`, etc.?** — Recommendation: `*.go` and
  `*.dart` only, per the issue body. `scripts/check_a11y.js` itself
  contains `Phase 2 of docs/ai/accessibility_plan.md` in its
  header and several `Phase 1 added the parameter` comments;
  bringing JS into scope would force a rewrite of unrelated
  scripting infrastructure that's deliberately allowed to track
  rollout phases (it's tooling-internal documentation, not source).
  Markdown under `docs/` is canonically allowed to discuss phases
  (CLAUDE.md says so explicitly: *"That context belongs in the
  commit message, PR description, or plan/eval docs under
  `docs/`"*). Stay narrow.

- **What counts as a "comment"?** — Recommendation: anything that
  the language tokenizer would classify as comment. Rather than
  shipping a tokenizer in a node script, approximate via line
  shape: a Go line is a comment if it matches `^\s*//`, `^\s*/\*`,
  `^\s*\*\s` (continuation), or sits inside a `/* … */` block;
  a Dart line additionally counts `^\s*///`. **Do not** match
  inside string literals — handle the common cases by skipping
  any line whose phase/hash match is enclosed by an odd number
  of unescaped `"` characters before it. False positives in this
  edge case are acceptable: a violation flagged inside a string
  is rare, and the fix (rewriting the string) is cheap.

- **Should `#1234` matches be limited to comment context, the
  same as `Phase N`, or fail anywhere?** — Recommendation:
  comment context only. String literals legitimately contain `#`
  + digits in test fixtures, simulation data, and (rarely) UI
  copy. The risk of bleed-into-comments is the only thing this
  rule fights.

- **Are CSS/Flutter hex color codes a false-positive risk?** —
  Recommendation: no. The regex `\B#\d{3,4}\b` requires a word
  boundary after 3–4 digits, so `#268080` (6 chars including
  letters or 6 digits) doesn't match. The existing offenders in
  `app/lib/presentation/widgets/impact/community/impact_tab_bar.dart`
  (`#268080`, `#786A50`) were flagged by my initial broad regex
  but are correctly excluded by the bounded form.

- **What happens to forward-looking comments like `Phase 4+
  registry populator must…` in `server/undo/registry.go:114`?**
  — Recommendation: rewrite as a present-tense `TODO`-style
  comment when the work is genuinely outstanding (`// The
  registry populator that ships alongside each Undo* RPC must
  set this; …`). Drop the phase-number forward reference;
  callers don't care which planning iteration the Undo RPC
  ships in.

- **Should the Dart enums (`LoanBorrowerPhase`, etc.) themselves
  be renamed to `LoanBorrowerStep`?** — Recommendation: no. The
  type name is consumed by `LoanBorrowerState.currentPhase`, by
  every viewmodel that switches on these values, and by tests.
  Renaming the type ripples through ~20 files for no semantic
  win — the type name is internal-only Dart, never user-visible.
  Rewrite only the comment text inside each enum declaration;
  leave the type name untouched. The lint is comment-only, so
  `enum LoanBorrowerPhase { … }` doesn't trigger it.

- **What about multi-line comments that *span* a phase reference,
  e.g. `// Phase 1\n// adds X\n// Phase 2 adds Y`?** —
  Recommendation: the lint matches per-line; a multi-line
  comment with three phase markers reports three violations.
  This is correct behaviour — each is a separate ephemeral
  reference.

## Implementation Plan

### One PR, two commit groups

The issue explicitly asks for **one cleanup PR**. Phase 1 and
Phase 2 below are commit groups within the **same** branch and
the **same** PR — not separate PRs. Splitting them is purely a
review-tractability device. There is **no** temporary baseline
file or `--report-only` mode; the lint ships in its final strict
form, and the cleanup commits drive the violation count to zero
before the PR opens for review.

If a reviewer wants to see the lint catching the violations
*before* the cleanup, they can `git checkout` the first commit
on the branch and run `npm run lint:ephemeral-comments` — it
will list all ~141 hits. The final HEAD passes cleanly.

### Phase 1 (commit 1 on the branch): Land the lint script + CI wiring

**Goal:** Ship the gate in its final form. The script will fail
on the existing 125 violations until the subsequent cleanup
commits land. CI on this branch will be red until Phase 2's
commits push the violation count to zero.

**Tasks:**
- [ ] Create `scripts/check_ephemeral_comments.js` modelled on
  `scripts/check_codeinternal.js` (same exemption-free header
  comment: *"any new violation is a regression"*). Walk
  `app/lib/` and `server/` for `*.dart` and `*.go`. Exclusions:
    - any path containing `/gen/`
    - filenames matching `*_test.go`, `*_test.dart`, `*.g.dart`,
      `*.freezed.dart`, `*.mocks.dart`
    - `app/lib/l10n/app_localizations*.dart`
    - the entire `app/patrol_test/` tree (E2E test code,
      including page-object-model helpers like
      `app/patrol_test/pages/conversation_page.dart`)
    - the script itself (`scripts/check_ephemeral_comments.js`)
      — its header docs intentionally describe what it does and
      may include the word "Phase" in plain prose
  Parse line-by-line; classify each line as comment or non-comment
  using the heuristic in Open Questions. Emit one violation per
  matching line of the form `<rel-path>:<line>:<reason> -- <text>`.
  Two reasons: `phase-ref` (matches `\bPhase\s*\d+\b`,
  case-insensitive) and `issue-ref` (matches `\B#\d{3,4}\b`).
  Exit code is 0 only when there are zero violations.
- [ ] Wire into `package.json`:
    - new sub-script: `"lint:ephemeral-comments": "node
      scripts/check_ephemeral_comments.js"`
    - append `&& npm run lint:ephemeral-comments` to the `lint`
      aggregate script (after `lint:nav`)
- [ ] Confirm CI invokes the new sub-script. The repo-wide
  `npm run lint` already runs in `.github/workflows/test_go.yaml`
  and `.github/workflows/test_flutter.yaml` (or whichever job
  invokes the aggregate); appending to the aggregate is enough.
  No new workflow file needed.
- [ ] Add inline self-tests in the script that run against
  fixture strings (no external test runner needed; the script
  exits non-zero if its self-tests fail before walking the
  tree). Cover:
    - matches `// Phase 2: foo` in Go and `/// Phase 2: foo` in Dart
    - matches `// TODO(phase1): bar` (the literal `phase1` inside
      a TODO is the same regex match as freestanding `phase 1`)
    - matches `// see #1185`
    - **does not** match `#268080` (hex color, 6 chars) or
      `#786A50` (mixed digits+letters)
    - **does not** match `phase` without a digit, or `Phase` in
      a non-comment line (e.g. an enum value name like
      `LoanBorrowerPhase`)
    - skips `*.g.dart`, `*_test.go`, and `app/patrol_test/`

**Validates:** Running `npm run lint:ephemeral-comments` at this
commit lists ~141 violations (125 phase-refs + ~16 issue-refs)
and exits non-zero. The script's inline self-tests pass.
Subsequent cleanup commits drive the count down; the PR is
green at HEAD.

### Phase 2 (commits 2–N on the same branch): Bulk cleanup of all ~141 violations

**Goal:** Drive the violation count to zero so the PR's HEAD
passes `npm run lint:ephemeral-comments`.

This is one bulk PR per the issue recommendation. To keep
review tractable, organise the diff into thematic commits even
though they all land in the same PR:

**Tasks (each a separate commit on the branch):**

- [ ] **Domain-phase enum rewrites (Dart).** Files (each
  contains a `*Phase` enum with `/// Phase N: …` docstrings):
    - `app/lib/presentation/viewmodels/loan_borrower_state.dart`
      (5 entries)
    - `app/lib/presentation/viewmodels/loan_owner_state.dart`
      (5 entries)
    - `app/lib/presentation/viewmodels/giveaway_giver_state.dart`
      (3 entries)
    - `app/lib/presentation/viewmodels/giveaway_receiver_state.dart`
      (3 entries)
    - `app/lib/presentation/viewmodels/request_owner_state.dart`
      (5 entries)
    - `app/lib/presentation/viewmodels/request_helper_state.dart`
      (2 entries)
  Replace `/// Phase N: …` → `/// Step N: …` in dartdoc only.
  Type name (`LoanBorrowerPhase`, etc.) untouched. The literal
  numeral N stays as-is so cross-references in modal screens
  don't renumber.

- [ ] **Modal-screen phase-marker comment rewrites (Dart).** Files:
    - `app/lib/presentation/screens/gear/loan_borrower_modal.dart`
      (5 hits)
    - `app/lib/presentation/screens/gear/loan_owner_modal.dart`
      (5 hits)
    - `app/lib/presentation/screens/experience/event_organizer_modal.dart`
      (4 hits)
    - `app/lib/presentation/screens/experience/event_joiner_modal.dart`
      (4 hits)
  Same pattern: `// Phase N: …` → `// Step N: …`.

- [ ] **Modal-viewmodel phase-marker rewrites (Dart).**
    - `app/lib/presentation/viewmodels/event_modal_view_model.dart`
      (5 hits) — update the `_determinePhase` method's body
      comments and dartdoc; the method itself can keep its name
      since it's not in a comment.
    - `app/lib/presentation/viewmodels/time_modal_view_model.dart`
      (8 hits) — these are largely **migration ephemera**
      ("Phase 9: Determine initial mode based on whether time
      is locked"); strip the phase prefix entirely rather than
      renaming, because there is no current "Phase 9" in the
      time-modal feature surface.
    - `app/lib/presentation/viewmodels/time_modal_state.dart`,
      `gen_experience_view_model.dart`,
      `leaderboard_view_model.dart`,
      `gear_metric_view_model.dart`,
      `feed_view_model.dart` — strip migration ephemera.

- [ ] **Other Dart phase-marker rewrites (presentation widgets,
  services, repositories).** Files:
    - `app/lib/core/theme/app_colors.dart` (1 hit, "from Phase 7
      design spec" — strip)
    - `app/lib/core/utils/date_time_formatter.dart` (1 hit — strip)
    - `app/lib/data/repositories/transfer_repository.dart` (1 hit
      — strip)
    - `app/lib/services/transfer_service.dart` (1 hit — strip)
    - `app/lib/presentation/screens/feed/feed_screen.dart` (2 hits — strip)
    - `app/lib/presentation/screens/profile/manage_memberships_screen.dart`
      (3 hits — these reference `Phase 4.4` for unimplemented
      future work; replace with present-tense "placeholder
      implementation; full version not yet shipped")
    - `app/lib/presentation/widgets/accessibility/semantic_announcer.dart`
      (1 hit — strip)
    - `app/lib/presentation/widgets/accessibility/tappable.dart`
      (1 hit — strip)
    - `app/lib/presentation/widgets/chat/message_list.dart`
      (1 hit — strip)
    - `app/lib/presentation/widgets/experience/experience_timeline.dart`
      (1 hit — strip)
    - `app/lib/presentation/widgets/experience/time_slot_card.dart`
      (1 hit — strip)

- [ ] **Server phase-marker rewrites (Go).** Strip ephemera or
  rephrase forward-looking refs. Files (group commit):
    - `server/ai/provider.go` (1 hit), `server/ai/provider_gemini_streaming.go`
      (1 hit) — strip
    - `server/chat/system_messages.go` (3 hits, includes both a
      phase ref and a `#1186` issue ref) — rephrase the
      durable invariant, drop the phase/hash markers
    - `server/impact_metrics/builder.go`,
      `server/impact_metrics/estimator/social.go`,
      `server/impact_metrics/metric_detail_calculator.go`,
      `server/impact_metrics/metric_detail_factors.go` — strip
      "Phase N" prefixes; comments describe what each block
      computes, which is durable
    - `server/jobs/impact_backfill.go` (2 hits) — strip
    - `server/metrics/metrics.go` (1 hit, forward-looking) — strip
    - `server/services/experience/save.go`,
      `server/services/experience/preview.go`,
      `server/services/experience/stats.go`,
      `server/services/experience/gen_ai_streaming.go` (mixes
      phase + hash refs) — strip phase/hash prefixes
    - `server/services/feed/service.go`,
      `server/services/gear/gen_ai_streaming.go`,
      `server/services/impact_metrics/drafting.go`,
      `server/services/impact_metrics/metric_detail.go`,
      `server/services/portfolio/portfolio_metrics.go`,
      `server/services/request/people.go`,
      `server/services/request/stats.go`,
      `server/services/transfer/interest.go` — strip
    - `server/simulation/activity.go`,
      `server/simulation/runner.go` (these are *internal*
      simulation phases, not migration phases) — rename to
      `Step N` to match the Dart-side rename
    - `server/story/generator.go` — strip "Phase 1 uses
      templates only", reword as durable invariant
    - `server/undo/registry.go` (1 hit, forward-looking) —
      reword as `// The registry populator that ships alongside
      each Undo* RPC sets this …`

- [ ] **`TODO(phase1)` cleanup in `server/services/leaderboard/leaderboard.go`.**
  Seven occurrences — replace each `TODO(phase1):` with `TODO:`.
  Keep the rest of each comment intact.

- [ ] **Standalone hash-ref cleanup.** Files (each rewrites one
  or two hash references in comments to either delete them or
  describe the durable reason):
    - `server/main.go` (3: `#929`, `#929`, `#1185`)
    - `server/jobs/community_cascade_backfill.go` (1: `#1185`)
    - `server/services/roundtrip_helpers.go` (1: `#1142`)
    - `server/ai/provider_factory.go` (1: `#1216`)
    - `server/storage/testing.go` (1: `#1238`)
    - `server/services/experience/gen_ai_streaming.go` (already
      covered above; also has `#1229`)
    - `server/services/gear/gen_ai_streaming.go` (`#1229`)
    - `app/lib/services/device_location_service.dart` (1: `#1175`)
    - `app/lib/presentation/screens/discover/discover_screen.dart`
      (1: `#1174`)
    - `app/lib/presentation/widgets/content/comment_bar.dart` (2: `#853`)
  **Out of scope:** `app/patrol_test/pages/conversation_page.dart`
  (`#838`). Patrol-test code is excluded from the lint and from
  this cleanup.

- [ ] **Final verification commit.** Run `npm run lint` and
  `npm run build` locally. If the lint reports zero violations
  and the build is clean, push the branch and open the PR.

- [ ] **Update CLAUDE.md's "Go Code Comments" section** to add a
  one-line cross-reference to the new lint script — `// Enforced
  by scripts/check_ephemeral_comments.js — phase markers and
  bare issue references in `*.go`/`*.dart` comments fail the
  build.`

**Validates:** `npm run lint` passes with no baseline file, no
allowlist, no exemption flags. The bulk cleanup matches the
violation count from Phase 1's baseline (delta zero — every
listed violation was rewritten). No new violations were
introduced (CI catches that). The domain-phase enums still
type-check (`LoanBorrowerPhase` etc. are unchanged) and all
existing tests pass.

### Phase 3: Documentation refresh (optional follow-up)

**Goal:** Capture the rule for future contributors.

**Tasks:**
- [ ] Add a one-paragraph section to `docs/server/architecture.md`
  or a new `docs/comment_conventions.md` summarising the rule:
  "Don't reference rollout phases or issue numbers in `*.go` or
  `*.dart` comments. Durable behaviour goes in the comment;
  rollout context goes in the commit message, PR description,
  or plan docs under `docs/`."
- [ ] Mention the lint in the relevant README sections of any
  multi-file directory whose comments were touched in Phase 2.

**Validates:** A new contributor reading `CLAUDE.md` and
`docs/comment_conventions.md` can answer "should I write `Phase
2: …` in this comment?" without needing to find the lint script.

This phase is optional because the CLAUDE.md section + lint
itself already enforces the rule; explicit documentation is
just nice to have.

## Related Issues

- #1340 — `connecterr.Internal` migration. The lint pattern in
  this plan is modelled on `scripts/check_codeinternal.js`,
  which is the canonical "exemption-free Go-source lint script"
  in the tree. The issue body's `(issue #1340)` reference at
  `scripts/check_codeinternal.js:11` itself is a candidate hash
  ref to clean up — but that file is a JS script, out of the
  `*.go`/`*.dart` lint scope, so it stays.
- #1334 — File comments and READMEs. Same governance area
  (durable comment hygiene), separate scope.
- #1324 — Dart file size lint. Same precedent (per-file CI
  gate with optional inline escape hatch); this plan
  intentionally diverges from that precedent by *not* adding
  an escape hatch (see Option B rejection).
