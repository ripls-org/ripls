import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/services/auth_service.dart';
import 'package:ripls/services/community_event_poller.dart';
import 'package:ripls/services/community_event_stream.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

@GenerateMocks([
  AuthService,
  FlutterSecureStorage,
  CacheManager,
  SharedPreferencesAsync,
  CommunityEventPoller,
  CommunityEventStreamService,
])
import 'auth_state_notifier_test.mocks.dart';

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

const _storedRefreshToken = 'stored-refresh-token';
const _newAccessToken = 'new-access-token-abc';
const _newRefreshToken = 'new-refresh-token-xyz';
final _userJson = jsonEncode({
  'id': 'user-123',
  'name': 'Test User',
  'mediaId': 'media-123',
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

ProviderContainer _makeContainer({
  required MockAuthService authService,
  required MockFlutterSecureStorage secureStorage,
  required MockCacheManager cacheManager,
  required MockSharedPreferencesAsync prefs,
  MockCommunityEventPoller? poller,
  MockCommunityEventStreamService? streamSvc,
}) {
  return ProviderContainer(
    overrides: [
      authServiceProvider.overrideWithValue(authService),
      secureStorageProvider.overrideWithValue(secureStorage),
      cacheManagerProvider.overrideWithValue(cacheManager),
      sharedPreferencesAsyncProvider.overrideWithValue(prefs),
      if (poller != null) communityEventPollerProvider.overrideWithValue(poller),
      if (streamSvc != null) communityEventStreamProvider.overrideWithValue(streamSvc),
    ],
  );
}

// Stubs a successful token refresh on all relevant mocks.
void _stubSuccessfulRefresh({
  required MockAuthService authService,
  required MockFlutterSecureStorage secureStorage,
  required MockSharedPreferencesAsync prefs,
}) {
  // Stub only the refresh_token key so that the access_token key falls
  // through to the setUp catch-all (null), allowing loadAuthState to
  // take the cold-start recovery path.
  when(secureStorage.read(key: 'refresh_token'))
      .thenAnswer((_) async => _storedRefreshToken);
  when(secureStorage.write(key: anyNamed('key'), value: anyNamed('value')))
      .thenAnswer((_) async {});
  when(authService.refreshToken(refreshToken: anyNamed('refreshToken')))
      .thenAnswer((_) async => TokenRefreshResult(
            success: true,
            accessToken: _newAccessToken,
            refreshToken: _newRefreshToken,
          ));
  when(prefs.setString(any, any)).thenAnswer((_) async {});
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late MockAuthService mockAuthService;
  late MockFlutterSecureStorage mockSecureStorage;
  late MockCacheManager mockCacheManager;
  late MockSharedPreferencesAsync mockPrefs;

  setUp(() {
    mockAuthService = MockAuthService();
    mockSecureStorage = MockFlutterSecureStorage();
    mockCacheManager = MockCacheManager();
    mockPrefs = MockSharedPreferencesAsync();

    // Default: empty storage — no credentials persisted.
    when(mockPrefs.getString(any)).thenAnswer((_) async => null);
    when(mockPrefs.setString(any, any)).thenAnswer((_) async {});
    when(mockPrefs.remove(any)).thenAnswer((_) async {});
    when(mockCacheManager.clear()).thenAnswer((_) async {});
    when(mockSecureStorage.read(key: anyNamed('key'))).thenAnswer((_) async => null);
    when(mockSecureStorage.delete(key: anyNamed('key'))).thenAnswer((_) async {});
    when(mockSecureStorage.write(key: anyNamed('key'), value: anyNamed('value')))
        .thenAnswer((_) async {});
  });

  // -------------------------------------------------------------------------
  // Phase 1: Cold-start refresh probe
  // -------------------------------------------------------------------------

  group('loadAuthState — cold-start refresh probe', () {
    test('recovers when access_token is missing but refresh_token is present', () async {
      when(mockPrefs.getString('user')).thenAnswer((_) async => _userJson);
      _stubSuccessfulRefresh(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        prefs: mockPrefs,
      );

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      await container.read(authStateProvider.notifier).loadAuthState();

      final authState = container.read(authStateProvider);
      expect(authState.isAuthenticated, true);
      expect(authState.accessToken, _newAccessToken);
      expect(authState.user, isNotNull);
      expect(authState.user?.id, 'user-123');

      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);
    });

    test('falls through to unauthenticated when no refresh_token is present', () async {
      // Default mock already returns null for secureStorage.read — no refresh token.

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      await container.read(authStateProvider.notifier).loadAuthState();

      final authState = container.read(authStateProvider);
      expect(authState.isAuthenticated, false);
      expect(authState.accessToken, isNull);

      verifyNever(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      ));
    });

    test('falls through when refresh_token is present but refresh fails', () async {
      when(mockSecureStorage.read(key: 'refresh_token'))
          .thenAnswer((_) async => _storedRefreshToken);
      when(mockAuthService.refreshToken(refreshToken: anyNamed('refreshToken')))
          .thenAnswer((_) async => TokenRefreshResult(success: false));

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      await container.read(authStateProvider.notifier).loadAuthState();

      final authState = container.read(authStateProvider);
      expect(authState.isAuthenticated, false);

      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);
    });

    test('falls through with warning when refresh succeeds but user JSON is missing', () async {
      // Refresh succeeds but no user JSON in prefs — can happen if storage was
      // partially wiped by an external mechanism.
      when(mockPrefs.getString('user')).thenAnswer((_) async => null);
      _stubSuccessfulRefresh(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        prefs: mockPrefs,
      );

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      await container.read(authStateProvider.notifier).loadAuthState();

      // Even though refresh succeeded, no user record → fall through to unauthenticated.
      final authState = container.read(authStateProvider);
      expect(authState.isAuthenticated, false);
      expect(authState.user, isNull);

      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);
    });
  });

  // -------------------------------------------------------------------------
  // Phase 2: Sequential-refresh rate limiter
  // -------------------------------------------------------------------------

  group('refreshAccessToken — rate limiter', () {
    test('returns cached true within rate-limit window without calling service again', () async {
      _stubSuccessfulRefresh(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        prefs: mockPrefs,
      );

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      final notifier = container.read(authStateProvider.notifier);

      final first = await notifier.refreshAccessToken();
      expect(first, true);

      // Immediate second call — inside the 10 s rate-limit window.
      final second = await notifier.refreshAccessToken();
      expect(second, true);

      // Service was called exactly once despite two refresh calls.
      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);
    });

    test('logout suppresses refresh; relogin via setAuthState resets the rate limit', () async {
      // SharedPreferences.getInstance() is used in logout() for the legacy clear path.
      SharedPreferences.setMockInitialValues({});

      final mockPoller = MockCommunityEventPoller();
      final mockStream = MockCommunityEventStreamService();

      _stubSuccessfulRefresh(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        prefs: mockPrefs,
      );

      final container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
        poller: mockPoller,
        streamSvc: mockStream,
      );
      addTearDown(container.dispose);

      final notifier = container.read(authStateProvider.notifier);

      // First refresh — calls service and sets the rate-limit timestamp.
      await notifier.refreshAccessToken();

      // Immediate second call confirms rate-limit is active.
      await notifier.refreshAccessToken();
      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);

      // Logout latches the _isLoggingOut guard introduced in #2158 to prevent
      // the post-logout refresh rebound.
      await notifier.logout();

      // Re-stub since logout clears storage.
      _stubSuccessfulRefresh(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        prefs: mockPrefs,
      );

      // Refresh after logout is suppressed — the guard blocks the call entirely.
      final suppressed = await notifier.refreshAccessToken();
      expect(suppressed, false);
      verifyNever(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      ));

      // A new session via setAuthState clears the logout guard and the
      // rate-limit timestamp, so the next refresh call hits the service.
      await notifier.setAuthState(
        accessToken: 'fresh-access-token',
        user: User(id: 'user-123', name: 'Test User', mediaId: ''),
        refreshToken: _storedRefreshToken,
      );

      await notifier.refreshAccessToken();
      verify(mockAuthService.refreshToken(
        refreshToken: anyNamed('refreshToken'),
      )).called(1);
    });
  });

  // -------------------------------------------------------------------------
  // Phase 3: Persist-before-state ordering
  // -------------------------------------------------------------------------

  group('refreshAccessToken — persist-before-state ordering', () {
    test('writes secure storage before mutating in-memory state', () async {
      final callOrder = <String>[];
      late ProviderContainer container;

      // Register the specific stub AFTER setUp()'s catch-all so it takes precedence.
      when(mockSecureStorage.write(key: anyNamed('key'), value: anyNamed('value')))
          .thenAnswer((_) async {
        callOrder.add('secure_write');
      });
      when(mockSecureStorage.read(key: anyNamed('key')))
          .thenAnswer((_) async => _storedRefreshToken);
      when(mockAuthService.refreshToken(refreshToken: anyNamed('refreshToken')))
          .thenAnswer((_) async => TokenRefreshResult(
                success: true,
                accessToken: _newAccessToken,
                refreshToken: _newRefreshToken,
              ));

      container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      container.listen(
        authStateProvider,
        (_, next) {
          if (next.accessToken == _newAccessToken) {
            callOrder.add('state_mutation');
          }
        },
        fireImmediately: false,
      );

      await container.read(authStateProvider.notifier).refreshAccessToken();

      expect(callOrder, contains('secure_write'));
      expect(callOrder, contains('state_mutation'));
      expect(
        callOrder.indexOf('secure_write'),
        lessThan(callOrder.indexOf('state_mutation')),
        reason: 'secure storage must be written before state is mutated',
      );
    });
  });

  group('setAuthState — persist-before-state ordering', () {
    test('writes secure storage before mutating in-memory state', () async {
      final callOrder = <String>[];
      late ProviderContainer container;

      const testToken = 'set-auth-access-token';
      const testRefreshToken = 'set-auth-refresh-token';

      // Register the specific stub AFTER setUp()'s catch-all so it takes precedence.
      when(mockSecureStorage.write(key: anyNamed('key'), value: anyNamed('value')))
          .thenAnswer((_) async {
        callOrder.add('secure_write');
      });

      container = _makeContainer(
        authService: mockAuthService,
        secureStorage: mockSecureStorage,
        cacheManager: mockCacheManager,
        prefs: mockPrefs,
      );
      addTearDown(container.dispose);

      container.listen(
        authStateProvider,
        (_, next) {
          if (next.accessToken == testToken) {
            callOrder.add('state_mutation');
          }
        },
        fireImmediately: false,
      );

      final testUser = User(id: 'u1', name: 'Alice', mediaId: '');
      await container.read(authStateProvider.notifier).setAuthState(
            accessToken: testToken,
            user: testUser,
            refreshToken: testRefreshToken,
          );

      expect(callOrder, contains('secure_write'));
      expect(callOrder, contains('state_mutation'));
      expect(
        callOrder.indexOf('secure_write'),
        lessThan(callOrder.indexOf('state_mutation')),
        reason: 'secure storage must be written before state is mutated',
      );
    });
  });
}
