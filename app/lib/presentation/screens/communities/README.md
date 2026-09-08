# Community Screens

Screens and modals for viewing, editing, and managing communities.

## Purpose

These files form the community management surface: the main detail view, editing, member management, public discovery, and invite flows.

## Key Files

### Main views
- **`community_content_view.dart`** — the scrollable body of a community: media carousel, description, feed, and tab bar. Used inside the home screen shell when a community is selected.
- **`community_public_screen.dart`** — read-only community profile shown to non-members (from discover).

### Creation and editing
- **`community_creation_modal.dart`** — single community-creation modal: a glass card with a required name field and optional description, a streamed-in background image (replaceable), and a Create button.
- **`community_edit_screen.dart`** — full-screen editor for community name, description, and settings.

### Member management
- **`manage_members_screen.dart`** — admin screen for viewing, removing, and promoting members.
- **`invite_sheet.dart`** — bottom sheet for sharing the community invite link.

## When to add here vs. elsewhere

Community-specific screens and modals belong here. Reusable widgets that render community data (e.g., community avatar, community card) live in `widgets/sharing/`. Impact and metrics screens for communities live in `screens/impact_metrics/`.
