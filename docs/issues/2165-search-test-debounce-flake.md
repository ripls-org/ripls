# Issue #2165: Flaky test: search_view_model_test 350ms debounce wait

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2165
**Filed:** 2026-05-25 | **Last activity:** 2026-05-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`app/test/presentation/viewmodels/search_view_model_test.dart` races the 300 ms
debounce timer in `SearchNotifier.search()` (and `searchWithLocation` /
`toggleIncludeCompleted`) with a wall-clock `await Future.delayed(const
Duration(milliseconds: 350))` after each `notifier.search(...)` call. The 50 ms
margin is too tight under CI load: the timer's microtask can be parked past the
margin, the search RPC and analytics publish run after the assertion, and the
test fails with `Expected: true / Actual: <false>`. PR #2163 surfaced the flake
on the `analytics logs SearchPerformedEvent on successful search with non-empty
query` case (line 1684); the test passes locally and on re-run.

The same `Future.delayed(...)`-after-`search(...)` pattern appears **38 times
in this one file** with various margins (350, 500, 800, 3500 ms) and three more
times in adjacent debounce-driven viewmodel tests (`inbox_search`,
`event_modal`, `fulfill_modal`). All of them are wall-clock-dependent and
silently broken under load — only the 50 ms case has tripped CI so far.

## Context from Comments

One comment from collaborator maintainer: `@claude`. No new technical
direction; the issue body is the authoritative spec. The body already proposes
three options, recommends `fake_async`, and lists acceptance criteria
(no `Future.delayed` waits for debounce, stress-run demo, audit of adjacent
debounce-driven tests).

Note that the body says "Mirrors how chat-debounce tests already structure
timer-driven flows (see `conversation_view_model_test.dart`)" — but
`conversation_view_model_test.dart` actually does **not** use `fake_async`
(it uses the same `Future.delayed` pattern). No production or test code in
this repo currently imports `package:fake_async`. The package is present as a
transitive dependency (via `flutter_test`) but not declared in
`app/pubspec.yaml`. This plan introduces `fake_async` as a new pattern, so the
plan must include the small precedent-setting pieces (pubspec entry,
README/conventions note, helper) — not just edit the test file.

## Potential Solutions

### Option A: Migrate to `package:fake_async` (recommended)

- **Approach:** Add `fake_async` to `dev_dependencies`. For each test that fires
  a debounced action and asserts on the post-debounce state, wrap the body in
  `fakeAsync((async) { ... })`. Replace each `await Future.delayed(const
  Duration(milliseconds: N))` with `async.elapse(const Duration(milliseconds:
  N))` followed (where the assertion checks state that lands via a `Future`
  continuation) by `async.flushMicrotasks()`. Tests that today read
  `await container.read(searchProvider.future)` to wait for the `AsyncNotifier`
  build use the same pattern: schedule, elapse zero, flush microtasks, then
  read `state.value`. Add a thin local helper (`runFakeAsyncTest` or extension)
  in this file so the boilerplate stays uniform across 38 sites; keep it
  file-local since no other test currently needs it. Document the pattern in
  `app/test/presentation/viewmodels/README.md` once it lands so the next
  debounce test can copy it.
- **Pros:** Deterministic regardless of CI load; the test stops depending on
  the value of `_debounceDuration` (a constant in production code); fixes the
  problem at the root for every site, including the 3500 ms iOS-hang
  simulation case which currently waits 3.5 wall-clock seconds per run.
- **Cons:** First user of `fake_async` in the codebase — there's a learning
  curve and a few real gotchas (`thenAnswer((_) async {...})` continuations
  run inside the fake zone; `Completer.future` waits must be paired with the
  right elapse; Riverpod's `AsyncNotifier` build propagation is microtask-
  based). Worth the one-time cost given the 38 affected sites.
- **Effort:** Medium. ~40 sites across 4 files, plus pubspec/README. Most are
  mechanical; a handful (race-condition test on line 255, 3-emission startup
  on line 931, Phase 3 hang simulation on line 1279) need careful elapse +
  microtask ordering.

### Option B: Pad the wait

- **Approach:** Change `Duration(milliseconds: 350)` to `Duration(milliseconds:
  1000)` (or extract a `const _afterDebounce = Duration(milliseconds: 1000)`
  constant — `inbox_search_view_model_test.dart` already has the `_afterDebounce`
  helper, at 100 ms over 200 ms debounce). Same edit applied to the other three
  files.
- **Pros:** ~50-line diff, zero behaviour change, ships in an hour.
- **Cons:** Just delays the next flake — under sufficiently degraded CI a 700 ms
  margin can still miss, and the test suite gets ~10 s slower per run. Doesn't
  satisfy the issue's stated acceptance criterion ("No `Future.delayed(...)` to
  wait for the search debounce in this file"). Also leaves the 3500 ms iOS-hang
  test untouched — that one still costs real wall-clock seconds.
- **Effort:** Small.

### Option C: Expose a `@visibleForTesting` settle hook on `SearchNotifier`

- **Approach:** Add `@visibleForTesting Future<void> awaitInFlight()` to
  `SearchNotifier` that completes when the debounce timer has fired and the
  current `_executeSearch` future has resolved. Tests `await notifier
  .awaitInFlight()` instead of waiting wall-clock time. Repeat for the other
  affected viewmodels.
- **Pros:** No new test dependency; tests stay readable; works regardless of
  how the debounce constant changes.
- **Cons:** Adds production surface area for a test concern. Tracking
  "in-flight" requires a private `Future? _inFlight` field per debounce site —
  multiple per notifier — and careful nulling on completion. Easy to get
  subtly wrong; harder than fakeAsync to reason about under cancellation
  (timer cancel + new search arriving before old completes).
- **Effort:** Medium.

## Recommendation

**Option A.** The issue explicitly recommends it, the alternatives either don't
solve the problem (B) or pollute production code with a test affordance (C).
The one-time cost of introducing the `fake_async` pattern is paid back across
all 38 sites in this file and ~10 sites in adjacent debounce-driven tests, and
the 3500 ms iOS-hang test in Phase 3 stops costing real seconds of CI time.

Two tradeoffs worth calling out before implementation:

1. **Mocks that return immediate `Future`s.** Tests using
   `thenAnswer((_) async => results)` work cleanly inside `fakeAsync` because
   the continuation runs inside the fake zone. Tests that wrap
   `await Future.delayed(...)` inside the mock body (line 269 for the 500 ms
   slow search, line 1019 for the 100 ms race-condition simulation) need to
   advance time inside the zone — they become naturally simpler since the
   simulated delay is now also fake.

2. **Real timer in the production code (the 3-second location timeout in
   `_performInitialSearch`).** Already a `Timer`-based timeout; will fire
   correctly under fakeAsync as long as `async.elapse(Duration(seconds: 3))`
   is reached before the assertion. The current test (`Phase 3:` "search
   completes with fallback coordinates when location fetch times out", line
   1279) already waits 3500 ms; under fakeAsync that wait moves from wall-clock
   to simulated time.

## Open Questions

- **Do we adopt `fakeAsync` repo-wide, or just for these four debounce-driven
  files?** — Recommendation: just these four for #2165, file a follow-up issue
  to audit `Future.delayed` usage across `app/test/` once the pattern proves
  itself. The other 22 viewmodel test files using `Future.delayed` are mostly
  short polling waits (10–100 ms for stream reconnect/scheduling), not
  debounce races, so they're less acutely flaky.
- **Should the helper live in `app/test/helpers/` or stay file-local?** —
  Recommendation: start file-local in `search_view_model_test.dart` as a top-
  level `void runDebounced(void Function(FakeAsync async) body)`. Once Phase 3
  migrates the three adjacent files (`inbox_search`, `event_modal`,
  `fulfill_modal`), promote the helper to
  `app/test/helpers/fake_async_helpers.dart` (alongside the existing
  `l10n_helpers.dart` and `test_data_helper.dart`) and update the four
  imports. Two real call sites is enough evidence to commit to a shared API.
  (Note: `docs/client/testing.md` calls this directory `test_helpers/` but
  the actual on-disk path is `app/test/helpers/`. The doc is slightly out of
  sync; do not rename it as part of this work.)
- **Promote `fake_async` from transitive to direct dev dependency, or rely on
  the transitive import via `flutter_test`?** — Recommendation: declare it
  explicitly in `app/pubspec.yaml` under `dev_dependencies`. Implicit transitive
  imports are fragile (a future `flutter_test` upgrade could drop it) and the
  CLAUDE.md style guide implicitly favours explicit declarations.
- **Should the 3500 ms iOS-hang test stay or shrink?** — Recommendation: keep
  the 3-second timeout assertion (it validates the production timeout works)
  but under fakeAsync the test runs in zero wall-clock time. No additional
  test code needed.
- **Stress-run evidence: where does it live?** — Recommendation: run it
  locally as `for i in {1..50}; do (cd app && flutter test --reporter compact
  test/presentation/viewmodels/search_view_model_test.dart) || break; done`
  and paste the "50/50 passed" output into the PR description. Do not add a
  CI loop — the deterministic fix removes the need for repeated-run gating.

## Implementation Plan

### Phase 1: Pubspec + helper + one converted test as proof-of-concept
**Goal:** Establish the pattern with the smallest possible footprint so the
rest of the migration is mechanical.
**Tasks:**
- [ ] Add `fake_async: ^1.3.3` (or whatever version matches the current
  `pubspec.lock` transitive entry) to `dev_dependencies` in
  `app/pubspec.yaml`.
- [ ] Run `cd app && flutter pub get` to lock the version.
- [ ] In `app/test/presentation/viewmodels/search_view_model_test.dart`, add
  the helper at the top of the file:
  ```dart
  /// Runs [body] inside a FakeAsync zone, advancing simulated time and
  /// flushing microtasks so debounce timers + their continuations resolve
  /// deterministically. Replaces the historical
  /// `await Future.delayed(Duration(milliseconds: 350))` pattern.
  void runDebounced(void Function(FakeAsync async) body) {
    fakeAsync((async) {
      body(async);
      async.flushTimers();
    });
  }
  ```
  (Top-level function; not a class. Named so it reads as "run a debounce-
  driven test".)
- [ ] Convert the first failing test ("logs SearchPerformedEvent on successful
  search with non-empty query", line 1646) to use the helper. Pattern:
  ```dart
  test('logs SearchPerformedEvent ...', () {
    runDebounced((async) {
      container.read(searchProvider.future); // fire build
      async.flushMicrotasks();
      // when(...) setup, notifier.search(...)
      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();
      expect(mockObservability.hasEventOfType<SearchPerformedEvent>(), isTrue);
    });
  });
  ```
- [ ] Run `flutter test --reporter expanded
  test/presentation/viewmodels/search_view_model_test.dart -n "logs
  SearchPerformedEvent"` to confirm the converted test passes.
- [ ] Run a 50× stress loop locally on just the converted test to confirm
  zero flakes.
**Validates:** `fake_async` plays correctly with Mockito-backed
`AsyncNotifier` tests using the `ProviderContainer` setUp pattern. If this
step uncovers an unexpected interaction (e.g. Riverpod schedules work outside
the zone), the design changes; everything below depends on this proof.

### Phase 2: Migrate all 38 sites in search_view_model_test.dart
**Goal:** Delete every `await Future.delayed(...)` that exists to bridge a
debounce window.
**Tasks:**
- [ ] Walk every site listed in the issue (search → 350 ms, race-condition →
  500 ms + 800 ms, refresh → 350 ms, Phase 8 → 300/400 ms, Phase 9 → 50/300 ms,
  Phase 3 → 3500/300 ms, timer disposal → 350 ms, cache invalidation → 350 ms,
  analytics group → 350 ms). For each: convert the surrounding `async test`
  to `test` + `runDebounced`, and replace the wall-clock delay with
  `async.elapse(...)` plus `async.flushMicrotasks()` where the assertion
  reads state populated by an async continuation.
- [ ] For Phase 8/9/3 tests that use `c.listen(searchProvider, (_, _) {})`
  plus `await Future.microtask(() {})`, replace the microtask awaits with
  `async.flushMicrotasks()`. The `c.listen` call itself is sync.
- [ ] For the Phase 3 hang-simulation test (line 1279), the 3-second
  production timeout is itself a `Timer`. Confirm `async.elapse(Duration(
  milliseconds: 3500))` fires it correctly and the search RPC completes.
- [ ] For the line 1019 inside-mock `await Future.delayed(...)` simulating a
  slow RPC, leave the `await Future.delayed` inside the mock body as-is
  (it executes inside the fake zone and elapses with the outer `async`).
  Same for line 269 and line 288.
- [ ] Run `flutter test --reporter expanded
  test/presentation/viewmodels/search_view_model_test.dart` and iterate until
  all tests pass.
- [ ] Run the 50× stress loop on the full file. Paste the output into the
  PR description (and the issue acceptance checklist).
**Validates:** The pattern scales to a heterogeneous test file with race-
condition tests, multi-emission notifier tests, and timeout simulation.

### Phase 3: Audit and migrate adjacent debounce-driven tests
**Goal:** Cover the issue's audit acceptance criterion. Apply the same
treatment to the other three known debounce-driven viewmodel tests.
**Tasks:**
- [ ] `app/test/presentation/viewmodels/inbox_search_view_model_test.dart`
  (4 sites, 300 ms wait over 200 ms `InboxSearchNotifier.debounce`). The
  existing `const _afterDebounce = Duration(milliseconds: 300)` helper goes
  away in favour of `async.elapse(InboxSearchNotifier.debounce)`.
- [ ] `app/test/presentation/viewmodels/event_modal_view_model_test.dart`
  (1 site at line 669, 400 ms wait over 250 ms preview debounce).
- [ ] `app/test/presentation/viewmodels/fulfill_modal_view_model_test.dart`
  (5 sites at lines 87/118/141/150/187, 320 ms wait over 250 ms preview
  debounce — 70 ms margin, second-most-flaky pattern in the repo).
- [ ] After migrating each file, run it 50× locally to confirm determinism.
- [ ] Promote the file-local helper into
  `app/test/helpers/fake_async_helpers.dart` (the real on-disk location for
  shared test utilities — peers: `l10n_helpers.dart`,
  `test_data_helper.dart`). Update the four migrated files to import it. The
  promotion happens now, not in a follow-up, because Phase 3 itself produces
  four call sites — the API is committable.
- [ ] Update `docs/client/testing.md`. Its current "Testing Timer Management"
  section (lines 265–286) demonstrates exactly the bad pattern this issue
  fixes (`await Future.delayed(Duration(seconds: 11));` to "Fast-forward
  time"). Replace that example with a `fakeAsync` + `async.elapse(...)`
  example pointing at `app/test/helpers/fake_async_helpers.dart`, and add a
  one-line note discouraging wall-clock waits for debounced or timer-driven
  code. Do not rewrite the rest of the doc.
**Validates:** The pattern is reusable beyond the original file. The shared
helper compiles against four real call sites. The docs no longer recommend
the bad pattern. No production code is touched in any of these files —
they're pure test edits.

## Related Issues
- #2155 / PR #2163 — the PR whose CI run surfaced the flake; the fix is
  unrelated to that PR's content (Connect-stream serialization), it just
  happened to be running when the flake landed.
- #1364 — Go-side analogue: `time.Sleep`-based test synchronization across 15
  server test files. Solved with simulated time + done channels, not fakeAsync
  (Go has different ergonomics). Cited here as precedent for fixing flakiness
  at the root rather than padding margins.
- #1238 — flaky testcontainer setup; another instance of CI-load flakes.
  Different root cause (Docker container startup) but the principle ("delete
  the source of nondeterminism, don't widen the window") is the same.
