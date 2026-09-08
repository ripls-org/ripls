import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/audio/audio_session_manager.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const channel = MethodChannel(AudioSessionManager.channelName);
  final calls = <String>[];

  // Installs a mock handler on the manager's channel that records the method
  // names invoked and optionally throws to exercise the non-fatal path.
  void installHandler({bool throwError = false}) {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      if (throwError) {
        throw PlatformException(code: 'ERR', message: 'boom');
      }
      return null;
    });
  }

  setUp(calls.clear);

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
    debugDefaultTargetPlatformOverride = null;
  });

  group('AudioSessionManager on mobile', () {
    test('configureForMutedPlayback invokes configureMuted', () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
      installHandler();

      await AudioSessionManager().configureForMutedPlayback();

      expect(calls, ['configureMuted']);
    });

    test('configureForAudiblePlayback invokes configureAudible', () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.android;
      installHandler();

      await AudioSessionManager().configureForAudiblePlayback();

      expect(calls, ['configureAudible']);
    });

    test('platform errors are swallowed — the call is non-fatal', () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
      installHandler(throwError: true);

      // Must not throw even though the platform handler raised.
      await AudioSessionManager().configureForMutedPlayback();

      expect(calls, ['configureMuted']);
    });
  });

  group('AudioSessionManager off mobile (deliberate no-op)', () {
    test('does not touch the channel on desktop platforms', () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
      installHandler();

      await AudioSessionManager().configureForMutedPlayback();
      await AudioSessionManager().configureForAudiblePlayback();

      // The web guard (`kIsWeb`) short-circuits identically; it can't be
      // exercised under the VM test runner, so the desktop guard stands in
      // for the "no audio-session API available" path.
      expect(calls, isEmpty);
    });
  });
}
