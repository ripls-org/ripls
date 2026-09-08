// Coordination pattern: [RequestNotifier] is the single Riverpod provider.
// It composes behaviour from two mixins:
//   RequestMediaActionsMixin  — media loading, picking, and management
//   RequestEditActionsMixin   — content editing, fulfillment, offers, and cancellation
//
// All mixins share [state] (the coordinator's own [Notifier.state]) and
// communicate via non-private abstract method declarations. There is exactly
// one [requestProvider] family so call sites are unchanged.
import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/community_resolver.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/presentation/viewmodels/request_edit_actions.dart';
import 'package:ripls/presentation/viewmodels/request_media_actions.dart';
import 'package:ripls/presentation/viewmodels/request_state.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/services/providers.dart';

// Re-export resolveCommunityId so existing importers of this file work unchanged.
export 'package:ripls/core/utils/community_resolver.dart'
    show resolveCommunityId;
// Re-export RequestState so existing importers of this file work unchanged.
export 'package:ripls/presentation/viewmodels/request_state.dart'
    show RequestState;

final _log = Logger('RequestViewModel');

/// RequestNotifier manages request detail state.
///
/// Parameterized by requestId via the family modifier. Behaviour is composed
/// from [RequestMediaActionsMixin] and [RequestEditActionsMixin]; core loading
/// and refresh logic lives here.
class RequestNotifier extends Notifier<RequestState>
    with RequestMediaActionsMixin, RequestEditActionsMixin {
  /// RequestNotifier accepts the requestId parameter from the family modifier.
  RequestNotifier(this.requestId);

  /// The requestId for this specific request instance.
  final String requestId;

  @override
  final ImagePicker imagePicker = ImagePicker();

  @override
  RequestState build() {
    // Seed mute state from the session-wide `videoUnmutedProvider` so a
    // user who has already chosen "audio on" hears the next request
    // background when they navigate to it (issue #1250).
    final isUnmuted = ref.read(videoUnmutedProvider);
    return RequestState(requestId: requestId, isMuted: !isUnmuted);
  }

  // ---------------------------------------------------------------------------
  // Initialization
  // ---------------------------------------------------------------------------

  /// initialize sets up the notifier with the given parameters and loads
  /// request details.
  Future<void> initialize({
    required String requestId,
    required String? currentUserId,
    String? communityId,
    double? initialDistanceMeters,
  }) async {
    state = state.copyWith(
      requestId: requestId,
      currentUserId: currentUserId,
      communityId: communityId,
      locationDistanceMeters: initialDistanceMeters,
      isLoading: true,
      error: null,
    );

    await loadRequestDetails();
  }

  // ---------------------------------------------------------------------------
  // Core loading (implements abstract methods required by mixins)
  // ---------------------------------------------------------------------------

  /// refreshRequest silently reloads request details and media without showing
  /// a loading spinner. Keeps existing content visible while data updates in
  /// the background.
  @override
  Future<void> refreshRequest() async {
    final requestId = state.requestId;
    if (requestId == null) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final response = await requestRepository.getRequest(
        requestId: requestId,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;

      state = state.copyWith(
        requestDetails: response,
        // Keep the shared-community list fresh too — the access sheet / "Shared
        // with" surfaces read state.sharedCommunities, so a silent refresh that
        // only updated requestDetails left them stale after a community was
        // added (#2492).
        sharedCommunities: response.sharedCommunities,
        error: null,
      );

      await loadAllMediaFromServer(response.mediaIds);

      _log.fine('🔄 Request refreshed in background: $requestId');
    } catch (e) {
      _log.warning('⚠️ Failed to background-refresh request $requestId: $e');
      // Silently ignore — existing content stays visible.
    }
  }

  /// refresh is the public alias for [refreshRequest], kept for call-site
  /// compatibility with screens that call `notifier.refresh()`.
  Future<void> refresh() => refreshRequest();

  /// acceptGearOffer toggles the requester's acceptance of a gear-backed offer
  /// ("this one works", #2702). Requester only; accepting an already-accepted
  /// offer clears it. The server's updated request lands straight into state
  /// so the offer chips flip without a separate refresh.
  Future<void> acceptGearOffer(String contributionId) async {
    final requestId = state.requestId;
    if (requestId == null) return;

    final requestRepository = ref.read(requestRepositoryProvider);
    final response = await requestRepository.acceptRequestOffer(
      requestId: requestId,
      contributionId: contributionId,
    );
    if (!ref.mounted) return;
    state = state.copyWith(requestDetails: response, error: null);
  }

  /// refreshRequestDetails invalidates the details and stats caches, then
  /// reloads fresh data without a loading spinner. Call this after any
  /// mutation that should update both the content view and impact tiles.
  @override
  Future<void> refreshRequestDetails() async {
    final requestId = state.requestId;
    if (requestId == null) return;
    final requestRepository = ref.read(requestRepositoryProvider);

    await requestRepository.invalidate(requestId);
    await refreshRequest();

    if (!ref.mounted) return;

    await requestRepository.invalidateStats(
      requestId,
      communityId: state.communityId,
    );
    await loadStats();
  }

  
  /// switchCommunity reloads the request with a different community context
  /// without changing the global communitiesProvider.
  Future<void> switchCommunity(String communityId) async {
    state = state.copyWith(communityId: communityId);
    await loadRequestDetails();
  }

  /// loadRequestDetails fetches request details, requester name, location, and
  /// media from the repository.
  @override
  Future<void> loadRequestDetails() async {
    final requestId = state.requestId;
    if (requestId == null) return;

    _log.info('📥 Loading request details for ID: $requestId');

    state = state.copyWith(isLoading: true, error: null);

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final response = await requestRepository.getRequest(
        requestId: requestId,
        communityId: state.communityId,
      );

      // Resolve a community the user is actually a member of before any
      // community-scoped follow-up calls. For multi-community requests the
      // supplied/state communityId may point at a community the user does not
      // belong to, which makes server endpoints that gate on
      // RequireMemberOfActiveCommunity reject with permission_denied (#1705, #1752).
      Set<String>? userMemberIds;
      try {
        final userCommunities = await ref
            .read(communityRepositoryProvider)
            .listUserCommunities();
        userMemberIds = {for (final c in userCommunities) c.id};
      } catch (e) {
        _log.warning('⚠️ Failed to fetch user communities for resolver: $e');
      }

      final communities = response.sharedCommunities;
      final effectiveCommunityId = resolveCommunityId(
        supplied: state.communityId,
        shared: communities,
        userMemberIds: userMemberIds,
      );
      state = state.copyWith(
        requestDetails: response,
        sharedCommunities: communities,
        communityId: effectiveCommunityId,
      );

      if (response.requester.id.isNotEmpty) {
        try {
          final userRepository = ref.read(userRepositoryProvider);
          final userProfile = await userRepository.getUserProfile(
            response.requester.id,
          );
          state = state.copyWith(requesterName: userProfile.user.name);
          _log.info('👤 Requester name fetched: ${userProfile.user.name}');
        } catch (e) {
          _log.warning('⚠️ Failed to fetch requester name: $e');
        }
      }

      if (response.locationId.isNotEmpty) {
        try {
          final locationRepository = ref.read(locationRepositoryProvider);
          final locationResponse = await locationRepository.getLocation(
            response.locationId,
          );
          final formattedName = LocationFormatter.formatLocationNameShort(
            locationResponse,
          );
          state = state.copyWith(
            locationName: formattedName,
            locationLatitude: locationResponse.latitudeDeg,
            locationLongitude: locationResponse.longitudeDeg,
          );
          _log.info(
            '📍 Location fetched: $formattedName (${locationResponse.latitudeDeg}, ${locationResponse.longitudeDeg})',
          );
        } catch (e) {
          _log.warning('⚠️ Failed to fetch location: $e');
        }
      }

      final hasBgMedia = response.mediaIds.isNotEmpty;
      if (hasBgMedia) {
        _log.info(
          '📷 Request has ${response.mediaIds.length} media items! Loading...',
        );
        await loadAllMediaFromServer(response.mediaIds);
        if (!ref.mounted) return;
      } else {
        _log.info('📭 No media associated with this request');
      }

      // Mark metadata loading done; surface the thumbnail immediately so
      // text content is visible while the video controller initializes.
      state = state.copyWith(
        isLoading: false,
        isBackgroundMediaLoading: hasBgMedia,
        backgroundThumbnailUrl:
            hasBgMedia ? state.allMediaItems.firstOrNull?.thumbnailUrl : null,
      );

      // Fire-and-forget: initialize the video/image controller after the UI
      // has already rendered with textual content + thumbnail.
      if (hasBgMedia) {
        unawaited(loadMediaFromServer(response.mediaIds.first));
      }

      unawaited(loadStats());
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading request: $e', e, stackTrace);
      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// loadStats fetches impact statistics for the Details tab.
  ///
  /// Called after loadRequestDetails and after offer/withdraw mutations. Uses
  /// fire-and-forget from callers so it never blocks the main load path.
  @override
  Future<void> loadStats() async {
    final requestId = state.requestId;
    if (requestId == null) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final statsResponse = await requestRepository.getStats(
        requestId,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;

      state = state.copyWith(requestStats: statsResponse);
    } catch (e) {
      _log.warning('⚠️ Failed to load request stats: $e');
      // Non-fatal: Details tab renders without impact tiles.
    }
  }
}

/// requestProvider is the provider for request state by requestId.
final requestProvider =
    NotifierProvider.family<RequestNotifier, RequestState, String>(
      RequestNotifier.new,
    );
