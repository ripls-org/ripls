# Issue #2291: Expand the experience location widget into a full-screen morph-reveal panel (map + inline poll voting)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2291
**Filed:** 2026-06-04 | **Last activity:** 2026-06-04
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/client/modals.md](../client/modals.md), [docs/client/geospatial.md](../client/geospatial.md)

## Problem Statement

Tapping the WHERE (location) widget on the experience content view currently opens a **glass bottom-sheet** — one of four state-dependent location-poll modals (`LocationPollProposeModal`, `LocationPollVoteModal`, `LocationPollConfirmModal`, `LocationPollFinalizedModal`), dispatched from `_showLocationPickerModal()` in [experience_content_view.dart:877-940](../../app/lib/presentation/screens/experience/experience_content_view.dart#L877-L940). None of these sheets shows a map.

The two sibling widgets on the same content view — the **discussion** card and the **"Who's pitching in?"** roster — were already migrated to a different, now-canonical surface: a **morph-reveal content panel** that grows from the tapped card's footprint, overlays the still-playing hero behind a 70%-black scrim, and replaces the rest of the content view (docs/client/modals.md § *Morph-reveal content panels*). The location widget is the odd one out.

This issue asks to bring the location widget onto that same surface and, while doing so, make it genuinely useful:

1. **Surface change:** tapping WHERE morph-expands into a full-screen, semi-transparent panel (identical animation/scrim/dismiss to discussion & Who's In) — not a bottom sheet.
2. **Embedded map:** the panel shows a real map with the phone's blue dot plus either the single confirmed/selected location, or **all proposed locations as markers** when a poll is running.
3. **Inline voting:** attendees vote on poll options directly in the panel (auto-submit on tap), with no secondary modal.
4. **Owner controls:** the owner gets buttons to **pick a different spot** and to **add spots and ask the group** (start/extend a poll).

This is fundamentally a **presentation/surface change**, not a new feature: the location-poll domain (proto, RPCs, view-model, optimistic voting) already exists end-to-end and ships today behind the bottom sheets. The work is re-housing that logic on the morph surface plus adding the map.

No contradictions in the issue itself; it is internally consistent and well-scoped. The one ambiguity it explicitly defers to implementation is whether the existing bottom-sheet modals' internals can be **reused inside the panel body** or should be **retired for this entry point** (see Open Questions).

## Context from Comments

The issue has **no comments** — it was filed and immediately picked up for planning. All requirements come from the issue body, which is unusually detailed (it already names the exact files, line ranges, RPCs, and building blocks). No maintainer debate or clarifications exist yet, so the Open Questions below carry more weight than usual: they are the decisions a reviewer should confirm before implementation, not points already settled in discussion.

## Research Findings (codebase)

**The morph surface and its launcher are shared and ready to reuse:**
- [`openExperienceContentPanel(...)`](../../app/lib/presentation/screens/experience/widgets/experience_content_panel_launcher.dart#L23-L54) does all the bookkeeping: sets `experienceContentExpandedProvider` (hides the read shell), `HomeNotifier.lockNav()`, pushes `morphRevealRoute` on the root navigator with `scrimMaxOpacity: 0`, and restores shell + nav on pop.
- [`ContentMorphPanel`](../../app/lib/presentation/widgets/content/content_morph_panel.dart) supplies the scrim + horizontal swipe-to-close chrome; panel bodies wrap their content in it and add their own close button. See [ExperienceConversationPanel](../../app/lib/presentation/screens/experience/widgets/experience_conversation_panel.dart) (tiny — just a `Stack` + close ✕) and [ExperiencePitchingInScreen](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart) (richer — header + scrolling body) as the two reference panels.

**The Rect-capture pattern differs for the location card — this is the main novelty:**
- Discussion/edges cards are **standalone** widgets that own their own `Tappable` and capture their `RenderBox` footprint at tap time (see [content_discussion_card.dart:200-213](../../app/lib/presentation/widgets/content/content_discussion_card.dart#L200-L213)), handing a `Rect` up via `ValueChanged<Rect> onTap`.
- The WHERE card is **not** standalone. It is a `ContentFactData` rendered inside a shared two-up [`ContentFactsRow`](../../app/lib/presentation/widgets/content/content_facts_row.dart) alongside the WHEN card. Its `onTap` is a plain `VoidCallback?` ([content_facts_row.dart:59](../../app/lib/presentation/widgets/content/content_facts_row.dart#L59)) and its CTA chip's `onTap` is a `VoidCallback` ([:16](../../app/lib/presentation/widgets/content/content_facts_row.dart#L16)). The read shell wires all three (card tap, CTA tap, semantics) to the same `onShowLocation` `VoidCallback` ([experience_read_shell.dart:50, 226, 238, 250](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart#L226-L250)), which the content view binds to `_showLocationPickerModal(...)` ([experience_content_view.dart:514](../../app/lib/presentation/screens/experience/experience_content_view.dart#L514)).
- So to morph from **just the WHERE card's footprint** (not the whole row), we must plumb a `Rect` through `ContentFactsRow` → `ContentFactData`/`ContentFactCta`, capturing the individual card's `RenderBox` at tap time. `onExpandConversation` is already a `ValueChanged<Rect>` on the read shell ([:44](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart#L44)), so there is an established precedent for the signature; `onShowLocation` would change from `VoidCallback` to `ValueChanged<Rect>`. (The WHEN/time card has the identical shape and would benefit from the same plumbing later — design the Rect hook generically.)

**The voting logic, view-model, and optimistic UI already exist and are surface-agnostic:**
- [`locationModalProvider`](../../app/lib/presentation/viewmodels/location_modal_view_model.dart) (`AsyncNotifier<LocationModalData>`) exposes everything the panel needs: `proposals`, `currentUserYesProposalIds`, `currentLocationPollId`, `locationPollActive/Completed`, `isOrganizer`, `eventLocationId`, plus mutations `voteOnLocation` (optimistic, auto-submit), `setFlexibleOnLocation`, `proposeSavedLocation`, `proposeGeocodedLocation`, `setSingleLocation` (propose+confirm for the "pick a different spot" path), `confirmLocation`, `endLocationPoll`, `setLocationPollDeadline`, `setLocationProposalsLocked`, `nudgeUnreplied`.
- The auto-submit-on-tap voting row + supporting atoms (`_VoteLocRow`, `_Checkbox`, `_VoterStack`, `_RowContent`, the vote counter, the leading-badge logic, `PollFlexibleVoteRow`, `AddSpotCard`) all live **privately** inside [location_poll_vote_modal.dart](../../app/lib/presentation/screens/experience/location_poll_vote_modal.dart). They are written against the **modal glass token family** (`AppColors.modalTextPrimary/Secondary`, `GlassSheet`), whereas morph panels render on **`AppColors.onContentImage` / `darkText*` over the media scrim**. Reusing them in the panel means **extracting** them into shared widgets and re-theming for the on-content surface.

**The map building blocks exist but are single-marker today:**
- [`LocationMapPreview`](../../app/lib/presentation/widgets/location/location_modal_widgets.dart#L23) takes a single `latitude`/`longitude`, renders **one** `selected_location` marker, supports `onMapTap`/`onCameraChanged`, smooth `animateCamera` on coordinate change, and lazy dark style. It does **not** accept a marker set, so showing **all poll proposals at once** needs either an extension (`Set<Marker>` input) or a dedicated multi-marker map view modeled on [`DiscoverMapHelper`](../../app/lib/core/utils/discover_map_helper.dart) patterns (docs/client/geospatial.md).
- The phone blue dot is `GoogleMap.myLocationEnabled` driven by a session-cached GPS fix, **no prompt on mount** (#1174) — read `userLocationProvider(precisePin)` rather than prompting.
- Per-proposal coordinates: a proposal carries either an inline `location.geocoded` (coords inline) or a saved `location.locationId` (resolve via `locationRepositoryProvider.get(id)`, as `_RowContent` already does). `LocationModalData` does not surface coordinates directly, so the panel/map layer must resolve them.

## Potential Solutions

### Option A: New dedicated `ExperienceLocationPanel`, reuse `locationModalProvider`, extract shared vote/map widgets
- **Approach:** Build a new panel widget wrapped in `ContentMorphPanel` (mirroring `ExperiencePitchingInScreen`). It watches `locationModalProvider(experienceId)` and renders: a map header (blue dot + single marker or all-proposal markers), an inline vote list (extracted from the vote modal and re-themed for the on-content dark surface), and an owner action area ("Pick a different spot", "Add spots & ask the group", plus the existing manage menu). Plumb a `Rect` through `ContentFactsRow`/`ContentFactData` so the WHERE card morphs from its own footprint; change `onShowLocation` to `ValueChanged<Rect>` and route it to `openExperienceContentPanel`. Extend `LocationMapPreview` to accept a marker set (or add a sibling multi-marker map). The existing bottom-sheet modals become **sub-flows** invoked from the panel only where a transient picker is genuinely warranted (e.g. the Google location picker for adding a spot), and the dead state-dispatch in `_showLocationPickerModal` is removed for this entry point.
- **Pros:** Matches the canonical surface and the issue's intent exactly. Reuses the entire poll domain (zero new RPC/view-model work). Map + voting + owner controls all live in one navigable surface, as asked. Re-theming during extraction is the right moment to delete the modal/morph token drift. Leaves time-card Rect plumbing trivially addable later.
- **Cons:** Largest surface area — touches `ContentFactsRow`, the read shell, the content view, the map widget, and requires extracting ~6 private widgets from the vote modal. Re-theming glass→on-content is fiddly. Multi-marker map is net-new.
- **Effort:** Large.

### Option B: Wrap the existing `LocationPollVoteModal` body in `ContentMorphPanel` (minimal extraction)
- **Approach:** Reuse the vote modal's `_buildLoaded` content almost verbatim inside a morph panel, bolt a map on top, and route the WHERE tap to the panel.
- **Pros:** Fastest path to "it's a morph panel with voting." Minimal new widget code.
- **Cons:** The vote-modal body is built for `GlassSheet` + modal tokens; dropped onto the media scrim it will look wrong (white-on-glass legibility deviation is scoped to the glass stack per docs/client/modals.md). It only covers the *active-poll* state — the confirmed/empty/owner-pick states still live in the other three modals, so we either re-create them or keep dispatching to bottom sheets, defeating the goal. Bolting a map above a sheet-shaped body fights the full-screen panel layout. Likely collapses back into Option A once the theming and non-poll states are handled.
- **Effort:** Medium, but with high rework risk.

### Option C: Keep bottom sheets, add a map header to each
- **Approach:** Leave the dispatch and four modals as-is; add a `LocationMapPreview` header to the vote/finalized sheets.
- **Pros:** Smallest change; gets a map in front of users quickly.
- **Cons:** Does **not** satisfy the issue — still a bottom sheet, still opens a separate modal to vote, no morph, inconsistent with discussion/Who's In. Explicitly contrary to the requested surface.
- **Effort:** Small — but wrong target.

## Recommendation

**Option A.** It is the only option that satisfies all four acceptance criteria and aligns with the documented standardization on the morph-reveal surface for content-view expansions. The poll domain (proto, RPCs, `locationModalProvider`, optimistic voting) is fully reusable, so the cost is concentrated in **presentation**: Rect plumbing, widget extraction + re-theming, and a multi-marker map — all tractable and all things the codebase already has close analogues for.

Phase the work so the surface lands first (a morphing panel that simply hosts today's content), then the map, then the inline voting and owner controls, then cleanup. This keeps each phase shippable and lets us validate the trickiest new piece — the per-card Rect morph from inside `ContentFactsRow` — before investing in map/voting UI.

The main judgement call a reviewer should confirm is the **fate of the four bottom-sheet modals** (Open Questions) — Option A assumes they are reused as narrow sub-flows where a transient picker fits and otherwise retired for this entry point, but they may also be reachable from other surfaces (e.g. a Plan tab) that must keep working.

## Open Questions

- **Do the four location-poll bottom sheets have callers other than `_showLocationPickerModal`?** — Recommendation: grep for `LocationPollVoteModal.show` / `…Propose…` / `…Confirm…` / `…Finalized….show` and `onShowLocationPickerModal` ([experience_content_view.dart:631](../../app/lib/presentation/screens/experience/experience_content_view.dart#L631)) before deleting anything. If they are only reached from this content-view entry point, retire them after extraction; if a Plan tab or banner also opens them, keep the sheets and have both surfaces share the extracted vote/propose widgets. Default to **extract-and-share, delete only confirmed-dead** code paths.
- **Single marker vs. multi-marker map — extend `LocationMapPreview` or add a sibling?** — Recommendation: extend `LocationMapPreview` with an optional `Set<Marker>` (falling back to its current single-marker behavior when absent) rather than forking a `DiscoverMapHelper`-based view, to avoid a second map-state implementation. If marker tap → "scroll to / highlight that proposal row" interactivity is wanted, that argues for a small dedicated helper; confirm whether marker interactivity is in scope for v1 (recommend: read-only markers in v1, tappable later).
- **Map placement & height within the full-screen panel.** — Recommendation: a fixed-height map header (≈40% of panel height) above a scrolling vote/controls list, matching the roster panel's header-over-list rhythm; full-bleed map with the morph/scrim handled by `ContentMorphPanel`. Confirm with design before pixel-polishing.
- **Owner "pick a different spot" while a poll is active.** `setSingleLocation` asserts no active poll ([location_modal_view_model.dart:353-356](../../app/lib/presentation/viewmodels/location_modal_view_model.dart#L353-L356)). — Recommendation: when a poll is active, "pick a different spot" should route through `confirmLocation` (owner picks a winner / overrides) or first `endLocationPoll`, not `setSingleLocation`. Mirror the existing manage-menu "Set final" path. Confirm the exact owner mental model with design.
- **Non-owner with no poll (read-only).** Today such users fall through to the read-only Google location picker ([:890-905](../../app/lib/presentation/screens/experience/experience_content_view.dart#L890-L905)). — Recommendation: in the panel, non-owners with a single confirmed spot see the map + address + a "Get directions" affordance (read-only), no voting UI; preserve directions parity with the old picker.
- **Terminology.** — Recommendation: audit every new user-facing string for "Event" not "Experience" (per CLAUDE.md), including the panel title, owner buttons, and any a11y/semantics labels.

## Audit Notes (architecture compliance)

> Added by `/ripls-audit` on 2026-06-04 against docs/client/{architecture,caching,i18n,accessibility,testing}.md and the two server docs. Changes below are folded into the phases/open questions; this section records *why*.

- **MVVM layering — fix an existing violation during extraction.** Architecture rule #1: *ViewModels are the only layer that calls Repositories; widgets never access repositories directly.* The vote modal's `_RowContent` resolves a saved location's name/address by calling `ref.read(locationRepositoryProvider).get(...)` inside a `FutureBuilder` **in the widget** ([location_poll_vote_modal.dart:722-723](../../app/lib/presentation/screens/experience/location_poll_vote_modal.dart#L722-L723)). The Phase 3 extraction must not copy that pattern onto the new surface — instead, resolve each proposal's display name/address **and coordinates** in the view-model (`LocationModalData`) so the panel/map read pre-resolved fields. This both fixes the layering breach and gives the map a single coordinate source (see next point).
- **Caching — resolve coordinates through the repository, never the service.** Per docs/client/caching.md, saved `Location` records are cached under the `'location'` namespace; the map markers must read coords via `locationRepositoryProvider` (cached) for saved proposals and from the inline `geocoded` payload for drop-pins — no direct `LocationService`/geocoder calls from the panel. Geocode/reverse-geocode pass-throughs are intentionally uncached, so don't add a per-marker geocode. The panel's `contentCacheInvalidationProvider` listener (Phase 1) mirrors the roster panel and is the correct freshness mechanism; do not add a manual cache map.
- **Accessibility — name the primitives, don't hand-roll.** Per docs/client/accessibility.md: close button → `IconAction` (requires `semanticsLabel` + `tooltip`); vote rows → `Toggle` with `selected:` for the voted state (the existing `_VoteLocRow` already does this — preserve it); status text (vote counter) → `LiveRegion` (already used); "Vote submitted" one-shot → `SemanticAnnouncer.announce`; any sub-sheet (the add-spot picker) → `showAccessibleModal`, never raw `showModalBottomSheet`; the morph duration already flows through `accessibleDuration` inside `openExperienceContentPanel`. **The `GoogleMap` is a platform view screen readers cannot introspect** — markers carry no semantics. The scrolling proposal/vote list below the map **is** the accessible alternative and must remain the canonical way to read and act on options; give the map container a short `Semantics` description and never make map-only interactions load-bearing. CI (`npm run lint:dart:a11y`) runs against an empty allowlist, so any new violation must be fixed in the same PR.
- **i18n — both locales, `a11y` prefix, ICU plurals, no strings in the view-model.** Every new user-facing string lands in **both** `app_en.arb` and `app_es.arb` (the `lint:dart:l10n-parity` gate fails otherwise; English placeholder text is acceptable for the Spanish value). Semantic labels use the `a11y` prefix. Counts ("{n} options", "N of M replied") use ICU plural blocks — reuse the existing `contentFactPollOptions` / `locationPollVoteCounter` keys rather than minting new ones. The view-model must **not** resolve strings; the panel resolves via `context.l10n` in `build()`. And per CLAUDE.md, audit every new string for **Event**, not "Experience".
- **Testing — lead with ViewModel tests; widget tests only where the surface is the thing under test; golden is optional.** docs/client/testing.md makes ViewModel tests the primary signal and explicitly lists golden tests as a *missing* pattern, so Phase 5 is reframed below: cover voting / owner-control / single-vs-poll state logic with `locationModalProvider` ViewModel tests (`ProviderContainer` + Mockito-mocked `ExperienceRepository`), and reserve widget tests for the morph/Rect interaction and panel composition that can't be exercised headless. No new async notifier is introduced (we reuse the existing `locationModalProvider` `AsyncNotifier`), so no new disposal-safety suite is required — but if the panel adds any local `StatefulWidget` async, guard with `if (!mounted) return;` per the documented widget-async exceptions. Override `locationSearchServiceProvider` and `userLocationProvider` with fakes (geospatial Pattern 5).
- **Server boundary — none.** This is a client-only presentation change; it reuses the existing location-poll RPCs end-to-end and adds **no** server code. docs/server/{architecture,observability}.md (structured logging, request IDs, masking, log levels) therefore do not apply — there is no new handler, log line, or RPC to bring into compliance. Flag in review if any phase starts to require a server change: that would be a scope change, not part of this issue.

## Implementation Plan

### Phase 1: Morph the WHERE card into a panel (surface parity, no map/voting yet)
**Goal:** Tapping WHERE morph-expands from the card's own footprint into a full-screen `ContentMorphPanel`, dismissible by ✕ / back / swipe — visually identical entry to discussion & Who's In. The panel initially hosts the *current* content (can temporarily embed today's state-dispatched body or a placeholder) to prove the surface.
**Tasks:**
- [ ] Plumb a `Rect` through `ContentFactsRow` → `ContentFactData` (and `ContentFactCta`): capture the individual card's `RenderBox` footprint at tap time and pass it up. Keep the change generic so the WHEN card can adopt it later.
- [ ] Change `ExperienceReadShell.onShowLocation` from `VoidCallback` to `ValueChanged<Rect>`; update the three wire points ([:226, :238, :250](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart#L226-L250)).
- [ ] Add `ExperienceLocationPanel` (wraps `ContentMorphPanel`, header + close ✕, à la `ExperienceConversationPanel`) and a content-view handler `_expandLocation(Rect)` that calls `openExperienceContentPanel(routeName: 'experience_location', …)`.
- [ ] Add the `contentCacheInvalidationProvider` listener inside the panel (like the roster panel) so it refreshes while open.
**Validates:** The per-card Rect morph works from inside the shared facts row — the riskiest new mechanic — independent of map/voting content.

### Phase 2: Embed the map
**Goal:** The panel shows a live map: phone blue dot + the single confirmed/selected location, or all active-poll proposals as markers.
**Tasks:**
- [ ] Extend `LocationMapPreview` to accept an optional `Set<Marker>` (preserve single-marker fallback), or add a thin multi-marker map per the Open Questions decision.
- [ ] Resolve per-proposal coordinates (inline `geocoded` vs. saved `locationId` via `locationRepositoryProvider`) into markers; center/fit the camera on the marker set (single → centered; many → bounds).
- [ ] Drive the blue dot from `userLocationProvider(precisePin)` with no prompt on mount (#1174).
**Validates:** Coordinate resolution for both proposal kinds and the single-vs-poll camera framing.

### Phase 3: Inline voting on the panel
**Goal:** Attendees vote on proposals directly in the panel, auto-submit on tap, optimistic UI, no secondary modal.
**Tasks:**
- [ ] Move per-proposal **display name / address / coordinate** resolution out of the widget (`_RowContent`'s `FutureBuilder` over `locationRepositoryProvider`) and into `LocationModalData` / `locationModalProvider`, so proposals arrive pre-resolved. This fixes the existing widgets-call-repositories layering breach (Audit Notes) and gives Phase 2's map one coordinate source.
- [ ] Extract the vote row + atoms (`_VoteLocRow`, `_Checkbox`, `_VoterStack`, the now-thin `_RowContent`, vote counter, leading-badge, `PollFlexibleVoteRow`, `AddSpotCard`) from `location_poll_vote_modal.dart` into shared widgets; re-theme for the on-content dark surface (`onContentImage`/`darkText*` instead of `modal*`). Keep `Toggle` (with `selected:`) for the vote rows and `LiveRegion` for the counter — preserve the a11y primitives, don't hand-roll.
- [ ] Render the extracted list in the panel against `locationModalProvider`; wire tap → `voteOnLocation` (already optimistic) with per-row in-flight disabling and the flexible "any spot works" row for non-owners. Announce submission via `SemanticAnnouncer`.
- [ ] Have the old `LocationPollVoteModal` consume the same extracted widgets + pre-resolved view-model fields (no behavior change there) so there is one implementation.
**Validates:** The voting flow on the new surface with the existing optimistic view-model; confirms extraction didn't regress the bottom-sheet path and that coordinate/name resolution now lives in the view-model.

### Phase 4: Owner controls + sub-flows
**Goal:** The owner can pick a different spot and add spots / ask the group from within the panel; non-owner read-only state is correct.
**Tasks:**
- [ ] Add owner buttons: "Pick a different spot" (routes to `confirmLocation` / end-then-set per Open Questions, **not** `setSingleLocation` while a poll is active) and "Add spots & ask the group" (`proposeSavedLocation` / `proposeGeocodedLocation`, opening the Google location picker as a transient sub-sheet for selection only).
- [ ] Surface the existing manage actions (set final, lock proposals, set deadline, nudge, cancel) — reuse `LocationPollManageMenuSheet`.
- [ ] Implement the non-owner read-only state (map + address + directions, no voting UI) for a single confirmed spot with no active poll.
- [ ] Ensure all new strings use **Event**, not "Experience"; add a11y/semantics labels.
**Validates:** Full owner/attendee/read-only matrix on the new surface.

### Phase 5: Retire the old dispatch, test, document
**Goal:** Remove the superseded path for this entry point and lock behavior in with tests + docs.
**Tasks:**
- [ ] Replace `_showLocationPickerModal`'s state dispatch for the WHERE entry point with the panel launch; delete now-dead modal files/callers confirmed unreferenced (per Open Questions), or leave them shared if other surfaces use them.
- [ ] **Primary coverage — ViewModel tests** (`ProviderContainer` + Mockito-mocked `ExperienceRepository`, no widget tree) for `locationModalProvider`: inline voting auto-submit + optimistic rollback on RPC failure, owner pick-a-winner / add-spots paths, single-location vs. active-poll vs. completed state, and the new pre-resolved name/address/coordinate fields from Phase 3. This is the main regression signal per docs/client/testing.md.
- [ ] **Targeted widget tests** only for what ViewModel tests can't reach: the morph open/close from the WHERE card's footprint (Rect capture through `ContentFactsRow`), owner-vs-non-owner control visibility, and the read-only state. Wrap with `localizationsDelegates` (see `app/test/helpers/l10n_helpers.dart`) and override `locationSearchServiceProvider` / `userLocationProvider` with fakes (geospatial Pattern 5). Golden tests are **optional** — they are not an established pattern in this repo (testing.md lists them as a gap), so don't gate the work on them.
- [ ] Add the new ARB keys to **both** `app_en.arb` and `app_es.arb` (parity gate) with `a11y`-prefixed semantic labels and **Event** (not "Experience") wording; run `npm run generate` before testing.
- [ ] Update [docs/client/modals.md](../client/modals.md) (add the location panel to the morph-reveal reference list) and [docs/client/geospatial.md](../client/geospatial.md) (note the multi-marker map use + the map's accessible-list alternative).
**Validates:** No dead code, ViewModel-first regression coverage, l10n parity, docs match the shipped surface.

## Related Issues
- #2280 — *"Who's pitching in?" expand*: established the morph-reveal panel and the shared launcher this issue reuses ([docs/issues/2280-pitching-in-expand.md](2280-pitching-in-expand.md)).
- #2278 — *Experience content redesign*: introduced the `ContentFactsRow` / `ContentDiscussionCard` read-shell this issue extends ([docs/issues/2278-experience-content-redesign.md](2278-experience-content-redesign.md)).
- #2091 — *Location polls*: the underlying poll domain (proto, RPCs, view-model) being re-housed ([docs/issues/2091-location-polls.md](2091-location-polls.md)).
- #1174 — no-prompt-on-mount GPS permission policy the panel's blue dot must honor.
- #2188 — provider-agnostic geocoding (Mapbox/Google) the map + picker sub-flow run on.
