import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// UI interaction helpers for standardized modal behavior.
///
/// Provides helper functions for:
/// - Showing modals with keyboard handling
/// - Handling modal close with unsaved changes
/// - Dismissing keyboard
class ModalHelpers {
  ModalHelpers._();

  /// Show standard modal bottom sheet with keyboard handling.
  ///
  /// Features:
  /// - 75% height by default, expands to 90% when keyboard appears
  /// - Smooth 200ms animation for height changes
  /// - Always dismissible (tap outside, swipe down)
  /// - Transparent background
  /// - Automatic keyboard handling via isScrollControlled
  /// - Tap outside inputs to dismiss keyboard (optional, default: true)
  ///
  /// Example:
  /// ```dart
  /// await ModalHelpers.showStandardModal(
  ///   context,
  ///   builder: (context) => MyModalContent(),
  ///   heightFactor: 0.75,
  ///   dismissKeyboardOnTapOutside: true, // Default
  /// );
  /// ```
  static Future<T?> showStandardModal<T>(
    BuildContext context, {
    required Widget Function(BuildContext) builder,
    double heightFactor = 0.75,
    bool isDismissible = true,
    bool enableDrag = true,
    bool dismissKeyboardOnTapOutside = true,
    bool useRootNavigator = false,
  }) {
    return showAccessibleModal<T>(context,
      isScrollControlled: true, // REQUIRED for keyboard handling
      backgroundColor: Colors.transparent,
      isDismissible: isDismissible,
      enableDrag: enableDrag,
      useRootNavigator: useRootNavigator,
      builder: (context) {
        // Detect keyboard height
        final keyboardHeight = MediaQuery.of(context).viewInsets.bottom;
        final hasKeyboard = keyboardHeight > 0;
        final screenHeight = MediaQuery.of(context).size.height;

        // Calculate height: 90% with keyboard, heightFactor (75%) without
        final modalHeight = hasKeyboard ? screenHeight * 0.90 : screenHeight * heightFactor;

        Widget content = builder(context);

        // Wrap in Tappable for tap-to-dismiss keyboard
        if (dismissKeyboardOnTapOutside) {
          content = Tappable(
            semanticsLabel: context.l10n.a11yMiscDismissKeyboard,
            onTap: () {
              // Unfocus when tapping outside input fields
              FocusScope.of(context).unfocus();
            },
            excludeChildSemantics: false,
            child: content,
          );
        }

        return AnimatedContainer(
          duration: accessibleDuration(context, const Duration(milliseconds: 200)),
          height: modalHeight,
          child: ClipRRect(
            borderRadius: const BorderRadius.vertical(
              top: Radius.circular(20),
            ),
            child: content,
          ),
        );
      },
    );
  }

  /// Handle modal close with optional unsaved changes check.
  ///
  /// Shows a confirmation dialog if there are unsaved changes.
  /// Otherwise closes immediately.
  ///
  /// Example:
  /// ```dart
  /// ModalHelpers.handleModalClose(
  ///   context,
  ///   hasUnsavedChanges: state.isDirty,
  ///   onConfirmClose: () {
  ///     // Clean up
  ///     Navigator.pop(context);
  ///   },
  /// );
  /// ```
  static Future<void> handleModalClose(
    BuildContext context, {
    bool hasUnsavedChanges = false,
    VoidCallback? onConfirmClose,
  }) async {
    if (!hasUnsavedChanges) {
      if (onConfirmClose != null) {
        onConfirmClose();
      } else {
        Navigator.of(context).pop();
      }
      return;
    }

    // Show confirmation dialog
    final shouldClose = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Unsaved Changes'),
        content: const Text(
          'You have unsaved changes. Are you sure you want to close?',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(context.l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Discard'),
          ),
        ],
      ),
    );

    if ((shouldClose ?? false) && context.mounted) {
      if (onConfirmClose != null) {
        onConfirmClose();
      } else {
        Navigator.of(context).pop();
      }
    }
  }

  /// Dismiss keyboard and unfocus.
  ///
  /// Utility function to hide the keyboard by unfocusing all focus nodes.
  ///
  /// Example:
  /// ```dart
  /// ModalHelpers.dismissKeyboard(context);
  /// ```
  static void dismissKeyboard(BuildContext context) {
    FocusScope.of(context).unfocus();
  }
}
