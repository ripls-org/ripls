import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/models/avatar_status_badge.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

void main() {
  Widget buildAvatar({
    required AvatarStatusBadge? badge,
    Color? surfaceColor,
  }) {
    final user = User()
      ..name = 'Test User'
      ..mediaId = '';
    return ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          body: UserAvatar(
            user: user,
            radius: 20,
            statusBadge: badge,
            badgeSurfaceColor: surfaceColor,
          ),
        ),
      ),
    );
  }

  group('AvatarStatusBadge', () {
    testWidgets('renders Positioned badge when statusBadge is provided',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: AvatarStatusBadge.going));
      await tester.pump();

      // Badge overlay uses a Positioned widget.
      expect(find.byType(Positioned), findsWidgets);
    });

    testWidgets('no Positioned overlay when statusBadge is null',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: null));
      await tester.pump();

      // Without badge there is no Positioned widget.
      expect(find.byType(Positioned), findsNothing);
    });

    testWidgets('badge uses correct icon for going status',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: AvatarStatusBadge.going));
      await tester.pump();

      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('badge uses correct icon for interested status',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: AvatarStatusBadge.interested));
      await tester.pump();

      expect(find.byIcon(Icons.pan_tool_outlined), findsOneWidget);
    });

    testWidgets('badge uses correct icon for owner status',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: AvatarStatusBadge.owner));
      await tester.pump();

      expect(find.byIcon(Icons.star), findsOneWidget);
    });

    testWidgets('badge uses correct icon for maybe status',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildAvatar(badge: AvatarStatusBadge.maybe));
      await tester.pump();

      expect(find.byIcon(Icons.question_mark), findsOneWidget);
    });
  });

  group('badgeColor', () {
    test('interested returns coral', () {
      final context = _FakeContext();
      expect(
        badgeColor(AvatarStatusBadge.interested, context as BuildContext),
        isNot(equals(Colors.transparent)),
      );
    });
  });

  group('badgeIcon', () {
    test('going returns check icon', () {
      expect(badgeIcon(AvatarStatusBadge.going), equals(Icons.check));
    });

    test('helping returns check icon', () {
      expect(badgeIcon(AvatarStatusBadge.helping), equals(Icons.check));
    });

    test('selected returns check icon', () {
      expect(badgeIcon(AvatarStatusBadge.selected), equals(Icons.check));
    });

    test('maybe returns question_mark icon', () {
      expect(badgeIcon(AvatarStatusBadge.maybe), equals(Icons.question_mark));
    });

    test('owner returns star icon', () {
      expect(badgeIcon(AvatarStatusBadge.owner), equals(Icons.star));
    });

    test('organizer returns star icon', () {
      expect(badgeIcon(AvatarStatusBadge.organizer), equals(Icons.star));
    });

    test('requester returns star icon', () {
      expect(badgeIcon(AvatarStatusBadge.requester), equals(Icons.star));
    });

    test('interested returns hand icon', () {
      expect(
        badgeIcon(AvatarStatusBadge.interested),
        equals(Icons.pan_tool_outlined),
      );
    });
  });
}

// Minimal fake BuildContext for unit tests that need it.
class _FakeContext extends Fake implements BuildContext {
  @override
  T? findAncestorWidgetOfExactType<T extends Widget>() => null;
}
