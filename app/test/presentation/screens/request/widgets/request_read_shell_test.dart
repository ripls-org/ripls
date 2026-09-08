import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show CarbonEstimate, Estimate;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show GetRequestStatsResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferState, TransferType;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/request/widgets/request_read_shell.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/sharing/shared_with_card.dart';

import '../../../../helpers/l10n_helpers.dart';

const _id = 'req-1';

/// Fake that returns a fixed [RequestState] from build(), bypassing the real
/// notifier's network load so the shell can be rendered in isolation.
class _FakeRequestNotifier extends RequestNotifier {
  _FakeRequestNotifier(super.requestId, this._state);
  final RequestState _state;
  @override
  RequestState build() => _state;
}

class _FakeNeedsNotifier extends RequestNeedsNotifier {
  _FakeNeedsNotifier(super.requestId, this._state);
  final RequestNeedsState _state;
  @override
  RequestNeedsState build() => _state;
}

RequestState _state({
  List<User> offerers = const [],
  pb.RequestState requestState = pb.RequestState.REQUEST_STATE_OFFERS_RECEIVED,
  List<pb.RequestGearOffer> gearOffers = const [],
  int fulfilledAtUnixSec = 0,
  GetRequestStatsResponse? requestStats,
}) {
  return RequestState(
    requestId: _id,
    currentUserId: 'viewer',
    requestDetails: pb.Request(
      id: _id,
      title: 'A lawn mower for the weekend',
      description: 'Ours died halfway through the front yard.',
      requester: User(id: 'june', name: 'June Park'),
      state: requestState,
      conversationId: 'conv-1',
      messageCount: 1,
      offerers: offerers,
      gearOffers: gearOffers,
      fulfilledAtUnixSec:
          fulfilledAtUnixSec > 0 ? Int64(fulfilledAtUnixSec) : null,
    ),
    requestStats: requestStats,
  );
}

/// Stats carrying a populated impact estimate, the shape behind the
/// post-fulfillment inline impact row.
GetRequestStatsResponse _statsWithImpact() {
  return GetRequestStatsResponse(
    timesFulfilled: 1,
    impact: ImpactEstimate(
      moneySaved: MoneySavings(valueUsd: Estimate(mean: 210)),
      qualityTime: QualityTimeEstimate(
        qualityTimeMinutes: Estimate(mean: 237),
      ),
      emissionsPrevented: PreventedEmissions(
        manufactureAvoidedCarbon: CarbonEstimate(
          co2eGrams: Estimate(mean: 40000),
        ),
      ),
    ),
  );
}

/// A single claimed need ("Lawn mower", 1 of 1 slots taken) with Theo's claim
/// contribution attached — the shape behind the lawn-mower walkthrough story.
RequestNeedsState _claimedNeedsState() {
  return RequestNeedsState(
    needs: [
      pb.RequestNeedResponse(
        id: 'n1',
        requestId: _id,
        name: 'Lawn mower',
        slots: 1,
        slotsRemaining: 0,
      ),
    ],
    contributions: [
      pb.RequestContributionResponse(
        id: 'c1',
        requestId: _id,
        title: 'Lawn mower',
        fromNeedId: 'n1',
        contributor: User(id: 'theo', name: 'Theo Alvarez'),
      ),
    ],
  );
}

pb.RequestGearOffer _gearOffer({
  required TransferType type,
  required TransferState state,
  bool accepted = false,
}) {
  return pb.RequestGearOffer(
    transferId: 't1',
    transferType: type,
    state: state,
    gearId: 'g1',
    gearName: 'Lawn mower',
    helper: User(id: 'theo', name: 'Theo Alvarez'),
    contributionId: 'c1',
    needId: 'n1',
    acceptedAtUnixSec: accepted ? Int64(1000) : null,
  );
}

Future<void> _pump(
  WidgetTester tester, {
  required RequestState state,
  RequestNeedsState needsState = const RequestNeedsState(),
  VoidCallback? onViewImpact,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        requestProvider(
          _id,
        ).overrideWith(() => _FakeRequestNotifier(_id, state)),
        requestNeedsProvider(
          _id,
        ).overrideWith(() => _FakeNeedsNotifier(_id, needsState)),
      ],
      child: localizedApp(
        RequestReadShell(
          requestId: _id,
          accentColor: const Color(0xFF7A9B8C),
          onExpandConversation: (_) {},
          onShowLocation: (_) {},
          onShowHelpers: (_) {},
          onShowAccess: () {},
          onManage: () {},
          onViewImpact: onViewImpact,
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  group('RequestReadShell offerers (#2701)', () {
    testWidgets('lists people offering to help on the collapsed card', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(offerers: [User(id: 'theo', name: 'Theo Alvarez')]),
      );

      expect(find.text('OFFERING TO HELP'), findsOneWidget);
      expect(find.text('Theo Alvarez'), findsOneWidget);
      // Someone else's offer carries no "You" marker.
      expect(find.text('You'), findsNothing);
    });

    testWidgets('marks the viewer\'s own offer with "You"', (tester) async {
      await _pump(
        tester,
        state: _state(offerers: [User(id: 'viewer', name: 'Theo Alvarez')]),
      );

      expect(find.text('OFFERING TO HELP'), findsOneWidget);
      expect(find.text('Theo Alvarez'), findsOneWidget);
      expect(find.text('You'), findsOneWidget);
    });

    testWidgets('hides the section when nobody has offered', (tester) async {
      await _pump(tester, state: _state());

      expect(find.text('OFFERING TO HELP'), findsNothing);
    });

    testWidgets('skips offerers already shown as contributors', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(offerers: [User(id: 'theo', name: 'Theo Alvarez')]),
        needsState: RequestNeedsState(
          contributions: [
            pb.RequestContributionResponse(
              id: 'c1',
              title: 'Wheelbarrow',
              contributor: User(id: 'theo', name: 'Theo Alvarez'),
            ),
          ],
        ),
      );

      // Theo appears once, as the BRINGING contributor — not again as a
      // general offerer.
      expect(find.text('BRINGING'), findsOneWidget);
      expect(find.text('OFFERING TO HELP'), findsNothing);
      expect(find.text('Wheelbarrow'), findsOneWidget);
    });
  });

  group('RequestReadShell collapsed-row count (#2724)', () {
    /// Five needs — one more than the collapsed card's four-row cap.
    RequestNeedsState fiveNeeds({
      List<pb.RequestContributionResponse> contributions = const [],
    }) {
      return RequestNeedsState(
        needs: [
          for (var i = 0; i < 5; i++)
            pb.RequestNeedResponse(
              id: 'n$i',
              requestId: _id,
              name: 'Need $i',
              slots: 1,
              slotsRemaining: i == 4 ? 1 : 0,
            ),
        ],
        contributions: contributions,
      );
    }

    testWidgets('five needs fold to "1 more ›" with no other groups', (
      tester,
    ) async {
      await _pump(tester, state: _state(), needsState: fiveNeeds());

      expect(find.text('Need 0'), findsOneWidget);
      expect(find.text('Need 3'), findsOneWidget);
      expect(find.text('Need 4'), findsNothing);
      expect(find.text('1 more ›'), findsOneWidget);
    });

    testWidgets(
      'bringing + offering entries never inflate the "more" count — every '
      'viewer of the same 5 needs reads "1 more ›"',
      (tester) async {
        await _pump(
          tester,
          state: _state(offerers: [User(id: 'rob', name: 'Rob Chen')]),
          needsState: fiveNeeds(
            contributions: [
              pb.RequestContributionResponse(
                id: 'c-free',
                requestId: _id,
                title: 'Label maker',
                contributor: User(id: 'bea', name: 'Bea Ortiz'),
              ),
            ],
          ),
        );

        // Still exactly the hidden-needs count, not needs+bringing+offers.
        expect(find.text('1 more ›'), findsOneWidget);
        expect(find.text('2 more ›'), findsNothing);
        expect(find.text('3 more ›'), findsNothing);
        // The folded groups stay discoverable via headers with total counts.
        expect(find.text('BRINGING'), findsOneWidget);
        expect(find.text('OFFERING TO HELP'), findsOneWidget);
      },
    );

    test('collapseNeedsCardRows: more == total needs − visible need rows', () {
      final bare = collapseNeedsCardRows(needs: 5, bringing: 0, offerers: 0);
      expect(bare.needRows, 4);
      expect(bare.moreNeeds, 1);

      // Bringing/offering compete for leftover row budget but never leak
      // into the needs overflow count.
      final busy = collapseNeedsCardRows(needs: 5, bringing: 2, offerers: 1);
      expect(busy.needRows, 4);
      expect(busy.bringRows, 0);
      expect(busy.offerRows, 0);
      expect(busy.moreNeeds, 1);

      final short = collapseNeedsCardRows(needs: 2, bringing: 2, offerers: 1);
      expect(short.needRows, 2);
      expect(short.bringRows, 2);
      expect(short.offerRows, 0);
      expect(short.moreNeeds, 0);
    });
  });

  group('RequestReadShell fulfilled state (#2724)', () {
    final twoMinutesAgo =
        DateTime.now().millisecondsSinceEpoch ~/ 1000 - 2 * 60;

    testWidgets('renders the FULFILLED edge-status chip with time ago', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(
          requestState: pb.RequestState.REQUEST_STATE_FULFILLED,
          fulfilledAtUnixSec: twoMinutesAgo,
        ),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('FULFILLED · 2m ago'), findsOneWidget);
    });

    testWidgets('renders the FULFILLED chip without time when unset', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(requestState: pb.RequestState.REQUEST_STATE_FULFILLED),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('FULFILLED'), findsOneWidget);
    });

    testWidgets('a fulfilled request stops offering to widen its audience', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(requestState: pb.RequestState.REQUEST_STATE_FULFILLED),
        needsState: _claimedNeedsState(),
      );

      // Nobody can act on an invitation to a request that is already answered.
      expect(find.byType(SharedWithCard), findsNothing);
    });

    testWidgets('an open request still offers to widen its audience', (
      tester,
    ) async {
      await _pump(tester, state: _state());

      expect(find.byType(SharedWithCard), findsOneWidget);
    });

    testWidgets('keeps the CANCELLED chip for cancelled requests', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(requestState: pb.RequestState.REQUEST_STATE_CANCELLED),
      );

      expect(find.text('CANCELLED'), findsOneWidget);
      expect(find.textContaining('FULFILLED'), findsNothing);
    });

    testWidgets('header reads "All N covered" when every need is claimed', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(requestState: pb.RequestState.REQUEST_STATE_FULFILLED),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('All 1 covered'), findsOneWidget);
      expect(find.text('1 of 1 claimed'), findsNothing);
    });

    testWidgets('delivered loan offer flips the need row to HANDED OFF', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(
          requestState: pb.RequestState.REQUEST_STATE_FULFILLED,
          gearOffers: [
            _gearOffer(
              type: TransferType.TRANSFER_TYPE_LOAN,
              state: TransferState.TRANSFER_STATE_ACTIVE,
              accepted: true,
            ),
          ],
        ),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('HANDED OFF ✓'), findsOneWidget);
      expect(find.text('ACCEPTED ✓'), findsNothing);
      expect(find.text('Theo'), findsOneWidget);
    });

    testWidgets('delivered giveaway offer flips the need row to GIVEN', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(
          requestState: pb.RequestState.REQUEST_STATE_FULFILLED,
          gearOffers: [
            _gearOffer(
              type: TransferType.TRANSFER_TYPE_GIVEAWAY,
              state: TransferState.TRANSFER_STATE_COMPLETED,
              accepted: true,
            ),
          ],
        ),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('GIVEN ✓'), findsOneWidget);
      expect(find.text('ACCEPTED ✓'), findsNothing);
    });

    testWidgets('claim without a gear transfer shows the DONE tag', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(requestState: pb.RequestState.REQUEST_STATE_FULFILLED),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('DONE ✓'), findsOneWidget);
      expect(find.text('Theo'), findsOneWidget);
    });

    testWidgets('labels the inline impact chips after fulfillment', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(
          requestState: pb.RequestState.REQUEST_STATE_FULFILLED,
          requestStats: _statsWithImpact(),
        ),
        needsState: _claimedNeedsState(),
        onViewImpact: () {},
      );

      // Bare "$210 · 237 mins · 40kg" chips were uninterpretable and raw
      // minutes never render (#2724 + A3).
      expect(find.text('\$210 saved'), findsOneWidget);
      expect(find.text('~4 h together'), findsOneWidget);
      expect(find.text('40kg CO₂'), findsOneWidget);
    });

    testWidgets('impact row opens the receipt inline (#2724)', (
      tester,
    ) async {
      var opened = false;
      await _pump(
        tester,
        state: _state(
          requestState: pb.RequestState.REQUEST_STATE_FULFILLED,
          requestStats: _statsWithImpact(),
        ),
        needsState: _claimedNeedsState(),
        onViewImpact: () => opened = true,
      );

      expect(find.byIcon(Icons.chevron_right), findsOneWidget);
      await tester.tap(find.byIcon(Icons.insights));
      expect(opened, isTrue);
    });

    testWidgets('no impact row before fulfillment', (tester) async {
      await _pump(
        tester,
        state: _state(requestStats: _statsWithImpact()),
        needsState: _claimedNeedsState(),
        onViewImpact: () {},
      );

      expect(find.byIcon(Icons.insights), findsNothing);
      expect(find.textContaining('saved'), findsNothing);
    });

    testWidgets('non-fulfilled request keeps the pre-fulfillment reading', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _state(
          gearOffers: [
            _gearOffer(
              type: TransferType.TRANSFER_TYPE_LOAN,
              state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
              accepted: true,
            ),
          ],
        ),
        needsState: _claimedNeedsState(),
      );

      expect(find.text('1 of 1 claimed'), findsOneWidget);
      expect(find.text('ACCEPTED ✓'), findsOneWidget);
      expect(find.textContaining('FULFILLED'), findsNothing);
      expect(find.textContaining('HANDED OFF'), findsNothing);
      expect(find.textContaining('DONE'), findsNothing);
      expect(find.textContaining('All 1 covered'), findsNothing);
    });
  });
}
