# Presentation Providers

Riverpod providers that are scoped to the presentation layer and do not belong to a specific viewmodel.

## Purpose

These providers manage cross-cutting UI concerns — modal state, user preferences — that multiple screens or widgets need to read or write independently of any single screen's viewmodel.

## Key Files

- **`screen_modal_provider.dart`** — `screenModalProvider` manages which modal widget is currently displayed in the overlay layer above the bottom navigation bar. `HomeScreen` renders whatever widget this provider holds. Use it when a child widget needs to display a modal that must float above the nav bar.
- **`user_timezone_provider.dart`** — `userTimezoneProvider` exposes the current user's preferred IANA timezone. `resolvedTimezoneProvider` falls back to the device timezone when no preference is set. Consume this provider instead of reading `tz.local.name` directly.

## When to add here vs. elsewhere

Add a provider here when it is presentation-scoped (no server I/O beyond reading cached repository data) and needed by more than one screen or widget. Providers that are tightly coupled to a single screen belong in that screen's file or its viewmodel file.
