import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/auth/phone_consent_disclosure.dart';

import '../../../helpers/l10n_helpers.dart';

/// The A2P 10DLC body every variant must carry verbatim — it is mirrored on
/// the hosted opt-in proof at example.com/sms-opt-in.html.
const _complianceBody =
    'Message frequency varies. Msg & data rates may apply. '
    'Reply STOP to cancel, HELP for help.';

/// Stand-ins for the TERMS_URL / PRIVACY_URL dart-defines. A `flutter test`
/// run supplies no defines, so the widget takes them as parameters rather than
/// reading `Environment` — otherwise the links would always be absent here and
/// the carrier-requirement test below would be vacuous (#2953).
const _testTermsUrl = 'https://example.org/sms-terms.html';
const _testPrivacyUrl = 'https://example.org/privacy-policy.html';

Future<void> _pump(
  WidgetTester tester,
  PhoneConsentContext entryContext, {
  String? termsUrl = _testTermsUrl,
  String? privacyUrl = _testPrivacyUrl,
}) async {
  await tester.pumpWidget(
    localizedApp(
      Scaffold(
        body: PhoneConsentDisclosure(
          entryContext: entryContext,
          termsUrl: termsUrl,
          privacyUrl: privacyUrl,
        ),
      ),
    ),
  );
}

void main() {
  group('PhoneConsentDisclosure', () {
    testWidgets('leads with event invites on an event invite', (tester) async {
      await _pump(tester, PhoneConsentContext.event);
      expect(
        find.textContaining('receive event invites and community updates'),
        findsOneWidget,
      );
    });

    testWidgets('leads with help requests on a help-request landing (#2724)', (
      tester,
    ) async {
      await _pump(tester, PhoneConsentContext.request);

      expect(
        find.textContaining('receive help requests and community updates'),
        findsOneWidget,
      );
      // The whole point: a guest offering to help with a request is not
      // being told they're signing up for event invites.
      expect(find.textContaining('event invites'), findsNothing);
    });

    testWidgets('leads with item updates on an item landing', (tester) async {
      await _pump(tester, PhoneConsentContext.item);
      expect(
        find.textContaining('receive item updates and community updates'),
        findsOneWidget,
      );
    });

    testWidgets('leads with community updates alone by default', (
      tester,
    ) async {
      await _pump(tester, PhoneConsentContext.community);
      expect(
        find.textContaining('receive community updates from Ripls'),
        findsOneWidget,
      );
    });

    testWidgets('every entry point keeps the A2P compliance body intact', (
      tester,
    ) async {
      for (final entryContext in PhoneConsentContext.values) {
        await _pump(tester, entryContext);
        expect(
          find.textContaining(_complianceBody),
          findsOneWidget,
          reason: 'compliance body missing for $entryContext',
        );
        // Submitting the number is the opt-in, so the trigger must be named.
        expect(
          find.textContaining('By clicking Send Code'),
          findsOneWidget,
          reason: 'opt-in trigger missing for $entryContext',
        );
      }
    });

    testWidgets('shows the carrier-required Terms and Privacy links', (
      tester,
    ) async {
      await _pump(tester, PhoneConsentContext.event);
      expect(find.text('Terms of Service'), findsOneWidget);
      expect(find.text('Privacy Policy'), findsOneWidget);
    });

    testWidgets('omits a link the deployment has no URL for', (tester) async {
      await _pump(tester, PhoneConsentContext.event, termsUrl: '');

      // A dead link is worse than none, but the consent body itself is not
      // conditional — it still has to be on the screen.
      expect(find.text('Terms of Service'), findsNothing);
      expect(find.text('Privacy Policy'), findsOneWidget);
      expect(find.textContaining(_complianceBody), findsOneWidget);
      // The separator only earns its place between two links.
      expect(find.text('  •  '), findsNothing);
    });

    testWidgets('drops the whole link row when neither URL is set', (
      tester,
    ) async {
      await _pump(
        tester,
        PhoneConsentContext.event,
        termsUrl: '',
        privacyUrl: '',
      );

      expect(find.text('Terms of Service'), findsNothing);
      expect(find.text('Privacy Policy'), findsNothing);
      expect(find.textContaining(_complianceBody), findsOneWidget);
    });
  });
}
