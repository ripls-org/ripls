# Issue #2933: CI: Flutter 3.47.0 segfaults flutter_tester, failing random test files on every branch

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2933
**Filed:** 2026-08-16 00:50 UTC | **Last activity:** 2026-08-16 00:50 UTC
**Labels:** bug, P1 | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (map), `docs/infra_mimir.md`, `docs/dependency_automation.md`,
`docs/client/testing.md`, `docs/testing/architecture.md` ⚓, `docs/client/conventions.md` ⚓,
`docs/reviews/testing_review_prompt.md`, `docs/reviews/security_review_prompt.md`, plus in-repo:
`.github/workflows/test_flutter.yaml`, `.github/workflows/lint_workflows.yaml`,
`scripts/runner_candidate_checks.sh`, `versions.env`, `renovate.json`, `runners/`.

> **Status: root-caused 2026-08-16** by reproduction, core-dump analysis, and controlled
> experiments on `mimir`. This supersedes the first version of this plan, which theorised
> Dart-VM heap corruption and proposed concurrency capping plus a signature-gated retry.
> That theory was wrong; the earlier draft is in git history.

---

## Problem Statement

`Flutter Tests` fails intermittently on every branch. The Dart tests pass; the job fails
because a `flutter_tester` subprocess dies with SIGSEGV, and `flutter_tools` attributes the
dead process to whichever test file that tester happened to own — so a red run names an
innocent, unrelated widget test.

The cause is **not** in this repo, and not in any test file. It is a defect in the Dart VM
profiler's native stack walker shipped in Flutter 3.47.0 (Dart 3.13.0). `flutter_tools`
starts every `flutter_tester` with `--enable-dart-profiling`, the profiler samples by
signalling the thread, and when a sample lands on code that has no frame pointer the walker
follows a garbage pointer into the heap and dereferences an unmapped page — inside the
signal handler, so the process dies immediately.

## Root Cause

### The chain

1. **Profiling is always on.** `flutter test` builds `DebuggingOptions.enabled(...)`
   (`packages/flutter_tools/lib/src/commands/test.dart`) without ever passing
   `enableDartProfiling`, so it takes the default `true`, and
   `flutter_tester_device.dart:117` appends `--enable-dart-profiling` to every tester.
   **`flutter test` exposes no flag to turn this off** — verified against `flutter test --help`
   on 3.47.0.
2. **A sample interrupts arbitrary code.** The VM samples by signalling the target thread
   (`pthread_kill` + `sigaction` are both imported by the binary). In the captured core the
   signal landed **mid-epilogue**, at `pop %rbx`, one instruction after `mov %rbx,%rax`, inside
   a `shared_ptr` move-assignment. At that instant `%rbp` is not a frame pointer — the engine
   is built without frame pointers, so it holds an ordinary value (here, a Dart heap pointer).
3. **The walker trusts that seed.** Its validation is only *monotonically increasing* and
   *below the stack ceiling*; the lower bound is seeded from the initial cursor and then
   ratchets upward. There is **no stack lower-bound check**, so a cursor sitting far below the
   stack — in the heap mapping — passes both tests and is dereferenced.
4. **Second frame in, it reads an unmapped page** → SIGSEGV *inside the signal handler* → the
   process dies. `flutter_tools` observes a dead device and reports
   `Failed to load "<test file>": Shell subprocess crashed with segmentation fault.`

### The evidence

**Core dump.** `gdb` on a captured core shows `<signal handler called>` at frame #3: frames
#0–#2 are the handler, and the interrupted PC (frame #4) is the `pop %rbx`. The walker's
context, read out of the core, is unambiguous:

| offset | value | meaning |
| --- | --- | --- |
| `+0x18`,`+0x20` | `2`, `2` | frames walked — it died on the second |
| `+0x30` | `0x7ffff05ce000` | stack upper bound (page-aligned) |
| `+0x38` | `0x5610a075926e` | interrupted PC — matches frame #4 exactly |
| `+0x48` | `0x5615c033c660` | frame cursor — **equals the faulting address** |

The context lives on the stack (`rbx = 0x7ffff05c98a8`), and the caller initialises it with
`movups %xmm0,0x20(%rsp)` (counters := 0) plus the bound and PC fields, then calls the walk
loop that faults at `+0xda`.

**Controlled experiments** (mimir, 20 cores, runner image, the real 397-file suite, arms run
concurrently so host load is identical):

| arm | Flutter | profiler | `-j` | runs | runs red | crashes |
| --- | --- | --- | --- | --- | --- | --- |
| `core`/`core2` | 3.47.0 | on (default 1 kHz) | 10 (auto) | 3 | 0 | 0 |
| `conc20-1` | 3.47.0 | on (default) | 20 | 8 | 3 | ≥4 |
| `noprof` | 3.47.0 | **stripped** | 20 | 5 | 0 | 0 |
| `hifreq` | 3.47.0 | on @ 10 kHz | 20 | 8 | **8** | **20** |
| `noprof2` | 3.47.0 | **stripped** | 20 | 8 | 0 | 0 |

Causation is established two ways: **removal** (profiling off → 0 crashes in 13 runs ≈ 5,160
tester starts) and **dose-response** (10× the sample rate → every single run red, 20 crashes
in ~3,176 starts). `hifreq` and `noprof2` ran side by side on the same host at the same
concurrency.

**Is 3.44.8 affected? No — this is a real regression.** Head-to-head on the same host, the
same 397-file suite, the same `-j 20`, and the same 10× profiler amplification, run
concurrently:

| Flutter | Dart | runs | runs red | crashes | tester starts |
| --- | --- | --- | --- | --- | --- |
| 3.47.0 | 3.13.0 | 8 | 6 | **15** | ~3,176 |
| 3.44.8 | 3.12.2 | 8 | 0 | **0** | ~3,176 |

3.44.8 ran the whole suite clean every iteration (4,370 passed, 0 failures) under the exact
conditions that make 3.47.0 crash fifteen times. Together with the fingerprint evidence, that
places the defect in the Dart 3.13.0 VM rather than in a latent path we merely started
exercising — and it keeps Option C (pin back) genuinely available.

One caveat on that comparison: resolving the app's dependencies under 3.44.8 moved 7 package
versions, since pub picked releases compatible with Dart 3.12.2. The failing code is in the
VM's profiler, not in package code, so this cannot plausibly explain a 15-vs-0 split — but it
is the only difference between the arms besides the SDK itself.

### Why it hurts so much here

One `flutter_tester` process is started per test file. The suite has **397 test files**, and
the observed per-process failure rate in CI is ~0.2%. `1 − (1 − 0.002)^397 ≈ 55%` — which is
why roughly three in five executed jobs go red while every individual test file looks fine.

### What the issue's evidence actually meant

The original diagnosis was careful and its data was sound; three inferences need correcting.

- **The constant offset `0x1533a5a` is the crash handler, not the bug site.** It never moved
  because it is one code path — the stack walker — reached from every crash. This is also why
  `addr2line` was never going to help: the shipped binary is stripped (no `.symtab`, no
  `.debug*`, 419 dynamic symbols).
- **The 46 TB `__vm_enough_memory` line is the same bogus arithmetic, not memory pressure.**
  Across all ten observed events the requested size equals `stack_pointer − a low non-stack
  address` to within 0.03–0.05%. The issue was right to rule out exhaustion; the reason is
  stronger than stated — the request is absurd by construction and would be refused on any
  host at any load.
- **The crash is at tester *startup*, not teardown.** CI logs read
  `❌ loading <file> (failed)` → `Failed to load ...`. The `finalize` frame in the issue's
  stack trace is flutter_tools' reporting path, not the failure phase.
- **Two run-history details.** `Flutter Tests` skips (and reports success) on PRs that touch no
  Flutter paths, so three of the five "passes on 3.47.0" cited in the issue were skipped jobs.
  Counting only executed jobs on the 3.47.0 fleet: 5 executions, 3 red — and the one job that
  was re-run crashed again 3 hours later, so re-running is not a reliable escape.
- **The timeline needs one fix, but the conclusion survives.** The bump merged to `main` at
  15:54:48 UTC, *after* the first two segfaults, so "5m19s after the merge" was measuring
  Renovate's commit timestamp. Version attribution instead rests on the binary fingerprint:
  the kernel prints the faulting VMA as `+102b000`, which is exactly 3.47.0's executable
  segment size; 3.44.8's is `+1006000`. All six original events were 3.47.0 binaries.

## Reproduction

The harness lives in scratch on `mimir` under `/mnt/app_cache/2933/` (workspace copies, the
loop/experiment scripts, run logs, and one core dump). It is **not** repo tooling; if it is
needed again it is cheap to recreate:

1. Copy a generated workspace (e.g. `runner-2`'s) so `flutter test` starts warm.
2. Run the suite in a loop inside `ripls-runner:latest` at `-j 20`, `--ulimit core=-1`.
3. To amplify, shim the tester artifact to add `--dart-flags=--profile_period=100`; to
   suppress, shim it to strip `--enable-dart-profiling`.

> **The core dump is secret-bearing** — it snapshots a process whose environment carries
> `BUF_TOKEN` and other CI credentials. Keep cores root-only, on the host, and delete them
> after use; publish only the derived backtrace. Host `core_pattern` was never modified —
> the stock `core` pattern writes into the crashing process's own cwd, which is inside the
> container, so `--ulimit core=-1` is sufficient.

## Potential Solutions

### Option A: Disable the tester's Dart profiler in CI
- **Approach:** A small, idempotent repo script rewrites the SDK's `flutter_tester` artifact
  into a shim that drops `--enable-dart-profiling` from its argv (keeping the real binary as
  `flutter_tester.real`), invoked from `test_flutter.yaml` and `runner_candidate_checks.sh`
  before the test step.
- **Pros:** Directly removes the trigger — 0 crashes in 13 runs / ~5,160 tester starts.
  Behaviour-neutral for us: our CI runs plain `flutter test`, which already passes
  `--disable-vm-service`, so nothing ever consumes the samples the profiler collects. Lands on
  the next CI run after merge, with no runner-image rebuild or fleet recreate. Trivially
  reverted by deleting one step.
- **Cons:** Rewrites a vendored SDK artifact — unusual, and needs a loud comment plus a
  removal criterion so it does not become permanent. Applies to CI only; a developer on 3.47.0
  locally can still hit it.
- **Effort:** Small.

### Option B: Report upstream and wait
- **Approach:** File flutter/flutter with the core analysis, the dose-response data, and a
  minimal repro; ask for the walker fix and, as an interim, a `--no-enable-dart-profiling`
  escape hatch on `flutter test`.
- **Pros:** The only real fix, and it protects every Flutter user. Our evidence is already
  most of a strong report.
- **Cons:** Lands on upstream's schedule. Does nothing for our CI today.
- **Effort:** Medium. **Not optional** — it is the exit path from Option A.

### Option C: Pin Flutter back
- **Approach:** `FLUTTER_VERSION=3.44.8` in `versions.env`, a documented Renovate hold, an
  entry under *Held-back dependencies*, then image rebuild → fleet recreate.
- **Pros:** Uses machinery that already exists, and the pin-back PR is itself proven by
  `Validate Runner Candidate` before merge.
- **Cons:** Strictly worse than Option A if A works: it gives up two weeks of toolchain, costs
  a rebuild/recreate cycle each way, and re-adoption reopens the same question. It is,
  however, **proven to work** — 3.44.8 took 8 amplified runs without a single crash — so this
  is a real contingency, not a hope.
- **Effort:** Small–Medium.

### Option D: Signature-gated retry
- **Approach:** Retry the suite when the crash signature appears.
- **Pros:** Works without understanding the cause — which is why the first draft of this plan
  reached for it.
- **Cons:** Now that the trigger can simply be removed, this is strictly worse: it doubles job
  time on affected runs, still leaves red runs when a retry also crashes (observed once in
  CI), and normalises retrying a real crash.
- **Effort:** Small. **Not recommended.**

## Recommendation

**Do A now, B immediately after, and keep C as a contingency only if A is rejected.**

Option A is the unusual-looking choice, so the reasoning matters: we are not silencing a
symptom, we are switching off a feature that this workload never uses. `flutter test` already
runs with `--disable-vm-service`; the profiler's samples are collected into a ring buffer that
nothing reads. Turning it off removes the trigger completely and measurably (13 runs, zero
crashes), costs nothing in coverage or behaviour, and is a five-line script.

Option D is explicitly dropped. A retry was defensible while the cause was unknown; it is not
defensible now that the trigger is removable.

## Open Questions

- **Where should the shim live — a repo script called from CI, or `runners/Dockerfile`?** —
  Recommendation: **repo script.** Baking it into the image is conceptually cleaner but takes
  effect only after a merge to `main`, an image build, and the daily `recreate_runners.sh`
  roll — up to a day, while CI is ~60% red. A script also keeps the hack visible in the repo
  where the removal criterion lives, and it applies to nested candidate-image runs for free.
- **Should the shim apply to `runner_candidate_checks.sh` too?** — Recommendation: **yes.**
  That gate runs the same suite inside a candidate image and would otherwise stay ~60% flaky,
  blocking every future baked-tool bump. Note the deliberate divergence in the script header,
  which mandates parity with `test_flutter.yaml` (#2395 audit finding D) — here both sides get
  the shim, so parity is preserved.
- **How do we avoid the shim outliving the bug?** — Recommendation: a `TODO(#2933):` in the
  script stating the removal criterion, plus a Phase 3 task to re-test with the shim disabled
  whenever `FLUTTER_VERSION` changes. The harness makes that a ~10-minute check.
- **Do developers need this locally?** — Recommendation: not yet. Local machines are on
  3.44.x (`npm run doctor:versions` flags the drift). Document the symptom in
  `docs/client/testing.md` so a developer who upgrades recognises it, and mention that the
  same script can be run locally.
- **Should the candidate gate run the Flutter suite more than once on a `FLUTTER_VERSION`
  bump?** — Recommendation: worth doing but demoted. A single-shot gate cannot see an
  intermittent toolchain regression — this bump auto-merged on one green candidate run at what
  we now know was a ~60% per-job failure rate. Three runs would have caught it. Cost is ~4
  minutes on a job that already builds an image and an APK.
- **Are the older upstream `--coverage` segfault reports the same defect?** —
  Recommendation: mention them in the upstream issue as probably-related rather than claiming
  it. `--coverage` keeps the VM service connected, which is the configuration that keeps the
  profiler busiest, so #128953 / #124145 / #103656 plausibly share this root cause.

## Implementation Plan

**How this ships:** one PR per phase, never a push to `main`. Gates each code-bearing phase
must clear: `Lint Workflows` (actionlint, shellcheck on `**/*.sh` via `npm run lint:shell`,
yamllint, the `.yaml`-not-`.yml` guard) for the workflow/script edits, and
`npm run lint:context-map` after doc edits. Any marker comment must be `TODO(#2933):`.

### Phase 1: Remove the trigger in CI
**Goal:** `Flutter Tests` stops failing for reasons unrelated to the code under test.
**Tasks:**
- [x] Add `scripts/disable_flutter_tester_profiler.sh`: locate the SDK's
      `bin/cache/artifacts/engine/linux-x64/flutter_tester`, move it to `flutter_tester.real`
      (skip if already done), write an argv-filtering shim in its place, `chmod +x`. Idempotent,
      atomic (write to temp, then `mv`), and **exits non-zero if it cannot shim** so a silent
      no-op can't hide the regression. Header carries `TODO(#2933):` with the removal criterion.
- [x] Call `bash scripts/disable_flutter_tester_profiler.sh` from
      `.github/workflows/test_flutter.yaml` immediately before "Run Flutter tests". *(Shipped
      as a direct `bash scripts/...` step rather than the npm wrapper this plan first proposed:
      operational scripts are invoked directly in workflows today — `build_runner_image.sh`,
      `check_web_bundle_size.sh` — while the npm layer is for lint gates. `npm run lint:shell`
      shellchecks `git ls-files '*.sh'`, so the script is linted either way.)*
- [x] Call it from `scripts/runner_candidate_checks.sh` before its `flutter test`, and record in
      that script's anti-drift header why both sides carry it.
- [x] Add a "known CI failure modes" note to `docs/infra_mimir.md` (signature → #2933 → the
      named test file is innocent), cross-linked from the "Running tests" section of
      `docs/client/testing.md`; re-run `npm run lint:context-map`.
- [x] Verify on a runner: script installs the shim, is idempotent (`.real` preserved on
      re-run), live testers carry no `--enable-dart-profiling`, and the full suite exits 0 with
      4,370 passed and 0 crashes. Gates run locally: `lint:shell`, `lint:todo-refs`,
      `lint:context-map`, `actionlint`, `yamllint` — all clean.
**Validates:** That CI red means a real test failure again.

### Phase 2: Report upstream
**Goal:** Get the walker fixed for everyone, and establish our exit criterion.
**Tasks:**
- [ ] Build a minimal public repro (a `flutter create` project plus an allocation-churning
      widget test, run with `--dart-flags=--profile_period=100`) so the report needs none of
      our code. If it will not reproduce at small scale, say so and supply the rate data
      instead.
- [ ] File the flutter/flutter issue: version/kernel, the reproduction, the dose-response
      table, the disassembly at file offset `0x1533a5a`, the walker-context field dump, and
      the observation that the walker lacks a stack lower-bound check. Backtrace only — never
      the core file.
- [ ] Note that `flutter test` offers no way to disable Dart profiling, and request one as an
      interim mitigation.
- [ ] Link the upstream issue into #2933 and into the shim script's `TODO(#2933):`.
**Validates:** That the defect is in upstream's hands with reproducible evidence.

### Phase 3: Keep the shim honest
**Goal:** The workaround leaves when the bug does.
**Tasks:**
- [ ] Add to the baked-tool bump routine (`docs/dependency_automation.md` §5): when
      `FLUTTER_VERSION` changes, re-test with the shim disabled using the harness, and record
      the result on #2933.
- [ ] When a Flutter release carries the fix: delete the script and its two call sites, close
      #2933, and remove the docs note.
- [ ] Optional, if we want the broader guard: make `runner_candidate_checks.sh` run the Flutter
      suite 3× when the PR changes `FLUTTER_VERSION`, so the next intermittent toolchain
      regression fails pre-merge instead of coin-flipping past it.
**Validates:** That we notice the fix instead of carrying the shim forever.

### Phase 4 (contingency): Pin Flutter back
**Goal:** A fallback if Phase 1 is rejected or proves insufficient.
**Tasks:**
- [ ] `FLUTTER_VERSION=3.44.8` in `versions.env`; Renovate hold on `matchDepNames: ["flutter"]`
      with `allowedVersions: "<3.47.0"`; *Held-back dependencies* entry naming the re-adoption
      criterion (upstream fix shipped).
- [ ] Confirm `Check Versions` and `Validate Runner Candidate` are green — the candidate image
      built from the pinned-back `versions.env` is the proof the app still analyzes, tests, and
      builds on 3.44.8.
- [ ] After merge, confirm the image rebuilt and the fleet actually rolled onto it (check
      `flutter --version` inside a container; the daily `recreate_runners.sh` owns the roll).
- [ ] Tell developers to re-run `npm run doctor:versions`.
**Validates:** Only run if Phase 1 is off the table.

## Related Issues
- **#2931** — where the failure was first noticed; its app changes are unrelated. Four of the
  six original crash events occurred outside it, including two on a push to `main`.
- **#2883** — "Update dependency flutter to v3.47.0", the bump that introduced this
  (merged 2026-08-14 15:54:48 UTC).
- **#2914** — the analyzer fix that unblocked #2883's candidate gate.
- **#2395** — the candidate-image validation gate; single-shot, so it could not see a ~60%
  intermittent regression. Phase 3's optional task hardens it.
- **#2287** — required-checks / `needs runner rebuild` routing for baked tools.
- **#2657** — precedent for treating a "random test failure" as a real defect rather than a
  flake to retry away. That discipline is what produced this root cause.

---

## Audit notes (`/ripls-audit`, 2026-08-16, rewrite)

**Scope:** infra/CI. No `app/lib/**` or `server/**` code, so the client/server architecture
anchors and proto conventions do not engage. Dimensions: **testing**, **dependency**,
**code_quality**, **security** (core-dump handling), **tech_debt** (the shim's removal path).

**Applied during the rewrite:**
1. **The whole solution set changed.** v1's concurrency cap and signature-gated retry were
   built on a heap-corruption theory that the core dump disproves. Concurrency only ever
   changed *exposure* (more testers → more profiler ticks), which is why capping it looked
   promising; it treats a symptom. Retry is now explicitly rejected rather than recommended.
2. **npm-script wiring** added to Phase 1: every shell gate in `test_flutter.yaml` is invoked
   as `npm run <name>`, and `lint:shell` shellchecks all tracked `*.sh`.
3. **No `.test.js` sibling required this time.** v1 mandated one because the retry logic parsed
   test output and a false negative would swallow real failures. The shim has no parsing —
   it is a file operation that either succeeds or exits non-zero — so the correctness gate is
   the fail-loud exit plus a real CI run, not a unit test.
4. **Security carried forward:** core dumps are secret-bearing (the process environment holds
   `BUF_TOKEN`); cores stay on `mimir`, only backtraces are published. Also recorded that host
   `core_pattern` was never modified, so nothing needs reverting on the host.
5. **Pin-back kept as a proven contingency** rather than a hypothesis, on the strength of the
   8-run amplified comparison.
6. **Removal path made explicit** (Phase 3) so the shim cannot quietly become permanent — the
   `TODO(#2933):` convention plus a re-test whenever `FLUTTER_VERSION` moves.

**Consistency with existing decisions:** the shim in `runner_candidate_checks.sh` is a
deliberate, documented divergence from that script's parity mandate (#2395 audit finding D) —
parity is actually preserved, since both it and `test_flutter.yaml` gain the same step. The
optional repeat-run hardening extends #2395's gate rather than weakening it.

**Docs consulted:** `docs/llms.txt`; `docs/infra_mimir.md`; `docs/dependency_automation.md`
(§2, §3, §5, *Held-back dependencies*); `docs/client/testing.md`; `docs/testing/architecture.md` ⚓;
`docs/reviews/testing_review_prompt.md` ⚓ (Areas 4–5); `docs/reviews/security_review_prompt.md`
(Area 3); `docs/reviews/dependency_review_prompt.md`; `docs/reviews/code_quality_review_prompt.md`;
`CLAUDE.md`. In-repo: `.github/workflows/test_flutter.yaml`, `.github/workflows/lint_workflows.yaml`,
`scripts/runner_candidate_checks.sh`, `package.json`, `versions.env`, `renovate.json`, and the
3.47.0 SDK's `flutter_tools` sources on the runner image.
