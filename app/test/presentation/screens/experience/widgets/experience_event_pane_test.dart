import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';

/// Builds a localized test app wrapping [child].
Widget _buildApp(Widget child) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(body: child),
  );
}

/// Returns the three CloseItemModal option Tappables in [unshare, cancel,
/// delete] order, filtering out the GlassSheet's backdrop Tappable.
List<Tappable> _optionRows(WidgetTester tester) {
  const optionLabels = {
    'Unshare from this community',
    'Mark as Cancelled',
    'Delete Permanently',
  };
  return tester
      .widgetList<Tappable>(find.byType(Tappable))
      .where((t) => optionLabels.contains(t.semanticsLabel))
      .toList();
}

void main() {
  group('CloseItemModal — event callback wiring', () {
    testWidgets('all three rows enabled when all callbacks are non-null',
        (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.event,
          onUnshare: () {},
          onCancel: () {},
          onDelete: () {},
        ),
      ));
      await tester.pump();

      final tiles = _optionRows(tester);
      expect(tiles.length, 3);
      expect(tiles[0].onTap, isNotNull,
          reason: 'unshare row should be enabled');
      expect(tiles[1].onTap, isNotNull,
          reason: 'cancel row should be enabled');
      expect(tiles[2].onTap, isNotNull,
          reason: 'delete row should be enabled');
    });

    testWidgets('unshare row disabled when onUnshare is null (no communityId)',
        (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.event,
          onUnshare: null,
          onCancel: () {},
          onDelete: () {},
        ),
      ));
      await tester.pump();

      final tiles = _optionRows(tester);
      expect(tiles.length, 3);
      expect(tiles[0].onTap, isNull,
          reason: 'unshare row should be disabled when communityId is null');
      expect(tiles[1].onTap, isNotNull,
          reason: 'cancel row should remain enabled');
      expect(tiles[2].onTap, isNotNull,
          reason: 'delete row should remain enabled');
    });
  });
}
