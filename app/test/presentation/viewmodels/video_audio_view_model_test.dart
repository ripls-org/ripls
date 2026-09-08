import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';

void main() {
  group('VideoUnmutedNotifier', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('defaults to muted (false) on first build', () {
      expect(container.read(videoUnmutedProvider), isFalse);
    });

    test('set(true) flips the flag and is observable', () {
      container.read(videoUnmutedProvider.notifier).set(true);
      expect(container.read(videoUnmutedProvider), isTrue);
    });

    test('toggle flips the flag in both directions', () {
      final notifier = container.read(videoUnmutedProvider.notifier);

      notifier.toggle();
      expect(container.read(videoUnmutedProvider), isTrue);

      notifier.toggle();
      expect(container.read(videoUnmutedProvider), isFalse);
    });

    test('listener fires with previous and next when state changes', () {
      final changes = <(bool?, bool)>[];
      container.listen<bool>(
        videoUnmutedProvider,
        (previous, next) => changes.add((previous, next)),
      );

      container.read(videoUnmutedProvider.notifier).set(true);
      container.read(videoUnmutedProvider.notifier).set(false);

      expect(changes, [(false, true), (true, false)]);
    });
  });
}
