import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/data/repositories/tier_tracking_repository.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_metric.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/time_health_ladder.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../../../helpers/l10n_helpers.dart';

class _FakeObservabilityService extends ObservabilityService {
  final List<AnalyticsEvent> recorded = [];

  _FakeObservabilityService() : super();

  @override
  Future<void> logAnalyticsEvent(AnalyticsEvent event) async {
    recorded.add(event);
  }
}

Widget _wrap({
  required List<Object> overrides,
  required Widget child,
}) {
  return ProviderScope(
    overrides: overrides.cast(),
    child: localizedApp(child),
  );
}

void main() {
  late _FakeObservabilityService fakeObservability;
  late TierTrackingRepository repository;

  setUp(() async {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
    final prefs = await SharedPreferences.getInstance();
    fakeObservability = _FakeObservabilityService();
    repository = TierTrackingRepository(prefs);
  });

  testWidgets('EquivalenceLadderCard renders the resolved tier headline',
      (tester) async {
    await tester.pumpWidget(_wrap(
      overrides: [
        tierTrackingRepositoryProvider.overrideWithValue(repository),
        observabilityServiceProvider.overrideWithValue(fakeObservability),
      ],
      child: const Scaffold(
        body: EquivalenceLadderCard(
          ladder: kTimeHealthLadder,
          metric: EquivalenceMetric.timeTogether,
          surface: EquivalenceSurface.communityStage,
          value: 42, // 42 hr → t_40hr "Two weeks of un-smoking"
          communitySize: 2,
          emoji: '\u{1F496}',
          accentColor: Color(0xFF1565C0),
          fallbackBody: 'fallback body',
        ),
      ),
    ));
    await tester.pumpAndSettle();

    expect(find.textContaining('Two weeks of un-smoking'), findsOneWidget);
    expect(find.textContaining('Source:'), findsOneWidget);
  });

  testWidgets('EquivalenceLadderCard renders fallback when below lowest tier',
      (tester) async {
    await tester.pumpWidget(_wrap(
      overrides: [
        tierTrackingRepositoryProvider.overrideWithValue(repository),
        observabilityServiceProvider.overrideWithValue(fakeObservability),
      ],
      child: const Scaffold(
        body: EquivalenceLadderCard(
          ladder: kTimeHealthLadder,
          metric: EquivalenceMetric.timeTogether,
          surface: EquivalenceSurface.communityStage,
          // Below 5 min (= 5/60 hr threshold for tier 1).
          value: 0,
          communitySize: 2,
          emoji: '\u{1F496}',
          accentColor: Color(0xFF1565C0),
          fallbackBody: 'fallback body — testing 123',
        ),
      ),
    ));
    await tester.pumpAndSettle();

    expect(find.text('fallback body — testing 123'), findsOneWidget);
    // No source link should render when no tier resolves.
    expect(find.textContaining('Source:'), findsNothing);
  });

  testWidgets('EquivalenceLadderCard fires tier_unlocked once after build',
      (tester) async {
    await tester.pumpWidget(_wrap(
      overrides: [
        tierTrackingRepositoryProvider.overrideWithValue(repository),
        observabilityServiceProvider.overrideWithValue(fakeObservability),
      ],
      child: const Scaffold(
        body: EquivalenceLadderCard(
          ladder: kTimeHealthLadder,
          metric: EquivalenceMetric.timeTogether,
          surface: EquivalenceSurface.communityStage,
          value: 42,
          communitySize: 2,
          emoji: '\u{1F496}',
          accentColor: Color(0xFF1565C0),
          fallbackBody: 'fallback body',
        ),
      ),
    ));
    await tester.pumpAndSettle();

    expect(fakeObservability.recorded, hasLength(1));
    final event = fakeObservability.recorded.single as TierUnlockedEvent;
    expect(event.metric, 'time_together');
    expect(event.tierId, 't_40hr');
  });
}
