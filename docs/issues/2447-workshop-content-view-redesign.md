# Issue #2447: Redesign the Workshop screen to match the content views (v4.1 photo spec)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2447
**Filed:** 2026-06-15 | **Last activity:** 2026-06-15
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/cowork/App UXR/community-overview-v4.1-photo.html` (design spec, input), `docs/workshop.md`, `docs/client/content.md` (both input + map-selected), plus map anchors `client/architecture.md`, `client/conventions.md`, `proto_conventions.md`, and leaf docs `client/inbox.md` (the #2435 precedent), `client/media.md`, `client/modals.md`, `client/chat.md`, `client/design.md`, `users/profile.md`, `impact_metrics.md`. Review dimensions skimmed: performance (N+1 on the new surfaces), api_design (any new RPC), accessibility (contrast over photo), testing.

## Problem Statement

The Workshop tab today is a plain scrolling `ListView` of profile-section widgets on a
flat background ([workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart)):
community carousel → identity block → known-for chips → impact rows → available-now rail →
action postcards (`LeanCards`). It reads as a utilitarian host dashboard.

The v4.1 spec
([community-overview-v4.1-photo.html](../cowork/App%20UXR/community-overview-v4.1-photo.html))
re-skins and re-orders this into a **community overview** that borrows the **visual language
of the content views** — a full-bleed community photo, a dark gradient scrim, and
glass-morphic cards floating over it — and resequences the content so social proof leads and
numbers follow:

1. **Compact switcher pill** (the scope selector) → opens a **full-screen community chooser**
   in the same photo language (spec phone 3), replacing today's inline carousel + any sheet.
2. **Serif title hero** — the community name in Newsreader, giving the screen identity.
3. **A rotating "glimpse"** directly under the title, no section label: a cycling **chat
   preview** on a single circle; the **community flipper** (circles rotating) on "Everything"
   (phones 1–2).
4. **People** (member rollup faces).
5. **Metrics, numbers only** — hours / saved / CO₂ as a glass card. Equivalence + commentary
   move *into* the metric deep-dives.
6. **"How you can help"** — the `cta_rows` nudges as glass rows (renamed from the postcard list).
7. **Category chips** (specialties / known-for).
8. **Community Library** — a slim newest-items line plus event-style **Where / When** cards
   that open a **library map** (items as pins + distance, phone 5) and a **library calendar**
   (time-bound library items as an agenda, phone 6). This replaces the available-now rail.
9. **Recent Activities dropped** (redundant with the calendar).
10. **Metric deep-dives (phones 7–9)** re-skinned onto the same photo overlay, each carrying
    equivalence + source, a by-month chart, a source breakdown that reconciles to the headline,
    recent contributions, and one nudge.
11. Lighter floating nav; rebalanced scrim so the photo actually shows.

**Important framing note (flag for product):** this is not purely cosmetic. It repositions the
tab from a *host momentum surface* (postcards that launch `GenerateWorkshopDraft`) toward a
*community overview* (chat-first, library-forward). The momentum/detector chain still feeds
"How you can help," so the server pipeline is preserved — but the tab's information architecture
and emphasis change materially. Confirm this is the intended product direction before building.

### What the codebase research established (grounding the plan)

**The visual scaffolding is fully reusable.** Content views are a `Stack` of
[VideoBackgroundHost](../../app/lib/presentation/widgets/media/video_background_host.dart)
(full-bleed photo/video) +
[ContentGradientOverlay](../../app/lib/presentation/widgets/content/content_gradient_overlay.dart)
+ glass cards. Glass primitives already exist:
[`GlassSurface`](../../app/lib/presentation/widgets/modal/glass/glass_surface.dart) and
`GlassInsetCard`. The community-level chat is reachable via `CommunityChatPane` →
`InlineConversationView`.

**Most of the server data already exists.** Verified:
- `GetWorkshopBrief` / `BriefPayload` (proto `workshop_service.proto`) already carries identity,
  `hours_together` / `replaced_cost_usd` / `co2_avoided_pounds` / `unique_people_count`,
  `specialties`, `available_now_items`, and `cta_rows`. **No change needed** for the home body.
- The metric deep-dives are **~95% already built.** `GetCommunityMetricDetail` returns
  `cumulative_trend` + `monthly_bars` (by-month chart), `source_breakdown` (reconciles to the
  headline with per-source `count`), `recent_items`, and equivalence strings. The existing
  [WorkshopTimeDetailScreen](../../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart) /
  Money / Co2 screens already render all of that — they just use a **plain light background**
  and lack the one **nudge** the spec shows. The deep-dive work is mostly a re-skin, not new data.
- **Community chat glimpse** is feasible today: `GetConversationForCommunity(community_id)` →
  `GetConversationHistory(conversation_id, max_messages)`.
- **Library map** can be fed by existing `Search` (semantic + proximity, returns
  `distance_meters`) or `GetNearbyGear`.

**The genuinely new server work is narrow:**
- **A community library calendar** has no single source. `ListExperiences(state=SCHEDULED)`
  covers events, but time-bound **loans** (Transfer `due_back_unix_sec`) and dated **asks** have
  no unified endpoint. Either a new `GetCommunityCalendar` RPC or client-side stitching.
- Optional: a **nudge** field for the deep-dive screens (could reuse a `cta_row`), and optional
  **source citation** metadata on `source_breakdown`.

**A strong in-repo precedent exists.** The Home/Inbox v4.1 redesign (#2435,
[inbox.md](../client/inbox.md)) solved the same shape: one `GetHomeView` RPC assembles all
sections server-side; the root screen shows capped previews; "see all" sub-screens
(`HomeCalendarScreen`, etc.) share the same cached response; widgets live under
`widgets/home/`. The Workshop redesign should mirror this structure for consistency.

## Context from Comments

**The issue has no comments** — it was filed and immediately picked up for planning. All
requirements come from the issue body and the linked HTML spec. The spec itself contains a
"Notes & open questions" section that is the authoritative source of the known unknowns (chat
scope on Everything, fuzzed pins for locationless loans, undated asks on the calendar). Because
there's no maintainer discussion, the **product-direction question above (overview vs. host
surface) and the phasing question should be confirmed before Phase 2+ work begins.**

## Potential Solutions

The central decision is **how to source the redesigned screen's data**, since the visual
scaffolding is a settled reuse of content-view widgets either way.

### Option A: Reuse existing RPCs as-is, assemble on the client
- **Approach:** Keep `GetWorkshopBrief` for the home body, reuse `GetCommunityMetricDetail` for
  the (re-skinned) deep-dives, fetch the chat glimpse via the existing conversation RPCs, and
  feed the library map from `Search`. Add only the calendar data, stitched client-side from
  `ListExperiences` + transfers. No new aggregation RPC.
- **Pros:** Fastest path to the visual redesign; the home + deep-dives need *zero* server work;
  smallest blast radius.
- **Cons:** Multiple client round-trips for the new surfaces (chat glimpse, library map,
  calendar); calendar stitching duplicates logic the server is better placed to own; diverges
  from the #2435 single-RPC pattern.
- **Effort:** Medium.

### Option B: New unified `GetWorkshopView` RPC mirroring `GetHomeView`
- **Approach:** Build a single server-assembled view RPC that returns every section (identity,
  chat glimpse, people, metrics, help rows, chips, library newest + where/when) in one response,
  exactly like `GetHomeView`. Deep-dives + library map/calendar still use their own RPCs as
  "see all" destinations sharing the cached response.
- **Pros:** Consistent with the freshest precedent in the repo; one round-trip for the home;
  server owns assembly + caps; cleanest client view-model.
- **Cons:** Largest server effort; throws away the already-working `GetWorkshopBrief` assembler
  (or wraps it); higher risk to ship the *visual* win, which is the issue's actual ask.
- **Effort:** Large.

### Option C: Hybrid — reuse what exists, add only the missing data, phase it
- **Approach:** Phase 1 ships the **visual redesign of the home + deep-dives on existing data**
  (`GetWorkshopBrief` + `GetCommunityMetricDetail`, both already complete). Phase 2 adds the
  **chat glimpse** and **full-screen chooser** (existing RPCs). Phase 3 adds the **Community
  Library** — map (existing `Search`) and calendar (the one genuinely new server endpoint,
  `GetCommunityCalendar`, or a scoped client stitch). Each phase is shippable.
- **Pros:** Delivers the headline visual win first with near-zero server risk; isolates the only
  real new server work (calendar) into its own phase; matches the issue's own suggested phasing;
  lets the overview-vs-host-surface product question be validated on the cheap Phase 1.
- **Cons:** The home stays on `GetWorkshopBrief` rather than converging on the `GetHomeView`
  pattern (acceptable — they're peer surfaces, not required to share one RPC).
- **Effort:** Large overall, but front-loads value into a Medium first increment.

## Recommendation

**Option C (hybrid, phased).** The issue's real ask is the *visual language* change, and the
research shows that for the home body and the three deep-dives the data is already there — Phase
1 is essentially a re-skin + re-order using `VideoBackgroundHost` + `ContentGradientOverlay` +
`GlassSurface`, reusing the profile-section widgets' data. That ships the headline win with
almost no server exposure and lets us validate the overview-vs-host-surface reframing before
investing in the library. The only material new server work (a community calendar) is correctly
isolated to the last phase, where a `GetCommunityCalendar` RPC mirroring the `GetHomeView`
assembler pattern is the natural home for it.

Reject Option B as the *starting* point: building a new unified RPC up front delays the visual
win behind server work the issue doesn't require. If, after Phase 1, we want to collapse the
home's round-trips, a `GetWorkshopView` RPC can be retrofitted then — but it shouldn't gate the
redesign.

**Cross-cutting constraints to honor throughout:**
- **Theming.** Reuse content-view shared widgets and `AppColors` tokens / `AppTheme` fonts; do
  **not** invent per-section coral/cream/font constants (the spec's CSS hex values are mockup-only
  — map them to existing Ink & Sage tokens, adding named `AppColors` helpers only where a genuinely
  new semantic color is needed). All colors theme-aware via `AppColors.xxx(context)`; no hardcoded
  `Colors.white` / `Color(0xFF…)`. No version-suffixed directories (`v4/`, `v4.1/`) — extend the
  existing `screens/workshop/` and `widgets/workshop/` trees.
- **File size.** Keep every Dart file < 1000 lines (`npm run lint:dart:size`). The new screen will
  be section-heavy — extract each section into a `widgets/workshop/` widget rather than growing
  `workshop_screen.dart`.
- **MVVM/data flow.** All new data access (chat glimpse, library map, calendar) goes through a
  **repository** with namespaced cache keys (`'workshop:…'`), called only from a ViewModel — never
  a service call from a widget/ViewModel directly. Any new async `Notifier` needs a **disposal
  test** and `ref.mounted` guards after awaits (per `client/architecture.md`).
- **Navigation.** New full-screen destinations (chooser, library map, library calendar) use
  `NavigationHelpers.pushScreen` + `SwipeToCloseMixin` (pass `useRootNavigator: true` when launched
  from a modal); the existing deep-dive screens already follow the pattern. No raw
  `Navigator.push(MaterialPageRoute)` / `context.push` (`npm run lint:nav`).
- **i18n.** Every new user-facing string (section headers "How you can help" / "Where" / "When",
  chooser + calendar copy) is ARB-sourced and resolved in the widget layer, never in a ViewModel.
  "Event" not "Experience" in any user-facing string (calendar/library copy especially).
- **Accessibility.** White text over a real photo must hit AA (4.5:1) — lean on the spec's
  rebalanced scrim and verify each backdrop. Decorative photo background hidden from the a11y tree;
  semantic labels on the tappable metric card, switcher pill, chips, and avatar stacks; touch
  targets ≥ 48×48 dp (pills/chips need expanded hit areas). The auto-rotating chat-glimpse / circle
  flipper must honor reduced-motion (no animation, show first item) and be pausable. Use the
  accessibility primitives (`Tappable`, `IconAction`, `LiveRegion`) and run `npm run check:a11y`.
- **Proto (only if a new RPC lands).** A `GetCommunityCalendar` RPC gets its own dedicated
  request/response (no cross-RPC reuse), flattened fields, `optional` for conditionally-present
  fields, and vendor-free proto comments (`proto_conventions.md`); add an `AssertMaxQueries` N+1
  guard on its handler.

## Open Questions

- **Is the tab being repositioned from host-surface to community-overview?** — Recommendation:
  Treat it as a re-skin + re-order that *preserves* the momentum pipeline ("How you can help" is
  still `cta_rows`), not a removal of host features. Confirm with product before Phase 1, since
  it changes how we describe the tab in `docs/workshop.md`.
- **Chat glimpse on "Everything" scope.** The spec shows the community *flipper* (rotating
  circles) on Everything and the *chat* glimpse only on a single circle, because one thread can't
  represent the aggregate. — Recommendation: follow the spec exactly — chat glimpse single-circle
  only; Everything shows the rotating circle highlights. Defer an aggregate "latest across your
  circles" rollup.
- **Library map: locationless loans.** — Recommendation: items without a resolvable
  `location_id` are **listed but not pinned** (shown in the list with "no location" rather than a
  fuzzed pin), avoiding a misleading dot. Revisit a default/fuzzed pin only if the list-without-pin
  reads poorly.
- **Library calendar: undated asks.** — **Decided:** an ask with no explicit date is anchored to
  its **created/shared date** (the earliest of `created_at_unix_sec` / first-shared timestamp) so
  it still appears on the calendar rather than dropping off. This mirrors the Home calendar's
  fallback anchoring (`home_calendar_screen.dart` anchors requests by due/first-shared date). No
  "no date yet" bucket needed.
- **"Everything" has no single cover photo.** — Recommendation: for Everything scope, fall back
  to a stable default backdrop (a neutral house photo) or the highest-signal community's photo;
  don't attempt a live montage in v1. Decide the exact fallback during Phase 1.
- **Calendar data: new RPC vs. client stitch?** — Recommendation: start with a client-side stitch
  (`ListExperiences(state=SCHEDULED)` + active-loan due dates) in Phase 3 to validate the surface,
  then promote to a server `GetCommunityCalendar` if the round-trips or dedup logic justify it.
- **Deep-dive nudge.** The spec shows one nudge per deep-dive; the screens don't have it today. —
  Recommendation: reuse the top-ranked `cta_row` for that community rather than adding a new
  server field.

## Implementation Plan

### Phase 1: Visual redesign of the home + deep-dives (existing data)
**Goal:** The Workshop tab and its three metric deep-dives adopt the photo-background / glass-card
language and the v4.1 section order, using only data we already fetch.
**Tasks:**
- [ ] Confirm the product-direction and Everything-backdrop questions; record the answer in
  `docs/workshop.md`'s plan section.
- [ ] Restructure [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart)
  onto a `Stack`: `VideoBackgroundHost` (community `mediaId`, Everything fallback) +
  `ContentGradientOverlay` + a scrolling body of `GlassSurface` cards, keeping the floating header
  + bottom nav (it stays a nav destination, **not** the content-view tab/pane-switcher pattern).
- [ ] Add the **serif title hero** (community name) and convert the carousel into the compact
  **switcher pill**.
- [ ] Re-order sections per spec: glimpse placeholder → people → metrics (numbers only) → "How
  you can help" (rename `LeanCards` header) → category chips → newest-items line (interim, still
  available-now data). Drop nothing's-data sections gracefully.
- [ ] Re-skin the metric trio as a single glass "numbers only" card; move equivalence/commentary
  out of the overview (it already lives in the deep-dives).
- [ ] Re-skin `WorkshopTime/Money/Co2DetailScreen` onto the photo overlay (dark scrim, glass-dark
  data) and add the **nudge** row (top `cta_row`). No new RPCs.
- [ ] Map all spec CSS colors to `AppColors`; extract each section into a `widgets/workshop/`
  widget to stay under the line cap.
- [ ] Tests: ViewModel unchanged but add/adjust widget-level expectations; disposal test still
  green. Update `docs/workshop.md` "Client rendering" section.
**Validates:** The visual direction and the overview-vs-host reframing, at near-zero server risk,
on real data.

### Phase 2: Full-screen community chooser + chat glimpse
**Goal:** The switcher opens a full-screen chooser in the photo language, and the under-title
glimpse becomes live (rotating chat on a circle, rotating circle highlights on Everything).
**Tasks:**
- [ ] Build the full-screen chooser screen (recent strip, search, A–Z circles + groups, manage,
  create/join) pushed via `NavigationHelpers`, replacing any sheet. Reuse community/sharing
  widgets where possible.
- [ ] Add a chat-glimpse widget fed by `GetConversationForCommunity` →
  `GetConversationHistory(max_messages=N)` through a repository method; auto-rotate with
  reduced-motion respect (mirror the spec's roller behavior).
- [ ] On Everything scope, render the rotating circle-highlights flipper instead of chat.
- [ ] Tests for the new repository method + the chooser/glimpse view-model; widget tests for the
  rotation + reduced-motion path.
**Validates:** The social-first lead and the chooser-as-screen pattern; confirms the chat-scope
decision in practice.

### Phase 3: Community Library — Where/When cards, map, and calendar
**Goal:** The available-now rail becomes a Community Library: a newest-items line + Where/When
cards opening a library map and a library calendar.
**Tasks:**
- [ ] Add the Where/When glass cards + slim newest-items line to the overview.
- [ ] Build the **library map** screen (numbered pins + list + distance + LOAN/ASK type) fed by
  `Search`/`GetNearbyGear`; locationless items listed but not pinned (per open question).
- [ ] Build the **library calendar** screen (week strip + adaptive agenda) — start with a
  client-side stitch of `ListExperiences(state=SCHEDULED)` + active-loan due dates + asks anchored
  by their explicit date or, when absent, their created/shared date. Promote to a
  `GetCommunityCalendar` RPC (mirroring the `GetHomeView` assembler) only if round-trips/dedup
  justify it.
- [ ] Remove the legacy available-now rail once the library supersedes it; confirm Recent
  Activities is gone.
- [ ] Tests for the map/calendar view-models (proximity, date bucketing, empty states); N+1 guard
  if a `GetCommunityCalendar` RPC lands (`AssertMaxQueries`).
- [ ] Update `docs/workshop.md` end-to-end to describe the new IA; refresh `docs/client/content.md`
  if the Workshop now shares its scaffolding.
**Validates:** The library destinations and the calendar data model; closes the locationless-loan
and undated-ask questions.

## Related Issues
- **#2435** — Home/Inbox v4.1 editorial redesign ([inbox.md](../client/inbox.md)). The structural
  precedent: single view-assembly RPC, root preview + shared-cache "see all" screens, `widgets/home/`
  organization. The Workshop redesign should mirror its patterns and is the sibling "v4.1" surface.
- **#1618** — Workshop daily-brief direction ([issues/1618-workshop-daily-brief.md](1618-workshop-daily-brief.md));
  established the current `BriefPayload` + postcard model this redesign re-skins.
- **#1579** — Workshop tab v1 (cascade detector chain). The detector chain feeding "How you can
  help" is unchanged by this work.
- **#2445** — `AppColors` status-getter migration; relevant when mapping the spec's colors onto
  theme-aware tokens.

## Audit notes

Audited against the documentation context map. **Scope:** both sides (client-heavy; one optional
new server RPC). **Engaged dimensions:** client architecture/conventions, accessibility, api_design
(potential `GetCommunityCalendar`), performance (map/calendar N+1), testing, code_quality, i18n,
caching, navigation.

Fixes applied: expanded the cross-cutting constraints with explicit MVVM/data-flow + disposal-test
rules, the `NavigationHelpers` + `SwipeToCloseMixin` requirement for the new pushed screens, an
i18n/ARB rule (plus the existing "Event" not "Experience" rule), a concrete accessibility checklist
(AA contrast, decorative-photo hiding, semantic labels, ≥48 dp touch targets, reduced-motion for the
auto-rotating glimpse), and proto-convention requirements for any new RPC (dedicated req/resp,
`optional` presence, vendor-free comments, N+1 guard). No architectural conflicts found — the plan
reuses content-view scaffolding and existing RPCs, and the only genuinely new server surface
(community calendar) is correctly isolated and gated behind a client-stitch-first step.

**Docs consulted:** `proto_conventions.md`; `client/architecture.md`, `client/conventions.md`,
`client/content.md`, `client/inbox.md`, `client/media.md`, `client/modals.md`, `client/chat.md`,
`client/design.md`; `users/profile.md`; `impact_metrics.md`; `workshop.md`;
`docs/reviews/accessibility_review_prompt.md` (Areas to Review). Selected from `docs/llms.txt` by
the touched paths (`app/lib/presentation/**`, `server/services/workshop/**`, `proto/ripls/api/**`)
and engaged dimensions.
