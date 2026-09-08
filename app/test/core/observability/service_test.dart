import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/core/observability/settings.dart';

void main() {
  group('ObservabilityService', () {
    group('initialization', () {
      test('starts with no-op providers', () {
        final service = ObservabilityService();

        expect(service.crashReportingEnabled, isFalse);
        expect(service.performanceMonitoringEnabled, isFalse);
        expect(service.analyticsEnabled, isFalse);
        expect(service.anyEnabled, isFalse);
        expect(service.isInitialized, isFalse);
      });

      test('initializes with consent settings', () async {
        final service = ObservabilityService(
          crashReporter: _MockCrashReporter(),
          performanceMonitor: _MockPerformanceMonitor(),
          analyticsProvider: _MockAnalyticsProvider(),
        );

        await service.initialize(ObservabilitySettings.allGranted);

        expect(service.crashReportingEnabled, isTrue);
        expect(service.performanceMonitoringEnabled, isTrue);
        expect(service.analyticsEnabled, isTrue);
        expect(service.anyEnabled, isTrue);
        expect(service.isInitialized, isTrue);
      });

      test('warns on double initialization', () async {
        final service = ObservabilityService();

        await service.initialize(ObservabilitySettings.notAsked);
        await service.initialize(ObservabilitySettings.allGranted);

        // Second initialization should be ignored
        expect(service.crashReportingEnabled, isFalse);
      });
    });

    group('consent updates', () {
      test('enables crash reporting when consent granted', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);

        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        expect(service.crashReportingEnabled, isTrue);
        expect(mockCrash.initializeCalled, isTrue);
      });

      test('disables crash reporting when consent denied', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);

        // First enable
        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        // Then disable
        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.denied,
          isLoading: false,
        ));

        expect(service.crashReportingEnabled, isFalse);
        expect(mockCrash.clearUserDataCalled, isTrue);
        expect(mockCrash.disableCalled, isTrue);
      });

      test('sends unsent reports when consent granted', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);

        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        expect(mockCrash.sendUnsentReportsCalled, isTrue);
      });

      test('enables performance monitoring when consent granted', () async {
        final mockPerf = _MockPerformanceMonitor();
        final service = ObservabilityService(performanceMonitor: mockPerf);

        await service.updateConsent(const ObservabilitySettings(
          performanceMonitoring: ObservabilityConsent.granted,
          isLoading: false,
        ));

        expect(service.performanceMonitoringEnabled, isTrue);
        expect(mockPerf.initializeCalled, isTrue);
      });

      test('enables analytics when consent granted', () async {
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(analyticsProvider: mockAnalytics);

        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.granted,
          isLoading: false,
        ));

        expect(service.analyticsEnabled, isTrue);
        expect(mockAnalytics.initializeCalled, isTrue);
      });

      test('resets analytics data when consent revoked', () async {
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(analyticsProvider: mockAnalytics);

        // First enable
        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.granted,
          isLoading: false,
        ));

        // Then disable
        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.denied,
          isLoading: false,
        ));

        expect(service.analyticsEnabled, isFalse);
        expect(mockAnalytics.resetAnalyticsDataCalled, isTrue);
      });
    });

    group('crash reporting methods', () {
      test('recordError delegates to crash reporter', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);
        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        await service.recordError(
          Exception('test error'),
          stackTrace: StackTrace.current,
          reason: 'test reason',
          fatal: true,
        );

        expect(mockCrash.recordedErrors, hasLength(1));
        expect(mockCrash.recordedErrors.first['fatal'], isTrue);
      });

      test('addBreadcrumb delegates to crash reporter', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);
        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        service.addBreadcrumb(
          category: 'navigation',
          message: 'User navigated to home',
          data: {'screen': 'home'},
          level: BreadcrumbLevel.info,
        );

        expect(mockCrash.breadcrumbs, hasLength(1));
        expect(mockCrash.breadcrumbs.first.category, 'navigation');
      });

      test('setCrashUserId delegates to crash reporter', () async {
        final mockCrash = _MockCrashReporter();
        final service = ObservabilityService(crashReporter: mockCrash);
        await service.updateConsent(const ObservabilitySettings(
          crashReporting: ObservabilityConsent.granted,
          isLoading: false,
        ));

        service.setCrashUserId('user123');

        expect(mockCrash.userId, 'user123');
      });
    });

    group('performance monitoring methods', () {
      test('startTrace delegates to performance monitor', () async {
        final mockPerf = _MockPerformanceMonitor();
        final service = ObservabilityService(performanceMonitor: mockPerf);
        await service.updateConsent(const ObservabilitySettings(
          performanceMonitoring: ObservabilityConsent.granted,
          isLoading: false,
        ));

        final trace = service.startTrace('test_trace');

        expect(trace, isA<PerformanceTrace>());
        expect(mockPerf.startedTraces, contains('test_trace'));
      });

      test('recordScreenLoad delegates to performance monitor', () async {
        final mockPerf = _MockPerformanceMonitor();
        final service = ObservabilityService(performanceMonitor: mockPerf);
        await service.updateConsent(const ObservabilitySettings(
          performanceMonitoring: ObservabilityConsent.granted,
          isLoading: false,
        ));

        service.recordScreenLoad('HomeScreen', const Duration(milliseconds: 500));

        expect(mockPerf.screenLoads, hasLength(1));
        expect(mockPerf.screenLoads.first['screenName'], 'HomeScreen');
      });
    });

    group('analytics methods', () {
      test('logEvent delegates to analytics provider', () async {
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(analyticsProvider: mockAnalytics);
        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.granted,
          isLoading: false,
        ));

        await service.logEvent('button_click', parameters: {'button_id': 'submit'});

        expect(mockAnalytics.loggedEvents, hasLength(1));
        expect(mockAnalytics.loggedEvents.first['name'], 'button_click');
      });

      test('logScreenView delegates to analytics provider', () async {
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(analyticsProvider: mockAnalytics);
        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.granted,
          isLoading: false,
        ));

        await service.logScreenView(screenName: 'HomeScreen');

        expect(mockAnalytics.screenViews, hasLength(1));
        expect(mockAnalytics.screenViews.first['screenName'], 'HomeScreen');
      });

      test('setUserProperty delegates to analytics provider', () async {
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(analyticsProvider: mockAnalytics);
        await service.updateConsent(const ObservabilitySettings(
          analytics: ObservabilityConsent.granted,
          isLoading: false,
        ));

        await service.setUserProperty('premium_user', 'true');

        expect(mockAnalytics.userProperties, containsPair('premium_user', 'true'));
      });
    });

    group('combined methods', () {
      test('setUserId sets ID on both crash and analytics', () async {
        final mockCrash = _MockCrashReporter();
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(
          crashReporter: mockCrash,
          analyticsProvider: mockAnalytics,
        );
        await service.updateConsent(ObservabilitySettings.allGranted);

        await service.setUserId('user123');

        expect(mockCrash.userId, 'user123');
        expect(mockAnalytics.userId, 'user123');
      });

      test('clearUserData clears both crash and analytics', () async {
        final mockCrash = _MockCrashReporter();
        final mockAnalytics = _MockAnalyticsProvider();
        final service = ObservabilityService(
          crashReporter: mockCrash,
          analyticsProvider: mockAnalytics,
        );
        await service.updateConsent(ObservabilitySettings.allGranted);

        await service.clearUserData();

        expect(mockCrash.clearUserDataCalled, isTrue);
        expect(mockAnalytics.resetAnalyticsDataCalled, isTrue);
      });
    });

    group('no-op behavior when disabled', () {
      test('recordError does nothing when disabled', () async {
        final service = ObservabilityService();

        // Should not throw
        await expectLater(
          service.recordError(Exception('test')),
          completes,
        );
      });

      test('logEvent does nothing when disabled', () async {
        final service = ObservabilityService();

        // Should not throw
        await expectLater(
          service.logEvent('test_event'),
          completes,
        );
      });

      test('startTrace returns no-op trace when disabled', () {
        final service = ObservabilityService();

        final trace = service.startTrace('test');
        expect(() => trace.start(), returnsNormally);
        expect(() => trace.stop(), returnsNormally);
      });
    });
  });
}

// Mock implementations for testing

class _MockCrashReporter implements CrashReporter {
  bool initializeCalled = false;
  bool clearUserDataCalled = false;
  bool disableCalled = false;
  bool sendUnsentReportsCalled = false;
  String? userId;
  final List<Map<String, dynamic>> recordedErrors = [];
  final List<Breadcrumb> breadcrumbs = [];
  final Map<String, dynamic> customKeys = {};

  @override
  Future<void> initialize() async {
    initializeCalled = true;
  }

  @override
  Future<void> recordError(
    dynamic error, {
    StackTrace? stackTrace,
    String? reason,
    bool fatal = false,
  }) async {
    recordedErrors.add({
      'error': error,
      'stackTrace': stackTrace,
      'reason': reason,
      'fatal': fatal,
    });
  }

  @override
  void addBreadcrumb(Breadcrumb breadcrumb) {
    breadcrumbs.add(breadcrumb);
  }

  @override
  void setUserId(String? id) {
    userId = id;
  }

  @override
  void setCustomKey(String key, dynamic value) {
    customKeys[key] = value;
  }

  @override
  void clearUserData() {
    clearUserDataCalled = true;
    userId = null;
    breadcrumbs.clear();
  }

  @override
  Future<void> disable() async {
    disableCalled = true;
    initializeCalled = false;
  }

  @override
  Future<void> sendUnsentReports() async {
    sendUnsentReportsCalled = true;
  }

  @override
  bool get isAvailable => initializeCalled;
}

class _MockPerformanceMonitor implements PerformanceMonitor {
  bool initializeCalled = false;
  final List<String> startedTraces = [];
  final List<Map<String, dynamic>> screenLoads = [];
  final List<Map<String, dynamic>> metrics = [];

  @override
  Future<void> initialize() async {
    initializeCalled = true;
  }

  @override
  PerformanceTrace startTrace(String name) {
    startedTraces.add(name);
    return _MockTrace(name);
  }

  @override
  void recordScreenLoad(String screenName, Duration duration) {
    screenLoads.add({
      'screenName': screenName,
      'duration': duration,
    });
  }

  @override
  void recordMetric(String name, double value, {Map<String, String>? attributes}) {
    metrics.add({
      'name': name,
      'value': value,
      'attributes': attributes,
    });
  }

  @override
  bool get isAvailable => initializeCalled;
}

class _MockTrace implements PerformanceTrace {
  final String name;
  bool started = false;
  bool stopped = false;

  _MockTrace(this.name);

  @override
  void start() {
    started = true;
  }

  @override
  void stop() {
    stopped = true;
  }

  @override
  void putMetric(String name, int value) {}

  @override
  void putAttribute(String name, String value) {}
}

class _MockAnalyticsProvider implements AnalyticsProvider {
  bool initializeCalled = false;
  bool resetAnalyticsDataCalled = false;
  bool disableCalled = false;
  String? userId;
  final List<Map<String, dynamic>> loggedEvents = [];
  final List<Map<String, dynamic>> screenViews = [];
  final Map<String, String?> userProperties = {};

  @override
  Future<void> initialize() async {
    initializeCalled = true;
  }

  @override
  Future<void> logEvent(String name, {Map<String, dynamic>? parameters}) async {
    loggedEvents.add({
      'name': name,
      'parameters': parameters,
    });
  }

  @override
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  }) async {
    screenViews.add({
      'screenName': screenName,
      'screenClass': screenClass,
    });
  }

  @override
  Future<void> setUserProperty(String name, String? value) async {
    userProperties[name] = value;
  }

  @override
  Future<void> setUserId(String? id) async {
    userId = id;
  }

  @override
  Future<void> resetAnalyticsData() async {
    resetAnalyticsDataCalled = true;
    loggedEvents.clear();
    screenViews.clear();
    userProperties.clear();
    userId = null;
  }

  @override
  Future<void> disable() async {
    disableCalled = true;
    initializeCalled = false;
  }

  @override
  bool get isAvailable => initializeCalled;
}
