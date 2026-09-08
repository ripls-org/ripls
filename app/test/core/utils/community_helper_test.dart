import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/services/providers.dart' show CommunitiesState;

CommunityItem _community(String id) => CommunityItem(id: id, name: 'c-$id');

SharedCommunity _shared(String id) =>
    SharedCommunity(communityId: id, communityName: 'shared-$id');

void main() {
  group('CommunityHelper.invitableSharedCommunities', () {
    test(
        'returns the shared list as-is — the server already scopes it to '
        'communities the caller is a member of', () {
      final state = CommunitiesState(
        communities: [_community('a'), _community('b')],
      );
      final shared = [_shared('a'), _shared('b'), _shared('c')];

      final invitable =
          CommunityHelper.invitableSharedCommunities(shared, state);

      expect(invitable.map((c) => c.communityId).toList(), ['a', 'b', 'c']);
    });

    test(
        'does NOT drop communities missing from the (lagging) client list — a '
        'freshly-created per-item community is still invitable', () {
      final state = CommunitiesState(
        communities: [_community('x'), _community('y')],
      );
      final shared = [_shared('a'), _shared('b')];

      final invitable =
          CommunityHelper.invitableSharedCommunities(shared, state);

      expect(invitable.map((c) => c.communityId).toList(), ['a', 'b']);
    });

    test('returns the shared list even when the client has no communities '
        'loaded yet', () {
      const state = CommunitiesState();
      final shared = [_shared('a')];

      final invitable =
          CommunityHelper.invitableSharedCommunities(shared, state);

      expect(invitable.map((c) => c.communityId).toList(), ['a']);
    });

    test('returns empty when shared list is empty', () {
      final state = CommunitiesState(
        communities: [_community('a')],
      );

      final invitable =
          CommunityHelper.invitableSharedCommunities(const [], state);

      expect(invitable, isEmpty);
    });

    test('preserves the order of the input shared list', () {
      final state = CommunitiesState(
        communities: [_community('a'), _community('b'), _community('c')],
      );
      final shared = [_shared('z'), _shared('b'), _shared('a')];

      final invitable =
          CommunityHelper.invitableSharedCommunities(shared, state);

      expect(invitable.map((c) => c.communityId).toList(), ['z', 'b', 'a']);
    });
  });
}
