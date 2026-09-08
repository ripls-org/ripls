import 'package:flutter/material.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';

part 'time_modal_state.freezed.dart';

/// Data for the Time Modal (loaded state only - AsyncValue handles loading/error)
@freezed
sealed class TimeModalData with _$TimeModalData {
  const factory TimeModalData({
    @Default([]) List<TimeProposal> proposals,
    String? lockedProposalId,
    @Default(false) bool isOrganizer,
    @Default(false) bool isReadOnly,
    @Default({}) Map<String, TimeVoteStatus> currentUserVotes, // proposalId -> vote status
    @Default('default') String mode, // "default" | "propose" | "confirmed" | "edit" | "poll"
    String? expandedProposalId,

    @Default(false) bool canSuggest, // Whether server allows suggestions (based on experience state)
    String? selectedProposalId, // For select/finalize flow
    ExperienceTime? eventTime, // Experience's main time for hero card
    String? currentUserId, // For "by You" display

    // Propose form state
    DateTime? proposeDate,
    TimeOfDay? proposeTime,
    int? proposeDurationMinutes,

    // Poll state — see [time_poll_propose_modal.dart] and
    // [time_poll_vote_modal.dart] for the surfaces that read these.
    @Default(false) bool timePollActive, // Server-side flag: owner has an active poll
    @Default(false) bool timePollCompleted, // Server-side flag: a poll was created and ended
    String? currentPollId, // Server-side identifier of the most recent poll
    @Default(false) bool proposalsLocked, // Server-side: LockTimeProposals applied
    int? pollDeadlineUnixSec, // Server-side: SetTimePollDeadline value
  }) = _TimeModalData;

  const TimeModalData._();

  /// Returns whether a time is locked
  bool get hasLockedTime => lockedProposalId != null;

  /// Coarse status driving the morphing entry sheet, mirroring
  /// [LocationModalData.pollStatus]:
  ///   - `poll` — a poll is active.
  ///   - `set`  — a time is confirmed and no poll is running.
  ///   - `tbd`  — no confirmed time and no active poll (the default).
  String get pollStatus {
    if (timePollActive) return 'poll';
    final hasValue = hasLockedTime ||
        (eventTime?.hasSpecific() ?? false) ||
        (eventTime?.hasRange() ?? false);
    return hasValue ? 'set' : 'tbd';
  }

  /// Returns whether the modal is in propose mode
  bool get isProposing => mode == 'propose';

  /// Returns whether the modal is in edit mode
  bool get isEditing => mode == 'edit';

  /// Returns whether the modal is in confirmed mode
  bool get isConfirmed => mode == 'confirmed';

  
  /// Returns the locked proposal if one exists
  TimeProposal? get lockedProposal {
    if (lockedProposalId == null) return null;
    return proposals
        .where((p) => p.id == lockedProposalId)
        .firstOrNull;
  }

  
  /// Returns the user's vote for a specific proposal
  TimeVoteStatus? getUserVote(String proposalId) =>
      currentUserVotes[proposalId];

  
  /// Proposals that belong to the most recently started poll. Mirrors
  /// [LocationModalData.currentPollProposals] so the two flows scope their
  /// vote math identically.
  List<TimeProposal> get currentPollProposals {
    final pollId = currentPollId;
    if (pollId == null || pollId.isEmpty) return proposals;
    return proposals.where((p) => p.hasPollId() && p.pollId == pollId).toList();
  }

  /// User ids who marked themselves flexible ("any time works") on the
  /// current poll. A flexible vote is stored as `FLEXIBLE` on a single
  /// representative proposal but counts toward every option.
  Set<String> get currentPollFlexibleVoterIds {
    final ids = <String>{};
    for (final p in currentPollProposals) {
      for (final v in p.votes) {
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_FLEXIBLE) {
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
    final proposal = proposals.where((p) => p.id == proposalId).firstOrNull;
    if (proposal != null) {
      for (final v in proposal.votes) {
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) {
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
  /// earliest-listed proposal.
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
  /// an explicit YES on any proposal, or a flexible vote.
  Set<String> get repliedUserIds {
    final ids = <String>{...currentPollFlexibleVoterIds};
    for (final p in currentPollProposals) {
      for (final v in p.votes) {
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) {
          ids.add(v.user.id);
        }
      }
    }
    return ids;
  }
  }
