// Part of ExperienceService. Contains RSVP and time scheduling RPCs via mixin:
// rsvp, recordAttendance, proposeTime, voteOnTime, confirmTime, unlockTime.
part of '../experience_service.dart';

mixin ExperienceRsvpTimeMethods on ExperienceServiceBase {
  /// Rsvp updates the user's RSVP intention for an experience.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> rsvp({
    required String experienceId,
    required String communityId,
    required RSVPIntention intention,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RSVPToExperienceRequest(
          experienceId: experienceId,
          communityId: communityId,
          intention: intention,
        );
        await _client.rSVPToExperience(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RSVPToExperience',
    );
  }

  /// SetMemberRsvp lets the host set an invitee's RSVP on their behalf from the
  /// Who's In roster. An UNSPECIFIED intention resets them to "invited".
  /// Owner-only server-side. Throws [ServiceException] on failure.
  Future<void> setMemberRsvp({
    required String experienceId,
    required String memberUserId,
    required RSVPIntention intention,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.setExperienceMemberRSVP(
          SetExperienceMemberRSVPRequest(
            experienceId: experienceId,
            memberUserId: memberUserId,
            intention: intention,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetExperienceMemberRSVP',
    );
  }

  /// RemoveMember uninvites a directly-invited individual from the experience.
  /// Owner-only server-side. Throws [ServiceException] on failure.
  Future<void> removeMember({
    required String experienceId,
    required String memberUserId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.removeExperienceMember(
          RemoveExperienceMemberRequest(
            experienceId: experienceId,
            memberUserId: memberUserId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveExperienceMember',
    );
  }

  /// RecordAttendance records attendance for multiple users (owner only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> recordAttendance({
    required String experienceId,
    required String communityId,
    required List<AttendanceRecord> attendance,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RecordAttendanceRequest(
          experienceId: experienceId,
          communityId: communityId,
          attendance: attendance,
        );
        await _client.recordAttendance(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RecordAttendance',
    );
  }

  /// ProposeTime creates a new time proposal for an experience.
  ///
  /// Only the owner or users who RSVP'd Yes/Maybe can propose times.
  /// Returns the created time proposal with votes.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<TimeProposal> proposeTime({
    required String experienceId,
    required ExperienceTime time,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ProposeTimeRequest(
          experienceId: experienceId,
          time: time,
        );
        final response = await _client.proposeTime(
          request,
          headers: _buildHeaders(),
        );

        return response.proposal;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ProposeTime',
    );
  }

  /// VoteOnTime records a user's vote on a time proposal.
  ///
  /// Only the owner or users who RSVP'd Yes/Maybe can vote on times.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> voteOnTime({
    required String proposalId,
    required TimeVoteStatus status,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = VoteOnTimeRequest(
          proposalId: proposalId,
          status: status,
        );
        await _client.voteOnTime(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'VoteOnTime',
    );
  }

  /// ConfirmTime confirms a time proposal as the final time for the experience.
  ///
  /// Only the experience owner can confirm times.
  /// This marks the proposal as confirmed and updates the experience's time field.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> confirmTime({
    required String experienceId,
    required String proposalId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ConfirmTimeRequest(
          experienceId: experienceId,
          proposalId: proposalId,
        );
        await _client.confirmTime(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ConfirmTime',
    );
  }

  /// UnlockTime unlocks a previously confirmed time for the experience.
  ///
  /// Only the experience owner can unlock times.
  /// This marks all confirmed proposals as not confirmed, allowing voting to continue.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unlockTime({
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UnlockTimeRequest(
          experienceId: experienceId,
        );
        await _client.unlockTime(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnlockTime',
    );
  }

  /// DeleteTimeProposal removes a single proposal (and its votes) from the
  /// active time poll. Owner can delete any proposal; participants may
  /// delete only their own.
  Future<void> deleteTimeProposal({
    required String experienceId,
    required String proposalId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteTimeProposalRequest(
          experienceId: experienceId,
          proposalId: proposalId,
        );
        await _client.deleteTimeProposal(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteTimeProposal',
    );
  }

  /// SetTimePollDeadline records (or clears) the reply-by deadline on the
  /// active time poll. Pass `deadlineUnixSec=0` to clear.
  Future<void> setTimePollDeadline({
    required String experienceId,
    required int deadlineUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SetTimePollDeadlineRequest(
          experienceId: experienceId,
          deadlineUnixSec: Int64(deadlineUnixSec),
        );
        await _client.setTimePollDeadline(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetTimePollDeadline',
    );
  }

  /// LockTimeProposals freezes the proposal list on the active poll. While
  /// locked, ProposeTime returns FailedPrecondition.
  Future<void> lockTimeProposals({
    required String experienceId,
    required bool locked,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = LockTimeProposalsRequest(
          experienceId: experienceId,
          locked: locked,
        );
        await _client.lockTimeProposals(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'LockTimeProposals',
    );
  }

  /// NudgeTimePollVoters dispatches a reminder push to Yes/Maybe RSVPs
  /// who haven't voted on the active poll. Returns the count notified.
  Future<int> nudgeTimePollVoters({required String experienceId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = NudgeTimePollVotersRequest(experienceId: experienceId);
        final response = await _client.nudgeTimePollVoters(
          request,
          headers: _buildHeaders(),
        );
        return response.nudgedCount;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'NudgeTimePollVoters',
    );
  }

  /// ExtractTimeCandidates parses a free-form text message into one or
  /// more [ExperienceTime] candidates. Used by the propose modal's
  /// bulk-paste shortcut — mirrors `extractLocationCandidates`.
  Future<List<ExperienceTime>> extractTimeCandidates({
    required String text,
    int? currentTimeUnixSec,
    String? timezone,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ExtractTimeCandidatesRequest(text: text);
        if (currentTimeUnixSec != null) {
          request.currentTimeUnixSec = Int64(currentTimeUnixSec);
        }
        if (timezone != null) request.timezone = timezone;
        final response = await _client.extractTimeCandidates(
          request,
          headers: _buildHeaders(),
        );
        return response.candidates;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ExtractTimeCandidates',
    );
  }
}
