// Part of ExperienceService. Contains lifecycle RPCs via mixin:
// saveExperience, shareExperience, unshareExperience, cancelExperience,
// deleteExperience, markInProcess, completeExperience,
// undoCompleteExperience, previewExperienceImpact, cancelTimePoll.
part of '../experience_service.dart';

mixin ExperienceLifecycleMethods on ExperienceServiceBase {
  /// SaveExperience saves (creates or updates) an experience.
  /// If id is null or empty, creates a new experience.
  /// If id is provided, updates the existing experience.
  ///
  /// Returns the ID of the saved experience.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> saveExperience({
    String? id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    ExperienceTime? time,
    int? maxParticipants,
    String? sourceUrl,
    ExperienceMetadata? metadata,
    SocialContext? socialContext,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SaveExperienceRequest();

        if (id != null && id.isNotEmpty) {
          request.id = id;
        }
        if (name != null && name.isNotEmpty) {
          request.name = name;
        }
        if (description != null && description.isNotEmpty) {
          request.description = description;
        }
        if (sourceUrl != null && sourceUrl.isNotEmpty) {
          request.sourceUrl = sourceUrl;
        }
        if (mediaIds != null && mediaIds.isNotEmpty) {
          request.mediaIds.addAll(mediaIds);
        }
        if (locationId != null && locationId.isNotEmpty) {
          request.locationId = locationId;
        }
        if (time != null) {
          request.time = time;
        }
        if (maxParticipants != null && maxParticipants > 0) {
          request.maxParticipants = maxParticipants;
        }
        if (metadata != null) {
          request.metadata = metadata;
        }
        if (socialContext != null) {
          request.socialContext = socialContext;
        }
        final response = await _client.saveExperience(
          request,
          headers: _buildHeaders(),
        );

        return response.experience.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SaveExperience',
    );
  }

  /// ShareExperience shares an experience with a community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> shareExperience({
    required String experienceId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        // Converged onto CommunityService.ShareItem (#2526).
        final request = ShareItemRequest(shareToCommunityIds: [communityId])
          ..experienceId = experienceId;
        await _communityClient.shareItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ShareItem',
    );
  }

  /// UnshareExperience removes an experience from a specific community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> unshareExperience({
    required String experienceId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        // Converged onto CommunityService.UnshareItem (#2526).
        final request = UnshareItemRequest(communityId: communityId)
          ..experienceId = experienceId;
        await _communityClient.unshareItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UnshareItem',
    );
  }

  /// MarkInProcess marks an experience as in process (owner only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> markInProcess(String experienceId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = MarkExperienceInProcessRequest(
          experienceId: experienceId,
        );
        await _client.markExperienceInProcess(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'MarkExperienceInProcess',
    );
  }

  /// CompleteExperience marks an experience as completed (owner only).
  /// Optionally accepts a [summary] of how the experience went.
  ///
  /// Returns the response including updated experience and savings metrics.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<CompleteExperienceResponse> completeExperience(
    String experienceId, {
    String? summary,
    List<String>? confirmedAttendeeIds,
    int? confirmedAttendeeCount,
    QualityTimeAttributes? qualityTimeOverrides,
    MoneySavings? moneySavingsOverrides,
    PreventedEmissions? emissionsOverrides,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CompleteExperienceRequest(experienceId: experienceId);
        if (summary != null && summary.isNotEmpty) {
          request.summary = summary;
        }
        // The confirmed set previewed in the completion modal — the server
        // reuses it so the committed impact matches the preview (#2724).
        if (confirmedAttendeeIds != null) {
          request.confirmedAttendeeIds.addAll(confirmedAttendeeIds);
        }
        if (confirmedAttendeeCount != null && confirmedAttendeeCount > 0) {
          request.confirmedAttendeeCount = confirmedAttendeeCount;
        }
        if (qualityTimeOverrides != null) {
          request.qualityTimeOverrides = qualityTimeOverrides;
        }
        if (moneySavingsOverrides != null) {
          request.moneySavingsOverrides = moneySavingsOverrides;
        }
        if (emissionsOverrides != null) {
          request.emissionsOverrides = emissionsOverrides;
        }
        final response = await _client.completeExperience(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CompleteExperience',
    );
  }

  /// UndoCompleteExperience undoes a prior CompleteExperience call.
  ///
  /// Throws [ServiceException] when the action cannot be reversed; the underlying
  /// ConnectException carries an UndoErrorDetail for typed handling.
  Future<void> undoCompleteExperience({
    required String communityEventId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UndoCompleteExperienceRequest(
          communityEventId: communityEventId,
        );
        await _client.undoCompleteExperience(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoCompleteExperience',
    );
  }

  /// PreviewExperienceImpact computes a live impact estimate for the completion
  /// modal without persisting anything.
  ///
  /// [confirmedAttendeeIds] is the list of confirmed registered-user attendee IDs.
  /// [confirmedAttendeeCount] is the total including provisional users; when > 0 it
  /// overrides the length of [confirmedAttendeeIds] for group-size computation.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ImpactEstimate> previewExperienceImpact({
    required String experienceId,
    List<String> confirmedAttendeeIds = const [],
    int confirmedAttendeeCount = 0,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = PreviewExperienceImpactRequest(
          experienceId: experienceId,
          confirmedAttendeeCount: confirmedAttendeeCount,
        );
        request.confirmedAttendeeIds.addAll(confirmedAttendeeIds);
        final response = await _client.previewExperienceImpact(
          request,
          headers: _buildHeaders(),
        );
        return response.impact;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'PreviewExperienceImpact',
    );
  }

  /// CancelExperience marks an experience as cancelled (owner only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> cancelExperience(String experienceId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CancelExperienceRequest(experienceId: experienceId);
        await _client.cancelExperience(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CancelExperience',
    );
  }

  /// DeleteExperience deletes an experience (owner only).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteExperience(String experienceId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteExperienceRequest(experienceId: experienceId);
        await _client.deleteExperience(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteExperience',
    );
  }

  /// CancelTimePoll cancels the active time poll for an experience.
  ///
  /// Only the experience owner can cancel a poll.
  /// Deletes all proposals and clears the time_poll_active flag.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> cancelTimePoll({required String experienceId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CancelTimePollRequest(experienceId: experienceId);
        await _client.cancelTimePoll(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CancelTimePoll',
    );
  }
}
