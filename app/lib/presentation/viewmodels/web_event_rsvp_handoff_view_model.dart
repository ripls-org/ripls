// Auto-fires RSVPToExperience after the post-registration redirect
// brings a freshly-authenticated Flutter Web visitor back to
// /event/{id}?rsvp=…&code=…. See WebExperienceScreen and
// docs/issues/2050-web-rsvp-actions.md §D1 for the wider context:
// register → CommunityUser (via short_code) → RSVPToExperience runs
// here as the third step in the client-side flow.

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/experience_providers.dart';

part 'web_event_rsvp_handoff_view_model.freezed.dart';

/// Phase of the auto-RSVP attempt. `idle` is the initial state before
/// `attempt()` is called. `running` covers the in-flight attempts
/// (resolve short_code → fire RPC, with retries). `succeeded` is the
/// terminal success. `failed` is the terminal failure surface that
/// drives the LiveRegion retry banner.
enum WebRsvpHandoffStatus {
  idle,
  running,
  succeeded,
  failed,
}

@freezed
abstract class WebRsvpHandoffState with _$WebRsvpHandoffState {
  const factory WebRsvpHandoffState({
    @Default(WebRsvpHandoffStatus.idle) WebRsvpHandoffStatus status,
    @Default(0) int attempts,
  }) = _WebRsvpHandoffState;

  const WebRsvpHandoffState._();

  bool get isRunning => status == WebRsvpHandoffStatus.running;
  bool get hasFailed => status == WebRsvpHandoffStatus.failed;
  bool get hasSucceeded => status == WebRsvpHandoffStatus.succeeded;
}

/// Family-keyed by experienceId so the state is scoped per-event.
/// Same-tab navigation between two different events (rare in the
/// guest flow) gets independent handoff state.
final webEventRsvpHandoffProvider = NotifierProvider.autoDispose
    .family<WebEventRsvpHandoffNotifier, WebRsvpHandoffState, String>(
  WebEventRsvpHandoffNotifier.new,
);

class WebEventRsvpHandoffNotifier extends Notifier<WebRsvpHandoffState>
    with SafeNotifierMixin<WebRsvpHandoffState> {
  /// Constructor accepts the experienceId parameter from the family
  /// modifier (see `CommunityEditNotifier` for the canonical pattern
  /// in this codebase).
  WebEventRsvpHandoffNotifier(this.experienceId);

  /// The experienceId for this specific Web event-detail instance.
  final String experienceId;

  static const _maxAttempts = 3;
  static const _baseBackoff = Duration(milliseconds: 500);

  @override
  WebRsvpHandoffState build() {
    return const WebRsvpHandoffState();
  }

  /// Resolves the share code to the hosting community and fires
  /// RSVPToExperience with exponential backoff. Idempotent on
  /// (user_id, experience_id) server-side, so retry-on-error is safe.
  Future<void> attempt({
    required String rsvpIntention,
    required String shortCode,
  }) async {
    if (state.isRunning || state.hasSucceeded) {
      // Already in flight or already done. Don't double-fire on
      // rebuild.
      return;
    }
    final intention = _parseIntention(rsvpIntention);
    if (intention == null) {
      // Unknown intention from the URL — nothing to do. Stay idle so
      // the screen renders the event view without a banner.
      return;
    }

    safeUpdateState((s) => s.copyWith(
          status: WebRsvpHandoffStatus.running,
          attempts: 0,
        ));

    final shareLinkRepo = ref.read(shareLinkRepositoryProvider);
    final experienceService = ref.read(experienceServiceProvider);

    for (var i = 0; i < _maxAttempts; i++) {
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(attempts: i + 1));

      try {
        // Re-resolve the share link each attempt so a transient
        // resolution failure doesn't lock us out. The repo's Stash
        // cache means we typically hit memory after the first call.
        final invitation = await shareLinkRepo.describe(shortCode);
        if (!ref.mounted) return;

        if (invitation.communityId.isEmpty) {
          throw StateError('share_link has no community_id');
        }

        await experienceService.rsvp(
          experienceId: experienceId,
          communityId: invitation.communityId,
          intention: intention,
        );
        if (!ref.mounted) return;

        safeUpdateState(
            (s) => s.copyWith(status: WebRsvpHandoffStatus.succeeded));
        return;
      } catch (_) {
        if (i == _maxAttempts - 1) {
          if (!ref.mounted) return;
          safeUpdateState(
              (s) => s.copyWith(status: WebRsvpHandoffStatus.failed));
          return;
        }
        final backoff = _baseBackoff * (1 << i);
        await Future<void>.delayed(backoff);
      }
    }
  }

  /// Retries from the failed-terminal state. Resets attempt count and
  /// re-fires `attempt()`. No-op if not in failed state.
  Future<void> retry({
    required String rsvpIntention,
    required String shortCode,
  }) async {
    if (!state.hasFailed) return;
    safeUpdateState((_) => const WebRsvpHandoffState());
    await attempt(rsvpIntention: rsvpIntention, shortCode: shortCode);
  }
}

/// Maps the URL-query-param representation onto the proto enum.
/// Returns `null` for unrecognized values so the caller can skip the
/// attempt silently.
RSVPIntention? _parseIntention(String raw) {
  switch (raw.toLowerCase()) {
    case 'yes':
      return RSVPIntention.RSVP_INTENTION_YES;
    case 'maybe':
      return RSVPIntention.RSVP_INTENTION_MAYBE;
    case 'no':
      return RSVPIntention.RSVP_INTENTION_NO;
    default:
      return null;
  }
}
