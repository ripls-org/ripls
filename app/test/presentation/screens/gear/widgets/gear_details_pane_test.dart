import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show TrackedString, TrackedEstimate, Estimate;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata, GetGearResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_details_pane.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import '../../../../helpers/l10n_helpers.dart';

GearDetailsPane _buildPane({
  required GearState state,
  String editingBrand = '',
  String editingModel = '',
  String editingValueUsd = '',
  String editingWeightGrams = '',
  String editingWebsite = '',
}) {
  return GearDetailsPane(
    state: state,
    gearId: 'gear-1',
    isEditingMetadataInitialized: false,
    editingBrand: editingBrand,
    editingModel: editingModel,
    editingValueUsd: editingValueUsd,
    editingWeightGrams: editingWeightGrams,
    editingWebsite: editingWebsite,
    onBrandChanged: (_) {},
    onModelChanged: (_) {},
    onValueChanged: (_) {},
    onWeightChanged: (_) {},
    onWebsiteChanged: (_) {},
    onLocationTap: () {},
  );
}

GearState _ownerState({required GetGearResponse gear, bool isEditing = false}) {
  return GearState(
    gearId: gear.id,
    currentUserId: gear.owner.id,
    gearDetails: gear,
    isLoading: false,
    isEditing: isEditing,
  );
}

GetGearResponse _gearWithMetadata({
  String brand = 'DeWalt',
  String model = 'DCD771C2',
  double weightGramsMean = 1500,
}) {
  return GetGearResponse(
    id: 'gear-1',
    name: 'Test Gear',
    description: 'A test gear item',
    owner: User(id: 'owner-1', name: 'Test Owner'),
    metadata: GearMetadata(
      brand: TrackedString(value: brand),
      model: TrackedString(value: model),
      weightGrams: TrackedEstimate(value: Estimate(mean: weightGramsMean)),
    ),
  );
}

void main() {
  group('GearDetailsPane — read-only owner view (Bug 1 fix)', () {
    testWidgets('owner not in edit mode sees brand value not hint placeholder',
        (tester) async {
      final gear = _gearWithMetadata(brand: 'DeWalt');
      final state = _ownerState(gear: gear, isEditing: false);

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(state: state))));
      await tester.pump();

      expect(find.text('DeWalt'), findsOneWidget,
          reason: 'brand value should be visible in read-only owner view');
      expect(find.text('Brand name'), findsNothing,
          reason: 'hint placeholder must not appear in non-editing view');
    });

    testWidgets('owner not in edit mode sees model value', (tester) async {
      final gear = _gearWithMetadata(model: 'DCD771C2');
      final state = _ownerState(gear: gear, isEditing: false);

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(state: state))));
      await tester.pump();

      expect(find.text('DCD771C2'), findsOneWidget,
          reason: 'model value should be visible in read-only owner view');
    });

    testWidgets('model row absent when metadata.model is empty', (tester) async {
      final gear = _gearWithMetadata(model: '');
      final state = _ownerState(gear: gear, isEditing: false);

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(state: state))));
      await tester.pump();

      // Model label should not appear when model is empty.
      expect(find.text('Model'), findsNothing);
    });
  });

  group('GearDetailsPane — non-owner read-only view (Bug 2 fix)', () {
    testWidgets('non-owner sees both brand and model values', (tester) async {
      final gear = _gearWithMetadata(brand: 'Bosch', model: 'PS31-2A');
      final state = GearState(
        gearId: gear.id,
        currentUserId: 'other-user',
        gearDetails: gear,
        isLoading: false,
      );

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(state: state))));
      await tester.pump();

      expect(find.text('Bosch'), findsOneWidget);
      expect(find.text('PS31-2A'), findsOneWidget);
    });
  });

  group('GearDetailsPane — edit mode (owner is editing)', () {
    testWidgets('owner in edit mode sees editable rows for brand and model',
        (tester) async {
      final gear = _gearWithMetadata(brand: 'DeWalt', model: 'DCD771C2');
      final state = _ownerState(gear: gear, isEditing: true);

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(
        state: state,
        editingBrand: 'DeWalt',
        editingModel: 'DCD771C2',
      ))));
      await tester.pump();

      // In edit mode we see TextFormFields for brand and model with the
      // current values pre-populated via initialValue.
      final brandField = find.widgetWithText(TextFormField, 'DeWalt');
      final modelField = find.widgetWithText(TextFormField, 'DCD771C2');
      expect(brandField, findsOneWidget, reason: 'editable brand row should be present');
      expect(modelField, findsOneWidget, reason: 'editable model row should be present');
    });

    testWidgets('edit mode renders TextFormFields not a ContentDetailCard',
        (tester) async {
      final gear = _gearWithMetadata(brand: 'DeWalt');
      final state = _ownerState(gear: gear, isEditing: true);

      await tester.pumpWidget(localizedApp(Scaffold(body: _buildPane(
        state: state,
        editingBrand: 'DeWalt',
      ))));
      await tester.pump();

      // In edit mode the editable container is shown (has TextFormFields).
      expect(find.byType(TextFormField), findsWidgets);
    });
  });
}
