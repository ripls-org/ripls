// Part of CommunityService. Contains the community-event RPCs: the per-user
// realtime stream and its poll backstop (#2867), plus the per-community
// listing they replaced as a backstop.
//
// Split out because community_service.dart crossed the 1,000-line gate, and
// these three are the one group in it with a shared subject.
part of '../community_service.dart';

mixin CommunityEventMethods on CommunityServiceBase {
  /// ListCommunityEvents retrieves events in a community.
  ///
  /// If [sinceUnixSec] is provided, only events newer than that timestamp
  /// are returned. The server caps this at 7 days ago.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityEventItem>> listCommunityEvents(
    String communityId, {
    int? sinceUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListCommunityEventsRequest(communityId: communityId);
        if (sinceUnixSec != null) {
          request.sinceUnixSec = Int64(sinceUnixSec);
        }
        final response = await _client.listCommunityEvents(
          request,
          headers: _buildHeaders(),
        );

        return response.events;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListCommunityEvents',
    );
  }

  /// ListUserEvents retrieves recent events across every community the caller
  /// belongs to — the poll backstop, in one request rather than one per
  /// community (#2867).
  ///
  /// If [sinceUnixSec] is provided, only newer events are returned. The server
  /// caps this at 7 days ago.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<CommunityEventItem>> listUserEvents({int? sinceUnixSec}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListUserEventsRequest();
        if (sinceUnixSec != null) {
          request.sinceUnixSec = Int64(sinceUnixSec);
        }
        final response = await _client.listUserEvents(
          request,
          headers: _buildHeaders(),
        );

        return response.events;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListUserEvents',
    );
  }

  /// StreamUserEvents opens the caller's single real-time event stream,
  /// covering every community they belong to (#2867).
  ///
  /// If [sinceUnixSec] is provided, the stream replays events since that time
  /// (capped at 7 days) before streaming new ones. If omitted, only new events
  /// are streamed.
  ///
  /// Errors are logged and close the stream rather than propagating: the
  /// caller's reconnect loop treats a completed stream as the signal to
  /// reopen, and the poll backstop covers the gap.
  Stream<StreamUserEventsResponse> streamUserEvents({int? sinceUnixSec}) async* {
    final request = StreamUserEventsRequest();
    if (sinceUnixSec != null) {
      request.sinceUnixSec = Int64(sinceUnixSec);
    }

    try {
      final stream = _client.streamUserEvents(
        request,
        headers: _buildHeaders(),
      );

      await for (final event in stream) {
        yield event;
      }
    } on connect.ConnectException catch (e) {
      if (CommunityService.isBenignStreamClose(e)) {
        // The server caps stream lifetime and returns nil (a clean EOF). Some
        // transports surface that as an HTTP/2 INTERNAL_ERROR frame. The
        // caller reconnects immediately, so this is not a real failure.
        _log.info('User event stream closed at server lifetime cap; reconnecting');
      } else {
        _log.warning(
          'User event stream error: ${e.code.name} - ${e.message}',
        );
      }
    } catch (e) {
      _log.warning('User event stream error: $e');
    }
  }
}
