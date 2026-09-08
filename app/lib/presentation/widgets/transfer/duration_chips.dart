import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// DurationChips provides quick loan duration selection options.
///
/// Re-skinned to glass: each preset (1 day / 3 days / 1 week / 2 weeks)
/// is a single-line [GlassChip].
class DurationChips extends StatelessWidget {
  /// Currently selected duration in days
  final int? selectedDurationDays;

  /// Callback when a duration is selected (in days)
  final ValueChanged<int> onDurationSelected;

  /// Optional label to display above the chips
  final String? label;

  const DurationChips({
    super.key,
    this.selectedDurationDays,
    required this.onDurationSelected,
    this.label,
  });

  static const _durations = [
    _DurationOption(days: 1, label: '1 day'),
    _DurationOption(days: 3, label: '3 days'),
    _DurationOption(days: 7, label: '1 week'),
    _DurationOption(days: 14, label: '2 weeks'),
  ];

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
                  for (int i = 0; i < _durations.length; i++) ...[
                    if (i > 0) const SizedBox(width: 7),
                    Expanded(
                      child: GlassChip(
                        primary: _durations[i].label,
                        selected:
                            selectedDurationDays == _durations[i].days,
                        onTap: () => onDurationSelected(_durations[i].days),
                        semanticsLabel: _durations[i].label,
                      ),
                    ),
                  ],
                ],
              ),
              const SizedBox(height: 8),
              Text(
                'You can request an extension later',
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w400,
                  color: AppColors.modalTextMuted,
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _DurationOption {
  final int days;
  final String label;

  const _DurationOption({required this.days, required this.label});
}
