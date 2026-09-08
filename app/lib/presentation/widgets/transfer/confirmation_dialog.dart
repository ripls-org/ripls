import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Standard confirmation dialog with customizable title, message, actions.
class ConfirmationDialog extends StatelessWidget {
  const ConfirmationDialog({
    super.key,
    required this.title,
    required this.message,
    this.cancelLabel = 'Cancel',
    this.confirmLabel = 'Confirm',
    this.isDestructive = false,
  });

  final String title;
  final String message;
  final String cancelLabel;
  final String confirmLabel;
  final bool isDestructive;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(
        title,
        style: TextStyle(
          fontSize: 18,
          fontWeight: FontWeight.w700,
          color: AppColors.transferTextPrimary(context),
        ),
      ),
      content: Text(
        message,
        style: TextStyle(
          fontSize: 14,
          color: AppColors.transferTextSecondary(context),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(false),
          child: Text(
            cancelLabel,
            style: TextStyle(
              color: AppColors.transferTextSecondary(context),
            ),
          ),
        ),
        ElevatedButton(
          onPressed: () => Navigator.of(context).pop(true),
          style: ElevatedButton.styleFrom(
            // `transferCoral` is not coral — it is the dark primary, a light
            // sage — so a destructive confirm rendered sage, and white on it
            // measured 2.01:1. The status ramp says what destructive looks
            // like, and this dialog follows the app theme, so both ends resolve
            // for it. `on-primary`'s polarity suits the error ramp too: the
            // light theme's error is dark and the dark theme's is light.
            backgroundColor: isDestructive
                ? AppColors.statusError(context)
                : AppColors.primary(context),
            foregroundColor: AppColors.onPrimary(context),
            shape: const StadiumBorder(),
          ),
          child: Text(confirmLabel),
        ),
      ],
    );
  }
}

/// Helper function to show confirmation dialog.
Future<bool?> showConfirmationDialog({
  required BuildContext context,
  required String title,
  required String message,
  String cancelLabel = 'Cancel',
  String confirmLabel = 'Confirm',
  bool isDestructive = false,
}) {
  return showDialog<bool>(
    context: context,
    builder: (context) => ConfirmationDialog(
      title: title,
      message: message,
      cancelLabel: cancelLabel,
      confirmLabel: confirmLabel,
      isDestructive: isDestructive,
    ),
  );
}
