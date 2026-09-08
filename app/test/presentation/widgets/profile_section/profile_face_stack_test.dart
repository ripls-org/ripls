import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) {
  return ProviderScope(child: localizedApp(child));
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('ProfileFaceStack', () {
    testWidgets('renders nothing when there are no faces and no overflow',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileFaceStack(faces: []),
      ));
      await tester.pumpAndSettle();
      expect(find.byType(Stack), findsNothing);
    });

    testWidgets('renders fallback initials for faces without media',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileFaceStack(faces: [
          FaceStackEntry(initial: 'A'),
          FaceStackEntry(initial: 'B'),
        ]),
      ));
      await tester.pumpAndSettle();
      expect(find.text('A'), findsOneWidget);
      expect(find.text('B'), findsOneWidget);
    });

    testWidgets('renders the +N overflow tail', (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileFaceStack(
          faces: [FaceStackEntry(initial: 'A')],
          overflowCount: 2,
        ),
      ));
      await tester.pumpAndSettle();
      expect(find.text('+2'), findsOneWidget);
    });

    testWidgets('is excluded from semantics unless a label is provided',
        (tester) async {
      await tester.pumpWidget(_wrap(
        const ProfileFaceStack(
          faces: [FaceStackEntry(initial: 'A')],
          semanticsLabel: 'Alfred and 1 other are in',
        ),
      ));
      await tester.pumpAndSettle();
      expect(
        find.bySemanticsLabel('Alfred and 1 other are in'),
        findsOneWidget,
      );
    });
  });
}
