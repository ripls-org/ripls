import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/repositories/home_prefs_repository.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;
import 'package:ripls/services/providers.dart';
import 'package:timezone/timezone.dart' as tz;

part 'home_tab_view_model.freezed.dart';

final _log = Logger('HomeTabViewModel');

/// State for the v4.1 Home tab (#2435).
@freezed
sealed class HomeTabState with _$HomeTabState {
  const factory HomeTabState({
    GetHomeViewResponse? view,
    @Default(true) bool isLoading,
    UserError? error,

    /// Decision IDs optimistically removed this session — either resolved
    /// (accepted) or dismissed ("Not now"). The card animates out and the
    /// badge decrements immediately; cleared on the next server refresh.
    @Default(<String>{}) Set<String> removedDecisionIds,

    /// "Yours" content IDs (gear / request / event) optimistically removed by a
    /// bulk action (e.g. mark-completed). Holds the row's own ID across all
    /// three ID spaces; they never collide. Kept hidden until the server
    /// stops returning them so a bulk edit reads as one committed change
    /// rather than rows vanishing one at a time.
    @Default(<String>{}) Set<String> removedYoursIds,

    /// When the user last opened Recent activity (Unix seconds). Activity
    /// entries newer than this drive the "N new" pill.
    @Default(0) int activityLastOpenedUnixSec,

    /// Inbox nudge IDs consumed this session (CTA tapped). The nudge is hidden
    /// immediately (optimistic dismiss) while the ConsumeNudge RPC fires.
    @Default(<String>{}) Set<String> consumedNudgeIds,
  }) = _HomeTabState;

  const HomeTabState._();

  bool get hasError => error != null;

  /// Decisions still visible after optimistic removals, oldest first.
  List<HomeDecision> get visibleDecisions => view == null
      ? const []
      : view!.decisions
          .where((d) => !removedDecisionIds.contains(d.id))
          .toList();

  /// "Yours" gear still visible after a bulk optimistic removal.
  List<HomeGearItem> get visibleGear => view == null
      ? const []
      : view!.gear.where((g) => !removedYoursIds.contains(g.gearId)).toList();

  /// "Yours" requests still visible after a bulk optimistic removal.
  List<HomeAsk> get visibleAsks => view == null
      ? const []
      : view!.yourAsks
          .where((a) => !removedYoursIds.contains(a.requestId))
          .toList();

  /// "Yours" events still visible after a bulk optimistic removal.
  List<HomeOwnedEvent> get visibleEvents => view == null
      ? const []
      : view!.yourEvents
          .where((e) => !removedYoursIds.contains(e.eventId))
          .toList();

  /// The inbox nudge to render (empty calendar day / zero-state), or null when
  /// the server sent none or it's been consumed this session.
  NudgePayload? get inboxNudge {
    final v = view;
    if (v == null || !v.hasNudge()) return null;
    if (consumedNudgeIds.contains(v.nudge.nudgeId)) return null;
    return v.nudge;
  }

  /// Open-decision count after optimistic removals. Drives the Home tab
  /// badge and the OS app-icon badge; the badge hides entirely at zero.
  int get effectiveDecisionCount {
    if (view == null) return 0;
    final removed =
        view!.decisions.where((d) => removedDecisionIds.contains(d.id)).length;
    final count = view!.decisionCount - removed;
    return count < 0 ? 0 : count;
  }

  /// Count of Recent-activity entries newer than the last time the user
  /// opened that screen — the "N new" pill. Zero hides the pill.
  int get newActivityCount => view == null
      ? 0
      : view!.recentActivity
          .where((a) => a.occurredAtUnixSec > activityLastOpenedUnixSec)
          .length;

  /// True when the user has nothing at all on their plate — the new-user
  /// hero state (frame 6). Recent activity counts as history: a user with
  /// past activity sees the normal layout, not the hero.
  bool get isCompletelyEmpty {
    final v = view;
    if (v == null) return false;
    return v.decisions.isEmpty &&
        v.upNext.isEmpty &&
        v.yourAsks.isEmpty &&
        v.gear.isEmpty &&
        v.recentActivity.isEmpty;
  }
}

/// HomeTabNotifier loads and maintains the Home view: initial load,
/// background refresh on mutation signals, optimistic decision resolution,
/// and the persisted Up-next toggle.
class HomeTabNotifier extends Notifier<HomeTabState> {
  PortfolioRepository get _repository => ref.read(portfolioRepositoryProvider);
  HomePrefsRepository get _prefs => ref.read(homePrefsRepositoryProvider);

  Timer? _refreshDebounce;

  @override
  HomeTabState build() {
    ref.onDispose(() {
      _refreshDebounce?.cancel();
    });

    // Background-refresh whenever a mutation repository signals that
    // portfolio data may have changed (same signal the legacy inbox uses).
    ref.listen(portfolioCacheInvalidationProvider, (_, _) {
      if (ref.read(authStateProvider).user == null) return;
      if (state.view != null) {
        _scheduleRefresh();
      }
    });

    return const HomeTabState();
  }

  /// load performs the initial fetch and restores persisted preferences.
  Future<void> load() async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      final userId = ref.read(authStateProvider).user?.id ?? '';
      final lastOpened =
          userId.isEmpty ? 0 : await _prefs.getActivityLastOpened(userId);
      if (!ref.mounted) return;

      final view = await _repository.getHomeView(await _resolveTimezone());
      if (!ref.mounted) return;

      state = state.copyWith(
        view: view,
        isLoading: false,
        activityLastOpenedUnixSec: lastOpened,
        removedDecisionIds: _prunedRemovals(view),
        removedYoursIds: _prunedYoursRemovals(view),
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to load home view', e, stackTrace);
      if (!ref.mounted) return;
      state = state.copyWith(
        error: RpcErrorHandler.classify(e),
        isLoading: false,
      );
    }
  }

  /// Keeps optimistically-removed decision IDs hidden until the server has
  /// actually dropped them. A bulk wrap-up fires one mutation per item, each
  /// of which invalidates the cache and schedules a refetch; without this, a
  /// mid-operation refetch would reset the removals and re-surface the
  /// not-yet-processed rows one at a time. Pruning to "still present in the
  /// new view" keeps the bulk removal whole and self-cleans once the server
  /// catches up (gone-from-view IDs drop out).
  Set<String> _prunedRemovals(GetHomeViewResponse view) {
    if (state.removedDecisionIds.isEmpty) return const {};
    final present = view.decisions.map((d) => d.id).toSet();
    return state.removedDecisionIds.intersection(present);
  }

  /// Same guard as [_prunedRemovals], for the "Yours" optimistic removals: keeps
  /// bulk-removed gear/request/event IDs hidden until the server stops returning
  /// them, so a mid-operation refetch doesn't re-surface not-yet-processed rows.
  Set<String> _prunedYoursRemovals(GetHomeViewResponse view) {
    if (state.removedYoursIds.isEmpty) return const {};
    final present = <String>{
      ...view.gear.map((g) => g.gearId),
      ...view.yourAsks.map((a) => a.requestId),
      ...view.yourEvents.map((e) => e.eventId),
    };
    return state.removedYoursIds.intersection(present);
  }

  /// refresh re-fetches from the server (pull-to-refresh).
  Future<void> refresh() async {
    try {
      await _repository.refreshHomeView();
      final view = await _repository.getHomeView(await _resolveTimezone());
      if (!ref.mounted) return;
      state = state.copyWith(
        view: view,
        isLoading: false,
        error: null,
        removedDecisionIds: _prunedRemovals(view),
        removedYoursIds: _prunedYoursRemovals(view),
      );
    } catch (e, stackTrace) {
      _log.warning('Failed to refresh home view', e, stackTrace);
      if (!ref.mounted) return;
      // Keep showing stale data on a failed background refresh.
      if (state.view == null) {
        state = state.copyWith(error: RpcErrorHandler.classify(e));
      }
    }
  }

  /// acceptDecision resolves a lend/give decision in place: the card is
  /// removed optimistically and the recipient is selected. Returns the error to
  /// surface (null on success) plus the `communityEventId` undo token from the
  /// mutation, which the caller passes to [undoAcceptDecision] to power a
  /// snackbar undo.
  ///
  /// Request-claim decisions have no in-place mutation (helpers are confirmed
  /// on the request screen at fulfillment); callers navigate instead of
  /// invoking this.
  Future<({UserError? error, String communityEventId})> acceptDecision(
      HomeDecision decision) async {
    if (decision.transferId.isEmpty || !decision.hasCounterparty()) {
      return (error: null, communityEventId: '');
    }
    final previous = state.removedDecisionIds;
    state = state.copyWith(
      removedDecisionIds: {...previous, decision.id},
    );
    try {
      final result = await ref.read(transferRepositoryProvider).selectRecipient(
            transferId: decision.transferId,
            recipientId: decision.counterparty.userId,
          );
      return (error: null, communityEventId: result.communityEventId);
    } catch (e, stackTrace) {
      _log.severe('Failed to accept decision ${decision.id}', e, stackTrace);
      if (!ref.mounted) {
        return (error: RpcErrorHandler.classify(e), communityEventId: '');
      }
      // Roll back the optimistic removal so the card reappears.
      state = state.copyWith(removedDecisionIds: previous);
      return (error: RpcErrorHandler.classify(e), communityEventId: '');
    }
  }

  /// undoAcceptDecision reverses a prior [acceptDecision]: it un-selects the
  /// recipient on the server (which invalidates the portfolio cache and
  /// re-surfaces the decision) and restores the card immediately so the undo is
  /// visible without waiting for the refetch. Returns the error to surface, or
  /// null on success.
  Future<UserError?> undoAcceptDecision(
      HomeDecision decision, String communityEventId) async {
    if (communityEventId.isEmpty) {
      // Nothing to reverse server-side; just bring the card back.
      state = state.copyWith(
        removedDecisionIds: {...state.removedDecisionIds}..remove(decision.id),
      );
      return null;
    }
    try {
      await ref
          .read(transferRepositoryProvider)
          .undoSelectRecipient(communityEventId: communityEventId);
      if (!ref.mounted) return null;
      state = state.copyWith(
        removedDecisionIds: {...state.removedDecisionIds}..remove(decision.id),
      );
      return null;
    } catch (e, stackTrace) {
      _log.severe('Failed to undo accept ${decision.id}', e, stackTrace);
      if (!ref.mounted) return RpcErrorHandler.classify(e);
      return RpcErrorHandler.classify(e);
    }
  }

  /// confirmTransferUpdate advances a borrowing status update in place: the
  /// card is removed optimistically and the typed action (`transferAction`)
  /// runs the matching mutation — StartLoan for a loan pickup, CompleteTransfer
  /// for a loan return or giveaway pickup. Returns the error to surface (null on
  /// success) plus the `communityEventId` undo token the caller passes to
  /// [undoTransferUpdate]. A no-op (empty token, no error) when the decision
  /// carries no typed action — the caller navigates to the item instead.
  Future<({UserError? error, String communityEventId})> confirmTransferUpdate(
      HomeDecision decision) async {
    final action = decision.transferAction;
    if (decision.transferId.isEmpty ||
        action == HomeTransferAction.HOME_TRANSFER_ACTION_UNSPECIFIED) {
      return (error: null, communityEventId: '');
    }
    final previous = state.removedDecisionIds;
    state = state.copyWith(removedDecisionIds: {...previous, decision.id});
    try {
      final repo = ref.read(transferRepositoryProvider);
      final String eventId;
      switch (action) {
        case HomeTransferAction.HOME_TRANSFER_ACTION_START_LOAN:
          eventId = await repo.startLoan(transferId: decision.transferId);
        case HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN:
        case HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_GIVEAWAY:
          final resp =
              await repo.completeTransfer(transferId: decision.transferId);
          eventId = resp.communityEventId;
        default:
          eventId = '';
      }
      return (error: null, communityEventId: eventId);
    } catch (e, stackTrace) {
      _log.severe('Failed to advance transfer ${decision.id}', e, stackTrace);
      if (!ref.mounted) {
        return (error: RpcErrorHandler.classify(e), communityEventId: '');
      }
      // Roll back the optimistic removal so the card reappears.
      state = state.copyWith(removedDecisionIds: previous);
      return (error: RpcErrorHandler.classify(e), communityEventId: '');
    }
  }

  /// undoTransferUpdate reverses a prior [confirmTransferUpdate]: it runs the
  /// matching undo (via the repo, so the home view re-surfaces the decision)
  /// and restores the card immediately so the undo is visible without waiting
  /// for the refetch. Returns the error to surface, or null on success.
  Future<UserError?> undoTransferUpdate(
      HomeDecision decision, String communityEventId) async {
    void restore() => state = state.copyWith(
          removedDecisionIds: {...state.removedDecisionIds}..remove(decision.id),
        );
    if (communityEventId.isEmpty) {
      restore();
      return null;
    }
    try {
      final repo = ref.read(transferRepositoryProvider);
      switch (decision.transferAction) {
        case HomeTransferAction.HOME_TRANSFER_ACTION_START_LOAN:
          await repo.undoStartLoan(communityEventId: communityEventId);
        case HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN:
          await repo.undoCompleteLoan(communityEventId: communityEventId);
        case HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_GIVEAWAY:
          await repo.undoCompleteGiveaway(communityEventId: communityEventId);
        default:
          break;
      }
      if (!ref.mounted) return null;
      restore();
      return null;
    } catch (e, stackTrace) {
      _log.severe('Failed to undo transfer ${decision.id}', e, stackTrace);
      if (!ref.mounted) return RpcErrorHandler.classify(e);
      return RpcErrorHandler.classify(e);
    }
  }

  /// resolveAskClaim acknowledges a request offer: it records the decision as
  /// resolved so it leaves the queue (the badge decrements immediately), while
  /// the caller opens the request's thread expanded so the owner can say thanks in
  /// their own words. Owners don't gate offers, so acknowledging is the whole
  /// action. Best-effort, like a snooze: a failed write rolls back so the card
  /// returns rather than surfacing an error over the thread the user just
  /// opened.
  Future<void> resolveAskClaim(HomeDecision decision) async {
    final previous = state.removedDecisionIds;
    state = state.copyWith(removedDecisionIds: {...previous, decision.id});
    try {
      await _repository.markInboxItemRead(
        itemType: DailyItemType.DAILY_ITEM_TYPE_DECISION,
        itemId: decision.id,
      );
    } catch (e, stackTrace) {
      _log.warning('Failed to resolve ask claim ${decision.id}', e, stackTrace);
      if (!ref.mounted) return;
      state = state.copyWith(removedDecisionIds: previous);
    }
  }

  /// dismissDecision is "Not now" — a snooze, not a permanent dismissal. The
  /// badge decrements immediately (optimistic removal) and the card leaves
  /// the queue, but the server re-surfaces it after the snooze window
  /// (~24h), so a decision is never silently lost. The request also stays
  /// actionable on the item's detail screen in the meantime.
  Future<void> dismissDecision(HomeDecision decision) async {
    final previous = state.removedDecisionIds;
    state = state.copyWith(
      removedDecisionIds: {...previous, decision.id},
    );
    try {
      await _repository.dismissInboxItem(
        itemType: DailyItemType.DAILY_ITEM_TYPE_DECISION,
        itemId: decision.id,
      );
    } catch (e, stackTrace) {
      _log.warning('Failed to dismiss decision ${decision.id}', e, stackTrace);
      if (!ref.mounted) return;
      state = state.copyWith(removedDecisionIds: previous);
    }
  }

  /// bulkWrapUp completes every supplied retrospective ("all wrapped up?")
  /// decision in one go — past hosted events and past-due requests — without the
  /// per-item summary/helpers the full modal collects. Rows are optimistically
  /// removed; any that fail to complete roll back so they reappear. Returns the
  /// number that failed (0 on full success).
  Future<int> bulkWrapUp(List<HomeDecision> decisions) async {
    if (decisions.isEmpty) return 0;
    final ids = decisions.map((d) => d.id).toSet();
    final previous = state.removedDecisionIds;
    state = state.copyWith(removedDecisionIds: {...previous, ...ids});

    final failed = <String>{};
    for (final d in decisions) {
      try {
        switch (d.itemType) {
          case DailyItemType.DAILY_ITEM_TYPE_REQUEST:
            await ref
                .read(requestRepositoryProvider)
                .markRequestFulfilled(requestId: d.contentId);
          case DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE:
            await ref
                .read(experienceRepositoryProvider)
                .completeExperience(d.contentId);
          default:
            // Nothing to complete for this type — just clear the prompt.
            await _repository.markInboxItemRead(
              itemType: DailyItemType.DAILY_ITEM_TYPE_DECISION,
              itemId: d.id,
            );
        }
      } catch (e, stackTrace) {
        _log.warning('Failed to wrap up ${d.id}', e, stackTrace);
        failed.add(d.id);
      }
    }

    if (ref.mounted && failed.isNotEmpty) {
      // Roll back only the ones that failed; successes stay removed.
      state = state.copyWith(
        removedDecisionIds: {...previous, ...ids.difference(failed)},
      );
    }
    unawaited(refresh());
    return failed.length;
  }

  /// bulkComplete marks a "Yours" selection completed as one transaction: every
  /// selected row is removed optimistically up front (so the change reads as a
  /// single committed edit, not rows vanishing one at a time), the underlying
  /// mutations run, and any that fail roll back so those rows reappear. The
  /// optimistic-removal IDs are the rows' own IDs — `requestId` for requests,
  /// `eventId` for events, and the loan's `gearId` (mapped to `transferId` for
  /// the actual mutation). Plain items with no active loan are not passed in.
  /// Refreshes once at the end and returns the done/failed counts so the caller
  /// can show a single precise toast.
  Future<({int done, int failed})> bulkComplete({
    required List<String> requestIds,
    required List<String> eventIds,
    required List<({String gearId, String transferId})> loans,
  }) async {
    final optimisticIds = <String>{
      ...requestIds,
      ...eventIds,
      ...loans.map((l) => l.gearId),
    };
    if (optimisticIds.isEmpty) return (done: 0, failed: 0);

    final previous = state.removedYoursIds;
    state = state.copyWith(removedYoursIds: {...previous, ...optimisticIds});

    // Track the optimistic-removal ID of every failure so only those roll back.
    final failedRemovalIds = <String>{};
    var done = 0;

    Future<void> run(String removalId, Future<void> Function() op) async {
      try {
        await op();
        done++;
      } catch (e, stackTrace) {
        _log.warning('Failed to bulk-complete $removalId', e, stackTrace);
        failedRemovalIds.add(removalId);
      }
    }

    final requestRepo = ref.read(requestRepositoryProvider);
    final experienceRepo = ref.read(experienceRepositoryProvider);
    final transferRepo = ref.read(transferRepositoryProvider);

    for (final id in requestIds) {
      await run(id, () => requestRepo.markRequestFulfilled(requestId: id));
    }
    for (final id in eventIds) {
      await run(id, () => experienceRepo.completeExperience(id));
    }
    for (final l in loans) {
      await run(
          l.gearId, () => transferRepo.completeTransfer(transferId: l.transferId));
    }

    if (ref.mounted && failedRemovalIds.isNotEmpty) {
      // Roll back only the rows whose mutation failed; successes stay removed.
      state = state.copyWith(
        removedYoursIds: {...previous, ...optimisticIds.difference(failedRemovalIds)},
      );
    }
    unawaited(refresh());
    return (done: done, failed: failedRemovalIds.length);
  }

  /// consumeInboxNudge dismisses the inbox nudge when its CTA is tapped: it is
  /// hidden immediately (optimistic), then the ConsumeNudge RPC fires through
  /// the feed repository so it never reappears. Fire-and-forget, mirroring the
  /// feed — a failed consume just means the nudge may return on a later refresh.
  void consumeInboxNudge(String nudgeId, String action) {
    if (nudgeId.isEmpty) return;
    state = state.copyWith(consumedNudgeIds: {...state.consumedNudgeIds, nudgeId});
    ref.read(feedRepositoryProvider).consumeNudge(nudgeId, action).catchError(
        (Object e) {
      _log.warning('ConsumeNudge RPC failed (nudge may reappear): $e');
    });
  }

  
  /// _scheduleRefresh debounces invalidation-driven background refreshes so
  /// mutation bursts collapse into a single request.
  void _scheduleRefresh() {
    _refreshDebounce?.cancel();
    _refreshDebounce = Timer(const Duration(milliseconds: 300), () {
      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;
      unawaited(refresh());
    });
  }

  /// _resolveTimezone resolves the user's effective timezone via the central
  /// provider, falling back to the device's system timezone.
  Future<String> _resolveTimezone() async {
    try {
      return await ref.read(resolvedTimezoneProvider.future);
    } catch (_) {
      return tz.local.name;
    }
  }
}

/// Provider for the Home tab state.
final homeTabProvider = NotifierProvider<HomeTabNotifier, HomeTabState>(
  HomeTabNotifier.new,
);

/// asksByAttention orders requests so the ones that most need attention surface
/// first: zero-claim requests (no progress) ahead of partially-claimed ones,
/// least-claimed first, with the server's newest-first order as the stable
/// tiebreak. Used for both the capped root preview and the see-all screen so
/// they agree.
List<HomeAsk> asksByAttention(List<HomeAsk> asks) {
  double progress(HomeAsk a) =>
      a.totalCount == 0 ? 0 : a.claimedCount / a.totalCount;
  final indexed = asks.asMap().entries.toList();
  indexed.sort((a, b) {
    final cmp = progress(a.value).compareTo(progress(b.value));
    if (cmp != 0) return cmp;
    return a.key.compareTo(b.key);
  });
  return [for (final e in indexed) e.value];
}

/// homeNeedsYouCountProvider is the single Needs-you number — open decisions
/// after optimistic removals (#2435). Every action type (Lend, Say thanks,
/// Reply, Mark done) is a server-assembled decision, so unread threads are
/// already counted here; each clears with one user action. This is the
/// authoritative count for the Home tab badge, the OS app-icon badge, and the
/// "Needs you · N" section header, so the badge can never diverge from what
/// the section renders.
final homeNeedsYouCountProvider = Provider<int>((ref) {
  return ref.watch(homeTabProvider).effectiveDecisionCount;
});
