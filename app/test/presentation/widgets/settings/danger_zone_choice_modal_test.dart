import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/settings/danger_zone_choice_modal.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('showDangerZoneChoiceModal', () {
    Future<DangerZoneChoice?> openAndPick(
      WidgetTester tester,
      String tapText,
    ) async {
      DangerZoneChoice? result;
      await tester.pumpWidget(localizedApp(
        Builder(
          builder: (context) => Center(
            child: ElevatedButton(
              onPressed: () async {
                result = await showDangerZoneChoiceModal(
                  context: context,
                  communityName: 'Tide Pool',
                );
              },
              child: const Text('open'),
            ),
          ),
        ),
      ));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();
      // Confirm modal rendered the title.
      expect(find.text('Delete or leave?'), findsOneWidget);
      await tester.tap(find.text(tapText));
      await tester.pumpAndSettle();
      return result;
    }

    testWidgets('Delete-for-everyone returns DangerZoneChoice.delete',
        (tester) async {
      final picked =
          await openAndPick(tester, 'Delete Tide Pool for everyone');
      expect(picked, DangerZoneChoice.delete);
    });

    testWidgets('Just-leave returns DangerZoneChoice.leaveWithHandoff',
        (tester) async {
      final picked = await openAndPick(
          tester, 'Just leave — let someone else take over');
      expect(picked, DangerZoneChoice.leaveWithHandoff);
    });

    testWidgets('Cancel returns DangerZoneChoice.cancel', (tester) async {
      final picked = await openAndPick(tester, 'Cancel');
      expect(picked, DangerZoneChoice.cancel);
    });

    testWidgets('Drag-down dismisses with null (treated as cancel)',
        (tester) async {
      DangerZoneChoice? result = DangerZoneChoice.delete; // sentinel
      await tester.pumpWidget(localizedApp(
        Builder(
          builder: (context) => Center(
            child: ElevatedButton(
              onPressed: () async {
                result = await showDangerZoneChoiceModal(
                  context: context,
                  communityName: 'Tide Pool',
                );
              },
              child: const Text('open'),
            ),
          ),
        ),
      ));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Tap the barrier (outside the sheet) to dismiss.
      await tester.tapAt(const Offset(20, 20));
      await tester.pumpAndSettle();

      expect(result, isNull);
    });
  });
}
