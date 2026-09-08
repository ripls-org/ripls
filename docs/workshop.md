---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Workshop tab — the host's proactive surface (community carousel, identity block, known-for chips, impact rows, available-now rail, action postcards via the shared LeanCards widget) driven by the momentum/needs/catalyst detector chain that emits StoredNudge rows; postcards launch GenerateWorkshopDraft.
  globs: [server/services/workshop/**, server/needs/**, app/lib/presentation/screens/workshop/**, app/lib/presentation/widgets/workshop/**]
  triggers: [workshop, nudge, postcard, momentum, catalyst, detector, host-surface, repeat-signal]
  lens: [domain, client, server]
  domain: workshop
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# Workshop Tab

> **Retired as a tab (#2568).** The Workshop **bottom-nav tab is replaced by
> the Directory** (address book) surface — see
> [docs/issues/2568-directory-and-profiles.md](issues/2568-directory-and-profiles.md).
> The cutover is gated by `directoryEnabledProvider`
> ([app/lib/core/config/feature_flags.dart](../app/lib/core/config/feature_flags.dart)),
> now defaulted **on**: `HomeScreen` renders `DirectoryScreen` in the former
> Workshop slot and the nav shows the Directory item. **The momentum/needs/
> catalyst/ESM engine described below is retained** — it keeps writing
> `StoredNudge` rows that surface in the **Feed** ([nudges.md](nudges.md)) and
> recap-story ESM. The community calendar's open-day-suggestion surface moved
> out of the Workshop morph into a standalone
> [`SharedCalendarScreen`](../app/lib/presentation/screens/communities/shared_calendar_screen.dart)
> reachable from the community and person profile's **"Plans"** action (it reuses
> the same shared `Calendar*` widgets + `workshopCommunityCalendarProvider`). The
> Workshop *client screen/widgets* (`screens/workshop/**`, `widgets/workshop/**`)
> remain in the tree for now — several pieces are still reused (the calendar
> provider/widgets, metric detail screens, the name-this-group switcher). The
> switcher is no longer the default community-promotion entry point — the
> Directory's own nameless-group "Name" chip is (see
> [workflows/community_promotion.md](workflows/community_promotion.md)) — but the
> switcher's flow still exists and still works when `directoryEnabledProvider`
> is off. Their physical removal/relocation is a tracked residual cleanup under
> #2568. The sections below describe the (retained) server engine and the
> now-removed tab UI for historical reference.

> **v4.1 community-overview redesign (#2447).** The Workshop tab's *rendering
> layer* was rebuilt to adopt the content-view visual language — a full-bleed
> community photo, a dark scrim, and glass-morphic cards floating over it. The
> **server payload, detector chain, and data model below are unchanged**; only
> the client presentation changed. Where the sections below describe the old
> light-surface widgets (carousel, identity block, `ImpactRows`, `LeanCards`,
> `AvailableNowRail`), see **[Client rendering](#client-rendering)** for the
> current widgets. New on-photo section widgets live under
> `app/lib/presentation/widgets/workshop/` and share one palette/glass-card
> primitive ([`workshop_overview_theme.dart`](../app/lib/presentation/widgets/workshop/workshop_overview_theme.dart)).
> The tab is a vertical `PageView` — one full-bleed community page per circle,
> swipe up/down to switch — and the deep dives (roster, metric detail screens,
> conversation, library) **morph-open** as overlays hosted over the same
> community photo. The library calendar now ships the full month-grid ×
> photo-backdrop visual (matching the inbox calendar, #2514); the library map
> remains a deferred follow-up, shipping as a functional item-list destination
> rather than the full pin-map visual.

## Overview

The Workshop tab is the host's proactive surface — where the system suggests
what to schedule, revive, or hand off without the host having to think
about it. It replaces the legacy *Impact* tab as the 5th destination in the
bottom navigation; the portfolio metrics screen still exists and is
reachable from the profile/settings drawer ("Your impact"), but the tab
itself is now this momentum-spotting surface.

**The Workshop body is a single scrolling `ListView` of stacked
sections, top to bottom:**

1. A **community carousel** (`WorkshopCommunityCarousel`) — a scope
   selector that pins one circle or "Everything". It scrolls with the
   rest of the content; only the top and bottom navs stay pinned.
2. An **identity block** (`WorkshopCommunityIdentity`) — the pinned
   community's avatar + serif display name, with a muted subtitle that
   underlines a "Mike, Diego, Sarah + 8 others" member rollup and
   appends the brief's "Since August 2024" anchor. Tapping the
   underlined member span opens the roster sheet.
3. A **known-for chip row** (`KnownForChips`) — the `specialties`
   capability tags. Tapping a chip opens the known-for category detail
   screen for that tag.
4. An **impact row triplet** (`ImpactRows`, the shared profile widget) —
   hours together, saved dollars, and CO₂ avoided. Each row is its own
   tap target that opens the matching detail screen (time / money /
   CO₂). There is **no** acts detail screen.
5. An **available-now rail** (`AvailableNowRail`) — the brief's
   `available_now_items` as borrowable/open entity tiles.
6. An **action-postcard list** (`LeanCards`, the shared profile widget)
   under a serif header derived from the first row's `kicker`. Each card
   is a photo thumbnail with a serif action-shaped headline and an
   italic atmosphere line. The first three cards are visible by default;
   a "Show N more" link inside `LeanCards` expands the rest.

**No AI writes any of this copy.** Two separate removals got here. First the
"daily brief" paragraph and its inline tappable levers went — `BriefProvider`,
`prompts_workshop_brief.go`, and the `GenerateWorkshopBrief` provider methods
are gone. Then #2936 removed the per-card AI pass: `GenerateHeroCardCopy`, the
`HeroCardCopyProvider` seam, and `momentum.CopyGenerator`'s `applyAI`. The home
payload is structured numerals plus structured postcards, and every string is
host-voiced templated text from the detector that emitted it.

The AI pass was already nearly vestigial when it was removed: the scheduled
pre-warm job has always persisted template-only copy, so the two paths could
disagree about the same signal, and the prompt was fed the template copy as
`DETECTOR DEFAULTS` and asked to improve it.

The cascade detector chain (`server/services/workshop/momentum/`, `server/needs/`,
`server/services/workshop/catalyst/`, `server/impact_metrics/`) is unchanged — its
`StoredNudge` rows feed the postcard list directly.

## What's on the screen

```
┌──────────────────────────────────────┐
│ [invite]   Workshop   [avatar]       │  ← FloatingHeader (pinned)
├──────────────────────────────────────┤
│ ( Everything )( Smoke Crew )( Trail )│  ← WorkshopCommunityCarousel
│ ──────────────────────────────────── │     (scrolls with the body)
│ ⬤  Smoke Crew                        │  ← WorkshopCommunityIdentity
│    Mike, Diego, Sarah +8 · Since Aug │     serif name, underlined
│                                      │     member rollup → roster
│ (Power Tools lender)(Camping lender) │  ← KnownForChips (specialties)
│                                      │
│   742 hrs  │  $5,759  │  898 kg      │  ← ImpactRows (3 tappable rows)
│   together │  saved   │  CO₂ avoided │     → time / money / CO₂ detail
│                                      │
│ Available now                        │  ← AvailableNowRail
│ ┌────┐┌────┐┌────┐                  │     (available_now_items tiles)
│                                      │
│ Three ways to keep it going          │  ← LeanCards header (from kicker)
│ ┌─────────┬──────────────────────┐  │  ← LeanCards (cta_rows)
│ │  photo  │ Schedule Sunday      │  │     thumbnail + serif headline
│ │         │ smoker again         │  │     + italic atmosphere line
│ │         │ Alice and Bob want   │  │
│ │         │ another              │  │
│ └─────────┴──────────────────────┘  │
│ ┌─────────┬──────────────────────┐  │
│ │  photo  │ Bring back Sunday    │  │
│ │         │ brunch               │  │
│ │         │ Ran 6 times, then    │  │
│ │         │ quiet                │  │
│ └─────────┴──────────────────────┘  │
│           Show 4 more                │  ← _ShowMoreLink (inside LeanCards
│                                      │     when cta_rows > 3)
└──────────────────────────────────────┘
```

Tapping a postcard always runs the same flow regardless of the
underlying detector slot:

1. The client calls
   [`GenerateWorkshopDraft(experience_id)`](#generateworkshopdraft) on
   the server.
2. The server loads the prior-instance `Experience` referenced by the
   postcard's `context_id` and returns name, description, time,
   location, media ids, and the prior RSVP-attended user-ids. The
   suggested time is the prior instance's **weekday and wall-clock hour
   at the next such slot in the future**, stepped in that event's own
   timezone so the hour survives a DST boundary (`nextWeeklySlot`,
   [draft_generation.go](../server/services/workshop/draft_generation.go)).
   A TBD or unset prior time yields no suggestion and the host picks one.
3. The client seeds [`genExperienceProvider`](../app/lib/presentation/viewmodels/gen_experience_view_model.dart)
   with the draft (after a `reset()` so prior tap state doesn't leak)
   and opens [`ExperiencePreviewModal`](../app/lib/presentation/screens/experience/experience_preview_modal.dart).
4. The host edits and publishes from the preview.

**The draft outlived the tab.** Steps 1–4 are now `openRepeatDraft`
([blank_create_dispatcher.dart](../app/lib/presentation/screens/create/blank_create_dispatcher.dart)),
shared between the nudge CTAs and the **wrapped-up event's "Schedule the next
one" chip** at the foot of its Who's-in card
([experience_read_shell.dart](../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart)).
The chip is the route a default install actually reaches: with this tab off
the nav, workshop-surface nudges have no reader, so before it the detector
chain below wrote `schedule_repeat` rows nobody could act on.

The action-vocabulary switch (`schedule_repeat` / `revive_experience` /
`post_in_chat` …) that the daily brief used has been
collapsed: every postcard goes through the draft → preview path. The
`cta_action` string each detector still emits internally is no longer sent
raw on the wire — `briefToPayload` splits it server-side into `item`
(entity-referencing rows, carrying `item.context_id`) or `action_name`
(non-entity verbs) before it reaches `BriefCTARow` (see *BriefPayload
shape* below).

Tapping a metric figure opens its corresponding detail screen —
[`WorkshopTimeDetailScreen`](../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart),
[`WorkshopMoneyDetailScreen`](../app/lib/presentation/screens/workshop/workshop_money_detail_screen.dart),
[`WorkshopCo2DetailScreen`](../app/lib/presentation/screens/workshop/workshop_co2_detail_screen.dart),
or
[`WorkshopProblemsSolvedDetailScreen`](../app/lib/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart).
The **"problems handled"** figure is an **X of Y** fraction: handled
(completed loans + fulfilled requests + claimed needs, the brief's
`problems_solved_count`) over potential (all non-cancelled loans / requests /
posted needs, `problems_potential_count`) — **not** the broader `acts_count`.
Its deep-dive reads `GetCommunityProblemsSolvedDetail` (see
[impact_metrics.md](impact_metrics.md)). Each detail screen morph-opens as an
overlay (over the community photo) seeded only with the active `communityIds`
(the page-scoped set from `workshopEnabledCommunityIdsProvider`); the screen
re-fetches its own per-community breakdown from the impact repository rather
than carrying a brief total.

## Home payload assembly

Implemented at [server/services/workshop/brief.go](../server/services/workshop/brief.go).
The handler is a pure templated assembler — no AI call on the request
path. The legacy `BriefProvider` seam, `prompts_workshop_brief.go`, and
the per-provider `GenerateWorkshopBrief` methods have been deleted; the
`Brief` Go type now carries only `CTARows`. Hero / ticker numerals
flow straight from `impact_metrics.Calculator`.

### Pipeline

```
GetWorkshopBrief(circle_ids[])
    ↓
auth.RequireAuth(ctx)
    ↓
auth.FilterActiveMemberCommunities  // drop soft-deleted / non-member
    ↓
Kick off background materialization (logging.GoSafe, detached ctx):
    For each active community in scope:
      EnsureHeroCard(user, community)
      EnsureBringBackItems(user, community, "")
    (idempotent — skips when an active card already exists; runs the
     detector chain per detection, so it's kept off the request path)
    ↓
loadBriefNudges(user, communityIDs)        // batched per-community read
    ↓
BriefRanker.Rank(nudges)                    // dedupe + interest-then-recency sort
    ↓
resolveContextThumbnails(ranked)            // first-media id per row
    ↓
collectSeasonTotals(communityIDs)           // acts / cost / time / CO₂
    ↓
countUniquePeople(communityIDs)             // distinct members (#1898)
    ↓
buildTemplatedBrief(signal, ranked)         // *momentum.Brief — BriefCTARow[]
    ↓
earliestCommunityCreatedAt(communityIDs)    // hero "Since X" anchor
    ↓
loadSuppressedKeysForScope(...) + known_for.Derive(...)   // specialties chips
    ↓
available_now.Gather(communityIDs)          // available_now_items rail
    ↓
briefToPayload(brief, thumbs, signal,       // wire shape
               sinceUnixSec, specialties, availableNow)
    ↓
return BriefPayload                          // identity + impact rows +
                                             //  chips + rail + cta_rows
```

The handler always returns a `BriefPayload` (even when no nudges have
been materialized yet) so the home renders the hero + ticker on first
open. The background goroutine repopulates nudges after a wipe; the
client can pull-to-refresh ([`WorkshopNotifier.refresh`](../app/lib/presentation/viewmodels/workshop_view_model.dart))
to pick them up once they land.

### BriefPayload shape

The home is a structured payload, not free-form prose:

```
BriefPayload {
  // Stat hero
  since_unix_sec:      int64    // earliest community.created_at across scope
  hours_together:      int32    // big serif numeral
  // Ticker
  acts_count:          int32    // total community acts (broader than
                                //  events: items shared, help req/off,
                                //  events created, RSVPs, pitch-ins,
                                //  completed loans/giveaways). Carried on
                                //  the wire; the home no longer renders an
                                //  acts cell (no acts detail screen)
  events_count:        int32    // legacy field — kept for old-server
                                //  fallback before acts_count rolled out
  problems_solved_count:    int32  // "problems handled" numerator (X):
                                   //  completed loans + fulfilled requests
                                   //  + claimed needs. Drives the overview's
                                   //  "X of Y problems" figure + its deep dive
  problems_potential_count: int32  // "problems handled" denominator (Y): all
                                   //  non-cancelled loans/requests + posted
                                   //  needs. Always >= problems_solved_count
  replaced_cost_usd:   int32    // ImpactRows "saved" row
  co2_avoided_pounds:  int32    // wire field name; client converts to
                                //  kilograms for display
  unique_people_count: int32    // distinct members across the scope
                                //  (#1898). Surfaced via the identity-block
                                //  member rollup (not a ticker cell); the
                                //  rollup's underlined span opens the roster
  // Identity decoration
  specialties:         list<string>  // known-for capability tags rendered
                                     //  as chips (KnownForChips)
  available_now_items: list<Item>    // currently-borrowable/open entities
                                     //  rendered in the AvailableNowRail
  // Postcards
  cta_rows:            list<BriefCTARow>

  // DEPRECATED — retained for wire compatibility, no longer populated:
  kicker_label, segments, closing_line, metrics_link_label
}

BriefCTARow {
  kicker:              string   // optional uppercase kicker / circle-chip
                                //  label
  headline:            string   // action-shaped ("Schedule X again",
                                //  "Bring back X", "Confirm next X")
  atmosphere_line:     string   // signal / reason
                                //  ("Alice and Bob want another",
                                //   "Ran 6 times, then quiet",
                                //   "Just wrapped Saturday")
  is_primary:          bool     // first row in the ranked list
                                //  (legacy primary-pill flag; the
                                //   client renders all rows the same
                                //   now that the lever button row was
                                //   removed)
  community_id:        string   // for the per-row circle chip
  item:                Item     // the entity this row's action references
                                //  (gear / experience / request), when the
                                //  action targets an entity — carries its
                                //  own context_id / title / subtitle /
                                //  media_id. Populated by briefToPayload
                                //  from itemkind.FromCTAActionString.
                                //  Absent for non-entity verbs
  action_name:         string   // the non-entity action verb
                                //  ("schedule_repeat", "revive_experience")
                                //  when item is absent. Mutually exclusive
                                //  with item — the #2020 split that
                                //  replaces cta_action

  // REMOVED, not on the wire: thumbnail_media_id, thumbnail_caption,
  // headline_emphasis, context_id (superseded by item's fields, #2012
  // cleanup), action_label, cta_action (#2835 retired the last of these —
  // the lever-pill copy fields that predated the item/action_name split)
}
```

### Postcard tap dispatcher

Every postcard tap routes through `_openWorkshopDraft` in
[`nudge_content_view.dart`](../app/lib/presentation/widgets/nudge/nudge_content_view.dart):

```
NudgeContentView._openWorkshopDraft(context, ref)   // contextId from the nudge
    ↓
WorkshopRepository.generateDraft(experienceId: contextId)
    ↓ (server)
GenerateWorkshopDraft(experience_id) → name, description,
                                       time_unix_sec, location_id,
                                       media_ids, participant_ids
    ↓ (client)
genExperienceProvider.notifier.reset()       // clear prior tap state
genExperienceProvider.notifier.seedFromWorkshopDraft(...)
    ↓
showDialog(builder: ExperiencePreviewModal())
```

The `reset()` step is load-bearing: without it,
`ExperiencePreviewModal.initState` would read
`state.editedName ?? state.aiGeneratedName` and surface stale title/
description/time values from the previous postcard's tap.

The empty-context-id, empty-draft-name, and server-error paths all fall back
to `openBlankCreate` so the host isn't dead-ended. Async paths guard
`if (!context.mounted) return;` between `await` and navigation.

### GenerateWorkshopDraft

Implemented at
[server/services/workshop/draft_generation.go](../server/services/workshop/draft_generation.go).
Loads the prior `Experience` by id (the postcard's `context_id`) and
returns its templated fields plus the user-id list of attendees who
RSVPed YES. Authorization: the prior experience's `owner_id` must
match the calling user.

The proto field used to be `nudge_id` and resolved through a
`StoredNudge` lookup; that indirection has been removed since the
postcards already carry the underlying experience id directly.

## Detector chain (signal inputs to the postcard list)

The five cascade priorities + the per-event detector chain emit
`StoredNudge` rows that the brief assembler reads and ranks. Each
nudge becomes one postcard. The cards are uniform in shape — the
slot only affects copy.

### Ranking

`BriefRanker.Rank` ([brief_ranker.go](../server/services/workshop/brief_ranker.go))
sorts the nudges into two tiers:

1. **Interest tier (top).** Nudges whose `KickerLabel` is in
   `interestKickers` — currently `"Worth doing again"` and
   `"Pulse came back"`, both emitted by the ESM Repeat Signal
   detector when ≥2 attendees said `do_again`. These represent
   explicit "someone else is asking" signal and always sort above
   everything else.
2. **Recency tier.** Every other slot — active quest, calendar gap,
   seasonal, bring-back, recap fallback — sorted by
   `created_at_unix_sec` descending.

Within each tier, ties are broken by `created_at_unix_sec`
descending (newest first). The interest-tier boost is a flat
`+1_000_000_000_000` so no amount of recency lifts a non-interest
nudge above an interest one.

After sorting, the ranker dedupes by **rhythm key** (the normalized
first 60 characters of the headline) so two nudges naming the same
event collapse into one — the highest-ranked instance wins. The top
`maxBriefPriorities` (3) entries become `TopPriorities`; the rest
flow into `CTARows` and pass through `diversifyAndCap` to favor
`cta_action` diversity before any single action repeats.

New "interest" detectors (e.g., a future "Diego, Eli, and Maya
asked about your pressure washer" surface) opt into the interest
tier by reusing one of the existing `interestKickers` labels or
extending the set in [brief_ranker.go](../server/services/workshop/brief_ranker.go).

Every detector emits two pieces of copy that the postcard renders:

- **`Headline`** — action-shaped, referencing the prior thing
  ("Schedule Sunday smoker again", "Bring back Sunday brunch",
  "Confirm next Morning Hike").
- **`AtmosphereLine`** — signal/reason ("Alice and Bob want another",
  "Ran 6 times, then quiet", "Ran around this time last year",
  "Just wrapped Saturday").

Detectors also still set `KickerLabel`, `CtaLabel`, and `CtaAction` on
their detections — these are stored on `StoredNudge` for compatibility
with code that reads them, but the new postcard widget renders neither
the kicker chip nor the lever button.

### 1. Active Quest

Implemented at [active_quest_detector.go](../server/services/workshop/momentum/active_quest_detector.go).

**Fires when:** the host has an experience in `EXPERIENCE_STATE_ACTIVE` or
`EXPERIENCE_STATE_JOINED` whose name has at least one prior completed
instance (also owned by the host). The framing is "round N+1 is the
natural next move."

**Postcard copy:** Headline = `"Schedule {Name} again"`; AtmosphereLine =
`"Hosted N times before"`. Tapping opens the experience-preview modal
seeded from the most-rhythm-established active instance.

**Why this is highest:** the host has already started planning the next
instance. The card's job is to confirm it on the calendar with one tap,
not to invent a new event.

### 2. ESM Repeat Signal

Implemented at [esm_repeat_signal_detector.go](../server/services/workshop/momentum/esm_repeat_signal_detector.go).

**Fires when:** the host has a recently-completed experience (within 7
days) with strong post-event signal — at least 2 respondents on a
`StoredESMPrompt` for that experience, ≥60% of whom selected the
`do_again` option key.

The detector calls into [server/esm/](../server/esm/) (the ESM shared
library) to read prompts and aggregate responses (`AggregateAttendeesOnly`).
Aggregate signal is server-internal: the resulting `WORKSHOP_QUEST_HERO`
nudge is host-scoped via the existing `user_id` + `community_id` filter on
`StoredNudge`.

**Postcard copy:**

- Open-window variant: Headline = `"Schedule {Name} again"`;
  AtmosphereLine = `"{X} of {Y} said yes — pulse came back"`.
- Closed-window variant (after the prompt's 48-hour window expires):
  Headline = `"Make {Name} a weekly thing"`; AtmosphereLine =
  `"{X} of {Y} said yes, window closed"`.

The standalone `ESMRepeatSignalDetector` also surfaces a
names-formatted variant when the user records resolve cleanly:
`"Alice and Bob want another"` for two voters, `"Alice, Bob, and 2
others want another"` for more. The count-shaped fallback above kicks
in when the user lookup fails or returns no usable display names.

**Dependency:** ESM prompts must be materialized against the experience for
this to fire. In v1 they ship inline with the
`STORY_TYPE_EXPERIENCE_CONCLUDED` recap story (see
[docs/esm.md](esm.md)); v2's automated trigger fires on experience
completion.

### 3. Calendar Gap

Implemented at [calendar_gap_detector.go](../server/services/workshop/momentum/calendar_gap_detector.go).

**Fires when:** the host has a rhythm — at least 3 completed instances of
the same-named experience within the last 90 days — and the most recent
instance was at least 14 days ago. v1 detects rhythms by exact name
match.

**Postcard copy:** Headline = `"Bring back {Name}"`; AtmosphereLine =
`"Ran N times, then quiet"`.

### 4. Seasonal Trigger

Implemented at [seasonal_trigger_detector.go](../server/services/workshop/momentum/seasonal_trigger_detector.go).

**Fires when:** the host completed an experience around this time last
year (within a 30-day window centered on the anniversary) and hasn't
hosted that same-named experience in the last 60 days.

**Postcard copy:** Headline = `"Bring back {Name}"`; AtmosphereLine =
`"Ran around this time last year"`.

### 5. Idle Offer — unfilled

**No detector claims this slot.** The one that did was deleted in #2892. It
fired on the mere existence of a `CommunityGear` row — no idleness check, no
`GearState`, no `Transfer` activity, despite the name — so it was the default
card for any gear-owning host with no event signal. Its copy asserted what it
had never measured ("People keep asking about the {gear}"), asked the host to
share gear that was already shared, and proposed a "sharing event so everyone
can borrow it at once", which is neither a primitive in the product nor
possible for a single item. Its `propose_share` CTA passed a gear id to
`GenerateWorkshopDraft`, which looks up an `Experience` — so every tap
returned `NotFound` and dropped the host into a blank create modal.

`SlotIdleOffer` remains in the `Slot` enum: slot names are written to logs and
the `iota` numbering is load-bearing. A grounded replacement would key off
`Transfer` (`state`, `latest_request_unix_sec`) plus `CommunityGear.archived`
and `GearState`, and would need a CTA that resolves.

## Bring-Back signal

Implemented at [bring_back.go](../server/services/workshop/momentum/bring_back.go).

The Bring-Back detector emits `StoredNudge` rows with
`surface = NUDGE_SURFACE_WORKSHOP_BRING_BACK` for love-revival items —
recurring rhythms with a gap. In the home direction they're just one
more category of postcard, ranked alongside the per-event ones.

**Postcard copy:** Headline = `"Bring back {Name}"`; AtmosphereLine =
`"Ran N times, then quiet"`. Threshold constants (`BringBackMaxItems`,
`BringBackLookbackDays`) are unchanged.

## Per-event detection + recap fallback

Implemented at [server/services/workshop/momentum/per_event.go](../server/services/workshop/momentum/per_event.go).
For every recently-completed experience the host owns,
`DetectForEvent` walks a small priority chain:

1. **Active follow-on quest** — if a future-instance experience with
   the same name is already in `ACTIVE` or `JOINED` state. Headline =
   `"Confirm next {Name}"`; AtmosphereLine = `"Round N+1 already on
   deck"`.
2. **ESM repeat signal for this experience** (≥2 respondents, ≥60%
   `do_again`). Headline + AtmosphereLine match the standalone
   `ESMRepeatSignalDetector` copy described above.
3. **Recap fallback** — guarantees a postcard for every recently-
   completed event, even when no specific signal has fired yet.
   Headline = `"Schedule {Name} again"`; AtmosphereLine = `"Just
   wrapped {weekday}"`. KickerLabel is the literal string
   `"Just wrapped"` — `EnsureHeroCard` checks for that string to
   identify upgradeable cards.

**Recap-fallback upgrade.** The fallback is intentionally low-quality —
it's the placeholder that shows while richer signals are still
collecting. When `EnsureHeroCard` runs again later (e.g., on the next
pull-to-refresh), it re-detects per-event slots whose stored nudge
has `kicker_label == "Just wrapped"`. If the new detection's
KickerLabel is different (= a more specific slot fired), the fallback
nudge is soft-deleted (`consumed_at_unix_sec = now`) and replaced with
the upgraded card. This is what lets a card change from
"Schedule Sunday brunch again / Just wrapped Saturday" → "Schedule
Sunday brunch again / Alice and Bob want another" once two attendees
answer the ESM prompt.

Stored nudges that are NOT recap-fallbacks (kicker is anything else)
are left alone; only fallback → specific upgrades are allowed, never
specific → specific churn.

## Catalyst-pull (load distribution)

Implemented at [server/services/workshop/catalyst/](../server/services/workshop/catalyst/) (the shared
library). The library is unchanged but is **not yet wired into the
home assembly** — when the AI brief was removed, the catalyst
`seed_subhost` lever lost its inline-prose home. A future iteration
will surface catalyst pulls as a postcard (or as a separate row) with
templated copy; the current home does not show them. Library helpers
(`catalyst.DetectLoad`, `catalyst.GetSuggestion`,
`catalyst.HostModeEligibleUserIDs`, the rate-limit + decline-cooldown
stubs) all still exist and are exercised by tests.

## Recap-and-thanks (post-experience Story)

`STORY_TYPE_EXPERIENCE_CONCLUDED` Story rows are created by the
event-bus story subscriber at
[server/story/subscriber/experience.go](../server/story/subscriber/experience.go),
which fires when an experience-completed `CommunityEvent` is published
(the legacy background job under `server/jobs/` was removed in the
story-generation migration to the event bus — see
[docs/stories.md](stories.md)).

The recap Story is the surface where the embedded ESM prompt lives.
The home itself does not currently render a "publish the recap"
postcard — when the AI brief was removed, that surface went with it.
The recap Story is still the data source the per-event ESM detector
reads to score the `do_again` ratio; see [docs/esm.md](esm.md).

## Generation pipeline

```
GetWorkshopBrief(circle_ids[])
    ↓
auth.RequireAuth(ctx)
    ↓
auth.FilterActiveMemberCommunities                    // drop deleted/non-member
    ↓
logging.GoSafe — detached background goroutine:       ← materialization
    For each active community in scope:                  (idempotent;
        if !community.IsActive(ctx, store, id): skip      runs the
        EnsureHeroCard(user, community, now)              templated
        EnsureBringBackItems(user, community, "", now)    detector chain
                                                          per detection)
    ↓
loadBriefNudges(user, communityIDs)                   // batched per-community
    ↓
BriefRanker.Rank(nudges)                              // dedupe + interest-then-recency
    ↓
resolveContextThumbnails(ranked)                      // first-media id per row
    ↓
collectSeasonTotals(communityIDs)                     // acts / cost / time / CO₂
    ↓
countUniquePeople(communityIDs)                       // distinct members (#1898)
    ↓
buildTemplatedBrief(signal, ranked) → *momentum.Brief // CTARows only
    ↓
earliestCommunityCreatedAt(communityIDs)              // hero "Since X" anchor
    ↓
loadSuppressedKeysForScope + known_for.Derive         // specialties chips
    ↓
available_now.Gather(communityIDs)                    // available_now_items rail
    ↓
briefToPayload(brief, thumbs, signal,                 // wire shape
               sinceUnixSec, specialties, availableNow)
    ↓
return BriefPayload
```

A few important details:

- **No AI anywhere on this surface.** The handler is fully templated, and
  since #2936 so is card materialization: `EnsureHeroCard` →
  `momentum.CopyGenerator` now only runs the lever word-count guard on the
  detector's own copy. Materialization still happens in a detached goroutine,
  so the brief response stays sub-second and the next pull-to-refresh picks up
  freshly-materialized cards.
- **Service Independence.** The workshop service queries `StoredNudge`
  storage directly. It does not call the feed service or the ESM
  service — detectors that consume ESM signal go through the
  `server/esm/` shared library. Cross-service coupling is forbidden
  per [docs/server/architecture.md](server/architecture.md).
- **Active-community gate.** The materialization goroutine re-checks
  `community.IsActive(ctx, store, communityID)` inside the loop so a
  community soft-deleted between the request entering and the goroutine
  running doesn't get fresh nudges resurrected. Enforced by
  [`scripts/check_active_community_gate.js`](../scripts/check_active_community_gate.js)
  Rule D.
- **Goroutine recovery.** The materialization goroutine is spawned
  through [`logging.GoSafe`](../server/logging/logger.go), which wraps
  it in a `defer recover()` so a panic logs and dies in isolation
  rather than crashing the server. Enforced by
  [`scripts/check_ephemeral_comments.js`](../scripts/check_ephemeral_comments.js)'s
  sibling goroutine-protection check.
- **Pre-warming.** A scheduled `WorkshopGenerationJob` (in
  `server/jobs/workshop_generation.go`) walks active (host, community)
  pairs and calls `EnsureHeroCard` ahead of host-tab-open so the screen
  is instant when the brief is first requested. The job no longer
  pre-warms an AI brief — that path was removed when the brief itself
  was removed.
- **N+1 guard.** The signal aggregator uses batched
  `storage.QueryByFields` calls — never per-experience round-trips.
  Tests use `storage.AssertMaxQueries` to pin the bound.

## Client rendering

The Workshop tab is at
[app/lib/presentation/screens/workshop/workshop_screen.dart](../app/lib/presentation/screens/workshop/workshop_screen.dart).

Layers:

- **`WorkshopRepository`** wraps the `WorkshopServiceClient` with
  transparent caching.
  - Brief: namespace `'workshop_brief'`, key `'brief:<sorted_ids>'`.
  - Synthesis: namespace `'workshop'`, key `'synthesis:<sorted_ids>'`.

  Both use the `CacheService` defaults. `WorkshopRepository.invalidateAll()`
  clears both namespaces and is what `WorkshopNotifier.refresh` calls
  before re-running `build()`.
- **`WorkshopNotifier`** (`AsyncNotifierProvider.autoDispose`, state
  `WorkshopState`) holds the brief + synthesis. `build()` fetches both
  in parallel via `Future.wait` (scoped to
  `workshopEnabledCommunityIdsProvider`) with a `ref.mounted` guard
  between the await and any state writes. `refresh()` invalidates the
  repo caches, calls `ref.invalidateSelf()`, and `await future` so a
  `RefreshIndicator` spinner dismisses only when the rebuild has
  settled. `WorkshopState` still exposes `hasContent`
  (`hasBrief || hasSynthesis`), but the post-#2447 overview renders
  every page regardless — each section shows its own zero-state
  prompt rather than collapsing the page. The only empty placeholder
  now is the no-communities case (`communities.isEmpty`).

**Post-#2447 (current).** The tab is a **vertical `PageView`** — one full-bleed
community page per circle (like the Feed). Swiping up/down moves between
communities and pins the one on screen via `workshopCommunityProvider`; each
page wraps its body in a `ProviderScope` that overrides
`workshopEnabledCommunityIdsProvider` to its own circle so the brief, member
rollup, glimpse, and library all resolve per-page. There is **no** carousel /
"Everything" scope selector and **no** scrolling list of glass cards — the body
is a single centered editorial column. Each page is a `Stack` of a
[`WorkshopBackdrop`](../app/lib/presentation/widgets/workshop/workshop_backdrop.dart)
(the community's first photo), a balanced `_OverviewScrim`, and the centered
overview, with a pinned `FloatingHeader` (the switcher pill) on top.

- [`WorkshopSwitcherPill`](../app/lib/presentation/widgets/workshop/workshop_switcher_pill.dart)
  sits in the `FloatingHeader` and names the community on screen. Tapping it
  turns the pill into a search field and opens the inline
  [`WorkshopSwitcherDropdown`](../app/lib/presentation/widgets/workshop/workshop_switcher_dropdown.dart)
  anchored just below the header (search + list + create/join); selecting a
  community jumps the pager to it.
- [`WorkshopCenteredOverview`](../app/lib/presentation/widgets/workshop/workshop_centered_overview.dart)
  is the whole body — a vertically-centered column over the photo, with these
  tappable blocks top to bottom: an **eyebrow** (community name · member count),
  a **serif member-name rollup** (tap → roster), a **metrics prose sentence**
  with four underlined tappable numbers — problems, money, hours, CO₂ (tap →
  the matching deep dive) — that is replaced by a `_CrewInvitation` zero-state
  until any number is non-zero, a **latest-message** glimpse
  ([`workshopGlimpseProvider`](../app/lib/presentation/viewmodels/workshop_glimpse_view_model.dart),
  tap → conversation), an **on-the-calendar** block (the soonest upcoming
  experience, tap → library calendar), and a **Common Ground** chip row (the
  `specialties` tags plus a leading "All", tap → library map filtered to that
  category). There is no `cta_rows`/postcard "help" card on the overview — the
  postcard surface was not carried into the centered design.
- Deep dives **morph-open** as overlays rather than `push`-ing a route: each
  block reports its tapped footprint `Rect`, and `_expandOverlayScreen` calls
  `openContentMorphPanel` to grow the destination from that rect, hosted under
  [`WorkshopOverlayHost`](../app/lib/presentation/widgets/workshop/workshop_morph.dart)
  so it overlays the same community photo. Destinations: the roster sheet, the
  `WorkshopTime/Money/Co2/ProblemsSolvedDetailScreen` metric screens (each
  passed `overlay: true`),
  [`WorkshopConversationPanel`](../app/lib/presentation/screens/workshop/workshop_conversation_panel.dart),
  and the library
  [`WorkshopLibraryMapPanel`](../app/lib/presentation/screens/workshop/workshop_library_map_panel.dart)
  /
  [`WorkshopLibraryCalendarPanel`](../app/lib/presentation/screens/workshop/workshop_library_calendar_panel.dart),
  which share
  [`workshop_library_common.dart`](../app/lib/presentation/screens/workshop/workshop_library_common.dart).
  The calendar reuses the cached `GetHomeView` read (the **same** feed as the
  inbox calendar, filtered to the in-scope communities) and ships the full
  month-grid × photo-backdrop visual with per-day weather + open-day suggestions
  (#2514); the map's pin-map visual is a follow-up.

**Pre-#2447 (superseded).** The prior light-surface assembly reused the
user-profile section widgets — a community carousel and identity block,
`KnownForChips`, `ImpactRows`, `AvailableNowRail`, and `LeanCards` postcards
dispatched through `WorkshopLeverDispatcher.openPostcard`, plus a
`KnownForCategoryDetailScreen` and the legacy `SeasonReportScreen`. All of it
has been deleted; git history before #2447 has the details. What survives from
that era is the per-dimension detail screens
([`WorkshopTimeDetailScreen`](../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart),
[`WorkshopMoneyDetailScreen`](../app/lib/presentation/screens/workshop/workshop_money_detail_screen.dart),
[`WorkshopCo2DetailScreen`](../app/lib/presentation/screens/workshop/workshop_co2_detail_screen.dart),
[`WorkshopProblemsSolvedDetailScreen`](../app/lib/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart)),
each of which re-fetches its own per-community metric breakdown from the impact
repository rather than carrying a brief seed total.

Surfacing discipline (cap on visible postcards) is enforced
client-side inside `LeanCards`: it shows the first
`_initialVisibleCount` (3) rows by default and reveals the rest behind
the `_ShowMoreLink`. The server returns up to `maxBriefCTARows` rows in
`BriefRanker.diversifyAndCap`.

## Configuration thresholds

All in one place for tuning:

| Constant | Value | Where |
|----------|-------|-------|
| `MaxLeverWords` | 9 | [lever_copy.go](../server/services/workshop/momentum/lever_copy.go) — applies to hero card copy + each detector's `CtaLabel` |
| `maxBriefPriorities` | 3 | [server/services/workshop/brief_ranker.go](../server/services/workshop/brief_ranker.go) |
| `maxBriefCTARows` | 5 | same |
| `RecentEventLookbackDays` | (see momentum constant) | [server/services/workshop/momentum/per_event.go](../server/services/workshop/momentum/per_event.go) — how far back the per-event flow looks for completed experiences |
| `ActiveQuestRecurringInstances` | 1 | [active_quest_detector.go](../server/services/workshop/momentum/active_quest_detector.go) |
| `ESMRepeatSignalThreshold` | 0.60 | [esm_repeat_signal_detector.go](../server/services/workshop/momentum/esm_repeat_signal_detector.go) |
| `ESMRepeatSignalMinRespondents` | 2 | same |
| `ESMRepeatSignalLookbackDays` | 7 | same |
| `CalendarGapMinInstances` | 3 | [calendar_gap_detector.go](../server/services/workshop/momentum/calendar_gap_detector.go) |
| `CalendarGapRhythmWindowDays` | 90 | same |
| `CalendarGapMinGapDays` | 14 | same |
| `SeasonalAnniversaryDays` | 30 | [seasonal_trigger_detector.go](../server/services/workshop/momentum/seasonal_trigger_detector.go) |
| `SeasonalNotRecentDays` | 60 | same |
| `BringBackMaxItems` | 3 | [bring_back.go](../server/services/workshop/momentum/bring_back.go) |
| `BringBackLookbackDays` | 365 | same |
| `LoadStreakThreshold` | 3 | [load.go](../server/services/workshop/catalyst/load.go) — currently unused on the home, library still exercised by tests |
| `RateLimitMaxPullsPerWindow` | 3 | [suggestions.go](../server/services/workshop/catalyst/suggestions.go) — stub |
| `RateLimitWindowDays` | 14 | same — stub |
| `DeclineCooldownDays` | 30 | same — stub |
| `recapFallbackKicker` | `"Just wrapped"` | [server/services/workshop/generation.go](../server/services/workshop/generation.go) — sentinel for fallback-upgrade detection |

## Adding a new detector

Detectors emit `StoredNudge` rows; the brief assembler reads them and
the home renders one postcard per nudge.

1. Create a new file under [server/services/workshop/momentum/](../server/services/workshop/momentum/) (e.g.
   `your_detector.go`).
2. Implement the `Detector` interface:
   ```go
   type Detector interface {
       Slot() Slot
       Detect(ctx, store, userID, communityID) (*Detection, error)
   }
   ```
3. **Every user-visible field must be derivable from a value the detector
   read from storage.** Counts, names and dates come from the query. Social
   proof ("people keep asking about X") requires a query that measured it.
   A citation requires a real source. Say what you counted, and no more:
   "most of the circle" is wrong when the threshold ran over respondents,
   and "and it landed" is wrong when all you know is that the event
   completed. Copy that asserts what the detector never measured is what
   #2892 deleted a whole slot over — a host who catches the product making
   something up stops trusting every suggestion after it.

   `TestCopyCorpus_IsAccountedFor`
   ([copy_corpus_test.go](../server/services/workshop/momentum/copy_corpus_test.go))
   holds a golden list of every string the package can put on screen. New
   copy fails until it is listed, so the diff is where a reviewer asks
   "did we measure that?".

4. The `Detection` must populate:
   - `Headline` — action-shaped, referencing the prior thing
     ("Schedule X again", "Bring back X", "Confirm next X"). This
     becomes the postcard title.
   - `AtmosphereLine` — a signal/reason ("Alice and Bob want
     another", "Ran 6 times, then quiet"). This becomes the italic
     subtitle.
   - `CtaLabel` — must pass `ValidateLeverCopy` (≤9 words). Carried
     on the wire as `BriefCTARow.action_label` for back-compat but
     not rendered.
   - `KickerLabel` — short slot identifier ("On the way", "Worth
     doing again"). Stored on the nudge for back-compat; not
     rendered in the new postcard.
5. Add your detector to `DefaultDetectors()` in
   [detector.go](../server/services/workshop/momentum/detector.go) at the right priority
   slot.
6. Add your copy to `wantCopyCorpus` in
   [copy_corpus_test.go](../server/services/workshop/momentum/copy_corpus_test.go).
7. Write unit tests in
   [detectors_test.go](../server/services/workshop/momentum/detectors_test.go) covering
   at least: empty-input no-op, the canonical fires-correctly case,
   the "almost-fires-but-doesn't" suppression case.
8. Run `go test ./server/services/workshop/momentum/... ./server/services/workshop/...`.

## Data prerequisites for testing in dev

The Workshop's empty state fires when no detector produces signal —
the home will still render the hero + ticker numerals from
`impact_metrics`, but no postcards. To exercise individual signals,
seed:

| Signal | Minimum data |
|---|---|
| Active Quest | 1 completed `Experience` named e.g. "Sunday brunch" + 1 active `Experience` with the same name, both `owner_id=$you` |
| ESM Repeat Signal | 1 completed `Experience` (within 7 days) + 1 `StoredESMPrompt` against it + 2+ `StoredESMResponse` rows with `consumed_action=RESPONDED` and `response_option_key="do_again"` |
| Calendar Gap | 3+ completed `Experience` rows with the same name, all 14+ days ago, the most recent within 90 days |
| Seasonal Trigger | 1 completed `Experience` ~365 days ago (within ±15 days of today's date last year), no other instance in the last 60 days |
| Bring-Back signal | Same as Calendar Gap (3+ instances, gap, within 365 days). When Active Quest also fires, this stays in the candidate set but ranks below the active quest. |
| Recap fallback | 1 completed `Experience` you own (within `RecentEventLookbackDays`). Always materializes when nothing more specific fires; replaced by a specific-signal card when one becomes available. |

**Forcing a re-materialization in dev.** Existing nudges block
re-detection (the per-event flow is idempotent). To exercise new
detector copy or threshold changes against past data, wipe the rows:

```sql
DELETE FROM stored_nudge WHERE user_id = '<your-user-id>';
```

The next pull-to-refresh on the Workshop tab fires the background
materialization goroutine, which repopulates from the current
detector chain. Recap-fallback rows specifically also auto-upgrade
on subsequent passes when a more-specific signal arrives.

## Known limitations

- **No ESM authoring affordance on the home.** Recap stories
  (`STORY_TYPE_EXPERIENCE_CONCLUDED`) are now auto-created by the
  event-bus subscriber on experience completion, so the ESM Repeat
  detector sees real recap-borne prompts. But the host has no on-home
  surface to author or tune ESM prompts directly; manual authoring
  still goes through the runbook.
- **Catalyst-pull is no longer surfaced.** When the AI brief was
  removed, the inline `seed_subhost` lever lost its home. The
  underlying library still works; a future iteration will surface
  it as a postcard or a separate row.
- **Catalyst rate-limit + decline cooldown are stubs.** The helpers
  exist in `server/services/workshop/catalyst/` with documented stub bodies that
  always return false. They activate when the `CatalystPullRecord`
  proto + chat-message dispatch ship.
- **No per-attendee detection.** Lapsed-regular and bridge-invite
  cards are deferred until the `server/attendance/` per-attendee
  model lands.

## Related references

- [docs/issues/1618-workshop-daily-brief.md](issues/1618-workshop-daily-brief.md)
  — the canonical plan for the brief direction; phases, decisions, and
  cleanup work.
- [docs/issues/1579-workshop-tab.md](issues/1579-workshop-tab.md) — the
  v1 plan that established the cascade detector chain and the
  per-event Hero card model. The detector chain is unchanged; the
  rendering layer changed in #1618.
- [docs/issues/1580-esm-cards.md](issues/1580-esm-cards.md) — the ESM
  feed-card surface that produces the data the ESM Repeat Signal
  detector consumes.
- The host journey — the 27-step path from first idea to keeping a
  recurring thing alive — used in the gaps analysis below.
- [docs/nudges.md](nudges.md) — the nudge data model the cascade
  detectors persist into.
- [docs/esm.md](esm.md) — ESM prompt + response data model and the
  per-event ESM Repeat Signal detection logic.

## Coverage gaps against the host journey

The host journey lays out the 27 steps a host takes from first idea to
keeping a recurring thing alive.
The momentum layer (phases 4–6, steps 17–27) is the product's
differentiator — the work that turns one-offs into ongoing things.

### What the Workshop covers

| Journey step | Priority | Postcard surface |
|---|---|---|
| 19 — Decide if it should recur | 9 | ESM Repeat Signal postcard with names atmosphere line |
| 20 — Schedule the next one | 16 | Active Quest postcard + `GenerateWorkshopDraft` prefill in `ExperiencePreviewModal` |
| 23 — Notice early decay | 15 | Calendar Gap → "Bring back X" postcard with `Ran N times, then quiet` |
| recap fallback | — | Generic "Schedule X again / Just wrapped" postcard for any recently-completed event |
| seasonal | — | "Bring back X / Ran around this time last year" postcard |

### What the Workshop covers partially

- **Step 17 — Thank people.** The post-experience recap Story is the
  vehicle and is now auto-created on completion by the event-bus
  subscriber, but the home no longer renders a "publish the recap"
  postcard (the lever was AI-prose, removed with the brief).
- **Step 18 — Read the room.** ESM prompts can be authored manually;
  no host-tab affordance for authoring lives on the home.
- **Step 21 — Maintain the cadence.** No "still on this week?" pulse
  type; the ESM library doesn't have a pre-event cadence-check
  prompt template.
- **Step 25 — Refresh the format when stale.** No detector for
  "stale" signal yet.
- **Step 27 — Pause or retire gracefully.** No detector identifies
  the right moment.

### What the Workshop does not cover yet

- **Step 22 — Rotate or share host load.** Catalyst-pull lost its
  inline-prose home when the AI brief was removed; the surface
  hasn't been re-introduced as a postcard.
- **Step 24 — Re-engage lapsed regulars.** Needs per-attendee
  detection (`server/attendance/`).
- **Step 26 — Bring in new blood.** Cross-circle bridge; needs
  per-attendee + interest-graph data.
- **Steps 4–13 — most of the single-event layer.** Out of scope for
  the Workshop *by design* — those belong in the create flow.

### Cross-cutting gaps

- **Per-attendee detection.** Blocks steps 22–26.
- **Catalyst-pull on the new home.** Library is intact; no postcard
  surface yet.
- **ESM authoring affordance on the home.** The bottom-sheet
  authoring widget still exists (`EsmAuthorSheet`) but no postcard
  opens it.

### Recommended sequencing

1. **Add a Catalyst postcard surface.** Reintroduce step 22 with a
   templated copy ("Pass round N to {Name}" / "You've hosted the
   last N in a row").
2. **Add a "publish the recap" postcard.** Recap stories already
   auto-create via the event-bus subscriber; the gap is a home surface
   that opens the recap for the host to publish. Lights up step 17.
3. **Wire the v2 ESM automated trigger** ([docs/issues/1580-esm-cards.md](issues/1580-esm-cards.md)
   Phase 4). Step 18 fires automatically; the ESM Repeat detector
   then sees real signal in production.
4. **Per-attendee detection model.** Unlocks steps 23–26.
