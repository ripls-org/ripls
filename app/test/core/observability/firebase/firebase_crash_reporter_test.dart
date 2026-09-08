import 'package:firebase_crashlytics/firebase_crashlytics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/firebase/firebase_crash_reporter.dart';
import 'package:ripls/core/observability/providers.dart';

@GenerateMocks([FirebaseCrashlytics])
import 'firebase_crash_reporter_test.mocks.dart';

void main() {
  late MockFirebaseCrashlytics mockCrashlytics;
  late FirebaseCrashReporter reporter;

  setUp(() {
    mockCrashlytics = MockFirebaseCrashlytics();
    reporter = FirebaseCrashReporter(crashlytics: mockCrashlytics);

    // Setup default stubs
    when(mockCrashlytics.setCrashlyticsCollectionEnabled(any)).thenAnswer((
      _,
    ) async {});
    when(
      mockCrashlytics.recordError(
        any,
        any,
        reason: anyNamed('reason'),
        fatal: anyNamed('fatal'),
      ),
    ).thenAnswer((_) async {});
    when(mockCrashlytics.log(any)).thenAnswer((_) async {});
    when(mockCrashlytics.setUserIdentifier(any)).thenAnswer((_) async {});
    when(mockCrashlytics.setCustomKey(any, any)).thenAnswer((_) async {});
    when(mockCrashlytics.deleteUnsentReports()).thenAnswer((_) async {});
    when(mockCrashlytics.sendUnsentReports()).thenAnswer((_) async {});
  });

  group('FirebaseCrashReporter', () {
    group('initialization', () {
      test('isAvailable returns false before initialization', () {
        expect(reporter.isAvailable, isFalse);
      });

      test('initialize enables crashlytics collection', () async {
        await reporter.initialize();

        verify(mockCrashlytics.setCrashlyticsCollectionEnabled(true)).called(1);
        expect(reporter.isAvailable, isTrue);
      });

      test('initialize does nothing if already initialized', () async {
        await reporter.initialize();
        await reporter.initialize();

        // Should only be called once
        verify(mockCrashlytics.setCrashlyticsCollectionEnabled(true)).called(1);
      });

      test(
          'initialize fails soft when setCrashlyticsCollectionEnabled throws '
          '(e.g. no platform implementation, as on web)', () async {
        when(mockCrashlytics.setCrashlyticsCollectionEnabled(any))
            .thenThrow(Exception('no platform implementation'));

        // Must not throw — an uncaught error here takes down the whole app
        // (this exact failure mode reached production via the
        // consent-driven ObservabilityService path constructing a real
        // FirebaseCrashReporter on web; see observabilityServiceProvider).
        await reporter.initialize();

        expect(reporter.isAvailable, isFalse);
      });
    });

    group('recordError', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('records error with stack trace', () async {
        final error = Exception('test error');
        final stack = StackTrace.current;

        await reporter.recordError(error, stackTrace: stack);

        verify(
          mockCrashlytics.recordError(error, stack, reason: null, fatal: false),
        ).called(1);
      });

      test('records fatal error', () async {
        final error = Exception('fatal error');

        await reporter.recordError(error, fatal: true);

        verify(
          mockCrashlytics.recordError(error, any, reason: null, fatal: true),
        ).called(1);
      });

      test('redacts email in reason', () async {
        await reporter.recordError(
          Exception('error'),
          reason: 'User alice@example.com failed',
        );

        verify(
          mockCrashlytics.recordError(
            any,
            any,
            reason: argThat(
              allOf(
                contains('User'),
                contains('***'),
                isNot(contains('alice@example.com')),
              ),
              named: 'reason',
            ),
            fatal: false,
          ),
        ).called(1);
      });

      test('does nothing before initialization', () async {
        final uninitializedReporter = FirebaseCrashReporter(
          crashlytics: mockCrashlytics,
        );

        await uninitializedReporter.recordError(Exception('test'));

        verifyNever(mockCrashlytics.recordError(any, any));
      });
    });

    group('addBreadcrumb', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('logs breadcrumb with category and message', () {
        reporter.addBreadcrumb(
          Breadcrumb(category: 'navigation', message: 'Screen changed'),
        );

        verify(mockCrashlytics.log('[navigation] Screen changed')).called(1);
      });

      test('logs breadcrumb with data', () {
        reporter.addBreadcrumb(
          Breadcrumb(
            category: 'network',
            message: 'API call',
            data: {'endpoint': '/users', 'status': 200},
          ),
        );

        verify(
          mockCrashlytics.log(
            argThat(
              allOf(
                contains('[network] API call'),
                contains('endpoint=/users'),
                contains('status=200'),
              ),
            ),
          ),
        ).called(1);
      });

      test('redacts PII in breadcrumb data', () {
        reporter.addBreadcrumb(
          Breadcrumb(
            category: 'auth',
            message: 'Login attempt',
            data: {'email': 'alice@example.com'},
          ),
        );

        verify(
          mockCrashlytics.log(
            argThat(
              allOf(
                contains('[auth] Login attempt'),
                isNot(contains('alice@example.com')),
                contains('***'),
              ),
            ),
          ),
        ).called(1);
      });
    });

    group('setUserId', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('sets user identifier', () {
        reporter.setUserId('user123');

        verify(mockCrashlytics.setUserIdentifier('user123')).called(1);
      });

      test('clears user identifier with null', () {
        reporter.setUserId(null);

        verify(mockCrashlytics.setUserIdentifier('')).called(1);
      });
    });

    group('setCustomKey', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('sets custom key', () {
        reporter.setCustomKey('app_version', '1.0.0');

        verify(mockCrashlytics.setCustomKey('app_version', '1.0.0')).called(1);
      });

      test('redacts email values', () {
        reporter.setCustomKey('user_email', 'alice@example.com');

        verify(
          mockCrashlytics.setCustomKey(
            'user_email',
            argThat(allOf(isNot(equals('alice@example.com')), contains('***'))),
          ),
        ).called(1);
      });

      test('redacts token values', () {
        reporter.setCustomKey('auth_token', 'secret12345678');

        verify(
          mockCrashlytics.setCustomKey(
            'auth_token',
            argThat(startsWith('secret12')),
          ),
        ).called(1);
      });
    });

    group('clearUserData', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('clears user identifier and deletes unsent reports', () {
        reporter.clearUserData();

        verify(mockCrashlytics.setUserIdentifier('')).called(1);
        verify(mockCrashlytics.deleteUnsentReports()).called(1);
      });
    });

    group('disable', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('disables crashlytics collection', () async {
        await reporter.disable();

        verify(
          mockCrashlytics.setCrashlyticsCollectionEnabled(false),
        ).called(1);
        expect(reporter.isAvailable, isFalse);
      });
    });

    group('sendUnsentReports', () {
      setUp(() async {
        await reporter.initialize();
      });

      test('sends unsent reports', () async {
        await reporter.sendUnsentReports();

        verify(mockCrashlytics.sendUnsentReports()).called(1);
      });
    });

    group('breadcrumb buffer integration', () {
      late BreadcrumbBuffer buffer;

      setUp(() async {
        buffer = BreadcrumbBuffer(capacity: 5);
        reporter = FirebaseCrashReporter(
          crashlytics: mockCrashlytics,
          breadcrumbBuffer: buffer,
        );

        when(mockCrashlytics.setCrashlyticsCollectionEnabled(any)).thenAnswer((
          _,
        ) async {});
        when(
          mockCrashlytics.recordError(
            any,
            any,
            reason: anyNamed('reason'),
            fatal: anyNamed('fatal'),
          ),
        ).thenAnswer((_) async {});
        when(mockCrashlytics.log(any)).thenAnswer((_) async {});

        await reporter.initialize();
      });

      test('flushes breadcrumbs before recording error', () async {
        buffer.add(Breadcrumb(category: 'test', message: 'breadcrumb 1'));
        buffer.add(Breadcrumb(category: 'test', message: 'breadcrumb 2'));

        await reporter.recordError(Exception('error'));

        // Verify breadcrumbs were logged before error
        verifyInOrder([
          mockCrashlytics.log(argThat(contains('breadcrumb 1'))),
          mockCrashlytics.log(argThat(contains('breadcrumb 2'))),
          mockCrashlytics.recordError(
            any,
            any,
            reason: anyNamed('reason'),
            fatal: anyNamed('fatal'),
          ),
        ]);
      });

      test('does not flush when buffer is empty', () async {
        await reporter.recordError(Exception('error'));

        // No log calls for breadcrumbs
        verifyNever(mockCrashlytics.log(any));
        verify(
          mockCrashlytics.recordError(
            any,
            any,
            reason: anyNamed('reason'),
            fatal: anyNamed('fatal'),
          ),
        ).called(1);
      });
    });
  });
}
