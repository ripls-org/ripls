import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/tab_search_scope_provider.dart';

void main() {
  group('TabSearchScopeNotifier', () {
    test('defaults to no scope', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      expect(container.read(plansSearchScopeProvider), isNull);
      expect(container.read(peopleSearchScopeProvider), isNull);
    });

    test('set trims the query; blank queries clear', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final notifier = container.read(plansSearchScopeProvider.notifier);

      notifier.set('  drill  ');
      expect(container.read(plansSearchScopeProvider), 'drill');

      notifier.set('   ');
      expect(container.read(plansSearchScopeProvider), isNull);

      notifier.set(null);
      expect(container.read(plansSearchScopeProvider), isNull);
    });

    test('clear cancels an active scope', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      container.read(peopleSearchScopeProvider.notifier).set('betty');
      expect(container.read(peopleSearchScopeProvider), 'betty');

      container.read(peopleSearchScopeProvider.notifier).clear();
      expect(container.read(peopleSearchScopeProvider), isNull);
    });

    test('Plans and People scopes are independent', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      container.read(plansSearchScopeProvider.notifier).set('hike');
      expect(container.read(peopleSearchScopeProvider), isNull);
      expect(container.read(plansSearchScopeProvider), 'hike');
    });
  });
}
