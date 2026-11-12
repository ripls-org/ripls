# Feed Redesign v2

## Problem Statement

The community feed (`FeedScreen`) is the primary surface where users discover
what's happening in their circles. It currently has four interconnected
problems:

### Problem 1: No lifecycle activity

The feed shows when things are *created* but not when they *change*. A gear
item shared last week that just had its recipient selected, or a request that
just got three new offers, looks identical to a stale item. The feed misses
comments, state changes, RSVPs, offers, and detail updates — even though many
of these already have `CommunityEventType` enum values that the feed's
`eventToFeedItem()` silently drops.

### Problem 2: No freshness management

The feed grows unboundedly over time, showing views all the way back to the
beginning. There is no concept of "you've already seen this and nothing has
changed." Users scroll through pages of stale items they've already reviewed
to find anything new. The feed should be "here's the latest you need to
know" — items seen and unchanged should expire after a short grace period.
Users can always find old items via the Discover screen search.

### Problem 3: Stories are one-dimensional

When a loan, giveaway, experience, or request completes, the feed shows a
story — but every story uses the same single layout (`StoryContentView`).
Stories are a key moment of community celebration and should feel varied,
editorial, and worth lingering on. We need multiple story card formats so we
can test which presentations resonate most with users.

### Problem 4: No system-generated inspiration

The feed is entirely reactive — it only shows things that have already
happened. There is no mechanism for the system to proactively inspire users
to take action: plan an experience, share gear, ask for help, or simply get
outside. The feed needs **nudge cards** — contextual prompts interspersed
among community activity that encourage the user to do something.

### Desired behavior

The feed should be a curated, fresh stream of three types of content:

1. **Content Views** — gear, experience, request, and community cards showing
   items with unseen activity, ordered by most recent change. Items that have
   been seen and are unchanged expire after a short grace period.
2. **Stories** — completed items (loans returned, giveaways picked up,
   experiences concluded, requests fulfilled) rendered in varied editorial
   layouts. The system randomly selects from 10 card formats per story.
3. **Nudges** — system-generated prompts that appear every ~8 items, with one
   always at the end of the feed as a terminator. Nudges have background
   imagery from stock providers and CTAs that route to creation flows.

## Current Architecture

### How the feed works today

1. `CommunityEvent` records are created by services (community, transfer,
   request, experience) via `RecordCommunityEventAndNotify()`
2. `FeedService.GetFeed()` queries all `CommunityEvent`s for a community,
   batch-fetches related data, and converts each event to a `FeedItem` via
   `eventToFeedItem()`
3. Only 4 event types are converted: `GEAR_SHARED`, `REQUEST_CREATED`,
   `EXPERIENCE_CREATED`, `COMMUNITY_CREATED`. Plus `Story` items. All other
   event types return nil and are skipped
4. Items are sorted by `occurred_at_unix_sec` descending and paginated
5. `FeedItemView` tracks per-user view counts (how many times viewed) but not
   "last seen at" relative to item changes

### What CommunityEvent types exist but aren't surfaced

```
TRANSFER_INTEREST_EXPRESSED    TRANSFER_RECIPIENT_SELECTED
TRANSFER_ACTIVE                TRANSFER_COMPLETED
TRANSFER_CANCELLED             TRANSFER_INTEREST_WITHDRAWN
REQUEST_OFFER_MADE             REQUEST_OFFER_SELECTED
REQUEST_FULFILLED              REQUEST_CANCELLED
REQUEST_OFFER_WITHDRAWN        EXPERIENCE_RSVP_YES
EXPERIENCE_RSVP_MAYBE          EXPERIENCE_RSVP_NO
GEAR_UNSHARED                  MEMBER_LEFT
INVITATION_LINK_USED
```

### What doesn't generate CommunityEvents at all

- **Chat messages / comments** — these go through the chat system
  (`ChatService.SendMessage`) and are not recorded as community events

### Current content types and their rendering

The feed currently has two categories of card, both rendered full-screen in a
vertical `PageView`:

- **Content Views** — `GearContentView`, `ExperienceContentView`,
  `RequestContentView`, `CommunityContentView`. Each uses a full-bleed
  background image via `ContentViewBuilders.buildMediaBackground()` with a
  gradient overlay and content positioned over it.
- **Stories** — `StoryContentView`. A single layout: background media with
  60% black overlay, title, description, and participant list. Every story
  looks identical regardless of type or significance.

All content uses the shared `MediaBackground` widget which supports both
images (`CachedNetworkImage` with stable media-ID cache keys) and video
(`VideoPlayerController`).

## Approach: Hybrid activity feed with heterogeneous card types

The redesigned feed combines the Approach D hybrid activity model (from the
original analysis) with two new content types — varied story cards and nudge
cards — creating a feed with three distinct categories of content that each
serve a different purpose.

### Content Views: activity-ordered, deduplicated, expiring

Content views (gear, experience, request, community) use the hybrid activity
model: `CommunityEvent`s provide structural change signals, and
`ChatConversation.last_message_at_unix_sec` provides comment activity —
without creating a `CommunityEvent` per comment.

**Activity computation:**
- `last_activity_at = max(most_recent_event.occurred_at, conversation.last_message_at)`
  per item, computed at feed generation time from data already fetched
- Items are deduplicated: each item appears at most once, showing the most
  recent event
- Items are sorted by `last_activity_at` descending

**Seen tracking:**
- `FeedItemView` gains `last_seen_at_unix_sec` — updated when the user views
  an item via `MarkFeedItemsViewed`
- An item is "unseen" if `last_activity_at > last_seen_at`

**Freshness expiry:**
- Items that have been seen and have not changed should not persist
  indefinitely. After a grace period, they are excluded:
  - Exclude when `last_seen_at >= last_activity_at` AND
    (`views_since_last_activity >= 2` OR `now - last_seen_at > 24 hours`)
  - `views_since_last_activity` is a new counter on `FeedItemView`,
    incremented on each view when the user has already seen the current
    state, reset to 0 when new activity occurs
- Items with new activity always resurface regardless of prior history
- Users find expired items via Discover screen search

**Activity context on feed items:**
- Each `FeedItem` carries `unseen_comment_count` and the most recent event
  type, rendered as a badge ("3 new comments", "Recipient selected")

### Stories: varied editorial layouts for completed items

When items reach terminal state (loan completed, giveaway picked up,
experience concluded, request fulfilled), they appear in the feed as story
cards. Stories follow the same seen-tracking and expiry rules as content
views, with a 7-day hard cap as a backstop.

Story *creation* (title, description, participants, media) is unchanged —
the `story.StoryCreator` library (`server/story/`) continues to generate
story records via AI or template fallback, triggered synchronously from
community events (see `docs/stories.md`). The feed service converts stored
`Story` models to `FeedItem`s with `StoryPayload` in `createStoryItem()`.

The key change is that stories are no longer a single *layout*. The system
supports 10 distinct card formats, each with its own visual personality.
The variant is assigned deterministically per story (`hash(story_id) % 10 + 1`)
at feed generation time (a presentation concern, not a storage concern).
All variants receive the same `StoryPayload` data and use the same
`ContentViewBuilders.buildMediaBackground()` for background images.

**Story card layouts** (reference: `tmp/feed/stories.jsx`):

1. **The Magazine Spread** — big serif headline over full-bleed photo.
   Editorial italic prose. Glass location pill at top. Avatar stack + date
   at bottom. Gradient: bottom-heavy (0.88 → 0.3 → transparent). ~580px.

2. **The Photo Journal** — dark card (#0e0e0e). Avatar + name header.
   Horizontal photo carousel (item media, up to 3 images, 260px,
   scroll-snap with dot indicators). Long-form narrative. Individual
   avatar + first-name chips at bottom.

3. **The Headline Card** — vertically centered over full background. Avatar
   stack, large serif headline, subtitle, glass location pill + date at
   bottom. ~520px.

4. **The Postcard** — horizontal 42/58 split. Left: photo with right-edge
   fade to #111. Right: two avatars with arrow (giver → receiver), serif
   headline, description, date divider. ~420px. Requires exactly 2
   participants; falls back to variant 1 otherwise.

5. **The Ticker Tape** — full background, headline + description, then
   horizontal scrolling stat ribbon (bordered band, `rgba(0,0,0,0.3)`):
   4 value/label pairs. Avatar + attribution below. ~480px.

6. **The Dispatch** — newspaper style. All-caps dateline at top ("Help
   Request · Fulfilled"). Large headline, italic prose, avatar stack +
   date at bottom. ~480px.

7. **The Pull Quote** — giant hero stat (72px, e.g. "$1,240") vertically
   centered. All-caps subtitle, paragraph, avatar stack above. Radial
   gradient highlight. ~500px. Best for milestone/aggregate stories; falls
   back to variant 3 if no aggregate stats.

8. **The Scrapbook** — dark card (#0c0c0c), no full-bleed photo. Three
   overlapping rotated photos (±2-3°, card-color borders, drop shadows)
   in 240px collage. Glass "3 photos" badge. Headline, description,
   avatar stack below.

9. **The Field Guide** — place-focused. Large serif place name at top with
   all-caps region. Glass stat pills (grade, duration, group size).
   Description, avatar stack at bottom. ~540px.

10. **The Golden Hour** — dreamy, nostalgic. Floating italic timestamp
    top-right. Multi-line serif headline, italic narrative, muted secondary
    paragraph. Two avatar + name + role chips at bottom. ~540px.

**Variant assignment:**
- `story_variant = (FNV-1a(story_id) % 10) + 1`
- Data compatibility fallbacks: variant 4 requires exactly 2 participants
  (→ 1 if not), variant 7 requires `StoryPayload.impact` data (→ 3 if
  empty)
- New field: `int32 story_variant` on `FeedItem`

**Theming note:** Story and nudge cards render over full-bleed photos with
dark overlays, so text is always white-on-dark regardless of app theme.
Overlay colors (`rgba(0,0,0,0.4)`, `rgba(255,255,255,0.7)` for text, etc.)
are intrinsic to the card design and do not vary by theme — this matches the
existing `StoryContentView` and all content view patterns. These are not
`AppColors` violations.

### Nudges: system-generated inspiration cards

Nudges are a new feed item type — system prompts that inspire the user to
take action. They are not tied to a specific community event. They are
generated server-side based on user behavior, circle activity, and context.

**Feed placement:**
- Inserted every ~8 content items in the final feed
- One nudge always placed at the end as a feed terminator ("You're all
  caught up — close the app and go outside")
- Tracked for seen expiry using the same `FeedItemView` mechanism as content
  views. Nudge IDs are deterministic (`nudge:{userID}:{position}`), so
  `MarkFeedItemsViewed` and `shouldIncludeItem()` freshness rules apply
  naturally — a nudge viewed twice or unchanged for 24 hours expires, and a
  new one is generated on the next feed load

**Background imagery:**
- All nudge cards have a background image or video sourced from the stock
  imagery system (`server/media/`). The same
  `ContentViewBuilders.buildMediaBackground()` pattern is used on the client.
- The feed service calls `StockImageryProvider.GetStockImage()` (or
  `StockVideoProvider.GetStockVideo()`) with a context-appropriate query
  derived from the nudge content. The existing three-layer architecture
  (semantic cache → Pexels → Unsplash/Pixabay fallback) handles caching
  and rate limiting automatically — most queries will hit the semantic
  cache after the first fetch, so no separate pre-fetching mechanism is
  needed.
- **Copy-on-use:** Per `docs/stock_imagery.md`, nudge media must follow
  the copy-on-use pattern. The feed service creates a media copy of the
  stock image (via `storage.StoreMedia()` with `sourceStockImageID` set)
  rather than referencing the canonical stock image media ID directly.
  This enables proper attribution tracking and cleanup.

**Nudge card layouts** (reference: `tmp/feed/nudges.jsx`):

1. **The Magazine Spread** — aspirational editorial. Glass "20 min from
   you" pill. Big serif headline ("Good things happen when you get your
   friends outside."). Italic prose referencing recent activity. Primary
   CTA + avatar stack. ~540px.

2. **The Photo Journal** — dark card. Star-icon header ("Your gear, their
   next adventure"). Horizontal scroll of 3 gear suggestion cards
   (160×200px, background + gradient + emoji + name + demand signal).
   Narrative about circle demand. Primary CTA.

3. **The Headline Card** — centered poster, minimal. "You're all caught
   up." Short paragraph encouraging the user to close the app. Single CTA.
   Used as feed terminator. ~480px.

4. **The Postcard** — 42/58 split. Left: aspirational photo with glass
   weather pill. Right: all-caps category, serif headline about
   conditions, description with friend's gear availability, avatar + note,
   CTA. ~400px.

5. **The Ticker Tape** — celebrates a friend's generosity. Headline naming
   the friend. Stat ribbon (items shared, $ saved, people helped,
   ranking). Two CTAs: primary "List an item" + ghost "Offer time". ~460px.

6. **The Open Door** — warm nudge to ask for help. Serif headline ("Need a
   hand with something?"). Italic description. Glass card with circle
   avatars + "Tools, time, skills, and rides". Primary CTA "Ask for Help."

7. **The Pull Quote** — giant stat (80px, e.g. "21"). All-caps subtitle
   ("days since your last adventure"). Paragraph about circle activity
   the user has missed. CTA "Change that." ~480px.

8. **The Scrapbook** — overlapping memory cards from past experiences. Dark
   card. Three rotated photos with glass date labels. "Remember these?"
   headline. CTA + avatar stack.

9. **The Field Guide** — place-focused. All-caps "Suggested destination."
   Large serif place name. Glass stat pills (distance, elevation, time,
   weather). Description referencing a friend's recent visit. CTA + avatar.
   ~520px.

10. **The Golden Hour** — dreamy, poetic. Italic timestamp top-right.
    Multi-line serif headline. Italic description naming friends. Avatar +
    name + role chips. CTA "Plan an Experience." ~520px.

**CTA actions:**
- Each nudge carries a `cta_action` identifier that the client maps to
  navigation: `plan_experience` → `UnifiedCreationModal` (experience
  intent), `list_item` → gear intent, `ask_for_help` → request intent,
  `check_listings` → user's gear list

**Variant selection:**
- `variant = (hash(userID + date_string) % 10) + 1` per position, avoiding
  repeats within a single feed load

**Server generation** (`server/services/feed/nudges.go`):
- `generateNudges()` analyzes user context to select relevant nudge content:
  - Days since last activity (Pull Quote variant 7)
  - Circle member sharing stats (Ticker Tape variant 5)
  - Recent experiences for memory prompts (Scrapbook variant 8)
  - Available gear demand (Photo Journal variant 2)
  - Weather/conditions if available (Field Guide variant 9)
  - General inspiration (Magazine Spread, Golden Hour, etc.)
- Background media fetched via `StockImageryProvider` / `StockVideoProvider`
  interfaces (from `server/media/`). The existing semantic cache layer means
  repeated nudge queries for similar themes ("hiking trail", "camping
  outdoors") will cache-hit without external API calls
- The feed service receives these provider interfaces via constructor
  injection (per server architecture Pattern 2: Interface-Based
  Dependencies). Providers are optional — nudges degrade gracefully with
  no background if providers are nil

**Nudge text localization:**
- Nudge `headline`, `description`, `cta_label`, and `secondary_cta_label`
  are server-generated strings. The server must produce localized text based
  on the user's locale preference (available from the user record). This
  keeps the client simple — it renders whatever text the server sends.
- CTA action identifiers (`cta_action`) are not localized — they are
  machine-readable routing keys.

**Proto changes:**
- `FEED_ITEM_TYPE_NUDGE` added to `FeedItemType` enum
- New messages:
  ```
  message NudgePayload {
    int32 nudge_variant = 1;
    string headline = 2;
    string description = 3;
    string cta_label = 4;
    string cta_action = 5;
    repeated string media_ids = 6;
    repeated string participant_ids = 7;
    optional string secondary_cta_label = 8;
    optional string secondary_cta_action = 9;
    optional string location_hint = 10;
    repeated NudgeStat stats = 11;
  }

  message NudgeStat {
    string value = 1;
    string label = 2;
  }
  ```
- `NudgePayload nudge` added to `FeedItem.payload` oneof

## Implementation Plan

### Phase 1: Server — seen tracking + lifecycle events + freshness

**Goal:** Track when users last saw each item, surface lifecycle events, and
expire stale items.

1. Add `int64 last_seen_at_unix_sec = 9` to `FeedItemView` in
   `proto/ripls/models/feed.proto`
2. Add `int32 views_since_last_activity = 10` to `FeedItemView`
3. Update `FeedStorage.IncrementViewCount` to set `last_seen_at_unix_sec = now`
   and increment `views_since_last_activity` when `last_seen_at >= last_activity_at`,
   reset counter to 0 otherwise
4. Update `FeedStorage.GetViewCountsForItems` to return both timestamps and
   view-since-activity count
5. Add `eventToFeedItem()` handlers for lifecycle event types:
   `TRANSFER_INTEREST_EXPRESSED`, `TRANSFER_RECIPIENT_SELECTED`,
   `TRANSFER_ACTIVE`, `TRANSFER_COMPLETED`, `TRANSFER_CANCELLED`,
   `REQUEST_OFFER_MADE`, `REQUEST_FULFILLED`, `REQUEST_CANCELLED`,
   `EXPERIENCE_RSVP_YES`, `EXPERIENCE_RSVP_MAYBE`, `EXPERIENCE_RSVP_NO`
6. Add freshness filter in `generateFeedItems()`: exclude items where
   `last_seen_at >= last_activity_at` AND (`views_since_last_activity >= 2`
   OR `now - last_seen_at > 24 hours`)
7. Add unit tests for seen tracking, lifecycle handlers, and freshness expiry

### Phase 2: Server — per-item deduplication + conversation activity

**Goal:** Each item appears at most once, ordered by most recent activity
across events and comments.

1. In `generateFeedItems()`, group events by item ID, keep only the most
   recent event per item
2. Batch-fetch `ChatConversation` records for all items. Extract
   `last_message_at_unix_sec` per item
3. Compute `last_activity_at = max(event.occurred_at, conversation.last_message_at)`
   per item. Sort by `last_activity_at` descending
4. Filter: exclude items where `last_activity_at <= user's last_seen_at`
5. Add `int64 last_activity_at_unix_sec` and `int32 unseen_comment_count` to
   `FeedItem` in `proto/ripls/api/feed_service.proto`. Populate comment count
   via batch query (not per-item loop)
6. Terminal items (completed/cancelled) are included as stories with
   `is_story = true`, with 7-day hard cap
7. Add unit tests for deduplication, conversation merging, and story filtering

### Phase 3: Server — story variants + nudge generation

**Goal:** Assign varied layouts to stories and generate nudge cards.

1. Add `int32 story_variant = 18` to `FeedItem` in
   `proto/ripls/api/feed_service.proto`
2. In `createStoryItem()`, assign `story_variant = (FNV-1a(story_id) % 10) + 1`
   with data-compatibility fallbacks (variant 4 needs 2 participants → 1,
   variant 7 needs stats → 3)
3. Add `FEED_ITEM_TYPE_NUDGE` to `FeedItemType`, add `NudgePayload`,
   `NudgeStat` messages, and `NudgePayload nudge` to `FeedItem.payload` oneof
4. Add `StockImageryProvider` (from `server/media/`) as optional constructor
   dependency on the feed `Service` struct (interface-based injection per
   server architecture Pattern 2). Nudge generation degrades gracefully
   (no background) if provider is nil
5. New file: `server/services/feed/nudges.go` — `generateNudges()` that
   analyzes user context and produces nudge items. Background media fetched
   via `StockImageryProvider.GetStockImage()` with the copy-on-use pattern
   (create media copy, set `source_stock_image_id` for provenance). The
   semantic cache layer in `server/media/` handles deduplication across
   repeated nudge queries
6. Nudge text must be generated in the user's locale (read from user record).
   Use a simple template approach — no i18n framework needed server-side,
   just locale-keyed string maps for headlines, descriptions, and CTA labels
7. Insert nudges at positions `[8, 16, 24, ...]` plus a variant-3 terminator
   at the end
8. Variant selection: `hash(userID + date) % 10 + 1` per position, no
   repeats within one feed load
9. Add unit tests for story variant assignment, nudge placement, nudge
   content generation, and locale-aware text

### Phase 4: Client — activity-ordered feed with badges

**Goal:** Render the deduplicated, activity-ordered feed with unseen
indicators.

1. Update `feed_view_model.dart`: server returns pre-filtered, pre-ordered
   items. Client trusts server order. `FeedState` works as-is
2. Update `feed_screen.dart` item rendering:
   - Activity badges on content views ("3 new comments", "Recipient
     selected") using `unseen_comment_count` and event type
   - Badge text resolved via `context.l10n` in the widget, not in the
     ViewModel (per i18n best practices: ViewModels expose typed state,
     widgets resolve to localized text)
   - Use `AppColors.*` for badge styling (badges render over the app
     chrome, not over photos, so they must be theme-aware)
3. Add i18n keys for activity badges to both `app_en.arb` and `app_es.arb`:
   - `feedNewComments` (plural: `{count, plural, =1{1 new comment} other{{count} new comments}}`)
   - `feedRecipientSelected`, `feedOfferMade`, `feedRsvpAdded`, etc.
   - Key prefix: `feed*` per i18n naming convention
4. Seen tracking: `markCurrentItemViewed()` already calls
   `MarkFeedItemsViewed`, which now updates `last_seen_at_unix_sec`
5. Add widget tests for activity badges

### Phase 5: Client — shared card sub-widgets + story variants

**Goal:** Build shared UI components, then render 10 story card layouts.

1. New file: `app/lib/presentation/widgets/content/card_components.dart`
   — shared sub-widgets used by both story and nudge card variants:
   - `GlassPill` — `ClipRRect` + `BackdropFilter(blur: 20)` + dark
     semi-transparent background + thin white border
   - `AvatarStack` — overlapping `CircleAvatar` with negative margin
   - `StatRibbon` — horizontal scrollable value/label pairs in bordered band
   - `PhotoCarousel` — horizontal `PageView` with dot indicators
   - `PhotoCollage` — 3 rotated `Transform.rotate` containers
   These are public widgets (not `_`-prefixed) because they are shared
   across story and nudge variant files
2. New file: `app/lib/presentation/widgets/story/story_card_variants.dart`
   — 10 builder methods using the shared components, one per layout
3. All variants use `ContentViewBuilders.buildMediaBackground()` for
   backgrounds. Media loading goes through `MediaRepository` via the
   existing `StoryViewModel` (no direct service access from widgets)
4. `StoryContentView` dispatches to variant builder based on `story_variant`
   from parent `FeedItem`. Fallback to variant 1 if 0 or unrecognized
5. Add widget tests for each variant and for shared components

### Phase 6: Client — nudge cards

**Goal:** Render nudge cards with CTAs in the feed.

1. New file: `app/lib/presentation/widgets/nudge/nudge_card_variants.dart`
   — 10 layout builders using shared `card_components.dart` widgets
2. New file: `app/lib/presentation/widgets/nudge/nudge_content_view.dart`
   — `NudgeContentView` ConsumerWidget that dispatches to variant builder.
   Located in `widgets/` (not `screens/`) because it is a card component
   rendered inside `FeedScreen`, not a standalone navigable screen
3. Background images via `ContentViewBuilders.buildMediaBackground()`.
   Media URLs fetched through `MediaRepository` (repository pattern —
   widgets never call services directly)
4. CTA buttons: primary uses `AppColors.coral` (already a semantic color).
   Ghost secondary buttons use overlay-appropriate semi-transparent
   styling consistent with existing content view patterns
5. CTA navigation: use `NavigationHelpers` for all navigation (per
   architecture). Map `cta_action` values:
   - `plan_experience` → `UnifiedCreationModal` (experience)
   - `list_item` → `UnifiedCreationModal` (gear)
   - `ask_for_help` → `UnifiedCreationModal` (request)
   - `check_listings` → user gear list
6. Nudge text (headline, description, CTA labels) comes pre-localized
   from the server. The client renders them directly — no `context.l10n`
   lookups needed for nudge content (unlike activity badges which are
   client-resolved)
7. `feed_screen.dart` adds `FEED_ITEM_TYPE_NUDGE` → `NudgeContentView`
8. Nudges skip seen-expiry tracking
9. Add widget tests for nudge variants and CTA routing

### Phase 7: Testing and polish

1. Server integration tests: deduplication, conversation merging, seen
   filtering end-to-end
2. Server tests: freshness expiry (items expire after 2 views or 24 hours)
3. Server tests: story variant assignment determinism and fallbacks
4. Server tests: nudge placement at every 8th position + terminator
5. Server tests: nudge text generation in both en and es locales
6. Client tests: feed renders in server-provided order
7. Client tests: all 10 story variants render without errors
8. Client tests: all 10 nudge variants render without errors
9. Performance: verify with `storage.AssertMaxQueries` that query count
   stays bounded after adding conversation fetch
10. Edge cases: items with no conversation, items with conversation-only
    activity, variant fallback paths, nil stock media providers
11. Pagination: deduplication changes effective page size. Fetch more events
    than `pageSize` and trim after dedup, or switch to cursor-based
    pagination keyed on `last_activity_at`

### Phase 8: Nudge background imagery + testing flag

#### Problem 1: Nudge cards have no background imagery

Nudge cards are designed as full-bleed photo cards (Magazine Spread, Headline
Card, Golden Hour, etc.), but currently render with solid dark backgrounds.
The original implementation attempted to fetch stock imagery on-demand during
`GetFeed`, which caused two failures:

- **Blocking latency:** `GetStockImage()` makes network calls to Pexels/
  Unsplash that take 1-5 seconds per image. Generating 3 nudges would add
  3-15 seconds to every feed load.
- **Broken media records:** `copyStockMedia()` created `Media` DB records
  pointing to `StorageUrl` paths, but never copied the actual image bytes to
  bucket storage. The client then tried to fetch presigned URLs for
  nonexistent files, producing `file not found` errors and black screens.

**Key constraints:**
- Nudges are personalized per-user based on their behavior (days since last
  activity, circle stats, recent experiences, gear demand). They are
  continuously generated as users interact with the app.
- A nudge must **never** be shown to a user without a background image. A
  black screen with white text is a broken experience. Every solution must
  guarantee imagery is present *before* the nudge enters the feed response.
- Background images must be **contextually matched** to the personalized
  nudge content, not a generic image per variant template. A nudge about
  Sarah's sharing stats needs a different image than one about hiking
  conditions near you.
- **Used = dismissed:** When a user taps a nudge's CTA (e.g., "Plan an
  Experience"), the nudge is **consumed** and must never appear in the feed
  again. The CTA action was taken — the nudge served its purpose.
- **Seen expiry:** If a nudge is seen but no action is taken, it expires
  after a grace period (e.g., seen twice or 24 hours with no tap — same
  freshness rules as content views). The user should not be nagged by the
  same nudge indefinitely.
- **Records are kept for analytics:** Expired and consumed nudges are
  soft-deleted, not hard-deleted. The `StoredNudge` record retains the
  nudge content, variant, CTA action, whether it was consumed or expired,
  and timestamps. This enables effectiveness tracking: which nudge variants
  drive the most CTA taps, which expire unseen, etc.
- **Feed terminator:** The last item in the feed is always a "go outside"
  nudge. This terminator must also have contextually matched imagery and
  varied content — the user should not see the same "You're all caught up"
  card every time. The terminator is a special nudge type: it is always
  generated fresh (not reused from stored nudges), uses variant 3's layout
  but with rotating content (different headline/description each time),
  and is not subject to seen-expiry (it regenerates on every feed load).

**How other creation flows solve this:** Experience, request, and gear
creation all follow the same pattern: (1) generate descriptive content
(title, description, keywords), (2) derive a stock image search query from
that content, (3) fetch a stock image matched to the query, (4) copy the
image for the user via `CopyStockImageForUser`, (5) attach the media ID.
The request service does step 3-5 *asynchronously* in a background
goroutine after creation so the RPC returns immediately — the request
appears without media at first, then the image shows up shortly after.

Nudges should follow this same create-then-enrich pattern, but with one
critical difference: a nudge with no image must be **invisible** (not shown
in the feed), whereas a request with no image is still usable.

**Option A: Stored nudge records with async imagery; invisible until
complete**
Nudges become first-class stored records (`StoredNudge` table), created
ahead of display. The creation flow mirrors request creation:
(1) `generateNudges()` produces personalized text + derives search keywords
from the nudge content, (2) persists `StoredNudge` records (no media yet),
(3) launches async goroutines per nudge to fetch stock imagery via the
semantic cache / provider chain and `CopyStockImageForUser()`,
(4) updates each `StoredNudge.media_id` when the fetch completes.

`GetFeed` only includes stored nudges where `media_id` is populated —
incomplete nudges are invisible. Creation is triggered during `GetFeed`
when the user has no unexpired, unseen nudges. The nudges appear on the
*next* feed load (or scroll refresh) once imagery arrives (typically seconds
via semantic cache).

- Pro: Exact same pattern as request creation — proven, understood. No user
  ever sees a nudge without imagery. Background images are contextually
  matched to personalized content. Stored records enable seen-tracking and
  expiry via existing `FeedItemView` mechanism. The semantic cache means
  most queries resolve from DB, not external APIs.
- Con: Nudges never appear on the very first feed load for a new user (they
  are created then, imagery fetched async, visible on next load). Adds a
  new storage table and async write path. Need to manage nudge lifecycle
  (creation triggers, expiry cleanup).

**Option B: Generate nudges synchronously during `GetFeed` with stock
imagery from semantic cache only**
During `GetFeed`, generate personalized nudge content, derive search
keywords, and query the semantic cache (DB only — no external API calls).
If the semantic cache has a match (similarity ≥ 0.85), use it immediately.
If not, skip that nudge entirely (suppress, don't degrade) and enqueue an
async stock provider fetch to warm the cache for next time.

- Pro: No new storage table for nudges themselves. Contextual imagery from
  cache is fast (DB query, ~10ms). In steady state most queries hit the
  semantic cache. No user sees a nudge without imagery.
- Con: Nudges are unavailable until the semantic cache warms for their
  specific queries. Cold cache means zero nudges until background fetches
  populate it. No stored records means nudge seen-tracking relies on
  deterministic IDs (fragile if nudge content changes between loads).
  Nudge generation adds DB queries to every `GetFeed` call.

**Option C: Event-driven nudge creation on user activity**
Instead of generating nudges at feed time, create them when relevant events
occur: user completes a loan → create a "celebrate your circle" nudge;
user inactive for 7 days → a daily cron creates a "come back" nudge. Each
creation triggers async imagery fetch immediately. `GetFeed` simply queries
`StoredNudge` records with `media_id IS NOT NULL` and includes them.

- Pro: Feed generation has zero nudge-related computation — it's a DB query.
  Imagery is always present by the time the user opens the feed. Nudges are
  truly personalized to specific events, not just user state at feed time.
  Same lifecycle as stories: stored, tracked, expired.
- Con: Most complex option. Requires identifying all trigger points and
  creating nudges from multiple services (transfer, experience, request,
  cron). Risk of over-generating nudges. Event-driven creation adds
  coupling between services and the nudge system.

**Recommendation: Option A (stored records with async imagery, invisible
until complete).**
This follows the exact pattern already proven by request creation: create
the record, fetch imagery asynchronously, and only show the item once it's
complete. The trade-off — nudges don't appear on the very first feed load
after creation — is acceptable because the imagery fetch takes seconds via
the semantic cache, and the user sees their regular content in the meantime.

Stored records also naturally solve the lifecycle requirements: consumed
nudges are soft-deleted with a reason, expired nudges are soft-deleted by
the freshness rules, and all records are retained for analytics.

Implementation sketch:

**Model:**
1. New model: `StoredNudge` proto in `proto/ripls/models/` with fields:
   - `id`, `user_id`, `community_id`
   - `nudge_variant` (int32), `headline`, `description`
   - `cta_label`, `cta_action`
   - `secondary_cta_label`, `secondary_cta_action`, `location_hint`
   - `media_id` (empty until imagery fetch completes)
   - `participant_ids` (repeated string)
   - `stats` (repeated `StoredNudgeStat` with value/label)
   - `stock_query` (derived search query for imagery)
   - `created_at_unix_sec`
   - `is_terminator` (bool — true for "go outside" feed-end nudges)
   - `consumed_at_unix_sec` (optional — set when user taps a CTA)
   - `consumed_action` (optional — which CTA was tapped, for analytics)
   - `deleted` (optional `DeletedMetadata` — soft-delete for expiry)

**Storage:**
2. New storage: `NudgeStorage` with methods:
   - `Insert(ctx, nudge)` — create a new nudge record.
   - `GetActiveForUser(ctx, userID, communityID)` — returns nudges where:
     `media_id IS NOT NULL` AND `consumed_at_unix_sec IS NULL` AND
     `deleted IS NULL`. Ordered by `created_at_unix_sec DESC`.
   - `UpdateMediaID(ctx, nudgeID, mediaID)` — set media after async fetch.
   - `MarkConsumed(ctx, nudgeID, action)` — sets `consumed_at_unix_sec`
     and `consumed_action`. The nudge is no longer returned by
     `GetActiveForUser` but the record is preserved for analytics.
   - `SoftDelete(ctx, nudgeID)` — marks as deleted (expired). Record
     retained for analytics.
   - `GetAnalytics(ctx, communityID, since)` — returns all nudges
     (including consumed and deleted) for effectiveness reporting:
     conversion rate per variant, per CTA action, time-to-consume, etc.

**Feed generation:**
3. In `GetFeed`: query `StoredNudge` for the user via `GetActiveForUser`
   (1 batch query). Apply `shouldIncludeItem()` freshness rules using
   `FeedItemView` data (same as content views — excludes nudges seen twice
   or not interacted with for 24 hours). Nudges that fail freshness are
   soft-deleted (expired) so they are preserved for analytics.

   If the active nudge count is below the needed count (fewer than
   `feedSize / nudgeInterval`), call `generateAndPersistNudges()` to
   create new ones:
   (a) generates personalized nudge content using user context,
   (b) derives a stock query from each nudge's headline + description,
   (c) persists `StoredNudge` records (no media yet),
   (d) launches async goroutines to fetch imagery per nudge.
   New nudges are invisible on this call (no media yet) and appear on
   the next feed load once imagery arrives (typically seconds via
   semantic cache).

**Terminator nudge:**
4. The feed terminator ("go outside") is a special `StoredNudge` with
   `is_terminator = true`. It is always appended as the last feed item.
   Terminators are **not** subject to seen-expiry — instead, a new
   terminator is generated on each feed load with varied content
   (different headline/description drawn from a rotating pool or
   derived from the user's recent activity). The previous terminator is
   soft-deleted when the new one is created. This ensures the user never
   sees the same "go outside" card twice. Terminators use variant 3's
   layout but are not limited to a single headline.

**Async imagery:**
5. For each new `StoredNudge` without media, a goroutine (with semaphore
   for concurrency control, like request creation) runs:
   derive query from nudge content → `GetStockImage(ctx, query, nil)` →
   `CopyStockImageForUser()` → `NudgeStorage.UpdateMediaID()`.

**CTA consumption:**
6. New RPC: `ConsumeNudge(nudge_id, action)` — called by the client when
   the user taps a CTA. Sets `consumed_at_unix_sec` and `consumed_action`
   on the `StoredNudge` record. The nudge disappears from subsequent feed
   loads. The client navigates to the CTA target (creation modal, etc.)
   after calling this RPC.

**Expiry and cleanup:**
7. Nudges that fail `shouldIncludeItem()` freshness (seen ≥ 2 times with
   no CTA tap, or last seen > 24 hours ago with no activity) are
   soft-deleted with `deleted.reason = "expired"`. A periodic cleanup job
   hard-deletes records older than 90 days (analytics retention window).

**Analytics:**
8. All nudge records are retained (soft-deleted, not hard-deleted) for
   effectiveness tracking. Key metrics derivable from the stored data:
   - **Conversion rate**: `consumed_at IS NOT NULL / total` per variant
   - **CTA breakdown**: which `consumed_action` values are most common
   - **Time to convert**: `consumed_at - created_at` distribution
   - **Expiry rate**: `deleted.reason = "expired" / total` per variant
   - **Impression count**: `FeedItemView.view_count` per nudge ID

#### Problem 2: No way to test nudges without scrolling past 8+ content items

During development and QA, nudges only appear after every 8 content items
plus a terminator at the end. This makes it hard to visually test nudge
rendering, CTA routing, and variant layouts — you have to scroll through the
entire feed. A testing flag that forces nudges to appear first would
dramatically speed up iteration.

**Option A: Server-side `debug_nudge_count` field on `GetFeedRequest`**
Add an `optional int32 debug_nudge_count` field to `GetFeedRequest`. When
set to N > 0, the server prepends N nudge items at the top of the feed
(before any content items). The field is ignored in production (gated behind
environment check or always available — nudges are harmless).

- Pro: Works immediately — server controls nudge injection. No client-side
  state to manage. Any client (Flutter, curl, tests) can use it.
- Con: Adds a debug field to the public API proto. Could be misused.

**Option B: Client-side env var / dart-define flag**
Add `NUDGE_TEST_COUNT` to `env.dev.json` and `Environment` class. The feed
view model reads it and, if set, moves nudge items to the front of the
client-side item list before rendering.

- Pro: No server changes. Client-only. Easy to toggle per environment.
- Con: Requires client recompilation to change. Doesn't help with server-side
  testing or curl-based debugging. Feed ordering manipulation in the view
  model is fragile.

**Option C: Server-side request field + client passes from env var**
Combine A and B: add `debug_nudge_count` to the proto, and have the client
read it from `Environment` and pass it in the request. Best of both worlds.

- Pro: Server controls injection (clean). Client toggles easily via env.
  Works from curl too. No client-side list manipulation.
- Con: Debug field in proto (acceptable for a request field, not stored).

**Recommendation: Option C (server field + client env var).**
The server already has the nudge generation logic; adding a prepend mode is
trivial. The client reads a `NUDGE_TEST_COUNT` env var and passes it through.
In production, the field defaults to 0 and has no effect. This approach is
the most flexible for testing: Flutter app, integration tests, and curl all
benefit.

Implementation sketch:
1. Add `optional int32 debug_nudge_count = 4` to `GetFeedRequest`.
2. In `GetFeed`, if `debug_nudge_count > 0`: generate that many nudges and
   prepend them before content items (instead of interleaving at every 8th
   position). Still append the terminator.
3. Add `NUDGE_TEST_COUNT` to `env.dev.json` (default: `"0"`).
4. Add `static int get nudgeTestCount` to `Environment`.
5. `FeedService.getFeed()` passes the value as `debugNudgeCount` in the
   request.
6. `env.prod.json` omits the field (defaults to 0).

## Audit Log

Changes made during architecture compliance review against
`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`,
`docs/stock_imagery.md`, `docs/stories.md`:

1. **Shared sub-widgets extracted (architecture.md: reusable components).**
   `GlassPill`, `AvatarStack`, `StatRibbon`, `PhotoCarousel`, `PhotoCollage`
   were originally private (`_`-prefixed) inside `story_card_variants.dart`.
   Nudge cards need the same widgets. Moved to shared
   `widgets/content/card_components.dart` as public widgets to avoid
   duplication. Both story and nudge variant files import from there.

2. **Nudge content view location corrected (architecture.md: directory
   structure).** Was `screens/feed/nudge_content_view.dart`. Nudge cards
   are not standalone navigable screens — they are card components rendered
   inside `FeedScreen`. Moved to `widgets/nudge/nudge_content_view.dart`
   to match widget vs screen convention.

3. **Media access via repository enforced (architecture.md: rule 1+2,
   caching.md: rule 1).** Plan mentioned
   `ContentViewBuilders.buildMediaBackground()` but didn't specify how
   nudge media URLs are obtained. Clarified: all media access goes through
   `MediaRepository` via ViewModels. Widgets never call services directly.
   Story variants use existing `StoryViewModel` for media. Nudge media IDs
   are in `NudgePayload`; the client fetches URLs through
   `MediaRepository`.

4. **CTA navigation via NavigationHelpers (architecture.md: navigation
   rules).** CTA routing originally just said "maps to navigation."
   Clarified: use `NavigationHelpers` for all CTA navigation, never raw
   `Navigator.push(MaterialPageRoute(...))`.

5. **i18n: activity badges use context.l10n, nudge text is server-localized
   (i18n.md: ViewModels never resolve strings).** Activity badge text
   ("3 new comments") is client-side and must use `context.l10n` with ICU
   plural format. Nudge text (headline, description, CTA labels) is
   server-generated and arrives pre-localized — the client renders it
   directly. Added explicit i18n key examples with `feed*` prefix and
   plural format. Added server-side locale requirement for nudge text
   generation.

6. **Stock imagery interface name and location corrected
   (stock_imagery.md).** Plan originally referenced `StockImageProvider`
   in `server/ai/`. The correct interface is `StockImageryProvider` in
   `server/media/stock_imagery_provider.go`. Fixed all references.

7. **Pre-fetched pool replaced with existing semantic cache
   (stock_imagery.md: three-layer architecture).** Original plan proposed
   a custom pre-fetched stock media pool refreshed by background job.
   This duplicates the existing semantic cache layer in `server/media/`
   which already caches stock imagery by query similarity (threshold 0.85)
   with Pexels → Unsplash fallback. Replaced with direct use of
   `StockImageryProvider.GetStockImage()` — repeated nudge queries for
   similar themes will cache-hit automatically.

8. **Copy-on-use pattern added (stock_imagery.md: common mistake).**
   When the feed service obtains stock images for nudge backgrounds, it
   must create a media copy (via `storage.StoreMedia()` with
   `sourceStockImageID` set) rather than referencing the canonical stock
   image media ID directly. This enables attribution tracking and
   cascade deletion. Missing this is explicitly called out as a common
   mistake in the stock imagery docs.

9. **Story architecture clarified (stories.md: library-based).**
   Clarified that story *creation* (title, description, AI/template
   content) is unchanged — handled by the `story.StoryCreator` library
   in `server/story/`. The feed2 changes are purely about client-side
   *presentation* (layout variants). The variant is assigned at feed
   generation time in `createStoryItem()`, not at story creation time.

10. **Story variant 7 data source specified (stories.md: StoryPayload).**
    The Pull Quote variant needs aggregate stats. These come from the
    existing `StoryPayload.impact` field. Updated fallback rule: variant 7
    falls back to 3 if `impact` is empty (not "aggregate stats" generically).

11. **Theming clarification (architecture.md: colors and theming).** Added
    explicit note that overlay colors on full-bleed photo cards (story and
    nudge variants) are design-intrinsic and not `AppColors` violations —
    matching the existing `StoryContentView` and content view patterns.
    `AppColors` is required for activity badges and other elements that
    render over the app chrome.

12. **Server nudge locale support added (i18n.md: both locales required).**
    Nudge text generation must produce strings in the user's locale. Added
    Phase 3 step for locale-aware text with locale-keyed string maps.
    Added Phase 7 test for both en and es nudge text.
