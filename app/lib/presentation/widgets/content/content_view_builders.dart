import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_editable_field.dart';
import 'package:ripls/presentation/widgets/content/content_error_view.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:timeago/timeago.dart' as timeago;

/// Pure UI builder functions for content views.
///
/// These builders are stateless functions that take data as parameters
/// and return widgets. They do NOT access repositories or fetch data.
/// All data must be passed from ViewModels via parameters.
///
/// Architecture compliance:
/// - ✅ Pure UI composition (no data fetching)
/// - ✅ All data passed as parameters
/// - ✅ No repository access
/// - ✅ ViewModels handle all data access
class ContentViewBuilders {
  // Private constructor to prevent instantiation
  ContentViewBuilders._();

  /// Builds a standard loading view with black background and white spinner.
  ///
  /// Used when content is being loaded from the ViewModel.
  static Widget buildLoadingView() {
    return Container(
      color: Colors.black,
      child: const Center(
        child: CircularProgressIndicator(color: Colors.white),
      ),
    );
  }

  /// Builds a standard error view with retry button.
  ///
  /// Parameters:
  /// - [title]: Error title to display
  /// - [errorMessage]: Detailed error message
  /// - [onRetry]: Callback to retry the failed operation (typically calls ViewModel method)
  static Widget buildErrorView({
    required String title,
    required String errorMessage,
    required VoidCallback onRetry,
  }) {
    return ContentErrorView(
      title: title,
      errorMessage: errorMessage,
      onRetry: onRetry,
    );
  }

  
  
  
  /// Builds an editable title field.
  ///
  /// Pure UI - editing state managed by StatefulWidget, not ViewModel.
  ///
  /// Parameters:
  /// - [value]: Current text value (from local editing buffer)
  /// - [onChanged]: Callback when text changes (updates local editing buffer)
  /// - [enabled]: Whether field is enabled (from ViewModel saving state)
  /// - [label]: Field label
  /// - [hintText]: Placeholder text
  static Widget buildEditableTitle({
    required String value,
    required ValueChanged<String> onChanged,
    required bool enabled,
    String? label,
    String? hintText,
  }) {
    return ContentEditableField(
      value: value,
      label: label ?? 'Title',
      hintText: hintText ?? 'Enter title',
      onChanged: onChanged,
      enabled: enabled,
      maxLines: 1,
      useOverlayStyle: true,
    );
  }

  /// Builds an editable description field.
  ///
  /// Pure UI - editing state managed by StatefulWidget, not ViewModel.
  ///
  /// Parameters:
  /// - [value]: Current text value (from local editing buffer)
  /// - [onChanged]: Callback when text changes (updates local editing buffer)
  /// - [enabled]: Whether field is enabled (from ViewModel saving state)
  /// - [label]: Field label
  /// - [hintText]: Placeholder text
  /// - [maxLines]: Maximum number of lines
  /// - [minLines]: Minimum number of lines
  static Widget buildEditableDescription({
    required String value,
    required ValueChanged<String> onChanged,
    required bool enabled,
    String? label,
    String? hintText,
    int maxLines = 8,
    int minLines = 4,
  }) {
    return ContentEditableField(
      value: value,
      label: label ?? 'Description',
      hintText: hintText ?? 'Enter description',
      onChanged: onChanged,
      enabled: enabled,
      maxLines: maxLines,
      minLines: minLines,
      useOverlayStyle: true,
    );
  }

  /// Builds an expandable description with tap-to-expand functionality.
  ///
  /// Expansion state is local UI state managed by StatefulWidget.
  /// Description text comes from ViewModel state.
  ///
  /// Parameters:
  /// - [description]: Description text (from ViewModel state)
  /// - [isExpanded]: Whether description is expanded (local UI state)
  /// - [onTap]: Callback when tapped (toggles local expansion state)
  /// - [maxLinesCollapsed]: Number of lines to show when collapsed (default 3)
  static Widget buildExpandableDescription({
    required BuildContext context,
    required String description,
    required bool isExpanded,
    required VoidCallback onTap,
    int maxLinesCollapsed = 3,
  }) {
    return Tappable(
      semanticsLabel: isExpanded
          ? context.l10n.a11yHideDetails
          : context.l10n.a11yShowDetails,
      excludeChildSemantics: false,
      onTap: onTap,
      child: Text(
        description,
        style: TextStyle(
          color: Colors.white,
          fontSize: 15,
          height: 1.6,
          shadows: const [
            Shadow(offset: Offset(0, 1), blurRadius: 2, color: Colors.black45),
          ],
        ),
        maxLines: isExpanded ? null : maxLinesCollapsed,
        overflow: isExpanded ? null : TextOverflow.ellipsis,
      ),
    );
  }

  /// Builds a read-only title text widget.
  ///
  /// Parameters:
  /// - [context]: Build context for theme access
  /// - [title]: Title text (from ViewModel state)
  static Widget buildTitle({
    required BuildContext context,
    required String title,
  }) {
    return Text(
      title,
      style: Theme.of(context).textTheme.headlineSmall?.copyWith(
        fontWeight: FontWeight.bold,
        color: Colors.white,
        shadows: [
          const Shadow(
            offset: Offset(0, 1),
            blurRadius: 3,
            color: Colors.black54,
          ),
        ],
      ),
    );
  }

  /// Builds the feed header overlay with actor info and gradient.
  ///
  /// Shows actor avatar, name, action text, and timestamp.
  /// Includes gradient background for readability.
  /// Used only when content is displayed in feed context.
  ///
  /// This widget is designed to be placed as the top layer in a Stack,
  /// appearing above floating action buttons. It uses Positioned to
  /// overlay the content with full width (no right constraint).
  ///
  /// Architecture compliance:
  /// - Pure UI builder (no data fetching)
  /// - All data passed as parameters (from feed event)
  /// - Uses existing helper for user navigation
  ///
  /// Parameters:
  /// - [context]: Build context for navigation
  /// - [actor]: User who performed the action (from feed event)
  /// - [occurredAtUnixSec]: When action occurred in Unix seconds (from feed event)
  /// - [actionText]: Description of action (e.g., "Shared for loan", "Posted request")
  ///
  /// Returns a Positioned widget that overlays the top of the screen with
  /// gradient background and actor information.
  static Widget buildFeedHeader({
    required BuildContext context,
    required User actor,
    required int occurredAtUnixSec,
    required String actionText,
    bool isNavVisible = true,
    VoidCallback? onInfoTap,
  }) {
    final createdAt = DateTime.fromMillisecondsSinceEpoch(
      occurredAtUnixSec * 1000,
    );
    final timeAgo = timeago.format(createdAt);

    return Stack(
      children: [
        // Gradient background (bottom layer, non-interactive)
        // Height increases when nav is visible to cover the extra top padding.
        Positioned(
          top: 0,
          left: 0,
          right: 0,
          height: isNavVisible ? 188 : 140,
          child: IgnorePointer(
            child: Container(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [
                    Colors.black.withValues(alpha: 0.7),
                    Colors.black.withValues(alpha: 0),
                  ],
                ),
              ),
            ),
          ),
        ),

        // Header content (top layer, interactive)
        Positioned(
          top: 0,
          left: 0,
          right: 0,
          child: SafeArea(
            bottom: false,
            child: AnimatedPadding(
              duration: accessibleDuration(context, const Duration(milliseconds: 200)),
              padding: EdgeInsets.fromLTRB(
                16,
                isNavVisible
                    ? FloatingHeader.headerHeight + FloatingHeader.headerSpacing
                    : 8.0,
                16,
                24,
              ),
              child: Row(
                children: [
                  // Avatar (tappable → user profile)
                  Tappable(
                    semanticsLabel: actor.name,
                    onTap: () =>
                        ContentViewHelpers.openUserScreen(context, actor.id),
                    child: UserAvatar(user: actor, radius: 20),
                  ),
                  const SizedBox(width: 12),

                  // Name and action text
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          actor.name,
                          style: const TextStyle(
                            color: Colors.white,
                            fontSize: 16,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        Text(
                          '$actionText • $timeAgo',
                          style: TextStyle(
                            color: Colors.white.withValues(alpha: 0.7),
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (onInfoTap != null) ...[
                    const SizedBox(width: 8),
                    Tappable(
                      semanticsLabel: context.l10n.a11yShowDetails,
                      onTap: onInfoTap,
                      child: Container(
                        width: 32,
                        height: 32,
                        decoration: BoxDecoration(
                          color: Colors.black.withValues(alpha: 0.22),
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: Colors.white.withValues(alpha: 0.18),
                          ),
                        ),
                        child: const Icon(
                          Icons.info_outline,
                          color: Colors.white,
                          size: 24,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  
  /// Builds a tappable location chip with icon, text, and trailing chevron.
  ///
  /// Replaces plain-text location display with a chip-style widget that clearly
  /// signals interactivity. Tap behavior is unchanged from the previous
  /// plain-text implementation (location modal for owners, map view for viewers).
  ///
  /// Parameters:
  /// - [text]: Address or location text to display
  /// - [onTap]: Callback when tapped (opens location modal or map)
  static Widget buildLocationChip({
    required String text,
    required VoidCallback onTap,
  }) {
    return _buildInfoChip(text: text, icon: Icons.location_on, onTap: onTap);
  }

  /// Builds a time chip with icon and text.
  ///
  /// Used for experience date/time display in the same chip style as
  /// [buildLocationChip]. When [onTap] is null the chip is read-only
  /// (non-organizers with no active poll).
  ///
  /// Parameters:
  /// - [text]: Formatted time/date text to display
  /// - [onTap]: Callback when tapped; null renders the chip as read-only
  static Widget buildTimeChip({required String text, VoidCallback? onTap}) {
    return _buildInfoChipOpt(text: text, icon: Icons.access_time, onTap: onTap);
  }

  /// Builds a tappable URL chip with icon, text, and trailing chevron.
  ///
  /// Used for displaying a link in the same chip style as [buildLocationChip]
  /// and [buildTimeChip].
  ///
  /// Parameters:
  /// - [text]: URL or label text to display
  /// - [onTap]: Callback when tapped (e.g. launches the URL)
  static Widget buildUrlChip({
    required String text,
    required VoidCallback onTap,
  }) {
    return _buildInfoChip(text: text, icon: Icons.link, onTap: onTap);
  }

  /// Builds a tappable age chip showing how long ago the content was posted.
  ///
  /// Uses a schedule icon and formats the elapsed time as a human-readable
  /// relative string (e.g. "Opened 2 hrs ago", "Opened Mon, Feb 13").
  ///
  /// When [solvedAt] is provided, renders "Resolved in X" showing the duration
  /// from [postedAt] to [solvedAt] instead of the elapsed time since posting.
  /// When [isFulfilled] is true but [solvedAt] is null (legacy data), renders
  /// "Resolved" without a duration.
  ///
  /// Parameters:
  /// - [postedAt]: The UTC datetime when the content was created
  /// - [solvedAt]: Optional datetime when the content was resolved/fulfilled
  /// - [isFulfilled]: Whether the content has been resolved (for legacy fallback)
  /// - [onTap]: Callback when tapped
  static Widget buildAgeChip({
    required DateTime postedAt,
    required VoidCallback onTap,
    DateTime? solvedAt,
    bool isFulfilled = false,
  }) {
    final String text;
    if (solvedAt != null) {
      text = _formatSolvedDuration(postedAt, solvedAt);
    } else if (isFulfilled) {
      text = 'Resolved';
    } else {
      text = _formatAge(postedAt);
    }
    return _buildInfoChip(text: text, icon: Icons.schedule, onTap: onTap);
  }

  /// Formats a posted datetime into a human-readable relative age string.
  ///
  /// - < 1 minute: "Opened just now"
  /// - < 60 minutes: "Opened N min ago"
  /// - < 24 hours: "Opened N hrs ago"
  /// - < 7 days: "Opened N days ago"
  /// - >= 7 days: "Opened Mon, Feb 13"
  static String _formatAge(DateTime postedAt) {
    final now = DateTime.now();
    final diff = now.difference(postedAt);

    if (diff.inMinutes < 1) return 'Opened just now';
    if (diff.inMinutes < 60) return 'Opened ${diff.inMinutes} min ago';
    if (diff.inHours < 24) return 'Opened ${diff.inHours} hrs ago';
    if (diff.inDays < 7) return 'Opened ${diff.inDays} days ago';
    return 'Opened ${DateFormat('EEE, MMM d').format(postedAt)}';
  }

  /// Formats the duration from [postedAt] to [solvedAt] as "Resolved in X".
  ///
  /// - < 1 minute: "Resolved in moments"
  /// - < 60 minutes: "Resolved in N min"
  /// - < 24 hours: "Resolved in N hrs"
  /// - < 14 days: "Resolved in N days"
  /// - >= 14 days: "Resolved in N weeks"
  static String _formatSolvedDuration(DateTime postedAt, DateTime solvedAt) {
    final diff = solvedAt.difference(postedAt);

    if (diff.inMinutes < 1) return 'Resolved in moments';
    if (diff.inMinutes < 60) return 'Resolved in ${diff.inMinutes} min';
    if (diff.inHours < 24) return 'Resolved in ${diff.inHours} hrs';
    if (diff.inDays < 14) return 'Resolved in ${diff.inDays} days';
    return 'Resolved in ${(diff.inDays / 7).round()} weeks';
  }

  
  /// Builds the shared chip layout with an optional tap handler.
  ///
  /// Used by [buildTimeChip] where [onTap] may be null for read-only display.
  static Widget _buildInfoChipOpt({
    required String text,
    required IconData icon,
    VoidCallback? onTap,
  }) {
    return Tappable(
      semanticsLabel: text,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(10),
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.06),
          border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
          borderRadius: BorderRadius.circular(10),
        ),
        padding: const EdgeInsets.fromLTRB(10, 9, 14, 9),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: AppColors.transferCoral),
            const SizedBox(width: 6),
            Flexible(
              child: Text(
                text,
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Builds the shared chip layout used by [buildLocationChip], [buildTimeChip], [buildUrlChip], and [buildAgeChip].
  static Widget _buildInfoChip({
    required String text,
    required IconData icon,
    required VoidCallback onTap,
  }) {
    return Tappable(
      semanticsLabel: text,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(10),
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.06),
          border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
          borderRadius: BorderRadius.circular(10),
        ),
        padding: const EdgeInsets.fromLTRB(10, 9, 14, 9),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: AppColors.transferCoral),
            const SizedBox(width: 6),
            Flexible(
              child: Text(
                text,
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
                style: TextStyle(
                  color: Colors.white,
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Builds an upload progress overlay.
  ///
  /// Shows a semi-transparent overlay with a spinner and "Uploading..." text.
  /// Used to prevent user interaction during media upload operations.
  ///
  /// Usage:
  /// ```dart
  /// Stack(
  ///   children: [
  ///     // ... existing content ...
  ///     if (state.isUploadingMedia) ContentViewBuilders.buildUploadOverlay(),
  ///   ],
  /// )
  /// ```
  static Widget buildUploadOverlay() {
    return Positioned.fill(
      child: Container(
        color: Colors.black54,
        child: const Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              CircularProgressIndicator(color: Colors.white),
              SizedBox(height: 16),
              Text(
                'Uploading...',
                style: TextStyle(color: Colors.white, fontSize: 16),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
