import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/avatar_status_badge.dart';
import 'package:ripls/services/providers.dart' show mediaUrlProvider;

/// UserAvatar displays a user's profile picture or initials.
///
/// Requires a User object with media_id. Resolves media URLs via MediaRepository.
/// Falls back to showing initials if no profile image is available.
///
/// When [statusBadge] is provided, a small circular badge is rendered at the
/// bottom-right corner of the avatar (42% of avatar diameter). The badge uses
/// a 2px border matching the ambient surface color for a cutout effect.
class UserAvatar extends ConsumerWidget {
  final User user;
  final double radius;
  final bool showIcon;

  /// Optional status badge overlaid at the bottom-right of the avatar.
  final AvatarStatusBadge? statusBadge;

  /// Surface color behind the avatar used for the badge border cutout.
  /// When null, uses transparent so the badge still renders without a border.
  final Color? badgeSurfaceColor;

  /// Fill for the initials circle. Defaults to the theme's primary.
  ///
  /// Callers rendering ON MEDIA must override it. The default follows the app
  /// theme, and a hero is dark whatever the theme says — the washed photo is
  /// the backdrop, not the page. In light theme the default resolves to the
  /// deep green #3E5A47, which measured 2.52:1 against a washed hero, while
  /// the same avatar passed in dark theme. A circle whose edge conveys
  /// grouping is a UI component under WCAG 1.4.11 and owes 3:1.
  final Color? backgroundColor;

  const UserAvatar({
    super.key,
    required this.user,
    this.radius = 16,
    this.showIcon = false,
    this.statusBadge,
    this.badgeSurfaceColor,
    this.backgroundColor,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    Widget avatar;

    // If user has a media_id, fetch the media URL.
    if (user.mediaId.isNotEmpty) {
      final mediaAsync = ref.watch(mediaUrlProvider(user.mediaId));

      avatar = mediaAsync.when(
        data: (mediaUrl) {
          if (mediaUrl.isNotEmpty) {
            return CircleAvatar(
              radius: radius,
              backgroundImage: CachedNetworkImageProvider(
                mediaUrl,
                cacheKey: ImageCacheKeys.thumbnail(user.mediaId),
              ),
              backgroundColor: backgroundColor ?? AppColors.primary(context),
              onBackgroundImageError: (exception, stackTrace) {
                // Fallback handled by showing initials.
              },
            );
          }
          return _buildInitialsAvatar(context);
        },
        loading: () => _buildInitialsAvatar(context),
        error: (error, stackTrace) => _buildInitialsAvatar(context),
      );
    } else if (showIcon) {
      avatar = _buildPlaceholderIcon(context);
    } else {
      avatar = _buildInitialsAvatar(context);
    }

    if (statusBadge == null) return avatar;

    return Semantics(
      label: context.l10n.a11yUserAvatarWithStatus(
        user.name,
        _badgeSemanticLabel(context, statusBadge!),
      ),
      child: ExcludeSemantics(child: _wrapWithBadge(context, avatar)),
    );
  }

  String _badgeSemanticLabel(BuildContext context, AvatarStatusBadge badge) {
    switch (badge) {
      case AvatarStatusBadge.going:
        return context.l10n.a11yAvatarBadgeGoing;
      case AvatarStatusBadge.maybe:
        return context.l10n.a11yAvatarBadgeMaybe;
      case AvatarStatusBadge.interested:
        return context.l10n.a11yAvatarBadgeInterested;
      case AvatarStatusBadge.organizer:
        return context.l10n.a11yAvatarBadgeOrganizer;
      case AvatarStatusBadge.owner:
        return context.l10n.a11yAvatarBadgeOwner;
      case AvatarStatusBadge.helping:
        return context.l10n.a11yAvatarBadgeHelping;
      case AvatarStatusBadge.requester:
        return context.l10n.a11yAvatarBadgeRequester;
      case AvatarStatusBadge.selected:
        return context.l10n.a11yAvatarBadgeSelected;
    }
  }

  /// _wrapWithBadge overlays a status badge at the bottom-right of [avatar].
  Widget _wrapWithBadge(BuildContext context, Widget avatar) {
    final badgeSize = radius * 0.84; // 42% of diameter = 84% of radius
    final color = badgeColor(statusBadge!, context);
    final icon = badgeIcon(statusBadge!);
    final borderColor = badgeSurfaceColor ?? Colors.transparent;

    return SizedBox(
      width: radius * 2,
      height: radius * 2,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          avatar,
          Positioned(
            right: -2,
            bottom: -2,
            child: Container(
              width: badgeSize + 4,
              height: badgeSize + 4,
              decoration: BoxDecoration(
                color: borderColor,
                shape: BoxShape.circle,
              ),
              child: Center(
                child: Container(
                  width: badgeSize,
                  height: badgeSize,
                  decoration: BoxDecoration(
                    color: color,
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    icon,
                    size: badgeSize * 0.65,
                    color: Colors.white,
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildInitialsAvatar(BuildContext context) {
    return CircleAvatar(
      radius: radius,
      backgroundColor: backgroundColor ?? AppColors.primary(context),
      child: Text(
        _getInitials(user.name),
        style: TextStyle(
          color: AppColors.onPrimary(context),
          fontSize: radius * 0.875, // Scale font size with radius
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  Widget _buildPlaceholderIcon(BuildContext context) {
    return Icon(
      Icons.account_circle_outlined,
      size: radius * 2,
      color: AppColors.textSecondary(context),
    );
  }

  /// Extracts initials from a name (e.g., "John Doe" -> "JD").
  String _getInitials(String? name) {
    if (name == null || name.isEmpty) return 'U';

    final parts = name.trim().split(RegExp(r'\s+'));
    if (parts.isEmpty) return 'U';

    if (parts.length == 1) {
      return parts[0][0].toUpperCase();
    }

    return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
  }
}
