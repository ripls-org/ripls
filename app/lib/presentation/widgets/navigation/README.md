# navigation

The converged bottom dock (#2634): the app's primary navigation chrome.

## Purpose

Renders the floating glass capsule with the four place tabs
(Home · Plans · Library · People) and the universal-search cap tucked
into the capsule's right end behind a hairline divider, plus Create as
the single detached filled orb (the V2 "trailing search cap" layout from
the dock-variations mock). The dock is a floating overlay owned by
`HomeScreen` — it is not a `Scaffold.bottomNavigationBar`, so tab bodies
must reserve `NavDock.bottomContentInset` of scroll clearance.

## Key files / types

- `nav_destination.dart` — `RiplsNavDestination`, the enum of the four
  place destinations. Each carries its icons, l10n label, and its fixed
  index in the home shell's `IndexedStack` (`stackIndex`). Orphaned stack
  entries (the Feed at 0, legacy flag-off tabs) intentionally have no
  destination.
- `nav_dock.dart` — `NavDock`, the capsule (tabs + search cap) and the
  detached Create orb. Composes `GlassSurface` for the frosted material
  and swaps to a solid fill at identical geometry when
  `MediaQuery.highContrast` is on (the reduce-transparency fallback).

## When to add code here vs. adjacent directories

Add code here only for primary-navigation chrome (the dock itself, its
targets, its badges). Screen-level headers live with their screens;
glass modal surfaces live in `../modal/glass/`; the legacy pre-dock pill
still lives inline in `screens/home/home_screen.dart` behind the
`navDockEnabled` flag-off path.
