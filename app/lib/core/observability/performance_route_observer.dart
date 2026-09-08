import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/providers.dart';

final _log = Logger('PerformanceRouteObserver');

/// A NavigatorObserver that tracks screen load performance.
///
/// This observer measures the time from route push to first frame render,
/// providing screen load time metrics for performance monitoring.
///
/// Usage with GoRouter:
/// ```dart
/// GoRouter(
///   observers: [
///     PerformanceRouteObserver(
///       getPerformanceMonitor: () => observabilityService.performanceMonitor,
///     ),
///   ],
///   routes: [...],
/// )
/// ```
class PerformanceRouteObserver extends NavigatorObserver {
  /// Function to get the current performance monitor.
  /// This allows the observer to get the consent-aware monitor.
  final PerformanceMonitor Function()? _getMonitor;

  /// Pending traces waiting for frame render completion.
  final Map<String, _PendingScreenTrace> _pendingTraces = {};

  /// Creates a PerformanceRouteObserver.
  ///
  /// [getPerformanceMonitor] is an optional function that returns the current
  /// performance monitor. If not provided, screen timing will not be recorded.
  PerformanceRouteObserver({
    PerformanceMonitor Function()? getPerformanceMonitor,
  }) : _getMonitor = getPerformanceMonitor;

  @override
  void didPush(Route<dynamic> route, Route<dynamic>? previousRoute) {
    _startScreenTrace(route);
  }

  @override
  void didReplace({Route<dynamic>? newRoute, Route<dynamic>? oldRoute}) {
    if (newRoute != null) {
      _startScreenTrace(newRoute);
    }
  }

  @override
  void didPop(Route<dynamic> route, Route<dynamic>? previousRoute) {
    // Cancel any pending trace for the popped route
    final routeId = _getRouteId(route);
    _pendingTraces.remove(routeId);
  }

  /// Starts tracking screen load time for a route.
  void _startScreenTrace(Route<dynamic> route) {
    final monitor = _getMonitor?.call();
    if (monitor == null || !monitor.isAvailable) {
      return;
    }

    final screenName = _extractScreenName(route);
    if (screenName == null) {
      return;
    }

    final routeId = _getRouteId(route);
    final startTime = DateTime.now();

    // Create a trace for this screen load
    final trace = monitor.startTrace('screen_$screenName');
    trace.start();
    trace.putAttribute('screen_name', screenName);

    // Store pending trace
    _pendingTraces[routeId] = _PendingScreenTrace(
      trace: trace,
      screenName: screenName,
      startTime: startTime,
    );

    // Schedule completion after the next frame is rendered
    SchedulerBinding.instance.addPostFrameCallback((_) {
      _completeScreenTrace(routeId);
    });
  }

  /// Completes a screen trace after the frame is rendered.
  void _completeScreenTrace(String routeId) {
    final pending = _pendingTraces.remove(routeId);
    if (pending == null) {
      return;
    }

    final duration = DateTime.now().difference(pending.startTime);
    pending.trace.putMetric('render_time_ms', duration.inMilliseconds);
    pending.trace.stop();

    _log.fine(
      'Screen ${pending.screenName} rendered in ${duration.inMilliseconds}ms',
    );
  }

  /// Extracts a screen name from a route.
  String? _extractScreenName(Route<dynamic> route) {
    final settings = route.settings;

    // Try route name first
    if (settings.name != null && settings.name!.isNotEmpty) {
      return _sanitizeScreenName(settings.name!);
    }

    // Fall back to route type, stripping common suffixes
    final typeName = route.runtimeType.toString();
    return typeName.replaceAll('Route', '').replaceAll('Page', '');
  }

  /// Sanitizes a route name to a clean screen name.
  String _sanitizeScreenName(String name) {
    // Remove leading slash
    var screenName = name.startsWith('/') ? name.substring(1) : name;

    // Remove query parameters
    screenName = screenName.split('?').first;

    // Replace path separators with underscores
    screenName = screenName.replaceAll('/', '_');

    // Replace UUIDs and long IDs with placeholder
    screenName = screenName.replaceAllMapped(
      RegExp(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', caseSensitive: false),
      (match) => 'id',
    );

    // Replace long alphanumeric segments (likely IDs)
    screenName = screenName.replaceAllMapped(
      RegExp(r'[a-zA-Z0-9_-]{20,}'),
      (match) => 'id',
    );

    // Limit length for Firebase (max 100 chars for trace names)
    if (screenName.length > 80) {
      screenName = screenName.substring(0, 80);
    }

    return screenName.isEmpty ? 'unknown' : screenName;
  }

  /// Gets a unique identifier for a route instance.
  String _getRouteId(Route<dynamic> route) {
    return '${route.hashCode}';
  }
}

/// Holds data for a pending screen trace.
class _PendingScreenTrace {
  final PerformanceTrace trace;
  final String screenName;
  final DateTime startTime;

  _PendingScreenTrace({
    required this.trace,
    required this.screenName,
    required this.startTime,
  });
}
