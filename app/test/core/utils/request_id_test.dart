import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/request_id.dart';

void main() {
  group('RequestIdGenerator', () {
    test('headerName is X-Request-ID', () {
      expect(RequestIdGenerator.headerName, 'X-Request-ID');
    });

    test('generate returns a non-empty string', () {
      final requestId = RequestIdGenerator.generate();

      expect(requestId, isNotEmpty);
    });

    test('generate returns UUID v4 format', () {
      final requestId = RequestIdGenerator.generate();

      // UUID v4 format: 8-4-4-4-12 hex characters
      final uuidRegex = RegExp(
        r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
        caseSensitive: false,
      );
      expect(uuidRegex.hasMatch(requestId), isTrue);
    });

    test('generate returns unique IDs', () {
      final ids = <String>{};

      for (var i = 0; i < 100; i++) {
        ids.add(RequestIdGenerator.generate());
      }

      expect(ids.length, 100);
    });

    test('current is null outside of a runWithRequestId scope', () {
      expect(RequestIdGenerator.current, isNull);
    });

    test('runWithRequestId exposes the id via current inside the body', () {
      String? observed;
      RequestIdGenerator.runWithRequestId('req-xyz', () {
        observed = RequestIdGenerator.current;
      });
      expect(observed, 'req-xyz');
    });

    test('runWithRequestId restores the parent scope after returning', () {
      RequestIdGenerator.runWithRequestId('outer', () {
        expect(RequestIdGenerator.current, 'outer');
        RequestIdGenerator.runWithRequestId('inner', () {
          expect(RequestIdGenerator.current, 'inner');
        });
        expect(RequestIdGenerator.current, 'outer');
      });
      expect(RequestIdGenerator.current, isNull);
    });

    test('runWithRequestId propagates the id across awaits', () async {
      final observed = await RequestIdGenerator.runWithRequestId<Future<String?>>(
        'req-async',
        () async {
          await Future<void>.delayed(Duration.zero);
          return RequestIdGenerator.current;
        },
      );
      expect(observed, 'req-async');
    });
  });
}
