# Issue #2568: Replace Workshop with Directory; redesign community & user profiles

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2568
**Filed:** 2026-06-27 | **Last activity:** 2026-06-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** issue body; design mocks `docs/cowork/App Design/directory-gutter-final-v3.html` + `docs/cowork/App Design/profile-final.html`; auto-selected from `docs/llms.txt`: `docs/workshop.md`, `docs/users/profile.md`, `docs/impact_metrics.md`, `docs/users/priorities.md` (product anchor), `docs/ad_hoc_communities.md` (by topic); anchors consulted via index: `client/architecture.md`, `client/conventions.md`, `client/design.md`, `server/architecture.md`, `server/conventions.md`, `proto_conventions.md`. Engaged review dimensions: api_design, error_handling, security/privacy, accessibility, performance (N+1), testing.

## Problem Statement

Three coupled surfaces change:

1. **Replace the Workshop tab with a Directory.** Today the Workshop tab (`docs/workshop.md`) is a momentum-spotting host surface: a vertical `PageView` of full-bleed community pages driven by the momentum/needs/catalyst detector chain emitting `StoredNudge` rows, with deep-dives that morph open over the community photo. The new Directory (`directory-gutter-final-v3.html`) is **the viewer's private address book** — *not a public directory* — listing only the **circles, ad-hoc groups, and people the viewer has actually interacted with / shares membership with**. It renders them in one scrollable list, with a neutral **sort toggle** (Recent default / A–Z) and an accent **type filter** (All / People / Groups), per-row quick actions (calendar + chat), and a pinned search bar that searches *within the viewer's own address book only*. There is no global index, no stranger discovery, and no browsing of entities the viewer isn't already connected to.

2. **Redesign the user (person) profile** (`profile-final.html`, person variant) into a hero-photo + 2×2 metric tiles (**Problems solved · Time together · Money saved · CO₂ avoided**) + a "Recent Activity" card strip + a pinned **Sharing / Calendar / Message** action dock.

3. **Redesign the community (group) profile** with the *same* layout, swapping avatar→cover photo, "you & X"→"all members", and pair totals→collective totals.

**Contradictions / ambiguities to resolve up front:**

- **"5th bottom-nav slot" is inaccurate.** Post-#1895 the bottom nav has **3 visual tabs** — Home/Inbox, Feed, Workshop (`home_screen.dart`, `IndexedStack`) — plus a floating Create FAB. Workshop is the *last* tab (visual index 2 / stack index 3), not a 5th destination. Directory replaces that slot. The directory mock's own nav shows four glyphs (home, search, ripls-brand+badge, plus), which does **not** match today's nav — treat the mock chrome as illustrative, not normative.
- **"Person metrics are your shared totals."** The mock's note says the person tiles are *viewer↔target pair* totals ("10 hrs & 6 swaps match Betty's directory subtitle exactly"). But today's user profile (`docs/users/profile.md`) shows the target's **full aggregate across all their communities**, explicitly *not* scoped to the intersection. **Per-pair metrics do not exist server-side** (confirmed: no `GetPairMetrics`/user↔user aggregation). This is the single biggest new lift and a real product decision — see Open Questions.
- **Two community surfaces exist.** `community_public_screen.dart` (wraps `CommunityMetricsScreen`, opened from links) and `community_screen.dart` (in-community tabs: Community / Members / Discuss, wrapping `CommunityContentView`). The group mock maps most closely to the *public/overview* surface; which one this replaces vs. augments must be pinned.

## Context from Comments

**No comments on the issue** — it was filed fresh (by this author) immediately before this plan. All requirements come from the body + the two design mocks. That means every product decision below is unratified; the Open Questions section is correspondingly load-bearing and should be reviewed before any code is written.

## Codebase findings (Phase 2 research)

**Navigation** — `app/lib/presentation/screens/home/home_screen.dart`: bottom nav built in `_buildBottomNavigation()` (~L300-394); visual↔stack index mapping (`_visualToStackIndex`/`_stackToVisualIndex`) exists because Discover (stack idx 2) is orphaned-but-retained. `WorkshopScreen` imported L40, mounted in `_buildBody()` (~L546). Nav label key `a11yMiscNavWorkshop`; Workshop icon is `RiplsIcons.ripls`. No GoRouter route for Workshop — it's an `IndexedStack` child.

**User profile** — `app/lib/presentation/screens/users/user_screen.dart` (route `'profile'`, opened via `ContentViewHelpers.openUserScreen`). Sections: `UserProfileIdentity` (`widgets/user_profile/`), `KnownForChips`, `ImpactRows`, `AvailableNowRail`, `LeanCards` (`widgets/profile_section/`). VM `viewer_profile_view_model.dart` (autoDispose.family by targetId) → `ProfileRepository` (cache ns `'profile'`, key `profile:<id>`) → `ProfileService.GetUserProfileForViewer`.

**Community profile** — `community_public_screen.dart` → `getCommunityProvider` (`CommunityRepository.get`) → `CommunityService.GetCommunity` (returns id/name/description/mediaIds/numMembers/maxMembers/ownerUserId — **no hero URL resolution, no member rollup, no impact totals**); embeds `CommunityMetricsScreen` for impact. No viewer-scoped community profile RPC, no community AI narrative, no available-now rail.

**Metrics** — `server/impact_metrics/` + `server/services/impact_metrics/`. Community totals: `GetCommunityImpactMetrics`; "problems handled" X-of-Y via `Calculator.CalculateProblemsCounts` + `GetCommunityProblemsSolvedDetail`. User aggregate: `GetUserImpactMetrics` (money/time/CO₂ across all the user's communities, **not** problems-solved, **not** pair-scoped). The four tile metrics all exist *at community level*; at person level only three exist and only as full-aggregate, not pair.

**Directory data** — **no single RPC** returns "my communities + my people + last-activity-per-entity". Candidates: `ListCommunities`, `ListCommunityUsers` (has `joined_at`), `GetFeed` (has `last_activity_at`), `GetPortfolioInboxFeed` (recent-activity ordered). "Recent" needs per-entity last-activity; "A–Z" needs per-entity impact stats. Directory rows also include **ad-hoc/nameless groups** ("Devon, Priya & Wes") — see `docs/ad_hoc_communities.md`.

**Calendar suggestion system (already built — reuse target)** — the Home calendar already renders open-day "suggest things to do" cards: `OpenDaySuggestion` (`portfolio.proto`) ← `assembleOpenDaySuggestions` (`server/services/portfolio/home_suggestions.go`, pure: viewer history × crew × weather) → `CalendarSuggestionPanel`/`CalendarDayDetail` → tap `openSuggestedCreate`. The **community calendar reuses it verbatim** (`workshop_library_calendar_panel.dart` over a community-filtered cached `GetHomeView`). This is **separate** from the Workshop momentum detector chain (`StoredNudge`/`GenerateWorkshopDraft`), which surfaces in the **Feed** (`docs/nudges.md`). No per-person calendar exists yet, but the shared `Calendar*` widgets + `assembleOpenDaySuggestions` make one cheap.

**Workshop teardown blast radius** — server: clean (no service depends on `WorkshopService`; only storage/nudge mutations). Client: `screens/workshop/**` + `widgets/workshop/**` + `workshop_view_model.dart`. **Caveats — not a pure delete:**
- `workshop_switcher_dropdown.dart` is used by the community-promotion ("name-this-group") workflow (`docs/workflows/community_promotion.md`).
- `workshop_library_calendar_panel.dart` is referenced by `docs/client/calendar.md` (shares the `GetHomeView` calendar substrate).
- `WorkshopProblemsSolvedDetailScreen`, the time/money/CO₂ detail screens, and `KnownForCategoryDetailScreen` live under `screens/workshop/` but back metric drill-downs the new profile tiles will likely reuse.
- The momentum/needs/catalyst/ESM detector chain (`server/services/workshop/**`, `server/needs/`, `server/esm/`) feeds nudges; ESM repeat-signal also reads recap stories. Removing the *tab* ≠ removing the *engine*.

## Potential Solutions

### Option A: Full replacement — new Directory tab + pair-scoped profiles + Workshop teardown in one epic
- **Approach:** Build the Directory screen + a new aggregating RPC; rebuild both profiles to the mock incl. new per-pair metric RPC; delete the Workshop tab and dead momentum code.
- **Pros:** Matches the issue literally; no transitional half-states.
- **Cons:** Couples three independently-riskable changes; the per-pair metric work is a large, unratified product change; risks orphaning the momentum engine and the calendar/promotion dependencies; one huge PR.
- **Effort:** Large (multi-week).

### Option B: Phased — profiles first (reuse existing aggregate metrics), Directory second, Workshop retire last (Recommended)
- **Approach:** (1) Reskin both profiles to the mock's hero + 2×2 tile + recent-activity + dock layout, **driven by the metrics that already exist** (target full-aggregate for person; collective for group) — defer the "pair totals" decision. (2) Build the Directory as a new tab behind a feature flag, replacing the Workshop slot, with an explicit directory RPC. (3) Once Directory is shipping, retire the Workshop tab and carefully relocate the still-needed pieces (metric detail screens, switcher dropdown, calendar panel), leaving the momentum/ESM engine intact or explicitly scoped for separate removal.
- **Pros:** Each phase ships independently and is independently revertible; profile reskin needs *no new server work*; isolates the expensive pair-metric question; protects the calendar/promotion/momentum dependencies; aligns with the repo's incremental-PR norm and the Dart 1000-line file gate.
- **Cons:** Two profile data sources (community uses `GetCommunity`+impact, user uses `GetUserProfileForViewer`) persist longer. (Pair-scoped person tiles were considered but **dropped** — see Resolved Decision 2.)
- **Effort:** Medium per phase; Large overall but decomposed.

### Option C: Directory-only now, defer profile redesign
- **Approach:** Ship just the Directory (replacing Workshop); leave both profiles as-is.
- **Pros:** Smallest first step; immediately removes the Workshop tab.
- **Cons:** The mocks tie the directory row subtitle to the profile tiles ("match exactly"); shipping the directory without the profile redesign leaves an inconsistent data story and re-does row/metric plumbing twice.
- **Effort:** Medium.

## Recommendation

**Option B (phased).** Sequence: **Profiles → Directory (flagged) → retire Workshop (relocate the existing community calendar, keep the detector chain).** Rationale:

- The profile reskin is a **pure client** change over metrics that already exist, so it delivers visible value with zero proto/server risk and establishes the shared visual primitives (hero, `MetricTileGrid`, recent-activity strip, action dock) the Directory rows reference. Person tiles are **target-aggregate** (today's behavior) — no pair-metric work (decided).
- The Directory is **mostly a UX change**: it reuses the existing service that already lists all the communities a user is in (the same read that backs the Workshop community switcher), plus existing people/activity reads — rather than introducing a brand-new `GetDirectory` RPC. New server work is minimized to whatever is needed to avoid client-side N+1 and to carry per-entity last-activity / stats (decided).
- Retiring Workshop **last** lets us see exactly which sub-screens the new surfaces still import before deleting, protecting the calendar substrate, the name-this-group switcher, and the metric-detail drill-downs. "Suggest things to do" on monthly views **reuses the existing calendar open-day-suggestion system** (already live on Home + community calendars) — *not* the Workshop postcards; the momentum detector chain is retained and keeps surfacing in the Feed (decided 6). No new momentum surface to build beyond hosting the existing community calendar on the merged community surface and adding a person calendar that reuses the same widgets.

## Resolved Decisions

Product has ratified the questions below; these are now requirements, not options. (Items 7–8 remain recommendations with no objection.)

- **1. Nav slot/icon (DECIDED).** Directory **replaces the current Workshop tab** (visual idx 2) in `home_screen.dart`; keep the floating Create FAB. Use a directory/address-book glyph + new `a11yMiscNavDirectory` ARB key; retire `a11yMiscNavWorkshop`. The 4-glyph mock nav is illustrative only.
- **2. Person-profile tile scope (DECIDED: target-aggregate).** Person tiles use **target-aggregate** metrics — the target's full aggregate across their communities, today's `GetUserProfileForViewer` / `GetUserImpactMetrics` behavior (`docs/users/profile.md`). **No pair-scoped ("you & target") metrics** — that work is dropped. The mock's "matches the directory subtitle exactly" framing is satisfied by both surfaces reading the same aggregate, not by a pair query.
- **3. Community surface (DECIDED: merge).** `community_public_screen.dart` and `community_screen.dart` **merge** into one community surface that adopts the new hero/tiles/recent-activity/dock layout, with the in-community Community/Members/Discuss content reachable from it. Reconcile the two entry points (links currently open the public screen; in-community navigation opens the tabbed screen) onto the merged surface.
- **4. Directory data contract (DECIDED: reuse existing services, UX-first).** This is **mostly a UX change**. Reuse the existing service that already lists all the communities a user is in (the read backing the Workshop community switcher) for circles + ad-hoc groups, and reuse existing people/activity reads (e.g. portfolio `DailyPerson` / feed `last_activity_at`) for people. **Do not build a new `GetDirectory` RPC** unless composing existing reads forces client-side N+1; if it does, add a thin server aggregation (or extend an existing read) rather than fanning `ListCommunityUsers` per community — pin the bound with `AssertMaxQueries`. Whatever read serves people-with-last-activity must be membership-scoped and paginated.
- **4a. Connections-only — NOT a discovery surface (HARD constraint, DECIDED).** **This is an address book. It is NOT for finding new people or communities.** It lists only entities the viewer is already connected to — circles and ad-hoc groups they belong to, and people they share a circle with / have interacted with. Server: `auth.RequireAuth` + membership-scoped queries, never a global index. Search is scoped to the address book only. IDOR boundary: a viewer must never be able to enumerate a person/community they aren't connected to. (`docs/users/priorities.md`: "No discoverability. No browsing. No strangers.")
- **5. Ad-hoc/nameless groups (DECIDED: yes, with name prompt).** Ad-hoc/nameless groups appear as rows (mock: "Devon, Priya & Wes" with a member-count badge), sourced from the user's ad-hoc communities (`docs/ad_hoc_communities.md`) with the auto-derived name. Each nameless row carries a **"name this group" prompt**, reusing the existing name-this-group / community-promotion flow that the Workshop switcher uses today (`workflows/community_promotion.md`, `workshop_switcher_dropdown.dart`) — so that flow's logic must be retained/relocated, not deleted, in Phase 3.
- **6. "Suggest things to do" on monthly views (DECIDED: reuse the EXISTING calendar suggestion system, not the workshop postcards).** Re-investigation (`docs/client/calendar.md` + code) showed the monthly/calendar views **already have** a complete open-day suggestion surface that we reuse rather than rebuild:
  - **Two distinct systems, don't conflate them.** (a) **Calendar open-day suggestions** — `OpenDaySuggestion` (`portfolio.proto`) built by `assembleOpenDaySuggestions` (`server/services/portfolio/home_suggestions.go`, a pure function over the viewer's history × crew × weather), rendered by `CalendarSuggestionPanel` / `CalendarDayDetail`, tapping → `openSuggestedCreate` (seeds the unified-create draft). (b) **Workshop momentum detector chain** — `StoredNudge` / `GenerateWorkshopDraft`, which **already surfaces in the Feed** (`docs/nudges.md`) and recap-story ESM, *plus* the (going-away) Workshop postcards. These are separate; the calendar falls back to a `NudgePayload` card only when no open-day suggestion is ready.
  - **What we reuse:** the calendar open-day suggestion machinery is **already live on the Home calendar and already reused verbatim by the community calendar** (`workshop_library_calendar_panel.dart` composes the same `Calendar*` widgets over a community-filtered, cached `GetHomeView`). So the "suggest things to do" surface for Communities exists today — it just needs to ride the merged community surface (Decision 3) instead of the Workshop morph host. The **Person** monthly view is the only net-new instance, and it reuses the same shared `Calendar*` widgets + `assembleOpenDaySuggestions` over a person-scoped calendar feed.
  - **What this means for the Workshop teardown:** removing the tab does **not** orphan momentum — the detector chain keeps writing `StoredNudge`, the Feed keeps rendering nudges, and the calendar keeps generating open-day suggestions. We do **not** wire `GetWorkshopBrief`/postcards into the monthly views. `GenerateWorkshopDraft` (the postcard-only draft path) likely becomes dead once the tab is gone — confirm with grep and remove if so (the calendar uses `openSuggestedCreate`, a different path).
- **7. "Recent Activity" card-strip source (recommendation).** Reuse the existing `available_now_items` plumbing from `GetUserProfileForViewer` for the person strip; add an equivalent available-now/upcoming gather for the community strip (none exists today). Confirm whether "upcoming plans" + "available gear" are one merged strip or two rows.
- **8. Theme tokens (recommendation).** Mocks are dark-themed serif (Newsreader) with sage accent `#8FAE7E`. Per `client/design.md` + saved feedback, render via existing `AppColors`/`AppTheme.headingFont` tokens — no per-screen coral/cream/sage constants or version-suffixed dirs.

## Implementation Plan

### Phase 1: Shared profile layout + person & community reskin (client-only)
**Goal:** Both profiles render the mock layout (hero + 2×2 tiles + recent-activity strip + pinned Sharing/Calendar/Message dock) over existing metrics.
**Tasks:**
- [ ] Build shared primitives: a hero (cover/avatar + eyebrow + serif name + common-ground sentence + "with" line), a `MetricTileGrid` (2×2: Problems solved · Time together · Money saved · CO₂ avoided), a recent-activity card strip, and the pinned action dock — under `widgets/profile_section/` (keep each file <1000 lines).
- [ ] Rebuild `user_screen.dart` to compose them, driven by `viewer_profile_view_model` with **target-aggregate** metrics (decided). If a user-level "problems solved" total isn't already exposed, add a getter/derivation over existing impact reads — no pair-metric work.
- [ ] **Merge `community_public_screen.dart` + `community_screen.dart`** into one community surface with the new layout (decided), driven by `GetCommunity` + `GetCommunityImpactMetrics` + `GetCommunityProblemsSolvedDetail` (all exist); fold the in-community Community/Members/Discuss content in behind the dock/tabs; reconcile both entry points onto the merged surface. Wire dock actions (Sharing → sharing/access, Calendar → community calendar, Message → conversation). The **Calendar** action hosts the existing community calendar (the `Calendar*` widgets + `workshopCommunityCalendarProvider` filter, already complete with open-day suggestions) — relocate it out of the Workshop morph host onto this surface rather than rebuilding (decided 6).
- [ ] Tile taps reuse the existing metric detail/drill-down screens (relocate from `screens/workshop/` if needed, or point at `screens/impact_metrics/`).
- [ ] `CachedMediaImage` with stable media-id cache keys for the hero cover/avatar (resolve `target_media_id` / community `mediaIds[0]`); never `Image.network` for user media (`client/conventions.md`).
- [ ] l10n keys for all new copy (no hardcoded strings; resolve in widgets per `client/i18n.md`). Any user-facing event copy uses "Event", not "Experience".
- [ ] Widget + viewmodel tests; a11y per `reviews/accessibility_review_prompt.md` — semantic labels on hero/tiles/dock, dock buttons ≥48×48, tiles announce value+label, action dock built from `IconAction`/`Tappable`.
**Validates:** The mock layout works against real data with no server changes; confirms the user-level "problems solved" total is available (or add a getter) before any Directory work.

### Phase 2: Directory address-book surface (reuse existing services, behind a flag)
**Goal:** A Directory screen listing the viewer's connected people + groups + communities with Recent/A–Z sort and All/People/Groups filter, replacing the Workshop nav slot behind a feature flag. **Mostly a UX change** (decided) — reuse existing reads, minimize new server work.
**Tasks:**
- [ ] Data: reuse the existing service that lists the communities a user is in (the read backing the Workshop community switcher) for circles + ad-hoc groups; reuse existing people/activity reads (portfolio `DailyPerson` / feed `last_activity_at`) for people. **No new `GetDirectory` RPC** unless composing these forces client-side N+1 — if so, add a thin membership-scoped, paginated server aggregation (pin with `AssertMaxQueries`); never fan `ListCommunityUsers` per community.
- [ ] Scope = address book only (decided 4a): membership-scoped, `auth.RequireAuth`, no global index, no enumeration of non-connected entities; search scoped to the address book. Add wire-byte leakage tests (mirror `profile_test.go`) for any new/extended read.
- [ ] Ad-hoc/nameless group rows carry a **"name this group" prompt** reusing the existing community-promotion flow (`workflows/community_promotion.md`) (decided 5).
- [ ] Client: `DirectoryViewModel` (AsyncNotifier, loading/empty/error states) + `DirectoryScreen` with sort toggle, type chips, time-bucketed Recent list, A–Z sections + scrub rail, per-row calendar/chat actions, search bar (address-book-scoped). If a new/extended read is added, wrap it in a repository with cache namespace `'directory'` (key `directory:<viewer>:<sort>:<filter>`, invalidate on community join/leave like `profile:*`).
- [ ] Use `CachedMediaImage` (stable media-id cache keys) for all avatars/cover photos — never `Image.network`. Build via accessibility primitives (`Tappable`/`IconAction`) with l10n semantic labels; quick-action buttons meet the 48×48 touch-target min (the mock's 29px chips are visual-only); the A–Z scrub rail needs an accessible alternative (section-jump semantics, not gesture-only).
- [ ] Mount as a new bottom-nav tab in `home_screen.dart` behind a feature-flag provider (Workshop still reachable while flag off); reuse the existing visual↔stack index machinery.
- [ ] Rows route: person→`UserScreen`, community/group→merged community surface; quick actions → calendar / conversation. Any user-facing event copy says "Event", not "Experience" (`client/conventions.md`).
- [ ] Tests: VM (sort/filter combinations, Recent default, empty/no-connections, error states), server (authz scoping, query bound), wire-leakage tests, e2e smoke.
**Validates:** Directory is usable end-to-end against existing data and its row subtitles reconcile with the Phase 1 profile tiles; flag lets us dogfood before cutover.

### Phase 3: Cut over and retire the Workshop tab
**Goal:** Directory is the default tab; the existing calendar/Feed surfaces keep carrying "suggest things to do"; Workshop tab + tab-only client code removed without breaking shared dependencies.
**Tasks:**
- [ ] Flip the flag default; remove the Workshop nav entry, `a11yMiscNavWorkshop`, and `WorkshopScreen` mounting from `home_screen.dart`.
- [ ] **Relocate the community calendar** (`workshop_library_calendar_panel.dart` + `workshop_library_common.dart`'s `workshopCommunityCalendarProvider`, which already reuse the shared `Calendar*` widgets + open-day suggestions) out of the Workshop morph host onto the merged community surface (Phase 1). Add a **person calendar** instance reusing the same widgets + `assembleOpenDaySuggestions` over a person-scoped feed. No new suggestion engine.
- [ ] Relocate other still-needed sub-screens out of `screens/workshop/` — metric detail screens, `KnownForCategoryDetailScreen`, and the **name-this-group switcher/promotion flow** (decided 5) — update imports.
- [ ] **Retain the momentum detector chain server-side** (`server/services/workshop/momentum/**`, `server/needs/`, `server/esm/`, `StoredNudge` writes) — it keeps feeding Feed nudges + recap ESM. Grep `GenerateWorkshopDraft`/`GetWorkshopBrief`/postcard usage; remove whatever is now dead (the postcard draft path is tab-only and likely orphaned — calendar suggestions use `openSuggestedCreate`, a separate path).
- [ ] Delete now-dead `screens/workshop/**` + `widgets/workshop/**` + `workshop_view_model.dart` that nothing else imports (grep to confirm).
- [ ] Update `docs/workshop.md` (rewrite to describe the retained detector chain + its Feed/calendar surfaces, tab removed), `client/calendar.md` (calendars now hosted on the merged community surface + person profile, not the Workshop morph), `workflows/community_promotion.md`, and `docs/llms.txt` (`npm run generate:context-map`); expect the doc-freshness ratchet to flag governed docs.
**Validates:** App builds and ships with Directory as the surface; community + person calendars (with their existing open-day suggestions) work outside the Workshop host; Feed nudges still fire; no broken imports; name-this-group + metric drill-downs still work.

## Related Issues
- #1889 (`1889-user-profile-workshop.md`) — established the user-profile/workshop relationship; §Phase 3.5 is where impact-row drill-downs were deferred.
- #1929 (`1929-profile-v4-redesign.md`) — the current v4 profile this redesign supersedes.
- #1996 (`1996-self-view-items.md`) — self-view item sourcing rules the new person profile must preserve.
- #2447 (`2447-workshop-content-view-redesign.md`) — the current Workshop content-view rendering being replaced.
- #1898 / #1579 / #1618 — Workshop tab + brief history; context for what the momentum engine is and why it should be retired deliberately, not silently.
- #2492 (community-promotion epic) — the name-this-group flow `workshop_switcher_dropdown.dart` is **reused** for the Directory's nameless-group rows (Decision 5); Phase 3 relocates it rather than deleting it.

## Audit Notes (ripls-audit)

Audited against the selected standards; the plan was compliant on structure and phasing, with these fixes applied in-place:

1. **Product-constraint conflict surfaced (now resolved).** `docs/users/priorities.md` makes "No discoverability. No browsing. No strangers." a defining constraint and names the proactive-momentum/catalyst loop the product's compounding differentiator. A "Directory" risked reading as a browsing surface, and removing the Workshop tab risked orphaning the host-facing momentum action. **Both resolved by product** (Resolved Decisions 4a + 6): the surface is an **address book** scoped to existing connections only (never a global/stranger index; search-within-connections), and the momentum engine is retained with its suggestions relocated into the monthly views on Home/Communities/Person profiles.
2. **API design (`proto_conventions.md`).** Specified dedicated request/response, flattened fields, `optional` field-presence (so the client uses `has*()`), and pagination for `GetDirectory`. New RPCs are additive/back-compat. Round-trip tests don't apply (read-only RPCs).
3. **Security/privacy (`security` rubric + `docs/users/profile.md` invariants).** Added explicit authz scoping (`auth.RequireAuth`, membership-only, IDOR boundary), extension of the profile no-leak invariants (no non-shared community IDs on the wire/in logs) to the Directory, and wire-byte leakage tests mirroring `profile_test.go`.
4. **Performance (`performance` rubric).** Pinned no-N+1 via `AssertMaxQueries`, batched membership reads (not per-community `ListCommunityUsers` fan-out), pagination, and a `'directory'` cache namespace invalidated on membership change.
5. **Accessibility (`accessibility` rubric).** Added touch-target minimums (mock's 29px quick-action chips → ≥48×48), `IconAction`/`Tappable` primitives, semantic labels on hero/tiles/dock, and an accessible alternative to the A–Z scrub rail (gesture-only fine-grained target).
6. **Client conventions (`client/conventions.md`).** Required `CachedMediaImage` + stable media-id keys for avatars/cover photos (mock uses gradients; real data is user media), repository cache-key naming, api-protos-only, and the "Event" not "Experience" user-facing terminology rule on the recent-activity strip and directory rows.
7. **Error handling / testing.** Added explicit loading/empty/error states for the new AsyncNotifiers and empty-state coverage (no-connections case) to the test lists.

**Docs consulted:** `docs/llms.txt`; anchors `client/conventions.md`, `proto_conventions.md`, `client/architecture.md`, `server/architecture.md`, `server/conventions.md`, `client/design.md`; review rubrics `api_design`, `security`, `accessibility`, `performance`, `error_handling`, `testing` (Areas to Review); leaf/product docs `docs/workshop.md`, `docs/users/profile.md`, `docs/impact_metrics.md`, `docs/users/priorities.md`, `docs/ad_hoc_communities.md`, `docs/nudges.md`, `docs/client/calendar.md`, `docs/workflows/community_promotion.md`.
