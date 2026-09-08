import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';

/// Helper functions for common content view actions.
///
/// These helpers encapsulate common UI interactions and navigation patterns.
/// Most helpers do NOT access repositories - data operations are passed as callbacks.
/// Exception: openMediaCarousel accesses MediaRepository directly for convenience.
///
/// Architecture compliance:
/// - ✅ Pure UI interactions (dialogs, navigation, toasts)
/// - ✅ Data operations passed as callbacks (except MediaRepository in carousel)
/// - ✅ ViewModels handle all data access via callbacks
class ContentViewHelpers {
  // Private constructor to prevent instantiation
  ContentViewHelpers._();

  /// Shows the standard media picker dialog.
  ///
  /// The dialog allows users to pick video, photo, or use camera.
  /// Actual media picking is done by ViewModel methods passed as callbacks.
  ///
  /// Parameters:
  /// - [context]: Build context
  /// - [hasMedia]: Whether content currently has media (affects button text)
  /// - [onVideoTap]: Callback to pick video (calls ViewModel method)
  /// - [onPhotoTap]: Callback to pick photo (calls ViewModel method)
  /// - [onCameraTap]: Callback to use camera (calls ViewModel method)
  static void showMediaPickerDialog({
    required BuildContext context,
    required bool hasMedia,
    required Future<void> Function() onVideoTap,
    required Future<void> Function() onPhotoTap,
    required Future<void> Function() onCameraTap,
  }) {
    MediaPickerDialog.show(
      context: context,
      hasMedia: hasMedia,
      onVideoTap: () => _handleMediaPick(context, onVideoTap),
      onPhotoTap: () => _handleMediaPick(context, onPhotoTap),
      onCameraTap: () => _handleMediaPick(context, onCameraTap),
    );
  }

  /// Handles media pick operation with success/error toasts.
  ///
  /// Wraps the ViewModel media pick method with toast notifications.
  ///
  /// Parameters:
  /// - [context]: Build context for toasts
  /// - [pickFunction]: ViewModel method to pick media
  static Future<void> _handleMediaPick(
    BuildContext context,
    Future<void> Function() pickFunction,
  ) async {
    try {
      await pickFunction();
    } catch (e) {
      if (context.mounted) {
        ToastHelper.showError(context, 'Failed to upload media: $e');
      }
    }
  }

  /// Shows the standard delete confirmation dialog.
  ///
  /// Returns true if user confirms deletion, false otherwise.
  ///
  /// Parameters:
  /// - [context]: Build context
  /// - [contentType]: Type of content being deleted (e.g., 'Gear', 'Request', 'Community')
  /// - [contentName]: Optional name of content for more specific message
  static Future<bool> showDeleteConfirmation({
    required BuildContext context,
    required String contentType,
    String? contentName,
  }) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Delete $contentType'),
        content: Text(
          contentName != null
              ? 'Are you sure you want to delete "$contentName"? This action cannot be undone.'
              : 'Are you sure you want to delete this $contentType? This action cannot be undone.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(context.l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: Text(context.l10n.commonDelete),
          ),
        ],
      ),
    );
    return confirmed ?? false;
  }

  /// Opens user screen with standard slide transition.
  ///
  /// Navigation pattern used across all content views to view user
  /// profiles. Routes every user — including the viewer — to
  /// [UserScreen]. When the viewer is the target, the identity block
  /// lists every community the viewer is in and the postcard section
  /// is hidden (you can't borrow your own gear).
  static void openUserScreen(
    BuildContext context,
    String userId, {
    VoidCallback? onReturn,
  }) {
    NavigationHelpers.pushScreen(
      context: context,
      screen: UserScreen(userId: userId, onReturn: onReturn),
      useRootNavigator: true,
      routeName: 'profile',
    );
  }

  /// Handles save operation with success/error toasts.
  ///
  /// Wraps the ViewModel save method with toast notifications.
  ///
  /// Parameters:
  /// - [context]: Build context for toasts
  /// - [saveFunction]: ViewModel method to save changes
  /// - [successMessage]: Optional custom success message
  /// - [errorMessage]: Optional custom error message prefix
  static Future<void> handleSave({
    required BuildContext context,
    required Future<void> Function() saveFunction,
    String successMessage = 'Changes saved successfully!',
    String errorMessage = 'Failed to save',
  }) async {
    try {
      await saveFunction();
    } catch (e) {
      if (context.mounted) {
        ToastHelper.showError(context, '$errorMessage: $e');
      }
    }
  }

  /// Handles delete operation with success/error toasts and optional callback.
  ///
  /// Wraps the ViewModel delete method with toast notifications and onDeleted callback.
  ///
  /// Parameters:
  /// - [context]: Build context for toasts
  /// - [deleteFunction]: ViewModel method to delete content
  /// - [onDeleted]: Optional callback after successful deletion
  /// - [successMessage]: Optional custom success message
  /// - [errorMessage]: Optional custom error message prefix
  static Future<void> handleDelete({
    required BuildContext context,
    required Future<void> Function() deleteFunction,
    VoidCallback? onDeleted,
    String successMessage = 'Deleted successfully!',
    String errorMessage = 'Failed to delete',
  }) async {
    try {
      await deleteFunction();
      if (context.mounted) {
        ToastHelper.showSuccess(context, successMessage);
        onDeleted?.call();
      }
    } catch (e) {
      if (context.mounted) {
        ToastHelper.showError(context, '$errorMessage: $e');
      }
    }
  }

  /// Extracts attribution from background media.
  ///
  /// Finds the media item matching the background mediaId and returns its attribution.
  /// Used in overflow menus to display photo credits for stock images.
  ///
  /// Parameters:
  /// - [mediaId]: ID of the background media
  /// - [allMediaItems]: List of all media items for this content
  ///
  /// Returns the Attribution if found, null otherwise.
  static Attribution? getBackgroundAttribution({
    required String? mediaId,
    required List<MediaItemData> allMediaItems,
  }) {
    if (mediaId == null || allMediaItems.isEmpty) {
      return null;
    }

    return allMediaItems
        .firstWhere(
          (item) => item.id == mediaId,
          orElse: () => allMediaItems.first,
        )
        .attribution;
  }
  }
