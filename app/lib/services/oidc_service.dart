import 'package:google_sign_in/google_sign_in.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/data/gen/ripls/api/login_service.pb.dart';

/// OIDCService handles OpenID Connect authentication flows for Google.
class OIDCService {
  final GoogleSignIn _googleSignIn = GoogleSignIn.instance;
  bool _initialized = false;

  /// Initialize Google Sign-In with serverClientId for Android OIDC support.
  /// This must be called before attempting to sign in.
  Future<void> _ensureInitialized() async {
    if (_initialized) return;

    final clientId = Environment.googleClientId;
    if (clientId.isNotEmpty) {
      await _googleSignIn.initialize(serverClientId: clientId);
    }
    _initialized = true;
  }

  /// SignInWithGoogle initiates Google sign-in and returns the ID token and user info.
  Future<OIDCSignInResult?> signInWithGoogle() async {
    try {
      await _ensureInitialized();

      final account = await _googleSignIn.authenticate(
        scopeHint: ['email', 'profile'],
      );

      final auth = account.authentication;
      final idToken = auth.idToken;

      if (idToken == null) {
        throw Exception('Failed to get ID token from Google');
      }

      return OIDCSignInResult(
        provider: OIDCProvider.OIDC_PROVIDER_GOOGLE,
        idToken: idToken,
        email: account.email,
        name: account.displayName ?? '',
      );
    } on GoogleSignInException catch (e) {
      // Check if this is actually a user cancellation vs a configuration error.
      // Error code 16 "Account reauth failed" means OAuth misconfiguration,
      // typically a missing Android OAuth client for this build's package name
      // and signing certificate in the identity provider's console.
      final errorStr = e.toString();
      if (e.code == GoogleSignInExceptionCode.canceled &&
          !errorStr.contains('reauth failed')) {
        return null; // User cancelled
      }
      // Re-throw configuration errors so they're displayed to the user
      rethrow;
    }
  }
  }

/// OIDCSignInResult contains the result of an OIDC sign-in operation.
class OIDCSignInResult {
  final OIDCProvider provider;
  final String idToken;
  final String email;
  final String name;

  OIDCSignInResult({
    required this.provider,
    required this.idToken,
    required this.email,
    required this.name,
  });
}