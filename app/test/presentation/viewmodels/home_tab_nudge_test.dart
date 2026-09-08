import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;
import 'package:ripls/services/providers.dart'
    show authStateProvider, feedRepositoryProvider, portfolioRepositoryProvider;

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

class _FakeFeedRepository extends Fake implements FeedRepository {
  final List<({String id, String action})> consumed = [];

  @override
  Future<void> consumeNudge(String nudgeId, String action) async {
    consumed.add((id: nudgeId, action: action));
  }
}

void main() {
  Future<({ProviderContainer container, _FakeFeedRepository feed})> setUp(
    GetHomeViewResponse view,
  ) async {
    final feed = _FakeFeedRepository();
    final container = ProviderContainer(overrides: [
      portfolioRepositoryProvider
          .overrideWithValue(_FakePortfolioRepository(view)),
      feedRepositoryProvider.overrideWithValue(feed),
      resolvedTimezoneProvider.overrideWith((ref) async => 'UTC'),
      authStateProvider.overrideWith(_FakeAuthNotifier.new),
    ]);
    addTearDown(container.dispose);
    await container.read(homeTabProvider.notifier).load();
    return (container: container, feed: feed);
  }

  test('inboxNudge exposes the server nudge', () async {
    final h = await setUp(GetHomeViewResponse(
      nudge: NudgePayload(nudgeId: 'n-1', ctaAction: 'plan_experience'),
    ));
    expect(h.container.read(homeTabProvider).inboxNudge?.nudgeId, 'n-1');
  });

  test('inboxNudge is null when the server sent none', () async {
    final h = await setUp(GetHomeViewResponse());
    expect(h.container.read(homeTabProvider).inboxNudge, isNull);
  });

  test('consumeInboxNudge hides the nudge optimistically and fires the RPC',
      () async {
    final h = await setUp(GetHomeViewResponse(
      nudge: NudgePayload(nudgeId: 'n-1', ctaAction: 'list_item'),
    ));
    final notifier = h.container.read(homeTabProvider.notifier);

    notifier.consumeInboxNudge('n-1', 'list_item');

    // Hidden immediately.
    expect(h.container.read(homeTabProvider).inboxNudge, isNull);
    // RPC fired with the tapped action.
    expect(h.feed.consumed, [(id: 'n-1', action: 'list_item')]);
  });
}
