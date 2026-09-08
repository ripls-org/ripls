import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/fulfill_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/services/providers.dart';

import '../../helpers/fake_async_helpers.dart';
import 'fulfill_modal_view_model_test.mocks.dart';

@GenerateMocks([RequestRepository, ImpactMetricsRepository])
void main() {
  late MockRequestRepository mockRequestRepo;
  late MockImpactMetricsRepository mockImpactRepo;

  const testRequestId = 'req-1';
  const testCommunityId = 'comm-1';

  ImpactEstimate makeImpact({required int groupSize}) {
    return ImpactEstimate(
      qualityTime: QualityTimeEstimate(
        qualityTimeMinutes: Estimate(mean: groupSize * 60.0),
        attributes: QualityTimeAttributes(groupSize: groupSize),
      ),
    );
  }

  setUp(() {
    mockRequestRepo = MockRequestRepository();
    mockImpactRepo = MockImpactMetricsRepository();

    // Default stubs — concrete tests override as needed.
    when(
      mockRequestRepo.getStats(testRequestId, communityId: testCommunityId),
    ).thenAnswer((_) async => GetRequestStatsResponse(
          potentialImpact: makeImpact(groupSize: 2),
        ));
    when(
      mockRequestRepo.previewRequestImpact(
        requestId: anyNamed('requestId'),
        confirmedHelperIds: anyNamed('confirmedHelperIds'),
        confirmedHelperCount: anyNamed('confirmedHelperCount'),
      ),
    ).thenAnswer((inv) async {
      final count = inv.namedArguments[#confirmedHelperCount] as int;
      // Group size = host (1) + helpers, mirroring the server logic.
      return makeImpact(groupSize: count + 1);
    });
    when(mockImpactRepo.draftImpact(requestId: testRequestId))
        .thenAnswer((_) async => makeImpact(groupSize: 2));
  });

  ProviderContainer makeContainer() {
    final container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(mockRequestRepo),
        impactMetricsRepositoryProvider.overrideWithValue(mockImpactRepo),
      ],
    );
    // The fulfill modal and impact-draft providers are autoDispose family —
    // a one-shot `read` would let them tear down between calls. Subscribing
    // mirrors the production setup where the modal's build watches them.
    container.listen(fulfillModalProvider(testRequestId), (_, _) {});
    container.listen(impactDraftProvider(testRequestId), (_, _) {});
    return container;
  }

  User makeUser(String id, String name) => User(id: id, name: name);

  group('FulfillModalNotifier — contributor pre-population', () {
    test('contributors-only (no offerers): contributors appear confirmed', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);
        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);

        final carol = makeUser('carol', 'Carol');
        notifier.initialize(
          const [],
          contributors: [carol],
        );

        final state = container.read(fulfillModalProvider(testRequestId));
        expect(state.offerers.map((u) => u.id), contains('carol'));
        expect(state.confirmedIds, contains('carol'));
      });
    });

    test('contributors overlapping offerers are deduped', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);
        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);

        final alice = makeUser('alice', 'Alice');
        notifier.initialize(
          [alice], // alice is already an offerer
          contributors: [alice], // alice also contributed — must not duplicate
        );

        final state = container.read(fulfillModalProvider(testRequestId));
        // Alice appears exactly once in the list.
        expect(state.offerers.where((u) => u.id == 'alice').length, 1);
        expect(state.confirmedIds, contains('alice'));
      });
    });

    test('contributors + offerers: both appear confirmed, no duplicates', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);
        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);

        final alice = makeUser('alice', 'Alice');
        final bob = makeUser('bob', 'Bob');
        notifier.initialize(
          [alice],
          contributors: [bob],
        );

        final state = container.read(fulfillModalProvider(testRequestId));
        expect(state.offerers.map((u) => u.id), containsAll(['alice', 'bob']));
        expect(state.offerers.length, 2);
        expect(state.confirmedIds, containsAll(['alice', 'bob']));
      });
    });

    test('empty contributors list preserves current behavior', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);
        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);

        final alice = makeUser('alice', 'Alice');
        notifier.initialize(
          [alice],
          // contributors defaults to []
        );

        final state = container.read(fulfillModalProvider(testRequestId));
        expect(state.offerers.map((u) => u.id), equals(['alice']));
        expect(state.confirmedIds, equals({'alice'}));
      });
    });
  });

  group('FulfillModalNotifier — live helper-toggle preview', () {
    test('confirming a helper triggers debounced preview that updates state.impactEstimate', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);

        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);
        notifier.initialize(
          const [],
        );

        // Confirm one helper.
        notifier.toggleHelper('helper-1');

        // Advance past the 250ms debounce window.
        async.elapse(const Duration(milliseconds: 250));
        async.flushMicrotasks();

        // Server received the live count (1 helper).
        verify(
          mockRequestRepo.previewRequestImpact(
            requestId: testRequestId,
            confirmedHelperIds: ['helper-1'],
            confirmedHelperCount: 1,
          ),
        ).called(greaterThanOrEqualTo(1));

        // The fulfill modal's state.impactEstimate now carries the
        // group-size-adjusted QT (host + 1 helper = 2).
        final state = container.read(fulfillModalProvider(testRequestId));
        expect(state.impactEstimate, isNotNull);
        expect(state.impactEstimate!.qualityTime.attributes.groupSize, 2);
      });
    });

    test('confirming a helper mirrors the preview into impactDraftProvider so QT detail modal sees the live group size', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);

        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);
        notifier.initialize(const []);

        // Confirm three helpers.
        notifier
          ..toggleHelper('helper-1')
          ..toggleHelper('helper-2')
          ..toggleHelper('helper-3');

        async.elapse(const Duration(milliseconds: 250));
        async.flushMicrotasks();

        // The draft notifier's state now has the live group size (host + 3 = 4)
        // so opening the QT detail modal will read the correct value.
        final draftState = container.read(impactDraftProvider(testRequestId));
        expect(draftState.draft, isNotNull,
            reason: 'preview must be mirrored into impactDraftProvider.draft '
                'or the QT detail modal will see a stale group size');
        expect(
          draftState.draft!.qualityTime.attributes.groupSize,
          4,
          reason: 'group size must match the live confirmed helper count + host',
        );
      });
    });

    test('debounced toggles coalesce into one preview RPC', () {
      runDebounced((async) {
        final container = makeContainer();
        addTearDown(container.dispose);

        final notifier = container.read(fulfillModalProvider(testRequestId).notifier);
        notifier.initialize(const []);

        // Advance past initialize()'s own preview debounce, then reset counts.
        async.elapse(const Duration(milliseconds: 250));
        async.flushMicrotasks();
        clearInteractions(mockRequestRepo);

        // Three rapid toggles within the debounce window.
        notifier
          ..toggleHelper('h1')
          ..toggleHelper('h2')
          ..toggleHelper('h3');

        async.elapse(const Duration(milliseconds: 250));
        async.flushMicrotasks();

        verify(
          mockRequestRepo.previewRequestImpact(
            requestId: testRequestId,
            confirmedHelperIds: anyNamed('confirmedHelperIds'),
            confirmedHelperCount: 3,
          ),
        ).called(1);
      });
    });

    test('applyExternalDraft no-ops when the user has applied a QT override', () async {
      when(
        mockImpactRepo.redraftImpact(
          experienceId: anyNamed('experienceId'),
          requestId: anyNamed('requestId'),
          qualityTimeInput: anyNamed('qualityTimeInput'),
          moneySavingsInput: anyNamed('moneySavingsInput'),
          emissionsInput: anyNamed('emissionsInput'),
        ),
      ).thenAnswer((_) async => makeImpact(groupSize: 7));

      final container = makeContainer();
      addTearDown(container.dispose);

      // Simulate the user manually overriding group size in the QT detail modal.
      await container
          .read(impactDraftProvider(testRequestId).notifier)
          .applyQualityTimeOverrides(
            QualityTimeAttributes(groupSize: 7),
            isExperience: false,
          );

      final notifier = container.read(fulfillModalProvider(testRequestId).notifier);
      notifier.initialize(const []);
      notifier.toggleHelper('helper-x');

      runDebounced((async) {
        async.elapse(const Duration(milliseconds: 250));
        async.flushMicrotasks();
      });

      // User's override (7) must remain authoritative — preview can't clobber it.
      final draftState = container.read(impactDraftProvider(testRequestId));
      expect(draftState.draft!.qualityTime.attributes.groupSize, 7);
    });
  });

  group('FulfillModalNotifier — the preview is the only impact source', () {
    test('opening with confirmed helpers shows the confirmed-set estimate, '
        'never the request-wide potential one', () async {
      final container = makeContainer();
      addTearDown(container.dispose);

      final notifier =
          container.read(fulfillModalProvider(testRequestId).notifier);
      // Two pre-confirmed helpers: the preview stub answers group size 3
      // (host + 2), the getStats stub answers the generic 2.
      notifier.initialize([makeUser('a', 'A'), makeUser('b', 'B')]);
      await pumpEventQueue();

      final state = container.read(fulfillModalProvider(testRequestId));
      expect(
        state.impactEstimate?.qualityTime.attributes.groupSize,
        3,
        reason: 'the modal must quote the estimate the fulfillment commits, '
            'not the request-wide potential estimate',
      );
      // getStats answers a different question and must not be a second writer
      // of impactEstimate: as an async race it wins or loses per run.
      verifyNever(
        mockRequestRepo.getStats(testRequestId,
            communityId: anyNamed('communityId')),
      );
    });
  });
}
