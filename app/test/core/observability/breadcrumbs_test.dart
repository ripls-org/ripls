import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/providers.dart';

void main() {
  group('BreadcrumbCategory', () {
    test('has all expected categories', () {
      expect(BreadcrumbCategory.values, hasLength(6));
      expect(BreadcrumbCategory.navigation.value, 'navigation');
      expect(BreadcrumbCategory.userAction.value, 'user_action');
      expect(BreadcrumbCategory.network.value, 'network');
      expect(BreadcrumbCategory.error.value, 'error');
      expect(BreadcrumbCategory.stateChange.value, 'state_change');
      expect(BreadcrumbCategory.auth.value, 'auth');
    });
  });

  group('BreadcrumbBuffer', () {
    late BreadcrumbBuffer buffer;

    setUp(() {
      buffer = BreadcrumbBuffer();
    });

    group('capacity and initialization', () {
      test('has default capacity of 100', () {
        expect(buffer.capacity, 100);
      });

      test('accepts custom capacity', () {
        final customBuffer = BreadcrumbBuffer(capacity: 50);
        expect(customBuffer.capacity, 50);
      });

      test('uses default capacity for invalid values', () {
        final zeroCapacity = BreadcrumbBuffer(capacity: 0);
        expect(zeroCapacity.capacity, BreadcrumbBuffer.defaultCapacity);

        final negativeCapacity = BreadcrumbBuffer(capacity: -5);
        expect(negativeCapacity.capacity, BreadcrumbBuffer.defaultCapacity);
      });

      test('starts empty', () {
        expect(buffer.isEmpty, isTrue);
        expect(buffer.length, 0);
        expect(buffer.isFull, isFalse);
      });
    });

    group('add', () {
      test('adds breadcrumb to buffer', () {
        buffer.add(Breadcrumb(category: 'test', message: 'test message'));

        expect(buffer.length, 1);
        expect(buffer.isEmpty, isFalse);
      });

      test('preserves breadcrumb properties', () {
        final timestamp = DateTime(2025, 1, 15, 10, 30);
        buffer.add(Breadcrumb(
          category: 'navigation',
          message: 'Navigated to home',
          data: {'screen': 'HomeScreen'},
          level: BreadcrumbLevel.info,
          timestamp: timestamp,
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs, hasLength(1));
        expect(breadcrumbs[0].category, 'navigation');
        expect(breadcrumbs[0].message, 'Navigated to home');
        expect(breadcrumbs[0].data, {'screen': 'HomeScreen'});
        expect(breadcrumbs[0].level, BreadcrumbLevel.info);
        expect(breadcrumbs[0].timestamp, timestamp);
      });

      test('removes oldest when at capacity', () {
        final smallBuffer = BreadcrumbBuffer(capacity: 3);

        smallBuffer.add(Breadcrumb(category: 'test', message: 'first'));
        smallBuffer.add(Breadcrumb(category: 'test', message: 'second'));
        smallBuffer.add(Breadcrumb(category: 'test', message: 'third'));
        expect(smallBuffer.isFull, isTrue);

        smallBuffer.add(Breadcrumb(category: 'test', message: 'fourth'));

        expect(smallBuffer.length, 3);
        final messages = smallBuffer.toList().map((b) => b.message).toList();
        expect(messages, ['second', 'third', 'fourth']);
      });
    });

    group('PII redaction', () {
      test('redacts email in data', () {
        buffer.add(Breadcrumb(
          category: 'auth',
          message: 'User logged in',
          data: {'email': 'alice@example.com'},
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['email'], 'al***@example.com');
      });

      test('redacts token in data', () {
        buffer.add(Breadcrumb(
          category: 'auth',
          message: 'Token refreshed',
          data: {'token': 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload'},
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['token'], 'eyJhbGci...');
      });

      test('redacts phone in data', () {
        buffer.add(Breadcrumb(
          category: 'user_action',
          message: 'Phone verified',
          data: {'phone': '+15551234567'},
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data!['phone'], '+***4567');
      });

      test('redacts nested PII', () {
        buffer.add(Breadcrumb(
          category: 'auth',
          message: 'Profile updated',
          data: {
            'user': {
              'email': 'alice@example.com',
              'name': 'Alice',
            },
          },
        ));

        final breadcrumbs = buffer.toList();
        final userData = breadcrumbs[0].data!['user'] as Map;
        expect(userData['email'], 'al***@example.com');
        expect(userData['name'], 'Alice');
      });

      test('preserves breadcrumb without data', () {
        buffer.add(Breadcrumb(
          category: 'navigation',
          message: 'Navigated to home',
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data, isNull);
      });

      test('preserves breadcrumb with empty data', () {
        buffer.add(Breadcrumb(
          category: 'navigation',
          message: 'Navigated to home',
          data: {},
        ));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].data, isEmpty);
      });
    });

    group('addWithCategory', () {
      test('creates breadcrumb with category enum', () {
        buffer.addWithCategory(
          category: BreadcrumbCategory.navigation,
          message: 'Screen changed',
        );

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'navigation');
        expect(breadcrumbs[0].message, 'Screen changed');
      });

      test('accepts all optional parameters', () {
        final timestamp = DateTime(2025, 1, 15);
        buffer.addWithCategory(
          category: BreadcrumbCategory.error,
          message: 'Error occurred',
          data: {'code': 500},
          level: BreadcrumbLevel.error,
          timestamp: timestamp,
        );

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs[0].category, 'error');
        expect(breadcrumbs[0].data, {'code': 500});
        expect(breadcrumbs[0].level, BreadcrumbLevel.error);
        expect(breadcrumbs[0].timestamp, timestamp);
      });
    });

    group('toList', () {
      test('returns breadcrumbs in chronological order', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));
        buffer.add(Breadcrumb(category: 'test', message: 'second'));
        buffer.add(Breadcrumb(category: 'test', message: 'third'));

        final breadcrumbs = buffer.toList();
        expect(breadcrumbs.map((b) => b.message), ['first', 'second', 'third']);
      });

      test('returns empty list for empty buffer', () {
        expect(buffer.toList(), isEmpty);
      });
    });

    group('toReversedList', () {
      test('returns breadcrumbs in reverse chronological order', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));
        buffer.add(Breadcrumb(category: 'test', message: 'second'));
        buffer.add(Breadcrumb(category: 'test', message: 'third'));

        final breadcrumbs = buffer.toReversedList();
        expect(breadcrumbs.map((b) => b.message), ['third', 'second', 'first']);
      });
    });

    group('takeLast', () {
      test('returns last N breadcrumbs', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));
        buffer.add(Breadcrumb(category: 'test', message: 'second'));
        buffer.add(Breadcrumb(category: 'test', message: 'third'));
        buffer.add(Breadcrumb(category: 'test', message: 'fourth'));

        final breadcrumbs = buffer.takeLast(2);
        expect(breadcrumbs.map((b) => b.message), ['third', 'fourth']);
      });

      test('returns all if count exceeds length', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));
        buffer.add(Breadcrumb(category: 'test', message: 'second'));

        final breadcrumbs = buffer.takeLast(10);
        expect(breadcrumbs, hasLength(2));
      });

      test('returns empty list for count <= 0', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));

        expect(buffer.takeLast(0), isEmpty);
        expect(buffer.takeLast(-1), isEmpty);
      });
    });

    group('whereCategory', () {
      test('filters by category', () {
        buffer.add(Breadcrumb(category: 'navigation', message: 'nav1'));
        buffer.add(Breadcrumb(category: 'error', message: 'error1'));
        buffer.add(Breadcrumb(category: 'navigation', message: 'nav2'));

        final navBreadcrumbs = buffer.whereCategory('navigation');
        expect(navBreadcrumbs, hasLength(2));
        expect(navBreadcrumbs.map((b) => b.message), ['nav1', 'nav2']);
      });

      test('returns empty list for non-existent category', () {
        buffer.add(Breadcrumb(category: 'navigation', message: 'nav1'));

        expect(buffer.whereCategory('unknown'), isEmpty);
      });
    });

    group('whereLevel', () {
      test('filters by minimum level', () {
        buffer.add(Breadcrumb(
            category: 'test', message: 'debug', level: BreadcrumbLevel.debug));
        buffer.add(Breadcrumb(
            category: 'test', message: 'info', level: BreadcrumbLevel.info));
        buffer.add(Breadcrumb(
            category: 'test', message: 'warning', level: BreadcrumbLevel.warning));
        buffer.add(Breadcrumb(
            category: 'test', message: 'error', level: BreadcrumbLevel.error));

        final warnings = buffer.whereLevel(BreadcrumbLevel.warning);
        expect(warnings, hasLength(2));
        expect(warnings.map((b) => b.message), ['warning', 'error']);
      });
    });

    group('clear', () {
      test('removes all breadcrumbs', () {
        buffer.add(Breadcrumb(category: 'test', message: 'first'));
        buffer.add(Breadcrumb(category: 'test', message: 'second'));
        expect(buffer.length, 2);

        buffer.clear();

        expect(buffer.isEmpty, isTrue);
        expect(buffer.length, 0);
      });
    });
  });

  group('BreadcrumbBufferExtensions', () {
    late BreadcrumbBuffer buffer;

    setUp(() {
      buffer = BreadcrumbBuffer();
    });

    test('addNavigation adds navigation breadcrumb', () {
      buffer.addNavigation('Screen changed', data: {'screen': 'Home'});

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'navigation');
      expect(breadcrumbs[0].message, 'Screen changed');
      expect(breadcrumbs[0].level, BreadcrumbLevel.info);
    });

    test('addUserAction adds user action breadcrumb', () {
      buffer.addUserAction('Button tapped', data: {'button': 'submit'});

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'user_action');
      expect(breadcrumbs[0].message, 'Button tapped');
    });

    test('addNetwork adds network breadcrumb', () {
      buffer.addNetwork(
        'API call',
        data: {'endpoint': '/users'},
        level: BreadcrumbLevel.debug,
      );

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'network');
      expect(breadcrumbs[0].level, BreadcrumbLevel.debug);
    });

    test('addError adds error breadcrumb with error level', () {
      buffer.addError('Something failed', data: {'code': 500});

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'error');
      expect(breadcrumbs[0].level, BreadcrumbLevel.error);
    });

    test('addStateChange adds state change breadcrumb', () {
      buffer.addStateChange('App backgrounded');

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'state_change');
    });

    test('addAuth adds auth breadcrumb', () {
      buffer.addAuth('User logged in', data: {'email': 'alice@example.com'});

      final breadcrumbs = buffer.toList();
      expect(breadcrumbs[0].category, 'auth');
      // Verify PII redaction
      expect(breadcrumbs[0].data!['email'], 'al***@example.com');
    });
  });
}
