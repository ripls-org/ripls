import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/toast_helper.dart';

void main() {
  // Wraps [child] in a minimal app with a Scaffold so ScaffoldMessenger works.
  Widget app(Widget child) => MaterialApp(home: Scaffold(body: child));

  // Duration used across tests — short enough to advance cheaply in tests.
  const testDuration = Duration(seconds: 4);

  group('ToastHelper.showUndo', () {
    testWidgets('shows snackbar with correct message and undo label',
        (tester) async {
      await tester.pumpWidget(app(Builder(
        builder: (context) => ElevatedButton(
          onPressed: () => ToastHelper.showUndo(
            context: context,
            message: 'Item removed',
            undoLabel: 'Undo',
            onUndo: () {},
            duration: testDuration,
          ),
          child: const Text('Trigger'),
        ),
      )));

      await tester.tap(find.text('Trigger'));
      await tester.pump();

      expect(find.text('Item removed'), findsOneWidget);
      expect(find.text('Undo'), findsOneWidget);

      // Drain the pending Future.delayed timer so the test framework is happy.
      await tester.pump(testDuration);
      await tester.pumpAndSettle();
    });

    testWidgets('calls onUndo when undo button is tapped', (tester) async {
      var undoCalled = false;

      await tester.pumpWidget(app(Builder(
        builder: (context) => ElevatedButton(
          onPressed: () => ToastHelper.showUndo(
            context: context,
            message: 'Item removed',
            undoLabel: 'Undo',
            onUndo: () => undoCalled = true,
            duration: testDuration,
          ),
          child: const Text('Trigger'),
        ),
      )));

      await tester.tap(find.text('Trigger'));
      await tester.pump(); // Enqueue SnackBar
      // Advance past the slide-in animation so the action button is hittable.
      await tester.pump(const Duration(milliseconds: 300));

      await tester.tap(find.text('Undo'));
      await tester.pump();

      expect(undoCalled, isTrue);

      // Drain the pending Future.delayed timer.
      await tester.pump(testDuration);
      await tester.pumpAndSettle();
    });

    testWidgets('dismisses automatically after duration elapses',
        (tester) async {
      await tester.pumpWidget(app(Builder(
        builder: (context) => ElevatedButton(
          onPressed: () => ToastHelper.showUndo(
            context: context,
            message: 'Item removed',
            undoLabel: 'Undo',
            onUndo: () {},
            duration: testDuration,
          ),
          child: const Text('Trigger'),
        ),
      )));

      await tester.tap(find.text('Trigger'));
      await tester.pump();

      expect(find.text('Item removed'), findsOneWidget);

      // Advance past the Future.delayed duration, then let the hide animation complete.
      await tester.pump(testDuration);
      await tester.pumpAndSettle();

      expect(find.text('Item removed'), findsNothing);
    });

    testWidgets('does not dismiss before duration elapses', (tester) async {
      await tester.pumpWidget(app(Builder(
        builder: (context) => ElevatedButton(
          onPressed: () => ToastHelper.showUndo(
            context: context,
            message: 'Item removed',
            undoLabel: 'Undo',
            onUndo: () {},
            duration: testDuration,
          ),
          child: const Text('Trigger'),
        ),
      )));

      await tester.tap(find.text('Trigger'));
      await tester.pump();

      // Only 3 seconds have passed — snackbar must still be visible.
      await tester.pump(const Duration(seconds: 3));
      expect(find.text('Item removed'), findsOneWidget);

      // Drain the remaining 1 second so the timer fires cleanly.
      await tester.pump(const Duration(seconds: 1));
      await tester.pumpAndSettle();
    });

    testWidgets('clears existing snack bars before showing new one',
        (tester) async {
      await tester.pumpWidget(app(Builder(
        builder: (context) => Column(
          children: [
            ElevatedButton(
              onPressed: () => ToastHelper.showUndo(
                context: context,
                message: 'First item removed',
                undoLabel: 'Undo',
                onUndo: () {},
                duration: testDuration,
              ),
              child: const Text('Dismiss first'),
            ),
            ElevatedButton(
              onPressed: () => ToastHelper.showUndo(
                context: context,
                message: 'Second item removed',
                undoLabel: 'Undo',
                onUndo: () {},
                duration: testDuration,
              ),
              child: const Text('Dismiss second'),
            ),
          ],
        ),
      )));

      await tester.tap(find.text('Dismiss first'));
      await tester.pump();
      expect(find.text('First item removed'), findsOneWidget);

      await tester.tap(find.text('Dismiss second'));
      await tester.pump();

      // Only the second snackbar should be visible.
      expect(find.text('Second item removed'), findsOneWidget);
      expect(find.text('First item removed'), findsNothing);

      // Drain both pending timers.
      await tester.pump(testDuration);
      await tester.pumpAndSettle();
    });

    testWidgets('uses custom duration when provided', (tester) async {
      const customDuration = Duration(seconds: 2);

      await tester.pumpWidget(app(Builder(
        builder: (context) => ElevatedButton(
          onPressed: () => ToastHelper.showUndo(
            context: context,
            message: 'Item removed',
            undoLabel: 'Undo',
            onUndo: () {},
            duration: customDuration,
          ),
          child: const Text('Trigger'),
        ),
      )));

      await tester.tap(find.text('Trigger'));
      await tester.pump();

      expect(find.text('Item removed'), findsOneWidget);

      await tester.pump(customDuration);
      await tester.pumpAndSettle();

      expect(find.text('Item removed'), findsNothing);
    });
  });

  group('toast text contrast (#2788)', () {
    // Originally: the toast filled itself with the status colour and picked
    // white or black text by `luminance > 0.5`, the usual heuristic. White and
    // black are equally legible at luminance 0.179, not 0.5, so every fill in
    // between took white when black was better — all four dark-theme status
    // colours. Warning measured 2.09:1.
    //
    // There is no status fill any more: a green success bar next to ~120 bare
    // `SnackBar`s on the Material default made one flow's toasts look like
    // three different components, so status moved to the icon and every toast
    // uses the theme's snack-bar surface. The label is therefore measured
    // against THAT surface, which is the thing it is now drawn on.
    //
    // Asserted through a real toast rather than any private helper, so it holds
    // whatever the implementation is, and in BOTH themes: the original bug
    // existed only in dark, so a light-only test would have passed throughout.
    double contrast(Color a, Color b) {
      final la = a.computeLuminance();
      final lb = b.computeLuminance();
      final hi = la > lb ? la : lb;
      final lo = la > lb ? lb : la;
      return (hi + 0.05) / (lo + 0.05);
    }

    Future<void> check(
      WidgetTester tester,
      Brightness brightness,
      void Function(BuildContext) show,
      String message,
    ) async {
      // The real app theme, because the surface the label is measured against
      // now comes from `snackBarTheme`. A bare `ThemeData(brightness:)` would
      // measure against Material's default and prove nothing about Ripls.
      final theme =
          brightness == Brightness.light ? AppTheme.lightTheme : AppTheme.darkTheme;
      await tester.pumpWidget(MaterialApp(
        theme: theme,
        home: Scaffold(
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => show(context),
              child: const Text('go'),
            ),
          ),
        ),
      ));
      await tester.tap(find.text('go'));
      await tester.pump();

      final text = tester.widget<Text>(find.text(message));
      final fg = text.style?.color;
      final bg = theme.snackBarTheme.backgroundColor;
      expect(fg, isNotNull, reason: 'toast text must set an explicit colour');
      expect(bg, isNotNull,
          reason: 'the theme must define a snack-bar surface, or bare '
              'SnackBars fall back to the Material default');

      final ratio = contrast(fg!, bg!);
      expect(
        ratio,
        greaterThanOrEqualTo(4.5),
        reason: '$brightness toast "$message" measured '
            '${ratio.toStringAsFixed(2)}:1 on the snack-bar surface',
      );
      await tester.pumpAndSettle(const Duration(seconds: 5));
    }

    for (final brightness in [Brightness.light, Brightness.dark]) {
      testWidgets('success clears 4.5:1 ($brightness)', (t) async {
        await check(t, brightness, (c) => ToastHelper.showSuccess(c, 'saved'),
            'saved');
      });
      testWidgets('warning clears 4.5:1 ($brightness)', (t) async {
        await check(t, brightness, (c) => ToastHelper.showWarning(c, 'careful'),
            'careful');
      });
      testWidgets('error clears 4.5:1 ($brightness)', (t) async {
        await check(t, brightness, (c) => ToastHelper.showError(c, 'failed'),
            'failed');
      });
      testWidgets('info clears 4.5:1 ($brightness)', (t) async {
        await check(t, brightness, (c) => ToastHelper.showInfo(c, 'heads up'),
            'heads up');
      });
    }
  });
}
