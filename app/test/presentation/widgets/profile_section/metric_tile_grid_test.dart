import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/metric_tile_grid.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) {
  return ProviderScope(child: localizedApp(child));
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('MetricTileGrid', () {
    testWidgets('renders value, unit, and label for each tile',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const MetricTileGrid(tiles: [
          MetricTileData(value: '6', label: 'Problems solved'),
          MetricTileData(value: '10', unit: 'hrs', label: 'Time together'),
          MetricTileData(value: r'$40', label: 'Money saved'),
          MetricTileData(value: '18', unit: 'kg', label: 'CO₂ avoided'),
        ]),
      ));
      await tester.pumpAndSettle();

      expect(find.text('6'), findsOneWidget);
      expect(find.text('10'), findsOneWidget);
      expect(find.text('hrs'), findsOneWidget);
      expect(find.text(r'$40'), findsOneWidget);
      expect(find.text('18'), findsOneWidget);
      expect(find.text('kg'), findsOneWidget);
      // Labels are uppercased for display.
      expect(find.text('PROBLEMS SOLVED'), findsOneWidget);
      expect(find.text('TIME TOGETHER'), findsOneWidget);
    });

    testWidgets('invokes onTap when a tile is tapped', (tester) async {
      var tapped = 0;
      await tester.pumpWidget(_wrap(
        MetricTileGrid(tiles: [
          MetricTileData(
            value: '6',
            label: 'Problems solved',
            onTap: () => tapped++,
          ),
        ]),
      ));
      await tester.pumpAndSettle();

      await tester.tap(find.text('6'));
      expect(tapped, 1);
    });

    testWidgets('inert tile (no onTap) does not crash on tap', (tester) async {
      await tester.pumpWidget(_wrap(
        const MetricTileGrid(tiles: [
          MetricTileData(value: '0', label: 'Money saved'),
        ]),
      ));
      await tester.pumpAndSettle();
      await tester.tap(find.text('0'));
      expect(find.text('0'), findsOneWidget);
    });
  });
}
