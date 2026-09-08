import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart'
    show CompleteTransferResponse;
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart'
    show authStateProvider, portfolioRepositoryProvider, transferRepositoryProvider;

class _FakeAuthNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => const AuthStateData(isLoading: false);
}

class _FakePortfolioRepository extends Fake implements PortfolioRepository {
  _FakePortfolioRepository(this.viewToReturn);
  GetHomeViewResponse viewToReturn;

  @override
  Future<GetHomeViewResponse> getHomeView(String timezone) async =>
      viewToReturn;

  @override
  Future<void> refreshHomeView() async {}
}

/// Records each forward and undo call so tests can assert the right mutation
/// ran for the typed action.
class _FakeTransferRepository extends Fake implements TransferRepository {
  final List<String> started = [];
  final List<String> completed = [];
  final List<String> undoneStart = [];
  final List<String> undoneCompleteLoan = [];
  final List<String> undoneCompleteGiveaway = [];
  final bool fail;
  _FakeTransferRepository({this.fail = false});

  @override
  Future<String> startLoan({required String transferId}) async {
    if (fail) throw Exception('boom');
    started.add(transferId);
    return 'evt-start';
  }

  @override
  Future<CompleteTransferResponse> completeTransfer({
    required String transferId,
  }) async {
    if (fail) throw Exception('boom');
    completed.add(transferId);
    return CompleteTransferResponse(communityEventId: 'evt-complete');
  }

  @override
  Future<void> undoStartLoan({required String communityEventId}) async {
    undoneStart.add(communityEventId);
  }

  @override
  Future<void> undoCompleteLoan({required String communityEventId}) async {
    undoneCompleteLoan.add(communityEventId);
  }

  @override
  Future<void> undoCompleteGiveaway({required String communityEventId}) async {
    undoneCompleteGiveaway.add(communityEventId);
  }
}

HomeDecision _transferDecision(HomeTransferAction action) => HomeDecision(
      id: 'd1',
      kind: HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE,
      transferId: 't-1',
      transferAction: action,
    );

void main() {
  Future<
      ({
        ProviderContainer container,
        _FakeTransferRepository transfer,
      })> setUp(
    List<HomeDecision> decisions, {
    bool fail = false,
  }) async {
    final portfolio = _FakePortfolioRepository(GetHomeViewResponse(
      decisions: decisions,
      decisionCount: decisions.length,
    ));
    final transfer = _FakeTransferRepository(fail: fail);
    final container = ProviderContainer(overrides: [
      portfolioRepositoryProvider.overrideWithValue(portfolio),
      transferRepositoryProvider.overrideWithValue(transfer),
      resolvedTimezoneProvider.overrideWith((ref) async => 'UTC'),
      authStateProvider.overrideWith(_FakeAuthNotifier.new),
    ]);
    addTearDown(container.dispose);
    await container.read(homeTabProvider.notifier).load();
    return (container: container, transfer: transfer);
  }

  test('loan pickup starts the loan and removes the card', () async {
    final d = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_START_LOAN);
    final h = await setUp([d]);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.confirmTransferUpdate(d);

    expect(result.error, isNull);
    expect(result.communityEventId, 'evt-start');
    expect(h.transfer.started, ['t-1']);
    expect(h.transfer.completed, isEmpty);
    expect(h.container.read(homeTabProvider).visibleDecisions, isEmpty);
  });

  test('loan return completes the transfer', () async {
    final d = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN);
    final h = await setUp([d]);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.confirmTransferUpdate(d);

    expect(result.communityEventId, 'evt-complete');
    expect(h.transfer.completed, ['t-1']);
    expect(h.transfer.started, isEmpty);
  });

  test('giveaway pickup completes the transfer', () async {
    final d = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_GIVEAWAY);
    final h = await setUp([d]);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.confirmTransferUpdate(d);

    expect(result.communityEventId, 'evt-complete');
    expect(h.transfer.completed, ['t-1']);
  });

  test('unspecified action is a no-op (caller navigates instead)', () async {
    final d = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_UNSPECIFIED);
    final h = await setUp([d]);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.confirmTransferUpdate(d);

    expect(result.error, isNull);
    expect(result.communityEventId, '');
    expect(h.transfer.started, isEmpty);
    expect(h.transfer.completed, isEmpty);
    // Card stays — nothing was resolved in place.
    expect(h.container.read(homeTabProvider).visibleDecisions, hasLength(1));
  });

  test('a failed confirm rolls back the optimistic removal', () async {
    final d = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_START_LOAN);
    final h = await setUp([d], fail: true);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.confirmTransferUpdate(d);

    expect(result.error, isNotNull);
    // Card restored so the user can retry.
    expect(h.container.read(homeTabProvider).visibleDecisions, hasLength(1));
  });

  test('undo routes to the matching reversal and restores the card', () async {
    final loan = _transferDecision(
        HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN);
    final h = await setUp([loan]);
    final notifier = h.container.read(homeTabProvider.notifier);

    await notifier.confirmTransferUpdate(loan);
    expect(h.container.read(homeTabProvider).visibleDecisions, isEmpty);

    final undoError = await notifier.undoTransferUpdate(loan, 'evt-complete');
    expect(undoError, isNull);
    expect(h.transfer.undoneCompleteLoan, ['evt-complete']);
    expect(h.transfer.undoneStart, isEmpty);
    // Card restored immediately so the undo is visible.
    expect(
      h.container.read(homeTabProvider).visibleDecisions.map((d) => d.id),
      contains('d1'),
    );
  });
}
