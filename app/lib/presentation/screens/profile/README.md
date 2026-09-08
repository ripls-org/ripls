# Profile Screens

Screens for the current user's profile, settings, memberships, and account management.

## Purpose

These screens let the authenticated user view and edit their own profile, manage community memberships, configure saved locations, and adjust app preferences.

## Key Files

- **`profile_edit_screen.dart`** — edit display name, bio, and profile photo.
- **`profile_settings_screen.dart`** — app-level preferences: language, timezone, notifications, and account actions.
- **`profile_locations_screen.dart`** — manage the user's saved pickup/dropoff locations.
- **`manage_memberships_screen.dart`** — list and leave communities the user belongs to.
- **`delete_account_screen.dart`** — confirmation flow for permanent account deletion.

## When to add here vs. elsewhere

Settings and account management for the signed-in user belong here. Another user's profile (viewed from the community member list or a transfer) is in `screens/users/`. Community management performed by an admin is in `screens/communities/`.
