import 'package:connectrpc/connect.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/errors/user_error.dart';

void main() {
  group('UserError', () {
    test('serverMessage carries message and code', () {
      const e = UserError.serverMessage(message: 'oops', code: Code.notFound);
      expect(e, isA<UserErrorServerMessage>());
      e as UserErrorServerMessage;
      expect(e.message, 'oops');
      expect(e.code, Code.notFound);
    });

    test('rpcCode carries the code', () {
      const e = UserError.rpcCode(Code.unauthenticated);
      expect(e, isA<UserErrorRpcCode>());
      expect((e as UserErrorRpcCode).code, Code.unauthenticated);
    });

    test('transport variants distinguish socket/timeout/http', () {
      const a = UserError.transport(TransportKind.socket);
      const b = UserError.transport(TransportKind.timeout);
      const c = UserError.transport(TransportKind.http);
      expect((a as UserErrorTransport).kind, TransportKind.socket);
      expect((b as UserErrorTransport).kind, TransportKind.timeout);
      expect((c as UserErrorTransport).kind, TransportKind.http);
    });

    test('generic without fallback', () {
      const e = UserError.generic();
      expect(e, isA<UserErrorGeneric>());
      expect((e as UserErrorGeneric).fallback, isNull);
    });

    test('generic with fallback', () {
      const e = UserError.generic(fallback: 'Could not save');
      expect((e as UserErrorGeneric).fallback, 'Could not save');
    });

    test('equality on values', () {
      expect(
        const UserError.serverMessage(message: 'x', code: Code.notFound),
        const UserError.serverMessage(message: 'x', code: Code.notFound),
      );
      expect(
        const UserError.transport(TransportKind.socket),
        const UserError.transport(TransportKind.socket),
      );
      expect(
        const UserError.transport(TransportKind.socket),
        isNot(const UserError.transport(TransportKind.timeout)),
      );
    });
  });
}
