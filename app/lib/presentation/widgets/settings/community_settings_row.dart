import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';

/// One community row in the settings hub — avatar, name over an optional
/// second line, chevron.
///
/// A nameless (ad-hoc / per-item) community has no name, no description, and no
/// photo, so rendering [CommunityItem.name] directly produced a blank row with a
/// generic "C" circle (#2937). Both halves come from the shared derivation
/// instead: [communityDisplayName] renders the members like a group chat, and
/// [CommunityAvatar] falls back to a cluster of their faces.
class CommunitySettingsRow extends ConsumerWidget {
  final CommunityItem community;
  final VoidCallback onTap;

  const CommunitySettingsRow({
    super.key,
    required this.community,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final title = communityDisplayName(community, l10n);
    final subtitle = communitySubtitle(community, l10n);
    final isNameless = community.name.trim().isEmpty;

    return Tappable(
      semanticsLabel: title,
      semanticsIdentifier: isNameless ? 'settings-nameless-row' : null,
      onTap: onTap,
      inkBorderRadius: BorderRadius.zero,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            CommunityAvatar(
              community: community,
              radius: 20,
              // The row sits on the settings card's surface, so the cluster's
              // separating ring has to match it rather than the card default.
              ringColor: AppColors.surface(context),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    // A derived label ("You, Alex, Sam, and 5 others") is far
                    // longer than the community names this row was built for,
                    // and overflows outright at large text scales.
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          fontWeight: FontWeight.w500,
                          color: AppColors.textPrimary(context),
                        ),
                  ),
                  if (subtitle != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                            color: AppColors.textSecondary(context),
                          ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
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
