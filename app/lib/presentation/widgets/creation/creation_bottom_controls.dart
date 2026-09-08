import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';

/// Shared bottom controls for creation modals.
///
/// Provides consistent UI for:
/// - Text/Image mode toggle
/// - Generate button with loading state
/// - Manual creation button (icon-only)
///
/// Used across gear, request, experience, and community creation modals.
///
/// Example:
/// ```dart
/// CreationBottomControls(
///   inputMode: state.inputMode,
///   onModeChanged: (mode) => notifier.setInputMode(mode),
///   isLoading: state.isLoading,
///   canGenerate: canGenerate,
///   onGenerate: _handleGenerate,
///   onManualCreate: _handleManualCreate,
///   buttonLabel: 'Create Request',
///   textModeEnabled: true,
///   textModeLabel: 'Text / Link',
/// )
/// ```
class CreationBottomControls extends StatelessWidget {
  final CreationInputMode inputMode;
  final Function(CreationInputMode) onModeChanged;
  final bool isLoading;
  final bool canGenerate;
  final VoidCallback onGenerate;
  final VoidCallback? onManualCreate;
  final String buttonLabel;
  final bool textModeEnabled;
  final String textModeLabel;

  const CreationBottomControls({
    super.key,
    required this.inputMode,
    required this.onModeChanged,
    required this.isLoading,
    required this.canGenerate,
    required this.onGenerate,
    required this.buttonLabel,
    this.onManualCreate,
    this.textModeEnabled = true,
    this.textModeLabel = 'Text',
  });

  @override
  Widget build(BuildContext context) {
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    return Container(
      decoration: BoxDecoration(color: AppColors.experienceModalBackground(context)),
      padding: EdgeInsets.fromLTRB(20, 10, 20, bottomPadding),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Text/Image toggle and manual create button
          Row(
            children: [
              // Text/Image toggle
              Expanded(
                child: InputModeToggle(
                  currentMode: inputMode,
                  onModeChanged: onModeChanged,
                  isLoading: isLoading,
                  textModeEnabled: textModeEnabled,
                  textModeLabel: textModeLabel,
                ),
              ),
              if (onManualCreate != null) ...[
                const SizedBox(width: 12),
                // Manual creation button (icon-only, matches toggle height)
                Tappable(
                  key: const Key('manual_create_button'),
                  semanticsLabel: context.l10n.a11yMiscManualCreate,
                  onTap: isLoading ? null : onManualCreate,
                  child: Container(
                    height: 40,
                    width: 40,
                    decoration: BoxDecoration(
                      color: AppColors.surface(context),
                      borderRadius: BorderRadius.circular(20),
                      border: Border.all(
                        color: AppColors.divider(context),
                        width: 1,
                      ),
                    ),
                    child: Center(
                      child: Icon(
                        Icons.edit_note,
                        size: 20,
                        color: isLoading
                            ? AppColors.textSecondary(
                                context,
                              ).withValues(alpha: 0.3)
                            : AppColors.textSecondary(context),
                      ),
                    ),
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 16),
          // Generate button
          ElevatedButton(
            onPressed: (canGenerate && !isLoading) ? onGenerate : null,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppColors.primary(context),
              foregroundColor: AppColors.onPrimary(context),
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: const StadiumBorder(),
              elevation: 0,
              disabledBackgroundColor: AppColors.primary(
                context,
              ).withValues(alpha: 0.3),
              disabledForegroundColor: Colors.white.withValues(
                alpha: 0.5,
              ),
            ),
            child: isLoading
                ? Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          valueColor: AlwaysStoppedAnimation<Color>(
                            Colors.white,
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      const Text(
                        'Generating...',
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ],
                  )
                : SizedBox(
                    width: double.infinity,
                    child: Text(
                      buttonLabel,
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        color: canGenerate
                            ? Colors.white
                            : Colors.white.withValues(alpha: 0.5),
                      ),
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}
