# Issue #2158: Logout crash after adding need in new experience needs flow (RenderIgnorePointer mutated in performLayout)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2158
**Filed:** 2026-05-25 | **Last activity:** 2026-05-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** [`docs/client/logout.md`](../client/logout.md), [`docs/client/caching.md`](../client/caching.md) §Pattern 7, [`docs/client/architecture.md`](../client/architecture.md) §Async Safety in Notifiers, [`docs/client/testing.md`](../client/testing.md)

## Problem Statement

When a user fires a mutation in the v2 Experience Needs flow (#2148) and then
logs out before the mutation's downstream side effects settle, Flutter throws

```
A RenderIgnorePointer was mutated in _RenderLayoutBuilder.performLayout.
The RenderObject was mutated when none of its ancestors is actively performing layout.
```

with the home `IndexedStack` at [`home_screen.dart:481`](../../app/lib/presentation/screens/home/home_screen.dart#L481)
named as the error-causing widget. The stack ends in
`Element._retakeInactiveElement` — Flutter is taking a previously deactivated
GlobalKey'd subtree (the discover `Navigator` keyed by
`discoverNavigatorKeyProvider`) and re-inserting it during the `_ScaffoldLayout`
layout pass. The detach chain hits a `RenderIgnorePointer` whose
`markNeedsLayout` is rejected because no ancestor is currently in layout.

The same crash class was previously fixed for the location and time poll modals
in commit [`624e6bc4d`](https://github.com/Type2Fun-Foundation/ripls/commit/624e6bc4d)
("Fix logout crash") via per-method `if (authState.user == null) return;` guards.
The needs v2 viewmodels already adopted the same pattern (`_loggedOut` getter,
guards before and after every `await`), and as a follow-up to filing this issue
we tightened every post-await `if (!ref.mounted) return;` to also check
`_loggedOut`. **That did not fix the crash** — the same stack trace still
reproduces. So the race is not (only) in the per-mutation state write. The
"plug each viewmodel as you find it" fix model is exhausted.

The repository call after a successful `addNeed` does two things beyond the
state write the viewmodel controls:

1. Invalidates the `needs_and_contributions:<experienceId>` cache key.
2. Calls the repository's `_onDailyInvalidated` callback, which fires
   `portfolioCacheInvalidationProvider.notify()` (wired in
   [`experience_providers.dart`](../../app/lib/services/providers/experience_providers.dart)).

`notify()` increments a counter on a global notifier. Every `ref.listen` /
`ref.listenManual` consumer of that notifier will be invoked on the next
microtask — regardless of auth state — and is individually responsible for
remembering to bail. There are 25+ `notify()` call sites and ~10 listener
sites across the app. Any one of them that runs in the logout frame is a
candidate trigger for the IndexedStack rebuild that retakes the keyed
Navigator and crashes.

Compounding this, `home_screen.dart:471–474` already carries a comment that
admits the same failure mode is structurally possible:

> Watching the whole HomeState would rebuild the IndexedStack on every
> isNavVisible toggle, and a rebuild that lands during an upstream
> LayoutBuilder pass can retake an inactive GlobalKey'd subtree (the discover
> Navigator) mid-layout — Flutter rejects that with "RenderIgnorePointer was
> mutated in performLayout".

The mitigation there was narrow (watch only `selectedIndex`). The hazard is
still latent: *any* upstream rebuild during layout that touches the IndexedStack
subtree can trip this. The needs-mutation-then-logout flow happens to be one
such trigger; this issue exists to fix it in a way that closes the class, not
just the instance.

**This is a documented historical failure mode.** [`docs/client/logout.md`](../client/logout.md)
explicitly calls out the same class of crash as a known hazard:

> Firing two Riverpod state changes (auth clear + the reset) in the same frame
> as the router redirect causes the IndexedStack's nested Navigators (with
> stable `GlobalKey`s) to rebuild mid-transition and produce a **Duplicate
> GlobalKey** crash.

The documented mitigation in `logout.md` is to *not* reset per-user Riverpod
notifiers inside `logout()` — do it at the **login entry point**
(`_loadCommunities()` in [`main.dart`](../../app/lib/main.dart)) instead. The
issue we're solving in this plan is the dual of that one: not Riverpod resets
triggered *by* logout, but in-flight async work triggered *before* logout
whose downstream side effects resolve *during* the logout frame. The fix must
not violate `logout.md`'s ordering — see the Cross-cutting conventions section
for how phase 2's interlock change stays inside that contract.

## Context from Comments

No comments yet — the issue was filed in this session. The body itself
includes the investigation plan and a checklist of solution ideas. Two things
worth flagging because they aren't fully resolved in the issue:

1. The issue lists five "thorough solution" checkboxes (central interlock,
   repo-level no-op, mutation helper, fix the IndexedStack hazard, regression
   test, docs). They are not independent — the order matters and some can be
   done without others. This plan sequences them.
2. The issue's investigation step #1 ("reproduce with a deterministic delay")
   is the right first move and is non-negotiable. Without confirming whether
   the crash also reproduces when the RPC has already settled before logout,
   we don't know whether the bug is "post-await write races logout" (current
   leading hypothesis) or "logout itself triggers a structurally unsafe
   widget-tree teardown" (which would mean the IndexedStack fix is the only
   real fix and the rest is hygiene).

## Cross-cutting conventions (audit guardrails)

These apply to every phase. Listed up front so the per-phase task lists can
stay focused.

- **Repository pattern + caching ([`docs/client/architecture.md`](../client/architecture.md), [`docs/client/caching.md`](../client/caching.md) §Pattern 7).** All data access stays through repositories; do not introduce a parallel "skip cache when logging out" branch in viewmodels. Any logout-aware behavior belongs in the repository or the invalidation notifier — never duplicated per call site. Pattern 7 (Event-Driven Cache Invalidation) is the chain we're suppressing: `repository → onDailyInvalidated → portfolioCacheInvalidationProvider.notify() → listeners` debounces and refreshes. Suppressing `notify()` during logout is safe because (a) the debounce never fires, and (b) the next login's `_loadCommunities()` re-issues the fetch.
- **Logout teardown ordering ([`docs/client/logout.md`](../client/logout.md)).** `AuthStateNotifier.logout()` already does **one** synchronous `state = AuthStateData(...)` write at the start, then awaits cache + secure-storage clears. The phase-2 interlock change *adds a field to that existing write* — it does not introduce a second `state = ...` mutation in the same frame. That distinction matters because `logout.md` documents that two Riverpod state changes in the logout frame is exactly the failure mode (Duplicate GlobalKey crash). Promoting `_isLoggingOut` to `state.isLoggingOut` and writing it inside the existing `state = AuthStateData(..., isLoggingOut: true)` call sidesteps the hazard.
- **Async safety in notifiers ([`docs/client/architecture.md` §Async Safety in Notifiers](../client/architecture.md#async-safety-in-notifiers)).** The doc currently mandates `if (!ref.mounted)` guards after every `await` and points at `SafeNotifierMixin` for the boilerplate. This issue adds a second hazard (the logout-frame race) that the current section does not name. The phase-5 docs update extends the section to call it out and to point at the interlock as the canonical fix; it does not introduce a new pattern outside the doc.
- **Riverpod provider lifecycle.** `autoDispose.family` notifiers (the needs viewmodels are exactly this shape) can survive briefly after the last `ref.watch` is gone. The interlock check has to be readable from any notifier that has a `ref` — including ones that don't watch `authStateProvider` themselves — so the interlock provider must be a plain `Provider<bool>` or a public getter on `AuthStateNotifier`, not a value that requires subscription.
- **i18n / a11y ([`docs/client/i18n.md`](../client/i18n.md), [`docs/client/accessibility.md`](../client/accessibility.md)).** Nothing in this plan adds user-visible strings (the fix is invisible). If a phase ends up surfacing a "session ended" toast or banner, that string lands in **both** `app_en.arb` and `app_es.arb` with matching `@key` metadata — `npm run lint:dart:l10n-parity` gates merge.
- **Testing ([`docs/client/testing.md`](../client/testing.md)).** The doc is emphatic: "Test ViewModels, not widgets." Phase 2's `cache_providers_test.dart` and `auth_state_test.dart` are correctly at the viewmodel/provider layer with `ProviderContainer` + Mockito and follow the canonical setup pattern. **Phase 3's regression test is a justified exception** — the symptom is a widget-tree assertion (`RenderIgnorePointer` mid-layout) that the viewmodel layer cannot observe. Widget-test precedent in the codebase: [`app/test/presentation/screens/create/unified_create_modal_dismiss_test.dart`](../../app/test/presentation/screens/create/unified_create_modal_dismiss_test.dart), [`app/test/presentation/widgets/modal/glass/date_time_picker_modal_test.dart`](../../app/test/presentation/widgets/modal/glass/date_time_picker_modal_test.dart). Pump the widget tree through [`app/test/helpers/l10n_helpers.dart`](../../app/test/helpers/l10n_helpers.dart) so `localizationsDelegates` and `supportedLocales` are wired ([`docs/client/i18n.md`](../client/i18n.md) §Test localization). Run with `flutter test --reporter expanded` per [`CLAUDE.md`](../../CLAUDE.md).
- **i18n viewmodel rule ([`docs/client/i18n.md`](../client/i18n.md)).** Viewmodels never resolve strings. The needs viewmodels already follow this (`UserError? mutationError` typed state, widgets call `RpcErrorHandler.localize` in `build()`). Nothing in this fix should change that — phase 5's per-method-guard removal must not accidentally inline localized text into the viewmodel.
- **No server changes ([`docs/server/architecture.md`](../server/architecture.md), [`docs/server/observability.md`](../server/observability.md)).** This is a client-only bug. No RPC signatures change, no proto edits, no structured-logging fields added. The repository-level guard discussed under Option B remains client-side. If we discover during phase 1 that a server-side change is needed (e.g. an explicit "cancel pending work" RPC), it gets a separate plan and a separate PR.
- **Backwards-compatibility hacks are forbidden** per [`CLAUDE.md`](../../CLAUDE.md). The viewmodels currently carry per-method `_loggedOut` guards. Once the interlock + helper land and the regression test passes, the per-method guards become dead weight; phase 5 removes them. Do not leave the per-method guards in place "as defense in depth" — they are the pattern that has already failed and they should be deleted, not preserved.
- **Phase markers in source comments fail the build** per [`CLAUDE.md`](../../CLAUDE.md) §Go Code Comments. No "Phase 3a", "added in #2158", etc. in `*.dart` files. PR descriptions and this plan carry that context.
- **TODOs.** Any TODO left behind by this work must be `TODO(#NNNN):` with an open tracked issue per [`CLAUDE.md`](../../CLAUDE.md) §TODO Comments. Deferred items in this plan (e.g. extending the interlock to the request and gear repositories if we scope phase 2 narrowly first) need GitHub issues filed before the corresponding TODO is added in code.

## Potential Solutions

### Option A: Central logout interlock at the invalidation-notifier layer

Add a single `isLoggingOutProvider` (or expose `_isLoggingOut` from
`AuthStateNotifier` as a public getter / `state.isLoggingOut` field). Every
`*CacheInvalidationNotifier.notify()` consults the interlock and no-ops while
it is set. Repository callbacks (`_onDailyInvalidated`, `_onContentInvalidated`,
`_onImpactInvalidated`) still fire but their downstream effect (the notifier
increment) is suppressed. Listeners therefore don't fire at all during logout,
and the per-listener auth guards become belt-and-suspenders rather than the
load-bearing fix.

- **Approach:** ~30 lines in `auth_state.dart` (expose the flag) + ~15 lines per cache invalidation notifier (early-return in `notify()`). Roughly 6 notifiers in `cache_providers.dart`, plus any in `community_providers.dart`. No repository changes, no viewmodel changes. Sequence: flip interlock at the *start* of `logout()` synchronously, before any state write or cache clear; reset on the next successful `setAuthState`.
- **Pros:** Single point of control. Catches every existing and future listener with zero per-call-site effort. Maps naturally onto the existing `_isLoggingOut` flag that already exists in `AuthStateNotifier`. Cheap to add a regression test that verifies `notify()` is suppressed while the flag is set.
- **Cons:** Doesn't fix the underlying IndexedStack hazard — if any *other* state change (FCM, event poller, locale change) lands in the logout frame, the crash can still recur. Doesn't help mutations whose state writes don't go through a cache-invalidation notifier. The repository continues to invalidate its own cache; if a viewmodel reads through the cache after logout it will trigger an RPC that fails — that needs a separate guard or relies on the per-viewmodel `_loggedOut` checks we already have.
- **Effort:** Small (~half a day).

### Option B: Logout interlock + repository-level side-effect suppression + structural IndexedStack fix

Option A, plus:

1. Repository methods (`addNeed`, `addNeedsBatch`, `claimNeed`, `addContribution`, and the same set on `RequestRepository` / `GearRepository`) check the interlock before firing the invalidation callbacks and before invalidating their own cache keys.
2. The home `IndexedStack` is restructured so a state change during layout cannot retake the GlobalKey'd discover Navigator. Two viable shapes: (a) move the keyed Navigator out of the IndexedStack into a sibling stack that's always at offset; (b) wrap the IndexedStack children in `KeyedSubtree`s that are stable across rebuilds and don't depend on auth-sensitive InheritedNotifier traffic.

- **Approach:** All of option A, plus repository-layer guard via `ref` injection (each repository already takes a `Ref` indirectly through the provider — the interlock becomes a constructor param the provider passes through). Then a focused refactor of `home_screen._buildBody()` to remove the structural hazard.
- **Pros:** Closes the class of bug, not just the instance. The IndexedStack fix is the real root cause — without it, future regressions in unrelated code paths can still trip the assertion. Repository-layer suppression also catches the case where a viewmodel calls a repository method directly without going through its own notifier (less common but exists).
- **Cons:** Largest blast radius. The IndexedStack refactor touches code that several other features depend on (discover tab, navigation lock, scroll-to-top) — needs careful testing across all four tabs. The repository guard requires changing constructor signatures; some tests will need updating. More moving parts in a single PR.
- **Effort:** Large (1–1.5 weeks including IndexedStack refactor + cross-tab smoke testing).

### Option C: Just the regression test + AsyncNotifier migration of the needs viewmodels

Migrate `ExperienceNeedsNotifier` and `RequestNeedsNotifier` from `Notifier`
with `SafeNotifierMixin` to `AsyncNotifier`. Per `docs/client/architecture.md`
§Async Safety in Notifiers, `AsyncNotifier.build()` handles disposal
automatically. Mutation methods still need manual guards, but the disposal
window shrinks. Add the regression test. Leave the cache invalidation
notifiers and IndexedStack alone.

- **Approach:** Drop `_loggedOut` getter + manual checks from the needs viewmodels; rewrite `build()` to return a `Future<State>` and let Riverpod manage disposal. Mutation methods become `AsyncValue.guard`-wrapped. Add the widget test.
- **Pros:** Aligns the needs viewmodels with the canonical "preferred" pattern in the architecture doc. Small surface area. Easier to review than option B. The architecture doc explicitly calls out that this is the migration path.
- **Cons:** Doesn't actually fix the bug as understood. The architecture doc itself says (line 716): "Methods that manually set state still need `ref.mounted` checks" — the disposal-window race we have is in mutation methods, not in `build()`. Migration doesn't close the cache-invalidation listener exposure either. Most likely outcome: this is a worthwhile cleanup but the crash still reproduces.
- **Effort:** Medium. Half a day for the migration, plus a half-day debugging the lifecycle differences.

## Recommendation

**Option B, phased — start with the interlock from option A as phase 2, gate
on whether the crash is fully fixed before committing to the IndexedStack
refactor.**

Reasoning:

- **Option A alone is probably not enough.** The home_screen comment is explicit: the IndexedStack/keyed-Navigator combination is fragile. Removing one trigger (cache-invalidation traffic during logout) likely fixes this specific reproduction but leaves the structural hazard in place. We'll be back here when FCM or the event poller next races a logout.
- **Option C alone is not enough.** The architecture doc itself disclaims the partial coverage of AsyncNotifier migration for mutation paths. It's worth doing eventually but it's not load-bearing for this bug.
- **Option B is the only one that closes the class** — but it's also the largest. The risk of doing Option B in one PR is that we can't tell which part actually fixed the crash, which makes the regression test hard to anchor.
- **Phasing solves the diagnostic problem.** Phase 1 (deterministic reproduction) tells us whether the post-await state write or the IndexedStack teardown is the load-bearing trigger. Phase 2 (the interlock) is the cheap, high-leverage fix that may by itself close the user-visible bug. Phase 3 (regression test) anchors whatever we ship in phase 2. Phase 4 (IndexedStack restructure) is only worth its cost if phases 2–3 don't fully close the bug or if we observe a related crash in the meantime; we hold the option to do it but don't pre-commit.

**What I'd want to know before committing to phase 4:** does the deterministic
repro in phase 1 still reproduce after phase 2 is in place? If yes, phase 4
becomes mandatory. If no, file phase 4 as a separate hardening issue and let
the next observed instance pull it forward.

## Open Questions

- **Should the interlock be a separate provider or a field on `AuthStateData`?** — Recommendation: **field on `AuthStateData`** (`isLoggingOut`, defaulting to `false`). The `_isLoggingOut` private flag in `AuthStateNotifier` already exists for the duplicate-logout-call guard; promoting it to public state means every consumer can `ref.read(authStateProvider).isLoggingOut` without subscribing to a new provider. Keeps the auth-related state in one place.
- **Should `notify()` log a warning when suppressed?** — Recommendation: **yes, at FINE level only.** Suppressed `notify()` calls during logout are expected and frequent; FINE keeps them out of the default log stream while still being available for debugging. Pattern matches the `SafeNotifierMixin._log.warning` call when state updates are skipped, but at a lower level because suppression is by design rather than a code smell.
- **Should the repository-layer guard be added in phase 2 or deferred to phase 4?** — Recommendation: **defer to phase 4.** The invalidation-notifier guard catches the downstream effect for free. Repository-layer guard is only relevant if some path bypasses the notifier (which we haven't found a case of). Adds constructor-signature churn without a known triggering bug.
- **Does the regression test belong in `app/test/presentation/viewmodels/` or `app/test/presentation/screens/`?** — Recommendation: **`screens/` (widget test).** The crash is a widget-tree assertion, not a state-write assertion. The viewmodel layer doesn't have access to the IndexedStack widget that's being mutated. The test must drive the actual UI — pump the home screen, trigger the mutation, log out, await both, and assert no `FlutterError` was emitted via `FlutterError.onError`.
- **If phase 4 happens, which IndexedStack restructure?** — Recommendation: **start with (a) — move the keyed Navigator out.** The discover tab is already special-cased (it has its own GlobalKey, its own refresh-key provider, its own scroll controller). Pulling it out of the IndexedStack into a sibling that stays mounted regardless of `selectedIndex` removes the retake hazard without changing tab-switching semantics. Wrap it in `Offstage` or `Visibility(maintainState: true)` so it doesn't take input when not selected. This is a smaller refactor than reshaping the whole IndexedStack pattern.
- **What about the v1 sheets (`view_need_sheet.dart`, etc.) that the redesign hasn't deleted yet?** — Recommendation: **no special action.** They share the `experienceNeedsProvider` / `requestNeedsProvider` and inherit any fix at the notifier / repository level. Phase 5 of #2148 will delete them anyway.

## Implementation Plan

### Phase 1: Deterministic reproduction

**Goal:** Confirm the race timing and pinpoint which downstream effect
actually triggers the IndexedStack rebuild. This phase produces no user-visible
change.

**Tasks:**
- [ ] Add a debug-only `Future.delayed(const Duration(seconds: 3))` inside `_service.addNeed` (or a test-injected slow mock) so the mutation RPC takes a controllable amount of time.
- [ ] Repro the crash on a debug iOS / Android build with the delay in place: open Plan tab → start `addNeed` → log out within the 3-second window → wait for RPC to resolve. Capture whether the crash fires (a) before the RPC resolves, (b) at resolution, or (c) after resolution during the post-await side-effect chain.
- [ ] Repeat with the RPC delay removed but a delay added in `_refreshAfterMutation` instead. Determines whether the crash needs the refresh side-effect or just the initial mutation.
- [ ] Repeat with `_onDailyInvalidated` stubbed to a no-op. If the crash stops, the cache-invalidation notifier path is confirmed as the trigger. If it still happens, the bug is elsewhere (likely the IndexedStack structural hazard).
- [ ] Write the findings as a 5-line update on the issue thread. Decide phase-4 commitment based on the result.

**Validates:** Whether the cache-invalidation notifier traffic is the
load-bearing trigger. Drives whether phase 4 is mandatory or deferred.

### Phase 2: Central logout interlock

**Goal:** Make `*CacheInvalidationNotifier.notify()` a no-op while logout is
in progress, eliminating the cache-invalidation listener exposure as a
trigger.

**Tasks:**
- [ ] Promote `AuthStateNotifier._isLoggingOut` to a public field on `AuthStateData` (`isLoggingOut`, defaults to `false`). Update the existing `state = AuthStateData(...)` write inside `logout()` to set `isLoggingOut: true` *as part of that same single write* — do **not** add a second state mutation. Per [`docs/client/logout.md`](../client/logout.md), two Riverpod state changes in the logout frame is the documented Duplicate-GlobalKey hazard; adding a field to the existing write is safe. Reset to `false` on the next `setAuthState` call (login path), matching the existing `_isLoggingOut = false` reset.
- [ ] Audit every cache-invalidation notifier in [`cache_providers.dart`](../../app/lib/services/providers/cache_providers.dart) (`SearchCacheInvalidationNotifier`, `EsmCacheInvalidationNotifier`, `FeedStatusCacheInvalidationNotifier`, `FeedListingCacheInvalidationNotifier`, `PortfolioCacheInvalidationNotifier`, `ContentCacheInvalidationNotifier`, plus `ImpactCacheInvalidationNotifier`, `TransferCacheInvalidationNotifier`) and any sibling notifiers in [`community_providers.dart`](../../app/lib/services/providers/community_providers.dart). Gate each `notify()` with `if (ref.read(authStateProvider).isLoggingOut) return;`. Add a `_log.fine(...)` line for traceability (FINE not WARN — suppression is by design, not a code smell).
- [ ] **Unit tests** (`app/test/services/providers/cache_providers_test.dart`): use `ProviderContainer` with an overridden `authStateProvider` that yields `isLoggingOut: true`; assert `notify()` does not increment state. Mirror for the false case. Run `flutter test --reporter expanded`.
- [ ] **Unit test** (`app/test/services/auth_state_test.dart`): assert `logout()` transitions `state.isLoggingOut` to `true` synchronously before any `await`, and that it stays `true` after `logout()` completes until the next `setAuthState`.

**Validates:** The cache-invalidation notifier path no longer fires during
logout. Re-run the manual repro from phase 1 with the slow-RPC harness
removed; if the crash is gone, phase 2 is sufficient and phase 4 is filed as
a hardening follow-up rather than blocking this PR.

### Phase 3: Regression test

**Goal:** Anchor whatever phases 1–2 produced so the next contributor
who reintroduces the race fails fast in CI rather than in a user's hands.

**Tasks:**
- [ ] Add `app/test/presentation/screens/home/home_screen_logout_race_test.dart`. Setup: pump `HomeScreen` inside a `ProviderScope` with overridden `authStateProvider`, `experienceNeedsProvider`, and `experienceServiceProvider` (Mockito mocks per [`docs/client/testing.md`](../client/testing.md) §Mock Generation — use `@GenerateMocks([...])` and `flutter pub run build_runner build`). Wrap the test app with `localizationsDelegates` + `supportedLocales` via [`app/test/helpers/l10n_helpers.dart`](../../app/test/helpers/l10n_helpers.dart) per [`docs/client/i18n.md`](../client/i18n.md) §Test localization.
- [ ] Test body: (a) navigate to a mock Experience, open the Volunteer sheet, tap the AddOption ghost; (b) submit the picker — the mock `addNeed` returns a `Completer<>().future` that the test controls; (c) drive `authStateProvider.notifier.logout()`; (d) complete the `addNeed` future; (e) `await tester.pumpAndSettle()`; (f) assert `FlutterError.onError` was not invoked. Wire `FlutterError.onError` to a `List<FlutterErrorDetails>` collector in `setUp` and restore the original handler in `tearDown`. Existing widget-test precedent: [`app/test/presentation/screens/create/unified_create_modal_dismiss_test.dart`](../../app/test/presentation/screens/create/unified_create_modal_dismiss_test.dart) for modal-pumping shape; [`app/test/presentation/widgets/modal/glass/date_time_picker_modal_test.dart`](../../app/test/presentation/widgets/modal/glass/date_time_picker_modal_test.dart) for the `ProviderScope`-wrapped smoke pattern.
- [ ] Mirror for the Request scope: `request_screen_logout_race_test.dart`. Same shape, swapped providers.
- [ ] Confirm the test **fails before phase 2** by checking out the pre-phase-2 commit and running it; **passes after phase 2**. Document the failing-output in the PR description so reviewers can verify.
- [ ] Wire into CI: the test runs as part of the existing `npm run test:app` step. No new CI step needed.
- [ ] Acknowledge the testing-doc exception in the PR description: per [`docs/client/testing.md`](../client/testing.md), widget tests are explicitly **not** the primary layer ("Test ViewModels, not widgets"). This test is the documented exception — the assertion is against a `FlutterError` raised by the widget framework during layout, which has no viewmodel-layer analogue. If this exception becomes a pattern (more than two such tests land), file a follow-up to extend `testing.md` with a "widget-tree-assertion test" subsection.

**Validates:** Future regressions in any path (cache invalidation, FCM event
router, locale change, etc.) that races logout will be caught by this test
since the assertion is on `FlutterError.onError`, not on a specific code path.

### Phase 4 (conditional): Structural fix to the home IndexedStack

**Goal:** Eliminate the latent IndexedStack / keyed-Navigator hazard so
*any* future state change during the logout frame is safe, not just the
ones we've found so far. **Skip this phase if phase 1 + phase 2 demonstrate
the crash is gone and no related regression appears within two weeks of
shipping.**

**Tasks:**
- [ ] In [`home_screen.dart`](../../app/lib/presentation/screens/home/home_screen.dart) `_buildBody()`, pull the discover-tab `Navigator(key: ref.read(discoverNavigatorKeyProvider), ...)` out of the IndexedStack `children:` list. Place it in a sibling `Stack` whose visibility is gated by `selectedIndex == 2` via `Offstage`. The IndexedStack now only holds widgets that don't carry GlobalKeys.
- [ ] Verify tab-switch behaviour across all four tabs: tap each tab, back-navigation inside the discover stack, deep-linking via the router, scroll-to-top on re-tap. Manually exercise each scenario; no automated coverage exists today.
- [ ] Verify the navigation lock (`homeProvider.notifier.lockNav` / `unlockNav`) still suppresses the bottom nav correctly when the discover navigator pushes a route.
- [ ] **Widget test** (`app/test/presentation/screens/home/home_screen_indexed_stack_test.dart`): assert each tab renders its expected screen, switching tabs preserves state for tabs 0/1/3 and re-pushes for tab 2 only when `discoverKeyProvider.refresh()` is called, and the discover navigator's `currentState` survives a tab switch away and back.
- [ ] Delete the now-stale comment block at [`home_screen.dart:469–477`](../../app/lib/presentation/screens/home/home_screen.dart#L469) — the workaround it documents is no longer needed once the keyed Navigator is out of the IndexedStack.

**Validates:** The structural retake-inactive-element hazard is gone, not
just one trigger removed.

### Phase 5: Cleanup, docs, and the per-viewmodel guard removal

**Goal:** Remove the now-redundant per-viewmodel `_loggedOut` checks (the
pattern that already failed), update the architecture doc, and close the
loop on the issue.

**Tasks:**
- [ ] In [`experience_needs_view_model.dart`](../../app/lib/presentation/viewmodels/experience_needs_view_model.dart) and [`request_needs_view_model.dart`](../../app/lib/presentation/viewmodels/request_needs_view_model.dart), remove the `_loggedOut` getter and every `if (!ref.mounted || _loggedOut)` check. Replace with just the standard `if (!ref.mounted) return;` guards required by [`docs/client/architecture.md` §Async Safety in Notifiers](../client/architecture.md#async-safety-in-notifiers). Per [`CLAUDE.md`](../../CLAUDE.md) §Backwards-compatibility hacks, do not leave them in as defense-in-depth — they are the pattern that already failed and they obscure the load-bearing fix.
- [ ] Audit the other notifiers that adopted the same `_loggedOut` pattern (`LocationModalNotifier`, `TimeModalNotifier`, `PortfolioInboxNotifier`, `ExperienceNotifier`). Remove the per-method auth checks added in commit 624e6bc4d **only after** confirming the interlock + regression test cover the location-poll and time-poll repros from that commit. Add the equivalent regression tests for those flows before removing.
- [ ] Update [`docs/client/architecture.md` §Async Safety in Notifiers](../client/architecture.md#async-safety-in-notifiers) to add a "Logout-frame race" subsection: explain that `ref.mounted` does not detect mid-logout state, point at `state.isLoggingOut`, and link the interlock implementation. Trim any per-call-site guidance that the interlock makes redundant. Cross-link to [`docs/client/logout.md`](../client/logout.md) so the two docs reinforce each other (logout.md = teardown contract; architecture.md = how downstream code stays compatible with it).
- [ ] Update [`docs/client/caching.md`](../client/caching.md) §Pattern 7 to document the `notify()`-suppression behaviour so future contributors adding a new cache-invalidation notifier know the interlock is part of the contract and they don't need to add per-listener auth guards.
- [ ] Update [`docs/client/logout.md`](../client/logout.md) to add `state.isLoggingOut` to the documented teardown sequence (between step 1 "Clear in-memory auth state" and step 2). It currently documents the *what* of the synchronous state write but doesn't mention the interlock field — once shipped, it should.
- [ ] Run the full Dart lint + test gate per the cross-cutting conventions: `npm run lint:dart:size`, `lint:dart:a11y`, `lint:dart:i18n`, `lint:dart:l10n-parity`, `lint:nav`, `lint:ephemeral-comments`, then `flutter test --reporter expanded`.

**Validates:** The class of bug is closed and the codebase no longer
carries the failed earlier pattern alongside the fix. The architecture doc
matches reality.

## Related Issues

- #2148 — Needs & Contributions redesign; the v2 flow where this surfaces. Phase 5 of #2148 deletes the v1 sheets — coordinate so the cleanup in this issue's phase 5 happens after #2148 phase 5 to avoid touching files that are about to be deleted.
- Commit [`624e6bc4d`](https://github.com/Type2Fun-Foundation/ripls/commit/624e6bc4d) ("Fix logout crash") — prior fix for the equivalent race in location / time poll modals. The template this work borrows from and the canary for whether phase 5's guard removal is safe.

---

## Audit changelog (ripls-audit, 2026-05-25)

Audit reviewed the plan against `docs/client/{logout,caching,architecture,testing,i18n,accessibility}.md` and `docs/server/{architecture,observability}.md`. Changes applied in-place:

- **Added [`docs/client/logout.md`](../client/logout.md) to the Context docs line.** This is the most directly relevant doc — it documents the same family of Duplicate-GlobalKey crashes and the established teardown ordering — and the original plan missed it entirely.
- **Added a paragraph to the Problem Statement** quoting `logout.md`'s prior-art callout. Frames this bug as the dual of the documented "don't reset Riverpod state in `logout()`" hazard.
- **Reworded the Cross-cutting "Repository + caching" bullet** to cite `caching.md` §Pattern 7 by name and explain why suppression during logout is non-lossy (debounce never fires; next login re-issues the fetch).
- **Added a new Cross-cutting bullet for "Logout teardown ordering"** that explicitly aligns phase 2's interlock change with `logout.md` — the field gets added to the *existing* single `state = AuthStateData(...)` write in `logout()`, not as a second mutation. Heads off the most likely review objection.
- **Rewrote the Cross-cutting "Testing" bullet** to acknowledge `testing.md`'s emphatic "ViewModel-first" stance, classify phase 3 as a justified exception, and cite two existing widget-test files as precedent (`unified_create_modal_dismiss_test.dart`, `date_time_picker_modal_test.dart`). Also pointed at `app/test/helpers/l10n_helpers.dart` per `i18n.md` §Test localization.
- **Added a Cross-cutting bullet for "i18n viewmodel rule"** — viewmodels never resolve strings; phase 5's guard removal must not accidentally inline localized text.
- **Added a Cross-cutting bullet for "No server changes"** — this is a client-only bug; `server/architecture.md` and `server/observability.md` audit cleanly because no server code is touched.
- **Phase 2 task 1 (`AuthStateData.isLoggingOut`):** rewrote to explicitly state that the new flag is added to the *existing* `state = AuthStateData(...)` write, with a citation to `logout.md`. Removed the ambiguous "two write sites" wording (the second write is in `setAuthState`, not in `logout()`).
- **Phase 2 task 2 (notifier audit):** softened the wording from a confident list of `community_providers.dart` notifiers (which the audit could not verify exist as cache-invalidation notifiers, vs. just `notify()` *callers*) to an explicit audit task. Specified `_log.fine(...)` for the suppression log line.
- **Phase 3 (regression test):** added Mockito generation citation (`@GenerateMocks` + `build_runner`), `l10n_helpers.dart` reference, `pumpAndSettle()`, and a new task acknowledging the testing-doc exception in the PR description. Pointed at concrete widget-test precedent files in the codebase.
- **Phase 5 (docs update):** added a third doc-update task — extend `logout.md` itself to mention the new `state.isLoggingOut` field in the teardown sequence (currently absent). Cross-linked architecture.md ↔ logout.md so future readers find both.
