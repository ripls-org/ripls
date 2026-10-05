---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Nudge cards — the hand-written feed terminator and the momentum engine's host prompts, with async stock imagery, seen/consume expiry, and the daily-rotating terminator pool. Generic AI-written nudges were removed by #2936.
  globs: [server/services/feed/**, app/lib/presentation/widgets/nudge/**]
  triggers: [nudge, terminator, feed-card, consume-nudge, stock-image, cta, all-caught-up]
  lens: [domain, client, server]
  domain: feed
freshness:
  verified_commit: "e52dda434"
  verified_on: "2026-09-20"
---
# Nudge Cards

## Overview

Nudge cards are system-generated cards rendered by `NudgeContentView`. Two
kinds survive:

- **The feed terminator** — the "you're all caught up" card that closes the
  feed. Hand-written copy from a static pool, stating a fact.
- **Host prompts** — cards the momentum engine writes on
  `NUDGE_SURFACE_WORKSHOP_*` when a real signal fires: an event worth
  repeating, a gap in the calendar, an offer nobody has taken up. They name
  the entity they act on and route into its flow.

**Generic nudges are gone (#2936.)** They were the third kind and by far the
most numerous: a per-user pool an LLM wrote — headline, description, CTA
label — illustrated by an async stock-image fetch, interleaved into the feed
every eight items and used to fill the Home zero state. Nothing about them
described a fact; they invented a reason to prompt you, and read as generated.

What replaced them on Home is three unconditional affordances — **Plan an
event**, **Ask for help**, **Offer something** — which need no pool, no
imagery, and no server round trip, so the zero state can never be empty. The
calendar's open day falls through to its static plan prompt. See
[client/inbox.md](client/inbox.md).

## Architecture Principles

**Community-scoped:** A host prompt belongs to a `(user_id, community_id)`
pair. Nudges are not deduplicated or shared across communities.

**The card owns its frame.** `NudgeContentView` is a full-bleed composition —
background imagery, radial overlay, headline, and its own filled CTA — so
every host gives it a frame of its own and bounds its height; the variants are
`Stack(fit: StackFit.expand)` and cannot self-size. It is never inset inside
another card's copy. Nesting it inside the Home zero state's request hero put
two headlines and two primary CTAs in one box (#2796).

**Consumed on tap:** When a user taps a CTA, the card is immediately removed
from local state (optimistic dismiss) and marked consumed server-side via
`ConsumeNudge`. Consumed nudges never reappear.

**Age expiry:** Rows older than 24 hours since creation are soft-deleted
(asynchronously) on the next feed or inbox load via `pruneExpiredNudges`.
Soft-deleted records are retained for analytics — never hard-deleted.

**Terminator is special:** It uses a daily-rotating pool of static copy, is
never subject to age-expiry, and always routes to experience creation.

## Feed Placement

The feed appends the terminator and nothing else. `appendTerminator`
(`server/services/feed/feed_nudges.go`) is the whole of it;
`TestAppendTerminator_NoInterleavedNudges` pins that stored rows never surface
between content items.

`GetFeedRequest.debug_nudge_count` is deprecated and ignored — it asked the
server to generate and prepend N fresh nudges for development, and there is
nothing left to generate.

## Terminator Pool

The feed terminator uses a static pool of headline/description pairs defined in [terminator_pool.go](../server/services/feed/terminator_pool.go) — hand-written, never generated. Each day, exactly `TerminatorsPerDay` (default: 5) **global** records are created from that pool, with one stock-image fetch per slot. These records are shared across all users and all communities: everyone sees the same 5 headlines, descriptions, and background images on a given day.

When `buildTerminator` is called, it:
1. Soft-deletes any global terminator records from previous days
2. Fills the global pool up to `TerminatorsPerDay` if not yet created (the first user to load their feed that day triggers this)
3. Picks one at random from today's pool, preferring records that already have imagery

The slot index for each entry is derived from `((day-1) * TerminatorsPerDay + slot) % len(pool)`, ensuring slots within a day are distinct and the full set rotates across days.

Global records are stored with `user_id = "_terminator_pool_"` and no community ID. Their stock imagery is owned by `media.SystemUserID`, not by whoever's feed load created it: the card is served to everyone, and media owned by an individual is readable only by that person — every other viewer got `PermissionDenied` from GetMedia and a card with a broken image (#3105). System-owned stock is public to any authenticated caller by design. When a new day begins, the previous day's global records are soft-deleted and a fresh set is created, so a day's pool is the only imagery in play at a time.

**Configuration:** Change `TerminatorsPerDay` in [terminator_pool.go](../server/services/feed/terminator_pool.go) to control how many options are generated per day.

### Terminator copy rules

Every pool entry must follow these rules — the headline, description, and CTA are a unit and must be coherent with each other:

- **Headline:** Signals the user is done with the feed ("You're all caught up", "Nothing left to see", etc.)
- **Description:** Encourages going outside, calling a friend, or making a plan. Never about sharing gear, returning favors, or any other in-app action.
- **CTA label:** Always `"Plan something"` (or a close variant like `"Get outside"`).
- **CTA action:** Always `plan_experience` — opens the experience creation modal.

Entries that say "share something you've got" or "what are you going to share?" are incoherent with a "Plan something" button and must not appear in the pool.

## Host prompts on the inbox

The momentum engine ([workshop.md](workshop.md)) writes `StoredNudge` rows on
`NUDGE_SURFACE_WORKSHOP_*` — "Schedule {event} again", "Bring back {event}" —
that name the entity they act on. Their reader was the Workshop tab, which left
the nav with #2568, so for a while nothing surfaced them at all.

The **Home inbox** reads them now. `queryActiveNudges` takes a `nudgeAudience`
(`server/services/feed/nudge_storage.go`):

- `feedAudience` — imagery required, feed-surface only. A host prompt has no
  business appearing between two pieces of community content.
- `inboxAudience` — text-only accepted, plus any workshop-surface row carrying
  a `context_id`. A workshop row *without* one stays out: it would dispatch to
  an empty create modal, and the Home affordances already cover blank creation.

`InboxNudge` returns **host prompts only** (#2936). It used to rank
host prompt → has imagery → text-only across a mixed pool; the tiering existed
because the momentum engine skips stock imagery for these surfaces, so ranking
on pictures alone buried "schedule your Wednesday run again" behind whatever
the generic pool last generated — a real bug the `runclub` walkthrough caught
(docs/walkthroughs.md). With the generic pool deleted there is nothing left to
outrank.

Their lever copy is short and verb-shaped ("Schedule it", "Bring it back")
rather than a restatement of the headline: on the Workshop postcard the label
was never rendered beside the headline, but on a nudge card the two sit
together and a repeat reads as a stutter.

**They wear the referenced event's own photo.** These surfaces skip stock
imagery (`StockQuery: ""`), so before this the card was a headline on black.
`persistHeroCard` now copies the first `media_id` off the experience
`context_id` points at (`contextMediaID`,
`server/services/workshop/generation.go`). Asking a stock provider for a
picture of "schedule Wednesday morning run again" would be both wasteful and
worse than the run's own photo. An event with no media leaves the card on a
solid background; it is never a reason to fail materialization.

## Nudge Variants

Each nudge has a `nudge_variant` (1–3) set by whatever wrote the row — the momentum detector for host prompts, the terminator builder for the feed-end card. The variant controls the card layout rendered on the client.

| Variant | Name | Layout |
|---------|------|--------|
| 1 | Magazine Spread | Centered serif headline, italic description, optional location pill, single CTA |
| 2 | Ticker Tape | Centered content, bottom stats bar with up to 3 stats, primary + optional secondary CTA |
| 3 | Headline Card | Simple centered layout, used for the feed terminator |

## CTA Actions

Each nudge carries a `cta_action` string that the client maps to a navigation target. The `secondary_cta_action` field supports an optional second button (variant 2).

| Action | Navigation |
|--------|-----------|
| `plan_experience` | Experience creation modal |
| `list_item` | Gear creation modal |
| `ask_for_help` | Request creation modal (uses current community ID) |
| `check_listings` | Experience creation modal (fallback) |

Workshop-surface nudges ([workshop.md](workshop.md)) — `surface` =
`NUDGE_SURFACE_WORKSHOP_*` — reuse the same card and add CTAs that reference an
**existing** entity via `NudgePayload.context_id`, so they land the host in that
entity's flow rather than an empty modal:

| Action | Navigation |
|--------|-----------|
| `schedule_repeat`, `revive_experience` | `openRepeatDraft` → `WorkshopRepository.generateDraft` → experience preview modal, pre-filled from the prior instance |
| `seed_subhost` | blank creation (defensive fallback — the catalyst-pull path normally handles it) |

`propose_share` is **retired** (#2892). It was the idle-offer detector's
lever, and its `context_id` was a gear id — which `GenerateWorkshopDraft`
looks up as an `Experience`, so it returned `NotFound` on every tap and the
client fell through to blank creation anyway. The client no longer has a
branch for it; a stale persisted row lands on `default:`, reaching the same
blank modal without the failing round trip. Every remaining workshop
`context_id` is an experience id.

`openRepeatDraft` ([blank_create_dispatcher.dart](../app/lib/presentation/screens/create/blank_create_dispatcher.dart))
is shared, not nudge-private: the same helper backs the **wrapped-up event's
"Schedule the next one" chip** at the foot of its Who's-in card
([experience_read_shell.dart](../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart)).

**It reads everything it needs from `ref` before its first `await`.** The
Home inbox dismisses its nudge optimistically on tap, unmounting the widget
that owns the ref — and Riverpod *throws* on a ref used after its widget is
gone, so a post-round-trip `ref.read` is a crash, not a stale value. The
blank-create fallbacks still need a live host and are guarded on the caller
still being mounted; that split is fine in practice, because the surface that
dismisses itself is the one whose draft always resolves.

Anything unrecognized falls through to `plan_experience`'s blank creation modal,
which is also what the terminator uses.

Every `cta_label` is now written by a human — the momentum detector's templates
or the terminator pool — so the ["Event", not "Experience"](client/conventions.md)
rule is enforced the ordinary way, by reading the copy.

It used to be otherwise. The label was model-authored, and the prompt handed
the model `"Plan an Experience"` as an exemplar, which it copied verbatim into
production cards (#2803). Because the offending string was generated rather
than written, it passed every literal-scanning gate downstream, and the guard
had to live on the prompt bytes themselves. That whole class of problem goes
away with the prompt.

### Coherence rule

The headline, description, and CTA label of every nudge must all point at the
same action. A reader who sees only the description must be able to predict
what the button does, and the button label must deliver exactly that. This
applies to the terminator pool and to every momentum-detector template.

| Action | Description is about | CTA label examples |
|--------|---------------------|-------------------|
| `plan_experience` | Planning an outing, adventure, or group activity | "Plan an Event", "Plan a trip" |
| `list_item` | Sharing or lending gear you own | "List an item", "Share your gear" |
| `ask_for_help` | Posting a request for help, skills, or a tool | "Ask for Help", "Post a request" |
| `check_listings` | Borrowing something available nearby | "Check listings", "See what's available" |

Tapping a CTA opens the corresponding creation modal on top of the current screen. The nudge card remains visible behind the modal.

## Data Model

### Server (`StoredNudge`)

Defined in [nudge.proto](../proto/ripls/models/nudge.proto). Key fields:

- `id`, `user_id`, `community_id` — identity and ownership
- `nudge_variant` — layout variant (1–3)
- `headline`, `description`, `cta_label`, `cta_action` — the card's copy, from the momentum detector's templates or the terminator pool
- `optional secondary_cta_label`, `optional secondary_cta_action` — second button (variant 2)
- `optional location_hint` — shown in the glass pill on variant 1
- `optional media_id` — set once async imagery fetch completes; nil means invisible
- `repeated stats` — value+label pairs for the variant 2 stats bar
- `stock_query` — search keywords used to fetch background imagery
- `is_terminator` — true for feed-end nudges
- `surface` (`NudgeSurface`) — which surface generated the nudge. `queryActiveNudges` takes a `nudgeAudience` rather than a bare surface filter: the **feed** keeps only `NUDGE_SURFACE_FEED` (and the unspecified zero value, for pre-existing rows), while the **inbox** additionally takes `NUDGE_SURFACE_WORKSHOP_*` rows that carry a `context_id` — see *Host prompts on the inbox* below
- `optional context_id` — the existing entity a Workshop-surface CTA references; empty for feed-surface nudges. `nudgeToPayload` forwards it, without which such a CTA dispatches to an empty create modal
- `optional consumed_at_unix_sec`, `optional consumed_action` — set on CTA tap
- `optional deleted` — soft-delete metadata (retained for analytics)

### API (`NudgePayload`)

Defined in [feed_service.proto](../proto/ripls/api/feed_service.proto). Sent to the client as part of `FeedItem` with type `FEED_ITEM_TYPE_NUDGE`. The payload carries `media_ids` (resolved from `media_id`) for the client to look up presigned URLs via `mediaUrlProvider`.

## Server Implementation

| File | Purpose |
|------|---------|
| [nudges.go](../server/services/feed/nudges.go) | `fetchNudgeImagery` and shared helpers |
| [nudge_storage.go](../server/services/feed/nudge_storage.go) | Storage helpers: insert, fetch active, update media ID, mark consumed, soft-delete |
| [feed_nudges.go](../server/services/feed/feed_nudges.go) | `appendTerminator`, `InboxNudge`, `buildTerminator`, `nudgeToFeedItem`, freshness/expiry logic |
| [terminator_pool.go](../server/services/feed/terminator_pool.go) | Static pool of terminator copy, `GetTerminatorForSlot`, `TerminatorsPerDay` constant |

The terminator's imagery depends on an optional `StockImageryProvider` injected on the feed `Service` via `SetStockImageryProvider`; if nil, `fetchNudgeImagery` skips the fetch and the card renders without a background image. The feed `Service` also stores an `ai.Provider` via `SetAIProvider` (still wired in `server/wiring.go`), but nothing in this package reads it — that field only backed the now-deleted generic nudge generator.

## Client Implementation

| File | Purpose |
|------|---------|
| [nudge_content_view.dart](../app/lib/presentation/widgets/nudge/nudge_content_view.dart) | `NudgeContentView` ConsumerWidget — resolves media URL and attribution, dispatches to variant builder, handles CTA routing |
| [nudge_card_variants.dart](../app/lib/presentation/widgets/nudge/nudge_card_variants.dart) | `NudgeCardVariants` — static builders for each of the 3 layouts |
| [nudge_presentation.dart](../app/lib/presentation/widgets/nudge/nudge_presentation.dart) | `NudgePresentation` — full-screen vs. embedded geometry |

`NudgeContentView` is a widget, not a screen — it renders inside the `PageView` in `FeedScreen` like any other content view. Media URLs are resolved via `mediaUrlProvider` using the nudge's `media_id` as a stable cache key. All text styles derive from constants defined in [app_theme.dart](../app/lib/core/theme/app_theme.dart) (`nudgeHeadlineLargeStyle`, `nudgeHeadlineMediumStyle`, `nudgeBodyStyle`, `nudgeStatValueStyle`).

### Presentation

A card renders in one of two frames, chosen by the host:

| `NudgePresentation` | Host | Geometry |
|---|---|---|
| `fullScreen` (default) | the feed's `PageView` | owns the screen edges — internal `SafeArea`, editorial padding, 32 px headline |
| `embedded` | the calendar's open-day card (200–280 px), the Home zero-state hero (320 px) | no `SafeArea` (the host is already inside one), tighter padding and gaps, 26 px headline, compact CTA |

In both frames the headline and description **fit their line count to the height they are given**. `TextOverflow.ellipsis` alone does not protect a text block whose *height* is the binding constraint — `RenderParagraph` clips the raster mid-glyph instead of ellipsizing, which is exactly what an embedded card did to its description (#2801). The description is the sole `Flexible`, so its incoming `maxHeight` already is whatever its fixed siblings left over; the headline gets a fixed share of the content box (`_headlineHeightShare`) so it cannot eat a small card whole. Line heights are text-scaled, so the budget survives large accessibility font sizes. When not even one line fits, the block renders nothing rather than a fraction of a line.

### Photo attribution

Stock imagery from Unsplash **requires** visible attribution ([stock_imagery.md](stock_imagery.md)). When `mediaAttributionProvider` resolves an `Attribution` for the card's background, the shared [`PhotoAttributionLine`](../app/lib/presentation/widgets/content/photo_attribution_line.dart) — the same credit the content views use — renders top-right over the imagery, last in the `Stack` so the CTA still precedes it in reading order. Embedded cards reserve `_attributionReserve` of extra top padding so the centered headline can't run under it. This replaced an ⓘ button that opened the credit in a bottom sheet at the far edge of the screen (#2801).

## Consumption State

The `FeedNotifier` in [feed_view_model.dart](../app/lib/presentation/viewmodels/feed_view_model.dart) manages a `consumedNudgeIds` set in `FeedState`. `FeedScreen` filters out consumed IDs from the visible item list, so tapped nudges disappear immediately without waiting for a server round-trip or feed reload.

## Freshness Rules

Nudges follow the same freshness rules as content views:

- **Consumed:** User tapped a CTA → soft-deleted immediately.
- **Age expiry:** Older than 24 hours since creation → soft-deleted (async) on the next `GetFeed`/inbox load via `pruneExpiredNudges`.

Soft-deleted nudges are excluded from all active queries but the records remain in the database for engagement analysis.

## Stock Imagery

Background imagery uses the copy-on-use pattern described in [stock_imagery.md](stock_imagery.md). The terminator's `stock_query` comes from its pool entry. Host prompts skip stock imagery entirely and wear the referenced event's own photo (see *Host prompts on the inbox*). On the client, imagery is always displayed via `CachedMediaImage` (never `Image.network` or `Image.file`) using the nudge's `media_id` as the stable cache key.

## Trade-offs

- **Fire-and-forget consumption:** `ConsumeNudge` RPC failures are silently ignored. A failed consume means the card may reappear on the next load. This is acceptable given the optimistic local dismiss.
- **Host prompts are the only personalized card left.** That is the point — a card should say something about what the viewer actually did. The cost is that a user with no momentum signal sees only the three Home affordances, which is the correct amount of prompting for someone the product has nothing specific to say to.

---

**Last Updated:** 2026-08-16
**Status:** Production-ready (terminator pool + momentum host prompts; generic AI nudges removed by #2936)
