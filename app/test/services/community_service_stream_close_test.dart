import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/services/community_service.dart';

void main() {
  group('CommunityService.isBenignStreamClose', () {
    test('matches Code.internal with HTTP/2 INTERNAL_ERROR message', () {
      final e = connect.ConnectException(
        connect.Code.internal,
        'http/2 stream closed with error code INTERNAL_ERROR (0x2)',
      );

      expect(CommunityService.isBenignStreamClose(e), isTrue);
    });

    test('does not match Code.internal with a different message', () {
      final e = connect.ConnectException(
        connect.Code.internal,
        'database query failed',
      );

      expect(CommunityService.isBenignStreamClose(e), isFalse);
    });

    test('does not match Code.internal with bare INTERNAL_ERROR (no http/2 prefix)', () {
      final e = connect.ConnectException(
        connect.Code.internal,
        'INTERNAL_ERROR',
      );

      expect(CommunityService.isBenignStreamClose(e), isFalse);
    });

    test('does not match Code.unavailable even with INTERNAL_ERROR text', () {
      final e = connect.ConnectException(
        connect.Code.unavailable,
        'INTERNAL_ERROR',
      );

      expect(CommunityService.isBenignStreamClose(e), isFalse);
    });

    test('does not match Code.unauthenticated', () {
      final e = connect.ConnectException(
        connect.Code.unauthenticated,
        'token expired',
      );

      expect(CommunityService.isBenignStreamClose(e), isFalse);
    });
  });
}
