# Portfolio Inbox Redesign

> **Audit Status:** Reviewed against `docs/client/architecture.md`,
> `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`
> on 2026-03-17. Findings inlined as `[AUDIT FIX]` markers below.

## Problem Statement

The current portfolio inbox screen is a flat chronological list of items grouped
into "In Progress", "Coming Up", and weekly metrics. It works but has several UX
problems:

1. **No day-level navigation.** Users cannot browse what happened on a specific
   day this week or in past weeks. Everything is bucketed as "today" vs. "coming
   up" with no way to revisit yesterday or plan for Thursday.

2. **Metrics are disconnected from items.** The weekly metrics carousel sits at
   the top but has no visual relationship to the day the user is viewing. The
   three progress rings (Social Time, Money Saved, CO₂ Avoided) should be the
   hero element with clear goal tracking.

3. **No week navigation.** Users cannot look back at prior weeks to see what
   they accomplished. The `WeeklyDayCard` has disabled chevrons.

4. **Item cards are plain list rows.** The current `DailyItemRow` is a compact
   text row. The reference design calls for rich cards with background images,
   gradient overlays, last-comment previews, and inline "Review" buttons — more
   like the discover gallery cards.

5. **No activity-count dots on day selector.** Users cannot glance at the week
   and see which days are busy or have unread messages without tapping each one.

6. **No weekly goals for metrics.** The current metrics have no user-settable
   goals; the only goal is the hardcoded 360-minute quality time target. The
   reference shows goals for all three ring metrics.

7. **Metric detail screen is basic.** The reference shows a sparkline chart
   ("Social Time Over Week"), an insight quote, a weekly goal row, and a sorted
   activity list — richer than the current tabular `MetricDetailScreen`.

## Reference Design Summary

From the screenshots and React prototype:

- **Week navigator** — `‹ [This Week / Mar 9–15] ›` bar at top; tapping center
  resets to current week; forward button disabled when on current week.
- **Three metric rings** — Social Time, Money Saved, CO₂ Avoided as tappable
  circular progress indicators with value/goal inside. Color: sage when ≥100%,
  sage-soft when ≥50%, muted when <50%.
- **Day selector** — 7-column Mon–Sun bar showing day abbreviation, date number,
  and up to 5 activity dots beneath each date. Selected day has coral outline.
  Orange dots = unread messages; muted dots = all read. Today's date number is
  coral even when not selected.
- **Day label** — "Today", "Yesterday", "Tomorrow", or full date for past weeks.
- **Action cards** — rounded 16px cards with item background image + dark
  gradient overlay. Layout:
  - Title row: item name (bold white), owner avatar, type/status pill
  - Divider line
  - If has comments: last comment (avatar + name + time + text), then
    "N unread" / "All caught up" + coral "Review" button
  - If no comments: chat input placeholder + "Review" button
- **Empty day** — centered "Nothing that day" card.
- **Metric detail** — slides from right. Shows: large italic value, subtitle,
  insight quote, sparkline chart (Mon–Sun), weekly goal row, sorted activity list
  with values.

## Potential Solutions

### A. Client-only day grouping (recommended)

The server already returns `timed_items`, `anytime_items`, and `coming_up`
groups. We extend the server to accept a `week_offset` parameter and return
items grouped by ISO day index (0=Mon..6=Sun) instead of the current
today/coming-up split. The client manages day selection state locally.

**Pros:** Clean separation; server does the heavy lifting of grouping and
sorting; client stays simple. Single RPC per week load.

**Cons:** Need a new response shape or a new RPC.

### B. Multiple RPCs per day

Add a `GetPortfolioDay(date)` RPC and call it on each day tap.

**Pros:** Minimal data per request.

**Cons:** Laggy day switching; many round trips when browsing; complex caching.

### C. Purely client-side from existing data

Use the current response and try to group items by day on the client.

**Pros:** No server changes.

**Cons:** Current response doesn't include items for past days or other days
this week; would need to fetch everything and filter client-side; doesn't scale
to past weeks.

**Recommendation: Option A.** Add a new RPC `GetPortfolioInboxWeek` that returns
a full week's data in one call. Keep the existing `GetPortfolioInboxView` for
backward compatibility during migration, then deprecate it.

## Open Questions

1. **Weekly goals storage.** Where should user-customizable weekly goals live?
   Options:
   - (a) New fields on the `User` model (simple, but couples user to metrics)
   - (b) A new `UserGoals` model in `models/` (clean separation)
   - (c) Fields on `WeeklyMetrics` response only, with defaults server-side and
     a `SetWeeklyGoals` RPC to persist

   **Recommendation:** Option (c) — add a `SetWeeklyGoals` RPC and a small
   `UserWeeklyGoals` storage model. The goals appear on `WeeklyMetrics` in the
   response (populated from storage or defaults).

2. **Sparkline data for metric detail.** The reference shows a cumulative line
   chart per metric across the 7 days. Should the server compute daily
   cumulative totals, or should the client derive them from per-item data?

   **Recommendation:** Server computes 7 daily cumulative values and returns
   them in the metric detail response. Keeps the client simple.

3. **How far back can users navigate?** Unlimited history or capped?

   **Recommendation:** Cap at 12 weeks (3 months). Beyond that, direct users to
   the Portfolio Metrics tab for long-term trends.

4. **"Review" button behavior.** Should it open the conversation chat, or the
   item screen (gear/experience/request)?

   **Recommendation:** Open the item screen (same as current tap behavior).
   The chat is accessible from within the item screen.

5. **"Leave a note…" input.** Is this a real inline input or just a tap target
   that opens the conversation?

   **Recommendation:** Tap target only — opens the conversation screen. Building
   an inline compose field is complex and low-value for v1.

## Implementation Plan

### Phase 0: Proto & Server — New `GetPortfolioInboxWeek` RPC

**Goal:** Server returns a full week of items grouped by day, plus weekly
metrics with goal data and sparkline points.

**Proto changes (`proto/ripls/api/portfolio.proto`):**

```protobuf
// New RPC
rpc GetPortfolioInboxWeek(GetPortfolioInboxWeekRequest)
    returns (GetPortfolioInboxWeekResponse);

// New RPC for setting user weekly goals
rpc SetWeeklyGoals(SetWeeklyGoalsRequest)
    returns (SetWeeklyGoalsResponse);

message GetPortfolioInboxWeekRequest {
  string timezone = 1;
  // Week offset from current week. 0 = this week, -1 = last week, etc.
  // Capped at -12.
  int32 week_offset = 2;
}

message GetPortfolioInboxWeekResponse {
  // Week label, e.g. "This Week" or "Mar 9 – Mar 15".
  string week_label = 1;
  // ISO week start date formatted, e.g. "Mar 16".
  string week_start_label = 2;
  // ISO week end date formatted, e.g. "Mar 22".
  string week_end_label = 3;
  // Whether this is the current week (controls forward nav button state).
  bool is_current_week = 4;
  // [AUDIT FIX] Use `optional` instead of -1 sentinel per proto field
  // conventions in CLAUDE.md. Absent = not current week; present = day index.
  optional int32 today_index = 5;

  // 7 day buckets (Mon=0..Sun=6), each with items and metadata.
  repeated InboxDay days = 6;

  // Weekly metrics (same shape, extended with goals and sparkline data).
  WeeklyMetrics weekly_metrics = 7;
}

// InboxDay represents one day in the week view.
message InboxDay {
  // Day index 0–6 (Mon–Sun).
  int32 day_index = 1;
  // Date number for display, e.g. 16.
  int32 date_number = 2;
  // Day abbreviation, e.g. "Mon".
  string day_abbrev = 3;
  // Full date label, e.g. "Monday, Mar 16" or "Today".
  string date_label = 4;
  // Items for this day, sorted by time then urgency.
  repeated DailyItem items = 5;
  // Count of items (for dot display; capped at 5 in UI).
  int32 item_count = 6;
  // Whether any item on this day has unread messages.
  bool has_unread = 7;
  // Whether any item on this day is urgent.
  bool has_urgent = 8;
}

message SetWeeklyGoalsRequest {
  // Social time goal in minutes. 0 = use system default.
  int32 social_time_minutes_goal = 1;
  // Money saved goal in cents. 0 = use system default.
  int32 money_saved_cents_goal = 2;
  // CO₂ avoided goal in grams. 0 = use system default.
  int32 co2_avoided_grams_goal = 3;
}

message SetWeeklyGoalsResponse {
  // The effective goals after saving (may be defaults if 0 was sent).
  int32 social_time_minutes_goal = 1;
  int32 money_saved_cents_goal = 2;
  int32 co2_avoided_grams_goal = 3;
}
```

**Extend `WeeklyMetrics`:**

```protobuf
// Add to existing WeeklyMetrics message:

// User's weekly money-saved goal, formatted, e.g. "$500".
string cost_saved_goal = 20;
// User's weekly CO₂ goal, formatted, e.g. "30kg".
string co2_avoided_goal = 21;
// 7 daily cumulative social time values (Mon–Sun) for sparkline.
repeated int32 daily_social_cumulative = 22;
// 7 daily cumulative cost saved values in cents for sparkline.
repeated int32 daily_cost_cumulative = 23;
// 7 daily cumulative CO₂ avoided values in grams for sparkline.
repeated int32 daily_co2_cumulative = 24;
```

**New storage model (`proto/ripls/models/user.proto`):**

```protobuf
message UserWeeklyGoals {
  string id = 1;
  string user_id = 2;
  int32 social_time_minutes_goal = 3;
  int32 money_saved_cents_goal = 4;
  int32 co2_avoided_grams_goal = 5;
  int64 updated_at_unix_sec = 6;
}
```

**Server implementation (`server/services/portfolio/`):**

- **[AUDIT FIX] Extract shared batch-fetch logic.** The current `inbox_view.go`
  is ~1060 lines. Adding a second handler (`inbox_week.go`) that duplicates the
  batch-fetch steps (communities, transfers, experiences, requests, conversations,
  messages, gear, locations, users) would violate the <1000-line file guideline
  and DRY. Instead:
  - New file `fetch.go` — extract the common batch-fetch pipeline (steps 1–10
    from `inbox_view.go`) into a shared `fetchContext` struct and `fetchAll()`
    function. Both `GetPortfolioInboxView` and `GetPortfolioInboxWeek` call
    `fetchAll()` then diverge only on grouping/sorting.
- New file `inbox_week.go` — `GetPortfolioInboxWeek` handler. Calls
  `fetchAll()`, then groups items into 7 day buckets by their `time_unix_sec`
  (or creation/activity timestamp for anytime items). For past weeks, includes
  completed/terminal items that had activity during that week.
- New file `goals.go` — `SetWeeklyGoals` handler + storage logic.
- Modify `metrics.go` to compute daily cumulative sparkline arrays and populate
  goal fields from `UserWeeklyGoals` storage (with system defaults as fallback).

**System defaults:** Social time = 360 min (6h), Money saved = $500, CO₂ = 30kg.

**Files created/modified:**
- `proto/ripls/api/portfolio.proto` (extend)
- `proto/ripls/models/user.proto` (add `UserWeeklyGoals`)
- `server/services/portfolio/fetch.go` (new — extracted shared batch-fetch pipeline)
- `server/services/portfolio/inbox_view.go` (refactor to call `fetchAll()`)
- `server/services/portfolio/inbox_week.go` (new)
- `server/services/portfolio/goals.go` (new)
- `server/services/portfolio/metrics.go` (modify for sparkline + goals)
- `server/storage/protosql_schema.go` (register `UserWeeklyGoals`)
- `server/main.go` (register new RPCs)

**Tests:**
- `server/services/portfolio/inbox_week_test.go`
- `server/services/portfolio/goals_test.go`

---

### Phase 1: Client data layer

**Goal:** Wire up the new RPC and caching on the client.

- `app/lib/services/portfolio_service.dart` — add `getInboxWeek(timezone,
  weekOffset)` and `setWeeklyGoals(...)` methods.
- `app/lib/data/repositories/portfolio_repository.dart` — add
  `getInboxWeek(timezone, weekOffset)` with cache key
  `'portfolio:inbox_week:$weekOffset'`, `refreshInboxWeek(weekOffset)`,
  `setWeeklyGoals(...)`.
- `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart` — rewrite
  `PortfolioInboxState` and `PortfolioInboxNotifier`:
  - **[AUDIT FIX]** State must continue to use `@freezed` for immutable state
    per architecture.md. The existing `PortfolioInboxState` already uses Freezed;
    the rewrite extends it with new fields.
  - New state fields: `weekOffset` (int, default 0), `selectedDayIndex` (int,
    defaults to `todayIndex` from response), `weekResponse`
    (`GetPortfolioInboxWeekResponse?`).
  - New actions: `selectDay(index)`, `previousWeek()`, `nextWeek()`,
    `setWeeklyGoals(...)`.
  - On `load()`: fetch week 0, set `selectedDayIndex` to `todayIndex`.
  - On `previousWeek()`: decrement offset (min -12), fetch, select day 0.
  - On `nextWeek()`: increment offset (max 0), fetch, select today if week 0.
  - **[AUDIT FIX]** Cache invalidation for week data must be wired in Phase 1
    (not deferred to Phase 7). The `portfolioCacheInvalidationProvider` listener
    in `build()` must call `_repository.refreshInboxWeek(state.weekOffset)` on
    signal, following the existing debounce/cooldown pattern from caching.md
    Pattern 7.

**Files modified:**
- `app/lib/services/portfolio_service.dart`
- `app/lib/data/repositories/portfolio_repository.dart`
- `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart`

**Tests:**
- `app/test/presentation/viewmodels/portfolio_inbox_view_model_test.dart`

---

### Phase 2: Week navigator + metric rings + day selector widgets

**Goal:** Build the three new header widgets.

**`app/lib/presentation/widgets/portfolio/week_navigator.dart`:**
- Row: `‹` button, center label button ("This Week" / "Mar 9 – Mar 15"), `›`
  button.
- Center button taps → reset to week 0.
- Forward button disabled (muted) when `isCurrentWeek`.
- Coral border on center button when `isCurrentWeek`.

**`app/lib/presentation/widgets/portfolio/metric_rings_row.dart`:**
- Row of 3 tappable `MiniRing` widgets (58×58).
- Each ring: circular progress indicator (CustomPainter), value text centered,
  goal text beneath.
- **[AUDIT FIX]** Ring colors must use `AppColors` helpers, not hardcoded values.
  Use `AppColors.transferSage` for ≥100%, a new `AppColors.sageSoft` (or
  existing `transferSage.withValues(alpha:)`) for ≥50%, and
  `AppColors.textTertiary(context)` for <50%. All text colors via
  `AppColors.textPrimary(context)` / `AppColors.textSecondary(context)`.
- `onTap(metricKey)` callback for opening metric detail.

**`app/lib/presentation/widgets/portfolio/day_selector.dart`:**
- 7-column row in a rounded card.
- Each column: day abbrev, date number, up to 5 dots.
- Animated coral border around selected column (AnimatedPositioned or
  AnimatedContainer).
- **[AUDIT FIX]** Dot/text colors must use `AppColors` helpers:
  `AppColors.primary(context)` for coral/selected, `AppColors.textTertiary(context)`
  for muted. Card background via `AppColors.cardBackground(context)`, border via
  `AppColors.border(context)`. No hardcoded `Color(0xFF...)` values.
- Today's date number always `AppColors.primary(context)` (even when not
  selected), past days `AppColors.textTertiary(context)` when on current week.

**Files created:**
- `app/lib/presentation/widgets/portfolio/week_navigator.dart`
- `app/lib/presentation/widgets/portfolio/metric_rings_row.dart`
- `app/lib/presentation/widgets/portfolio/day_selector.dart`

**Tests:**
- `app/test/presentation/widgets/portfolio/week_navigator_test.dart`
- `app/test/presentation/widgets/portfolio/metric_rings_row_test.dart`
- `app/test/presentation/widgets/portfolio/day_selector_test.dart`

---

### Phase 3: Action card widget (initial)

**Goal:** Build the initial item card widget. This phase creates the scaffold;
Phase 7 refines the layout to match the final `inbox-item` reference.

**`app/lib/presentation/widgets/portfolio/inbox_action_card.dart`:**

Two-zone layout (see Phase 7 for the full card spec):

**Image hero zone** (ClipRRect top, 16px radius, min-height ~150px):
1. **Background layer:** `CachedMediaImage` from `backgroundMediaId`, or
   gradient fallback based on item type (same pattern as `DiscoverHelpCard`).
2. **Gradient overlay:** Linear gradient — subtle at top, ~50% black at bottom.
3. **Row 1 (top):** `[Avatar] [First name] [in Community Name]` left,
   `[Badge or time]` right. Community name from `DailyItem.communityName`.
   For experiences, the badge is a clock icon + formatted date/time string
   (not a type label).
4. **Row 2 (bottom, margin-top auto):** Item title, 20px bold white.

**Frosted bottom bar** (dark translucent + backdrop blur):
5. **Comment row** (if has comment): avatar + text.
6. **Action row:** overlapping commenter avatars or chat placeholder (left),
   action button flush-right (right). Button label/style from
   `DailyItem.actionButtonLabel` / `actionButtonStyle`.

**[AUDIT FIX]** The card widget must NOT navigate directly. It exposes
`onActionTap` and `onNoteTap` callbacks. The parent screen handles navigation
via `NavigationHelpers.pushScreen()` per architecture.md. This keeps the widget
testable without a Navigator.

**[AUDIT FIX]** Use `CachedMediaImage` with `ImageCacheKeys.thumbnail(mediaId)`
for card backgrounds (< 200px display context per caching.md). Gradient overlay
colors use `Colors.black.withValues(alpha:)` which is acceptable for overlay
opacity blending (not a theme color).

**Files created:**
- `app/lib/presentation/widgets/portfolio/inbox_action_card.dart`

**Tests:**
- `app/test/presentation/widgets/portfolio/inbox_action_card_test.dart`

---

### Phase 4: Metric detail screen redesign

**Goal:** Upgrade `MetricDetailScreen` to match the reference.

Replace or extend the existing `MetricDetailScreen` with:

1. **Back button** (chevron left).
2. **Title** centered in app bar.
3. **Hero value** — large italic serif text (52px) in coral.
4. **Subtitle** — description of the metric.
5. **Insight quote** — italic serif (15px), contextual encouragement.
6. **Sparkline card** — rounded card with:
   - "SOCIAL TIME OVER WEEK" label.
   - SVG-like polyline chart (use `CustomPaint`) with 7 data points (Mon–Sun).
   - Filled dot on the latest point.
   - Mon/Sun labels beneath.
7. **Weekly goal row** — icon + "Weekly goal: 6h" text.
8. **Activity list** — "MOST SOCIAL ACTIVITIES" header, then sorted rows with
   title, person + status, and value + chevron.

Since the existing `MetricDetailScreen` is used from the portfolio inbox, we can
modify it in place. Add optional fields for sparkline data, insight text, and
goal text.

**Files modified:**
- `app/lib/presentation/screens/daily/metric_detail_screen.dart` (extend)
- `app/lib/presentation/models/metric_detail_item.dart` (no change expected)

**New widget:**
- `app/lib/presentation/widgets/portfolio/sparkline_chart.dart` — CustomPainter
  for the 7-point line chart.

**Tests:**
- `app/test/presentation/widgets/portfolio/sparkline_chart_test.dart`
- `app/test/presentation/screens/daily/metric_detail_screen_test.dart` (update)

---

### Phase 5: Portfolio inbox screen rewrite

**Goal:** Replace the current `PortfolioInboxScreen` build with the new layout.

New screen structure (top to bottom in `CustomScrollView`):

1. `FloatingHeader` spacing.
2. `WeekNavigator` — week back/forward/reset.
3. `MetricRingsRow` — three tappable progress rings.
4. `DaySelector` — 7-day bar with dots.
5. **Day label** — "Today", "Yesterday", "Monday, Mar 16", etc.
6. **Item list** — `InboxActionCard` for each item on the selected day.
7. **Empty state** — "Nothing that day" card when no items.
8. Bottom spacing.

Remove the current `WeeklyDayCard`, `WeeklyMetricsCarousel`, section headers,
`DailyItemRow` usage, `DailyFutureRow` usage, and "Coming Up" groups from
this screen.

The view model drives everything: changing `selectedDayIndex` rebuilds the item
list; changing `weekOffset` triggers a fetch and resets the day selector.

**Files modified:**
- `app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart` (rewrite)

**Tests:**
- `app/test/presentation/screens/portfolio/portfolio_inbox_screen_test.dart`

---

### Phase 6: Weekly goals UI

**Goal:** Let users view and edit their weekly goals.

- Add a "Weekly goal: 6h" row in the metric detail screen (from Phase 4) that
  is tappable → opens a small bottom sheet with a number picker or text input.
- On save, calls `setWeeklyGoals` through the repository; refreshes the week
  data to update rings.
- Goals persist server-side and appear in `WeeklyMetrics` on every load.

**Files created:**
- `app/lib/presentation/widgets/portfolio/weekly_goal_editor.dart` — bottom
  sheet with three fields (social minutes, dollars, kg CO₂).

**Files modified:**
- `app/lib/presentation/screens/daily/metric_detail_screen.dart` (add tap on
  goal row)
- `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart` (add
  `setWeeklyGoals` action)

**Tests:**
- `app/test/presentation/widgets/portfolio/weekly_goal_editor_test.dart`

---

### Phase 7: Item day-placement rules and card redesign

**Goal:** Define how items land on specific days, show community context on each
card, and handle multi-community items as separate cards.

#### Item → day placement rules

An item appears on a given day if any of these conditions are true for that day:

| Item type | Placement trigger |
|-----------|-------------------|
| **Loan (lending/borrowing)** | Day it was shared with a community; estimated pickup day; estimated return day (pickup + loan duration); day of any state transition (confirmed, picked up, returned) |
| **Giveaway** | Day it was shared; day of recipient selection; day of pickup |
| **Experience** | Day it was shared; scheduled start date/time |
| **Request (help)** | Day it was shared; day an offer was accepted |

The server's `GetPortfolioInboxWeek` handler in `inbox_week.go` computes which
day bucket(s) each item belongs to. An item can appear on multiple days within
the same week (e.g., a loan shared on Monday with pickup on Wednesday appears on
both days).

For **past weeks**, the server includes completed/terminal items that had
activity during that week (state transitions, messages, sharing events).

For the **current week**, future days show items with scheduled events
(experience start times, estimated pickups/returns). Past days show items that
had activity on that day.

#### Multi-community cards

If the same underlying item (e.g., a piece of gear) is shared with multiple
communities, the server emits one `DailyItem` per community. Each `DailyItem`
already carries `community_name` (field 22, `optional string`). The client
renders a separate `InboxActionCard` for each, showing the community name on the
card.

#### Card layout update (replaces Phase 3 description)

The card layout is updated based on the `inbox-item.jsx` / `inbox-item.png`
reference. The card has two distinct zones:

**Image hero zone** (top, min-height ~150px, background image + gradient):
- **Row 1 (top):** `[Avatar] [First name] [in Community Name]` left-aligned,
  `[Badge OR time]` right-aligned.
  - Avatar: 26px `UserAvatar` with white border.
  - First name: 13px semibold white with text shadow.
  - Community: 12px `rgba(255,255,255,0.45)`, prefixed with "in", ellipsis
    overflow.
  - Badge (non-experience): type pill — "Lending", "Borrowing", "Helping",
    "Help", "Giving" in uppercase with variant-colored translucent background.
  - Badge (experience): clock icon + formatted date/time string (e.g.,
    "Sat, Mar 1 · 9 AM") in translucent background. No "Event" label.
  - If no person is assigned (e.g., open request), show a `?` placeholder
    avatar.
- **Row 2 (bottom, pushed down with `marginTop: auto`):** Item title, 20px
  bold white with text shadow.

**Frosted glass bottom bar** (dark translucent `rgba(20,18,16,0.82)` +
backdrop blur):
- **Comment row** (if has last comment): avatar (24px) + comment text (13px,
  `rgba(255,255,255,0.75)`), separated from action row by a subtle divider.
- **Action row:**
  - Left side (if has comments): overlapping mini-avatars of commenters (max 3,
    22px, stacked with -8px margin) + "View all N comments" text.
  - Left side (if no comments): chat icon + "Ask a question or leave a note…"
    placeholder.
  - Right side: action button flush-right, no border-radius, full height.
    Button label/color varies by context:
    - Experience with RSVP "YES": sage green, "You're Going"
    - Experience with RSVP "MAYBE": coralSoft, "Maybe"
    - Experience without RSVP: coral, "RSVP"
    - All other items: coral, "Review" (or "View" for read-only)

**Key differences from original Phase 3 design:**
- Community name is shown on every card (Row 1, after person name).
- Experience badge is a date/time string with clock icon, not a "Completed" or
  "Event" pill.
- Bottom bar is a separate frosted-glass zone (not inside the gradient overlay).
- Comment preview is a dedicated row above the action row.
- Commenter avatars are overlapping mini-avatars, not inline with unread count.
- Action button is full-height flush-right (not a rounded pill).

#### Proto changes

No new proto fields needed — `DailyItem` already has:
- `community_name` (field 22, `optional string`) — populated by the server.
- `last_message_sender` (field 15) + `last_message_text` (field 16) — for
  comment preview.
- `type_label` (field 17) — "Lending", "Borrowing", "Helping", etc.
- `time_display` (field 4) — formatted time for experiences.

Add one new field to support the commenter avatar stack:

```protobuf
// Add to DailyItem:
// People who have commented in the conversation, most recent first.
// Used for the overlapping avatar stack on the card. Max 3 entries.
repeated DailyPerson commenters = 23;
```

And add a field for the action button state:

```protobuf
// Add to DailyItem:
// Action button label, e.g. "Review", "RSVP", "You're Going", "View".
string action_button_label = 24;
// Action button style hint: "coral", "sage", "coral_soft".
string action_button_style = 25;
```

#### Server changes

- `server/services/portfolio/inbox_week.go` — implement day-placement logic per
  the rules table above. For each item, compute which day indices (0–6) it
  belongs to based on timestamps and the week's Monday–Sunday bounds.
- `server/services/portfolio/assembly.go` — populate `commenters` (top 3 from
  messages, excluding the current user, most recent first), `action_button_label`,
  and `action_button_style` on each `DailyItem`.

#### Client changes

- `app/lib/presentation/widgets/portfolio/inbox_action_card.dart` — update to
  match the new two-zone layout (image hero + frosted bottom bar). Add community
  name display, experience time badge, comment row, overlapping avatars.

#### i18n additions

| Key | English |
|-----|---------|
| `portfolioInboxInCommunity` | `in {name}` |
| `portfolioInboxViewAllComments` | `View all {count} comments` (ICU plural) |
| `portfolioInboxAskQuestion` | `Ask a question or leave a note…` |
| `portfolioInboxYoureGoing` | `You're Going` |
| `portfolioInboxMaybe` | `Maybe` |
| `portfolioInboxRsvp` | `RSVP` |
| `portfolioInboxView` | `View` |

**Files modified:**
- `proto/ripls/api/portfolio.proto` (add fields 23–25 to `DailyItem`)
- `server/services/portfolio/inbox_week.go` (day-placement logic)
- `server/services/portfolio/assembly.go` (commenters, action button fields)
- `app/lib/presentation/widgets/portfolio/inbox_action_card.dart` (redesign)
- `app_en.arb`, `app_es.arb` (new keys)

**Tests:**
- `server/services/portfolio/inbox_week_test.go` (day-placement edge cases:
  item on multiple days, multi-community duplication, past-week terminal items)
- `app/test/presentation/widgets/portfolio/inbox_action_card_test.dart` (update
  for new layout: community name, experience time badge, comment row, action
  button variants)

---

### Phase 8: Cleanup and migration

**Goal:** Remove deprecated code and ensure clean transition.

- Deprecate `GetPortfolioInboxView` RPC (keep server handler for older clients
  but mark proto as deprecated).
- Remove unused widgets: `WeeklyDayCard` (if only used here), old
  `WeeklyMetricsCarousel` (if only used here), `DailyItemRow` references from
  portfolio screen.
- Remove `DailyFutureRow` references from portfolio screen.
- **[AUDIT FIX]** Cache invalidation for week data was moved to Phase 1 (it
  must be wired when the ViewModel is built, not deferred).
- Run full test suite and fix any breakage.
- Run `npm run lint:dart:i18n` to verify no hardcoded strings slipped through.

**Files modified:**
- `proto/ripls/api/portfolio.proto` (deprecation annotations)
- Various widget files (remove dead imports)
- `app/lib/data/repositories/portfolio_repository.dart` (cleanup only; cache
  invalidation already wired in Phase 1)

---

### Phase 9: Tests

**Goal:** Comprehensive test coverage for all new code.

- Server: unit + DB integration tests for `GetPortfolioInboxWeek` (multiple
  weeks, edge cases for week boundaries, empty weeks, timezone handling).
- Server: tests for `SetWeeklyGoals` (create, update, defaults).
- Server: day-placement tests (item on multiple days, multi-community
  duplication, experience scheduled time vs. sharing day, loan return date
  calculation, past-week terminal items).
- Client widgets: render tests for `WeekNavigator`, `MetricRingsRow`,
  `DaySelector`, `InboxActionCard` (community name, experience time badge,
  comment row, action button variants, multi-community cards),
  `SparklineChart`, `WeeklyGoalEditor`.
- Client view model: state management tests for week navigation, day selection,
  goal setting, cache invalidation.
- Client screen: integration-style tests for `PortfolioInboxScreen`.

Target: tests written alongside each phase (listed above), with Phase 9 as a
catch-all for any gaps.

### Phase 10: Community-scoped content views

**Goal:** When a user views their own inventory item (gear, experience, or
request), the content view header shows "Shared with **[Community Name]**" with
a dropdown if the item is shared with multiple communities. Selecting a different
community changes the comment preview, comment count, action status/button, and
floating action button behavior to reflect that community's context.

#### Current architecture

- `GearContentView`, `ExperienceContentView`, and `RequestContentView` already
  accept a community context via `selectedCommunityProvider` and pass
  `communityId` to their ViewModel's `initialize()` / `loadDetails()`.
- `GetGearResponse` (and the equivalent experience/request responses) already
  return community-scoped fields: `conversation_id`, `message_count`,
  `unread_count`, `last_message_text`, `last_message_sender`,
  `last_message_time_ago`, `recent_commenters`, and `shared_at_unix_sec`.
- Changing `selectedCommunityProvider` already triggers a reload of the detail
  with the new community context.
- The feed header (`ContentViewBuilders.buildFeedHeader`) shows
  `"$actionText • $timeAgo"` but only when coming from the feed screen. When
  viewing from the portfolio inbox or directly, this header is not shown.

#### What needs to change

**1. New shared widget: `CommunityContextHeader`**

File: `app/lib/presentation/widgets/content/community_context_header.dart`

A widget that replaces (or sits below) the current feed header area on content
views when the user is viewing their own item. Layout matches the reference
designs:

- **Single community (or viewing from within a community):**
  `"Shared with **Breck Neighbors** · 4d ago"` — plain text, no dropdown.
- **Multiple communities (viewing from portfolio / inventory):**
  `"Shared with **Breck Neighbors** ˅ · 4d ago"` — tapping the community name
  opens a dropdown popover showing all communities the item is shared with, each
  with their avatar and member count. The selected community has a checkmark.
  A "Share with another group" row with a `+` icon at the bottom opens the
  existing community sharing modal.

Widget parameters:
```dart
class CommunityContextHeader extends ConsumerWidget {
  /// Communities the item is shared with, in display order.
  final List<SharedCommunityInfo> communities;
  /// Currently selected community ID.
  final String selectedCommunityId;
  /// Relative time since sharing (e.g., "4d ago").
  final String sharedTimeAgo;
  /// Called when the user selects a different community.
  final void Function(String communityId) onCommunitySelected;
  /// Called when the user taps "Share with another group".
  final VoidCallback? onShareWithAnother;
}
```

The dropdown is a `showMenu()` or custom `OverlayEntry` positioned below the
community name, matching the reference design (dark popover with community
avatars, names, member counts, checkmark on selected, + "Share with another
group" at the bottom).

**2. New data class: `SharedCommunityInfo`**

File: `app/lib/presentation/models/shared_community_info.dart`

```dart
class SharedCommunityInfo {
  final String communityId;
  final String communityName;
  final String mediaId; // for community avatar
  final int memberCount;
  final int64 sharedAtUnixSec;
}
```

**3. Proto/server changes**

The server already returns community-scoped data per request. What's missing is
a list of all communities an item is shared with (for the dropdown). Options:

- **(a) New fields on GetGearResponse / GetExperienceResponse / GetRequestResponse:**
  Add `repeated SharedCommunity shared_communities` to each response, populated
  by querying `CommunityGear` / `CommunityExperience` / `CommunityRequest` for
  the item.
- **(b) Reuse existing ListUserGear / community endpoints:** The client already
  knows which communities the user belongs to. Cross-reference with the item's
  sharing state.

**Recommendation:** Option (a). Add a `SharedCommunity` message to each service
proto:

```protobuf
message SharedCommunity {
  string community_id = 1;
  string community_name = 2;
  string media_id = 3;       // community avatar
  int32 member_count = 4;
  int64 shared_at_unix_sec = 5;
}
```

Add `repeated SharedCommunity shared_communities = N` to `GetGearResponse`,
`GetExperienceResponse`, and `GetRequestResponse`. The server populates this
by querying all `CommunityGear` / `CommunityExperience` / `CommunityRequest`
entries for the item, joining with community data. This is a small addition
since the server already fetches community membership.

**4. Content view changes**

In each of the three content views (`gear_content_view.dart`,
`experience_content_view.dart`, `request_content_view.dart`):

- When the user is the owner and `sharedCommunities.length >= 1`, render
  `CommunityContextHeader` at the top of the content overlay (below the
  gradient, above the title).
- When viewing from within a specific community (single community context),
  show the non-dropdown variant.
- When viewing from portfolio/inventory with multiple communities, show the
  dropdown variant.
- `onCommunitySelected` calls the ViewModel to reload with the new community
  context (same as the existing `selectedCommunityProvider` mechanism, but now
  scoped to the content view rather than the global sidebar community).

**5. ViewModel changes**

Each content ViewModel already accepts `communityId` and reloads on change. The
new requirement is:

- Expose `sharedCommunities` from the response state so the content view can
  render the dropdown.
- When `onCommunitySelected` is called, update the local community context and
  reload. This should NOT change `selectedCommunityProvider` (which is the
  sidebar global community) — it should be a local state change scoped to the
  content view.

Add a `selectedContentCommunityId` field to each content ViewModel's state.
When the user picks a different community from the dropdown, update this field
and call `loadDetails(communityId: newId)`.

#### i18n additions

| Key | English |
|-----|---------|
| `contentSharedWith` | `Shared with {name}` |
| `contentShareWithAnother` | `Share with another group` |
| `contentMemberCount` | `{count, plural, =1{1 member} other{{count} members}}` |

#### Files created
- `app/lib/presentation/widgets/content/community_context_header.dart`
- `app/lib/presentation/models/shared_community_info.dart`

#### Files modified
- `proto/ripls/api/gear_service.proto` (add `SharedCommunity`, add field to response)
- `proto/ripls/api/experience_service.proto` (same)
- `proto/ripls/api/request_service.proto` (same)
- `server/services/gear/` (populate `shared_communities`)
- `server/services/community/` (populate `shared_communities` for experiences)
- `server/services/community/` (populate `shared_communities` for requests)
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/presentation/screens/request/request_content_view.dart`
- `app/lib/presentation/viewmodels/gear_view_model.dart`
- `app/lib/presentation/viewmodels/experience_view_model.dart`
- `app/lib/presentation/viewmodels/request_view_model.dart`
- `app_en.arb`, `app_es.arb` (3 new keys)

#### Tests
- Widget tests for `CommunityContextHeader` (single community, multi-community
  dropdown, selection callback, share-with-another callback)
- ViewModel tests for community switching within content view
- Server tests for `shared_communities` population

---

## Phase 11 — Card Polish, Community-Scoped Navigation, Loan Bug Fix

Three targeted improvements that round out the portfolio inbox experience.

### 11a — Remove comment preview row from InboxActionCard

The card is currently too data-dense. Remove the `_buildCommentPreviewRow` row
entirely so the card shows only the action-bar row (commenter avatars + unread
count label + action button). The comment preview was conveying redundant
information already visible after tapping through.

**Changes to `app/lib/presentation/widgets/portfolio/inbox_action_card.dart`:**

1. Delete `_buildCommentPreviewRow()` method.
2. Delete the `_commentRowHeight` constant (no longer needed).
3. Simplify `build()` — `bottomBarHeight` is always just `_actionRowHeight`
   (remove the `hasComment` branching).
4. Simplify `_buildBottomBar()` — remove the `if (hasComment)` branch; render
   only `_buildActionRow()`.
5. `_buildActionRow()`: The left side always shows the comment-count / leave-note
   row (no change to that content, just remove the old comment preview above it).
6. Remove the `onNoteTap` callback parameter entirely — all taps now call
   `onReviewTap`. Update `InboxActionCard`'s constructor and all call sites in
   `portfolio_inbox_screen.dart`.

The card shrinks by ~44 px and the visual hierarchy becomes cleaner.

### 11b — Pass community ID when opening content from a card

When a user taps a card grouped under community "Breck Neighbors", the
gear/experience/request content view should open with **Breck Neighbors**
already selected as the content community — not whatever the global sidebar
community happens to be.

**How it works today:** `_navigateToContent` in `portfolio_inbox_screen.dart`
pushes `GearScreen(gearId: item.contentId)` etc. with no community hint. The
content view picks up `selectedCommunityProvider` (global sidebar) on load.

**Change:** Pass `item.communityId` (already present on every `DailyItem` via
field 26) as an optional `initialCommunityId` parameter to each screen.

#### Screen / content view changes

Add `final String? initialCommunityId` to:
- `GearScreen` → forwarded to `GearContentView`
- `ExperienceScreen` → forwarded to `ExperienceContentView`
- `RequestScreen` → forwarded to `RequestContentView`

In each content view, if `initialCommunityId` is non-null and non-empty,
override the community used for the initial data load instead of reading
`selectedCommunityProvider`. Store it as local `selectedContentCommunityId`
state in the view model (introduced by Phase 10). This prevents the global
sidebar community from being changed just because the user tapped an inbox card.

#### Portfolio inbox screen changes

In `_navigateToContent`:
```dart
case DailyItemType.DAILY_ITEM_TYPE_TRANSFER:
  NavigationHelpers.pushScreen(
    context: context,
    screen: GearScreen(
      gearId: item.contentId,
      initialCommunityId: item.hasCommunityId() ? item.communityId : null,
    ),
    useRootNavigator: true,
  ).then((_) => _onReturnFromScreen());
```
Apply the same pattern for `ExperienceScreen` and `RequestScreen`.

### 11c — Fix loan item card: hex title and missing background image

**Symptom:** Transfer (loan/giveaway) cards in the portfolio inbox show a
hexadecimal string as the title (the raw gear ID) and have no background image.

**Root cause:** In `server/services/portfolio/fetch.go`, the gear batch fetch
(step 7) collects gear IDs only from `activeTransfers`:

```go
// fetch.go — current (buggy)
gearIDs := make([]string, 0, len(activeTransfers))
for _, t := range activeTransfers {
    gearIDs = append(gearIDs, t.GearId)
}
```

But `inbox_week.go` iterates over `d.allTransfers` (which includes
completed/cancelled transfers for historical week views):

```go
// inbox_week.go
for _, t := range d.allTransfers {
    ...
    if g, ok := d.gearProtoMap[t.GearId]; ok { ... } // miss!
    // fallback: gearName = t.GearId  ← hex string shown on card
}
```

Any transfer in `allTransfers` but not `activeTransfers` has its gear missing
from `gearProtoMap`, producing the hex title and no background image.

**Fix in `server/services/portfolio/fetch.go`:** Collect gear IDs from
`allTransfers` instead of `activeTransfers`:

```go
// fetch.go — fixed
gearIDs := make([]string, 0, len(d.allTransfers))
for _, t := range allTransfers {
    gearIDs = append(gearIDs, t.GearId)
}
```

(At this point in `fetchAll`, the local variable is `allTransfers`, not
`d.allTransfers`.)

No proto changes, no client changes.

#### Files modified

| File | Change |
|------|--------|
| `app/lib/presentation/widgets/portfolio/inbox_action_card.dart` | Remove comment preview row and `onNoteTap` param |
| `app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart` | Pass `initialCommunityId` to content screens; remove `onNoteTap` |
| `app/lib/presentation/screens/gear/gear_screen.dart` | Add `initialCommunityId` param |
| `app/lib/presentation/screens/experience/experience_screen.dart` | Add `initialCommunityId` param |
| `app/lib/presentation/screens/request/request_screen.dart` | Add `initialCommunityId` param |
| `app/lib/presentation/screens/gear/gear_content_view.dart` | Accept and apply `initialCommunityId` |
| `app/lib/presentation/screens/experience/experience_content_view.dart` | Accept and apply `initialCommunityId` |
| `app/lib/presentation/screens/request/request_content_view.dart` | Accept and apply `initialCommunityId` |
| `server/services/portfolio/fetch.go` | Collect gear IDs from `allTransfers` instead of `activeTransfers` |

#### Tests

- Widget tests for simplified `InboxActionCard` (no comment row, single onTap callback)
- Unit test for `fetchAll` gear map: verify that a completed transfer's gear
  name and media ID are populated correctly
- Navigation test: tapping a card with `communityId` opens content view with
  that community pre-selected

---

### Cross-Cutting: i18n (applies to all client phases)

> **[AUDIT FIX]** Per `docs/client/i18n.md`, every user-visible string must use
> `context.l10n.*` from ARB files. The plan originally omitted i18n entirely.

**New ARB keys required** (add to `app_en.arb` and `app_es.arb`):

| Key | English | Usage |
|-----|---------|-------|
| `portfolioInboxThisWeek` | `This Week` | Week navigator center label |
| `portfolioInboxReview` | `Review` | Action card button |
| `portfolioInboxLeaveNote` | `Leave a note…` | Action card placeholder |
| `portfolioInboxUnread` | `{count} unread` | Action card unread count (ICU plural) |
| `portfolioInboxAllCaughtUp` | `All caught up` | Action card no-unread state |
| `portfolioInboxNothingThatDay` | `Nothing that day` | Empty day card |
| `portfolioInboxToday` | `Today` | Day label |
| `portfolioInboxYesterday` | `Yesterday` | Day label |
| `portfolioInboxTomorrow` | `Tomorrow` | Day label |
| `portfolioMetricSocialTime` | `Social Time` | Metric detail title |
| `portfolioMetricMoneySaved` | `Money Saved` | Metric detail title |
| `portfolioMetricCo2Avoided` | `CO₂ Avoided` | Metric detail title |
| `portfolioMetricOverWeek` | `{metric} Over Week` | Sparkline card label |
| `portfolioMetricWeeklyGoal` | `Weekly goal: {value}` | Goal row |
| `portfolioMetricMostSocial` | `Most Social Activities` | Activity list header |
| `portfolioMetricMostValuable` | `Most Valuable Items` | Activity list header |
| `portfolioMetricBiggestImpact` | `Biggest Impact Items` | Activity list header |

**Widgets must not contain hardcoded English strings.** Server-provided strings
(day labels in `InboxDay.date_label`, metric values, type labels) are already
localized server-side and pass through directly. Client-only UI chrome uses
`context.l10n.*`.

**Day labels ("Today", "Yesterday", full dates):** Computed server-side in
`InboxDay.date_label` to keep timezone logic on the server. But the short
relative labels ("Today", "Yesterday", "Tomorrow") should also have ARB keys as
fallbacks for client-side display when the response hasn't loaded yet.

---

## File Impact Summary

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 0 | fetch.go, inbox_week.go, goals.go, inbox_week_test.go, goals_test.go, UserWeeklyGoals model | portfolio.proto, user.proto, inbox_view.go (refactor), protosql_schema.go, metrics.go, main.go |
| 1 | — | portfolio_service.dart, portfolio_repository.dart, portfolio_inbox_view_model.dart |
| 2 | week_navigator.dart, metric_rings_row.dart, day_selector.dart + tests | — |
| 3 | inbox_action_card.dart + test | — |
| 4 | sparkline_chart.dart + test | metric_detail_screen.dart |
| 5 | — | portfolio_inbox_screen.dart |
| 6 | weekly_goal_editor.dart + test | metric_detail_screen.dart, portfolio_inbox_view_model.dart |
| 7 | — | portfolio.proto (fields 23–25), inbox_week.go (day-placement), assembly.go (commenters, action button), inbox_action_card.dart (redesign) |
| 8 | — | portfolio.proto (deprecation), various cleanup |
| 10 | community_context_header.dart, shared_community_info.dart | gear/experience/request service protos (SharedCommunity + shared_communities), server services, content view + viewmodel files, app_en.arb, app_es.arb |
| 11 | — | inbox_action_card.dart (remove comment row), portfolio_inbox_screen.dart (community nav), gear/experience/request screen + content view files (initialCommunityId), fetch.go (gear ID fix) |
| i18n | — | app_en.arb, app_es.arb (24+ new keys, added alongside each client phase) |
