import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/auth/login_screen.dart';
import 'package:ripls/presentation/widgets/oidc_sign_in_buttons.dart';
import 'package:ripls/services/providers.dart';

void main() {
  Widget pumpLogin() {
    return ProviderScope(
      overrides: [
        observabilityServiceProvider.overrideWithValue(ObservabilityService()),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: const LoginScreen(),
      ),
    );
  }

  group('LoginScreen', () {
    testWidgets('shows phone and Google as primary actions', (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      expect(find.text('Log in with Phone'), findsOneWidget);
      expect(find.byType(GoogleSignInButton), findsOneWidget);
    });

    testWidgets('phone button appears above the Google button', (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      final phoneY = tester.getTopLeft(find.text('Log in with Phone')).dy;
      final googleY = tester.getTopLeft(find.byType(GoogleSignInButton)).dy;
      expect(phoneY, lessThan(googleY));
    });

    testWidgets('email form is collapsed initially', (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('login_email_expander')), findsOneWidget);
      expect(find.byKey(const Key('login_email_field')), findsNothing);
      expect(find.byKey(const Key('login_password_field')), findsNothing);
    });

    // Email sign-in leads with a mailed code (#2571). The password form is
    // still reachable for accounts that hold one, but only on request.
    testWidgets('tapping the email expander reveals the code sign-in flow',
        (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('login_email_expander')));
      await tester.pumpAndSettle();

      expect(find.byKey(const ValueKey('email_auth_address_input')), findsOneWidget);
      expect(find.byKey(const Key('login_password_field')), findsNothing);
      expect(find.byKey(const Key('login_use_password_instead')), findsOneWidget);
    });

    testWidgets('the legacy password form is reachable on request',
        (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('login_email_expander')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('login_use_password_instead')));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('login_email_field')), findsOneWidget);
      expect(find.byKey(const Key('login_password_field')), findsOneWidget);

      // …and the user can get back to the code flow without leaving the screen.
      // The password form is taller than the code form, so the return link can
      // sit below the test viewport — scroll it in rather than tapping blind.
      final backToCode = find.byKey(const Key('login_use_code_instead'));
      await tester.ensureVisible(backToCode);
      await tester.pumpAndSettle();
      await tester.tap(backToCode);
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('email_auth_address_input')), findsOneWidget);
    });

    testWidgets('email expander appears below the primary auth buttons',
        (tester) async {
      await tester.pumpWidget(pumpLogin());
      await tester.pumpAndSettle();

      final googleY =
          tester.getTopLeft(find.byType(GoogleSignInButton)).dy;
      final expanderY =
          tester.getTopLeft(find.byKey(const Key('login_email_expander'))).dy;
      expect(googleY, lessThan(expanderY));
    });
  });
}
