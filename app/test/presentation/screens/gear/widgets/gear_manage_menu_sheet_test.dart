import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

GearState _state({
  required Availability availability,
  int timesLoaned = 0,
}) {
  final gear = GetGearResponse(
    id: 'gear-1',
    name: 'Hammer',
    owner: User(id: 'owner-1', name: 'Thomas'),
    availability: availability,
    timesLoaned: timesLoaned,
  );
  return GearState(
    gearId: 'gear-1',
    currentUserId: 'owner-1',
    gearDetails: gear,
    isLoading: false,
  );
}

void main() {
  group('GearManageMenuSheet', () {
    testWidgets('loan gear shows the item-level rows', (tester) async {
      await tester.pumpWidget(localizedApp(
        GearManageMenuSheet.forState(
          _state(availability: Availability.AVAILABILITY_FOR_LOAN),
        ),
      ));
      await tester.pump();

      expect(find.text('Set details'), findsOneWidget);
      expect(find.text('Set location'), findsOneWidget);
      expect(find.text('Close Lending'), findsOneWidget);
    });

    testWidgets('View Impact is hidden when the gear has never been loaned',
        (tester) async {
      await tester.pumpWidget(localizedApp(
        GearManageMenuSheet.forState(
          _state(availability: Availability.AVAILABILITY_FOR_LOAN),
        ),
      ));
      await tester.pump();

      expect(find.text('View Impact'), findsNothing);
    });

    testWidgets('View Impact appears once the gear has been loaned',
        (tester) async {
      await tester.pumpWidget(localizedApp(
        GearManageMenuSheet.forState(
          _state(
            availability: Availability.AVAILABILITY_FOR_LOAN,
            timesLoaned: 3,
          ),
        ),
      ));
      await tester.pump();

      expect(find.text('View Impact'), findsOneWidget);
    });

    testWidgets('giveaway gear uses the Close Giveaway destructive row',
        (tester) async {
      await tester.pumpWidget(localizedApp(
        GearManageMenuSheet.forState(
          _state(availability: Availability.AVAILABILITY_FOR_GIVEAWAY),
        ),
      ));
      await tester.pump();

      expect(find.text('Close Giveaway'), findsOneWidget);
      expect(find.text('Close Lending'), findsNothing);
    });
  });
}
