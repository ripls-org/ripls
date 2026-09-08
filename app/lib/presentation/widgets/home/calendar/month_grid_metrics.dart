/// Layout metrics shared by the Monday-first month grids (the Plans calendar's
/// [CalendarMonthGrid] and gear's booking grid). Both draw a 7-column grid of
/// *square* day cells, so a grid left to size itself from its width alone grows
/// taller as the window widens.
///
/// That is what broke the app in landscape browser windows (#2908): the Plans
/// grid sits in a non-scrolling Column above the day-detail panel, so past
/// roughly a 1.1 : 1 width-to-height ratio — every landscape window there is —
/// the grid's height consumed the whole viewport, the day detail was starved to
/// nothing, and the month's last rows clipped away with no way to scroll to
/// them.
///
/// The fix is [monthGridCellSide]: derive the cell from *both* axes and take
/// the smaller. Width remains the binding constraint on phones, so the phone
/// layout is bit-for-bit unchanged; height binds on wide viewports, where the
/// month becomes a centred block that always leaves room for what sits below.
library;

import 'dart:math' as math;

/// Gap between day cells, on both axes.
const double monthGridSpacing = 5;

/// Share of a screen's height the month grid may occupy when it is stacked
/// above other content (a day-detail panel) rather than sitting in a scroll
/// view. Chosen so phones — where the grid is well under half the screen and
/// width is the binding constraint — render exactly as before, while wide
/// windows get a month capped short enough to leave a usable panel below.
const double monthGridHeightShare = 0.55;

/// Height reserved for the weekday-initials header row above the cells.
/// Pinned rather than intrinsic so the cell arithmetic is exact.
const double monthGridWeekdayHeaderHeight = 14;

/// Gap between the weekday header and the first row of cells.
const double monthGridHeaderGap = 7;

/// Vertical extent the header occupies above the cells.
const double monthGridHeaderExtent =
    monthGridWeekdayHeaderHeight + monthGridHeaderGap;

/// Side length of one square day cell.
///
/// [availableWidth] is the width the grid may occupy; [rows] the number of
/// week rows the month needs (4–6); [maxHeight] the total height budget for
/// header *and* cells — pass [double.infinity] when the grid lives in a
/// scrollable parent and only [maxCellSide] should cap it.
///
/// [maxCellSide] stops a cell from ballooning on a very wide viewport even
/// when height is unbounded; a month of 200px squares is not a calendar
/// anybody wants to read.
///
/// Returns a strictly positive value: a caller with a degenerate budget still
/// gets a laid-out (if tiny) grid rather than a negative-extent crash.
double monthGridCellSide({
  required double availableWidth,
  required double maxHeight,
  required int rows,
  double maxCellSide = 96,
}) {
  assert(rows > 0, 'a month always has at least one week row');
  final fromWidth = (availableWidth - monthGridSpacing * 6) / 7;
  final cellsBudget =
      maxHeight - monthGridHeaderExtent - monthGridSpacing * (rows - 1);
  final fromHeight = cellsBudget / rows;
  final side = math.min(math.min(fromWidth, fromHeight), maxCellSide);
  // Guard the degenerate cases (a caller mid-animation with a zero or negative
  // budget) so layout never sees a negative extent.
  return side.isFinite && side > 0 ? side : 1;
}

/// Total width a grid of [rows]-agnostic 7-column [cellSide] squares occupies,
/// including the inter-cell gaps. Used to centre the block when height — not
/// width — is the binding constraint.
double monthGridWidth(double cellSide) => cellSide * 7 + monthGridSpacing * 6;

/// Week rows a Monday-first grid needs to draw [month] — 4 (a non-leap
/// February starting on a Monday) through 6.
///
/// Shared so a layout that must size a pane *around* the grid computes the
/// same row count the grid itself will use; [CalendarMonthGrid] derives it
/// from the cell list it builds, and the two must not drift.
int monthGridRows(DateTime month) {
  final leadingBlanks = DateTime(month.year, month.month, 1).weekday - 1;
  final daysInMonth = DateTime(month.year, month.month + 1, 0).day;
  return ((leadingBlanks + daysInMonth) / 7).ceil();
}

/// The width the grid will actually occupy for [rows] rows given the space it
/// is offered — i.e. [monthGridWidth] of the side [monthGridCellSide] picks.
///
/// A pane that wants to fit the grid rather than merely contain it needs this:
/// square cells capped at `maxCellSide` mean the grid frequently *cannot*
/// fill a wide pane, and the surplus has to be placed deliberately rather
/// than left for a `Center` to split (#2926).
double monthGridNaturalWidth({
  required double availableWidth,
  required double maxHeight,
  required int rows,
}) =>
    monthGridWidth(monthGridCellSide(
      availableWidth: availableWidth,
      maxHeight: maxHeight,
      rows: rows,
    ));

/// Total height the header plus [rows] rows of [cellSide] squares occupies.
double monthGridHeight(double cellSide, int rows) =>
    monthGridHeaderExtent + cellSide * rows + monthGridSpacing * (rows - 1);
