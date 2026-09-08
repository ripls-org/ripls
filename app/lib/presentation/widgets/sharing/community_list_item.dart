import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/viewmodels/experience_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_sharing_view_model.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_tile.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show authStateProvider;

/// CommunityListItem displays a single community with a simple on/off toggle.
///
/// Supports gear, requests, and experiences by using [itemType] to determine which provider to use.
class CommunityListItem extends ConsumerWidget {
  final CommunityItem community;
  final bool isOwner;
  final String itemId;
  final String itemType; // 'gear', 'request', or 'experience'

  const CommunityListItem({
    super.key,
    required this.community,
    required this.isOwner,
    required this.itemId,
    required this.itemType,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isShared = _isShared(ref);
    final isLoading = _isLoading(ref);

    return CommunitySelectionTile(
      key: Key('community_share_toggle_${community.id}'),
      name: communityDisplayName(community, context.l10n),
      isSelected: isShared,
      isLoading: isLoading,
      groupMembers: communityGroupAvatarMembers(
        community: community,
        viewer: ref.watch(authStateProvider.select((s) => s.user)),
      ),
      onChanged: isOwner
          ? (value) => _toggleCommunitySharing(context, ref, value)
          : null,
    );
  }

  bool _isShared(WidgetRef ref) {
    if (itemType == 'gear') {
      final state = ref.watch(gearSharingProvider);
      return state.sharedCommunityIds.contains(community.id);
    } else if (itemType == 'request') {
      final state = ref.watch(requestSharingProvider);
      return state.sharedCommunityIds.contains(community.id);
    } else if (itemType == 'experience') {
      final state = ref.watch(experienceSharingNotifierProvider);
      return state.sharedCommunityIds.contains(community.id);
    }
    return false;
  }

  bool _isLoading(WidgetRef ref) {
    if (itemType == 'gear') {
      final state = ref.watch(gearSharingProvider);
      return state.loadingCommunityIds.contains(community.id);
    } else if (itemType == 'request') {
      final state = ref.watch(requestSharingProvider);
      return state.loadingCommunityIds.contains(community.id);
    } else if (itemType == 'experience') {
      final state = ref.watch(experienceSharingNotifierProvider);
      return state.isSharing;
    }
    return false;
  }

  Future<void> _toggleCommunitySharing(
    BuildContext context,
    WidgetRef ref,
    bool shouldShare,
  ) async {
    if (itemType == 'gear') {
      await _toggleGearSharing(context, ref, shouldShare);
    } else if (itemType == 'request') {
      await _toggleRequestSharing(context, ref, shouldShare);
    } else if (itemType == 'experience') {
      await _toggleExperienceSharing(context, ref, shouldShare);
    }
  }

  Future<void> _toggleGearSharing(
    BuildContext context,
    WidgetRef ref,
    bool shouldShare,
  ) async {
    final gearSharingState = ref.read(gearSharingProvider);

    // Prevent duplicate requests
    if (gearSharingState.loadingCommunityIds.contains(community.id)) return;

    // Use the existing availability from the first shared community, falling
    // back to FOR_LOAN when no communities are currently shared.
    final currentGearAvailability =
        gearSharingState.communityAvailability.values.firstOrNull ??
        Availability.AVAILABILITY_FOR_LOAN;

    final error = await ref.read(gearSharingProvider.notifier).setCommunityAvailability(
      communityId: community.id,
      availability: shouldShare ? currentGearAvailability : null,
    );

    if (!context.mounted) return;

    if (error != null) {
      ToastHelper.showError(context, error);
    }
  }

  Future<void> _toggleRequestSharing(
    BuildContext context,
    WidgetRef ref,
    bool shouldShare,
  ) async {
    final requestSharingState = ref.read(requestSharingProvider);

    // Prevent duplicate requests
    if (requestSharingState.loadingCommunityIds.contains(community.id)) return;

    try {
      if (shouldShare) {
        await ref.read(requestSharingProvider.notifier).shareWithCommunity(
          itemId,
          community.id,
        );
      } else {
        await ref.read(requestSharingProvider.notifier).unshareFromCommunity(
          itemId,
          community.id,
        );
      }
    } catch (e) {
      if (!context.mounted) return;
      ToastHelper.showError(context, e.toString());
    }
  }

  Future<void> _toggleExperienceSharing(
    BuildContext context,
    WidgetRef ref,
    bool shouldShare,
  ) async {
    final experienceSharingState = ref.read(experienceSharingNotifierProvider);

    // Prevent duplicate requests
    if (experienceSharingState.isSharing) return;

    try {
      if (shouldShare) {
        await ref.read(experienceSharingNotifierProvider.notifier).shareWithCommunity(
          itemId,
          community.id,
        );
      } else {
        await ref.read(experienceSharingNotifierProvider.notifier).unshareFromCommunity(
          itemId,
          community.id,
        );
      }
    } catch (e) {
      if (!context.mounted) return;
      ToastHelper.showError(context, e.toString());
    }
  }
}
