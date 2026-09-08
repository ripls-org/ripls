# Adaptive layout widgets

Widgets that make screens read as designed on desktop-wide windows (#2912)
while leaving phone layouts untouched by construction.

The house rule is **constraint-first, breakpoint-rare** (see
`docs/client/responsive.md`): prefer width caps that simply never bind on
phones over window classification, and reserve the one structural breakpoint
(`Responsive.expandedBreakpoint`) for screens that genuinely change shape.
Shared constants live in `core/utils/responsive.dart`.

## Key files

- `content_column.dart` — `ContentColumn`, the measure primitive: centers its
  child at `Responsive.contentMaxWidth` (640, the *reading* measure) by
  default, or at whatever `maxWidth:` the surface passes. Wrap a stream
  screen's rows, a read shell's sheet, or any content that would otherwise
  stretch to the window. Surfaces laying out fixed-size tiles rather than
  prose pass `Responsive.galleryMaxWidth` instead — the Library does (#2926).
  It must never be load-bearing for fit — that is the #2908 height-budget
  work.
- `auth_screen_wrapper.dart` — `AuthScreenWrapper`, the older auth-only
  treatment: below `Responsive.mobileBreakpoint` it is a no-op; above it the
  auth flow renders as a centered 480px elevated `Card` over a soft gradient.

## When to add code here vs. adjacent directories

Add a widget here when it exists to adapt *composition to available width*
and is content-agnostic. Content-specific widgets belong with their feature
(e.g. `widgets/home/`, `widgets/content/`); accessibility primitives belong
in `widgets/accessibility/`. Note that bottom-sheet width capping does NOT
live here — it is applied centrally in
`widgets/accessibility/show_accessible_modal.dart`, and the `NavDock` caps
itself in `widgets/navigation/nav_dock.dart`.
