# services

Remote API clients: one file per server-side RPC service.

## What belongs here

Each file wraps a generated Connect/gRPC client (from `data/gen/`) and exposes typed Dart methods. Services:

- Build request protos and call the generated client.
- Add authentication headers via `RpcUtils.buildHeaders()`.
- Translate `ConnectException` into `ServiceException` with user-friendly messages via `RpcErrorHandler`.
- Export the proto types that callers need, so importers only depend on the service file.

Services do **not** cache, aggregate, or apply business logic. That belongs in repositories (for caching) or viewmodels (for UI logic).

## Key files

| File | RPC service |
|---|---|
| `auth_service.dart` | Registration and authentication |
| `gear_service.dart` | Gear item CRUD and AI generation |
| `community_service.dart` | Community management and membership |
| `transfer_service.dart` | Loan and giveaway state machine |
| `request_service.dart` | Help requests |
| `experience_service.dart` | Community experiences/events |
| `media_service.dart` | Media upload and retrieval |
| `search_service.dart` | Cross-entity search |
| `user_service.dart` | User profile management |
| `location_service.dart`, `mapbox_location_service.dart` | Geocoding and place search |
| `chat_service.dart` | Messaging |
| `feed_service.dart` | Community feed |
| `impact_service.dart` | Impact metrics |
| `fcm_service.dart` | Push notification token registration |
| `notification_route_replayer.dart` | Captures + replays notification deep-link routes so a tap that can't navigate immediately is recovered (#2636) |
| `oidc_service.dart` | Identity token exchange |
| `community_event_stream.dart`, `community_event_poller.dart` | Real-time community event delivery |
| `event_router.dart` | Routes incoming events to the correct handler |
| `providers.dart` | Riverpod providers for all services |

## When to add code here vs. elsewhere

- **New RPC method on an existing service** → add a method to the existing service file.
- **New server-side service** → new file following the same pattern; register a provider in `providers.dart`.
- **Caching or data combination** → `data/repositories/`, not here.
- **UI state or derived display data** → viewmodel, not here.
