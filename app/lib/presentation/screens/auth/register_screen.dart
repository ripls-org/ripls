import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/post_registration_destination.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/screens/auth/email_auth_screen.dart';
import 'package:ripls/presentation/screens/auth/phone_auth_screen.dart';
import 'package:ripls/presentation/viewmodels/register_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/oidc_sign_in_buttons.dart';
import 'package:ripls/services/oidc_service.dart';
import 'package:ripls/services/providers.dart';

import '../../widgets/adaptive/auth_screen_wrapper.dart';
import '../../widgets/keyboard_dismiss_wrapper.dart';

/// Parses a manual code input into an invitation short code.
///
/// Accepts:
/// - Bare code: `hTW82dhM`
/// - Full URL: `https://ripls.app/go/hTW82dhM`
/// - Custom scheme: `ripls://invite?token=hTW82dhM`
///
/// The deep-link target (if any) is resolved from the share-link row via
/// CheckInvitation, so legacy item-id query params are ignored.
String parseInviteCodeInput(String input) {
  final trimmed = input.trim();

  // Try parsing as a URL with /go/{code} path.
  final uri = Uri.tryParse(trimmed);
  if (uri != null && uri.hasScheme) {
    // https://ripls.app/go/CODE or http://localhost:8080/go/CODE
    final goMatch = RegExp(r'^/go/([^/?]+)').firstMatch(uri.path);
    if (goMatch != null) {
      return goMatch.group(1)!;
    }

    // ripls://invite?token=CODE
    if (uri.scheme == 'ripls' && uri.host == 'invite') {
      final token = uri.queryParameters['token'];
      if (token != null && token.isNotEmpty) {
        return token;
      }
    }
  }

  // Treat as bare short code.
  return trimmed;
}

/// RegisterScreen provides a user interface for new user registration.
class RegisterScreen extends ConsumerStatefulWidget {
  final String? shortCode;

  const RegisterScreen({
    super.key,
    this.shortCode,
  });

  @override
  ConsumerState<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends ConsumerState<RegisterScreen> {
  final _inviteCodeController = TextEditingController();
  final _oidcService = OIDCService();
  bool _isLoading = false;
  bool _emailExpanded = false;
  String? _errorMessage;
  bool _isCheckingManualCode = false;
  String? _manualCodeError;

  @override
  void initState() {
    super.initState();
    // Check invitation token when screen initializes
    if (widget.shortCode != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        ref.read(registerProvider.notifier).checkShortCode(widget.shortCode!);
      });
    }
  }


  @override
  void dispose() {
    _inviteCodeController.dispose();
    super.dispose();
  }

  Future<void> _handleGoogleSignIn() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final oidcResult = await _oidcService.signInWithGoogle();
      if (oidcResult == null) {
        // User cancelled
        setState(() {
          _isLoading = false;
        });
        return;
      }

      final authService = ref.read(authServiceProvider);
      final result = await authService.oidcRegister(
        provider: oidcResult.provider,
        idToken: oidcResult.idToken,
        shortCode: widget.shortCode ?? '',
      );

      if (!mounted) return;

      if (result.success) {
        await ref
            .read(authStateProvider.notifier)
            .setAuthState(
              accessToken: result.accessToken!,
              user: result.user!,
              refreshToken: result.refreshToken,
            );

        if (!mounted) return;

        context.go(_postRegistrationDestination());

        ToastHelper.showSuccess(context, 'Welcome, ${oidcResult.name}!');
      } else {
        setState(() {
          _errorMessage = result.error ?? 'Registration failed';
        });
      }
    } catch (e) {
      setState(() {
        _errorMessage = e.toString();
      });
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
  }

  Widget _buildErrorMessage(BuildContext context) {
    if (_errorMessage == null) return const SizedBox.shrink();

    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: LiveRegion(
        child: Text(
          _errorMessage!,
          style: TextStyle(color: Theme.of(context).colorScheme.error),
          textAlign: TextAlign.center,
        ),
      ),
    );
  }


  Widget _buildPrimaryAuthButtons() {
    return Column(
      children: [
        _buildPhoneRegisterButton(),
        const SizedBox(height: 12),
        GoogleSignInButton(
          onPressed: _handleGoogleTap,
          isLoading: _isLoading,
        ),
      ],
    );
  }

  Widget _buildPhoneRegisterButton() {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton.icon(
        onPressed: _isLoading ? null : _handlePhoneTap,
        icon: const Icon(Icons.phone),
        label: Text(context.l10n.registerWithPhone),
      ),
    );
  }

  void _handlePhoneTap() {
    ref
        .read(observabilityServiceProvider)
        .logAnalyticsEvent(RegisterMethodSelectedEvent(method: 'phone'));
    _navigateToPhoneAuth();
  }

  void _handleGoogleTap() {
    ref
        .read(observabilityServiceProvider)
        .logAnalyticsEvent(RegisterMethodSelectedEvent(method: 'google'));
    _handleGoogleSignIn();
  }

  Widget _buildOrDivider(BuildContext context) {
    return Row(
      children: [
        const Expanded(child: Divider()),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Text(context.l10n.loginOrDivider),
        ),
        const Expanded(child: Divider()),
      ],
    );
  }

  Widget _buildEmailExpander(BuildContext context) {
    return Theme(
      data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
      child: ExpansionTile(
        key: const Key('register_email_expander'),
        tilePadding: EdgeInsets.zero,
        childrenPadding: const EdgeInsets.only(top: 8),
        initiallyExpanded: _emailExpanded,
        onExpansionChanged: (expanded) {
          setState(() => _emailExpanded = expanded);
          if (expanded) {
            ref
                .read(observabilityServiceProvider)
                .logAnalyticsEvent(
                  RegisterMethodSelectedEvent(method: 'email'),
                );
          }
        },
        leading: const Icon(Icons.email),
        title: Text(context.l10n.registerWithEmail),
        children: [
          EmailAuthScreen(
            shortCode: widget.shortCode,
            onAuthenticated: () => context.go(_postRegistrationDestination()),
          ),
        ],
      ),
    );
  }

  void _navigateToPhoneAuth() {
    NavigationHelpers.pushWithSlide<void>(
      context: context,
      screen: PhoneAuthScreen(
        mode: PhoneAuthMode.register,
        shortCode: widget.shortCode,
      ),
    );
  }


  /// Delegates to the top-level [parseInviteCodeInput] for testability.
  String _parseCodeInput(String input) => parseInviteCodeInput(input);

  Future<void> _handleManualCodeSubmit() async {
    final rawInput = _inviteCodeController.text.trim();
    if (rawInput.isEmpty) return;

    final code = _parseCodeInput(rawInput);
    if (code.isEmpty) return;

    setState(() {
      _isCheckingManualCode = true;
      _manualCodeError = null;
    });

    try {
      // Validate and load invitation details via the ViewModel
      await ref.read(registerProvider.notifier).checkShortCode(code);

      if (!mounted) return;

      final registerState = ref.read(registerProvider);
      if (registerState.invitationVerified) {
        // Navigate to register with the code; the deep-link target is
        // resolved from the share-link row via CheckInvitation.
        context.go('/register?token=${Uri.encodeComponent(code)}');
      } else {
        setState(() {
          _manualCodeError = context.l10n.registerInviteCodeInvalid;
          _isCheckingManualCode = false;
        });
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _manualCodeError = context.l10n.registerInviteCodeInvalid;
        _isCheckingManualCode = false;
      });
    }
  }

  Widget _buildInviteCodeEntry() {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            context.l10n.registerInviteCodeLabel,
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary(context),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            context.l10n.registerInviteCodeHelp,
            style: TextStyle(
              fontSize: 13,
              color: AppColors.textSecondary(context),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _inviteCodeController,
            decoration: InputDecoration(
              hintText: context.l10n.registerInviteCodeHint,
              border: const OutlineInputBorder(),
              contentPadding: const EdgeInsets.symmetric(
                horizontal: 12,
                vertical: 12,
              ),
              errorText: _manualCodeError,
              errorMaxLines: 2,
            ),
            textCapitalization: TextCapitalization.none,
            autocorrect: false,
            enableSuggestions: false,
            onChanged: (_) => setState(() {
              _manualCodeError = null;
            }),
            onSubmitted: (_) => _handleManualCodeSubmit(),
          ),
          const SizedBox(height: 12),
          ElevatedButton(
            onPressed: _isCheckingManualCode ||
                    _inviteCodeController.text.trim().isEmpty
                ? null
                : _handleManualCodeSubmit,
            child: _isCheckingManualCode
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(context.l10n.registerInviteCodeSubmit),
          ),
        ],
      ),
    );
  }

  Widget _buildExistingAccountButton() {
    return SizedBox(
      width: double.infinity,
      child: OutlinedButton(
        onPressed: _isLoading ? null : _navigateToLogin,
        style: OutlinedButton.styleFrom(
          padding: const EdgeInsets.symmetric(vertical: 14),
        ),
        child: const Text('I already have an account'),
      ),
    );
  }

  /// Returns the URL the screen navigates to after a successful
  /// registration. Delegates to the shared
  /// `postRegistrationDestination` helper so both email/OIDC paths
  /// here and the phone-OTP path in `PhoneAuthScreen` route the user
  /// to the same place after auth completes.
  String _postRegistrationDestination() {
    return postRegistrationDestination(
      shortCode: widget.shortCode,
    );
  }

  void _navigateToLogin() {
    final shortCode = widget.shortCode;
    if (shortCode == null) {
      context.go('/login');
      return;
    }
    // The deep-link target resolves from the share-link row after login,
    // so only the token needs to survive the login round-trip.
    final redirect =
        Uri.encodeComponent('/invite?token=${Uri.encodeComponent(shortCode)}');
    context.go('/login?from=$redirect');
  }

  Widget _buildCommunityFullScreen(RegisterState registerState) {
    final primary = Theme.of(context).colorScheme.primary;
    return AuthScreenWrapper(
      child: Scaffold(
        appBar: AppBar(),
        body: SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Icon(
                  Icons.group_off,
                  size: 80,
                  color: AppColors.statusWarning(context),
                ),
                const SizedBox(height: 32),
                Text(
                  'Community is Full',
                  style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                        fontWeight: FontWeight.bold,
                        color: primary,
                      ),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 16),
                Text(
                  'Sorry, ${registerState.communityName} has reached its maximum capacity of ${registerState.maxMembers} members.',
                  style: Theme.of(context).textTheme.bodyLarge,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 16),
                Text(
                  "You can let ${registerState.inviterName} know that you weren't able to join.",
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: Colors.grey[600],
                      ),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 48),
                OutlinedButton(
                  onPressed: () => context.go('/'),
                  style: OutlinedButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 16),
                  ),
                  child: const Text(
                    'Go Home',
                    style: TextStyle(fontSize: 16),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildInvitationRequiredMessage() {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Column(
        children: [
          Icon(
            Icons.mail_outline,
            color: Theme.of(context).colorScheme.primary,
            size: 40,
          ),
          const SizedBox(height: 12),
          Text(
            context.l10n.registerInvitationRequired,
            style: TextStyle(
              color: AppColors.textPrimary(context),
              fontSize: 15,
            ),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          OutlinedButton.icon(
            onPressed: () => context.go('/login'),
            icon: const Icon(Icons.arrow_back_ios_new),
            label: Text(context.l10n.commonBackToLogin),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    // If no invitation token provided, show message and don't show form
    if (widget.shortCode == null) {
      return AuthScreenWrapper(
        child: Scaffold(
          appBar: AppBar(title: Text(context.l10n.registerJoinTitle)),
          body: KeyboardDismissWrapper(
            child: SafeArea(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(16),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const SizedBox(height: 16),
                    _buildInvitationRequiredMessage(),
                    const SizedBox(height: 16),
                    _buildInviteCodeEntry(),
                  ],
                ),
              ),
            ),
          ),
        ),
      );
    }

    // Has invitation token - show registration form
    final registerState = ref.watch(registerProvider);
    if (registerState.invitationVerified && registerState.isCommunityFull) {
      return _buildCommunityFullScreen(registerState);
    }

    return AuthScreenWrapper(
      child: Scaffold(
        appBar: AppBar(),
        body: KeyboardDismissWrapper(
          child: SafeArea(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildPrimaryAuthButtons(),
                  const SizedBox(height: 16),
                  // Google/OIDC errors surface here (the email path renders its
                  // own inline errors inside EmailAuthScreen).
                  _buildErrorMessage(context),
                  const SizedBox(height: 8),
                  _buildOrDivider(context),
                  const SizedBox(height: 8),
                  _buildEmailExpander(context),
                  const SizedBox(height: 24),
                  _buildExistingAccountButton(),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
