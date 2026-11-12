# Issue #2675: How to find my event? (+ #2677: Event missing from calendar)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2675 · https://github.com/Type2Fun-Foundation/ripls/issues/2677
**Filed:** 2026-07-08 | **Last activity:** 2026-07-08
**Labels:** user-feedback | **Assignees:** unassigned
**Reporter:** user7@example.org (host of the event) · app 0.26.3, Android 16, Pixel 9 Pro, PROD
**Context docs:** docs/client/calendar.md, docs/client/inbox.md, docs/workflows/experience.md, docs/ad_hoc_communities.md, docs/proto_conventions.md, server/services/portfolio/home_view.go

## Problem Statement

Two user-feedback reports from the same user on the same day describe one bug.
The host of a community event ("Neighborhood Craft Circle", scheduled Fri Aug 21
3:00 PM) opens her community "Travis Heights Early Adopters", sees the event
listed under **ON THE CALENDAR · UPCOMING**, taps it, and is dropped on the
**calendar showing today** with the daily suggestions. Scrolling forward to Aug
21, the event is **not there** ("Nothing planned yet"). #2677 attaches the event
detail screen as proof the event genuinely exists (Aug 21 3:00 PM, she's HOST,
2 going).

The maintainer added a decisive clue: **as a non-host member he sees the event on
his calendar; the host does not.** That turned out to be a red herring about
host-vs-member and is really **inbox-calendar vs community-calendar**: the event
is present on the whole-timeline inbox Calendar but absent from the
community-scoped "When" calendar — and Leslie reached the latter by tapping "ON
THE CALENDAR".

There are two distinct defects:
1. **Primary — event missing from the community calendar.** The event is
   dropped from the community-scoped "When" calendar for *every* viewer (host and
   member alike), because its calendar entry is tagged with the wrong community.
2. **Secondary — tap lands on "today".** Tapping "ON THE CALENDAR" opens the
   community calendar pinned to today, not the event's date, so even absent bug
   #1 the user has to hunt for the event.

## Context from Comments

Both issues have **no GitHub comments** — the only extra signal is the
maintainer's in-session note (non-host sees it, host doesn't). No prior
discussion; requirements come entirely from the screenshots + that clue, both
reproduced below.

## UI Investigation

**Investigated:** 2026-07-08 on `event_fixes`@94f077643 (fresh HEAD rebuild) | **Environment:** hermetic e2e stack (`ripls-ui-investigate`, slug 2675)
**Verdict:** **server-logic** (primary) + **UX-affordance-gap** (secondary). An event's calendar entry is tagged with the event's *nameless per-item ad-hoc "backing" community* instead of the user-facing named community; the community-scoped calendar filters on that single `community_id` by exact match and drops the event. Not host-specific — it's inbox-calendar (unfiltered, shows it) vs community-calendar (filtered, drops it).

### Reproduction
Seeded world: host **Leslie** owns community **Travis Heights Early Adopters**
(`9324055f…`); member **Bryan** joins by invite; Leslie creates
**Neighborhood Craft Circle** with a **SpecificTime** (Aug 21 2026 3:00 PM CDT)
and shares it to Travis Heights; Bryan RSVPs yes.

- The event was auto-shared to **two** communities: Travis Heights (`9324055f…`,
  named) **and** an auto-created, **empty-named** community `b0680f15…` whose
  `origin_experience_id` is the event itself.
- `GetHomeView.calendar` returns exactly one entry for the event, tagged
  `community_id = b0680f15…` (the backing community) for **both** host and member.
- **Community calendar** (Workshop → "Open the library calendar", filtered to
  `9324055f…`): **0 entries → "Nothing planned yet" on Aug 21** — matches the
  reporter's screenshot exactly.
- **Inbox calendar** (Home → "Calendar", unfiltered): **shows "Friday, August 21
  · Neighborhood Craft Circle · Hosting · 2 going"** — event present.
- Community calendar also opened pinned to **today (Jul 8)**, not August (the
  secondary bug).

### Hypotheses tested
| # | Hypothesis | Probe | Observation | Verdict |
|---|---|---|---|---|
| H2 | `experience.time` is TBD (time from a confirmed proposal not written back) → excluded for everyone | rail shows UPCOMING; probe-01 | community "UPCOMING" rail *requires* `experience.time>now`; entry has valid `time=1787342400`, `display="3:00 PM"` | **refuted** |
| H3 | owner-id mismatch makes it host-specific | probe-01 | host **and** member both get the entry, both tagged `b0680f15` | **refuted** (not host-specific) |
| H1 | entry's `community_id` mismatches the viewed community → community filter drops it | probe-01/02/04 | entry `community_id=b0680f15`; filter to `9324055f` → 0 entries; UI shows "Nothing planned yet" | **confirmed** |
| H5 | tap lands on "today", not event date | probe-04 | community calendar opened on Jul 8 | **confirmed** |
| — | inbox (unfiltered) calendar shows it | probe-05 | "Friday, August 21, Neighborhood Craft Circle" present | confirmed (contrast) |

### Evidence
- **DB:** two `community_experience` rows for the event → `{9324055f (Travis
  Heights), b0680f15 (empty name, origin_experience_id = event)}`. Both host and
  member are members of both communities.
- **probe-02 (filter simulation):** filtering `view.calendar` to `9324055f` (the
  Dart provider's exact-match) → **0 entries** for host and member; unfiltered →
  1 entry. Entry `community_id` seen: `b0680f15` only.
- **probe-04 (community calendar UI):** every August day incl. Aug 21 → "Nothing
  planned yet"; panel opened on Jul 8. Screenshot matches issue #2675 screenshot 2.
- **probe-05 (inbox calendar UI):** Aug 21 cell shows the event; detail "FRIDAY ·
  AUG 21 · Neighborhood Craft Circle · Hosting · 2 going".

### Root-cause mechanism
1. Every event is born with a **per-item ad-hoc backing community**
   (`ProvisionPerItemCommunity`, `server/services/experience/save.go:267`, from
   the #2492 phone-first pivot) — a real `Community` row deliberately left
   **nameless** (`IsAdHoc(c) == (c.Name == "")`,
   `server/services/community/adhoc.go:24`) with `origin_experience_id` set, used
   as the event's audience/group-chat. The event is shared into it **and** the
   real named community → two `CommunityExperience` pivots.
2. `assembleHomeCalendar` tags each event entry with
   `communityId := firstID(d.expCommunityIDs[e.Id])`
   (`server/services/portfolio/home_view.go:981`).
3. `d.expCommunityIDs` is built from `QueryByFieldIn(... CommunityExperience)`
   with **no `ORDER BY`** (`fetch.go:553-560`, `storage/protosql_batch.go:97`), so
   the slice order is arbitrary and `firstID` (index 0) frequently returns the
   **nameless backing community** rather than the named one. Nothing filters
   ad-hoc/empty-name communities out of that set.
4. `HomeUpNextEntry.community_id` is a **single scalar**
   (`proto/ripls/api/portfolio.proto:1297`). The community calendar
   (`workshopCommunityCalendarProvider`,
   `app/lib/presentation/screens/workshop/workshop_library_common.dart:65-75`)
   keeps entries where `ids.contains(e.communityId)` — exact match on that one id.
   When the id is the backing community, the entry is dropped from the named
   community's "When" calendar → "Nothing planned yet".
5. Secondary: `WorkshopLibraryCalendarPanel` hardcodes `_selected = today`
   (`workshop_library_calendar_panel.dart:52-58`) and the "ON THE CALENDAR" tap
   passes only community ids, no date (`workshop_screen.dart:393`).

### Fix direction
Stop letting the ad-hoc backing community be the entry's community, and make the
community filter tolerant of an item belonging to several communities. See
Potential Solutions.

### Distilled e2e spec (recommended)
This is a cross-surface invariant unit tests missed, so it earns a browser spec:
*"an event shared to a named community appears on that community's calendar even
when it also belongs to its ad-hoc backing community."* Seed as above; assert the
community calendar (Workshop → Open the library calendar) shows the event on its
date. Ship it green in the fix PR. A Go integration test on `assembleHomeCalendar`
(entry tagged with the named community) covers the server half more cheaply.

## Potential Solutions

### Option A: Prefer the named community for the single `community_id` (server-only)
- **Approach:** Replace `firstID(d.expCommunityIDs[e.Id])` with a helper
  `preferNamed(ids, d.communityNameMap)` that returns the first id whose name is
  non-empty (ad-hoc ⇒ empty name), falling back to `firstID`. Apply at every
  display-community pick in the portfolio assembler (home_view.go:686, 777, 981,
  1280, 1571). `communityNameMap` is already in `fetchedData`, so no extra fetch.
- **Pros:** Tiny, server-only, no proto/regen/client change; low risk; fixes the
  reported case immediately; also fixes the blank community-name label the
  mis-tag causes. High-confidence.
- **Cons:** An event shared to **two named** communities still collapses to one —
  it'd show on only the first named community's calendar. Doesn't model
  multi-community membership.
- **Effort:** Small.

### Option B: Carry all communities on the entry (`repeated community_ids`) + intersecting filter
- **Approach:** Add `repeated string community_ids` to `HomeUpNextEntry`
  (portfolio.proto), populate it from the full `d.expCommunityIDs[e.Id]` set (and
  the analogous full sets for asks/gear/obligations), regenerate, and change the
  Dart community filter to `if (e.communityIds.any(ids.contains))`. Keep
  `community_id` as the display community, chosen via Option A's `preferNamed`.
- **Pros:** Correct for events genuinely cross-shared to multiple **named**
  communities (a real feature); deterministic; the calendar surfaces the event in
  every community it belongs to. Subsumes Option A for the display label.
- **Cons:** Proto change + regen + client change; more surface; needs a
  client+server ship together (old clients ignore the new field — degrade to
  current behavior).
- **Effort:** Medium.

### Option C: Deep-link the community "ON THE CALENDAR" tap to the event's date (secondary)
- **Approach:** Pass an initial selected date into `WorkshopLibraryCalendarPanel`
  — the soonest upcoming community calendar entry's day (or the tapped event's
  day) — instead of hardcoding today.
- **Pros:** Fixes the "takes me to today, event's not there" half of the report;
  small, client-only.
- **Cons:** Cosmetic without A/B (you'd land on the right day but the event still
  wouldn't be there). Must ship after (or with) A/B.
- **Effort:** Small.

## Recommendation

**Ship Option A first (Phase 1) to resolve the reported bug immediately, then
Option B (Phase 2) for full multi-community correctness, then Option C (Phase 3)
for the navigation gap.**

Rationale: Option A is a small, server-only, high-confidence change that fully
resolves the reported scenario (an event in one named community + its ad-hoc
backing community) and the class of "ad-hoc community wins the label" bugs, with
almost no risk. But cross-community event sharing is a real feature, and under A
alone an event shared to two *named* communities is still missing from one — so
Option B is the correct end state and A's `preferNamed` is exactly the display
picker B also needs. Option C is genuinely part of the user's confusion but is
inert until the event actually appears, so it comes last. Do **not** remove the
ad-hoc community from `expCommunityIDs` — it is a legitimate community the event
belongs to; the fix is to choose/label correctly (A) and match against the full
set (B).

## Open Questions

- **Does an event ever legitimately need to appear on the ad-hoc backing
  community's own calendar?** — Recommendation: yes, keep it in `community_ids`
  (Option B) so it still matches if someone views that group's calendar; just
  never let it be the *display* community when a named one exists. No behavior is
  lost.
- **Should Phase 1 ship alone, or wait for Phase 2?** — Recommendation: ship
  Phase 1 alone as the hotfix (server-only, unblocks PROD users), then Phase 2 as
  a follow-up PR. Phase 1 is a strict improvement and safe to release
  independently.
- **Which date should Option C land on?** — Recommendation: the **soonest
  upcoming** entry for that community (fallback: today), computed client-side from
  the already-fetched community calendar data, rather than threading the tapped
  event's date through — simpler and robust when a community has several events.
- **Is the nondeterminism causing intermittent *correct* behavior in PROD?** —
  Recommendation: assume yes (unordered query). This explains why some
  events/communities look fine and others don't; the fix removes the dependence
  on row order entirely.

## Implementation Plan

### Phase 1: Prefer the named community (server-only hotfix)
**Goal:** The event appears on its named community's calendar; ship to PROD.
**Tasks:**
- [ ] Add `preferNamed(ids []string, names map[string]string) string` in the
  portfolio package: first id whose `names[id] != ""`, else `firstID(ids)`. The
  non-empty-name test is the **canonical ad-hoc discriminator**
  (`community.IsAdHoc(c) == (c.Name == "")`, `docs/ad_hoc_communities.md`) — key
  on name presence, do not invent a new signal. This also makes a
  **promoted** (named) former-ad-hoc community a valid pick automatically.
- [ ] Replace `firstID(d.expCommunityIDs[...])` / analogous display-community
  picks at home_view.go:686, 777, 981, 1280, 1571 with `preferNamed(...)`. Note
  the same mis-tag currently also blanks `community_name`
  (`d.communityNameMap[adhocId] == ""`), so this fixes the label too.
- [ ] Fallback behavior to state in the test: a host-only event that is in
  **only** its ad-hoc community (no named community yet) still resolves to that
  community via `firstID` — correct; it simply has no named calendar to appear on
  and still shows on the unfiltered inbox calendar.
- [ ] Go integration test on `assembleHomeCalendar`: event shared to a named
  community + its ad-hoc backing community → entry `community_id` is the **named**
  community; the community-filtered projection includes the event. (Server tests
  run locally — unit + integration — per `docs/server/conventions.md`.)
- [ ] Verify locally (server tests + a re-run of the hermetic community-calendar
  probe) that the community calendar now shows the event on Aug 21.
**Validates:** Removes the reported symptom for single-named-community events and
the blank-community-name label the mis-tag also causes.

### Phase 2: Carry all communities + intersecting filter (full correctness)
**Goal:** Events cross-shared to multiple named communities appear in each.
**Tasks:**
- [ ] Add `repeated string community_ids` to `HomeUpNextEntry` (portfolio.proto):
  use the **next free field number** (no reuse/gap — api_design review), declare
  it **adjacent to `community_id`/`community_name`** in source order (proto
  field-order convention — group semantically related fields, not append-at-end),
  and give it a proto comment. `HomeUpNextEntry` is intentionally shared by both
  `up_next` and `calendar` (`docs/client/calendar.md`), so the field rides both
  harmlessly. Regenerate Go + Dart. Keep `community_id` as the `preferNamed`
  display value.
- [ ] Backward compatibility: old clients ignore the new repeated field and keep
  matching on the single `community_id` — which Phase 1 already made the *named*
  community, so they degrade to correct-for-the-common-case, not to the bug.
- [ ] Populate `community_ids` from the full `d.expCommunityIDs[e.Id]` (and the
  ask/gear/obligation equivalents) in the assemblers.
- [ ] Change `workshopCommunityCalendarProvider` to keep entries where
  `e.communityIds.any(ids.contains)` (fallback to `community_id` when
  `community_ids` is empty, for entries from an older server).
- [ ] Distilled e2e spec: event shared to a **named** community shows on that
  community's calendar (the cross-surface invariant unit tests missed). No new
  l10n strings — reuses existing `workshopCalendarNothingPlanned` /
  `homeCalSuggestPlan`; server renders no calendar copy, so the l10n render
  boundary is unaffected.
- [ ] Dart provider test for the intersecting filter.
**Validates:** Multi-community events are no longer dropped from any of their
communities' calendars; behavior is order-independent.

### Phase 3: Land the community calendar on the event's date (nav gap)
**Goal:** Tapping "ON THE CALENDAR" opens on a day that has the event.
**Tasks:**
- [ ] Give `WorkshopLibraryCalendarPanel` an optional initial-selected-date;
  default it to the soonest upcoming community entry (else today).
- [ ] Wire the "ON THE CALENDAR" tap (`workshop_screen.dart:393`) to pass it.
- [ ] Widget/golden or e2e assertion that the panel opens on the event's month/day.
**Validates:** Resolves the "takes me to today… it's not there" half of #2675.

## Related Issues
- #2677 — same bug, host's confirmation that the event exists (event detail
  screenshot). Close together / as duplicates of one fix.
- #2492 — phone-first pivot introduced the per-item ad-hoc communities that this
  bug is a downstream consequence of (context, not a regression to revert).

## Audit Notes

Audited against the documentation context map. **Verdict: compliant** after the
Phase 1/Phase 2 refinements above.

- **Ad-hoc community model (`docs/ad_hoc_communities.md`):** the fix keys the
  named-vs-ad-hoc decision on the canonical `IsAdHoc(c) == (c.Name == "")` signal
  and does **not** invent a new discriminator or strip the ad-hoc community from
  the item's community set — consistent with the doc's "name-presence is the whole
  signal" and "promote-by-naming" model (a named former-ad-hoc community becomes a
  valid pick automatically).
- **Proto/API (`docs/proto_conventions.md`, api_design review):** Phase 2 adds a
  `repeated string community_ids` to the intentionally-shared `HomeUpNextEntry`
  (documented reuse in `docs/client/calendar.md`, not a per-RPC request/response
  type — CLAUDE.md's no-reuse rule is not violated); flagged next-free field
  number, adjacent placement, proto comment, and old-client backward compat.
- **Calendar architecture (`docs/client/calendar.md`):** fix preserves the
  single server-assembled timeline feeding both the inbox and community calendars;
  only corrects which community each entry is attributed to and how the community
  filter matches. No client date/time invention introduced.
- **Testing (`docs/server/conventions.md`, testing review):** server logic covered
  by a local Go integration test on `assembleHomeCalendar`; the cross-surface
  invariant gets a distilled e2e spec (the class of bug unit tests missed);
  intersecting filter gets a Dart provider test.
- **Performance:** `preferNamed` is O(n) over a tiny slice against an
  already-fetched `communityNameMap` — no new queries, no N+1.
- **Security / observability / error handling:** no auth, PII, secrets, outbound
  HTTP, or new failure modes introduced; display-only community attribution.
- **l10n render boundary (`feedback_l10n_render_boundary`):** no new user-facing
  strings; existing calendar copy reused; server renders no calendar text.

**Docs consulted:** docs/llms.txt, docs/client/calendar.md, docs/client/inbox.md,
docs/ad_hoc_communities.md, docs/proto_conventions.md, docs/workflows/experience.md,
docs/reviews/api_design_review_prompt.md, docs/server/conventions.md (testing/undo-registry).
