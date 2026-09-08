import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/need_quantity_badge.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  const style = TextStyle(fontSize: 14);

  group('NeedQuantityBadge', () {
    testWidgets('renders nothing at quantity 1', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(body: NeedQuantityBadge(quantity: 1, style: style)),
        ),
      );
      expect(find.textContaining('×'), findsNothing);
    });

    testWidgets('renders nothing at quantity 0', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(body: NeedQuantityBadge(quantity: 0, style: style)),
        ),
      );
      expect(find.textContaining('×'), findsNothing);
    });

    testWidgets('renders ×N glyph at quantity > 1', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(body: NeedQuantityBadge(quantity: 4, style: style)),
        ),
      );
      expect(find.text('×4'), findsOneWidget);
    });

    testWidgets('exposes a localized quantity semantics label, not the glyph', (
      tester,
    ) async {
      await tester.pumpWidget(
        localizedApp(
          const Scaffold(body: NeedQuantityBadge(quantity: 2, style: style)),
        ),
      );
      // The raw "×2" glyph is excluded from the semantics tree; the
      // screen-reader hears "quantity 2" instead.
      expect(find.bySemanticsLabel('quantity 2'), findsOneWidget);
      expect(find.bySemanticsLabel('×2'), findsNothing);
    });
  });
}
