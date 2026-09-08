// Part of ExperienceService. Contains location-poll RPCs via mixin:
// proposeLocation, voteOnLocation, confirmLocation, unlockLocation,
// cancelLocationPoll.
part of '../experience_service.dart';

mixin ExperienceLocationPollMethods on ExperienceServiceBase {
  /// ProposeLocation creates a new candidate location for an experience.
  ///
  /// The proposal either references a saved location (via [locationId]) or
  /// carries an inline geocoded location (drop-pin / unsaved search result).
  /// Exactly one of [locationId] / [geocoded] must be set.
  Future<LocationProposal> proposeLocation({
    required String experienceId,
    String? locationId,
    GeocodedLocation? geocoded,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final loc = ProposedLocation();
        if (locationId != null && locationId.isNotEmpty) {
          loc.locationId = locationId;
        } else if (geocoded != null) {
          loc.geocoded = geocoded;
        } else {
          throw ArgumentError('proposeLocation requires either locationId or geocoded');
        }
        final request = ProposeLocationRequest(
          experienceId: experienceId,
          location: loc,
        );
        final response = await _client.proposeLocation(
          request,
          headers: _buildHeaders(),
        );
        return response.proposal;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ProposeLocation',
    );
  }

  /// VoteOnLocation records a user's vote on a location proposal.
  Future<void> voteOnLocation({
    required String proposalId,
    required LocationVoteStatus status,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = VoteOnLocationRequest(
          proposalId: proposalId,
          status: status,
        );
        await _client.voteOnLocation(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'VoteOnLocation',
    );
  }

  /// ConfirmLocation confirms a location proposal as the final location.
  ///
  /// Only the experience owner can confirm. If the winning proposal carries
  /// an inline geocoded location, the server materializes it into a stored
  /// Location and updates the experience's location_id. Returns the
  /// resolved location_id so the caller can patch its local state.
  Future<String> confirmLocation({
    required String experienceId,
    required String proposalId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ConfirmLocationRequest(
          experienceId: experienceId,
          proposalId: proposalId,
        );
        final response =
            await _client.confirmLocation(request, headers: _buildHeaders());
        return response.locationId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ConfirmLocation',
    );
  }

  /// UnlockLocation unlocks a previously confirmed location (owner only).
  Future<void> unlockLocation({required String experienceId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UnlockLocationRequest(experienceId: experienceId);
        await _client.unlockLocation(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnlockLocation',
    );
  }

  /// CancelLocationPoll ends the active location poll (owner only).
  /// Proposals and votes are preserved for read-only results display.
  Future<void> cancelLocationPoll({required String experienceId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CancelLocationPollRequest(experienceId: experienceId);
        await _client.cancelLocationPoll(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CancelLocationPoll',
    );
  }

  /// DeleteLocationProposal removes a proposal (and its votes) from a poll.
  /// Owner may delete any proposal; participants may delete their own.
  Future<void> deleteLocationProposal({
    required String experienceId,
    required String proposalId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteLocationProposalRequest(
          experienceId: experienceId,
          proposalId: proposalId,
        );
        await _client.deleteLocationProposal(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteLocationProposal',
    );
  }

  /// SetLocationPollDeadline records (or clears) the reply-by deadline on
  /// the active location poll. Pass `deadlineUnixSec=0` to clear.
  Future<void> setLocationPollDeadline({
    required String experienceId,
    required int deadlineUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SetLocationPollDeadlineRequest(
          experienceId: experienceId,
          deadlineUnixSec: Int64(deadlineUnixSec),
        );
        await _client.setLocationPollDeadline(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetLocationPollDeadline',
    );
  }

  /// LockLocationProposals freezes the list of proposals on the active
  /// poll. While locked, ProposeLocation returns FailedPrecondition.
  Future<void> lockLocationProposals({
    required String experienceId,
    required bool locked,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = LockLocationProposalsRequest(
          experienceId: experienceId,
          locked: locked,
        );
        await _client.lockLocationProposals(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'LockLocationProposals',
    );
  }

  /// NudgeLocationPollVoters dispatches a reminder push to Yes/Maybe RSVPs
  /// who haven't voted on the current poll. Returns the count notified.
  Future<int> nudgeLocationPollVoters({required String experienceId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            NudgeLocationPollVotersRequest(experienceId: experienceId);
        final response = await _client.nudgeLocationPollVoters(
          request,
          headers: _buildHeaders(),
        );
        return response.nudgedCount;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'NudgeLocationPollVoters',
    );
  }

  /// ExtractLocationCandidates parses a free-form text message into
  /// geocoded location candidates. Used by the propose modal's
  /// bulk-paste shortcut.
  Future<List<GeocodedLocation>> extractLocationCandidates({
    required String text,
    double? latitudeDeg,
    double? longitudeDeg,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ExtractLocationCandidatesRequest(text: text);
        if (latitudeDeg != null) request.latitudeDeg = latitudeDeg;
        if (longitudeDeg != null) request.longitudeDeg = longitudeDeg;
        final response = await _client.extractLocationCandidates(
          request,
          headers: _buildHeaders(),
        );
        return response.candidates;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ExtractLocationCandidates',
    );
  }
}
