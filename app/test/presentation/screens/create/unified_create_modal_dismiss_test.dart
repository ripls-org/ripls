import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

// #1980 — tests for the new dismiss behavior on UnifiedCreateModal:
// in non-camera states the X is gone and a Tappable barrier dismisses
// the modal on tap. Camera mode keeps the X and is covered by manual
// QA + code review since the live-camera plugin can't be mocked in
// widget tests.

class _FakeVm extends UnifiedCreateViewModel {
  _FakeVm(this._initial);
  final UnifiedCreateState _initial;

  @override
  UnifiedCreateState build() {
    super.build();
    return _initial;
  }
}

Widget _appHarness({required UnifiedCreateState state}) {
  return ProviderScope(
    overrides: [
      unifiedCreateViewModelProvider.overrideWith(() => _FakeVm(state)),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Builder(
        builder: (ctx) => Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () => showAccessibleModal<bool>(
                ctx,
                isScrollControlled: true,
                useSafeArea: false,
                backgroundColor: Colors.transparent,
                builder: (_) => const UnifiedCreateModal(),
              ),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    ),
  );
}

Future<void> _openModal(WidgetTester tester) async {
  await tester.tap(find.text('open'));
  await tester.pumpAndSettle();
}

void main() {
  group('UnifiedCreateModal dismiss behavior (#1980)', () {
    testWidgets(
        'text mode: no X button, Tappable dismiss zone is present with the Dismiss semantics label',
        (tester) async {
      await tester.pumpWidget(
        _appHarness(
          state: const UnifiedCreateState(inputMode: CreateInputMode.text),
        ),
      );
      await _openModal(tester);

      // The X is gone in non-camera states.
      expect(find.byIcon(Icons.close), findsNothing);

      // The Tappable dismiss zone is present and labeled correctly.
      final dismissZone = find.byWidgetPredicate(
        (w) => w is Tappable && w.semanticsLabel == 'Dismiss',
      );
      expect(dismissZone, findsOneWidget);
    });

    testWidgets('url mode: same — no X, dismiss zone present', (tester) async {
      await tester.pumpWidget(
        _appHarness(
          state: const UnifiedCreateState(inputMode: CreateInputMode.url),
        ),
      );
      await _openModal(tester);

      expect(find.byIcon(Icons.close), findsNothing);
      final dismissZone = find.byWidgetPredicate(
        (w) => w is Tappable && w.semanticsLabel == 'Dismiss',
      );
      expect(dismissZone, findsOneWidget);
    });

    testWidgets(
        'preview stage without media: no X, dismiss zone present',
        (tester) async {
      await tester.pumpWidget(
        _appHarness(
          // Preview stage but no mediaIds, so we hit the BackdropFilter
          // branch instead of _HeroMediaBackground (which would need
          // mediaRepositoryProvider).
          state: const UnifiedCreateState(stage: CreateStage.preview),
        ),
      );
      await _openModal(tester);

      expect(find.byIcon(Icons.close), findsNothing);
      final dismissZone = find.byWidgetPredicate(
        (w) => w is Tappable && w.semanticsLabel == 'Dismiss',
      );
      expect(dismissZone, findsOneWidget);
    });

    testWidgets(
        'text mode: tapping the dismiss zone pops the modal',
        (tester) async {
      await tester.pumpWidget(
        _appHarness(
          state: const UnifiedCreateState(inputMode: CreateInputMode.text),
        ),
      );
      await _openModal(tester);

      // Sanity: modal is mounted.
      final dismissZone = find.byWidgetPredicate(
        (w) => w is Tappable && w.semanticsLabel == 'Dismiss',
      );
      expect(dismissZone, findsOneWidget);

      // Tap a point near the top-center of the screen, well above
      // the bottom drawer, to ensure the hit lands on the dismiss
      // zone and not on the drawer's tab strip.
      final screen = tester.getSize(find.byType(MaterialApp));
      await tester.tapAt(Offset(screen.width / 2, 80));
      await tester.pumpAndSettle();

      // Modal is gone — the launcher button is back in view, and the
      // dismiss zone is no longer in the tree.
      expect(find.text('open'), findsOneWidget);
      expect(
        find.byWidgetPredicate(
          (w) => w is Tappable && w.semanticsLabel == 'Dismiss',
        ),
        findsNothing,
      );
    });
  });
}
