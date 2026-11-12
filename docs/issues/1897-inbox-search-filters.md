# Issue #1897: Inbox: floating-nav search bar + filter pills (Needs attention / Upcoming / My stuff / Done)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1897
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/mocks/simple/new_inbox.png](../cowork/mocks/simple/new_inbox.png), [docs/cowork/App Design/direction-a-working-mock.html](../cowork/App%20Design/direction-a-working-mock.html) (TodoHeader/TodoScreen, lines 1059–1189), [docs/client/inbox.md](../client/inbox.md)

## Problem Statement

After #1895 ships, the Inbox's `FloatingHeader` loses its hamburger
icon and its leading slot becomes empty. This issue uses that space
to add a **"Search inbox"** input affordance and pushes a row of
**filter pills** below the header so users can scope the inbox to:

- **Needs attention** — items requiring the user's next action.
- **Upcoming** — items with a scheduled future moment (events, return
  due dates).
- **My stuff** — items the user owns/created.
- **Done** — recently-completed items.

Both surfaces are net-new UI on top of the existing flat inbox list.
The `GetPortfolioInboxFeed` RPC and the [`PortfolioInboxNotifier`](../../app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart)
ViewModel stay intact; the new pieces are presentation-level filtering
plus search-text matching.

The "Done" segment is structurally different from the other three.
Per [inbox.md](../client/inbox.md), the inbox today **explicitly
excludes** completed/cancelled items (their watches are auto-dismissed
on terminal events). #1897 lights up the Done tab by adding an
`include_done: bool` request parameter to `GetPortfolioInboxFeed`;
when set, the server appends terminal-state items the viewer
participated in (last 7 days, capped) to the same response. See
Phase 2.

## Context from Comments

No comments. The issue body and the `direction-a-working-mock.html`
`TodoHeader`/`TodoScreen` block (lines 1059–1189) are the
specification. Two pieces of behavior come from the mock that the
issue text doesn't mention:

- **Pill labels carry counts** — e.g. `"Needs you · 3"`,
  `"Upcoming · 2"`. The mock computes count = `items.length` for
  each tab.
- **Each pill has an empty-state body** — "Needs attention" empty
  surfaces a "Set help areas →" CTA; "Done" empty surfaces "Nothing
  closed this week." (See the `TodoScreen` empty-state JSX block
  starting at HTML line 1167.)

The mock's default filter is `'needs'`.

## Audit Notes (2026-05-12)

Audited against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/modals.md](../client/modals.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/testing.md](../client/testing.md),
[docs/client/inbox.md](../client/inbox.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md).

Most of the plan aligns with established patterns. Folded-in
corrections:

- **`InboxSearchQueryNotifier` disposal safety made explicit.** The
  Notifier holds a 200ms debounce `Timer` and is `autoDispose`. Per
  [architecture.md "Resource Cleanup"](../client/architecture.md#resource-cleanup),
  the Timer must be cancelled in `ref.onDispose(...)` so it doesn't
  fire after the user leaves the screen. Phase 1's test list adds a
  disposal-safety test that disposes the container while a debounce
  is pending and asserts no state writes / no exceptions.
- **Proto-comment style example added.** Phase 2's proto-comment
  guidance was abstract ("describe semantics only"). The plan now
  shows a concrete acceptable comment for `is_action_required` so
  the implementer doesn't have to interpret the CLAUDE.md proto-
  comment rule from scratch.
- **Server observability stance confirmed.** Per [observability.md](../server/observability.md),
  user-facing errors are INFO/WARN, ERROR is reserved for system
  failures, and the existing per-RPC log already covers
  `GetPortfolioInboxFeed`. Phase 2 deliberately adds no new log
  lines — the audit confirms that's the correct posture, not a gap.
  If the server-side classification logic ever needs to surface a
  weird-data case, use `WARN` with the standard field names
  (`operation`, `target_request_id` for `models.Request` entity IDs,
  etc.) — never raw `log.Printf` per [CLAUDE.md "Structured Logging
  (Go Server)"](../../CLAUDE.md#structured-logging-go-server).
- **Server error-wrapping rule applies to the Phase 2 work.**
  `connecterr.Internal(ctx, op, err, kv...)` (not bare
  `connect.NewError(connect.CodeInternal, err)`) — gated by
  `scripts/check_codeinternal.js`. The completed-items fetch path
  added in Phase 2 will pass storage errors through the inbox-feed
  RPC handler; the handler must wrap them via `connecterr.Internal`.
- **Cache strategy verified.** New `DailyItem` fields cache
  transparently under the existing `portfolio:inbox_feed` namespace.
  No new cache namespaces, no invalidation changes.
- **Service Independence verified.** Phase 2's server work stays
  inside `server/services/portfolio/` (`assembly.go`,
  `inbox_feed.go`). No new cross-service dependencies introduced
  per [server architecture.md "Service Independence"](../server/architecture.md#1-service-independence).

## Codebase Survey

- **Screen** — [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart).
  Today: `FloatingHeader(title: portfolioInboxTitle, showMenuIcon: true)`
  + `RefreshIndicator` + `SliverList` of `InboxRow`s. After #1895 the
  `showMenuIcon` argument goes away; the header's leading slot
  becomes empty.
- **ViewModel** — [portfolio_inbox_view_model.dart](../../app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart).
  `PortfolioInboxNotifier` manages `PortfolioInboxState` (Freezed):
  `feedResponse`, `unreadDeltas`, `optimisticallyReadItemIds`,
  `isLoading`, `errorMessage`. Already has debounce + cooldown +
  background refresh wired through `portfolioCacheInvalidationProvider`.
- **Derived providers:**
  - `inboxFlatItemsProvider` — filters `feedResponse.items` by
    `selectedCommunityProvider.enabledCommunityIds`. After #1895 Phase 4
    this filter is inert (always-all-communities). This is the
    natural insertion point for the new search + status filtering.
  - `inboxUnreadCountProvider` — count of items with
    `effectiveIsWatchedUnread(item) == true`. Drives the nav badge.
- **Row** — [inbox_row.dart](../../app/lib/presentation/widgets/portfolio/inbox_row.dart).
  No changes needed for #1897 (the pill / search filtering operates
  on the list, not the row).
- **`DailyItem` proto** — [docs/client/inbox.md "Data Model"](../client/inbox.md#data-model).
  Today carries `title`, `item_type`, `inbox_status_label`,
  `action_description`, `action_button_label`, `unread_count`,
  `last_message_*`, `time_display`, `is_watched_unread`, etc.
  **No discrete `is_owner` / `is_action_required` /
  `scheduled_at_unix_sec` fields today** — the role and action-pending
  state are baked into the localized `inbox_status_label` /
  `action_description` strings. Parsing those strings client-side
  would be brittle (localized) and against [i18n.md](../client/i18n.md)
  "Viewmodels never resolve strings."
- **Server inbox assembly** — [server/services/portfolio/](../../server/services/portfolio/).
  `assembly.go::resolveInboxStatusLabel` already knows the
  owner-vs-non-owner role per item type;
  `assembleTransfer`/`Experience`/`Request`/`SharedGear` already
  decide owner status. Adding classification flags to `DailyItem`
  reuses logic the server already has.
- **Caching** — `portfolio:inbox_feed` namespace. The classification
  flags ride on `DailyItem` and cache transparently with the feed.
  No new cache namespaces.
- **`FloatingHeader.content:` slot** — established as the "header
  search pill" location by #1896 Phase 1 for the Feed. Inbox follows
  the same pattern.

## Potential Solutions

### Option A: Pure client-side filtering with status-string heuristics

- **Approach:** Filter pills derive their membership by string-matching
  `item.inboxStatusLabel` / `item.actionButtonLabel` against known
  English values ("Borrow Request" → Needs attention, "Hosting" →
  My stuff, etc.). Search-text is a simple substring match across
  `title`/`actionDescription`/`lastMessageText`.
- **Pros:** Zero server / proto changes. Smallest diff.
- **Cons:** Status labels are localized (`inboxStatusLabel` comes
  from server-resolved English strings today, but the resolver lives
  on the server and could localize later). String-based
  classification is fragile — adding a new status label silently
  drops items from filters. Violates [i18n.md](../client/i18n.md)
  "Viewmodels never resolve strings" by depending on the *form* of
  display strings.
- **Effort:** Small.

### Option B: Server-side classification flags + client-side filter (recommended)

- **Approach:** Add four boolean / scalar fields to `DailyItem`:
  - `is_owner: bool` — the viewing user is the owner/host/requester.
  - `is_action_required: bool` — the inbox row asks the viewer to do
    something next (e.g. owner with pending borrow request, requester
    with offers received). Server-computed from the same signals
    that drive `action_description` / `action_button_label`.
  - `scheduled_at_unix_sec: int64` (optional) — for items with a
    future-scheduled moment (experiences, transfers with pickup
    times). 0 / unset when not applicable.
  - `is_done: bool` — set on terminal-state items returned via the
    new `include_done: true` request mode (see Done data path below).

  Server populates these in `assembly.go` from logic that already
  exists (the role + pending-state are already computed to drive
  `resolveInboxStatusLabel`/`resolveInboxPill`). Wire-compatible
  proto changes; no migration.

  Client adds `selectedFilterProvider` (a single-select pill state)
  and `searchQueryProvider` (a debounced text input). The existing
  `inboxFlatItemsProvider` extends to apply both filters in turn.

- **Pros:** Server owns classification logic — single source of truth
  per [the_feed.md](../client/the_feed.md) pattern (server owns
  unread rules). Client filters by stable booleans, not localized
  strings. New filter pills won't break when status copy changes.
- **Cons:** Requires proto + server changes (medium scope). "Done"
  still needs its own data path eventually.
- **Effort:** Medium.

### Option C: New `filter:` param on `GetPortfolioInboxFeed`

- **Approach:** Add `filter: InboxFilter` enum
  (`NEEDS_ATTENTION`/`UPCOMING`/`MY_STUFF`/`DONE`) to the RPC. Server
  returns only matching items. Pill counts come from a separate
  counts-only RPC or from returning counts alongside the filtered
  list.
- **Pros:** Cleanest data model. Server filters can use more
  expensive criteria (e.g. recently-completed-within-7-days for
  Done) without bloating the wire payload.
- **Cons:** 4 cache slots (one per filter) — the existing `portfolio:inbox_feed`
  cache key would need to incorporate the filter. Switching filters
  costs a network round-trip. The badge count
  (`inboxUnreadCountProvider`) needs all items, not just one filter
  — so we still need an "all" fetch in parallel. Doubles the cache
  surface for ambiguous benefit.
- **Effort:** Medium-Large.

## Recommendation

**Option B.** It's the right balance of "server owns the truth" and
"keep the UI snappy with local filtering." Classification flags on
`DailyItem` reuse logic the server already has; the client filter and
search are pure list operations on the cached list — no extra network
trips when switching pills.

Done ships in v1 with a real data path: an `include_done: bool`
parameter on `GetPortfolioInboxFeed` that, when set, appends
terminal-state items the viewer participated in (last 7 days, capped
at 50) to the same response. The client toggles this on in the
repository request and filters on the new `is_done` flag client-side.
This keeps all four pills on the same unified-list pattern — no
parallel RPC, no separate cache namespace.

## Resolved Decisions (2026-05-12 issue review)

All open questions resolved during issue review with the project
owner. Decisions are folded into the Implementation Plan below;
recorded here for the audit trail.

- **"Needs attention"** = `is_action_required == true`. Concrete
  server-side spec:
  - Transfer (owner side): one or more transfers in
    `INTEREST_EXPRESSED` not yet acted on.
  - Giveaway (owner side): one or more interested users not yet
    selected.
  - Request (owner side): `REQUEST_STATE_OFFERS_RECEIVED` with
    unaccepted offers.
  - Experience (owner side): pending RSVPs needing approval (if that
    flow exists; otherwise no experiences fire this).
  - Any item where `is_watched_unread == true` AND the unread
    activity is a non-system message from a non-self author after
    `effectiveLastSeenAt(view)`.

      Fallback when precise classification is hard:
      `is_action_required = (is_watched_unread && action_description.isNotEmpty)`.
- **"Upcoming"** = `scheduled_at_unix_sec > now`. Experiences use
  `Experience.scheduled_at_unix_sec`; transfers use the pickup time
  when set. Items without a scheduled time never appear in Upcoming.
- **"My stuff"** = `is_owner == true`. **Items can appear in
  multiple tabs simultaneously** — a borrow request the user owns
  and must act on shows up in both "Needs attention" **and** "My
  stuff". No priority ordering, no mutual exclusion. Each pill's
  count is independent (counts may sum to more than the total inbox
  size, which is fine).
- **"Done"** = completed items the user participated in (terminal-
  state transfers/giveaways/experiences/requests where the user was
  owner, borrower, attendee, requester, or helper). Ships in v1 with
  a real data path — not stubbed. Implementation in Phase 2 (server)
  + Phase 3 (client filter).
- **Pill counts shown on the pill** (mock format: `"Needs you · 3"`).
  Localized ICU plural strings.
- **Filter selection is not persisted** across app launches —
  always defaults to "Needs attention" on cold start.
- **Search is scoped to the current pill.** Switching pills keeps
  the search query active; the matches just operate against a
  different base set.
- **Search bar lives in `FloatingHeader.content:`** with idle/focused
  states. In-place single-line `TextField` — no separate suggestions
  sheet (justified for the Feed's open-ended search; not needed for
  the bounded inbox).
- **Pull-to-refresh and per-row unread dots stay** — the new filter
  row is purely additive. `inboxUnreadCountProvider` (nav badge)
  continues to count all unread items regardless of selected pill.

## Implementation Plan

### Phase 1: Add the search bar to the floating header

**Goal:** Ship the inbox search affordance. No filter pills yet — the
search bar filters the existing flat list across all items. Validates
the `FloatingHeader.content:` pattern on the Inbox (parity with
#1896's Feed) and gives users the search win immediately.

**Tasks:**
- [ ] Add new l10n keys to **both** [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb)
      **and** [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb):
  - `inboxSearchPlaceholder` — "Search inbox".
  - `inboxSearchEmpty` — empty-results copy when the query
    matches nothing.
  - `a11yInboxSearchInput` — `Tappable`/`TextField` semantic
    label for the search input.
  - `a11yInboxSearchClear` — `IconAction` label for the clear-search
    button.

      Run `npm run generate`.
- [ ] Build the inbox-search pill widget at
      `app/lib/presentation/widgets/portfolio/inbox_search_pill.dart`.
  - Reuse the visual treatment of the Feed's search-nearby pill from
    #1896 (consistent header-search affordance). If the Feed widget
    is reusable as-is, import it directly; otherwise extract a
    shared `HeaderSearchPill` widget in `app/lib/presentation/widgets/search/`
    that both can use.
  - Idle state: search icon + placeholder "Search inbox" text. Wrap
    in [`Tappable`](../../app/lib/presentation/widgets/accessibility/)
    with `semanticsLabel: context.l10n.a11yInboxSearchInput`.
  - On tap: expand to a single-line `TextField` in-place (no
    separate suggestions sheet — see Resolved Decisions for
    rationale). Add a clear-icon ([`IconAction`](../../app/lib/presentation/widgets/accessibility/)
    with `tooltip: context.l10n.a11yInboxSearchClear`) on the right
    when the field is non-empty.
  - Theme-aware colors via `AppColors.*(context)`; no `Colors.white` /
    hardcoded hex per [architecture.md "Colors and Theming"](../client/architecture.md#colors-and-theming).
- [ ] Add `inboxSearchQueryProvider` as a `NotifierProvider.autoDispose<InboxSearchQueryNotifier, String>`
      holding the current query. Methods: `setQuery(String)`,
      `clear()`. Use `SafeNotifierMixin` per [architecture.md "Async
      Safety in Notifiers"](../client/architecture.md#async-safety-in-notifiers).
      Debounce input via a 200ms `Timer` inside the notifier so each
      keystroke doesn't re-filter immediately. **Cancel the debounce
      Timer in `ref.onDispose(...)`** per [architecture.md "Resource
      Cleanup"](../client/architecture.md#resource-cleanup) — without
      this, a pending Timer can fire after the user leaves the inbox
      screen and trigger a state write on a disposed notifier (logged
      by `SafeNotifierMixin` but indicates a real leak).
- [ ] Extend `inboxFlatItemsProvider` (or wrap it in a new
      `inboxVisibleItemsProvider`) that applies the query filter:
      lowercase substring match across
      `item.title`, `item.actionDescription`, `item.lastMessageText`,
      and the rendered `inboxStatusLabel`. Use a stable, pure-Dart
      filter — no l10n string interpretation.
- [ ] Update [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart):
  - Replace `FloatingHeader(title: portfolioInboxTitle, showMenuIcon: true)`
    with `FloatingHeader(content: InboxSearchPill())`. After #1895
    the leading slot is gone; the search pill takes the full content
    width.
  - Watch `inboxVisibleItemsProvider` instead of `inboxFlatItemsProvider`
    so the list reflects the search filter.
  - Render an empty-state when the query yields no matches:
    centered helper text using `context.l10n.inboxSearchEmpty`.
- [ ] **Tests** per [docs/client/testing.md](../client/testing.md):
  - ViewModel test for `InboxSearchQueryNotifier`: debounce coalesces
    rapid `setQuery` calls; `clear()` resets to empty string.
  - **Disposal-safety test** for `InboxSearchQueryNotifier` per
    [architecture.md "Disposal Testing"](../client/architecture.md#disposal-testing):
    call `setQuery("hi")`, immediately `container.dispose()` while
    the 200ms debounce is pending, advance fake time past 200ms, and
    assert no exception and no state-write log line.
  - ViewModel test for `inboxVisibleItemsProvider`: with seeded
    items + empty query, returns all; with query that matches one
    item's title, returns only that item; case-insensitive matching.
  - Widget-level dispatch test for `InboxSearchPill`: tap shows
    field; typing dispatches `setQuery`; clear icon dispatches
    `clear()`.
- [ ] Run `npm run lint:dart:a11y` and `npm run lint:dart:i18n` to
      confirm clean.

**Validates:** Users can search the inbox by free text from the
header. The list re-filters live; empty state copy renders correctly.
No pill row yet — the full list is searchable.

### Phase 2: Server-side classification flags + completed-items data path

**Goal:** Land the proto + server changes that the filter pills need
— including the data path for the "Done" tab. No client UI changes
in this phase — clients ignore the new fields and ignore completed
items until Phase 3.

**Tasks:**
- [ ] **Proto changes** in [proto/ripls/api/](../../proto/ripls/api/)
      (DailyItem is in the portfolio service proto). Add four
      `optional` fields on `DailyItem` (wire-compatible per
      [CLAUDE.md "Proto Field Conventions"](../../CLAUDE.md#proto-field-conventions)):
  - `optional bool is_owner = N;` — viewing user is owner/host/requester.
  - `optional bool is_action_required = N+1;` — the row asks the
    viewer to act.
  - `optional int64 scheduled_at_unix_sec = N+2;` — future scheduled
    moment, when applicable.
  - `optional bool is_done = N+3;` — the item has reached terminal
    state (completed or cancelled).

      Also add one field to the request message
      `GetPortfolioInboxFeedRequest`:
  - `optional bool include_done = M;` — when `true`, the response
    appends terminal-state items the viewer participated in. When
    `false` or unset, the existing inbox semantics apply (terminal
    items are excluded).

      Field comments follow [CLAUDE.md "Proto Comments"](../../CLAUDE.md#proto-comments)
      — no vendor names, transport plumbing, server mechanics, or
      project-convention references. Describe semantics only.
      Example acceptable phrasing for `is_action_required`:

      ```proto
      // True when this row is waiting on the viewer to take the next
      // action — for example, a borrow request awaiting the owner's
      // decision, or an offer awaiting the requester's acceptance.
      // Used by clients to surface action-required items separately
      // from steady-state items.
      optional bool is_action_required = N+1;
      ```

      Do NOT write: "Set in `assembleTransfer` when pending_count > 0",
      "Computed in the inbox feed assembler", "Used by the 'Needs
      attention' filter pill" (the second references server mechanics;
      the third references a client-specific UI surface that may
      evolve).
- [ ] Regenerate code: `npm run generate`.
- [ ] **Server: populate the flags** in [server/services/portfolio/assembly.go](../../server/services/portfolio/assembly.go).
      Each per-type assembler already knows the role and pending
      state:
  - `assembleTransfer` — `is_owner` from `transfer.OwnerId == userId`;
    `is_action_required` from pending interest count on owner side
    OR pending action on borrower side (e.g.
    `RECIPIENT_SELECTED` awaiting borrower confirmation);
    `scheduled_at_unix_sec` from pickup time when set.
  - `assembleExperience` — `is_owner` from `exp.OwnerId == userId`;
    `is_action_required` from pending RSVPs (if approval flow exists)
    or unread chat messages with the viewer as a participant;
    `scheduled_at_unix_sec` from `exp.scheduled_at_unix_sec`.
  - `assembleRequest` — `is_owner` from `request.RequesterId == userId`;
    `is_action_required` from `REQUEST_STATE_OFFERS_RECEIVED` with
    unaccepted offers; no scheduled time.
  - `assembleSharedGear` — `is_owner == true` always (shared gear
    listings are owner-perspective per inbox.md); `is_action_required`
    follows unread message-from-someone-else heuristic; no
    scheduled time.

      Add helper functions where logic repeats; do not parse status
      strings.

      `is_done` is set in a parallel path (next task) — the
      assemblers above always set `is_done = false`.
- [ ] **Server: completed-items fetch path** for the new
      `include_done: true` request mode. Likely lives next to
      [inbox_feed.go](../../server/services/portfolio/inbox_feed.go) — add
      `completed_inbox_feed.go` (a new file in the same package, per
      [server architecture.md "Short, Focused Files"](../server/architecture.md#3-short-focused-files)).
  - Define the window: terminal events in the **last 7 days** —
    matches the mock copy ("Nothing closed this week"). Configurable
    via a package-level constant `DoneInboxWindowDays = 7` for easy
    tuning.
  - Query terminal-state transfers / experiences / requests that the
    viewer participated in:
    - **Transfers/giveaways**: `Transfer.state IN
      (COMPLETED, CANCELLED)` AND (`OwnerId == userId` OR
      `BorrowerId == userId` OR `RecipientId == userId`) AND
      terminal-event time within window.
    - **Experiences**: `Experience.state IN
      (COMPLETED, CANCELLED)` AND (`OwnerId == userId` OR the user
      has an `RSVP_YES`/`RSVP_MAYBE` record for it) AND terminal-event
      time within window.
    - **Requests**: `Request.state IN
      (FULFILLED, CANCELLED)` AND (`RequesterId == userId` OR the
      user has an offer record against it) AND terminal-event time
      within window.

      All queries use the existing `storage.QueryByFields` / batch
      patterns per [CLAUDE.md "SQL Efficiency"](../../CLAUDE.md#sql-efficiency)
      — never per-item N+1 queries inside a loop. Pin with
      `storage.AssertMaxQueries` in tests.
  - Assemble each terminal item into a `DailyItem` via the existing
    `assembleTransfer`/`assembleExperience`/`assembleRequest`
    helpers, then explicitly set `is_done = true` and
    `is_action_required = false` on the returned item before
    appending to the response. The other flags (`is_owner`,
    `scheduled_at_unix_sec`) come from the assembler as usual.
  - Sort the completed items by terminal-event time descending.
    Append them to the existing `items` list (after the active
    items). The client sorts/groups by `is_done` when rendering the
    Done tab.
  - Bound the appended list with a hardcoded cap — `DoneInboxMaxItems = 50`
    — so a long history can't bloat the response.
- [ ] **Gate the fetch on the `include_done` flag** in
      [inbox_feed.go](../../server/services/portfolio/inbox_feed.go). When
      `req.IncludeDone` is false/unset, skip the completed-items
      path entirely so the existing inbox semantics are unchanged
      for any caller that hasn't opted in.
- [ ] **Server tests** at [server/services/portfolio/assembly_test.go](../../server/services/portfolio/assembly_test.go),
      `inbox_feed_test.go`, and a new `completed_inbox_feed_test.go`:
  - Owner-side transfer with pending interest → `is_owner=true`,
    `is_action_required=true`, `is_done=false`.
  - Borrower-side transfer in active state → `is_owner=false`,
    `is_action_required=false`, `is_done=false`.
  - Future-scheduled experience → `scheduled_at_unix_sec > now`.
  - Past-scheduled experience that's still in the inbox →
    `scheduled_at_unix_sec` in the past (the inbox keeps experiences
    around past start time until completion).
  - **`include_done=false` (default)** — completed items NOT in the
    response (existing semantics preserved).
  - **`include_done=true` with a transfer completed 2 days ago where
    the user was the borrower** → item appears in response with
    `is_done=true`, `is_owner=false`, sorted after active items.
  - **`include_done=true` with a transfer completed 10 days ago** →
    item NOT in response (outside the 7-day window).
  - **`include_done=true` with 60 completed items** → response
    contains only the most-recent 50 (`DoneInboxMaxItems` cap).
  - **N+1 guard** — wrap the `include_done=true` test in
    `storage.AssertMaxQueries(t, ctx, max, fn)` so a per-item
    storage call regression is caught immediately.
  - Make sure `npm run build` and `go test ./server/services/portfolio/...`
    pass.
- [ ] **Server observability** per [server/observability.md](../server/observability.md):
      no new RPC, no new log lines needed — the existing
      `GetPortfolioInboxFeed` request log already covers this. Don't
      add fields just because we're touching the file. If
      `include_done=true` requests look noticeably slower in
      practice, add a DEBUG line with `duration_ms` for the
      completed-items branch — but don't preemptively log.

**Validates:** The wire protocol carries `is_owner`,
`is_action_required`, `scheduled_at_unix_sec`, and `is_done`
correctly. `include_done=true` returns recent terminal items
correctly windowed and capped. Server tests prove the values match
the inbox.md inclusion rules and the Resolved Decisions above. No
user-visible change in the app yet.

### Phase 3: Filter pills on top of the classification flags

**Goal:** Ship the four filter pills (Needs attention / Upcoming / My
stuff / Done) sitting below the search bar. Search and filter compose
— search is scoped to the current pill.

**Tasks:**
- [ ] Add l10n keys (both ARB files):
  - Pill labels with count placeholders (ICU plural for the count):
    - `inboxFilterNeedsAttention` — `"Needs you · {count, plural, =0{0} other{{count}}}"`.
    - `inboxFilterUpcoming` — `"Upcoming · {count}"`.
    - `inboxFilterMyStuff` — `"Your stuff · {count}"`.
    - `inboxFilterDone` — `"Done · {count}"`.
  - Empty-state copy per filter:
    - `inboxFilterNeedsAttentionEmpty` — "When this list is empty"
      (mock copy).
    - `inboxFilterNeedsAttentionEmptyHelp` — "Update your help areas
      — we'll route matching asks near you to your inbox."
    - `inboxFilterNeedsAttentionEmptyCta` — "Set help areas →".
    - `inboxFilterUpcomingEmpty`, `inboxFilterMyStuffEmpty`,
      `inboxFilterDoneEmpty` / `inboxFilterDoneEmptySub`.
  - `a11yInboxFilter{NeedsAttention,Upcoming,MyStuff,Done}` for the
    `Toggle` semantic labels.

      Run `npm run generate`.
- [ ] Define the `InboxFilter` enum in Dart (`needsAttention`,
      `upcoming`, `myStuff`, `done`) at
      `app/lib/presentation/viewmodels/inbox_filter.dart`.
- [ ] Add `inboxFilterProvider` as a `NotifierProvider.autoDispose<InboxFilterNotifier, InboxFilter>`.
      Default state: `InboxFilter.needsAttention`. Method:
      `select(InboxFilter)`.
- [ ] Define `inboxItemMatchesFilter(item, filter)` as a pure function
      in a new file `app/lib/presentation/viewmodels/inbox_filter_logic.dart`.
      Filters are **independent** — items can match multiple filters
      simultaneously (per Resolved Decisions):
  ```
  Needs attention: !is_done && is_action_required == true
  Upcoming:        !is_done && scheduled_at_unix_sec > now
  My stuff:        !is_done && is_owner == true
  Done:            is_done == true
  ```

      Pure-Dart, unit-testable, no Riverpod. The `!is_done` guard on
      the first three is what keeps completed items out of the active
      tabs once the client starts sending `include_done: true` (see
      the request-flag task below).
- [ ] **Flip the client to send `include_done: true`.** In
      [portfolio_repository.dart](../../app/lib/data/repositories/portfolio_repository.dart)'s
      inbox-feed fetch, set `request.includeDone = true`. The
      response now includes terminal items (with `is_done=true`) in
      the same `items` list. **Update the cache key** to incorporate
      the flag — change `portfolio:inbox_feed` to
      `portfolio:inbox_feed:include_done=true` (or similar) so the
      pre-flip cache entry doesn't leak active-only data into the
      post-flip flow. Per [caching.md](../client/caching.md), cache
      keys follow the `'namespace:identifier'` convention.
- [ ] Extend `inboxVisibleItemsProvider` to apply both filter and
      search:
  1. Read `inboxFlatItemsProvider` (already community-filtered).
  2. Filter by `inboxItemMatchesFilter(item, currentFilter)`.
  3. Apply the search-query filter (Phase 1) on the result.

      Counts for the pill labels come from a separate
      `inboxFilterCountsProvider` that returns
      `Map<InboxFilter, int>` (computed by applying
      `inboxItemMatchesFilter` against every filter in turn —
      pre-search). Pills show pre-search counts so the user can see
      that switching pills will land somewhere non-empty.
- [ ] Build the filter pill row widget at
      `app/lib/presentation/widgets/portfolio/inbox_filter_row.dart`:
  - Horizontal scrollable row (4 pills today; allow more in case
    future segments are added).
  - Each pill is a [`Toggle`](../../app/lib/presentation/widgets/accessibility/)
    (not a `Tappable`) per [accessibility.md "When to reach for which
    primitive"](../client/accessibility.md#when-to-reach-for-which-primitive)
    — toggle is the right primitive for selectable single-state
    chips. Set `selected:` on the active pill; do **not** embed
    "selected" in the semantic label.
  - Label format: `context.l10n.inboxFilterNeedsAttention(count)`,
    etc. (ICU plural).
  - Active pill: dark fill (`AppColors.ink`-equivalent semantic
    token) with cream text. Inactive: muted stone fill with secondary
    text. Use `AppColors.*(context)` helpers throughout.
- [ ] Update [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart):
  - Insert `InboxFilterRow` immediately below the
    `FloatingHeader` (mock places it as a separate row at the top
    of the scroll content, scrolling away with the list — or pinned
    above the list; pick what matches the mock at implementation
    time).
  - Empty-state branch: when `inboxVisibleItemsProvider` returns an
    empty list, render the per-filter empty-state copy + (for
    `needsAttention`) the "Set help areas →" CTA.
  - Reset the search input when the user changes filter? No — search
    is scoped to filter (Resolved Decisions). The search query
    persists across filter changes; it just operates against
    different base sets.
- [ ] **"Done" tab** renders real completed-items data from Phase 2:
  - `inboxItemMatchesFilter(item, InboxFilter.done)` returns
    `item.isDone == true`.
  - The Done tab uses the same `InboxRow` widget as the other tabs
    (no visual divergence). Items are sorted by terminal-event time
    descending — the server already sorts them this way, so the
    client just renders in the order received.
  - **Tap behavior on a Done row**: open the underlying content
    screen in read-only/historical mode (the screens already handle
    terminal state). Confirm by inspecting `GearScreen`,
    `ExperienceScreen`, and `RequestScreen` — if any of them
    misbehave for terminal-state items (e.g. show "Borrow Now"
    button for a completed transfer), surface that as a spot-fix in
    this issue.
  - Pill label shows `Done · {count}` from the same
    `inboxFilterCountsProvider`.
  - **Empty-state copy** when no completed items in the 7-day window:
    "Nothing closed this week. Returns, RSVPs, and replies will land
    here once they're wrapped up." (already in the l10n key list
    above as `inboxFilterDoneEmpty` / `inboxFilterDoneEmptySub`).
- [ ] **Tests:**
  - Pure-Dart unit tests for `inboxItemMatchesFilter`: one test per
    `(item profile, filter) → expected` combination. Cover the
    **multi-tab membership** explicitly — an item with
    `is_owner == true` AND `is_action_required == true` (and not
    done) must match **both** `needsAttention` and `myStuff`.
    Likewise an owner-hosted future-scheduled experience matches
    `needsAttention` (if RSVPs pending), `upcoming`, and `myStuff`.
  - ViewModel test for `inboxFilterProvider`: default is
    `needsAttention`; `select(upcoming)` updates state.
  - ViewModel test for `inboxFilterCountsProvider`: with seeded item
    set, each count matches the expected membership. Verify counts
    can sum to more than `items.length` (multi-tab membership).
  - Widget test for `InboxFilterRow`: tapping a pill calls
    `select`; active pill has `selected: true` semantics.
  - End-to-end widget test (one): seed a feed response with a
    mix of active items + 2 done items; verify the Done tab renders
    the 2 done items; verify switching to My stuff hides them.

**Validates:** Filter pills correctly classify the inbox; items
appear in multiple tabs when their flags match; switching pills
updates the list and the counts; the Done tab shows real
completed-items data from Phase 2 (or the empty-state when there
are none in the 7-day window); search composes with filter; all
accessibility and i18n lints pass.

## Related Issues

- #1895 — Bottom + floating nav restructure. **Hard dependency.**
  Inbox's header gets a free leading slot only after #1895 removes
  the hamburger; Phase 1 of this issue assumes that.
- #1896 — Feed "Search nearby" search affordance. **Pattern overlap.**
  Inbox follows the same `FloatingHeader.content:` pill pattern. After
  both land, consider extracting a shared `HeaderSearchPill` widget
  — but only if both shipped widgets actually overlap (don't pre-
  abstract).
- #1898 — Workshop redesign. No direct dependency. Same nav-restructure
  cohort.
- (TBD post-#1895/#1896/#1897/#1898) — Dead-code cleanup PR. After
  this issue lands, the screen still reads
  `selectedCommunityProvider.enabledCommunityIds` via
  `inboxFlatItemsProvider`; the cleanup PR collapses that read once
  the provider's full neutering ships.
