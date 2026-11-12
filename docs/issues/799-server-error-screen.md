# Issue #799: Client needs new server error screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/799
**Filed:** 2026-03-06 | **Last activity:** 2026-03-06
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When the server is unreachable, the app displays a broken error experience with
multiple issues:

1. **Error banner clipped at top of screen** — The community error banner is
   `Positioned(top: 0)` in the home screen's `Stack`, placing it behind the
   system status bar. Users can't read the full message or tap "Retry."
2. **No way to report a problem** — There's no contact link, support email, or
   bug-report action anywhere in the error state.
3. **Misleading guidance** — The error text suggests creating a community, which
   requires a working server — the exact thing that's down.
4. **Bottom nav still active** — Users can tap into other tabs (Discover, Metrics,
   Daily) which also fail, creating a cascade of broken screens instead of a
   single clear error state.

The root cause is that the app treats a total server outage the same way it
treats a single failed API call — a small banner on the home screen. A full
server outage needs a dedicated, full-screen error experience.

## Context from Comments

No comments on this issue. The issue is freshly filed with a clear problem
description and four specific failure modes. No additional input is needed to
proceed.

## Potential Solutions

### Option A: Full-screen server error overlay

- **Approach:** Detect server unreachability at the community-loading level (the
  first RPC after login). When the server is unreachable, display a full-screen
  error page that replaces the entire home screen (including bottom nav). The
  page shows a clear message, a retry button, and a "Report a Problem" link
  (email or URL). When retry succeeds, transition back to the normal home screen.
- **Pros:** Clean separation — users see exactly one error state, not a broken
  app. Bottom nav is hidden so users can't navigate to other broken screens.
  Simple to implement — just a conditional in the home screen build.
- **Cons:** If the server is only partially down (one service failing), this may
  be too aggressive.
- **Effort:** Small.

### Option B: Connectivity-aware error router

- **Approach:** Add a health-check ping (e.g., a lightweight RPC or HTTP GET to
  the server). If the ping fails, redirect to a dedicated `/server-error` route
  that blocks all navigation. If the ping succeeds but individual RPCs fail,
  keep the current per-screen error handling.
- **Pros:** Distinguishes between "server is down" and "one call failed." More
  precise error reporting.
- **Cons:** Requires a new health endpoint on the server. Adds complexity
  (health check polling, route guards). Over-engineered for the current problem.
- **Effort:** Medium.

### Option C: Enhanced error banner with safe area

- **Approach:** Fix the existing error banner to respect safe area insets, add a
  "Report a Problem" link, remove the misleading "create a community" text, and
  disable bottom nav tabs when in error state.
- **Pros:** Minimal change to existing code.
- **Cons:** A banner is the wrong UX for a full server outage — it implies a
  minor issue. Doesn't solve the fundamental problem that users see a broken app
  with empty screens behind the banner.
- **Effort:** Small.

## Recommendation

**Option A: Full-screen server error overlay.** It directly addresses all four
issues in the bug report with minimal complexity. The existing error detection
(community load failure in `main.dart`) is sufficient — if the very first RPC
after login fails, the server is effectively down for this user. No new server
endpoint needed. The "partial outage" concern from Option B is theoretical;
in practice, if community loading fails, nothing else will work either.

## Open Questions

- **What should "Report a Problem" link to?** — Recommendation: A `mailto:` link
  to a support email address (e.g., `support@ripls.app`), with the app version
  and device info pre-filled in the subject line. This works offline and doesn't
  depend on the server.
- **Should there be automatic retry with backoff?** — Recommendation: No. Keep
  it manual (tap to retry). Automatic retry adds complexity and battery drain
  for a rare edge case. The user is already looking at the screen.
- **Should the error screen distinguish network-offline from server-down?** —
  Recommendation: Yes, minimally. If the device has no network connectivity
  (detected via `connectivity_plus`), show "You're offline" with appropriate
  messaging. If the device is online but the server is unreachable, show
  "Server unavailable." Both use the same full-screen layout but with different
  copy. The existing `ConnectivityBanner` and `connectivityStatusProvider`
  already provide this signal.

## Audit Notes

**Architecture compliance review** against `docs/client/architecture.md` and
`docs/client/caching.md`:

1. **[FIXED] Screens must not access repositories directly.** The existing
   `_buildCommunityErrorBanner` retry handler in `home_screen.dart` calls
   `ref.read(communityRepositoryProvider).listUserCommunities()` directly from
   the widget — violating the rule "ViewModels are the ONLY layer that calls
   Repositories" and "Screens only interact with ViewModels." The plan now
   requires the retry to go through `SelectedCommunityNotifier` (which already
   has `setLoading`, `setCommunities`, `setError` methods). A new
   `retryLoadCommunities()` method on the notifier encapsulates the
   repository call.
2. **OK — No caching concerns.** This is a presentation-layer change. No new
   repositories, services, or cache keys are introduced.
3. **OK — No server changes.** This is purely a client-side fix. No proto or
   server changes needed.
4. **OK — Widget placement.** `ServerErrorScreen` goes in
   `presentation/widgets/` as a reusable pure-presentation component. It
   receives callbacks and data via constructor — no business logic inside.

## Implementation Plan

### Phase 1: Server error screen widget

**Goal:** Create a reusable full-screen error widget and integrate it into the
home screen.

**Tasks:**
- [ ] Add a `retryLoadCommunities()` method to `SelectedCommunityNotifier` in
      `providers.dart` that encapsulates the repository call, loading state, and
      error handling. This moves the retry logic out of the widget and into the
      ViewModel layer where it belongs (architecture rule: screens only interact
      with ViewModels, ViewModels are the only layer that calls repositories).
- [ ] Create `app/lib/presentation/widgets/server_error_screen.dart` — full-screen
      `StatelessWidget` with: centered error icon, "Something went wrong" title,
      descriptive message, prominent "Try Again" button, "Report a Problem"
      mailto link, app version display. Must respect `SafeArea`. No bottom nav
      visible. Pure presentation — receives `errorMessage`, `onRetry` callback,
      and `isOffline` flag via constructor.
- [ ] Modify `home_screen.dart` — when `communityState.errorMessage != null`,
      return the `ServerErrorScreen` instead of the normal `Stack` with
      `IndexedStack` + bottom nav. The retry callback calls
      `ref.read(selectedCommunityProvider.notifier).retryLoadCommunities()`.
- [ ] Remove the existing `_buildCommunityErrorBanner` method and its
      `Positioned` widget from the home screen (replaced by full-screen).
- [ ] Differentiate network-offline vs server-unreachable using the existing
      `connectivityStatusProvider`. Pass `isOffline` to `ServerErrorScreen` to
      show "You're offline" vs "Server unavailable" with appropriate icons and
      copy.
- [ ] Write widget tests for `ServerErrorScreen` — verify retry callback fires,
      report link renders, both offline and server-error variants display
      correct copy.

**Validates:** The error screen is visible, readable, and actionable. Bottom nav
is gone. Users can retry and report problems. Retry logic lives in the ViewModel
layer, not in widgets.

### Phase 2: Error message cleanup

**Goal:** Remove misleading text and ensure error messages are helpful.

**Tasks:**
- [ ] Audit all "Failed to load communities" messages in `main.dart`,
      `home_screen.dart`, `login_screen.dart`, `sidebar.dart`, and
      `profile_communities_screen.dart`. Replace with clear, non-misleading text.
- [ ] Remove any suggestion to "create a community" from error states.
- [ ] Ensure the `rpc_error_handler.dart` transport error messages are used
      consistently (they already have good copy for socket/timeout errors).
- [ ] Fix any other places where screens call repositories directly for
      community retry logic (sidebar, profile) — route through
      `SelectedCommunityNotifier.retryLoadCommunities()` instead.

**Validates:** No error message suggests an action that requires the server.
All retry logic flows through ViewModels, not directly from widgets.

## Related Issues

- None identified.
