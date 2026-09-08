import 'package:connectrpc/connect.dart' as connect;
import 'package:firebase_auth/firebase_auth.dart' as fb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/post_registration_destination.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/services/oidc_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../widgets/accessibility/icon_action.dart';
import '../../widgets/adaptive/auth_screen_wrapper.dart';
import '../../widgets/auth/otp_code_field.dart';
import '../../widgets/keyboard_dismiss_wrapper.dart';
import '../../widgets/oidc_sign_in_buttons.dart';
import 'email_auth_screen.dart';
import 'phone_auth_header.dart';
import 'phone_consent_disclosure.dart';
import 'phone_verification_setup.dart';

final _log = Logger('PhoneAuthScreen');

/// Mode determines whether the phone auth screen handles registration, login,
/// or attaching a phone to the already-signed-in account.
enum PhoneAuthMode { register, login, attach }

/// PhoneAuthScreen handles the full phone authentication flow:
/// 1. Enter phone number → Firebase sends OTP
/// 2. Enter OTP code → Firebase verifies → get ID token
/// 3. (Register only) Enter name → call PhoneRegister RPC
/// 4. (Login only) Call PhoneLogin RPC directly after verification
/// 5. (Attach only) Call AddPhoneNumber RPC, then return to the caller
class PhoneAuthScreen extends ConsumerStatefulWidget {
  final PhoneAuthMode mode;
  final String? shortCode;
  final String? gearId;
  final String? requestId;
  final String? experienceId;

  /// Community the guest was invited to join, threaded from the web community
  /// guest entry (`WebCommunityScreen`) for a plain community invite (#2875).
  /// Null on mobile and on every item-flavored flow.
  final String? communityId;

  /// Which community surface to land on afterwards, threaded from the web
  /// community screen so a "say hi" deep link still opens the discussion after
  /// the guest verifies (#2876).
  final String? communityTab;

  /// RSVP intention threaded through from the Flutter Web guest entry
  /// point (`WebExperienceScreen`). On web, the post-success nav uses
  /// this to return the visitor to `/event/{id}?rsvp=…&code=…` so
  /// the auto-RSVP handoff fires. `null` on mobile and on non-event
  /// auth flows.
  final String? rsvpIntention;

  /// Item name + hero image for the phone-first guest screen — "Confirm your
  /// phone to RSVP to {eventName}" (event) / "…to claim {name}" (gear) / "…to
  /// help with {name}" (request) / "…to join {name}" (community) over a
  /// dark-washed hero (#2492, #2875). Threaded from the SSR landing since
  /// neither is fetchable pre-auth. The `event*` names are the shared
  /// web-guest carrier across every landing type.
  final String? eventName;
  final String? eventImageUrl;

  const PhoneAuthScreen({
    super.key,
    required this.mode,
    this.shortCode,
    this.gearId,
    this.requestId,
    this.experienceId,
    this.communityId,
    this.communityTab,
    this.rsvpIntention,
    this.eventName,
    this.eventImageUrl,
  });

  @override
  ConsumerState<PhoneAuthScreen> createState() => _PhoneAuthScreenState();
}

enum _PhoneAuthStep { phoneInput, otpInput, nameInput }

class _PhoneAuthScreenState extends ConsumerState<PhoneAuthScreen> {
  final _phoneController = TextEditingController();
  final _otpController = TextEditingController();
  final _nameController = TextEditingController();
  final _nameFocusNode = FocusNode();
  final _firebaseAuth = fb.FirebaseAuth.instance;

  _PhoneAuthStep _step = _PhoneAuthStep.phoneInput;
  bool _isLoading = false;
  String? _errorMessage;
  String? _verificationId;
  int? _resendToken;

  /// True once the Auth Emulator has been wired (e2e only). `useAuthEmulator`
  /// must precede the first Firebase Auth operation and should run at most once.
  /// The phone-register flow is the *only* screen that touches Firebase Auth, so
  /// the emulator is wired lazily here rather than at app startup — that keeps
  /// the `AUTH_EMULATOR_HOST` dart-define fully inert for every non-phone-auth
  /// spec (its startup round-trip to the emulator otherwise taxed every page
  /// load). Empty host in real builds, so this stays false in production.
  bool _emulatorWired = false;
  // Dedicated flag for the Google button so its spinner doesn't piggy-back on
  // the shared _isLoading (which the phone "Send Code" flow drives) — the G
  // should stay a G while the phone number is submitting.
  bool _isGoogleLoading = false;
  String _formattedPhone = '';
  final _oidcService = OIDCService();

  /// True when the screen was reached from an event invite (the SSR "I'm in" /
  /// "Maybe" path). Drives the phone-first "Confirm your phone to RSVP" framing
  /// and the below-the-fold email/Google options that replaced the separate
  /// register screen (#2492).
  bool get _isRsvpFlow =>
      widget.mode == PhoneAuthMode.register &&
      widget.experienceId != null &&
      (widget.rsvpIntention ?? '').isNotEmpty;

  /// True for any phone-first guest action flow — an event RSVP, a gear
  /// borrow/claim (WEB-4), a request offer (WEB-3), or a community join
  /// (#2875). Drives the shared framing (hero backdrop, back-to-landing app
  /// bar, inline secondary auth options, no back-to-login). Non-event guests
  /// carry a gear_id / request_id / community_id + the share code instead of
  /// an rsvp intention (#2492).
  bool get _isGuestActionFlow =>
      _isRsvpFlow ||
      (widget.mode == PhoneAuthMode.register &&
          (widget.shortCode ?? '').isNotEmpty &&
          ((widget.gearId ?? '').isNotEmpty ||
              (widget.requestId ?? '').isNotEmpty ||
              (widget.communityId ?? '').isNotEmpty));

  /// The entry point that brought this user to the phone step, which decides
  /// what the SMS consent disclosure says they're opting into. Reuses the same
  /// signals the header uses to frame its title, so the consent lead-in and the
  /// heading always describe the same thing (#2724).
  PhoneConsentContext get _consentContext {
    if ((widget.experienceId ?? '').isNotEmpty) return PhoneConsentContext.event;
    if ((widget.requestId ?? '').isNotEmpty) return PhoneConsentContext.request;
    if ((widget.gearId ?? '').isNotEmpty) return PhoneConsentContext.item;
    return PhoneConsentContext.community;
  }

  /// Which verb the phone-entry heading uses for a non-RSVP guest flow. Same
  /// precedence as [_consentContext] so the heading and the consent lead-in
  /// can never describe different things.
  GuestFlowKind get _guestFlowKind {
    if ((widget.requestId ?? '').isNotEmpty) return GuestFlowKind.request;
    if ((widget.gearId ?? '').isNotEmpty) return GuestFlowKind.gear;
    if ((widget.communityId ?? '').isNotEmpty) return GuestFlowKind.community;
    return GuestFlowKind.gear;
  }

  @override
  void initState() {
    super.initState();
    // Rebuild when text changes so the button enabled state updates.
    _phoneController.addListener(_onInputChanged);
    _otpController.addListener(_onInputChanged);
    _nameController.addListener(_onInputChanged);
  }

  void _onInputChanged() => setState(() {});

  @override
  void dispose() {
    _phoneController.removeListener(_onInputChanged);
    _otpController.removeListener(_onInputChanged);
    _nameController.removeListener(_onInputChanged);
    _phoneController.dispose();
    _otpController.dispose();
    _nameController.dispose();
    _nameFocusNode.dispose();
    super.dispose();
  }

  /// Lowercases the first letter of an item name so it reads naturally when
  /// embedded mid-sentence (gear/request titles are common nouns:
  /// "An extension ladder" → "an extension ladder"). Only the first character
  /// is touched, so interior capitals survive ("iPhone case" stays as-is).
  /// Normalizes a phone number to E.164 format. If the input doesn't start
  /// with '+', assumes US country code (+1).
  String _normalizePhoneNumber(String input) {
    // Strip everything except digits and leading '+'.
    final digits = input.replaceAll(RegExp(r'[^\d+]'), '');
    if (digits.startsWith('+')) return digits;
    return '+1$digits';
  }

  /// Whether to show the full-bleed, dark-washed item hero behind the
  /// phone-first guest screen (only when we were handed an image).
  bool get _hasEventBackdrop =>
      _isGuestActionFlow && (widget.eventImageUrl ?? '').isNotEmpty;

  @override
  Widget build(BuildContext context) {
    // Build the scaffold under a Builder so its context (and the inline _buildX
    // helpers it calls) is a *descendant* of the forced Theme below — those
    // helpers read Theme.of(context), so they must resolve the dark theme, not
    // the State's ambient context, which sits above the Theme.
    Widget scaffold = Builder(builder: _buildScaffold);
    if (_hasEventBackdrop) {
      // The RSVP hero sits behind a dark wash, so the screen must read
      // white-on-dark regardless of the system theme — otherwise light mode
      // paints dark text/inputs onto the dark backdrop (illegible). Force
      // [AppTheme.darkTheme] over this subtree (same idiom as the workshop
      // overlay host) so light and dark themes render identically.
      scaffold = Theme(data: AppTheme.darkTheme, child: scaffold);
    }
    return AuthScreenWrapper(child: scaffold);
  }

  Widget _buildScaffold(BuildContext context) {
    return Scaffold(
      // Guest action flow: no title (the in-body "Confirm your phone to …"
      // header carries it) and a transparent bar so the item hero shows behind.
      extendBodyBehindAppBar: _hasEventBackdrop,
      backgroundColor: _hasEventBackdrop ? Colors.black : null,
      appBar: _isGuestActionFlow
          ? AppBar(
              backgroundColor: Colors.transparent,
              elevation: 0,
              foregroundColor: _hasEventBackdrop ? Colors.white : null,
              // The guest arrived here via a route replacement, so there's
              // nothing to pop — back returns them to the SSR event landing.
              leading: IconAction(
                icon: Icons.arrow_back,
                semanticsLabel: context.l10n.a11yBack,
                color: _hasEventBackdrop ? Colors.white : null,
                onPressed: _backToEventLanding,
              ),
            )
          : AppBar(
              title: Text(
                widget.mode == PhoneAuthMode.attach
                    ? context.l10n.phoneAuthAddTitle
                    : context.l10n.phoneAuthTitle,
              ),
            ),
      // Route-level anchor for the Playwright e2e (WEB-2 full-loop). Sits at
      // Scaffold.body depth so it reliably reaches the flt-semantics DOM (see
      // docs/client/testing/semantics_identifiers.md); the spec scopes phone/
      // OTP/name input + button locators under it.
      body: Semantics(
        identifier: 'phone-auth-screen',
        explicitChildNodes: true,
        container: true,
        child: _withEventBackdrop(
          KeyboardDismissWrapper(
            child: SafeArea(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    PhoneAuthHeader(
                      isAttach: widget.mode == PhoneAuthMode.attach,
                      isPhoneInputStep: _step == _PhoneAuthStep.phoneInput,
                      isNameInputStep: _step == _PhoneAuthStep.nameInput,
                      isRsvpFlow: _isRsvpFlow,
                      isGuestActionFlow: _isGuestActionFlow,
                      eventName: widget.eventName,
                      guestFlowKind: _guestFlowKind,
                    ),
                    const SizedBox(height: 32),
                    _buildCurrentStep(context),
                    if (_errorMessage != null) ...[
                      const SizedBox(height: 16),
                      _buildErrorMessage(context),
                    ],
                    // A guest action flow never shows the generic back-to-login:
                    // the phone step carries its own "other ways to sign in"
                    // block, and the OTP/name steps are mid-account-creation (no
                    // "already have an account?" — they're past that decision).
                    // Attach mode has no login/register choice either — the app
                    // bar back cancels — so it's hidden too.
                    if (!_isGuestActionFlow &&
                        widget.mode != PhoneAuthMode.attach) ...[
                      const SizedBox(height: 24),
                      _buildBackToLogin(context),
                    ],
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// Stacks the event hero (dark-washed for legibility) behind [content] on the
  /// phone-first RSVP screen; passes [content] through unchanged otherwise.
  Widget _withEventBackdrop(Widget content) {
    if (!_hasEventBackdrop) return content;
    return Stack(
      fit: StackFit.expand,
      children: [
        Image.network(
          widget.eventImageUrl!,
          fit: BoxFit.cover,
          errorBuilder: (_, _, _) => const SizedBox.shrink(),
        ),
        // Dark wash — darker at top/bottom so the title and inputs stay legible.
        const DecoratedBox(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [Color(0xE6000000), Color(0xB3000000), Color(0xF2000000)],
              stops: [0.0, 0.4, 1.0],
            ),
          ),
        ),
        content,
      ],
    );
  }

  Widget _buildCurrentStep(BuildContext context) {
    switch (_step) {
      case _PhoneAuthStep.phoneInput:
        return _buildPhoneInputStep(context);
      case _PhoneAuthStep.otpInput:
        return _buildOtpInputStep(context);
      case _PhoneAuthStep.nameInput:
        return _buildNameInputStep(context);
    }
  }

  Widget _buildPhoneInputStep(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // The guest-action header already frames the phone field, so skip the
        // redundant "Enter your phone number" label there.
        if (!_isGuestActionFlow) ...[
          Text(
            context.l10n.phoneAuthEnterNumber,
            style: Theme.of(context).textTheme.bodyLarge,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
        ],
        TextField(
          key: const ValueKey('phone_input'),
          controller: _phoneController,
          keyboardType: TextInputType.phone,
          autofillHints: const [AutofillHints.telephoneNumber],
          decoration: InputDecoration(
            prefixIcon: const Icon(Icons.phone),
            hintText: '+1 (555) 123-4567',
            border: const OutlineInputBorder(),
            filled: true,
            fillColor: Theme.of(context).colorScheme.surfaceContainerHighest,
          ),
        ),
        const SizedBox(height: 16),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _isLoading || _phoneController.text.trim().isEmpty
                ? null
                : _handleSendCode,
            child: _isLoading
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(context.l10n.phoneAuthSendCode),
          ),
        ),
        const SizedBox(height: 12),
        // SMS opt-in consent disclosure + the RCS-required Terms/Privacy links
        // (#2492); extracted to PhoneConsentDisclosure to keep this file small.
        PhoneConsentDisclosure(entryContext: _consentContext),
        // Phone-first guest screen: the email/Google paths live below the fold
        // (they replaced the separate register screen for this flow, #2492).
        if (_isGuestActionFlow) _buildSecondaryAuthOptions(context),
      ],
    );
  }

  /// The below-the-fold "other ways to sign in" block on the phone-first RSVP
  /// screen — keeps the phone path primary while still supporting Google /
  /// email / existing-account sign-in.
  Widget _buildSecondaryAuthOptions(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 28),
        Row(
          children: [
            const Expanded(child: Divider()),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: Text(
                context.l10n.phoneAuthOtherOptions,
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
              ),
            ),
            const Expanded(child: Divider()),
          ],
        ),
        const SizedBox(height: 16),
        GoogleSignInButton(
          onPressed: _handleGoogleSignIn,
          isLoading: _isGoogleLoading,
        ),
        // The two provider options are peers, so they sit on the same rhythm
        // (matches the login screen's phone/Google pair).
        const SizedBox(height: 12),
        _buildEmailExpander(context),
        const SizedBox(height: 8),
        TextButton(
          onPressed: _isLoading ? null : _goToLogin,
          child: Text(context.l10n.registerHaveAccount),
        ),
      ],
    );
  }

  /// Inline email/password sign-up, expanded in place so the guest never leaves
  /// the phone-first screen (previously this route-hopped to the separate
  /// RegisterScreen, #2595). Hosts the shared [EmailRegisterForm]; on success
  /// its `onRegistered` runs `_navigateToHome`, which returns web guests to the
  /// item view so the auto-RSVP / interest / offer handoff still fires.
  ///
  /// Wears the same outlined-button treatment as the Google button above it:
  /// the two are one choice — which provider you continue with — so they read
  /// as one row of options rather than a button and a list item (#2724).
  /// The chevron stays: unlike Google, this one expands in place.
  Widget _buildEmailExpander(BuildContext context) {
    final theme = Theme.of(context);
    final outline = BorderSide(color: theme.colorScheme.outline);
    return Theme(
      data: theme.copyWith(dividerColor: Colors.transparent),
      child: ExpansionTile(
        key: const Key('phone_email_expander'),
        // Collapsed, this is a pill exactly like the Google button above it
        // (Material 3 gives OutlinedButton a StadiumBorder). Expanded, the
        // outline grows into a card around the form, so it softens to a
        // rounded rect — a stadium around a tall form reads as a mistake.
        collapsedShape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(24),
          side: outline,
        ),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(20),
          side: outline,
        ),
        tilePadding: const EdgeInsets.symmetric(horizontal: 16),
        childrenPadding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
        onExpansionChanged: (expanded) {
          if (expanded) {
            ref
                .read(observabilityServiceProvider)
                .logAnalyticsEvent(
                  RegisterMethodSelectedEvent(method: 'email'),
                );
          }
        },
        leading: const Icon(Icons.email_outlined, size: 24),
        title: Text(context.l10n.authContinueWithEmail),
        children: [
          EmailAuthScreen(
            shortCode: widget.shortCode,
            onAuthenticated: _navigateToHome,
            submitLabel: _isRsvpFlow
                ? context.l10n.phoneAuthFinishRsvp
                : _isGuestActionFlow
                ? context.l10n.phoneAuthFinishItem
                : null,
          ),
        ],
      ),
    );
  }

  Widget _buildOtpInputStep(BuildContext context) {
    // Gates the button and the keyboard's submit key alike: the two must agree,
    // or the keyboard can fire a second verification while the first is in
    // flight.
    final codeSubmittable =
        !_isLoading && OtpCodeField.isComplete(_otpController.text);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          context.l10n.phoneAuthCodeSent(_formattedPhone),
          style: Theme.of(context).textTheme.bodyLarge,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 16),
        OtpCodeField(
          fieldKey: const ValueKey('otp_input'),
          controller: _otpController,
          labelText: context.l10n.phoneAuthEnterCode,
          onSubmitted: codeSubmittable ? _handleVerifyCode : null,
        ),
        const SizedBox(height: 16),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: codeSubmittable ? _handleVerifyCode : null,
            child: _isLoading
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(context.l10n.authVerifyCode),
          ),
        ),
        const SizedBox(height: 8),
        TextButton(
          onPressed: _isLoading ? null : _handleResendCode,
          child: Text(context.l10n.authResendCode),
        ),
      ],
    );
  }

  Widget _buildNameInputStep(BuildContext context) {
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
          key: const ValueKey('name_input'),
          controller: _nameController,
          focusNode: _nameFocusNode,
          keyboardType: TextInputType.name,
          textInputAction: TextInputAction.done,
          textCapitalization: TextCapitalization.words,
          autofocus: true,
          autofillHints: const [AutofillHints.name],
          decoration: InputDecoration(
            prefixIcon: const Icon(Icons.person),
            hintText: context.l10n.phoneAuthNameHint,
            border: const OutlineInputBorder(),
            filled: true,
            fillColor: Theme.of(context).colorScheme.surfaceContainerHighest,
          ),
        ),
        const SizedBox(height: 16),
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _isLoading || _nameController.text.trim().isEmpty
                ? null
                : _handlePhoneRegister,
            child: _isLoading
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(
                    _isRsvpFlow
                        ? context.l10n.phoneAuthFinishRsvp
                        : _isGuestActionFlow
                        ? context.l10n.phoneAuthFinishItem
                        : context.l10n.phoneAuthRegisterButton,
                  ),
          ),
        ),
      ],
    );
  }

  Widget _buildErrorMessage(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        _errorMessage!,
        style: TextStyle(color: Theme.of(context).colorScheme.error),
        textAlign: TextAlign.center,
      ),
    );
  }

  Widget _buildBackToLogin(BuildContext context) {
    return TextButton(
      onPressed: () => Navigator.of(context).pop(),
      child: Text(
        widget.mode == PhoneAuthMode.register
            ? context.l10n.registerHaveAccount
            : context.l10n.loginNoAccount,
      ),
    );
  }

  // --- Action handlers ---

  Future<void> _handleSendCode() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    _formattedPhone = _normalizePhoneNumber(_phoneController.text.trim());

    // Sign-in only: refuse a number with no account BEFORE Firebase sends
    // anything. The code is sent from this device, so there is no declining it
    // afterwards — without this, signing in with an unregistered number costs a
    // real text message and still ends at "no account found", one OTP later.
    //
    // Register and attach modes skip the check: an unregistered number is the
    // normal case for both.
    if (widget.mode == PhoneAuthMode.login) {
      final registered =
          await ref.read(authServiceProvider).isPhoneRegistered(_formattedPhone);
      if (!mounted) return;
      if (!registered) {
        setState(() {
          _isLoading = false;
          _errorMessage = context.l10n.phoneAuthNoAccount;
        });
        return;
      }
    }

    _log.info('sending verification code to $_formattedPhone');

    // Platform/provider setup, both no-ops in a real production build. See
    // phone_verification_setup.dart for why each is done here rather than at
    // app startup.
    await ensureAPNsToken();
    _emulatorWired = await wireAuthEmulatorIfNeeded(
      _firebaseAuth,
      alreadyWired: _emulatorWired,
    );

    try {
      await _firebaseAuth.verifyPhoneNumber(
        phoneNumber: _formattedPhone,
        verificationCompleted: (fb.PhoneAuthCredential credential) async {
          _log.info('auto-verification completed (Android)');
          await _signInWithCredential(credential);
        },
        verificationFailed: (fb.FirebaseAuthException e) {
          _log.warning(
            'verification failed: code=${e.code} message=${e.message}',
          );
          if (!mounted) return;
          setState(() {
            _isLoading = false;
            _errorMessage = context.l10n.phoneAuthVerificationFailed;
          });
        },
        codeSent: (String verificationId, int? resendToken) {
          _log.info('OTP code sent, verificationId=$verificationId');
          if (!mounted) return;
          setState(() {
            _isLoading = false;
            _verificationId = verificationId;
            _resendToken = resendToken;
            _step = _PhoneAuthStep.otpInput;
          });
        },
        codeAutoRetrievalTimeout: (String verificationId) {
          _log.info('auto-retrieval timeout');
          _verificationId = verificationId;
        },
        forceResendingToken: _resendToken,
      );
    } catch (e) {
      _log.severe('verifyPhoneNumber threw: $e');
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _errorMessage = context.l10n.phoneAuthVerificationFailed;
      });
    }
  }

  Future<void> _handleVerifyCode() async {
    if (_verificationId == null) return;

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final credential = fb.PhoneAuthProvider.credential(
        verificationId: _verificationId!,
        smsCode: _otpController.text.trim(),
      );
      await _signInWithCredential(credential);
    } on fb.FirebaseAuthException catch (e) {
      _log.warning(
        'OTP verification failed: code=${e.code} message=${e.message}',
      );
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _errorMessage = context.l10n.phoneAuthInvalidCode;
      });
    }
  }

  Future<void> _handleResendCode() async {
    _otpController.clear();
    await _handleSendCode();
  }

  Future<void> _signInWithCredential(fb.PhoneAuthCredential credential) async {
    try {
      final userCredential = await _firebaseAuth.signInWithCredential(
        credential,
      );
      final idToken = await userCredential.user?.getIdToken();
      _log.info(
        'Firebase signIn succeeded, uid=${userCredential.user?.uid}, '
        'idToken=${idToken != null ? "present" : "null"}',
      );

      if (idToken == null) {
        _log.warning('Firebase signIn returned null idToken');
        if (!mounted) return;
        setState(() {
          _isLoading = false;
          _errorMessage = context.l10n.phoneAuthVerificationFailed;
        });
        return;
      }

      if (widget.mode == PhoneAuthMode.login) {
        await _completePhoneLogin(idToken);
      } else if (widget.mode == PhoneAuthMode.attach) {
        // Attaching to the signed-in account needs no name — go straight to the
        // AddPhoneNumber call and return to the caller.
        await _completeAttachPhone(idToken);
      } else {
        // For registration, move to name entry step.
        if (!mounted) return;
        setState(() {
          _isLoading = false;
          _step = _PhoneAuthStep.nameInput;
        });
        // Store the ID token for the registration call.
        _firebaseIdToken = idToken;
      }
    } on fb.FirebaseAuthException catch (e) {
      _log.warning(
        'Firebase signInWithCredential failed: code=${e.code} '
        'message=${e.message}',
      );
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _errorMessage = context.l10n.phoneAuthInvalidCode;
      });
    }
  }

  String? _firebaseIdToken;

  Future<void> _handlePhoneRegister() async {
    if (_firebaseIdToken == null) return;

    _log.info('calling PhoneRegister RPC');
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    final authService = ref.read(authServiceProvider);
    final result = await authService.phoneRegister(
      firebaseIdToken: _firebaseIdToken!,
      name: _nameController.text.trim(),
      shortCode: widget.shortCode ?? '',
    );

    if (!mounted) return;

    if (result.success) {
      _log.info('PhoneRegister succeeded');
      await ref
          .read(authStateProvider.notifier)
          .setAuthState(
            accessToken: result.accessToken!,
            user: result.user!,
            refreshToken: result.refreshToken,
          );
      if (!mounted) return;
      ToastHelper.showSuccess(context, 'Account created');
      _navigateToHome();
    } else if (result.errorCode == connect.Code.alreadyExists &&
        _firebaseIdToken != null) {
      // The phone is already on a Ripls account — the invitee is a returning
      // member, not a new sign-up. Log them in with the same verified token
      // instead of erroring; the post-auth handoff (auto-RSVP / interest /
      // offer) then fires exactly as it does after registration (#2492).
      _log.info('PhoneRegister: account exists, logging in instead');
      await _completePhoneLogin(_firebaseIdToken!);
    } else {
      _log.warning('PhoneRegister failed: ${result.error}');
      setState(() {
        _isLoading = false;
        _errorMessage = result.error;
      });
    }
  }

  Future<void> _completePhoneLogin(String idToken) async {
    _log.info('calling PhoneLogin RPC');
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    final authService = ref.read(authServiceProvider);
    final result = await authService.phoneLogin(firebaseIdToken: idToken);

    if (!mounted) return;

    if (result.success) {
      _log.info('PhoneLogin succeeded');
      await ref
          .read(authStateProvider.notifier)
          .setAuthState(
            accessToken: result.accessToken!,
            user: result.user!,
            refreshToken: result.refreshToken,
          );
      if (!mounted) return;
      ToastHelper.showSuccess(context, 'Logged in');
      _navigateToHome();
    } else {
      _log.warning('PhoneLogin failed: ${result.error}');
      setState(() {
        _isLoading = false;
        _errorMessage = result.error;
      });
    }
  }

  /// Attaches the verified phone to the signed-in account (attach mode), then
  /// pops back to the caller with the new number. No session change — the
  /// account stays the same; a token refresh just pulls the phone claim into
  /// the access token. On conflict (number on another account) the
  /// user-friendly error is shown inline so the user can back out.
  Future<void> _completeAttachPhone(String idToken) async {
    _log.info('calling AddPhoneNumber RPC');
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final phoneNumber = await ref
          .read(userServiceProvider)
          .addPhoneNumber(firebaseIdToken: idToken);

      // The account record changed server-side; refresh the access token so it
      // carries the phone claim immediately.
      await ref.read(authStateProvider.notifier).refreshAccessToken();

      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.phoneAuthAddedSuccess);
      Navigator.of(context).pop(phoneNumber);
    } on ServiceException catch (e) {
      _log.warning('AddPhoneNumber failed: ${e.message}');
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _errorMessage = e.message;
      });
    }
  }

  /// Returns the guest to the SSR event landing (/go/{code}) — where they saw
  /// the event and the I'm in / Maybe / I'm out choices. It's server-rendered,
  /// so a same-tab full navigation rather than an in-app route. Falls back to
  /// a pop / home when there's no share code.
  void _backToEventLanding() {
    final code = widget.shortCode;
    if (code == null || code.isEmpty) {
      if (Navigator.of(context).canPop()) {
        Navigator.of(context).pop();
      } else {
        context.go('/');
      }
      return;
    }
    launchUrl(
      Uri.base.resolve('/go/${Uri.encodeComponent(code)}'),
      webOnlyWindowName: '_self',
    );
  }

  void _goToLogin() {
    context.go('/login?from=${Uri.encodeComponent(_postAuthDestination())}');
  }

  /// Where the guest belongs once they're authenticated. Both the
  /// register-success path (`_navigateToHome`) and the existing-account hop
  /// (`_goToLogin`, which threads this through `?from=`) must pass the *same*
  /// carriers — dropping one here is how a guest who takes the below-the-fold
  /// Google/email option lands on `/` while the phone-OTP path works fine.
  String _postAuthDestination() {
    return postRegistrationDestination(
      shortCode: widget.shortCode,
      gearId: widget.gearId,
      requestId: widget.requestId,
      experienceId: widget.experienceId,
      communityId: widget.communityId,
      communityTab: widget.communityTab,
      rsvpIntention: widget.rsvpIntention,
    );
  }

  Future<void> _handleGoogleSignIn() async {
    setState(() {
      _isLoading = true;
      _isGoogleLoading = true;
      _errorMessage = null;
    });
    try {
      final oidcResult = await _oidcService.signInWithGoogle();
      if (oidcResult == null) {
        if (mounted) {
          setState(() {
            _isLoading = false;
            _isGoogleLoading = false;
          });
        }
        return; // user cancelled
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
        ToastHelper.showSuccess(context, 'Welcome, ${oidcResult.name}!');
        _navigateToHome();
      } else {
        setState(() {
          _isLoading = false;
          _isGoogleLoading = false;
          _errorMessage =
              result.error ?? context.l10n.phoneAuthVerificationFailed;
        });
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _isGoogleLoading = false;
        _errorMessage = e.toString();
      });
    }
  }

  void _navigateToHome() {
    if (!mounted) return;
    // Delegate to the shared helper so both this phone-OTP path and
    // RegisterScreen's email-code / OIDC paths land the user in
    // the same place. On Flutter Web, an event-flavored auth flow
    // routes back to /event/{id}?rsvp=…&code=… so the
    // WebEventRsvpHandoffNotifier picks up the rsvp intention.
    context.go(_postAuthDestination());
  }
}
