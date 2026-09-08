import 'package:freezed_annotation/freezed_annotation.dart';

part 'needs_scope.freezed.dart';

/// Coarse discriminator used by the needs/contributions modals to pick
/// the right copy variant. Surfaces don't need every field on
/// [NeedsScope] to resolve their strings — they only need to know which
/// scope flavour they're rendering for.
enum NeedsScopeKind { experience, request }

/// Sealed discriminator for needs/contributions scope.
///
/// Carry the minimum context each scope needs:
///   - `experience`: experienceId + communityId + RSVP state + owner info
///   - `request`:    requestId + communityId + isOwner flag
///
/// Passed to `NeedsSection` / `NeedsActions` so the same widget tree renders
/// for both scopes while dispatching to the correct provider and applying the
/// correct permission rules.
@freezed
sealed class NeedsScope with _$NeedsScope {
  const factory NeedsScope.experience({
    required String experienceId,
    required String communityId,
    String? currentUserId,
    required bool isTerminal,
    required bool isRsvped,
    required bool isRsvpedMaybe,
    required String ownerId,
    required String experienceName,
  }) = ExperienceNeedsScope;

  const factory NeedsScope.request({
    required String requestId,
    required String communityId,
    String? currentUserId,
    required bool isTerminal,

    /// True when the current user is the requester.
    required bool isOwner,

    /// ID of the requester — used to label owner-posted contributions.
    required String requestOwnerId,

    /// Request title. Surfaced in the claim sheet eyebrow as
    /// "CLAIM A NEED · {REQUEST NAME}". Optional because legacy
    /// callsites may not have it; the sheet falls back gracefully.
    @Default('') String requestName,
  }) = RequestNeedsScope;
}

/// Convenience accessor that maps a [NeedsScope] to its [NeedsScopeKind]
/// so callers can pick the right copy variant without unwrapping the
/// sealed class.
extension NeedsScopeKindExt on NeedsScope {
  NeedsScopeKind get kind => switch (this) {
        ExperienceNeedsScope() => NeedsScopeKind.experience,
        RequestNeedsScope() => NeedsScopeKind.request,
      };
}
