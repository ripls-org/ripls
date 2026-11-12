# Issue #2280: Pitching-in widget: expand-in-place (no modal) + faces, stocked bar, claim pills

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2280
**Filed:** 2026-06-03 | **Last activity:** 2026-06-03
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/issues/2278-experience-content-redesign.md](2278-experience-content-redesign.md), [docs/cowork/App UXR/who-pitching-in-morph-v6.html](../cowork/App%20UXR/who-pitching-in-morph-v6.html)

> **Compliance audit pass (2026-06-03) — reviewed against [`docs/client/{architecture,caching,i18n,accessibility,testing}.md`](../client/).** Server docs (`architecture`, `observability`) are **not applicable**: this is a client-only UI/UX change with no RPC, proto, or backend work. Six corrections were folded into the Open Questions and phases below: **(a) Color-only status** — the mockup's maybe `?` badge, faded "not going" face, and the stocked progress bar convey state by color/opacity alone; `avoid_color_only_status` (a11y gate) flags any `*Dot`/`*Indicator`/`*Badge` class without `Semantics`/`Tooltip`. Each status marker needs a `Semantics(label:)` (from `context.l10n`, `a11y` prefix) and the stocked bar needs a `Semantics(value:)`. **(b) Async safety is narrow here** — the new view-data getters are **pure/sync** extensions on `ExperienceState` (like the existing `edges()`), so they need *no* `ref.mounted` guards and are unit-testable without a container. The only async surface is the **existing** claim/RSVP mutations on `experienceProvider` (a `NotifierProvider.autoDispose.family`) — those already carry guards; do not add new async to the view-data. **(c) Navigation caveats** — `pushScreen` from inside the feed/carousel may need `useRootNavigator: true` so the panel overlays the nav bar; the panel's close affordance uses `AppBarBackButton(onPressed: handleClose)` / `BackButtonWidget` (never `Icons.arrow_back`), with an `if (!context.mounted) return;` guard between any `pop()` and a follow-on nav call. The roster is a scrollable list with **no text input**, so `SwipeToCloseMixin` is appropriate (same pattern as the conversation screen) — confirm the vertical-scroll vs. horizontal-swipe arena behaves. **(d) i18n** — migrated roster assembly must expose **enums/typed data, not resolved strings** (the current sheet's `_statusLine` string-building stays in the widget via `context.l10n`); plural copy ("N going", "N still need a hero") uses ICU `plural`; any new async error surfaces as `UserError? error` (none expected). **(e) Caching** — no manual cache maps; the stocked % is derived from already-cached `experienceNeedsProvider` data; claim/RSVP invalidation stays owned by `ExperienceRepository` via the existing providers. **(f) Testing** — pure view-data getters are tested by constructing `ExperienceState` directly (no container, mirroring the existing `edges()` tests); container tests use `addTearDown(container.dispose)` + `reset(mock)` in `tearDown`; widget tests use the `app/test/helpers/l10n_helpers.dart` `MaterialApp` wrapper; any delight debounce uses `runDebounced`, never `Future.delayed`.

## Implementation status (2026-06-03) — all phases shipped on `exp-advanced`

All four phases are implemented and green. Summary of what landed:
- **Phase 1 (view-data):** `stockedSummaryFrom(needs)` (Σ slots / slots_remaining → covered/percent/fully-stocked) and `rosterGroups()` (Going incl. host-first / Maybe / Not going / **No reply**) added as **pure getters** on `ExperienceState` in [experience_content_view_data.dart](../../app/lib/presentation/viewmodels/experience_content_view_data.dart). Unit-tested by constructing state directly.

### "Still no reply" — now backed by the server (2026-06-03)

The "still no reply" section is no longer data-gated/empty. `GetExperienceResponse` gained a `repeated User invited_no_reply` field ([experience_service.proto](../../proto/ripls/api/experience_service.proto)): in [`GetExperience`](../../server/services/experience/get.go) the server computes the **caller-visible** community members who have no RSVP and are not the host, deterministically sorted and capped at `maxInvitedNoReply` (50). It is **caller-scoped** — only members of communities the caller belongs to — so it never leaks members of communities the caller isn't in (mirroring the `shared_communities` privacy model). The client maps `details.invitedNoReply` into `rosterGroups().noReply` (a new `RosterStatus.noReply`), deduped against anyone already listed. Covered by `TestGetExperience_InvitedNoReply` (server, incl. the caller-scoping case) and the `rosterGroups` no-reply unit tests (client).
- **Phase 2 (collapsed widget):** `ContentEdgeRow` gained `showStatusPill` + an avatar maybe-`?` badge (semantic-labelled, not color-only); `ContentEdgesCard` gained a `headerLeading` slot. The read shell now renders faces (`ContentAvatar`), a header `ContentAvatarStack`, and a footer that shows "be the first to pitch in" / the stocked-needs summary.
- **Phase 3 (expand-in-place):** new [experience_pitching_in_screen.dart](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart) revealed by a **clip-reveal morph** ([morph_reveal_route.dart](../../app/lib/presentation/widgets/content/morph_reveal_route.dart)) — a transparent `PageRoute` whose destination is clipped from the card's on-screen footprint (captured via the card's `RenderBox` and passed through `ContentEdgesCard.onTap(Rect)`) growing to full screen, mirroring the mockup's `clip-path` grow. Pushed on the root navigator; close via `IconAction` → `Navigator.pop` (reverse morph) or system back. Duration via `accessibleDuration` (instant under reduce-motion). Body has a `ContentStockedBar`, a "still needed" claim-pill section, and the grouped roster. The old `experience_pitching_in_sheet.dart` + its `showAccessibleModal` entry were **deleted**; claim/edit/RSVP all reuse the existing `NeedClaimChip`/`NeedsActions`/RSVP-composer flows unchanged.
  - **Overlays the existing hero, not the whole content.** The panel is a transparent scaffold over a semi-transparent dark gradient, so the experience's hero (video/image) stays visible behind it — **the same `VideoBackgroundHost` keeps playing**, it is not re-rendered (which would restart the video). While the panel is open, a per-experience flag ([experience_pitching_in_expanded_provider.dart](../../app/lib/presentation/viewmodels/experience_pitching_in_expanded_provider.dart)) hides the read shell's body so none of the other content (facts, discussion, collapsed roster) shows through; the flag is set on push and cleared on close. The morph route's own backdrop scrim is disabled (`scrimMaxOpacity: 0`) so the hero isn't dimmed twice.
- **Phase 4 (gates):** `flutter analyze` clean; `lint:nav`, `lint:dart:{a11y,i18n,l10n-parity,size}`, `lint:ephemeral-comments` green; full `flutter test` passes (4690 / 1 pre-existing skip). New l10n keys added to both ARB locales. **Deferred (optional delight):** the claim-pop/toast animation — the claim already routes through the existing sheet which gives its own confirmation; can be added later behind `accessibleDuration` + `SemanticAnnouncer`.

### Add-buttons modal wiring — final (2026-06-04)

The panel's two "add" affordances reuse the canonical experience modals (no new sheets):
- **"Needed · N" → "Add a need"** opens `ExperienceComposeSheet.show(...)` — the **same** modal the event's "What's Needed" row opens (chips, paste-list, inline-add, pre-claim), mirroring the Request side's `RequestComposeSheet`.
- **Viewer row → "+ Add" (what you're bringing)** opens `ExperienceRsvpComposerSheet.show(...)` — the "what are you bringing" composer, which includes the **link-from-gear-library** affordance (`_pickFromGear` → `NeedsClaimGearPicker`) and handles the implicit RSVP itself.

Both are dispatched through `NeedsActions.openAddNeed` / `openAddContribution` (experience-scope-only; the latter takes the panel's `accentColor`). The earlier `openBatchAddNeed`/`openBatchAddContribution` → `BatchAddNeedSheet`/`BatchAddContributionSheet` wiring was **removed** from `NeedsActions`; those batch sheets remain in use by `needs_section.dart`.

### Contribution composer — contributions-only mode (2026-06-04)

When opened from the panel's "+ Add", `ExperienceRsvpComposerSheet` runs in a new `contributionsOnly` mode: the RSVP intention toggle is hidden, the header reads **"Your Contribution"** (contribution eyebrow/subtitle), the footer summary line is dropped, and the primary action is **"Save"** (contributions commit inline as they're added, so Save just closes). The full RSVP composer is unchanged — the behavior is gated behind the flag.

### v7 aesthetic pass (2026-06-04, ref `who-pitching-in-expanded-standalone-v7.html`)

Visual alignment to v7, no structural rewrite of the RSVP control or roster grouping:
- **Empty-slot needs + open count.** Open needs render as neutral dashed "slots" (bright white label + accent `+`) instead of amber; the header reads **"Needed · N open"** counting only unclaimed needs, flipping to **"All covered 🎉"** when none remain. Claimed chips keep the accent ✓ and gain a faint **"· you"** when the viewer is the one bringing it (`ContentNeedChip` gained an `attribution` slot).
- **Top action row.** New `_actionRow`: **+ I'll bring** (primary, accent) · **+ We need** · **+ Invite**. These replace the inline "Add a need" chip (removed from the Needed section) and the viewer-row "+ Add" (removed). "I'll bring" → `openAddContribution`, "We need" → `openAddNeed`, "Invite" → the share-invite-link sheet (`ItemShareSheet`, scoped via `CommunityHelper.invitableSharedCommunities`; messages "No one to invite" when there's no invitable community).
- **Viewer row.** Replaced the accent left-border + tint with a **ringed avatar** + a **bordered "YOU" pill**.
- **Tighter rows.** Roster avatars 40 → 30px; each row carries a hairline top divider (`modalBorderSubtle`).

The old `pitchingInAddNeed` / `pitchingInAddBringing` ARB strings were removed; new `pitchingInNeededOpen` / `pitchingInAllCovered` / `pitchingInClaimedByYou` / `pitchingInAction{Bring,Need,Invite}` / `pitchingInInviteUnavailable` / `a11yRosterInvite` keys added to both locales. The panel **title** ("Who's pitching in") and the header count summary were left as-is (not in the selected scope).

## Problem Statement

The "Who's pitching in?" surface on the Experience (Event) content view shipped under #2278 as a **collapsed card** ([content_edges_card.dart](../../app/lib/presentation/widgets/content/content_edges_card.dart)) that opens a **bottom-sheet modal** ([experience_pitching_in_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_sheet.dart)) via `showAccessibleModal`. This issue refines both surfaces against a newer mockup (`who-pitching-in-morph-v6.html`).

The headline change: **tapping the widget expands it in place into a full-screen roster panel (X to close) — it is no longer a modal sheet.** Secondary changes bring the surface closer to the mockup: faces/avatars on rows (today rows are names-only), status-as-the-exception markers (going = unmarked, maybe = amber `?`, not-going = faded), a "stocked" progress bar, tap-to-claim "still needed" pills, a grouped roster (Going / Maybe / Not going), a "still no reply" line, and a no-RSVP empty state that lists who's invited.

**Hard constraint, stated by the user and in the issue:** *do not touch the actual needs/contributions claim/edit modals.* The mockup's `subsheet` / `openItem` / `openEdit` / `claim` interactions are prototype stand-ins — they must wire to the **existing** shared flows: `NeedClaimChip` → `NeedsActions.openClaimSheet` → the Request flow's `NeedsClaimConfirmSheet`, and the existing RSVP composer ([experience_rsvp_composer_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart)) for "edit your RSVP / what you're bringing."

No backend work: all data is already loaded — RSVPs from `experienceProvider` (`details.rsvps`, `experience.owner`) and needs/contributions from `experienceNeedsProvider`.

**The single biggest design ambiguity** (see Open Questions and the Recommendation) is *what "expand in place" actually means in our hosting context*. The read shell is **not** a normal scrollable screen — it is a **bottom-anchored, non-scrolling panel** that is one horizontal tab inside [experience_conversation_carousel.dart](../../app/lib/presentation/screens/experience/widgets/experience_conversation_carousel.dart), itself embedded in the TikTok-style **vertical feed**. The panel deliberately *does not capture vertical drag* so the feed can. A literal clip-path "grow from footprint" morph (as the HTML prototype does) would have to win a gesture/painting fight against both the carousel's horizontal drag and the feed's vertical drag. That constraint drives the recommendation below.

## Context from Comments

The issue has **no comments** — it was filed in this session from a direct user request. The authoritative spec is the mockup ([who-pitching-in-morph-v6.html](../cowork/App%20UXR/who-pitching-in-morph-v6.html)) plus the issue body. The mockup's own caption flags items as prototype-only:

- Avatars use **initials**; "swap in photos in production."
- The pop → "✓ you've got it" chip, the haptic, and the cheer toast are **delight polish**, not the contract — the claim itself must go through the real claim flow.
- The clip-path grow is the prototype's chosen motion; the issue says "match the spirit within our motion conventions," not "replicate the clip-path."

Because there is no discussion, the Open Questions section is the place where the real decisions get made; each carries a recommendation.

## Codebase findings (what exists vs. what's new)

**Reusable today:**
- **Collapsed card:** [content_edges_card.dart](../../app/lib/presentation/widgets/content/content_edges_card.dart) — `ContentEdgesCard` (tappable header + divided rows + footer slot) and `ContentEdgeRow` (avatar · name · contribution · status pill, with `showAvatar` and `trailing` hooks). Today the read shell passes `showAvatar: false`.
- **Avatars:** [content_avatar.dart](../../app/lib/presentation/widgets/content/content_avatar.dart) (`ContentAvatar(user:, size:)`) and [content_avatar_stack.dart](../../app/lib/presentation/widgets/content/content_avatar_stack.dart) (`ContentAvatarStack(users:, maxVisible:)` with `+N` overflow) already exist — ideal for the row faces and the header stack. `ContentAvatar` already routes through `CachedMediaImage` semantics.
- **View-data assembly:** `edges()` in [experience_content_view_data.dart](../../app/lib/presentation/viewmodels/experience_content_view_data.dart) — pure extension on `ExperienceState`, host-first, status derived from RSVP intention, wrapped-aware. **Currently omits declines and never emits `EdgeStatus.invited`.** `EdgeStatus.invited` already exists in [content_edge_data.dart](../../app/lib/presentation/widgets/content/content_edge_data.dart) but is unused by the assembler.
- **The modal's roster logic** (`_PitchEntry`, dedup, host-injection, recency sort, per-person contributions/needs grouping) lives **inline in the widget** in [experience_pitching_in_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_sheet.dart) — i.e. business logic in the widget layer, which the MVVM rule wants in the view-model.
- **Coverage data for the stocked bar:** `ExperienceNeedResponse` has `slots` (int32, field 6) and `slots_remaining` (int32, field 7) — so "N still need a hero" and a percent are derivable client-side from `experienceNeedsProvider`'s `needs`. No backend needed.
- **Claim/edit flows:** `NeedClaimChip` + `NeedsActions` (`NeedsScope.experience(...)`) and `ExperienceRsvpComposerSheet.show(...)` — reuse verbatim.
- **Motion / nav primitives:** `accessibleDuration(context, ...)` ([accessible_duration.dart](../../app/lib/presentation/widgets/accessibility/accessible_duration.dart)); `NavigationHelpers.pushScreen<T>(...)` + `SwipeToCloseMixin` ([navigation_helpers.dart](../../app/lib/core/utils/navigation_helpers.dart)) — the only sanctioned push path (`npm run lint:nav` blocks raw `Navigator.push` / `context.push`).

**Hosting constraint (critical):**
- The read shell is built inside [experience_conversation_carousel.dart](../../app/lib/presentation/screens/experience/widgets/experience_conversation_carousel.dart) (horizontal carousel: content ↔ conversation) which itself lives in the vertical feed. The shell is a `Positioned` non-scrolling panel; scroll/drag is intentionally not captured. Any "expand" must therefore present **above** this gesture arena, not morph within it.

**New (small):**
- Extend the view-data to emit declines (`EdgeStatus` for not-going) and invitees with no reply (`EdgeStatus.invited`), grouped, plus a stocked summary `(covered, total, remaining)`.
- A full-screen roster panel/screen that renders that grouped view-data and reuses the existing claim/RSVP flows.
- Collapsed-card avatar rendering + maybe/not-going markers + needs-summary footer + empty state.

## Potential Solutions

### Option A: Pushed full-screen route (`pushScreen` + `SwipeToCloseMixin`) with a Hero/fade "grow" polish
- **Approach:** Extract the roster body out of the modal sheet into a stateless, role-agnostic panel widget. Present it via `NavigationHelpers.pushScreen()` into a `SwipeToCloseMixin` screen. Add a `Hero` (or a short `accessibleDuration` fade/scale) from the collapsed card to approximate the mockup's "grow." Delete the `showAccessibleModal` path. Move roster assembly into the view-model.
- **Pros:** Escapes the carousel/feed gesture arena cleanly; reuses the project's sanctioned navigation + swipe-to-close; passes `lint:nav`; the panel is independently testable; swipe-back is a natural "close." Lowest risk given the hosting constraints.
- **Cons:** Not a literal clip-path morph — the "card grows from its own footprint" is approximated (Hero/fade), not pixel-faithful to the prototype.
- **Effort:** Medium.

### Option B: In-screen `OverlayEntry` / `AnimatedContainer` clip-path morph
- **Approach:** Keep the expansion inside the content screen as a full-bleed overlay that animates a clip-path/size from the card's `RenderBox` to full screen, mirroring the HTML.
- **Pros:** Closest to the mockup's exact motion.
- **Cons:** Fights the carousel's horizontal drag and the feed's vertical drag (gesture arena conflicts, dismiss ambiguity); no sanctioned dismissal gesture; harder to make reduced-motion-correct; `RenderBox` footprint math is brittle across feed scroll positions; doesn't compose with `SwipeToCloseMixin`. High risk for marginal fidelity gain the issue explicitly doesn't require.
- **Effort:** Large.

### Option C: Keep the modal, just restyle its contents
- **Approach:** Leave `showAccessibleModal` in place; add faces/stocked bar/grouping to the existing sheet and collapsed card.
- **Pros:** Smallest change; lowest risk.
- **Cons:** **Fails the issue's headline requirement** ("it is no longer a modal" / "expands it in place"). Non-starter as the primary deliverable, though its content work overlaps Option A's panel.
- **Effort:** Small.

## Recommendation

**Option A — pushed full-screen route with a Hero/fade "grow" polish.** It is the only option that respects the real hosting constraint (carousel-in-vertical-feed) while satisfying "no longer a modal" and "expands to show the full content," and it stays on the project's sanctioned navigation rails (`pushScreen` + `SwipeToCloseMixin`, `lint:nav`-clean). The mockup's clip-path morph is explicitly a "match the spirit" target, not a contract — a `Hero` transition (the collapsed card's avatar stack / container flying into the panel header) plus an `accessibleDuration` fade reads as "the widget grew" without entering a three-way gesture fight.

Pair it with the **MVVM correction**: move the roster assembly (currently inline `_PitchEntry` logic in the sheet) into the view-model as grouped, typed view-data, and **extend `edges()`** to optionally surface declines and no-reply invitees. The collapsed card and the expanded panel then render the *same* derived data at two levels of detail.

If, after a spike, a literal clip-path morph proves clean within the carousel without breaking feed/carousel gestures or reduced-motion, it can replace the Hero polish later — but it should not block the deliverable.

## Open Questions

> **Decisions confirmed (user, 2026-06-03): every recommendation below is accepted as written and is now the plan of record.** Implementation proceeds on all phases: pushed full-screen route via `pushScreen` + `SwipeToCloseMixin` (Hero/fade "grow", no literal clip-path); roster assembly + stocked summary moved into the view-model as pure getters; `roster()` buckets Going / Maybe / Not going, with the "no reply" line **data-gated** (no invitee source client-side today, so it stays hidden until a backend follow-up supplies one — gap noted, not invented); stocked % derived from `slots` / `slots_remaining`; theme via `AppTheme.headingFont` + `AppColors`; the existing claim/RSVP modals are reused unchanged; the old `ExperiencePitchingInSheet` is refactored into the pushed screen (not forked) and its `showAccessibleModal` entry deleted.

- **What navigation primitive backs "expand in place"?** — Recommendation: a **pushed full-screen route** via `NavigationHelpers.pushScreen()` into a `SwipeToCloseMixin` screen, *not* a literal in-screen clip-path morph. Rationale: the read shell is a non-scrolling tab inside a horizontal carousel inside the vertical feed; an overlay morph would fight both drag axes and has no sanctioned dismiss gesture, while `pushScreen` escapes the arena and gives swipe-back for free. Approximate the "grow" with a `Hero` + `accessibleDuration` fade. **Audit caveats (architecture.md §Navigation & Gestures):** (1) pass `useRootNavigator: true` so the panel overlays the home nav bar from inside the feed/carousel; (2) the close affordance is `AppBarBackButton(onPressed: handleClose)` (AppBar pattern) or `BackButtonWidget` (overlay pattern) — never `Icons.arrow_back` directly — so the reverse slide plays before pop; (3) put an `if (!context.mounted) return;` guard between any `Navigator.pop()` and a subsequent nav call; (4) `SwipeToCloseMixin` is sanctioned here because the roster is a scrollable list with **no text input** (the doc's "don't add to list/menu screens" warning targets text-input and nav-menu screens; the conversation screen is the precedent for a scrollable content screen using it) — verify the vertical-scroll/horizontal-swipe gesture arena during the spike.
- **Where does the roster assembly live?** — Recommendation: **the view-model.** Move the sheet's inline `_PitchEntry` dedup/host-injection/sort and the per-person contribution/need grouping into `experience_content_view_data.dart` (or a sibling view-data extension) as typed structs. The new panel and the collapsed card both consume it. This fixes an existing MVVM-boundary violation (business logic in the widget) flagged in #2278's compliance audit.
- **How is "not going" / "still no reply" modeled in `edges()`?** — Recommendation: extend `edges()` with opt-in flags (e.g. `includeDeclines`, `includeInvited`) or add a richer grouped getter that returns Going / Maybe / NotGoing / NoReply buckets. `EdgeStatus.invited` already exists and is unused; add/confirm a not-going status mapping. The collapsed card shows faded faces for not-going; the expanded panel shows the full grouped list + the "still no reply" line. **Where does the invitee list come from?** — flag: the current data only has RSVPs + owner; confirm whether an "invited but no reply" set is actually available client-side (it may require enumerating community/invitee membership). If not available without backend, ship the no-reply line **data-gated** (render only when the set is non-empty) and note the gap — do not invent an invitee source.
- **How is the "stocked" bar computed?** — Recommendation: derive client-side from `experienceNeedsProvider` needs using `slots` / `slots_remaining`: `total = Σ slots`, `remaining = Σ slots_remaining`, `covered = total - remaining`, `pct = covered/total`. Copy ("Almost stocked — N things still need a hero") via `context.l10n`. Pure → unit-testable in the view-model.
- **Is the clip-path "grow" morph in scope for v1?** — Recommendation: **No** as a literal effect; ship the Hero/fade approximation. Revisit only if a spike proves it's gesture-safe and reduced-motion-correct.
- **Avatars: photos or initials?** — Recommendation: reuse `ContentAvatar` / `ContentAvatarStack` (already photo-capable via `CachedMediaImage`, with initials fallback). Satisfies the `require_semantic_label_on_cached_media_image` a11y gate automatically.
- **Does the existing `ExperiencePitchingInSheet` get deleted or refactored?** — Recommendation: **refactor, don't fork.** Extract its row-rendering body into the new panel widget (reused by the pushed screen); delete the `showAccessibleModal` `show(...)` entry point and the `_openPitchingIn` modal call in the read shell. Avoid a `_v2` file (memory: feedback_theme_reuse — no version-suffixed files/dirs).
- **Theme tokens?** — Recommendation: map the mockup's Newsreader serif → `AppTheme.headingFont` and its green/amber → `AppColors` sage/`AppColors.warning`; no new font, no per-section hex, no `v2/` dirs (memory: [feedback_theme_reuse](../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md)).
- **File-size ceiling.** — Recommendation: the new full-screen panel + extracted view-data must each stay under the 1,000-line CI cap (`npm run lint:dart:size`); extraction naturally keeps the read shell thin. No `// dart-line-count-allow` hatch.

## Implementation Plan

### Phase 1: View-data — grouped roster + stocked summary (no UI change)
**Goal:** Make all the data the new surfaces need available as pure, typed, unit-tested view-data in the view-model, fixing the MVVM-boundary violation.
**Tasks:**
- [ ] Extend `edges()` (or add a grouped getter) in [experience_content_view_data.dart](../../app/lib/presentation/viewmodels/experience_content_view_data.dart) to bucket participants into Going / Maybe / Not going (declines via faded/`EdgeStatus`) and — data-gated — No reply (`EdgeStatus.invited`), preserving host-first + the existing wrapped-aware behavior.
- [ ] Add a pure stocked-summary derivation from `experienceNeedsProvider` needs (`slots` / `slots_remaining` → covered/total/remaining/pct). Keep it pure (inject the needs list) so it's clock-/provider-free in tests.
- [ ] Migrate the sheet's inline `_PitchEntry` assembly (dedup, host injection, recency sort, per-person contributions/needs grouping) into the view-model as typed structs.
- [ ] **Tests:** unit-test grouping (host-first, declines bucketed, no-reply only when present, wrapped state), stocked math (incl. zero-needs → no bar), and the migrated roster assembly. These getters are **pure** — construct `ExperienceState` directly, no `ProviderContainer` needed (mirror the existing `edges()` tests). Keep them clock-free by injecting `nowUnixSec` where lifecycle phase matters (as `lifecyclePhaseAt` already does).
**Validates:** all new surfaces can be built from pure view-data; no business logic remains in the widget.

### Phase 2: Collapsed widget refresh (faces, markers, footer, empty state)
**Goal:** Bring the always-visible card to the mockup's collapsed look without changing the open behavior yet.
**Tasks:**
- [ ] In the read shell `_edges`, render avatars (`showAvatar: true`) + a header `ContentAvatarStack` with `+N`. Apply status-as-exception markers: going unmarked, maybe = amber `?` badge, not-going = faded face. Add the needs-summary footer ("N things still need a hero") from the stocked summary.
- [ ] **A11y (audit — `avoid_color_only_status`):** the maybe `?` badge, the faded "not going" face, and any header status dots convey state by color/opacity alone. Each marker class (`*Badge`/`*Dot`/`*Indicator`) must carry a `Semantics(label:)` from `context.l10n` (`a11y` prefix, e.g. `a11yRsvpMaybe` / `a11yRsvpNotGoing`) or a `Tooltip`, or fold the status into the avatar's existing `semanticsLabel` ("Photo of {name}, maybe"). Avatars via `ContentAvatar` (already `CachedMediaImage`-backed with semantics).
- [ ] Empty state: when there are zero RSVPs, show the "Be the first to pitch in" CTA + an invited list (data-gated to whatever invitee data is actually available).
- [ ] Route any new strings through `context.l10n` in **both** `app_en.arb` and `app_es.arb` (reuse `contentEdgesHeader`, `experienceGoingCount`, `a11yExpViewAttendees`; add new keys for stocked/empty copy with `a11y`-prefixed labels). Use **"Event," not "Experience,"** in user-visible copy. Run `npm run generate`.
- [ ] Theme via `AppColors` + `AppTheme.headingFont` only.
- [ ] **Tests:** thin widget tests (l10n wrapper) for collapsed card with faces, each marker, footer, and the empty state.
**Validates:** the collapsed card matches the mockup and reads correctly across going/maybe/not-going/empty.

### Phase 3: Expand-in-place — pushed full-screen roster panel (replaces the modal)
**Goal:** Tapping the card opens the full roster as a pushed screen (no modal), reusing existing claim/RSVP flows.
**Tasks:**
- [ ] Extract the modal's roster body into a role-agnostic panel widget driven by Phase-1 view-data: stocked bar, "still needed" claim pills (via `NeedClaimChip` → `NeedsActions`), grouped Going/Maybe/Not-going rows with contribution chips (chips open the existing item claim/edit sheet), viewer "Edit ›" → `ExperienceRsvpComposerSheet.show`, and the "still no reply" line.
- [ ] **A11y (audit):** the stocked progress bar conveys state visually — give it a `Semantics(value:)` (e.g. "9 of 12 covered") from `context.l10n`, not color/width alone. Section headers (Going / Maybe / Not going) use `Semantics(header: true)`. Close control = `IconAction` / `AppBarBackButton` per the navigation caveats above.
- [ ] Host the panel in a `SwipeToCloseMixin` screen; present via `NavigationHelpers.pushScreen()`. Add a `Hero`/`accessibleDuration` fade to approximate "grow." Close control = `IconAction` (tooltip + `semanticsLabel`).
- [ ] Delete the `showAccessibleModal` path: remove `ExperiencePitchingInSheet.show(...)` and repoint `_openPitchingIn` in [experience_read_shell.dart](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart) at the pushed route.
- [ ] Verify the gesture handoff: opening from inside the carousel/feed doesn't trigger a carousel or feed swipe; swipe-back closes cleanly.
- [ ] **Tests:** widget test (with the `l10n_helpers.dart` `MaterialApp` wrapper) that the card tap pushes the panel (not a modal); panel renders stocked bar / pills / grouped roster / no-reply line; claim pill and "Edit ›" invoke the **existing** flows (mock `NeedsActions` / composer entry). Container tests use `addTearDown(container.dispose)` + `reset(mock)` in `tearDown`. **Disposal note:** no *new* async is introduced (view-data is pure; mutations reuse the already-guarded `experienceProvider` paths), so a new disposal-safety test is only needed if this work adds an async method to a notifier — otherwise rely on the existing coverage for the claim/RSVP transitions.
**Validates:** the headline interaction works end-to-end on existing data, the modal is gone, and all claim/edit/RSVP routes hit the existing modals.

### Phase 4: Polish, gates, on-device verification
**Goal:** Ship-ready.
**Tasks:**
- [ ] Optional delight: claim pill pop → "✓ you've got it" + toast (only after the real claim succeeds), gated on reduced-motion via `accessibleDuration`. **A11y:** announce the success via `SemanticAnnouncer.announce(context, …)` / `LiveRegion` rather than a purely visual toast, so screen-reader users hear the confirmation. If the pop/toast uses a debounce or timed dismiss, drive it via `runDebounced` in tests — never `Future.delayed`.
- [ ] Full gate run: `npm run lint:nav`, `lint:dart:{a11y,i18n,l10n-parity,size}`, `lint:ephemeral-comments`; `npm run generate`; `flutter test --reporter expanded`.
- [ ] Manual verify on iOS sim + Android emulator ([/ripls-app-logs](../../.claude/skills/ripls-app-logs/SKILL.md)): expand/close, claim a need, edit RSVP, empty state, maybe/not-going rendering, reduced-motion.
- [ ] Update [docs/client/content.md](../client/content.md) / the #2278 doc's "Implementation status" to note the pitching-in surface is now expand-in-place, not a modal.
**Validates:** gates green, behavior correct on device, docs match reality.

## Related Issues
- #2278 — Experience content redesign that shipped this surface as a modal; this issue supersedes that interaction and reuses its primitives (`ContentEdgesCard`, `edges()` view-data, `NeedClaimChip`, the RSVP composer).
- #2148 — Needs & Contributions modal redesign — the `widgets/needs/*` claim/edit flow this issue must reuse unchanged.
- #2175 — Request content view redesign — the eventual Request consumer of the same role-agnostic roster/edges components.
