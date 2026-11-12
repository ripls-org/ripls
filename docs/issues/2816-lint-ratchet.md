# Issue #2816: [tech-debt] Ratchet funlen, cyclop, dupl and goconst down (needs refactoring first)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2816
**Filed:** 2026-07-30 | **Last activity:** 2026-07-30
**Labels:** `review:code_quality` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`, `docs/server/architecture.md` (§3 Short Focused Files; §File
Organization), `docs/server/conventions.md`, `docs/server/portfolio.md`,
`docs/server/profiling.md` (AssertMaxQueries), `docs/server/l10n.md` (§The render boundary),
`docs/proto_conventions.md`, `docs/client/inbox.md`, `docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`, `docs/reviews/performance_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`

## Problem Statement

Four linters — `funlen`, `cyclop`, `dupl`, `goconst` — are configured in
`server/.golangci.yml` but disabled. #1611 enabled 41 others; these four could not
be turned on because the threshold that would be green today catches nothing
(`funlen`'s worst function is 570 lines, so a green threshold is ~571).

The issue frames this as "do the refactors, then ratchet down." Research says that
framing is right for `funlen`/`cyclop`/`dupl` and **wrong for `goconst`**.

**Re-measured on `main` (2026-07-30) — matches the issue exactly:**

| Linter | Findings (non-test) |
|---|---:|
| `cyclop` @15 | 248 |
| `funlen` @100/60 | 172 |
| `dupl` @150 | 108 |
| `goconst` @3 | 64 |

Two things the issue does not say, both of which change the plan:

**1. The real unit of work is 271 functions, not 420.** `funlen` and `cyclop`
overlap heavily — 149 functions are flagged by both, 23 by `funlen` only, 99 by
`cyclop` only. Splitting a long function usually fixes its complexity too, so the
two should be driven as one workstream, not two.

**2. It is a long tail, not a handful of god-functions.** The 271 functions are
spread across **194 files**, with at most 5 in any one file. There is no "fix the
top 10 and you're done" — the distribution is wide and shallow, which is what
makes a rung-by-rung ratchet the right shape and a big-bang PR impossible.

**`goconst` is a different problem wearing a lint costume.** Its 64 findings are
symptoms of three real defects. Mechanically hoisting the strings to Go constants
would entrench all three:

- **Stringly-typed storage fields.** `proto/ripls/models/experience.proto`
  declares `string intention = 5;  // RSVPIntention as string (YES, MAYBE, NO)`
  and, immediately below it, `string attended = 6;  // AttendedStatus as string
  (UNKNOWN, YES, NO)` — while `api.RsvpIntention` is a real enum
  (`experience_service.proto:31`). `services/experience/build_api.go:196` does
  `switch rsvp.Intention { case "YES": ... }`. That is 28 of the 64 findings and
  the fix is an enum migration, not a `const rsvpYes = "YES"`.

  Two constraints make this more than a type swap, both confirmed against the
  codebase:
  - **It must be a *new* `models.RSVPIntention` enum, not `api.RsvpIntention`.**
    `docs/proto_conventions.md` §The separation rule: "the two packages never
    import each other." `models` already carries its own enums for exactly this
    (`ExperienceState`, `Availability`, `CommunityEventType`), so the pattern
    exists — but it means defining and maintaining a parallel enum plus a
    conversion, which is a cost worth pricing in before committing.
  - **The field number cannot be reused.** proto3 encodes `string` as
    length-delimited (wire type 2) and `enum` as varint (wire type 0), so
    redefining field 5 would mis-decode every stored RSVP. It needs a new field,
    dual-population, a backfill, then retirement of field 5 — the pattern
    `TODO(#2020)` already uses in `proto/ripls/api/portfolio.proto`.
  - **CI will not catch a mistake here.** `buf.yaml` declares a `breaking:`
    config but **no workflow runs `buf breaking`** — only `buf lint`. A
    wire-incompatible change would pass CI and surface as corrupt reads in prod.
- **A render-boundary violation.** `services/portfolio/assembly.go`'s
  `resolveActionButton` and `resolveInboxStatusLabel` build English in-app UI copy
  server-side (`"Going"`, `"Review"`, `"Borrow Request"`, `"On Loan"`,
  `"Lending"`, `"Borrowing"`) and ship it in the `api.Home*` response.
  `docs/server/l10n.md` §The render boundary is explicit: *"The **client** renders
  any string the user reads inside the app. The **server** renders only strings
  where the server is the terminal emitter — push notifications, emails, SMS."*
  These are none of those. Hoisting them to Go consts makes the violation
  permanent-looking.
- **Control flow keyed on display text.** The same helpers branch on the label
  strings: `case stateLabel == "You're going" || stateLabel == "Hosting":`,
  `case stateLabel == "On loan":`, `case actionLabel == "Review":`. Reword a
  label — or localize one — and the routing silently changes behaviour. This is
  the most dangerous of the three and no lint rule is going to catch it.

## Context from Comments

**No comments.** The issue was filed by me during #1611 as a hand-off for work
that PR deliberately did not do, so the body is a research dump rather than a
discussion. That means:

- Nothing here has been reviewed by anyone else. The `goconst` findings above are
  new analysis, not agreed direction — the enum migration and the render-boundary
  work in particular are decisions someone should sign off on before code starts.
- The issue's own numbers were re-verified against `main` for this plan and are
  correct.

## A measurement trap that will bite whoever picks this up

Two of them, actually. Both cost real time during #1611 and both are easy to
repeat:

1. **`issues.uniq-by-line` defaults to `true`** — one issue per source line. Any
   linter that anchors to a function's declaration line masks the others there.
   With `cyclop` on, 340 of `funlen`'s 414 findings vanished and the survivors
   looked like the whole backlog. Already set `false` repo-wide in
   `server/.golangci.yml`; **do not remove it**, and treat any count measured
   without it as wrong.
2. **`funlen` emits at most one finding per function.** With both `lines` and
   `statements` configured, a big function trips `statements` first and its line
   count never appears. That is why `funlen`'s "is too long" list appears to top
   out at 157 while `fetchAll` is genuinely 570 lines. To read the true line
   distribution, set `statements` to something huge and measure alone.

## Potential Solutions

### Option A: One linter at a time, cleared to target, then enabled
- **Approach:** Pick `cyclop`, refactor all 248, enable at 15. Repeat per linter.
- **Pros:** Each linter lands fully enforced. Simple to describe.
- **Cons:** The first milestone is 248 refactors across 178 files before *any*
  enforcement exists — months with zero regression protection, and a merge-conflict
  surface across most of `server/`. Ignores the funlen/cyclop overlap, so a large
  share of the work would be done twice.
- **Effort:** Large, and unshippable in increments.

### Option B: Rung-by-rung ratchet, funlen+cyclop driven together
- **Approach:** Enable each linter immediately at a threshold that is green today,
  then lower it one rung per PR, fixing only what that rung surfaces. Treat
  `funlen` and `cyclop` as one workstream since 149 functions are flagged by both.
  Handle `goconst`'s three sub-problems as separate, design-led changes.
- **Pros:** Enforcement starts on day one — nothing can get *worse* while the
  backlog drains. Every rung is a bounded, reviewable PR (18 functions, then 13,
  then 22 …). The overlap is exploited instead of paid twice. Each rung is
  independently revertable.
- **Cons:** The opening thresholds are embarrassing (cyclop 130, funlen 300/570)
  and look like the linter is doing nothing. Needs a written commitment to the
  ladder or it stalls at rung one. More PRs.
- **Effort:** Large overall; each rung small-to-medium.

### Option C: Ratchet the three mechanical linters; split `goconst` out entirely
- **Approach:** Option B for `funlen`/`cyclop`/`dupl`. Move `goconst` to its own
  issue covering the enum migration, the render boundary, and the display-string
  branching — with `goconst` enabled at the end as the regression guard rather
  than the driver.
- **Pros:** Everything in B, plus it stops a lint ticket from silently becoming a
  proto migration and an i18n change. The `goconst` work has different reviewers
  (proto/API + client), different risk (a stored-field migration), and a different
  test surface. Bundling it under "ratchet four linters" hides that.
- **Cons:** One more issue to track.
- **Effort:** Same total, better separated.

## Recommendation

**Option C.**

Option A is disqualified by arithmetic: 248 refactors before the first byte of
enforcement, on a codebase where 194 files are in scope and `main` moves daily.

Between B and C, the deciding factor is that `goconst` is not a lint problem.
Two of its three sub-problems are architectural — a stringly-typed field that
should be a proto enum, and server-side rendering of in-app copy that
`docs/server/l10n.md` forbids. The third (branching on display strings) is a
latent correctness bug. None of them should be reviewed as "add some constants",
and none should block the `funlen`/`cyclop` ratchet, which is pure mechanical
extraction and can start immediately.

Two things I'd want confirmed before starting (see Open Questions): whether the
`intention` enum migration is wanted at all given it touches stored data, and
whether the portfolio labels move to the client now or just stop being
branched on.

## Open Questions

- **Do the opening ratchet thresholds need to be defensible, or just green?** —
  Recommendation: **just green, with the ladder written into the config.** cyclop
  130 and funlen 300 look absurd in isolation; their job is to make *worse* fail,
  not to make *current* pass. The config comment already carries the ladder, and
  #1611 established the precedent. The alternative — waiting until a respectable
  threshold is reachable — is Option A.
- **Should `models.ExperienceRSVP.intention` become an enum?** — Recommendation:
  **probably not yet — take the typed-constant fallback first.** The full migration
  needs a new `models.RSVPIntention` enum (the separation rule forbids reusing
  `api.RsvpIntention`), a new field number (string and enum are wire-incompatible),
  dual-population, a backfill, and retirement of field 5 — with **no `buf breaking`
  gate in CI to catch an error**. That is a real project, and `attended` has the
  same shape, so it is two of them. The cheap 80%: a typed
  `RSVPIntention string` with a constant block in
  `server/services/experience/`, which kills the bare literals, gives the compiler
  something to check, and leaves the enum migration as a clean later step. Per
  CLAUDE.md, whichever path is chosen, file the backfill cleanup issue *before* any
  `TODO(#NNNN)` lands.
- **Do the portfolio UI labels move to the client, or just stop driving control
  flow?** — Recommendation: **split it.** Removing the display-string branching is
  a pure server-side correctness fix and should land first: introduce an internal
  enum for the row state and branch on that. Moving the labels themselves to the
  client ARB catalogue is a client + server change that needs
  `docs/client/inbox.md` updated and coordination with the app. Do the first now,
  file the second.
- **Should `dupl` exclude test files permanently?** — Recommendation: **yes.**
  `docs/reviews/code_quality_review_prompt.md` §1 exempts tests from DRY in favour
  of DAMP, and 71 of the 179 findings are in tests. #1611 already excluded
  `funlen`/`cyclop`/`dupl` on `_test.go` for the measurements this plan is based on.
- **What stops the ratchet stalling at rung one?** — Recommendation: **one tracked
  issue per rung, filed up front**, each naming its threshold and its expected
  function count from the tables below. A single "ratchet it down" issue is what
  #1611 already learned decays; the ladder is only real if each rung has an owner.

## Implementation Plan

### Phase 1: Turn the ratchet on at hold-the-line thresholds
**Goal:** `funlen`, `cyclop` and `dupl` become enforced immediately, so nothing can
regress while the backlog drains.
**Tasks:**
- [ ] Enable `cyclop` at `max-complexity: 130`, `funlen` at `lines: 600` /
      `statements: 300`, `dupl` at `threshold: 500` — each green today.
- [ ] Keep `_test.go` excluded for all three (already the case).
- [ ] Put the measured ladder for each in the config next to its setting, with the
      target and the next rung named, in the shape #1611 established.
- [ ] File one issue per rung (see tables in Phases 2–4) so each has an owner.
- [ ] Verify by pushing a deliberately worse function and confirming CI blocks it.
**Validates:** That enforcement is real before any refactoring starts — the
property Option A cannot offer until the very end.

### Phase 2: Query-count guards on the portfolio hot path
**Goal:** Make the Phase 3 refactors safe to review.
**Tasks:**
- [ ] `server/services/portfolio/` currently has **zero** `AssertMaxQueries`
      coverage — verified. Add guards around `fetchAll`, `computeWeeklyMetrics`,
      `assembleInboxView` and `assembleActionsToday` **before** touching them, so
      the pre-refactor query count is the baseline. Pattern:
      `server/storage/cascade_leaver_query_count_test.go`; rationale:
      `docs/server/profiling.md`.
- [ ] Record the current P95 for `GetHomeView` / `GetPortfolioInboxView` from the
      dashboards (#1613) so the post-release comparison has a before.
**Validates:** That a "pure refactor" of the assembly path can be proven not to
have added an N+1 — the failure mode `docs/server/profiling.md` exists to catch,
on the paths #2646 already burned us on.

### Phase 3: Ratchet funlen + cyclop together, rung by rung
**Goal:** Drive the 271-function union down, exploiting the 149-function overlap.
**Tasks:** one PR per rung, in this order:

| # | cyclop | funlen statements | New functions | Files |
|---|---:|---:|---:|---:|
| 1 | 100 | 200 | ~3 | 3 |
| 2 | 70 | 150 | ~10 | 10 |
| 3 | 50 | 120 | ~22 | 22 |
| 4 | 40 | 100 | ~24 | 24 |
| 5 | 30 | 80 | ~38 | 36 |
| 6 | 25 | — | ~26 | 26 |
| 7 | 20 | 60 | ~107 | 90 |
| 8 | 15 | — | ~106 | 78 |

- [ ] Split per `docs/server/architecture.md` §3 — `service.go` keeps the struct,
      constructor and `Set*` wiring; feature files named after functionality.
- [ ] Run `npm run lint:go:size` with every rung; several of these files are near
      the 1,000-line gate (#1424) and splitting *out* helps, but splitting *within*
      does not.
- [ ] If any split creates a **new package** (rather than a sibling file in the
      existing one), it needs tests or an entry in `.github/coverage-allowlist.txt`
      — `test_go.yaml` fails on an untested package. Sibling files in the same
      package, which is what §3 prescribes, are unaffected.
- [ ] Leave the portfolio/feed hot paths (rungs 1–3: `fetchAll` 127,
      `computeWeeklyMetrics` 97, `generateFeedItems` 93, `SaveExperience` 91) until
      Phase 2's guards exist.
- [ ] `wireServices` (44) is linear constructor wiring — `//nolint:cyclop` with a
      rationale is the right answer there, not a split.
**Validates:** Whether the rung sizes hold. If rung 3 balloons past its 22
functions, the later rungs need re-cutting before rung 7's 107.

### Phase 4: dupl
**Goal:** Enforce at the declared threshold of 150.
**Tasks:**
- [ ] Ratchet 500 → 400 (4 findings) → 300 (11) → 200 (35) → 150 (108).
- [ ] Expect overlap with Phase 3: extracting a shared helper to fix `dupl` often
      removes a `funlen`/`cyclop` finding too. Sequence `dupl` rungs *after* the
      corresponding `funlen` rung to avoid re-editing the same functions.
**Validates:** That the two workstreams compose rather than fight.

### Phase 5: `goconst` — split to its own issue (do not do it here) — **DONE, filed as #2820**
**Goal:** Get the three sub-problems reviewed as what they are.
**Tasks:**
- [x] File the issue with the three groups: (a) the `intention` **and `attended`**
      stringly-typed fields (28 findings; scope the typed-constant fallback vs the
      full enum migration explicitly, and note that `buf breaking` is not gated),
      (b) the render-boundary fix for the portfolio labels, (c) the genuine
      constants — `USER_PRIMARY_LOCATION` (×14),
      `weight_material_carbon`, `spend_based_carbon`, `"http"`/`"https"` in
      `webfetch/fetcher.go`, the `simulation/report.go` section names.
- [ ] Land (c) first — it is the only mechanical part and unblocks nothing else.
- [ ] Remove the display-string branching in `resolveActionButton` /
      `resolveInboxStatusLabel` before or alongside (b).
- [ ] Enable `goconst` at `min-occurrences: 3` last, as the regression guard.
**Validates:** That the lint finding was a symptom — enabling `goconst` at the end
should require no new constants if (a)–(c) were done properly.

## Implementation Status (2026-07-30, branch `lint_ratchet_2816`)

| Phase | State |
|---|---|
| 1 — enable at hold-the-line thresholds | **done** — `dupl` 400, `cyclop` 130, `funlen` 600/300, ladders in config |
| 2 — query-count guards | **done** — portfolio had zero; `GetHomeView` 23 queries (bound 30), `GetPortfolioInboxView` 28 (bound 35) |
| 3 — funlen+cyclop rungs | **done through rung 1** — `fetchAll` 127 → gone; `cyclop` 130 → 95, `funlen` 600/300 → 430/195 |
| 4 — dupl rungs | **done through the 250 rung** — nine extractions; `dupl` 400 → 250 |
| 5 — split `goconst` out | **done** — filed as #2820 |

**Refactors landed** (−1,726 net lines):

- location/time poll **Manage** — 305 of 318 duplicated lines → one `pollSpec`-driven
  implementation.
- location/time **proposal deletion** — 108 lines of permission, cascade-delete and
  empty-poll cleanup, previously duplicated. Security-relevant, so one copy matters
  more than the line count.
- the three **`conversation_id` backfills** — 71-line copies × 3, the largest in-file
  duplication in the repo.
- **`GetNearbyUsers` / `GetNearbyGear`** — 70 lines of auth, validation, membership
  check and spatial query.
- **`computeWeeklyMetrics`** — three ~60-line metric-folding loops → one
  `weeklyAccum.add`.
- **`fetchAll`** — the worst function in the codebase (671 lines, 298 statements,
  complexity 127) → eleven section methods populating `*fetchedData` in place, plus
  a `newFetchedData` constructor that replaces the hand-built early-return struct
  and a generic `liveOwned` that replaces three copies of the owned-item filter.
- **`known_for` scope loading** — `loadScopedExperiences` / `loadScopedRequests`,
  86 lines apart from two stored types and the owner field.
- **`ListMyTransfers` / `ListReceivedTransfers`** — 54 lines of auth, filtering,
  query and build.
- **Up Next / calendar gear obligations** — the same 62 lines in
  `assembleHomeUpNext` and `assembleHomeCalendar`.
- **AI per-method override lookup** — two twelve-case switches → one table each.

**Corrections to this plan found during implementation:**

- The `funlen` "lines" ladder here is misleading on its own. funlen emits at most
  **one finding per function**, and `statements` trips first for big functions, so
  the lines list appears to cap at 157 while `fetchAll` is genuinely 570. Measure
  lines alone (statements set huge) to see the real spread. Now noted in the config.
- A config edit put a stray `max-complexity` key **nested under `forbidigo`**, where
  YAML ignored it. Only the real `cyclop` key was ever in effect, so no behaviour
  changed — but editing the stray one silently did nothing, which cost a debugging
  cycle. Removed.
- **`dupl` found a real defect, not just a copy.** `MethodFallbacks` in
  `server/ai/provider_fallback.go` had a `GenerateNudgeContent` field but no case
  for it in the switch that read the fields back, so a fallback chain configured
  for nudge generation was silently ignored. Nothing outside tests configures these
  today, which is why it had gone unnoticed. Both switches are now tables listing
  each field once, with a regression test that fails when the entry is dropped.
- **The refactors were mutation-checked, not just run.** Pointing the `known_for`
  and leaver-cascade specs at the wrong owner field fails their packages, and
  removing the nudge table entry fails the new AI test — so the green is load-bearing.

**Rung 1 is closed.** `fetchAll` is decomposed and the query-count guards from
Phase 2 still measure exactly 23 queries for `GetHomeView` and 28 for
`GetPortfolioInboxView` — the numbers they recorded before the split — so the
refactor introduced no N+1 on the SLO path.

**Where the ratchet stands now**, with the measured next rung in each config block:

| Linter | Was | Now | Next rung costs |
|---|---|---|---|
| `cyclop` | 130 | 95 | 90 — split `generateFeedItems` (93) and `SaveExperience` (91) |
| `funlen` | 600 / 300 | 430 / 195 | 350 / 150 |
| `dupl` | 400 | 250 | 200 — eight pairs, listed in the config |

Every threshold now sits just above the worst function in the tree rather than well
clear of it, so the ratchet holds the line against new debt as well as measuring the
old.

## Related Issues
- **#1611** — enabled the other 41 linters; this is the work it deliberately
  deferred. Its plan (`docs/issues/1611-wave2-linters.md`) has the measurement
  methodology.
- **#1424** — the 1,000-line Go file-size gate that Phase 3's splits interact with.
- **#1613 / #2646** — the SLOs and the latency regression on
  `GetHomeView`/`GetPortfolioInboxView`, the paths Phase 3's worst offenders sit on.
- **#2794** — Dart analyzer strictness; the client-side sibling of the same
  "linter declared but never enforced" problem.
- **New issue to file:** `goconst` (Phase 5), plus one per ratchet rung.

---

## Audit Notes (`/ripls-audit`, 2026-07-30)

**Scope:** server only. Subsystems: portfolio, experience, feed, impact_metrics,
storage, webfetch, simulation, plus `proto/ripls/models`.
**Engaged dimensions:** code_quality, tech_debt, performance, testing, api_design,
observability, i18n/l10n.

**Corrections applied:**

1. **The enum migration targeted the wrong package.** The draft said the fix was
   "an enum migration" and pointed at `api.RsvpIntention`. `docs/proto_conventions.md`
   §The separation rule forbids it — "the two packages never import each other."
   It needs a *new* `models.RSVPIntention`, following the existing
   `models.ExperienceState` / `Availability` / `CommunityEventType` pattern, which
   means a parallel enum plus a conversion rather than a reuse.
2. **The field number cannot be reused, and the draft glossed it.** proto3 encodes
   `string` as wire type 2 and `enum` as wire type 0, so redefining field 5 would
   mis-decode every stored RSVP. Now spelled out, with the `TODO(#2020)`
   dual-population precedent in `proto/ripls/api/portfolio.proto` named as the
   pattern to copy.
3. **`buf breaking` is not gated.** `buf.yaml` carries a `breaking:` config, but no
   workflow runs it — CI runs `buf lint` only. A wire-incompatible proto change
   passes CI and surfaces as corrupt reads. Flagged, because it removes the safety
   net this particular migration would most rely on.
4. **The recommendation flipped as a result.** Given three constraints plus a
   duplicate migration for `attended`, the Open Question now recommends the typed
   `RSVPIntention string` constant block first, with the full enum migration as a
   priced, separate step — rather than the draft's "yes, do the migration."
5. **`attended` has the identical defect.** `string attended = 6; // AttendedStatus
   as string (UNKNOWN, YES, NO)` sits directly below `intention`, and `"UNKNOWN"`
   is among goconst's findings. The draft treated the problem as `intention`-only;
   it is two fields.
6. **Coverage gate on new packages.** Added a Phase 3 task: `test_go.yaml` fails on
   any Go package without tests unless it is in `.github/coverage-allowlist.txt`.
   Sibling files in the same package (what `architecture.md` §3 prescribes) are
   unaffected, but a split that creates a package is not.

**Checked, already compliant:** splitting guidance matches `server/architecture.md`
§3 and the `lint:go:size` gate (#1424); the query-count approach matches
`server/profiling.md` and the `cascade_leaver_query_count_test.go` pattern; the
render-boundary reading is a direct quote from `server/l10n.md`; the
test-file exemptions match `code_quality_review_prompt.md` §1 (DAMP over DRY);
`performance_review_prompt.md` §1 (N+1) is addressed by Phase 2 preceding Phase 3;
no new dependencies, no auth/secrets surface, so security and dependency are not
engaged beyond the proto note above.

**Docs consulted:** `docs/llms.txt`; `docs/server/architecture.md`;
`docs/server/conventions.md`; `docs/server/portfolio.md`; `docs/server/profiling.md`;
`docs/server/l10n.md`; `docs/proto_conventions.md`; `docs/client/inbox.md`;
`docs/context_map.md`; `docs/reviews/code_quality_review_prompt.md`;
`docs/reviews/tech_debt_review_prompt.md`; `docs/reviews/performance_review_prompt.md`;
`docs/reviews/testing_review_prompt.md`; `docs/reviews/api_design_review_prompt.md`.
