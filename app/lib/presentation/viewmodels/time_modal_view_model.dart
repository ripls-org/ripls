import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:protobuf/protobuf.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/services/providers.dart';

import 'time_modal_state.dart';

export 'time_modal_state.dart';

final _log = Logger('TimeModalViewModel');

/// AsyncNotifier for managing time modal state.
///
/// Uses AsyncNotifier pattern which automatically handles disposal - no manual
/// `ref.mounted` checks needed in build(). The `build()` method loads data
/// automatically when the provider is first accessed.
///
/// Family pattern: pass experienceId when accessing the provider.
class TimeModalNotifier extends AsyncNotifier<TimeModalData> {
  /// Constructor accepts the experienceId parameter from the family modifier
  TimeModalNotifier(this.experienceId);

  /// The experienceId for this specific time modal instance
  final String experienceId;

  ExperienceRepository get _repository =>
      ref.read(experienceRepositoryProvider);

  /// Resolves the user's IANA timezone string via the central provider.
  ///
  /// Falls back to the device's local timezone when the user hasn't set a
  /// preference. This is used when creating time proposals so the stored
  /// timezone reflects the user's actual location rather than 'UTC'.
  Future<String> _resolveUserTimezone() async {
    return await ref.read(resolvedTimezoneProvider.future);
  }

  @override
  Future<TimeModalData> build() async {
    // Automatic disposal handling — no ref.mounted checks needed in build()
    ref.onDispose(() {
      _log.fine('TimeModalNotifier disposed for experience: $experienceId');
    });

    // Bail before any async work if the user is not authenticated. A rebuild
    // triggered by `ref.invalidateSelf()` from an in-flight mutation can race
    // logout; another RPC here would write state into the logout frame and
    // crash the IndexedStack with a duplicate NavigatorState GlobalKey.
    if (ref.read(authStateProvider).user == null) {
      return const TimeModalData();
    }

    _log.info('Loading time proposals for experience: $experienceId');

    // Always invalidate to ensure fresh vote data is loaded from the server,
    // including votes cast via other paths (e.g., inline chat voting card).
    await _repository.invalidate(experienceId);

    final response = await _repository.getExperienceDetails(experienceId);
    final authState = ref.read(authStateProvider);
    if (authState.user == null) {
      return const TimeModalData();
    }
    final currentUserId = authState.user?.id;

    // Find locked proposal
    final lockedProposal = response.experience.timeProposals
        .where((p) => p.isConfirmed)
        .firstOrNull;

    // Extract current user's votes
    final userVotes = <String, TimeVoteStatus>{};
    for (final proposal in response.experience.timeProposals) {
      final userVote = proposal.votes
          .where((v) => v.user.id == currentUserId)
          .firstOrNull;
      if (userVote != null) {
        userVotes[proposal.id] = userVote.status;
      }
    }

    // Determine if experience is in a terminal state (read-only)
    final experienceState = response.experience.state;
    final isReadOnly = experienceState == ExperienceState.EXPERIENCE_STATE_COMPLETED ||
        experienceState == ExperienceState.EXPERIENCE_STATE_CANCELLED;

    // Determine initial mode based on whether time is locked
    final initialMode = lockedProposal != null ? 'confirmed' : 'default';

    // Determine if suggestions are allowed by the server
    // Only allow when: not read-only, no locked time, and in an active state
    final allowSuggestions = !isReadOnly &&
        lockedProposal == null &&
        (experienceState == ExperienceState.EXPERIENCE_STATE_ACTIVE ||
         experienceState == ExperienceState.EXPERIENCE_STATE_JOINED);

    // Poll fields: read time_poll_active from API response
    final serverPollActive = response.experience.timePollActive;

    // Auto-select the proposal that matches the event time
    String? initialSelectedProposalId;
    if (response.experience.time.hasSpecific()) {
      final eventStartTime = DateTime.fromMillisecondsSinceEpoch(
        response.experience.time.specific.unixTimestampSec.toInt() * 1000,
      );

      // Find proposal matching event time
      for (final proposal in response.experience.timeProposals) {
        if (proposal.time.hasSpecific()) {
          final proposalStart = DateTime.fromMillisecondsSinceEpoch(
            proposal.time.specific.unixTimestampSec.toInt() * 1000,
          );
          if (_isSameDateTime(proposalStart, eventStartTime)) {
            initialSelectedProposalId = proposal.id;
            break;
          }
        }
      }
    }

    // Preserve existing selection if user has already made a choice, otherwise use auto-selected
    final selectedProposalId = (state.hasValue ? state.value!.selectedProposalId : null) ?? initialSelectedProposalId;

    // Pre-populate "Set time" form fields from the current event time
    // so the organizer sees the form ready to edit without extra taps.
    // Preserve user-edited values across refreshes by only initializing when null.
    final isOrganizer = response.experience.owner.id == currentUserId;
    DateTime? proposeDate;
    TimeOfDay? proposeTime;
    int? proposeDuration;
    if (isOrganizer && !serverPollActive && response.experience.time.hasSpecific()) {
      if (state.hasValue && state.value!.proposeDate != null) {
        // Preserve whatever the user has already edited
        proposeDate = state.value!.proposeDate;
        proposeTime = state.value!.proposeTime;
        proposeDuration = state.value!.proposeDurationMinutes;
      } else {
        // Initialize from event time on first load
        final dt = DateTime.fromMillisecondsSinceEpoch(
          response.experience.time.specific.unixTimestampSec.toInt() * 1000,
        );
        proposeDate = dt;
        proposeTime = TimeOfDay(hour: dt.hour, minute: dt.minute);
        proposeDuration = response.experience.time.specific.durationMinutes;
      }
    }

    return TimeModalData(
      proposals: response.experience.timeProposals,
      lockedProposalId: lockedProposal?.id,
      isOrganizer: isOrganizer,
      isReadOnly: isReadOnly,
      currentUserVotes: userVotes,
      eventTime: response.experience.time,
      currentUserId: currentUserId,
      mode: initialMode,
      canSuggest: allowSuggestions, // Server-side permission
      selectedProposalId: selectedProposalId, // Auto-select event time proposal
      timePollActive: serverPollActive,
      timePollCompleted: response.experience.hasTimePollCompleted() &&
          response.experience.timePollCompleted,
      proposeDate: proposeDate,
      proposeTime: proposeTime,
      proposeDurationMinutes: proposeDuration,
      currentPollId: response.experience.hasCurrentPollId()
          ? response.experience.currentPollId
          : null,
      proposalsLocked: response.experience.hasTimeProposalsLocked() &&
          response.experience.timeProposalsLocked,
      pollDeadlineUnixSec: response.experience.hasTimePollDeadlineUnixSec()
          ? response.experience.timePollDeadlineUnixSec.toInt()
          : null,
    );
  }

  /// Votes on a time proposal with toggle behavior.
  ///
  /// Tap to vote yes, tap again to remove vote. No explicit "no" vote.
  /// Methods that manually set state still need ref.mounted checks (per architecture doc).
  Future<void> voteOnTime(String proposalId) async {
    final previous = state.requireValue;
    final currentVote = previous.currentUserVotes[proposalId];

    // Toggle behavior: if already voted yes, remove vote; otherwise vote yes
    final newVote = currentVote == TimeVoteStatus.TIME_VOTE_STATUS_YES
        ? TimeVoteStatus.TIME_VOTE_STATUS_UNSPECIFIED
        : TimeVoteStatus.TIME_VOTE_STATUS_YES;

    // Optimistic update
    final updatedVotes = {...previous.currentUserVotes};
    if (newVote == TimeVoteStatus.TIME_VOTE_STATUS_UNSPECIFIED) {
      updatedVotes.remove(proposalId);
    } else {
      updatedVotes[proposalId] = newVote;
    }

    state = AsyncData(previous.copyWith(currentUserVotes: updatedVotes));

    try {
      await _repository.voteOnTime(
        proposalId: proposalId,
        status: newVote,
        experienceId: experienceId,
      );

      if (!ref.mounted) return; // Required after async gap

      // Refresh from server
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to vote on time: $e');

      if (!ref.mounted) return;

      // Rollback on failure
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Marks (or clears) the current user as flexible — "any time works" — on
  /// the active poll, with optimistic UI.
  ///
  /// Mirrors [LocationModalNotifier.setFlexibleOnLocation]: the flexible
  /// marker is recorded as `FLEXIBLE` on a single representative proposal
  /// (the first in the current poll), and [TimeModalData] folds flexible
  /// voters into every option's effective tally. No-op when the poll has no
  /// proposals to attach the marker to.
  Future<void> setFlexibleOnTime(bool flexible) async {
    final previous = state.requireValue;
    final representative = previous.currentPollProposals.firstOrNull;
    if (representative == null) return;
    final voter = ref.read(authStateProvider).user;
    final newStatus = flexible
        ? TimeVoteStatus.TIME_VOTE_STATUS_FLEXIBLE
        : TimeVoteStatus.TIME_VOTE_STATUS_UNSPECIFIED;

    // Optimistically project the flexible vote onto the representative
    // proposal so the row reflects the change before the server refresh.
    final optimisticProposals = previous.proposals.map((p) {
      if (p.id != representative.id || voter == null || voter.id.isEmpty) {
        return p;
      }
      final cloned = p.deepCopy();
      cloned.votes.removeWhere((v) => v.user.id == voter.id);
      if (newStatus != TimeVoteStatus.TIME_VOTE_STATUS_UNSPECIFIED) {
        cloned.votes.add(TimeVote()
          ..status = newStatus
          ..user = voter);
      }
      return cloned;
    }).toList();
    state = AsyncData(previous.copyWith(proposals: optimisticProposals));

    try {
      await _repository.voteOnTime(
        proposalId: representative.id,
        status: newStatus,
        experienceId: experienceId,
      );
      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to set flexible vote on time: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Locks a time proposal (organizer only). Sets mode to 'confirmed'.
  Future<void> lockTime(String proposalId) async {
    final previous = state.requireValue;

    // Optimistic update - set locked proposal and transition to confirmed mode
    state = AsyncData(previous.copyWith(
      lockedProposalId: proposalId,
      mode: 'confirmed',
      selectedProposalId: null, // Clear selection after locking
    ));

    try {
      await _repository.confirmTime(
        experienceId: experienceId,
        proposalId: proposalId,
      );

      if (!ref.mounted) return; // Required after async gap

      // Refresh from server
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to lock time: $e');

      if (!ref.mounted) return;

      // Rollback on failure
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Unlocks a previously confirmed time (organizer only).
  Future<void> unlockTime() async {
    final previous = state.requireValue;

    // Optimistic update
    state = AsyncData(previous.copyWith(lockedProposalId: null));

    try {
      await _repository.unlockTime(experienceId: experienceId);

      if (!ref.mounted) return; // Required after async gap

      // Refresh from server
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to unlock time: $e');

      if (!ref.mounted) return;

      // Rollback on failure
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Proposes a new time with the current form state.
  Future<void> proposeTime() async {
    final current = state.requireValue;

    if (current.proposeDate == null || current.proposeTime == null) {
      throw Exception('Date and time are required to propose a time');
    }

    // Build ExperienceTime from form state
    final dateTime = DateTime(
      current.proposeDate!.year,
      current.proposeDate!.month,
      current.proposeDate!.day,
      current.proposeTime!.hour,
      current.proposeTime!.minute,
    );

    final unixTimestamp = dateTime.millisecondsSinceEpoch ~/ 1000;
    final userTz = await _resolveUserTimezone();

    final time = ExperienceTime(
      specific: SpecificTime(
        unixTimestampSec: Int64(unixTimestamp),
        timezone: userTz,
        durationMinutes: current.proposeDurationMinutes ?? 60,
      ),
    );

    // Check uniqueness against existing proposals
    for (final existing in current.proposals) {
      if (existing.time.hasSpecific()) {
        final existingStart = DateTime.fromMillisecondsSinceEpoch(
          existing.time.specific.unixTimestampSec.toInt() * 1000,
        );
        if (_isSameDateTime(existingStart, dateTime)) {
          throw Exception('This time has already been proposed');
        }
      }
    }

    try {
      await _repository.proposeTime(
        experienceId: experienceId,
        time: time,
      );

      if (!ref.mounted) return; // Required after async gap

      // Refresh and exit propose mode
      ref.invalidateSelf();

      // Exit propose mode after successful proposal
      exitProposeMode();
    } catch (e) {
      _log.warning('Failed to propose time: $e');
      rethrow;
    }
  }

  // Synchronous state updates — no ref.mounted checks needed

  
  
  /// Enters propose mode to show the time proposal form.
  /// Pre-fills the form with the current event time if it exists.
  void enterProposeMode() {
    final current = state.requireValue;

    // Smart defaults: pre-select event time values
    DateTime? defaultDate;
    TimeOfDay? defaultTime;
    int? defaultDuration;

    if (current.eventTime != null && current.eventTime!.hasSpecific()) {
      final startTime = DateTime.fromMillisecondsSinceEpoch(
        current.eventTime!.specific.unixTimestampSec.toInt() * 1000,
      );
      defaultDate = startTime;
      defaultTime = TimeOfDay(hour: startTime.hour, minute: startTime.minute);
      defaultDuration = current.eventTime!.specific.durationMinutes;
    }

    state = AsyncData(current.copyWith(
      mode: 'propose',
      proposeDate: defaultDate,
      proposeTime: defaultTime,
      proposeDurationMinutes: defaultDuration,
    ));
  }

  
  /// Exits propose/edit mode to return to the time list.
  void exitProposeMode() {
    final current = state.requireValue;
    final previousMode = current.mode == 'confirmed' ? 'confirmed' : 'default';
    state = AsyncData(current.copyWith(
      mode: previousMode,
      proposeDate: null,
      proposeTime: null,
      proposeDurationMinutes: null,
    ));
  }

  
  
  /// Updates the proposed date.
  void updateProposeDate(DateTime date) {
    final current = state.requireValue;
    state = AsyncData(current.copyWith(proposeDate: date));
  }

  /// Updates the proposed time.
  void updateProposeTime(TimeOfDay time) {
    final current = state.requireValue;
    state = AsyncData(current.copyWith(proposeTime: time));
  }

  /// Updates the proposed duration in minutes.
  void updateProposeDuration(int minutes) {
    final current = state.requireValue;
    state = AsyncData(current.copyWith(proposeDurationMinutes: minutes));
  }

  /// Sets a single candidate as the actual event time, bypassing the poll
  /// flow. Used when the user stages exactly one time in the propose modal
  /// and taps "Set the time" — the singular CTA conveys a direct-set
  /// contract, so we call SaveExperience directly (like [updateEventTime])
  /// rather than opening a transient one-option poll that leaks a spurious
  /// "Poll ended" chat banner (see #2598).
  ///
  /// Precondition: there must not already be an active time poll — callers
  /// in live-edit mode should hit [addTimesToActivePoll] instead.
  Future<void> setSingleTime(DateTime dt) async {
    final previous = state.requireValue;
    assert(
      !previous.timePollActive,
      'setSingleTime must not be called while a poll is active',
    );
    try {
      final userTz = await _resolveUserTimezone();
      if (!ref.mounted) return;
      final unixTimestamp = dt.millisecondsSinceEpoch ~/ 1000;
      final time = ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: Int64(unixTimestamp),
          timezone: userTz,
          durationMinutes: 60,
        ),
      );
      final response = await _repository.getExperienceDetails(experienceId);
      if (!ref.mounted) return;
      final experience = response.experience;
      await _repository.saveExperience(
        id: experienceId,
        name: experience.name,
        description: experience.description,
        mediaIds: experience.mediaIds,
        locationId: experience.locationId,
        time: time,
        maxParticipants: experience.maxParticipants > 0
            ? experience.maxParticipants
            : null,
        sourceUrl: experience.sourceUrl.isNotEmpty
            ? experience.sourceUrl
            : null,
      );
      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to set single time: $e');
      if (!ref.mounted) return;
      ref.invalidateSelf();
      rethrow;
    }
  }

  /// Creates a time poll by proposing all options sequentially (owner only).
  ///
  /// Sequential rather than parallel: the server emits a "poll opened" system
  /// chat message only when [TimePollActive] flips from false to true. If we
  /// fired all ProposeTime requests in parallel they would race — every
  /// concurrent request would see the flag still false and each would emit its
  /// own duplicate system message. Awaiting each call guarantees the first
  /// request flips the flag, and subsequent requests see it already true and
  /// stay quiet.
  ///
  /// Any single ProposeTime failure propagates and aborts the rest. Partial
  /// proposals may have been saved; the owner can cancel the poll to clean up.
  Future<void> createTimePoll(List<(DateTime, TimeOfDay)> options) async {
    if (!ref.mounted) return;
    state = const AsyncLoading();

    try {
      final userTz = await _resolveUserTimezone();
      for (final opt in options) {
        final (date, time) = opt;
        final dateTime = DateTime(
          date.year, date.month, date.day, time.hour, time.minute,
        );
        final unixTimestamp = dateTime.millisecondsSinceEpoch ~/ 1000;
        await _repository.proposeTime(
          experienceId: experienceId,
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixTimestamp),
              timezone: userTz,
              durationMinutes: 60,
            ),
          ),
        );
        if (!ref.mounted) return;
      }

      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to create time poll: $e');
      if (!ref.mounted) return;
      // Refresh to resolve partial state
      ref.invalidateSelf();
      rethrow;
    }
  }

  /// Adds additional time options to an already-active poll (any participant).
  ///
  /// Unlike [proposeTime], this does not touch the form state (`proposeDate` /
  /// `proposeTime`) and does not validate against existing proposals — every
  /// option is proposed as-is. Calls [_repository.proposeTime] sequentially so
  /// requests don't race, then invalidates the provider once at the end.
  Future<void> addTimesToActivePoll(List<(DateTime, TimeOfDay)> options) async {
    if (!ref.mounted) return;
    if (options.isEmpty) return;

    try {
      final userTz = await _resolveUserTimezone();
      for (final opt in options) {
        final (date, time) = opt;
        final dateTime = DateTime(
          date.year, date.month, date.day, time.hour, time.minute,
        );
        final unixTimestamp = dateTime.millisecondsSinceEpoch ~/ 1000;
        await _repository.proposeTime(
          experienceId: experienceId,
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(unixTimestamp),
              timezone: userTz,
              durationMinutes: 60,
            ),
          ),
        );
        if (!ref.mounted) return;
      }

      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to add times to active poll: $e');
      if (!ref.mounted) return;
      ref.invalidateSelf();
      rethrow;
    }
  }

  /// Parses a free-form description (e.g. "tonight, tomorrow night, or
  /// Monday afternoon") into one or more [ExperienceTime] candidates via
  /// the AI provider. Returns the empty list when no segment parsed into
  /// a usable specific or range time so the caller can show a friendly
  /// hint. Mirrors [extractLocationCandidates] on the location flow.
  Future<List<ExperienceTime>> extractTimeCandidates(String description) async {
    final trimmed = description.trim();
    if (trimmed.isEmpty) return const [];
    try {
      final timezone = await ref.read(resolvedTimezoneProvider.future);
      return await _repository.extractTimeCandidates(
        text: trimmed,
        currentTimeUnixSec: DateTime.now().millisecondsSinceEpoch ~/ 1000,
        timezone: timezone,
      );
    } catch (e) {
      _log.warning('Failed to extract time candidates: $e');
      rethrow;
    }
  }

  /// Locks (or unlocks) the proposal list on the active poll (owner only).
  Future<void> lockTimeProposals(bool locked) async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(proposalsLocked: locked));
    try {
      await _repository.lockTimeProposals(
        experienceId: experienceId,
        locked: locked,
      );
      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to lock time proposals: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Pings unreplied voters with a reminder push (owner only). Returns the
  /// count notified.
  Future<int> nudgeTimePollVoters() async {
    try {
      return await _repository.nudgeTimePollVoters(experienceId: experienceId);
    } catch (e) {
      _log.warning('Failed to nudge time-poll voters: $e');
      rethrow;
    }
  }

  /// Sets (or clears) the reply-by deadline on the active poll (owner only).
  /// Pass `deadlineUnixSec=0` to clear.
  Future<void> setTimePollDeadline(int deadlineUnixSec) async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(
      pollDeadlineUnixSec: deadlineUnixSec <= 0 ? null : deadlineUnixSec,
    ));
    try {
      await _repository.setTimePollDeadline(
        experienceId: experienceId,
        deadlineUnixSec: deadlineUnixSec,
      );
      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to set time-poll deadline: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Deletes a proposal from the active poll. Owner can delete any proposal;
  /// participants may delete only their own.
  Future<void> deleteTimeProposal(String proposalId) async {
    try {
      await _repository.deleteTimeProposal(
        experienceId: experienceId,
        proposalId: proposalId,
      );
      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to delete time proposal: $e');
      rethrow;
    }
  }

  /// Ends the active time poll (owner only).
  ///
  /// Calls CancelTimePoll RPC, resets timePollActive, clears form fields so
  /// the "Set time" view re-initializes from the current event time, giving
  /// the owner an immediate path to update the time after ending the poll.
  Future<void> endTimePoll() async {
    final previous = state.requireValue;

    if (!ref.mounted) return;
    // Clear form fields so build() re-initializes them from the event time.
    state = AsyncData(previous.copyWith(
      timePollActive: false,
      proposeDate: null,
      proposeTime: null,
      proposeDurationMinutes: null,
    ));

    try {
      await _repository.cancelTimePoll(experienceId: experienceId);

      if (!ref.mounted) return;
      ref.invalidateSelf();
    } catch (e) {
      _log.warning('Failed to end time poll: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Helper method to check if two DateTimes represent the same date and time
  /// (ignoring seconds and milliseconds).
  bool _isSameDateTime(DateTime a, DateTime b) {
    return a.year == b.year &&
           a.month == b.month &&
           a.day == b.day &&
           a.hour == b.hour &&
           a.minute == b.minute;
  }
}

/// Provider for time modal view model.
///
/// Uses family pattern - pass experienceId to get time modal state for that experience.
/// Data loads automatically when provider is first watched.
/// State persists across modal open/close to preserve UI toggle state.
///
/// Example:
/// ```dart
/// final timeDataAsync = ref.watch(timeModalProvider(experienceId));
/// return timeDataAsync.when(
///   data: (data) => _buildContent(data),
///   loading: () => CircularProgressIndicator(),
///   error: (e, st) => _buildError(e.toString()),
/// );
/// ```
final timeModalProvider =
    AsyncNotifierProvider.family<TimeModalNotifier, TimeModalData, String>(
  TimeModalNotifier.new,
);
