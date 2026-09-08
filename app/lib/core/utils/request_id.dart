import 'dart:async';

import 'package:uuid/uuid.dart';

/// RequestIdGenerator generates and propagates HTTP-correlation request IDs.
///
/// Each outbound API request carries an `X-Request-ID` header so client logs
/// and server logs can be joined by the same UUID. The same ID is also made
/// visible to client-side log entries that run inside the RPC scope, via a
/// Dart [Zone] value, so `ObservableLogger` automatically tags each entry
/// with the request_id of the active call.
///
/// **Not to be confused with `models.Request` entity IDs**, which identify
/// the database object representing a community member asking for help.
/// Those are logged as `target_request_id` to avoid colliding with this
/// HTTP-correlation field. See `docs/server/observability.md` for the
/// glossary.
class RequestIdGenerator {
  static const _uuid = Uuid();

  /// The HTTP header name for request ID propagation.
  static const headerName = 'X-Request-ID';

  /// Zone key used to scope a request ID to a single async call tree.
  ///
  /// Code running inside [runWithRequestId] (and any awaited continuations)
  /// can read the active ID via [current]. Logging infrastructure uses this
  /// to attach `request_id` automatically without threading it through every
  /// call site.
  static const Symbol zoneKey = #riplsRequestId;

  /// Generates a new UUID v4 request ID.
  ///
  /// Returns a unique identifier like "550e8400-e29b-41d4-a716-446655440000".
  static String generate() {
    return _uuid.v4();
  }

  /// Returns the request ID active in the current [Zone], or null if none
  /// is set. Reads `Zone.current[zoneKey]`.
  static String? get current {
    final value = Zone.current[zoneKey];
    return value is String ? value : null;
  }

  /// Runs [body] in a child zone whose [zoneKey] resolves to [requestId].
  ///
  /// Inside [body] (and across awaits within it), [current] returns
  /// [requestId]. Use this to stamp a single request ID across both an
  /// outbound RPC's headers and any client-side logs emitted while the RPC
  /// is in flight.
  static T runWithRequestId<T>(String requestId, T Function() body) {
    return runZoned<T>(body, zoneValues: {zoneKey: requestId});
  }
}
