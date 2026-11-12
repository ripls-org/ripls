# Content View Post-Completion Fixes (content5)

## Problem Statement

The content4 tabbed redesign (Phases 0-10) is complete, but several issues have surfaced during real-device testing and workflow review. These fall into three categories: **device-specific rendering bugs**, **chat input regressions**, and **workflow/menu UX gaps**.

---

## Problems

### P1: Feed Nav Bars Reappear After Scrolling to a New Item (Device Only)

**Symptom:** On physical devices (not simulator), scrolling the feed correctly hides the top and bottom floating nav bars. However, when the PageView settles on a new item, the nav bars reappear after ~1 second. They should stay hidden until the user taps or swipes in the reverse direction.

**Root cause hypothesis:** When a PageView snaps to a new page, Flutter generates a small scroll settle animation. This settle animation produces `ScrollUpdateNotification` events with negative `scrollDelta` (bouncing back toward the snap position). On physical devices, the inertia and deceleration curve differ from the simulator, producing delta values that exceed the `-10` threshold in `feed_screen.dart:215-237`, triggering `showNav()`. The simulator's lighter scroll physics rarely produce large enough settle deltas to cross the threshold.

**Evidence:** The `ScrollUpdateNotification` listener uses a fixed 10-pixel threshold for both directions (`> 10` to hide, `< -10` to show). A page snap settle on a physical device can easily produce transient negative deltas of 15-30px.

### P2: Chat Tab Full-Screen Flicker on New Line Entry (Device Only)

**Symptom:** In the inline chat (Chat tab), entering a new line on a physical device keyboard causes a full-screen flicker that appears to reload everything. It should just append content and push existing messages up.

**Root cause hypothesis:** The `InlineConversationView` is embedded inside an `AnimatedSize` content area. When the TextField grows by a line, the content area's height changes, triggering `AnimatedSize` to animate. On a physical device, the keyboard `viewInsets` change + `AnimatedSize` height change + message list scroll adjustment all fire within the same frame, causing a layout cascade. The original `ConversationScreen` avoids this by using `resizeToAvoidBottomInset: false` and manually positioning elements with `viewInsets.bottom` — the inline view does not use this pattern.

**Key difference from ConversationScreen:** The `conversation_screen.dart` (line 391) sets `resizeToAvoidBottomInset: false` and manually drives layout via `MediaQuery.of(context).viewInsets.bottom` (lines 283, 401, 441). The `InlineConversationView` relies on Flutter's default `resizeToAvoidBottomInset: true` behavior inherited from the parent Scaffold, plus an `AnimatedSize` wrapper, creating a double-animation conflict.

### P3: Virtual Keyboard Closes on Enter in Chat Tab

**Symptom:** Pressing Enter on the virtual keyboard in the chat input sends the message but closes the keyboard. The keyboard should stay open for rapid message entry.

**Root cause analysis:** The expanded mode input in `inline_conversation_view.dart` (line 382) correctly uses `onEditingComplete: () {}` to suppress unfocus. However, `textInputAction: TextInputAction.send` combined with `onSubmitted` on some Android keyboards and iOS versions triggers a platform-level dismissal that the empty `onEditingComplete` cannot override. The original `conversation_screen.dart` passes the focus node to `MessageInputBar`, which also uses `TextInputAction.send` + `onSubmitted` without an `onEditingComplete` override — yet the keyboard stays open there because `MessageInputBar` re-requests focus after send.

**Key difference:** In `conversation_screen.dart`, the `_handleSendMessage` method (after clearing and sending) does not explicitly unfocus, and the `_messageFocusNode` retains focus because the TextField is the only focusable element in the scaffold. In the inline view, the parent content view's tab bar and other focusable elements can steal focus during the send-clear-rebuild cycle.

### P4: Experience "Find Best Time" Menu Item Doesn't Update After Poll Creation

**Symptom:** In `ExperienceContentView`, the owner taps "Find Best Time" and creates a poll. Once the poll is live, the same menu item still says "Find Best Time" instead of switching to something like "See Time Results" that shows results and allows ending the poll and setting the time.

**Current state:** The menu item at `experience_content_view.dart:543-552` is static — it always shows "Find Best Time" regardless of whether a poll already exists. The `time_voting_card.dart` already renders poll results with voter avatars, but there is no modal for the owner to view aggregate results and confirm a time.

### P5: Loan Owner Menu Doesn't Identify Which Requestor's Workflow Is Active

**Symptom:** For loans, an owner may have multiple borrow requests simultaneously. The action dropdown menu shows workflow steps (Arrange Pickup, Mark Picked Up, etc.) but doesn't indicate which requestor they apply to. If there are multiple requestors, the owner needs to see who and switch between them.

**Current state:** The menu builders in `gear_content_view.dart:1035-1208` build menu items for the current `transferContext` but don't show the requestor's name or provide a way to switch between multiple active requests.

### P6: Loan "Arrange Pickup" Should Create a Time Poll

**Symptom:** The "Arrange Pickup" menu item for loans currently does nothing meaningful (or navigates to a placeholder). It should create a time poll between the owner and the selected borrower, using the same poll mechanism as experiences.

**Current state:** The "Arrange Pickup" item exists in the menu but the handler is minimal. The experience time poll infrastructure (`FindBestTimeModal`, `time_voting_card.dart`) already exists and can be reused.

### P7: Giveaway Post-Selection Menu Lacks Recipient Context

**Symptom:** After a giveaway recipient is selected, the owner's menu should show the selected recipient's name at the top (like the proposed loan menu change) and display the same pickup workflow steps as loans.

**Current state:** The giveaway owner menu (`_buildGiveawayOwnerMenuItems`, lines 1037-1067) has Select Recipient, Mark as Picked Up, and Cancel, but doesn't show who was selected or mirror the loan pickup workflow.

### P8: No Completed Phase Indicators in Menu Items

**Symptom:** Workflow menu items don't show completion status. Once something has been picked up, "Arrange Pickup" and "Mark Picked Up" should appear with green checkmarks to show progress through the workflow phases.

**Current state:** Menu items via `ActionDropdownItem` have `icon`, `label`, `subtitle`, `isDestructive`, and `onTap` — but no `isCompleted` flag or visual treatment for completed steps.

### P9: Overflow Menu Contains Actions That Should Be Elsewhere

**Symptom:** The overflow menu (···) contains Edit, RSVP, Communities, Delete, etc. These are action items, not informational. The overflow button should become an info (i) button that shows informational details only: owner, creation time, last updated, and (where applicable) background image attribution. Action items should be moved to the workflow action button menu or removed.

**Current state:** `content_overflow_menu.dart` renders a configurable bottom sheet with Edit, Share, Delete, Report, and custom items. The audio play button is also in the overflow menu area.

### P10: Owner Row Visibility When Opening from Discover Screen vs Chat Tab

**Symptom:** When opening content views from the discover screen, the back button row with the owner is shown at the top (correct). When in the chat tab, the owner details should be hidden (only back button visible), consistent with the feed screen behavior when a user is on the chat tab.

**Current state:** The content views already hide the owner row when `activeTab == 2` (Chat tab). However, when opened from the discover screen (not in feed context, `showFeedHeader=false`), the back button is part of the `ContentOwnerRow` — hiding the entire row also hides the back button. Need to separate back-button-only from full owner row.

---

## Potential Solutions

### S1: Feed Nav Bar Settle Fix

**Option A — Ignore settle-phase deltas:** After a page change, set a `_isSettling` flag for ~500ms during which `showNav()` calls are suppressed. Reset the flag after the settle completes. Simple but timing-dependent.

**Option B — Use scroll direction tracking with hysteresis:** Instead of raw `scrollDelta`, track cumulative scroll distance in one direction. Only trigger show/hide after accumulating >30px in a consistent direction. Resets on direction change. More robust, handles both settle and finger wobble.

**Option C — Listen for `ScrollEndNotification` instead of `ScrollUpdateNotification` for show:** Only call `showNav()` when a `UserScrollNotification` with `ScrollDirection.reverse` is detected (actual user-initiated reverse scroll), not during settle. Use `ScrollUpdateNotification` only for hide. This distinguishes user-initiated scrolls from physics-driven settle.

**Recommendation:** **Option C** — it directly addresses the root cause (settle physics triggering show) without arbitrary timers. `UserScrollNotification` fires only for deliberate user gestures, while settle-phase events only produce `ScrollUpdateNotification`. This is the most precise fix.

### S2: Chat Flicker Fix

**Option A — Adopt ConversationScreen's manual viewInsets pattern:** Set `resizeToAvoidBottomInset: false` on the nearest Scaffold and manually position the chat content using `MediaQuery.viewInsets.bottom`. This matches the proven pattern in `conversation_screen.dart`.

**Option B — Remove AnimatedSize for Chat tab:** When the Chat tab is active, bypass the `AnimatedSize` wrapper and use a fixed-height container driven by the keyboard insets. Other tabs continue using `AnimatedSize`.

**Option C — Use `AnimatedSize` with `clipBehavior: Clip.none` and a layout builder:** Prevent the size animation from triggering a relayout of the message list by clipping only the container, not the content.

**Recommendation:** **Option A** — the ConversationScreen's pattern is battle-tested and eliminates the double-animation conflict. The Chat tab should detect when it's active and switch the content area from `AnimatedSize` to a `viewInsets`-driven `Positioned` layout. This also fixes P3 (keyboard closing) as a side effect, since manual viewInsets management doesn't trigger the platform-level dismiss.

### S3: Keyboard Stays Open Fix

**Option A — Re-request focus after send:** After `_handleSendMessage()` clears the text and sends, explicitly call `_inputFocusNode.requestFocus()`. This is the simplest fix.

**Option B — Use `TextInputAction.newline` with a manual send gesture:** Change to newline action and detect send via a separate button only. This changes UX (Enter = newline, not send).

**Recommendation:** **Option A** — after `_handleSendMessage()`, call `_inputFocusNode.requestFocus()`. This matches the effective behavior of `ConversationScreen` where the focus node retains focus. Combined with S2 (manual viewInsets), this should eliminate the keyboard bounce.

### S4: Time Poll Menu State

**Approach:** Check if `experienceDetails.timeProposals` is non-empty. If so, show "See Time Results" instead of "Find Best Time". Tapping opens the same `FindBestTimeModal` but in a results mode: showing existing proposals with vote counts and voter avatars. The owner can select a proposal and tap "Confirm Time" to auto-update the experience's date/time and send a system message — but they are not forced to accept the popular choice (they can ignore the results or pick any proposal). An "End Poll" option closes the poll without confirming a time.

### S5: Loan Requestor Header

**Approach:** Add a requestor name section at the top of the loan workflow dropdown menu. If there are multiple active transfer requests, show a dropdown/selector to switch between them. This switcher controls which transfer context the menu actions (Arrange Pickup, Mark Picked Up, etc.) apply to — it does not affect the chat, which is a single shared per-gear conversation. The `transferContext` in the ViewModel would need to support a list of active transfers, with a `selectedTransferIndex` to track which is active for the menu.

### S6: Loan Arrange Pickup Poll

**Approach:** Reuse `FindBestTimeModal` for the loan pickup time poll. The poll is created in the existing per-gear conversation (all parties can see it, since conversations are per-gear, not per-transfer). The modal is generalized to accept the gear's `conversationId`. The same `time_voting_card.dart` renders inline in the gear conversation just like for experiences.

### S7: Giveaway Post-Selection Menu

**Approach:** After recipient selection, the giveaway owner menu should mirror the loan menu: show the selected recipient's name at the top, then Arrange Pickup, Mark Picked Up, Cancel. Reuse the same menu builder as loans with giveaway-specific labels.

### S8: Completed Phase Indicators

**Approach:** Add an `isCompleted` field to `ActionDropdownItem`. When true, render a green checkmark icon on the right side and reduce the item's opacity or use a muted text style. The workflow state (from ViewModel) determines which items are completed based on the current phase.

### S9: Overflow to Info Button + Close Modal in Manage Menu

**Approach:** Two changes:

1. **Info button:** Replace the ··· overflow icon with an (i) info icon. The modal shows read-only information: owner name/avatar, created date, last updated date, and (where applicable) background image attribution. Move the volume/speaker button (video backgrounds only, logic already exists in content views) to the left of the info button. Remove Edit, RSVP, Communities, Delete, Share from this modal — they belong elsewhere.

2. **Close action in manage menu:** Add "Change Details" and "Close [Item/Event/Request]" to the owner's workflow action dropdown (the `MorphingActionButton` menu). "Close" opens a modal following the existing `CancelEventModal` pattern (`cancel_event_modal.dart`): three options — unshare from community, mark as cancelled, or delete permanently. Build a generalized `CloseItemModal` that works for all three content types, using `CancelEventModal` as the template.

### S10: Back Button Without Owner Row on Chat Tab

**Approach:** When `activeTab == 2` (Chat tab) and not in feed context, show a minimal back button only (no avatar, no name, no subtitle, no overflow). This can be a separate `ContentBackButton` widget or a mode flag on `ContentOwnerRow` that renders only the back arrow.

---

## Design Decisions (Resolved)

1. **P1 (Nav bars):** The issue is specifically when NOT on the Chat tab. When the Chat tab is active, `lockNav()` already suppresses `showNav()` correctly. The fix targets the settle-phase false triggers on non-Chat tabs.

2. **P4 (Time poll results):** Auto-update the experience's date/time fields when the owner confirms a time from poll results, but the owner must explicitly submit (tap "Confirm Time") — they can view results and choose to ignore them. The confirmed time writes to the experience's `event_time` field via the ViewModel.

3. **P5 (Multiple requestors):** The requestor switcher applies only to the workflow menu items (which transfer context the Arrange Pickup / Mark Picked Up / etc. actions target). The conversation itself is per-gear and shared across all parties — there is no per-transfer conversation. The chat tab always shows the single gear conversation.

4. **P6 (Loan pickup poll):** The poll is created in the existing gear conversation (the same shared conversation all parties see). Conversations are per-gear, not per-transfer. The time voting card renders inline in the gear conversation just like for experiences.

5. **P9 (Close/Cancel/Delete):** The owner's workflow manage menu should include "Change Details" and a "Close [Item/Event/Request]" option. "Close" opens a modal offering: unshare from community, mark as cancelled, or delete — following the existing `CancelEventModal` pattern in `cancel_event_modal.dart`. This replaces the old Edit/Delete items in the overflow menu.

6. **P9 (Audio):** The volume/speaker button is only for content with video backgrounds. The logic already exists in `ExperienceContentView` (and the other content views); only the button's position needs to move — from its current location to the left of the info (i) button in the owner row.

---

## Phased Implementation Plan

### Phase 1: Feed Nav Bar Settle Fix (P1)

**Goal:** Prevent nav bars from reappearing during PageView settle animation on physical devices.

**Modified files:**
- `app/lib/presentation/screens/feed/feed_screen.dart`:
  - Replace the `ScrollUpdateNotification` listener for showing nav
  - Use `UserScrollNotification` with `ScrollDirection.reverse` to trigger `showNav()`
  - Keep `ScrollUpdateNotification` with `scrollDelta > 10` for `hideNav()` (hiding during forward scroll is correct)
  - The `UserScrollNotification` only fires for user-initiated gestures, not physics settle, eliminating the false trigger

**Tests:** Verify nav stays hidden after page snap; verify nav shows on deliberate reverse swipe; verify tap-to-toggle still works.

---

### Phase 2: Chat Flicker and Keyboard Fixes (P2, P3)

**Goal:** Eliminate full-screen flicker when typing new lines in chat and keep the keyboard open after sending a message.

**Modified files:**
- `app/lib/presentation/widgets/content/inline_conversation_view.dart`:
  - After `_handleSendMessage()` clears text and sends, call `_inputFocusNode.requestFocus()` to retain keyboard
  - Review the expanded-mode layout: ensure the message list uses a fixed-height container when the keyboard is open, not an `AnimatedSize`-driven height
  - Add `keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.manual` to the message list's `ListView` to prevent scroll-to-dismiss behavior that conflicts with typing

- `app/lib/presentation/screens/gear/gear_content_view.dart` (and experience/request equivalents):
  - When `activeTab == 2` (Chat tab), bypass the `AnimatedSize` wrapper for the content pane
  - Use a fixed pixel height for the Chat pane driven by `MediaQuery.of(context).size.height` minus the tab bar position, adjusted by `MediaQuery.of(context).viewInsets.bottom`
  - This matches the `ConversationScreen` pattern of manual viewInsets management

**Key principle:** The Chat tab content area height should be driven directly by `viewInsets.bottom` changes (instant layout), not by `AnimatedSize` (animated layout). The animation creates a visual "bounce" as the size animates while the keyboard is already at its final position.

**Tests:** Verify no flicker when adding newlines; verify keyboard stays open after send; verify message list scrolls smoothly as new messages appear.

---

### Phase 3: Experience Time Poll Menu State (P4)

**Goal:** Switch "Find Best Time" to "See Time Results" once a poll exists, and allow the owner to confirm a time and end the poll.

**Modified files:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`:
  - In the owner menu builder, check `state.experienceDetails?.timeProposals.isNotEmpty == true`
  - If poll exists: show "See Time Results" with a results icon instead of "Find Best Time"
  - Tapping "See Time Results" opens `FindBestTimeModal` in results mode

- `app/lib/presentation/screens/experience/find_best_time_modal.dart`:
  - Add a `resultsMode` parameter (or detect from existing proposals)
  - In results mode: show existing proposals with vote counts and voter avatars
  - Add "Confirm Time" button that writes the selected time to the experience and sends a system message
  - Add "End Poll" option that closes the poll without confirming a time
  - Reuse the existing `time_voting_card.dart` tile rendering for consistency

- `app/lib/presentation/viewmodels/experience_view_model.dart`:
  - Add `confirmPollTime(proposalId)` method that calls the server to set the experience time and end the poll

**Tests:** Verify menu item switches label based on poll state; verify results mode shows vote counts; verify "Confirm Time" updates the experience.

---

### Phase 4: Loan Requestor Switcher (P5) and Pickup Poll (P6)

**Goal:** Show which requestor's workflow is active in the loan menu, allow switching between multiple requestors, and create time polls for pickup arrangement.

**Key architecture note:** Conversations are per-gear, not per-transfer. All interested parties share one conversation on the gear item. The requestor switcher only affects which transfer context the workflow menu actions target — it does not change the conversation.

**Modified files:**
- `app/lib/presentation/viewmodels/gear_view_model.dart`:
  - Expose `allActiveTransfers` list (not just the single `transferContext`)
  - Add `selectedTransferIndex` to state
  - Add `switchTransfer(index)` method
  - The conversation remains the single per-gear conversation (`gear.conversationId`)

- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Loan owner menu: add requestor name + avatar as the first item (non-actionable, informational header)
  - If `allActiveTransfers.length > 1`, make the header tappable to open a requestor selector (simple bottom sheet with avatar + name list)
  - "Arrange Pickup" item: open `FindBestTimeModal` using the gear's `conversationId` (the shared per-gear conversation)

- `app/lib/presentation/screens/experience/find_best_time_modal.dart`:
  - Generalize to accept a `conversationId` parameter (currently experience-specific)
  - For loans, the poll appears in the shared gear conversation visible to all parties

**Tests:** Verify requestor name shows in menu; verify switcher works with multiple requests; verify pickup poll creates in the gear conversation.

---

### Phase 5: Giveaway Post-Selection Menu (P7) and Completed Indicators (P8)

**Goal:** After giveaway recipient selection, show the recipient in the menu and add green checkmarks for completed workflow phases.

**Modified files:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - `_buildGiveawayOwnerMenuItems`: after recipient selection, add recipient name/avatar header (same pattern as P5 loan menu)
  - Add Arrange Pickup and Mark Picked Up items (mirroring loan menu)
  - Mark items as completed based on giveaway phase

- `app/lib/presentation/widgets/chat/action_dropdown_menu.dart`:
  - Add `isCompleted` field to `ActionDropdownItem`
  - When `isCompleted: true`: show green checkmark icon on the trailing edge, use muted text color, disable tap interaction
  - The green checkmark uses `Icons.check_circle` in `AppColors.sageDark`

- `app/lib/presentation/screens/gear/gear_content_view.dart` (loan menus too):
  - Set `isCompleted: true` on menu items based on transfer phase:
    - "Arrange Pickup" → completed when phase >= PICKUP_ARRANGED (or equivalent)
    - "Mark Picked Up" → completed when phase >= ACTIVE
    - "Mark Returned" → completed when phase >= COMPLETED

**Tests:** Verify giveaway menu shows recipient; verify completed items show checkmarks; verify completed items are not tappable.

---

### Phase 6: Info Button, Close Modal, and Audio Repositioning (P9)

**Goal:** Replace overflow (···) with info (i) button showing read-only details. Add "Change Details" and "Close [Item]" to the owner's manage menu. Move the volume button.

**Part A — Info button and modal:**

**Modified files:**
- `app/lib/presentation/widgets/content/content_owner_row.dart`:
  - Change overflow icon from `Icons.more_horiz` to `Icons.info_outline`
  - Move the volume/speaker button (video backgrounds only) to the left of the info button. The toggle logic already exists in the content views — just reposition the button into the owner row's trailing area.
  - Rename callback from `onOverflowTap` to `onInfoTap`

- `app/lib/presentation/widgets/content/content_overflow_menu.dart`:
  - Rename to `content_info_modal.dart` (or repurpose)
  - Remove Edit, Share, RSVP, Communities, Delete items
  - Show read-only information only:
    - Owner avatar + name
    - "Created" + formatted date
    - "Last updated" + formatted date
    - Background image attribution (if available, from media metadata)
  - Keep Report for non-owners

**Part B — Close modal in manage menu:**

**Modified files:**
- `app/lib/presentation/screens/gear/gear_content_view.dart` (and experience/request):
  - Add "Change Details" item to the owner's `MorphingActionButton` dropdown menu (triggers edit mode)
  - Add "Close Item" / "Close Event" / "Close Request" item to the manage menu
  - Tapping "Close" opens a `CloseItemModal` (generalized from the existing `CancelEventModal` in `cancel_event_modal.dart`)

- New: `app/lib/presentation/widgets/content/close_item_modal.dart`:
  - Generalized from `cancel_event_modal.dart` (which already has the three-option pattern)
  - Three options: Unshare from community, Mark as Cancelled, Delete permanently
  - Accepts content type parameter to customize labels ("event" vs "item" vs "request")
  - Callbacks for each action delegate to the respective ViewModel methods

**i18n:** Add ARB keys for info modal labels (`contentInfoCreated`, `contentInfoLastUpdated`, `contentInfoAttribution`) and close modal labels (`closeItemUnshare`, `closeItemCancel`, `closeItemDelete`, etc.).

**Tests:** Verify info icon renders; verify modal shows correct read-only information; verify "Close" in manage menu opens the modal; verify all three close options work; verify volume button position next to info button.

---

### Phase 7: Back Button on Chat Tab from Discover (P10)

**Goal:** When viewing content from the discover screen with the Chat tab active, show only the back button (no owner details).

**Modified files:**
- `app/lib/presentation/widgets/content/content_owner_row.dart`:
  - Add a `compactMode` parameter (default `false`)
  - When `compactMode: true`: render only the back button (no avatar, name, subtitle, or info button)
  - Alternatively, extract a `ContentBackButton` widget that both `ContentOwnerRow` and the compact mode use

- `app/lib/presentation/screens/gear/gear_content_view.dart` (and experience/request):
  - When `activeTab == 2` (Chat tab) and not in feed context (`!showFeedHeader`):
    - Show `ContentOwnerRow(compactMode: true)` instead of hiding the row entirely
  - When in feed context: behavior unchanged (feed header handles it)

**Tests:** Verify back button shows on Chat tab from discover; verify owner row shows on other tabs from discover; verify feed context behavior unchanged.

---

## Phase Dependencies

```
Phase 1 (nav bars)     — independent
Phase 2 (chat flicker) — independent
Phase 3 (time poll)    — independent
Phase 4 (loan menu)    — depends on Phase 3 (reuses FindBestTimeModal generalization)
Phase 5 (giveaway + checkmarks) — depends on Phase 4 (same menu pattern)
Phase 6 (info button)  — independent
Phase 7 (back button)  — independent
```

Phases 1, 2, 3, 6, 7 can be done in parallel. Phases 4 and 5 are sequential.

---

## File Impact Summary

### Modified Files
| File | Phases | Changes |
|------|--------|---------|
| `feed_screen.dart` | 1 | Scroll listener fix for nav bar settle |
| `inline_conversation_view.dart` | 2 | Re-request focus after send, layout fix |
| `gear_content_view.dart` | 2, 4, 5, 6, 7 | Chat pane layout, loan menu, giveaway menu, info button, back button |
| `experience_content_view.dart` | 2, 3, 6, 7 | Chat pane layout, time poll menu, info button, back button |
| `request_content_view.dart` | 2, 6, 7 | Chat pane layout, info button, back button |
| `find_best_time_modal.dart` | 3, 4 | Results mode, generalize for loans |
| `experience_view_model.dart` | 3 | confirmPollTime method |
| `gear_view_model.dart` | 4 | allActiveTransfers, selectedTransferIndex |
| `action_dropdown_menu.dart` | 5 | isCompleted field and rendering |
| `content_owner_row.dart` | 6, 7 | Info icon, audio button, compactMode |
| `content_overflow_menu.dart` | 6 | Rename/repurpose to info modal |
| `cancel_event_modal.dart` | 6 | Template for generalized `CloseItemModal` |
| `workflow_action_buttons.dart` | 5, 6 | Pass isCompleted flags; add Change Details + Close items |

### New Files
| File | Phase | Purpose |
|------|-------|---------|
| `close_item_modal.dart` | 6 | Generalized close/cancel/delete modal (from `CancelEventModal` pattern) |

### Potentially Renamed Files
| File | Phase | New Name |
|------|-------|----------|
| `content_overflow_menu.dart` | 6 | `content_info_modal.dart` |

---

**Last Updated:** 2026-03-27
