import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import 'request_needs_view_model_test.mocks.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'user-1', name: 'Tester'),
      );
}

@GenerateMocks([RequestRepository])
void main() {
  late MockRequestRepository mockRepo;
  late ProviderContainer container;

  const requestId = 'req1';

  ListRequestNeedsAndContributionsResponse emptyResponse() =>
      ListRequestNeedsAndContributionsResponse();

  setUp(() {
    mockRepo = MockRequestRepository();
    // Default: listNeedsAndContributions returns empty response
    when(mockRepo.listNeedsAndContributions(any))
        .thenAnswer((_) async => emptyResponse());
    when(mockRepo.refreshNeedsAndContributions(any))
        .thenAnswer((_) async => emptyResponse());

    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(mockRepo),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  group('initialize / _load', () {
    test('loads needs and contributions on build', () async {
      final response = ListRequestNeedsAndContributionsResponse(
        additionalAsks: ['Help needed'],
        offerIdeas: ['Bring tools'],
      );
      // refresh() force-fetches past the cache after consuming the
      // pending init-load future, so both code paths need the seeded
      // response. Pattern 2 in docs/client/caching.md: repo invalidates
      // → notifier re-reads via the force-fetch helper.
      when(mockRepo.listNeedsAndContributions(requestId))
          .thenAnswer((_) async => response);
      when(mockRepo.refreshNeedsAndContributions(requestId))
          .thenAnswer((_) async => response);

      // Read the provider (triggers build which schedules _load)
      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh(); // Awaits the initial load future, then force-fetches.

      final state = container.read(requestNeedsProvider(requestId));
      expect(state.isLoading, isFalse);
      expect(state.additionalAsks, ['Help needed']);
      expect(state.offerIdeas, ['Bring tools']);
    });

    test('error path: sets errorMessage on load failure', () async {
      when(mockRepo.listNeedsAndContributions(requestId))
          .thenThrow(Exception('Load failed'));
      // refresh() now force-fetches past the cache after the init load;
      // make the force-fetch fail too so the error state survives the
      // second pass.
      when(mockRepo.refreshNeedsAndContributions(requestId))
          .thenThrow(Exception('Load failed'));

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();

      final state = container.read(requestNeedsProvider(requestId));
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('addNeed', () {
    test('happy path: calls addNeed and refreshes', () async {
      when(mockRepo.addNeed(
        requestId: anyNamed('requestId'),
        name: anyNamed('name'),
        note: anyNamed('note'),
        slots: anyNamed('slots'),
      )).thenAnswer((_) async => RequestNeedResponse());

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.addNeed(name: 'Ladder');

      verify(mockRepo.addNeed(
        requestId: requestId,
        name: 'Ladder',
        note: null,
        slots: 1,
      )).called(1);
      final state = container.read(requestNeedsProvider(requestId));
      expect(state.isMutating, isFalse);
      expect(state.mutationError, isNull);
    });

    test('error path: sets mutationError', () async {
      when(mockRepo.addNeed(
        requestId: anyNamed('requestId'),
        name: anyNamed('name'),
        note: anyNamed('note'),
        slots: anyNamed('slots'),
      )).thenThrow(Exception('Add failed'));

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.addNeed(name: 'Ladder');

      final state = container.read(requestNeedsProvider(requestId));
      expect(state.isMutating, isFalse);
      expect(state.mutationError, isNotNull);
    });
  });

  group('removeNeed', () {
    test('happy path: calls removeNeed and refreshes', () async {
      when(mockRepo.removeNeed(
        needId: anyNamed('needId'),
        requestId: anyNamed('requestId'),
      )).thenAnswer((_) async {});

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.removeNeed('need1');

      verify(mockRepo.removeNeed(
        needId: 'need1',
        requestId: requestId,
      )).called(1);
    });

    test('error path: sets mutationError', () async {
      when(mockRepo.removeNeed(
        needId: anyNamed('needId'),
        requestId: anyNamed('requestId'),
      )).thenThrow(Exception('Remove failed'));

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.removeNeed('need1');

      final state = container.read(requestNeedsProvider(requestId));
      expect(state.mutationError, isNotNull);
    });
  });

  group('claimNeed', () {
    test('happy path: calls claimNeed and refreshes', () async {
      when(mockRepo.claimNeed(
        needId: anyNamed('needId'),
        requestId: anyNamed('requestId'),
        communityId: anyNamed('communityId'),
        note: anyNamed('note'),
      )).thenAnswer((_) async => RequestContributionResponse());

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.claimNeed('need1', communityId: 'comm1');

      verify(mockRepo.claimNeed(
        needId: 'need1',
        requestId: requestId,
        communityId: 'comm1',
        note: null,
      )).called(1);
    });

    test('error path: sets mutationError', () async {
      when(mockRepo.claimNeed(
        needId: anyNamed('needId'),
        requestId: anyNamed('requestId'),
        communityId: anyNamed('communityId'),
        note: anyNamed('note'),
      )).thenThrow(Exception('Claim failed'));

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.claimNeed('need1', communityId: 'comm1');

      final state = container.read(requestNeedsProvider(requestId));
      expect(state.mutationError, isNotNull);
    });
  });

  group('unclaimNeed', () {
    test('happy path: calls unclaimNeed and refreshes', () async {
      when(mockRepo.unclaimNeed(
        contributionId: anyNamed('contributionId'),
        requestId: anyNamed('requestId'),
      )).thenAnswer((_) async {});

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.unclaimNeed('contrib1');

      verify(mockRepo.unclaimNeed(
        contributionId: 'contrib1',
        requestId: requestId,
      )).called(1);
    });
  });

  group('addContribution / editContribution / removeContribution', () {
    test('addContribution happy path', () async {
      when(mockRepo.addContribution(
        requestId: anyNamed('requestId'),
        title: anyNamed('title'),
        description: anyNamed('description'),
      )).thenAnswer((_) async => RequestContributionResponse());

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.addContribution(title: 'Bring tools');

      verify(mockRepo.addContribution(
        requestId: requestId,
        title: 'Bring tools',
        description: null,
      )).called(1);
    });

    test('removeContribution happy path', () async {
      when(mockRepo.removeContribution(
        contributionId: anyNamed('contributionId'),
        requestId: anyNamed('requestId'),
      )).thenAnswer((_) async {});

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      await notifier.refresh();
      await notifier.removeContribution('contrib1');

      verify(mockRepo.removeContribution(
        contributionId: 'contrib1',
        requestId: requestId,
      )).called(1);
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight completes without throwing', () async {
      when(mockRepo.listNeedsAndContributions(requestId))
          .thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return emptyResponse();
      });

      final notifier = container.read(requestNeedsProvider(requestId).notifier);
      final future = notifier.refresh();
      container.dispose();
      await expectLater(future, completes);
    });
  });
}
