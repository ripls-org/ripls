import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('DeletedCommunitiesViewModel');

/// AsyncNotifier exposing the soft-deleted communities the caller is
/// eligible to restore. Powers the Settings → Communities recently-deleted
/// section (#1717).
///
/// Loads from `CommunityRepository.listDeletedCommunitiesForRestore()`,
/// which already caches under `community:deleted:list` and is invalidated
/// by `deleteCommunity` and `restoreCommunity`. The server returns items
/// ordered ascending by `deletedAtUnixSec` — oldest deletion first.
class DeletedCommunitiesNotifier
    extends AsyncNotifier<List<DeletedCommunityItem>> {
  @override
  Future<List<DeletedCommunityItem>> build() async {
    // Re-run build whenever the deleted-list cache is invalidated
    // (delete or restore). Without this watch, the notifier holds
    // its initial AsyncData while the user is mid-flow and the
    // section appears stale until the SettingsHub fully unmounts.
    ref.watch(deletedCommunitiesCacheInvalidationProvider);
    return _load();
  }

  Future<List<DeletedCommunityItem>> _load() async {
    final repo = ref.read(communityRepositoryProvider);
    final items = await repo.listDeletedCommunitiesForRestore();
    _log.fine('Loaded ${items.length} restorable communities');
    return items;
  }

  /// Forces a fresh fetch from the server, bypassing the cache.
  Future<void> refresh() async {
    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final repo = ref.read(communityRepositoryProvider);
      return repo.listDeletedCommunitiesForRestore(refresh: true);
    });

    if (!ref.mounted) return;
    state = result;
  }
}

/// Provider for the deleted-communities-for-restore list.
///
/// Auto-disposes when no widget is watching, so navigating away from
/// Settings releases the list.
final deletedCommunitiesProvider = AsyncNotifierProvider.autoDispose<
    DeletedCommunitiesNotifier, List<DeletedCommunityItem>>(
  DeletedCommunitiesNotifier.new,
);
