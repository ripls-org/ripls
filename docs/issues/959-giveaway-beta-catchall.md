# Issue #959: Giveaway catch-all issue for closed beta

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/959
**Filed:** 2026-03-31 | **Last activity:** 2026-03-31
**Labels:** P1 (High) | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The giveaway flow has multiple UX gaps and bugs that collectively degrade the
closed beta experience. The issue covers 11 distinct items spanning the managing
menu, modal simplification, notifications/deep-linking, real-time state updates,
pickup coordination, completion confirmation, and cosmetic issues. These need to
be addressed together as a P1 before wider launch.

Key themes:
1. **State-awareness**: The managing menu and modals don't adapt well to transfer
   state, leaving users unsure what to do next.
2. **Missing pickup coordination**: Giveaways lack the scheduling flow loans
   have (propose time, notify, confirm).
3. **Real-time freshness**: Recipients must pull-refresh to see state changes;
   system chat messages should trigger cache invalidation.
4. **Notification gaps**: No deep-link to the gear/transfer modal; no
   notification when pickup time is proposed.
5. **Completion UX**: Confusing icons and missing confirmation modal.

## Context from Comments

No comments on this issue. Given the breadth of the items listed, some may need
further clarification during implementation (noted in Open Questions).

## Sub-issues Breakdown

For reference, the 11 items from the issue mapped to implementation areas:

| # | Item | Area |
|---|------|------|
| 1 | Managing menu should be responsive to state (gray out select recipient) | Client: gear_content_view |
| 2 | Managing should show next step when clear (e.g. "waiting for interest") | Client: gear_content_view |
| 3 | Simplify modal — both parties see interest list, owner can choose, remove clutter | Client: giveaway modals |
| 4 | Recipient notification doesn't deep link to item correctly | Client: FCM routing |
| 5 | Recipient must pull-refresh to go from interested to selected — use system chat messages as trigger | Client: cache invalidation |
| 6 | Add pickup coordination like loans (both parties manage) | Server + Client |
| 7 | Pickup time proposal needs notification | Server: notifications |
| 8 | Confirm received has confusing check icon similar to completed steps | Client: icons |
| 9 | Recipient can't confirm picked up | Server + Client (may already work) |
| 10 | Gear title is lowercase in post-giveaway description | Client: string formatting |
| 11 | When either participant confirms pickup, show completion modal | Client: completion flow |

## Codebase Analysis

### Current Architecture

**Server**: Unified transfer service handles both loans and giveaways via a
type-aware state machine. Giveaway states: INTEREST_EXPRESSED →
RECIPIENT_SELECTED → COMPLETED (no ACTIVE state, unlike loans). Pickup time
fields (`estimated_pickup_unix_sec`, `actual_pickup_unix_sec`) exist on both
transfer types but giveaways have no coordination flow for them.

**Client**: `TransferModalRouter` dispatches to `GiveawayGiverModal` or
`GiveawayReceiverModal`. The managing menu in `gear_content_view.dart` builds
items dynamically per phase but doesn't gray out or show guidance text.

### Key Findings

1. **Item 9 (recipient can't confirm)**: Server-side `isValidTransferTransition`
   already allows either party to complete a giveaway. The receiver modal has a
   `markPickedUp()` method that calls `completeTransfer`. This likely works but
   may have a UI bug hiding the button in certain states.

2. **Item 4 (deep linking)**: FCM routing only supports `conversation_id` —
   there's no gear or transfer modal deep link path. Notifications route to the
   conversation screen, not the gear screen.

3. **Item 5 (pull-refresh)**: No mechanism for system chat messages to trigger
   transfer state refresh on the client. The cache layers are separate.

4. **Item 6 (pickup coordination)**: Loans use `UpdateTransfer` to set
   `estimated_pickup_unix_sec` and `StartLoan` as the coordination handshake.
   Giveaways skip the ACTIVE state entirely. The `ArrangePickupModal` exists for
   loans but isn't wired for giveaways.

5. **Item 10 (lowercase title)**: Explicit `.toLowerCase()` calls in
   `giveaway_receiver_modal.dart:288` and `giveaway_giver_modal.dart:313,374`.

6. **Item 8 (confusing icon)**: `checklist_status_icons.dart` uses
   `Icons.check_circle_outline` for the incomplete "confirm received" step —
   same icon as other incomplete checklist items.

## Potential Solutions

### Option A: Incremental fixes (per-item patches)

- **Approach:** Fix each of the 11 items independently in separate PRs. No
  architectural changes; work within the existing state machine and modal
  structure.
- **Pros:** Low risk per change; easy to review and ship incrementally.
- **Cons:** Doesn't address the underlying structural gap (no ACTIVE state for
  giveaways, no pickup coordination flow). Items 6 and 7 are hard to solve
  cleanly without an ACTIVE state.
- **Effort:** Medium (11 small changes, but item 6 is awkward without arch
  changes).

### Option B: Add giveaway pickup coordination phase, then fix UX

- **Approach:** Introduce an optional ACTIVE state for giveaways to enable
  pickup coordination (matching loans). Then fix UX issues on top of a cleaner
  state model. The ACTIVE state represents "pickup scheduled / in progress"
  before final confirmation.
- **Pros:** Clean architecture; reuses loan pickup patterns; both parties can
  propose/confirm pickup; enables proper notifications.
- **Cons:** Larger scope; requires server state machine changes, proto updates,
  and client flow rework. Risk of scope creep.
- **Effort:** Large.

### Option C: Lightweight pickup coordination without ACTIVE state (recommended)

- **Approach:** Use the existing `UpdateTransfer` RPC and system chat messages
  for pickup coordination without adding an ACTIVE state. When either party
  sets `estimated_pickup_unix_sec`, send a notification to the other party.
  Wire the `ArrangePickupModal` for giveaways. Fix all UX items within the
  existing 3-state model (INTEREST_EXPRESSED → RECIPIENT_SELECTED → COMPLETED).
- **Pros:** Minimal server changes; reuses existing pickup time fields and
  `ArrangePickupModal`; addresses all 11 items without architectural upheaval.
- **Cons:** No formal "pickup confirmed" state distinction — coordination
  happens within RECIPIENT_SELECTED via chat + pickup time fields.
- **Effort:** Medium.

## Recommendation

**Option C** — lightweight pickup coordination within the existing state model.
The giveaway flow doesn't need an ACTIVE state because there's no "return"
phase. The RECIPIENT_SELECTED → COMPLETED transition is sufficient when
augmented with pickup time coordination and proper notifications.

This keeps the server simple while addressing all user-facing issues. If beta
feedback reveals that users need a more formal coordination handshake, we can
add an ACTIVE state later (Option B) as a follow-up.

## Open Questions

- **Q1: What should the managing menu show before anyone has expressed interest?**
  Recommendation: Show "Waiting for interest" as disabled text (not a button).
  The "Select Recipient" action should be grayed out until at least one person
  has expressed interest.

- **Q2: Should the simplified modal show all interested users to both parties?**
  The issue says "both owner/recipient see a list of who has expressed interest."
  Recommendation: Yes — the gear conversation already includes all interested
  parties. Showing the interest list to everyone is transparent and encourages
  engagement. The "select" action is only available to the owner.

- **Q3: How should deep linking work for transfer notifications?**
  Recommendation: Add a `gear_id` field to the notification payload. When
  present, route to the gear detail screen and auto-open the transfer modal.
  This is more useful than routing to the conversation.

- **Q4: What should happen after completion?** *Resolved*: No completion modal
  needed. Both giver and receiver can mark the giveaway complete. After
  completion, each is taken to correctly updated detail screens — recipient sees
  impact metrics, giver sees a completed detail card.

- **Q5: What "other stuff" should be removed from the modal?** *Resolved*:
  Strip the modal down to a pure recipient-selection UI. Remove everything not
  related to listing/selecting recipients: item name/title display, workflow
  stage explanations, WaitingOnUserCard, OtherPartySees, WhatTheySeeCard, step
  headers/progress indicators. The modal should no longer be a multi-stage
  explanatory workflow — just a focused recipient picker.

## Audit Notes

Changes made during `/ripls-audit` review:

- **[i18n]** Phases 1, 2, 5: Added ARB file requirements for all new
  user-visible strings (both `app_en.arb` and `app_es.arb`). Per i18n
  guidelines, viewmodels must expose typed state (enums), not resolved strings;
  screens resolve to `context.l10n.*` in `build()`.
- **[caching]** Phase 4: Specified event-driven invalidation pattern (following
  `dailyCacheInvalidationProvider` / `searchCacheInvalidationProvider`
  precedent) instead of vague "cache invalidation triggered by system chat
  messages." Invalidation must happen at the repository layer.
- **[navigation]** Phase 4: FCM deep link to gear screen must use
  `NavigationHelpers.pushScreen()`, not raw `Navigator.push` or
  `context.push()`. Gear screen must use `SwipeToCloseMixin`.
- **[server]** Phase 3: Specified that pickup notifications use existing
  `RecordCommunityEventAndNotify` + `shouldNotify()` pattern, not a new
  notification path.
- **[testing]** Phases 1, 3: Clarified test patterns — client ViewModel tests
  use `ProviderContainer` with mock repositories; server tests use table-driven
  Go tests.
- **[architecture]** Phase 4: Clarified that cache invalidation is owned by
  repositories, not ViewModels. System message detection triggers repository
  invalidation via an event-driven provider.

## Implementation Plan

### Phase 1: Managing menu and state awareness

**Goal:** Make the gear detail managing menu responsive to giveaway state.

**Tasks:**

- [ ] Gray out "Select Recipient" when no users have expressed interest
- [ ] Show contextual next-step text per state — define an enum in the
  ViewModel (e.g. `GiveawayMenuState.waitingForInterest`,
  `.selectRecipient`, `.coordinatePickup`, `.complete`); resolve to
  localized display text in the screen's `build()` via `context.l10n.*`
- [ ] Add ARB entries for all new menu state strings in both `app_en.arb` and
  `app_es.arb`; run `npm run generate` to regenerate `AppLocalizations`
- [ ] Add ViewModel unit tests using `ProviderContainer` with mock
  `TransferRepository` — verify menu state enum per transfer context

**Validates:** Owner always knows what the next action is without opening the
modal.

### Phase 2: Rebuild giveaway modal as focused recipient picker

**Goal:** Replace the multi-stage explanatory workflow modal with a focused
recipient-selection UI.

**Tasks:**

- [ ] Strip giveaway giver modal down to: list of interested users, tap to
  select, confirm selection — nothing else
- [ ] Remove all workflow-stage elements: step headers, progress indicators,
  item name/title display, WaitingOnUserCard, OtherPartySees, WhatTheySeeCard,
  stage explanation text
- [ ] Giveaway receiver modal: show interest list (read-only) and own status
  only — same minimal approach
- [ ] Fix the confusing `check_circle_outline` icon for "confirm received" —
  use a distinct icon (e.g. `Icons.inventory_2_outlined` or
  `Icons.card_giftcard`) to differentiate from progress step checkmarks
- [ ] Add/update ARB entries for any new or changed user-visible strings in
  both `app_en.arb` and `app_es.arb`; remove ARB entries for deleted UI
  elements
- [ ] Use `AppColors` theme-aware helpers for all modal colors (backgrounds,
  borders, text) — no hardcoded `Colors.*` values

**Validates:** Modal is a simple, focused recipient picker with no legacy
multi-stage chrome.

### Phase 3: Pickup coordination for giveaways

**Goal:** Enable pickup scheduling for giveaways like loans have.

**Tasks:**

- [ ] Wire `ArrangePickupModal` for giveaway transfers (currently loan-only)
- [ ] Allow both giver and receiver to propose/edit pickup time via
  `UpdateTransfer` in RECIPIENT_SELECTED state
- [ ] Add server notification when `estimated_pickup_unix_sec` is set/changed
  on a giveaway — use the existing `RecordCommunityEventAndNotify` pattern
  with a new or extended community event type; add the event to
  `shouldNotify()` and `getNotificationRecipients()` in
  `server/services/community/notifications.go`
- [ ] Show pickup details in gear detail screen for giveaways
- [ ] Server tests: table-driven Go tests for pickup time update on giveaway
  transfers and notification dispatch
- [ ] Client tests: ViewModel unit tests using `ProviderContainer` with mock
  `TransferRepository` for pickup coordination state

**Validates:** Both parties can schedule and see pickup details; the other
party is notified of proposals.

### Phase 4: Notifications and real-time updates

**Goal:** Fix notification deep linking and cache freshness.

**Tasks:**

- [ ] Add `gear_id` to notification payload for transfer events (server-side,
  in `RecordCommunityEventAndNotify` or the FCM payload builder)
- [ ] Update FCM routing in `fcm_service.dart` to detect `gear_id` in payload;
  navigate to gear screen using `NavigationHelpers.pushScreen()` (not raw
  `Navigator.push` or `context.push()`) — gear screen already uses
  `SwipeToCloseMixin`
- [ ] Implement event-driven transfer cache invalidation: create a
  `transferCacheInvalidationProvider` (following the
  `dailyCacheInvalidationProvider` / `searchCacheInvalidationProvider`
  pattern). `TransferRepository` listens and invalidates on signal.
  Trigger the signal from the conversation/chat layer when system messages
  indicating transfer state changes are received. Cache invalidation must
  stay in the repository layer — ViewModels must not manipulate caches
  directly.
- [ ] Ensure recipient transitions from "interested" to "selected" without
  manual pull-refresh (the invalidation provider above should handle this)
- [ ] Test deep link from notification → gear screen flow
- [ ] ViewModel disposal safety: if new async listeners are added, include
  `ref.mounted` checks after async gaps per architecture guidelines

**Validates:** Recipients are notified and routed to the right screen; state
updates appear in real time.

### Phase 5: Completion and cosmetic fixes

**Goal:** Ensure both parties can complete and see correct post-completion
screens.

**Tasks:**

- [ ] Verify recipient can mark giveaway as complete (item 9) — server allows
  it; fix any UI condition hiding the button
- [ ] After either party marks complete, navigate to correctly updated detail
  screens: recipient sees impact metrics, giver sees completed detail card
- [ ] Fix gear title casing: remove `.toLowerCase()` calls in
  `giveaway_giver_modal.dart:313,374` and
  `giveaway_receiver_modal.dart:288`. These inline sentences with gear names
  should use localized ARB strings with `{gearName}` placeholders (ICU
  message format) — add entries in both `app_en.arb` and `app_es.arb`
- [ ] Test end-to-end giveaway flow: express interest → select → coordinate
  pickup → confirm → updated detail screens

**Validates:** Complete, polished giveaway flow ready for closed beta.

## Related Issues
- None referenced in the issue or comments.
