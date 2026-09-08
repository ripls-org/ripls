import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/repositories/search_suggestions_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

part 'search_suggestions_view_model.freezed.dart';

final _log = Logger('SearchSuggestionsViewModel');

/// State for the personalized chips shown in the idle search panel.
///
/// Wraps the proto response into a Dart-flavored shape so widget code
/// can call `.topCommunities` etc. directly without binding to
/// protobuf classes. `AsyncValue<SearchSuggestionsState>` handles the
/// loading + error transitions at the provider boundary.
@freezed
sealed class SearchSuggestionsState with _$SearchSuggestionsState {
  const factory SearchSuggestionsState({
    @Default(<SharedCommunityRef>[]) List<SharedCommunityRef> topCommunities,
    @Default(<String>[]) List<String> topKnownForCategories,
  }) = _SearchSuggestionsState;

  const SearchSuggestionsState._();

  bool get hasCommunities => topCommunities.isNotEmpty;
  bool get hasKnownForCategories => topKnownForCategories.isNotEmpty;
  bool get isEmpty => !hasCommunities && !hasKnownForCategories;
}

/// AsyncNotifier backing the personalized chips in the idle search
/// panel. Auto-dispose so panel-scoped state cleans up on close.
///
/// `build()` reads the current user id and effective community ids,
/// returning an empty state if either is unavailable (logged-out
/// flows, no community selected — the panel falls back to the
/// hardcoded quick chips and recent searches).
class SearchSuggestionsNotifier extends AsyncNotifier<SearchSuggestionsState> {
  @override
  Future<SearchSuggestionsState> build() async {
    // Piggy-back on the existing search cache invalidation signal —
    // every mutation path that already invalidates `SearchRepository`
    // (gear/request/experience CRUD, community join/leave, loan
    // completion) will also refresh suggestions. Keeps the suggestions
    // cache eventually consistent without each mutation site having
    // to know about it.
    ref.listen(searchCacheInvalidationProvider, (prev, next) async {
      if (prev == next) return;
      _log.info('build(): search cache invalidated, refreshing suggestions');
      final repo = ref.read(searchSuggestionsRepositoryProvider);
      await repo.refreshSuggestions();
      if (ref.mounted) ref.invalidateSelf();
    });

    final user = ref.watch(authStateProvider).user;
    if (user == null || user.id.isEmpty) {
      _log.info('build(): no current user, returning empty');
      return const SearchSuggestionsState();
    }

    final repo = ref.read(searchSuggestionsRepositoryProvider);
    final resp = await repo.getSuggestions(userId: user.id);
    _log.info(
      'build(): loaded ${resp.topKnownForCategories.length} categories, '
      '${resp.topCommunities.length} communities',
    );
    return SearchSuggestionsState(
      topCommunities: List.unmodifiable(resp.topCommunities),
      topKnownForCategories: List.unmodifiable(resp.topKnownForCategories),
    );
  }

  /// Manually invalidate the cache and re-fetch. Typically not needed
  /// from the widget — invalidation is push-based via the repository's
  /// `refreshSuggestions()` invoked from mutation paths.
  Future<void> refresh() async {
    final repo = ref.read(searchSuggestionsRepositoryProvider);
    await repo.refreshSuggestions();
    ref.invalidateSelf();
  }
}

/// Provider for [SearchSuggestionsNotifier].
final searchSuggestionsProvider = AsyncNotifierProvider.autoDispose<
    SearchSuggestionsNotifier, SearchSuggestionsState>(
  SearchSuggestionsNotifier.new,
);
