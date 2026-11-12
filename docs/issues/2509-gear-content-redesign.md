# Issue #2509: Migrate gearContentView away from tab/menu/modal to the self-serve scrolling design (match gear-screen-redesign-v2)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2509
**Filed:** 2026-06-18 | **Last activity:** 2026-06-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/client/content.md, docs/client/manage_menus.md, docs/client/architecture.md, docs/client/conventions.md (anchors), docs/cowork/App Design/gear-screen-redesign-v2.html (target mock); reference implementations docs/issues/2278-experience-content-redesign.md, docs/issues/2293-request-content-morph-ux.md, docs/issues/2280-pitching-in-expand.md, docs/issues/2291-location-morph-panel.md

## Problem Statement

`GearContentView` is the last of the four primary content views still on the
**legacy tab + bottom-panel + dropdown-menu + modal** architecture
([gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart),
[gear_bottom_content.dart](../../app/lib/presentation/screens/gear/widgets/gear_bottom_content.dart)).
It renders a 3-tab pane switcher (Loan/Giveaway · Details · Chat), drives owner
actions through the legacy **status-icon checklist** menu
([gear_menu_items.dart](../../app/lib/presentation/screens/gear/widgets/gear_menu_items.dart)),
and pushes a stack of bottom-sheet modals (location picker, transfer, arrange
pickup, past transfer, equity).

Experience (#2278) and Request (#2293) already migrated to the **read-shell**
pattern: a single bottom-anchored editorial sheet over the full-bleed hero, with
cards that *morph-expand* into full-screen panels (conversation, location,
roster), a top-bar `···` overflow that opens a flat
[`PollManageMenuSheet`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart),
and edit routed through `ContentEditBar` + `ContentEditingMixin`. The supporting
widget kit (`ContentHeadline`, `ContentDiscussionCard`,
`ContentFactsRow`/`ContentFactData`/`ContentFactCta`, `ContentEdgesCard`,
`ContentDetailCard`, `ContentLocationCard`, `ContentImpactRow`,
`ContentActionChip`, `openContentMorphPanel`) is already shared and battle-tested
by those two views.

This issue brings gear onto the same architecture, matching the visual/interaction
target in
[gear-screen-redesign-v2.html](../../docs/cowork/App%20Design/gear-screen-redesign-v2.html):
status pill + mode flag, title, quote/description card, a **Details** 2×2 specs
grid, a **Where** location card, a **Who's using it** card that expands into a
story (metrics + past borrowers), an optional **Hand-off** card (current holder
only), a single sticky primary action whose verb adapts to state, and owner
management folded into the `···` overflow.

**Important nuance the issue text understates:** gear is the *hardest* of the
three to migrate, because unlike request/experience it carries two distinct
workflows (loan vs giveaway) over a five-state transfer machine, supports
**multiple concurrent loans per item**, and the legacy gear menu encodes
**per-borrower** actions (confirm pickup time / pickup / return for each active
transfer) that do not map onto a flat enum sheet. The mock also introduces a
**borrow queue / "Get in line" waitlist** ("1 waiting", "Next up", auto-nudge)
that the current loan state machine does not model. Whether that queue is real or
visual-only is the single biggest scope decision (see Open Questions).

## Context from Comments

The issue has **no comments** — it was filed and immediately picked up for
planning. All requirements come from the issue body and the linked mock. Two
implications:

- The acceptance criteria are the author's (and were written by the same person
  filing this plan), so they're trustworthy but unreviewed by a second party.
- The mock is explicitly *"a static HTML prototype; treat it as the visual /
  interaction target, not pixel-exact spec."* Several mock affordances (the
  waitlist, the hand-off card, the "story" expand) need a product decision about
  depth vs. parity-with-current-behaviour before implementation — those are
  raised in Open Questions rather than silently assumed.

The [manage_menus.md](../../docs/client/manage_menus.md) doc independently
confirms the direction: it already records that the gear checklist menu is
**"slated to migrate to the flat-sheet `PollManageMenuSheet` pattern"** — so the
overflow-menu half of this work is pre-blessed.

## Potential Solutions

### Option A: Full read-shell migration, parity-first (recommended)

- **Approach:** Build `GearReadShell` + `GearTopBar` mirroring the request
  files, composing the existing shared content widgets. Add
  `gearContentExpandedProvider` to
  [content_expanded_provider.dart](../../app/lib/presentation/viewmodels/content_expanded_provider.dart)
  and route Chat, Where, and Who's-using-it into morph panels via
  `openContentMorphPanel`. Replace `GearMenuItems` checklist with a flat
  `GearManageMenuSheet` + `GearManageAction` enum for the *owner-level* actions
  (Set Details / Switch mode / Lending log / View impact / Mark returned & close
  / Log past), while **keeping per-borrower confirm actions inside the
  Who's-using-it panel** (where each borrower row lives) rather than forcing them
  into the flat sheet. Render the mock's queue using the **existing** transfer
  data (current holder + pending requesters) without introducing a new
  server-side waitlist. The single sticky action verb is computed from
  role + availability + transfer state.
- **Pros:** Maximum consistency with request/experience; reuses the entire shared
  kit; ships the visual redesign without a server/proto change; respects the
  documented manage-menu migration; per-borrower actions stay where they're
  legible.
- **Cons:** Largest client diff; must carefully preserve every existing loan +
  giveaway transition (the gear menu has the richest action set of any content
  type); the "queue" reads as a queue visually but isn't a true waitlist, which
  could mildly over-promise.
- **Effort:** Large.

### Option B: Visual reskin only — keep tabs internally, restyle panes

- **Approach:** Keep `GearBottomContent`/tab scaffolding but restyle the Type
  pane to look like the scrolling sheet (specs grid, location card, who's-using
  card) and hide the tab bar.
- **Pros:** Smaller diff; lower regression risk on transfer flows.
- **Cons:** Fails the core acceptance criterion ("the 3-tab bar / pane switcher
  is removed"); diverges from request/experience architecture; leaves the legacy
  checklist menu in place; accrues more tech debt rather than retiring it. The
  morph-expand interactions (which *are* the redesign) can't be faithfully built
  on the pane switcher.
- **Effort:** Medium.

### Option C: Full migration **plus** a real borrow queue/waitlist

- **Approach:** Option A, but also add server-side waitlist modelling so "Get in
  line", "Next up", "1 waiting", and auto-nudge-on-return are real.
- **Pros:** Fully realizes the mock's interaction model.
- **Cons:** Requires proto + transfer-service + notification changes far outside
  a client redesign; turns a UI issue into a multi-surface feature; high risk and
  scope creep. The loan state machine
  ([workflows/loan.md](../../docs/workflows/loan.md)) would need a new state or a
  parallel queue entity.
- **Effort:** Extra-large (multi-PR, server + client).

## Recommendation

**Option A.** It satisfies every acceptance criterion in the issue, reuses the
shared read-shell kit that request/experience already proved out, and retires the
legacy gear checklist menu per the documented plan — all without a proto/server
change. The one deliberate compromise is rendering the mock's "queue" from
existing transfer data (current holder + pending requesters) rather than a true
waitlist; the real waitlist (Option C) should be a **separate, server-led
follow-up issue** so this redesign isn't blocked on a workflow change.

Key tradeoff being made: per-borrower loan actions (a single gear item can have
multiple concurrent loans, each needing confirm-pickup-time / pickup / return)
**do not** belong in the flat overflow sheet. Keep the overflow sheet for
*item-level* owner actions only, and surface per-borrower controls on the
borrower rows inside the Who's-using-it morph panel — this is both more legible
than the current dropdown and avoids cramming a variable-length, per-person action
list into a fixed enum.

## Open Questions

- **Is the borrow queue / "Get in line" waitlist in scope, or visual-only?** —
  Recommendation: **visual-only for this issue.** Render the current holder and
  any pending requesters as the "who's using it" list, but do not build a true
  ordered waitlist, "Next up" promotion, or auto-nudge-on-return — those need a
  transfer-state-machine change. File a follow-up for the real queue and link it.
  This keeps #2509 a pure client redesign.

- **How should the "Who's using it" card expand — reuse the equity/metrics
  surface or a new panel?** — Recommendation: a **new gear morph panel**
  (`GearStoryPanel` / who's-using-it panel) that shows the metrics tiles + past
  borrowers list from the mock, and hosts the **per-borrower owner actions**. Feed
  its metrics from the existing equity data (`showGearEquityModal` /
  `ItemMetricsScreen` content) rather than duplicating computation. Don't try to
  reuse `ItemMetricsScreen` whole — it's a pushed screen, not a morph panel.

- **Where do the per-borrower confirm actions (pickup time / pickup / return)
  live?** — Recommendation: on the **borrower rows inside the Who's-using-it
  panel**, not the overflow sheet (see Recommendation). The flat
  `GearManageMenuSheet` carries only item-level owner actions.

- **Does the optional Hand-off card replace `arrange_pickup_modal`, or wrap it?**
  — Recommendation: surface Where/When as an inline card on the read shell for
  the current holder, but have each row **open the existing
  [arrange_pickup_modal.dart](../../app/lib/presentation/screens/gear/arrange_pickup_modal.dart)**
  flow rather than reimplementing pickup/return editing. Minimizes new surface.

- **Should mode-switching (borrow ↔ giveaway) mid-loan stay supported?** — The
  mock's `···` menu offers "Switch to giveaway" even while out with a borrower
  ("current holder becomes the new owner"). Recommendation: preserve **exactly**
  the current capability — do not add or remove a mode-switch path; just relocate
  whatever exists today into the new overflow sheet. Confirm current behaviour
  against `GearMenuItems` before wiring.

- **Edit mode: new `GearEditPane` or reuse the existing edit fields?** —
  Recommendation: extract a `GearEditPane` mirroring `RequestEditPane`
  (title/description) and keep the **debounced metadata auto-save** (brand/model/
  value/weight/website) on the specs card in edit mode, since that 800ms-debounce
  behaviour is gear-specific and already exists. Don't regress it. **But fix the
  architecture violation while moving it:** today's `_saveMetadataInline` reads
  `gearRepositoryProvider` and calls `gearRepository.saveGear` **directly from the
  widget**, breaking the "ViewModels are the ONLY layer that calls Repositories"
  rule ([architecture.md](../../docs/client/architecture.md)). Route the debounced
  save through a `GearNotifier` method (e.g. a silent `saveMetadata`) instead of
  re-creating the direct-repository call in the new code.

## Implementation Plan

### Phase 1: Scaffolding + read shell skeleton (no behaviour change to flows)

**Goal:** Stand up the new shell behind the existing one and render the
available/owner state end-to-end, proving the layout and the shared-widget
composition before touching transfer flows.

**Tasks:**
- [ ] Read the anchors and reference PRs: `content.md`, `manage_menus.md`,
      `conventions.md`, and the request/experience read-shell + content-view files.
- [ ] Add `gearContentExpandedProvider` to
      [content_expanded_provider.dart](../../app/lib/presentation/viewmodels/content_expanded_provider.dart),
      mirroring the request/experience flags.
- [ ] Create `gear_read_shell.dart` (`GearReadShell`) and `gear_top_bar.dart`
      (`GearTopBar` — attribution + `···` overflow), mirroring the request files.
- [ ] Compose the static cards from shared widgets: `ContentHeadline` (title),
      status pill + mode flag, `ContentDiscussionCard` (quote/description →
      conversation), a **specs** card (`ContentDetailCard`/`ContentDetailRows` or
      a 2×2 grid — pick the closest existing primitive; reuse theme tokens, no new
      colour/font consts), and `ContentLocationCard` for **Where**.
- [ ] Wire `GearContentView.build` to render `GearReadShell` in read mode behind
      a feature flag or simple swap, keeping the old `GearBottomContent` path
      available until Phase 4 so nothing regresses mid-migration.

**Validates:** The shared kit composes a faithful gear layout; accent
(loan vs giveaway) and the hero/gradient/safe-area scaffolding behave; provider
wiring is correct.

### Phase 2: Morph panels — Chat, Where, Who's-using-it

**Goal:** Replace the tab panes with morph-expand panels.

**Tasks:**
- [ ] Route the discussion card → conversation via `openContentMorphPanel`
      (`gearContentExpandedProvider`), wrapping the existing
      `InlineConversationView` in a `GearChatPane` + `ContentMorphPanel`
      (mirror `RequestConversationPanel`/`RequestChatPane`). Preserve media
      carousel add/delete/reorder and unread reset.
- [ ] Route the **Where** card → a location morph panel reusing the
      `LocationPickerHelper` flow (mirror `request_location_panel.dart` /
      #2291). Owner-editable; everyone gets directions.
- [ ] Build the **Who's-using-it** card (`ContentEdgesCard`-style) listing the
      current holder + pending requesters from `state.transferContext`, with the
      footer share-stats trail (times shared · friends · saved), expanding into a
      `GearStoryPanel` morph panel (metrics tiles + past borrowers). Per-borrower
      owner actions live on the rows here.
- [ ] Add the optional **Hand-off** card for the current holder, each row opening
      `arrange_pickup_modal`.
- [ ] **Accessibility** ([accessibility.md](../../docs/client/accessibility.md)):
      every new interactive surface (cards, queue rows, hand-off rows, top-bar
      overflow) uses `Tappable`/`IconAction` with an l10n-sourced `semanticsLabel`;
      announce queue/holder changes via `LiveRegion`; keep icon-only buttons at a
      ≥48dp hit target. Status must not be conveyed by colour alone — the status
      pill and "Has it now"/"Next up" badges pair colour with text. Run
      `node scripts/check_a11y.js` (the CI a11y gate) before the PR.

**Validates:** The morph-reveal UX matches request/experience; the queue renders
from real transfer data; per-borrower actions are reachable and legible; the
a11y gate passes.

### Phase 3: Sticky action + flat overflow manage sheet

**Goal:** Replace the action-button dropdown and the legacy checklist menu.

**Tasks:**
- [ ] Implement the single sticky primary action whose verb adapts to
      role + availability + transfer state across the four mock states:
      owner-available ("manage"/share), non-owner available ("I've got it" /
      request), out-with-a-friend ("Get in line" — visual queue per Open
      Questions), current-holder ("Mark as back"), giveaway ("Claim it").
      Reuse `MorphingActionButton`/`WorkflowActionButtons` factories where the
      state machine already provides the right status.
- [ ] Create `gear_manage_menu_sheet.dart` (`GearManageMenuSheet` +
      `GearManageAction`) on `PollManageMenuSheet`, carrying **item-level** owner
      actions only: Set Details (edit), Switch mode (preserve current capability),
      Lending log / Log past, View impact (gated on `timesLoaned > 0` for loans),
      Mark returned & close (destructive). Dispatch in `GearContentView`
      (`_showManageSheet`), mirroring request.
- [ ] Keep the **debounced metadata auto-save** on the specs card in edit mode,
      but route it through a `GearNotifier` method (not a direct
      `gearRepositoryProvider` call from the widget — see Open Questions); extract a
      `GearEditPane` for title/description + `ContentEditBar`.
- [ ] **l10n:** source every new user-facing string (sticky-action verbs, status
      pill, queue/holder labels, manage-sheet rows, hand-off labels) from
      `app/lib/l10n/app_*.arb` — no hardcoded strings. Reuse existing `gear*` ARB
      keys where they fit (e.g. `gearBorrower*`); fold in the nearby hardcoded
      strings the legacy code left behind (`'Replace Media'`, `'Add Media'`,
      `'Failed to save'`) while you're in the file.

**Validates:** Every existing loan + giveaway transition is still reachable; the
overflow sheet matches request/experience chrome; impact gating is preserved; no
hardcoded user-facing copy.

### Phase 4: Retire legacy code, tests, docs, size gate

**Goal:** Remove the dead tab/menu/modal paths and lock in the new structure.

**Tasks:**
- [ ] Delete the old read path: `gear_bottom_content.dart`, `gear_type_pane.dart`,
      `gear_details_pane.dart`, `gear_actions_section.dart`, and the gear usage of
      `ContentViewTabBar`/`ContentViewPaneSwitcher`. Remove `GearMenuItems`
      checklist builders (and prune now-unused `checklist_status_icons.dart`
      helpers if no other caller remains — grep first).
- [ ] Ensure every new Dart file (and the slimmed `gear_content_view.dart`) is
      **under 1,000 lines** (`npm run lint:dart:size`); the shell + panels split
      should bring the 945-line content view well under.
- [ ] Add/port widget + view-model tests: the four state renders, the sticky-verb
      computation, manage-sheet dispatch, and morph-panel open/close. Follow the
      disposal-safety test convention for any new async notifier work.
- [ ] Update [docs/client/content.md](../../docs/client/content.md) (gear is no
      longer tab-based; document the read shell + morph panels) and
      [docs/client/manage_menus.md](../../docs/client/manage_menus.md) (gear now
      uses the flat sheet; remove/retire the "legacy checklist" section). Refresh
      freshness stamps. Expect the doc-freshness ratchet to touch governed docs.
- [ ] Keep the migration's phasing **out of Dart comments** — "Phase 1/2/…",
      "new in this PR", and similar ephemeral markers fail
      `npm run lint:ephemeral-comments`
      ([conventions.md](../../docs/client/conventions.md) → Dart comments). That
      context lives in this plan, not the code. Issue numbers (`#2509`) are fine.
- [ ] Place tests under `app/test/` mirroring `app/lib/` (view-model tests in
      `app/test/presentation/viewmodels/`, widget tests alongside the shell/panels);
      run with `flutter test --reporter expanded` / `npm run test:app`.
- [ ] Run `npm run generate`, `flutter analyze`, `npm run lint`, and the Flutter
      test suite locally before opening the PR.

**Validates:** No dead code; CI size + lint + context-map gates pass; docs match
the new structure.

## Related Issues
- #2278 — Experience content redesign (read-shell precedent; closest template).
- #2293 — Request content morph UX (read-shell + manage-sheet precedent).
- #2280 — "Pitching in" expand (morph-panel-from-card precedent for the
  Who's-using-it expand).
- #2291 — Location morph panel (precedent for the Where card → panel).
- *(to file)* — Real borrow queue / waitlist for loans (server-led follow-up; the
  "Get in line" / "Next up" / auto-nudge behaviour deferred from this issue).

---

## Audit Notes

Audited against the documentation context map (client-only change; engaged
dimensions: code_quality, accessibility, testing, tech_debt). Changes applied:

1. **Architecture compliance (widget → repository violation).** Flagged that the
   existing debounced metadata auto-save calls `gearRepositoryProvider` directly
   from the widget — a breach of "ViewModels are the ONLY layer that calls
   Repositories". Plan now requires routing it through a `GearNotifier` method
   during the migration (Open Questions + Phase 3).
2. **Accessibility.** Added an explicit Phase-2 task: l10n-sourced `semanticsLabel`
   on all new interactive surfaces via `Tappable`/`IconAction`, `LiveRegion`
   announcements for holder/queue changes, ≥48dp hit targets, status not conveyed
   by colour alone, and running the `scripts/check_a11y.js` CI gate.
3. **l10n / no hardcoded strings.** Added a Phase-3 task to source all new
   user-facing copy from ARB and to fold in the legacy hardcoded media-picker /
   save strings.
4. **Comment hygiene.** Added a Phase-4 reminder that the plan's phase markers
   must not leak into Dart comments (`npm run lint:ephemeral-comments`).
5. **Testing convention.** Pinned test locations under `app/test/` and the
   `--reporter expanded` requirement.

No proto/API, server, or security dimensions are engaged (pure client UI
redesign, no contract change). The plan's deliberate decision to render the mock's
queue from existing transfer data (deferring a real waitlist to a server-led
follow-up) is sound and avoids scope creep into the transfer state machine.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `proto_conventions.md` (confirmed not engaged); leaf docs
`client/content.md`, `client/manage_menus.md`, `client/accessibility.md`; review
rubrics `docs/reviews/accessibility_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`; reference plans
`docs/issues/2278-experience-content-redesign.md`,
`docs/issues/2293-request-content-morph-ux.md`.
