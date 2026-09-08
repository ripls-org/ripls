import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/services/providers.dart';

export 'package:ripls/data/repositories/experience_repository.dart'
    show
        ExperienceNeedResponse,
        ExperienceContributionResponse,
        ListExperienceNeedsAndContributionsResponse,
        BatchNeedItem,
        BatchContributionItem;

part 'experience_needs_view_model.freezed.dart';

/// State for the collaborative needs and contributions lists on the Details tab.
@freezed
sealed class ExperienceNeedsState with _$ExperienceNeedsState {
  const factory ExperienceNeedsState({
    /// Needs ("Still needed") for the experience.
    @Default([]) List<ExperienceNeedResponse> needs,

    /// Contributions ("Who's bringing what") for the experience.
    @Default([]) List<ExperienceContributionResponse> contributions,

    /// LLM-generated suggestion chip labels for the Plan tab empty state.
    /// Sourced from ListExperienceNeedsAndContributionsResponse.suggestions;
    /// generated lazily by the server if missing at load time.
    @Default([]) List<String> suggestions,

    /// Short human-readable category label accompanying suggestions.
    String? categoryHint,

    @Default(true) bool isLoading,

    /// Set when the last load failed.
    UserError? error,

    /// Set while a mutation (add/remove/claim/etc.) is in progress.
    @Default(false) bool isMutating,

    /// Set when a mutation fails; cleared on next successful mutation.
    UserError? mutationError,
  }) = _ExperienceNeedsState;

  const ExperienceNeedsState._();

  bool get hasError => error != null;
}

/// Notifier that manages experience needs and contributions state.
///
/// Parameterized by experienceId via the autoDispose.family modifier.
class ExperienceNeedsNotifier extends Notifier<ExperienceNeedsState>
    with SafeNotifierMixin<ExperienceNeedsState> {
  ExperienceNeedsNotifier(this.experienceId);

  final String experienceId;

  /// Future for the initial load scheduled in [build].
  ///
  /// [refresh] awaits this on first call to avoid issuing a duplicate request.
  Future<void>? _initialLoadFuture;

  /// Debounce timer for the content-invalidation refresh. See [build].
  Timer? _invalidationRefreshTimer;

  /// Debounce window for invalidation-driven refreshes — the same 300 ms
  /// coalescing window `ExperienceNotifier.scheduleRefresh` uses (see
  /// docs/client/caching.md, Pattern 7), so a burst of community events
  /// triggers one force-fetch instead of one per event.
  static const Duration _invalidationRefreshDebounce =
      Duration(milliseconds: 300);

  @override
  ExperienceNeedsState build() {
    ref.onDispose(() {
      _invalidationRefreshTimer?.cancel();
      _invalidationRefreshTimer = null;
    });
    // Live cross-user updates: another participant's claim or contribution
    // reaches this client as a PLANNING_* community event (stream/poll/push —
    // docs/realtime_updates.md), which the event router turns into a
    // contentCacheInvalidationProvider tick. The roster's RSVP rows already
    // refresh through the experience notifier's listener; subscribing here
    // puts the needs strip and per-person claim pills of an open "Who's In"
    // pane on the same signal (#2724).
    ref.listen(contentCacheInvalidationProvider, (previous, next) {
      if (previous == next) return;
      _scheduleInvalidationRefresh();
    });
    _initialLoadFuture = Future.microtask(_load);
    return const ExperienceNeedsState();
  }

  /// Schedules a debounced force-refresh in response to a content cache
  /// invalidation, coalescing rapid event bursts into a single fetch.
  void _scheduleInvalidationRefresh() {
    _invalidationRefreshTimer?.cancel();
    _invalidationRefreshTimer = Timer(_invalidationRefreshDebounce, () {
      _invalidationRefreshTimer = null;
      // The notifier may have been disposed between the tick and the debounce
      // fire (user navigated away) — `ref.mounted` is the disposal check.
      if (!ref.mounted) return;
      // Fire-and-forget: nothing awaits invalidation-driven refreshes.
      refresh();
    });
  }

  ExperienceRepository get _repo => ref.read(experienceRepositoryProvider);

  /// True when no authenticated user is in scope. Guarded at every
  /// mutation + load entry so a late-firing RPC from a tap that races
  /// logout doesn't write into the logout frame (IndexedStack rebuild)
  /// and trigger a duplicate NavigatorState GlobalKey crash. Mirrors
  /// the same pattern documented on `location_modal_view_model.dart`
  /// (commit "Fix logout crash" / 624e6bc4d).
  bool get _loggedOut => ref.read(authStateProvider).user == null;

  /// Records a viewmodel-level trace event scoped to this notifier's
  /// experience id. Always records — the ring buffer is sized to hold a few
  /// minutes of activity, so pre-logout BEGIN events for in-flight mutations
  /// survive long enough to appear in the dump after the crash. See
  /// `core/utils/logout_diagnostics.dart` for the wiring.
  void _trace(String label, [String? details]) {
    final scope = 'experience=$experienceId';
    LogoutDiagnostics.trace(
      'EXP_NEEDS_VM_$label',
      details == null ? scope : '$scope | $details',
    );
  }

  Future<void> _load() async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isLoading: true, error: null);
    try {
      final response = await _repo.listNeedsAndContributions(experienceId);
      if (!ref.mounted || _loggedOut) return;
      state = state.copyWith(
        needs: response.needs,
        contributions: response.contributions,
        suggestions: response.suggestions,
        categoryHint: response.hasCategoryHint() ? response.categoryHint : null,
        isLoading: false,
      );
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isLoading: false, error: RpcErrorHandler.classify(e));
    }
  }

  /// Reloads needs and contributions, **force-fetching** past the cache.
  ///
  /// On the first call after [build] the in-flight initial load is awaited
  /// before issuing the force-fetch, so the two requests don't race.
  /// Subsequent calls go straight to the force-fetch path. Callers that
  /// have just performed an out-of-band mutation (the compose-publish
  /// flow, the cancel-the-breakdown action, pull-to-refresh) rely on
  /// this method to pick up server state — a plain cached read would
  /// surface the pre-mutation snapshot.
  Future<void> refresh() async {
    final pending = _initialLoadFuture;
    if (pending != null) {
      _initialLoadFuture = null;
      await pending;
    }
    await _refreshAfterMutation();
  }

  /// Force-refreshes by bypassing the cache. Called after mutations to ensure
  /// the server's latest state is reflected.
  Future<void> _refreshAfterMutation() async {
    // Important: `_loggedOut` reads `ref` (via `authStateProvider`),
    // which throws once the notifier has been disposed. Snapshot
    // `mounted` first and only consult `_loggedOut` when the ref is
    // still alive, so this method is safe to be awaited across a
    // disposal.
    final mounted = ref.mounted;
    final loggedOut = mounted && _loggedOut;
    if (!mounted || loggedOut) {
      _trace('REFRESH_GUARDED_PRE',
          'mounted=$mounted loggedOut=$loggedOut');
      return;
    }
    _trace('REFRESH_BEGIN');
    state = state.copyWith(isLoading: true, error: null);
    try {
      final response = await _repo.refreshNeedsAndContributions(experienceId);
      _trace('REFRESH_RPC_RESUMED',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      if (!ref.mounted || _loggedOut) {
        _trace('REFRESH_GUARDED_POST',
            'mounted=${ref.mounted} loggedOut=$_loggedOut');
        return;
      }
      _trace('REFRESH_STATE_WRITE',
          'needs=${response.needs.length} contributions=${response.contributions.length}');
      state = state.copyWith(
        needs: response.needs,
        contributions: response.contributions,
        suggestions: response.suggestions,
        categoryHint: response.hasCategoryHint() ? response.categoryHint : null,
        isLoading: false,
      );
    } catch (e) {
      if (!ref.mounted) return;
      _trace('REFRESH_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isLoading: false, error: RpcErrorHandler.classify(e));
    }
  }

  /// Adds a need to the experience.
  /// Adds a single need to the experience. Returns the new need's id
  /// on success, or null when the call was guarded (logged-out /
  /// unmounted) or the underlying RPC failed. Mirrors the Request-side
  /// signature so the shared "Add to the list" sheet can pre-claim
  /// via `claimNeed(newId, …)` regardless of scope.
  Future<String?> addNeed({
    required String name,
    String? note,
    int slots = 1,
  }) async {
    if (!ref.mounted || _loggedOut) {
      _trace('ADD_NEED_GUARDED_PRE',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      return null;
    }
    _trace('ADD_NEED_BEGIN', 'name=$name slots=$slots');
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      final added = await _repo.addNeed(
        experienceId: experienceId,
        name: name,
        note: note,
        slots: slots,
      );
      _trace('ADD_NEED_RPC_RESUMED',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      await _refreshAfterMutation();
      if (!ref.mounted) {
        _trace('ADD_NEED_POST_REFRESH_UNMOUNTED');
        return added.id;
      }
      _trace('ADD_NEED_STATE_WRITE');
      state = state.copyWith(isMutating: false);
      return added.id;
    } catch (e) {
      if (!ref.mounted) return null;
      _trace('ADD_NEED_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
      return null;
    }
  }

  /// Removes a need (proposer only).
  Future<void> removeNeed(String needId) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.removeNeed(needId: needId, experienceId: experienceId);
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Updates an existing need in place (proposer only).
  Future<void> updateNeed(
    String needId, {
    String? name,
    String? note,
    int? slots,
  }) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.updateNeed(
        needId: needId,
        experienceId: experienceId,
        name: name,
        note: note,
        slots: slots,
      );
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(
        isMutating: false,
        mutationError: RpcErrorHandler.classify(e),
      );
    }
  }

  /// Nudges YES/MAYBE RSVPs who haven't created a contribution yet
  /// (organizer only). Returns the number of users notified.
  Future<int?> nudgeUncoveredNeedClaimers() async {
    if (!ref.mounted || _loggedOut) return null;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      final count = await _repo.nudgeUncoveredNeedClaimers(
        experienceId: experienceId,
      );
      if (!ref.mounted) return count;
      state = state.copyWith(isMutating: false);
      return count;
    } catch (e) {
      if (!ref.mounted) return null;
      state = state.copyWith(
        isMutating: false,
        mutationError: RpcErrorHandler.classify(e),
      );
      return null;
    }
  }

  /// Claims a need slot (any participant). Optionally links the
  /// resulting contribution to a [gearId] from the caller's library.
  Future<void> claimNeed(
    String needId, {
    String? note,
    String? gearId,
  }) async {
    if (!ref.mounted || _loggedOut) {
      _trace('CLAIM_NEED_GUARDED_PRE',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      return;
    }
    _trace('CLAIM_NEED_BEGIN', 'needId=$needId');
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.claimNeed(
        needId: needId,
        experienceId: experienceId,
        note: note,
        gearId: gearId,
      );
      _trace('CLAIM_NEED_RPC_RESUMED',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      await _refreshAfterMutation();
      if (!ref.mounted) {
        _trace('CLAIM_NEED_POST_REFRESH_UNMOUNTED');
        return;
      }
      _trace('CLAIM_NEED_STATE_WRITE');
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      _trace('CLAIM_NEED_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Returns a claimed contribution (contributor only).
  Future<void> unclaimNeed(String contributionId) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.unclaimNeed(
        contributionId: contributionId,
        experienceId: experienceId,
      );
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Adds a free-form contribution (any participant). Optionally
  /// links the contribution to a [gearId] from the caller's library.
  Future<void> addContribution({
    required String title,
    String? description,
    String? gearId,
  }) async {
    if (!ref.mounted || _loggedOut) {
      _trace('ADD_CONTRIBUTION_GUARDED_PRE',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      return;
    }
    _trace('ADD_CONTRIBUTION_BEGIN', 'title=$title');
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.addContribution(
        experienceId: experienceId,
        title: title,
        description: description,
        gearId: gearId,
      );
      _trace('ADD_CONTRIBUTION_RPC_RESUMED',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      await _refreshAfterMutation();
      if (!ref.mounted) {
        _trace('ADD_CONTRIBUTION_POST_REFRESH_UNMOUNTED');
        return;
      }
      _trace('ADD_CONTRIBUTION_STATE_WRITE');
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      _trace('ADD_CONTRIBUTION_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Edits an existing contribution (contributor only). Pass
  /// [clearGearId] = true to drop an existing gear link; pass a
  /// non-empty [gearId] to set or replace it.
  Future<void> editContribution({
    required String contributionId,
    required String title,
    String? description,
    String? gearId,
    bool clearGearId = false,
  }) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.editContribution(
        contributionId: contributionId,
        experienceId: experienceId,
        title: title,
        description: description,
        gearId: gearId,
        clearGearId: clearGearId,
      );
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// RSVPs the current user to the experience with the given intention.
  ///
  /// Called automatically when the user claims a need or adds a contribution
  /// without an existing RSVP.
  Future<void> rsvpWithIntention(
    String communityId,
    RSVPIntention intention,
  ) async {
    if (!ref.mounted || _loggedOut) return;
    try {
      await _repo.rsvp(
        experienceId: experienceId,
        communityId: communityId,
        intention: intention,
      );
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Adds multiple needs to the experience in a single call.
  Future<void> addNeedsBatch(List<BatchNeedItem> items) async {
    if (!ref.mounted || _loggedOut) {
      _trace('ADD_NEEDS_BATCH_GUARDED_PRE',
          'mounted=${ref.mounted} loggedOut=$_loggedOut count=${items.length}');
      return;
    }
    _trace('ADD_NEEDS_BATCH_BEGIN', 'count=${items.length}');
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.addNeedsBatch(experienceId: experienceId, items: items);
      _trace('ADD_NEEDS_BATCH_RPC_RESUMED',
          'mounted=${ref.mounted} loggedOut=$_loggedOut');
      await _refreshAfterMutation();
      if (!ref.mounted) {
        _trace('ADD_NEEDS_BATCH_POST_REFRESH_UNMOUNTED');
        return;
      }
      _trace('ADD_NEEDS_BATCH_STATE_WRITE');
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      _trace('ADD_NEEDS_BATCH_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Adds multiple free-form contributions to the experience in a single call.
  Future<void> addContributionsBatch(List<BatchContributionItem> items) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.addContributionsBatch(
          experienceId: experienceId, items: items);
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Removes a contribution (contributor only).
  Future<void> removeContribution(String contributionId) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.removeContribution(
        contributionId: contributionId,
        experienceId: experienceId,
      );
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }
}

/// Provider for [ExperienceNeedsNotifier], scoped by experience ID.
///
/// autoDispose ensures the state is cleaned up when no widget is watching.
final experienceNeedsProvider = NotifierProvider.autoDispose
    .family<ExperienceNeedsNotifier, ExperienceNeedsState, String>(
  ExperienceNeedsNotifier.new,
);
