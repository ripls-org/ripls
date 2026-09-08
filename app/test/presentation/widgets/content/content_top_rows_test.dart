import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/content/content_info_row.dart';
import 'package:ripls/presentation/widgets/content/content_title_row.dart';
import 'package:ripls/presentation/widgets/content/content_top_rows.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('ContentTitleRow', () {
    testWidgets('renders title in the serif heading font', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentTitleRow(title: 'Morning Hike'),
          ),
        ),
      );

      final textFinder = find.text('Morning Hike');
      expect(textFinder, findsOneWidget);
      final text = tester.widget<Text>(textFinder);
      expect(text.style?.fontFamily, AppTheme.headingFont);
      expect(text.style?.color, Colors.white);
    });

  });

  group('ContentInfoRow', () {
    testWidgets('renders icon and value', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentInfoRow(
              icon: Icons.access_time_rounded,
              value: 'Sun, May 10',
              rowSemanticsLabel: 'Edit time',
            ),
          ),
        ),
      );

      expect(find.byIcon(Icons.access_time_rounded), findsOneWidget);
      expect(find.text('Sun, May 10'), findsOneWidget);
    });

    testWidgets('renders trailing text when provided', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentInfoRow(
              icon: Icons.person_outline,
              value: 'Thomas',
              rowSemanticsLabel: 'Thomas',
              trailingText: 'shared 1d ago',
            ),
          ),
        ),
      );

      expect(find.text('shared 1d ago'), findsOneWidget);
    });

    testWidgets('invokes onTap when tapped', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: ContentInfoRow(
              icon: Icons.location_on_outlined,
              value: 'Mt Sanitas',
              rowSemanticsLabel: 'Edit location',
              onTap: () => taps++,
            ),
          ),
        ),
      );

      await tester.tap(find.text('Mt Sanitas'));
      await tester.pump();
      expect(taps, 1);
    });

    testWidgets('asserts that actionIcon requires actionSemanticsLabel',
        (tester) async {
      expect(
        () => ContentInfoRow(
          icon: Icons.access_time_rounded,
          value: 'X',
          rowSemanticsLabel: 'X',
          actionIcon: Icons.calendar_today,
          onAction: () {},
        ),
        throwsAssertionError,
      );
    });
  });

  group('ContentTopRows', () {
    testWidgets('renders title and description slot in order', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentTopRows(
              title: 'Morning Hike',
              descriptionSlot: Text('Moderate pace, all levels welcome.'),
              rows: [],
            ),
          ),
        ),
      );

      final titlePos = tester.getCenter(find.text('Morning Hike'));
      final descPos =
          tester.getCenter(find.text('Moderate pace, all levels welcome.'));
      expect(descPos.dy, greaterThan(titlePos.dy),
          reason: 'description should render below the title');
    });

    testWidgets('renders info rows after the description', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentTopRows(
              title: 'X',
              descriptionSlot: Text('Description'),
              rows: [
                ContentInfoRow(
                  icon: Icons.access_time_rounded,
                  value: 'Sun, May 10',
                  rowSemanticsLabel: 'Edit time',
                ),
              ],
            ),
          ),
        ),
      );

      final descPos = tester.getCenter(find.text('Description'));
      final rowPos = tester.getCenter(find.text('Sun, May 10'));
      expect(rowPos.dy, greaterThan(descPos.dy),
          reason: 'info rows should render below the description');
    });

    testWidgets('uses description text when no descriptionSlot is given',
        (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(
            body: ContentTopRows(
              title: 'X',
              description: 'Plain description',
            ),
          ),
        ),
      );

      expect(find.text('Plain description'), findsOneWidget);
    });
  });
}
