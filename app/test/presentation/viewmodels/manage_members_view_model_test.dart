import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/presentation/viewmodels/manage_members_view_model.dart';
import 'package:ripls/services/community_service.dart'
    show CommunityMember, User;
import 'package:ripls/services/providers.dart';

import 'manage_members_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository, ProvisionalUserRepository])
void main() {
  group('ManageMembersNotifier', () {
    late ProviderContainer container;
    late MockCommunityRepository mockCommunityRepo;
    late MockProvisionalUserRepository mockProvisionalRepo;

    final communityId = 'community-1';

    setUp(() {
      mockCommunityRepo = MockCommunityRepository();
      mockProvisionalRepo = MockProvisionalUserRepository();
      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
          provisionalUserRepositoryProvider.overrideWithValue(
            mockProvisionalRepo,
          ),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('initial state', () {
      test('starts empty with no loading flags', () {
        final state = container.read(manageMembersProvider);

        expect(state.members, isEmpty);
        expect(state.provisionalUsers, isEmpty);
        expect(state.isLoadingMembers, false);
        expect(state.isLoadingProvisionalUsers, false);
        expect(state.error, isNull);
      });
    });

    group('load', () {
      test('populates members and provisional users on success', () async {
        final user = CommunityMember(user: User()..name = 'Alice');
        final prov = ProvisionalUser()
          ..id = 's1'
          ..name = 'Bob Provisional'
          ..isClaimed = false;

        when(
          mockCommunityRepo.listCommunityUsers(communityId),
        ).thenAnswer((_) async => [user]);
        when(
          mockProvisionalRepo.listProvisionalUsers(communityId),
        ).thenAnswer((_) async => [prov]);

        await container.read(manageMembersProvider.notifier).load(communityId);

        final state = container.read(manageMembersProvider);
        expect(state.members, hasLength(1));
        expect(state.members.first.user.name, 'Alice');
        expect(state.provisionalUsers, hasLength(1));
        expect(state.provisionalUsers.first.name, 'Bob Provisional');
        expect(state.isLoadingMembers, false);
        expect(state.isLoadingProvisionalUsers, false);
      });

      test('sets errorMessage when members load fails', () async {
        when(
          mockCommunityRepo.listCommunityUsers(communityId),
        ).thenThrow(Exception('network error'));
        when(
          mockProvisionalRepo.listProvisionalUsers(communityId),
        ).thenAnswer((_) async => []);

        await container.read(manageMembersProvider.notifier).load(communityId);

        final state = container.read(manageMembersProvider);
        expect(state.error, isNotNull);
        expect(state.isLoadingMembers, false);
      });
    });

    group('getInviteLink', () {
      test('stores invite link response in state', () async {
        final linkResponse = GetProvisionalUserInviteLinkResponse()
          ..shortCode = 'ABC12345'
          ..inviteUrl = 'https://test.example.com/go/ABC12345';

        when(
          mockProvisionalRepo.getInviteLink(
            communityId: communityId,
            provisionalUserId: 's1',
          ),
        ).thenAnswer((_) async => linkResponse);

        await container
            .read(manageMembersProvider.notifier)
            .getInviteLink(communityId: communityId, provisionalUserId: 's1');

        final state = container.read(manageMembersProvider);
        expect(state.pendingInviteLink, isNotNull);
        expect(state.pendingInviteLink!.shortCode, 'ABC12345');
        expect(state.inviteLinkProvisionalUserId, 's1');
      });
    });

    group('clearInviteLink', () {
      test('clears pending invite link and provisional user id', () async {
        final linkResponse = GetProvisionalUserInviteLinkResponse()
          ..shortCode = 'XYZ'
          ..inviteUrl = 'https://test.example.com/go/XYZ';

        when(
          mockProvisionalRepo.getInviteLink(
            communityId: communityId,
            provisionalUserId: 's1',
          ),
        ).thenAnswer((_) async => linkResponse);

        await container
            .read(manageMembersProvider.notifier)
            .getInviteLink(communityId: communityId, provisionalUserId: 's1');

        container.read(manageMembersProvider.notifier).clearInviteLink();

        final state = container.read(manageMembersProvider);
        expect(state.pendingInviteLink, isNull);
        expect(state.inviteLinkProvisionalUserId, isNull);
      });
    });

    group('clearError', () {
      test('clears error message', () async {
        when(
          mockCommunityRepo.listCommunityUsers(communityId),
        ).thenThrow(Exception('fail'));
        when(
          mockProvisionalRepo.listProvisionalUsers(communityId),
        ).thenAnswer((_) async => []);

        await container.read(manageMembersProvider.notifier).load(communityId);

        container.read(manageMembersProvider.notifier).clearError();

        expect(container.read(manageMembersProvider).error, isNull);
      });
    });

    group('disposal safety', () {
      test('does not throw after container is disposed', () {
        final localContainer = ProviderContainer(
          overrides: [
            communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
            provisionalUserRepositoryProvider.overrideWithValue(
              mockProvisionalRepo,
            ),
          ],
        );

        // Read the notifier to instantiate it, then dispose immediately.
        localContainer.read(manageMembersProvider.notifier);
        expect(() => localContainer.dispose(), returnsNormally);
      });
    });
  });
}
