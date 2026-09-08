import 'dart:async';

import 'package:connectrpc/connect.dart' as connect;
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/analytics_route_observer.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/utils/request_id.dart';

final _log = Logger('RpcUtils');
final _audit = Logger('RpcAudit');

// Zone key used by buildHeaders() to publish the X-Request-ID it generated
// so executeRpc() can stamp it on the audit log line. The Zone scope is
// per-call, so concurrent RPCs do not race.
const Symbol _requestIdSinkKey = #ripls.rpc.requestIdSink;

/// RpcUtils provides utility functions for RPC operations.
class RpcUtils {
  // Global token refresh handler — set once during app initialization.
  // Authentication is a cross-cutting concern; this avoids threading a
  // refresh callback through every service and every RPC call site.
  static Future<bool> Function()? _tokenRefreshHandler;

  // Global locale getter — returns a BCP-47 language tag (e.g. "en",
  // "es") that the client wants the server to render in. buildHeaders
  // attaches the value as the Accept-Language header on every RPC so
  // the server can resolve the recipient locale immediately, before
  // the persisted preferred_language has a chance to round-trip.
  // Returning null skips the header — the server falls back to its
  // own resolution order (stored preference → en).
  static String? Function()? _localeGetter;

  /// When true, every executeRpc() call emits an `RPC_AUDIT` log line and
  /// every cache lookup emits a `CACHE_AUDIT` line, both grep-friendly.
  /// Defaults to false. Flip on at app start (e.g. from main.dart) when
  /// running an RPC traffic audit (issue #1581).
  static bool auditLogging = false;

  /// Optional sink for emitting `network` breadcrumbs. Set from main.dart
  /// once ObservabilityService is constructed. When null, breadcrumbs are
  /// skipped — the audit log line is still emitted.
  static void Function(String message, Map<String, dynamic> data)?
      networkBreadcrumbSink;

  /// Configures the global token refresh handler used by [executeRpc].
  ///
  /// Must be called during app initialization before any authenticated RPCs.
  /// The handler should attempt a token refresh and return true on success.
  static void configureTokenRefresh(Future<bool> Function() handler) {
    _tokenRefreshHandler = handler;
  }

  /// Configures the global locale getter used by [buildHeaders] to set
  /// the `Accept-Language` request header.
  ///
  /// The getter is invoked on every RPC; it should be cheap and return
  /// the currently-active BCP-47 language tag (e.g. `"en"`, `"es"`).
  /// Returning `null` or empty omits the header — the server falls
  /// back to the stored `preferred_language`.
  ///
  /// Call once at app initialization with a closure that reads
  /// `localePreferenceProvider` and falls back to the device locale.
  static void configureLocaleGetter(String? Function() getter) {
    _localeGetter = getter;
  }

  /// Builds headers with optional authentication token and request ID.
  ///
  /// Always includes an X-Request-ID header for request correlation.
  /// If [getAccessToken] returns a non-null, non-empty token, adds an
  /// Authorization header with Bearer scheme. Otherwise returns headers
  /// with only the request ID.
  ///
  /// If the token is null or empty and [onUnauthenticated] is provided,
  /// triggers the callback to handle the missing token scenario.
  static connect.Headers buildHeaders(
    String? Function() getAccessToken, {
    Future<void> Function()? onUnauthenticated,
  }) {
    // Reuse the zone-active request ID if executeRpc is wrapping this call,
    // so the outbound X-Request-ID matches the one tagging client logs.
    // Otherwise (e.g. streaming RPCs that bypass executeRpc) generate a fresh
    // ID for this call.
    final requestId = RequestIdGenerator.current ?? RequestIdGenerator.generate();
    final headers = connect.Headers()
      ..set(RequestIdGenerator.headerName, [requestId]);

    // Attach the user's currently-active locale so the server can
    // render push notifications, emails, and any other terminal copy
    // in the right language for this request. The l10n middleware on
    // the server captures the header into context for downstream
    // handlers (see server/middleware/accept_language.go).
    final localeTag = _localeGetter?.call();
    if (localeTag != null && localeTag.isNotEmpty) {
      headers.set('Accept-Language', [localeTag]);
    }

    // Publish the generated request ID to the surrounding executeRpc()
    // (if any) via a Zone-local sink so it can be logged for client/server
    // correlation. Outside an executeRpc the sink is absent and this is a
    // no-op.
    final sink = Zone.current[_requestIdSinkKey];
    if (sink is void Function(String)) {
      sink(requestId);
    }

    final token = getAccessToken();
    if (token != null && token.isNotEmpty) {
      headers.set('Authorization', ['Bearer $token']);
      _log.fine('Request $requestId: authenticated');
      return headers;
    }

    // If no token and we have a callback, trigger it
    // This handles the case where the token is empty/expired before making the call
    if (onUnauthenticated != null) {
      _log.warning(
        'Request $requestId: no access token available, triggering authentication handler',
      );
      // Schedule the callback to run after this method returns
      Future.microtask(onUnauthenticated);
    }

    return headers;
  }

  /// Executes an RPC call with standardized error handling and optional performance tracking.
  ///
  /// Wraps the RPC call in try-catch blocks that convert ConnectException
  /// and other errors into user-friendly ServiceException messages.
  ///
  /// If the error is [connect.Code.unauthenticated], calls the optional
  /// [onUnauthenticated] callback to allow the app to handle logout/redirect.
  ///
  /// If [performanceMonitor] is provided, records the RPC latency as a
  /// performance trace keyed by [operationName].
  static Future<T> executeRpc<T>(
    Future<T> Function() rpcCall,
    RpcErrorHandler errorHandler, {
    Future<void> Function()? onUnauthenticated,
    required String operationName,
    PerformanceMonitor? performanceMonitor,
  }) {
    // Scope a single request ID to the entire RPC attempt (including the
    // post-token-refresh retry). buildHeaders inside rpcCall picks it up,
    // and ObservableLogger tags any log entries emitted during the call
    // with the same request_id, giving end-to-end client/server correlation.
    final requestId = RequestIdGenerator.generate();
    return RequestIdGenerator.runWithRequestId(
      requestId,
      () => _executeRpcBody(
        rpcCall,
        errorHandler,
        onUnauthenticated: onUnauthenticated,
        operationName: operationName,
        performanceMonitor: performanceMonitor,
      ),
    );
  }

  static Future<T> _executeRpcBody<T>(
    Future<T> Function() rpcCall,
    RpcErrorHandler errorHandler, {
    Future<void> Function()? onUnauthenticated,
    required String operationName,
    PerformanceMonitor? performanceMonitor,
  }) async {
    final stopwatch = Stopwatch()..start();
    PerformanceTrace? trace;
    String? capturedRequestId;

    // Start performance trace if monitoring is enabled
    if (performanceMonitor != null && performanceMonitor.isAvailable) {
      trace = performanceMonitor.startTrace('rpc_$operationName');
      trace.start();
      trace.putAttribute('operation', operationName);
    }

    Future<T> runWithCapture() {
      if (!auditLogging) {
        return rpcCall();
      }
      return runZoned(
        rpcCall,
        zoneValues: {
          _requestIdSinkKey: (String id) {
            capturedRequestId = id;
          },
        },
      );
    }

    try {
      final result = await runWithCapture();

      // Record success
      stopwatch.stop();
      if (trace != null) {
        trace.putMetric('duration_ms', stopwatch.elapsedMilliseconds);
        trace.putAttribute('status', 'success');
        trace.stop();
      }
      _log.fine('RPC $operationName completed in ${stopwatch.elapsedMilliseconds}ms');
      _emitAudit(
        operationName: operationName,
        requestId: capturedRequestId,
        durationMs: stopwatch.elapsedMilliseconds,
        status: 'ok',
      );

      return result;
    } on connect.ConnectException catch (e) {
      // Record failure
      stopwatch.stop();
      if (trace != null) {
        trace.putMetric('duration_ms', stopwatch.elapsedMilliseconds);
        trace.putAttribute('status', 'error');
        trace.putAttribute('error_code', e.code.name);
        trace.stop();
      }
      _log.warning('RPC $operationName failed after ${stopwatch.elapsedMilliseconds}ms: ${e.code.name}');
      _emitAudit(
        operationName: operationName,
        requestId: capturedRequestId,
        durationMs: stopwatch.elapsedMilliseconds,
        status: 'err',
        errorCode: e.code.name,
      );

      // On unauthenticated, attempt token refresh then retry before giving up.
      if (e.code == connect.Code.unauthenticated && _tokenRefreshHandler != null) {
        _log.info('Unauthenticated error, attempting token refresh');
        final refreshed = await _tokenRefreshHandler!();
        if (refreshed) {
          _log.info('Token refreshed, retrying RPC $operationName');
          try {
            // Single retry. The refresh handler is rate-limited so a second 401 is fatal.
            return await rpcCall();
          } on connect.ConnectException catch (retryError) {
            _log.warning('RPC $operationName failed after token refresh: ${retryError.code.name}');
            if (retryError.code == connect.Code.unauthenticated && onUnauthenticated != null) {
              await onUnauthenticated();
            }
            throw ServiceException(errorHandler.getUserMessageForCode(retryError.code), code: retryError.code);
          }
        }
        // Refresh failed — fall through to logout.
        _log.warning('Token refresh failed, triggering logout');
      }

      if (e.code == connect.Code.unauthenticated && onUnauthenticated != null) {
        await onUnauthenticated();
      }
      throw ServiceException(errorHandler.getUserMessageForCode(e.code), code: e.code);
    } catch (e) {
      // Record unexpected failure
      stopwatch.stop();
      if (trace != null) {
        trace.putMetric('duration_ms', stopwatch.elapsedMilliseconds);
        trace.putAttribute('status', 'error');
        trace.putAttribute('error_code', 'unexpected');
        trace.stop();
      }
      _log.severe('RPC $operationName failed unexpectedly after ${stopwatch.elapsedMilliseconds}ms', e);
      _emitAudit(
        operationName: operationName,
        requestId: capturedRequestId,
        durationMs: stopwatch.elapsedMilliseconds,
        status: 'err',
        errorCode: 'unexpected',
      );

      throw ServiceException('Something went wrong. Please try again.');
    }
  }

  static void _emitAudit({
    required String? operationName,
    required String? requestId,
    required int durationMs,
    required String status,
    String? errorCode,
  }) {
    if (!auditLogging) {
      return;
    }
    final rpc = operationName ?? 'unknown';
    final screen = AnalyticsRouteObserver.currentScreen ?? 'unknown';
    final reqId = requestId ?? 'unknown';
    final code = errorCode != null ? ' code=$errorCode' : '';
    _audit.info(
      'RPC_AUDIT rpc=$rpc screen=$screen request_id=$reqId duration_ms=$durationMs status=$status$code',
    );
    final sink = networkBreadcrumbSink;
    if (sink != null) {
      sink('rpc $rpc', {
        'rpc': rpc,
        'screen': screen,
        'request_id': reqId,
        'duration_ms': durationMs,
        'status': status,
        'error_code': ?errorCode,
      });
    }
  }
}
