import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_booking.pb.dart'
    show GearBooking, GearBookingState;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearPeopleResponse, GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase, TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_interest_panel.dart';
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart'
    as vm;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/services/providers.dart'
    show CommunitiesNotifier, CommunitiesState, communitiesProvider;

import '../../../../helpers/l10n_helpers.dart';

/// A GearNotifier stub returning a fixed [GearState].
class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;
  @override
  GearState build() => _state;
}

/// A booking notifier stub that never touches the repository.
class _FakeBookingNotifier extends vm.GearBookingNotifier {
  _FakeBookingNotifier(this._state) : super('gear-1');
  final vm.GearBookingState _state;
  @override
  vm.GearBookingState build() => _state;
  @override
  Future<void> initialize({required String communityId}) async {}
}

/// A communities notifier stub with a fixed portfolio (the share nudge is
/// gated on having a named community the gear is not yet shared with —
/// #2724).
class _FakeCommunitiesNotifier extends CommunitiesNotifier {
  _FakeCommunitiesNotifier(this._communities);
  final List<CommunityItem> _communities;
  @override
  CommunitiesState build() =>
      CommunitiesState(communities: _communities, isLoading: false);
}

Widget _panel(
  GearState state, {
  vm.GearBookingState? bookingState,
  List<CommunityItem> userCommunities = const [],
}) {
  return ProviderScope(
    overrides: [
      gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
      vm.gearBookingProvider('gear-1').overrideWith(
          () => _FakeBookingNotifier(bookingState ?? const vm.GearBookingState())),
      communitiesProvider
          .overrideWith(() => _FakeCommunitiesNotifier(userCommunities)),
    ],
    child: localizedApp(
      GearInterestPanel(
        gearId: 'gear-1',
        communityId: 'community-1',
      ),
    ),
  );
}

GearState _giveawayState({
  required String currentUserId,
  GetGearPeopleResponse? people,
  GearTransferContext? transferContext,
  List<SharedCommunity> sharedCommunities = const [],
}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: currentUserId,
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Tent',
      owner: User(id: 'owner-1', name: 'Thomas'),
      availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
    ),
    gearPeople: people,
    transferContext: transferContext,
    sharedCommunities: sharedCommunities,
  );
}

GearBooking _booking({required User borrower}) {
  return GearBooking(
    id: 'booking-1',
    gearId: 'gear-1',
    borrower: borrower,
    startDateUnixSec: Int64(0),
    endDateUnixSec: Int64(0),
    state: GearBookingState.GEAR_BOOKING_STATE_RESERVED,
    isMine: false,
  );
}

void main() {
  group('GearInterestPanel', () {
    testWidgets('non-owner with no interest sees the express-interest CTA',
        (tester) async {
      await tester.pumpWidget(_panel(_giveawayState(currentUserId: 'viewer-1')));
      await tester.pump();

      expect(find.text('Who wants it'), findsOneWidget);
      expect(find.text("I'm interested"), findsOneWidget);
    });

    testWidgets('owner reach view: stat strip, share nudge, inline Give-to',
        (tester) async {
      await tester.pumpWidget(_panel(
        _giveawayState(
          currentUserId: 'owner-1',
          people: GetGearPeopleResponse(
            interestedParties: [User(id: 'u1', name: 'Maya')],
          ),
          transferContext: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
            pendingRequests: [
              TransferRequest(
                  transferId: 't1', borrower: User(id: 'u1', name: 'Maya')),
            ],
          ),
        ),
        // A named circle the gear is NOT yet shared with — the nudge's
        // precondition (#2724).
        userCommunities: [CommunityItem(id: 'c-2', name: 'Cedar Court')],
      ));
      await tester.pump();

      expect(find.text('Maya'), findsOneWidget);
      // Reach chrome from the design.
      expect(find.text('Interested'.toUpperCase()), findsOneWidget);
      expect(find.textContaining('Reach a bit further'), findsOneWidget);
      expect(find.text('Raised a hand · 1'.toUpperCase()), findsOneWidget);
      // Inline give-to, not the old modal-opening "Choose a recipient" CTA.
      expect(find.text('Give to'), findsOneWidget);
      expect(find.text('Choose a recipient'), findsNothing);
    });

    testWidgets(
        'share nudge is hidden when every circle already has the item '
        '(#2724)', (tester) async {
      await tester.pumpWidget(_panel(
        _giveawayState(
          currentUserId: 'owner-1',
          transferContext: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
          ),
          sharedCommunities: [SharedCommunity(communityId: 'c-2')],
        ),
        // The owner's only named circle is already shared with — plus a
        // nameless per-item ad-hoc group, which never counts as a circle.
        userCommunities: [
          CommunityItem(id: 'c-2', name: 'Cedar Court'),
          CommunityItem(id: 'c-adhoc', name: ''),
        ],
      ));
      await tester.pump();

      expect(find.textContaining('Reach a bit further'), findsNothing);
    });

    testWidgets(
        'a completed giveaway renders the terminal summary, not an empty '
        'signup pane (#2724)', (tester) async {
      final recipient = User(id: 'u1', name: 'Maya Lopez');
      await tester.pumpWidget(_panel(_giveawayState(
        currentUserId: 'owner-1',
        transferContext: GearTransferContext(
          isOwner: true,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED,
          selectedRecipient:
              TransferRequest(transferId: 't1', borrower: recipient),
        ),
      )));
      await tester.pump();

      // The completed summary card: outcome header + recipient + badge.
      expect(find.text('Giveaway completed'.toUpperCase()), findsOneWidget);
      expect(find.text('Maya Lopez'), findsOneWidget);
      expect(find.text('RECEIVED IT'), findsOneWidget);
      // No in-flight affordances or selection chrome remain.
      expect(find.text("I'm interested"), findsNothing);
      expect(find.text('Give to'), findsNothing);
      expect(find.text('SELECTED'), findsNothing);
      expect(find.textContaining('Mark as given'), findsNothing);
    });

    testWidgets(
        'a completed giveaway without a recipient reads "Gone to a new home"',
        (tester) async {
      await tester.pumpWidget(_panel(_giveawayState(
        currentUserId: 'viewer-1',
        transferContext: GearTransferContext(
          isOwner: false,
          overallPhase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED,
        ),
      )));
      await tester.pump();

      expect(find.text('Giveaway completed'.toUpperCase()), findsOneWidget);
      expect(find.text('Gone to a new home'), findsOneWidget);
      expect(find.text("I'm interested"), findsNothing);
    });

    testWidgets('a selected recipient flips to the hand-off coordination view',
        (tester) async {
      final selected = User(id: 'u1', name: 'Maya Lopez');
      await tester.pumpWidget(_panel(
        _giveawayState(
          currentUserId: 'owner-1',
          people: GetGearPeopleResponse(interestedParties: [selected]),
          transferContext: GearTransferContext(
            isOwner: true,
            overallPhase: GiveawayPhase.GIVEAWAY_PHASE_RECIPIENT_SELECTED,
            selectedRecipient:
                TransferRequest(transferId: 't1', borrower: selected),
          ),
        ),
        bookingState: vm.GearBookingState(bookings: [
          // The selected giveaway transfer surfaced as a booking.
          _booking(borrower: selected),
        ]),
      ));
      await tester.pump();

      // Recipient confirmation card.
      expect(find.text('Going to'.toUpperCase()), findsOneWidget);
      expect(find.text('Maya Lopez'), findsOneWidget);
      // The hand-off section's PLACE / TIME segments render (pickup only).
      expect(find.text('PLACE'), findsOneWidget);
      expect(find.text('TIME'), findsOneWidget);
      expect(find.textContaining('DROP-OFF'), findsNothing);
      // Owner can close the loop.
      expect(find.text('Mark as given to Maya'), findsOneWidget);
    });
  });
}
