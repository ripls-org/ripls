import 'package:firebase_analytics/firebase_analytics.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/providers.dart';

final _log = Logger('FirebaseAnalyticsProvider');

/// Firebase Analytics implementation of [AnalyticsProvider].
///
/// Wraps Firebase Analytics SDK to provide event tracking, screen views,
/// and user property management with consent support.
///
/// Usage:
/// ```dart
/// final provider = FirebaseAnalyticsProvider();
/// await provider.initialize();
///
/// // Log an event
/// await provider.logEvent('button_click', parameters: {'button_id': 'submit'});
///
/// // Log a screen view
/// await provider.logScreenView(screenName: 'HomeScreen');
///
/// // Set user ID
/// await provider.setUserId('user123');
/// ```
class FirebaseAnalyticsProvider implements AnalyticsProvider {
  final FirebaseAnalytics _analytics;
  bool _isInitialized = false;

  /// Creates a FirebaseAnalyticsProvider.
  ///
  /// If [analytics] is not provided, uses the default instance.
  FirebaseAnalyticsProvider({FirebaseAnalytics? analytics})
      : _analytics = analytics ?? FirebaseAnalytics.instance;

  @override
  bool get isAvailable => _isInitialized;

  @override
  Future<void> initialize() async {
    if (_isInitialized) {
      _log.warning('FirebaseAnalyticsProvider already initialized');
      return;
    }

    // Enable analytics collection
    await _analytics.setAnalyticsCollectionEnabled(true);

    _isInitialized = true;
    _log.info('FirebaseAnalyticsProvider initialized');
  }

  @override
  Future<void> logEvent(String name, {Map<String, dynamic>? parameters}) async {
    if (!_isInitialized) {
      _log.warning('Attempted to log event before initialization: $name');
      return;
    }

    try {
      // Convert parameters to the expected types
      final sanitizedParams = _sanitizeParameters(parameters);

      await _analytics.logEvent(
        name: name,
        parameters: sanitizedParams,
      );

      _log.fine('Logged event: $name');
    } catch (e, s) {
      _log.warning('Failed to log event: $name', e, s);
    }
  }

  @override
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  }) async {
    if (!_isInitialized) {
      _log.warning('Attempted to log screen view before initialization: $screenName');
      return;
    }

    try {
      await _analytics.logScreenView(
        screenName: screenName,
        screenClass: screenClass,
      );

      _log.fine('Logged screen view: $screenName');
    } catch (e, s) {
      _log.warning('Failed to log screen view: $screenName', e, s);
    }
  }

  @override
  Future<void> setUserId(String? userId) async {
    if (!_isInitialized) {
      _log.warning('Attempted to set user ID before initialization');
      return;
    }

    try {
      await _analytics.setUserId(id: userId);
      _log.fine('Set user ID: ${userId != null ? "[SET]" : "[CLEARED]"}');
    } catch (e, s) {
      _log.warning('Failed to set user ID', e, s);
    }
  }

  @override
  Future<void> setUserProperty(String name, String? value) async {
    if (!_isInitialized) {
      _log.warning('Attempted to set user property before initialization: $name');
      return;
    }

    try {
      await _analytics.setUserProperty(name: name, value: value);
      _log.fine('Set user property: $name=${value ?? "[CLEARED]"}');
    } catch (e, s) {
      _log.warning('Failed to set user property: $name', e, s);
    }
  }

  @override
  Future<void> resetAnalyticsData() async {
    if (!_isInitialized) {
      return;
    }

    try {
      await _analytics.resetAnalyticsData();
      _log.info('Reset analytics data');
    } catch (e, s) {
      _log.warning('Failed to reset analytics data', e, s);
    }
  }

  @override
  Future<void> disable() async {
    try {
      await _analytics.setAnalyticsCollectionEnabled(false);
      _isInitialized = false;
      _log.info('Analytics disabled');
    } catch (e, s) {
      _log.warning('Failed to disable analytics', e, s);
    }
  }

  /// Sanitizes event parameters for Firebase Analytics.
  ///
  /// Firebase Analytics has restrictions on parameter types:
  /// - String values max 100 characters
  /// - Numeric values (int, double)
  /// - Max 25 parameters per event
  Map<String, Object>? _sanitizeParameters(Map<String, dynamic>? parameters) {
    if (parameters == null || parameters.isEmpty) {
      return null;
    }

    final sanitized = <String, Object>{};
    var count = 0;

    for (final entry in parameters.entries) {
      if (count >= 25) {
        _log.warning('Event parameters exceed 25 limit, truncating');
        break;
      }

      final key = entry.key;
      final value = entry.value;

      if (value == null) {
        continue;
      }

      if (value is String) {
        // Truncate strings to 100 characters
        sanitized[key] = value.length > 100 ? value.substring(0, 100) : value;
      } else if (value is int) {
        sanitized[key] = value;
      } else if (value is double) {
        // Split from the int branch: `value is int || value is double` does
        // not promote, so `value` stays dynamic and the assignment becomes an
        // unchecked cast into Map<String, Object> (#2794).
        sanitized[key] = value;
      } else if (value is bool) {
        sanitized[key] = value.toString();
      } else {
        // Convert other types to string
        final stringValue = value.toString();
        sanitized[key] = stringValue.length > 100
            ? stringValue.substring(0, 100)
            : stringValue;
      }

      count++;
    }

    return sanitized.isEmpty ? null : sanitized;
  }
}
