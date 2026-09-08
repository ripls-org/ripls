import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/auth/register_screen.dart';
import 'package:ripls/presentation/viewmodels/register_view_model.dart';
import 'package:ripls/presentation/widgets/oidc_sign_in_buttons.dart';
import 'package:ripls/services/providers.dart';

class _StubRegisterNotifier extends RegisterNotifier {
  @override
  RegisterState build() {
    return const RegisterState(
      invitationVerified: true,
      communityId: 'community-1',
      communityName: 'Test Community',
      inviterName: 'Test Inviter',
    );
  }

  @override
  Future<void> checkShortCode(String shortCode) async {
    // No-op: invitation already verified by build().
  }
}

void main() {
  Widget pumpRegister({
    String? shortCode = 'INVITE',
  }) {
    return ProviderScope(
      overrides: [
        observabilityServiceProvider.overrideWithValue(ObservabilityService()),
        registerProvider.overrideWith(_StubRegisterNotifier.new),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: RegisterScreen(
          shortCode: shortCode,
        ),
      ),
    );
  }

  group('RegisterScreen with invitation', () {
    testWidgets('shows phone and Google as primary actions', (tester) async {
      await tester.pumpWidget(pumpRegister());
      await tester.pumpAndSettle();

      expect(find.text('Register with Phone'), findsOneWidget);
      expect(find.byType(GoogleSignInButton), findsOneWidget);
    });

    testWidgets('phone button appears above the Google button', (tester) async {
      await tester.pumpWidget(pumpRegister());
      await tester.pumpAndSettle();

      final phoneY = tester.getTopLeft(find.text('Register with Phone')).dy;
      final googleY = tester.getTopLeft(find.byType(GoogleSignInButton)).dy;
      expect(phoneY, lessThan(googleY));
    });

    testWidgets('email registration form is collapsed initially',
        (tester) async {
      await tester.pumpWidget(pumpRegister());
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('register_email_expander')), findsOneWidget);
      expect(find.byKey(const ValueKey('email_auth_address_input')), findsNothing);
    });

    // Email sign-up is a mailed code now, not a password (#2571), so the
    // expander opens on an address field and a "Send code" button — there is
    // no password or confirm-password field to reveal.
    testWidgets('tapping the email expander reveals the code sign-up flow',
        (tester) async {
      await tester.pumpWidget(pumpRegister());
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('register_email_expander')));
      await tester.pumpAndSettle();

      expect(find.byKey(const ValueKey('email_auth_address_input')), findsOneWidget);
      expect(find.text('Email me a code'), findsOneWidget);
      expect(find.text('Confirm Password'), findsNothing);
    });

    testWidgets('no "You\'re Invited" hero — chooser shows directly (#2492)',
        (tester) async {
      await tester.pumpWidget(pumpRegister());
      await tester.pumpAndSettle();

      // The "You're Invited!" hero (community name + image) was removed for all
      // paths; the auth chooser is the first thing on the screen.
      expect(find.text("You're Invited!"), findsNothing);
      expect(find.text('Test Community'), findsNothing);
      expect(find.text('Register with Phone'), findsOneWidget);
    });
  });

  group('RegisterScreen without invitation', () {
    testWidgets('shows invitation-required message when shortCode is null',
        (tester) async {
      await tester.pumpWidget(pumpRegister(shortCode: null));
      await tester.pumpAndSettle();

      expect(find.text('Have an invite code?'), findsOneWidget);
      expect(find.byKey(const Key('register_email_expander')), findsNothing);
    });
  });
}
