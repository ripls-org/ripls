import 'package:flutter/material.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/observability/logging/redactor.dart';

/// A NavigatorObserver that records navigation events as breadcrumbs.
///
/// This observer automatically tracks screen navigation for crash context.
/// Add it to your GoRouter or Navigator to capture navigation breadcrumbs.
///
/// Usage with GoRouter:
/// ```dart
/// GoRouter(
///   observers: [BreadcrumbNavigatorObserver()],
///   routes: [...],
/// )
/// ```
class BreadcrumbNavigatorObserver extends NavigatorObserver {
  final BreadcrumbBuffer _buffer;

  /// Creates a BreadcrumbNavigatorObserver.
  ///
  /// If [buffer] is not provided, uses the shared buffer from [LogContextHolder].
  BreadcrumbNavigatorObserver({BreadcrumbBuffer? buffer})
      : _buffer = buffer ?? LogContextHolder.breadcrumbBuffer;

  @override
  void didPush(Route<dynamic> route, Route<dynamic>? previousRoute) {
    _recordNavigation(
      'push',
      route,
      from: previousRoute,
    );
  }

  @override
  void didPop(Route<dynamic> route, Route<dynamic>? previousRoute) {
    _recordNavigation(
      'pop',
      previousRoute,
      from: route,
    );
  }

  @override
  void didReplace({Route<dynamic>? newRoute, Route<dynamic>? oldRoute}) {
    _recordNavigation(
      'replace',
      newRoute,
      from: oldRoute,
    );
  }

  @override
  void didRemove(Route<dynamic> route, Route<dynamic>? previousRoute) {
    _recordNavigation(
      'remove',
      route,
      from: previousRoute,
    );
  }

  /// Records a navigation event as a breadcrumb.
  void _recordNavigation(
    String action,
    Route<dynamic>? toRoute, {
    Route<dynamic>? from,
  }) {
    final toName = _extractRouteName(toRoute);
    final fromName = _extractRouteName(from);

    // Build message
    String message;
    if (action == 'pop') {
      message = toName != null ? 'Back to $toName' : 'Navigated back';
    } else if (action == 'push') {
      message = toName != null ? 'Navigated to $toName' : 'Navigated to new screen';
    } else if (action == 'replace') {
      message = toName != null ? 'Replaced with $toName' : 'Replaced screen';
    } else {
      message = toName != null ? 'Removed $toName' : 'Removed screen';
    }

    // Build data with redacted parameters
    final data = <String, dynamic>{
      'action': action,
      'to': ?toName,
      'from': ?fromName,
    };

    // Add route arguments if present (redacted)
    final arguments = toRoute?.settings.arguments;
    if (arguments != null && arguments is Map<String, dynamic>) {
      data['arguments'] = LogRedactor.redactMap(arguments);
    }

    _buffer.addNavigation(message, data: data);
  }

  /// Extracts a human-readable name from a route.
  String? _extractRouteName(Route<dynamic>? route) {
    if (route == null) return null;

    final settings = route.settings;

    // Try route name first
    if (settings.name != null && settings.name!.isNotEmpty) {
      return _sanitizeRouteName(settings.name!);
    }

    // Fall back to route type
    return route.runtimeType.toString();
  }

  /// Sanitizes a route name for logging.
  ///
  /// Removes query parameters and redacts path parameters that might contain IDs.
  String _sanitizeRouteName(String name) {
    // Remove query parameters
    final pathOnly = name.split('?').first;

    // Redact UUIDs and long IDs in path segments
    final segments = pathOnly.split('/');
    final sanitized = segments.map((segment) {
      // Check if segment looks like an ID (UUID or long alphanumeric)
      if (_looksLikeId(segment)) {
        return LogRedactor.maskIdentifier(segment);
      }
      return segment;
    }).join('/');

    return sanitized;
  }

  /// Checks if a string looks like an ID that should be redacted.
  bool _looksLikeId(String segment) {
    if (segment.isEmpty) return false;

    // UUID pattern (with or without dashes)
    final uuidPattern = RegExp(
      r'^[0-9a-f]{8}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{12}$',
      caseSensitive: false,
    );
    if (uuidPattern.hasMatch(segment)) return true;

    // Long alphanumeric strings (likely IDs)
    if (segment.length > 16 && RegExp(r'^[a-zA-Z0-9_-]+$').hasMatch(segment)) {
      return true;
    }

    return false;
  }
}
