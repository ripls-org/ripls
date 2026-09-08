import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/create/unified_item_details_sheet.dart';

Widget _harness(Widget Function(BuildContext) builder) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(body: Builder(builder: builder)),
  );
}

void main() {
  group('UnifiedItemDetailsSheet', () {
    testWidgets('renders all 5 fields with sheet title + Done button',
        (tester) async {
      await tester.pumpWidget(
        _harness((context) => ElevatedButton(
              onPressed: () => UnifiedItemDetailsSheet.show(context),
              child: const Text('Open'),
            )),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Item details'), findsOneWidget);
      expect(find.text('Brand'), findsOneWidget);
      expect(find.text('Model'), findsOneWidget);
      expect(find.text('Est. Value'), findsOneWidget);
      expect(find.text('Material'), findsOneWidget);
      expect(find.text('Weight'), findsOneWidget);
      expect(find.text('Done'), findsOneWidget);
    });

    testWidgets('initial value pre-fills fields', (tester) async {
      const initial = ItemDetailsValue(
        brand: 'Edelrid',
        model: 'BOA ECO 2R',
        estValueUsd: '\$185',
        material: 'Nylon',
        weight: '2.1 kg',
      );
      await tester.pumpWidget(
        _harness((context) => ElevatedButton(
              onPressed: () =>
                  UnifiedItemDetailsSheet.show(context, initial: initial),
              child: const Text('Open'),
            )),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      expect(find.text('Edelrid'), findsOneWidget);
      expect(find.text('BOA ECO 2R'), findsOneWidget);
      expect(find.text('\$185'), findsOneWidget);
      expect(find.text('Nylon'), findsOneWidget);
      expect(find.text('2.1 kg'), findsOneWidget);
    });

    testWidgets('Done button returns ItemDetailsValue with field values',
        (tester) async {
      ItemDetailsValue? returned;
      await tester.pumpWidget(
        _harness((context) => ElevatedButton(
              onPressed: () async {
                returned = await UnifiedItemDetailsSheet.show(context);
              },
              child: const Text('Open'),
            )),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Type a brand. The Brand row's TextField is the one with the
      // 'Brand' label sibling. Use enterText against the first
      // unlabelled TextField in row order (Brand is row 1).
      final fields = find.byType(TextField);
      expect(fields, findsNWidgets(5));
      await tester.enterText(fields.at(0), 'Acme');
      await tester.enterText(fields.at(1), 'X100');

      await tester.tap(find.text('Done'));
      await tester.pumpAndSettle();

      expect(returned, isNotNull);
      expect(returned!.brand, 'Acme');
      expect(returned!.model, 'X100');
    });
  });
}
