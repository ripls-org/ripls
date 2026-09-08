import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/home/calendar/plans_date_sheet.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../../helpers/l10n_helpers.dart';

const _groups = [
  PlansSheetGroup(id: 'c1', name: 'Maple Street'),
  PlansSheetGroup(id: 'c2', name: 'Hiking Crew'),
];

/// Pumps the sheet directly and records every live-applied change.
Future<({List<DateTime> dates, List<String?> groups})> _pump(
  WidgetTester tester, {
  String? selectedGroupId,
}) async {
  final dates = <DateTime>[];
  final groups = <String?>[];
  await tester.pumpWidget(localizedApp(Scaffold(
    body: SingleChildScrollView(
      child: PlansDateSheet(
        initialDate: DateTime(2026, 7, 7),
        groups: _groups,
        selectedGroupId: selectedGroupId,
        onDateChanged: dates.add,
        onGroupChanged: groups.add,
      ),
    ),
  )));
  await tester.pumpAndSettle();
  return (dates: dates, groups: groups);
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('PlansDateSheet (live apply)', () {
    testWidgets('prefills the segments and resolves the date plainly',
        (tester) async {
      await _pump(tester);

      expect(find.text('Go to date'), findsOneWidget);
      expect(find.text('07'), findsNWidgets(2)); // month + day
      expect(find.text('2026'), findsOneWidget);
      // No "Type it —" instruction, just the resolved date; no footer.
      expect(find.text('Tuesday, July 7, 2026'), findsOneWidget);
      expect(find.textContaining('Type it'), findsNothing);
      expect(find.textContaining('Go to July'), findsNothing);
      expect(find.text('Clear'), findsNothing);
    });

    testWidgets('typing a valid day applies immediately', (tester) async {
      final changes = await _pump(tester);

      await tester.enterText(find.byType(TextField).at(1), '12');
      await tester.pumpAndSettle();

      expect(changes.dates, [DateTime(2026, 7, 12)]);
    });

    testWidgets('an impossible date shows the invalid line and applies nothing',
        (tester) async {
      final changes = await _pump(tester);

      await tester.enterText(find.byType(TextField).at(0), '02');
      await tester.enterText(find.byType(TextField).at(1), '31');
      await tester.pumpAndSettle();

      expect(find.text('Not a real date yet'), findsOneWidget);
      // The month edit (02/07) still resolved; the impossible 02/31 didn't.
      expect(changes.dates, [DateTime(2026, 2, 7)]);
    });

    testWidgets('tapping a group chip applies the scope immediately',
        (tester) async {
      final changes = await _pump(tester);

      await tester.tap(find.text('Hiking Crew'));
      await tester.pumpAndSettle();
      expect(changes.groups, ['c2']);

      await tester.tap(find.text('All groups'));
      await tester.pumpAndSettle();
      expect(changes.groups, ['c2', null]);
    });

    testWidgets('quick chip applies today immediately', (tester) async {
      final changes = await _pump(tester);

      await tester.tap(find.text('Today'));
      await tester.pumpAndSettle();

      final now = DateTime.now();
      expect(changes.dates, [DateTime(now.year, now.month, now.day)]);
    });
  });
}
