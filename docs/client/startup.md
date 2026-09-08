---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client startup sequence — multi-phase cold-start and post-login init (Firebase, auth check, community loading, service init, GPS), adaptive splash screen, GoRouter auth-based routing, and resume refresh.
  globs: [app/lib/main.dart, app/lib/core/router/**, app/lib/presentation/screens/splash/**]
  triggers: [startup, cold-start, splash, initialization, routing, app-resume, bootstrap]
  lens: [client, architecture]
  domain: client
freshness:
  verified_commit: "5c2d3e59c"
  verified_on: "2026-07-07"
---
# Client Application Startup

## Table of Contents

1. [Overview](#overview)
2. [Architecture Principles](#architecture-principles)
3. [Startup Flow](#startup-flow)
   - [Cold Start Flow](#cold-start-flow-app-launch)
   - [Post-Login Flow](#post-login-flow-authentication-change)
4. [Initialization Phases](#initialization-phases)
   - [Phase 1: Firebase Initialization](#phase-1-firebase-initialization)
   - [Phase 2: Authentication Check](#phase-2-authentication-check)
    - [Phase 3: Tutorial State Loading (removed)](#phase-3-tutorial-state-loading-removed)
   - [Phase 4: Community Loading](#phase-4-community-loading)
   - [Phase 5: Service Initialization](#phase-5-service-initialization)
   - [Phase 5b: Location & GPS at Startup](#phase-5b-location--gps-at-startup)
   - [Phase 6: Splash Screen Management](#phase-6-splash-screen-management)
5. [Routing & Navigation](#routing--navigation)
   - [Authentication-Based Routing](#authentication-based-routing)
   - [Onboarding Redirect Logic (removed)](#onboarding-redirect-logic-removed)
   - [Community Selection Persistence](#community-selection-persistence)
6. [Observability Integration](#observability-integration)
7. [Error Handling](#error-handling)
8. [Performance Optimization](#performance-optimization)
9. [Testing](#testing)
10. [Architecture Strengths & Weaknesses](#architecture-strengths)
11. [Future Improvements](#future-improvements)
12. [Appendix: Key Components](#appendix)

---

## Overview

The Ripls client app follows a multi-phase initialization sequence that handles Firebase setup, authentication verification, community data loading, and observability configuration. The startup process uses an adaptive splash screen (minimum 1.5s for branding, maximum 5s to prevent hanging) with progressive loading indicators showing real-time status. The system includes comprehensive error handling with retry mechanisms, performance monitoring with granular Firebase traces, and automatic community refresh on app resume.

The system handles three primary startup scenarios:

1. **Cold Start (Unauthenticated):** New or logged-out users see splash screen with progress indicator → login (or `/register` on first launch, or `/invite` when a deferred deep-link context is present).
2. **Cold Start (Authenticated):** Existing users see splash screen → home screen with communities pre-loaded.
3. **Post-Login Start:** Users who log in during the session trigger community loading via the auth-state listener in `GearApp.build()` and navigate to home.

> **Onboarding status.** The 4-page onboarding tutorial was disabled in #983 (closed 2026-04-03) and its code — `TutorialNotifier`, `TutorialStateRepository`, `OnboardingScreen` — has since been deleted. Startup no longer loads tutorial state and the router no longer redirects to `/onboarding`.

## Architecture Principles

**Adaptive Splash Duration:** Minimum 1.5s for branding visibility, maximum 5s to prevent hanging. If loading takes 2-4.9s, proceeds immediately without artificial delay. Logs warning if initialization exceeds 5s.

**Progressive Status Feedback:** Splash screen shows determinate progress indicator (20%, 40%, 60%, 80%, 100%) matching each loading phase, giving users real-time visibility into startup progress.

**Async-First Initialization:** All data loading operations are asynchronous with granular Firebase Performance traces (`startup_total_init`, `startup_auth_check`, `startup_load_communities`) for bottleneck identification.

**Graceful Error Recovery:** Failed community loading sets error state with a retry button in the HomeScreen error banner. App remains functional - users aren't blocked by network failures.

**Single-Path Post-Login Load:** Login screen no longer prefetches communities. Both cold-start and post-login loading go through the same `_loadCommunities()` helper in `main.dart`. The auth-state listener (`ref.listen(authStateProvider)` in `GearApp.build()`) is gated by an `_initialLoadComplete` flag so it only fires for post-startup auth changes (fresh login), never during cold start where the `initState()` microtask already handles it. A brief prefetch attempt from the login screen existed previously but was removed after it caused a race condition — duplicate `setCommunities()` calls that could orphan the initial search.

**App Resume Refresh:** When app returns from background, communities automatically refresh to catch invitations or removals that occurred while app was inactive. On resume, cached user position is also invalidated (both `LocationIntent` slots) if the app was paused longer than `_positionStalenessThreshold` — see [Location & GPS at Startup](#location--gps-at-startup) below.

**Auth State Listener:** Riverpod `ref.listen(authStateProvider)` in `GearApp.build()` detects authentication state changes to trigger community loading after login. Guarded by `_initialLoadComplete` so it is exclusively for post-startup transitions.

**Router-Driven Navigation:** GoRouter handles all navigation logic based on authentication state and current location, eliminating manual route management.

**Parallel Service Initialization:** Non-critical services (FCM, presence, unread count) initialize in background without blocking main startup sequence.

**Centralized State Management:** Riverpod providers manage authentication, community (with loading/error states), and splash states globally, ensuring consistency across the app.

## Startup Flow

### Cold Start Flow (App Launch)

**Example: User opens app for the first time**

**Reference:** [main.dart:154-275](../app/lib/main.dart#L154-L275)

```
User launches app
    ↓
main() entry point
    ├─ Configure logging (Environment.configureLogging)
    ├─ Start app startup trace (Performance.startTrace('app_startup'))
    ├─ Initialize Firebase (Firebase.initializeApp)
    └─ Run app in error zone with crash reporting
    ↓
GearApp widget initialization
    ├─ Record app start time (DateTime.now())
    ├─ Register lifecycle observer (WidgetsBindingObserver)
    └─ Schedule post-frame callback to stop startup trace
    ↓
Future.microtask() initialization sequence
    ├─ Start Firebase trace: 'startup_total_init'
    ├─ Start Firebase trace: 'startup_auth_check' (20% progress)
    │
    ├─ Update splash: SplashLoadingStep.checkingAuth
    │
    ├─ Load authentication state
    │   └─ AuthStateNotifier.loadAuthState()
    │       ├─ Read JWT token from SecureStorage
    │       ├─ Parse user info from token
    │       └─ Update authState (isAuthenticated: true/false)
    │
    ├─ Stop Firebase trace: 'startup_auth_check' (40% progress)
    │
    └─ Check authentication result
        │
        ├─ IF authenticated:
        │   ├─ Initialize FCM (background, fire-and-forget)
        │   ├─ Update presence (background, fire-and-forget)
        │   ├─ Start Firebase trace: 'startup_load_communities' (60% progress)
        │   ├─ Update splash: SplashLoadingStep.loadingCommunities
        │   ├─ Set loading state: CommunitiesNotifier.setLoading(true)
        │   ├─ Load communities
        │   │   └─ CommunityRepository.listUserCommunities()
        │   │       ├─ Check cache (Stash in-memory)
        │   │       ├─ Fetch from server if cache miss
        │   │       └─ Return List<CommunityItem> OR throw error
        │   ├─ Handle result
        │   │   ├─ SUCCESS: Set communities in state (80% progress)
        │   │   │   └─ CommunitiesNotifier.setCommunities()
        │   │   │       ├─ Clear loading state, clear any errors
        │   │   │       └─ Store the full portfolio list (no per-community
        │   │   │          selection to restore post-#1895)
        │   │   └─ ERROR: Set error state
        │   │       └─ CommunitiesNotifier.setError("Failed to load...")
        │   │           ├─ Clear loading state
        │   │           ├─ Set errorMessage (shown in UI)
        │   │           └─ Log error but continue (defensive continuation)
        │   ├─ Stop Firebase trace: 'startup_load_communities'
        │   └─ Continue to splash duration check
        │
        └─ IF not authenticated:
            └─ Continue to splash duration check
    ↓
Adaptive splash screen duration (min 1.5s, max 5s)
    ├─ Calculate elapsed time since app start
    ├─ IF elapsed < 1.5s:
    │   └─ Wait remaining time (ensure branding visibility)
    ├─ ELSE IF elapsed > 5s:
    │   └─ Log warning: "App initialization took Xms (exceeds max 5000ms)"
    └─ ELSE (1.5s ≤ elapsed ≤ 5s):
        └─ Proceed immediately (no artificial delay)
    ↓
Update splash: SplashLoadingStep.ready (100% progress)
Stop Firebase trace: 'startup_total_init'
    ↓
GoRouter redirect logic executes
    ├─ Wait for loading state (authState.isLoading)
    │
    ├─ IF not authenticated:
    │   └─ Redirect to /login
    │
    └─ IF authenticated:
        └─ Navigate to / (home screen)
    ↓
User sees appropriate screen
    ├─ LoginScreen (if not authenticated)
    └─ HomeScreen (if authenticated)
        └─ Communities already loaded and selected
```

### Post-Login Flow (Authentication Change)

**Example: User logs in from LoginScreen**

**References:**
- [login_screen.dart:62-84](../app/lib/presentation/screens/auth/login_screen.dart#L62-L84) (navigation only — no community prefetch)
- [main.dart:509-519](../app/lib/main.dart#L509-L519) (auth listener)
- [main.dart:282-314](../app/lib/main.dart#L282-L314) (`_loadCommunities()`)

```
User submits login credentials
    ↓
LoginScreen calls AuthService.login()
    ├─ Send credentials to server
    ├─ Receive JWT token
    └─ Save token to SecureStorage
    ↓
AuthStateNotifier.setAuthState() updates state
    ├─ authState.isAuthenticated changes: false → true
    └─ Riverpod notifies all listeners
    ↓
LoginScreen calls context.go(destination) to navigate away
    (no community prefetch from the login screen — removed due to
     race condition with the auth-state listener)
    ↓
ref.listen(authStateProvider) in GearApp.build() fires
    ├─ Guard: _initialLoadComplete must be true (skips during cold start)
    ├─ Condition: !previous.isAuthenticated && next.isAuthenticated
    ├─ Log: "Auth state changed to authenticated, loading communities"
    ├─ _initializeFCM() (fire-and-forget; will prompt for notification
    │                    permission if not yet granted)
    └─ _loadCommunities()
        ├─ Clear per-user Riverpod state from any prior session
        │   ├─ communitiesProvider.notifier.clear()
        │   └─ workshopCommunityProvider.notifier.select(null)
        ├─ CommunitiesNotifier.setLoading(true)
        ├─ CommunityRepository.listUserCommunities()
        ├─ CommunitiesNotifier.setCommunities()
        │   └─ Sets the full portfolio list and clears legacy
        │       SharedPreferences selection keys (post-#1895 there is
        │       no selected-community filter to restore)
        ├─ Start community event pollers + streams for real-time updates
        └─ Log: "Community selection initialized"
    ↓
GoRouter redirect logic re-evaluates
    ├─ isAuthenticated = true → no redirect away from /
    └─ Navigate to / (home screen)
    ↓
User sees HomeScreen with communities loaded
```

**Why the upfront state clear?** `logout()` does **not** reset per-user Riverpod notifiers (cache + SharedPreferences clearing is its job — see [logout.md](logout.md)). If a different user logs back in on the same device, the prior user's `communitiesProvider.communities` and `workshopCommunityProvider` carousel pin are still in memory until `setCommunities(newList)` resolves. During that window, anything reading `workshopEnabledCommunityIdsProvider` (e.g. the Workshop view-model) sees the prior user's circle IDs and issues RPCs the new user has no permission for — `GetWorkshopBrief` / `GetWorkshopSynthesis` come back as `permission_denied`. Clearing the notifiers as the very first step of `_loadCommunities()` collapses that window: every downstream consumer reads an empty list during the fetch, the Workshop view-model short-circuits on `communityIds.isEmpty`, and no stale-IDs RPC ever fires.

## Initialization Phases

### Phase 1: Firebase Initialization

**Purpose:** Initialize Firebase services (Analytics, Crashlytics, Performance) before app renders.

**When:** `main()` function, before `runApp()`

**Key Actions:**
- Configure structured logging with log levels
- Start app startup performance trace
- Initialize Firebase app
- Setup crash reporting error zone

**Files:**
- [main.dart:92-136](../app/lib/main.dart#L92-L136)
- [environment.dart](../app/lib/core/config/environment.dart)

**Critical:** Firebase must initialize before any Firebase service is accessed, so this happens synchronously before the widget tree builds.

### Phase 2: Authentication Check

**Purpose:** Determine if user has valid authentication token.

**When:** `initState()` via `Future.microtask()`

**Key Actions:**
- Update splash state: "Checking authentication..."
- Read JWT token from SecureStorage
- Parse user information from token
- Update `authStateProvider` with authentication status

**Files:**
- [main.dart:164-174](../app/lib/main.dart#L164-L174)
- [auth_state.dart](../app/lib/services/auth_state.dart)

**Result:** `authState.isAuthenticated` determines routing destination.

### Phase 3: Tutorial State Loading (removed)

> **Removed.** Onboarding was turned off in #983 and its code — the tutorial
> viewmodel, the `tutorial_onboarding_seen` repository, and the onboarding
> screen — was deleted once nothing referenced it. Startup no longer reads
> tutorial state, and the router no longer waits on it.

### Phase 4: Community Loading

**Purpose:** Fetch user's communities and restore last selected community with comprehensive error handling.

**When:**
- **Cold Start:** After authentication check if `isAuthenticated = true`
- **Post-Login:** Via `ref.listen(authStateProvider)` only (the LoginScreen prefetch was removed — see "Single-Path Post-Login Load")
- **App Resume:** When app returns to foreground from background

**Key Actions:**
- Clear per-user Riverpod state from any prior session (post-login path only;
  no-op on cold start because the notifiers default to empty)
  - `CommunitiesNotifier.clear()` — drops the prior user's community list
  - `WorkshopCommunityNotifier.select(null)` — drops the Workshop carousel
    selection (a circle ID that would otherwise leak into Workshop RPCs)
  - Add any new per-user notifiers here, not in `logout()` — see
    [logout.md](logout.md) for why the cleanup site is the login path.
- Set loading state: `CommunitiesNotifier.setLoading(true)`
- Update splash state: "Loading communities..." (cold start only)
- Call `CommunityRepository.listUserCommunities()`
  - Check Stash in-memory cache
  - Fetch from server if cache miss/expired
  - Cache results with global TTL (30 minutes default)
  - Throw error on network failure
- Handle success: `CommunitiesNotifier.setCommunities()`
  - Clear loading state and error message
  - Store the full portfolio list (post-#1895 there is no
    selected-community filter; `setCommunities` also removes the legacy
    `selected_community_id` / `enabled_community_ids` SharedPreferences keys)
- Handle error: `CommunitiesNotifier.setError(message)`
  - Clear loading state
  - Set error message for UI display
  - Log error but continue (defensive continuation)

**Files:**
- [main.dart:222-240](../app/lib/main.dart#L222-L240) (cold start authenticated branch)
- [main.dart:509-519](../app/lib/main.dart#L509-L519) (post-login listener, guarded by `_initialLoadComplete`)
- [main.dart:282-314](../app/lib/main.dart#L282-L314) (`_loadCommunities()` method)
- [main.dart:396-415](../app/lib/main.dart#L396-L415) (`_refreshCommunitiesOnResume()` method)
- [community_repository.dart](../app/lib/data/repositories/community_repository.dart)
- [community_providers.dart](../app/lib/services/providers/community_providers.dart) (`CommunitiesState` with error tracking)

**Error Handling:**
- Failed loading sets `errorMessage` in state
- HomeScreen displays error banner with retry button at top
- App remains functional - users can retry or use app with limited functionality

**State Management:**
```dart
class CommunitiesState {
  final List<CommunityItem> communities; // the full portfolio
  final bool isLoading;
  final String? errorMessage;
}
```

**Logging:**
- "Starting community loading for authenticated user"
- "Fetching user communities from cache or server (cacheKey: user:list)"
- "Community fetch completed: N communities returned"
- "Setting communities: N communities received"
- "Persisted community ID: [id or none]"
- "Community selection finalized: [name] ([id])"
- "Failed to load communities during initialization" (on error)
- "Refreshing communities on app resume" (app resume)
- "Communities refreshed on resume: N communities" (app resume success)

### Phase 5: Service Initialization

**Purpose:** Initialize non-critical background services.

**When:** After authentication check, parallel with community loading

**Key Actions:**
- Initialize FCM (Firebase Cloud Messaging) — `_initializeFCM()`
  - Set up message handlers (must happen even if the rest of init fails, so notification deep links keep working)
  - **Request notification permission** (`FirebaseMessaging.requestPermission`) — this is the only permission prompt during cold start
  - Initialize local notifications plugin (Android + iOS)
  - Register device token with server (wrapped separately; failure does not disable handlers)
  - Wire `onNavigate` (for notification tap deep links) and `onCommunityEvent` (routes FCM events through `EventRouter`)
- Update user presence (online status) via `_updatePresence()`
- Start community event pollers and streams (per-community), triggered from `_loadCommunities()` after `setCommunities` resolves
  - `CommunityEventPoller.startAll()` — 30s staggered poll backstop
  - `CommunityEventStreamService.connectAll()` — gRPC server-streams for sub-second updates
  - Both route events through `EventRouter` for deduplication and cache invalidation
  - Both restart on app resume, stop on app pause (see lifecycle handling below)
  - Both reset on logout before cache clearing (see [logout.md](logout.md))
  - New communities get poller + stream immediately via `onCommunityJoined` callback
- Schedule unread count initialization with a 500ms delay after `SplashLoadingStep.ready`

**Files:**
- [main.dart:221-226](../app/lib/main.dart#L221-L226) (authenticated-branch fire-and-forget)
- [main.dart:433-465](../app/lib/main.dart#L433-L465) (`_initializeFCM()`)
- [main.dart:374-389](../app/lib/main.dart#L374-L389) (`_updatePresence()`)
- [main.dart:396-415](../app/lib/main.dart#L396-L415) (`_refreshCommunitiesOnResume()`)
- [main.dart:420-431](../app/lib/main.dart#L420-L431) (`_startEventPollers` / `_startEventStreams`)
- [main.dart:268-272](../app/lib/main.dart#L268-L272) (unread count deferred init)
- [event_router.dart](../app/lib/services/event_router.dart) (`EventRouter`)
- [community_event_poller.dart](../app/lib/services/community_event_poller.dart) (`CommunityEventPoller`)
- [community_event_stream.dart](../app/lib/services/community_event_stream.dart) (`CommunityEventStreamService`)

**Pattern:** Fire-and-forget — services initialize in background without blocking UI.

**Permission-prompt note.** Only the FCM notification prompt fires during cold start. The location-permission prompt was deliberately moved out of the startup path after #1174 / #1182; see [Location & GPS at Startup](#location--gps-at-startup) for why and where it prompts instead.

### Phase 5b: Location & GPS at Startup

**Purpose:** Make user position available for proximity-bias queries and on-screen map pins without forcing a permission prompt at cold-start or racing the FCM permission prompt.

**Key principles (established in #1174 / #1182 and refined in #1175):**

1. **GPS is never requested unsolicited at cold-start.** `DeviceLocationService.getCurrentPositionIfGranted()` checks the existing permission state and returns null if not granted, rather than showing the system dialog. The prompting variant, `requestAndGetCurrentPosition()`, is only called from explicit user-gesture sites:
   - `DiscoverScreen._centerOnUserLocation` (map center-on-me button)
   - `HomeScreen._onAddTap` (+ button in the bottom nav)
   - `LocationPickerModal._useCurrentLocation` ("use current location" in the address picker)
   Each gesture site routes through `requestLocationWithSettingsFallback()`, which handles the `deniedForever` state with an in-app "Open Settings" dialog.

2. **Location reads are keyed by intent.** `userLocationProvider` is a `FutureProvider.family<Position?, LocationIntent>` with two cache slots:
   - `LocationIntent.proximityBias` — prefers a recent (≤ 5 min) OS last-known fix; falls through to `Geolocator.getCurrentPosition(accuracy: medium)`. Used by anything that ranks/sorts/biases results without displaying the position to the user (Discover initial search, autocomplete proximity, creation suggestion bias).
   - `LocationIntent.precisePin` — always calls `Geolocator.getCurrentPosition(accuracy: high)`. Used by anything that renders the position on a map as a pin/dot or saves it as a precise address (Discover map, leaderboard map, location picker, blue-dot overlay).

3. **The Discover tab is eagerly built at cold-start.** `HomeScreen._buildBody` uses an `IndexedStack` (`home_screen.dart:454-477`), which mounts every tab child at construction time — not just the visible one. This is deliberate: issue #803 established that the initial zero-state Discover search must fire at startup so the tab is populated the first time the user taps over to it. `SearchNotifier` also calls `ref.keepAlive()` so its results survive tab switching.

4. **Startup location cost is bounded.** Because Discover is eagerly built:
   - `SearchNotifier.build()` reads `userLocationProvider(LocationIntent.proximityBias).future` at cold-start to seed the initial search's bias (`search_view_model.dart:357`).
   - `DiscoverScreen._buildMapView` watches `userLocationProvider(LocationIntent.precisePin)` for the map camera and user pin.
   Both are session-cached by the family provider; each cache slot does one GPS fetch per session unless explicitly invalidated.

5. **Cold-start safety net for slow GPS.** `SearchNotifier._performInitialSearch` wraps its location read in a 3s timeout (`search_view_model.dart:381-392`). If the `proximityBias` path is slow (both last-known is missing AND `getCurrentPosition(medium)` hangs), the search still fires with fallback coordinates rather than blocking. In normal operation this timeout does not fire — the last-known path returns sub-100ms, and the fresh-medium path lands well inside 3s.

6. **Invalidation.** Callers force a re-fetch by invalidating a specific intent slot. Both are invalidated when:
   - The user pulls to refresh the feed (`feed_screen.dart:_onRefresh`).
   - `requestLocationWithSettingsFallback` succeeds (after a user grants permission via a gesture).
   - Auth state transitions (login, logout, long backgrounding) in `auth_state.dart`.

**Files:**
- [device_location_service.dart](../app/lib/services/device_location_service.dart) — `LocationIntent` enum, staleness-gated last-known, accuracy selection
- [user_position_resolver.dart](../app/lib/services/user_position_resolver.dart) — GPS → primary-residence → null ladder, intent-aware
- [providers.dart (userLocationProvider)](../app/lib/services/providers.dart) — family provider + architectural note
- [location_permission_helper.dart](../app/lib/core/utils/location_permission_helper.dart) — gesture-path prompt helper with settings-fallback dialog
- [search_view_model.dart](../app/lib/presentation/viewmodels/search_view_model.dart) — proximity-bias reader with 3s safety-net timeout
- [discover_screen.dart](../app/lib/presentation/screens/discover/discover_screen.dart) — precise-pin reader for map camera + user location puck
- [home_screen.dart:454-477](../app/lib/presentation/screens/home/home_screen.dart#L454-L477) — `IndexedStack` eager tab mount

**Related issues:**
- #1174 — FCM/Geolocator permission collision (closed by #1182).
- #1175 — cold-start GPS timeout (closed by the intent split).
- #803 — Discover zero-state must populate on first view (reason for eager build).
- #444 — original GPS → primary residence → Austin fallback ladder.

### Phase 6: Splash Screen Management

**Purpose:** Adaptive splash duration with progressive loading feedback.

**When:** After all critical initialization tasks

**Key Actions:**
- Calculate elapsed time since app start
- Adaptive timing logic:
  - **IF elapsed < 1.5s:** Wait remaining time (minimum branding visibility)
  - **IF 1.5s ≤ elapsed ≤ 5s:** Proceed immediately (no artificial delay)
  - **IF elapsed > 5s:** Proceed immediately + log warning
- Update splash state: `SplashLoadingStep.ready`
- Stop Firebase trace: `startup_total_init`
- Trigger router redirect logic

**Files:**
- [main.dart:191-222](../app/lib/main.dart#L191-L222) (adaptive timing)
- [splash_screen.dart:54-100](../app/lib/presentation/screens/splash/splash_screen.dart#L54-L100) (progressive indicator)
- [splash_view_model.dart](../app/lib/presentation/viewmodels/splash_view_model.dart)

**Visual Elements:**

**Splash Screen:** Displays app logo with determinate progress indicator and status message

**Progress Mapping:**
```dart
SplashLoadingStep.initializing       → 20% progress
SplashLoadingStep.checkingAuth       → 40% progress
SplashLoadingStep.loadingCommunities → 60% progress
SplashLoadingStep.loadingFeed        → 80% progress
SplashLoadingStep.ready              → 100% progress (check icon)
SplashLoadingStep.error              → 0% progress (error icon)
```

**Status Messages:**
- "Initializing..."
- "Checking authentication..."
- "Loading communities..."
- "Loading feed..."
- "Ready"
- "Failed to load" (with error details and retry button)

**Benefits:**
- Users see real-time progress instead of spinning loader
- Adaptive timing prevents both too-fast (jarring) and too-slow (frustrating) experiences
- Error states provide actionable retry option

## Routing & Navigation

### Authentication-Based Routing

**GoRouter Redirect Logic:** [app_router.dart:62-221](../app/lib/core/router/app_router.dart#L62-L221)

**Decision Flow:**

```
Router evaluates redirect
    ↓
Ignore Firebase Auth callback URLs — mobile only (reCAPTCHA redirect during phone auth; skipped on web, where Firebase handles its own callback under /__/auth/*)
    ↓
Wait for auth loading state
    ├─ IF authState.isLoading AND !isGoingToAuthScreen:
    │   └─ Redirect to /login (avoid rendering Home without auth data)
    ↓
Unauthenticated path
    ├─ IF !isAuthenticated AND !isGoingToAuthScreen:
    │   ├─ Deferred deep link present (shortCode set)?
    │   │   └─ Consume and redirect to /invite?token=...
    │   ├─ First launch (no prior session)?
    │   │   └─ Redirect to /register
    │   └─ Otherwise redirect to /login?from=[intended_destination]
    ↓
Authenticated path
    └─ IF isAuthenticated AND (isGoingToLogin OR isGoingToRegister):
        └─ Redirect to /
    ↓
Return null (no redirect needed)
```

**Key Features:**
- Preserves intended destination via `from` query parameter
- Prefers the deferred deep-link invite flow over generic login when a share link triggered the install
- Sends genuine first-launch unauthenticated users to `/register` (they need an invite code) rather than `/login`
- Prevents circular redirects with `isGoingTo*` checks
- Redirects to `/login` while auth is loading to avoid rendering Home without auth data

**Onboarding redirects have been removed.** The `hasSeenOnboarding` → `/onboarding` redirect was disabled in #983 (2026-04-03), and the tutorial state it read has since been deleted. Nothing in the router touches onboarding.

### Onboarding Redirect Logic (removed)

> **Removed.** Onboarding was turned off in #983; the router stopped
> redirecting to `/onboarding` then, and the screen and its state were deleted
> once nothing referenced them. Git history has the four-page flow if it is
> ever revived.

### Community Selection Persistence (removed)

> **Removed in #1895.** There is no longer a persisted "selected community."
> The sidebar and its per-community filter were deleted; `communitiesProvider`
> now holds the user's full portfolio with no selected-id pin. The legacy
> SharedPreferences keys `'selected_community_id'` and `'enabled_community_ids'`
> are no longer written or read — `CommunitiesNotifier.setCommunities()`
> proactively *removes* them on each load so upgrading users get their stale
> single-select state cleared (a #2023 cleanup follow-up will drop the removal
> once it has shipped for a few weeks).

**Reference:** [community_providers.dart](../app/lib/services/providers/community_providers.dart) (`CommunitiesNotifier.setCommunities`, legacy-key removal)

## Observability Integration

**Firebase Performance Traces:**

The startup system uses multiple granular traces for bottleneck identification:

1. **`app_startup`** (End-to-End)
   - **Start:** `main()` before Firebase initialization
   - **Stop:** First frame rendered (`WidgetsBinding.instance.addPostFrameCallback`)
   - **Metric:** Total cold start time (user tap to first pixels)
   - **Reference:** [main.dart:94-96](../app/lib/main.dart#L94-L96)

2. **`startup_total_init`** (Initialization Phase)
   - **Start:** Beginning of `Future.microtask()` in initState
   - **Stop:** When `SplashLoadingStep.ready` is set
   - **Metric:** Auth check + community loading + splash timing
   - **Reference:** [main.dart:165-228](../app/lib/main.dart#L165-L228)

3. **`startup_auth_check`** (Authentication Phase)
   - **Start:** Before auth state loading
   - **Stop:** After auth state is determined
   - **Metric:** JWT reading + token parsing
   - **Reference:** [main.dart:170-180](../app/lib/main.dart#L170-L180)

4. **`startup_load_communities`** (Community Loading Phase)
   - **Start:** Before community repository call (authenticated users only)
   - **Stop:** After communities set in state (success or error)
   - **Metric:** Repository call + cache check + server fetch + state update
   - **Reference:** [main.dart:190-200](../app/lib/main.dart#L190-L200)

**Trace Hierarchy:**
```
app_startup (total cold start)
  └─ startup_total_init (initState to ready)
       ├─ startup_auth_check (auth)
       └─ startup_load_communities (communities)
```

**Performance Insights:**
- Traces isolate specific bottlenecks (auth vs. community loading vs. splash timing)
- Enables targeted optimization based on which phase is slowest
- Can identify cache effectiveness (fast `startup_load_communities` = cache hit)

**Structured Logging:**
- **Logger:** `logging` package with hierarchical loggers
- **Levels:** DEBUG, INFO, WARNING, SEVERE
- **Output:** Console (debug), Firebase Crashlytics (production)
- **Format:** JSON-structured logs with timestamps

**Key Log Points:**
- "User authenticated on app startup, initializing services and loading communities"
- "Starting community loading for authenticated user"
- "Communities loaded successfully: N communities"
- "Community selection initialized"
- "Auth state changed to authenticated, loading communities" (post-login)

**Reference:** [main.dart](../app/lib/main.dart), [environment.dart](../app/lib/core/config/environment.dart)

## Error Handling

**Community Loading Failure:**
- **State:** `CommunitiesState.errorMessage = "Failed to load communities..."`
- **UI Display:**
  - **HomeScreen:** Red error banner at top with retry button (`home_screen.dart`, wired to `CommunitiesNotifier.retryLoadCommunities`)
- **Recovery:**  Users can tap retry button to re-attempt loading
- **Behavior:** App remains functional - users can navigate tabs, access other features
- **Logging:** "Failed to load communities during initialization" (SEVERE level)
- **Reference:** [main.dart:306-313](../app/lib/main.dart#L306-L313)

**Authentication Token Invalid:**
- **Action:** Set `isAuthenticated = false`
- **Recovery:** Redirect to login screen
- **Logging:** "Failed to load auth state" or silent (token not found)

**Firebase Initialization Failure:**
- **Action:** Caught by error zone
- **Recovery:** Crash (Firebase is critical dependency)
- **Logging:** Crashlytics report with full stack trace

**App Initialization Timeout:**
- **Detection:** `elapsed > 5000ms` (5 seconds)
- **Action:** Proceed to home screen anyway
- **Logging:** WARNING: "App initialization took Xms (exceeds max 5000ms)"
- **Benefit:** Prevents indefinite splash screen hang

**Router Redirect Loop:**
- **Prevention:** `isGoingTo*` checks prevent circular redirects
- **Debugging:** `debugLogDiagnostics: true` in GoRouter config

**Community Refresh on Resume Failure:**
- **Action:** Log warning, silent failure
- **Recovery:** User continues with cached communities
- **Logging:** "Failed to refresh communities on resume" (WARNING level)
- **Rationale:** Background refresh failure shouldn't interrupt user

## Performance Optimization

**Parallel Loading:**
- FCM and presence initialize in background (fire-and-forget)
- Unread count initialization delayed 500ms (non-critical)

**Adaptive Splash Duration:**
- **Fast loads (< 1.5s):** Waits to ensure branding visibility
- **Optimal loads (1.5-5s):** Proceeds immediately without artificial delay
- **Slow loads (> 5s):** Proceeds immediately + logs warning for investigation
- **Benefit:** Balances branding, speed, and hang prevention

**Single-Path Community Load:**
- Post-login, community loading happens exclusively through the auth-state listener in `GearApp.build()`.
- The previous login-screen prefetch was removed after it caused a race condition — duplicate `setCommunities()` calls that could orphan the initial search. Comment in `login_screen.dart:73-75` documents the decision.
- The same `_loadCommunities()` helper is used by both cold-start and post-login paths, keeping behavior consistent.
- **Reference:** [main.dart:282-314](../app/lib/main.dart#L282-L314) and [main.dart:509-519](../app/lib/main.dart#L509-L519)

**Community Caching:**
- Stash in-memory cache with 30-minute TTL (default)
- Cache hits return instantly (no network delay)
- Cache key: `'community:user:list'`
- Cache hits make both the cold-start and post-login `_loadCommunities()` paths cheap

**Community Refresh on Resume:**
- Automatic background refresh when app returns to foreground
- Catches invitations/removals that occurred while app was inactive
- Fire-and-forget, non-blocking
- Silent failure (continues with cached data)
- **Reference:** [main.dart:334-358](../app/lib/main.dart#L334-L358)

**Progressive Loading Indicator:**
- Determinate progress (20%, 40%, 60%, 80%, 100%) vs. indeterminate spinner
- Gives users visibility into what's happening
- Reduces perceived wait time (psychological benefit)

**Granular Performance Traces:**
- 4 separate Firebase traces identify specific bottlenecks
- Enables targeted optimization (e.g., slow auth vs. slow communities)
- Can measure cache effectiveness (fast `startup_load_communities` = cache hit)

**Auth State Listener Efficiency:**
- `ref.listen` only fires on state changes (not rebuilds)
- Condition check prevents redundant community loads
- Only triggers on `false → true` authentication transition

**Lazy Service Initialization:**
- Non-critical services initialize after splash screen
- FCM registration happens in background
- Doesn't block critical path (auth, communities, routing)

## Testing

**Unit Tests:**

**Integration Tests:**
- Community loading flow (repository → service → cache)
- Authentication state transitions
- Router redirect logic

**Widget Tests:**
- [profile_settings_screen_test.dart](../app/test/presentation/screens/profile/profile_settings_screen_test.dart) - Settings screen behavior

**Manual Testing:**
- Cold start with authentication
- Cold start without authentication
- Login → community loading → home flow
- Community loading failure recovery
- Minimum splash duration verification

## Architecture Strengths

✅ **Strengths:**

1. **Predictable Initialization:** Sequential phases ensure dependencies load in correct order with clear progress feedback
2. **Async-First:** Non-blocking operations prevent UI freezing during startup with granular performance traces
3. **Comprehensive Error Recovery:** Failed operations set error state with a retry UI in the HomeScreen error banner; app remains functional
4. **Centralized State:** Riverpod providers with loading/error tracking create single source of truth for startup states
5. **Granular Observability:** 4 Firebase traces + structured logging pinpoint bottlenecks (auth, communities, total init, end-to-end)
6. **Adaptive Performance:** Fast loads wait for branding, optimal loads proceed immediately, slow loads avoid hanging
7. **Progressive User Feedback:** Determinate progress indicator (20%-100%) shows real-time status instead of spinner
8. **Router-Driven:** Declarative routing logic based on state, not imperative navigation calls
9. **Single-Path Post-Login Loading:** One `_loadCommunities()` helper serves both cold-start and post-login, avoiding the prefetch/listener race that used to orphan the initial search
10. **Auto-Refresh on Resume:** Background community refresh catches changes while app was inactive
12. **Resilient to Network Failures:** Community loading errors don't block app; users get actionable retry options
13. **Performance Monitoring:** Granular traces enable data-driven optimization decisions

## Architecture Weaknesses

⚠️ **Weaknesses:**

1. **Complex Initialization Sequence:** Multiple phases with dependencies can be hard to debug (mitigated by granular performance traces)
2. **No Offline Queue:** Failed initialization operations require manual retry via UI buttons
3. **Listener in Build Method:** `ref.listen` in build can be unintuitive for new developers
4. **Community Selection Persistence:** SharedPreferences single point of failure for last selection
5. **Firebase Dependency:** App cannot function without Firebase initialization

## Future Improvements

### Completed Enhancements

✅ **Recently Implemented:**

1. **Progressive Splash Screen:** Determinate progress indicator (20%, 40%, 60%, 80%, 100%) shows current loading step
2. **Single-Path Community Load:** One `_loadCommunities()` helper for cold-start and post-login (the old login-screen prefetch was removed to fix a race)
3. **Smart Splash Duration:** Adaptive timing (min 1.5s, optimal 1.5-5s, max 5s with warning)
4. **Error Recovery UI:** HomeScreen shows an error banner with a retry button for failed community loads
5. **Startup Performance Metrics:** 4 granular Firebase traces track auth check, community loading, total init, and end-to-end
6. **Community Prefetch on App Resume:** Automatic background refresh when app returns to foreground

### Recommended Enhancements

**1. Offline Queue:**
- Automatically retry failed community loads (currently requires manual retry button)
- Store pending operations for later execution
- Improve reliability on poor networks

**4. Community Cache Warmup on Login:**
- Prime cache during login API call (parallel to JWT validation)
- Further reduce post-login wait time
- Requires careful error handling

**5. Predictive Preloading:**
- Load feed data for most likely selected community during community load
- Reduce perceived latency when entering home screen
- Requires usage pattern analysis

## Appendix

### Key Components

#### Main Application Entry Point
**File:** [main.dart](../app/lib/main.dart)
- **Purpose:** App entry point, initialization orchestrator
- **Key Methods:**
  - `main()` - Firebase initialization and app launch
  - `_GearAppState.initState()` - Startup sequence orchestration
  - `_GearAppState.build()` - Auth state listener and UI rendering
  - `_loadCommunities()` - Community loading shared method

#### Authentication State Management
**File:** [auth_state.dart](../app/lib/services/auth_state.dart)
- **Purpose:** Manages user authentication state globally
- **Key Methods:**
  - `loadAuthState()` - Read JWT from SecureStorage
  - `saveAuthState()` - Save JWT after login
  - `logout()` - Clear auth state and cache

#### Community Repository
**File:** [community_repository.dart](../app/lib/data/repositories/community_repository.dart)
- **Purpose:** Fetch and cache community data
- **Key Methods:**
  - `listUserCommunities()` - Get user's communities with caching
  - `refreshUserCommunities()` - Force refresh from server

#### Selected Community State
**File:** [providers.dart:476-566](../app/lib/services/providers.dart#L476-L566)
- **Purpose:** Manage currently selected community and persistence
- **Key Methods:**
  - `setCommunities()` - Set communities list and restore selection
  - `selectCommunity()` - Change selected community and persist

#### Router Configuration
**File:** [app_router.dart](../app/lib/core/router/app_router.dart)
- **Purpose:** Configure routes and redirect logic
- **Key Features:**
  - Authentication-based redirects
  - Intent preservation via `from` parameter

#### Splash Screen
**File:** [splash_screen.dart](../app/lib/presentation/screens/splash/splash_screen.dart)
- **Purpose:** Display branded loading screen during initialization
- **Key Features:**
  - App logo and loading indicator
  - Controlled by `splashProvider` state

#### Environment Configuration
**File:** [environment.dart](../app/lib/core/config/environment.dart)
- **Purpose:** Configure app behavior per environment
- **Key Settings:**
  - `CACHE_TTL_MINUTES` - Community cache duration
  - `ENABLE_NOTIFICATIONS` - FCM initialization flag
  - Logging configuration

### Related Documentation
- [client_architecture.md](client_architecture.md) - Overall app architecture patterns
- [client_caching.md](client_caching.md) - Stash-based caching system
- [client_testing.md](client_testing.md) - Testing patterns and guidelines

---

**Last Updated:** 2026-05-13
**Architecture Version:** 2.4 (Riverpod 3.0 + Stash + deferred location prompt + `LocationIntent` family + onboarding removed + per-user notifier reset on login)
