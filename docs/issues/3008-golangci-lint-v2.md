# Issue #3008: Migrate golangci-lint v1.64.8 → v2 (surfaces ~432 production findings)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/3008
**Filed:** 2026-08-28 | **Last activity:** 2026-08-28
**Labels:** `dependencies` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `docs/server/architecture.md`,
`docs/server/conventions.md` (§"Build verification" — the `npm run lint:go`
chain), `docs/proto_conventions.md`; leaves `ripls-notes/dependency_automation.md`
(§1 Renovate, §2 versions.env, §3 required checks, §5 runner image, Gotchas),
`ripls-notes/infra_mimir.md` (§build/recreate pipeline), `docs/simulation.md`,
`docs/server/semantic_search.md`; review prompts
`docs/reviews/dependency_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`,
`docs/reviews/security_review_prompt.md` (for the new gosec taint rules).

**UI investigation:** skipped — no UI-observable behavior. This is a CI
toolchain and lint-backlog issue with no user-facing symptom to reproduce.

**Comments:** the issue has **none**. Everything below either comes from the
issue body or from measurements I ran against `main` (`bbb76443e`).

---

## Problem Statement

`versions.env` pins `GOLANGCI_LINT_VERSION=1.64.8`. v1 is end-of-line; latest is
**2.13.2** (v2.13.1 at filing). The issue frames this as one problem — "473
findings need triage" — and documents that half well. But the migration is
actually **two independent problems**, and the one the issue does not mention is
the harder of the two:

1. **Delivery.** golangci-lint is a *baked* tool: `runners/Dockerfile` `go
   install`s it from `versions.env` into the runner image, and the fleet only
   picks up a new image *after* merge. The v1→v2 config schema change is an
   **atomic, mutually-exclusive cutover** — neither binary will read the other's
   config — so there is no version of this PR that is green both before and
   after the fleet rolls. This is the ONNX two-step problem (`dependency_automation.md`
   → "Go `onnxruntime_go` — native ORT version coupling") *without the escape
   hatch that made ONNX solvable*.
2. **Findings.** 473 issues under the migrated config, of which 432–435 are in
   production files.

The issue's measurements are correct — I reproduced all of them. But its
characterization of the *shape* of the backlog is off by roughly 3×, in a way
that makes the work substantially cheaper than it reads.

---

## Verification of the issue's claims

Run against `main` @ `bbb76443e` with golangci-lint **2.13.1** installed into an
isolated `GOBIN` (so the repo's pinned v1.64.8 stayed intact), after a clean
`rm -rf server/gen && npm run generate:proto:go`.

| Claim | Verdict |
|---|---|
| `migrate` refuses on `revive.min-confidence` | ✅ exact error reproduced |
| Removing that key lets migration complete | ✅ |
| `run.timeout` is dropped | ✅ warned by the migrator |
| The migrator strips every comment | ✅ **18,987 B → 3,285 B** (83% of the file is the comments) |
| 473 issues total | ✅ exact |
| goconst 356 / staticcheck 46 / gofumpt 30 / noctx 28 / gosec 12 / govet 1 | ✅ every count exact |
| ~432 production / 41 test | ✅ 435 / 41 by my split |
| goconst explosion is **not** a config-translation bug | ✅ — see below, I tried to falsify this and failed |

**A hypothesis I tested and had to discard.** The migrator drops
`goconst.ignore-tests: true` and emits a `path: (.+)_test\.go` exclusion rule
instead — semantically a different thing (don't *count* test occurrences vs.
don't *report* test-anchored findings), and exactly the trap #2820 documented.
v2 still accepts `goconst.ignore-tests` (`golangci-lint config verify` passes),
so I re-ran with it restored and the migrator's exclusion removed. **Result:
identical, 356 either way.** The issue's claim stands; the driver is elsewhere.

---

## Material corrections and additions

### 1. The cutover is atomic — there is no green intermediate state

Both directions refuse, verified live:

```
$ golangci-lint-v1.64.8 run --config <migrated v2 config> ./server/...
Error: you are using a configuration file for golangci-lint v2 with golangci-lint v1: please use golangci-lint v2

$ golangci-lint-v2.13.1 run --config server/.golangci.yml ./server/...
Error: can't load config: unsupported version of the configuration: ""
```

Consequences, none of which the issue anticipates:

- A single PR that bumps `versions.env` **and** migrates the config **fails its
  own `Go Tests`**, because that job runs on the current fleet, which has v1.
- If it were force-merged anyway, **`main` stays red for every PR** until
  `build_runner_image` rebuilds *and* mimir's `recreate_runners.sh` cron rolls
  the fleet — a daily schedule plus an `--idle-only` pass
  (`infra_mimir.md` §"Build and recreate are deliberately split"). Worst case is
  the better part of a day of red required checks on every open PR.
- **The ONNX sequencing recipe does not transfer.** That one works only because
  "ORT is backward compatible — a 1.29 runtime still serves API 22", so step 1
  is green on both fleets. There is no analogous config that v1.64.8 and v2.13.1
  both accept.
- **`Validate Runner Candidate` would not catch this even on a Renovate PR.**
  `scripts/runner_candidate_checks.sh` runs `go build` / `go build
  -tags=integration`, never `npm run lint:go`. The candidate image is built and
  compile-smoked; the lint toolchain inside it is never invoked.

So the delivery mechanism has to change before, or as part of, the migration.
This is the decision the issue leaves implicit and it should be made first.

### 2. 356 goconst findings are 113 distinct strings

v2's goconst reports **once per (string, file) pair**; v1 reported a repeated
string **once, at a single occurrence**. Measured:

- 356 findings
- **356** distinct (string, file) pairs — an exact match, confirming the rule
- **113** distinct strings

So the backlog is 113 decisions, not 356. And it is heavily concentrated:

| | findings cleared |
|---|---|
| top 5 strings | 94 / 356 (26%) |
| top 10 strings | 132 / 356 (37%) |
| top 20 strings | 175 / 356 (49%) |
| top 50 strings | 267 / 356 (75%) |

The single largest coherent group is **SQL column-name literals** — ten `*_id`
strings accounting for **111 of 356 findings (31%)**:

```
35 community_id   18 user_id     15 gear_id        14 experience_id  12 request_id
 8 owner_id        4 recipient_id  2 location_id     2 conversation_id  1 requester_id
```

These are structural to the proto-SQL storage pattern, where column names derive
from proto field names (`docs/server/architecture.md` §"Field Flattening"). They
are one decision, not 111.

A second concentration is **fixture-shaped production trees** that `_test.go`
exclusions structurally cannot reach: `ai/eval/` (58) and `server/simulation/`
(46) — 104 findings, 29% — plus `auth/test_utils.go` (6), a test helper whose
filename keeps it out of every test exclusion. `docs/reviews/code_quality_review_prompt.md`
§1 exempts tests from DRY in favour of DAMP; these files are tests by role and
not by filename. `auth/test_utils.go`'s six are `iss`/`aud`/`sub`/`exp`/`iat` —
**standard JWT claim names**, where hoisting to constants is strictly worse.

**A subset I expected to be high-signal, and is not.** 19 findings say *"but such
constant `X` already exists"* — a literal that matches a named constant in scope
(`topicExperience`, `topicGear`, `topicRequest`, `ExperienceNameEmbeddingVariant`).
I planned to fix these first as real inconsistencies. Reading them says otherwise:
goconst's `match-constant` matches by **value, not meaning**, and the three
`topic*` constants are documented in `storage/activity_digest.go` as
*"conversation topic_type values, as stored on the conversation row"*. The sites
it wants to redirect are a **SQL table name** (`itemTable: "gear"` in
`cascade_leaver.go`), a **backfill log label** (`kind` in `community_schema.go`,
whose own comment says *"names the parent in log lines"*), and a **derived log
value** (`topicKindStr`). Taking the suggestion would couple three independent
vocabularies through one constant — the #2820 lesson running backwards. Where the
meaning genuinely matches, the constant is already used.

That correction generalises: the goconst backlog is not 113 magic values, it is
mostly **shared vocabulary** — protosql-derived column and table names, and the
structured-log field names that `docs/server/observability.md` →
*"Standard Field Names"* defines deliberately so log queries work across
services. `community_id` appears in 35 files because the schema and the logging
convention both say it should.

### 3. 31 of 46 staticcheck findings are checks that never ran under v1

v2 merges `gosimple` + `stylecheck` + `staticcheck` into one linter. The migrator
folded the repo's `stylecheck.checks: ["all"]` into `staticcheck.checks: ["all"]`
— which in v2 means SA + S + ST **+ QF**. QF (quickfix) checks ran under no v1
linter in this config. Breakdown:

```
31 QF   (QF1012 ×14, QF1008 ×10, QF1011 ×2, QF1003 ×2, QF1001 ×2, QF1002 ×1)
15 SA   (all SA1019 — deprecated API usage, from a much newer staticcheck bundle)
 0 S    0 ST   (the repo was already clean on both)
```

So "staticcheck: 46" is 31 findings of **newly-in-scope opinion** and 15 of
**genuine deprecation debt**. Those are different decisions and should be triaged
separately — `checks: ["all"]` was written when it governed stylecheck alone, and
carrying it verbatim onto the merged linter silently widens the gate.

### 4. Every count above is explained by a bundled-tool version, not by v2 itself

`go version -m` on both binaries settles what actually changed. This is the
single most useful table in this document, and it was missing from the first
draft:

| bundled tool | golangci-lint 1.64.8 | golangci-lint 2.13.2 | explains |
|---|---|---|---|
| `goconst` | 1.7.1 | **1.11.0** | per-file reporting → 113 strings become 356 findings |
| `noctx` | 0.1.0 | **0.5.1** | the entire `database/sql` rule family (§5) |
| `gosec` | 2.22.2 | **2.28.0** | G703/G704/G705 taint analysis (§6) |
| `gofumpt` | 0.7.0 | **0.11.0** | the 30 formatting findings |

So "v2 is stricter" is the wrong frame throughout: **v2 ships newer analyzers**,
and the schema change is orthogonal to every finding except the QF expansion in
§3, which really is a config-translation effect.

The gofumpt row corrects a claim in the first draft. There is a two-gofumpts
problem, but it is **not** a version skew and the skew that existed ran the other
way: `GOFUMPT_VERSION=0.11.0` pins the *standalone* binary that `npm run
lint:go:fmt` runs, and golangci-lint **v1** bundled 0.7.0 — three releases
*behind* the pin. 2.13.2 bundles exactly 0.11.0, so today the two agree by
coincidence.

The durable asymmetry is the flag, not the version: `lint:go:fmt` runs
`gofumpt -l .` **without `-extra`**, while this config sets
`gofumpt.extra.group-params`. The golangci-lint gate is therefore a strict
superset, and the standalone one can be green while it is red — which is exactly
what happened. Nothing pins the bundled version, so a future golangci-lint bump
can move it again silently; the check is
`go version -m "$(command -v golangci-lint)" | grep gofumpt`.

### 5. noctx's 28 are a new rule family, concentrated in startup DDL

```
16  (*sql.DB).Exec       → ExecContext
 5  exec.Command         → CommandContext
 2  net.Listen           → (*net.ListenConfig).Listen
 2  (*sql.DB).QueryRow   → QueryRowContext
 2  (*sql.DB).Ping       → PingContext
 1  (*sql.DB).Query      → QueryContext
```

13 of 28 are in `storage/protosql_schema.go` (boot-time schema DDL), the rest in
`storage/testing.go`, the two column-migration files, `main.go`, and
`cmd/check-image-gps`. These are context-less by construction today, and the
`*Context` conversion is mechanical and arguably an improvement (shutdown
cancellation). Note these sites are on concrete `*sql.DB` — the `SQLDB`/
`SQLExecutor` interfaces used throughout `server/storage/` are invisible to
noctx for the same reason `gosec` G201/G202 can't see them (documented at length
in `server/.golangci.yml`).

### 6. gosec's 12 are three brand-new taint-analysis rules

`G703` path traversal ×9, `G704` SSRF ×2, `G705` XSS ×1 — none of which exist in
the gosec bundled with v1.64.8. The two G704s (`location/mapbox.go:455,466`) are
**provably false**: the URL is `fmt.Sprintf("…?longitude=%f&latitude=%f", …)` over
two `float64`s that the function range-checks (`-90..90`, `-180..180`) three
lines earlier; a float64 cannot inject a host. The G703 family
(`storage/bucket.go` ×5, `storage/media.go`, `ai/embedding/embedding.go`, two
`cmd/` tools) and the single G705 (`services/web/app_handler.go:173`, a static
asset write) need per-site adjudication — this is the one group where a real
defect could hide, and it engages `docs/reviews/security_review_prompt.md`.

### 7. Three things that do NOT need changing (checked, so nobody re-checks)

- **`npm run lint:go:fix` still works.** v2's `run --fix` help says "Apply the
  fixes detected by the linters **and formatters**" — the separate `golangci-lint
  fmt` command is an addition, not a replacement.
- **`scripts/doctor_versions.js` already parses v2.** v2 prints `has version
  2.13.1` with no `v` prefix; `extractGolangciLint`'s regex is
  `/has version v?([0-9]+\.[0-9]+\.[0-9]+)/` — the `v?` already covers it.
- **`scripts/check_versions.js` needs no change** for the version itself; but see
  Phase 1, the `runners/Dockerfile` install line hardcodes the **v1 module path**
  (`github.com/golangci/golangci-lint/cmd/...`) and must become `/v2/cmd/...`.

### 8. OSS angle: public CI cannot run `npm run lint:go` at all today

`oss_manifest.txt` ships `versions.env` (line 73) and `server/**` (line 34) but
excludes `runners/**` (line 97) and `validate_runner_candidate.yaml` (line 101).
`Go Tests` runs on `${{ vars.CI_RUNNER_LINUX || 'ubuntu-latest' }}`, and
`.github/actions/setup-toolchain` provisions **go / node / flutter / java only** —
not golangci-lint, not gofumpt. So in the public repo the `Lint Go code` step
fails at its first command. That gap is adjacent enough that the fix for it and
the fix for problem (1) are the same change.

---

## Potential Solutions

Two independent axes. **Axis A** is delivery (how the new binary reaches CI
without a red window); **Axis B** is findings triage. Axis A must be settled
first — it determines whether the migration is one PR or four.

### Axis A — delivery

#### Option A1: Bake both binaries through a transition window
- **Approach:** PR 1 adds `GOLANGCI_LINT_V2_VERSION` to `versions.env` and a
  second `go install .../v2/cmd/golangci-lint` in `runners/Dockerfile` under a
  distinct binary name; merge, let the fleet roll. PR 2 migrates the config and
  points `lint:go:vet` at the v2 binary. PR 3 removes v1 and renames back.
- **Pros:** Keeps the baked-tool model exactly as documented. Every PR is green
  on the fleet it runs on. No new network dependency in CI.
- **Cons:** Three PRs and **two fleet rolls** (≥2 days of calendar time given the
  daily cron). The temporary binary name leaks into `package.json`,
  `doctor_versions.js`, `check_versions.js`'s `runnerBakedKeys`, and the
  `Bash(golangci-lint:*)` allowlists in five `claude_*.yaml` workflows. Leaves
  the OSS gap (§8) and the two-gofumpts problem (§4) untouched, and leaves the
  next schema-breaking bump to repeat the whole dance.
- **Effort:** medium, spread over days.

#### Option A2: Keep the bake, add a version probe that overrides it (recommended)
- **Approach:** Extend `.github/actions/setup-toolchain` with a `go-lint` input
  that compares the on-PATH `golangci-lint` / `gofumpt` against `versions.env`
  and installs the pinned version **only on mismatch** — the action's existing
  probe-by-version pattern, whose rationale ("WHY VERSION, NOT MERE PRESENCE")
  covers this case exactly. `runners/Dockerfile` keeps baking both tools.
- **Why this resolves the cutover:** the baked version is the steady-state fast
  path, and the probe is what makes a `versions.env` bump take effect *in the PR
  that makes it*. During the migration PR the fleet has v1.64.8 and
  `versions.env` says 2.13.2, so the probe installs 2.13.2 and lint runs v2
  against the v2 config — green, on the old fleet. After merge the fleet rolls,
  the baked version becomes 2.13.2, and the probe goes back to skipping.
- **Pros:** One self-consistent PR, no red window, no fleet-roll coupling — and
  permanently so for every future golangci-lint bump, not just this one.
  Simultaneously fixes the OSS hosted-runner gap (§8), where nothing is baked and
  the probe installs from scratch. Because the bake is retained,
  `runnerBakedKeys`, `check_versions.js`, `runners/docker-compose.yml` and the
  `needs runner rebuild` machinery all stay exactly as they are.
- **Cons:** A download on precisely the jobs where the versions disagree — the
  migration PR and the window before the fleet rolls. Steady-state cost is zero
  (the probe skips). A GitHub-releases outage would fail lint *during that
  window* rather than falling back to the baked copy. Adds a second install path
  for one tool, which `dependency_automation.md` §5 must document.
- **Effort:** small (one composite-action step), then the migration is small.

#### Option A3: Transitional dual config + a version-detecting shim
- **Approach:** Ship `server/.golangci.v2.yml` alongside the v1 file and have
  `lint:go:vet` call a script that picks by installed major; delete the v1 file
  after the fleet rolls.
- **Pros:** Works without touching the baked-tool model, and with one fleet roll.
- **Cons:** Two configs expressing one gate, live simultaneously — the exact
  "is this green?" ambiguity `server/.golangci.yml` was written to prevent. A
  shim script and a duplicated 19 KB of commentary, both temporary in intent and
  permanent in practice if the cleanup PR slips.
- **Effort:** medium.

### Axis B — findings triage

Per linter, applying `server/.golangci.yml`'s own rule ("Enable one only when its
findings are **fixed**, not suppressed"):

| linter | count | disposition |
|---|---|---|
| govet `inline` | 1 | fix (one line, `auth/firebase.go:31`) |
| gofumpt | 30 | config: `extra-rules` → `extra.group-params`, then `--fix`; §4 |
| staticcheck SA1019 | 15 | fix — real deprecation debt |
| staticcheck QF | 31 | **decide scope first**: `checks: ["all"]` meant stylecheck-only when written |
| noctx | 28 | fix — mechanical `*Context` conversion |
| gosec | 12 | adjudicate per site; 2 provably false, 10 need review |
| goconst | 356 → **113 strings** | phased; top-10 strings = 37%, the `*_id` family = 31% |

---

## Recommendation

**Take Option A2, and split the work into four PRs.**

The decisive argument is that A2 is the only option that removes the *class* of
problem rather than routing around this instance of it. golangci-lint is the one
baked tool that is neither self-validating (unlike Go, where `GOTOOLCHAIN=auto`
means `Go Tests` really does exercise a bump on the old image — `dependency_automation.md`
Gotchas) nor covered by the candidate gate (which runs `go build`, not lint). That
makes it uniquely able to turn `main` red after merge with no pre-merge signal,
and A1 and A3 both leave that true for the next schema break. A2 also closes the
OSS hosted-runner gap (§8) that will otherwise need its own issue.

Its real cost — a download while the pinned and baked versions disagree — is the
same mechanism the repo already accepts for Go, Node, Flutter and the JDK on any
non-baked runner, and it is zero in the steady state because the probe skips.

**One benefit not to claim for it:** this does *not* unlock Renovate auto-merge
for lint bumps. `renovate.json`'s `matchFileNames: ["versions.env"]` +
`matchUpdateTypes: ["minor","patch"]` rule already sets `automerge: true` for
them, and majors — which is what v2 is — stay manual via the global major rule
either way. That is why this bump has been sitting in "Awaiting Schedule": it is
a major, not because it is baked. What A2 changes is that the `needs runner
rebuild` label stops being misleading for this tool, and that auto-merge stops
waiting on a `Validate Runner Candidate` check which never exercised lint at all.

On Axis B, the finding I would act on hardest is that **goconst is 113 decisions,
not 356**, and that a third of it is a single question: *are SQL column-name
literals in the storage layer a goconst finding at all?* I'd answer no — column
names deriving from proto field names is the protosql pattern working as designed
— and scope goconst away from `server/storage/**` on that reasoning, stated in
the config, rather than minting 10 column-name constants that duplicate the proto
field names they already mirror. That is scoping a rule *with* a reason, which
the config's own rule permits; it is not the silent loosening the issue warns
against.

---

## Open Questions

- **Should the Go lint tools be un-baked, or should we keep the baked model and
  take three PRs?** — Recommendation: un-bake (A2). It is the only option that
  makes the next schema break a non-event, and it closes the OSS gap for free.
  If the reviewer's priority is minimizing change to CI plumbing during the OSS
  cutover, A1 is the conservative fallback and costs ~2 extra days.
- **Do SQL column-name literals count as goconst findings?** — Recommendation:
  no. Scope goconst off `server/storage/**` (or off the `*_id` shape) with the
  reasoning recorded inline. 111 of 356 findings, and constants named
  `columnCommunityID = "community_id"` add a layer without removing a failure
  mode.
- **Should `ai/eval/**` and `server/simulation/**` be excluded from goconst the
  way `_test.go` is?** — Recommendation: yes, and from `dupl`/`funlen`/`cyclop`
  too if they aren't already. They are DAMP fixture tables by role;
  `code_quality_review_prompt.md` §1's tests-are-DAMP exemption is about the role,
  not the filename suffix. Same for `auth/test_utils.go`. 104 + 6 findings.
- **Should the merged staticcheck keep `checks: ["all"]`?** — Recommendation: no,
  not verbatim. Set it to the union of what v1 actually ran (`SA*`, `S*`, `ST*`)
  and treat QF as a separate opt-in with its own 31-finding backlog. Carrying
  `["all"]` across the linter merge widens the gate by accident, which is the
  same failure mode as the `goimports.local-prefixes` typo the config already
  documents.
- **Which gofumpt owns formatting — the pinned standalone or golangci-lint's
  bundled copy?** — Recommendation: golangci-lint's, since only it enforces
  `goimports.local-prefixes`. Keep `GOFUMPT_VERSION` for `npm run fmt:go` and
  drop `lint:go:fmt`'s redundant repo-wide pass. **The two really do cover the
  same code:** there are zero tracked `*.go` files outside `server/`, so
  `gofumpt -l .` and `golangci-lint … ./server/...` see the same tracked set; the
  only difference is that `gofumpt -l .` also walks the untracked, gitignored
  `server/gen/` (which protoc emits gofmt-clean, and which the lint config
  excludes by design). The alternative — keeping both and adding a
  `check_versions.js` assertion that the bundled and pinned gofumpt agree —
  requires golangci-lint to expose its bundled version, which it does not.
- **Pin 2.13.1 or 2.13.2?** — Recommendation: 2.13.2 (current latest, released
  2026-08-27). All measurements here are on 2.13.1; re-measure on whatever is
  latest at implementation time before committing to the counts.
- **Is `run.timeout: 10m` disappearing acceptable?** — Recommendation: yes. v2
  disables the timeout by default and the `Go Tests` job already carries
  `timeout-minutes: 30`. Worth a comment in the config so the removal reads as
  deliberate.

---

## Implementation Plan

### Phase 1: Make the pinned lint version override the baked one
**Goal:** `npm run lint:go` uses the golangci-lint version in the PR's own
`versions.env`, on any runner, with no fleet-roll dependency. Still v1.64.8 —
this phase changes delivery only, so it is provably a no-op on the gate.
**Tasks:**
- [ ] Add a `go-lint` input to `.github/actions/setup-toolchain` that probes
      `golangci-lint --version` / `gofumpt --version` against `versions.env` and
      installs the pinned version **on mismatch only**, following the action's
      existing probe-and-log pattern (three outcomes logged per tool).
- [ ] Install from the upstream release tarball, matching how
      `runners/Dockerfile` already fetches Go and ONNX Runtime. golangci-lint
      publishes checksums alongside its release assets — verify them, which is
      cheap and a little better than the existing bar.
- [ ] Pass `go-lint: "true"` from `test_go.yaml`. Confirm in the job log that the
      fleet **skips** (baked version == pinned version → no-op).
- [ ] **Leave `runners/Dockerfile` baking both tools**, and leave
      `check_versions.js`'s `runnerBakedKeys`, `runners/docker-compose.yml` and
      the `ARG`/smoke-test lines untouched. The bake is the fast path; the probe
      only overrides it during a bump. Nothing in `check_versions.js` changes.
- [ ] Extend `.github/workflows/verify_toolchain.yaml` to request `go-lint` and
      assert both tools install at the pinned versions on bare `ubuntu-latest`.
      It already triggers on `.github/actions/setup-toolchain/**` and
      `versions.env`, so it covers both this PR and Phase 2's. This is the only
      place the install path gets exercised — the fleet exercises the skip path
      and nothing else, which is the exact blind spot that workflow exists for.
- [ ] If the probe's version parsing is shared with `scripts/doctor_versions.js`,
      extend that file's `node:test` units rather than duplicating the regexes.
- [ ] Document the second install path in `ripls-notes/dependency_automation.md`
      §2/§5 — a baked tool that a PR can override is a new shape in that doc.
**Validates:** that a `versions.env` lint-version change takes effect **in the PR
that makes it** — provable by bumping to any other v1.x in a throwaway commit and
watching the job log report the install, then reverting.

### Phase 2: Migrate the config, unchanged in scope
**Goal:** v2.13.2 running a config that gates exactly what v1.64.8 gated —
so any finding that appears is a v2 behaviour change, not a scope change.
**Tasks:**
- [ ] Remove `revive.min-confidence: 0`, run `golangci-lint migrate`, bump
      `GOLANGCI_LINT_VERSION`, and fix the v1 module path in any remaining
      install site to `.../v2/cmd/golangci-lint`.
- [ ] **Re-add the stripped comments by hand** — all 15.7 KB of them. The
      `--config` lesson from #1611, the funlen/cyclop/dupl ratchet ladders, the
      gosec G115 and G201/G202 audits, the govet `fieldalignment`/`shadow`
      rationale, the godot `--fix` warning, the `uniq-by-line` trap. Diff the
      pre-migration file against the new one comment-by-comment; nothing may be
      dropped silently.
- [ ] Narrow `staticcheck.checks` from `["all"]` to the v1-equivalent set so QF
      is not silently enabled (Open Question 4).
- [ ] **File the QF follow-up issue before writing the comment that defers to
      it.** CLAUDE.md's TODO rule applies to config commentary that parks work:
      an inline note needs a live `#NNNN`, and the file already carries a
      deferral pointing at **#2816, which is closed** — fix that reference in the
      same pass rather than adding a second one like it.
- [ ] Set `gofumpt.extra.group-params` in place of the deprecated `extra-rules`.
- [ ] Add a comment recording that `run.timeout` is intentionally gone.
**Validates:** the finding count drops to the v2-behaviour-change subset only —
expect goconst + SA1019 + noctx + gosec + gofumpt, and **not** the 31 QF.

### Phase 3: Clear the non-goconst backlog
**Goal:** every linter except goconst green on v2, by fixing rather than
suppressing.
**Tasks:**
- [ ] govet `inline` ×1 — `auth/firebase.go:31`.
- [ ] gofumpt ×30 — `npm run lint:go:fix` after the `group-params` change;
      confirm the standalone `gofumpt -l .` gate still agrees (Open Question 5).
- [ ] staticcheck SA1019 ×15 — migrate each deprecated API; these are real debt.
- [ ] noctx ×28 — mechanical `*Context` conversion. Thread the **existing
      startup context** into `protosql_schema.go`'s boot DDL and the two
      column-migration files rather than sprinkling `context.Background()`; the
      point of the conversion is that a shutdown mid-migration cancels, and
      `Background()` gets the finding to go away while keeping the defect.
- [ ] gosec ×12 — adjudicate per site against
      `docs/reviews/security_review_prompt.md`. The two G704s are false
      (range-checked `float64` in a `%f`); the nine G703s and the G705 need real
      review. Suppressions need `//nolint:gosec` **with an explanation** —
      `nolintlint` has `require-explanation: true` and `allow-unused: false`.
**Validates:** whether the new taint rules found anything real. If G703 surfaces
a genuine traversal in `storage/bucket.go`, that alone justifies the migration.

### Phase 4: goconst — 113 decisions, not 356
**Goal:** goconst green at `min-occurrences: 3` without hoisting literals that
should not be constants.
**Tasks:**
- [x] ~~Fix the 19 "such constant already exists" findings first~~ — **withdrawn
      after reading them.** goconst matches by value, not meaning; the suggested
      constants belong to a different vocabulary than the sites. See §2.
- [x] Scope the entity-ID / table-name / log-field vocabulary out via
      `goconst.ignore-string-values`, with the reasoning and the
      `docs/server/observability.md` citation recorded inline.
- [x] Fixture exclusions for `ai/eval/**`, `server/simulation/**`,
      `auth/test_utils.go`, the mock/e2e AI providers, and the seed/example CLI
      tools — DAMP data tables whose only distinction from `_test.go` is the
      filename.
- [ ] Triage whatever survives the above the #2820 way: ask what each repeated
      string is *evidence of* before reaching for a constant.
- [ ] Record the new baseline in the config comments, including the reporting
      change (per-string → per string-and-file) so the next reader does not
      re-derive it.
**Validates:** that the backlog really was vocabulary rather than magic values —
if a substantial residue survives the vocabulary scoping, that residue is the
real finding and deserves the #2820 treatment.

---

## Related Issues

- **#2820** (closed) — cleared goconst's previous 64-finding backlog by fixing
  the defects behind the literals. Its method is the one Phase 4 should follow,
  and its `ignore-tests` lesson is what I tested and found not to apply here.
- **#1611** (closed) — made `npm run lint:go` a required check and wrote the
  `--config` discipline that the stripped comments document. Phase 2's comment
  restoration is protecting that lesson.
- **#2816** (closed) — the funlen/cyclop/dupl ratchet ladders in the config.
  Those thresholds survive migration unchanged but their measurement notes are in
  the comments Phase 2 must restore. Note the config still cites #2816 as
  tracking future rungs, which is now a closed-issue reference.
- **#2395 / #2287 / #2883** (closed) — the `needs runner rebuild` machinery that
  Phase 1 partly retires for this tool. #2395's candidate-image gate is the thing
  that *would* have caught this and does not, because it runs `go build`.
- **#2953** (OSS release epic) — §8's hosted-runner lint gap is an OSS-readiness
  defect that Phase 1 closes as a side effect.
- **#2416** (closed) — "Renovate proposes baked-tool versions the runner image
  can't build". After Phase 1, golangci-lint leaves that failure class entirely.

---

## Implementation notes

Written after implementing the plan. Where the plan and the code disagree, the
code is right and this section says why.

**The delivery fix landed as designed.** `setup-toolchain` gained a `go-lint`
input that installs the `versions.env` pin when it differs from what is on PATH,
with the runner image still baking both tools. `verify_toolchain.yaml` exercises
the install path on bare `ubuntu-latest`, which is the only place it is
exercised at all.

**Every finding count traces to a bundled analyzer, not to v2.** The table in §4
is the useful artifact: goconst 1.7.1→1.11.0, noctx 0.1.0→0.5.1, gosec
2.22→2.28, gofumpt 0.7.0→0.11.0.

**The security rule earned the migration.** `LocalBucketStorage` joined
caller-supplied keys onto `basePath` with `filepath.Join`, which *cleans* its
result — so a key escaping the bucket resolved outside it and every method on the
type would have followed. Unreachable from today's callers, but only because
every caller happens to build keys with `MediaBucketKey`. Now guarded at all
eight sites by `resolveKey`, with a test that fails 22 assertions without it.

**Two things I got wrong and had to reverse:**

1. *Hoisting the workshop momentum CTA labels into constants broke a gate.*
   `TestCopyCorpus_IsAccountedFor` walks the package AST for **string literals**
   assigned to `Detection` fields and diffs them against a declared corpus, so
   new user-visible copy fails the build until a human lists it. Replacing the
   literals with constants made the walker find nothing and the corpus go stale
   — the copy review would have silently stopped happening. Reverted, and the
   package is excluded from goconst with that reason recorded. The duplication
   there is load-bearing.
2. *I added the `ctx` parameter to the storage initializers and then did not use
   it* — the `db.Exec`/`db.Ping` calls were left as-is. `unparam` caught it
   (`openDatabaseNoDDL - ctx is unused`), which is a good argument for that
   linter.

**goconst's shape discriminator.** Auditing all 113 strings, the property that
separated vocabulary from magic values was **shape**: lower_snake_case strings
were protosql column names, table names, log fields, template keys or RFC 7519
claim names essentially without exception, while the ones worth naming read as
prose. The config expresses that with one broad documented regex plus
`ignore-map-keys`, rather than an accreting list of individual strings. Thirteen
findings survived it, and all thirteen were prose — which is the evidence that
the discriminator is the right one.

**The 19 "already exists" findings were not cleanups.** See §2; goconst matches
by value, and the constants it proposed belong to different vocabularies than the
sites. Withdrawn.

**Follow-ups filed, then resolved in the same branch.** #3042 and #3043 are
closed by commits here; #3044 is partly done and its remainder is a separate
change.

- **#3042 (QF checks) — done.** Reviewed per check rather than wholesale.
  Enabled and fixed QF1012 (14), QF1008 (10), QF1011 (2). Left QF1001/1002/1003
  off: they are the "rewrite this control flow" family, the same trade the
  config already refuses for gocritic's `ifElseChain`.
- **#3043 (FCM `Token` → `Fid`) — done, with a caveat.** The FCM v1 discovery
  document settles the design: `token` "also accepts a Firebase Installation ID
  (FID)" but only "during the transition period", while `fid` accepts a FID and
  nothing else. So the two are not interchangeable and a rename would have
  stopped delivery. The client now reports its installation ID, the server
  stores it, and the provider sends to whichever it has. **The deprecated
  `Token` field and its `//nolint` are permanent**, not transitional: every
  device registered before this has no FID. Delivery is not verified end to end
  — that needs a real device against APNs/FCM.
- **#3044 — two of three parts done; the third was misdiagnosed.** The impact
  breakdown turned out to be read by nobody and was deleted rather than
  localized. `Provenance.Sources` turned out to be a citation list, which is not
  translated. The momentum CTA labels are the real item, and this plan's claim
  that the client could derive them from `cta_action` is **wrong** —
  `schedule_repeat` maps to both "Schedule it" and "Make it weekly". Nor is it
  only the button: the package declares 33 user-visible strings, most with
  format params. That is the #2827 structured-template migration applied to a
  new surface, and it stays in #3044.

---

## Audit

Run against the documentation context map after the first draft. Six findings,
all applied above.

1. **Phase 1 contradicted itself (significant).** The draft both removed the
   `go install` lines from `runners/Dockerfile` *and* claimed the fleet would
   skip the install because versions match. With the bake removed the probe
   finds nothing and installs on **every job on every runner**. Rewritten to
   **keep the bake and add the probe as an override**, which is strictly better:
   steady-state cost is zero, the migration window is covered, and
   `check_versions.js` / `runnerBakedKeys` / `runners/docker-compose.yml` need no
   changes at all. Option A2 renamed and its cons corrected to match.
2. **An overstated benefit (factual).** The draft claimed A2 "gives lint bumps
   the ordinary Renovate auto-merge path". `renovate.json` already auto-merges
   minor/patch `versions.env` bumps; v2 is stuck because it is a **major**, which
   the global major rule keeps manual regardless. Corrected, and the real benefit
   (an accurate label, and auto-merge no longer waiting on a check that never
   ran lint) stated instead.
3. **No test tasks (testing dimension).** The draft changed a composite action
   with no coverage. Added `verify_toolchain.yaml` extension — that workflow
   exists precisely because the fleet exercises only the action's *skip* path,
   and it already triggers on both `setup-toolchain/**` and `versions.env` — plus
   `doctor_versions.js`'s `node:test` units if the parsing is shared.
4. **Deferral hygiene (tech_debt).** Phases 2 and 4 park work in config
   comments. CLAUDE.md requires a live tracking issue for that, and
   `server/.golangci.yml` already defers to **#2816, now closed** — added a task
   to file the QF follow-up first and fix the stale reference in the same pass.
5. **Open Question 5 rested on an unverified assumption.** Confirmed there are
   **zero tracked `*.go` files outside `server/`**, so the standalone gofumpt
   pass and golangci-lint cover the same set and the "drop the redundant pass"
   recommendation is safe; noted the one real difference (`gofumpt -l .` walks
   untracked `server/gen/`). Also noted that the pin-both alternative is not
   available, since golangci-lint does not expose its bundled gofumpt version.
6. **noctx fix could enshrine the defect (error handling).** Added the
   requirement to thread the real startup context rather than
   `context.Background()`, which silences the linter while keeping the
   uncancellable migration.

**Docs consulted:** `docs/llms.txt`; anchors `docs/server/architecture.md`,
`docs/server/conventions.md`, `docs/proto_conventions.md`; review prompts
`docs/reviews/dependency_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`,
`docs/reviews/security_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`; leaves
`ripls-notes/dependency_automation.md`, `ripls-notes/infra_mimir.md`,
`docs/simulation.md`. Sources verified directly: `server/.golangci.yml`,
`versions.env`, `runners/Dockerfile`, `scripts/check_versions.js`,
`scripts/doctor_versions.js`, `scripts/runner_candidate_checks.sh`,
`renovate.json`, `oss_manifest.txt`, `package.json`,
`.github/actions/setup-toolchain/action.yaml`,
`.github/workflows/{test_go,validate_runner_candidate,verify_toolchain}.yaml`.

**Not engaged:** `api_design` (no proto/RPC change), `accessibility` and the
client anchors (no `app/lib/**` change), `performance` (CI wall-clock only,
addressed in A2's cons), `observability` (no logging/metrics surface).
