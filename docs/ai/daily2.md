# Daily Screen v2: Fresh Start

## Overview

Replace the inbox tab with a **daily screen** — a personal daily rhythm view
that blends time, social connection, and shared activity. This is a complete
redesign replacing the original `daily.md` plan, which accumulated 15 phases of
corrections. We start fresh here with a simpler, more focused design.

**Reference materials:**
- [daily2.jsx](../../tmp/daily/daily2.jsx) — React reference implementation
- [daily2a.png](../../tmp/daily/daily2a.png) — Top of screen: header, metrics, people, today items with thumbnails
- [daily2b.png](../../tmp/daily/daily2b.png) — Coming Up section with simpler line items
- [daily2c.png](../../tmp/daily/daily2c.png) — Expanded metric breakdown (tapping a stat pill)
- [daily2d.png](../../tmp/daily/daily2d.png) — Expanded person (tapping a person circle shows their items)
- [daily2e.png](../../tmp/daily/daily2e.png) — Shared value in header, quality time inline, "Your Actions Today" section
- [daily2f.png](../../tmp/daily/daily2f.png) — Expanded shared value panel (inventory list)
- [daily2i.png](../../tmp/daily/daily2i.png) — Checkmark styling: dark green check with green border and light green background

---

## Problems with the Original Plan

### P1: Accumulating Corrective Phases

The original plan (daily.md) grew from 8 phases to 15 as each phase tried to
fix problems introduced by earlier ones. Phase 10 redesigned hero cards that
Phase 4 had just built. Phase 12 added an action bar that Phase 14 removed.
Phase 13 added a dark overlay that Phase 14 replaced with a light theme. Phase
15 fixed unread count consistency that should have been a constraint from day
one. This "build then tear down" pattern wasted effort and made the spec hard
to follow.

### P2: Over-Designed Hero Cards

The original plan invested heavily in hero cards with per-item background
images, dark gradient overlays, emoji watermarks, frosted glass effects, and a
three-segment action bar matching `ContentActionBar`. This was visually heavy
and expensive to render (each card fetched its own image, applied backdrop
blur). The new design uses simple list rows with small emoji thumbnails — far
lighter and more scannable.

### P3: Missing People Dimension

The original plan was purely item-centric: items sorted by time and type. It
had no concept of "who" — no way to see at a glance which people you're
interacting with today. The new design adds a **people row** (story-circle
avatars) that shows everyone in your daily activity at a glance, with tap-to-
expand showing that person's items.

### P4: Non-Interactive Metrics

The original metrics section was a static display: ring + three tiles. The new
design makes each metric **tappable and expandable** — tapping "Saved" shows a
breakdown of which items contributed how much. Tapping the ring shows which
activities generated quality minutes. This turns metrics from decoration into
a tool for understanding your impact.

### P5: Unread Count Consistency Was an Afterthought

The original plan didn't establish unread count consistency as a constraint
until Phase 15 (the last phase). By then, the daily screen's filtering logic
and the bottom nav badge's logic had diverged. The fix required extracting
shared functions and adding a "N more conversations" overflow row. This should
have been a first-class constraint from the start.

### P6: No Thumbnails for Visual Scanning

The original plan used text-only list rows (timeline nodes) or full-bleed
hero cards. Neither provided the quick visual scanning that small thumbnails
enable. The new design uses 48px emoji thumbnails with colored backgrounds
for today's items, giving each item a visual identity without the weight of
full images.

---

## Proposed Solution

### Screen Structure

The daily screen has six sections, matching the reference designs:

```
+------------------------------+
| TODAY            [215]       |  <-- Date header + ring (right)
| Monday, Feb 23   quality min |
+------------------------------+
| [$127] [2.5h] [0.8kg]       |  <-- Tappable stat pills
|  SAVED  TIME    CO2          |
|  [expanded breakdown panel]  |  <-- If a pill is tapped
+------------------------------+
| (TE)(FL)(ET)(DK)(GC)        |  <-- People row (story circles)
|  Tom Fio Emm Der Gary       |
|  [expanded person panel]     |  <-- If a person is tapped
+------------------------------+
| ALL TODAY                    |
| 9:00 AM . Help               |
| [thumb] Help Thomas carry... |  <-- Timed items with thumbnails
|         Thomas . South en... |
|                              |
| Lending                      |  <-- Anytime grouped by type
| [thumb] Circular Saw return  |
|         Emma . Cleaned up... |
+------------------------------+
| COMING UP                    |
| Tomorrow                     |
| (av) Skiing at Breck  Going  |  <-- Simple line items
| (av) Bowling at Dark  Going  |
| Friday                       |
| (av) Calligraphy      RSVP   |
+------------------------------+
```

### Section 1: Date Header + Ring

The header integrates the date and the Rich Minutes ring in a single row.

**Left side:**
- "TODAY" label: 11px, w600, coral, uppercase, 0.5 letter-spacing
- Date: 22px, w800, primary text, Georgia serif

**Right side (ring):**
- Circular progress ring: 80px, 5px stroke, coral on light track
- Center: minutes count (18px, w800, Georgia) + "quality min" label (6px, w600, muted, uppercase)
- Tapping the ring opens/closes the Rich Minutes breakdown panel

### Section 2: Stat Pills

Three equal-width tappable tiles below the header.

**Default state:**
- Background: `rgba(255,255,255,0.8)`, border: `1px solid border-color`, 8px radius
- Value: 13px, w800, coral, Georgia serif
- Label: 7px, w700, muted, uppercase, 0.3 letter-spacing

**Selected state (when tapped):**
- Background: `rgba(255,255,255,0.95)`, border: `1px solid accent-color`
- Opens an expandable breakdown panel below the pills

**Expanded breakdown panel:**
- Glass card: 14px radius, `rgba(255,255,255,0.85)` background, blur(16px)
- Header: metric label + total in accent color + summary sentence
- Line items: emoji (18px) + title + note + amount in accent color
- Footer: "This week . Feb 17 - 23" centered
- Tapping the same pill again collapses the panel

**Special: Ring breakdown panel** (when ring is tapped):
- Same glass card style
- Header: "215 / 360 min goal" + "Quality time with friends this week"
- Line items: completed activities this week with +minutes in green
- Subtotal row at bottom

### Section 3: People Row

A horizontally scrollable row of avatar circles for everyone involved in
today's items.

**Avatar circles:**
- Size: 50px with gradient background (name-seeded color)
- Ring: coral gradient ring around avatars with unread messages
- First name below (10px, w600, secondary text; coral when selected)
- Sorted by: most recent activity first

**Tap behavior:**
- Tapping a person opens an expandable panel below the people row
- Panel shows that person's items as a horizontal card scroll
- Each card: emoji thumbnail image (100px height) + type label + time + title + comment preview
- Cards: white background, 12px radius, subtle shadow
- Single item: card takes full width; multiple items: 200px wide, horizontally scrollable
- Tapping the same person again collapses the panel

### Section 4: Today's Items

All of today's items as compact rows with thumbnails. Two sub-sections:

**Timed items** (items with specific times, sorted earliest first):
- Time + type label above: "9:00 AM . Help" (9px, w700, coral)
- Row: [48px emoji thumbnail] + [title + comment preview] + [unread badge + time-ago]
- Thumbnail: 48px square, 12px radius, colored gradient background, emoji centered
- Title: 13px, w700, primary text, single line, ellipsis
- Comment: "FirstName . message text" — 11px, w400, secondary text, single line, ellipsis
- Unread badge: 20px circle, coral background, white count (9px, w700) — only if > 0
- Time ago: 9px, w600, muted (e.g., "12m", "1h", "3h")

**Anytime items** (no specific time, grouped by type):
- Type group headers in fixed order: "Help", "Events", "Lending", "Giving"
- Same row layout as timed items but without the time+type label above
- Within each group, sorted by most recent activity first
- Empty groups are omitted entirely

**Impact overlay (when a metric is expanded):**
When a stat pill is tapped, each item row gains a per-item impact value on the
right side (e.g., "$25" for Saved, "+30m" for Time, "0.1kg" for CO2). This
gives context to how each item contributes to the weekly metric.

### Section 5: Coming Up

Future items as simple line rows grouped by day period.

**Section header:** "COMING UP" (10px, w700, muted, uppercase, 0.5 letter-spacing)
Separated from today by a top border.

**Day group headers:** "Tomorrow", "Friday", "This Weekend", "Next Week"
(10px, w700, secondary text)

**Row layout:**
- [20px avatar] + [title (12px, w600, primary)] + [state text or action button] + [unread badge]
- State text: "You're going", "On loan" — 10px, w600, green
- Action button: coral-tinted background, coral text (10px, w700), 6px radius
- Unread badge: same 20px circle as today items
- Bottom border between rows

### Section 6: Floating Header (Scroll Behavior)

Floating header at top with community name (same pattern as feed screen).
Hides on scroll down, reappears on scroll up. Bottom nav follows the same
hide/show behavior.

---

## Key Constraint: Unread Count Consistency

**Every item with unread messages MUST appear somewhere on the daily screen.**
The sum of all unread badges visible on the daily screen must equal the total
unread count shown on the bottom navigation bar.

### Implementation:

1. **Server-side:** The `GetDailyView` endpoint uses the same `IsItemDone()`
   filtering as `GetUnreadCounts` to exclude terminal-state items. Both
   endpoints share this function from a common package
   (`server/conversation/state.go`).

2. **Server-side:** The response includes a `total_unread_count` field that
   equals the sum of all per-item unread counts. The client can verify this
   matches the bottom nav badge.

3. **Client-side:** If any unread items don't fit into Today or Coming Up
   sections (edge case), they appear in a "N more conversations with new
   messages" overflow row after the last today item.

4. **Items with unread messages always surface:** Even if an item has no
   scheduled time and no pending action, if it has unread messages, it appears
   in the anytime section. Unread messages are a sufficient condition for
   daily screen inclusion.

---

## Item Inclusion Rules

### Today section (Needs Attention):

**Transfers** — shown only if EITHER:
1. The conversation has unread messages, OR
2. A material event is happening today: state changed within today's bounds
   (e.g., recipient selected for pickup, transfer went ACTIVE for handoff).
   Detected via `LatestRequestUnixSec` falling within today's day bounds.
User must be the transfer owner or recipient.

**Experiences** — bucketed by scheduled time:
- **Timed today:** Scheduled within today → always shown in the timed section.
  User must be the experience host or have RSVP'd YES/MAYBE.
- **Coming Up:** All non-terminal, non-deleted community experiences scheduled
  in the future appear in Coming Up, **regardless of whether the user is
  attending**. The scheduled time is itself a material event.
- **Anytime (no/past time):** Shown only if the conversation has unread
  messages. User must be the experience host or have RSVP'd YES/MAYBE.

**Requests** — all open (non-terminal, non-archived, non-deleted) community
requests are shown, regardless of whether the viewing user is the requester
or has offered to help. Every community member sees every open request.

### Coming Up section (future items):

**Experiences** — all non-terminal, non-archived, non-deleted community
experiences with a scheduled time in the future. No RSVP required.

**Transfers** — active (non-terminal) transfers where the user is owner or
recipient, shown when:
- `EstimatedPickupUnixSec` is in the future (item placed at that time), OR
- The loan has a known `ActualPickupUnixSec` and `LoanDurationDays` and the
  computed return date (`actualPickup + loanDurationDays × 86400`) is in the
  future.

A single transfer can appear in both the Needs Attention and Coming Up
sections simultaneously (e.g., it has an unread message AND a future pickup).

### Excluded from daily screen:

- Terminal-state items (completed/cancelled transfers, fulfilled/cancelled
  requests)
- Transfers with no unread messages and no state change today (excluded from
  Needs Attention; may still appear in Coming Up if future pickup/return)
- Untimed/past experiences with no unread messages
- Archived or deleted items

---

## Action Chips and State Chips

Each item may show either an action chip or a state chip in the Coming Up
section. Today's items do NOT show action/state chips — they show unread
badges and comment previews instead. The action context is apparent from the
item title and accessible by tapping into the conversation.

**Coming Up chips:**

| Scenario | Chip | Label | Color |
|----------|------|-------|-------|
| Experience you can RSVP to | Action | "RSVP" | Coral button |
| Experience you're attending | State | "You're going" | Green text |
| Item on loan | State | "On loan" | Green text |
| Request you're helping with | State | "You're helping" | Green text |

---

## Color Palette

Warm earth-tone palette matching the reference designs:

| Token | Value | Usage |
|-------|-------|-------|
| `bg` | `#FDF8F0` | Screen background |
| `text` | `#1C1917` | Primary text |
| `sec` | `#57534E` | Secondary text, comment bodies |
| `mute` | `#A8A29E` | Muted text, labels, time-ago |
| `accent` | `#C2603D` | Coral — ring, badges, stat values, type headers |
| `accentSoft` | `#D8845E` | Lighter coral — ring gradient |
| `green` | `#4E7D5B` | Sage green — state labels, quality min values |
| `card` | `#FFFFFF` | Card backgrounds |
| `glass` | `rgba(255,255,255,0.62)` | Glass card background |
| `glassStrong` | `rgba(255,255,255,0.85)` | Expanded panel background |
| `border` | `rgba(214,207,196,0.5)` | Borders, dividers |
| `overlay` | `rgba(253,248,240,0.82)` | Full-screen background overlay |
| `display` | Georgia, serif | Ring numbers, stat values, date |
| `body` | System default | All other text |

---

## Recommendations

### R1: Keep Inbox Screen Intact

Leave `inbox_screen.dart` and all associated code untouched. The daily screen
is a new screen that replaces the inbox's position in the bottom navigation.
This reduces risk and allows reverting.

### R2: Dedicated `GetDailyView` RPC

A single server endpoint returns the entire daily view in one response. The
server handles querying, filtering, sorting, grouping, metrics computation,
and unread count resolution. The client renders the response.

### R3: Dedicated `GetDailyMetricsBreakdown` RPC

The metric breakdown panels (tapping a stat pill or the ring) need itemized
data that the main `GetDailyView` response doesn't carry. Options:

- **Option A (recommended):** Include breakdown data in the `GetDailyView`
  response. The data is small (5-10 line items per metric) and always useful.
  This keeps everything in one round trip and avoids a loading state when
  expanding a metric.
- **Option B:** Separate `GetDailyMetricsBreakdown` RPC called on-demand when
  a pill is tapped. Reduces initial payload but introduces a loading flash.

### R4: People Row Data from Today's Items

The people row is derived from the `DailyItem` data — unique owners across
all today items. The server can return a `repeated DailyPerson` at the
response level (with unread totals per person and their item IDs) so the
client doesn't need to compute this.

### R5: Thin Client ViewModel

`DailyNotifier` is `AsyncNotifier<DailyViewData>` with `autoDispose`, where
`DailyViewData` is a Freezed class containing the `GetDailyViewResponse` plus
UI toggle state (`expandedMetric`, `expandedPersonId`). This follows the
`TimeModalNotifier` pattern in the codebase. It calls the repository via
`selectedCommunityProvider`, manages refresh, and exposes toggle methods.
No client-side data assembly.

### R6: No Background Image

The new design uses a flat warm cream background (`#FDF8F0`) with a blurred
community image overlay — but the reference screenshots show this is very
subtle. If the community image is unavailable, the flat cream background works
on its own. This is much simpler than the Phase 13/14 community background
stack.

### R7: Expandable Panels as Animated Containers

The metric breakdowns and person expansions should use `AnimatedContainer` or
`AnimatedCrossFade` for smooth expand/collapse. Only one panel should be open
at a time — expanding a metric closes any open person panel, and vice versa.

### R8: Future Items Show Unread Badges

The original Coming Up section had no unread badges. The new design shows
them on future items too (visible in daily2b.png). This is critical for the
unread count consistency constraint.

---

## Open Questions

### Q1: Swipe-to-Ignore

The original plan included swipe-to-ignore (Phase 10d) for dismissing items
from the daily view. The new reference designs don't show this interaction.
**Should swipe-to-ignore be included in v2?** If so, it needs an
`IgnoreDailyItem` RPC and server-side storage for ignored items.

### Q2: Metric Breakdown Data Source

Where do per-item impact values (e.g., "$45 saved from Circular Saw") come
from? Is this data already computed and stored, or does it need a new
computation pipeline? The reference shows specific dollar amounts, time
contributions, and CO2 values per item — this suggests the server must be able
to attribute aggregate metrics back to individual items.

**⚠️ Blocker:** The `impact_saved`, `impact_time`, `impact_co2`, and
`impact_minutes` fields in `DailyItem` (proto fields 18–21) should be
**deferred until this data source is confirmed**. If the impact calculator
in `server/impact/` cannot attribute per-item values, remove these fields
from Phase 1 and add them in a later phase once the pipeline exists. Including
empty/stub fields in the proto from day one will mislead clients.

### Q3: Rich Minutes Breakdown Activities

The ring breakdown shows "completed activities this week" with quality minutes
per activity. Where does this data come from? Are these past transfers/
experiences that contributed to the Rich Minutes score? Is there a
`rich_minutes_per_activity` field or does the server need to reconstruct this
from activity history?

### Q4: Per-Item Impact Values When Metric Is Expanded

When a stat pill is tapped, each item row in the "All Today" section gains a
right-aligned impact value (visible in daily2c.png — "$25" next to an item
when "Saved" is expanded). This means the server needs to provide per-item
impact values for each metric dimension.

**⚠️ Deferred (see Q2):** Do not implement the impact overlay or per-item
impact proto fields until Q2 is resolved. The visual in daily2c.png is a
goal, not a Phase 1 requirement. Phase 1 should omit `impact_*` fields from
the proto and the impact overlay from the UI. Add them as a discrete phase
once the data source is established.

### Q5: People Row Ordering

The reference shows people sorted by most recent activity. Should people with
unread messages always sort first (regardless of activity recency)? Should
the current user appear in the row?

### Q6: Bottom Navigation Position

The original Phase 11d moved Daily to position 1 (next to Feed). The new
design doesn't specify a position. Should Daily be at:
- Position 1 (Feed, **Daily**, Discover, Metrics, Create)?
- Position 2 (Feed, Discover, **Daily**, Metrics, Create)?
- Position 3 (same as current inbox position)?

---

## Phased Implementation Plan

### Phase 1: Proto Definitions and Server RPC

**Goal:** Define the `GetDailyView` RPC and implement the server handler.

**Proto changes** (`proto/ripls/api/daily.proto` — new file):

```protobuf
syntax = "proto3";
package ripls.api;

service DailyService {
  rpc GetDailyView(GetDailyViewRequest) returns (GetDailyViewResponse);
}

message GetDailyViewRequest {
  string community_id = 1;
  string timezone = 2;  // IANA timezone, e.g. "America/Denver"
}

message GetDailyViewResponse {
  string date_label = 1;                // "Monday, Feb 23"
  WeeklyMetrics weekly_metrics = 2;
  repeated DailyPerson people = 3;      // Unique people from today's items
  repeated DailyItem timed_items = 4;   // Today, sorted earliest -> latest
  repeated DailyItem anytime_items = 5; // Today, sorted by type group then recency
  repeated DailyGroup coming_up = 6;    // Future items grouped by day
  int32 total_unread_count = 7;         // Must match bottom nav badge
}

message WeeklyMetrics {
  int32 rich_minutes = 1;
  int32 rich_minutes_goal = 2;         // 360
  string cost_saved = 3;               // Formatted: "$127"
  string time_recovered = 4;           // Formatted: "2.5h"
  string co2_avoided = 5;              // Formatted: "0.8kg"
  string week_label = 6;               // "This week . Feb 17 - 23"

  // Ring breakdown (completed activities this week)
  repeated MetricLineItem rich_minutes_breakdown = 7;
  int32 rich_minutes_remaining = 8;

  // Per-metric breakdowns
  repeated MetricLineItem cost_breakdown = 9;
  string cost_total = 10;              // "$127"
  string cost_summary = 11;            // "Your circle saved..."

  repeated MetricLineItem time_breakdown = 12;
  string time_total = 13;              // "2h 10m"
  string time_summary = 14;

  repeated MetricLineItem co2_breakdown = 15;
  string co2_total = 16;               // "0.8 kg"
  string co2_summary = 17;
}

message MetricLineItem {
  string emoji = 1;
  string title = 2;
  string amount = 3;                   // "+180", "$45", "0.4 kg"
  string note = 4;                     // "vs. buying new", "Monday"
}

message DailyItem {
  string id = 1;
  string title = 2;
  int64 time_unix_sec = 3;             // 0 if anytime
  string time_display = 4;             // "9:00 AM" or empty
  repeated DailyPerson people = 5;
  string action_label = 6;             // "RSVP", etc. For coming-up items only
  string state_label = 7;              // "You're going", etc. For coming-up items
  string conversation_id = 8;
  int32 unread_count = 9;
  DailyItemType item_type = 10;
  string item_id = 11;
  string type_label = 12;              // "Help", "Events", "Lending", "Giving"
  string emoji = 13;                   // Item emoji for thumbnail
  string emoji_background = 14;        // CSS gradient or hex for thumbnail bg
  string last_message_sender = 15;     // First name of last commenter
  string last_message_text = 16;       // Truncated last message
  string last_message_ago = 17;        // "12m", "1h", "3h"

  // Per-item impact values (shown when a metric is expanded)
  string impact_saved = 18;            // "$25"
  string impact_time = 19;             // "30m"
  string impact_co2 = 20;              // "0.1kg"
  int32 impact_minutes = 21;           // 30 (for ring overlay)
}

message DailyPerson {
  string user_id = 1;
  string display_name = 2;
  string first_name = 3;
  string media_id = 4;
  int32 total_unread = 5;              // Sum of unread across this person's items
  repeated string item_ids = 6;        // IDs of this person's items in today
}

message DailyGroup {
  string label = 1;                    // "Tomorrow", "Friday", "This Weekend"
  repeated DailyItem items = 2;
}

enum DailyItemType {
  DAILY_ITEM_TYPE_UNSPECIFIED = 0;
  DAILY_ITEM_TYPE_TRANSFER = 1;
  DAILY_ITEM_TYPE_EXPERIENCE = 2;
  DAILY_ITEM_TYPE_REQUEST = 3;
}
```

**Server handler** (`server/services/daily/`):

File organization:
- `service.go`: Service struct, constructor (~50 lines)
- `daily_view.go`: `GetDailyView` handler — orchestrates queries and assembly
- `assembly.go`: Item assembly (time resolution, action classification,
  emoji/thumbnail mapping, comment preview, per-item impact)
- `metrics.go`: Weekly metrics + breakdowns via shared impact calculator
- `people.go`: Compute unique people from today's items with unread totals
- `daily_test.go`: Table-driven unit + DB integration tests

The handler:
1. Calls `auth.RequireAuth(ctx)` to get the authenticated user
2. Queries active transfers, experiences, requests for the user in the community
3. Resolves conversations and unread counts (using shared `IsItemDone` filter)
4. Assembles `DailyItem` for each (time, title, emoji, comment preview, impact)
5. Splits into timed (sorted ascending) and anytime (sorted by type group, then
   recency), enforcing: anything with unread > 0 appears
6. Builds coming-up groups with future items (including those with unread)
7. Computes weekly metrics with full breakdowns
8. Computes people list from today's items
9. Computes `total_unread_count` (must match `GetUnreadCounts`)

**Shared function extraction:**
- Extract `IsItemDone()` from `server/services/chat/conversations.go` to
  `server/conversation/state.go` so both chat and daily services can use it

**Tests:**
- Timed items sort ascending by time
- Anytime items grouped by type (Help, Events, Lending, Giving) then by recency
- Items with unread messages always included in today/coming-up
- `total_unread_count` matches sum of all per-item unread counts
- Terminal-state items excluded (using `IsItemDone`)
- Coming-up items group by day correctly
- People list has correct unread totals and item IDs
- Metric breakdowns have correct line items
- Empty community returns empty lists
- Timezone respected for "today" boundary

### Phase 2: Client Service, Repository, and ViewModel

**Goal:** Wire the RPC through client data and presentation layers.

**Files created:**
- `app/lib/services/daily_service.dart` — Connect-Go client
- `app/lib/data/repositories/daily_repository.dart` — Cached wrapper
- `app/lib/presentation/viewmodels/daily_view_model.dart` — Thin AsyncNotifier

**DailyService:**
- `getDailyView(communityId, timezone)` -> calls `GetDailyView` RPC

**DailyRepository:**
- Constructor with `CacheService(cacheManager, 'daily')`
- `getDailyView(communityId, timezone)` -> cached call
- `refresh(communityId)` -> invalidates `'view:$communityId'` cache key, re-fetches
- Cache key: `'view:$communityId'`

**Riverpod providers** (in `app/lib/services/providers.dart` or a new `daily_providers.dart`):
```dart
final dailyServiceProvider = Provider<DailyService>((ref) => DailyService(...));
final dailyRepositoryProvider = Provider<DailyRepository>((ref) =>
  DailyRepository(ref.watch(cacheManagerProvider), ref.watch(dailyServiceProvider)));
final dailyProvider = AsyncNotifierProvider.autoDispose<DailyNotifier, DailyViewData>(
  DailyNotifier.new);
```

**DailyViewData (Freezed):**
`AsyncNotifier<GetDailyViewResponse>` cannot hold UI state alongside async loading/error
states. Instead, use a Freezed wrapper (matching the `TimeModalData` pattern):

```dart
@freezed
class DailyViewData with _$DailyViewData {
  const factory DailyViewData({
    required GetDailyViewResponse response,
    String? expandedMetric,    // null | "ring" | "saved" | "time" | "co2"
    String? expandedPersonId,  // userId, null when no person expanded
  }) = _DailyViewData;
}
```

**DailyNotifier:**
- `AsyncNotifier<DailyViewData>` with `autoDispose`
- `build()`: reads `selectedCommunityProvider` and device timezone, calls repository,
  returns `DailyViewData(response: response)`
- `refresh()`: invalidates cache, re-fetches (with `ref.mounted` checks after await)
- `toggleMetric(String metric)`: sets `expandedMetric` (same value → null), clears `expandedPersonId`
- `togglePerson(String userId)`: sets `expandedPersonId` (same value → null), clears `expandedMetric`
- Expanding one closes the other via the toggle methods above

**`expandedPersonId` uses `userId`** (not display name — names are not unique).

**Tests:**
- `build()` returns data from repository
- `refresh()` invalidates cache and re-fetches
- Error handling surfaces as `AsyncError`
- Disposal safety: disposed during async completes without throwing
- `toggleMetric` expands the correct metric, collapses on re-tap, clears person
- `togglePerson` expands the correct person, collapses on re-tap, clears metric

### Phase 3: Date Header, Ring, and Stat Pills

**Goal:** Build the top portion of the screen: date header with integrated
ring, and tappable stat pills with expandable breakdown panels.

**Files created:**
- `app/lib/presentation/widgets/daily/daily_header.dart` — Date + ring row
- `app/lib/presentation/widgets/daily/daily_stat_pills.dart` — Three tappable tiles + breakdown panel
- `app/lib/presentation/widgets/daily/daily_ring.dart` — Circular progress ring `CustomPainter`
- `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` — Expandable glass card with line items

**DailyHeader:**
- Row: left (TODAY label + date) + right (ring)
- Ring: `CustomPainter` with coral stroke on light track, center text
- Ring tappable: toggles "ring" expanded state in viewmodel

**DailyStatPills:**
- Row of three `Expanded` tiles
- Each tile tappable: toggles that metric expanded state in viewmodel
- Selected state: brighter background, accent border
- Below: `AnimatedCrossFade` or `AnimatedSize` showing breakdown panel

**DailyMetricBreakdown:**
- Glass card with header, line items, footer
- Accepts `MetricLineItem` list, total, summary from proto
- Rich Minutes variant: shows "This week so far" + subtotal row

**Tests:**
- Ring displays correct minutes and percentage
- Stat pills render all three with values
- Tapping a pill expands the correct breakdown
- Tapping the same pill collapses it
- Only one panel open at a time (ring or pill, not both)
- Breakdown panel shows correct line items
- Ring breakdown shows activities with +minutes values

### Phase 4: People Row

**Goal:** Build the horizontally scrollable people row with tap-to-expand.

**Files created:**
- `app/lib/presentation/widgets/daily/daily_people_row.dart` — Horizontal scroll of avatar circles
- `app/lib/presentation/widgets/daily/daily_person_panel.dart` — Expanded person's items as horizontal card scroll

**DailyPeopleRow:**
- `ListView.builder` with `scrollDirection: Axis.horizontal`
- Each person: avatar (50px, gradient, ring if unread) + first name
- Tappable: toggles person expanded state in viewmodel
- Selected person: name text in coral

**DailyPersonPanel:**
- `AnimatedSize` wrapper for smooth expand/collapse
- Glass card with horizontal card scroll inside
- Each item card: emoji thumbnail (100px) + type label + time + title + comment
- Single item: full width; multiple: 200px, scrollable

**Tests:**
- People row shows unique people from today's items
- Avatar rings appear on people with unread messages
- Tapping person expands panel with their items
- Tapping again collapses
- Expanding person closes any open metric panel
- Person cards show correct emoji, type, title, comment

### Phase 5: Today Items — Thumbnails and Rows

**Goal:** Build the main item list with emoji thumbnails, comment previews,
and unread badges.

**Files created:**
- `app/lib/presentation/widgets/daily/daily_item_row.dart` — Single item row with thumbnail
- `app/lib/presentation/widgets/daily/daily_emoji_thumbnail.dart` — 48px emoji square with gradient bg

**DailyItemRow:**
- `ConsumerWidget` (needs `UserAvatar` for people)
- Layout: [thumbnail 48px] + [title + comment] + [impact value?] + [unread + ago]
- Tappable: opens conversation screen
- Shows per-item impact value when a metric is expanded

**DailyEmojiThumbnail:**
- 48px square, 12px border radius
- Gradient background (from `emoji_background` field)
- Emoji centered (18px)

**Section rendering in DailyScreen:**
- "ALL TODAY" section label
- Timed items: each preceded by "9:00 AM . Help" type+time label
- Anytime items: grouped by type with "Help", "Events", "Lending", "Giving" headers
- Empty type groups omitted

**Tests:**
- Item row renders thumbnail, title, comment preview, unread badge
- Unread badge hidden when count is 0
- Comment row hidden when no last message
- Time+type label shown above timed items
- Anytime items grouped by type in correct order
- Per-item impact value shown when metric is expanded, hidden otherwise
- Tapping row navigates to conversation screen

### Phase 6: Coming Up Section

**Goal:** Build the future items section with simple line rows.

**Files created:**
- `app/lib/presentation/widgets/daily/daily_future_row.dart` — Compact line item

**DailyFutureRow:**
- [20px avatar] + [title] + [action chip or state text] + [unread badge]
- Action chip: coral-tinted background, coral text, 6px radius
- State text: 10px, w600, green
- Unread badge: 20px circle, coral background (same as today items)
- Bottom border between rows

**Section rendering:**
- "COMING UP" header with top border separator
- Day group labels: "Tomorrow", "Friday", "This Weekend", "Next Week"
- `DailyFutureRow` for each item

**Tests:**
- Future row renders avatar, title, state/action, unread badge
- Day groups render with correct labels
- Unread badge visible on future items with unread > 0
- Action chip and state text mutually exclusive

### Phase 7: Daily Screen Assembly and Navigation

**Goal:** Assemble all components into `DailyScreen`, wire into bottom nav.

**Files created:**
- `app/lib/presentation/screens/daily/daily_screen.dart`

**Files changed:**
- `app/lib/presentation/screens/home/home_screen.dart` — Replace inbox tab
- `app/lib/presentation/viewmodels/home_view_model.dart` — Update nav keys

**DailyScreen:**
- `ConsumerStatefulWidget` (tab screen, no `SwipeToCloseMixin`)
- Background: community image with warm cream overlay + blur (subtle) on flat
  `#FDF8F0` fallback
- `CustomScrollView` or `ListView` with section builder methods:
  - `_buildHeader()` — DailyHeader widget
  - `_buildStatPills()` — DailyStatPills widget
  - `_buildPeopleRow()` — DailyPeopleRow widget
  - `_buildTodayItems()` — section label + timed items + anytime groups
  - `_buildComingUp()` — section label + day groups
- `RefreshIndicator` wrapping content
- `NotificationListener<ScrollNotification>` for hide/show floating header + nav

**Navigation wiring:**
- Replace inbox tab with daily tab (sun icon)
- Move to appropriate nav position (pending Q6 resolution)
- Keep unread badge on daily tab (same `UnreadCountNotifier`)
- Item taps -> conversation screen
- Metrics tap -> community metrics screen (optional)

**Tests:**
- Daily screen renders all sections with mock response
- Pull-to-refresh calls `refresh()`
- Unread badge on nav tab shows count
- Item taps navigate to conversation
- Empty state shows friendly message
- Loading state shows skeleton/shimmer

### Phase 8: Real-Time Updates and Polish

**Goal:** Wire real-time unread updates, handle edge cases, polish.

**Files changed:**
- `app/lib/presentation/viewmodels/daily_view_model.dart` — Stream subscriptions
- `app/lib/main.dart` — Notification handler for daily tab

**Real-time updates:**
- Subscribe to message streams for conversations in the daily view
- Update unread counts locally (optimistic) without full RPC re-call
- Register cleanup via `ref.onDispose()`

**Notification navigation:**
- Notification tap: switch to daily tab, push conversation onto daily navigator

**Tab-focus refresh (required for cache staleness):**
- `DailyScreen` implements `WidgetsBindingObserver` and calls
  `ref.read(dailyProvider.notifier).refresh()` when `AppLifecycleState.resumed`
  is detected (app returning from background)
- Community switch auto-refreshes because `build()` watches `selectedCommunityProvider`

**Edge cases:**
- Empty state: warm message + suggestion when no items
- Error state: error message + retry button
- Date rollover: detect midnight, trigger refresh
- "N more conversations" overflow row if visible unreads < total_unread_count

**Accessibility:**
- Semantic labels on all interactive elements
- Sufficient contrast for all text on warm background

**Tests:**
- Unread badges update on stream message without full refresh
- Notification tap navigates correctly
- Stream subscriptions cancelled on dispose
- Overflow row appears/hides correctly
- Date rollover triggers refresh

### Phase 9: UI Fixes — Metric Breakdowns, Person Panel, Item Row Status

**Goal:** Fix four issues with the current implementation:
1. Tapping stat pills does nothing visible — add expandable breakdown panels
2. Person panel cards overflow and don't match the reference design
3. Today items show status text (Hosting, Waiting, etc.) even with no unread —
   should only show status when there are no unread AND no comment
4. Stat pill cards are too tall and should have white backgrounds

**Reference materials:**
- [daily2c.png](../../tmp/daily/daily2c.png) — Expanded metric breakdown
  (tapping "Saved" pill shows itemized list in a glass card below the pills)
- [daily2d.png](../../tmp/daily/daily2d.png) — Expanded person panel (tapping
  a person shows their items as visual cards with thumbnail images, type labels,
  titles, and comment previews)

---

#### 9a: Stat Pill Restyling

The stat pills (`daily_stat_pills.dart`) are currently too tall and use a cream
background. Per the reference (daily2c.png), they should be more compact with
white backgrounds.

**Changes to `_StatPill`:**

| Property | Current | New |
|----------|---------|-----|
| Background color | `#F8F3EE` (cream) | `rgba(255,255,255,0.8)` (white glass) |
| Vertical padding | 10px | 4px |
| Border (default) | transparent | `1px solid border-color` (`rgba(214,207,196,0.5)`) |
| Border (selected) | 1.5px coral | 1px coral |
| Value font size | 14px | 13px |
| Label font size | 9px | 7px |

The pills should feel like compact indicator chips, not tall cards.

**Files changed:**
- `app/lib/presentation/widgets/daily/daily_stat_pills.dart`

---

#### 9b: Metric Breakdown Panel

When a stat pill is tapped, an expandable glass card appears below the pills
row showing an itemized breakdown. The ring tap also shows a breakdown. Per the
reference (daily2c.png):

**Panel layout (glass card):**
- Background: `rgba(255,255,255,0.85)`, border: `1px solid border-color`,
  14px radius, subtle shadow `0 2px 12px rgba(0,0,0,0.05)`
- Header section (border-bottom): metric label + total in accent color +
  summary sentence
- Line items: emoji (18px, 28px width) + title (12px, w600) + note (10px,
  muted) + amount (13px, w700, accent color, right-aligned)
- Footer: "This week . Feb 17 - 23" centered (9px, w600, muted)

**Ring breakdown variant:**
- Header: "215 / 360 min goal" + "Quality time with friends this week.
  X min to go."
- Subsection label: "This week so far" (9px, w700, muted, uppercase)
- Line items: completed activities with +minutes in green
- Subtotal row: "Total so far" + total minutes in accent

**Expand/collapse behavior:**
- `AnimatedSize` wrapper around the panel
- Only one panel visible at a time (ring or a pill, not both) — already handled
  by viewmodel's `toggleMetric` mutual exclusion
- Tapping the active pill again collapses it

**Proto dependency:** The current `WeeklyMetrics` proto does NOT have breakdown
fields (`MetricLineItem`, `cost_breakdown`, etc.). Until those are added to the
proto and server, the breakdown panel should display using **placeholder/mock
data** or show the summary text from `encouragementHeadline` /
`encouragementSubtitle`. Once the proto is extended with breakdown fields
(a separate proto+server task), the panel will render real data.

**Interim approach:** Build the panel widget to accept a list of breakdown
line items. Wire it to render from proto data when available, or show a
simplified view (just the header with total + summary) when breakdown data
is empty.

**Files created:**
- `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` — Glass
  card breakdown panel widget

**Files changed:**
- `app/lib/presentation/screens/daily/daily_screen.dart` — Insert breakdown
  panel as a `SliverToBoxAdapter` below the stat pills, conditionally shown
  when `expandedMetric` is non-null

---

#### 9c: Person Panel Redesign

The person panel (`daily_person_panel.dart`) has two issues:
1. **RenderFlex overflow** — the fixed 120px height can't accommodate cards
   with two-line titles + type label + time + thumbnail
2. **Doesn't match reference** — daily2d.png shows larger cards with actual
   thumbnail images (not just emoji), full titles, and comment previews in
   a more spacious layout

**Reference layout (daily2d.png):**
- Outer container: 18px border radius, `rgba(255,255,255,0.75)` background,
  blur(16px), subtle shadow
- Horizontal card scroll with 12px padding
- Each card:
  - Width: 200px (or 100% if single item)
  - Thumbnail image area: full width, 100px height, emoji on colored gradient
    background (or actual media if `backgroundMediaId` is available)
  - Content padding: 10px horizontal, 10px vertical
  - Type label + time row: type in accent (9px, w700), time in muted (9px, w600)
  - Title: 13px, w700, primary text, up to 2 lines
  - Comment preview: "FirstName . comment text" — 11px, secondary text
  - Card background: white, 12px radius, subtle shadow

**Key changes from current implementation:**
- Remove fixed `SizedBox(height: 120)` — let cards size to content
- Card width: 110px → 200px (or full width for single item)
- Add thumbnail image area (100px tall)
- Add comment preview below title
- Container gets glass styling (background, border, shadow)
- Use `ConstrainedBox` with `maxHeight` if needed to prevent unbounded growth,
  but allow enough room for content

**Files changed:**
- `app/lib/presentation/widgets/daily/daily_person_panel.dart` — Complete
  rewrite of card layout and container sizing

---

#### 9d: Today Item Row — Status Display Logic

The current `DailyItemRow` shows status (state label or action label) as a
trailing chip whenever there's no unread badge. This clutters the view with
"Hosting", "Waiting", etc. on items that don't need attention.

**New rules for today's items:**

1. If `unreadCount > 0`: show unread badge + time-ago (unchanged)
2. If `unreadCount == 0` AND has comment (`lastMessageText` non-empty): show
   comment preview in the subtitle row, **no status chip** in trailing
3. If `unreadCount == 0` AND no comment AND has status (`stateLabel` or
   `actionLabel`): show status text **in the subtitle position** (where
   the comment would go), not as a trailing chip
4. If nothing: just show title, nothing else

In short: the trailing area only shows an unread badge (or nothing). Status
text moves to the subtitle row as a fallback when there's no comment.

**Files changed:**
- `app/lib/presentation/widgets/daily/daily_item_row.dart` — Restructure
  `_buildTrailing()` to only show unread badge, move status to subtitle row

---

#### 9e: Summary of All Changes

**Files created:**
| File | Purpose |
|------|---------|
| `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` | Expandable glass card for metric breakdowns |

**Files changed:**
| File | Change |
|------|--------|
| `app/lib/presentation/widgets/daily/daily_stat_pills.dart` | Compact sizing, white bg, thinner borders |
| `app/lib/presentation/widgets/daily/daily_person_panel.dart` | Larger cards with images, comment previews, fix overflow |
| `app/lib/presentation/widgets/daily/daily_item_row.dart` | Status moves to subtitle, trailing only shows unread badge |
| `app/lib/presentation/screens/daily/daily_screen.dart` | Insert metric breakdown panel below stat pills |

**Tests:**
- Widget: tapping stat pill shows breakdown panel below pills
- Widget: tapping same pill again collapses breakdown
- Widget: tapping different pill swaps breakdown content
- Widget: ring breakdown shows "This week so far" with activity list
- Widget: breakdown panel shows summary when no line items available
- Widget: person panel cards don't overflow (no RenderFlex error)
- Widget: person panel cards show thumbnail, type, title, comment
- Widget: single-item person panel card takes full width
- Widget: today item with unread shows badge, no status chip
- Widget: today item with no unread and comment shows comment, no status
- Widget: today item with no unread, no comment, but status shows status in subtitle position
- Widget: today item with nothing shows only title

### Phase 10: Floating Header, Scroll-Hide Nav, Tab Reorder, Item Navigation

**Goal:** Add a floating header matching the feed screen pattern, hide/show top
and bottom nav on scroll and tap, change the daily tab icon, reorder the bottom
nav tabs, and change item tap behavior to open item screens instead of
conversation screens.

**Reference patterns:**
- `app/lib/presentation/screens/feed/feed_screen.dart` — `NotificationListener<ScrollNotification>` scroll detection, `AnimatedSlide` header, `GestureDetector onTap: toggleNav()`
- `app/lib/presentation/screens/home/home_screen.dart` — `AnimatedSlide` bottom nav, `IndexedStack`, tab order, selection circle animation
- `app/lib/presentation/widgets/floating_header.dart` — `FloatingHeader` widget (48px, pill shape, semi-transparent glass, `contentTop`/`contentBottom` static helpers)
- `app/lib/presentation/viewmodels/home_view_model.dart` — `isNavVisible`, `showNav()`/`hideNav()`/`toggleNav()`, navigator keys, scroll controller providers, `_scrollToTop()`

---

#### 10a: Floating Header and Scroll-Hide Navigation

Add the same floating header + scroll-driven nav visibility pattern used by the
feed screen.

**Scroll detection** — wrap the daily screen's `CustomScrollView` in a
`NotificationListener<ScrollNotification>`:
- On `ScrollUpdateNotification`, check `scrollDelta`:
  - `scrollDelta > 10` -> `ref.read(homeProvider.notifier).hideNav()` (scrolling down)
  - `scrollDelta < -10` -> `ref.read(homeProvider.notifier).showNav()` (scrolling up)
- Ignore negative `metrics.pixels` (pull-to-refresh region)
- Return `false` to allow notification bubbling

**Tap-to-toggle** — wrap the entire scrollable area in a `GestureDetector`:
- `onTap: () => ref.read(homeProvider.notifier).toggleNav()`
- Tapping the screen content toggles header/nav visibility (same as feed)

**Floating header** — positioned above the scroll content:
```dart
Positioned(
  top: MediaQuery.of(context).padding.top,
  left: 0,
  right: 0,
  child: AnimatedSlide(
    duration: const Duration(milliseconds: 200),
    offset: ref.watch(homeProvider).isNavVisible
        ? Offset.zero
        : const Offset(0, -3),
    child: FloatingHeader(
      title: communityName,
      // Same trailing widget pattern as feed screen
    ),
  ),
)
```

**Content padding** — use `FloatingHeader.contentTop(context)` for top padding
and `FloatingHeader.contentBottom(context)` for bottom padding so content
doesn't sit behind the floating header or bottom nav.

**Screen structure change:** The current `Scaffold > SafeArea > RefreshIndicator >
CustomScrollView` wrapping needs to become a `Stack` with the background, the
scrollable content, and the floating header layered on top:
```dart
Scaffold(
  backgroundColor: _bg,
  body: Stack(
    children: [
      // Layer 1: scrollable content with padding for header/nav
      NotificationListener<ScrollNotification>(
        onNotification: _handleScroll,
        child: GestureDetector(
          onTap: () => ref.read(homeProvider.notifier).toggleNav(),
          child: RefreshIndicator(
            onRefresh: ...,
            child: CustomScrollView(
              // First sliver has top padding for header
              // Last sliver has bottom padding for nav
            ),
          ),
        ),
      ),
      // Layer 2: floating header
      Positioned(
        top: MediaQuery.of(context).padding.top,
        left: 0, right: 0,
        child: AnimatedSlide(..., child: FloatingHeader(...)),
      ),
    ],
  ),
)
```

Remove `SafeArea` wrapper — the floating header handles safe area positioning,
and `FloatingHeader.contentTop(context)` accounts for safe area in the padding.

**Bottom nav** — already handled by `HomeScreen` via `AnimatedSlide` with
`Offset(0, 1.5)` when `isNavVisible` is false. Triggering `hideNav()`/
`showNav()` from the daily screen's scroll listener is sufficient.

**Files changed:**
- `app/lib/presentation/screens/daily/daily_screen.dart` — Add `NotificationListener`,
  `GestureDetector`, `Stack` with `FloatingHeader` + `AnimatedSlide`, update content
  padding, remove `SafeArea`

---

#### 10b: Bottom Navigation Tab Icon

Change the daily screen tab icon from `CupertinoIcons.sun_max` /
`CupertinoIcons.sun_max_fill` to `Icons.today_rounded` (both selected and
unselected states).

**Files changed:**
- `app/lib/presentation/screens/home/home_screen.dart` — Update icon in nav item

---

#### 10c: Bottom Navigation Tab Reorder

Reorder the bottom navigation tabs. The current order (based on the
`IndexedStack` and nav builder) needs to become:

| Position | Screen | Icon |
|----------|--------|------|
| 0 | Feed | `Icons.home` (unchanged) |
| 1 | Daily | `Icons.today_rounded` |
| 2 | Discover | `Icons.place` |
| 3 | Metrics | `RiplsIcons.ripls` |
| 4 | Add button | `Icons.add` (unchanged) |

Daily moves to position 1 (second tab, next to feed). Discover and Metrics
shift right by one position.

**Changes to `HomeScreen`:**
- Reorder the nav items in the bottom navigation builder
- Update the `AnimatedPositioned` selection circle positions to match new indices
- Update the `IndexedStack` children order to match:
  - Index 0 -> `FeedScreen`
  - Index 1 -> Navigator -> `DailyScreen`
  - Index 2 -> Navigator -> `DiscoverScreen`
  - Index 3 -> Navigator -> `MetricsScreen`
- Move the unread badge from its current position to position 1 (Daily tab)
- Fix any index mapping in `_onBottomNavTap()` — with the new order, visual
  position should match internal index directly (remove any index swap workaround)

**Changes to `HomeViewModel`:**
- Update `_scrollToTop()` switch cases to match new index mapping
- Update any `selectedIndex` default or initial values if needed

**Files changed:**
- `app/lib/presentation/screens/home/home_screen.dart` — Reorder nav items,
  update index mapping, update `IndexedStack` order, move unread badge
- `app/lib/presentation/viewmodels/home_view_model.dart` — Update
  `_scrollToTop()` switch cases

---

#### 10d: Item Navigation — Open Item Screens Instead of Conversations

Currently, tapping a `DailyItemRow` calls `_onItemTap(conversationId)` which
opens the conversation screen. The new behavior should open the **item content
screen** (GearScreen, ExperienceScreen, RequestScreen) based on the item type.

**Navigation mapping:**

| `DailyItemType` | Screen | ID needed |
|-----------------|--------|-----------|
| `DAILY_ITEM_TYPE_TRANSFER` | `GearScreen(gearId: gearId)` | gear ID (not transfer ID) |
| `DAILY_ITEM_TYPE_EXPERIENCE` | `ExperienceScreen(experienceId: itemId)` | experience ID |
| `DAILY_ITEM_TYPE_REQUEST` | `RequestScreen(requestId: itemId)` | request ID |

**Problem:** For transfers, `DailyItem.item_id` contains the transfer ID, but
`GearScreen` requires a gear ID. The existing `NavigationHelpers.pushToItemScreen`
expects the ID to be the content ID (gear, experience, request), not the transfer
ID.

**Solution — Proto change:** Add a `string content_id` field to `DailyItem` that
always contains the navigable content ID:
- For transfers: the gear ID (server resolves `transfer.gear_id`)
- For experiences: the experience ID (same as `item_id`)
- For requests: the request ID (same as `item_id`)

Alternatively, rename the existing `item_id` to carry the content ID and add a
separate `transfer_id` if needed. The key requirement is that the client can
navigate to the item screen without an extra server call.

**Navigation implementation:**
```dart
void _onItemTap(DailyItem item) {
  final navigatorKey = ref.read(dailyNavigatorKeyProvider);
  final navigator = navigatorKey.currentState;
  if (navigator == null) return;

  final Widget screen;
  switch (item.itemType) {
    case DailyItemType.DAILY_ITEM_TYPE_TRANSFER:
      screen = GearScreen(gearId: item.contentId);
    case DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE:
      screen = ExperienceScreen(experienceId: item.contentId);
    case DailyItemType.DAILY_ITEM_TYPE_REQUEST:
      screen = RequestScreen(requestId: item.contentId);
    default:
      return;
  }
  NavigationHelpers.pushScreen(context: context, screen: screen);
}
```

This replaces the current `_onItemTap(String conversationId)` with
`_onItemTap(DailyItem item)`. The conversation screen is still accessible from
within the item screen (existing navigation pattern).

**Proto change** (`proto/ripls/api/daily.proto`):
```protobuf
message DailyItem {
  // ... existing fields ...

  // Navigable content ID. For transfers this is the gear_id; for experiences
  // and requests this equals item_id. Used by the client to open the correct
  // content screen without an extra lookup.
  string content_id = 19;
}
```

**Server change** (`server/services/daily/assembly.go`):
- When assembling a transfer `DailyItem`, set `content_id = transfer.gear_id`
- When assembling an experience or request, set `content_id = item.id`

**Files changed:**
- `proto/ripls/api/daily.proto` — Add `string content_id` to `DailyItem`
- `server/services/daily/assembly.go` — Populate `content_id` during assembly
- `app/lib/presentation/screens/daily/daily_screen.dart` — Change `_onItemTap`
  to accept `DailyItem`, navigate by type to GearScreen/ExperienceScreen/
  RequestScreen using `contentId`
- `app/lib/presentation/widgets/daily/daily_person_panel.dart` — Update
  `onItemTap` callback to pass `DailyItem` instead of conversation ID

---

#### 10e: Summary of All Changes

**Proto changes:**
| Field | Message | Purpose |
|-------|---------|---------|
| `string content_id = 19` | `DailyItem` | Navigable content ID (gear_id for transfers) |

**Server changes:**
| File | Change |
|------|--------|
| `server/services/daily/assembly.go` | Populate `content_id` (gear_id for transfers, item ID for others) |

**Client changes:**
| File | Change |
|------|--------|
| `app/lib/presentation/screens/daily/daily_screen.dart` | Add floating header with `AnimatedSlide`, scroll-hide nav via `NotificationListener`, tap-to-toggle via `GestureDetector`, replace `SafeArea` with `Stack` layout, add content padding for header/nav, change item tap to open item screens |
| `app/lib/presentation/screens/home/home_screen.dart` | Reorder nav: Feed/Daily/Discover/Metrics/Add, change daily icon to `Icons.today_rounded`, update index mapping and `IndexedStack`, move unread badge to position 1 |
| `app/lib/presentation/viewmodels/home_view_model.dart` | Update `_scrollToTop()` switch cases for new tab indices |
| `app/lib/presentation/widgets/daily/daily_person_panel.dart` | Update `onItemTap` to pass `DailyItem` instead of conversation ID |

**Tests:**
- Widget: floating header visible on initial load
- Widget: floating header hides on scroll down (delta > 10)
- Widget: floating header reappears on scroll up (delta < -10)
- Widget: tapping screen content toggles header/nav visibility
- Widget: bottom nav hides/shows in sync with header
- Widget: content has correct top/bottom padding for header and nav
- Widget: daily tab icon is `Icons.today_rounded`
- Widget: bottom nav tabs in order Feed, Daily, Discover, Metrics, Add
- Widget: tapping position 1 shows daily screen
- Widget: unread badge appears on position 1 (Daily tab)
- Widget: tapping a transfer item opens `GearScreen` with correct gear ID
- Widget: tapping an experience item opens `ExperienceScreen`
- Widget: tapping a request item opens `RequestScreen`
- Widget: person panel card tap opens item screen (not conversation)
- Server: `content_id` is gear_id for transfer items
- Server: `content_id` equals item_id for experience and request items

---

### Phase 11: Shared Value, Quality Time Inline, Your Actions Today

**Goal:** Add a "Shared Value" display to the header, move the quality time ring
inline with the stat pills, add a "Your Actions Today" section showing real
actions the user took today with per-action impact values, and add a tappable
expanded inventory list for the shared value.

**Reference materials:**
- [daily2e.png](../../tmp/daily/daily2e.png) — Updated daily screen: shared value
  in upper right, quality time inline with stat pills, "Your Actions Today"
  section with per-action impact breakdown, "Needs Attention" section below
- [daily2f.png](../../tmp/daily/daily2f.png) — Expanded shared value panel showing
  inventory list with item names, status, and individual values
- [daily2.jsx](../../tmp/daily/daily2.jsx) — Updated React reference

---

#### 11a: Header Layout — Shared Value + Inline Quality Time

Redesign the header area. Currently the date header sits left and the quality
time ring sits right at the same level. The new layout adds a **shared value**
display in the upper right and moves the quality time ring **down** to sit
inline with the stat pills.

**New header layout:**
```
+----------------------------------------+
| TODAY                    $1,247         |
| Monday, Feb 23        SHARED VALUE     |
+----------------------------------------+
| [215 ring] [$127] [2.5h] [0.8kg]      |
|  qual min   SAVED  TIME    CO2         |
+----------------------------------------+
```

**Shared value display** — top right, aligned with the date:
- Value: large text (e.g. `$1,247`), coral color, Georgia serif, w800
- Label: `SHARED VALUE` below value, small caps, muted color, 7px
- Tappable — toggles the inventory breakdown panel (see 11c)

**Quality time ring** — moves from the header row to the stat pills row:
- Sits as the first element in the pills row (leftmost position)
- Same ring design: circular SVG with progress arc, centered value + "QUALITY
  MIN" label
- Smaller size to fit the row: ~56px diameter (down from 80px)
- Still tappable — toggles the ring breakdown panel (existing behavior)

**Impact on existing widgets:**
- `DailyHeader` — remove the ring from the header, add shared value display
  on the right. Accept `sharedValue` (formatted string) and `onValueTap`
  callback.
- `DailyStatPills` — add the quality time ring as the first element in the row.
  The ring and three pills form a 4-element horizontal strip.

---

#### 11b: Your Actions Today Section

Add a new section between the stat pills area and the people row. This shows
**real actions** the user took today — things they did, not things that happened
to them. Only concrete actions qualify:

**Qualifying actions:**
- **Shared an item** (created a transfer as lender/giver): "Lent Pressure Washer
  to Gary", "Gave away Old camping stove to Derek"
- **Offered help** (responded to a request): "Helped Thomas carry luggage"
- **RSVP'd to an event** (confirmed attendance): "RSVP'd Skiing at Breck
  tomorrow"

**Not qualifying:**
- Commenting on things (not a real action)
- Receiving items (not the user's action)
- Browsing or viewing (passive)

**Per-action impact values** — each action shows its impact values inline:
```
🔧 Lent Pressure Washer to Gary
   Saved $65 · +11m quality time · 0.6kg CO₂ avoided
```

Impact fields per action: cost saved, quality time earned, CO₂ avoided. Only
show non-zero/non-empty values. The format uses `·` separators.

**Section layout:**
```
+----------------------------------------+
| YOUR ACTIONS TODAY                     |
|                                        |
| 🔧 Lent Pressure Washer to Gary       |
|    Saved $65 · +11m quality time ·     |
|    0.6kg CO₂ avoided                  |
|                                        |
| ⛷️ RSVP'd Skiing at Breck tomorrow    |
|    Saved $40 · 0.8kg CO₂ avoided      |
|                                        |
| 🧳 Helped Thomas carry luggage        |
|    Saved $25 · +30m quality time ·     |
|    0.1kg CO₂ avoided                  |
|                                        |
| 🏕️ Gave away Old camping stove        |
|    +10m quality time · 0.3kg CO₂      |
+----------------------------------------+
```

Each row: emoji on the left, title + impact below. No trailing badges or
navigation — these are accomplishments, not actionable items. The section
header uses the same `_buildSectionHeader` uppercase muted style.

**Empty state:** If the user has no actions today, omit the section entirely
(no empty state message — the section just doesn't appear).

---

#### 11c: Shared Value Expanded Panel — Inventory List

Tapping the shared value display in the header toggles an expanded panel that
shows the user's full sharing inventory with individual values. This is an
**AnimatedSize** panel that appears below the header, pushing content down
(same pattern as metric breakdown panels).

**Panel design** (from daily2f.png):
```
+----------------------------------------+
| Your shared items are worth $1,247     |
| Total value of everything you're       |
| sharing with friends.                  |
+----------------------------------------+
| Pressure Washer              $320      |
| Lent to Gary                           |
|----------------------------------------|
| Stand Mixer                  $280      |
| Lent to Betty                          |
|----------------------------------------|
| Camping Tent                 $245      |
| Lent to Gary                           |
|----------------------------------------|
| Circular Saw                 $189      |
| Lent to Emma                           |
|----------------------------------------|
| Tool Set                     $135      |
| Available                              |
|----------------------------------------|
| Camera Tripod                $78       |
| Available                              |
+----------------------------------------+
```

**Panel details:**
- Glass container: same style as metric breakdown panels
  (`rgba(255,255,255,0.85)`, 14px radius, border, shadow)
- Header: bold title with value in coral, subtitle in secondary text
- Each row: item name (left, w600) + value (right, w700 coral, Georgia serif)
- Status line below name: "Lent to [name]" or "Available" in muted text
- Rows separated by `1px solid border` dividers
- No icons (per user requirement) — just text rows
- Sorted by value descending

**State toggle:** Add `expandedValue` (bool) to the viewmodel state. Tapping
the shared value display toggles this. When expanded, the inventory panel shows
below the header/date row and above the stat pills row. Collapsing the panel
(re-tapping or tapping elsewhere) hides it with AnimatedSize.

---

#### 11d: Proto Changes

Add new fields and messages for actions today and shared value.

**New messages in `proto/ripls/api/daily.proto`:**

```protobuf
// DailyAction represents a real action the user took today.
message DailyAction {
  // Human-readable title, e.g. "Lent Pressure Washer to Gary".
  string title = 1;

  // Emoji for visual display.
  string emoji = 2;

  // Formatted cost saved by this action, e.g. "$65". Empty if zero.
  string cost_saved = 3;

  // Formatted quality time earned, e.g. "+11m". Empty if zero.
  string quality_time = 4;

  // Formatted CO₂ avoided, e.g. "0.6kg". Empty if zero.
  string co2_avoided = 5;
}

// SharedItem represents a single item in the user's sharing inventory.
message SharedItem {
  // Item name, e.g. "Pressure Washer".
  string name = 1;

  // Formatted value, e.g. "$320".
  string value = 2;

  // Status line, e.g. "Lent to Gary" or "Available".
  string status = 3;
}
```

**New fields on `GetDailyViewResponse`:**

```protobuf
message GetDailyViewResponse {
  // ... existing fields 1-6 ...

  // Formatted total value of user's shared inventory, e.g. "$1,247".
  string shared_value = 7;

  // Actions the user took today (shares, help offered, RSVPs).
  repeated DailyAction actions_today = 8;

  // User's sharing inventory for the expanded value panel, sorted by value
  // descending.
  repeated SharedItem shared_items = 9;
}
```

**Server assembly** — new functions in a `server/services/daily/actions.go`
file:
- `assembleActionsToday(ctx, store, userID, communityID, dayStart, dayEnd)` —
  queries today's transfers where user is lender/giver, requests where user
  is helper, and experiences where user RSVP'd today. Formats each as a
  `DailyAction` with impact values.
- `assembleSharedInventory(ctx, store, userID, communityID)` — queries all
  active transfers where user is lender plus all gear the user has listed as
  available. Sums values for `shared_value`, builds `SharedItem` list sorted
  by value descending.

---

#### 11e: Screen Layout Order Update

The section order in `_buildScrollView` changes. Insert the new sections:

```
1. Top padding (FloatingHeader.contentTop)
2. Date header + shared value display    ← CHANGED (added shared value)
3. Shared value expanded panel           ← NEW (if expanded)
4. Stat pills row (with quality time)    ← CHANGED (ring moves here)
5. Metric breakdown panel                ← existing (if expanded)
6. Your Actions Today                    ← NEW (if non-empty)
7. People row                            ← existing
8. Person panel                          ← existing (if expanded)
9. Today section (Needs Attention)       ← existing
10. Hidden unread overflow row           ← existing
11. Coming Up section                    ← existing
12. Empty state                          ← existing
13. Bottom padding (FloatingHeader.contentBottom)
```

Note: The "Today" section header text may change to "NEEDS ATTENTION" to
distinguish it from "Your Actions Today". This is a cosmetic label change
only — the content and filtering logic remain the same.

---

#### 11f: New Widget Files

| Widget | File | Purpose |
|--------|------|---------|
| `DailySharedValue` | `daily_shared_value.dart` | Upper-right value display ($1,247 / SHARED VALUE) |
| `DailyValuePanel` | `daily_value_panel.dart` | Expanded inventory list (glass panel with item rows) |
| `DailyActionsToday` | `daily_actions_today.dart` | "Your Actions Today" section with action rows + impact values |

---

#### 11g: Summary of All Changes

**Proto changes:**
| Field/Message | Location | Purpose |
|---------------|----------|---------|
| `DailyAction` message | `daily.proto` | Action with title, emoji, and per-action impact values |
| `SharedItem` message | `daily.proto` | Inventory item with name, value, and status |
| `string shared_value = 7` | `GetDailyViewResponse` | Formatted total shared value |
| `repeated DailyAction actions_today = 8` | `GetDailyViewResponse` | Today's real actions |
| `repeated SharedItem shared_items = 9` | `GetDailyViewResponse` | Inventory for expanded panel |

**Server changes:**
| File | Change |
|------|--------|
| `server/services/daily/actions.go` | New — `assembleActionsToday`, `assembleSharedInventory` |
| `server/services/daily/daily_view.go` | Call new assembly functions, populate response fields |

**Client changes:**
| File | Change |
|------|--------|
| `app/lib/presentation/widgets/daily/daily_header.dart` | Remove ring, add shared value display with `onValueTap` |
| `app/lib/presentation/widgets/daily/daily_stat_pills.dart` | Add quality time ring as first element in pills row |
| `app/lib/presentation/widgets/daily/daily_shared_value.dart` | New — upper-right shared value display |
| `app/lib/presentation/widgets/daily/daily_value_panel.dart` | New — expanded inventory list panel |
| `app/lib/presentation/widgets/daily/daily_actions_today.dart` | New — actions today section with impact values |
| `app/lib/presentation/screens/daily/daily_screen.dart` | Add new sections to sliver list, add shared value panel, pass new props |
| `app/lib/presentation/viewmodels/daily_view_model.dart` | Add `expandedValue` toggle state |

**Tests:**
- Widget: shared value displays formatted total in upper right
- Widget: tapping shared value toggles inventory panel
- Widget: inventory panel shows items sorted by value descending
- Widget: inventory panel shows "Lent to [name]" or "Available" status
- Widget: quality time ring appears inline with stat pills
- Widget: "Your Actions Today" section shows only real actions (shares, help, RSVPs)
- Widget: action rows display per-action impact values with `·` separators
- Widget: actions today section hidden when no actions exist
- Widget: "Needs Attention" section header replaces "Today"
- Server: `assembleActionsToday` includes transfers where user is lender
- Server: `assembleActionsToday` includes requests where user is helper
- Server: `assembleActionsToday` includes experience RSVPs
- Server: `assembleActionsToday` excludes comments-only interactions
- Server: `assembleSharedInventory` sums values correctly for `shared_value`
- Server: `shared_items` sorted by value descending

---

### Phase 12: Actions Layout Polish + Fixed-Height Person Cards

**Goal:** Move "Your Actions Today" below the people carousel, redesign action
rows with a subtle checkmark + time-on-right layout, and fix the person panel
magazine cards to a uniform height with a description fallback when no comment
exists.

---

#### 12a: Move "Your Actions Today" Below the People Carousel

The actions section is currently positioned above the people row (Phase 11,
section 6 in `_buildScrollView`). Move it to after the person panel so the
people carousel leads and the actions follow.

**Updated section order in `_buildScrollView`:**

```
1.  Top padding (FloatingHeader.contentTop)
2.  Date header + shared value display
3.  Shared value expanded panel           (if expanded)
4.  Stat pills row (with quality time)
5.  Metric breakdown panel                (if expanded)
6.  People row                            ← moved up (was 7)
7.  Person panel                          ← moved up (was 8)
8.  Your Actions Today                    ← moved down (was 6)
9.  Today / Needs Attention section
10. Hidden unread overflow row
11. Coming Up section
12. Empty state
13. Bottom padding (FloatingHeader.contentBottom)
```

**Client change:** `daily_screen.dart` — reorder the sliver children so
`DailyPeopleRow` and `DailyPersonPanel` are emitted before `DailyActionsToday`.
No logic changes; purely a positional swap.

---

#### 12b: Action Row Layout — Checkmark, Time Right, Impact Below

Redesign each row in `DailyActionsToday`. Replace the emoji-led layout with a
checkmark-led layout that shows the timestamp on the right and impact metrics
on a second line below the title.

**New row layout:**
```
+--------------------------------------------------+
| ✓  Lent Pressure Washer to Gary        2:34 PM  |
|    Saved $65 · +11m quality time · 0.6kg CO₂    |
+--------------------------------------------------+
```

**Checkmark:**
- Widget: `Icon(Icons.check_rounded)`
- Size: 14px
- Color: `Theme.of(context).colorScheme.outline` (subtle, muted)
- Aligned to the top of the title line
- No emoji — the checkmark replaces the emoji as the leading element

**Title + time row:**
- `Row` with `MainAxisAlignment.spaceBetween`
- Title: left-aligned, normal weight, body text style
- Time: right-aligned, `style: Theme.of(context).textTheme.bodySmall`, muted
  color (`colorScheme.outline`), formatted as `h:mm a` (e.g. `2:34 PM`)

**Impact line:**
- Second line below the title row, indented to align with the title (past the
  checkmark)
- Same `·`-separated format as before
- Style: `textTheme.bodySmall`, muted color

**Proto change — add `time` field to `DailyAction`:**
```protobuf
message DailyAction {
  // ... existing fields 1-5 ...

  // Wall-clock time of the action, formatted for display, e.g. "2:34 PM".
  string time = 6;
}
```

**Server change:** `server/services/daily/actions.go` — populate `time` in
`assembleActionsToday` by formatting the action's timestamp using the user's
timezone (same `LoadLocation` helper used elsewhere).

---

#### 12c: Fixed-Height Person Panel Cards

The magazine-style cards inside `DailyPersonPanel` are currently variable
height because comment text has different lengths. Fix all cards to the same
height so the panel presents a uniform grid.

**Target height:** Use the current maximum natural height as the fixed size.
From the existing design, the tallest card (item name + "lent" subtitle +
full comment, ~2 lines) renders at approximately **140px**. All cards should
be constrained to this height.

**Implementation:**
- Wrap each card `Container` in a `SizedBox(height: 140)` (or the measured
  max — verify against the running UI and adjust the constant if needed)
- Cards that would naturally be shorter fill the extra space with vertical
  padding or empty space at the bottom — do not stretch text
- Cards that would be taller (very long comments) clip to 140px using
  `ClipRect` + `overflow: TextOverflow.ellipsis` on the comment text

**Define a shared constant** in `daily_person_panel.dart`:
```dart
const double _kCardHeight = 140.0;
```

Use `_kCardHeight` for the `SizedBox` so it is easy to tune.

---

#### 12d: Description Fallback When No Comment

Person panel cards show a comment excerpt below the item name and status line.
When the item has no comment, the card looks empty. Instead, show the first
one or two lines of the item's description.

**Logic (in `DailyPersonPanel` card builder):**
```
if (item.comment.isNotEmpty)
  show item.comment (truncated to 2 lines)
else if (item.description.isNotEmpty)
  show item.description (truncated to 2 lines, same style)
else
  leave blank (no subtitle)
```

**Style:** Same `textTheme.bodySmall` muted style already used for comments.
Use `maxLines: 2, overflow: TextOverflow.ellipsis` on both paths.

**Proto change — add `description` field to `DailyItem`:**
```protobuf
message DailyItem {
  // ... existing fields ...

  // Short description of the item (gear description, experience summary,
  // request body). Used as fallback when comment is empty in person cards.
  string description = 20;
}
```

**Server change:** `server/services/daily/assembly.go` — populate `description`
when assembling items: gear description for transfers, summary/body for
experiences and requests (truncated at 200 chars server-side to avoid bloat).

---

#### 12e: Summary of All Changes

**Proto changes:**
| Field | Message | Purpose |
|-------|---------|---------|
| `string time = 6` | `DailyAction` | Formatted wall-clock time, e.g. `"2:34 PM"` |
| `string description = 20` | `DailyItem` | Fallback text when comment is empty |

**Server changes:**
| File | Change |
|------|--------|
| `server/services/daily/actions.go` | Populate `time` field in `assembleActionsToday` |
| `server/services/daily/assembly.go` | Populate `description` field for transfers, experiences, requests |

**Client changes:**
| File | Change |
|------|--------|
| `app/lib/presentation/screens/daily/daily_screen.dart` | Move `DailyActionsToday` below people row + person panel |
| `app/lib/presentation/widgets/daily/daily_actions_today.dart` | Redesign rows: checkmark + time-right + impact-below layout |
| `app/lib/presentation/widgets/daily/daily_person_panel.dart` | Fixed `_kCardHeight = 140.0` for all cards; description fallback |

**Tests:**
- Widget: "Your Actions Today" section appears after the people row in scroll order
- Widget: action row leading icon is a small check, not an emoji
- Widget: action row time is right-aligned on the same line as the title
- Widget: action row impact metrics appear on a second line below the title
- Widget: check icon color matches `colorScheme.outline`
- Widget: person panel cards all have the same rendered height (`_kCardHeight`)
- Widget: person panel card shows comment when present
- Widget: person panel card shows description (max 2 lines) when comment is empty
- Widget: person panel card shows nothing below item name when both comment and description are empty
- Server: `DailyAction.time` is formatted in the user's local timezone
- Server: `DailyItem.description` is populated for transfer, experience, and request items
- Server: `DailyItem.description` is truncated at 200 characters

---

### Phase 13: Quality Minutes Breakdown Data, Checkmark Styling, Action/Inventory Navigation, Week Label

**Goal:** Fix the empty quality minutes breakdown panel, restyle the action row
checkmarks to match the reference design (daily2i.png), make action rows and
inventory rows tappable to open item screens, and add the "This week" date
range label below the stat pills.

**Reference materials:**
- [daily2i.png](../../tmp/daily/daily2i.png) — Checkmark styling: dark green
  check icon with dark green border and light green background circle; week
  label "This week · Feb 17 – 23" centered below the stat pills row

---

#### 13a: Quality Minutes Breakdown Panel — Show Data

The ring breakdown panel (`DailyMetricBreakdown` when `expandedMetric == 'ring'`)
currently only shows a header with "X / 360 min goal" and a summary sentence.
The body is empty even when `richMinutes` shows a value like 315. This is
because the `WeeklyMetrics` proto has no breakdown line items — only aggregate
totals.

**Problem:** The panel calls `_buildRingBreakdown()` which renders the header
and footer but has no line items to display. The proto's `WeeklyMetrics` message
needs breakdown data fields.

**Solution — extend `WeeklyMetrics` proto:**

```protobuf
message WeeklyMetrics {
  // ... existing fields 1-8 ...

  // Itemized quality minutes earned this week, e.g. activities + durations.
  repeated MetricLineItem rich_minutes_breakdown = 9;

  // Itemized cost savings this week.
  repeated MetricLineItem cost_breakdown = 10;
  string cost_summary = 11;

  // Itemized time recovered this week.
  repeated MetricLineItem time_breakdown = 12;
  string time_summary = 13;

  // Itemized CO₂ avoided this week.
  repeated MetricLineItem co2_breakdown = 14;
  string co2_summary = 15;
}

// MetricLineItem represents one line in a metric breakdown panel.
message MetricLineItem {
  // Emoji for the line item, e.g. "⛷️", "🧳".
  string emoji = 1;

  // Title of the activity, e.g. "Skiing at Eldora with Alfred & Gary".
  string title = 2;

  // Formatted amount, e.g. "+180", "$45", "0.4 kg".
  string amount = 3;

  // Contextual note, e.g. "Mon", "vs. buying new at Home Depot".
  string note = 4;
}
```

**Server change** (`server/services/daily/metrics.go`):
- `computeWeeklyMetrics` already computes aggregate values. Extend it to also
  build `MetricLineItem` lists by iterating over the week's completed
  activities/transfers and attributing per-item values.
- For the ring breakdown: query completed activities this week (experiences
  attended, help given, items shared) and assign quality minutes to each.
- For cost/time/CO₂ breakdowns: same source data, attributed by metric
  dimension.

**Client change** (`daily_metric_breakdown.dart`):
- `_buildRingBreakdown()`: After the header, iterate over
  `metrics.richMinutesBreakdown` and render each as a line item row (emoji +
  title + note + amount in green).
- Add a subtotal row at the bottom: "Total so far" + `richMinutes` min.
- `_buildMetricBreakdown()`: Similarly iterate over the corresponding breakdown
  list (`costBreakdown`, `timeBreakdown`, or `co2Breakdown`) and render line
  items with amounts in accent color.
- When the breakdown list is empty, keep the current simplified view (header +
  summary only) as a graceful fallback.

**Line item row layout** (matching the JSX reference):
```
+--------------------------------------------------+
| ⛷️  Skiing at Eldora with Alfred & Gary    +180  |
|     Mon                                          |
+--------------------------------------------------+
| 🧳  Helped Thomas move furniture            +30  |
|     Tue                                          |
+--------------------------------------------------+
```

- Emoji: 16px, 24px width, centered
- Title: 12px, w600, primary text
- Note: 10px, muted
- Amount: 13px, w700, green (ring) or coral (cost/time/CO₂), right-aligned
- Row padding: 8px 14px, border-bottom between rows

---

#### 13b: Checkmark Styling — Green Border + Light Background

The current checkmark in `_ActionRow` (`daily_actions_today.dart`) uses a solid
teal circle (`Color(0xFF4E9D8A)`) with a white check icon. Per daily2i.png, it
should be:

**New checkmark design:**
- Circle: 20px diameter
- Background: light green (`Color(0xFFE8F5E9)` or similar light sage)
- Border: 1.5px solid dark green (`Color(0xFF4E7D5B)`)
- Check icon: `Icons.check_rounded`, 12px, dark green (`Color(0xFF4E7D5B)`)

This gives a softer, outlined appearance rather than the current filled teal
circle.

**Implementation:** Replace the `Container` decoration in `_ActionRow`:
```dart
Container(
  width: 20,
  height: 20,
  decoration: BoxDecoration(
    color: const Color(0xFFE8F5E9),  // light green bg
    shape: BoxShape.circle,
    border: Border.all(
      color: const Color(0xFF4E7D5B),  // dark green border
      width: 1.5,
    ),
  ),
  child: const Icon(
    Icons.check_rounded,
    size: 12,
    color: Color(0xFF4E7D5B),  // dark green check
  ),
)
```

**Files changed:**
- `app/lib/presentation/widgets/daily/daily_actions_today.dart` — Update
  `_ActionRow` checkmark styling

---

#### 13c: Action Row Tap → Open Item Screen

Tapping an action row in "Your Actions Today" should open the corresponding
item screen (GearScreen, ExperienceScreen, RequestScreen), just like tapping
items in the "Needs Attention" and "Coming Up" sections.

**Proto change — add navigation fields to `DailyAction`:**

```protobuf
message DailyAction {
  // ... existing fields 1-9 ...

  // Type of the source item (transfer, experience, or request).
  DailyItemType item_type = 10;

  // Navigable content ID. For transfers this is the gear_id; for experiences
  // and requests this equals item_id. Used to open the item screen.
  string content_id = 11;
}
```

**Server change** (`server/services/daily/actions.go`):
- `assembleActionsToday` already knows the source item type and ID. Populate
  `item_type` and `content_id` (same logic as `assembly.go` — gear_id for
  transfers, item_id for others).

**Client change** (`daily_actions_today.dart`):
- Accept an `onActionTap` callback: `ValueChanged<DailyAction>?`
- Wrap each `_ActionRow` in `GestureDetector` or `InkWell` with
  `onTap: () => onActionTap?.call(action)`

**Client change** (`daily_screen.dart`):
- Pass `onActionTap: _onActionTap` to `DailyActionsToday`
- Add `_onActionTap(DailyAction action)` method that navigates by type:
  ```dart
  void _onActionTap(DailyAction action) {
    if (action.contentId.isEmpty) return;
    switch (action.itemType) {
      case DailyItemType.DAILY_ITEM_TYPE_TRANSFER:
        NavigationHelpers.pushScreen(
          context: context,
          screen: GearScreen(gearId: action.contentId),
        );
      case DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE:
        NavigationHelpers.pushScreen(
          context: context,
          screen: ExperienceScreen(experienceId: action.contentId),
        );
      case DailyItemType.DAILY_ITEM_TYPE_REQUEST:
        NavigationHelpers.pushScreen(
          context: context,
          screen: RequestScreen(requestId: action.contentId),
        );
      default:
        break;
    }
  }
  ```

---

#### 13d: Inventory Row Tap → Open Item Screen

Tapping a row in the expanded shared value inventory panel should open the
corresponding item screen (GearScreen for gear items).

**Proto change — add navigation fields to `SharedItem`:**

```protobuf
message SharedItem {
  // ... existing fields 1-3 ...

  // Type of the source item (always DAILY_ITEM_TYPE_TRANSFER for shared items).
  DailyItemType item_type = 4;

  // Navigable content ID (gear_id). Used to open GearScreen.
  string content_id = 5;
}
```

**Server change** (`server/services/daily/actions.go`):
- `assembleSharedInventory` populates `item_type` (always
  `DAILY_ITEM_TYPE_TRANSFER`) and `content_id` (gear_id) for each shared item.

**Client change** (`daily_value_panel.dart`):
- Accept an `onItemTap` callback: `ValueChanged<SharedItem>?`
- Wrap each `_buildItemRow` in `GestureDetector` or `InkWell`:
  `onTap: () => onItemTap?.call(item)`

**Client change** (`daily_screen.dart`):
- Pass `onItemTap: _onSharedItemTap` to `DailyValuePanel`
- Add `_onSharedItemTap(SharedItem item)` method:
  ```dart
  void _onSharedItemTap(SharedItem item) {
    if (item.contentId.isEmpty) return;
    NavigationHelpers.pushScreen(
      context: context,
      screen: GearScreen(gearId: item.contentId),
    );
  }
  ```

---

#### 13e: Week Label Below Stat Pills

Add a centered "This week · Feb 17 – 23" label below the stat pills row,
matching the reference in daily2i.png. This provides temporal context for the
metric values without requiring a tap.

**Layout:** A single `Text` widget centered below the pills row, outside the
pills `Row` but still within `DailyStatPills` (or as a separate
`SliverToBoxAdapter` in `daily_screen.dart`).

**Styling:**
- Text: `"This week · $weekLabel"` where `weekLabel` comes from
  `WeeklyMetrics.week_label` (e.g. `"Feb 17 – 23"`)
- Font: 9px, w600, muted color (`#9A8D86`), 0.3 letter-spacing
- Centered horizontally
- Top margin: 4px below the pills row, bottom margin: 8px before the next
  section

**Implementation option A — inside `DailyStatPills`:**
Add `weekLabel` as a new parameter. Render a `Text` widget below the pills
`Row` in a `Column` wrapper.

**Implementation option B — in `daily_screen.dart`:**
Add a separate `SliverToBoxAdapter` after the stat pills sliver containing
the centered week label text. This keeps `DailyStatPills` focused on the pills
only.

Either approach works. Option A is slightly cleaner since the label is visually
part of the metrics area.

**Proto dependency:** Uses the existing `WeeklyMetrics.week_label` field (field
8), which is already populated by the server. No proto changes needed.

**Files changed:**
- `app/lib/presentation/widgets/daily/daily_stat_pills.dart` — Accept
  `weekLabel` parameter, render centered text below the pills row
  (or `daily_screen.dart` if using option B)

---

#### 13f: Summary of All Changes

**Proto changes:**
| Field/Message | Location | Purpose |
|---------------|----------|---------|
| `repeated MetricLineItem rich_minutes_breakdown = 9` | `WeeklyMetrics` | Itemized quality minutes |
| `repeated MetricLineItem cost_breakdown = 10` | `WeeklyMetrics` | Itemized cost savings |
| `string cost_summary = 11` | `WeeklyMetrics` | Summary sentence for cost |
| `repeated MetricLineItem time_breakdown = 12` | `WeeklyMetrics` | Itemized time recovered |
| `string time_summary = 13` | `WeeklyMetrics` | Summary sentence for time |
| `repeated MetricLineItem co2_breakdown = 14` | `WeeklyMetrics` | Itemized CO₂ avoided |
| `string co2_summary = 15` | `WeeklyMetrics` | Summary sentence for CO₂ |
| `MetricLineItem` message | `daily.proto` | Single line item in a metric breakdown |
| `DailyItemType item_type = 10` | `DailyAction` | Source item type for navigation |
| `string content_id = 11` | `DailyAction` | Navigable content ID for item screen |
| `DailyItemType item_type = 4` | `SharedItem` | Source item type for navigation |
| `string content_id = 5` | `SharedItem` | Navigable content ID for item screen |

**Server changes:**
| File | Change |
|------|--------|
| `server/services/daily/metrics.go` | Build `MetricLineItem` lists for all four breakdown types |
| `server/services/daily/actions.go` | Populate `item_type` + `content_id` on `DailyAction` and `SharedItem` |

**Client changes:**
| File | Change |
|------|--------|
| `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` | Render breakdown line items for ring and metric panels |
| `app/lib/presentation/widgets/daily/daily_actions_today.dart` | Restyle checkmark (green border + light bg), accept `onActionTap` |
| `app/lib/presentation/widgets/daily/daily_value_panel.dart` | Accept `onItemTap`, wrap rows in tappable gesture |
| `app/lib/presentation/screens/daily/daily_screen.dart` | Add `_onActionTap` and `_onSharedItemTap` handlers, pass to widgets |
| `app/lib/presentation/widgets/daily/daily_stat_pills.dart` | Accept `weekLabel`, render centered week date range below pills row |

**Tests:**
- Widget: ring breakdown panel shows line items when `richMinutesBreakdown` is non-empty
- Widget: ring breakdown shows subtotal row with total minutes
- Widget: ring breakdown gracefully falls back when breakdown list is empty
- Widget: cost/time/CO₂ breakdown panels show line items from proto data
- Widget: checkmark uses light green background with dark green border and icon
- Widget: tapping an action row calls `onActionTap` with the action
- Widget: action tap navigates to GearScreen for transfer actions
- Widget: action tap navigates to ExperienceScreen for experience actions
- Widget: action tap navigates to RequestScreen for request actions
- Widget: tapping an inventory row calls `onItemTap` with the shared item
- Widget: inventory tap navigates to GearScreen with correct gear ID
- Widget: week label "This week · ..." appears centered below stat pills
- Server: `richMinutesBreakdown` contains one entry per completed activity this week
- Server: `costBreakdown` line item amounts sum to `costSaved` total
- Server: `DailyAction.content_id` is gear_id for transfer actions
- Server: `SharedItem.content_id` is gear_id for all inventory items

---

### Phase 15: Fix Daily Actions Today — Correct Qualifying Criteria

**Goal:** Fix `assembleActionsToday` on the server and the proto comment to match the
three correct categories. The current implementation shows the wrong things: it
misses completed-today activities, skips "expressed interest" transfers, and uses
an imprecise proxy timestamp for "shared today".

---

#### 15a: Diagnosis — What's Wrong Now

**Problem 1 — "Shared today" is too narrow.**
The current code only includes transfers where the user is the lender/owner **and**
the state is not `INTEREST_EXPRESSED` and `LatestRequestUnixSec` falls in today's
bounds. This means it shows "I confirmed a loan/giveaway today" — which is correct
for "shared today" — but the label `LatestRequestUnixSec` is the last state-change
timestamp, which could reflect the recipient requesting, not the owner confirming.

**Problem 2 — "Completed today" is entirely missing.**
`deduplicateAndFilterTransfers` strips out terminal-state transfers
(`COMPLETED`/`CANCELLED`) before they ever reach `assembleActionsToday`. The same
applies to fulfilled requests and completed experiences. If the user participated in
a loan that was returned today, a help request that was resolved today, or an
experience that happened today, none of that appears.

**Problem 3 — "Expressed interest" is explicitly skipped.**
The current code has an explicit `continue` for `TRANSFER_STATE_INTEREST_EXPRESSED`.
When a user expresses interest in borrowing or receiving a giveaway, that should
appear in "Your Actions Today" but currently does not.

---

#### 15b: Three Correct Categories

Replace the current single-pass logic with three clearly-named sub-categories.

**Category A — Shared today** *(user lent or gave something)*

Transfers where:
- `owner_id == userID` (user is the lender/giver)
- State is `RECIPIENT_SELECTED` or `ACTIVE` (loan/giveaway confirmed — past the
  interest stage)
- `LatestRequestUnixSec` falls within today's bounds (proxy for when the owner
  confirmed the share)

Display: "Lent Pressure Washer to Gary" / "Gave away Old Camping Stove to Derek"

**Category B — Completed today** *(user participated in something that finished)*

Things that reached a terminal or "happened" state today, where the user was a
participant:

| Item type | Qualifying condition | User role | Display |
|-----------|----------------------|-----------|---------|
| Loan | State = `COMPLETED`, `LatestRequestUnixSec` in today | owner **or** recipient | "Loan of X returned" |
| Giveaway | State = `COMPLETED`, `LatestRequestUnixSec` in today | owner **or** recipient | "Gave away X to Y" / "Received X from Y" |
| Request | State = `FULFILLED`, completion timestamp in today | requester **or** confirmed helper | "Help request fulfilled: X" |
| Experience | `StartTime` falls in today's bounds, state ≠ `CANCELLED` | host **or** has YES RSVP | "X happened today" |

For experiences, "completed today" means the start time is today — the experience
is either happening now or already happened. Use the experience `StartTime`
(unix seconds) compared against `todayStart`/`todayEnd`.

For requests, `LatestRequestUnixSec` is not a field on `models.Request` — use the
`UpdatedAtUnixSec` field if available, otherwise fall back to `CreatedAtUnixSec`.
Check the proto field names in `models.Request` before implementing.

**Category C — Engaged today** *(user expressed intent)*

- **Expressed interest in a loan or giveaway**: Transfer where `recipient_id ==
  userID`, state = `INTEREST_EXPRESSED`, `LatestRequestUnixSec` is today.
  Display: "Expressed interest in X from Y"
- **Offered to help on a request**: User is in `confirmed_helper_ids` on an active
  (non-terminal, non-deleted) request where `requester_id != userID`. No precise
  timestamp available — include if helper role is present. Display: "Helping with X"
- **RSVPed to an experience**: User has a YES RSVP on an active (non-cancelled,
  non-deleted) experience where `owner_id != userID`. No precise timestamp — include
  if RSVP is present. Display: "RSVP'd to X"

---

#### 15c: Server Changes

**`server/services/daily/daily_view.go`**

Pass the raw owner/recipient transfer slices (before terminal filtering) into
`assembleActionsToday` so it can find completed-today transfers:

```go
// Before calling assembleActionsToday, collect all owner+recipient transfers
// (not just active ones) for the completed-today category.
allOwnerTransfers := transfersAsOwner   // raw, includes terminal
allRecipientTransfers := transfersAsRecipient  // raw, includes terminal
```

Update the `assembleActionsToday` call signature to pass the raw slices
(`allOwnerTransfers`, `allRecipientTransfers`) instead of or in addition to
`activeTransfers`. Keep `activeTransfers` for the existing item assembly steps 8+
(those must remain active-only).

Also pass `reqMap` (which already contains all requests including terminal) so
`assembleActionsToday` can check for fulfilled-today requests.

**`server/services/daily/actions.go`**

Refactor `assembleActionsToday` to implement the three categories above. The new
signature:

```go
func assembleActionsToday(
    activeTransfers []*models.Transfer,   // active (non-terminal) — for Category C
    allOwnerTransfers []proto.Message,     // all owner transfers incl. terminal — for A + B
    allRecipientTransfers []proto.Message, // all recipient transfers — for B
    gearProtoMap map[string]proto.Message,
    userMap map[string]*api.User,
    activeReqIDs []string,
    reqMap map[string]proto.Message,       // all requests incl. terminal — for B
    activeExpIDs []string,
    expMap map[string]proto.Message,
    userRSVPExpIDs map[string]bool,
    userID string,
    dayStart, dayEnd int64,
    now time.Time,
    tz *time.Location,
) []*api.DailyAction
```

Implementation:

```
Category A: loop allOwnerTransfers
  - state == RECIPIENT_SELECTED or ACTIVE
  - LatestRequestUnixSec in [dayStart, dayEnd)
  → append "Lent X to Y" or "Gave away X to Y"

Category B:
  Loans/giveaways completed today:
    loop allOwnerTransfers + allRecipientTransfers (dedup by transfer ID)
    - state == COMPLETED
    - LatestRequestUnixSec in [dayStart, dayEnd)
    - For owner: "Loan of X returned" (loan) or "Gave away X to Y" (giveaway)
    - For recipient: "Returned X to Y" (loan) or "Received X from Y" (giveaway)
    - Skip if already added from Category A (same transfer ID)

  Requests fulfilled today:
    loop reqMap
    - state == FULFILLED
    - completion timestamp in [dayStart, dayEnd)  (use updated_at or created_at field)
    - user is requester OR confirmed helper
    - For requester: "Help request fulfilled: X"
    - For helper: "Helped with X"

  Experiences happening today:
    loop expMap (not just activeExpIDs — check all non-cancelled)
    - StartTime in [dayStart, dayEnd)
    - state != CANCELLED and not deleted
    - user is owner OR has YES RSVP (userRSVPExpIDs[expID])
    - For owner: "Hosted X today"
    - For attendee: "Attended X"

Category C:
  Expressed interest:
    loop allRecipientTransfers (and allOwnerTransfers to catch any edge cases)
    - recipient_id == userID
    - state == INTEREST_EXPRESSED
    - LatestRequestUnixSec in [dayStart, dayEnd)
    → "Expressed interest in X from Y"

  Offered help (existing logic, keep as-is):
    loop activeReqIDs where user is confirmed helper and not requester
    → "Helping with X"

  RSVPed (existing logic, keep as-is):
    loop activeExpIDs where userRSVPExpIDs[expID] and user is not owner
    → "RSVP'd to X"
```

Update the proto comment on `actions_today` in `daily.proto`:

```proto
// Concrete actions the user took or participated in today, grouped into three
// categories: (A) shared something today (lent or gave an item), (B) participated
// in something completed today (loan returned, help request fulfilled, experience
// happened), (C) expressed intent (offered help, RSVPed, expressed interest in
// a loan or giveaway). Hidden when empty.
```

---

#### 15d: Files Changed

| File | Change |
|------|--------|
| `server/services/daily/actions.go` | Refactor `assembleActionsToday` — three categories, fix expressed interest, add completed-today |
| `server/services/daily/daily_view.go` | Pass raw owner/recipient transfer slices to `assembleActionsToday`; keep `activeTransfers` for item assembly |
| `proto/ripls/api/daily.proto` | Update `actions_today` comment to describe the three categories |

No client changes needed — `DailyActionsToday` renders whatever actions the server
returns, and the existing `_ActionRow` layout (check icon, title, impact) works for
all three categories.

---

**Tests** (`server/services/daily/daily_test.go`):
- `assembleActionsToday` includes a loan where owner confirmed (RECIPIENT_SELECTED) today → Category A
- `assembleActionsToday` includes a giveaway where owner confirmed today → Category A
- `assembleActionsToday` omits a loan confirmed yesterday (outside today bounds)
- `assembleActionsToday` includes a completed loan (COMPLETED state, today) where user is recipient → Category B
- `assembleActionsToday` includes a fulfilled request (today) where user is helper → Category B
- `assembleActionsToday` includes an experience happening today where user has YES RSVP → Category B
- `assembleActionsToday` includes INTEREST_EXPRESSED transfer today where user is recipient → Category C
- `assembleActionsToday` omits INTEREST_EXPRESSED transfer expressed yesterday
- `assembleActionsToday` includes confirmed helper on active request → Category C
- `assembleActionsToday` omits requester from their own request in Category C (not an outbound action)
- `assembleActionsToday` includes RSVP'd experience where user is attendee, not host → Category C
- `assembleActionsToday` does not duplicate a transfer that appears in both Category A and B

---

## Architecture Compliance

### Compliant

- **MVVM boundaries:** ViewModel fetches through Repository -> Service. Screen
  uses `ref.watch()`. No direct service access from widgets.
- **Repository pattern with CacheService:** Namespace-isolated caching,
  standard invalidation pattern.
- **AsyncNotifier<DailyViewData> with autoDispose:** `DailyViewData` is a
  Freezed class holding both the server response and UI toggle state
  (`expandedMetric`, `expandedPersonId`). This matches the `TimeModalData`
  pattern. `ref.mounted` checks after every async gap in non-build methods.
- **Server service independence:** Daily service depends only on
  `storage.ProtoSQLStorage` + shared libraries. No cross-service imports.
- **Proto package separation:** All types in `proto/ripls/api/`. Client never
  references `proto/ripls/models/`.
- **File size discipline:** Screen, viewmodel, and each widget in separate
  files. Server split by feature.
- **Single RPC per load:** One `GetDailyView` call returns everything,
  including metric breakdowns.
- **Unread count consistency:** Shared `IsItemDone` filter, `total_unread_count`
  field, overflow row for edge cases.
- **Structured logging:** `logging.LoggerWithContext(ctx)` with standard fields.
- **Widget extraction:** Each section is a separate widget, keeping `build()`
  small.
- **UserAvatar for profile images:** People row and item rows use `UserAvatar`.
- **NavigationHelpers:** All navigation via `NavigationHelpers.pushScreen()`.

### Issues to Watch

1. **Metric breakdown data (blocker):** Per-item impact values (`impact_*`
   proto fields) need a confirmed data source before implementation. See Q2/Q4
   above. Defer those proto fields and the impact overlay UI until resolved.
2. **People row rebuild frequency:** Each avatar may watch `mediaUrlProvider`.
   Ensure rebuilds are scoped to individual avatars (use a separate
   `ConsumerWidget` per avatar, not a single widget watching all).
3. **Expanded panel animations:** `AnimatedSize` with `BackdropFilter` can
   be expensive. Profile on low-end devices; fall back to `AnimatedCrossFade`
   without blur if needed.
4. **Cache staleness:** The global 30-minute TTL is too long for a screen that
   shows "today's items". Phase 8 must add automatic refresh on tab focus:
   listen to `selectedCommunityProvider` changes in `build()` (switching
   community auto-refreshes), and add a `WidgetsBindingObserver`/
   `AppLifecycleState.resumed` listener to invalidate cache after the app
   returns from background.
5. **CommunityId in `build()`:** `DailyNotifier.build()` reads
   `selectedCommunityProvider` for the current community ID. If the user
   switches communities while the tab is visible, `build()` is re-evaluated
   automatically because `ref.watch(selectedCommunityProvider)` is called
   inside `build()` — no extra wiring needed.

---

## Summary of Files

### Proto
| File | Status |
|------|--------|
| `proto/ripls/api/daily.proto` | New |

### Server
| File | Status |
|------|--------|
| `server/conversation/state.go` | New (extract `IsItemDone` from chat) |
| `server/services/daily/service.go` | New |
| `server/services/daily/daily_view.go` | New |
| `server/services/daily/assembly.go` | New |
| `server/services/daily/actions.go` | New (Phase 11: actions today + shared inventory) |
| `server/services/daily/metrics.go` | New |
| `server/services/daily/people.go` | New |
| `server/services/daily/daily_test.go` | New |
| `server/services/chat/unread_counts.go` | Changed (use shared `IsItemDone`) |
| `server/services/chat/conversations.go` | Changed (remove local `isItemDone`) |
| `server/main.go` | Changed (register DailyService) |

### Client
| File | Status |
|------|--------|
| `app/lib/services/daily_service.dart` | New |
| `app/lib/data/repositories/daily_repository.dart` | New |
| `app/lib/presentation/viewmodels/daily_view_model.dart` | New |
| `app/lib/presentation/screens/daily/daily_screen.dart` | New |
| `app/lib/presentation/widgets/daily/daily_header.dart` | New |
| `app/lib/presentation/widgets/daily/daily_ring.dart` | New |
| `app/lib/presentation/widgets/daily/daily_stat_pills.dart` | New |
| `app/lib/presentation/widgets/daily/daily_metric_breakdown.dart` | New |
| `app/lib/presentation/widgets/daily/daily_people_row.dart` | New |
| `app/lib/presentation/widgets/daily/daily_person_panel.dart` | New |
| `app/lib/presentation/widgets/daily/daily_item_row.dart` | New |
| `app/lib/presentation/widgets/daily/daily_emoji_thumbnail.dart` | New |
| `app/lib/presentation/widgets/daily/daily_future_row.dart` | New |
| `app/lib/presentation/widgets/daily/daily_shared_value.dart` | New (Phase 11) |
| `app/lib/presentation/widgets/daily/daily_value_panel.dart` | New (Phase 11) |
| `app/lib/presentation/widgets/daily/daily_actions_today.dart` | New (Phase 11) |
| `app/lib/presentation/screens/home/home_screen.dart` | Changed (swap inbox -> daily tab) |
| `app/lib/presentation/viewmodels/home_view_model.dart` | Changed (daily nav keys) |
| `app/lib/main.dart` | Changed (notification handler) |

### Tests
| File | Status |
|------|--------|
| `server/services/daily/daily_test.go` | New |
| `app/test/data/repositories/daily_repository_test.dart` | New |
| `app/test/presentation/viewmodels/daily_view_model_test.dart` | New |
| `app/test/presentation/widgets/daily/daily_header_test.dart` | New |
| `app/test/presentation/widgets/daily/daily_stat_pills_test.dart` | New |
| `app/test/presentation/widgets/daily/daily_people_row_test.dart` | New |
| `app/test/presentation/widgets/daily/daily_item_row_test.dart` | New |
| `app/test/presentation/widgets/daily/daily_future_row_test.dart` | New |

---

**Last Updated:** 2026-02-27 — Phase 13 updated: added week label below stat pills (13e)
