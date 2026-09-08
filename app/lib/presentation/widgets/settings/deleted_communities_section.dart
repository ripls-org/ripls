import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/viewmodels/deleted_communities_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/settings/deleted_countdown_badge.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';

/// Renders the Settings → Communities "Recently deleted" section.
///
/// Watches [deletedCommunitiesProvider] and shows nothing when the
/// list is empty, still loading, or in error — the surface is
/// supplemental and should not block the active-communities list
/// from rendering. Pull-to-refresh on the hub remains the path to
/// surface real errors.
class DeletedCommunitiesSection extends ConsumerWidget {
  /// Wall-clock time used to compute the countdown badge. Tests pin
  /// this; production callers leave it null and the widget reads
  /// `DateTime.now()`.
  final DateTime? now;

  /// Invoked when a row is tapped. May be null when the Restore screen
  /// is not yet wired up; the section then renders without tap handling.
  final void Function(DeletedCommunityItem item)? onTapItem;

  const DeletedCommunitiesSection({super.key, this.now, this.onTapItem});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final items = ref.watch(deletedCommunitiesProvider).maybeWhen(
          data: (list) => list,
          orElse: () => const <DeletedCommunityItem>[],
        );
    if (items.isEmpty) {
      return const SizedBox.shrink();
    }

    final clock = (now ?? DateTime.now()).millisecondsSinceEpoch ~/ 1000;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SizedBox(height: 24),
        buildSettingsSectionHeader(
          context,
          context.l10n.settingsRecentlyDeletedHeader,
        ),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            for (var i = 0; i < items.length; i++) ...[
              if (i > 0) buildSettingsDivider(context),
              _buildRow(context, items[i], clock),
            ],
          ],
        ),
      ],
    );
  }

  Widget _buildRow(
    BuildContext context,
    DeletedCommunityItem item,
    int nowUnixSec,
  ) {
    final daysLeft = daysLeftUntilPurge(
      deletedAtUnixSec: item.deletedAtUnixSec.toInt(),
      nowUnixSec: nowUnixSec,
    );

    return Tappable(
      semanticsLabel:
          context.l10n.a11yRestoreCommunityRow(item.name, daysLeft),
      onTap: () => onTapItem?.call(item),
      inkBorderRadius: BorderRadius.zero,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            CommunityAvatar(
              community: CommunityItem(
                id: item.id,
                name: item.name,
                description: item.description,
                mediaIds: item.mediaIds,
              ),
              radius: 20,
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.name,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          fontWeight: FontWeight.w500,
                          color: AppColors.textPrimary(context),
                        ),
                  ),
                  const SizedBox(height: 4),
                  DeletedCountdownBadge(daysLeft: daysLeft),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right,
              color: AppColors.textSecondary(context),
            ),
          ],
        ),
      ),
    );
  }
}
