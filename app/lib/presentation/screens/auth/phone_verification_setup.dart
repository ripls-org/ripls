import 'package:firebase_auth/firebase_auth.dart' as fb;
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:logging/logging.dart';
import 'package:ripls/core/config/environment.dart';

/// Platform and provider setup that must happen before the first phone
/// verification call.
///
/// Split out of `phone_auth_screen.dart` because none of it is about the
/// screen: it is per-platform plumbing for the identity provider, with no
/// widget state, and it kept the screen over the 1,000-line gate.
final _log = Logger('PhoneVerificationSetup');

/// Registers an APNs token so the provider can verify by silent push instead
/// of falling back to reCAPTCHA.
///
/// On real iOS devices this is what makes verification invisible. Silent
/// notifications need no user permission (iOS 8.0+), so nothing is prompted.
/// Simulators have no APNs, so the provider falls back to reCAPTCHA. On
/// Flutter Web there is no APNs at all — `getAPNSToken()` is an iOS-only
/// plugin method that throws there — and the provider always uses the
/// reCAPTCHA verifier, so this is skipped entirely.
///
/// Never throws: a missing token only costs the reCAPTCHA fallback, which
/// still verifies.
Future<void> ensureAPNsToken() async {
  if (kIsWeb) {
    _log.info('skipping APNs token on web (reCAPTCHA verifier)');
    return;
  }
  try {
    final token = await FirebaseMessaging.instance.getAPNSToken();
    _log.info('APNs token ${token != null ? "obtained" : "unavailable"}');
  } catch (e) {
    _log.warning('APNs token unavailable (reCAPTCHA fallback): $e');
  }
}

/// Points Firebase Auth at the local emulator and disables app verification,
/// so the deterministic test code is issued without a reCAPTCHA challenge.
///
/// Returns whether the emulator is now wired, so the caller can avoid calling
/// `useAuthEmulator` twice — it must run before the first auth operation and
/// at most once, and resending a code re-enters this path.
///
/// Wired here, immediately before the first auth call on the only screen that
/// touches Firebase Auth, rather than at app startup: `useAuthEmulator` does a
/// network round-trip, and paying it on every page load slowed the e2e suite
/// enough to time out the multi-client spec. Gated on the emulator host, which
/// is empty in real builds, so this is inert in production.
Future<bool> wireAuthEmulatorIfNeeded(
  fb.FirebaseAuth auth, {
  required bool alreadyWired,
}) async {
  if (!kIsWeb || Environment.authEmulatorHost.isEmpty) return alreadyWired;

  await auth.setSettings(appVerificationDisabledForTesting: true);
  if (alreadyWired) return true;

  final hostPort = Environment.authEmulatorHost.split(':');
  final host = hostPort.first;
  final port = int.tryParse(hostPort.length > 1 ? hostPort[1] : '') ?? 9099;
  await auth.useAuthEmulator(host, port);
  return true;
}
