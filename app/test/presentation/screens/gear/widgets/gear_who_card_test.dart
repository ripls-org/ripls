import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart'
    show ActiveLoan, Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase, TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_who_card.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;
  bool expressInterestCalled = false;
  @override
  GearState build() => _state;
  @override
  Future<void> expressInterest() async {
    expressInterestCalled = true;
  }
}

Widget _card(
  GearState state, {
  VoidCallback? onShowCalendar,
  ValueChanged<Rect>? onExpandInterest,
  VoidCallback? onMarkReturned,
}) {
  return ProviderScope(
    overrides: [
      gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
    ],
    child: localizedApp(
      GearWhoCard(
        gearId: 'gear-1',
        accentColor: const Color(0xFF8FAE7E),
        onShowCalendar: onShowCalendar ?? () {},
        onExpandInterest: onExpandInterest ?? (_) {},
        onMarkReturned: onMarkReturned ?? () {},
      ),
    ),
  );
}

GearState _state({
  required Availability availability,
  required String currentUserId,
  ActiveLoan? activeLoan,
  GearTransferContext? transferContext,
}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: currentUserId,
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Shovel',
      owner: User(id: 'owner-1', name: 'Thomas Escobar'),
      availability: availability,
      activeLoan: activeLoan,
    ),
    transferContext: transferContext,
  );
}

void main() {
  group('GearWhoCard', () {
    testWidgets('loan non-owner shows the Book-for-borrowing CTA',
        (tester) async {
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_LOAN,
        currentUserId: 'viewer-1',
      )));
      await tester.pump();

      expect(find.text("Who's using it".toUpperCase()), findsOneWidget);
      expect(find.text('Book for borrowing'), findsOneWidget);
      expect(find.text("It's home and free — grab it whenever."),
          findsOneWidget);
    });

    testWidgets('giveaway non-owner with no interest shows the raise-hand row '
        'and CTA', (tester) async {
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
        ),
      )));
      await tester.pump();

      expect(find.text('Who wants it'.toUpperCase()), findsOneWidget);
      expect(find.text("You haven't raised your hand yet"), findsOneWidget);
      expect(find.text("I'm interested"), findsOneWidget);
    });

    testWidgets('giveaway lists an interested member with their signup time',
        (tester) async {
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
          pendingRequests: [
            TransferRequest(
              transferId: 't1',
              borrower: User(id: 'u1', name: 'Maya Chen'),
              requestedAtUnixSec: Int64(1750000000),
            ),
          ],
        ),
      )));
      await tester.pump();

      expect(find.text('Maya Chen'), findsOneWidget);
      // The header meta reflects the count.
      expect(find.text('1 interested'), findsOneWidget);
    });

    testWidgets(
        'giveaway header count de-dups the selected recipient the server '
        'keeps inside pendingRequests (#2724)', (tester) async {
      final maya = User(id: 'u1', name: 'Maya Chen');
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
        currentUserId: 'owner-1',
        transferContext: GearTransferContext(
          isOwner: true,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
          selectedRecipient: TransferRequest(transferId: 't1', borrower: maya),
          pendingRequests: [
            // The server keeps the RECIPIENT_SELECTED transfer in
            // pendingRequests alongside everyone else's interest.
            TransferRequest(
              transferId: 't1',
              borrower: maya,
              requestedAtUnixSec: Int64(1750000000),
            ),
            TransferRequest(
              transferId: 't2',
              borrower: User(id: 'u2', name: 'Luis Ortiz'),
              requestedAtUnixSec: Int64(1750000100),
            ),
          ],
        ),
      )));
      await tester.pump();

      // Two distinct people → "2 interested", matching the two roster rows
      // (Maya once as SELECTED, Luis once) — not "3 interested".
      expect(find.text('2 interested'), findsOneWidget);
      expect(find.text('3 interested'), findsNothing);
      expect(find.text('Maya Chen'), findsOneWidget);
      expect(find.text('Luis Ortiz'), findsOneWidget);
    });

    testWidgets(
        'loan queue rows carry booking-window pills so two bookings by the '
        'same borrower are distinguishable (#2638)', (tester) async {
      int unix(DateTime d) => d.millisecondsSinceEpoch ~/ 1000;
      final borrower = User(id: 'u1', name: 'Priya Raman');
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_LOAN,
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          pendingRequests: [
            TransferRequest(
              transferId: 't1',
              borrower: borrower,
              estimatedPickupUnixSec: Int64(unix(DateTime(2026, 7, 11))),
              expectedReturnUnixSec: Int64(unix(DateTime(2026, 7, 11))),
            ),
            TransferRequest(
              transferId: 't2',
              borrower: borrower,
              estimatedPickupUnixSec: Int64(unix(DateTime(2026, 7, 18))),
              expectedReturnUnixSec: Int64(unix(DateTime(2026, 7, 20))),
            ),
          ],
        ),
      )));
      await tester.pump();

      expect(find.text('Priya Raman'), findsNWidgets(2));
      expect(find.text('JUL 11'), findsOneWidget);
      expect(find.text('JUL 18–20'), findsOneWidget);
    });

    testWidgets(
        'viewer holding the item sees a Mark-returned CTA that fires the '
        'callback (#2638 follow-up)', (tester) async {
      var returned = false;
      await tester.pumpWidget(_card(
        _state(
          availability: Availability.AVAILABILITY_FOR_LOAN,
          currentUserId: 'viewer-1',
          activeLoan: ActiveLoan(
            transferId: 't-active',
            borrower: User(id: 'viewer-1', name: 'Alice Holder'),
          ),
        ),
        onMarkReturned: () => returned = true,
      ));
      await tester.pump();

      expect(find.text('Mark returned'), findsOneWidget);
      expect(find.text('Book for borrowing'), findsNothing);
      await tester.tap(find.text('Mark returned'));
      await tester.pump();
      expect(returned, isTrue);
    });

    testWidgets(
        'someone ELSE holding the item keeps the Book-for-borrowing CTA',
        (tester) async {
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_LOAN,
        currentUserId: 'viewer-1',
        activeLoan: ActiveLoan(
          transferId: 't-active',
          borrower: User(id: 'other-1', name: 'Someone Else'),
        ),
      )));
      await tester.pump();

      expect(find.text('Book for borrowing'), findsOneWidget);
      expect(find.text('Mark returned'), findsNothing);
    });

    testWidgets('undated pending request renders without a date pill',
        (tester) async {
      await tester.pumpWidget(_card(_state(
        availability: Availability.AVAILABILITY_FOR_LOAN,
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          pendingRequests: [
            TransferRequest(
              transferId: 't1',
              borrower: User(id: 'u1', name: 'Maya Chen'),
              requestedAtUnixSec: Int64(1750000000),
            ),
          ],
        ),
      )));
      await tester.pump();

      expect(find.text('Maya Chen'), findsOneWidget);
      expect(find.textContaining('JUL'), findsNothing);
    });

    testWidgets('tapping the loan card body (not the CTA) opens the calendar',
        (tester) async {
      var opened = false;
      await tester.pumpWidget(_card(
        _state(
          availability: Availability.AVAILABILITY_FOR_LOAN,
          currentUserId: 'viewer-1',
        ),
        onShowCalendar: () => opened = true,
      ));
      await tester.pump();

      // Tap the header, which is part of the card body — not the CTA button.
      await tester.tap(find.text("Who's using it".toUpperCase()));
      await tester.pump();
      expect(opened, isTrue);
    });

    testWidgets('tapping the giveaway card body opens the interest panel',
        (tester) async {
      var expanded = false;
      await tester.pumpWidget(_card(
        _state(
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          currentUserId: 'viewer-1',
          transferContext: GearTransferContext(
            isOwner: false,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
          ),
        ),
        onExpandInterest: (_) => expanded = true,
      ));
      await tester.pump();

      await tester.tap(find.text('Who wants it'.toUpperCase()));
      await tester.pump();
      expect(expanded, isTrue);
    });

    testWidgets("tapping the I'm-interested CTA expresses interest, not the "
        'card-body tap', (tester) async {
      var expanded = false;
      final fake = _FakeGearNotifier(_state(
        availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
        ),
      ));
      await tester.pumpWidget(ProviderScope(
        overrides: [gearProvider('gear-1').overrideWith(() => fake)],
        child: localizedApp(GearWhoCard(
          gearId: 'gear-1',
          accentColor: const Color(0xFF8FAE7E),
          onShowCalendar: () {},
          onExpandInterest: (_) => expanded = true,
          onMarkReturned: () {},
        )),
      ));
      await tester.pump();

      await tester.tap(find.text("I'm interested"));
      await tester.pump();

      // The CTA ran its own action; the card-body tap (open panel) did NOT fire.
      expect(fake.expressInterestCalled, isTrue);
      expect(expanded, isFalse);
    });

    testWidgets('completed giveaway shows "Giveaway completed" + recipient, no '
        'expand', (tester) async {
      var expanded = false;
      final recipient = User(id: 'u1', name: 'Maya Chen');
      await tester.pumpWidget(_card(
        _state(
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          currentUserId: 'owner-1',
          transferContext: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED,
            selectedRecipient:
                TransferRequest(transferId: 't1', borrower: recipient),
          ),
        ),
        onExpandInterest: (_) => expanded = true,
      ));
      await tester.pump();

      expect(find.text('Giveaway completed'.toUpperCase()), findsOneWidget);
      expect(find.text('Maya Chen'), findsOneWidget);
      // Tapping it does nothing — no expanded version.
      await tester.tap(find.text('Maya Chen'));
      await tester.pump();
      expect(expanded, isFalse);
    });
  });
}
