import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:protobuf/protobuf.dart';
import 'package:ripls/services/community_service.dart'
    show GetCommunityPresenceForViewerResponse;
import 'package:ripls/services/profile_service.dart'
    show
        GetProfilePresenceForViewerResponse,
        ProfileAskCard,
        ProfileEventCard,
        ProfileQueuedAsk,
        ProfileSheetKind;
import 'package:ripls/services/providers.dart'
    show communityRepositoryProvider;
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';

export 'package:ripls/services/profile_service.dart'
    show ProfileAskCard, ProfileEventCard, ProfilePresenceFace, ProfileQueuedAsk;

final _log = Logger('ProfileSheetViewModel');

/// The pinned sheet's content state (issue #2568,
/// `profile-final-hybrid-v2.html`) — "the sheet holds the ask". The
/// server selects along the fallback chain (active ask → next shared
/// event → quiet, with cold-start gating); the client maps the
/// response 1:1 onto this union and never re-ranks.
sealed class ProfileSheetState {
  const ProfileSheetState();

  /// True when the profile should hide its history-derived sections
  /// (metric strip, open list) — the cold-start gate.
  bool get suppressHistory => this is ColdStartSheet;
}

/// Quiet — nothing open between the viewer and this person or group
/// right now. The sheet collapses to a mini action row (Message
/// primary, plus the Sharing/Calendar doors).
class QuietSheet extends ProfileSheetState {
  const QuietSheet();
}

/// An open ask holds the sheet ("Needs this week"). On a community
/// profile, [queued] names the asks that qualified but lost the
/// priority contest (the peek row); they stay claimable in the open
/// list.
class ActiveAskSheet extends ProfileSheetState {
  const ActiveAskSheet(this.ask, {this.queued = const []});

  final ProfileAskCard ask;
  final List<ProfileQueuedAsk> queued;
}

/// No ask — the next upcoming event the viewer and target are both in
/// holds the sheet ("Next up together").
class NextEventSheet extends ProfileSheetState {
  const NextEventSheet(this.event);

  final ProfileEventCard event;
}

/// Brand-new connection: the pair's single shared upcoming event holds
/// the sheet ("First one together") and history sections are hidden.
class ColdStartSheet extends ProfileSheetState {
  const ColdStartSheet(this.event);

  final ProfileEventCard event;
}

/// Maps the presence response onto the sheet-state union. Falls back
/// to quiet when a card the kind promises is missing — a malformed
/// response must not take down the profile.
ProfileSheetState mapPresenceToSheetState(
    GetProfilePresenceForViewerResponse resp) {
  switch (resp.sheetKind) {
    case ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK:
      return resp.hasActiveAsk()
          ? ActiveAskSheet(resp.activeAsk)
          : const QuietSheet();
    case ProfileSheetKind.PROFILE_SHEET_KIND_NEXT_EVENT:
      return resp.hasNextEvent()
          ? NextEventSheet(resp.nextEvent)
          : const QuietSheet();
    case ProfileSheetKind.PROFILE_SHEET_KIND_COLD_START:
      return resp.hasNextEvent()
          ? ColdStartSheet(resp.nextEvent)
          : const QuietSheet();
    default:
      return const QuietSheet();
  }
}

/// AsyncNotifier for the pinned-sheet state of [targetUserId]'s
/// profile. Auto-disposes per-target alongside the profile notifier.
///
/// A presence-read failure resolves to [QuietSheet] instead of an
/// error state — the profile stays fully usable without the sheet's
/// dynamic content, and quiet is the correct visual fallback.
class ProfileSheetNotifier extends AsyncNotifier<ProfileSheetState> {
  ProfileSheetNotifier(this.targetUserId);

  /// Target user id this notifier instance is bound to.
  final String targetUserId;

  @override
  Future<ProfileSheetState> build() async {
    try {
      final resp =
          await ref.read(profileRepositoryProvider).getPresence(targetUserId);
      return mapPresenceToSheetState(resp);
    } catch (e) {
      _log.warning('presence load failed, falling back to quiet: $e');
      return const QuietSheet();
    }
  }

  /// Commits the viewer to the active ask ("I'm in") by offering to
  /// fulfill the underlying request. Optimistically flips the sheet to
  /// the committed treatment; reverts and rethrows on failure so the
  /// caller can surface the error.
  Future<void> commitToAsk() async {
    final current = state.value;
    if (current is! ActiveAskSheet || current.ask.viewerCommitted) return;
    final ask = current.ask;
    final committed = ask.deepCopy()
      ..viewerCommitted = true
      ..committedCount = ask.committedCount + 1;
    state = AsyncData(ActiveAskSheet(committed));
    try {
      await ref.read(requestServiceProvider).offerToFulfill(
            requestId: ask.requestId,
            communityId: ask.communityId,
          );
    } catch (e) {
      if (!ref.mounted) return;
      state = AsyncData(ActiveAskSheet(ask));
      rethrow;
    }
  }

  /// Undoes the viewer's commit by withdrawing the offer. Mirrors
  /// [commitToAsk]: optimistic flip back, revert and rethrow on
  /// failure.
  Future<void> undoCommit() async {
    final current = state.value;
    if (current is! ActiveAskSheet || !current.ask.viewerCommitted) return;
    final ask = current.ask;
    final reverted = ask.deepCopy()
      ..viewerCommitted = false
      ..committedCount = (ask.committedCount - 1).clamp(0, 1 << 30);
    state = AsyncData(ActiveAskSheet(reverted));
    try {
      await ref.read(requestServiceProvider).withdrawOffer(
            requestId: ask.requestId,
            communityId: ask.communityId,
          );
    } catch (e) {
      if (!ref.mounted) return;
      state = AsyncData(ActiveAskSheet(ask));
      rethrow;
    }
  }
}

/// Provider for the pinned-sheet state, keyed by target user id.
final profileSheetProvider = AsyncNotifierProvider.autoDispose
    .family<ProfileSheetNotifier, ProfileSheetState, String>(
  ProfileSheetNotifier.new,
);

/// Maps the community presence response onto the sheet-state union.
/// Communities have no cold-start kind; the ask card carries the
/// priority queue for the peek row.
ProfileSheetState mapCommunityPresenceToSheetState(
    GetCommunityPresenceForViewerResponse resp) {
  switch (resp.sheetKind) {
    case ProfileSheetKind.PROFILE_SHEET_KIND_ACTIVE_ASK:
      return resp.hasActiveAsk()
          ? ActiveAskSheet(resp.activeAsk, queued: resp.queuedAsks)
          : const QuietSheet();
    case ProfileSheetKind.PROFILE_SHEET_KIND_NEXT_EVENT:
      return resp.hasNextEvent()
          ? NextEventSheet(resp.nextEvent)
          : const QuietSheet();
    default:
      return const QuietSheet();
  }
}

/// AsyncNotifier for the pinned-sheet state of a community profile.
/// Same fallback-to-quiet contract as [ProfileSheetNotifier].
class CommunitySheetNotifier extends AsyncNotifier<ProfileSheetState> {
  CommunitySheetNotifier(this.communityId);

  /// Community id this notifier instance is bound to.
  final String communityId;

  @override
  Future<ProfileSheetState> build() async {
    try {
      final resp = await ref
          .read(communityRepositoryProvider)
          .getPresence(communityId);
      return mapCommunityPresenceToSheetState(resp);
    } catch (e) {
      _log.warning('community presence load failed, falling back: $e');
      return const QuietSheet();
    }
  }

  /// Commits the viewer to the sheet's ask; see
  /// [ProfileSheetNotifier.commitToAsk].
  Future<void> commitToAsk() async {
    final current = state.value;
    if (current is! ActiveAskSheet || current.ask.viewerCommitted) return;
    final ask = current.ask;
    final committed = ask.deepCopy()
      ..viewerCommitted = true
      ..committedCount = ask.committedCount + 1;
    state = AsyncData(ActiveAskSheet(committed, queued: current.queued));
    try {
      await ref.read(requestServiceProvider).offerToFulfill(
            requestId: ask.requestId,
            communityId: ask.communityId,
          );
    } catch (e) {
      if (!ref.mounted) return;
      state = AsyncData(ActiveAskSheet(ask, queued: current.queued));
      rethrow;
    }
  }

  /// Withdraws the viewer's commit; see [ProfileSheetNotifier.undoCommit].
  Future<void> undoCommit() async {
    final current = state.value;
    if (current is! ActiveAskSheet || !current.ask.viewerCommitted) return;
    final ask = current.ask;
    final reverted = ask.deepCopy()
      ..viewerCommitted = false
      ..committedCount = (ask.committedCount - 1).clamp(0, 1 << 30);
    state = AsyncData(ActiveAskSheet(reverted, queued: current.queued));
    try {
      await ref.read(requestServiceProvider).withdrawOffer(
            requestId: ask.requestId,
            communityId: ask.communityId,
          );
    } catch (e) {
      if (!ref.mounted) return;
      state = AsyncData(ActiveAskSheet(ask, queued: current.queued));
      rethrow;
    }
  }
}

/// Provider for the community pinned-sheet state, keyed by community id.
final communitySheetProvider = AsyncNotifierProvider.autoDispose
    .family<CommunitySheetNotifier, ProfileSheetState, String>(
  CommunitySheetNotifier.new,
);
