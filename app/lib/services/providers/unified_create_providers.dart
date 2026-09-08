import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_timezone/flutter_timezone.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/user_location_initializer.dart';
import 'package:ripls/presentation/viewmodels/unified_create_streaming_actions.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/user_location_provider.dart';
import 'package:ripls/services/unified_create_service.dart';

final _log = Logger('UnifiedCreateProviders');

/// Provider for UnifiedCreateService — thin connect-go client wrapper.
final unifiedCreateServiceProvider = Provider<UnifiedCreateService>((ref) {
  return UnifiedCreateService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for the production [UnifiedCreateStreamingActions]. Resolves
/// the per-request context (user's primary location, GPS proximity,
/// IANA timezone, current Unix time) before opening the streaming RPC,
/// so the server's resolveRegion / resolveTimezone / proximity helpers
/// have everything they need to produce time-aware AI output and
/// geocoding suggestions tuned to the user's region.
final unifiedCreateStreamingActionsProvider =
    Provider<UnifiedCreateStreamingActions>((ref) {
  final service = ref.watch(unifiedCreateServiceProvider);
  return LiveUnifiedCreateStreamingActions(({
    text,
    mediaId,
    websiteUrl,
    forceType,
  }) async* {
    String? locationId;
    try {
      locationId = await UserLocationInitializer.getPrimaryLocationIfSet(ref);
    } catch (e) {
      _log.fine('failed to read primary residence', e);
    }

    String? timezone;
    try {
      timezone = (await FlutterTimezone.getLocalTimezone()).identifier;
    } catch (e) {
      _log.warning('failed to get local timezone, falling back to UTC', e);
      timezone = 'UTC';
    }

    double? lat;
    double? lng;
    try {
      final pos =
          await ref.read(userLocationProvider(LocationIntent.proximityBias).future);
      if (pos != null) {
        lat = pos.latitude;
        lng = pos.longitude;
      }
    } catch (e) {
      _log.fine('GPS unavailable for proximity bias', e);
    }

    yield* service.streamGenUnifiedCreate(
      text: text,
      mediaId: mediaId,
      websiteUrl: websiteUrl,
      forceType: forceType,
      locationId: locationId,
      latitudeDeg: lat,
      longitudeDeg: lng,
      timezone: timezone,
    );
  });
});
