import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/screens/governance/governance_screen.dart';
import 'package:ripls/presentation/screens/profile/profile_settings_screen.dart';
import 'package:ripls/presentation/screens/settings/rejoin_community_screen.dart';
import 'package:ripls/presentation/screens/settings/restore_community_screen.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/settings/community_settings_row.dart';
import 'package:ripls/presentation/widgets/settings/deleted_communities_section.dart';
import 'package:ripls/presentation/widgets/settings/recently_left_section.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/auth_state.dart' show AuthStateData;
import 'package:ripls/services/providers.dart';

/// SettingsHubScreen is the top-level settings entry point.
///
/// Displays the current user and each community the user belongs to.
/// Tapping the user row opens [ProfileSettingsScreen]; tapping a community
/// row opens [GovernanceScreen] for that community.
class SettingsHubScreen extends ConsumerStatefulWidget {
  const SettingsHubScreen({super.key});

  @override
  ConsumerState<SettingsHubScreen> createState() => _SettingsHubScreenState();
}

class _SettingsHubScreenState extends ConsumerState<SettingsHubScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    final authState = ref.watch(authStateProvider);
    final communityState = ref.watch(communitiesProvider);
    // Name-presence is the ad-hoc discriminator (docs/ad_hoc_communities.md) —
    // there is no kind enum to switch on.
    final named = communityState.communities
        .where((c) => c.name.trim().isNotEmpty)
        .toList(growable: false);
    final groups = communityState.communities
        .where((c) => c.name.trim().isEmpty)
        .toList(growable: false);

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: handleClose),
        title: Text(
          context.l10n.commonSettings,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _buildAccountSection(context, authState),
          // Named communities and nameless per-item groups are both memberships
          // with governance actions, but they read differently enough — a name
          // you chose vs. the people you shared one thing with — that mixing
          // them buries the communities (#2937).
          if (named.isNotEmpty) ...[
            const SizedBox(height: 24),
            _buildCommunitiesSection(
              context,
              context.l10n.sidebarCommunitiesHeader,
              named,
            ),
          ],
          if (groups.isNotEmpty) ...[
            const SizedBox(height: 24),
            _buildCommunitiesSection(
              context,
              context.l10n.settingsGroupsHeader,
              groups,
            ),
          ],
          DeletedCommunitiesSection(
            onTapItem: (item) => NavigationHelpers.pushWithSlide(
              context: context,
              screen: RestoreCommunityScreen(
                communityId: item.id,
                communityName: item.name,
              ),
              routeName: 'restore_community',
            ),
          ),
          RecentlyLeftSection(
            onTapItem: (item) => NavigationHelpers.pushWithSlide(
              context: context,
              screen: RejoinCommunityScreen(community: item),
              routeName: 'rejoin_community',
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAccountSection(BuildContext context, AuthStateData authState) {
    final user = authState.user;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.commonProfile),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            _buildAvatarActionItem(
              context,
              leading: user != null
                  ? UserAvatar(user: user, radius: 20)
                  : CircleAvatar(
                      radius: 20,
                      backgroundColor: AppColors.primary(context),
                      child: const Icon(Icons.person, color: Colors.white, size: 20),
                    ),
              title: user?.name ?? context.l10n.commonProfile,
              subtitle: context.l10n.profileSettingsTitle,
              onTap: () => NavigationHelpers.pushScreen(
                context: context,
                screen: ProfileSettingsScreen(userId: authState.user?.id),
                routeName: 'profile_settings',
              ),
            ),
            buildSettingsDivider(context),
            _buildAvatarActionItem(
              context,
              leading: CircleAvatar(
                radius: 20,
                backgroundColor: AppColors.primary(context),
                child: Icon(
                  Icons.trending_up,
                  color: AppColors.onPrimary(context),
                  size: 20,
                ),
              ),
              title: context.l10n.settingsYourImpactTitle,
              subtitle: context.l10n.settingsYourImpactSubtitle,
              onTap: () {
                final userId = authState.user?.id;
                if (userId == null || userId.isEmpty) return;
                NavigationHelpers.pushWithSlide(
                  context: context,
                  screen: UserScreen(userId: userId),
                  routeName: 'profile',
                );
              },
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildCommunitiesSection(
    BuildContext context,
    String header,
    List<CommunityItem> communities,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, header),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            for (var i = 0; i < communities.length; i++) ...[
              if (i > 0) buildSettingsDivider(context),
              CommunitySettingsRow(
                community: communities[i],
                onTap: () => NavigationHelpers.pushScreen(
                  context: context,
                  screen: GovernanceScreen(community: communities[i]),
                  routeName: 'governance',
                ),
              ),
            ],
          ],
        ),
      ],
    );
  }

  Widget _buildAvatarActionItem(
    BuildContext context, {
    required Widget leading,
    required String title,
    String? subtitle,
    required VoidCallback onTap,
  }) {
    return Tappable(
      semanticsLabel: title,
      onTap: onTap,
      inkBorderRadius: BorderRadius.zero,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            leading,
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          fontWeight: FontWeight.w500,
                          color: AppColors.textPrimary(context),
                        ),
                  ),
                  if (subtitle != null && subtitle.isNotEmpty) ...[
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
