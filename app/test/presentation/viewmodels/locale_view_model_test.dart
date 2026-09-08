import 'dart:ui';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' as user_api;
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/locale_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/user_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  setUp(() {
    // Reset SharedPreferences to empty state for each test
    SharedPreferences.setMockInitialValues({});
  });

  group('initial state', () {
    test('returns null (system locale) when no preference stored', () async {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      final locale =
          await container.read(localePreferenceProvider.future);
      expect(locale, isNull);
    });

    test('returns stored locale when preference exists', () async {
      SharedPreferences.setMockInitialValues({'app_locale_language_code': 'es'});

      final container = ProviderContainer();
      addTearDown(container.dispose);

      final locale =
          await container.read(localePreferenceProvider.future);
      expect(locale, const Locale('es'));
    });
  });

  group('setLocale', () {
    test('updates state to the new locale', () async {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future); // wait for build
      final notifier =
          container.read(localePreferenceProvider.notifier);
      await notifier.setLocale(const Locale('fr'));

      final state = container.read(localePreferenceProvider).value;
      expect(state, const Locale('fr'));
    });

    test('sets to null when passing null (reset to system locale)', () async {
      SharedPreferences.setMockInitialValues({'app_locale_language_code': 'es'});

      final container = ProviderContainer();
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      final notifier =
          container.read(localePreferenceProvider.notifier);
      await notifier.setLocale(null);

      final state = container.read(localePreferenceProvider).value;
      expect(state, isNull);
    });

    test('persists locale to SharedPreferences', () async {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      final notifier =
          container.read(localePreferenceProvider.notifier);
      await notifier.setLocale(const Locale('de'));

      // Verify persistence by reading prefs directly
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getString('app_locale_language_code'), 'de');
    });

    test('removes key from SharedPreferences when setting null', () async {
      SharedPreferences.setMockInitialValues({'app_locale_language_code': 'es'});
      final container = ProviderContainer();
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      await container.read(localePreferenceProvider.notifier).setLocale(null);

      final prefs = await SharedPreferences.getInstance();
      expect(prefs.containsKey('app_locale_language_code'), isFalse);
    });
  });

  group('server-side persistence', () {
    test('persists language to the server when a user is signed in',
        () async {
      final captured = <_UpdateCall>[];
      final container = _containerWithMockedAuthAndRepo(
        userId: 'user-123',
        onUpdate: (id, lang) {
          captured.add(_UpdateCall(userId: id, language: lang));
        },
      );
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      await container
          .read(localePreferenceProvider.notifier)
          .setLocale(const Locale('es'));

      // _persistToServer is fire-and-forget, so let pending microtasks
      // resolve before asserting.
      await Future<void>.delayed(Duration.zero);

      expect(captured, hasLength(1));
      expect(captured.first.userId, 'user-123');
      expect(captured.first.language, 'es');
    });

    test('clears server-side language when locale is reset to null',
        () async {
      final captured = <_UpdateCall>[];
      final container = _containerWithMockedAuthAndRepo(
        userId: 'user-123',
        onUpdate: (id, lang) {
          captured.add(_UpdateCall(userId: id, language: lang));
        },
      );
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      await container
          .read(localePreferenceProvider.notifier)
          .setLocale(null);

      await Future<void>.delayed(Duration.zero);

      expect(captured, hasLength(1));
      expect(captured.first.language, isEmpty);
    });

    test('no-ops when no user is signed in', () async {
      final captured = <_UpdateCall>[];
      final container = _containerWithMockedAuthAndRepo(
        userId: null,
        onUpdate: (id, lang) {
          captured.add(_UpdateCall(userId: id, language: lang));
        },
      );
      addTearDown(container.dispose);

      await container.read(localePreferenceProvider.future);
      await container
          .read(localePreferenceProvider.notifier)
          .setLocale(const Locale('es'));

      await Future<void>.delayed(Duration.zero);

      expect(captured, isEmpty);
    });
  });

  group('disposal safety', () {
    // Per docs/client/testing.md § "Disposal Testing": notifiers
    // that schedule async work on user-driven events must survive
    // being disposed mid-flight without crashing.
    test('does not throw when container is disposed mid-setLocale',
        () async {
      final container = _containerWithMockedAuthAndRepo(
        userId: 'user-disposed',
        onUpdate: (_, _) {},
      );

      await container.read(localePreferenceProvider.future);
      final notifier =
          container.read(localePreferenceProvider.notifier);

      // Kick off setLocale, then dispose before the async chain
      // resolves (the SharedPreferences write is on a background
      // isolate, and the state assignment + fire-and-forget repo
      // call must tolerate disposal between those points).
      final pending = notifier.setLocale(const Locale('es'));
      container.dispose();

      await expectLater(pending, completes);
    });

    test(
      'does not throw when container is disposed before _persistToServer runs',
      () async {
        final container = _containerWithMockedAuthAndRepo(
          userId: 'user-disposed-2',
          onUpdate: (_, _) {
            throw StateError(
                'updatePreferredLanguage must not run after disposal');
          },
        );

        await container.read(localePreferenceProvider.future);
        // Dispose first, then attempt setLocale via the cached
        // notifier reference. The implementation already returns
        // early when ref.mounted is false, so this should be a
        // silent no-op.
        final notifier =
            container.read(localePreferenceProvider.notifier);
        container.dispose();

        await expectLater(notifier.setLocale(const Locale('es')), completes);
      },
    );
  });
}

class _UpdateCall {
  final String userId;
  final String language;
  _UpdateCall({required this.userId, required this.language});
}

class _FakeUserRepository implements UserRepository {
  _FakeUserRepository(this._onUpdate);

  final void Function(String userId, String language) _onUpdate;

  @override
  Future<void> updatePreferredLanguage(
    String userId,
    String? language,
  ) async {
    _onUpdate(userId, language ?? '');
  }

  @override
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
    if (preferredLanguage != null) {
      _onUpdate(userId, preferredLanguage);
    }
  }

  // Below: the test only exercises updatePreferredLanguage. The
  // remaining UserRepository surface is unused by LocalePreferenceNotifier
  // and we throw if any of it gets called so test wiring stays honest.
  @override
  dynamic noSuchMethod(Invocation invocation) {
    throw UnimplementedError(
      'UserRepository.${invocation.memberName} not stubbed in locale test',
    );
  }
}

ProviderContainer _containerWithMockedAuthAndRepo({
  required String? userId,
  required void Function(String userId, String language) onUpdate,
}) {
  final user = userId == null
      ? null
      : user_api.User(id: userId, name: 'Test User');
  final authData = AuthStateData(
    accessToken: userId == null ? null : 'fake-token',
    user: user,
    isLoading: false,
  );

  return ProviderContainer(
    overrides: [
      // overrideWithValue on userRepositoryProvider short-circuits
      // the production dependency graph (cache manager, user
      // service, media repository), so the test does not need to
      // stand up those collaborators.
      authStateProvider.overrideWith(() => _FixedAuthNotifier(authData)),
      userRepositoryProvider.overrideWithValue(
        _FakeUserRepository(onUpdate),
      ),
    ],
  );
}

class _FixedAuthNotifier extends AuthStateNotifier {
  _FixedAuthNotifier(this._value);
  final AuthStateData _value;
  @override
  AuthStateData build() => _value;
}
