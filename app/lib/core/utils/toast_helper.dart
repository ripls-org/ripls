import 'dart:async';

import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/undo.pb.dart' as undo_pb;

/// ToastHelper provides centralized, theme-aware toast notifications.
///
/// This utility ensures consistent styling across the app and proper
/// support for light/dark themes.
class ToastHelper {
  /// Shows a success toast message with theme-aware colors.
  static void showSuccess(BuildContext context, String message) {
    _showSnackBar(
      context: context,
      message: message,
      statusColor: AppColors.statusSuccess(context),
      icon: Icons.check_circle,
    );
  }

  /// Shows an error toast message with theme-aware colors.
  static void showError(BuildContext context, String message) {
    _showSnackBar(
      context: context,
      message: message,
      statusColor: AppColors.statusError(context),
      icon: Icons.error,
    );
  }

  /// Shows a warning toast message with theme-aware colors.
  static void showWarning(BuildContext context, String message) {
    _showSnackBar(
      context: context,
      message: message,
      statusColor: AppColors.statusWarning(context),
      icon: Icons.warning,
    );
  }

  /// Shows an info toast message with theme-aware colors.
  static void showInfo(BuildContext context, String message) {
    _showSnackBar(
      context: context,
      message: message,
      statusColor: AppColors.statusInfo(context),
      icon: Icons.info,
    );
  }

  /// showUndo displays a floating SnackBar with an undo action that
  /// auto-dismisses after [duration] regardless of the calling widget's
  /// position in an IndexedStack.
  ///
  /// Flutter's built-in SnackBar timer only starts when the show-animation
  /// reaches AnimationStatus.completed. In IndexedStack layouts, non-active
  /// tabs are wrapped in TickerMode(enabled: false), which prevents the
  /// animation from completing on any Scaffold registered from within those
  /// tabs. showUndo bypasses this by setting an effectively-infinite duration
  /// on the SnackBar itself and dismissing it manually via Future.delayed.
  ///
  /// The ScaffoldMessenger reference is captured before the delay, so it
  /// remains valid even if the calling widget is later unmounted. Existing
  /// snack bars are cleared before the new one is shown.
  static void showUndo({
    required BuildContext context,
    required String message,
    required String undoLabel,
    required VoidCallback onUndo,
    Duration duration = const Duration(seconds: 4),
    EdgeInsetsGeometry? margin,
  }) {
    final messenger = ScaffoldMessenger.of(context);
    messenger.clearSnackBars();
    messenger.showSnackBar(
      SnackBar(
        content: Text(message),
        duration: const Duration(days: 365),
        behavior: SnackBarBehavior.floating,
        margin: margin ?? const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        action: SnackBarAction(
          label: undoLabel,
          onPressed: onUndo,
        ),
      ),
    );
    Future.delayed(duration, messenger.hideCurrentSnackBar);
  }

  /// showServerUndo fires an optimistic snackbar tied to a server-side
  /// undo RPC. The RPC call happens on tap; failure surfaces the
  /// UndoErrorDetail user_message as a secondary toast and invokes
  /// [onUndoFailed] so the caller can re-apply the forward state.
  ///
  /// [optimisticRevert] is invoked first so the UI reflects the
  /// pre-action state immediately. [onUndoSucceeded] fires after the
  /// server confirms reversal; callers typically navigate back to the
  /// now-active item screen there.
  static void showServerUndo({
    required BuildContext context,
    required String message,
    required String undoLabel,
    required Future<void> Function() onUndo,
    VoidCallback? optimisticRevert,
    VoidCallback? onUndoSucceeded,
    void Function(undo_pb.UndoFailureReason reason, String userMessage)?
        onUndoFailed,
    Duration duration = const Duration(seconds: 4),
    EdgeInsetsGeometry? margin,
  }) {
    final messenger = ScaffoldMessenger.of(context);
    messenger.clearSnackBars();
    messenger.showSnackBar(
      SnackBar(
        content: Text(message),
        duration: const Duration(days: 365),
        behavior: SnackBarBehavior.floating,
        margin:
            margin ?? const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        action: SnackBarAction(
          label: undoLabel,
          onPressed: () async {
            messenger.hideCurrentSnackBar();
            optimisticRevert?.call();
            try {
              await onUndo();
              onUndoSucceeded?.call();
            } on connect.ConnectException catch (e) {
              final reason = extractUndoFailureReason(e);
              final userMessage = extractUndoUserMessage(e) ?? e.message;
              if (context.mounted) {
                showError(context, userMessage);
              }
              onUndoFailed?.call(reason, userMessage);
            } catch (e) {
              final msg = e.toString();
              if (context.mounted) {
                showError(context, msg);
              }
              onUndoFailed?.call(
                undo_pb.UndoFailureReason.UNDO_FAILURE_REASON_UNSPECIFIED,
                msg,
              );
            }
          },
        ),
      ),
    );
    Future.delayed(duration, messenger.hideCurrentSnackBar);
  }

  /// Extracts the typed UndoFailureReason from a Connect error's
  /// attached UndoErrorDetail, or returns UNSPECIFIED when none is
  /// present.
  static undo_pb.UndoFailureReason extractUndoFailureReason(
    connect.ConnectException e,
  ) {
    for (final detail in e.details) {
      if (detail.type == 'ripls.api.UndoErrorDetail') {
        try {
          final parsed = undo_pb.UndoErrorDetail.fromBuffer(detail.value);
          return parsed.reason;
        } catch (_) {
          // fall through to UNSPECIFIED
        }
      }
    }
    return undo_pb.UndoFailureReason.UNDO_FAILURE_REASON_UNSPECIFIED;
  }

  /// Extracts the server-rendered user_message from a Connect error's
  /// attached UndoErrorDetail, or returns null when none is present.
  static String? extractUndoUserMessage(connect.ConnectException e) {
    for (final detail in e.details) {
      if (detail.type == 'ripls.api.UndoErrorDetail') {
        try {
          final parsed = undo_pb.UndoErrorDetail.fromBuffer(detail.value);
          if (parsed.userMessage.isNotEmpty) {
            return parsed.userMessage;
          }
        } catch (_) {
          // fall through
        }
      }
    }
    return null;
  }

  /// Internal method to show a SnackBar with consistent styling.
  ///
  /// [statusColor] tints the ICON, not the sheet. The status variants used to
  /// fill the whole bar — a green success toast, a red error one — while the
  /// ~120 bare `SnackBar(content: ...)` call sites elsewhere in the app fell
  /// through to the Material default. The result was that a run of toasts in
  /// one flow came up green, white, green, and read as three different
  /// components. They now share the theme's snack-bar surface (see
  /// `AppTheme.snackBarTheme`) and differ only by the glyph.
  ///
  /// It also removes the contrast problem at the root rather than solving it:
  /// there is no longer a status fill for the label to be measured against.
  static void _showSnackBar({
    required BuildContext context,
    required String message,
    required Color statusColor,
    required IconData icon,
    Duration duration = const Duration(seconds: 3),
  }) {
    final scheme = Theme.of(context).snackBarTheme;
    final textColor = scheme.contentTextStyle?.color ??
        AppColors.textPrimary(context);

    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Directionality(
          textDirection: TextDirection.ltr,
          child: Row(
            children: [
              Icon(icon, color: statusColor, size: 20),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  message,
                  style: TextStyle(
                    color: textColor,
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
            ],
          ),
        ),
        duration: duration,
      ),
    );
  }

}
