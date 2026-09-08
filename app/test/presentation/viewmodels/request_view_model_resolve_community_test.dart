import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/presentation/viewmodels/request_view_model.dart';

void main() {
  group('resolveCommunityId', () {
    SharedCommunity sc(String id) => SharedCommunity(communityId: id);

    test('keeps supplied id when it is in shared communities', () {
      final result = resolveCommunityId(
        supplied: 'a',
        shared: [sc('a'), sc('b')],
      );
      expect(result, 'a');
    });

    test('falls back to first shared community when supplied is null', () {
      final result = resolveCommunityId(
        supplied: null,
        shared: [sc('a'), sc('b')],
      );
      expect(result, 'a');
    });

    test('regression #1547: corrects supplied id not in shared list', () {
      // Reproduces the prod failure: screen pinned communityId b42c4991
      // (user's stale active community), but the request was only shared
      // with a1611f6f. Without correction, OfferToFulfill 404s.
      final result = resolveCommunityId(
        supplied: 'b42c4991-2daf-44df-bb7f-2cb9614ee530',
        shared: [sc('a1611f6f-4002-4fa1-82b4-a1f725bbd2b2')],
      );
      expect(result, 'a1611f6f-4002-4fa1-82b4-a1f725bbd2b2');
    });

    test('returns null when no shared communities and no valid supplied', () {
      final result = resolveCommunityId(
        supplied: null,
        shared: const <SharedCommunity>[],
      );
      expect(result, isNull);
    });

    test('returns null when supplied is empty and no shared communities', () {
      final result = resolveCommunityId(
        supplied: '',
        shared: const <SharedCommunity>[],
      );
      expect(result, isNull);
    });

    test('falls back when supplied is empty string', () {
      final result = resolveCommunityId(
        supplied: '',
        shared: [sc('a')],
      );
      expect(result, 'a');
    });

    group('membership-aware resolution (#1705)', () {
      test('keeps supplied id when it is shared and user is a member', () {
        final result = resolveCommunityId(
          supplied: 'a',
          shared: [sc('a'), sc('b')],
          userMemberIds: ['a', 'c'],
        );
        expect(result, 'a');
      });

      test(
          'corrects supplied id to a community the user is a member of when '
          'the supplied one is shared but not a member-community', () {
        // Multi-community gear shared with {a, b}; user only in b.
        // Old behavior would have returned a (it is in shared) and then
        // permission_denied. Now we pick b.
        final result = resolveCommunityId(
          supplied: 'a',
          shared: [sc('a'), sc('b')],
          userMemberIds: ['b', 'c'],
        );
        expect(result, 'b');
      });

      test('falls back to first shared+member intersection when supplied is null',
          () {
        final result = resolveCommunityId(
          supplied: null,
          shared: [sc('a'), sc('b'), sc('c')],
          userMemberIds: ['c', 'b'],
        );
        // Iteration order matches the shared list, so b comes before c.
        expect(result, 'b');
      });

      test(
          'falls back to first shared community when intersection is empty '
          '(degraded but matches pre-membership behavior)', () {
        final result = resolveCommunityId(
          supplied: 'a',
          shared: [sc('a'), sc('b')],
          userMemberIds: ['x', 'y'],
        );
        expect(result, 'a');
      });

      test('empty userMemberIds is treated as "membership unknown"', () {
        // A defensive choice: when listUserCommunities fails or returns
        // empty, we should not aggressively reject every supplied id —
        // fall back to the pre-membership semantics.
        final result = resolveCommunityId(
          supplied: 'a',
          shared: [sc('a'), sc('b')],
          userMemberIds: const <String>[],
        );
        expect(result, 'a');
      });
    });
  });
}
