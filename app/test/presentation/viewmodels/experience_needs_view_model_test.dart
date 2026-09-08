import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

import 'experience_needs_view_model_test.mocks.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'user-1', name: 'Tester'),
      );
}

@GenerateMocks([ExperienceRepository])
void main() {
  const experienceId = 'exp-abc';

  // Helper builders
  ExperienceNeedResponse buildNeed({
    String id = 'need1',
    String name = 'Red wine',
    int slots = 2,
    int slotsRemaining = 2,
  }) {
    return ExperienceNeedResponse(
      id: id,
      experienceId: experienceId,
      proposer: User(id: 'u1', name: 'Gary'),
      name: name,
      slots: slots,
      slotsRemaining: slotsRemaining,
    );
  }

  ExperienceContributionResponse buildContribution({
    String id = 'contrib1',
    String title = 'Salad',
  }) {
    return ExperienceContributionResponse(
      id: id,
      experienceId: experienceId,
      contributor: User(id: 'u2', name: 'Ana'),
      title: title,
    );
  }

  ListExperienceNeedsAndContributionsResponse buildResponse({
    List<ExperienceNeedResponse>? needs,
    List<ExperienceContributionResponse>? contributions,
  }) {
    return ListExperienceNeedsAndContributionsResponse(
      needs: needs ?? [],
      contributions: contributions ?? [],
    );
  }

  late MockExperienceRepository mockRepo;
  late ProviderContainer container;

  setUp(() {
    mockRepo = MockExperienceRepository();
    container = ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(mockRepo),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
      ],
    );
  });

  tearDown(() => container.dispose());

  group('ExperienceNeedsNotifier — initial load', () {
    test('loads needs and contributions on build', () async {
      final need = buildNeed();
      final contrib = buildContribution();
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse(needs: [need], contributions: [contrib]));

      // Read the provider to trigger build.
      container.read(experienceNeedsProvider(experienceId));

      // Wait for async _load to settle.
      await container
          .read(experienceNeedsProvider(experienceId).notifier)
          .refresh();

      final state = container.read(experienceNeedsProvider(experienceId));
      expect(state.isLoading, false);
      expect(state.needs, hasLength(1));
      expect(state.needs.first.name, 'Red wine');
      expect(state.contributions, hasLength(1));
      expect(state.contributions.first.title, 'Salad');
    });

    test('sets errorMessage on load failure', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenThrow(Exception('network error'));

      container.read(experienceNeedsProvider(experienceId));
      await container
          .read(experienceNeedsProvider(experienceId).notifier)
          .refresh();

      final state = container.read(experienceNeedsProvider(experienceId));
      expect(state.isLoading, false);
      expect(state.hasError, true);
      expect(state.error, isNotNull);
    });
  });

  group('ExperienceNeedsNotifier — addNeed', () {
    test('calls repo and refreshes', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.addNeed(
        experienceId: anyNamed('experienceId'),
        name: anyNamed('name'),
        note: anyNamed('note'),
        slots: anyNamed('slots'),
      )).thenAnswer((_) async => buildNeed());
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse(needs: [buildNeed()]));

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);
      await notifier.addNeed(name: 'Red wine', slots: 2);

      verify(mockRepo.addNeed(
        experienceId: experienceId,
        name: 'Red wine',
        note: null,
        slots: 2,
      )).called(1);
      verify(mockRepo.refreshNeedsAndContributions(experienceId)).called(greaterThan(0));

      final state = container.read(experienceNeedsProvider(experienceId));
      expect(state.isMutating, false);
      expect(state.mutationError, isNull);
    });

    test('sets mutationError on failure', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.addNeed(
        experienceId: anyNamed('experienceId'),
        name: anyNamed('name'),
        note: anyNamed('note'),
        slots: anyNamed('slots'),
      )).thenThrow(Exception('server error'));

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);
      await notifier.addNeed(name: 'Wine');

      final state = container.read(experienceNeedsProvider(experienceId));
      expect(state.isMutating, false);
      expect(state.mutationError, isNotNull);
    });
  });

  group('ExperienceNeedsNotifier — claimNeed', () {
    test('calls repo with needId and refreshes', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.claimNeed(
        needId: anyNamed('needId'),
        experienceId: anyNamed('experienceId'),
        note: anyNamed('note'),
        gearId: anyNamed('gearId'),
      )).thenAnswer((_) async => buildContribution());
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse(contributions: [buildContribution()]));

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);
      await notifier.claimNeed('need1', note: 'Tiramisu');

      verify(mockRepo.claimNeed(
        needId: 'need1',
        experienceId: experienceId,
        note: 'Tiramisu',
        gearId: null,
      )).called(1);
    });

    test('forwards optional gearId to the repository', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.claimNeed(
        needId: anyNamed('needId'),
        experienceId: anyNamed('experienceId'),
        note: anyNamed('note'),
        gearId: anyNamed('gearId'),
      )).thenAnswer((_) async => buildContribution());
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse(contributions: [buildContribution()]));

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);
      await notifier.claimNeed('need1', gearId: 'gear-42');

      verify(mockRepo.claimNeed(
        needId: 'need1',
        experienceId: experienceId,
        note: null,
        gearId: 'gear-42',
      )).called(1);
    });
  });

  group('ExperienceNeedsNotifier — removeContribution', () {
    test('calls repo and refreshes', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.removeContribution(
        contributionId: anyNamed('contributionId'),
        experienceId: anyNamed('experienceId'),
      )).thenAnswer((_) async {});
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);
      await notifier.removeContribution('contrib1');

      verify(mockRepo.removeContribution(
        contributionId: 'contrib1',
        experienceId: experienceId,
      )).called(1);

      final state = container.read(experienceNeedsProvider(experienceId));
      expect(state.isMutating, false);
    });
  });

  group('ExperienceNeedsNotifier — live cross-user updates (#2724)', () {
    test(
        'a content invalidation (e.g. a claim event routed off the community '
        'stream) force-refreshes needs + contributions', () async {
      // Initial load: one open need, nobody bringing anything yet.
      when(mockRepo.listNeedsAndContributions(experienceId)).thenAnswer(
          (_) async => buildResponse(needs: [buildNeed(name: 'Dessert')]));
      // The refresh triggered by the invalidation returns the post-claim
      // state: another guest claimed the dessert while this pane was open.
      when(mockRepo.refreshNeedsAndContributions(experienceId)).thenAnswer(
          (_) async => buildResponse(
              needs: [buildNeed(name: 'Dessert', slotsRemaining: 0)],
              contributions: [buildContribution(title: 'Dessert')]));

      // Keep the autoDispose provider alive across the debounce window.
      final sub = container.listen(
          experienceNeedsProvider(experienceId), (_, _) {});
      addTearDown(sub.close);

      // Let the initial (microtask-scheduled) load settle.
      await Future<void>.delayed(Duration.zero);
      var state = container.read(experienceNeedsProvider(experienceId));
      expect(state.contributions, isEmpty);
      expect(state.needs.single.slotsRemaining, 2);

      // A claim event arrives via the event router → content invalidation.
      container.read(contentCacheInvalidationProvider.notifier).notify();

      // Nothing yet inside the debounce window…
      verifyNever(mockRepo.refreshNeedsAndContributions(experienceId));

      // …then exactly one force-fetch after it, and the pane's data flips.
      await Future<void>.delayed(const Duration(milliseconds: 400));
      verify(mockRepo.refreshNeedsAndContributions(experienceId)).called(1);
      state = container.read(experienceNeedsProvider(experienceId));
      expect(state.contributions.single.title, 'Dessert');
      expect(state.needs.single.slotsRemaining, 0);
    });

    test('an invalidation burst coalesces into a single refresh', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());

      final sub = container.listen(
          experienceNeedsProvider(experienceId), (_, _) {});
      addTearDown(sub.close);
      await Future<void>.delayed(Duration.zero);

      final invalidations =
          container.read(contentCacheInvalidationProvider.notifier);
      invalidations.notify();
      invalidations.notify();
      invalidations.notify();

      await Future<void>.delayed(const Duration(milliseconds: 400));
      verify(mockRepo.refreshNeedsAndContributions(experienceId)).called(1);
    });

    test('disposal inside the debounce window does not throw', () async {
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async => buildResponse());

      container.read(experienceNeedsProvider(experienceId));
      await Future<void>.delayed(Duration.zero);

      container.read(contentCacheInvalidationProvider.notifier).notify();
      container.dispose();

      // Cross the debounce window — the cancelled timer must not fire into
      // the disposed notifier.
      await Future<void>.delayed(const Duration(milliseconds: 400));
      verifyNever(mockRepo.refreshNeedsAndContributions(experienceId));
    });
  });

  group('ExperienceNeedsNotifier — disposal safety', () {
    test('does not throw when disposed mid-refresh', () async {
      // First call: returns quickly; second (from refresh) never completes.
      var callCount = 0;
      when(mockRepo.listNeedsAndContributions(experienceId))
          .thenAnswer((_) async {
        callCount++;
        if (callCount > 1) {
          // Simulate a long-running request that outlives the provider.
          await Future.delayed(const Duration(seconds: 30));
          return buildResponse();
        }
        return buildResponse();
      });
      when(mockRepo.refreshNeedsAndContributions(experienceId))
          .thenAnswer((_) async {
        await Future.delayed(const Duration(seconds: 30));
        return buildResponse();
      });

      final notifier =
          container.read(experienceNeedsProvider(experienceId).notifier);

      // Start a refresh but dispose immediately — must not throw.
      final refreshFuture = notifier.refresh();
      container.dispose();

      await expectLater(refreshFuture, completes);
    });
  });
}
