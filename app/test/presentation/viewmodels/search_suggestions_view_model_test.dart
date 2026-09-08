import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/search_suggestions_repository.dart';
import 'package:ripls/presentation/viewmodels/search_suggestions_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

import 'search_suggestions_view_model_test.mocks.dart';

class _FakeAuthNotifier extends AuthStateNotifier {
  _FakeAuthNotifier(this._state);
  final AuthStateData _state;

  @override
  AuthStateData build() => _state;
}

ProviderContainer _container({
  required MockSearchSuggestionsRepository repo,
  User? user,
}) {
  final authState = AuthStateData(
    accessToken: user == null ? null : 'token',
    user: user,
    isLoading: false,
  );
  return ProviderContainer(
    overrides: [
      searchSuggestionsRepositoryProvider.overrideWithValue(repo),
      authStateProvider.overrideWith(() => _FakeAuthNotifier(authState)),
    ],
  );
}

@GenerateMocks([SearchSuggestionsRepository])
void main() {
  late MockSearchSuggestionsRepository mockRepo;

  setUp(() {
    mockRepo = MockSearchSuggestionsRepository();
  });

  tearDown(() => reset(mockRepo));

  GetSearchSuggestionsResponse fakeResp({
    List<String> categories = const ['Power Tools', 'Camping'],
    List<({String id, String name})> communities = const [
      (id: 'c1', name: 'Community 1'),
      (id: 'c2', name: 'Community 2'),
    ],
  }) {
    return GetSearchSuggestionsResponse()
      ..topKnownForCategories.addAll(categories)
      ..topCommunities.addAll([
        for (final c in communities) SharedCommunityRef(id: c.id, name: c.name),
      ]);
  }

  test('build: returns empty state when no current user', () async {
    final container = _container(repo: mockRepo, user: null);
    addTearDown(container.dispose);

    final state = await container.read(searchSuggestionsProvider.future);
    expect(state.topCommunities, isEmpty);
    expect(state.topKnownForCategories, isEmpty);
    expect(state.isEmpty, isTrue);
    verifyNever(mockRepo.getSuggestions(userId: anyNamed('userId')));
  });

  test('build: surfaces both lists when populated', () async {
    when(mockRepo.getSuggestions(userId: anyNamed('userId')))
        .thenAnswer((_) async => fakeResp());

    final container = _container(
      repo: mockRepo,
      user: User(id: 'u1', name: 'Alice'),
    );
    addTearDown(container.dispose);

    final state = await container.read(searchSuggestionsProvider.future);
    expect(state.topKnownForCategories, ['Power Tools', 'Camping']);
    expect(state.topCommunities.map((c) => c.id), ['c1', 'c2']);
    expect(state.hasCommunities, isTrue);
    expect(state.hasKnownForCategories, isTrue);
    expect(state.isEmpty, isFalse);
    verify(mockRepo.getSuggestions(userId: 'u1')).called(1);
  });

  test('build: order matches repository response (most-recent first)',
      () async {
    when(mockRepo.getSuggestions(userId: anyNamed('userId')))
        .thenAnswer((_) async => fakeResp(
              communities: const [
                (id: 'newest', name: 'Newest'),
                (id: 'middle', name: 'Middle'),
                (id: 'oldest', name: 'Oldest'),
              ],
            ));

    final container = _container(
      repo: mockRepo,
      user: User(id: 'u1', name: 'Alice'),
    );
    addTearDown(container.dispose);

    final state = await container.read(searchSuggestionsProvider.future);
    expect(state.topCommunities.map((c) => c.id),
        ['newest', 'middle', 'oldest']);
  });

  test('build: error from repository surfaces as AsyncError', () async {
    when(mockRepo.getSuggestions(userId: anyNamed('userId')))
        .thenThrow(Exception('boom'));

    final container = _container(
      repo: mockRepo,
      user: User(id: 'u1', name: 'Alice'),
    );
    addTearDown(container.dispose);

    // Keep the autoDispose provider alive long enough to settle.
    final sub = container.listen(searchSuggestionsProvider, (_, _) {});
    addTearDown(sub.close);
    await Future<void>.delayed(Duration.zero);

    expect(container.read(searchSuggestionsProvider).hasError, isTrue);
  });
}
