// Part of ExperienceService. Contains AI generation RPCs via mixin:
// streamGenExperience, convertInformalTime, generateCompletionSummary.
part of '../experience_service.dart';

mixin ExperienceGenMethods on ExperienceServiceBase {
  /// StreamGenExperience generates experience content using AI, emitting
  /// title / geocoded / media_ready events as soon as each resolves, then
  /// a terminal `final` (full response) or `error` event. Errors from the
  /// transport surface through the returned stream; callers must handle
  /// `onError` on their subscription.
  Stream<StreamGenExperienceResponse> streamGenExperience({
    String? text,
    String? mediaId,
    String? websiteUrl,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) async* {
    final request = StreamGenExperienceRequest();

    if (text != null && text.isNotEmpty) {
      request.text = text;
    } else if (mediaId != null && mediaId.isNotEmpty) {
      request.mediaId = mediaId;
    } else if (websiteUrl != null && websiteUrl.isNotEmpty) {
      request.websiteUrl = websiteUrl;
    }

    if (locationId != null && locationId.isNotEmpty) {
      request.locationId = locationId;
    }
    if (latitudeDeg != null && longitudeDeg != null) {
      request.latitudeDeg = latitudeDeg;
      request.longitudeDeg = longitudeDeg;
    }

    request.currentTimeUnixSec =
        Int64(DateTime.now().millisecondsSinceEpoch ~/ 1000);
    request.timezone = await _getIANATimezone();

    yield* _client.streamGenExperience(request, headers: _buildHeaders());
  }

  /// ConvertInformalTime converts an informal time description to structured ExperienceTime.
  ///
  /// Uses server-side LLM to parse natural language time expressions like:
  /// - "tomorrow afternoon"
  /// - "next Friday at 6pm"
  /// - "this weekend"
  ///
  /// Returns a ConvertInformalTimeResponse containing the parsed time and confidence level.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ConvertInformalTimeResponse> convertInformalTime(
    String informalDescription,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ConvertInformalTimeRequest()
          ..informalDescription = informalDescription
          ..currentTimeUnixSec = Int64(DateTime.now().millisecondsSinceEpoch ~/ 1000)
          ..timezone = await _getIANATimezone();

        final response = await _client.convertInformalTime(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ConvertInformalTime',
    );
  }

}
