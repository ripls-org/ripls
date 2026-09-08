import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart';

/// Helper to build a status circle matching production code.
Widget buildStatusCircle({
  required Color backgroundColor,
  required Color iconColor,
  required IconData icon,
}) {
  return Container(
    width: 36,
    height: 36,
    decoration: BoxDecoration(
      color: backgroundColor,
      shape: BoxShape.circle,
    ),
    child: Icon(icon, size: 18, color: iconColor),
  );
}

void main() {
  Widget buildApp({required Widget child}) {
    return MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: child),
    );
  }

  group('Experience checklist menu — active event', () {
    testWidgets('renders all 6 menu items in correct order', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.check,
            ),
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.event_available_outlined,
            label: 'RSVP',
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.tune_outlined,
            label: 'Confirm Time',
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.check_circle_outline,
            label: 'Mark Completed',
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.delete_outline,
            label: 'Close Event',
            isDestructive: true,
            onTap: () {},
          ),
        ]),
      ));

      // Verify all labels are present.
      expect(find.text('Set Details'), findsOneWidget);
      expect(find.text('RSVP'), findsOneWidget);
      expect(find.text('Confirm Time'), findsOneWidget);
      expect(find.text('Confirm Location'), findsOneWidget);
      expect(find.text('Mark Completed'), findsOneWidget);
      expect(find.text('Close Event'), findsOneWidget);

      // Verify order via widget positions.
      final labels = tester
          .widgetList<Text>(find.byType(Text))
          .map((t) => t.data)
          .where((d) => d != null)
          .toList();
      final updateIdx = labels.indexOf('Set Details');
      final rsvpIdx = labels.indexOf('RSVP');
      final timeIdx = labels.indexOf('Confirm Time');
      final locationIdx = labels.indexOf('Confirm Location');
      final completedIdx = labels.indexOf('Mark Completed');
      final closeIdx = labels.indexOf('Close Event');
      expect(updateIdx, lessThan(rsvpIdx));
      expect(rsvpIdx, lessThan(locationIdx));
      expect(locationIdx, lessThan(timeIdx));
      expect(timeIdx, lessThan(completedIdx));
      expect(completedIdx, lessThan(closeIdx));
    });

    testWidgets('details complete shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.check,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.edit_outlined), findsNothing);
    });

    testWidgets('details incomplete shows default icon', (tester) async {
      // When title, description, or media is missing, show default edit icon.
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: buildStatusCircle(
              backgroundColor: Colors.grey.shade100,
              iconColor: Colors.grey,
              icon: Icons.edit_outlined,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.edit_outlined), findsOneWidget);
      expect(find.byIcon(Icons.check), findsNothing);
    });

    testWidgets('RSVP Yes shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.event_available_outlined,
            label: 'RSVP',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.check,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.event_available_outlined), findsNothing);
    });

    testWidgets('RSVP No shows red X', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.event_available_outlined,
            label: 'RSVP',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFFFEBEE),
              iconColor: const Color(0xFFC62828),
              icon: Icons.close,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.close), findsOneWidget);
    });

    testWidgets('RSVP Maybe shows yellow question mark', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.event_available_outlined,
            label: 'RSVP',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFFFF3E0),
              iconColor: const Color(0xFFF57C00),
              icon: Icons.question_mark,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.question_mark), findsOneWidget);
    });

    testWidgets('time poll active shows ballot icon', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.poll_outlined,
            label: 'Confirm Time',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFFFEBEE),
              iconColor: const Color(0xFFC62828),
              icon: Icons.how_to_vote,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.how_to_vote), findsOneWidget);
    });

    testWidgets('time confirmed shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.tune_outlined,
            label: 'Confirm Time',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.check,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.tune_outlined), findsNothing);
    });

    testWidgets('location missing shows red X', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFFFEBEE),
              iconColor: const Color(0xFFC62828),
              icon: Icons.close,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.close), findsOneWidget);
    });

    testWidgets('location set shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.check,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.location_on_outlined), findsNothing);
    });
  });

  group('Experience checklist menu — post-completion', () {
    testWidgets('shows only View Impact (no Close Event)', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.insights,
            label: 'View Impact',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.insights,
            ),
            onTap: () {},
          ),
        ]),
      ));

      // Only one item.
      expect(find.text('View Impact'), findsOneWidget);

      // Close Event and active-event items should NOT be present.
      expect(find.text('Close Event'), findsNothing);
      expect(find.text('Set Details'), findsNothing);
      expect(find.text('RSVP'), findsNothing);
      expect(find.text('Confirm Time'), findsNothing);
      expect(find.text('Confirm Location'), findsNothing);
      expect(find.text('Mark Completed'), findsNothing);
    });

    testWidgets('View Impact has green insights icon', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.insights,
            label: 'View Impact',
            leadingWidget: buildStatusCircle(
              backgroundColor: const Color(0xFFE8F3E9),
              iconColor: const Color(0xFF3D7048),
              icon: Icons.insights,
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.insights), findsOneWidget);
    });
  });
}
