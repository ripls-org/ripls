import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show MarkRequestFulfilledResponse;
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart'
    show
        authStateProvider,
        experienceRepositoryProvider,
        portfolioRepositoryProvider,
        requestRepositoryProvider,
        transferRepositoryProvider;

/// Stubs auth so the notifier's [build] doesn't touch WidgetsBinding (absent in
/// a pure `test()`); a null user is all the home loader reads.
class _FakeAuthNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => const AuthStateData(isLoading: false);
}

/// Fake portfolio repo that hands the home view back unchanged so the notifier
/// can load() a seeded queue, and no-ops the refresh after a bulk action.
class _FakePortfolioRepository extends Fake implements PortfolioRepository {
  _FakePortfolioRepository(this.viewToReturn);
  GetHomeViewResponse viewToReturn;

  @override
  Future<GetHomeViewResponse> getHomeView(String timezone) async =>
      viewToReturn;

  @override
  Future<void> refreshHomeView() async {}

  @override
  Future<void> dismissInboxItem({
    required dynamic itemType,
    required String itemId,
  }) async {}

  @override
  Future<void> markInboxItemRead({
    required dynamic itemType,
    required String itemId,
  }) async {}
}

class _FakeRequestRepository extends Fake implements RequestRepository {
  final List<String> fulfilled = [];

  @override
  Future<MarkRequestFulfilledResponse> markRequestFulfilled({
    required String requestId,
    String? resolutionSummary,
    List<String>? confirmedHelperIds,
    int? confirmedHelperCount,
    int? fulfilledAtUnixSec,
    Object? qualityTimeOverrides,
    Object? moneySavingsOverrides,
    Object? emissionsOverrides,
  }) async {
    fulfilled.add(requestId);
    return MarkRequestFulfilledResponse();
  }
}

class _FakeExperienceRepository extends Fake implements ExperienceRepository {
  final List<String> completed = [];
  final Set<String> failIds;
  _FakeExperienceRepository({this.failIds = const {}});

  @override
  Future<CompleteExperienceResponse> completeExperience(
    String experienceId, {
    String? summary,
    List<String>? confirmedAttendeeIds,
    int? confirmedAttendeeCount,
    Object? qualityTimeOverrides,
    Object? moneySavingsOverrides,
    Object? emissionsOverrides,
  }) async {
    if (failIds.contains(experienceId)) {
      throw Exception('boom');
    }
    completed.add(experienceId);
    return CompleteExperienceResponse();
  }
}

class _FakeTransferRepository extends Fake implements TransferRepository {
  final List<String> selected = [];
  final List<String> undone = [];
  final String eventId;
  _FakeTransferRepository({this.eventId = ''});

  @override
  Future<({String conversationId, String communityEventId})> selectRecipient({
    required String transferId,
    required String recipientId,
  }) async {
    selected.add(transferId);
    return (conversationId: 'conv-1', communityEventId: eventId);
  }

  @override
  Future<void> undoSelectRecipient({required String communityEventId}) async {
    undone.add(communityEventId);
  }
}

HomeDecision _markDone(String id, String contentId, DailyItemType type) =>
    HomeDecision(
      id: id,
      kind: HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE,
      contentId: contentId,
      itemType: type,
    );

void main() {
  Future<
      ({
        ProviderContainer container,
        _FakeRequestRepository req,
        _FakeExperienceRepository exp,
        _FakeTransferRepository transfer,
      })> setUp(
    List<HomeDecision> decisions, {
    Set<String> failExpIds = const {},
    String transferEventId = '',
  }) async {
    final portfolio = _FakePortfolioRepository(GetHomeViewResponse(
      decisions: decisions,
      decisionCount: decisions.length,
    ));
    final req = _FakeRequestRepository();
    final exp = _FakeExperienceRepository(failIds: failExpIds);
    final transfer = _FakeTransferRepository(eventId: transferEventId);
    final container = ProviderContainer(overrides: [
      portfolioRepositoryProvider.overrideWithValue(portfolio),
      requestRepositoryProvider.overrideWithValue(req),
      experienceRepositoryProvider.overrideWithValue(exp),
      transferRepositoryProvider.overrideWithValue(transfer),
      resolvedTimezoneProvider.overrideWith((ref) async => 'UTC'),
      authStateProvider.overrideWith(_FakeAuthNotifier.new),
    ]);
    addTearDown(container.dispose);
    await container.read(homeTabProvider.notifier).load();
    return (container: container, req: req, exp: exp, transfer: transfer);
  }

  test('bulkWrapUp completes each item by type and removes them', () async {
    final decisions = [
      _markDone('d1', 'req-1', DailyItemType.DAILY_ITEM_TYPE_REQUEST),
      _markDone('d2', 'exp-1', DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE),
    ];
    final h = await setUp(decisions);
    final notifier = h.container.read(homeTabProvider.notifier);

    final failed = await notifier.bulkWrapUp(decisions);

    expect(failed, 0);
    expect(h.req.fulfilled, ['req-1']);
    expect(h.exp.completed, ['exp-1']);
    // Both optimistically removed.
    expect(
      h.container.read(homeTabProvider).removedDecisionIds,
      containsAll(<String>{'d1', 'd2'}),
    );
  });

  test('a failed item rolls back while successes stay removed', () async {
    final decisions = [
      _markDone('d1', 'req-1', DailyItemType.DAILY_ITEM_TYPE_REQUEST),
      _markDone('d2', 'exp-1', DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE),
    ];
    final h = await setUp(decisions, failExpIds: {'exp-1'});
    final notifier = h.container.read(homeTabProvider.notifier);

    final failed = await notifier.bulkWrapUp(decisions);

    expect(failed, 1);
    final removed = h.container.read(homeTabProvider).removedDecisionIds;
    expect(removed, contains('d1')); // request succeeded → stays gone
    expect(removed, isNot(contains('d2'))); // experience failed → reappears
  });

  test('acceptDecision selects the recipient and undo restores the card',
      () async {
    final decision = HomeDecision(
      id: 'd1',
      kind: HomeDecisionKind.HOME_DECISION_KIND_LENDING_REQUEST,
      transferId: 't-1',
      counterparty: DailyPerson(userId: 'u-2', displayName: 'Sarah'),
    );
    final h = await setUp([decision], transferEventId: 'evt-1');
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.acceptDecision(decision);
    expect(result.error, isNull);
    expect(result.communityEventId, 'evt-1');
    expect(h.transfer.selected, ['t-1']);
    // Card removed optimistically on accept.
    expect(h.container.read(homeTabProvider).visibleDecisions, isEmpty);

    final undoError =
        await notifier.undoAcceptDecision(decision, result.communityEventId);
    expect(undoError, isNull);
    expect(h.transfer.undone, ['evt-1']); // server reversal fired
    // Card restored so the undo is visible immediately.
    expect(
      h.container.read(homeTabProvider).visibleDecisions.map((d) => d.id),
      contains('d1'),
    );
  });

  test('a mid-operation refresh keeps the bulk removal whole (no incrementing)',
      () async {
    // The per-item mutations invalidate the cache and trigger refetches while
    // the server is still catching up; the optimistically-removed rows must
    // stay hidden as a group rather than re-appearing one at a time.
    final decisions = [
      _markDone('d1', 'req-1', DailyItemType.DAILY_ITEM_TYPE_REQUEST),
      _markDone('d2', 'req-2', DailyItemType.DAILY_ITEM_TYPE_REQUEST),
    ];
    final h = await setUp(decisions);
    final notifier = h.container.read(homeTabProvider.notifier);

    // Optimistically remove both.
    await notifier.dismissDecision(decisions[0]);
    await notifier.dismissDecision(decisions[1]);
    expect(h.container.read(homeTabProvider).visibleDecisions, isEmpty);

    // Server still returns both (lag). A refetch must NOT re-surface them.
    await notifier.refresh();
    expect(
      h.container.read(homeTabProvider).removedDecisionIds,
      containsAll(<String>{'d1', 'd2'}),
    );
    expect(h.container.read(homeTabProvider).visibleDecisions, isEmpty);
  });
}
