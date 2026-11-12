# Issue #1919: Polish pass: nav restructure / inbox / workshop follow-ups

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1919
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Catch-all polish issue collecting follow-ups that surfaced after the four
nav-restructure issues (#1895 nav, #1896 feed search, #1897 inbox
filters, #1898 workshop redesign) landed. Each bullet is small on its
own; grouping them avoids a half-dozen micro-issues and lets them be
picked off opportunistically.

The work splits into four loosely-related buckets, each of which can
ship independently:

1. **UX polish** — bordered search inputs on focus, inconsistent floating
   nav coloration across surfaces, tight spacing above the inbox filter
   pills, bespoke pill highlight color, and a navigation-after-creation
   gap on the community creation flow.
2. **Inbox bulk-editing** — re-introduce the legacy Portfolio "My Items"
   multi-select / bulk-action affordance, scoped to the **My Stuff**
   filter only (the other three filters are engagement-driven and don't
   benefit from it). Note: this filter is `InboxFilter.myStuff` in
   code but is referred to as "Your Stuff" in
   [docs/client/inbox.md](../client/inbox.md) — same pill, two names;
   the user-visible label comes from `l10n.inboxFilterMyStuff`. The
   pill is its own RPC (`GetMyStuffItems` via `myStuffItemsProvider`),
   not a predicate over the engagement feed — selection state must
   bind to that separate list.
3. **Workshop tailoring** — nudges on the Workshop home are identical
   regardless of carousel selection; detail screens (acts / money /
   CO₂ / time) carry the same nudges and should instead show
   metric-specific recent contributors.
4. **Dead-code cleanup** — sweep the orphans left by the four parent
   PRs: the legacy `PortfolioScreen` shell (no callers), the unused
   `WorkshopActsDetailScreen`, sidebar widgets and l10n strings, and
   any predicate / filter helpers replaced by the per-pill RPC split in
   #1897.

## Context from Comments

The issue has **no comments** at the time of planning. Treat each
bullet's text in the description as authoritative; the bullets are
already scoped narrowly enough not to need further clarification. The
one bullet flagged as "may be a pre-existing bug" — community
creation not navigating to the new community on Feed — explicitly
asks for a `main` comparison before treating it as a regression.

## Potential Solutions

### Option A: One large catch-all PR

- **Approach:** Land all four buckets in a single branch / PR. Matches
  the way the issue is filed (one tracker for the whole sweep).
- **Pros:** Single review pass; the related cleanup naturally lands
  with the UX changes that produced the orphans.
- **Cons:** Large diff is noisier to review; a regression in one bucket
  blocks the others; the workshop "recent items" work alone needs new
  server-side aggregation and shouldn't gate the trivial spacing fix.
- **Effort:** Large (one big merge).

### Option B: One PR per bucket (recommended)

- **Approach:** Four PRs, one per bucket above. Each is independently
  shippable; the dead-code sweep ships last so the other three don't
  re-introduce now-orphan references.
- **Pros:** Each PR is small, reviewable, and revertible; the polish
  bucket can ship in a day while the workshop "recent items" change
  takes its own week; the dead-code sweep ships against a stable
  reference point after the others have landed.
- **Cons:** Coordination overhead of four PRs; some shared helpers
  (e.g., a reusable focus-less search input) may bounce across PRs.
- **Effort:** Medium across four medium PRs.

### Option C: Opportunistic — leave the issue open, pick bullets off

- **Approach:** Treat #1919 as a permanent grab-bag and let bullets land
  inside whatever feature PR is open at the time.
- **Pros:** Zero coordination overhead.
- **Cons:** Bullets rot; the dead-code sweep never happens because
  there's never a "right" PR for it; the issue stays open forever.
- **Effort:** Variable, but the cost is the bullets we never ship.

## Recommendation

**Option B** — one PR per bucket — is the right shape. The four
buckets have different complexity ceilings (the UX polish is hours;
the workshop "recent items" work is a week) and they don't share a
mechanical dependency. The dead-code sweep needs to land last so it
can clean up against the merged state of the other three.

The UX polish bucket should ship first because it's the cheapest and
the user-visible payoff per LoC is highest. Workshop tailoring goes
second because it requires a server-side change (new "recent
contributors" payload per metric) and we want that exercised on
production before the dead-code sweep removes the last fallbacks.
Inbox bulk-edit goes third — it's the heaviest UI bucket but the
mechanical work largely already exists (lifted from `PortfolioScreen`
/ `PortfolioItemsScreen`). Dead-code cleanup goes last.

## Decisions

The plan resolves the open-question set from the original draft into
the following decisions (set after a 2026-05-13 review pass with the
issue author):

- **Workshop "identical nudges" bug — confirm-then-test.** The
  hypothesis is that the user's current account has shared the same
  items across every community, so a single-community brief and the
  "Everything" brief converge on the same ranked nudges by content,
  not by code. Phase 2 starts by **confirming**: build (or pick) a
  multi-community test fixture where the per-community shares
  diverge, and assert the brief actually returns distinct nudge sets
  per scope. If the assertion passes, the bug is data-driven (no
  code change), close the bullet and ship a regression test. If it
  fails, file a server-side ranker issue with the failing fixture
  attached.

- **Recent contributors — reuse `RecentActivity`.** Extend each of
  the four detail RPCs (`GetCommunityActsDetail`,
  `GetCommunityMoneyDetail`, `GetCommunityCo2Detail`,
  `GetCommunityTimeDetail`) with a `repeated RecentActivity
  recent_items` field. Do **not** introduce a parallel message type;
  the existing `RecentActivity` at
  [impact_service.proto:301](../../proto/ripls/api/impact_service.proto#L301)
  already has `label`, `person_name`, `value`, `media_id`.

- **Post-create navigation — fix in viewmodel.** Move the post-create
  sequence (reload communities with `selectId`, invalidate feed
  cache, `navigateToTab(0)`) into a viewmodel and have both call
  sites (`empty_content_state.dart`,
  `workshop_community_carousel.dart`) trigger that viewmodel. The
  most likely root cause is the carousel call site discards the
  return value, so the new community ID never propagates to Feed.

- **Inbox pill highlight — `AppColors.primary(context)`.** Selected
  pill `bg = AppColors.primary(context)`, `fg =
  AppColors.background(context)` (coral fill, cream text). Matches
  the rest of the Discover / portfolio surfaces.

- **Search-input border-on-focus — explicit `enabledBorder` and
  `focusedBorder`.** Material's `border:` is the fallback only; the
  focused underline comes from `focusedBorder` unless explicitly
  overridden. Mirror the pattern at
  [portfolio_screen.dart:110-112](../../app/lib/presentation/screens/portfolio/portfolio_screen.dart#L110-L112).

- **`WorkshopActsDetailScreen` — delete in Phase 4.** No callers
  after the #1898 ticker swap. Resurrect from git history if a future
  ticker slot needs it; carrying dead code is a per-build maintenance
  tax.

## Implementation Plan

### Phase 1: UX polish bucket

**Goal:** Land the four cheap, user-visible fixes from the "UX polish"
section in a single PR.

**Tasks:**
- [ ] Make the inbox search pill borderless on focus
      ([inbox_search_pill.dart](../../app/lib/presentation/widgets/portfolio/inbox_search_pill.dart)):
      add explicit `enabledBorder: InputBorder.none` and
      `focusedBorder: InputBorder.none` to the
      `InputDecoration` at line 70. (Material's `border:` is the
      fallback only; the focused underline is drawn from
      `focusedBorder` unless overridden.)
- [ ] Same fix on the feed search active state
      ([search_nearby_pill.dart:284](../../app/lib/presentation/widgets/search/search_nearby_pill.dart#L284)).
- [ ] Audit floating nav background consistency. The shared
      [FloatingHeader](../../app/lib/presentation/widgets/floating_header.dart)
      uses `AppColors.cardBackground(context).withAlpha(217)` — so any
      cross-surface difference is from the underlying scaffold
      background showing through the 85% alpha. Confirm all four hosting
      surfaces (Feed / Inbox / Workshop / Profile-menu modal backdrop)
      use `AppColors.background(context)` as the scaffold color; align
      Profile (which currently uses different backgrounds in some
      cases — see `manage_memberships_screen.dart`). All scaffold
      backgrounds must use the theme-aware
      `AppColors.background(context)` helper per
      [docs/client/architecture.md](../client/architecture.md) "Colors
      and Theming" — never hardcoded `Colors.white` / `Color(0xFF...)`.
- [ ] Add ~12px gap above
      [InboxFilterRow](../../app/lib/presentation/widgets/portfolio/inbox_filter_row.dart)
      ([portfolio_inbox_screen.dart:181](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L181)
      — the `SizedBox(height: 4)` between the floating header and the
      pill row). If the increased spacing changes the overlay's total
      height, update
      `_filterRowOverlayHeight`
      ([portfolio_inbox_screen.dart:53](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L53))
      to match so the scroll-body inset still clears the chips.
- [ ] Switch inbox pill selected colors to the standard highlight
      ([inbox_filter_row.dart:101-106](../../app/lib/presentation/widgets/portfolio/inbox_filter_row.dart#L101-L106)):
      `bg = AppColors.primary(context)`,
      `fg = AppColors.background(context)`. The pill already uses the
      `Toggle` accessibility primitive (`selected:` propagates to the
      screen reader) so no a11y label changes are needed.
- [ ] Verify against `main` whether community creation navigates to
      the new community in the Feed. If it doesn't on `main`, file as
      a separate bug. If it does on `main` but regresses on this
      branch, fix `_onCreateCommunityTap` in
      [empty_content_state.dart:33-40](../../app/lib/presentation/widgets/empty_content_state.dart#L33-L40)
      and `_onAddCommunity` in
      [workshop_community_carousel.dart:52-54](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart#L52-L54)
      to mirror `gen_community_view_model`'s post-create sequence:
      `await selectedCommunityProvider.reloadCommunities(selectId: id)`
      before `navigateToTab(0)`, so the new community ID is a valid
      selection by the time Feed re-reads
      `selectedCommunityProvider.communities`. The post-create
      navigation work belongs in
      [gen_community_view_model.dart](../../app/lib/presentation/viewmodels/gen_community_view_model.dart)
      (or a dedicated `community_creation_view_model`) — not inside
      the widget — to keep with the
      "ViewModels own business logic" rule from
      [docs/client/architecture.md](../client/architecture.md). The
      callsites in `empty_content_state.dart` /
      `workshop_community_carousel.dart` should just trigger the
      viewmodel action.
- [ ] **Align the Feed-search results sheet background with the
      floating search bar.** Screenshot from 2026-05-13 shows the
      floating search pill rendered in cream
      (`AppColors.cardBackground(context).withAlpha(217)` blended over
      the map) while the results sheet underneath
      ([discover_bottom_sheet.dart:128](../../app/lib/presentation/widgets/discover/discover_bottom_sheet.dart#L128))
      is solid `AppColors.cardBackground(context)` (pure white in
      light mode). The two surfaces should read as one continuous
      panel during search. Pick one source of truth — either drop the
      header's alpha so the pill matches the sheet, or thread the
      sheet's color to match the alpha-blended header. Recommendation:
      drop the alpha during the active-search state so the pill renders
      solid `cardBackground`, matching the sheet; the alpha-blended
      look is only the right read on the idle Feed where the map shows
      through. The active-search overlay should pass an explicit solid
      background to the
      [_PillFrame](../../app/lib/presentation/widgets/search/search_nearby_pill.dart#L148)
      (a new `backgroundColor:` parameter that overrides the default
      alpha-217 surface). Use `AppColors.cardBackground(context)`, not
      hardcoded values.
- [ ] **Stop truncating quick-search chip labels.** Same screenshot
      shows "Help needed n…" and "People in your …" being clipped in
      [_QuickSearchGrid](../../app/lib/presentation/widgets/search/search_nearby_pill.dart#L371)
      — the 2-column fixed grid (`(constraints.maxWidth - 8) / 2`)
      doesn't leave enough room for the longer labels. Replace the
      strict 2-column `Row`/`SizedBox` layout with a `Wrap` (or a
      `Flex` with `IntrinsicWidth` cells) so each chip sizes to its
      content and longer labels reflow to their own row when needed.
      Do **not** shorten the ARB strings to fit a fixed grid — Spanish
      translations would still overflow. Both ARB files
      (`app_en.arb`, `app_es.arb`) keep their existing keys
      unchanged.

**Validates:** The four user-visible defects flagged in the issue's
"UX polish" section are gone; floating nav looks consistent across
all four primary surfaces; community creation lands on Feed
scoped to the new community.

### Phase 2: Workshop tailoring

**Goal:** Make workshop nudges actually reflect the carousel selection,
and replace nudges on detail screens with metric-specific recent
contributors.

**Tasks:**
- [ ] **Confirm the "identical nudges" perception, then lock the
      behavior in with a regression test.** Working hypothesis (per
      issue author): the dev account has shared the same items into
      every community, so a single-community brief and the
      "Everything" brief converge by content, not because the ranker
      is broken. Steps:
  - Build a multi-community storage fixture in
    [server/services/workshop/](../../server/services/workshop/) (or
    extend an existing one) where community A has a distinct set of
    completed events / transfers / pitch-ins from community B —
    enough that the cascade should produce different ranked nudges.
    Use `storage.SetupTestStorage(t)` per CLAUDE.md so the test
    isolates against the pgvector testcontainer.
  - Add a unit test that calls `GetWorkshopBrief` three times — once
    scoped to community A, once to community B, once across both —
    and asserts: the per-community responses each return a non-empty
    `ctaRows`, and **the set of context IDs in each per-community
    response is distinct from the other community's**. This is the
    regression lock: if a future ranker change collapses scopes back
    to a single ranking, the test fails.
  - Run the test against `main` with the user's hypothesis fixture
    (same items shared into every community). It should still pass:
    when content is genuinely identical across scopes, returning
    overlapping nudges is correct, not a bug.
  - On the client, mirror with a `ProviderContainer` test in
    [app/test/presentation/viewmodels/workshop_view_model_test.dart](../../app/test/presentation/viewmodels/)
    that overrides `WorkshopRepository.getBrief` to return distinct
    payloads per `communityIds` and asserts the viewmodel refetches
    on `workshopCommunityProvider` change. This guards the client
    side of the perception bug — that the viewmodel actually wires
    the scope through to the repo on selection change.
  - If both tests pass, close this bullet in the PR description with
    "confirmed perception-only; data-driven, not a code bug; locked
    in by tests T1 and T2." If either fails, the failing test is the
    bug report — file the server-side or client-side fix as a
    separate issue and link from #1919.
  - **No production debug log** is needed if the tests confirm the
    expected behavior. If the tests reveal a real bug and you do need
    to instrument production temporarily, use
    `logging.LoggerWithContext(ctx)` with `operation`, `user_id`,
    `community_count`, and the returned nudge IDs — never
    `fmt.Printf` / `log.Printf` (see
    [docs/server/observability.md](../server/observability.md)).
- [ ] Extend the four detail-screen RPCs (`GetCommunityActsDetail`,
      `GetCommunityMoneyDetail`, `GetCommunityCo2Detail`,
      `GetCommunityTimeDetail`) with a `repeated RecentActivity
      recent_items = N` field — reuse the existing `RecentActivity`
      message at
      [impact_service.proto:301](../../proto/ripls/api/impact_service.proto#L301).
      Per
      [CLAUDE.md "Proto Comments"](../../CLAUDE.md), comments on the
      new field describe **what** is returned (e.g. "Most recent items
      that contributed to this metric, newest first.") — never
      reference vendors, internal pipeline stages, or server-side
      mechanics.
- [ ] Server-side: build the per-metric recent-items aggregator. For
      money/CO₂ savings, walk completed transfers ranked by
      `completed_at DESC` over the scope's communities and limit to
      the top N (suggest N=8). For time, walk past events. For acts,
      walk the union and pick the most-recent N regardless of category.
      Per
      [CLAUDE.md "SQL Efficiency"](../../CLAUDE.md): one batched
      query per metric (no per-community loops), push the `LIMIT N`
      and the community-membership filter into SQL, and add an
      `AssertMaxQueries(t, ctx, 1, ...)` regression test for each
      detail RPC.
- [ ] Add structured logging to the new aggregator path: at the
      start log `operation="GetCommunityMoneyDetail.recentItems"` (or
      the metric-appropriate name) with `user_id`, `community_count`,
      and `limit`; at the end log `result_count` and `duration_ms`.
      Use `logger.WarnContext` (not "warn and continue") only if the
      empty-result path is genuinely expected; otherwise propagate
      errors via `connecterr.Internal(ctx, op, err, kv...)`.
- [ ] Drop the `momentumRows` slot from
      [workshop_acts_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_acts_detail_screen.dart),
      [workshop_money_detail_screen.dart:282-296](../../app/lib/presentation/screens/workshop/workshop_money_detail_screen.dart#L282-L296),
      [workshop_co2_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_co2_detail_screen.dart),
      [workshop_time_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart);
      replace each `WorkshopPostcard`-based sliver with a
      `RecentItemsList` reading from the new `recent_items` field on
      the detail response. The detail-screen viewmodels (not the
      widget) call `ImpactRepository` for the detail response — the
      existing cache key on each detail RPC continues to cover the
      whole response (including the new `recent_items` field) since
      [client caching](../client/caching.md) stores the response
      object verbatim under a per-scope cache key. No new cache
      namespace is needed.
- [ ] Add a shared `RecentItemsList` widget under
      [widgets/workshop/](../../app/lib/presentation/widgets/workshop/)
      that takes a `List<RecentActivity>` and renders one row per
      entry: avatar from `media_id` (via
      `CachedMediaImage(semanticsLabel: 'Photo of ${item.label}')` —
      required by
      [docs/client/accessibility.md](../client/accessibility.md)),
      label, person name, value. Wrap each row in
      `Tappable(semanticsLabel: ...)` (not `GestureDetector`) and
      route taps via
      `NavigationHelpers.pushScreen(... useRootNavigator: true,
      routeName: 'gear_detail' /* or experience/request */)` — never
      raw `Navigator.push(MaterialPageRoute(...))`. New a11y label
      strings go in `app_en.arb` + `app_es.arb` under the `a11y`
      prefix (and section-header strings under e.g.
      `workshopMoneyRecentItemsHeader`).

**Validates:** The Workshop home reflects the carousel selection;
each detail screen shows the most-recent contributors instead of the
generic nudge postcards.

### Phase 3: Inbox bulk-edit on "My Stuff"

**Goal:** Re-introduce the multi-select / bulk-action UX from the
legacy `PortfolioScreen`, scoped to the My Stuff filter
(`InboxFilter.myStuff`).

**Tasks:**
- [ ] Lift the selection state and bulk-action business logic out of
      [profile_gear_view_model](../../app/lib/presentation/viewmodels/profile_gear_view_model.dart)
      into a dedicated **ViewModel** (a `Notifier` with a Freezed
      state) at
      `app/lib/presentation/viewmodels/inbox_selection_view_model.dart`.
      Per
      [docs/client/architecture.md](../client/architecture.md): use
      `NotifierProvider<InboxSelectionNotifier,
      InboxSelectionState>`; state is Freezed
      (`isSelectionMode`, `selectedGearIds`,
      `selectedRequestIds`, `selectedExperienceIds`,
      `totalSelectedCount` as a getter). Do not place it under
      `services/providers/` — that path is for repositories /
      services. Do not put bulk action logic in `core/utils/` either —
      utilities are for SDK lifecycle / pure functions, not stateful
      business logic. The new viewmodel owns the three bulk-action
      methods (`updateLocationFor`, `shareInto`, `delete`); the
      screen only triggers them.
- [ ] In
      [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart),
      gate an "edit" affordance on `inboxFilterProvider == myStuff`.
      When active, render a selection-mode floating header (mirroring
      [portfolio_screen.dart:69-99](../../app/lib/presentation/screens/portfolio/portfolio_screen.dart#L69-L99))
      with a close-button leading, an "N selected" title, and trailing
      icons for `location`, `share`, and `delete`. Use the
      `IconAction` primitive (not raw `IconButton`) per
      [docs/client/accessibility.md](../client/accessibility.md);
      every action needs `semanticsLabel` and `tooltip` pulled from
      `context.l10n.a11y*` (new keys under
      `a11yInboxSelectionLocation`,
      `a11yInboxSelectionShare`,
      `a11yInboxSelectionDelete`,
      `a11yInboxSelectionExit`).
- [ ] Adapt
      [inbox_row.dart](../../app/lib/presentation/widgets/portfolio/inbox_row.dart)
      to support a selection-mode tap that toggles selection (and
      shows a check overlay) instead of pushing the content screen.
      Wrap the tap target with the `Toggle` primitive (per the
      a11y "selectable state" matrix in
      [accessibility.md](../client/accessibility.md)) so screen
      readers announce the selected state. In non-selection mode,
      behavior is unchanged. The row needs to skip selection for
      `is_library_item` rows that aren't owned (per
      [docs/client/inbox.md](../client/inbox.md), My Stuff also
      includes inline community-chat rows — those aren't bulk-actionable
      and should not show a checkbox).
- [ ] Wire the three bulk-action handlers (location / share / delete)
      in the new `InboxSelectionNotifier`. The notifier calls
      `gearRepositoryProvider` / `requestRepositoryProvider` /
      `experienceRepositoryProvider` (the legacy
      [portfolio_screen.dart:260-286](../../app/lib/presentation/screens/portfolio/portfolio_screen.dart#L260-L286)
      already calls these). After mutation, invalidate the My Stuff
      cache via
      [PortfolioRepository.refreshInboxFeed()](../../app/lib/data/repositories/portfolio_repository.dart)
      — `refreshInboxFeed` already invalidates the My Stuff cache per
      [docs/client/inbox.md](../client/inbox.md). Do not call
      services directly from the viewmodel
      (per [docs/client/caching.md](../client/caching.md)) — go
      through repositories so caching invariants are preserved.
- [ ] Tests. Per
      [docs/client/testing.md](../client/testing.md), add a
      ViewModel-first test file
      `app/test/presentation/viewmodels/inbox_selection_view_model_test.dart`
      using `ProviderContainer` overrides and `@GenerateMocks` for
      `GearRepository` / `RequestRepository` /
      `ExperienceRepository` / `PortfolioRepository`. Cover:
      toggling selection mode, adding / removing IDs, the three
      bulk-action paths (success / partial failure), and disposal
      safety. Run via `flutter test --reporter expanded` per
      [CLAUDE.md "Testing"](../../CLAUDE.md). Regenerate the
      Mockito mocks with `flutter pub run build_runner build`.

**Validates:** When **My Stuff** is the active filter, the user can
tap an edit affordance, multi-select rows, and apply
location/share/delete to the selection — matching the legacy
Portfolio experience. The new viewmodel's business logic is covered
by unit tests independent of the widget tree.

### Phase 4: Dead-code sweep

**Goal:** Remove code orphaned by #1895 / #1896 / #1897 / #1898 once
the previous three phases have stabilized.

**Tasks:**
- [ ] Delete
      [portfolio_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_screen.dart)
      (no remaining callers after Phase 3 lifts the bulk-edit
      helpers). Note that this also removes the last
      architecture violation where bulk-action code lived in a widget
      state class instead of a ViewModel (per
      [docs/client/architecture.md](../client/architecture.md)).
- [ ] Delete
      [workshop_acts_detail_screen.dart](../../app/lib/presentation/screens/workshop/workshop_acts_detail_screen.dart)
      (no callers after the #1898 ticker swap; the file's own header
      comment flags it as cleanup-eligible).
- [ ] Grep-and-delete sidebar widget files and orphaned l10n strings
      under
      [l10n/app_*.arb](../../app/lib/l10n/) (`sidebarAllCircles`,
      `sidebarCommunitiesHeader`, etc.). Per
      [docs/client/i18n.md](../client/i18n.md), every locale (en +
      es) must drop the key together — leaving a string in only one
      ARB file leaves a half-localized key. One key
      (`sidebarCommunitiesHeader`) is still referenced from
      [settings_hub_screen.dart:145](../../app/lib/presentation/screens/settings/settings_hub_screen.dart#L145)
      — either keep it (renaming would churn translations), or
      rename to e.g. `settingsCommunitiesHeader` and re-translate.
- [ ] Audit predicate / filter helpers under
      [presentation/viewmodels/inbox_filter*.dart](../../app/lib/presentation/viewmodels/)
      for branches superseded by the per-pill RPC split (#1897).
      Anything that's only referenced from a test now should be
      deleted along with its test. The viewmodel-first testing
      convention from
      [docs/client/testing.md](../client/testing.md) means orphan
      tests should not be retained as dead weight.
- [ ] Audit Workshop pre-redesign layouts: anything in
      [widgets/workshop/](../../app/lib/presentation/widgets/workshop/)
      that's not in the post-#1898 render tree
      (`workshop_home_layout`, `workshop_community_carousel`,
      `workshop_lever_dispatcher` — plus the postcard / ticker /
      section header widgets they pull in).
- [ ] Run `npm run lint:dart:size`, `npm run lint:dart:a11y`,
      `npm run lint:dart:i18n`, and `npm run build` to make sure the
      deletions don't trip the 1000-line gate, the a11y allowlist
      ratchet (per
      [docs/client/accessibility.md](../client/accessibility.md)),
      the i18n hardcoded-string lint (per
      [docs/client/i18n.md](../client/i18n.md)), or break any
      lingering type references.

**Validates:** `git grep` for the names of the deleted classes returns
zero hits; the test suite is still green; bundle size shrinks by the
removed files' line count.

## Related Issues

- #1895 — Nav restructure (parent of the polish surface area).
- #1896 — Feed search (source of the search-pill border bullet).
- #1897 — Inbox filters (source of the spacing / highlight color
  bullets).
- #1898 — Workshop redesign (source of the carousel-scoping and
  detail-screen bullets).

---

## Audit Notes (`/ripls-audit` pass)

The following changes were applied after auditing against
`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/client/inbox.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`:

- **MVVM compliance (Phase 3).** The original plan suggested putting
  selection state under `services/providers/` or extracting bulk-action
  handlers under `core/utils/`. Both violate the
  architecture rule that business logic lives in a Notifier (and that
  utilities are for SDK lifecycle, not stateful logic). Fixed to spec
  a dedicated `InboxSelectionNotifier` / `InboxSelectionState` under
  `presentation/viewmodels/`. The same fix removes the existing
  violation in `portfolio_screen.dart` where bulk-action helpers call
  repositories directly from the widget state — Phase 4 calls this out.

- **Post-create navigation (Phase 1).** The original plan would have
  fixed `_onCreateCommunityTap` / `_onAddCommunity` in the widget
  layer. Moved the fix into a viewmodel and added a sequencing note
  (`await reloadCommunities(selectId:)` before `navigateToTab(0)`)
  to address the root cause hypothesis.

- **Caching (Phase 2).** Clarified that the new `recent_items` field
  on the four detail responses is covered by the existing per-scope
  cache key (one response per (`metric`, `community_ids`) tuple — no
  new cache namespace needed). Confirmed
  `refreshInboxFeed()` already invalidates the My Stuff cache, so
  Phase 3's bulk-action handlers can reuse the existing path.

- **Accessibility (Phases 2 + 3).** Added explicit references to the
  primitives: `Tappable` / `Toggle` / `IconAction` / `CachedMediaImage`
  with `semanticsLabel`. Called out the `a11y*` ARB key prefix for
  every new tap target and the requirement to wire `tooltip:` on
  every new icon button. The `Toggle` primitive on
  `inbox_filter_row.dart` is already correct — confirmed no change is
  needed beyond the color swap.

- **i18n (Phases 2 + 4).** Required new strings to land in both
  `app_en.arb` and `app_es.arb`. Phase 4 was updated to drop strings
  from both ARBs together (not just one) and to be explicit about the
  `sidebarCommunitiesHeader` decision (keep vs. rename).

- **Navigation (Phase 2).** Required the recent-items row taps to use
  `NavigationHelpers.pushScreen(... useRootNavigator: true)` per the
  architecture doc — never raw `Navigator.push(MaterialPageRoute(...))`.

- **Server observability (Phase 2).** Required the diagnostic and
  aggregator log lines to use `logging.LoggerWithContext(ctx)` with
  the standard field names (`operation`, `user_id`, `community_count`,
  `duration_ms`) and explicitly banned `fmt.Printf` / `log.Printf` per
  `docs/server/observability.md`. Required error propagation via
  `connecterr.Internal(ctx, op, err, kv...)` (no "warn and continue"
  short-circuits) per `CLAUDE.md`.

- **SQL efficiency (Phase 2).** Required the new recent-items
  aggregator to use one batched query per metric — no per-community
  loops, `LIMIT N` pushed into SQL — and to add an
  `AssertMaxQueries(t, ctx, 1, ...)` regression test, per the
  CLAUDE.md SQL Efficiency rules.

- **Testing (Phase 3).** Made the testing requirement explicit:
  ViewModel-first tests using `ProviderContainer` + Mockito-generated
  mocks of the four repositories, covering selection-mode toggle,
  bulk-action paths, and disposal safety. Specified `flutter test
  --reporter expanded` per the CLAUDE.md testing rule.

- **Filter terminology.** Added a note clarifying that
  `InboxFilter.myStuff` in code maps to "Your Stuff" in
  `docs/client/inbox.md` — same pill, two names — so the next reader
  isn't tripped by the mismatch. Also called out that My Stuff is
  served by its own RPC (`GetMyStuffItems`), not a predicate, so
  selection must bind to that list. Included a guard against
  selecting the inline community-chat rows that appear in the My
  Stuff list under inbox.md's "Your Stuff — the owned-item library"
  section.

- **Proto comments (Phase 2).** Reminded that the new field's proto
  comment follows the vendor/transport/server-mechanics ban from
  `CLAUDE.md` — describe what the field means, not how it's produced.

- **Lint gates (Phase 4).** Added `npm run lint:dart:a11y` and
  `npm run lint:dart:i18n` to the verification step alongside the
  existing `lint:dart:size` and `build`, so the dead-code sweep
  doesn't accidentally introduce uncovered violations.

- **Overlay height (Phase 1).** When increasing the spacer above the
  filter pill row, also bump
  `_filterRowOverlayHeight`
  ([portfolio_inbox_screen.dart:53](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L53))
  so the scroll-body inset still clears the chips — otherwise the
  first inbox row slides under the pill row.

### Second-pass updates (2026-05-13, after issue-author review)

- **Open Questions → Decisions.** Folded all six open questions into
  the resolved "Decisions" section. The remaining work is now
  unambiguous; no further input needed from the author before Phase 1
  ships.

- **Workshop nudges (Phase 2).** Replaced the "add a debug log,
  inspect, decide" task with a confirm-then-lock test pair: a
  server-side fixture-driven test asserting distinct nudges for
  distinct community content, and a client viewmodel test asserting
  the viewmodel refetches on `workshopCommunityProvider` change. The
  user's hypothesis (same shares across communities → same nudges) is
  exactly the data shape that should produce overlap, so the tests
  validate the working theory rather than chase a phantom bug.

- **Search results sheet color mismatch (Phase 1, new bullet).**
  Added a task to align the floating Feed-search pill's background
  with the results sheet so they read as one continuous panel during
  search. Specified `AppColors.cardBackground(context)` (theme-aware,
  not hardcoded) per the architecture doc's "Colors and Theming"
  rule.

- **Quick-search chip truncation (Phase 1, new bullet).** Added a
  task to replace the strict 2-column quick-search grid with a `Wrap`
  / intrinsic-width layout so longer labels — including Spanish
  translations — no longer get clipped. Explicitly forbade
  shortening ARB strings, since that would only mask the layout bug.
