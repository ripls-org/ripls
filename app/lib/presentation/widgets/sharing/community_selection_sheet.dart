import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart' show UserError;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/viewmodels/experience_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_sharing_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/sharing/community_list_item.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_tile.dart';
import 'package:ripls/services/providers.dart';

/// CommunitySelectionMode controls the commit semantics of the picker.
///
/// - [deferred]: multi-select; results are returned to the caller on Confirm.
/// - [invite]: single-select; tapping a row picks-and-pops with that ID.
/// - [immediate]: multi-select; each toggle fires API mutations through the
///   appropriate sharing provider (see [itemType]). Used to edit access on
///   an existing item.
enum CommunitySelectionMode { deferred, invite, immediate }

/// Source identifiers for [CommunitySelectionOpenedEvent]. Use these
/// constants at every call site so analytics can distinguish entry points.
class CommunitySelectionSource {
  CommunitySelectionSource._();

  static const String invite = 'invite';
  static const String gearCreate = 'gear_create';
    static const String experienceCreate = 'experience_create';
  static const String experienceAccess = 'experience_access';
  static const String requestCreate = 'request_create';
  static const String requestAccess = 'request_access';
  }

/// Unified community-selection bottom sheet used across creation, access,
/// and invite flows.
///
/// Wraps content in a [GlassSheet] and always includes a sticky
/// "Create new community" affordance that opens [CommunityCreationModal]
/// and, on success, refreshes the user-communities cache via
/// [CommunityRepository.refreshUserCommunities] before auto-selecting the
/// new community.
///
/// Use [showForDeferred] for item-creation flows (multi-select, returns
/// the chosen IDs on confirm) or [showForInvite] when picking a single
/// community for inviting (tap-to-pop).
class CommunitySelectionSheet extends ConsumerStatefulWidget {
  final CommunitySelectionMode mode;
  final List<String> initialSelection;

  /// Communities to hide from the list — used by additive flows (e.g. the
  /// share sheet's "Invite community") to drop ones the item is already shared
  /// with, so the picker only offers new communities to add.
  final List<String> excludeCommunityIds;

  final String source;

  /// Used only in [CommunitySelectionMode.immediate]. Identifies the item
  /// whose access is being edited.
  final String? itemId;

  /// Used only in [CommunitySelectionMode.immediate]. One of `'gear'`,
  /// `'request'`, or `'experience'` — selects the sharing provider.
  final String? itemType;

  /// Used only in [CommunitySelectionMode.immediate]. When false, toggles
  /// are disabled (read-only view).
  final bool isOwner;

  const CommunitySelectionSheet._({
    required this.mode,
    required this.initialSelection,
    required this.source,
    this.excludeCommunityIds = const [],
    this.itemId,
    this.itemType,
    this.isOwner = false,
  });

  /// Opens the sheet in deferred multi-select mode for item-creation flows.
  ///
  /// Returns the selected community IDs on Confirm, or `null` if dismissed.
  static Future<List<String>?> showForDeferred(
    BuildContext context, {
    required String source,
    List<String> initialSelection = const [],
    List<String> excludeCommunityIds = const [],
  }) {
    return showAccessibleModal<List<String>>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => CommunitySelectionSheet._(
        mode: CommunitySelectionMode.deferred,
        initialSelection: initialSelection,
        excludeCommunityIds: excludeCommunityIds,
        source: source,
      ),
    );
  }

  /// Opens the sheet in immediate mode for editing access on an existing item.
  ///
  /// Toggles fire API mutations through the appropriate sharing provider
  /// (gear/request/experience). The sheet auto-loads its community list via
  /// `loadUserCommunities()` on the resolved provider before display.
  ///
  /// [itemType] must be one of `'gear'`, `'request'`, or `'experience'`.
  static Future<void> showForImmediate(
    BuildContext context, {
    required String itemId,
    required String itemType,
    required bool isOwner,
    required String source,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => CommunitySelectionSheet._(
        mode: CommunitySelectionMode.immediate,
        initialSelection: const [],
        source: source,
        itemId: itemId,
        itemType: itemType,
        isOwner: isOwner,
      ),
    );
  }

  /// Opens the sheet in single-select invite mode.
  ///
  /// Tapping any row pops with that community's ID. Returns `null` if
  /// dismissed without a selection.
  static Future<String?> showForInvite(
    BuildContext context, {
    String? initialSelectedId,
  }) async {
    final result = await showAccessibleModal<List<String>>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => CommunitySelectionSheet._(
        mode: CommunitySelectionMode.invite,
        initialSelection:
            initialSelectedId == null ? const [] : [initialSelectedId],
        source: CommunitySelectionSource.invite,
      ),
    );
    return result?.firstOrNull;
  }

  @override
  ConsumerState<CommunitySelectionSheet> createState() =>
      _CommunitySelectionSheetState();
}

class _CommunitySelectionSheetState
    extends ConsumerState<CommunitySelectionSheet> {
  late Set<String> _selected;

  /// True when the picker is adding communities to an existing item's audience
  /// (the share sheet's "Invite community"): a deferred multi-select picker
  /// opened with the invite source. Drives the invite-specific title, drops the
  /// redundant subtitle, and hides the create-new affordance (not useful when
  /// inviting existing communities).
  bool get _isInviteCommunities =>
      widget.mode == CommunitySelectionMode.deferred &&
      widget.source == CommunitySelectionSource.invite;

  @override
  void initState() {
    super.initState();
    _selected = Set<String>.from(widget.initialSelection);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      ref
          .read(observabilityServiceProvider)
          .logAnalyticsEvent(
            CommunitySelectionOpenedEvent(source: widget.source),
          );
    });
  }

  Future<void> _handleCreateNewCommunity() async {
    final newId = await CommunityCreationModal.show(context);
    if (newId == null || !mounted) return;

    await ref.read(communityRepositoryProvider).refreshUserCommunities();
    if (!mounted) return;
    await ref.read(communitiesProvider.notifier).reloadCommunities();
    if (!mounted) return;

    if (widget.mode == CommunitySelectionMode.invite) {
      Navigator.of(context).pop([newId]);
    } else {
      setState(() => _selected.add(newId));
    }
  }

  void _onTileChanged(String communityId, bool value) {
    if (widget.mode == CommunitySelectionMode.invite) {
      Navigator.of(context).pop([communityId]);
      return;
    }
    setState(() {
      if (value) {
        _selected.add(communityId);
      } else {
        _selected.remove(communityId);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final screenHeight = MediaQuery.of(context).size.height;

    return SizedBox(
      height: screenHeight * 0.6,
      child: GlassSheet(
        padding: EdgeInsets.zero,
        child: Column(
          children: [
            _buildHeader(context),
            Expanded(child: _buildBody(context)),
            _buildFooter(context),
          ],
        ),
      ),
    );
  }

  Widget _buildBody(BuildContext context) {
    if (widget.mode == CommunitySelectionMode.immediate) {
      return _buildImmediateBody(context);
    }
    final excluded = widget.excludeCommunityIds.toSet();
    final communities = ref
        .watch(communitiesProvider)
        .communities
        .where((c) => !excluded.contains(c.id))
        .toList();
    return communities.isEmpty
        ? _buildEmpty(context)
        : _buildList(communities);
  }

  Widget _buildImmediateBody(BuildContext context) {
    final itemType = widget.itemType!;
    if (itemType == 'gear') {
      final state = ref.watch(gearSharingProvider);
      return _buildImmediateContent(
        context,
        isLoading: state.isLoadingCommunities,
        error: state.communitiesError,
        communities: state.userCommunities,
      );
    } else if (itemType == 'request') {
      final state = ref.watch(requestSharingProvider);
      return _buildImmediateContent(
        context,
        isLoading: state.isLoadingCommunities,
        error: state.communitiesError,
        communities: state.userCommunities,
      );
    } else if (itemType == 'experience') {
      final state = ref.watch(experienceSharingNotifierProvider);
      return _buildImmediateContent(
        context,
        isLoading: state.isLoadingCommunities,
        error: state.communitiesError,
        communities: state.userCommunities,
      );
    }
    return _buildErrorView(context, 'Unknown item type: $itemType');
  }

  Widget _buildImmediateContent(
    BuildContext context, {
    required bool isLoading,
    required UserError? error,
    required List<CommunityItem> communities,
  }) {
    if (isLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (error != null) {
      return _buildErrorView(
        context,
        RpcErrorHandler.localize(error, context.l10n),
      );
    }
    if (communities.isEmpty) {
      return _buildEmpty(context);
    }
    return ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 24),
      itemCount: communities.length,
      itemBuilder: (context, index) {
        final community = communities[index];
        return CommunityListItem(
          community: community,
          isOwner: widget.isOwner,
          itemId: widget.itemId!,
          itemType: widget.itemType!,
        );
      },
    );
  }

  Widget _buildErrorView(BuildContext context, String error) {
    final l10n = context.l10n;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error, color: AppColors.modalTextPrimary, size: 48),
            const SizedBox(height: 12),
            Text(
              l10n.communityPickerErrorTitle,
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 16,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              error,
              style: TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 14,
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    final l10n = context.l10n;
    final isInvite = widget.mode == CommunitySelectionMode.invite;
    final String title;
    if (_isInviteCommunities) {
      title = l10n.communityPickerInviteCommunitiesTitle;
    } else if (isInvite) {
      title = l10n.communityPickerInviteTitle;
    } else {
      title = l10n.communityPickerHeading;
    }
    // The invite-communities title is self-explanatory, so its subtitle is
    // dropped; other modes keep their explanatory line.
    final String? subtitle = _isInviteCommunities
        ? null
        : isInvite
            ? l10n.communityPickerInviteSubtitle
            : l10n.communityPickerHeadingSubtitle;
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 4, 24, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(
            header: true,
            child: Text(
              title,
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 24,
                fontWeight: FontWeight.bold,
              ),
            ),
          ),
          if (subtitle != null) ...[
            const SizedBox(height: 8),
            Text(
              subtitle,
              style: TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 14,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildList(List<CommunityItem> communities) {
    final isSingleSelect = widget.mode == CommunitySelectionMode.invite;
    final tileStyle = isSingleSelect
        ? CommunitySelectionStyle.single
        : CommunitySelectionStyle.multiCheck;
    return ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 24),
      itemCount: communities.length,
      itemBuilder: (context, index) {
        final community = communities[index];
        final isSelected = _selected.contains(community.id);
        final displayName = communityDisplayName(community, context.l10n);
        return Toggle(
          key: Key('community_selection_tile_${community.id}'),
          semanticsLabel: displayName,
          selected: isSelected,
          inMutuallyExclusiveGroup: isSingleSelect,
          onTap: () => _onTileChanged(community.id, !isSelected),
          child: CommunitySelectionTile(
            name: displayName,
            isSelected: isSelected,
            style: tileStyle,
            groupMembers: communityGroupAvatarMembers(
              community: community,
              viewer: ref.watch(authStateProvider.select((s) => s.user)),
            ),
            // Decorative-only indicator in single-select mode; the row's
            // outer Toggle is the sole tap surface there.
            onChanged: isSingleSelect
                ? null
                : (value) => _onTileChanged(community.id, value),
          ),
        );
      },
    );
  }

  Widget _buildEmpty(BuildContext context) {
    final l10n = context.l10n;
    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.groups, color: AppColors.modalTextPrimary, size: 48),
          const SizedBox(height: 12),
          Text(
            l10n.communityPickerEmpty,
            style: TextStyle(
              color: AppColors.modalTextPrimary,
              fontSize: 16,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            l10n.communityPickerEmptyBody,
            style: TextStyle(
              color: AppColors.modalTextSecondary,
              fontSize: 14,
            ),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }

  Widget _buildFooter(BuildContext context) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Creating a new community is irrelevant when inviting existing
          // communities to an item, so it's hidden in that flow.
          if (!_isInviteCommunities)
            GlassInlineAction(
              text: l10n.communityPickerCreateNew,
              semanticsLabel: l10n.a11yCommunityPickerCreateNew,
              icon: Icons.add,
              onTap: _handleCreateNewCommunity,
            ),
          if (widget.mode == CommunitySelectionMode.deferred)
            GlassFooterButtons(
              secondaryLabel: l10n.commonCancel,
              secondaryEnabled: true,
              onSecondary: () => Navigator.of(context).pop(),
              primaryLabel: l10n.commonConfirm,
              primaryEnabled: _selected.isNotEmpty,
              // Higher-contrast sage on the dark glass than the deep-sage
              // default (which reads as low-contrast on this surface).
              primaryColor: AppColors.experienceSageGreen,
              onPrimary: _selected.isEmpty
                  ? null
                  : () => Navigator.of(context).pop(_selected.toList()),
            ),
        ],
      ),
    );
  }
}
