# Issue #1895: Restructure bottom + floating nav: drop search tab, move create button right, remove hamburger, avatar opens profile modal

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1895
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/App Design/direction-a-working-mock.html](../cowork/App%20Design/direction-a-working-mock.html), [docs/cowork/mocks/simple/new_profile_modal.png](../cowork/mocks/simple/new_profile_modal.png)

## Problem Statement

The current nav has two distinct surfaces for "where am I and where do I
go": a bottom pill nav (Feed / Inbox / Create / Search / Workshop) and a
top floating header whose hamburger opens a left sidebar that combines
two unrelated concerns — **per-community filtering** (All Circles + one
entry per community) and **account actions** (profile, settings,
feedback, logout).

The new design direction collapses both:

1. **Context is always the whole portfolio.** Per-community filtering at
   the global-context level goes away. Filtering moves down into the
   screens that need it (e.g. the Workshop carousel landing in #1898).
2. **Search moves into the Feed header.** The dedicated Search/Discover
   tab is no longer a destination (#1896 lands the in-Feed search
   affordance).
3. **Create becomes a FAB.** Currently it's a visual tab at index 2 that
   actually opens a modal rather than navigating — it's already
   modal-shaped, just positioned as a tab. The mock floats it to the
   right.
4. **Account actions move into an avatar-tap modal.** The four items
   that survive from the sidebar (Profile, Settings, Feedback, Logout)
   land in a frosted-glass modal opened by tapping the user's avatar.
   The avatar lives on the **Workshop tab's** floating header
   (trailing/right edge) — not on Feed, Inbox, or other tabs, and not
   where the hamburger was (leading/left edge of every screen).

This issue is the **foundation** — three sibling issues (#1896 feed,
#1897 inbox, #1898 workshop) all assume the hamburger + sidebar are
gone, so they can't ship until this lands.

## Context from Comments

No comments on the issue — it was filed and immediately planned. The
risk: there's been no review or pushback yet on the prescriptive UI
choices (FAB position, modal exactly mirrors sidebar contents minus
filter, etc.). The mock answers most of these, but some product
decisions (see Resolved Decisions) deserve a check before implementation.

## Audit Notes (2026-05-12)

This plan was audited against [docs/client/architecture.md](../client/architecture.md),
[docs/client/modals.md](../client/modals.md), [docs/client/accessibility.md](../client/accessibility.md),
[docs/client/i18n.md](../client/i18n.md), [docs/client/testing.md](../client/testing.md),
[docs/client/caching.md](../client/caching.md), and [docs/client/logout.md](../client/logout.md).
Server docs (`architecture.md`, `observability.md`) were checked but
contain no requirements that apply to a pure-client nav restructure.

The following corrections were folded into the body below; this section
records what changed and why, not the new instructions themselves:

- **Modal architecture corrected.** The original draft cited
  [duration_picker_modal.dart](../../app/lib/presentation/widgets/experience/duration_picker_modal.dart)
  as the structural model. That's a legacy centered-`Dialog` pattern.
  Per [docs/client/modals.md](../client/modals.md), every new
  bottom-sheet modal goes through `showAccessibleModal` + `GlassSheet`
  with `ModalTheme` geometry and `AppColors.modal*` tokens; the glass
  primitives in [`app/lib/presentation/widgets/modal/glass/`](../../app/lib/presentation/widgets/modal/glass/)
  are required, not optional. Phase 1 tasks rewritten accordingly. The
  bottom-sheet-vs-centered-dialog form factor became a new Open
  Question.
- **Accessibility primitives required.** Per [docs/client/accessibility.md](../client/accessibility.md),
  tap targets compose from `Tappable` (not `GestureDetector` /
  `InkWell`), icon-only controls from `IconAction`, and the modal
  itself is opened via `showAccessibleModal`. Each interactive widget
  needs a `semanticsLabel:` sourced from `context.l10n`. CI
  (`lint:dart:a11y`) enforces this.
- **i18n is mandatory.** No raw string literals on `tooltip:` /
  `semanticLabel:` / inside `Semantics(...)`, no hardcoded English in
  the modal copy. New keys go in both `app/lib/l10n/app_en.arb` and
  `app_es.arb` with the `profile*` and `a11y*` prefixes, then
  `npm run generate` regenerates the synthetic package. Added a
  dedicated l10n subtask to Phase 1.
- **Navigation from inside the modal.** Per [docs/client/architecture.md](../client/architecture.md#navigation--gestures),
  navigations out of a modal use `NavigationHelpers.pushScreen()` or
  `pushWithSlide()` with `useRootNavigator: true` (so the new screen
  overlays the modal). Plan now spells this out for each of the four
  modal rows.
- **Theme-aware colors.** Per [docs/client/architecture.md](../client/architecture.md#colors-and-theming),
  the new FAB + any non-glass surfaces touched in Phase 3 must use
  `AppColors.*(context)` helpers, never `Colors.white`, `Colors.grey[...]`,
  or hardcoded hex. Inside the glass modal itself, the documented
  white-on-glass legibility exception applies (`AppColors.modal*`
  tokens are theme-agnostic by design — see [modals.md "White-on-glass
  legibility deviation"](../client/modals.md#glass-modal-primitives)).
- **Testing focus realigned.** Per [docs/client/testing.md](../client/testing.md),
  ViewModel tests are primary; widget tests are secondary. The profile
  modal is presentation-only with no Notifier of its own — so it gets
  one widget test for dispatch verification (each row triggers the
  right downstream call) and no ViewModel test. No disposal test
  needed because no new async Notifier is introduced.
- **`selectedCommunityProvider` setter — fail fast.** Per [CLAUDE.md](../../CLAUDE.md)
  Fail Fast guidance, Phase 4's neutering uses an assertion that
  rejects non-null IDs, rather than a silent no-op. The cleanup PR
  after #1896–#1898 will collapse the field shape entirely; this
  change just ensures any stray caller surfaces immediately during
  the interim.
- **Logout async-safety.** Per [docs/client/logout.md](../client/logout.md),
  `AuthStateNotifier.logout()` is async and triggers a router
  redirect. The modal's Logout row handler pops the modal first, then
  calls `logout()`, with a `context.mounted` guard between (documented
  Widget Async Exception pattern in [architecture.md](../client/architecture.md#async-safety-in-notifiers)).

## Codebase Survey

The Explore agent's full survey is summarized inline; the key handles:

- **Bottom nav** — built inline in [app/lib/presentation/screens/home/home_screen.dart:254-453](../../app/lib/presentation/screens/home/home_screen.dart#L254).
  Five visual items; index 2 (Create) is modal-only and skipped in the
  `IndexedStack` mapping ([_visualToStackIndex](../../app/lib/presentation/screens/home/home_screen.dart#L172)).
  Create tap routes to [PlusButtonModal](../../app/lib/presentation/widgets/plus_button_modal.dart)
  via `_onAddTap()`.
- **Floating header** — [app/lib/presentation/widgets/floating_header.dart](../../app/lib/presentation/widgets/floating_header.dart).
  `showMenuIcon: true` renders the hamburger via `_buildMenuIconWithBadge()`;
  default tap calls `ref.read(homeProvider.notifier).toggleSidebar()`.
- **Sidebar** — [app/lib/presentation/screens/home/sidebar.dart](../../app/lib/presentation/screens/home/sidebar.dart).
  Three mixed concerns: user card zone (`_buildUserCardZone`, line 98)
  → Portfolio; communities list (`_buildCommunitiesList`, line 226)
  with All-Circles selector at line 306 and per-community entries at
  line 370; footer with Settings (line 443), Feedback (line 486),
  Logout (line 525).
- **Frosted-glass modal pattern (canonical, per [docs/client/modals.md](../client/modals.md)).**
  Bottom-sheet modals — which is the form factor the new profile modal
  takes (confirmed in Resolved Decisions) — are opened with
  `showAccessibleModal` and wrap their content in [`GlassSheet`](../../app/lib/presentation/widgets/modal/glass/)
  from `app/lib/presentation/widgets/modal/glass/`. Geometry comes
  from `ModalTheme` in [app_theme.dart](../../app/lib/core/theme/app_theme.dart)
  (12px horizontal inset, 28px top corners, 92% max height); tints from
  `AppColors.modal*` ([app_colors.dart](../../app/lib/core/theme/app_colors.dart));
  the dark scrim is the system modal barrier (pass
  `barrierColor: AppColors.modalBackdrop`). Title rows compose from
  `GlassModalHeader`, footer rows from `GlassFooterButtons`, body rows
  from `GlassInlineAction` (a 44px translucent row with leading icon +
  chevron — natural fit for the four menu items). Reference modals to
  imitate: [unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart)
  (default shape) and [location_picker_modal.dart](../../app/lib/presentation/widgets/location/location_picker_modal.dart)
  (overlay-handle variant). The legacy [duration_picker_modal.dart](../../app/lib/presentation/widgets/experience/duration_picker_modal.dart)
  centered-`Dialog` pattern is **not** the model for new bottom-sheet
  modals.
- **Selected community state** — [community_providers.dart:105-149](../../app/lib/services/providers/community_providers.dart#L105).
  `selectedCommunityProvider`'s `effectiveCommunityIds` getter is read
  by virtually every screen's RPC call. Today `null` means "All Circles",
  resolved to the full list; non-null means single-community filter.
- **User avatar widget** — [app/lib/presentation/widgets/user_avatar.dart](../../app/lib/presentation/widgets/user_avatar.dart),
  currently only used in the sidebar.
- **Routing** — [app/lib/core/router/app_router.dart:434](../../app/lib/core/router/app_router.dart#L434).
  Tabs are not separate routes — they're indices inside `HomeScreen`'s
  `IndexedStack`. So "removing the search tab" is a one-screen change,
  not a router change.

## Potential Solutions

### Option A: UI hide only (minimum movement)

- **Approach:** Hide the search tab, hamburger icon, and community list
  in the sidebar. Add the FAB. Add the new profile modal. Leave the
  sidebar widget alive, leave `selectedCommunityProvider` untouched
  (just force `selectedCommunityId == null` so `effectiveCommunityIds`
  always returns the full list).
- **Pros:** Smallest diff. Lowest risk of regressing per-screen
  filtering that legitimately uses `effectiveCommunityIds`. Easy to
  revert.
- **Cons:** Leaves a non-trivial amount of dead UI code (sidebar widget,
  hamburger handler, community-filter selectors) in the tree, which the
  cleanup PR is already promised to remove anyway. Reviewers may push
  back: "if we're already touching this, why not delete?"
- **Effort:** Small.

### Option B: UI restructure + delete dead UI surfaces (recommended)

- **Approach:** Same UI changes as Option A. Additionally:
  - Delete the sidebar widget entirely (its three concerns are split:
    Portfolio/Settings/Feedback/Logout → profile modal; community
    filter → removed). The `homeProvider.toggleSidebar()` plumbing goes
    too.
  - Make `selectedCommunityProvider` a pass-through that always returns
    every membership in `effectiveCommunityIds` (don't delete the
    provider — it's read everywhere and ripping that out is the job of
    the cleanup PR after #1896–#1898 land).
  - Bottom nav drops to 4 items (Feed / Inbox / Workshop / + one
    floating Create FAB on the right). The `_visualToStackIndex` map
    collapses; the Discover stack entry remains (#1896 will repoint the
    Feed's search affordance at the Discover machinery before deleting
    the Discover tab).
- **Pros:** Cleanly removes the surfaces this issue explicitly calls
  out. Leaves the deeper filter-wiring cleanup (RPC sites passing
  `effectiveCommunityIds`) for the dedicated cleanup PR, which is the
  scope the issue note describes.
- **Cons:** Has to be careful not to regress per-screen consumers of
  `selectedCommunityProvider` while it's being neutered (Workshop
  filters by selection today and gets a carousel in #1898 — between the
  two issues landing, Workshop just shows the full portfolio, which is
  fine and matches the new direction).
- **Effort:** Medium.

### Option C: Full ripout including provider + all call sites

- **Approach:** Option B plus delete `selectedCommunityProvider` and
  update every consumer.
- **Pros:** Single PR ends the per-community filter story.
- **Cons:** Bleeds into the scope the issue explicitly defers to a
  cleanup PR after #1896–#1898. Inflates blast radius and review time.
  Workshop's #1898 carousel may want to *reuse* the
  selection-state provider (renamed / repurposed), so deleting it now
  could force a re-introduction.
- **Effort:** Large.

## Recommendation

**Option B.** It's the scope the issue actually describes — the bullet
"Remove the community-filter selection UI (sidebar / drawer) that the
hamburger opened" reads as deleting the sidebar widget, but the closing
note ("After all four nav-restructure issues land, do a sweep for dead
code... and clean it up in a follow-up PR") explicitly defers the
deeper ripout.

The provider stays alive but neutered (always all communities). That
keeps RPC call sites that currently pass `effectiveCommunityIds` working
without modification, while removing the user-facing way to ever set it
to anything but the full list. The cleanup PR after #1896–#1898 collapses
those call sites and may delete (or rename to a workshop-scoped
provider) `selectedCommunityProvider`.

## Resolved Decisions (2026-05-12 issue review)

All open questions were resolved during issue review with the
project owner. Decisions are folded into the Implementation Plan
below; recorded here for the audit trail.

- **Profile modal is a bottom-sheet** on the glass material (`showAccessibleModal` + `GlassSheet`).
- **FAB invokes the existing `PlusButtonModal`** unchanged (4 options:
  Share Gear, Invite Experience, Request Something, Invite User).
- **FAB position: bottom-right, to the right of the floating bottom nav, vertically level with it** — not above it. See `direction-a-working-mock.html` and `new_*.png` mocks for exact placement.
- **Avatar lives only on the Workshop tab's floating header, on the
  trailing (right) edge.** It does **not** replace the hamburger
  position on Feed/Inbox/etc. The hamburger is removed from every
  screen's floating header entirely; only the Workshop's header gets
  the new avatar control. (The hamburger sat on the leading/left
  edge; the avatar sits on the trailing/right edge of one screen.)
- **Profile modal has exactly four rows** — no Portfolio entry. The
  "Profile" row opens the user's own profile screen (the standard
  user-detail screen reached by tapping yourself in a roster), **not**
  the Portfolio impact-summary screen.
- **The unread badge on the hamburger no longer exists** (already
  removed elsewhere). No badge-investigation task needed.
- **Avatar in the header shows the user's actual avatar** via the
  existing [`UserAvatar`](../../app/lib/presentation/widgets/user_avatar.dart) widget.
- **Profile modal uses standard bottom-sheet slide-up animation** —
  no custom animation.
- **No in-flight feature flags or A/B tests** on the sidebar; safe
  to delete.

## Implementation Plan

### Phase 1: Build the avatar-tap profile modal

**Goal:** Ship the new modal as a self-contained widget. Not yet
reachable from any tap target, so this phase has zero functional
change in the app.

**Tasks:**
- [ ] Add the new l10n keys to **both** [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb)
      **and** [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb), with the
      `profile*` / `a11y*` prefixes:
  - `profileMenuTitle` (modal header, if the design uses one)
  - `profileMenuProfile`, `profileMenuSettings`, `profileMenuFeedback`,
    `profileMenuLogout`
  - `a11yOpenProfileMenu` (avatar tap target label used in Phase 2)
  - `a11yProfileMenuRow` parameterized variants if needed for each row's
    `Tappable` `semanticsLabel`.

      Then run `npm run generate` to regenerate the synthetic
      `AppLocalizations` package.
- [ ] Create `app/lib/presentation/widgets/profile_menu_modal.dart` as
      a `StatelessWidget` composing the glass primitives — **not** the
      legacy centered-Dialog pattern:
  - Body wrapped in [`GlassSheet`](../../app/lib/presentation/widgets/modal/glass/).
    Use the default geometry (drag handle in column mode, 28px top
    corners, 92% max height — handled by `GlassSheet` itself via
    `ModalTheme`). Do not hard-code radii, blur sigmas, or insets.
  - Title via [`GlassModalHeader`](../../app/lib/presentation/widgets/modal/glass/glass_modal_header.dart)
    if the mock includes a header band (verify against
    `new_profile_modal.png` during implementation).
  - Each of the four menu rows rendered as [`GlassInlineAction`](../../app/lib/presentation/widgets/modal/glass/)
    (44px translucent row, leading icon, chevron). Use [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
    underneath if `GlassInlineAction` doesn't already wrap its tap
    target with one (verify against the widget's API).
  - Icons + labels:
    - **Profile** → leading person icon, label `context.l10n.profileMenuProfile`.
    - **Settings** → leading settings icon, label `context.l10n.profileMenuSettings`.
    - **Feedback** → leading feedback icon, label `context.l10n.profileMenuFeedback`.
    - **Logout** → leading logout icon, label `context.l10n.profileMenuLogout`.
- [ ] Row tap handlers — all use the documented Widget Async Exception
      pattern (single `await`, `if (!context.mounted) return;` guard
      after):
  - **Profile** → `Navigator.pop(context);` then
    `NavigationHelpers.pushScreen(context, UserScreen(userId: currentUserId), useRootNavigator: true)`
    — the standard user-detail screen (the one shown when tapping
    yourself in a roster). **Not** the Portfolio screen the sidebar's
    `_buildUserCardZone` navigates to today; that destination is
    intentionally dropped in the new direction. Confirm the screen
    class name during implementation (e.g. `UserScreen` /
    `UserDetailScreen`) by grepping for the existing roster-tap
    destination.
  - **Settings** → `Navigator.pop(context);` then
    `NavigationHelpers.pushScreen(context, const SettingsHubScreen(), useRootNavigator: true)`
    (matches the sidebar's existing `_buildSettingsSection` call).
  - **Feedback** → `Navigator.pop(context);` then
    `await FeedbackSheet.show(context);` (matches the sidebar's
    `_buildFooter` Feedback button).
  - **Logout** → `Navigator.pop(context);` then
    `await ref.read(authStateProvider.notifier).logout();` (matches the
    sidebar's `_buildFooter` Logout button; the notifier itself handles
    the full teardown sequence per [docs/client/logout.md](../client/logout.md)).
- [ ] Add the canonical entry point as a static method following the
      shape in [modals.md "Standard glass-modal shape"](../client/modals.md#standard-glass-modal-shape):

      ```dart
      static Future<void> show(BuildContext context) {
        return showAccessibleModal<void>(
          context,
          isScrollControlled: true,
          backgroundColor: Colors.transparent,
          barrierColor: AppColors.modalBackdrop,
          builder: (context) => const ProfileMenuModal(),
        );
      }
      ```
- [ ] All colors come from `AppColors.modal*` tokens; all icons from
      the existing icon set used elsewhere in the app. No `Color()`
      literals, no hardcoded radii — `lint:dart:a11y` and the modal
      primitives enforce this.
- [ ] **Test (widget-level dispatch verification, per [docs/client/testing.md](../client/testing.md))**:
      one widget test in `app/test/presentation/widgets/profile_menu_modal_test.dart`
      that pumps the modal inside a `ProviderContainer` and verifies
      each of the four `Tappable` rows fires the expected dispatch
      (mock `AuthStateNotifier` for the logout row; verify
      `Navigator.pop` is called before the dispatch for the navigating
      rows). Use the l10n helpers from
      [app/test/helpers/l10n_helpers.dart](../../app/test/helpers/l10n_helpers.dart)
      to wrap `MaterialApp`. No ViewModel test needed — the modal is
      presentation-only with no new business logic.

**Validates:** The new modal renders against the glass primitives and
each of the four destinations works without changing any nav surface.
`lint:dart:a11y`, `lint:dart:i18n`, and `flutter test` all pass.

### Phase 2: Floating header — remove hamburger; add avatar to Workshop only

**Goal:** Remove the hamburger from every screen's floating header.
Add the user avatar **only** to the Workshop tab's floating header,
on the trailing (right) edge, wired to open the profile modal. After
this phase, the sidebar's account items are reachable solely via the
Workshop tab; the sidebar widget still exists (and its community
filter is still wired) but no UI path opens it. Cleanup happens in
Phase 4.

**Tasks:**
- [ ] **Remove the hamburger from every floating-header call site.**
      Audit all callers of [`FloatingHeader`](../../app/lib/presentation/widgets/floating_header.dart)
      and ensure none of them pass `showMenuIcon: true`. Where call
      sites today explicitly pass `true`, drop the argument (default
      handling becomes irrelevant once Phase 4 deletes the menu
      branch entirely). At the end of this task, no live UI path
      renders the hamburger icon.
- [ ] **Add the avatar to the Workshop tab's floating header
      trailing slot only.** In [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart)
      (or wherever the Workshop tab's `FloatingHeader` is constructed
      — see [docs/workshop.md "Floating header"](../workshop.md#client-rendering)),
      pass `trailing:` set to a [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
      wrapping the user's current [`UserAvatar`](../../app/lib/presentation/widgets/user_avatar.dart)
      (sourced from the existing logged-in user provider — same data
      the sidebar's user-card zone reads today). The `Tappable` takes
      `semanticsLabel: context.l10n.a11yOpenProfileMenu` (the key added
      in Phase 1) and `onTap: () => ProfileMenuModal.show(context)`.
- [ ] **Do not** add the avatar to Feed, Inbox, or any non-Workshop
      tab's floating header. Those headers lose the hamburger and
      gain nothing in its place (the leading slot becomes empty, with
      the `title:` / `content:` slots laying out as today). Verify
      visually against the mocks: `new_feed.png`, `new_inbox.png`
      show no leading-edge control; `new_workshop.png` shows the
      avatar trailing.
- [ ] Update widget tests asserting the hamburger icon's presence —
      delete those assertions on non-Workshop headers, replace with
      avatar-presence + `semanticsLabel` assertion on the Workshop
      header.
- [ ] Run `npm run lint:dart:a11y` to confirm no new accessibility
      violations (the avatar `Tappable` must have a non-empty l10n
      `semanticsLabel`).

**Validates:** Hamburger is gone from every screen. Workshop's
floating header shows the avatar trailing; tapping it opens the
profile modal; the four account actions are reachable from there.
The sidebar widget is now orphaned (no UI path opens it) — deleted
in Phase 4.

### Phase 3: Bottom nav — drop Search tab, float Create as FAB

**Goal:** Bottom nav becomes 4 items (Feed / Inbox / Workshop +
floating Create FAB on the right).

**Tasks:**
- [ ] In [home_screen.dart](../../app/lib/presentation/screens/home/home_screen.dart):
  - Remove the Search/Discover visual tab from the bottom nav
    (`_buildBottomNavigation` line 254 and the `_visualToStackIndex`
    map at line 172).
  - Keep the `DiscoverScreen` in the `IndexedStack` for now — #1896
    repoints the Feed's search affordance at the Discover machinery
    before any actual delete.
  - Extract the Create button out of the bottom nav row entirely.
    Render it as a custom FAB-shaped widget matching the mock's visual
    treatment, positioned **bottom-right, immediately to the right of
    the floating bottom nav pill, vertically level with it** (same Y
    center as the pill — not raised above the pill). Show on every
    tab where the bottom nav appears. Verify exact position and
    spacing against `direction-a-working-mock.html` and `new_feed.png`
    / `new_inbox.png` / `new_workshop.png`. Wrap the tap target in
    [`Tappable`](../../app/lib/presentation/widgets/accessibility/) with
    `semanticsLabel: context.l10n.a11yCreate` (or similar — reuse any
    existing Create / Plus l10n key; add one if none exists).
  - Re-route the FAB's tap to the existing `_onAddTap()` (no change to
    `PlusButtonModal`).
- [ ] **Theme-aware colors**, per [architecture.md "Colors and Theming"](../client/architecture.md#colors-and-theming):
      use `AppColors.*(context)` helpers throughout — never
      `Colors.white`, `Colors.grey[...]`, or hardcoded hex. Add a new
      `AppColors` semantic helper if the FAB's specific surface color
      isn't already represented (e.g. `AppColors.createButtonBackground(context)`).
      `AppColors.primary` already exists per the survey and likely
      covers the green tint.
- [ ] Update widget tests asserting nav-item count or specific indices.
- [ ] Snapshot-test or manually verify the bottom-nav pill against
      `direction-a-working-mock.html` for spacing and selection-circle
      animation parity, **and in dark mode** to confirm theme-awareness.

**Validates:** The bottom nav has 4 items in the new positions; tapping
the FAB still opens `PlusButtonModal`; no in-app navigation is broken
(Search tab removal doesn't dead-end any link because the routes route
to home, not to a search-tab path).

### Phase 4: State neutering — sidebar deletion + provider pass-through

**Goal:** Delete the now-unreachable sidebar widget and force
`selectedCommunityProvider` to always represent the full portfolio.
After this phase, the UI surfaces described in the issue are gone but
RPC call sites passing `effectiveCommunityIds` still work because
`effectiveCommunityIds` now always equals the full list.

**Tasks:**
- [ ] Delete [sidebar.dart](../../app/lib/presentation/screens/home/sidebar.dart)
      and any rendering of it from `HomeScreen`.
- [ ] Remove `toggleSidebar` / sidebar-state fields from the home
      notifier (the home `Riverpod` notifier or equivalent that backs
      `homeProvider`).
- [ ] In [floating_header.dart](../../app/lib/presentation/widgets/floating_header.dart),
      delete the `showMenuIcon` parameter and both default-leading
      branches:
  - The hamburger branch (`_buildMenuIconWithBadge` and its
    `toggleSidebar` tap handler) — orphaned after Phase 2.
  - The community-avatar branch (the `showMenuIcon: false` default
    that renders the selected community's avatar) — orphaned by the
    portfolio-only direction; no caller has a selected community to
    show.

      The `leading:` slot stays as an optional override (Workshop
      passes the user avatar via `trailing:`, but other callers may
      legitimately want a custom leading widget in future).
- [ ] In [community_providers.dart](../../app/lib/services/providers/community_providers.dart):
  - Replace `setSelectedCommunity`'s body with an `assert(communityId == null,
    'Per-community filter removed in #1895; selection must be null')`
    followed by setting `selectedCommunityId: null` (no-ops in practice
    since state already starts null, but the assert surfaces any stray
    caller immediately in debug builds). Fail-fast per [CLAUDE.md "Fail
    Fast"](../../CLAUDE.md#error-handling-and-robustness).
  - Verify `effectiveCommunityIds` already returns the full list when
    `selectedCommunityId == null` (it does, per the survey).
  - Leave the provider, its derived getters, and all RPC call sites
    alone — the cleanup PR after #1896–#1898 collapses those.
  - **Cache implications, per [docs/client/caching.md](../client/caching.md):**
    after this change, any cache entry keyed off
    `effectiveCommunityIds.join(',')` (or similar derived-from-selection
    keys) now always uses the full-portfolio key. Existing per-community
    entries become unreachable but will expire on the 30-minute TTL — no
    manual invalidation required. Verify by grep that no cache namespace
    uses `selectedCommunityId` directly as a key prefix (if any does,
    add a one-shot `repository.invalidateAll()` to the migration).
- [ ] Sweep imports for `Sidebar` / `selectedCommunityId` setter calls;
      remove any newly-dead refs. Don't touch the provider itself or
      its readers.
- [ ] Run `npm run lint:dart:size` and `flutter test --reporter expanded`
      to confirm no regressions.

**Validates:** All sidebar UI is gone. Every screen reads from "the
full portfolio" regardless of any prior in-flight community selection.
The four sibling issues (#1896 feed, #1897 inbox, #1898 workshop) are
unblocked.

## Related Issues

- #1896 — Feed "Search nearby" header + simplified search results.
  Depends on Phase 3 (Search tab removal) of this issue.
- #1897 — Inbox floating-nav search bar + filter pills. Depends on
  Phase 2 (hamburger removal) of this issue.
- #1898 — Workshop community carousel + people-count metric + slide-in
  detail. Depends on Phase 2 + Phase 4 (community-filter UI removal,
  selection-provider neutering) of this issue.
- (TBD post-#1896/#1897/#1898) — Dead-code cleanup PR. Collapses RPC
  call sites passing `effectiveCommunityIds` and may delete or rename
  `selectedCommunityProvider`.
