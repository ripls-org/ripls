import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/web_experience_screen.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';

import '../../../helpers/l10n_helpers.dart';

/// `_FakeAuthStateNotifier` lets us override the auth state for the
/// unauthenticated case without spinning up the real Firebase + secure-
/// storage machinery the production notifier needs.
class _FakeAuthStateNotifier extends AuthStateNotifier {
  _FakeAuthStateNotifier(this._initial);

  final AuthStateData _initial;

  @override
  AuthStateData build() => _initial;
}

void main() {
  group('WebExperienceScreen — unauthenticated visitor', () {
    testWidgets('renders the auth-pending placeholder', (tester) async {
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authStateProvider.overrideWith(
              () => _FakeAuthStateNotifier(AuthStateData.empty),
            ),
          ],
          child: localizedApp(
            const WebExperienceScreen(experienceId: 'exp-123'),
          ),
        ),
      );
      // The auth-pending placeholder is fully sync (no providers to
      // resolve) so a single pump completes the build.
      await tester.pump();

      // English template — the test fixture defaults to en.
      expect(find.text('RSVP to this event'), findsOneWidget);
      expect(find.text('Sign in to RSVP'), findsOneWidget);
      expect(
        find.textContaining('Sign in or create an account'),
        findsOneWidget,
      );

      // ExperienceContentView is NOT mounted on the unauth path.
      // (Probing for absence by name to keep this test independent of
      // ExperienceContentView's internals.)
      expect(
        find.byWidgetPredicate(
          (w) => w.runtimeType.toString() == 'ExperienceContentView',
        ),
        findsNothing,
      );
    });

    testWidgets('rsvp guest is routed to the phone-first screen, not the gate',
        (tester) async {
      // Phone-first onboarding (#2492): an unauthenticated guest who arrived
      // with an RSVP intention + code is sent straight to /verify-phone (the
      // "Confirm your phone to RSVP" screen) — the sign-in gate is skipped.
      final router = GoRouter(
        initialLocation: '/event/exp-123?rsvp=yes&code=TESTCODE',
        routes: [
          GoRoute(
            path: '/event/:id',
            builder: (context, state) => WebExperienceScreen(
              experienceId: state.pathParameters['id']!,
              rsvpIntention: state.uri.queryParameters['rsvp'],
              shortCode: state.uri.queryParameters['code'],
            ),
          ),
          GoRoute(
            path: '/verify-phone',
            builder: (context, state) => const Text('PHONE_FIRST_STUB'),
          ),
        ],
      );
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authStateProvider.overrideWith(
              () => _FakeAuthStateNotifier(AuthStateData.empty),
            ),
          ],
          child: MaterialApp.router(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            routerConfig: router,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('PHONE_FIRST_STUB'), findsOneWidget);
      expect(find.text('Sign in to RSVP'), findsNothing);
    });
  });
}
