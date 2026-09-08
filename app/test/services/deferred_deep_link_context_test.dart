import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/services/deferred_deep_link_service.dart';

void main() {
  group('DeferredDeepLinkContextHolder', () {
    setUp(() {
      DeferredDeepLinkContextHolder.value = null;
    });

    test('initial value is null', () {
      expect(DeferredDeepLinkContextHolder.value, isNull);
    });

    test('set stores the context', () {
      const deferred = DeferredDeepLinkContext(shortCode: 'ABC123');
      DeferredDeepLinkContextHolder.value = deferred;

      expect(DeferredDeepLinkContextHolder.value, isNotNull);
      expect(DeferredDeepLinkContextHolder.value!.shortCode, equals('ABC123'));
    });

    test('consume returns current context and resets to null', () {
      const deferred = DeferredDeepLinkContext(
        shortCode: 'XYZ',
      );
      DeferredDeepLinkContextHolder.value = deferred;

      final consumed = DeferredDeepLinkContextHolder.consume();
      expect(consumed, isNotNull);
      expect(consumed!.shortCode, equals('XYZ'));

      // After consuming, value should be null.
      expect(DeferredDeepLinkContextHolder.value, isNull);
    });

    test('consume returns null when no context is set', () {
      final consumed = DeferredDeepLinkContextHolder.consume();
      expect(consumed, isNull);
    });

    test('setting null clears the context', () {
      const deferred = DeferredDeepLinkContext(shortCode: 'ABC');
      DeferredDeepLinkContextHolder.value = deferred;
      expect(DeferredDeepLinkContextHolder.value, isNotNull);

      DeferredDeepLinkContextHolder.value = null;
      expect(DeferredDeepLinkContextHolder.value, isNull);
    });
  });

  group('FirstLaunchFlag', () {
    setUp(() {
      FirstLaunchFlag.value = false;
    });

    test('default is false', () {
      expect(FirstLaunchFlag.value, isFalse);
    });

    test('can be set to true and back', () {
      FirstLaunchFlag.value = true;
      expect(FirstLaunchFlag.value, isTrue);
      FirstLaunchFlag.value = false;
      expect(FirstLaunchFlag.value, isFalse);
    });
  });
}
