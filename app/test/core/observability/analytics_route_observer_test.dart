import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/analytics_route_observer.dart';
import 'package:ripls/core/observability/providers.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('AnalyticsRouteObserver', () {
    late _MockAnalyticsProvider mockProvider;
    late AnalyticsRouteObserver observer;

    setUp(() {
      mockProvider = _MockAnalyticsProvider();
      observer = AnalyticsRouteObserver(
        getAnalyticsProvider: () => mockProvider,
      );
    });

    group('didPush', () {
      test('logs screen view when route is pushed', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/home');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('home'));
      });

      test('does not log screen view when provider is unavailable', () {
        mockProvider.isAvailable = false;
        final route = _MockRoute('/home');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, isEmpty);
      });

      test('sanitizes route name with path segments', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/gear/details');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_details'));
      });

      test('removes query parameters from route name', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/search?query=test&page=1');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('search'));
      });

      test('redacts UUIDs in route name', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/gear/123e4567-e89b-12d3-a456-426614174000');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_id'));
      });

      test('redacts long alphanumeric IDs in route name', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/gear/abc123def456789012345678');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_id'));
      });

      test('includes screen class from route type', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/home');

        observer.didPush(route, null);

        expect(mockProvider.lastScreenClass, contains('_MockRoute'));
      });
    });

    group('didReplace', () {
      test('logs screen view for new route', () {
        mockProvider.isAvailable = true;
        final newRoute = _MockRoute('/profile');
        final oldRoute = _MockRoute('/settings');

        observer.didReplace(newRoute: newRoute, oldRoute: oldRoute);

        expect(mockProvider.loggedScreenViews, contains('profile'));
      });

      test('does not log when new route is null', () {
        mockProvider.isAvailable = true;
        final oldRoute = _MockRoute('/settings');

        observer.didReplace(newRoute: null, oldRoute: oldRoute);

        expect(mockProvider.loggedScreenViews, isEmpty);
      });
    });

    group('didPop', () {
      test('logs screen view for previous route when popping', () {
        mockProvider.isAvailable = true;
        final poppedRoute = _MockRoute('/details');
        final previousRoute = _MockRoute('/home');

        observer.didPop(poppedRoute, previousRoute);

        expect(mockProvider.loggedScreenViews, contains('home'));
      });

      test('does not log when previous route is null', () {
        mockProvider.isAvailable = true;
        final poppedRoute = _MockRoute('/details');

        observer.didPop(poppedRoute, null);

        expect(mockProvider.loggedScreenViews, isEmpty);
      });
    });

    group('route name handling', () {
      test('handles empty route name', () {
        mockProvider.isAvailable = true;
        final route = _MockRoute('/');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('unknown'));
      });

      test('uses route type when name is empty', () {
        mockProvider.isAvailable = true;
        final route = _MockRouteWithoutName();

        observer.didPush(route, null);

        // Should fall back to runtime type
        expect(mockProvider.loggedScreenViews.isNotEmpty, isTrue);
      });

      test('prefers settings.name over route runtime type', () {
        mockProvider.isAvailable = true;
        // A route whose settings carry an explicit name should use that name
        // rather than the fallback runtime-type string (e.g. 'Builder<dynamic>').
        final route = _MockRoute('gear_detail');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_detail'));
        // Must not fall through to the runtime-type fallback
        expect(mockProvider.loggedScreenViews, isNot(contains('_MockRoute')));
      });

      test('sanitizes id-bearing path name to replace id segment', () {
        mockProvider.isAvailable = true;
        // GoRoute name 'gear_detail' paired with the path /gear/<uuid> — the
        // name itself has no ID, so no replacement should occur.
        final route = _MockRoute('gear_detail');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_detail'));
      });

      test('redacts id segment from path-style name', () {
        mockProvider.isAvailable = true;
        // When the route name itself contains a path with an alphanumeric ID
        // (e.g. from pushWithSlide using RouteSettings(name: 'gear_detail/abc123')),
        // the sanitiser should replace the ID segment.
        final route = _MockRoute('gear_detail/abc123def456789012345678');

        observer.didPush(route, null);

        expect(mockProvider.loggedScreenViews, contains('gear_detail_id'));
      });

      test('truncates long screen names to 100 characters', () {
        mockProvider.isAvailable = true;
        final longName = '/${'a' * 120}';
        final route = _MockRoute(longName);

        observer.didPush(route, null);

        final logged = mockProvider.loggedScreenViews.first;
        expect(logged.length, lessThanOrEqualTo(100));
      });
    });

    group('without provider', () {
      test('does not crash when no provider configured', () {
        final observerWithoutProvider = AnalyticsRouteObserver();
        final route = _MockRoute('/test');

        expect(() => observerWithoutProvider.didPush(route, null), returnsNormally);
      });
    });
  });
}

/// Mock route for testing.
class _MockRoute extends Route<dynamic> {
  _MockRoute(String name) : super(settings: RouteSettings(name: name));

  @override
  List<OverlayEntry> get overlayEntries => [];
}

/// Mock route without a name for testing fallback behavior.
class _MockRouteWithoutName extends Route<dynamic> {
  _MockRouteWithoutName() : super(settings: const RouteSettings());

  @override
  List<OverlayEntry> get overlayEntries => [];
}

/// Mock analytics provider for testing.
class _MockAnalyticsProvider implements AnalyticsProvider {
  @override
  bool isAvailable = false;
  final List<String> loggedScreenViews = [];
  String? lastScreenClass;
  final List<Map<String, dynamic>> loggedEvents = [];

  @override
  Future<void> initialize() async {
    isAvailable = true;
  }

  @override
  Future<void> logEvent(String name, {Map<String, dynamic>? parameters}) async {
    loggedEvents.add({'name': name, 'parameters': parameters});
  }

  @override
  Future<void> logScreenView({
    required String screenName,
    String? screenClass,
  }) async {
    loggedScreenViews.add(screenName);
    lastScreenClass = screenClass;
  }

  @override
  Future<void> setUserId(String? userId) async {}

  @override
  Future<void> setUserProperty(String name, String? value) async {}

  @override
  Future<void> resetAnalyticsData() async {}

  @override
  Future<void> disable() async {}
}
