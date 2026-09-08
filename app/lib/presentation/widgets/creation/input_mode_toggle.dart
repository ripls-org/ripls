import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// Defines the input mode for creation flows.
enum CreationInputMode { text, image }

/// Toggle for switching between input modes (text/link, image).
///
/// Used in creation modals for gear, communities, requests, and experiences.
/// Design based on metrics screen toggle pattern.
///
/// The text mode supports both text descriptions and pasted URLs (auto-detected).
///
/// Example:
/// ```dart
/// InputModeToggle(
///   currentMode: state.inputMode,
///   onModeChanged: (mode) => notifier.setInputMode(mode),
///   isLoading: state.isLoading,
///   textModeEnabled: true,  // Set to false to disable text mode
///   textModeLabel: 'Text / Link',  // Custom label for text mode
/// )
/// ```
class InputModeToggle extends StatelessWidget {
  final CreationInputMode currentMode;
  final Function(CreationInputMode) onModeChanged;
  final bool isLoading;
  final bool textModeEnabled;
  final String textModeLabel;

  const InputModeToggle({
    super.key,
    required this.currentMode,
    required this.onModeChanged,
    this.isLoading = false,
    this.textModeEnabled = true,
    this.textModeLabel = 'Text',
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 40,
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: AppColors.divider(context), width: 1),
      ),
      child: Row(
        children: [
          _buildToggleButton(
            context: context,
            label: textModeLabel,
            mode: CreationInputMode.text,
          ),
          _buildToggleButton(
            context: context,
            label: context.l10n.a11yInputModeImage,
            mode: CreationInputMode.image,
          ),
        ],
      ),
    );
  }

  Widget _buildToggleButton({
    required BuildContext context,
    required String label,
    required CreationInputMode mode,
  }) {
    final isSelected = currentMode == mode;
    final isModeEnabled = mode == CreationInputMode.text ? textModeEnabled : true;
    final isEnabled = !isLoading && isModeEnabled;

    return Expanded(
      child: Toggle(
        semanticsLabel: label,
        selected: isSelected,
        onTap: isEnabled
            ? () {
                HapticFeedback.selectionClick();
                onModeChanged(mode);
              }
            : null,
        child: Container(
          height: 32,
          margin: const EdgeInsets.all(4),
          decoration: BoxDecoration(
            color: isSelected ? AppColors.primary(context) : Colors.transparent,
            borderRadius: BorderRadius.circular(16),
          ),
          child: Center(
            child: Text(
              label,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: isSelected
                    ? Colors.white
                    : isModeEnabled
                        ? AppColors.textSecondary(context)
                        : AppColors.textSecondary(context).withValues(alpha: 0.3),
                fontSize: 12,
                fontWeight: isSelected ? FontWeight.w700 : FontWeight.w500,
                letterSpacing: 0.3,
              ),
            ),
          ),
        ),
      ),
    );
  }
}
