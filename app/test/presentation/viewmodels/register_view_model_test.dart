import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/presentation/viewmodels/register_view_model.dart';
import 'package:ripls/services/auth_service.dart';
import 'package:ripls/services/providers.dart';

import 'register_view_model_test.mocks.dart';

@GenerateMocks([AuthService])
void main() {
  late ProviderContainer container;
  late MockAuthService mockAuthService;

  setUp(() {
    mockAuthService = MockAuthService();
    container = ProviderContainer(
      overrides: [
        authServiceProvider.overrideWithValue(mockAuthService),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('RegisterState', () {
    test('initial state is correct', () {
      final state = container.read(registerProvider);

      expect(state.isCheckingInvitation, isFalse);
      expect(state.invitationVerified, isFalse);
      expect(state.error, isNull);
      expect(state.communityId, '');
      expect(state.communityName, '');
      expect(state.inviterName, '');
    });
  });

  group('checkShortCode', () {
    test('successfully verifies valid invitation', () async {
      const token = 'valid-token';
      final response = InvitationCheckResult(
        isValid: true,
        communityId: 'community-123',
        communityName: 'Test Community',
        inviterName: 'John Doe',
        errorMessage: '',
      );

      when(mockAuthService.checkInvitation(shortCode: token))
          .thenAnswer((_) async => response);

      final notifier = container.read(registerProvider.notifier);
      await notifier.checkShortCode(token);

      final state = container.read(registerProvider);
      expect(state.isCheckingInvitation, isFalse);
      expect(state.invitationVerified, isTrue);
      expect(state.communityId, 'community-123');
      expect(state.communityName, 'Test Community');
      expect(state.inviterName, 'John Doe');
      expect(state.error, isNull);
    });

    test('handles invalid invitation', () async {
      const token = 'invalid-token';
      final response = InvitationCheckResult(
        isValid: false,
        communityId: '',
        communityName: '',
        inviterName: '',
        errorMessage: 'Invalid invitation token',
      );

      when(mockAuthService.checkInvitation(shortCode: token))
          .thenAnswer((_) async => response);

      final notifier = container.read(registerProvider.notifier);
      await notifier.checkShortCode(token);

      final state = container.read(registerProvider);
      expect(state.isCheckingInvitation, isFalse);
      expect(state.invitationVerified, isFalse);
      expect(state.communityId, '');
      expect(state.communityName, '');
      expect(state.inviterName, '');
      expect(state.error, isNotNull);
    });

    test('handles service exception', () async {
      const token = 'error-token';

      when(mockAuthService.checkInvitation(shortCode: token))
          .thenThrow(Exception('Network error'));

      final notifier = container.read(registerProvider.notifier);
      await notifier.checkShortCode(token);

      final state = container.read(registerProvider);
      expect(state.isCheckingInvitation, isFalse);
      expect(state.invitationVerified, isFalse);
      expect(state.communityId, '');
      expect(state.communityName, '');
      expect(state.inviterName, '');
      expect(state.error, isNotNull);
    });

    test('sets loading state during check', () async {
      const token = 'valid-token';
      final response = InvitationCheckResult(
        isValid: true,
        communityId: 'community-123',
        communityName: 'Test Community',
        inviterName: 'John Doe',
        errorMessage: '',
      );

      when(mockAuthService.checkInvitation(shortCode: token))
          .thenAnswer((_) async {
        // Simulate delay
        await Future.delayed(const Duration(milliseconds: 10));
        return response;
      });

      final notifier = container.read(registerProvider.notifier);
      final checkFuture = notifier.checkShortCode(token);

      // Check loading state immediately
      await Future.delayed(const Duration(milliseconds: 1));
      expect(container.read(registerProvider).isCheckingInvitation, isTrue);

      // Wait for completion
      await checkFuture;
      expect(container.read(registerProvider).isCheckingInvitation, isFalse);
    });

    test('clears error message on new check', () async {
      const token1 = 'invalid-token';
      const token2 = 'valid-token';

      // First check - invalid
      final errorResponse = InvitationCheckResult(
        isValid: false,
        communityId: '',
        communityName: '',
        inviterName: '',
        errorMessage: 'Invalid token',
      );

      when(mockAuthService.checkInvitation(shortCode: token1))
          .thenAnswer((_) async => errorResponse);

      final notifier = container.read(registerProvider.notifier);
      await notifier.checkShortCode(token1);

      expect(container.read(registerProvider).error, isNotNull);

      // Second check - valid
      final successResponse = InvitationCheckResult(
        isValid: true,
        communityId: 'community-123',
        communityName: 'Test Community',
        inviterName: 'John Doe',
        errorMessage: '',
      );

      when(mockAuthService.checkInvitation(shortCode: token2))
          .thenAnswer((_) async => successResponse);

      await notifier.checkShortCode(token2);

      expect(container.read(registerProvider).error, isNull);
    });
  });

  group('reset', () {
    test('resets all state to initial values', () async {
      // First set some state
      const token = 'valid-token';
      final response = InvitationCheckResult(
        isValid: true,
        communityId: 'community-123',
        communityName: 'Test Community',
        inviterName: 'John Doe',
        errorMessage: '',
      );

      when(mockAuthService.checkInvitation(shortCode: token))
          .thenAnswer((_) async => response);

      final notifier = container.read(registerProvider.notifier);
      await notifier.checkShortCode(token);

      // Verify state is set
      expect(container.read(registerProvider).invitationVerified, isTrue);
      expect(container.read(registerProvider).communityName, 'Test Community');

      // Reset
      notifier.reset();

      // Verify state is reset
      final state = container.read(registerProvider);
      expect(state.isCheckingInvitation, isFalse);
      expect(state.invitationVerified, isFalse);
      expect(state.error, isNull);
      expect(state.communityId, '');
      expect(state.communityName, '');
      expect(state.inviterName, '');
    });
  });
}
