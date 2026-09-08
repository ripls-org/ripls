import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/metric_tile_grid.dart'
    show MetricTileData;
import 'package:ripls/presentation/widgets/profile_section/profile_metric_rows.dart';
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

  group('ProfileMetricRows', () {
    testWidgets('renders one row per metric with label, value, and unit',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileMetricRows(metrics: [
          MetricTileData(value: '6', label: 'Problems solved'),
          MetricTileData(value: '10', unit: 'hrs', label: 'Time together'),
          MetricTileData(value: r'$40', label: 'Money saved'),
          MetricTileData(value: '18', unit: 'kg', label: 'CO₂ avoided'),
        ]),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Problems solved'), findsOneWidget);
      expect(find.text('6'), findsOneWidget);
      expect(find.text('Time together'), findsOneWidget);
      // Value + unit render as one rich-text run.
      expect(find.text('10 hrs'), findsOneWidget);
      expect(find.text(r'$40'), findsOneWidget);
      expect(find.text('18 kg'), findsOneWidget);
    });

    testWidgets(
        'tappable row fires onTap and carries the v11 chevron; inert '
        'rows carry none', (tester) async {
      var tapped = 0;
      await tester.pumpWidget(_wrap(
        ProfileMetricRows(metrics: [
          MetricTileData(
            value: '6',
            label: 'Problems solved',
            subtitle: 'asks answered by a friend',
            onTap: () => tapped++,
          ),
          const MetricTileData(value: '10', label: 'Time together'),
        ]),
      ));
      await tester.pumpAndSettle();

      // Only the tappable row carries the drill-down chevron, and its
      // plain-language subtext renders under the label.
      expect(find.byIcon(Icons.chevron_right), findsOneWidget);
      expect(find.text('asks answered by a friend'), findsOneWidget);
      await tester.tap(find.text('Problems solved'));
      expect(tapped, 1);
    });

    testWidgets(
        'rect-aware row fires onTapRect with its own footprint '
        '(the morph-panel source rect)', (tester) async {
      Rect? tappedRect;
      await tester.pumpWidget(_wrap(
        ProfileMetricRows(metrics: [
          MetricTileData(
            value: '9',
            label: 'Library',
            onTapRect: (rect) => tappedRect = rect,
          ),
        ]),
      ));
      await tester.pumpAndSettle();

      // Rect-aware rows carry the drill-down chevron too.
      expect(find.byIcon(Icons.chevron_right), findsOneWidget);
      await tester.tap(find.text('Library'));
      expect(tappedRect, isNotNull);
      expect(tappedRect, isNot(Rect.zero));
    });

    testWidgets('inert row (no onTap) does not crash on tap', (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileMetricRows(metrics: [
          MetricTileData(value: '0', label: 'Money saved'),
        ]),
      ));
      await tester.pumpAndSettle();
      await tester.tap(find.text('0'));
      expect(find.text('0'), findsOneWidget);
    });

    testWidgets('renders nothing when metrics is empty', (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileMetricRows(metrics: []),
      ));
      await tester.pumpAndSettle();
      expect(find.byType(Row), findsNothing);
    });
  });
}
