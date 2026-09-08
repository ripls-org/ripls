// Idempotently joins the community behind an event share link before the event
// view loads. An authenticated visitor who follows an event share link to a
// community they're not in must become a member first — otherwise
// GetExperience / RSVPToExperience are rejected by the community-scoped access
// gate (#2136). New users join inside registration (via the short_code,
// 2050 §D1); this covers the already-authenticated case (#2237) on both web
// (WebExperienceScreen) and mobile (the /invite event branch).

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/services/providers/community_providers.dart';

part 'event_invite_join_view_model.freezed.dart';

/// Phase of the join attempt. `idle` is the initial state before `join()` is
/// called. `running` covers the in-flight AcceptInvitationLink call.
/// `succeeded` is the terminal success (including the already-a-member
/// idempotent case). `failed` is the terminal failure that gates the event
/// view behind a retry surface.
enum EventInviteJoinStatus {
  idle,
  running,
  succeeded,
  failed,
}

@freezed
abstract class EventInviteJoinState with _$EventInviteJoinState {
  const factory EventInviteJoinState({
    @Default(EventInviteJoinStatus.idle) EventInviteJoinStatus status,
  }) = _EventInviteJoinState;

  const EventInviteJoinState._();

  bool get isRunning => status == EventInviteJoinStatus.running;
  bool get hasFailed => status == EventInviteJoinStatus.failed;
  bool get hasSucceeded => status == EventInviteJoinStatus.succeeded;
}

/// Family-keyed by shortCode so the join state is scoped per share link.
final eventInviteJoinProvider = NotifierProvider.autoDispose
    .family<EventInviteJoinNotifier, EventInviteJoinState, String>(
  EventInviteJoinNotifier.new,
);

class EventInviteJoinNotifier extends Notifier<EventInviteJoinState>
    with SafeNotifierMixin<EventInviteJoinState> {
  /// Constructor accepts the shortCode from the family modifier (see
  /// `WebEventRsvpHandoffNotifier` for the canonical pattern).
  EventInviteJoinNotifier(this.shortCode);

  /// The event share-link code whose community this joins.
  final String shortCode;

  @override
  EventInviteJoinState build() {
    return const EventInviteJoinState();
  }

  /// Joins the community behind [shortCode] via AcceptInvitationLink. The
  /// server resolves the community from the code and is idempotent — an
  /// existing member returns success — so this is safe to call unconditionally
  /// whenever a code is present. No-op if already running or done.
  Future<void> join() async {
    if (state.isRunning || state.hasSucceeded) {
      return;
    }
    safeUpdateState((s) => s.copyWith(status: EventInviteJoinStatus.running));
    try {
      await ref
          .read(communityRepositoryProvider)
          .acceptInvitationLink(shortCode: shortCode);
      if (!ref.mounted) return;
      safeUpdateState(
          (s) => s.copyWith(status: EventInviteJoinStatus.succeeded));
    } catch (_) {
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(status: EventInviteJoinStatus.failed));
    }
  }

  /// Retries from the failed-terminal state. No-op otherwise.
  Future<void> retry() async {
    if (!state.hasFailed) return;
    safeUpdateState((_) => const EventInviteJoinState());
    await join();
  }
}
