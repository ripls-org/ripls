# Issue #1255: Time poll creation bugs: black screen on request/need modal, and new poll overwrites closed poll comment

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1255
**Filed:** 2026-04-24 | **Last activity:** 2026-04-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Two distinct bugs surface when the owner creates a second time poll after closing the first one
(state: `timePollActive=false`, `timePollCompleted=true`):

1. **Black screen** — opening the request/need ("propose time") modal and submitting it causes the
   screen to go black instead of completing normally.
2. **Stale chat comment mutation** — creating a new poll via the time chip on the detail tab does
   NOT add a new system message to the chat. Instead the *existing* closed-poll chat comment
   visually re-points to the new poll, making it appear as though the same comment was mutated.

Both bugs share the same precondition: a completed poll (`timePollCompleted=true`) sitting on the
experience record.

## Context from Comments

No comments — the issue was filed directly from a bug report. No maintainer direction yet.

## Root Cause Analysis

### Bug 1: Black screen

The `TimePollProposeModal._save()` method
([`time_poll_propose_modal.dart:101-149`](app/lib/presentation/screens/experience/time_poll_propose_modal.dart#L101))
calls `notifier.createTimePoll(options)`. That method
([`time_modal_view_model.dart:875`](app/lib/presentation/viewmodels/time_modal_view_model.dart#L875))
sets `state = const AsyncLoading()` immediately, which causes the entire modal's `build()` to
re-render into a loading spinner. If the navigation stack then pops (or the modal dismisses) before
the `AsyncLoading → AsyncData` transition completes, Flutter renders nothing — a black screen.

The likeliest trigger is that the modal pops itself after calling `createTimePoll` (line 131 of the
modal calls `context.pop()` or similar after the await), but the ViewModel's AsyncLoading state
races with that pop, leaving a momentary black frame that persists because the parent route is gone.
Alternatively, `createTimePoll` throws an unhandled exception on the `timePollCompleted=true` path
and the modal never navigates away.

### Bug 2A: Old chat comment appears to change poll

The chat message renderer
([`message_list.dart:164-175`](app/lib/presentation/widgets/chat/message_list.dart#L164) and
`_PollBannerOrLabel` widget at lines 982–998) does **not** use a poll_id stored on the chat
message. Instead it watches `experienceProvider(experienceId)` and checks the live
`experience.timePollActive` flag and `experience.currentPollId`. When a new poll is created, the
experience record's `currentPollId` changes, so every `CHAT_SYSTEM_ACTION_TIME_PROPOSED` message
in chat — including the old one from the closed poll — now renders the new poll's data. The message
itself was never updated; only the experience record changed, but all messages share that live
reference.

### Bug 2B: No new chat comment added

The server's `ProposeTime` handler
([`time_proposals.go:61`](server/services/experience/time_proposals.go#L61)) derives
`isFirstPollOption := !expStored.TimePollActive`. After a cancelled poll `TimePollActive=false`, so
`isFirstPollOption=true` and the code path at lines 83–105 *should* call `InsertSystemMessage`.
The message insertion failure is only logged as a `WarnContext` (line 99) and does not propagate,
so a transient error here would be silently swallowed. More importantly, even if the server does
insert the message, the client-side rendering (Bug 2A) means both the old and the new message would
render identically, making the new insertion invisible or indistinguishable.

## Potential Solutions

### Option A: Fix rendering — anchor messages to poll_id at creation time (recommended)

Store the `poll_id` on the `ChatMessage` record when the system message is inserted (or embed it
in the message metadata). The chat renderer reads `poll_id` from the message record, not from the
live experience state. Old messages retain their original `poll_id`; new messages get the new one.

- **Pros:** Correct by construction. Each poll comment is permanently tied to the poll that
  created it. Handles any future "third poll, fourth poll" scenario automatically.
- **Cons:** Requires a proto change (add `poll_id` field to `ChatMessage`/`InsertSystemMessageOpts`),
  a storage migration, and a client-side renderer change. Medium scope.
- **Effort:** Medium

### Option B: Scope chat message rendering to closed-state awareness

Keep the live experience reference but add a closed-state check: if the message was posted before
the current `currentPollId` was created (compare `createdAt` timestamps), render it as a
permanently closed/historical banner regardless of live experience state.

- **Pros:** No proto or storage migration. Pure client logic.
- **Cons:** Fragile — relies on timestamp comparisons that can race. Does not fix the case where
  a new poll is created very quickly after closing the old one. Does not fix the missing new
  message (Bug 2B).
- **Effort:** Small, but incomplete.

### Option C: Deduplicate at render time — show only one live poll banner

Only the *most recent* `CHAT_SYSTEM_ACTION_TIME_PROPOSED` message shows the live poll banner; older
ones in the same conversation degrade to a static "time poll was opened" label.

- **Pros:** Eliminates the visual confusion of multiple messages pointing to one poll.
- **Cons:** Requires identifying "most recent" message of that action type in the message list,
  which requires client-side filtering. Does not fix the missing new chat message.
- **Effort:** Small.

## Recommendation

**Option A** is the correct long-term fix and should be the goal. The `poll_id` field already
exists on `ExperienceTimeProposal` — the same concept needs to flow into the system message.

In the short term, pair Option A with a targeted fix for the black screen (Bug 1) and a fix to
ensure `InsertSystemMessage` errors surface loudly rather than being silently swallowed.

## Open Questions

- **Why does the black screen happen specifically on the request/need modal and not the time chip
  path?** Both ultimately call `createTimePoll()`. Recommendation: instrument both paths with
  error logging to catch unhandled exceptions; suspect the request/need modal has a different
  `context.pop()` or dismiss-on-completion path that races with AsyncLoading.
- **Does the server actually insert the new system message on the second poll?** The warn-and-
  continue pattern at `time_proposals.go:99` makes this invisible in logs. Recommendation: promote
  this to an error return so failures are caught in tests.
- **Should `poll_id` be added to the protobuf `InsertSystemMessageOpts` or stored as opaque
  metadata?** Recommendation: add an optional `poll_id` field to `InsertSystemMessageOpts` and
  thread it through to the `ChatMessage` storage model; this is explicit and type-safe.
- **What's the `ChatMessage` storage schema — does it have an extensible metadata field already?**
  Recommendation: check `proto/ripls/models/chat.proto` for an existing `metadata` map before
  adding a new typed field.

## Implementation Plan

### Phase 1: Fix the black screen (Bug 1)
**Goal:** Eliminate the black screen when creating a new poll via the request/need modal.

> **Audit note:** Error handling must live in the ViewModel, not the modal widget. Per the client
> architecture, screens only observe ViewModel state — they must not contain `try/catch` business
> logic. Likewise, any async gap after `await createTimePoll()` in the ViewModel requires a
> `ref.mounted` check (or `SafeNotifierMixin.safeUpdateState`) before accessing `state`, per the
> async safety rules for `autoDispose` providers.

**Tasks:**
- [ ] Read `time_poll_propose_modal.dart:101-149` (`_save()`) and trace exactly when
  `context.pop()` / `Navigator.pop()` is called relative to the `createTimePoll()` await.
- [ ] Confirm whether the black screen is a navigation race or an unhandled exception by adding
  error state (`String? errorMessage`) to `TimeModalViewModel`'s Freezed state and surfacing any
  thrown exception there — do not add `try/catch` inside the modal widget.
- [ ] In `createTimePoll()` (`time_modal_view_model.dart:875`), add `if (!ref.mounted) return;`
  after every `await` before assigning to `state`, matching the async safety pattern required
  for `autoDispose` providers. Alternatively, migrate to `SafeNotifierMixin.safeUpdateState`.
- [ ] Fix navigation: the modal should pop only after the ViewModel transitions out of
  `AsyncLoading` — watch ViewModel state in `_save()` and gate the `context.pop()` on
  success state, not on the raw await completing. If the ViewModel surfaces an error state,
  show a snackbar (reading the ViewModel error) instead of popping.
- [ ] Promote the `InsertSystemMessage` warn-and-continue at `time_proposals.go:99` to a returned
  error so it is visible in server logs and fails tests.
**Validates:** Request/need modal creates a new poll and returns the user to the previous screen
without any black frame, even after a prior poll has been closed.

### Phase 2: Fix message rendering — anchor to poll_id (Bug 2A + 2B)
**Goal:** Each "time poll opened" chat message renders the poll it was created for, not the
current live poll on the experience.
**Tasks:**
- [ ] Add an optional `poll_id` field to `InsertSystemMessageOpts` in
  `server/chat/system_messages.go` (and the underlying storage model if needed).
- [ ] Thread `pollID` into the `InsertSystemMessage` call in `time_proposals.go:93-103`.
- [ ] Store `poll_id` on the `ChatMessage` storage record (proto change +
  migration if required).
- [ ] Update the chat message renderer (`message_list.dart:164-175`, `_PollBannerOrLabel` at
  lines 982–998) to read `poll_id` from the message and pass it to the time poll tap handler
  rather than reading the live `experience.currentPollId`.
- [ ] Ensure old messages without a `poll_id` (backcompat) continue to render as historical
  closed-poll labels rather than live poll banners.
**Validates:** After creating a second poll, the first closed-poll comment still shows the first
poll; a new second-poll comment appears below it pointing to the new poll.

### Phase 3: Tests
**Goal:** Prevent regression.
**Tasks:**
- [ ] Add a server integration test for `ProposeTime` that: closes a poll, then calls
  `ProposeTime` again, and asserts that exactly two `CHAT_SYSTEM_ACTION_TIME_PROPOSED` system
  messages are present in the conversation.
- [ ] Add a Flutter widget test through `MessageList` (not directly on the private
  `_PollBannerOrLabel` widget, which cannot be imported from test files) asserting that a
  `CHAT_SYSTEM_ACTION_TIME_PROPOSED` message with a stale `poll_id` renders as a historical
  closed-poll label even when the experience has a newer active poll. If isolation is needed,
  make `PollBannerOrLabel` package-private (remove the leading `_`) before writing the test.
- [ ] Add a Flutter integration/golden test for the request/need propose modal that verifies it
  navigates away correctly after poll creation (no black screen).
**Validates:** CI catches both bugs if they regress.

## Related Issues

- #1219 — Plan tab time poll, likely shares the same time poll state management surface area.
- #1188 — Coalesce system messages, relevant to how `CHAT_SYSTEM_ACTION_TIME_PROPOSED` is
  (intentionally) not coalesced.
- #925 — System messages not showing, prior investigation into system message insertion failures
  that may have similar root causes.
