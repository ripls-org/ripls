# Issue #1056: [Feature Request] swipe among item tabs

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1056
**Filed:** 2026-04-07 | **Last activity:** 2026-04-17
**Labels:** enhancement, user-feedback | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

Tab swiping between item detail tabs (Type/Details/Chat) was implemented in PR #1102 but has two UX issues reported by Bryan:

1. **Swipe only works below the tab pills.** The `GestureDetector` wrapping tab content only covers the tab content area (`AnimatedSize` child), not the full screen. Since content above the pills (media background, back button) is outside the gesture detector, users can't discover swiping by dragging anywhere on the screen.

2. **No swipe animation on content.** When swiping, the tab content switches instantly (conditional rendering) and only the pill indicator animates. There's no visual feedback that content is sliding in from the side, making it non-obvious what's happening.

## Context from Comments

- **Bryan (reporter):** Initially couldn't figure out swiping was supported — it appeared to "intermittently work" until he realized only the area below the pills responded. Wants the whole screen to be the swipe target and content to animate during the swipe.
- **Thomas (assignee):** Argues that swiping above the pills shouldn't change tabs because (a) the top area is semantically unrelated to tab content, and (b) when viewing items outside the feed, right-swipe navigates back (SwipeToCloseMixin) and would conflict. Agreed to add content slide animation to make tab switching more discoverable.
- **Decision:** Add slide animation to tab content. Keep the swipe gesture zone limited to the tab content area below the pills.

## Potential Solutions

### Option A: AnimatedSwitcher with SlideTransition

- **Approach:** Wrap the tab content conditional in an `AnimatedSwitcher` with a custom `SlideTransition` builder. Track the swipe direction (left vs right) to determine whether content slides in from the left or right. Use a `ValueKey(state.activeTab)` so AnimatedSwitcher detects changes.
- **Pros:** Simple to implement, uses standard Flutter widgets, no new state management needed. AnimatedSwitcher handles both enter and exit transitions.
- **Cons:** AnimatedSwitcher cross-fades by default and needs a custom `transitionBuilder` + `layoutBuilder` to get pure slide behavior. Both old and new content render simultaneously during the transition, which could cause layout issues with the chat pane's fixed height.
- **Effort:** Small.

### Option B: PageView with physics-based swiping

- **Approach:** Replace the `GestureDetector` + conditional rendering with a `PageView` containing the three tab panes. Wire the `PageController` to the viewmodel's `activeTab` state. The PageView handles both gesture detection and content animation natively.
- **Pros:** Native swipe-to-page behavior with real-time drag tracking (content follows finger). Handles momentum, snap-back, and edge bounce automatically. Best UX fidelity.
- **Cons:** Requires reworking the tab content layout. The chat pane has a fixed height (`chatPaneHeight`) while other panes use intrinsic sizing — mixing these in a PageView requires careful height management. PageView wants all children to be the same size. May conflict with scroll gestures inside the chat pane. Larger refactor across all three content views (gear, experience, request).
- **Effort:** Large.

### Option C: Explicit slide animation with AnimationController

- **Approach:** Add an `AnimationController` to each content view. On tab change, animate the old content out (slide off-screen in the swipe direction) and animate the new content in (slide in from the opposite side). Use `SlideTransition` wrapping each pane.
- **Pros:** Full control over animation timing, direction, and easing. Can handle the chat pane's fixed height cleanly since each pane is independently wrapped.
- **Cons:** More boilerplate — need to manage animation controller lifecycle, track previous tab for direction, and coordinate the exit/enter animations. Must be replicated in all three content views.
- **Effort:** Medium.

## Recommendation

**Option A (AnimatedSwitcher with SlideTransition)** is the right choice. It provides the visual feedback Bryan requested (content sliding in/out on tab switch) with minimal code change. The key implementation details:

- Track swipe direction in state (or derive from `previousTab` vs `activeTab`) to determine slide direction.
- Use `AnimatedSwitcher` with a custom `transitionBuilder` that uses `SlideTransition`.
- Keep the existing `GestureDetector` for swipe detection — just add the animation wrapper around the content.
- The `AnimatedSize` can stay to handle height differences between panes.

Option B (PageView) would give the best UX but is a significant refactor with real risks around the chat pane's fixed height and scroll gesture conflicts. Not worth it for the incremental benefit over a slide animation.

## Open Questions

- **Should the slide animation also trigger on tab pill taps?** — Recommendation: Yes, for consistency. If content slides on swipe, it should also slide on tap. The direction should be based on whether the new tab index is higher or lower than the current one.
- **Animation duration?** — Recommendation: 250-300ms with `Curves.easeInOut`, matching the existing pill slide animation (280ms) for visual coherence.
- **Should the swipe velocity threshold (300) be adjusted?** — Recommendation: Keep at 300. Bryan's issue was discoverability (no animation feedback), not sensitivity.
- **Any new user-visible strings?** — Recommendation: Unlikely (animation is purely visual), but if accessibility labels or tooltips are added they must go through i18n (`app_en.arb` + `app_es.arb`).

## Implementation Plan

### Phase 1: Add slide animation to tab content

**Goal:** Content visually slides left/right when switching tabs, making the tab navigation obvious.

**Tasks:**
- [ ] Add a `_previousTab` field (or equivalent) to each content view to track swipe direction (this is local UI state — `setState` is appropriate per architecture guidelines)
- [ ] Wrap the tab content conditional in an `AnimatedSwitcher` with a custom `SlideTransition` transitionBuilder in `gear_content_view.dart`
- [ ] Determine slide direction: if new tab > old tab, slide in from right; otherwise from left
- [ ] Key the child widget with `ValueKey(state.activeTab)` so AnimatedSwitcher detects the change
- [ ] Ensure the chat pane's fixed `SizedBox(height: chatPaneHeight)` works correctly during the transition
- [ ] Replicate the same pattern in `experience_content_view.dart` and `request_content_view.dart`
- [ ] Test on Android and iOS: verify slide animation on swipe and on pill tap

**Validates:** Bryan's feedback that the animation makes tab switching discoverable and obvious.

### Phase 2: Extract shared swipe+animation logic (optional)

**Goal:** Reduce duplication across the three content views.

**Tasks:**
- [ ] Extract the `GestureDetector` + `AnimatedSwitcher` + direction tracking into a shared widget (e.g., `SwipeableTabContent`)
- [ ] Refactor gear, experience, and request content views to use the shared widget
- [ ] Ensure the tab bar pill animation timing stays coordinated with content animation

**Validates:** Code stays maintainable as more item types or tabs are added.

## Architecture Compliance Notes

- **Presentation-only change.** No data layer, repository, service, or server changes needed.
- **`_previousTab` is local UI state** — tracked via `setState` in the widget, not in the Freezed viewmodel state. Animation direction is ephemeral and has no business logic significance.
- **`SwipeToCloseMixin` conflict is already mitigated.** The screen-level `SwipeToCloseWrapper` uses `onHorizontalDragEnd` on the outer `GestureDetector`. The tab-level `GestureDetector` is nested inside and only wraps the content area below the pills. Flutter's gesture arena resolves this correctly — the inner detector wins for the tab content area. The current approach (keeping swipe zone below the pills) avoids the conflict Bryan raised.
- **No caching, i18n, or server boundary impact.**

## Related Issues

- #1102 — PR that originally implemented tab swiping (convergence branch)
