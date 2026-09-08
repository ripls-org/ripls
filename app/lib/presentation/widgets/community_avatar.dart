import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/unread_badge.dart';
import 'package:ripls/services/providers.dart';

/// CommunityAvatar displays a community's image, a member cluster, or initials.
///
/// Shows the community's media image if available. Failing that, a **nameless**
/// (ad-hoc / per-item) community falls back to a [GroupAvatar] cluster of its
/// members — it has no name to take initials from, so the initials path used to
/// render a bare "C" on every such community at once (#2937). A named community
/// without a photo still falls back to its initials.
///
/// Can be constructed with a full [CommunityItem], or with just a [name]
/// string when only the name is available (e.g. in the access sheet/ring).
///
/// Optionally shows a badge with the total unread message count across all
/// communities when [showBadge] is true.
class CommunityAvatar extends ConsumerWidget {
  final CommunityItem? community;
  final String? mediaUrl;

  /// Used when no [CommunityItem] is available. Renders initials only.
  final String? name;

  final double radius;
  final bool showBadge;

  /// Ring color separating the overlapping circles of the nameless-community
  /// cluster. Set it to the surface the avatar sits on; defaults to the card
  /// background. Ignored for the image and initials paths.
  final Color? ringColor;

  const CommunityAvatar({
    super.key,
    this.community,
    this.mediaUrl,
    this.name,
    this.radius = 16,
    this.showBadge = false,
    this.ringColor,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    Widget avatarWidget;

    if (community == null && name != null) {
      avatarWidget = _buildPlaceholder(context, _getInitials(name));
    } else if (community == null) {
      avatarWidget = _buildPlaceholder(context, 'C');
    } else if (mediaUrl != null && mediaUrl!.isNotEmpty) {
      // If mediaUrl is provided directly, use it
      // Note: When mediaUrl is provided directly, we don't have mediaId for cache key.
      // This should be avoided - prefer passing community with mediaIds instead.
      avatarWidget = _buildImageAvatar(context, mediaUrl!, community!.id);
    } else {
      final mediaId = community!.mediaIds.firstOrNull ?? '';
      if (mediaId.isNotEmpty) {
        // Watch the cached URL provider so the future isn't rebuilt on every
        // parent rebuild — fixes the placeholder flash on carousel selection.
        final urlAsync = ref.watch(mediaUrlProvider(mediaId));
        avatarWidget = urlAsync.when(
          data: (url) => url.isNotEmpty
              ? _buildImageAvatar(context, url, mediaId)
              : _buildFallback(context, ref),
          loading: () => _buildFallback(context, ref),
          error: (_, _) => _buildFallback(context, ref),
        );
      } else {
        avatarWidget = _buildFallback(context, ref);
      }
    }

    // If showBadge is false, return avatar without badge
    if (!showBadge) {
      return avatarWidget;
    }

    // Watch total unread count
    final totalUnreadCount = ref.watch(unreadMessageCountTotalProvider);

    // If no unread messages, return avatar without badge
    if (totalUnreadCount == 0) {
      return avatarWidget;
    }

    // Return avatar with badge overlay
    return Stack(
      clipBehavior: Clip.none,
      children: [
        avatarWidget,
        Positioned(
          right: -4,
          top: -4,
          child: UnreadBadge(count: totalUnreadCount),
        ),
      ],
    );
  }

  Widget _buildImageAvatar(BuildContext context, String url, String mediaId) {
    return CircleAvatar(
      radius: radius,
      backgroundImage: CachedNetworkImageProvider(
        url,
        cacheKey: ImageCacheKeys.thumbnail(mediaId),
      ),
      backgroundColor: AppColors.primary(context),
      onBackgroundImageError: (exception, stackTrace) {
        // Error handled by showing placeholder
      },
    );
  }

  /// Photo-less fallback for a resolved [community]: a member cluster when the
  /// community is nameless, its initials when it has a name.
  Widget _buildFallback(BuildContext context, WidgetRef ref) {
    final members = communityGroupAvatarMembers(
      community: community!,
      viewer: ref.watch(authStateProvider.select((s) => s.user)),
    );
    if (members.isNotEmpty) {
      return GroupAvatar(
        members: members,
        diameter: radius * 2,
        ringColor: ringColor ?? AppColors.cardBackground(context),
      );
    }
    return _buildPlaceholder(context, _getInitials(community!.name));
  }

  Widget _buildPlaceholder(BuildContext context, String text) {
    return CircleAvatar(
      radius: radius,
      backgroundColor: AppColors.primary(context),
      child: Text(
        text,
        style: TextStyle(
          color: AppColors.onPrimary(context),
          fontSize: radius * 0.875,
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  /// Extracts initials from a community name (e.g., "My Community" -> "MC").
  String _getInitials(String? name) {
    if (name == null || name.isEmpty) return 'C';

    final parts = name.trim().split(RegExp(r'\s+'));
    if (parts.isEmpty) return 'C';

    if (parts.length == 1) {
      return parts[0][0].toUpperCase();
    }

    return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
  }
}
