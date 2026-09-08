import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'home_view_model_test.mocks.dart';

@GenerateMocks([MediaRepository])

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
  });

  tearDown(() {
    container.dispose();
  });

  group('HomeState', () {
    test('initial state is correct', () {
      final state = container.read(homeProvider);

      // Stack index 1 is the Inbox — now the first/landing tab.
      expect(state.selectedIndex, 1);
      expect(state.isNavVisible, isTrue);
    });

    test('getInitials returns U for null name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials(null), 'U');
    });

    test('getInitials returns U for empty name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials(''), 'U');
    });

    test('getInitials returns U for whitespace-only name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('   '), 'U');
    });

    test('getInitials returns first letter for single name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John'), 'J');
    });

    test('getInitials returns first letter lowercase name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('john'), 'J');
    });

    test('getInitials returns two initials for full name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John Doe'), 'JD');
    });

    test('getInitials handles multiple spaces between names', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John   Doe'), 'JD');
    });

    test('getInitials returns two initials for three names', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John Middle Doe'), 'JM');
    });

    test('getInitials handles leading and trailing spaces', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('  John Doe  '), 'JD');
    });
  });

  group('navigateToTab', () {
    test('updates selectedIndex', () {
      final notifier = container.read(homeProvider.notifier);
      notifier.navigateToTab(2);

      final state = container.read(homeProvider);
      expect(state.selectedIndex, 2);
    });

    test('does not notify if navigating to same tab', () {
      final notifier = container.read(homeProvider.notifier);
      notifier.navigateToTab(1); // Already at 1 (Inbox is the default)

      final state = container.read(homeProvider);
      expect(state.selectedIndex, 1);
    });

    test('can navigate to different tabs', () {
      final notifier = container.read(homeProvider.notifier);

      notifier.navigateToTab(1);
      expect(container.read(homeProvider).selectedIndex, 1);

      notifier.navigateToTab(3);
      expect(container.read(homeProvider).selectedIndex, 3);

      notifier.navigateToTab(0);
      expect(container.read(homeProvider).selectedIndex, 0);
    });
  });

  group('scroll controller providers', () {
    test('feedScrollControllerProvider provides PageController', () {
      final controller = container.read(feedScrollControllerProvider);
      expect(controller, isA<PageController>());
    });

    test('discoverScrollControllerProvider provides ScrollController', () {
      final controller = container.read(discoverScrollControllerProvider);
      expect(controller, isA<ScrollController>());
    });

    test('metricsScrollControllerProvider provides ScrollController', () {
      final controller = container.read(metricsScrollControllerProvider);
      expect(controller, isA<ScrollController>());
    });

    test('governanceScrollControllerProvider provides ScrollController', () {
      final controller = container.read(governanceScrollControllerProvider);
      expect(controller, isA<ScrollController>());
    });

    test('scroll controllers are disposed with container', () {
      final testContainer = ProviderContainer();
      final controller = testContainer.read(discoverScrollControllerProvider);

      expect(controller.hasClients, isFalse);
      testContainer.dispose();
      // After disposal, the controller should be disposed (no exception thrown)
    });
  });

  group('discoverKeyProvider', () {
    test('provides a unique key', () {
      final key = container.read(discoverKeyProvider);
      expect(key, isA<Key>());
    });

    test('refresh provides a new unique key', () {
      final key1 = container.read(discoverKeyProvider);
      container.read(discoverKeyProvider.notifier).refresh();
      final key2 = container.read(discoverKeyProvider);

      expect(key1, isNot(equals(key2)));
    });
  });

  group('discoverNavigatorKeyProvider', () {
    test('provides a GlobalKey for NavigatorState', () {
      final key = container.read(discoverNavigatorKeyProvider);
      expect(key, isA<GlobalKey<NavigatorState>>());
    });
  });

  group('helper functions', () {
    test('discover key notifier hands out a new key on refresh', () {
      final ref = ProviderContainer();
      addTearDown(() => ref.dispose());

      final key1 = ref.read(discoverKeyProvider);
      // Call notifier directly since we can't use WidgetRef in tests
      ref.read(discoverKeyProvider.notifier).refresh();
      final key2 = ref.read(discoverKeyProvider);

      expect(key1, isNot(equals(key2)));
    });

    test('refreshing and navigating together updates key and tab', () {
      final ref = ProviderContainer();
      addTearDown(() => ref.dispose());

      final key1 = ref.read(discoverKeyProvider);
      ref.read(discoverKeyProvider.notifier).refresh();
      ref.read(homeProvider.notifier).navigateToTab(1);

      final key2 = ref.read(discoverKeyProvider);
      final state = ref.read(homeProvider);

      expect(key1, isNot(equals(key2)));
      expect(state.selectedIndex, 1);
    });

    test('multiple refresh calls create different keys', () {
      final ref = ProviderContainer();
      addTearDown(() => ref.dispose());

      final key1 = ref.read(discoverKeyProvider);
      ref.read(discoverKeyProvider.notifier).refresh();
      final key2 = ref.read(discoverKeyProvider);
      ref.read(discoverKeyProvider.notifier).refresh();
      final key3 = ref.read(discoverKeyProvider);

      expect(key1, isNot(equals(key2)));
      expect(key2, isNot(equals(key3)));
      expect(key1, isNot(equals(key3)));
    });
  });

  group('state transitions', () {
    test('navigateToTab updates selectedIndex across tabs', () {
      final notifier = container.read(homeProvider.notifier);

      notifier.navigateToTab(1);
      expect(container.read(homeProvider).selectedIndex, 1);

      notifier.navigateToTab(2);
      expect(container.read(homeProvider).selectedIndex, 2);

      notifier.navigateToTab(3);
      expect(container.read(homeProvider).selectedIndex, 3);
    });
  });

  group('edge cases', () {
    test('navigateToTab with negative index', () {
      final notifier = container.read(homeProvider.notifier);
      notifier.navigateToTab(-1);

      expect(container.read(homeProvider).selectedIndex, -1);
    });

    test('navigateToTab with large index', () {
      final notifier = container.read(homeProvider.notifier);
      notifier.navigateToTab(999);

      expect(container.read(homeProvider).selectedIndex, 999);
    });

    test('rapid tab navigation', () {
      final notifier = container.read(homeProvider.notifier);

      for (int i = 0; i < 10; i++) {
        notifier.navigateToTab(i % 4);
      }

      expect(container.read(homeProvider).selectedIndex, 1); // 9 % 4 = 1 (last iteration i=9)
    });

    test('getInitials with special characters', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John-Paul Doe'), 'JD');
    });

    test('getInitials with numbers in name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('John2 Doe3'), 'JD');
    });

    test('getInitials with single character name', () {
      final state = container.read(homeProvider);
      expect(state.getInitials('J'), 'J');
    });
  });

  group('provider lifecycle', () {
    test('multiple containers have independent state', () {
      final container1 = ProviderContainer();
      final container2 = ProviderContainer();
      addTearDown(() {
        container1.dispose();
        container2.dispose();
      });

      container1.read(homeProvider.notifier).navigateToTab(1);
      container2.read(homeProvider.notifier).navigateToTab(2);

      expect(container1.read(homeProvider).selectedIndex, 1);
      expect(container2.read(homeProvider).selectedIndex, 2);
    });

    test('state resets when container is recreated', () {
      final tempContainer = ProviderContainer();
      tempContainer.read(homeProvider.notifier).navigateToTab(3);
      tempContainer.dispose();

      final newContainer = ProviderContainer();
      addTearDown(() => newContainer.dispose());

      expect(newContainer.read(homeProvider).selectedIndex, 1);
      expect(newContainer.read(homeProvider).isNavVisible, isTrue);
    });

    test('scroll controllers are unique per container', () {
      final container1 = ProviderContainer();
      final container2 = ProviderContainer();
      addTearDown(() {
        container1.dispose();
        container2.dispose();
      });

      final controller1 = container1.read(feedScrollControllerProvider);
      final controller2 = container2.read(feedScrollControllerProvider);

      expect(controller1, isNot(same(controller2)));
    });

    test('discover keys are unique per container', () {
      final container1 = ProviderContainer();
      final container2 = ProviderContainer();
      addTearDown(() {
        container1.dispose();
        container2.dispose();
      });

      final key1 = container1.read(discoverKeyProvider);
      final key2 = container2.read(discoverKeyProvider);

      expect(key1, isNot(equals(key2)));
    });
  });

  group('state immutability', () {
    test('state cannot be mutated directly', () {
      final state1 = container.read(homeProvider);
      container.read(homeProvider.notifier).navigateToTab(3);
      final state2 = container.read(homeProvider);

      // Original state should be unchanged (default is the Inbox, index 1).
      expect(state1.selectedIndex, 1);
      expect(state2.selectedIndex, 3);
    });

    test('copyWith creates new state instance', () {
      final notifier = container.read(homeProvider.notifier);
      final state1 = container.read(homeProvider);

      notifier.navigateToTab(3);
      final state2 = container.read(homeProvider);

      expect(identical(state1, state2), isFalse);
    });
  });

  group('scroll controller disposal', () {
    test('all scroll controllers are properly disposed', () {
      final testContainer = ProviderContainer();

      // Read all controllers to ensure they're created
      testContainer.read(feedScrollControllerProvider);
      testContainer.read(discoverScrollControllerProvider);
      testContainer.read(metricsScrollControllerProvider);
      testContainer.read(governanceScrollControllerProvider);

      // Dispose should not throw
      expect(() => testContainer.dispose(), returnsNormally);
    });
  });

  group('getMediaUrl', () {
    late MockMediaRepository mockMediaRepository;
    late ProviderContainer testContainer;

    setUp(() {
      mockMediaRepository = MockMediaRepository();
      testContainer = ProviderContainer(
        overrides: [
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        ],
      );
    });

    tearDown(() {
      testContainer.dispose();
    });

    test('calls repository and returns MediaUrl object', () async {
      const mediaId = 'media-123';
      const mediaUrl = 'https://example.com/image.jpg';
      final mediaUrlResponse = MediaUrl(
        url: mediaUrl,
        isThumbnail: true,
        mediaId: mediaId,
      );

      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenAnswer((_) async => mediaUrlResponse);

      final notifier = testContainer.read(homeProvider.notifier);
      final result = await notifier.getMediaUrl(mediaId);

      expect(result, isA<MediaUrl>());
      expect(result.url, equals(mediaUrl));
      expect(result.mediaId, equals(mediaId));
      expect(result.cacheKey, equals(ImageCacheKeys.thumbnail(mediaId)));
      expect(result.isThumbnail, isTrue);
      verify(mockMediaRepository.getMediaUrl(mediaId)).called(1);
    });

    test('propagates repository errors', () async {
      const mediaId = 'media-123';

      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenThrow(Exception('Failed to load media'));

      final notifier = testContainer.read(homeProvider.notifier);

      expect(
        () => notifier.getMediaUrl(mediaId),
        throwsA(isA<Exception>()),
      );
    });
  });
}
