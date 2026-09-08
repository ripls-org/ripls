import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/profile_service.pb.dart'
    show SharedCommunityRef;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/discover/discover_screen.dart';
import 'package:ripls/presentation/viewmodels/recent_searches_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_suggestions_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/services/providers.dart' show mediaObjectProvider;
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

/// Idle-state search affordance the Feed's floating header slots into
/// its `content:` slot (#1896). Renders as a rounded pill with a search
/// icon and "Search nearby" placeholder. Tapping promotes the pill to
/// an active inline TextField with a trailing Cancel and shows a
/// dim-backed suggestions card directly beneath the input — no bottom
/// modal. Submitting (typed query or chip tap) persists to recents,
/// fires [SearchNotifier.searchWithLocation], and pushes the
/// [DiscoverScreen] results surface.
class SearchNearbyPill extends ConsumerStatefulWidget {
  const SearchNearbyPill({super.key});

  @override
  ConsumerState<SearchNearbyPill> createState() => _SearchNearbyPillState();
}

class _SearchNearbyPillState extends ConsumerState<SearchNearbyPill> {
  final OverlayPortalController _portal = OverlayPortalController();
  final TextEditingController _textController = TextEditingController();
  final FocusNode _focusNode = FocusNode();
  bool _isActive = false;

  @override
  void dispose() {
    _textController.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  void _activate() {
    if (_isActive) return;
    _portal.show();
    setState(() => _isActive = true);
    ref.read(searchPillActiveProvider.notifier).set(true);
    // Defer focus to after the rebuild so the TextField exists in the
    // overlay layer before we try to attach the keyboard.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      _focusNode.requestFocus();
    });
  }

  void _deactivate() {
    _focusNode.unfocus();
    _portal.hide();
    _textController.clear();
    if (mounted) setState(() => _isActive = false);
    ref.read(searchPillActiveProvider.notifier).set(false);
  }

  Future<void> _submit(String query) async {
    final trimmed = query.trim();
    if (trimmed.isEmpty) return;

    await ref.read(recentSearchesProvider.notifier).add(trimmed);
    if (!mounted) return;

    final communityIds =
        ref.read(communitiesProvider).communityIds;
    ref.read(searchProvider.notifier).searchWithLocation(
          query: trimmed,
          communityIds: communityIds,
        );

    _deactivate();
    if (!mounted) return;

    unawaited(NavigationHelpers.pushScreen(
      context: context,
      screen: const DiscoverScreen(),
      useRootNavigator: true,
      routeName: 'discover_search',
    ));
  }

  void _browsePeople() {
    final communityIds =
        ref.read(communitiesProvider).communityIds;
    ref.read(searchProvider.notifier).browsePeople(
          communityIds: communityIds,
        );

    _deactivate();
    if (!mounted) return;

    NavigationHelpers.pushScreen(
      context: context,
      screen: const DiscoverScreen(),
      useRootNavigator: true,
      routeName: 'discover_people',
    );
  }

  void _browseRequests() {
    final communityIds =
        ref.read(communitiesProvider).communityIds;
    ref.read(searchProvider.notifier).browseRequests(
          communityIds: communityIds,
        );

    _deactivate();
    if (!mounted) return;

    NavigationHelpers.pushScreen(
      context: context,
      screen: const DiscoverScreen(),
      useRootNavigator: true,
      routeName: 'discover_requests',
    );
  }

  void _browseCommunity(String communityId) {
    ref.read(searchProvider.notifier).browseCommunity(
          communityId: communityId,
        );

    _deactivate();
    if (!mounted) return;

    NavigationHelpers.pushScreen(
      context: context,
      screen: const DiscoverScreen(),
      useRootNavigator: true,
      routeName: 'discover_community',
    );
  }

  @override
  Widget build(BuildContext context) {
    // React to *external* dismiss requests — the FloatingHeader's
    // trailing X (rendered by FeedScreen) sets searchPillActiveProvider
    // to false directly. When that happens and the overlay is still
    // showing, tear it down. The second `set(false)` inside
    // [_deactivate] is a noop since the state is already false.
    ref.listen<bool>(searchPillActiveProvider, (prev, next) {
      if ((prev ?? false) && next == false && _isActive) {
        _deactivate();
      }
    });

    return OverlayPortal(
      controller: _portal,
      overlayChildBuilder: (overlayContext) => _Overlay(
        anchorContext: context,
        textController: _textController,
        focusNode: _focusNode,
        onDismiss: _deactivate,
        onPick: _submit,
        onBrowsePeople: _browsePeople,
        onBrowseRequests: _browseRequests,
        onBrowseCommunity: _browseCommunity,
      ),
      child: _isActive
          // While active, the inline TextField lives inside the overlay
          // layer so it can paint above the backdrop. Reserve the
          // original pill footprint here so the floating header's
          // layout doesn't reflow.
          ? const SizedBox(height: 36)
          : _IdlePill(onTap: _activate),
    );
  }
}

/// The static "Search nearby" pill that's shown until the user taps it.
class _IdlePill extends StatelessWidget {
  const _IdlePill({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yOpenSearchSuggestions,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(20),
      child: _PillFrame(
        child: Row(
          children: [
            Icon(
              Icons.search,
              size: 18,
              color: AppColors.textSecondary(context),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                context.l10n.feedSearchNearbyPlaceholder,
                style: TextStyle(
                  fontSize: 13,
                  color: AppColors.textSecondary(context),
                ),
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Shared rounded-pill frame used by both the idle pill and the active
/// input so the visual transition between states is seamless. The frame
/// itself is transparent so the surrounding FloatingHeader's alpha-blended
/// cream is the single visible surface — leading icon, input, and trailing
/// cancel all read as one continuous panel.
class _PillFrame extends StatelessWidget {
  const _PillFrame({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 36,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12),
        child: child,
      ),
    );
  }
}

/// Overlay layer rendered while the pill is active. Anchors the active
/// input to the original pill position, dims the rest of the screen
/// (tap to dismiss), and stacks a suggestions card directly beneath the
/// input.
class _Overlay extends ConsumerWidget {
  const _Overlay({
    required this.anchorContext,
    required this.textController,
    required this.focusNode,
    required this.onDismiss,
    required this.onPick,
    required this.onBrowsePeople,
    required this.onBrowseRequests,
    required this.onBrowseCommunity,
  });

  final BuildContext anchorContext;
  final TextEditingController textController;
  final FocusNode focusNode;
  final VoidCallback onDismiss;
  final ValueChanged<String> onPick;
  final VoidCallback onBrowsePeople;
  final VoidCallback onBrowseRequests;
  final ValueChanged<String> onBrowseCommunity;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final renderBox = anchorContext.findRenderObject() as RenderBox?;
    if (renderBox == null || !renderBox.attached) {
      return const SizedBox.shrink();
    }
    final anchorTopLeft = renderBox.localToGlobal(Offset.zero);
    final anchorSize = renderBox.size;
    final inputTop = anchorTopLeft.dy;
    final inputLeft = anchorTopLeft.dx;
    final inputWidth = anchorSize.width;
    final inputHeight = anchorSize.height;
    // The pill is centered inside FloatingHeader (48 high, pill 36) — so
    // the floating header extends 6px below the pill on every side. The
    // dim backdrop must start at the floating header's bottom, not the
    // pill's, otherwise it clips the header's rounded bottom edge.
    final floatingHeaderBottom = inputTop +
        inputHeight +
        ((FloatingHeader.headerHeight - inputHeight) / 2);
    final panelTop = floatingHeaderBottom + 8;

    return Stack(
      children: [
        // Dim backdrop covers everything BELOW the floating header.
        // Tapping it dismisses the search and restores the idle pill.
        Positioned(
          top: floatingHeaderBottom,
          left: 0,
          right: 0,
          bottom: 0,
          child: Tappable(
            semanticsLabel: context.l10n.a11ySearchDismiss,
            onTap: onDismiss,
            inkBorderRadius: BorderRadius.zero,
            child: ColoredBox(color: Colors.black.withValues(alpha: 0.45)),
          ),
        ),
        // Active input + Cancel button, painted in the same screen
        // location the idle pill occupied.
        Positioned(
          top: inputTop,
          left: inputLeft,
          width: inputWidth,
          height: inputHeight,
          child: _ActiveInputRow(
            controller: textController,
            focusNode: focusNode,
            onSubmitted: onPick,
          ),
        ),
        // Suggestions card, positioned to mirror the floating header's
        // horizontal gutters so the card and the input visually align.
        Positioned(
          top: panelTop,
          left: 16,
          right: 16,
          child: _SuggestionsPanel(
            onPick: onPick,
            onBrowsePeople: onBrowsePeople,
            onBrowseRequests: onBrowseRequests,
            onBrowseCommunity: onBrowseCommunity,
          ),
        ),
      ],
    );
  }
}

/// The Row shown in place of the idle pill while the search is
/// active. Hosts the live TextField; the dismiss X is rendered by the
/// surrounding [FloatingHeader]'s trailing slot (see
/// [SearchNearbyPill] — the trailing slot swap is driven by
/// [searchPillActiveProvider]), matching how [DiscoverScreen]
/// positions its trailing close affordance.
class _ActiveInputRow extends StatelessWidget {
  const _ActiveInputRow({
    required this.controller,
    required this.focusNode,
    required this.onSubmitted,
  });

  final TextEditingController controller;
  final FocusNode focusNode;
  final ValueChanged<String> onSubmitted;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return _PillFrame(
      child: Row(
        children: [
          Icon(
            Icons.search,
            size: 18,
            color: AppColors.textSecondary(context),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: controller,
              focusNode: focusNode,
              onSubmitted: onSubmitted,
              textInputAction: TextInputAction.search,
              style: TextStyle(
                fontSize: 13,
                color: AppColors.textPrimary(context),
              ),
              decoration: InputDecoration(
                // isCollapsed + zero contentPadding pins the field to
                // the row's vertical center the same way the idle
                // pill's Text widget is — eliminates the few-pixel
                // baseline jump caused by the TextField's default
                // 16px vertical content padding on activation.
                isCollapsed: true,
                contentPadding: EdgeInsets.zero,
                filled: false,
                fillColor: Colors.transparent,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                hintText: l10n.feedSearchNearbyPlaceholder,
                hintStyle: TextStyle(
                  fontSize: 13,
                  color: AppColors.textSecondary(context),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Card-style panel beneath the active input. Top-down layout:
/// Quick Search (static fallback chips) → Communities (horizontal
/// avatar carousel) → Interests (chip wrap, no icons) → Recents
/// (two-column grid with per-item delete). Every chip seeds
/// [onPick] except the People chip and community avatars, which
/// dispatch to dedicated browse paths.
class _SuggestionsPanel extends ConsumerWidget {
  const _SuggestionsPanel({
    required this.onPick,
    required this.onBrowsePeople,
    required this.onBrowseRequests,
    required this.onBrowseCommunity,
  });

  final ValueChanged<String> onPick;
  final VoidCallback onBrowsePeople;
  final VoidCallback onBrowseRequests;
  final ValueChanged<String> onBrowseCommunity;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final suggestionsAsync = ref.watch(searchSuggestionsProvider);
    final suggestions = suggestionsAsync.value;
    final hasCommunities = suggestions?.hasCommunities ?? false;
    final hasInterests = suggestions?.hasKnownForCategories ?? false;

    return Material(
      color: AppColors.cardBackground(context),
      elevation: 8,
      borderRadius: BorderRadius.circular(20),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _SectionLabel(text: l10n.searchSuggestionsQuickSearchHeader),
            const SizedBox(height: 8),
            _QuickSearchGrid(
              onPick: onPick,
              onBrowsePeople: onBrowsePeople,
              onBrowseRequests: onBrowseRequests,
            ),
            if (hasCommunities) ...[
              const SizedBox(height: 16),
              _SectionLabel(text: l10n.searchSuggestionsTopCommunitiesHeader),
              const SizedBox(height: 8),
              _CommunityCarousel(
                communities: suggestions!.topCommunities,
                onTap: (c) => onBrowseCommunity(c.id),
              ),
            ],
            if (hasInterests) ...[
              const SizedBox(height: 16),
              _SectionLabel(text: l10n.searchSuggestionsInterestsHeader),
              const SizedBox(height: 8),
              _InterestChipWrap(
                interests: suggestions!.topKnownForCategories,
                onTap: onPick,
              ),
            ],
            const SizedBox(height: 16),
            _RecentSearchesSection(onPick: onPick),
          ],
        ),
      ),
    );
  }
}

/// Horizontal scrollable rail of community avatars + names. Each tile
/// is a circular avatar with the community name below it.
class _CommunityCarousel extends ConsumerWidget {
  const _CommunityCarousel({required this.communities, required this.onTap});

  final List<SharedCommunityRef> communities;
  final ValueChanged<SharedCommunityRef> onTap;

  static const double _tileWidth = 72;
  static const double _avatarSize = 56;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    return SizedBox(
      height: 84,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: communities.length,
        separatorBuilder: (_, _) => const SizedBox(width: 12),
        itemBuilder: (context, index) {
          final c = communities[index];
          return SizedBox(
            width: _tileWidth,
            child: Tappable(
              semanticsLabel: l10n.a11ySearchSuggestionCommunity(c.name),
              onTap: () => onTap(c),
              inkBorderRadius: BorderRadius.circular(12),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  _CommunityAvatar(
                    mediaId: c.hasMediaId() ? c.mediaId : '',
                    name: c.name,
                    size: _avatarSize,
                  ),
                  const SizedBox(height: 6),
                  Text(
                    c.name,
                    style: TextStyle(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w500,
                      color: AppColors.textPrimary(context),
                      height: 1.1,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    textAlign: TextAlign.center,
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

/// Circular avatar for a community. Falls back to a colored circle
/// with the community's first letter when no avatar media is set or
/// the URL hasn't resolved yet.
class _CommunityAvatar extends ConsumerWidget {
  const _CommunityAvatar({
    required this.mediaId,
    required this.name,
    required this.size,
  });

  final String mediaId;
  final String name;
  final double size;

  String get _initial {
    final trimmed = name.trim();
    if (trimmed.isEmpty) return '?';
    return trimmed.characters.first.toUpperCase();
  }

  Widget _placeholder(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        shape: BoxShape.circle,
      ),
      alignment: Alignment.center,
      child: Text(
        _initial,
        style: TextStyle(
          fontSize: size * 0.4,
          fontWeight: FontWeight.w600,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (mediaId.isEmpty) return _placeholder(context);
    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return ClipOval(
      child: mediaAsync.when(
        data: (mediaUrl) {
          if (mediaUrl.url.isEmpty) return _placeholder(context);
          return CachedMediaImage(
            // Decorative; the surrounding Tappable carries the semantic label.
            semanticsLabel: null,
            imageUrl: mediaUrl.url,
            cacheKey: ImageCacheKeys.thumbnail(mediaId),
            width: size,
            height: size,
            fit: BoxFit.cover,
          );
        },
        loading: () => _placeholder(context),
        error: (_, _) => _placeholder(context),
      ),
    );
  }
}

/// Chip wrap used for the Interests section. Plain text on a surface
/// pill — no icons (per design: keep interests visually distinct from
/// the icon-rich Quick Search row). Each chip shrink-wraps to its
/// text so multiple chips lay out inline per row before wrapping.
class _InterestChipWrap extends StatelessWidget {
  const _InterestChipWrap({required this.interests, required this.onTap});

  final List<String> interests;
  final ValueChanged<String> onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final interest in interests)
          Tappable(
            semanticsLabel: l10n.a11ySearchSuggestionInterest(interest),
            onTap: () => onTap(interest),
            inkBorderRadius: BorderRadius.circular(20),
            child: Container(
              padding: const EdgeInsets.symmetric(
                horizontal: 14,
                vertical: 8,
              ),
              decoration: BoxDecoration(
                color: AppColors.surface(context),
                borderRadius: BorderRadius.circular(20),
              ),
              child: Text(
                interest,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textPrimary(context),
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ),
      ],
    );
  }
}

class _SectionLabel extends StatelessWidget {
  const _SectionLabel({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w600,
        letterSpacing: 1.6,
        color: AppColors.textSecondary(context),
      ),
    );
  }
}

class _QuickSearchGrid extends StatelessWidget {
  const _QuickSearchGrid({
    required this.onPick,
    required this.onBrowsePeople,
    required this.onBrowseRequests,
  });

  final ValueChanged<String> onPick;
  final VoidCallback onBrowsePeople;
  final VoidCallback onBrowseRequests;

  static const double _rowGap = 8;
  static const double _columnGap = 8;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final chips = _quickChips(l10n);
    return LayoutBuilder(
      builder: (context, constraints) {
        final cellWidth = (constraints.maxWidth - _columnGap) / 2;
        return Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (int i = 0; i < chips.length; i += 2) ...[
              if (i > 0) const SizedBox(height: _rowGap),
              Row(
                children: [
                  SizedBox(
                    width: cellWidth,
                    child: _QuickChipButton(
                      label: chips[i].label,
                      icon: chips[i].icon,
                      onTap: chips[i].onTap,
                    ),
                  ),
                  if (i + 1 < chips.length) ...[
                    const SizedBox(width: _columnGap),
                    SizedBox(
                      width: cellWidth,
                      child: _QuickChipButton(
                        label: chips[i + 1].label,
                        icon: chips[i + 1].icon,
                        onTap: chips[i + 1].onTap,
                      ),
                    ),
                  ],
                ],
              ),
            ],
          ],
        );
      },
    );
  }

  List<_QuickChip> _quickChips(AppLocalizations l10n) {
    return [
      // "Help Needed" is a *browse* action, not a keyword search —
      // dispatch to SearchNotifier.browseRequests so the chip
      // surfaces actual help requests instead of running a literal
      // keyword search for "help nearby".
      _QuickChip(
        label: l10n.searchSuggestionsChipHelpNearby,
        icon: Icons.location_on_outlined,
        onTap: onBrowseRequests,
      ),
      _QuickChip(
        label: l10n.searchSuggestionsChipToolsToBorrow,
        icon: Icons.build_outlined,
        onTap: () => onPick(l10n.searchSuggestionsChipToolsToBorrowQuery),
      ),
      _QuickChip(
        label: l10n.searchSuggestionsChipEventsThisWeek,
        icon: Icons.event_outlined,
        onTap: () => onPick(l10n.searchSuggestionsChipEventsThisWeekQuery),
      ),
      // "People in Circles" is a *browse* action, not a keyword search —
      // dispatch to SearchNotifier.browsePeople instead of seeding the
      // input with the placeholder query string.
      _QuickChip(
        label: l10n.searchSuggestionsChipPeopleInCircles,
        icon: Icons.people_alt_outlined,
        onTap: onBrowsePeople,
      ),
    ];
  }
}

class _QuickChip {
  _QuickChip({required this.label, required this.icon, required this.onTap});

  final String label;
  final IconData icon;
  final VoidCallback onTap;
}

class _QuickChipButton extends StatelessWidget {
  const _QuickChipButton({
    required this.label,
    required this.icon,
    required this.onTap,
  });

  final String label;
  final IconData icon;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(20),
      child: Container(
        height: 40,
        padding: const EdgeInsets.symmetric(horizontal: 14),
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(20),
        ),
        child: Row(
          children: [
            Icon(icon, size: 16, color: AppColors.textSecondary(context)),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textPrimary(context),
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _RecentSearchesSection extends ConsumerWidget {
  const _RecentSearchesSection({required this.onPick});

  final ValueChanged<String> onPick;

  static const double _rowGap = 8;
  static const double _columnGap = 8;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final asyncRecents = ref.watch(recentSearchesProvider);
    final recents = asyncRecents.value ?? const <String>[];
    if (recents.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            _SectionLabel(text: l10n.searchSuggestionsRecentHeader),
            Tappable(
              semanticsLabel: l10n.a11yClearRecentSearches,
              onTap: () =>
                  ref.read(recentSearchesProvider.notifier).clear(),
              inkBorderRadius: BorderRadius.circular(8),
              child: Padding(
                padding: const EdgeInsets.symmetric(
                  horizontal: 8,
                  vertical: 4,
                ),
                child: Text(
                  l10n.searchSuggestionsRecentClear,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),
        LayoutBuilder(
          builder: (context, constraints) {
            final cellWidth = (constraints.maxWidth - _columnGap) / 2;
            return Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (int i = 0; i < recents.length; i += 2) ...[
                  if (i > 0) const SizedBox(height: _rowGap),
                  Row(
                    children: [
                      SizedBox(
                        width: cellWidth,
                        child: _RecentSearchPill(
                          query: recents[i],
                          onTap: () => onPick(recents[i]),
                          onRemove: () => ref
                              .read(recentSearchesProvider.notifier)
                              .remove(recents[i]),
                        ),
                      ),
                      if (i + 1 < recents.length) ...[
                        const SizedBox(width: _columnGap),
                        SizedBox(
                          width: cellWidth,
                          child: _RecentSearchPill(
                            query: recents[i + 1],
                            onTap: () => onPick(recents[i + 1]),
                            onRemove: () => ref
                                .read(recentSearchesProvider.notifier)
                                .remove(recents[i + 1]),
                          ),
                        ),
                      ],
                    ],
                  ),
                ],
              ],
            );
          },
        ),
      ],
    );
  }
}

/// One recent-search pill: text label on the left, X delete on the
/// right. Tapping the text re-runs the search; tapping the X removes
/// just that entry without disturbing the rest of the list.
class _RecentSearchPill extends StatelessWidget {
  const _RecentSearchPill({
    required this.query,
    required this.onTap,
    required this.onRemove,
  });

  final String query;
  final VoidCallback onTap;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Container(
      height: 40,
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: query,
              onTap: onTap,
              inkBorderRadius: BorderRadius.circular(20),
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 14),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    query,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: AppColors.textPrimary(context),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ),
            ),
          ),
          Tappable(
            semanticsLabel: l10n.a11yRemoveRecentSearch(query),
            onTap: onRemove,
            inkBorderRadius: BorderRadius.circular(14),
            child: SizedBox(
              width: 32,
              height: 32,
              child: Icon(
                Icons.close,
                size: 14,
                color: AppColors.textSecondary(context),
              ),
            ),
          ),
          const SizedBox(width: 4),
        ],
      ),
    );
  }
}
