import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';

void main() {
  GetHomeViewResponse view({
    List<HomeDecision> decisions = const [],
    int? decisionCount,
    List<HomeUpNextEntry> upNext = const [],
    List<HomeAsk> yourAsks = const [],
    List<HomeGearItem> gear = const [],
    List<HomeActivityEntry> recentActivity = const [],
  }) {
    return GetHomeViewResponse(
      decisions: decisions,
      decisionCount: decisionCount ?? decisions.length,
      upNext: upNext,
      yourAsks: yourAsks,
      gear: gear,
      recentActivity: recentActivity,
    );
  }

  HomeDecision decision(String id) => HomeDecision(id: id);

  group('HomeTabState decision counters', () {
    test('effectiveDecisionCount subtracts optimistic removals', () {
      final state = HomeTabState(
        view: view(decisions: [decision('a'), decision('b'), decision('c')]),
        removedDecisionIds: const {'b'},
      );
      expect(state.effectiveDecisionCount, 2);
      expect(state.visibleDecisions.map((d) => d.id), ['a', 'c']);
    });

    test('count clamps at zero and is zero with no view', () {
      const empty = HomeTabState();
      expect(empty.effectiveDecisionCount, 0);

      final overRemoved = HomeTabState(
        view: view(decisions: [decision('a')]),
        removedDecisionIds: const {'a'},
      );
      expect(overRemoved.effectiveDecisionCount, 0);
    });

    test('pre-cap decision_count drives the badge beyond returned cards',
        () {
      // Server caps the returned list but reports the full count.
      final state = HomeTabState(
        view: view(decisions: [decision('a')], decisionCount: 7),
      );
      expect(state.effectiveDecisionCount, 7);
    });
  });

  group('HomeTabState activity pill', () {
    test('counts only entries newer than last opened', () {
      final state = HomeTabState(
        view: view(recentActivity: [
          HomeActivityEntry(id: '1', occurredAtUnixSec: fixnum.Int64(100)),
          HomeActivityEntry(id: '2', occurredAtUnixSec: fixnum.Int64(200)),
          HomeActivityEntry(id: '3', occurredAtUnixSec: fixnum.Int64(300)),
        ]),
        activityLastOpenedUnixSec: 150,
      );
      expect(state.newActivityCount, 2);
    });
  });

  group('asksByAttention', () {
    HomeAsk ask(String id, int claimed, int total) =>
        HomeAsk(requestId: id, claimedCount: claimed, totalCount: total);

    test('orders zero-claim and least-progress first, stable on ties', () {
      // Server returns newest-first; e.g. [done, half, noNeeds, zero].
      final ordered = asksByAttention([
        ask('done', 2, 2), // 100%
        ask('half', 1, 2), // 50%
        ask('noNeeds', 0, 0), // no needs → treated as 0%
        ask('zero', 0, 3), // 0%
      ]);
      // 0% group first (preserving server order: noNeeds before zero),
      // then 50%, then 100%.
      expect(
        ordered.map((a) => a.requestId).toList(),
        ['noNeeds', 'zero', 'half', 'done'],
      );
    });
  });

  group('HomeTabState empty states', () {
    test('isCompletelyEmpty only when every section and history is empty',
        () {
      final hero = HomeTabState(view: view());
      expect(hero.isCompletelyEmpty, isTrue);

      final withHistory = HomeTabState(
        view: view(recentActivity: [HomeActivityEntry(id: '1')]),
      );
      expect(withHistory.isCompletelyEmpty, isFalse);

      final withGear =
          HomeTabState(view: view(gear: [HomeGearItem(gearId: 'g-1')]));
      expect(withGear.isCompletelyEmpty, isFalse);

      // No view loaded yet → not "empty", just loading.
      const loading = HomeTabState();
      expect(loading.isCompletelyEmpty, isFalse);
    });
  });
}
