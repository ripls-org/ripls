import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb_experience;
import 'package:ripls/data/gen/ripls/api/impact.pb.dart' as pb_impact;
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart' as pb_service;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' as pb_request;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' as pb_transfer;
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/community_impact_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'community_impact_view_model_test.mocks.dart';

/// Creates test metrics with Estimate-based savings fields.
pb_impact.CommunityImpactMetrics _testMetrics({
  double costSavingsUsd = 500.0,
  double carbonSavingsGrams = 25500.0,
  double timeBankedMinutes = 2880.0, // 48 hours
  double totalValueUsd = 1000.0,
}) {
  return pb_impact.CommunityImpactMetrics(
    memberCount: 25,
    gearCount: 50,
    activeLoans: 5,
    completedLoans: 20,
    openGiveaways: 3,
    completedGiveaways: 15,
    openRequests: 2,
    fulfilledRequests: 8,
    upcomingEvents: 4,
    pastEvents: 12,
    costSavingsUsd: pb_common.Estimate(mean: costSavingsUsd, stddev: costSavingsUsd * 0.3),
    costSavingsCount: 20,
    carbonSavingsGrams: pb_common.Estimate(mean: carbonSavingsGrams, stddev: carbonSavingsGrams * 0.4),
    carbonSavingsCount: 20,
    timeBankedMinutes: pb_common.Estimate(mean: timeBankedMinutes, stddev: timeBankedMinutes * 0.3),
    timeFromLoansMinutes: pb_common.Estimate(mean: 1800, stddev: 540),
    timeFromSkillsMinutes: pb_common.Estimate(mean: 600, stddev: 180),
    timeFromRequestsMinutes: pb_common.Estimate(mean: 480, stddev: 144),
    totalValueUsd: totalValueUsd,
  );
}

@GenerateMocks([
  ImpactMetricsRepository,
  MediaRepository,
  TransferRepository,
  ExperienceRepository,
  RequestRepository,
])
void main() {
  late MockImpactMetricsRepository mockImpactMetricsRepository;
  late MockMediaRepository mockMediaRepository;
  late MockTransferRepository mockTransferRepository;
  late MockExperienceRepository mockExperienceRepository;
  late MockRequestRepository mockRequestRepository;

  /// Builds the standard set of provider overrides for tests, including the
  /// per-user activity repositories that the viewmodel now consults.
  // ignore: always_declare_return_types
  testOverrides() => [
        impactMetricsRepositoryProvider
            .overrideWithValue(mockImpactMetricsRepository),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        transferRepositoryProvider.overrideWithValue(mockTransferRepository),
        experienceRepositoryProvider
            .overrideWithValue(mockExperienceRepository),
        requestRepositoryProvider.overrideWithValue(mockRequestRepository),
      ];

  setUp(() {
    mockImpactMetricsRepository = MockImpactMetricsRepository();
    mockMediaRepository = MockMediaRepository();
    mockTransferRepository = MockTransferRepository();
    mockExperienceRepository = MockExperienceRepository();
    mockRequestRepository = MockRequestRepository();

    // Default stubs for the per-user activity repositories return empty lists,
    // matching the "user has no in-flight activity" baseline.
    when(mockTransferRepository.listMyTransfers(
      transferType: anyNamed('transferType'),
      state: anyNamed('state'),
    )).thenAnswer((_) async => <pb_transfer.Transfer>[]);
    when(mockTransferRepository.listReceivedTransfers(
      transferType: anyNamed('transferType'),
      state: anyNamed('state'),
    )).thenAnswer((_) async => <pb_transfer.Transfer>[]);
    when(mockExperienceRepository.listMyExperiences())
        .thenAnswer((_) async => <pb_experience.Experience>[]);
    when(mockRequestRepository.listMyRequests())
        .thenAnswer((_) async => <pb_request.Request>[]);

    // Default stub for getMetrics
    when(mockImpactMetricsRepository.getMetrics(any)).thenAnswer(
      (_) async => _testMetrics(),
    );

    // Default stub for getMetricsResponse
    when(mockImpactMetricsRepository.getMetricsResponse(any)).thenAnswer(
      (_) async => pb_service.GetCommunityImpactMetricsResponse(
        metrics: _testMetrics(),
      ),
    );

    // Default stub for refreshMetrics
    when(mockImpactMetricsRepository.refreshMetrics(any)).thenAnswer(
      (_) async => _testMetrics(
        costSavingsUsd: 600,
        carbonSavingsGrams: 30000,
        timeBankedMinutes: 3000,
        totalValueUsd: 1100,
      ),
    );

    // Default stub for invalidateAll
    when(mockImpactMetricsRepository.invalidateAll(any))
        .thenAnswer((_) async => Future.value());

  });

  group('disposal safety', () {
    test('handles disposal during initial load gracefully', () async {
      final testContainer = ProviderContainer(overrides: testOverrides());

      final future = testContainer.read(
        communityImpactProvider('community123').future,
      );

      testContainer.dispose();

      await expectLater(future, completes);
    });

    test('handles disposal during refresh gracefully', () async {
      final testContainer = ProviderContainer(overrides: testOverrides());

      await testContainer.read(communityImpactProvider('community123').future);

      final notifier = testContainer.read(
        communityImpactProvider('community123').notifier,
      );
      final future = notifier.refresh();

      testContainer.dispose();

      await expectLater(future, completes);
    });

    test('handles disposal during refreshMetrics gracefully', () async {
      final testContainer = ProviderContainer(overrides: testOverrides());

      await testContainer.read(communityImpactProvider('community123').future);

      final notifier = testContainer.read(
        communityImpactProvider('community123').notifier,
      );
      final future = notifier.refreshMetrics();

      testContainer.dispose();

      await expectLater(future, completes);
    });

    // loadCommunityImage was removed in #2064 Phase 3 — CommunityMetricsScreen
    // now watches heroMediaUrlProvider directly. Repository-level coverage of
    // the video-vs-image hero shape lives in
    // test/data/repositories/media_repository_test.dart.
  });

  group('computed properties', () {
    test('totalValueFormatted returns correct format for different values', () {
      final testContainer = ProviderContainer(overrides: testOverrides());

      testContainer.read(communityImpactProvider('community123').future);

      testContainer.listen<AsyncValue<CommunityImpactData>>(
        communityImpactProvider('community123'),
        (previous, next) {
          next.whenData((impact) {
            // $1000 formatted
            expect(impact.totalValueFormatted, equals('\$1,000'));
          });
        },
      );

      testContainer.dispose();
    });

    test('hasTotalValue returns true when total value is greater than zero',
        () async {
      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.hasTotalValue, isTrue);

      testContainer.dispose();
    });

    test('hasTotalValue returns false when total value is zero', () async {
      // Override only the metric repo for this case; testOverrides() still
      // applies the rest. We replace its stub to return zero.
      when(mockImpactMetricsRepository.getMetricsResponse(any)).thenAnswer(
        (_) async => pb_service.GetCommunityImpactMetricsResponse(
          metrics: _testMetrics(totalValueUsd: 0),
        ),
      );

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.hasTotalValue, isFalse);

      testContainer.dispose();
    });
  });

  group('per-user activity flags', () {
    pb_transfer.Transfer activeLoan() => pb_transfer.Transfer(
          id: 't1',
          transferType: pb_transfer.TransferType.TRANSFER_TYPE_LOAN,
          state: pb_transfer.TransferState.TRANSFER_STATE_ACTIVE,
        );

    pb_experience.Experience activeExperience() => pb_experience.Experience(
          id: 'e1',
          state: pb_experience.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        );

    pb_request.Request activeRequest() => pb_request.Request(
          id: 'r1',
          state: pb_request.RequestState.REQUEST_STATE_ACTIVE,
        );

    test('all flags false by default (empty repositories)', () async {
      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveLoan, isFalse);
      expect(impact.userHasActiveExperience, isFalse);
      expect(impact.userHasActiveRequest, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveLoan true when listMyTransfers returns active loan',
        () async {
      when(mockTransferRepository.listMyTransfers(
        transferType: anyNamed('transferType'),
        state: anyNamed('state'),
      )).thenAnswer((_) async => [activeLoan()]);

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveLoan, isTrue);
      expect(impact.userHasActiveExperience, isFalse);
      expect(impact.userHasActiveRequest, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveLoan true when listReceivedTransfers returns active',
        () async {
      when(mockTransferRepository.listReceivedTransfers(
        transferType: anyNamed('transferType'),
        state: anyNamed('state'),
      )).thenAnswer((_) async => [activeLoan()]);

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveLoan, isTrue);

      testContainer.dispose();
    });

    test('userHasActiveLoan false for completed and cancelled transfers',
        () async {
      when(mockTransferRepository.listMyTransfers(
        transferType: anyNamed('transferType'),
        state: anyNamed('state'),
      )).thenAnswer((_) async => [
            pb_transfer.Transfer(
              id: 't1',
              transferType: pb_transfer.TransferType.TRANSFER_TYPE_LOAN,
              state: pb_transfer.TransferState.TRANSFER_STATE_COMPLETED,
            ),
            pb_transfer.Transfer(
              id: 't2',
              transferType: pb_transfer.TransferType.TRANSFER_TYPE_LOAN,
              state: pb_transfer.TransferState.TRANSFER_STATE_CANCELLED,
            ),
          ]);

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveLoan, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveExperience true when listMyExperiences returns active',
        () async {
      when(mockExperienceRepository.listMyExperiences())
          .thenAnswer((_) async => [activeExperience()]);

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveExperience, isTrue);
      expect(impact.userHasActiveLoan, isFalse);
      expect(impact.userHasActiveRequest, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveExperience false for completed/cancelled experiences',
        () async {
      when(mockExperienceRepository.listMyExperiences()).thenAnswer(
        (_) async => [
          pb_experience.Experience(
            id: 'e1',
            state: pb_experience.ExperienceState.EXPERIENCE_STATE_COMPLETED,
          ),
          pb_experience.Experience(
            id: 'e2',
            state: pb_experience.ExperienceState.EXPERIENCE_STATE_CANCELLED,
          ),
        ],
      );

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveExperience, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveRequest true when listMyRequests returns active',
        () async {
      when(mockRequestRepository.listMyRequests())
          .thenAnswer((_) async => [activeRequest()]);

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveRequest, isTrue);
      expect(impact.userHasActiveLoan, isFalse);
      expect(impact.userHasActiveExperience, isFalse);

      testContainer.dispose();
    });

    test('userHasActiveRequest false for fulfilled/cancelled requests',
        () async {
      when(mockRequestRepository.listMyRequests()).thenAnswer(
        (_) async => [
          pb_request.Request(
            id: 'r1',
            state: pb_request.RequestState.REQUEST_STATE_FULFILLED,
          ),
          pb_request.Request(
            id: 'r2',
            state: pb_request.RequestState.REQUEST_STATE_CANCELLED,
          ),
        ],
      );

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveRequest, isFalse);

      testContainer.dispose();
    });

    test('fail-open: transfer fetch error leaves loan flag false', () async {
      when(mockTransferRepository.listMyTransfers(
        transferType: anyNamed('transferType'),
        state: anyNamed('state'),
      )).thenThrow(Exception('boom'));
      when(mockTransferRepository.listReceivedTransfers(
        transferType: anyNamed('transferType'),
        state: anyNamed('state'),
      )).thenThrow(Exception('boom'));

      final testContainer = ProviderContainer(overrides: testOverrides());

      // Must still load the metrics screen — failing one fetch must not break it.
      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveLoan, isFalse);
      expect(impact.metrics.gearCount, greaterThan(0));

      testContainer.dispose();
    });

    test('fail-open: experience fetch error leaves experience flag false',
        () async {
      when(mockExperienceRepository.listMyExperiences())
          .thenThrow(Exception('boom'));

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveExperience, isFalse);
      expect(impact.metrics.gearCount, greaterThan(0));

      testContainer.dispose();
    });

    test('fail-open: request fetch error leaves request flag false', () async {
      when(mockRequestRepository.listMyRequests()).thenThrow(Exception('boom'));

      final testContainer = ProviderContainer(overrides: testOverrides());

      final impact = await testContainer.read(
        communityImpactProvider('community123').future,
      );

      expect(impact.userHasActiveRequest, isFalse);
      expect(impact.metrics.gearCount, greaterThan(0));

      testContainer.dispose();
    });
  });
}
