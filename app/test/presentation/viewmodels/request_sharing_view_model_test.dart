import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/request_sharing_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'request_sharing_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository, RequestRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late MockRequestRepository mockRequestRepo;
  late ProviderContainer container;

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockRequestRepo = MockRequestRepository();
    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        requestRepositoryProvider.overrideWithValue(mockRequestRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepo);
    reset(mockRequestRepo);
  });

  group('initial state', () {
    test('starts with empty state', () {
      final state = container.read(requestSharingProvider);
      expect(state.userCommunities, isEmpty);
      expect(state.sharedCommunityIds, isEmpty);
      expect(state.isLoadingCommunities, isFalse);
      expect(state.loadingCommunityIds, isEmpty);
    });
  });

  group('loadUserCommunities', () {
    test('happy path: populates userCommunities', () async {
      final communities = [
        CommunityItem(id: 'c1', name: 'Community 1'),
        CommunityItem(id: 'c2', name: 'Community 2'),
      ];
      when(mockCommunityRepo.listUserCommunities())
          .thenAnswer((_) async => communities);

      final notifier = container.read(requestSharingProvider.notifier);
      await notifier.loadUserCommunities();

      final state = container.read(requestSharingProvider);
      expect(state.userCommunities, hasLength(2));
      expect(state.isLoadingCommunities, isFalse);
    });

    test('error path: sets communitiesError', () async {
      when(mockCommunityRepo.listUserCommunities())
          .thenThrow(Exception('Load failed'));

      final notifier = container.read(requestSharingProvider.notifier);
      await notifier.loadUserCommunities();

      final state = container.read(requestSharingProvider);
      expect(state.isLoadingCommunities, isFalse);
      expect(state.communitiesError, isNotNull);
    });
  });

  group('setSharedCommunities', () {
    test('updates sharedCommunityIds', () {
      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1', 'c2']);

      final state = container.read(requestSharingProvider);
      expect(state.sharedCommunityIds, ['c1', 'c2']);
    });
  });

  group('isSharedWithCommunity / isLoadingCommunity', () {
    test('returns correct presence/loading states', () {
      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1']);

      final state = container.read(requestSharingProvider);
      expect(state.isSharedWithCommunity('c1'), isTrue);
      expect(state.isSharedWithCommunity('c2'), isFalse);
      expect(state.isLoadingCommunity('c1'), isFalse);
    });
  });

  group('shareWithCommunity', () {
    test('happy path: adds communityId to sharedCommunityIds', () async {
      final updatedRequest = Request(sharedCommunityIds: ['c1']);
      when(mockRequestRepo.shareRequest(
        requestId: anyNamed('requestId'),
        communityIds: anyNamed('communityIds'),
      )).thenAnswer((_) async => updatedRequest);

      final notifier = container.read(requestSharingProvider.notifier);
      await notifier.shareWithCommunity('req1', 'c1');

      final state = container.read(requestSharingProvider);
      expect(state.sharedCommunityIds, contains('c1'));
      expect(state.loadingCommunityIds, isEmpty);
    });

    test('no-op when already shared', () async {
      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1']);
      await notifier.shareWithCommunity('req1', 'c1');

      verifyNever(mockRequestRepo.shareRequest(
        requestId: anyNamed('requestId'),
        communityIds: anyNamed('communityIds'),
      ));
    });

    test('error path: sets sharingError and rethrows', () async {
      when(mockRequestRepo.shareRequest(
        requestId: anyNamed('requestId'),
        communityIds: anyNamed('communityIds'),
      )).thenThrow(Exception('Share failed'));

      final notifier = container.read(requestSharingProvider.notifier);
      await expectLater(
        notifier.shareWithCommunity('req1', 'c1'),
        throwsA(isA<Exception>()),
      );

      final state = container.read(requestSharingProvider);
      expect(state.sharingError, isNotNull);
      expect(state.loadingCommunityIds, isEmpty);
    });
  });

  group('unshareFromCommunity', () {
    test('happy path: removes communityId from sharedCommunityIds', () async {
      when(mockRequestRepo.unshareRequest(
        requestId: anyNamed('requestId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});

      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1', 'c2']);
      await notifier.unshareFromCommunity('req1', 'c1');

      final state = container.read(requestSharingProvider);
      expect(state.sharedCommunityIds, isNot(contains('c1')));
      expect(state.sharedCommunityIds, contains('c2'));
    });

    test('throws when unsharing from last community', () async {
      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1']);

      await expectLater(
        notifier.unshareFromCommunity('req1', 'c1'),
        throwsA(isA<Exception>()),
      );
    });

    test('no-op when not shared with community', () async {
      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1', 'c2']);
      await notifier.unshareFromCommunity('req1', 'c3');

      verifyNever(mockRequestRepo.unshareRequest(
        requestId: anyNamed('requestId'),
        communityId: anyNamed('communityId'),
      ));
    });

    test('error path: sets sharingError and rethrows', () async {
      when(mockRequestRepo.unshareRequest(
        requestId: anyNamed('requestId'),
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('Unshare failed'));

      final notifier = container.read(requestSharingProvider.notifier);
      notifier.setSharedCommunities(['c1', 'c2']);

      await expectLater(
        notifier.unshareFromCommunity('req1', 'c1'),
        throwsA(isA<Exception>()),
      );

      final state = container.read(requestSharingProvider);
      expect(state.sharingError, isNotNull);
    });
  });

  group('clearError', () {
    test('clears all error messages', () async {
      when(mockCommunityRepo.listUserCommunities())
          .thenThrow(Exception('Error'));
      final notifier = container.read(requestSharingProvider.notifier);
      await notifier.loadUserCommunities();
      notifier.clearError();

      final state = container.read(requestSharingProvider);
      expect(state.sharingError, isNull);
      expect(state.communitiesError, isNull);
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight completes without throwing', () async {
      when(mockCommunityRepo.listUserCommunities()).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return [];
      });

      final notifier = container.read(requestSharingProvider.notifier);
      final future = notifier.loadUserCommunities();
      container.dispose();
      await expectLater(future, completes);
    });
  });
}
