import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;
import 'package:shared_preferences/shared_preferences.dart';

/// Cap on the number of recent search terms kept per user. Older entries
/// fall off in FIFO order as new searches arrive.
const int recentSearchesMax = 8;

String _key(String userId) => 'recent_searches_$userId';

/// recentSearchesProvider exposes the user's recent feed-search history.
///
/// Backed directly by SharedPreferences (no caching layer — the
/// persistence IS the storage). Follows the localePreferenceProvider
/// precedent in [presentation/viewmodels/locale_view_model.dart] rather
/// than introducing a Stash-backed repository, which is reserved for
/// remote-data access per [docs/client/caching.md].
///
/// Logout's prefs.clear() (see [docs/client/logout.md]) wipes the
/// per-user key as a side-effect, so there is no manual teardown
/// step here.
final recentSearchesProvider =
    AsyncNotifierProvider<RecentSearchesNotifier, List<String>>(
  RecentSearchesNotifier.new,
);

class RecentSearchesNotifier extends AsyncNotifier<List<String>> {
  String? _resolveUserId() {
    return ref.read(authStateProvider).user?.id;
  }

  @override
  Future<List<String>> build() async {
    final userId = _resolveUserId();
    if (userId == null) return const [];
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_key(userId));
    if (raw == null || raw.isEmpty) return const [];
    try {
      final decoded = jsonDecode(raw);
      if (decoded is List) {
        return decoded.whereType<String>().toList(growable: false);
      }
    } catch (_) {
      // Corrupt entry — treat as empty rather than crashing the Feed.
    }
    return const [];
  }

  /// Adds [query] to the front of the recents list. Dedupes (an existing
  /// match is moved to the front rather than duplicated) and trims to
  /// [recentSearchesMax] entries. No-op when [query] is empty or the
  /// user is not loaded.
  Future<void> add(String query) async {
    final trimmed = query.trim();
    if (trimmed.isEmpty) return;
    final userId = _resolveUserId();
    if (userId == null) return;

    final current = state.value ?? const <String>[];
    final updated = <String>[trimmed];
    for (final existing in current) {
      if (existing == trimmed) continue;
      updated.add(existing);
      if (updated.length >= recentSearchesMax) break;
    }

    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key(userId), jsonEncode(updated));
    if (!ref.mounted) return;
    state = AsyncValue.data(updated);
  }

  /// Removes a single [query] from the recents list. No-op if the
  /// query is not present.
  Future<void> remove(String query) async {
    final userId = _resolveUserId();
    if (userId == null) return;
    final current = state.value ?? const <String>[];
    if (!current.contains(query)) return;
    final updated = current.where((q) => q != query).toList(growable: false);
    final prefs = await SharedPreferences.getInstance();
    if (updated.isEmpty) {
      await prefs.remove(_key(userId));
    } else {
      await prefs.setString(_key(userId), jsonEncode(updated));
    }
    if (!ref.mounted) return;
    state = AsyncValue.data(updated);
  }

  /// Clears the recents list for the current user.
  Future<void> clear() async {
    final userId = _resolveUserId();
    final prefs = await SharedPreferences.getInstance();
    if (userId != null) {
      await prefs.remove(_key(userId));
    }
    if (!ref.mounted) return;
    state = const AsyncValue.data([]);
  }
}
