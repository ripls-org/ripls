import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/logging/logger.dart';

final _log = ObservableLogger.named('NotificationRouteReplayer');

/// Delay before the single replay attempt. Long enough for a transient
/// override (auth redirect churn, a not-yet-settled router) to finish, short
/// enough to feel instant. A timer — not a post-frame callback — because an
/// idle app schedules no frames, so a frame-linked retry could stall
/// indefinitely.
const _replayDelay = Duration(milliseconds: 50);

/// A resolved notification navigation target, captured the instant the FCM
/// payload is decoded.
///
/// Capturing the target up front — before any navigation is attempted — is what
/// lets a deep link survive a router that is not yet mounted or a rebuilt FCM
/// service whose imperative navigation callback was never re-wired (#2636).
@immutable
class PendingNotificationRoute {
  /// Resolved GoRouter location, e.g. `/experience/exp-1`. Path + entity id
  /// only — no PII.
  final String route;

  /// Community to select on arrival, when the payload carried one.
  final String? communityId;

  /// Where the tap entered from: `opened` / `initial` / `missed` / `local`.
  /// Carried through to the `nav_result` telemetry so a query can correlate a
  /// drop with its ingress vector.
  final String source;

  const PendingNotificationRoute({
    required this.route,
    this.communityId,
    required this.source,
  });

  @override
  String toString() =>
      'PendingNotificationRoute(route: $route, communityId: $communityId, '
      'source: $source)';
}

/// Reported to Crashlytics as a **non-fatal** (never thrown) when a
/// notification deep link cannot be navigated even after one replay — a
/// permanent client-side drop.
///
/// A crash breadcrumb only surfaces on an actual crash, and an analytics event
/// is only per-incident queryable if the BigQuery export is wired; a non-fatal,
/// by contrast, is reported from real prod devices, grouped by this type, and
/// can drive the existing Crashlytics alert→issue pipeline (#2636). Carries the
/// entity-id-bearing route + ingress source only — no titles/names/bodies.
class NotificationDeepLinkDroppedException implements Exception {
  final String route;
  final String source;

  const NotificationDeepLinkDroppedException({
    required this.route,
    required this.source,
  });

  @override
  String toString() =>
      'NotificationDeepLinkDroppedException(source: $source, route: $route)';
}

/// Process-wide hand-off slot for a pending notification route.
///
/// Mirrors [DeferredDeepLinkContextHolder]: a static field (not a Riverpod
/// provider) because it is written from FCMService's routing path — which has
/// no `ref` — and drained from the widget tree at defined lifecycle points.
///
/// Writing here is the **catch**: once a payload resolves to a route the target
/// is never silently lost, even when the immediate executor is unavailable
/// (router not mounted, callback unwired). A later [NotificationRouteReplayer]
/// drain — at splash→ready or app-resume — **recovers** it.
class PendingNotificationRouteHolder {
  /// The route awaiting navigation, or null when nothing is pending. Assigning
  /// replaces any earlier un-drained target (the newest tap wins).
  static PendingNotificationRoute? value;

  /// Consumes and returns the current target, clearing the slot so a
  /// subsequent drain does not re-navigate to it.
  static PendingNotificationRoute? consume() {
    final current = value;
    value = null;
    return current;
  }
}

/// Drives a captured [PendingNotificationRoute] onto the router and verifies it
/// stuck, retrying once when it did not — the **recover** half of #2636.
///
/// Kept off the widget tree (it takes a router accessor + a telemetry sink
/// rather than a `BuildContext`) so the go → verify → replay logic is
/// unit-testable against a real [GoRouter] without pumping the whole app.
class NotificationRouteReplayer {
  final GoRouter Function() _router;
  final void Function(AnalyticsEvent event) _logAnalyticsEvent;
  final void Function(PendingNotificationRoute route, {String actualLocation})?
      _onUnrecoverableDrop;

  /// True while a navigation + its post-frame verification is in flight, so a
  /// second drain (e.g. a live `tap` poke racing the `ready` drain) doesn't
  /// fire an overlapping navigation for a freshly-stashed target.
  bool _navigating = false;

  NotificationRouteReplayer({
    required GoRouter Function() router,
    required void Function(AnalyticsEvent event) logAnalyticsEvent,
    // Invoked exactly once when a route can't be landed even after the replay —
    // wired to a Crashlytics non-fatal so a permanent drop is visible from prod
    // devices, not just in aggregate analytics. Not fired for a first-attempt
    // miss that later recovers.
    void Function(PendingNotificationRoute route, {String actualLocation})?
        onUnrecoverableDrop,
  })  : _router = router,
        _logAnalyticsEvent = logAnalyticsEvent,
        _onUnrecoverableDrop = onUnrecoverableDrop;

  /// Consumes any pending route and navigates to it. [trigger] names the drain
  /// vector (`tap` / `ready` / `resume`) for telemetry. No-op when nothing is
  /// pending — safe to call from every lifecycle hook.
  void drain(String trigger) {
    final pending = PendingNotificationRouteHolder.consume();
    if (pending == null) return;

    if (_navigating) {
      // A navigation is already settling; re-stash so this target isn't lost
      // and let the in-flight attempt (or its replay) complete first. The next
      // drain hook picks it up.
      PendingNotificationRouteHolder.value = pending;
      return;
    }
    _navigate(pending, trigger: trigger, attempt: 0);
  }

  void _navigate(
    PendingNotificationRoute pending, {
    required String trigger,
    required int attempt,
  }) {
    _navigating = true;
    final router = _router();
    _log.info(
      'Notification nav: go("${pending.route}") '
      'trigger=$trigger attempt=$attempt',
    );
    router.go(pending.route);

    // Confirm the navigation actually stuck a frame later. A mismatch means
    // something overrode it (auth redirect churn, a not-yet-settled router) —
    // the exact drop this issue keeps hitting — so retry once before giving up.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final actual = router.routerDelegate.currentConfiguration.uri.toString();
      final landed = actual == pending.route;
      final isReplay = attempt > 0;

      if (!landed && !isReplay) {
        // First attempt slipped — record the miss, then replay one frame later
        // once whatever overrode it has settled.
        _log.warning(
          'Notification nav did not stick: intended="${pending.route}" '
          'actual="$actual" — scheduling one replay',
        );
        _logAnalyticsEvent(NotificationDeepLinkEvent(
          phase: 'nav_result',
          source: pending.source,
          route: pending.route,
          actualLocation: actual,
          landed: false,
          trigger: trigger,
        ));
        Timer(_replayDelay, () {
          _navigate(pending, trigger: 'replay', attempt: attempt + 1);
        });
        return;
      }

      _navigating = false;
      if (!landed) {
        // Reaching here with !landed means the replay also missed (a first
        // attempt miss returns early above), so this is a permanent drop.
        _log.warning(
          'Notification nav replay also failed: intended="${pending.route}" '
          'actual="$actual" — giving up (deep link dropped)',
        );
        _onUnrecoverableDrop?.call(pending, actualLocation: actual);
      } else if (isReplay) {
        _log.info('Notification nav recovered on replay: "${pending.route}"');
      }
      _logAnalyticsEvent(NotificationDeepLinkEvent(
        phase: 'nav_result',
        source: pending.source,
        route: pending.route,
        actualLocation: actual,
        landed: landed,
        trigger: isReplay ? 'replay' : trigger,
        // recovered is meaningful only for a replay: true ⇒ the retry rescued a
        // link the first attempt dropped; false ⇒ a permanent client-side drop.
        recovered: isReplay ? landed : null,
      ));
    });
  }
}
