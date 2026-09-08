import 'package:connectrpc/protobuf.dart';
import 'package:connectrpc/protocol/connect.dart' as protocol;
// Conditional imports for platform-specific HTTP clients
import 'package:connectrpc/web.dart'
    if (dart.library.io) 'package:connectrpc/http2.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/firebase/firebase_analytics_provider.dart';
import 'package:ripls/core/observability/firebase/firebase_crash_reporter.dart';
import 'package:ripls/core/observability/firebase/firebase_performance_monitor.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/observability/providers.dart' show NoOpCrashReporter;
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/data/repositories/auth_repository.dart';
import 'package:ripls/data/repositories/share_link_repository.dart';
import 'package:ripls/services/auth_service.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Provider for Connect RPC transport (shared across all services)
final transportProvider = Provider<protocol.Transport>((ref) {
  final serverUrl = Environment.getServer;

  return protocol.Transport(
    baseUrl: serverUrl,
    codec: const ProtoCodec(),
    httpClient: createHttpClient(),
  );
});

/// Provider for AuthState (global auth state management)
final authStateProvider = NotifierProvider<AuthStateNotifier, AuthStateData>(
  () {
    return AuthStateNotifier();
  },
);

/// Provider for ObservabilitySettings (global privacy/consent settings)
final observabilitySettingsProvider =
    NotifierProvider<ObservabilitySettingsNotifier, ObservabilitySettings>(
  () {
    return ObservabilitySettingsNotifier();
  },
);

/// Provider for ObservabilityService (crash reporting, performance, analytics)
///
/// The service coordinates all observability features based on user consent.
/// When consent is not granted, it uses no-op implementations.
///
/// Crashlytics is mobile-only — there's no web implementation, and calling
/// FirebaseCrashlytics methods on web throws (see main.dart's early-boot
/// init, which gates every call the same way). This is the *second*
/// crash-reporting entry point (the consent-driven one), so it needs the
/// same !kIsWeb gate: a real [FirebaseCrashReporter] here would call
/// setCrashlyticsCollectionEnabled as soon as the user accepts the consent
/// dialog (or an e2e test preseeds granted consent), throwing an uncaught
/// error that takes down the web page.
final observabilityServiceProvider = Provider<ObservabilityService>((ref) {
  return ObservabilityService(
    crashReporter: kIsWeb
        ? NoOpCrashReporter()
        : FirebaseCrashReporter(
            breadcrumbBuffer: LogContextHolder.breadcrumbBuffer,
          ),
    performanceMonitor: FirebasePerformanceMonitor(),
    analyticsProvider: FirebaseAnalyticsProvider(),
  );
});

/// Provider for RpcErrorHandler.
///
/// Services use this to turn ConnectRPC codes into user-facing messages.
/// Crash reporting happens in the `main.dart` error zone, not here.
final rpcErrorHandlerProvider = Provider<RpcErrorHandler>((ref) {
  return RpcErrorHandler();
});

/// Provider for AuthService
final authServiceProvider = Provider<AuthService>((ref) {
  return AuthService(
    transport: ref.watch(transportProvider),
    serverUrl: Environment.getServer,
  );
});

/// Provider for AuthRepository
final authRepositoryProvider = Provider<AuthRepository>((ref) {
  final service = ref.watch(authServiceProvider);
  return AuthRepository(service);
});

/// Provider for [ShareLinkRepository] — resolves share-link shortcodes
/// to typed invitation context with Stash caching.
final shareLinkRepositoryProvider = Provider<ShareLinkRepository>((ref) {
  return ShareLinkRepository(
    ref.watch(cacheManagerProvider),
    ref.watch(authRepositoryProvider),
  );
});

/// Provider for FlutterSecureStorage.
///
/// Exposed as a provider so tests can override it with a mock, giving
/// AuthStateNotifier a clean seam for secure-storage reads and writes.
final secureStorageProvider = Provider<FlutterSecureStorage>(
  (ref) => const FlutterSecureStorage(),
);

/// Provider for SharedPreferencesAsync.
///
/// Exposed as a provider so tests can override it with a mock, giving
/// AuthStateNotifier a clean seam for shared-preference reads and writes.
final sharedPreferencesAsyncProvider = Provider<SharedPreferencesAsync>(
  (ref) => SharedPreferencesAsync(),
);
