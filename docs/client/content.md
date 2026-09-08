---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client full-screen content views for gear, experience, request, and community — shared Stack scaffolding with background media, gradient overlay, bottom tab panel, and pane switcher.
  globs: [app/lib/presentation/widgets/content/**, app/lib/presentation/screens/gear/**, app/lib/presentation/screens/experience/**, app/lib/presentation/screens/request/**, app/lib/presentation/screens/communities/**]
  triggers: [content-view, tab-bar, pane-switcher, gradient-overlay, bottom-content, glass-panel]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "05161e43f"
  verified_on: "2026-06-30"
---
# Client Content Views Architecture

## Overview

The Ripls client displays four primary content types as full-screen "content views":

1. **Gear Content:** Equipment shared for loan or giveaway ([gear_content_view.dart](../app/lib/presentation/screens/gear/gear_content_view.dart))
2. **Experience Content:** Events organized by members ([experience_content_view.dart](../app/lib/presentation/screens/experience/experience_content_view.dart))
3. **Request Content:** Help requests posted by members ([request_content_view.dart](../app/lib/presentation/screens/request/request_content_view.dart))
4. **Community Content:** Communities themselves ([community_content_view.dart](../app/lib/presentation/screens/communities/community_content_view.dart))

All four share the same outer scaffolding — a full-bleed background, a dark
gradient overlay, and a bottom content panel — but **Gear (#2509), Experience
(#2278), and Request (#2293) now use the bottom-anchored "read shell" pattern**:
a non-scrolling editorial sheet of cards over the hero, where the
discussion/location/roster cards **morph-expand into full-screen panels**
(`openContentMorphPanel` + a per-id `*ContentExpandedProvider`) rather than
switching swipeable tabs. Owner management moved off the action button into the
top-bar `···` overflow (a flat `PollManageMenuSheet`). **Only Community still
uses the legacy `ContentViewTabBar` + `ContentViewPaneSwitcher`** described in
the [Tab Bar](#tab-bar) and [Bottom Content Panel](#bottom-content-panel)
sections below.

For the gear read shell specifically:
[gear_read_shell.dart](../app/lib/presentation/screens/gear/widgets/gear_read_shell.dart)
composes the title, a discussion card, a specs card, a
WHERE card, a "Shared with" card ([SharedWithCard](../app/lib/presentation/widgets/sharing/shared_with_card.dart) — audience surface, taps to the access sheet; see [sharing.md](sharing.md)), and a "Who's using it" card, with the workflow CTA as a sticky action
and
[gear_manage_menu_sheet.dart](../app/lib/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart)
behind the top-bar overflow. **Four cards morph-expand in place** (the "Shared with" card opens the access sheet rather than morphing; each of the four grows a
full-screen panel from its own footprint via `openContentMorphPanel` +
`gearContentExpandedProvider`):

- discussion → conversation ([gear_conversation_panel.dart](../app/lib/presentation/screens/gear/widgets/gear_conversation_panel.dart))
- WHERE → location panel ([gear_location_panel.dart](../app/lib/presentation/screens/gear/widgets/gear_location_panel.dart)) — single owner-set spot (map + directions + inline picker), no polling
- Who's using it → roster/story panel ([gear_whos_using_panel.dart](../app/lib/presentation/screens/gear/widgets/gear_whos_using_panel.dart)) — usage metrics, current holder + queue, **inline per-borrower workflow actions** (the `gear_menu_items.dart` checklist rendered via `ActionDropdownMenu`), and past borrowers
- specs → details panel ([gear_details_panel.dart](../app/lib/presentation/screens/gear/widgets/gear_details_panel.dart)) — self-contained `GearDetailsPanel`: full spec rows, an optional product-page link, and an "Impact so far" tiles row, with owner-inline metadata editing (not a reuse of `GearDetailsPane`)

The per-borrower workflow actions surface both inline in the who's-using panel and
on the sticky action button's dropdown (single source: `gear_menu_items.dart`).
The location picker modal is still used for the owner-management entry points
(manage sheet "Set location", owner menu, edit pane) that have no card to morph
from.

---

## Screen Stack Layout

Every content view is a `Stack` with the same layer order:

1. **Background media** — full-bleed photo or video ([VideoBackgroundHost](../app/lib/presentation/widgets/media/video_background_host.dart))
2. **Gradient overlay** — dark bottom-to-top gradient for text readability ([ContentGradientOverlay](../app/lib/presentation/widgets/content/content_gradient_overlay.dart))
3. **Bottom content panel** — view-specific `*BottomContent` widget housing the attribution/mute row, tab bar, and pane switcher (see [Bottom Content Panel](#bottom-content-panel))
4. **Edit bar** — Cancel / Editing / Save pills, only when `state.isEditing` is true ([ContentEditBar](../app/lib/presentation/widgets/content/content_edit_bar.dart))
5. **Feed header** — optional actor + timestamp overlay used when the view is rendered inside a feed ([ContentViewBuilders.buildFeedHeader()](../app/lib/presentation/widgets/content/content_view_builders.dart))
6. **Owner row** — Community only; avatar + name + subtitle row ([ContentOwnerRow](../app/lib/presentation/widgets/content/content_owner_row.dart))
7. **Media picker overlay** — positioned cover-photo replace button when `state.canEditCoverPhoto`
8. **Upload overlay** — progress indicator during media upload

**Note:** Gear, Experience, and Request do **not** instantiate `ContentOwnerRow`. They surface actor identity inside their primary tab pane (via `ContentTopRows` / `ContentInfoRow` patterns) rather than as a top overlay. Community is the only view that keeps the dedicated owner row.

---

## Bottom Content Panel

Each view delegates its bottom panel to a dedicated widget that composes the same three pieces:

| View | Bottom widget | Tabs |
|------|--------------|------|
| Gear | [GearReadShell](../app/lib/presentation/screens/gear/widgets/gear_read_shell.dart) (read shell, not tabs) | — (cards + morph panels) |
| Experience | [ExperienceBottomContent](../app/lib/presentation/screens/experience/widgets/experience_bottom_content.dart) | 2 — Event · Chat |
| Request | [RequestBottomContent](../app/lib/presentation/screens/request/widgets/request_bottom_content.dart) | 2 — Request · Discuss |
| Community | inlined in `community_content_view.dart` | 3 — Community · Members · Discuss |

Each panel is a `Positioned` container at the bottom of the stack, with:

1. **Attribution / mute row** — optional photo credit line on the left, optional `VideoMuteToggleButton` on the right. Rendered only when the background is a video, when an attribution line exists, or both.
2. **Tab bar** — [ContentViewTabBar](../app/lib/presentation/widgets/content/content_view_tab_bar.dart) (see [Tab Bar](#tab-bar)).
3. **Pane switcher** — [ContentViewPaneSwitcher](../app/lib/presentation/widgets/content/content_view_pane_switcher.dart) drives a sliding transition between the active tab's pane, with direction derived from parent-owned `slideDirection` state (+1 left-to-right, -1 right-to-left).

The Chat pane uses a clamped pixel height (~62% of screen, expanding to ~73% when the Chat tab is active). Other panes size to their natural height.

---

## Tab Bar

[ContentTabBar](../app/lib/presentation/widgets/content/content_tab_bar.dart) renders a horizontal row of pill-shaped tabs. A single sliding accent background pill animates between tab positions via `AnimatedPositioned`. Inactive pills use glass morphism (backdrop blur + semi-transparent fill).

[ContentViewTabBar](../app/lib/presentation/widgets/content/content_view_tab_bar.dart) is the wrapper used inside content views. It watches `unreadCountProvider` (keyed on `conversationId`) to compute live unread counts and forwards only presentation state to `ContentTabBar`. It accepts `firstTabLabel`, optional `secondTabLabel` (or `showSecondTab: false` to hide the middle tab), and `thirdTabLabel`, so 2-tab and 3-tab views share the same widget.

### Chat tab badge

The chat tab (always rightmost) shows:
- An accent dot when there are unread messages.
- A gray pill with message count when read.
- No badge in edit mode.

### Active pill color

The first tab accepts an optional `firstTabColor` override to distinguish owner / non-owner / action-taken states. Tabs after the first always render with the panel's `accentColor` (typically `AppColors.transferCoral`).

Per view, `_firstTabColor()` computes the override from ViewModel state:

- **Coral** — owner, or non-owner with action available
- **Sage** — non-owner who has taken action (offered, helping, RSVPed)
- **Null / muted** — terminal state (cancelled, unavailable)

Community does not override `firstTabColor`; it uses `AppColors.primary(context)` for all tabs.

---

## chatTabIndex

`chatTabIndex` tells the view where the chat tab lives. Experience and Request expose it as a getter on their ViewModel state; Gear and Community hardcode the chat tab at index `2`:

| View | `chatTabIndex` |
|------|---------------|
| Gear | n/a — read shell; chat is a morph panel grown from the discussion card |
| Experience | `1` (state getter) |
| Request | `1` (state getter) |
| Community | not exposed; chat is fixed at index `2` |

Experience and Request use `state.activeTab.clamp(0, state.chatTabIndex)` everywhere they read the active index. This lets a ViewModel collapse to fewer tabs in some states without the widget tree caring, and lets feed-entry code pass `initialTab` values aimed at a logical position (e.g. "open in chat") that get clamped to whatever the current state supports.

---

## Pane 0 — Type Tab (Loan / Event / Request / Community)

The first tab is content-type specific:

- **Gear:** no longer uses this legacy pane — Gear renders the bottom-anchored read shell ([gear_read_shell.dart](../app/lib/presentation/screens/gear/widgets/gear_read_shell.dart)) with morph panels instead (see [Overview](#overview)).
- **Experience:** [ExperienceEventPane] — title, description, time and location chips, polls, RSVP / Manage action. Title, description, and source URL are editable inline; time and location open modals.
- **Request:** [RequestRequestPane] — title, description, location, "I'll Help" / "Manage" action.
- **Community:** local `_buildReadPane()` in `community_content_view.dart` — community name, description, creator/timestamp row, and an "Invite" action button.

In read mode, descriptions render via [ContentExpandableDescription](../app/lib/presentation/widgets/content/content_shared_widgets.dart) — collapsed to 4 lines, tap to expand. In edit mode, title and description switch to inline editable fields backed by the [ContentEditingMixin](#contenteditingmixin) buffers.

### Action buttons

See [WorkflowActionButtons and MorphingActionButton](#workflowactionbuttons-and-morphingactionbutton) below. Community's action button is a plain `MorphingActionButton` labeled "Invite" — it does not participate in the workflow status morphing used by the other three.

---

## Gear specs + edit mode (no Details tab)

Gear no longer ships a dedicated Details tab. In **read mode** the gear-specific
metadata (brand, model, value, material, weight) renders as a compact specs
[ContentDetailCard](../app/lib/presentation/widgets/content/content_shared_widgets.dart)
in the read shell. In **edit mode** the shell is replaced by
[gear_edit_pane.dart](../app/lib/presentation/screens/gear/widgets/gear_edit_pane.dart)
(editable title + description) which embeds `GearDetailsPane` for the editable
metadata fields. Those fields **auto-save with an 800ms debounce** rather than
waiting for the global Save button; the debounced save is routed through
`GearNotifier.saveMetadataSilent` (the widget never calls the repository
directly). The media carousel lives in the conversation morph panel.

Experience and Request also fold their metadata inline (Experience exposes time /
location / polls inline; Request exposes location and metadata inline). Community
has a Members pane in this slot.

---

## Pane 1 (Community) — Members Tab

Community-only. Renders [CommunityMembersPane] — the community's member list/grid. No edit affordances.

---

## Chat / Discuss Tab (rightmost)

Each view embeds a conversation in the rightmost tab, via [InlineConversationView](../app/lib/presentation/widgets/content/inline_conversation_view.dart) or a view-specific chat pane that wraps it:

| View | Chat pane widget | Carousel inside chat pane? |
|------|------------------|----------------------------|
| Gear | `InlineConversationView` directly (inside the discussion morph panel, [gear_conversation_panel.dart](../app/lib/presentation/screens/gear/widgets/gear_conversation_panel.dart)) | Yes — carousel lives in the conversation panel |
| Experience | [ExperienceChatPane] | Yes — owner can reorder/delete/add |
| Request | [RequestChatPane] | Yes — owner can reorder/delete/add |
| Community | [CommunityChatPane] | Yes — any community member can add |

The view is lazy-initialized: the conversation is fetched only when the chat tab first becomes active. Once loaded, it stays alive via `AutomaticKeepAliveClientMixin` so switching tabs does not reload messages.

### Message list

Uses [MessageList](../app/lib/presentation/widgets/chat/message_list.dart): chat bubbles, participant avatars with workflow badges, and centered system-message rows. System messages render as [SystemMessageRow](../app/lib/presentation/widgets/chat/message_list/system_message_row.dart), with `TIME_PROPOSED` promoted to an inline [PollBannerOrLabel](../app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart) when the conversation has a live poll.

### Input bar

A glass pill input with an attach (image) button, a text field, and a circular send button. For terminal conversations (completed or cancelled), the input is replaced with a status label.

---

## Edit Mode

Edit mode is global to a content view — entering it enables editing across all tabs simultaneously. Save commits all changes; Cancel reverts them.

### ContentEditingMixin

[ContentEditingMixin](../app/lib/presentation/widgets/content/content_editing_mixin.dart) is a Dart mixin applied to all four content view `State` classes. It provides a simple string editing buffer for title and description — no `TextEditingController` objects. Initialization is idempotent, so re-renders during an active edit session do not overwrite the buffer.

Gear additionally mixes in `GearTransferHandlersMixin`; Experience mixes in `ExperienceCloseHandlersMixin`.

### ContentEditBar

[ContentEditBar](../app/lib/presentation/widgets/content/content_edit_bar.dart) is an overlay at the top of the screen during editing. Three equal-width pills: **Cancel** (glass), **Editing** (glass label, non-interactive), **Save** (accent). Save calls `_handleSaveChanges()`, which collects all editing buffers, invokes the ViewModel's save method, and toasts on success/failure.

### What is editable

| View | Editable in primary pane | Editable elsewhere |
|------|--------------------------|--------------------|
| Gear | Title, description, location | Brand, model, value, weight, website (edit pane `GearDetailsPane`, debounced auto-save); media carousel (conversation panel) |
| Experience | Title, description, source URL, time, location | Cover photo + media carousel |
| Request | Title, description, location | Cover photo + media carousel |
| Community | Title (name), description | Cover photo + media carousel |

---

## ContentMediaCarousel

[ContentMediaCarousel](../app/lib/presentation/widgets/content/content_shared_widgets.dart) renders a horizontal 94px-tall strip of 72×72 media thumbnails.

- **Read mode:** Tapping a thumbnail opens the full-screen [MediaCarousel.show()](../app/lib/presentation/widgets/media/media_carousel.dart) viewer. If the caller passes `showAddButton: true` (owners on Gear/Experience/Request; any community member on Community), an add (+) cell appears after the last thumbnail so media can be added without entering edit mode.
- **Edit mode:** Each thumbnail shows a red delete (×) badge. When `onReorderItems` is provided, thumbnails become long-press draggable via `ReorderableListView`. An add (+) cell always appears at the end.
- Uses [CachedMediaImage](../app/lib/presentation/widgets/chat/cached_media_image.dart) with stable cache keys (`thumbnail_$mediaId`) to avoid presigned URL invalidation.

The cover photo (background) is replaced separately, via the floating `_buildMediaPickerOverlay()` button shown when `state.canEditCoverPhoto` is true.

---

## MorphingActionButton

[MorphingActionButton](../app/lib/presentation/widgets/chat/morphing_action_button.dart) has four visual states:

| Status | Appearance | Interaction |
|--------|-----------|-------------|
| `preAction` | Coral CTA pill | Tapping calls `onAction` |
| `postAction` | Sage (or coral for owners) confirmed pill | Tapping opens the `menuItems` dropdown, or calls `onConfirmedTap` if empty |
| `terminal` | Muted gray, non-interactive | None — workflow was cancelled |
| `completed` | Green with ✓ checkmark, non-interactive | None — workflow completed successfully |

Two styling modes: **pill** (rounded corners, used in conversation bottom bars) and **flat** (no border radius, fills a segmented container — used in content view action rows).

Community uses a `MorphingActionButton` directly (not via a factory) for its Invite button — there is no terminal/completed state machinery.

### Status logic by content type

**Gear (giveaway):** Owner → always `postAction` (coral). Terminal states split: completed → `completed`, cancelled / unspecified → `terminal`. Non-owner with interest → `postAction` (sage); without → `preAction`.

**Request:** Fulfilled → `completed`. Cancelled → `terminal`. Owner with offers → `postAction` (coral). Owner without → `preAction`. Non-owner who has offered → `postAction` (sage); without → `preAction`.

**Experience:** Completed → `completed`. Cancelled → `terminal`. Owner → `postAction` (coral, "Manage"). Non-owner with RSVP → `postAction` (sage, "Going"/"Maybe"). Non-owner without → `preAction` ("RSVP").

---

## Owner vs. Non-Owner

The `state.isOwner` boolean (computed in the ViewModel from current user ID vs. item owner ID) controls:

- Whether the edit bar and save controls are shown
- The `firstTabColor` (coral for owners, computed dynamically for non-owners)
- The action button label and dropdown menu items
- Whether the add (+) button appears on the media carousel in read mode

Community uses the parent's `widget.canEdit` parameter for the same role — community editing permission is passed in rather than derived from a per-view ViewModel.

---

## Feed Integration

Content views are used directly inside the feed — there are no feed-specific wrapper files. [FeedScreen](../app/lib/presentation/screens/feed/feed_screen.dart) passes optional feed header parameters (`showFeedHeader`, `feedActor`, `feedOccurredAtUnixSec`, `feedActionText`) to content views, which display an actor/timestamp overlay at the top when in feed context.

The owner row (Community only) is hidden when a feed header is active. The tab bar and content pane remain fully functional in feed context, though horizontal swipe-between-tabs is disabled (tap-only) to avoid conflicting with the feed's vertical scroll gesture.

**Nav locking:** when in edit mode or on the chat tab, the content view calls `homeProvider.notifier.lockNav()` so the bottom nav bar is hidden. The chat-tab check uses `state.chatTabIndex` so it works regardless of total tab count.

---

## Community Context

When a Gear / Experience / Request view opens, the conversation thread, action buttons, and transfer context are scoped to a specific community. Three mechanisms resolve which community to use:

1. **`initialCommunityId` parameter** — explicit ID from the caller (e.g. portfolio inbox). Takes priority.
2. **`selectedCommunityProvider`** — the global sidebar community.
3. **First shared community** — fallback if neither of the above resolves.

The resolved community is what the ViewModel's `switchCommunity()` changes: it reloads the view in the new community context without navigating away and without touching the global sidebar. This does not apply to the Community content view itself — Community already _is_ the community context.

---

## State Management

All four views use Riverpod 3.0 with Freezed state classes. Gear, Experience, and Request each use a single unified provider. Community is the exception:

| View | Provider(s) |
|------|------------|
| Gear | `gearProvider(gearId)` |
| Experience | `experienceProvider(experienceId)` |
| Request | `requestProvider(requestId)` |
| Community | `communityEditProvider(communityId)` (background, editing, media) **+** `communityContentProvider` (active tab, members, conversation) |

Common state fields across the unified providers:

- `isLoading`, `isEditing`, `isSaving`, `errorMessage`
- `activeTab`, `chatTabIndex` — chat-tab index is exposed on state so widgets and feed entry points stay tab-count-agnostic
- `selectedMediaIndex`, `allMediaItems` — for the carousel
- `mediaId`, `isVideo`, `isMuted`, `canEditCoverPhoto` — background media controls
- Content-specific details (`gearDetails`, `experienceDetails`, `requestDetails`)
- `transferContext` / RSVP state / offer state for action button logic
- `isOwner`, `currentUserId`, `communityId`

**ViewModel files:**
- [gear_view_model.dart](../app/lib/presentation/viewmodels/gear_view_model.dart)
- [experience_view_model.dart](../app/lib/presentation/viewmodels/experience_view_model.dart)
- [request_view_model.dart](../app/lib/presentation/viewmodels/request_view_model.dart)
- [community_content_view_model.dart](../app/lib/presentation/viewmodels/community_content_view_model.dart)

---

## Content Type Differences

| Feature | Gear | Experience | Request | Community |
|---------|------|------------|---------|-----------|
| Tab count | — (read shell + morph panels) | 2 | 2 | 3 |
| `chatTabIndex` | n/a (chat is a morph panel) | 1 | 1 | 2 (fixed) |
| Tab labels | — (cards: discussion · specs · WHERE · who's-using) | Event · Chat | Request · Discuss | Community · Members · Discuss |
| Action button | Request/Manage (loan) or Interest/Managing (giveaway) | RSVP / Manage | I'll Help / Manage | Invite |
| Owner row at top | No | No | No | Yes |
| Media carousel lives in | Conversation panel | Chat pane | Chat pane | Chat pane |
| Carousel-add permission | Owner | Owner | Owner | Any community member |
| Provider count | 1 | 1 | 1 | 2 (edit + content) |
| Workflow status morphing | Yes | Yes | Yes | No |

---

## Shared Components Reference

| Component | File | Purpose |
|-----------|------|---------|
| `ContentTabBar` | [content_tab_bar.dart](../app/lib/presentation/widgets/content/content_tab_bar.dart) | Sliding pill tab implementation |
| `ContentViewTabBar` | [content_view_tab_bar.dart](../app/lib/presentation/widgets/content/content_view_tab_bar.dart) | Tab bar wrapper; watches unread count provider; supports 2- or 3-tab layouts |
| `ContentViewPaneSwitcher` | [content_view_pane_switcher.dart](../app/lib/presentation/widgets/content/content_view_pane_switcher.dart) | Slide-animated pane transitions with parent-owned slide direction |
| `ContentEditBar` | [content_edit_bar.dart](../app/lib/presentation/widgets/content/content_edit_bar.dart) | Cancel / Editing / Save top overlay |
| `ContentOwnerRow` | [content_owner_row.dart](../app/lib/presentation/widgets/content/content_owner_row.dart) | Owner avatar + name + subtitle (Community only) |
| `ContentEditingMixin` | [content_editing_mixin.dart](../app/lib/presentation/widgets/content/content_editing_mixin.dart) | String editing buffer for title/description |
| `ContentViewBuilders` | [content_view_builders.dart](../app/lib/presentation/widgets/content/content_view_builders.dart) | Pure static UI builder functions (feed header, etc.) |
| `ContentViewHelpers` | [content_view_helpers.dart](../app/lib/presentation/widgets/content/content_view_helpers.dart) | UI interaction helpers (dialogs, toasts, navigation) |
| `ContentGradientOverlay` | [content_gradient_overlay.dart](../app/lib/presentation/widgets/content/content_gradient_overlay.dart) | Dark gradient over background media |
| `ContentTopRows` | [content_top_rows.dart](../app/lib/presentation/widgets/content/content_top_rows.dart) | Title + description + actor info-rows used inside the primary pane |
| `ContentExpandableDescription` | [content_shared_widgets.dart](../app/lib/presentation/widgets/content/content_shared_widgets.dart) | Tap-to-expand description text |
| `ContentDetailRows` | [content_shared_widgets.dart](../app/lib/presentation/widgets/content/content_shared_widgets.dart) | Key/value detail rows with dividers |
| `ContentMediaCarousel` | [content_shared_widgets.dart](../app/lib/presentation/widgets/content/content_shared_widgets.dart) | Horizontal media thumbnail strip |
| `ContentAccessPill` | [content_access_pill.dart](../app/lib/presentation/widgets/content/content_access_pill.dart) | Trailing tab-bar affordance showing "{count} Invites"; opens the access sheet |
| `InlineConversationView` | [inline_conversation_view.dart](../app/lib/presentation/widgets/content/inline_conversation_view.dart) | Full conversation experience embedded in chat tab |
| `MorphingActionButton` | [morphing_action_button.dart](../app/lib/presentation/widgets/chat/morphing_action_button.dart) | Coral → sage → completed morphing button |
| `VideoBackgroundHost` | [video_background_host.dart](../app/lib/presentation/widgets/media/video_background_host.dart) | Full-bleed background photo or video with shared player lifecycle |
| `MediaCarousel` | [media_carousel.dart](../app/lib/presentation/widgets/media/media_carousel.dart) | Full-screen photo/video viewer with swipe and pinch-to-zoom |

---

## Content View Files

| File | Purpose |
|------|---------|
| [gear_content_view.dart](../app/lib/presentation/screens/gear/gear_content_view.dart) | Gear content — loan and giveaway workflows (read shell + morph panels) |
| [experience_content_view.dart](../app/lib/presentation/screens/experience/experience_content_view.dart) | Experience content — RSVP, time voting, attendance (2 tabs) |
| [request_content_view.dart](../app/lib/presentation/screens/request/request_content_view.dart) | Request content — offer to help, fulfillment (2 tabs) |
| [community_content_view.dart](../app/lib/presentation/screens/communities/community_content_view.dart) | Community content — name, members, discussion (3 tabs) |

---

**Last Updated:** 2026-05-31