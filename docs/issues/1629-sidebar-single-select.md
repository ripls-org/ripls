# Issue #1629: Sidebar: replace multi-select with single-select + 'All Circles' aggregate

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1629
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit Notes (2026-04-30)

This plan was reviewed against the project's client architecture, i18n,
accessibility, testing, and caching reference docs. Adjustments applied:

- **Notifier API consistency.** Original plan renamed `selectCommunity` to
  take a `String communityId`, breaking parity with `switchTo(CommunityItem)`.
  Updated Phase 1 to keep `selectCommunity(CommunityItem)` so both APIs match
  and existing call sites that already hold a `CommunityItem` don't have to
  reach for `.id`.
- **Accessibility primitive choice.** Both the new All Circles row and the
  existing community rows are selectable-state items — they should keep using
  `Toggle` (per `docs/client/accessibility.md`, the right primitive for
  "tap that toggles a selectable state"), not `Tappable`. Phase 2 was hedging
  ("`Tappable` (or keep `Toggle` if its `selected` prop is just visual)") and
  is now decisive: keep `Toggle`, set `selected:` to express the active row.
  This also satisfies the "state first when state matters" rule — don't embed
  "selected"/"not selected" into the label.
- **`accessibleDuration` preserved.** The existing `_buildCommunityItem`
  wraps its `AnimatedOpacity` in `accessibleDuration(context, ...)`. Phase 2
  now calls this out explicitly so the rewrite doesn't accidentally drop
  reduce-motion support.
- **i18n metadata.** Original plan listed the ARB keys but didn't mention
  `@key` metadata blocks. Per `docs/client/i18n.md`, ICU placeholder strings
  must declare the placeholder type in `@key`. Phase 2 now spells this out
  for `sidebarAllCirclesSubtitle` and is explicit that both `app_en.arb` and
  `app_es.arb` get the new keys (and lose the old `sidebarClearAll` /
  `sidebarSelectAll`).
- **Test layering.** Original plan led with a sidebar widget test. Per
  `docs/client/testing.md` ("test business logic, not Flutter widgets"), the
  primary coverage is `ProviderContainer`-driven Notifier tests in
  `selected_community_test.dart`. Widget tests are supplementary and reserved
  for things only a widget tree can verify (visual state, tap target wiring).
  Phase 4 now leads with the Notifier tests and treats the widget test as
  optional secondary coverage.
- **Async safety.** `SelectedCommunityNotifier` is a non-autoDispose
  `Notifier` and persistence calls (`SharedPreferences`) are awaited inside
  notifier methods. The new flows don't change that, so we don't need
  `SafeNotifierMixin` or `ref.mounted` checks beyond what's already there.
  Called out in Phase 1.
- **Server boundary unchanged.** Confirmed: no RPC, proto, or server
  observability impact. The existing `repeated string community_ids` fields
  on `GetFeedRequest` and `SearchRequest` accept a single-element list with
  no server change, and no new log fields are introduced. Server reference
  docs (`docs/server/architecture.md`, `docs/server/observability.md`) are
  out of scope for this issue.
- **Caching layer unchanged.** This issue does not touch the
  Stash/Repository caching layer (`docs/client/caching.md`). Sidebar state
  is a Notifier + SharedPreferences concern, not a `CacheService` concern.

## Problem Statement

The sidebar currently uses a multi-select model: every community in the user's
list has an independent on/off toggle, the feed and other screens show the union
of content across all enabled communities, and a "Clear all"/"Select all"
button at the top of the section flips every toggle at once. This produces two
distinct semantics for "see everything":

1. *All toggles on* — feed/discover/search/metrics aggregate across every
   community.
2. *All toggles off* — "Portfolio mode": feed and discover render empty states
   that prompt the user to enable a community.

These two states look identical in the sidebar header (no toggle visibly
"selected") but produce opposite content states (everything vs. nothing). The
issue asks us to replace the model with single-select: one row at a time can be
active, with a new explicit **All Circles** entry pinned at the top that
represents the aggregate view. The "Clear all" affordance disappears because it
no longer has a meaning that differs from selecting **All Circles**.

The request is unambiguous on the user-visible behavior but leaves the
interaction model for a few adjacent flows implicit (deep-link routing,
post-create navigation, what happens when a community is deleted, persistence
of the **All Circles** choice). Those are flagged in Open Questions below.

**Terminology note:** the issue body uses **"Circles"** (the user-facing term).
The code identifier remains `Community` / `community_id` everywhere. Per the
project's "UI Terminology" rule, code stays `Community`; user-visible strings
say **Circles**. The new sidebar entry is **"All Circles"** in the UI; in code
it is whatever the most natural identifier is (`allCircles` / a sentinel
`null` selection).

## Context from Comments

The issue has no comments. The issue author (the project owner) wrote the
description and filed it the same session this plan was requested, so the
description is the authoritative spec. No prior discussion to reconcile.

This means the Open Questions section below is doing more work than it
otherwise would — anything not in the issue body is genuinely undecided, not
just undocumented.

## Potential Solutions

### Option A: Sentinel `null` for All Circles, keep `Set<String>` shape

- **Approach:** Keep `SelectedCommunityState` and `enabledCommunityIds: Set<String>`,
  but enforce single-select at the notifier level. Define the **All Circles**
  state as "the set contains every community in `communities`" (i.e. the
  current "all enabled" semantics, just reached via a single tap on the new
  All Circles row). Tapping a single community replaces the set with that one
  ID. Remove `toggleCommunity`, `enableAll`, `disableAll`, and the "Clear all"
  UI.
- **Pros:** Smallest blast radius. Every consumer
  (`feed_screen.dart`, `discover_screen.dart`, `search_view_model.dart`,
  `portfolio_metrics_view_model.dart`, `portfolio_inbox_view_model.dart`)
  already reads `enabledCommunityIdList` and passes it as a list to RPCs that
  accept `repeated string community_ids`. No call-site changes needed beyond
  the notifier and the sidebar widget.
- **Cons:** The data shape lies a little — the field is still a `Set` even
  though invariants now require it to be either empty, a singleton, or
  "everything". Future readers may not realize single-select is enforced and
  reintroduce multi-select bugs. Requires a runtime invariant check or a
  comment to keep the contract clear.
- **Effort:** small.

### Option B: Refactor state to `String? selectedCommunityId` with explicit "all" sentinel

- **Approach:** Change `SelectedCommunityState` to carry a single
  `String? selectedCommunityId` (null = All Circles, non-null = that specific
  community). Derive `enabledCommunityIdList` as `null → all IDs`,
  `id → [id]`. Update the notifier API: `selectAllCircles()`, `selectCommunity(id)`,
  `switchTo(community)` becomes a thin wrapper. Remove `toggleCommunity`,
  `enableAll`, `disableAll`, `selectCommunity` (additive variant).
- **Pros:** The data model expresses the invariant directly. Future readers
  cannot accidentally enable two communities — the type system forbids it.
  Cleaner semantics for callers that want to know "is the user viewing one
  specific community right now?" (a question that `enabledCommunityIds.length
  == 1` answers awkwardly today).
- **Cons:** Touches every read site. `enabledCommunityIdList` getter can
  preserve compatibility for RPC call sites, but viewmodels that branch on
  `enabledCommunityIds.isEmpty` (Portfolio mode rendering in `feed_screen.dart`
  and `discover_screen.dart`) need to be re-thought, because the empty case no
  longer exists in the new model. Also touches the persistence layer
  (`SharedPreferences` key shape changes from `List<String>` to a single
  `String?`).
- **Effort:** medium.

### Option C: Hybrid — keep `Set<String>` shape, derive a `selectedMode` enum

- **Approach:** Keep `enabledCommunityIds: Set<String>` for read-site
  compatibility, but add a derived `SelectedMode mode` getter
  (`{allCircles, single, none}`) and enforce in the notifier that `mode != none`
  (since "Clear all" is gone, `none` should never happen except briefly during
  sign-in). The sidebar reads `mode` to highlight the correct row.
- **Pros:** Same call-site compatibility as Option A, plus a clean way for the
  sidebar to know which row to highlight without reasoning about set sizes.
- **Cons:** Three names for two states (the underlying set, the mode getter,
  the UI). More moving parts than Option A without the type-level guarantees
  of Option B.
- **Effort:** small-to-medium.

## Recommendation

**Option B** — refactor to `String? selectedCommunityId`.

The current model's bug surface is real: issue #1547 (referenced in the
existing tests at
[selected_community_test.dart:215-236](app/test/services/selected_community_test.dart#L215-L236))
is a regression where `selectedCommunity` returned a stale community because
the multi-select set still held a deleted ID. That class of bug exists because
the data shape allows states the UI never wants. Single-select is a chance to
delete that bug class, not just paper over it.

The migration cost is bounded. Five viewmodels read `enabledCommunityIdList` /
`enabledCommunityIds`; all of them already accept the "send a single
community_id" shape because the RPCs were proto3-multi from the start. The
real work is rethinking the **All Circles** vs. **portfolio mode** distinction
in `feed_screen.dart` and `discover_screen.dart`, which currently render an
empty-state CTA when the set is empty — that empty state has no place in the
new model and should go away.

Option A is tempting because it ships fastest, but it preserves the exact
shape that produced #1547, just with a tighter set of notifier methods. The
next reader who calls `state.enabledCommunityIds.add(id)` directly (it's a
public field on a freezed class) will reintroduce the bug.

If review pushes back on the migration cost, fall back to Option A and add a
runtime assert in the notifier that the set is always either empty, a
singleton, or `communities.length`.

## Decisions

_Resolved 2026-04-30 by the issue author. All recommendations below are
accepted; treat each as load-bearing for the implementation phases._

- **All Circles persistence shape.** Use SharedPreferences key
  `'selected_community_id'` with `String?` semantics: **absence-of-key = All
  Circles**, **present value = that community's ID**. Migration on first
  launch: read the old `'enabled_community_ids'` key; if it contains exactly
  one ID, write that ID under the new key; in all other cases (empty,
  multi-element, missing), delete the old key — the user lands in All
  Circles. Then delete the old key in either branch so we don't carry it
  forward.

- **`switchTo` entry point retained.** Notification taps, post-create flows,
  and invite acceptance keep calling `switchTo(community)`. Under the new
  model it's a thin alias for `selectCommunity(community)` — same behavior,
  but the call sites that semantically express "switch to" don't need a
  rename.

- **Deleted / departed community fallback.** When `setCommunities` runs and
  the persisted single ID is no longer in the loaded list, fall back to All
  Circles. Mirrors the current "fall back to enable all" behavior with
  cleaner semantics.

- **All Circles visual treatment.** Render with a generic icon (e.g.
  `Icons.all_inclusive` or a concentric-circles glyph — concrete choice in
  Phase 2) rather than a `CommunityAvatar`. Subtitle shows the user's total
  community count using `sidebarAllCirclesSubtitle` (parallel to the "N
  members" subtitle on regular rows). Keep a divider between the All
  Circles row and the community list to signal the grouping.

- **ARB strings.** Final wording:
  - `sidebarAllCircles` = `"All Circles"` (es: `"Todos los círculos"`).
  - `sidebarAllCirclesSubtitle` uses ICU plurals on a `count` placeholder
    (e.g. `=0{No circles} =1{1 circle} other{{count} circles}`). `@key`
    metadata declares `placeholders: {count: {type: int}}` per
    [docs/client/i18n.md](../client/i18n.md).
  - Remove `sidebarClearAll` and `sidebarSelectAll` (and their `@` blocks)
    from both `app_en.arb` and `app_es.arb`.

- **Empty-state branches removed.** The `enabledCommunityIds.isEmpty`
  empty-state branches in `feed_screen.dart` and `discover_screen.dart` are
  deleted in Phase 3. The new model has no "nothing selected" state; All
  Circles owns that slot with actual content.

- **Leaderboard "My Communities Only" filter.** Out of scope. It's a
  separate filter from the sidebar selection and is not changing in this
  issue.

## Implementation Plan

### Phase 1: State refactor + persistence migration
**Goal:** Move `SelectedCommunityState` from multi-select set to single-select
optional ID, with a one-time migration of existing persisted state.

**Tasks:**
- [ ] Change `SelectedCommunityState` to carry `String? selectedCommunityId`
      and remove `enabledCommunityIds: Set<String>`. Keep `communities` and
      `isLoading`/`errorMessage` unchanged.
- [ ] Add derived getters: `bool get isAllCircles` (= `selectedCommunityId == null`),
      `List<String> get effectiveCommunityIds` (returns `[id]` for single,
      all IDs for All Circles — used to feed RPCs that take `repeated string`).
      Keep `enabledCommunityIdList` as a deprecated alias that delegates to
      `effectiveCommunityIds` so consumers can migrate gradually within the
      same PR.
- [ ] Replace notifier methods: remove `toggleCommunity`, `enableAll`,
      `disableAll`, `deselectCommunity`, additive `selectCommunity`. Add
      `selectAllCircles()` and a new `selectCommunity(CommunityItem)` that
      replaces (rather than adds to) the selection. Keep
      `switchTo(CommunityItem)` as an alias for deep-link / post-create
      callers — same behavior as the new `selectCommunity`, retained so call
      sites that semantically express "switch to" don't need a rename. Both
      take `CommunityItem` for parity (audit: don't drift to `String`).
- [ ] Update persistence: change SharedPreferences key from
      `'enabled_community_ids'` (`List<String>`) to
      `'selected_community_id'` (`String?`). On first launch, read the old
      key; if exactly one ID, write the new key; in all other cases, delete
      the old key (→ All Circles). Delete the old key after migration.
- [ ] Update `setCommunities` fallback: when the persisted single ID is no
      longer in the communities list, fall back to All Circles (not "enable
      all of them" — same effect, cleaner semantics).
- [ ] Update unit tests in
      [selected_community_test.dart](app/test/services/selected_community_test.dart):
      replace multi-select scenarios with single-select equivalents, keep the
      #1547 regression test (now phrased as "selecting a deleted community
      falls back to All Circles"), add a new test for the migration path.

> **Audit note (async safety):** `selectedCommunityProvider` is a
> non-autoDispose `Notifier`, so the `Cannot use the Ref ... after disposed`
> class of bug doesn't apply here — no `ref.mounted` checks or
> `SafeNotifierMixin` adoption are required for the new flows. If a future
> change converts this provider to `autoDispose`, revisit per
> `docs/client/architecture.md` § Async Safety in Notifiers.

**Validates:** new state shape compiles, all existing call sites read through
`effectiveCommunityIds` without behavior change, persisted state migrates
cleanly across versions.

### Phase 2: Sidebar UI rewrite
**Goal:** Replace the multi-select toggle list with a single-select list led
by an All Circles row; remove the Clear all / Select all button.

**Tasks:**
- [ ] In [sidebar.dart:182-231](app/lib/presentation/screens/home/sidebar.dart#L182-L231),
      delete `_buildCommunitiesHeader`'s right-side `Tappable` (the
      Clear all / Select all toggle). Keep the `COMMUNITIES` section header
      label.
- [ ] Add a new `_buildAllCirclesItem` widget at the top of the communities
      list. Renders a generic icon (e.g. `Icons.all_inclusive` or a custom
      "stack of circles" glyph), the label `sidebarAllCircles`, and a
      subtitle showing the community count (using the new ICU plural
      string). Use the **`Toggle`** primitive (per
      `docs/client/accessibility.md`: "Tap that toggles a selectable
      state"), with `selected: state.isAllCircles` and
      `semanticsLabel: context.l10n.sidebarAllCircles`. Do **not** embed
      "selected"/"not selected" in the label — let `selected:` carry that.
- [ ] Modify `_buildCommunityItem` ([sidebar.dart:310-378](app/lib/presentation/screens/home/sidebar.dart#L310-L378)):
      keep the `Toggle` primitive (audit: it's the right one for
      selectable-state items — don't downgrade to `Tappable`). Wire
      `onTap` to `selectedCommunityProvider.notifier.selectCommunity(community)`
      and set `selected: state.selectedCommunityId == community.id`.
- [ ] Update `AnimatedOpacity` logic: in single-select, the *active* row is
      fully opaque and others are dimmed, mirroring the current visual
      language but inverted in meaning (today: enabled = bright; tomorrow:
      selected = bright). Keep the existing
      `accessibleDuration(context, const Duration(milliseconds: 150))`
      wrapper so reduce-motion stays honored.
- [ ] Add divider between the All Circles row and the regular community
      list to make the grouping obvious.
- [ ] Add new ARB strings to **both** `app_en.arb` and `app_es.arb`
      under [app/lib/l10n/](app/lib/l10n/):
      - `sidebarAllCircles` = `"All Circles"` (es: `"Todos los círculos"`)
        with a `@sidebarAllCircles` description block.
      - `sidebarAllCirclesSubtitle` = `"{count, plural, =0{No circles} =1{1 circle} other{{count} circles}}"`
        (es equivalent), with a `@sidebarAllCirclesSubtitle` block declaring
        `placeholders: {count: {type: int}}`. Per
        `docs/client/i18n.md`, ICU placeholders must be declared in the
        `@key` metadata or `flutter gen-l10n` will fail.
      - Remove `sidebarClearAll`, `sidebarSelectAll`, and their `@`
        metadata blocks from both locale files.
      - Run `npm run generate` after editing ARB files.

**Validates:** sidebar renders the new layout on desktop and mobile, single
tap selects one circle, tapping All Circles restores aggregate view, no
"Clear all" button visible.

### Phase 3: Consumer cleanup — delete portfolio-mode empty states
**Goal:** Now that the model has no "nothing selected" state, delete the
empty-state branches in feed and discover that no longer trigger.

**Tasks:**
- [ ] In `feed_screen.dart` (around line 60-64 per research): remove the
      `enabledCommunityIds.isEmpty` empty-state branch. Always pass
      `effectiveCommunityIds` to `feedProvider.notifier.initialize`.
- [ ] In `discover_screen.dart`: remove the equivalent empty-state branch.
- [ ] In `search_view_model.dart` ([:331-380](app/lib/presentation/viewmodels/search_view_model.dart#L331-L380)):
      switch the `ref.watch` from `selectedCommunityProvider` (set) to
      watching `selectedCommunityId` only. The community-switch reset logic
      needs to fire on the scalar change, not on set diffing. Keep the
      person-filter clear behavior.
- [ ] In `portfolio_metrics_view_model.dart` ([:71-85](app/lib/presentation/viewmodels/portfolio_metrics_view_model.dart#L71-L85)):
      same treatment — listen on `selectedCommunityId` instead of the set.
- [ ] In `portfolio_inbox_view_model.dart`: confirm the unread-badge logic
      still works when reading per-community counts; should be unaffected
      since it iterates `state.communities`, not the selection set.
- [ ] Delete the `enabledCommunityIdList` deprecation alias added in
      Phase 1 once all call sites are migrated. Search for any remaining
      references.

**Validates:** no dead branches for "no communities selected", all screens
render correctly under both All Circles and single-circle states.

### Phase 4: Test sweep
**Goal:** Update or add tests for every behavior change; ensure the #1547
regression remains covered. Per `docs/client/testing.md`, lead with
`ProviderContainer`-driven Notifier tests; reserve widget tests for things
only a widget tree can verify.

**Tasks:**
- [ ] **Notifier tests (primary).** Update
      [selected_community_test.dart](app/test/services/selected_community_test.dart)
      to cover, via `ProviderContainer` overrides:
      - Default state after `setCommunities` is All Circles
        (`state.selectedCommunityId == null`).
      - `selectCommunity(c)` replaces the selection; calling it again with
        a different community switches the selection (no additive set
        behavior).
      - `selectAllCircles()` returns the user to the aggregate view.
      - `effectiveCommunityIds` returns `[id]` for single, all IDs for
        All Circles.
      - **Migration test**: with old `'enabled_community_ids'` of length 1,
        the new key is written; with old length > 1 or empty, the old key
        is deleted and the user lands in All Circles.
      - **#1547 regression**: when the persisted ID is no longer in the
        loaded communities list, fall back to All Circles.
- [ ] Update
      [unread_count_multi_community_integration_test.dart](app/test/presentation/viewmodels/unread_count_multi_community_integration_test.dart):
      "Switching A → B → C" scenarios remain valid; remove any "all
      simultaneously enabled" scenarios since that state is now All Circles.
- [ ] Update viewmodel tests
      ([feed_view_model_test.dart](app/test/presentation/viewmodels/feed_view_model_test.dart),
      [search_view_model_test.dart](app/test/presentation/viewmodels/search_view_model_test.dart),
      [discover_view_model_test.dart](app/test/presentation/viewmodels/discover_view_model_test.dart))
      to drive selection through the new API. Mock dependencies as the
      testing doc prescribes; reset mocks in `tearDown`; dispose containers.
- [ ] **Widget test (supplementary, optional).** Only add if Phase 2's
      visual contract isn't fully covered by the Notifier tests — e.g. to
      assert that the All Circles row appears above the community list and
      that `Toggle.selected` flips correctly on tap. Use
      `app/test/helpers/l10n_helpers.dart` for the localization setup.
- [ ] Run `npm run test:app`, `flutter analyze`, and `npm run lint:dart:a11y`
      — the a11y lint will catch missing `semanticsLabel` on the new
      `Toggle` if it slips through review.

**Validates:** test suite passes; behavioral regressions caught; a11y
lint stays at zero new violations.

## Related Issues

- **#1547** — Stale `selectedCommunity` after community deletion. The
  regression test for this lives in
  [selected_community_test.dart:215-236](app/test/services/selected_community_test.dart#L215-L236)
  and is exactly the kind of bug the type-level single-select model will
  prevent. Carry the test forward in Phase 1, rephrased for the new shape.
