import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/viewmodels/universal_search_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'universal_search_view_model_test.mocks.dart';

@GenerateMocks([SearchRepository])
void main() {
  late MockSearchRepository mockRepo;
  late ProviderContainer container;

  UniversalSearchResponse gearResponse(String name) => UniversalSearchResponse(
        libraryResults: [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: 'g1', name: name),
          ),
        ],
      );

  setUp(() {
    mockRepo = MockSearchRepository();
    container = ProviderContainer(
      overrides: [
        searchRepositoryProvider.overrideWithValue(mockRepo),
      ],
    );
    // The provider is autoDispose; hold a listener for the test's lifetime
    // the way the mounted search screen does.
    container.listen(universalSearchProvider, (_, _) {});
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  test('debounces and stores grouped results', () async {
    when(mockRepo.universalSearch(
            query: anyNamed('query'),
            maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenAnswer((_) async => gearResponse('Ladder'));

    final notifier = container.read(universalSearchProvider.notifier);
    notifier.onQueryChanged('ladd');
    notifier.onQueryChanged('ladder');

    // Debounce window: no fetch yet.
    verifyNever(mockRepo.universalSearch(
        query: anyNamed('query'),
        maxResultsPerGroup: anyNamed('maxResultsPerGroup')));

    await Future<void>.delayed(const Duration(milliseconds: 400));

    // Only the settled query fired.
    verify(mockRepo.universalSearch(
            query: 'ladder', maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .called(1);
    final state = container.read(universalSearchProvider);
    expect(state.isLoading, isFalse);
    expect(state.hasResults, isTrue);
    expect(state.response!.libraryResults.single.gear.name, 'Ladder');
  });

  test('clearing the query clears results without a fetch', () async {
    when(mockRepo.universalSearch(
            query: anyNamed('query'),
            maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenAnswer((_) async => gearResponse('Ladder'));

    final notifier = container.read(universalSearchProvider.notifier);
    notifier.onQueryChanged('ladder');
    await Future<void>.delayed(const Duration(milliseconds: 400));
    expect(container.read(universalSearchProvider).hasResults, isTrue);

    notifier.onQueryChanged('');
    final state = container.read(universalSearchProvider);
    expect(state.hasResults, isFalse);
    expect(state.response, isNull);
    expect(state.isEmptyResult, isFalse);
  });

  test('empty grouped response is the ask-before-you-buy state', () async {
    when(mockRepo.universalSearch(
            query: anyNamed('query'),
            maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenAnswer((_) async => UniversalSearchResponse());

    container.read(universalSearchProvider.notifier).onQueryChanged('projector');
    await Future<void>.delayed(const Duration(milliseconds: 400));

    final state = container.read(universalSearchProvider);
    expect(state.isEmptyResult, isTrue);
    expect(state.hasError, isFalse);
  });

  test('failure surfaces an error and stops loading', () async {
    when(mockRepo.universalSearch(
            query: anyNamed('query'),
            maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenThrow(Exception('boom'));

    container.read(universalSearchProvider.notifier).onQueryChanged('ladder');
    await Future<void>.delayed(const Duration(milliseconds: 400));

    final state = container.read(universalSearchProvider);
    expect(state.hasError, isTrue);
    expect(state.isLoading, isFalse);
  });

  test('stale responses do not overwrite a newer query state', () async {
    // First query resolves slowly; second resolves fast. The slow result
    // must be discarded by the generation guard.
    when(mockRepo.universalSearch(
            query: 'slow', maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenAnswer((_) async {
      await Future<void>.delayed(const Duration(milliseconds: 300));
      return gearResponse('Slow');
    });
    when(mockRepo.universalSearch(
            query: 'fast', maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
        .thenAnswer((_) async => gearResponse('Fast'));

    final notifier = container.read(universalSearchProvider.notifier);
    notifier.onQueryChanged('slow');
    await Future<void>.delayed(const Duration(milliseconds: 350));
    notifier.onQueryChanged('fast');
    await Future<void>.delayed(const Duration(milliseconds: 700));

    final state = container.read(universalSearchProvider);
    expect(state.response!.libraryResults.single.gear.name, 'Fast');
  });

  group('disposal safety', () {
    test('handles disposal during debounce gracefully', () async {
      when(mockRepo.universalSearch(
              query: anyNamed('query'),
              maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
          .thenAnswer((_) async => gearResponse('Ladder'));

      container.read(universalSearchProvider.notifier).onQueryChanged('ladder');
      container.dispose();

      // The debounce timer was cancelled on dispose; waiting past the
      // window must not throw or fetch.
      await Future<void>.delayed(const Duration(milliseconds: 400));
      verifyNever(mockRepo.universalSearch(
          query: anyNamed('query'),
          maxResultsPerGroup: anyNamed('maxResultsPerGroup')));
    });

    test('handles disposal during in-flight search gracefully', () async {
      when(mockRepo.universalSearch(
              query: anyNamed('query'),
              maxResultsPerGroup: anyNamed('maxResultsPerGroup')))
          .thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 200));
        return gearResponse('Ladder');
      });

      container.read(universalSearchProvider.notifier).onQueryChanged('ladder');
      await Future<void>.delayed(const Duration(milliseconds: 350));
      container.dispose();

      // Completion after disposal must not throw.
      await Future<void>.delayed(const Duration(milliseconds: 300));
    });
  });
}
