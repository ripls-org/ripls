import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:protobuf/protobuf.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/utils/community_resolver.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/social.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/presentation/viewmodels/experience_completion_actions.dart';
import 'package:ripls/presentation/viewmodels/experience_media_actions.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/services/providers.dart';

// Presentation derivations for the redesigned content view (lifecycle phase,
// edges graph, distance) live in an extension to keep this file under the
// 1,000-line gate. Re-exported so importers of the view-model get them.
export 'package:ripls/presentation/viewmodels/experience_content_view_data.dart';

part 'experience_view_model.freezed.dart';

final _log = Logger('ExperienceViewModel');

/// State for experience detail functionality.
@freezed
sealed class ExperienceState with _$ExperienceState {
  const factory ExperienceState({
    String? experienceId,
    String? currentUserId,
    String? communityId, // Community context for this experience
    GetExperienceResponse? experienceDetails,
    String? ownerName,
    String? locationName,
    double? locationLatitude,
    double? locationLongitude,
    double? locationDistanceMeters,
    @Default(true) bool isLoading,
    @Default(false) bool isEditing,
    @Default(false) bool isSaving,
    @Default(false) bool isUploading,
    UserError? error,
    // Media state
    String? mediaPath,
    String? mediaId,
    @Default(false) bool isVideo,
    @Default(true) bool isMuted,
    @Default([]) List<MediaItemData> allMediaItems,
    // True while the video controller is being initialized; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
    // Completion flow state
    @Default(false) bool isLoadingSummary, // AI is generating summary
    @Default(false) bool isEditingSummary, // Showing summary editor UI
    @Default(false) bool isCompleting, // Saving completion
    String? completionSummary, // Current/edited summary text
    ImpactEstimate? completionSavings, // Impact metrics from completion
    // All communities this experience is shared with.
    @Default([]) List<SharedCommunity> sharedCommunities,
    // Impact stats (loaded alongside details)
    GetExperienceStatsResponse? experienceStats,
    // Tab navigation state
    @Default(0) int activeTab,
    @Default(0) int selectedMediaIndex,
    // Number of images that failed during a batch upload (null when no batch in progress).
    int? batchUploadFailedCount,
  }) = _ExperienceState;

  const ExperienceState._();

  /// Returns whether the current user is the owner of the experience.
  bool get isOwner =>
      currentUserId != null &&
      experienceDetails != null &&
      currentUserId == experienceDetails!.experience.owner.id;

  /// The viewer's own reply, read from the roster the server returned.
  ///
  /// Derived rather than stored. Several paths publish [experienceDetails],
  /// each after its own async gap, so a slower one can land after a faster
  /// one. A stored copy of the viewer's reply is only as fresh as whoever
  /// wrote it last, which is not the same as whoever fetched last — so the
  /// roster on screen could show the viewer's reply while this said they had
  /// none, in the same card (#2727). Deriving it means one fetch publishes
  /// both, and they cannot disagree.
  RSVPIntention? get currentUserIntention {
    final me = currentUserId;
    final rsvps = experienceDetails?.rsvps;
    if (me == null || rsvps == null) return null;
    for (final rsvp in rsvps) {
      if (rsvp.user.id == me) return rsvp.intention;
    }
    return null;
  }

  /// True when the cover-photo picker overlay should be shown.
  bool get canEditCoverPhoto => isEditing && isOwner;

  /// Returns whether the state has an error.
  bool get hasError => error != null;

  bool get isCompleted =>
      experienceDetails?.experience.state ==
      pb.ExperienceState.EXPERIENCE_STATE_COMPLETED;

  /// Index of the chat tab in the two-tab layout. The Results tab was
  /// dropped in favour of the inline impact row on the Event tab — chat
  /// is always at index 1 now, completed or not.
  int get chatTabIndex => 1;
}

/// ExperienceNotifier manages experience state.
///
/// Parameterized by experienceId via the family modifier.
/// Media upload/delete/reorder behaviour is provided by [ExperienceMediaActionsMixin].
class ExperienceNotifier extends Notifier<ExperienceState>
    with
        SafeNotifierMixin<ExperienceState>,
        ExperienceCompletionActionsMixin,
        ExperienceMediaActionsMixin {
  /// ExperienceNotifier accepts the experienceId parameter from the family modifier.
  ExperienceNotifier(this.experienceId);

  /// The experienceId for this specific experience instance.
  final String experienceId;

  /// Monotonic counter incremented at the start of every load/refresh.
  /// Each call captures its generation; on completion it only writes state
  /// when its generation is still the latest. Without this, batches that
  /// invalidate the experience cache N times in rapid succession (e.g. the
  /// owner submitting N location-poll spots at once) kick off N parallel
  /// refreshes, and the slowest stale fetch can write LAST — overwriting
  /// the latest fetch's correct proposal count with an older snapshot.
  int _loadGeneration = 0;

  /// Debounce timer for [scheduleRefresh]. See that method.
  Timer? _scheduledRefreshTimer;

  /// Debounce window for [scheduleRefresh] — see `docs/client/caching.md`
  /// Pattern 7 ("Event-Driven Cache Invalidation"). 300 ms is the same
  /// window the daily screen uses; it's chosen to coalesce the cascade
  /// invalidations that a single user action (e.g. submitting N
  /// location-poll proposals in sequence) produces.
  static const Duration _scheduledRefreshDebounce = Duration(milliseconds: 300);

  /// Schedules a silent refresh of experience details, coalescing rapid
  /// invocations into a single fetch.
  ///
  /// Per `docs/client/caching.md` Pattern 7: a single user action can fire
  /// the content-invalidation listener many times in quick succession
  /// (e.g. submitting 3 location-poll proposals = 3 cache invalidations =
  /// 3 listener fires). Each one would otherwise kick off its own refresh.
  /// Those refreshes share the cache's stampede-dedup, so the FIRST fetch
  /// to start wins for cache repopulation — and if that fetch captured the
  /// server state BEFORE all the mutations committed, every subsequent
  /// cache hit returns the same stale snapshot until a manual refresh
  /// clears it.
  ///
  /// Debouncing ensures one refresh fires AFTER the mutation burst is
  /// done, when the server reflects the final state.
  void scheduleRefresh({String? communityId}) {
    _scheduledRefreshTimer?.cancel();
    _scheduledRefreshTimer = Timer(_scheduledRefreshDebounce, () {
      _scheduledRefreshTimer = null;
      // The notifier may have been disposed between the cascade and the
      // debounce fire (e.g. the user navigated away). `ref.mounted` is
      // the canonical Riverpod disposal check.
      if (!ref.mounted) return;
      // Fire-and-forget: callers of scheduleRefresh don't await the actual
      // refresh; they only want to ensure one happens.
      refreshExperienceDetails(communityId: communityId);
    });
  }

  @override
  ExperienceState build() {
    // Cancel any pending debounced refresh on disposal so we don't try
    // to write into a disposed notifier.
    ref.onDispose(() {
      _scheduledRefreshTimer?.cancel();
      _scheduledRefreshTimer = null;
    });
    // Re-key the viewer identity when the authenticated user changes while
    // this notifier is alive. currentUserId is otherwise seeded exactly once
    // by [initialize] from the widget's mount-time auth read, so a guest who
    // completes phone-OTP registration from an event share link keeps
    // matching RSVPs against the stale (null) id — the roster renders
    // "You haven't replied yet" and the RSVP CTA reappears even though the
    // registration flow already recorded the RSVP server-side (#2724).
    ref.listen(authStateProvider, (previous, next) {
      final userId = next.user?.id;
      // Logout (userId == null) is handled by the auth guards on the
      // load/refresh paths; only a new or switched identity matters here.
      if (userId == null || userId == state.currentUserId) return;
      state = state.copyWith(currentUserId: userId);
      if (state.experienceDetails == null) {
        // Nothing fetched yet — unauthenticated loads bail at the auth
        // guard, leaving isLoading stuck true. Run the full load (which
        // resolves isLoading) plus the stats fetch, mirroring [initialize].
        loadExperienceDetails();
        loadStats();
      } else {
        // The loaded snapshot was fetched under the old identity — possibly
        // before the registration-time RSVP write — so invalidate and
        // refetch instead of merely re-matching the stale RSVP list.
        refreshExperienceDetails(communityId: state.communityId);
      }
    });
    // Seed mute state from the session-wide `videoUnmutedProvider` so a
    // user who has already chosen "audio on" hears the next experience
    // when they swipe (issue #1250). Use `ref.read` — `ref.watch` here
    // would replace state entirely on every toggle, wiping unrelated
    // fields like RSVP counts.
    final isUnmuted = ref.read(videoUnmutedProvider);
    return ExperienceState(experienceId: experienceId, isMuted: !isUnmuted);
  }

  /// initialize loads experience details and fires a background stats fetch.
  Future<void> initialize({
    required String experienceId,
    required String? currentUserId,
    String? communityId,
    double? initialDistanceMeters,
  }) async {
    state = state.copyWith(
      experienceId: experienceId,
      currentUserId: currentUserId,
      communityId: communityId,
      locationDistanceMeters: initialDistanceMeters,
      isLoading: true,
      error: null,
    );

    await loadExperienceDetails(communityId: communityId);

    // Fire-and-forget: load impact stats after details are ready.
    unawaited(loadStats());
  }

  /// Reloads experience details against a different per-screen community
  /// context. Only mutates this viewmodel's state — communitiesProvider
  /// (the user's portfolio) is global and unrelated.
  Future<void> switchCommunity(String communityId) async {
    state = state.copyWith(communityId: communityId);
    await loadExperienceDetails(communityId: communityId);
  }

  /// refreshExperienceDetails silently reloads experience data and impact stats.
  ///
  /// Uses [refresh] (no isLoading spinner) instead of [loadExperienceDetails]
  /// to avoid flicker. Called by the contentCacheInvalidationProvider listener
  /// after mutations (RSVP, chat messages) and explicitly after time/location
  /// updates.
  Future<void> refreshExperienceDetails({String? communityId}) async {
    final experienceId = state.experienceId;
    if (experienceId == null) return;

    // Bail if the user is no longer authenticated. This guard prevents a
    // late-firing contentCacheInvalidationProvider notification (queued by
    // an in-flight mutation) from racing logout and triggering a state
    // write in the same frame as the IndexedStack rebuild — which would
    // cause a duplicate NavigatorState GlobalKey crash.
    if (ref.read(authStateProvider).user == null) return;

    _log.info(
      '🔄 refreshExperienceDetails: starting silent refresh for $experienceId',
    );

    final experienceRepository = ref.read(experienceRepositoryProvider);
    await experienceRepository.invalidate(
      experienceId,
      communityId: communityId,
    );
    await refresh();

    if (!ref.mounted) return;

    // Invalidate stats cache so the server recalculates with current data.
    await experienceRepository.invalidateStats(
      experienceId,
      communityId: state.communityId,
    );
    await loadStats();

    _log.info('🔄 refreshExperienceDetails: completed for $experienceId');
  }

  Future<void> loadExperienceDetails({String? communityId}) async {
    final experienceId = state.experienceId;
    if (experienceId == null) return;

    // Bail if the user is no longer authenticated. Same guard as
    // refreshExperienceDetails — see comment there.
    if (ref.read(authStateProvider).user == null) return;

    // Preserve the community context established at load time so that mutations
    // (upload, delete, reorder) that call loadExperienceDetails() without an
    // explicit communityId don't revert to the no-community-context response,
    // which can return a different conversationId and cause InlineConversationView
    // to be remounted with stale carousel closures.
    final fetchCommunityId = communityId ?? state.communityId;

    state = state.copyWith(isLoading: true, error: null);

    final generation = ++_loadGeneration;
    try {
      // Use repository for cached experience data
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final response = await experienceRepository.getExperienceDetails(
        experienceId,
        communityId: fetchCommunityId,
      );

      // Check if still mounted after async gap
      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;
      // Skip the write if a newer load/refresh has started — see field
      // comment on [_loadGeneration].
      if (generation != _loadGeneration) return;

      // Resolve a community the user is actually a member of before any
      // community-scoped follow-up calls. For multi-community experiences the
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
      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;

      final communities = response.sharedCommunities;
      final effectiveCommunityId = resolveCommunityId(
        supplied: state.communityId,
        shared: communities,
        userMemberIds: userMemberIds,
      );
      state = state.copyWith(
        experienceDetails: response,
        sharedCommunities: communities,
        communityId: effectiveCommunityId,
      );

      // Fetch owner name using repository (with caching)
      if (response.experience.owner.id.isNotEmpty) {
        try {
          final userRepository = ref.read(userRepositoryProvider);
          final userProfile = await userRepository.getUserProfile(
            response.experience.owner.id,
          );

          // Check if still mounted after async gap
          if (!ref.mounted) return;
          if (ref.read(authStateProvider).user == null) return;

          state = state.copyWith(ownerName: userProfile.user.name);
        } catch (e) {
          _log.warning('⚠️ Failed to fetch owner name: $e');
        }
      }

      // Fetch location name and coordinates
      if (response.experience.locationId.isNotEmpty) {
        try {
          final locationRepository = ref.read(locationRepositoryProvider);
          final locationResponse = await locationRepository.getLocation(
            response.experience.locationId,
          );

          // Check if still mounted after async gap
          if (!ref.mounted) return;
          if (ref.read(authStateProvider).user == null) return;

          // Use LocationFormatter to get the short name (POI name, street address, or city)
          final formattedName = LocationFormatter.formatLocationNameShort(
            locationResponse,
          );
          state = state.copyWith(
            locationName: formattedName,
            locationLatitude: locationResponse.latitudeDeg,
            locationLongitude: locationResponse.longitudeDeg,
          );
        } catch (e) {
          _log.warning('⚠️ Failed to fetch location: $e');
        }
      }

      // Load all media for carousel and background.
      if (response.experience.mediaIds.isNotEmpty) {
        await loadAllMediaFromServer(response.experience.mediaIds);

        // Check if still mounted after async gap.
        if (!ref.mounted) return;
        if (ref.read(authStateProvider).user == null) return;
      }

      // Check if still mounted before metadata-complete state update.
      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;

      // Mark metadata loading done and surface the thumbnail immediately so
      // text content is visible while the video controller initializes.
      final hasBgMedia = response.experience.mediaIds.isNotEmpty;
      state = state.copyWith(
        isLoading: false,
        isBackgroundMediaLoading: hasBgMedia,
        backgroundThumbnailUrl: hasBgMedia
            ? state.allMediaItems.firstOrNull?.thumbnailUrl
            : null,
      );

      // Fire-and-forget: initialize the video/image controller after the UI
      // has already rendered with textual content + thumbnail.
      if (hasBgMedia) {
        final firstMediaId = response.experience.mediaIds.first;
        unawaited(loadMediaFromServer(firstMediaId));
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading experience: $e', e, stackTrace);

      // Check if still mounted before updating error state
      if (!ref.mounted) return;

      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// refresh silently reloads experience data without resetting [isLoading].
  ///
  /// Used by [ConversationViewModel] to sync [ExperienceContentView] after
  /// chat-side mutations (RSVP, time vote, lock time, etc.) without replacing
  /// the visible content with a loading spinner. Silently swallows errors so
  /// the existing content stays visible if the refresh fails.
  Future<void> refresh() async {
    final id = state.experienceId;
    if (id == null || id.isEmpty) return;

    // Bail if the user is no longer authenticated. Same logout-race guard
    // as refreshExperienceDetails; needed here too because refresh() is
    // called directly from ConversationViewModel and from the debounced
    // scheduleRefresh timer, which can fire after logout.
    if (ref.read(authStateProvider).user == null) return;

    final generation = ++_loadGeneration;
    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final response = await experienceRepository.getExperienceDetails(
        id,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;
      // Skip the write if a newer load/refresh has started — see field
      // comment on [_loadGeneration].
      if (generation != _loadGeneration) return;

      state = state.copyWith(
        experienceDetails: response,
        error: null,
      );

      // Resolve the place name if it isn't set yet. refresh() normally inherits
      // the locationName the initial load resolved, but when this refresh
      // *supersedes* that load (the web RSVP-handoff case — the load bails at
      // its post-fetch generation guard before reaching its own location
      // fetch), nothing would otherwise resolve it and WHERE falls back to
      // "TBD". Cheap and skipped on the common path (place already set).
      final place = state.locationName;
      if ((place == null || place.isEmpty) &&
          response.experience.locationId.isNotEmpty) {
        try {
          final loc = await ref
              .read(locationRepositoryProvider)
              .getLocation(response.experience.locationId);
          if (!ref.mounted || generation != _loadGeneration) return;
          state = state.copyWith(
            locationName: LocationFormatter.formatLocationNameShort(loc),
            locationLatitude: loc.latitudeDeg,
            locationLongitude: loc.longitudeDeg,
          );
        } catch (e) {
          _log.warning('⚠️ refresh: failed to resolve location: $e');
        }
      }

      await loadAllMediaFromServer(response.experience.mediaIds);

      // If the first media item changed (e.g. after reorder), refresh the
      // background image to match.
      if (response.experience.mediaIds.isNotEmpty) {
        final newFirstId = response.experience.mediaIds.first;
        if (newFirstId != state.mediaId) {
          await loadMediaFromServer(newFirstId);
        }
      }

      _log.fine('🔄 Experience refreshed in background: $id');
    } catch (e) {
      _log.warning('⚠️ Failed to background-refresh experience $id: $e');
      // Silently ignore — existing content stays visible.
    }
  }

  /// refreshWithoutLoadingSpinner implements the cross-mixin hook used by
  /// [ExperienceMediaActionsMixin] to trigger a silent refresh after mutations.
  @override
  Future<void> refreshWithoutLoadingSpinner() => refresh();

  void toggleEditMode() {
    state = state.copyWith(isEditing: !state.isEditing);
  }

  Future<void> saveChanges({
    required String name,
    required String description,
    ExperienceTime? time,
    int? maxParticipants,
    String? sourceUrl,
    SocialContext? socialContext,
  }) async {
    if (state.isSaving || state.experienceDetails == null) return;

    state = state.copyWith(isSaving: true);

    try {
      final experienceDetails = state.experienceDetails!;

      // Call saveExperience to update the experience
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.saveExperience(
        id: experienceDetails.experience.id,
        name: name.trim(),
        description: description.trim(),
        mediaIds:
            experienceDetails.experience.mediaIds, // Preserve existing media
        time: time,
        maxParticipants: maxParticipants,
        sourceUrl: sourceUrl,
        socialContext: socialContext,
      );

      // Reload experience details to get updated data
      await loadExperienceDetails();

      safeUpdateState((s) => s.copyWith(isEditing: false, isSaving: false));
    } catch (e, stackTrace) {
      _log.severe('❌ Error saving experience: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(isSaving: false));
      rethrow; // Let caller handle the error display
    }
  }

  void cancelEdit() {
    state = state.copyWith(isEditing: false);
  }

  /// saveSocialContext saves only the social dimension attributes for the
  /// current experience, performing a silent background refresh so impact
  /// tiles update without a loading spinner.
  ///
  /// Only [id], [mediaIds], and [socialContext] are sent. Text fields, time,
  /// and location are intentionally omitted so the server does not treat them
  /// as changed — in particular, passing [time] would set timeChanged=true and
  /// emit a spurious DETAIL_CHANGED system message in the conversation.
  /// [mediaIds] must always be sent because the server always replaces the
  /// repeated field (even with an empty slice), which would clear existing media.
  Future<void> saveSocialContext(SocialContext socialContext) async {
    final details = state.experienceDetails;
    if (details == null) return;
    final exp = details.experience;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.saveExperience(
        id: exp.id,
        mediaIds: exp.mediaIds,
        socialContext: socialContext,
      );
      await refreshExperienceDetails(communityId: state.communityId);
    } catch (e, stackTrace) {
      _log.severe('❌ Error saving social context: $e', e, stackTrace);
      rethrow;
    }
  }

  /// Updates the location of the experience item.
  Future<void> updateLocation(String locationId) async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final experience = state.experienceDetails!;

      // Preserve all existing fields when updating location
      await experienceRepository.saveExperience(
        id: experience.experience.id,
        name: experience.experience.name,
        description: experience.experience.description,
        mediaIds: experience.experience.mediaIds,
        locationId: locationId,
        time: experience.experience.time,
        maxParticipants: experience.experience.maxParticipants,
      );

      // Reload experience details to get updated location
      await loadExperienceDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error updating location: $e', e, stackTrace);
      safeUpdateState(
        (s) => s.copyWith(
          error: const UserError.generic(fallback: 'Could not update location'),
        ),
      );
      rethrow;
    }
  }

  /// Updates the time for the current experience.
  ///
  /// Preserves all other fields. Uses [refreshExperienceDetails] for a silent
  /// reload of both experience data and impact stats after the save.
  Future<void> updateTime(ExperienceTime time) async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final experience = state.experienceDetails!;

      // Preserve all existing fields when updating time
      await experienceRepository.saveExperience(
        id: experience.experience.id,
        name: experience.experience.name,
        description: experience.experience.description,
        mediaIds: experience.experience.mediaIds,
        locationId: experience.experience.locationId,
        time: time,
        maxParticipants: experience.experience.maxParticipants,
      );

      _log.info('⏱️ updateTime: saved, refreshing details + stats');
      await refreshExperienceDetails(communityId: state.communityId);
    } catch (e, stackTrace) {
      _log.severe('❌ Error updating time: $e', e, stackTrace);
      safeUpdateState(
        (s) => s.copyWith(
          error: const UserError.generic(fallback: 'Could not update time'),
        ),
      );
      rethrow;
    }
  }

  /// Deletes the current experience item.
  /// Throws an error if deletion fails.
  Future<void> deleteExperience() async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.deleteExperience(
        state.experienceDetails!.experience.id,
      );
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting experience: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  /// Updates the current user's RSVP intention.
  ///
  /// The repository's rsvp() method fires _onContentInvalidated, which triggers
  /// the contentCacheInvalidationProvider listener in ExperienceContentView.
  /// That listener calls refreshExperienceDetails() which silently reloads
  /// experience data + stats. No explicit refresh or stats reload needed here.
  Future<GetExperienceResponse> updateRSVP(RSVPIntention intention) async {
    if (state.experienceDetails == null) {
      throw Exception('Experience not loaded');
    }

    if (state.communityId == null) {
      throw Exception('Community ID not available');
    }

    _log.info('📝 updateRSVP: sending $intention for ${state.experienceId}');

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.rsvp(
        experienceId: state.experienceDetails!.experience.id,
        communityId: state.communityId!,
        intention: intention,
      );

      _log.info(
        '📝 updateRSVP: rsvp sent, listener will handle refresh + stats',
      );

      // Return current state — the listener-triggered refreshExperienceDetails()
      // will update state reactively (silent refresh + stats reload).
      return state.experienceDetails!;
    } catch (e, stackTrace) {
      _log.severe('❌ Error updating RSVP: $e', e, stackTrace);
      rethrow;
    }
  }

  /// Marks experience as in process (owner only).
  Future<void> markInProcess() async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.markInProcess(
        state.experienceDetails!.experience.id,
      );

      // Reload experience details to get updated state
      await loadExperienceDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error marking in process: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  /// Records attendance for multiple users (owner only).
  ///
  /// Reloads experience details to reflect updated attendance.
  Future<void> recordAttendance(List<AttendanceRecord> attendance) async {
    if (state.experienceDetails == null || state.communityId == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.recordAttendance(
        experienceId: state.experienceDetails!.experience.id,
        communityId: state.communityId!,
        attendance: attendance,
      );

      // Reload experience details to get updated attendance
      await loadExperienceDetails(communityId: state.communityId);
    } catch (e, stackTrace) {
      _log.severe('❌ Error recording attendance: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  /// Implements [ExperienceCompletionActionsMixin.reloadExperienceDetails]
  /// by delegating to the full [loadExperienceDetails] flow.
  @override
  Future<void> reloadExperienceDetails() => loadExperienceDetails();

  /// Marks experience as cancelled (owner only).
  Future<void> cancelExperience() async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.cancelExperience(
        state.experienceDetails!.experience.id,
      );

      // Reload experience details to get updated state
      await loadExperienceDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error cancelling experience: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  /// Resolves the loaded experience, an optional public Ripls invite URL,
  /// and a human-readable address suitable for the calendar entry.
  ///
  /// The widget passes the returned URL and address to
  /// [CalendarHelper.addToCalendar] — this notifier never touches the platform
  /// calendar UI itself. Resolving the address here (rather than in the helper)
  /// keeps the helper free of network calls and reuses the cached
  /// [LocationRepository] lookup the rest of the screen already performs.
  Future<
    ({pb.Experience experience, String? riplsUrl, String? locationDisplay})?
  >
  exportToCalendar() async {
    final experience = state.experienceDetails?.experience;
    if (experience == null) return null;

    final communityId = state.communityId;
    String? riplsUrl;
    if (communityId != null && communityId.isNotEmpty) {
      try {
        final response = await ref
            .read(communityRepositoryProvider)
            .getOrCreateShareLink(
              communityId: communityId,
              experienceId: experience.id,
            );
        if (!ref.mounted) return null;
        riplsUrl = response.shareUrl.isNotEmpty ? response.shareUrl : null;
      } catch (e, stackTrace) {
        _log.warning(
          'Failed to fetch invite link for calendar export',
          e,
          stackTrace,
        );
        // Don't block export on URL minting — fall through with null URL.
      }
    }

    String? locationDisplay;
    if (experience.locationId.isNotEmpty) {
      try {
        final location = await ref
            .read(locationRepositoryProvider)
            .getLocation(experience.locationId);
        if (!ref.mounted) return null;
        locationDisplay = LocationFormatter.formatLocationDetailed(location);
      } catch (e, stackTrace) {
        _log.warning(
          'Failed to resolve address for calendar export',
          e,
          stackTrace,
        );
        // Don't block export — CalendarHelper will fall back to lat/lng.
      }
    }

    return (
      experience: experience,
      riplsUrl: riplsUrl,
      locationDisplay: locationDisplay,
    );
  }

  /// Shares the experience with a community.
  Future<void> shareWithCommunity(String communityId) async {
    if (state.experienceDetails == null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.shareExperience(
        experienceId: state.experienceDetails!.experience.id,
        communityId: communityId,
      );
    } catch (e, stackTrace) {
      _log.severe('❌ Error sharing experience: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  /// loadStats fetches impact statistics for this experience.
  ///
  /// Called by [refreshExperienceDetails] and [initialize] to keep impact tiles
  /// current. Silently ignores errors so the UI stays functional.
  Future<void> loadStats() async {
    if (!ref.mounted) return;
    final id = state.experienceId;
    if (id == null || id.isEmpty) return;

    // Bail if the user is no longer authenticated — same logout-race guard
    // as refreshExperienceDetails.
    if (ref.read(authStateProvider).user == null) return;

    _log.info('📊 loadStats: fetching stats for $id');

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final stats = await experienceRepository.getStats(
        id,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;
      if (ref.read(authStateProvider).user == null) return;

      final hasPotential = stats.hasPotentialImpact();
      final hasImpact = stats.hasImpact();
      _log.info(
        '📊 loadStats: received (hasPotentialImpact=$hasPotential, hasImpact=$hasImpact)',
      );

      state = state.copyWith(experienceStats: stats);
    } catch (e) {
      _log.warning('⚠️ Failed to load experience stats: $e');
    }
  }

  /// Applies [patch] to a clone of the cached [Experience] and publishes
  /// a fresh [ExperienceState]. Used by sibling notifiers to propagate
  /// known mutations (new proposal, recorded vote, confirmed winner)
  /// without an RPC. The clone matters — mutating in place leaves the
  /// response and inner experience references identical to the prior
  /// state, so Freezed-based equality treats it as unchanged and
  /// dependents wouldn't rebuild until pull-to-refresh.
  void applyExperiencePatch(void Function(pb.Experience exp) patch) {
    final details = state.experienceDetails;
    if (details == null) return;
    final patchedExperience = details.experience.deepCopy();
    patch(patchedExperience);
    final patchedDetails = details.deepCopy()..experience = patchedExperience;
    state = state.copyWith(experienceDetails: patchedDetails);
  }

  /// setActiveTab sets the active tab index for the tabbed content layout.
  void setActiveTab(int index) {
    safeUpdateState((s) => s.copyWith(activeTab: index));
  }

  /// setSelectedMedia sets the selected media index for the media pane.
  void setSelectedMedia(int index) {
    safeUpdateState((s) => s.copyWith(selectedMediaIndex: index));
  }

  /// toggleMute toggles video mute state and updates the session-wide
  /// `videoUnmutedProvider` so the choice persists across feed swipes
  /// and drives the audio-session category swap (issue #1250).
  void toggleMute() {
    final newMuted = !state.isMuted;
    state = state.copyWith(isMuted: newMuted);
    ref.read(videoUnmutedProvider.notifier).set(!newMuted);
  }
}

/// Provider for experience state.
///
/// Uses .family to create separate instances per experienceId.
/// Uses .autoDispose to clean up state when widget is disposed.
final experienceProvider = NotifierProvider.autoDispose
    .family<ExperienceNotifier, ExperienceState, String>(
      ExperienceNotifier.new,
    );
