import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart' show ConversationItem;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/community_content_state.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('CommunityContentViewModel');

/// ViewModel for managing community content data (members, events, gear).
///
/// This ViewModel follows the MVVM architecture pattern and is responsible for:
/// - Loading members, events, and gear count from the repository
/// - Managing loading and error states for each data source
/// - Handling refresh operations with cache invalidation
/// - Providing parallel data loading for optimal performance
///
/// The ViewModel uses CommunityRepository which provides transparent caching
/// with environment-configured TTL (default 30 minutes).
class CommunityContentViewModel extends Notifier<CommunityContentState>
    with SafeNotifierMixin<CommunityContentState> {
  late final CommunityRepository _repository;

  @override
  CommunityContentState build() {
    _repository = ref.watch(communityRepositoryProvider);
    return const CommunityContentState();
  }

  /// Sets the active tab index (0 = Community info, 1 = Discuss).
  void setActiveTab(int index) {
    safeUpdateState((s) => s.copyWith(activeTab: index));
  }

  /// Clears the unread count on the cached community conversation in state.
  ///
  /// Call after the user has read the community chat so the Discuss tab badge
  /// and inbox card reflect the updated count without a full reload.
  void markConversationRead() {
    final conv = state.communityConversation;
    if (conv == null) return;
    final updated = ConversationItem()..mergeFromMessage(conv)..unreadCount = 0;
    safeUpdateState((s) => s.copyWith(communityConversation: updated));
  }

  /// Loads the community-wide conversation, if not already loaded.
  Future<void> loadConversation(String communityId) async {
    if (state.communityConversation != null || state.isLoadingConversation) return;
    safeUpdateState((s) => s.copyWith(isLoadingConversation: true, conversationError: null));
    try {
      final conversation = await ref.read(chatRepositoryProvider).getConversationForCommunity(communityId);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        communityConversation: conversation,
        isLoadingConversation: false,
      ));
    } catch (e, stackTrace) {
      _log.severe('Failed to load community conversation', e, stackTrace);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        conversationError: RpcErrorHandler.classify(e),
        isLoadingConversation: false,
      ));
    }
  }

  /// Initializes the ViewModel by loading all community content data in parallel.
  ///
  /// This method loads members, events, and gear count concurrently for optimal
  /// performance. Each data source has independent loading and error states,
  /// allowing the UI to show partial data while other sources are still loading.
  ///
  /// Example usage:
  /// ```dart
  /// @override
  /// void initState() {
  ///   super.initState();
  ///   WidgetsBinding.instance.addPostFrameCallback((_) {
  ///     ref.read(communityContentProvider.notifier).initialize(widget.communityId);
  ///   });
  /// }
  /// ```
  Future<void> initialize(String communityId) async {
    if (communityId.isEmpty) {
      _log.warning('initialize called with empty communityId');
      return;
    }

    _log.info('Initializing community content for: $communityId');

    // Load all data sources in parallel for optimal performance
    await Future.wait([
      loadMembers(communityId),
      loadEvents(communityId),
      loadGearCount(communityId),
    ]);

    if (!ref.mounted) return;

    _log.info('Community content initialized for: $communityId');
  }

  /// Loads the list of members in the community.
  ///
  /// Results are cached by the repository. Sets [isLoadingMembers]
  /// to true while loading and updates [membersError] if loading fails.
  Future<void> loadMembers(String communityId) async {
    _log.info('👥 loadMembers START - communityId: $communityId');
    safeUpdateState((s) => s.copyWith(isLoadingMembers: true, membersError: null));
    _log.info('👥 State updated - isLoadingMembers: true');

    try {
      _log.info('👥 Calling repository.getMembers...');
      final members = await _repository.getMembers(communityId);
      _log.info('👥 Repository returned ${members.length} members');

      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
        members: members,
        isLoadingMembers: false,
      ));
      _log.info('👥 loadMembers COMPLETE - ${members.length} members loaded, state updated');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to load members', e, stackTrace);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        membersError: RpcErrorHandler.classify(e),
        isLoadingMembers: false,
      ));
      _log.severe('❌ loadMembers FAILED - error state updated');
    }
  }

  /// Loads the list of events in the community.
  ///
  /// Results are cached by the repository. Sets [isLoadingEvents]
  /// to true while loading and updates [eventsError] if loading fails.
  Future<void> loadEvents(String communityId) async {
    safeUpdateState((s) => s.copyWith(isLoadingEvents: true, eventsError: null));
    try {
      final events = await _repository.getEvents(communityId);

      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
        events: events,
        isLoadingEvents: false,
      ));
      _log.fine('Loaded ${events.length} events for: $communityId');
    } catch (e, stackTrace) {
      _log.severe('Failed to load events', e, stackTrace);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        eventsError: RpcErrorHandler.classify(e),
        isLoadingEvents: false,
      ));
    }
  }

  /// Loads the count of gear items shared in the community.
  ///
  /// Results are cached by the repository. Sets [isLoadingGear]
  /// to true while loading and updates [gearError] if loading fails.
  Future<void> loadGearCount(String communityId) async {
    safeUpdateState((s) => s.copyWith(isLoadingGear: true, gearError: null));
    try {
      final count = await _repository.getGearCount(communityId);

      if (!ref.mounted) return;

      safeUpdateState((s) => s.copyWith(
        gearCount: count,
        isLoadingGear: false,
      ));
      _log.fine('Loaded gear count ($count) for: $communityId');
    } catch (e, stackTrace) {
      _log.severe('Failed to load gear count', e, stackTrace);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        gearError: RpcErrorHandler.classify(e),
        isLoadingGear: false,
      ));
    }
  }

  /// Refreshes all community content by invalidating caches and reloading.
  ///
  /// This method is typically called from a pull-to-refresh gesture. It:
  /// 1. Invalidates all caches in parallel (members, events, gear)
  /// 2. Reloads all data sources in parallel
  ///
  /// Example usage:
  /// ```dart
  /// RefreshIndicator(
  ///   onRefresh: () => ref
  ///       .read(communityContentProvider.notifier)
  ///       .refresh(communityId),
  ///   child: ListView(...),
  /// )
  /// ```
  Future<void> refresh(String communityId) async {
    _log.info('Refreshing community content for: $communityId');

    // Invalidate all caches in parallel
    await _repository.refreshAll(communityId);

    if (!ref.mounted) return;

    // Reload all data in parallel
    await initialize(communityId);
  }

  /// Refreshes only the members list.
  ///
  /// Invalidates the members cache and reloads the member list. Use this
  /// when you know only the members have changed (e.g., after a user joins).
  Future<void> refreshMembers(String communityId) async {
    await _repository.refreshMembers(communityId);

    if (!ref.mounted) return;

    await loadMembers(communityId);
  }

  /// Refreshes only the events list.
  ///
  /// Invalidates the events cache and reloads the events list. Use this
  /// when you know only the events have changed (e.g., after a new activity).
  Future<void> refreshEvents(String communityId) async {
    await _repository.refreshEvents(communityId);

    if (!ref.mounted) return;

    await loadEvents(communityId);
  }

  /// Refreshes only the gear count.
  ///
  /// Invalidates the gear cache and reloads the gear count. Use this when
  /// you know only the gear has changed (e.g., after sharing/unsharing gear).
  Future<void> refreshGearCount(String communityId) async {
    await _repository.refreshGearCount(communityId);

    if (!ref.mounted) return;

    await loadGearCount(communityId);
  }
}

/// Provider for CommunityContentViewModel.
///
/// Uses autoDispose since community content is screen-scoped and should be
/// cleaned up when the screen is disposed.
final communityContentProvider =
    NotifierProvider.autoDispose<CommunityContentViewModel, CommunityContentState>(
  CommunityContentViewModel.new,
);

/// Provider for community regions
final communityRegionsProvider =
    FutureProvider.family<List<CommunityRegionItem>, String>((ref, communityId) async {
  _log.info('📥 Loading community regions: $communityId');
  final communityRepository = ref.read(communityRepositoryProvider);

  try {
    final regions = await communityRepository.getCommunityRegions(communityId);
    _log.info('✅ Community regions loaded: ${regions.length} regions');
    return regions;
  } catch (e) {
    _log.warning('⚠️ Failed to load community regions: $e');
    return [];
  }
});

/// Provider for community members
final communityMembersProvider =
    FutureProvider.autoDispose.family<List<CommunityMember>, String>((ref, communityId) async {
  _log.info('👥 Loading community members: $communityId');
  final communityRepository = ref.read(communityRepositoryProvider);

  try {
    final members = await communityRepository.getMembers(communityId);
    _log.info('✅ Community members loaded: ${members.length} members');
    return members;
  } catch (e) {
    _log.severe('❌ Failed to load community members: $e');
    rethrow;
  }
});

