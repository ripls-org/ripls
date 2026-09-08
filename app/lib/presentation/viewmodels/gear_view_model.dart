// dart-line-count-allow: #2003 — GearNotifier split into mixins is a follow-up refactor
import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:protobuf/protobuf.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/community_resolver.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/media_upload_helper.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show Estimate, SharedCommunity, TrackedEstimate, TrackedString;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata, GetGearPeopleResponse, GetGearResponse, GetGearStatsResponse;
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/presentation/widgets/gear/gear_metadata_sheet.dart'
    show GearMetadataEditResult;
import 'package:ripls/services/providers.dart';

part 'gear_view_model.freezed.dart';

final _log = Logger('GearViewModel');

/// State for gear detail functionality
@freezed
sealed class GearState with _$GearState {
  const factory GearState({
    String? gearId,
    String? currentUserId,
    String? communityId,
    GetGearResponse? gearDetails,
    String? ownerName,
    String? locationName,
    String? locationLocality,
    String? locationRegionCode,
    double? locationLatitude,
    double? locationLongitude,
    double? locationDistanceMeters,
    @Default(true) bool isLoading,
    // True once the first loadGearDetails pass has fully settled (details +
    // owner + location + transfer context). The content view holds on its
    // loading treatment until then: gearDetails lands mid-pass, and painting
    // the read shell at that point films placeholder facts — "WHERE / TBD",
    // an empty owner avatar, a false "no one's raised their hand" empty
    // state (#2724). Later reloads keep the previous hydrated state on
    // screen, so they never re-trip the loading view.
    @Default(false) bool hasHydrated,
    @Default(false) bool isEditing,
    @Default(false) bool isSaving,
    @Default(false) bool isUploadingMedia,
    UserError? error,
    // Media state
    String? mediaPath,
    String? mediaId,
    @Default(false) bool isVideo,
    @Default(true) bool isMuted,
    @Default([]) List<MediaItemData> allMediaItems,
    Attribution?
    backgroundAttribution, // Attribution for background media (overflow menu)
    // True while the video controller is being initialized; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
    // Transfer request count state
    int? transferRequestCount,
    bool? hasActiveRequest,
    String? activeConversationId,
    int? participantCount,
    // Transfer context (full data for requester row)
    GearTransferContext? transferContext,
    // All communities this gear is shared with (for community-switcher dropdown).
    @Default([]) List<SharedCommunity> sharedCommunities,
    // Active tab index for the tabbed content layout (0=Type, 1=Details, 2=Chat, 3=Media).
    @Default(0) int activeTab,
    // Index of the selected media item shown in the background.
    @Default(0) int selectedMediaIndex,
    // Impact and usage statistics for the Details tab.
    GetGearStatsResponse? gearStats,
    // Owner, borrowers, and interested parties for the Details tab.
    GetGearPeopleResponse? gearPeople,
    // Number of images that failed during a batch upload (null when no batch in progress).
    int? batchUploadFailedCount,
  }) = _GearState;

  const GearState._();

  /// Returns whether the current user is the owner of the gear
  bool get isOwner =>
      currentUserId != null &&
      gearDetails != null &&
      currentUserId == gearDetails!.owner.id;

  /// canEditCoverPhoto reports whether the cover-photo picker overlay should be
  /// shown. Owners may swap the cover photo whenever the gear is in edit mode.
  bool get canEditCoverPhoto => isEditing && isOwner;

  /// The item's current sharing mode (loan vs giveaway) in the loaded community
  /// context, or null before details load.
  Availability? get availability => gearDetails?.availability;

  /// Whether the item is currently shared for giveaway (vs loan).
  bool get isGiveaway =>
      gearDetails?.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;

  /// Whether a loan or giveaway is in progress for this item, which locks the
  /// lend/give toggle — the server rejects a mode change mid-transfer. Derived
  /// from the loaded transfer context (pending interest or a chosen recipient)
  /// and any active loan; the server guard is authoritative.
  bool get hasInFlightTransfer =>
      (gearDetails?.hasActiveLoan() ?? false) ||
      (transferContext?.pendingRequests.isNotEmpty ?? false) ||
      (transferContext?.hasSelectedRecipient() ?? false);

  /// Returns whether the state has an error
  bool get hasError => error != null;
}

/// Notifier for managing gear state
/// This notifier is parameterized by gearId via the family modifier
class GearNotifier extends Notifier<GearState> with SafeNotifierMixin<GearState> {
  /// Constructor accepts the gearId parameter from the family modifier
  GearNotifier(this.gearId);

  /// The gearId for this specific gear instance
  final String gearId;

  @override
  GearState build() {
    // Seed mute state from the session-wide `videoUnmutedProvider` so a
    // user who has already chosen "audio on" hears the next gear background
    // when they navigate to it (issue #1250).
    final isUnmuted = ref.read(videoUnmutedProvider);
    return GearState(gearId: gearId, isMuted: !isUnmuted);
  }

  /// setActiveTab updates the active tab index in the tabbed content layout.
  void setActiveTab(int index) {
    state = state.copyWith(activeTab: index);
  }

  /// setSelectedMedia updates the selected media index shown in the background.
  void setSelectedMedia(int index) {
    final allMedia = state.allMediaItems;
    if (allMedia.isEmpty || index < 0 || index >= allMedia.length) return;

    final media = allMedia[index];
    state = state.copyWith(
      selectedMediaIndex: index,
      mediaPath: media.url,
      mediaId: media.id,
      isVideo: media.isVideo,
      backgroundAttribution: media.attribution,
    );
  }

  /// Initializes and loads gear details
  Future<void> initialize({
    required String gearId,
    required String? currentUserId,
    String? communityId,
    double? initialDistanceMeters,
  }) async {
    state = state.copyWith(
      gearId: gearId,
      currentUserId: currentUserId,
      communityId: communityId,
      locationDistanceMeters: initialDistanceMeters,
      isLoading: true,
      error: null,
    );

    await loadGearDetails(communityId: communityId);
  }

  /// loadStats fetches impact statistics for the gear item.
  Future<void> loadStats() async {
    final gearId = state.gearId;
    if (gearId == null) return;

    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      final stats = await gearRepository.getStats(
        gearId,
        communityId: state.communityId,
      );
      safeUpdateState((s) => s.copyWith(gearStats: stats));
    } catch (e) {
      _log.warning('Failed to load gear stats: $e');
    }
  }

  /// loadPeople fetches owner, borrowers, and interested parties for the gear.
  Future<void> loadPeople() async {
    final gearId = state.gearId;
    if (gearId == null) return;

    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      final people = await gearRepository.getPeople(
        gearId,
        communityId: state.communityId,
      );
      safeUpdateState((s) => s.copyWith(gearPeople: people));
    } catch (e) {
      _log.warning('Failed to load gear people: $e');
    }
  }

  /// _invalidateAndReloadStatsAndPeople invalidates cached stats and people
  /// data, then reloads both from the server.
  Future<void> _invalidateAndReloadStatsAndPeople() async {
    final gearId = state.gearId;
    if (gearId == null) return;

    final gearRepository = ref.read(gearRepositoryProvider);
    await gearRepository.invalidateStats(gearId, communityId: state.communityId);
    await gearRepository.invalidatePeople(gearId, communityId: state.communityId);
    if (!ref.mounted) return;
    unawaited(loadStats());
    unawaited(loadPeople());
  }

  /// Reloads gear details against a different per-screen community context.
  /// Only mutates this viewmodel's state — communitiesProvider (the user's
  /// portfolio) is global and unrelated.
  Future<void> switchCommunity(String communityId) async {
    state = state.copyWith(communityId: communityId);
    await loadGearDetails(communityId: communityId);
  }

  /// refreshGearDetails invalidates the cache for this gear item and reloads
  /// fresh data from the server. Use this after chat mutations.
  Future<void> refreshGearDetails({String? communityId}) async {
    final gearId = state.gearId;
    if (gearId == null) return;
    final gearRepository = ref.read(gearRepositoryProvider);
    await gearRepository.invalidate(gearId, communityId: communityId);
    await loadGearDetails(communityId: communityId);
  }

  /// Silently reloads gear data and stats without showing a loading spinner.
  ///
  /// Invalidates caches, refetches gear details and stats, and swaps
  /// them into state. No `isLoading: true` transition, so the UI does
  /// not flicker. Use after inline metadata edits.
  Future<void> refreshGearDetailsSilent({String? communityId}) async {
    final gearId = state.gearId;
    if (gearId == null) return;

    final fetchCommunityId = communityId ?? state.communityId;
    final gearRepository = ref.read(gearRepositoryProvider);

    // Invalidate caches so the next fetch goes to the server.
    await gearRepository.invalidate(gearId, communityId: fetchCommunityId);
    await gearRepository.invalidateStats(gearId, communityId: fetchCommunityId);

    if (!ref.mounted) return;

    // Refetch gear details silently (no isLoading flag).
    try {
      final gear = await gearRepository.get(gearId, communityId: fetchCommunityId);
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(
        gearDetails: gear,
        isLoading: false,
        isSaving: false,
        error: null,
      ));
    } catch (e) {
      _log.warning('Silent gear refresh failed: $e');
    }

    // Reload stats separately (also silent).
    if (!ref.mounted) return;
    await loadStats();
  }

  Future<void> loadGearDetails({String? communityId}) async {
    final gearId = state.gearId;
    if (gearId == null) return;

    // Preserve the community context established at load time so that mutations
    // (media reorder, delete, location update) that call loadGearDetails() without
    // an explicit communityId don't revert to a no-community-context response,
    // which returns shared_at_unix_sec = 0 because the server only populates that
    // field when a community_id is provided.
    final fetchCommunityId = communityId ?? state.communityId;

    state = state.copyWith(isLoading: true, error: null);

    try {
      // Use repository for cached gear data
      final gearRepository = ref.read(gearRepositoryProvider);
      final response = await gearRepository.getGearDetails(
        gearId,
        communityId: fetchCommunityId,
      );

      // Resolve a community the user is actually a member of before any
      // community-scoped follow-up calls. For multi-community gear the
      // supplied/state communityId may point at a community the user does not
      // belong to, which makes server endpoints that gate on
      // RequireMemberOfActiveCommunity reject with permission_denied (#1705).
      Set<String>? userMemberIds;
      try {
        final communities = await ref
            .read(communityRepositoryProvider)
            .listUserCommunities();
        userMemberIds = {for (final c in communities) c.id};
      } catch (e) {
        _log.warning('⚠️ Failed to fetch user communities for resolver: $e');
      }
      if (!ref.mounted) return;

      String? effectiveCommunityId;
      if (!safeUpdateState((s) {
        final communities = response.sharedCommunities;
        effectiveCommunityId = resolveCommunityId(
          supplied: s.communityId,
          shared: communities,
          userMemberIds: userMemberIds,
        );
        return s.copyWith(
          gearDetails: response,
          sharedCommunities: communities,
          communityId: effectiveCommunityId,
        );
      })) {
        return;
      }

      // `availability` and `active_loan` are community-scoped — the server only
      // populates them when the GetGear call resolves a CommunityGear for the
      // passed community_id. The fetch above used `fetchCommunityId`, which is
      // often null (deep link, or straight after creation before a community is
      // threaded through). If we only just resolved the gear's community, the
      // first response carried availability=UNSPECIFIED, which the UI renders as
      // a loan — so a giveaway shows up as a loan. Re-fetch with the resolved
      // community so the community-scoped fields are correct.
      if (effectiveCommunityId != null &&
          effectiveCommunityId!.isNotEmpty &&
          effectiveCommunityId != fetchCommunityId) {
        try {
          final scoped = await gearRepository.getGearDetails(
            gearId,
            communityId: effectiveCommunityId,
          );
          if (!ref.mounted) return;
          if (!safeUpdateState((s) => s.copyWith(gearDetails: scoped))) return;
        } catch (e) {
          _log.warning(
              '⚠️ Failed to re-fetch gear for resolved community: $e');
        }
      }

      // Fetch owner name using repository (with caching)
      if (response.owner.id.isNotEmpty) {
        try {
          final userRepository = ref.read(userRepositoryProvider);
          final userProfile = await userRepository.getUserProfile(
            response.owner.id,
          );
          if (!safeUpdateState((s) => s.copyWith(ownerName: userProfile.user.name))) return;
        } catch (e) {
          _log.warning('⚠️ Failed to fetch owner name: $e');
        }
      }

      // Fetch location name and coordinates
      if (response.locationId.isNotEmpty) {
        try {
          final locationRepository = ref.read(locationRepositoryProvider);
          final locationResponse = await locationRepository.getLocation(
            response.locationId,
          );
          // Use LocationFormatter to get the short name (POI name, street address, or city)
          final formattedName = LocationFormatter.formatLocationNameShort(
            locationResponse,
          );
          if (!safeUpdateState((s) => s.copyWith(
            locationName: formattedName,
            locationLocality: locationResponse.locality,
            locationRegionCode: locationResponse.regionCode,
            locationLatitude: locationResponse.latitudeDeg,
            locationLongitude: locationResponse.longitudeDeg,
          ))) {
            return;
          }
        } catch (e) {
          _log.warning('⚠️ Failed to fetch location: $e');
        }
      }

      // Fetch transfer context for requester row. Use the resolved
      // (membership-aware) community id rather than the parameter passed in:
      // for multi-community gear the parameter can point at a community the
      // user is not a member of, which fails permission_denied (#1705).
      final transferCommunityId = effectiveCommunityId;
      if (transferCommunityId != null && transferCommunityId.isNotEmpty) {
        try {
          final transferRepository = ref.read(transferRepositoryProvider);

          final context = await transferRepository.getGearTransferContext(
            gearId: gearId,
            communityId: transferCommunityId,
          );

          if (!safeUpdateState((s) => s.copyWith(transferContext: context))) return;
        } catch (e, stackTrace) {
          _log.warning('⚠️ Failed to fetch transfer context: $e');
          _log.warning('⚠️ Stack trace: $stackTrace');
        }
      }

      // If gear has media, fetch all media items (thumbnails) for the carousel.
      if (response.mediaIds.isNotEmpty) {
        await _loadAllMediaFromServer(response.mediaIds);
        if (!ref.mounted) return;
      }

      // Mark metadata loading done; surface the thumbnail immediately so
      // text content is visible while the video controller initializes.
      final hasBgMedia = response.mediaIds.isNotEmpty;
      if (!safeUpdateState((s) => s.copyWith(
        isLoading: false,
        hasHydrated: true,
        isBackgroundMediaLoading: hasBgMedia,
        backgroundThumbnailUrl:
            hasBgMedia ? s.allMediaItems.firstOrNull?.thumbnailUrl : null,
      ))) {
        return;
      }

      // Fire-and-forget: initialize the video/image controller after the UI
      // has already rendered with textual content + thumbnail.
      if (hasBgMedia) {
        unawaited(_loadMediaFromServer(response.mediaIds.first));
      }

      // Fire-and-forget: load stats and people for the Details tab.
      unawaited(loadStats());
      unawaited(loadPeople());
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading gear: $e', e, stackTrace);
      // hasHydrated: the pass is over (the error view renders instead); a
      // retry that succeeds repopulates everything in one batch anyway.
      safeUpdateState((s) => s.copyWith(
          isLoading: false,
          hasHydrated: true,
          error: RpcErrorHandler.classify(e)));
    }
  }

  Future<void> _loadAllMediaFromServer(List<String> mediaIds) async {
    try {
      final allMedia = await MediaHelpers.loadAllMedia(ref, mediaIds);
      safeUpdateState((s) => s.copyWith(allMediaItems: allMedia));
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading all media from server: $e', e, stackTrace);
    }
  }

  Future<void> _loadMediaFromServer(String mediaId) async {
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      // Use repository for FULL media URL (not thumbnail) for background display
      final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);

      // Also fetch full media response to get attribution for overflow menu
      final media = await mediaRepository.get(mediaId);
      final backgroundAttribution = media.hasAttribution()
          ? media.attribution
          : null;

      // Use content type for reliable video detection (not URL heuristics)
      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      safeUpdateState((s) => s.copyWith(
        mediaPath: mediaUrl.url,
        mediaId: mediaId,
        isVideo: isVideo,
        backgroundAttribution: backgroundAttribution,
        isBackgroundMediaLoading: false,
        backgroundThumbnailUrl: null,
      ));
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading media from server: $e', e, stackTrace);
      // Clear loading flag so the UI doesn't remain stuck on the thumbnail.
      safeUpdateState((s) => s.copyWith(isBackgroundMediaLoading: false));
    }
  }

  /// toggleMute toggles video mute state and updates the session-wide
  /// `videoUnmutedProvider` so the choice persists across navigation and
  /// drives the audio-session category swap (issue #1250).
  void toggleMute() {
    final newMuted = !state.isMuted;
    state = state.copyWith(isMuted: newMuted);
    ref.read(videoUnmutedProvider.notifier).set(!newMuted);
  }

  void toggleEditMode() {
    state = state.copyWith(isEditing: !state.isEditing);
  }

  /// Saves name, description, and optionally metadata/sourceUrl.
  ///
  /// Also called automatically after media upload/delete to persist the updated
  /// media ID list — in those cases metadata and sourceUrl are omitted and the
  /// server preserves the existing values (partial-update semantics).
  /// Reloads from server after save so the local state always reflects the full
  /// persisted record (avoids constructing a lossy local copy that loses fields
  /// like metadata, valueEstimate, availability, etc.).
  Future<void> saveChanges({
    required String name,
    required String description,
    GearMetadata? metadata,
    String? sourceUrl,
  }) async {
    if (state.isSaving || state.gearDetails == null) return;

    state = state.copyWith(isSaving: true);

    try {
      final gearDetails = state.gearDetails!;

      // Build media IDs list from all media items or current media ID
      final mediaIds = state.allMediaItems.isNotEmpty
          ? state.allMediaItems.map((item) => item.id).toList()
          : (state.mediaId != null ? [state.mediaId!] : <String>[]);

      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.saveGear(
        id: gearDetails.id,
        name: name.trim(),
        description: description.trim(),
        mediaIds: mediaIds,
        metadata: metadata,
        sourceUrl: sourceUrl,
      );

      if (ref.mounted) {
        unawaited(ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(GearEditedEvent(gearId: gearDetails.id)));
      }

      // Reload from server so local state has the full record (metadata,
      // valueEstimate, sourceUrl, availability, etc.) without us having to
      // reconstruct it manually.
      if (!ref.mounted) return;
      await loadGearDetails(communityId: state.communityId);

      safeUpdateState((s) => s.copyWith(isEditing: false, isSaving: false));
    } catch (e, stackTrace) {
      _log.severe('❌ Error saving gear: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(isSaving: false));
      rethrow;
    }
  }

  void cancelEdit() {
    state = state.copyWith(isEditing: false);
  }

  /// setAvailability flips the item's sharing mode (loan vs giveaway) across
  /// every community it is shared with, then reloads so the UI reflects the new
  /// mode. No-op when unchanged. Throws (after recording [GearState.error]) when
  /// the change is rejected — e.g. while a loan or giveaway is in progress — so
  /// the caller can surface the message.
  Future<void> setAvailability(Availability availability) async {
    if (state.isSaving || state.gearDetails == null) return;
    if (state.gearDetails!.availability == availability) return;

    state = state.copyWith(isSaving: true);
    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.setGearAvailability(gearId, availability);
      if (!ref.mounted) return;
      await loadGearDetails(communityId: state.communityId);
      safeUpdateState((s) => s.copyWith(isSaving: false));
    } catch (e, stackTrace) {
      _log.warning('Failed to set gear availability: $e', e, stackTrace);
      // Deliberately do NOT set state.error here: the gear content view renders
      // a full-screen error whenever state.error is set, and a transient
      // action failure (e.g. the in-progress-transfer guard) must not blow away
      // the whole screen. The caller surfaces the message as a toast.
      safeUpdateState((s) => s.copyWith(isSaving: false));
      rethrow;
    }
  }

  /// Persists inline metadata edits without a loading flicker, then silently
  /// refreshes gear + stats so impact metrics update live. Use for the
  /// debounced specs-card auto-save in edit mode.
  ///
  /// Routes the save through the view-model (not a direct repository call from
  /// the widget) so the presentation layer stays out of the data layer.
  Future<void> saveMetadataSilent({
    required String brand,
    required String model,
    required String valueUsd,
    required String weightGrams,
    required String website,
  }) async {
    final gear = state.gearDetails;
    if (gear == null) return;

    final trimmedBrand = brand.trim();
    final trimmedModel = model.trim();
    final weightMean = double.tryParse(weightGrams.trim());
    final valueAmount = double.tryParse(valueUsd.trim());

    final hasBrand = trimmedBrand.isNotEmpty;
    final hasModel = trimmedModel.isNotEmpty;
    final hasWeight = weightMean != null && weightMean > 0;
    final hasValue = valueAmount != null && valueAmount > 0;

    final metadata = GearMetadata(
      brand: hasBrand ? TrackedString(value: trimmedBrand) : null,
      model: hasModel ? TrackedString(value: trimmedModel) : null,
      weightGrams: hasWeight
          ? TrackedEstimate(value: Estimate(mean: weightMean))
          : null,
      valueEstimate:
          hasValue ? ValueEstimate(estimatedValueUsd: valueAmount) : null,
    );

    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.saveGear(
        id: gear.id,
        name: gear.name,
        description: gear.description,
        mediaIds: gear.mediaIds,
        metadata: metadata,
        sourceUrl: website.trim().isNotEmpty ? website.trim() : null,
      );
      if (!ref.mounted) return;
      await refreshGearDetailsSilent(communityId: state.communityId);
    } catch (e) {
      _log.warning('Auto-save metadata failed: $e');
    }
  }

  /// Saves updated metadata from the metadata editing sheet.
  Future<void> saveMetadata(GearMetadataEditResult result) async {
    if (state.gearDetails == null) return;

    state = state.copyWith(isSaving: true);

    try {
      final gear = state.gearDetails!;

      // Merge value estimate into metadata if present
      final metadata = result.metadata;
      if (result.valueEstimate != null) {
        metadata.valueEstimate = result.valueEstimate!;
      }

      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.saveGear(
        id: gear.id,
        name: gear.name,
        description: gear.description,
        mediaIds: gear.mediaIds,
        metadata: metadata,
        sourceUrl: result.sourceUrl,
      );

      // Reload gear details to get updated metadata with provenance
      if (!ref.mounted) return;
      await loadGearDetails(communityId: state.communityId);
    } catch (e, stackTrace) {
      _log.severe('Error saving metadata: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(isSaving: false));
      rethrow;
    }
  }

  /// Updates the location of the gear item.
  Future<void> updateLocation(String locationId) async {
    if (state.gearDetails == null) return;

    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      final gear = state.gearDetails!;

      // Preserve all existing fields when updating location
      await gearRepository.saveGear(
        id: gear.id,
        name: gear.name,
        description: gear.description,
        mediaIds: gear.mediaIds,
        locationId: locationId,
      );

      // Reload gear details to get updated location
      await loadGearDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error updating location: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
            error: const UserError.generic(fallback: 'Could not update location'),
          ));
      rethrow;
    }
  }

  /// Deletes the current gear item.
  /// Throws an error if deletion fails.
  Future<void> deleteGear() async {
    if (state.gearDetails == null) return;

    final gearId = state.gearDetails!.id;

    try {
      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.deleteGear(gearId);

      // Log analytics event (only if still mounted after async gap)
      if (ref.mounted) {
        unawaited(ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(GearDeletedEvent(gearId: gearId)));
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting gear: $e', e, stackTrace);
      rethrow; // Let caller handle the error display
    }
  }

  Future<void> pickImageFromGallery({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickImageFromGallery();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to pick image: $e');
      rethrow;
    }
  }

  /// Picks multiple images from the gallery and uploads them sequentially,
  /// calling saveChanges() once after all uploads complete rather than once
  /// per image.
  Future<void> pickMultipleImagesFromGallery({bool insertAtFront = false}) async {
    final files = await MediaPickerHelper.pickMultipleImagesFromGallery();
    if (files.isEmpty) return;
    await _setMultipleMediaFiles(files, insertAtFront: insertAtFront);
  }

  Future<void> pickImageFromCamera({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickImageFromCamera();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to take photo: $e');
      rethrow;
    }
  }

  Future<void> pickVideoFromGallery({bool insertAtFront = false}) async {
    try {
      final file = await MediaPickerHelper.pickVideoFromGallery();
      if (file != null) {
        await _setMediaFile(file, insertAtFront: insertAtFront);
      }
    } catch (e) {
      _log.severe('❌ Failed to pick video: $e');
      rethrow;
    }
  }

  @visibleForTesting
  Future<void> setMediaFileForTesting(
    XFile file, {
    bool insertAtFront = false,
  }) =>
      _setMediaFile(file, insertAtFront: insertAtFront);

  Future<void> _setMediaFile(XFile file, {bool insertAtFront = false}) async {
    // The new media becomes the background only when it ends up at index 0:
    // either explicitly prepended (background-replace flow) or appended to an
    // empty list. Carousel-add into a non-empty list appends to the end and
    // must leave the existing background untouched.
    final becomesPrimary = insertAtFront || state.allMediaItems.isEmpty;

    // Create optimistic placeholder
    final tempId = MediaUploadHelper.generateTempId();
    final placeholderItem = MediaUploadHelper.createPlaceholder(
      tempId: tempId,
      file: file,
    );

    // Insert placeholder at correct position
    final placeholderAllMedia = MediaUploadHelper.insertPlaceholder(
      placeholder: placeholderItem,
      currentItems: state.allMediaItems,
      insertAtFront: insertAtFront,
    );

    state = state.copyWith(
      isUploadingMedia: true,
      allMediaItems: placeholderAllMedia,
    );

    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);

      // Upload media to server
      final mediaId = await mediaRepository.addMedia(
        file: file,
        description: 'Gear media for ${state.gearDetails?.name ?? "gear"}',
      );

      // After upload, fetch the media to get the URL and content type
      final mediaResponse = await mediaRepository.getMedia(mediaId);
      final isVideoFromServer = mediaResponse.contentType.startsWith('video/');

      // Replace placeholder with real media item
      final newMediaItem = MediaItemData(
        id: mediaId,
        url: mediaResponse.url,
        contentType: mediaResponse.contentType,
        isVideo: isVideoFromServer,
      );

      // Remove placeholder and insert real item at same position
      // Note: Access state.allMediaItems before the async gap check since we need it for the map
      if (!ref.mounted) return;
      final updatedAllMedia = state.allMediaItems
          .map((item) => item.id == tempId ? newMediaItem : item)
          .toList();

      // Update state. When the new media is appended to a non-empty list, the
      // primary background stays as-is. The VideoBackgroundHost widget will
      // create the video controller on its own when it sees mediaPath + isVideo.
      safeUpdateState((s) => becomesPrimary
          ? s.copyWith(
              mediaPath: mediaResponse.url,
              mediaId: mediaId,
              isVideo: isVideoFromServer,
              allMediaItems: updatedAllMedia,
              isUploadingMedia: false,
            )
          : s.copyWith(
              allMediaItems: updatedAllMedia,
              isUploadingMedia: false,
            ));

      // Automatically save gear to persist the media ID to the server
      if (!ref.mounted) return;
      if (state.gearDetails != null) {
        await saveChanges(
          name: state.gearDetails!.name,
          description: state.gearDetails!.description,
        );
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Error in _setMediaFile: $e', e, stackTrace);
      // Remove placeholder on error - need to check mounted and get current state
      if (!ref.mounted) return;
      final mediaWithoutPlaceholder = MediaUploadHelper.removePlaceholder(
        tempId: tempId,
        currentItems: state.allMediaItems,
      );
      safeUpdateState((s) => s.copyWith(
        isUploadingMedia: false,
        allMediaItems: mediaWithoutPlaceholder,
      ));
      rethrow;
    }
  }

  /// Uploads multiple image files sequentially and persists with a single
  /// saveChanges() call after all uploads complete.
  Future<void> _setMultipleMediaFiles(
    List<XFile> files, {
    bool insertAtFront = false,
  }) async {
    // Create all optimistic placeholders up front so the UI shows them immediately.
    final tempIds = [for (final _ in files) MediaUploadHelper.generateTempId()];
    var allMedia = state.allMediaItems;
    for (var i = 0; i < files.length; i++) {
      allMedia = MediaUploadHelper.insertPlaceholder(
        placeholder: MediaUploadHelper.createPlaceholder(
          tempId: tempIds[i],
          file: files[i],
        ),
        currentItems: allMedia,
        insertAtFront: insertAtFront,
      );
    }
    state = state.copyWith(isUploadingMedia: true, allMediaItems: allMedia);

    final (:uploadedIds, :failedCount) = await MediaHelpers.batchUploadFiles(
      ref: ref,
      files: files,
      tempIds: tempIds,
      isMounted: () => ref.mounted,
      onSuccess: (tempId, item) => safeUpdateState(
        (s) => s.copyWith(
          allMediaItems:
              s.allMediaItems.map((m) => m.id == tempId ? item : m).toList(),
        ),
      ),
      onFailure: (tempId) => safeUpdateState(
        (s) => s.copyWith(
          allMediaItems: MediaUploadHelper.removePlaceholder(
            tempId: tempId,
            currentItems: s.allMediaItems,
          ),
        ),
      ),
      description: 'Gear media for ${state.gearDetails?.name ?? "gear"}',
      entityLabel: 'gear',
    );

    if (!ref.mounted) return;
    safeUpdateState((s) => s.copyWith(
      isUploadingMedia: false,
      batchUploadFailedCount: failedCount > 0 ? failedCount : null,
    ));

    if (uploadedIds.isNotEmpty && state.gearDetails != null) {
      await saveChanges(
        name: state.gearDetails!.name,
        description: state.gearDetails!.description,
      );
    }

    if (failedCount > 0) {
      _log.warning(
        '⚠️ $failedCount of ${files.length} gear media uploads failed',
      );
    }
  }

  /// Deletes a media item from the gear.
  Future<void> deleteMedia(String mediaId) async {
    try {
      // Remove media from local state first for immediate UI update
      final updatedAllMedia = state.allMediaItems
          .where((item) => item.id != mediaId)
          .toList();

      state = state.copyWith(allMediaItems: updatedAllMedia);

      // If the deleted media was the currently displayed one, switch to first available
      if (state.mediaId == mediaId && updatedAllMedia.isNotEmpty) {
        final firstMedia = updatedAllMedia.first;
        state = state.copyWith(
          mediaId: firstMedia.id,
          mediaPath: firstMedia.url,
          isVideo: firstMedia.isVideo,
        );
      } else if (updatedAllMedia.isEmpty) {
        // No media left
        state = state.copyWith(mediaId: null, mediaPath: null, isVideo: false);
      }

      // Delete media from server
      final mediaRepository = ref.read(mediaRepositoryProvider);
      await mediaRepository.deleteMedia(mediaId);

      // Update gear on server to remove media ID from list
      if (!ref.mounted) return;
      if (state.gearDetails != null) {
        await saveChanges(
          name: state.gearDetails!.name,
          description: state.gearDetails!.description,
        );
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting media: $e', e, stackTrace);
      // Reload gear to restore correct state
      await loadGearDetails();
      rethrow;
    }
  }

  /// Reorders media for the gear.
  Future<void> reorderMedia(List<String> mediaIds) async {
    if (state.gearDetails == null) return;

    try {
      // Update media order on server
      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.updateMediaOrder(state.gearDetails!.id, mediaIds);

      // Reload to get updated state
      await loadGearDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error reordering media: $e', e, stackTrace);
      // Reload gear to restore correct state
      await loadGearDetails();
      rethrow;
    }
  }

  /// Expresses interest in borrowing/claiming the gear.
  /// Creates a transfer request and joins the gear conversation.
  Future<void> expressInterest() async {
    final gearId = state.gearId;
    if (gearId == null) return;
    if (state.isSaving) return;

    state = state.copyWith(isSaving: true, error: null);

    final Transfer newTransfer;
    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      newTransfer = await transferRepository.expressInterest(gearId: gearId);
    } catch (e, stackTrace) {
      _log.severe('❌ Error expressing interest: $e', e, stackTrace);
      safeUpdateState(
        (s) => s.copyWith(isSaving: false, error: RpcErrorHandler.classify(e)),
      );
      return;
    }

    if (!ref.mounted) return;

    // Use the server-authoritative community_id from the mutation response, not
    // state.communityId. For gear shared into multiple communities the latter
    // can point at a community the user isn't a member of, which makes the
    // refresh call fail with permission_denied (#1705).
    GearTransferContext? refreshed;
    try {
      refreshed = await ref
          .read(transferRepositoryProvider)
          .getGearTransferContext(
            gearId: gearId,
            communityId: newTransfer.communityId,
          );
    } catch (e, stackTrace) {
      _log.warning(
        'Refreshing transfer context after expressInterest failed; '
        'falling back to optimistic update from mutation response',
        e,
        stackTrace,
      );
    }

    if (!ref.mounted) return;
    safeUpdateState((s) {
      // Build the next context. If refresh succeeded, use it. Otherwise,
      // optimistically reflect the new transfer in the prior context so the
      // UI flips to "in line" instead of inviting another tap (#1684, #1685).
      final next = refreshed ??
          ((s.transferContext?.deepCopy() ?? GearTransferContext())
            ..userTransfer = newTransfer);
      return s.copyWith(transferContext: next, isSaving: false);
    });

    // Refresh stats (interest_count) and people (interested_parties).
    if (ref.mounted) unawaited(_invalidateAndReloadStatsAndPeople());
  }

  /// Withdraws interest in borrowing/claiming the gear.
  /// Cancels the transfer request.
  Future<void> withdrawInterest() async {
    final context = state.transferContext;
    if (context == null || !context.hasUserTransfer()) return;
    if (state.isSaving) return;

    final transfer = context.userTransfer;
    final transferId = transfer.id;
    // Use the transfer's own community_id (server-authoritative) instead of
    // state.communityId. The transfer was created in a community the user is
    // a member of; state.communityId may not match for multi-community gear
    // (#1705).
    final transferCommunityId = transfer.communityId;

    state = state.copyWith(isSaving: true, error: null);

    try {
      await ref
          .read(transferRepositoryProvider)
          .withdrawInterest(transferId: transferId);
    } catch (e, stackTrace) {
      _log.severe('❌ Error withdrawing interest: $e', e, stackTrace);
      safeUpdateState(
        (s) => s.copyWith(isSaving: false, error: RpcErrorHandler.classify(e)),
      );
      return;
    }

    if (!ref.mounted) return;

    GearTransferContext? refreshed;
    final gearId = state.gearId;
    if (gearId != null && transferCommunityId.isNotEmpty) {
      try {
        refreshed = await ref
            .read(transferRepositoryProvider)
            .getGearTransferContext(
              gearId: gearId,
              communityId: transferCommunityId,
            );
      } catch (e, stackTrace) {
        _log.warning(
          'Refreshing transfer context after withdrawInterest failed; '
          'falling back to optimistic clear of userTransfer',
          e,
          stackTrace,
        );
      }
    }

    if (!ref.mounted) return;
    safeUpdateState((s) {
      // If refresh succeeded, use it. Otherwise, optimistically clear the
      // user's transfer from the prior context so the UI flips back to
      // "Borrow" — server already confirmed the withdraw (#1705).
      final next = refreshed ??
          (s.transferContext?.deepCopy()?..clearUserTransfer());
      return s.copyWith(transferContext: next, isSaving: false);
    });

    // Refresh stats (interest_count) and people (interested_parties).
    if (ref.mounted) unawaited(_invalidateAndReloadStatsAndPeople());
  }

  // Tracks whether a gear conversation creation is already in flight so
  // rapid duplicate calls from the widget layer do not double-create.
  bool _conversationCreateInFlight = false;

  /// ensureGearConversation creates or retrieves the perpetual gear conversation
  /// when the chat tab is opened and the gear's conversation_id is absent.
  ///
  /// No-op if a conversation_id is already present or a create call is already
  /// in flight. On success, invalidates the gear cache and reloads gear details
  /// so the parent view picks up the new conversation_id without a content flicker.
  Future<void> ensureGearConversation() async {
    final gearId = state.gearId;
    // No-op: conversation already exists.
    if (state.gearDetails?.conversationId.isNotEmpty ?? false) return;
    // No-op: call already in flight.
    if (_conversationCreateInFlight) return;
    if (gearId == null) return;

    // Resolve which community_id to pass. Use the current community context if
    // the gear is shared there; otherwise fall back to the first shared community
    // the user belongs to (per the second Open Question in the plan).
    final communityId = _resolveChatCommunityId();
    if (communityId == null) return; // Gear not shared anywhere — show unshared state.

    _conversationCreateInFlight = true;
    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      );
      if (!ref.mounted) return;

      // Invalidate the gear cache so the next read sees the persisted conversation_id.
      final gearRepository = ref.read(gearRepositoryProvider);
      await gearRepository.invalidate(gearId, communityId: state.communityId);
      if (!ref.mounted) return;

      await loadGearDetails(communityId: state.communityId);
    } catch (e) {
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(error: RpcErrorHandler.classify(e)));
    } finally {
      _conversationCreateInFlight = false;
    }
  }

  /// _resolveChatCommunityId picks the community to use when calling
  /// startGearConversation. Returns null if the gear is not shared anywhere.
  String? _resolveChatCommunityId() {
    final sharedCommunities = state.sharedCommunities;
    if (sharedCommunities.isEmpty) return null;

    // Prefer the current community context if the gear is shared there.
    final current = state.communityId;
    if (current != null &&
        sharedCommunities.any((c) => c.communityId == current)) {
      return current;
    }

    // Otherwise use the first shared community (all are accessible to the user
    // because sharedCommunities is already filtered to the user's memberships
    // by the server's GetGear response).
    return sharedCommunities.first.communityId;
  }
}

/// Provider for gear state
/// Uses .family to create separate instances per gearId
/// Uses .autoDispose to clean up state when widget is disposed
final gearProvider = NotifierProvider.autoDispose
    .family<GearNotifier, GearState, String>(GearNotifier.new);
