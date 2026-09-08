import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/recent_searches_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart'
    show searchProvider;
import 'package:ripls/presentation/viewmodels/tab_search_scope_provider.dart';
import 'package:ripls/presentation/viewmodels/universal_search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';
import 'package:ripls/services/providers.dart'
    show communitiesProvider, mediaObjectProvider;
import 'package:ripls/services/search_service.dart'
    show SearchItemType, SearchResultItem, UniversalSearchResponse;

/// The hybrid universal search (#2634, search-hybrid mock): one overlay,
/// one contract, three layers of the same screen.
///
/// * **Empty** — the launchpad: recent searches as one-tap re-runs.
/// * **Matched** — Spotlight grammar: a committable Top Hit, then one
///   capped section per tab in dock order (Plans · Library · People).
///   Each section header carries the honest full count and escapes to
///   its tab scoped to the query; the Do floor pins above the field.
/// * **Unmatched** — the floor becomes the screen: the same two Do rows
///   at full size under one quiet line of copy, so zero-results is
///   structurally impossible.
///
/// The field is welded to the bottom — above the keyboard when it's up,
/// above the dock when it's not. Return commits the Top Hit when matches
/// exist, the ask row when none do.
class UniversalSearchScreen extends ConsumerStatefulWidget {
  const UniversalSearchScreen({super.key});

  /// Pushes the search overlay with a plain fade so the veiled tab
  /// beneath stays visually in place.
  static Future<void> show(BuildContext context) {
    return Navigator.of(context, rootNavigator: true).push(
      PageRouteBuilder<void>(
        opaque: false,
        barrierColor: Colors.transparent,
        pageBuilder: (context, animation, secondaryAnimation) =>
            const UniversalSearchScreen(),
        transitionsBuilder: (context, animation, secondaryAnimation, child) =>
            FadeTransition(opacity: animation, child: child),
      ),
    );
  }

  @override
  ConsumerState<UniversalSearchScreen> createState() =>
      _UniversalSearchScreenState();
}

class _UniversalSearchScreenState extends ConsumerState<UniversalSearchScreen> {
  /// Rows shown per section before the honest-count escape absorbs the
  /// tail (density rule from the hybrid mock).
  static const int _sectionCap = 3;

  final TextEditingController _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(universalSearchProvider);

    return Scaffold(
      // Solid fill: results must read on their own surface, not over a
      // see-through veil of the tab beneath (the route still fades in,
      // so the tab shows only during the transition).
      backgroundColor: AppColors.background(context),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(child: _buildBody(context, state)),
            // The Do floor pins above the field whenever matches exist —
            // ask and offer stay one thumb away at any result count.
            if (state.hasResults) _buildDoFloor(context, state),
            Padding(
              padding: const EdgeInsets.fromLTRB(14, 8, 14, 14),
              child: _buildSearchField(context, state),
            ),
          ],
        ),
      ),
    );
  }

  // ─── Body: launchpad · matched · unmatched ───

  Widget _buildBody(BuildContext context, UniversalSearchState state) {
    if (state.query.trim().isEmpty) return _buildLaunchpad(context);
    if (state.isLoading && !state.hasResults) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.hasError) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Text(
            RpcErrorHandler.localize(state.error!, context.l10n),
            textAlign: TextAlign.center,
            style: TextStyle(color: AppColors.textSecondary(context)),
          ),
        ),
      );
    }
    if (state.isEmptyResult) return _buildUnmatched(context, state);
    if (!state.hasResults) return const SizedBox.shrink();
    return _buildMatched(context, state);
  }

  // ─── State 1 — launchpad (recents as one-tap re-runs) ───

  Widget _buildLaunchpad(BuildContext context) {
    final l10n = context.l10n;
    final recents =
        ref.watch(recentSearchesProvider).asData?.value ?? const <String>[];
    if (recents.isEmpty) return const SizedBox.shrink();

    return ListView(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: const EdgeInsets.fromLTRB(14, 8, 14, 8),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(6, 8, 6, 2),
          child: Row(
            children: [
              _groupLabel(context, l10n.searchRecentsHeader),
              const Spacer(),
              Tappable(
                semanticsLabel: l10n.searchRecentsClear,
                onTap: () => unawaited(
                    ref.read(recentSearchesProvider.notifier).clear()),
                inkBorderRadius: BorderRadius.circular(8),
                child: Text(
                  l10n.searchRecentsClear,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.primary(context),
                  ),
                ),
              ),
            ],
          ),
        ),
        for (final query in recents) _recentRow(context, query),
      ],
    );
  }

  Widget _recentRow(BuildContext context, String query) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Tappable(
        semanticsLabel: query,
        onTap: () => _rerun(query),
        inkBorderRadius: BorderRadius.circular(16),
        child: Container(
          padding: const EdgeInsets.fromLTRB(12, 4, 4, 4),
          decoration: _cardDecoration(context),
          child: Row(
            children: [
              Icon(Icons.history,
                  size: 16, color: AppColors.textSecondary(context)),
              const SizedBox(width: 11),
              Expanded(
                child: Text(
                  query,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 13.5,
                    fontWeight: FontWeight.w500,
                    color: AppColors.textPrimary(context),
                  ),
                ),
              ),
              Tappable(
                semanticsLabel: context.l10n.a11ySearchRemoveRecent(query),
                onTap: () => unawaited(
                    ref.read(recentSearchesProvider.notifier).remove(query)),
                inkBorderRadius: BorderRadius.circular(16),
                child: Padding(
                  padding: const EdgeInsets.all(10),
                  child: Icon(Icons.close,
                      size: 15, color: AppColors.textSecondary(context)),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _rerun(String query) {
    _controller.text = query;
    _controller.selection =
        TextSelection.collapsed(offset: query.length);
    ref.read(universalSearchProvider.notifier).onQueryChanged(query);
  }

  // ─── State 2 — matched (Top Hit + capped sections) ───

  Widget _buildMatched(BuildContext context, UniversalSearchState state) {
    final l10n = context.l10n;
    final response = state.response!;
    final topHit = _topHit(response);

    // One section per dock destination. The header — name + honest full
    // count — is the escape: tapping it opens that tab showing only this
    // query's results (the tab keeps a search pill so the user can
    // cancel). Rows stay capped; the count covers the whole tab.
    Widget section(
      String title,
      List<SearchResultItem> items,
      RiplsNavDestination destination,
    ) {
      if (items.isEmpty) return const SizedBox.shrink();
      final rows =
          items.where((i) => !identical(i, topHit)).toList(growable: false);
      final shown = rows.take(_sectionCap).toList();
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(6, 12, 6, 4),
            child: Tappable(
              semanticsLabel:
                  l10n.a11ySearchOpenSection(title, items.length),
              onTap: () => _escapeToTab(destination),
              inkBorderRadius: BorderRadius.circular(8),
              child: Row(
                children: [
                  _groupLabel(context, title),
                  const SizedBox(width: 7),
                  Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 7, vertical: 1),
                    decoration: BoxDecoration(
                      color: AppColors.primary(context).withAlpha(31),
                      borderRadius: BorderRadius.circular(9),
                    ),
                    child: Text(
                      '${items.length}',
                      style: TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.w700,
                        color: AppColors.primary(context),
                      ),
                    ),
                  ),
                  const Spacer(),
                  Icon(Icons.chevron_right,
                      size: 16, color: AppColors.textSecondary(context)),
                ],
              ),
            ),
          ),
          for (final item in shown) _resultRow(context, item),
        ],
      );
    }

    return ListView(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: const EdgeInsets.fromLTRB(14, 8, 14, 8),
      children: [
        if (topHit != null) ...[
          Padding(
            padding: const EdgeInsets.fromLTRB(6, 8, 6, 4),
            child: _groupLabel(context, l10n.searchTopHit),
          ),
          _TopHitCard(
            item: topHit,
            onOpen: () => _openResult(topHit),
          ),
        ],
        // Dock order: Plans · Library · People.
        section(
            l10n.navDockPlans, response.plansResults, RiplsNavDestination.plans),
        section(l10n.navDockLibrary, response.libraryResults,
            RiplsNavDestination.library),
        section(l10n.navDockPeople, response.peopleResults,
            RiplsNavDestination.people),
        const SizedBox(height: 8),
      ],
    );
  }

  /// The committable best hit: the top of the first non-empty group in
  /// fixed IA order.
  SearchResultItem? _topHit(UniversalSearchResponse response) {
    if (response.libraryResults.isNotEmpty) {
      return response.libraryResults.first;
    }
    if (response.plansResults.isNotEmpty) return response.plansResults.first;
    if (response.peopleResults.isNotEmpty) return response.peopleResults.first;
    return null;
  }

  /// A section escape hands the user to the canonical tab, scoped to the
  /// current query: the tab shows only this search's results plus a
  /// search pill whose ✕ cancels the scope. Library re-runs its own
  /// search RPC with the query (map pins, shelves and the card rail all
  /// render from it); Plans and People filter client-side via their
  /// scope providers.
  void _escapeToTab(RiplsNavDestination destination) {
    final query = ref.read(universalSearchProvider).query.trim();
    _recordRecent();
    Navigator.of(context).pop();
    switch (destination) {
      case RiplsNavDestination.library:
        ref.read(searchProvider.notifier).searchWithLocation(
              query: query,
              communityIds: ref.read(communitiesProvider).communityIds,
            );
      case RiplsNavDestination.plans:
        ref.read(plansSearchScopeProvider.notifier).set(query);
      case RiplsNavDestination.people:
        ref.read(peopleSearchScopeProvider.notifier).set(query);
      case RiplsNavDestination.home:
        break;
    }
    ref.read(homeProvider.notifier).navigateToTab(destination.stackIndex);
  }

  // ─── State 3 — unmatched (the floor is the screen) ───

  Widget _buildUnmatched(BuildContext context, UniversalSearchState state) {
    final l10n = context.l10n;
    final query = state.query.trim();
    return ListView(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: const EdgeInsets.fromLTRB(14, 12, 14, 8),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(6, 2, 6, 6),
          child: Row(
            children: [
              Icon(Icons.search,
                  size: 15, color: AppColors.textSecondary(context)),
              const SizedBox(width: 9),
              Expanded(
                child: Text(
                  l10n.searchNoMatchLine,
                  style: TextStyle(
                    fontSize: 12.5,
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(6, 4, 6, 4),
          child: _groupLabel(context, l10n.searchDoHeader),
        ),
        _doRow(
          context,
          amber: true,
          title: l10n.searchDoAsk(query),
          subtitle: l10n.searchDoAskSubtitle(query),
          onTap: () => unawaited(_askCommunities(query)),
        ),
        const SizedBox(height: 6),
        _doRow(
          context,
          amber: false,
          title: l10n.searchDoAdd(query),
          subtitle: l10n.searchDoAddSubtitle,
          onTap: () => unawaited(_addToLibrary()),
        ),
      ],
    );
  }

  // ─── The Do floor (pinned when matches exist) ───

  Widget _buildDoFloor(BuildContext context, UniversalSearchState state) {
    final l10n = context.l10n;
    final query = state.query.trim();
    return Container(
      padding: const EdgeInsets.fromLTRB(14, 6, 14, 0),
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: AppColors.border(context))),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(6, 2, 6, 4),
            child: _groupLabel(context, l10n.searchDoHeader),
          ),
          _doRow(
            context,
            amber: true,
            title: l10n.searchDoAsk(query),
            compact: true,
            onTap: () => unawaited(_askCommunities(query)),
          ),
          const SizedBox(height: 5),
          _doRow(
            context,
            amber: false,
            title: l10n.searchDoAdd(query),
            compact: true,
            onTap: () => unawaited(_addToLibrary()),
          ),
        ],
      ),
    );
  }

  /// One Do row — amber = ask (wants something), sage = add (offer).
  /// Compact rows drop the subtitle for the pinned floor.
  Widget _doRow(
    BuildContext context, {
    required bool amber,
    required String title,
    String? subtitle,
    bool compact = false,
    required VoidCallback onTap,
  }) {
    final glyphSize = compact ? 32.0 : 40.0;
    return Tappable(
      semanticsLabel: title,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        padding: EdgeInsets.symmetric(
            horizontal: 12, vertical: compact ? 7 : 10),
        decoration: _cardDecoration(context),
        child: Row(
          children: [
            Container(
              width: glyphSize,
              height: glyphSize,
              decoration: BoxDecoration(
                color: amber
                    ? AppColors.rsvpMaybeBackground(context)
                    : AppColors.primary(context).withAlpha(31),
                borderRadius: BorderRadius.circular(compact ? 9 : 11),
              ),
              child: Icon(
                amber ? Icons.help_outline : Icons.inventory_2_outlined,
                size: compact ? 16 : 18,
                color: amber
                    ? AppColors.rsvpMaybeText(context)
                    : AppColors.primary(context),
              ),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: compact ? 12.5 : 13.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                  if (subtitle != null && subtitle.isNotEmpty)
                    Text(
                      subtitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 11.5,
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                ],
              ),
            ),
            Icon(Icons.chevron_right,
                size: 18, color: AppColors.textSecondary(context)),
          ],
        ),
      ),
    );
  }

  // ─── The welded field ───

  Widget _buildSearchField(BuildContext context, UniversalSearchState state) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Row(
        children: [
          Icon(Icons.search, size: 20, color: AppColors.textSecondary(context)),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: _controller,
              autofocus: true,
              textInputAction: TextInputAction.search,
              decoration: InputDecoration(
                hintText: context.l10n.searchUniversalHint,
                border: InputBorder.none,
              ),
              onChanged:
                  ref.read(universalSearchProvider.notifier).onQueryChanged,
              onSubmitted: (_) => _commit(state),
            ),
          ),
          Tappable(
            semanticsLabel: context.l10n.a11yClose,
            onTap: () => Navigator.of(context).pop(),
            inkBorderRadius: BorderRadius.circular(22),
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Icon(Icons.close,
                  size: 20, color: AppColors.textSecondary(context)),
            ),
          ),
        ],
      ),
    );
  }

  /// Return commits: the Top Hit when matches exist, the ask row when
  /// nothing matches.
  void _commit(UniversalSearchState state) {
    if (state.hasResults) {
      final topHit = _topHit(state.response!);
      if (topHit != null) _openResult(topHit);
      return;
    }
    if (state.isEmptyResult) {
      unawaited(_askCommunities(state.query.trim()));
    }
  }

  // ─── Shared pieces ───

  Widget _groupLabel(BuildContext context, String text) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.2,
        color: AppColors.textSecondary(context),
      ),
    );
  }

  BoxDecoration _cardDecoration(BuildContext context) {
    return BoxDecoration(
      color: AppColors.cardBackground(context),
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: AppColors.border(context)),
    );
  }

  Widget _resultRow(BuildContext context, SearchResultItem item) {
    final (title, subtitle, icon) = switch (item.itemType) {
      SearchItemType.SEARCH_ITEM_TYPE_GEAR => (
          item.gear.name,
          item.gear.owner.name,
          Icons.inventory_2_outlined,
        ),
      SearchItemType.SEARCH_ITEM_TYPE_REQUEST => (
          item.request.title,
          item.request.requester.name,
          Icons.help_outline,
        ),
      SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE => (
          item.experience.name,
          item.experience.owner.name,
          Icons.calendar_month_outlined,
        ),
      _ => (item.user.name, '', Icons.person_outline),
    };
    if (title.isEmpty) return const SizedBox.shrink();

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Tappable(
        semanticsLabel: title,
        onTap: () => _openResult(item),
        inkBorderRadius: BorderRadius.circular(16),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
          decoration: _cardDecoration(context),
          child: Row(
            children: [
              Container(
                width: 36,
                height: 36,
                decoration: BoxDecoration(
                  color: AppColors.surface(context),
                  borderRadius: BorderRadius.circular(10),
                ),
                child:
                    Icon(icon, size: 19, color: AppColors.primary(context)),
              ),
              const SizedBox(width: 11),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context)
                          .textTheme
                          .bodyMedium
                          ?.copyWith(fontWeight: FontWeight.w600),
                    ),
                    if (subtitle.isNotEmpty)
                      Text(
                        subtitle,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                              color: AppColors.textSecondary(context),
                            ),
                      ),
                  ],
                ),
              ),
              Icon(Icons.chevron_right,
                  size: 18, color: AppColors.textSecondary(context)),
            ],
          ),
        ),
      ),
    );
  }

  void _recordRecent() {
    final query = ref.read(universalSearchProvider).query.trim();
    if (query.isEmpty) return;
    unawaited(ref.read(recentSearchesProvider.notifier).add(query));
  }

  void _openResult(SearchResultItem item) {
    _recordRecent();
    switch (item.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        unawaited(NavigationHelpers.pushToItemScreen(
          context: context,
          itemId: item.gear.id,
          itemType: 'gear',
        ));
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        unawaited(NavigationHelpers.pushToItemScreen(
          context: context,
          itemId: item.request.id,
          itemType: 'request',
        ));
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        unawaited(NavigationHelpers.pushToItemScreen(
          context: context,
          itemId: item.experience.id,
          itemType: 'experience',
        ));
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        ContentViewHelpers.openUserScreen(context, item.user.id);
      default:
        break;
    }
  }

  /// Ask flow: close the overlay and open unified create seeded with
  /// the query, so the unmet need becomes an ask.
  Future<void> _askCommunities(String query) async {
    _recordRecent();
    Navigator.of(context).pop();
    // Widget async exception: the create modal is a UI-level bottom sheet
    // and must be shown from the surviving root-navigator context.
    await UnifiedCreateModal.show(context, ref, initialPrompt: query);
  }

  /// Add flow: close the overlay and open the gear capture flow — snap
  /// it, name it, done.
  Future<void> _addToLibrary() async {
    _recordRecent();
    Navigator.of(context).pop();
    // Widget async exception: the create modal is a UI-level bottom sheet
    // and must be shown from the surviving root-navigator context.
    await openBlankCreateGear(context, ref);
  }
}

/// The committable Top Hit card: 64px hero thumbnail, serif title, meta,
/// and an availability pill for gear. The whole card is the action —
/// tapping it (or pressing return) opens the result; there is no
/// separate CTA button.
class _TopHitCard extends ConsumerWidget {
  final SearchResultItem item;
  final VoidCallback onOpen;

  const _TopHitCard({required this.item, required this.onOpen});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final (title, meta, mediaId) = switch (item.itemType) {
      SearchItemType.SEARCH_ITEM_TYPE_GEAR => (
          item.gear.name,
          item.gear.owner.name,
          item.gear.mediaIds.isEmpty ? '' : item.gear.mediaIds.first,
        ),
      SearchItemType.SEARCH_ITEM_TYPE_REQUEST => (
          item.request.title,
          item.request.requester.name,
          item.request.mediaIds.isEmpty ? '' : item.request.mediaIds.first,
        ),
      SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE => (
          item.experience.name,
          item.experience.owner.name,
          item.experience.mediaIds.isEmpty
              ? ''
              : item.experience.mediaIds.first,
        ),
      _ => (item.user.name, '', item.user.mediaId),
    };

    return Tappable(
      semanticsLabel: title,
      onTap: onOpen,
      inkBorderRadius: BorderRadius.circular(20),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: AppColors.border(context)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                children: [
                  _hero(context, ref, mediaId),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontFamily: AppTheme.headingFont,
                            fontSize: 17,
                            fontWeight: FontWeight.w500,
                            color: AppColors.textPrimary(context),
                          ),
                        ),
                        if (meta.isNotEmpty)
                          Text(
                            meta,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 12,
                              color: AppColors.textSecondary(context),
                            ),
                          ),
                        if (item.itemType ==
                            SearchItemType.SEARCH_ITEM_TYPE_GEAR) ...[
                          const SizedBox(height: 5),
                          Container(
                            padding: const EdgeInsets.symmetric(
                                horizontal: 10, vertical: 4),
                            decoration: BoxDecoration(
                              color:
                                  AppColors.primary(context).withAlpha(31),
                              borderRadius: BorderRadius.circular(9),
                            ),
                            child: Text(
                              l10n.discoverAvailable,
                              style: TextStyle(
                                fontSize: 11,
                                fontWeight: FontWeight.w600,
                                color: AppColors.primary(context),
                              ),
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _hero(BuildContext context, WidgetRef ref, String mediaId) {
    Widget placeholder() => Container(
          width: 64,
          height: 64,
          decoration: BoxDecoration(
            color: AppColors.surface(context),
            borderRadius: BorderRadius.circular(15),
          ),
        );
    if (mediaId.isEmpty) return placeholder();
    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return mediaAsync.when(
      data: (media) {
        if (media.url.isEmpty) return placeholder();
        return ClipRRect(
          borderRadius: BorderRadius.circular(15),
          child: CachedMediaImage(
            // Decorative: the card's Tappable announces the title.
            semanticsLabel: null,
            imageUrl: media.url,
            cacheKey: ImageCacheKeys.thumbnail(mediaId),
            width: 64,
            height: 64,
            fit: BoxFit.cover,
          ),
        );
      },
      loading: () => placeholder(),
      error: (_, _) => placeholder(),
    );
  }
}
