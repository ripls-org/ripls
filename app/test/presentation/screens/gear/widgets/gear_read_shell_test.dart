import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show TrackedString;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata, GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase, TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/screens/gear/widgets/gear_read_shell.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_who_card.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/sharing/shared_with_card.dart';

import '../../../../helpers/l10n_helpers.dart';

/// A GearNotifier stub returning a fixed [GearState].
class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;
  @override
  GearState build() => _state;
}

GearState _giveawayState({required GiveawayPhase phase}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: 'viewer-1',
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Stand mixer',
      description: 'Works great — moving away, someone take it.',
      owner: User(id: 'owner-1', name: 'Thomas'),
      availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
      conversationId: 'conv-1',
    ),
    transferContext: GearTransferContext(
      isOwner: false,
      overallPhase: phase,
      selectedRecipient: phase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED
          ? TransferRequest(
              transferId: 't1',
              borrower: User(id: 'u1', name: 'Dana Reed'),
            )
          : null,
    ),
  );
}

/// A lendable item whose detection produced [metadata] — the details card
/// summarizes it.
GearState _metadataState(GearMetadata metadata) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: 'owner-1',
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Lawn mower',
      description: 'Starts on the first pull.',
      owner: User(id: 'owner-1', name: 'Thomas'),
      availability: Availability.AVAILABILITY_FOR_LOAN,
      conversationId: 'conv-1',
      metadata: metadata,
    ),
  );
}

Future<void> _pump(WidgetTester tester, GearState state) async {
  // Tall viewport so the bottom-anchored sheet lays out fully.
  tester.view.physicalSize = const Size(1200, 3000);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
      ],
      child: localizedApp(
        GearReadShell(
          gearId: 'gear-1',
          accentColor: const Color(0xFFEF7A6D),
          onExpandConversation: (_) {},
          onShowLocation: (_) {},
          onShowWhosUsing: (_) {},
          onShowDetails: (_) {},
          onShowCalendar: () {},
          onMarkReturned: () {},
          onShowAccess: () {},
          onManage: () {},
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  group('GearReadShell giveaway role line (#2724)', () {
    testWidgets('an open giveaway advertises "Giving this away for free"', (
      tester,
    ) async {
      await _pump(
        tester,
        _giveawayState(phase: GiveawayPhase.GIVEAWAY_PHASE_OPEN),
      );

      expect(find.text('Giving this away for free'), findsOneWidget);
      expect(find.text('Gone to a new home'), findsNothing);
    });

    testWidgets(
      'a completed giveaway flips the owner-card status line to '
      '"Gone to a new home"',
      (tester) async {
        await _pump(
          tester,
          _giveawayState(phase: GiveawayPhase.GIVEAWAY_PHASE_COMPLETED),
        );

        // The owner card stops advertising the item above the GIVEAWAY
        // COMPLETED summary.
        expect(find.text('Giving this away for free'), findsNothing);
        expect(find.text('Gone to a new home'), findsOneWidget);
        // The who card shows the completed summary with the recipient.
        expect(find.text('Giveaway completed'.toUpperCase()), findsOneWidget);
        expect(find.text('Dana Reed'), findsOneWidget);
        expect(find.text('RECEIVED IT'), findsOneWidget);
        // Nobody can act on an invitation to an item that is already gone.
        expect(find.byType(SharedWithCard), findsNothing);
      },
    );

    testWidgets('an open giveaway still offers to widen the audience', (
      tester,
    ) async {
      await _pump(tester, _giveawayState(phase: GiveawayPhase.GIVEAWAY_PHASE_OPEN));

      expect(find.byType(SharedWithCard), findsOneWidget);
    });
  });

  group('GearReadShell item-details card (#2724)', () {
    testWidgets('summarizes brand and model when detection found them', (
      tester,
    ) async {
      await _pump(
        tester,
        _metadataState(
          GearMetadata(
            brand: TrackedString(value: 'Craftsman'),
            model: TrackedString(value: 'M110'),
          ),
        ),
      );

      expect(find.text('Craftsman · M110'), findsOneWidget);
    });

    testWidgets('falls back to the category when there is no brand or model', (
      tester,
    ) async {
      await _pump(
        tester,
        _metadataState(
          GearMetadata(category: TrackedString(value: 'Yard & Garden')),
        ),
      );

      expect(find.text('Yard & Garden'), findsOneWidget);
    });

    testWidgets('falls back to the estimated value — the food-item path', (
      tester,
    ) async {
      await _pump(
        tester,
        _metadataState(
          GearMetadata(valueEstimate: ValueEstimate(estimatedValueUsd: 12)),
        ),
      );

      expect(find.text(r'~$12'), findsOneWidget);
    });

    testWidgets(
      'never echoes its own label when detection found nothing to summarize',
      (tester) async {
        await _pump(tester, _metadataState(GearMetadata()));

        // The card's label stays, but its value must not repeat it — a
        // literal "ITEM DETAILS / Item Details" read as an unfilled
        // placeholder next to the real WHERE card.
        expect(find.text('Item Details'), findsNothing);
        expect(find.text('View details'), findsOneWidget);
      },
    );
  });

  group('desktop caption column (#2912)', () {
    Future<void> pumpAt(WidgetTester tester, Size surface) async {
      tester.view.physicalSize = surface;
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            gearProvider('gear-1').overrideWith(
              () => _FakeGearNotifier(_metadataState(GearMetadata())),
            ),
          ],
          child: localizedApp(
            GearReadShell(
              gearId: 'gear-1',
              accentColor: const Color(0xFFEF7A6D),
              onExpandConversation: (_) {},
              onShowLocation: (_) {},
              onShowWhosUsing: (_) {},
              onShowDetails: (_) {},
              onShowCalendar: () {},
              onMarkReturned: () {},
              onShowAccess: () {},
              onManage: () {},
            ),
          ),
        ),
      );
    }

    testWidgets('the sheet holds the centered measure at 1440x810',
        (tester) async {
      const surface = Size(1440, 810);
      await pumpAt(tester, surface);

      final wash = tester.getRect(find.byType(HeroContentWash));
      expect(wash.width, Responsive.contentMaxWidth);
      expect(wash.center.dx, moreOrLessEquals(surface.width / 2, epsilon: 1));

      // The workflow CTA card stays on-surface inside the column.
      final who = tester.getRect(find.byType(GearWhoCard));
      expect(who.bottom, lessThanOrEqualTo(surface.height));
      expect(who.left,
          greaterThanOrEqualTo((surface.width - Responsive.contentMaxWidth) / 2));
    });

    testWidgets('phone geometry is unchanged at 390x844', (tester) async {
      const surface = Size(390, 844);
      await pumpAt(tester, surface);

      final wash = tester.getRect(find.byType(HeroContentWash));
      expect(wash.width, surface.width);
    });
  });
}
