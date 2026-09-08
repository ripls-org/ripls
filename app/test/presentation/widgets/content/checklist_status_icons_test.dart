import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/checklist_status_icons.dart';

void main() {
  Widget buildApp(Widget child) {
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

  group('buildPickupTimeStatusIcon', () {
    testWidgets('shows check when pickup time is set', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildPickupTimeStatusIcon(
            context: context,
            hasPickupTime: true,
          ),
        ),
      ));
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('shows calendar icon when no pickup time', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildPickupTimeStatusIcon(
            context: context,
            hasPickupTime: false,
          ),
        ),
      ));
      expect(find.byIcon(Icons.calendar_today_outlined), findsOneWidget);
    });
  });

  group('buildPickupStatusIcon', () {
    testWidgets('shows check when picked up', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildPickupStatusIcon(
            context: context,
            isPickedUp: true,
          ),
        ),
      ));
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('shows outline icon when not picked up', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildPickupStatusIcon(
            context: context,
            isPickedUp: false,
          ),
        ),
      ));
      expect(find.byIcon(Icons.inventory_2_outlined), findsOneWidget);
    });
  });

  group('buildReturnStatusIcon', () {
    testWidgets('shows check when returned', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildReturnStatusIcon(
            context: context,
            isReturned: true,
          ),
        ),
      ));
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('shows return icon when not returned', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildReturnStatusIcon(
            context: context,
            isReturned: false,
          ),
        ),
      ));
      expect(find.byIcon(Icons.assignment_return_outlined), findsOneWidget);
    });
  });

  group('buildReceivedStatusIcon', () {
    testWidgets('shows check when completed', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildReceivedStatusIcon(
            context: context,
            isCompleted: true,
          ),
        ),
      ));
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('shows outline icon when not completed', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildReceivedStatusIcon(
            context: context,
            isCompleted: false,
          ),
        ),
      ));
      expect(find.byIcon(Icons.card_giftcard), findsOneWidget);
    });
  });

  group('buildRecipientLeadingWidget', () {
    testWidgets('shows initials when no avatar widget', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildRecipientLeadingWidget(
            context: context,
            displayName: 'Alice',
          ),
        ),
      ));
      expect(find.text('A'), findsOneWidget);
      expect(find.byType(CircleAvatar), findsOneWidget);
    });

    testWidgets('uses avatar widget when provided', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildRecipientLeadingWidget(
            context: context,
            displayName: 'Alice',
            avatarWidget: Container(key: const Key('avatar')),
          ),
        ),
      ));
      expect(find.byKey(const Key('avatar')), findsOneWidget);
      // Should be wrapped in a 36x36 SizedBox.
      final sizedBox = tester.widget<SizedBox>(
        find.ancestor(
          of: find.byKey(const Key('avatar')),
          matching: find.byType(SizedBox),
        ),
      );
      expect(sizedBox.width, 36);
      expect(sizedBox.height, 36);
    });

    testWidgets('shows ? for empty name', (tester) async {
      await tester.pumpWidget(buildApp(
        Builder(
          builder: (context) => buildRecipientLeadingWidget(
            context: context,
            displayName: '',
          ),
        ),
      ));
      expect(find.text('?'), findsOneWidget);
    });
  });
}
