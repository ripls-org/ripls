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
  // These tests verify the CloseItemModal row states that the gear close
  // handlers are expected to produce. Each scenario maps to a call site in
  // GearMenuItems:
  //   - giveaway recipient-selected (line 126): all three enabled
  //   - giveaway pre-recipient    (line 163): cancel disabled, unshare + delete enabled
  //   - loan with active transfer (line 381): all three enabled
  //   - loan with no active transfer:         cancel disabled, unshare + delete enabled

  group('CloseItemModal — giveaway recipient-selected (all three enabled)',
      () {
    testWidgets('all three rows enabled when cancel and unshare callbacks set',
        (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.item,
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
  });

  group('CloseItemModal — giveaway pre-recipient (cancel disabled)', () {
    testWidgets(
        'cancel row disabled, unshare and delete enabled '
        'when no active transfer', (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.item,
          onUnshare: () {},
          onCancel: null,
          onDelete: () {},
        ),
      ));
      await tester.pump();

      final tiles = _optionRows(tester);
      expect(tiles.length, 3);
      expect(tiles[0].onTap, isNotNull,
          reason: 'unshare row should be enabled');
      expect(tiles[1].onTap, isNull,
          reason: 'cancel row should be disabled when no active transfer');
      expect(tiles[2].onTap, isNotNull,
          reason: 'delete row should be enabled');
    });
  });

  group('CloseItemModal — loan with active transfer (all three enabled)', () {
    testWidgets('all three rows enabled', (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.loan,
          onUnshare: () {},
          onCancel: () {},
          onDelete: () {},
        ),
      ));
      await tester.pump();

      final tiles = _optionRows(tester);
      expect(tiles.length, 3);
      expect(tiles[0].onTap, isNotNull);
      expect(tiles[1].onTap, isNotNull);
      expect(tiles[2].onTap, isNotNull);
    });
  });

  group('CloseItemModal — loan without active transfer (cancel disabled)', () {
    testWidgets('cancel row disabled when no active transfer', (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.loan,
          onUnshare: () {},
          onCancel: null,
          onDelete: () {},
        ),
      ));
      await tester.pump();

      final tiles = _optionRows(tester);
      expect(tiles.length, 3);
      expect(tiles[0].onTap, isNotNull);
      expect(tiles[1].onTap, isNull,
          reason: 'cancel row should be disabled when no active transfer');
      expect(tiles[2].onTap, isNotNull);
    });
  });

  group('CloseItemModal — no community context (unshare disabled)', () {
    testWidgets('unshare row disabled when no communityId', (tester) async {
      await tester.pumpWidget(_buildApp(
        CloseItemModal(
          contentType: CloseItemContentType.item,
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
      expect(tiles[1].onTap, isNotNull);
      expect(tiles[2].onTap, isNotNull);
    });
  });
}
