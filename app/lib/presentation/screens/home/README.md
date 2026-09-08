# Home Screens

The root shell of the authenticated app experience.

## Purpose

These files define the persistent scaffold that hosts all main tabs (discover, daily, feed, metrics) and the global sidebar navigation.

## Key Files

- **`home_screen.dart`** — top-level `ConsumerStatefulWidget` after login. Owns the bottom navigation bar, the `screenModalProvider` overlay layer, the global community selector, and coordinates creation modals (gear, request, experience) launched from the floating action button.
- **`sidebar.dart`** — slide-in navigation panel showing the current user's communities, profile link, and settings. Used by `HomeScreen`.

## When to add here vs. elsewhere

Only scaffolding and global navigation belong here. Feature screens displayed within the tabs (discover, portfolio, metrics) live in their own directories under `screens/`. Widgets used only inside the sidebar or home shell can live in these files directly; reusable widgets go in `widgets/`.
