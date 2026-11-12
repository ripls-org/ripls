# Issue #1858: Update content views to match new event-item-2 design

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1858
**Filed:** 2026-05-11 | **Last activity:** 2026-05-11
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit Summary (2026-05-11)

Audited against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/testing.md](../client/testing.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md).

Updates applied to the plan below:

- **MVVM/caching:** Watch state lives in [feed_screen.dart:151-169](../../app/lib/presentation/screens/feed/feed_screen.dart#L151-L169)
  today as a per-widget `Map<String, bool?>` — that's the "manual cache map
  in widgets" anti-pattern the caching doc explicitly forbids. Moving the
  star into the content view is a chance to *fix* that, not relocate it.
  Phase 3 now mandates loading watch state through a Notifier/Repository,
  not lifting the widget-state map into the new home.
- **i18n:** Added explicit ARB tasks in every phase that introduces a new
  user-visible string. Existing keys `watchItemTooltip` /
  `unwatchItemTooltip` ([app_en.arb:3785-3789](../../app/lib/l10n/app_en.arb#L3785-L3789))
  are reused for the star's selected/unselected semantic label. Spanish
  ARB updates are required in the same PR.
- **Accessibility primitives:** All taps in the new widgets go through
  `Tappable` / `Toggle` / `IconAction` — no `GestureDetector` /
  `InkWell` / raw `IconButton`. The star is a `Toggle` (selectable
  state) with the label describing the *element*, not the action. `Claim`
  / `+ Add` rows use `Tappable` with `context.l10n` semantic labels.
- **Theming:** Hardcoded hex from the HTML mock (coral `#E89B7E`,
  `rgba(255,255,255,0.65)`, etc.) must route through `AppColors.xxx(context)`.
  If a semantic color is missing, add a named helper to `AppColors`
  rather than inlining the hex.
- **File size:** Each content view is already 700–964 lines; the gate
  fires at 1,000 lines. Phase 3 explicitly extracts logic into the
  shared widgets and into a small helper rather than expanding the views.
  The `// dart-line-count-allow` hatch is forbidden for this change.
- **Behavior preservation:** The existing `SuggestionChipStrips`
  ([suggestion_widgets.dart:14-89](../../app/lib/presentation/widgets/shared/needs/suggestion_widgets.dart#L14-L89))
  has an ask/offer toggle that the new design's single vertical list
  doesn't show. This is a behavior change against the "no other
  functionality changes" rule — added as an Open Question.
- **Testing:** Reweighted toward ViewModel tests (where the watch-state
  lift happens) per [docs/client/testing.md](../client/testing.md);
  widget tests on the shared widgets are still listed but secondary.
- **Server boundary:** Confirmed no server, proto, or observability
  changes are needed. Section added under "Codebase Findings" to make
  this explicit so reviewers don't have to infer it.

## Problem Statement

Refresh the four content views — `GearContentView`, `ExperienceContentView`,
`RequestContentView`, `CommunityContentView` — to match the new visual layout
in [docs/cowork/App Design/event-item-2.html](../cowork/App%20Design/event-item-2.html).
The refresh has two parts:

1. **First-tab refresh (all four views).** Remove the two top icons (watch and
   info) that sit in the upper-right corner today. Convert the watch action
   into a **star** that appears in the title row, immediately next to the
   title. Render the event's **time**, **location**, and **owner** as
   tappable rows beneath the description on the first tab — same shape across
   all four views, with consistent icon + value + optional action affordance.
2. **Second-tab refresh (Experience + Request only).** Replace the current
   contributions/needs rendering with a **checklist row layout** matching
   [new-checklist.png](../cowork/mocks/content/new-checklist.png). Replace the
   existing "suggestions cloud" empty state with a **vertical suggestion
   checklist** matching [new-suggestions.png](../cowork/mocks/content/new-suggestions.png).
   Inline the same suggestion list into the **Request modal** and **Offer/
   Contribution modal** so users can add from the list while composing
   ([new-request-modal.png](../cowork/mocks/content/new-request-modal.png)).

The issue explicitly scopes this as a visual/layout refresh — no behavioral
or functional changes.

## Context from Comments

There are no comments on this issue. The description is the only source of
intent, but it's detailed and the mocks are concrete. The main ambiguity is
how to apply the "same first-tab pattern" to content types that don't have
a one-to-one mapping for time and/or location (see Open Questions).

## Codebase Findings

- **All four content views follow the same shell pattern**: a full-bleed
  background image, a `Positioned(top:0, right:0)` icon stack in the upper
  right (`_buildInfoButton`), and a bottom content area driven by a
  `*BottomContent` widget that hosts the tab bar and panes.
  - Gear:   [gear_content_view.dart:389-475](../../app/lib/presentation/screens/gear/gear_content_view.dart#L389-L475)
  - Experience: [experience_content_view.dart:511-625](../../app/lib/presentation/screens/experience/experience_content_view.dart#L511-L625)
  - Request: [request_content_view.dart:459-528](../../app/lib/presentation/screens/request/request_content_view.dart#L459-L528)
  - Community: [community_content_view.dart:326-463](../../app/lib/presentation/screens/communities/community_content_view.dart#L326-L463)
- **The watch button is owned by the feed**, not by the content view. The
  feed constructs a `WatchIconButton` via `_buildWatchIconForItem` and passes
  it in as `widget.watchButton` ([feed_screen.dart:151-169](../../app/lib/presentation/screens/feed/feed_screen.dart#L151-L169)).
  Content views render whatever they're handed. The watch state is held in
  a `Map<String, bool?>` (`_watchStateCache`) inside the feed widget state
  — that's a **manual cache map inside a widget**, which the caching doc
  explicitly forbids (see [docs/client/caching.md §"Repositories own
  caching - ViewModels don't"](../client/caching.md#L42-L48)). The refresh
  should lift this state into a Notifier / Repository (e.g., a watch-state
  notifier keyed by `(itemType, itemId)`), and content views should consume
  it via `ref.watch(...)`.
- **The info modal exposes owner, createdAt, updatedAt, and photo
  attribution** ([content_info_modal.dart:15-32](../../app/lib/presentation/widgets/content/content_info_modal.dart#L15-L32)).
  Owner + "shared 1d ago" maps cleanly to the new owner row in the mock.
  Photo attribution has no obvious home in the new layout — the HTML mock
  shows a small right-aligned "Photo by … on Unsplash" line just above the
  tabs, which is the natural place to put it.
- **Existing first-tab content varies by type.** Experience already has
  time/location/RSVP rows in `ExperienceEventPane`. Request has owner +
  location in `RequestRequestPane`. Gear has location + description in
  `GearTypePane` but no time. Community has type label + member count + no
  time/location.
- **The suggestion cloud is two widgets** in
  [suggestion_widgets.dart](../../app/lib/presentation/widgets/shared/needs/suggestion_widgets.dart):
  `SuggestionChipStrips` (used by Request) and `EmptyStateSuggestions` (used
  by Experience). Both render a wrapped center-aligned cloud of `+ Foo`
  pill chips. The new design replaces these with a vertical checklist of
  rows: dashed square + label + right-aligned coral `+ Add` text.
- **Watch state is loaded async per item** with a per-widget cache; that
  *behavior* (cached, non-blocking, `null` while loading) needs to be
  preserved when the star moves into the title row so the title doesn't
  jank on mount — but the *mechanism* moves to the ViewModel/Repository
  layer (see point above).
- **Server/proto boundary:** This is a client-only change. No server or
  proto changes, no new RPCs, no migration. The watch-state lift uses
  whatever RPC the existing `WatchIconButton.onToggle` already calls.

## Potential Solutions

### Option A — Refactor in place, one view at a time

- **Approach:** Edit each of the four content views directly to remove the
  top-right icon stack and add the new title-row star + info rows in the
  first tab. Update the feed to stop passing `watchButton` (or repurpose the
  prop into a `starWidget` slot rendered inside the pane). For Experience
  and Request, replace the suggestion cloud + contribution rendering in
  `ExperienceDetailsPane`, `ExperienceResultsPane`, and `RequestPlanPane`
  with new checklist row widgets. Update `RequestModal` (and the existing
  offer/contribution sheet) to mount the same vertical suggestion list.
- **Pros:** Smallest diff per view; respects "no other functionality
  changes"; doesn't require new abstractions.
- **Cons:** The four views diverge today in how they render the first tab
  (different panes, different prop shapes), so we still end up writing four
  separate top-row implementations. Risk of subtle inconsistencies if one
  view's pane evolves before the next.
- **Effort:** Medium.

### Option B — Extract shared `ContentTopRows` widget, then refactor each view

- **Approach:** Build a new shared widget (`ContentTopRows` or similar) that
  renders the title-with-star row + the time/location/owner rows from a
  small data interface (`title`, optional `whenLabel + onWhenTap`, optional
  `whereLabel + onWhereTap`, `owner`, `createdAt`, `isStarred`,
  `onToggleStar`). Drop the existing top-right icon stack from each view
  and mount `ContentTopRows` at the top of each first-tab pane. Build a
  shared `ChecklistRow` + `SuggestionChecklist` widget pair for the
  second-tab redesign.
- **Pros:** Single source of truth for the first-tab visual; future content
  types (e.g. polls) plug into the same widget; matches the HTML mock's
  uniform `InfoRow` primitive; smaller per-pane edits after the shared
  widget lands.
- **Cons:** More upfront work (a new widget plus its tests); requires
  deciding the data interface carefully so it survives type-specific
  optionality (no time on Gear/Community, etc.).
- **Effort:** Medium-Large.

### Option C — Big-bang single PR

- **Approach:** Land everything in one branch — top-row changes across all
  four views, the second-tab checklist refactor for Experience + Request,
  and the modal inline suggestion changes.
- **Pros:** No interim half-shipped look.
- **Cons:** Hard to review; hard to test incrementally; if any one piece
  drags (especially the modal change), all the visual refresh is blocked.
- **Effort:** Large.

## Recommendation

**Option B, phased.** The four views already share a top-right icon stack
and the mock asks for an identical top-row across all of them — that's
exactly when a shared widget pays for itself. Doing it via Option A leaves
four near-duplicate implementations and invites drift the next time someone
edits one view. The second-tab checklist also has clear reuse between
Experience and Request: one row widget, one zero-state widget, shared by
both panes and by the two modals.

Phase the work so the shared widgets land first and are reviewed in
isolation, then each view is migrated in a separate, small PR.

## Resolved Decisions

All open questions were answered by the issue author on 2026-05-11. The
implementation plan below reflects these decisions.

- **Time / location / owner on Gear and Community:** All three rows are
  optional in `ContentTopRows`. Each view shows only the rows that
  apply — Gear shows location + owner (no time); Community shows owner
  only (no time, no location).
- **Photo attribution:** Move to the small right-aligned "Photo by … on
  {source}" line above the tab bar, exactly as in the HTML mock.
- **createdAt / updatedAt:** Replace the info modal's createdAt with an
  inline "shared {relative} ago" string on the owner row. Drop
  `updatedAtUnixSec` from the surface — it has no place in the new
  layout.
- **Star vs. watch — UX change only, no renames:** The star is the new
  visual presentation of the existing watch feature. **Keep all
  implementation names** —
  [`WatchIconButton`](../../app/lib/presentation/widgets/content/watch_icon_button.dart),
  whatever provider/repository wraps the watch RPC, the `watchItemTooltip`
  / `unwatchItemTooltip` l10n keys, and the `_watchStateCache` in
  `feed_screen.dart` while it still exists. The new `ContentTitleRow`
  star is a thin visual wrapper that consumes the same watch state.
  No proto changes, no symbol renames, no RPC changes.
- **Need-row state:** Decorative checkbox icon next to label; coral
  `Claim` button on the right when unclaimed; filled green check +
  claimer avatar when claimed (matching the HTML mock's `ClaimedRow`).
- **Offer/Contribution modal location:** It's the sheet opened by the
  "Offer something" button on the content view — implemented today as
  [batch_add_contribution_sheet.dart](../../app/lib/presentation/widgets/experience/needs/batch_add_contribution_sheet.dart).
  Phase 5 mounts the new `SuggestionChecklist` inside this sheet plus
  the matching request-something sheet.
- **Ask/offer toggle removal:** The `ConnectedToggle` in
  `SuggestionChipStrips` is removed. The Request-scope empty state
  becomes a single vertical list of asks; the offer-side is no longer
  surfaced from this entry point. This is an accepted functional
  change as part of this refresh — no follow-up issue needed.

## Implementation Plan

### Phase 1: Shared row primitives

**Goal:** Land the shared widgets that all four content views will use,
unit-tested in isolation, before touching any pane.

**Tasks:**
- [ ] Create `app/lib/presentation/widgets/content/content_info_row.dart`
      with a `ContentInfoRow` widget (icon + value + optional trailing
      action button + bottom divider toggle). Row tap target uses
      `Tappable`; trailing icon button uses `IconAction`. All semantic
      labels come from `context.l10n`. Colors via `AppColors.xxx(context)`
      — no hex literals.
- [ ] Create `app/lib/presentation/widgets/content/content_title_row.dart`
      with a `ContentTitleRow` widget (serif title + optional star
      `Toggle`). The star uses `Toggle` with `selected: isStarred` and a
      label that describes the element (`context.l10n.watchItemTooltip` /
      `unwatchItemTooltip` already exist — reuse them; don't embed state
      into the label string). Accepts an async `bool?` watched state
      (`null` = loading, disables tap). The widget takes the value, not
      the loader — loading lives in the ViewModel.
- [ ] Create `app/lib/presentation/widgets/content/content_top_rows.dart`
      composing title + description + optional time/location/owner rows.
      Each row is optional so types without time (Gear, Community) or
      without location (Community) just skip them.
- [ ] Create `app/lib/presentation/widgets/shared/needs/checklist_row.dart`
      with `NeedRow` (unclaimed + Claim button via `Tappable`), `ClaimedRow`
      (filled + claimer avatar), and `SuggestionRow` (dashed checkbox + `+
      Add` via `Tappable`).
- [ ] Add a small `SuggestionChecklist` widget that wraps a list of
      `SuggestionRow`s with the `SUGGESTIONS · Add all` header.
- [ ] Add ARB entries for the new section labels (`weNeedHeader`,
      `suggestionsHeader`, `needAgreementOnHeader`, `addAllAction`,
      `sharedRelativeAgo` with `{relative}` placeholder, etc.) to both
      `app_en.arb` (with `@key` metadata) and `app_es.arb`. Run
      `npm run generate` to update `AppLocalizations`. All user-visible
      strings in the new widgets come from `context.l10n.*` — none
      hardcoded.
- [ ] Add any missing `AppColors` helpers if the mock introduces a new
      semantic color (e.g., the coral on dark background variant).
- [ ] Widget tests for each new widget under
      `app/test/presentation/widgets/content/` and
      `app/test/presentation/widgets/shared/needs/` — visual smoke tests
      with both light and dark theme. Note: per
      [docs/client/testing.md](../client/testing.md), business logic
      tests belong with the ViewModel work in Phase 3; these widget
      tests are visual-only.

**Validates:** Visual primitives match the mocks under both light/dark
and accessibility scale; CI lint passes (`npm run lint:dart:a11y`,
`npm run lint:dart:i18n`, `npm run lint:dart:size`); nothing depends on
these widgets yet, so they can be reviewed in isolation.

### Phase 2: Photo attribution surface

**Goal:** Decide where photo attribution lives once the info modal goes
away, and ship that placement first so it isn't lost when each view loses
its info button.

**Tasks:**
- [ ] Add a small `PhotoAttributionLine` widget that renders the right-
      aligned credit line shown in the HTML mock (just above the tab bar).
      The two links (artist + Unsplash) use `Tappable` with l10n'd
      semantic labels (e.g., `a11yPhotoCreditArtist`, `a11yPhotoCreditSource`
      — add to ARB).
- [ ] Wire it into each content view's bottom-content stack so it appears
      only when attribution is present. The data already flows through
      `ContentViewHelpers.getBackgroundAttribution`.
- [ ] Text color via `AppColors.textOnPhoto(context)` (or whichever
      existing helper is used for text-on-dark-image — extend `AppColors`
      if a new one is needed).
- [ ] Widget test verifying the line renders only when attribution is
      non-null, and that the links use `context.l10n` labels.

**Validates:** Attribution is still visible everywhere it was before; no
Unsplash-credit regression; a11y lint passes.

### Phase 3: First-tab refresh — one view at a time, behind a watch-state Notifier

**Goal:** Migrate each view to the new top-row, in order of complexity.
The first sub-phase is the watch-state lift, since all four views consume
it.

**Order:**
- 3.0 — watch-state Notifier + Repository (shared groundwork)
- 3.1 — Experience
- 3.2 — Request
- 3.3 — Gear
- 3.4 — Community

Experience has the fullest data shape (time + location + owner), so any
data-shape problems surface there first.

**Phase 3.0 tasks (watch-state lift — keep all "watch" naming):**
- [ ] Audit the existing watch-toggle RPC (whatever
      `WatchIconButton.onToggle` triggers today). If a repository wrapper
      exists, extend it; if not, add one in `app/lib/data/repositories/`
      with namespace `'watch'` and key shape `'watch:{itemType}:{itemId}'`.
      Keep "watch" everywhere in implementation — this is a UX change
      only.
- [ ] Add a `WatchStateNotifier` (or family provider keyed by
      `(itemType, itemId)`) that exposes `AsyncValue<bool>` (null/loading
      → `AsyncLoading`; loaded → `AsyncData(bool)`). Toggle goes through
      the repository, which handles cache invalidation. Reuse the
      `WatchIconButton` widget where it still appears (e.g., inbox swipe
      action) — only its placement in content views changes.
- [ ] ViewModel tests: cache hit returns immediately; cache miss fetches;
      toggle calls the right service method and invalidates; disposal
      during async completes without throwing (see
      [docs/client/testing.md §Disposal Testing](../client/testing.md)).

**Phase 3.x tasks per view:**
- [ ] Remove `_buildInfoButton` and the `Positioned(top:0, right:0)` icon
      stack.
- [ ] Drop the `watchButton` prop. The view consumes
      `watchStateProvider((itemType, itemId))` via `ref.watch` and passes
      the `bool?` and `onToggle` callback into `ContentTitleRow`.
- [ ] Replace the existing first-tab title/description/etc. with
      `ContentTitleRow` + `ContentTopRows`. Skip rows that don't apply for
      that type (Gear: no time; Community: no time, no location).
- [ ] Replace `createdAt → "shared {relative} ago"` rendering using the
      new `sharedRelativeAgo` l10n key from Phase 1. Drop the
      `ContentInfoModal` import.
- [ ] Watch the file size — if Phase 1's extractions still leave the
      view >1,000 lines, extract more sub-widgets (e.g., split the
      bottom-content builders into a separate file). Do **not** add
      `// dart-line-count-allow`.
- [ ] Update `feed_screen.dart` once per view migration: remove the
      `watchButton: _buildWatchIconForItem(item)` argument. After all
      four views are migrated, delete `_buildWatchIconForItem` and
      `_watchStateCache` from `feed_screen.dart`.

**Validates:** Each view ships independently, can be reviewed in a small
diff, and any regression is bounded to that one view. The watch toggle
works from feed, deep-link, and inbox preview. `npm run lint:dart:size`
stays green.

### Phase 4: Second-tab checklist (Experience + Request)

**Goal:** Replace contribution rendering + suggestion cloud with the
vertical checklist layout.

**Tasks:**
- [ ] Update `ExperienceDetailsPane` and `ExperienceResultsPane` to render
      needs via `NeedRow` / `ClaimedRow`. Keep all existing
      claim/offer/cancel callbacks — visual change only.
- [ ] Update `RequestPlanPane` the same way.
- [ ] Replace `EmptyStateSuggestions` callers (Experience scope) and
      `SuggestionChipStrips` callers (Request scope) with the new
      `SuggestionChecklist`. Keep both old widgets until call sites are
      migrated; delete them afterward.
- [ ] Remove the `ConnectedToggle` ask/offer switch from
      `SuggestionChipStrips`. The Request-scope empty state becomes a
      single vertical list of asks; the offer side is no longer surfaced
      here. (Confirmed product decision — see Resolved Decisions.)
- [ ] ARB entries already exist after Phase 1 — verify no additional
      strings needed. If the toggle's `requestNeedsAddToAsk` /
      `requestNeedsOfferSomething` keys become unused anywhere else,
      delete them in Phase 6 cleanup.

**Validates:** Visual changes; existing claim/offer flows still produce
the same state transitions (covered by existing pane tests). ViewModel
tests for the panes' callbacks still pass unchanged.

### Phase 5: Modal inline suggestions

**Goal:** Mount the vertical suggestion list inside the Request modal
and the Offer/Contribution modal.

**Tasks:**
- [ ] Update the "Offer something" sheet —
      [batch_add_contribution_sheet.dart](../../app/lib/presentation/widgets/experience/needs/batch_add_contribution_sheet.dart) —
      to mount the new `SuggestionChecklist` between the text input and
      the footer buttons. Tapping a suggestion appends it to the list
      (matching the mock's `+ Add` action).
- [ ] Locate the matching "Request something" sheet (the request-side
      counterpart in
      [_shared_sheet_widgets.dart](../../app/lib/presentation/widgets/experience/needs/_shared_sheet_widgets.dart)
      or its caller in `ExperienceDetailsPane` / `RequestPlanPane`) and
      apply the same treatment.
- [ ] Add `NEED AGREEMENT ON` section when relevant (Request modal in the
      mock shows a "Location  + Poll" row).
- [ ] Modals must continue to be shown via `showAccessibleModal` (a11y
      lint rule `prefer_accessible_modal`).
- [ ] Verify l10n: ARB keys already added in Phase 1; if Phase 5 surfaces
      any modal-specific copy from the mocks (e.g., "Ask the group"),
      add those keys now to both `app_en.arb` and `app_es.arb`.
- [ ] All buttons inside the sheet use l10n'd labels and existing
      `ModalHeader` for the title (already wraps in `Semantics(header: true)`).

**Validates:** Composing a request/offer no longer needs an external
suggestion cloud; the flow inside the sheet covers the full empty-state
path. Focus restoration works on dismiss (handled by `showAccessibleModal`).

### Phase 6: Cleanup

**Goal:** Remove the now-orphaned widgets and any dead props.

**Tasks:**
- [ ] Delete `EmptyStateSuggestions` and `SuggestionChipStrips` if
      nothing still references them.
- [ ] Delete `ContentInfoModal` (or strip down to a thin wrapper if
      anything else uses it).
- [ ] Confirm the `watchButton` prop is gone from every content view and
      from `feed_screen.dart`; delete `_watchStateCache` and
      `_buildWatchIconForItem` from the feed.
- [ ] Update `app/test/` widget tests to match the new structure;
      remove tests for deleted widgets.
- [ ] Drop any l10n keys orphaned by deletions (the `lint:dart:i18n`
      check flags unused keys).
- [ ] Manual test pass on a real device: each view in feed, in deep-link,
      and in the inbox preview. Verify in both light and dark theme and
      with text scale ≥1.3.
- [ ] Full CI green: `npm run build`, `flutter test --reporter expanded`,
      `npm run lint:dart:a11y`, `npm run lint:dart:i18n`,
      `npm run lint:dart:size`, `npm run lint:nav`,
      `npm run lint:ephemeral-comments`.

**Validates:** No dead code; lint passes; `flutter test` green.

## Related Issues

- #1802 — "modals-glass-sweep" (file currently open in IDE): visual
  cleanup of modal surfaces. Likely touches the same Request/Offer sheets
  that Phase 5 modifies; coordinate or sequence so the suggestion-checklist
  change lands on top of the glass-sweep result.
- #1056 — "swipe-item-tabs": tab interaction redesign across content
  views. The first-tab refresh here doesn't change tab structure, but
  worth confirming neither lands a conflicting layout.
