import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:ripls/services/device_location_service.dart' show LocationIntent;
import 'package:ripls/services/user_position_resolver.dart';

/// Cached user position for proximity bias and precise-pin display across
/// the app.
///
/// **This is the single entry point for the user's device position.** All
/// callers — view models, screens, map helpers — must read this provider
/// (or pass its value through), never call a geolocator API directly. The
/// provider owns session-level caching and invalidation; duplicate GPS
/// fetches are expensive (battery, latency, permission popups) and break
/// the invalidation contract.
///
/// Keyed on [LocationIntent]. Each intent has its own cache slot, so
/// proximity and precise reads don't share state:
///   - [LocationIntent.proximityBias]: coarse, OS-cached last-known fix
///     preferred (≤ 5 min stale). Medium accuracy when fresh.
///   - [LocationIntent.precisePin]: always fresh, high accuracy. Never
///     served from a stale cache or an OS last-known shortcut.
///
/// Example:
/// ```dart
/// final pos = await ref.read(
///   userLocationProvider(LocationIntent.proximityBias).future,
/// );
/// ```
///
/// Resolution ladder (via UserPositionResolver, see that file for source
/// enum + logging):
///   1. Live device GPS (accuracy + staleness depend on intent).
///   2. User's primary residence location (coarse fallback).
///   3. null — unbiased geocoding.
///
/// One-shot fetch per (session, intent). Use
/// `ref.invalidate(userLocationProvider(intent))` to force a re-fetch of a
/// specific intent, or invalidate both to re-fetch everything (e.g. after
/// pull-to-refresh or primary residence change).
///
/// **Architectural note.** The "repositories are the only layer that calls
/// services" rule in docs/client/architecture.md does not apply here. GPS
/// state is transient device state, not a cacheable server resource, so
/// wrapping DeviceLocationService in a Stash-backed repository would be a
/// category error. This provider (plus UserPositionResolver) plays the
/// "cache + invalidation owner" role a repository would otherwise play.
/// Do not copy this pattern for non-GPS concerns.
final userLocationProvider =
    FutureProvider.family<Position?, LocationIntent>((ref, intent) {
  return UserPositionResolver.resolve(ref, intent);
});
