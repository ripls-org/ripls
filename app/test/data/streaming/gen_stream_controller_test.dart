import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/streaming/gen_stream_controller.dart';

void main() {
  group('GenStreamController', () {
    test('forwards events and transitions to completed on stream close',
        () async {
      final received = <int>[];
      var doneCalled = false;
      final controller = GenStreamController<int>(
        onEvent: received.add,
        onDone: () => doneCalled = true,
      );

      final source = StreamController<int>();
      controller.start(source.stream);
      expect(controller.status, GenStreamStatus.streaming);
      expect(controller.isActive, isTrue);

      source.add(1);
      source.add(2);
      source.add(3);
      await source.close();
      // Allow the stream subscription to drain.
      await Future<void>.delayed(Duration.zero);

      expect(received, [1, 2, 3]);
      expect(doneCalled, isTrue);
      expect(controller.status, GenStreamStatus.completed);
      expect(controller.isActive, isFalse);
    });

    test('transitions to errored on stream error and stops forwarding events',
        () async {
      final received = <int>[];
      Object? capturedError;
      final controller = GenStreamController<int>(
        onEvent: received.add,
        onError: (err, _) => capturedError = err,
      );

      final source = StreamController<int>();
      controller.start(source.stream);

      source.add(1);
      source.addError(Exception('boom'));
      // With cancelOnError the subscription is gone — further adds are ignored.
      source.add(2);
      await source.close();
      await Future<void>.delayed(Duration.zero);

      expect(received, [1]);
      expect(capturedError, isA<Exception>());
      expect(controller.status, GenStreamStatus.errored);
    });

    test('dispose mid-stream cancels and silences later events', () async {
      final received = <int>[];
      var doneCalled = false;
      final controller = GenStreamController<int>(
        onEvent: received.add,
        onDone: () => doneCalled = true,
      );

      final source = StreamController<int>();
      controller.start(source.stream);
      source.add(1);
      await Future<void>.delayed(Duration.zero);
      expect(received, [1]);

      await controller.dispose();
      source.add(2);
      await source.close();
      await Future<void>.delayed(Duration.zero);

      expect(received, [1], reason: 'post-dispose events must be dropped');
      expect(doneCalled, isFalse,
          reason: 'onDone must not fire after dispose');
      expect(controller.isDisposed, isTrue);
    });

    test('dispose is idempotent', () async {
      final controller = GenStreamController<int>();
      await controller.dispose();
      await controller.dispose();
      expect(controller.isDisposed, isTrue);
    });

    test('cancel stops forwarding but leaves controller usable-once', () async {
      final received = <int>[];
      final controller = GenStreamController<int>(onEvent: received.add);

      final source = StreamController<int>();
      controller.start(source.stream);
      source.add(1);
      await Future<void>.delayed(Duration.zero);

      await controller.cancel();
      source.add(2);
      await source.close();
      await Future<void>.delayed(Duration.zero);

      expect(received, [1]);
      expect(controller.status, GenStreamStatus.cancelled);
      expect(controller.isDisposed, isFalse);
    });

    test('start after dispose throws', () async {
      final controller = GenStreamController<int>();
      await controller.dispose();
      expect(
        () => controller.start(const Stream<int>.empty()),
        throwsStateError,
      );
    });

    test('start twice throws (single-use)', () async {
      final controller = GenStreamController<int>();
      final source = StreamController<int>();
      controller.start(source.stream);
      expect(
        () => controller.start(const Stream<int>.empty()),
        throwsStateError,
      );
      await source.close();
    });
  });
}
