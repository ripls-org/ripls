import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/repositories/gear_repository.dart' show GearBooking;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearBookingGrid is the Monday-first month grid for the who's-using calendar:
/// a weekday header and a 7-column grid of day cells. Booked days are tinted by
/// the borrower's colour with their avatar; the viewer's own days get a mint
/// ring; open future days show a "+"; past days are dimmed. Selected days (the
/// claim range) get a white ring. Tapping a day reports the day and its booking
/// (if any) back to the parent, which owns the selection + claim logic.
class GearBookingGrid extends StatelessWidget {
  final DateTime month;
  final Map<DateTime, GearBooking> byDay;
  final DateTime? selStart;
  final DateTime? selEnd;
  final void Function(DateTime day, GearBooking? booking) onTapDay;

  const GearBookingGrid({
    super.key,
    required this.month,
    required this.byDay,
    required this.selStart,
    required this.selEnd,
    required this.onTapDay,
  });

  static const _mint = Color(0xFFA7C59E);

  DateTime _dayKey(DateTime d) => DateTime(d.year, d.month, d.day);

  bool _inSelection(DateTime day) {
    if (selStart == null || selEnd == null) return false;
    return !day.isBefore(selStart!) && !day.isAfter(selEnd!);
  }

  @override
  Widget build(BuildContext context) {
    final firstOfMonth = DateTime(month.year, month.month, 1);
    final daysInMonth = DateTime(month.year, month.month + 1, 0).day;
    final leadingBlanks = firstOfMonth.weekday - 1; // Monday-first
    final today = _dayKey(DateTime.now());

    final cells = <Widget>[
      for (var i = 0; i < leadingBlanks; i++) const SizedBox.shrink(),
    ];
    for (var d = 1; d <= daysInMonth; d++) {
      final day = DateTime(month.year, month.month, d);
      cells.add(_cell(day, byDay[day], today));
    }
    while (cells.length % 7 != 0) {
      cells.add(const SizedBox.shrink());
    }

    final rows = cells.length ~/ 7;
    return LayoutBuilder(
      builder: (context, constraints) {
        // Same square-cell sizing as the Plans month grid: without a cap the
        // cells grow with the viewport width and the month turns into a wall of
        // huge squares on a wide window (#2908). This grid lives in a
        // SingleChildScrollView, so height stays unbounded and only the
        // per-cell cap binds — enough to keep the month readable.
        final side = monthGridCellSide(
          availableWidth: constraints.maxWidth,
          maxHeight: double.infinity,
          rows: rows,
        );
        return Center(
          child: SizedBox(
            width: monthGridWidth(side),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                SizedBox(
                  height: monthGridWeekdayHeaderHeight,
                  child: _weekdayHeader(),
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

  /// Monday-first single-letter weekday initials. Pinned to
  /// [monthGridWeekdayHeaderHeight] by its caller so the cell arithmetic in
  /// [monthGridCellSide] is exact.
  Widget _weekdayHeader() {
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

  Widget _cell(DateTime day, GearBooking? booking, DateTime today) {
    const gold = Color(0xFFD8A13A); // pending link reservation
    const self = Color(0xFFCDBB8A); // owner's self-block
    final isPast = day.isBefore(today);
    final isToday = day == today;
    final isSelected = _inSelection(day);
    final isMine = booking?.isMine ?? false;
    final isPending = booking?.isPending ?? false;
    final isBlock = booking?.isOwnerBlock ?? false;
    final Color? color = booking == null
        ? null
        : isPending
            ? gold
            : isBlock
                ? self
                : _userColor(booking.borrower.id);

    final tappable = booking != null || !isPast;

    Border border;
    if (isSelected) {
      border = Border.all(color: Colors.white, width: 2.5);
    } else if (isPending) {
      border = Border.all(color: gold, width: 2);
    } else if (isBlock) {
      border = Border.all(color: self, width: 2);
    } else if (isMine) {
      border = Border.all(color: _mint, width: 2);
    } else if (isToday) {
      border = Border.all(color: Colors.white, width: 2);
    } else {
      border = Border.all(color: Colors.white.withAlpha(20));
    }

    final cell = Container(
      decoration: BoxDecoration(
        color: booking != null
            ? color!.withAlpha(64)
            : (isPast ? Colors.transparent : _mint.withAlpha(20)),
        borderRadius: BorderRadius.circular(12),
        border: border,
      ),
      child: Stack(
        children: [
          Positioned(
            top: 4,
            left: 6,
            child: Text(
              '${day.day}',
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                color: Colors.white,
                shadows: [Shadow(blurRadius: 3, color: Color(0xBF000000))],
              ),
            ),
          ),
          if (isPending)
            const Positioned(
              bottom: 3,
              right: 5,
              child: Text('?',
                  style: TextStyle(
                      color: gold, fontSize: 14, fontWeight: FontWeight.w800)),
            )
          else if (isBlock)
            const Positioned(
              bottom: 3,
              right: 5,
              child: Icon(Icons.lock_rounded, size: 12, color: self),
            )
          else if (booking != null)
            Positioned(
              bottom: 4,
              right: 4,
              child: UserAvatar(user: booking.borrower, radius: 9),
            )
          else if (!isPast)
            Center(
              child: Text(
                '+',
                style: TextStyle(
                  fontSize: 18,
                  color: _mint.withAlpha(166),
                  fontWeight: FontWeight.w400,
                ),
              ),
            ),
          if (isToday)
            Positioned(
              top: 6,
              right: 6,
              child: Container(
                width: 5,
                height: 5,
                decoration: const BoxDecoration(
                  color: Colors.white,
                  shape: BoxShape.circle,
                ),
              ),
            ),
        ],
      ),
    );

    return AspectRatio(
      aspectRatio: 1,
      child: Opacity(
        opacity: isPast && booking == null ? 0.32 : 1,
        child: tappable
            ? Tappable(
                semanticsLabel: DateFormat('EEEE, MMMM d').format(day),
                onTap: () => onTapDay(day, booking),
                inkBorderRadius: BorderRadius.circular(12),
                child: cell,
              )
            : cell,
      ),
    );
  }

  /// A stable, seeded fill colour per borrower so each person's days read as a
  /// distinct colour across the month.
  Color _userColor(String seed) {
    const palette = [
      Color(0xFF7FB6EE),
      Color(0xFFE7B98A),
      Color(0xFFE7906A),
      Color(0xFF9B8ED6),
      Color(0xFF6B8F71),
      Color(0xFFB08A4F),
    ];
    var h = 0;
    for (final code in seed.codeUnits) {
      h = (h * 31 + code) & 0x7fffffff;
    }
    return palette[seed.isEmpty ? 0 : h % palette.length];
  }
}
