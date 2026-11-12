# Issue #800: The new user wizard screen appears too often

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/800
**Filed:** 2026-03-06 | **Last activity:** 2026-03-06
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

The onboarding wizard (4-page intro flow at `/onboarding`) reappears even after a
user has completed it or tapped "Skip." The issue title says "too often," meaning
the completion state isn't being reliably persisted or read across app sessions.

The root cause is architectural: the onboarding completion flag lives exclusively
in client-side `SharedPreferences` (`tutorial_onboarding_seen`), and the
`TutorialNotifier` has several fragility points:

1. **`build()` defaults to `hasSeenOnboarding: false`** — If the Riverpod provider
   is ever rebuilt (invalidated, hot-restarted, or ProviderScope recreated), the
   state resets to `hasSeenOnboarding: false, isLoading: true`. The `initialize()`
   call in `main.dart` only runs once at startup, so a rebuilt provider never
   recovers.

2. **Router `refreshListenable` ignores tutorial state** — The
   `AuthStateChangeNotifier` only listens to `authStateProvider`. When the tutorial
   provider finishes loading or changes state, GoRouter's redirect logic is never
   re-triggered. This creates a timing gap: if auth state settles before tutorial
   state finishes loading, the redirect sees `isTutorialLoading=true`, returns
   `null` (no redirect), and never re-evaluates once tutorial state is ready.

3. **Token refresh re-triggers redirects** — Proactive token refresh and app-resume
   refresh both update `authStateProvider`, which triggers the `refreshListenable`,
   which re-runs the redirect. If the `TutorialNotifier` was rebuilt between sessions
   or during a lifecycle event, the redirect sees `hasSeenOnboarding: false` and
   sends the user back to `/onboarding`.

4. **New `TutorialStateRepository` on every `build()`** — Each `build()` call
   creates `TutorialStateRepository()` with `_prefsInstance = null`. While the
   repository re-fetches from SharedPreferences when used, the notifier state is
   already reset to defaults and nothing triggers the re-fetch.

## Context from Comments

No comments on this issue. The description is brief: "If a user has seen it or
tapped 'skip' it should not appear again." This is a clear, unambiguous requirement.

## Potential Solutions

### Option A: Self-initializing TutorialNotifier

- **Approach:** Make `TutorialNotifier.build()` automatically load the onboarding
  state from SharedPreferences instead of relying on a separate `initialize()` call.
  Use Riverpod's async initialization pattern (either `AsyncNotifier` or trigger
  initialization inside `build()` via `Future.microtask`). Also add `tutorialProvider`
  to the router's `refreshListenable` so tutorial state changes trigger re-evaluation.
- **Pros:** Fixes all identified root causes. Provider rebuilds self-heal because
  `build()` always loads the real state. Router correctly responds to tutorial state
  changes. Minimal server changes (none).
- **Cons:** Adds a brief async loading window on provider rebuild where
  `hasSeenOnboarding` is false. The router already handles this with the
  `isTutorialLoading` guard, so this is safe.
- **Effort:** Small.

### Option B: Server-side onboarding flag

- **Approach:** Add a `has_seen_onboarding` field to the User proto and persist it
  server-side. The client reads this flag from the user profile after login instead
  of SharedPreferences. Completion writes to the server.
- **Pros:** Survives app reinstalls, device changes, and all client-side state issues.
  Single source of truth.
- **Cons:** Requires proto changes, server migration, new RPC or field on existing
  RPCs, and client changes. Over-engineered for the problem — SharedPreferences is
  the right tool for "have I shown this UI before" flags.
- **Effort:** Medium.

### Option C: Hybrid — client-side with self-healing

- **Approach:** Keep SharedPreferences as the store but make the `TutorialNotifier`
  robust: (1) auto-initialize in `build()`, (2) make `TutorialStateRepository` a
  Riverpod provider (singleton lifecycle), (3) add tutorial state to the router's
  `refreshListenable`.
- **Pros:** Fixes all root causes without server changes. Repository singleton
  prevents unnecessary SharedPreferences re-initialization. Clean Riverpod patterns.
- **Cons:** Slightly more refactoring than Option A (repository becomes a provider).
- **Effort:** Small.

## Recommendation

**Option C (Hybrid — client-side with self-healing).** This fixes every identified
failure mode with no server changes:

- Self-initializing `build()` eliminates the "initialize only called once" problem
- Repository as a provider eliminates redundant instances
- Router listening to tutorial state eliminates the timing gap

The tradeoffs are minimal — this is a straightforward Riverpod pattern fix.

## Open Questions

- **Does the onboarding need to survive app reinstall?** — Recommendation: No.
  A reinstalled app is effectively a new user experience. SharedPreferences is
  appropriate. If this changes later, Option B can be layered on.
- **Should we convert `TutorialNotifier` to `AsyncNotifier`?** — Recommendation:
  Yes, this is the cleanest Riverpod pattern for state that requires async
  initialization. The `isLoading` field becomes implicit via `AsyncValue`.

## Implementation Plan

### Phase 1: Make TutorialStateRepository a provider

**Goal:** Ensure a single repository instance across the app lifecycle, following
the established repository-as-provider pattern (see `docs/client/caching.md`
Pattern 3: Dependency Injection with Riverpod).

**Tasks:**
- [ ] Create a `tutorialStateRepositoryProvider` in
  `app/lib/data/repositories/tutorial_state_repository.dart` that provides a
  singleton `TutorialStateRepository`
- [ ] Update tests to provide the repository via provider overrides

**Validates:** No redundant `TutorialStateRepository` instances are created on
provider rebuild.

### Phase 2: Convert TutorialNotifier to AsyncNotifier

**Goal:** Eliminate the single-call `initialize()` dependency so the provider
self-heals on rebuild. `AsyncNotifier` is the recommended pattern for state
requiring async initialization (see `docs/client/architecture.md` Async Safety
section).

**Tasks:**
- [ ] Convert `TutorialNotifier` from `Notifier<TutorialState>` to
  `AsyncNotifier<TutorialState>` so `build()` can be async and load from
  SharedPreferences directly. Change provider to
  `AsyncNotifierProvider<TutorialNotifier, TutorialState>`
- [ ] Read the repository via `ref.read(tutorialStateRepositoryProvider)` in
  `build()` instead of creating a new instance
- [ ] Remove the `isLoading` field from `TutorialState` — `AsyncValue` provides
  loading/error/data states implicitly
- [ ] Remove the `initialize()` method — `build()` now handles initialization
- [ ] Remove the `await ref.read(tutorialProvider.notifier).initialize()` call
  from `main.dart` (line 188)
- [ ] Add `ref.mounted` checks after every `await` in mutation methods
  (`completeOnboarding`, `markTutorialComplete`, `resetOnboarding`, etc.)
  since `AsyncNotifier` state assignment requires it
- [ ] Update `app_router.dart` redirect to handle `AsyncValue` state:
  ```dart
  final tutorialAsync = ref.read(tutorialProvider);
  final hasSeenOnboarding = tutorialAsync.valueOrNull?.hasSeenOnboarding ?? false;
  final isTutorialLoading = tutorialAsync.isLoading;
  ```
- [ ] Update `profile_settings_screen.dart` to use `ref.watch(tutorialProvider)`
  with `.when()` or `.valueOrNull` pattern for the toggle switch
- [ ] Update `onboarding_screen.dart` — `ref.read(tutorialProvider.notifier)`
  usage remains the same since notifier access is unchanged with AsyncNotifier

**Validates:** Provider can be rebuilt at any time and always reflects the
persisted SharedPreferences state.

### Phase 3: Add tutorial state to router refreshListenable

**Goal:** Ensure the router re-evaluates redirects when tutorial state changes.

**Tasks:**
- [ ] Rename `AuthStateChangeNotifier` to `RouterRefreshNotifier` to reflect
  its broader scope
- [ ] Add `ref.listen(tutorialProvider, ...)` alongside the existing auth state
  listener so tutorial state changes trigger `notifyListeners()`
- [ ] Verify that completing onboarding correctly redirects away from
  `/onboarding` even without the manual `context.go('/')`

**Validates:** Auth state changes (token refresh, app resume) no longer cause
the wizard to reappear. Tutorial state changes correctly trigger routing.

### Phase 4: Update tests

**Goal:** Ensure all changes are covered by tests.

**Tasks:**
- [ ] Update `tutorial_view_model_test.dart` for the `AsyncNotifier` pattern
  — state is now `AsyncValue<TutorialState>`, use `container.read(tutorialProvider)`
  which returns `AsyncValue`, assert with `.value` or `.valueOrNull`
- [ ] Update `tutorial_state_repository_test.dart` if the repository API changed
- [ ] Add a test verifying that a rebuilt `TutorialNotifier` correctly loads
  persisted state (the core regression test — invalidate provider, verify it
  re-loads from SharedPreferences)
- [ ] Add a disposal safety test: start async operation, dispose container,
  verify no crash (required by architecture doc for all async notifiers)
- [ ] Update `profile_settings_screen_test.dart` for `AsyncValue` state shape
- [ ] Verify existing router tests (if any) still pass

**Validates:** The fix is covered by automated tests and the "appears too often"
regression cannot recur.

## Architecture Audit

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/server/architecture.md`

**Findings:**

1. **AsyncNotifier is the correct pattern.** The architecture doc explicitly
   recommends `AsyncNotifier` for state requiring async initialization (Async
   Safety section). The plan's conversion aligns with this guidance.

2. **Repository as provider aligns with established patterns.** The caching doc
   (Pattern 3) shows repositories provided via Riverpod providers. The current
   `TutorialStateRepository` is instantiated directly in `build()`, which
   violates this pattern. Note: this repository uses `SharedPreferences`, not
   Stash, which is correct — it stores local UI state, not API response data.

3. **Phase ordering corrected.** Repository provider (Phase 1) must be created
   before the AsyncNotifier conversion (Phase 2) since the notifier depends on
   the repository provider. Original plan had these reversed.

4. **`ref.mounted` checks required.** The architecture doc mandates `ref.mounted`
   checks after every `await` in `AsyncNotifier` mutation methods. Added to
   Phase 2 tasks.

5. **Disposal safety test required.** Architecture doc requires disposal tests
   for all async notifiers. Added to Phase 4 tasks.

6. **Router accessing provider is an acceptable exception.** The architecture
   rule "Screens only interact with ViewModels" applies to the presentation
   layer. The router is infrastructure (`core/router/`) and legitimately reads
   tutorial state for redirect logic.

7. **No server changes needed.** SharedPreferences is the correct persistence
   layer for "have I shown this UI" flags. Server-side persistence (Option B)
   would be over-engineering per the architecture principle of minimal
   dependencies.

**No compliance issues found. Plan is architecturally sound.**

## Related Issues

- #757 — Refresh token auth (added proactive token refresh which triggers auth
  state changes that re-run the router redirect — a contributing trigger for
  the wizard reappearing)
