# Issue #1036: [Bug Report] chat notifications don't deep link to chat tab

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1036
**Filed:** 2026-04-05 | **Last activity:** 2026-04-05
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

Chat message notifications don't deep link to the chat tab of the relevant
item. Tapping a chat notification just opens the app (to the home screen)
instead of navigating to the gear/experience/request detail screen with the
Chat tab selected.

This is surprising because PR #978 (merged 2026-04-01) added exactly this deep
linking capability. The server-side payload construction in
`server/services/chat/notifications.go` correctly populates entity IDs
(`gear_id`, `experience_id`, `request_id`) from the conversation topic, and the
client-side routing in `routeNotificationPayload()` correctly builds routes like
`/gear/<id>?tab=chat`. The routing logic itself is well-tested in
`app/test/services/fcm_navigation_test.dart`.

The bug must be in the **navigation execution** path — between
`routeNotificationPayload` generating the route string and the router actually
displaying the screen.

## Context from Comments

No comments on this issue.

## Root Cause Analysis

The notification tap handler in `app/lib/main.dart:383-395` uses a two-step
navigation pattern:

```dart
fcmService.onNavigate = (String route) {
  debugPrint('FCM navigation request: $route');
  if (route.startsWith('/gear/') || ...) {
    final router = ref.read(routerProvider);
    router.go('/');                                    // Step 1: go home
    WidgetsBinding.instance.addPostFrameCallback((_) {
      router.push(route);                              // Step 2: push item
    });
  } else {
    final router = ref.read(routerProvider);
    router.go(route);
  }
};
```

There are two likely failure modes:

### Failure mode 1: `push()` dropping query parameters

GoRouter's `push()` pushes a new page onto the navigation stack. While it
should parse URI strings including query parameters, its behavior with query
params has been inconsistent across GoRouter versions. If `push('/gear/abc?tab=chat')`
parses as path `/gear/abc?tab=chat` (with `?tab=chat` as part of the path
segment) rather than path `/gear/abc` with query `tab=chat`, the route won't
match the `/gear/:id` pattern and navigation silently fails.

This would explain the observed behavior: the app opens to home (step 1
succeeds) but the push (step 2) fails to match a route.

### Failure mode 2: Timing — frame callback lost during route rebuild

`router.go('/')` replaces the entire navigation stack and triggers a full
widget tree rebuild. The `addPostFrameCallback` fires after the next frame, but
if the router is still settling (e.g., redirect guards, auth checks) the
`push()` may execute against a partially-rebuilt navigation state.

### Why this wasn't caught

The `routeNotificationPayload` unit tests verify the route string is correct
(`/gear/gear-123?tab=chat`) — which it is. But there are no integration tests
that verify GoRouter actually navigates when given that route via `push()`.

## Potential Solutions

### Option A: Replace go+push two-step with single `go(route)`

- **Approach:** Replace the `go('/') → push(route)` two-step with a single
  `router.go(route)`. Back navigation is already handled — all item detail
  routes wrap their screens in `PopScope` (added in PR #978) which intercepts
  back and calls `context.go('/')`.
- **Pros:** Eliminates both failure modes. Single atomic navigation. No frame
  timing issues. `go()` is GoRouter's primary navigation method and handles
  query params reliably. Simpler code.
- **Cons:** Navigation stack is `[item]` instead of `[home, item]`, but PopScope
  already handles back-to-home, so this is functionally equivalent.
- **Effort:** Small — ~10 lines changed.

### Option B: Keep two-step but use `goNamed` with query params

- **Approach:** Replace `push(route)` with `goNamed('gear', pathParameters: {'id': gearId}, queryParameters: {'tab': 'chat'})`. Parse the route string in the
  callback to extract components.
- **Pros:** Explicitly passes query params, avoids URI parsing ambiguity.
- **Cons:** Requires parsing the route string back into components in the
  callback, duplicating routing logic. More code, more complexity. Still has the
  frame timing issue.
- **Effort:** Medium.

### Option C: Add diagnostic logging first, then fix

- **Approach:** Add logging at every step (route generation, callback execution,
  router state) to confirm the exact failure point, then apply the targeted fix.
- **Pros:** Confirms root cause before changing behavior.
- **Cons:** Requires a diagnostic build, user testing, and a second code change.
  Slower turnaround.
- **Effort:** Small per step, but elapsed time is longer.

## Recommendation

**Option A.** The `go('/') → addPostFrameCallback → push(route)` pattern is
unnecessarily complex and fragile. A single `router.go(route)` is simpler,
more reliable, and produces identical UX because PopScope already handles
back navigation. Even if the exact failure mode turns out to be different,
this change eliminates the known risks.

Add diagnostic logging as part of the fix (not as a separate phase) to aid
future debugging.

## Open Questions

- **Does `router.push()` actually drop query params in our GoRouter version?**
  — Recommendation: Don't investigate — Option A sidesteps this entirely. The
  fix is the same regardless.

- **Are there notification scenarios where no entity ID is populated?** — 
  Recommendation: Yes — community-only conversations (no topic) have no
  `gear_id`/`experience_id`/`request_id`. `routeNotificationPayload` correctly
  does nothing in this case (no screen to navigate to). This is acceptable for
  now since community-level chat is rare. Track as a separate enhancement if
  needed.

- **Should we handle the terminated-state launch path differently?** —
  Recommendation: No. The `getInitialMessage()` flow calls the same
  `onNavigate` callback. With `router.go(route)`, the terminated-state path
  becomes simpler — a single navigation call once the app is initialized. The
  current code already sets `onNavigate` before calling `initialize()`, so the
  callback is available when `getInitialMessage()` resolves.

## Implementation Plan

### Phase 1: Fix navigation and add logging
**Goal:** Chat notification taps navigate to the correct item with Chat tab.
**Tasks:**
- [ ] In `app/lib/main.dart`, replace the `go('/') → addPostFrameCallback →
  push(route)` pattern with a single `router.go(route)` call for item routes
- [ ] Add structured logging to `routeNotificationPayload()` in
  `fcm_service.dart` — log the incoming data keys and the generated route
  (or "no matching entity ID" if none match)
- [ ] In the `onNavigate` callback, replace `debugPrint` with `_log.info()`
  for structured logging consistency with the rest of `main.dart`
- [ ] Remove the `else` branch's redundant `router.go(route)` and merge into
  a single path (all routes now use `go()`)
- [ ] Run `flutter test --reporter expanded app/test/services/fcm_navigation_test.dart`
  to confirm routing logic still passes
**Validates:** Tapping a chat notification on Android opens the item detail
screen with the Chat tab selected.

### Phase 2: Add navigation integration test
**Goal:** Prevent regression with a test that exercises GoRouter with the
notification route strings.
**Tasks:**
- [ ] Add a widget test that creates a GoRouter with the item route patterns
  (`/gear/:id`, `/experience/:id`, `/request/:id`) using lightweight stub
  widgets that capture the parsed `initialTab` value — avoids wiring up real
  screen dependencies (architecture.md §Testing: don't test UI and logic
  together)
- [ ] Test `router.go('/gear/test-id?tab=chat')` and verify stub receives
  `initialTab: 2`
- [ ] Add equivalent tests for `/experience/:id?tab=chat` and
  `/request/:id?tab=chat`
- [ ] Add a test for routes without `?tab=chat` to verify `initialTab: 0`
**Validates:** GoRouter correctly parses `?tab=chat` query parameter via `go()`.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

### Issues found and fixed

1. **Navigation pattern compliance (architecture.md §Navigation & Gestures):**
   The architecture doc reserves `context.push()` for auth flows only and
   requires `NavigationHelpers` for in-app forward navigation. The current buggy
   code uses `router.push()` from the FCM callback in `main.dart` — this is a
   programmatic router call for an external entry point (notification tap), not
   in-app navigation from a widget. The proposed fix (`router.go()`) is correct
   for external entry points and matches how the GoRoute builders + PopScope are
   already configured. **No change needed — plan is compliant, but added this
   note for clarity.**

2. **Phase 2 test scope (architecture.md §Testing):** Original plan proposed
   widget tests rendering full GearScreen/ExperienceScreen/RequestScreen, which
   have heavy provider dependencies (repositories, services, viewmodels). This
   violates the "don't test UI and logic together" guideline. **Updated Phase 2
   to test GoRouter route matching and query parameter parsing with lightweight
   stub widgets**, avoiding the need to wire up real screen dependencies.

3. **Logging consistency:** The `onNavigate` callback uses `debugPrint` while
   the rest of `main.dart` uses the `_log` Logger instance. **Updated Phase 1
   tasks to use `_log` (structured logging) instead of `debugPrint`.**

### No issues

- **Caching:** No caching changes. Notification routing is stateless.
- **i18n:** No new user-visible strings. Notification titles/bodies are
  server-generated.
- **Server architecture:** No server changes. Server-side payload construction
  verified as correct (entity IDs populated from conversation topic).
- **Proto boundary:** Client reads raw FCM data fields (`Map<String, dynamic>`),
  not generated models code. No proto boundary violations.

## Related Issues
- #962 — Chat notifications lead to old chat UI (fixed by PR #978)
- #977 — Non-chat notifications deep linking (separate, covers community events
  and experience lifecycle notifications)
