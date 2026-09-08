import 'package:flutter_riverpod/flutter_riverpod.dart';

/// The query a universal-search section escape carries into a dock tab:
/// tapping a section header (name + count) on the search overlay opens
/// the matching tab showing **only** that query's results, plus a
/// cancellable search pill. Null = no scope (the full tab).
///
/// Only Plans and People use these — they filter client-side over data
/// the tab already holds. The Library tab is not here: its own
/// `searchProvider` RPC carries the query end-to-end (map pins, shelves
/// and the card rail all render from `filteredResults`).
class TabSearchScopeNotifier extends Notifier<String?> {
  @override
  String? build() => null;

  /// Sets the scope; blank queries clear it.
  void set(String? query) {
    final q = query?.trim() ?? '';
    state = q.isEmpty ? null : q;
  }

  /// Cancels the scope, restoring the full tab.
  void clear() => state = null;
}

/// The Plans tab's active search scope (filters calendar entries).
final plansSearchScopeProvider =
    NotifierProvider<TabSearchScopeNotifier, String?>(
      TabSearchScopeNotifier.new,
    );

/// The People tab's active search scope (filters directory rows).
final peopleSearchScopeProvider =
    NotifierProvider<TabSearchScopeNotifier, String?>(
      TabSearchScopeNotifier.new,
    );
