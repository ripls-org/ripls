import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/services/community_service.dart'
    show GetCommunityPresenceForViewerResponse;
import 'package:ripls/services/profile_service.dart'
    show
        GetProfilePresenceForViewerResponse,
        ProfileAskCard,
        ProfileEventCard,
        ProfileQueuedAsk,
        ProfileSheetKind;
import 'package:ripls/services/providers.dart'
    show communityRepositoryProvider;
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';
import 'package:ripls/services/request_service.dart';

class _FakeProfileRepository extends Fake implements ProfileRepository {
  _FakeProfileRepository(this._presence);

  final Future<GetProfilePresenceForViewerResponse> Function() _presence;

  @override
  Future<GetProfilePresenceForViewerResponse> getPresence(
          String targetUserId) =>
      _presence();
}

class _FakeCommunityRepository extends Fake implements CommunityRepository {
  _FakeCommunityRepository(this._presence);

  final Future<GetCommunityPresenceForViewerResponse> Function() _presence;

  @override
  Future<GetCommunityPresenceForViewerResponse> getPresence(
          String communityId) =>
      _presence();
}

class _FakeRequestService extends Fake implements RequestService {
  int offers = 0;
  int withdrawals = 0;
  bool failNext = false;

  @override
  Future<Request> offerToFulfill({
    required String requestId,
    required String communityId,
  }) async {
    if (failNext) throw Exception('boom');
    offers++;
    return Request(id: requestId);
  }

  @override
  Future<void> withdrawOffer({
    required String requestId,
    required String communityId,
  }) async {
    if (failNext) throw Exception('boom');
    withdrawals++;
  }
}

ProfileAskCard _ask({bool committed = false}) => ProfileAskCard(
      requestId: 'r1',
      title: 'Belay partner',
      communityId: 'c1',
      committedCount: committed ? 1 : 0,
      viewerCommitted: committed,
      neededByUnixSec: Int64(2000000000),
    );

ProfileEventCard _event() => ProfileEventCard(
      experienceId: 'e1',
      title: 'Maple Canyon',
      communityId: 'c1',
      goingCount: 2,
      startUnixSec: Int64(2000000000),
    );

ProviderContainer _container({
  required Future<GetProfilePresenceForViewerResponse> Function() presence,
  _FakeRequestService? requestService,
}) {
  final container = ProviderContainer(overrides: [
    profileRepositoryProvider
        .overrideWithValue(_FakeProfileRepository(presence)),
    requestServiceProvider
        .overrideWithValue(requestService ?? _FakeRequestService()),
  ]);
  addTearDown(container.dispose);
  return container;
}

void main() {
  group('mapPresenceToSheetState', () {
    test('maps each kind to its state', () {
      expect(
        mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
          activeAsk: _ask(),
        )),
        isA<ActiveAskSheet>(),
      );
      expect(
        mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_NEXT_EVENT,
          nextEvent: _event(),
        )),
        isA<NextEventSheet>(),
      );
      final cold =
          mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
        sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_COLD_START,
        nextEvent: _event(),
      ));
      expect(cold, isA<ColdStartSheet>());
      expect(cold.suppressHistory, isTrue);
      expect(
        mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_QUIET,
        )),
        isA<QuietSheet>(),
      );
    });

    test('falls back to quiet when the promised card is missing', () {
      expect(
        mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
        )),
        isA<QuietSheet>(),
      );
      expect(
        mapPresenceToSheetState(GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_COLD_START,
        )),
        isA<QuietSheet>(),
      );
    });
  });

  group('ProfileSheetNotifier', () {
    test('build maps the presence response', () async {
      final container = _container(
        presence: () async => GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
          activeAsk: _ask(),
        ),
      );
      final state = await container.read(profileSheetProvider('t1').future);
      expect(state, isA<ActiveAskSheet>());
    });

    test('build falls back to quiet on presence failure', () async {
      final container = _container(
        presence: () async => throw Exception('offline'),
      );
      final state = await container.read(profileSheetProvider('t1').future);
      expect(state, isA<QuietSheet>());
    });

    test('commitToAsk flips optimistically and calls the service', () async {
      final requestService = _FakeRequestService();
      final container = _container(
        presence: () async => GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
          activeAsk: _ask(),
        ),
        requestService: requestService,
      );
      await container.read(profileSheetProvider('t1').future);
      final sub = container.listen(profileSheetProvider('t1'), (_, _) {});

      await container
          .read(profileSheetProvider('t1').notifier)
          .commitToAsk();

      final state = sub.read().value;
      expect(state, isA<ActiveAskSheet>());
      final ask = (state! as ActiveAskSheet).ask;
      expect(ask.viewerCommitted, isTrue);
      expect(ask.committedCount, 1);
      expect(requestService.offers, 1);
    });

    test('commitToAsk reverts on failure and rethrows', () async {
      final requestService = _FakeRequestService()..failNext = true;
      final container = _container(
        presence: () async => GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
          activeAsk: _ask(),
        ),
        requestService: requestService,
      );
      await container.read(profileSheetProvider('t1').future);
      final sub = container.listen(profileSheetProvider('t1'), (_, _) {});

      await expectLater(
        container.read(profileSheetProvider('t1').notifier).commitToAsk(),
        throwsException,
      );

      final state = sub.read().value;
      expect((state! as ActiveAskSheet).ask.viewerCommitted, isFalse);
      expect(requestService.offers, 0);
    });

    test('mapCommunityPresence carries the priority queue', () {
      final state =
          mapCommunityPresenceToSheetState(GetCommunityPresenceForViewerResponse(
        sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
        activeAsk: _ask(),
        queuedAsks: [
          ProfileQueuedAsk(requestId: 'r2', title: 'Crash pad'),
          ProfileQueuedAsk(requestId: 'r3', title: 'Kid-wrangler'),
        ],
      ));
      expect(state, isA<ActiveAskSheet>());
      final queued = (state as ActiveAskSheet).queued;
      expect(queued.map((q) => q.title), ['Crash pad', 'Kid-wrangler']);
    });

    test('community notifier maps presence and falls back to quiet',
        () async {
      final container = ProviderContainer(overrides: [
        communityRepositoryProvider.overrideWithValue(
          _FakeCommunityRepository(() async =>
              GetCommunityPresenceForViewerResponse(
                sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_NEXT_EVENT,
                nextEvent: _event(),
              )),
        ),
        requestServiceProvider.overrideWithValue(_FakeRequestService()),
      ]);
      addTearDown(container.dispose);
      final state =
          await container.read(communitySheetProvider('c1').future);
      expect(state, isA<NextEventSheet>());

      final failing = ProviderContainer(overrides: [
        communityRepositoryProvider.overrideWithValue(
          _FakeCommunityRepository(() async => throw Exception('offline')),
        ),
        requestServiceProvider.overrideWithValue(_FakeRequestService()),
      ]);
      addTearDown(failing.dispose);
      expect(await failing.read(communitySheetProvider('c1').future),
          isA<QuietSheet>());
    });

    test('undoCommit withdraws and flips back', () async {
      final requestService = _FakeRequestService();
      final container = _container(
        presence: () async => GetProfilePresenceForViewerResponse(
          sheetKind: ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK,
          activeAsk: _ask(committed: true),
        ),
        requestService: requestService,
      );
      await container.read(profileSheetProvider('t1').future);
      final sub = container.listen(profileSheetProvider('t1'), (_, _) {});

      await container.read(profileSheetProvider('t1').notifier).undoCommit();

      final state = sub.read().value;
      final ask = (state! as ActiveAskSheet).ask;
      expect(ask.viewerCommitted, isFalse);
      expect(ask.committedCount, 0);
      expect(requestService.withdrawals, 1);
    });
  });
}
