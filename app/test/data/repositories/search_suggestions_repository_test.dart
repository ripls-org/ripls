import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/search_suggestions_repository.dart';
import 'package:ripls/services/search_service.dart';

import 'search_suggestions_repository_test.mocks.dart';

@GenerateMocks([SearchService])
void main() {
  group('SearchSuggestionsRepository', () {
    late SearchSuggestionsRepository repository;
    late MockSearchService mockService;
    late StashCacheManager cache;

    setUp(() async {
      mockService = MockSearchService();
      cache = StashCacheManager();
      await cache.initialize();
      repository = SearchSuggestionsRepository(cache, mockService);
    });

    GetSearchSuggestionsResponse fakeResp() {
      return GetSearchSuggestionsResponse()
        ..topKnownForCategories.addAll(['Power Tools', 'Camping'])
        ..topCommunities.addAll([
          SharedCommunityRef(id: 'c1', name: 'Community 1'),
          SharedCommunityRef(id: 'c2', name: 'Community 2'),
        ]);
    }

    test('returns suggestions from service', () async {
      when(mockService.getSuggestions()).thenAnswer((_) async => fakeResp());

      final result = await repository.getSuggestions(userId: 'u1');

      expect(result.topKnownForCategories, ['Power Tools', 'Camping']);
      expect(result.topCommunities.length, 2);
      verify(mockService.getSuggestions()).called(1);
    });

    test('hits the cache on second call with same user', () async {
      when(mockService.getSuggestions()).thenAnswer((_) async => fakeResp());

      await repository.getSuggestions(userId: 'u1');
      await repository.getSuggestions(userId: 'u1');

      verify(mockService.getSuggestions()).called(1);
    });

    test('different userIds use separate cache entries', () async {
      when(mockService.getSuggestions()).thenAnswer((_) async => fakeResp());

      await repository.getSuggestions(userId: 'u1');
      await repository.getSuggestions(userId: 'u2');

      verify(mockService.getSuggestions()).called(2);
    });

    test('refreshSuggestions() forces a refetch on next call', () async {
      when(mockService.getSuggestions()).thenAnswer((_) async => fakeResp());

      await repository.getSuggestions(userId: 'u1');
      await repository.refreshSuggestions();
      await repository.getSuggestions(userId: 'u1');

      verify(mockService.getSuggestions()).called(2);
    });

    test('refreshSuggestions() fires onCacheInvalidated callback', () async {
      var fired = 0;
      final repoWithCallback = SearchSuggestionsRepository(
        cache,
        mockService,
        () => fired += 1,
      );
      await repoWithCallback.refreshSuggestions();
      expect(fired, 1);
    });
  });
}
