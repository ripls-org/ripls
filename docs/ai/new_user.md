# New User Tutorial System

**Status**: Architecture audit complete ✅
**Compliance**: [docs/client/architecture.md](../client/architecture.md) - MVVM with Riverpod 3.0
**Key Pattern**: Repository → ViewModel → Screen (no direct repository access from UI)

---

## Executive Summary

This document outlines a four-phase tutorial system for Ripls:

1. **Phase 1**: Conceptual introduction (4 screens) - ✅ **COMPLETE**
2. **Phase 2**: Startup experience improvements (progressive splash, error recovery)
3. **Phase 3**: No-communities flow (community creation requirement)
4. **Phase 4**: Contextual feature discovery (7 tutorials) using `tutorial_coach_mark` plugin

**Phase 1 Status**: ✅ **COMPLETE** (January 2026)
- Custom PageView-based onboarding with full-screen images
- TutorialStateRepository with async-first SharedPreferences
- TutorialViewModel with Riverpod Notifier + Freezed state
- Router integration with authentication and onboarding checks
- Settings toggle for testing/replay
- 19/19 tests passing

**Architecture highlights:**
- `TutorialStateRepository` (data layer) - async-first SharedPreferences access
- `TutorialViewModel` (presentation layer) - Riverpod Notifier with Freezed state
- Screens use `ref.watch(tutorialProvider)` - no direct repository calls
- All methods async for consistency with architecture guidelines

---

## Problem Statement

Ripls is a novel community-building application with no direct comparables in the market. Users need to understand:
1. **The "Why"**: The conceptual value of community sharing, skill development, and mutual aid
2. **The "What"**: Specific features (gear sharing, events, help requests, skills)
3. **The "How"**: Interface mechanics and interaction patterns

Without guided onboarding, users may:
- Miss the app's core value proposition
- Fail to discover key features
- Abandon the app due to confusion
- Not understand the social/community aspects

## Current State

- No existing tutorial system
- Complex feature set across multiple domains (gear, events, requests, skills)
- Novel interaction patterns (e.g., camera-first gear posting)
- Reference design exists: `tmp/onboard/welcome-home.jsx` (web-based onboarding inspiration)

## Potential Solutions

### Phase 1: Conceptual Introduction (First Launch)

**Option A: introduction_screen plugin**
- **Pros**: Mature, widely used, simple API, supports images/videos
- **Cons**: Less customizable styling, basic animations
- **Best for**: Quick implementation, standard onboarding flows

**Option B: flutter_onboarding_slider plugin**
- **Pros**: Modern animations, swipeable cards, customizable
- **Cons**: Less mature, potentially more setup
- **Best for**: More engaging visual experience

**Option C: Custom implementation**
- **Pros**: Full control, matches app design system exactly
- **Cons**: More development time, maintenance burden
- **Best for**: Unique brand experience

### Phase 4: Interactive Feature Discovery (Contextual)

**Option A: tutorial_coach_mark plugin**
- **Pros**: Mature, comprehensive, supports complex flows, good animations
- **Cons**: Heavier dependency, learning curve
- **Best for**: Complex multi-step tutorials

**Option B: showcaseview plugin**
- **Pros**: Lightweight, simple API, good for highlighting individual elements
- **Cons**: Less suitable for complex flows
- **Best for**: Simple feature highlights

**Option C: Custom overlay system**
- **Pros**: Full control, lightweight, app-specific
- **Cons**: Development time, edge cases
- **Best for**: Minimal, highly integrated experience

## Architecture Compliance

This tutorial system follows the **MVVM architecture with Riverpod 3.0** as documented in [docs/client/architecture.md](../client/architecture.md).

### Key Architectural Rules Applied

1. **Repository Pattern for All Data Access**
   - `TutorialStateRepository` handles SharedPreferences access
   - All methods are async-first: `Future<bool> hasSeenTutorial(String key)`
   - No direct SharedPreferences access from ViewModels or screens

2. **ViewModels Handle Business Logic**
   - `TutorialViewModel` (Riverpod Notifier) manages tutorial trigger logic
   - Uses Freezed for immutable state: `TutorialState`
   - Screens never call repositories directly

3. **Screens Only Interact with ViewModels**
   - `ref.watch(tutorialProvider)` for reactive state
   - `ref.read(tutorialProvider.notifier).checkAndShowTutorial()` for actions
   - No business logic in screens

4. **No Controllers**
   - Using Riverpod Notifier pattern, not "TutorialController"
   - Helpers/utils only for pure functions (no state)

### Data Flow

```
Screen (onboarding_screen.dart)
  ↓ ref.watch(tutorialProvider)
TutorialViewModel (tutorial_view_model.dart)
  ↓ await repository.hasSeenTutorial()
TutorialStateRepository (tutorial_state_repository.dart)
  ↓ SharedPreferences
```

### Example Code Structure

**Repository (Data Layer):**
```dart
class TutorialStateRepository {
  final SharedPreferences _prefs;

  TutorialStateRepository(this._prefs);

  // Async-first: all methods return Future
  Future<bool> hasSeenOnboarding() async {
    return _prefs.getBool('tutorial_onboarding_seen') ?? false;
  }

  Future<void> markOnboardingComplete() async {
    await _prefs.setBool('tutorial_onboarding_seen', true);
  }

  Future<bool> hasSeenTutorial(String key) async {
    return _prefs.getBool('tutorial_$key') ?? false;
  }

  Future<void> markTutorialSeen(String key) async {
    await _prefs.setBool('tutorial_$key', true);
  }
}

// Provider for repository
final tutorialStateRepositoryProvider = Provider<TutorialStateRepository>((ref) {
  final prefs = ref.watch(sharedPreferencesProvider);
  return TutorialStateRepository(prefs);
});
```

**ViewModel (Presentation Layer):**
```dart
@freezed
class TutorialState with _$TutorialState {
  const factory TutorialState({
    @Default(false) bool hasSeenOnboarding,
    @Default({}) Map<String, bool> seenTutorials,
    String? activeTutorial,
  }) = _TutorialState;
}

class TutorialNotifier extends Notifier<TutorialState> {
  @override
  TutorialState build() => const TutorialState();

  Future<void> checkOnboardingStatus() async {
    final repository = ref.read(tutorialStateRepositoryProvider);
    final hasSeen = await repository.hasSeenOnboarding();
    state = state.copyWith(hasSeenOnboarding: hasSeen);
  }

  Future<void> completeOnboarding() async {
    final repository = ref.read(tutorialStateRepositoryProvider);
    await repository.markOnboardingComplete();
    state = state.copyWith(hasSeenOnboarding: true);
  }

  Future<void> checkAndShowTutorial(String key) async {
    final repository = ref.read(tutorialStateRepositoryProvider);
    final hasSeen = await repository.hasSeenTutorial(key);
    if (!hasSeen) {
      state = state.copyWith(activeTutorial: key);
    }
  }

  Future<void> markTutorialComplete(String key) async {
    final repository = ref.read(tutorialStateRepositoryProvider);
    await repository.markTutorialSeen(key);
    state = state.copyWith(
      seenTutorials: {...state.seenTutorials, key: true},
      activeTutorial: null,
    );
  }
}

final tutorialProvider = NotifierProvider<TutorialNotifier, TutorialState>(
  TutorialNotifier.new,
);
```

**Screen (Presentation Layer):**
```dart
class OnboardingScreen extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final tutorialState = ref.watch(tutorialProvider);

    return IntroductionScreen(
      pages: _buildPages(),
      onDone: () async {
        // Call ViewModel, not repository
        await ref.read(tutorialProvider.notifier).completeOnboarding();
        // Navigate to main app
      },
    );
  }
}
```

## Recommendations

### Phase 1: Use `introduction_screen` plugin

**Rationale:**
- Proven track record in production apps
- Simple integration with existing codebase
- Supports background images (requirement from welcome-home.jsx)
- Easy to maintain and update content
- Quick time-to-market

**Implementation approach:**
- 4 screens total (sweet spot between information and attention span)
- Background images for each screen (placeholders initially)
- Minimal text, focus on value proposition
- Skip button for returning users
- "Get Started" CTA on final screen

### Phase 2: Use `tutorial_coach_mark` plugin

**Rationale:**
- Better suited for complex, multi-step tutorials
- More mature than showcaseview for complex flows
- Supports sequential highlighting (critical for multi-step processes like gear posting)
- Good community support and documentation
- Handles edge cases (scrolling, overlays, navigation)

**Implementation approach:**
- Store tutorial completion state in shared preferences
- Trigger contextually on first visit to each major screen
- Keep each tutorial to 3-5 steps maximum
- Allow skip/dismiss at any time
- Never show the same tutorial twice

## Open Questions

### Technical Questions

1. **State Management**: Where should tutorial completion state be stored?
   - ✅ **Decision**: Use `TutorialStateRepository` with SharedPreferences
   - **Architecture compliance**: Repository pattern for all data access (async-first)
   - Optional future: Sync to user profile in database for cross-device consistency

2. **ViewModel vs Screen Logic**: Who triggers tutorials?
   - ✅ **Decision**: `TutorialViewModel` (Riverpod Notifier) handles all trigger logic
   - **Architecture compliance**: Business logic lives in ViewModels, not screens
   - Screens call `ref.read(tutorialProvider.notifier).checkAndShowTutorial('key')`

3. **User Preferences**: Should users be able to replay tutorials?
   - ✅ **Decision**: Yes, via Settings screen ("Help & Tutorials" section)
   - Call `tutorialViewModel.resetTutorial('key')` or `resetAllTutorials()`

4. **A/B Testing**: Do we need analytics to measure tutorial effectiveness?
   - Questions: What metrics matter (completion rate, feature adoption, retention)?
   - Recommendation: Start simple, add analytics if retention is an issue

### Content Questions

1. **Phase 1 Content**: The 4 screens will use exact text from `tmp/onboard/welcome-home.jsx`:
   - Screen 1: **"You belong here"** - "Neighbors who know your name. Friends who show up. A place where asking for help feels natural."
   - Screen 2: **"Generosity made easy"** - "That tent from last summer. The bread maker gathering dust. Sharing lightens your load and helps your neighbor."
   - Screen 3: **"Every neighbor is a teacher"** - "Bike repair. Sourdough. Three languages. Skills bloom when shared, not hoarded."
   - Screen 4: **"Start building community"** - "We'll guide you from your first shared meal to sustained mutual aid. Start with people you trust. Grow at your pace."

2. **Placeholder Images**: What style/theme for background images?
   - Should they show actual app UI or conceptual illustrations?
   - Recommendation: Conceptual illustrations (less likely to become outdated)

3. **Camera Tutorial**: How much detail for gear photo guidance?
   - Just "point and shoot" or composition tips?
   - Recommendation: Keep simple initially, can expand based on photo quality

4. **Skills Concept**: How to explain skills without overwhelming?
   - Is there a simple analogy or metaphor?
   - Recommendation: "Things you know how to do that others can learn from you"

### UX Questions

1. **Tutorial Interruption**: Can users dismiss and resume later?
   - Recommendation: Yes, always allow skip with option to restart from Settings

2. **Contextual Timing**: When exactly to trigger each tutorial?
   - On screen load? After N seconds? On specific interaction?
   - Recommendation: 500ms delay after screen renders (allows UI to settle)

3. **Tutorial Conflicts**: What if multiple tutorials could trigger simultaneously?
   - Recommendation: Queue system with priority (e.g., primary features first)

4. **Accessibility**: How do tutorials work with screen readers?
   - Need to verify both plugins support TalkBack/VoiceOver
   - Recommendation: Manual testing required, may need fallback text instructions

## Implementation Plan

### Phase 1: Conceptual Introduction ✅ **COMPLETE**

**Status**: Completed January 2026

**What Was Built:**
- Custom PageView-based onboarding screen (replaced `introduction_screen` plugin due to rendering issues)
- 4 full-screen pages with centered white text on semi-transparent black overlay (60% opacity)
- TutorialStateRepository with async-first SharedPreferences access
- TutorialViewModel with Riverpod Notifier + Freezed state
- Router integration checking `hasSeenOnboarding` and redirecting to `/onboarding`
- Settings screen toggle for "Onboarding Completed" (testing/replay)
- Auth state listener in main.dart to load communities after login
- Comprehensive logging throughout community loading flow
- 19/19 tests passing (tutorial + repository + profile settings)

**Key Files:**
- [onboarding_screen.dart](../app/lib/presentation/screens/onboarding/onboarding_screen.dart) - Custom PageView implementation
- [tutorial_view_model.dart](../app/lib/presentation/viewmodels/tutorial_view_model.dart) - Riverpod Notifier with Freezed
- [tutorial_state_repository.dart](../app/lib/data/repositories/tutorial_state_repository.dart) - SharedPreferences persistence
- [app_router.dart](../app/lib/core/router/app_router.dart) - Onboarding redirect logic
- [main.dart](../app/lib/main.dart) - Community loading + auth state listener
- [startup.md](../client/startup.md) - Complete startup flow documentation

**Bugs Fixed:**
- ✅ Communities not loading after login (added auth state listener)
- ✅ Profile settings tests failing (updated for 4th switch)

**Documentation:**
- ✅ Created [docs/client/startup.md](../client/startup.md) - Comprehensive startup flow documentation

---

### Phase 2: Startup Experience Improvements (Sprint 3-4)

**Goal**: Improve perceived performance and error recovery during app initialization based on recommendations from [docs/client/startup.md](../client/startup.md#recommended-enhancements).

#### Sprint 3: Progressive Splash Screen & Error Recovery

**1. Progressive Loading Indicator**
- **File**: [splash_screen.dart](../app/lib/presentation/screens/splash/splash_screen.dart)
- **Changes**:
  - Add step indicator showing current loading phase
  - Display text: "Checking authentication...", "Loading communities...", "Preparing home..."
  - Use `SplashLoadingStep` enum values to determine text
  - Add progress indicator (determinate if possible, indeterminate otherwise)
- **UX Benefit**: Users understand what's happening during long loads
- **Architecture**: Watch `splashProvider` for step changes, update UI reactively

**2. Community Loading Error Recovery**
- **File**: [home_screen.dart](../app/lib/presentation/screens/home/home_screen.dart)
- **Changes**:
  - Check `selectedCommunityProvider.communities.isEmpty` in build
  - Show centered error state: "Failed to load communities"
  - Add "Retry" button calling `CommunityRepository.refreshUserCommunities()`
  - Show loading indicator during retry
- **UX Benefit**: Users can recover from failed community loads without restarting app
- **Architecture**: Screen watches provider state, calls repository on retry button tap

**3. Sidebar Error State**
- **File**: [sidebar.dart](../app/lib/presentation/widgets/sidebar.dart)
- **Changes**:
  - Replace "No communities to show" with actionable message
  - Add "Retry Loading Communities" button
  - Call `CommunityRepository.refreshUserCommunities()` on tap
  - Show loading spinner during retry
- **UX Benefit**: Immediate retry from navigation drawer
- **Architecture**: Widget calls repository method via provider

**4. Smart Splash Duration**
- **File**: [main.dart](../app/lib/main.dart)
- **Changes**:
  - Calculate total load time from app start
  - If load completes in < 500ms, skip minimum splash duration
  - If load takes 500ms-1.5s, enforce 1.5s minimum (current behavior)
  - If load takes > 1.5s, show immediately when ready
- **UX Benefit**: Very fast loads feel instant, slow loads don't add artificial delay
- **Architecture**: Adjust minimum splash calculation logic

#### Sprint 4: Community Pre-Loading & Performance

**5. Community Prefetch During Login**
- **File**: [login_screen.dart](../app/lib/presentation/screens/auth/login_screen.dart), [auth_state.dart](../app/lib/services/auth_state.dart)
- **Changes**:
  - After `AuthService.login()` succeeds but before navigation
  - Call `CommunityRepository.listUserCommunities()` immediately
  - Store result in cache (repository handles this automatically)
  - When main.dart auth listener fires, cache hit returns instantly
- **UX Benefit**: Eliminates post-login community loading delay
- **Architecture**: Prefetch during login flow, rely on Stash cache for instant retrieval

**6. Startup Performance Metrics**
- **File**: [main.dart](../app/lib/main.dart), [environment.dart](../app/lib/core/config/environment.dart)
- **Changes**:
  - Add separate traces for each initialization phase:
    - `auth_check_duration`
    - `tutorial_state_load_duration`
    - `community_load_duration`
    - `splash_minimum_wait_duration`
  - Log durations to Firebase Performance
  - Add metrics dashboard in Firebase Console
- **UX Benefit**: Identify bottlenecks in production, optimize slowest phases
- **Architecture**: Wrap each phase in performance trace

**7. Community Refresh on App Resume**
- **File**: [main.dart](../app/lib/main.dart) in `didChangeAppLifecycleState`
- **Changes**:
  - When app resumes from background (`AppLifecycleState.resumed`)
  - Call `CommunityRepository.refreshUserCommunities()` if user authenticated
  - Update `selectedCommunityProvider` with fresh data
  - Don't block UI (fire-and-forget background refresh)
- **UX Benefit**: Stale data refreshes automatically when returning to app
- **Architecture**: Lifecycle observer triggers repository refresh

**Testing**:
- Manual testing: Airplane mode → launch app → verify error states
- Manual testing: Slow network → verify progressive splash messages
- Manual testing: Fast network → verify smart splash duration
- Unit tests: Error state rendering in HomeScreen and Sidebar
- Integration tests: Prefetch during login → cache hit in main.dart

---

### Phase 3: No-Communities Flow (Sprint 5-6)

**Goal**: Handle users who have zero communities by requiring initial community creation instead of showing empty home screen.

**Problem**: Currently, if a user somehow has zero communities (new account, removed from all communities, etc.), they see the home screen with an empty state. This creates a dead-end where users don't know how to proceed.

**Solution**: Detect zero-communities state and route to a mandatory community creation flow.

#### Sprint 5: Zero-Communities Detection & Routing

**1. Add CommunityCreationRequiredScreen**
- **New File**: `app/lib/presentation/screens/community/community_creation_required_screen.dart`
- **Content**:
  - Full-screen centered UI
  - Hero image (community illustration)
  - Title: "Let's Start Your First Community"
  - Body text: "Communities are at the heart of Ripls. Create your first community to start sharing with neighbors, organizing events, and building connections."
  - Primary button: "Create Community" → navigates to community creation flow
  - Secondary text link: "Join an Existing Community" → navigates to community join/invite flow
- **Architecture**: ConsumerStatefulWidget, no business logic (just navigation)

**2. Update Router Redirect Logic**
- **File**: [app_router.dart](../app/lib/core/router/app_router.dart)
- **Changes**:
  - After onboarding redirect check, add community count check:
    ```dart
    // After authenticated + hasSeenOnboarding checks
    final communityState = ref.read(selectedCommunityProvider);

    if (isAuthenticated &&
        hasSeenOnboarding &&
        communityState.communities.isEmpty &&
        !isGoingToCommunityCreationRequired) {
      _log.info('User has zero communities, redirecting to community creation');
      return '/community-creation-required';
    }
    ```
  - Prevent access to home, discover, feed, inbox if communities.isEmpty
  - Allow access to profile, settings (user can log out)
- **Architecture**: Router logic based on provider state

**3. Add Route Definition**
- **File**: [app_router.dart](../app/lib/core/router/app_router.dart)
- **Changes**:
  - Add route:
    ```dart
    GoRoute(
      path: '/community-creation-required',
      builder: (context, state) => const CommunityCreationRequiredScreen(),
    ),
    ```

**4. Handle Community Creation Success**
- **File**: [community_creation_screen.dart](../app/lib/presentation/screens/community/community_creation_screen.dart) (or similar)
- **Changes**:
  - After successful community creation
  - Call `CommunityRepository.refreshUserCommunities()`
  - Update `SelectedCommunityNotifier.setCommunities()` with fresh list
  - Auto-select the newly created community
  - Navigate to home: `context.go('/')`
  - Router will now pass (communities.isNotEmpty)
- **Architecture**: Screen calls repository refresh, router re-evaluates automatically

#### Sprint 6: Community Join Flow & Edge Cases

**5. Add Community Join/Invite Flow**
- **New Files**:
  - `app/lib/presentation/screens/community/community_join_screen.dart`
  - `app/lib/presentation/screens/community/community_invite_accept_screen.dart`
- **Features**:
  - Search for communities by name/location
  - Browse nearby communities (if location permission granted)
  - Enter invite code to join private community
  - Accept invite from deep link/notification
- **Architecture**: Screens use `CommunityViewModel`, call `CommunityRepository` methods

**6. Handle "Join an Existing Community" Flow**
- **File**: [community_creation_required_screen.dart](../app/lib/presentation/screens/community/community_creation_required_screen.dart)
- **Changes**:
  - "Join an Existing Community" link navigates to `/community-join`
  - After successful join, refresh communities
  - Router redirects to home automatically

**7. Edge Case: Last Community Left/Removed**
- **Scenario**: User is in home screen, gets removed from all communities
- **Solution**:
  - Add listener in main.dart or home_screen for `selectedCommunityProvider.communities`
  - If list changes from non-empty → empty while app running
  - Show dialog: "You've left all communities"
  - Navigate to `/community-creation-required`
- **Architecture**: ref.listen on communities list, navigate on empty transition

**8. Prevent Community-Required Screens Without Communities**
- **Files**: [home_screen.dart](../app/lib/presentation/screens/home/home_screen.dart), [discover_screen.dart](../app/lib/presentation/screens/discover/discover_screen.dart), etc.
- **Changes**:
  - Add defensive check in build():
    ```dart
    if (communityState.communities.isEmpty) {
      // Router should have redirected, but defensive check
      return Center(child: Text('No communities available'));
    }
    ```
  - Prevents crashes if router logic fails
- **Architecture**: Defensive programming in screens

**Testing**:
- Unit tests: Router redirect logic with zero communities
- Widget tests: CommunityCreationRequiredScreen rendering
- Integration tests: Create community → refresh → router allows home access
- Integration tests: Join community → refresh → router allows home access
- Manual testing: Fresh account → verify forced to create/join community
- Manual testing: Leave all communities → verify redirect to creation screen

---

### Phase 4: Interactive Feature Discovery (Sprint 7-11)

**Previous**: Phase 2
**Renamed**: Phase 4 to accommodate new Phase 2 (startup improvements) and Phase 3 (no-communities flow)

### Phase 4: Conceptual Introduction (Sprint 1-2)

#### Sprint 1: Foundation
1. **Add dependencies**
   - `introduction_screen: ^3.1.12` to `app/pubspec.yaml`
   - Run `flutter pub get`

2. **Create onboarding screen**
   - New file: `app/lib/presentation/screens/onboarding/onboarding_screen.dart`
   - Create 4 PageViewModel instances with exact text from `tmp/onboard/welcome-home.jsx`:
     ```dart
     PageViewModel(
       title: "You belong here",
       body: "Neighbors who know your name. Friends who show up. A place where asking for help feels natural.",
       image: Image.asset('assets/images/onboarding/screen1.png'),
     ),
     PageViewModel(
       title: "Generosity made easy",
       body: "That tent from last summer. The bread maker gathering dust. Sharing lightens your load and helps your neighbor.",
       image: Image.asset('assets/images/onboarding/screen2.png'),
     ),
     PageViewModel(
       title: "Every neighbor is a teacher",
       body: "Bike repair. Sourdough. Three languages. Skills bloom when shared, not hoarded.",
       image: Image.asset('assets/images/onboarding/screen3.png'),
     ),
     PageViewModel(
       title: "Start building community",
       body: "We'll guide you from your first shared meal to sustained mutual aid. Start with people you trust. Grow at your pace.",
       image: Image.asset('assets/images/onboarding/screen4.png'),
       footer: ElevatedButton(
         onPressed: _onOnboardingComplete,
         child: Text("Get Started"),
       ),
     ),
     ```
   - **IMPORTANT - Visual Design Pattern**:
     - Images should be full-screen backgrounds (like `gear_content_view.dart`)
     - Use `ContentGradientOverlay` widget (from `content_gradient_overlay.dart`) for semi-transparent black overlay
     - Text should be white and overlaid on the gradient using theme styles:
       - Title: `Theme.of(context).textTheme.headlineSmall` or `headlineMedium` with `color: Colors.white`
       - Body: `Theme.of(context).textTheme.bodyLarge` with `color: Colors.white`
     - See [gear_content_view.dart](../app/lib/presentation/screens/gear/gear_content_view.dart:484-535) for reference implementation
     - Match fonts from [app_theme.dart](../app/lib/core/theme/app_theme.dart) (Libre Baskerville for headings, SF Pro Text for body)
     - Match colors from [app_colors.dart](../app/lib/core/theme/app_colors.dart)
   - Add "Get Started" button on final page styled with `AppColors.primary(context)`

3. **State management**
   - Create `app/lib/data/repositories/tutorial_state_repository.dart`
   - Store `has_seen_onboarding` boolean in SharedPreferences
   - Repository methods: `hasSeenOnboarding()`, `markOnboardingComplete()`
   - **Architecture compliance**: Use repository pattern, not utils/helpers for state storage

4. **Navigation logic**
   - Update `app/lib/main.dart` or routing logic
   - Show onboarding on first launch, then main app
   - Add skip button for quick access

5. **Placeholder assets**
   - Create `app/assets/images/onboarding/` directory
   - Add 4 placeholder images (screen1.png through screen4.png)
   - Update `pubspec.yaml` to include new assets

#### Sprint 2: Content & Polish
1. **Content writing**
   - Finalize text for all 4 screens
   - Keep each under 2 sentences
   - Focus on benefits, not features

2. **Visual design**
   - Work with design team on background images
   - Ensure images work with overlay text (contrast, positioning)
   - Test on multiple screen sizes

3. **Settings integration**
   - Add "Replay Tutorial" option in Settings
   - Should reset tutorial state and show onboarding again

4. **Testing**
   - Unit tests for TutorialPreferences
   - Widget tests for OnboardingScreen
   - Manual testing on iOS and Android
   - Test skip/complete flows

### Phase 2: Interactive Feature Discovery (Sprint 3-5)

#### Sprint 3: Infrastructure
1. **Add dependencies**
   - `tutorial_coach_mark: ^1.2.11` to `app/pubspec.yaml`
   - Run `flutter pub get`

2. **Tutorial state repository**
   - New file: `app/lib/data/repositories/tutorial_state_repository.dart`
   - Track completion per feature: `gear_post`, `event_create`, `help_request`, `feed_usage`, `discover_search`, `inbox_intro`, `skills_intro`
   - Methods: `hasSeenTutorial(String key)`, `markTutorialSeen(String key)`, `resetAllTutorials()`
   - Use SharedPreferences for storage (async-first, all methods return Future)
   - **Architecture compliance**: Repository pattern for all data access

3. **Tutorial ViewModel with Riverpod**
   - New file: `app/lib/presentation/viewmodels/tutorial_view_model.dart`
   - Use Riverpod Notifier pattern for tutorial UI state
   - Freezed state class: `TutorialState` with fields for active tutorial, queue, etc.
   - Business logic: trigger logic, queueing, theming
   - **Architecture compliance**: ViewModels call repositories, not services or utils
   - Provider: `tutorialProvider = NotifierProvider<TutorialNotifier, TutorialState>`

4. **Code generation**
   - Run `flutter pub run build_runner build` to generate Freezed code
   - Creates `tutorial_view_model.freezed.dart` with `copyWith()` and other methods
   - **Required before running/testing**

5. **Testing infrastructure**
   - Unit tests for TutorialStateRepository
   - ViewModel tests using ProviderContainer
   - Mock repository for testing tutorial triggers
   - **Architecture compliance**: Test ViewModels with fake repository implementations

#### Sprint 4: Core Feature Tutorials
1. **Gear posting tutorial**
   - Integrate into gear creation flow
   - Steps:
     1. Highlight camera button ("Start by taking a photo")
     2. Explain camera positioning ("Point at your gear, fill the frame")
     3. Highlight title field ("Give it a clear name")
     4. Highlight post button ("Share with your community")
   - Trigger: First visit to gear creation screen

2. **Feed usage tutorial**
   - Integrate into main feed screen
   - Steps:
     1. Highlight card ("Each card is something shared by neighbors")
     2. Swipe gesture indicator ("Swipe up and down to browse")
     3. Card interactions ("Tap to see details, heart to save")
   - Trigger: First visit to feed screen

3. **Discover search tutorial**
   - Integrate into discover screen
   - Steps:
     1. Highlight search bar ("Find specific gear or topics")
     2. Category filters ("Browse by type")
     3. Map view toggle ("See what's nearby")
   - Trigger: First visit to discover screen

#### Sprint 5: Secondary Feature Tutorials
1. **Event creation tutorial**
   - Similar structure to gear posting
   - Steps:
     1. Event type selection
     2. Date/time picker
     3. Invite friends
     4. Post event
   - Trigger: First visit to event creation screen

2. **Help request tutorial**
   - Steps:
     1. Describe what you need
     2. Add details (time, urgency)
     3. Send to community
   - Trigger: First tap on help request button

3. **Inbox tutorial**
   - Steps:
     1. Message organization (requests, offers, chats)
     2. Responding to messages
     3. Archiving/managing
   - Trigger: First visit to inbox with messages

4. **Skills introduction**
   - Steps:
     1. What skills are ("Things you can teach others")
     2. Adding your skills
     3. Browsing others' skills
     4. Requesting to learn
   - Trigger: First visit to skills section

#### Sprint 5 (continued): Testing & Refinement
1. **Integration testing**
   - Test each tutorial in context
   - Verify tutorials don't re-trigger
   - Test skip/dismiss behavior
   - Test tutorial queueing

2. **User testing**
   - Watch real users go through tutorials
   - Identify confusion points
   - Measure completion rates

3. **Settings integration**
   - Add "Tutorial Guide" section in Settings
   - List all available tutorials
   - Allow manual replay of any tutorial
   - "Reset all tutorials" option

4. **Analytics (if needed)**
   - Track tutorial starts, completions, skips
   - Measure feature adoption before/after tutorials
   - A/B test tutorial variations

### Phase 3: Iteration & Optimization (Ongoing)

1. **Monitor metrics**
   - Tutorial completion rates
   - Feature adoption rates
   - User retention
   - Support tickets related to feature confusion

2. **Content updates**
   - Refine tutorial copy based on user feedback
   - Update tutorials when features change
   - Add new tutorials for new features

3. **Technical improvements**
   - Optimize tutorial rendering performance
   - Add animations/transitions
   - Improve accessibility support

## File Structure

```
app/lib/
├── presentation/
│   ├── screens/
│   │   └── onboarding/
│   │       ├── onboarding_screen.dart          # Phase 1 introduction screen
│   │       └── widgets/
│   │           └── onboarding_page.dart        # Reusable page component
│   └── viewmodels/
│       └── tutorial_view_model.dart            # Tutorial business logic & state
├── data/
│   └── repositories/
│       └── tutorial_state_repository.dart      # Tutorial completion tracking (SharedPreferences)
└── core/
    └── utils/
        └── tutorial_helpers.dart               # Pure utility functions (if needed)

app/assets/images/onboarding/
├── screen1.png                                 # Placeholder backgrounds
├── screen2.png
├── screen3.png
└── screen4.png

app/test/
├── data/repositories/
│   └── tutorial_state_repository_test.dart     # Repository unit tests
├── presentation/
│   ├── viewmodels/
│   │   └── tutorial_view_model_test.dart       # ViewModel tests with ProviderContainer
│   └── screens/onboarding/
│       └── onboarding_screen_test.dart         # Widget tests

docs/ai/
└── new_user.md                                 # This document
```

**Architecture Compliance Notes:**
- **Repository Pattern**: All state persistence goes through `TutorialStateRepository`
- **ViewModel Layer**: `TutorialViewModel` manages business logic and calls repository
- **Async-First**: All repository methods return `Future<T>` for consistency
- **No Controllers**: Using Riverpod Notifier pattern, not controllers
- **Utils Only for Pure Functions**: No stateful logic in utils/helpers
- **Screens Access ViewModels**: Screens use `ref.watch(tutorialProvider)`, never repositories directly

## Success Metrics

### Phase 1 (Onboarding)
- **Primary**: 80%+ completion rate (users who see intro complete all screens)
- **Secondary**: <10% skip rate on first screen
- **Tertiary**: User proceeds to create account/profile after onboarding

### Phase 2 (Feature Discovery)
- **Primary**: Feature adoption increases by 30%+ after tutorial introduction
- **Secondary**: 70%+ tutorial completion rate (users don't skip)
- **Tertiary**: Reduced support tickets for "how do I..." questions
- **Quaternary**: Improved retention at Day 7 and Day 30

## Dependencies

### Flutter Packages
- `introduction_screen: ^3.1.12` (Phase 1 - onboarding UI)
- `tutorial_coach_mark: ^1.2.11` (Phase 2 - contextual tutorials)
- `shared_preferences: ^2.2.2` (already in project, verify version)
- `freezed: ^2.4.6` (already in project - for TutorialState)
- `freezed_annotation: ^2.4.1` (already in project)
- `riverpod: ^3.0.0` (already in project - for state management)
- `flutter_riverpod: ^3.0.0` (already in project)

### Design Assets
- 4 onboarding background images (placeholder → final)
- Icon/illustration assets for tutorial highlights (optional)

### Team Dependencies
- **Design**: Background images, visual style guide for tutorials
- **Content**: Copy for all tutorial screens and steps
- **QA**: User testing, accessibility testing
- **Analytics** (optional): Event tracking setup

## Risk Mitigation

### Technical Risks
1. **Plugin compatibility issues**
   - Mitigation: Test on multiple devices/OS versions early
   - Fallback: Ready to switch plugins or build custom if critical bugs

2. **Performance impact**
   - Mitigation: Lazy-load tutorial system, don't impact app startup time
   - Monitoring: Track app launch time before/after implementation

3. **State synchronization**
   - Mitigation: Keep tutorial state local (SharedPreferences) initially
   - Future: Sync to server for cross-device consistency if needed

### UX Risks
1. **Tutorial fatigue**
   - Mitigation: Keep each tutorial under 5 steps, always allow skip
   - Testing: Monitor skip rates, adjust content/timing

2. **Information overload**
   - Mitigation: Spread tutorials across first-use contexts, don't show all at once
   - Testing: User testing to validate pacing

3. **Tutorial becoming outdated**
   - Mitigation: Version tutorial content, update with feature changes
   - Process: Include tutorial review in feature change checklist

## Future Enhancements

### Post-Launch Improvements
1. **Progressive disclosure**: Show advanced features only after basic usage
2. **Personalization**: Different tutorial paths based on user interests
3. **Video tutorials**: Short clips for complex interactions
4. **Gamification**: Badges/rewards for completing tutorials or using features
5. **In-app help**: Searchable help center with tutorial replay
6. **Community tips**: User-generated tutorial content for advanced workflows

### Technical Evolution
1. **Server-side tutorial configuration**: Update tutorials without app release
2. **A/B testing framework**: Test tutorial variations systematically
3. **Advanced analytics**: Funnel analysis, cohort comparison
4. **Accessibility improvements**: Audio descriptions, haptic feedback
5. **Localization**: Translate tutorials for international users

## References

- Plugin: [introduction_screen](https://pub.dev/packages/introduction_screen)
- Plugin: [tutorial_coach_mark](https://pub.dev/packages/tutorial_coach_mark)
- Design reference: `/tmp/onboard/welcome-home.jsx`
- Flutter docs: [Onboarding best practices](https://docs.flutter.dev/cookbook/design/package-fonts)
- UX patterns: [Material Design onboarding](https://m3.material.io/foundations/content-design/onboarding-and-offboarding)
