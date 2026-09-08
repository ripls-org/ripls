import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/auth/email_auth_screen.dart';

/// Widget tests for the passwordless email sign-in screen (#2571).
///
/// These cover the parts that do not need a server: what renders, when the
/// buttons are live, and the two layout guarantees that have bitten this
/// surface before. The RPC round-trip is covered server-side in
/// `server/services/login/email_otp_test.go`.
void main() {
  Widget wrap(Widget child) {
    return ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: child),
      ),
    );
  }

  testWidgets('opens on the address step and gates Send code on a valid address',
      (tester) async {
    await tester.pumpWidget(
      wrap(EmailAuthScreen(shortCode: 'INVITE', onAuthenticated: () {})),
    );
    await tester.pumpAndSettle();

    final sendButton = find.widgetWithText(ElevatedButton, 'Email me a code');
    expect(sendButton, findsOneWidget);
    expect(
      tester.widget<ElevatedButton>(sendButton).onPressed,
      isNull,
      reason: 'Email me a code should be disabled before an address is entered',
    );

    // An address without an @ is still not sendable.
    await tester.enterText(find.byType(TextField), 'not-an-address');
    await tester.pumpAndSettle();
    expect(
      tester.widget<ElevatedButton>(sendButton).onPressed,
      isNull,
      reason: 'Email me a code should stay disabled for an address with no @',
    );

    await tester.enterText(find.byType(TextField), 'sam@example.com');
    await tester.pumpAndSettle();
    expect(
      tester.widget<ElevatedButton>(sendButton).onPressed,
      isNotNull,
      reason: 'Email me a code should be enabled once the address looks like one',
    );
  });

  // The address field and the code field swap keyboardType (emailAddress →
  // number) within a single screen. Without distinct keys Flutter reuses the
  // element and the keyboard stays on the previous type, which is a bug this
  // codebase has hit before.
  testWidgets('the address and code fields carry distinct keys', (tester) async {
    await tester.pumpWidget(
      wrap(EmailAuthScreen(onAuthenticated: () {})),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('email_auth_address_input')), findsOneWidget);
    expect(find.byKey(const ValueKey('email_auth_code_input')), findsNothing);
  });

  // The dev-code autofill is what keeps "make up an address and register" a
  // two-tap operation while playing with the app locally. It is driven purely
  // by the server echoing the code back, which only a dev server does — so on
  // the address step, before any response, there is nothing filled in and
  // nothing claiming there is.
  testWidgets('shows no dev-code affordance before the server responds',
      (tester) async {
    await tester.pumpWidget(
      wrap(EmailAuthScreen(onAuthenticated: () {})),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('Dev server'), findsNothing);
    expect(
      tester.widget<TextField>(find.byType(TextField)).controller!.text,
      isEmpty,
      reason: 'nothing is prefilled until a dev server hands a code back',
    );
  });

  // The code field's own input handling and accessibility guarantees live in
  // test/presentation/widgets/auth/otp_code_field_test.dart, alongside the
  // widget. That this screen labels the field from the localizations rather
  // than a literal is enforced structurally instead: dropping
  // `context.l10n.emailAuthEnterCode` would orphan the ARB key and fail
  // `npm run lint:dart:arb-orphans`.

  /// Reproduces PhoneAuthScreen's body scaffolding — a SingleChildScrollView
  /// with the email option sitting BELOW the fold (the phone/OTP inputs push it
  /// down) — and proves that when the software keyboard appears and the field
  /// is focused, it scrolls up into the visible area above the keyboard.
  ///
  /// This is the keyboard-avoidance guarantee for the inline email option
  /// (#2595), carried over from the password form this screen replaced:
  /// resizeToAvoidBottomInset (default true) shrinks the scroll viewport by the
  /// keyboard inset, and Flutter's EditableText auto-scrolls the focused field
  /// into view.
  testWidgets('a below-the-fold email field scrolls above the keyboard on focus',
      (tester) async {
    tester.view.physicalSize = const Size(400, 800);
    tester.view.devicePixelRatio = 1.0;
    const keyboardInset = 320.0;
    tester.view.viewInsets = const FakeViewPadding(bottom: keyboardInset);
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      wrap(
        SafeArea(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // Stand-in for the phone-first header + phone step that sit
                // above the email expander, pushing the field below the fold.
                const SizedBox(height: 650),
                EmailAuthScreen(shortCode: 'INVITE', onAuthenticated: () {}),
              ],
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    final addressField = find.byType(TextField).last;
    final visibleBottom = 800.0 - keyboardInset;

    await tester.showKeyboard(addressField);
    await tester.pumpAndSettle();

    final rect = tester.getRect(addressField);
    expect(
      rect.top,
      greaterThanOrEqualTo(0.0),
      reason: 'focused field should not be scrolled above the top',
    );
    expect(
      rect.bottom,
      lessThanOrEqualTo(visibleBottom + 1.0),
      reason: 'focused field should be visible above the on-screen keyboard '
          '(bottom ${rect.bottom} must clear the keyboard at $visibleBottom)',
    );
  });
}
