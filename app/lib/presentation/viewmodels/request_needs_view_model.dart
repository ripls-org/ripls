import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/services/providers.dart';

export 'package:ripls/data/repositories/request_repository.dart'
    show
        RequestNeedResponse,
        RequestContributionResponse,
        ListRequestNeedsAndContributionsResponse,
        RequestBatchNeedItem;

part 'request_needs_view_model.freezed.dart';

/// State for the collaborative needs and contributions lists on the Plan tab.
@freezed
sealed class RequestNeedsState with _$RequestNeedsState {
  const factory RequestNeedsState({
    /// Needs ("Still needed") for the request.
    @Default([]) List<RequestNeedResponse> needs,

    /// Contributions ("Who's helping with what") for the request.
    @Default([]) List<RequestContributionResponse> contributions,

    /// LLM-generated chip suggestions for the requester's add-to-request strip.
    @Default([]) List<String> additionalAsks,

    /// LLM-generated chip suggestions for breaking the request into pieces.
    @Default([]) List<String> breakdownPieces,

    /// LLM-generated chip suggestions shown to helpers as "Ways to help".
    @Default([]) List<String> offerIdeas,

    @Default(true) bool isLoading,

    /// Set when the last load failed.
    UserError? error,

    /// Set while a mutation (add/remove/claim/etc.) is in progress.
    @Default(false) bool isMutating,

    /// Set when a mutation fails; cleared on next successful mutation.
    UserError? mutationError,
  }) = _RequestNeedsState;

  const RequestNeedsState._();

  bool get hasError => error != null;
}

/// Notifier that manages request needs and contributions state.
///
/// Parameterized by requestId via the autoDispose.family modifier.
class RequestNeedsNotifier extends Notifier<RequestNeedsState>
    with SafeNotifierMixin<RequestNeedsState> {
  RequestNeedsNotifier(this.requestId);

  final String requestId;

  /// Future for the initial load scheduled in [build].
  ///
  /// [refresh] awaits this on first call to avoid issuing a duplicate request.
  Future<void>? _initialLoadFuture;

  @override
  RequestNeedsState build() {
    _initialLoadFuture = Future.microtask(_load);
    return const RequestNeedsState();
  }

  RequestRepository get _repo => ref.read(requestRepositoryProvider);

  /// True when no authenticated user is in scope. Guarded at every
  /// mutation + load entry so a late-firing RPC from a tap that races
  /// logout doesn't write into the logout frame (IndexedStack rebuild)
  /// and trigger a duplicate NavigatorState GlobalKey crash. Mirrors
  /// the pattern in `location_modal_view_model.dart` (commit
  /// "Fix logout crash" / 624e6bc4d).
  // `ref.read(...)` throws once the Notifier has been disposed; guard the
  // read so the trace and guard paths in `_load`/`_refreshAfterMutation`
  // can safely evaluate this getter after disposal too.
  bool get _loggedOut =>
      ref.mounted ? ref.read(authStateProvider).user == null : true;

  /// Records a viewmodel-level trace event scoped to this notifier's
  /// request id. Always records — the ring buffer is sized to hold a few
  /// minutes of activity, so pre-logout BEGIN events for in-flight mutations
  /// survive long enough to appear in the dump after the crash. See
  /// `core/utils/logout_diagnostics.dart` for the wiring.
  void _trace(String label, [String? details]) {
    final scope = 'request=$requestId';
    LogoutDiagnostics.trace(
      'REQ_NEEDS_VM_$label',
      details == null ? scope : '$scope | $details',
    );
  }

  Future<void> _load() async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isLoading: true, error: null);
    try {
      final response = await _repo.listNeedsAndContributions(requestId);
      if (!ref.mounted || _loggedOut) return;
      state = state.copyWith(
        needs: response.needs,
        contributions: response.contributions,
        additionalAsks: response.additionalAsks,
        breakdownPieces: response.breakdownPieces,
        offerIdeas: response.offerIdeas,
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
    // disposal (compose-sheet pop, test container teardown, etc.).
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
      final response = await _repo.refreshNeedsAndContributions(requestId);
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
        additionalAsks: response.additionalAsks,
        breakdownPieces: response.breakdownPieces,
        offerIdeas: response.offerIdeas,
        isLoading: false,
      );
    } catch (e) {
      if (!ref.mounted) return;
      _trace('REFRESH_ERROR_STATE_WRITE', e.runtimeType.toString());
      state = state.copyWith(isLoading: false, error: RpcErrorHandler.classify(e));
    }
  }

  /// Adds a need to the request (requester only).
  /// Adds a single need to the request. Returns the new need's id on
  /// success, or null when the call was guarded (logged-out / unmounted)
  /// or the underlying RPC failed. Callers that want to immediately
  /// claim the new need (e.g. the requester pre-claiming via the "I'll
  /// bring this myself" toggle on the add-to-list sheet) use the
  /// returned id to dispatch `claimNeed` without diffing state.
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
          requestId: requestId, name: name, note: note, slots: slots);
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
      await _repo.removeNeed(needId: needId, requestId: requestId);
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
        requestId: requestId,
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

  /// Nudges RequestOffer members who haven't created a contribution
  /// yet (requester only). Returns the number of users notified.
  Future<int?> nudgeUncoveredNeedClaimers() async {
    if (!ref.mounted || _loggedOut) return null;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      final count = await _repo.nudgeUncoveredNeedClaimers(
        requestId: requestId,
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

  /// Adds multiple needs in a single call (requester only).
  Future<void> addNeedsBatch(List<RequestBatchNeedItem> items) async {
    if (!ref.mounted || _loggedOut) {
      _trace('ADD_NEEDS_BATCH_GUARDED_PRE',
          'mounted=${ref.mounted} loggedOut=$_loggedOut count=${items.length}');
      return;
    }
    _trace('ADD_NEEDS_BATCH_BEGIN', 'count=${items.length}');
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.addNeedsBatch(requestId: requestId, items: items);
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

  /// Claims a need slot (any helper).
  ///
  /// Automatically creates a RequestOffer and transitions the request to
  /// OFFERS_RECEIVED if no offer exists.
  Future<void> claimNeed(
    String needId, {
    required String communityId,
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
        requestId: requestId,
        communityId: communityId,
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

  /// Returns a claimed contribution slot (contributor only).
  Future<void> unclaimNeed(String contributionId) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.unclaimNeed(contributionId: contributionId, requestId: requestId);
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }

  /// Adds a free-form contribution (any helper).
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
        requestId: requestId,
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

  /// Updates an existing contribution (contributor only).
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
        requestId: requestId,
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

  /// Removes a contribution (contributor only).
  Future<void> removeContribution(String contributionId) async {
    if (!ref.mounted || _loggedOut) return;
    state = state.copyWith(isMutating: true, mutationError: null);
    try {
      await _repo.removeContribution(contributionId: contributionId, requestId: requestId);
      await _refreshAfterMutation();
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isMutating: false, mutationError: RpcErrorHandler.classify(e));
    }
  }
}

/// Provider for [RequestNeedsNotifier], scoped by request ID.
///
/// autoDispose ensures the state is cleaned up when no widget is watching.
final requestNeedsProvider = NotifierProvider.autoDispose
    .family<RequestNeedsNotifier, RequestNeedsState, String>(
  RequestNeedsNotifier.new,
);
