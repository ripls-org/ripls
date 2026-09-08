import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal.dart';

import '../../../../helpers/l10n_helpers.dart';

/// Smoke widget tests that actually pump [DateTimePickerModal]. These cover
/// the entire failure class that pure ViewModel tests miss: provider build
/// crashes, layout overflows, and Semantics-tree invariant violations.
void main() {
  testWidgets('opens, renders, and pops with null on Cancel', (tester) async {
    await tester.binding.setSurfaceSize(const Size(390, 844)); // iPhone 13
    addTearDown(() => tester.binding.setSurfaceSize(null));

    DateTimePickerResult? result;
    bool completed = false;

    await tester.pumpWidget(ProviderScope(
      child: localizedApp(Builder(builder: (context) {
        return Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () async {
                result = await DateTimePickerModal.show(
                  context,
                  initialDateTime: DateTime(2026, 5, 16, 9, 0),
                  firstDate: DateTime(2026, 5, 1),
                  lastDate: DateTime(2026, 12, 31),
                );
                completed = true;
              },
              child: const Text('open'),
            ),
          ),
        );
      })),
    ));

    await tester.tap(find.text('open'));
    // Two pumps: open the modal route, then run the post-frame initialize +
    // setState. The exception in production fired during this window — the
    // ViewModel test never exercised it because it doesn't pump a widget.
    await tester.pumpAndSettle();

    // Hero renders (proves build / semantics / layout succeeded).
    expect(find.text('Sat, May 16'), findsOneWidget);

    // Cancel by tapping the modal barrier (the system scrim outside the
    // sheet). Use a tap at the very top of the screen.
    await tester.tapAt(const Offset(10, 10));
    await tester.pumpAndSettle();

    expect(completed, isTrue);
    expect(result, isNull);
  });

  testWidgets('Edit mode shows Remove and returns removed variant',
      (tester) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    DateTimePickerResult? result;

    await tester.pumpWidget(ProviderScope(
      child: localizedApp(Builder(builder: (context) {
        return Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () async {
                result = await DateTimePickerModal.show(
                  context,
                  initialDateTime: DateTime(2026, 5, 16, 9, 0),
                  firstDate: DateTime(2026, 5, 1),
                  lastDate: DateTime(2026, 12, 31),
                  allowRemove: true,
                );
              },
              child: const Text('open'),
            ),
          ),
        );
      })),
    ));

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    expect(find.text('Remove'), findsOneWidget);

    await tester.tap(find.text('Remove'));
    await tester.pumpAndSettle();

    expect(result, isA<DateTimePickerResultRemoved>());
  });

  testWidgets('Save returns the initialized DateTime', (tester) async {
    await tester.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    DateTimePickerResult? result;
    final initial = DateTime(2026, 5, 16, 9, 0);

    await tester.pumpWidget(ProviderScope(
      child: localizedApp(Builder(builder: (context) {
        return Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () async {
                result = await DateTimePickerModal.show(
                  context,
                  initialDateTime: initial,
                  firstDate: DateTime(2026, 5, 1),
                  lastDate: DateTime(2026, 12, 31),
                );
              },
              child: const Text('open'),
            ),
          ),
        );
      })),
    ));

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(result, isA<DateTimePickerResultSaved>());
    expect((result! as DateTimePickerResultSaved).value, initial);
  });

  testWidgets('renders on a small phone surface (catches layout overflow)',
      (tester) async {
    // iPhone SE-ish — smaller than what triggered the 34px overflow in
    // production. If the layout doesn't fit, this test reports the overflow
    // via tester.takeException().
    await tester.binding.setSurfaceSize(const Size(375, 667));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(ProviderScope(
      child: localizedApp(Builder(builder: (context) {
        return Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () => DateTimePickerModal.show(
                context,
                initialDateTime: DateTime(2026, 5, 16, 9, 0),
                firstDate: DateTime(2026, 5, 1),
                lastDate: DateTime(2026, 12, 31),
              ),
              child: const Text('open'),
            ),
          ),
        );
      })),
    ));

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    expect(tester.takeException(), isNull);
    expect(find.text('Sat, May 16'), findsOneWidget);
  });

}
