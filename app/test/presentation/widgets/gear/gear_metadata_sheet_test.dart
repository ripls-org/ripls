import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show Estimate, MaterialCategory, TrackedEstimate, TrackedMaterialCategory, TrackedString;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem, GearMetadata;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/gear/gear_metadata_sheet.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(body: child));
}

Future<void> _showSheet(
  WidgetTester tester, {
  GearMetadata? metadata,
  ValueEstimate? valueEstimate,
  DetectedGearItem? detectedGear,
  bool readOnly = false,
  String? sourceUrl,
  Future<DetectedGearItem?> Function(String url)? onAutoFill,
}) async {
  await tester.pumpWidget(
    _wrap(
      Builder(
        builder: (ctx) => ElevatedButton(
          onPressed: () => GearMetadataSheet.show(
            ctx,
            metadata: metadata,
            valueEstimate: valueEstimate,
            detectedGear: detectedGear,
            readOnly: readOnly,
            sourceUrl: sourceUrl,
            onAutoFill: onAutoFill,
          ),
          child: const Text('Open'),
        ),
      ),
    ),
  );
  await tester.tap(find.text('Open'));
  await tester.pumpAndSettle();
}

void main() {
  // ── Brand section ────────────────────────────────────────────────────────────

  group('Brand section — read-only', () {
    testWidgets('shows brand and model text', (tester) async {
      final metadata = GearMetadata(
        brand: TrackedString(value: 'Ryobi'),
        model: TrackedString(value: 'RY142300'),
      );
      await _showSheet(tester, metadata: metadata, readOnly: true);

      expect(find.text('Ryobi'), findsOneWidget);
      expect(find.text('RY142300'), findsOneWidget);
    });

    testWidgets('shows -- when brand is empty', (tester) async {
      await _showSheet(tester, readOnly: true);
      expect(find.text('--'), findsWidgets);
    });

    testWidgets('hides model row when model is empty', (tester) async {
      final metadata = GearMetadata(brand: TrackedString(value: 'Ryobi'));
      await _showSheet(tester, metadata: metadata, readOnly: true);
      // Model field should not appear
      expect(find.text('RY142300'), findsNothing);
    });

    testWidgets('shows category pill when category is set', (tester) async {
      final metadata = GearMetadata(
        brand: TrackedString(value: 'Ryobi'),
        category: TrackedString(value: 'Power Tools'),
      );
      await _showSheet(tester, metadata: metadata, readOnly: true);
      expect(find.text('Power Tools'), findsOneWidget);
    });

    testWidgets('shows material pill when material is set', (tester) async {
      final metadata = GearMetadata(
        brand: TrackedString(value: 'Ryobi'),
        materialCategory: TrackedMaterialCategory(
          value: MaterialCategory.MATERIAL_CATEGORY_CORDED_POWER_TOOL,
        ),
      );
      await _showSheet(tester, metadata: metadata, readOnly: true);
      expect(find.text('Corded Power Tool'), findsOneWidget);
    });

    testWidgets('hides tiles when both category and material are empty in read-only', (tester) async {
      final metadata = GearMetadata(brand: TrackedString(value: 'Ryobi'));
      await _showSheet(tester, metadata: metadata, readOnly: true);
      expect(find.text('CATEGORY'), findsNothing);
      expect(find.text('MATERIAL'), findsNothing);
    });
  });

  group('Brand section — edit mode', () {
    testWidgets('shows category and material tiles in edit mode when fields empty', (tester) async {
      await _showSheet(tester);
      expect(find.text('CATEGORY'), findsOneWidget);
      expect(find.text('MATERIAL'), findsOneWidget);
    });

    testWidgets('brand and model fields are editable', (tester) async {
      await _showSheet(tester);
      await tester.enterText(find.byType(TextField).first, 'DeWalt');
      expect(find.text('DeWalt'), findsOneWidget);
    });
  });

  // ── Metric tiles ─────────────────────────────────────────────────────────────

  group('Metric tiles — read-only', () {
    testWidgets('shows EST. VALUE label', (tester) async {
      await _showSheet(tester, readOnly: true);
      expect(find.text('EST. VALUE'), findsOneWidget);
    });

    testWidgets('shows WEIGHT label', (tester) async {
      await _showSheet(tester, readOnly: true);
      expect(find.text('WEIGHT'), findsOneWidget);
    });

    testWidgets('shows formatted value when set', (tester) async {
      final ve = ValueEstimate(estimatedValueUsd: 165);
      await _showSheet(tester, valueEstimate: ve, readOnly: true);
      expect(find.text('165.00'), findsOneWidget);
    });

    testWidgets('shows -- when value is absent', (tester) async {
      await _showSheet(tester, readOnly: true);
      // -- appears for both value and weight
      expect(find.text('--'), findsWidgets);
    });

    testWidgets('shows weight with unit suffix', (tester) async {
      final metadata = GearMetadata(
        weightGrams: TrackedEstimate(value: Estimate(mean: 5000)),
      );
      await _showSheet(tester, metadata: metadata, readOnly: true);
      expect(find.text('5.0'), findsOneWidget);
      expect(find.text('kg'), findsOneWidget);
    });
  });

  group('Metric tiles — edit mode', () {
    testWidgets('weight unit toggle visible', (tester) async {
      await _showSheet(tester);
      expect(find.text('g'), findsOneWidget);
      expect(find.text('kg'), findsOneWidget);
      expect(find.text('lbs'), findsOneWidget);
    });
  });

  // ── URL section ──────────────────────────────────────────────────────────────

  group('URL section — read-only', () {
    testWidgets('shows URL card when sourceUrl is provided', (tester) async {
      await _showSheet(
        tester,
        sourceUrl: 'https://www.ryobitools.com/product/123',
        readOnly: true,
      );
      // The display strips the leading https://www. prefix
      expect(find.text('ryobitools.com/product/123'), findsOneWidget);
    });

    testWidgets('hides URL section when no URL and read-only', (tester) async {
      await _showSheet(tester, readOnly: true);
      expect(find.text('Product page'), findsNothing);
    });
  });

  group('URL section — edit mode', () {
    testWidgets('shows Product page label', (tester) async {
      await _showSheet(tester);
      expect(find.textContaining('Product page'), findsOneWidget);
    });

    testWidgets('shows empty-state hint when no URL entered', (tester) async {
      await _showSheet(tester);
      expect(
        find.textContaining("Add a link to auto-fill specs"),
        findsOneWidget,
      );
    });

    testWidgets('auto-fill button hidden when no URL', (tester) async {
      await _showSheet(tester, onAutoFill: (url) async => null);
      expect(find.text('Auto-fill from this page'), findsNothing);
    });

    testWidgets('auto-fill button visible when URL is typed', (tester) async {
      await _showSheet(tester, onAutoFill: (url) async => null);
      // Find the URL input field and enter a URL
      final urlFields = find.byType(TextField);
      await tester.enterText(urlFields.last, 'https://example.com/product');
      await tester.pump();
      expect(find.text('Auto-fill from this page'), findsOneWidget);
    });

    testWidgets('auto-fill button hidden when onAutoFill callback is null', (tester) async {
      await _showSheet(tester, sourceUrl: 'https://example.com');
      expect(find.text('Auto-fill from this page'), findsNothing);
    });
  });

  // ── Auto-fill flow ───────────────────────────────────────────────────────────

  group('Auto-fill', () {
    testWidgets('populates fields from DetectedGearItem on success', (tester) async {
      final detected = DetectedGearItem(
        brand: 'DeWalt',
        model: 'DCD771C2',
        category: 'Power Tools',
        materialCategory: MaterialCategory.MATERIAL_CATEGORY_CORDLESS_POWER_TOOL,
        valueEstimate: ValueEstimate(estimatedValueUsd: 129),
        weightGrams: Estimate(mean: 1200),
      );

      await _showSheet(
        tester,
        onAutoFill: (url) async => detected,
      );

      // Enter a URL to show the auto-fill button
      final urlFields = find.byType(TextField);
      await tester.enterText(urlFields.last, 'https://example.com');
      await tester.pump();

      await tester.ensureVisible(find.text('Auto-fill from this page'));
      await tester.tap(find.text('Auto-fill from this page'), warnIfMissed: false);
      await tester.pumpAndSettle();

      expect(find.text('DeWalt'), findsOneWidget);
      expect(find.text('DCD771C2'), findsOneWidget);
      expect(find.text('Power Tools'), findsOneWidget);
    });

    testWidgets('shows error and leaves fields unchanged when callback returns null', (tester) async {
      await _showSheet(
        tester,
        onAutoFill: (url) async => null,
      );

      final urlFields = find.byType(TextField);
      await tester.enterText(urlFields.last, 'https://example.com');
      await tester.pump();

      await tester.ensureVisible(find.text('Auto-fill from this page'));
      await tester.tap(find.text('Auto-fill from this page'), warnIfMissed: false);
      await tester.pumpAndSettle();

      expect(find.textContaining("Couldn't read product details"), findsOneWidget);
    });

    testWidgets('shows inline error and leaves fields unchanged on callback error', (tester) async {
      await _showSheet(
        tester,
        onAutoFill: (url) async => throw Exception('network error'),
      );

      final urlFields = find.byType(TextField);
      await tester.enterText(urlFields.last, 'https://example.com');
      await tester.pump();

      await tester.ensureVisible(find.text('Auto-fill from this page'));
      await tester.tap(find.text('Auto-fill from this page'), warnIfMissed: false);
      await tester.pumpAndSettle();

      expect(find.textContaining("Couldn't read that page"), findsOneWidget);
      // Brand should still be empty (no overwrite on error)
      expect(find.text('DeWalt'), findsNothing);
    });
  });

  // ── Bottom bar state transitions ─────────────────────────────────────────────

  group('Bottom bar', () {
    testWidgets('shows No changes yet initially', (tester) async {
      await _showSheet(tester);
      expect(find.text('No changes yet'), findsOneWidget);
    });

    testWidgets('shows Save Changes after editing brand', (tester) async {
      await _showSheet(tester);
      await tester.enterText(find.byType(TextField).first, 'Bosch');
      await tester.pump();
      expect(find.text('Save Changes'), findsOneWidget);
    });

    testWidgets('hidden in read-only mode', (tester) async {
      await _showSheet(tester, readOnly: true);
      expect(find.text('No changes yet'), findsNothing);
      expect(find.text('Save Changes'), findsNothing);
    });

    testWidgets('shows Saved and checkmark after tap', (tester) async {
      await _showSheet(tester);
      await tester.enterText(find.byType(TextField).first, 'Bosch');
      await tester.pump();
      await tester.tap(find.text('Save Changes'));
      await tester.pump();
      expect(find.text('Saved'), findsOneWidget);
      expect(find.byIcon(Icons.check), findsOneWidget);
      // Consume the 2-second delayed pop to avoid pending timer assertion.
      await tester.pump(const Duration(seconds: 2));
      await tester.pumpAndSettle();
    });

    testWidgets('buildResult includes sourceUrl when URL set', (tester) async {
      GearMetadataEditResult? result;
      await tester.pumpWidget(
        _wrap(
          Builder(
            builder: (ctx) => ElevatedButton(
              onPressed: () async {
                result = await GearMetadataSheet.show(
                  ctx,
                  sourceUrl: 'https://example.com',
                );
              },
              child: const Text('Open'),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Trigger a save
      await tester.enterText(find.byType(TextField).first, 'Ryobi');
      await tester.pump();
      await tester.tap(find.text('Save Changes'));
      await tester.pump(); // shows Saved
      await tester.pump(const Duration(seconds: 2)); // waits for pop
      await tester.pumpAndSettle();

      expect(result?.sourceUrl, 'https://example.com');
    });
  });

  // ── GearMetadataEditResult ────────────────────────────────────────────────────

  group('GearMetadataEditResult', () {
    test('sourceUrl defaults to null', () {
      final result = GearMetadataEditResult(metadata: GearMetadata());
      expect(result.sourceUrl, isNull);
    });

    test('stores sourceUrl when provided', () {
      final result = GearMetadataEditResult(
        metadata: GearMetadata(),
        sourceUrl: 'https://example.com',
      );
      expect(result.sourceUrl, 'https://example.com');
    });
  });
}
