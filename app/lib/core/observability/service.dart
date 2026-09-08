import 'package:logging/logging.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/observability/settings.dart';

final _log = Logger('ObservabilityService');

/// ObservabilityService is the unified facade for all observability features.
///
/// It coordinates crash reporting, performance monitoring, and analytics
/// based on user consent settings. When a feature is disabled, the service
/// uses a no-op implementation to avoid data collection.
///
/// Usage:
/// ```dart
/// final service = ObservabilityService(
///   crashReporter: FirebaseCrashReporter(),
///   performanceMonitor: FirebasePerformanceMonitor(),
///   analyticsProvider: FirebaseAnalytics(),
/// );
///
/// // Update consent and reconfigure providers
/// await service.updateConsent(settings);
///
/// // Use the service
/// service.recordError(error, stackTrace: trace);
/// service.logEvent('button_tap', parameters: {'button_id': 'submit'});
/// ```
class ObservabilityService {
  /// The crash reporter implementation (or no-op if disabled).
  CrashReporter _crashReporter;

  /// The performance monitor implementation (or no-op if disabled).
  PerformanceMonitor _performanceMonitor;

  /// The analytics provider implementation (or no-op if disabled).
  AnalyticsProvider _analyticsProvider;

  /// The actual implementations to use when consent is granted.
  final CrashReporter? _realCrashReporter;
  final PerformanceMonitor? _realPerformanceMonitor;
  final AnalyticsProvider? _realAnalyticsProvider;

  /// Current consent settings.
  ObservabilitySettings _settings = ObservabilitySettings.notAsked;

  /// Whether the service has been initialized.
  bool _isInitialized = false;

  ObservabilityService({
    CrashReporter? crashReporter,
    PerformanceMonitor? performanceMonitor,
    AnalyticsProvider? analyticsProvider,
  })  : _realCrashReporter = crashReporter,
        _realPerformanceMonitor = performanceMonitor,
        _realAnalyticsProvider = analyticsProvider,
        _crashReporter = NoOpCrashReporter(),
        _performanceMonitor = NoOpPerformanceMonitor(),
        _analyticsProvider = NoOpAnalyticsProvider();

  /// Current consent settings.
  ObservabilitySettings get settings => _settings;

  /// Whether crash reporting is currently enabled.
  bool get crashReportingEnabled =>
      _settings.crashReporting == ObservabilityConsent.granted;

  /// Whether performance monitoring is currently enabled.
  bool get performanceMonitoringEnabled =>
      _settings.performanceMonitoring == ObservabilityConsent.granted;

  /// Whether analytics is currently enabled.
  bool get analyticsEnabled =>
      _settings.analytics == ObservabilityConsent.granted;

  /// Whether any observability feature is enabled.
  bool get anyEnabled => _settings.anyEnabled;

  /// Whether the service has been initialized.
  bool get isInitialized => _isInitialized;

  /// Gets the current performance monitor (consent-aware).
  PerformanceMonitor get performanceMonitor => _performanceMonitor;

  /// Gets the current analytics provider (consent-aware).
  AnalyticsProvider get analyticsProvider => _analyticsProvider;

  /// Initialize the service with the given consent settings.
  ///
  /// This should be called on app startup after loading user consent.
  Future<void> initialize(ObservabilitySettings settings) async {
    if (_isInitialized) {
      _log.warning('ObservabilityService already initialized');
      return;
    }

    _log.info('Initializing ObservabilityService with settings: $settings');
    await updateConsent(settings);
    _isInitialized = true;
  }

  /// Update consent settings and reconfigure providers accordingly.
  ///
  /// When consent is granted, switches to real implementations.
  /// When consent is denied, switches to no-op implementations.
  Future<void> updateConsent(ObservabilitySettings settings) async {
    final oldSettings = _settings;
    _settings = settings;

    // Handle crash reporting consent change
    if (settings.crashReporting != oldSettings.crashReporting) {
      await _updateCrashReporter(settings.crashReporting);
    }

    // Handle performance monitoring consent change
    if (settings.performanceMonitoring != oldSettings.performanceMonitoring) {
      await _updatePerformanceMonitor(settings.performanceMonitoring);
    }

    // Handle analytics consent change
    if (settings.analytics != oldSettings.analytics) {
      await _updateAnalyticsProvider(settings.analytics);
    }

    _log.info('Consent updated: $settings');
  }

  Future<void> _updateCrashReporter(ObservabilityConsent consent) async {
    final realReporter = _realCrashReporter;
    if (consent == ObservabilityConsent.granted && realReporter != null) {
      _log.info('Enabling crash reporting');
      await realReporter.initialize();
      _crashReporter = realReporter;
      // Send any reports that were recorded while consent was pending
      await realReporter.sendUnsentReports();
    } else {
      _log.info('Disabling crash reporting');
      if (_crashReporter == realReporter && realReporter != null) {
        // Currently using real reporter, need to disable it
        _crashReporter.clearUserData();
        await realReporter.disable();
      }
      _crashReporter = NoOpCrashReporter();
    }
  }

  Future<void> _updatePerformanceMonitor(ObservabilityConsent consent) async {
    final realMonitor = _realPerformanceMonitor;
    if (consent == ObservabilityConsent.granted && realMonitor != null) {
      _log.info('Enabling performance monitoring');
      await realMonitor.initialize();
      _performanceMonitor = realMonitor;
    } else {
      _log.info('Disabling performance monitoring');
      _performanceMonitor = NoOpPerformanceMonitor();
    }
  }

  Future<void> _updateAnalyticsProvider(ObservabilityConsent consent) async {
    final realProvider = _realAnalyticsProvider;
    if (consent == ObservabilityConsent.granted && realProvider != null) {
      _log.info('Enabling analytics');
      await realProvider.initialize();
      _analyticsProvider = realProvider;
    } else {
      _log.info('Disabling analytics');
      if (_analyticsProvider == realProvider && realProvider != null) {
        await realProvider.resetAnalyticsData();
        await realProvider.disable();
      }
      _analyticsProvider = NoOpAnalyticsProvider();
    }
  }

  // ===== Crash Reporting Methods =====

  /// Record an error with optional stack trace.
  Future<void> recordError(
    dynamic error, {
    StackTrace? stackTrace,
    String? reason,
    bool fatal = false,
  }) async {
    await _crashReporter.recordError(
      error,
      stackTrace: stackTrace,
      reason: reason,
      fatal: fatal,
    );
  }

  /// Add a breadcrumb for crash context.
  void addBreadcrumb({
    required String category,
    required String message,
    Map<String, dynamic>? data,
    BreadcrumbLevel level = BreadcrumbLevel.info,
  }) {
    _crashReporter.addBreadcrumb(Breadcrumb(
      category: category,
      message: message,
      data: data,
      level: level,
    ));
  }

  /// Set user identifier for crash reports.
  void setCrashUserId(String? userId) {
    _crashReporter.setUserId(userId);
  }

  
  // ===== Performance Monitoring Methods =====

  /// Start a performance trace.
  PerformanceTrace startTrace(String name) {
    return _performanceMonitor.startTrace(name);
  }

  /// Record a screen load duration.
  void recordScreenLoad(String screenName, Duration duration) {
    _performanceMonitor.recordScreenLoad(screenName, duration);
  }

  /// Record a custom metric.
  void recordMetric(
    String name,
    double value, {
    Map<String, String>? attributes,
  }) {
    _performanceMonitor.recordMetric(name, value, attributes: attributes);
  }

  // ===== Analytics Methods =====

  /// Log a custom event.
  Future<void> logEvent(
    String name, {
    Map<String, dynamic>? parameters,
  }) async {
    await _analyticsProvider.logEvent(name, parameters: parameters);
  }

  /// Log a typed analytics event.
  ///
  /// Usage:
  /// ```dart
  /// service.logAnalyticsEvent(GearViewedEvent(gearId: 'gear123'));
  /// ```
  Future<void> logAnalyticsEvent(AnalyticsEvent event) async {
    await _analyticsProvider.logEvent(event.name, parameters: event.parameters);
  }

  /// Log a screen view.
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  }) async {
    await _analyticsProvider.logScreenView(
      screenName: screenName,
      screenClass: screenClass,
    );
  }

  /// Set a user property.
  Future<void> setUserProperty(String name, String? value) async {
    await _analyticsProvider.setUserProperty(name, value);
  }

  /// Set the user ID for analytics.
  Future<void> setAnalyticsUserId(String? userId) async {
    await _analyticsProvider.setUserId(userId);
  }

  // ===== Combined Methods =====

  /// Set the user ID across all observability services.
  Future<void> setUserId(String? userId) async {
    setCrashUserId(userId);
    await setAnalyticsUserId(userId);
  }

  /// Clear all user data from observability services.
  ///
  /// Call this on logout to ensure user data is not retained.
  Future<void> clearUserData() async {
    _crashReporter.clearUserData();
    await _analyticsProvider.resetAnalyticsData();
    _log.info('Cleared all user data from observability services');
  }
}
