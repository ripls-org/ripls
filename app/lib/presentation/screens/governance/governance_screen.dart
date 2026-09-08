import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';
import 'package:ripls/presentation/screens/communities/community_edit_screen.dart';
import 'package:ripls/presentation/screens/communities/manage_members_screen.dart';
import 'package:ripls/presentation/screens/communities/manage_notifications_screen.dart';
import 'package:ripls/presentation/screens/governance/delete_community_screen.dart';
import 'package:ripls/presentation/screens/governance/leave_community_screen.dart';
import 'package:ripls/presentation/screens/governance/owner_leave_member_picker_screen.dart';
import 'package:ripls/presentation/screens/governance/sole_member_leave_explainer_screen.dart';
import 'package:ripls/presentation/viewmodels/governance_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/location/location_autocomplete_field.dart';
import 'package:ripls/presentation/widgets/settings/danger_zone_choice_modal.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/providers.dart';

import '../../../core/theme/app_colors.dart';
import '../../../core/utils/navigation_helpers.dart';
import '../../../core/utils/toast_helper.dart';

/// _GovernanceAction represents an action that can be performed on a community.
class _GovernanceAction {
  final String title;
  final String description;
  final IconData icon;
  final VoidCallback onTap;

  const _GovernanceAction({
    required this.title,
    required this.description,
    required this.icon,
    required this.onTap,
  });
}

/// GovernanceScreen displays community governance actions for [community].
class GovernanceScreen extends ConsumerStatefulWidget {
  final ScrollController? scrollController;

  /// The community whose governance actions to display.
  final CommunityItem? community;

  const GovernanceScreen({super.key, this.scrollController, this.community});

  @override
  ConsumerState<GovernanceScreen> createState() => _GovernanceScreenState();
}

class _GovernanceScreenState extends ConsumerState<GovernanceScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  CommunityItem? get _community => widget.community;

  @override
  void initState() {
    super.initState();
    // Eager refresh so the tap-time `numMembers` branch (sole-member
    // vs. multi-member) reads fresh server truth without an extra
    // round-trip per Leave tap. Per #1740, invalidating the Stash
    // cache layer alone is a no-op on `communityProvider`'s family
    // entry — both layers must be invalidated. Once #1740's signal
    // ships the second call becomes redundant.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final community = _community;
      if (community == null) return;
      ref.read(communityRepositoryProvider).invalidate(community.id);
      ref.invalidate(communityProvider(community.id));
    });
  }

  @override
  Widget build(BuildContext context) {
    // A nameless (ad-hoc) community's name is "", not null, so the null-coalesce
    // never fired and the app bar rendered blank (#2937). Derive it instead.
    final community = _community;
    final communityName = community != null
        ? communityDisplayName(community, context.l10n)
        : context.l10n.settingsCommunityFallbackTitle;
    final currentUserId = ref.watch(authStateProvider).user?.id;
    final actions = _buildActions(context, ref, currentUserId);

    // The Delete Community entry is owner-only; when the caller isn't
    // the owner, the actions list has 6 entries instead of 7 and the
    // Danger Zone section is suppressed entirely.
    final showDangerSection = actions.length == 7;

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          communityName,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        leading: AppBarBackButton(
          onPressed: handleClose,
        ),
      ),
      body: ListView(
        controller: widget.scrollController,
        padding: const EdgeInsets.all(16),
        children: [
          _buildCommunitySection(context, actions.sublist(0, 2)),
          const SizedBox(height: 24),
          _buildMembershipSection(context, actions.sublist(2, 6)),
          if (showDangerSection) ...[
            const SizedBox(height: 24),
            _buildDangerSection(context, actions.sublist(6, 7)),
          ],
        ],
      ),
    );
  }

  List<_GovernanceAction> _buildActions(
    BuildContext context,
    WidgetRef ref,
    String? currentUserId,
  ) {
    final selectedCommunity = _community;
    final isOwner = selectedCommunity != null &&
        currentUserId != null &&
        selectedCommunity.ownerUserId == currentUserId;
    // Read `numMembers` off the cached `GetCommunityResponse`. The
    // value populates on first watch and refreshes on every event
    // routed through `eventRouterProvider` (and on the eager
    // initState invalidation above). When the community hasn't
    // resolved yet (transient null), default to "not sole" so the
    // user falls through to the multi-member flow rather than
    // routing into the sole-member explainer with stale data.
    final numMembers = selectedCommunity == null
        ? null
        : ref
            .watch(communityProvider(selectedCommunity.id))
            .asData
            ?.value
            .numMembers;
    final isSoleMember = numMembers != null && numMembers <= 1;

    return [
      _GovernanceAction(
        title: 'Update Profile',
        description: 'Change name, image, or description',
        icon: Icons.edit_outlined,
        onTap: selectedCommunity != null
            ? () => _navigateToEditScreen(context, ref, selectedCommunity.id)
            : () => _showNoCommunitySelectedSnackbar(context),
      ),
      _GovernanceAction(
        title: 'Manage Region',
        description: 'View and manage community region',
        icon: Icons.location_on_outlined,
        onTap: selectedCommunity != null
            ? () => _navigateToAction(
                context,
                _ManageRegionScreen(communityId: selectedCommunity.id),
              )
            : () => _showNoCommunitySelectedSnackbar(context),
      ),
      _GovernanceAction(
        title: 'View & Invite Members',
        description: 'See who\'s here and invite someone new',
        icon: Icons.people_outline,
        onTap: selectedCommunity != null
            ? () => ManageMembersScreen.show(context, selectedCommunity.id)
            : () => _showNoCommunitySelectedSnackbar(context),
      ),
      _GovernanceAction(
        title: context.l10n.settingsManageNotificationsAction,
        description: context.l10n.settingsManageNotificationsActionSubtitle,
        icon: Icons.notifications_outlined,
        onTap: selectedCommunity != null
            ? () => NavigationHelpers.pushScreen(
                  context: context,
                  screen: ManageNotificationsScreen(
                      communityId: selectedCommunity.id),
                  routeName: 'manage_notifications',
                )
            : () => _showNoCommunitySelectedSnackbar(context),
      ),
      _GovernanceAction(
        title: 'Manage Privacy',
        description: 'Modify who can see what about the community',
        icon: Icons.lock_outline,
        onTap: () => _navigateToAction(context, _LockCommunityScreen(),
            routeName: 'lock_community'),
      ),
      _GovernanceAction(
        title: 'Leave Community',
        description: 'Remove yourself from this community',
        icon: Icons.exit_to_app,
        // Three-way branch (§2.4 / §7.3):
        //   1. sole-member (regardless of role) → explainer +
        //      sole-member confirm. Server cascades into a community
        //      soft-delete.
        //   2. owner of multi-member → picker → handoff confirm →
        //      leave confirm.
        //   3. non-owner of multi-member → leave confirm.
        onTap: selectedCommunity == null
            ? () => _showNoCommunitySelectedSnackbar(context)
            : isSoleMember
                ? () => NavigationHelpers.pushWithSlide(
                      context: context,
                      screen: SoleMemberLeaveExplainerScreen(
                        community: selectedCommunity,
                      ),
                      routeName: 'sole_member_leave_explainer',
                    )
                : isOwner
                    ? () => NavigationHelpers.pushWithSlide(
                          context: context,
                          screen: OwnerLeaveMemberPickerScreen(
                            community: selectedCommunity,
                          ),
                          routeName: 'owner_leave_member_picker',
                        )
                    : () => NavigationHelpers.pushWithSlide(
                          context: context,
                          screen: LeaveCommunityScreen(
                            community: selectedCommunity,
                          ),
                          routeName: 'leave_community',
                        ),
      ),
      if (isOwner)
        _GovernanceAction(
          title: 'Delete Community',
          description: 'Permanently delete this community',
          icon: Icons.delete_outline,
          onTap: () => _onDeleteCommunityTap(context, selectedCommunity),
        ),
    ];
  }

  Future<void> _onDeleteCommunityTap(
    BuildContext context,
    CommunityItem community,
  ) async {
    final choice = await showDangerZoneChoiceModal(
      context: context,
      communityName: communityDisplayName(community, context.l10n),
    );
    if (!context.mounted || choice == null) return;
    switch (choice) {
      case DangerZoneChoice.delete:
        await NavigationHelpers.pushWithSlide(
          context: context,
          screen: DeleteCommunityScreen(community: community),
          routeName: 'delete_community',
        );
      case DangerZoneChoice.leaveWithHandoff:
        // Just-leave routes the same three-way the membership-section
        // Leave button does. If membership has dropped to one since
        // the modal opened, route to the sole-member explainer
        // instead of the picker (which would hit its defensive
        // empty state).
        if (!context.mounted) return;
        final numMembers = ref
            .read(communityProvider(community.id))
            .asData
            ?.value
            .numMembers;
        final isSoleMember = numMembers != null && numMembers <= 1;
        if (isSoleMember) {
          await NavigationHelpers.pushWithSlide(
            context: context,
            screen: SoleMemberLeaveExplainerScreen(community: community),
            routeName: 'sole_member_leave_explainer',
          );
        } else {
          await NavigationHelpers.pushWithSlide(
            context: context,
            screen: OwnerLeaveMemberPickerScreen(community: community),
            routeName: 'owner_leave_member_picker',
          );
        }
      case DangerZoneChoice.cancel:
        return;
    }
  }

  Widget _buildCommunitySection(
    BuildContext context,
    List<_GovernanceAction> actions,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, 'Community'),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: _buildActionItems(context, actions),
        ),
      ],
    );
  }

  Widget _buildMembershipSection(
    BuildContext context,
    List<_GovernanceAction> actions,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, 'Membership'),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: _buildActionItems(context, actions),
        ),
      ],
    );
  }

  Widget _buildDangerSection(
    BuildContext context,
    List<_GovernanceAction> actions,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, 'Danger Zone'),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: _buildActionItems(context, actions),
        ),
      ],
    );
  }

  List<Widget> _buildActionItems(
    BuildContext context,
    List<_GovernanceAction> actions,
  ) {
    final items = <Widget>[];
    for (var i = 0; i < actions.length; i++) {
      items.add(
        buildSettingsActionItem(
          context,
          icon: actions[i].icon,
          title: actions[i].title,
          subtitle: actions[i].description,
          onTap: actions[i].onTap,
        ),
      );
      if (i < actions.length - 1) {
        items.add(buildSettingsDivider(context));
      }
    }
    return items;
  }

  void _navigateToAction(BuildContext context, Widget screen,
      {String? routeName}) {
    NavigationHelpers.pushScreen(
      context: context,
      screen: screen,
      routeName: routeName,
    );
  }

  void _navigateToEditScreen(
    BuildContext context,
    WidgetRef ref,
    String communityId,
  ) async {
    final communityService = ref.read(communityServiceProvider);
    final mediaService = ref.read(mediaServiceProvider);

    final result = await NavigationHelpers.pushScreen(
      context: context,
      screen: CommunityEditScreen(
        communityId: communityId,
        communityService: communityService,
        mediaService: mediaService,
      ),
      routeName: 'community_edit',
    );

    // If the edit was successful, refresh the user's community list so the
    // updated name/description appears everywhere on the next read.
    if (result == true) {
      try {
        final communities = await communityService.listCommunities();
        await ref
            .read(communitiesProvider.notifier)
            .setCommunities(communities);
      } catch (e) {
        // Silently fail - the community list will be refreshed on next load
      }
    }
  }

  void _showNoCommunitySelectedSnackbar(BuildContext context) {
    ToastHelper.showError(context, 'Please select a community first');
  }
}


/// _LockCommunityScreen allows locking the community.
class _LockCommunityScreen extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          'Manage Privacy',
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        leading: AppBarBackButton(
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: Center(
        child: Text(
          'Privacy Management - Coming Soon',
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
      ),
    );
  }
}

/// _ManageRegionScreen allows viewing and manually overriding the community region.
class _ManageRegionScreen extends ConsumerStatefulWidget {
  final String communityId;

  const _ManageRegionScreen({required this.communityId});

  @override
  ConsumerState<_ManageRegionScreen> createState() =>
      _ManageRegionScreenState();
}

class _ManageRegionScreenState extends ConsumerState<_ManageRegionScreen> {
  @override
  void initState() {
    super.initState();
    // Initialize the ViewModel with the community ID
    Future.microtask(() {
      ref.read(governanceProvider.notifier).initialize(widget.communityId);
    });
  }

  Future<void> _openLocationPicker() async {
    final locationController = TextEditingController();
    LocationResult? selectedLocation;

    // Show location search dialog
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => Dialog(
        backgroundColor: AppColors.surface(context),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                'Select Region',
                style: TextStyle(
                  color: AppColors.textPrimary(context),
                  fontSize: 20,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 16),
              LocationAutocompleteField(
                controller: locationController,
                onLocationSelected: (location) {
                  selectedLocation = location;
                },
                search: ref
                    .read(governanceProvider.notifier)
                    .searchGeographicAreas,
                hintText: 'Search for city, county, or state...',
                geographicAreasOnly: true,
              ),
              const SizedBox(height: 16),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton(
                    onPressed: () => Navigator.of(context).pop(false),
                    child: Text(
                      context.l10n.commonCancel,
                      style: TextStyle(color: AppColors.textSecondary(context)),
                    ),
                  ),
                  const SizedBox(width: 8),
                  ElevatedButton(
                    onPressed: () => Navigator.of(context).pop(true),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppColors.primary(context),
                    ),
                    child: const Text('Set Region'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );

    // Delay disposal to allow dialog animation to complete
    Future.delayed(const Duration(milliseconds: 300), () {
      locationController.dispose();
    });

    if ((confirmed ?? false) && selectedLocation != null) {
      await _setOverrideFromLocation(selectedLocation!);
    }
  }

  Future<void> _editRegion(CommunityRegionItem region) async {
    final locationController = TextEditingController();
    LocationResult? selectedLocation;

    // Fetch current region to show display name
    final locationRepository = ref.read(locationRepositoryProvider);
    RegionItem? currentRegion;
    try {
      currentRegion = await locationRepository.getRegion(region.regionId);
      locationController.text = currentRegion.displayName;
    } catch (e) {
      // Continue without pre-filled name
    }

    if (!mounted) return;

    // Show location search dialog
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => Dialog(
        backgroundColor: AppColors.surface(context),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                'Edit Region',
                style: TextStyle(
                  color: AppColors.textPrimary(context),
                  fontSize: 20,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 16),
              LocationAutocompleteField(
                controller: locationController,
                onLocationSelected: (location) {
                  selectedLocation = location;
                },
                search: ref
                    .read(governanceProvider.notifier)
                    .searchGeographicAreas,
                hintText: 'Search for city, county, or state...',
                geographicAreasOnly: true,
              ),
              const SizedBox(height: 16),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton(
                    onPressed: () => Navigator.of(context).pop(false),
                    child: Text(
                      context.l10n.commonCancel,
                      style: TextStyle(color: AppColors.textSecondary(context)),
                    ),
                  ),
                  const SizedBox(width: 8),
                  ElevatedButton(
                    onPressed: () => Navigator.of(context).pop(true),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppColors.primary(context),
                    ),
                    child: const Text('Update Region'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );

    // Delay disposal to allow dialog animation to complete
    Future.delayed(const Duration(milliseconds: 300), () {
      locationController.dispose();
    });

    if ((confirmed ?? false) && selectedLocation != null) {
      await _setOverrideFromLocation(selectedLocation!);
    }
  }

  Future<void> _setOverrideFromLocation(LocationResult location) async {
    try {
      // Call the ViewModel to handle the region override
      await ref
          .read(governanceProvider.notifier)
          .setRegionOverride(location: location);

      if (mounted) {
        ToastHelper.showSuccess(context, 'Region updated successfully');
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to update region: $e');
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final governanceState = ref.watch(governanceProvider);

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          'Manage Region',
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        leading: AppBarBackButton(
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: governanceState.isLoading
          ? const Center(child: CircularProgressIndicator())
          : SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildCurrentRegionsSection(governanceState.regions),
                  const SizedBox(height: 24),
                  _buildActionSection(),
                  const SizedBox(height: 24),
                  _buildExplanationSection(),
                ],
              ),
            ),
    );
  }

  Widget _buildExplanationSection() {
    return Card(
      color: AppColors.surface(context),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.info_outline,
                  color: AppColors.primary(context),
                  size: 20,
                ),
                const SizedBox(width: 8),
                Text(
                  'How Automatic Regions Work',
                  style: TextStyle(
                    color: AppColors.textPrimary(context),
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              'Regions are automatically computed based on where your members live. '
              'If 50% or more members share the same location (neighborhood, city, county, or state), '
              'the community will appear in that region.',
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
                height: 1.5,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'You can manually override this to set a specific region for your community.',
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildCurrentRegionsSection(List<CommunityRegionItem> regions) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Current Regions',
          style: TextStyle(
            color: AppColors.textPrimary(context),
            fontSize: 18,
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: 12),
        if (regions.isEmpty)
          Card(
            color: AppColors.surface(context),
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Text(
                'No regions computed yet. Regions will appear once members add their locations.',
                style: TextStyle(color: AppColors.textSecondary(context)),
              ),
            ),
          )
        else
          ...regions.map((region) => _buildRegionCard(region)),
      ],
    );
  }

  Widget _buildRegionCard(CommunityRegionItem region) {
    final locationRepository = ref.read(locationRepositoryProvider);

    return Card(
      color: AppColors.surface(context),
      margin: const EdgeInsets.only(bottom: 8),
      child: FutureBuilder<RegionItem>(
        future: locationRepository.getRegion(region.regionId),
        builder: (context, snapshot) {
          if (!snapshot.hasData) {
            return const Padding(
              padding: EdgeInsets.all(16),
              child: Center(child: CircularProgressIndicator()),
            );
          }

          final regionItem = snapshot.data!;

          return Tappable(
            semanticsLabel: regionItem.displayName,
            onTap: () => _editRegion(region),
            inkBorderRadius: BorderRadius.circular(12),
            excludeChildSemantics: false,
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Icon(
                    _getRegionIcon(regionItem.regionType),
                    color: AppColors.primary(context),
                    size: 24,
                  ),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          regionItem.displayName,
                          style: TextStyle(
                            color: AppColors.textPrimary(context),
                            fontWeight: FontWeight.w500,
                            fontSize: 16,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                        const SizedBox(height: 4),
                        Text(
                          region.isOverride
                              ? _capitalizeRegionType(regionItem.regionType)
                              : '${_capitalizeRegionType(regionItem.regionType)} • ${(region.memberPercentage * 100).toStringAsFixed(0)}% of members',
                          style: TextStyle(
                            color: AppColors.textSecondary(context),
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                  Icon(
                    Icons.edit,
                    color: AppColors.textSecondary(context),
                    size: 20,
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }

  IconData _getRegionIcon(String regionType) {
    switch (regionType) {
      case 'neighborhood':
        return Icons.home_work;
      case 'city':
        return Icons.location_city;
      case 'county':
        return Icons.landscape;
      case 'state':
        return Icons.map;
      default:
        return Icons.location_on;
    }
  }

  Widget _buildActionSection() {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton.icon(
        onPressed: _openLocationPicker,
        style: ElevatedButton.styleFrom(
          backgroundColor: AppColors.primary(context),
          padding: const EdgeInsets.symmetric(vertical: 16),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(8),
          ),
        ),
        icon: const Icon(Icons.edit_location),
        label: const Text(
          'Set Manual Override',
          style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        ),
      ),
    );
  }

  /// Capitalizes region type for display (e.g., "city" -> "City").
  String _capitalizeRegionType(String regionType) {
    if (regionType.isEmpty) return regionType;
    return regionType[0].toUpperCase() + regionType.substring(1).toLowerCase();
  }
}
