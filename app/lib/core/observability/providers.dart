// Provider interfaces for observability features.
//
// These abstractions allow swapping implementations (e.g., Firebase to Sentry)
// without changing the rest of the application.

/// Breadcrumb represents a user action or app event for crash context.
class Breadcrumb {
  final String category;
  final String message;
  final Map<String, dynamic>? data;
  final BreadcrumbLevel level;
  final DateTime timestamp;

  Breadcrumb({
    required this.category,
    required this.message,
    this.data,
    this.level = BreadcrumbLevel.info,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();
}

/// Severity level for breadcrumbs.
enum BreadcrumbLevel {
  debug,
  info,
  warning,
  error,
}

/// Interface for crash reporting providers.
///
/// Implementations should handle crash capture, breadcrumb tracking,
/// and user context for crash reports.
abstract interface class CrashReporter {
  /// Initialize the crash reporter.
  Future<void> initialize();

  /// Record a non-fatal error with optional stack trace.
  Future<void> recordError(
    dynamic error, {
    StackTrace? stackTrace,
    String? reason,
    bool fatal = false,
  });

  /// Add a breadcrumb for crash context.
  void addBreadcrumb(Breadcrumb breadcrumb);

  /// Set user identifier for crash reports.
  void setUserId(String? userId);

  /// Set custom key-value pairs for crash reports.
  void setCustomKey(String key, dynamic value);

  /// Clear all user data and breadcrumbs.
  void clearUserData();

  /// Disable crash collection and delete unsent reports.
  ///
  /// Call this when user revokes consent.
  Future<void> disable();

  /// Send any unsent reports that were stored while offline.
  ///
  /// Call this after user grants consent to send any reports
  /// recorded while consent was pending.
  Future<void> sendUnsentReports();

  /// Check if crash reporting is available/initialized.
  bool get isAvailable;
}

/// Interface for performance monitoring providers.
///
/// Implementations should handle traces, spans, and performance metrics.
abstract interface class PerformanceMonitor {
  /// Initialize the performance monitor.
  Future<void> initialize();

  /// Start a new trace with the given name.
  PerformanceTrace startTrace(String name);

  /// Record a screen load trace.
  void recordScreenLoad(String screenName, Duration duration);

  /// Record a custom metric.
  void recordMetric(String name, double value, {Map<String, String>? attributes});

  /// Check if performance monitoring is available/initialized.
  bool get isAvailable;
}

/// A single performance trace for measuring operation duration.
abstract interface class PerformanceTrace {
  /// Start the trace.
  void start();

  /// Stop the trace and record the measurement.
  void stop();

  /// Add a metric to this trace.
  void putMetric(String name, int value);

  /// Add an attribute to this trace.
  void putAttribute(String name, String value);
}

/// Interface for analytics providers.
///
/// Implementations should handle event tracking, user properties,
/// and screen view tracking.
abstract interface class AnalyticsProvider {
  /// Initialize the analytics provider.
  Future<void> initialize();

  /// Log a custom event with optional parameters.
  Future<void> logEvent(String name, {Map<String, dynamic>? parameters});

  /// Log a screen view event.
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  });

  /// Set a user property.
  Future<void> setUserProperty(String name, String? value);

  /// Set the user ID for analytics.
  Future<void> setUserId(String? userId);

  /// Reset analytics state (e.g., on logout).
  Future<void> resetAnalyticsData();

  /// Disable analytics collection.
  ///
  /// Call this when user revokes consent.
  Future<void> disable();

  /// Check if analytics is available/initialized.
  bool get isAvailable;
}

/// No-op implementation of CrashReporter.
///
/// Used when crash reporting is disabled or not yet initialized.
class NoOpCrashReporter implements CrashReporter {
  @override
  Future<void> initialize() async {}

  @override
  Future<void> recordError(
    dynamic error, {
    StackTrace? stackTrace,
    String? reason,
    bool fatal = false,
  }) async {}

  @override
  void addBreadcrumb(Breadcrumb breadcrumb) {}

  @override
  void setUserId(String? userId) {}

  @override
  void setCustomKey(String key, dynamic value) {}

  @override
  void clearUserData() {}

  @override
  Future<void> disable() async {}

  @override
  Future<void> sendUnsentReports() async {}

  @override
  bool get isAvailable => false;
}

/// No-op implementation of PerformanceMonitor.
///
/// Used when performance monitoring is disabled or not yet initialized.
class NoOpPerformanceMonitor implements PerformanceMonitor {
  @override
  Future<void> initialize() async {}

  @override
  PerformanceTrace startTrace(String name) => _NoOpTrace();

  @override
  void recordScreenLoad(String screenName, Duration duration) {}

  @override
  void recordMetric(String name, double value, {Map<String, String>? attributes}) {}

  @override
  bool get isAvailable => false;
}

class _NoOpTrace implements PerformanceTrace {
  @override
  void start() {}

  @override
  void stop() {}

  @override
  void putMetric(String name, int value) {}

  @override
  void putAttribute(String name, String value) {}
}

/// No-op implementation of AnalyticsProvider.
///
/// Used when analytics is disabled or not yet initialized.
class NoOpAnalyticsProvider implements AnalyticsProvider {
  @override
  Future<void> initialize() async {}

  @override
  Future<void> logEvent(String name, {Map<String, dynamic>? parameters}) async {}

  @override
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  }) async {}

  @override
  Future<void> setUserProperty(String name, String? value) async {}

  @override
  Future<void> setUserId(String? userId) async {}

  @override
  Future<void> resetAnalyticsData() async {}

  @override
  Future<void> disable() async {}

  @override
  bool get isAvailable => false;
}
