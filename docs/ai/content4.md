# Content View Tabbed Redesign (content4)

## Problem Statement

The current content views (gear, experience, request) use a **floating action button (FAB) column** on the right side of the screen to access secondary features: media gallery, share, overflow menu, metrics, and volume control. The conversation is accessed via a comment bar at the bottom, and metadata via a chip tap. This creates several UX problems:

1. **Discoverability**: FABs are compact icons without labels — new users don't know what they do until they tap them.
2. **Context switching**: Tapping a FAB opens a full-screen modal (media carousel, conversation screen, metadata sheet), losing the visual context of the content behind it.
3. **Right-side clutter**: The FAB column occupies ~43px of horizontal space, forcing the content area to add right padding and reducing readable width.
4. **Inconsistent access patterns**: Chat is accessed via a comment bar, metadata via a chip, media via a FAB, and editing via an overflow menu item — four different interaction patterns for related features.
5. **Modal fatigue**: Every secondary action navigates away from the content view, requiring back-navigation to return.

## Desired State

Replace FABs with a **four-tab interface** overlaid on the background media, inside a semi-opaque dark content area at the bottom of the screen. The tabs consolidate all content into a single, swipeable view:

| Tab | Label | Content |
|-----|-------|---------|
| **Type** (e.g., "Loan") | Dynamic per content type | Title, description, location chip, action button + share button |
| **Details** | "Details" | Metric tiles (value, weight, interested count) + key-value detail rows |
| **Chat** | "Chat" + unread badge | Inline conversation messages + reply input; tappable to expand |
| **Media** | "Media" + count badge | 2-column grid of media thumbnails; tapping selects background image |

### Key Design Principles (from reference designs)

- **Owner row** at top: avatar + name + subtitle + overflow (···) button
- **Edit bar** replaces owner row when editing: Cancel / "Editing" label / Save (accent color)
- **Tab bar** sits between background image and content pane, with pill-shaped tabs
- **Active tab** gets accent background color; inactive tabs get glass morphism
- **Chat badge** (unread count) shown on Chat tab with accent dot; **Media badge** (count) shown on Media tab
- **Content pane** has semi-transparent dark background with glass morphism
- **Swipe between tabs** supported via touch gestures
- **Edit mode** is global across tabs — editing on Type tab edits title/description/location; editing on Details tab edits metadata fields; editing on Media tab enables add/delete/reorder

### Cross-Cutting Concerns

- **i18n (REQUIRED)**: All user-visible strings must use `context.l10n.*` — tab labels ("Loan", "Details", "Chat", "Media", "Event", "Request"), edit bar labels ("Cancel", "Editing", "Save"), button text ("Request to Borrow", "Share", "Read more", "Add a comment…"), and any new UI copy. Add ARB keys to both `app_en.arb` and `app_es.arb` before each phase is complete. ViewModels must NOT resolve display strings — they expose typed state (enums, counts); widgets resolve to localized text in `build()`. Run `npm run lint:dart:i18n` to catch violations.
- **Theme-aware colors (REQUIRED)**: New shared widgets (`ContentTabBar`, `ContentEditBar`, `ContentOwnerRow`) must use `AppColors.*` helpers for any colors that vary by theme. Dark semi-transparent overlays on background media (e.g., `Colors.black.withOpacity(0.2)`) are acceptable since they sit atop user photos regardless of theme. Do NOT copy hardcoded rgba hex values from the JSX reference into Dart code for backgrounds, text colors, or borders — use `AppColors.textPrimary(context)`, `AppColors.textSecondary(context)`, `AppColors.border(context)`, etc.
- **Async safety (REQUIRED)**: All async ViewModel methods (lazy chat loading, message sending, metadata saving) must include `ref.mounted` checks after every `await` before accessing `state`. Use `SafeNotifierMixin` or `AsyncNotifier` patterns per `docs/client/architecture.md`.

## Reference Designs

| File | Description |
|------|-------------|
| `tmp/content4/loan.png` | Main (Type) tab — title, description, location, action button |
| `tmp/content4/loan-details.png` | Details tab — metric tiles + key-value rows |
| `tmp/content4/loan-chat.png` | Chat tab — inline messages + reply input |
| `tmp/content4/loan-media.png` | Media tab — 2-column thumbnail grid |
| `tmp/content4/overflow-menu.png` | Overflow menu — Edit, Communities, Delete + photo credit |
| `tmp/content4/edit-main.png` | Edit mode on Type tab — editable title, location, description |
| `tmp/content4/edit-details.png` | Edit mode on Details tab — editable detail rows |
| `tmp/content4/edit-media.png` | Edit mode on Media tab — add/delete thumbnails |
| `tmp/content4/content4.jsx` | Complete React reference implementation |

## Current Architecture (What Changes)

### Removed Components
- **ContentFloatingActions** — entire FAB column removed from content views
- **CommentBar / action bar** at bottom — replaced by action button inside Type tab
- **Full-screen conversation navigation** — replaced by inline Chat tab (with expand-to-fullscreen)
- **Full-screen metadata sheet navigation** — replaced by Details tab
- **Full-screen media carousel navigation** — replaced by Media tab (tapping a thumbnail still changes background)
- **Right-side padding** (43px) in content area — no longer needed without FABs

### Preserved Components
- **ContentEditingMixin** — still used for editing buffer management
- **ContentViewBuilders** — static builder methods still used for loading/error/gradient/title/description
- **ContentViewHelpers** — save/delete/media picker helpers still used
- **ContentOwnerSection** — adapted for the new owner row at top
- **CommunityContextHeader** — still shown in owner subtitle area
- **MediaBackground** — still used for background media display
- **ContentGradientOverlay** — still used
- **Overflow menu** (showContentOverflowMenu) — still triggered from ··· button in owner row
- **WorkflowActionButtons / MorphingActionButton** — moved into Type tab's action area
- **GearMetadataSheet** — functionality absorbed into Details tab, but the sheet's field logic/auto-fill can be reused

### Modified Components
- **GearContentView** — major rewrite to use tabbed layout instead of FAB+modal pattern
- **GearViewModel** — add `activeTab` to state, remove FAB-related methods
- **ExperienceContentView / RequestContentView** — same tabbed pattern (Phase 2+)

## Recommendations

1. **Start with GearContentView only.** It's the most complex content type (multi-media, metadata, loans/giveaways, chat). Getting it right here validates the pattern before extending to experience and request.

2. **Build the tab bar as a new shared widget** (`ContentTabBar`) so it can be reused across all content types with different tab labels and badges.

3. **Keep the chat tab as a lightweight inline view** — show recent messages + reply input. On tap/expand, it grows to fill ~82% of the screen height (matching the JSX reference). Do NOT navigate to a separate ConversationScreen.

4. **Reuse existing conversation infrastructure** — the `ConversationNotifier` and message rendering from `conversation_screen.dart` should be extracted into a reusable widget (`InlineConversationView`) that can work both inline and full-screen.

5. **The Details tab should inline the metadata fields** that currently live in `GearMetadataSheet`. The metric tiles (value, weight, interested count) go at the top; the key-value detail rows below.

6. **The Media tab replaces MediaCarousel** for browsing. Tapping a thumbnail changes the background image. In edit mode, thumbnails get delete (x) buttons and an add (+) cell appears.

7. **Keep the overflow menu as-is** — it's triggered from the ··· button in the owner row (top right). Edit, Communities, Delete, Report options remain the same.

8. **Feed context**: When shown in a feed, the feed header overlay still appears at the top. The tab bar and content pane appear at the bottom. The owner row is hidden in feed context (feed header replaces it).

## Design Decisions (Resolved)

1. **Chat tab keyboard handling**: Yes, auto-expand on focus. Typing a reply needs more space.

2. **Chat tab message loading**: Lazy load on first tab selection to avoid unnecessary API calls.

3. **Tab persistence**: Yes, persist the active tab position when navigating away and back.

4. **Swipe gesture conflicts**: Only enable swipe-between-tabs when the content view is full-screen (not in feed context). In feed context, use tab taps only.

5. **Video volume control**: Overlay it as a small button on the background media area (bottom-right corner, above the tab bar).

6. **Share button**: Follow the reference design — share button sits alongside the action button on the Type tab. Use the existing share icon (not the one in the reference design).

7. **Metrics/equity**: Add metrics to the Details tab detail table. Remove the standalone metrics FAB.

8. **Edit mode scope**: Entering edit mode enables editing on all tabs. Save commits all changes across all tabs. Cancel reverts all changes.

9. **Experience tab labels**: "Event" as the type tab label. Details tab shows date/time/RSVP instead of value/weight.

10. **Request tab labels**: "Request" as the type tab label. Details tab shows type/location/duration instead of value/weight. Media tab may have only 1 item (requests support single photo).

---

## Phased Implementation Plan

### Phase 0: Shared Tab Infrastructure
**Goal**: Build the reusable tab bar widget and supporting types.

**New files**:
- `app/lib/presentation/widgets/content/content_tab_bar.dart` — `ContentTabBar` widget
  - Accepts: `tabs` (list of `ContentTab`), `activeIndex`, `onChanged`, `accentColor`, `isEditing`
  - `ContentTab` data class: `label`, `badge` (optional String), `showBadgeDot` (bool)
  - Pill-shaped tab buttons with glass morphism (inactive) or accent fill (active)
  - Badge rendering: accent-colored dot badge for Chat, count badge for Media
  - Hides Chat badge when editing (per reference)
  - Horizontally distributed, equal-width tabs
  - Tests: tab rendering, active state, badge display, editing state

**New files**:
- `app/lib/presentation/widgets/content/content_edit_bar.dart` — `ContentEditBar` widget
  - Cancel button (glass pill) / "Editing" label (glass pill) / Save button (accent pill)
  - Positioned at top of screen, replaces owner row during edit mode
  - `onCancel`, `onSave` callbacks
  - Tests: rendering, tap callbacks

**Modified files**:
- None yet — these are new standalone widgets

**i18n**: Add ARB keys for ContentEditBar labels (`contentEditBarCancel`, `contentEditBarEditing`, `contentEditBarSave`) and any shared tab labels (`contentTabDetails`, `contentTabChat`, `contentTabMedia`) to both `app_en.arb` and `app_es.arb`. Type-specific tab labels (e.g., `gearTabLoan`, `experienceTabEvent`, `requestTabRequest`) are added in the phase that introduces each content type.

**Tests**: ~20 tests for ContentTabBar + ContentEditBar

---

### Phase 1: Gear Type Tab (Main Tab)
**Goal**: Replace the bottom content section + comment bar + action bar with the Type tab pane.

**Modified files**:
- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Remove `ContentFloatingActions` usage
  - Remove right-side padding (43px)
  - Remove `CommentBar` and action bar from bottom
  - Add `ContentTabBar` between background and content pane
  - Read `activeTab` from ViewModel state (not local widget state — tab persistence requires ViewModel ownership)
  - Implement Type tab pane:
    - **Read mode**: Title (serif, 24pt), location chip, expandable description, action button + share button row
    - **Edit mode**: Editable title field, tappable location (opens location modal), editable description textarea
  - Move action button logic (Request to Borrow / I'm Interested / etc.) from action bar into Type tab
  - Move share button from FAB into Type tab action row
  - Owner row at top: avatar + name + subtitle + ··· overflow button
  - Edit bar replaces owner row when editing
  - Support horizontal swipe between tabs (non-feed context only)

- `app/lib/presentation/viewmodels/gear_view_model.dart`:
  - Add `activeTab` (int) to GearState
  - Remove `_buildFloatingActionButtons()` method
  - Add `setActiveTab(int)` method

**New files**:
- `app/lib/presentation/widgets/content/content_owner_row.dart` — Owner row with avatar + name + subtitle + overflow button (extracted from current owner section, adapted for new top-bar layout)

**Tests**: ~15 tests for Type tab rendering in read/edit modes

---

### Phase 2: Gear Details Tab
**Goal**: Replace the metadata sheet modal with an inline Details tab.

**Modified files**:
- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Implement Details tab pane:
    - **Read mode**: 3 metric tiles in a row (glass cards: value, weight, interested count) + key-value detail rows in a glass card (brand, model, blade/material, location, listed date)
    - **Edit mode**: Metric tiles become read-only (computed values), detail rows become editable input fields
  - Remove metadata chip from Type tab (no longer needed — Details tab replaces it)
  - Remove `_showMetadataSheet()` method

- `app/lib/presentation/viewmodels/gear_view_model.dart`:
  - Add editing state for detail fields (or extend ContentEditingMixin)
  - `saveChanges()` now includes metadata edits from Details tab
  - Remove `_showMetadataSheet` related code

**Reused from GearMetadataSheet**:
  - Field definitions (brand, model, category, material, weight, value)
  - Auto-fill callback logic (sourceUrl detection)
  - Value/weight formatting

**Tests**: ~15 tests for Details tab rendering and editing

---

### Phase 3: Gear Chat Tab (Minimal)
**Goal**: Wire up the Chat tab with a minimal inline conversation placeholder. Phase 9 replaces this with the full conversation experience.

**New files**:
- `app/lib/presentation/widgets/content/inline_conversation_view.dart` — `InlineConversationView`
  - **ConsumerStatefulWidget** — reads `conversationProvider(conversationId)` via `ref.watch()` for message state (per architecture: screens interact with ViewModels only, never repositories directly)
  - Renders recent messages in a scrollable list
  - Reply input at bottom with send button
  - `expanded` mode: fills ~82% of screen height with slide-up animation
  - Tapping when collapsed expands; focusing input expands
  - Renders messages with avatar + name + glass bubble (matching reference design)
  - Unread badge count passed to parent for tab badge
  - All user-visible strings (placeholder text, timestamps) use `context.l10n`
  - **Note**: This is a simplified first pass. Phase 9 replaces this with the full `ConversationScreen` experience (widget cards, workflow progress bar, participant badges, image upload, etc.)

**Modified files**:
- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Implement Chat tab using `InlineConversationView`
  - Remove `_openChat()` navigation method
  - Pass unread count to `ContentTabBar` for Chat badge
  - Chat tab expand/collapse state management

- `app/lib/presentation/viewmodels/gear_view_model.dart`:
  - Expose `unreadCount` for Chat tab badge
  - Load conversation lazily on first Chat tab selection

**Tests**: ~20 tests for InlineConversationView (messages, reply, expand/collapse, badges)

---

### Phase 4: Gear Media Tab (Minimal — superseded by Phase 10)
**Goal**: Wire up the Media tab with a basic thumbnail grid. Phase 10 replaces this with full MediaCarousel behavior (full-screen tap, drag-to-reorder).

**Modified files**:
- `app/lib/presentation/screens/gear/gear_content_view.dart`:
  - Implement Media tab pane:
    - **Read mode**: 2-column grid of media thumbnails (glass cards with image + label). Tapping a thumbnail changes the background image to that media item.
    - **Edit mode**: Thumbnails get delete (x) buttons (red circle, top-right). Add (+) cell appears at end with dashed border. Tapping add opens MediaPickerHelper.
  - Remove `_showMediaCarousel()` method
  - Pass media count to `ContentTabBar` for Media badge
  - Selected media highlighted with thicker border
  - **Note**: Phase 10 adds full-screen image viewing on tap and long-press drag-to-reorder

- `app/lib/presentation/viewmodels/gear_view_model.dart`:
  - Add `selectedMediaIndex` to state
  - `setSelectedMedia(int index)` updates background
  - Media add/delete/reorder methods adapted for inline grid (no full-screen carousel)

**Tests**: ~15 tests for Media tab grid rendering, selection, edit mode add/delete

---

### Phase 5: Gear Polish & Cleanup
**Goal**: Remove dead code, fix edge cases, verify all paths.

**Tasks**:
- Remove `ContentFloatingActions` import/usage from gear_content_view
- Remove the 43px right padding that cleared FABs
- Volume toggle: add small overlay button on background media (bottom-right, above tabs — per Decision #5)
- Verify feed context works correctly (feed header + tabs, no owner row)
- Verify community switching works with tabs
- Verify giveaway workflow modal still works from Type tab action button
- Verify loan request workflow still works from Type tab action button
- Clean up any dead methods in GearViewModel
- Update `docs/client/content.md` to reflect new architecture

**Tests**: ~10 integration-style tests verifying full tab flow

---

### Phase 6: Experience Content View Migration
**Goal**: Apply the same tabbed pattern to ExperienceContentView.

**Tab configuration**:
| Tab | Label | Content |
|-----|-------|---------|
| Event | "Event" | Title, description, date/time, RSVP button |
| Details | "Details" | Metrics (Going, Maybe, Duration) + details (date, time, location, host, posted) |
| Chat | "Chat" + badge | Inline conversation |
| Media | "Media" + count | 2-column thumbnail grid |

**Modified files**:
- `app/lib/presentation/screens/experience/experience_content_view.dart` — same structural changes as gear
- `app/lib/presentation/viewmodels/experience_view_model.dart` — add `activeTab`, remove FAB methods

**Reuses**: ContentTabBar, ContentEditBar, ContentOwnerRow, InlineConversationView (all from Phases 0-3)

**Tests**: ~20 tests for experience-specific tab content

---

### Phase 7: Request Content View Migration
**Goal**: Apply the same tabbed pattern to RequestContentView.

**Tab configuration**:
| Tab | Label | Content |
|-----|-------|---------|
| Request | "Request" | Title, description, location, "Offer to Help" button |
| Details | "Details" | Metrics (Comments, Helpers, Open days) + details (type, location, opened, duration, listed) |
| Chat | "Chat" + badge | Inline conversation |
| Media | "Media" + count | Single photo or 2-column grid |

**Modified files**:
- `app/lib/presentation/screens/request/request_content_view.dart` — same structural changes
- `app/lib/presentation/viewmodels/request_view_model.dart` — add `activeTab`, remove FAB methods

**Reuses**: All shared widgets from Phases 0-3

**Tests**: ~20 tests for request-specific tab content

---

### Phase 8: Final Cleanup
**Goal**: Remove dead shared code, update documentation.

**Tasks**:
- If `ContentFloatingActions` is no longer used by any content view, delete it and its tests
- If `CommentBar` is no longer used, evaluate deletion (may still be used elsewhere)
- Update `docs/client/content.md` with new architecture description
- Verify all existing test suites pass
- Run `flutter analyze` — zero issues
- Run `npm run build` — server still compiles (no server changes expected)

---

### Phase 9: Full Conversation Experience in Chat Tab
**Goal**: Replace the minimal `InlineConversationView` (Phase 3) with a chat experience identical to `ConversationScreen`, reusing the same widgets and rendering pipeline. The Chat tab should feel like the full conversation screen embedded directly under the tab bar — not a simplified preview.

**NOT YET IMPLEMENTED** — this phase is planned but not started.

#### What changes from the Phase 3 placeholder

Phase 3 builds a simple message list + text input. Phase 9 replaces that with the full `ConversationScreen` feature set:

| Feature | Phase 3 (placeholder) | Phase 9 (full) |
|---------|----------------------|----------------|
| Message bubbles | Simple glass bubbles | Exact `MessageList` rendering: solid white bubbles, sender name + timeago, delivery receipt icons |
| Avatars | Basic circle | Radius-16 avatars with **participant badges** (RSVP status, "Helping", "Interested", "Selected") from `ConversationViewModel` |
| System messages | Not rendered | Full `SystemPill` widgets — color-coded (sage/coral/neutral), collapsible, with inline widget card expansion |
| Widget cards | Not rendered | All workflow cards via `WorkflowWidgetFactory`: RSVP invite, time voting, attendance, completion, request info, gear info, giveaway selection |
| Input area | Plain text field + send button | Full `MessageInputBar`: plus button for image upload, mention-capable text field, send button, terminal label for completed workflows |
| Progress bar | Not rendered | Phase progress bar from `MessageInputBar`: colored segments (sage=done, coral=current, grey=future) |
| Action row | Not rendered | `MorphingActionButton` (coral CTA → sage confirmed → muted terminal) + overlapping participant avatar row |
| Media in messages | Not rendered | Inline media attachments (200px height, rounded corners, tap for full-screen lightbox) |
| Optimistic sending | Not implemented | Pending → delivered → failed states with delivery receipt icons |
| Auto-scroll | Basic | Scroll to bottom on new messages, 300ms delay after keyboard appearance |
| Background | Semi-transparent tab area | **No additional semi-transparent layer** — the existing tab content area provides the dark overlay; chat content renders directly on it |

#### Architecture approach

**Reuse, don't rebuild.** The existing `MessageList`, `MessageInputBar`, `SystemPill`, `WorkflowWidgetFactory`, and `MorphingActionButton` widgets are already well-tested and feature-complete. The goal is to embed them in the Chat tab's content pane rather than rebuilding from scratch.

**Key extraction**: `ConversationScreen` currently owns the full Stack layout (background + message list + floating top bar + floating input bar). For Phase 9, the Chat tab needs only the **message list** and **input bar** — the background and top bar are already provided by the content view's tab layout.

**Approach options (to evaluate during implementation)**:

1. **Direct widget reuse**: Render `MessageList` and `MessageInputBar` directly in the Chat tab pane. The Chat tab pane becomes a `Column` with `Expanded(child: MessageList(...))` + `MessageInputBar(...)`. This requires that `MessageList` and `MessageInputBar` accept all their data as parameters (they already do — they're presentation widgets, not state-owning).

2. **Extract `ConversationBody` widget**: If shared state wiring is complex, extract the inner body of `ConversationScreen` (everything except the Stack, background, and top bar) into a reusable `ConversationBody` widget that both `ConversationScreen` and the Chat tab can use.

#### Dynamic height behavior

- **Few or no messages**: Keep the tab content area at its default height (same as other tabs). The message list fills available space with empty area above messages.
- **Many messages**: Expand the tab content area so the tab bar moves near the top of the screen, giving the conversation maximum vertical space. This is an animated transition, not a jump.
- **Keyboard open**: Tab content area expands to near-top regardless of message count (keyboard needs space). Input bar rides above the keyboard via `viewInsets.bottom`.

#### What NOT to include

- **ConversationScreen's background layer** (blurred image + tint overlay + gradient) — the tab's existing semi-transparent dark area replaces this. The message list renders on the tab's glass background directly.
- **ConversationTopBar** (floating title + back button) — the content view's owner row and tab bar already provide navigation context.
- **ConversationScreen's navigation logic** (push to item screen on title tap, swipe-to-close) — not applicable when embedded in a tab.

#### State management

- The Chat tab reads from `conversationProvider(conversationId)` via `ref.watch()`, same as Phase 3. The `ConversationNotifier` already manages message streaming, optimistic sends, workflow state, RSVP maps, and participant info.
- `MessageInputBar` callbacks (`onSendMessage`, `onPickImage`, etc.) delegate to `ConversationNotifier` methods through the ViewModel layer.
- Widget cards read live workflow state from the same `ConversationNotifier` — no additional state management needed.

#### Files likely modified

- `app/lib/presentation/widgets/content/inline_conversation_view.dart` — **rewritten** to use `MessageList` + `MessageInputBar` instead of custom simple rendering
- `app/lib/presentation/screens/gear/gear_content_view.dart` — Chat tab pane updated for dynamic height behavior
- `app/lib/presentation/viewmodels/gear_view_model.dart` — expose `chatExpanded` state for height animation
- Possibly `conversation_screen.dart` — if extracting `ConversationBody`, the screen becomes a thin wrapper around it

#### i18n

No new ARB keys expected — all strings already exist in the conversation widgets being reused.

#### Tests

- Verify `MessageList` renders correctly in tab context (no background layer)
- Verify `MessageInputBar` progress bar, action row, and input all render
- Verify widget cards appear at correct system message anchor points
- Verify dynamic height transitions (few messages → many messages → keyboard)
- Verify image upload flow works from tab context
- Verify terminal label appears for completed workflows

---

### Phase 10: Full Media Experience in Details Tab
**Goal**: Replace the Phase 4 minimal thumbnail grid with full `MediaCarousel`-equivalent behavior. Media thumbnails now live under the Details tab (below the metric tiles and detail rows). Tapping a thumbnail opens the full-screen image above the content. Edit mode supports add, delete, and long-press drag-to-reorder.

**NOT YET IMPLEMENTED** — this phase is planned but not started.

#### What changes from the Phase 4 placeholder

Phase 4 builds a basic 2-column thumbnail grid with tap-to-select-background and edit-mode add/delete. Phase 10 replaces that with the full `MediaCarousel` feature set:

| Feature | Phase 4 (placeholder) | Phase 10 (full) |
|---------|----------------------|-----------------|
| Tap thumbnail (read mode) | Changes background image | Opens **full-screen image viewer** above the content (same as `MediaCarousel` page view with `PhotoView` pinch-to-zoom) |
| Tap thumbnail (edit mode) | No action | No action (edit mode is for reordering/adding/deleting) |
| Background image selection | Tap thumbnail → background changes | First media item is always the background; reordering changes which item is first |
| Add media (edit mode) | (+) cell opens `MediaPickerHelper` | Same, but new media is **appended** to the end (does NOT become the background image) |
| Delete media (edit mode) | (x) button on thumbnail | Same behavior as `MediaCarousel.onDeleteMedia` with confirmation |
| Reorder media (edit mode) | Not supported | **Long-press and drag** thumbnails to reorder; calls `onReorderMedia` with updated ID list; moving an item to position 0 makes it the background |
| Video thumbnails | Static thumbnail image | Thumbnail with **play icon overlay** (matching `MediaCarousel` video detection via `contentType`) |
| Video playback | Not supported in grid | Tapping a video thumbnail opens full-screen video player (same as `MediaCarousel` per-page `VideoPlayerController`) |
| Thumbnail images | `CachedMediaImage` with thumbnail URL | Same — `MediaRepository.getMediaUrl()` for thumbnails, `MediaRepository.getFullMediaUrl()` for full-screen viewer |
| Empty state | No special handling | Show a hint text and add button when no media exists |

#### Where thumbnails live

Media thumbnails move from being a standalone Media tab (Phase 4) to being a section **within the Details tab**, below the metric tiles and key-value detail rows. This consolidates item metadata and media into a single scrollable view.

The tab bar changes from 4 tabs to 3 tabs:

| Tab | Label | Content |
|-----|-------|---------|
| **Type** (e.g., "Loan") | Dynamic | Title, description, location, action button + share |
| **Details** | "Details" | Metric tiles + detail rows + **media thumbnails** |
| **Chat** | "Chat" + badge | Full conversation (Phase 9) |

The "Media" tab is removed. The media count badge moves to the Details tab (or is dropped if the Details tab already shows enough context).

#### Full-screen image viewer behavior

When a user taps a thumbnail in read mode, a full-screen overlay appears **above** the content view (not a navigation push — stays in context). This overlay reuses the `MediaCarousel` viewer behavior:

- **`PhotoView`** with pinch-to-zoom (`minScale: contained`, `maxScale: covered * 3`)
- **Horizontal swipe** between media items (same `PageView` pattern as `MediaCarousel`)
- **Close** by tapping the background or a close button
- **Video items** show the video player instead of `PhotoView`
- **Stable cache keys** via `ImageCacheKeys.full(mediaId)` for the full-resolution image
- **Thumbnail strip** at bottom of overlay showing all media with active item highlighted (matching `MediaCarousel`'s thumbnail row)

#### Drag-to-reorder in edit mode

When in edit mode, thumbnails become reorderable via long-press drag:

- Use Flutter's `ReorderableGridView` or a custom `LongPressDraggable` + `DragTarget` implementation on the 2-column grid
- **Long press** initiates drag (standard Flutter gesture — no conflict with tap)
- **Visual feedback**: dragged item scales up slightly with a drop shadow; placeholder gap shows where it will land
- **On drop**: calls `GearViewModel.reorderMedia(List<String> newMediaIds)` which updates the order on the server via `GearRepository`
- **Background updates**: if an item is dragged to position 0, it becomes the new background image (position 0 = primary media)
- **Append on add**: new photos/videos added via the (+) button are appended to the end of the list, preserving the current background image

#### Architecture approach

**Reuse `MediaCarousel` internals.** The full-screen viewer should reuse `MediaCarousel`'s `PhotoView` rendering, video player setup (`createCachedVideoController`), and thumbnail strip. Options:

1. **`MediaCarousel.show()` directly**: Call the existing static method when a thumbnail is tapped. This already handles `PhotoView`, video, pinch-to-zoom, and the thumbnail strip. The only difference is that it currently uses `NavigationHelpers.pushScreen()` — it may need an option to show as an overlay instead.

2. **Extract viewer widget**: If `MediaCarousel.show()` is too tightly coupled to navigation, extract the viewer portion (PageView + PhotoView + thumbnail strip) into a `MediaViewer` widget that can be rendered as an overlay within the content view's Stack.

For drag-to-reorder, the existing `MediaCarousel.onReorderMedia` callback pattern is reused — same signature (`Future<void> Function(List<String> mediaIds)`), same server-side behavior.

#### Media URL strategy

Per `docs/client/media.md`:
- **Thumbnail grid**: Use `MediaRepository.getMediaUrl()` — returns thumbnail URLs (400px) for efficient loading
- **Full-screen viewer**: Use `MediaRepository.getFullMediaUrl()` — returns full-resolution URLs for quality
- **Video detection**: Use `mediaUrl.contentType?.startsWith('video/')` (not URL heuristics)
- **Cache keys**: `ImageCacheKeys.thumbnail(mediaId)` for grid, `ImageCacheKeys.full(mediaId)` for viewer
- **Video caching**: Use `createCachedVideoController()` via `MediaRepository.getVideoFile()` — never `VideoPlayerController.networkUrl()` directly

#### Files likely modified

- `app/lib/presentation/screens/gear/gear_content_view.dart` — Details tab pane gains media thumbnail section; Media tab removed; full-screen overlay added to Stack
- `app/lib/presentation/viewmodels/gear_view_model.dart` — `reorderMedia()` method; `selectedMediaIndex` for full-screen viewer; remove media tab index references
- `app/lib/presentation/widgets/content/content_tab_bar.dart` — support 3-tab configuration (no longer always 4)
- Possibly `app/lib/presentation/widgets/media/media_carousel.dart` — extract `MediaViewer` if reuse requires it

#### i18n

Add ARB keys for empty-state hint text (e.g., `gearDetailsNoMedia`, `gearDetailsAddMedia`). Existing media-related strings from `MediaCarousel` are already localized.

#### Tests

- Verify thumbnail grid renders under Details tab with correct images and count
- Verify tapping thumbnail opens full-screen viewer with `PhotoView`
- Verify horizontal swipe between images in viewer
- Verify video thumbnails show play icon overlay
- Verify edit mode: add appends to end (not position 0)
- Verify edit mode: long-press drag reorders thumbnails
- Verify edit mode: drag to position 0 updates background
- Verify edit mode: delete removes thumbnail and calls repository
- Verify 3-tab configuration (no Media tab)
- Verify media URLs use correct thumbnail/full strategy per context

---

## File Impact Summary

### New Files
| File | Phase | Purpose |
|------|-------|---------|
| `content_tab_bar.dart` | 0 | Shared pill-tab bar widget |
| `content_edit_bar.dart` | 0 | Cancel / Editing / Save top bar |
| `content_owner_row.dart` | 1 | Owner avatar + name + overflow button |
| `inline_conversation_view.dart` | 3, 9 | Phase 3: simple chat; Phase 9: full ConversationScreen experience |
| `media_viewer.dart` (if extracted) | 10 | Reusable full-screen PhotoView + video viewer from MediaCarousel |

### Major Modifications
| File | Phases | Changes |
|------|--------|---------|
| `gear_content_view.dart` | 1-5, 9-10 | Full rewrite: FABs → tabs, inline all modals, full chat + media |
| `gear_view_model.dart` | 1-5, 9-10 | Add tab state, remove FAB methods, reorder media |
| `experience_content_view.dart` | 6 | Same tabbed migration |
| `experience_view_model.dart` | 6 | Add tab state, remove FAB methods |
| `request_content_view.dart` | 7 | Same tabbed migration |
| `request_view_model.dart` | 7 | Add tab state, remove FAB methods |

### Potentially Deleted Files
| File | Phase | Condition |
|------|-------|-----------|
| `content_floating_actions.dart` | 8 | If no remaining consumers |

### Unchanged Files
- All proto definitions (no API changes)
- All server code (no backend changes)
- All repositories and services (data layer unchanged)
- `content_view_builders.dart`, `content_view_helpers.dart` (still used)
- `content_editing_mixin.dart` (still used)
- `media_background.dart`, `content_gradient_overlay.dart` (still used)
- `content_overflow_menu.dart` (still used, triggered from owner row)

---

**Last Updated:** 2026-03-25
