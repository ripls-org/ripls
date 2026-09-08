import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/presentation/widgets/profile_section/open_now_list.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) {
  return ProviderScope(
    child: localizedApp(SingleChildScrollView(child: child)),
  );
}

Item _item(String id, ItemKind kind, String title, {String? subtitle}) {
  final item = Item(contextId: id, kind: kind, title: title);
  if (subtitle != null) item.subtitle = subtitle;
  return item;
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('OpenNowList', () {
    testWidgets('renders nothing when items is empty', (tester) async {
      await tester.pumpWidget(_wrap(
        const OpenNowList(items: [], kicker: 'Open right now'),
      ));
      await tester.pumpAndSettle();
      expect(find.text('OPEN RIGHT NOW'), findsNothing);
    });

    testWidgets('renders uppercased kicker, titles, and subtitles',
        (tester) async {
      await tester.pumpWidget(_wrap(
        OpenNowList(
          items: [
            _item('e1', ItemKind.ITEM_KIND_EXPERIENCE, 'Maple Canyon climb',
                subtitle: 'In 3 days'),
            _item('g1', ItemKind.ITEM_KIND_GEAR, 'Crash pad'),
          ],
          kicker: 'Open right now',
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('OPEN RIGHT NOW'), findsOneWidget);
      expect(find.text('Maple Canyon climb'), findsOneWidget);
      expect(find.text('In 3 days'), findsOneWidget);
      expect(find.text('Crash pad'), findsOneWidget);
    });

    testWidgets('derives the verb from the item kind', (tester) async {
      await tester.pumpWidget(_wrap(
        OpenNowList(
          items: [
            _item('e1', ItemKind.ITEM_KIND_EXPERIENCE, 'Climb'),
            _item('g1', ItemKind.ITEM_KIND_GEAR, 'Crash pad'),
            _item('v1', ItemKind.ITEM_KIND_GIVEAWAY, 'Chainsaw'),
            _item('r1', ItemKind.ITEM_KIND_REQUEST, 'Belay partner'),
          ],
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Join'), findsOneWidget);
      expect(find.text('Borrow'), findsOneWidget);
      expect(find.text('Claim'), findsOneWidget);
      expect(find.text("I'm in"), findsOneWidget);
    });

    testWidgets('caps rows at maxRows', (tester) async {
      await tester.pumpWidget(_wrap(
        OpenNowList(
          items: [
            _item('1', ItemKind.ITEM_KIND_GEAR, 'One'),
            _item('2', ItemKind.ITEM_KIND_GEAR, 'Two'),
            _item('3', ItemKind.ITEM_KIND_GEAR, 'Three'),
            _item('4', ItemKind.ITEM_KIND_GEAR, 'Four'),
          ],
          maxRows: 3,
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('One'), findsOneWidget);
      expect(find.text('Three'), findsOneWidget);
      expect(find.text('Four'), findsNothing);
    });

    testWidgets('verb button and row dispatch the tapped item',
        (tester) async {
      final tapped = <String>[];
      await tester.pumpWidget(_wrap(
        OpenNowList(
          items: [
            _item('g1', ItemKind.ITEM_KIND_GEAR, 'Crash pad'),
          ],
          onOpenItem: (item) => tapped.add(item.contextId),
        ),
      ));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Borrow'));
      await tester.tap(find.text('Crash pad'));
      expect(tapped, ['g1', 'g1']);
    });
  });
}
