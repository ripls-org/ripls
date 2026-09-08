import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/community_event_poller.dart';
import 'package:ripls/services/community_event_stream.dart';
import 'package:ripls/services/providers.dart'
    show
        authStateProvider,
        cacheManagerProvider,
        communityEventPollerProvider,
        communityEventStreamProvider;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/in_memory_shared_preferences_async.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_async_platform_interface.dart';

import 'auth_state_test.mocks.dart';

@GenerateMocks([CacheManager, CommunityEventPoller, CommunityEventStreamService])
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('AuthStateData', () {
    test('isAuthenticated returns true when accessToken is present', () {
      final state = AuthStateData(
        accessToken: 'test-token',
        user: User(id: 'user-123', name: 'Test User'),
        isLoading: false,
      );

      expect(state.isAuthenticated, true);
    });

    test('isAuthenticated returns false when accessToken is null', () {
      final state = AuthStateData(
        accessToken: null,
        user: User(id: 'user-123', name: 'Test User'),
        isLoading: false,
      );

      expect(state.isAuthenticated, false);
    });

    test('isAuthenticated returns false when accessToken is empty', () {
      final state = AuthStateData(
        accessToken: '',
        user: User(id: 'user-123', name: 'Test User'),
        isLoading: false,
      );

      expect(state.isAuthenticated, false);
    });

    test('copyWith preserves existing values when not overridden', () {
      final original = AuthStateData(
        accessToken: 'token',
        user: User(id: 'id', name: 'name'),
        isLoading: false,
      );

      final copied = original.copyWith(user: User(id: 'new-id', name: 'name'));

      expect(copied.accessToken, 'token');
      expect(copied.user?.id, 'new-id');
      expect(copied.user?.name, 'name');
      expect(copied.isLoading, false);
    });

    test('copyWith can update accessToken', () {
      final original = AuthStateData(
        accessToken: 'token',
        user: User(id: 'id', name: 'name'),
        isLoading: false,
      );

      final copied = original.copyWith(accessToken: 'new-token');

      expect(copied.accessToken, 'new-token');
      expect(copied.user?.id, 'id');
      expect(copied.user?.name, 'name');
      expect(copied.isLoading, false);
    });

    test('copyWith can update user', () {
      final original = AuthStateData(
        accessToken: 'token',
        user: User(id: 'id', name: 'name'),
        isLoading: false,
      );

      final copied = original.copyWith(user: User(id: 'id', name: 'new-name'));

      expect(copied.accessToken, 'token');
      expect(copied.user?.id, 'id');
      expect(copied.user?.name, 'new-name');
      expect(copied.isLoading, false);
    });

    test('copyWith can update isLoading', () {
      final original = AuthStateData(
        accessToken: 'token',
        user: User(id: 'id', name: 'name'),
        isLoading: false,
      );

      final copied = original.copyWith(isLoading: true);

      expect(copied.accessToken, 'token');
      expect(copied.user?.id, 'id');
      expect(copied.user?.name, 'name');
      expect(copied.isLoading, true);
    });

    test('empty state has no auth data and is not loading', () {
      expect(AuthStateData.empty.accessToken, null);
      expect(AuthStateData.empty.user, null);
      expect(AuthStateData.empty.isLoading, false);
      expect(AuthStateData.empty.isAuthenticated, false);
    });
  });

  // Tests for AuthStateNotifier storage behaviour.
  //
  // SharedPreferencesAsync is mocked via SharedPreferencesAsyncPlatform.instance
  // (InMemorySharedPreferencesAsync). FlutterSecureStorage is mocked via a
  // MethodChannel handler backed by an in-memory map.
  group('AuthStateNotifier', () {
    late Map<String, String> secureStore;
    late MockCacheManager mockCacheManager;
    late MockCommunityEventPoller mockPoller;
    late MockCommunityEventStreamService mockStreamService;
    late ProviderContainer container;

    // Channel name matches the flutter_secure_storage plugin registration.
    const secureChannel =
        MethodChannel('plugins.it_nomads.com/flutter_secure_storage');

    // The stored auth state is keyed by the server it came from, and
    // loadAuthState deliberately discards state belonging to a different one.
    // Read from Environment rather than repeating a host: the value is now a
    // dart-define (#2953), which `flutter test` does not supply, so any literal
    // here would silently stop matching and take the assertions with it.
    final serverUrl = Environment.getServer;
    final userJson = jsonEncode(
        {'id': 'user-123', 'name': 'Test User', 'mediaId': 'media-456'});

    void setUpSecureStorageMock() {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(secureChannel, (MethodCall call) async {
        final args = call.arguments as Map<Object?, Object?>;
        switch (call.method) {
          case 'read':
            return secureStore[args['key']! as String];
          case 'write':
            final key = args['key']! as String;
            final value = args['value'];
            if (value != null) {
              secureStore[key] = value as String;
            }
            return null;
          case 'delete':
            secureStore.remove(args['key']! as String);
            return null;
          case 'readAll':
            return Map<String, String>.from(secureStore);
          case 'deleteAll':
            secureStore.clear();
            return null;
          default:
            return null;
        }
      });
    }

    setUp(() {
      secureStore = {};
      SharedPreferences.setMockInitialValues({});
      SharedPreferencesAsyncPlatform.instance =
          InMemorySharedPreferencesAsync.empty();
      setUpSecureStorageMock();

      mockCacheManager = MockCacheManager();
      when(mockCacheManager.clear()).thenAnswer((_) async {});
      when(mockCacheManager.clear(pattern: anyNamed('pattern')))
          .thenAnswer((_) async {});

      mockPoller = MockCommunityEventPoller();
      mockStreamService = MockCommunityEventStreamService();

      container = ProviderContainer(
        overrides: [
          cacheManagerProvider.overrideWithValue(mockCacheManager),
          communityEventPollerProvider.overrideWithValue(mockPoller),
          communityEventStreamProvider.overrideWithValue(mockStreamService),
        ],
      );
    });

    tearDown(() {
      container.dispose();
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(secureChannel, null);
    });

    test(
        'loadAuthState migrates access_token from SharedPreferences to SecureStorage',
        () async {
      SharedPreferencesAsyncPlatform.instance =
          InMemorySharedPreferencesAsync.withData({
        'server_url': serverUrl,
        'user': userJson,
        'access_token': 'legacy-token',
      });

      final notifier = container.read(authStateProvider.notifier);
      await notifier.loadAuthState();

      // Token is migrated to secure storage.
      expect(secureStore['access_token'], 'legacy-token');
      // State is authenticated using the migrated token.
      expect(container.read(authStateProvider).isAuthenticated, isTrue);
      expect(container.read(authStateProvider).accessToken, 'legacy-token');
    });

    test(
        'loadAuthState uses SecureStorage value when already present',
        () async {
      secureStore['access_token'] = 'secure-token';
      SharedPreferencesAsyncPlatform.instance =
          InMemorySharedPreferencesAsync.withData({
        'server_url': serverUrl,
        'user': userJson,
      });

      final notifier = container.read(authStateProvider.notifier);
      await notifier.loadAuthState();

      // The secure-storage value is used.
      expect(container.read(authStateProvider).accessToken, 'secure-token');
      // The secure-storage value is unchanged.
      expect(secureStore['access_token'], 'secure-token');
    });

    test('loadAuthState discards state stored against a different server',
        () async {
      // The two tests above seed `serverUrl`, so they exercise the *match*
      // path. This covers the discard that gives that comparison its point —
      // and it is the one that matters now that the server URL is build-time
      // config (#2953): pointing a build at another deployment must not carry
      // the previous deployment's token along.
      secureStore['access_token'] = 'token-from-elsewhere';
      SharedPreferencesAsyncPlatform.instance =
          InMemorySharedPreferencesAsync.withData({
        'server_url': 'https://some-other-deployment.example.org',
        'user': userJson,
      });

      final notifier = container.read(authStateProvider.notifier);
      await notifier.loadAuthState();

      expect(container.read(authStateProvider).isAuthenticated, isFalse);
      expect(container.read(authStateProvider).accessToken, isNull);
    });

    test('loadAuthState ends in unauthenticated state when both stores are empty',
        () async {
      // Both stores are empty (set in setUp via InMemorySharedPreferencesAsync.empty()).
      final notifier = container.read(authStateProvider.notifier);
      await notifier.loadAuthState();

      final state = container.read(authStateProvider);
      expect(state.isAuthenticated, isFalse);
      expect(state.isLoading, isFalse);
      expect(state.accessToken, isNull);
    });

    test('setAuthState writes access_token only to SecureStorage', () async {
      final notifier = container.read(authStateProvider.notifier);
      final user = User(id: 'user-123', name: 'Test User', mediaId: '');

      await notifier.setAuthState(
        accessToken: 'new-secure-token',
        user: user,
        refreshToken: 'refresh-token',
      );

      // Access token must be in secure storage.
      expect(secureStore['access_token'], 'new-secure-token');
      // In-memory state reflects the new token.
      expect(container.read(authStateProvider).accessToken, 'new-secure-token');
    });

    test('logout clears access_token and refresh_token from SecureStorage',
        () async {
      secureStore['access_token'] = 'existing-token';
      secureStore['refresh_token'] = 'existing-refresh';

      final notifier = container.read(authStateProvider.notifier);
      await notifier.logout();

      expect(secureStore.containsKey('access_token'), isFalse);
      expect(secureStore.containsKey('refresh_token'), isFalse);
      expect(container.read(authStateProvider).isAuthenticated, isFalse);
    });
  });
}
