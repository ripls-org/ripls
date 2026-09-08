import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Invitee;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/completion/dark_person_search.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart'
    show ShareableItemType;
import 'package:ripls/services/providers.dart';

/// InviteMembersSheet adds existing Ripls members to an item's audience (#2492).
///
/// First pass of single-person invites: members only. The host chip-completes
/// people from their own communities (reusing [DarkPersonSearch], the same
/// component as the completion flows, on a matching [CompletionColors] surface)
/// and on confirm each is added to the item's ad-hoc community via
/// [CommunityRepository.shareItem] with a `member_user_id` invitee — no consent
/// needed since they're already on Ripls. Off-app invitees (phone/email) are
/// gated on the A2P 10DLC campaign and show a "coming soon" hint.
///
/// [show] resolves to `true` once at least one person was invited, so the
/// caller (the share sheet) can dismiss itself rather than dropping back to the
/// QR code.
class InviteMembersSheet extends ConsumerStatefulWidget {
  const InviteMembersSheet({
    super.key,
    required this.itemType,
    required this.itemId,
  });

  final ShareableItemType itemType;
  final String itemId;

  /// Opens the invite-members sheet. Resolves to `true` if anyone was invited.
  static Future<bool?> show(
    BuildContext context, {
    required ShareableItemType itemType,
    required String itemId,
  }) {
    return showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => InviteMembersSheet(itemType: itemType, itemId: itemId),
    );
  }

  @override
  ConsumerState<InviteMembersSheet> createState() =>
      _InviteMembersSheetState();
}

class _InviteMembersSheetState extends ConsumerState<InviteMembersSheet> {
  final List<User> _selected = [];
  bool _isSharing = false;

  String? get _experienceId =>
      widget.itemType == ShareableItemType.experience ? widget.itemId : null;
  String? get _gearId =>
      widget.itemType == ShareableItemType.gear ? widget.itemId : null;
  String? get _requestId =>
      widget.itemType == ShareableItemType.request ? widget.itemId : null;

  void _addMember(User user) {
    if (_selected.any((u) => u.id == user.id)) return;
    setState(() => _selected.add(user));
  }

  void _removeMember(String userId) {
    setState(() => _selected.removeWhere((u) => u.id == userId));
  }

  /// People already in the event's audience (host + responders + directly
  /// invited), so the picker doesn't offer to invite someone twice.
  /// Experience-only; empty for gear/request (no individual roster yet).
  Set<String> _alreadyInvitedMemberIds() {
    if (widget.itemType != ShareableItemType.experience) return const {};
    final details =
        ref.read(experienceProvider(widget.itemId)).experienceDetails;
    if (details == null) return const {};
    return {
      details.experience.owner.id,
      for (final r in details.rsvps) r.user.id,
      for (final u in details.invitedIndividuals) u.id,
    };
  }

  void _offAppComingSoon() {
    ToastHelper.showInfo(
      context,
      "Inviting people who aren't on Ripls yet is coming soon.",
    );
  }

  Future<void> _confirm() async {
    if (_selected.isEmpty || _isSharing) return;
    setState(() => _isSharing = true);
    final count = _selected.length;
    final firstName = _selected.first.name;
    try {
      await ref.read(communityRepositoryProvider).shareItem(
            experienceId: _experienceId,
            gearId: _gearId,
            requestId: _requestId,
            invitees:
                _selected.map((u) => Invitee(memberUserId: u.id)).toList(),
          );
      if (!mounted) return;
      // Reflect the new member without a full reload (#2492): refresh the
      // community list (a now-multi-member per-item community surfaces in the
      // Workshop switcher, with the "name this group" nudge) and schedule a
      // roster refresh (the member appears; the roster nudge unlocks at ≥2
      // members). Best-effort — never block/fail the invite on a refresh hiccup.
      final expId = _experienceId;
      if (expId != null) {
        ref.read(experienceProvider(expId).notifier).scheduleRefresh();
      }
      unawaited(ref.read(communitiesProvider.notifier).reloadCommunities());
      ToastHelper.showSuccess(
        context,
        count == 1 ? 'Invited $firstName' : 'Invited $count people',
      );
      Navigator.of(context).pop(true);
    } catch (e) {
      if (!mounted) return;
      setState(() => _isSharing = false);
      ToastHelper.showError(context, "Couldn't invite: $e");
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final hostCommunityIds = ref
        .watch(communitiesProvider)
        .communities
        .map((c) => c.id)
        .toSet();
    final maxSearchHeight = MediaQuery.of(context).size.height * 0.42;

    // Same frosted GlassSheet as the Invite-communities picker, so the two
    // invite surfaces match.
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: Padding(
        padding:
            EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 0),
              child: Semantics(
                header: true,
                child: Text(
                  l10n.shareSheetInvitePeople,
                  style: TextStyle(
                    color: AppColors.modalTextPrimary,
                    fontSize: 24,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
            ),
            if (_selected.isNotEmpty) _buildSelectedChips(context),
            ConstrainedBox(
              constraints: BoxConstraints(maxHeight: maxSearchHeight),
              child: SingleChildScrollView(
                child: DarkPersonSearch(
                  communityId: hostCommunityIds.isNotEmpty
                      ? hostCommunityIds.first
                      : '',
                  searchCommunityIds: hostCommunityIds,
                  excludedMemberIds: {
                    ..._alreadyInvitedMemberIds(),
                    ..._selected.map((u) => u.id),
                  },
                  excludedProvisionalIds: const {},
                  onMemberSelected: _addMember,
                  onProvisionalSelected: (_) => _offAppComingSoon(),
                  onNewProvisionalRequested: (_) => _offAppComingSoon(),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
              child: GlassFooterButtons(
                secondaryLabel: l10n.commonCancel,
                secondaryEnabled: !_isSharing,
                onSecondary: () => Navigator.of(context).pop(),
                primaryLabel: _selected.isEmpty
                    ? 'Invite'
                    : 'Invite ${_selected.length}',
                primaryEnabled: _selected.isNotEmpty && !_isSharing,
                primaryColor: AppColors.experienceSageGreen,
                onPrimary:
                    _selected.isEmpty || _isSharing ? null : _confirm,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSelectedChips(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 12, 24, 0),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: _selected
            .map(
              (u) => Container(
                key: ValueKey('selected-${u.id}'),
                padding: const EdgeInsets.fromLTRB(12, 6, 6, 6),
                decoration: BoxDecoration(
                  color: AppColors.modalInsetCardBg,
                  borderRadius: BorderRadius.circular(20),
                  border: Border.all(color: AppColors.modalInsetCardBorder),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      u.name,
                      style: TextStyle(
                        color: AppColors.modalTextPrimary,
                        fontSize: 14,
                      ),
                    ),
                    const SizedBox(width: 4),
                    Tappable(
                      semanticsLabel: context.l10n.a11yRemovePerson(u.name),
                      onTap: _isSharing ? () {} : () => _removeMember(u.id),
                      child: Icon(
                        Icons.close,
                        size: 16,
                        color: AppColors.modalTextSecondary,
                      ),
                    ),
                  ],
                ),
              ),
            )
            .toList(),
      ),
    );
  }
}
