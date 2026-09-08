import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart'
    show ActiveLoan, Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearPeopleResponse, GetGearResponse, GetGearStatsResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_whos_using_panel.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

/// A GearNotifier stub that returns a fixed [GearState] (bypasses build()'s
/// provider reads) so the panel can be pumped against canned data.
class _FakeGearNotifier extends GearNotifier {
  _FakeGearNotifier(this._state) : super('gear-1');
  final GearState _state;
  @override
  GearState build() => _state;
}

Widget _panel(GearState state) {
  return ProviderScope(
    overrides: [
      gearProvider('gear-1').overrideWith(() => _FakeGearNotifier(state)),
    ],
    child: localizedApp(
      GearWhosUsingPanel(
        gearId: 'gear-1',
        accentColor: const Color(0xFF8FAE7E),
        actionItemsBuilder: (_) => const [],
      ),
    ),
  );
}

GearState _loanState({
  ActiveLoan? activeLoan,
  GetGearStatsResponse? stats,
  GetGearPeopleResponse? people,
}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: 'viewer-1',
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Hammer',
      owner: User(id: 'owner-1', name: 'Thomas'),
      availability: Availability.AVAILABILITY_FOR_LOAN,
      activeLoan: activeLoan,
    ),
    gearStats: stats,
    gearPeople: people,
  );
}

void main() {
  group('GearWhosUsingPanel', () {
    testWidgets('renders the three story tiles from stats', (tester) async {
      await tester.pumpWidget(_panel(_loanState(
        stats: GetGearStatsResponse(
          timesLoaned: 7,
          peopleHelped: 4,
          valueSharedUsd: 140,
        ),
      )));
      await tester.pump();

      expect(find.text('7'), findsOneWidget);
      expect(find.text('4'), findsOneWidget);
      expect(find.text('~\$140'), findsOneWidget);
      expect(find.text('times shared'), findsOneWidget);
    });

    testWidgets('shows the current holder with the "Has it now" badge',
        (tester) async {
      await tester.pumpWidget(_panel(_loanState(
        activeLoan: ActiveLoan(borrower: User(id: 'b1', name: 'Maya')),
      )));
      await tester.pump();

      expect(find.text('Maya'), findsOneWidget);
      expect(find.text('HAS IT NOW'), findsOneWidget);
      expect(find.text('Who has it now'.toUpperCase()), findsOneWidget);
    });

    testWidgets('lists past borrowers under their section', (tester) async {
      await tester.pumpWidget(_panel(_loanState(
        people: GetGearPeopleResponse(
          pastBorrowers: [User(id: 'b2', name: 'Jamie')],
        ),
      )));
      await tester.pump();

      expect(find.text('Past borrowers'.toUpperCase()), findsOneWidget);
      expect(find.text('Jamie'), findsOneWidget);
    });

    testWidgets('shows the available empty state when no one has it',
        (tester) async {
      await tester.pumpWidget(_panel(_loanState()));
      await tester.pump();

      expect(
        find.text("It's home and free — grab it whenever."),
        findsOneWidget,
      );
    });
  });
}
