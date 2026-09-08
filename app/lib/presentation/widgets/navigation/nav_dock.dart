import 'dart:math' as math;
import 'dart:ui' show lerpDouble;

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';
import 'package:ripls/presentation/widgets/unread_badge.dart';

/// The converged bottom dock (#2634), restyled to the "liquid glass,
/// light" variation (NavVariations-v3 · 1): one frosted white pill with a
/// specular edge holding the four place tabs and the Create circle. Tabs
/// are icon-only; the selected tab expands into a lighter inner pill that
/// carries the only visible label. There is no search in the bar — search
/// lives in the destination header, beside the avatar chip.
///
/// Changing tabs slides the inner pill (the selection "bubble") from the
/// old tab to the new one. The dock's horizontal layout is fully computed
/// (label widths are measured with a TextPainter rather than laid out
/// intrinsically) so slot widths, the bubble's rect, the label reveal, and
/// icon tint can all interpolate in lockstep from a single animation —
/// the bubble stays pixel-aligned with its tab at rest. Reduce-motion
/// collapses the slide to an instant snap via [accessibleDuration].
///
/// The dock is an overlay, not a Scaffold.bottomNavigationBar, so tab
/// bodies must keep [NavDock.bottomContentInset] of scroll clearance.
///
/// The bar commits to the light glass material in both themes (the
/// blur keeps it legible over any content). Rendering degrades to a
/// solid, blur-free surface at identical geometry when the platform
/// reports high-contrast/reduced-transparency, keeping contrast above
/// the glass floor without moving any touch target.
class NavDock extends StatefulWidget {
  /// Capsule height. Every target inside spans this full height, so each
  /// touch target is at least 48dp tall regardless of icon size.
  static const double dockHeight = 58;

  /// Scroll-view bottom inset that keeps the last row clear of the dock:
  /// dock height + breathing room (see the build spec in the #2634 mock).
  static const double bottomContentInset = dockHeight + 12;

  /// Key on the sliding selection bubble, for widget tests that assert on
  /// its travel between tabs.
  @visibleForTesting
  static const Key selectionPillKey = Key('nav-dock-selection-pill');

  /// The IndexedStack index whose tab renders selected. Indices that no
  /// destination maps to (e.g. the orphaned Feed) render with no selection.
  final int selectedStackIndex;

  /// Needs-you count badged on the Home tab; hidden at zero.
  final int homeBadgeCount;

  final ValueChanged<RiplsNavDestination> onDestinationTap;
  final VoidCallback onCreateTap;

  const NavDock({
    super.key,
    required this.selectedStackIndex,
    required this.homeBadgeCount,
    required this.onDestinationTap,
    required this.onCreateTap,
  });

  @override
  State<NavDock> createState() => _NavDockState();
}

class _NavDockState extends State<NavDock>
    with SingleTickerProviderStateMixin {
  static const _slideDuration = Duration(milliseconds: 280);
  static const _slideCurve = Curves.easeInOutCubic;

  static const double _iconSize = 22;
  static const double _labelGap = 8;
  static const double _pillPadding = 15;

  /// Bubble width beyond the label text: side paddings + icon + gap.
  static const double _pillChrome =
      2 * _pillPadding + _iconSize + _labelGap;
  static const double _pillHeight = 38;
  static const double _pillRadius = 16;

  /// The Create circle (48) plus the spacer between it and the tabs (4).
  static const double _createRegionWidth = 52;

  /// Floor for unselected slot widths: the bubble may not squeeze the
  /// other tabs below a findable target, so oversized labels (long
  /// translations, large text scale) clamp and ellipsize instead.
  static const double _minSlotWidth = 40;

  /// Style of the bubble label. Kept here (not on the Text alone) because
  /// slot widths are computed from a TextPainter measurement of it.
  static const TextStyle _labelStyle = TextStyle(
    fontSize: 12,
    fontWeight: FontWeight.w600,
    letterSpacing: 0.1,
  );

  late final AnimationController _slide = AnimationController(
    vsync: this,
    duration: _slideDuration,
    value: 1,
  );

  /// Geometry the bubble departs from while [_slide] runs. A pixel
  /// snapshot of the last rendered frame (not a destination) so a
  /// selection change landing mid-flight departs from where the bubble
  /// currently is instead of teleporting to the old target first.
  _DockGeometry? _departure;

  /// Geometry of the most recently rendered frame; becomes [_departure]
  /// on the next selection change.
  _DockGeometry? _rendered;

  @override
  void didUpdateWidget(NavDock oldWidget) {
    super.didUpdateWidget(oldWidget);
    final from =
        RiplsNavDestination.fromStackIndex(oldWidget.selectedStackIndex);
    final to = RiplsNavDestination.fromStackIndex(widget.selectedStackIndex);
    if (from == to) return;
    if (from == null || to == null || _rendered == null) {
      // Entering from or leaving to an orphaned stack index: there is no
      // travel to show, so snap to the new state.
      _departure = null;
      _slide.value = 1;
      return;
    }
    _departure = _rendered;
    _slide.duration = accessibleDuration(context, _slideDuration);
    _slide.forward(from: 0);
  }

  @override
  void dispose() {
    _slide.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // High contrast doubles as the reduce-transparency signal: swap the
    // blur for a solid fill at identical geometry.
    final solid = MediaQuery.highContrastOf(context);
    final selected =
        RiplsNavDestination.fromStackIndex(widget.selectedStackIndex);
    const radius = 26.0;

    // The capsule caps itself at the dock measure (#2912): on a desktop-wide
    // window it stays a hand-sized centered pill instead of stretching its
    // four icons across the whole viewport. Phone widths sit under the cap,
    // so this never binds there. The slot math below is constraint-driven
    // (LayoutBuilder), so it simply receives the capped width.
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: Responsive.dockMaxWidth),
        child: _buildCapsule(context, selected, radius, solid: solid),
      ),
    );
  }

  Widget _buildCapsule(
    BuildContext context,
    RiplsNavDestination? selected,
    double radius, {
    required bool solid,
  }) {
    return DecoratedBox(
      // The drop shadow must paint outside GlassSurface's clip.
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(radius),
        boxShadow: const [
          BoxShadow(
            color: Color(0x2E3C462D), // 0 10 26 rgba(60,70,45,.18)
            blurRadius: 26,
            offset: Offset(0, 10),
          ),
        ],
      ),
      child: GlassSurface(
        useBlur: !solid,
        // The light glass material is deliberate in both themes (mock
        // variant 1); high contrast gets the opaque light surface.
        fill: solid
            ? AppColors.lightCardBackground
            // 0.42 made this "committed light material" light only over a light
            // page. Over the dark theme's #141714 it composites to #777877, and
            // the dark glyphs it is designed for measured 1.82:1 there — the
            // dock reads fine in light theme and is unreadable in dark, which is
            // exactly how it was reported. 0.60 composites to #a1a2a1 in the
            // worst case while still showing the backdrop.
            : Colors.white.withValues(alpha: 0.60),
        border: Colors.white.withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(radius),
        child: SizedBox(
          height: NavDock.dockHeight,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 11),
            child: LayoutBuilder(
              builder: (context, constraints) {
                final slotsWidth = constraints.maxWidth - _createRegionWidth;
                final labelWidths = _labelWidths(context, slotsWidth);
                return AnimatedBuilder(
                  animation: _slide,
                  builder: (context, _) => _dockBody(
                    context,
                    selected,
                    slotsWidth,
                    labelWidths,
                    solid: solid,
                  ),
                );
              },
            ),
          ),
        ),
      ),
    );
  }

  Widget _dockBody(
    BuildContext context,
    RiplsNavDestination? selected,
    double slotsWidth,
    Map<RiplsNavDestination, double> labelWidths, {
    required bool solid,
  }) {
    final arrival = _geometryFor(selected, slotsWidth, labelWidths);
    final t = _slideCurve.transform(_slide.value);
    final geometry = t >= 1 || _departure == null
        ? arrival
        : _DockGeometry.lerp(_departure!, arrival, t);
    _rendered = geometry;

    final destinations = RiplsNavDestination.values;
    return Stack(
      children: [
        if (geometry.pillStart != null)
          PositionedDirectional(
            start: geometry.pillStart!,
            width: geometry.pillWidth,
            top: (NavDock.dockHeight - _pillHeight) / 2,
            height: _pillHeight,
            child: DecoratedBox(
              key: NavDock.selectionPillKey,
              decoration: BoxDecoration(
                color: solid
                    ? Colors.white
                    : Colors.white.withValues(alpha: 0.72),
                borderRadius: BorderRadius.circular(_pillRadius),
              ),
            ),
          ),
        Row(
          children: [
            for (final (i, destination) in destinations.indexed)
              _slot(
                context,
                destination,
                width: geometry.slotWidths[i],
                labelReveal: geometry.labelReveals[i],
                selection: geometry.selection[i],
                isSelected: destination == selected,
                labelWidth: labelWidths[destination]!,
                badgeCount: destination == RiplsNavDestination.home
                    ? widget.homeBadgeCount
                    : 0,
              ),
            const SizedBox(width: 4),
            _CreateCircle(onTap: widget.onCreateTap, solid: solid),
          ],
        ),
      ],
    );
  }

  /// One tab slot. [width], [labelReveal], and [selection] come
  /// pre-interpolated from the dock geometry so the slot never overflows:
  /// its content (icon + revealed label region) shrinks in lockstep with
  /// the slot itself.
  Widget _slot(
    BuildContext context,
    RiplsNavDestination destination, {
    required double width,
    required double labelReveal,
    required double selection,
    required bool isSelected,
    required double labelWidth,
    int badgeCount = 0,
  }) {
    // Committed light-material palette (the bar never flips dark).
    //
    // The unselected end is textPrimary, not textSecondary. The secondary step
    // is validated against a white PAGE (6.56:1 there), but this dock is a
    // translucent material whose composite is far darker, and the unselected
    // glyphs measured 2.48:1 on it. The selected end is unaffected — that label
    // sits on its own white pill at 7.61:1, which is why only the unselected
    // icons failed.
    final color = Color.lerp(
      AppColors.lightTextPrimary,
      AppColors.lightPrimary,
      selection,
    )!;

    final icon = Stack(
      clipBehavior: Clip.none,
      children: [
        Icon(
          isSelected ? destination.activeIcon : destination.icon,
          size: _iconSize,
          color: color,
        ),
        if (badgeCount > 0)
          Positioned(
            right: -8,
            top: -6,
            child: UnreadBadge(
              count: badgeCount,
              fontSize: 10,
              padding:
                  const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
              backgroundColor: AppColors.transferCoral,
              textColor: Colors.white,
            ),
          ),
      ],
    );

    return SizedBox(
      width: width,
      child: Tappable(
        semanticsLabel: destination.label(context),
        onTap: () => widget.onDestinationTap(destination),
        inkBorderRadius: BorderRadius.circular(_pillRadius),
        child: SizedBox(
          height: NavDock.dockHeight,
          child: Center(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                icon,
                if (labelReveal > 0)
                  SizedBox(
                    width: labelReveal,
                    height: _pillHeight,
                    child: ClipRect(
                      child: OverflowBox(
                        // The label keeps its full-reveal layout and gets
                        // clipped by the animating reveal region, so the
                        // text slides out beside the icon instead of
                        // reflowing.
                        minWidth: 0,
                        minHeight: 0,
                        maxWidth: _labelGap + labelWidth,
                        alignment: AlignmentDirectional.centerStart,
                        child: Padding(
                          padding: const EdgeInsetsDirectional.only(
                            start: _labelGap,
                          ),
                          child: Opacity(
                            opacity: selection,
                            child: Text(
                              destination.label(context),
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: _labelStyle.copyWith(color: color),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Resolved geometry for one selection state.
  _DockGeometry _geometryFor(
    RiplsNavDestination? selected,
    double slotsWidth,
    Map<RiplsNavDestination, double> labelWidths,
  ) {
    final destinations = RiplsNavDestination.values;
    if (selected == null) {
      return _DockGeometry(
        slotWidths:
            List.filled(destinations.length, slotsWidth / destinations.length),
        labelReveals: List.filled(destinations.length, 0),
        selection: List.filled(destinations.length, 0),
        pillStart: null,
        pillWidth: null,
      );
    }
    final pillWidth = _pillChrome + labelWidths[selected]!;
    final unselectedWidth =
        (slotsWidth - pillWidth) / (destinations.length - 1);
    return _DockGeometry(
      slotWidths: [
        for (final d in destinations)
          d == selected ? pillWidth : unselectedWidth,
      ],
      labelReveals: [
        for (final d in destinations)
          d == selected ? _labelGap + labelWidths[d]! : 0.0,
      ],
      selection: [
        for (final d in destinations) d == selected ? 1.0 : 0.0,
      ],
      // Every slot before the selected one is unselected, so the bubble
      // starts at a whole number of unselected widths.
      pillStart: unselectedWidth * selected.index,
      pillWidth: pillWidth,
    );
  }

  Map<RiplsNavDestination, double> _labelWidths(
    BuildContext context,
    double slotsWidth,
  ) {
    // Explicit type argument, not a `0.0` literal: `prefer_int_literals`
    // rewrites the literal to `0`, which infers `math.max<num>` and breaks the
    // `double` return this feeds (#2794).
    final maxLabel = math.max<double>(
      0,
      slotsWidth -
          (RiplsNavDestination.values.length - 1) * _minSlotWidth -
          _pillChrome,
    );
    final scaler = MediaQuery.textScalerOf(context);
    final direction = Directionality.of(context);
    return {
      for (final d in RiplsNavDestination.values)
        d: math.min(
          _measureLabel(d.label(context), scaler, direction),
          maxLabel,
        ),
    };
  }

  static double _measureLabel(
    String text,
    TextScaler scaler,
    TextDirection direction,
  ) {
    final painter = TextPainter(
      text: TextSpan(text: text, style: _labelStyle),
      textDirection: direction,
      textScaler: scaler,
      maxLines: 1,
    )..layout();
    final width = painter.width.ceilToDouble();
    painter.dispose();
    return width;
  }
}

/// Horizontal geometry of the dock's tab strip for one selection state —
/// or, mid-slide, the interpolation between two states. All lists are in
/// [RiplsNavDestination.values] order.
class _DockGeometry {
  final List<double> slotWidths;

  /// Width of each slot's revealed label region (gap + text); zero when
  /// the slot shows no label.
  final List<double> labelReveals;

  /// Per-slot selection strength in 0..1, driving icon tint and label
  /// opacity.
  final List<double> selection;

  /// Bubble rect within the tab strip; null when nothing is selected.
  final double? pillStart;
  final double? pillWidth;

  const _DockGeometry({
    required this.slotWidths,
    required this.labelReveals,
    required this.selection,
    required this.pillStart,
    required this.pillWidth,
  });

  factory _DockGeometry.lerp(_DockGeometry a, _DockGeometry b, double t) {
    List<double> lerpList(List<double> x, List<double> y) =>
        [for (var i = 0; i < x.length; i++) lerpDouble(x[i], y[i], t)!];
    double? lerpPill(double? x, double? y) =>
        x == null || y == null ? y : lerpDouble(x, y, t);
    return _DockGeometry(
      slotWidths: lerpList(a.slotWidths, b.slotWidths),
      labelReveals: lerpList(a.labelReveals, b.labelReveals),
      selection: lerpList(a.selection, b.selection),
      pillStart: lerpPill(a.pillStart, b.pillStart),
      pillWidth: lerpPill(a.pillWidth, b.pillWidth),
    );
  }
}

/// The Create action: a glass circle inside the bar's right end — the
/// dock's biggest target, in the thumb corner.
class _CreateCircle extends StatelessWidget {
  final VoidCallback onTap;
  final bool solid;

  const _CreateCircle({required this.onTap, required this.solid});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yMiscNavAdd,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(NavDock.dockHeight / 2),
      child: SizedBox(
        width: 48,
        height: NavDock.dockHeight,
        child: Center(
          child: Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              // High contrast fills the circle solid for a hard target;
              // the glass look is a translucent white with specular border.
              color: solid
                  ? AppColors.lightPrimary
                  : Colors.white.withValues(alpha: 0.55),
              shape: BoxShape.circle,
              border: solid
                  ? null
                  : Border.all(color: Colors.white.withValues(alpha: 0.7)),
            ),
            child: Icon(
              Icons.add,
              size: 23,
              color: solid ? Colors.white : AppColors.lightPrimary,
            ),
          ),
        ),
      ),
    );
  }
}
