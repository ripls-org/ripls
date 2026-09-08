import 'dart:async';

import 'package:firebase_crashlytics/firebase_crashlytics.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/logging/redactor.dart';
import 'package:ripls/core/observability/providers.dart';

final _log = Logger('FirebaseCrashReporter');

/// Firebase Crashlytics implementation of [CrashReporter].
///
/// Wraps Firebase Crashlytics to provide crash reporting functionality
/// with automatic PII redaction and breadcrumb support.
///
/// Usage:
/// ```dart
/// final reporter = FirebaseCrashReporter();
/// await reporter.initialize();
///
/// // Record an error
/// reporter.recordError(error, stackTrace: stackTrace);
///
/// // Add breadcrumb for context
/// reporter.addBreadcrumb(Breadcrumb(
///   category: 'network',
///   message: 'API call to /users',
/// ));
/// ```
class FirebaseCrashReporter implements CrashReporter {
  final FirebaseCrashlytics _crashlytics;
  final BreadcrumbBuffer? _breadcrumbBuffer;
  bool _isInitialized = false;

  /// Creates a FirebaseCrashReporter.
  ///
  /// If [crashlytics] is not provided, uses the default instance.
  /// If [breadcrumbBuffer] is provided, breadcrumbs will be flushed to
  /// Crashlytics logs when errors are recorded.
  FirebaseCrashReporter({
    FirebaseCrashlytics? crashlytics,
    BreadcrumbBuffer? breadcrumbBuffer,
  })  : _crashlytics = crashlytics ?? FirebaseCrashlytics.instance,
        _breadcrumbBuffer = breadcrumbBuffer;

  @override
  bool get isAvailable => _isInitialized;

  @override
  Future<void> initialize() async {
    if (_isInitialized) {
      _log.warning('FirebaseCrashReporter already initialized');
      return;
    }

    try {
      // Enable crash collection
      await _crashlytics.setCrashlyticsCollectionEnabled(true);

      _isInitialized = true;
      _log.info('FirebaseCrashReporter initialized');
    } catch (e, s) {
      // Firebase Crashlytics has no web implementation; callers are
      // expected to substitute a NoOpCrashReporter on web (see
      // observabilityServiceProvider), but fail soft here too so a
      // reporter constructed on an unsupported platform can't take down
      // the app with an uncaught error — consistent with every other
      // method on this class.
      _log.severe('Failed to initialize FirebaseCrashReporter', e, s);
    }
  }

  @override
  Future<void> recordError(
    dynamic error, {
    StackTrace? stackTrace,
    String? reason,
    bool fatal = false,
  }) async {
    if (!_isInitialized) {
      _log.warning('Attempted to record error before initialization');
      return;
    }

    try {
      // Flush breadcrumbs to Crashlytics logs before recording error
      _flushBreadcrumbs();

      // Redact any PII from the error message
      final redactedReason = reason != null ? _redactMessage(reason) : null;

      await _crashlytics.recordError(
        error,
        stackTrace ?? StackTrace.current,
        reason: redactedReason,
        fatal: fatal,
      );

      _log.fine(
        'Recorded ${fatal ? 'fatal' : 'non-fatal'} error: '
        '${error.runtimeType}${redactedReason != null ? ' ($redactedReason)' : ''}',
      );
    } catch (e, s) {
      // Don't let crash reporting errors crash the app
      _log.severe('Failed to record error', e, s);
    }
  }

  /// Flushes breadcrumbs from the buffer to Crashlytics logs.
  ///
  /// Sends all breadcrumbs in chronological order so they appear
  /// in the crash report with the most recent context.
  void _flushBreadcrumbs() {
    final buffer = _breadcrumbBuffer;
    if (buffer == null || buffer.isEmpty) return;

    try {
      // Get breadcrumbs in chronological order (oldest first)
      final breadcrumbs = buffer.toList();
      for (final breadcrumb in breadcrumbs) {
        addBreadcrumb(breadcrumb);
      }
      _log.fine('Flushed ${breadcrumbs.length} breadcrumbs to Crashlytics');
    } catch (e, s) {
      _log.warning('Failed to flush breadcrumbs', e, s);
    }
  }

  @override
  void addBreadcrumb(Breadcrumb breadcrumb) {
    if (!_isInitialized) {
      return;
    }

    try {
      // Crashlytics uses log() for breadcrumb-like messages
      // Format: [category] message (key=value, ...)
      final buffer = StringBuffer('[${breadcrumb.category}] ${breadcrumb.message}');

      if (breadcrumb.data != null && breadcrumb.data!.isNotEmpty) {
        // Redact PII from breadcrumb data
        final redactedData = LogRedactor.redactMap(breadcrumb.data!);
        final dataStr = redactedData.entries
            .map((e) => '${e.key}=${e.value}')
            .join(', ');
        buffer.write(' ($dataStr)');
      }

      _crashlytics.log(buffer.toString());
    } catch (e, s) {
      _log.warning('Failed to add breadcrumb', e, s);
    }
  }

  @override
  void setUserId(String? userId) {
    if (!_isInitialized) {
      return;
    }

    try {
      // Crashlytics setUserIdentifier accepts empty string to clear
      _crashlytics.setUserIdentifier(userId ?? '');
      _log.fine('Set user ID: ${userId != null ? LogRedactor.maskUserId(userId) : 'null'}');
    } catch (e, s) {
      _log.warning('Failed to set user ID', e, s);
    }
  }

  @override
  void setCustomKey(String key, dynamic value) {
    if (!_isInitialized) {
      return;
    }

    try {
      // Redact PII if the key suggests sensitive data
      final redactedValue = _redactCustomKeyValue(key, value);
      if (redactedValue == null) {
        // Crashlytics rejects a null value; log it here rather than letting
        // it surface as an opaque TypeError in the catch below.
        _log.warning('Ignoring null custom key value: $key');
        return;
      }
      _crashlytics.setCustomKey(key, redactedValue);
      _log.fine('Set custom key: $key=$redactedValue');
    } catch (e, s) {
      _log.warning('Failed to set custom key: $key', e, s);
    }
  }

  @override
  void clearUserData() {
    if (!_isInitialized) {
      return;
    }

    try {
      // Clear user identifier
      _crashlytics.setUserIdentifier('');

      // Delete any unsent reports
      _crashlytics.deleteUnsentReports();

      _log.info('Cleared user data and deleted unsent reports');
    } catch (e, s) {
      _log.warning('Failed to clear user data', e, s);
    }
  }

  @override
  Future<void> sendUnsentReports() async {
    if (!_isInitialized) {
      return;
    }

    try {
      await _crashlytics.sendUnsentReports();
      _log.info('Sent unsent reports');
    } catch (e, s) {
      _log.warning('Failed to send unsent reports', e, s);
    }
  }

  @override
  Future<void> disable() async {
    try {
      await _crashlytics.setCrashlyticsCollectionEnabled(false);
      _isInitialized = false;
      _log.info('Crash reporting disabled');
    } catch (e, s) {
      _log.warning('Failed to disable crash reporting', e, s);
    }
  }

  /// Redacts PII from a custom key value based on the key name.
  Object? _redactCustomKeyValue(String key, Object? value) {
    if (value is! String) {
      return value;
    }

    final lowerKey = key.toLowerCase();
    if (lowerKey.contains('email')) {
      return LogRedactor.maskEmail(value);
    } else if (lowerKey.contains('token') ||
        lowerKey.contains('password') ||
        lowerKey.contains('secret')) {
      return LogRedactor.maskToken(value);
    } else if (lowerKey.contains('phone')) {
      return LogRedactor.maskPhoneNumber(value);
    } else if (lowerKey.contains('user_id') || lowerKey.contains('userid')) {
      return LogRedactor.maskUserId(value);
    }

    return value;
  }

  /// Redacts common PII patterns from a message string.
  String _redactMessage(String message) {
    var redacted = message;

    // Redact email-like patterns
    redacted = redacted.replaceAllMapped(
      RegExp(r'[\w.+-]+@[\w.-]+\.\w+'),
      (match) => LogRedactor.maskEmail(match.group(0)!),
    );

    // Redact potential phone numbers (simple pattern)
    redacted = redacted.replaceAllMapped(
      RegExp(r'\+?\d{10,}'),
      (match) => LogRedactor.maskPhoneNumber(match.group(0)!),
    );

    return redacted;
  }
}
