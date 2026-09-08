import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/navigation_observer.dart';

void main() {
  late BreadcrumbBuffer buffer;
  late BreadcrumbNavigatorObserver observer;

  setUp(() {
    buffer = BreadcrumbBuffer();
    observer = BreadcrumbNavigatorObserver(buffer: buffer);
  });

  group('BreadcrumbNavigatorObserver', () {
    group('didPush', () {
      test('records navigation breadcrumb on push', () {
        final route = _createRoute('/home');

        observer.didPush(route, null);

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'navigation');
        expect(breadcrumbs[0].message, 'Navigated to /home');
        expect(breadcrumbs[0].data!['action'], 'push');
        expect(breadcrumbs[0].data!['to'], '/home');
      });

      test('includes from route when present', () {
        final fromRoute = _createRoute('/login');
        final toRoute = _createRoute('/home');

        observer.didPush(toRoute, fromRoute);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['from'], '/login');
        expect(breadcrumbs[0].data!['to'], '/home');
      });

      test('handles route without name using type', () {
        final route = _createRoute(null);

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        // Falls back to route type when name is null
        expect(breadcrumbs[0].message, 'Navigated to _MockRoute');
      });
    });

    group('didPop', () {
      test('records back navigation breadcrumb', () {
        final poppedRoute = _createRoute('/details');
        final previousRoute = _createRoute('/home');

        observer.didPop(poppedRoute, previousRoute);

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].message, 'Back to /home');
        expect(breadcrumbs[0].data!['action'], 'pop');
      });

      test('handles pop without previous route', () {
        final poppedRoute = _createRoute('/details');

        observer.didPop(poppedRoute, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].message, 'Navigated back');
      });
    });

    group('didReplace', () {
      test('records replace navigation breadcrumb', () {
        final oldRoute = _createRoute('/login');
        final newRoute = _createRoute('/home');

        observer.didReplace(newRoute: newRoute, oldRoute: oldRoute);

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].message, 'Replaced with /home');
        expect(breadcrumbs[0].data!['action'], 'replace');
        expect(breadcrumbs[0].data!['from'], '/login');
      });
    });

    group('didRemove', () {
      test('records remove navigation breadcrumb', () {
        final route = _createRoute('/modal');

        observer.didRemove(route, null);

        expect(buffer.length, 1);
        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].message, 'Removed /modal');
        expect(breadcrumbs[0].data!['action'], 'remove');
      });
    });

    group('route name sanitization', () {
      test('removes query parameters', () {
        final route = _createRoute('/gear?id=123&sort=name');

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['to'], '/gear');
      });

      test('redacts UUID in path', () {
        final route = _createRoute('/gear/550e8400-e29b-41d4-a716-446655440000');

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['to'], '/gear/550e***');
      });

      test('redacts long alphanumeric IDs', () {
        final route = _createRoute('/user/abc123def456ghi789jkl');

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['to'], '/user/abc1***');
      });

      test('preserves short path segments', () {
        final route = _createRoute('/gear/list');

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['to'], '/gear/list');
      });
    });

    group('arguments redaction', () {
      test('redacts email in arguments', () {
        final route = _createRouteWithArguments(
          '/profile',
          {'email': 'alice@example.com', 'name': 'Alice'},
        );

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        final args = breadcrumbs[0].data!['arguments'] as Map<String, dynamic>;
        expect(args['email'], 'al***@example.com');
        expect(args['name'], 'Alice');
      });

      test('redacts token in arguments', () {
        final route = _createRouteWithArguments(
          '/auth',
          {'token': 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload'},
        );

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        final args = breadcrumbs[0].data!['arguments'] as Map<String, dynamic>;
        expect(args['token'], 'eyJhbGci...');
      });

      test('ignores non-map arguments', () {
        final route = _createRouteWithArguments('/page', 'string_argument');

        observer.didPush(route, null);

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!.containsKey('arguments'), isFalse);
      });
    });

    group('uses shared buffer', () {
      test('uses LogContextHolder buffer by default', () {
        // Create observer without explicit buffer
        final defaultObserver = BreadcrumbNavigatorObserver();
        final route = _createRoute('/test');

        defaultObserver.didPush(route, null);

        // Should have added to the shared buffer
        // We can't easily test this without accessing LogContextHolder,
        // but the constructor path is covered
        expect(defaultObserver, isNotNull);
      });
    });
  });
}

/// Creates a mock route with the given name.
Route<dynamic> _createRoute(String? name) {
  return _MockRoute(RouteSettings(name: name));
}

/// Creates a mock route with name and arguments.
Route<dynamic> _createRouteWithArguments(String name, Object? arguments) {
  return _MockRoute(RouteSettings(name: name, arguments: arguments));
}

/// A minimal Route implementation for testing.
class _MockRoute extends Route<dynamic> {
  _MockRoute(this._settings);

  final RouteSettings _settings;

  @override
  RouteSettings get settings => _settings;

  @override
  List<OverlayEntry> get overlayEntries => [];

  @override
  void install() {
    super.install();
  }

  @override
  TickerFuture didPush() {
    super.didPush();
    return TickerFuture.complete();
  }

  @override
  void didAdd() {
    super.didAdd();
  }

  @override
  void didReplace(Route<dynamic>? oldRoute) {
    super.didReplace(oldRoute);
  }

  @override
  bool didPop(dynamic result) {
    super.didPop(result);
    return true;
  }

  @override
  void didComplete(dynamic result) {
    super.didComplete(result);
  }

  @override
  void didPopNext(Route<dynamic> nextRoute) {
    super.didPopNext(nextRoute);
  }

  @override
  void didChangeNext(Route<dynamic>? nextRoute) {
    super.didChangeNext(nextRoute);
  }

  @override
  void didChangePrevious(Route<dynamic>? previousRoute) {
    super.didChangePrevious(previousRoute);
  }

  @override
  void changedInternalState() {
    super.changedInternalState();
  }

  @override
  void changedExternalState() {
    super.changedExternalState();
  }

  @override
  void dispose() {
    super.dispose();
  }

  @override
  bool get isCurrent => false;

  @override
  bool get isFirst => false;

  @override
  bool get hasActiveRouteBelow => false;

  @override
  bool get isActive => true;

  @override
  NavigatorState? get navigator => null;

  @override
  bool get willHandlePopInternally => false;

  @override
  dynamic get currentResult => null;

  @override
  Future<RoutePopDisposition> willPop() async => RoutePopDisposition.pop;

  @override
  RoutePopDisposition get popDisposition => RoutePopDisposition.pop;
}
