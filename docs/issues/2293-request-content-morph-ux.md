# Issue #2293: Apply the redesigned experienceContentView UX to requestContentView

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2293
**Filed:** 2026-06-05 | **Last activity:** 2026-06-05
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The Experience read surface was redesigned around a **morph-reveal read shell**
([experience_read_shell.dart](app/lib/presentation/screens/experience/widgets/experience_read_shell.dart)):
a bottom-anchored, non-scrolling overlay on top of the hero image whose cards
(Where / When / discussion / roster) grow into full-screen panels from their own
`Rect` footprint instead of opening stacked bottom sheets. The redesign also
introduced an always-on top-right `···` overflow, a top-bar boxed photo
attribution line, inline pickers (date/time, location, lettered-pin map), and a
roster screen whose `···` opens the access/share sheet.

The Request read surface
([request_content_view.dart](app/lib/presentation/screens/request/request_content_view.dart),
918 lines) has **already adopted much of the shared component library** —
`PhotoAttributionLine`, `AccessSheet`, `ContentInfoRow`/`ContentTopRows`,
`RowCallToActionPill`, `RequestManageMenuSheet` (built on the generic
`PollManageMenuSheet`), and the shared `LocationPickerView`. What it has **not**
adopted is the redesign's interaction *architecture*:

- It still uses a **two-tab layout** (Request / Discuss) with a tab bar and a
  `ContentViewPaneSwitcher`, rather than a single read shell whose cards
  morph-reveal into panels.
- Expandable surfaces (access, helper list, manage menu, location picker, media
  picker) open as **stacked bottom sheets / modals**, not morph-reveal panels.
- The owner manage menu is a **pencil icon** gated to owner + non-terminal,
  rather than an **always-on top-right `···`**.
- Photo attribution sits **above the tab bar at the bottom**, not in the
  **top-bar with the boxed pill**.
- The location picker opens as a **modal** rather than **inline** in a panel.

The goal is to converge the two surfaces on the same mechanics where they map
cleanly, while respecting that a Request has **no time poll, no location poll,
and no RSVP roster** — so several Experience panels (When, time/location polls,
RSVP grouping) have no Request equivalent. The shareable parts are the
*mechanics* (morph-reveal launcher/route/panel, overflow menu, attribution,
inline pickers, access flow), not the Experience-specific content.

**Ambiguity to resolve up front:** the issue checklist reads as *incremental*
("adopt the morph-reveal panel pattern … where applicable"), but true
consistency with the Experience read shell implies a larger architectural
shift away from the Request tab layout. The central recommendation below
addresses how far to go. This is the one decision that should be confirmed
before coding.

## Context from Comments

**There are no comments on this issue.** All direction comes from the
(detailed) issue body, which itself defers to this `/ripls-issue` planning step.
The body is unusually thorough — it enumerates the five redesign patterns, names
the exact shared helpers to reuse (`experience_settings_menu_items.dart`,
`PollManageMenuSheet`, `PhotoAttributionLine` with `boxed: true`), and flags the
Request-specific caveats (no poll, no roster) — so the lack of discussion is not
a blocker. The main thing the body does *not* settle is the architecture
question above.

## Codebase Findings (research summary)

What is **already shared and reusable as-is** (no extraction needed):

| Mechanic | Location | Status |
|---|---|---|
| Morph-reveal launcher | `openExperienceContentPanel()` — [experience_content_panel_launcher.dart](app/lib/presentation/screens/experience/widgets/experience_content_panel_launcher.dart) | Generic *behaviour*, but named/typed to "experience" (sets `experienceContentExpandedProvider`). Needs a rename/generalize to share. |
| Morph route | `morphRevealRoute<T>()` — [morph_reveal_route.dart](app/lib/presentation/widgets/content/morph_reveal_route.dart) | Fully generic. |
| Panel chrome | `ContentMorphPanel` — [content_morph_panel.dart](app/lib/presentation/widgets/content/content_morph_panel.dart) | Fully generic (scrim + swipe-to-close). |
| Manage menu container | `PollManageMenuSheet<T>` — [poll_manage_menu_sheet.dart](app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart) | Fully generic; **Request already uses it** in [request_manage_menu_sheet.dart](app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart). |
| Settings menu items helper | `experienceSettingsMenuItems<T>()` — [experience_settings_menu_items.dart](app/lib/presentation/screens/experience/widgets/experience_settings_menu_items.dart) | Generic over `T`; ARB keys are Experience-worded. |
| Photo attribution | `PhotoAttributionLine(boxed: true)` — [photo_attribution_line.dart](app/lib/presentation/widgets/content/photo_attribution_line.dart) | Fully generic; **Request already renders it** (un-boxed, bottom). |
| Access/share sheet | `AccessSheet` — [access_sheet.dart](app/lib/presentation/widgets/content/access_sheet.dart) | Fully generic; **Request already uses it**. |
| Inline location picker | `LocationPickerView` — [location_picker_modal.dart](app/lib/presentation/widgets/location/location_picker_modal.dart) | Callback-driven, embeds inline; **Request uses the modal form today**. |
| Lettered-pin map | `LocationPinRenderer` / `LocationPinCache` — [location_pin_renderer.dart](app/lib/core/utils/location_pin_renderer.dart) | Pure utilities. |

What is **Experience-specific** (would need a Request twin or extraction):

- `openExperienceContentPanel` — couples to `experienceContentExpandedProvider`
  + `HomeNotifier.lockNav()`. Generalizing means a content-type-agnostic
  launcher (e.g. `openContentMorphPanel({expandedProvider, ...})`).
- `ExperienceManageMenuSheet` — Request already has its own twin
  (`RequestManageMenuSheet`). The remaining work is making it **always-on** and
  **top-right**, not changing the sheet itself.
- `ExperiencePitchingInScreen` (roster + needs morph panel) — Request's nearest
  analog is the **offerers** list, today shown via `HelperListSheet` (a bottom
  sheet). A `RequestHelpersPanel` would be the morph-reveal twin.
- `experience_read_shell.dart` itself — the bottom-anchored card stack. Request
  uses `RequestBottomContent` + tabs instead.

Request-only domain (drives which panels port):

- `offerers` (helpers) instead of an RSVP roster; `offerToFulfill()` /
  `withdrawOffer()` ([request_edit_actions.dart](app/lib/presentation/screens/request/request_edit_actions.dart)).
- States `ACTIVE / OFFERS_RECEIVED / FULFILLED / CANCELLED`; fulfillment via
  `MarkFulfilledModal` + optional `resolutionSummary`.
- Optional single location, **no time**, **no polls**.

## Potential Solutions

### Option A: Incremental overlay — keep tabs, add the redesign chrome
- **Approach:** Leave the two-tab `RequestBottomContent` architecture in place.
  Layer on the four chrome/mechanic changes that don't require restructuring:
  (1) move `PhotoAttributionLine` to the **top-bar with `boxed: true`**;
  (2) replace the title pencil with an **always-on top-right `···`** that
  combines manage + context actions (reusing `RequestManageMenuSheet` content,
  gated like the Experience overflow); (3) convert the **location picker** and
  **helper list** from bottom sheets to **morph-reveal panels** via a
  generalized launcher; (4) move the **access sheet entry** into the helpers
  panel's `···` (roster-overflow → access pattern).
- **Pros:** Lowest risk; ships the visible consistency wins (attribution, `···`,
  morph panels) without rewriting the read surface; each bullet is independently
  shippable; no behavioural change to the Discuss tab.
- **Cons:** Request keeps tabs while Experience uses a single read shell, so the
  two surfaces still differ at the top level — partial convergence. The
  "morph-reveal in place of stacked modals" is applied to pickers/lists, not to
  a card-grid read shell.
- **Effort:** Medium.

### Option B: Full read-shell adoption — replace tabs with a Request read shell
- **Approach:** Build a `RequestReadShell` mirroring `ExperienceReadShell`: a
  bottom-anchored, non-scrolling overlay over the hero with cards (discussion,
  Where, owner/needs, helpers). Each card morph-reveals into a panel
  (`RequestDiscussionPanel`, `RequestLocationPanel` with inline picker,
  `RequestHelpersPanel`). Drop the tab bar / `ContentViewPaneSwitcher`. Adopt the
  always-on `···`, top-bar boxed attribution, and roster-overflow → access.
- **Pros:** True convergence — the two read surfaces share one architecture and
  largely one widget set; future redesigns touch both at once; fully realizes
  the issue's "feel consistent" goal.
- **Cons:** Largest change; rewrites the Request read layout and its
  discussion/chat surface (today an inline `InlineConversationView` in a tab, vs.
  a morph discussion card). Higher regression risk across feed embeds
  (`showFeedHeader`, `feedActor`, etc. params on `RequestContentView`). Needs
  careful handling of edit mode (`ContentEditBar`, media picker overlay).
- **Effort:** Large.

### Option C: Hybrid — generalize shared mechanics first, then read-shell behind a flag
- **Approach:** Phase 1 extracts/generalizes the shared mechanics
  (content-agnostic morph-panel launcher, shared settings-menu wording, top-bar
  attribution + overflow scaffold) and applies the *low-risk* chrome to the
  existing tab layout (Option A's wins). Phase 2 introduces `RequestReadShell`
  as the read surface (Option B) reusing those now-generalized mechanics, behind
  a feature flag so it can bake against the live tab layout before cutover.
- **Pros:** Captures Option A's quick wins immediately while keeping the
  Option B end-state on the table; the extraction work is reused either way, so
  no throwaway effort; the flag de-risks the big cutover.
- **Cons:** Two-step delivery is more coordination; carrying both the tab layout
  and the read shell briefly adds surface area; needs a flag + cleanup TODO.
- **Effort:** Large (spread across phases; each phase shippable).

## Recommendation

**Option C (hybrid), executed so that Phase 1–3 deliver Option A's value and
Phase 4 is an explicitly-gated decision point on whether to go all the way to
the read shell.**

Reasoning:

1. The Request view has *already* absorbed the shared component library, so the
   remaining gap is mostly **mechanics + chrome placement**, not new widgets.
   That makes the Option A wins cheap and high-value — do them first.
2. The one piece of genuinely reusable-but-not-yet-shared plumbing is the
   **morph-panel launcher** (`openExperienceContentPanel` is hard-coded to
   Experience state). Generalizing it is required for *any* morph-reveal on
   Request and pays off for both surfaces — so it belongs in Phase 1 regardless
   of how far we go.
3. Whether to replace the Request tabs with a full read shell (Option B) is a
   product-level call about the Request surface's information architecture, not
   just a refactor. Sequencing it as a flagged Phase 4 lets us ship consistency
   wins now and make the big-cutover decision with the generalized mechanics
   already in hand — no rework either way.

**Confirmed by the issue owner (2026-06-05):** proceed with the hybrid path —
Phases 1–3 first, Phase 4 (read shell) as a flagged, gated decision. If the
Phase 4 spike shows full read-shell parity is worth it, collapse it into the
mainline and remove the flag; otherwise stop after the helpers panel and keep the
tab layout. All Open Questions below are likewise settled.

**Update (executed):** the owner subsequently directed the full read-shell
cutover with **no feature flag**. Implemented: `RequestReadShell` (bottom-anchored
sheet over the hero — headline + discussion card + location/owner/needs/helpers
row stack + top-bar attribution/overflow + mute), with the discussion card
morph-expanding into `RequestConversationPanel` (hosting `RequestChatPane`). The
tabbed layout (`RequestBottomContent`, the tab bar / pane switcher) and the
standalone access pill were removed; edit mode now uses `RequestEditPane` +
`ContentEditBar`. `HelperListSheet` was deleted. The `initialTab` deep-link
("chat") now opens the conversation panel.

**Update (carded parity with experience):** to match the experience content
surface, `RequestReadShell` now renders the redesigned **cards** rather than flat
info rows — the location is a full-width **WHERE** `ContentFactsRow` card
(morph-expands the location panel) and participation is a **WHO'S IN**
`ContentEdgesCard` (sage check-circle rows for the requester (HOST) and each
offerer, contributions shown inline, a "{n} helping" header count, a footer Help
CTA, and tap-to-expand the helpers panel). The standalone location / owner /
needs / offer rows were removed; needs **management** moved into
`RequestHelpersPanel` as a "Needed · N open" chip section + "We need" / "I'll
bring" actions (reusing `NeedsActions` / `ContentNeedChip`), mirroring the
experience pitching-in panel. One request-specific a11y key (`a11yReqAddNeed`)
was added in both locales. The vestigial `activeTab` / `chatTabIndex` /
`setActiveTab` viewmodel members were **removed** (freezed regenerated).

## Open Questions

**All recommendations below were confirmed by the issue owner (2026-06-05) and
are now decisions.** They are retained with reasoning so the contract stays
auditable; the phases already reflect them.

- **How far should convergence go — chrome-only (Option A) or full read-shell
  (Option B)?** — **Decided:** hybrid path. Ship Phases 1–3 first (valuable and
  low-risk under either answer; they generalize the mechanics Phase 4 needs);
  treat Phase 4 (read shell) as a gated decision made with real code in hand.
- **Should the location picker become inline-in-a-panel, given Request has no
  When/poll panel to host it?** — **Decided:** yes — create a minimal
  `RequestLocationPanel` (morph-reveal) that embeds `LocationPickerView` inline
  for the owner and shows the lettered-pin `LocationMapPreview` + directions for
  everyone, mirroring `ExperienceLocationPanel` minus the poll/options UI. It
  reuses the inline picker and pin renderer directly.
- **What populates the always-on `···` for a Request, and how is it gated?** —
  **Decided:** reuse `RequestManageMenuSheet`'s items (Edit Details / Mark
  Fulfilled / Close Request / View Impact) for the owner; for non-owners show
  only context actions (Share / Report) or omit the menu. Gate settings items in
  terminal states (`FULFILLED` / `CANCELLED`) the same way Experience hides
  settings while a poll is open. Keep it always-rendered for the owner so the
  affordance is stable.
- **Should `HelperListSheet` become a morph-reveal `RequestHelpersPanel`, and
  should access move into its `···`?** — **Decided:** yes, for parity with the
  roster-overflow → access pattern, but this is the highest-churn Request bullet
  (offer/withdraw actions live in the sheet today). Do it in its own phase so it
  can be reverted independently if offer flows regress.
- **Where does the generalized launcher live and what is it named?** —
  **Decided:** move it to `app/lib/presentation/widgets/content/` as
  `openContentMorphPanel({required expandedProvider, ...})` taking the
  content-type's expanded-state provider as a parameter; have the Experience
  call sites pass `experienceContentExpandedProvider`. Avoids a
  Request-specific fork of identical plumbing.
- **Does edit mode survive a read-shell cutover (Phase 4)?** — **Decided
  (pending Phase 4 spike confirmation):** keep the existing `ContentEditBar` +
  `MediaPickerButton` overlay model; the read shell governs read mode only, and
  edit mode already overlays the whole view. Validate in the Phase 4 spike before
  committing the cutover.

## Architecture & Compliance Audit (`/ripls-audit`)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. This is a **client-only**
change with **no new RPCs, protos, or server code**, so the server boundary is
respected and the server observability/logging conventions don't apply. Findings
(folded into the phases above):

- **Testing philosophy (was non-compliant).** The first draft used widget-test
  assertions as the validation signal. [client/testing.md](docs/client/testing.md)
  is explicit: *test ViewModels, not widgets; don't test UI and logic together.*
  All gating/capability logic (owner-vs-non-owner, terminal-state gating,
  offer/withdraw) is now specified to live where a `ProviderContainer` + Mockito
  ViewModel test can assert it. Widget tests are demoted to thin smoke checks.
  Note there are **no golden tests** in this repo (listed under "weaknesses"), so
  the plan doesn't lean on visual-regression coverage.
- **i18n parity + terminology (was missing).** Every new string (overflow items,
  attribution, inline pickers) needs a key in **both** `app_en.arb` and
  `app_es.arb` — `npm run lint:dart:l10n-parity` fails otherwise. User-facing
  copy follows the **"Event"/"Request", never "Experience"** rule from CLAUDE.md;
  the Experience-worded `experienceSettingsMenuItems` ARB keys ("Mark Completed",
  "Close Event") must not bleed into Request copy (Request already says "Mark
  Fulfilled" / "Close Request"). Viewmodels never resolve strings — widgets
  resolve in `build()`.
- **Accessibility (was missing).** The empty, ratcheting a11y allowlist means any
  new violation must be fixed in-PR. The always-on `···` is icon-only → use
  `IconAction` with an l10n `semanticsLabel` + `tooltip`; bottom sheets open via
  `showAccessibleModal`; taps use `Tappable`; morph animations route through
  `accessibleDuration` (the shared `morphRevealRoute` already does). Keep
  `npm run lint:dart:a11y` green.
- **Navigation lint (clarified).** `openContentMorphPanel` pushes a
  `morphRevealRoute` (a custom `PageRouteBuilder`) on the root navigator. This is
  the *existing intentional exception* the Experience launcher already relies on
  — generalizing it must not regress `npm run lint:nav`, and must not substitute
  a raw `Navigator.push(MaterialPageRoute(...))`.
- **State management (clarified).** `requestContentExpandedProvider` must be a
  real Riverpod provider (family by `requestId`), mirroring the Experience one —
  not widget `setState`. Async actions keep `ref.mounted` guards per the
  async-safety rules.
- **Dart file-size gate (added).** [request_content_view.dart](app/lib/presentation/screens/request/request_content_view.dart)
  is already 918 lines; the top-bar/overflow and any new panels must be extracted
  into their own widget files to stay under the 1,000-line `npm run lint:dart:size`
  gate, without the `// dart-line-count-allow` hatch.
- **Caching (no change needed).** The plan reuses existing repositories/actions
  (`updateLocation`, `offerToFulfill`, `withdrawOffer`, mark-fulfilled) which
  already own their cache invalidation; this is a presentation-layer refactor that
  introduces no new data access, so the caching conventions are untouched.

## Implementation Plan

### Phase 1: Generalize the morph-panel mechanics
**Goal:** Make the morph-reveal launcher and settings-menu wording
content-type-agnostic so Request can drive panels without forking Experience
plumbing. No user-visible change yet.
**Tasks:**
- [ ] Move/rename `openExperienceContentPanel` → `openContentMorphPanel` in
      `app/lib/presentation/widgets/content/`, taking the expanded-state provider
      (and nav lock) as parameters; update Experience call sites to pass
      `experienceContentExpandedProvider`.
- [ ] Add a `requestContentExpandedProvider` (mirror of the Experience one) so
      the Request read surface can hide while a panel is open.
- [ ] Confirm `experienceSettingsMenuItems<T>()` generalizes; if Request needs
      different wording, add `requestSettingsMenuItems<T>()` reusing
      `PollManageMenuItem` (don't duplicate the sheet). **i18n:** any new
      menu-item strings get ARB keys in **both** `app_en.arb` and `app_es.arb`
      (parity gate `npm run lint:dart:l10n-parity`); user-facing wording uses
      **"Event"/"Request"**, never "Experience" (see CLAUDE.md terminology rule).
- [ ] **Riverpod compliance:** `requestContentExpandedProvider` is a real
      Riverpod provider (family keyed by `requestId`, mirroring the Experience
      one), not widget `setState`. The generalized launcher keeps its
      `morphRevealRoute` push on the root navigator — this is the existing,
      intentional exception to the `NavigationHelpers`/`lint:nav` rule that the
      Experience launcher already relies on; do **not** introduce a raw
      `Navigator.push(MaterialPageRoute(...))`. Confirm `npm run lint:nav` still
      passes after the rename.
- [ ] `flutter analyze` + `npm run generate` (regenerates l10n; no proto
      changes). Re-run existing Experience **ViewModel** tests to prove the
      rename didn't change behaviour.
**Validates:** The launcher and menu helpers work for an arbitrary content type;
Experience behaviour is unchanged after the rename. (Per
[client/testing.md](docs/client/testing.md), the rename is a pure refactor —
ViewModel tests, not widget tests, are the regression signal.)

### Phase 2: Top-bar attribution + always-on `···` overflow on Request
**Goal:** Bring the Experience top-bar chrome to the Request view while keeping
the existing tab layout.
**Tasks:**
- [ ] Move `PhotoAttributionLine` from above the tab bar (in
      `request_bottom_content.dart`) to a **top-bar position**, rendered with
      `boxed: true`, matching `ExperienceReadShell._topBar`.
- [ ] Replace the title pencil with an **always-on top-right `···`** for the
      owner that opens `RequestManageMenuSheet`; place attribution to its left.
- [ ] Gate settings items in terminal states (`FULFILLED`/`CANCELLED`),
      analogous to the poll-open gating on Experience.
- [ ] Decide/implement the non-owner overflow contents (Share/Report or none).
- [ ] **Accessibility:** the icon-only `···` uses `IconAction` (not raw
      `IconButton`/`GestureDetector`) with a localized `semanticsLabel` +
      `tooltip` from `context.l10n` (`a11y*` key, both locales); the manage menu
      opens via `showAccessibleModal` so focus restores on dismiss. Keep
      `npm run lint:dart:a11y` green (allowlist is empty and ratchets — fix any
      new violation in-PR).
- [ ] **File size:** extract the new top-bar (attribution + overflow) into its
      own widget so [request_content_view.dart](app/lib/presentation/screens/request/request_content_view.dart)
      (already 918 lines) stays under the 1,000-line `npm run lint:dart:size`
      gate — do **not** add the hatch comment.
- [ ] `flutter analyze`. **Testing:** put the owner-vs-non-owner and
      terminal-state **gating logic** where a ViewModel/`ProviderContainer` test
      can assert it (e.g. a derived "available manage actions" getter on request
      state), per the ViewModel-first rule in
      [client/testing.md](docs/client/testing.md); avoid making a widget test the
      primary signal for gating.
**Validates:** The two surfaces share the same top-bar affordance and
attribution treatment; manage-action availability is correctly gated and
unit-testable without a widget tree.

### Phase 3: Morph-reveal location panel with inline picker
**Goal:** Replace the modal location picker with a morph-reveal
`RequestLocationPanel` that inlines `LocationPickerView` (owner) and shows the
lettered-pin map + directions (everyone).
**Tasks:**
- [ ] Create `RequestLocationPanel` (wrap child in `ContentMorphPanel`) modeled
      on `ExperienceLocationPanel` minus poll/options UI.
- [ ] Embed `LocationPickerView` inline (callback-driven `onSaved` → existing
      `updateLocation` ViewModel action) and `LocationMapPreview` with
      `LocationPinCache`. **A11y:** the morph route already routes its duration
      through `accessibleDuration`; the panel close control uses an a11y
      primitive (`IconAction`/`Tappable`) with an l10n label; any inline picker
      strings come from `context.l10n` (both locales).
- [ ] Launch it from the Request location row via `openContentMorphPanel` with
      the row's source `Rect`; remove the `LocationPickerHelper.showLocationPicker`
      modal path on this surface.
- [ ] `flutter analyze`. **Testing:** the owner-vs-non-owner *capability*
      (can-edit-location) is a request-state predicate — assert it in a
      `ProviderContainer` ViewModel test; reserve any widget check for a thin
      smoke pass, not the primary signal.
**Validates:** Morph-reveal + inline picker works end-to-end on Request; the
location row grows from its footprint instead of opening a modal sheet, and the
edit-capability gate is unit-tested.

### Phase 4 (gated): Helpers panel + access-from-overflow, and read-shell decision
**Goal:** Port the roster-overflow → access pattern to Request's offerers, and
make the explicit go/no-go call on full read-shell adoption.
**Tasks:**
- [ ] Build `RequestHelpersPanel` (morph-reveal twin of `HelperListSheet`)
      showing offerers + offer/withdraw actions, with a `···` (`IconAction`,
      l10n label) that opens `AccessSheet` via `showAccessibleModal` (replacing
      the standalone access-pill entry where it makes sense). Keep
      `HelperListSheet` until the panel is proven, then remove it.
- [ ] Spike `RequestReadShell` (Option B) reusing the now-generalized mechanics;
      put it behind a feature flag and validate feed embeds + edit mode. New
      panels must each stay under the 1,000-line `lint:dart:size` gate.
- [ ] Decision checkpoint: ship the read shell to replace tabs, or stop at the
      chrome/panel wins (Phases 1–3 + helpers panel). If shipping, file a cleanup
      issue + `TODO(#NNNN):` for removing the tab layout and flag (per CLAUDE.md
      TODO rule — every TODO references an open issue).
- [ ] `flutter analyze`. **Testing (ViewModel-first):** offer/withdraw flows go
      through the existing request actions/ViewModel — add `ProviderContainer` +
      Mockito tests covering offer → state, withdraw → state, and the error/
      disposal cases (`ref.mounted`), per [client/testing.md](docs/client/testing.md).
      Run `npm run lint:dart:size`, `lint:dart:a11y`, `lint:dart:l10n-parity`,
      `lint:nav` before pushing.
**Validates:** Whether the offerer/access flow survives the morph-panel form, and
whether full read-shell parity is worth the cutover — with real code to judge,
not a guess.

## Related Issues
- #2175 — referenced in research as the change that dropped the Request "plan"
  tab (Request is now two tabs: Request / Discuss). Relevant because Phase 4's
  read-shell cutover would remove the remaining tab bar entirely.
- The Experience redesign work that established `experience_read_shell.dart`,
  `ContentMorphPanel`, `morphRevealRoute`, and the shared `content/` widgets —
  this issue is the Request-side follow-through to that effort.
