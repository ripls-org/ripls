import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/create/unified_preview_card.dart';

/// Fake notifier that returns a fixed [UnifiedCreateState]. Lets each
/// test seed the exact state-shape it wants to render.
class _FakeUnifiedCreateNotifier extends UnifiedCreateViewModel {
  _FakeUnifiedCreateNotifier(this._initial);
  final UnifiedCreateState _initial;

  @override
  UnifiedCreateState build() => _initial;
}

Widget _harness(UnifiedCreateState state, Widget child) {
  return ProviderScope(
    overrides: [
      unifiedCreateViewModelProvider.overrideWith(
        () => _FakeUnifiedCreateNotifier(state),
      ),
    ],
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
  group('UnifiedPreviewCard', () {
    testWidgets('renders title + description when populated', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
        title: 'Looking for a drill',
        description: 'Anyone have one?',
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      expect(find.text('Looking for a drill'), findsOneWidget);
      expect(find.text('Anyone have one?'), findsOneWidget);
    });

    testWidgets('shows Lend/Give toggle + item-details row for GEAR',
        (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Climbing rope',
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      // Lend / Give labels (SegmentedButton renders them as Text).
      expect(find.text('Lend'), findsOneWidget);
      expect(find.text('Give'), findsOneWidget);
      // Empty-state item-details row.
      expect(find.text('Item details'), findsOneWidget);
    });

    testWidgets(
        'item-details row shows the estimated value when only the value is '
        'set — never a blank row (#2724)', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Lawn mower',
        itemDetails: const ItemDetailsValue(estValueUsd: '120.00'),
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      expect(find.text('Estimated value ~\$120'), findsOneWidget);
    });

    testWidgets('item-details row appends the estimated value to brand/model',
        (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Lawn mower',
        itemDetails: const ItemDetailsValue(
          brand: 'Craftsman',
          model: 'M110',
          estValueUsd: '120.00',
        ),
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      expect(find.text('Craftsman · M110 · ~\$120'), findsOneWidget);
    });

    testWidgets(
        'item-details row falls back to the "Item details" label when the '
        'set fields have no summary form (material only)', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Lawn mower',
        itemDetails: const ItemDetailsValue(material: 'Steel'),
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      expect(find.text('Item details'), findsOneWidget);
    });

    testWidgets('shows time row for EVENT type', (tester) async {
      final eventState = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        title: 'Hike',
      );
      await tester.pumpWidget(_harness(eventState, const UnifiedPreviewCard()));
      await tester.pumpAndSettle();
      expect(find.text('Set time'), findsOneWidget);
    });

    testWidgets('does NOT show time row for GEAR type', (tester) async {
      final gearState = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Climbing rope',
      );
      await tester.pumpWidget(_harness(gearState, const UnifiedPreviewCard()));
      await tester.pumpAndSettle();
      expect(find.text('Set time'), findsNothing);
    });

    testWidgets('save button disabled when isSaveable=false', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        // streamComplete is false → isSaveable=false regardless of fields
        streamComplete: false,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
        title: 'Anything',
        description: 'Some body',
      );
      await tester.pumpWidget(_harness(state, const UnifiedPreviewCard()));
      // Per-type label: request → "Save Request".
      final button = find.text('Save Request');
      expect(button, findsOneWidget);
      final inkWell = tester.widget<InkWell>(
        find.ancestor(of: button, matching: find.byType(InkWell)).first,
      );
      expect(inkWell.onTap, isNull);
    });

    testWidgets('save button enabled when isSaveable=true', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
        title: 'Anyone with a drill',
        description: 'Some body',
      );
      await tester.pumpWidget(
        _harness(state, const UnifiedPreviewCard(onShare: _noop)),
      );
      final button = find.text('Save Request');
      final inkWell = tester.widget<InkWell>(
        find.ancestor(of: button, matching: find.byType(InkWell)).first,
      );
      expect(inkWell.onTap, isNotNull);
    });

    testWidgets('save button label is "Save Event" for events', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        title: 'Group hike',
        description: 'Mt Sanitas tomorrow morning, all paces welcome',
      );
      await tester.pumpWidget(
        _harness(state, const UnifiedPreviewCard(onShare: _noop)),
      );
      expect(find.text('Save Event'), findsOneWidget);
    });

    testWidgets('save button label is "Save" for gear', (tester) async {
      final state = UnifiedCreateState(
        stage: CreateStage.preview,
        streamComplete: true,
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Climbing rope',
        description: '9.6mm',
      );
      await tester.pumpWidget(
        _harness(state, const UnifiedPreviewCard(onShare: _noop)),
      );
      expect(find.text('Save'), findsOneWidget);
    });
  });
}

void _noop() {}
