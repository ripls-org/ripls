import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// QuickDateChips provides quick date selection options.
///
/// Re-skinned to glass: each preset (Today / Tomorrow / This weekend)
/// is a [GlassChip] with primary label + secondary date text. The
/// "Pick another date…" fall-through is a [GlassInlineAction].
class QuickDateChips extends StatelessWidget {
  /// Currently selected date
  final DateTime? selectedDate;

  /// Callback when a date is selected
  final ValueChanged<DateTime> onDateSelected;

  /// Optional label to display above the chips
  final String? label;

  const QuickDateChips({
    super.key,
    this.selectedDate,
    required this.onDateSelected,
    this.label,
  });

  @override
  Widget build(BuildContext context) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final tomorrow = today.add(const Duration(days: 1));
    final thisWeekend = _getNextWeekend(today);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (label != null) ...[
          Padding(
            padding: const EdgeInsets.only(left: 16),
            child: GlassFieldLabel(text: label!),
          ),
        ],
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Column(
            children: [
              Row(
                children: [
                  Expanded(
                    child: GlassChip(
                      primary: 'Today',
                      secondary: DateFormat('MMM d').format(today),
                      selected: _isSameDay(selectedDate, today),
                      onTap: () => onDateSelected(today),
                      semanticsLabel: context.l10n.a11yLabelDateTime(
                        'Today',
                        DateFormat('MMM d').format(today),
                      ),
                    ),
                  ),
                  const SizedBox(width: 7),
                  Expanded(
                    child: GlassChip(
                      primary: 'Tomorrow',
                      secondary: DateFormat('MMM d').format(tomorrow),
                      selected: _isSameDay(selectedDate, tomorrow),
                      onTap: () => onDateSelected(tomorrow),
                      semanticsLabel: context.l10n.a11yLabelDateTime(
                        'Tomorrow',
                        DateFormat('MMM d').format(tomorrow),
                      ),
                    ),
                  ),
                  const SizedBox(width: 7),
                  Expanded(
                    child: GlassChip(
                      primary: 'This weekend',
                      secondary: DateFormat('MMM d').format(thisWeekend),
                      selected: _isSameDay(selectedDate, thisWeekend),
                      onTap: () => onDateSelected(thisWeekend),
                      semanticsLabel: context.l10n.a11yLabelDateTime(
                        'This weekend',
                        DateFormat('MMM d').format(thisWeekend),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              GlassInlineAction(
                icon: Icons.calendar_today,
                text: selectedDate != null
                    ? DateFormat('EEEE, MMMM d, yyyy').format(selectedDate!)
                    : 'Select a date',
                semanticsLabel: selectedDate != null
                    ? DateFormat('EEEE, MMMM d, yyyy').format(selectedDate!)
                    : 'Select a date',
                onTap: () => _showDatePicker(context),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Future<void> _showDatePicker(BuildContext context) async {
    final now = DateTime.now();
    final firstDate = DateTime(now.year, now.month, now.day);
    final lastDate = firstDate.add(const Duration(days: 365));

    final pickedDate = await showGlassDatePicker(
      context: context,
      initialDate: selectedDate ?? firstDate,
      firstDate: firstDate,
      lastDate: lastDate,
    );

    if (pickedDate != null) {
      onDateSelected(pickedDate);
    }
  }

  bool _isSameDay(DateTime? date1, DateTime? date2) {
    if (date1 == null || date2 == null) return false;
    return date1.year == date2.year &&
        date1.month == date2.month &&
        date1.day == date2.day;
  }

  DateTime _getNextWeekend(DateTime from) {
    // Returns next Saturday
    final daysUntilSaturday = (DateTime.saturday - from.weekday) % 7;
    if (daysUntilSaturday == 0) {
      // If today is Saturday, return next Saturday
      return from.add(const Duration(days: 7));
    }
    return from.add(Duration(days: daysUntilSaturday));
  }
}
