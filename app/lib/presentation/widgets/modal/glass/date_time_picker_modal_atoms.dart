import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:intl/intl.dart' hide TextDirection;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Calendar grid + month-nav header used inside the date-time picker modal.
class CalendarGrid extends StatelessWidget {
  final DateTime visibleMonth;
  final DateTime selectedDate;
  final DateTime firstDate;
  final DateTime lastDate;
  final bool canGoToPreviousMonth;
  final bool canGoToNextMonth;
  final ValueChanged<DateTime> onDaySelected;
  final VoidCallback onPreviousMonth;
  final VoidCallback onNextMonth;

  const CalendarGrid({
    super.key,
    required this.visibleMonth,
    required this.selectedDate,
    required this.firstDate,
    required this.lastDate,
    required this.canGoToPreviousMonth,
    required this.canGoToNextMonth,
    required this.onDaySelected,
    required this.onPreviousMonth,
    required this.onNextMonth,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final monthLabel = DateFormat.yMMMM().format(visibleMonth);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                monthLabel,
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
            IconAction(
              icon: Icons.chevron_left,
              semanticsLabel: l10n.a11yDateTimeModalMonthPrev,
              onPressed: canGoToPreviousMonth ? onPreviousMonth : null,
              color: AppColors.modalTextPrimary,
              iconSize: 20,
              padding: const EdgeInsets.all(4),
              constraints: const BoxConstraints.tightFor(width: 32, height: 32),
            ),
            IconAction(
              icon: Icons.chevron_right,
              semanticsLabel: l10n.a11yDateTimeModalMonthNext,
              onPressed: canGoToNextMonth ? onNextMonth : null,
              color: AppColors.modalTextPrimary,
              iconSize: 20,
              padding: const EdgeInsets.all(4),
              constraints: const BoxConstraints.tightFor(width: 32, height: 32),
            ),
          ],
        ),
        const SizedBox(height: 8),
        _WeekdayHeader(),
        const SizedBox(height: 4),
        _DayGrid(
          visibleMonth: visibleMonth,
          selectedDate: selectedDate,
          firstDate: firstDate,
          lastDate: lastDate,
          onDaySelected: onDaySelected,
        ),
      ],
    );
  }
}

class _WeekdayHeader extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    // Locale-aware short weekday names. DateFormat.E gives "Sun, Mon, ...".
    final symbols = DateFormat.E(Localizations.localeOf(context).toString())
        .dateSymbols
        .NARROWWEEKDAYS;
    return Row(
      children: [
        for (final day in symbols)
          Expanded(
            child: Center(
              child: Text(
                day,
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: AppColors.modalTextMuted,
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class _DayGrid extends StatelessWidget {
  final DateTime visibleMonth;
  final DateTime selectedDate;
  final DateTime firstDate;
  final DateTime lastDate;
  final ValueChanged<DateTime> onDaySelected;

  const _DayGrid({
    required this.visibleMonth,
    required this.selectedDate,
    required this.firstDate,
    required this.lastDate,
    required this.onDaySelected,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final firstOfMonth = DateTime(visibleMonth.year, visibleMonth.month, 1);
    final daysInMonth =
        DateTime(visibleMonth.year, visibleMonth.month + 1, 0).day;
    // Dart's DateTime.weekday: Mon=1..Sun=7. The grid starts on Sunday, so
    // weekday 7 maps to leading-blank index 0.
    final leading = firstOfMonth.weekday % 7;
    final totalCells = leading + daysInMonth;
    final rows = (totalCells / 7).ceil();
    final firstDay = DateTime(firstDate.year, firstDate.month, firstDate.day);
    final lastDay = DateTime(lastDate.year, lastDate.month, lastDate.day);
    final selected =
        DateTime(selectedDate.year, selectedDate.month, selectedDate.day);

    // Hand-built Column of Rows — one Row per week. Previously this was a
    // GridView.builder with shrinkWrap + childAspectRatio: 1.05, which
    // over-allocated ~30 logical px of phantom row space at the bottom on
    // months with a short final week (e.g. May 2026 ends with "31" alone).
    // Explicit rows make height = rows × cellHeight + (rows-1) × spacing,
    // exactly.
    const cellHeight = 40.0;
    const rowSpacing = 2.0;
    final weeks = <List<int?>>[];
    for (var row = 0; row < rows; row++) {
      final week = <int?>[];
      for (var col = 0; col < 7; col++) {
        final index = row * 7 + col;
        final dayNum = index - leading + 1;
        week.add(dayNum < 1 || dayNum > daysInMonth ? null : dayNum);
      }
      weeks.add(week);
    }

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (var i = 0; i < weeks.length; i++) ...[
          if (i > 0) const SizedBox(height: rowSpacing),
          SizedBox(
            height: cellHeight,
            child: Row(
              children: [
                for (var col = 0; col < 7; col++) ...[
                  if (col > 0) const SizedBox(width: rowSpacing),
                  Expanded(
                    child: _buildCell(
                      l10n,
                      weeks[i][col],
                      firstDay,
                      lastDay,
                      selected,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ],
    );
  }

  Widget _buildCell(
    AppLocalizations l10n,
    int? dayNum,
    DateTime firstDay,
    DateTime lastDay,
    DateTime selected,
  ) {
    if (dayNum == null) return const SizedBox.shrink();
    final cellDate = DateTime(visibleMonth.year, visibleMonth.month, dayNum);
    final outOfRange =
        cellDate.isBefore(firstDay) || cellDate.isAfter(lastDay);
    final isSelected = cellDate == selected;
    return _DayCell(
      dayNum: dayNum,
      isSelected: isSelected,
      isEnabled: !outOfRange,
      semanticsLabel: l10n.a11yDateTimeModalDayCell(
        DateFormat.yMMMMEEEEd().format(cellDate),
      ),
      onTap: outOfRange ? null : () => onDaySelected(cellDate),
    );
  }
}

class _DayCell extends StatelessWidget {
  final int dayNum;
  final bool isSelected;
  final bool isEnabled;
  final String semanticsLabel;
  final VoidCallback? onTap;

  const _DayCell({
    required this.dayNum,
    required this.isSelected,
    required this.isEnabled,
    required this.semanticsLabel,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final Color textColor;
    if (!isEnabled) {
      textColor = AppColors.modalTextMuted.withValues(alpha: 0.35);
    } else if (isSelected) {
      textColor = AppColors.modalPrimaryButtonText;
    } else {
      textColor = AppColors.modalTextPrimary;
    }
    final body = Container(
      decoration: BoxDecoration(
        color: isSelected
            ? AppColors.modalPrimaryButtonBackground
            : Colors.transparent,
        borderRadius: BorderRadius.circular(8),
      ),
      alignment: Alignment.center,
      child: Text(
        dayNum.toString(),
        style: TextStyle(
          fontSize: 13,
          fontWeight: isSelected ? FontWeight.w800 : FontWeight.w500,
          color: textColor,
        ),
      ),
    );
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: body,
    );
  }
}

/// Hour / minute / AM-PM wheels rendered as a 3-column group.
class TimeWheelGroup extends StatelessWidget {
  final int hour12;
  final int minute;
  final bool isPm;
  final ValueChanged<int> onHourChanged;
  final ValueChanged<int> onMinuteChanged;
  final ValueChanged<bool> onIsPmChanged;

  const TimeWheelGroup({
    super.key,
    required this.hour12,
    required this.minute,
    required this.isPm,
    required this.onHourChanged,
    required this.onMinuteChanged,
    required this.onIsPmChanged,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Container(
      decoration: BoxDecoration(
        color: AppColors.modalChipBackground,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.modalBorderSubtle),
      ),
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: SizedBox(
        height: 120,
        child: Stack(
          children: [
            // Single full-width selection band sitting behind the wheels —
            // matches the reference design's "one prominent pill" pattern.
            // Non-interactive so the wheels themselves still receive scroll.
            Positioned.fill(
              child: Center(
                child: IgnorePointer(
                  child: Container(
                    margin: const EdgeInsets.symmetric(horizontal: 8),
                    height: 38,
                    decoration: BoxDecoration(
                      color: AppColors.modalPrimaryButtonBackground
                          .withValues(alpha: 0.18),
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(
                        color: AppColors.modalPrimaryButtonBackground
                            .withValues(alpha: 0.6),
                        width: 1,
                      ),
                    ),
                  ),
                ),
              ),
            ),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                _WheelColumn<int>(
              semanticsLabel: l10n.a11yDateTimeModalHourWheel,
              announceLabel: (v) =>
                  l10n.a11yDateTimeModalHourValue(v.toString()),
              values: const [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12],
              selectedValue: hour12,
              formatter: (v) => v.toString(),
              onChanged: onHourChanged,
              width: 56,
            ),
            const _WheelDivider(text: ':'),
            _WheelColumn<int>(
              semanticsLabel: l10n.a11yDateTimeModalMinuteWheel,
              announceLabel: (v) => l10n.a11yDateTimeModalMinuteValue(
                v.toString().padLeft(2, '0'),
              ),
              values: const [0, 15, 30, 45],
              selectedValue: minute,
              formatter: (v) => v.toString().padLeft(2, '0'),
              onChanged: onMinuteChanged,
              width: 64,
            ),
            const SizedBox(width: 8),
            _WheelColumn<bool>(
              semanticsLabel: l10n.a11yDateTimeModalPeriodWheel,
              announceLabel: (v) => l10n.a11yDateTimeModalPeriodValue(
                v ? l10n.commonPm : l10n.commonAm,
              ),
              values: const [false, true],
              selectedValue: isPm,
              formatter: (v) => v ? l10n.commonPm : l10n.commonAm,
              onChanged: onIsPmChanged,
              width: 60,
            ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _WheelDivider extends StatelessWidget {
  final String text;
  const _WheelDivider({required this.text});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 2),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 22,
          fontWeight: FontWeight.w700,
          // #2764: was the button FILL token, painted as text on the bare
          // sheet at ~1:1. On-glass text comes from the text ramp.
          color: AppColors.modalTextPrimary,
        ),
      ),
    );
  }
}

class _WheelColumn<T> extends StatefulWidget {
  final String semanticsLabel;
  final String Function(T) announceLabel;
  final List<T> values;
  final T selectedValue;
  final String Function(T) formatter;
  final ValueChanged<T> onChanged;
  final double width;

  const _WheelColumn({
    required this.semanticsLabel,
    required this.announceLabel,
    required this.values,
    required this.selectedValue,
    required this.formatter,
    required this.onChanged,
    required this.width,
  });

  @override
  State<_WheelColumn<T>> createState() => _WheelColumnState<T>();
}

class _WheelColumnState<T> extends State<_WheelColumn<T>> {
  late FixedExtentScrollController _controller;

  @override
  void initState() {
    super.initState();
    final initialIndex = widget.values.indexOf(widget.selectedValue);
    _controller = FixedExtentScrollController(
      initialItem: initialIndex < 0 ? 0 : initialIndex,
    );
  }

  @override
  void didUpdateWidget(covariant _WheelColumn<T> oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selectedValue != widget.selectedValue) {
      final newIndex = widget.values.indexOf(widget.selectedValue);
      if (newIndex >= 0 && _controller.selectedItem != newIndex) {
        _controller.jumpToItem(newIndex);
      }
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _announce(T value) {
    final view = View.maybeOf(context);
    if (view == null) return;
    final dir = Directionality.maybeOf(context) ?? TextDirection.ltr;
    SemanticsService.sendAnnouncement(view, widget.announceLabel(value), dir);
  }

  void _increment() {
    final idx = _controller.selectedItem;
    if (idx + 1 < widget.values.length) {
      _controller.animateToItem(
        idx + 1,
        duration:
            accessibleDuration(context, const Duration(milliseconds: 150)),
        curve: Curves.easeOut,
      );
    }
  }

  void _decrement() {
    final idx = _controller.selectedItem;
    if (idx - 1 >= 0) {
      _controller.animateToItem(
        idx - 1,
        duration:
            accessibleDuration(context, const Duration(milliseconds: 150)),
        curve: Curves.easeOut,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final currentIdx = widget.values.indexOf(widget.selectedValue);
    final value = currentIdx >= 0
        ? widget.values[currentIdx]
        : widget.values.first;
    // Flutter requires that a Semantics node exposing onIncrease/onDecrease
    // provide *all three* of value / increasedValue / decreasedValue (or
    // all three empty). When at an edge of the list, repeat the current
    // value so the strings stay non-empty.
    final nextValue = currentIdx + 1 < widget.values.length
        ? widget.values[currentIdx + 1]
        : value;
    final prevValue = currentIdx - 1 >= 0
        ? widget.values[currentIdx - 1]
        : value;
    return Semantics(
      label: widget.semanticsLabel,
      value: widget.formatter(value),
      increasedValue: widget.formatter(nextValue),
      decreasedValue: widget.formatter(prevValue),
      onIncrease: _increment,
      onDecrease: _decrement,
      child: SizedBox(
        width: widget.width,
        child: ListWheelScrollView.useDelegate(
              controller: _controller,
              physics: const FixedExtentScrollPhysics(),
              itemExtent: 36,
              diameterRatio: 1.6,
              perspective: 0.004,
              onSelectedItemChanged: (index) {
                final v = widget.values[index];
                widget.onChanged(v);
                _announce(v);
              },
              childDelegate: ListWheelChildBuilderDelegate(
                childCount: widget.values.length,
                builder: (context, index) {
                  final v = widget.values[index];
                  final isCenter = v == widget.selectedValue;
                  return Center(
                    child: Text(
                      widget.formatter(v),
                      style: TextStyle(
                        fontFeatures: const [FontFeature.tabularFigures()],
                        fontSize: isCenter ? 22 : 18,
                        fontWeight:
                            isCenter ? FontWeight.w700 : FontWeight.w400,
                        color: AppColors.modalTextPrimary,
                      ),
                    ),
                  );
                },
              ),
        ),
      ),
    );
  }
}
