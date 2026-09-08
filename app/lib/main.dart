import 'dart:async';
import 'dart:ui';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_crashlytics/firebase_crashlytics.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:firebase_performance/firebase_performance.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_plugins/url_strategy.dart';
import 'package:logging/logging.dart';
import 'package:mapbox_maps_flutter/mapbox_maps_flutter.dart';
import 'package:ripls/core/audio/audio_session_manager.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/core/config/firebase_options_web.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/manager.dart';
import 'package:ripls/core/router/app_router.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/core/utils/timeago_short_messages.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/debug/poll_preview.dart';
import 'package:ripls/presentation/screens/splash/splash_screen.dart';
import 'package:ripls/presentation/viewmodels/locale_view_model.dart';
import 'package:ripls/presentation/viewmodels/splash_view_model.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/presentation/widgets/connectivity_banner.dart';
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/deferred_deep_link_service.dart';
import 'package:ripls/services/fcm_service.dart';
import 'package:ripls/services/notification_route_replayer.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:timeago/timeago.dart' as timeago;
import 'package:timezone/data/latest.dart' as tz;

final _log = Logger('main');

/// Trace for measuring app startup time.
/// Started in main(), stopped after first frame in GearApp.
Trace? _appStartupTrace;

/// Stops the app startup trace if it's running.
void _stopAppStartupTrace() {
  final trace = _appStartupTrace;
  if (trace != null) {
    trace.stop();
    _appStartupTrace = null;
  }
}

/// Heuristic matcher for the #2158 logout-frame race signature. Used by the
/// global error handlers to decide whether to dump the [LogoutDiagnostics]
/// ring buffer alongside the crash report. We match on the documented
/// substrings rather than the full stack so the heuristic survives Flutter
/// framework wording tweaks.
bool _matchesLogoutRaceSignature(String message) {
  if (message.contains('RenderIgnorePointer') &&
      message.contains('performLayout')) {
    return true;
  }
  if (message.contains('Duplicate GlobalKey')) return true;
  if (message.contains('_retakeInactiveElement')) return true;
  if (message.contains('was mutated when none of its ancestors is actively '
      'performing layout')) {
    return true;
  }
  return false;
}

void main() async {
  // Run app in a zone that catches uncaught errors
  await runZonedGuarded<Future<void>>(
    () async {
      WidgetsFlutterBinding.ensureInitialized();

      // Debug-only visual preview harness for the poll morphing sheet. When
      // launched with `--dart-define=POLL_PREVIEW=<kind>` (e.g. time-tbd),
      // boot straight into a seeded poll body over a mock hero — no auth, no
      // network — for iterating on the redesign against the HTML prototypes.
      const pollPreview = String.fromEnvironment('POLL_PREVIEW');
      if (pollPreview.isNotEmpty) {
        runApp(PollPreviewApp(kind: pollPreview));
        return;
      }

      // Flutter Web defaults to hash-based routing (URLs like
      // /#/event/EXP_ID), which causes the GoRouter to ignore the
      // actual `/event/...` browser path on first load and bounce
      // to /register (since the empty hash means it sees /). Switch
      // to PathUrlStrategy so GoRouter reads the real
      // `window.location.pathname`. No-op on non-web platforms.
      usePathUrlStrategy();

      // Force the Flutter Web semantics tree to serialize to DOM on
      // every load, rather than waiting for assistive tech to be
      // detected. Two reasons: (1) accessibility — the a11y plan
      // (docs/ai/accessibility_plan.md) wants this anyway so screen
      // readers don't need a detection round-trip; (2) the e2e
      // Playwright harness (#2162) queries widgets via the
      // `flt-semantics-identifier` DOM attribute that this tree
      // produces. Perf cost tracked in #2229.
      if (kIsWeb) {
        SemanticsBinding.instance.ensureSemantics();
      }

      // Lock orientation to portrait-up only
      if (!kIsWeb) {
        unawaited(SystemChrome.setPreferredOrientations([
          DeviceOrientation.portraitUp,
        ]));
      }

      // Configure logging
      Environment.configureLogging();

      // Issue #1581 RPC traffic audit: emit RPC_AUDIT and CACHE_AUDIT log
      // lines in debug builds so a session walkthrough can be analyzed for
      // redundant RPCs. Off in release.
      if (kDebugMode) {
        RpcUtils.auditLogging = true;
      }

      // Set up short timeago messages (e.g., "2h" instead of "2 hours ago")
      timeago.setLocaleMessages('en_short', ShortMessages());
      timeago.setDefaultLocale('en_short');

      // Initialize timezone database for timezone-aware date formatting
      tz.initializeTimeZones();

      // Configure the platform audio session to coexist with other apps'
      // audio. Without this, `video_player` defaults to a `playback`
      // category on iOS that interrupts Spotify / Apple Music / podcasts
      // the moment a muted feed video appears (issue #1250). A failure
      // here is non-fatal — the app still runs, just with the original
      // hijack behavior — so we warn and continue rather than blocking
      // startup on a platform-channel hiccup.
      try {
        await AudioSessionManager().configureForMutedPlayback();
      } catch (e, stackTrace) {
        _log.warning(
            'Failed to configure muted audio session at startup', e, stackTrace);
      }

      // Initialize Firebase (required for crashlytics, performance,
      // and notifications). Mobile auto-detects config from the
      // platform-native google-services.json (Android) /
      // GoogleService-Info.plist (iOS) files; web has no equivalent
      // auto-detection and requires explicit FirebaseOptions from
      // firebase_options_web.dart.
      if (kIsWeb) {
        await Firebase.initializeApp(options: webFirebaseOptions());
      } else {
        await Firebase.initializeApp();
      }

      // NOTE: Firebase Auth Emulator wiring (for the phone-first e2e) is NOT
      // done here. useAuthEmulator does a network round-trip to the emulator,
      // and paying it on every web page load taxed the whole e2e suite (it
      // timed out the multi-client spec). The phone-register screen is the only
      // place that uses Firebase Auth, so it wires the emulator lazily right
      // before verifyPhoneNumber — keeping the AUTH_EMULATOR_HOST define inert
      // for every non-phone-auth flow. See phone_auth_screen.dart.

      // CRITICAL: Enable crash reporting EARLY (before anything can fail)
      // Load user consent from SharedPreferences and enable immediately if granted.
      //
      // Crashlytics is mobile-only — there's no web implementation, and
      // calling FirebaseCrashlytics methods on web throws
      // MissingPluginException. Gate every Crashlytics call on !kIsWeb.
      // Performance has a web implementation, so it stays unguarded.
      try {
        final prefs = await SharedPreferences.getInstance();
        final crashConsent =
            prefs.getString('observability_crash') ?? 'granted';
        final perfConsent = prefs.getString('observability_perf') ?? 'granted';

        // Enable crash reporting immediately if user has consented
        if (!kIsWeb) {
          await FirebaseCrashlytics.instance.setCrashlyticsCollectionEnabled(
            crashConsent == 'granted',
          );
        }

        // Enable performance monitoring immediately if user has consented
        await FirebasePerformance.instance.setPerformanceCollectionEnabled(
          perfConsent == 'granted',
        );
      } catch (e) {
        // If loading consent fails, default to enabled (better to have crash reports)
        _log.warning('Failed to load observability settings', e);
        if (!kIsWeb) {
          await FirebaseCrashlytics.instance.setCrashlyticsCollectionEnabled(true);
        }
        await FirebasePerformance.instance.setPerformanceCollectionEnabled(true);
      }

      // Start app startup performance trace
      // This will be stopped after the first frame is rendered
      _appStartupTrace = FirebasePerformance.instance.newTrace('app_startup');
      await _appStartupTrace?.start();

      // Set up Firebase Messaging background handler (only if
      // notifications enabled). FCM on web requires a service worker
      // (firebase-messaging-sw.js) that we don't ship, so the
      // onBackgroundMessage hook can't fire there — gate on !kIsWeb.
      if (Environment.notificationsEnabled && !kIsWeb) {
        FirebaseMessaging.onBackgroundMessage(
          firebaseMessagingBackgroundHandler,
        );
      }

      // Set up Flutter error handler to report to Crashlytics
      FlutterError.onError = (FlutterErrorDetails details) {
        final exceptionString = details.exceptionAsString();
        _log.severe(
          'Flutter error: $exceptionString',
          details.exception,
          details.stack,
        );
        // Dump the logout diagnostics ring buffer whenever a crash signature
        // matching the documented #2158 race appears, or whenever the crash
        // arrives inside the logout window. This is the load-bearing
        // diagnostic for the "RenderIgnorePointer was mutated in
        // performLayout" / duplicate-GlobalKey class of crashes.
        if (_matchesLogoutRaceSignature(exceptionString) ||
            LogoutDiagnostics.inLogoutWindow()) {
          _log.severe(
            'LogoutDiagnostics dump (Flutter error):\n${LogoutDiagnostics.dump()}',
          );
        }
        // Crashlytics is mobile-only; the log line above is the only
        // sink on web (which is fine for v1 — web is a small guest
        // surface).
        if (!kIsWeb) {
          FirebaseCrashlytics.instance.recordFlutterFatalError(details);
        }
      };

      // Set up handler for errors outside of Flutter framework (async errors)
      PlatformDispatcher.instance.onError = (error, stack) {
        _log.severe('Platform error: $error', error, stack);
        if (_matchesLogoutRaceSignature(error.toString()) ||
            LogoutDiagnostics.inLogoutWindow()) {
          _log.severe(
            'LogoutDiagnostics dump (platform error):\n${LogoutDiagnostics.dump()}',
          );
        }
        if (!kIsWeb) {
          // Network/connection errors are non-fatal - app can recover.
          // Expected RPC failures (permission_denied, not_found, ...) are also
          // non-fatal — they're server-driven decisions, not crashes (#1703).
          final isFatal = !RpcErrorHandler.isNetworkError(error) &&
              !RpcErrorHandler.isExpectedRpcError(error);
          FirebaseCrashlytics.instance.recordError(error, stack, fatal: isFatal);
        }
        return true;
      };

      // Initialize Mapbox with access token (not supported on web)
      if (!kIsWeb) {
        MapboxOptions.setAccessToken(Environment.mapboxAccessToken);
      }

      // Initialize Google Maps SDK on iOS (#2188). Same secret-flow
      // shape as Mapbox: --dart-define value → Dart → native SDK via
      // platform channel. AppDelegate's didInitializeImplicitFlutterEngine
      // registers the handler, which calls GMSServices.provideAPIKey.
      // Skipped on web (loaded via index.html script tag) and Android
      // (key is read from the manifest meta-data at process launch,
      // before Dart runs).
      if (defaultTargetPlatform == TargetPlatform.iOS &&
          !kIsWeb &&
          Environment.googleMapsApiKey.isNotEmpty) {
        const channel = MethodChannel('org.ripls.app/google_maps_init');
        try {
          await channel.invokeMethod<void>('provideAPIKey', Environment.googleMapsApiKey);
        } catch (e, stack) {
          _log.warning('Google Maps init failed; map widgets will not render', e, stack);
        }
      }

      runApp(const ProviderScope(child: GearApp()));
    },
    (error, stack) {
      // This catches errors that escape the Zone (e.g., from isolates)
      _log.severe('Uncaught error in zone: $error', error, stack);
      if (Environment.notificationsEnabled && !kIsWeb) {
        // Network/connection errors are non-fatal - app can recover.
        // Expected RPC failures (permission_denied, not_found, ...) are also
        // non-fatal — they're server-driven decisions, not crashes (#1703).
        // Crashlytics is mobile-only.
        final isFatal = !RpcErrorHandler.isNetworkError(error) &&
            !RpcErrorHandler.isExpectedRpcError(error);
        FirebaseCrashlytics.instance.recordError(error, stack, fatal: isFatal);
      }
    },
  );
}

class GearApp extends ConsumerStatefulWidget {
  const GearApp({super.key});

  @override
  ConsumerState<GearApp> createState() => _GearAppState();
}

class _GearAppState extends ConsumerState<GearApp> with WidgetsBindingObserver {
  late final DateTime _appStartTime;

  /// Set after the initState() microtask finishes initial community loading.
  /// Guards the auth listener so it only fires _loadCommunities() for
  /// post-startup auth changes (fresh login), not during cold start where
  /// the initState microtask already handles it.
  bool _initialLoadComplete = false;

  /// Single executor for notification deep links (#2636). Drains the
  /// [PendingNotificationRouteHolder] onto the router — from the live-tap poke,
  /// at splash→ready, and on resume — and replays once if a navigation doesn't
  /// stick. Reads the router/telemetry via `ref` at call time so it survives a
  /// provider rebuild.
  late final NotificationRouteReplayer _notificationReplayer =
      NotificationRouteReplayer(
    router: () => ref.read(routerProvider),
    logAnalyticsEvent: (event) =>
        ref.read(observabilityServiceProvider).logAnalyticsEvent(event),
    // A permanent client-side drop (replay also failed) is reported as a
    // Crashlytics non-fatal so it's visible per-incident from prod user
    // devices — not just in aggregate analytics — and can drive the
    // alert→issue pipeline (#2636).
    onUnrecoverableDrop: (route, {String actualLocation = ''}) {
      ref.read(observabilityServiceProvider).recordError(
            NotificationDeepLinkDroppedException(
              route: route.route,
              source: route.source,
            ),
            reason: 'Notification deep link unrecoverable after replay '
                '(source=${route.source}, route=${route.route}, '
                'actual=$actualLocation)',
          );
    },
  );

  @override
  void initState() {
    super.initState();

    // Track app start time for minimum splash duration
    _appStartTime = DateTime.now();

    // Stop app startup trace after first frame is rendered
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _stopAppStartupTrace();
    });

    // Register lifecycle observer
    WidgetsBinding.instance.addObserver(this);

    // Wire the global locale getter so RpcUtils.buildHeaders can
    // attach Accept-Language to every outbound RPC. Falls back to the
    // device locale when the user has not explicitly picked one. The
    // closure is invoked once per RPC, so it is cheap by design.
    RpcUtils.configureLocaleGetter(() {
      final picked =
          ref.read(localePreferenceProvider).asData?.value?.languageCode;
      if (picked != null && picked.isNotEmpty) {
        return picked;
      }
      final device =
          PlatformDispatcher.instance.locale.languageCode;
      return device.isEmpty ? null : device;
    });

    // Swap the platform audio-session category whenever the user
    // toggles a feed video's mute state. `.ambient` lets other apps'
    // audio coexist when no video is unmuted; `.playback` + duckOthers
    // takes exclusive audio when the user has explicitly unmuted
    // (issue #1250). Failures here are non-fatal — falling back to the
    // last-set category just means the swap didn't apply.
    ref.listenManual<bool>(videoUnmutedProvider, (previous, next) {
      if (previous == next) return;
      final manager = ref.read(audioSessionManagerProvider);
      final future = next
          ? manager.configureForAudiblePlayback()
          : manager.configureForMutedPlayback();
      future.catchError((Object e, StackTrace st) {
        _log.warning('Failed to swap audio session category', e, st);
      });
    });

    // Listen for auth state changes to load communities when user logs in.
    // Guarded by _initialLoadComplete so this only fires for post-startup auth
    // changes (fresh login). During cold start, the initState() microtask
    // already calls _loadCommunities() — without this guard both paths fire
    // and produce duplicate network calls.
    ref.listenManual(authStateProvider, (previous, next) {
      if (!mounted) return;
      if (_initialLoadComplete &&
          previous != null &&
          !previous.isAuthenticated &&
          next.isAuthenticated) {
        _log.info(
            'Auth state changed to authenticated, loading communities');
        _initializeFCM();
        _loadCommunities();
      }
    });

    // Load auth state on app start
    Future.microtask(() async {
      // Start total initialization trace (from initState to ready)
      final initTrace =
          FirebasePerformance.instance.newTrace('startup_total_init');
      await initTrace.start();

      // Start auth check trace
      final authTrace =
          FirebasePerformance.instance.newTrace('startup_auth_check');
      await authTrace.start();

      ref
          .read(splashProvider.notifier)
          .updateStep(SplashLoadingStep.checkingAuth);

      // Check for deferred deep link context BEFORE loading auth state.
      // loadAuthState() triggers a router refresh, and the redirect logic
      // needs these providers already set to route first-launch users
      // correctly. The redirect only reads these when unauthenticated, so
      // setting them unconditionally is safe.
      //
      // Skipped on web: the deferred-deep-link concept is an Android
      // Install Referrer / iOS clipboard-paste mechanic for users who
      // installed the app via a share link before opening it. Web has
      // no install step — the SSR landing's CTA already carries the
      // share code via the URL. The service uses `Platform.is*` which
      // throws `Unsupported operation: Platform._operatingSystem` on
      // web; gating here keeps startup clean.
      if (!kIsWeb) {
        try {
          final service = ref.read(deferredDeepLinkServiceProvider);
          // Check first-launch status before getContext() marks it
          // consumed.
          final firstLaunch = await service.isFirstLaunch();
          if (firstLaunch) {
            FirstLaunchFlag.value = true;
          }
          final deferredContext = await service.getContext();
          if (deferredContext != null) {
            _log.info('Deferred deep link context found, '
                'shortCode=${deferredContext.shortCode != null}');
            DeferredDeepLinkContextHolder.value = deferredContext;
          }
        } catch (e) {
          _log.warning('Failed to check deferred deep link: $e');
        }
      }

      await ref.read(authStateProvider.notifier).loadAuthState();

      final authState = ref.read(authStateProvider);
      await authTrace.stop();

      // Initialize services (fire-and-forget, non-blocking)
      if (authState.isAuthenticated) {
        _log.info(
            'User authenticated on app startup, initializing services and loading communities');
        _initializeFCM(); // Fire and forget
        _updatePresence(isInForeground: true); // Fire and forget

        // Load communities (CRITICAL — blocks showing main app)
        final communitiesTrace =
            FirebasePerformance.instance.newTrace('startup_load_communities');
        await communitiesTrace.start();

        ref
            .read(splashProvider.notifier)
            .updateStep(SplashLoadingStep.loadingCommunities);

        await _loadCommunities();

        await communitiesTrace.stop();
      }

      _initialLoadComplete = true;

      // Adaptive splash screen duration (min 1.5s for branding, max 5s to prevent hanging)
      final elapsed = DateTime.now().difference(_appStartTime).inMilliseconds;
      const minSplashDuration = 1500; // 1.5 seconds minimum for branding
      const maxSplashDuration = 5000; // 5 seconds maximum to prevent long waits

      if (elapsed < minSplashDuration) {
        // Loading finished quickly - wait remaining time for branding visibility
        final remaining = minSplashDuration - elapsed;
        await Future.delayed(Duration(milliseconds: remaining));
      } else if (elapsed > maxSplashDuration) {
        // Loading took too long - proceed immediately without additional delay
        _log.warning(
            'App initialization took ${elapsed}ms (exceeds max ${maxSplashDuration}ms)');
      }
      // else: elapsed is between min and max - proceed immediately

      // Transition to main app
      ref.read(splashProvider.notifier).updateStep(SplashLoadingStep.ready);

      // Recover any notification route captured before the router was mounted
      // (a terminated-state tap that resolved during cold start). The router is
      // live now; drain once the first framed build settles (#2636).
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _notificationReplayer.drain('ready');
      });

      // Stop total initialization trace
      await initTrace.stop();

      // Defer non-critical initialization (background)
      if (authState.isAuthenticated) {
        Future.delayed(const Duration(milliseconds: 500), () {
          if (mounted) {
            ref.read(unreadCountProvider.notifier).initialize();
          }
        });
      }
    });
  }

  /// Loads the authenticated user's communities into [communitiesProvider].
  ///
  /// Called on app startup if the user is authenticated, and again after
  /// successful login.
  Future<void> _loadCommunities() async {
    _log.info('Starting community loading for authenticated user');

    // Wipe any per-user state left over from a previous session
    // BEFORE awaiting the new user's communities. Without this, the
    // brief window between auth-state-change and setCommunities() lets
    // the prior user's community IDs bleed into RPC requests like
    // GetWorkshopBrief / GetWorkshopSynthesis — the new user isn't a
    // member of those circles, and the server returns
    // permission_denied. Specifically:
    //   - communitiesProvider.communities still holds the prior
    //     list (setLoading doesn't clear it), so
    //     workshopEnabledCommunityIdsProvider emits stale IDs.
    //   - workshopCommunityProvider may hold a carousel pin set by
    //     the prior user that doesn't exist for the new one.
    // Both are safe to clear here because the IndexedStack-driven
    // /home screen has not mounted yet (we're still on /login during
    // this listener tick).
    ref.read(communitiesProvider.notifier).clear();
    ref.read(workshopCommunityProvider.notifier).select(null);

    // Set loading state
    ref.read(communitiesProvider.notifier).setLoading(true);

    try {
      final communities =
          await ref.read(communityRepositoryProvider).listUserCommunities();

      _log.info(
          'Communities loaded successfully: ${communities.length} communities');

      await ref
          .read(communitiesProvider.notifier)
          .setCommunities(communities);

      _log.info('Communities initialized');

      // Open the realtime layer for real-time updates.
      _startRealtime();
    } catch (e, stackTrace) {
      _log.severe(
          'Failed to load communities during initialization', e, stackTrace);
      // Set error state - HomeScreen will show error UI and allow retry
      ref
          .read(communitiesProvider.notifier)
          .setError('Unable to reach the server. Please try again.');
    }
  }

  @override
  void dispose() {
    // Unregister lifecycle observer
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    super.didChangeAppLifecycleState(state);

    _log.info('App lifecycle state changed: $state');

    final authState = ref.read(authStateProvider);
    if (!authState.isAuthenticated) {
      // Don't send presence updates if not authenticated
      return;
    }

    switch (state) {
      case AppLifecycleState.resumed:
        // App is in foreground and visible
        _updatePresence(isInForeground: true);

        // Check for notification that opened the app from background.
        // Catches missed onMessageOpenedApp events (e.g., after Android
        // Activity recreation). Deduplicates by message ID internally.
        _checkForMissedNotification();

        // Recover a notification route that was captured but never navigated
        // (e.g. resolved while onNavigate was transiently unwired). No-op when
        // nothing is pending (#2636).
        _notificationReplayer.drain('resume');

        // Refresh communities when app resumes (fire-and-forget, non-blocking)
        // This ensures community list stays up-to-date if user was invited/removed while away
        _refreshCommunitiesOnResume();

        // Reopen the realtime layer. No community list is needed: the stream
        // and poll are user-scoped and the server resolves membership.
        _startRealtime();
        break;
      case AppLifecycleState.paused:
        // App is in background or minimized
        _updatePresence(isInForeground: false);
        ref.read(communityEventPollerProvider).stop();
        ref.read(communityEventStreamProvider).disconnect();
        break;
      case AppLifecycleState.inactive:
      case AppLifecycleState.detached:
      case AppLifecycleState.hidden:
        // App is transitioning or terminating - treat as background
        _updatePresence(isInForeground: false);
        break;
    }
  }

  /// Updates the user's presence status on the server.
  void _updatePresence({required bool isInForeground}) {
    try {
      final chatService = ref.read(chatServiceProvider);
      final status = isInForeground ? 'FOREGROUND' : 'BACKGROUND';
      _log.info('Updating presence to: $status');

      // Fire and forget - don't wait for response
      chatService.updatePresence(isInForeground: isInForeground).catchError((
        error,
      ) {
        _log.warning('Failed to update presence: $error');
      });
    } catch (e) {
      _log.severe('Error updating presence: $e');
    }
  }

  /// Refreshes the community list when the app resumes from background.
  ///
  /// This ensures the community list stays up-to-date if the user was
  /// invited to new communities or removed from existing ones while the app
  /// was in the background.
  void _refreshCommunitiesOnResume() {
    _log.info('Refreshing communities on app resume');

    // Fire-and-forget refresh (non-blocking)
    ref.read(communityRepositoryProvider).listUserCommunities().then(
      (communities) {
        if (mounted) {
          ref
              .read(communitiesProvider.notifier)
              .setCommunities(communities);
          _log.info(
              'Communities refreshed on resume: ${communities.length} communities');
        }
      },
      onError: (error) {
        _log.warning('Failed to refresh communities on resume: $error');
        // Don't show error - this is a background refresh, not critical
      },
    );
  }

  /// Opens the user's event stream and starts the poll backstop.
  ///
  /// Both are user-scoped and idempotent, so this is safe to call on every
  /// resume. It takes no community list: the server resolves membership per
  /// event, which is what lets a join take effect without reopening anything
  /// (#2867).
  void _startRealtime() {
    ref.read(communityEventPollerProvider).start();
    ref.read(communityEventStreamProvider).connect();
  }

  void _initializeFCM() {
    if (!Environment.notificationsEnabled) {
      _log.info('Notifications disabled via ENABLE_NOTIFICATIONS=false');
      return;
    }
    // FCM on web requires a service worker (firebase-messaging-sw.js)
    // that we don't ship, plus fcm_service uses dart:io Platform
    // checks for platform-specific initialization. Skip on web.
    // See #2157 plugin audit.
    if (kIsWeb) {
      return;
    }

    try {
      final fcmService = ref.read(fcmServiceProvider);

      // Notification-tap navigation (#2636). The resolved route is already in
      // PendingNotificationRouteHolder (written in routeNotificationPayload the
      // instant the payload decoded), so this callback is just a poke: the app
      // is live, drain now. The replayer owns go() + the post-frame landed
      // check + the one bounded replay + nav_result telemetry, so a link that
      // doesn't stick is recovered rather than lost. If this callback is ever
      // null (a rebuilt service), the holder still holds the route and the
      // ready/resume drains recover it.
      fcmService.onNavigate = (String route, {String? communityId}) {
        _notificationReplayer.drain('tap');
      };

      fcmService.onCommunityEvent = (data) {
        ref.read(eventRouterProvider).routeFcmEvent(data);
      };

      fcmService.initialize();
    } catch (e) {
      _log.severe('Failed to initialize FCM: $e');
    }
  }

  /// Checks for a notification that opened the app but wasn't handled.
  void _checkForMissedNotification() {
    if (!Environment.notificationsEnabled) return;
    // No FCM on web — see _initializeFCM comment.
    if (kIsWeb) return;
    try {
      ref.read(fcmServiceProvider).checkForMissedNotification();
    } catch (e) {
      _log.warning('Failed to check for missed notification: $e');
    }
  }

  @override
  Widget build(BuildContext context) {
    final splashState = ref.watch(splashProvider);
    final router = ref.watch(routerProvider);
    final themeMode = ref.watch(themeModeProvider);
    final locale = ref.watch(localePreferenceProvider).asData?.value;

    // Show splash screen until explicitly marked ready
    // CRITICAL: Use splashState.step, NOT authState.isLoading
    // This allows us to control splash duration independently of auth loading
    if (splashState.step != SplashLoadingStep.ready) {
      return MaterialApp(
        key: const ValueKey('splash'),
        title: Environment.getAppName,
        theme: AppTheme.lightTheme,
        darkTheme: AppTheme.darkTheme,
        themeMode: themeMode,
        locale: locale,
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: const SplashScreen(),
      );
    }
    return ObservabilityManager(
      child: ConnectivityBanner(
        child: MaterialApp.router(
          title: Environment.getAppName,
          theme: AppTheme.lightTheme,
          darkTheme: AppTheme.darkTheme,
          themeMode: themeMode,
          routerConfig: router,
          locale: locale,
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          // Seeds localized strings used by ChatNotificationManager from the
          // Android background isolate (which has no l10n context).
          // Inserted via `builder` so we run as a descendant of Localizations
          // and can resolve `context.l10n`.
          builder: (context, child) =>
              _ChatNotificationStringSeeder(child: child),
        ),
      ),
    );
  }
}

/// Seeds [ChatNotificationManager] localized strings on first build of a
/// localized subtree. One-shot per app launch — repeated builds are no-ops.
class _ChatNotificationStringSeeder extends StatefulWidget {
  const _ChatNotificationStringSeeder({required this.child});

  final Widget? child;

  @override
  State<_ChatNotificationStringSeeder> createState() =>
      _ChatNotificationStringSeederState();
}

class _ChatNotificationStringSeederState
    extends State<_ChatNotificationStringSeeder> {
  bool _seeded = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_seeded) return;
    _seeded = true;
    final l10n = AppLocalizations.of(context);
    unawaited(
      ChatNotificationManager.persistLocalizedStrings(
        selfName: l10n.notificationSelfName,
        chatsChannelName: l10n.notificationChatsChannelName,
        chatsChannelDescription: l10n.notificationChatsChannelDescription,
      ),
    );
  }

  @override
  Widget build(BuildContext context) => widget.child ?? const SizedBox.shrink();
}
