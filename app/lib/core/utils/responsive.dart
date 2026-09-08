import 'dart:math' as math;

import 'package:flutter/material.dart';

/// Responsive breakpoint and measure constants for adaptive layouts (#2912).
///
/// The house rule is **constraint-first, breakpoint-rare**: derive layout from
/// the constraints a widget is actually given (`LayoutBuilder`,
/// `ConstrainedBox`) rather than from window classification. The measures
/// below bound content width wherever a stream, sheet, or dock would
/// otherwise stretch to a desktop-wide window — each is a no-op on any window
/// narrower than the cap, so phone layouts are untouched by construction.
/// [expandedBreakpoint] is the one structural threshold, consumed via
/// `LayoutBuilder` constraints where a screen genuinely changes shape (the
/// Plans calendar's two-pane arm).
class Responsive {
  Responsive._();

  /// Below this width the screen is phone-sized ([isMobile]).
  static const double mobileBreakpoint = 600;

  /// At or above this available width a screen may switch structure (e.g.
  /// the Plans calendar's side-by-side arm). Material's expanded
  /// window-size-class boundary.
  static const double expandedBreakpoint = 840;

  /// Reading measure for stream and text surfaces — the Home/People columns
  /// and the item routes' caption column. Consumed via `ContentColumn`
  /// (widgets/adaptive/).
  static const double contentMaxWidth = 640;

  /// Measure for *gallery* surfaces — ones laying out fixed-size tiles rather
  /// than prose, where line length is not what bounds the width. The Library's
  /// category shelves are the case (#2926): held at the reading measure they
  /// left a 420px void down the left of a 1440 window while tiles ran off the
  /// right. Wider than [contentMaxWidth] on purpose; still a cap, so an
  /// ultrawide monitor doesn't get a 2500px row of 150px tiles.
  static const double galleryMaxWidth = 1200;

  /// The standard edge inset for content that is not itself width-capped.
  static const double baseInset = 20;

  /// The leading inset that lines a full-width surface's *content* up with a
  /// [measure]-wide centered column, without constraining the surface itself.
  ///
  /// The Library's shelves need exactly this: the horizontal `ListView` must
  /// keep a window-wide RenderBox (bounding it clips tiles mid-drag, and
  /// scrolling items off the left edge is a wanted affordance — #2926), so the
  /// column alignment has to live in the scroll padding instead of in a
  /// `ConstrainedBox`. Below [measure] this is just [inset], which is what
  /// makes it a no-op on phones.
  static double measureInset(
    double availableWidth, {
    double measure = contentMaxWidth,
    double inset = baseInset,
  }) {
    return math.max(inset, (availableWidth - measure) / 2 + inset);
  }

  /// Width cap for bottom sheets; applied centrally in `showAccessibleModal`.
  static const double sheetMaxWidth = 560;

  /// Width cap for the floating `NavDock` capsule.
  static const double dockMaxWidth = 500;

  /// Returns true if the screen width is mobile-sized (< 600px).
  static bool isMobile(BuildContext context) {
    return MediaQuery.sizeOf(context).width < mobileBreakpoint;
  }
}
