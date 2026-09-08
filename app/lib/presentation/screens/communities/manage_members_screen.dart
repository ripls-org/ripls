import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/viewmodels/manage_members_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/presentation/widgets/provisional_user_profile_sheet.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/community_service.dart' show CommunityItem;
import 'package:ripls/services/providers.dart' show communitiesProvider;

/// ManageMembersScreen shows real and provisional (non-registered) community members.
///
/// From this screen, community members can:
/// - View all real members with join dates; tap to open their profile
/// - View all prov participants with their claim status
/// - Invite someone via the standard invite sheet (FAB)
/// - Send a personal invite link to prov participants
class ManageMembersScreen extends ConsumerStatefulWidget {
  final String communityId;

  const ManageMembersScreen({required this.communityId, super.key});

  /// Pushes the screen onto the navigator stack with a slide-from-right animation.
  static Future<void> show(BuildContext context, String communityId) {
    return NavigationHelpers.pushWithSlide(
      context: context,
      screen: ManageMembersScreen(communityId: communityId),
      routeName: 'manage_members',
    );
  }

  @override
  ConsumerState<ManageMembersScreen> createState() =>
      _ManageMembersScreenState();
}

class _ManageMembersScreenState extends ConsumerState<ManageMembersScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(manageMembersProvider.notifier).load(widget.communityId);
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(manageMembersProvider);
    final communityState = ref.watch(communitiesProvider);
    final community = communityState.communities
        .where((c) => c.id == widget.communityId)
        .firstOrNull;

    ref.listen<ManageMembersState>(manageMembersProvider, (_, next) {
      if (next.pendingInviteLink != null) {
        _showProvisionalInviteSheet(
          context,
          next.pendingInviteLink!.inviteUrl,
          community,
        );
        ref.read(manageMembersProvider.notifier).clearInviteLink();
      }
    });

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: _buildAppBar(context),
      body: _buildBody(context, state),
      floatingActionButton: _buildInviteButton(context, community),
    );
  }

  AppBar _buildAppBar(BuildContext context) {
    return AppBar(
      backgroundColor: AppColors.appBarBackground(context),
      elevation: 0,
      leading: const AppBarBackButton(),
      title: Text(
        context.l10n.manageMembersTitle,
        style: Theme.of(context).textTheme.titleMedium,
      ),
    );
  }

  Widget _buildBody(BuildContext context, ManageMembersState state) {
    return CustomScrollView(
      slivers: [
        if (state.error != null) _buildErrorBanner(context, state.error!),
        _buildMembersSection(context, state),
        _buildProvisionalSection(context, state),
        const SliverToBoxAdapter(child: SizedBox(height: 80)),
      ],
    );
  }

  Widget _buildErrorBanner(BuildContext context, UserError error) {
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
        child: ContentErrorBanner(
          errorMessage: RpcErrorHandler.localize(error, context.l10n),
        ),
      ),
    );
  }

  Widget _buildMembersSection(BuildContext context, ManageMembersState state) {
    return SliverToBoxAdapter(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildSectionHeader(
            context,
            context.l10n.manageMembersRealSectionHeader,
          ),
          if (state.isLoadingMembers)
            const Padding(
              padding: EdgeInsets.all(16),
              child: Center(child: CircularProgressIndicator()),
            )
          else
            ...state.members.map((member) => _buildMemberTile(context, member)),
        ],
      ),
    );
  }

  Widget _buildProvisionalSection(
    BuildContext context,
    ManageMembersState state,
  ) {
    return SliverToBoxAdapter(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildSectionHeader(
            context,
            context.l10n.manageMembersProvisionalSectionHeader,
          ),
          if (state.isLoadingProvisionalUsers)
            const Padding(
              padding: EdgeInsets.all(16),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (state.provisionalUsers.isEmpty)
            _buildEmptyProvisionalState(context)
          else
            ...state.provisionalUsers.map(
              (prov) => _buildProvisionalTile(context, prov),
            ),
        ],
      ),
    );
  }

  Widget _buildSectionHeader(BuildContext context, String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 4),
      child: Text(
        title,
        style: Theme.of(context).textTheme.labelLarge?.copyWith(
          color: AppColors.textSecondary(context),
          letterSpacing: 0.5,
        ),
      ),
    );
  }

  Widget _buildMemberTile(BuildContext context, CommunityMember member) {
    final joinedAt = member.joinedAtUnixSec.toInt();
    final joinDate = joinedAt > 0
        ? _formatJoinDate(DateTime.fromMillisecondsSinceEpoch(joinedAt * 1000))
        : null;

    return ListTile(
      leading: UserAvatar(user: member.user, radius: 20),
      title: Text(member.user.name),
      subtitle: joinDate != null
          ? Text(
              joinDate,
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 12,
              ),
            )
          : null,
      onTap: () => ContentViewHelpers.openUserScreen(context, member.user.id),
    );
  }

  Widget _buildProvisionalTile(BuildContext context, ProvisionalUser prov) {
    final isClaimed = prov.isClaimed;
    return ListTile(
      leading: ProvisionalUserAvatar(name: prov.name, radius: 20),
      title: Text(prov.name),
      subtitle: Text(
        isClaimed
            ? context.l10n.manageMembersProvisionalClaimed
            : context.l10n.manageMembersProvisionalUnclaimed,
        style: TextStyle(
          color: isClaimed ? Colors.green : AppColors.textSecondary(context),
          fontSize: 12,
        ),
      ),
      trailing: isClaimed
          ? null
          : IconButton(
              icon: Icon(
                Icons.send_outlined,
                color: AppColors.primary(context),
                size: 20,
              ),
              tooltip: context.l10n.manageMembersSendInvite,
              onPressed: () => ref
                  .read(manageMembersProvider.notifier)
                  .getInviteLink(
                    communityId: widget.communityId,
                    provisionalUserId: prov.id,
                  ),
            ),
      onTap: () => ProvisionalUserProfileSheet.show(
        context,
        provisionalUser: prov,
        communityId: widget.communityId,
      ),
    );
  }

  Widget _buildEmptyProvisionalState(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
      child: Text(
        context.l10n.manageMembersEmptyProvisional,
        style: TextStyle(color: AppColors.textSecondary(context)),
      ),
    );
  }

  Widget _buildInviteButton(BuildContext context, CommunityItem? community) {
    return FloatingActionButton.extended(
      onPressed: community != null
          ? () => _showCommunityInviteSheet(context, community)
          : null,
      icon: const Icon(Icons.person_add_outlined),
      label: Text(context.l10n.plusButtonInviteSomeone),
    );
  }

  Future<void> _showCommunityInviteSheet(
    BuildContext context,
    CommunityItem community,
  ) async {
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      // The sheet composes outbound share text ("Join {name}! {link}") sent to
      // people who are NOT members, so a nameless community takes the generic
      // public label — never the member rollup communityDisplayName would give
      // (#2937). Same rule the server applies on the public /go/ landing.
      builder: (ctx) => InviteSheet(
        communityId: community.id,
        communityName: communityPublicName(community, context.l10n),
        onClose: () => Navigator.of(ctx).pop(),
      ),
    );
  }

  Future<void> _showProvisionalInviteSheet(
    BuildContext context,
    String inviteUrl,
    CommunityItem? community,
  ) async {
    if (community == null) {
      ToastHelper.showInfo(context, inviteUrl);
      return;
    }
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      // The sheet composes outbound share text ("Join {name}! {link}") sent to
      // people who are NOT members, so a nameless community takes the generic
      // public label — never the member rollup communityDisplayName would give
      // (#2937). Same rule the server applies on the public /go/ landing.
      builder: (ctx) => InviteSheet(
        communityId: community.id,
        communityName: communityPublicName(community, context.l10n),
        onClose: () => Navigator.of(ctx).pop(),
      ),
    );
  }

  String _formatJoinDate(DateTime date) {
    final months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    return '${months[date.month - 1]} ${date.day}, ${date.year}';
  }
}
