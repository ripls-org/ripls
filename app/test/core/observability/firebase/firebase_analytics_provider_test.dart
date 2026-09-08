import 'package:firebase_analytics/firebase_analytics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/observability/firebase/firebase_analytics_provider.dart';

@GenerateMocks([FirebaseAnalytics])
import 'firebase_analytics_provider_test.mocks.dart';

void main() {
  late MockFirebaseAnalytics mockAnalytics;
  late FirebaseAnalyticsProvider provider;

  setUp(() {
    mockAnalytics = MockFirebaseAnalytics();
    provider = FirebaseAnalyticsProvider(analytics: mockAnalytics);

    // Setup default stubs
    when(mockAnalytics.setAnalyticsCollectionEnabled(any)).thenAnswer((
      _,
    ) async {});
    when(
      mockAnalytics.logEvent(
        name: anyNamed('name'),
        parameters: anyNamed('parameters'),
      ),
    ).thenAnswer((_) async {});
    when(
      mockAnalytics.logScreenView(
        screenName: anyNamed('screenName'),
        screenClass: anyNamed('screenClass'),
      ),
    ).thenAnswer((_) async {});
    when(mockAnalytics.setUserId(id: anyNamed('id'))).thenAnswer((_) async {});
    when(
      mockAnalytics.setUserProperty(
        name: anyNamed('name'),
        value: anyNamed('value'),
      ),
    ).thenAnswer((_) async {});
    when(mockAnalytics.resetAnalyticsData()).thenAnswer((_) async {});
  });

  group('FirebaseAnalyticsProvider', () {
    group('initialization', () {
      test('isAvailable returns false before initialization', () {
        expect(provider.isAvailable, isFalse);
      });

      test('initialize enables analytics collection', () async {
        await provider.initialize();

        verify(mockAnalytics.setAnalyticsCollectionEnabled(true)).called(1);
        expect(provider.isAvailable, isTrue);
      });

      test('initialize does nothing if already initialized', () async {
        await provider.initialize();
        await provider.initialize();

        // Should only be called once
        verify(mockAnalytics.setAnalyticsCollectionEnabled(true)).called(1);
      });
    });

    group('logEvent', () {
      test('logs event after initialization', () async {
        await provider.initialize();

        await provider.logEvent('test_event', parameters: {'key': 'value'});

        verify(
          mockAnalytics.logEvent(
            name: 'test_event',
            parameters: {'key': 'value'},
          ),
        ).called(1);
      });

      test('does not log event before initialization', () async {
        await provider.logEvent('test_event');

        verifyNever(
          mockAnalytics.logEvent(
            name: anyNamed('name'),
            parameters: anyNamed('parameters'),
          ),
        );
      });

      test('logs event without parameters', () async {
        await provider.initialize();

        await provider.logEvent('simple_event');

        verify(
          mockAnalytics.logEvent(name: 'simple_event', parameters: null),
        ).called(1);
      });
    });

    group('logScreenView', () {
      test('logs screen view after initialization', () async {
        await provider.initialize();

        await provider.logScreenView(
          screenName: 'HomeScreen',
          screenClass: 'HomePage',
        );

        verify(
          mockAnalytics.logScreenView(
            screenName: 'HomeScreen',
            screenClass: 'HomePage',
          ),
        ).called(1);
      });

      test('does not log screen view before initialization', () async {
        await provider.logScreenView(screenName: 'TestScreen');

        verifyNever(
          mockAnalytics.logScreenView(
            screenName: anyNamed('screenName'),
            screenClass: anyNamed('screenClass'),
          ),
        );
      });

      test('logs screen view without screen class', () async {
        await provider.initialize();

        await provider.logScreenView(screenName: 'SettingsScreen');

        verify(
          mockAnalytics.logScreenView(
            screenName: 'SettingsScreen',
            screenClass: null,
          ),
        ).called(1);
      });
    });

    group('setUserId', () {
      test('sets user ID after initialization', () async {
        await provider.initialize();

        await provider.setUserId('user123');

        verify(mockAnalytics.setUserId(id: 'user123')).called(1);
      });

      test('clears user ID when null', () async {
        await provider.initialize();

        await provider.setUserId(null);

        verify(mockAnalytics.setUserId(id: null)).called(1);
      });

      test('does not set user ID before initialization', () async {
        await provider.setUserId('user123');

        verifyNever(mockAnalytics.setUserId(id: anyNamed('id')));
      });
    });

    group('setUserProperty', () {
      test('sets user property after initialization', () async {
        await provider.initialize();

        await provider.setUserProperty('plan', 'premium');

        verify(
          mockAnalytics.setUserProperty(name: 'plan', value: 'premium'),
        ).called(1);
      });

      test('clears user property when value is null', () async {
        await provider.initialize();

        await provider.setUserProperty('plan', null);

        verify(
          mockAnalytics.setUserProperty(name: 'plan', value: null),
        ).called(1);
      });

      test('does not set user property before initialization', () async {
        await provider.setUserProperty('plan', 'premium');

        verifyNever(
          mockAnalytics.setUserProperty(
            name: anyNamed('name'),
            value: anyNamed('value'),
          ),
        );
      });
    });

    group('resetAnalyticsData', () {
      test('resets analytics data after initialization', () async {
        await provider.initialize();

        await provider.resetAnalyticsData();

        verify(mockAnalytics.resetAnalyticsData()).called(1);
      });

      test('does not reset before initialization', () async {
        await provider.resetAnalyticsData();

        verifyNever(mockAnalytics.resetAnalyticsData());
      });
    });

    group('disable', () {
      test('disables analytics collection', () async {
        await provider.initialize();
        await provider.disable();

        verify(mockAnalytics.setAnalyticsCollectionEnabled(false)).called(1);
        expect(provider.isAvailable, isFalse);
      });
    });

    group('parameter sanitization', () {
      test('truncates string parameters longer than 100 chars', () async {
        await provider.initialize();
        final longValue = 'a' * 150;

        await provider.logEvent('test', parameters: {'key': longValue});

        verify(
          mockAnalytics.logEvent(name: 'test', parameters: {'key': 'a' * 100}),
        ).called(1);
      });

      test('passes through numeric parameters', () async {
        await provider.initialize();

        await provider.logEvent(
          'test',
          parameters: {'count': 42, 'price': 19.99},
        );

        verify(
          mockAnalytics.logEvent(
            name: 'test',
            parameters: {'count': 42, 'price': 19.99},
          ),
        ).called(1);
      });

      test('converts booleans to strings', () async {
        await provider.initialize();

        await provider.logEvent('test', parameters: {'enabled': true});

        verify(
          mockAnalytics.logEvent(name: 'test', parameters: {'enabled': 'true'}),
        ).called(1);
      });

      test('skips null parameter values', () async {
        await provider.initialize();

        await provider.logEvent(
          'test',
          parameters: {'valid': 'value', 'null_key': null},
        );

        verify(
          mockAnalytics.logEvent(name: 'test', parameters: {'valid': 'value'}),
        ).called(1);
      });

      test('truncates parameters to max 25', () async {
        await provider.initialize();
        final params = <String, dynamic>{};
        for (var i = 0; i < 30; i++) {
          params['key$i'] = 'value$i';
        }

        await provider.logEvent('test', parameters: params);

        final captured =
            verify(
                  mockAnalytics.logEvent(
                    name: 'test',
                    parameters: captureAnyNamed('parameters'),
                  ),
                ).captured.single
                as Map<String, Object>;

        expect(captured.length, equals(25));
      });

      test('converts other types to strings', () async {
        await provider.initialize();

        await provider.logEvent(
          'test',
          parameters: {
            'list': [1, 2, 3],
          },
        );

        verify(
          mockAnalytics.logEvent(
            name: 'test',
            parameters: {'list': '[1, 2, 3]'},
          ),
        ).called(1);
      });
    });
  });
}
