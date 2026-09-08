import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/auth/phone_auth_header.dart';

import '../../../helpers/l10n_helpers.dart';

/// The phone-entry heading is the one place a guest is told what confirming
/// their phone is *for*. Every landing type has its own verb, and getting the
/// wrong one is a silent copy bug — the screen still works, it just tells the
/// guest they're claiming an item when they're joining a group.
void main() {
  group('PhoneAuthHeader — guest action flows', () {
    Future<void> pumpHeader(
      WidgetTester tester, {
      required GuestFlowKind kind,
      String? name,
    }) async {
      await tester.pumpWidget(
        localizedApp(
          PhoneAuthHeader(
            isAttach: false,
            isPhoneInputStep: true,
            isNameInputStep: false,
            isRsvpFlow: false,
            isGuestActionFlow: true,
            eventName: name,
            guestFlowKind: kind,
          ),
        ),
      );
      await tester.pump();
    }

    testWidgets('community flow names the group', (tester) async {
      await pumpHeader(
        tester,
        kind: GuestFlowKind.community,
        name: 'Ferndale Tool Library',
      );
      expect(
        find.text('Confirm your phone to join Ferndale Tool Library'),
        findsOneWidget,
      );
    });

    testWidgets('community flow keeps the group name capitalized', (
      tester,
    ) async {
      // Gear and request titles are common nouns lowercased mid-sentence; a
      // group name is a proper noun and must not be ("…to join ferndale…").
      await pumpHeader(
        tester,
        kind: GuestFlowKind.community,
        name: 'Ferndale Tool Library',
      );
      expect(find.textContaining('join ferndale'), findsNothing);
    });

    testWidgets('community flow falls back when the name is unavailable', (
      tester,
    ) async {
      await pumpHeader(tester, kind: GuestFlowKind.community);
      expect(find.text('Confirm your phone to join'), findsOneWidget);
    });

    testWidgets('gear flow lowercases the item name mid-sentence', (
      tester,
    ) async {
      await pumpHeader(
        tester,
        kind: GuestFlowKind.gear,
        name: 'Cordless Drill',
      );
      expect(
        find.text('Confirm your phone to claim cordless Drill'),
        findsOneWidget,
      );
    });

    testWidgets('request flow uses the help verb', (tester) async {
      await pumpHeader(
        tester,
        kind: GuestFlowKind.request,
        name: 'An extension ladder',
      );
      expect(
        find.text('Confirm your phone to help with an extension ladder'),
        findsOneWidget,
      );
    });

    testWidgets('rsvp flow outranks the guest-flow kind', (tester) async {
      // An event RSVP has its own branch ahead of the kind switch; the default
      // kind must not leak "claim" copy into an RSVP.
      await tester.pumpWidget(
        localizedApp(
          const PhoneAuthHeader(
            isAttach: false,
            isPhoneInputStep: true,
            isNameInputStep: false,
            isRsvpFlow: true,
            isGuestActionFlow: true,
            eventName: 'Block Party',
          ),
        ),
      );
      await tester.pump();

      expect(find.textContaining('RSVP'), findsOneWidget);
      expect(find.textContaining('claim'), findsNothing);
    });
  });
}
