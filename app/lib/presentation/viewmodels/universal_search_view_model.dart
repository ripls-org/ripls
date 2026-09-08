import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/search_service.dart' show UniversalSearchResponse;

part 'universal_search_view_model.freezed.dart';

final _log = Logger('UniversalSearchViewModel');

/// State of the universal search overlay (#2634): the live query, the
/// grouped response for the last completed search, and load/error flags.
@freezed
sealed class UniversalSearchState with _$UniversalSearchState {
  const factory UniversalSearchState({
    @Default('') String query,
    @Default(false) bool isLoading,
    UniversalSearchResponse? response,
    UserError? error,
  }) = _UniversalSearchState;

  const UniversalSearchState._();

  bool get hasError => error != null;

  bool get hasResults =>
      response != null &&
      (response!.libraryResults.isNotEmpty ||
          response!.plansResults.isNotEmpty ||
          response!.peopleResults.isNotEmpty);

  /// True when a completed search for a non-empty query found nothing —
  /// the state that converts to a contribution (ask before you buy).
  bool get isEmptyResult =>
      query.trim().isNotEmpty && !isLoading && response != null && !hasResults;
}

/// Debounced notifier behind the universal search overlay. Queries go
/// through [SearchRepository.universalSearch]; the server derives the
/// community scope from the caller, so no scope is held here.
class UniversalSearchNotifier extends Notifier<UniversalSearchState> {
  static const Duration _debounce = Duration(milliseconds: 300);
  Timer? _debounceTimer;
  int _searchGeneration = 0;

  @override
  UniversalSearchState build() {
    ref.onDispose(() => _debounceTimer?.cancel());
    return const UniversalSearchState();
  }

  /// Updates the live query and schedules a debounced search. An emptied
  /// query clears results immediately without a round trip.
  void onQueryChanged(String query) {
    state = state.copyWith(query: query);
    _debounceTimer?.cancel();
    if (query.trim().isEmpty) {
      _searchGeneration++;
      state = state.copyWith(
        isLoading: false,
        response: null,
        error: null,
      );
      return;
    }
    _debounceTimer = Timer(_debounce, () => unawaited(_search(query.trim())));
  }

  Future<void> _search(String query) async {
    final generation = ++_searchGeneration;
    state = state.copyWith(isLoading: true, error: null);
    try {
      final response = await ref
          .read(searchRepositoryProvider)
          .universalSearch(query: query);
      if (!ref.mounted || generation != _searchGeneration) return;
      state = state.copyWith(isLoading: false, response: response);
    } catch (e, stackTrace) {
      _log.severe('Universal search failed', e, stackTrace);
      if (!ref.mounted || generation != _searchGeneration) return;
      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }
}

/// Screen-scoped provider: disposes (cancelling any pending debounce)
/// when the search overlay closes.
final universalSearchProvider = NotifierProvider.autoDispose<
    UniversalSearchNotifier, UniversalSearchState>(
  UniversalSearchNotifier.new,
);
