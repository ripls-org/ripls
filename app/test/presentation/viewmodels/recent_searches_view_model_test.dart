import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/recent_searches_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Test notifier that surfaces a user without running the real
/// auth-state lifecycle (secure storage, token refresh, etc.).
class _TestAuthStateNotifier extends AuthStateNotifier {
  _TestAuthStateNotifier(this._initial);
  final AuthStateData _initial;

  @override
  AuthStateData build() => _initial;
}

AuthStateData _withUser(String id) {
  return AuthStateData(
    accessToken: 'token',
    user: User()..id = id,
    isLoading: false,
  );
}

String _prefsKey(String userId) => 'recent_searches_$userId';

ProviderContainer _container({String? userId = 'user-1'}) {
  final authState = userId == null ? AuthStateData.empty : _withUser(userId);
  return ProviderContainer(
    overrides: [
      authStateProvider.overrideWith(() => _TestAuthStateNotifier(authState)),
    ],
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('initial state is empty when no key exists', () async {
    final container = _container();
    addTearDown(container.dispose);

    final initial = await container.read(recentSearchesProvider.future);
    expect(initial, isEmpty);
  });

  test('add() prepends and persists to SharedPreferences', () async {
    final container = _container();
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    await container.read(recentSearchesProvider.notifier).add('drill');

    final state = container.read(recentSearchesProvider).value;
    expect(state, ['drill']);

    final prefs = await SharedPreferences.getInstance();
    expect(
      jsonDecode(prefs.getString(_prefsKey('user-1'))!),
      ['drill'],
    );
  });

  test('add() dedupes by moving an existing term to the front', () async {
    final container = _container();
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    final notifier = container.read(recentSearchesProvider.notifier);
    await notifier.add('drill');
    await notifier.add('ladder');
    await notifier.add('hammer');
    await notifier.add('drill'); // re-add an existing term

    expect(container.read(recentSearchesProvider).value,
        ['drill', 'hammer', 'ladder']);
  });

  test('add() trims to recentSearchesMax entries (FIFO)', () async {
    final container = _container();
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    final notifier = container.read(recentSearchesProvider.notifier);
    for (var i = 0; i < recentSearchesMax + 5; i++) {
      await notifier.add('term-$i');
    }

    final list = container.read(recentSearchesProvider).value!;
    expect(list.length, recentSearchesMax);
    // The most recent insertion sits at the head; the oldest entries
    // (term-0..4) drop off after the cap.
    expect(list.first, 'term-${recentSearchesMax + 5 - 1}');
    expect(list.contains('term-0'), isFalse);
  });

  test('add() ignores empty and whitespace-only queries', () async {
    final container = _container();
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    final notifier = container.read(recentSearchesProvider.notifier);
    await notifier.add('');
    await notifier.add('   ');

    expect(container.read(recentSearchesProvider).value, isEmpty);
  });

  test('clear() wipes the prefs key and resets state', () async {
    final container = _container();
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    final notifier = container.read(recentSearchesProvider.notifier);
    await notifier.add('drill');
    await notifier.add('ladder');

    await notifier.clear();
    expect(container.read(recentSearchesProvider).value, isEmpty);

    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString(_prefsKey('user-1')), isNull);
  });

  test('add() is a no-op when no user is loaded', () async {
    final container = _container(userId: null);
    addTearDown(container.dispose);

    await container.read(recentSearchesProvider.future);
    await container.read(recentSearchesProvider.notifier).add('drill');

    expect(container.read(recentSearchesProvider).value, isEmpty);
  });

  test('build() restores from a persisted JSON list', () async {
    SharedPreferences.setMockInitialValues({
      _prefsKey('user-1'): jsonEncode(['ladder', 'drill']),
    });
    final container = _container();
    addTearDown(container.dispose);

    final list = await container.read(recentSearchesProvider.future);
    expect(list, ['ladder', 'drill']);
  });

  test('build() returns empty on corrupt JSON', () async {
    SharedPreferences.setMockInitialValues({
      _prefsKey('user-1'): '<<not-json>>',
    });
    final container = _container();
    addTearDown(container.dispose);

    final list = await container.read(recentSearchesProvider.future);
    expect(list, isEmpty);
  });
}
