import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show MarkRequestFulfilledResponse;
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart'
    show CompleteTransferResponse;
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

/// Hands the seeded home view back unchanged so the notifier can load() a
/// "Yours" selection, and no-ops the refresh after a bulk action.
class _FakePortfolioRepository extends Fake implements PortfolioRepository {
  _FakePortfolioRepository(this.viewToReturn);
  GetHomeViewResponse viewToReturn;

  @override
  Future<GetHomeViewResponse> getHomeView(String timezone) async =>
      viewToReturn;

  @override
  Future<void> refreshHomeView() async {}
}

class _FakeRequestRepository extends Fake implements RequestRepository {
  final List<String> fulfilled = [];
  final Set<String> failIds;
  _FakeRequestRepository({this.failIds = const {}});

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
    if (failIds.contains(requestId)) throw Exception('boom');
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
    if (failIds.contains(experienceId)) throw Exception('boom');
    completed.add(experienceId);
    return CompleteExperienceResponse();
  }
}

class _FakeTransferRepository extends Fake implements TransferRepository {
  final List<String> completed = [];
  final Set<String> failIds;
  _FakeTransferRepository({this.failIds = const {}});

  @override
  Future<CompleteTransferResponse> completeTransfer({
    required String transferId,
  }) async {
    if (failIds.contains(transferId)) throw Exception('boom');
    completed.add(transferId);
    return CompleteTransferResponse();
  }
}

/// Builds a home view seeded with the "Yours" sections under test.
GetHomeViewResponse _view({
  List<HomeAsk> asks = const [],
  List<HomeOwnedEvent> events = const [],
  List<HomeGearItem> gear = const [],
}) =>
    GetHomeViewResponse(yourAsks: asks, yourEvents: events, gear: gear);

void main() {
  Future<
      ({
        ProviderContainer container,
        _FakeRequestRepository req,
        _FakeExperienceRepository exp,
        _FakeTransferRepository transfer,
      })> setUp(
    GetHomeViewResponse view, {
    Set<String> failReqIds = const {},
    Set<String> failExpIds = const {},
    Set<String> failTransferIds = const {},
  }) async {
    final portfolio = _FakePortfolioRepository(view);
    final req = _FakeRequestRepository(failIds: failReqIds);
    final exp = _FakeExperienceRepository(failIds: failExpIds);
    final transfer = _FakeTransferRepository(failIds: failTransferIds);
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

  test('completes every kind and removes the whole selection at once',
      () async {
    final view = _view(
      asks: [HomeAsk(requestId: 'req-1', title: 'Ladder')],
      events: [HomeOwnedEvent(eventId: 'evt-1', title: 'Walk')],
      gear: [
        HomeGearItem(
          gearId: 'gear-1',
          transferId: 't-1',
          category: HomeGearCategory.HOME_GEAR_CATEGORY_OUT,
        ),
      ],
    );
    final h = await setUp(view);
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.bulkComplete(
      requestIds: ['req-1'],
      eventIds: ['evt-1'],
      loans: [(gearId: 'gear-1', transferId: 't-1')],
    );

    expect(result.done, 3);
    expect(result.failed, 0);
    expect(h.req.fulfilled, ['req-1']);
    expect(h.exp.completed, ['evt-1']);
    expect(h.transfer.completed, ['t-1']);
    // All three rows hidden from the "Yours" sections by their own IDs.
    final state = h.container.read(homeTabProvider);
    expect(state.visibleAsks, isEmpty);
    expect(state.visibleEvents, isEmpty);
    expect(state.visibleGear, isEmpty);
  });

  test('a failed item rolls back while successes stay removed', () async {
    final view = _view(
      asks: [
        HomeAsk(requestId: 'req-1', title: 'A'),
        HomeAsk(requestId: 'req-2', title: 'B'),
      ],
    );
    final h = await setUp(view, failReqIds: {'req-2'});
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.bulkComplete(
      requestIds: ['req-1', 'req-2'],
      eventIds: const [],
      loans: const [],
    );

    expect(result.done, 1);
    expect(result.failed, 1);
    final visible =
        h.container.read(homeTabProvider).visibleAsks.map((a) => a.requestId);
    expect(visible, isNot(contains('req-1'))); // succeeded → stays gone
    expect(visible, contains('req-2')); // failed → reappears
  });

  test('empty selection is a no-op', () async {
    final h = await setUp(_view());
    final notifier = h.container.read(homeTabProvider.notifier);

    final result = await notifier.bulkComplete(
      requestIds: const [],
      eventIds: const [],
      loans: const [],
    );

    expect(result.done, 0);
    expect(result.failed, 0);
    expect(h.req.fulfilled, isEmpty);
  });

  test('a mid-operation refresh keeps the bulk removal whole (no incrementing)',
      () async {
    final view = _view(
      asks: [
        HomeAsk(requestId: 'req-1', title: 'A'),
        HomeAsk(requestId: 'req-2', title: 'B'),
      ],
    );
    final h = await setUp(view);
    final notifier = h.container.read(homeTabProvider.notifier);

    await notifier.bulkComplete(
      requestIds: ['req-1', 'req-2'],
      eventIds: const [],
      loans: const [],
    );
    expect(h.container.read(homeTabProvider).visibleAsks, isEmpty);

    // Server still returns both (lag). A refetch must NOT re-surface them.
    await notifier.refresh();
    expect(h.container.read(homeTabProvider).visibleAsks, isEmpty);
    expect(
      h.container.read(homeTabProvider).removedYoursIds,
      containsAll(<String>{'req-1', 'req-2'}),
    );
  });
}
