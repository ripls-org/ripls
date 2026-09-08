// Part of ExperienceService. Contains read-only RPCs via mixin:
// getExperience, listExperiences, listMyExperiences,
// getExperiencePeople, getExperienceStats.
part of '../experience_service.dart';

mixin ExperienceFetchMethods on ExperienceServiceBase {
  /// GetExperience retrieves a specific experience by ID.
  ///
  /// [communityId] is optional - when provided, returns community-specific conversation_id.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetExperienceResponse> getExperience(
    String id, {
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetExperienceRequest(
          id: id,
          communityId: communityId ?? '',
        );
        final response = await _client.getExperience(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetExperience',
    );
  }

  /// GetExperienceDayWeather retrieves the hour-by-hour forecast at the
  /// experience's location for one local day (the "When" screen weather strip).
  ///
  /// [dateUnixSec] is local midnight of the day to forecast. Returns an empty
  /// list when the day is beyond the forecast horizon or weather is unavailable.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetExperienceDayWeatherResponse> getExperienceDayWeather({
    required String experienceId,
    required Int64 dateUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetExperienceDayWeatherRequest(
          experienceId: experienceId,
          dateUnixSec: dateUnixSec,
        );
        return _client.getExperienceDayWeather(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetExperienceDayWeather',
    );
  }

  /// GetExperienceCalendarWeather retrieves the daily forecast across the "When"
  /// calendar window — all at the experience's own location, so the month grid
  /// paints weather for the event's place rather than the viewer's home.
  ///
  /// Returns an empty list when the event has no coordinates or weather is
  /// unavailable. Throws [ServiceException] on failure.
  Future<List<DayForecast>> getExperienceCalendarWeather(
    String experienceId,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.getExperienceCalendarWeather(
          GetExperienceCalendarWeatherRequest(experienceId: experienceId),
          headers: _buildHeaders(),
        );
        return response.days;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetExperienceCalendarWeather',
    );
  }

  /// ListExperiences retrieves experiences for a specific community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Experience>> listExperiences(String communityId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListExperiencesRequest(communityId: communityId);
        final response = await _client.listExperiences(
          request,
          headers: _buildHeaders(),
        );

        return response.experiences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListExperiences',
    );
  }

  /// ListMyExperiences retrieves experiences created by the authenticated user.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Experience>> listMyExperiences() async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListMyExperiencesRequest();
        final response = await _client.listMyExperiences(
          request,
          headers: _buildHeaders(),
        );

        return response.experiences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListMyExperiences',
    );
  }

  /// GetExperiencePeople retrieves people associated with an experience.
  ///
  /// Returns host, upcoming RSVPs, and past attendees.
  /// If [communityId] is provided, filters results for that specific community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetExperiencePeopleResponse> getExperiencePeople({
    required String experienceId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetExperiencePeopleRequest(
          experienceId: experienceId,
          communityId: communityId ?? '',
        );
        final response = await _client.getExperiencePeople(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetExperiencePeople',
    );
  }

  /// GetExperienceStats retrieves statistics for an experience.
  ///
  /// Returns sessions held, total attendees, value created, and upcoming RSVPs.
  /// If [communityId] is provided, returns stats specific to that community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetExperienceStatsResponse> getExperienceStats({
    required String experienceId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetExperienceStatsRequest(
          experienceId: experienceId,
          communityId: communityId ?? '',
        );
        final response = await _client.getExperienceStats(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetExperienceStats',
    );
  }
}
