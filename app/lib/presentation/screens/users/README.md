# User Screens

Screens for viewing another user's profile through the lens of communities the viewer and target share.

## Purpose

These screens are shown when the current user taps anyone's name or avatar — themselves included. They are distinct from the signed-in user's settings under `screens/profile/`.

## Key Files

- **`user_screen.dart`** — viewer-facing profile composed of an identity block (avatar + name + shared-community subtitle), Workshop-style hero and ticker rendering the target user's full aggregate activity, and a postcard section that links the viewer to actionable entities inside shared communities. Renders for any target including the viewer themselves; postcards are hidden when viewer == target (you can't borrow your own gear).

## When to add here vs. elsewhere

Screens showing another user's public data belong here. Settings and account management are in `screens/profile/`. Community-scoped impact screens are in `screens/impact_metrics/`.
