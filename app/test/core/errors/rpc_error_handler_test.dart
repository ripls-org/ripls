import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:connectrpc/connect.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/errors.pb.dart' as errors_pb;
import 'package:ripls/l10n/app_localizations.dart';

void main() {
  late AppLocalizations en;
  late AppLocalizations es;

  setUpAll(() async {
    en = await AppLocalizations.delegate.load(const Locale('en'));
    es = await AppLocalizations.delegate.load(const Locale('es'));
  });

  group('RpcErrorHandler.classify', () {
    test('ServiceException → serverMessage with message and code', () {
      final r = RpcErrorHandler.classify(
        ServiceException('not found here', code: Code.notFound),
      );
      expect(r, isA<UserErrorServerMessage>());
      r as UserErrorServerMessage;
      expect(r.message, 'not found here');
      expect(r.code, Code.notFound);
    });

    test('ConnectException → rpcCode', () {
      final r = RpcErrorHandler.classify(
        ConnectException(Code.unauthenticated, 'noauth'),
      );
      expect(r, isA<UserErrorRpcCode>());
      expect((r as UserErrorRpcCode).code, Code.unauthenticated);
    });

    test('ConnectException with LocalizedErrorDetail → localizedCode', () {
      // Server attaches a LocalizedErrorDetail when it wants the
      // client to render the message in the recipient's locale.
      final detail = errors_pb.LocalizedErrorDetail()
        ..code = 'experience_owner_required_for_delete';
      final r = RpcErrorHandler.classify(
        ConnectException(
          Code.permissionDenied,
          'only the owner can delete this event',
          details: [
            ErrorDetail(
              'ripls.api.LocalizedErrorDetail',
              detail.writeToBuffer(),
            ),
          ],
        ),
      );
      expect(r, isA<UserErrorLocalizedCode>());
      final lc = r as UserErrorLocalizedCode;
      expect(lc.code, 'experience_owner_required_for_delete');
      expect(lc.params, isEmpty);
    });

    test('ConnectException with LocalizedErrorDetail params is decoded', () {
      final detail = errors_pb.LocalizedErrorDetail()
        ..code = 'experience_already_shared'
        ..params['eventName'] = 'Saturday potluck'
        ..params['actorName'] = 'Alice';
      final r = RpcErrorHandler.classify(
        ConnectException(
          Code.alreadyExists,
          'event already shared',
          details: [
            ErrorDetail(
              'ripls.api.LocalizedErrorDetail',
              detail.writeToBuffer(),
            ),
          ],
        ),
      );
      final lc = r as UserErrorLocalizedCode;
      expect(lc.params['eventName'], 'Saturday potluck');
      expect(lc.params['actorName'], 'Alice');
    });

    test('ConnectException with malformed detail falls through to rpcCode', () {
      // A detail whose type matches but whose bytes don't decode
      // must not crash classify — fall through to the per-Code
      // branch so the user still sees something readable.
      final r = RpcErrorHandler.classify(
        ConnectException(
          Code.permissionDenied,
          'denied',
          details: [
            ErrorDetail(
              'ripls.api.LocalizedErrorDetail',
              Uint8List.fromList([0xFF, 0xFE, 0xFD]),
            ),
          ],
        ),
      );
      expect(r, isA<UserErrorRpcCode>());
      expect((r as UserErrorRpcCode).code, Code.permissionDenied);
    });

    test('ConnectException with unrelated detail type → rpcCode', () {
      final r = RpcErrorHandler.classify(
        ConnectException(
          Code.permissionDenied,
          'denied',
          details: [
            ErrorDetail(
              'ripls.api.SomeOtherDetail',
              Uint8List.fromList([0x01, 0x02]),
            ),
          ],
        ),
      );
      expect(r, isA<UserErrorRpcCode>());
    });

    test('SocketException → transport(socket)', () {
      final r = RpcErrorHandler.classify(const SocketException('no net'));
      expect(r, isA<UserErrorTransport>());
      expect((r as UserErrorTransport).kind, TransportKind.socket);
    });

    test('TimeoutException → transport(timeout)', () {
      final r = RpcErrorHandler.classify(TimeoutException('slow'));
      expect((r as UserErrorTransport).kind, TransportKind.timeout);
    });

    test('HttpException → transport(http)', () {
      final r = RpcErrorHandler.classify(const HttpException('boom'));
      expect((r as UserErrorTransport).kind, TransportKind.http);
    });

    test('FormatException with fallback → generic(fallback)', () {
      final r = RpcErrorHandler.classify(
        const FormatException('SQL: oops'),
        fallback: 'Could not save',
      );
      expect(r, isA<UserErrorGeneric>());
      expect((r as UserErrorGeneric).fallback, 'Could not save');
    });

    test('StateError without fallback → generic(null)', () {
      final r = RpcErrorHandler.classify(StateError('null check'));
      expect((r as UserErrorGeneric).fallback, isNull);
    });

    test('plain Object → generic(null)', () {
      final r = RpcErrorHandler.classify(Object());
      expect((r as UserErrorGeneric).fallback, isNull);
    });

    test('does not call toString() on the exception', () {
      final r = RpcErrorHandler.classify(
        ServiceException('safe', code: Code.notFound),
      );
      // serverMessage variant carries the .message field directly, not
      // anything derived from toString().
      expect((r as UserErrorServerMessage).message, 'safe');
    });
  });

  group('RpcErrorHandler.isExpectedRpcError', () {
    test('ServiceException with permissionDenied → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('denied', code: Code.permissionDenied),
        ),
        isTrue,
      );
    });

    test('ServiceException with notFound → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('missing', code: Code.notFound),
        ),
        isTrue,
      );
    });

    test('ServiceException with unauthenticated → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('login', code: Code.unauthenticated),
        ),
        isTrue,
      );
    });

    test('ServiceException with failedPrecondition → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('bad state', code: Code.failedPrecondition),
        ),
        isTrue,
      );
    });

    test('ServiceException with invalidArgument → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('bad arg', code: Code.invalidArgument),
        ),
        isTrue,
      );
    });

    test('ServiceException with alreadyExists → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('dup', code: Code.alreadyExists),
        ),
        isTrue,
      );
    });

    test('ServiceException with resourceExhausted → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('throttled', code: Code.resourceExhausted),
        ),
        isTrue,
      );
    });

    test('ServiceException with canceled → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('canceled', code: Code.canceled),
        ),
        isTrue,
      );
    });

    test('ServiceException with internal → false (real bug)', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('boom', code: Code.internal),
        ),
        isFalse,
      );
    });

    test('ServiceException with unavailable → false (server outage)', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ServiceException('down', code: Code.unavailable),
        ),
        isFalse,
      );
    });

    test('ServiceException with no code → false', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(ServiceException('no code')),
        isFalse,
      );
    });

    test('raw ConnectException with permissionDenied → true', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ConnectException(Code.permissionDenied, 'denied'),
        ),
        isTrue,
      );
    });

    test('raw ConnectException with internal → false', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(
          ConnectException(Code.internal, 'boom'),
        ),
        isFalse,
      );
    });

    test('non-RPC exception (SocketException) → false', () {
      expect(
        RpcErrorHandler.isExpectedRpcError(const SocketException('no net')),
        isFalse,
      );
    });

    test('plain Object → false', () {
      expect(RpcErrorHandler.isExpectedRpcError(Object()), isFalse);
    });
  });

  group('RpcErrorHandler.localize', () {
    test('serverMessage returns the server message verbatim', () {
      const e = UserError.serverMessage(message: 'hi', code: Code.notFound);
      expect(RpcErrorHandler.localize(e, en), 'hi');
      expect(RpcErrorHandler.localize(e, es), 'hi');
    });

    test('rpcCode resolves via AppLocalizations', () {
      const e = UserError.rpcCode(Code.notFound);
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorNotFound);
      expect(RpcErrorHandler.localize(e, es), es.rpcErrorNotFound);
      expect(RpcErrorHandler.localize(e, en),
          isNot(RpcErrorHandler.localize(e, es)));
    });

    test('transport(socket) resolves to localized socket message', () {
      const e = UserError.transport(TransportKind.socket);
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorTransportSocket);
      expect(RpcErrorHandler.localize(e, es), es.rpcErrorTransportSocket);
    });

    test('transport(timeout) resolves', () {
      const e = UserError.transport(TransportKind.timeout);
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorTransportTimeout);
    });

    test('transport(http) resolves', () {
      const e = UserError.transport(TransportKind.http);
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorTransportHttp);
    });

    test('generic without fallback uses rpcErrorGeneric', () {
      const e = UserError.generic();
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorGeneric);
      expect(RpcErrorHandler.localize(e, es), es.rpcErrorGeneric);
    });

    test('generic with fallback uses fallback verbatim', () {
      const e = UserError.generic(fallback: 'Could not save');
      expect(RpcErrorHandler.localize(e, en), 'Could not save');
      expect(RpcErrorHandler.localize(e, es), 'Could not save');
    });

    test('localizedCode known code resolves via AppLocalizations', () {
      const e = UserError.localizedCode(
        code: 'experience_owner_required_for_delete',
      );
      expect(RpcErrorHandler.localize(e, en),
          en.rpcErrorExperienceOwnerRequiredForDelete);
      expect(RpcErrorHandler.localize(e, es),
          es.rpcErrorExperienceOwnerRequiredForDelete);
      // EN and ES must differ — otherwise the parity is bogus.
      expect(RpcErrorHandler.localize(e, en),
          isNot(RpcErrorHandler.localize(e, es)));
    });

    test('localizedCode unknown code falls back to generic message', () {
      // Client may be older than server; an unrecognized code must
      // still render something readable, not blow up.
      const e = UserError.localizedCode(code: 'this_code_does_not_exist');
      expect(RpcErrorHandler.localize(e, en), en.rpcErrorGeneric);
      expect(RpcErrorHandler.localize(e, es), es.rpcErrorGeneric);
    });
  });
}
