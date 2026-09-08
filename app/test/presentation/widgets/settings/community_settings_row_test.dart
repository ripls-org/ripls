import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/settings/community_settings_row.dart';

import '../../../helpers/l10n_helpers.dart';

/// The row's derivation is unit-tested in
/// `test/core/utils/community_display_test.dart`; these are the smoke tests
/// that the widget actually renders it — the regression #2937 reported was a
/// blank row, so "the label is non-empty" is the assertion that matters.
void main() {
  Future<void> pump(WidgetTester tester, CommunityItem community) {
    return tester.pumpWidget(
      ProviderScope(
        child: localizedApp(
          Scaffold(
            body: CommunitySettingsRow(community: community, onTap: () {}),
          ),
        ),
      ),
    );
  }

  testWidgets('a named community renders its name', (tester) async {
    await pump(
      tester,
      CommunityItem(id: 'c1', name: 'Trail Crew', memberCount: 5),
    );

    expect(find.text('Trail Crew'), findsOneWidget);
  });

  testWidgets('a nameless group renders its members, not a blank row',
      (tester) async {
    await pump(
      tester,
      CommunityItem(
        id: 'g1',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
      ),
    );

    expect(find.text('You, Alex, and Sam'), findsOneWidget);
    // The blank-name bug also produced a generic "C" circle.
    expect(find.text('C'), findsNothing);
    expect(find.byType(GroupAvatar), findsOneWidget);
  });

  testWidgets('a nameless group is identified by the item that spawned it',
      (tester) async {
    await pump(
      tester,
      CommunityItem(
        id: 'g1',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
        originItemName: 'Dinner at Este',
      ),
    );

    expect(find.text('from Dinner at Este'), findsOneWidget);
  });

  testWidgets('a nameless group whose members have no names still labels',
      (tester) async {
    // The worst case on the wire: preview names all empty. The row must still
    // say something rather than falling back to blank.
    await pump(
      tester,
      CommunityItem(id: 'g1', name: '', memberCount: 4),
    );

    expect(find.text('4 members'), findsOneWidget);
  });

  testWidgets('the row is announced by its derived label', (tester) async {
    final handle = tester.ensureSemantics();
    await pump(
      tester,
      CommunityItem(
        id: 'g1',
        name: '',
        memberCount: 2,
        memberPreviewFirstNames: ['Alex'],
      ),
    );

    // Blank rows announced nothing at all to TalkBack/VoiceOver — no lint
    // catches a runtime-empty label, only literal ones.
    expect(
      find.bySemanticsLabel('You and Alex'),
      findsOneWidget,
    );
    handle.dispose();
  });
}
