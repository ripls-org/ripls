// Web-only Firebase options. Mobile uses google-services.json
// (Android) and GoogleService-Info.plist (iOS), which the native
// Firebase SDK auto-loads — `Firebase.initializeApp()` with no args
// works there. Web has no equivalent auto-detection; we must pass
// explicit `FirebaseOptions` to `Firebase.initializeApp(...)`.
//
// Every field arrives as a build-time dart-define rather than being
// compiled in. The identifiers are public by design — they name a project,
// not a user — but they name *which* project, so hardcoding them meant this
// file only ever worked for one deployment (#2953). They now come from the
// same `--dart-define-from-file=env.<environment>.json` that already carries
// GOOGLE_CLIENT_ID, so switching deployments is a matter of which env file
// the build passes, not a code change.
//
// The `apiKey` is the exception in provenance, not in mechanism: it is
// fetched from Secret Manager and injected as its own `--dart-define` by the
// `build:web:*` npm scripts, so it can be rotated without editing an env
// file. Firebase documents the web apiKey as safe to embed in client code;
// keeping it in Secret Manager just keeps the posture consistent with the
// other vendor tokens.
//
// Values come from the Firebase Console (Project settings → Your apps → Web).

import 'package:firebase_core/firebase_core.dart';

/// Build-time Firebase web configuration, read from dart-defines.
///
/// Empty means "not supplied by this build" for every field.
const _apiKey = String.fromEnvironment('FIREBASE_WEB_API_KEY');
const _authDomain = String.fromEnvironment('FIREBASE_AUTH_DOMAIN');
const _projectId = String.fromEnvironment('FIREBASE_PROJECT_ID');
const _storageBucket = String.fromEnvironment('FIREBASE_STORAGE_BUCKET');
const _messagingSenderId = String.fromEnvironment('FIREBASE_MESSAGING_SENDER_ID');
const _appId = String.fromEnvironment('FIREBASE_APP_ID');

/// Optional: only set for deployments that enable Analytics.
const _measurementId = String.fromEnvironment('FIREBASE_MEASUREMENT_ID');

/// The fields `Firebase.initializeApp` cannot start without, paired with the
/// dart-define that supplies each. Used to build one actionable error rather
/// than letting the SDK fail on whichever it happens to read first.
Map<String, String> get _requiredFields => const {
      'FIREBASE_WEB_API_KEY': _apiKey,
      'FIREBASE_AUTH_DOMAIN': _authDomain,
      'FIREBASE_PROJECT_ID': _projectId,
      'FIREBASE_STORAGE_BUCKET': _storageBucket,
      'FIREBASE_MESSAGING_SENDER_ID': _messagingSenderId,
      'FIREBASE_APP_ID': _appId,
    };

/// Names of the required dart-defines this build did not supply.
///
/// Exposed for testing: the failure it guards against — a web bundle built
/// without its Firebase config — only shows up at runtime in a browser, where
/// the SDK's own error names one missing field at a time.
List<String> missingFirebaseWebDefines() => _requiredFields.entries
    .where((e) => e.value.isEmpty)
    .map((e) => e.key)
    .toList(growable: false);

/// Returns the Firebase web config supplied by this build's dart-defines.
///
/// Callers should gate on `kIsWeb` first; on mobile, Firebase auto-detects via
/// google-services.json / GoogleService-Info.plist and these options aren't
/// needed.
///
/// Throws [StateError] when the build did not supply the configuration.
/// Failing here is deliberate: the alternative is a bundle that boots and then
/// fails every auth and messaging call in the browser, which is far harder to
/// trace back to a missing build flag.
FirebaseOptions webFirebaseOptions() {
  final missing = missingFirebaseWebDefines();
  if (missing.isNotEmpty) {
    throw StateError(
      'Firebase web configuration is missing: ${missing.join(', ')}. '
      'Pass them with --dart-define-from-file=env.<environment>.json '
      '(FIREBASE_WEB_API_KEY comes from Secret Manager via the build:web:* '
      'npm scripts).',
    );
  }

  return FirebaseOptions(
    apiKey: _apiKey,
    authDomain: _authDomain,
    projectId: _projectId,
    storageBucket: _storageBucket,
    messagingSenderId: _messagingSenderId,
    appId: _appId,
    measurementId: _measurementId.isEmpty ? null : _measurementId,
  );
}
