import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/observability/logging/redactor.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/login_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/login_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

final _log = ObservableLogger.named('AuthService');

/// AuthService handles user registration and authentication using the LoginService API.
class AuthService {
  final LoginServiceClient _client;
  final String _baseUrl;

  AuthService({required connect.Transport transport, required String serverUrl})
    : _client = LoginServiceClient(transport),
      _baseUrl = serverUrl;

  /// Builds headers for unauthenticated login/register RPCs. Stamps an
  /// X-Request-ID for client/server log correlation; intentionally does not
  /// pass an onUnauthenticated callback because the missing access token is
  /// the *expected* state during these flows.
  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(() => null);
  }

  /// RequestEmailCode asks the server to mail a one-time sign-in code.
  ///
  /// Succeeds whether or not an account exists for the address — the same
  /// entry point serves sign-up and sign-in, so the caller learns nothing
  /// about who has an account. Which of register/login to call is decided
  /// after ownership is proven, not here.
  Future<EmailCodeResult> requestEmailCode({
    required String email,
    bool requireExistingAccount = false,
  }) async {
    try {
      _log.info('Requesting email sign-in code', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
      });

      final response = await _client.requestEmailCode(
        RequestEmailCodeRequest(
          email: email,
          requireExistingAccount: requireExistingAccount,
        ),
        headers: _buildHeaders(),
      );

      // devCode is only populated by a development server; it is the seam the
      // e2e harness uses to complete the loop without a mailbox.
      return EmailCodeResult(success: true, devCode: response.hasDevCode() ? response.devCode : null);
    } on connect.ConnectException catch (e) {
      _log.warning('Email code request failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return EmailCodeResult(
        success: false,
        error: e.message,
        noAccount: e.code == connect.Code.notFound,
      );
    } catch (e) {
      _log.error('Email code request failed with exception', e);
      return EmailCodeResult(
        success: false,
        error: RpcErrorHandler.getTransportErrorMessage(e),
      );
    }
  }

  /// VerifyEmailCode exchanges a mailed code for proof of address ownership.
  ///
  /// The returned token is passed to [registerWithEmailCode] or
  /// [loginWithEmailCode] in place of a password — the same shape the phone
  /// flow uses, where a verified provider token is obtained first and then
  /// handed to whichever of register/login applies. It is not a session token.
  Future<EmailProofResult> verifyEmailCode({
    required String email,
    required String code,
  }) async {
    try {
      _log.info('Verifying email sign-in code', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
      });

      final response = await _client.verifyEmailCode(
        VerifyEmailCodeRequest(email: email, code: code),
        headers: _buildHeaders(),
      );

      return EmailProofResult(
        success: true,
        emailProofToken: response.emailProofToken,
        accountExists: response.accountExists,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Email code verification failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return EmailProofResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Email code verification failed with exception', e);
      return EmailProofResult(
        success: false,
        error: RpcErrorHandler.getTransportErrorMessage(e),
      );
    }
  }

  /// RegisterWithEmailCode registers a new account using a verified-email
  /// proof token instead of a password.
  Future<AuthResult> registerWithEmailCode({
    required String email,
    required String name,
    required String emailProofToken,
    required String shortCode,
  }) async {
    try {
      _log.info('Calling register API', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
        'credential': 'email_code',
      });

      final response = await _client.emailRegister(
        EmailRegisterRequest(
          email: email,
          name: name,
          emailProofToken: emailProofToken,
          shortCode: shortCode,
        ),
        headers: _buildHeaders(),
      );

      _log.info('Register successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Register failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message, errorCode: e.code);
    } catch (e) {
      _log.error('Register failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// LoginWithEmailCode signs in using a verified-email proof token.
  ///
  /// Accepts any account holding a verified email, not just email/password
  /// accounts — an OIDC-registered account can sign in this way too.
  Future<AuthResult> loginWithEmailCode({
    required String email,
    required String emailProofToken,
  }) async {
    try {
      _log.info('Calling login API', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
        'credential': 'email_code',
      });

      final response = await _client.emailLogin(
        EmailLoginRequest(email: email, emailProofToken: emailProofToken),
        headers: _buildHeaders(),
      );

      _log.info('Login successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Login failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message, errorCode: e.code);
    } catch (e) {
      _log.error('Login failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// RegisterWithPassword registers a new user account with email and password.
  ///
  /// Legacy: new accounts register through [registerWithEmailCode]. Retained
  /// until the password path is removed (#2571).
  Future<AuthResult> registerWithPassword({
    required String email,
    required String name,
    required String password,
    required String shortCode,
  }) async {
    try {
      _log.info('Calling register API', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
      });

      final request = EmailRegisterRequest(
        email: email,
        name: name,
        password: password,
        shortCode: shortCode,
      );

      final response = await _client.emailRegister(request, headers: _buildHeaders());

      _log.info('Register successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Register failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Register failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// LoginWithPassword logs in an existing user with email and password.
  Future<AuthResult> loginWithPassword({
    required String email,
    required String password,
  }) async {
    try {
      _log.info('Calling login API', {
        'server': _baseUrl,
        'email': LogRedactor.maskEmail(email),
      });

      final request = EmailLoginRequest(email: email, password: password);
      final response = await _client.emailLogin(request, headers: _buildHeaders());

      _log.info('Login successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Login failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Login failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// OIDCRegister registers a new user with an OIDC provider (Google, Apple, etc.).
  Future<AuthResult> oidcRegister({
    required OIDCProvider provider,
    required String idToken,
    required String shortCode,
  }) async {
    try {
      final request = OIDCRegisterRequest(
        provider: provider,
        idToken: idToken,
        shortCode: shortCode,
      );

      final response = await _client.oIDCRegister(request, headers: _buildHeaders());

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      return AuthResult(success: false, error: e.message);
    } catch (e) {
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// OIDCLogin authenticates an existing user with an OIDC provider (Google, Apple, etc.).
  Future<AuthResult> oidcLogin({
    required OIDCProvider provider,
    required String idToken,
  }) async {
    try {
      final request = OIDCLoginRequest(provider: provider, idToken: idToken);

      final response = await _client.oIDCLogin(request, headers: _buildHeaders());

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      return AuthResult(success: false, error: e.message);
    } catch (e) {
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// PhoneRegister registers a new user with a verified phone number (passwordless).
  Future<AuthResult> phoneRegister({
    required String firebaseIdToken,
    required String name,
    required String shortCode,
  }) async {
    try {
      _log.info('Calling phone register API', {'server': _baseUrl});

      final request = PhoneRegisterRequest(
        firebaseIdToken: firebaseIdToken,
        name: name,
        shortCode: shortCode,
      );

      final response = await _client.phoneRegister(request, headers: _buildHeaders());

      _log.info('Phone register successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Phone register failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message, errorCode: e.code);
    } catch (e) {
      _log.error('Phone register failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// CheckPhoneRegistered reports whether a number already has an account.
  ///
  /// Sign-in surfaces call this BEFORE starting verification. The one-time code
  /// is sent by the identity provider from this device, so there is no way to
  /// decline it afterwards — without this, signing in with an unregistered
  /// number sends a real text that cannot lead anywhere and still ends at
  /// "no account found".
  ///
  /// Returns true when the check itself fails: a degraded check must not block
  /// a legitimate sign-in, and the flow still reports "no account" after
  /// verification exactly as it did before.
  Future<bool> isPhoneRegistered(String phoneNumber) async {
    try {
      final response = await _client.checkPhoneRegistered(
        CheckPhoneRegisteredRequest(phoneNumber: phoneNumber),
        headers: _buildHeaders(),
      );
      return response.isRegistered;
    } catch (e) {
      _log.warning('Phone registration check failed; continuing', {
        'error': e.toString(),
      });
      return true;
    }
  }

  /// PhoneLogin authenticates an existing user with a verified phone number (passwordless).
  Future<AuthResult> phoneLogin({
    required String firebaseIdToken,
  }) async {
    try {
      _log.info('Calling phone login API', {'server': _baseUrl});

      final request = PhoneLoginRequest(firebaseIdToken: firebaseIdToken);
      final response = await _client.phoneLogin(request, headers: _buildHeaders());

      _log.info('Phone login successful');

      return AuthResult(
        success: true,
        user: response.user,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Phone login failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return AuthResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Phone login failed with exception', e);
      return AuthResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// RefreshToken exchanges a refresh token for a new access token and rotated refresh token.
  Future<TokenRefreshResult> refreshToken({
    required String refreshToken,
  }) async {
    try {
      final request = RefreshTokenRequest(refreshToken: refreshToken);
      final response = await _client.refreshToken(request, headers: _buildHeaders());

      _log.info('Token refresh successful');

      return TokenRefreshResult(
        success: true,
        accessToken: response.tokens.accessToken,
        refreshToken: response.tokens.refreshToken,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Token refresh failed with ConnectException', {
        'code': e.code.name,
      });
      return TokenRefreshResult(success: false);
    } catch (e) {
      _log.error('Token refresh failed with exception', e);
      return TokenRefreshResult(success: false);
    }
  }

  /// CheckInvitation validates an invitation token and returns associated details.
  Future<InvitationCheckResult> checkInvitation({
    required String shortCode,
  }) async {
    try {
      _log.info('Checking invitation code');

      final request = CheckInvitationRequest(shortCode: shortCode);
      final response = await _client.checkInvitation(request, headers: _buildHeaders());

      if (response.isValid) {
        _log.info('Invitation check valid', {
          'community': response.communityName,
          'inviter': response.inviterName,
        });
      } else {
        _log.info('Invitation check invalid', {
          'error': response.errorMessage,
        });
      }

      return InvitationCheckResult(
        isValid: response.isValid,
        communityId: response.communityId,
        communityName: response.communityName,
        inviterName: response.inviterName,
        errorMessage: response.errorMessage,
        numMembers: response.numMembers,
        maxMembers: response.maxMembers,
        communityImageUrl: response.communityImageUrl,
        targetKind: response.hasTargetKind() ? response.targetKind : null,
        targetId: response.hasTargetId() ? response.targetId : null,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Invitation check failed with ConnectException', {
        'code': e.code.name,
        'message': e.message,
      });
      return InvitationCheckResult(
        isValid: false,
        communityId: '',
        communityName: '',
        inviterName: '',
        errorMessage: e.message,
      );
    } catch (e) {
      _log.error('Invitation check failed with exception', e);
      return InvitationCheckResult(
        isValid: false,
        communityId: '',
        communityName: '',
        inviterName: '',
        errorMessage: RpcErrorHandler.getTransportErrorMessage(e),
      );
    }
  }

  /// RequestPasswordReset initiates a password reset by sending an email.
  Future<PasswordResetRequestResult> requestPasswordReset({
    required String email,
  }) async {
    try {
      _log.info('Requesting password reset', {
        'email': LogRedactor.maskEmail(email),
      });

      final request = RequestPasswordResetRequest(email: email);
      await _client.requestPasswordReset(request, headers: _buildHeaders());

      _log.info('Password reset request successful');

      return PasswordResetRequestResult(success: true);
    } on connect.ConnectException catch (e) {
      _log.warning('Password reset request failed', {
        'code': e.code.name,
        'message': e.message,
      });
      return PasswordResetRequestResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Password reset request failed', e);
      return PasswordResetRequestResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }

  /// CheckResetPasswordToken validates a password reset token.
  Future<ResetTokenCheckResult> checkResetPasswordToken({
    required String token,
  }) async {
    try {
      _log.info('Checking password reset token');

      final request = CheckResetPasswordTokenRequest(token: token);
      final response = await _client.checkResetPasswordToken(request, headers: _buildHeaders());

      if (response.isValid) {
        _log.info('Token check valid', {
          'email': LogRedactor.maskEmail(response.email),
        });
      } else {
        _log.info('Token check invalid', {
          'error': response.errorMessage,
        });
      }

      return ResetTokenCheckResult(
        isValid: response.isValid,
        email: response.email,
        errorMessage: response.errorMessage,
      );
    } on connect.ConnectException catch (e) {
      _log.warning('Token check failed', {
        'code': e.code.name,
      });
      return ResetTokenCheckResult(
        isValid: false,
        email: '',
        errorMessage: e.message,
      );
    } catch (e) {
      _log.error('Token check failed', e);
      return ResetTokenCheckResult(
        isValid: false,
        email: '',
        errorMessage: RpcErrorHandler.getTransportErrorMessage(e),
      );
    }
  }

  /// ResetPassword completes a password reset with a new password.
  Future<PasswordResetResult> resetPassword({
    required String token,
    required String newPassword,
  }) async {
    try {
      _log.info('Resetting password');

      final request = ResetPasswordRequest(token: token, newPassword: newPassword);
      await _client.resetPassword(request, headers: _buildHeaders());

      _log.info('Password reset successful');

      return PasswordResetResult(success: true);
    } on connect.ConnectException catch (e) {
      _log.warning('Password reset failed', {
        'code': e.code.name,
        'message': e.message,
      });
      return PasswordResetResult(success: false, error: e.message);
    } catch (e) {
      _log.error('Password reset failed', e);
      return PasswordResetResult(success: false, error: RpcErrorHandler.getTransportErrorMessage(e));
    }
  }
}

/// EmailCodeResult is the outcome of asking for a one-time sign-in code.
///
/// Success means the request was accepted — it says nothing about whether an
/// account exists for the address, by design.
class EmailCodeResult {
  final bool success;
  final String? error;

  /// The address has no account, on a surface that requires one. Distinct from
  /// a generic failure so the UI can say so plainly rather than showing a
  /// send error for something that was never sent.
  final bool noAccount;

  /// The issued code, echoed back only by a development server so automated
  /// tests can complete the flow without a mailbox. Always null in production.
  final String? devCode;

  EmailCodeResult({
    required this.success,
    this.error,
    this.devCode,
    this.noAccount = false,
  });
}

/// EmailProofResult carries the short-lived proof that the caller can receive
/// mail at an address. Pass [emailProofToken] to register or login in place of
/// a password; it is not a session token and cannot authorize requests.
class EmailProofResult {
  final bool success;
  final String? error;
  final String? emailProofToken;

  /// Whether an account already exists for the address.
  ///
  /// Only meaningful once ownership is proven — which is the only point the
  /// server discloses it. Lets the flow sign a returning member straight in
  /// instead of asking for a display name they already chose.
  final bool accountExists;

  EmailProofResult({
    required this.success,
    this.error,
    this.emailProofToken,
    this.accountExists = false,
  });
}

/// AuthResult contains the result of an authentication operation.
class AuthResult {
  final bool success;
  final User? user;
  final String? accessToken;
  final String? refreshToken;
  final String? error;

  /// The Connect error code on failure (null on success / transport errors).
  /// Lets callers branch on the failure kind — e.g. PhoneRegister returning
  /// [connect.Code.alreadyExists] for an existing account, so the phone-first
  /// flow can fall back to login instead of erroring (#2492).
  final connect.Code? errorCode;

  AuthResult({
    required this.success,
    this.user,
    this.accessToken,
    this.refreshToken,
    this.error,
    this.errorCode,
  });
}

/// InvitationCheckResult contains the result of checking an invitation token.
class InvitationCheckResult {
  final bool isValid;
  final String communityId;
  final String communityName;
  final String inviterName;
  final String errorMessage;
  final int numMembers;
  final int maxMembers;
  final String communityImageUrl;

  /// What kind of target the share link points at. `null` for the
  /// bootstrap-user case or when the server response predates the
  /// target_kind field. Use [isEvent] for the common branch.
  final ShareLinkTargetKind? targetKind;

  /// When the share link points at a specific item, the ID of that
  /// item. `null` for community-invite, bootstrap, or unsupported
  /// variants.
  final String? targetId;

  InvitationCheckResult({
    required this.isValid,
    required this.communityId,
    required this.communityName,
    required this.inviterName,
    required this.errorMessage,
    this.numMembers = 0,
    this.maxMembers = 32,
    this.communityImageUrl = '',
    this.targetKind,
    this.targetId,
  });

  /// Returns true if the community is at maximum capacity.
  /// Note: maxMembers == 0 indicates a bootstrap case (no tier set) and should not be considered full.
  bool get isCommunityFull => maxMembers > 0 && numMembers >= maxMembers;

  /// True when the share link points at a specific event. Clients use
  /// this to branch to the event-aware landing screen.
  bool get isEvent =>
      targetKind == ShareLinkTargetKind.SHARE_LINK_TARGET_KIND_EVENT &&
      (targetId?.isNotEmpty ?? false);
}

/// PasswordResetRequestResult contains the result of requesting a password reset.
class PasswordResetRequestResult {
  final bool success;
  final String? error;

  PasswordResetRequestResult({
    required this.success,
    this.error,
  });
}

/// ResetTokenCheckResult contains the result of checking a password reset token.
class ResetTokenCheckResult {
  final bool isValid;
  final String email;
  final String errorMessage;

  ResetTokenCheckResult({
    required this.isValid,
    required this.email,
    required this.errorMessage,
  });
}

/// PasswordResetResult contains the result of completing a password reset.
class PasswordResetResult {
  final bool success;
  final String? error;

  PasswordResetResult({
    required this.success,
    this.error,
  });
}

/// TokenRefreshResult contains the result of a token refresh operation.
class TokenRefreshResult {
  final bool success;
  final String? accessToken;
  final String? refreshToken;

  TokenRefreshResult({
    required this.success,
    this.accessToken,
    this.refreshToken,
  });
}
