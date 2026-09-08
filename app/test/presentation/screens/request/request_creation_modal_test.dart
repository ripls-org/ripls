import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/request/request_creation_modal.dart';
import '../../../helpers/l10n_helpers.dart';

void main() {
  group('RequestCreationModal', () {
    testWidgets('can be constructed without a communityId argument',
        (WidgetTester tester) async {
      // Verify that the no-arg constructor compiles and renders without error.
      await tester.pumpWidget(
        ProviderScope(
          child: localizedApp(const Scaffold(body: RequestCreationModal())),
        ),
      );
      await tester.pump();

      // Modal rendered — no communityId required.
      expect(find.byType(RequestCreationModal), findsOneWidget);
    });
  });
}
