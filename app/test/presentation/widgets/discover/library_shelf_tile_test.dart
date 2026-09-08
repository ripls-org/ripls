import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability, Gear;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType, SearchResultItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/widgets/discover/library_shelf_tile.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

SearchDiscoverItem _gearItem({
  String ownerName = 'Betty Ripls',
  Availability availability = Availability.AVAILABILITY_FOR_LOAN,
}) {
  return SearchDiscoverItem(SearchResultItem(
    itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
    gear: Gear(
      id: 'g1',
      name: 'Crash pad',
      owner: User(id: 'owner-1', name: ownerName),
      availability: availability,
    ),
  ));
}

Widget _wrap(Widget child) {
  return ProviderScope(child: localizedApp(Center(child: child)));
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('LibraryShelfTile', () {
    testWidgets('renders the name at the bigger 150×144 size with no pill',
        (tester) async {
      await tester.pumpWidget(_wrap(
        LibraryShelfTile(item: _gearItem(), onTap: () {}),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Crash pad'), findsOneWidget);
      // Borrowable is the default and wears no pill.
      expect(find.text('Giveaway'), findsNothing);
      final size = tester.getSize(find.byType(LibraryShelfTile));
      expect(size, const Size(150, 144));
    });

    testWidgets('giveaway gear shows the Giveaway pill', (tester) async {
      await tester.pumpWidget(_wrap(
        LibraryShelfTile(
          item: _gearItem(
              availability: Availability.AVAILABILITY_FOR_GIVEAWAY),
          onTap: () {},
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Giveaway'), findsOneWidget);
    });

    testWidgets('tap fires onTap', (tester) async {
      var tapped = 0;
      await tester.pumpWidget(_wrap(
        LibraryShelfTile(item: _gearItem(), onTap: () => tapped++),
      ));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Crash pad'));
      expect(tapped, 1);
    });
  });

  group('LibraryShelfGhostTile', () {
    testWidgets('renders the add label at the bigger size and fires onTap',
        (tester) async {
      var tapped = 0;
      await tester.pumpWidget(_wrap(
        LibraryShelfGhostTile(onTap: () => tapped++),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Add to this shelf'), findsOneWidget);
      final size = tester.getSize(find.byType(LibraryShelfGhostTile));
      expect(size, const Size(150, 144));
      await tester.tap(find.text('Add to this shelf'));
      expect(tapped, 1);
    });
  });
}
