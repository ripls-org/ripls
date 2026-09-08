# Structured Logging

This package provides structured logging for the server using Go's standard `log/slog` package.

## Features

- JSON output for production (auto-indexed by Cloud Logging)
- Human-readable text output for development
- Request ID correlation via middleware
- Automatic context enrichment (request_id, user_id)
- Sensitive data masking (emails, tokens)
- Configurable log levels

## Usage in RPC Handlers

Use `LoggerWithContext(ctx)` to get a logger enriched with request context:

```go
func (s *Service) SaveGear(
    ctx context.Context,
    req *connect.Request[api.SaveGearRequest],
) (*connect.Response[api.SaveGearResponse], error) {
    authInfo, err := auth.RequireAuth(ctx)
    if err != nil {
        return nil, err
    }

    logger := logging.LoggerWithContext(ctx).With(
        "user_id", authInfo.UserID,
        "user_email", logging.MaskEmail(authInfo.Email),
    )

    logger.Info("creating gear", "gear_name", req.Msg.Name)
    // ...
}
```

The logger automatically includes `request_id` from the middleware.

## Log Levels

| Level | Use For | Example |
|-------|---------|---------|
| `Debug` | Verbose info, read operations | "requesting gear", "cache hit" |
| `Info` | Normal operations worth recording | "gear created", "user logged in" |
| `Warn` | Unexpected but handled situations | "slow query detected", "retry succeeded" |
| `Error` | Failures requiring attention | "database error", "external API failed" |

**Rules:**
- Default production level: `info`
- Default development level: `debug`
- Never log at Error for user errors (invalid input, not found) - use Info or Warn
- Always log at Error for system errors (database down, external service failed)

## Masking Sensitive Data

Always mask sensitive data before logging:

```go
// Mask emails: "alice@example.com" → "al***@example.com"
logger.Info("user action", "user_email", logging.MaskEmail(email))

// Mask tokens: "eyJhbGciOiJIUzI1NiIs..." → "eyJhbGci..."
logger.Debug("token validated", "token", logging.MaskToken(token))
```

For automatic masking, use the `RedactedEmail` and `RedactedToken` types:

```go
logger.Info("user action", "email", logging.RedactedEmail(email))
```

## Standard Field Names

Use these field names consistently:

| Field | Type | Description |
|-------|------|-------------|
| `request_id` | string | Unique request identifier (auto-added) |
| `user_id` | string | Authenticated user ID |
| `user_email` | string | Masked email |
| `operation` | string | RPC/function name |
| `duration_ms` | int64 | Operation duration |
| `error` | error | Error value |

### Entity-Specific Fields

| Field | Used By |
|-------|---------|
| `gear_id` | gear, loan, media |
| `loan_id` | loan |
| `community_id` | community |
| `location_id` | location, gear |
| `media_id` | media, gear |

## Migration Checklist

When migrating a file from `log.Printf` to structured logging:

1. Replace `import "log"` with `"go.ripls.org/ripls/server/logging"`

2. At the start of each RPC method, create a logger with context:
   ```go
   logger := logging.LoggerWithContext(ctx).With(
       "user_id", authInfo.UserID,
       // add other relevant context fields
   )
   ```

3. Convert each `log.Printf`:
   - Success operations → `logger.Info()`
   - Failures → `logger.Error()`
   - Verbose/debug info → `logger.Debug()`
   - Unexpected but handled → `logger.Warn()`

4. Add context fields instead of interpolating into message:
   ```go
   // Before
   log.Printf("User %s creating gear: %s", userID, gearName)

   // After
   logger.Info("creating gear", "gear_name", gearName)
   // user_id is already in logger context
   ```

5. Mask sensitive data (emails, tokens)

6. Remove `log` import if no longer used

7. Run tests to verify
