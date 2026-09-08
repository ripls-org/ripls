import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// QuickTimeChips provides quick time selection options.
///
/// Re-skinned to glass: each preset (Morning / Afternoon / Evening) is a
/// [GlassChip] with primary label + secondary time text. The
/// "Pick another time…" fall-through is a [GlassInlineAction].
class QuickTimeChips extends StatelessWidget {
  /// Currently selected time of day
  final TimeOfDay? selectedTime;

  /// Callback when a time is selected
  final ValueChanged<TimeOfDay> onTimeSelected;

  /// Optional label to display above the chips
  final String? label;

  const QuickTimeChips({
    super.key,
    this.selectedTime,
    required this.onTimeSelected,
    this.label,
  });

  static const _morningTime = TimeOfDay(hour: 9, minute: 0); // 9 AM
  static const _afternoonTime = TimeOfDay(hour: 13, minute: 0); // 1 PM
  static const _eveningTime = TimeOfDay(hour: 18, minute: 0); // 6 PM

  @override
  Widget build(BuildContext context) {
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
                      primary: 'Morning',
                      secondary: '9 AM',
                      selected: _isSameTime(selectedTime, _morningTime),
                      onTap: () => onTimeSelected(_morningTime),
                      semanticsLabel:
                          context.l10n.a11yLabelDateTime('Morning', '9 AM'),
                    ),
                  ),
                  const SizedBox(width: 7),
                  Expanded(
                    child: GlassChip(
                      primary: 'Afternoon',
                      secondary: '1 PM',
                      selected: _isSameTime(selectedTime, _afternoonTime),
                      onTap: () => onTimeSelected(_afternoonTime),
                      semanticsLabel:
                          context.l10n.a11yLabelDateTime('Afternoon', '1 PM'),
                    ),
                  ),
                  const SizedBox(width: 7),
                  Expanded(
                    child: GlassChip(
                      primary: 'Evening',
                      secondary: '6 PM',
                      selected: _isSameTime(selectedTime, _eveningTime),
                      onTap: () => onTimeSelected(_eveningTime),
                      semanticsLabel:
                          context.l10n.a11yLabelDateTime('Evening', '6 PM'),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              GlassInlineAction(
                icon: Icons.access_time,
                text: selectedTime != null
                    ? selectedTime!.format(context)
                    : 'Select a time',
                semanticsLabel: selectedTime != null
                    ? selectedTime!.format(context)
                    : 'Select a time',
                onTap: () => _showTimePicker(context),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Future<void> _showTimePicker(BuildContext context) async {
    final pickedTime = await showGlassTimePicker(
      context: context,
      initialTime: selectedTime ?? TimeOfDay.now(),
    );

    if (pickedTime != null) {
      onTimeSelected(pickedTime);
    }
  }

  bool _isSameTime(TimeOfDay? time1, TimeOfDay? time2) {
    if (time1 == null || time2 == null) return false;
    return time1.hour == time2.hour && time1.minute == time2.minute;
  }
}
