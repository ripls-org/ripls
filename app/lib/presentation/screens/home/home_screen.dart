import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

import '../../../core/config/feature_flags.dart';
import '../../../core/observability/events.dart';
import '../../../core/observability/manager.dart';
import '../../../core/theme/app_colors.dart';
import '../../../core/utils/location_permission_helper.dart';
import '../../../core/utils/ripls_icons.dart';
import '../../../services/app_badge_service.dart';
import '../../../services/connectivity_service.dart';
import '../../../services/gear_service.dart';
import '../../../services/post_creation_service.dart';
import '../../../services/providers.dart';
import '../../providers/screen_modal_provider.dart';
import '../../viewmodels/gen_experience_view_model.dart' show ExperienceCreationResult;
import '../../viewmodels/home_tab_view_model.dart'
    show homeNeedsYouCountProvider;
import '../../viewmodels/home_view_model.dart';
import '../../widgets/navigation/nav_destination.dart';
import '../../widgets/navigation/nav_dock.dart';
import '../../widgets/plus_button_modal.dart';
import '../../widgets/server_error_screen.dart';
import '../../widgets/unread_badge.dart';
import '../communities/invite_sheet.dart';
import '../create/blank_create_dispatcher.dart';
import '../create/unified_create_modal.dart';
import '../directory/directory_screen.dart';
import '../discover/discover_screen.dart';
import '../experience/experience_creation_modal.dart';
import '../feed/feed_screen.dart';
import '../portfolio/home_calendar_screen.dart';
import '../portfolio/home_tab_screen.dart';
import '../workshop/workshop_screen.dart';
class HomeScreen extends ConsumerStatefulWidget {
  final GearService gearService;

  const HomeScreen({required this.gearService, super.key});

  @override
  ConsumerState<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends ConsumerState<HomeScreen> {
  @override
  void initState() {
    super.initState();

    // Communities are already loaded during splash screen (see main.dart)
    // No need to load them again here

    // Log initial screen view after the first frame
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _logScreenViewEvent(ref, 0); // Feed is the default tab
    });
  }

  @override
  void dispose() {
    super.dispose();
  }

  void _onAddTap() {
    // The + button is a user gesture — a valid point to prompt for location
    // the first time (the OS system dialog fires on `denied`, and a grant
    // wires up autofill immediately). But once the user has refused and
    // permission is `deniedForever`, the in-app "Open Settings" alert would
    // be a surprise here: the + flow has multiple actions that don't need
    // location at all (Invite User, Request Something), so we shouldn't keep
    // pestering. `suppressSettingsDialog: true` keeps the first-time prompt
    // and silences the repeat dialog (#2002).
    //
    // On web, `geolocator_web` is auto-included with the package; the
    // browser's Geolocation API handles the permission prompt. The
    // unified-create modal's Image tab gracefully degrades to a
    // gallery-only fallback (see `unified_create_camera_layer.dart`'s
    // kIsWeb branch); Text and URL tabs are fully web-safe.
    unawaited(
      requestLocationWithSettingsFallback(
        context,
        ref,
        suppressSettingsDialog: true,
      ),
    );

    // When the unified-create flag is on, skip the legacy chooser
    // (PlusButtonModal) and open the single unified-create modal
    // instead. See docs/design/unified-create.md § Feature flag.
    if (ref.read(unifiedCreateEnabledProvider)) {
      // Await the modal and run post-creation AFTER it pops — mirrors
      // the request flow below. Running handlePostCreation while the
      // modal is still in the route stack races with the new
      // ContentView mount and leaves the feed stuck on a spinner.
      //
      // runPostSaveAndShare owns the whole post-save sequence: it lands
      // the creator on the new entity's own screen FIRST, opens the Share
      // sheet over it (not over the feed — #2724), then toasts the
      // creation and refetches feed + home once the sheet resolves.
      unawaited(UnifiedCreateModal.show(context, ref).then((result) async {
        if (result == null || !mounted) return;
        await runPostSaveAndShare(context, ref, result);
      }));
      return;
    }

    showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => PlusButtonModal(
        onShareGear: () async {
          await openBlankCreateGear(context, ref);
        },
        onInviteExperience: () async {
          // Capture the outer screen context before any async gaps, since
          // the plus-button modal's builder context may be unmounted by the
          // time MarkCompletedModal needs to be shown.
          final screenContext = this.context;
          final result = await ExperienceCreationModal.show(context);
          if (!mounted) return;
          if (result is ExperienceCreationResult && result.isPastEvent) {
            // TODO(#2040): MarkCompletedModal needs a communityId. The pre-#1895
            // path read it from selectedCommunity (always null after #1895). The
            // creation flow should plumb the chosen community through
            // ExperienceCreationResult so this works again. Until then, skip the
            // post-create completion modal — the experience still creates.
            if (!screenContext.mounted) return;
          }
        },
        onRequestSomething: () async {
          final communityState = ref.read(communitiesProvider);

          if (communityState.communities.isEmpty) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(context.l10n.requestCreateNoCommunities)),
            );
            return;
          }

          final requestId = await openBlankCreateRequest(context, ref);

          // If request was created, use PostCreationService for consistent UX
          if (requestId != null && mounted) {
            await ref.read(postCreationServiceProvider).handlePostCreation();
          }
        },
        onInviteUser: () async {
          final inviteContext = this.context;
          final communityState = ref.read(communitiesProvider);
          final communities = communityState.communities;

          if (communities.isEmpty) {
            if (!inviteContext.mounted) return;
            ScaffoldMessenger.of(inviteContext).showSnackBar(
              const SnackBar(content: Text('Join a community to invite someone')),
            );
            return;
          }

          if (!inviteContext.mounted) return;
          await showAccessibleModal(inviteContext,
            isScrollControlled: true,
            backgroundColor: Colors.transparent,
            builder: (context) => InviteSheet(
              communityId: communities.first.id,
              communityName: communities.first.name,
              communities: communities,
              onClose: () => Navigator.of(context).pop(),
            ),
          );
        },
        onClose: () => Navigator.of(context).pop(),
      ),
    );
  }

  void _logScreenViewEvent(WidgetRef ref, int stackIndex) {
    switch (stackIndex) {
      case 0: // Feed
        ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(FeedViewedEvent(communityId: ''));
        break;
      case 2: // Discover
        ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(DiscoverViewedEvent());
        break;
    }
  }

  /// Tap handler for the converged dock (#2634): re-tap scrolls to top,
  /// otherwise switch to the destination's stack entry.
  void _onDestinationTap(RiplsNavDestination destination) {
    final state = ref.read(homeProvider);
    if (state.selectedIndex == destination.stackIndex) {
      scrollToTop(ref, destination.stackIndex);
    } else {
      ref.read(homeProvider.notifier).navigateToTab(destination.stackIndex);
    }
  }

  /// The converged bottom dock (#2634): glass capsule + search orb.
  Widget _buildNavDock() {
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    return Padding(
      padding: EdgeInsets.only(
        left: 14,
        right: 14,
        bottom: bottomPadding > 0 ? bottomPadding : 12,
      ),
      child: NavDock(
        selectedStackIndex:
            ref.watch(homeProvider.select((s) => s.selectedIndex)),
        // The Home badge is the one Needs-you number — the same query
        // that renders the section (hidden at zero).
        homeBadgeCount: ref.watch(homeNeedsYouCountProvider),
        onDestinationTap: _onDestinationTap,
        onCreateTap: _onAddTap,
      ),
    );
  }

  /// Maps a visual bottom nav position to an IndexedStack index for the
  /// legacy flag-off pill (#1895): three items wide, Create in a separate
  /// floating FAB, Discover unreachable.
  /// Visual: 0=Inbox, 1=Feed, 2=Workshop (Inbox is now the first/landing tab)
  /// Stack:  0=Feed, 1=Inbox, 2=Discover (orphaned), 3=Workshop
  int? _visualToStackIndex(int visualIndex) {
    switch (visualIndex) {
      case 0: return 1; // Inbox
      case 1: return 0; // Feed
      case 2: return 3; // Workshop
      default: return null;
    }
  }

  void _onBottomNavTap(int visualIndex) {
    final stackIndex = _visualToStackIndex(visualIndex);
    if (stackIndex == null) return;

    final state = ref.read(homeProvider);
    if (state.selectedIndex == stackIndex) {
      scrollToTop(ref, stackIndex);
    } else {
      ref.read(homeProvider.notifier).navigateToTab(stackIndex);
    }
  }

  PreferredSizeWidget _buildAppBar() {
    final state = ref.watch(homeProvider);
    switch (state.selectedIndex) {
      case 0: // Feed tab
        return _buildHomeAppBar();
      case 1: // Inbox tab
        return _buildHomeAppBar();
      case 2: // Discover tab
        return _buildDiscoverAppBar();
      case 3: // Workshop tab
        return _buildMetricsAppBar();
      default:
        return _buildHomeAppBar();
    }
  }

  PreferredSizeWidget _buildHomeAppBar() {
    return AppBar(
      toolbarHeight: 0,
      backgroundColor: Colors.transparent,
      scrolledUnderElevation: 0,
      elevation: 0,
    );
  }

  PreferredSizeWidget _buildDiscoverAppBar() {
    return AppBar(
      toolbarHeight: 0,
      backgroundColor: Colors.transparent,
      scrolledUnderElevation: 0,
      elevation: 0,
    );
  }

  PreferredSizeWidget _buildMetricsAppBar() {
    return AppBar(
      toolbarHeight: 0,
      backgroundColor: Colors.transparent,
      scrolledUnderElevation: 0,
      elevation: 0,
    );
  }

  /// Maps a stack index back to a visual index for the selection circle.
  /// The Discover stack entry (index 2) is no longer reachable from the
  /// bottom nav but is preserved in the IndexedStack for legacy routing;
  /// the selection circle defaults to Feed when the stack happens to be
  /// pointed there.
  int _stackToVisualIndex(int stackIndex) {
    switch (stackIndex) {
      case 0: return 1; // Feed
      case 1: return 0; // Inbox (now the first tab)
      case 2: return 1; // Discover — unreachable via nav; treat as Feed visually
      case 3: return 2; // Workshop
      default: return 0;
    }
  }

  Widget _buildBottomNavigation() {
    final state = ref.watch(homeProvider);
    final visualIndex = _stackToVisualIndex(state.selectedIndex);

    final bottomPadding = MediaQuery.of(context).padding.bottom;

    return Container(
      // Right margin leaves room for the floating Create FAB (#1895):
      // 20px screen pad + 54px FAB + 12px gap = 86px.
      margin: EdgeInsets.only(
        left: 20,
        right: 86,
        bottom: bottomPadding > 0 ? bottomPadding : 12,
      ),
      height: 54,
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context).withAlpha(217),
        borderRadius: BorderRadius.circular(27),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withAlpha(30),
            blurRadius: 12,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: LayoutBuilder(
        builder: (context, constraints) {
          // 3 visual items: Feed, Inbox, Workshop. Create is a separate
          // floating FAB rendered to the right of this pill (#1895).
          final itemWidth = constraints.maxWidth / 3;
          const circleSize = 44.0;

          final circleLeft =
              (visualIndex * itemWidth) + (itemWidth - circleSize) / 2;

          return Stack(
            children: [
              AnimatedPositioned(
                duration: accessibleDuration(context, const Duration(milliseconds: 300)),
                curve: Curves.easeInOut,
                left: circleLeft,
                top: (54 - circleSize) / 2,
                child: Container(
                  width: circleSize,
                  height: circleSize,
                  decoration: BoxDecoration(
                    color: AppColors.cardBackground(context),
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: AppColors.primary(context),
                      width: 1.5,
                    ),
                  ),
                ),
              ),
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceAround,
                children: [
                  // The Inbox (Home) is now the first/landing tab; the Feed
                  // (compass) moves to second.
                  _buildNavItem(
                    icon: Icons.home_outlined,
                    activeIcon: Icons.home,
                    visualIndex: 0,
                    currentVisualIndex: visualIndex,
                    // The Home badge is the one Needs-you number — the same
                    // query that renders the section (hidden at zero).
                    badgeCount: ref.watch(homeNeedsYouCountProvider),
                    label: context.l10n.a11yMiscNavHome,
                  ),
                  _buildNavItem(
                    icon: Icons.explore_outlined,
                    activeIcon: Icons.explore,
                    visualIndex: 1,
                    currentVisualIndex: visualIndex,
                    label: context.l10n.a11yMiscNavFeed,
                  ),
                  if (ref.watch(directoryEnabledProvider))
                    _buildNavItem(
                      icon: Icons.people_outline,
                      activeIcon: Icons.people,
                      visualIndex: 2,
                      currentVisualIndex: visualIndex,
                      label: context.l10n.a11yMiscNavDirectory,
                    )
                  else
                    _buildNavItem(
                      icon: RiplsIcons.ripls,
                      activeIcon: RiplsIcons.ripls,
                      visualIndex: 2,
                      currentVisualIndex: visualIndex,
                      label: context.l10n.a11yMiscNavWorkshop,
                    ),
                ],
              ),
            ],
          );
        },
      ),
    );
  }

  Widget _buildNavItem({
    required IconData icon,
    required IconData activeIcon,
    required int visualIndex,
    required int currentVisualIndex,
    required String label,
    int badgeCount = 0,
    bool showDot = false,
  }) {
    final isSelected = visualIndex == currentVisualIndex;
    return Expanded(
      child: Tappable(
        semanticsLabel: label,
        onTap: () => _onBottomNavTap(visualIndex),
        inkBorderRadius: BorderRadius.zero,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 2),
          child: Center(
            child: Stack(
              clipBehavior: Clip.none,
              children: [
                SizedBox(
                  width: 44,
                  height: 44,
                  child: Icon(
                    isSelected ? activeIcon : icon,
                    size: 26,
                    color: isSelected
                        ? AppColors.primary(context)
                        : AppColors.textSecondary(context),
                  ),
                ),
                if (badgeCount > 0)
                  Positioned(
                    right: -8,
                    top: -4,
                    child: UnreadBadge(
                      count: badgeCount,
                      fontSize: 10,
                      padding: const EdgeInsets.symmetric(
                        horizontal: 5,
                        vertical: 2,
                      ),
                      backgroundColor: AppColors.transferCoral,
                      textColor: Colors.white,
                    ),
                  ),
                if (showDot)
                  const Positioned(
                    right: 2,
                    top: 2,
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        color: Color(0xFFF97316),
                        shape: BoxShape.circle,
                      ),
                      child: SizedBox(width: 8, height: 8),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Floating Create button (#1895). Lives outside the bottom-nav pill,
  /// positioned bottom-right and vertically level with the pill. Tapping
  /// opens the same PlusButtonModal the old bottom-nav Create item used.
  Widget _buildCreateFab() {
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    return Padding(
      padding: EdgeInsets.only(
        right: 20,
        bottom: bottomPadding > 0 ? bottomPadding : 12,
      ),
      child: Tappable(
        semanticsLabel: context.l10n.a11yMiscNavAdd,
        onTap: _onAddTap,
        inkBorderRadius: BorderRadius.circular(27),
        child: Container(
          width: 54,
          height: 54,
          decoration: BoxDecoration(
            color: Colors.white,
            shape: BoxShape.circle,
            boxShadow: [
              BoxShadow(
                color: Colors.black.withAlpha(30),
                blurRadius: 12,
                offset: const Offset(0, 4),
              ),
            ],
          ),
          child: Icon(
            Icons.add,
            color: AppColors.primary(context),
            size: 28,
          ),
        ),
      ),
    );
  }

  Widget _buildBody() {
    // Watch only selectedIndex. Watching the whole HomeState would rebuild
    // the IndexedStack on every isNavVisible toggle, and a rebuild that
    // lands during an upstream LayoutBuilder pass can retake an inactive
    // GlobalKey'd subtree (the discover Navigator) mid-layout — Flutter
    // rejects that with "RenderIgnorePointer was mutated in performLayout".
    final selectedIndex =
        ref.watch(homeProvider.select((s) => s.selectedIndex));
    // Read provider values at build time and capture into locals. The
    // Navigator's onGenerateRoute -> MaterialPageRoute.builder closures
    // can fire after this State is deactivated (e.g. during home teardown),
    // and `ref` is unsafe to touch then (#2150).
    final discoverKey = ref.watch(discoverKeyProvider);
    final discoverNavigatorKey = ref.read(discoverNavigatorKeyProvider);

    // Use IndexedStack to preserve screen state when switching tabs.
    // Wrap with ObservabilityConsentTrigger to show consent dialog when needed.
    return ObservabilityConsentTrigger(
      child: IndexedStack(
        index: selectedIndex,
        children: [
          // Index 0: Feed tab
          FeedScreen(
            scrollController: ref.read(feedScrollControllerProvider),
          ),
          // Index 1: Home tab (#2435) — the v4.1 sectioned Home view.
          const HomeTabScreen(),
          // Index 2: Discover tab with nested navigator
          Navigator(
            key: discoverNavigatorKey,
            onGenerateRoute: (settings) {
              return MaterialPageRoute(
                builder: (context) => DiscoverScreen(
                  key: discoverKey,
                ),
              );
            },
          ),
          // Index 3: Directory (address book, #2568) when enabled, else the
          // legacy Workshop tab. With the dock (#2634) this is the People
          // destination.
          if (ref.watch(directoryEnabledProvider))
            const DirectoryScreen()
          else
            WorkshopScreen(
              scrollController: ref.read(workshopScrollControllerProvider),
            ),
          // Index 4: Plans (#2634) — the Home calendar promoted to a
          // top-level destination. Only reachable from the dock, so the
          // legacy flag-off nav simply never selects it.
          const HomeCalendarScreen(embedded: true),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    // Update the OS app-icon badge to match the bottom-nav Home badge:
    // the single Needs-you number (decisions + unread threads), #2435.
    ref.listen<int>(homeNeedsYouCountProvider, (previous, next) {
      AppBadgeService.updateBadge(next);
    });

    // Log screen view analytics when tab changes
    ref.listen<HomeState>(homeProvider, (previous, next) {
      if (previous?.selectedIndex != next.selectedIndex) {
        _logScreenViewEvent(ref, next.selectedIndex);
      }
    });

    // When community loading fails, replace the entire home UI (including
    // bottom nav) with a full-screen error. All tabs need the server, so
    // there is nothing useful behind the nav to navigate to.
    final communityState = ref.watch(communitiesProvider);
    if (communityState.errorMessage != null) {
      final isOffline = ref
          .watch(connectivityStatusProvider)
          .maybeWhen(data: (online) => !online, orElse: () => false);
      return ServerErrorScreen(
        errorMessage: communityState.errorMessage!,
        isOffline: isOffline,
        isRetrying: communityState.isLoading,
        onRetry: () =>
            ref.read(communitiesProvider.notifier).retryLoadCommunities(),
      );
    }

    // Watch screen modal provider for modal layer above bottom nav
    final screenModal = ref.watch(screenModalProvider);

    // On desktop, wrap the entire scaffold to offset it by sidebar width

    // After #1895 the sidebar is gone; desktop and mobile share the
    // same bottom-nav + floating-Create-FAB layout. Account actions
    // live in the Workshop tab's profile-menu modal.
    final homeState = ref.watch(homeProvider);
    // Inbox pins the bottom nav and Create FAB open so account-menu /
    // edit-mode affordances stay reachable regardless of any prior
    // tab's hide-nav state (#1963).
    final pinFloatingNavs =
        homeState.isNavVisible || homeState.selectedIndex == 1;
    return Stack(
      children: [
        Scaffold(
          extendBodyBehindAppBar: true,
          appBar: _buildAppBar(),
          body: Stack(
            children: [
              _buildBody(),
              if (ref.watch(navDockEnabledProvider))
                // Converged dock (#2634): one floating overlay holds the
                // glass capsule and the search orb.
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: 0,
                  child: AnimatedSlide(
                    duration: accessibleDuration(
                        context, const Duration(milliseconds: 200)),
                    offset:
                        pinFloatingNavs ? Offset.zero : const Offset(0, 1.5),
                    child: _buildNavDock(),
                  ),
                )
              else ...[
                // Legacy pill + FAB (#1895), kept as the flag-off rollback.
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: -10,
                  child: AnimatedSlide(
                    duration: accessibleDuration(
                        context, const Duration(milliseconds: 200)),
                    offset:
                        pinFloatingNavs ? Offset.zero : const Offset(0, 1.5),
                    child: _buildBottomNavigation(),
                  ),
                ),
                Positioned(
                  right: 0,
                  bottom: -10,
                  child: AnimatedSlide(
                    duration: accessibleDuration(
                        context, const Duration(milliseconds: 200)),
                    offset:
                        pinFloatingNavs ? Offset.zero : const Offset(0, 1.5),
                    child: _buildCreateFab(),
                  ),
                ),
              ],
            ],
          ),
        ),
        // Screen-level modal layer - renders above bottom nav in z-space
        // Screens inject modal content via screenModalProvider
        if (screenModal != null) Positioned.fill(child: screenModal),
      ],
    );
  }
}
