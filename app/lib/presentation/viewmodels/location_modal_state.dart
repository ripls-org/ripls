import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';

part 'location_modal_state.freezed.dart';

/// State for the unified location modal. Mirrors [TimeModalData] but for the
/// location-poll flow.
///
/// Modes:
///   - `default`  — read-only display of the confirmed location (if any)
///                  plus the poll history list.
///   - `confirmed` — same as default but the experience has a locked location.
///   - `picking`   — owner is about to add a candidate (used to gate UI
///                   while the LocationPickerModal is open).
@freezed
sealed class LocationModalData with _$LocationModalData {
  const factory LocationModalData({
    @Default(<LocationProposal>[]) List<LocationProposal> proposals,
    String? lockedProposalId,
    @Default(false) bool isOrganizer,
    @Default(false) bool isReadOnly,
    @Default(<String>{}) Set<String> currentUserYesProposalIds,
    String? currentUserId,
    GeocodedLocation? eventGeocodedLocation,
    String? eventLocationId,
    @Default('default') String mode,
    @Default(false) bool locationPollActive,
    @Default(false) bool locationPollCompleted,
    String? currentLocationPollId,
    int? locationPollDeadlineUnixSec,
    @Default(false) bool locationProposalsLocked,
    String? selectedProposalId,
  }) = _LocationModalData;

  const LocationModalData._();

  
  /// Coarse status driving the morphing entry sheet:
  ///   - `poll` — a poll is active (voters weigh in).
  ///   - `set`  — a location is confirmed and no poll is running.
  ///   - `tbd`  — no confirmed location and no active poll (the default).
  String get pollStatus {
    if (locationPollActive) return 'poll';
    final hasValue =
        (eventLocationId != null && eventLocationId!.isNotEmpty) ||
            lockedProposalId != null;
    return hasValue ? 'set' : 'tbd';
  }

  /// Proposals that belong to the most recently started poll. Used to scope
  /// banner / FAB state — older polls' votes never trigger a "voted" badge
  /// on a newly opened poll.
  List<LocationProposal> get currentPollProposals {
    final pollId = currentLocationPollId;
    if (pollId == null || pollId.isEmpty) return proposals;
    return proposals.where((p) => p.hasPollId() && p.pollId == pollId).toList();
  }

  /// True when the current user has cast at least one YES vote on the
  /// currently active poll. Scoped to [currentLocationPollId] so historical
  /// poll votes don't pollute new-poll state.
  bool get hasVotedOnCurrentPoll {
    final pollId = currentLocationPollId;
    if (pollId == null || pollId.isEmpty) {
      return currentUserYesProposalIds.isNotEmpty;
    }
    return currentPollProposals
        .any((p) => currentUserYesProposalIds.contains(p.id));
  }

  /// User ids who marked themselves flexible ("any spot works") on the
  /// current poll. A flexible vote is stored as `FLEXIBLE` on a single
  /// representative proposal but counts toward every option, so it is
  /// folded into each proposal's effective tally and the leader calc.
  Set<String> get currentPollFlexibleVoterIds {
    final ids = <String>{};
    for (final p in currentPollProposals) {
      for (final v in p.votes) {
        if (v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_FLEXIBLE) {
          ids.add(v.user.id);
        }
      }
    }
    return ids;
  }

  /// Whether the current user has marked themselves flexible on this poll.
  bool get currentUserIsFlexible =>
      currentUserId != null &&
      currentPollFlexibleVoterIds.contains(currentUserId);

  /// Distinct user ids whose vote supports [proposalId]: explicit YES voters
  /// on that proposal plus everyone who is flexible.
  Set<String> effectiveVoterIds(String proposalId) {
    final ids = <String>{...currentPollFlexibleVoterIds};
    final proposal =
        proposals.where((p) => p.id == proposalId).firstOrNull;
    if (proposal != null) {
      for (final v in proposal.votes) {
        if (v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES) {
          ids.add(v.user.id);
        }
      }
    }
    return ids;
  }

  /// Effective support count for [proposalId] (YES voters folded with
  /// flexible voters, de-duplicated).
  int effectiveVoteCount(String proposalId) =>
      effectiveVoterIds(proposalId).length;

  /// Id of the proposal currently leading the poll by effective support, or
  /// null when no proposal has any support. Ties resolve to the
  /// earliest-listed proposal so the badge doesn't flicker between equals.
  String? get leadingProposalId {
    String? leadId;
    var leadCount = 0;
    for (final p in currentPollProposals) {
      final count = effectiveVoteCount(p.id);
      if (count > leadCount) {
        leadCount = count;
        leadId = p.id;
      }
    }
    return leadCount > 0 ? leadId : null;
  }

  /// Distinct user ids who have replied to the current poll in any way —
  /// an explicit YES on any proposal, or a flexible vote. Drives the
  /// reply-progress count.
  Set<String> get repliedUserIds {
    final ids = <String>{...currentPollFlexibleVoterIds};
    for (final p in currentPollProposals) {
      for (final v in p.votes) {
        if (v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES) {
          ids.add(v.user.id);
        }
      }
    }
    return ids;
  }
}
