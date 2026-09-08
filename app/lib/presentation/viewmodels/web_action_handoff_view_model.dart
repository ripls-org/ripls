// Auto-fires a guest's single item action after the post-registration
// redirect brings a freshly-authenticated Flutter Web visitor back to
// /item/{gearId}?intent=interest&code=… or /need/{requestId}?intent=offer&code=…
// (the gear / request analogs of the event RSVP handoff in
// web_event_rsvp_handoff_view_model.dart). The flow mirrors the event
// one: register → join the ad-hoc community (via short_code) → fire the
// action here as the third step.
//
//   - gear   → ExpressInterest(gearId)            (#2492, WEB-4)
//   - request→ OfferToFulfill(requestId, communityId)  (#2492, WEB-3)
//
// ExpressInterest derives the community server-side from the gear's
// shared communities, so the gear handoff needs only the gearId. The
// request handoff resolves the hosting community from the share code
// (OfferToFulfill requires an explicit community_id), exactly as the
// RSVP handoff does.

import 'dart:async';

import 'package:flutter/foundation.dart' show immutable;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/gear_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';

/// Phase of the auto-fire attempt. `idle` is the initial state before
/// `attempt()` is called; `running` covers the in-flight attempts (fire
/// RPC, with retries); `succeeded` / `failed` are terminal. `failed`
/// drives the LiveRegion retry banner on the web screen.
enum WebActionHandoffStatus { idle, running, succeeded, failed }

/// Shared state for the gear/request web action handoffs. Plain
/// immutable (no freezed) so these view-models add no build_runner
/// codegen — the shape is the same two fields the event handoff uses.
@immutable
class WebActionHandoffState {
  const WebActionHandoffState({
    this.status = WebActionHandoffStatus.idle,
    this.attempts = 0,
  });

  final WebActionHandoffStatus status;
  final int attempts;

  bool get isRunning => status == WebActionHandoffStatus.running;
  bool get hasFailed => status == WebActionHandoffStatus.failed;
  bool get hasSucceeded => status == WebActionHandoffStatus.succeeded;

  WebActionHandoffState copyWith({
    WebActionHandoffStatus? status,
    int? attempts,
  }) {
    return WebActionHandoffState(
      status: status ?? this.status,
      attempts: attempts ?? this.attempts,
    );
  }
}

/// Base notifier carrying the retry/backoff loop. Subclasses implement
/// [performAction] with the type-specific RPC. The action runs only
/// after the community join has succeeded (the web screen gates it on
/// the join, mirroring the RSVP handoff), so membership is in place
/// before the RPC fires; retries cover transient failures and are safe
/// because both ExpressInterest and OfferToFulfill are idempotent for a
/// given (user, item).
abstract class WebActionHandoffNotifier
    extends Notifier<WebActionHandoffState>
    with SafeNotifierMixin<WebActionHandoffState> {
  static const _maxAttempts = 3;
  static const _baseBackoff = Duration(milliseconds: 500);

  @override
  WebActionHandoffState build() => const WebActionHandoffState();

  /// Performs the type-specific action (ExpressInterest / OfferToFulfill).
  /// [shortCode] is the share code (a request resolves its hosting
  /// community from it). Throws on failure so the loop can retry.
  Future<void> performAction(String shortCode);

  /// Fires [performAction] with exponential backoff. No-op if already
  /// running or done — safe to call on rebuild.
  Future<void> attempt({required String shortCode}) async {
    if (state.isRunning || state.hasSucceeded) return;

    safeUpdateState(
      (s) => s.copyWith(status: WebActionHandoffStatus.running, attempts: 0),
    );

    for (var i = 0; i < _maxAttempts; i++) {
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(attempts: i + 1));

      try {
        await performAction(shortCode);
        if (!ref.mounted) return;
        safeUpdateState(
          (s) => s.copyWith(status: WebActionHandoffStatus.succeeded),
        );
        return;
      } catch (_) {
        if (i == _maxAttempts - 1) {
          if (!ref.mounted) return;
          safeUpdateState(
            (s) => s.copyWith(status: WebActionHandoffStatus.failed),
          );
          return;
        }
        final backoff = _baseBackoff * (1 << i);
        await Future<void>.delayed(backoff);
      }
    }
  }

  /// Retries from the failed-terminal state. No-op otherwise.
  Future<void> retry({required String shortCode}) async {
    if (!state.hasFailed) return;
    safeUpdateState((_) => const WebActionHandoffState());
    await attempt(shortCode: shortCode);
  }
}

/// Family-keyed by gearId so state is scoped per gear.
final webGearInterestHandoffProvider = NotifierProvider.autoDispose
    .family<WebGearInterestHandoffNotifier, WebActionHandoffState, String>(
      WebGearInterestHandoffNotifier.new,
    );

class WebGearInterestHandoffNotifier extends WebActionHandoffNotifier {
  WebGearInterestHandoffNotifier(this.gearId);

  /// The gear this expresses interest in. ExpressInterest derives the
  /// matching community server-side, so no community lookup is needed.
  final String gearId;

  @override
  Future<void> performAction(String shortCode) async {
    await ref.read(transferRepositoryProvider).expressInterest(gearId: gearId);
  }
}

/// Family-keyed by requestId so state is scoped per request.
final webRequestOfferHandoffProvider = NotifierProvider.autoDispose
    .family<WebRequestOfferHandoffNotifier, WebActionHandoffState, String>(
      WebRequestOfferHandoffNotifier.new,
    );

class WebRequestOfferHandoffNotifier extends WebActionHandoffNotifier {
  WebRequestOfferHandoffNotifier(this.requestId);

  /// The request this offers to help with. OfferToFulfill requires an
  /// explicit community_id, resolved from the share code below.
  final String requestId;

  @override
  Future<void> performAction(String shortCode) async {
    // Re-resolve the share link each attempt; the repo's Stash cache
    // means we typically hit memory after the first call.
    final invitation = await ref
        .read(shareLinkRepositoryProvider)
        .describe(shortCode);
    if (!ref.mounted) return;
    if (invitation.communityId.isEmpty) {
      throw StateError('share_link has no community_id');
    }
    await ref
        .read(requestRepositoryProvider)
        .offerToFulfill(
          requestId: requestId,
          communityId: invitation.communityId,
        );
  }
}
