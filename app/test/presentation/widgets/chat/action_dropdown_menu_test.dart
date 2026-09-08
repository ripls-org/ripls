import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart';

void main() {
  Widget buildApp({required List<ActionDropdownItem> items}) {
    return MaterialApp(
      home: Scaffold(
        body: ActionDropdownMenu(items: items),
      ),
    );
  }

  group('ActionDropdownMenu leadingWidget', () {
    testWidgets('renders default icon circle when leadingWidget is null',
        (tester) async {
      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Edit',
          onTap: () {},
        ),
      ]));

      // Default icon should be present.
      expect(find.byIcon(Icons.edit), findsOneWidget);
      // No custom leading widget marker.
      expect(find.byKey(const Key('custom_leading')), findsNothing);
    });

    testWidgets('renders leadingWidget instead of default icon when set',
        (tester) async {
      final customWidget = Container(
        key: const Key('custom_leading'),
        width: 36,
        height: 36,
        decoration: const BoxDecoration(
          color: Colors.green,
          shape: BoxShape.circle,
        ),
        child: const Icon(Icons.check, size: 18, color: Colors.white),
      );

      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Edit',
          leadingWidget: customWidget,
          onTap: () {},
        ),
      ]));

      // Custom leading widget should be present.
      expect(find.byKey(const Key('custom_leading')), findsOneWidget);
      // The check icon from the custom widget.
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('leadingWidget is sized to 36x36', (tester) async {
      final customWidget = Container(
        key: const Key('sized_widget'),
        color: Colors.blue,
      );

      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Edit',
          leadingWidget: customWidget,
          onTap: () {},
        ),
      ]));

      final sizedBox = tester.widget<SizedBox>(
        find.ancestor(
          of: find.byKey(const Key('sized_widget')),
          matching: find.byType(SizedBox),
        ),
      );
      expect(sizedBox.width, 36);
      expect(sizedBox.height, 36);
    });

    testWidgets('mix of items with and without leadingWidget',
        (tester) async {
      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Edit',
          leadingWidget: Container(
            key: const Key('custom_1'),
            decoration: const BoxDecoration(
              color: Colors.green,
              shape: BoxShape.circle,
            ),
            child: const Icon(Icons.check, size: 18),
          ),
          onTap: () {},
        ),
        ActionDropdownItem(
          icon: Icons.delete,
          label: 'Delete',
          isDestructive: true,
          onTap: () {},
        ),
      ]));

      // First item has custom leading widget.
      expect(find.byKey(const Key('custom_1')), findsOneWidget);
      // Second item has default delete icon.
      expect(find.byIcon(Icons.delete), findsOneWidget);
      // Both labels rendered.
      expect(find.text('Edit'), findsOneWidget);
      expect(find.text('Delete'), findsOneWidget);
    });
  });

  group('ActionDropdownMenu children support', () {
    testWidgets('header row with children is not tappable', (tester) async {
      var headerTapped = false;
      var childTapped = false;

      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.person,
          label: 'Alice',
          onTap: () => headerTapped = true,
          children: [
            ActionDropdownItem(
              icon: Icons.check_circle_outline,
              label: 'Confirm Pickup',
              onTap: () => childTapped = true,
            ),
          ],
        ),
      ]));

      // Header row should be rendered but not respond to tap.
      expect(find.text('Alice'), findsOneWidget);
      await tester.tap(find.text('Alice'));
      await tester.pump();
      expect(headerTapped, isFalse);

      // Child row should respond to tap.
      expect(find.text('Confirm Pickup'), findsOneWidget);
      await tester.tap(find.text('Confirm Pickup'));
      await tester.pump();
      expect(childTapped, isTrue);
    });

    testWidgets('child items render with indentation', (tester) async {
      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Top Level',
          onTap: () {},
        ),
        ActionDropdownItem(
          icon: Icons.person,
          label: 'Header',
          children: [
            ActionDropdownItem(
              icon: Icons.check,
              label: 'Child Item',
              onTap: () {},
            ),
          ],
        ),
      ]));

      // Both items rendered.
      expect(find.text('Top Level'), findsOneWidget);
      expect(find.text('Header'), findsOneWidget);
      expect(find.text('Child Item'), findsOneWidget);

      // Find the Padding widgets for the top-level vs child rows.
      // Top-level row should have left padding of 16.
      final topLevelPadding = tester.widget<Padding>(
        find.ancestor(
          of: find.text('Top Level'),
          matching: find.byType(Padding),
        ).first,
      );
      expect(topLevelPadding.padding, isA<EdgeInsets>());
      final topInsets = topLevelPadding.padding as EdgeInsets;
      expect(topInsets.left, 16.0);

      // Child row should have left padding of 32.
      final childPadding = tester.widget<Padding>(
        find.ancestor(
          of: find.text('Child Item'),
          matching: find.byType(Padding),
        ).first,
      );
      expect(childPadding.padding, isA<EdgeInsets>());
      final childInsets = childPadding.padding as EdgeInsets;
      expect(childInsets.left, 32.0);
    });

    testWidgets('multiple borrower groups render all children',
        (tester) async {
      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.person,
          label: 'Borrower 1',
          children: [
            ActionDropdownItem(
              icon: Icons.check,
              label: 'Pickup 1',
              onTap: () {},
            ),
            ActionDropdownItem(
              icon: Icons.assignment_return,
              label: 'Return 1',
              onTap: () {},
            ),
          ],
        ),
        ActionDropdownItem(
          icon: Icons.person,
          label: 'Borrower 2',
          children: [
            ActionDropdownItem(
              icon: Icons.check,
              label: 'Pickup 2',
              onTap: () {},
            ),
            ActionDropdownItem(
              icon: Icons.assignment_return,
              label: 'Return 2',
              onTap: () {},
            ),
          ],
        ),
      ]));

      expect(find.text('Borrower 1'), findsOneWidget);
      expect(find.text('Pickup 1'), findsOneWidget);
      expect(find.text('Return 1'), findsOneWidget);
      expect(find.text('Borrower 2'), findsOneWidget);
      expect(find.text('Pickup 2'), findsOneWidget);
      expect(find.text('Return 2'), findsOneWidget);
    });

    testWidgets('items without children still render normally',
        (tester) async {
      var tapped = false;
      await tester.pumpWidget(buildApp(items: [
        ActionDropdownItem(
          icon: Icons.edit,
          label: 'Regular Item',
          onTap: () => tapped = true,
        ),
      ]));

      expect(find.text('Regular Item'), findsOneWidget);
      await tester.tap(find.text('Regular Item'));
      await tester.pump();
      expect(tapped, isTrue);
    });
  });
}
