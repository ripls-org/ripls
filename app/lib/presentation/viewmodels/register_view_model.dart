import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/services/providers.dart';

part 'register_view_model.freezed.dart';

final _log = Logger('RegisterViewModel');

/// State for the registration screen
@freezed
sealed class RegisterState with _$RegisterState {
  const factory RegisterState({
    @Default(false) bool isCheckingInvitation,
    @Default(false) bool invitationVerified,
    UserError? error,
    @Default('') String communityId,
    @Default('') String communityName,
    @Default('') String inviterName,
    @Default('') String communityImageUrl,
    @Default(0) int numMembers,
    @Default(0) int maxMembers,
  }) = _RegisterState;
}

extension RegisterStateX on RegisterState {
  /// True when the invited community has reached its member cap.
  /// `maxMembers == 0` indicates a bootstrap case (no tier set) and is not
  /// considered full.
  bool get isCommunityFull => maxMembers > 0 && numMembers >= maxMembers;
}

/// Notifier for managing registration state
class RegisterNotifier extends Notifier<RegisterState> {
  @override
  RegisterState build() {
    return const RegisterState();
  }

  /// Validates an invitation short code and updates state.
  Future<void> checkShortCode(String shortCode) async {
    state = state.copyWith(
      isCheckingInvitation: true,
      error: null,
    );

    try {
      final authRepository = ref.read(authRepositoryProvider);
      final result = await authRepository.checkInvitation(shortCode: shortCode);

      if (result.isValid) {
        state = state.copyWith(
          isCheckingInvitation: false,
          invitationVerified: true,
          communityId: result.communityId,
          communityName: result.communityName,
          inviterName: result.inviterName,
          communityImageUrl: result.communityImageUrl,
          numMembers: result.numMembers,
          maxMembers: result.maxMembers,
          error: null,
        );
        _log.info('✅ Invitation verified for community: ${result.communityName}');
      } else {
        state = state.copyWith(
          isCheckingInvitation: false,
          invitationVerified: false,
          communityId: '',
          communityName: '',
          inviterName: '',
          communityImageUrl: '',
          error: UserError.generic(fallback: result.errorMessage),
        );
        _log.warning('❌ Invalid invitation: ${result.errorMessage}');
      }
    } catch (e) {
      state = state.copyWith(
        isCheckingInvitation: false,
        invitationVerified: false,
        communityId: '',
        communityName: '',
        inviterName: '',
        communityImageUrl: '',
        error: RpcErrorHandler.classify(e, fallback: 'Could not verify invitation'),
      );
      _log.severe('Error checking invitation: $e');
    }
  }

  /// Resets the registration state
  void reset() {
    state = const RegisterState();
    _log.info('🔄 Registration state reset');
  }
}

/// Provider for the registration state
final registerProvider = NotifierProvider<RegisterNotifier, RegisterState>(
  RegisterNotifier.new,
);
