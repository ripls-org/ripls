import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/services/user_service.dart';

/// User profile data with media URL.
class UserProfile {
  final GetUserResponse user;
  final String? mediaUrl;

  const UserProfile({required this.user, this.mediaUrl});
}

/// Repository for user data with transparent caching.
///
/// This repository wraps UserService and provides caching for user
/// data using the global TTL from the environment (CACHE_TTL_MINUTES).
class UserRepository {
  final CacheService _cache;
  final UserService _service;
  final MediaRepository _mediaRepository;

  UserRepository(
    CacheManager cacheManager,
    this._service,
    this._mediaRepository,
  ) : _cache = CacheService(cacheManager, 'user');

  /// Gets a user by ID with caching.
  Future<GetUserResponse> get(String userId) async {
    return _cache.get(
      key: userId,
      fetch: () => _service.getUser(userId),
    );
  }

  /// Gets a user profile with media URL resolved.
  ///
  /// This is a composite operation that fetches the user and their
  /// primary media URL (if they have one). Both are cached independently,
  /// so subsequent calls can benefit from either being cached.
  Future<UserProfile> getUserProfile(String userId) async {
    final user = await get(userId);

    String? mediaUrl;
    if (user.mediaIds.isNotEmpty) {
      try {
        final media = await _mediaRepository.getMediaUrl(user.mediaIds.first);
        mediaUrl = media.url;
      } catch (e) {
        // Gracefully handle missing media - user profile is still valid
      }
    }

    return UserProfile(user: user, mediaUrl: mediaUrl);
  }

  /// Gets all media URLs for a user with caching.
  ///
  /// Returns URLs for all media IDs in user.mediaIds.
  /// Empty list if user has no media.
  Future<List<String>> getUserMediaUrls(String userId) async {
    return _cache.get(
      key: 'media_urls:$userId',
      fetch: () async {
        final user = await get(userId);

        if (user.mediaIds.isEmpty) {
          return <String>[];
        }

        final urls = <String>[];
        for (final mediaId in user.mediaIds) {
          try {
            final media = await _mediaRepository.getMediaUrl(mediaId);
            urls.add(media.url);
          } catch (e) {
            // Gracefully handle missing media - continue with other URLs
          }
        }

        return urls;
      },
    );
  }

  /// Saves user profile information.
  ///
  /// This is a write operation that invalidates the user cache.
  /// Supports partial updates - only provided fields are updated.
  Future<void> saveUser({
    required String userId,
    String? name,
    List<String>? mediaIds,
    String? description,
    String? primaryResidenceLocationId,
    List<String>? otherLocationIds,
    String? preferredTimezone,
    String? preferredLanguage,
  }) async {
    await _service.saveUser(
      userId: userId,
      name: name,
      mediaIds: mediaIds,
      description: description,
      primaryResidenceLocationId: primaryResidenceLocationId,
      otherLocationIds: otherLocationIds,
      preferredTimezone: preferredTimezone,
      preferredLanguage: preferredLanguage,
    );

    // Invalidate user cache and media URLs cache after mutation
    await invalidate(userId);
    await _cache.invalidate('media_urls:$userId');
  }

  /// Gets the preferred timezone for a user.
  ///
  /// Returns the IANA timezone string (e.g., "America/New_York") or null if not set.
  /// The user will fall back to device timezone if null.
  Future<String?> getPreferredTimezone(String userId) async {
    final user = await get(userId);
    return user.preferredTimezone.isNotEmpty ? user.preferredTimezone : null;
  }

  /// Updates the user's preferred timezone.
  ///
  /// This is a write operation that invalidates the user cache.
  /// Pass an empty string or null to clear the timezone preference.
  Future<void> updatePreferredTimezone(
    String userId,
    String? timezone,
  ) async {
    await saveUser(
      userId: userId,
      preferredTimezone: timezone ?? '',
    );
  }

  /// Updates the user's preferred language (BCP-47 tag, e.g. `"en"`,
  /// `"es"`). Pass `null` or an empty string to clear and revert the
  /// server to header-based resolution.
  ///
  /// The server normalizes unsupported tags to its default. The
  /// LocalePreferenceNotifier fires this fire-and-forget when the
  /// user picks a new language so the server's stored preference
  /// catches up to the device's chosen locale.
  Future<void> updatePreferredLanguage(
    String userId,
    String? language,
  ) async {
    await saveUser(
      userId: userId,
      preferredLanguage: language ?? '',
    );
  }

  /// Gets aggregate user statistics with caching.
  ///
  /// Returns statistics including community count, item count, loans, borrows,
  /// giveaways, help offered, events hosted, and savings metrics.
  Future<GetUserStatsResponse> getUserStats(String userId) async {
    return _cache.get(
      key: 'stats:$userId',
      fetch: () => _service.getUserStats(userId),
    );
  }

  /// Invalidates cached statistics for a user.
  ///
  /// Use this when user stats may have changed (e.g., after creating/updating items,
  /// completing transfers, joining communities).
  Future<void> invalidateStats(String userId) async {
    await _cache.invalidate('stats:$userId');
  }

  /// Deletes the current user's account.
  ///
  /// This soft-deletes the user account on the server.
  /// After deletion, the user should be logged out and returned to login screen.
  Future<void> deleteUser() async {
    await _service.deleteUser();
    // Note: Cache invalidation is not critical here since the user will be logged out
    await invalidateAll();
  }

  /// Reads the calling user's user-scoped notification preferences with
  /// caching. Returns a row whose unset fields mean "category enabled"
  /// per the server's "unset = on" convention.
  Future<UserNotificationPreferences> getNotificationPreferences() async {
    return _cache.get(
      key: 'notification_prefs',
      fetch: () => _service.getUserNotificationPreferences(),
    );
  }

  /// Persists the calling user's user-scoped notification preferences and
  /// invalidates the cache so the next read fetches the fresh row.
  Future<UserNotificationPreferences> updateNotificationPreferences({
    required UserNotificationPreferences preferences,
  }) async {
    final updated = await _service.updateUserNotificationPreferences(
      preferences: preferences,
    );
    await _cache.invalidate('notification_prefs');
    return updated;
  }

  /// Invalidates a specific user entry.
  Future<void> invalidate(String userId) async {
    await _cache.invalidate(userId);
  }

  /// Invalidates all cached users.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }
}
