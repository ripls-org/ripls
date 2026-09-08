# RPC Error Handling

This directory contains centralized error handling for ConnectRPC errors.

## Key types

- `ServiceException` (`rpc_error_handler.dart`) — thrown by service-layer
  code with a user-friendly `message` and an optional structured `Code`.
- `RpcErrorHandler` (`rpc_error_handler.dart`) — instance API for
  service callers (`handleError`) plus static helpers `classify`,
  `localize`, `getTransportErrorMessage`, `getUserMessageForCode`.
- `UserError` / `TransportKind` (`user_error.dart`) — the typed,
  freezed sealed error representation that viewmodels store on state.

## Overview

The `RpcErrorHandler` provides a consistent way to:
- Log technical error details for debugging
- Map ConnectRPC error codes to user-friendly messages
- Support future integration with crash reporting services
- Extract server-provided user messages when available
- Convert any caught exception into a typed `UserError` for
  viewmodel state, with localization deferred to the widget layer

## Viewmodel error state: `UserError`

Viewmodels must **not** store resolved English strings on state. The
typed `UserError` type covers every category the handler expresses:

| Variant                | Source                                  | How widgets render       |
|------------------------|-----------------------------------------|--------------------------|
| `serverMessage`        | `ServiceException` (server-supplied)    | server's exact message   |
| `rpcCode`              | raw `ConnectException` (no wrap)        | `_localizeCode(code, l10n)` |
| `transport(socket/timeout/http)` | `SocketException` / `TimeoutException` / `HttpException` | `l10n.rpcErrorTransport*` |
| `generic({fallback})`  | catch-all                                | `fallback` or `l10n.rpcErrorGeneric` |

Two static methods are the surface:

- `RpcErrorHandler.classify(Object e, {String? fallback}) → UserError` —
  the sole conversion path. Reads `e.message` and `e.code` directly,
  never `e.toString()`, so a future override on `ServiceException`
  cannot leak.
- `RpcErrorHandler.localize(UserError, AppLocalizations) → String` —
  resolves to localized text at render time.

```dart
// In the viewmodel:
} catch (e, stackTrace) {
  _log.severe('Failed to load gear', e, stackTrace);
  state = state.copyWith(error: RpcErrorHandler.classify(e));
}

// In the widget:
if (state.error != null) {
  return Text(RpcErrorHandler.localize(state.error!, context.l10n));
}
```

This pattern prevents two classes of bug:

1. **Sanitization leak** — `e.toString()` on a non-`ServiceException`
   leaks `ConnectException(code: …, message: …)`, `FormatException`
   text exposing SQL hints, or `StateError`/`TypeError` strings from
   nullability bugs.
2. **Localization gap** — resolved English strings in viewmodel
   state never reach the Spanish bundle. With `UserError`, the
   `AppLocalizations` lookup happens in `build()` on the right
   `BuildContext`.

A CI grep gate (`npm run lint:dart:errortostring`) blocks both
regressions: the original `e.toString()` pattern and re-introduction
of `String? errorMessage` on viewmodel state classes.

## Usage

### Basic Service Integration

```dart
import 'package:ripls/core/errors/rpc_error_handler.dart';

class MyService {
  final MyServiceClient _client;
  final RpcErrorHandler _errorHandler;

  MyService({
    required Transport transport,
    RpcErrorHandler? errorHandler,
  })  : _client = MyServiceClient(transport),
        _errorHandler = errorHandler ?? RpcErrorHandler();

  Future<MyData> doSomething(String param) async {
    try {
      final response = await _client.doSomething(Request(param: param));
      return response.data;
    } on ConnectException catch (e) {
      throw ServiceException(_errorHandler.getUserMessageForCode(e.code));
    } catch (e) {
      throw ServiceException('Something went wrong. Please try again.');
    }
  }
}
```

**Note**: `ServiceException` is a general-purpose exception class defined in `rpc_error_handler.dart` that can be used by any service in the application.

### UI Layer Usage

The service methods throw exceptions with user-friendly messages on errors:

```dart
try {
  final data = await myService.doSomething('value');
  // Handle success
} catch (e) {
  if (!mounted) return;

  // Display user-friendly error message
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(content: Text(e.toString())),
  );
}
```

**Note**: No BuildContext is needed in service method calls, avoiding async gap lint warnings.

## Error Code Mapping

The handler maps ConnectRPC error codes to user-friendly messages:

| Error Code | User Message |
|-----------|--------------|
| `invalidArgument` | "Invalid input. Please check your information and try again." |
| `unauthenticated` | "You need to sign in to continue." |
| `permissionDenied` | "You don't have permission to perform this action." |
| `notFound` | "The requested item could not be found." |
| `alreadyExists` | "This item already exists." |
| `resourceExhausted` | "Too many requests. Please try again later." |
| `unavailable` | "Service is temporarily unavailable. Please try again." |
| `deadlineExceeded` | "Request timed out. Please check your connection and try again." |
| `canceled` | "Request was canceled." |
| `unimplemented` | "This feature is not yet available." |
| `internal` | "An internal error occurred. Please try again." |
| Other | "Something went wrong. Please try again." |

## Logging

The handler uses the `logging` package to log all errors with full technical details:

```
[SEVERE] RPC Error occurred: <full exception>
[SEVERE] ConnectException Details: code=unavailable, message=..., details=..., metadata=...
[INFO] Additional context: {operation: listGear}
```

## Future Enhancements

### Internationalization (i18n)

To add multi-language support, replace the hardcoded messages with a localization solution:

```dart
// Replace in _getUserMessage
return AppLocalizations.of(context)!.errorPermissionDenied;
```

### Crash Reporting

To integrate with a Firebase Crashlytics equivalent:

```dart
import 'package:firebase_crashlytics/firebase_crashlytics.dart';

class RpcErrorHandler {
  final FirebaseCrashlytics? _crashlytics;

  RpcErrorHandler({FirebaseCrashlytics? crashlytics})
    : _crashlytics = crashlytics;

  String handleError(...) {
    // ... existing code ...

    _crashlytics?.recordError(
      error,
      StackTrace.current,
      reason: 'ConnectRPC Error: ${error.code}',
      information: [...],
    );
  }
}
```

### Server-Provided Messages

To extract localized messages from server error details, implement the `_extractServerUserMessage` method based on your server's error detail structure.
