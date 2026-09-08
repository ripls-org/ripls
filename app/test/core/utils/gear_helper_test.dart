import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/gear_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;

void main() {
  group('GearHelper', () {
    group('validateGearFields', () {
      testWidgets('returns true when all fields are valid',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: 'Test Gear',
          description: 'Test Description',
          locationId: 'location123',
        );

        // Assert
        expect(result, isTrue);
      });

      testWidgets('returns false when name is empty',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: '',
          description: 'Test Description',
          locationId: 'location123',
        );

        // Assert
        expect(result, isFalse);
      });

      testWidgets('returns false when name is whitespace only',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: '   ',
          description: 'Test Description',
          locationId: 'location123',
        );

        // Assert
        expect(result, isFalse);
      });

      testWidgets('returns false when description is empty',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: 'Test Gear',
          description: '',
          locationId: 'location123',
        );

        // Assert
        expect(result, isFalse);
      });

      testWidgets('returns false when description is whitespace only',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: 'Test Gear',
          description: '   ',
          locationId: 'location123',
        );

        // Assert
        expect(result, isFalse);
      });

      testWidgets('returns true when locationId is null (location is optional)',
          (WidgetTester tester) async {
        // Arrange
        await tester.pumpWidget(
          const MaterialApp(
            home: Scaffold(
              body: Text('Test'),
            ),
          ),
        );
        final context = tester.element(find.text('Test'));

        // Act
        final result = GearHelper.validateGearFields(
          context: context,
          name: 'Test Gear',
          description: 'Test Description',
          locationId: null,
        );

        // Assert
        expect(result, isTrue);
      });
    });

    group('getAvailabilityActionText', () {
      test('returns "shared" for AVAILABILITY_FOR_LOAN', () {
        // Act
        final result = GearHelper.getAvailabilityActionText(
          Availability.AVAILABILITY_FOR_LOAN,
        );

        // Assert
        expect(result, equals('shared'));
      });

      test('returns "given away" for AVAILABILITY_FOR_GIVEAWAY', () {
        // Act
        final result = GearHelper.getAvailabilityActionText(
          Availability.AVAILABILITY_FOR_GIVEAWAY,
        );

        // Assert
        expect(result, equals('given away'));
      });
    });

    group('getSuccessMessage', () {
      test('returns correct message for AVAILABILITY_FOR_LOAN', () {
        // Act
        final result = GearHelper.getSuccessMessage(
          Availability.AVAILABILITY_FOR_LOAN,
        );

        // Assert
        expect(result, equals('Gear shared successfully!'));
      });

      test('returns correct message for AVAILABILITY_FOR_GIVEAWAY', () {
        // Act
        final result = GearHelper.getSuccessMessage(
          Availability.AVAILABILITY_FOR_GIVEAWAY,
        );

        // Assert
        expect(result, equals('Gear given away successfully!'));
      });
    });
  });
}
