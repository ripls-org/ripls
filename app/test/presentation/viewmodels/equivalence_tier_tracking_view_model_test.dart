import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/data/repositories/tier_tracking_repository.dart';
import 'package:ripls/presentation/viewmodels/equivalence_tier_tracking_view_model.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Fake observability service that records analytics events for assertion.
class _FakeObservabilityService extends ObservabilityService {
  final List<AnalyticsEvent> recorded = [];

  _FakeObservabilityService() : super();

  @override
  Future<void> logAnalyticsEvent(AnalyticsEvent event) async {
    recorded.add(event);
  }
}

void main() {
  late ProviderContainer container;
  late _FakeObservabilityService fakeObservability;
  late TierTrackingRepository repository;

  setUp(() async {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
    final prefs = await SharedPreferences.getInstance();

    fakeObservability = _FakeObservabilityService();
    repository = TierTrackingRepository(prefs);

    container = ProviderContainer(
      overrides: [
        tierTrackingRepositoryProvider.overrideWithValue(repository),
        observabilityServiceProvider.overrideWithValue(fakeObservability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('EquivalenceTierTrackingNotifier.recordCurrentTier', () {
    test('fires tier_unlocked on first call for a new tier', () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );

      expect(fakeObservability.recorded, hasLength(1));
      final event = fakeObservability.recorded.single as TierUnlockedEvent;
      expect(event.metric, 'time_together');
      expect(event.tierId, 't_40hr');
      expect(event.value, 42);
      expect(event.communitySize, 5);

      final persisted =
          await repository.getLastSeenTier('workshop_detail', 'time_together');
      expect(persisted, 't_40hr');
    });

    test('does not re-fire when the same tier is re-recorded', () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );
      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 50,
        communitySize: 5,
      );

      expect(fakeObservability.recorded, hasLength(1));
    });

    test('fires again when the tier id changes', () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );
      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_60hr',
        value: 65,
        communitySize: 5,
      );

      expect(fakeObservability.recorded, hasLength(2));
      expect(
        (fakeObservability.recorded.last as TierUnlockedEvent).tierId,
        't_60hr',
      );
    });

    test('does not fire when tierId is null (below lowest threshold)',
        () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: null,
        value: 0,
        communitySize: 5,
      );

      expect(fakeObservability.recorded, isEmpty);
      final persisted =
          await repository.getLastSeenTier('workshop_detail', 'time_together');
      expect(persisted, isNull);
    });

    test('respects per-surface dedup (same tier on different surfaces fires twice)',
        () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );
      await notifier.recordCurrentTier(
        surface: 'profile_row',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 1,
      );

      expect(fakeObservability.recorded, hasLength(2));
    });

    test('does not re-fire when persisted tier matches the new render',
        () async {
      await repository.recordTier('workshop_detail', 'time_together', 't_40hr');

      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      await notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );

      expect(fakeObservability.recorded, isEmpty);
    });
  });

  group('disposal safety', () {
    test('survives container dispose mid-flight without crashing', () async {
      final notifier =
          container.read(equivalenceTierTrackingProvider.notifier);

      // Start the call and dispose before it completes.
      final future = notifier.recordCurrentTier(
        surface: 'workshop_detail',
        metric: 'time_together',
        tierId: 't_40hr',
        value: 42,
        communitySize: 5,
      );
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
