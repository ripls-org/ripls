//
// Set the environment variables for the app
//
// Environment variables are passed at build/run time using --dart-define-from-file
// Example: flutter run --dart-define-from-file=env.dev.json
//

import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';
import 'package:logging/logging.dart';

class Environment {
  /// Base URL of the API server this build talks to.
  ///
  /// Comes from the `SERVER_URL` dart-define — set per environment in
  /// `env.<environment>.json`, or passed directly for real-device local testing
  /// (`--dart-define=SERVER_URL=http://192.168.1.72:8080`). It used to be a
  /// hardcoded host per environment, which meant any build of this code talked
  /// to one particular deployment's servers (#2953).
  ///
  /// `ENVIRONMENT=local` is the exception and stays derived: the right answer
  /// differs by platform and by *how* the bundle is being served, so it cannot
  /// be written down ahead of time.
  static String get getServer {
    const serverOverride = String.fromEnvironment('SERVER_URL');
    if (serverOverride.isNotEmpty) return serverOverride;

    const env = String.fromEnvironment('ENVIRONMENT', defaultValue: 'dev');

    switch (env) {
      case 'local':
        // On web, derive the server URL from the page's origin
        // (Uri.base on Flutter Web returns the document URL). This
        // makes the bundle work for any host that serves it:
        // `localhost:8080` on a desktop browser, `10.0.2.2:8080`
        // when Chrome inside the Android emulator hits the host,
        // a LAN IP for real-device testing, etc. Hardcoding
        // `localhost` here breaks the emulator-Chrome case because
        // `localhost` inside the emulator resolves to the emulator
        // itself, not the host.
        if (kIsWeb) {
          return Uri.base.origin;
        }
        // Android emulator needs special IP to reach host machine
        if (Platform.isAndroid) {
          return 'http://10.0.2.2:8080';
        }
        return 'http://localhost:8080';
      default:
        // dev, prod, or anything else: SERVER_URL is the only source. Empty
        // means the build did not supply it — every RPC will fail, which is
        // the loud failure we want rather than quietly reaching some other
        // deployment's server.
        return '';
    }
  }

  static String get getAppName {
    return 'Ripls';
  }

  /// Custom URL scheme this build registers for deep links (`<scheme>://…`).
  ///
  /// Two installs cannot claim the same scheme on one device, so a fork
  /// shipping to users overrides it — and must change the matching
  /// `AndroidManifest.xml` intent-filter and iOS `CFBundleURLSchemes` entry,
  /// which are native config the Dart layer cannot read (#2953).
  static String get deepLinkScheme {
    return const String.fromEnvironment('DEEP_LINK_SCHEME',
        defaultValue: 'ripls');
  }

  /// Absolute URL of this deployment's terms of service, linked from the
  /// SMS-consent disclosure. Empty hides the link rather than rendering a dead
  /// one — the consent copy still stands on its own.
  static String get termsUrl {
    return const String.fromEnvironment('TERMS_URL', defaultValue: '');
  }

  /// Absolute URL of this deployment's privacy policy. Empty hides the link.
  static String get privacyUrl {
    return const String.fromEnvironment('PRIVACY_URL', defaultValue: '');
  }

  /// Address users are directed to for help. Empty hides the contact
  /// affordance rather than offering an address nobody reads.
  static String get supportEmail {
    return const String.fromEnvironment('SUPPORT_EMAIL', defaultValue: '');
  }

  static String get mapboxAccessToken {
    return const String.fromEnvironment('MAPBOX_ACCESS_TOKEN', defaultValue: '');
  }

  /// When set (e.g. "127.0.0.1:9099"), Firebase Auth is routed through a local
  /// Auth Emulator instead of the real project. Set ONLY by the e2e web build
  /// (`--dart-define=AUTH_EMULATOR_HOST=…`) so Playwright can drive deterministic
  /// phone OTP with no reCAPTCHA/SMS; empty in every real build. See
  /// `docs/issues/2492-web2-pivot-e2e.md`.
  static String get authEmulatorHost {
    return const String.fromEnvironment('AUTH_EMULATOR_HOST', defaultValue: '');
  }

  /// Native map-widget SDK key (#2188). Injected per-platform and passed
  /// to the platform Maps SDK: Android manifest `geo.API_KEY`, iOS
  /// `GMSServices.provideAPIKey` (via the `google_maps_init` channel in
  /// main.dart), web JS loader. Application-restricted (package/cert,
  /// bundle id, HTTP referrer) per platform. Do **not** use this for the
  /// Places/Geocoding REST client — that's [googlePlacesApiKey].
  static String get googleMapsApiKey {
    return const String.fromEnvironment('GOOGLE_MAPS_API_KEY', defaultValue: '');
  }

  /// Client-side Places API (New) + Geocoding API key used by the
  /// GoogleLocationSearchService REST calls (#2246). A single key across
  /// all platforms, API-restricted to those two services and **not**
  /// application-restricted — so no per-platform request headers are
  /// needed. Distinct from [googleMapsApiKey] (the per-platform map-SDK
  /// keys, which carry application restrictions that don't validate on the
  /// REST path). Sourced from the `google-maps-api-key-client-places` GSM
  /// secret and injected as the GOOGLE_MAPS_PLACES_KEY --dart-define.
  static String get googlePlacesApiKey {
    return const String.fromEnvironment(
      'GOOGLE_MAPS_PLACES_KEY',
      defaultValue: '',
    );
  }

  /// Active location provider for geocoding + place search (#2188).
  /// Either 'mapbox' (default) or 'google'. The corresponding credential
  /// must be populated; an empty credential plus the matching provider
  /// is a configuration error that surfaces at first call.
  static String get mapProvider {
    return const String.fromEnvironment('MAP_PROVIDER', defaultValue: 'mapbox');
  }

  static bool get notificationsEnabled {
    const value = String.fromEnvironment('ENABLE_NOTIFICATIONS', defaultValue: 'false');
    return value.toLowerCase() == 'true';
  }

  /// Whether the unified-create flow is enabled. Build-time only; flip
  /// the value in `app/env.*.json` and rebuild to change. See
  /// `docs/design/unified-create.md` § Feature flag.
  static bool get unifiedCreateEnabled {
    const value = String.fromEnvironment('ENABLE_UNIFIED_CREATE', defaultValue: 'false');
    return value.toLowerCase() == 'true';
  }

  /// Whether the converged bottom dock (#2634) is enabled: the five-target
  /// glass capsule (Home · Plans · Create · Library · People) plus the
  /// detached search orb. On by default; set ENABLE_NAV_DOCK=false to fall
  /// back to the legacy three-tab pill + floating Create FAB (#1895).
  static bool get navDockEnabled {
    const value =
        String.fromEnvironment('ENABLE_NAV_DOCK', defaultValue: 'true');
    return value.toLowerCase() == 'true';
  }

  static String get googleClientId {
    return const String.fromEnvironment('GOOGLE_CLIENT_ID', defaultValue: '');
  }

  static Duration get cacheTtl {
    const minutes = int.fromEnvironment('CACHE_TTL_MINUTES', defaultValue: 30);
    return Duration(minutes: minutes);
  }

  /// configureLogging sets up the logging system for the app.
  /// Call this once at app startup.
  static void configureLogging() {
    const logLevelStr = String.fromEnvironment('LOG_LEVEL', defaultValue: 'INFO');

    // Find the matching log level or default to INFO
    final logLevel = Level.LEVELS.firstWhere(
      (level) => level.name == logLevelStr.toUpperCase(),
      orElse: () => Level.INFO,
    );

    Logger.root.level = logLevel;
    // Always register the listener - logs should work in all build modes.
    // In release builds, print() outputs to system logs (adb logcat / Console.app).
    Logger.root.onRecord.listen((record) {
      // ignore: avoid_print
      print(
        '${record.level.name}: ${record.time}: ${record.loggerName}: ${record.message}',
      );
    });
  }
}
