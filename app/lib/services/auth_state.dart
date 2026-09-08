import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/feed_view_model.dart'
    show feedProvider, feedStatusProvider;
import 'package:ripls/presentation/viewmodels/portfolio_view_model.dart'
    show portfolioViewModelProvider;
import 'package:ripls/services/device_location_service.dart' show LocationIntent;
import 'package:ripls/services/providers.dart' show cacheManagerProvider, authServiceProvider, communityEventPollerProvider, communityEventStreamProvider, userLocationProvider, secureStorageProvider, sharedPreferencesAsyncProvider;
import 'package:shared_preferences/shared_preferences.dart';

import '../core/config/environment.dart';

final _log = Logger('AuthState');

/// AuthStateData holds the authentication state data
class AuthStateData {
  final String? accessToken;
  final User? user;
  final bool isLoading;

  const AuthStateData({
    this.accessToken,
    this.user,
    this.isLoading = true, // Default to loading until we check storage
  });

  bool get isAuthenticated => accessToken != null && accessToken!.isNotEmpty;

  AuthStateData copyWith({
    String? accessToken,
    User? user,
    bool? isLoading,
  }) {
    return AuthStateData(
      accessToken: accessToken ?? this.accessToken,
      user: user ?? this.user,
      isLoading: isLoading ?? this.isLoading,
    );
  }

  static const empty = AuthStateData(isLoading: false);
}

/// AuthStateNotifier manages the authentication state of the user using Riverpod.
class AuthStateNotifier extends Notifier<AuthStateData> with WidgetsBindingObserver {
  // Providers give tests a clean injection seam without changing production behaviour.
  SharedPreferencesAsync get _prefs => ref.read(sharedPreferencesAsyncProvider);
  FlutterSecureStorage get _secureStorage => ref.read(secureStorageProvider);

  static const _refreshTokenKey = 'refresh_token';
  static const _accessTokenKey = 'access_token';

  // ---------------------------------------------------------------
  // Secure storage with web-insecure-origin fallback.
  //
  // flutter_secure_storage on web uses `crypto.subtle`, which Chrome
  // only exposes in secure contexts. `localhost` is always treated as
  // secure (even on HTTP), but other origins served over HTTP — most
  // notably `http://10.0.2.2:8080` from the Android emulator — are
  // not. Any call to `_secureStorage.{read,write,delete}` on such an
  // origin throws SecurityError, the awaiting code unwinds, and the
  // app freezes mid-setAuthState with no token persisted.
  //
  // On web, when the secure-storage path throws, fall back to
  // SharedPreferences for the same key. This is not a security
  // regression: SharedPreferences on web is also localStorage and is
  // already used for the user profile + server URL elsewhere in this
  // file. The "secure" part of secure-storage-on-web is largely
  // cosmetic anyway — anyone with DevTools can read localStorage —
  // so collapsing the two on insecure-context web origins is fine.
  // On mobile native, _secureStorage uses Keychain / EncryptedShared
  // Preferences and never throws this way; the helpers are pass-
  // through there.

  Future<String?> _readToken(String key) async {
    try {
      final v = await _secureStorage.read(key: key);
      if (v != null) return v;
      // Even on success-with-null, check the prefs fallback in case
      // a prior insecure-context write landed there.
      if (kIsWeb) return await _prefs.getString(key);
      return null;
    } catch (e) {
      if (kIsWeb) {
        _log.warning(
          '_secureStorage.read failed on web, falling back to '
          'SharedPreferences. This is expected on insecure-context '
          'origins like http://10.0.2.2:8080. err=$e',
        );
        return _prefs.getString(key);
      }
      rethrow;
    }
  }

  Future<void> _writeToken(String key, String value) async {
    try {
      await _secureStorage.write(key: key, value: value);
    } catch (e) {
      if (kIsWeb) {
        _log.warning(
          '_secureStorage.write failed on web, falling back to '
          'SharedPreferences. err=$e',
        );
        await _prefs.setString(key, value);
        return;
      }
      rethrow;
    }
  }

  Future<void> _deleteToken(String key) async {
    try {
      await _secureStorage.delete(key: key);
    } catch (_) {
      // ignore — fall through to prefs cleanup.
    }
    if (kIsWeb) {
      // Always also clear the prefs fallback so a re-login lands in
      // a clean state regardless of which path the previous write
      // took.
      await _prefs.remove(key);
    }
  }

  /// Minimum interval between sequential token refreshes.
  ///
  /// Guards against a server bug producing repeated 401s after a valid refresh,
  /// which would otherwise cause an unbounded refresh loop at ~150–200 ms per cycle.
  /// A successful refresh within this window is treated as a cache hit and returns
  /// true immediately — the proactive refresh timer (set during the original refresh)
  /// stays valid, so no timer reschedule is needed.
  static const _refreshMinInterval = Duration(seconds: 10);

  /// Timestamp of the last successful token refresh, used by the rate limiter.
  DateTime? _lastSuccessfulRefreshAt;

  /// Guards against concurrent logout calls.
  ///
  /// Multiple concurrent RPC failures (e.g. parallel InboxViewModel context
  /// fetches) each trigger onUnauthenticated independently. Without this guard
  /// they all call logout() simultaneously, running concurrent prefs.clear()
  /// and cacheManager.clear() calls that saturate the HTTP/2 connection and
  /// leave it in a broken state for the next user login.
  bool _isLoggingOut = false;

  /// Guards against concurrent token refresh attempts.
  ///
  /// Multiple simultaneous 401 responses trigger only one refresh attempt.
  /// All callers await the same in-flight refresh to prevent token rotation
  /// race conditions where concurrent refreshes invalidate each other.
  Completer<bool>? _refreshCompleter;

  Timer? _proactiveRefreshTimer;

  /// Timestamp of the last time the app transitioned to the background. Used
  /// to decide whether cached device position is stale on resume.
  DateTime? _lastPausedAt;

  /// Minimum time the app must spend in the background before the cached
  /// device position is considered stale and re-fetched on resume.
  static const _positionStalenessThreshold = Duration(minutes: 10);

  @override
  AuthStateData build() {
    // Register for lifecycle events so we can refresh on app resume.
    WidgetsBinding.instance.addObserver(this);
    ref.onDispose(() {
      WidgetsBinding.instance.removeObserver(this);
      _proactiveRefreshTimer?.cancel();
    });

    // Wire the global RPC refresh handler so executeRpc can transparently
    // refresh tokens without threading the callback through every service.
    RpcUtils.configureTokenRefresh(refreshAccessToken);

    return const AuthStateData(isLoading: true);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused) {
      _lastPausedAt = DateTime.now();
      return;
    }
    if (state == AppLifecycleState.resumed) {
      _checkAndRefreshOnResume();
      _maybeInvalidateUserLocationOnResume();
    }
  }

  /// Invalidate the cached device position if the app has been backgrounded
  /// for longer than [_positionStalenessThreshold]. The next consumer triggers
  /// a fresh GPS fix — necessary because the user may have physically moved
  /// while the app was backgrounded.
  void _maybeInvalidateUserLocationOnResume() {
    final pausedAt = _lastPausedAt;
    if (pausedAt == null) return;
    final elapsed = DateTime.now().difference(pausedAt);
    _lastPausedAt = null;
    if (elapsed < _positionStalenessThreshold) return;
    _log.info('🔄 App resumed after ${elapsed.inMinutes}m, invalidating cached user position');
    ref.invalidate(userLocationProvider(LocationIntent.proximityBias));
    ref.invalidate(userLocationProvider(LocationIntent.precisePin));
  }

  /// Checks whether the access token is near expiry on app resume and refreshes if so.
  void _checkAndRefreshOnResume() {
    final token = state.accessToken;
    if (token == null) return;
    final expiry = _parseJwtExpiry(token);
    if (expiry == null) return;
    if (expiry.isBefore(DateTime.now().add(const Duration(minutes: 5)))) {
      _log.info('🔄 Token near expiry on app resume, refreshing');
      refreshAccessToken();
    }
  }

  /// Schedules a proactive refresh 5 minutes before the access token expires.
  void _scheduleProactiveRefresh(String accessToken) {
    _proactiveRefreshTimer?.cancel();
    final expiry = _parseJwtExpiry(accessToken);
    if (expiry == null) return;

    final refreshAt = expiry.subtract(const Duration(minutes: 5));
    final now = DateTime.now();

    if (refreshAt.isAfter(now)) {
      final delay = refreshAt.difference(now);
      _log.info('⏰ Proactive refresh scheduled in ${delay.inMinutes}m');
      _proactiveRefreshTimer = Timer(delay, () {
        _log.info('⏰ Proactive token refresh triggered');
        refreshAccessToken();
      });
    } else {
      // Token is already near expiry — refresh immediately.
      _log.info('🔄 Token near expiry at startup, refreshing immediately');
      refreshAccessToken();
    }
  }

  /// Parses the JWT expiry time from the `exp` claim without requiring a JWT library.
  ///
  /// JWTs are base64url-encoded — decodes the payload segment directly.
  DateTime? _parseJwtExpiry(String token) {
    final parts = token.split('.');
    if (parts.length != 3) return null;

    // Pad base64url to a multiple of 4.
    var payload = parts[1];
    switch (payload.length % 4) {
      case 2:
        payload += '==';
      case 3:
        payload += '=';
    }

    try {
      final decoded = utf8.decode(base64Url.decode(payload));
      final claims = jsonDecode(decoded) as Map<String, dynamic>;
      final exp = claims['exp'];
      if (exp == null) return null;
      return DateTime.fromMillisecondsSinceEpoch((exp as int) * 1000);
    } catch (e) {
      _log.warning('⚠️ Failed to parse JWT expiry: $e');
      return null;
    }
  }

  /// Load authentication state from persistent storage.
  Future<void> loadAuthState() async {
    try {
      // Read from secure storage; fall back to SharedPreferences for one-shot migration.
      String? accessToken = await _readToken(_accessTokenKey);
      if (accessToken == null) {
        final legacy = await _prefs.getString('access_token');
        if (legacy != null) {
          await _writeToken(_accessTokenKey, legacy);
          await _prefs.remove('access_token');
          _log.info('🔑 Migrated access_token from SharedPreferences to SecureStorage');
          accessToken = legacy;
        }
      }
      final storedServerUrl = await _prefs.getString('server_url');
      final currentServerUrl = Environment.getServer;

      _log.info('🔑 Loading auth state from storage:');
      _log.info('   Stored server URL: $storedServerUrl');
      _log.info('   Current server URL: $currentServerUrl');

      // If we have a token but no stored server URL (old version),
      // we can't verify which server it's for, so clear it for safety
      if (accessToken != null && storedServerUrl == null) {
        _log.warning('⚠️ Found auth token without server URL - clearing for safety');
        _log.info('🚪 Please log in again to continue');
        await logout();
        return;
      }

      // If server URL changed, clear auth and return unauthenticated
      if (storedServerUrl != null && storedServerUrl != currentServerUrl) {
        _log.warning('⚠️ Server URL changed from $storedServerUrl to $currentServerUrl');
        _log.info('🚪 Clearing auth state due to server switch');
        await logout();
        return;
      }

      // Cold-start recovery: when access_token is absent but a refresh_token is
      // present in secure storage, attempt a refresh before falling through to
      // unauthenticated. This closes the gap where the OS killed the process
      // mid-write and the access_token write was lost while the refresh_token
      // write (earlier in setAuthState) survived.
      if (accessToken == null) {
        final storedRefreshToken = await _readToken(_refreshTokenKey);
        if (storedRefreshToken != null && storedRefreshToken.isNotEmpty) {
          _log.info('🔄 No access token on cold start, attempting refresh from stored refresh token');
          final refreshed = await refreshAccessToken();
          if (refreshed) {
            accessToken = state.accessToken;
            _log.info('✅ Cold-start refresh succeeded');
          } else {
            _log.warning('⚠️ Cold-start refresh failed, user must re-authenticate');
          }
        }
      }

      final userJson = await _prefs.getString('user');
      User? user;
      if (userJson != null) {
        try {
          final userMap = jsonDecode(userJson) as Map<String, dynamic>;
          user = User(
            id: userMap['id'] as String? ?? '',
            name: userMap['name'] as String? ?? '',
            mediaId: userMap['mediaId'] as String? ?? '',
          );
        } catch (e) {
          _log.warning('⚠️ Failed to parse user JSON: $e');
        }
      }

      // Edge case: refresh succeeded but user record is absent (e.g. external
      // storage wipe left only the refresh_token). Fall through to unauthenticated
      // rather than presenting a half-initialized session.
      if (accessToken != null && user == null) {
        _log.warning('⚠️ Token present but user record missing - falling through to unauthenticated');
        accessToken = null;
      }

      _log.info('   accessToken: ${accessToken != null ? "[${accessToken.length} chars]" : "null"}');
      _log.info('   user: ${user?.id} (${user?.name})');

      final newState = AuthStateData(
        accessToken: accessToken,
        user: user,
        isLoading: false,
      );

      state = newState;

      // Schedule proactive refresh if we have a valid token.
      if (accessToken != null) {
        _scheduleProactiveRefresh(accessToken);
      }

      _log.info('✅ Auth state loaded: isAuthenticated=${newState.isAuthenticated}');
    } catch (e) {
      _log.severe('❌ Failed to load auth state: $e');
      state = AuthStateData.empty;
    }
  }

  /// Save authentication state to persistent storage.
  ///
  /// **CRITICAL SECURITY:** Clears all caches before setting new user state.
  /// This handles edge cases where logout wasn't explicitly called:
  /// - App force-quit (OS cleared SharedPreferences but cache persists)
  /// - App reinstall (SharedPreferences gone, cache may remain)
  /// - Development/testing (manual storage clearing)
  ///
  /// This provides defense in depth alongside logout cache clearing.
  Future<void> setAuthState({
    required String accessToken,
    required User user,
    String? refreshToken,
  }) async {
    // Reset logout guard so the new session can log out normally later.
    _isLoggingOut = false;

    // CRITICAL: Clear cache before setting new user to prevent cross-user leakage
    await _clearUserCaches();

    // Persist before mutating in-memory state so an OS kill mid-write leaves disk consistent.
    await _writeToken(_accessTokenKey, accessToken);

    // Store refresh token in encrypted storage.
    if (refreshToken != null && refreshToken.isNotEmpty) {
      await _writeToken(_refreshTokenKey, refreshToken);
    }

    // Serialize user as JSON
    final userJson = jsonEncode({
      'id': user.id,
      'name': user.name,
      'mediaId': user.mediaId,
    });
    await _prefs.setString('user', userJson);
    await _prefs.setString('server_url', Environment.getServer);

    state = AuthStateData(
      accessToken: accessToken,
      user: user,
      isLoading: false,
    );

    // Schedule proactive refresh for the new token.
    _scheduleProactiveRefresh(accessToken);

    _log.info('✅ Auth state saved for user: ${user.name}');
    _log.info('   Server URL: ${Environment.getServer}');
  }

  /// Updates the current user's data in auth state and persistent storage.
  ///
  /// This method is used when the user updates their profile (e.g., name, mediaId)
  /// to ensure the auth state reflects the latest user data without requiring re-login.
  /// Unlike setAuthState, this does NOT clear caches since it's the same user.
  Future<void> updateUser(User user) async {
    if (state.accessToken == null) {
      _log.warning('⚠️ Cannot update user - no active session');
      return;
    }

    state = state.copyWith(user: user);

    // Persist updated user to SharedPreferences
    final userJson = jsonEncode({
      'id': user.id,
      'name': user.name,
      'mediaId': user.mediaId,
    });
    await _prefs.setString('user', userJson);

    _log.info('✅ User data updated in auth state: ${user.name}');
  }

  /// Attempts to refresh the access token using the stored refresh token.
  ///
  /// Returns true if the access token was successfully refreshed, false otherwise.
  /// Guards against concurrent refresh calls — multiple callers await the same
  /// in-flight refresh and all receive the same result.
  /// Rate-limited: returns true immediately if a successful refresh completed
  /// within [_refreshMinInterval] to prevent unbounded sequential refresh storms.
  Future<bool> refreshAccessToken() async {
    // Refusing to refresh during logout is the load-bearing guard against
    // the rebound described in #2158: a stale in-flight RPC that lost its
    // access token to logout will hit 401, ask us to refresh, and — because
    // SecureStorage hasn't yet finished deleting the refresh_token — get a
    // brand-new access token here. The subsequent `state =
    // state.copyWith(accessToken: ...)` flips `isAuthenticated` back to true
    // and the main.dart auth listener silently re-enters the app. From the
    // user's perspective: tap logout, briefly land on /login, then bounce
    // back to /home with no warning. The router transition mid-bounce is
    // what trips the IndexedStack/RenderIgnorePointer assertion.
    if (_isLoggingOut) {
      _log.fine('🔄 Refresh suppressed: logout in progress');
      LogoutDiagnostics.trace('REFRESH_TOKEN_SUPPRESSED_DURING_LOGOUT');
      return false;
    }
    // If a refresh is already in progress, wait for it rather than starting another.
    if (_refreshCompleter != null) {
      _log.fine('🔄 Refresh already in progress, awaiting result');
      return _refreshCompleter!.future;
    }

    // Rate-limit sequential refreshes. The proactive refresh timer was already
    // scheduled by the original successful refresh and remains valid, so no
    // reschedule is needed here.
    final lastRefresh = _lastSuccessfulRefreshAt;
    if (lastRefresh != null &&
        DateTime.now().difference(lastRefresh) < _refreshMinInterval) {
      _log.fine(
        '🔄 Skipping refresh: succeeded ${DateTime.now().difference(lastRefresh).inMilliseconds}ms ago',
      );
      return true;
    }

    _refreshCompleter = Completer<bool>();
    try {
      final rawRefreshToken = await _readToken(_refreshTokenKey);
      if (rawRefreshToken == null || rawRefreshToken.isEmpty) {
        _log.warning('⚠️ No refresh token available');
        _refreshCompleter!.complete(false);
        return false;
      }

      final authService = ref.read(authServiceProvider);
      final result = await authService.refreshToken(refreshToken: rawRefreshToken);

      if (!result.success || result.accessToken == null || result.refreshToken == null) {
        _log.warning('⚠️ Token refresh failed');
        _refreshCompleter!.complete(false);
        return false;
      }

      // Persist before mutating in-memory state so an OS kill mid-refresh leaves disk consistent.
      // Concurrent reads of state.accessToken between the disk write start and state mutation
      // can use the old token for ~20-50 ms; those RPCs will succeed or hit the 401-retry path
      // which finds the new token in state after this mutation completes.
      await _writeToken(_accessTokenKey, result.accessToken!);

      // Rotate: store new refresh token, discard old.
      await _writeToken(_refreshTokenKey, result.refreshToken!);

      // Update state with new access token.
      state = state.copyWith(accessToken: result.accessToken);

      // Schedule next proactive refresh.
      _scheduleProactiveRefresh(result.accessToken!);

      _lastSuccessfulRefreshAt = DateTime.now();

      _log.info('✅ Token refreshed successfully');
      _refreshCompleter!.complete(true);
      return true;
    } catch (e) {
      _log.severe('❌ Token refresh error: $e');
      _refreshCompleter?.complete(false);
      return false;
    } finally {
      _refreshCompleter = null;
    }
  }

  /// Clear authentication state (logout).
  /// This removes all user data from both memory and persistent storage.
  ///
  /// When logout is called, the router will automatically redirect to the login screen
  /// and preserve the current location in the 'from' query parameter, allowing the user
  /// to return to their intended destination after re-authenticating.
  ///
  /// **CRITICAL SECURITY:** Also clears all user-specific caches to prevent
  /// cross-user data leakage when another user logs in on the same device.
  Future<void> logout() async {
    if (_isLoggingOut) {
      _log.fine('🔒 Logout already in progress, skipping duplicate call');
      LogoutDiagnostics.trace('LOGOUT_DUPLICATE_CALL_SKIPPED');
      return;
    }
    _isLoggingOut = true;
    _lastSuccessfulRefreshAt = null;
    _proactiveRefreshTimer?.cancel();
    LogoutDiagnostics.markLogoutStarted();
    _log.info('🚪 Starting logout process...');

    try {
      // First, clear in-memory state
      // This triggers the router's redirect logic which will preserve the current location
      LogoutDiagnostics.trace('AUTH_STATE_CLEAR_BEGIN');
      state = const AuthStateData(
        accessToken: null,
        user: null,
        isLoading: false,
      );
      LogoutDiagnostics.trace('AUTH_STATE_CLEAR_END');

      // Then, clear all stored authentication data
      await _prefs.remove('access_token'); // belt-and-suspenders: clears any pre-migration value
      await _prefs.remove('user');
      LogoutDiagnostics.trace('PREFS_REMOVE_AUTH_KEYS_DONE');

      // Clear tokens from encrypted storage.
      await _deleteToken(_accessTokenKey);
      await _deleteToken(_refreshTokenKey);
      LogoutDiagnostics.trace('SECURE_STORAGE_DELETE_DONE');

      // Stop event pollers and streams before clearing caches
      ref.read(communityEventPollerProvider).reset();
      ref.read(communityEventStreamProvider).reset();
      LogoutDiagnostics.trace('POLLERS_STOPPED');

      // CRITICAL: Clear user-specific caches to prevent cross-user data leakage
      await _clearUserCaches();
      LogoutDiagnostics.trace('CACHES_CLEARED');

      // Also clear any other user-related data that might be stored
      final prefs = await SharedPreferences.getInstance();
      final allKeys = prefs.getKeys();
      _log.info('📋 Found ${allKeys.length} stored keys');

      // Clear all keys to ensure complete logout
      await prefs.clear();
      LogoutDiagnostics.trace('PREFS_CLEAR_DONE');

      _log.info('✅ All user data cleared');
      _log.info('👋 User logged out successfully');
    } catch (e) {
      _log.severe('❌ Error during logout: $e');
      LogoutDiagnostics.trace('LOGOUT_ERROR', e.toString());
    }
    LogoutDiagnostics.markLogoutFinished();
    // Note: _isLoggingOut is intentionally NOT reset here. It stays true
    // until the next successful login (setAuthState). This prevents the
    // cascade where in-flight RPCs see no token after logout, trigger
    // onUnauthenticated, which calls logout again in a tight loop.
  }

  /// Clears all caches to prevent cross-user data leakage.
  ///
  /// Called on both logout and login to ensure User B never sees User A's cached data.
  ///
  /// **Nuclear clear approach:**
  /// - Clears ALL caches to guarantee no cross-user data leakage
  /// - Simpler and more maintainable than pattern-based clearing
  /// - Zero risk of forgetting to clear a new cache pattern
  /// - Minor performance trade-off (~200-500ms slower first load after auth change)
  ///
  /// See docs/ai/cache_security_audit.md for full security analysis.
  Future<void> _clearUserCaches() async {
    try {
      final cacheManager = ref.read(cacheManagerProvider);

      _log.info('🧹 Clearing all caches');

      // Clear ALL caches to guarantee no cross-user data leakage
      // This is simpler and more secure than pattern-based clearing
      await cacheManager.clear();

      // Reset Riverpod provider state so the next user's initialize() always
      // fetches fresh data instead of hitting the early-return guard with
      // the previous user's communityId + items still in memory.
      ref.invalidate(feedProvider);
      ref.invalidate(feedStatusProvider);
      ref.invalidate(portfolioViewModelProvider);
      // Position is scoped to the current user (primary-residence fallback
      // differs between users). Invalidate both intents so the new session's
      // first read re-resolves via UserPositionResolver with the new auth
      // context.
      ref.invalidate(userLocationProvider(LocationIntent.proximityBias));
      ref.invalidate(userLocationProvider(LocationIntent.precisePin));

      _log.info('✅ All caches cleared');
    } catch (e) {
      _log.severe('❌ Error clearing caches: $e');
      // Don't fail auth operations if cache clearing fails
    }
  }
}
