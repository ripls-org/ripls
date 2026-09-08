import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/auth/email_auth_screen.dart';
import 'package:ripls/presentation/screens/auth/phone_auth_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/environment_badge.dart';
import 'package:ripls/presentation/widgets/oidc_sign_in_buttons.dart';
import 'package:ripls/services/oidc_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../widgets/adaptive/auth_screen_wrapper.dart';
import '../../widgets/keyboard_dismiss_wrapper.dart';

/// LoginScreen provides a user interface for existing user login.
class LoginScreen extends ConsumerStatefulWidget {
  /// The path to redirect to after successful login
  final String? redirectTo;

  const LoginScreen({super.key, this.redirectTo});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  final _oidcService = OIDCService();
  bool _isLoading = false;
  bool _obscurePassword = true;
  bool _emailExpanded = false;

  /// True when the user has explicitly asked for the legacy password form.
  /// Email sign-in leads with a mailed code (#2571); this only flips on demand.
  bool _usePasswordFallback = false;
  String? _errorMessage;

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  Future<void> _handleLogin() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final authService = ref.read(authServiceProvider);
      final result = await authService.loginWithPassword(
        email: _emailController.text.trim(),
        password: _passwordController.text,
      );

      if (!mounted) return;

      if (result.success) {
        // Notify autofill to save credentials
        TextInput.finishAutofillContext();
        await SemanticAnnouncer.announce(context, context.l10n.a11yLoaded);

        // Save auth state
        await ref
            .read(authStateProvider.notifier)
            .setAuthState(accessToken: result.accessToken!, user: result.user!, refreshToken: result.refreshToken);

        if (!mounted) return;

        // Community loading is handled by the auth state listener in main.dart.
        // Starting a second load here caused a race condition (duplicate
        // setCommunities() calls) that could orphan the initial search.

        // Log analytics event
        unawaited(ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(LoginSuccessEvent(method: 'email')));

        // Navigate to the intended destination or home
        final destination = widget.redirectTo ?? '/';
        context.go(destination);
      } else {
        // Log analytics event for failure
        unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
              LoginFailedEvent(
                method: 'email',
                errorCode: result.error ?? 'unknown_error',
              ),
            ));
        setState(() {
          _errorMessage = result.error ?? context.l10n.loginFailed;
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
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
      final result = await authService.oidcLogin(
        provider: oidcResult.provider,
        idToken: oidcResult.idToken,
      );

      if (!mounted) return;

      if (result.success) {
        await ref
            .read(authStateProvider.notifier)
            .setAuthState(accessToken: result.accessToken!, user: result.user!, refreshToken: result.refreshToken);

        if (!mounted) return;

        // Log analytics event
        unawaited(ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(LoginSuccessEvent(method: 'google')));

        final destination = widget.redirectTo ?? '/';
        context.go(destination);
      } else {
        // Log analytics event for failure
        unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
              LoginFailedEvent(
                method: 'google',
                errorCode: result.error ?? 'unknown_error',
              ),
            ));
        setState(() {
          _errorMessage = result.error ?? context.l10n.loginFailed;
        });
      }
    } catch (e) {
      // Log analytics event for exception
      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            LoginFailedEvent(method: 'google', errorCode: e.toString()),
          ));
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

  Widget _buildHeader(BuildContext context) {
    return Column(
      children: [
        Text(
          context.l10n.loginWelcomeBack,
          style: Theme.of(context).textTheme.headlineMedium,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 12),
        const EnvironmentBadge(),
      ],
    );
  }

  Widget _buildEmailField(BuildContext context) {
    return TextFormField(
      key: const Key('login_email_field'),
      controller: _emailController,
      decoration: InputDecoration(
        labelText: context.l10n.loginEmailLabel,
        border: const OutlineInputBorder(),
        prefixIcon: const Icon(Icons.email),
      ),
      keyboardType: TextInputType.emailAddress,
      textInputAction: TextInputAction.next,
      autofillHints: const [AutofillHints.email, AutofillHints.username],
      validator: (value) {
        if (value == null || value.trim().isEmpty) {
          return context.l10n.loginEmailValidationEmpty;
        }
        if (!value.contains('@')) {
          return context.l10n.loginEmailValidationInvalid;
        }
        return null;
      },
    );
  }

  Widget _buildPasswordField(BuildContext context) {
    return TextFormField(
      key: const Key('login_password_field'),
      controller: _passwordController,
      decoration: InputDecoration(
        labelText: context.l10n.loginPasswordLabel,
        border: const OutlineInputBorder(),
        prefixIcon: const Icon(Icons.lock),
        suffixIcon: IconAction(
          icon: _obscurePassword ? Icons.visibility : Icons.visibility_off,
          semanticsLabel: _obscurePassword
              ? context.l10n.a11yShowPassword
              : context.l10n.a11yHidePassword,
          onPressed: () {
            setState(() {
              _obscurePassword = !_obscurePassword;
            });
          },
        ),
      ),
      obscureText: _obscurePassword,
      textInputAction: TextInputAction.done,
      autofillHints: const [AutofillHints.password],
      validator: (value) {
        if (value == null || value.isEmpty) {
          return context.l10n.loginPasswordValidationEmpty;
        }
        return null;
      },
      onFieldSubmitted: (_) => _handleLogin(),
    );
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

  Widget _buildLoginButton(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: _isLoading ? null : _handleLogin,
        child: _isLoading
            ? const SizedBox(
                height: 20,
                width: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : Text(context.l10n.loginButton),
      ),
    );
  }

  Widget _buildForgotPasswordLink(BuildContext context) {
    return Align(
      alignment: Alignment.centerRight,
      child: TextButton(
        onPressed: () => context.push('/forgot-password'),
        child: Text(context.l10n.loginForgotPassword),
      ),
    );
  }

  Widget _buildPrimaryAuthButtons(BuildContext context) {
    return Column(
      children: [
        _buildPhoneLoginButton(context),
        const SizedBox(height: 12),
        GoogleSignInButton(
          onPressed: _handleGoogleTap,
          isLoading: _isLoading,
        ),
      ],
    );
  }

  Widget _buildPhoneLoginButton(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton.icon(
        onPressed: _isLoading ? null : _handlePhoneTap,
        icon: const Icon(Icons.phone),
        label: Text(context.l10n.loginWithPhone),
      ),
    );
  }

  void _handlePhoneTap() {
    ref
        .read(observabilityServiceProvider)
        .logAnalyticsEvent(LoginMethodSelectedEvent(method: 'phone'));
    _navigateToPhoneLogin();
  }

  void _handleGoogleTap() {
    ref
        .read(observabilityServiceProvider)
        .logAnalyticsEvent(LoginMethodSelectedEvent(method: 'google'));
    _handleGoogleSignIn();
  }

  void _navigateToPhoneLogin() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PhoneAuthScreen(
          mode: PhoneAuthMode.login,
        ),
      ),
    );
  }

  Widget _buildEmailExpander(BuildContext context) {
    return Theme(
      data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
      child: ExpansionTile(
        key: const Key('login_email_expander'),
        tilePadding: EdgeInsets.zero,
        childrenPadding: const EdgeInsets.only(top: 8),
        initiallyExpanded: _emailExpanded,
        onExpansionChanged: (expanded) {
          setState(() => _emailExpanded = expanded);
          if (expanded) {
            ref
                .read(observabilityServiceProvider)
                .logAnalyticsEvent(
                  LoginMethodSelectedEvent(method: 'email'),
                );
          }
        },
        leading: const Icon(Icons.email),
        title: Text(context.l10n.loginWithEmail),
        children: _usePasswordFallback
            ? _buildPasswordSignIn(context)
            : _buildCodeSignIn(context),
      ),
    );
  }

  /// The default email sign-in: a mailed one-time code, no password (#2571).
  List<Widget> _buildCodeSignIn(BuildContext context) {
    return [
      EmailAuthScreen(
        onAuthenticated: () => context.go(widget.redirectTo ?? '/'),
        // "Log In", not the shared default "Create Account": this host is the
        // login screen, and it passes no invite short code, so a genuinely new
        // person cannot register from here anyway. Labelling the button
        // "Create Account" told a returning member they were making a second
        // account.
        submitLabel: context.l10n.loginButton,
        // Sign-in only: this screen carries no invite code, so an address with
        // no account is reported straight away rather than being mailed a code
        // that leads nowhere.
        requireExistingAccount: true,
      ),
      const SizedBox(height: 8),
      // The escape hatch for the accounts that still hold a password. It is
      // deliberately a plain text button below the fold rather than a peer of
      // the code flow: password sign-in is being retired, and this exists so
      // nobody is stranded mid-migration, not as a standing choice.
      TextButton(
        key: const Key('login_use_password_instead'),
        onPressed: () => setState(() => _usePasswordFallback = true),
        child: Text(context.l10n.loginUsePasswordInstead),
      ),
    ];
  }

  /// Legacy password sign-in. Removed once a release carrying the code flow has
  /// soaked and password sign-ins have gone to zero (#2571).
  List<Widget> _buildPasswordSignIn(BuildContext context) {
    return [
      AutofillGroup(
        child: Column(
          children: [
            _buildEmailField(context),
            const SizedBox(height: 16),
            _buildPasswordField(context),
          ],
        ),
      ),
      const SizedBox(height: 8),
      _buildForgotPasswordLink(context),
      const SizedBox(height: 16),
      _buildErrorMessage(context),
      _buildLoginButton(context),
      const SizedBox(height: 8),
      TextButton(
        key: const Key('login_use_code_instead'),
        onPressed: () => setState(() {
          _usePasswordFallback = false;
          _errorMessage = null;
        }),
        child: Text(context.l10n.loginUseCodeInstead),
      ),
    ];
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

  Future<void> _launchSupportEmail() async {
    final uri = Uri.parse('mailto:${Environment.supportEmail}');
    if (await canLaunchUrl(uri)) {
      await launchUrl(uri);
    }
  }

  Widget _buildNewUserMessage(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Column(
        children: [
          Icon(
            Icons.info_outline,
            color: AppColors.textSecondary(context),
            size: 24,
          ),
          const SizedBox(height: 8),
          Text(
            context.l10n.loginNewToRipls,
            style: TextStyle(
              color: AppColors.textPrimary(context),
              fontSize: 16,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            context.l10n.loginInvitationRequired,
            style: TextStyle(
              color: AppColors.textSecondary(context),
              fontSize: 14,
            ),
            textAlign: TextAlign.center,
          ),
          // Offered only when this deployment supplies an address (#2953);
          // a "contact support" link with nothing behind it is worse than
          // no link at all.
          if (Environment.supportEmail.isNotEmpty) ...[
            const SizedBox(height: 12),
            Tappable(
              semanticsLabel:
                  context.l10n.loginContactSupport(Environment.supportEmail),
              isLink: true,
              onTap: _launchSupportEmail,
              child: Container(
                constraints: const BoxConstraints(minHeight: 48),
                alignment: Alignment.center,
                padding:
                    const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                child: Text(
                  context.l10n.loginContactSupport(Environment.supportEmail),
                  style: TextStyle(
                    color: Theme.of(context).colorScheme.primary,
                    fontSize: 14,
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AuthScreenWrapper(
      child: Scaffold(
        appBar: AppBar(title: Text(context.l10n.loginScreenTitle)),
        body: KeyboardDismissWrapper(
          child: SafeArea(
            child: SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: Form(
              key: _formKey,
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildHeader(context),
                  const SizedBox(height: 32),
                  _buildPrimaryAuthButtons(context),
                  const SizedBox(height: 24),
                  _buildOrDivider(context),
                  const SizedBox(height: 8),
                  _buildEmailExpander(context),
                  const SizedBox(height: 24),
                  _buildNewUserMessage(context),
                ],
              ),
            ),
          ),
        ),
      ),
    ),
  );
  }
}
