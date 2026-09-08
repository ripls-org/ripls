import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/unified_create_input_drawer.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';

class _FakeVm extends UnifiedCreateViewModel {
  _FakeVm(this._initial);
  final UnifiedCreateState _initial;

  @override
  UnifiedCreateState build() => _initial;
}

Widget _harness(UnifiedCreateState state) {
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
      home: const Scaffold(body: UnifiedCreateInputDrawer()),
    ),
  );
}

void main() {
  group('UnifiedCreateInputDrawer', () {
    testWidgets('all three tab labels render', (tester) async {
      await tester.pumpWidget(
        _harness(const UnifiedCreateState(inputMode: CreateInputMode.text)),
      );
      expect(find.text('Text'), findsOneWidget);
      expect(find.text('Image'), findsOneWidget);
      expect(find.text('Link'), findsOneWidget);
    });

    testWidgets('Text tab renders a textarea + Draft it button',
        (tester) async {
      await tester.pumpWidget(
        _harness(const UnifiedCreateState(inputMode: CreateInputMode.text)),
      );
      expect(find.byType(TextField), findsOneWidget);
      expect(find.text('Draft it'), findsOneWidget);
    });

    testWidgets(
        'Image tab renders no panel content — drawer collapses to tab strip',
        (tester) async {
      // The live camera + coaching carousel + shutter + gallery
      // button live in UnifiedCreateCameraLayer (rendered behind the
      // drawer by UnifiedCreateModal), NOT inside the drawer itself.
      // The drawer's job in Image mode is simply to expose the tab
      // strip so the user can switch back to Text / URL.
      await tester.pumpWidget(
        _harness(const UnifiedCreateState(inputMode: CreateInputMode.image)),
      );
      expect(find.text('Image'), findsOneWidget);
      // No textarea, no Draft it button, no Camera / Gallery action chips.
      expect(find.byType(TextField), findsNothing);
      expect(find.text('Draft it'), findsNothing);
      expect(find.text('Camera'), findsNothing);
      expect(find.text('Gallery'), findsNothing);
    });

    testWidgets('URL tab shows a URL input', (tester) async {
      await tester.pumpWidget(
        _harness(const UnifiedCreateState(inputMode: CreateInputMode.url)),
      );
      // One TextField (the URL input).
      expect(find.byType(TextField), findsOneWidget);
      // Link icon as the prefix.
      expect(find.byIcon(Icons.link), findsOneWidget);
    });

    testWidgets('URL field never autocapitalizes (URLs are case-sensitive)',
        (tester) async {
      // Regression test for #1990. The Text tab gets
      // TextCapitalization.sentences for sentence-style auto-cap; the URL
      // tab MUST NOT, because hostnames and paths are case-sensitive and
      // silently uppercasing the first letter corrupts the URL.
      await tester.pumpWidget(
        _harness(const UnifiedCreateState(inputMode: CreateInputMode.url)),
      );
      final field = tester.widget<TextField>(find.byType(TextField));
      expect(field.textCapitalization, isNot(TextCapitalization.sentences));
    });

    testWidgets('Draft it button is disabled while prompt is empty',
        (tester) async {
      await tester.pumpWidget(_harness(
        const UnifiedCreateState(inputMode: CreateInputMode.text),
      ));
      final inkWell = tester.widget<InkWell>(
        find
            .ancestor(of: find.text('Draft it'), matching: find.byType(InkWell))
            .first,
      );
      expect(inkWell.onTap, isNull);
    });

    testWidgets('Draft it button is enabled when prompt has content',
        (tester) async {
      await tester.pumpWidget(_harness(
        const UnifiedCreateState(
            inputMode: CreateInputMode.text, prompt: 'hike'),
      ));
      final inkWell = tester.widget<InkWell>(
        find
            .ancestor(of: find.text('Draft it'), matching: find.byType(InkWell))
            .first,
      );
      expect(inkWell.onTap, isNotNull);
    });
  });
}
