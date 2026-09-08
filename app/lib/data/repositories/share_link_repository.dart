import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/repositories/auth_repository.dart';

export 'package:ripls/data/repositories/auth_repository.dart'
    show InvitationCheckResult, ShareLinkTargetKind;

/// Repository for resolving share-link shortcodes to their typed
/// invitation context (target kind, target ID, hosting community,
/// inviter, member counts).
///
/// Wraps [AuthRepository.checkInvitation] and adds Stash caching so
/// repeat resolutions of the same short code skip the network. Cache
/// namespace `share_link` is parallel to the existing `invite`
/// namespace from the legacy GetOrCreateInviteLink flow.
class ShareLinkRepository {
  final CacheService _cache;
  final AuthRepository _authRepository;

  ShareLinkRepository(CacheManager cacheManager, this._authRepository)
      : _cache = CacheService(cacheManager, 'share_link');

  /// Resolves [shortCode] to invitation context. Cached by short code.
  Future<InvitationCheckResult> describe(String shortCode) {
    return _cache.get<InvitationCheckResult>(
      key: shortCode,
      fetch: () => _authRepository.checkInvitation(shortCode: shortCode),
    );
  }

  /// Drops the cached resolution for [shortCode]. Call after the link
  /// is accepted or revoked so the next read pulls fresh state.
  Future<void> invalidate(String shortCode) {
    return _cache.invalidate(shortCode);
  }
}
