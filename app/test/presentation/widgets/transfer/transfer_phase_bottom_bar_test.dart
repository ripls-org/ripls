import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_phase_bottom_bar.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(
      backgroundColor: Colors.black,
      body: SafeArea(child: child),
    ),
  );
}

/// Walks the [tester]'s widget tree to find the first [Container] whose
/// [BoxDecoration] fill matches [color]. Returns null when nothing matches.
Container? _findContainerWithColor(WidgetTester tester, Color color) {
  for (final element in tester.allWidgets.whereType<Container>()) {
    final decoration = element.decoration;
    if (decoration is BoxDecoration && decoration.color == color) {
      return element;
    }
  }
  return null;
}

void main() {
  group('TransferPhaseBottomBar', () {
    testWidgets('renders primary button label', (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
        )),
      );
      expect(find.text('Confirm'), findsOneWidget);
    });

    testWidgets('renders secondary button when label + callback provided',
        (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
          secondaryLabel: 'Cancel',
          onSecondaryPressed: () {},
        )),
      );
      expect(find.text('Cancel'), findsOneWidget);
      expect(find.text('Confirm'), findsOneWidget);
    });

    testWidgets('omits secondary button when only label provided (no callback)',
        (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
          secondaryLabel: 'Cancel',
        )),
      );
      expect(find.text('Cancel'), findsNothing);
      expect(find.text('Confirm'), findsOneWidget);
    });

    testWidgets('fires primary callback on tap', (tester) async {
      var primaryCount = 0;
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Save',
          onPrimaryPressed: () => primaryCount += 1,
        )),
      );
      await tester.tap(find.text('Save'));
      await tester.pump();
      expect(primaryCount, 1);
    });

    testWidgets('fires secondary callback on tap', (tester) async {
      var secondaryCount = 0;
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Save',
          onPrimaryPressed: () {},
          secondaryLabel: 'Discard',
          onSecondaryPressed: () => secondaryCount += 1,
        )),
      );
      await tester.tap(find.text('Discard'));
      await tester.pump();
      expect(secondaryCount, 1);
    });

    testWidgets('renders leading widget when provided', (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          leadingWidget: const Icon(Icons.chat_bubble_outline,
              key: Key('chat-icon')),
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
        )),
      );
      expect(find.byKey(const Key('chat-icon')), findsOneWidget);
    });

    testWidgets('shows progress indicator + suppresses callbacks when loading',
        (tester) async {
      var primaryCount = 0;
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () => primaryCount += 1,
          isLoading: true,
        )),
      );
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      await tester.tap(find.byType(CircularProgressIndicator));
      await tester.pump();
      expect(primaryCount, 0,
          reason: 'loading state must disable the primary tap');
    });

    // Regression: the primary button uses transferSage as the default
    // semantic accent and transferCoral when destructive. These are the
    // only legacy transfer tokens preserved through the glass migration —
    // anyone removing them flips the button to gray and breaks the
    // success/destructive distinction.
    testWidgets('default primary uses transferSage', (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
        )),
      );
      expect(_findContainerWithColor(tester, AppColors.transferSage),
          isNotNull,
          reason: 'default primary should fill with transferSage');
      expect(_findContainerWithColor(tester, AppColors.transferCoral),
          isNull,
          reason: 'default primary should not use transferCoral');
    });

    testWidgets('destructive primary uses transferCoral', (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Cancel transfer',
          onPrimaryPressed: () {},
          isPrimaryDestructive: true,
        )),
      );
      expect(_findContainerWithColor(tester, AppColors.transferCoral),
          isNotNull,
          reason: 'destructive primary should fill with transferCoral');
    });

    // Regression: the divider above the buttons must use modalFooterDivider
    // (white @ 15% alpha) rather than the legacy cream border. This tracks
    // the glass migration of the bar's surrounding chrome.
    testWidgets('hairline divider uses modalFooterDivider', (tester) async {
      await tester.pumpWidget(
        _wrap(TransferPhaseBottomBar(
          primaryLabel: 'Confirm',
          onPrimaryPressed: () {},
        )),
      );
      // The divider is a Container with color: modalFooterDivider — find it
      // by exact-color match (glass tokens are const so equality is safe).
      final divider = tester.allWidgets
          .whereType<Container>()
          .where((c) => c.color == AppColors.modalFooterDivider)
          .toList();
      expect(divider, isNotEmpty,
          reason: 'glass migration requires the modalFooterDivider hairline');
    });
  });
}
