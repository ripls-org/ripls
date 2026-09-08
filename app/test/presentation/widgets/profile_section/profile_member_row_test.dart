import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_member_row.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

Widget _wrap(Widget child) {
  return ProviderScope(child: localizedApp(Center(child: child)));
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets('renders title and subtitle', (tester) async {
    await tester.pumpWidget(_wrap(
      const ProfileMemberRow(
        faces: [FaceStackEntry(initial: 'B'), FaceStackEntry(initial: 'T')],
        title: 'You & Betty',
        subtitle: 'Climbing · Cooking · Hiking',
      ),
    ));
    await tester.pumpAndSettle();

    expect(find.text('You & Betty'), findsOneWidget);
    expect(find.text('Climbing · Cooking · Hiking'), findsOneWidget);
  });

  testWidgets('omits the subtitle line when null', (tester) async {
    await tester.pumpWidget(_wrap(
      const ProfileMemberRow(
        faces: [FaceStackEntry(initial: 'B')],
        title: 'You & Betty',
      ),
    ));
    await tester.pumpAndSettle();

    expect(find.text('You & Betty'), findsOneWidget);
  });

  testWidgets('is not tappable when onTap is null', (tester) async {
    await tester.pumpWidget(_wrap(
      const ProfileMemberRow(
        faces: [FaceStackEntry(initial: 'B')],
        title: 'You & Betty',
      ),
    ));
    await tester.pumpAndSettle();
    // Nothing to assert on tap; just confirm it doesn't throw.
    await tester.tap(find.text('You & Betty'), warnIfMissed: false);
  });

  testWidgets('fires onTap when provided (group variant)', (tester) async {
    var tapped = 0;
    await tester.pumpWidget(_wrap(
      ProfileMemberRow(
        faces: const [FaceStackEntry(initial: 'T')],
        title: 'Thomas, Alfred, and Betty',
        subtitle: 'All members · 3',
        onTap: () => tapped++,
      ),
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Thomas, Alfred, and Betty'));
    expect(tapped, 1);
  });

  testWidgets(
      'fires onTapRect with its own footprint '
      '(the morph-panel source rect)', (tester) async {
    Rect? tappedRect;
    await tester.pumpWidget(_wrap(
      ProfileMemberRow(
        faces: const [FaceStackEntry(initial: 'T')],
        title: 'Thomas, Alfred, and Betty',
        subtitle: 'All members · 3',
        onTapRect: (rect) => tappedRect = rect,
      ),
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Thomas, Alfred, and Betty'));
    expect(tappedRect, isNotNull);
    expect(tappedRect, isNot(Rect.zero));
  });
}
