import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// Duration picker modal with wheel pickers for days, hours, and minutes.
///
/// Opens as a centered glass dialog. Returns the selected duration in
/// minutes via the [onConfirm] callback, or null if cancelled.
///
/// Visually paired with [showGlassDatePicker] / [showGlassTimePicker]:
/// translucent white surface on a blurred dark scrim, white-on-glass text,
/// coral primary CTA. The days column is hidden by default and toggled via
/// a "Days" button.
class DurationPickerModal extends StatefulWidget {
  const DurationPickerModal({
    super.key,
    required this.initialDurationMinutes,
    required this.onConfirm,
  });

  final int initialDurationMinutes;
  final ValueChanged<int> onConfirm;

  /// Shows the duration picker as a dialog and returns the selected
  /// duration in minutes, or null if cancelled.
  ///
  /// Widget async exception: showDialog opens platform dialog.
  static Future<int?> show(
    BuildContext context, {
    int initialDurationMinutes = 60,
  }) async {
    int? result;
    await showDialog(
      context: context,
      // We paint our own blurred scrim inside the dialog body so the
      // backdrop matches the date / time pickers exactly.
      barrierColor: Colors.transparent,
      builder: (context) => DurationPickerModal(
        initialDurationMinutes: initialDurationMinutes,
        onConfirm: (minutes) => result = minutes,
      ),
    );
    return result;
  }

  @override
  State<DurationPickerModal> createState() => _DurationPickerModalState();
}

class _DurationPickerModalState extends State<DurationPickerModal> {
  late int days;
  late int hours;
  late int minutes;
  bool showDays = false;

  @override
  void initState() {
    super.initState();
    final totalMinutes = widget.initialDurationMinutes;
    days = totalMinutes ~/ (24 * 60);
    hours = (totalMinutes % (24 * 60)) ~/ 60;
    minutes = totalMinutes % 60;

    if (days > 0) {
      showDays = true;
    }
  }

  int get totalMinutes => (days * 24 * 60) + (hours * 60) + minutes;

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        // Full-screen lightly-blurred dark scrim — matches the time modal's
        // outer scrim (the area above the sheet).
        Positioned.fill(
          child: BackdropFilter(
            filter: ui.ImageFilter.blur(
              sigmaX: AppColors.modalBackdropBlurSigma,
              sigmaY: AppColors.modalBackdropBlurSigma,
            ),
            child: Container(color: AppColors.modalBackdrop),
          ),
        ),
        // Centered frosted-glass dialog. ClipRRect + inner BackdropFilter
        // mirrors GlassSurface so the dialog reads as the same frosted
        // material as the time modal's sheet — heavy blur within the
        // dialog's bounds + a 15%-white tint on top.
        Dialog(
          backgroundColor: Colors.transparent,
          insetPadding: const EdgeInsets.symmetric(
            horizontal: 32,
            vertical: 24,
          ),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(24),
            child: BackdropFilter(
              filter: ui.ImageFilter.blur(
                sigmaX: AppColors.modalSurfaceBlurSigma,
                sigmaY: AppColors.modalSurfaceBlurSigma,
              ),
              child: Container(
                decoration: BoxDecoration(
                  color: AppColors.modalSurface,
                  borderRadius: BorderRadius.circular(24),
                  border: Border.all(color: AppColors.modalBorder),
                ),
                padding: const EdgeInsets.all(24),
                child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  context.l10n.timeDurationTitle,
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                        color: AppColors.modalTextPrimary,
                        fontWeight: FontWeight.w400,
                      ),
                ),
                const SizedBox(height: 24),
                SizedBox(
                  height: 220,
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      if (showDays) ...[
                        _WheelPicker(
                          label: context.l10n.timeDurationDays,
                          values: List.generate(31, (i) => i),
                          selectedValue: days,
                          onChanged: (value) =>
                              setState(() => days = value),
                        ),
                        const SizedBox(width: 16),
                      ],
                      _WheelPicker(
                        label: context.l10n.timeDurationHours,
                        values: List.generate(24, (i) => i),
                        selectedValue: hours,
                        onChanged: (value) =>
                            setState(() => hours = value),
                      ),
                      const SizedBox(width: 16),
                      _WheelPicker(
                        label: context.l10n.timeDurationMinutes,
                        values: List.generate(60, (i) => i),
                        selectedValue: minutes,
                        onChanged: (value) =>
                            setState(() => minutes = value),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 24),
                Row(
                  children: [
                    _DaysToggle(
                      enabled: showDays,
                      label: context.l10n.timeDurationDays,
                      onTap: () => setState(() {
                        showDays = !showDays;
                        if (!showDays) {
                          days = 0;
                        } else if (days == 0) {
                          days = 1;
                          hours = 0;
                          minutes = 0;
                        }
                      }),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _SecondaryActionButton(
                        label: context.l10n.commonCancel,
                        onTap: () => Navigator.of(context).pop(),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _PrimaryActionButton(
                        label: context.l10n.commonConfirm,
                        onTap: () {
                          widget.onConfirm(totalMinutes);
                          Navigator.of(context).pop();
                        },
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
            ),
          ),
        ),
      ],
    );
  }
}

class _DaysToggle extends StatelessWidget {
  const _DaysToggle({
    required this.enabled,
    required this.label,
    required this.onTap,
  });

  final bool enabled;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final bg = enabled
        ? AppColors.modalChipBackgroundActive
        : AppColors.modalChipBackground;
    final border = enabled
        ? AppColors.modalChipBorderActive
        : AppColors.modalChipBorder;
    final labelColor = enabled
        ? AppColors.modalChipTextActive
        : AppColors.modalChipText;
    return TextButton(
      onPressed: onTap,
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        backgroundColor: bg,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(14),
          side: BorderSide(color: border),
        ),
      ),
      child: Text(
        label,
        style: TextStyle(color: labelColor, fontSize: 14),
      ),
    );
  }
}

class _SecondaryActionButton extends StatelessWidget {
  const _SecondaryActionButton({required this.label, required this.onTap});

  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return TextButton(
      onPressed: onTap,
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(vertical: 14),
        backgroundColor: AppColors.modalSecondaryButtonBackground,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
          side: BorderSide(color: AppColors.modalSecondaryButtonBorder),
        ),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: AppColors.modalSecondaryButtonText,
          fontSize: 14,
        ),
      ),
    );
  }
}

class _PrimaryActionButton extends StatelessWidget {
  const _PrimaryActionButton({required this.label, required this.onTap});

  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ElevatedButton(
      onPressed: onTap,
      style: ElevatedButton.styleFrom(
        padding: const EdgeInsets.symmetric(vertical: 14),
        backgroundColor: AppColors.modalPrimaryButtonBackground,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
        ),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: AppColors.modalPrimaryButtonText,
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

/// Wheel picker widget for selecting numeric values.
class _WheelPicker extends StatelessWidget {
  const _WheelPicker({
    required this.label,
    required this.values,
    required this.selectedValue,
    required this.onChanged,
  });

  final String label;
  final List<int> values;
  final int selectedValue;
  final ValueChanged<int> onChanged;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 11,
            letterSpacing: 2,
            color: AppColors.modalTextMuted,
            fontWeight: FontWeight.w500,
          ),
        ),
        const SizedBox(height: 12),
        Expanded(
          child: Stack(
            alignment: Alignment.center,
            children: [
              Container(
                height: 48,
                decoration: BoxDecoration(
                  color: AppColors.modalChipBackground,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: AppColors.modalChipBorder),
                ),
              ),
              SizedBox(
                width: 65,
                child: ListWheelScrollView.useDelegate(
                  itemExtent: 48,
                  diameterRatio: 1.5,
                  physics: const FixedExtentScrollPhysics(),
                  onSelectedItemChanged: (index) => onChanged(values[index]),
                  controller: FixedExtentScrollController(
                    initialItem: values.indexOf(selectedValue),
                  ),
                  childDelegate: ListWheelChildBuilderDelegate(
                    childCount: values.length,
                    builder: (context, index) {
                      final value = values[index];
                      final isSelected = value == selectedValue;
                      return Center(
                        child: Text(
                          value.toString().padLeft(2, '0'),
                          style: TextStyle(
                            fontSize: isSelected ? 26 : 20,
                            fontWeight: isSelected
                                ? FontWeight.w400
                                : FontWeight.w300,
                            color: isSelected
                                ? AppColors.modalTextPrimary
                                : AppColors.modalTextMuted,
                          ),
                        ),
                      );
                    },
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
