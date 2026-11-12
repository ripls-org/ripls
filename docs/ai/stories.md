# Story Card Variants: Experimental Layouts with A/B Testing

## Problem Statement

When a loan, giveaway, experience, or request completes, the feed shows a
story — but every story uses the same single layout (`StoryContentView` in
`app/lib/presentation/screens/story/story_content_view.dart`). Stories are a
key moment of community celebration and should feel varied, editorial, and
worth lingering on.

We need multiple story card formats so we can test which presentations
resonate most with users. Rather than building all 10 designs from the
reference (`tmp/feed/stories.jsx`) at once, we start with 3 variants and
build an experimental framework that lets us measure which ones perform best.

### Desired behavior

- Stories in the feed are rendered in one of 3 varied editorial layouts
  instead of the current single layout
- Each story is deterministically assigned a variant (same story always shows
  the same layout)
- The system tracks which variant each story uses and how users interact
  with it (view duration, taps) so we can measure engagement
- The framework is extensible — adding new variants later is just adding a
  new builder method and bumping the variant count

## Current Architecture

### How stories work today

**Story creation** (unchanged by this work):
1. Various services call `StoryCreator.CreateStory()` in `server/story/generator.go`
   with a `CreateStoryRequest` containing community ID, story type, participants,
   media IDs, impact metrics, etc.
2. The generator checks for duplicates, generates title + description via AI
   (or template fallback), and inserts a `models.Story` record
3. Stories are triggered synchronously from community events (e.g., loan
   completed, giveaway picked up)

**Story rendering in feed:**
1. `generateFeedItems()` in `server/services/feed/service.go` queries all
   stories for the community via `storyStorage.ListByCommunity()`
2. `createStoryItem()` (line 611) converts each `models.Story` to a
   `FeedItem` with `StoryPayload` — including title, description, media IDs,
   participants, and impact metrics
3. On the client, `feed_screen.dart` (line 255) renders `FEED_ITEM_TYPE_STORY`
   items using `StoryContentView`
4. `StoryContentView` renders every story identically: background media with
   60% black overlay, title, description, participant list, and optional
   impact savings

**Story data model** (`proto/ripls/models/story.proto`):
- `id`, `community_id`, `story_type`, `title`, `description`
- `repeated string media_ids`, `repeated string participant_ids`
- `gear_id`, `loan_id`, `experience_id`, `request_id`
- `cost_saved_usd`, `time_saved_minutes`, `co2_saved_grams`

**Story API payload** (`proto/ripls/api/feed_service.proto`):
- `StoryPayload` with `story_type`, `title`, `description`, `media_ids`,
  `participants`, `impact`, and item ID fields

### Story types (enum StoryType)

- `STORY_TYPE_LOAN_COMPLETED` — gear returned
- `STORY_TYPE_GIVEAWAY_COMPLETED` — gear given away
- `STORY_TYPE_EXPERIENCE_CONCLUDED` — experience ended
- `STORY_TYPE_REQUEST_FULFILLED` — request fulfilled
- `STORY_TYPE_NEW_MEMBER_WELCOME` — new member joined
- `STORY_TYPE_EXPERIENCE_RSVP` — RSVP milestone

## Approach: 3 initial variants + experimental framework

### Starting with 3 variants

We pick 3 visually distinct layouts from the reference designs that cover
the most common story shapes:

1. **The Magazine Spread** (reference: Story1) — big serif headline over
   full-bleed photo. Editorial italic prose. Glass location pill at top.
   Avatar stack + date at bottom. Gradient: bottom-heavy
   (0.88 → 0.3 → transparent). ~580px. Works for all story types.

2. **The Photo Journal** (reference: Story2) — dark card (#0e0e0e). Avatar
   + name header. Horizontal photo carousel (item media, up to 3 images,
   260px, scroll-snap with dot indicators). Long-form narrative. Individual
   avatar + first-name chips at bottom. Works best for stories with
   multiple media.

3. **The Headline Card** (reference: Story3) — vertically centered over
   full background. Avatar stack, large serif headline, subtitle, glass
   location pill + date at bottom. ~520px. Clean, minimal — good default
   for stories with single media or sparse data.

### Variant assignment

- `story_variant = (FNV-1a(story_id) % 3) + 1`
- Deterministic: same story always gets the same variant
- New field: `optional int32 story_variant` on `FeedItem` in the API proto
- Assigned at feed generation time in `createStoryItem()` — a presentation
  concern, not a storage concern

### Data compatibility fallbacks

- Variant 2 (Photo Journal) works best with multiple media. If a story has
  0 media, fall back to variant 3 (Headline Card)
- All 3 variants receive the same `StoryPayload` data

### A/B testing framework

Rather than random assignment, the system uses deterministic hashing so
results are reproducible. The experimental framework tracks engagement
through the existing client observability system (see
`docs/client/observability.md`).

**Server-side (already available):**
- Which variant was shown (`story_variant` on the `FeedItem`)
- View count (already tracked)
- View timestamps (already tracked via `last_seen_at_unix_sec` from the
  freshness work in `docs/ai/feed-freshness.md`)

**Client-side (via existing `ObservabilityService`):**
- Two new typed analytics events in `events.dart`:
  - `StoryViewedEvent` — logged on story appear with `story_variant`,
    `story_type`, `view_duration_ms` (measured by `Stopwatch`, reported
    on disappear/dispose)
  - `StoryTappedEvent` — logged when user taps through to the underlying
    item, with `story_variant` and `story_type`
- Events flow through `ObservabilityService.logAnalyticsEvent()`, which
  respects the existing opt-in consent model (Analytics category). No data
  is sent if the user has not granted analytics consent.
- No new RPC, no server storage changes, no proto additions beyond the
  `story_variant` field on `FeedItem`

**Analysis:**
- Per-variant metrics: average view duration, tap-through rate, completion
  rate (scrolled past vs. lingered)
- Queryable in Firebase Analytics console or BigQuery export — filter by
  `story_variant` event parameter
- Once we have data, we can adjust variant weights or add new variants

### Theming note

Story cards render over full-bleed photos with dark overlays, so text is
always white-on-dark regardless of app theme. Overlay colors
(`rgba(0,0,0,0.4)`, `rgba(255,255,255,0.7)` for text, etc.) are intrinsic
to the card design and do not vary by theme — this matches the existing
`StoryContentView` and all content view patterns. These are not `AppColors`
violations.

## Open Questions

1. **Variant assignment stability when adding variants:** The current
   formula is `FNV-1a(story_id) % 3 + 1`. When we later expand to 10
   variants, existing stories will get different variant assignments
   (`% 10` gives different results than `% 3`). Is that acceptable? Users
   who saw a story as Magazine Spread will see it as something else on
   their next feed load. Alternatively, we could store the assigned variant
   on the `Story` model at creation time so it never changes — but that
   makes it a storage concern rather than a presentation concern.

2. **Serif font on mobile:** The reference designs use Georgia/Times New
   Roman for headlines. Flutter's default serif font varies by platform.
   Should we bundle a specific serif font (e.g., Playfair Display, Lora)
   for consistency, or use the platform default? Bundling adds app size
   but guarantees the editorial feel.

3. **~~Engagement tracking privacy~~ — RESOLVED:** Story engagement
   events flow through the existing `ObservabilityService` which enforces
   opt-in consent (Analytics category). No data is collected unless the
   user grants analytics consent. No new privacy policy disclosure needed
   beyond what already covers the analytics consent flow.

4. **Photo Journal without multiple media:** Variant 2 falls back to
   variant 3 when a story has 0 media. What about stories with exactly
   1 media? A single-image carousel looks odd. Should variant 2 require
   2+ media to be assigned, falling back to variant 1 or 3 for single-media
   stories?

5. **Story variant outside the feed:** Stories also appear in
   `StoryThumbnail` (110x100 cards) and via `ListStories` /
   `ListItemStories` RPCs. Should variant assignment apply there too, or
   only in the feed `PageView`? The thumbnail is too small for variant
   differences to matter, but the full-screen story view (navigated from
   thumbnail) could use the variant layout.

6. **Minimum data for engagement analysis:** How many story views per
   variant do we need before the A/B data is actionable? With 3 variants
   and a small user base, it may take weeks to accumulate meaningful
   signal. Should we define a minimum sample size threshold before making
   variant weight adjustments?

## Implementation Plan

### Phase 1: Proto + server — variant assignment

**Goal:** Assign a variant to each story in the feed response.

1. Add `optional int32 story_variant = 18` to `FeedItem` in
   `proto/ripls/api/feed_service.proto` (must be `optional` — only
   present on story-type feed items; gives `Has*()` in Dart, `!= nil`
   in Go per proto field conventions)
2. In `createStoryItem()` (`server/services/feed/service.go`), assign
   `story_variant = (FNV-1a(story_id) % 3) + 1` with data-compatibility
   fallback (variant 2 with 0 media → variant 3)
3. Run `npm run generate` to regenerate Go and Dart code
4. Run `npm run build` to verify lint + compilation
5. Add unit tests for variant assignment determinism and fallbacks

### Phase 2: Client — shared card sub-widgets

**Goal:** Build reusable UI components shared across story variants.

1. New file: `app/lib/presentation/widgets/content/card_components.dart`
   — shared sub-widgets:
   - `GlassPill` — `ClipRRect` + `BackdropFilter(blur: 20)` + dark
     semi-transparent background + thin white border
   - `AvatarStack` — overlapping `CircleAvatar` with negative margin.
     Avatars use `CachedMediaImage` with `ImageCacheKeys.thumbnail()`
     cache keys (< 200px display)
   - `PhotoCarousel` — horizontal `PageView` with dot indicators (for
     variant 2). Each image uses `CachedMediaImage` with
     `ImageCacheKeys.full()` cache keys (260px display, >= 200px
     threshold)
   These are public widgets (not `_`-prefixed) because they are shared
   across variant files and will be reused by nudge cards later

### Phase 3: Client — 3 story variant builders

**Goal:** Implement the 3 story card layouts.

1. One file per variant to keep each under 1000 lines (per file size
   guidelines):
   - `app/lib/presentation/widgets/story/story_magazine_spread.dart`
     — `StoryMagazineSpread` widget (variant 1)
   - `app/lib/presentation/widgets/story/story_photo_journal.dart`
     — `StoryPhotoJournal` widget (variant 2)
   - `app/lib/presentation/widgets/story/story_headline_card.dart`
     — `StoryHeadlineCard` widget (variant 3)
   Each is a `StatelessWidget` (or `ConsumerWidget` if it needs `ref`)
   that receives `StoryPayload`, `List<MediaUrl>`, and participant data
   as constructor parameters
2. All variants use `ContentViewBuilders.buildMediaBackground()` for
   full-bleed backgrounds with `CachedMediaImage` and
   `ImageCacheKeys.full()` cache keys (backgrounds are large display).
   Media loading goes through `MediaRepository` via the existing
   `StoryViewModel` (no direct service access from widgets)
3. `StoryContentView` dispatches to the variant widget based on
   `story_variant` from parent `FeedItem`. Fallback to variant 3 if
   `hasStoryVariant()` is false or value is unrecognized
4. The existing `StoryContentView` layout becomes variant 3 (Headline
   Card) with visual refinements — no separate "legacy" code path
5. Any user-visible strings (date labels, location names, accessibility
   labels) must use `context.l10n` — add keys to `app_en.arb` and
   `app_es.arb`

### Phase 4: Client — engagement tracking via observability

**Goal:** Track how users interact with each variant for A/B analysis,
using the existing `ObservabilityService` (see `docs/client/observability.md`).

1. Add two typed analytics events to
   `app/lib/core/observability/events.dart`:
   - `StoryViewedEvent` — parameters: `story_variant` (int),
     `story_type` (string), `view_duration_ms` (int), `story_id` (string)
   - `StoryTappedEvent` — parameters: `story_variant` (int),
     `story_type` (string), `story_id` (string)
2. `StoryContentView` must be a `ConsumerStatefulWidget` (needs
   `Stopwatch` lifecycle in `initState`/`dispose` and `ref.read()`
   access for logging). Start the `Stopwatch` on appear. On disappear
   or dispose, log `StoryViewedEvent` via
   `ref.read(observabilityServiceProvider).logAnalyticsEvent(...)`.
   On tap-through navigation, log `StoryTappedEvent` before navigating
3. Events respect the existing opt-in consent model — no data is sent
   unless the user has granted Analytics consent. No new RPC, no proto
   changes, no server storage changes
4. Add unit tests using `MockObservabilityService` (from
   `test/core/observability/analytics_test_helper.dart`) to verify
   events are logged with correct parameters

### Phase 5: Testing and polish

1. Widget tests for each of the 3 story variants
2. Widget tests for shared `card_components.dart` widgets
3. Server tests for variant assignment determinism
4. Analytics event tests using `MockObservabilityService` — verify
   `StoryViewedEvent` and `StoryTappedEvent` parameters
5. Verify all existing story-related tests still pass
6. Visual QA: test with stories that have 0 media, 1 media, 3+ media,
   0 participants, 5+ participants, with/without impact data

### Future: Adding more variants

Once the framework is working and we have engagement data from the initial
3 variants:

1. Add new builder methods to `story_card_variants.dart`
2. Bump the modulo from 3 to N in `createStoryItem()`
3. Add data-compatibility fallbacks for any new variants with special
   requirements (e.g., variant 4 "Postcard" needs exactly 2 participants,
   variant 7 "Pull Quote" needs impact data)
4. The remaining 7 reference designs from `tmp/feed/stories.jsx`:
   - 4: The Postcard — horizontal split, photo + story
   - 5: The Ticker Tape — scrolling stat ribbon
   - 6: The Dispatch — newspaper headline style
   - 7: The Pull Quote — giant hero stat
   - 8: The Scrapbook — overlapping photo collage
   - 9: The Field Guide — place-focused
   - 10: The Golden Hour — dreamy, nostalgic
