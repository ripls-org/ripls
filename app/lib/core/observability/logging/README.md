# core/observability/logging

Structured logging utilities: context propagation, PII redaction, and an `ObservableLogger` wrapper.

## Key types

- **`ObservableLogger`** (`logger.dart`) — wraps the standard `Logger` with automatic request-context injection, PII redaction, and breadcrumb recording. Use `ObservableLogger.named('SomeClass')` in place of `Logger('SomeClass')` for any code that touches user data or makes network calls.
- **`LogContext`** / **`LogContextHolder`** (`logger.dart`) — lightweight container for the current `request_id` and `user_id`. Set `LogContextHolder.setUserId()` after login and `LogContextHolder.setRequestId()` per RPC call; both are picked up automatically by all `ObservableLogger` instances.
- **`LogRedactor`** (`redactor.dart`) — static helpers for masking PII: `maskEmail()`, `maskToken()`, `maskUserId()`, `maskPhoneNumber()`, `redactMap()`. Use these whenever logging values that may contain user-identifiable data.

## When to add code here vs. elsewhere

- New **redaction pattern** (e.g., mask a new kind of identifier) → `redactor.dart`.
- New **context field** to propagate through requests → `LogContext` / `LogContextHolder`.
- Logging a specific business event → call `ObservableLogger` methods at the call site; no new file needed here.
- Analytics or crash-reporting logic → `core/observability/` (parent directory).
