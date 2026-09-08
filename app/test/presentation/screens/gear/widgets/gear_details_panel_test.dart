import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_details_panel.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

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
    child: localizedApp(const GearDetailsPanel(gearId: 'gear-1')),
  );
}

GearState _state({required String currentUserId}) {
  return GearState(
    gearId: 'gear-1',
    currentUserId: currentUserId,
    isLoading: false,
    gearDetails: GetGearResponse(
      id: 'gear-1',
      name: 'Tent',
      owner: User(id: 'owner-1', name: 'Owner'),
    ),
  );
}

void main() {
  group('GearDetailsPanel', () {
    testWidgets('owner sees the edit button', (tester) async {
      await tester.pumpWidget(_panel(_state(currentUserId: 'owner-1')));
      await tester.pump();
      expect(find.byIcon(Icons.edit_outlined), findsOneWidget);
    });

    testWidgets('non-owner does not see the edit button', (tester) async {
      await tester.pumpWidget(_panel(_state(currentUserId: 'viewer-1')));
      await tester.pump();
      expect(find.byIcon(Icons.edit_outlined), findsNothing);
    });

    testWidgets('tapping edit opens inline metadata editing in place',
        (tester) async {
      await tester.pumpWidget(_panel(_state(currentUserId: 'owner-1')));
      await tester.pump();

      await tester.tap(find.byIcon(Icons.edit_outlined));
      await tester.pump();

      // Still on the details screen (not popped) — now showing editable fields
      // with save / cancel actions.
      expect(find.byType(GearDetailsPanel), findsOneWidget);
      expect(find.byType(TextFormField), findsWidgets);
      expect(find.byIcon(Icons.check_rounded), findsOneWidget);
      // No title/description fields — metadata only (brand row label present).
      expect(find.text('Brand'), findsOneWidget);
    });
  });
}
