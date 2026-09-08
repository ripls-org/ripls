import 'package:flutter/material.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/providers.dart';

final _log = Logger('AnalyticsRouteObserver');

/// A NavigatorObserver that logs screen views to analytics.
///
/// This observer tracks navigation events and logs screen views
/// to Firebase Analytics for user journey analysis.
///
/// Usage with GoRouter:
/// ```dart
/// GoRouter(
///   observers: [
///     AnalyticsRouteObserver(
///       getAnalyticsProvider: () => observabilityService.analyticsProvider,
///     ),
///   ],
///   routes: [...],
/// )
/// ```
class AnalyticsRouteObserver extends NavigatorObserver {
  /// Most-recently-pushed/replaced/popped-to screen name, sanitized the
  /// same way analytics receives it. Read by `RpcUtils.executeRpc()` to
  /// stamp every audit log line with the screen the user was on when the
  /// RPC fired. Null until the first navigation event.
  static String? _currentScreen;

  /// Returns the most recent screen name observed by this observer.
  /// See [_currentScreen] for semantics.
  static String? get currentScreen => _currentScreen;

  /// Function to get the current analytics provider.
  /// This allows the observer to get the consent-aware provider.
  final AnalyticsProvider Function()? _getProvider;

  /// Creates an AnalyticsRouteObserver.
  ///
  /// [getAnalyticsProvider] is an optional function that returns the current
  /// analytics provider. If not provided, screen views will not be logged.
  AnalyticsRouteObserver({
    AnalyticsProvider Function()? getAnalyticsProvider,
  }) : _getProvider = getAnalyticsProvider;

  @override
  void didPush(Route<dynamic> route, Route<dynamic>? previousRoute) {
    _logScreenView(route);
  }

  @override
  void didReplace({Route<dynamic>? newRoute, Route<dynamic>? oldRoute}) {
    if (newRoute != null) {
      _logScreenView(newRoute);
    }
  }

  @override
  void didPop(Route<dynamic> route, Route<dynamic>? previousRoute) {
    // When popping, log the screen we're returning to
    if (previousRoute != null) {
      _logScreenView(previousRoute);
    }
  }

  /// Logs a screen view for the given route.
  void _logScreenView(Route<dynamic> route) {
    final screenName = _extractScreenName(route);
    if (screenName == null) {
      return;
    }

    // Always update the static current screen pointer, even if the user
    // hasn't consented to analytics — it's read by RpcUtils for local
    // debug logging only and never leaves the device.
    _currentScreen = screenName;

    final provider = _getProvider?.call();
    if (provider == null || !provider.isAvailable) {
      return;
    }

    final screenClass = _extractScreenClass(route);

    provider.logScreenView(
      screenName: screenName,
      screenClass: screenClass,
    );

    _log.fine('Logged screen view: $screenName');
  }

  /// Extracts a screen name from a route.
  String? _extractScreenName(Route<dynamic> route) {
    final settings = route.settings;

    // Try route name first
    if (settings.name != null && settings.name!.isNotEmpty) {
      return _sanitizeScreenName(settings.name!);
    }

    // Fall back to route type
    final typeName = route.runtimeType.toString();
    return typeName.replaceAll('Route', '').replaceAll('Page', '');
  }

  /// Extracts a screen class from a route.
  String? _extractScreenClass(Route<dynamic> route) {
    // Use the route's runtime type as the screen class
    return route.runtimeType.toString();
  }

  /// Sanitizes a route name to a clean screen name.
  ///
  /// This removes sensitive data like IDs and normalizes the format.
  String _sanitizeScreenName(String name) {
    // Remove leading slash
    var screenName = name.startsWith('/') ? name.substring(1) : name;

    // Remove query parameters (may contain sensitive data)
    screenName = screenName.split('?').first;

    // Replace path separators with underscores
    screenName = screenName.replaceAll('/', '_');

    // Replace UUIDs with placeholder to avoid logging user-specific data
    screenName = screenName.replaceAllMapped(
      RegExp(
        r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}',
        caseSensitive: false,
      ),
      (match) => 'id',
    );

    // Replace long alphanumeric segments (likely IDs).
    // Excludes '_' so joined path segments such as 'gear_detail_<id>' only
    // replace the ID portion, not the readable screen-name prefix.
    screenName = screenName.replaceAllMapped(
      RegExp(r'[a-zA-Z0-9-]{20,}'),
      (match) => 'id',
    );

    // Firebase Analytics screen names have a max of 100 chars
    if (screenName.length > 100) {
      screenName = screenName.substring(0, 100);
    }

    return screenName.isEmpty ? 'unknown' : screenName;
  }
}
