import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/gear_content_view.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/l10n_helpers.dart';

/// A GearNotifier stub returning a fixed [GearState]. [initialize] is a no-op
/// so the content view's post-frame init doesn't overwrite the fixture.
class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;

  @override
  GearState build() => _state;

  @override
  Future<void> initialize({
    required String gearId,
    required String? currentUserId,
    String? communityId,
    double? initialDistanceMeters,
  }) async {}
}

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'viewer-1', name: 'Viewer'),
      );
}

GetGearResponse _giveawayDetails() => GetGearResponse(
      id: 'gear-1',
      name: 'Ladder',
      description: 'Sturdy 8-foot ladder.',
      owner: User(id: 'owner-1', name: 'Thomas'),
      availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
    );

/// Mid-first-hydration: gearDetails has landed but the follow-up fetches
/// (location, owner profile, transfer context) are still in flight.
GearState _hydratingState() => GearState(
      gearId: 'gear-1',
      currentUserId: 'viewer-1',
      isLoading: true,
      hasHydrated: false,
      gearDetails: _giveawayDetails(),
    );

/// Fully hydrated with genuinely-empty data: no location, no interest.
/// Viewed as the OWNER — for a non-owner the roster shows a "you haven't
/// raised your hand" row instead of the empty-state sentence.
GearState _hydratedEmptyState() => GearState(
      gearId: 'gear-1',
      currentUserId: 'owner-1',
      isLoading: false,
      hasHydrated: true,
      gearDetails: _giveawayDetails(),
      transferContext: GearTransferContext(
        isOwner: true,
        overallPhase: GiveawayPhase.GIVEAWAY_PHASE_OPEN,
      ),
    );

Future<void> _pump(WidgetTester tester, GearState state) async {
  tester.view.physicalSize = const Size(1200, 3000);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
      ],
      child: localizedApp(const GearContentView(gearId: 'gear-1')),
    ),
  );
  await tester.pump();
}

void main() {
  group('GearContentView first-hydration gating (#2724)', () {
    testWidgets(
        'mid-hydration shows the loading view — no placeholder facts, '
        'no false empty-state copy', (tester) async {
      await _pump(tester, _hydratingState());

      // The loading treatment, not the half-hydrated read shell.
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(find.text('Ladder'), findsNothing);

      // No "WHERE / TBD" placeholder fact while the location is resolving.
      expect(find.text('TBD'), findsNothing);

      // No false empty state while the interest query is in flight.
      expect(
        find.textContaining("No one's raised their hand"),
        findsNothing,
      );
    });

    testWidgets(
        'after hydration the shell renders, and genuinely-empty data may '
        'show its real empty state', (tester) async {
      await _pump(tester, _hydratedEmptyState());

      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(find.text('Ladder'), findsOneWidget);

      // Data-loaded-and-empty is the legitimate case for the empty-state
      // sentence — the C3 rule is about queries in flight, not empty data.
      expect(
        find.textContaining("No one's raised their hand"),
        findsOneWidget,
      );
    });
  });
}
