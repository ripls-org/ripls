# Discover Widgets

Components for the Library (discover) screen: shelf tiles, the location
anchor sheet, and the map/photo overlays.

## Purpose

The Library screen presents a map with nearby gear laid out on shelves.
These widgets compose the shelf content and the sheets that control what
it shows.

## Key Files

### Library shelves (#2634 v2)
- **`library_shelf_tile.dart`** — `LibraryShelfTile`, the 150×144 shelf item card (photo with bottom scrim and serif name; a Giveaway pill only for that state — borrowable is the default and wears none) and `LibraryShelfGhostTile`, the dashed add-in-context end-cap. Anatomy from `docs/cowork/App Design/bottom-nav-liquid-glass-mock-v3.html`.
- **`library_location_sheet.dart`** — bottom location sheet that picks the Library's distance anchor.

### Overlays
- **`discover_gallery_overlay.dart`** — full-screen image gallery overlay shown when a discover photo is tapped.
- **`map_style_sheet.dart`** — sheet for switching the map's base style.

## When to add here vs. elsewhere

Discover-specific widgets belong here. Generic list rows or cards reused across discover and other screens belong in the relevant feature widget directory.
