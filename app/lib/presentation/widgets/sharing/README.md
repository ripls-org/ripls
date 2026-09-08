# Sharing Widgets

Community picker and multi-community selection components used when sharing items.

## Purpose

When a user creates or updates a gear item, request, or experience, they choose which communities to share it with. These widgets provide the community selection UI for those flows.

## Key Files

- **`community_selection_sheet.dart`** — unified bottom sheet for picking communities. Three entry points:
  - `showForDeferred` — multi-select for item creation; returns selected IDs on Confirm.
  - `showForInvite` — single-select for picking which community to invite a user into; pops with the picked ID.
  - `showForImmediate` — multi-select for editing access on an existing item; toggles fire sharing-provider mutations immediately.
  Includes a sticky "Create new community" footer that opens `CommunityCreationModal` and refreshes `CommunityRepository` on success.
- **`community_selection_tile.dart`** — a single community row used inside `CommunitySelectionSheet`. Renders a `Toggle`/`Switch` for multi-select modes and a decorative radio indicator for invite mode.
- **`community_list_item.dart`** — a single community row with avatar and toggle, used by the immediate-mode flow.
- **`item_share_sheet.dart`** — bottom sheet for sharing a specific item to additional communities after it has been created.
- **`shared_with_card.dart`** — read-only card listing the communities an item is currently shared with.
- **`invite_members_sheet.dart`** — bottom sheet for inviting people into a community.

## When to add here vs. elsewhere

Community selection and sharing widgets belong here. Widgets that display community information without selection (community avatar, community card, leaderboard entries) live in their respective feature widget directories. Community management (member list, admin actions) is in `screens/communities/`.
