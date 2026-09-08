import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';

/// Unit coverage for the month-grid cell arithmetic behind #2908 — the app was
/// unusable in landscape browser windows because square day cells sized
/// themselves from width alone, so the grid's height grew with the window's
/// width until it swallowed the day-detail panel below it.
void main() {
  group('monthGridCellSide', () {
    test('width binds on a phone, leaving the pre-fix layout unchanged', () {
      // 390-wide phone, 6 rows, generous height budget: width is the smaller
      // constraint, so the cell is exactly what the old width-only maths gave.
      const availableWidth = 390.0 - 36; // screen minus the 18pt side padding
      final side = monthGridCellSide(
        availableWidth: availableWidth,
        maxHeight: 844 * monthGridHeightShare,
        rows: 6,
      );
      expect(side, closeTo((availableWidth - monthGridSpacing * 6) / 7, 0.001));
    });

    test('height binds on a landscape window', () {
      // The reporter's class of viewport. Width alone would give a ~196pt cell
      // and a grid taller than the whole screen.
      const availableWidth = 1440.0 - 36;
      const budget = 810.0 * monthGridHeightShare;
      final side = monthGridCellSide(
        availableWidth: availableWidth,
        maxHeight: budget,
        rows: 6,
      );
      final widthDerived = (availableWidth - monthGridSpacing * 6) / 7;
      expect(side, lessThan(widthDerived));
      expect(monthGridHeight(side, 6), lessThanOrEqualTo(budget + 0.001));
    });

    test('the grid fits its budget across a spread of viewports', () {
      // The matrix probe-03 swept, plus the wide-and-tall case that proved the
      // trigger is width rather than height.
      const viewports = <List<double>>[
        [390, 844],
        [390, 620],
        [900, 810],
        [1205, 1080],
        [1440, 810],
        [1440, 1400],
        [1920, 1080],
      ];
      for (final vp in viewports) {
        for (final rows in [4, 5, 6]) {
          final budget = vp[1] * monthGridHeightShare;
          final side = monthGridCellSide(
            availableWidth: vp[0] - 36,
            maxHeight: budget,
            rows: rows,
          );
          expect(
            monthGridHeight(side, rows),
            lessThanOrEqualTo(budget + 0.001),
            reason: 'grid overflows its budget at ${vp[0]}x${vp[1]}, $rows rows',
          );
          expect(
            monthGridWidth(side),
            lessThanOrEqualTo(vp[0] - 36 + 0.001),
            reason: 'grid overflows its width at ${vp[0]}x${vp[1]}',
          );
        }
      }
    });

    test('an unbounded budget still caps the cell so a month stays readable',
        () {
      // The scroll-view case (gear booking grid): height never binds, so only
      // the absolute cap stops cells ballooning on a wide window.
      final side = monthGridCellSide(
        availableWidth: 1920,
        maxHeight: double.infinity,
        rows: 6,
      );
      expect(side, 96);
    });

    test('a degenerate budget yields a positive extent rather than a crash',
        () {
      // A caller mid-animation can hand us a zero or negative budget; layout
      // must never see a negative extent.
      for (final budget in [0.0, -500.0, 10.0]) {
        final side = monthGridCellSide(
          availableWidth: 400,
          maxHeight: budget,
          rows: 6,
        );
        expect(side, greaterThan(0));
      }
    });
  });
}
