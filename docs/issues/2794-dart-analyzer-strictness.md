# Issue #2794: [code_quality] Dart analyzer strictness: enable strict-casts and the SDK bug-class lint rules

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2794
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** `review:code_quality` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; `docs/client/architecture.md` (⚓); `docs/client/conventions.md` (⚓);
`docs/proto_conventions.md` (⚓); `docs/client/logout.md`; `docs/client/observability.md`;
`docs/client/testing.md`; `docs/client/caching.md`; `docs/server/conventions.md` (lint-gate prose
model); `docs/context_map.md`; `docs/dependency_automation.md`; `docs/issues/1611-wave2-linters.md`;
`docs/reviews/code_quality_review_prompt.md`; `docs/reviews/tech_debt_review_prompt.md`;
`docs/reviews/error_handling_review_prompt.md`; `docs/reviews/testing_review_prompt.md`;
`docs/reviews/security_review_prompt.md`; `docs/reviews/observability_review_prompt.md`;
`docs/reviews/dependency_review_prompt.md`

## Problem Statement

`app/analysis_options.yaml` is 12 lines: `include: package:flutter_lints/flutter.yaml`, two
excludes, one rule disabled. No analyzer strictness modes, none of the bug-class lint rules
the Dart SDK ships. Across 833 non-generated Dart files that is the largest unlinted surface
in the repo — the Dart counterpart to the Go gate that #1611 just closed (PR #2817).

The issue was filed with a measurement table carried over from `docs/issues/1611-wave2-linters.md`
(measured 2026-07-28) and a suggested six-step order. **I re-measured everything on 2026-07-30
against `main` with generated code present.** The totals are close, but four of the
characterizations that drive the suggested order are wrong, and one of the two questions the
issue says it owns rests on a false premise. Those corrections are what reshape the plan.

### Re-measurement, 2026-07-30

Method: back up `app/analysis_options.yaml`, enable one candidate group at a time, run
`flutter analyze` from `app/`, parse by rule and by area. Flutter 3.44.8 / Dart 3.12.0,
generated code present (`npm run generate`). Raw output in the scratchpad; every number below
is reproducible with the same procedure.

**Analyzer strictness modes** (each measured alone, `flutter_lints` baseline):

| Mode | Findings | Severity | lib / test | Diagnostic codes |
|---|---:|---|---|---|
| `strict-raw-types` | **2** | warning | 2 / 0 | `strict_raw_type` |
| `strict-casts` | **115** | 114 error, 1 warning | 114 / 1 | `argument_type_not_assignable` 90, `non_bool_condition` 9, `return_of_invalid_type` 4, `invalid_assignment` 4, `non_bool_operand` 3, `list_element_type_not_assignable` 2, `for_in_of_invalid_type` 2, `invalid_return_type_for_catch_error` 1 |
| `strict-inference` | **175** | warning | 89 / 86 | `inference_failure_on_instance_creation` 82, `on_function_invocation` 74, `on_function_return_type` 15, `on_untyped_parameter` 4 |

**Lint rules** (all enabled together, one pass — 1,631 findings, **all info severity**):

| Rule | Total | lib | test | `dart fix` clears it? |
|---|---:|---:|---:|---|
| `directives_ordering` | 511 | 382 | 129 | yes (252 file-level fixes) |
| `discarded_futures` | 382 | 320 | 62 | no |
| `prefer_int_literals` | 253 | 124 | 129 | yes |
| `avoid_dynamic_calls` | 229 | 229 | 0 | no |
| `unawaited_futures` | 73 | 69 | 4 | no |
| `use_if_null_to_convert_nulls_to_bools` | 71 | 70 | 1 | yes |
| `unnecessary_await_in_return` | 34 | 31 | 3 | yes |
| `cast_nullable_to_non_nullable` | 24 | 0 | 24 | **yes — all 24** |
| `avoid_positional_boolean_parameters` | 19 | 18 | 1 | no |
| `prefer_final_locals` | 9 | 5 | 4 | yes |
| `close_sinks` | 7 | **0** | 7 | no |
| `unnecessary_parenthesis` | 5 | 4 | 1 | yes |
| `only_throw_errors` | 5 | 3 | 2 | no |
| `unnecessary_statements` | 3 | 0 | 3 | no |
| `sort_pub_dependencies` | **2** | — | — | no (`pubspec.yaml`) |
| `cancel_subscriptions` | 1 | 1 | 0 | no |
| `one_member_abstracts` | 1 | 1 | 0 | no |
| `prefer_constructors_over_static_methods` | 1 | 1 | 0 | no |
| `always_declare_return_types` | 1 | 0 | 1 | yes |

Still zero, enable free: `missing_whitespace_between_adjacent_strings`, `test_types_in_equals`,
`throw_in_finally`, `avoid_slow_async_io`, `no_adjacent_strings_in_list`,
`use_late_for_private_fields_and_variables`, `join_return_with_assignment`,
`avoid_returning_null_for_void`. That is **8** free rules, not 9 — `sort_pub_dependencies`
has drifted to 2 findings in `app/pubspec.yaml`.

### Six corrections that change the plan

**1. `flutter analyze` already treats infos as fatal. The gate is not decorative.**
This is the issue's first "question this issue owns," and the premise is false.
`flutter analyze --help` on Flutter 3.44.8: `--[no-]fatal-infos … (defaults to on)`.
Verified empirically — with the candidate rules enabled, `flutter analyze` exited **1** on
1,631 findings that were **100% info severity**. (The confusion is understandable:
`dart analyze` defaults infos to non-fatal; `flutter analyze` does not.)

*Consequence, and it is the opposite of what the issue assumed:* there is no advisory mode to
land a rule into. Every rule flip must ship green in the same PR as its fixes. No change to
`.github/workflows/test_flutter.yaml` is needed at any phase — the `flutter analyze` step at
line 93 already enforces whatever `analysis_options.yaml` says.

**2. `strict-casts` and `avoid_dynamic_calls` are one defect in one file set, not two steps.**
The issue orders them as steps 4 and 6. Measured: `strict-casts` touches 22 files,
`avoid_dynamic_calls` touches 21, and **18 are the same file**. Union: **25 files carry all
343 findings.** They are two symptoms of the same cause — `dynamic` written by hand in a
signature to avoid an import:

```dart
// app/lib/presentation/screens/directory/directory_screen.dart:491
final dynamic l10n;                          // → 11 strict-casts + 11 avoid_dynamic_calls
// app/lib/presentation/screens/request/widgets/request_read_shell.dart:460
dynamic need, List<dynamic> claims,          // → 11 + 26
// app/lib/presentation/screens/settings/settings_hub_screen.dart:82
Widget _buildAccountSection(BuildContext context, dynamic authState)
```

`dynamic l10n` alone appears at **13 sites** across 8 files; every one of them should be
`AppLocalizations`. This is not inference leakage the analyzer is being pedantic about — it is
a type hole someone typed on purpose. Splitting the fix across two PRs means touching the same
25 files twice and guarantees a conflict between them.

It is also an *active* regression source, not a historical one: the `dynamic l10n` in
`directory_screen.dart` was introduced 2026-07-07. The lint is the thing that stops the next one.

**3. `unawaited_futures` and `discarded_futures` do not overlap. At all.**
The issue calls `discarded_futures` "real (overlaps `unawaited_futures`)". Measured overlap of
the 73 and 382 site lists: **0**. They are complementary by construction — `unawaited_futures`
fires on unawaited futures inside `async` bodies, `discarded_futures` fires inside **non-`async`**
functions and closures. Combined they are 455 sites in 199 files, not 382 with 73 inside it.

They also differ sharply in value. `unawaited_futures` (73, in `async` bodies) is where a
missing `await` is an actual ordering bug — and the codebase already agrees: there are **8
hand-written `// ignore: unawaited_futures` directives in `app/lib`** for a rule that has never
been enabled. `discarded_futures` (382) mostly fires on code that is correct as written:

```dart
lib/main.dart:53    trace.stop();                                  // Firebase perf, fire-and-forget by design
lib/main.dart:242   FirebaseCrashlytics.instance.recordFlutterFatalError(details);   // inside sync FlutterError.onError — cannot be awaited
lib/main.dart:301   future.catchError((Object e, StackTrace st) { … });              // the error IS handled
```

The "fix" for those is wrapping in `unawaited(...)`, which changes nothing and buries the 73
real signals under 382 units of ceremony.

**4. `cast_nullable_to_non_nullable` and `close_sinks` are not the leak class the issue frames.**
- `cast_nullable_to_non_nullable`: 24 findings, **all in `app/test/`**, and **`dart fix` clears
  all 24 mechanically**. The issue puts it in "Bug/leak class — fix by hand."
- `close_sinks`: 7 findings, **all in `app/test/`** — unclosed `StreamController`s in test
  fixtures (`conversation_view_model_test.dart` ×5, two others). Real hygiene, zero production
  impact; the fix is `addTearDown(controller.close)`.
- `cancel_subscriptions`: the single finding is a **false positive**.
  `app/lib/data/streaming/gen_stream_controller.dart:26` does cancel, in both `cancel()` and
  `dispose()` — but via `final sub = _subscription; await sub?.cancel();`, and the lint cannot
  trace through the local copy. Enabling it costs one `// ignore:` with a rationale comment.

So the "leak class" bucket the issue describes as 5 rules / 111 findings is really:
73 production fire-and-forget sites (`unawaited_futures`), 3 production `only_throw_errors`,
31 test-hygiene findings, and 1 false positive.

**5. The `dart fix --apply` sweep is bigger than stated and *not* semantically safe.**
Measured by actually running it: **653 fixes in 346 files**, clearing **908** analyzer findings
(`directives_ordering` 511, `prefer_int_literals` 253, `use_if_null…` 71,
`unnecessary_await_in_return` 34, `cast_nullable_to_non_nullable` 24, `prefer_final_locals` 9,
`unnecessary_parenthesis` 5, `always_declare_return_types` 1). Diff: 345 files,
+1,203 / −1,201.

It also **introduced a compile error**:

```diff
  // app/lib/presentation/widgets/navigation/nav_dock.dart:409
    final maxLabel = math.max(
-     0.0,
+     0,
        slotsWidth - (…) * _minSlotWidth - _pillChrome,
      );
```

`prefer_int_literals` rewrote `math.max(0.0, x)` → `math.max(0, x)`, which makes `math.max<T>`
infer `num` instead of `double` and fails with
`error • The argument type 'num' can't be assigned to the parameter type 'double'`. The issue
calls this step "near-zero semantic risk." It is one hand-fix, but it means the sweep needs
`flutter analyze` **and** `flutter test` afterward, not a rubber stamp.

*(With that one line reverted, the full Flutter suite passes on the swept tree —
`flutter test` → **4,295 passed, 1 skipped, 0 failed**. So the sweep is behavior-preserving; it
is just not *self-verifying*.)*

**6. `avoid_positional_boolean_parameters` contradicts our own reviewed standard.**
The issue asks whether it is worth the churn. Measured answer: no. 17 of the 19 findings are
single-bool setters on Riverpod notifiers —

```dart
class ContentExpanded extends _$ContentExpanded {
  @override bool build() => false;
  void set(bool value) => state = value;   // ← flagged
}
```

— and `docs/reviews/code_quality_review_prompt.md` §11 says explicitly: *"Flag any function with
2+ bool params… Single-bool parameters are tolerable."* The rule flags positional bools
regardless of count, so adopting it means 19 renames that our own code-quality standard does not
ask for.

## Context from Comments

**The issue has no comments.** It was filed 2026-07-29 by `/ripls-reviews` as a split-out of
#1611 and nothing has been added since — so there is no maintainer direction beyond what #1611
already recorded, and no counter-argument on record.

That is less of a gap than it looks, because the real discussion happened on #1611 and is
preserved in `docs/issues/1611-wave2-linters.md`:

- **Decided 2026-07-28 (Bryan):** the Dart work does not ship in #1611's PR — different
  reviewer, different CI workflow, no shared code. #2794 exists because of that decision.
- **Decided 2026-07-28 (Bryan), and it transfers directly here:** for Go, *"an advisory gate
  that nobody reads is how we got here; growing a green required gate beats declaring a red one
  and muting it."* Correction 1 above means Dart gets that property for free — `flutter analyze`
  is already required and already fatal-on-info.
- **#1611's own audit found that its first draft mis-sized `nolintlint` by 26×** because it
  measured with default settings. The same failure mode is what corrections 1–6 are: this issue's
  table was inherited, not re-derived.
- The one residual Dart touch from #1611 — `scripts/check_todo_refs.js` scanning
  `app/lib/**/*.dart` — has already landed (`npm run lint:todo-refs`, wired into
  `test_flutter.yaml:205`). It is comment scanning, not analysis, and does not interact here.

## Potential Solutions

### Option A: Group by defect — four PRs, decline three rules
- **Approach:** Reorganize the issue's six rule-ordered steps into four defect-ordered PRs:
  (1) strictness floor + the free rules; (2) the `dart fix` mechanical sweep; (3) de-`dynamic`
  the 25 files, which lands `strict-casts` **and** `avoid_dynamic_calls` together;
  (4) the fire-and-forget audit (`unawaited_futures` + `only_throw_errors` + test hygiene).
  Explicitly decline `discarded_futures`, `avoid_positional_boolean_parameters`, and
  `strict-inference`, each with the measurement that justifies it.
- **Pros:** Each of the 25 dynamic-typed files is touched exactly once. Every PR is a coherent
  review unit ("give these widgets real types") rather than a rule-shaped grab bag. Final rule
  set is 100% defensible against `docs/reviews/code_quality_review_prompt.md`. Drops ~576
  findings of measured-low-value churn.
- **Cons:** PR 3 is the biggest single review (25 files, real type decisions) and can't be
  meaningfully subdivided without re-splitting the same files. Declining three rules is a
  judgment call a reviewer may want to overturn.
- **Effort:** Medium. PR 1 ≈ 1h, PR 2 ≈ 2h (mostly test-suite wall clock), PR 3 ≈ 1 day,
  PR 4 ≈ 1 day.

### Option B: Rule-at-a-time, exactly as the issue orders it
- **Approach:** Six PRs following the issue's suggested order verbatim, all 19 rules plus all
  three strictness modes.
- **Pros:** Trivial bookkeeping — one rule flip per PR, each independently revertable. Zero
  interpretation of the filed issue.
- **Cons:** Steps 4 and 6 rewrite the same 25 files, so the second inherits a guaranteed
  conflict with the first (and neither is complete on its own — typing `dynamic l10n` fixes
  `strict-casts` and `avoid_dynamic_calls` in one edit, so the split is artificial).
  `discarded_futures` (382) and `strict-inference` (175) are 557 findings of ceremony with no
  measured defect behind them. `avoid_positional_boolean_parameters` contradicts §11 of our own
  review standard.
- **Effort:** Large — roughly double Option A for the same defects closed.

### Option C: Flip everything now, remediate behind an `// ignore_for_file:` ratchet
- **Approach:** One PR enables all rules and all strictness modes, adds
  `// ignore_for_file: <rules>` headers to the ~437 offending files, and adds a CI ratchet that
  counts the directives and fails if the count grows — the pattern already used for
  `scripts/a11y_allowlist.txt` (`test_flutter.yaml:107`) and the color allowlist.
- **Pros:** The gate is live on day one; nothing new can regress while remediation proceeds.
  Matches an enforcement idiom this repo already runs twice.
- **Cons:** `app/lib` currently carries **20** hand-written ignore directives outside generated
  code. This adds ~437 file headers — a 20× increase in suppression, and every one of them
  ages into background noise. The two existing ratchets gate a *single* dimension with a
  *flat file* of paths; a multi-rule per-file header set is materially harder to read and to
  shrink. It also front-loads zero actual bug fixes.
- **Effort:** Small to land, large and open-ended to unwind.

## Recommendation

**Option A.** Three things decide it.

*The gate is already fatal*, so the usual reason to reach for Option C — "get enforcement on
before the cleanup finishes" — does not apply. There is no window where a rule is on but
unenforced; a rule is either green or CI is red. That makes suppression-first strictly worse
here than it is in the a11y and color cases, where the ratchet buys a real property we couldn't
otherwise have.

*The 25-file dynamic cluster is the whole prize.* 343 of the 1,746 findings, in 25 files,
from one root cause, actively regressing (July 2026). It is the only part of this issue that
closes a class of latent runtime `NoSuchMethodError`s rather than reformatting. Option B splits
it in half for bookkeeping reasons; Option A does not.

*The declines are what make the rest defensible.* `discarded_futures` fires on
`FirebaseCrashlytics.recordFlutterFatalError` inside a synchronous `FlutterError.onError`
handler — code that physically cannot await. `avoid_positional_boolean_parameters` asks for 19
renames that `code_quality_review_prompt.md` §11 explicitly says are fine. `strict-inference` is
175 type arguments with zero measured defects behind them. Adopting those three would mean 576
findings of churn, and the reviewer who has to sign off on `unawaited(trace.stop())` 382 times
is the reviewer who stops trusting the gate.

Net rule set: **13 lint rules + 2 strictness modes**, closing **1,170** findings, of which 908
are mechanical and 262 are hand work concentrated in 25 + 36 files.

What would change this: if the intent is a hard "no `dynamic` anywhere, ever" posture, then
`strict-inference` belongs in scope after all and Phase 3 should absorb it (it is the same
files, mostly). Flagged as an open question rather than assumed.

## Open Questions

- **Should `test_flutter.yaml` add `flutter analyze --fatal-infos`?** — Recommendation: **no,
  and remove the question from the issue.** Verified on Flutter 3.44.8: `--fatal-infos` defaults
  to **on** for `flutter analyze`, and an all-info run exits 1. The workflow needs no edit at
  any phase. Worth one sentence in `docs/client/conventions.md` so the next person does not
  re-ask.
- **Is `avoid_positional_boolean_parameters` (19) worth the churn?** — Recommendation: **no.**
  17 of 19 are single-bool setters, which `docs/reviews/code_quality_review_prompt.md` §11
  explicitly tolerates. Adopting it puts the lint config in conflict with the review standard.
- **`discarded_futures` (382) — decline outright, or defer to its own issue?** —
  Recommendation: **decline in this issue, file a follow-up** that decides the prior question
  ("when is `unawaited(...)` documentation and when is it noise?") before enabling anything. The
  rule is not wrong, but 382 sites is a policy decision about Flutter idiom, not a lint flip.
- **`strict-inference` (175) — in or out?** — Recommendation: **out, follow-up issue.** Purely
  mechanical (add type arguments), zero defect yield measured, and 86 of 175 are in `app/test/`.
  Revisit once Phase 3 has removed the hand-written `dynamic`; the remaining count will be lower
  and the value clearer.
- **Should `app/test/` get its own `analysis_options.yaml` with a relaxed rule set?** —
  Recommendation: **no.** Nested options files are supported, but every test-side finding in the
  adopted set is either `dart fix`-clearable (24 + 129 + 129 + …) or a genuine fixture leak
  (`close_sinks` ×7). Splitting the config would cost a permanent second file to save about a
  day of one-time work, and `docs/client/testing.md` treats `app/test/` as first-class.
- **When does the 345-file sweep land?** — Recommendation: **immediately after a release cut, as
  its own PR, merged same-day.** It rewrites import blocks in 252 files; every hour it sits open
  it conflicts with something. Do not bundle it with Phase 1.
- **Does `// ignore: cancel_subscriptions` need a tracking issue?** — Recommendation: **no
  issue, but a rationale comment.** It is a lint limitation (cancellation through a local
  variable), not deferred work — `TODO(#NNNN)` per CLAUDE.md is for things that should change,
  and this one should not.
- **Does the lint baseline need pinning, the way #1611 pinned `golangci-lint`?** —
  Recommendation: **no action, but know where the exposure is.** #1611 flagged that
  `runners/Dockerfile` installs `golangci-lint@latest`, so a runner rebuild could turn a green
  branch red with no repo change. Dart has two analogous inputs and both are already pinned:
  the analyzer ships with the SDK, versioned by `FLUTTER_VERSION=3.44.8` in `versions.env`
  (Renovate-tracked via `datasource=flutter-version`), and `flutter_lints: ^6.0.0` is a caret
  range that is lock-pinned by the committed `app/pubspec.lock`. Neither moves without a
  deliberate PR. What does change after this issue: once 15 rules are named explicitly, a
  Renovate bump to either input becomes a lint-config change, and because the gate is
  fatal-on-info it will surface as a red PR rather than as silent drift. That is the desired
  behavior — just don't diagnose it as a flaky runner.

## Implementation Plan

Each phase is one PR. Phases 1–2 are independent; 3 and 4 are independent of each other but both
want 2 landed first (the sweep rewrites imports in files they touch).

### Phase 1: Strictness floor + the free rules
**Goal:** `analysis_options.yaml` stops being a stub, at zero remediation cost, and the two rules
that cost nothing today can never regress.
**Tasks:**
- [ ] `analyzer.language`: `strict-raw-types: true`. Fix both findings —
      `lib/services/community_event_stream.dart:19` (`StreamSubscription<dynamic>` →
      explicit type argument) and the second `strict_raw_type` site.
- [ ] Enable the 8 zero-finding rules: `missing_whitespace_between_adjacent_strings`,
      `test_types_in_equals`, `throw_in_finally`, `avoid_slow_async_io`,
      `no_adjacent_strings_in_list`, `use_late_for_private_fields_and_variables`,
      `join_return_with_assignment`, `avoid_returning_null_for_void`.
- [ ] Enable `sort_pub_dependencies` and sort the two out-of-order blocks in
      `app/pubspec.yaml` (lines 13 and 81). *(The issue lists this as zero-finding; it has
      drifted to 2.)*
- [ ] Enable `unnecessary_statements` (3, all in
      `test/presentation/viewmodels/experience_view_model_test.dart`),
      `one_member_abstracts` (1, `lib/presentation/viewmodels/unified_create_streaming_actions.dart:68`),
      `prefer_constructors_over_static_methods` (1, `lib/presentation/widgets/navigation/nav_dock.dart:469`).
- [ ] Structure `analysis_options.yaml` so it stays readable at 25+ rules: one comment block per
      group stating *why* the rule is on, in the style of `server/.golangci.yml`.
- [ ] `npm run lint:dart` green; `cd app && flutter test --reporter expanded` green.
**Validates:** That the config can carry rules at all, and that `flutter analyze`'s
fatal-on-info behavior is what Correction 1 measured — a deliberately broken rule should turn CI
red with no workflow edit.

### Phase 2: The `dart fix` mechanical sweep
**Goal:** 908 findings gone in one mechanical, reviewable-by-shape commit, before anything
hand-written touches the same files.
**Tasks:**
- [ ] Enable `directives_ordering`, `prefer_int_literals`,
      `use_if_null_to_convert_nulls_to_bools`, `unnecessary_await_in_return`,
      `cast_nullable_to_non_nullable`, `prefer_final_locals`, `unnecessary_parenthesis`,
      `always_declare_return_types`.
- [ ] `cd app && dart fix --apply` → expect **653 fixes in 346 files** (345 tracked;
      +1,203/−1,201).
- [ ] **Re-run `flutter analyze` and expect it to fail.** `prefer_int_literals` rewrites
      `math.max(0.0, …)` → `math.max(0, …)` at
      `lib/presentation/widgets/navigation/nav_dock.dart:409`, which infers `num` and breaks the
      `double` parameter. Revert that literal by hand. Scan the rest of the
      `prefer_int_literals` diff for the same shape (generic call sites where the literal drives
      inference) — this was the only one in this tree, but the class is real.
- [ ] `flutter test` in full. Do not skip: this is 345 files of automated rewrites, and Phase 2
      is the one phase where a silent behavior change is plausible.
- [ ] Merge same-day. 252 of the 345 files are import-block rewrites; this conflicts with
      everything in flight.
**Validates:** That `dart fix --apply` converges (measured: those 8 rules go 908 → 0 in one
pass, no second run needed) and that the rewrite is behavior-preserving — with the one
`nav_dock.dart` literal restored, the full suite is **4,295 passed / 1 skipped / 0 failed** on
the swept tree.

### Phase 3: De-`dynamic` the 25 files (`strict-casts` + `avoid_dynamic_calls`)
**Goal:** The highest-value step in the issue. Close 343 findings by deleting hand-written
`dynamic` from 25 files, and make the pattern un-reintroducible.
**Tasks:**
- [ ] Replace the 13 `dynamic l10n` parameters/fields with `AppLocalizations` (import
      `package:ripls/l10n/app_localizations.dart`). Files: `directory_screen.dart:491`,
      `experience_rsvp_composer_sheet.dart` (×6), `request_contribution_composer_sheet.dart` (×2),
      `needs_claim_gear_picker.dart` (×2), `experience_read_shell.dart:661`,
      `content_impact_row.dart:116`.
- [ ] Type the remaining `dynamic` signatures against their real proto/model types:
      `request_read_shell.dart:460` and `request_helpers_panel.dart:446` (`dynamic need`,
      `List<dynamic> claims`, `dynamic myClaim`), `gear_details_pane.dart:170,288`
      (`dynamic metadata`), `settings_hub_screen.dart:82` (`dynamic authState`),
      `request_metric_view_model.dart:102` and `experience_metric_view_model.dart:102`
      (`Future.wait<dynamic>` → a typed record or sequential awaits).
- [ ] Work the union file list, not the two rule lists — 18 of 25 files carry both diagnostics
      and one edit clears both. Remaining 7: `firebase_analytics_provider.dart`,
      `firebase_crash_reporter.dart`, `community_selection_sheet.dart`,
      `viewer_profile_view_model_test.dart` (`strict-casts` only);
      `request_compose_view_model.dart`, `content_impact_row.dart`,
      `mapbox_location_service.dart` (`avoid_dynamic_calls` only, 25 of them in the Mapbox JSON
      decode path — that one wants a typed response model, not a cast).
- [ ] **Treat the two `core/observability/firebase/` sites as a separate, careful change.**
      They are not incidental: `firebase_crash_reporter.dart:179` is
      `_crashlytics.setCustomKey(key, redactedValue)` where `redactedValue` is the `dynamic`
      return of `_redactCustomKeyValue`, and `firebase_analytics_provider.dart:183` is the
      `sanitized[key] = value` line of the analytics value sanitizer. Both sit **inside the PII
      redaction path** that [`docs/client/observability.md`](../client/observability.md)
      §"PII Protection" describes as the guarantee that *"PII never reaches external services
      regardless of what application code logs"* — the same property
      `security_review_prompt.md` §5 ("Sensitive Data: check for PII in local storage or logs")
      and `observability_review_prompt.md` §7 check by hand. Retyping the redactor's return to
      `Object` must not widen what reaches Crashlytics. Re-run
      `app/test/core/observability/` (including `firebase/`) as a named gate on this task, not
      as part of the general suite run.
- [ ] Then flip `analyzer.language.strict-casts: true` and `avoid_dynamic_calls: true`.
- [ ] `npm run lint:dart:size` (#1424). Five of the target files are within ~160 lines of the
      1,000-line cap — `needs_claim_gear_picker.dart` 941, `request_helpers_panel.dart` 906,
      `gear_interest_panel.dart` 897, `request_read_shell.dart` 883,
      `experience_read_shell.dart` 854 — and this phase adds imports and longer type names to
      all of them. If one crosses, **split it** per `docs/client/conventions.md` §"Flutter
      Screens"; do not add a `// dart-line-count-allow` hatch. CLAUDE.md requires a tracking
      issue for the hatch, and spending one on a retyping PR is how
      `needs_actions.dart:1` ended up with an issue-less `// dart-line-count-allow: pushed over
      by needs-v2 additions`.
- [ ] `flutter test` — this phase changes real signatures, so viewmodel and widget tests are the
      signal.
**Validates:** That 343 findings across two rules collapse to zero from a single pass over 25
files — i.e. that the root-cause grouping in Correction 2 holds. Also proves the class is
closed: with both rules on, a future `dynamic l10n` fails CI at the point it is written.

### Phase 4: Fire-and-forget audit + test-fixture hygiene
**Goal:** The one genuinely bug-shaped rule in the set, plus the small leftovers.
**Tasks:**
- [ ] Enable `unawaited_futures`. Triage all 73 sites **by hand** — this is the phase where a
      mechanical `unawaited(...)` sweep would destroy the entire point. For each: is the missing
      `await` an ordering bug, or is fire-and-forget correct here?
      `docs/client/architecture.md` §"Async-First Architecture" is the standard to judge against
      — *"All data access is async. There are no synchronous repository methods"* — so an
      unawaited repository call in a ViewModel is presumptively a bug, while an unawaited
      analytics or navigation side effect is presumptively fine.
- [ ] Cross-check every site in `app/lib/services/auth_state.dart` and anything in the logout
      path against [`docs/client/logout.md`](../client/logout.md). That doc specifies a strict
      5-step teardown ordering — *"pollers/streams stop before caches clear"* — and an unawaited
      step there is exactly the defect `unawaited_futures` exists to catch.
- [ ] Re-examine the **8 pre-existing `// ignore: unawaited_futures` directives** in
      `app/lib/presentation/viewmodels/` (`experience_view_model.dart:433`,
      `request_edit_actions.dart:239,285`, `story_view_model.dart:99`,
      `gear_view_model.dart:449`, `community_view_model.dart:118`,
      `request_view_model.dart:272,276`). They were written against a rule that was never on;
      confirm each is still deliberate and give it a one-line rationale, or fix it.
- [ ] Enable `only_throw_errors` (5). The 3 production sites are in
      `lib/presentation/viewmodels/gear_sharing_transfer_actions.dart:35,90,97` — check them
      against `docs/reviews/error_handling_review_prompt.md` §2 (*"Verify Future and Stream
      errors are handled"*, *"error messages are user-friendly, not stack traces"*) before
      choosing between a typed exception and a rethrow.
- [ ] Enable `close_sinks` (7, all test) — add `addTearDown(controller.close)` in
      `conversation_view_model_test.dart` (×5), `conversation_messages_ordering_test.dart`,
      `request_creation_view_model_test.dart`. This mechanizes
      `docs/reviews/testing_review_prompt.md` §4 (*"Setup/Teardown — verify proper test setup
      and cleanup"*), which is currently a hand-review criterion.
- [ ] Enable `cancel_subscriptions` (1) with a single
      `// ignore: cancel_subscriptions` at `lib/data/streaming/gen_stream_controller.dart:26`
      plus a comment recording *why* it is a false positive (cancellation goes through a local
      copy in both `cancel()` and `dispose()`, which the lint cannot trace). No tracking issue —
      nothing here should change.
- [ ] Update [`docs/client/conventions.md`](../client/conventions.md) with an "Analyzer and lint
      rules" section, mirroring the structure `docs/server/conventions.md` §"Linting" already
      uses for Go: what the whole gate is, that it is the same command CI runs, the adopted set
      and why, the three declines with their measurements, and the trap — **`dart analyze` and
      `flutter analyze` disagree on `--fatal-infos`** (off and on respectively), so a local
      `dart analyze` can come back clean on a tree that fails CI. That is the Dart analogue of
      the `golangci-lint run ./...` config-discovery trap already documented on the Go side.
      Re-run `npm run generate:context-map`.
      *(#1611 Phase 6 proposed a standalone cross-repo "Lint gates" table; it did not land, and
      per-side prose is the pattern that actually exists. Follow the pattern, not the proposal.)*
- [ ] **Keep the plan's phase numbers out of the code.**
      `scripts/check_ephemeral_comments.js` (`npm run lint:ephemeral-comments`, wired into
      `test_flutter.yaml:200`) fails the build on `\bphase\s*\d+\b` in any `.dart` or `.go`
      comment under `app/lib/` or `server/`. Every rationale comment this issue adds — the
      `cancel_subscriptions` suppression, the `unawaited_futures` annotations — must reference
      **#2794**, never "Phase 3"/"Phase 4". (`_test.dart` is exempt from that gate, but write
      them the same way anyway.) See `docs/client/conventions.md` §"Dart comments".
- [ ] File the two follow-up issues: `discarded_futures` (382) and `strict-inference` (175),
      each carrying its measurement and the open question that gates it.
**Validates:** That the 73 `unawaited_futures` sites resolve into a small number of real ordering
fixes plus deliberate, annotated fire-and-forget — and that the logout teardown contract in
`docs/client/logout.md` is now machine-checked rather than doc-checked.

## Related Issues

- **#1611** — Go Wave 2 linters + lint enforcement (closed by PR #2817). Parent; this issue was
  split out of it on 2026-07-28. Its plan doc `docs/issues/1611-wave2-linters.md` holds the
  original Dart measurement, the *Out of Scope* hand-off, and the "grow a green required gate"
  decision that Correction 1 shows Dart already satisfies.
- **#1514 / #1605** — Wave 1 Go linters. Context only; no shared code.
- **#2816** — the Go linters parked out of #1611 (`funlen`, `cyclop`, `dupl`, `goconst`).
  Structurally the same problem on the other side: measured, deferred, needs a ladder.
- **#1424** — Dart/Go file-size gates. Phase 3 retypes signatures in large presentation files;
  run `npm run lint:dart:size` alongside.
- **#2020** — the `TODO(#NNNN)` convention CLAUDE.md cites, relevant to the
  `cancel_subscriptions` suppression decision (rationale comment, not a TODO).
- **New issues to file in Phase 4:**
  - `discarded_futures` (382 sites, 163 files) — needs the `unawaited(...)` policy decided first.
  - `strict-inference` (175 findings, 89 lib / 86 test) — revisit after Phase 3.

---

## Implementation Notes (2026-07-30)

**Shipped as one PR** on branch `dart_lint`, at the user's direction, rather than
the four PRs the plan proposes. All four phases landed as four commits in that
branch. Final state: **22 lint rules + 2 strictness modes on, `flutter analyze`
clean, full Flutter suite green.**

Where the implementation departed from the plan:

1. **`one_member_abstracts` was dropped from the adopted set** (plan Phase 1
   listed it as a free 1-finding win). Its only finding is
   `UnifiedCreateStreamingActions` — a deliberate single-method test seam whose
   own doc comment says *"Tests substitute a fake"*, and which
   `docs/client/testing.md` builds on. Enabling it would have bought one
   suppression on working code rather than preventing anything, which is the
   opposite of the standard the rest of the set is held to. Recorded in
   `analysis_options.yaml` alongside the three planned declines. Adopted total is
   therefore 22 rules, not 23.
2. **`sort_pub_dependencies` cost more than the two flagged lines.** Sorting
   `app/pubspec.yaml` moved 15 dependencies, because the file had drifted well
   past the two positions the analyzer names (it reports the first offender per
   block, not every one).
3. **The `nav_dock.dart` regression reproduced exactly**, and is now pinned with
   `math.max<double>(...)` plus a comment naming the rule, so the next
   `dart fix` run cannot reintroduce it.
4. **Phase 3 came in under budget.** The 25-file list held: no file outside it
   needed a type, and `dart fix --code=unnecessary_cast` cleared 6 casts that
   became redundant once the real types were in place. Two sites needed more
   than a type name — `mapbox_location_service` got `_features()` /
   `_contextField()` helpers so its three geocode paths stop indexing into
   `dynamic`, and `needs_actions._surfaceMutationError` now takes the
   `UserError` instead of a whole notifier state (13 call sites).
5. **The audit's file-size warning came true.** `unawaited_futures` pushed
   `experience_time_panel.dart` from 999 to 1,005 lines. Per the audit it was
   **split, not hatched**: the owner overflow menu moved to
   `experience_time_manage_menu.dart` with a public `TimeManageAction`, matching
   the sibling-library pattern the other `experience_*_panel` widgets use.
   Nothing else crossed the cap.
6. **The `unawaited_futures` triage found no ordering bugs.** All 73 sites are
   deliberate fire-and-forget — analytics, haptics, screen-reader
   announcements, controller disposal, navigation, and stat/people refreshes the
   code already labels as such in comments. Notably **none is on the logout
   path**, which is the case `docs/client/logout.md` exists to protect. They are
   marked with `unawaited(...)` rather than awaited: each was verified
   intentional, and awaiting them would change timing that only integration
   tests could re-validate. The rule's forward value — every *new* one has to be
   deliberate — is what the phase actually buys.
7. **`only_throw_errors` found a real defect.**
   `gear_sharing_transfer_actions.expressInterest` threw bare `String`s on
   failure while *also* putting a classified `UserError` on state, so the throw
   destroyed the stack trace and duplicated an error the UI already had. Now
   `rethrow` (type and stack preserved) plus `StateError` for the guard. One
   test asserted on the raw string and was updated — the only test the whole PR
   changed for behavior rather than lint.

Two follow-up issues filed as planned: **#2823** (`discarded_futures`, 382
sites, gated on deciding the `unawaited(...)` policy first) and **#2824**
(`strict-inference`, 175 findings, re-measure now that the hand-written
`dynamic` is gone).

---

## Audit Notes (`/ripls-audit`, 2026-07-30)

**Scope:** client only (`app/analysis_options.yaml`, `app/pubspec.yaml`, `app/lib/**`,
`app/test/**`, `docs/**`). No server, proto, RPC, or infra surface — `proto_conventions.md` and
`api_design` are read but not engaged. Subsystems: presentation (screens/widgets/viewmodels),
`services/` (Mapbox, auth state, event stream), `data/streaming/`, `core/observability/firebase/`,
l10n. **Engaged dimensions:** code_quality, tech_debt, error_handling, testing, observability,
security, dependency.

**Corrections applied to this plan:**

1. **Missed a governing leaf doc — `docs/client/observability.md`.** Phase 3's file list included
   `firebase_crash_reporter.dart` and `firebase_analytics_provider.dart` as two unremarkable
   entries in "the remaining 7." They are not unremarkable: both `strict-casts` findings land
   *inside* the PII redaction/sanitization path (`_redactCustomKeyValue`'s return feeding
   `setCustomKey`; the analytics value sanitizer's `sanitized[key] = value`). That path is a
   documented security guarantee, and it is exactly the kind of thing a "just add types" PR
   widens by accident. Promoted to its own task with a named test gate
   (`app/test/core/observability/`) and cross-referenced to `security_review_prompt.md` §5 and
   `observability_review_prompt.md` §7.
2. **File-size gate (#1424) was unaddressed.** Phase 3 adds imports and longer type names to
   five files within ~160 lines of the 1,000-line `lint:dart:size` cap
   (`needs_claim_gear_picker.dart` at 941 has 59 lines of headroom). Added the gate as an
   explicit task, with the instruction to *split* rather than reach for
   `// dart-line-count-allow` — CLAUDE.md requires a tracking issue for that hatch, and the
   existing issue-less hatch on `needs_actions.dart:1` is the precedent not to repeat.
3. **Ephemeral-comment gate would have failed the PR.** Phase 4 asks for rationale comments in
   `.dart`; `scripts/check_ephemeral_comments.js` fails the build on `\bphase\s*\d+\b` in any
   `.dart`/`.go` comment under `app/lib/` or `server/`. A plan organized into numbered phases is
   a live tripwire for it. Added an explicit task to reference **#2794** in every comment
   instead. Matches `docs/client/conventions.md` §"Dart comments".
4. **Docs home was asserted without checking the existing pattern.** Verified that
   `docs/server/conventions.md` §"Linting" is the model — prose per side, describing the whole
   gate plus its one trap — and that #1611 Phase 6's proposed standalone cross-repo "Lint gates"
   table never landed. Rewrote the Phase 4 docs task to mirror the Go section, and gave the Dart
   section its own analogous trap to document: `dart analyze` defaults `--fatal-infos` **off**
   while `flutter analyze` defaults it **on**, so a clean local `dart analyze` can sit on a tree
   that fails CI. (Same shape as the `golangci-lint run ./...` config-discovery trap that cost
   this repo the entire window between #1605 and #1611.)
5. **Dependency dimension was unexamined.** Checked both inputs that can move the baseline —
   the SDK analyzer (`FLUTTER_VERSION=3.44.8`, Renovate-tracked in `versions.env`) and
   `flutter_lints: ^6.0.0` (caret range, lock-pinned by the committed `app/pubspec.lock`).
   Neither is a `@latest`-style exposure, so no pinning work is needed; added an open question
   recording the finding so it isn't re-litigated, and noting that after this issue a Renovate
   bump to either becomes a visible red PR rather than silent drift.
6. **Two rule adoptions gained a standards citation they were missing.** `close_sinks` is
   `testing_review_prompt.md` §4 ("Setup/Teardown") mechanized; the `unawaited_futures` triage
   now has an actual decision rule from `docs/client/architecture.md` §"Async-First Architecture"
   (unawaited repository call in a ViewModel = presumptively a bug; unawaited analytics or
   navigation side effect = presumptively fine) rather than leaving 73 judgment calls unguided.

**Not changed (checked, already compliant):** every measured number in the plan is reproducible
from the procedure stated in *Re-measurement* — re-verified `flutter analyze`'s fatal-on-info
default from `--help` and from an all-info run exiting 1, and confirmed the tree returns to
`No issues found!` after the experiment was reverted. The decision to decline
`avoid_positional_boolean_parameters` is correctly grounded in
`code_quality_review_prompt.md` §11 rather than in taste. No new dependency is introduced (all
19 rules ship with Dart 3.12.0 — verified by enabling them). No proto, RPC, storage, caching,
i18n, or routing surface is touched, so `proto_conventions.md`, `client/caching.md`, and the
api_design/performance rubrics are not engaged. Accessibility is untouched: Phase 3 retypes
signatures without altering semantics labels, and the `check_a11y.js` allowlist ratchet can only
be affected by adding new widgets, which no phase does.

**Docs consulted:** `docs/llms.txt`; `docs/context_map.md`; `docs/client/architecture.md`;
`docs/client/conventions.md`; `docs/client/observability.md`; `docs/client/logout.md`;
`docs/client/testing.md`; `docs/server/conventions.md`; `docs/proto_conventions.md`;
`docs/dependency_automation.md`; `docs/issues/1611-wave2-linters.md`;
`docs/reviews/code_quality_review_prompt.md`; `docs/reviews/tech_debt_review_prompt.md`;
`docs/reviews/error_handling_review_prompt.md`; `docs/reviews/testing_review_prompt.md`;
`docs/reviews/security_review_prompt.md`; `docs/reviews/observability_review_prompt.md`;
`docs/reviews/dependency_review_prompt.md`; `CLAUDE.md`.
