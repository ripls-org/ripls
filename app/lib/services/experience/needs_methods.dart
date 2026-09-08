// Part of ExperienceService. Contains needs and contributions RPCs via mixin:
// listNeedsAndContributions, addNeed, removeNeed, claimNeed, unclaimNeed,
// addContribution, batchAddNeeds, batchAddContributions,
// editContribution, removeContribution.
part of '../experience_service.dart';

mixin ExperienceNeedsMethods on ExperienceServiceBase {
  /// ListNeedsAndContributions lists all needs and contributions for an experience.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ListExperienceNeedsAndContributionsResponse> listNeedsAndContributions({
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.listExperienceNeedsAndContributions(
          ListExperienceNeedsAndContributionsRequest(
            experienceId: experienceId,
          ),
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListExperienceNeedsAndContributions',
    );
  }

  /// AddNeed adds a need to an experience (any participant).
  ///
  /// [slots] defaults to 1 when omitted or zero.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ExperienceNeedResponse> addNeed({
    required String experienceId,
    required String name,
    String? note,
    int slots = 1,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = AddExperienceNeedRequest(
          experienceId: experienceId,
          name: name,
          slots: slots,
        );
        if (note != null) request.note = note;
        final response = await _client.addExperienceNeed(
          request,
          headers: _buildHeaders(),
        );
        return response.need;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddExperienceNeed',
    );
  }

  /// RemoveNeed removes a need from an experience (proposer only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> removeNeed({
    required String needId,
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.removeExperienceNeed(
          RemoveExperienceNeedRequest(
            needId: needId,
            experienceId: experienceId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveExperienceNeed',
    );
  }

  /// UpdateNeed edits an existing need in place (proposer only). Any
  /// non-null field replaces the current value; null fields are
  /// preserved. Reducing slots below the current claim count fails
  /// server-side with FailedPrecondition.
  Future<ExperienceNeedResponse> updateNeed({
    required String needId,
    required String experienceId,
    String? name,
    String? note,
    int? slots,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateExperienceNeedRequest(
          needId: needId,
          experienceId: experienceId,
        );
        if (name != null) request.name = name;
        if (note != null) request.note = note;
        if (slots != null) request.slots = slots;
        final response = await _client.updateExperienceNeed(
          request,
          headers: _buildHeaders(),
        );
        return response.need;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateExperienceNeed',
    );
  }

  /// NudgeUncoveredNeedClaimers pings every YES/MAYBE RSVP who hasn't
  /// created any contribution yet (organizer only). Returns the count
  /// of users notified.
  Future<int> nudgeUncoveredNeedClaimers({
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.nudgeUncoveredNeedClaimers(
          NudgeUncoveredNeedClaimersRequest(experienceId: experienceId),
          headers: _buildHeaders(),
        );
        return response.nudgedCount;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'NudgeUncoveredNeedClaimers',
    );
  }

  /// ClaimNeed claims a need slot, creating a linked contribution (any participant).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ExperienceContributionResponse> claimNeed({
    required String needId,
    required String experienceId,
    String? note,
    String? gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ClaimExperienceNeedRequest(
          needId: needId,
          experienceId: experienceId,
        );
        if (note != null) request.note = note;
        if (gearId != null && gearId.isNotEmpty) request.gearId = gearId;
        final response = await _client.claimExperienceNeed(
          request,
          headers: _buildHeaders(),
        );
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ClaimExperienceNeed',
    );
  }

  /// UnclaimNeed returns a claimed contribution slot (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unclaimNeed({
    required String contributionId,
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.unclaimExperienceNeed(
          UnclaimExperienceNeedRequest(
            contributionId: contributionId,
            experienceId: experienceId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnclaimExperienceNeed',
    );
  }

  /// AddContribution adds a free-form contribution to an experience (any participant).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ExperienceContributionResponse> addContribution({
    required String experienceId,
    required String title,
    String? description,
    String? gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = AddExperienceContributionRequest(
          experienceId: experienceId,
          title: title,
        );
        if (description != null) request.description = description;
        if (gearId != null && gearId.isNotEmpty) request.gearId = gearId;
        final response = await _client.addExperienceContribution(
          request,
          headers: _buildHeaders(),
        );
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddExperienceContribution',
    );
  }

  /// BatchAddNeeds adds multiple needs to an experience in a single call (any participant).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<ExperienceNeedResponse>> batchAddNeeds({
    required String experienceId,
    required List<BatchNeedItem> items,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.batchAddExperienceNeeds(
          BatchAddExperienceNeedsRequest(
            experienceId: experienceId,
            items: items,
          ),
          headers: _buildHeaders(),
        );
        return response.needs;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'BatchAddExperienceNeeds',
    );
  }

  /// BatchAddContributions adds multiple free-form contributions to an experience
  /// in a single call (any participant).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<ExperienceContributionResponse>> batchAddContributions({
    required String experienceId,
    required List<BatchContributionItem> items,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.batchAddExperienceContributions(
          BatchAddExperienceContributionsRequest(
            experienceId: experienceId,
            items: items,
          ),
          headers: _buildHeaders(),
        );
        return response.contributions;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'BatchAddExperienceContributions',
    );
  }

  /// EditContribution edits an existing contribution (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ExperienceContributionResponse> editContribution({
    required String contributionId,
    required String experienceId,
    required String title,
    String? description,
    String? gearId,
    bool clearGearId = false,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = EditExperienceContributionRequest(
          contributionId: contributionId,
          experienceId: experienceId,
          title: title,
        );
        if (description != null) request.description = description;
        if (clearGearId) {
          request.clearGearId();
        } else if (gearId != null && gearId.isNotEmpty) {
          request.gearId = gearId;
        }
        final response = await _client.editExperienceContribution(
          request,
          headers: _buildHeaders(),
        );
        return response.contribution;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'EditExperienceContribution',
    );
  }

  /// RemoveContribution removes a contribution (contributor only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> removeContribution({
    required String contributionId,
    required String experienceId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.removeExperienceContribution(
          RemoveExperienceContributionRequest(
            contributionId: contributionId,
            experienceId: experienceId,
          ),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveExperienceContribution',
    );
  }
}
