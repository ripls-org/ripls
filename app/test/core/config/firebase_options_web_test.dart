import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/config/firebase_options_web.dart';

/// These run with no dart-defines, which is exactly the misconfiguration the
/// guard exists for: a web bundle built without `--dart-define-from-file`
/// used to compile fine and then fail every auth and messaging call in the
/// browser, with the SDK naming one missing field at a time (#2953).
void main() {
  group('firebase web options', () {
    test('names every required define a build did not supply', () {
      expect(
        missingFirebaseWebDefines(),
        containsAll([
          'FIREBASE_WEB_API_KEY',
          'FIREBASE_AUTH_DOMAIN',
          'FIREBASE_PROJECT_ID',
          'FIREBASE_STORAGE_BUCKET',
          'FIREBASE_MESSAGING_SENDER_ID',
          'FIREBASE_APP_ID',
        ]),
      );
    });

    test('does not require the optional analytics id', () {
      expect(missingFirebaseWebDefines(), isNot(contains('FIREBASE_MEASUREMENT_ID')));
    });

    test('throws naming the missing defines rather than returning a half-built config', () {
      expect(
        webFirebaseOptions,
        throwsA(
          isA<StateError>().having(
            (e) => e.message,
            'message',
            allOf(
              contains('FIREBASE_PROJECT_ID'),
              // The message has to say how to fix it, not just what is wrong.
              contains('--dart-define-from-file'),
            ),
          ),
        ),
      );
    });
  });
}
