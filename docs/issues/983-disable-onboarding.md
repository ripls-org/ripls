# Issue #983: Turn off the onboarding screens

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/983
**Filed:** 2026-04-01 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

Users sometimes see 3-4 onboarding screens when they log in. Since the app currently operates on an invite-only model (no open registration), every new user enters an existing community that's already populated with items and people. That lived experience *is* the onboarding, making the explicit onboarding wizard redundant and potentially confusing.

The onboarding flow should be disabled now and redesigned later for a future cold-start registration scenario where users create their first community.

## Context from Comments

The issue author (tescobar, maintainer) clarified the reasoning in a follow-up comment:

- **Why disable now:** All users are invited into pre-populated communities. The wizard adds no value in this context.
- **Future vision:** Onboarding should only appear for cold-start registrations (no existing community). It should culminate in requiring the user to create their first community before proceeding into the main app.
- **One-time guarantee:** After the first entrance, the user should never see onboarding screens again.

These comments supersede the original terse description and define both the immediate task (disable) and the future direction (redesign).

## Potential Solutions

### Option A: Remove the router redirect

Comment out or remove the onboarding redirect block in [app_router.dart:121-128](app/lib/core/router/app_router.dart#L121-L128). This is the single decision point that sends authenticated users to `/onboarding`. Keep all other onboarding code intact for future reuse.

- **Approach:** Delete or comment out the `if (isAuthenticated && !hasSeenOnboarding && !isGoingToOnboarding)` redirect block. Also remove the reverse guard at lines 131-133 that redirects away from `/onboarding` if already completed (no longer needed if nobody is routed there). Remove the onboarding route definition itself, or leave it but unreachable.
- **Pros:** Minimal change, single file edit. All onboarding UI code remains for future redesign. Easy to re-enable.
- **Cons:** Leaves dead code in the codebase (onboarding screen, viewmodel, repository, assets, settings toggle). The settings toggle for "Onboarding Completed" becomes meaningless.
- **Effort:** Small.

### Option B: Remove the router redirect + clean up settings toggle

Same as Option A, but also remove the "Onboarding Completed" toggle from the profile settings screen and the tutorial provider/repository wiring that supports it.

- **Approach:** Remove the router redirect, remove the tutorials section from profile settings, and remove the `TutorialStateRepository`, `TutorialNotifier`, and `tutorialProvider`. Keep the `OnboardingScreen` widget and assets for future reuse.
- **Pros:** No dead/confusing UI. Cleaner state management (removes an unused provider from the router's refresh listenable).
- **Cons:** More files touched. When onboarding is redesigned, the tutorial state infrastructure will need to be rebuilt (though likely with different requirements).
- **Effort:** Medium.

### Option C: Force-complete onboarding for all users

Instead of removing the redirect, auto-mark onboarding as complete immediately after authentication. The redirect logic stays but never triggers because `hasSeenOnboarding` is always `true`.

- **Approach:** In the auth state notifier, after successful login, call `completeOnboarding()` on the tutorial notifier.
- **Pros:** No routing changes needed. Onboarding infrastructure stays intact.
- **Cons:** Adds complexity (auth depends on tutorial state). Feels like a workaround rather than a fix. The onboarding screens still exist as a reachable route.
- **Effort:** Small, but hacky.

## Recommendation

**Option A: Remove the router redirect only.** Keep all tutorial infrastructure (`TutorialNotifier`, `TutorialStateRepository`, `tutorialProvider`, `OnboardingScreen`) intact for future use in open registration. Only the routing decision that sends logged-in users to onboarding is removed.

## Open Questions

- **Should the settings toggle be removed now?** — **Decision: Yes.** Remove the tutorials section from profile settings.
- **Should the onboarding route definition be removed from the router?** — **Decision: Yes.** Remove the route definition to prevent direct navigation to `/onboarding`.
- **Should the `RouterRefreshNotifier` stop listening to `tutorialProvider`?** — **Decision: Yes.** Remove it from the refresh listenable to avoid unnecessary router rebuilds.

## Implementation Plan

### Phase 1: Disable onboarding redirect
**Goal:** Users never see onboarding screens on login. Tutorial infrastructure is preserved for future open registration use.
**Tasks:**
- [ ] Remove the onboarding redirect block in [app_router.dart:121-128](app/lib/core/router/app_router.dart#L121-L128)
- [ ] Remove the reverse onboarding guard at [app_router.dart:130-133](app/lib/core/router/app_router.dart#L130-L133)
- [ ] Remove the `/onboarding` route definition from the router's route list
- [ ] Remove `tutorialProvider` from `RouterRefreshNotifier` listener in [app_router.dart:514](app/lib/core/router/app_router.dart#L514)
- [ ] Remove the `isGoingToOnboarding`, `hasSeenOnboarding`, `isTutorialLoading` variables and reads (lines 87-89, 97)
- [ ] Remove the tutorials section (`_buildTutorialsSection`) from [profile_settings_screen.dart](app/lib/presentation/screens/profile/profile_settings_screen.dart)
- [ ] Verify the app builds and existing tests pass
- [ ] Add or update tests to confirm onboarding redirect no longer triggers
**Validates:** Authenticated users go straight to home. `TutorialNotifier`, `TutorialStateRepository`, `OnboardingScreen`, and assets remain untouched.

### Phase 2: Future — Redesign onboarding for cold-start registration
**Goal:** New users who register without an invitation see an onboarding flow that ends with community creation.
**Tasks:**
- [ ] Design the new onboarding flow (community creation as final step)
- [ ] Gate onboarding on "user has no communities" rather than a SharedPreferences flag
- [ ] Ensure onboarding is truly one-time (never shown again after first community is created)
- [ ] Remove or repurpose the settings toggle
**Validates:** Cold-start users complete onboarding and enter a community they created.

## Architecture Audit

**Status:** Compliant — no architectural violations found.

**Findings:**
1. **Router is the correct place to disable.** The redirect logic in `app_router.dart` is the single decision point. Removing it there follows the existing routing architecture pattern and avoids scattering the change across multiple layers.
2. **`tutorialProvider` usage is limited.** It's referenced in: `app_router.dart` (redirect + refresh listener), `onboarding_screen.dart` (completion), `profile_settings_screen.dart` (settings toggle), and its own definition. No other viewmodels or screens depend on it, confirming the cleanup scope in the plan is accurate.
3. **No server-side changes needed.** Onboarding state is entirely client-side (SharedPreferences). No API or proto changes required.
4. **No caching impact.** The tutorial state uses SharedPreferences directly via `TutorialStateRepository`, not the Stash-based `CacheManager`. No cache invalidation concerns.
5. **i18n note:** The onboarding localization keys (`onboardingSkip`, `onboardingNext`, `onboardingDone`, `settingsOnboardingCompleted`, `settingsOnboardingCompletedSubtitle`) in `app_en.arb` and `app_es.arb` can be left in place for Phase 1 — unused ARB keys are harmless and don't affect bundle size. Clean them up when onboarding is redesigned or removed permanently.
6. **Phase 2 alignment:** The future onboarding redesign (gated on "user has no communities") aligns with the architectural principle that routing decisions should be based on server-provided state (community membership) rather than client-local flags (SharedPreferences). This is a good direction.

## Related Issues
- #800 — "Wizard too often" — likely related to users seeing onboarding unexpectedly; may be resolved by this change
- #979 — "Community creation" — the future onboarding redesign (Phase 2) depends on community creation functionality
