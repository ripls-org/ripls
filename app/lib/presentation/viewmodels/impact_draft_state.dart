import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';

part 'impact_draft_state.freezed.dart';

@freezed
sealed class ImpactDraftState with _$ImpactDraftState {
  const factory ImpactDraftState({
    ImpactEstimate? draft,
    @Default(false) bool isLoading,
    @Default(false) bool isRedrafting,
    UserError? error,
    QualityTimeAttributes? qualityTimeOverrides,
    MoneySavings? moneySavingsOverrides,
    PreventedEmissions? emissionsOverrides,

    /// Whether [draft] came from a sibling's live preview of the confirmed
    /// set. Once it has, the generic opening draft must not overwrite it:
    /// both are in flight at once, and only the preview matches what the
    /// commit will persist.
    @Default(false) bool isFromPreview,
  }) = _ImpactDraftState;
}
