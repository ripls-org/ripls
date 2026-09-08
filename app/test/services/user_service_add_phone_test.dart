import 'package:connectrpc/connect.dart' as connect;
import 'package:connectrpc/test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user_service.connect.spec.dart'
    as specs;
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/services/user_service.dart';

void main() {
  group('UserService.addPhoneNumber', () {
    test('sends the firebase + access tokens and returns the verified phone',
        () async {
      String? capturedFbToken;
      Iterable<String>? capturedAuth;

      final transport = FakeTransportBuilder()
          .unary(specs.UserService.addPhoneNumber, (req, ctx) {
        capturedFbToken = req.firebaseIdToken;
        capturedAuth = ctx.requestHeaders.get('Authorization');
        return AddPhoneNumberResponse(phoneNumber: '+15551234567');
      }).build();

      final service = UserService(
        transport: transport,
        getAccessToken: () => 'access-tok',
      );

      final phone = await service.addPhoneNumber(firebaseIdToken: 'fb-token');

      expect(phone, '+15551234567');
      expect(capturedFbToken, 'fb-token',
          reason: 'the OTP token must be forwarded to the server');
      expect(capturedAuth, contains('Bearer access-tok'),
          reason: 'the call must be authenticated as the current user');
    });

    test('maps an already-on-another-account conflict to a ServiceException',
        () async {
      final transport = FakeTransportBuilder()
          .unary(specs.UserService.addPhoneNumber, (req, ctx) {
        throw connect.ConnectException(
          connect.Code.alreadyExists,
          'this phone number is already linked to another account',
        );
      }).build();

      final service = UserService(
        transport: transport,
        getAccessToken: () => 'access-tok',
      );

      await expectLater(
        () => service.addPhoneNumber(firebaseIdToken: 'fb-token'),
        throwsA(isA<ServiceException>()),
      );
    });
  });
}
