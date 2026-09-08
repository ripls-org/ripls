import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RejoinableCommunitiesViewModel');

/// AsyncNotifier exposing the active communities the caller can rejoin
/// without a fresh invite — those where the caller has a soft-deleted
/// CommunityUser row less than 30 days old AND the community itself is
/// active (§2.7). Powers the Settings → Communities recently-left
/// section (#1721).
///
/// Loads from `CommunityRepository.listRejoinableCommunities()`, which
/// caches under `community:rejoinable:list` and is invalidated by
/// `rejoinCommunity` (and externally by leave/delete/restore flows that
/// fire `rejoinableCommunitiesCacheInvalidationProvider`). The server
/// returns items ordered descending by `leftAtUnixSec` —
/// most-recently-left first.
class RejoinableCommunitiesNotifier
    extends AsyncNotifier<List<RejoinableCommunityItem>> {
  @override
  Future<List<RejoinableCommunityItem>> build() async {
    // Re-run build whenever the rejoinable-list cache is invalidated.
    // Without this watch, the section appears stale until the
    // SettingsHub fully unmounts. Same fix that closed the equivalent
    // bug in DeletedCommunitiesNotifier (#1717).
    ref.watch(rejoinableCommunitiesCacheInvalidationProvider);
    return _load();
  }

  Future<List<RejoinableCommunityItem>> _load() async {
    final repo = ref.read(communityRepositoryProvider);
    final items = await repo.listRejoinableCommunities();
    _log.fine('Loaded ${items.length} rejoinable communities');
    return items;
  }

  /// Forces a fresh fetch from the server, bypassing the cache.
  Future<void> refresh() async {
    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final repo = ref.read(communityRepositoryProvider);
      return repo.listRejoinableCommunities(refresh: true);
    });

    if (!ref.mounted) return;
    state = result;
  }
}

/// Provider for the rejoinable-communities list.
///
/// Auto-disposes when no widget is watching, so navigating away from
/// Settings releases the list.
final rejoinableCommunitiesProvider = AsyncNotifierProvider.autoDispose<
    RejoinableCommunitiesNotifier, List<RejoinableCommunityItem>>(
  RejoinableCommunitiesNotifier.new,
);
