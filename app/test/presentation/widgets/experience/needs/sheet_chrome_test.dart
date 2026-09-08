import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';

void main() {
  group('resolveBatchSheetPadding', () {
    test('no keyboard returns base bottom padding unchanged', () {
      final padding = resolveBatchSheetPadding(
        horizontal: kSheetHPadding,
        top: kSheetTopPadding,
        base: kSheetBottomPadding,
        keyboardInset: 0,
      );

      expect(padding.left, kSheetHPadding);
      expect(padding.right, kSheetHPadding);
      expect(padding.top, kSheetTopPadding);
      expect(padding.bottom, kSheetBottomPadding);
    });

    test('keyboard inset stacks on top of base bottom padding', () {
      final padding = resolveBatchSheetPadding(
        horizontal: kSheetHPadding,
        top: kSheetTopPadding,
        base: kSheetBottomPadding,
        keyboardInset: 336,
      );

      expect(padding.left, kSheetHPadding);
      expect(padding.right, kSheetHPadding);
      expect(padding.top, kSheetTopPadding);
      expect(padding.bottom, kSheetBottomPadding + 336);
    });

    test('home-indicator breathing room is preserved when keyboard is down',
        () {
      final padding = resolveBatchSheetPadding(
        horizontal: 24,
        top: 4,
        base: 16,
        keyboardInset: 0,
      );

      expect(padding, const EdgeInsets.fromLTRB(24, 4, 24, 16));
    });

    test('horizontal and top are unaffected by keyboard inset', () {
      final padding = resolveBatchSheetPadding(
        horizontal: 24,
        top: 4,
        base: 16,
        keyboardInset: 100,
      );

      expect(padding.left, 24);
      expect(padding.right, 24);
      expect(padding.top, 4);
      expect(padding.bottom, 116);
    });
  });

  // Regression for #2004: under a dark theme whose inputDecorationTheme has
  // `filled: true` with a dark fillColor, the modal sheet inputs must opt
  // out so the cream pill isn't painted over with a "black box".
  group('cream pill inputs ignore theme fill', () {
    Future<TextField> pumpAndFind(
      WidgetTester tester,
      Widget Function(BuildContext) builder,
    ) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: ThemeData(
            brightness: Brightness.dark,
            inputDecorationTheme: const InputDecorationTheme(
              filled: true,
              fillColor: AppColors.darkSurface,
            ),
          ),
          home: Scaffold(
            body: Builder(builder: builder),
          ),
        ),
      );
      return tester.widget<TextField>(find.byType(TextField));
    }

    testWidgets('buildInputField sets filled: false', (tester) async {
      final field = await pumpAndFind(
        tester,
        (context) => buildInputField(
          context: context,
          controller: TextEditingController(),
          accent: AppColors.transferCoral,
          hint: 'hint',
          onChanged: () {},
        ),
      );
      expect(field.decoration?.filled, isFalse);
    });

    testWidgets('buildNoteInputField sets filled: false', (tester) async {
      final field = await pumpAndFind(
        tester,
        (context) => buildNoteInputField(
          context: context,
          controller: TextEditingController(),
          hint: 'hint',
        ),
      );
      expect(field.decoration?.filled, isFalse);
    });
  });
}
