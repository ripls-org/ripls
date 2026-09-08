import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/settings/deleted_countdown_badge.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('daysLeftUntilPurge', () {
    const day = 86400;

    test('full window remaining returns 30', () {
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000,
      );
      expect(result, 30);
    });

    test('one day past delete returns 29', () {
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000 + day,
      );
      expect(result, 29);
    });

    test('exactly one day before purge returns 1', () {
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000 + 29 * day,
      );
      expect(result, 1);
    });

    test('partial day before purge still reads as 1 (ceiling)', () {
      // 29.4 days elapsed → 0.6 days left → ceiling to 1.
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000 + (29 * day + day ~/ 2),
      );
      expect(result, 1);
    });

    test('exactly at purge boundary returns 0', () {
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000 + 30 * day,
      );
      expect(result, 0);
    });

    test('past the purge boundary clamps to 0', () {
      final result = daysLeftUntilPurge(
        deletedAtUnixSec: 1000,
        nowUnixSec: 1000 + 60 * day,
      );
      expect(result, 0);
    });
  });

  group('DeletedCountdownBadge', () {
    Future<void> pumpBadge(WidgetTester tester, int daysLeft) async {
      await tester.pumpWidget(localizedApp(
        Scaffold(body: Center(child: DeletedCountdownBadge(daysLeft: daysLeft))),
      ));
    }

    testWidgets('renders "Deleted, 30 days left" at full window',
        (tester) async {
      await pumpBadge(tester, 30);
      expect(find.text('Deleted, 30 days left'), findsOneWidget);
    });

    testWidgets('renders "Deleted, 15 days left" mid-window', (tester) async {
      await pumpBadge(tester, 15);
      expect(find.text('Deleted, 15 days left'), findsOneWidget);
    });

    testWidgets('renders "Deletes tomorrow" at 1 day left', (tester) async {
      await pumpBadge(tester, 1);
      expect(find.text('Deletes tomorrow'), findsOneWidget);
    });

    testWidgets('renders "Deletes today" at 0 days left', (tester) async {
      await pumpBadge(tester, 0);
      expect(find.text('Deletes today'), findsOneWidget);
    });

    testWidgets('wraps the badge in a Semantics with the visible label',
        (tester) async {
      await pumpBadge(tester, 5);

      // Per the avoid_color_only_status lint rule, the *Badge widget
      // must carry a Semantics annotation matching the visible
      // urgency cue. We assert the Semantics widget directly (rather
      // than via the rendered tree) because the badge intentionally
      // does not exclude its child Text's semantics — the merged
      // node is what's expected.
      final semantics = tester.widgetList<Semantics>(find.byType(Semantics));
      final hasMatchingLabel = semantics.any(
        (s) => s.properties.label == 'Deleted, 5 days left',
      );
      expect(hasMatchingLabel, isTrue);
    });
  });
}
