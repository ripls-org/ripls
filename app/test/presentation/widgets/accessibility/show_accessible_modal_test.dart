import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';

void main() {
  group('showAccessibleModal', () {
    testWidgets('restores focus to the explicit returnFocusTo node on dismiss',
        (tester) async {
      final focusNode = FocusNode(debugLabel: 'caller');
      addTearDown(focusNode.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(builder: (context) {
            return Column(children: [
              Focus(focusNode: focusNode, child: const SizedBox(width: 1, height: 1)),
              ElevatedButton(
                onPressed: () => showAccessibleModal<void>(
                  context,
                  returnFocusTo: focusNode,
                  builder: (_) => const Text('sheet'),
                ),
                child: const Text('open'),
              ),
            ]);
          }),
        ),
      ));

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();
      expect(find.text('sheet'), findsOneWidget);

      // Dismiss by tapping the barrier.
      await tester.tapAt(const Offset(20, 20));
      await tester.pumpAndSettle();

      expect(focusNode.hasPrimaryFocus, isTrue);
    });

    testWidgets('returns the modal result', (tester) async {
      Object? captured = 'unset';

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Builder(builder: (context) {
            return ElevatedButton(
              onPressed: () async {
                captured = await showAccessibleModal<String>(
                  context,
                  builder: (sheetContext) => ElevatedButton(
                    onPressed: () => Navigator.of(sheetContext).pop('chosen'),
                    child: const Text('pick'),
                  ),
                );
              },
              child: const Text('open'),
            );
          }),
        ),
      ));

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('pick'));
      await tester.pumpAndSettle();

      expect(captured, 'chosen');
    });

    group('sheet width cap (#2912)', () {
      const sheetKey = Key('measured-sheet');

      Future<void> openSheet(WidgetTester tester, Size surface) async {
        tester.view.physicalSize = surface;
        tester.view.devicePixelRatio = 1.0;
        addTearDown(tester.view.reset);

        await tester.pumpWidget(MaterialApp(
          home: Scaffold(
            body: Builder(builder: (context) {
              return ElevatedButton(
                onPressed: () => showAccessibleModal<void>(
                  context,
                  builder: (_) => const SizedBox(
                    key: sheetKey,
                    height: 200,
                    width: double.infinity,
                  ),
                ),
                child: const Text('open'),
              );
            }),
          ),
        ));
        await tester.tap(find.text('open'));
        await tester.pumpAndSettle();
      }

      testWidgets('caps and centers the sheet on a desktop-wide window',
          (tester) async {
        const surface = Size(1440, 810);
        await openSheet(tester, surface);

        final rect = tester.getRect(find.byKey(sheetKey));
        expect(rect.width, lessThanOrEqualTo(Responsive.sheetMaxWidth));
        expect(rect.center.dx, moreOrLessEquals(surface.width / 2, epsilon: 1));
      });

      testWidgets('is a no-op at a phone width', (tester) async {
        const surface = Size(390, 844);
        await openSheet(tester, surface);

        // Narrower than the cap, so the sheet keeps the full window width it
        // has always had.
        final rect = tester.getRect(find.byKey(sheetKey));
        expect(rect.width, surface.width);
      });

      testWidgets('caller-supplied constraints win over the default',
          (tester) async {
        tester.view.physicalSize = const Size(1440, 810);
        tester.view.devicePixelRatio = 1.0;
        addTearDown(tester.view.reset);

        await tester.pumpWidget(MaterialApp(
          home: Scaffold(
            body: Builder(builder: (context) {
              return ElevatedButton(
                onPressed: () => showAccessibleModal<void>(
                  context,
                  constraints: const BoxConstraints(maxWidth: 300),
                  builder: (_) => const SizedBox(
                    key: sheetKey,
                    height: 200,
                    width: double.infinity,
                  ),
                ),
                child: const Text('open'),
              );
            }),
          ),
        ));
        await tester.tap(find.text('open'));
        await tester.pumpAndSettle();

        expect(tester.getRect(find.byKey(sheetKey)).width, 300);
      });
    });
  });
}
