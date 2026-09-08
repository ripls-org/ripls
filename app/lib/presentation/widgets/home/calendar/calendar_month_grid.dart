import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_cell.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_data.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';

/// CalendarMonthGrid is the Monday-first month grid floating over the backdrop:
/// a weekday header row and a 7-column grid of [CalendarDayCell]s for [month].
/// Leading/trailing slots from adjacent months are blank, matching the design
/// mockup's clean month frame.
///
/// Cells are square, so the grid's height follows its width. Callers that stack
/// content beneath it must pass a [maxHeight] budget or the grid will grow past
/// the viewport on wide windows and starve whatever sits below (#2908) — see
/// [monthGridCellSide].
class CalendarMonthGrid extends StatelessWidget {
  /// Any day within the month to render (used only for year/month).
  final DateTime month;
  final CalendarMonthData data;
  final DateTime selectedDay;
  final DateTime today;

  /// Height budget for the weekday header plus all cell rows. Defaults to
  /// unbounded, which reproduces the pre-#2908 width-driven sizing — correct
  /// only inside a scrollable parent.
  final double maxHeight;

  /// The committed event date on the "When" screen, rendered as a solid amber
  /// fill that stays highlighted even while [selectedDay] moves to another day
  /// the viewer is exploring. Null on the Home calendar (no chosen date).
  final DateTime? chosenDay;

  /// The last day of a multi-day event — every day in [chosenDay]..[chosenEndDay]
  /// (inclusive) gets the chosen fill. Null/equal to [chosenDay] for a
  /// single-day event.
  final DateTime? chosenEndDay;
  final void Function(DateTime) onDaySelected;

  const CalendarMonthGrid({
    super.key,
    required this.month,
    required this.data,
    required this.selectedDay,
    required this.today,
    this.chosenDay,
    this.chosenEndDay,
    this.maxHeight = double.infinity,
    required this.onDaySelected,
  });

  @override
  Widget build(BuildContext context) {
    final firstOfMonth = DateTime(month.year, month.month, 1);
    final daysInMonth = DateTime(month.year, month.month + 1, 0).day;
    // Monday-first: weekday 1=Mon..7=Sun → leading blanks before day 1.
    final leadingBlanks = firstOfMonth.weekday - 1;

    final cells = <Widget>[];
    for (var i = 0; i < leadingBlanks; i++) {
      cells.add(const SizedBox.shrink());
    }
    // The chosen span [chosenStart, chosenEnd] (inclusive) — every day in it
    // gets the chosen fill, so a multi-day event highlights all its days.
    final chosenStart =
        chosenDay == null ? null : CalendarMonthData.dayKey(chosenDay!);
    final chosenEnd = chosenStart == null
        ? null
        : CalendarMonthData.dayKey(chosenEndDay ?? chosenDay!);
    for (var d = 1; d <= daysInMonth; d++) {
      final day = DateTime(month.year, month.month, d);
      final isChosen = chosenStart != null &&
          !day.isBefore(chosenStart) &&
          !day.isAfter(chosenEnd!);
      cells.add(CalendarDayCell(
        day: day,
        events: data.eventsOn(day),
        forecast: data.forecastOn(day),
        photoMediaId: data.marqueePhotoOn(day),
        isToday: day == today,
        // The chosen day owns the highlight; don't also draw the selection ring
        // on it when the viewer is parked there.
        isSelected: !isChosen && day == CalendarMonthData.dayKey(selectedDay),
        isChosen: isChosen,
        onTap: () => onDaySelected(day),
      ));
    }
    // Trailing blanks to complete the final row.
    while (cells.length % 7 != 0) {
      cells.add(const SizedBox.shrink());
    }

    // Same count monthGridRows(month) yields — a pane sizing itself around
    // this grid predicts the width from that helper, so the two must agree.
    assert(cells.length ~/ 7 == monthGridRows(month));
    final rows = cells.length ~/ 7;
    return LayoutBuilder(
      builder: (context, constraints) {
        // Square cells mean width alone would set the grid's height. Size from
        // both axes so a landscape window can't push the day detail off-screen
        // (#2908); on phones width still binds, leaving that layout untouched.
        final side = monthGridCellSide(
          availableWidth: constraints.maxWidth,
          maxHeight: maxHeight,
          rows: rows,
        );
        return Center(
          child: SizedBox(
            width: monthGridWidth(side),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const SizedBox(
                  height: monthGridWeekdayHeaderHeight,
                  child: _WeekdayHeader(),
                ),
                const SizedBox(height: monthGridHeaderGap),
                GridView.count(
                  crossAxisCount: 7,
                  shrinkWrap: true,
                  physics: const NeverScrollableScrollPhysics(),
                  mainAxisSpacing: monthGridSpacing,
                  crossAxisSpacing: monthGridSpacing,
                  children: cells,
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Monday-first single-letter weekday initials, localized. A widget rather than
/// a method so the header can sit in a const [SizedBox] of the exact height the
/// cell arithmetic in [monthGridCellSide] assumes.
class _WeekdayHeader extends StatelessWidget {
  const _WeekdayHeader();

  @override
  Widget build(BuildContext context) {
    final base = DateTime(2024, 1, 1); // a Monday
    return Row(
      children: [
        for (var i = 0; i < 7; i++)
          Expanded(
            child: Center(
              child: Text(
                DateFormat('EEE')
                    .format(base.add(Duration(days: i)))
                    .substring(0, 1)
                    .toUpperCase(),
                style: TextStyle(
                  fontSize: 9,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 0.4,
                  color: Colors.white.withAlpha(158),
                ),
              ),
            ),
          ),
      ],
    );
  }
}
