import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/widgets/experience_location_panel.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_panel_providers.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

import '../../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

/// Returns fixed [LocationModalData] from build(), bypassing the real
/// notifier's network load so the panel renders in isolation.
class _FakeModalNotifier extends LocationModalNotifier {
  _FakeModalNotifier(this._data) : super(_id);
  final LocationModalData _data;

  @override
  Future<LocationModalData> build() async => _data;
}

ProviderScope _host(
  LocationModalData data, {
  LocationPanelGeo geo = const LocationPanelGeo(proposals: []),
}) {
  return ProviderScope(
    overrides: [
      locationModalProvider(_id).overrideWith(() => _FakeModalNotifier(data)),
      // Proposals are resolved without coordinates → no map markers, so
      // GoogleMap is never mounted and the test stays free of platform-view
      // plugins.
      locationPanelGeoProvider(_id).overrideWith((ref) async => geo),
      // Avoid the geolocator plugin in tests.
      userLocationProvider(LocationIntent.precisePin)
          .overrideWith((ref) async => null),
    ],
    child: localizedApp(
      const ExperienceLocationPanel(
        experienceId: _id,
        accentColor: Color(0xFF7A9B8C),
      ),
    ),
  );
}

LocationProposal _proposal(String id, String locationId, {User? by}) {
  final p = LocationProposal()
    ..id = id
    ..location = (ProposedLocation()..locationId = locationId);
  if (by != null) p.proposedBy = by;
  return p;
}

ResolvedLocationProposal _resolved(LocationProposal p, String name) =>
    ResolvedLocationProposal(proposal: p, name: name);

void main() {
  group('ExperienceLocationPanel (TBD state)', () {
    testWidgets('owner sees the pick / ask-the-group controls', (tester) async {
      await tester.pumpWidget(
        _host(const LocationModalData(isOrganizer: true)),
      );
      await tester.pumpAndSettle();

      // Header.
      expect(find.text('Where'), findsOneWidget);
      // Owner primary + secondary actions.
      expect(find.text('Pick a spot'), findsOneWidget);
      expect(find.text('Add spots & ask the group'), findsOneWidget);
    });

    testWidgets('non-owner sees the read-only host note and no controls', (
      tester,
    ) async {
      await tester.pumpWidget(
        _host(const LocationModalData(isOrganizer: false)),
      );
      await tester.pumpAndSettle();

      expect(
        find.text("The host hasn't picked a spot yet."),
        findsOneWidget,
      );
      expect(find.text('Pick a spot'), findsNothing);
      expect(find.text('Add spots & ask the group'), findsNothing);
    });
  });

  group('ExperienceLocationPanel (active poll)', () {
    testWidgets('owner can add another spot to a running poll', (tester) async {
      final p1 = _proposal('p1', 'loc-1');
      await tester.pumpWidget(
        _host(
          LocationModalData(
            isOrganizer: true,
            locationPollActive: true,
            currentLocationPollId: 'poll-1',
            proposals: [p1],
          ),
          geo: LocationPanelGeo(
            // No coordinate → no map marker, so GoogleMap is not mounted.
            proposals: [ResolvedLocationProposal(proposal: p1, name: 'The Sink')],
          ),
        ),
      );
      await tester.pumpAndSettle();

      // The proposal renders as a vote row...
      expect(find.text('The Sink'), findsOneWidget);
      // ...and the owner sees the inline "Add another spot" affordance (#2291
      // follow-up: previously hidden for organizers).
      expect(find.text('Add another spot'), findsOneWidget);
    });

    testWidgets('owner can open the inline set-final-spot view', (tester) async {
      final p1 = _proposal('p1', 'loc-1');
      final p2 = _proposal('p2', 'loc-2');
      await tester.pumpWidget(
        _host(
          LocationModalData(
            isOrganizer: true,
            locationPollActive: true,
            currentLocationPollId: 'poll-1',
            proposals: [p1, p2],
          ),
          geo: LocationPanelGeo(
            proposals: [_resolved(p1, 'The Sink'), _resolved(p2, 'Pearl St')],
          ),
        ),
      );
      await tester.pumpAndSettle();

      // The owner's primary action opens the inline confirm — no pushed modal.
      final setFinal = find.text('SET THE FINAL SPOT').last;
      await tester.ensureVisible(setFinal);
      await tester.pumpAndSettle();
      await tester.tap(setFinal);
      await tester.pumpAndSettle();

      // The set-final view's lock-in button is now on screen.
      expect(find.text('Lock in & notify group'), findsOneWidget);
      // It re-uses the inline panel, not a route, so the breakdown is visible.
      expect(find.text('HOW EVERYONE PICKED'), findsOneWidget);
    });
  });

  group('ExperienceLocationPanel (single spot, n=1)', () {
    testWidgets('non-owner sees works / doesn\'t-work, not poll framing', (
      tester,
    ) async {
      final p1 = _proposal('p1', 'loc-1', by: User(id: 'maya', name: 'Maya'));
      await tester.pumpWidget(
        _host(
          LocationModalData(
            isOrganizer: false,
            currentUserId: 'viewer',
            locationPollActive: true,
            currentLocationPollId: 'poll-1',
            proposals: [p1],
          ),
          geo: LocationPanelGeo(proposals: [_resolved(p1, 'Betasso Trailhead')]),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Betasso Trailhead'), findsOneWidget);
      expect(find.text('Works for me'), findsOneWidget);
      expect(find.text("Doesn't work"), findsOneWidget);
      // Single-spot framing replaces the poll subtitle.
      expect(find.text('Where works for you?'), findsNothing);
    });
  });
}
