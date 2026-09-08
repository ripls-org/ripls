# Settings Widgets

Building blocks for settings screens: section headers, card containers, and preference pickers.

## Purpose

These components provide consistent structure and styling for the profile settings and app-preferences screens.

## Key Files

- **`settings_widgets.dart`** — `buildSettingsSectionHeader()` and `buildSettingsCard()` — the two core layout primitives shared by all settings screens. Every settings section uses a header above a card container.
- **`community_settings_row.dart`** — one community/group row in the settings hub. Derives its label and second line through `core/utils/community_display.dart` rather than reading `CommunityItem.name`, so a nameless (ad-hoc) community renders as its members instead of a blank row (#2937).
- **`language_picker.dart`** — bottom sheet for selecting the app display language.
- **`timezone_picker.dart`** — searchable bottom sheet for selecting a preferred IANA timezone.

## When to add here vs. elsewhere

Settings UI primitives belong here. The screens that use them (`profile_settings_screen.dart`, `manage_memberships_screen.dart`) live in `screens/profile/`. App-wide theme tokens are in `core/theme/`.
