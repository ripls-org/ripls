import 'dart:math' as math;

import 'package:flutter/widgets.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';

/// Lays out a month grid over/beside its day detail (#2912).
///
/// The three calendar surfaces (Plans, the Workshop community calendar, the
/// shared calendar) all stack `[grid, Expanded(detail)]` in a column. In a
/// compact window this widget reproduces that column exactly — the grid keeps
/// the caller-computed height budget (#2908), so phone geometry is untouched.
/// At [Responsive.expandedBreakpoint] of available width the two become
/// side-by-side panes.
///
/// **The side-by-side arm sizes the grid pane to the grid, then centres the
/// pair.** Its first cut gave the grid an `Expanded` pane and centred the grid
/// inside it, which looked right and wasn't: square cells are capped at
/// `monthGridCellSide`'s `maxCellSide`, so on a wide window the grid *cannot*
/// fill the pane, and the `Center` split the surplus evenly — half of it
/// landing in the seam between the two panes while the rail stayed starved at
/// its minimum. Now the grid pane takes only the width the grid will occupy,
/// the rail absorbs the surplus up to [detailPaneWidth], and whatever is left
/// over goes to the outer margins where it reads as page margin rather than
/// as a gap.
///
/// Child order is grid-then-detail in both arms, so semantics traversal and
/// keyboard focus order match the visual reading order either way.
class CalendarPanes extends StatelessWidget {
  const CalendarPanes({
    super.key,
    required this.stackedGridMaxHeight,
    required this.gridRows,
    required this.gridBuilder,
    required this.detail,
    this.detailPaneWidth = 560,
    this.minDetailPaneWidth = 380,
  });

  /// Week rows the month needs — `monthGridRows(month)`. The side-by-side arm
  /// needs it to predict the grid's width before laying the panes out; the
  /// grid derives the same number internally from its own cell list.
  final int gridRows;

  /// The grid's height budget in the stacked arm — computed by the caller
  /// from the full body height exactly as before this widget existed
  /// (`outer.maxHeight * monthGridHeightShare`), so the compact layout is
  /// bit-identical to the pre-#2912 one.
  final double stackedGridMaxHeight;

  /// Builds the month grid given the active arm's height budget.
  final Widget Function(BuildContext context, double maxGridHeight)
      gridBuilder;

  /// The day-detail area — the caller's existing `Align` + scroll subtree.
  /// Stacked, it fills the space under the grid; side-by-side, it fills the
  /// trailing rail.
  final Widget detail;

  /// Widest the trailing day-detail rail may grow in the side-by-side arm.
  /// It only reaches this once the grid has taken the width it can use — the
  /// rail is where reclaimed space goes, because that is what event titles
  /// (two lines, then ellipsis) actually need.
  final double detailPaneWidth;

  /// Narrowest the rail may be squeezed to. Holds the pre-#2926 width, so at
  /// the breakpoint — where there is no surplus to share — the layout is
  /// unchanged.
  final double minDetailPaneWidth;

  /// Horizontal inset around the grid inside its pane.
  static const double _gridInset = 18;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, region) {
      final sideBySide = region.maxWidth >= Responsive.expandedBreakpoint;
      if (!sideBySide) {
        return Column(children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(_gridInset, 8, _gridInset, 0),
            child: gridBuilder(context, stackedGridMaxHeight),
          ),
          Expanded(child: detail),
        ]);
      }

      final gridHeightBudget = math.max<double>(0, region.maxHeight - 16);

      // What the grid would occupy if width never bound it — height and the
      // cell cap alone. The surplus over this is what there is to share.
      final unconstrainedGrid = monthGridNaturalWidth(
        availableWidth: double.infinity,
        maxHeight: gridHeightBudget,
        rows: gridRows,
      );
      final rail = (region.maxWidth - unconstrainedGrid - _gridInset * 2)
          .clamp(minDetailPaneWidth, detailPaneWidth)
          .toDouble();

      // Re-derive the grid against the width actually left for it: near the
      // breakpoint the rail wins and width binds the cells after all.
      final gridPane = monthGridNaturalWidth(
            availableWidth: math.max<double>(
                0, region.maxWidth - rail - _gridInset * 2),
            maxHeight: gridHeightBudget,
            rows: gridRows,
          ) +
          _gridInset * 2;

      return Center(
        child: SizedBox(
          width: math.min(region.maxWidth, gridPane + rail),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SizedBox(
                width: gridPane,
                child: Center(
                  child: Padding(
                    padding: const EdgeInsets.fromLTRB(
                        _gridInset, 8, _gridInset, 8),
                    child: gridBuilder(context, gridHeightBudget),
                  ),
                ),
              ),
              SizedBox(width: rail, child: detail),
            ],
          ),
        ),
      );
    });
  }
}
