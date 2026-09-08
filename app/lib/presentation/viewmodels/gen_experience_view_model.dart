import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_upload_mixin.dart';
import 'package:ripls/core/utils/user_location_initializer.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/core/utils/video_mute_helper.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_types.dart';
import 'package:ripls/services/community_service.dart'
    show HostInviteConsent, Invitee, ShareItemResult;
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

export 'package:ripls/presentation/viewmodels/gen_experience_types.dart';

part 'gen_experience_view_model.freezed.dart';

final _log = Logger('GenExperienceViewModel');

/// Workflow steps for AI-assisted experience creation
enum GenExperienceStep {
  input, // User provides text prompt OR uploads image
  uploading, // (Image mode only) Uploading image
  generating, // AI generating content
  preview, // User previews and edits AI suggestions
  creating, // Saving and sharing experience
  completed,
}

/// Input mode for experience generation
enum GenExperienceInputMode {
  text, // Text prompt mode
  image, // Media/flyer mode
  url, // Website URL mode
}

/// State for the Gen Experience workflow
@freezed
sealed class GenExperienceState with _$GenExperienceState {
  const factory GenExperienceState({
    @Default(GenExperienceStep.input) GenExperienceStep currentStep,
    @Default([]) List<GenExperienceStep> completedSteps,
    @Default(GenExperienceInputMode.text) GenExperienceInputMode inputMode,
    String? textPrompt,
    String? selectedImagePath,
    String? uploadedMediaId,
    String? urlInput,
    String? aiGeneratedName,
    String? aiGeneratedDescription,
    String? editedName,
    String? editedDescription,
    List<String>? selectedMediaIds,
    String? selectedLocationId,
    ExperienceTime? selectedTime,
    int? maxParticipants,
    @Default(false) bool isLoading,
    UserError? error,
    GenExperienceStep? errorStep,
    String? createdExperienceId,
    // AI-extracted time and location fields
    Int64?
    extractedTimeUnixSec, // Unix timestamp from AI (null if not extracted)
    TimeConfidence? timeConfidence, // AI confidence in time extraction
    String? locationQuery, // AI-extracted location string
    GeocodedLocation?
    geocodedLocation, // Geocoded data from server (cached for storage on confirm)
    String? sourceUrl, // Source URL for experiences created from website URLs
    ExperienceMetadata?
    metadata, // AI-generated metadata (value estimate, category)
    @Default(false) bool isUploadingMedia,
    UserError? uploadError,
    // Batch upload progress: completed count and total count.
    int? batchUploadCompleted,
    int? batchUploadTotal,
    int? batchUploadFailedCount,
    // Video preview support: set when the AI-generated media is a video
    VideoPlayerController? previewVideoController,
    @Default(true) bool previewIsMuted,
    // Participant tagging: names extracted by AI and user-selected participant IDs
    @Default([]) List<String> mentionedNames,
    @Default([]) List<String> taggedParticipantIds,
  }) = _GenExperienceState;

  const GenExperienceState._();

  /// Check if the URL input is valid (basic HTTP/HTTPS check).
  bool get isValidUrl {
    if (urlInput == null || urlInput!.isEmpty) return false;
    final uri = Uri.tryParse(urlInput!);
    if (uri == null || !uri.hasScheme || !uri.hasAuthority) return false;
    return uri.scheme == 'http' || uri.scheme == 'https';
  }

  /// Check if we can proceed from the current step
  bool get canProceed {
    switch (currentStep) {
      case GenExperienceStep.input:
        return (inputMode == GenExperienceInputMode.text &&
                textPrompt != null &&
                textPrompt!.isNotEmpty) ||
            (inputMode == GenExperienceInputMode.image &&
                selectedImagePath != null) ||
            (inputMode == GenExperienceInputMode.url && isValidUrl);
      case GenExperienceStep.uploading:
        return false; // Auto-progresses when upload completes
      case GenExperienceStep.generating:
        return false; // Auto-progresses when generation completes
      case GenExperienceStep.preview:
        return (editedName ?? aiGeneratedName) != null &&
            (editedDescription ?? aiGeneratedDescription) != null;
      case GenExperienceStep.creating:
        return false; // Auto-progresses when creation completes
      case GenExperienceStep.completed:
        return false; // Final step
    }
  }

  /// Calculate progress percentage (0.0 to 1.0)
  double get progress =>
      completedSteps.length / GenExperienceStep.values.length.toDouble();

  /// Check if workflow has completed
  bool get isCompleted => currentStep == GenExperienceStep.completed;

  /// Check if there's an error
  bool get hasError => error != null;
}

/// Notifier for managing the Gen Experience workflow
class GenExperienceNotifier extends Notifier<GenExperienceState>
    with MediaUploadMixin<GenExperienceState> {
  VideoPlayerController? _previewVideoController;

  @override
  GenExperienceState build() {
    ref.onDispose(() {
      _previewVideoController?.dispose();
    });
    return const GenExperienceState();
  }

  ExperienceRepository get _experienceRepository =>
      ref.read(experienceRepositoryProvider);

  MediaRepository get _mediaRepository => ref.read(mediaRepositoryProvider);

  // MediaUploadMixin implementation
  @override
  MediaRepository get mediaRepository => _mediaRepository;

  @override
  Logger get log => _log;

  @override
  String get mediaUploadDescription => 'Experience media';

  @override
  void onMediaUploadStarted() {
    state = state.copyWith(isUploadingMedia: true, uploadError: null);
  }

  @override
  void onMediaUploaded(String mediaId) {
    _replacePreviewMedia([mediaId]);
    state = state.copyWith(isUploadingMedia: false);
  }

  @override
  void onMediaUploadFailed(String error) {
    state = state.copyWith(
      uploadError: UserError.generic(fallback: error),
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadProgress(int completed, int total) {
    state = state.copyWith(
      batchUploadCompleted: completed,
      batchUploadTotal: total,
    );
  }

  @override
  void onBatchUploadCompleted(List<String> mediaIds) {
    _replacePreviewMedia(mediaIds);
    state = state.copyWith(
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount) {
    _replacePreviewMedia(uploadedIds);
    state = state.copyWith(
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: failedCount,
    );
  }

  /// Replaces the current preview media, disposing any stale video controller
  /// and updating state atomically so no rebuild sees an old controller with a new id.
  void _replacePreviewMedia(List<String> mediaIds) {
    _previewVideoController?.dispose();
    _previewVideoController = null;
    state = state.copyWith(
      selectedMediaIds: mediaIds,
      previewVideoController: null,
      previewIsMuted: true,
    );
    if (mediaIds.isNotEmpty) unawaited(_loadPreviewVideoIfNeeded(mediaIds));
  }

  /// Set the input mode (text, image, or url)
  void setInputMode(GenExperienceInputMode mode) {
    state = state.copyWith(
      inputMode: mode,
      textPrompt: null,
      selectedImagePath: null,
      urlInput: null,
      error: null,
      errorStep: null,
    );
  }

  /// Set the text prompt for text mode
  void setTextPrompt(String prompt) {
    state = state.copyWith(textPrompt: prompt, error: null, errorStep: null);
  }

  /// Set the selected image path for image mode
  void setImagePath(String path) {
    state = state.copyWith(
      selectedImagePath: path,
      error: null,
      errorStep: null,
    );
  }

  /// Set the URL input for URL mode
  void setUrlInput(String url) {
    state = state.copyWith(urlInput: url, error: null, errorStep: null);
  }

  /// Clear the URL input
  void clearUrl() {
    state = state.copyWith(urlInput: null, error: null, errorStep: null);
  }

  /// Clear the selected image
  void clearImage() {
    state = state.copyWith(
      selectedImagePath: null,
      uploadedMediaId: null,
      currentStep: GenExperienceStep.input,
      completedSteps: [],
      error: null,
      errorStep: null,
    );
  }

  /// Loads a VideoPlayerController for the preview if the first generated media is a video.
  /// Called after AI generation completes and selectedMediaIds is populated.
  Future<void> _loadPreviewVideoIfNeeded(List<String> mediaIds) async {
    if (mediaIds.isEmpty) return;

    try {
      final mediaUrl = await _mediaRepository.getFullMediaUrl(mediaIds[0]);

      if (!ref.mounted) return;

      if (mediaUrl.contentType?.startsWith('video/') != true) return;

      // Dispose any existing preview controller
      unawaited(_previewVideoController?.dispose());
      _previewVideoController = null;

      final controller = await createCachedVideoController(
        _mediaRepository,
        mediaIds[0],
        mediaUrl.url,
      );
      _previewVideoController = controller;

      if (!ref.mounted) {
        unawaited(controller.dispose());
        _previewVideoController = null;
        return;
      }

      if (!ref.mounted) return;

      state = state.copyWith(
        previewVideoController: controller,
        previewIsMuted: true,
      );
    } catch (e) {
      _log.warning('Failed to load preview video: $e');
      // Non-fatal — preview just shows static image fallback
    }
  }

  /// Toggles the mute state of the preview video.
  void togglePreviewMute() {
    toggleVideoMute(
      controller: _previewVideoController,
      currentlyMuted: state.previewIsMuted,
      updateState: (muted) => state = state.copyWith(previewIsMuted: muted),
    );
  }

  /// Update the edited name (overrides AI suggestion)
  void updateName(String name) {
    state = state.copyWith(editedName: name, error: null, errorStep: null);
  }

  /// Update the edited description (overrides AI suggestion)
  void updateDescription(String description) {
    state = state.copyWith(
      editedDescription: description,
      error: null,
      errorStep: null,
    );
  }

  /// Update the selected media IDs
  void updateMedia(List<String> mediaIds) {
    state = state.copyWith(selectedMediaIds: mediaIds);
  }

  /// Update the selected location ID
  void updateLocation(String locationId) {
    state = state.copyWith(selectedLocationId: locationId);
  }

  /// Update the experience time (user manual override — clears AI suggestion tracking).
  void updateTime(ExperienceTime time) {
    state = state.copyWith(
      selectedTime: time,
      // Null out extractedTimeUnixSec so that the "Suggested from description"
      // label disappears once the user manually picks a different time.
      extractedTimeUnixSec: null,
    );
  }

  
  
  /// Tag a community member as a participant in this experience.
  void tagParticipant(String userId) {
    if (state.taggedParticipantIds.contains(userId)) return;
    state = state.copyWith(
      taggedParticipantIds: [...state.taggedParticipantIds, userId],
    );
  }

  /// Remove a community member from the tagged participants list.
  void untagParticipant(String userId) {
    state = state.copyWith(
      taggedParticipantIds: state.taggedParticipantIds
          .where((id) => id != userId)
          .toList(),
    );
  }

  /// Initialize preview state with AI-generated results from GenExperienceResponse.
  ///
  /// This is used by the new experience creation modal to hand off AI-generated
  /// content to the existing preview workflow.
  void initializeFromGenResponse({
    required String name,
    required String description,
    required List<String> mediaIds,
    required ExperienceTime? suggestedTime,
    required Int64? extractedTimeUnixSec,
    required TimeConfidence? timeConfidence,
    required String? locationQuery,
    required GeocodedLocation? geocodedLocation,
    String? sourceUrl,
    ExperienceMetadata? metadata,
    List<String> mentionedNames = const [],
  }) {
    _log.info('📝 initializeFromGenResponse called:');
    _log.info('  name: "$name"');
    _log.info('  description: "$description"');
    _log.info('  mediaIds: $mediaIds');
    _log.info('  suggestedTime: $suggestedTime');
    _log.info('  extractedTimeUnixSec: $extractedTimeUnixSec');
    _log.info('  timeConfidence: $timeConfidence');
    _log.info('  locationQuery: $locationQuery');
    _log.info('  geocodedLocation: ${geocodedLocation?.name}');
    _log.info('  sourceUrl: $sourceUrl');

    state = state.copyWith(
      aiGeneratedName: name,
      aiGeneratedDescription: description,
      selectedMediaIds: mediaIds,
      selectedTime: suggestedTime,
      extractedTimeUnixSec:
          extractedTimeUnixSec != null && extractedTimeUnixSec > Int64.ZERO
          ? extractedTimeUnixSec
          : null,
      timeConfidence: timeConfidence,
      locationQuery: locationQuery != null && locationQuery.isNotEmpty
          ? locationQuery
          : null,
      geocodedLocation: geocodedLocation,
      sourceUrl: sourceUrl != null && sourceUrl.isNotEmpty ? sourceUrl : null,
      metadata: metadata,
      mentionedNames: mentionedNames,
      taggedParticipantIds: [],
      currentStep: GenExperienceStep.preview,
      // Clear edited state to prevent pollution from previous sessions
      editedName: null,
      editedDescription: null,
    );

    _log.info('✅ State updated after initializeFromGenResponse:');
    _log.info('  aiGeneratedName: "${state.aiGeneratedName}"');
    _log.info('  aiGeneratedDescription: "${state.aiGeneratedDescription}"');
    _log.info('  selectedMediaIds: ${state.selectedMediaIds}');
    _log.info('  currentStep: ${state.currentStep}');

    // Fire and forget: loads VideoPlayerController if the first media is a video.
    // _loadPreviewVideoIfNeeded handles ref.mounted checks internally.
    if (mediaIds.isNotEmpty) {
      unawaited(_loadPreviewVideoIfNeeded(mediaIds));
    }
  }

  // Media upload methods provided by MediaUploadMixin:
  // - pickAndUploadImageFromGallery()
  // - pickAndUploadVideoFromGallery()
  // - pickAndUploadImageFromCamera()

  /// Gets the media URL for a given media ID.
  ///
  /// The repository handles caching, so this is just a thin wrapper.
  /// Returns the MediaUrl object containing both URL and cache key.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);

  /// Get user-friendly display of extracted time with confidence indicator.
  ///
  /// Returns a formatted string showing the time and a visual indicator:
  /// - "✓ Tomorrow at 2:00 PM" (EXPLICIT confidence)
  /// - "~ Tomorrow afternoon" (INFERRED confidence with fuzzy time)
  /// - null (UNKNOWN or no time extracted)
  String? getExtractedTimeDisplay() {
    if (state.extractedTimeUnixSec == null ||
        state.extractedTimeUnixSec!.toInt() == 0) {
      return null;
    }

    final confidence = state.timeConfidence;
    if (confidence == TimeConfidence.TIME_CONFIDENCE_UNKNOWN ||
        confidence == TimeConfidence.TIME_CONFIDENCE_UNSPECIFIED) {
      return null;
    }

    final extractedDateTime = DateTime.fromMillisecondsSinceEpoch(
      state.extractedTimeUnixSec!.toInt() * 1000,
    );

    final prefix = confidence == TimeConfidence.TIME_CONFIDENCE_EXPLICIT
        ? '✓'
        : '~';
    final dateStr = _formatDate(extractedDateTime);
    final timeStr = _formatTime(extractedDateTime);

    return '$prefix $dateStr at $timeStr';
  }

  /// Check if the AI-extracted date is in the past.
  ///
  /// Returns true if extractedTimeUnixSec is set and represents a date before today.
  bool isExtractedDateInPast() {
    if (state.extractedTimeUnixSec == null ||
        state.extractedTimeUnixSec!.toInt() == 0) {
      return false;
    }

    final extractedDateTime = DateTime.fromMillisecondsSinceEpoch(
      state.extractedTimeUnixSec!.toInt() * 1000,
    );
    final today = DateTime.now();
    final todayDate = DateTime(today.year, today.month, today.day);

    return extractedDateTime.isBefore(todayDate);
  }

  /// Initialize selectedTime from the AI-extracted date/time, if any.
  ///
  /// The stored timezone must be IANA (e.g. "America/Chicago") — the server's
  /// `time.LoadLocation` only parses IANA names. [DateTime.now().timeZoneName]
  /// returns an OS abbreviation on Android ("CDT") that fails to parse and
  /// silently falls back to UTC (issue #2599), so resolve via
  /// [resolvedTimezoneProvider].
  Future<void> initializeTimeFromExtracted() async {
    if (state.extractedTimeUnixSec == null ||
        state.extractedTimeUnixSec!.toInt() == 0) {
      // No extracted time, keep default TBD
      return;
    }

    try {
      final ianaTimezone = await ref.read(resolvedTimezoneProvider.future);
      if (!ref.mounted) return;

      // Create SpecificTime from Unix timestamp
      final specificTime = SpecificTime()
        ..unixTimestampSec = state.extractedTimeUnixSec!
        ..timezone = ianaTimezone
        ..durationMinutes = 60; // Default 1 hour duration

      final experienceTime = ExperienceTime()..specific = specificTime;

      state = state.copyWith(selectedTime: experienceTime);

      final dateTime = DateTime.fromMillisecondsSinceEpoch(
        state.extractedTimeUnixSec!.toInt() * 1000,
      );
      _log.info('✅ Initialized time from extraction: $dateTime');
    } catch (e) {
      _log.warning('Failed to initialize time from extracted values: $e');
      // Keep default TBD time
    }
  }

  /// Format DateTime to user-friendly display.
  ///
  /// Examples:
  /// - Today → "Today"
  /// - Tomorrow → "Tomorrow"
  /// - 2025-12-01 → "Dec 1"
  String _formatDate(DateTime date) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    // Use calendar arithmetic instead of Duration(days:1) to avoid DST skew —
    // adding 86400s on a spring-forward day lands at 01:00, not midnight.
    final tomorrow = DateTime(now.year, now.month, now.day + 1);
    final dateOnly = DateTime(date.year, date.month, date.day);

    if (dateOnly == today) {
      return 'Today';
    } else if (dateOnly == tomorrow) {
      return 'Tomorrow';
    } else {
      final months = [
        'Jan',
        'Feb',
        'Mar',
        'Apr',
        'May',
        'Jun',
        'Jul',
        'Aug',
        'Sep',
        'Oct',
        'Nov',
        'Dec',
      ];
      return '${months[date.month - 1]} ${date.day}';
    }
  }

  /// Format DateTime to 12-hour format.
  ///
  /// Examples:
  /// - 08:00 → "8:00 AM"
  /// - 14:30 → "2:30 PM"
  String _formatTime(DateTime dateTime) {
    final hour = dateTime.hour;
    final minute = dateTime.minute;

    final period = hour >= 12 ? 'PM' : 'AM';
    final hour12 = hour == 0 ? 12 : (hour > 12 ? hour - 12 : hour);
    final minuteStr = minute.toString().padLeft(2, '0');

    return '$hour12:$minuteStr $period';
  }

  /// Initialize default location using shared utility.
  ///
  /// Delegates to [UserLocationInitializer.getPrimaryLocation] which handles:
  /// - Fetching user's primary residence location if set
  /// - Detecting device location if no primary residence exists
  /// - Reverse geocoding to get address details
  /// - Saving the location and setting it as primary residence
  ///
  /// Returns the location ID or null if location cannot be determined.
  Future<String?> _initializeDefaultLocation() async {
    return UserLocationInitializer.getPrimaryLocation(ref);
  }

  /// Create and store Location from cached geocoded data.
  ///
  /// This is called when the user confirms the experience, creating a Location
  /// record from the geocoded data cached during preview generation (Option C).
  ///
  /// Returns the created location ID, or null if creation fails or no data available.
  Future<String?> _createLocationFromGeocoded() async {
    final geocoded = state.geocodedLocation;
    if (geocoded == null) {
      _log.warning('No geocoded data available, cannot create location');
      return null;
    }
    // Coordinates are the only field required to save a location. A
    // POI-only Mapbox match (e.g. a park) can carry empty name/postal/
    // locality and the AI preview displays it via address lines — refusing
    // to save in that case would silently drop the predicted spot and the
    // shared experience would land as TBD.
    if (geocoded.latitudeDeg == 0 && geocoded.longitudeDeg == 0) {
      _log.warning('Geocoded data has no coordinates, cannot create location');
      return null;
    }

    try {
      // Create location via repository using flattened SaveLocationRequest fields
      final locationRepository = ref.read(locationRepositoryProvider);
      final locationId = await locationRepository.saveLocation(
        latitudeDeg: geocoded.latitudeDeg,
        longitudeDeg: geocoded.longitudeDeg,
        regionCode: geocoded.regionCode,
        postalCode: geocoded.postalCode,
        locality: geocoded.locality,
        addressLines: geocoded.addressLines.isNotEmpty
            ? geocoded.addressLines
            : null,
        name: geocoded.name,
        neighborhood: geocoded.neighborhood.isNotEmpty
            ? geocoded.neighborhood
            : null,
        county: geocoded.county.isNotEmpty ? geocoded.county : null,
        administrativeArea: geocoded.administrativeArea.isNotEmpty
            ? geocoded.administrativeArea
            : null,
        externalPlaceId: geocoded.externalPlaceId.isNotEmpty
            ? geocoded.externalPlaceId
            : null,
        externalPlaceProvider: geocoded.externalPlaceProvider.isNotEmpty
            ? geocoded.externalPlaceProvider
            : null,
      );

      _log.info(
        'Created location from cached geocoded data: $locationId (${geocoded.name})',
      );

      return locationId;
    } catch (e) {
      _log.severe('Failed to create location from geocoded data: $e');
      return null;
    }
  }

  /// Create and share experience with the specified communities.
  /// Creates the experience, then shares it via CommunityService.ShareItem:
  /// to each community in [communityIds] (additive) and/or to the [invitees]
  /// (members, phone, email) as an ad-hoc audience. [consent] is required when
  /// any phone invitee is present. Returns the share result (carrying the
  /// ad-hoc community id + open link for host-relay), or null on failure.
  Future<ShareItemResult?> createAndShareExperience(
    List<String> communityIds, {
    List<Invitee> invitees = const [],
    HostInviteConsent? consent,
  }) async {
    // Get name and description - either edited values or AI-generated
    final name = state.editedName ?? state.aiGeneratedName;
    final description = state.editedDescription ?? state.aiGeneratedDescription;

    // Validate required fields
    if (name == null || name.isEmpty) {
      _setError('Experience name is required', GenExperienceStep.preview);
      return null;
    }

    if (description == null || description.isEmpty) {
      _setError(
        'Experience description is required',
        GenExperienceStep.preview,
      );
      return null;
    }

    // The ad-hoc audience from invitees satisfies the "at least one audience"
    // rule, so an individuals-only event needs no pre-selected community.
    if (communityIds.isEmpty && invitees.isEmpty) {
      _setError('Select a community or invite someone', GenExperienceStep.preview);
      return null;
    }

    // Check if still mounted before starting async operations
    if (!ref.mounted) return null;

    state = state.copyWith(
      isLoading: true,
      currentStep: GenExperienceStep.creating,
      error: null,
      errorStep: null,
    );

    try {
      // Option C: Create location from cached geocoded data if available.
      // Treat empty selectedLocationId the same as null — workshop-draft
      // seeding and other entry points can leave it as the empty string,
      // and an empty string here would skip the geocoded path AND skip
      // the primary-residence fallback below, landing the experience as
      // TBD even though the AI predicted a spot.
      final selected = state.selectedLocationId;
      String? finalLocationId = (selected == null || selected.isEmpty)
          ? null
          : selected;

      if (finalLocationId == null && state.geocodedLocation != null) {
        finalLocationId = await _createLocationFromGeocoded();
        if (finalLocationId != null && ref.mounted) {
          // Persist back to state so a retry after a transient downstream
          // failure doesn't re-save the same location.
          state = state.copyWith(selectedLocationId: finalLocationId);
        }
      }

      // Fall back to default location if geocoding failed or location creation failed
      finalLocationId ??= await _initializeDefaultLocation();

      // Step 1: Create experience
      final experienceId = await _experienceRepository.createExperience(
        name: name,
        description: description,
        mediaIds: state.selectedMediaIds,
        locationId: finalLocationId,
        time: state.selectedTime,
        maxParticipants: state.maxParticipants,
        sourceUrl: state.sourceUrl,
        metadata: state.metadata,
      );

      // Check if still mounted after async gap
      if (!ref.mounted) return null;

      // Step 2: Share with selected communities and/or invite individuals in a
      // single call. When invitees are present this provisions the ad-hoc
      // audience community and returns its open link for host-relay.
      final shareResult = await ref
          .read(communityRepositoryProvider)
          .shareItem(
            experienceId: experienceId,
            invitees: invitees,
            consent: consent,
            shareToCommunityIds: communityIds,
          );

      // Log analytics event
      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            ExperienceCreatedEvent(
              communityId: communityIds.isNotEmpty
                  ? communityIds.first
                  : (shareResult.adhocCommunityId ?? ''),
            ),
          ));

      // Check if still mounted before updating state
      if (!ref.mounted) return null;

      state = state.copyWith(
        isLoading: false,
        createdExperienceId: experienceId,
        completedSteps: [...state.completedSteps, GenExperienceStep.creating],
        currentStep: GenExperienceStep.completed,
      );
      return shareResult;
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to create and share experience: $e', e, stackTrace);

      // Check if still mounted before setting error state
      if (!ref.mounted) return null;

      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(
          e,
          fallback: 'Could not create the event',
        ),
        errorStep: GenExperienceStep.creating,
        currentStep: GenExperienceStep.preview,
      );
      return null;
    }
  }

  /// Resolves AI-mentioned names against community members and provisional users.
  ///
  /// For each name in [mentionedNames]:
  /// - If one member matches: auto-accept (add to [ResolvedParticipants.userIds]).
  /// - If multiple members match: add all as suggestions (caller deduplicates via UI).
  /// - If zero members match: create a provisional user (add to [ResolvedParticipants.provisionalUsers]).
  ///
  /// Returns [null] if no community is selected.
  Future<ResolvedParticipants?> resolveParticipantsForCompletion() async {
    // TODO(#2040): selectedCommunity was the per-community filter removed in
    // #1895 and now always null, so this method always returns null. Plumb a
    // community ID through from the experience-creation flow when restoring
    // the post-create completion path.
    return null;
  }

  /// Reset the workflow to start
  void reset() {
    _log.info('🔄 Resetting workflow');
    state = const GenExperienceState();
  }

  /// Seed the workflow with a Workshop-generated draft so the host lands
  /// directly in the preview step with prefilled fields. Mirrors the
  /// final-state of a successful `generateExperience` call but skips the
  /// AI round-trip — fields come from the prior instance via
  /// `WorkshopRepository.generateDraft`.
  ///
  /// Call before pushing `ExperiencePreviewModal` from a Workshop CTA.
  void seedFromWorkshopDraft({
    required String name,
    required String description,
    int? timeUnixSec,
    String? locationId,
    List<String>? mediaIds,
    List<String>? participantIds,
  }) {
    _log.info('🪡 seeding workflow from Workshop draft (name=$name)');
    state = state.copyWith(
      currentStep: GenExperienceStep.preview,
      aiGeneratedName: name,
      aiGeneratedDescription: description,
      selectedMediaIds: mediaIds,
      selectedLocationId: locationId,
      extractedTimeUnixSec: timeUnixSec != null ? Int64(timeUnixSec) : null,
      taggedParticipantIds: participantIds ?? const [],
      isLoading: false,
      error: null,
      errorStep: null,
    );
  }

  /// Marks the workflow as actively streaming AI output. Called once when the
  /// streaming generation starts so the preview modal can show skeleton state
  /// while partial fields populate.
  void beginStreaming() {
    state = state.copyWith(
      currentStep: GenExperienceStep.generating,
      error: null,
      errorStep: null,
      aiGeneratedName: null,
      aiGeneratedDescription: null,
      selectedMediaIds: null,
      geocodedLocation: null,
      locationQuery: null,
      sourceUrl: null,
      metadata: null,
      extractedTimeUnixSec: null,
      timeConfidence: null,
      editedName: null,
      editedDescription: null,
    );
  }

  /// Applies an early `title` event from the streaming generator. Leaves the
  /// step at `generating` so the preview modal continues showing a skeleton
  /// for not-yet-arrived fields.
  void applyStreamingTitle(String title) {
    if (title.isEmpty) return;
    state = state.copyWith(aiGeneratedName: title);
  }

  /// Applies an early `description` event from the streaming generator. The
  /// server emits this immediately at stream open (description = prompt
  /// verbatim in text mode) so the preview modal can render it before the AI
  /// returns its first token.
  void applyStreamingDescription(String description) {
    if (description.isEmpty) return;
    state = state.copyWith(aiGeneratedDescription: description);
  }

  /// Applies a mid-stream `time` event carrying the parsed time extraction
  /// (suggested_time, confidence, extracted_time_unix_sec).
  void applyStreamingTime({
    ExperienceTime? suggestedTime,
    TimeConfidence? confidence,
    Int64? extractedTimeUnixSec,
  }) {
    state = state.copyWith(
      selectedTime: suggestedTime,
      timeConfidence: confidence,
      extractedTimeUnixSec:
          extractedTimeUnixSec != null && extractedTimeUnixSec > Int64.ZERO
          ? extractedTimeUnixSec
          : null,
    );
  }

  /// Applies an early `geocoded` event from the streaming generator.
  void applyStreamingGeocoded(GeocodedLocation geocoded) {
    state = state.copyWith(
      geocodedLocation: geocoded,
      locationQuery: geocoded.name.isNotEmpty ? geocoded.name : null,
    );
  }

  /// Applies an early `media_ready` event from the streaming generator.
  void applyStreamingMediaReady(List<String> mediaIds) {
    if (mediaIds.isEmpty) return;
    state = state.copyWith(selectedMediaIds: mediaIds);
  }

  /// Applies the terminal `final` event from the streaming generator.
  /// Delegates to [initializeFromGenResponse] so the normal preview flow picks
  /// up where streaming left off.
  void finalizeStreaming(GenExperienceResponse response) {
    initializeFromGenResponse(
      name: response.name,
      description: response.description,
      mediaIds: response.mediaIds,
      suggestedTime: response.hasSuggestedTime()
          ? response.suggestedTime
          : null,
      extractedTimeUnixSec: response.hasExtractedTimeUnixSec()
          ? response.extractedTimeUnixSec
          : null,
      timeConfidence: response.hasTimeConfidence()
          ? response.timeConfidence
          : null,
      locationQuery: response.locationQuery.isNotEmpty
          ? response.locationQuery
          : null,
      geocodedLocation: response.hasGeocodedLocation()
          ? response.geocodedLocation
          : null,
      sourceUrl: response.sourceUrl.isNotEmpty ? response.sourceUrl : null,
      metadata: response.hasMetadata() ? response.metadata : null,
    );
  }

  /// Applies the terminal `error` event from the streaming generator.
  void failStreaming(String message) {
    _setError(message, GenExperienceStep.generating);
  }

  void _setError(String message, GenExperienceStep step) {
    _log.warning('⚠️ Error at step $step: $message');
    state = state.copyWith(
      error: UserError.generic(fallback: message),
      errorStep: step,
      currentStep: step,
    );
  }
}

/// Provider for the Gen Experience workflow
///
/// Note: This provider does NOT use autoDispose because state needs to persist
/// when transitioning from the creation modal to the preview modal (separate dialog contexts).
final genExperienceProvider =
    NotifierProvider<GenExperienceNotifier, GenExperienceState>(
      GenExperienceNotifier.new,
    );
