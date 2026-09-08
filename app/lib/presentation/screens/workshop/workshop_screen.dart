import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/screens/workshop/workshop_co2_detail_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_conversation_panel.dart';
import 'package:ripls/presentation/screens/workshop/workshop_library_calendar_panel.dart';
import 'package:ripls/presentation/screens/workshop/workshop_library_map_panel.dart';
import 'package:ripls/presentation/screens/workshop/workshop_money_detail_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_roster_sheet.dart';
import 'package:ripls/presentation/screens/workshop/workshop_time_detail_screen.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/workshop_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel_launcher.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/search/close_search_button.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_backdrop.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_centered_overview.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_morph.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_switcher_dropdown.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_switcher_pill.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show authStateProvider;
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// Top-level Workshop tab — the community overview (#2447).
///
/// A vertical pager (like the Feed): one full-bleed community page per circle —
/// the community photo, a dark scrim, and the centered editorial overview over
/// it. Swiping up/down moves between communities and updates the pinned
/// community ([workshopCommunityProvider]) so the floating switcher pill always
/// names the community on screen. The pill itself opens the full-screen chooser,
/// whose selection jumps the pager back in sync.
///
/// Each page overrides [workshopEnabledCommunityIdsProvider] to its own circle
/// so the brief, member rollup, glimpse, and library all resolve per-page.
class WorkshopScreen extends ConsumerStatefulWidget {
  const WorkshopScreen({super.key, this.scrollController});

  /// Optional vertical pager controller injected by the host so that re-tapping
  /// the Workshop bottom-nav item can scroll back to the first community page
  /// (same behavior as the feed). When null the screen owns its own controller.
  final PageController? scrollController;

  @override
  ConsumerState<WorkshopScreen> createState() => _WorkshopScreenState();
}

class _WorkshopScreenState extends ConsumerState<WorkshopScreen> {
  late final PageController _controller =
      widget.scrollController ?? PageController();
  final TextEditingController _searchController = TextEditingController();
  bool _syncedInitial = false;

  /// Whether the community-switcher dropdown is open (pill is the search box).
  bool _switcherOpen = false;
  String _query = '';

  @override
  void initState() {
    super.initState();
    // Re-align the pager when the community LIST changes (e.g. ListCommunities
    // re-sorts after a nameless community is promoted to a name): the pinned
    // community's page index may move while the pin itself is unchanged,
    // leaving the pager on a different community than the pill names (#2492).
    // Registered here via listenManual rather than ref.listen in build (which
    // would re-subscribe on every rebuild).
    ref.listenManual(communitiesProvider.select((s) => s.communities),
        (prev, next) {
      final pin = ref.read(workshopCommunityProvider);
      if (pin == null) return;
      final i = next.indexWhere((c) => c.id == pin);
      if (i < 0) return;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted &&
            _controller.hasClients &&
            (_controller.page?.round() ?? 0) != i) {
          _controller.jumpToPage(i);
        }
      });
    });
  }

  @override
  void dispose() {
    // Only dispose the controller we created; an injected one is owned by the
    // provider that supplied it.
    if (widget.scrollController == null) {
      _controller.dispose();
    }
    _searchController.dispose();
    super.dispose();
  }

  /// Settling on a page pins that community so the top-nav pill follows.
  void _onPageChanged(List<CommunityItem> communities, int index) {
    if (index < 0 || index >= communities.length) return;
    ref.read(workshopCommunityProvider.notifier).select(communities[index].id);
  }

  void _openSwitcher() => setState(() => _switcherOpen = true);

  void _closeSwitcher() {
    _searchController.clear();
    setState(() {
      _switcherOpen = false;
      _query = '';
    });
  }

  void _selectCommunity(String id) {
    ref.read(workshopCommunityProvider.notifier).select(id);
    _closeSwitcher();
  }

  Future<void> _createCommunity() async {
    _closeSwitcher();
    await CommunityCreationModal.show(context);
  }

  @override
  Widget build(BuildContext context) {
    final communities =
        ref.watch(communitiesProvider.select((s) => s.communities));
    final expanded = ref.watch(workshopContentExpandedProvider);
    final safeAreaTop = MediaQuery.of(context).padding.top;

    // Keep the pager and the pinned community in sync when the selection
    // changes from elsewhere (the chooser).
    ref.listen<String?>(workshopCommunityProvider, (prev, next) {
      if (next == null || !_controller.hasClients) return;
      final i = communities.indexWhere((c) => c.id == next);
      if (i >= 0 && (_controller.page?.round() ?? 0) != i) {
        _controller.jumpToPage(i);
      }
    });

    if (communities.isEmpty) {
      return Scaffold(
        backgroundColor: const Color(0xFF0A0C0A),
        body: const Stack(
          fit: StackFit.expand,
          children: [
            ColoredBox(color: Color(0xFF0A0C0A)),
            WorkshopBackdrop(mediaId: null),
            _OverviewScrim(),
            Center(child: _EmptyPlaceholder()),
          ],
        ),
      );
    }

    // On first build with communities available, jump to the pinned page (e.g.
    // a selection that survived a tab rebuild).
    if (!_syncedInitial) {
      _syncedInitial = true;
      final pin = ref.read(workshopCommunityProvider);
      final i = pin == null ? 0 : communities.indexWhere((c) => c.id == pin);
      if (i > 0) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted && _controller.hasClients) _controller.jumpToPage(i);
        });
      }
    }

    return Scaffold(
      backgroundColor: const Color(0xFF0A0C0A),
      body: Stack(
        fit: StackFit.expand,
        children: [
          const ColoredBox(color: Color(0xFF0A0C0A)),
          PageView.builder(
            controller: _controller,
            scrollDirection: Axis.vertical,
            itemCount: communities.length,
            onPageChanged: (i) => _onPageChanged(communities, i),
            itemBuilder: (ctx, i) {
              final community = communities[i];
              // Scope the workshop providers to this page's community so its
              // brief / rollup / library resolve independently of the pin.
              return ProviderScope(
                overrides: [
                  workshopEnabledCommunityIdsProvider
                      .overrideWithValue([community.id]),
                ],
                child: _CommunityPage(community: community),
              );
            },
          ),
          // Scrim dims the community content behind the open dropdown; tapping
          // it (outside the panel) closes the switcher.
          if (_switcherOpen && !expanded)
            Positioned.fill(
              child: Tappable(
                semanticsLabel: context.l10n.a11yWorkshopCloseChooser,
                onTap: _closeSwitcher,
                child: const ColoredBox(color: Color(0x52141612)),
              ),
            ),
          // The switcher dropdown, anchored just below the floating header.
          if (_switcherOpen && !expanded)
            Positioned(
              top: FloatingHeader.contentTop(context),
              left: 16,
              right: 16,
              child: ConstrainedBox(
                constraints: BoxConstraints(
                  maxHeight: MediaQuery.of(context).size.height -
                      FloatingHeader.contentTop(context) -
                      28,
                ),
                child: WorkshopSwitcherDropdown(
                  query: _query,
                  onSelect: _selectCommunity,
                  onCreate: _createCommunity,
                ),
              ),
            ),
          // Shared FloatingHeader (same floating top nav as Feed/Inbox). The
          // switcher pill names the community on screen; tapping opens the
          // dropdown (the pill then becomes its search field). Hidden while a
          // morph panel is expanded.
          if (!expanded)
            Positioned(
              top: safeAreaTop,
              left: 0,
              right: 0,
              child: FloatingHeader(
                content: WorkshopSwitcherPill(
                  searching: _switcherOpen,
                  onTap: _openSwitcher,
                  controller: _searchController,
                  onQueryChanged: (q) => setState(() => _query = q),
                ),
                // Like the Feed: while searching the trailing slot is a Close X
                // (sitting outside the pill), not an in-pill button.
                trailing: _switcherOpen
                    ? CloseSearchButton(
                        onTap: _closeSwitcher,
                        semanticsLabel: context.l10n.a11ySearchDismiss,
                      )
                    : null,
              ),
            ),
        ],
      ),
    );
  }
}

/// One community page: its photo backdrop + scrim + centered overview. Reads
/// the workshop providers scoped (by the enclosing [ProviderScope]) to this
/// community.
class _CommunityPage extends ConsumerWidget {
  const _CommunityPage({required this.community});

  final CommunityItem community;

  String? get _backdropMediaId =>
      community.mediaIds.isNotEmpty ? community.mediaIds.first : null;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(workshopProvider);
    // While a morph panel is open, hide the overview body so the panel overlays
    // the community photo alone (the panel grows over this page's backdrop).
    final expanded = ref.watch(workshopContentExpandedProvider);

    return Stack(
      fit: StackFit.expand,
      children: [
        const ColoredBox(color: Color(0xFF0A0C0A)),
        WorkshopBackdrop(mediaId: _backdropMediaId),
        const _OverviewScrim(),
        if (!expanded)
          Positioned.fill(
            top: FloatingHeader.contentTop(context),
            child: state.when(
              data: (data) => _body(context, ref, data),
              loading: () => const Center(
                child: CircularProgressIndicator(
                  color: WorkshopOverviewPalette.onPhoto,
                ),
              ),
              error: (e, _) => Center(
                child: Padding(
                  padding: const EdgeInsets.all(24),
                  child: Text(
                    e.toString(),
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                      color: WorkshopOverviewPalette.onPhotoDim,
                    ),
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }

  Widget _body(BuildContext context, WidgetRef ref, WorkshopState data) {
    // Always render the overview — every section shows a zero-state prompt
    // when it has nothing yet, rather than collapsing the whole page.
    final brief = data.brief;
    final communityIds = [community.id];

    final hours = (brief?.hasHoursTogether() ?? false) ? brief!.hoursTogether : 0;
    final dollars =
        (brief?.hasReplacedCostUsd() ?? false) ? brief!.replacedCostUsd : 0;
    // BriefPayload carries pounds; convert to kg (same 2.205 factor as before).
    final co2Kg = (brief?.hasCo2AvoidedPounds() ?? false)
        ? brief!.co2AvoidedPounds / 2.205
        : 0.0;
    final problems = (brief?.hasProblemsSolvedCount() ?? false)
        ? brief!.problemsSolvedCount
        : 0;
    final problemsPotential = (brief?.hasProblemsPotentialCount() ?? false)
        ? brief!.problemsPotentialCount
        : 0;
    final memberCount =
        (brief?.hasUniquePeopleCount() ?? false) ? brief!.uniquePeopleCount : 0;
    final items = brief?.availableNowItems.toList() ?? const <Item>[];
    final specialties = brief?.specialties.toList() ?? const <String>[];

    return WorkshopCenteredOverview(
      communityId: community.id,
      communityName: community.name,
      originItemName: community.originItemName,
      hours: hours,
      dollars: dollars,
      co2Kg: co2Kg,
      problems: problems,
      problemsPotential: problemsPotential,
      memberCount: memberCount,
      items: items,
      specialties: specialties,
      onTapMembers: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        const WorkshopRosterSheet(overlay: true),
        'workshop_roster',
      ),
      onTapTime: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopTimeDetailScreen(communityIds: communityIds, overlay: true),
        'workshop_time_detail',
      ),
      onTapMoney: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopMoneyDetailScreen(communityIds: communityIds, overlay: true),
        'workshop_money_detail',
      ),
      onTapCo2: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopCo2DetailScreen(communityIds: communityIds, overlay: true),
        'workshop_co2_detail',
      ),
      onTapProblems: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopProblemsSolvedDetailScreen(
          communityIds: communityIds,
          overlay: true,
        ),
        'workshop_problems_detail',
      ),
      onTapConversation: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopConversationPanel(communityId: community.id),
        'workshop_conversation',
      ),
      onTapCalendar: (rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopLibraryCalendarPanel(communityIds: communityIds),
        'workshop_library_calendar',
      ),
      onTapCommonGround: (category, rect) => _expandOverlayScreen(
        context,
        ref,
        rect,
        WorkshopLibraryMapPanel(
          communityIds: communityIds,
          category: category,
        ),
        'workshop_library_map',
      ),
      onInvite: () => _invite(context, ref, community),
      // A nameless ad-hoc community the viewer owns can be promoted (named) from
      // the header pill — the create/AI modal in promote mode (#2492).
      onNameGroup: (community.name.trim().isEmpty &&
              community.ownerUserId.isNotEmpty &&
              community.ownerUserId == ref.read(authStateProvider).user?.id)
          ? () => CommunityCreationModal.show(context,
              promoteCommunityId: community.id)
          : null,
    );
  }

  /// Opens the community invite sheet — the zero-state's primary CTA.
  void _invite(BuildContext context, WidgetRef ref, CommunityItem community) {
    final communities = ref.read(communitiesProvider).communities;
    showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (ctx) => InviteSheet(
        communityId: community.id,
        communityName: community.name,
        communities: communities,
        onClose: () => Navigator.of(ctx).pop(),
      ),
    );
  }

  /// Morph-opens a destination [screen] hosted under [WorkshopOverlayHost] so it
  /// overlays this community's photo, growing from [rect].
  void _expandOverlayScreen(
    BuildContext context,
    WidgetRef ref,
    Rect rect,
    Widget screen,
    String routeName,
  ) {
    openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: workshopContentExpandedProvider,
      sourceRect: rect,
      routeName: routeName,
      screen: WorkshopOverlayHost(
        backdropMediaId: _backdropMediaId,
        child: screen,
      ),
    );
  }
}

/// The balanced photo scrim (see [WorkshopOverviewPalette.scrimColors]).
class _OverviewScrim extends StatelessWidget {
  const _OverviewScrim();

  @override
  Widget build(BuildContext context) {
    return const DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: WorkshopOverviewPalette.scrimColors,
          stops: WorkshopOverviewPalette.scrimStops,
        ),
      ),
    );
  }
}

class _EmptyPlaceholder extends StatelessWidget {
  const _EmptyPlaceholder();

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 24),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Text(
            context.l10n.workshopEmptyHeadline,
            style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                  color: WorkshopOverviewPalette.onPhoto,
                  fontWeight: FontWeight.w600,
                ),
          ),
          const SizedBox(height: 12),
          Text(
            context.l10n.workshopEmptySubtitle,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  color: WorkshopOverviewPalette.onPhotoDim,
                ),
          ),
        ],
      ),
    );
  }
}
