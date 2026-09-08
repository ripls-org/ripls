import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

List<User> _users(int n) =>
    [for (int i = 0; i < n; i++) User(name: 'Person$i')];

Future<void> _pump(WidgetTester tester, List<User> members) async {
  await tester.pumpWidget(
    ProviderScope(
      child: MaterialApp(
        home: Scaffold(
          body: Center(
            child: GroupAvatar(
              members: members,
              diameter: 48,
              ringColor: Colors.black,
            ),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  group('GroupAvatar', () {
    testWidgets('renders one circle per member up to four', (tester) async {
      await _pump(tester, _users(3));
      expect(find.byType(UserAvatar), findsNWidgets(3));
    });

    testWidgets('caps the cluster at four circles (extras dropped)',
        (tester) async {
      await _pump(tester, _users(6));
      expect(find.byType(UserAvatar), findsNWidgets(4));
    });

    testWidgets('a single member renders one full circle', (tester) async {
      await _pump(tester, _users(1));
      expect(find.byType(UserAvatar), findsOneWidget);
    });

    testWidgets('no members renders no avatars', (tester) async {
      await _pump(tester, const []);
      expect(find.byType(UserAvatar), findsNothing);
    });
  });

  group('groupAvatarMembers', () {
    test('places the viewer first, then the preview names', () {
      final viewer = User(id: 'me', name: 'Me');
      final members = groupAvatarMembers(
        viewer: viewer,
        memberPreviewFirstNames: ['Ada', 'Sam'],
      );
      expect(members.map((u) => u.name), ['Me', 'Ada', 'Sam']);
    });

    test('omits a null viewer and skips blank preview names', () {
      final members = groupAvatarMembers(
        viewer: null,
        memberPreviewFirstNames: ['Ada', '  ', ''],
      );
      expect(members.map((u) => u.name), ['Ada']);
    });
  });
}
