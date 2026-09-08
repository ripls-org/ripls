import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/screens/governance/owner_leave_handoff_confirm_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

/// OwnerLeaveMemberPickerScreen lists active members of the community
/// (minus the caller) and lets the owner pick one to take over before
/// leaving. Tap a row routes to handoff confirm, then leave confirm.
///
/// Reached from two entry points:
///   1. Membership-section **Leave Community** button when the caller
///      is the owner.
///   2. #1718's Danger Zone choice modal **Just leave — let someone
///      else take over** branch.
///
/// Sole-member empty state is defence-in-depth only — governance
/// screen routes sole-member communities through
/// `SoleMemberLeaveExplainerScreen` before this screen is reached.
/// The empty state fires only when branch detection mis-classifies
/// (e.g. the cached `numMembers` is briefly stale).
class OwnerLeaveMemberPickerScreen extends ConsumerStatefulWidget {
  final CommunityItem community;

  const OwnerLeaveMemberPickerScreen({super.key, required this.community});

  @override
  ConsumerState<OwnerLeaveMemberPickerScreen> createState() =>
      _OwnerLeaveMemberPickerScreenState();
}

class _OwnerLeaveMemberPickerScreenState
    extends ConsumerState<OwnerLeaveMemberPickerScreen> {
  late final Future<List<CommunityMember>> _membersFuture;

  @override
  void initState() {
    super.initState();
    _membersFuture = _loadMembers();
  }

  Future<List<CommunityMember>> _loadMembers() async {
    final callerId = ref.read(authStateProvider).user?.id;
    final members = await ref
        .read(communityServiceProvider)
        .listCommunityUsers(widget.community.id);
    return members.where((m) => m.user.id != callerId).toList();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityOwnerLeavePickerTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: FutureBuilder<List<CommunityMember>>(
        future: _membersFuture,
        builder: (context, snapshot) {
          if (snapshot.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }
          if (snapshot.hasError) {
            return _buildError(context, snapshot.error);
          }
          final members = snapshot.data ?? const <CommunityMember>[];
          if (members.isEmpty) {
            return _buildSoleMemberEmptyState(context);
          }
          return _buildMemberList(context, members);
        },
      ),
    );
  }

  Widget _buildError(BuildContext context, Object? error) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(
              context.l10n.communityOwnerLeavePickerLoadError,
              style: TextStyle(color: AppColors.textPrimary(context)),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),
            ElevatedButton(
              onPressed: () => setState(() {}),
              child: Text(context.l10n.commonRetry),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSoleMemberEmptyState(BuildContext context) {
    // Defence-in-depth: governance_screen routes sole-member communities
    // through the dedicated SoleMemberLeaveExplainerScreen before this
    // screen is reached. This empty-state only fires when branch
    // detection mis-classifies (e.g. cached numMembers stale by one
    // event), and tells the user to back out and retry rather than
    // attempting an owner-handoff with no candidates.
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.person_outline,
              size: 48,
              color: AppColors.textSecondary(context),
            ),
            const SizedBox(height: 16),
            Text(
              context.l10n.communityOwnerLeaveSoleMemberFallback,
              style: TextStyle(color: AppColors.textPrimary(context)),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildMemberList(
    BuildContext context,
    List<CommunityMember> members,
  ) {
    return ListView.builder(
      padding: const EdgeInsets.symmetric(vertical: 8),
      itemCount: members.length,
      itemBuilder: (context, index) {
        final member = members[index];
        return Tappable(
          semanticsLabel: context.l10n.a11yOwnerLeavePickRow(member.user.name),
          onTap: () => _onMemberTap(member),
          inkBorderRadius: BorderRadius.zero,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Row(
              children: [
                UserAvatar(user: member.user, radius: 20),
                const SizedBox(width: 16),
                Expanded(
                  child: Text(
                    member.user.name,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          color: AppColors.textPrimary(context),
                        ),
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
      },
    );
  }

  void _onMemberTap(CommunityMember member) {
    NavigationHelpers.pushWithSlide(
      context: context,
      screen: OwnerLeaveHandoffConfirmScreen(
        community: widget.community,
        candidate: member.user,
      ),
      routeName: 'owner_leave_handoff_confirm',
    );
  }
}
