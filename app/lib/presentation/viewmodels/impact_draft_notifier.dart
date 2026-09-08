import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_state.dart';
import 'package:ripls/services/providers.dart';

final impactDraftProvider = NotifierProvider.autoDispose
    .family<ImpactDraftNotifier, ImpactDraftState, String>(
  ImpactDraftNotifier.new,
);

/// ImpactDraftNotifier manages the LLM-drafted impact estimate lifecycle for
/// the completion modal. The target ID is either an experience or request ID.
class ImpactDraftNotifier extends Notifier<ImpactDraftState>
    with SafeNotifierMixin<ImpactDraftState> {
  ImpactDraftNotifier(this._targetId);

  final String _targetId;

  @override
  ImpactDraftState build() => const ImpactDraftState();

  /// draftExperience fetches an LLM-enriched draft for the given experience.
  /// Does not cache the result.
  Future<void> draftExperience() async {
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));
    try {
      final response = await ref
          .read(impactMetricsRepositoryProvider)
          .draftImpact(experienceId: _targetId);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(draft: response, isLoading: false));
    } catch (e) {
      if (!ref.mounted) return;
      safeUpdateState(
        (s) => s.copyWith(isLoading: false, error: RpcErrorHandler.classify(e)),
      );
    }
  }

  /// draftRequest fetches an LLM-enriched draft for the given request.
  /// Does not cache the result.
  Future<void> draftRequest() async {
    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));
    try {
      final response = await ref
          .read(impactMetricsRepositoryProvider)
          .draftImpact(requestId: _targetId);
      if (!ref.mounted) return;
      // A live preview of the confirmed set may have landed while this was
      // in flight. It wins: it is the estimate the fulfillment commits,
      // whereas this one answers the generic "what is one fulfillment
      // worth". Racing them left the modal quoting a number the commit
      // never reproduced (#2724).
      if (state.isFromPreview) {
        safeUpdateState((s) => s.copyWith(isLoading: false));
        return;
      }
      safeUpdateState((s) => s.copyWith(draft: response, isLoading: false));
    } catch (e) {
      if (!ref.mounted) return;
      safeUpdateState(
        (s) => s.copyWith(isLoading: false, error: RpcErrorHandler.classify(e)),
      );
    }
  }

  /// applyQualityTimeOverrides stores QT attribute overrides and triggers a redraft.
  Future<void> applyQualityTimeOverrides(
    QualityTimeAttributes overrides, {
    required bool isExperience,
  }) async {
    safeUpdateState(
      (s) => s.copyWith(
        isRedrafting: true,
        qualityTimeOverrides: overrides,
        error: null,
      ),
    );
    await _redraft(isExperience: isExperience);
  }

  /// applyMoneySavingsOverrides stores money input overrides and triggers a redraft.
  Future<void> applyMoneySavingsOverrides(
    MoneySavings overrides, {
    required bool isExperience,
  }) async {
    safeUpdateState(
      (s) => s.copyWith(
        isRedrafting: true,
        moneySavingsOverrides: overrides,
        error: null,
      ),
    );
    await _redraft(isExperience: isExperience);
  }

  /// applyEmissionsOverrides stores CO₂ input overrides and triggers a redraft.
  Future<void> applyEmissionsOverrides(
    PreventedEmissions overrides, {
    required bool isExperience,
  }) async {
    safeUpdateState(
      (s) => s.copyWith(
        isRedrafting: true,
        emissionsOverrides: overrides,
        error: null,
      ),
    );
    await _redraft(isExperience: isExperience);
  }

  /// applyExternalDraft mirrors a freshly-computed impact estimate from a
  /// sibling source (the parent completion modal's live attendance preview)
  /// into the draft. Skipped when the user has applied any QT/money/CO₂
  /// override — in that case the user's draft (returned by the last
  /// redraft) is authoritative and must not be clobbered.
  ///
  /// This keeps the per-metric detail modals in sync with attendance
  /// toggles in the parent modal: opening the QT detail sees the live
  /// group size instead of a stale snapshot from the initial draft load.
  void applyExternalDraft(ImpactEstimate impact) {
    final s = state;
    if (s.qualityTimeOverrides != null ||
        s.moneySavingsOverrides != null ||
        s.emissionsOverrides != null) {
      return;
    }
    safeUpdateState((cur) => cur.copyWith(draft: impact, isFromPreview: true));
  }

  /// resetOverrides clears all user overrides and triggers a redraft with LLM values.
  Future<void> resetOverrides({required bool isExperience}) async {
    safeUpdateState(
      (s) => s.copyWith(
        isRedrafting: true,
        qualityTimeOverrides: null,
        moneySavingsOverrides: null,
        emissionsOverrides: null,
        error: null,
      ),
    );
    await _redraft(isExperience: isExperience);
  }

  Future<void> _redraft({required bool isExperience}) async {
    final current = state;
    try {
      final result = await ref
          .read(impactMetricsRepositoryProvider)
          .redraftImpact(
            experienceId: isExperience ? _targetId : null,
            requestId: isExperience ? null : _targetId,
            qualityTimeInput: current.qualityTimeOverrides,
            moneySavingsInput: current.moneySavingsOverrides,
            emissionsInput: current.emissionsOverrides,
          );
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(draft: result, isRedrafting: false));
    } catch (e) {
      if (!ref.mounted) return;
      safeUpdateState(
        (s) => s.copyWith(isRedrafting: false, error: RpcErrorHandler.classify(e)),
      );
    }
  }
}
