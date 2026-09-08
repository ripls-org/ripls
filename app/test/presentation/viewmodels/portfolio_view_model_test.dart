import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/portfolio_view_model.dart';

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
  });

  tearDown(() {
    container.dispose();
  });

  group('initial state', () {
    test('isNavVisible is true by default', () {
      final state = container.read(portfolioViewModelProvider);
      expect(state.isNavVisible, isTrue);
    });
  });

  group('showNav', () {
    test('sets isNavVisible = true when hidden', () {
      final notifier = container.read(portfolioViewModelProvider.notifier);
      notifier.hideNav();
      notifier.showNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isTrue);
    });

    test('no-op when already visible', () {
      final notifier = container.read(portfolioViewModelProvider.notifier);
      // Already visible
      notifier.showNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isTrue);
    });
  });

  group('hideNav', () {
    test('sets isNavVisible = false', () {
      final notifier = container.read(portfolioViewModelProvider.notifier);
      notifier.hideNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isFalse);
    });

    test('no-op when already hidden', () {
      final notifier = container.read(portfolioViewModelProvider.notifier);
      notifier.hideNav();
      notifier.hideNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isFalse);
    });
  });

  group('toggleNav', () {
    test('toggleNav round-trip: true → false → true', () {
      final notifier = container.read(portfolioViewModelProvider.notifier);
      expect(container.read(portfolioViewModelProvider).isNavVisible, isTrue);

      notifier.toggleNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isFalse);

      notifier.toggleNav();
      expect(container.read(portfolioViewModelProvider).isNavVisible, isTrue);
    });
  });
}
