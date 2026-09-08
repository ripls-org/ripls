# Notifications

The `notifications` package provides the push notification service abstraction: registering device tokens, routing notifications to the correct platform, and handling invalid-token cleanup. Provider implementations live in sub-packages.

## Key files

- `service.go` — `Service` interface (`NotifyUser`, `HasDevices`, `UnregisterDevice`) and `Provider` interface. The default implementation queries device tokens from storage and dispatches to the appropriate provider.
- `mock.go` — in-memory mock `Service` for use in tests.
- `fcm/` — push notification delivery via Firebase Cloud Messaging (FCM) for Android and iOS devices.
- `noop/` — no-op provider that logs notifications without sending them; used in local development and end-to-end tests.

## When to add code here vs. elsewhere

A new delivery platform belongs as a new sub-package implementing `notifications.Provider`. Notification content and routing logic (what to say, which users to notify) belongs in the service that triggers the notification (e.g. `server/services/transfer`, `server/services/community`). Device token registration belongs in `server/services/device`.
