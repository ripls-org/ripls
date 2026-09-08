import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase, TransferRequest, Transfer;
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_interest_roster.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;
  @override
  GearState build() => _state;
}

Widget _roster(
  GearState state, {
  int? maxOthers,
  void Function(String, User)? onSelectRecipient,
}) {
  return ProviderScope(
    overrides: [
      gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
    ],
    child: localizedApp(
      GearInterestRoster(
        gearId: 'gear-1',
        maxOthers: maxOthers,
        onSelectRecipient: onSelectRecipient,
      ),
    ),
  );
}

TransferRequest _req(String id, String name, {int at = 1750000000}) =>
    TransferRequest(
      transferId: 't-$id',
      borrower: User(id: id, name: name),
      requestedAtUnixSec: Int64(at),
    );

GearState _state({
  required String currentUserId,
  required GearTransferContext ctx,
}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: currentUserId,
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Tent',
      owner: User(id: 'owner-1', name: 'Owner'),
      availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
    ),
    transferContext: ctx,
  );
}

void main() {
  group('GearInterestRoster', () {
    testWidgets('caps others at maxOthers and shows a "+N more" row',
        (tester) async {
      // Viewer expressed interest, plus four others → cap of 2 shows two names
      // and "+2 more".
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'me',
          ctx: GearTransferContext(
            isOwner: false,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
            userTransfer: Transfer(
              id: 't-me',
              state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            ),
            pendingRequests: [
              _req('me', 'Me'),
              _req('a', 'Ann'),
              _req('b', 'Bo'),
              _req('c', 'Cy'),
              _req('d', 'Di'),
            ],
          ),
        ),
        maxOthers: 2,
      ));
      await tester.pump();

      // The viewer's own row.
      expect(find.text('You raised your hand'), findsOneWidget);
      // Exactly two of the four others render, plus the "+2 more" row.
      expect(find.text('Ann'), findsOneWidget);
      expect(find.text('Bo'), findsOneWidget);
      expect(find.text('Cy'), findsNothing);
      expect(find.text('+2 more interested'), findsOneWidget);
    });

    testWidgets('uncapped shows everyone and no "+N more" row', (tester) async {
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'me',
          ctx: GearTransferContext(
            isOwner: false,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
            userTransfer: Transfer(
              id: 't-me',
              state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            ),
            pendingRequests: [
              _req('me', 'Me'),
              _req('a', 'Ann'),
              _req('b', 'Bo'),
              _req('c', 'Cy'),
              _req('d', 'Di'),
            ],
          ),
        ),
      ));
      await tester.pump();

      expect(find.text('Ann'), findsOneWidget);
      expect(find.text('Cy'), findsOneWidget);
      expect(find.text('Di'), findsOneWidget);
      expect(find.textContaining('more interested'), findsNothing);
    });

    testWidgets(
        'completed giveaway flips the recipient badge to RECEIVED IT (#2724)',
        (tester) async {
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'owner-1',
          ctx: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED,
            selectedRecipient: _req('a', 'Ann'),
          ),
        ),
      ));
      await tester.pump();

      expect(find.text('Ann'), findsOneWidget);
      expect(find.text('RECEIVED IT'), findsOneWidget);
      expect(find.text('SELECTED'), findsNothing);
    });

    testWidgets(
        "the recipient's own completed row reads RECEIVED IT, not SELECTED",
        (tester) async {
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'a',
          ctx: GearTransferContext(
            isOwner: false,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED,
            selectedRecipient: _req('a', 'Ann'),
          ),
        ),
      ));
      await tester.pump();

      expect(find.text("You're getting it"), findsOneWidget);
      expect(find.text('RECEIVED IT'), findsOneWidget);
      expect(find.text('SELECTED'), findsNothing);
    });

    testWidgets('an in-flight selection still reads SELECTED', (tester) async {
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'owner-1',
          ctx: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_RECIPIENT_SELECTED,
            selectedRecipient: _req('a', 'Ann'),
          ),
        ),
      ));
      await tester.pump();

      expect(find.text('SELECTED'), findsOneWidget);
      expect(find.text('RECEIVED IT'), findsNothing);
    });

    testWidgets('owner Select button reports the tapped (transferId, recipient)',
        (tester) async {
      String? gotTransfer;
      String? gotRecipient;
      await tester.pumpWidget(_roster(
        _state(
          currentUserId: 'owner-1',
          ctx: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
            pendingRequests: [_req('a', 'Ann')],
          ),
        ),
        onSelectRecipient: (t, r) {
          gotTransfer = t;
          gotRecipient = r.id;
        },
      ));
      await tester.pump();

      expect(find.text('Ann'), findsOneWidget);
      await tester.tap(find.text('Give to'));
      await tester.pump();
      expect(gotTransfer, 't-a');
      expect(gotRecipient, 'a');
    });
  });
}
