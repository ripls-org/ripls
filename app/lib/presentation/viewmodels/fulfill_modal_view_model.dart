import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/provisional_user.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/services/providers.dart';

/// State for the MarkFulfilledModal.
class FulfillModalState {
  FulfillModalState({
    this.offerers = const [],
    this.provisionalHelpers = const [],
    this.confirmedIds = const {},
    this.isLoading = false,
    this.errorMessage,
    this.impactEstimate,
    DateTime? fulfilledAt,
  }) : fulfilledAt = fulfilledAt ?? DateTime.now();

  /// Registered users who offered to help (pre-populated from request.offerers).
  final List<User> offerers;

  /// Provisional-user helpers added via search.
  final List<ProvisionalUser> provisionalHelpers;

  /// IDs of helpers currently toggled on (member or prov).
  final Set<String> confirmedIds;

  final bool isLoading;
  final String? errorMessage;
  final ImpactEstimate? impactEstimate;
  final DateTime fulfilledAt;

  FulfillModalState copyWith({
    List<User>? offerers,
    List<ProvisionalUser>? provisionalHelpers,
    Set<String>? confirmedIds,
    bool? isLoading,
    Object? errorMessage = _sentinel,
    Object? impactEstimate = _sentinel,
    DateTime? fulfilledAt,
  }) {
    return FulfillModalState(
      offerers: offerers ?? this.offerers,
      provisionalHelpers: provisionalHelpers ?? this.provisionalHelpers,
      confirmedIds: confirmedIds ?? this.confirmedIds,
      isLoading: isLoading ?? this.isLoading,
      errorMessage: errorMessage == _sentinel
          ? this.errorMessage
          : errorMessage as String?,
      impactEstimate: impactEstimate == _sentinel
          ? this.impactEstimate
          : impactEstimate as ImpactEstimate?,
      fulfilledAt: fulfilledAt ?? this.fulfilledAt,
    );
  }
}

const _sentinel = Object();

/// Provider for FulfillModalNotifier, scoped to a specific request ID.
final fulfillModalProvider = NotifierProvider.autoDispose
    .family<FulfillModalNotifier, FulfillModalState, String>(
      FulfillModalNotifier.new,
    );

/// Notifier that drives the request fulfillment confirmation modal.
///
/// Pre-populates confirmed helpers from the request's offerers list, allows
/// the caller to toggle helpers on/off and add new provisional users, then submits
/// the fulfilled state via the request repository.
class FulfillModalNotifier extends Notifier<FulfillModalState>
    with SafeNotifierMixin<FulfillModalState> {
  FulfillModalNotifier(this._requestId);

  final String _requestId;
  Timer? _previewTimer;

  @override
  FulfillModalState build() {
    ref.onDispose(() => _previewTimer?.cancel());
    return FulfillModalState();
  }

  /// Schedules a debounced live impact preview (250 ms). Mirrors the
  /// experience completion modal's attendance-toggle pattern so the top
  /// tile bar and the per-metric detail modals stay in sync as the host
  /// confirms helpers.
  void _schedulePreviewImpact() {
    _previewTimer?.cancel();
    _previewTimer = Timer(const Duration(milliseconds: 250), previewImpact);
  }

  /// Computes the live impact estimate for the current confirmed-helper set
  /// and writes the result into both `state.impactEstimate` (drives the top
  /// tile bar) and `impactDraftProvider.draft` (drives the detail modals,
  /// including QT group size).
  Future<void> previewImpact() async {
    final confirmed = state.confirmedIds.toList();
    final provisionalCount = state.provisionalHelpers
        .where((s) => state.confirmedIds.contains(s.id))
        .length;
    final registeredIds = confirmed
        .where((id) => !state.provisionalHelpers.any((s) => s.id == id))
        .toList();
    final totalCount = registeredIds.length + provisionalCount;

    try {
      final impact = await ref
          .read(requestRepositoryProvider)
          .previewRequestImpact(
            requestId: _requestId,
            confirmedHelperIds: registeredIds,
            confirmedHelperCount: totalCount,
          );
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(impactEstimate: impact));

      // Mirror into the per-metric draft so the QT/Money/CO₂ detail modals
      // see the same group size and composites the user just saw on the top
      // tile bar. The draft notifier no-ops when the user has any active
      // override — their explicit edits stay authoritative.
      ref
          .read(impactDraftProvider(_requestId).notifier)
          .applyExternalDraft(impact);
    } catch (_) {
      // Preview failure is non-critical: keep showing the last known estimate.
    }
  }

  /// Initializes the modal with the request's existing offerers and free-form
  /// contributors, all confirmed, and starts the live impact preview.
  ///
  /// [contributors] are merged with [offerers] (deduped by ID). Contributors
  /// who are already in [offerers] are not duplicated in the list.
  void initialize(
    List<User> offerers, {
    List<User> contributors = const [],
  }) {
    final seenIds = <String>{};
    final merged = <User>[];
    for (final u in [...offerers, ...contributors]) {
      if (seenIds.add(u.id)) merged.add(u);
    }
    state = state.copyWith(
      offerers: merged,
      confirmedIds: merged.map((u) => u.id).toSet(),
    );
    // The live preview is the ONLY writer of impactEstimate here, and it runs
    // undebounced on open: it is the same computation the fulfillment commits,
    // so what the requester reads is what gets persisted (#2724). The request's
    // stored potential-impact estimate must not also be loaded — it answers a
    // different question (what one generic fulfillment is worth, group size 2),
    // and as a second async writer of one field it would win or lose the race
    // per run, leaving the modal quoting a number the commit never reproduces.
    previewImpact();
  }

  /// Updates the fulfillment date.
  void setFulfilledAt(DateTime date) {
    state = state.copyWith(fulfilledAt: date);
  }

  /// Toggles whether a helper (member or prov) is confirmed.
  void toggleHelper(String id) {
    final next = Set<String>.from(state.confirmedIds);
    if (next.contains(id)) {
      next.remove(id);
    } else {
      next.add(id);
    }
    state = state.copyWith(confirmedIds: next);
    _schedulePreviewImpact();
  }

  /// Adds a registered community member as a helper and confirms them.
  void addMemberHelper(User user) {
    if (state.offerers.any((u) => u.id == user.id)) return;
    state = state.copyWith(
      offerers: [...state.offerers, user],
      confirmedIds: {...state.confirmedIds, user.id},
    );
    _schedulePreviewImpact();
  }

  /// Adds an existing provisional user as a helper and confirms them.
  void addProvisionalHelper(ProvisionalUser prov) {
    if (state.provisionalHelpers.any((s) => s.id == prov.id)) return;
    state = state.copyWith(
      provisionalHelpers: [...state.provisionalHelpers, prov],
      confirmedIds: {...state.confirmedIds, prov.id},
    );
    _schedulePreviewImpact();
  }

  /// Creates a new provisional user for [name] and adds them as a confirmed helper.
  Future<void> createAndAddProvisionalHelper({
    required String communityId,
    required String name,
  }) async {
    safeUpdateState((s) => s.copyWith(isLoading: true, errorMessage: null));
    try {
      final prov = await ref
          .read(provisionalUserRepositoryProvider)
          .createProvisionalUser(communityId: communityId, name: name);
      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          provisionalHelpers: [...s.provisionalHelpers, prov],
          confirmedIds: {...s.confirmedIds, prov.id},
        ),
      );
      _schedulePreviewImpact();
    } catch (_) {
      safeUpdateState(
        (s) =>
            s.copyWith(isLoading: false, errorMessage: 'Could not add person'),
      );
    }
  }

  /// Submits the fulfillment with the currently confirmed helper IDs.
  ///
  /// Returns the community_event_id on success (for undo wiring), or
  /// null on failure (errorMessage is also set in state).
  Future<String?> submit() async {
    safeUpdateState((s) => s.copyWith(isLoading: true, errorMessage: null));
    try {
      final draftState = ref.read(impactDraftProvider(_requestId));
      // Registered helpers first and the same total the live preview used
      // (registered + provisional) — the server derives group size and
      // connection context from these, so ordering and count must match the
      // preview for the committed impact to equal the previewed one (#2724).
      final provisionalIds = state.provisionalHelpers.map((s) => s.id).toSet();
      final registeredIds = state.confirmedIds
          .where((id) => !provisionalIds.contains(id))
          .toList();
      final confirmedProvisionalIds = state.confirmedIds
          .where(provisionalIds.contains)
          .toList();
      final response = await ref
          .read(requestRepositoryProvider)
          .markRequestFulfilled(
            requestId: _requestId,
            confirmedHelperIds: [...registeredIds, ...confirmedProvisionalIds],
            confirmedHelperCount: state.confirmedIds.length,
            fulfilledAtUnixSec:
                state.fulfilledAt.millisecondsSinceEpoch ~/ 1000,
            qualityTimeOverrides: draftState.qualityTimeOverrides,
            moneySavingsOverrides: draftState.moneySavingsOverrides,
            emissionsOverrides: draftState.emissionsOverrides,
          );
      safeUpdateState((s) => s.copyWith(isLoading: false));
      return response.communityEventId;
    } catch (_) {
      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          errorMessage: 'Failed to mark as fulfilled. Please try again.',
        ),
      );
      return null;
    }
  }
}
