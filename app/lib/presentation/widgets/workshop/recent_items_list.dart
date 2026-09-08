import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart'
    show ImpactMetricDimension, RecentActivity, RecentActivityKind;
import 'package:ripls/data/repositories/media_url.dart' show MediaUrl;
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/screens/gear/gear_screen.dart';
import 'package:ripls/presentation/screens/request/request_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/impact/impact_copy.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// Renders a vertical list of recent contributing items for a workshop
/// detail screen (money / time / CO2 / quality time / acts). Each row
/// shows the item's thumbnail, the item's name (or a localized fallback
/// derived from its kind), "owner · time ago" on the second line, and the
/// raw metric value formatted with a localized unit for [dimension].
/// Tapping a row opens the underlying content screen with a
/// slide-from-right transition.
class RecentItemsList extends StatelessWidget {
  const RecentItemsList({
    super.key,
    required this.items,
    required this.dimension,
  });

  /// Server-supplied recent contributors, newest first.
  final List<RecentActivity> items;

  /// The metric dimension these rows contributed to — selects the unit the
  /// value column renders with.
  final ImpactMetricDimension dimension;

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) {
      return const SizedBox.shrink();
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (int i = 0; i < items.length; i++) ...[
          if (i > 0) const Divider(height: 1),
          _RecentItemRow(item: items[i], dimension: dimension),
        ],
      ],
    );
  }
}

class _RecentItemRow extends ConsumerWidget {
  const _RecentItemRow({required this.item, required this.dimension});

  final RecentActivity item;
  final ImpactMetricDimension dimension;

  void _onTap(BuildContext context) {
    if (item.contentId.isEmpty) return;
    switch (item.kind) {
      case RecentActivityKind.RECENT_ACTIVITY_KIND_GEAR:
        NavigationHelpers.pushScreen(
          context: context,
          screen: GearScreen(gearId: item.contentId),
          useRootNavigator: true,
          routeName: 'gear_detail',
        );
      case RecentActivityKind.RECENT_ACTIVITY_KIND_EXPERIENCE:
        NavigationHelpers.pushScreen(
          context: context,
          screen: ExperienceScreen(experienceId: item.contentId),
          useRootNavigator: true,
          routeName: 'experience_detail',
        );
      case RecentActivityKind.RECENT_ACTIVITY_KIND_REQUEST:
        NavigationHelpers.pushScreen(
          context: context,
          screen: RequestScreen(requestId: item.contentId),
          useRootNavigator: true,
          routeName: 'request_detail',
        );
      case RecentActivityKind.RECENT_ACTIVITY_KIND_UNSPECIFIED:
        break;
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final label = recentActivityLabel(context.l10n, item);
    final value = item.hasRawValue()
        ? impactValueText(context.l10n, dimension, item.rawValue)
        : '';
    final subtitle = _buildSubtitle(item);
    final row = Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          _Thumbnail(mediaId: item.mediaId, label: label),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  label,
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: AppColors.textPrimary(context),
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                if (subtitle.isNotEmpty) ...[
                  const SizedBox(height: 2),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12,
                      color: AppColors.textSecondary(context),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ],
            ),
          ),
          if (value.isNotEmpty) ...[
            const SizedBox(width: 8),
            Text(
              value,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: AppColors.textPrimary(context),
              ),
            ),
          ],
        ],
      ),
    );
    if (item.contentId.isEmpty) return row;
    return Tappable(
      semanticsLabel: label,
      onTap: () => _onTap(context),
      excludeChildSemantics: false,
      child: row,
    );
  }

  static String _buildSubtitle(RecentActivity item) {
    final owner = item.hasPersonDisplayName() ? item.personDisplayName : '';
    final completedSec = item.completedAtUnixSec.toInt();
    final ago = completedSec > 0
        ? _formatTimeAgo(
            DateTime.fromMillisecondsSinceEpoch(completedSec * 1000),
          )
        : '';
    if (owner.isEmpty) return ago;
    if (ago.isEmpty) return owner;
    return '$owner · $ago';
  }

  /// Formats a past timestamp as a compact relative string (e.g.
  /// "just now", "3m ago", "2h ago", "5d ago", "3w ago"). Future
  /// timestamps collapse to "just now".
  static String _formatTimeAgo(DateTime then) {
    final diff = DateTime.now().difference(then);
    if (diff.inSeconds < 60) return 'just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    if (diff.inDays < 7) return '${diff.inDays}d ago';
    if (diff.inDays < 30) return '${(diff.inDays / 7).floor()}w ago';
    if (diff.inDays < 365) return '${(diff.inDays / 30).floor()}mo ago';
    return '${(diff.inDays / 365).floor()}y ago';
  }
}

class _Thumbnail extends ConsumerWidget {
  const _Thumbnail({required this.mediaId, required this.label});

  final String mediaId;
  final String label;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    const size = 44.0;
    if (mediaId.isEmpty) {
      return _PlaceholderThumbnail(size: size);
    }
    final asyncMedia = ref.watch(mediaObjectProvider(mediaId));
    return asyncMedia.when(
      data: (MediaUrl mediaUrl) => CachedMediaImage(
        imageUrl: mediaUrl.url,
        cacheKey: mediaUrl.cacheKey,
        width: size,
        height: size,
        borderRadius: BorderRadius.circular(8),
        fit: BoxFit.cover,
        semanticsLabel: context.l10n.a11yPhotoOfItem(label),
      ),
      loading: () => _PlaceholderThumbnail(size: size),
      error: (_, _) => _PlaceholderThumbnail(size: size),
    );
  }
}

class _PlaceholderThumbnail extends StatelessWidget {
  const _PlaceholderThumbnail({required this.size});

  final double size;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Icon(
        Icons.image_outlined,
        size: size * 0.55,
        color: AppColors.textSecondary(context),
      ),
    );
  }
}
