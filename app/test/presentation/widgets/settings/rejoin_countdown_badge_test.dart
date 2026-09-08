import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/settings/rejoin_countdown_badge.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('daysLeftUntilRejoinExpiry', () {
    const day = 86400;

    test('full window remaining returns 30', () {
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000,
      );
      expect(result, 30);
    });

    test('one day after leaving returns 29', () {
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000 + day,
      );
      expect(result, 29);
    });

    test('exactly one day before expiry returns 1', () {
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000 + 29 * day,
      );
      expect(result, 1);
    });

    test('partial day before expiry still reads as 1 (ceiling)', () {
      // 29.5 days elapsed → 0.5 days left → ceiling to 1.
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000 + (29 * day + day ~/ 2),
      );
      expect(result, 1);
    });

    test('exactly at expiry boundary returns 0', () {
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000 + 30 * day,
      );
      expect(result, 0);
    });

    test('past the expiry boundary clamps to 0', () {
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1000,
        nowUnixSec: 1000 + 60 * day,
      );
      expect(result, 0);
    });

    test('clamps to windowDays when server clock leads client clock', () {
      // Real-world: server records leftAtUnixSec a fraction of a second
      // AFTER the client took its `now` reading. The naive ceiling math
      // would round up to 31; the clamp keeps it at 30. Models a 1-second
      // server-leads-client skew.
      final result = daysLeftUntilRejoinExpiry(
        leftAtUnixSec: 1001,
        nowUnixSec: 1000,
      );
      expect(result, 30);
    });
  });

  group('RejoinCountdownBadge', () {
    Future<void> pumpBadge(WidgetTester tester, int daysLeft) async {
      await tester.pumpWidget(localizedApp(
        Scaffold(body: Center(child: RejoinCountdownBadge(daysLeft: daysLeft))),
      ));
    }

    testWidgets('renders "30 days left to rejoin" at full window',
        (tester) async {
      await pumpBadge(tester, 30);
      expect(find.text('30 days left to rejoin'), findsOneWidget);
    });

    testWidgets('renders "15 days left to rejoin" mid-window', (tester) async {
      await pumpBadge(tester, 15);
      expect(find.text('15 days left to rejoin'), findsOneWidget);
    });

    testWidgets('renders "1 day left to rejoin" at 1 day left', (tester) async {
      await pumpBadge(tester, 1);
      expect(find.text('1 day left to rejoin'), findsOneWidget);
    });

    testWidgets('renders "Last day to rejoin" at 0 days left', (tester) async {
      await pumpBadge(tester, 0);
      expect(find.text('Last day to rejoin'), findsOneWidget);
    });

    testWidgets('wraps the badge in a Semantics with the visible label',
        (tester) async {
      await pumpBadge(tester, 5);

      // Per the avoid_color_only_status lint rule, the *Badge widget
      // must carry a Semantics annotation matching the visible
      // urgency cue. We assert the Semantics widget directly because
      // the badge intentionally does not exclude its child Text's
      // semantics — the merged node is what's expected.
      final semantics = tester.widgetList<Semantics>(find.byType(Semantics));
      final hasMatchingLabel = semantics.any(
        (s) => s.properties.label == '5 days left to rejoin',
      );
      expect(hasMatchingLabel, isTrue);
    });
  });
}
