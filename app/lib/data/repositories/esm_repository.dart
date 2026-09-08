import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/esm_service.dart';

/// EsmRepository wraps the ESM RPC service for client-side voting on the
/// recap-story embedded prompt. Submitting a response flips the
/// recipient's row off `UNRESOLVED` server-side; the client notifies
/// feed-cache listeners so the next pull-to-refresh / feed reload picks
/// up the change.
///
/// Cache namespace: `'esm'`.
class EsmRepository {
  final CacheService _cache;
  final EsmService _service;
  final void Function()? _onFeedListingInvalidated;
  final void Function()? _onEsmInvalidated;

  EsmRepository(
    CacheManager cacheManager,
    this._service, {
    void Function()? onFeedListingInvalidated,
    void Function()? onEsmInvalidated,
  })  : _cache = CacheService(cacheManager, 'esm'),
        _onFeedListingInvalidated = onFeedListingInvalidated,
        _onEsmInvalidated = onEsmInvalidated;

  /// Submit a structured response to an ESM prompt. The server flips the
  /// recipient's row to RESPONDED and the client notifies feed listeners
  /// so the screen refreshes.
  Future<void> respond({
    required String promptId,
    required String responseOptionKey,
  }) async {
    await _service.respondToESMPrompt(
      promptId: promptId,
      responseOptionKey: responseOptionKey,
    );
    await _cache.invalidate(promptId);
    _onFeedListingInvalidated?.call();
    _onEsmInvalidated?.call();
  }
}
