import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/core/utils/media_upload_mixin.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/request_service.dart' show RequestMetadata;

part 'gen_request_view_model.freezed.dart';

final _log = Logger('GenRequestViewModel');

/// Workflow steps for generating a request from a prompt
enum GenRequestStep { prompt, generating, preview, submitting, completed }

/// State for the Gen Request workflow with preview support
@freezed
sealed class GenRequestState with _$GenRequestState {
  const factory GenRequestState({
    @Default(GenRequestStep.prompt) GenRequestStep currentStep,
    @Default([]) List<GenRequestStep> completedSteps,
    String? generatedTitle,
    String? generatedDescription,
    String? generatedMediaId,
    String? generatedLocationId,
    GeocodedLocation? geocodedLocation,
    String? locationQuery,
    @Default([]) List<String> generatedTags,
    String? editedTitle,
    String? editedDescription,
    String? newMediaId,
    String? selectedLocationId,
    Location? resolvedLocation,
    /// Optional "needed by" date (Unix seconds) the user picked for the request.
    /// Places the request on the Home calendar/Up-next. Null when undated.
    int? neededByUnixSec,
    @Default(false) bool isLoading,
    UserError? error,
    GenRequestStep? errorStep,
    String? createdRequestId,
    @Default(false) bool isUploadingMedia,
    UserError? uploadError,
    // Batch upload progress: completed count and total count.
    int? batchUploadCompleted,
    int? batchUploadTotal,
    int? batchUploadFailedCount,
    RequestMetadata? metadata,
    // AI-generated suggestion chips for the compose stage. Populated by
    // the streaming gen flow's final event; consumed by the compose
    // sheet to seed the "tap to add" chip row. Empty when the server
    // chip call failed (best-effort — never blocks gen).
    @Default([]) List<String> additionalAsks,
    @Default([]) List<String> breakdownPieces,
    @Default([]) List<String> offerIdeas,
    // The AI-extracted claimable things the request's text plainly names
    // ("Lawn mower"; or "Picture books", "Whiteboard", …). Passed to
    // SubmitRequest as seed_need_names so the request is born with one need per
    // entry (#2702, #2731). Empty when the text named nothing concrete — the
    // request is then born with no needs for the compose flow to fill.
    @Default([]) List<String> seedNeeds,
  }) = _GenRequestState;

  const GenRequestState._();

  /// Check if we can proceed from the current step
  bool get canProceed {
    switch (currentStep) {
      case GenRequestStep.prompt:
        return false; // User must click "Generate"
      case GenRequestStep.generating:
        return false; // Auto-progresses when generation completes
      case GenRequestStep.preview:
        return ((editedTitle ?? generatedTitle)?.isNotEmpty ?? false) &&
            ((editedDescription ?? generatedDescription)?.isNotEmpty ?? false);
      case GenRequestStep.submitting:
        return false; // Auto-progresses when submission completes
      case GenRequestStep.completed:
        return false; // Final step
    }
  }

  /// Calculate progress percentage (0.0 to 1.0)
  double get progress =>
      completedSteps.length / GenRequestStep.values.length.toDouble();

  /// Check if workflow has completed
  bool get isCompleted => currentStep == GenRequestStep.completed;

  /// Check if there's an error
  bool get hasError => error != null;

  /// Get the final title (edited or generated)
  String? get finalTitle => editedTitle ?? generatedTitle;

  /// Get the final description (edited or generated)
  String? get finalDescription => editedDescription ?? generatedDescription;

  /// Get the final media ID (new or generated)
  String? get finalMediaId => newMediaId ?? generatedMediaId;

  /// Get the final location ID (selected or generated)
  String? get finalLocationId => selectedLocationId ?? generatedLocationId;
}

/// Notifier for managing the Gen Request workflow
class GenRequestNotifier extends Notifier<GenRequestState>
    with MediaUploadMixin<GenRequestState> {
  @override
  GenRequestState build() {
    return const GenRequestState();
  }

  RequestRepository get _requestRepository =>
      ref.read(requestRepositoryProvider);
  MediaRepository get _mediaRepository => ref.read(mediaRepositoryProvider);
  LocationRepository get _locationRepository =>
      ref.read(locationRepositoryProvider);

  // MediaUploadMixin implementation
  @override
  MediaRepository get mediaRepository => _mediaRepository;

  @override
  Logger get log => _log;

  @override
  String get mediaUploadDescription => 'Request media';

  @override
  void onMediaUploadStarted() {
    state = state.copyWith(isUploadingMedia: true, uploadError: null);
  }

  @override
  void onMediaUploaded(String mediaId) {
    state = state.copyWith(newMediaId: mediaId, isUploadingMedia: false);
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
    state = state.copyWith(
      newMediaId: mediaIds.isNotEmpty ? mediaIds.last : null,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount) {
    state = state.copyWith(
      newMediaId: uploadedIds.isNotEmpty ? uploadedIds.last : null,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: failedCount,
    );
  }

  /// Set (or clear) the optional needed-by date for the request. [unixSec] null
  /// clears it; a positive value places the request on the Home calendar.
  void setNeededBy(int? unixSec) {
    state = state.copyWith(neededByUnixSec: unixSec);
  }

  /// Update the edited title
  void updateTitle(String title) {
    _log.info('✏️ Updating request title to: "$title"');
    state = state.copyWith(editedTitle: title);
    _log.info(
      '✏️ State after update - editedTitle: "${state.editedTitle}", finalTitle: "${state.finalTitle}"',
    );
  }

  /// Replace the seeded-needs list shown on the preview (#2702, #2731). Blank
  /// entries are dropped; an empty list means the request is born with no needs.
  void updateSeedNeeds(List<String> seedNeeds) {
    state = state.copyWith(
      seedNeeds: [
        for (final n in seedNeeds)
          if (n.trim().isNotEmpty) n.trim(),
      ],
    );
  }

  
  
  void updateDescription(String description) {
    _log.info('✏️ Updating request description to: "$description"');
    state = state.copyWith(editedDescription: description);
    _log.info(
      '✏️ State after update - editedDescription: "${state.editedDescription}", finalDescription: "${state.finalDescription}"',
    );
  }

  /// Update the selected location ID and fetch the resolved [Location]
  /// so the preview can render its formatted name without keeping its own
  /// copy of the data. The selected ID lands in state immediately so the
  /// row can show a loading affordance; the resolved [Location] lands in a
  /// second `copyWith` after the repository fetch.
  Future<void> updateLocation(String locationId) async {
    _log.info('📍 Updating request location');
    // Step 1: synchronously reflect the new selection so callers and the
    // preview row know which location is active. Clear `geocodedLocation`
    // because the user has explicitly overridden the AI suggestion, and
    // clear any previously-resolved Location so the row can render its
    // loading state until the fetch lands.
    state = state.copyWith(
      selectedLocationId: locationId,
      resolvedLocation: null,
      geocodedLocation: null,
    );

    try {
      final location = await _locationRepository.getLocation(locationId);
      if (!ref.mounted) return;
      // Skip if the user has moved on to a different location while the
      // fetch was in flight.
      if (state.selectedLocationId != locationId) return;
      state = state.copyWith(resolvedLocation: location);
    } catch (e, stackTrace) {
      _log.warning('Failed to resolve location $locationId', e, stackTrace);
      // The selection itself stands; the preview row falls back to its
      // loading/unknown text until a successful resolve happens.
    }
  }

  /// Resolve the current location into [GenRequestState.resolvedLocation]
  /// if it isn't already. Idempotent; safe to call on every screen open.
  /// Prefers [selectedLocationId] over [generatedLocationId] (the primary
  /// residence used as AI context).
  Future<void> ensureLocationResolved() async {
    if (state.resolvedLocation != null) return;
    final id = state.selectedLocationId ?? state.generatedLocationId;
    if (id == null) return;
    try {
      final location = await _locationRepository.getLocation(id);
      if (!ref.mounted) return;
      // Another path may have populated resolvedLocation while we were
      // fetching; defer to it.
      if (state.resolvedLocation != null) return;
      // The active selection may have shifted to a different ID; defer
      // to the newer fetch path.
      final activeId = state.selectedLocationId ?? state.generatedLocationId;
      if (activeId != id) return;
      state = state.copyWith(resolvedLocation: location);
    } catch (e, stackTrace) {
      _log.warning(
        'Failed to resolve initial location $id',
        e,
        stackTrace,
      );
    }
  }

  /// Set generated data from external source (e.g., RequestCreationModal).
  ///
  /// This method allows populating the preview state with data generated
  /// from a different flow (like the camera/text input modal).
  void setGeneratedData({
    required String title,
    required String description,
    List<String>? mediaIds,
    String? locationId,
    GeocodedLocation? geocodedLocation,
    List<String>? tags,
    RequestMetadata? metadata,
    List<String>? additionalAsks,
    List<String>? breakdownPieces,
    List<String>? offerIdeas,
    List<String>? seedNeeds,
  }) {
    _log.info('📝 Setting generated data from external source');
    _log.info(
      '📍 geocodedLocation: ${geocodedLocation?.name ?? "null"}, locationId: ${locationId ?? "null"}',
    );
    // Use first media ID for backward compatibility with single mediaId field
    final mediaId = mediaIds?.firstOrNull;
    state = state.copyWith(
      generatedTitle: title,
      generatedDescription: description,
      generatedMediaId: mediaId,
      generatedLocationId: locationId,
      geocodedLocation: geocodedLocation,
      generatedTags: tags ?? [],
      metadata: metadata,
      additionalAsks: additionalAsks ?? const [],
      breakdownPieces: breakdownPieces ?? const [],
      offerIdeas: offerIdeas ?? const [],
      seedNeeds: seedNeeds ?? const [],
      currentStep: GenRequestStep.preview,
    );
  }

  // Media upload methods provided by MediaUploadMixin:
  // - pickAndUploadImageFromGallery()
  // - pickAndUploadImageFromCamera()
  // Note: Requests only support images, not videos, so pickAndUploadVideoFromGallery() is not used.

  /// Submit request to server for each of the specified communities.
  Future<void> submitRequest(List<String> communityIds) async {
    _log.info('📤 submitRequest called - State before submission:');
    _log.info('  generatedTitle: "${state.generatedTitle}"');
    _log.info('  editedTitle: "${state.editedTitle}"');
    _log.info('  finalTitle: "${state.finalTitle}"');
    _log.info('  generatedDescription: "${state.generatedDescription}"');
    _log.info('  editedDescription: "${state.editedDescription}"');
    _log.info('  finalDescription: "${state.finalDescription}"');

    final title = state.finalTitle;
    final description = state.finalDescription;

    _log.info('📤 Using title: "$title", description: "$description"');

    if (title == null || title.isEmpty) {
      _setError('Title cannot be empty', GenRequestStep.preview);
      return;
    }

    if (description == null || description.isEmpty) {
      _setError('Description cannot be empty', GenRequestStep.preview);
      return;
    }

    _log.info('📤 Submitting request with validated title and description...');
    state = state.copyWith(
      currentStep: GenRequestStep.submitting,
      isLoading: true,
      error: null,
      errorStep: null,
    );

    try {
      // Step 1: Save geocoded location if needed
      String? locationId = state.selectedLocationId;
      if (locationId == null && state.geocodedLocation != null) {
        // Save the AI-extracted geocoded location
        _log.info(
          '📍 Saving AI-generated geocoded location: ${state.geocodedLocation!.name}',
        );
        final locationService = ref.read(locationServiceProvider);
        locationId = await locationService.saveLocation(
          name: state.geocodedLocation!.name,
          latitudeDeg: state.geocodedLocation!.latitudeDeg,
          longitudeDeg: state.geocodedLocation!.longitudeDeg,
          locality: state.geocodedLocation!.locality,
          regionCode: state.geocodedLocation!.regionCode,
          postalCode: state.geocodedLocation!.postalCode,
          addressLines: state.geocodedLocation!.addressLines,
          neighborhood: state.geocodedLocation!.neighborhood.isNotEmpty
              ? state.geocodedLocation!.neighborhood
              : null,
          county: state.geocodedLocation!.county.isNotEmpty
              ? state.geocodedLocation!.county
              : null,
          administrativeArea:
              state.geocodedLocation!.administrativeArea.isNotEmpty
                  ? state.geocodedLocation!.administrativeArea
                  : null,
          externalPlaceId: state.geocodedLocation!.externalPlaceId.isNotEmpty
              ? state.geocodedLocation!.externalPlaceId
              : null,
          externalPlaceProvider:
              state.geocodedLocation!.externalPlaceProvider.isNotEmpty
                  ? state.geocodedLocation!.externalPlaceProvider
                  : null,
        );
        _log.info('📍 Location saved with ID: $locationId');
      } else if (locationId == null && state.generatedLocationId != null) {
        // Fallback to generatedLocationId (user's primary residence used for AI context)
        locationId = state.generatedLocationId;
        _log.info('📍 Using fallback location ID: $locationId');
      }

      // Step 2: Create one Request (in its own per-item community), then share
      // it into the selected existing communities via ShareItem (#2529).
      // Submitting per-community would create duplicate Request records — each
      // with its own conversation and community link — so we submit once and
      // share the rest.
      final mediaIds = state.finalMediaId != null
          ? [state.finalMediaId!]
          : null;
      final requestId = await _requestRepository.submitRequest(
        title: title,
        description: description,
        mediaIds: mediaIds,
        locationId: locationId,
        metadata: state.metadata,
        neededByUnixSec: state.neededByUnixSec,
        // Send the names only when there are any; an empty list means the
        // request is born with no needs, same as omitting the field.
        seedNeedNames: state.seedNeeds.isEmpty ? null : state.seedNeeds,
      );
      _log.info('✅ Request submitted: $requestId');
      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            RequestCreatedEvent(
              communityId: communityIds.first,
              hasLocation: locationId != null,
            ),
          ));

      if (communityIds.isNotEmpty) {
        await _requestRepository.shareRequest(
          requestId: requestId,
          communityIds: communityIds,
        );
        _log.info(
          '✅ Request shared with ${communityIds.length} '
          'communities: $communityIds',
        );
      }

      state = state.copyWith(
        createdRequestId: requestId,
        completedSteps: [...state.completedSteps, GenRequestStep.submitting],
        currentStep: GenRequestStep.completed,
        isLoading: false,
      );
    } catch (e, stackTrace) {
      _log.severe('❌ Submission failed: $e', e, stackTrace);
      _setError('Failed to submit request: $e', GenRequestStep.submitting);
    }
  }

  /// Reset the workflow to start
  void reset() {
    _log.info('🔄 Resetting workflow');
    state = const GenRequestState();
  }

  /// Marks the workflow as actively streaming AI output. Called once when the
  /// streaming generation starts so the preview modal can show skeleton state
  /// while partial fields populate.
  void beginStreaming() {
    state = state.copyWith(
      currentStep: GenRequestStep.generating,
      error: null,
      errorStep: null,
      generatedTitle: null,
      generatedDescription: null,
      generatedMediaId: null,
      geocodedLocation: null,
      locationQuery: null,
      generatedTags: const [],
      metadata: null,
      editedTitle: null,
      editedDescription: null,
      resolvedLocation: null,
    );
  }

  /// Applies an early `title` event from the streaming generator.
  void applyStreamingTitle(String title) {
    if (title.isEmpty) return;
    state = state.copyWith(generatedTitle: title);
  }

  /// Applies an early `description` event from the streaming generator. The
  /// server emits this immediately at stream open (description = prompt
  /// verbatim) so the preview modal can render it before the AI responds.
  void applyStreamingDescription(String description) {
    if (description.isEmpty) return;
    state = state.copyWith(generatedDescription: description);
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
    state = state.copyWith(generatedMediaId: mediaIds.first);
  }

  /// Applies the terminal `final` event from the streaming generator.
  void finalizeStreaming({
    required String title,
    required String description,
    List<String> mediaIds = const [],
    List<String> tags = const [],
    String? locationQuery,
    GeocodedLocation? geocodedLocation,
    RequestMetadata? metadata,
    List<String> additionalAsks = const [],
    List<String> breakdownPieces = const [],
    List<String> offerIdeas = const [],
    List<String> seedNeeds = const [],
  }) {
    setGeneratedData(
      title: title,
      description: description,
      mediaIds: mediaIds,
      geocodedLocation: geocodedLocation,
      tags: tags,
      metadata: metadata,
      additionalAsks: additionalAsks,
      breakdownPieces: breakdownPieces,
      offerIdeas: offerIdeas,
      seedNeeds: seedNeeds,
    );
  }

  /// Applies the terminal `error` event from the streaming generator.
  void failStreaming(String message) {
    _setError(message, GenRequestStep.generating);
  }

  /// Gets media URL for a given media ID via MediaRepository.
  ///
  /// The repository handles caching, so this is just a thin wrapper.
  /// Returns the MediaUrl object containing both URL and cache key.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);

  void _setError(String message, GenRequestStep step) {
    _log.warning('⚠️ Error at step $step: $message');
    state = state.copyWith(
      isLoading: false,
      error: UserError.generic(fallback: message),
      errorStep: step,
      currentStep: step,
    );
  }
}

/// Provider for the Gen Request workflow
/// Note: Not using autoDispose because this workflow spans multiple screens/dialogs
final genRequestProvider =
    NotifierProvider<GenRequestNotifier, GenRequestState>(
      GenRequestNotifier.new,
    );
