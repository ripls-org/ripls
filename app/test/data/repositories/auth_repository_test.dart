import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/auth_repository.dart';
import 'package:ripls/services/auth_service.dart';

import 'auth_repository_test.mocks.dart';

@GenerateMocks([AuthService])
void main() {
  late AuthRepository repository;
  late MockAuthService mockService;

  setUp(() {
    mockService = MockAuthService();
    repository = AuthRepository(mockService);
  });

  group('AuthRepository.checkInvitation', () {
    test('returns valid result for a valid short code', () async {
      when(mockService.checkInvitation(shortCode: 'VALID123')).thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: true,
          communityId: 'comm1',
          communityName: 'Test Community',
          inviterName: 'Alice',
          errorMessage: '',
          numMembers: 5,
          maxMembers: 32,
        ),
      );

      final result = await repository.checkInvitation(shortCode: 'VALID123');

      expect(result.isValid, isTrue);
      expect(result.communityId, 'comm1');
      expect(result.communityName, 'Test Community');
      expect(result.inviterName, 'Alice');
      verify(mockService.checkInvitation(shortCode: 'VALID123')).called(1);
    });

    test('returns invalid result for an unknown short code', () async {
      when(mockService.checkInvitation(shortCode: 'BADCODE')).thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: false,
          communityId: '',
          communityName: '',
          inviterName: '',
          errorMessage: 'Invitation not found',
        ),
      );

      final result = await repository.checkInvitation(shortCode: 'BADCODE');

      expect(result.isValid, isFalse);
      expect(result.errorMessage, 'Invitation not found');
    });

    test('propagates ConnectException as failed result', () async {
      when(mockService.checkInvitation(shortCode: anyNamed('shortCode')))
          .thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: false,
          communityId: '',
          communityName: '',
          inviterName: '',
          errorMessage: 'not_found',
        ),
      );

      final result = await repository.checkInvitation(shortCode: 'X');
      expect(result.isValid, isFalse);
    });

    test('propagates generic exception from service', () async {
      when(mockService.checkInvitation(shortCode: anyNamed('shortCode')))
          .thenThrow(Exception('network unreachable'));

      expect(
        () => repository.checkInvitation(shortCode: 'X'),
        throwsException,
      );
    });

    test('isCommunityFull is false when under capacity', () async {
      when(mockService.checkInvitation(shortCode: anyNamed('shortCode')))
          .thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: true,
          communityId: 'c1',
          communityName: 'Small',
          inviterName: 'Bob',
          errorMessage: '',
          numMembers: 10,
          maxMembers: 32,
        ),
      );

      final result = await repository.checkInvitation(shortCode: 'ANY');
      expect(result.isCommunityFull, isFalse);
    });

    test('isCommunityFull is true when at capacity', () async {
      when(mockService.checkInvitation(shortCode: anyNamed('shortCode')))
          .thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: true,
          communityId: 'c1',
          communityName: 'Full',
          inviterName: 'Bob',
          errorMessage: '',
          numMembers: 32,
          maxMembers: 32,
        ),
      );

      final result = await repository.checkInvitation(shortCode: 'ANY');
      expect(result.isCommunityFull, isTrue);
    });
  });

  group('AuthRepository.loginWithPassword', () {
    test('returns successful auth result with tokens', () async {
      when(
        mockService.loginWithPassword(
          email: 'alice@example.com',
          password: 'secret',
        ),
      ).thenAnswer(
        (_) async => AuthResult(
          success: true,
          accessToken: 'access-tok',
          refreshToken: 'refresh-tok',
        ),
      );

      final result = await repository.loginWithPassword(
        email: 'alice@example.com',
        password: 'secret',
      );

      expect(result.success, isTrue);
      expect(result.accessToken, 'access-tok');
      expect(result.refreshToken, 'refresh-tok');
      expect(result.error, isNull);
      verify(
        mockService.loginWithPassword(
          email: 'alice@example.com',
          password: 'secret',
        ),
      ).called(1);
    });

    test('returns failed result on invalid credentials', () async {
      when(
        mockService.loginWithPassword(
          email: anyNamed('email'),
          password: anyNamed('password'),
        ),
      ).thenAnswer(
        (_) async => AuthResult(success: false, error: 'Invalid credentials'),
      );

      final result = await repository.loginWithPassword(
        email: 'alice@example.com',
        password: 'wrong',
      );

      expect(result.success, isFalse);
      expect(result.error, 'Invalid credentials');
      expect(result.accessToken, isNull);
    });

    test('propagates network failure exception from service', () async {
      when(
        mockService.loginWithPassword(
          email: anyNamed('email'),
          password: anyNamed('password'),
        ),
      ).thenThrow(Exception('network failure'));

      expect(
        () => repository.loginWithPassword(
          email: 'alice@example.com',
          password: 'secret',
        ),
        throwsException,
      );
    });

    test('returns failed result on ConnectException response', () async {
      when(
        mockService.loginWithPassword(
          email: anyNamed('email'),
          password: anyNamed('password'),
        ),
      ).thenAnswer(
        (_) async => AuthResult(
          success: false,
          error: connect.Code.unauthenticated.name,
        ),
      );

      final result = await repository.loginWithPassword(
        email: 'alice@example.com',
        password: 'wrong',
      );

      expect(result.success, isFalse);
      expect(result.error, isNotNull);
    });
  });

  group('AuthRepository.registerWithPassword', () {
    test('returns successful auth result on registration', () async {
      when(
        mockService.registerWithPassword(
          email: 'new@example.com',
          name: 'New User',
          password: 'pass123',
          shortCode: 'INVITE1',
        ),
      ).thenAnswer(
        (_) async => AuthResult(
          success: true,
          accessToken: 'access-tok',
          refreshToken: 'refresh-tok',
        ),
      );

      final result = await repository.registerWithPassword(
        email: 'new@example.com',
        name: 'New User',
        password: 'pass123',
        shortCode: 'INVITE1',
      );

      expect(result.success, isTrue);
      expect(result.accessToken, 'access-tok');
      verify(
        mockService.registerWithPassword(
          email: 'new@example.com',
          name: 'New User',
          password: 'pass123',
          shortCode: 'INVITE1',
        ),
      ).called(1);
    });

    test('returns failed result when email is already taken', () async {
      when(
        mockService.registerWithPassword(
          email: anyNamed('email'),
          name: anyNamed('name'),
          password: anyNamed('password'),
          shortCode: anyNamed('shortCode'),
        ),
      ).thenAnswer(
        (_) async =>
            AuthResult(success: false, error: 'Email already registered'),
      );

      final result = await repository.registerWithPassword(
        email: 'existing@example.com',
        name: 'User',
        password: 'pass',
        shortCode: 'CODE',
      );

      expect(result.success, isFalse);
      expect(result.error, 'Email already registered');
    });

    test('propagates network failure exception from service', () async {
      when(
        mockService.registerWithPassword(
          email: anyNamed('email'),
          name: anyNamed('name'),
          password: anyNamed('password'),
          shortCode: anyNamed('shortCode'),
        ),
      ).thenThrow(Exception('network failure'));

      expect(
        () => repository.registerWithPassword(
          email: 'u@example.com',
          name: 'U',
          password: 'p',
          shortCode: 'C',
        ),
        throwsException,
      );
    });
  });

  group('AuthRepository.refreshToken', () {
    test('returns new tokens on success', () async {
      when(mockService.refreshToken(refreshToken: 'old-refresh-tok'))
          .thenAnswer(
        (_) async => TokenRefreshResult(
          success: true,
          accessToken: 'new-access-tok',
          refreshToken: 'new-refresh-tok',
        ),
      );

      final result =
          await repository.refreshToken(refreshToken: 'old-refresh-tok');

      expect(result.success, isTrue);
      expect(result.accessToken, 'new-access-tok');
      expect(result.refreshToken, 'new-refresh-tok');
      verify(mockService.refreshToken(refreshToken: 'old-refresh-tok'))
          .called(1);
    });

    test('returns failed result when session has expired', () async {
      when(mockService.refreshToken(refreshToken: anyNamed('refreshToken')))
          .thenAnswer(
        (_) async => TokenRefreshResult(success: false),
      );

      final result =
          await repository.refreshToken(refreshToken: 'expired-tok');

      expect(result.success, isFalse);
      expect(result.accessToken, isNull);
      expect(result.refreshToken, isNull);
    });

    test('returns failed result on ConnectException response', () async {
      when(mockService.refreshToken(refreshToken: anyNamed('refreshToken')))
          .thenAnswer(
        (_) async => TokenRefreshResult(success: false),
      );

      final result =
          await repository.refreshToken(refreshToken: 'bad-tok');

      expect(result.success, isFalse);
    });

    test('propagates network failure exception from service', () async {
      when(mockService.refreshToken(refreshToken: anyNamed('refreshToken')))
          .thenThrow(Exception('network unreachable'));

      expect(
        () => repository.refreshToken(refreshToken: 'tok'),
        throwsException,
      );
    });
  });
}
