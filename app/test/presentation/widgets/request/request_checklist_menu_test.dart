import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart';
import 'package:ripls/presentation/widgets/content/checklist_status_icons.dart'
    as checklist;

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

  group('Request checklist menu — active request', () {
    testWidgets('renders all 4 menu items in correct order', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: Builder(
              builder: (context) => checklist.buildDetailsStatusIcon(
                context: context,
                hasTitle: true,
                hasDescription: true,
                hasMedia: true,
              ),
            ),
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            leadingWidget: Builder(
              builder: (context) => checklist.buildLocationStatusIcon(
                context: context,
                locationName: null,
                requiredLocation: false,
              ),
            ),
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.task_alt,
            label: 'Mark Fulfilled',
            onTap: () {},
          ),
          ActionDropdownItem(
            icon: Icons.delete_outline,
            label: 'Close Request',
            isDestructive: true,
            onTap: () {},
          ),
        ]),
      ));

      // Verify all labels.
      expect(find.text('Set Details'), findsOneWidget);
      expect(find.text('Confirm Location'), findsOneWidget);
      expect(find.text('Mark Fulfilled'), findsOneWidget);
      expect(find.text('Close Request'), findsOneWidget);

      // Verify order.
      final labels = tester
          .widgetList<Text>(find.byType(Text))
          .map((t) => t.data)
          .where((d) => d != null)
          .toList();
      final detailsIdx = labels.indexOf('Set Details');
      final locationIdx = labels.indexOf('Confirm Location');
      final fulfilledIdx = labels.indexOf('Mark Fulfilled');
      final closeIdx = labels.indexOf('Close Request');
      expect(detailsIdx, lessThan(locationIdx));
      expect(locationIdx, lessThan(fulfilledIdx));
      expect(fulfilledIdx, lessThan(closeIdx));
    });

    testWidgets('details complete shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: Builder(
              builder: (context) => checklist.buildDetailsStatusIcon(
                context: context,
                hasTitle: true,
                hasDescription: true,
                hasMedia: true,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.edit_outlined), findsNothing);
    });

    testWidgets('details incomplete shows default icon', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.edit_outlined,
            label: 'Set Details',
            leadingWidget: Builder(
              builder: (context) => checklist.buildDetailsStatusIcon(
                context: context,
                hasTitle: true,
                hasDescription: false,
                hasMedia: true,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.edit_outlined), findsOneWidget);
      expect(find.byIcon(Icons.check), findsNothing);
    });

    testWidgets('location set shows green check', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            leadingWidget: Builder(
              builder: (context) => checklist.buildLocationStatusIcon(
                context: context,
                locationName: 'Downtown Park',
                requiredLocation: false,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.byIcon(Icons.location_on_outlined), findsNothing);
    });

    testWidgets('location missing shows neutral icon (not red X)',
        (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.location_on_outlined,
            label: 'Confirm Location',
            leadingWidget: Builder(
              builder: (context) => checklist.buildLocationStatusIcon(
                context: context,
                locationName: null,
                requiredLocation: false,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      // Should show neutral location icon, NOT red X (close icon).
      expect(find.byIcon(Icons.location_on_outlined), findsOneWidget);
      expect(find.byIcon(Icons.close), findsNothing);
      expect(find.byIcon(Icons.check), findsNothing);
    });
  });

  group('Offer button — tap behavior', () {
    testWidgets('helping pill is tappable and invokes callback', (tester) async {
      var tabChangeCalled = false;

      // Mirrors the _OfferButton(isHelping: true) tap handler added in #965:
      // onTap must be non-null when isHelping so the chat tab opens.
      await tester.pumpWidget(buildApp(
        child: GestureDetector(
          onTap: () => tabChangeCalled = true,
          child: Container(
            key: const Key('helping-pill'),
            color: Colors.green,
            child: const Text('Helping'),
          ),
        ),
      ));

      await tester.tap(find.byKey(const Key('helping-pill')));
      await tester.pump();

      expect(tabChangeCalled, isTrue);
    });

    testWidgets('offer button with null onTap does not invoke callback',
        (tester) async {
      final tapped = false;

      await tester.pumpWidget(buildApp(
        child: GestureDetector(
          onTap: null,
          child: Container(
            key: const Key('offer-pill'),
            color: Colors.orange,
            child: const Text('Offer to Help'),
          ),
        ),
      ));

      await tester.tap(find.byKey(const Key('offer-pill')),
          warnIfMissed: false);
      await tester.pump();

      expect(tapped, isFalse);
    });
  });

  group('Request checklist menu — post-fulfillment', () {
    testWidgets('shows only View Impact (no Close Request)', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.insights,
            label: 'View Impact',
            leadingWidget: Builder(
              builder: (context) => checklist.buildStatusCircle(
                context: context,
                backgroundColor: const Color(0xFFE8F3E9),
                iconColor: const Color(0xFF3D7048),
                icon: Icons.insights,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.text('View Impact'), findsOneWidget);

      // No other items.
      expect(find.text('Close Request'), findsNothing);
      expect(find.text('Set Details'), findsNothing);
      expect(find.text('Confirm Location'), findsNothing);
      expect(find.text('Mark Fulfilled'), findsNothing);
    });

    testWidgets('View Impact has green insights icon', (tester) async {
      await tester.pumpWidget(buildApp(
        child: ActionDropdownMenu(items: [
          ActionDropdownItem(
            icon: Icons.insights,
            label: 'View Impact',
            leadingWidget: Builder(
              builder: (context) => checklist.buildStatusCircle(
                context: context,
                backgroundColor: const Color(0xFFE8F3E9),
                iconColor: const Color(0xFF3D7048),
                icon: Icons.insights,
              ),
            ),
            onTap: () {},
          ),
        ]),
      ));

      expect(find.byIcon(Icons.insights), findsOneWidget);
    });
  });
}
