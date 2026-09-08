import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/experience/duration_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_pickers.dart';

/// Form for proposing a new time slot for an experience.
///
/// Renders date / start-time / duration chip rows and fall-through inline
/// action rows ("Select a date", "Select a time", "Select duration") using
/// the modal-glass palette. There is one styling for this form across the
/// app — callers that don't have a glass surface still get the glass look
/// (per docs/issues/1797-glass-modal-revamp.md: never fall back to Material
/// default pickers, even when the parent surface is being redesigned
/// separately).
///
/// The widget uses [StatefulWidget] for local form input state — see the
/// "widget async exceptions" section of [docs/client/architecture.md].
class ProposeTimeForm extends StatefulWidget {
  const ProposeTimeForm({
    super.key,
    required this.onDateChanged,
    required this.onTimeChanged,
    required this.onDurationChanged,
    this.selectedDate,
    this.selectedTime,
    this.selectedDuration,
    this.existingDate,
    this.existingTime,
    this.existingDuration,
    this.allowPastDates = false,
  });

  /// Callback when date changes.
  final ValueChanged<DateTime> onDateChanged;

  /// Callback when time changes.
  final ValueChanged<TimeOfDay> onTimeChanged;

  /// Callback when duration changes.
  final ValueChanged<int> onDurationChanged;

  /// Currently selected date.
  final DateTime? selectedDate;

  /// Currently selected time.
  final TimeOfDay? selectedTime;

  /// Currently selected duration in minutes.
  final int? selectedDuration;

  /// Existing event date (for smart chip defaults).
  final DateTime? existingDate;

  /// Existing event time (for smart chip defaults).
  final TimeOfDay? existingTime;

  /// Existing event duration in minutes (for smart chip defaults).
  final int? existingDuration;

  /// Whether to allow selecting dates in the past.
  final bool allowPastDates;

  @override
  State<ProposeTimeForm> createState() => _ProposeTimeFormState();
}

class _ProposeTimeFormState extends State<ProposeTimeForm> {
  // Widget async exception: Form input state managed locally
  DateTime? _selectedDate;
  TimeOfDay? _selectedTime;
  int? _selectedDuration;

  @override
  void initState() {
    super.initState();
    _selectedDate = widget.selectedDate;
    _selectedTime = widget.selectedTime;
    _selectedDuration = widget.selectedDuration;
  }

  @override
  Widget build(BuildContext context) {
    final now = DateTime.now();

    final dateChips = widget.existingDate != null
        ? _buildDateChipsFromExisting(widget.existingDate!)
        : _buildDefaultDateChips(now);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _SectionLabel(text: 'DATE'),
        const SizedBox(height: 10),
        Row(
          children: [
            for (int i = 0; i < dateChips.length; i++) ...[
              if (i > 0) const SizedBox(width: 8),
              Expanded(
                child: _ChipWithSubtitle(
                  label: dateChips[i].label,
                  subtitle: dateChips[i].sub,
                  isSelected: _isSameDay(_selectedDate, dateChips[i].date),
                  onTap: () => _onDateSelected(dateChips[i].date),
                ),
              ),
            ],
          ],
        ),
        const SizedBox(height: 10),
        _FieldRow(
          text: _selectedDate != null
              ? DateFormat('MMMM d, yyyy').format(_selectedDate!)
              : 'Select a date',
          icon: Icons.calendar_today,
          hasValue: _selectedDate != null,
          onTap: _showDatePicker,
        ),
        const SizedBox(height: 22),
        _SectionLabel(text: 'START TIME'),
        const SizedBox(height: 10),
        Builder(
          builder: (context) {
            final timeChips = widget.existingTime != null
                ? _buildTimeChipsFromExisting(widget.existingTime!)
                : _buildDefaultTimeChips();

            return Row(
              children: [
                for (int i = 0; i < timeChips.length; i++) ...[
                  if (i > 0) const SizedBox(width: 8),
                  Expanded(
                    child: _ChipWithSubtitle(
                      label: timeChips[i].label,
                      subtitle: timeChips[i].sub,
                      isSelected: _selectedTime == timeChips[i].time,
                      onTap: () => _onTimeSelected(timeChips[i].time),
                    ),
                  ),
                ],
              ],
            );
          },
        ),
        const SizedBox(height: 10),
        _FieldRow(
          text: _selectedTime != null
              ? _selectedTime!.format(context)
              : 'Select a time',
          icon: Icons.access_time,
          hasValue: _selectedTime != null,
          onTap: _showTimePicker,
        ),
        const SizedBox(height: 22),
        _SectionLabel(text: 'DURATION'),
        const SizedBox(height: 10),
        Builder(
          builder: (context) {
            final durationChips = widget.existingDuration != null
                ? _buildDurationChipsFromExisting(widget.existingDuration!)
                : _buildDefaultDurationChips();

            return Row(
              children: [
                for (int i = 0; i < durationChips.length; i++) ...[
                  if (i > 0) const SizedBox(width: 8),
                  Expanded(
                    child: _SimpleChip(
                      label: durationChips[i].label,
                      isSelected:
                          _selectedDuration == durationChips[i].minutes,
                      onTap: () =>
                          _onDurationSelected(durationChips[i].minutes),
                    ),
                  ),
                ],
              ],
            );
          },
        ),
        const SizedBox(height: 10),
        _FieldRow(
          text: _selectedDuration != null
              ? _formatDuration(_selectedDuration!)
              : 'Select duration',
          icon: Icons.timelapse,
          hasValue: _selectedDuration != null,
          onTap: _showDurationPicker,
        ),
      ],
    );
  }

  /// Formats duration in minutes as human-readable string.
  String _formatDuration(int minutes) {
    if (minutes >= 60) {
      final hours = minutes ~/ 60;
      if (minutes % 60 == 0) {
        return '$hours hr${hours == 1 ? '' : 's'}';
      } else {
        final remainingMinutes = minutes % 60;
        return '$hours hr${hours == 1 ? '' : 's'} $remainingMinutes min';
      }
    }
    return '$minutes min';
  }

  void _onDateSelected(DateTime date) {
    setState(() {
      _selectedDate = date;
    });
    widget.onDateChanged(date);
  }

  void _onTimeSelected(TimeOfDay time) {
    setState(() {
      _selectedTime = time;
    });
    widget.onTimeChanged(time);
  }

  void _onDurationSelected(int minutes) {
    setState(() {
      _selectedDuration = minutes;
    });
    widget.onDurationChanged(minutes);
  }

  /// Widget async exception: showGlassDatePicker opens system date picker.
  Future<void> _showDatePicker() async {
    final now = DateTime.now();
    final firstDate =
        widget.allowPastDates ? DateTime(now.year - 10) : now;
    final picked = await showGlassDatePicker(
      context: context,
      initialDate: _selectedDate ??
          (widget.allowPastDates ? now : now.add(const Duration(days: 1))),
      firstDate: firstDate,
      lastDate: now.add(const Duration(days: 365)),
    );
    if (picked != null) {
      _onDateSelected(picked);
    }
  }

  /// Widget async exception: showDialog opens duration picker modal.
  Future<void> _showDurationPicker() async {
    final picked = await DurationPickerModal.show(
      context,
      initialDurationMinutes: _selectedDuration ?? 60,
    );
    if (picked != null) {
      _onDurationSelected(picked);
    }
  }

  /// Widget async exception: showGlassTimePicker opens system time picker.
  Future<void> _showTimePicker() async {
    final picked = await showGlassTimePicker(
      context: context,
      initialTime: _selectedTime ?? const TimeOfDay(hour: 8, minute: 0),
    );
    if (picked != null) {
      _onTimeSelected(picked);
    }
  }

  DateTime _getNextWeekday(DateTime from, int targetWeekday) {
    final daysUntil = (targetWeekday - from.weekday + 7) % 7;
    return from.add(Duration(days: daysUntil == 0 ? 7 : daysUntil));
  }

  bool _isSameDay(DateTime? a, DateTime? b) {
    if (a == null || b == null) return false;
    return a.year == b.year && a.month == b.month && a.day == b.day;
  }

  /// Builds date chips relative to the existing event date.
  List<_DateChip> _buildDateChipsFromExisting(DateTime existing) {
    final chips = <_DateChip>[];

    chips.add(_DateChip(
      label: DateFormat('EEE').format(existing),
      sub: DateFormat('MMM d').format(existing),
      date: existing,
    ));

    final plusOne = existing.add(const Duration(days: 1));
    chips.add(_DateChip(
      label: DateFormat('EEE').format(plusOne),
      sub: DateFormat('MMM d').format(plusOne),
      date: plusOne,
    ));

    var weekend = _getNextWeekday(existing, DateTime.saturday);
    if (_isSameDay(weekend, existing) || _isSameDay(weekend, plusOne)) {
      weekend = _getNextWeekday(existing, DateTime.sunday);
    }
    if (_isSameDay(weekend, existing) || _isSameDay(weekend, plusOne)) {
      weekend = _getNextWeekday(
        existing.add(const Duration(days: 7)),
        DateTime.saturday,
      );
    }
    final isThisWeekend = weekend.difference(DateTime.now()).inDays <= 7;
    chips.add(_DateChip(
      label:
          '${isThisWeekend ? "This" : "Next"} ${DateFormat('EEE').format(weekend)}',
      sub: DateFormat('MMM d').format(weekend),
      date: weekend,
    ));

    return chips;
  }

  /// Builds time chips relative to the existing event time.
  List<_TimeChip> _buildTimeChipsFromExisting(TimeOfDay existing) {
    final chips = <_TimeChip>[];

    chips.add(_TimeChip(
      label: _formatTimeLabel(existing),
      sub: _formatTimeSub(existing),
      time: existing,
    ));

    var afternoon = const TimeOfDay(hour: 14, minute: 0);
    if (afternoon == existing) {
      afternoon = const TimeOfDay(hour: 15, minute: 0);
    }
    chips.add(_TimeChip(
      label: afternoon.hour == 15 ? 'Late afternoon' : 'Afternoon',
      sub: _formatTimeSub(afternoon),
      time: afternoon,
    ));

    var evening = const TimeOfDay(hour: 17, minute: 0);
    if (evening == existing) {
      evening = const TimeOfDay(hour: 19, minute: 0);
    }
    chips.add(_TimeChip(
      label: evening.hour == 19 ? 'Night' : 'Evening',
      sub: _formatTimeSub(evening),
      time: evening,
    ));

    return chips;
  }

  /// Builds duration chips relative to the existing duration.
  List<_DurationChip> _buildDurationChipsFromExisting(int existingMinutes) {
    final chips = <_DurationChip>[];

    chips.add(
      _DurationChip(
          label: _formatDuration(existingMinutes), minutes: existingMinutes),
    );

    var oneHr = 60;
    if (oneHr == existingMinutes) oneHr = 30;
    chips.add(_DurationChip(label: _formatDuration(oneHr), minutes: oneHr));

    var twoHrs = 120;
    if (twoHrs == existingMinutes) twoHrs = 180;
    chips.add(_DurationChip(label: _formatDuration(twoHrs), minutes: twoHrs));

    return chips;
  }

  /// Builds default date chips (when no existing date).
  List<_DateChip> _buildDefaultDateChips(DateTime now) {
    final tomorrow = now.add(const Duration(days: 1));
    final thisSat = _getNextWeekday(now, DateTime.saturday);
    final nextSun = _getNextWeekday(now, DateTime.sunday);

    return [
      _DateChip(
        label: 'Tomorrow',
        sub: DateFormat('MMM d').format(tomorrow),
        date: tomorrow,
      ),
      _DateChip(
        label: 'This ${DateFormat('EEE').format(thisSat)}',
        sub: DateFormat('MMM d').format(thisSat),
        date: thisSat,
      ),
      _DateChip(
        label: 'Next ${DateFormat('EEE').format(nextSun)}',
        sub: DateFormat('MMM d').format(nextSun),
        date: nextSun,
      ),
    ];
  }

  /// Builds default time chips (when no existing time).
  List<_TimeChip> _buildDefaultTimeChips() {
    return const [
      _TimeChip(
          label: 'Morning', sub: '8 AM', time: TimeOfDay(hour: 8, minute: 0)),
      _TimeChip(
          label: 'Midday', sub: '12 PM', time: TimeOfDay(hour: 12, minute: 0)),
      _TimeChip(
          label: 'Evening', sub: '5 PM', time: TimeOfDay(hour: 17, minute: 0)),
    ];
  }

  /// Builds default duration chips (when no existing duration).
  List<_DurationChip> _buildDefaultDurationChips() {
    return const [
      _DurationChip(label: '1 hr', minutes: 60),
      _DurationChip(label: '2 hrs', minutes: 120),
      _DurationChip(label: 'All day', minutes: 480),
    ];
  }

  String _formatTimeLabel(TimeOfDay time) {
    final hour = time.hourOfPeriod == 0 ? 12 : time.hourOfPeriod;
    final period = time.period == DayPeriod.am ? 'AM' : 'PM';
    return '$hour:${time.minute.toString().padLeft(2, '0')} $period';
  }

  String _formatTimeSub(TimeOfDay time) {
    final hour = time.hourOfPeriod == 0 ? 12 : time.hourOfPeriod;
    final period = time.period == DayPeriod.am ? 'AM' : 'PM';
    return '$hour $period';
  }
}

/// Helper class for date chip data
class _DateChip {
  const _DateChip({
    required this.label,
    required this.sub,
    required this.date,
  });

  final String label;
  final String sub;
  final DateTime date;
}

/// Helper class for time chip data
class _TimeChip {
  const _TimeChip({
    required this.label,
    required this.sub,
    required this.time,
  });

  final String label;
  final String sub;
  final TimeOfDay time;
}

/// Helper class for duration chip data
class _DurationChip {
  const _DurationChip({
    required this.label,
    required this.minutes,
  });

  final String label;
  final int minutes;
}

/// Section label ("DATE", "START TIME", "DURATION").
class _SectionLabel extends StatelessWidget {
  const _SectionLabel({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(left: 4),
      child: Text(
        text,
        style: ModalTheme.fieldLabelStyle.copyWith(
          color: AppColors.modalTextTertiary,
        ),
      ),
    );
  }
}

/// Chip with a main label and subtitle (used for date and time chips).
class _ChipWithSubtitle extends StatelessWidget {
  const _ChipWithSubtitle({
    required this.label,
    required this.subtitle,
    required this.isSelected,
    required this.onTap,
  });

  final String label;
  final String subtitle;
  final bool isSelected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final bg = isSelected
        ? AppColors.modalChipBackgroundActive
        : AppColors.modalChipBackground;
    final border = isSelected
        ? AppColors.modalChipBorderActive
        : AppColors.modalChipBorder;
    final labelColor = isSelected
        ? AppColors.modalChipTextActive
        : AppColors.modalChipText;
    final subtitleColor = isSelected
        ? AppColors.modalChipTextSecondaryActive
        : AppColors.modalChipTextSecondary;

    return Toggle(
      semanticsLabel: context.l10n.a11yLabelDescription(label, subtitle),
      selected: isSelected,
      onTap: onTap,
      child: Container(
        padding:
            const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(ModalTheme.chipRadius),
          border: Border.all(color: border),
        ),
        child: Column(
          children: [
            Text(
              label,
              style: ModalTheme.chipPrimaryStyle.copyWith(color: labelColor),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            const SizedBox(height: 2),
            Text(
              subtitle,
              style:
                  ModalTheme.chipSecondaryStyle.copyWith(color: subtitleColor),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ],
        ),
      ),
    );
  }
}

/// Simple chip without subtitle (used for duration chips).
class _SimpleChip extends StatelessWidget {
  const _SimpleChip({
    required this.label,
    required this.isSelected,
    required this.onTap,
  });

  final String label;
  final bool isSelected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final bg = isSelected
        ? AppColors.modalChipBackgroundActive
        : AppColors.modalChipBackground;
    final border = isSelected
        ? AppColors.modalChipBorderActive
        : AppColors.modalChipBorder;
    final labelColor = isSelected
        ? AppColors.modalChipTextActive
        : AppColors.modalChipText;

    return Toggle(
      semanticsLabel: label,
      selected: isSelected,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(ModalTheme.chipRadius),
          border: Border.all(color: border),
        ),
        child: Center(
          child: Text(
            label,
            style: ModalTheme.chipPrimaryStyle.copyWith(color: labelColor),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ),
    );
  }
}

/// Full-width tappable field row showing selected value with trailing icon.
class _FieldRow extends StatelessWidget {
  const _FieldRow({
    required this.text,
    required this.icon,
    required this.hasValue,
    required this.onTap,
  });

  final String text;
  final IconData icon;
  final bool hasValue;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final textColor = hasValue
        ? AppColors.modalInlineActionText
        : AppColors.modalTextMuted;
    return Tappable(
      semanticsLabel: text,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
      child: Container(
        height: ModalTheme.inlineActionHeight,
        padding: const EdgeInsets.symmetric(horizontal: 14),
        decoration: BoxDecoration(
          color: AppColors.modalInlineActionBackground,
          borderRadius:
              BorderRadius.circular(ModalTheme.inlineActionRadius),
          border: Border.all(color: AppColors.modalInlineActionBorder),
        ),
        child: Row(
          children: [
            Icon(icon, size: 16, color: AppColors.modalTextMuted),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                text,
                style: ModalTheme.inlineActionTextStyle.copyWith(
                  color: textColor,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            Icon(
              Icons.chevron_right,
              size: 18,
              color: AppColors.modalInlineActionChevron,
            ),
          ],
        ),
      ),
    );
  }
}
