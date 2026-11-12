# Issue #1963: Make user settings & feedback easier to find: show avatar on Feed/Inbox; replace Workshop avatar with create-community button

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1963
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none

## Decisions made during planning (2026-05-15)

The issue body left two AC items open ("avatar on Inbox" and "where the Workshop profile-menu surface lives"). The author resolved both during planning, narrowing the scope:

- **Inbox is out of scope.** No avatar on the inbox floating header. The inbox keeps its existing trailing edit affordance unchanged. Rationale: the inbox already has a competing trailing control (edit affordance), and the discoverability problem is solved well enough by the avatar living on Feed alone — the Feed is the most-trafficked tab.
- **Workshop has no profile-menu surface.** The Workshop floating header loses the avatar entirely and gains the create-community `IconAction` in its trailing slot. Users reach Settings/Feedback from Feed. Rationale: Workshop is a community-curation surface; mixing account-level controls into it muddies the meaning, and the create-community action is on-theme.

Decisions on the remaining OQs (icon, sizing, analytics, single-community edge case) follow the planner's recommendations — see [Open Questions](#open-questions).

## Audit Notes (2026-05-15)

The original draft passed an audit against `docs/client/{architecture,accessibility,i18n,testing,caching}.md`. The fixes below were folded into the plan; flagged here so the rationale is preserved:

- **a11y primitive (Phase 3):** swap raw `Tappable` + `Icon` for the codebase's canonical icon-only primitive `IconAction` (under `widgets/accessibility/`). `IconAction` already requires `semanticsLabel` and provides `tooltip` (defaults to `semanticsLabel`), which satisfies `require_semantic_label_on_icon_button`. Source: [docs/client/accessibility.md](../client/accessibility.md) — "Icon-only button → `IconAction`".
- **i18n symmetry (Phases 3 & 4):** every l10n change must hit BOTH `app_en.arb` AND `app_es.arb`, each with `@key` metadata. Spanish translations for `a11yOpenProfileMenu`, `workshopCarouselNew`, `a11yWorkshopCarouselAddCommunity` already exist (verified); the orphan keys must be deleted from both files, and the new `a11yWorkshopCreateCommunity` key must be added to both. Source: [docs/client/i18n.md](../client/i18n.md) — "Each new string needs both locales."
- **Testing scope (Phases 1–4):** the original plan over-indexed on widget tests. Per [docs/client/testing.md](../client/testing.md) the doctrine is "Test ViewModels, not widgets" and "Don't test Flutter framework behavior." Widget tests were trimmed to two cases that actually exercise branching logic: (a) `ProfileMenuAvatar`'s auth-null-vs-loaded branch in Phase 1, and (b) the carousel index-math regression in Phase 4. Phase 2/3 tests that merely asserted "trailing slot has widget X" were removed — that's a composition assertion the framework already enforces.
- **`leading` slot pattern (Phase 3):** clarified that the canonical use of `FloatingHeader` is `leading: <bare child>` + `onLeadingTap: …` + `leadingSemanticsLabel: …`. The header wraps the child in its own `Tappable` internally, so passing a pre-`Tappable`-wrapped widget double-wraps and can defeat the semantics label. Dropped the original "fallback" wording — use the canonical form from the start.
- **Verification commands (Phase 5):** added `npm run lint:dart:a11y`, `npm run lint:dart:i18n`, and `npm run lint:ephemeral-comments` to the gate list. The a11y allowlist is empty and CI rejects any net new violation; the i18n lint catches hardcoded strings; the ephemeral-comments lint (per CLAUDE.md) rejects phase markers in comments.
- **No server, caching, or observability impact:** this is a pure presentation-layer change. No new repositories, no new ViewModels, no proto changes, no logging. The repository/cache/server reference docs apply but produce no new tasks.

## Problem Statement

The profile/account menu (`ProfileMenuModal`) is the entry point to **Settings** and **Feedback**, but today it is only reachable from the **Workshop** tab's floating-header trailing slot. Users sitting on **Feed** or **Inbox** — by far the more common tabs — have no obvious affordance to reach those flows. They have to switch tabs first, which is non-obvious and unrelated to the task.

Meanwhile, the Workshop's community carousel has a leading dashed-circle "+" entry ([workshop_community_carousel.dart:235-261](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L235-L261)) whose sole job is to open `CommunityCreationModal`. The action is Workshop-specific (creating a community), so it would fit cleanly in the Workshop tab's floating header — freeing the carousel from a non-community pill, and freeing the trailing-nav slot from a generic-account control on a tab that's specifically about a single community.

Net change:

- **Feed:** floating-header `trailing` = profile avatar (this is the only profile-menu surface across the app).
- **Inbox:** unchanged. (Originally proposed in the issue body but de-scoped — see [Decisions made during planning](#decisions-made-during-planning-2026-05-15).)
- **Workshop:** floating-header `trailing` = "create community" group icon (replaces the avatar). No leading; no profile-menu surface on this tab.
- **Workshop carousel:** remove the leading "+ New" entry.

## Context from Comments

The issue has **no comments** — it was just filed. The full design rationale lives in the issue body, including the acceptance criteria. The author explicitly flagged one open question: where (if anywhere) the profile menu should live on the Workshop screen now that the floating-nav slot is taken. That decision is intentionally deferred to the implementation phase.

No discussion = no maintainer pushback to weigh, no workarounds to inherit, and no alternative designs to consider yet. The plan below proposes a default for the open question rather than leaving it unbounded.

## Codebase context

What exists today (verified):

- **`FloatingHeader`** ([floating_header.dart](../../app/lib/presentation/widgets/floating_header.dart)) already supports `trailing: Widget?` as a first-class slot. No widget changes needed.
- **Workshop screen** ([workshop_screen.dart:65-71](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L65-L71)) wires `trailing: _WorkshopAvatarButton()` today, where the private `_WorkshopAvatarButton` ([:275-291](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L275-L291)) calls `ProfileMenuModal.show(context)`.
- **Feed screen** ([feed_screen.dart:133-135](../../app/lib/presentation/screens/feed/feed_screen.dart#L133-L135)) constructs `const FloatingHeader(content: SearchNearbyPill())` — no trailing. Going non-const is required to inject a trailing widget.
- **Inbox screen** ([portfolio_inbox_screen.dart:189-195](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L189-L195)) uses two header variants:
  - Non-selection mode: `FloatingHeader(content: InboxSearchPill(), trailing: canSelect ? _buildEditAffordance : null)`.
  - Selection mode: a separate `_buildSelectionHeader` swaps the whole header. **Selection mode should keep its current trailing controls untouched** — only non-selection mode gains the avatar.
- **Workshop carousel** ([workshop_community_carousel.dart:122-165](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L122-L165)) builds its items via `ListView.separated` with index math that hard-codes index 0 as "+ New":
  - `itemCount: communityStart + communities.length` where `communityStart = showEverything ? 2 : 1`.
  - Index 0 → `_AddAvatar`, index 1 → "Everything" (if `showEverything`), then communities.
  - The scroll-into-view math in [_scrollSelectedIntoView](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L56-L85) assumes the same layout: `selectedId == null` falls back to `showEverything ? 1 : 0` (the "Everything" or "New" entry).
  - Removing "+ New" shifts every later index down by one. `communityStart` becomes `showEverything ? 1 : 0`. The `selectedId == null` fallback needs to land on "Everything" when present and on the lone community otherwise.
- **CommunityCreationModal** has a static `show(context)` that returns `Future<String?>` — already drop-in.
- **ProfileMenuModal** likewise has a static `show(context)`.
- **L10n strings** in [app_en.arb](../../app/lib/l10n/app_en.arb):
  - `a11yOpenProfileMenu` ("Open account menu") — reusable verbatim for Feed and Inbox.
  - `workshopCarouselNew` ("New") and `a11yWorkshopCarouselAddCommunity` ("Add a community") — orphaned after the carousel entry is removed; either repurpose for the new trailing icon or add a new key and delete the old ones.

Icons: the codebase consistently uses `Icons.groups` / `Icons.groups_outlined` for community context. Material has `Icons.group_add_outlined`, which is the canonical "create a community" affordance. The codebase's primitive for icon-only buttons is **`IconAction`** ([widgets/accessibility/icon_action.dart](../../app/lib/presentation/widgets/accessibility/icon_action.dart)) — required by the a11y lint and used elsewhere in the inbox selection header. `Tappable` is reserved for tap targets whose child is **not** an `Icon` (e.g. text labels, avatars).

## Potential Solutions

With Inbox de-scoped and Workshop confirmed to have no profile-menu surface, the only remaining design choice is the carousel index math — and that has only one viable shape. The "Potential Solutions" section is therefore brief.

### Decision 1 — Carousel index math after removing "+ New"

Pure refactor with one viable shape: shift all subsequent indices down by one (`communityStart = showEverything ? 1 : 0`) and update `_scrollSelectedIntoView`'s `selectedId == null` fallback. No design alternatives — just a careful edit at two sites (the item builder and the scroll helper).

## Recommendation

Ship the planning-narrowed scope:

- **Feed:** add `ProfileMenuAvatar` to the floating header's `trailing` slot. This becomes the app's single profile-menu surface.
- **Inbox:** no change.
- **Workshop floating header:** delete the trailing avatar; put an `IconAction` for create-community in the trailing slot. Leading is untouched (the header continues to show only a centred title + trailing control).
- **Workshop carousel:** delete `_AddAvatar`, remove the `index == 0` branch, set `communityStart = showEverything ? 1 : 0`, and fix `_scrollSelectedIntoView`'s null-selection fallback.
- **L10n:** add `a11yWorkshopCreateCommunity` ("Create a new community" / "Crear una nueva comunidad") to both `app_en.arb` and `app_es.arb`; delete `workshopCarouselNew` and `a11yWorkshopCarouselAddCommunity` from both files once their last reference is gone.
- **Icon:** `Icons.group_add_outlined` wrapped in `IconAction(semanticsLabel: l10n.a11yWorkshopCreateCommunity, onPressed: …)`.

## Open Questions

All resolved by the author during planning:

- **OQ1 — Avatar on Inbox?** — **Resolved: no.** Inbox is out of scope.
- **OQ2 — Workshop profile-menu placement?** — **Resolved: none.** Workshop has no profile-menu surface; users reach Settings/Feedback from Feed.
- **OQ3 — Icon for "create community"?** — **Resolved: `Icons.group_add_outlined`.** Material's canonical "add to group" semantic, consistent with the codebase's use of `Icons.groups*` for community context.
- **OQ4 — Avatar size on Feed?** — **Resolved: `radius: 14`.** Same as the old Workshop avatar — no reason to diverge.
- **OQ5 — Analytics event for create-community?** — **Resolved: reuse, don't add.** If a "+ New" tap was tracked, fire the same event from the new Workshop header tap site. Verify by greping the analytics layer before deleting `_onAddCommunity`.
- **OQ6 — Single-community null-selection fallback?** — **Resolved: auto-select the lone community.** When there's one community and no "Everything", `workshopCommunityProvider` should hold that community's id rather than null. Confirm by reading the provider's initialization logic; if it currently leaves `selectedId == null` in that state, tighten it as part of Phase 4.

## Implementation Plan

### Phase 1: Extract the profile-menu avatar widget
**Goal:** Promote the Workshop-private `_WorkshopAvatarButton` into a reusable widget so Feed can consume it. (Workshop will lose its own copy in Phase 3 — this is not "share across three tabs" anymore, but lifting the widget still keeps Feed's footprint small and avoids duplicating the auth-null guard.)
**Tasks:**
- [ ] Create `app/lib/presentation/widgets/profile_menu_avatar.dart` containing a `ProfileMenuAvatar` `ConsumerWidget` with a `const` constructor. Body: read `authStateProvider`, return `SizedBox.shrink()` when there is no user, otherwise `Tappable(semanticsLabel: context.l10n.a11yOpenProfileMenu, onTap: () => ProfileMenuModal.show(context), child: UserAvatar(user: user, radius: 14))`. `Tappable` is correct because the child is a `UserAvatar`, not an `Icon` (per [docs/client/accessibility.md](../client/accessibility.md)).
- [ ] Add **one** widget test for `ProfileMenuAvatar`: renders `SizedBox.shrink()` when `authStateProvider` has no user, and renders a tappable `UserAvatar` when it does. This is the only meaningful branch — broader composition tests are excluded per [docs/client/testing.md](../client/testing.md) ("Don't test Flutter framework behavior").

**Validates:** The widget works in isolation; the auth-state branch is covered. No user-visible change yet.

### Phase 2: Add the avatar to Feed
**Goal:** Profile menu reachable from Feed — the app's single profile-menu surface.
**Tasks:**
- [ ] [feed_screen.dart:133](../../app/lib/presentation/screens/feed/feed_screen.dart#L133): drop `const` on the `FloatingHeader`, add `trailing: const ProfileMenuAvatar()`.
- [ ] No new widget tests. The wiring is pure composition; the only branching logic (`ProfileMenuAvatar`'s auth-null guard) is already tested in Phase 1.

**Validates:** Tapping the avatar in the Feed floating header opens `ProfileMenuModal`. Visual check on iOS sim and Android emulator that the trailing slot doesn't overflow on narrow widths. **Inbox is not touched.**

### Phase 3: Replace Workshop trailing avatar with create-community button
**Goal:** Workshop's floating header reflects its function (community curation). The profile-menu surface is removed entirely from this tab.
**Tasks:**
- [ ] [workshop_screen.dart:65-71](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L65-L71): replace `trailing: _WorkshopAvatarButton()` with `trailing: IconAction(icon: Icons.group_add_outlined, semanticsLabel: context.l10n.a11yWorkshopCreateCommunity, onPressed: () => CommunityCreationModal.show(context), iconSize: 20)`. **Do not** add a `leading` widget — the Workshop header keeps its current shape (centred title + trailing control).
- [ ] Delete `_WorkshopAvatarButton` ([workshop_screen.dart:275-291](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L275-L291)) and remove the now-unused imports for `ProfileMenuModal`, `UserAvatar`, `Tappable`, and `authStateProvider` from `workshop_screen.dart`. Run `flutter analyze` to surface any stragglers.
- [ ] Add l10n key `a11yWorkshopCreateCommunity` to **both** `app_en.arb` ("Create a new community") and `app_es.arb` ("Crear una nueva comunidad"), each with a `@a11yWorkshopCreateCommunity` metadata block describing the accessibility-label use. Re-run `npm run generate`.

**Validates:** Workshop trailing slot is the create-community button, announced via `IconAction`'s required `semanticsLabel` (which also supplies the tooltip on long-press). The Workshop tab no longer exposes a profile-menu entry — by design.

### Phase 4: Remove "+ New" entry from Workshop community carousel
**Goal:** Carousel only shows community entries (+ "Everything" when relevant). The "create new" action is fully owned by the floating header.
**Tasks:**
- [ ] [workshop_community_carousel.dart:122-132](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L122-L132): delete the `if (index == 0)` branch.
- [ ] Change `communityStart` (line 110) from `showEverything ? 2 : 1` to `showEverything ? 1 : 0`. Update the equivalent math in [_scrollSelectedIntoView](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L56-L85): `targetIndex = (showEverything ? 1 : 0) + i` for per-community entries, and revise the `selectedId == null` fallback per OQ6 (target the "Everything" entry when present; otherwise no scrolling needed because there's only one entry — but still write a `targetIndex = 0` branch so the caller's contract stays consistent).
- [ ] Delete the now-unused `_AddAvatar` widget class and the `_onAddCommunity` helper. Grep the codebase first to confirm no other call site references them.
- [ ] Delete the orphan l10n keys `workshopCarouselNew` and `a11yWorkshopCarouselAddCommunity` (with their `@key` blocks) from **both** `app_en.arb` and `app_es.arb`. Confirm zero remaining references via `grep -r workshopCarouselNew app/lib` and `grep -r a11yWorkshopCarouselAddCommunity app/lib`.
- [ ] Add a widget test for the carousel covering both states: single-community (no "Everything", one entry, auto-selected) and multi-community (Everything first, then communities, selection scrolls into view). This is the one carousel test that exercises real branching — index math is the actual logic that can regress.

**Validates:** Carousel has no orphan "+ New" entry; index math is correct in both single- and multi-community states.

### Phase 5: Verification
**Goal:** Confirm nothing regressed and the feature is reachable end-to-end on both platforms.
**Tasks:**
- [ ] `npm run test:app` clean.
- [ ] `npm run lint:dart:size` (file-size gate) clean.
- [ ] `npm run lint:dart:a11y` clean — the a11y allowlist is empty; any new violation must be fixed in the same change (per [docs/client/accessibility.md](../client/accessibility.md)).
- [ ] `npm run lint:dart:i18n` clean — catches any forgotten raw string for the new controls.
- [ ] `npm run lint:ephemeral-comments` clean — per CLAUDE.md, no phase markers in source comments.
- [ ] `flutter analyze` clean.
- [ ] Manual: open the app on iOS sim, switch through Feed → Workshop → Inbox. Tap avatar on Feed → opens `ProfileMenuModal`. Inbox header is unchanged. Tap create-community icon on Workshop → opens `CommunityCreationModal`. Confirm Workshop floating header has no avatar (trailing icon only) and the carousel no longer shows a "+ New" entry.
- [ ] Repeat on Android emulator.
- [ ] Accessibility sanity: VoiceOver / TalkBack announces "Open account menu" for the Feed avatar and "Create a new community" for the Workshop icon. Long-press on the Workshop icon shows the tooltip (`IconAction` defaults the tooltip to `semanticsLabel`).

**Validates:** The narrowed scope (Feed avatar only; Workshop trailing icon; no carousel "+") is fully reachable and every lint gate the CI workflow runs passes locally. Acceptance-criteria items 2 ("avatar on Inbox") and 5 ("how to reach profile menu from Workshop") were resolved during planning as out-of-scope / no-surface respectively — communicate that change in the PR description so the issue author can update the checkboxes on #1963.

## Related Issues

- **#1579 — Workshop tab** (docs/issues/1579-workshop-tab.md): defines the Workshop tab as a host workshop / community-curation surface. This issue extends that framing by making the Workshop header more on-theme (create community) and pushing account-level controls onto Feed/Inbox.
- **#1898 — Workshop redesign** (docs/issues/1898-workshop-redesign.md): the existing trailing avatar in Workshop came from this redesign. This issue reverses that placement choice based on new discoverability evidence.
- **#1895** — referenced in `_WorkshopAvatarButton`'s doc comment as the source of the ProfileMenuModal pattern. Pattern is preserved; only the location changes.
