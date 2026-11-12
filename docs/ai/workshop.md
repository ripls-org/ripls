# Workshop Gap Analysis & Phase 2 Plan

**Date:** 2026-04-29
**Status:** Planning — do not implement yet
**Superseded in part:** the idle-offer cascade slot and its `propose_share`
lever, referenced throughout below, were deleted in #2892. The body of this
document is left as the April record of what was planned; see
[docs/workshop.md](../workshop.md) §5 for what shipped and why it went.
**Related plans:**
[docs/issues/1579-workshop-tab.md](../issues/1579-workshop-tab.md),
[docs/issues/1580-esm-cards.md](../issues/1580-esm-cards.md),
[docs/workshop.md](../workshop.md)
**Reference design:** [docs/cowork/App Design/impact-proactive-host.jsx](../cowork/App%20Design/impact-proactive-host.jsx)
**Brief:** [docs/cowork/App UXR/impact-screen-proactive-host-brief.md](../cowork/App%20UXR/impact-screen-proactive-host-brief.md)

---

## 1. Executive Summary

Issues #1579 and #1580 shipped — the Workshop tab exists, the cascade
detector chain runs, ESM aggregation works, the Crew strip renders, and
the Catalyst-pull review screen is wired. **But the visible surface lands
well short of the JSX reference design.** A host opening the Workshop
today sees a thin list of image-and-headline tiles with chevron CTAs
that route into existing detail screens. They do **not** see:

- the **crew member avatars** with load-overlay highlights,
- the **atmosphere line** under each Hero card ("Round 5 hosted by Mike
  · last Saturday at 9"),
- a **"Together this season"** synthesis block,
- a **"This week need"** ask (e.g., "Tom needs an airport ride Friday"),
- a **circle pill** with its own colour dot,
- a **circle chip** on tiles when scoped to *All*,
- the **All-Circles crew summary** that replaces the per-circle Crew
  strip when *All* is selected,
- a **survey/poll Hero card** (cascade #9 — `isSurveyCard`),
- a **recap-and-thanks Hero card** (cascade #10 — `isRecapCard`),
- a **verdict-after-pulse Hero card** (cascade #11 — `isVerdictCard`),
- a **bridge-invite Hero card** (cascade #15 — `isBridgeCard`),
- the **early-decay** / **format-refresh** / **pause-gracefully**
  cascades (#12, #14, #16),
- the **lapsed-regular re-invite** Hero card (#13),
- a working path to **create an ESM prompt from inside the app** (only a
  developer runbook exists today),
- a working path to **create an event from a lever tap** (the CTA opens
  the existing item screen, not a pre-populated draft).

Plus two architectural gaps that block multiple surfaces at once:
**no AI-driven copy generation** (templates only), and
**no scheduled background generation job** (detection runs only when
the host opens the tab).

This plan inventories every gap, proposes options for each, captures
the user's resolved decisions in §5, and lays out a three-phase
execution. Estimated total scope is **~8 weeks** of focused work
(Phase 1: ~3 wks, Phase 2: ~2.5 wks, Phase 3: ~3 wks). The
per-attendee cascades (#12 decay, #13 lapsed regular, #14 refresh,
#15 bridge invite, #16 pause) are explicitly deferred until the
per-attendee detection model lands.

**Load-bearing structural decision (Decision 2 in §5):** the Hero
card model shifts from "one cascade winner per host" to **one Hero
card per recently-completed event**, plus drift Hero cards for
calendar-gap / seasonal / idle-offer signals that don't anchor to a
recent event. This is the single biggest change from #1579's
original architecture and threads through every phase.

---

## 2. What Currently Exists (audit baseline)

A thorough sweep of the codebase against the two issue plans confirms
that the **infrastructure is real and working** — there are no stubs in
the cascade engine, the catalyst eligibility logic, the ESM aggregation,
or the proto contracts. The implementation gap is in **content surface
coverage**, not in the underlying machinery.

**Server (in place):**

- `server/momentum/` — five real detectors (active quest, ESM repeat,
  calendar gap, seasonal, idle offer) + `bring_back.go` + `lever_copy.go`.
- `server/catalyst/` — load detection, eligibility filtering, rate
  limit + decline cooldown logic.
- `server/esm/` — prompt + response storage, `Aggregate()`,
  `BuildPromptForExperience`, `AttendeeUserIDs`.
- `server/services/esm/` — `RespondToESMPrompt` and `GetESMSignal` RPCs
  with host-only auth on the read.
- `server/services/workshop/` — `GetWorkshopFeed` RPC, on-demand
  `EnsureQuestHero` + `EnsureBringBackItems` generation, `buildCrewStrip`.
- Proto contracts: `workshop_service.proto`, `esm_service.proto`,
  `models/esm.proto`, `models/nudge.proto` with `NudgeSurface` enum and
  workshop-specific `cta_action` values (`schedule_repeat`,
  `revive_experience`, `propose_share`, `seed_subhost`).

**Client (in place):**

- `app/lib/presentation/screens/workshop/workshop_screen.dart` — circle
  pill carousel + ListView of tiles.
- `WorkshopTile` (nudge or story; quest-hero / bring-back /
  seed-catalyst / season-hero variants).
- `CrewStrip` widget (Case A / Case B copy) + `CatalystPullReviewScreen`.
- `NudgeDetailScreen` and `StoryDetailScreen` slide-from-right wraps.
- `EsmPromptContentView` dispatched in `feed_screen.dart` for
  `FEED_ITEM_TYPE_ESM_PROMPT`.
- `EsmRepository` + `WorkshopRepository` (transparent caching).
- "Your impact" entry in settings hub → `PortfolioMetricsScreen`.
- l10n strings (en + es) for the workshop and ESM surfaces.

**Why "almost nothing shows up" anyway:**

1. The cascade detectors only fire when the host has data that matches
   their rules (active recurring experience, calendar gap on a 3+ name
   match, ESM signal ≥60% on a recent prompt, etc.). In a fresh dev
   account, none of these match, so the screen is empty.
2. Even when nudges fire, the tile is a single image-with-headline —
   it does not display the **member avatar row**, the **atmosphere
   line**, or the **circle chip** that make the JSX mock feel
   information-dense.
3. ESM signal in production requires a developer to run the manual
   authoring runbook (`docs/runbooks/esm-manual-authoring.md`); no
   in-app affordance exists.
4. Tapping a Hero card CTA routes into the existing
   `ExperienceItemScreen` or `GearItemScreen` rather than a draft
   experience preview with prefilled date / title / recipient list.

The plan below addresses each of these.

---

## 3. Gap Inventory

The reference JSX is the source of truth for "what the tab is supposed
to look and feel like." Each gap below names the surface, points at the
JSX section that defines it, and explains what the current Flutter
build is missing.

### 3.1 Hero card model: one per recently completed event, plus the tile is too thin

**Decision (2026-04-29):** Drop the "single highest-priority slot
wins" model. The Workshop should render **one Hero card for every
recently completed event** the host owns, plus zero or more *drift*
Hero cards for signals that don't anchor to a recent event (calendar
gap on a long-quiet rhythm, seasonal anniversary, idle gear offer).

**Why this is a structural shift.** The original plan in #1579
treated cascade priority as a chain that *picks one* —
`DetectHighestPriority` walks the chain and returns the first
non-nil hit, so the Workshop ever shows a single Hero card per
(host, community). The new model flips that: cascade priority
decides **which kind of Hero card** to render *for a given event*,
but every recent event gets one.

**Implications.**

- `EnsureHeroCard` (renamed from `EnsureQuestHero` — see Phase 1
  rename pass) iterates over the host's recently-completed events
  in the circle (last 14 days, configurable) and materializes one
  `StoredNudge` per event. The detector chain runs *per event*:
  active follow-on quest? ESM repeat signal? otherwise fall back
  to a recap-shaped "Schedule it again?" card so every recent
  event always produces something.
- `context_id` (§4.16) becomes load-bearing — every Hero card
  carries the experience id it's about, used for dedup against
  the Bring-Back row and for routing the lever to a draft
  prefilled from that event.
- Drift Hero cards (calendar gap, seasonal, idle offer) use a
  separate generation path that runs once per circle, not per
  event, and dedup by `context_id` against the per-event set.
- The proto field `quest_hero: NudgePayload?` becomes
  `hero_cards: list<NudgePayload>` on `GetWorkshopFeedResponse`.
  Per-event cards stack first (most recent first), drift cards
  follow.
- The `NUDGE_SURFACE_WORKSHOP_QUEST_HERO` enum value is renamed
  `NUDGE_SURFACE_WORKSHOP_HERO_CARD` (Phase 1 rename pass).

**JSX reference:** `function QuestHero` ([impact-proactive-host.jsx:1948–2181](../cowork/App%20Design/impact-proactive-host.jsx#L1948-L2181))

The mock's Hero card stacks four discrete information layers:

1. **Kicker** in olive/warm caps ("On the way", "Worth doing again",
   "Things this circle has loved", "This time last year", "Asked about
   lately").
2. **Headline** in serif (the full narrative, ~2 sentences).
3. **Atmosphere line** in micro-label sans ("Round 5 hosted by Mike ·
   last Saturday at 9" — names + date + role).
4. **Lever button** in dark olive at the bottom — full-width, pill
   shape, the *only* interactive affordance.

Plus an evidence bar (e.g., *"Recurring weekly contact predicts
well-being more than total time spent."*) on cascades #3 onwards.

**Current `WorkshopTile` ([app/lib/presentation/widgets/workshop/workshop_tile.dart:129](../../app/lib/presentation/widgets/workshop/workshop_tile.dart#L129)):**

- Renders only `headline` + a tiny `ctaLabel` chevron-row at the bottom.
- No kicker, no atmosphere line, no evidence bar.
- No styled lever button — the entire card is one Tappable.
- Variant differences are only height (220 vs. 160).

**What the data already supports:** `NudgePayload.headline`,
`NudgePayload.description`, `NudgePayload.ctaLabel` are all populated
by the momentum generator. The kicker is currently baked into
`headline` (e.g., "On the way — Round 6 of the smoker"); the atmosphere
line has nowhere to go.

### 3.2 No member avatar row in CrewStrip

**JSX reference:** `function CrewStrip` ([impact-proactive-host.jsx:2361–2537](../cowork/App%20Design/impact-proactive-host.jsx#L2361-L2537))

**Decision (2026-04-29):** the Crew strip should show **every member
of the community**, not the JSX's 6 + "+5 more". The strip
horizontally scrolls when the count exceeds the screen width.
Initials-only avatars first; progressively enrich to real avatars
via the existing `mediaUrlProvider` (same pattern Chat uses today).
The host's own avatar is dark-olive-filled; if Case A or Case B is
active, the over-loaded member's avatar gets a warm-orange ring.

The mock's row of 6–7 avatars is a layout reference, not a cap.
Above the row, a micro-label reads
"Crew · Backcountry" with a coloured dot, and a count "11 in this
circle" is right-aligned.

**Current `CrewStrip`:** renders only the headline + lever button when
`overlay` is set; no avatar row, no member count, no per-member
highlight.

**What the data needs:** `CrewStripPayload` already carries the
suggested user's name, but does **not** carry an ordered list of crew
members to render. The server would need to fetch community members
for the active circle and include their `user_id` + `display_name` (so
the client can render initials and resolve avatars via the existing
`mediaUrlProvider`).

### 3.3 No atmosphere / evidence text on tiles

The atmosphere line ("Round 5 hosted by Mike · last Saturday at 9")
and the optional evidence quote
(*"Showing up imperfectly is what builds a habit."*) are load-bearing
in the brief — they're how the Workshop *narrates* what the user is
seeing, not just *announces* it.

**What the data needs:** a structured place on `NudgePayload` to carry
an atmosphere string and (optionally) an evidence string + attribution.
Today these would have to be stuffed into `description`, which loses
typography.

Two clean options:
- **Add `atmosphere_line` + `evidence` fields to `NudgePayload`** (and
  to `StoredNudge`), populated by the detector at materialization time.
- **Re-purpose existing `description` as the atmosphere line** and add
  a separate `narrative` field for the long-form headline body. (More
  invasive — `description` is already used by feed nudges.)

### 3.4 Lever button styling is missing

The mock's lever button is the brand affordance — full-width, dark
olive, white sans-serif text, pill shape, with a chevron implied by
the layout (no glyph). It's the only place a host taps to take an
action.

**Current state:** the entire tile is a Tappable; there is no
visually-distinct lever button. Tapping anywhere on the tile pushes
the detail screen, where a separate CTA dispatcher fires the action.

**Why it matters:** the brief is explicit that **the lever is the
product**. Folding it into "tap the whole tile" loses the affordance
gestalt the brief invests in.

### 3.5 Circle chip on tiles when scoped to All

**JSX reference:** `function CircleChip` ([impact-proactive-host.jsx:1001](../cowork/App%20Design/impact-proactive-host.jsx#L1001)) and the conditional `showChip` check inside `QuestHero`.

When the active pill is *All*, every tile in the JSX gains a small
coloured chip in the top-right showing which circle that tile belongs
to (e.g., a brown dot + "Backcountry"). This is how the host parses a
mixed feed.

**Current state:** the active pill defaults to *All*, but tiles do not
display any circle attribution. The user cannot tell whether a Quest
hero is for the family circle, the backcountry crew, or the block.

**What the data needs:** `NudgePayload.communityId` (already on
`StoredNudge`, may not be on the wire-side payload — verify) plus a
client-side lookup of the community's display name + colour (already
available via `selectedCommunityProvider` and `communitiesProvider`).

### 3.6 No "Together this season" surface

**JSX reference:** `function TogetherThisSeason` ([impact-proactive-host.jsx:3035–3168](../cowork/App%20Design/impact-proactive-host.jsx#L3035-L3168))

A three-panel synthesis block at the bottom of the screen (when not
"hot day"):

1. **What this circle has done** — narrative summary of recent
   activity ("The Sunday smoker is your through-line.").
2. **What sharing replaced** — aggregate cost / CO₂ / hours-back
   estimate ("About $340 stayed off the rental shelf.").
3. **Together-time** — hours-together estimate ("Twenty-three hours
   together this month.").

Each panel is serif lede + serif body, aggregate-only, never
per-person, and ends with the disclosure *"Aggregate only — no
person named, no scoreboard."*

**Current state:** does not exist on the Workshop screen.
`PortfolioMetricsScreen` shows a different (richer, dashboard-style)
view of similar data — but the brief is explicit that the Workshop
needs its own tighter, narrative-shaped surface.

**Decision (2026-04-29):** Reuse the existing impact metrics
(`server/services/portfolio/` — counts, replaced-cost, CO₂ avoided,
together-time) rather than building a parallel pipeline. Surface
them through AI-generated *contextualizing sentences* — the metric
lives unchanged underneath, the prose around it gets generated per
circle ("About $340 stayed off the rental shelf — drills, the
pressure washer, the snake auger borrowed instead of rented.")
The Workshop adds a thin `GetWorkshopSynthesis(circle_ids[])` RPC
that joins the existing portfolio aggregates with per-circle
activity context (recent rhythm names, top contributors) and
hands them to the AI generator.

### 3.7 No "This week need" surface

**JSX reference:** `function ThisWeekNeed` ([impact-proactive-host.jsx:2900–2948](../cowork/App%20Design/impact-proactive-host.jsx#L2900-L2948))

A one-line "ask of the circle" surface that appears on hot days only
("Tom needs an airport ride Friday at 6am. · 1 ask · 0 claims yet").
The lever ("Nudge the early risers?") opens a 4-person targeted poll.

**Current state:** does not exist. Closest existing primitive is the
`Request` model — community members can ask for help, but the Workshop
does not curate or surface those requests.

**What the data needs:** a detector that scans open `Request` rows
for the host's circles, picks the most under-claimed ones (1+ asks,
0 claims, time-pressure within ~7 days), and surfaces them as a
"this week need" card. Could be a 6th cascade slot on the existing
detector chain or a separate small surface.

### 3.8 No All-Circles crew summary

**JSX reference:** `function AllCirclesCrewSummary` ([impact-proactive-host.jsx:2544–2641](../cowork/App%20Design/impact-proactive-host.jsx#L2544-L2641))

When the active pill is *All*, the Crew strip doesn't make sense
(load is per-circle), so the JSX swaps it for an "Your circles" panel
listing each circle with a one-line tension hint ("You've hosted
last 3 · Mike eligible", "Sunday brunch quiet · Diego asked").

**Current state:** when *All* is selected, the Workshop simply omits
the Crew strip — there's no replacement surface.

**What the data needs:** a per-circle summary that the existing
catalyst + cascade detectors already have most of the inputs for. The
right shape is probably an additional field on `GetWorkshopFeedResponse`
(`circles_summary: list<CircleTension>`) populated only when scope is
*All*.

### 3.9 Hero card variants (#9–#16) entirely missing

The JSX defines twelve cascade scenarios beyond the original five.
None of them are wired today. They split into two families:

**Phase 4 / 5 / 6 cascades** (cf. JSX cascade keys
`cascade10_survey`, `cascade11_recap`, `cascade12_verdict`,
`cascade13_decay`, `cascade14_lapsed`, `cascade15_refresh`,
`cascade16_bridge`, `cascade17_pause`, plus the seed-catalyst case):

| # | JSX key | Surface | Status |
|---|---------|---------|--------|
| 9 | `cascade10_survey` | Survey/poll Hero card — author a 2–3-option question to the circle | **Missing** — would let a host create an ESM prompt without a runbook |
| 10 | `cascade11_recap` | Recap-and-thanks Hero card — system-drafted Story, host reviews and publishes | **Missing** — the once-per-season hero gestures at this but no detector emits |
| 11 | `cascade12_verdict` | Verdict-after-pulse Hero card — "8 of 11 said yes — propose weekly?" | **Missing** — could derive from existing ESM aggregation |
| 12 | `cascade13_decay` | Early-decay signal — "9 → 7 → 5 last three runs · pulse softening" | **Missing** — needs per-attendee attendance trend detection |
| 13 | `cascade14_lapsed` | Lapsed-regular re-invite — 1:1 personal invite to someone who drifted | **Missing** — needs per-attendee detection + a different recipient model |
| 14 | `cascade15_refresh` | Format-refresh — "Pulse went flat — try Saturday breakfast?" | **Missing** — needs free-text ESM signal aggregation |
| 15 | `cascade16_bridge` | Bridge-invite — cross-circle vouched invite | **Missing** — needs cross-circle person-graph + interest signal |
| 16 | `cascade17_pause` | Pause-gracefully — wind down with dignity | **Missing** — needs a "is this dead?" detector |
| 17 | `seedCatalyst` | Soft seed when no catalyst is eligible | **Partial** — `NoEligibleRecipient` flag exists, no nudge generation wired |

Most of these depend on infrastructure not yet built (per-attendee
detection, AI copy generation, cross-circle bridge graph). They're
rightly scoped to follow-on work, but the plan should name them so
they don't get forgotten.

### 3.10 No in-app ESM authoring

**JSX reference:** `cascade10_survey` ([impact-proactive-host.jsx:292–336](../cowork/App%20Design/impact-proactive-host.jsx#L292-L336))

The brief and the issue plan #1580 explicitly defer "operator CLI" to
v2 and ship a runbook in v1. But the JSX shows an **in-app survey
authoring path**: the host taps "Worth doing again, the smoker?" lever
on the Hero card, gets a bottom sheet with 4 templated question
suggestions, picks one, edits it, and sends it as an ESM prompt to
their attendees.

**Current state:** the ESM card surface in the feed (`EsmPromptContentView`)
works for **respondents** but there is no host-side authoring UI. The
runbook at `docs/runbooks/esm-manual-authoring.md` is the only path.

**Two ways to interpret this gap:**

- The user's complaint that "there's no ability to create esm" is
  literal — they want a button that lets a host write an ESM prompt
  from inside the app. This is the "v2 automated trigger" deferred
  by #1580, but a manual host-side authoring surface is its own
  intermediate solution.
- Or it's a complaint about the broader cascade — they were expecting
  ESM signal to *appear* in the Workshop and saw none, because no
  prompts have been authored.

Either way the answer is the same: ship a host-side authoring surface
*before* the v2 automated trigger, because (a) it's a smaller change,
(b) it gives the brief's "ask the circle" lever a real implementation,
and (c) it lets us validate the prompt flow with real hosts before
committing to automated triggers.

### 3.11 No event creation pre-fill

**JSX reference:** `function BottomSheet` ([impact-proactive-host.jsx:3420 onwards](../cowork/App%20Design/impact-proactive-host.jsx#L3420)) — "Schedule round 6" sheet body

When the host taps "Schedule round 6 — Saturday at 9?" in the JSX, a
review sheet slides up with:

- a **prefilled draft message** ("Hey y'all — round six of the smoker
  series is ready. Same Saturday slot, 9pm. Most of you said let's do
  it again. Easy yes if you're in.")
- **defaults** as chips ("Sat May 2", "9:00 PM", "Same crew")
- a **time picker** with the three most likely slots
- a **send button** that publishes the experience

The host edits or accepts.

**Current state:** tapping `schedule_repeat` in
`nudge_content_view.dart:113` calls
`NavigationHelpers.pushToItemScreen` with the `contextId` of the prior
experience. This routes to **`ExperienceItemScreen`**, which is the
**read-only detail screen for the prior instance** — not a creation
modal with a prefilled draft.

The user's complaint that "there's no ability to create events" maps
directly here. The lever lands on the wrong screen.

**What the data needs:** Either:

- A **`GenerateWorkshopDraft` RPC** that takes a `nudge_id` and returns
  a fully-populated draft (title, time, recipient list, body) which the
  client routes into `ExperienceCreationModal` with prefilled fields, or
- A **client-side translator** that, given a Workshop nudge, opens the
  existing creation modal pre-populated from the nudge's `context_id`
  + recent-instance metadata (less faithful to the brief's "AI-drafted
  message body" but ships sooner).

### 3.12 No AI copy generation

The momentum detectors emit static template copy: `"On the way"` / `"Round %d of the {experience}"` / `"Schedule round %d?"`. The brief's
whole pitch is that the copy reads like the host wrote it themselves
("Hey y'all — round six of the smoker series is ready. Same Saturday
slot, 9pm…"). Static templates can't produce that.

**Where AI generation should land:**

- During detector materialization — the detector identifies the
  signal, the AI provider generates the headline / atmosphere /
  body / lever copy / draft message tuned to the circle's voice.
- The existing nudge pipeline at `server/services/feed/nudges.go`
  already calls `server/ai/` for feed nudges; the Workshop generator
  needs the same plumbing.

This is the single highest-leverage gap: lighting it up improves
every detector at once.

### 3.13 No scheduled background generation

`GetWorkshopFeed` runs `EnsureQuestHero` and `EnsureBringBackItems` on
demand. If a host doesn't open the tab, no nudges are generated, and
no push notifications fire. The brief's flow assumes the Workshop
populates itself in the background so a notification can pull the
host back when something needs attention.

**Effect:** the Workshop surface only refreshes when a host actively
visits it. There's no proactive notification ("Round 6 is ready —
schedule it?") because the nudge that would feed the notification
doesn't exist until the host opens the tab.

### 3.14 Story / season-summary generation not wired

`StoryPayload` is defined and `GetWorkshopFeedResponse.season_hero`
threads it through. No detector or pipeline emits a `season_summary`
Story today. The recap-and-thanks Hero card (cascade #10) is the
same gap viewed from a different angle.

### 3.15 Per-attendee detection model missing

Steps 23 (decay), 24 (lapsed regulars), 26 (bridge) in
[docs/users/host-journey.md](../users/host-journey.md) all need
per-attendee detection — who's missed how many recent instances,
whose attendance is trending down, who's a good cross-circle bridge.

Today the detectors operate at the rhythm level (counts of
same-named experiences). A per-attendee model + the queries built
on top would unlock most of cascades #12–#16.

### 3.16 No `context_id` on `StoredNudge`

Already documented in [docs/workshop.md](../workshop.md) Known
Limitations. Cross-surface dedup uses `cta_label` exact-match as a
deterministic stand-in. Many of the new surfaces above (recap,
verdict, refresh) are scoped to a specific experience; without
`context_id`, the dedup logic gets baroque.

### 3.17 No surfacing-discipline / day-density logic

The JSX defines three day-density modes (hot / average / quiet) that
collapse the screen down to 1–3 active sections based on signal
volume. Today the Workshop just renders whatever exists; there's no
collapse logic.

This matters most when the recap, verdict, season-summary, week-need,
and together-this-season surfaces all start firing — without
collapse, the screen becomes an information dump.

---

## 4. Solutions per Gap

For each gap, I propose 2–3 options ordered by cost.

### 4.1 Hero card tile is too thin → enrich `WorkshopTile`

**Option A — Render kicker + headline + atmosphere + lever from existing fields.** The detectors already have all this content; today they pack it into `headline`. Split it into structured fields on `NudgePayload` (`kicker_label`, `narrative`, `atmosphere_line`) at the proto level and re-author the detector copy. Render the structured tile.

**Option B — Add the structured fields but keep generating from templates.** Same proto change, narrower copy investment.

**Option C — Punt. Ship the thin tile and bet that AI-driven copy in §4.12 makes the existing tile feel rich enough.**

**Recommendation:** A. The proto change is small (4 new optional string fields + maybe an `evidence` block), and the visual shift is what makes the brief's core promise legible.

### 4.2 No member avatar row in CrewStrip → add member data to `CrewStripPayload`

**Option A — Server returns the full member list with one annotated as `is_overloaded`.** Add `crew_members: list<CrewMember{user_id, display_name, is_overloaded, is_viewer}>` to `CrewStripPayload`. Client renders avatars + initials, resolves images via `mediaUrlProvider`, ring-highlights the overloaded one.

**Option B — Client fetches members separately.** Workshop calls `community_service.GetMembers(community_id)` after the workshop feed lands. More round-trips, more complexity, marginal benefit.

**Option C — Render initials only (no avatar lookup).** Faster but visually cheaper.

**Recommendation:** A — single-fetch, server-authoritative ordering, cleanest dispatch. Cap at 6 + "+N more" tile per the JSX.

### 4.3 Atmosphere / evidence text → schema'd on `NudgePayload`

**Option A — Add `atmosphere_line: string` + `evidence: ` (optional message containing `quote` + `attribution`) fields.** Detectors populate them at materialization time.

**Option B — Use `description` for atmosphere only and a new `narrative` field for the headline body.** Re-uses existing field but breaks feed nudges that already use `description`.

**Recommendation:** A.

### 4.4 Lever button → render distinct, full-width

**Option A — Render the lever as a separate full-width button inside the tile (like the JSX).** Tile no longer wraps the whole thing in Tappable; the headline area can be a separate Tappable that pushes detail, and the lever runs the action directly via the dispatcher.

**Option B — Keep the whole-tile Tappable but visually decorate the bottom row to read as a button.** Easier; less faithful.

**Recommendation:** A. The brief's gestalt depends on this.

### 4.5 Circle chip on tiles → render from `community_id`

Add a small chip widget (coloured dot + circle short-name) shown on every tile when `workshopActivePillProvider` is *All*. Reads circle short-name and colour from `selectedCommunityProvider`. Pure client change once `NudgePayload.community_id` is populated end-to-end.

### 4.6 "Together this season" surface

**Option A — New `GetWorkshopSynthesis(circle_ids[]) → list<CircleSynthesis>` RPC** that aggregates done / replaced / together-time per circle for the last month. Reuses queries already shaped for `PortfolioMetricsScreen`. Client renders three serif panels per JSX.

**Option B — Reuse `PortfolioMetricsScreen` data** by surfacing a thin slice through the existing portfolio service. No new RPC.

**Option C — AI-generated narrative.** Once §4.12 lands, this becomes "feed last-30-days circle activity to the AI provider and ask for a 3-panel synthesis." Higher quality, more cost per render.

**Recommendation:** A for v1 (fixed-shape narrative templates filled
from real stats); C as a follow-up once AI generation is plumbed.

### 4.7 "This week need" surface

**Option A — Add a "request" detector to the cascade chain** that fires
when the host has open `Request` rows in their circles with low
claim-rate and approaching deadlines. Surfaces as its own card outside
the main 5-cascade Hero card (per the JSX, it's a separate sub-card
between Crew strip and Hero card).

**Option B — Add a separate `weekly_need: NeedPayload?` field to `GetWorkshopFeedResponse`** populated by a new `server/needs/` shared library that reads `Request` storage.

**Recommendation:** B. Mixing requests into the cascade detector chain
conflates two concept layers (requests are recipient-side, the
cascade is host-side). Keeping it as its own field is cleaner.

### 4.8 All-Circles crew summary

**Option A — Server-side summary.** When `circle_id == AllCirclesScope`,
include a `circles_summary: list<CircleTension{community_id, name, tension_line}>` field in the response, populated by walking each
community's catalyst + cascade signal and emitting a one-line hint.

**Option B — Client-side summary.** Client makes N calls (one per
circle) and synthesizes locally. N+1 round-trips; more drift over
time.

**Recommendation:** A.

### 4.9 Hero card variants #9–#16

Each is essentially "a new detector + new generation logic + sometimes
new data". They split into three subgroups, prioritized:

- **Survey-author lever (#9)** — see §4.10. High value, modest cost.
- **Verdict-after-pulse (#11)** — pure follow-on of ESM aggregation;
  modest cost.
- **Recap-and-thanks (#10)** — needs Story generation, which needs AI
  copy. High value, blocks on §4.12.
- **Decay / lapsed / refresh / bridge / pause (#12–#16)** — need
  per-attendee detection (§4.15) and AI copy (§4.12). Defer.

### 4.10 In-app ESM authoring

**Option A — In-app survey-author lever on Hero card (cascade #9).** When
the host taps "Worth doing again, the smoker?" lever, open a bottom
sheet with template suggestions ("Worth doing again, the {experience}?",
"Best time for round {N+1}?", "Pace feel right lately?"). Host picks
+ edits + sends. Backend RPC (`AuthorESMPrompt`) wraps the runbook's
`BuildPromptForExperience` helper. Recipient set defaults to
`AttendeeUserIDs`. This is exactly what the runbook does today, just
behind an authenticated host RPC instead of a developer's psql.

**Option B — Standalone "+New ESM prompt" button** on the Workshop
screen header. Less context-aware than (A) but reachable without an
active Hero card.

**Option C — Both A and B.** A is the contextual path (most expected),
B is the manual path (catches edge cases).

**Recommendation:** A first, ship B as a follow-up if real hosts want
it. The contextual path is closer to the brief and closer to v2's
automated trigger (which fires on experience completion, exactly
when A's lever is most relevant).

### 4.11 Event creation pre-fill

**Option A — `GenerateWorkshopDraft` RPC.** Server takes a `nudge_id`,
reads the nudge's `context_id`, fetches the prior instance, generates
the draft (title / date / time / recipient list / body) — using
templates today, AI when §4.12 lands — and returns a
`GeneratedExperienceDraft` that the client routes into
`ExperienceCreationModal` via `genExperienceProvider` (already exists).

**Option B — Client-side translator from `NudgePayload` to draft.**
Faster to ship; can't generate a body string of any quality without
AI; doesn't unlock §4.10's "Schedule round 6" flow either.

**Option C — Punt — keep the current `pushToItemScreen` behaviour and
add a "Schedule another" button to the experience detail screen.**
Cheapest; least faithful to the brief.

**Recommendation:** A. Wire the RPC, populate the draft templates with
prior-instance data, and replace the `pushToItemScreen` call in
`nudge_content_view.dart:113-148` with a call into the new draft
flow.

### 4.12 AI copy generation

**Option A — Wire `server/ai/` into the momentum generator.** The
detector returns a `Detection` with the *signal* (experience id,
counts, dates, names); a new `server/momentum/copy_generator.go`
calls `server/ai/` to render the kicker / headline / atmosphere /
lever / draft body / evidence quote tuned to the circle. Stays
behind `ValidateLeverCopy` + `AssertNoBannedWords` guards.

**Option B — Hybrid.** AI for body / atmosphere / draft, templates
for kicker / headline. Cheaper per render, less faithful.

**Option C — Punt.** Static templates indefinitely. The screen never
reads like the brief.

**Recommendation:** A. The plumbing exists already for feed nudges
(see [server/services/feed/nudges.go](../../server/services/feed/nudges.go)) — Workshop nudges should
ride the same path. Latency goes up at generation time but
generation runs server-side off the critical path, and the cache
already holds the result for the dedup window.

### 4.13 Scheduled background generation

**Option A — A new scheduled job at `server/jobs/workshop_generation.go`** (or extend an existing scheduler) that walks each
host's circles every N hours, runs `EnsureQuestHero` /
`EnsureBringBackItems`, and triggers a push notification when a *new*
nudge is materialized.

**Option B — Trigger generation off post-event hooks** (when
`experience.completed_at_unix_sec` is set, generate the next round's
Hero card immediately for the host). Reactive instead of polling.

**Option C — Both.** Reactive for the hot signals (post-event,
new ESM response), polling for the colder signals (calendar gap,
seasonal trigger).

**Recommendation:** C, but ship Option A first. A is independent and
unblocks notifications across all 5 detectors; B can pick off the
hottest signal (post-event ESM) afterwards.

### 4.14 Story / season-summary generation

**Option A — Recap detector that fires on experience completion.** New
detector under `server/momentum/` (or a sibling under
`server/services/stories/`) that, on `experience.completed_at_unix_sec`,
materializes a `Story` with `category = "recap"` plus contributor
metadata. The host sees it as a Hero card (cascade #10 in the JSX)
ready to review and publish.

**Option B — Once-per-season detector** that walks the circle's
active rhythms and emits a `season_summary` Story at equinoxes /
solstices / year-end. Independent of recap — these are different
cadences.

**Recommendation:** A blocks on AI copy (§4.12); ship A and B
together once §4.12 lands. They share the Story infrastructure, the
detector skeleton, and the contributor-extraction logic.

### 4.15 Per-attendee detection model

**Option A — New `server/attendance/` shared library** that maintains
a per-attendee view (last-N attendance trend, lapse-since timestamp,
total appearances, role-tag if known). Detectors read it.

**Option B — Compute on demand** in each detector. Simpler, much more
expensive per call.

**Recommendation:** A. The library is the shape that unlocks
cascades #12 / #13 / #16 plus the Phase-6 follow-on work named in
[docs/workshop.md §Recommended sequencing](../workshop.md#recommended-sequencing).

### 4.16 `context_id` on `StoredNudge`

**Option A — Add `context_id: optional string` field to `StoredNudge` and to `NudgePayload`.** Detector materialization populates from the
detection's anchor (experience id, gear id, user id depending on
slot). Cross-surface dedup keys off `context_id` instead of
`cta_label`. Client tile passes it to the draft generator.

This is mechanically simple and unblocks several of the surfaces
above. No migration risk because the field is additive.

**Recommendation:** A.

### 4.17 Surfacing discipline

**Option A — Server-side day-density classifier.** Given the count
+ priority of fired surfaces, classify as hot / average / quiet and
return a `display_mode` field; client respects it by collapsing the
"this week", "together this season", and Bring-Back surfaces on hot
days.

**Option B — Client-side count threshold.** Simpler.

**Option C — Punt** until enough surfaces exist to need it.

**Recommendation:** C until the recap, verdict, week-need, and
together-this-season surfaces ship. Then revisit with B (the
classifier doesn't need to live server-side).

---

## 5. Resolved Decisions (2026-04-29)

User-supplied direction collapsed every open question. Each is
applied to the phasing in §6.

1. **Naming.** "Quest hero" → **Hero card** throughout. Code
   identifiers also rename in Phase 1: `EnsureQuestHero` →
   `EnsureHeroCard`, `quest_hero` proto field →
   `hero_cards: list<NudgePayload>`,
   `NUDGE_SURFACE_WORKSHOP_QUEST_HERO` →
   `NUDGE_SURFACE_WORKSHOP_HERO_CARD`.

2. **One Hero card per recently completed event** (not the
   single-cascade-winner model). Drift Hero cards (calendar gap
   on long-quiet rhythm, seasonal anniversary, idle gear offer)
   stack below the per-event ones. See §3.1 for the structural
   shift.

3. **In-app ESM authoring: contextual only** (§4.10 Option A).
   No standalone button. The lever lives on the Hero card for a
   recently-completed event with no ESM prompt yet authored.

4. **AI-generated copy: implement now** (§4.12 Option A) — wire
   `server/ai/` into the momentum generator behind the existing
   `ValidateLeverCopy` + `AssertNoBannedWords` guards. Templates
   stay as the fallback when AI generation fails.

5. **Member avatars: initials with progressive enrichment** via
   `mediaUrlProvider`. Same pattern Chat already uses.

6. **Together-this-season metrics: reuse existing impact metrics**
   from `server/services/portfolio/` (counts, replaced-cost, CO₂,
   together-time) and contextualize them in AI-generated sentences
   per circle. No parallel aggregation pipeline.

7. **Lapsed-regular (#13) and bridge-invite (#15) cascades:
   defer.** They wait on the per-attendee detection model
   (§4.15). No stub versions in this plan's phasing.

8. **No push notifications for Workshop nudges.** Scheduled
   generation (§4.13) still ships — it pre-warms the data so a
   host opening the tab sees content immediately — but it does
   not fire notifications. Existing notification surfaces
   (chat, RSVPs, etc.) are unaffected.

9. **Replace `pushToItemScreen` with the draft generator**
   ([nudge_content_view.dart:113-148](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart#L113-L148)).
   Workshop CTAs land in a prefilled creation modal, not the
   read-only detail screen.

10. **Tile look: parchment-style panel** on the Workshop tab
    (kicker / serif headline / atmosphere line / lever button —
    no image background). **Tapping the tile pushes a full-image
    nudge detail screen** from the right
    ([NudgeDetailScreen](../../app/lib/presentation/screens/nudge/nudge_detail_screen.dart)),
    which keeps today's image-as-background look. The Workshop
    becomes a dense information surface; the detail screen
    stays evocative.

11. **Survey templates: hand-authored, not AI.** Four
    conventional templates ("Worth doing again, the {event}?",
    "Best time for round {N+1}?", "Pace feel right lately?",
    interest-pulse). Only the `{event}` / `{N+1}` slots
    interpolate from event metadata.

12. **Crew strip = full community membership.** Not 6 + "+5
    more" as in the JSX. The strip horizontally scrolls when
    the count exceeds the screen width. The host's avatar is
    dark-olive-filled; over-loaded member (Case A/B) gets a
    warm-orange ring.

---

## 6. Recommended Path & Phasing

The work splits naturally into three phases (#13 lapsed-regular and
#15 bridge-invite are deferred per Decision 7 in §5). Each phase is
independently shippable and provides value on its own.

### Phase 1 — Hero-card-per-event + AI copy + tile redesign (≈3 weeks)

**Goal:** every recently completed event renders its own Hero card,
with AI-generated kicker / headline / atmosphere / lever copy in the
parchment-style panel layout.

**In scope:**
- **Rename pass.** "Quest hero" → "Hero card" through code, proto,
  l10n. `EnsureQuestHero` → `EnsureHeroCard`.
  `NUDGE_SURFACE_WORKSHOP_QUEST_HERO` →
  `NUDGE_SURFACE_WORKSHOP_HERO_CARD`. `quest_hero: NudgePayload?` →
  `hero_cards: list<NudgePayload>`.
- **§3.1 — One Hero card per recent event.** Rework
  `EnsureHeroCard` to iterate the host's recently-completed events
  in scope (last 14 days, configurable) and emit one `StoredNudge`
  per event. The detector chain runs *per event* and falls back
  to a recap-shaped "Schedule it again?" card when no slot fires
  so every recent event always produces something. Drift Hero
  cards (calendar gap on long-quiet rhythm, seasonal anniversary,
  idle gear offer) generate via a separate per-circle path and
  dedup against the per-event set by `context_id`.
- **§4.1 — Structured Hero card fields.** Add `kicker_label`,
  `atmosphere_line`, and `evidence{quote, attribution}` (all
  `optional`) to `NudgePayload` + `StoredNudge`. Detector
  materialization populates them.
- **§4.16 — `context_id` on `StoredNudge` + `NudgePayload`** (the
  experience id, gear id, or rhythm id depending on detector).
- **§4.12 — AI copy generation** in `server/momentum/copy_generator.go`.
  AI generates kicker / headline / atmosphere / lever / draft
  message body / evidence quote per detection, gated by the
  existing `ValidateLeverCopy` and `AssertNoBannedWords` guards.
  Static templates remain as the fallback when generation fails.
- **§4.4 — Distinct lever button.** Render the lever as a
  full-width pill at the bottom of the tile. Headline area pushes
  detail on tap; lever fires the action directly.
- **§4.5 — Circle chip on tiles when scope is *All*** — surface
  `NudgePayload.community_id` on the wire and render the chip
  client-side.
- **§3.4 / Decision 10 — Parchment-style tile + image-rich detail.**
  Reshape `WorkshopTile` to the JSX panel layout (no image
  background; serif typography, structured stack). Tap retains
  the existing `NavigationHelpers.pushScreen(NudgeDetailScreen(...))`
  flow — the detail screen still renders the image-as-background
  `NudgeContentView`, so the evocative image-led look survives
  one tap away. The tile MUST use the `Tappable` accessibility
  primitive with an l10n-resolved `semanticsLabel` per
  [docs/client/accessibility.md](../client/accessibility.md);
  raw `GestureDetector` / `InkWell` will fail
  `npm run lint:dart:a11y`.
- **N+1 batching.** `EnsureHeroCard` iterates events × circles —
  load the recent-events-per-circle set in **one** batched query
  (`storage.QueryByFieldIn` or equivalent) per the SQL Efficiency
  rules in [CLAUDE.md](../../CLAUDE.md). The detector chain may
  fire per event but storage reads must batch.
- **Structured logging.** Every new RPC handler and the
  `copy_generator` use `logging.LoggerWithContext(ctx)` with
  standard fields (`operation`, `user_id`, `community_id`,
  `experience_id`, `duration_ms`) per
  [docs/server/observability.md](../server/observability.md).
  AI calls log `external_service` and `provider` / `model`.
  AI errors log at `Warn` (we fall back to templates), generation
  failures that drop a nudge log at `Info` (host-input
  classification), only system failures hit `Error`.
- **Goroutine safety.** Any `go ...` in the AI generation path
  must use `logging.GoSafe(ctx, "name", fn)` per
  [docs/server/architecture.md](../server/architecture.md) — the
  CI gate at `server/cmd/check-goroutines` enforces it.

**Out of scope:** event creation pre-fill, member avatars, ESM
authoring, scheduled generation, recap stories, "this week need",
"together this season".

**Validation:** ViewModel + repository tests for the per-event
Hero card rendering paths (mock `WorkshopServiceClient`, override
providers, dispose container in `tearDown`) per
[docs/client/testing.md](../client/testing.md). Disposal test for
`WorkshopNotifier`. Widget test that `WorkshopTile` renders each
variant and dispatches tap to `NudgeDetailScreen` via
`NavigationHelpers.pushScreen`. Server-side tests that
`EnsureHeroCard` produces N nudges for N recent events and that
the storage round-trip uses ≤ a small fixed number of queries
(use `storage.AssertMaxQueries` per
[CLAUDE.md](../../CLAUDE.md)). Verify the banned-words guard fires
on AI output before persistence.

### Phase 2 — Levers, members, and ESM authoring (≈2.5 weeks)

**Goal:** tapping a lever produces a creation modal with prefilled
content; the Crew strip shows the full community membership; the
host can author an ESM prompt contextually without a runbook.

**In scope:**
- **§4.11 — `GenerateWorkshopDraft` RPC** + client routing into
  `ExperienceCreationModal` with prefilled title / time / recipient
  list / body. Replace the `pushToItemScreen` call in
  [nudge_content_view.dart:113-148](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart#L113-L148)
  per Decision 9. Body copy comes from Phase 1's AI generator;
  recipient list defaults to the prior instance's `RSVP.attended ==
  YES` set.
- **§4.2 / Decision 12 — Crew strip = full community membership.**
  Server returns `crew_members: list<CrewMember{user_id,
  display_name, avatar_media_id?, is_overloaded, is_viewer}>` with
  every community member. Cap server-side at **a documented
  max** (suggest 200) with a `truncated` flag and a separate
  `total_member_count`; communities exceeding the cap render the
  first N + "+M more" chip per
  [CLAUDE.md](../../CLAUDE.md) SQL Efficiency rules ("Always
  LIMIT unbounded queries"). Single batched member fetch — no
  N+1 over members. Client renders horizontally-scrolling
  initials avatars with progressive `mediaUrlProvider` enrichment
  (Decision 5). Host avatar is dark-olive-filled; over-loaded
  member gets a warm-orange ring on Case A/B. Each avatar uses
  `Tappable` with an l10n-resolved `semanticsLabel` per
  [docs/client/accessibility.md](../client/accessibility.md).
- **§4.10 / Decision 3 — Contextual ESM authoring lever.** When a
  recently-completed event has no ESM prompt yet, the Hero card's
  lever variant is *"Worth doing again, the {event}?"*. Tapping
  opens a bottom sheet via `showAccessibleModal` (NOT raw
  `showModalBottomSheet` — caught by `npm run lint:dart:a11y`)
  with **four hand-authored templates** (Decision 11). Templates
  live as l10n keys in **both** [app_en.arb](../../app/lib/l10n/app_en.arb) and
  [app_es.arb](../../app/lib/l10n/app_es.arb)
  before `npm run generate` per
  [docs/client/i18n.md](../client/i18n.md):
  - `esmAuthorTplWorthDoingAgain` → "Worth doing again, the
    {event}?" → options: *Yes, every week / Yes, less often /
    Take a break*.
  - `esmAuthorTplBestTime` → "Best time for round {N+1}?" →
    options pulled from the event's prior time slots.
  - `esmAuthorTplPace` → "Pace feel right lately?" → options:
    *Loving the rhythm / About right / Could slow down*.
  - `esmAuthorTplSeasonInterest` → "Interest in {event} this
    season?" → options: *Yes, I'm in / Maybe — depends on day /
    Not this season*.

  The view-model exposes a `EsmAuthorTemplate` enum; the widget
  resolves via `context.l10n.*` per the i18n boundary in
  [docs/client/i18n.md](../client/i18n.md). The host picks,
  edits, and sends. Backend RPC `AuthorESMPrompt` wraps the
  existing `esm.BuildPromptForExperience` runbook helper.
  Recipient set defaults to `esm.AttendeeUserIDs`. Handler logs
  via `logging.LoggerWithContext(ctx)` with standard fields
  (`operation`, `user_id`, `experience_id`, `recipient_count`).
- **§4.7 — "This week need" surface.** New `server/needs/`
  **shared library** (Pattern 1 in
  [docs/server/architecture.md](../server/architecture.md), not a
  new service) reads open `Request` rows for the host's circles
  and picks the most under-claimed (1+ asks, 0 claims,
  time-pressure ≤ 7 days). Library has its own `doc.go` and
  `README.md` per [CLAUDE.md](../../CLAUDE.md). The Workshop
  service consumes the library directly — no service-to-service
  dependency. Surfaces as a `weekly_need: NeedPayload?` field on
  `GetWorkshopFeedResponse`.
- **§4.8 — All-Circles crew summary.** When scope is *All*,
  return `circles_summary: list<CircleTension{community_id, name,
  tension_line}>`. Tension line is AI-generated per circle from
  the same signal that drives the per-circle Hero cards.
- **Cache invalidation.** `WorkshopRepository` writes for
  `AuthorESMPrompt` and the resulting `GenerateWorkshopDraft`
  trigger a `workshopFeedInvalidationProvider` counter that
  `WorkshopNotifier` watches in `build()` per the
  notification-provider pattern (Patterns 6 & 7) in
  [docs/client/caching.md](../client/caching.md). **Do not
  invalidate by namespace pattern** like `'workshop:*'` —
  there's no `feed:` glob pattern in the existing setup, and the
  rule against namespace wildcards is explicit.
- **Repository constructor pattern.** Any new repository (e.g.,
  whatever holds the ESM-author templates client-side, or a
  `WorkshopDraftRepository` if `WorkshopRepository` would push
  past 1000 lines) takes `CacheManager + <ServiceClient>` in the
  constructor and creates `CacheService(cacheManager, '<ns>')`
  internally per [docs/client/caching.md](../client/caching.md).
- **Async safety.** Any non-`build()` notifier method that
  `await`s and then writes to `state` MUST guard with
  `if (!ref.mounted) return;` per the Async Safety section in
  [docs/client/architecture.md](../client/architecture.md).

**Out of scope:** scheduled generation, recap / season-summary
stories, per-attendee detection.

**Validation:** ViewModel tests over `WorkshopNotifier` and any new
draft / ESM-author notifiers (ProviderContainer + Mockito mocks of
the service clients, dispose container in `tearDown`); disposal
tests for every new `AsyncNotifier`; widget test that the ESM
authoring sheet is launched via `showAccessibleModal` and renders
the four template options. End-to-end smoke where a host taps
"Schedule round 6", lands in the prefilled creation modal, edits,
publishes — plus a parallel flow where they tap "Worth doing
again?", pick a template, send, and see the resulting ESM card in
another account's feed. Manual check that the Crew strip shows
every community member up to the cap and scrolls horizontally.

### Phase 3 — Together this season + scheduled generation + recap stories (≈3 weeks)

**Goal:** the screen has a calm aggregate floor surface, populates
itself before the host opens it, and offers a recap-and-thanks
Story for newly-completed events.

**In scope:**
- **§4.6 / Decision 6 — "Together this season"**. New
  `GetWorkshopSynthesis(circle_ids[])` RPC. **Service Independence
  rule** ([docs/server/architecture.md](../server/architecture.md))
  forbids the workshop service injecting the portfolio service —
  the existing portfolio aggregation logic must therefore live in
  (or be factored into) a shared library at `server/impact/`
  (Pattern 1). The portfolio service and the workshop service
  both consume that library; neither depends on the other. If
  the aggregation logic is currently inside
  `server/services/portfolio/` only, Phase 3 includes the refactor
  to extract it. The workshop's RPC handler combines the impact
  library output with AI-generated contextualizing sentences
  (three serif panels per the JSX layout). The shared library
  has its own `doc.go` + `README.md`.
- **§4.13 / Decision 8 — Scheduled `workshop_generation` job.**
  Walks active circles, runs `EnsureHeroCard` and
  `EnsureBringBackItems`, materializes nudges in the background.
  **No push notifications fire** — pre-warming the data is the
  whole point. Hosts opening the tab see content immediately
  instead of paying generation latency. Implementation rules:
  - Lives in `server/jobs/workshop_generation.go` (or extends
    an existing scheduler) and does NOT import the workshop
    service — it calls the same shared libraries
    (`server/momentum/`, `server/catalyst/`, `server/esm/`)
    the workshop service calls. Service Independence holds: the
    job is a separate consumer.
  - Every spawned goroutine wraps in
    `logging.GoSafe(ctx, "name", fn)` per
    [docs/server/architecture.md](../server/architecture.md).
    The CI gate at `server/cmd/check-goroutines` rejects bare
    `go ...` without recover-defers.
  - Logging uses `logging.LoggerWithContext(ctx)` with
    `operation = "workshop_generation"` plus per-circle fields
    (`community_id`, `nudges_generated`, `duration_ms`). Use
    `Info` for normal completion, `Warn` for per-circle
    failures that don't block the job, `Error` only for the job
    crashing.
  - The job MUST NOT enqueue notifications — Decision 8.
- **§4.14 — Recap-and-thanks Story.** New detector under
  `server/momentum/` (NOT a new service) that, on
  `experience.completed_at_unix_sec` being set, emits a `Story`
  with category `recap`, contributor list extracted from the
  event's RSVPs, and an AI-generated body. The host's Hero card
  for that event becomes the *"Last Saturday · ready to publish
  — Round 5 of the smoker — what a night."* recap variant
  (cascade #10), routing to the existing `StoryDetailScreen` for
  review-and-publish. Story persistence goes through the existing
  story storage layer (shared library, not the story service)
  per Service Independence.
- **§4.9 cascade #11 — Verdict-after-pulse.** Pure follow-on of
  ESM aggregation: when an ESM prompt's response window closes
  with ≥60% repeat signal, the host's Hero card for that event
  shifts from "Worth doing again?" to *"Pulse came back · 8 of
  11 said yes · Propose weekly through July?"*. Same nudge
  pipeline; new headline / atmosphere / lever variant. The
  detector consumes `server/esm/` directly per existing pattern.

**Out of scope:** decay (#12), lapsed regular (#13), format
refresh (#14), bridge invite (#15), pause gracefully (#16) —
deferred per Decision 7.

**Validation:** A/B the AI synthesis against the JSX mock copy
with internal hosts; verify the scheduled job runs without
notifications firing; manual smoke that completing an experience
generates a recap Story within the next scheduled-job tick.

### Deferred (no phase, no schedule)

- §4.15 — `server/attendance/` per-attendee detection library.
- Cascade #12 (early decay), #13 (lapsed regular), #14 (format
  refresh), #15 (bridge invite), #16 (pause gracefully).
- Once-per-season `season_summary` Story generator.
- v3 multi-question / cross-event ESM prompts.
- AI generation of survey-question templates (Decision 11 keeps
  templates hand-authored).

### Sequencing constraints

- Phase 1 has no hard prerequisites. The rename pass + the
  per-event Hero card rework + AI copy land together; AI is the
  natural fit because the per-event model needs richer copy than
  templates can produce ("schedule round N+1 of the {event} on
  {weekday} at {time}, with {top contributors}" is hard to
  template generically).
- Phase 2 depends on Phase 1's `context_id` field and the
  per-event Hero card model.
- Phase 3 depends on Phase 2's `GenerateWorkshopDraft`
  infrastructure (the recap-publish flow reuses it) and on
  Phase 1's AI plumbing.

---

## 7. Risks & Tradeoffs

- **Phase 1's proto changes ripple to feed nudges.** Adding
  `kicker_label`, `atmosphere_line`, `evidence`, `context_id` to
  `NudgePayload` means feed nudges either populate them too or
  ignore them. They should be optional everywhere — feed nudges
  that ignore them stay backward-compatible. Per
  [CLAUDE.md](../../CLAUDE.md) proto field conventions, all four
  use the `optional` keyword from the start.

- **Phase 1's per-event Hero card model multiplies nudge volume.**
  A host with 3 active rhythms could now see 3 Hero cards per
  Workshop open, plus drift cards. Surfacing discipline (cap
  per-event cards to ~5 most recent, collapse the rest behind
  a "show more" affordance) is a Phase 1 task — without it, a
  busy host's screen becomes a wall of cards.

- **Phase 1's `quest_hero` → `hero_cards: list<>` proto change is
  not wire-compatible.** Any client running pre-Phase-1 expects
  a singular field; the new server returns a list. Plan for a
  short window where the server populates *both* `quest_hero`
  (deprecated, first list element) and `hero_cards` (new), so
  old clients keep working until the app store rollout settles.
  Drop `quest_hero` two releases later.

- **Phase 1's CTA replacement is the biggest behaviour change.**
  Today's `pushToItemScreen` dispatches into a read-only detail
  screen; the new flow lands in a prefilled creation modal. Any
  dev who's testing Workshop CTAs today gets a different result.
  Mitigated by: shipping behind a feature flag for one release
  cycle.

- **AI copy in Phase 1 will go off-rails sometimes.** The
  `ValidateLeverCopy` and `AssertNoBannedWords` guards catch the
  worst offences; everything else needs human review at first.
  Plan for a "don't surface this nudge" admin path that lets us
  drop a bad nudge from a host's feed; templates remain as the
  fallback when AI generation fails or produces guard-violating
  output.

- **AI generation cost grows with the per-event model.** Each
  recently-completed event triggers one AI call per host
  (cached for the dedup window). For a host with 3 rhythms × 4
  recent events × N hosts, the cost is real. Monitor and add
  per-circle batching if it bites.

- **Phase 3's scheduled job pre-warms data without notifying.**
  Decision 8 explicitly rules out push notifications for Workshop
  nudges, so the job is silent — its only job is to make the
  Workshop tab feel instant when opened. If a future product
  decision wants notifications, that's a separate change behind a
  per-user opt-in.

- **The "together-this-season" surface overlaps with
  PortfolioMetricsScreen.** Two narrative surfaces with the same
  data cause drift. Pick one as canonical and link from the other.
  Recommendation: PortfolioMetricsScreen stays the dashboard view
  (numbers, charts, history), Workshop's surface stays the
  monthly serif synthesis (3 short panels, no charts) — the brief
  asks for two different shapes of the same data, on purpose.

- **Crew strip payload size.** Decision 12 says "every member of the
  community" — for a 200+ member community that's a fat payload.
  Phase 2 adds a server-side cap (suggest 200) with `truncated` +
  `total_member_count` metadata so the client can render a "+M
  more" affordance. Avoids the SQL Efficiency rule against
  unbounded queries (per [CLAUDE.md](../../CLAUDE.md)).

---

## 8. What This Plan Does Not Cover

- v3 multi-question / multi-event ESM prompts.
- A standalone admin / operator tool for ESM authoring (the runbook
  covers operator authoring; in-app contextual authoring is in
  Phase 2 per Decision 3; a developer admin panel is out of scope).
- The full Phase-3 all-circles aggregation from the brief
  (cross-circle load balancing, cross-circle bridge candidates from
  many circles). Bridge detection itself is deferred per Decision 7.
- Push notifications for Workshop nudges (Decision 8 — scheduled
  generation pre-warms data without notifying).
- Voice-first editing of drafts (the brief's "speak to edit" affordance
  was deferred in #1579's planning thread).
- Haptics on lever taps (also deferred).
- Analytics / metrics / dashboards for the new surfaces. Plan
  separately once Phase 1 ships and there's behaviour to measure.

---

## 9. Next Action

All open questions resolved (§5). Next step is to file three
GitHub issues — one per phase — with the in-scope items as sub-bullets
and the proto/RPC changes flagged as the sequencing-critical pieces:

- **Phase 1 issue:** rename pass + per-event Hero card model + AI
  copy + structured tile fields + parchment-style tile.
- **Phase 2 issue:** `GenerateWorkshopDraft` + Crew strip full
  membership + contextual ESM authoring + "this week need" +
  All-Circles summary.
- **Phase 3 issue:** "Together this season" + scheduled generation
  (no notifications) + recap-and-thanks Story + verdict-after-pulse
  cascade.

---

## Audit Notes (2026-04-29)

The plan was reviewed against
[docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/testing.md](../client/testing.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md). Changes
made in place — none of the audit notes alter the plan's structure,
only its compliance:

1. **Service Independence enforcement.** Phase 3's
   `GetWorkshopSynthesis` originally said it "reuses
   `server/services/portfolio/`" — that would inject one service
   into another, violating the rule in
   [docs/server/architecture.md](../server/architecture.md). Phase
   3 now requires extracting the impact aggregation into a
   `server/impact/` shared library (Pattern 1 — Standalone
   Libraries) which both the portfolio service and the workshop
   service consume independently. Phase 3 also clarifies that the
   recap-Story detector and the verdict-after-pulse detector
   consume `server/momentum/`, `server/esm/`, and the story
   storage layer directly — no service-to-service injection.

2. **Scheduled job placement.** Phase 3's `workshop_generation`
   job now explicitly lives in `server/jobs/`, calls the same
   shared libraries the workshop service calls (not the service
   itself), and wraps every spawned goroutine in
   `logging.GoSafe(ctx, ..., fn)` per the goroutine-safety rule
   in [docs/server/architecture.md](../server/architecture.md).
   The CI gate at `server/cmd/check-goroutines` rejects bare
   `go ...` without recover-defers.

3. **Structured logging baked into Phase 1 and 3.** Each new RPC
   handler, the `copy_generator`, and the scheduled job use
   `logging.LoggerWithContext(ctx)` with the standard fields from
   [docs/server/observability.md](../server/observability.md) —
   `operation`, `user_id`, `community_id`, `experience_id`,
   `duration_ms`, plus `external_service` / `provider` / `model`
   on AI calls. AI fallback to templates logs at `Warn`; only
   system failures hit `Error` (per the level-discipline rule
   that user-input failures are `Info`/`Warn`, not `Error`).

4. **SQL Efficiency / N+1 risk.** Phase 1's per-event Hero card
   rework iterates events × circles. Plan now requires batched
   storage reads (`storage.QueryByFieldIn` or equivalent) and
   adds `storage.AssertMaxQueries` to the validation step per the
   SQL Efficiency rules in [CLAUDE.md](../../CLAUDE.md). This
   risk is also called out in §7.

5. **Crew strip payload bound.** Decision 12 said "every member of
   the community" — unbounded in payload size. Phase 2 now caps
   server-side (suggest 200) with `truncated` + `total_member_count`
   metadata, plus a `+M more` chip on the client. Aligns with the
   "Always LIMIT unbounded queries" rule in
   [CLAUDE.md](../../CLAUDE.md).

6. **Accessibility primitives required, not optional.** Phase 1's
   tile redesign and Phase 2's avatars + bottom sheet now
   explicitly call for `Tappable` (with l10n-resolved
   `semanticsLabel`) and `showAccessibleModal` per
   [docs/client/accessibility.md](../client/accessibility.md). The
   lint rules (`require_l10n_for_semantic_labels`,
   `prefer_accessible_modal`, `avoid_raw_gesture_detector`) at
   `npm run lint:dart:a11y` will fail the build otherwise. The
   a11y allowlist at `scripts/a11y_allowlist.txt` is empty —
   ratchet-enforced.

7. **i18n boundary explicit for Phase 2 templates.** The four ESM
   author templates have named l10n keys
   (`esmAuthorTpl{Worth,BestTime,Pace,SeasonInterest}`) and must
   land in **both** `app_en.arb` and `app_es.arb` before
   `npm run generate` per [docs/client/i18n.md](../client/i18n.md).
   The view-model exposes a typed `EsmAuthorTemplate` enum; the
   widget resolves text via `context.l10n.*` in `build()`. Plan
   was implicit on this; now explicit — `npm run lint:dart:i18n`
   would otherwise reject hardcoded strings in the bottom sheet.

8. **Cache invalidation pattern correct.** Phase 2 explicitly
   uses a `workshopFeedInvalidationProvider` notification-counter
   that `WorkshopNotifier` watches in `build()`, mirroring the
   `searchCacheInvalidationProvider` / `dailyCacheInvalidationProvider`
   pattern in [docs/client/caching.md](../client/caching.md)
   (Patterns 6 & 7). **No namespace-pattern wildcards** like
   `'workshop:*'` — the existing pipeline doesn't support glob
   invalidation.

9. **Repository constructor pattern reinforced.** Phase 2 reminds
   that any new repository takes `CacheManager + <ServiceClient>`
   in the constructor and creates `CacheService(cacheManager,
   '<ns>')` internally per
   [docs/client/caching.md](../client/caching.md). No deeper
   chains — repository depth ≤ 1.

10. **AsyncNotifier safety + disposal tests.** Phases 1 and 2
    require `if (!ref.mounted) return;` after every async gap
    in non-`build()` notifier methods, plus disposal tests for
    every new `AsyncNotifier`, per
    [docs/client/architecture.md](../client/architecture.md) and
    [docs/client/testing.md](../client/testing.md). Plan named
    these implicitly; now explicit.

11. **Validation language reframed away from "golden tests".**
    The original Phase 1 validation said "golden tests for each
    of the 5 cascade variants." Per
    [docs/client/testing.md](../client/testing.md), golden tests
    are listed as a future improvement, not the current testing
    layer. Validation now leads with ViewModel + repository tests
    (ProviderContainer + Mockito mocks), disposal tests, and
    widget tests for tap-dispatch — the patterns the test doc
    currently endorses. Server-side adds `AssertMaxQueries`.

12. **Per-package README + doc.go for new directories.** Phase
    2's new `server/needs/` library and Phase 3's new
    `server/impact/` library each get a `doc.go` and `README.md`
    per [CLAUDE.md](../../CLAUDE.md)'s rule that every multi-file
    directory under `server/` has a README covering purpose, key
    files/types, and when to add code here vs. an adjacent
    directory.

No structural changes to the §1–§9 plan layout. The fixes are all
in §6 (Phase task lists), §7 (Risks added for Crew strip payload
size and reinforced for N+1), and this Audit Notes section.
