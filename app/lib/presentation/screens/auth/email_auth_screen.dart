import 'package:connectrpc/connect.dart' as connect;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/auth/otp_code_field.dart';
import 'package:ripls/services/auth_service.dart';
import 'package:ripls/services/providers.dart';

/// Which step of the email code flow is on screen.
enum EmailAuthStep {
  /// Collecting the address to mail a code to.
  address,

  /// Collecting the code that was mailed.
  code,

  /// Collecting a display name — only reached when the address turns out to
  /// have no account yet.
  name,
}

/// Passwordless email sign-in: enter an address, receive a 6-digit code, enter
/// it, and you are in (#2571).
///
/// This deliberately mirrors [PhoneAuthScreen] step for step. The two sit next
/// to each other as sign-in options, so a flow that typed a code in one place
/// and clicked a link in the other would read as two unrelated features — which
/// is why this is a code rather than a magic link.
///
/// Sign-up and sign-in are the *same* flow, decided after the code is verified
/// rather than before. Asking "do you have an account?" up front is a question
/// people get wrong, and answering it server-side before verification would
/// leak who has an account. So: verify first, then try to register, and treat
/// an "already exists" as a returning member and log them in with the proof
/// already in hand — the same fallback `PhoneAuthScreen` uses (#2492).
class EmailAuthScreen extends ConsumerStatefulWidget {
  /// Invitation short code threaded into registration (empty when none).
  final String? shortCode;

  /// Called once the session is saved, so the host navigates itself.
  final VoidCallback onAuthenticated;

  /// Whether this host can only sign in, never register.
  ///
  /// Set by the login screen, which carries no invite code. The server then
  /// rejects an unknown address up front instead of mailing a code that leads
  /// to "no account found" after the person has typed it.
  final bool requireExistingAccount;

  /// Overrides the final button's label. The phone-first guest screen names
  /// the action being completed ("Finish RSVP") rather than the account being
  /// made, since that is what the guest came to do.
  final String? submitLabel;

  const EmailAuthScreen({
    super.key,
    required this.onAuthenticated,
    this.shortCode,
    this.submitLabel,
    this.requireExistingAccount = false,
  });

  @override
  ConsumerState<EmailAuthScreen> createState() => _EmailAuthScreenState();
}

class _EmailAuthScreenState extends ConsumerState<EmailAuthScreen> {
  final _emailController = TextEditingController();
  final _codeController = TextEditingController();
  final _nameController = TextEditingController();

  EmailAuthStep _step = EmailAuthStep.address;
  bool _isLoading = false;
  String? _errorMessage;

  /// Held between the code step and the name step: proving the address once is
  /// enough for both the register attempt and the login fallback.
  String? _emailProofToken;

  /// The code, when the server handed it back instead of only mailing it.
  ///
  /// Only a development server populates this, so its mere presence is the
  /// signal — the client does not (and cannot) second-guess it with a build
  /// flavor check. A production server never sends it, which is what makes the
  /// autofill below unreachable there; a prod-flavored build pointed at a local
  /// dev server should still get the convenience.
  ///
  /// It exists so someone poking at the app can register a made-up address
  /// without a mailbox — the manual counterpart to the e2e harness reading the
  /// same field over RPC.
  String? _devCode;

  @override
  void initState() {
    super.initState();
    // Every button's enabled state derives from field contents.
    _emailController.addListener(_onInputChanged);
    _codeController.addListener(_onInputChanged);
    _nameController.addListener(_onInputChanged);
  }

  @override
  void dispose() {
    _emailController.removeListener(_onInputChanged);
    _codeController.removeListener(_onInputChanged);
    _nameController.removeListener(_onInputChanged);
    _emailController.dispose();
    _codeController.dispose();
    _nameController.dispose();
    super.dispose();
  }

  void _onInputChanged() => setState(() {});

  String get _email => _emailController.text.trim();

  bool get _isEmailValid => _email.isNotEmpty && _email.contains('@');

  Future<void> _handleSendCode() async {
    if (!_isEmailValid) {
      setState(() => _errorMessage = context.l10n.emailAuthEmailValidationEmpty);
      return;
    }

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final result = await ref.read(authServiceProvider).requestEmailCode(
            email: _email,
            requireExistingAccount: widget.requireExistingAccount,
          );
      if (!mounted) return;

      if (!result.success) {
        // Stay on the address step for a missing account: the person needs to
        // correct the address, not enter a code that was never sent.
        setState(() => _errorMessage = result.noAccount
            ? context.l10n.emailAuthNoAccount
            : context.l10n.emailAuthCodeSendFailed);
        return;
      }

      setState(() {
        _step = EmailAuthStep.code;
        _devCode = result.devCode;
        // Fill it in rather than only showing it: on a dev server the code is
        // pure ceremony, and typing it back is the one step that makes
        // "register a throwaway account" slower than it used to be.
        _codeController.text = _devCode ?? '';
      });
      if (!mounted) return;
      await SemanticAnnouncer.announce(context, context.l10n.emailAuthCodeSent(_email));
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _handleVerifyCode() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final proof = await ref
          .read(authServiceProvider)
          .verifyEmailCode(email: _email, code: _codeController.text.trim());
      if (!mounted) return;

      if (!proof.success || proof.emailProofToken == null) {
        // The server collapses wrong/expired/used/exhausted into one failure so
        // the response can't reveal whether a code is outstanding; the copy
        // matches that by pointing at "request a new one" rather than guessing
        // which of them happened.
        setState(() => _errorMessage = context.l10n.emailAuthInvalidCode);
        return;
      }

      _emailProofToken = proof.emailProofToken;

      // A returning member already has a name — asking for one is both
      // pointless and alarming ("why is my login asking who I am?"). Sign them
      // straight in. Only a genuinely new address reaches the name step.
      if (proof.accountExists) {
        await _signInExisting();
        return;
      }

      // No account, and no invite code to register under: this host is a
      // sign-in surface, so collecting a name would lead to a registration the
      // server must reject. Say so plainly instead.
      if ((widget.shortCode ?? '').isEmpty) {
        setState(() => _errorMessage = context.l10n.emailAuthNoAccount);
        return;
      }

      setState(() => _step = EmailAuthStep.name);
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  /// Signs in an address the server confirmed already has an account.
  Future<void> _signInExisting() async {
    final proofToken = _emailProofToken;
    if (proofToken == null) return;

    final loggedIn = await ref
        .read(authServiceProvider)
        .loginWithEmailCode(email: _email, emailProofToken: proofToken);
    if (!mounted) return;

    if (!loggedIn.success) {
      setState(() => _errorMessage = loggedIn.error);
      return;
    }
    await _finishAuth(loggedIn, greetingName: loggedIn.user?.name ?? _email);
  }

  /// Completes the flow from the name step.
  ///
  /// Registration is attempted first and an `alreadyExists` is treated as "this
  /// is a returning member", falling back to sign-in with the proof already in
  /// hand — the same shape `PhoneAuthScreen._handlePhoneRegister` uses. The
  /// alternative, asking the server up front whether the address has an
  /// account, would be an account-existence oracle on an unauthenticated RPC.
  Future<void> _handleSubmitName() async {
    final name = _nameController.text.trim();
    final proofToken = _emailProofToken;
    if (name.isEmpty || proofToken == null) return;

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final authService = ref.read(authServiceProvider);

      final registered = await authService.registerWithEmailCode(
        email: _email,
        name: name,
        emailProofToken: proofToken,
        shortCode: widget.shortCode ?? '',
      );
      if (!mounted) return;

      if (registered.success) {
        await _finishAuth(registered, greetingName: name);
        return;
      }

      if (registered.errorCode != connect.Code.alreadyExists) {
        setState(() => _errorMessage = registered.error);
        return;
      }

      final loggedIn = await authService.loginWithEmailCode(
        email: _email,
        emailProofToken: proofToken,
      );
      if (!mounted) return;

      if (!loggedIn.success) {
        setState(() => _errorMessage = loggedIn.error);
        return;
      }
      await _finishAuth(loggedIn, greetingName: loggedIn.user?.name ?? name);
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _finishAuth(AuthResult result, {required String greetingName}) async {
    await ref
        .read(authStateProvider.notifier)
        .setAuthState(
          accessToken: result.accessToken!,
          user: result.user!,
          refreshToken: result.refreshToken,
        );
    if (!mounted) return;

    ToastHelper.showSuccess(context, context.l10n.emailAuthWelcome(greetingName));
    widget.onAuthenticated();
  }

  Widget _buildError(BuildContext context) {
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

  Widget _buildAddressStep(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          // Distinct key from the code field: the two swap keyboardType within
          // one screen, and a shared element strands the numeric keyboard.
          key: const ValueKey('email_auth_address_input'),
          controller: _emailController,
          keyboardType: TextInputType.emailAddress,
          autofocus: true,
          autofillHints: const [AutofillHints.email],
          decoration: InputDecoration(
            labelText: context.l10n.emailAuthEnterEmail,
            border: const OutlineInputBorder(),
            prefixIcon: const Icon(Icons.email),
          ),
          onSubmitted: (_) => _isEmailValid ? _handleSendCode() : null,
        ),
        const SizedBox(height: 16),
        _buildError(context),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _isLoading || !_isEmailValid ? null : _handleSendCode,
            child: _isLoading ? const _ButtonSpinner() : Text(context.l10n.emailAuthSendCode),
          ),
        ),
      ],
    );
  }

  Widget _buildCodeStep(BuildContext context) {
    // Gates the button and the keyboard's submit key alike: the two must agree,
    // or the keyboard can fire a second verification while the first is in
    // flight.
    final codeSubmittable =
        !_isLoading && OtpCodeField.isComplete(_codeController.text);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          context.l10n.emailAuthCodeSent(_email),
          style: Theme.of(context).textTheme.bodyLarge,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 16),
        OtpCodeField(
          fieldKey: const ValueKey('email_auth_code_input'),
          controller: _codeController,
          labelText: context.l10n.emailAuthEnterCode,
          onSubmitted: codeSubmittable ? _handleVerifyCode : null,
        ),
        const SizedBox(height: 16),
        _buildError(context),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: codeSubmittable ? _handleVerifyCode : null,
            child: _isLoading ? const _ButtonSpinner() : Text(context.l10n.authVerifyCode),
          ),
        ),
        const SizedBox(height: 8),
        TextButton(
          onPressed: _isLoading ? null : _handleSendCode,
          child: Text(context.l10n.authResendCode),
        ),
        TextButton(
          onPressed: _isLoading
              ? null
              : () => setState(() {
                  _step = EmailAuthStep.address;
                  _errorMessage = null;
                }),
          child: Text(context.l10n.emailAuthChangeEmail),
        ),
        const SizedBox(height: 8),
        Text(
          _devCode != null
              ? context.l10n.emailAuthDevCodeFilled
              : context.l10n.emailAuthCheckSpam,
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
            color: _devCode != null ? Theme.of(context).colorScheme.primary : null,
          ),
          textAlign: TextAlign.center,
        ),
      ],
    );
  }

  Widget _buildNameStep(BuildContext context) {
    final nameFilled = _nameController.text.trim().isNotEmpty;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          context.l10n.phoneAuthEnterName,
          style: Theme.of(context).textTheme.bodyLarge,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 16),
        TextField(
          key: const ValueKey('email_auth_name_input'),
          controller: _nameController,
          autofocus: true,
          textCapitalization: TextCapitalization.words,
          autofillHints: const [AutofillHints.name],
          decoration: InputDecoration(
            labelText: context.l10n.registerNameLabel,
            hintText: context.l10n.registerNameHint,
            border: const OutlineInputBorder(),
            prefixIcon: const Icon(Icons.person),
          ),
          onSubmitted: (_) => nameFilled ? _handleSubmitName() : null,
        ),
        const SizedBox(height: 16),
        _buildError(context),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _isLoading || !nameFilled ? null : _handleSubmitName,
            child: _isLoading
                ? const _ButtonSpinner()
                : Text(widget.submitLabel ?? context.l10n.registerButton),
          ),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    return switch (_step) {
      EmailAuthStep.address => _buildAddressStep(context),
      EmailAuthStep.code => _buildCodeStep(context),
      EmailAuthStep.name => _buildNameStep(context),
    };
  }
}

class _ButtonSpinner extends StatelessWidget {
  const _ButtonSpinner();

  @override
  Widget build(BuildContext context) {
    return const SizedBox(
      height: 20,
      width: 20,
      child: CircularProgressIndicator(strokeWidth: 2),
    );
  }
}
