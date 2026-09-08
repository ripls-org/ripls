import 'package:ripls/services/auth_service.dart';

export 'package:ripls/data/gen/ripls/api/login_service.pbenum.dart'
    show ShareLinkTargetKind;
export 'package:ripls/services/auth_service.dart' show InvitationCheckResult;

/// Repository for authentication operations.
///
/// This repository wraps AuthService and provides a consistent interface
/// for authentication operations. Auth operations are generally not cached
/// as they involve security-sensitive data and state changes.
class AuthRepository {
  final AuthService _service;

  AuthRepository(this._service);

  /// Validates an invitation short code and returns associated details.
  ///
  /// This is a pass-through to the service (not cached) as invitation
  /// validation should always check the current server state.
  ///
  /// Parameters:
  /// - [shortCode]: The invitation short code to validate
  ///
  /// Returns:
  /// - [InvitationCheckResult] containing validation status and details
  Future<InvitationCheckResult> checkInvitation({
    required String shortCode,
  }) async {
    return _service.checkInvitation(shortCode: shortCode);
  }

  /// Authenticates an existing user with email and password.
  ///
  /// Returns an [AuthResult] containing tokens on success, or an error
  /// message (including invalid credentials) on failure.
  Future<AuthResult> loginWithPassword({
    required String email,
    required String password,
  }) async {
    return _service.loginWithPassword(email: email, password: password);
  }

  /// Registers a new user account with email and password.
  ///
  /// Returns an [AuthResult] containing tokens on success, or an error
  /// message on failure (e.g. duplicate email, invalid invitation).
  Future<AuthResult> registerWithPassword({
    required String email,
    required String name,
    required String password,
    required String shortCode,
  }) async {
    return _service.registerWithPassword(
      email: email,
      name: name,
      password: password,
      shortCode: shortCode,
    );
  }

  /// Exchanges a refresh token for a new access token and rotated refresh token.
  ///
  /// Returns a [TokenRefreshResult] with new tokens on success, or a failed
  /// result when the session has expired or the token is invalid.
  Future<TokenRefreshResult> refreshToken({
    required String refreshToken,
  }) async {
    return _service.refreshToken(refreshToken: refreshToken);
  }
}
