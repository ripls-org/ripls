# Issue #1611: [code_quality] Wave 2: enable funlen, cyclop, goconst, unparam

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1611
**Filed:** 2026-04-30 | **Last activity:** 2026-05-20
**Labels:** `auto-filed`, `review:code_quality` | **Assignees:** unassigned
**Priority:** P3 (triage 2026-05-19) | **Type:** Task | **Parent:** #1514 | **Wave 1 PR:** #1605
**Context docs:** `docs/llms.txt`, `docs/server/architecture.md` (§3 Short, Focused Files; §File
Organization), `docs/server/conventions.md` (Go Server Build Verification), `docs/proto_conventions.md`,
`docs/client/conventions.md`, `docs/dependency_automation.md` (baked-tool pinning; cloud_functions
eslint gap), `docs/reviews/code_quality_review_prompt.md`, `docs/reviews/tech_debt_review_prompt.md`,
`docs/reviews/reports/code_quality_review.md`

> **Scope note.** The requester asked for this issue **plus** "fixing violations
> in-place and adding any other useful tools/linters to improve and enforce high
> code quality." That widens the issue from "add four linters" to "make Go lint
> actually enforced, clear the backlog it reveals by refactoring rather than
> suppressing, and close the equivalent gaps on TypeScript / shell / toolchain
> pinning." (Dart was measured too, but is deliberately deferred — see
> *Out of Scope* below.) The
> plan below covers all of it, phased so each phase ships on its own.

---

## Problem Statement

The issue asks to enable four more `golangci-lint` linters (`funlen`, `cyclop`,
`goconst`, `unparam`) in `server/.golangci.yml`, with `//nolint:` grandfathering
where refactoring is hard.

**Research invalidates the premise.** `server/.golangci.yml` has never been
applied. Enabling four more linters in it would change nothing.

### The enforcement gap (measured, not inferred)

`npm run lint:go` runs `golangci-lint run ./...` **from the repo root**.
golangci-lint v1 derives its config search directory from the first positional
argument; `./...` resolves to the repo root, which has no `.golangci.yml`. So the
run silently falls back to golangci-lint's *default* linter set (`errcheck`,
`gosimple`, `govet`, `ineffassign`, `staticcheck`, `unused`) — none of the
config's `linters:` block, `linters-settings:`, or `issues.exclude-rules:` are
read. Evidence:

| Command | Result |
|---|---|
| `golangci-lint config path` (repo root) | `No config file detected` |
| `golangci-lint config path` (in `server/`) | `.golangci.yml` |
| `golangci-lint run ./...` (root — what npm runs) | 20 issues, **all** `staticcheck` (default set) |
| `golangci-lint run ./server/services/gear/...` | `godot`, `gofumpt`, `revive` findings appear (config set) |

Consequences:

1. **All of Wave 1 (#1605) is inert.** The three Wave 1 linters that actually
   landed — `errorlint`, `misspell`, `bodyclose` — plus `dupl`, `godot`,
   `gofumpt`, `revive`, `gosec`, `gocritic`, `stylecheck`, `forbidigo`,
   `goheader`, `unconvert`, `whitespace` and the `govet: enable-all` setting have
   never run.
   *(The other two Wave 1 linters, `nilerr` and `nilnil`, were deliberately
   removed hours after #1605 merged — commit `25349e387` "Drop nilerr and nilnil
   linters; remove all suppressions" — because ~30 functions legitimately return
   `(nil, nil)` as a not-found sentinel and streaming RPC handlers intentionally
   return `nil` while forwarding the work error over the stream. That is a
   documented decision; **this plan does not re-litigate it.** Today there are
   zero `//nolint:nilerr`/`//nolint:nilnil` annotations in the tree.)*
2. **`golangci-lint` does not run in CI at all.** `.github/workflows/test_go.yaml`
   runs `gofmt -l .` and the custom Node/bash gates. `npm run lint:go` is invoked
   by no workflow. The only "enforcement" is the local convention documented in
   `docs/server/conventions.md`.
3. **`npm run lint:go` is red on `main` today** — 20 `staticcheck` SA1019
   deprecation errors from the `ShareGear`/`ShareRequest`/`Unshare*` proto
   deprecations (#2526), in `server/e2e_*_test.go`,
   `server/integration_tests/*`, and `server/simulation/executor.go`. Nobody
   noticed, because nothing runs it.
4. **The gofumpt gate in `lint:go` is a no-op.** `gofumpt -d . && golangci-lint …`
   — `gofumpt -d` prints a diff and **always exits 0**, so the `&&` never
   short-circuits. (`gofumpt -l .` reports 0 files today, but golangci-lint's
   `gofumpt` with the config's `extra-rules: true` reports 198.)
5. **`./...` from the root lints vendored third-party Go.**
   `go list ./...` includes `cloud_functions/node_modules/flatted/golang/pkg/flatted`.
6. **`docs/server/conventions.md` is wrong.** Line 150 claims `npm run build`
   "runs `golangci-lint` (including `gofumpt`, `godot`, `revive`, and other
   strict linters)". It does not.
7. **114 `//nolint` directives are already in the tree**, written against a
   config that was never applied: 60 `staticcheck` (the ConversationId/CommunityId
   suppressions the code-quality review has flagged since 2026-04-24), 22 `gosec`,
   17 `forbidigo`, 6 `errcheck`, 5 `revive`, 2 `dupl`, 1 `govet`, and 1
   `contextcheck` — for a linter that is not enabled. Only the `staticcheck` and
   `errcheck` ones do anything today (those linters are in golangci-lint's default
   set); the rest activate at Phase 1/2.

### Measured backlog

`golangci-lint run --config server/.golangci.yml ./server/...` with
`--max-issues-per-linter 0 --max-same-issues 0`, after `npm run generate:proto:go`:

**Existing config, as written — 2,189 issues (1,043 in non-test files):**

| Linter | Total | Non-test | Note |
|---|---:|---:|---|
| `govet` | 1,165 | 559 | `enable-all: true` → 746 `shadow` + 419 `fieldalignment` |
| `stylecheck` | 249 | 126 | |
| `godot` | 214 | 88 | |
| `gofumpt` | 198 | 173 | all from `extra-rules: true` |
| `dupl` | 166 | 95 | threshold 150 |
| `gosec` | 111 | 111 | `_test.go` already excluded |
| `gocritic` | 27 | 22 | |
| `staticcheck` | 26 | 3 | incl. the 20 SA1019 above |
| `unconvert` | 12 | 12 | |
| `whitespace` | 8 | 8 | |
| `forbidigo` | 7 | 0 | `time.Sleep` in tests (#1364) |
| `revive` | 3 | 3 | |
| `misspell` | 2 | 2 | |
| `errorlint` | 1 | 1 | |

**Wave 2 at the issue's proposed thresholds** (`cyclop=15`, `funlen=100` lines /
60 statements, `goconst` 3 occurrences) — **845 more (392 non-test):**

| Linter | Total | Non-test |
|---|---:|---:|
| `cyclop` | 509 | 248 |
| `goconst` | 175 | 92 |
| `unparam` | 87 | 29 |
| `funlen` | 74 | 23 |

> **These funlen and goconst figures are wrong** — corrected during
> implementation, see *Implementation Status*. `issues.uniq-by-line` defaults to
> **true**, reporting one issue per source line, so `cyclop` (which anchors to the
> same function-declaration line) masked 340 of `funlen`'s 414 findings. The real
> `funlen` backlog is far larger: the worst function is **570 lines**, and 85
> exceed 100 lines. Measured counts are only trustworthy with
> `uniq-by-line: false`.

Two corrections to the issue body: `funlen`'s defaults are 60 lines / 40
statements (not 100), and `cyclop`'s default is complexity **10** (not 15) — at
the tool defaults Wave 2 is 1,556 issues, not 845.

### Why the shape of the work is different than the issue assumes

- **`unparam` is small and worth fixing in place** (29 non-test). The claim that
  `funlen` was too — "23 non-test violations" — was an artifact of
  `uniq-by-line`; see the correction above. The `unparam`
  findings are not cosmetic — several are latent bugs, e.g.
  `server/services/portfolio/home_view_gear.go:15` `assembleHomeAsks` never uses
  its `userID`, `home_view_decisions.go:245` `assembleThanksDecisions` never uses
  its `userID`, and `server/services/search/service.go:284`
  `convertToSearchResultItemBatch` never uses `communityID`. A portfolio
  assembler ignoring the user it assembles for deserves a look.
- **`cyclop` cannot be fixed in one pass.** 248 non-test functions exceed 15, and
  the worst are structural: `fetchAll` (127) in `server/services/portfolio/`,
  `computeWeeklyMetrics` (97), `generateFeedItems` (93), `SaveExperience` (91),
  `GetUserCommunityRanking` (65), `wireServices` (44). These are real refactors,
  and refactoring `server/services/portfolio/*` and `server/services/feed/*`
  under a lint deadline is how you ship regressions. This needs a **ratchet**,
  not a `//nolint` sweep.

---

## Context from Comments

The issue has exactly one comment: an automated `/ripls-triage` update
(2026-05-19) setting Type=Task and Priority=P3 with the rationale
"Lower-priority polish / sub-issue / engineering tail." No human has discussed
scope, thresholds, or the refactor-vs-suppress tradeoff.

**Flag:** the absence of discussion is itself a signal. The issue was auto-filed
by `/ripls-reviews` from `docs/reviews/reports/code_quality_review.md` and
inherits that report's framing — which, like the issue, assumed the config was
live. The report's own conclusion ("the missing Go linters … would all light up
real existing issues") is right; it just understated the problem.

Relevant context from the parent thread rather than this one: PR #1605's
description already flagged the CI gap explicitly —

> **Note:** `golangci-lint` is not yet wired into the CI workflow (only
> `gofmt -d .` runs there). … Wiring a `golangci-lint run ./...` step in
> `.github/workflows/test_go.yaml` would close that gap and is suggested as a
> follow-up or can be added here if the reviewer prefers.

That follow-up was never filed. It is the root cause of everything above, and it
is Phase 1 here.

---

## Potential Solutions

### Option A: Literal issue scope — add four linters to `server/.golangci.yml`
- **Approach:** Append `cyclop`, `funlen`, `goconst`, `unparam` to the `linters:`
  block, set thresholds, `//nolint` the known violators, close the issue.
- **Pros:** Half a day. Matches the issue text exactly.
- **Cons:** Changes nothing observable — the file is not read by any command that
  runs locally-by-convention or in CI. Ships the *appearance* of a quality gate,
  which is worse than no gate: the next code-quality review would again report
  "linters enabled" and again be wrong.
- **Effort:** Small.
- **Verdict:** Reject. This is the failure mode #1605 already hit.

### Option B: Fix enforcement first, then adopt in measured waves with in-place fixes and a ratchet
- **Approach:** (1) Make `lint:go` read the config and wire it into
  `test_go.yaml`; (2) settle the 2,189-issue backlog the live config reveals —
  auto-fix what's auto-fixable, refactor what's cheap, tune the two genuinely
  over-broad settings (`govet: enable-all`, `gofumpt: extra-rules`); (3) enable
  Wave 2 with `funlen`/`goconst`/`unparam` fully fixed in place and `cyclop`
  behind a documented, ratcheting threshold; (4) add the measured set of
  additional linters that are cheap and high-signal; (5) extend the same
  treatment to e2e TypeScript and shell, and pin the linter versions themselves.
  (Dart gets the same treatment on sibling issue #2794.)
- **Pros:** Every phase is independently shippable and independently valuable.
  Fixes are real (the user's explicit ask) rather than `//nolint`. The ratchet
  gives `cyclop` a credible path to 15 without a 248-function mega-PR. Closes the
  #1605 follow-up that was dropped.
- **Cons:** Six-to-eight PRs, not one. Phase 2 is a large mechanical diff that
  will conflict with in-flight work — needs to land fast once started.
- **Effort:** Large overall; each phase small-to-medium.

### Option C: Big-bang — turn on everything and fix all 3,034 issues in one PR
- **Approach:** One branch, all linters, all fixes, all refactors.
- **Pros:** One review, one merge, done.
- **Cons:** A ~3,000-issue diff spanning `portfolio`, `feed`, `experience`,
  `gear`, `storage`, and every test file is unreviewable and will conflict with
  every open branch. Refactoring `fetchAll` (complexity 127) and
  `generateFeedItems` (93) in the same PR as a `godot` period sweep guarantees
  the risky change gets rubber-stamped along with the trivial one.
- **Effort:** Large.
- **Verdict:** Reject.

---

## Recommendation

**Option B.**

The decisive argument is that Option A cannot be validated. There is no command
whose output changes when you apply it — which is exactly how #1605 shipped five
inert linters and ~54 `//nolint` annotations that suppress nothing.

Within Option B, three calls are worth stating explicitly:

1. **Refactor, don't `//nolint`.** The issue proposes `//nolint:funlen,cyclop`
   grandfathering. The requester asked for in-place fixes, and the numbers
   support it: `funlen` (23 non-test), `unparam` (29), `goconst` (92) are all
   small enough to fix properly. `//nolint` is reserved for cases where the
   suppression is the *correct* answer (e.g. `server/wiring.go`'s `wireServices`
   is linear constructor wiring — high branch count, zero cognitive load), and
   every one carries a rationale, enforced by adding `nolintlint`.

2. **`cyclop` ratchets; it does not grandfather.** Set the ceiling where it bites
   without blocking, and step it down in follow-up PRs. Measured ladder
   (non-test functions above threshold):

   | max-complexity | non-test violators |
   |---:|---:|
   | 100 | 1 |
   | 60 | 6 |
   | 50 | 18 |
   | 45 | 23 |
   | 40 | 31 |
   | 30 | 53 |
   | 25 | 80 |
   | 20 | 143 |
   | 15 | 248 |

   Start at **50** (18 functions — fixable in this issue's Phase 4), then 40 → 30
   → 20 → 15 in tracked follow-ups. Record the current rung and the target in
   `server/.golangci.yml` next to the setting, so the ratchet is visible where
   someone would try to raise it.

3. **Tune the two settings that are over-broad, and say why.** `govet:
   enable-all: true` pulls in `fieldalignment` (419) and `shadow` (746) — 1,165
   of the 2,189 baseline issues, i.e. 53% of the backlog from one line of config.
   `fieldalignment` reorders hand-written Go struct fields by size to shave
   padding bytes — a micro-optimization that costs semantic grouping and produces
   churn in exactly the request/response and config structs a reader most needs
   to scan; none of the server's hot paths are memory-bound. Disable it.
   `shadow` at 746 findings is dominated by idiomatic `err :=` re-declaration in
   nested scopes; disable it too and rely on `ineffassign` + `staticcheck` for
   the cases that actually bite. Keep the rest of `enable-all`. Similarly,
   `gofumpt.extra-rules` (198 findings, all auto-fixable by
   `golangci-lint run --fix`) stays on — it is free once fixed.

### Beyond Go: the equivalent gaps, measured

The "other useful tools/linters" half of the ask. Everything below was measured
in this repo, not guessed.

**Additional Go linters worth enabling (measured on `./server/...`):**

*Free or near-free — enable and fix in place (≈140 findings total):*
`sqlclosecheck` (7 — unclosed `sql.Rows`, a real leak class in `server/storage/`),
`rowserrcheck` (2), `noctx` (32 total / 9 non-test — HTTP without context),
`usestdlibvars` (30/11), `predeclared` (23/11), `containedctx` (12/2),
`inamedparam` (11/8), `canonicalheader` (7/5), `errname` (6/3), `copyloopvar`
(4), `fatcontext` (4), `testifylint` (4), `gochecknoinits` (3/2),
`interfacebloat` (1), `makezero` (1), `recvcheck` (1), `wastedassign` (1),
`nolintlint` (1 — and it enforces the "every `//nolint` carries a rationale"
convention Wave 1 established by hand).

*`sloglint` — zero-cost enforcement of documented conventions.* With
`no-global: all`, `static-msg: true`, `key-naming-case: snake`, and
`forbidden-keys: [time, level, msg, source]` it reports **0 findings today** — it
would lock in the snake_case key shape that `docs/server/observability.md`
§"Standard Field Names" mandates (`request_id`, `user_id`, `target_request_id`,
`gear_name`, …) for free. It *complements* rather than replaces
`server/logging/lint_test.go`'s `TestNoEntityRequestIDLogKey`, which enforces the
semantic rule (entity IDs must use `target_request_id`, never `request_id`) that
no generic linter can know. Its `context: scope` option reports 672
(`InfoContext`/`ErrorContext` instead of `Info`/`Error`); valuable for
request-scoped logging but a separate mechanical sweep — file as a follow-up,
don't bundle.

*Medium, worth doing but not here:* `prealloc` (99/92 — explicitly deferred from
Wave 1), `exhaustive` (216/199 — proto-enum switch coverage, complements the
existing `CommunityEventType` registry gates), `contextcheck` (186/28),
`gocognit` (301/127 — overlaps `cyclop`; pick one, prefer `cyclop`),
`forcetypeassert` (798/524), `perfsprint` (997/959 — largely `--fix`-able),
`usetesting` (2,744, almost all in tests — `os.Setenv` → `t.Setenv`,
`context.Background()` → `t.Context()`; `--fix`-able).

*`protogetter` — 17,304 findings (8,563 non-test).* Enforces nil-safe `GetX()`
over direct proto field access. Genuinely valuable in a proto-first codebase and
almost entirely auto-fixable, but a 17k-line mechanical diff is its own project.
**File as a separate issue**, do not attach to #1611.

*Rejected after measuring:* `tagliatelle` (371 — snake_case JSON keys are
deliberate in AI prompt structs), `godox` (27 — superseded by a proper
`TODO(#NNNN)` gate, below), `dupword` / `decorder` / `gosmopolitan` / `musttag` /
`gomoddirectives` (0 findings, no ongoing value).

**Dart — the largest unlinted surface.** `app/analysis_options.yaml` is bare
`flutter_lints` with no analyzer strictness modes. Measured over
`app/lib` + `app/test` (833 non-generated files):

| Change | Findings | Character |
|---|---:|---|
| `strict-raw-types: true` | 2 | free |
| `strict-casts: true` | ~114 | **real** — implicit dynamic casts (`argument_type_not_assignable` 90, `non_bool_condition` 9, `invalid_assignment` 4, `return_of_invalid_type` 4, …) |
| `strict-inference: true` | ~185 | mechanical (add type arguments) |
| `unawaited_futures` | 74 | **real** — fire-and-forget async |
| `avoid_dynamic_calls` | 229 | real |
| `discarded_futures` | 375 | real (overlaps `unawaited_futures`) |
| `cast_nullable_to_non_nullable` | 24 | real |
| `close_sinks` / `cancel_subscriptions` | 7 / 1 | **real** — leak class, directly relevant to Riverpod notifiers |
| `only_throw_errors` | 5 | real |
| `avoid_positional_boolean_parameters` | 19 | style |
| `directives_ordering` | 515 | `dart fix --apply`-able |
| `prefer_int_literals` | 250 | `dart fix`-able |
| `use_if_null_to_convert_nulls_to_bools` | 72 | `dart fix`-able |
| `unnecessary_await_in_return` | 34 | `dart fix`-able |
| `prefer_final_locals` / `unnecessary_parenthesis` / `unnecessary_statements` | 9 / 5 / 3 | `dart fix`-able |
| `always_declare_return_types`, `one_member_abstracts`, `prefer_constructors_over_static_methods` | 1 each | free |
| `sort_pub_dependencies`, `missing_whitespace_between_adjacent_strings`, `test_types_in_equals`, `throw_in_finally`, `avoid_slow_async_io`, `no_adjacent_strings_in_list`, `use_late_for_private_fields_and_variables`, `join_return_with_assignment`, `avoid_returning_null_for_void` | 0 | free — enable now |

This is a better path than the `very_good_analysis` swap the code-quality review
recommends: every rule above ships with the Dart SDK, so it costs no new
dependency, and the set is chosen from measurement rather than from a vendor's
opinion. (`very_good_analysis` additionally brings `public_member_api_docs` and
`lines_longer_than_80_chars`, both of which would be immediately suppressed here.)

**e2e TypeScript — 71 files, no lint, no typecheck, currently broken.**
`e2e/tsconfig.json` sets `strict: true`, but nothing runs `tsc`. `npx tsc
--noEmit` fails with **144 errors**, of which 126 (`TS2591` "Cannot find name
'process'", `TS2304` `__dirname`) are one config bug — `"types": ["node"]` is
missing despite `@types/node` being a devDependency — and ~18 are real
(`TS2322` `number` vs `bigint` in
`tests/workflows/loan-return-mark-returned.spec.ts`, `TS7006` implicit `any`,
`TS2353`, `TS2345`, `TS1355`). Adding `npm run lint:e2e:types` and a CI step is
cheap and catches a class of e2e breakage that currently only surfaces at
runtime. ESLint (`typescript-eslint` + `eslint-plugin-playwright`) is a
reasonable follow-up but is a new dependency; typecheck first.

**Shell — 42 scripts, unlinted.** `shellcheck -S warning` over
`scripts/`, `e2e/scripts/`, `runners/`, `server/` reports **3 findings** total
(`SC2207`, `SC2064` in `scripts/generate_release_notes.sh`; `SC1008` on
`runners/clean-entrypoint.sh`'s non-shell shebang, which needs a `# shellcheck
shell=bash` directive or an exclusion). `lint_workflows.yaml` already runs
shellcheck at `--severity=warning` embedded in workflow `run:` blocks — the same
threshold applied to standalone scripts is nearly free and consistent.

**Toolchain pinning.** `runners/Dockerfile:76` installs
`golangci-lint@latest`; `versions.env` does not pin it, and
`docs/dependency_automation.md` already lists this as an open follow-up ("Pin/track
the `@latest` tools baked in the runner image"). Once golangci-lint is a required
CI gate, an unpinned linter means a runner rebuild can turn a green branch red
with no repo change. Pin `GOLANGCI_LINT_VERSION` and `GOFUMPT_VERSION` in
`versions.env` with `# renovate:` annotations, consume them in
`runners/Dockerfile`, and add them to `scripts/doctor_versions.js`. (Note:
golangci-lint v2 lives at a different module path and uses a restructured config
schema — the v1 install path floats only within v1.x, so this is hygiene, not an
active fire. A v2 migration is its own issue.)

**TODO hygiene.** `CLAUDE.md` requires `TODO(#NNNN):`; there are **12** TODOs in
`server/` + `app/lib/` without an issue reference. Small enough to fix and gate
now — a `scripts/check_todo_refs.js` in the existing custom-gate style, wired into
both `test_go.yaml` and `test_flutter.yaml` like `check_ephemeral_comments.js`.

**Considered and deferred:** `markdownlint` for `docs/` (large prose surface, low
defect yield, high false-positive rate against this repo's line-wrapping style);
`hadolint` for `Dockerfile`; `tflint` beyond the existing `terraform fmt` +
`validate`; wiring `cspell` into CI (`cspell.yaml` documents itself as
editor-only by design); fixing the broken `cloud_functions` ESLint flat-config
migration (already tracked in `docs/dependency_automation.md`).

---

## Open Questions

> **Two questions below are settled** (2026-07-28, Bryan). They are kept here with
> their answers rather than deleted, so the reasoning survives into implementation.

- **DECIDED — `golangci-lint` is a *required* CI check from Phase 1**, with the
  config narrowed to exactly the linters that are green after Phase 1's fixes. An
  advisory gate that nobody reads is how we got here; growing a green required
  gate beats declaring a red one and muting it. Practical consequence: the
  narrowing in Phase 1 is not optional bookkeeping — it is what makes "required"
  safe on day one. Each parked linter carries a `# re-enabled in Phase N` comment,
  and Phases 2–5 are the schedule for un-parking them.
- **DECIDED — the Dart work does not ship in this PR.** Analyzer strictness
  (~1,600 findings) moves to sibling issue **#2794**; see *Out of Scope* below for the
  hand-off, and the Recommendation section for the measurements that become that
  issue's research basis. Different reviewer, different CI workflow
  (`test_flutter.yaml`), no shared code with the Go phases.
- **Where should `.golangci.yml` live — repo root or `server/`?** —
  Recommendation: **keep it at `server/.golangci.yml` and pass `--config`
  explicitly** in `package.json` (`golangci-lint run --config server/.golangci.yml
  ./server/...`). Explicit beats discovery, it stops `./...` from linting
  `cloud_functions/node_modules/**`, and it keeps the file next to the code it
  governs. Add `skip-dirs: [node_modules]` as a belt-and-braces measure.
- **`shadow` and `fieldalignment` — disable, or fix?** — Recommendation:
  **disable both**, with the rationale in a comment. `fieldalignment` actively
  conflicts with the documented field-ordering conventions; `shadow` at 746
  findings has a poor signal-to-noise ratio on idiomatic Go. If we later want
  shadow checking, `revive`'s `redefines-builtin-id` and a targeted
  `govet.enable: [shadow]` scoped to non-test files is a better shape.
- **Should test files be excluded from `funlen`, `cyclop`, and `dupl`?** —
  Recommendation: **yes for `cyclop`/`funlen`/`dupl`, no for `goconst`/`unparam`.**
  Table-driven tests are legitimately long and branchy, and the code-quality review
  prompt explicitly exempts tests from DRY ("prefer DAMP over DRY in test
  bodies"). This drops `cyclop` 509→248, `funlen` 74→23, `dupl` 166→95. It also
  means the ratchet measures production complexity, which is what we care about.
- **Which `cyclop` rung do we start at?** — Recommendation: **50**, giving 18
  functions to fix in Phase 4, then file the 40 → 30 → 20 → 15 steps as a single
  tracked follow-up issue with the ladder table above in the body.
- **The 20 SA1019 `Share*`/`Unshare*` deprecation errors** — Recommendation:
  these are the tests and simulation still calling the six deprecated per-type
  RPCs that #2526 replaced with `ShareItem`/`UnshareItem`. Phase 1 must not block
  on migrating them; add a **narrowly scoped, issue-referencing** exclusion for
  SA1019 on those paths and let #2526's branch remove it when the callers
  migrate. Do not blanket-disable `staticcheck`.

---

## Implementation Status (2026-07-28, branch `linters`)

| Phase | State | Commit |
|---|---|---|
| 1 — make lint run + required CI gate | **done** | `Make golangci-lint actually run, and gate it in CI` |
| 2 — formatting / style backlog | **done** | 6 commits; `dupl` parked (#2816) |
| 3 — `goconst`, `unparam`, `funlen` | **`unparam` done**; `goconst`/`funlen` parked (#2816) | 2 commits |
| 4 — `cyclop` ratchet | parked (#2816) — see below | — |
| 5 — cheap high-signal linters | **done** | 1 commit, 18 linters |
| 6 — non-Go surfaces + pinning | **done** | 4 commits; only the lint-gate doc table remains |

**Enabled and green:** `errcheck`, `gosimple`, `govet` (minus `fieldalignment`
and `shadow`), `ineffassign`, `staticcheck`, `typecheck`, `unused`, `bodyclose`,
`errorlint`, `forbidigo`, `goheader`, `misspell`, `revive`, `unconvert`,
`whitespace`, `gofmt`, `gofumpt`, `goimports`, `godot`, `gocritic`, `stylecheck`.

**Verified:** `npm run lint:go`, `lint:go:size`, `lint:e2e:types`, `lint:shell`,
`lint:versions`, `lint:proto`, `lint:ephemeral-comments`, `lint:context-map` all
exit 0; `go test -tags=integration ./server/...` passes (102 packages).
`actionlint` and `yamllint` pass on the modified workflows.

**Defects found and fixed along the way** (none of which the issue anticipated):

- Two slice-aliasing hazards — `append(eventIDs, storyIDs...)` in
  `server/services/feed/service.go` and `append(ownerTransfers, ...)` on a
  caller-owned slice in `server/services/user/stats.go`.
- `e2e/lib/seed/gear.ts` set `mode` on `SaveGearRequest`, whose field is
  `generation_mode`; protobuf-es drops unknown keys silently, so seeded gear
  never got the provenance assignment its comment promised.
- `communityId` set on `SubmitRequestRequest` in two e2e files — that field has
  not existed since #2492.
- `goimports.local-prefixes` was `library`, a module name gone for years, so
  import grouping was never checked (0 findings before the fix, 124 after).
- `gofumpt -d .` in `lint:go` always exits 0, so the formatting gate was a no-op.

**Deliberate config decisions, each with the rationale inline in
`server/.golangci.yml`:** `govet` drops `fieldalignment` + `shadow`; `gocritic`
drops `exitAfterDefer` + `ifElseChain`; `stylecheck` excludes only the
package-name half of ST1003.

**Traps recorded for whoever continues this:**

- `golangci-lint run --fix` **corrupts multi-line doc comments** when fixing
  `godot` (v1.64.8 replaced a 7-line comment with just its fixed last line). Use
  the standalone `tetafro/godot` binary. Noted in the config.
- `gocritic` findings are not auto-fixable at all — `--fix` is a no-op for them.
- `mapfile` is bash 4; `#!/bin/bash` on macOS is bash 3.2. A SC2207 fix that
  reaches for `mapfile` breaks every Mac dev.

**Filed while implementing:** #2794 (Dart analyzer strictness), #2795 (gosec
G201 blanket exclude), #2802 (extractStateCode returned empty — since fixed and
merged as PR #2805), #2814 (finish the GetConversationContext migration).

**Consolidation (2026-07-29).** Work had scattered across three branches after
an unrelated agent ran in this directory. Everything now lives on `linters`:
`origin/main` merged in (24 commits, no conflicts), the TODO triage cherry-picked
from `feedback_cleanup` minus its `regions.go` hunk — PR #2805 had already fixed
#2802 properly on main with a real `regionlib.ExtractStateCode`, so re-adding a
marker for a closed issue would have been backwards.

The merge also proved the gate works on code it had never seen: main's
`server/services/health/service_cached_test.go` arrived with two `time.Sleep`
calls that had never faced `forbidigo`. One was genuinely flaky — a 5 ms probe
racing 8 goroutines to a single-flight barrier — and now blocks on a channel
released once all callers are in flight (verified `-count=20 -race`). The other
is a real TTL expiry and keeps a rationale'd suppression.

**Why four linters stayed parked.** `funlen`, `cyclop`, `dupl` and `goconst`
cannot be usefully enabled by config alone: the threshold that would be green
today is loose enough to catch nothing (funlen's worst function is **570 lines**,
so a green threshold is ~571; cyclop's worst is 127, so ~130). They need the
splits and extractions first. #2816 tracks that with the full ladders, plus the
`goconst` design questions — several of its 64 findings are user-facing labels
that belong in the l10n catalogue rather than a Go const, and the worst
(`"YES"` x19) should use the RSVP enum's `String()`.

**Remaining, in the order I'd take it:** `gosec` + `nolintlint` together
(Phase 2 tail, ~137 findings, security-relevant); Phase 3's `unparam` (29
non-test, several look like real defects); `dupl`; Phase 5's cheap linters
(~140, includes `sqlclosecheck`/`rowserrcheck` resource leaks); Phase 3's
`goconst` + `funlen`; Phase 4's `cyclop` ratchet (largest, riskiest);
and the repo-wide lint-gate doc table. (Phase 6's TODO-reference gate is
**done** — `scripts/check_todo_refs.js`, wired into both test workflows.)

---

## Implementation Plan

### Phase 1: Make Go lint actually run (and be green)
**Goal:** `golangci-lint` reads `server/.golangci.yml`, runs in CI as a **required**
check (decided 2026-07-28), and passes — with the config narrowed to what is green
after this phase's fixes. This is the #1605 follow-up that was never filed.

**Tasks:**
- [ ] `package.json`: in `lint:go`, replace only the `golangci-lint run ./...`
      segment with `golangci-lint run --config server/.golangci.yml ./server/...`.
      **Keep the rest of the chain intact** — `check-goroutines`,
      `lint:go:codeinternal`, `lint:go:event-terminology`,
      `lint:go:active-community-gate`, `lint:go:l10n-parity`,
      `lint:go:no-inline-user-strings` are independent gates already wired into
      this script. Same edit for `lint:go:fix`. Replace `gofumpt -d .` with a
      failing check (`test -z "$(gofumpt -l .)"`) or drop it — golangci-lint's
      `gofumpt` covers it once the config is read.
- [ ] `server/.golangci.yml`: add `run.skip-dirs: [node_modules, gen]`.
- [ ] Temporarily reduce `linters.enable` to the set that is green after this
      phase's fixes (target: the default six + `misspell`, `errorlint`,
      `whitespace`, `unconvert`, `revive`, `bodyclose`, `forbidigo`, `goheader`).
      Every removal carries a `# re-enabled in Phase N` comment naming the phase
      below. Do **not** add `nilerr`/`nilnil` — see the note in Problem Statement.
- [ ] `govet`: drop `enable-all: true` → explicit list minus `fieldalignment` and
      `shadow`, with the rationale in a comment.
- [ ] Fix the 1 `errorlint`, 2 `misspell`, 3 `revive`, 8 `whitespace`,
      12 `unconvert` findings in place.
- [ ] Add the scoped SA1019 exclusion for the deprecated `Share*`/`Unshare*`
      callers, referencing #2526.
- [ ] `.github/workflows/test_go.yaml`: add a `Lint Go (golangci-lint)` step
      running `npm run lint:go` **inside the existing `test` job**, after
      "Generate code from protos" (it needs `server/gen`) and before the test
      step, so a lint failure is cheap. Putting it in that job — rather than a new
      job — means it inherits `Go Tests`' existing required-check status and the
      `changes` path-filter skip pattern; **no branch-protection edit needed**,
      which is what makes "required from day one" a one-PR change.
- [ ] Add `package.json` and `server/.golangci.yml` to both the `push` path
      filter and the `changes` job's `dorny/paths-filter` list, so a config-only
      change re-runs the gate.
- [ ] Fix `docs/server/conventions.md:150` — describe what the command actually
      runs.

**Validates:** That the gate is real, and required. Verify by pushing a
deliberately introduced violation, confirming the PR is blocked (not merely
annotated), then removing it. From this point forward every subsequent phase's
fixes are provably enforced.

---

### Phase 2: Clear the auto-fixable backlog
**Goal:** Re-enable the formatting/style linters Phase 1 parked, with their
findings fixed mechanically. Big diff, near-zero semantic risk — land it fast.

**Tasks:**
- [ ] Re-enable `gofumpt` (`extra-rules: true`), `gofmt`, `goimports`,
      `stylecheck`, `godot`, `gocritic`. Run `golangci-lint run --fix`.
- [ ] Hand-fix the residue `--fix` can't do: 88 non-test `godot`, 126 non-test
      `stylecheck`, 22 `gocritic`.
- [ ] Re-enable `gosec`; triage the 111 findings — fix genuine ones, and for each
      accepted risk add `//nolint:gosec // <rationale>` (not a config-level
      `excludes:` entry, so the rationale lives at the site).
- [ ] **Re-examine the blanket `gosec.excludes: [G201]`.** G201 is "SQL string
      formatting", muted repo-wide with the comment "acceptable when using
      parameterized queries." `docs/reviews/security_review_prompt.md` §2
      explicitly requires reviewing every `Query*`/`Exec*` in `server/storage/`
      for string concatenation — a repo-wide mute of the one linter that finds
      it defeats that check. Replace with per-site `//nolint:gosec // G201: …`
      after confirming each site is genuinely parameterized. If any site is not,
      that is a security finding, not a lint finding.
- [ ] Re-enable `dupl` (threshold 150) with `_test.go` excluded; fix or justify
      the 95 non-test duplicates.
- [ ] Add `nolintlint` (`require-explanation: true`, `require-specific: true`,
      `allow-unused: false`). Measured: **26 findings** on the current tree — 20
      `//nolint:gosec`, 5 `//nolint:errcheck`, 1 `//nolint:contextcheck` without
      explanations. Fix all 26; delete the `//nolint:contextcheck` (that linter
      is not enabled, so the directive is dead).

**Validates:** The whole originally-configured linter set is live and green. The
`//nolint` annotations from #1605 now actually do something, and `nolintlint`
proves each has a rationale.

---

### Phase 3: Wave 2 part 1 — `goconst`, `unparam`, `funlen` fixed in place
**Goal:** Three of the four issue linters enabled with **zero** `//nolint`
grandfathering.

**Tasks:**
- [ ] Enable `unparam`; fix all 29 non-test findings. Treat the unused-`userID` /
      unused-`communityID` cases (`home_view_gear.go:15`,
      `home_view_decisions.go:245`, `search/service.go:284`,
      `experience/summary.go:124`) as **bug investigations first** — determine
      whether the parameter should be used before deleting it, and add a
      regression test wherever it should have been.
- [ ] Enable `goconst` (`min-occurrences: 3`, `ignore-tests: true`); introduce
      named constants for the 92 non-test findings. Priority: the material /
      impact-factor strings (`weight_material_carbon`, `spend_based_carbon`), the
      `STORY_TYPE_*` literals (should be proto enum names via `.String()`), and
      `USER_PRIMARY_LOCATION`. The code-quality review has been calling out the
      stringly-typed material categories since 2026-04-24; a proto enum already
      exists — use it.
- [ ] Enable `funlen` (`lines: 100`, `statements: 60`, `ignore-tests: true`);
      split the 23 non-test functions per `docs/server/architecture.md` §3 —
      feature files named after functionality, `service.go` keeps struct +
      constructor + `Set*` wiring. Largest: `getUserImpactMetrics` (141),
      `buildInformalToCalendarPrompt` (131), `startBackgroundJobs` (126),
      `GetStockImage` (124), `NewE2EDeterministicProvider` (121).
- [ ] Run `npm run lint:go:size` after the splits — some of these files are near
      the 1,000-line gate already.
- [ ] `go test -tags=integration ./...` after each service split.

**Validates:** Three of the issue's four linters are enforced with real fixes,
not suppressions. `unparam` will likely have surfaced at least one real defect.

---

### Phase 4: Wave 2 part 2 — `cyclop` with a documented ratchet
**Goal:** `cyclop` enabled and enforced, at a rung that costs 18 refactors, with
the descent to 15 tracked.

**Tasks:**
- [ ] Enable `cyclop` at `max-complexity: 50`, `skip-tests: true`, with the
      ladder table and target (15) in a config comment.
- [ ] Refactor the 18 non-test functions above 50. Order by risk, each with its
      own commit: `wireServices` (44 — likely `//nolint:cyclop` with rationale,
      it's linear wiring), then the portfolio assemblers (`fetchAll` 127,
      `computeWeeklyMetrics` 97, `assembleHomeCalendar` 62, `assembleInboxView`
      59, `assembleFeedItems` 56, `assembleActionsToday` 55),
      `generateFeedItems` (93), `SaveExperience` (91), `RSVPToExperience` (60),
      `transitionTransferState` (58), `GetUserCommunityRanking` (65),
      `estimator/config.go` `validate` (60).
- [ ] **Guard the query counts.** `fetchAll`, `computeWeeklyMetrics`,
      `assembleInboxView`, and `assembleHomeCalendar` are the assembly hot paths;
      `docs/server/profiling.md` §"AssertMaxQueries" exists precisely to stop a
      refactor from turning a batched fetch into an N+1. Wrap each refactored
      assembler in a `storage.AssertMaxQueries(t, ctx, max, fn)` test **before**
      touching it, so the pre-refactor count is the baseline. Follow the existing
      pattern in `server/storage/cascade_leaver_query_count_test.go`.
- [ ] **Guard the latency SLO.** `GetHomeView` / `GetPortfolioInboxView` carry
      P95 SLOs and burn-rate alerts (`terraform/modules/monitoring/slos.tf`,
      #1613/#2646). These refactors must be behavior-preserving; re-pull the P95
      after the change reaches prod rather than assuming a pure refactor is free.
- [ ] `server/services/portfolio/*` and `server/services/feed/*` are covered by
      `docs/server/portfolio.md`, `docs/client/inbox.md`, `docs/client/calendar.md`,
      and `docs/client/the_feed.md` — run `/ripls-doc-freshness` on those after
      the refactors, or expect the nightly ratchet to.
- [ ] File the follow-up issue for 50 → 40 → 30 → 20 → 15, with the ladder table.
- [ ] Full `go test -tags=integration ./...` plus `npm run test:server:e2e`.

**Validates:** All four issue linters enforced. **#1611 closes here.** Phases 5–6
are the "other useful tools" half of the request.

---

### Phase 5: The cheap high-signal Go linters
**Goal:** ~140 findings across 18 linters, all fixed in place.

**Tasks:**
- [ ] Enable and fix: `sqlclosecheck` (7), `rowserrcheck` (2), `noctx` (9
      non-test), `usestdlibvars` (11), `predeclared` (11), `containedctx` (2),
      `inamedparam` (8), `canonicalheader` (5), `errname` (3), `copyloopvar`,
      `fatcontext`, `testifylint`, `gochecknoinits` (2), `interfacebloat`,
      `makezero`, `recvcheck`, `wastedassign`.
- [ ] Enable `sloglint` with `no-global: all`, `static-msg: true`,
      `key-naming-case: snake`, `forbidden-keys: [time, level, msg, source]` —
      0 findings today; this locks in `docs/server/observability.md`
      §"Standard Field Names" for free, alongside (not instead of) the existing
      `server/logging/lint_test.go` semantic checks.
- [ ] Prioritize `sqlclosecheck`/`rowserrcheck` — those are resource leaks in
      `server/storage/`, and `docs/server/profiling.md` documents the query path
      they sit on.
- [ ] File follow-up issues (do **not** bundle): `protogetter` (17,304, mostly
      `--fix`-able), `sloglint context: scope` (672), `usetesting` (2,744, test-only,
      `--fix`-able), `perfsprint` (997), `forcetypeassert` (798), `exhaustive`
      (199 non-test), `prealloc` (92), `contextcheck` (28).

**Validates:** A measurable class of latent resource leaks closed, and the
logging conventions become machine-enforced rather than doc-enforced.

---

### Phase 6: Non-Go surfaces + toolchain pinning
**Goal:** The remaining unlinted surfaces, all cheap. **Dart is explicitly out of
scope** — see "Out of scope" below.

**Tasks:**
- [ ] `e2e/tsconfig.json`: add `"types": ["node"]` (clears 126 of 144 errors);
      fix the ~18 real errors (the `bigint` mismatch in
      `loan-return-mark-returned.spec.ts`, implicit `any`s). Add
      `lint:e2e:types` → `cd e2e && tsc --noEmit`; wire into `test_web_e2e.yaml`.
- [ ] `lint:shell` → `shellcheck -S warning` over `scripts/`, `e2e/scripts/`,
      `runners/`, `server/` — 3 findings; fix them and add a
      `# shellcheck shell=bash` directive to `runners/clean-entrypoint.sh`. Wire
      into `lint_workflows.yaml` (or a new small workflow) at the same severity
      threshold that workflow already uses for embedded scripts.
- [ ] `scripts/check_todo_refs.js` — fail on `TODO`/`FIXME` without `(#NNNN)` in
      `server/**/*.go` and `app/lib/**/*.dart`; fix the 12 existing ones (file
      issues where needed, per CLAUDE.md). Wire into `test_go.yaml` and
      `test_flutter.yaml` alongside `check_ephemeral_comments.js`. Add a
      `test:scripts` unit test for it — `scripts/*.test.js` is the established
      pattern.
      *This is the only task in the issue that touches `app/lib/**` and
      `test_flutter.yaml`. It is a comment scanner, not Dart analysis — it shares
      nothing with the deferred analyzer work, and `check_ephemeral_comments.js`
      already spans both sides for exactly this reason. Splitting it by language
      would mean two scripts and two gates for one rule. Say the word if you'd
      rather it ship Go-only and pick up Dart with #2794.*
- [ ] `versions.env`: pin `GOLANGCI_LINT_VERSION` and `GOFUMPT_VERSION` with
      `# renovate:` annotations; consume in `runners/Dockerfile` (replacing
      `@latest`); add to `scripts/check_versions.js` and
      `scripts/doctor_versions.js`. Cross out the corresponding bullet in
      `docs/dependency_automation.md`.
- [ ] Add a `docs/` linting section — most naturally in `docs/server/conventions.md`
      (Go gates) and a short new "Lint gates" table listing every `npm run lint:*`
      script, what it enforces, and which workflow runs it. Register it in
      `docs/llms.txt` via `npm run generate:context-map`.

**Validates:** Every language surface in the repo except Dart has a CI-enforced
linter, and the linter versions themselves are pinned and Renovate-tracked — so a
runner rebuild can no longer silently change what "green" means.

---

## Out of Scope: Dart analyzer strictness — filed as #2794

**Decided 2026-07-28 — does not ship in this PR.** The measurement is done and
lives in the Recommendation section above ("Dart — the largest unlinted surface");
it is the research basis for **#2794**, which was filed 2026-07-28 with that table
in its body. Summary of what that issue covers:

- `app/analysis_options.yaml` is bare `flutter_lints` across 833 non-generated
  Dart files, with no analyzer strictness modes.
- Suggested order once it starts: `strict-raw-types` (2) + the 9 zero-finding
  rules → `dart fix --apply` sweep (`directives_ordering` 515,
  `prefer_int_literals` 250, `use_if_null_to_convert_nulls_to_bools` 72,
  `unnecessary_await_in_return` 34, and 17 more) → the leak/bug rules by hand
  (`unawaited_futures` 74, `cast_nullable_to_non_nullable` 24, `close_sinks` 7,
  `only_throw_errors` 5, `cancel_subscriptions` 1) → `strict-casts` (~114 real
  implicit dynamic casts) → `avoid_dynamic_calls` (229) and `discarded_futures`
  (375) → `strict-inference` (~185) last, highest volume and lowest value.
- Two things that issue must check that this one does not: whether
  `test_flutter.yaml` should run `flutter analyze --fatal-infos` (it currently
  does not, so analyzer infos are advisory), and whether
  `avoid_positional_boolean_parameters` (19) is worth the churn against
  `docs/client/conventions.md`.
- Cross-reference `docs/client/logout.md` when fixing `unawaited_futures` — the
  logout-teardown ordering contract is exactly the class of code that rule
  protects.

**Why it is separate:** different reviewer, different CI workflow, ~1,600
findings, and zero shared code with the Go phases. Bundling it would make the Go
lint gate hostage to a Dart review.

---

## Related Issues

- **#1514** — parent: "`.golangci.yml` still missing widely-adopted linters."
  Closed by #1605, but its premise (that the config was live) was wrong; this
  plan's Phase 1 is the correction.
- **#1605** — Wave 1 PR. Its own description flagged the missing CI wiring as an
  unfiled follow-up. Three of its five linters (`errorlint`, `misspell`,
  `bodyclose`) are in the config and become effective only at Phase 1; the other
  two were reverted the same day by `25349e387` with a documented rationale that
  this plan honors.
- **#1424** — Go file-size gate (`npm run lint:go:size`). Phase 3's `funlen`
  splits interact with it; run both.
- **#2526** — `ShareItem`/`UnshareItem` convergence. Owns the 20 SA1019
  deprecation errors that make `lint:go` red today; Phase 1 adds a scoped,
  issue-referencing exclusion that #2526 removes.
- **#1364** — `time.Sleep` in tests; the reason `forbidigo` exists in the config
  (7 findings, all test-only).
- **#2020** — proto-deprecation `TODO(#2020):` markers, cited in CLAUDE.md as the
  model for the `TODO(#NNNN)` convention Phase 6 gates.
- **#2287 / #2395 / #2416** — runner-image and version-pinning machinery that
  Phase 6's `versions.env` additions plug into.
- **New issues to file:**
  - ~~Dart analyzer strictness~~ — **filed as #2794** (2026-07-28).
  - `cyclop` ratchet 50 → 40 → 30 → 20 → 15 (with the ladder table).
  - `protogetter` adoption (17,304, mostly `--fix`-able).
  - `sloglint context: scope` (672); `usetesting` (2,744); `perfsprint` (997);
    `forcetypeassert` (798); `exhaustive` (199); `prealloc` (92);
    `contextcheck` (28).

---

## Audit Notes (`/ripls-audit`, 2026-07-28)

**Scope:** both sides + infra. Subsystems: build/CI tooling, `server/services/portfolio`,
`server/services/feed`, `server/storage`, `app/lib` (analyzer), `e2e`, `runners`.
**Engaged dimensions:** code_quality, tech_debt, security, performance, observability,
testing, dependency.

**Corrections applied to this plan:**

1. **`nilerr`/`nilnil` contradicted a documented decision.** The first draft
   listed both in Phase 1's target linter set. Commit `25349e387` removed them
   deliberately the same day #1605 merged, with a rationale (legitimate
   `(nil, nil)` not-found sentinels; streaming handlers that return `nil` while
   forwarding the work error over the stream). Removed from Phase 1 and recorded
   as a decision not to re-litigate. The draft's claim that #1605's
   `//nolint:nilerr`/`nilnil` annotations "suppress nothing" was also wrong —
   there are zero such annotations in the tree.
2. **`fieldalignment` rationale was wrong.** The draft justified disabling it by
   citing `docs/proto_conventions.md`'s field-ordering convention — that rule
   governs `.proto` field *declaration* order, not Go struct layout, and the
   generated Go structs live under `gen/` (already excluded). Replaced with the
   accurate argument (padding micro-optimization vs. semantic grouping in
   hand-written structs; no memory-bound hot paths).
3. **Phase 1 would have deleted six working CI gates.** `lint:go` is a chain:
   `golangci-lint` **plus** `check-goroutines`, `codeinternal`,
   `event-terminology`, `active-community-gate`, `l10n-parity`, and
   `no-inline-user-strings`. The draft's rewrite of the script replaced the whole
   chain. Now scoped to the `golangci-lint` segment only.
4. **Security gap added (security §2).** `gosec.excludes: [G201]` mutes SQL
   string-formatting detection repo-wide. `security_review_prompt.md` §2 requires
   auditing `Query*`/`Exec*` for concatenation — the blanket mute defeats the one
   automated check for it. Phase 2 now re-examines it per-site.
5. **Performance/observability guard added (performance §1–2, observability §5).**
   Phase 4 refactors the `GetHomeView`/`GetPortfolioInboxView` assembly path,
   which carries P95 SLOs and burn-rate alerts (#1613, #2646). Added a
   requirement to establish `storage.AssertMaxQueries` baselines *before*
   refactoring (per `docs/server/profiling.md`) and to re-pull P95 after release
   rather than assuming a refactor is latency-neutral.
6. **`nolintlint` sizing was unmeasured.** The draft cited 1 finding (default
   settings). Re-measured with `require-explanation`/`require-specific`/
   `allow-unused: false`: **26 findings**. Also inventoried the 114 existing
   `//nolint` directives, one of which (`contextcheck`) targets a linter that is
   not enabled.
7. **`sloglint` claim narrowed.** Verified snake_case against
   `docs/server/observability.md` §"Standard Field Names"; clarified that it
   complements, and does not replace, `server/logging/lint_test.go`'s semantic
   `request_id` vs `target_request_id` check.

**Not changed (checked, already compliant):** file-splitting guidance matches
`docs/server/architecture.md` §3 and the `lint:go:size` gate (#1424); the new
`scripts/check_todo_refs.js` gate follows the established
`scripts/*.js` + `scripts/*.test.js` + `test:scripts` pattern; version pinning
routes through `versions.env` + `check_versions.js` + `doctor_versions.js` per
`docs/dependency_automation.md`; test-file exemptions for `dupl`/`funlen`/`cyclop`
match `code_quality_review_prompt.md` §1 ("prefer DAMP over DRY in test bodies");
no proto or RPC surface is touched, so `proto_conventions.md` and api_design are
not engaged.

**Post-audit decisions (2026-07-28, Bryan):** golangci-lint is a **required** CI
check from Phase 1 with the narrowed config — Phase 1 now specifies placing the
step inside the existing `Go Tests` job so it inherits required status without a
branch-protection edit. Dart analyzer strictness is **out of scope** and was filed
as **#2794**; the former
Phase 6 became the *Out of Scope* hand-off section and the old Phase 7 is now
Phase 6. One residual Dart touch remains and is flagged inline for veto:
`scripts/check_todo_refs.js` scans `app/lib/**/*.dart` for untracked TODOs, which
is comment scanning rather than analyzer work.

**Docs consulted:** `docs/llms.txt`; `docs/server/architecture.md`;
`docs/server/conventions.md`; `docs/server/observability.md`;
`docs/server/profiling.md`; `docs/server/portfolio.md`; `docs/proto_conventions.md`;
`docs/client/conventions.md`; `docs/dependency_automation.md`;
`docs/context_map.md`; `docs/reviews/code_quality_review_prompt.md`;
`docs/reviews/tech_debt_review_prompt.md`; `docs/reviews/security_review_prompt.md`;
`docs/reviews/performance_review_prompt.md`;
`docs/reviews/observability_review_prompt.md`;
`docs/reviews/error_handling_review_prompt.md`;
`docs/reviews/testing_review_prompt.md`;
`docs/reviews/reports/code_quality_review.md`.
