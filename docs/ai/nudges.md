# Nudge Cards: System-Generated Inspiration

## Problem Statement

The feed is entirely reactive — it only shows things that have already
happened. There is no mechanism for the system to proactively inspire users
to take action: plan an experience, share gear, ask for help, or simply get
outside. The feed needs **nudge cards** — contextual prompts interspersed
among community activity that encourage the user to do something.

### Desired behavior

Nudges are a new feed item type — system prompts that inspire the user to
take action. They are not tied to a specific community event. They are
generated server-side based on user behavior, circle activity, and context.

- Inserted every ~8 content items in the final feed
- One nudge always placed at the end as a feed terminator ("You're all
  caught up — close the app and go outside")
- Background imagery from stock providers, contextually matched to content
- Pre-localized text from the server (client renders directly)
- CTAs that route to creation flows

### Prerequisites

This work depends on:
- **Feed Freshness** (`docs/ai/feed-freshness.md`) — seen tracking and
  expiry rules that nudges also follow
- **Story Card Variants** (`docs/ai/stories.md`) — shared card sub-widgets
  (`GlassPill`, `AvatarStack`, etc.) and the variant framework pattern

## Current Architecture

### Feed rendering

The feed currently renders content views (gear, experience, request,
community) and stories in a vertical `PageView`. Feed items come
pre-ordered from the server. The client trusts server order.

### Stock imagery system

The server has a stock imagery system in `server/media/` with:
- `StockImageryProvider` interface in `server/media/stock_imagery_provider.go`
- Three-layer architecture: semantic cache → Pexels → Unsplash/Pixabay fallback
- Semantic cache handles deduplication across repeated queries for similar themes
- `CopyStockImageForUser()` for the copy-on-use pattern (per `docs/stock_imagery.md`)

### Creation flows

Experience, request, and gear creation follow a pattern that nudges will
mirror: (1) generate content, (2) derive a stock image search query,
(3) fetch stock image, (4) copy for user, (5) attach media ID. The request
service does steps 3-5 asynchronously — the request appears without media
at first, then the image shows up shortly after.

## Approach: Stored nudge records with async imagery

Nudges become first-class stored records (`StoredNudge` table), created
ahead of display. The creation flow mirrors request creation:
1. `generateNudges()` produces personalized text + derives search keywords
2. Persists `StoredNudge` records (no media yet)
3. Launches async goroutines to fetch stock imagery via semantic cache /
   provider chain and `CopyStockImageForUser()`
4. Updates each `StoredNudge.media_id` when the fetch completes

`GetFeed` only includes stored nudges where `media_id` is populated —
incomplete nudges are invisible. Nudges appear on the *next* feed load
once imagery arrives (typically seconds via semantic cache).

### Key constraints

- A nudge must **never** be shown without a background image
- Background images must be **contextually matched** to personalized content
- **Used = dismissed:** When a user taps a CTA, the nudge is consumed and
  never appears again
- **Seen expiry:** Same freshness rules as content views (seen twice or
  24 hours with no tap — expires)
- **Records kept for analytics:** Expired and consumed nudges are
  soft-deleted, not hard-deleted

### Feed terminator

The last item in the feed is always a "go outside" nudge. The terminator:
- Uses variant 3's layout (Headline Card) but with rotating content
- Is always generated fresh (not reused from stored nudges)
- Is not subject to seen-expiry (regenerates on every feed load)
- The previous terminator is soft-deleted when a new one is created

### CTA actions

Each nudge carries a `cta_action` identifier that the client maps to
navigation:
- `plan_experience` → `UnifiedCreationModal` (experience intent)
- `list_item` → `UnifiedCreationModal` (gear intent)
- `ask_for_help` → `UnifiedCreationModal` (request intent)
- `check_listings` → user's gear list

### Nudge text localization

Nudge `headline`, `description`, `cta_label`, and `secondary_cta_label`
are server-generated strings. The server produces localized text based
on the user's locale preference. The client renders them directly — no
`context.l10n` lookups needed for nudge content.

## Resolved Design Decisions

1. **Nudge content generation: AI-first.** Nudge text (headline,
   description, CTA label) is AI-generated via the `ai.Provider` interface
   — a new `GenerateNudgeContent` method. This enables personalized,
   context-aware nudges ("Sarah shared 3 items this week — match her!").
   Because AI generation adds latency, nudges are **pre-generated** ahead
   of display: `generateAndPersistNudges()` runs when the active nudge
   count drops below threshold, and nudges only become visible on the
   *next* feed load once both text and imagery are ready.

2. **Nudge frequency: every 8 items, server-configurable.** The interval
   starts at 8 and is stored as a tunable constant
   (`defaultNudgeInterval = 8`). This can be adjusted without code changes
   via a server flag (`--nudge-interval`) in the future. For small feeds
   (< 8 items), only the terminator appears.

3. **Nudge scope: community-independent.** Nudges are scoped to a single
   `(user_id, community_id)` pair. The system does not deduplicate across
   communities. Each community's feed generates and manages its own nudge
   pool independently.

4. **Terminator: rotating pool of pre-written messages.** The feed
   terminator uses a daily-rotating pool of static headline/description
   pairs (no AI call per feed load). The pool rotates by day
   (`time.Now().YearDay() % len(pool)`), so every user across every
   community sees the same terminator message on a given day. A new day
   means a new message. Terminator stock imagery uses the same async
   fetch + semantic cache as regular nudges — one image per day, shared.

5. **ConsumeNudge timing: optimistic fire-and-forget.** On CTA tap, the
   client (a) marks the nudge as consumed in local state immediately so
   it disappears from the current feed, (b) navigates to the creation
   flow, and (c) fires `ConsumeNudge` RPC asynchronously. If the RPC
   fails, the nudge may reappear on the next feed load — an acceptable
   trade-off for smooth UX. The `FeedService` client method returns a
   `Future` that is not awaited before navigation.

6. **Nudge analytics: structured logging, no dashboard.** Nudge lifecycle
   events (generated, displayed, consumed, expired) are logged via
   structured logging with standard fields (`operation`, `nudge_id`,
   `community_id`, `cta_action`, `duration_ms`). This feeds into the
   existing Cloud Monitoring log-based metrics and alerting infrastructure
   (see `docs/server/observability.md`). Soft-deleted records remain in
   the database for future SQL analysis. No dedicated dashboard is in
   scope — the existing observability pipeline surfaces anomalies, and
   ad-hoc queries handle effectiveness analysis.

7. **Stock imagery: existing fallback chain is sufficient.** The stock
   imagery system already has a three-layer fallback (semantic cache →
   Pexels → Unsplash) with query keyword extraction strategies. Nudges
   that fail imagery fetch simply remain invisible until the next
   generation cycle retries. No additional local fallback pool is needed.

## Implementation Plan

### Phase 1: Proto + model — nudge storage

**Goal:** Define the nudge data model and storage layer.

1. New model: `StoredNudge` proto in `proto/ripls/models/nudge.proto`:
   - `id`, `user_id`, `community_id`
   - `nudge_variant` (int32), `headline`, `description`
   - `cta_label`, `cta_action`
   - `optional secondary_cta_label`, `optional secondary_cta_action`
   - `optional location_hint`
   - `optional media_id` (empty until imagery fetch completes)
   - `repeated string participant_ids`
   - `repeated StoredNudgeStat stats` (with `value` + `label`)
   - `stock_query` (derived search query for imagery)
   - `int64 created_at_unix_sec`
   - `bool is_terminator` (true for "go outside" feed-end nudges)
   - `optional int64 consumed_at_unix_sec` (set when user taps CTA)
   - `optional string consumed_action` (which CTA was tapped, for analytics)
   - `optional DeletedMetadata deleted` (soft-delete for expiry)

2. Add `FEED_ITEM_TYPE_NUDGE` to `FeedItemType` enum in
   `proto/ripls/api/feed_service.proto`

3. Add nudge payload messages to `proto/ripls/api/feed_service.proto`:
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

4. Add `NudgePayload nudge` to `FeedItem.payload` oneof

5. Add `ConsumeNudge` RPC to `FeedService` in
   `proto/ripls/api/feed_service.proto`:
   ```
   rpc ConsumeNudge(ConsumeNudgeRequest) returns (ConsumeNudgeResponse);

   message ConsumeNudgeRequest {
     string nudge_id = 1;
     string action = 2;
   }
   message ConsumeNudgeResponse {}
   ```

6. Run `npm run generate`

### Phase 2: Server — AI provider + nudge storage + generation

**Goal:** Add nudge generation to the AI provider interface, create storage,
and manage nudge lifecycle.

1. Add `GenerateNudgeContent` to `ai.Provider` interface in
   `server/ai/provider.go`:
   ```go
   // GenerateNudgeContent creates personalized nudge content for a user.
   GenerateNudgeContent(ctx context.Context, req *NudgeContentRequest) (*NudgeContentResponse, error)
   ```

2. New types in `server/ai/provider.go`:
   ```go
   type NudgeContentRequest struct {
       // User's first name for personalization
       UserName string
       // Names of active community members (for social nudges)
       MemberNames []string
       // Community name
       CommunityName string
       // Summary of recent community activity (items shared, experiences, etc.)
       ActivitySummary string
       // User's locale (e.g., "en", "es")
       Locale string
       // Number of nudge items to generate
       Count int
       // CTA actions to distribute across nudges
       CTAActions []string
   }

   type NudgeContentResponse struct {
       Nudges []GeneratedNudge `json:"nudges"`
   }

   type GeneratedNudge struct {
       Headline       string   `json:"headline"`
       Description    string   `json:"description"`
       CTALabel       string   `json:"cta_label"`
       CTAAction      string   `json:"cta_action"`
       SearchKeywords []string `json:"search_keywords"`
       NudgeVariant   int      `json:"nudge_variant"`
   }
   ```

3. Add prompt in `server/ai/prompts.go` — instructs the LLM to generate
   personalized, casual nudge content with stock image search keywords.
   Nudges are localized to the user's locale preference

4. Implement `GenerateNudgeContent` in all three providers (Gemini,
   OpenAI, Anthropic) + `MockProvider`

5. Register `StoredNudge` in `server/storage/protosql_schema.go`

6. New file: `server/services/feed/nudge_storage.go` — service-internal
   helpers that wrap `storage.ProtoSQLStorage` operations (per server
   architecture: use storage layer for all persistence, don't write raw
   SQL in services):
   - `insertNudge(ctx, store, nudge)` — `store.Insert(ctx, nudge)`
   - `getActiveNudgesForUser(ctx, store, userID, communityID)` — uses
     `store.QueryByFields()` with filters: `media_id IS NOT NULL`,
     `consumed_at_unix_sec IS NULL`, `deleted IS NULL`, ordered by
     `created_at_unix_sec DESC`
   - `updateNudgeMediaID(ctx, store, nudgeID, mediaID)` — `GetByID` +
     set field + `store.Update()`
   - `markNudgeConsumed(ctx, store, nudgeID, action)` — `GetByID` + set
     `consumed_at_unix_sec` + `consumed_action` + `store.Update()`
   - `softDeleteNudge(ctx, store, nudgeID)` — `GetByID` + set
     `deleted` metadata + `store.Update()`

7. New file: `server/services/feed/nudges.go` — nudge generation:
   - `generateAndPersistNudges(ctx, userID, communityID, count)` — gathers
     user context via `storage.ProtoSQLStorage` queries (user record for
     name/locale, community members, recent activity counts). Per server
     architecture: services depend on shared libraries, not other services.
     All context gathering uses `store.GetByID()` / `store.QueryByField()`
   - Calls `aiProvider.GenerateNudgeContent()` in a single batch call
   - Persists `StoredNudge` records (no media yet)
   - Launches async goroutines per nudge to fetch imagery via
     `StockImageryProvider.GetStockImage()` + `CopyStockImageForUser()`
     + `updateNudgeMediaID()`
   - Logs nudge generation events with structured logging (`operation:
     "GenerateNudges"`, `nudge_count`, `community_id`, `duration_ms`)

8. Add `ai.Provider` and `StockImageryProvider` as optional constructor
   dependencies on feed `Service` (interface-based injection). Nudge
   generation degrades gracefully (skips nudges) if either provider is nil

9. Add unit tests for nudge storage, generation, and AI mock responses

### Phase 3: Server — feed integration + terminator pool

**Goal:** Interleave nudges into the feed response. Implement the
daily-rotating terminator pool.

1. New file: `server/services/feed/terminator_pool.go` — static pool of
   ~20 terminator headline/description pairs. Daily rotation via
   `time.Now().YearDay() % len(pool)`. Each entry includes a
   `stock_query` for imagery. Examples:
   - "You're all caught up" / "Close the app and go outside. The world
     is better in person."
   - "Nothing left to see" / "Your community is thriving. Go be part
     of it."
   - `GetTerminatorForDay(day int) TerminatorEntry` — returns the
     entry for a given day

2. In `GetFeed`: query `StoredNudge` via `GetActiveForUser` (1 batch query)
3. Apply freshness rules using `FeedItemView` data (same as content views)
4. Nudges that fail freshness are soft-deleted (preserved for analytics).
   Log each soft-delete (`operation: "ExpireNudge"`, `nudge_id`,
   `reason: "freshness"`)
5. If active nudge count is below needed count (`feedSize / 8`), call
   `generateAndPersistNudges()` to create new ones. New nudges are
   invisible on this call (no media yet) and appear on next feed load
6. Insert active nudges at positions `[8, 16, 24, ...]` in the feed
7. Append terminator at end: look up today's `TerminatorEntry`, create or
   reuse a `StoredNudge` with `is_terminator=true` for today. If the
   terminator from a previous day exists, soft-delete it. Terminator
   imagery uses the same async fetch as regular nudges
8. Implement `ConsumeNudge` RPC handler: marks nudge consumed with
   timestamp and action string, logs event (`operation: "ConsumeNudge"`,
   `nudge_id`, `cta_action`). Nudge disappears from subsequent feed loads
9. Add unit tests for nudge placement, terminator rotation, and consumption

### Phase 4: Server — testing flag

**Goal:** Enable testing nudges without scrolling past 8+ content items.

1. Add `optional int32 debug_nudge_count = 4` to `GetFeedRequest`
2. In `GetFeed`, if `debug_nudge_count > 0`: generate that many nudges and
   prepend them before content items. Still append terminator
3. Add `NUDGE_TEST_COUNT` to `env.dev.json` (default: `"0"`)
4. Add `static int get nudgeTestCount` to `Environment`
5. `FeedService.getFeed()` passes the value as `debugNudgeCount` in the
   request
6. `env.prod.json` omits the field (defaults to 0)

### Phase 5: Client — data layer + nudge card rendering

**Goal:** Add client data layer for nudge consumption and render nudge
cards with CTAs in the feed.

**Data layer — repository and service:**

1. Add `consumeNudge(nudgeId, action)` to the existing `FeedService`
   client (`app/lib/services/feed_service.dart`). This wraps the
   `ConsumeNudge` RPC

2. Add `consumeNudge(nudgeId, action)` to `FeedRepository`
   (`app/lib/data/repositories/feed_repository.dart`). This calls through
   to `FeedService` and invalidates the feed cache so the next feed load
   excludes the consumed nudge. Per caching architecture: repositories are
   the ONLY layer that calls services

**ViewModel — nudge consumption state:**

3. Add nudge consumption handling to `FeedNotifier`
   (`app/lib/presentation/viewmodels/feed_view_model.dart`):
   - `consumeNudge(nudgeId, action)` — optimistic: immediately adds
     `nudgeId` to a `Set<String> consumedNudgeIds` in `FeedState`,
     then calls `feedRepository.consumeNudge()` fire-and-forget
   - `FeedState.consumedNudgeIds` — Freezed field, default empty set.
     Used by the feed list builder to skip rendering consumed nudges
   - This keeps consumption state in the ViewModel layer, not widgets
     (per architecture: no `setState()` for app state)

**Widgets — nudge card rendering:**

4. New file: `app/lib/presentation/widgets/nudge/nudge_card_variants.dart`
   — 3 layout builders reusing shared `card_components.dart` widgets from
   the stories work:
   - Variant 1: **Magazine Spread** — aspirational editorial with glass pill,
     big serif headline, italic prose, CTA + avatar stack
   - Variant 2: **Photo Journal** — dark card, star-icon header, horizontal
     gear suggestion cards, narrative, CTA
   - Variant 3: **Headline Card** — centered poster, "You're all caught up",
     short paragraph, single CTA. Used as feed terminator

5. New file: `app/lib/presentation/widgets/nudge/nudge_content_view.dart`
   — `NudgeContentView` ConsumerWidget that dispatches to variant builder.
   Located in `widgets/` (not `screens/`) because it is a card component
   rendered inside `FeedScreen`, not a standalone navigable screen

6. Background images via `ContentViewBuilders.buildMediaBackground()`.
   Media URLs resolved through `FeedNotifier` which reads
   `MediaRepository` — widgets access media URLs via ViewModel state,
   not by calling repositories directly (per architecture: ViewModels
   are the ONLY layer that calls Repositories)

7. CTA buttons: primary uses `AppColors.coral(context)`. Ghost secondary
   buttons use theme-aware semi-transparent styling via
   `AppColors` helpers. All colors must be theme-aware per architecture

8. CTA navigation: use `NavigationHelpers` for all navigation. Map
   `cta_action` values:
   - `plan_experience` → `UnifiedCreationModal` (experience)
   - `list_item` → `UnifiedCreationModal` (gear)
   - `ask_for_help` → `UnifiedCreationModal` (request)
   - `check_listings` → user gear list

9. CTA tap handler in widget calls
   `ref.read(feedProvider.notifier).consumeNudge(id, action)` then
   navigates. The ViewModel handles the optimistic dismiss + async RPC.
   Widget does not call services or repositories directly

10. `feed_screen.dart` adds `FEED_ITEM_TYPE_NUDGE` → `NudgeContentView`,
    skipping items where `consumedNudgeIds.contains(item.nudgeId)`

11. Nudge text (headline, description, CTA labels) comes pre-localized
    from the server. The client renders them directly — no
    `context.l10n` lookups for nudge content. Any static chrome text
    on the nudge card (badges, labels not from server) must use
    `context.l10n` and be added to both `app_en.arb` and `app_es.arb`

### Phase 6: Testing and polish

1. Server tests: nudge placement at every 8th position + terminator
2. Server tests: AI-generated nudge text via `MockProvider` in both en
   and es locales
3. Server tests: consumption and expiry lifecycle
4. Server tests: async imagery fetch and invisible-until-complete behavior
5. Server tests: terminator pool daily rotation
   (`GetTerminatorForDay` returns expected entries)
6. Client widget tests for all 3 nudge variants
7. Client tests for CTA routing + optimistic fire-and-forget dismiss
8. Performance: verify with `storage.AssertMaxQueries` that nudge queries
   stay bounded (nudge fetch is 1 batch query per `GetFeed`)
9. Structured logging: verify nudge lifecycle events appear in logs
   (generate, display, consume, expire) with correct fields
10. Edge cases: nil AI/stock providers (graceful degradation), empty feed
    (only terminator), new user first load (nudges appear on second load)

### Future: Adding more nudge variants

Once the initial 3 variants are working and we have engagement data:

1. Add new builder methods to `nudge_card_variants.dart`
2. The remaining reference designs from `tmp/feed/nudges.jsx`:
   - 4: The Postcard — weather/conditions split layout
   - 5: The Ticker Tape — celebrate a friend's generosity
   - 6: The Open Door — warm nudge to ask for help
   - 7: The Pull Quote — giant stat ("21 days since...")
   - 8: The Scrapbook — overlapping memory cards
   - 9: The Field Guide — suggested destination
   - 10: The Golden Hour — dreamy, poetic
3. Use engagement data from A/B framework (built in stories work) to
   inform which nudge variants to add next

## Audit Log

**2026-03-26 — Architecture compliance audit** (refs: `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`)

Changes made:

1. **Phase 1 — ConsumeNudge RPC location**: Specified that `ConsumeNudge`
   belongs on `FeedService` in `feed_service.proto`. Original plan left
   the owning service ambiguous.

2. **Phase 2 — Storage helper pattern**: Renamed nudge storage methods to
   lowercase (unexported, service-internal) and specified they wrap
   `storage.ProtoSQLStorage` operations. Original plan used capitalized
   names suggesting a separate public API, and didn't clarify that these
   use the storage layer (not raw SQL). Per server architecture: "use
   storage layer for all persistence."

3. **Phase 2 — Service independence**: Added explicit note that nudge
   generation gathers user context via `ProtoSQLStorage` queries, not by
   calling other services. Per server architecture: "services should not
   call or depend on other services."

4. **Phase 5 — Missing data layer**: Added FeedService client method,
   FeedRepository mutation method, and FeedNotifier consumption state.
   Original plan had the widget firing the RPC directly and managing
   local consumption state — two violations:
   - Per client architecture: "ViewModels are the ONLY layer that calls
     Repositories" / "Screens/Widgets never access repositories directly"
   - Per caching architecture: "Repositories are the ONLY layer that
     calls Services"

5. **Phase 5 — Media URL resolution**: Changed from widget reading
   `MediaRepository` directly to resolving through `FeedNotifier`. Per
   client architecture: "ViewModels are the ONLY layer that calls
   Repositories."

6. **Phase 5 — Theme-aware colors**: Changed `AppColors.coral` to
   `AppColors.coral(context)` and specified all colors must use
   theme-aware helpers. Per client architecture: "All colors must be
   theme-aware."

7. **Phase 5 — i18n for chrome text**: Added requirement that any static
   UI text on nudge cards (not from server) must use `context.l10n` and
   be added to both ARB files. Per i18n doc: "Every user-visible string
   should use `context.l10n.*`."
