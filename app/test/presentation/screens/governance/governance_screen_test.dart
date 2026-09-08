import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/screens/governance/governance_screen.dart';

void main() {
  group('GovernanceScreen', () {
    test('accepts explicit community parameter', () {
      final community = CommunityItem(
        id: 'community-2',
        name: 'Second Community',
        description: 'A second community',
      );

      final screen = GovernanceScreen(community: community);

      expect(screen.community, equals(community));
      expect(screen.community!.id, equals('community-2'));
    });

    test('community parameter is null when not provided', () {
      const screen = GovernanceScreen();

      expect(screen.community, isNull);
    });

    test('two screens with different communities are distinct', () {
      final community1 = CommunityItem(
        id: 'community-1',
        name: 'First Community',
      );
      final community2 = CommunityItem(
        id: 'community-2',
        name: 'Second Community',
      );

      final screen1 = GovernanceScreen(community: community1);
      final screen2 = GovernanceScreen(community: community2);

      expect(screen1.community!.id, equals('community-1'));
      expect(screen2.community!.id, equals('community-2'));
      expect(screen1.community!.id, isNot(equals(screen2.community!.id)));
    });
  });
}
