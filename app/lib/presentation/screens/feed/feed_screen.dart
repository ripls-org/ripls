import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/core/utils/experience_helper.dart';
import 'package:ripls/core/utils/gear_helper.dart';
import 'package:ripls/core/utils/request_helpers.dart';
import 'package:ripls/presentation/screens/communities/community_content_view.dart';
import 'package:ripls/presentation/screens/experience/experience_content_view.dart';
import 'package:ripls/presentation/screens/gear/gear_content_view.dart';
import 'package:ripls/presentation/screens/request/request_content_view.dart';
import 'package:ripls/presentation/screens/story/story_content_view.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/empty_content_state.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/presentation/widgets/profile_menu_avatar.dart';
import 'package:ripls/presentation/widgets/search/close_search_button.dart';
import 'package:ripls/presentation/widgets/search/search_nearby_pill.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

/// FeedScreen displays a vertical feed of personalized community content.
/// Users can swipe up/down to navigate between feed items.
/// Shows gear sharing events; additional item types can be added here as the feed expands.
class FeedScreen extends ConsumerStatefulWidget {
  final PageController? scrollController;

  const FeedScreen({super.key, this.scrollController});

  @override
  ConsumerState<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends ConsumerState<FeedScreen> {
  late final PageController _pageController;

  @override
  void initState() {
    super.initState();
    _pageController = widget.scrollController ?? PageController();
    _pageController.addListener(_onPageChanged);

    // Initialize feed after first frame
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      final communityIds = ref.read(communitiesProvider).communityIds;
      if (communityIds.isEmpty) return;
      await ref.read(feedProvider.notifier).initialize(communityIds);
      if (!mounted) return;
      // The page controller settles on page 0 without firing _onPageChanged,
      // so the first card would never get a view record. Only the swipe feed
      // needs this — a card is "viewed" here because it filled the screen.
      unawaited(ref.read(feedProvider.notifier).markCurrentItemViewed());
    });
  }

  void _onPageChanged() {
    if (_pageController.hasClients) {
      final page = _pageController.page?.round() ?? 0;
      ref.read(feedProvider.notifier).setPageIndex(page);
    }
  }

  @override
  void dispose() {
    _pageController.removeListener(_onPageChanged);
    // Only dispose the page controller if we created it ourselves
    if (widget.scrollController == null) {
      _pageController.dispose();
    }
    super.dispose();
  }

  Future<void> _onRefresh() async {
    // User explicitly asked to refresh — drop both cached device positions
    // (proximity + precise) so subsequent reads get fresh fixes. Cheap:
    // just forgets the caches; the refetches are lazy.
    ref.invalidate(userLocationProvider(LocationIntent.proximityBias));
    ref.invalidate(userLocationProvider(LocationIntent.precisePin));

    await ref.read(feedProvider.notifier).refresh();

    // After refresh, scroll back to the first entry
    if (!mounted) return;
    if (_pageController.hasClients) {
      unawaited(_pageController.animateToPage(
        0,
        duration: accessibleDuration(
          context,
          const Duration(milliseconds: 300),
        ),
        curve: Curves.easeInOut,
      ));
    }
  }

  @override
  Widget build(BuildContext context) {
    final communityState = ref.watch(communitiesProvider);
    final feedState = ref.watch(feedProvider);

    // Check if we need to initialize for a different set of communities
    final enabledIds = communityState.communityIds;
    final currentIds = feedState.currentCommunityIds;
    final setsMatch =
        enabledIds.length == currentIds.length &&
        enabledIds.toSet().containsAll(currentIds);
    if (!setsMatch && enabledIds.isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          ref.read(feedProvider.notifier).initialize(enabledIds);
        }
      });
    }

    final isNavVisible = ref.watch(homeProvider).isNavVisible;
    final isSearchActive = ref.watch(searchPillActiveProvider);
    final safeAreaTop = MediaQuery.of(context).padding.top;

    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: Stack(
        children: [
          communityState.communityIds.isEmpty
              ? _buildNoCommunityState(context)
              : _buildBody(context, feedState),
          // Floating header
          Positioned(
            top: safeAreaTop,
            left: 0,
            right: 0,
            child: AnimatedSlide(
              duration: accessibleDuration(
                context,
                const Duration(milliseconds: 200),
              ),
              offset: isNavVisible ? Offset.zero : const Offset(0, -3),
              child: FloatingHeader(
                content: const SearchNearbyPill(),
                // Swap the trailing slot to a Close X while the
                // search pill is active so the X sits where the
                // DiscoverScreen close button does — right-justified
                // in the floating header, outside the pill itself.
                trailing: isSearchActive
                    ? CloseSearchButton(
                        onTap: () => ref
                            .read(searchPillActiveProvider.notifier)
                            .set(false),
                        semanticsLabel: context.l10n.a11ySearchDismiss,
                      )
                    : const ProfileMenuAvatar(),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildNoCommunityState(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Text(
          'Select a community from the sidebar to see your feed',
          style: TextStyle(
            fontSize: 15,
            color: AppColors.textSecondary(context),
          ),
          textAlign: TextAlign.center,
        ),
      ),
    );
  }

  Widget _buildBody(BuildContext context, FeedState feedState) {
    if (feedState.isLoading) {
      return Center(
        child: CircularProgressIndicator(color: AppColors.primary(context)),
      );
    }

    if (feedState.hasError) {
      return RefreshIndicator(
        onRefresh: _onRefresh,
        child: SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          child: SizedBox(
            height: MediaQuery.of(context).size.height - 200,
            child: Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    Icons.error_outline,
                    color: AppColors.textPrimary(context),
                    size: 48,
                  ),
                  const SizedBox(height: 16),
                  Text(
                    'Failed to load feed',
                    style: TextStyle(
                      color: AppColors.textPrimary(context),
                      fontSize: 18,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    feedState.error == null
                        ? 'Unknown error'
                        : RpcErrorHandler.localize(
                            feedState.error!,
                            context.l10n,
                          ),
                    style: TextStyle(
                      color: AppColors.textSecondary(context),
                      fontSize: 14,
                    ),
                    textAlign: TextAlign.center,
                  ),
                ],
              ),
            ),
          ),
        ),
      );
    }

    if (feedState.isEmpty) {
      return RefreshIndicator(
        onRefresh: _onRefresh,
        child: SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          child: SizedBox(
            height: MediaQuery.of(context).size.height - 200,
            child: EmptyContentState(
              icon: Icons.inbox_outlined,
              title: context.l10n.feedAllCaughtUp,
              message: context.l10n.feedEmpty,
            ),
          ),
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: _onRefresh,
      child: Tappable(
        semanticsLabel: context.l10n.a11yMiscToggleNavigation,
        onTap: () {
          // Toggle nav when user taps on the screen
          ref.read(homeProvider.notifier).toggleNav();
        },
        excludeChildSemantics: false,
        child: NotificationListener<ScrollNotification>(
          onNotification: (notification) {
            if (notification is ScrollUpdateNotification &&
                notification.depth == 0) {
              // depth == 0: only react to the PageView's own scroll, not
              // nested scrollables (chat message list, media carousel).
              final scrollPosition = notification.metrics.pixels;
              if (scrollPosition < 0) return false;

              final delta = notification.scrollDelta;
              if (delta == null) return false;

              if (delta > 10) {
                ref.read(homeProvider.notifier).hideNav();
              } else if (delta < -10) {
                ref.read(homeProvider.notifier).showNav();
              }
            }
            return false;
          },
          child: PageView.builder(
            controller: _pageController,
            scrollDirection: Axis.vertical,
            physics: const AlwaysScrollableScrollPhysics(),
            itemCount: _visibleItems(feedState).length,
            itemBuilder: (context, index) {
              final item = _visibleItems(feedState)[index];
              // Use KeyedSubtree with item ID to force widget recreation when items change
              // This ensures initState() is called for each new item
              return KeyedSubtree(
                key: ValueKey(item.id),
                child: _buildFeedItem(item),
              );
            },
          ),
        ),
      ),
    );
  }

  List<FeedItem> _visibleItems(FeedState feedState) {
    return feedState.items.where((item) {
      // Hide consumed nudges.
      if (item.itemType == FeedItemType.FEED_ITEM_TYPE_NUDGE &&
          feedState.consumedNudgeIds.contains(item.nudge.nudgeId)) {
        return false;
      }
      return true;
    }).toList();
  }

  Widget _buildFeedItem(FeedItem item) {
    switch (item.itemType) {
      case FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED:
        // Display gear sharing events with feed header
        return GearContentView(
          gearId: item.gearShared.gearId,
          showEditControls: true,
          showOwnerInfo: false,
          onDeleted: _onRefresh,
          // Feed header parameters
          showFeedHeader: true,
          feedActor: item.gearShared.actor,
          feedOccurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          feedLastActivityAtUnixSec: item.lastActivityAtUnixSec.toInt(),
          feedActionText: GearHelper.getFeedActionText(
            item.gearShared.availability,
          ),
        );

      case FeedItemType.FEED_ITEM_TYPE_REQUEST_CREATED:
        // Display request posting events with feed header
        return RequestContentView(
          requestId: item.requestCreated.requestId,
          showEditControls: true,
          showFloatingActions: true,
          showOwnerInfo: false,
          onDeleted: _onRefresh,
          // Feed header parameters
          showFeedHeader: true,
          feedActor: item.requestCreated.requester,
          feedOccurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          feedLastActivityAtUnixSec: item.lastActivityAtUnixSec.toInt(),
          feedActionText: getRequestFeedActionText(),
        );

      case FeedItemType.FEED_ITEM_TYPE_COMMUNITY_CREATED:
        // Display community creation events with feed header
        final mediaId = item.communityCreated.mediaIds.firstOrNull ?? '';
        return CommunityContentView(
          communityId: item.communityCreated.communityId,
          communityName: item.communityCreated.communityName,
          communityDescription: item.communityCreated.communityDescription,
          actor: item.communityCreated.actor,
          mediaId: mediaId,
          canEdit: item.communityCreated.canEdit,
          occurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          // Feed header parameters
          showFeedHeader: true,
          feedActor: item.communityCreated.actor,
          feedOccurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          feedActionText: CommunityHelper.getFeedActionText(
            context.l10n,
            communityName: item.communityCreated.communityName,
          ),
        );

      case FeedItemType.FEED_ITEM_TYPE_EXPERIENCE_CREATED:
        // Display experience sharing events with feed header
        return ExperienceContentView(
          experienceId: item.experienceCreated.experienceId,
          showEditControls: true,
          showFloatingActions: true,
          showOwnerInfo: false,
          onDeleted: _onRefresh,
          // Feed header parameters
          showFeedHeader: true,
          feedActor: item.experienceCreated.creator,
          feedOccurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          feedLastActivityAtUnixSec: item.lastActivityAtUnixSec.toInt(),
          feedActionText: ExperienceHelper.getFeedActionText(),
        );

      case FeedItemType.FEED_ITEM_TYPE_STORY:
        // Display story events with feed header
        return StoryContentView(
          story: item.story,
          // Feed header parameters
          showFeedHeader: true,
          feedActor: null, // Stories have no actor (system-generated)
          feedOccurredAtUnixSec: item.occurredAtUnixSec.toInt(),
          feedActionText: 'Shared a story',
        );

      case FeedItemType.FEED_ITEM_TYPE_NUDGE:
        return NudgeContentView(nudge: item.nudge);

      // Additional item types can be added here:
      // case FeedItemType.FEED_ITEM_TYPE_UNREAD_NOTIFICATIONS:
      // case FeedItemType.FEED_ITEM_TYPE_INVITER_CELEBRATION:
      // etc.

      default:
        return _buildPlaceholderView(item);
    }
  }

  Widget _buildPlaceholderView(FeedItem item) {
    return Container(
      color: AppColors.background(context),
      child: Center(
        child: Text(
          'Feed item type: ${item.itemType.name}',
          style: TextStyle(
            color: AppColors.textSecondary(context),
            fontSize: 16,
          ),
        ),
      ),
    );
  }
}
