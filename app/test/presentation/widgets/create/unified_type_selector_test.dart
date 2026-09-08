import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/create/unified_type_selector.dart';

Widget _wrap(Widget child, ProviderContainer container) {
  return UncontrolledProviderScope(
    container: container,
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: child),
    ),
  );
}

void main() {
  group('UnifiedTypeSelector', () {
    testWidgets('renders three chips', (tester) async {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      await tester.pumpWidget(_wrap(const UnifiedTypeSelector(), container));
      expect(find.text('Request'), findsOneWidget);
      expect(find.text('Event'), findsOneWidget);
      expect(find.text('Item'), findsOneWidget);
    });

    testWidgets('selector is gated by selectorEnabled flag', (tester) async {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      await tester.pumpWidget(_wrap(const UnifiedTypeSelector(), container));
      // Default state has selectorEnabled=false, which the selector
      // surfaces as an AbsorbPointer that swallows taps.
      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.selectorEnabled, false);
      // The selector wraps its pill row in an AbsorbPointer that
      // toggles based on selectorEnabled; locate the one whose ancestor
      // is the UnifiedTypeSelector to avoid matching unrelated
      // AbsorbPointers from Material internals.
      final absorberFinder = find.descendant(
        of: find.byType(UnifiedTypeSelector),
        matching: find.byType(AbsorbPointer),
      );
      final absorber = tester.widget<AbsorbPointer>(absorberFinder.first);
      expect(absorber.absorbing, isTrue);
    });
  });
}
