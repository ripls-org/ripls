# Issue #1124: [Bug Report] Multiple photo select

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1124
**Filed:** 2026-04-14 | **Last activity:** 2026-04-14
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none
**Also addresses:** #1029 (Remove old conversation screen UI)

## Problem Statement

When a user taps the '+' button next to the text input in chat to add a photo,
they can only add a single image per message. Selecting a photo immediately
uploads it and sends the message. There is no opportunity to select multiple
photos or to review the selection before sending.

The real ask (clarified in comments) is: **support sending multiple photos in a
single chat message** via the '+' attachment button.

Additionally, there are two separate chat interfaces — an old standalone
`ConversationScreen` and a new inline `InlineConversationView` embedded as the
chat tab in item screens (gear, request, experience). The old screen is tech
debt (#1029) and should be removed as part of this work. All multi-photo work
targets the `InlineConversationView`.

## Context from Comments

- **maintainer** confirmed the bug and identified the UX split: the '+'
  button only supports single-pick, while the media bar supports multi-select.
- **maintainer** noted this "will probably require evolving the chat message
  data model."
- **tescobar** confirmed this was intentionally split into two features and
  described it as "a well trod problem/solution space."

**Key finding from codebase research:** The data model does NOT need to change.
`UserChatMessage` already has `repeated string media_ids = 3` in the proto.
The server `SendMessage` RPC already accepts and stores multiple media IDs.
The `_buildMediaAttachments()` widget already renders multiple images in a
vertical column. The gap is purely in the client-side pick-and-upload flow.

## Two Chat Interfaces

### Old: `ConversationScreen` (to be removed)

- **File:** `app/lib/presentation/screens/inbox/conversation_screen.dart` (1,581 lines)
- Full-screen standalone chat with glassmorphic header, workflow action buttons,
  background media, and swipe-to-close
- Navigated to from:
  - `portfolio_inbox_screen.dart` via `_navigateToConversation()`
  - 7 modal files (loan_borrower_modal, loan_owner_modal, loan_manage_modal,
    event_organizer_modal, event_joiner_modal, request_management_modal,
    request_help_modal)
- Uses exclusive widgets that no other code depends on:
  - `conversation_top_bar.dart`
  - `message_input_bar.dart` (540 lines)
  - `conversation_action_button.dart` (624 lines)
  - `workflow_widget_factory.dart` (308 lines)

### New: `InlineConversationView` (the target)

- **File:** `app/lib/presentation/widgets/content/inline_conversation_view.dart` (841 lines)
- Embedded as chat tab (tab index 2) in `GearContentView`, `RequestContentView`,
  `ExperienceContentView`
- Has collapsed (3-message preview) and expanded (full chat) modes
- Uses its own pill-style input with attach button + send button
- **Both interfaces share:** `conversation_view_model.dart`, `message_list.dart`

### Navigation impact

All callers that currently push `ConversationScreen` need to instead navigate
to the item screen with `initialTab: 2` (the chat tab). The portfolio inbox
already has `_navigateToContent()` that goes to item screens — the
`_navigateToConversation()` method just needs to call it instead. Modals that
navigate to the conversation screen should close the modal and either do
nothing (user is already on the item screen) or navigate to the item screen
with the chat tab selected.

## Current Architecture

### What already works for multiple media

1. **Proto model:** `UserChatMessage` has `repeated string media_ids = 3`
   (`proto/ripls/models/chat.proto:86`)
2. **API:** `SendMessageRequest` has `repeated string media_ids = 3`
   (`proto/ripls/api/chat_service.proto:42`)
3. **Server:** `SendMessage` in `server/services/chat/messages.go` stores and
   broadcasts the full `MediaIds` list
4. **Display:** `_buildMediaAttachments()` in `message_list.dart:624-714`
   renders all media IDs as a vertical column of images
5. **Picker helper:** `MediaPickerHelper.pickMultipleImagesFromGallery()` exists
   in `media_picker_helper.dart:64-81`
6. **Upload mixin:** `MediaUploadMixin.pickAndUploadMultipleImagesFromGallery()`
   exists in `media_upload_mixin.dart:144-155` with batch progress/failure
   handling

### What doesn't work

1. **InlineConversationView** (`inline_conversation_view.dart:674-714`):
   `_pickAndSendMedia()` calls `MediaPickerHelper.pickImageFromGallery()` (single
   pick) and immediately sends via `notifier.sendMessageWithImage(filePath)`
2. **Conversation view model** (`conversation_view_model.dart:1375-1400`):
   `sendMessageWithImage()` uploads one file, then calls
   `sendMessage(text, mediaIds: [mediaId])` with a single-element list
3. **No staging area:** Photos are immediately uploaded and sent — there's no
   state for "selected but not yet sent" images

## Potential Solutions

### Option A: Direct multi-pick and send

- **Approach:** Change the "Photo" option to call
  `pickMultipleImagesFromGallery()`. Upload all images sequentially, collect
  the media IDs, then call `sendMessage()` with the full list. No staging
  area — user picks, uploads happen, message is sent when all complete.
- **Pros:** Minimal UI changes. Simple implementation.
- **Cons:** No preview before sending. User can't remove a photo after
  selection. Upload progress is less visible.
- **Effort:** Small.

### Option B: Staging area with preview before send

- **Approach:** After picking photos, show thumbnails of selected images above
  the input bar (like iMessage or WhatsApp). User can review, remove individual
  photos, add more, then tap send. Uploads happen after send is tapped.
- **Pros:** Best UX — matches user expectations from major messaging apps.
  User has full control before committing. Can add text alongside photos.
- **Cons:** More UI work — need a thumbnail strip, remove buttons per image,
  and state management for pending attachments.
- **Effort:** Medium.

### Option C: Staging area with eager upload

- **Approach:** Same UI as Option B, but uploads begin immediately after
  selection. By the time the user taps send, most uploads are complete.
- **Pros:** Fastest perceived send time.
- **Cons:** Wastes bandwidth if user removes a photo after upload. Most complex
  to implement.
- **Effort:** Medium-large.

## Recommendation

**Option B: Staging area with preview before send.** This delivers the best UX
for the complexity involved:

1. Users expect to preview photos before sending in a chat app — every major
   messaging app works this way. Option A's "pick = instant send" pattern
   feels wrong for multiple photos.
2. The implementation is straightforward: add a `List<String>` of pending file
   paths to the conversation state, show local file thumbnails in a row above
   the input bar, and upload+send when the user taps the send button.
3. Option C's eager upload adds complexity for marginal benefit — typical chat
   photos are small and upload quickly.
4. The `MediaUploadMixin` pattern can't be directly used here (it's a
   `Notifier` mixin, and `ConversationNotifier` has its own upload pattern),
   but the sequential-upload-with-progress logic can be replicated.

## Open Questions

- **Should the camera option also support multiple captures?** —
  Recommendation: No. Camera stays single-capture. The user can take a camera
  photo (which stages it), then add more from gallery.

- **What's the maximum number of photos per chat message?** — Recommendation:
  Cap at 10 for chat messages (vs 20 for gear/experiences). Chat photos are
  typically quick snapshots. Can always raise later.

- **Should the vertical column layout be changed to a grid for multiple
  photos?** — Recommendation: Not in this issue. The current vertical stack
  works. A grid layout would be a nice follow-up but is a separate UX concern.

- **How should the staging thumbnails interact with the text input?** —
  Recommendation: Show a horizontal thumbnail strip above the pill input bar.
  Each thumbnail has an 'x' button to remove. The text field remains available
  for adding text alongside the photos. The send button sends both together.

- **What happens to modal-to-conversation navigation when the old screen is
  removed?** — Recommendation: Modals that currently push `ConversationScreen`
  should instead either (a) close the modal (user is already on the item
  screen that has the chat tab) or (b) navigate to the item screen with
  `initialTab: 2`. Most modals are launched from item screens, so option (a)
  applies to most cases.

## Implementation Plan

### Phase 1: Remove old ConversationScreen (#1029)

**Goal:** Remove the old standalone chat screen and redirect all navigation to
item screens with the chat tab.

**Tasks:**

- [x] Update `portfolio_inbox_screen.dart`: change `_navigateToConversation()`
      to call `_navigateToContent()` instead of pushing `ConversationScreen`.
      Pass `initialTab: 2` to `GearScreen`, `RequestScreen`, and
      `ExperienceScreen` constructors so they open on the chat tab
- [x] Update modal files that navigate to `ConversationScreen` (7 files):
      `event_organizer_modal.dart`, `event_joiner_modal.dart`,
      `loan_borrower_modal.dart`, `loan_owner_modal.dart`,
      `loan_manage_modal.dart`, `request_management_modal.dart`,
      `request_help_modal.dart` — replaced with `Navigator.pop(context)` to
      close the modal (chat tab is on the parent item screen underneath)
- [x] Delete `conversation_screen.dart` (1,581 lines)
- [x] Delete old-only widgets: `conversation_top_bar.dart`,
      `message_input_bar.dart`, `conversation_action_button.dart`,
      `workflow_widget_factory.dart` (~1,472 lines total)
- [x] Delete associated test files:
      `conversation_screen_chat_bubble_test.dart`,
      `message_input_bar_test.dart`, `loan_card_test.dart`
- [ ] Update patrol test page if it references conversation screen:
      `app/patrol_test/pages/conversation_page.dart` (deferred — 19 patrol
      tests reference ConversationPage and need separate update)
- [x] No notification deep-link routing targets `ConversationScreen`
- [x] Build passes and all 3,430 tests pass

**Validates:** No code references `ConversationScreen`. All inbox and modal
navigation lands on item screens with the chat tab visible.

### Phase 2: View model support for staged attachments

**Goal:** Add state and methods to `ConversationNotifier` for staging photos
before sending.

**Tasks:**

- [x] Add `pendingAttachments` field (`List<String>` of local file paths) to
      `ConversationState` in `conversation_view_model.dart`
- [x] Add `addPendingAttachments(List<String> filePaths)` method to
      `ConversationNotifier` that appends to the list (capped at 10)
- [x] Add `removePendingAttachment(int index)` method
- [x] Add `clearPendingAttachments()` method
- [x] Add `sendMessageWithPendingAttachments({String? text})` method with
      sequential upload, `ref.mounted` checks, and error handling
- [x] Remove `sendMessageWithImage()` and `sendMessageWithVideo()` from
      `ConversationNotifier` (no longer needed — staging replaces direct send)
- [x] Add 11 unit tests for the new methods (stage, remove, clear, send flow)

**Validates:** View model correctly manages staged attachment state and
upload+send flow.

### Phase 3: Multi-photo pick and staging in InlineConversationView

**Goal:** Wire multi-select gallery into the inline chat's attach button and
add a thumbnail preview strip.

**Tasks:**

- [x] Change `_pickAndSendMedia` to `_pickAndStageMedia` using
      `MediaPickerHelper.pickMultipleImagesFromGallery()` for photos
- [x] Single-pick for camera and video stages instead of sending immediately
- [x] Send button handler sends with pending attachments; button visible
      when attachments are staged (even with no text)
- [x] `_buildPendingAttachments()` widget: horizontal scrollable thumbnail
      strip with `Image.file()`, positioned above the pill input bar
- [x] Each thumbnail has 'x' remove button calling
      `notifier.removePendingAttachment(index)`
- [x] `_buildUploadProgress()` shows linear progress bar with
      "Uploading N of M..." text
- [ ] Add upload progress strings to `app_en.arb` and `app_es.arb` (currently
      hardcoded English — should be localized in a follow-up)
- [ ] Test on iOS and Android: verify gallery multi-select UX, thumbnail
      rendering, remove behavior, and send flow end-to-end

**Validates:** Full UX works: pick multiple photos, see thumbnails, remove
unwanted ones, add text, tap send, see upload progress, message appears in
chat with all photos.

## Related Issues

- #1029 — Remove old conversation screen UI (addressed in Phase 1)
- #1083 — Multi-image upload for gear/experiences. Infrastructure built for
  that issue (`pickMultipleImagesFromGallery`, `MediaUploadMixin` batch upload)
  provides the foundation. This issue extends multi-photo to chat messages.

---

## Audit Notes

_Audited against: docs/client/architecture.md, docs/client/caching.md,
docs/client/i18n.md, docs/server/architecture.md_

**Changes made:**

1. **Async safety (Phase 2):** Added `ref.mounted` check requirement between
   sequential uploads in `sendMessageWithPendingAttachments`. The
   `conversationProvider` uses `autoDispose`, so the notifier can be disposed
   mid-upload if the user navigates away. This matches the established pattern
   documented in architecture.md (Async Safety in Notifiers) and the existing
   `MediaUploadMixin._uploadMultipleMediaFiles()` implementation which already
   does this.

2. **i18n compliance (Phase 3):** Expanded the ARB task to specify ICU message
   format for count placeholders and to remind that ViewModels expose typed
   state (counts), not resolved strings — widgets resolve via `context.l10n`
   in `build()` (per docs/client/i18n.md).

3. **Markdown lint fixes:** Removed trailing colons from two headings (MD026).

**No issues found with:**

- **Caching strategy:** Upload flow correctly uses `mediaRepository.addMedia()`
  which doesn't cache write operations. Post-upload reads go through existing
  cached `getMedia()` / `getMediaUrl()` paths. No manual cache maps proposed
  in ViewModels.
- **Repository pattern:** All data access flows through repositories via
  `ref.read(mediaRepositoryProvider)`. No direct service access from
  ViewModels.
- **Server/client boundary:** No server changes required. The existing
  `SendMessage` RPC already handles `repeated string media_ids`. No new
  proto fields needed.
- **Navigation pattern:** Phase 1 correctly uses `NavigationHelpers.pushScreen()`
  via the existing `_navigateToContent()` method. No raw `Navigator.push`
  introduced.
- **State management:** Freezed state with `copyWith()` for pending
  attachments. Notifier pattern with Riverpod providers. No `ChangeNotifier`
  or `StatefulWidget` for business logic.
